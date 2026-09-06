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
	awsmq "github.com/aws/aws-sdk-go-v2/service/mq"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeMQBrokerAPI struct {
	create func(ctx context.Context, params *awsmq.CreateBrokerInput) (*awsmq.CreateBrokerOutput, error)

	createCalled bool
	deleteCalled bool
	deleteInput  *awsmq.DeleteBrokerInput
}

func (f *fakeMQBrokerAPI) CreateBroker(ctx context.Context, params *awsmq.CreateBrokerInput, _ ...func(*awsmq.Options)) (*awsmq.CreateBrokerOutput, error) {
	f.createCalled = true
	if f.create == nil {
		return nil, fmt.Errorf("unexpected call to CreateBroker")
	}
	return f.create(ctx, params)
}

func (f *fakeMQBrokerAPI) DescribeBroker(_ context.Context, _ *awsmq.DescribeBrokerInput, _ ...func(*awsmq.Options)) (*awsmq.DescribeBrokerOutput, error) {
	return nil, fmt.Errorf("unexpected call to DescribeBroker")
}

func (f *fakeMQBrokerAPI) UpdateBroker(_ context.Context, _ *awsmq.UpdateBrokerInput, _ ...func(*awsmq.Options)) (*awsmq.UpdateBrokerOutput, error) {
	return nil, fmt.Errorf("unexpected call to UpdateBroker")
}

func (f *fakeMQBrokerAPI) DeleteBroker(_ context.Context, params *awsmq.DeleteBrokerInput, _ ...func(*awsmq.Options)) (*awsmq.DeleteBrokerOutput, error) {
	f.deleteCalled = true
	f.deleteInput = params
	return &awsmq.DeleteBrokerOutput{}, nil
}

func (f *fakeMQBrokerAPI) ListBrokers(_ context.Context, _ *awsmq.ListBrokersInput, _ ...func(*awsmq.Options)) (*awsmq.ListBrokersOutput, error) {
	return &awsmq.ListBrokersOutput{}, nil
}

func mqBrokerCR(mutate ...func(*awsv1alpha1.MQBroker)) *awsv1alpha1.MQBroker {
	b := &awsv1alpha1.MQBroker{
		ObjectMeta: metav1.ObjectMeta{Name: "my-broker", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.MQBrokerSpec{
			BrokerName:       "my-broker",
			EngineType:       "RABBITMQ",
			EngineVersion:    "3.11.20",
			HostInstanceType: "mq.t3.micro",
			DeploymentMode:   "SINGLE_INSTANCE",
			Users: []awsv1alpha1.MQUser{{
				Username:    "admin",
				PasswordRef: awsv1alpha1.SecretRef{Name: "broker-secret", Key: "password"},
			}},
		},
	}
	for _, m := range mutate {
		m(b)
	}
	return b
}

func TestMQBrokerReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-broker", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}

	t.Run("create resolves password from secret and persists broker ID", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQBrokerAPI{
			create: func(_ context.Context, params *awsmq.CreateBrokerInput) (*awsmq.CreateBrokerOutput, error) {
				if aws.ToString(params.BrokerName) != "my-broker" {
					return nil, fmt.Errorf("unexpected broker name %q", aws.ToString(params.BrokerName))
				}
				if len(params.Users) != 1 || aws.ToString(params.Users[0].Password) != "s3cret-password" {
					return nil, fmt.Errorf("password not resolved from secret")
				}
				return &awsmq.CreateBrokerOutput{
					BrokerId:  aws.String("b-1234"),
					BrokerArn: aws.String("arn:aws:mq:us-east-1:123456789012:broker:my-broker:b-1234"),
				}, nil
			},
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "broker-secret", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
			Data:       map[string][]byte{"password": []byte("s3cret-password")},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQBroker{}).
			WithObjects(mqBrokerCR(), secret).Build()
		r := &MQBrokerReconciler{Client: c, Scheme: scheme, MQClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateBroker to be called")
		}
		if res.RequeueAfter == 0 {
			t.Error("expected polling requeue after create")
		}
		got := &awsv1alpha1.MQBroker{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.BrokerID != "b-1234" {
			t.Errorf("status.brokerId = %q, want b-1234 (must persist right after CreateBroker)", got.Status.BrokerID)
		}
		// Passwords must never end up in status or conditions.
		for _, cond := range got.Status.Conditions {
			if cond.Message == "s3cret-password" {
				t.Error("password leaked into conditions")
			}
		}
	})

	t.Run("delete with finalizer calls DeleteBroker", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQBrokerAPI{}
		now := metav1.Now()
		b := mqBrokerCR(func(b *awsv1alpha1.MQBroker) {
			b.Finalizers = []string{awsv1alpha1.FinalizerName}
			b.DeletionTimestamp = &now
			b.Status.BrokerID = "b-1234"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQBroker{}).
			WithObjects(b).Build()
		r := &MQBrokerReconciler{Client: c, Scheme: scheme, MQClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.deleteCalled {
			t.Fatal("expected DeleteBroker to be called")
		}
		if aws.ToString(f.deleteInput.BrokerId) != "b-1234" {
			t.Errorf("delete broker ID = %q, want b-1234", aws.ToString(f.deleteInput.BrokerId))
		}
		got := &awsv1alpha1.MQBroker{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation skips AWS delete", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQBrokerAPI{}
		now := metav1.Now()
		b := mqBrokerCR(func(b *awsv1alpha1.MQBroker) {
			b.Finalizers = []string{awsv1alpha1.FinalizerName}
			b.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			b.DeletionTimestamp = &now
			b.Status.BrokerID = "b-1234"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQBroker{}).
			WithObjects(b).Build()
		r := &MQBrokerReconciler{Client: c, Scheme: scheme, MQClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.deleteCalled {
			t.Error("expected DeleteBroker NOT to be called for abandoned resource")
		}
	})
}
