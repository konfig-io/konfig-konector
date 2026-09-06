# Running konfig-konector under Argo CD

konfig-konector resources are plain CRDs, so any GitOps tool applies them.
Two Argo CD settings make the experience good.

## Health checks

By default Argo CD shows custom resources as Healthy the moment they exist.
`argocd/health-customizations.yaml` teaches Argo to read the `Ready`
condition of every `aws.konfig.io` kind instead: a resource is Healthy when
`Ready=True` and Progressing otherwise, with the reason and the AWS error
message shown in the resource tooltip.

```sh
kubectl -n argocd patch configmap argocd-cm --type merge --patch-file argocd/health-customizations.yaml
kubectl -n argocd rollout restart statefulset/argocd-application-controller
```

If your `argocd-cm` already carries `resource.customizations`, merge the
`aws.konfig.io/*` block into it.

## Sync options

Use server-side apply. Several CRDs (and the generated Cloud Control kinds)
exceed the client-side last-applied annotation limit:

```yaml
syncPolicy:
  syncOptions: [ServerSideApply=true, CreateNamespace=true]
```

Leave `prune` off for Applications that manage AWS resources until you are
comfortable with the deletion policy. Deleting a CR deletes the AWS resource
unless it carries `aws.konfig.io/deletion-policy: abandon`.

## Charts

- `helm/konfig-konector`: the operator, CRDs, RBAC, and `AWSProvider`
  objects rendered from `providers`.
- `helm/konfig-smoke`: one minimal free-tier instance of every kind that
  costs nothing to hold, used as a live smoke test. Values: `accountId`,
  `region`, `amiId`.

Generated Cloud Control kinds are installed per service bundle
(`config/crd/cloudcontrol/<service>.yaml`), outside the chart. The operator
starts controllers only for installed CRDs; restart it after adding a bundle.

## Local bootstrap

`scripts/local-dev.sh up` stands up a k3d cluster with an in-cluster OCI
registry, installs Argo CD with the settings above, pushes the operator image
and chart to the registry, and creates the `konfig-konector` Application.
`scripts/local-dev.sh smoke` adds the smoke chart as a second Application;
`unsmoke` removes it and lets the operator delete everything in AWS. See the
header of the script for all commands.
