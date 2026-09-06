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
	awsamp "github.com/aws/aws-sdk-go-v2/service/amp"
	amptypes "github.com/aws/aws-sdk-go-v2/service/amp/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeAMP struct {
	describeWorkspace    func(ctx context.Context, params *awsamp.DescribeWorkspaceInput) (*awsamp.DescribeWorkspaceOutput, error)
	createWorkspace      func(ctx context.Context, params *awsamp.CreateWorkspaceInput) (*awsamp.CreateWorkspaceOutput, error)
	updateWorkspaceAlias func(ctx context.Context, params *awsamp.UpdateWorkspaceAliasInput) (*awsamp.UpdateWorkspaceAliasOutput, error)
	deleteWorkspace      func(ctx context.Context, params *awsamp.DeleteWorkspaceInput) (*awsamp.DeleteWorkspaceOutput, error)

	createCalled      bool
	updateAliasCalled bool
	deleteCalled      bool
	deletedID         string
}

func (f *fakeAMP) DescribeWorkspace(ctx context.Context, params *awsamp.DescribeWorkspaceInput, _ ...func(*awsamp.Options)) (*awsamp.DescribeWorkspaceOutput, error) {
	if f.describeWorkspace == nil {
		return nil, fmt.Errorf("unexpected call to DescribeWorkspace")
	}
	return f.describeWorkspace(ctx, params)
}

func (f *fakeAMP) CreateWorkspace(ctx context.Context, params *awsamp.CreateWorkspaceInput, _ ...func(*awsamp.Options)) (*awsamp.CreateWorkspaceOutput, error) {
	f.createCalled = true
	if f.createWorkspace == nil {
		return nil, fmt.Errorf("unexpected call to CreateWorkspace")
	}
	return f.createWorkspace(ctx, params)
}

func (f *fakeAMP) UpdateWorkspaceAlias(ctx context.Context, params *awsamp.UpdateWorkspaceAliasInput, _ ...func(*awsamp.Options)) (*awsamp.UpdateWorkspaceAliasOutput, error) {
	f.updateAliasCalled = true
	if f.updateWorkspaceAlias == nil {
		return nil, fmt.Errorf("unexpected call to UpdateWorkspaceAlias")
	}
	return f.updateWorkspaceAlias(ctx, params)
}

func (f *fakeAMP) DeleteWorkspace(ctx context.Context, params *awsamp.DeleteWorkspaceInput, _ ...func(*awsamp.Options)) (*awsamp.DeleteWorkspaceOutput, error) {
	f.deleteCalled = true
	f.deletedID = aws.ToString(params.WorkspaceId)
	if f.deleteWorkspace == nil {
		return nil, fmt.Errorf("unexpected call to DeleteWorkspace")
	}
	return f.deleteWorkspace(ctx, params)
}

// ampNotFound mimics the AMP ResourceNotFoundException smithy error.
type ampNotFound struct{}

func (ampNotFound) Error() string                 { return "ResourceNotFoundException: not found" }
func (ampNotFound) ErrorCode() string             { return "ResourceNotFoundException" }
func (ampNotFound) ErrorMessage() string          { return "not found" }
func (ampNotFound) ErrorFault() smithy.ErrorFault { return smithy.FaultClient }

const (
	testAMPWorkspaceID  = "ws-12345678-abcd-1234-abcd-123456789012"
	testAMPWorkspaceARN = "arn:aws:aps:us-east-1:123456789012:workspace/" + testAMPWorkspaceID
	testAMPEndpoint     = "https://aps-workspaces.us-east-1.amazonaws.com/workspaces/" + testAMPWorkspaceID + "/"
)

func promWorkspaceCR(mutate ...func(*awsv1alpha1.PrometheusWorkspace)) *awsv1alpha1.PrometheusWorkspace {
	ws := &awsv1alpha1.PrometheusWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "metrics",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
			Generation: 1,
		},
		Spec: awsv1alpha1.PrometheusWorkspaceSpec{
			Alias: "metrics",
			Tags:  map[string]string{"team": "devops"},
		},
	}
	for _, m := range mutate {
		m(ws)
	}
	return ws
}

func ampActiveDescribe(alias string) func(ctx context.Context, params *awsamp.DescribeWorkspaceInput) (*awsamp.DescribeWorkspaceOutput, error) {
	return func(_ context.Context, params *awsamp.DescribeWorkspaceInput) (*awsamp.DescribeWorkspaceOutput, error) {
		if aws.ToString(params.WorkspaceId) != testAMPWorkspaceID {
			return nil, fmt.Errorf("unexpected workspace ID %q", aws.ToString(params.WorkspaceId))
		}
		return &awsamp.DescribeWorkspaceOutput{Workspace: &amptypes.WorkspaceDescription{
			WorkspaceId:        aws.String(testAMPWorkspaceID),
			Arn:                aws.String(testAMPWorkspaceARN),
			Alias:              aws.String(alias),
			PrometheusEndpoint: aws.String(testAMPEndpoint),
			Status:             &amptypes.WorkspaceStatus{StatusCode: amptypes.WorkspaceStatusCodeActive},
		}}, nil
	}
}

func TestPrometheusWorkspaceReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "metrics", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeAMP
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, res ctrl.Result)
	}{
		{
			name: "create persists workspace ID and polls while CREATING",
			objs: []client.Object{promWorkspaceCR()},
			fake: &fakeAMP{
				createWorkspace: func(_ context.Context, params *awsamp.CreateWorkspaceInput) (*awsamp.CreateWorkspaceOutput, error) {
					if aws.ToString(params.Alias) != "metrics" {
						return nil, fmt.Errorf("unexpected alias %q", aws.ToString(params.Alias))
					}
					if params.Tags["team"] != "devops" {
						return nil, fmt.Errorf("expected tags to be passed")
					}
					return &awsamp.CreateWorkspaceOutput{
						WorkspaceId: aws.String(testAMPWorkspaceID),
						Arn:         aws.String(testAMPWorkspaceARN),
						Status:      &amptypes.WorkspaceStatus{StatusCode: amptypes.WorkspaceStatusCodeCreating},
					}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, res ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateWorkspace to be called")
				}
				if res.RequeueAfter != requeueDevOpsCostPolling.RequeueAfter {
					t.Errorf("RequeueAfter = %v, want polling interval %v", res.RequeueAfter, requeueDevOpsCostPolling.RequeueAfter)
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.WorkspaceID != testAMPWorkspaceID {
					t.Errorf("status.workspaceId = %q, want %q", got.Status.WorkspaceID, testAMPWorkspaceID)
				}
				if got.Status.ARN != testAMPWorkspaceARN {
					t.Errorf("status.arn = %q, want %q", got.Status.ARN, testAMPWorkspaceARN)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != "Provisioning" {
					t.Errorf("Ready condition = %+v, want False/Provisioning", cond)
				}
			},
		},
		{
			name: "polling while CREATING stays not ready",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Status.WorkspaceID = testAMPWorkspaceID
			})},
			fake: &fakeAMP{
				describeWorkspace: func(_ context.Context, _ *awsamp.DescribeWorkspaceInput) (*awsamp.DescribeWorkspaceOutput, error) {
					return &awsamp.DescribeWorkspaceOutput{Workspace: &amptypes.WorkspaceDescription{
						WorkspaceId: aws.String(testAMPWorkspaceID),
						Arn:         aws.String(testAMPWorkspaceARN),
						Status:      &amptypes.WorkspaceStatus{StatusCode: amptypes.WorkspaceStatusCodeCreating},
					}}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, res ctrl.Result) {
				if f.createCalled {
					t.Error("CreateWorkspace must not be called while polling")
				}
				if res.RequeueAfter != requeueDevOpsCostPolling.RequeueAfter {
					t.Errorf("RequeueAfter = %v, want polling interval", res.RequeueAfter)
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.Status != "CREATING" {
					t.Errorf("status.status = %q, want CREATING", got.Status.Status)
				}
			},
		},
		{
			name: "active workspace becomes Ready with endpoint",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Status.WorkspaceID = testAMPWorkspaceID
				ws.Status.ObservedGeneration = 1
			})},
			fake: &fakeAMP{
				describeWorkspace: ampActiveDescribe("metrics"),
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				if f.createCalled || f.updateAliasCalled {
					t.Error("no mutations expected in steady state")
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.PrometheusEndpoint != testAMPEndpoint {
					t.Errorf("status.prometheusEndpoint = %q, want %q", got.Status.PrometheusEndpoint, testAMPEndpoint)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "alias change triggers UpdateWorkspaceAlias",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Generation = 2
				ws.Spec.Alias = "metrics-renamed"
				ws.Status.WorkspaceID = testAMPWorkspaceID
				ws.Status.ObservedGeneration = 1
			})},
			fake: &fakeAMP{
				describeWorkspace: ampActiveDescribe("metrics"),
				updateWorkspaceAlias: func(_ context.Context, params *awsamp.UpdateWorkspaceAliasInput) (*awsamp.UpdateWorkspaceAliasOutput, error) {
					if aws.ToString(params.Alias) != "metrics-renamed" {
						return nil, fmt.Errorf("unexpected alias %q", aws.ToString(params.Alias))
					}
					return &awsamp.UpdateWorkspaceAliasOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				if !f.updateAliasCalled {
					t.Error("expected UpdateWorkspaceAlias to be called")
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "create failure surfaces error and Ready=False",
			objs: []client.Object{promWorkspaceCR()},
			fake: &fakeAMP{
				createWorkspace: func(_ context.Context, _ *awsamp.CreateWorkspaceInput) (*awsamp.CreateWorkspaceOutput, error) {
					return nil, fmt.Errorf("access denied")
				},
			},
			wantErr: true,
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready condition = %+v, want False", cond)
				}
				if got.Status.WorkspaceID != "" {
					t.Errorf("status.workspaceId = %q, want empty (nothing was created)", got.Status.WorkspaceID)
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete with workspace ID",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Status.WorkspaceID = testAMPWorkspaceID
			})},
			fake: &fakeAMP{
				deleteWorkspace: func(_ context.Context, _ *awsamp.DeleteWorkspaceInput) (*awsamp.DeleteWorkspaceOutput, error) {
					return &awsamp.DeleteWorkspaceOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, promWorkspaceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteWorkspace to be called")
				}
				if f.deletedID != testAMPWorkspaceID {
					t.Errorf("DeleteWorkspace ID = %q, want %q", f.deletedID, testAMPWorkspaceID)
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete without workspace ID skips AWS delete",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeAMP{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, promWorkspaceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteWorkspace must not be called without a workspace ID")
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete tolerates workspace already gone in AWS",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Status.WorkspaceID = testAMPWorkspaceID
			})},
			fake: &fakeAMP{
				deleteWorkspace: func(_ context.Context, _ *awsamp.DeleteWorkspaceInput) (*awsamp.DeleteWorkspaceOutput, error) {
					return nil, ampNotFound{}
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, promWorkspaceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{promWorkspaceCR(func(ws *awsv1alpha1.PrometheusWorkspace) {
				ws.Finalizers = []string{awsv1alpha1.FinalizerName}
				ws.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				ws.Status.WorkspaceID = testAMPWorkspaceID
			})},
			fake: &fakeAMP{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, promWorkspaceCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeAMP, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteWorkspace must not be called when abandoning")
				}
				got := &awsv1alpha1.PrometheusWorkspace{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := newDevOpsCostScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.PrometheusWorkspace{}).
				WithObjects(tc.objs...).
				Build()
			r := &PrometheusWorkspaceReconciler{Client: c, Scheme: scheme, AMPClient: tc.fake}

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
