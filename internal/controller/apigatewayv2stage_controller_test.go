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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeAPIGWv2Stage struct {
	getStage    func(ctx context.Context, params *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error)
	createStage func(ctx context.Context, params *awsapigwv2.CreateStageInput) (*awsapigwv2.CreateStageOutput, error)
	updateStage func(ctx context.Context, params *awsapigwv2.UpdateStageInput) (*awsapigwv2.UpdateStageOutput, error)
	deleteStage func(ctx context.Context, params *awsapigwv2.DeleteStageInput) (*awsapigwv2.DeleteStageOutput, error)

	createCalled     bool
	updateCalled     bool
	deleteCalled     bool
	deletedStageName string
	deletedAPIID     string
	createInput      *awsapigwv2.CreateStageInput
}

func (f *fakeAPIGWv2Stage) GetStage(ctx context.Context, params *awsapigwv2.GetStageInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.GetStageOutput, error) {
	if f.getStage == nil {
		return nil, fmt.Errorf("unexpected call to GetStage")
	}
	return f.getStage(ctx, params)
}

func (f *fakeAPIGWv2Stage) CreateStage(ctx context.Context, params *awsapigwv2.CreateStageInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateStageOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createStage == nil {
		return nil, fmt.Errorf("unexpected call to CreateStage")
	}
	return f.createStage(ctx, params)
}

func (f *fakeAPIGWv2Stage) UpdateStage(ctx context.Context, params *awsapigwv2.UpdateStageInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateStageOutput, error) {
	f.updateCalled = true
	if f.updateStage == nil {
		return nil, fmt.Errorf("unexpected call to UpdateStage")
	}
	return f.updateStage(ctx, params)
}

func (f *fakeAPIGWv2Stage) DeleteStage(ctx context.Context, params *awsapigwv2.DeleteStageInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteStageOutput, error) {
	f.deleteCalled = true
	f.deletedStageName = aws.ToString(params.StageName)
	f.deletedAPIID = aws.ToString(params.ApiId)
	if f.deleteStage == nil {
		return nil, fmt.Errorf("unexpected call to DeleteStage")
	}
	return f.deleteStage(ctx, params)
}

func stageCR(mutate ...func(*awsv1alpha1.APIGatewayV2Stage)) *awsv1alpha1.APIGatewayV2Stage {
	s := &awsv1alpha1.APIGatewayV2Stage{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-stage",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.APIGatewayV2StageSpec{
			APIRef:     awsv1alpha1.APIRef{Name: "my-api"},
			StageName:  "prod",
			AutoDeploy: true,
			StageVariables: map[string]string{
				"env": "prod",
			},
			DefaultRouteSettings: &awsv1alpha1.APIGatewayV2RouteSettings{
				ThrottlingBurstLimit: aws.Int32(100),
				ThrottlingRateLimit:  aws.Float64(50),
			},
		},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

func readyAPICR() *awsv1alpha1.APIGatewayV2API {
	return apiCR(func(a *awsv1alpha1.APIGatewayV2API) {
		a.Status.APIID = testAPIID
	})
}

func TestAPIGatewayV2StageReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-stage", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeAPIGWv2Stage
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, res ctrl.Result)
	}{
		{
			name: "create happy path persists identifiers and Ready",
			objs: []client.Object{stageCR(), readyAPICR()},
			fake: &fakeAPIGWv2Stage{
				getStage: func(_ context.Context, _ *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error) {
					return nil, apigwNotFoundErr()
				},
				createStage: func(_ context.Context, params *awsapigwv2.CreateStageInput) (*awsapigwv2.CreateStageOutput, error) {
					if aws.ToString(params.ApiId) != testAPIID {
						return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.ApiId))
					}
					if aws.ToString(params.StageName) != "prod" {
						return nil, fmt.Errorf("unexpected stage name %q", aws.ToString(params.StageName))
					}
					if !aws.ToBool(params.AutoDeploy) {
						return nil, fmt.Errorf("expected autoDeploy true")
					}
					if params.DefaultRouteSettings == nil || aws.ToInt32(params.DefaultRouteSettings.ThrottlingBurstLimit) != 100 {
						return nil, fmt.Errorf("expected default route settings with burst limit")
					}
					return &awsapigwv2.CreateStageOutput{StageName: params.StageName}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				got := &awsv1alpha1.APIGatewayV2Stage{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if !f.createCalled {
					t.Error("expected CreateStage to be called")
				}
				if got.Status.StageName != "prod" {
					t.Errorf("status.stageName = %q, want %q", got.Status.StageName, "prod")
				}
				if got.Status.APIID != testAPIID {
					t.Errorf("status.apiId = %q, want %q", got.Status.APIID, testAPIID)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "dependency not ready requeues without create",
			objs: []client.Object{stageCR(), apiCR()}, // API CR has no apiId yet
			fake: &fakeAPIGWv2Stage{},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateStage must not be called while API dependency is not ready")
				}
			},
		},
		{
			name: "direct apiId bypasses CR lookup",
			objs: []client.Object{stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
				s.Spec.APIRef = awsv1alpha1.APIRef{APIID: "direct456"}
			})},
			fake: &fakeAPIGWv2Stage{
				getStage: func(_ context.Context, params *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error) {
					if aws.ToString(params.ApiId) != "direct456" {
						return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.ApiId))
					}
					return nil, apigwNotFoundErr()
				},
				createStage: func(_ context.Context, params *awsapigwv2.CreateStageInput) (*awsapigwv2.CreateStageOutput, error) {
					return &awsapigwv2.CreateStageOutput{StageName: params.StageName}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateStage to be called")
				}
				if aws.ToString(f.createInput.ApiId) != "direct456" {
					t.Errorf("CreateStage api id = %q, want direct456", aws.ToString(f.createInput.ApiId))
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{
				stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Generation = 1
					s.Status.StageName = "prod"
					s.Status.APIID = testAPIID
					s.Status.ObservedGeneration = 1
				}),
				readyAPICR(),
			},
			fake: &fakeAPIGWv2Stage{
				getStage: func(_ context.Context, _ *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error) {
					return &awsapigwv2.GetStageOutput{StageName: aws.String("prod")}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateStage must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateStage must not be called when generation is unchanged")
				}
				got := &awsv1alpha1.APIGatewayV2Stage{}
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
			name: "spec change triggers UpdateStage",
			objs: []client.Object{
				stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Generation = 2
					s.Status.StageName = "prod"
					s.Status.APIID = testAPIID
					s.Status.ObservedGeneration = 1
				}),
				readyAPICR(),
			},
			fake: &fakeAPIGWv2Stage{
				getStage: func(_ context.Context, _ *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error) {
					return &awsapigwv2.GetStageOutput{StageName: aws.String("prod")}, nil
				},
				updateStage: func(_ context.Context, params *awsapigwv2.UpdateStageInput) (*awsapigwv2.UpdateStageOutput, error) {
					if aws.ToString(params.StageName) != "prod" {
						return nil, fmt.Errorf("unexpected stage name %q", aws.ToString(params.StageName))
					}
					if len(params.StageVariables) == 0 {
						return nil, fmt.Errorf("expected stage variables on update")
					}
					return &awsapigwv2.UpdateStageOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateStage to be called after spec change")
				}
				got := &awsv1alpha1.APIGatewayV2Stage{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "create error surfaces as Ready=False",
			objs: []client.Object{stageCR(), readyAPICR()},
			fake: &fakeAPIGWv2Stage{
				getStage: func(_ context.Context, _ *awsapigwv2.GetStageInput) (*awsapigwv2.GetStageOutput, error) {
					return nil, apigwNotFoundErr()
				},
				createStage: func(_ context.Context, _ *awsapigwv2.CreateStageInput) (*awsapigwv2.CreateStageOutput, error) {
					return nil, fmt.Errorf("throttled")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				got := &awsv1alpha1.APIGatewayV2Stage{}
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
			name: "delete with finalizer calls AWS delete with status identifiers",
			objs: []client.Object{
				stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Status.StageName = "prod"
					s.Status.APIID = testAPIID
				}),
			},
			fake: &fakeAPIGWv2Stage{
				deleteStage: func(_ context.Context, _ *awsapigwv2.DeleteStageInput) (*awsapigwv2.DeleteStageOutput, error) {
					return &awsapigwv2.DeleteStageOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, stageCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteStage to be called")
				}
				if f.deletedAPIID != testAPIID || f.deletedStageName != "prod" {
					t.Errorf("DeleteStage got api=%q stage=%q, want %q/%q", f.deletedAPIID, f.deletedStageName, testAPIID, "prod")
				}
				got := &awsv1alpha1.APIGatewayV2Stage{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete fallback resolves API from ref when status empty",
			objs: []client.Object{
				stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
				}),
				readyAPICR(),
			},
			fake: &fakeAPIGWv2Stage{
				deleteStage: func(_ context.Context, _ *awsapigwv2.DeleteStageInput) (*awsapigwv2.DeleteStageOutput, error) {
					return &awsapigwv2.DeleteStageOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, stageCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteStage to be called via spec-based fallback")
				}
				if f.deletedAPIID != testAPIID || f.deletedStageName != "prod" {
					t.Errorf("DeleteStage got api=%q stage=%q, want %q/%q", f.deletedAPIID, f.deletedStageName, testAPIID, "prod")
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{
				stageCR(func(s *awsv1alpha1.APIGatewayV2Stage) {
					s.Finalizers = []string{awsv1alpha1.FinalizerName}
					s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
					s.Status.StageName = "prod"
					s.Status.APIID = testAPIID
				}),
			},
			fake: &fakeAPIGWv2Stage{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, stageCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAPIGWv2Stage, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteStage must not be called when abandoning")
				}
				got := &awsv1alpha1.APIGatewayV2Stage{}
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
			r := &APIGatewayV2StageReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: tc.fake}

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
