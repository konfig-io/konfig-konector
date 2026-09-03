/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsmq "github.com/aws/aws-sdk-go-v2/service/mq"
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	mqhelper "github.com/konfig-io/konfig-konector/internal/aws/mq"
)

var requeueMQPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// MQBrokerAWSAPI is the subset of the Amazon MQ API used by this controller.
type MQBrokerAWSAPI interface {
	CreateBroker(ctx context.Context, params *awsmq.CreateBrokerInput, optFns ...func(*awsmq.Options)) (*awsmq.CreateBrokerOutput, error)
	DescribeBroker(ctx context.Context, params *awsmq.DescribeBrokerInput, optFns ...func(*awsmq.Options)) (*awsmq.DescribeBrokerOutput, error)
	UpdateBroker(ctx context.Context, params *awsmq.UpdateBrokerInput, optFns ...func(*awsmq.Options)) (*awsmq.UpdateBrokerOutput, error)
	DeleteBroker(ctx context.Context, params *awsmq.DeleteBrokerInput, optFns ...func(*awsmq.Options)) (*awsmq.DeleteBrokerOutput, error)
	ListBrokers(ctx context.Context, params *awsmq.ListBrokersInput, optFns ...func(*awsmq.Options)) (*awsmq.ListBrokersOutput, error)
}

// MQBrokerReconciler reconciles MQBroker objects.
type MQBrokerReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	MQClient MQBrokerAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqbrokers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqbrokers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=mqbrokers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *MQBrokerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	broker := &awsv1alpha1.MQBroker{}
	if err := r.Get(ctx, req.NamespacedName, broker); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !broker.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(broker, awsv1alpha1.FinalizerName) {
			if shouldAbandon(broker) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(broker, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, broker)
			}
			if err := r.deleteBroker(ctx, broker); err != nil {
				logger.Error(err, "failed to delete MQ broker")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(broker, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, broker)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(broker, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(broker, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, broker); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileBroker(ctx, broker)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, broker, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *MQBrokerReconciler) reconcileBroker(ctx context.Context, broker *awsv1alpha1.MQBroker) (ctrl.Result, error) {
	if broker.Status.BrokerID != "" {
		out, err := r.MQClient.DescribeBroker(ctx, &awsmq.DescribeBrokerInput{
			BrokerId: aws.String(broker.Status.BrokerID),
		})
		if err != nil && !mqhelper.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			broker.Status.BrokerState = string(out.BrokerState)
			if out.BrokerArn != nil {
				broker.Status.BrokerARN = *out.BrokerArn
			}

			if out.BrokerState != mqtypes.BrokerStateRunning {
				_ = r.setCondition(ctx, broker, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
					fmt.Sprintf("MQ broker is %s", out.BrokerState))
				return requeueMQPolling, nil
			}

			if broker.Status.ObservedGeneration != broker.Generation {
				sgIDs, err := resolveSGIDs(ctx, r.Client, broker.Namespace, broker.Spec.SecurityGroupRefs)
				if err != nil {
					return ctrl.Result{}, err
				}
				updateIn := &awsmq.UpdateBrokerInput{
					BrokerId:                aws.String(broker.Status.BrokerID),
					AutoMinorVersionUpgrade: aws.Bool(broker.Spec.AutoMinorVersionUpgrade),
					HostInstanceType:        aws.String(broker.Spec.HostInstanceType),
				}
				if broker.Spec.EngineVersion != "" {
					updateIn.EngineVersion = aws.String(broker.Spec.EngineVersion)
				}
				if len(sgIDs) > 0 {
					updateIn.SecurityGroups = sgIDs
				}
				if _, err := r.MQClient.UpdateBroker(ctx, updateIn); err != nil {
					return ctrl.Result{}, fmt.Errorf("update MQ broker: %w", err)
				}
			}

			broker.Status.ObservedGeneration = broker.Generation
			now := metav1.Now()
			broker.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, broker, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "MQ broker running")
		}
		// NotFound: fall through to create.
	}

	subnetIDs, err := resolveSubnetIDs(ctx, r.Client, broker.Namespace, broker.Spec.SubnetRefs)
	if err != nil {
		return ctrl.Result{}, err
	}
	sgIDs, err := resolveSGIDs(ctx, r.Client, broker.Namespace, broker.Spec.SecurityGroupRefs)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Resolve user passwords from Secrets. Passwords are only ever passed to
	// the AWS API; never stored in status or logged.
	users := make([]mqtypes.User, 0, len(broker.Spec.Users))
	for _, u := range broker.Spec.Users {
		password, err := resolveSecretValue(ctx, r.Client, broker.Namespace, u.PasswordRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		user := mqtypes.User{
			Username: aws.String(u.Username),
			Password: aws.String(password),
		}
		if u.ConsoleAccess {
			user.ConsoleAccess = aws.Bool(true)
		}
		if len(u.Groups) > 0 {
			user.Groups = u.Groups
		}
		users = append(users, user)
	}

	createIn := &awsmq.CreateBrokerInput{
		BrokerName:              aws.String(broker.Spec.BrokerName),
		EngineType:              mqtypes.EngineType(broker.Spec.EngineType),
		HostInstanceType:        aws.String(broker.Spec.HostInstanceType),
		DeploymentMode:          mqtypes.DeploymentMode(broker.Spec.DeploymentMode),
		PubliclyAccessible:      aws.Bool(broker.Spec.PubliclyAccessible),
		AutoMinorVersionUpgrade: aws.Bool(broker.Spec.AutoMinorVersionUpgrade),
		Users:                   users,
	}
	if broker.Spec.EngineVersion != "" {
		createIn.EngineVersion = aws.String(broker.Spec.EngineVersion)
	}
	if len(subnetIDs) > 0 {
		createIn.SubnetIds = subnetIDs
	}
	if len(sgIDs) > 0 {
		createIn.SecurityGroups = sgIDs
	}
	if len(broker.Spec.Tags) > 0 {
		createIn.Tags = broker.Spec.Tags
	}

	created, err := r.MQClient.CreateBroker(ctx, createIn)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("create MQ broker: %w", err)
	}
	broker.Status.BrokerID = aws.ToString(created.BrokerId)
	broker.Status.BrokerARN = aws.ToString(created.BrokerArn)
	broker.Status.BrokerState = string(mqtypes.BrokerStateCreationInProgress)
	// Persist the broker ID immediately: the AWS resource now exists, and
	// losing the identifier would orphan it or create a duplicate on retry.
	if err := persistStatus(ctx, r.Client, broker); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist broker ID after create: %w", err)
	}
	_ = r.setCondition(ctx, broker, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "MQ broker is CREATION_IN_PROGRESS")
	return requeueMQPolling, nil
}

func (r *MQBrokerReconciler) deleteBroker(ctx context.Context, broker *awsv1alpha1.MQBroker) error {
	brokerID := broker.Status.BrokerID
	if brokerID == "" {
		// Broker names are unique per account: look the ID up by name.
		out, err := r.MQClient.ListBrokers(ctx, &awsmq.ListBrokersInput{})
		if err != nil {
			return err
		}
		for _, s := range out.BrokerSummaries {
			if aws.ToString(s.BrokerName) == broker.Spec.BrokerName {
				brokerID = aws.ToString(s.BrokerId)
				break
			}
		}
		if brokerID == "" {
			return nil
		}
	}
	_, err := r.MQClient.DeleteBroker(ctx, &awsmq.DeleteBrokerInput{
		BrokerId: aws.String(brokerID),
	})
	if mqhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *MQBrokerReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.MQBroker, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: obj.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, obj); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *MQBrokerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.MQBroker{}).
		Complete(r)
}
