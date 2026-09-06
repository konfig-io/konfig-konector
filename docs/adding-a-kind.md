# Adding a resource kind

## First: does it need a hand-written kind?

Check `hack/gen-cloudcontrol/kinds.json`. If the CloudFormation type already
has a generated kind (`make gen-cloudcontrol`), prefer improving the generator
over writing a controller. Hand-write a kind only when it needs something the
Cloud Control engine cannot express: cross-resource refs into other CRs,
Secret refs, multi-step or two-sided handshakes (peering, RAM, Route 53
authorization), or an AWS API with no CloudFormation coverage. When a native
kind replaces a generated one, add the CloudFormation type to `NATIVE` in
`hack/gen-cloudcontrol/gen.py` so the two never coexist.

## Required vertical slice

Every hand-written kind ships ALL of the following. Reference implementations:
`api/v1alpha1/sqsqueue_types.go`, `internal/controller/sqsqueue_controller.go`
(+ its `_test.go`), `internal/export/exporters/sqs.go`.

### 1. API type — `api/v1alpha1/<lowerkind>_types.go`
- Spec/Status structs, `SchemeBuilder.Register` in init(), printcolumns for Ready + Age.
- Spec always starts with `ProviderRef *ProviderRef` (`json:"providerRef,omitempty"`);
  add `GetProviderRef()` and `SetProviderStatus()` accessors to
  `api/v1alpha1/zz_providerref.go`.
- Status always: primary AWS identifier, `AWSProvider *ProviderStatus`
  (`json:"awsProvider,omitempty"`), `Conditions []metav1.Condition`,
  `ObservedGeneration int64`, `LastSyncTime *metav1.Time`.
- CEL immutability on REQUIRED scalar fields that are immutable in AWS:
  `+kubebuilder:validation:XValidation:rule="self == oldSelf",message="<json> is immutable"`.
  Never on optional fields.
- Secret-shaped values use `SecretRef` (see common_types.go) — never a plain string.
- Reuse the shared ref types (RoleRef, SubnetRef, SecurityGroupRef, VPCResourceRef…).
- Fields AWS requires must be required in the CRD (X-Ray groups need a filter
  expression; do not let the API reject what the schema accepted).

### 2. Controller — `internal/controller/<lowerkind>_controller.go`
- Struct: `client.Client`, `Scheme *runtime.Scheme`, `<Svc>Client <Kind>AWSAPI`
  where `<Kind>AWSAPI` is an interface over EXACTLY the SDK methods used.
  Never a concrete `*multi.X` client: interfaces are what make the fake-based
  tests possible.
- RBAC markers: resources plural MUST match the CRD plural exactly
  (check `config/crd/bases` after `make manifests`).
- Reconcile flow (copy sqsqueue):
  1. Get; IgnoreNotFound.
  2. `ctx, scopeErr = withProviderScope(ctx, obj)` — resolves the AWSProvider,
     attaches the account/region scope, and records `status.awsProvider`.
  3. Deletion: ContainsFinalizer → `shouldAbandon(obj)` check FIRST (remove
     finalizer, no AWS call) → delete<Kind>() → RemoveFinalizer → Update.
  4. AddFinalizer if missing, Update, then **return `ctrl.Result{Requeue: true}`**.
     Creating in the same pass races the stale-cache reconcile queued by the
     finalizer update and produces duplicate AWS resources.
  5. reconcile<Kind>(): on `*dependencyNotReady` → `requeueDependency, nil`;
     on `errPendingAcceptance` (two-sided kinds) → `requeuePending, nil`;
     on error → setCondition(Error) + return err; success → `requeueResult(), nil`.
- **Look before you create.** Describe/Get by the deterministic name or
  attributes (name, VPC + CIDR, attachment) before calling Create, and adopt
  what you find. Treat `AlreadyExists` on Create as adopt-by-lookup. A
  resource that exists in AWS but not in status must never be created twice.
- **persistStatus(ctx, r.Client, obj) IMMEDIATELY after any AWS create call
  stores the identifier** — before any follow-up step. Non-negotiable.
- Delete helper: if the status identifier is empty, fall back to a
  deterministic spec-based lookup where the AWS API allows an unambiguous
  match; otherwise return nil. Not-found (including service quirks such as
  Backup's AccessDenied for a missing vault, or S3's NoSuchBucket on a
  sub-resource) is success.
- Update path: gate mutating calls on `ObservedGeneration != Generation`. Rate
  limited control planes (CloudFront, IAM, Route 53) must not be written on
  every drift check. If AWS can't update a changed field, set Ready=False with
  `awsv1alpha1.ReasonUpdateNotSupported` and do NOT bump ObservedGeneration.
- Never read the operator's own region or account (`os.Getenv("AWS_REGION")`,
  `Config.Region`, a startup account ID). Region and account come from the
  provider scope on the context or from the spec;
  `scope_guard_test.go` fails the build otherwise. Use
  `provider.ScopeFrom(ctx)` when you must build an ARN.
- Use `crossAccountContext(ctx, namespace, ref, region)` for the *other* side
  of a two-sided resource (accepter, VPC owner). Ready only when AWS confirms
  both sides; report `ReasonPendingAcceptance` meanwhile.
- setCondition helper uses `persistStatus` (copy from sqsqueue) — never a
  bare `r.Status().Update` with conflict-swallowing.
- Async resources: poll with a family-level `requeue<X>Polling` result.
- EC2 tag specifications with no tags are stripped by client middleware; do
  not special-case empty `spec.tags`.

### 3. Registration — `internal/controller/register_<family>.go`
Do NOT touch cmd/main.go. One file per family:
```go
func init() {
    RegisterSetup(func(mgr ctrl.Manager, clients *awsclient.Clients) error {
        if err := (&FooReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme(),
            FooClient: clients.Foo}).SetupWithManager(mgr); err != nil {
            return fmt.Errorf("Foo: %w", err)
        }
        return nil
    })
}
```
`clients.Foo` is a generated multi-account wrapper (`internal/aws/multi`); it
satisfies your interface and applies the provider scope per call. New AWS
services: add the SDK module, list it in `hack/gen-multiclient/main.go`, run
`go run ./hack/gen-multiclient`, add the client to `internal/aws/client.go`.
Upgrade every `aws-sdk-go-v2` module together; mixed versions fail at runtime
with `not found: ComputePayloadHash`.

### 4. AWS helper — `internal/aws/<service>/helpers.go`
`IsNotFound(err error) bool` via smithy `ErrorCode()` matching (copy an
existing one, use the service's real not-found codes).

### 5. Exporter — `internal/export/exporters/<family>.go`
- Register with a sensible Order (referenced kinds before referrers).
- Map ONLY spec fields from List/Describe output; required fields always set;
  never status; paginators everywhere.
- `export.ObjectMeta(name, opts)` for metadata; `opts.Index.Add(id/arn, cr.Name)`;
  cross-refs via `opts.Index.Lookup` (CR name) falling back to the raw-ID form.
- NEVER export secret material; `CHANGEME` placeholder + comment where a
  required field can't round-trip.
- Skip AWS-managed/default resources.
- Per-resource errors `continue`; top-level list errors return.

### 6. Tests — `internal/controller/<lowerkind>_controller_test.go`
Table-driven (NOT Ginkgo), controller-runtime fake client with
`.WithStatusSubresource(...)`, fake AWS struct with function fields.
Fixtures carry `Finalizers: []string{awsv1alpha1.FinalizerName}` (a fresh
object without the finalizer reconciles to `Requeue: true` and creates nothing).
Minimum cases: create happy path; adopt when the lookup finds an existing
resource (no Create call); identifier persisted when a post-create step
fails; steady state (no create, no update); delete with finalizer; abandon
annotation (no AWS delete); dependency-not-ready where refs exist.

### 7. Smoke coverage
If the kind is free to create and hold, add a minimal instance to
`test/smoke/free-tier.yaml` (the source of `helm/konfig-smoke`) so
`./scripts/local-dev.sh smoke` exercises it. Kinds with a blast radius beyond
their own resource (account/organization settings, credential material) are
generated with a destructive-scope warning instead; see
`docs/cloudcontrol-kinds.md`.

### 8. After the code
`make manifests generate` (regenerates CRDs, Helm CRDs and RBAC, Cloud Control
bundles), `make gen-reference` (site + examples), `make parity` (coverage
report). Add the CloudFormation type to `NATIVE` in the generator if it exists
there.
