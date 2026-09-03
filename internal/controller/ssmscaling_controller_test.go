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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsssm "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeSSMScaling struct {
	createMW func(ctx context.Context, params *awsssm.CreateMaintenanceWindowInput) (*awsssm.CreateMaintenanceWindowOutput, error)
	updateMW func(ctx context.Context, params *awsssm.UpdateMaintenanceWindowInput) (*awsssm.UpdateMaintenanceWindowOutput, error)
	deleteMW func(ctx context.Context, params *awsssm.DeleteMaintenanceWindowInput) (*awsssm.DeleteMaintenanceWindowOutput, error)

	createPB func(ctx context.Context, params *awsssm.CreatePatchBaselineInput) (*awsssm.CreatePatchBaselineOutput, error)
	updatePB func(ctx context.Context, params *awsssm.UpdatePatchBaselineInput) (*awsssm.UpdatePatchBaselineOutput, error)
	deletePB func(ctx context.Context, params *awsssm.DeletePatchBaselineInput) (*awsssm.DeletePatchBaselineOutput, error)

	createAssoc func(ctx context.Context, params *awsssm.CreateAssociationInput) (*awsssm.CreateAssociationOutput, error)
	updateAssoc func(ctx context.Context, params *awsssm.UpdateAssociationInput) (*awsssm.UpdateAssociationOutput, error)
	deleteAssoc func(ctx context.Context, params *awsssm.DeleteAssociationInput) (*awsssm.DeleteAssociationOutput, error)

	createMWCalled    bool
	deleteMWCalled    bool
	createPBCalled    bool
	deletePBCalled    bool
	createAssocCalled bool
	deleteAssocCalled bool
}

func (f *fakeSSMScaling) CreateMaintenanceWindow(ctx context.Context, params *awsssm.CreateMaintenanceWindowInput, _ ...func(*awsssm.Options)) (*awsssm.CreateMaintenanceWindowOutput, error) {
	f.createMWCalled = true
	if f.createMW == nil {
		return nil, fmt.Errorf("unexpected call to CreateMaintenanceWindow")
	}
	return f.createMW(ctx, params)
}

func (f *fakeSSMScaling) UpdateMaintenanceWindow(ctx context.Context, params *awsssm.UpdateMaintenanceWindowInput, _ ...func(*awsssm.Options)) (*awsssm.UpdateMaintenanceWindowOutput, error) {
	if f.updateMW == nil {
		return nil, fmt.Errorf("unexpected call to UpdateMaintenanceWindow")
	}
	return f.updateMW(ctx, params)
}

func (f *fakeSSMScaling) DeleteMaintenanceWindow(ctx context.Context, params *awsssm.DeleteMaintenanceWindowInput, _ ...func(*awsssm.Options)) (*awsssm.DeleteMaintenanceWindowOutput, error) {
	f.deleteMWCalled = true
	if f.deleteMW == nil {
		return nil, fmt.Errorf("unexpected call to DeleteMaintenanceWindow")
	}
	return f.deleteMW(ctx, params)
}

func (f *fakeSSMScaling) CreatePatchBaseline(ctx context.Context, params *awsssm.CreatePatchBaselineInput, _ ...func(*awsssm.Options)) (*awsssm.CreatePatchBaselineOutput, error) {
	f.createPBCalled = true
	if f.createPB == nil {
		return nil, fmt.Errorf("unexpected call to CreatePatchBaseline")
	}
	return f.createPB(ctx, params)
}

func (f *fakeSSMScaling) UpdatePatchBaseline(ctx context.Context, params *awsssm.UpdatePatchBaselineInput, _ ...func(*awsssm.Options)) (*awsssm.UpdatePatchBaselineOutput, error) {
	if f.updatePB == nil {
		return nil, fmt.Errorf("unexpected call to UpdatePatchBaseline")
	}
	return f.updatePB(ctx, params)
}

func (f *fakeSSMScaling) DeletePatchBaseline(ctx context.Context, params *awsssm.DeletePatchBaselineInput, _ ...func(*awsssm.Options)) (*awsssm.DeletePatchBaselineOutput, error) {
	f.deletePBCalled = true
	if f.deletePB == nil {
		return nil, fmt.Errorf("unexpected call to DeletePatchBaseline")
	}
	return f.deletePB(ctx, params)
}

func (f *fakeSSMScaling) CreateAssociation(ctx context.Context, params *awsssm.CreateAssociationInput, _ ...func(*awsssm.Options)) (*awsssm.CreateAssociationOutput, error) {
	f.createAssocCalled = true
	if f.createAssoc == nil {
		return nil, fmt.Errorf("unexpected call to CreateAssociation")
	}
	return f.createAssoc(ctx, params)
}

func (f *fakeSSMScaling) UpdateAssociation(ctx context.Context, params *awsssm.UpdateAssociationInput, _ ...func(*awsssm.Options)) (*awsssm.UpdateAssociationOutput, error) {
	if f.updateAssoc == nil {
		return nil, fmt.Errorf("unexpected call to UpdateAssociation")
	}
	return f.updateAssoc(ctx, params)
}

func (f *fakeSSMScaling) DeleteAssociation(ctx context.Context, params *awsssm.DeleteAssociationInput, _ ...func(*awsssm.Options)) (*awsssm.DeleteAssociationOutput, error) {
	f.deleteAssocCalled = true
	if f.deleteAssoc == nil {
		return nil, fmt.Errorf("unexpected call to DeleteAssociation")
	}
	return f.deleteAssoc(ctx, params)
}

func TestSSMMaintenanceWindowReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-window", Namespace: "default"}}
	mwCR := func(mutate ...func(*awsv1alpha1.SSMMaintenanceWindow)) *awsv1alpha1.SSMMaintenanceWindow {
		mw := &awsv1alpha1.SSMMaintenanceWindow{
			ObjectMeta: metav1.ObjectMeta{Name: "my-window", Namespace: "default"},
			Spec: awsv1alpha1.SSMMaintenanceWindowSpec{
				Name:     "my-window",
				Schedule: "cron(0 4 ? * SUN *)",
				Duration: 4,
				Cutoff:   1,
			},
		}
		for _, m := range mutate {
			m(mw)
		}
		return mw
	}

	t.Run("create persists window ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, mwCR())
		f := &fakeSSMScaling{
			createMW: func(_ context.Context, params *awsssm.CreateMaintenanceWindowInput) (*awsssm.CreateMaintenanceWindowOutput, error) {
				if params.Duration != 4 || params.Cutoff != 1 {
					t.Errorf("duration/cutoff = %d/%d", params.Duration, params.Cutoff)
				}
				return &awsssm.CreateMaintenanceWindowOutput{WindowId: aws.String("mw-abc123")}, nil
			},
		}
		r := &SSMMaintenanceWindowReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SSMMaintenanceWindow{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.WindowID != "mw-abc123" {
			t.Errorf("status.windowId = %q", got.Status.WindowID)
		}
	})

	t.Run("delete uses status window ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, mwCR(func(mw *awsv1alpha1.SSMMaintenanceWindow) {
			mw.Finalizers = []string{awsv1alpha1.FinalizerName}
			mw.Status.WindowID = "mw-abc123"
		}))
		f := &fakeSSMScaling{
			deleteMW: func(_ context.Context, params *awsssm.DeleteMaintenanceWindowInput) (*awsssm.DeleteMaintenanceWindowOutput, error) {
				if aws.ToString(params.WindowId) != "mw-abc123" {
					t.Errorf("delete window ID = %q", aws.ToString(params.WindowId))
				}
				return &awsssm.DeleteMaintenanceWindowOutput{}, nil
			},
		}
		r := &SSMMaintenanceWindowReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, mwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteMWCalled {
			t.Error("expected DeleteMaintenanceWindow to be called")
		}
		got := &awsv1alpha1.SSMMaintenanceWindow{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, mwCR(func(mw *awsv1alpha1.SSMMaintenanceWindow) {
			mw.Finalizers = []string{awsv1alpha1.FinalizerName}
			mw.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			mw.Status.WindowID = "mw-abc123"
		}))
		f := &fakeSSMScaling{}
		r := &SSMMaintenanceWindowReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, mwCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteMWCalled {
			t.Error("DeleteMaintenanceWindow must not be called when abandoning")
		}
	})
}

func TestSSMPatchBaselineReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-baseline", Namespace: "default"}}
	pbCR := func(mutate ...func(*awsv1alpha1.SSMPatchBaseline)) *awsv1alpha1.SSMPatchBaseline {
		pb := &awsv1alpha1.SSMPatchBaseline{
			ObjectMeta: metav1.ObjectMeta{Name: "my-baseline", Namespace: "default"},
			Spec: awsv1alpha1.SSMPatchBaselineSpec{
				Name:            "my-baseline",
				OperatingSystem: "AMAZON_LINUX_2",
				ApprovalRules: []awsv1alpha1.SSMPatchRule{{
					ApproveAfterDays: aws.Int32(7),
					ComplianceLevel:  "CRITICAL",
					PatchFilters: []awsv1alpha1.SSMPatchFilter{{
						Key: "CLASSIFICATION", Values: []string{"Security"},
					}},
				}},
			},
		}
		for _, m := range mutate {
			m(pb)
		}
		return pb
	}

	t.Run("create persists baseline ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pbCR())
		f := &fakeSSMScaling{
			createPB: func(_ context.Context, params *awsssm.CreatePatchBaselineInput) (*awsssm.CreatePatchBaselineOutput, error) {
				if params.OperatingSystem != ssmtypes.OperatingSystemAmazonLinux2 {
					t.Errorf("os = %q", params.OperatingSystem)
				}
				if params.ApprovalRules == nil || len(params.ApprovalRules.PatchRules) != 1 {
					t.Error("approval rules not passed")
				} else if aws.ToInt32(params.ApprovalRules.PatchRules[0].ApproveAfterDays) != 7 {
					t.Error("approveAfterDays not passed")
				}
				return &awsssm.CreatePatchBaselineOutput{BaselineId: aws.String("pb-abc123")}, nil
			},
		}
		r := &SSMPatchBaselineReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SSMPatchBaseline{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.BaselineID != "pb-abc123" {
			t.Errorf("status.baselineId = %q", got.Status.BaselineID)
		}
	})

	t.Run("delete uses status baseline ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pbCR(func(pb *awsv1alpha1.SSMPatchBaseline) {
			pb.Finalizers = []string{awsv1alpha1.FinalizerName}
			pb.Status.BaselineID = "pb-abc123"
		}))
		f := &fakeSSMScaling{
			deletePB: func(_ context.Context, params *awsssm.DeletePatchBaselineInput) (*awsssm.DeletePatchBaselineOutput, error) {
				if aws.ToString(params.BaselineId) != "pb-abc123" {
					t.Errorf("delete baseline ID = %q", aws.ToString(params.BaselineId))
				}
				return &awsssm.DeletePatchBaselineOutput{}, nil
			},
		}
		r := &SSMPatchBaselineReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, pbCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deletePBCalled {
			t.Error("expected DeletePatchBaseline to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, pbCR(func(pb *awsv1alpha1.SSMPatchBaseline) {
			pb.Finalizers = []string{awsv1alpha1.FinalizerName}
			pb.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			pb.Status.BaselineID = "pb-abc123"
		}))
		f := &fakeSSMScaling{}
		r := &SSMPatchBaselineReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, pbCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deletePBCalled {
			t.Error("DeletePatchBaseline must not be called when abandoning")
		}
	})
}

func TestSSMAssociationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-assoc", Namespace: "default"}}
	assocCR := func(mutate ...func(*awsv1alpha1.SSMAssociation)) *awsv1alpha1.SSMAssociation {
		a := &awsv1alpha1.SSMAssociation{
			ObjectMeta: metav1.ObjectMeta{Name: "my-assoc", Namespace: "default"},
			Spec: awsv1alpha1.SSMAssociationSpec{
				Name:            "AWS-RunPatchBaseline",
				AssociationName: "patch-all",
				Targets: []awsv1alpha1.SSMAssociationTarget{{
					Key: "tag:Environment", Values: []string{"prod"},
				}},
				ScheduleExpression: "rate(1 day)",
				Parameters:         map[string][]string{"Operation": {"Install"}},
			},
		}
		for _, m := range mutate {
			m(a)
		}
		return a
	}

	t.Run("create persists association ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, assocCR())
		f := &fakeSSMScaling{
			createAssoc: func(_ context.Context, params *awsssm.CreateAssociationInput) (*awsssm.CreateAssociationOutput, error) {
				if aws.ToString(params.Name) != "AWS-RunPatchBaseline" {
					t.Errorf("doc name = %q", aws.ToString(params.Name))
				}
				if len(params.Targets) != 1 || aws.ToString(params.Targets[0].Key) != "tag:Environment" {
					t.Error("targets not passed")
				}
				if params.Parameters["Operation"][0] != "Install" {
					t.Error("parameters not passed")
				}
				return &awsssm.CreateAssociationOutput{
					AssociationDescription: &ssmtypes.AssociationDescription{
						AssociationId: aws.String("assoc-abc123"),
					},
				}, nil
			},
		}
		r := &SSMAssociationReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.SSMAssociation{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.AssociationID != "assoc-abc123" {
			t.Errorf("status.associationId = %q", got.Status.AssociationID)
		}
	})

	t.Run("delete uses status association ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, assocCR(func(a *awsv1alpha1.SSMAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Status.AssociationID = "assoc-abc123"
		}))
		f := &fakeSSMScaling{
			deleteAssoc: func(_ context.Context, params *awsssm.DeleteAssociationInput) (*awsssm.DeleteAssociationOutput, error) {
				if aws.ToString(params.AssociationId) != "assoc-abc123" {
					t.Errorf("delete assoc ID = %q", aws.ToString(params.AssociationId))
				}
				return &awsssm.DeleteAssociationOutput{}, nil
			},
		}
		r := &SSMAssociationReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteAssocCalled {
			t.Error("expected DeleteAssociation to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, assocCR(func(a *awsv1alpha1.SSMAssociation) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.Status.AssociationID = "assoc-abc123"
		}))
		f := &fakeSSMScaling{}
		r := &SSMAssociationReconciler{Client: c, Scheme: scheme, SSMClient: f}
		if err := c.Delete(ctx, assocCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteAssocCalled {
			t.Error("DeleteAssociation must not be called when abandoning")
		}
	})
}
