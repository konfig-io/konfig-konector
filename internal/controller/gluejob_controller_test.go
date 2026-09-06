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
	awsglue "github.com/aws/aws-sdk-go-v2/service/glue"
	gluetypes "github.com/aws/aws-sdk-go-v2/service/glue/types"
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

type fakeGlueJobAPI struct {
	getJob    func(ctx context.Context, params *awsglue.GetJobInput) (*awsglue.GetJobOutput, error)
	createJob func(ctx context.Context, params *awsglue.CreateJobInput) (*awsglue.CreateJobOutput, error)
	updateJob func(ctx context.Context, params *awsglue.UpdateJobInput) (*awsglue.UpdateJobOutput, error)
	deleteJob func(ctx context.Context, params *awsglue.DeleteJobInput) (*awsglue.DeleteJobOutput, error)

	createCalled bool
	updateCalled bool
	deleteCalled bool
	createInput  *awsglue.CreateJobInput
	updateInput  *awsglue.UpdateJobInput
	deletedName  string
}

func (f *fakeGlueJobAPI) GetJob(ctx context.Context, params *awsglue.GetJobInput, _ ...func(*awsglue.Options)) (*awsglue.GetJobOutput, error) {
	if f.getJob == nil {
		return nil, fmt.Errorf("unexpected call to GetJob")
	}
	return f.getJob(ctx, params)
}

func (f *fakeGlueJobAPI) CreateJob(ctx context.Context, params *awsglue.CreateJobInput, _ ...func(*awsglue.Options)) (*awsglue.CreateJobOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createJob == nil {
		return nil, fmt.Errorf("unexpected call to CreateJob")
	}
	return f.createJob(ctx, params)
}

func (f *fakeGlueJobAPI) UpdateJob(ctx context.Context, params *awsglue.UpdateJobInput, _ ...func(*awsglue.Options)) (*awsglue.UpdateJobOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	if f.updateJob == nil {
		return nil, fmt.Errorf("unexpected call to UpdateJob")
	}
	return f.updateJob(ctx, params)
}

func (f *fakeGlueJobAPI) DeleteJob(ctx context.Context, params *awsglue.DeleteJobInput, _ ...func(*awsglue.Options)) (*awsglue.DeleteJobOutput, error) {
	f.deleteCalled = true
	f.deletedName = aws.ToString(params.JobName)
	if f.deleteJob == nil {
		return nil, fmt.Errorf("unexpected call to DeleteJob")
	}
	return f.deleteJob(ctx, params)
}

func glueNotFoundErr() error {
	return &gluetypes.EntityNotFoundException{Message: aws.String("entity not found")}
}

const testGlueRoleARN = "arn:aws:iam::123456789012:role/glue-role"

func glueJobScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	return scheme
}

func glueJobCR(mutate ...func(*awsv1alpha1.GlueJob)) *awsv1alpha1.GlueJob {
	j := &awsv1alpha1.GlueJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "my-job",
			Namespace:  "default",
			Finalizers: []string{awsv1alpha1.FinalizerName},
		},
		Spec: awsv1alpha1.GlueJobSpec{
			Name:    "my-job",
			RoleRef: awsv1alpha1.RoleRef{ARN: testGlueRoleARN},
			Command: awsv1alpha1.GlueJobCommand{
				Name:           "glueetl",
				ScriptLocation: "s3://scripts/etl.py",
				PythonVersion:  "3",
			},
			DefaultArguments: map[string]string{"--job-language": "python"},
			MaxRetries:       2,
			GlueVersion:      "4.0",
			NumberOfWorkers:  aws.Int32(4),
			WorkerType:       "G.1X",
			Tags:             map[string]string{"env": "test"},
		},
	}
	for _, m := range mutate {
		m(j)
	}
	return j
}

func TestGlueJobReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-job", Namespace: "default"}}

	tests := []struct {
		name    string
		objs    []client.Object
		fake    *fakeGlueJobAPI
		setup   func(t *testing.T, ctx context.Context, c client.Client)
		wantErr bool
		assert  func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, res ctrl.Result)
	}{
		{
			name: "create happy path persists identifier and Ready",
			objs: []client.Object{glueJobCR()},
			fake: &fakeGlueJobAPI{
				getJob: func(_ context.Context, _ *awsglue.GetJobInput) (*awsglue.GetJobOutput, error) {
					return nil, glueNotFoundErr()
				},
				createJob: func(_ context.Context, params *awsglue.CreateJobInput) (*awsglue.CreateJobOutput, error) {
					if aws.ToString(params.Name) != "my-job" {
						return nil, fmt.Errorf("unexpected job name %q", aws.ToString(params.Name))
					}
					if aws.ToString(params.Role) != testGlueRoleARN {
						return nil, fmt.Errorf("unexpected role %q", aws.ToString(params.Role))
					}
					if params.Command == nil || aws.ToString(params.Command.ScriptLocation) != "s3://scripts/etl.py" {
						return nil, fmt.Errorf("unexpected command %+v", params.Command)
					}
					return &awsglue.CreateJobOutput{Name: params.Name}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if !f.createCalled {
					t.Error("expected CreateJob to be called")
				}
				if f.createInput.WorkerType != gluetypes.WorkerTypeG1x {
					t.Errorf("worker type = %q, want G.1X", f.createInput.WorkerType)
				}
				if aws.ToInt32(f.createInput.NumberOfWorkers) != 4 {
					t.Errorf("numberOfWorkers = %d, want 4", aws.ToInt32(f.createInput.NumberOfWorkers))
				}
				got := &awsv1alpha1.GlueJob{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.JobName != "my-job" {
					t.Errorf("status.jobName = %q, want my-job", got.Status.JobName)
				}
				cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
				if cond == nil || cond.Status != metav1.ConditionTrue {
					t.Errorf("Ready condition = %+v, want True", cond)
				}
			},
		},
		{
			name: "steady state does not create or update",
			objs: []client.Object{glueJobCR(func(j *awsv1alpha1.GlueJob) {
				j.Finalizers = []string{awsv1alpha1.FinalizerName}
				j.Generation = 1
				j.Status.JobName = "my-job"
				j.Status.ObservedGeneration = 1
			})},
			fake: &fakeGlueJobAPI{
				getJob: func(_ context.Context, _ *awsglue.GetJobInput) (*awsglue.GetJobOutput, error) {
					return &awsglue.GetJobOutput{Job: &gluetypes.Job{Name: aws.String("my-job")}}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if f.createCalled {
					t.Error("CreateJob must not be called in steady state")
				}
				if f.updateCalled {
					t.Error("UpdateJob must not be called when generation is unchanged")
				}
				got := &awsv1alpha1.GlueJob{}
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
			name: "spec change triggers UpdateJob",
			objs: []client.Object{glueJobCR(func(j *awsv1alpha1.GlueJob) {
				j.Finalizers = []string{awsv1alpha1.FinalizerName}
				j.Generation = 2
				j.Status.JobName = "my-job"
				j.Status.ObservedGeneration = 1
			})},
			fake: &fakeGlueJobAPI{
				getJob: func(_ context.Context, _ *awsglue.GetJobInput) (*awsglue.GetJobOutput, error) {
					return &awsglue.GetJobOutput{Job: &gluetypes.Job{Name: aws.String("my-job")}}, nil
				},
				updateJob: func(_ context.Context, params *awsglue.UpdateJobInput) (*awsglue.UpdateJobOutput, error) {
					if aws.ToString(params.JobName) != "my-job" {
						return nil, fmt.Errorf("unexpected job name")
					}
					if params.JobUpdate == nil || aws.ToString(params.JobUpdate.Role) != testGlueRoleARN {
						return nil, fmt.Errorf("job update missing role")
					}
					return &awsglue.UpdateJobOutput{}, nil
				},
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if !f.updateCalled {
					t.Error("expected UpdateJob to be called on generation change")
				}
				got := &awsv1alpha1.GlueJob{}
				if err := c.Get(ctx, req.NamespacedName, got); err != nil {
					t.Fatalf("get: %v", err)
				}
				if got.Status.ObservedGeneration != 2 {
					t.Errorf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
				}
			},
		},
		{
			name: "role ref not ready requeues without AWS calls",
			objs: []client.Object{
				glueJobCR(func(j *awsv1alpha1.GlueJob) {
					j.Spec.RoleRef = awsv1alpha1.RoleRef{Name: "my-role"}
				}),
				&awsv1alpha1.IAMRole{
					ObjectMeta: metav1.ObjectMeta{Name: "my-role", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
					Spec:       awsv1alpha1.IAMRoleSpec{RoleName: "my-role"},
				},
			},
			fake: &fakeGlueJobAPI{},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, res ctrl.Result) {
				if res != requeueDependency {
					t.Errorf("result = %+v, want requeueDependency %+v", res, requeueDependency)
				}
				if f.createCalled {
					t.Error("CreateJob must not be called while role dependency is not ready")
				}
			},
		},
		{
			name: "delete with finalizer calls AWS delete",
			objs: []client.Object{glueJobCR(func(j *awsv1alpha1.GlueJob) {
				j.Finalizers = []string{awsv1alpha1.FinalizerName}
				j.Status.JobName = "my-job"
			})},
			fake: &fakeGlueJobAPI{
				deleteJob: func(_ context.Context, _ *awsglue.DeleteJobInput) (*awsglue.DeleteJobOutput, error) {
					return &awsglue.DeleteJobOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, glueJobCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteJob to be called")
				}
				if f.deletedName != "my-job" {
					t.Errorf("DeleteJob name = %q, want my-job", f.deletedName)
				}
				got := &awsv1alpha1.GlueJob{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
		{
			name: "delete falls back to spec name when status empty",
			objs: []client.Object{glueJobCR(func(j *awsv1alpha1.GlueJob) {
				j.Finalizers = []string{awsv1alpha1.FinalizerName}
			})},
			fake: &fakeGlueJobAPI{
				deleteJob: func(_ context.Context, _ *awsglue.DeleteJobInput) (*awsglue.DeleteJobOutput, error) {
					return &awsglue.DeleteJobOutput{}, nil
				},
			},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, glueJobCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if !f.deleteCalled {
					t.Error("expected DeleteJob to be called via spec-based fallback")
				}
				if f.deletedName != "my-job" {
					t.Errorf("DeleteJob name = %q, want my-job", f.deletedName)
				}
			},
		},
		{
			name: "abandon policy skips AWS delete",
			objs: []client.Object{glueJobCR(func(j *awsv1alpha1.GlueJob) {
				j.Finalizers = []string{awsv1alpha1.FinalizerName}
				j.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				j.Status.JobName = "my-job"
			})},
			fake: &fakeGlueJobAPI{},
			setup: func(t *testing.T, ctx context.Context, c client.Client) {
				if err := c.Delete(ctx, glueJobCR()); err != nil {
					t.Fatalf("delete: %v", err)
				}
			},
			assert: func(t *testing.T, ctx context.Context, c client.Client, f *fakeGlueJobAPI, _ ctrl.Result) {
				if f.deleteCalled {
					t.Error("DeleteJob must not be called when abandoning")
				}
				got := &awsv1alpha1.GlueJob{}
				if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
					t.Errorf("expected CR gone after finalizer removal, got err=%v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := glueJobScheme(t)
			c := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.GlueJob{}).
				WithObjects(tc.objs...).
				Build()
			r := &GlueJobReconciler{Client: c, Scheme: scheme, GlueClient: tc.fake}

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

// smoke check that the fake glue error is recognised by the helper via smithy
// as well as the typed path.
func TestGlueNotFoundGeneric(t *testing.T) {
	err := &smithy.GenericAPIError{Code: "EntityNotFoundException", Message: "nope"}
	f := &fakeGlueJobAPI{
		getJob: func(_ context.Context, _ *awsglue.GetJobInput) (*awsglue.GetJobOutput, error) {
			return nil, err
		},
		createJob: func(_ context.Context, params *awsglue.CreateJobInput) (*awsglue.CreateJobOutput, error) {
			return &awsglue.CreateJobOutput{Name: params.Name}, nil
		},
	}
	scheme := glueJobScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&awsv1alpha1.GlueJob{}).
		WithObjects(glueJobCR()).
		Build()
	r := &GlueJobReconciler{Client: c, Scheme: scheme, GlueClient: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-job", Namespace: "default"}}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !f.createCalled {
		t.Error("expected CreateJob after generic not-found error")
	}
}
