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
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	ec2helper "github.com/konfig-io/konfig-konector/internal/aws/ec2"
)

// ManagedPrefixListAWSAPI is the subset of the EC2 API used by this controller.
type ManagedPrefixListAWSAPI interface {
	DescribeManagedPrefixLists(ctx context.Context, params *awsec2.DescribeManagedPrefixListsInput, optFns ...func(*awsec2.Options)) (*awsec2.DescribeManagedPrefixListsOutput, error)
	GetManagedPrefixListEntries(ctx context.Context, params *awsec2.GetManagedPrefixListEntriesInput, optFns ...func(*awsec2.Options)) (*awsec2.GetManagedPrefixListEntriesOutput, error)
	CreateManagedPrefixList(ctx context.Context, params *awsec2.CreateManagedPrefixListInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateManagedPrefixListOutput, error)
	ModifyManagedPrefixList(ctx context.Context, params *awsec2.ModifyManagedPrefixListInput, optFns ...func(*awsec2.Options)) (*awsec2.ModifyManagedPrefixListOutput, error)
	DeleteManagedPrefixList(ctx context.Context, params *awsec2.DeleteManagedPrefixListInput, optFns ...func(*awsec2.Options)) (*awsec2.DeleteManagedPrefixListOutput, error)
	CreateTags(ctx context.Context, params *awsec2.CreateTagsInput, optFns ...func(*awsec2.Options)) (*awsec2.CreateTagsOutput, error)
}

// ManagedPrefixListReconciler reconciles ManagedPrefixList objects.
type ManagedPrefixListReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	EC2Client ManagedPrefixListAWSAPI
}

// +kubebuilder:rbac:groups=aws.konfig.io,resources=managedprefixlists,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=aws.konfig.io,resources=managedprefixlists/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=aws.konfig.io,resources=managedprefixlists/finalizers,verbs=update

func (r *ManagedPrefixListReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	obj := &awsv1alpha1.ManagedPrefixList{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	var scopeErr error
	if ctx, scopeErr = withProviderScope(ctx, obj); scopeErr != nil {
		return ctrl.Result{}, scopeErr
	}

	if !obj.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(obj, awsv1alpha1.FinalizerName) {
			if shouldAbandon(obj) {
				logger.Info("abandoning AWS resource per deletion policy annotation")
				controllerutil.RemoveFinalizer(obj, awsv1alpha1.FinalizerName)
				return ctrl.Result{}, r.Update(ctx, obj)
			}
			if err := r.deletePrefixList(ctx, obj); err != nil {
				logger.Error(err, "failed to delete ManagedPrefixList")
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

	if err := r.reconcilePrefixList(ctx, obj); err != nil {
		logger.Error(err, "reconcile error")
		_ = r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionFalse, awsv1alpha1.ReasonError, err.Error())
		return ctrl.Result{}, err
	}
	return requeueResult(), nil
}

func (r *ManagedPrefixListReconciler) reconcilePrefixList(ctx context.Context, obj *awsv1alpha1.ManagedPrefixList) error {
	if obj.Status.PrefixListID == "" {
		entries := make([]ec2types.AddPrefixListEntry, 0, len(obj.Spec.Entries))
		for _, e := range obj.Spec.Entries {
			e := e
			entry := ec2types.AddPrefixListEntry{Cidr: aws.String(e.CIDR)}
			if e.Description != "" {
				entry.Description = aws.String(e.Description)
			}
			entries = append(entries, entry)
		}
		out, err := r.EC2Client.CreateManagedPrefixList(ctx, &awsec2.CreateManagedPrefixListInput{
			PrefixListName: aws.String(obj.Spec.Name),
			AddressFamily:  aws.String(obj.Spec.AddressFamily),
			MaxEntries:     aws.Int32(obj.Spec.MaxEntries),
			Entries:        entries,
			TagSpecifications: []ec2types.TagSpecification{
				{ResourceType: ec2types.ResourceTypePrefixList, Tags: ec2helper.TagsFromMap(obj.Spec.Tags)},
			},
		})
		if err != nil {
			return fmt.Errorf("create managed prefix list: %w", err)
		}
		obj.Status.PrefixListID = aws.ToString(out.PrefixList.PrefixListId)
		obj.Status.ARN = aws.ToString(out.PrefixList.PrefixListArn)
		obj.Status.Version = aws.ToInt64(out.PrefixList.Version)
		obj.Status.State = string(out.PrefixList.State)
		if err := persistStatus(ctx, r.Client, obj); err != nil {
			return fmt.Errorf("persist prefix list ID after create: %w", err)
		}
		obj.Status.ObservedGeneration = obj.Generation
		now := metav1.Now()
		obj.Status.LastSyncTime = &now
		return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonCreated, "ManagedPrefixList created")
	}

	out, err := r.EC2Client.DescribeManagedPrefixLists(ctx, &awsec2.DescribeManagedPrefixListsInput{
		PrefixListIds: []string{obj.Status.PrefixListID},
	})
	if err != nil && !ec2helper.IsNotFound(err) {
		return fmt.Errorf("describe managed prefix list: %w", err)
	}
	if ec2helper.IsNotFound(err) || len(out.PrefixLists) == 0 {
		// Deleted out of band — recreate on the next pass.
		obj.Status.PrefixListID = ""
		obj.Status.Version = 0
		return r.reconcilePrefixList(ctx, obj)
	}
	pl := out.PrefixLists[0]
	obj.Status.ARN = aws.ToString(pl.PrefixListArn)
	obj.Status.Version = aws.ToInt64(pl.Version)
	obj.Status.State = string(pl.State)

	if obj.Status.ObservedGeneration != obj.Generation {
		if err := r.syncEntries(ctx, obj, pl); err != nil {
			return err
		}
		if len(obj.Spec.Tags) > 0 {
			if _, err := r.EC2Client.CreateTags(ctx, &awsec2.CreateTagsInput{
				Resources: []string{obj.Status.PrefixListID},
				Tags:      ec2helper.TagsFromMap(obj.Spec.Tags),
			}); err != nil {
				return fmt.Errorf("tag managed prefix list: %w", err)
			}
		}
	}

	obj.Status.ObservedGeneration = obj.Generation
	now := metav1.Now()
	obj.Status.LastSyncTime = &now
	return r.setCondition(ctx, obj, awsv1alpha1.ConditionReady, metav1.ConditionTrue, awsv1alpha1.ReasonSynced, "ManagedPrefixList reconciled")
}

// syncEntries diffs desired entries against live entries and issues one
// ModifyManagedPrefixList using the current version token.
func (r *ManagedPrefixListReconciler) syncEntries(ctx context.Context, obj *awsv1alpha1.ManagedPrefixList, pl ec2types.ManagedPrefixList) error {
	current := map[string]string{}
	var nextToken *string
	for {
		entOut, err := r.EC2Client.GetManagedPrefixListEntries(ctx, &awsec2.GetManagedPrefixListEntriesInput{
			PrefixListId: pl.PrefixListId,
			NextToken:    nextToken,
		})
		if err != nil {
			return fmt.Errorf("get prefix list entries: %w", err)
		}
		for _, e := range entOut.Entries {
			current[aws.ToString(e.Cidr)] = aws.ToString(e.Description)
		}
		if entOut.NextToken == nil {
			break
		}
		nextToken = entOut.NextToken
	}

	desired := map[string]string{}
	for _, e := range obj.Spec.Entries {
		desired[e.CIDR] = e.Description
	}

	var toAdd []ec2types.AddPrefixListEntry
	for cidr, desc := range desired {
		if cur, ok := current[cidr]; !ok || cur != desc {
			cidr, desc := cidr, desc
			entry := ec2types.AddPrefixListEntry{Cidr: &cidr}
			if desc != "" {
				entry.Description = &desc
			}
			toAdd = append(toAdd, entry)
		}
	}
	var toRemove []ec2types.RemovePrefixListEntry
	for cidr := range current {
		if _, ok := desired[cidr]; !ok {
			cidr := cidr
			toRemove = append(toRemove, ec2types.RemovePrefixListEntry{Cidr: &cidr})
		}
	}

	needsModify := len(toAdd) > 0 || len(toRemove) > 0 ||
		aws.ToInt32(pl.MaxEntries) != obj.Spec.MaxEntries ||
		aws.ToString(pl.PrefixListName) != obj.Spec.Name
	if !needsModify {
		return nil
	}

	input := &awsec2.ModifyManagedPrefixListInput{
		PrefixListId:   pl.PrefixListId,
		CurrentVersion: pl.Version,
		AddEntries:     toAdd,
		RemoveEntries:  toRemove,
	}
	if aws.ToString(pl.PrefixListName) != obj.Spec.Name {
		input.PrefixListName = aws.String(obj.Spec.Name)
	}
	if aws.ToInt32(pl.MaxEntries) != obj.Spec.MaxEntries {
		// AWS rejects requests that modify MaxEntries and entries together.
		if len(toAdd) == 0 && len(toRemove) == 0 {
			input.MaxEntries = aws.Int32(obj.Spec.MaxEntries)
		}
	}
	out, err := r.EC2Client.ModifyManagedPrefixList(ctx, input)
	if err != nil {
		return fmt.Errorf("modify managed prefix list: %w", err)
	}
	obj.Status.Version = aws.ToInt64(out.PrefixList.Version)
	obj.Status.State = string(out.PrefixList.State)
	return nil
}

func (r *ManagedPrefixListReconciler) deletePrefixList(ctx context.Context, obj *awsv1alpha1.ManagedPrefixList) error {
	id := obj.Status.PrefixListID
	if id == "" {
		// Fallback: look up by name; only delete on an unambiguous match.
		out, err := r.EC2Client.DescribeManagedPrefixLists(ctx, &awsec2.DescribeManagedPrefixListsInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("prefix-list-name"), Values: []string{obj.Spec.Name}},
			},
		})
		if err != nil || len(out.PrefixLists) != 1 {
			return nil
		}
		id = aws.ToString(out.PrefixLists[0].PrefixListId)
	}
	_, err := r.EC2Client.DeleteManagedPrefixList(ctx, &awsec2.DeleteManagedPrefixListInput{
		PrefixListId: aws.String(id),
	})
	if ec2helper.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *ManagedPrefixListReconciler) setCondition(ctx context.Context, obj *awsv1alpha1.ManagedPrefixList, condType string, status metav1.ConditionStatus, reason, message string) error {
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

func (r *ManagedPrefixListReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.ManagedPrefixList{}).
		Complete(r)
}
