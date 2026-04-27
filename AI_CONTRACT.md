# AI-Agent Diagnostic Subgraph Contract

This document defines the intended consumption contract for AI-Agent clients of `kubernetes-ontology` phase 1.

The goal is to make the current diagnostic subgraph output stable enough for downstream agent logic. The open-source MVP is backed only by an in-memory graph.

## Purpose

`GetDiagnosticSubgraph(entryRef, policy)` returns a **diagnostic slice** of the graph, not the whole graph.

An AI-Agent should treat this response as:
- a structured, provenance-aware set of graph facts
- scoped to diagnosis, not inventory completeness
- safe for best-effort reasoning, but not a substitute for cluster writes or remediation actions

## MVP runtime boundary

The MVP daemon keeps graph state in process memory only. Agents should assume:
- restarting `kubernetes-ontologyd` discards the current graph and rebuilds it from Kubernetes
- there is no persistent graph database or external graph adapter in the MVP
- there is no cluster-side agent, CRD, controller, or mutating Kubernetes path
- the HTTP API is intended for local or controlled environments

Persistent storage, RDF/OWL materialization, external graph backends, and public
multi-tenant API hardening are out of scope for the MVP contract.

## Entry model

### Required fields
The response always includes:
- `entry.kind`
- `entry.canonicalId`
- `entry.namespace` when namespaced
- `entry.name`

### Supported phase-1 entry kinds
- `Pod`
- `Workload`

## Response shape

```json
{
  "entry": {...},
  "nodes": [...],
  "edges": [...],
  "collectedAt": "...",
  "explanation": [...],
  "nodeCount": 12,
  "edgeCount": 18,
  "freshness": {
    "ready": true,
    "phase": "ready",
    "cluster": "cluster-a",
    "nodeCount": 120,
    "edgeCount": 240,
    "lastRefreshAt": "2026-04-23T10:01:00Z"
  }
}
```

`nodeCount`, `edgeCount`, and `freshness` are additive metadata. Older clients
may ignore them. Agents should use `freshness.ready`, `freshness.lastRefreshAt`,
and graph counts to decide whether to retry, warn the user, or proceed with
best-effort reasoning.

Server error responses keep the historical `error` string and add structured
fields:

```json
{
  "error": "entity not found",
  "message": "entity not found",
  "code": "not_found",
  "status": 404,
  "retryable": false,
  "source": "server"
}
```

The CLI can emit this shape directly on stderr for server query failures with
`--machine-errors`.

## Stability levels

Each part of the output falls into one of two buckets:

### A. Phase-1 guaranteed structure
These fields and semantics are expected to remain stable for AI consumers.

#### Nodes
Every node is expected to provide:
- `canonicalId` (required)
- `kind` (required)
- `sourceKind` (best-effort but usually present)
- `name` (best-effort but usually present)
- `namespace` for namespaced resources
- `attributes` for resource-specific details

#### Edges
Every edge is expected to provide:
- `from`
- `to`
- `kind`
- `provenance.sourceType`
- `provenance.state`
- `provenance.resolver`

#### Provenance semantics
AI-Agent logic may rely on the following meaning:
- `explicit_ref` = directly asserted from Kubernetes object fields
- `selector_match` = recovered by selector/label matching
- `owner_reference` = recovered from ownerReferences chain
- `binding_resolution` = recovered via binding-style resolution
- `inference_rule` = higher-level inferred relation
- `observed` = runtime-observed or event-style evidence

And:
- `asserted` = explicit fact from source objects
- `inferred` = derived by graph logic
- `observed` = seen from runtime/observability evidence

### B. Best-effort diagnostic evidence
These are intentionally useful but not guaranteed complete:
- `explanation`
- event-derived messages
- missing-agent evidence
- CSI correlation evidence when cluster state is partial or inconsistent
- any graph slice that depends on current depth policy

Agents should use these for explanation and ranking, but not as the sole source of truth.

## Canonical ID contract

AI-Agent consumers should treat `canonicalId` as the stable identity key.

Backend-specific storage IDs must never be used in downstream logic.

### Required AI behavior
Agents should:
- index nodes by `canonicalId`
- join edges via `from` / `to` using `canonicalId`
- avoid depending on node ordering in `nodes`
- avoid depending on edge ordering in `edges`

## Phase-1 guaranteed edge semantics

The following edge kinds are intended as stable phase-1 diagnostic semantics:

### Ownership / workload
- `managed_by`
- `owns_pod`
- `controlled_by`
- `managed_by_controller`
- `served_by_node_daemon`

### Scheduling / service / config
- `scheduled_on`
- `selects_pod`
- `uses_config_map`
- `uses_secret`
- `uses_service_account`

### Storage
- `mounts_pvc`
- `bound_to_pv`
- `uses_storage_class`
- `provisioned_by_csi_driver`
- `implemented_by_csi_controller`
- `implemented_by_csi_node_agent`

### Image / runtime
- `uses_image`

### Events / webhook / storage infra
- `reported_by_event`
- `affected_by_webhook`
- `managed_by_csi_controller`
- `served_by_csi_node_agent`

AI-Agent code should assume these meanings are stable, even if the set of returned nodes varies by policy.

## What AI-Agent code should treat as optional

Do not hard-require:
- any specific explanation string
- any specific Event node count
- any specific CSI node-agent edge count
- node presence beyond the selected traversal policy
- cluster-wide completeness

Examples:
- if `managed_by_csi_controller` exists, use it
- if `served_by_csi_node_agent` is absent, check explanation for missing-agent evidence before concluding no CSI path exists

## Policy semantics

### `maxDepth`
General traversal budget for normal topology.

### `storageMaxDepth`
Additional budget for storage / CSI traversal.

This means a response may go deeper along storage edges than along unrelated topology edges.

Agents should not assume a single uniform BFS depth across all returned paths.

### `terminalNodeKinds`
Boundary node kinds that are included in the response but are not used as
another traversal frontier.

Default boundary kinds keep shared/evidence nodes from exploding pod-centered
diagnostics. Examples include `ServiceAccount`, `ConfigMap`, `Secret`, `Node`,
`Service`, `Event`, `Image`, `OCIArtifactMetadata`, `WebhookConfig`,
`RoleBinding`, and `ClusterRoleBinding`.

### `expandTerminalNodes`
When true, the diagnostic query traverses through boundary nodes. This is useful
for deliberate fan-out investigations, but clients should expect larger graphs.

## Recommended downstream consumption pattern

### Step 1: Build indices
- nodeByID
- outgoing edges by node ID
- incoming edges by node ID
- edges grouped by `kind`

### Step 2: Prioritize asserted facts
When multiple explanations conflict, prefer edges with:
1. `state=asserted`
2. `state=observed`
3. `state=inferred`

### Step 3: Use inferred relations as shortcuts, not sole truth
For example:
- `managed_by` is useful as a stable semantic shortcut
- but the agent may still inspect the workload node and relevant events to validate context

### Step 4: Use explanation for narrative, not identity
`explanation` is for:
- summarization
- suspicion ranking
- helpful human-readable narration

It should not be used as the only signal for graph facts already present in nodes/edges.

## Phase-1 examples of valid AI conclusions

### Safe conclusions
- this pod mounts PVC X, which is bound to PV Y
- this PV uses CSI driver `local.csi.aliyun.com`
- this PV is managed by open-local controller components
- no open-local node agent was found for the PV affinity node in the current observed graph slice
- this workload owns the pod through the owner chain

### Unsafe conclusions
- no CSI agent exists anywhere in the cluster
- no controller exists for this driver
- this workload definitely has no events, when `IncludeEvents=false`
- the graph is complete for the entire namespace or cluster

## Current known limitations relevant to AI consumers

- `explanation` content is best-effort and not yet ranked
- event summarization is still shallow
- traversal policy can hide otherwise valid graph facts if policy is too narrow
- CSI correlation currently includes only the built-in open-local resolver
- output may include multiple controller components for the same CSI system

## Recommendation for downstream agent implementation

Treat the diagnostic subgraph as a **bounded, provenance-aware evidence graph**.

Good agent behavior:
- prefer daemon-backed read-only queries with `--server`
- use `--status`, `--resolve-entity`, `--expand-entity`, `--diagnose-pod`, `--diagnose-workload`, and `--list-filtered-relations` for common flows
- use edges and node attributes for hard reasoning
- use explanation for soft reasoning and user-facing summary
- detect missing evidence explicitly instead of assuming absence means non-existence
- keep backend assumptions out of downstream code

## Common daemon-backed flows

Status:

```bash
./bin/kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

Resolve an entity and capture `entity.entityGlobalId`:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name my-pod
```

Expand one entity:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --expand-entity \
  --entity-id 'your/entityGlobalId' \
  --expand-depth 1
```

Diagnose a pod or workload:

```bash
./bin/kubernetes-ontology --server "http://127.0.0.1:18080" --diagnose-pod --namespace default --name my-pod
./bin/kubernetes-ontology --server "http://127.0.0.1:18080" --diagnose-workload --namespace default --name my-deployment
```

List filtered relations:

```bash
./bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-filtered-relations \
  --from 'your/entityGlobalId' \
  --relation-kind scheduled_on \
  --limit 50
```

## Contract evolution rule

Post-MVP backend changes must preserve:
- `canonicalId`
- stable edge semantics
- provenance meaning
- policy meaning
- top-level response shape

If any of those change, downstream AI consumers should treat it as a contract version change.
