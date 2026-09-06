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
	awscc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cctypes "github.com/aws/aws-sdk-go-v2/service/cloudcontrol/types"
	"github.com/aws/smithy-go"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeCC struct {
	live        map[string]string // identifier -> properties JSON
	status      cctypes.OperationStatus
	created     int
	updated     int
	deleted     int
	lastPatch   string
	createToken string
	errorCode   cctypes.HandlerErrorCode
}

func (f *fakeCC) CreateResource(_ context.Context, in *awscc.CreateResourceInput, _ ...func(*awscc.Options)) (*awscc.CreateResourceOutput, error) {
	f.created++
	f.createToken = aws.ToString(in.ClientToken)
	if f.status == cctypes.OperationStatusSuccess {
		f.live["id-1"] = aws.ToString(in.DesiredState)
	}
	return &awscc.CreateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{
		RequestToken: aws.String("req-1"), OperationStatus: f.status, Identifier: aws.String("id-1"),
	}}, nil
}
func (f *fakeCC) GetResource(_ context.Context, in *awscc.GetResourceInput, _ ...func(*awscc.Options)) (*awscc.GetResourceOutput, error) {
	props, ok := f.live[aws.ToString(in.Identifier)]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "ResourceNotFoundException"}
	}
	return &awscc.GetResourceOutput{ResourceDescription: &cctypes.ResourceDescription{Identifier: in.Identifier, Properties: aws.String(props)}}, nil
}
func (f *fakeCC) UpdateResource(_ context.Context, in *awscc.UpdateResourceInput, _ ...func(*awscc.Options)) (*awscc.UpdateResourceOutput, error) {
	f.updated++
	f.lastPatch = aws.ToString(in.PatchDocument)
	return &awscc.UpdateResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String("req-2"), OperationStatus: cctypes.OperationStatusInProgress, Identifier: in.Identifier}}, nil
}
func (f *fakeCC) DeleteResource(_ context.Context, in *awscc.DeleteResourceInput, _ ...func(*awscc.Options)) (*awscc.DeleteResourceOutput, error) {
	f.deleted++
	delete(f.live, aws.ToString(in.Identifier))
	return &awscc.DeleteResourceOutput{ProgressEvent: &cctypes.ProgressEvent{RequestToken: aws.String("req-3"), OperationStatus: cctypes.OperationStatusSuccess}}, nil
}
func (f *fakeCC) GetResourceRequestStatus(context.Context, *awscc.GetResourceRequestStatusInput, ...func(*awscc.Options)) (*awscc.GetResourceRequestStatusOutput, error) {
	return &awscc.GetResourceRequestStatusOutput{ProgressEvent: &cctypes.ProgressEvent{OperationStatus: f.status, Identifier: aws.String("id-1"), ErrorCode: f.errorCode}}, nil
}

func ccObj(desired string) *awsv1alpha1.CloudControlResource {
	return &awsv1alpha1.CloudControlResource{
		ObjectMeta: metav1.ObjectMeta{Name: "lg", Namespace: "ns", UID: "uid-123", Generation: 1, Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec:       awsv1alpha1.CloudControlResourceSpec{TypeName: "AWS::Logs::LogGroup", DesiredState: apiextensionsv1.JSON{Raw: []byte(desired)}},
	}
}

func ccClient(t *testing.T, objs ...client.Object) client.Client {
	s := crossAccountScheme(t)
	return fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&awsv1alpha1.CloudControlResource{}).WithObjects(objs...).Build()
}

var ccKey = k8stypes.NamespacedName{Name: "lg", Namespace: "ns"}

func TestCloudControlCreatePollReady(t *testing.T) {
	obj := ccObj(`{"LogGroupName":"app","RetentionInDays":7}`)
	c := ccClient(t, obj)
	f := &fakeCC{live: map[string]string{}, status: cctypes.OperationStatusInProgress}
	r := &CloudControlResourceReconciler{Client: c, Scheme: c.Scheme(), CCClient: f}

	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: ccKey})
	if err != nil || res != requeuePending || f.created != 1 {
		t.Fatalf("expected create + pending, got %+v %v created=%d", res, err, f.created)
	}
	got := &awsv1alpha1.CloudControlResource{}
	_ = c.Get(context.Background(), ccKey, got)
	if got.Status.RequestToken != "req-1" || got.Status.Identifier != "id-1" || got.Status.Operation != "CREATE" {
		t.Fatalf("token/identifier must be persisted right after create: %+v", got.Status)
	}
	if f.createToken == "" {
		t.Fatal("client token must be set for idempotent create")
	}

	// Operation completes; live state matches desired → Ready, no update.
	f.status = cctypes.OperationStatusSuccess
	f.live["id-1"] = `{"LogGroupName":"app","RetentionInDays":7,"Arn":"arn:lg"}`
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: ccKey}); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(context.Background(), ccKey, got)
	if got.Status.RequestToken != "" || f.updated != 0 || got.Status.Properties == nil {
		t.Fatalf("expected token cleared, no update, live properties recorded: %+v upd=%d", got.Status, f.updated)
	}
	if cnd := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cnd == nil || cnd.Status != metav1.ConditionTrue {
		t.Fatalf("expected Ready, got %+v", cnd)
	}
}

func TestCloudControlDriftPatch(t *testing.T) {
	obj := ccObj(`{"LogGroupName":"app","RetentionInDays":30}`)
	obj.Status.Identifier = "id-1"
	c := ccClient(t, obj)
	f := &fakeCC{live: map[string]string{"id-1": `{"LogGroupName":"app","RetentionInDays":7}`}, status: cctypes.OperationStatusSuccess}
	r := &CloudControlResourceReconciler{Client: c, Scheme: c.Scheme(), CCClient: f}
	res, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: ccKey})
	if err != nil || res != requeuePending || f.updated != 1 || f.created != 0 {
		t.Fatalf("expected drift → update, got %+v %v upd=%d cre=%d", res, err, f.updated, f.created)
	}
	if f.lastPatch != `[{"op":"replace","path":"/RetentionInDays","value":30}]` {
		t.Fatalf("unexpected patch %s", f.lastPatch)
	}
}

func TestCloudControlDeleteAndAbandon(t *testing.T) {
	now := metav1.Now()
	obj := ccObj(`{"LogGroupName":"app"}`)
	obj.DeletionTimestamp = &now
	obj.Status.Identifier = "id-1"
	c := ccClient(t, obj)
	f := &fakeCC{live: map[string]string{"id-1": `{}`}, status: cctypes.OperationStatusSuccess}
	r := &CloudControlResourceReconciler{Client: c, Scheme: c.Scheme(), CCClient: f}
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: ccKey}); err != nil {
		t.Fatal(err)
	}
	if f.deleted != 1 || len(f.live) != 0 {
		t.Fatalf("expected delete, got deleted=%d live=%v", f.deleted, f.live)
	}

	ab := ccObj(`{"LogGroupName":"app"}`)
	ab.DeletionTimestamp = &now
	ab.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
	ab.Status.Identifier = "id-1"
	c2 := ccClient(t, ab)
	f2 := &fakeCC{live: map[string]string{"id-1": `{}`}}
	r2 := &CloudControlResourceReconciler{Client: c2, Scheme: c2.Scheme(), CCClient: f2}
	if _, err := r2.Reconcile(context.Background(), ctrl.Request{NamespacedName: ccKey}); err != nil {
		t.Fatal(err)
	}
	if f2.deleted != 0 {
		t.Fatal("abandon must not call DeleteResource")
	}
}
