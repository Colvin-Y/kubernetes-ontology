# kubernetes-ontology

A read-only Kubernetes ontology database for diagnostics and AI-agent consumption.

The project builds an in-memory ontology backend from Kubernetes objects, serves entity/relation queries for AI agents, and exposes diagnostic subgraph queries as the first MVP query family. It can keep the graph fresh during a bounded CLI observe window or as a long-running daemon.

The open-source MVP backend is in-memory only. Persistent graph storage,
external graph adapters, and runtime recovery beyond rebuilding from Kubernetes
after restart are intentionally out of scope for the MVP.

For the standard server + client workflow, start with [QUICKSTART.md](QUICKSTART.md).

## Install Without Compiling

The release path is designed so users do not need a Go toolchain:

1. Install the Helm chart with the published image:

   ```bash
   export KO_VERSION=v0.1.0
   export KO_IMAGE=ghcr.io/colvin-y/kubernetes-ontology

   helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
     --namespace kubernetes-ontology \
     --create-namespace \
     --set image.repository="${KO_IMAGE}" \
     --set image.tag="${KO_VERSION}" \
     --set cluster="your-logical-cluster" \
     --set contextNamespaces='{default,kube-system}'
   ```

   The chart is read-only. Its pod listeners use `:18080` and `:8765` rather
   than IPv4-only `0.0.0.0` addresses, so they work better in IPv4, IPv6, and
   dual-stack clusters. It does not grant `secrets` list/watch permission unless
   you set `rbac.readSecrets=true`.

2. Expose the server locally:

   ```bash
   kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology 18080:18080
   ```

3. Download the `kubernetes-ontology` CLI from
   [GitHub Releases](https://github.com/Colvin-Y/kubernetes-ontology/releases)
   and query the server:

   ```bash
   kubernetes-ontology --server "http://127.0.0.1:18080" --status
   ```

The same release archive includes `kubernetes-ontology-viewer`, a Go binary
that serves the topology viewer without Python:

```bash
kubernetes-ontology-viewer --server "http://127.0.0.1:18080"
```

The Helm chart also deploys the viewer by default:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology-viewer 8765:8765
```

## Release Publishing

Tagged releases publish no-compile artifacts:

- `.github/workflows/release.yml` builds Linux, macOS, and Windows archives for
  `kubernetes-ontology`, `kubernetes-ontologyd`, and
  `kubernetes-ontology-viewer`, then attaches them to the GitHub Release.
- `.github/workflows/docker.yml` builds a multi-arch image and pushes it to
  GitHub Container Registry as `ghcr.io/colvin-y/kubernetes-ontology:<tag>`
  plus the SemVer alias without the leading `v`, and `latest`.

No Docker Hub account or repository secret is required. The workflow uses the
repository `GITHUB_TOKEN` with `packages: write` permission.
See [docs/release.md](docs/release.md) for the full release checklist.

Then publish:

```bash
git tag v0.1.0
git push origin v0.1.0
```

After the first package is created, check the package visibility under your
GitHub profile's Packages tab. Public GHCR packages can be pulled anonymously;
if the package is private, change its visibility to Public before documenting it
for external users.

## Safety Model

The binary runs locally with a user-provided kubeconfig.

It does not:
- create Kubernetes objects
- patch or update resources
- write annotations or CRDs
- create RBAC resources
- modify cluster state

Collection is read-only.

## MVP Boundaries

The MVP is a local, read-only topology recovery service:
- `kubernetes-ontologyd` reads Kubernetes objects and keeps the ontology graph in process memory.
- Restarting the daemon discards the previous graph and rebuilds from the Kubernetes API.
- No persistent database, external graph backend, RDF/OWL materialization, CRD/controller installation, or cluster-side agent is required or included.
- The HTTP API is intended for local or controlled environments, not public multi-tenant exposure.

## Current Capabilities

### Diagnostic Entrypoints
- `Pod`
- `Workload`

### Runtime
- bootstrap full snapshot sync
- runtime status projection
- HTTP daemon entrypoint
- standalone CLI client for daemon-backed queries
- bounded observe mode with informer or polling streams
- category-aware change planning
- scoped graph mutation for common update categories

Current narrow strategies:
- `service-narrow`
- `event-narrow`
- `storage-narrow`
- `identity/security-narrow`
- `pod-narrow`
- `workload-narrow`

Unsupported categories still fall back to full rebuild.

### Graph Recovery
- recursive workload / pod ownership recovery through ownerReferences
- `Pod -> ReplicaSet -> Deployment` and deeper controller chains
- configurable custom workload collection for CRDs such as Kruise ASTS and Redis clusters
- configurable workload controller display rules, for control-plane pods that are not discoverable from ownerReferences
- service selector matching
- pod -> node
- pod -> secret / configmap / serviceaccount
- serviceaccount -> rolebinding / clusterrolebinding for ServiceAccount subjects
- pod -> image with lightweight OCI parsing
- pod -> PVC
- PVC -> PV
- PVC / PV -> StorageClass
- StorageClass -> CSIDriver when the provisioner maps to an observed or CSI-shaped driver
- event evidence
- admission webhook evidence
- PV CSI metadata extraction

### CSI Correlation
Current CSI resolver:
- `local.csi.aliyun.com` -> OpenLocal-style controller and node-agent inference

Recovered evidence can include:
- `provisioned_by_csi_driver`
- `implemented_by_csi_controller`
- `implemented_by_csi_node_agent`
- `managed_by_csi_controller`
- `served_by_csi_node_agent`
- explanation text when no matching node-local agent is found

## Quick Start

The recommended MVP workflow is covered in [QUICKSTART.md](QUICKSTART.md):

- start `kubernetes-ontologyd` as the continuous local ontology server
- query it with the standalone `kubernetes-ontology` CLI client
- use the same HTTP API directly from an AI agent or automation

Minimal build and test commands:

```bash
make build
```

```bash
make test
```

## CLI Usage

The Makefile wraps the common commands. Direct CLI usage is also supported:

```bash
go run ./cmd/kubernetes-ontology \
  --kubeconfig "$HOME/.kube/config" \
  --cluster "kind-kind" \
  --entry-kind "Pod" \
  --namespace "default" \
  --name "my-pod"
```

Status-only mode:

```bash
go run ./cmd/kubernetes-ontology \
  --kubeconfig "$HOME/.kube/config" \
  --cluster "kind-kind" \
  --namespace "default" \
  --status-only
```

Bounded observe mode:

```bash
go run ./cmd/kubernetes-ontology \
  --kubeconfig "$HOME/.kube/config" \
  --cluster "kind-kind" \
  --namespace "default" \
  --status-only \
  --observe-duration 40s \
  --stream-mode polling \
  --poll-interval 2s
```

YAML config is the recommended way to keep cluster-specific topology rules:

```bash
cp local/kubernetes-ontology.yaml.example local/kubernetes-ontology.yaml
go run ./cmd/kubernetes-ontologyd --config local/kubernetes-ontology.yaml
```

The config can define dynamic workload resources for ownerReference inference
and display-only controller rules:

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
controllerRules:
  - apiVersion: apps.kruise.io/*
    kind: "*"
    namespace: kruise-system
    controllerPodPrefixes:
      - kruise-controller-manager
    nodeDaemonPodPrefixes:
      - kruise-daemon
```

For the CLI, daemon-query mode remains explicit: pass `--server` or use the
`*-server` make targets. A `server.url` value in YAML does not silently switch
local collection commands into server mode.

Daemon-backed mode:

```bash
go run ./cmd/kubernetes-ontologyd \
  --kubeconfig "$HOME/.kube/config" \
  --cluster "kind-kind" \
  --context-namespaces "default,kube-system" \
  --workload-resources "apps.kruise.io/v1beta1/statefulsets/StatefulSet,redis.io/v1beta1/clusters/Cluster" \
  --controller-rules "apiVersion=apps.kruise.io/*;kind=*;namespace=kruise-system;controller=kruise-controller-manager;daemon=kruise-daemon" \
  --addr "127.0.0.1:18080"
```

```bash
go run ./cmd/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-entities \
  --entity-kind Pod \
  --namespace default
```

```bash
go run ./cmd/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --entry-kind Pod \
  --namespace default \
  --name my-pod
```

## AI-Agent Workflows

Agents should prefer daemon-backed read-only queries with `--server`. The common
flows are available as action-oriented aliases while the older flags remain
valid:

```bash
./bin/kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name my-pod
```

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --expand-entity \
  --entity-id 'your/entityGlobalId' \
  --expand-depth 1 \
  --limit 100
```

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --diagnose-pod \
  --namespace default \
  --name my-pod
```

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --diagnose-workload \
  --namespace default \
  --name my-deployment
```

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-filtered-relations \
  --from 'your/entityGlobalId' \
  --relation-kind scheduled_on \
  --limit 50
```

Graph and list responses preserve their existing fields and add `freshness`
metadata when the daemon has runtime status. Server errors keep the historical
`error` string and add `code`, `message`, `status`, `retryable`, and `source`.
Agents can request JSON-only stderr for server query failures:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --machine-errors \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name missing-pod
```

## Server API

The daemon exposes the current in-memory ontology database over HTTP:

- `GET /healthz`
- `GET /status`
- `GET /entity?entityGlobalId=...`
- `GET /entity?kind=Pod&namespace=default&name=my-pod`
- `GET /entities?kind=Pod&namespace=default&limit=50`
- `GET /relations?from=...&kind=scheduled_on`
- `GET /neighbors?entityGlobalId=...&direction=out`
- `GET /expand?entityGlobalId=...&depth=1`
- `GET /diagnostic/pod?namespace=default&name=my-pod`
- `GET /diagnostic/workload?namespace=default&name=my-deployment`

The diagnostic endpoints are built on top of the same ontology graph. They are not the whole product surface.

Entity, relation, neighbor, expand, and diagnostic responses include additive
`freshness` metadata with readiness, graph counts, and the last runtime refresh
timestamps when the daemon can provide them.

Diagnostic traversal uses boundary nodes to keep pod-centered graphs focused.
By default, shared or evidence-like nodes such as `ServiceAccount`, `ConfigMap`,
`Secret`, `Node`, `Service`, `Event`, `Image`, and webhook metadata are shown but
not used as another expansion frontier. HTTP clients can override this with
`terminalKinds=ServiceAccount,Secret` or `expandTerminalNodes=true`.

Useful flags:
- `--config`: YAML config file for kubeconfig, cluster, collection scope, CRD workload resources, controller display rules, server address, and traversal defaults
- `--kubeconfig`: kubeconfig path
- `--cluster`: logical cluster name used in canonical IDs
- `--entry-kind`: `Pod` or `Workload`
- `--namespace`: namespace filter or diagnostic entry namespace
- `--name`: entry object name
- `--context-namespaces`: comma-separated namespaces to collect as ontology context, for example `default,kube-system`; empty means all namespaces
- `--namespaces`: alias for `--context-namespaces`
- `--workload-resources`: comma-separated custom workload resources; overrides `workloadResources` from `--config`
- `--controller-rules`: comma-separated controller display rules; overrides `controllerRules` from `--config`
- `--max-depth`: general diagnostic traversal depth
- `--storage-max-depth`: deeper traversal budget for storage / CSI paths
- `--bootstrap-timeout`: timeout for the initial full snapshot sync
- `--stream-mode`: continuous update mode, either `informer` or `polling`; the daemon defaults to `informer` and falls back to polling if informer startup fails
- `--terminal-kinds`: comma-separated diagnostic boundary node kinds; default keeps shared/evidence nodes visible but stops traversal through them
- `--expand-terminal-nodes`: traverse through boundary nodes for deep debugging
- `--status-only`: print runtime status instead of a diagnostic subgraph
- `--status`: alias for `--status-only`
- `--observe-duration`: keep observing before printing output
- `--poll-interval`: polling interval used with polling mode and informer fallback
- `--server`: query an existing `kubernetes-ontologyd` server instead of collecting locally
- `--machine-errors`: print server query failures as structured JSON on stderr
- `--diagnose-pod`, `--diagnose-workload`: aliases for diagnostic entrypoints using `--namespace` and `--name`
- `--expand-node`, `--expand-entity`, `--entity-id`, `--expand-depth`: return the same bounded node expansion subgraph used by the viewer
- `--collapse-node`, `--graph-file`, `--entity-id`: collapse a node expansion from an exported viewer state JSON file
- `--list-entities`, `--get-entity`, `--resolve-entity`, `--list-relations`, `--list-filtered-relations`, `--neighbors`: daemon-backed ontology database queries

## Visualization

A local topology viewer is included:
- `tools/visualize/index.html`
- `tools/visualize/server.py`
- `kubernetes-ontology-viewer`, a dependency-free Go binary that embeds the
  same HTML UI

Start the ontology daemon first:

```bash
make serve
```

Start the viewer in another terminal:

```bash
make visualize
```

Or use the release viewer binary directly:

```bash
kubernetes-ontology-viewer --server "http://127.0.0.1:18080"
```

After code changes, use the fixed verification flow:

```bash
make verify
make serve
make visualize
make live-check NAMESPACE=default NAME=my-pod
```

`live-check` calls the daemon status endpoint, the viewer topology proxy, and a
pod diagnostic query through the viewer. This catches both backend graph issues
and the timeout-prone visualization path.

Then open:

```text
http://127.0.0.1:8765
```

The viewer can:
- load live topology from `SERVER_URL`
- poll the daemon for near-real-time refresh
- query pod or workload diagnostic subgraphs
- expand a selected node by one hop with the same `/expand` API exposed to CLI users
- collapse a previously expanded node; exported viewer state can be collapsed from the CLI with `make -s collapse-node-graph`
- filter by node kind, relation kind, namespace, text, and layout
- inspect node details, edge provenance, and connected relations
- export the visible subgraph as JSON

It can also load a diagnostic JSON file by:
- file picker
- drag and drop
- absolute file path through the local server
- `?file=/tmp/graph.json` URL hint

The browser has no direct cluster access. It talks to the local viewer server,
which proxies read-only calls to `kubernetes-ontologyd`.

## Architecture

Core layers:
- `internal/collect/k8s`: read-only Kubernetes collection, informer stream, and polling fallback
- `internal/runtime`: lifecycle, bootstrap, status, and stream application
- `internal/ontology`: backend abstraction for entity/relation storage and graph queries
- `internal/server`: HTTP API for runtime status, ontology queries, and diagnostics
- `internal/reconcile`: full rebuild plus scoped mutation reconcilers
- `internal/graph`: graph builder, kernel, and index
- `internal/query`: query facade
- `internal/service/diagnostic`: diagnostic subgraph query implementation
- `tools/visualize`: local graph viewer

Current owner-chain behavior:
- prefer controller ownerReferences when present
- resolve by UID first
- fall back to namespace/kind/name
- guard against cycles
- support deeper chains beyond `Pod -> ReplicaSet -> Deployment`

## Design References

Current restart entrypoint:
- `docs/design/current-state-and-next-steps.md`

Additional design documents:
- `docs/design/README.md`
- `docs/design/kubernetes-semantic-kernel-evolution.md`
- `docs/design/continuous-runtime-technical-design.md`
- `docs/design/continuous-runtime-progress-snapshot.md`

AI-agent contract:
- `AI_CONTRACT.md`

Research foundations:
- `docs/design/research-foundations.md`
- `docs/design/research/kubernetes-ontology-research.md`
- `docs/design/research/AICCSA66935.2025.11315476.md`

## Tests

Preferred stable test command:

```bash
go test -p 1 ./...
```

This is what `make test` runs.

Coverage includes:
- graph model and canonical IDs
- selector matching
- OCI parsing
- graph builder contracts
- diagnostic traversal behavior
- read-only fake client collection
- polling stream change detection
- informer event classification
- recursive owner-chain resolution
- scoped reconcilers
- runtime strategy accounting

## Known Limitations

Current limitations:
- graph state is in memory only by MVP design
- HTTP API is intended for local or controlled internal networks; auth/TLS is not implemented yet
- persistent graph backends and external graph adapters are intentionally outside the open-source MVP
- RBAC topology is represented for ServiceAccount subjects and binding objects, not as a full permission reasoning engine
- evidence ranking is basic
- ontology database materialization is not implemented

## Recommended Next Steps

1. Extend informer and scoped-reconcile coverage for future topology categories while preserving the same `ChangeEvent` sink.
2. Add HTTP auth/TLS and internal soak tests for daemon mode.
3. Improve diagnostic evidence ranking and schema coverage for downstream AI agents.
4. Broaden RBAC interpretation carefully without turning the MVP into a full authorization engine.
5. Keep persistent stores and external graph adapters as post-MVP research, separate from the open-source MVP path.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
