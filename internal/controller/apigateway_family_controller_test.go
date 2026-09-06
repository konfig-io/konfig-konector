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

// Create + delete + abandon coverage for the remaining apigateway-family
// kinds. APIGatewayV2API and APIGatewayV2Stage have full table-driven tests
// in their own files.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsapigw "github.com/aws/aws-sdk-go-v2/service/apigateway"
	awsapigwv2 "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	apigwv2types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// ---- APIGatewayV2Route ----

type fakeAPIGWv2Route struct {
	createRoute  func(ctx context.Context, params *awsapigwv2.CreateRouteInput) (*awsapigwv2.CreateRouteOutput, error)
	deleteCalled bool
	createCalled bool
	createInput  *awsapigwv2.CreateRouteInput
}

func (f *fakeAPIGWv2Route) GetRoute(context.Context, *awsapigwv2.GetRouteInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.GetRouteOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeAPIGWv2Route) CreateRoute(ctx context.Context, params *awsapigwv2.CreateRouteInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateRouteOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createRoute == nil {
		return nil, fmt.Errorf("unexpected call to CreateRoute")
	}
	return f.createRoute(ctx, params)
}
func (f *fakeAPIGWv2Route) UpdateRoute(context.Context, *awsapigwv2.UpdateRouteInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateRouteOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateRoute")
}
func (f *fakeAPIGWv2Route) DeleteRoute(context.Context, *awsapigwv2.DeleteRouteInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteRouteOutput, error) {
	f.deleteCalled = true
	return &awsapigwv2.DeleteRouteOutput{}, nil
}

func routeCR(mutate ...func(*awsv1alpha1.APIGatewayV2Route)) *awsv1alpha1.APIGatewayV2Route {
	r := &awsv1alpha1.APIGatewayV2Route{
		ObjectMeta: metav1.ObjectMeta{Name: "my-route", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2RouteSpec{
			APIRef:         awsv1alpha1.APIRef{Name: "my-api"},
			RouteKey:       "GET /pets",
			IntegrationRef: &awsv1alpha1.IntegrationRef{Name: "my-integration"},
		},
	}
	for _, m := range mutate {
		m(r)
	}
	return r
}

func TestAPIGatewayV2RouteLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-route", Namespace: "default"}}
	readyIntegration := &awsv1alpha1.APIGatewayV2Integration{
		ObjectMeta: metav1.ObjectMeta{Name: "my-integration", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2IntegrationSpec{
			APIRef:          awsv1alpha1.APIRef{Name: "my-api"},
			IntegrationType: "AWS_PROXY",
		},
		Status: awsv1alpha1.APIGatewayV2IntegrationStatus{IntegrationID: "integ1"},
	}

	t.Run("create resolves refs and persists route id", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, routeCR(), readyAPICR(), readyIntegration)
		fake := &fakeAPIGWv2Route{
			createRoute: func(_ context.Context, params *awsapigwv2.CreateRouteInput) (*awsapigwv2.CreateRouteOutput, error) {
				if aws.ToString(params.ApiId) != testAPIID {
					return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.ApiId))
				}
				if aws.ToString(params.Target) != "integrations/integ1" {
					return nil, fmt.Errorf("unexpected target %q", aws.ToString(params.Target))
				}
				return &awsapigwv2.CreateRouteOutput{RouteId: aws.String("route1")}, nil
			},
		}
		r := &APIGatewayV2RouteReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2Route{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.RouteID != "route1" || got.Status.APIID != testAPIID {
			t.Errorf("status = %+v, want routeId=route1 apiId=%s", got.Status, testAPIID)
		}
	})

	t.Run("dependency not ready requeues without create", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		// Integration exists but has no ID yet.
		pendingIntegration := readyIntegration.DeepCopy()
		pendingIntegration.Status.IntegrationID = ""
		c := newAPIGWFakeClient(scheme, routeCR(), readyAPICR(), pendingIntegration)
		fake := &fakeAPIGWv2Route{}
		r := &APIGatewayV2RouteReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if fake.createCalled {
			t.Error("CreateRoute must not be called while integration is not ready")
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := routeCR(func(r *awsv1alpha1.APIGatewayV2Route) {
			r.Finalizers = []string{awsv1alpha1.FinalizerName}
			r.Status.RouteID = "route1"
			r.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, routeCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Route{}
		r := &APIGatewayV2RouteReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteRoute to be called")
		}
		got := &awsv1alpha1.APIGatewayV2Route{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := routeCR(func(r *awsv1alpha1.APIGatewayV2Route) {
			r.Finalizers = []string{awsv1alpha1.FinalizerName}
			r.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			r.Status.RouteID = "route1"
			r.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, routeCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Route{}
		r := &APIGatewayV2RouteReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteRoute must not be called when abandoning")
		}
	})
}

// ---- APIGatewayV2Integration ----

type fakeAPIGWv2Integration struct {
	createIntegration func(ctx context.Context, params *awsapigwv2.CreateIntegrationInput) (*awsapigwv2.CreateIntegrationOutput, error)
	createCalled      bool
	deleteCalled      bool
	createInput       *awsapigwv2.CreateIntegrationInput
}

func (f *fakeAPIGWv2Integration) GetIntegration(context.Context, *awsapigwv2.GetIntegrationInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.GetIntegrationOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeAPIGWv2Integration) CreateIntegration(ctx context.Context, params *awsapigwv2.CreateIntegrationInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateIntegrationOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createIntegration == nil {
		return nil, fmt.Errorf("unexpected call to CreateIntegration")
	}
	return f.createIntegration(ctx, params)
}
func (f *fakeAPIGWv2Integration) UpdateIntegration(context.Context, *awsapigwv2.UpdateIntegrationInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateIntegrationOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateIntegration")
}
func (f *fakeAPIGWv2Integration) DeleteIntegration(context.Context, *awsapigwv2.DeleteIntegrationInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteIntegrationOutput, error) {
	f.deleteCalled = true
	return &awsapigwv2.DeleteIntegrationOutput{}, nil
}

func integrationCR(mutate ...func(*awsv1alpha1.APIGatewayV2Integration)) *awsv1alpha1.APIGatewayV2Integration {
	i := &awsv1alpha1.APIGatewayV2Integration{
		ObjectMeta: metav1.ObjectMeta{Name: "my-integration", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2IntegrationSpec{
			APIRef:               awsv1alpha1.APIRef{Name: "my-api"},
			IntegrationType:      "AWS_PROXY",
			FunctionRef:          &awsv1alpha1.LambdaFunctionRef{Name: "my-fn"},
			PayloadFormatVersion: "2.0",
		},
	}
	for _, m := range mutate {
		m(i)
	}
	return i
}

func TestAPIGatewayV2IntegrationLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-integration", Namespace: "default"}}
	const fnARN = "arn:aws:lambda:us-east-1:123456789012:function:my-fn"
	readyFn := &awsv1alpha1.LambdaFunction{
		ObjectMeta: metav1.ObjectMeta{Name: "my-fn", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Status:     awsv1alpha1.LambdaFunctionStatus{FunctionARN: fnARN},
	}

	t.Run("create resolves lambda ref into integration URI", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, integrationCR(), readyAPICR(), readyFn)
		fake := &fakeAPIGWv2Integration{
			createIntegration: func(_ context.Context, params *awsapigwv2.CreateIntegrationInput) (*awsapigwv2.CreateIntegrationOutput, error) {
				if aws.ToString(params.IntegrationUri) != fnARN {
					return nil, fmt.Errorf("unexpected uri %q", aws.ToString(params.IntegrationUri))
				}
				if params.IntegrationType != apigwv2types.IntegrationTypeAwsProxy {
					return nil, fmt.Errorf("unexpected type %q", params.IntegrationType)
				}
				if aws.ToString(params.PayloadFormatVersion) != "2.0" {
					return nil, fmt.Errorf("unexpected payload format %q", aws.ToString(params.PayloadFormatVersion))
				}
				return &awsapigwv2.CreateIntegrationOutput{IntegrationId: aws.String("integ1")}, nil
			},
		}
		r := &APIGatewayV2IntegrationReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2Integration{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.IntegrationID != "integ1" || got.Status.APIID != testAPIID {
			t.Errorf("status = %+v, want integrationId=integ1 apiId=%s", got.Status, testAPIID)
		}
	})

	t.Run("lambda ref not ready requeues", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		pendingFn := readyFn.DeepCopy()
		pendingFn.Status.FunctionARN = ""
		c := newAPIGWFakeClient(scheme, integrationCR(), readyAPICR(), pendingFn)
		fake := &fakeAPIGWv2Integration{}
		r := &APIGatewayV2IntegrationReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if fake.createCalled {
			t.Error("CreateIntegration must not be called while lambda is not ready")
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := integrationCR(func(i *awsv1alpha1.APIGatewayV2Integration) {
			i.Finalizers = []string{awsv1alpha1.FinalizerName}
			i.Status.IntegrationID = "integ1"
			i.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, integrationCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Integration{}
		r := &APIGatewayV2IntegrationReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteIntegration to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := integrationCR(func(i *awsv1alpha1.APIGatewayV2Integration) {
			i.Finalizers = []string{awsv1alpha1.FinalizerName}
			i.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			i.Status.IntegrationID = "integ1"
			i.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, integrationCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Integration{}
		r := &APIGatewayV2IntegrationReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteIntegration must not be called when abandoning")
		}
	})
}

// ---- APIGatewayV2Authorizer ----

type fakeAPIGWv2Authorizer struct {
	createAuthorizer func(ctx context.Context, params *awsapigwv2.CreateAuthorizerInput) (*awsapigwv2.CreateAuthorizerOutput, error)
	createCalled     bool
	deleteCalled     bool
}

func (f *fakeAPIGWv2Authorizer) GetAuthorizer(context.Context, *awsapigwv2.GetAuthorizerInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.GetAuthorizerOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeAPIGWv2Authorizer) CreateAuthorizer(ctx context.Context, params *awsapigwv2.CreateAuthorizerInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateAuthorizerOutput, error) {
	f.createCalled = true
	if f.createAuthorizer == nil {
		return nil, fmt.Errorf("unexpected call to CreateAuthorizer")
	}
	return f.createAuthorizer(ctx, params)
}
func (f *fakeAPIGWv2Authorizer) UpdateAuthorizer(context.Context, *awsapigwv2.UpdateAuthorizerInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateAuthorizerOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateAuthorizer")
}
func (f *fakeAPIGWv2Authorizer) DeleteAuthorizer(context.Context, *awsapigwv2.DeleteAuthorizerInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteAuthorizerOutput, error) {
	f.deleteCalled = true
	return &awsapigwv2.DeleteAuthorizerOutput{}, nil
}

func authorizerCR(mutate ...func(*awsv1alpha1.APIGatewayV2Authorizer)) *awsv1alpha1.APIGatewayV2Authorizer {
	a := &awsv1alpha1.APIGatewayV2Authorizer{
		ObjectMeta: metav1.ObjectMeta{Name: "my-authorizer", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2AuthorizerSpec{
			APIRef:         awsv1alpha1.APIRef{Name: "my-api"},
			Name:           "jwt-auth",
			AuthorizerType: "JWT",
			IdentitySource: []string{"$request.header.Authorization"},
			JWTConfiguration: &awsv1alpha1.JWTConfiguration{
				Issuer:   "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_abc",
				Audience: []string{"client1"},
			},
		},
	}
	for _, m := range mutate {
		m(a)
	}
	return a
}

func TestAPIGatewayV2AuthorizerLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-authorizer", Namespace: "default"}}

	t.Run("create JWT authorizer persists id", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, authorizerCR(), readyAPICR())
		fake := &fakeAPIGWv2Authorizer{
			createAuthorizer: func(_ context.Context, params *awsapigwv2.CreateAuthorizerInput) (*awsapigwv2.CreateAuthorizerOutput, error) {
				if params.AuthorizerType != apigwv2types.AuthorizerTypeJwt {
					return nil, fmt.Errorf("unexpected type %q", params.AuthorizerType)
				}
				if params.JwtConfiguration == nil || aws.ToString(params.JwtConfiguration.Issuer) == "" {
					return nil, fmt.Errorf("expected JWT configuration")
				}
				return &awsapigwv2.CreateAuthorizerOutput{AuthorizerId: aws.String("auth1")}, nil
			},
		}
		r := &APIGatewayV2AuthorizerReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2Authorizer{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.AuthorizerID != "auth1" {
			t.Errorf("status.authorizerId = %q, want auth1", got.Status.AuthorizerID)
		}
	})

	t.Run("REQUEST authorizer builds lambda invocation URI from ref", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		const fnARN = "arn:aws:lambda:us-east-1:123456789012:function:authz"
		readyFn := &awsv1alpha1.LambdaFunction{
			ObjectMeta: metav1.ObjectMeta{Name: "authz-fn", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Status:     awsv1alpha1.LambdaFunctionStatus{FunctionARN: fnARN},
		}
		obj := authorizerCR(func(a *awsv1alpha1.APIGatewayV2Authorizer) {
			a.Spec.AuthorizerType = "REQUEST"
			a.Spec.JWTConfiguration = nil
			a.Spec.FunctionRef = &awsv1alpha1.LambdaFunctionRef{Name: "authz-fn"}
			a.Spec.AuthorizerPayloadFormatVersion = "2.0"
		})
		c := newAPIGWFakeClient(scheme, obj, readyAPICR(), readyFn)
		fake := &fakeAPIGWv2Authorizer{
			createAuthorizer: func(_ context.Context, params *awsapigwv2.CreateAuthorizerInput) (*awsapigwv2.CreateAuthorizerOutput, error) {
				uri := aws.ToString(params.AuthorizerUri)
				if !strings.Contains(uri, "arn:aws:apigateway:us-east-1:lambda:path/2015-03-31/functions/"+fnARN+"/invocations") {
					return nil, fmt.Errorf("unexpected authorizer uri %q", uri)
				}
				return &awsapigwv2.CreateAuthorizerOutput{AuthorizerId: aws.String("auth2")}, nil
			},
		}
		r := &APIGatewayV2AuthorizerReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.createCalled {
			t.Error("expected CreateAuthorizer to be called")
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := authorizerCR(func(a *awsv1alpha1.APIGatewayV2Authorizer) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Status.AuthorizerID = "auth1"
			a.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, authorizerCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Authorizer{}
		r := &APIGatewayV2AuthorizerReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteAuthorizer to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := authorizerCR(func(a *awsv1alpha1.APIGatewayV2Authorizer) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.Status.AuthorizerID = "auth1"
			a.Status.APIID = testAPIID
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, authorizerCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Authorizer{}
		r := &APIGatewayV2AuthorizerReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteAuthorizer must not be called when abandoning")
		}
	})
}

// ---- APIGatewayV2DomainName ----

type fakeAPIGWv2Domain struct {
	createDomain func(ctx context.Context, params *awsapigwv2.CreateDomainNameInput) (*awsapigwv2.CreateDomainNameOutput, error)
	createCalled bool
	deleteCalled bool
	deletedName  string
}

func (f *fakeAPIGWv2Domain) GetDomainName(context.Context, *awsapigwv2.GetDomainNameInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.GetDomainNameOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeAPIGWv2Domain) CreateDomainName(ctx context.Context, params *awsapigwv2.CreateDomainNameInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateDomainNameOutput, error) {
	f.createCalled = true
	if f.createDomain == nil {
		return nil, fmt.Errorf("unexpected call to CreateDomainName")
	}
	return f.createDomain(ctx, params)
}
func (f *fakeAPIGWv2Domain) UpdateDomainName(context.Context, *awsapigwv2.UpdateDomainNameInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateDomainNameOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateDomainName")
}
func (f *fakeAPIGWv2Domain) DeleteDomainName(_ context.Context, params *awsapigwv2.DeleteDomainNameInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteDomainNameOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.DomainName)
	return &awsapigwv2.DeleteDomainNameOutput{}, nil
}

func domainCR(mutate ...func(*awsv1alpha1.APIGatewayV2DomainName)) *awsv1alpha1.APIGatewayV2DomainName {
	d := &awsv1alpha1.APIGatewayV2DomainName{
		ObjectMeta: metav1.ObjectMeta{Name: "my-domain", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2DomainNameSpec{
			DomainName:     "api.example.com",
			CertificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/abc",
		},
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

func TestAPIGatewayV2DomainNameLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-domain", Namespace: "default"}}

	t.Run("create persists domain and regional target", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, domainCR())
		fake := &fakeAPIGWv2Domain{
			createDomain: func(_ context.Context, params *awsapigwv2.CreateDomainNameInput) (*awsapigwv2.CreateDomainNameOutput, error) {
				if len(params.DomainNameConfigurations) != 1 ||
					aws.ToString(params.DomainNameConfigurations[0].CertificateArn) == "" {
					return nil, fmt.Errorf("expected a certificate ARN configuration")
				}
				return &awsapigwv2.CreateDomainNameOutput{
					DomainName: params.DomainName,
					DomainNameConfigurations: []apigwv2types.DomainNameConfiguration{{
						ApiGatewayDomainName: aws.String("d-abc.execute-api.us-east-1.amazonaws.com"),
						HostedZoneId:         aws.String("Z2FDTNDATAQYW2"),
					}},
				}, nil
			},
		}
		r := &APIGatewayV2DomainNameReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2DomainName{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DomainName != "api.example.com" {
			t.Errorf("status.domainName = %q", got.Status.DomainName)
		}
		if got.Status.APIGatewayDomainName != "d-abc.execute-api.us-east-1.amazonaws.com" {
			t.Errorf("status.apiGatewayDomainName = %q", got.Status.APIGatewayDomainName)
		}
	})

	t.Run("delete uses spec fallback when status empty", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := domainCR(func(d *awsv1alpha1.APIGatewayV2DomainName) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, domainCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Domain{}
		r := &APIGatewayV2DomainNameReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled || fake.deletedName != "api.example.com" {
			t.Errorf("expected DeleteDomainName(api.example.com), got called=%v name=%q", fake.deleteCalled, fake.deletedName)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := domainCR(func(d *awsv1alpha1.APIGatewayV2DomainName) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, domainCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Domain{}
		r := &APIGatewayV2DomainNameReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteDomainName must not be called when abandoning")
		}
	})
}

// ---- APIGatewayV2ApiMapping ----

type fakeAPIGWv2Mapping struct {
	createMapping func(ctx context.Context, params *awsapigwv2.CreateApiMappingInput) (*awsapigwv2.CreateApiMappingOutput, error)
	createCalled  bool
	deleteCalled  bool
}

func (f *fakeAPIGWv2Mapping) GetApiMapping(context.Context, *awsapigwv2.GetApiMappingInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.GetApiMappingOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeAPIGWv2Mapping) CreateApiMapping(ctx context.Context, params *awsapigwv2.CreateApiMappingInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateApiMappingOutput, error) {
	f.createCalled = true
	if f.createMapping == nil {
		return nil, fmt.Errorf("unexpected call to CreateApiMapping")
	}
	return f.createMapping(ctx, params)
}
func (f *fakeAPIGWv2Mapping) UpdateApiMapping(context.Context, *awsapigwv2.UpdateApiMappingInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.UpdateApiMappingOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateApiMapping")
}
func (f *fakeAPIGWv2Mapping) DeleteApiMapping(context.Context, *awsapigwv2.DeleteApiMappingInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteApiMappingOutput, error) {
	f.deleteCalled = true
	return &awsapigwv2.DeleteApiMappingOutput{}, nil
}

func mappingCR(mutate ...func(*awsv1alpha1.APIGatewayV2ApiMapping)) *awsv1alpha1.APIGatewayV2ApiMapping {
	m := &awsv1alpha1.APIGatewayV2ApiMapping{
		ObjectMeta: metav1.ObjectMeta{Name: "my-mapping", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2ApiMappingSpec{
			APIRef:        awsv1alpha1.APIRef{Name: "my-api"},
			DomainNameRef: awsv1alpha1.DomainNameRef{Name: "my-domain"},
			Stage:         "prod",
		},
	}
	for _, mu := range mutate {
		mu(m)
	}
	return m
}

func TestAPIGatewayV2ApiMappingLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-mapping", Namespace: "default"}}
	readyDomain := domainCR(func(d *awsv1alpha1.APIGatewayV2DomainName) {
		d.Status.DomainName = "api.example.com"
	})

	t.Run("create resolves refs and persists mapping id", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, mappingCR(), readyAPICR(), readyDomain)
		fake := &fakeAPIGWv2Mapping{
			createMapping: func(_ context.Context, params *awsapigwv2.CreateApiMappingInput) (*awsapigwv2.CreateApiMappingOutput, error) {
				if aws.ToString(params.DomainName) != "api.example.com" {
					return nil, fmt.Errorf("unexpected domain %q", aws.ToString(params.DomainName))
				}
				if aws.ToString(params.ApiId) != testAPIID {
					return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.ApiId))
				}
				return &awsapigwv2.CreateApiMappingOutput{ApiMappingId: aws.String("map1")}, nil
			},
		}
		r := &APIGatewayV2ApiMappingReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2ApiMapping{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.APIMappingID != "map1" || got.Status.DomainName != "api.example.com" {
			t.Errorf("status = %+v", got.Status)
		}
	})

	t.Run("domain not ready requeues", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		pendingDomain := domainCR()
		c := newAPIGWFakeClient(scheme, mappingCR(), readyAPICR(), pendingDomain)
		fake := &fakeAPIGWv2Mapping{}
		r := &APIGatewayV2ApiMappingReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if fake.createCalled {
			t.Error("CreateApiMapping must not be called while domain is not ready")
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := mappingCR(func(m *awsv1alpha1.APIGatewayV2ApiMapping) {
			m.Finalizers = []string{awsv1alpha1.FinalizerName}
			m.Status.APIMappingID = "map1"
			m.Status.DomainName = "api.example.com"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, mappingCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Mapping{}
		r := &APIGatewayV2ApiMappingReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteApiMapping to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := mappingCR(func(m *awsv1alpha1.APIGatewayV2ApiMapping) {
			m.Finalizers = []string{awsv1alpha1.FinalizerName}
			m.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			m.Status.APIMappingID = "map1"
			m.Status.DomainName = "api.example.com"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, mappingCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2Mapping{}
		r := &APIGatewayV2ApiMappingReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteApiMapping must not be called when abandoning")
		}
	})
}

// ---- APIGatewayV2VpcLink ----

type fakeAPIGWv2VpcLink struct {
	createVpcLink func(ctx context.Context, params *awsapigwv2.CreateVpcLinkInput) (*awsapigwv2.CreateVpcLinkOutput, error)
	getVpcLink    func(ctx context.Context, params *awsapigwv2.GetVpcLinkInput) (*awsapigwv2.GetVpcLinkOutput, error)
	createCalled  bool
	deleteCalled  bool
}

func (f *fakeAPIGWv2VpcLink) GetVpcLink(ctx context.Context, params *awsapigwv2.GetVpcLinkInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.GetVpcLinkOutput, error) {
	if f.getVpcLink == nil {
		return nil, fmt.Errorf("unexpected call to GetVpcLink")
	}
	return f.getVpcLink(ctx, params)
}
func (f *fakeAPIGWv2VpcLink) CreateVpcLink(ctx context.Context, params *awsapigwv2.CreateVpcLinkInput, _ ...func(*awsapigwv2.Options)) (*awsapigwv2.CreateVpcLinkOutput, error) {
	f.createCalled = true
	if f.createVpcLink == nil {
		return nil, fmt.Errorf("unexpected call to CreateVpcLink")
	}
	return f.createVpcLink(ctx, params)
}
func (f *fakeAPIGWv2VpcLink) DeleteVpcLink(context.Context, *awsapigwv2.DeleteVpcLinkInput, ...func(*awsapigwv2.Options)) (*awsapigwv2.DeleteVpcLinkOutput, error) {
	f.deleteCalled = true
	return &awsapigwv2.DeleteVpcLinkOutput{}, nil
}

func vpcLinkCR(mutate ...func(*awsv1alpha1.APIGatewayV2VpcLink)) *awsv1alpha1.APIGatewayV2VpcLink {
	v := &awsv1alpha1.APIGatewayV2VpcLink{
		ObjectMeta: metav1.ObjectMeta{Name: "my-vpclink", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.APIGatewayV2VpcLinkSpec{
			Name:       "my-vpclink",
			SubnetRefs: []awsv1alpha1.SubnetRef{{ID: "subnet-1"}, {ID: "subnet-2"}},
			SecurityGroupRefs: []awsv1alpha1.SecurityGroupRef{
				{ID: "sg-1"},
			},
		},
	}
	for _, m := range mutate {
		m(v)
	}
	return v
}

func TestAPIGatewayV2VpcLinkLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-vpclink", Namespace: "default"}}

	t.Run("create persists vpc link id and polls", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, vpcLinkCR())
		fake := &fakeAPIGWv2VpcLink{
			createVpcLink: func(_ context.Context, params *awsapigwv2.CreateVpcLinkInput) (*awsapigwv2.CreateVpcLinkOutput, error) {
				if len(params.SubnetIds) != 2 || len(params.SecurityGroupIds) != 1 {
					return nil, fmt.Errorf("unexpected subnets/sgs %v/%v", params.SubnetIds, params.SecurityGroupIds)
				}
				return &awsapigwv2.CreateVpcLinkOutput{
					VpcLinkId:     aws.String("vl1"),
					VpcLinkStatus: apigwv2types.VpcLinkStatusPending,
				}, nil
			},
		}
		r := &APIGatewayV2VpcLinkReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueAPIGWPolling {
			t.Errorf("result = %+v, want requeueAPIGWPolling", res)
		}
		got := &awsv1alpha1.APIGatewayV2VpcLink{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.VPCLinkID != "vl1" {
			t.Errorf("status.vpcLinkId = %q, want vl1", got.Status.VPCLinkID)
		}
	})

	t.Run("available vpc link becomes Ready", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := vpcLinkCR(func(v *awsv1alpha1.APIGatewayV2VpcLink) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Generation = 1
			v.Status.VPCLinkID = "vl1"
			v.Status.ObservedGeneration = 1
		})
		c := newAPIGWFakeClient(scheme, obj)
		fake := &fakeAPIGWv2VpcLink{
			getVpcLink: func(_ context.Context, _ *awsapigwv2.GetVpcLinkInput) (*awsapigwv2.GetVpcLinkOutput, error) {
				return &awsapigwv2.GetVpcLinkOutput{
					VpcLinkId:     aws.String("vl1"),
					VpcLinkStatus: apigwv2types.VpcLinkStatusAvailable,
				}, nil
			},
		}
		r := &APIGatewayV2VpcLinkReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.APIGatewayV2VpcLink{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := vpcLinkCR(func(v *awsv1alpha1.APIGatewayV2VpcLink) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Status.VPCLinkID = "vl1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, vpcLinkCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2VpcLink{}
		r := &APIGatewayV2VpcLinkReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteVpcLink to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := vpcLinkCR(func(v *awsv1alpha1.APIGatewayV2VpcLink) {
			v.Finalizers = []string{awsv1alpha1.FinalizerName}
			v.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			v.Status.VPCLinkID = "vl1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, vpcLinkCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeAPIGWv2VpcLink{}
		r := &APIGatewayV2VpcLinkReconciler{Client: c, Scheme: scheme, APIGatewayV2Client: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteVpcLink must not be called when abandoning")
		}
	})
}

// ---- RestAPI ----

type fakeRestAPI struct {
	createRestApi func(ctx context.Context, params *awsapigw.CreateRestApiInput) (*awsapigw.CreateRestApiOutput, error)
	putRestApi    func(ctx context.Context, params *awsapigw.PutRestApiInput) (*awsapigw.PutRestApiOutput, error)
	createCalled  bool
	putCalled     bool
	deleteCalled  bool
	deletedAPIID  string
}

func (f *fakeRestAPI) GetRestApi(context.Context, *awsapigw.GetRestApiInput, ...func(*awsapigw.Options)) (*awsapigw.GetRestApiOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeRestAPI) CreateRestApi(ctx context.Context, params *awsapigw.CreateRestApiInput, _ ...func(*awsapigw.Options)) (*awsapigw.CreateRestApiOutput, error) {
	f.createCalled = true
	if f.createRestApi == nil {
		return nil, fmt.Errorf("unexpected call to CreateRestApi")
	}
	return f.createRestApi(ctx, params)
}
func (f *fakeRestAPI) UpdateRestApi(context.Context, *awsapigw.UpdateRestApiInput, ...func(*awsapigw.Options)) (*awsapigw.UpdateRestApiOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateRestApi")
}
func (f *fakeRestAPI) PutRestApi(ctx context.Context, params *awsapigw.PutRestApiInput, _ ...func(*awsapigw.Options)) (*awsapigw.PutRestApiOutput, error) {
	f.putCalled = true
	if f.putRestApi == nil {
		return nil, fmt.Errorf("unexpected call to PutRestApi")
	}
	return f.putRestApi(ctx, params)
}
func (f *fakeRestAPI) DeleteRestApi(_ context.Context, params *awsapigw.DeleteRestApiInput, _ ...func(*awsapigw.Options)) (*awsapigw.DeleteRestApiOutput, error) {
	f.deleteCalled = true
	f.deletedAPIID = aws.ToString(params.RestApiId)
	return &awsapigw.DeleteRestApiOutput{}, nil
}
func (f *fakeRestAPI) GetResources(context.Context, *awsapigw.GetResourcesInput, ...func(*awsapigw.Options)) (*awsapigw.GetResourcesOutput, error) {
	return &awsapigw.GetResourcesOutput{}, nil
}

func restAPICR(mutate ...func(*awsv1alpha1.RestAPI)) *awsv1alpha1.RestAPI {
	a := &awsv1alpha1.RestAPI{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rest-api", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.RestAPISpec{
			Name:          "my-rest-api",
			EndpointTypes: []string{"REGIONAL"},
		},
	}
	for _, m := range mutate {
		m(a)
	}
	return a
}

func TestRestAPILifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-rest-api", Namespace: "default"}}

	t.Run("create persists api id", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, restAPICR())
		fake := &fakeRestAPI{
			createRestApi: func(_ context.Context, params *awsapigw.CreateRestApiInput) (*awsapigw.CreateRestApiOutput, error) {
				if params.EndpointConfiguration == nil || len(params.EndpointConfiguration.Types) != 1 {
					return nil, fmt.Errorf("expected endpoint configuration")
				}
				return &awsapigw.CreateRestApiOutput{
					Id:             aws.String("rest1"),
					RootResourceId: aws.String("root1"),
				}, nil
			},
		}
		r := &RestAPIReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.RestAPI{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.APIID != "rest1" || got.Status.RootResourceID != "root1" {
			t.Errorf("status = %+v", got.Status)
		}
		if fake.putCalled {
			t.Error("PutRestApi must not be called without a body")
		}
	})

	t.Run("create with body imports OpenAPI definition", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restAPICR(func(a *awsv1alpha1.RestAPI) {
			a.Spec.Body = `{"openapi":"3.0.1"}`
		})
		c := newAPIGWFakeClient(scheme, obj)
		fake := &fakeRestAPI{
			createRestApi: func(_ context.Context, _ *awsapigw.CreateRestApiInput) (*awsapigw.CreateRestApiOutput, error) {
				return &awsapigw.CreateRestApiOutput{Id: aws.String("rest1")}, nil
			},
			putRestApi: func(_ context.Context, params *awsapigw.PutRestApiInput) (*awsapigw.PutRestApiOutput, error) {
				if string(params.Body) != `{"openapi":"3.0.1"}` {
					return nil, fmt.Errorf("unexpected body %q", string(params.Body))
				}
				return &awsapigw.PutRestApiOutput{}, nil
			},
		}
		r := &RestAPIReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.putCalled {
			t.Error("expected PutRestApi to import the declared body")
		}
	})

	t.Run("identifier persisted even when body import fails", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restAPICR(func(a *awsv1alpha1.RestAPI) {
			a.Spec.Body = `{"openapi":"3.0.1"}`
		})
		c := newAPIGWFakeClient(scheme, obj)
		fake := &fakeRestAPI{
			createRestApi: func(_ context.Context, _ *awsapigw.CreateRestApiInput) (*awsapigw.CreateRestApiOutput, error) {
				return &awsapigw.CreateRestApiOutput{Id: aws.String("rest1")}, nil
			},
			putRestApi: func(_ context.Context, _ *awsapigw.PutRestApiInput) (*awsapigw.PutRestApiOutput, error) {
				return nil, fmt.Errorf("bad definition")
			},
		}
		r := &RestAPIReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err == nil {
			t.Fatal("expected error, got nil")
		}
		got := &awsv1alpha1.RestAPI{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.APIID != "rest1" {
			t.Errorf("status.apiId = %q, want rest1 (identifier must be persisted before failing step)", got.Status.APIID)
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restAPICR(func(a *awsv1alpha1.RestAPI) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restAPICR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestAPI{}
		r := &RestAPIReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled || fake.deletedAPIID != "rest1" {
			t.Errorf("expected DeleteRestApi(rest1), got called=%v id=%q", fake.deleteCalled, fake.deletedAPIID)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restAPICR(func(a *awsv1alpha1.RestAPI) {
			a.Finalizers = []string{awsv1alpha1.FinalizerName}
			a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			a.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restAPICR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestAPI{}
		r := &RestAPIReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteRestApi must not be called when abandoning")
		}
	})
}

// ---- RestAPIDeployment ----

type fakeRestDeployment struct {
	createDeployment func(ctx context.Context, params *awsapigw.CreateDeploymentInput) (*awsapigw.CreateDeploymentOutput, error)
	getDeployment    func(ctx context.Context, params *awsapigw.GetDeploymentInput) (*awsapigw.GetDeploymentOutput, error)
	createCalled     bool
	deleteCalled     bool
}

func (f *fakeRestDeployment) GetDeployment(ctx context.Context, params *awsapigw.GetDeploymentInput, _ ...func(*awsapigw.Options)) (*awsapigw.GetDeploymentOutput, error) {
	if f.getDeployment == nil {
		return nil, fmt.Errorf("unexpected call to GetDeployment")
	}
	return f.getDeployment(ctx, params)
}
func (f *fakeRestDeployment) CreateDeployment(ctx context.Context, params *awsapigw.CreateDeploymentInput, _ ...func(*awsapigw.Options)) (*awsapigw.CreateDeploymentOutput, error) {
	f.createCalled = true
	if f.createDeployment == nil {
		return nil, fmt.Errorf("unexpected call to CreateDeployment")
	}
	return f.createDeployment(ctx, params)
}
func (f *fakeRestDeployment) DeleteDeployment(context.Context, *awsapigw.DeleteDeploymentInput, ...func(*awsapigw.Options)) (*awsapigw.DeleteDeploymentOutput, error) {
	f.deleteCalled = true
	return &awsapigw.DeleteDeploymentOutput{}, nil
}

func restDeploymentCR(mutate ...func(*awsv1alpha1.RestAPIDeployment)) *awsv1alpha1.RestAPIDeployment {
	d := &awsv1alpha1.RestAPIDeployment{
		ObjectMeta: metav1.ObjectMeta{Name: "my-deployment", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.RestAPIDeploymentSpec{
			RestAPIRef: awsv1alpha1.APIRef{Name: "my-rest-api"},
			StageName:  "prod",
		},
	}
	for _, m := range mutate {
		m(d)
	}
	return d
}

func readyRestAPICR() *awsv1alpha1.RestAPI {
	return restAPICR(func(a *awsv1alpha1.RestAPI) {
		a.Status.APIID = "rest1"
	})
}

func TestRestAPIDeploymentLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-deployment", Namespace: "default"}}

	t.Run("create persists deployment id", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, restDeploymentCR(), readyRestAPICR())
		fake := &fakeRestDeployment{
			createDeployment: func(_ context.Context, params *awsapigw.CreateDeploymentInput) (*awsapigw.CreateDeploymentOutput, error) {
				if aws.ToString(params.RestApiId) != "rest1" {
					return nil, fmt.Errorf("unexpected api id %q", aws.ToString(params.RestApiId))
				}
				return &awsapigw.CreateDeploymentOutput{Id: aws.String("dep1")}, nil
			},
		}
		r := &RestAPIDeploymentReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.RestAPIDeployment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.DeploymentID != "dep1" || got.Status.APIID != "rest1" {
			t.Errorf("status = %+v", got.Status)
		}
	})

	t.Run("spec change sets UpdateNotSupported without bumping observedGeneration", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restDeploymentCR(func(d *awsv1alpha1.RestAPIDeployment) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Generation = 2
			d.Status.DeploymentID = "dep1"
			d.Status.APIID = "rest1"
			d.Status.ObservedGeneration = 1
		})
		c := newAPIGWFakeClient(scheme, obj, readyRestAPICR())
		fake := &fakeRestDeployment{
			getDeployment: func(_ context.Context, _ *awsapigw.GetDeploymentInput) (*awsapigw.GetDeploymentOutput, error) {
				return &awsapigw.GetDeploymentOutput{Id: aws.String("dep1")}, nil
			},
		}
		r := &RestAPIDeploymentReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.createCalled {
			t.Error("CreateDeployment must not be called on spec change")
		}
		got := &awsv1alpha1.RestAPIDeployment{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ObservedGeneration != 1 {
			t.Errorf("observedGeneration = %d, must stay 1", got.Status.ObservedGeneration)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != awsv1alpha1.ReasonUpdateNotSupported {
			t.Errorf("Ready condition = %+v, want False/UpdateNotSupported", cond)
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restDeploymentCR(func(d *awsv1alpha1.RestAPIDeployment) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Status.DeploymentID = "dep1"
			d.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restDeploymentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestDeployment{}
		r := &RestAPIDeploymentReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteDeployment to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restDeploymentCR(func(d *awsv1alpha1.RestAPIDeployment) {
			d.Finalizers = []string{awsv1alpha1.FinalizerName}
			d.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			d.Status.DeploymentID = "dep1"
			d.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restDeploymentCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestDeployment{}
		r := &RestAPIDeploymentReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteDeployment must not be called when abandoning")
		}
	})
}

// ---- RestAPIStage ----

type fakeRestStage struct {
	createStage  func(ctx context.Context, params *awsapigw.CreateStageInput) (*awsapigw.CreateStageOutput, error)
	createCalled bool
	deleteCalled bool
}

func (f *fakeRestStage) GetStage(context.Context, *awsapigw.GetStageInput, ...func(*awsapigw.Options)) (*awsapigw.GetStageOutput, error) {
	return nil, apigwNotFoundErr()
}
func (f *fakeRestStage) CreateStage(ctx context.Context, params *awsapigw.CreateStageInput, _ ...func(*awsapigw.Options)) (*awsapigw.CreateStageOutput, error) {
	f.createCalled = true
	if f.createStage == nil {
		return nil, fmt.Errorf("unexpected call to CreateStage")
	}
	return f.createStage(ctx, params)
}
func (f *fakeRestStage) UpdateStage(context.Context, *awsapigw.UpdateStageInput, ...func(*awsapigw.Options)) (*awsapigw.UpdateStageOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateStage")
}
func (f *fakeRestStage) DeleteStage(context.Context, *awsapigw.DeleteStageInput, ...func(*awsapigw.Options)) (*awsapigw.DeleteStageOutput, error) {
	f.deleteCalled = true
	return &awsapigw.DeleteStageOutput{}, nil
}

func restStageCR(mutate ...func(*awsv1alpha1.RestAPIStage)) *awsv1alpha1.RestAPIStage {
	s := &awsv1alpha1.RestAPIStage{
		ObjectMeta: metav1.ObjectMeta{Name: "my-rest-stage", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.RestAPIStageSpec{
			RestAPIRef:    awsv1alpha1.APIRef{Name: "my-rest-api"},
			StageName:     "prod",
			DeploymentRef: awsv1alpha1.RestAPIDeploymentRef{Name: "my-deployment"},
		},
	}
	for _, m := range mutate {
		m(s)
	}
	return s
}

func TestRestAPIStageLifecycle(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-rest-stage", Namespace: "default"}}
	readyDeployment := restDeploymentCR(func(d *awsv1alpha1.RestAPIDeployment) {
		d.Status.DeploymentID = "dep1"
	})

	t.Run("create resolves refs and persists identifiers", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		c := newAPIGWFakeClient(scheme, restStageCR(), readyRestAPICR(), readyDeployment)
		fake := &fakeRestStage{
			createStage: func(_ context.Context, params *awsapigw.CreateStageInput) (*awsapigw.CreateStageOutput, error) {
				if aws.ToString(params.RestApiId) != "rest1" || aws.ToString(params.DeploymentId) != "dep1" {
					return nil, fmt.Errorf("unexpected api/deployment %q/%q", aws.ToString(params.RestApiId), aws.ToString(params.DeploymentId))
				}
				return &awsapigw.CreateStageOutput{StageName: params.StageName}, nil
			},
		}
		r := &RestAPIStageReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.RestAPIStage{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.StageName != "prod" || got.Status.APIID != "rest1" {
			t.Errorf("status = %+v", got.Status)
		}
	})

	t.Run("deployment not ready requeues", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		pendingDeployment := restDeploymentCR()
		c := newAPIGWFakeClient(scheme, restStageCR(), readyRestAPICR(), pendingDeployment)
		fake := &fakeRestStage{}
		r := &RestAPIStageReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if fake.createCalled {
			t.Error("CreateStage must not be called while deployment is not ready")
		}
	})

	t.Run("delete calls AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restStageCR(func(s *awsv1alpha1.RestAPIStage) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Status.StageName = "prod"
			s.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restStageCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestStage{}
		r := &RestAPIStageReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !fake.deleteCalled {
			t.Error("expected DeleteStage to be called")
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newAPIGWScheme(t)
		obj := restStageCR(func(s *awsv1alpha1.RestAPIStage) {
			s.Finalizers = []string{awsv1alpha1.FinalizerName}
			s.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			s.Status.StageName = "prod"
			s.Status.APIID = "rest1"
		})
		c := newAPIGWFakeClient(scheme, obj)
		if err := c.Delete(ctx, restStageCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		fake := &fakeRestStage{}
		r := &RestAPIStageReconciler{Client: c, Scheme: scheme, APIGatewayClient: fake}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if fake.deleteCalled {
			t.Error("DeleteStage must not be called when abandoning")
		}
	})
}
