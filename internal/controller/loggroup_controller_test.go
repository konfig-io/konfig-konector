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
	awslogs "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeLogs struct {
	groups    map[string]bool
	created   int
	retention int
	deleted   int
}

func (f *fakeLogs) DescribeLogGroups(_ context.Context, in *awslogs.DescribeLogGroupsInput, _ ...func(*awslogs.Options)) (*awslogs.DescribeLogGroupsOutput, error) {
	out := &awslogs.DescribeLogGroupsOutput{}
	for g := range f.groups {
		if len(g) >= len(aws.ToString(in.LogGroupNamePrefix)) && g[:len(aws.ToString(in.LogGroupNamePrefix))] == aws.ToString(in.LogGroupNamePrefix) {
			out.LogGroups = append(out.LogGroups, logstypes.LogGroup{LogGroupName: aws.String(g), Arn: aws.String("arn:" + g)})
		}
	}
	return out, nil
}
func (f *fakeLogs) CreateLogGroup(_ context.Context, in *awslogs.CreateLogGroupInput, _ ...func(*awslogs.Options)) (*awslogs.CreateLogGroupOutput, error) {
	f.created++
	if f.groups[aws.ToString(in.LogGroupName)] {
		return nil, &smithy.GenericAPIError{Code: "ResourceAlreadyExistsException"}
	}
	f.groups[aws.ToString(in.LogGroupName)] = true
	return &awslogs.CreateLogGroupOutput{}, nil
}
func (f *fakeLogs) PutRetentionPolicy(context.Context, *awslogs.PutRetentionPolicyInput, ...func(*awslogs.Options)) (*awslogs.PutRetentionPolicyOutput, error) {
	f.retention++
	return &awslogs.PutRetentionPolicyOutput{}, nil
}
func (f *fakeLogs) DeleteRetentionPolicy(context.Context, *awslogs.DeleteRetentionPolicyInput, ...func(*awslogs.Options)) (*awslogs.DeleteRetentionPolicyOutput, error) {
	return &awslogs.DeleteRetentionPolicyOutput{}, nil
}
func (f *fakeLogs) DeleteLogGroup(_ context.Context, in *awslogs.DeleteLogGroupInput, _ ...func(*awslogs.Options)) (*awslogs.DeleteLogGroupOutput, error) {
	f.deleted++
	delete(f.groups, aws.ToString(in.LogGroupName))
	return &awslogs.DeleteLogGroupOutput{}, nil
}
func (f *fakeLogs) TagResource(context.Context, *awslogs.TagResourceInput, ...func(*awslogs.Options)) (*awslogs.TagResourceOutput, error) {
	return &awslogs.TagResourceOutput{}, nil
}
func (f *fakeLogs) ListTagsForResource(context.Context, *awslogs.ListTagsForResourceInput, ...func(*awslogs.Options)) (*awslogs.ListTagsForResourceOutput, error) {
	return &awslogs.ListTagsForResourceOutput{}, nil
}
func (f *fakeLogs) UntagResource(context.Context, *awslogs.UntagResourceInput, ...func(*awslogs.Options)) (*awslogs.UntagResourceOutput, error) {
	return &awslogs.UntagResourceOutput{}, nil
}
func (f *fakeLogs) AssociateKmsKey(context.Context, *awslogs.AssociateKmsKeyInput, ...func(*awslogs.Options)) (*awslogs.AssociateKmsKeyOutput, error) {
	return &awslogs.AssociateKmsKeyOutput{}, nil
}
func (f *fakeLogs) DisassociateKmsKey(context.Context, *awslogs.DisassociateKmsKeyInput, ...func(*awslogs.Options)) (*awslogs.DisassociateKmsKeyOutput, error) {
	return &awslogs.DisassociateKmsKeyOutput{}, nil
}

func newLogGroupTest(t *testing.T, existing ...string) (*LogGroupReconciler, *fakeLogs, k8stypes.NamespacedName) {
	s := crossAccountScheme(t)
	lg := &awsv1alpha1.LogGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "lg", Namespace: "ns", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.LogGroupSpec{LogGroupName: "/app/api", RetentionInDays: 7},
	}
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.LogGroup{}).WithObjects(lg).Build()
	f := &fakeLogs{groups: map[string]bool{}}
	for _, g := range existing {
		f.groups[g] = true
	}
	return &LogGroupReconciler{Client: c, Scheme: s, LogsClient: f}, f, k8stypes.NamespacedName{Name: "lg", Namespace: "ns"}
}

func TestLogGroupCreatesWhenMissing(t *testing.T) {
	r, f, key := newLogGroupTest(t)
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if f.created != 1 || !f.groups["/app/api"] || f.retention == 0 {
		t.Fatalf("expected create + retention, got created=%d groups=%v retention=%d", f.created, f.groups, f.retention)
	}
}

// A group that already exists (e.g. created by a reconcile whose status
// persist failed, or pre-existing) is adopted, never re-created.
func TestLogGroupAdoptsExisting(t *testing.T) {
	r, f, key := newLogGroupTest(t, "/app/api", "/app/api-other")
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	if f.created != 0 {
		t.Fatalf("must not call CreateLogGroup for an existing group (called %d)", f.created)
	}
	got := &awsv1alpha1.LogGroup{}
	_ = r.Get(context.Background(), key, got)
	if got.Status.ARN != "arn:/app/api" {
		t.Fatalf("expected adopted ARN, got %q", got.Status.ARN)
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready, got %+v", cnd)
	}
}
