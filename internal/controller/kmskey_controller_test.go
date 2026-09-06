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
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// fakeKMSKeyAPI implements KMSKeyAWSAPI via function fields.
type fakeKMSKeyAPI struct {
	DescribeKeyFn          func(ctx context.Context, params *awskms.DescribeKeyInput, optFns ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error)
	CreateKeyFn            func(ctx context.Context, params *awskms.CreateKeyInput, optFns ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error)
	UpdateKeyDescriptionFn func(ctx context.Context, params *awskms.UpdateKeyDescriptionInput, optFns ...func(*awskms.Options)) (*awskms.UpdateKeyDescriptionOutput, error)
	GetKeyPolicyFn         func(ctx context.Context, params *awskms.GetKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyPolicyOutput, error)
	PutKeyPolicyFn         func(ctx context.Context, params *awskms.PutKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.PutKeyPolicyOutput, error)
	GetKeyRotationStatusFn func(ctx context.Context, params *awskms.GetKeyRotationStatusInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyRotationStatusOutput, error)
	EnableKeyRotationFn    func(ctx context.Context, params *awskms.EnableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.EnableKeyRotationOutput, error)
	DisableKeyRotationFn   func(ctx context.Context, params *awskms.DisableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.DisableKeyRotationOutput, error)
	ScheduleKeyDeletionFn  func(ctx context.Context, params *awskms.ScheduleKeyDeletionInput, optFns ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error)
}

func (f *fakeKMSKeyAPI) DescribeKey(ctx context.Context, params *awskms.DescribeKeyInput, optFns ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error) {
	if f.DescribeKeyFn != nil {
		return f.DescribeKeyFn(ctx, params, optFns...)
	}
	return nil, &kmstypes.NotFoundException{}
}

func (f *fakeKMSKeyAPI) CreateKey(ctx context.Context, params *awskms.CreateKeyInput, optFns ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error) {
	if f.CreateKeyFn != nil {
		return f.CreateKeyFn(ctx, params, optFns...)
	}
	return nil, fmt.Errorf("unexpected CreateKey call")
}

func (f *fakeKMSKeyAPI) UpdateKeyDescription(ctx context.Context, params *awskms.UpdateKeyDescriptionInput, optFns ...func(*awskms.Options)) (*awskms.UpdateKeyDescriptionOutput, error) {
	if f.UpdateKeyDescriptionFn != nil {
		return f.UpdateKeyDescriptionFn(ctx, params, optFns...)
	}
	return &awskms.UpdateKeyDescriptionOutput{}, nil
}

func (f *fakeKMSKeyAPI) GetKeyPolicy(ctx context.Context, params *awskms.GetKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyPolicyOutput, error) {
	if f.GetKeyPolicyFn != nil {
		return f.GetKeyPolicyFn(ctx, params, optFns...)
	}
	return &awskms.GetKeyPolicyOutput{}, nil
}

func (f *fakeKMSKeyAPI) PutKeyPolicy(ctx context.Context, params *awskms.PutKeyPolicyInput, optFns ...func(*awskms.Options)) (*awskms.PutKeyPolicyOutput, error) {
	if f.PutKeyPolicyFn != nil {
		return f.PutKeyPolicyFn(ctx, params, optFns...)
	}
	return &awskms.PutKeyPolicyOutput{}, nil
}

func (f *fakeKMSKeyAPI) GetKeyRotationStatus(ctx context.Context, params *awskms.GetKeyRotationStatusInput, optFns ...func(*awskms.Options)) (*awskms.GetKeyRotationStatusOutput, error) {
	if f.GetKeyRotationStatusFn != nil {
		return f.GetKeyRotationStatusFn(ctx, params, optFns...)
	}
	return &awskms.GetKeyRotationStatusOutput{}, nil
}

func (f *fakeKMSKeyAPI) EnableKeyRotation(ctx context.Context, params *awskms.EnableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.EnableKeyRotationOutput, error) {
	if f.EnableKeyRotationFn != nil {
		return f.EnableKeyRotationFn(ctx, params, optFns...)
	}
	return &awskms.EnableKeyRotationOutput{}, nil
}

func (f *fakeKMSKeyAPI) DisableKeyRotation(ctx context.Context, params *awskms.DisableKeyRotationInput, optFns ...func(*awskms.Options)) (*awskms.DisableKeyRotationOutput, error) {
	if f.DisableKeyRotationFn != nil {
		return f.DisableKeyRotationFn(ctx, params, optFns...)
	}
	return &awskms.DisableKeyRotationOutput{}, nil
}

func (f *fakeKMSKeyAPI) ScheduleKeyDeletion(ctx context.Context, params *awskms.ScheduleKeyDeletionInput, optFns ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error) {
	if f.ScheduleKeyDeletionFn != nil {
		return f.ScheduleKeyDeletionFn(ctx, params, optFns...)
	}
	return &awskms.ScheduleKeyDeletionOutput{}, nil
}

const (
	testKeyID  = "1234abcd-12ab-34cd-56ef-1234567890ab"
	testKeyARN = "arn:aws:kms:us-east-1:123456789012:key/" + testKeyID
)

func newTestKMSKey(mutators ...func(*awsv1alpha1.KMSKey)) *awsv1alpha1.KMSKey {
	k := &awsv1alpha1.KMSKey{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-key",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.KMSKeySpec{
			Description: "test key",
		},
	}
	for _, m := range mutators {
		m(k)
	}
	return k
}

func newKMSKeyFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&awsv1alpha1.KMSKey{}).
		Build()
}

func TestKMSKeyCreateHappyPath(t *testing.T) {
	s := iamRoleTestScheme(t)
	k := newTestKMSKey()
	cl := newKMSKeyFakeClient(s, k)

	var created *awskms.CreateKeyInput
	fakeAWS := &fakeKMSKeyAPI{
		CreateKeyFn: func(_ context.Context, params *awskms.CreateKeyInput, _ ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error) {
			created = params
			return &awskms.CreateKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:    aws.String(testKeyID),
				Arn:      aws.String(testKeyARN),
				KeyState: kmstypes.KeyStateEnabled,
			}}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if created == nil {
		t.Fatal("expected CreateKey to be called")
	}
	if aws.ToString(created.Description) != "test key" {
		t.Errorf("CreateKey description = %q", aws.ToString(created.Description))
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.KeyID != testKeyID || got.Status.ARN != testKeyARN {
		t.Errorf("status KeyID/ARN = %q/%q", got.Status.KeyID, got.Status.ARN)
	}
	if got.Status.KeyState != "Enabled" {
		t.Errorf("status KeyState = %q", got.Status.KeyState)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestKMSKeyIDPersistedWhenEnableRotationFails(t *testing.T) {
	// THE orphan-mint regression test: CreateKey succeeds, EnableKeyRotation
	// fails. Creation is gated on Status.KeyID == "", so if the KeyID is not
	// persisted the next reconcile would mint a brand new key.
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Spec.EnableKeyRotation = true
	})
	cl := newKMSKeyFakeClient(s, k)

	fakeAWS := &fakeKMSKeyAPI{
		CreateKeyFn: func(_ context.Context, _ *awskms.CreateKeyInput, _ ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error) {
			return &awskms.CreateKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:    aws.String(testKeyID),
				Arn:      aws.String(testKeyARN),
				KeyState: kmstypes.KeyStateEnabled,
			}}, nil
		},
		EnableKeyRotationFn: func(_ context.Context, _ *awskms.EnableKeyRotationInput, _ ...func(*awskms.Options)) (*awskms.EnableKeyRotationOutput, error) {
			return nil, fmt.Errorf("throttled")
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected reconcile error when EnableKeyRotation fails")
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.KeyID != testKeyID {
		t.Errorf("status KeyID = %q; KeyID must be persisted before EnableKeyRotation or retries orphan the key", got.Status.KeyID)
	}
}

func TestKMSKeySteadyStateNoCreate(t *testing.T) {
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Status.KeyID = testKeyID
		k.Status.ARN = testKeyARN
		k.Status.ObservedGeneration = k.Generation
	})
	cl := newKMSKeyFakeClient(s, k)

	createCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		DescribeKeyFn: func(_ context.Context, params *awskms.DescribeKeyInput, _ ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error) {
			if aws.ToString(params.KeyId) != testKeyID {
				t.Errorf("DescribeKey id = %q", aws.ToString(params.KeyId))
			}
			return &awskms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:    aws.String(testKeyID),
				Arn:      aws.String(testKeyARN),
				KeyState: kmstypes.KeyStateEnabled,
			}}, nil
		},
		CreateKeyFn: func(_ context.Context, _ *awskms.CreateKeyInput, _ ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error) {
			createCalls++
			return nil, fmt.Errorf("should not be called")
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 0 {
		t.Errorf("CreateKey called %d times, want 0", createCalls)
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	cond := meta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("Ready condition = %+v, want True", cond)
	}
}

func TestKMSKeyUpdateGatedOnGeneration(t *testing.T) {
	// The fake client bumps Generation on spec-changing Update; here we simply
	// construct the CR with ObservedGeneration != Generation and a drifted
	// description/rotation, and expect the update calls.
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Generation = 2
		k.Spec.Description = "new description"
		k.Spec.EnableKeyRotation = true
		k.Spec.Policy = `{"Version":"2012-10-17"}`
		k.Status.KeyID = testKeyID
		k.Status.ARN = testKeyARN
		k.Status.ObservedGeneration = 1
	})
	cl := newKMSKeyFakeClient(s, k)

	var updatedDesc string
	var putPolicy string
	rotationEnabled := false
	fakeAWS := &fakeKMSKeyAPI{
		DescribeKeyFn: func(_ context.Context, _ *awskms.DescribeKeyInput, _ ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error) {
			return &awskms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:       aws.String(testKeyID),
				Arn:         aws.String(testKeyARN),
				KeyState:    kmstypes.KeyStateEnabled,
				Description: aws.String("old description"),
			}}, nil
		},
		UpdateKeyDescriptionFn: func(_ context.Context, params *awskms.UpdateKeyDescriptionInput, _ ...func(*awskms.Options)) (*awskms.UpdateKeyDescriptionOutput, error) {
			updatedDesc = aws.ToString(params.Description)
			return &awskms.UpdateKeyDescriptionOutput{}, nil
		},
		GetKeyPolicyFn: func(_ context.Context, _ *awskms.GetKeyPolicyInput, _ ...func(*awskms.Options)) (*awskms.GetKeyPolicyOutput, error) {
			return &awskms.GetKeyPolicyOutput{Policy: aws.String(`{"old":"policy"}`)}, nil
		},
		PutKeyPolicyFn: func(_ context.Context, params *awskms.PutKeyPolicyInput, _ ...func(*awskms.Options)) (*awskms.PutKeyPolicyOutput, error) {
			putPolicy = aws.ToString(params.Policy)
			return &awskms.PutKeyPolicyOutput{}, nil
		},
		GetKeyRotationStatusFn: func(_ context.Context, _ *awskms.GetKeyRotationStatusInput, _ ...func(*awskms.Options)) (*awskms.GetKeyRotationStatusOutput, error) {
			return &awskms.GetKeyRotationStatusOutput{KeyRotationEnabled: false}, nil
		},
		EnableKeyRotationFn: func(_ context.Context, _ *awskms.EnableKeyRotationInput, _ ...func(*awskms.Options)) (*awskms.EnableKeyRotationOutput, error) {
			rotationEnabled = true
			return &awskms.EnableKeyRotationOutput{}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updatedDesc != "new description" {
		t.Errorf("UpdateKeyDescription = %q, want new description", updatedDesc)
	}
	if putPolicy != `{"Version":"2012-10-17"}` {
		t.Errorf("PutKeyPolicy = %q, want spec policy", putPolicy)
	}
	if !rotationEnabled {
		t.Error("EnableKeyRotation not called, want called")
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.ObservedGeneration != 2 {
		t.Errorf("ObservedGeneration = %d, want 2", got.Status.ObservedGeneration)
	}
}

func TestKMSKeyNoUpdateWhenGenerationObserved(t *testing.T) {
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Generation = 2
		k.Spec.Description = "drifted in aws but generation already observed"
		k.Status.KeyID = testKeyID
		k.Status.ObservedGeneration = 2
	})
	cl := newKMSKeyFakeClient(s, k)

	updateCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		DescribeKeyFn: func(_ context.Context, _ *awskms.DescribeKeyInput, _ ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error) {
			return &awskms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:       aws.String(testKeyID),
				KeyState:    kmstypes.KeyStateEnabled,
				Description: aws.String("something else"),
			}}, nil
		},
		UpdateKeyDescriptionFn: func(_ context.Context, _ *awskms.UpdateKeyDescriptionInput, _ ...func(*awskms.Options)) (*awskms.UpdateKeyDescriptionOutput, error) {
			updateCalls++
			return &awskms.UpdateKeyDescriptionOutput{}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateKeyDescription called %d times, want 0 (generation already observed)", updateCalls)
	}
}

func TestKMSKeyDeleteWithFinalizer(t *testing.T) {
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Spec.PendingWindowInDays = 7
		k.Status.KeyID = testKeyID
	})
	cl := newKMSKeyFakeClient(s, k)

	scheduleCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		ScheduleKeyDeletionFn: func(_ context.Context, params *awskms.ScheduleKeyDeletionInput, _ ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error) {
			scheduleCalls++
			if aws.ToString(params.KeyId) != testKeyID {
				t.Errorf("ScheduleKeyDeletion id = %q", aws.ToString(params.KeyId))
			}
			if aws.ToInt32(params.PendingWindowInDays) != 7 {
				t.Errorf("PendingWindowInDays = %d, want 7", aws.ToInt32(params.PendingWindowInDays))
			}
			return &awskms.ScheduleKeyDeletionOutput{}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, k); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if scheduleCalls != 1 {
		t.Errorf("ScheduleKeyDeletion called %d times, want 1", scheduleCalls)
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestKMSKeyDeleteWithoutKeyIDSkipsAWS(t *testing.T) {
	// No fallback lookup by design: without a KeyID nothing can be deleted.
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
	})
	cl := newKMSKeyFakeClient(s, k)

	scheduleCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		ScheduleKeyDeletionFn: func(_ context.Context, _ *awskms.ScheduleKeyDeletionInput, _ ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error) {
			scheduleCalls++
			return &awskms.ScheduleKeyDeletionOutput{}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, k); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if scheduleCalls != 0 {
		t.Errorf("ScheduleKeyDeletion called %d times, want 0", scheduleCalls)
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestKMSKeyAbandonAnnotation(t *testing.T) {
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Status.KeyID = testKeyID
		k.Annotations = map[string]string{
			awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon,
		}
	})
	cl := newKMSKeyFakeClient(s, k)

	scheduleCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		ScheduleKeyDeletionFn: func(_ context.Context, _ *awskms.ScheduleKeyDeletionInput, _ ...func(*awskms.Options)) (*awskms.ScheduleKeyDeletionOutput, error) {
			scheduleCalls++
			return &awskms.ScheduleKeyDeletionOutput{}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	ctx := context.Background()
	if err := cl.Delete(ctx, k); err != nil {
		t.Fatalf("delete: %v", err)
	}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if scheduleCalls != 0 {
		t.Errorf("ScheduleKeyDeletion called %d times, want 0 (abandon)", scheduleCalls)
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(ctx, key, got); !errors.IsNotFound(err) {
		t.Errorf("expected object gone, got err=%v", err)
	}
}

func TestKMSKeyRecreateWhenDeletedExternally(t *testing.T) {
	// Status has a KeyID but DescribeKey reports NotFound: the controller
	// clears the stale ID and mints a new key.
	s := iamRoleTestScheme(t)
	k := newTestKMSKey(func(k *awsv1alpha1.KMSKey) {
		k.Finalizers = []string{awsv1alpha1.FinalizerName}
		k.Status.KeyID = "stale-key-id"
	})
	cl := newKMSKeyFakeClient(s, k)

	createCalls := 0
	fakeAWS := &fakeKMSKeyAPI{
		DescribeKeyFn: func(_ context.Context, _ *awskms.DescribeKeyInput, _ ...func(*awskms.Options)) (*awskms.DescribeKeyOutput, error) {
			return nil, &kmstypes.NotFoundException{}
		},
		CreateKeyFn: func(_ context.Context, _ *awskms.CreateKeyInput, _ ...func(*awskms.Options)) (*awskms.CreateKeyOutput, error) {
			createCalls++
			return &awskms.CreateKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{
				KeyId:    aws.String(testKeyID),
				Arn:      aws.String(testKeyARN),
				KeyState: kmstypes.KeyStateEnabled,
			}}, nil
		},
	}
	r := &KMSKeyReconciler{Client: cl, Scheme: s, KMSClient: fakeAWS}

	key := k8stypes.NamespacedName{Name: "test-key", Namespace: "default"}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if createCalls != 1 {
		t.Errorf("CreateKey called %d times, want 1", createCalls)
	}

	got := &awsv1alpha1.KMSKey{}
	if err := cl.Get(context.Background(), key, got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.KeyID != testKeyID {
		t.Errorf("status KeyID = %q, want new key ID", got.Status.KeyID)
	}
}
