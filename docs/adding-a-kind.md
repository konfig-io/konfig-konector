# Adding a resource kind — required vertical slice

Every new kind ships ALL of the following. Reference implementations:
`api/v1alpha1/sqsqueue_types.go`, `internal/controller/sqsqueue_controller.go`
(+ its `_test.go`), `internal/export/exporters/sqs.go`.

## 1. API type — `api/v1alpha1/<lowerkind>_types.go`
- Spec/Status structs, `SchemeBuilder.Register` in init(), printcolumns for Ready + Age.
- Status always: primary AWS identifier, `Conditions []metav1.Condition`,
  `ObservedGeneration int64`, `LastSyncTime *metav1.Time`.
- CEL immutability on REQUIRED scalar fields that are immutable in AWS:
  `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="<json> is immutable"`.
  Never on optional fields.
- Secret-shaped values use `SecretRef` (see common_types.go) — never a plain string.
- Reuse the shared ref types (RoleRef, SubnetRef, SecurityGroupRef, VPCResourceRef…).

## 2. Controller — `internal/controller/<lowerkind>_controller.go`
- Struct: `client.Client`, `Scheme *runtime.Scheme`, `<Svc>Client <Kind>AWSAPI`
  where `<Kind>AWSAPI` is an interface over EXACTLY the SDK methods used
  (satisfied by the concrete `*svc.Client`).
- RBAC markers: resources plural MUST match the CRD plural exactly
  (check `config/crd/bases` after `make manifests` — typos here caused
  runtime Forbidden bugs before).
- Reconcile flow (copy sqsqueue):
  1. Get; IgnoreNotFound.
  2. Deletion: ContainsFinalizer → `shouldAbandon(obj)` check FIRST (remove
     finalizer, no AWS call) → delete<Kind>() → RemoveFinalizer → Update.
  3. AddFinalizer if missing.
  4. reconcile<Kind>(): on `*dependencyNotReady` → `requeueDependency, nil`;
     on error → setCondition(Error) + return err; success → `requeueResult(), nil`.
- **persistStatus(ctx, r.Client, obj) IMMEDIATELY after any AWS create call
  stores the identifier** — before any follow-up step. Non-negotiable.
- Delete helper: if the status identifier is empty, fall back to a
  deterministic spec-based lookup where the AWS API allows an unambiguous
  match; otherwise return nil.
- Update path: gate mutating calls on `ObservedGeneration != Generation`.
  If AWS can't update a changed field, set Ready=False with
  `awsv1alpha1.ReasonUpdateNotSupported` and do NOT bump ObservedGeneration.
- setCondition helper uses `persistStatus` (copy from sqsqueue) — never a
  bare `r.Status().Update` with conflict-swallowing.
- Async resources: poll with a family-level `requeue<X>Polling` result.

## 3. Registration — `internal/controller/register_<family>.go`
Do NOT touch cmd/main.go. One file per family:
```go
func init() {
    RegisterSetup(func(mgr ctrl.Manager, clients *awsclient.Clients) error {
        if err := (&FooReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
            FooClient: clients.Foo}).SetupWithManager(mgr); err != nil {
            return fmt.Errorf("Foo: %w", err)
        }
        // ... one block per kind
        return nil
    })
}
```

## 4. AWS helper — `internal/aws/<service>/helpers.go`
`IsNotFound(err error) bool` via smithy `ErrorCode()` matching (copy an
existing one, use the service's real not-found codes).

## 5. Exporter — `internal/export/exporters/<family>.go`
- Register with a sensible Order (referenced kinds before referrers).
- Map ONLY spec fields from List/Describe output; required fields always set;
  never status; paginators everywhere.
- `export.ObjectMeta(name, opts)` for metadata; `opts.Index.Add(id/arn, cr.Name)`;
  cross-refs via `opts.Index.Lookup` (CR name) falling back to the raw-ID form.
- NEVER export secret material; `CHANGEME` placeholder + comment where a
  required field can't round-trip.
- Skip AWS-managed/default resources.
- Per-resource errors `continue`; top-level list errors return.

## 6. Tests — `internal/controller/<lowerkind>_controller_test.go`
Table-driven (NOT Ginkgo), controller-runtime fake client with
`.WithStatusSubresource(...)`, fake AWS struct with function fields.
Minimum cases: create happy path; identifier persisted when a post-create
step fails; steady state (no create); delete with finalizer; abandon
annotation (no AWS delete); dependency-not-ready where refs exist.
For big families, full tests on the 2-3 most important kinds and
create+delete+abandon on the rest is acceptable.

## 7. Verify before finishing
```
GOTOOLCHAIN=local make manifests generate
GOTOOLCHAIN=local go build ./...
GOTOOLCHAIN=local go test ./internal/controller/ -run 'Test<YourKinds>' -count=1
GOTOOLCHAIN=local go run ./cmd/export --list   # your kinds present
```
Check `config/rbac/role.yaml` contains your exact CRD plurals.
