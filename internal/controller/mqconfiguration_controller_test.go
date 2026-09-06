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
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

type fakeMQConfigurationAPI struct {
	createCalled bool
	updateCalled bool
	updateInput  *awsmq.UpdateConfigurationInput
}

func (f *fakeMQConfigurationAPI) CreateConfiguration(_ context.Context, params *awsmq.CreateConfigurationInput, _ ...func(*awsmq.Options)) (*awsmq.CreateConfigurationOutput, error) {
	f.createCalled = true
	if aws.ToString(params.Name) != "my-config" {
		return nil, fmt.Errorf("unexpected name %q", aws.ToString(params.Name))
	}
	return &awsmq.CreateConfigurationOutput{
		Id:             aws.String("c-1234"),
		Arn:            aws.String("arn:aws:mq:us-east-1:123456789012:configuration:c-1234"),
		LatestRevision: &mqtypes.ConfigurationRevision{Revision: aws.Int32(1)},
	}, nil
}

func (f *fakeMQConfigurationAPI) UpdateConfiguration(_ context.Context, params *awsmq.UpdateConfigurationInput, _ ...func(*awsmq.Options)) (*awsmq.UpdateConfigurationOutput, error) {
	f.updateCalled = true
	f.updateInput = params
	return &awsmq.UpdateConfigurationOutput{
		LatestRevision: &mqtypes.ConfigurationRevision{Revision: aws.Int32(2)},
	}, nil
}

func mqConfigurationCR(mutate ...func(*awsv1alpha1.MQConfiguration)) *awsv1alpha1.MQConfiguration {
	cfg := &awsv1alpha1.MQConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "my-config", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.MQConfigurationSpec{
			Name:       "my-config",
			EngineType: "ACTIVEMQ",
			Data:       "PGJyb2tlci8+",
		},
	}
	for _, m := range mutate {
		m(cfg)
	}
	return cfg
}

func TestMQConfigurationReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "my-config", Namespace: "default"}}
	scheme := runtime.NewScheme()
	if err := awsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	t.Run("create persists ID and pushes data revision", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQConfigurationAPI{}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQConfiguration{}).
			WithObjects(mqConfigurationCR()).Build()
		r := &MQConfigurationReconciler{Client: c, Scheme: scheme, MQClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if !f.createCalled {
			t.Fatal("expected CreateConfiguration to be called")
		}
		if !f.updateCalled {
			t.Fatal("expected UpdateConfiguration to push spec data after create")
		}
		if aws.ToString(f.updateInput.Data) != "PGJyb2tlci8+" {
			t.Errorf("update data = %q", aws.ToString(f.updateInput.Data))
		}
		got := &awsv1alpha1.MQConfiguration{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status.ConfigurationID != "c-1234" {
			t.Errorf("status.configurationId = %q, want c-1234", got.Status.ConfigurationID)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue {
			t.Errorf("Ready condition = %+v, want True", cond)
		}
	})

	t.Run("delete removes CR without AWS call (no delete API)", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQConfigurationAPI{}
		now := metav1.Now()
		cfg := mqConfigurationCR(func(cfg *awsv1alpha1.MQConfiguration) {
			cfg.Finalizers = []string{awsv1alpha1.FinalizerName}
			cfg.DeletionTimestamp = &now
			cfg.Status.ConfigurationID = "c-1234"
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQConfiguration{}).
			WithObjects(cfg).Build()
		r := &MQConfigurationReconciler{Client: c, Scheme: scheme, MQClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled || f.updateCalled {
			t.Error("expected no AWS calls on delete")
		}
		got := &awsv1alpha1.MQConfiguration{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})

	t.Run("abandon annotation removes finalizer without AWS call", func(t *testing.T) {
		ctx := context.Background()
		f := &fakeMQConfigurationAPI{}
		now := metav1.Now()
		cfg := mqConfigurationCR(func(cfg *awsv1alpha1.MQConfiguration) {
			cfg.Finalizers = []string{awsv1alpha1.FinalizerName}
			cfg.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
			cfg.DeletionTimestamp = &now
		})
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.MQConfiguration{}).
			WithObjects(cfg).Build()
		r := &MQConfigurationReconciler{Client: c, Scheme: scheme, MQClient: f}

		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if f.createCalled || f.updateCalled {
			t.Error("expected no AWS calls for abandoned resource")
		}
		got := &awsv1alpha1.MQConfiguration{}
		if err := c.Get(ctx, req.NamespacedName, got); !apierrors.IsNotFound(err) {
			t.Errorf("expected object gone after finalizer removal, got err=%v", err)
		}
	})
}
