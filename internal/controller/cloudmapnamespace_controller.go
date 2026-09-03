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
	awssd "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sdtypes "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	sdhelper "github.com/konfig-io/konfig-konector/internal/aws/servicediscovery"
)

var requeueCloudMapPolling = ctrl.Result{RequeueAfter: 10 * time.Second}

// CloudMapNamespaceAWSAPI is the subset of the Cloud Map API used by this controller.
type CloudMapNamespaceAWSAPI interface {
	CreatePrivateDnsNamespace(ctx context.Context, params *awssd.CreatePrivateDnsNamespaceInput, optFns ...func(*awssd.Options)) (*awssd.CreatePrivateDnsNamespaceOutput, error)
	CreatePublicDnsNamespace(ctx context.Context, params *awssd.CreatePublicDnsNamespaceInput, optFns ...func(*awssd.Options)) (*awssd.CreatePublicDnsNamespaceOutput, error)
	CreateHttpNamespace(ctx context.Context, params *awssd.CreateHttpNamespaceInput, optFns ...func(*awssd.Options)) (*awssd.CreateHttpNamespaceOutput, error)
	GetOperation(ctx context.Context, params *awssd.GetOperationInput, optFns ...func(*awssd.Options)) (*awssd.GetOperationOutput, error)
	GetNamespace(ctx context.Context, params *awssd.GetNamespaceInput, optFns ...func(*awssd.Options)) (*awssd.GetNamespaceOutput, error)
	DeleteNamespace(ctx context.Context, params *awssd.DeleteNamespaceInput, optFns ...func(*awssd.Options)) (*awssd.DeleteNamespaceOutput, error)
	ListNamespaces(ctx context.Context, params *awssd.ListNamespacesInput, optFns ...func(*awssd.Options)) (*awssd.ListNamespacesOutput, error)
}

// CloudMapNamespaceReconciler reconciles CloudMapNamespace objects.
type CloudMapNamespaceReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	ServiceDiscoveryClient CloudMapNamespaceAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=cloudmapnamespaces/finalizers,verbs=update

func (r *CloudMapNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	ns := &awsv1alpha1.CloudMapNamespace{}
	if err := r.Get(ctx, req.NamespacedName, ns); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !ns.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(ns, awsv1alpha1.FinalizerName) {
			if shouldAbandon(ns) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(ns, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, ns)
			}
			if err := r.deleteNamespace(ctx, ns); err != nil {
				logger.Error(err, "failed to delete Cloud Map namespace")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(ns, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, ns)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(ns, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(ns, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, ns); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileNamespace(ctx, ns)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *CloudMapNamespaceReconciler) reconcileNamespace(ctx context.Context, ns *awsv1alpha1.CloudMapNamespace) (ctrl.Result, error) {
	// Namespace already created and resolved.
	if ns.Status.NamespaceID != "" {
		ns.Status.ObservedGeneration = ns.Generation
		now := metav1.Now()
		ns.Status.LastSyncTime = &now
		return requeueResult(), r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Cloud Map namespace active")
	}

	// A create operation is in flight: poll it.
	if ns.Status.OperationID != "" {
		out, err := r.ServiceDiscoveryClient.GetOperation(ctx, &awssd.GetOperationInput{
			OperationId: aws.String(ns.Status.OperationID),
		})
		if err != nil {
			return ctrl.Result{}, err
		}
		if out.Operation == nil {
			return ctrl.Result{}, fmt.Errorf("GetOperation returned no operation for %s", ns.Status.OperationID)
		}
		switch out.Operation.Status {
		case sdtypes.OperationStatusSuccess:
			nsID := out.Operation.Targets[string(sdtypes.OperationTargetTypeNamespace)]
			ns.Status.NamespaceID = nsID
			ns.Status.OperationID = ""
			if got, err := r.ServiceDiscoveryClient.GetNamespace(ctx, &awssd.GetNamespaceInput{Id: aws.String(nsID)}); err == nil && got.Namespace != nil {
				ns.Status.ARN = aws.ToString(got.Namespace.Arn)
			}
			ns.Status.ObservedGeneration = ns.Generation
			now := metav1.Now()
			ns.Status.LastSyncTime = &now
			return requeueResult(), r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Cloud Map namespace active")
		case sdtypes.OperationStatusFail:
			ns.Status.OperationID = ""
			_ = persistStatus(ctx, r.Client, ns)
			return ctrl.Result{}, fmt.Errorf("create namespace operation failed: %s", aws.ToString(out.Operation.ErrorMessage))
		default:
			_ = r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning",
				fmt.Sprintf("Cloud Map namespace creation is %s", out.Operation.Status))
			return requeueCloudMapPolling, nil
		}
	}

	// Create.
	var operationID string
	switch ns.Spec.Type {
	case "PRIVATE_DNS":
		if ns.Spec.VPCRef == nil {
			return ctrl.Result{}, fmt.Errorf("vpcRef is required for PRIVATE_DNS namespaces")
		}
		vpcID, err := r.resolveVPCID(ctx, ns.Namespace, ns.Spec.VPCRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		in := &awssd.CreatePrivateDnsNamespaceInput{
			Name: aws.String(ns.Spec.Name),
			Vpc:  aws.String(vpcID),
		}
		if ns.Spec.Description != "" {
			in.Description = aws.String(ns.Spec.Description)
		}
		in.Tags = cloudMapTags(ns.Spec.Tags)
		out, err := r.ServiceDiscoveryClient.CreatePrivateDnsNamespace(ctx, in)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create private DNS namespace: %w", err)
		}
		operationID = aws.ToString(out.OperationId)
	case "PUBLIC_DNS":
		in := &awssd.CreatePublicDnsNamespaceInput{
			Name: aws.String(ns.Spec.Name),
		}
		if ns.Spec.Description != "" {
			in.Description = aws.String(ns.Spec.Description)
		}
		in.Tags = cloudMapTags(ns.Spec.Tags)
		out, err := r.ServiceDiscoveryClient.CreatePublicDnsNamespace(ctx, in)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create public DNS namespace: %w", err)
		}
		operationID = aws.ToString(out.OperationId)
	case "HTTP":
		in := &awssd.CreateHttpNamespaceInput{
			Name: aws.String(ns.Spec.Name),
		}
		if ns.Spec.Description != "" {
			in.Description = aws.String(ns.Spec.Description)
		}
		in.Tags = cloudMapTags(ns.Spec.Tags)
		out, err := r.ServiceDiscoveryClient.CreateHttpNamespace(ctx, in)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create HTTP namespace: %w", err)
		}
		operationID = aws.ToString(out.OperationId)
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported namespace type %q", ns.Spec.Type)
	}

	ns.Status.OperationID = operationID
	// Persist the operation ID immediately: the create is now in flight in
	// AWS, and losing the handle would start a duplicate create on retry.
	if err := persistStatus(ctx, r.Client, ns); err != nil {
		return ctrl.Result{}, fmt.Errorf("persist operation ID after create: %w", err)
	}
	_ = r.setCondition(ctx, ns, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Cloud Map namespace creation submitted")
	return requeueCloudMapPolling, nil
}

func cloudMapTags(tags map[string]string) []sdtypes.Tag {
	if len(tags) == 0 {
		return nil
	}
	out := make([]sdtypes.Tag, 0, len(tags))
	for k, v := range tags {
		k, v := k, v
		out = append(out, sdtypes.Tag{Key: &k, Value: &v})
	}
	return out
}

func (r *CloudMapNamespaceReconciler) resolveVPCID(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name != "" {
		vpcCR := &awsv1alpha1.VPC{}
		if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpcCR); err != nil {
			return "", err
		}
		if vpcCR.Status.VPCID == "" {
			return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no ID yet", namespace, ref.Name)}
		}
		return vpcCR.Status.VPCID, nil
	}
	return "", fmt.Errorf("vpcRef requires either name or id")
}

func (r *CloudMapNamespaceReconciler) deleteNamespace(ctx context.Context, ns *awsv1alpha1.CloudMapNamespace) error {
	namespaceID := ns.Status.NamespaceID
	if namespaceID == "" {
		// Look the namespace up by name; namespace names are unique per type.
		out, err := r.ServiceDiscoveryClient.ListNamespaces(ctx, &awssd.ListNamespacesInput{
			Filters: []sdtypes.NamespaceFilter{{
				Name:   sdtypes.NamespaceFilterNameName,
				Values: []string{ns.Spec.Name},
			}},
		})
		if err != nil {
			return err
		}
		for _, n := range out.Namespaces {
			if aws.ToString(n.Name) == ns.Spec.Name {
				namespaceID = aws.ToString(n.Id)
				break
			}
		}
		if namespaceID == "" {
			return nil
		}
	}
	_, err := r.ServiceDiscoveryClient.DeleteNamespace(ctx, &awssd.DeleteNamespaceInput{
		Id: aws.String(namespaceID),
	})
	if sdhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *CloudMapNamespaceReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.CloudMapNamespace, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *CloudMapNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.CloudMapNamespace{}).
		Complete(r)
}
