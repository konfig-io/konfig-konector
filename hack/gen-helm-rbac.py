#!/usr/bin/env python3
"""Rewrite the aws.konfig.io resource list in the Helm ClusterRole from the
generated CRDs so it never drifts from config/crd/bases. Run by `make manifests`."""
import glob, os, re
ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
plurals = sorted(os.path.basename(p)[len("aws.konfig.io_"):-len(".yaml")]
                 for p in glob.glob(os.path.join(ROOT, "config/crd/bases/aws.konfig.io_*.yaml")))
path = os.path.join(ROOT, "helm/konfig-konector/templates/clusterrole.yaml")
s = open(path).read()
block = "".join(f"      - {p}\n" for p in plurals)
new, n = re.subn(r'(  - apiGroups: \["aws\.konfig\.io"\]\n    resources:\n)(?:      - [a-z0-9]+\n)+', r"\g<1>" + block, s, count=1)
if n != 1:
    raise SystemExit("clusterrole.yaml: aws.konfig.io resources block not found")
open(path, "w").write(new)
print(f"helm clusterrole: {len(plurals)} aws.konfig.io resources")
