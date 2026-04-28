# Local Configuration

This directory is for machine-local Kubernetes debugging configuration.

Real files such as `local/kubernetes-ontology.yaml` are ignored by git. Put local kubeconfig paths, private cluster names, namespaces, topology collection resources, display rules, and scratch query defaults there.

Bootstrap from the example:

```bash
cp local/kubernetes-ontology.yaml.example local/kubernetes-ontology.yaml
```

Then edit `local/kubernetes-ontology.yaml` and run normal targets. The Makefile
automatically uses this file when it exists:

```bash
make status
make serve
make list-entities-server ENTITY_KIND=Pod
```

Use `CONFIG=other.yaml` for a different config file.

`contextNamespaces` is the server collection scope:

```yaml
contextNamespaces:
  - default
  - kube-system
```

Use an empty list to collect all namespaces:

```yaml
contextNamespaces: []
```

`bootstrapTimeout` controls the initial full snapshot sync timeout:

```yaml
bootstrapTimeout: 2m
```

Large clusters or slow API servers may need more than the previous 30 second
bootstrap window.

`workloadResources` configures CRD-like workloads that should participate in
ownerReference inference:

```yaml
workloadResources:
  - group: apps.kruise.io
    version: v1beta1
    resource: statefulsets
    kind: StatefulSet
    namespaced: true
  - group: redis.io
    version: v1beta1
    resource: clusters
    kind: Cluster
    namespaced: true
```

Use the actual Kubernetes ownerReference `kind`, such as `StatefulSet` for
Kruise ASTS, not a local nickname.

These entries are optional. On a clean kind cluster without OpenKruise, Redis
operators, or similar CRDs installed, informer setup logs the missing resource
and skips that custom workload instead of stopping the server.

`controllerRules` configures display-only controller relationships that are not
available from Kubernetes object references:

```yaml
controllerRules:
  - apiVersion: apps.kruise.io/*
    kind: "*"
    namespace: kruise-system
    controllerPodPrefixes:
      - kruise-controller-manager
    nodeDaemonPodPrefixes:
      - kruise-daemon
```

`csiComponentRules` configures CSI driver implementation relationships when
the StorageClass provisioner name is known but the controller and node-agent Pod
names are cluster-specific. No driver-specific component inference runs unless
a matching rule is configured:

```yaml
csiComponentRules:
  - driver: diskplugin.csi.alibabacloud.com
    namespace: kube-system
    controllerPodPrefixes:
      - csi-provisioner-
    nodeAgentPodPrefixes:
      - csi-plugin-
```

If you do not use a YAML config, `CONTEXT_NAMESPACES` is the make-variable
collection scope. It is a comma-separated string:

```makefile
CONTEXT_NAMESPACES := default,kube-system
```

Use an empty value to collect all namespaces:

```makefile
CONTEXT_NAMESPACES :=
```

This list does not mark pods as infrastructure or business resources. It only
controls which namespaces the server reads.

For make variables, do not use YAML, JSON, shell arrays, or repeated
`CONTEXT_NAMESPACES` lines. Spaces after commas are tolerated, but the no-space
form is preferred.

Command-line variables still work for scratch query defaults:

```bash
make diagnose-pod CONFIG=local/kubernetes-ontology.yaml NAMESPACE=other-ns NAME=my-pod
```

The CLI does not enter daemon-query mode just because `server.url` exists in
the YAML file. Use `--server` or the `*-server` make targets for that path.
