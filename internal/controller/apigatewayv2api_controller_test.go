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
	awsapigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/smithy-go"
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

func apigwNotFoundErr() error {
	return &smithy.GenericAPIError{Code: "NotFoundException", Message: "resource not found"}
}

func newAPIGWScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func newAPIGWFakeClient(scheme *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&awsv1alpha1.APIGatewayV2API{},
			&awsv1alpha1.APIGatewayV2Route{},
			&awsv1alpha1.APIGatewayV2Integration{},
			&awsv1alpha1.APIGatewayV2Stage{},
			&awsv1alpha1.APIGatewayV2Authorizer{},
			&awsv1alpha1.APIGatewayV2DomainName{},
			&awsv1alpha1.APIGatewayV2ApiMapping{},
			&awsv1alpha1.APIGatewayV2VpcLink{},
			&awsv1alpha1.RestAPI{},
			&awsv1alpha1.RestAPIDeployment{},
			&awsv1alpha1.RestAPIStage{},
			&awsv1alpha1.LambdaFunction{},
			&awsv1alpha1.Subnet{},
			&awsv1alpha1.SecurityGroup{},
		).
		WithObjects(objs...).
		Build()
}

type fakeAPIGWv2API struct {
	getApi    func(ctx context.Context, params *awsapigwv2.GetApiInput) (*awsapigwv2.GetApiOutput, error)
	createApi func(ctx context.Context, params *awsapigwv2.CreateApiInput) (*awsapigwv2.CreateApiOutput, error)
	updateApi func(ctx context.Context, params *awsapigwv2.UpdateApiInput) (*awsapigwv2.UpdateApiOutput, error)
	deleteApi func(ctx context.Context, params *awsapigwv2.DeleteApiInput) (*awsapigwv2.DeleteApiOutput, error)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	deletedAPIID string
	createInput  *awsapigwv2.CreateApiInput
}

func (f *fakeAPIGWv2API) GetApi(ctx context.Context, params *awsapigwv2.GetApiInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.GetApiOutput, error) {
	if f.getApi == nil {
		return nil, fmt.Errorf("unexpected call to GetApi")
	}
	return f.getApi(ctx, params)
}

func (f *fakeAPIGWv2API) CreateApi(ctx context.Context, params *awsapigwv2.CreateApiInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateApiOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createApi == nil {
		return nil, fmt.Errorf("unexpected call to CreateApi")
	}
	return f.createApi(ctx, params)
}

func (f *fakeAPIGWv2API) UpdateApi(ctx context.Context, params *awsapigwv2.UpdateApiInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateApiOutput, error) {
	f.updateCalled = true
	if f.updateApi == nil {
		return nil, fmt.Errorf("unexpected call to UpdateApi")
	}
	return f.updateApi(ctx, params)
}

func (f *fakeAPIGWv2API) DeleteApi(ctx context.Context, params *awsapigwv2.DeleteApiInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteApiOutput, error) {
	f.deleteCalled = true
	f.deletedAPIID = aws.ToString(params.ApiId)
	if f.deleteApi == nil {
		return nil, fmt.Errorf("unexpected call to DeleteApi")
	}
	return f.deleteApi(ctx, params)
}

const (
	testAPIID       = "abc123"
	testAPIEndpoint = "https://abc123.execute-api.us-east-1.amazonaws.com"
)

func apiCR(mutate ...func(*awsv1alpha1.APIGatewayV2API)) *awsv1alpha1.APIGatewayV2API {
	a := &awsv1alpha1.APIGatewayV2API{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-api",
			Namespace: "default",
		},
		Spec: awsv1alpha1.APIGatewayV2APISpec{
			Name:         "my-api",
			ProtocolType: "HTTP",
			Description:  "test api",
			Tags:         map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(a)
	}
	return a
}

func TestAPIGatewayV2APIReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-api", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeAPIGWv2API
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, res ctrl.Result)
	}{
		{
			name: "create happy path persists identifier and Ready",
			objs: []client.Object{apiCR()},
			fake: &fakeAPIGWv2API{
				createApi: func(_ context.Context, params *awsapigwv2.CreateApiInput) (*awsapigwv2.CreateApiOutput, error) {
					if aws.ToString(params.Name) != "my-api" {
						return nil, fmt.Errorf("unexpected api name %q", aws.ToString(params.Name))
					}
					if string(params.ProtocolType) != "HTTP" {
						return nil, fmt.Errorf("unexpected protocol type %q", params.ProtocolType)
					}
					return &awsapigwv2.CreateApiOutput{
						ApiId:       aws.String(testAPIID),
						ApiEndpoint: aws.String(testAPIEndpoint),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateApi to be called")
				}
				if got.Status.APIID != testAPIID {
					t.Errorf("status.apiId = %q, want %q", got.Status.APIID, testAPIID)
				}
				if got.Status.APIEndpoint != testAPIEndpoint {
					t.Errorf("status.apiEndpoint = %q, want %q", got.Status.APIEndpoint, testAPIEndpoint)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "CORS configuration passed on create",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Spec.CORSConfiguration = &awsv1alpha1.APIGatewayV2CorsConfiguration{
					AllowOrigins: []string{"https://example.com"},
					AllowMethods: []string{"GET", "POST"},
					MaxAge:       aws.Int32(300),
				}
			})},
			fake: &fakeAPIGWv2API{
				createApi: func(_ context.Context, params *awsapigwv2.CreateApiInput) (*awsapigwv2.CreateApiOutput, error) {
					if params.CorsConfiguration == nil {
						return nil, fmt.Errorf("expected CORS configuration")
					}
					if len(params.CorsConfiguration.AllowOrigins) != 1 || params.CorsConfiguration.AllowOrigins[0] != "https://example.com" {
						return nil, fmt.Errorf("unexpected CORS origins %v", params.CorsConfiguration.AllowOrigins)
					}
					return &awsapigwv2.CreateApiOutput{ApiId: aws.String(testAPIID)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateApi to be called")
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Generation = 1
				a.Status.APIID = testAPIID
				a.Status.ObservedGeneration = 1
			})},
			fake: &fakeAPIGWv2API{
				getApi: func(_ context.Context, _ *awsapigwv2.GetApiInput) (*awsapigwv2.GetApiOutput, error) {
					return &awsapigwv2.GetApiOutput{
						ApiId:       aws.String(testAPIID),
						ApiEndpoint: aws.String(testAPIEndpoint),
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateApi must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateApi must not be called when generation is unchanged")
				}
				got := &awsv1alpha1.APIGatewayV2API{}
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
			name: "spec change triggers UpdateApi",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Generation = 2
				a.Status.APIID = testAPIID
				a.Status.ObservedGeneration = 1
			})},
			fake: &fakeAPIGWv2API{
				getApi: func(_ context.Context, _ *awsapigwv2.GetApiInput) (*awsapigwv2.GetApiOutput, error) {
					return &awsapigwv2.GetApiOutput{ApiId: aws.String(testAPIID)}, nil
				},
				updateApi: func(_ context.Context, params *awsapigwv2.UpdateApiInput) (*awsapigwv2.UpdateApiOutput, error) {
					if aws.ToString(params.ApiId) != testAPIID {
						return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.ApiId))
					}
					return &awsapigwv2.UpdateApiOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateApi to be called after spec change")
				}
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "recreates API when it vanished in AWS",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.APIID = "gone123"
			})},
			fake: &fakeAPIGWv2API{
				getApi: func(_ context.Context, _ *awsapigwv2.GetApiInput) (*awsapigwv2.GetApiOutput, error) {
					return nil, apigwNotFoundErr()
				},
				createApi: func(_ context.Context, _ *awsapigwv2.CreateApiInput) (*awsapigwv2.CreateApiOutput, error) {
					return &awsapigwv2.CreateApiOutput{ApiId: aws.String(testAPIID)}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateApi to be called for vanished API")
				}
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.APIID != testAPIID {
					t.Errorf("status.apiId = %q, want %q", got.Status.APIID, testAPIID)
				}
			},
		},
		{
			name: "create error surfaces as Ready=False",
			objs: []client.Object{apiCR()},
			fake: &fakeAPIGWv2API{
				createApi: func(_ context.Context, _ *awsapigwv2.CreateApiInput) (*awsapigwv2.CreateApiOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != awsv1alpha1.ReasonError {
					t.Errorf("Ready condition = %+v, want False/Error", cond)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with status identifier",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Status.APIID = testAPIID
			})},
			fake: &fakeAPIGWv2API{
				deleteApi: func(_ context.Context, _ *awsapigwv2.DeleteApiInput) (*awsapigwv2.DeleteApiOutput, error) {
					return &awsapigwv2.DeleteApiOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, apiCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteApi to be called")
				}
				if f.deletedAPIID != testAPIID {
					t.Errorf("DeleteApi id = %q, want %q", f.deletedAPIID, testAPIID)
				}
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete without recorded identifier skips AWS delete",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeAPIGWv2API{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, apiCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteApi must not be called without a recorded identifier")
				}
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
				a.Finalizers = []string{awsv1alpha1.FinalizerName}
				a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				a.Status.APIID = testAPIID
			})},
			fake: &fakeAPIGWv2API{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, apiCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2API, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteApi must not be called when abandoning")
				}
				got := &awsv1alpha1.APIGatewayV2API{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newAPIGWScheme(t)
			c := newAPIGWFakeClient(scheme, tc.objs...)
			r := &APIGatewayV2APIReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: tc.fake}

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
