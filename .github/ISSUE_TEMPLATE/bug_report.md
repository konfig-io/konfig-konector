---
name: Bug report
about: Something isn't reconciling the way it should
labels: bug
---

**What happened**

<!-- What did you observe? Include the Ready condition / status of the CR. -->

**What you expected**

**The CR that reproduces it**

```yaml
# kubectl get <kind> <name> -n <ns> -o yaml (redact account IDs if you prefer)
```

**Controller logs**

```
# kubectl logs -n konfig-system deploy/konfig-controller | grep <name>
```

**Environment**

- konfig-konector version/image tag:
- Kubernetes version / EKS platform version:
- Install method: helm / make install / other
