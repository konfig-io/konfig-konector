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
	awss3control "github.com/aws/aws-sdk-go-v2/service/s3control"
	s3controltypes "github.com/aws/aws-sdk-go-v2/service/s3control/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeS3Control struct {
	getAP    func(ctx context.Context, params *awss3control.GetAccessPointInput) (*awss3control.GetAccessPointOutput, error)
	createAP func(ctx context.Context, params *awss3control.CreateAccessPointInput) (*awss3control.CreateAccessPointOutput, error)
	deleteAP func(ctx context.Context, params *awss3control.DeleteAccessPointInput) (*awss3control.DeleteAccessPointOutput, error)

	createCalled bool
	createInput  *awss3control.CreateAccessPointInput
	deleteCalled bool
	deleteInput  *awss3control.DeleteAccessPointInput
}

func (f *fakeS3Control) GetAccessPoint(ctx context.Context, params *awss3control.GetAccessPointInput, _ ...func(*awss3control.Options)) (*awss3control.GetAccessPointOutput, error) {
	if f.getAP == nil {
		return nil, fmt.Errorf("unexpected call to GetAccessPoint")
	}
	return f.getAP(ctx, params)
}

func (f *fakeS3Control) CreateAccessPoint(ctx context.Context, params *awss3control.CreateAccessPointInput, _ ...func(*awss3control.Options)) (*awss3control.CreateAccessPointOutput, error) {
	f.createCalled = true
	f.createInput = params
	if f.createAP == nil {
		return nil, fmt.Errorf("unexpected call to CreateAccessPoint")
	}
	return f.createAP(ctx, params)
}

func (f *fakeS3Control) DeleteAccessPoint(ctx context.Context, params *awss3control.DeleteAccessPointInput, _ ...func(*awss3control.Options)) (*awss3control.DeleteAccessPointOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	if f.deleteAP == nil {
		return nil, fmt.Errorf("unexpected call to DeleteAccessPoint")
	}
	return f.deleteAP(ctx, params)
}

func s3controlNotFoundErr() error {
	return &s3controltypes.NotFoundException{Message: aws.String("not found")}
}

func TestS3AccessPointReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-ap", Namespace: "default"}}
	apARN := "arn:aws:s3:us-east-1:123456789012:accesspoint/my-ap"
	apCR := func(mutate ...func(*awsv1alpha1.S3AccessPoint)) *awsv1alpha1.S3AccessPoint {
		ap := &awsv1alpha1.S3AccessPoint{
			ObjectMeta: metav1.ObjectMeta{Name: "my-ap", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Spec: awsv1alpha1.S3AccessPointSpec{
				Name:      "my-ap",
				AccountID: "123456789012",
				BucketRef: awsv1alpha1.S3BucketRef{BucketName: "my-bucket"},
				PublicAccessBlock: &awsv1alpha1.S3AccessPointPublicAccessBlock{
					BlockPublicAcls:       true,
					BlockPublicPolicy:     true,
					IgnorePublicAcls:      true,
					RestrictPublicBuckets: true,
				},
			},
		}
		for _, m := range mutate {
			m(ap)
		}
		return ap
	}

	t.Run("create persists ARN and alias", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, apCR())
		f := &fakeS3Control{
			getAP: func(_ context.Context, _ *awss3control.GetAccessPointInput) (*awss3control.GetAccessPointOutput, error) {
				return nil, s3controlNotFoundErr()
			},
			createAP: func(_ context.Context, params *awss3control.CreateAccessPointInput) (*awss3control.CreateAccessPointOutput, error) {
				if aws.ToString(params.AccountId) != "123456789012" {
					t.Errorf("account ID = %q", aws.ToString(params.AccountId))
				}
				if aws.ToString(params.Bucket) != "my-bucket" {
					t.Errorf("bucket = %q", aws.ToString(params.Bucket))
				}
				if params.PublicAccessBlockConfiguration == nil || !aws.ToBool(params.PublicAccessBlockConfiguration.BlockPublicAcls) {
					t.Error("public access block not passed")
				}
				return &awss3control.CreateAccessPointOutput{
					AccessPointArn: aws.String(apARN),
					Alias:          aws.String("my-ap-abc123-s3alias"),
				}, nil
			},
		}
		r := &S3AccessPointReconciler{Client: c, Scheme: scheme, S3ControlClient: f}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		got := &awsv1alpha1.S3AccessPoint{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ARN != apARN {
			t.Errorf("status.arn = %q", got.Status.ARN)
		}
		if got.Status.Alias != "my-ap-abc123-s3alias" {
			t.Errorf("status.alias = %q", got.Status.Alias)
		}
	})

	t.Run("bucket ref by CR name waits until bucket has ARN", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme,
			apCR(func(ap *awsv1alpha1.S3AccessPoint) {
				ap.Spec.BucketRef = awsv1alpha1.S3BucketRef{Name: "my-bucket-cr"}
			}),
			&awsv1alpha1.S3Bucket{
				ObjectMeta: metav1.ObjectMeta{Name: "my-bucket-cr", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
				Spec:       awsv1alpha1.S3BucketSpec{BucketName: "my-bucket"},
			},
		)
		f := &fakeS3Control{}
		r := &S3AccessPointReconciler{Client: c, Scheme: scheme, S3ControlClient: f}
		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if res != requeueDependency {
			t.Errorf("result = %+v, want requeueDependency", res)
		}
		if f.createCalled {
			t.Error("CreateAccessPoint must not be called while bucket not ready")
		}
	})

	t.Run("delete calls DeleteAccessPoint with account ID", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, apCR(func(ap *awsv1alpha1.S3AccessPoint) {
			ap.Finalizers = []string{awsv1alpha1.FinalizerName}
			ap.Status.ARN = apARN
		}))
		f := &fakeS3Control{
			deleteAP: func(_ context.Context, params *awss3control.DeleteAccessPointInput) (*awss3control.DeleteAccessPointOutput, error) {
				if aws.ToString(params.AccountId) != "123456789012" || aws.ToString(params.Name) != "my-ap" {
					t.Errorf("delete account/name = %q/%q", aws.ToString(params.AccountId), aws.ToString(params.Name))
				}
				return &awss3control.DeleteAccessPointOutput{}, nil
			},
		}
		r := &S3AccessPointReconciler{Client: c, Scheme: scheme, S3ControlClient: f}
		if err := c.Delete(ctx, apCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Error("expected DeleteAccessPoint to be called")
		}
		got := &awsv1alpha1.S3AccessPoint{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected CR gone, got err=%v", err)
		}
	})

	t.Run("abandon skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		scheme := newScalingScheme(t)
		c := newScalingFakeClient(scheme, apCR(func(ap *awsv1alpha1.S3AccessPoint) {
			ap.Finalizers = []string{awsv1alpha1.FinalizerName}
			ap.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
		}))
		f := &fakeS3Control{}
		r := &S3AccessPointReconciler{Client: c, Scheme: scheme, S3ControlClient: f}
		if err := c.Delete(ctx, apCR()); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("DeleteAccessPoint must not be called when abandoning")
		}
	})
}
