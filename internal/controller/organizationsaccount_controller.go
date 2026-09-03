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

	"github.com/aws/aws-sdk-go-v2/aws"
	awsorgs "github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	orghelper "github.com/konfig-io/konfig-konector/internal/aws/organizations"
)

// OrganizationsAccountAWSAPI is the subset of the Organizations API used by this controller.
type OrganizationsAccountAWSAPI interface {
	CreateAccount(ctx context.Context, params *awsorgs.CreateAccountInput, optFns ...func(*awsorgs.Options)) (*awsorgs.CreateAccountOutput, error)
	DescribeCreateAccountStatus(ctx context.Context, params *awsorgs.DescribeCreateAccountStatusInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DescribeCreateAccountStatusOutput, error)
	DescribeAccount(ctx context.Context, params *awsorgs.DescribeAccountInput, optFns ...func(*awsorgs.Options)) (*awsorgs.DescribeAccountOutput, error)
	MoveAccount(ctx context.Context, params *awsorgs.MoveAccountInput, optFns ...func(*awsorgs.Options)) (*awsorgs.MoveAccountOutput, error)
	ListParents(ctx context.Context, params *awsorgs.ListParentsInput, optFns ...func(*awsorgs.Options)) (*awsorgs.ListParentsOutput, error)
	CloseAccount(ctx context.Context, params *awsorgs.CloseAccountInput, optFns ...func(*awsorgs.Options)) (*awsorgs.CloseAccountOutput, error)
	TagResource(ctx context.Context, params *awsorgs.TagResourceInput, optFns ...func(*awsorgs.Options)) (*awsorgs.TagResourceOutput, error)
}

// OrganizationsAccountReconciler reconciles OrganizationsAccount objects.
//
// Account creation is asynchronous: CreateAccount returns a request ID which
// is polled via DescribeCreateAccountStatus. Deletion semantics: AWS accounts
// cannot be reliably deleted via API — CloseAccount is heavily rate-limited
// and irreversible — so on CR delete the account is closed only when
// spec.closeOnDelete is true; otherwise it is abandoned with a condition
// message recording that.
type OrganizationsAccountReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OrganizationsClient OrganizationsAccountAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsaccounts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=organizationsaccounts/finalizers,verbs=update

func (r *OrganizationsAccountReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	acct := &awsv1alpha1.OrganizationsAccount{}
	if err := r.Get(ctx, req.NamespacedName, acct); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !acct.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(acct, awsv1alpha1.FinalizerName) {
			if shouldAbandon(acct) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(acct, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, acct)
			}
			if acct.Spec.CloseOnDelete && acct.Status.AccountID != "" {
				if _, err := r.OrganizationsClient.CloseAccount(ctx, &awsorgs.CloseAccountInput{
					AccountId: aws.String(acct.Status.AccountID),
				}); err != nil && !orghelper.IsNotFound(err) && !orghelper.IsAccountAlreadyClosed(err) {
					logger.Error(err, "failed to close account")
					return ctrl.Result{}, err
				}
			} else if acct.Status.AccountID != "" {
				// AWS accounts cannot be deleted via API; without
				// closeOnDelete the account is abandoned intentionally.
				logger.Info("abandoning AWS account (spec.closeOnDelete is false)", "accountId", acct.Status.AccountID)
				_ = r.setCondition(ctx, acct, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonAbandoned,
					fmt.Sprintf("AWS account %s was not closed on CR deletion (spec.closeOnDelete=false); the account still exists", acct.Status.AccountID))
			}
			controllerutil.RemoveFinalizer(acct, awsv1alpha1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, acct)
		}
		return ctrl.Result{}, nil
	}

	if !controllerutil.ContainsFinalizer(acct, awsv1alpha1.FinalizerName) {
		controllerutil.AddFinalizer(acct, awsv1alpha1.FinalizerName)
		if err := r.Update(ctx, acct); err != nil {
			return ctrl.Result{}, err
		}
	}

	result, err := r.reconcileAccount(ctx, acct)
	if err != nil {
		var notReady *dependencyNotReady
		if errors.As(err, &notReady) {
			logger.Info("waiting for dependency", "reason", err.Error())
			return requeueDependency, nil
		}
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, acct, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return result, nil
}

func (r *OrganizationsAccountReconciler) resolveParent(ctx context.Context, acct *awsv1alpha1.OrganizationsAccount) (string, error) {
	if acct.Spec.ParentID != "" {
		return acct.Spec.ParentID, nil
	}
	if acct.Spec.ParentRef == nil || acct.Spec.ParentRef.Name == "" {
		return "", nil
	}
	ns := acct.Spec.ParentRef.Namespace
	if ns == "" {
		ns = acct.Namespace
	}
	parent := &awsv1alpha1.OrganizationsOU{}
	if err := r.Get(ctx, k8stypes.NamespacedName{Name: acct.Spec.ParentRef.Name, Namespace: ns}, parent); err != nil {
		return "", err
	}
	if parent.Status.OUID == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("OrganizationsOU %s/%s has no OU ID yet", ns, acct.Spec.ParentRef.Name)}
	}
	return parent.Status.OUID, nil
}

func (r *OrganizationsAccountReconciler) reconcileAccount(ctx context.Context, acct *awsv1alpha1.OrganizationsAccount) (ctrl.Result, error) {
	// Phase 1: kick off creation if the account does not exist and no create
	// request is in flight.
	if acct.Status.AccountID == "" && acct.Status.CreateAccountRequestID == "" {
		input := &awsorgs.CreateAccountInput{
			Email:       aws.String(acct.Spec.Email),
			AccountName: aws.String(acct.Spec.AccountName),
			Tags:        orgTags(acct.Spec.Tags),
		}
		if acct.Spec.RoleName != "" {
			input.RoleName = aws.String(acct.Spec.RoleName)
		}
		if acct.Spec.IAMUserAccessToBilling != "" {
			input.IamUserAccessToBilling = orgtypes.IAMUserAccessToBilling(acct.Spec.IAMUserAccessToBilling)
		}
		out, err := r.OrganizationsClient.CreateAccount(ctx, input)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("create account: %w", err)
		}
		// Persist the create request ID immediately: without it an operator
		// restart would kick off a second account creation.
		acct.Status.CreateAccountRequestID = aws.ToString(out.CreateAccountStatus.Id)
		acct.Status.State = string(out.CreateAccountStatus.State)
		if err := persistStatus(ctx, r.Client, acct); err != nil {
			return ctrl.Result{}, fmt.Errorf("persist create request ID after create: %w", err)
		}
	}

	// Phase 2: poll the async creation until it completes.
	if acct.Status.AccountID == "" {
		out, err := r.OrganizationsClient.DescribeCreateAccountStatus(ctx, &awsorgs.DescribeCreateAccountStatusInput{
			CreateAccountRequestId: aws.String(acct.Status.CreateAccountRequestID),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("describe create account status: %w", err)
		}
		st := out.CreateAccountStatus
		acct.Status.State = string(st.State)
		switch st.State {
		case orgtypes.CreateAccountStateInProgress:
			if err := r.setCondition(ctx, acct, awsv1alpha1.ConditionReady, metav1.ConditionFalse, "Provisioning", "account creation in progress"); err != nil {
				return ctrl.Result{}, err
			}
			return requeueOrgPolling, nil
		case orgtypes.CreateAccountStateFailed:
			// Clear the request ID so a subsequent reconcile can retry after
			// the failure cause (e.g. duplicate email) is fixed.
			reason := string(st.FailureReason)
			acct.Status.CreateAccountRequestID = ""
			_ = persistStatus(ctx, r.Client, acct)
			return ctrl.Result{}, fmt.Errorf("account creation failed: %s", reason)
		case orgtypes.CreateAccountStateSucceeded:
			acct.Status.AccountID = aws.ToString(st.AccountId)
			if err := persistStatus(ctx, r.Client, acct); err != nil {
				return ctrl.Result{}, fmt.Errorf("persist account ID after create: %w", err)
			}
		}
	}

	// Phase 3: steady state — verify the account, move under the desired
	// parent, and refresh status.
	desc, err := r.OrganizationsClient.DescribeAccount(ctx, &awsorgs.DescribeAccountInput{
		AccountId: aws.String(acct.Status.AccountID),
	})
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("describe account: %w", err)
	}
	acct.Status.ARN = aws.ToString(desc.Account.Arn)

	parentID, err := r.resolveParent(ctx, acct)
	if err != nil {
		return ctrl.Result{}, err
	}
	if parentID != "" {
		parents, err := r.OrganizationsClient.ListParents(ctx, &awsorgs.ListParentsInput{
			ChildId: aws.String(acct.Status.AccountID),
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("list parents: %w", err)
		}
		if len(parents.Parents) > 0 && aws.ToString(parents.Parents[0].Id) != parentID {
			if _, err := r.OrganizationsClient.MoveAccount(ctx, &awsorgs.MoveAccountInput{
				AccountId:           aws.String(acct.Status.AccountID),
				SourceParentId:      parents.Parents[0].Id,
				DestinationParentId: aws.String(parentID),
			}); err != nil {
				return ctrl.Result{}, fmt.Errorf("move account: %w", err)
			}
		}
	}

	if acct.Status.ObservedGeneration != acct.Generation && len(acct.Spec.Tags) > 0 {
		if _, err := r.OrganizationsClient.TagResource(ctx, &awsorgs.TagResourceInput{
			ResourceId: aws.String(acct.Status.AccountID),
			Tags:       orgTags(acct.Spec.Tags),
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("tag account: %w", err)
		}
	}

	acct.Status.ObservedGeneration = acct.Generation
	now := metav1.Now()
	acct.Status.LastSyncTime = &now
	if err := r.setCondition(ctx, acct, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "account reconciled"); err != nil {
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *OrganizationsAccountReconciler) setCondition(ctx context.Context, acct *awsv1alpha1.OrganizationsAccount, condType string, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&acct.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: acct.Generation,
		Reason:             reason,
		Message:            message,
	})
	if err := persistStatus(ctx, r.Client, acct); err != nil {
		log.FromContext(ctx).Error(err, "failed to update status condition")
		return err
	}
	return nil
}

func (r *OrganizationsAccountReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.OrganizationsAccount{}).
		Complete(r)
}
