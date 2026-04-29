# Open Source Diagnostics Evolution Plan

## Status

Autoplan-reviewed draft on `main` after syncing `origin/main` to commit
`a912fbd`.

Codex CLI outside voice was unavailable because local policy blocked sending
repo context to the external Codex service. The review continued in
`subagent-only` mode.

## North Star

Build a read-only provenance graph that answers:

> Why is this Kubernetes object failing, and who owns the blast radius?

This is deliberately narrower than "another Kubernetes dashboard." The durable
open-source artifact is the provenance-rich JSON/CLI/HTTP contract. The viewer
is a reference investigation workspace. The skill is an onboarding and operating
wrapper around the same docs and CLI recipes.

## User Promise

When `kubectl` turns into a 12-command scavenger hunt, `kubernetes-ontology`
returns a bounded evidence graph:

- the failing object
- directly related resources
- inferred ownership and routing edges
- Events and runtime symptoms
- Helm/package ownership when evidence is available
- freshness and graph-size metadata
- safe conclusions and explicit unknowns
- a viewer URL for human inspection

No writes to observed Kubernetes resources. No controller. No CRD. No persistent
database in this phase.

## Premises

| Premise | Decision | Rationale |
| --- | --- | --- |
| Keep backend in memory | Accepted | This keeps setup lightweight and matches the current architecture. The plan adds scale budgets before expanding graph scope. |
| Continue toward open source quality | Accepted | The project already has release packaging, Helm chart, skill, docs, samples, and safety messaging. |
| Help fault diagnosis and topology analysis | Accepted with focus | The wedge is incident evidence, not general inventory or observability. |
| Include resource-to-Helm-chart relationships | Accepted with confidence tiers | Helm labels are recommended, not guaranteed. Exact ownership should use read-only release manifest evidence when available. |
| Focus on skills, docs, and visualization | Accepted | These are adoption surfaces, but docs and machine-readable contracts remain source of truth. |

## What Already Exists

| Sub-problem | Existing code/docs | Reuse decision |
| --- | --- | --- |
| In-memory graph kernel | `internal/store/memory`, `internal/graph/kernel.go` | Reuse. Add budgets and graph-size warnings before adding broad query surfaces. |
| Canonical node/edge model | `internal/model`, `internal/api/types.go`, `AI_CONTRACT.md` | Reuse. Add Helm/package nodes and provenance fields additively. |
| Kubernetes collection | `internal/collect/k8s` | Reuse. Extend normalized metadata and Events rather than adding a parallel collector. |
| Relation recovery | `internal/graph/builder.go`, `internal/resolve/**/*` | Reuse. Add Helm/package resolver beside existing owner/selector/RBAC/CSI resolvers. |
| Diagnostic traversal | `internal/query/facade.go`, `internal/service/diagnostic` | Reuse. Productize recipes before adding endpoint families. |
| Storage/CSI graph | Latest `main` already includes PVC/PV/StorageClass/CSIDriver diagnostics in viewer | Treat as an existing showcase path, not new scope. |
| Viewer | `tools/visualize/index.html`, `tools/visualize/server.py` | Keep as static reference workspace. Improve evidence lanes and shareable state. |
| Skill | `skills/kubernetes-ontology-access` | Keep thin. Generate/test it from docs and CLI recipes where possible. |
| Docs | `README.md`, `README.zh-CN.md`, `QUICKSTART.md`, `AI_CONTRACT.md` | Make docs the primary product surface, with the skill as wrapper. |

## Landscape Check

The project should not compete head-on with:

- Headlamp: Kubernetes SIG UI, extensible UI and plugin ecosystem.
- Kubernetes Dashboard / K9s / Lens: resource management and day-to-day cluster
  inspection.
- KubeView: read-only topology visualization.
- Pixie / Grafana: telemetry-heavy observability.
- Robusta / Komodor: incident workflows and commercial root-cause analysis.

The differentiation is the contract:

```text
Kubernetes object -> bounded evidence graph -> provenance -> safe AI/human reasoning
```

That contract should be embeddable by CLIs, agents, plugins, docs, and the
reference viewer.

## Dream State Delta

```text
CURRENT STATE
  Read-only in-memory graph service with Pod/Workload and storage-capable
  diagnostic traversal, CLI, HTTP API, Helm chart, static viewer, and skill.

THIS PLAN
  Proves two showcase incident flows, adds Helm/package provenance, adds
  evidence ranking primitives, improves viewer investigation lanes, and turns
  docs/skill into tested recipes.

12-MONTH IDEAL
  A trusted Kubernetes incident context pack used by humans and agents. Other
  tools can embed its JSON contract or link into its viewer. Persistent backends
  remain optional, not required for first value.
```

## Showcase Flows

### Flow 1: Failing Pod To Helm Release/Chart

Question:

> This Pod is failing. Which Helm release/chart probably owns it, what evidence
> supports that, and what else in the release is in the blast radius?

Desired answer:

- Pod and owner workload
- Helm release and chart evidence
- related resources in the same release
- recent warning Events near the Pod/workload/release
- confidence tier for ownership
- conflicts or unknowns surfaced explicitly

### Flow 2: Failing Pod To Storage/CSI Cause

Question:

> This Pod is stuck. Is storage or CSI part of the cause?

Desired answer:

- Pod, PVC, PV, StorageClass, CSIDriver
- CSI controller and node-agent evidence when configured
- Events and missing-agent evidence
- bounded graph that avoids namespace-wide fan-out
- viewer lane focused on storage/CSI

## Implementation Alternatives

| Approach | Summary | Effort | Risk | Pros | Cons | Decision |
| --- | --- | --- | --- | --- | --- | --- |
| A. Standalone dashboard | Build a richer UI around topology and diagnostics | L | High | Easy to demo | Competes with Headlamp/KubeView/Lens; high maintenance | Reject |
| B. Provenance contract first | Strengthen JSON/CLI/HTTP contract, keep viewer as reference | M | Medium | Differentiated, agent-friendly, embeddable | Requires discipline to avoid UI creep | Choose |
| C. Helm plugin/integration first | Build Headlamp/K9s/Backstage integration before core contract | M | Medium | Faster distribution if partner ecosystem works | Premature until contract and recipes are stable | Defer |

## Product Scope

### In Scope

1. Helm/package provenance.
2. Evidence primitives and deterministic ranking.
3. Diagnostic recipes for Pod, Workload, Storage/CSI, HelmOwnership,
   ServiceRouting, Identity, and NodeContext.
4. Viewer investigation lanes for Workload, Network, Config, Identity, Storage,
   Runtime, Events, and Helm.
5. JSON schema and golden diagnostic outputs.
6. Skill v2 as a thin wrapper around recipes.
7. English and Chinese docs for the core workflows.
8. In-memory scale budgets and degradation behavior.

### NOT In Scope

| Item | Rationale |
| --- | --- |
| Persistent graph backend | Not needed to prove first diagnostic value. |
| Public multi-tenant service | Requires auth, tenancy, rate limits, and hardening outside this phase. |
| Automatic remediation | Violates the read-only trust promise. |
| Observed-workload CRDs/controllers | Adds install friction and mutating footprint. |
| Full Helm history reconstruction | Current release ownership is enough; history can come later. |
| NodePressure telemetry RCA | Without metrics and kubelet history, this would be inferior to observability tools. Use NodeContext only. |
| Standalone dashboard parity | The viewer is a reference workspace, not a Headlamp/Lens competitor. |

Deferred items are captured in `TODOS.md`.

## Helm And Package Provenance

### Evidence Tiers

| Tier | Source | Meaning | Edge state |
| --- | --- | --- | --- |
| Exact | Current Helm release Secret manifest, when readable | This object appears in the current release manifest | `asserted` |
| Strong | Complete Helm recommended labels match namespace + instance + chart | Strong release/chart hint | `inferred` with high confidence |
| Weak | Application/component labels only | App grouping hint, not release ownership | `inferred` with low confidence |
| Conflict | Labels or manifest evidence disagree | Do not collapse into one owner | explicit conflict evidence |

### New Concepts

- `HelmRelease`
- `HelmChart`
- `Application`
- `Component`
- `managed_by_helm_release`
- `installs_chart`
- `part_of_application`
- `has_component`

### Resolver Rules

- Prefer exact manifest membership when Secret reads are enabled and Helm
  release metadata is available.
- Use labels only as evidence, not ground truth.
- Namespace-scope release identity.
- Preserve conflicts rather than selecting a winner.
- Add confidence and resolver names to all inferred edges.

## Diagnostic Recipes

Do not add new HTTP endpoint families unless the output contract differs from
generic `/diagnostic`.

Each recipe must define:

- user question
- supported entry kinds
- default policy
- stop conditions
- ranked evidence fields
- expected graph size envelope
- safe conclusions
- unsafe conclusions
- CLI example
- HTTP example
- viewer URL example
- golden fixture

| Recipe | Question | Status |
| --- | --- | --- |
| PodIncident | Why is this Pod failing? | Existing path, expand evidence. |
| WorkloadIncident | Why is this workload unhealthy or blocked? | Existing path, expand evidence. |
| StorageCSI | Is storage/CSI involved? | Mostly exists on latest main; productize docs/goldens. |
| HelmOwnership | What release/chart owns this object and blast radius? | New. |
| ServiceRouting | Does Service selector/routing explain impact? | Recipe first. |
| Identity | What ServiceAccount/RBAC evidence matters? | Recipe first. |
| NodeContext | What topology surrounds this node? | Rename from NodePressure and avoid telemetry claims. |

## Evidence Primitives

Before ranking, collect the data ranking needs:

- Event type
- Event count
- first timestamp
- last timestamp
- reporting controller/source
- involved object fallback by kind/namespace/name when UID is missing
- Pod container waiting/terminated states
- restart counts
- Pod conditions
- Node conditions
- object generation/resourceVersion where useful
- bounded in-memory change ring per entity

Ranking stays additive:

```json
{
  "rankedEvidence": [
    {
      "entity": "...",
      "kind": "Event",
      "symptom": "FailedMount",
      "severity": "warning",
      "proximity": 1,
      "recency": "2026-04-29T05:00:00Z",
      "confidence": 0.92
    }
  ]
}
```

Raw nodes and edges remain the source of truth.

## In-Memory Scale Budget

Release-blocking defaults:

- examples default to scoped namespaces, not all namespaces
- status exposes object, node, edge, and retained-event counts
- warn when graph exceeds configured budget
- cap retained Events and change-ring entries per entity
- query latency target: p95 under 1s for documented small/medium envelopes
- memory target documented for synthetic clusters
- large-cluster behavior returns partial/degraded metadata instead of hanging

Synthetic scale tests should cover at least:

- 1k Pods
- 10k Pods
- high Event churn
- large Helm release
- namespace-scoped and all-namespace collection

### Diagnostic Budget Object

Depth is not enough. Every diagnostic recipe must carry an explicit budget:

```json
{
  "maxDepth": 2,
  "storageMaxDepth": 5,
  "maxNodes": 250,
  "maxEdges": 500,
  "maxEventsPerEntity": 20,
  "maxFanoutPerLane": {
    "Events": 50,
    "Helm": 100,
    "Identity": 80,
    "Storage": 80
  },
  "timeoutMs": 25000
}
```

When a budget is hit, responses must include:

```json
{
  "partial": true,
  "truncationReasons": [
    {"lane": "Events", "reason": "maxEventsPerEntity", "limit": 20}
  ]
}
```

### Partial RBAC Contract

Collection should become source-scoped instead of all-or-nothing.

If a resource source is forbidden or unavailable:

- continue collecting other safe sources
- record `degradedSources`
- surface exact evidence that is unavailable
- avoid false negative conclusions
- expose the warning in `/status`, diagnostic responses, CLI output, and viewer

Example:

```json
{
  "degradedSources": [
    {
      "source": "helm.releaseSecrets",
      "namespace": "payments",
      "reason": "forbidden",
      "impact": "exact Helm ownership unavailable; label evidence only"
    }
  ]
}
```

### Snapshot Indexing

Before adding Helm fan-out or scale tests, build reusable snapshot indexes:

- namespace/name
- UID
- pod labels by namespace and key/value
- Service selectors by namespace
- PVC by namespace/name
- PVC by bound PV
- Pods by PVC ref
- Pods by node name
- Helm label tuple: namespace, instance, chart

## Viewer Direction

The viewer is an investigation workspace for the contract, not the product
center.

First-screen hierarchy:

1. Incident header: recipe, entry object, namespace, freshness, degraded/partial
   status, node/edge count.
2. Ranked evidence and timeline: what likely matters first.
3. Focused graph: entry node and only the lanes relevant to the recipe.
4. Selection details: provenance, raw JSON, connected relations.
5. Advanced controls: server/file loading, filters, layout, expansion controls.

Add:

- entry-node focus
- breadcrumb trail
- lane grouping: Workload, Network, Config, Identity, Storage, Runtime, Events,
  Helm
- evidence timeline
- relation provenance legend
- confidence/conflict badges
- shareable query-state URLs
- copy CLI command and export JSON actions
- embedded mode suitable for plugins/docs

Avoid:

- dashboard card mosaics
- cluster management actions
- broad metrics panels
- competing with Headlamp or Grafana

### Viewer Lane Taxonomy

| Lane | Purpose | Node kinds | Relation kinds | Empty state | Conflict behavior |
| --- | --- | --- | --- | --- | --- |
| Workload | Explain ownership and rollout context | Workload, Pod | `managed_by`, `owns_pod`, `controlled_by` | "No owner chain found" | show ambiguous owner edges |
| Network | Explain Service-to-Pod reachability | Service, Pod | `selects_pod`, `selected_by_service` | "No Service selector evidence" | show selector mismatch |
| Config | Explain mounted or env-sourced config | ConfigMap, Secret, Pod | `uses_config_map`, `uses_secret` | "No config dependency evidence" | show forbidden Secret read |
| Identity | Explain ServiceAccount and RBAC bindings | ServiceAccount, RoleBinding, ClusterRoleBinding | `uses_service_account`, `bound_by_role_binding` | "No RBAC binding evidence in graph" | show missing/forbidden RBAC evidence |
| Storage | Explain PVC/PV/StorageClass/CSI chain | PVC, PV, StorageClass, CSIDriver, Pod | `mounts_pvc`, `bound_to_pv`, `uses_storage_class`, `provisioned_by_csi_driver`, CSI edges | "No storage path from entry" | show missing CSI agent/controller |
| Runtime | Explain node/image/runtime attachment | Node, Image, OCIArtifactMetadata, Pod | `scheduled_on`, `uses_image`, `has_oci_artifact` | "No runtime evidence" | show image parse/metadata unknown |
| Events | Explain time-ordered symptoms | Event plus target node | `reported_by_event`, `affected_by_webhook` | "No warning Events collected" | group duplicate events |
| Helm | Explain package ownership and blast radius | HelmRelease, HelmChart, Application, Component, owned resources | `managed_by_helm_release`, `installs_chart`, app/component edges | "No Helm ownership evidence" | show confidence tier and conflicts |

Recipe defaults:

- PodIncident: Workload, Events, Runtime, Config, Identity, Storage, Helm.
- StorageCSI: Storage, Events, Runtime, Workload.
- HelmOwnership: Helm, Workload, Events, Config, Storage.
- ServiceRouting: Network, Workload, Events, Helm.
- Identity: Identity, Workload, Events.
- NodeContext: Runtime, Storage, Events, Workload.

### Viewer State Matrix

| Backend state | UI treatment | User action |
| --- | --- | --- |
| loading | canvas overlay with requested recipe and entry | wait/cancel |
| empty graph | incident header plus empty evidence panel | check entry kind/name/namespace |
| stale runtime | yellow global banner with last refresh time | refresh or proceed best-effort |
| partial graph over budget | yellow global banner and lane-level partial badges | narrow namespace/depth |
| forbidden Helm Secret | Helm lane warning, label evidence still shown | enable Secret read or accept lower confidence |
| low-confidence only | confidence badge on summary and lane | inspect raw evidence |
| conflict evidence | red conflict badge, no single owner claim | inspect conflicting candidates |
| query timeout | error panel with retry and copy CLI command | retry with lower depth |
| file-loaded static graph | "exported snapshot" banner, no live refresh | reload from daemon if needed |

### Shareable URL Contract

The viewer should preserve:

- `mode=live|file|embedded`
- `recipe`
- `kind`
- `namespace`
- `name`
- `server` or `file`
- `maxDepth`
- `storageMaxDepth`
- `terminalKinds`
- `focusNode`
- `selectedEvidence`
- `lanes`
- `filters`
- `layout`
- `collectedAt`

Security rules:

- default upstream server must be localhost or loopback
- arbitrary upstream URL loading requires an explicit server flag
- file loading is disabled in embedded/read-only shared modes
- local file loading is confined to allowed roots when enabled
- `server` and `file` parameters are stripped from embedded docs/plugin links

Example:

```text
/?mode=live&recipe=PodIncident&kind=Pod&namespace=default&name=api-0&lanes=Workload,Events,Helm&focusNode=...
```

### Evidence Timeline Contract

Timeline rows:

| Field | Meaning |
| --- | --- |
| timestamp | last observed time if known, else collected time |
| severity | warning/error/info/unknown |
| symptom | normalized symptom tag |
| proximity | graph hops from entry |
| count | rolled-up repeated Events |
| confidence | exact/strong/weak/conflict/unknown |
| source | Event, status, resolver, runtime change |
| whyRanked | short deterministic explanation |

Sort order:

1. higher severity
2. closer proximity
3. newer last observed time
4. higher confidence

Repeated Events roll up by involved object, reason, message, and reporting
source, while preserving the latest timestamp and count.

### Viewer Modes

| Mode | Use | Controls |
| --- | --- | --- |
| Full workspace | Local operator investigating live daemon | all controls |
| Read-only shared URL | teammate opens a focused graph | no server mutation, filters allowed |
| Embedded docs/plugin frame | external docs/plugin embeds graph | no server input, no raw file path field, export allowed |
| Exported JSON inspection | static handoff from CLI/agent | no live refresh, raw JSON and copy command enabled |

## Design Review Summary

Initial design completeness: 5/10.

After adding first-screen hierarchy, lane taxonomy, degraded-state matrix,
shareable URL contract, evidence timeline model, and viewer modes: 8/10.

Remaining design risks:

- No rendered mockup has been generated in this plan file.
- Mobile layout remains unspecified because the current viewer is desktop-first
  diagnostic tooling.
- Final visual QA should run after implementation with the actual viewer.

Design dual voices:

- Subagent found 8 issues: inverted first-screen hierarchy, underspecified
  lanes, missing degraded states, weak handoff URL contract, evidence inspection
  buried behind graph selection, ambiguous confidence visuals, missing timeline
  contract, and vague embedded mode.
- Codex voice unavailable due external-service policy.

Design consensus: subagent-only, but all findings were actionable and accepted.

## Skill And Documentation Direction

Docs and schema are source of truth.

The skill should:

- detect setup state
- choose binary, Helm, or source path
- state the read-only boundary once
- ask before `helm upgrade --install`
- run status checks
- run documented diagnostic recipes
- open/provide viewer URL
- summarize using AI-contract-safe language
- produce cleanup commands

The skill must not contain product logic that is absent from docs, JSON schema,
CLI examples, or golden outputs.

## Developer Experience Plan

### Primary Persona

Platform engineer or SRE evaluating an OSS diagnostic helper during an incident
or post-incident review.

Context:

- already knows `kubectl`, Helm, and basic Kubernetes resources
- will not tolerate mutating behavior or unclear RBAC
- wants value in under five minutes
- wants JSON/CLI output that can be pasted into an issue, incident doc, or agent

### Time To Hello World Target

Target: under 5 minutes.

Champion path:

```bash
export KO_VERSION=v0.2.0
helm upgrade --install kubernetes-ontology oci://ghcr.io/colvin-y/charts/kubernetes-ontology \
  --version "${KO_VERSION}" \
  --namespace kubernetes-ontology \
  --create-namespace \
  --set cluster="demo" \
  --set contextNamespaces='{default}'
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology 18080:18080
kubernetes-ontology --server http://127.0.0.1:18080 recipe pod-incident --namespace default --name failing-pod
```

Release binary path remains the best private-cluster/no-in-cluster-footprint
path.

### DX Decisions

| Issue | Decision |
| --- | --- |
| Helm install currently requires repo checkout | Publish chart as OCI or release `.tgz`; make zero-clone install primary. |
| Secret reads default to enabled | Change planned default to `rbac.readSecrets=false`; make Helm exact evidence opt-in. |
| Skill tracks `main`, runtime tracks tags | Skill becomes version-aware bootstrapper or gets tagged/tested per runtime release. |
| Recipe names differ across CLI/API/viewer/docs | Define one recipe vocabulary and keep old flags/routes as aliases. |
| Errors lack next operator action | Add `nextCommand` or `nextActions` to CLI/server/viewer where useful. |
| Upgrade docs are maintainer-focused | Add `UPGRADING.md` for users. |
| Contributor path needs a cluster too early | Add `CONTRIBUTING.md`, `make demo`, and `make smoke` against sample graph/fake server. |

### Recipe Vocabulary

Canonical names:

- `pod-incident`
- `workload-incident`
- `storage-csi`
- `helm-ownership`
- `service-routing`
- `identity`
- `node-context`

Compatibility:

- keep `--diagnose-pod` as alias for `recipe pod-incident`
- keep `/diagnostic/pod` as alias for `/diagnostic?recipe=pod-incident`
- viewer accepts both `kind=Pod` and `recipe=pod-incident`, but generates URLs
  with recipe names

### Developer Journey Map

| Stage | Developer does | Current friction | Plan fix |
| --- | --- | --- | --- |
| Discover | Opens README | Product value is clear, but install paths are long | Put one zero-clone path first |
| Install | Chooses Helm or binary | Helm path requires checkout | Publish OCI/release chart |
| First run | Runs status | Needs CLI binary plus port-forward | Add copy-paste recipe with expected output |
| Diagnose | Runs PodIncident | Recipe vocabulary not unified | Add canonical recipe commands |
| Inspect | Opens viewer URL | URL state contract incomplete | Add recipe/focus/evidence URL contract |
| Debug | Hits RBAC/partial data | Errors not action-oriented enough | Add degradedSources and next actions |
| Upgrade | Moves versions | No user upgrade guide | Add `UPGRADING.md` |
| Contribute | Runs locally | Needs cluster too soon | Add sample/fake-server demo |

### First-Time Developer Narrative

I open the README because I have a stuck Pod and want to know whether this tool
can explain ownership and blast radius. I see the safety boundary, which matters.
Then I hit a split: binary path, Helm path, source path, skill path. If I choose
Helm today, the command uses `./charts/kubernetes-ontology`, so I need a repo
checkout before I can evaluate the product. That breaks the five-minute test.

The better path is: install chart from a release location, port-forward, run one
recipe command, get JSON plus a viewer URL. If Secret access is off by default
and the output says "exact Helm evidence unavailable, label evidence only," I
trust it more, not less. Honest unknowns beat magic.

### DX Scorecard

| Dimension | Current | Target | Notes |
| --- | --- | --- | --- |
| Getting Started | 6/10 | 9/10 | Needs zero-clone Helm and expected output. |
| CLI/API Naming | 6/10 | 9/10 | Needs recipe vocabulary across surfaces. |
| Error Messages | 6/10 | 9/10 | Needs degraded sources, truncation reasons, next actions. |
| Documentation | 7/10 | 9/10 | Good base; needs recipes and user upgrade docs. |
| Upgrade Path | 4/10 | 8/10 | Add `UPGRADING.md` and compatibility policy. |
| Dev Environment | 6/10 | 8/10 | Add fake-server/sample graph demo. |
| Community/OSS | 6/10 | 8/10 | Add `CONTRIBUTING.md`, issues, golden corpus. |
| DX Measurement | 3/10 | 8/10 | Track TTHW and recipe success through docs/tests. |

Overall DX: 5.5/10 current, 8.5/10 after planned fixes.

### DX Implementation Checklist

- [ ] Publish Helm chart as OCI or release `.tgz`.
- [ ] Make Secret reading default off.
- [ ] Add opt-in Helm exact evidence docs and chart value.
- [ ] Add canonical recipe CLI/API/viewer vocabulary.
- [ ] Add expected JSON snippets for each recipe.
- [ ] Add `UPGRADING.md`.
- [ ] Add `CONTRIBUTING.md`.
- [ ] Add `make demo` using sample graph or fake server.
- [ ] Add `make smoke` for build + sample viewer + key CLI paths.
- [ ] Test skill recipes against the same commands as docs.
- [ ] Document version compatibility between CLI, daemon, chart, viewer, and skill.

DX dual voices:

- Subagent found 7 issues.
- Codex voice unavailable due external-service policy.

DX consensus: subagent-only, all findings accepted.

## Output Contract Additions

All new fields are additive:

```json
{
  "warnings": [],
  "partial": false,
  "degradedSources": [],
  "budgets": {
    "maxNodes": 250,
    "maxEdges": 500,
    "nodesReturned": 42,
    "edgesReturned": 81
  },
  "rankedEvidence": [],
  "conflicts": []
}
```

`explanation` remains human-readable. Machine consumers should use structured
fields.

## Architecture Sketch

```text
Kubernetes API
  |
  v
Read-only collectors and informers
  |
  v
Normalized resource facts
  |  labels, annotations, owner refs, specs, status, events
  v
Relation resolvers
  |-- explicit refs
  |-- selector matches
  |-- owner chains
  |-- RBAC bindings
  |-- CSI inference
  |-- Helm/package evidence
  v
In-memory graph kernel
  |
  +--> generic entity/relation/expand API
  +--> diagnostic recipes
  +--> ranked evidence
  +--> JSON schema and golden outputs
  +--> CLI
  +--> static reference viewer
  +--> agent skill wrapper
```

## Data Flow With Shadow Paths

```text
Kubernetes object/event
  |
  +--> nil / missing fields -----------> preserve unknown, do not infer
  +--> malformed or forbidden source --> degraded status + explicit warning
  +--> huge fan-out -------------------> budget cap + partial flag
  v
Normalize
  |
  +--> unsupported kind ---------------> skip with collector evidence
  v
Resolve relations
  |
  +--> conflicting Helm evidence ------> conflict node/evidence, no winner
  +--> weak labels only ---------------> low-confidence inferred edge
  v
In-memory kernel
  |
  +--> over budget --------------------> warn, cap retained evidence
  v
Diagnostic recipe
  |
  +--> no entry found -----------------> machine-readable not_found
  +--> stale runtime ------------------> freshness warning
  v
CLI / HTTP / Viewer / Skill
```

## Error And Rescue Registry

| Failure | Detection | Rescue | User sees | Test |
| --- | --- | --- | --- | --- |
| Helm Secret read forbidden | Kubernetes API forbidden | Fall back to label evidence, mark exact evidence unavailable | Warning in status/explanation | Collector/reconciler test |
| Helm labels missing | Resolver sees incomplete label set | Do not infer release edge | Unknown ownership, not false negative claim | Resolver table test |
| Helm labels conflict | Multiple candidate releases/charts | Emit conflict evidence | Conflict badge and JSON field | Resolver conflict test |
| Event UID missing | Event has kind/name but no UID | fallback match by kind/namespace/name with lower confidence | Lower-confidence event edge | Event normalization test |
| Graph over budget | object/node/edge count exceeds limit | cap retained evidence, return partial metadata | Degraded freshness/status | Scale test |
| Query timeout | context deadline | abort and return retryable timeout | HTTP 504 / CLI machine error | Server test |
| Viewer load failure | fetch timeout or invalid JSON | show actionable error and keep prior graph | Error panel | Viewer test |
| Skill command fails | non-zero exit | stop, show command, cause, next check | Plain-language recovery step | Skill recipe test |

## Failure Modes Registry

| Codepath | Failure mode | Rescued? | Test? | User sees? | Logged? |
| --- | --- | --- | --- | --- | --- |
| Helm resolver | false ownership from labels | Planned | Planned | confidence/conflict | yes |
| Event ranking | stale Event outranks current symptom | Planned | Planned | ranked evidence reason | yes |
| In-memory kernel | large cluster OOM/stall | Planned | Planned | budget warning/degraded | yes |
| Diagnostic recipe | graph fan-out too large | Planned | Planned | partial graph metadata | yes |
| Viewer | evidence lanes hide key node | Planned | Planned | filter/breadcrumb reset | n/a |
| Skill | stale docs command | Planned | Planned | failing command + fix | n/a |

Critical gaps before implementation:

- no scale budget exists yet
- Event model lacks ranking primitives
- Helm ownership model is not implemented
- docs/skill recipes are not generated/tested from one source

## Test Diagram

```text
CODE PATHS                                           COVERAGE NEEDED

Helm metadata collection
  ├── release Secret readable                         unit + integration fake client
  ├── release Secret forbidden                        unit
  ├── labels complete                                 resolver table
  ├── labels missing                                  resolver table
  └── labels conflicting                              resolver table

Evidence primitives
  ├── Event type/count/timestamps/source              normalization unit
  ├── Event no UID fallback                           normalization + resolver
  ├── Pod container waiting/terminated                normalization unit
  └── bounded change ring                             runtime unit

Diagnostic recipes
  ├── Pod -> Helm release/chart                       golden fixture
  ├── Pod -> Storage/CSI cause                        golden fixture
  ├── ServiceRouting                                  golden fixture
  ├── Identity                                        golden fixture
  └── NodeContext                                     golden fixture

Viewer
  ├── lane grouping                                   visual/check script
  ├── conflict/confidence badge                       viewer fixture
  ├── shareable URL state                             browser/server test
  └── export/copy command                             browser/server test

Skill/docs
  ├── binary path recipe                              scripted fixture
  ├── Helm path recipe                                dry-run/text fixture
  ├── diagnostic command recipes                      docs lint/golden
  └── cleanup commands                                docs lint

Release-blocking additions:

- partial RBAC matrix tests
- diagnostic budget truncation tests
- full-rebuild vs narrow-reconcile equivalence for Helm/evidence edges
- query concurrency/race tests
- viewer SSRF and local-file restriction tests
- malformed and oversized Helm Secret decoding tests
- deterministic ranked evidence ordering tests
```

## Acceptance Criteria

- A user can run a Pod diagnostic and see its probable Helm release/chart with
  confidence and conflicts.
- A user can run the storage/CSI showcase flow on latest `main` behavior and get
  a bounded graph, explanation, and viewer handoff.
- All new Helm/package facts include provenance and confidence.
- Ranking output is additive and deterministic.
- Docs and skill both point to the same recipe commands.
- Viewer shows lanes, evidence timeline, and provenance/confidence without
  becoming a management dashboard.
- Large graph behavior is bounded and documented.
- No observed Kubernetes resource is mutated.
- `go test -p 1 ./...`, `make visualize-check`, and release packaging checks
  pass before ship.

## Workstreams

| Lane | Scope | Modules | Depends on |
| --- | --- | --- | --- |
| A | Partial collection and output contract | `internal/collect/k8s`, `internal/query`, `internal/server`, `internal/api` | none |
| B | Diagnostic budgets and query lock reduction | `internal/service/diagnostic`, `internal/runtime`, `internal/store/memory` | A contract shape |
| C | Snapshot indexes | `internal/graph`, `internal/collect/k8s/resources` | none |
| D | Helm/package provenance | `internal/collect/k8s`, `internal/resolve/infer`, `internal/graph`, `internal/model`, `internal/api`, `docs/ontology` | A + C |
| E | Evidence primitives and ranking | `internal/collect/k8s/resources`, `internal/query`, `internal/service/diagnostic`, fixtures | A |
| F | Recipes and golden corpus | `samples`, `internal/fixtures`, `README`, `AI_CONTRACT`, `QUICKSTART` | D + E for final goldens |
| G | Viewer investigation lanes and security boundaries | `tools/visualize`, viewer tests | A + B schema shape |
| H | Skill and docs wrapper | `skills/kubernetes-ontology-access`, docs | F |

Parallelization:

- Launch A, B, C in parallel.
- Start D after C gives indexes and A gives degraded-source shape.
- E can start after A.
- G can use fixture JSON once A/B shape is agreed.
- H waits for F.

## Engineering Review Summary

Initial engineering completeness: 6/10.

After review additions: 8/10. The plan now makes partial RBAC, diagnostic
budgets, stale streaming updates, algorithmic indexing, structured output,
viewer local security, and query concurrency explicit engineering scope.

Architecture graph:

```text
Collector
  |-- source results: ok / forbidden / unavailable / partial
  v
Snapshot + collection status
  |-- indexes: UID, namespace/name, labels, PVC, node, Helm tuple
  v
Builder / narrow reconcilers
  |-- graph facts
  |-- degraded sources
  v
Kernel
  v
Diagnostic recipe
  |-- budget
  |-- structured warnings
  |-- ranked evidence
  |-- partial metadata
  v
HTTP / CLI / Viewer / Skill
```

Engineering findings accepted:

| Finding | Decision |
| --- | --- |
| Partial RBAC currently fails closed in common sources | Add source-scoped degraded collection before Helm exact evidence. |
| Depth-limited traversal is not bounded enough | Add diagnostic budgets and partial response metadata. |
| Helm/evidence can go stale if only full-build resolver exists | Add stream categories and full-vs-narrow equivalence tests. |
| Graph build has O(Service * Pod) and storage scan hot spots | Add snapshot indexes before Helm fan-out. |
| Event model lacks ranking primitives | Normalize richer Event and Pod status fields first. |
| Output contract lacks structured partial/degraded/ranking fields | Add additive fields; keep explanation human-only. |
| Helm Secret exact evidence needs security boundary | Dedicated opt-in source, redaction, size limits, decode errors. |
| Viewer URL/file/server state can create SSRF/local-file risk | Default loopback-only, explicit flags, allowed roots, embedded stripping. |
| Runtime query lock can block freshness | Release manager lock before traversal and use budgets. |
| Test plan missed high-risk invariants | Add release-blocking test matrix. |

Eng dual voices:

- Subagent found 10 engineering issues.
- Codex voice unavailable due external-service policy.

Eng consensus: subagent-only, all findings accepted.

Test plan artifact:

`/Users/bytedance/.gstack/projects/Colvin-Y-kubernetes-ontology/bytedance-main-eng-review-test-plan-20260429-135113.md`

## Decision Audit Trail

| # | Phase | Decision | Classification | Principle | Rationale | Rejected |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | CEO | Reframe from broad diagnostic kernel to read-only provenance graph | User challenge accepted from review | Focus | Avoids competing with dashboards and observability stacks | Broad dashboard/kernel framing |
| 2 | CEO | Keep in-memory backend for this phase | Mechanical | Completeness | User explicitly required it; add scale budgets to make it credible | Persistent backend |
| 3 | CEO | Treat Helm labels as evidence, not truth | Mechanical | Explicit over clever | Helm/Kubernetes labels are recommended and informal | Label-only ownership truth |
| 4 | CEO | Use exact Helm manifest membership when release Secrets are readable | Taste | Completeness | Better accuracy while staying read-only; Secret RBAC can be disabled | Labels only |
| 5 | CEO | Productize diagnostic recipes before adding endpoint families | Mechanical | DRY | Generic diagnostic traversal already exists | Endpoint sprawl |
| 6 | CEO | Rename NodePressure to NodeContext | Mechanical | Pragmatic | Current product lacks metrics/kubelet history needed for pressure RCA | Telemetry claims |
| 7 | CEO | Make docs/schema primary and skill a wrapper | Mechanical | DRY | OSS adoption cannot depend on one agent runtime | Skill as primary product logic |
| 8 | CEO | Viewer is reference workspace, not product center | Mechanical | Focus | Avoids Headlamp/KubeView competition | Standalone dashboard strategy |
| 9 | Design | Make evidence/timeline first-screen primary surface | Mechanical | Hierarchy | Users need the likely cause before raw graph controls | Current control-first layout |
| 10 | Design | Define viewer lanes and degraded states before implementation | Mechanical | Completeness | Prevents generic graph UI and hidden partial data | Vague lane names |
| 11 | Eng | Add partial RBAC contract before Helm exact evidence | Mechanical | Safety | Least-privilege installs must degrade, not fail closed | All-or-nothing collection |
| 12 | Eng | Add diagnostic budgets beyond depth | Mechanical | Completeness | High fan-out can still explode within depth limits | Depth-only bound |
| 13 | Eng | Add snapshot indexes before Helm fan-out | Mechanical | Performance | Current relation recovery has known scan hot spots | More resolver scans |
| 14 | Eng | Treat viewer upstream/file URL state as security-sensitive | Mechanical | Security | Shared URLs can otherwise become local file or SSRF risks | Arbitrary server/file params |
| 15 | DX | Make zero-clone Helm install primary | Mechanical | Developer empathy | OSS evaluators should not clone repo just to install a chart | `./charts` as primary path |
| 16 | DX | Make Secret read default off | Mechanical | Trust | Exact Helm evidence is valuable but should be opt-in | default Secret read |
| 17 | DX | Unify recipe vocabulary across CLI/API/viewer/docs | Mechanical | DRY | Same concept should not have four names | ad hoc flags/routes |
| 18 | DX | Add user upgrade and contributor onboarding docs | Mechanical | Completeness | Release docs do not help users upgrade or contributors demo locally | maintainer-only release docs |

## CEO Review Summary

Mode: SELECTIVE EXPANSION.

Premise challenge:

- The in-memory premise is valid if the product is honest about budget and
  degradation.
- The Helm premise is valid only as confidence-scored evidence.
- The original scope was too broad. The corrected wedge is incident evidence and
  ownership blast radius.

Subagent outside voice:

- P0: plan was too broad and risked becoming a weaker dashboard.
- P0: competitive risk was understated.
- P1: Helm ownership needed confidence tiers and optional exact manifest
  evidence.
- P1: query families should be recipes unless contract differs.
- P1: Event ranking needs richer normalized event data first.
- P1: NodePressure should be dropped or renamed to topology-safe NodeContext.

Codex outside voice:

- Unavailable. Local policy rejected sending repo context to external Codex CLI.

Consensus:

| Dimension | Subagent | Codex | Consensus |
| --- | --- | --- | --- |
| Premises valid? | yes, with constraints | unavailable | subagent-only |
| Right problem? | yes, after reframe | unavailable | subagent-only |
| Scope calibrated? | too broad, now narrowed | unavailable | subagent-only |
| Alternatives explored? | needed integrations/plugins | unavailable | subagent-only |
| Competitive risk covered? | no, now added | unavailable | subagent-only |
| 6-month trajectory sound? | only with scale/eval gates | unavailable | subagent-only |

Phase 1 result: proceed with corrected plan.
