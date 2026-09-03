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
	awsnfw "github.com/aws/aws-sdk-go-v2/service/networkfirewall"
	nfwtypes "github.com/aws/aws-sdk-go-v2/service/networkfirewall/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	nfwhelper "github.com/konfig-io/konfig-konector/internal/aws/networkfirewall"
)

// requeueNetworkFirewallPolling is the requeue interval while a firewall is provisioning.
var requeueNetworkFirewallPolling = ctrl.Result{RequeueAfter: 30 * time.Second}

// FirewallAWSAPI is the subset of the Network Firewall API used by this controller.
type FirewallAWSAPI interface {
	DescribeFirewall(ctx context.Context, params *awsnfw.DescribeFirewallInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DescribeFirewallOutput, error)
	CreateFirewall(ctx context.Context, params *awsnfw.CreateFirewallInput, optFns ...func(*awsnfw.Options)) (*awsnfw.CreateFirewallOutput, error)
	DeleteFirewall(ctx context.Context, params *awsnfw.DeleteFirewallInput, optFns ...func(*awsnfw.Options)) (*awsnfw.DeleteFirewallOutput, error)
	UpdateFirewallDeleteProtection(ctx context.Context, params *awsnfw.UpdateFirewallDeleteProtectionInput, optFns ...func(*awsnfw.Options)) (*awsnfw.UpdateFirewallDeleteProtectionOutput, error)
	TagResource(ctx context.Context, params *awsnfw.TagResourceInput, optFns ...func(*awsnfw.Options)) (*awsnfw.TagResourceOutput, error)
}

// FirewallReconciler reconciles Firewall objects.
type FirewallReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	NetworkFirewallClient FirewallAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewalls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewalls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=firewalls/finalizers,verbs=update

func (r *FirewallReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.Firewall{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deleteFirewall(ctx, obj); err != nil {
				logger.Error(err, "failed to delete Firewall")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, obj)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(obj, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileFirewall(ctx, obj)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *FirewallReconciler) reconcileFirewall(ctx context.Context, obj *awsv1alpha1.Firewall) (ctrl.Result, error) {
	descOut, err := r.NetworkFirewallClient.DescribeFirewall(ctx, &awsnfw.DescribeFirewallInput{
		FirewallName: aws.String(obj.Spec.Name),
	})
	if err != nil && !nfwhelper.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("describe firewall: %w", err)
	}

	if nfwhelper.IsNotFound(err) {
		policyARN, err := r.resolvePolicyARN(ctx, obj)
		if err != nil {
			return ctrl.Result{}, err
		}
		vpcID, err := r.resolveVPCIDFirewall(ctx, obj.Namespace, &obj.Spec.VPCRef)
		if err != nil {
			return ctrl.Result{}, err
		}
		subnetIDs, err := resolveSubnetIDs(ctx, r.Client, obj.Namespace, obj.Spec.SubnetRefs)
		if err != nil {
			return ctrl.Result{}, err
		}
		mappings := make([]nfwtypes.SubnetMapping, 0, len(subnetIDs))
		for _, id := range subnetIDs {
			id := id
			mappings = append(mappings, nfwtypes.SubnetMapping{SubnetId: &id})
		}
		input := &awsnfw.CreateFirewallInput{
			FirewallName:      aws.String(obj.Spec.Name),
			FirewallPolicyArn: aws.String(policyARN),
			VpcId:             aws.String(vpcID),
			SubnetMappings:    mappings,
			DeleteProtection:  obj.Spec.DeleteProtection,
			Tags:              nfwTagsFromMap(obj.Spec.Tags),
		}
		if obj.Spec.Description != "" {
			input.Description = aws.String(obj.Spec.Description)
		}
		out, err := r.NetworkFirewallClient.CreateFirewall(ctx, input)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create firewall: %w", err)
		}
		obj.Status.ARN = aws.ToString(out.Firewall.FirewallArn)
		obj.Status.ID = aws.ToString(out.Firewall.FirewallId)
		if out.FirewallStatus != nil {
			obj.Status.State = string(out.FirewallStatus.Status)
		}
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist firewall ARN after create: %w", err)
		}
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "Firewall is being provisioned")
		return requeueNetworkFirewallPolling, nil
	}

	obj.Status.ARN = aws.ToString(descOut.Firewall.FirewallArn)
	obj.Status.ID = aws.ToString(descOut.Firewall.FirewallId)
	state := ""
	if descOut.FirewallStatus != nil {
		state = string(descOut.FirewallStatus.Status)
	}
	obj.Status.State = state

	if state != string(nfwtypes.FirewallStatusValueReady) {
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", fmt.Sprintf("Firewall state is %s", state))
		return requeueNetworkFirewallPolling, nil
	}

	// Update mutable attributes only when the spec changed since last sync.
	if obj.Status.ObservedGeneration != obj.Generation {
		if descOut.Firewall.DeleteProtection != obj.Spec.DeleteProtection {
			if _, err := r.NetworkFirewallClient.UpdateFirewallDeleteProtection(ctx, &awsnfw.UpdateFirewallDeleteProtectionInput{
				FirewallArn:      descOut.Firewall.FirewallArn,
				DeleteProtection: obj.Spec.DeleteProtection,
				UpdateToken:      descOut.UpdateToken,
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("update firewall delete protection: %w", err)
			}
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.NetworkFirewallClient.TagResource(ctx, &awsnfw.TagResourceInput{
				ResourceArn: descOut.Firewall.FirewallArn,
				Tags:        nfwTagsFromMap(obj.Spec.Tags),
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("tag firewall: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "Firewall ready"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *FirewallReconciler) resolvePolicyARN(ctx context.Context, obj *awsv1alpha1.Firewall) (string, error) {
	ref := obj.Spec.FirewallPolicyRef
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("firewallPolicyRef requires name or arn")
	}
	fp := &awsv1alpha1.FirewallPolicy{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: obj.Namespace}, fp); err != nil {
		return "", err
	}
	if fp.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("FirewallPolicy %s/%s has no ARN yet", obj.Namespace, ref.Name)}
	}
	return fp.Status.ARN, nil
}

func (r *FirewallReconciler) resolveVPCIDFirewall(ctx context.Context, namespace string, ref *awsv1alpha1.VPCResourceRef) (string, error) {
	if ref.ID != "" {
		return ref.ID, nil
	}
	if ref.Name == "" {
		return "", fmt.Errorf("vpcRef requires name or id")
	}
	vpc := &awsv1alpha1.VPC{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, vpc); err != nil {
		return "", err
	}
	if vpc.Status.VPCID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("VPC %s/%s has no vpcId yet", namespace, ref.Name)}
	}
	return vpc.Status.VPCID, nil
}

func (r *FirewallReconciler) deleteFirewall(ctx context.Context, obj *awsv1alpha1.Firewall) error {
	input := &awsnfw.DeleteFirewallInput{}
	if obj.Status.ARN != "" {
		input.FirewallArn = aws.String(obj.Status.ARN)
	} else {
		// Name is a deterministic identifier for firewalls.
		input.FirewallName = aws.String(obj.Spec.Name)
	}
	_, err := r.NetworkFirewallClient.DeleteFirewall(ctx, input)
	if nfwhelper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FirewallReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.Firewall, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *FirewallReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Firewall{}).
		Complete(r)
}
