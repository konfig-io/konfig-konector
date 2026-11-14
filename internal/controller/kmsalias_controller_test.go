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
	awskms "github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

const (
	testAliasName = "alias/example-alias"
	testAliasARN  = "arn:aws:kms:us-east-1:111122223333:alias/example-alias"
	testAliasKey  = "11111111-2222-3333-4444-555555555555"
	otherAliasKey = "99999999-8888-7777-6666-555555555555"
)

// fakeKMSAliasAPI is an in-memory KMS alias store. aliases maps alias name to
// target key ID. hidden simulates ListAliases not yet showing an alias that
// exists (KMS is eventually consistent after CreateAlias).
type fakeKMSAliasAPI struct {
	aliases map[string]string
	hidden  bool

	creates, updates, deletes int
}

func (f *fakeKMSAliasAPI) ListAliases(_ context.Context, in *awskms.ListAliasesInput, _ ...func(*awskms.Options)) (*awskms.ListAliasesOutput, error) {
	// Page size 1 with a filler entry first, so callers must paginate.
	all := []kmstypes.AliasListEntry{{AliasName: aws.String("alias/aws/s3")}}
	if !f.hidden {
		for name, key := range f.aliases {
			if in.KeyId != nil && kmsKeyIDFromRef(aws.ToString(in.KeyId)) != key {
				continue
			}
			all = append(all, kmstypes.AliasListEntry{
				AliasName:   aws.String(name),
				AliasArn:    aws.String("arn:aws:kms:us-east-1:111122223333:" + name),
				TargetKeyId: aws.String(key),
			})
		}
	}
	idx := 0
	if in.Marker != nil {
		fmt.Sscanf(aws.ToString(in.Marker), "%d", &idx)
	}
	out := &awskms.ListAliasesOutput{Aliases: all[idx : idx+1]}
	if idx+1 < len(all) {
		out.Truncated = true
		out.NextMarker = aws.String(fmt.Sprintf("%d", idx+1))
	}
	return out, nil
}

func (f *fakeKMSAliasAPI) CreateAlias(_ context.Context, in *awskms.CreateAliasInput, _ ...func(*awskms.Options)) (*awskms.CreateAliasOutput, error) {
	f.creates++
	if _, ok := f.aliases[aws.ToString(in.AliasName)]; ok {
		return nil, fmt.Errorf("operation error KMS: CreateAlias: %w", &kmstypes.AlreadyExistsException{})
	}
	f.aliases[aws.ToString(in.AliasName)] = kmsKeyIDFromRef(aws.ToString(in.TargetKeyId))
	return &awskms.CreateAliasOutput{}, nil
}

func (f *fakeKMSAliasAPI) UpdateAlias(_ context.Context, in *awskms.UpdateAliasInput, _ ...func(*awskms.Options)) (*awskms.UpdateAliasOutput, error) {
	f.updates++
	f.aliases[aws.ToString(in.AliasName)] = kmsKeyIDFromRef(aws.ToString(in.TargetKeyId))
	return &awskms.UpdateAliasOutput{}, nil
}

func (f *fakeKMSAliasAPI) DeleteAlias(_ context.Context, in *awskms.DeleteAliasInput, _ ...func(*awskms.Options)) (*awskms.DeleteAliasOutput, error) {
	f.deletes++
	delete(f.aliases, aws.ToString(in.AliasName))
	return &awskms.DeleteAliasOutput{}, nil
}

func kmsAliasCR(mutate ...func(*awsv1alpha1.KMSAlias)) *awsv1alpha1.KMSAlias {
	a := &awsv1alpha1.KMSAlias{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default", Finalizers: []string{awsv1alpha1.FinalizerName}},
		Spec: awsv1alpha1.KMSAliasSpec{
			AliasName:    testAliasName,
			TargetKeyRef: awsv1alpha1.KMSKeyRef{KeyID: testAliasKey},
		},
	}
	for _, m := range mutate {
		m(a)
	}
	return a
}

func TestKMSAliasReconcile(t *testing.T) {
	req := ctrl.Request{NamespacedName: k8stypes.NamespacedName{Name: "example", Namespace: "default"}}

	type want struct {
		creates, updates int
		requeueSoon      bool // short dependency-style requeue, no error
		ready            metav1.ConditionStatus
		arn              string
		target           string
	}
	cases := []struct {
		name    string
		cr      *awsv1alpha1.KMSAlias
		aliases map[string]string
		hidden  bool
		want    want
	}{
		{
			name:    "creates a missing alias and records its ARN",
			cr:      kmsAliasCR(),
			aliases: map[string]string{},
			want:    want{creates: 1, ready: metav1.ConditionTrue, arn: testAliasARN, target: testAliasKey},
		},
		{
			name:    "adopts an existing alias on the right key without create or update",
			cr:      kmsAliasCR(),
			aliases: map[string]string{testAliasName: testAliasKey},
			want:    want{ready: metav1.ConditionTrue, arn: testAliasARN, target: testAliasKey},
		},
		{
			name:    "steady state with ARN in status makes no mutating call",
			cr:      kmsAliasCR(func(a *awsv1alpha1.KMSAlias) { a.Status.ARN = testAliasARN }),
			aliases: map[string]string{testAliasName: testAliasKey},
			want:    want{ready: metav1.ConditionTrue, arn: testAliasARN, target: testAliasKey},
		},
		{
			name: "target given as a key ARN matches the listed key ID",
			cr: kmsAliasCR(func(a *awsv1alpha1.KMSAlias) {
				a.Spec.TargetKeyRef.KeyID = "arn:aws:kms:us-east-1:111122223333:key/" + testAliasKey
			}),
			aliases: map[string]string{testAliasName: testAliasKey},
			want:    want{ready: metav1.ConditionTrue, arn: testAliasARN, target: testAliasKey},
		},
		{
			name:    "moves an alias that points at another key",
			cr:      kmsAliasCR(),
			aliases: map[string]string{testAliasName: otherAliasKey},
			want:    want{updates: 1, ready: metav1.ConditionTrue, arn: testAliasARN, target: testAliasKey},
		},
		{
			name:    "AlreadyExists on create is a requeue, not an error",
			cr:      kmsAliasCR(),
			aliases: map[string]string{testAliasName: testAliasKey},
			hidden:  true,
			want:    want{creates: 1, requeueSoon: true, target: testAliasKey},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			scheme := dbInstanceScheme(t)
			f := &fakeKMSAliasAPI{aliases: tc.aliases, hidden: tc.hidden}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.KMSAlias{}).WithObjects(tc.cr).Build()
			r := &KMSAliasReconciler{Client: c, Scheme: scheme, KMSClient: f}

			res, err := r.Reconcile(ctx, req)
			if err != nil {
				t.Fatalf("reconcile: %v", err)
			}
			if f.creates != tc.want.creates || f.updates != tc.want.updates {
				t.Errorf("creates/updates = %d/%d, want %d/%d", f.creates, f.updates, tc.want.creates, tc.want.updates)
			}
			if got := f.aliases[testAliasName]; got != tc.want.target {
				t.Errorf("alias target = %q, want %q", got, tc.want.target)
			}
			got := &awsv1alpha1.KMSAlias{}
			if err := c.Get(ctx, req.NamespacedName, got); err != nil {
				t.Fatalf("get: %v", err)
			}
			cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
			if tc.want.requeueSoon {
				if res.RequeueAfter != requeueDependency.RequeueAfter {
					t.Errorf("RequeueAfter = %v, want short requeue %v", res.RequeueAfter, requeueDependency.RequeueAfter)
				}
				if cond != nil {
					t.Errorf("expected no condition written while waiting, got %+v", cond)
				}
				return
			}
			if cond == nil || cond.Status != tc.want.ready {
				t.Errorf("Ready = %+v, want %s", cond, tc.want.ready)
			}
			if got.Status.ARN != tc.want.arn {
				t.Errorf("status.arn = %q, want %q", got.Status.ARN, tc.want.arn)
			}
		})
	}

	t.Run("create not yet listed is never marked Ready; the next pass adopts it", func(t *testing.T) {
		ctx := context.Background()
		scheme := dbInstanceScheme(t)
		f := &fakeKMSAliasAPI{aliases: map[string]string{}, hidden: true}
		c := fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&awsv1alpha1.KMSAlias{}).WithObjects(kmsAliasCR()).Build()
		r := &KMSAliasReconciler{Client: c, Scheme: scheme, KMSClient: f}

		res, err := r.Reconcile(ctx, req)
		if err != nil {
			t.Fatalf("reconcile 1: %v", err)
		}
		got := &awsv1alpha1.KMSAlias{}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		if f.creates != 1 || res.RequeueAfter != requeueDependency.RequeueAfter {
			t.Fatalf("creates=%d requeue=%v, want 1 create and a short requeue", f.creates, res.RequeueAfter)
		}
		if cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady); cond != nil && cond.Status == metav1.ConditionTrue {
			t.Fatal("alias marked Ready without an ARN")
		}

		// Still not listed: the second create attempt hits AlreadyExists and requeues.
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 2: %v", err)
		}

		f.hidden = false
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("reconcile 3: %v", err)
		}
		if err := c.Get(ctx, req.NamespacedName, got); err != nil {
			t.Fatalf("get: %v", err)
		}
		cond := apimeta.FindStatusCondition(got.Status.Conditions, awsv1alpha1.ConditionReady)
		if cond == nil || cond.Status != metav1.ConditionTrue || got.Status.ARN != testAliasARN {
			t.Errorf("after listing catches up: Ready=%+v arn=%q, want True and %q", cond, got.Status.ARN, testAliasARN)
		}
		if f.updates != 0 {
			t.Errorf("updates = %d, want 0", f.updates)
		}
	})

	t.Run("delete removes the alias; abandon leaves it", func(t *testing.T) {
		for _, abandon := range []bool{false, true} {
			ctx := context.Background()
			scheme := dbInstanceScheme(t)
			now := metav1.Now()
			f := &fakeKMSAliasAPI{aliases: map[string]string{testAliasName: testAliasKey}}
			cr := kmsAliasCR(func(a *awsv1alpha1.KMSAlias) {
				a.DeletionTimestamp = &now
				if abandon {
					a.Annotations = map[string]string{awsv1alpha1.DeletionPolicyAnnotation: awsv1alpha1.DeletionPolicyAbandon}
				}
			})
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&awsv1alpha1.KMSAlias{}).WithObjects(cr).Build()
			r := &KMSAliasReconciler{Client: c, Scheme: scheme, KMSClient: f}
			if _, err := r.Reconcile(ctx, req); err != nil {
				t.Fatalf("reconcile (abandon=%v): %v", abandon, err)
			}
			wantDeletes := 1
			if abandon {
				wantDeletes = 0
			}
			if f.deletes != wantDeletes {
				t.Errorf("abandon=%v: deletes = %d, want %d", abandon, f.deletes, wantDeletes)
			}
		}
	})
}
