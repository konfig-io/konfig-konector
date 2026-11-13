/*
Copyright 2024.

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
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"time"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
	"github.com/konfig-io/konfig-konector/internal/aws/provider"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// roleNameFromARN extracts the role name from a full IAM role ARN.
// e.g. "arn:aws:iam::123456789012:role/my-role" → "my-role"
// Falls back to returning the input unchanged if parsing fails.
func roleNameFromARN(arn string) string {
	parts := strings.Split(arn, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return arn
}

// dependencyNotReady is returned by dependent controllers when a referenced
// resource exists in Kubernetes but has not yet been synced to AWS.
// The caller should requeue after a short delay rather than treat it as an error.
type dependencyNotReady struct{ msg string }

func (e *dependencyNotReady) Error() string { return e.msg }

// requeueNotReady returns a Result that retries after 5 seconds without
// incrementing the error counter or logging a stack trace.
var requeueDependency = ctrl.Result{RequeueAfter: 5 * time.Second}

// jittered spreads periodic resyncs over ±10% of the base interval so that
// reconciles don't synchronize into waves against AWS APIs after a restart.
func jittered(d time.Duration) time.Duration {
	f := 0.9 + 0.2*rand.Float64()
	return time.Duration(float64(d) * f)
}

// requeueResult is the standard steady-state result for a successful reconcile.
func requeueResult() ctrl.Result {
	return ctrl.Result{RequeueAfter: jittered(requeueAfter)}
}

// shouldAbandon reports whether the CR is annotated to keep the AWS resource
// on deletion (aws.konfig.io/deletion-policy: abandon).
func shouldAbandon(obj client.Object) bool {
	return obj.GetAnnotations()[awsv1alpha1.DeletionPolicyAnnotation] == awsv1alpha1.DeletionPolicyAbandon
}

// persistStatus writes obj's status subresource, retrying resourceVersion
// conflicts against the latest copy of the object.
//
// A conflict means obj was read before some other write landed. Usually that
// write only touched metadata or spec and obj's status is still the freshest
// view, so it is written as-is. But the reconcile may also have started from a
// stale cache read that predates the operator's own previous status write; a
// wholesale overwrite would then erase AWS identifiers recorded by that write
// (and the next reconcile would try to create the AWS resource again). So on
// conflict, string fields that are empty in obj but set in the latest status
// are carried over before retrying. Identifiers are never blanked by a
// conflicting write; a reconcile that really means to clear one will do so
// again on its next, conflict-free pass.
func persistStatus(ctx context.Context, c client.Client, obj client.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := c.Status().Update(ctx, obj)
		if err == nil || !apierrors.IsConflict(err) {
			return err
		}
		latest := reflect.New(reflect.TypeOf(obj).Elem()).Interface().(client.Object)
		if getErr := c.Get(ctx, client.ObjectKeyFromObject(obj), latest); getErr != nil {
			return getErr
		}
		if mergeErr := keepRecordedStatusStrings(obj, latest); mergeErr != nil {
			return mergeErr
		}
		obj.SetResourceVersion(latest.GetResourceVersion())
		return err
	})
}

// keepRecordedStatusStrings copies into obj.status every non-empty string that
// latest.status holds where obj.status has none, recursing through nested
// objects. Lists (conditions) and non-string scalars are left as obj has them.
func keepRecordedStatusStrings(obj, latest client.Object) error {
	ours, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	theirs, err := runtime.DefaultUnstructuredConverter.ToUnstructured(latest)
	if err != nil {
		return err
	}
	theirStatus, _ := theirs["status"].(map[string]interface{})
	if len(theirStatus) == 0 {
		return nil
	}
	ourStatus, _ := ours["status"].(map[string]interface{})
	if ourStatus == nil {
		ourStatus = map[string]interface{}{}
	}
	if !fillEmptyStrings(ourStatus, theirStatus) {
		return nil
	}
	ours["status"] = ourStatus
	return runtime.DefaultUnstructuredConverter.FromUnstructured(ours, obj)
}

func fillEmptyStrings(dst, src map[string]interface{}) bool {
	changed := false
	for k, sv := range src {
		switch sv := sv.(type) {
		case string:
			if sv == "" {
				continue
			}
			if dv, present := dst[k]; !present || dv == nil || dv == "" {
				dst[k] = sv
				changed = true
			}
		case map[string]interface{}:
			switch dv := dst[k].(type) {
			case map[string]interface{}:
				if fillEmptyStrings(dv, sv) {
					changed = true
				}
			case nil:
				sub := map[string]interface{}{}
				if fillEmptyStrings(sub, sv) {
					dst[k] = sub
					changed = true
				}
			}
		}
	}
	return changed
}

// newKindController starts the builder every kind's controller uses. It
// splits the kind's watch in two so the operator's own writes cannot drive a
// tight reconcile loop:
//
//   - changes a user makes (spec, labels, annotations, deletion) enqueue the
//     object immediately, as before;
//   - every other update (in practice the operator's own status and finalizer
//     writes) enqueues it through the rate limiter. A healthy reconcile resets
//     the limiter, so multi-step convergence that relies on the status echo
//     still follows within milliseconds; a failing reconcile that rewrites its
//     error condition on every attempt now backs off exponentially instead of
//     retrying as fast as the API server can deliver its own write back.
func newKindController(mgr ctrl.Manager, obj client.Object) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(obj, builder.WithPredicates(predicate.Funcs{UpdateFunc: isUserChange})).
		Watches(obj, handler.Funcs{
			UpdateFunc: func(_ context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				if e.ObjectNew == nil || isUserChange(e) {
					return
				}
				q.AddRateLimited(reconcile.Request{NamespacedName: client.ObjectKeyFromObject(e.ObjectNew)})
			},
		})
}

// isUserChange reports whether an update touched anything other than status,
// finalizers and bookkeeping metadata.
func isUserChange(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return true
	}
	return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
		!e.ObjectOld.GetDeletionTimestamp().Equal(e.ObjectNew.GetDeletionTimestamp()) ||
		!reflect.DeepEqual(e.ObjectOld.GetLabels(), e.ObjectNew.GetLabels()) ||
		!reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
}

// sameJSON reports whether two JSON documents are semantically equal
// (ignoring whitespace and key order). Unparseable input compares unequal.
func sameJSON(a, b string) bool {
	var av, bv interface{}
	if json.Unmarshal([]byte(a), &av) != nil || json.Unmarshal([]byte(b), &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// resolveSecretValue reads a sensitive value from a Kubernetes Secret via a
// SecretRef. The value must never be written to CR status, conditions, or logs.
func resolveSecretValue(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.SecretRef) (string, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("get secret %s/%s: %w", namespace, ref.Name, err)
	}
	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s has no key %q", namespace, ref.Name, ref.Key)
	}
	return string(val), nil
}

// resolveSubnetIDs resolves a slice of SubnetRefs to AWS subnet ID strings.
// Uses direct .ID if set, otherwise looks up the Subnet CR's status.
func resolveSubnetIDs(ctx context.Context, c client.Client, namespace string, refs []awsv1alpha1.SubnetRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sn := &awsv1alpha1.Subnet{}
		if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sn); err != nil {
			return nil, err
		}
		if sn.Status.SubnetID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("Subnet %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sn.Status.SubnetID)
	}
	return ids, nil
}

// resolveSGIDs resolves a slice of SecurityGroupRefs to AWS security group ID strings.
func resolveSGIDs(ctx context.Context, c client.Client, namespace string, refs []awsv1alpha1.SecurityGroupRef) ([]string, error) {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID != "" {
			ids = append(ids, ref.ID)
			continue
		}
		sgCR := &awsv1alpha1.SecurityGroup{}
		if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, sgCR); err != nil {
			return nil, err
		}
		if sgCR.Status.GroupID == "" {
			return nil, &dependencyNotReady{msg: fmt.Sprintf("SecurityGroup %s/%s has no ID yet", namespace, ref.Name)}
		}
		ids = append(ids, sgCR.Status.GroupID)
	}
	return ids, nil
}

// resolveIAMRoleARN resolves a RoleRef to an IAM role ARN.
// Uses direct .ARN if set, otherwise looks up the IAMRole CR's status.
func resolveIAMRoleARN(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.RoleRef) (string, error) {
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	ns := ref.Namespace
	if ns == "" {
		ns = namespace
	}
	role := &awsv1alpha1.IAMRole{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: ns}, role); err != nil {
		return "", err
	}
	if role.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMRole %s/%s has no ARN yet", ns, ref.Name)}
	}
	return role.Status.ARN, nil
}

// resolveLambdaFunctionName resolves a LambdaFunctionRef to a Lambda function name or ARN.
func resolveLambdaFunctionName(ctx context.Context, c client.Client, namespace string, directArn string, ref *awsv1alpha1.LambdaFunctionRef) (string, error) {
	if directArn != "" {
		return directArn, nil
	}
	if ref == nil {
		return "", fmt.Errorf("either functionArn or functionRef must be set")
	}
	if ref.FunctionName != "" {
		return ref.FunctionName, nil
	}
	fn := &awsv1alpha1.LambdaFunction{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, fn); err != nil {
		return "", err
	}
	if fn.Status.FunctionARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("LambdaFunction %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return fn.Status.FunctionARN, nil
}

// resolveECSClusterName resolves an ECSClusterRef to an AWS ECS cluster name or ARN.
func resolveECSClusterName(ctx context.Context, c client.Client, namespace string, directName string, ref *awsv1alpha1.ECSClusterRef) (string, error) {
	if directName != "" {
		return directName, nil
	}
	if ref == nil {
		return "", fmt.Errorf("either clusterName or clusterRef must be set")
	}
	if ref.ClusterName != "" {
		return ref.ClusterName, nil
	}
	clusterCR := &awsv1alpha1.ECSCluster{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, clusterCR); err != nil {
		return "", err
	}
	if clusterCR.Status.ClusterARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ECSCluster %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return clusterCR.Status.ClusterARN, nil
}

// resolveECSTaskDefinitionArn resolves an ECSTaskDefinitionRef to a task definition ARN.
func resolveECSTaskDefinitionArn(ctx context.Context, c client.Client, namespace string, directArn string, ref *awsv1alpha1.ECSTaskDefinitionRef) (string, error) {
	if directArn != "" {
		return directArn, nil
	}
	if ref == nil {
		return "", fmt.Errorf("either taskDefinitionArn or taskDefinitionRef must be set")
	}
	if ref.ARN != "" {
		return ref.ARN, nil
	}
	tdCR := &awsv1alpha1.ECSTaskDefinition{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, tdCR); err != nil {
		return "", err
	}
	if tdCR.Status.TaskDefinitionARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("ECSTaskDefinition %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return tdCR.Status.TaskDefinitionARN, nil
}

// resolveGroupName resolves a GroupRef to an AWS IAM group name.
func resolveGroupName(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.GroupRef) (string, error) {
	if ref.GroupName != "" {
		return ref.GroupName, nil
	}
	grp := &awsv1alpha1.IAMGroup{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, grp); err != nil {
		return "", err
	}
	if grp.Spec.GroupName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMGroup %s/%s has no groupName yet", namespace, ref.Name)}
	}
	if grp.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMGroup %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return grp.Spec.GroupName, nil
}

// resolveUserName resolves a UserRef to an AWS IAM user name.
func resolveUserName(ctx context.Context, c client.Client, namespace string, ref awsv1alpha1.UserRef) (string, error) {
	if ref.UserName != "" {
		return ref.UserName, nil
	}
	usr := &awsv1alpha1.IAMUser{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, usr); err != nil {
		return "", err
	}
	if usr.Spec.UserName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMUser %s/%s has no userName yet", namespace, ref.Name)}
	}
	if usr.Status.ARN == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("IAMUser %s/%s has no ARN yet", namespace, ref.Name)}
	}
	return usr.Spec.UserName, nil
}

// resolveEKSClusterName resolves an EKSClusterRef to an AWS cluster name.
// Uses direct .ClusterName if set, otherwise looks up the EKSCluster CR's status.
func resolveEKSClusterName(ctx context.Context, c client.Client, namespace string, directName string, ref *awsv1alpha1.EKSClusterRef) (string, error) {
	if directName != "" {
		return directName, nil
	}
	if ref == nil {
		return "", fmt.Errorf("either clusterName or clusterRef must be set")
	}
	if ref.ClusterName != "" {
		return ref.ClusterName, nil
	}
	clusterCR := &awsv1alpha1.EKSCluster{}
	if err := c.Get(ctx, k8stypes.NamespacedName{Name: ref.Name, Namespace: namespace}, clusterCR); err != nil {
		return "", err
	}
	if clusterCR.Spec.ClusterName == "" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("EKSCluster %s/%s has no clusterName yet", namespace, ref.Name)}
	}
	if clusterCR.Status.Status != "ACTIVE" {
		return "", &dependencyNotReady{msg: fmt.Sprintf("EKSCluster %s/%s is not yet ACTIVE (status: %s)", namespace, ref.Name, clusterCR.Status.Status)}
	}
	return clusterCR.Spec.ClusterName, nil
}

// providerResolver is set once at startup by SetProviderResolver. When nil
// (unit tests) every reconcile runs against the operator's own credentials.
var providerResolver *provider.Resolver

// SetProviderResolver installs the multi-account resolver used by every
// controller to scope AWS calls to the resource's AWSProvider.
func SetProviderResolver(r *provider.Resolver) { providerResolver = r }

// withProviderScope attaches the resolved AWS account/region scope for obj to
// ctx. The generated multi-account SDK wrappers read it on every call.
func withProviderScope(ctx context.Context, obj provider.ProviderScoped) (context.Context, error) {
	if providerResolver == nil {
		return ctx, nil
	}
	s, err := providerResolver.ForObject(ctx, obj)
	if err != nil {
		return ctx, fmt.Errorf("resolve AWS provider: %w", err)
	}
	// Record the target account/region on the object; it is persisted with
	// the next status write of the reconcile.
	if setter, ok := obj.(provider.ProviderStatusSetter); ok {
		setter.SetProviderStatus(providerResolver.StatusFor(s))
	}
	if s == nil {
		return ctx, nil
	}
	return provider.WithScope(ctx, s), nil
}

// crossAccountContext returns a context scoped to the *other* side of a
// two-sided resource (the accepter of a peering, the owner of a shared TGW,
// the VPC account of a private hosted zone...). ref names the AWSProvider for
// that side; region, when non-empty, overrides the region. With a nil ref the
// current scope is reused with only the region override applied, which covers
// same-account cross-region cases.
func crossAccountContext(ctx context.Context, namespace string, ref *awsv1alpha1.ProviderRef, region string) (context.Context, error) {
	var s *provider.Scope
	if ref != nil && ref.Name != "" {
		if providerResolver == nil {
			return ctx, nil
		}
		var err error
		s, err = providerResolver.ForName(ctx, ref.Name, namespace)
		if err != nil {
			return ctx, fmt.Errorf("resolve accepter AWS provider: %w", err)
		}
		if ref.Region != "" {
			s.Region = ref.Region
		}
	} else {
		cur := provider.ScopeFrom(ctx)
		if cur != nil {
			c := *cur
			s = &c
		} else {
			s = &provider.Scope{}
		}
	}
	if region != "" {
		s.Region = region
	}
	if s.Region == "" && s.Credentials == nil {
		return ctx, nil
	}
	return provider.WithScope(ctx, s), nil
}

// requeuePending is the poll interval for two-sided resources awaiting the
// other party's acceptance or an async state transition.
var requeuePending = ctrl.Result{RequeueAfter: 30 * time.Second}

// errPendingAcceptance signals a two-sided resource is waiting on the other
// party; reconcilers translate it into requeuePending without logging an error.
var errPendingAcceptance = errors.New("pending acceptance")
