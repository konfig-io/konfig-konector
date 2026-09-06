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
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsiam "github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	iamhelper "github.com/konfig-io/konfig-konector/internal/aws/iam"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
)

// IAMPolicyAWSAPI is the subset of the IAM SDK client used by this controller
// (directly and via the iamhelper managed-policy functions). *iam.Client satisfies it.
type IAMPolicyAWSAPI interface {
	iamhelper.PolicyAPI
	CreatePolicy(ctx context.Context, params *awsiam.CreatePolicyInput, optFns ...func(*awsiam.Options)) (*awsiam.CreatePolicyOutput, error)
}

// IAMPolicyReconciler reconciles IAMPolicy objects.
type IAMPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	IAMClient IAMPolicyAWSAPI
	// AccountID is the operator's own AWS account ID, used to construct policy
	// ARNs when the resource is not scoped to another AWSProvider.
	AccountID string
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=iampolicies/finalizers,verbs=update

func (r *IAMPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	p := &awsv1alpha1.IAMPolicy{}
	if err := r.Get(ctx, req.NamespacedName, p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, p); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !p.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(p, awsv1alpha1.FinalizerName) {
			if shouldAbandon(p) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(p, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, p)
			}
			arn := p.Status.ARN
			if arn == "" {
				// Status may have been lost before it was persisted; the policy
				// ARN is deterministically constructible from the spec (same
				// construction used by the adopt-on-EntityAlreadyExists path).
				constructed, err := r.lookupPolicyARNFromSpec(ctx, p)
				if err != nil {
					return ctrl.Result{}, err
				}
				arn = constructed
			}
			if arn != "" {
				if err := iamhelper.DeletePolicy(ctx, r.IAMClient, arn); err != nil {
					logger.Error(err, "failed to delete IAM policy", "arn", arn)
					return ctrl.Result{}, err
				}
			}
			controllerutil.RemoveFinalizer(p, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, p)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(p, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(p, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, p); err != nil {
			return ctrl.Result{}, err
		}
		// Return and let the update event drive the next reconcile: creating the
		// AWS resource in this pass races the stale-cache reconcile queued by the
		// finalizer update and produces duplicate creates (AlreadyExists).
		return ctrl.Result{Requeue: true}, nil
	}

	if err := r.reconcilePolicy(ctx, p); err != nil {
		logger.Error(err, "reconcile error", "policyName", p.Spec.PolicyName)
		_ = r.setPolicyCondition(ctx, p, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *IAMPolicyReconciler) reconcilePolicy(ctx context.Context, p *awsv1alpha1.IAMPolicy) error {
	var existing interface{}
	if p.Status.ARN != "" {
		pol, err := iamhelper.GetPolicy(ctx, r.IAMClient, p.Status.ARN)
		if err != nil {
			return err
		}
		existing = pol
	}

	if existing == nil {
		path := p.Spec.Path
		if path == "" {
			path = "/"
		}
		out, err := r.IAMClient.CreatePolicy(ctx, &awsiam.CreatePolicyInput{
			PolicyName:     aws.String(p.Spec.PolicyName),
			Description:    aws.String(p.Spec.Description),
			Path:           aws.String(path),
			PolicyDocument: aws.String(p.Spec.PolicyDocument),
		})
		if err != nil {
			// If the policy already exists in AWS but we have no ARN in status,
			// import it by looking it up by name.
			var already *iamtypes.EntityAlreadyExistsException
			if errors.As(err, &already) {
				arn := fmt.Sprintf("arn:aws:iam::%s:policy%s%s", r.accountID(ctx), path, p.Spec.PolicyName)
				pol, lookupErr := iamhelper.GetPolicy(ctx, r.IAMClient, arn)
				if lookupErr != nil || pol == nil {
					return fmt.Errorf("create policy: %w", err)
				}
				p.Status.ARN = aws.ToString(pol.Arn)
				p.Status.PolicyID = aws.ToString(pol.PolicyId)
				p.Status.DefaultVersionID = aws.ToString(pol.DefaultVersionId)
			} else {
				return fmt.Errorf("create policy: %w", err)
			}
		} else {
			p.Status.ARN = aws.ToString(out.Policy.Arn)
			p.Status.PolicyID = aws.ToString(out.Policy.PolicyId)
			p.Status.DefaultVersionID = aws.ToString(out.Policy.DefaultVersionId)
		}
		// Persist the ARN immediately: the AWS resource now exists (created or
		// adopted), and losing the identifier would orphan it on retry.
		if err := persistStatus(ctx, r.Client, p); err != nil {
			return fmt.Errorf("persist policy ARN after create: %w", err)
		}
	} else {
		// Check if document changed.
		currentDoc, err := iamhelper.GetPolicyDocument(ctx, r.IAMClient, p.Status.ARN, p.Status.DefaultVersionID)
		if err != nil {
			return err
		}
		// URL-decode the document AWS returns.
		decoded, _ := url.QueryUnescape(currentDoc)
		if decoded != p.Spec.PolicyDocument {
			newVersion, err := iamhelper.UpdatePolicyDocument(ctx, r.IAMClient, p.Status.ARN, p.Spec.PolicyDocument)
			if err != nil {
				return fmt.Errorf("update policy document: %w", err)
			}
			p.Status.DefaultVersionID = newVersion
		}
	}

	now := metav1.Now()
	p.Status.LastSyncTime = &now
	p.Status.ObservedGeneration = p.Generation
	return r.setPolicyCondition(ctx, p, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "IAM policy reconciled")
}

// lookupPolicyARNFromSpec constructs the policy ARN from account ID, path, and
// policy name (mirroring the adopt-by-constructed-ARN pattern on create) and
// verifies it exists. Returns "" if the account ID is unknown or the policy
// does not exist.
// accountID returns the account the current reconcile targets: the
// AWSProvider scope's account when one is attached to ctx, else the operator's.
func (r *IAMPolicyReconciler) accountID(ctx context.Context) string {
	if s := provider.ScopeFrom(ctx); s != nil && s.AccountID != "" {
		return s.AccountID
	}
	return r.AccountID
}

func (r *IAMPolicyReconciler) lookupPolicyARNFromSpec(ctx context.Context, p *awsv1alpha1.IAMPolicy) (string, error) {
	if r.accountID(ctx) == "" || p.Spec.PolicyName == "" {
		return "", nil
	}
	path := p.Spec.Path
	if path == "" {
		path = "/"
	}
	arn := fmt.Sprintf("arn:aws:iam::%s:policy%s%s", r.accountID(ctx), path, p.Spec.PolicyName)
	pol, err := iamhelper.GetPolicy(ctx, r.IAMClient, arn)
	if err != nil {
		return "", err
	}
	if pol == nil {
		return "", nil
	}
	return aws.ToString(pol.Arn), nil
}

func (r *IAMPolicyReconciler) setPolicyCondition(ctx context.Context, p *awsv1alpha1.IAMPolicy, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: p.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, p); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *IAMPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.IAMPolicy{}).
		Complete(r)
}
