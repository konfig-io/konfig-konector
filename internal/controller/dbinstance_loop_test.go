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
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsrds "github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// loopRDS is an in-memory RDS that behaves like the real one for the cases
// behind the v0.11.6 create loop: creates are slow, a second create of the same
// identifier fails with DBInstanceAlreadyExists, and identifiers prefixed
// "broken-" fail every describe with a message that differs on each call (as
// real AWS errors do, because they embed a request ID).
type loopRDS struct {
	mu        sync.Mutex
	instances map[string]bool
	creates   map[string]int
	describes atomic.Int64
}

func (f *loopRDS) DescribeDBInstances(_ context.Context, in *awsrds.DescribeDBInstancesInput, _ ...func(*awsrds.Options)) (*awsrds.DescribeDBInstancesOutput, error) {
	id := aws.ToString(in.DBInstanceIdentifier)
	if len(id) > 7 && id[:7] == "broken-" {
		return nil, fmt.Errorf("AccessDenied, RequestID: %d", f.describes.Add(1))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.instances[id] {
		return nil, &rdstypes.DBInstanceNotFoundFault{}
	}
	return &awsrds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{{
		DBInstanceArn:    aws.String("arn:aws:rds:us-east-1:123456789012:db:" + id),
		DBInstanceStatus: aws.String("creating"),
	}}}, nil
}

func (f *loopRDS) CreateDBInstance(_ context.Context, in *awsrds.CreateDBInstanceInput, _ ...func(*awsrds.Options)) (*awsrds.CreateDBInstanceOutput, error) {
	id := aws.ToString(in.DBInstanceIdentifier)
	time.Sleep(300 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates[id]++
	if f.instances[id] {
		return nil, &rdstypes.DBInstanceAlreadyExistsFault{}
	}
	f.instances[id] = true
	return &awsrds.CreateDBInstanceOutput{DBInstance: &rdstypes.DBInstance{
		DBInstanceArn:    aws.String("arn:aws:rds:us-east-1:123456789012:db:" + id),
		DBInstanceStatus: aws.String("creating"),
	}}, nil
}

func (f *loopRDS) ModifyDBInstance(context.Context, *awsrds.ModifyDBInstanceInput, ...func(*awsrds.Options)) (*awsrds.ModifyDBInstanceOutput, error) {
	return &awsrds.ModifyDBInstanceOutput{}, nil
}

func (f *loopRDS) DeleteDBInstance(context.Context, *awsrds.DeleteDBInstanceInput, ...func(*awsrds.Options)) (*awsrds.DeleteDBInstanceOutput, error) {
	return &awsrds.DeleteDBInstanceOutput{}, nil
}

var _ = Describe("DBInstance controller under a real manager", func() {
	It("creates once and backs off on persistent errors instead of hot-looping", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  scheme.Scheme,
			Metrics: metricsserver.Options{BindAddress: "0"},
		})
		Expect(err).NotTo(HaveOccurred())
		rds := &loopRDS{instances: map[string]bool{}, creates: map[string]int{}}
		Expect((&DBInstanceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), RDSClient: rds}).SetupWithManager(mgr)).To(Succeed())
		go func() {
			defer GinkgoRecover()
			Expect(mgr.Start(ctx)).To(Succeed())
		}()

		Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-creds", Namespace: "default"},
			Data:       map[string][]byte{"password": []byte(testDBPassword)},
		}))).To(Succeed())

		newDB := func(name string) *awsv1alpha1.DBInstance {
			db := dbInstanceCR()
			db.Name, db.Finalizers = name, nil
			db.Spec.DBInstanceIdentifier = name
			return db
		}
		good, broken := newDB("loop-good"), newDB("broken-loop")
		Expect(k8sClient.Create(ctx, good)).To(Succeed())
		Expect(k8sClient.Create(ctx, broken)).To(Succeed())
		DeferCleanup(func() {
			for _, db := range []*awsv1alpha1.DBInstance{good, broken} {
				cur := &awsv1alpha1.DBInstance{}
				if k8sClient.Get(context.Background(), client.ObjectKeyFromObject(db), cur) == nil {
					cur.Finalizers = nil
					_ = k8sClient.Update(context.Background(), cur)
					_ = k8sClient.Delete(context.Background(), cur)
				}
			}
		})

		Eventually(func() string {
			cur := &awsv1alpha1.DBInstance{}
			_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(good), cur)
			return cur.Status.DBInstanceARN
		}, 10*time.Second, 100*time.Millisecond).Should(Equal("arn:aws:rds:us-east-1:123456789012:db:loop-good"))

		// Let any loop run for a while, then count.
		time.Sleep(4 * time.Second)

		rds.mu.Lock()
		creates := rds.creates["loop-good"]
		rds.mu.Unlock()
		Expect(creates).To(Equal(1), "CreateDBInstance must be called exactly once")

		cur := &awsv1alpha1.DBInstance{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(good), cur)).To(Succeed())
		Expect(cur.Status.DBInstanceARN).NotTo(BeEmpty(), "ARN must survive later reconciles")

		// A failing reconcile rewrites its error condition (new request ID) on
		// every attempt. With exponential backoff that is ~10 attempts in the
		// first seconds; a status-echo hot loop makes hundreds.
		Expect(rds.describes.Load()).To(BeNumerically("<", 25), "persistent errors must back off")
	})
})
