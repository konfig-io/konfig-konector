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
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsct "github.com/aws/aws-sdk-go-v2/service/controltower"
	cttypes "github.com/aws/aws-sdk-go-v2/service/controltower/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const (
	testControlID  = "arn:aws:controltower:us-east-1::control/AWS-GR_ENCRYPTED_VOLUMES"
	testTargetOUID = "arn:aws:organizations::123456789012:ou/o-example/ou-abcd-11111111"
	testControlARN = "arn:aws:controltower:us-east-1:123456789012:enabledcontrol/abcdef123456"
)

type fakeControlTower struct {
	enableCalled  bool
	disableCalled bool
	enabled       []cttypes.EnabledControlSummary
	opStatus      cttypes.ControlOperationStatus
}

func (f *fakeControlTower) EnableControl(_ context.Context, _ *awsct.EnableControlInput, _ ...func(*awsct.Options)) (*awsct.EnableControlOutput, error) {
	f.enableCalled = true
	return &awsct.EnableControlOutput{
		OperationIdentifier: aws.String("op-123"),
		Arn:                 aws.String(testControlARN),
	}, nil
}

func (f *fakeControlTower) DisableControl(_ context.Context, _ *awsct.DisableControlInput, _ ...func(*awsct.Options)) (*awsct.DisableControlOutput, error) {
	f.disableCalled = true
	return &awsct.DisableControlOutput{OperationIdentifier: aws.String("op-456")}, nil
}

func (f *fakeControlTower) GetControlOperation(_ context.Context, _ *awsct.GetControlOperationInput, _ ...func(*awsct.Options)) (*awsct.GetControlOperationOutput, error) {
	status := f.opStatus
	if status == "" {
		status = cttypes.ControlOperationStatusInProgress
	}
	return &awsct.GetControlOperationOutput{
		ControlOperation: &cttypes.ControlOperation{
			OperationIdentifier: aws.String("op-123"),
			Status:              status,
		},
	}, nil
}

func (f *fakeControlTower) ListEnabledControls(_ context.Context, _ *awsct.ListEnabledControlsInput, _ ...func(*awsct.Options)) (*awsct.ListEnabledControlsOutput, error) {
	return &awsct.ListEnabledControlsOutput{EnabledControls: f.enabled}, nil
}

func ctControlCR(mutate ...func(*awsv1alpha1.CTEnabledControl)) *awsv1alpha1.CTEnabledControl {
	ec := &awsv1alpha1.CTEnabledControl{
		ObjectMeta: metav1.ObjectMeta{Name: "encrypted-volumes", Namespace: "default"},
		Spec: awsv1alpha1.CTEnabledControlSpec{
			ControlIdentifier: testControlID,
			TargetIdentifier:  testTargetOUID,
		},
	}
	for _, m := range mutate {
		m(ec)
	}
	return ec
}

func TestCTEnabledControlReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "encrypted-volumes", Namespace: "default"}}

	t.Run("create kicks off async enable and persists operation identifier", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CTEnabledControl{}).
			WithObjects(ctControlCR()).Build()
		f := &fakeControlTower{}
		r := &CTEnabledControlReconciler{Client: c, Scheme: scheme, ControlTowerClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueOrgPolling {
			t.Errorf("result = %+v, want requeueOrgPolling", res)
		}
		got := &awsv1alpha1.CTEnabledControl{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if !f.enableCalled {
			t.Error("expected EnableControl to be called")
		}
		if got.Status.OperationIdentifier != "op-123" {
			t.Errorf("status.operationIdentifier = %q, want op-123", got.Status.OperationIdentifier)
		}
		if got.Status.ARN != testControlARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testControlARN)
		}
	})

	t.Run("operation success plus listed control reaches Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CTEnabledControl{}).
			WithObjects(ctControlCR(func(ec *awsv1alpha1.CTEnabledControl) {
				ec.Finalizers = []string{awsv1alpha1.FinalizerName}
				ec.Status.OperationIdentifier = "op-123"
			})).Build()
		f := &fakeControlTower{
			opStatus: cttypes.ControlOperationStatusSucceeded,
			enabled: []cttypes.EnabledControlSummary{{
				Arn:               aws.String(testControlARN),
				ControlIdentifier: aws.String(testControlID),
				TargetIdentifier:  aws.String(testTargetOUID),
				StatusSummary:     &cttypes.EnablementStatusSummary{Status: cttypes.EnablementStatusSucceeded},
			}},
		}
		r := &CTEnabledControlReconciler{Client: c, Scheme: scheme, ControlTowerClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.CTEnabledControl{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if f.enableCalled {
			t.Error("EnableControl must not be called again when the control is already enabled")
		}
		if got.Status.OperationIdentifier != "" {
			t.Errorf("status.operationIdentifier = %q, want cleared", got.Status.OperationIdentifier)
		}
		if got.Status.ARN != testControlARN {
			t.Errorf("status.arn = %q, want %q", got.Status.ARN, testControlARN)
		}
	})

	t.Run("delete disables the control", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CTEnabledControl{}).
			WithObjects(ctControlCR(func(ec *awsv1alpha1.CTEnabledControl) {
				ec.Finalizers = []string{awsv1alpha1.FinalizerName}
				ec.Status.ARN = testControlARN
			})).Build()
		f := &fakeControlTower{}
		r := &CTEnabledControlReconciler{Client: c, Scheme: scheme, ControlTowerClient: f}

		if err := c.Delete(ctx, ctControlCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.disableCalled {
			t.Error("expected DisableControl to be called")
		}
		got := &awsv1alpha1.CTEnabledControl{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips disable", func(t *testing.T) {
		ctx := context.Background()
		scheme := newOrgScheme(t)
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.CTEnabledControl{}).
			WithObjects(ctControlCR(func(ec *awsv1alpha1.CTEnabledControl) {
				ec.Finalizers = []string{awsv1alpha1.FinalizerName}
				ec.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				ec.Status.ARN = testControlARN
			})).Build()
		f := &fakeControlTower{}
		r := &CTEnabledControlReconciler{Client: c, Scheme: scheme, ControlTowerClient: f}

		if err := c.Delete(ctx, ctControlCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.disableCalled {
			t.Error("DisableControl must not be called when abandoning")
		}
	})
}
