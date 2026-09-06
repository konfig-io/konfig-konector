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
	awssm "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/smithy-go"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeSecretsManager struct {
	describeSecret func(ctx context.Context, params *awssm.DescribeSecretInput) (*awssm.DescribeSecretOutput, error)
	createSecret   func(ctx context.Context, params *awssm.CreateSecretInput) (*awssm.CreateSecretOutput, error)
	getSecretValue func(ctx context.Context, params *awssm.GetSecretValueInput) (*awssm.GetSecretValueOutput, error)
	putSecretValue func(ctx context.Context, params *awssm.PutSecretValueInput) (*awssm.PutSecretValueOutput, error)
	deleteSecret   func(ctx context.Context, params *awssm.DeleteSecretInput) (*awssm.DeleteSecretOutput, error)

	createCalled bool
	putCalled    bool
	putValue     string
	deleteCalled bool
	deletedID    string
	deleteInput  *awssm.DeleteSecretInput
}

func (f *fakeSecretsManager) DescribeSecret(ctx context.Context, params *awssm.DescribeSecretInput, _ ...func(*awssm.Options)) (*awssm.DescribeSecretOutput, error) {
	if f.describeSecret == nil {
		return nil, fmt.Errorf("unexpected call to DescribeSecret")
	}
	return f.describeSecret(ctx, params)
}

func (f *fakeSecretsManager) CreateSecret(ctx context.Context, params *awssm.CreateSecretInput, _ ...func(*awssm.Options)) (*awssm.CreateSecretOutput, error) {
	f.createCalled = true
	if f.createSecret == nil {
		return nil, fmt.Errorf("unexpected call to CreateSecret")
	}
	return f.createSecret(ctx, params)
}

func (f *fakeSecretsManager) GetSecretValue(ctx context.Context, params *awssm.GetSecretValueInput, _ ...func(*awssm.Options)) (*awssm.GetSecretValueOutput, error) {
	if f.getSecretValue == nil {
		return nil, fmt.Errorf("unexpected call to GetSecretValue")
	}
	return f.getSecretValue(ctx, params)
}

func (f *fakeSecretsManager) PutSecretValue(ctx context.Context, params *awssm.PutSecretValueInput, _ ...func(*awssm.Options)) (*awssm.PutSecretValueOutput, error) {
	f.putCalled = true
	f.putValue = aws.ToString(params.SecretString)
	if f.putSecretValue == nil {
		return nil, fmt.Errorf("unexpected call to PutSecretValue")
	}
	return f.putSecretValue(ctx, params)
}

func (f *fakeSecretsManager) DeleteSecret(ctx context.Context, params *awssm.DeleteSecretInput, _ ...func(*awssm.Options)) (*awssm.DeleteSecretOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.SecretId)
	f.deleteInput = params
	if f.deleteSecret == nil {
		return nil, fmt.Errorf("unexpected call to DeleteSecret")
	}
	return f.deleteSecret(ctx, params)
}

func smNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "not found"}
}

const testSecretARN = "arn:aws:secretsmanager:us-east-1:123456789012:secret:my-secret-AbCdEf"

func secretCR(mutate ...func(*awsv1alpha1.Secret)) *awsv1alpha1.Secret {
	s := &awsv1alpha1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-secret",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.SecretSpec{
			SecretName: "my-secret",
			SecretStringRef: &awsv1alpha1.SecretRef{
				Name: "source-secret",
				Key:  "password",
			},
		},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

func sourceK8sSecret(value string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "source-secret",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Data: map[string][]byte{"password": []byte(value)},
	}
}

func newSecretScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add aws scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return scheme
}

func TestSecretReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-secret", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeSecretsManager
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, res ctrl.Result)
	}{
		{
			name: "create happy path persists ARN and Ready",
			objs: []client.Object{secretCR(), sourceK8sSecret("s3cret")},
			fake: &fakeSecretsManager{
				createSecret: func(_ context.Context, params *awssm.CreateSecretInput) (*awssm.CreateSecretOutput, error) {
					if aws.ToString(params.Name) != "my-secret" {
						return nil, fmt.Errorf("unexpected secret name %q", aws.ToString(params.Name))
					}
					if aws.ToString(params.SecretString) != "s3cret" {
						return nil, fmt.Errorf("unexpected secret value")
					}
					return &awssm.CreateSecretOutput{ARN: aws.String(testSecretARN)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateSecret to be called")
				}
				if got.Status.ARN != testSecretARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testSecretARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "ARN retained when post-describe value sync fails",
			objs: []client.Object{
				secretCR(func(s *awsv1alpha1.Secret) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Status.ARN = testSecretARN
				}),
				sourceK8sSecret("rotated"),
			},
			fake: &fakeSecretsManager{
				describeSecret: func(_ context.Context, _ *awssm.DescribeSecretInput) (*awssm.DescribeSecretOutput, error) {
					return &awssm.DescribeSecretOutput{ARN: aws.String(testSecretARN)}, nil
				},
				getSecretValue: func(_ context.Context, _ *awssm.GetSecretValueInput) (*awssm.GetSecretValueOutput, error) {
					return &awssm.GetSecretValueOutput{SecretString: aws.String("stale")}, nil
				},
				putSecretValue: func(_ context.Context, _ *awssm.PutSecretValueInput) (*awssm.PutSecretValueOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ARN != testSecretARN {
					t.Errorf("status.arn = %q, want %q (identifier must survive failed sync)", got.Status.ARN, testSecretARN)
				}
				if f.createCalled {
					t.Error("CreateSecret must not be called when secret already exists")
				}
			},
		},
		{
			name: "steady state does not create",
			objs: []client.Object{
				secretCR(func(s *awsv1alpha1.Secret) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Status.ARN = testSecretARN
				}),
				sourceK8sSecret("s3cret"),
			},
			fake: &fakeSecretsManager{
				describeSecret: func(_ context.Context, params *awssm.DescribeSecretInput) (*awssm.DescribeSecretOutput, error) {
					if aws.ToString(params.SecretId) != testSecretARN {
						return nil, fmt.Errorf("unexpected secret id")
					}
					return &awssm.DescribeSecretOutput{ARN: aws.String(testSecretARN)}, nil
				},
				getSecretValue: func(_ context.Context, _ *awssm.GetSecretValueInput) (*awssm.GetSecretValueOutput, error) {
					return &awssm.GetSecretValueOutput{SecretString: aws.String("s3cret")}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateSecret must not be called in steady state")
				}
				if f.putCalled {
					t.Error("PutSecretValue must not be called when value matches")
				}
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "drifted secretStringRef value synced via PutSecretValue",
			objs: []client.Object{
				secretCR(func(s *awsv1alpha1.Secret) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Status.ARN = testSecretARN
				}),
				sourceK8sSecret("rotated-value"),
			},
			fake: &fakeSecretsManager{
				describeSecret: func(_ context.Context, _ *awssm.DescribeSecretInput) (*awssm.DescribeSecretOutput, error) {
					return &awssm.DescribeSecretOutput{ARN: aws.String(testSecretARN)}, nil
				},
				getSecretValue: func(_ context.Context, _ *awssm.GetSecretValueInput) (*awssm.GetSecretValueOutput, error) {
					return &awssm.GetSecretValueOutput{SecretString: aws.String("old-value")}, nil
				},
				putSecretValue: func(_ context.Context, params *awssm.PutSecretValueInput) (*awssm.PutSecretValueOutput, error) {
					if aws.ToString(params.SecretId) != testSecretARN {
						return nil, fmt.Errorf("unexpected secret id")
					}
					return &awssm.PutSecretValueOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				if !f.putCalled {
					t.Error("expected PutSecretValue to be called for drifted value")
				}
				if f.putValue != "rotated-value" {
					t.Errorf("PutSecretValue value = %q, want %q", f.putValue, "rotated-value")
				}
				if f.createCalled {
					t.Error("CreateSecret must not be called")
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status ARN",
			objs: []client.Object{secretCR(func(s *awsv1alpha1.Secret) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Status.ARN = testSecretARN
				s.Spec.RecoveryWindowInDays = 7
			})},
			fake: &fakeSecretsManager{
				deleteSecret: func(_ context.Context, _ *awssm.DeleteSecretInput) (*awssm.DeleteSecretOutput, error) {
					return &awssm.DeleteSecretOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, secretCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteSecret to be called")
				}
				if f.deletedID != testSecretARN {
					t.Errorf("DeleteSecret id = %q, want %q", f.deletedID, testSecretARN)
				}
				if f.deleteInput == nil || aws.ToInt64(f.deleteInput.RecoveryWindowInDays) != 7 {
					t.Errorf("DeleteSecret recovery window = %+v, want 7", f.deleteInput.RecoveryWindowInDays)
				}
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{secretCR(func(s *awsv1alpha1.Secret) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
				s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				s.Status.ARN = testSecretARN
			})},
			fake: &fakeSecretsManager{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, secretCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteSecret must not be called when abandoning")
				}
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete with empty status ARN skips AWS delete (no fallback)",
			objs: []client.Object{secretCR(func(s *awsv1alpha1.Secret) {
				s.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeSecretsManager{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, secretCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeSecretsManager, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteSecret must not be called when no ARN is recorded")
				}
				got := &awsv1alpha1.Secret{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newSecretScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.Secret{}).
				WithObjects(tc.objs...).
				Build()
			r := &SecretReconciler{Client: c, Scheme: scheme, SecretsManagerClient: tc.fake}

			if tc.setup != nil {
				tc.setup(t, ctx, c)
			}

			res, err := r.Reconcile(ctx, req)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.assert(t, ctx, c, tc.fake, res)
		})
	}
}
