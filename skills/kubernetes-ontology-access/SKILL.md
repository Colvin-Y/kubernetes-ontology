---
name: kubernetes-ontology-access
description: Use this skill whenever a user wants to onboard, deploy, install, or operate kubernetes-ontology; set up its Helm chart, release CLI, daemon, or topology viewer; run Kubernetes topology queries; diagnose Pod or Workload failures with AI-agent workflows; or connect human visual troubleshooting to the CLI and HTTP API. This skill should trigger for requests about Kubernetes ontology onboarding, Helm deployment, topology query, diagnostic subgraph, ImagePullBackOff or storage/RBAC/Event graph troubleshooting, viewer usage, and agent integration.
---

# Kubernetes Ontology Access

Guide users from zero to useful diagnosis with `kubernetes-ontology`.

This skill is the repository's onboarding playbook for three connected modes:

1. AI-agent automatic troubleshooting through stable daemon-backed queries.
2. CLI-driven topology and diagnostic inspection.
3. Human visual intervention through the topology viewer.

Prefer read-only, daemon-backed workflows. The project observes Kubernetes
objects and builds an in-memory graph; it must not mutate observed workloads.

## Operating Posture

Do not merely summarize the docs. Drive the user toward the next useful action:

- Identify the user's current state: no setup, existing daemon, CLI-only query,
  diagnostic request, viewer handoff, or source development.
- Choose the shortest path that satisfies the request.
- Give concrete commands the user or agent can run next, with safe defaults
  named inline.
- When command output is needed, ask for or run that command before moving to
  later steps.
- Keep cluster-changing actions explicit: ask for confirmation before running
  `helm upgrade --install` or any command that changes cluster resources.
- Do not require a repository checkout for CLI-only diagnostics against an
  already running daemon.

## First Response Checklist

When this skill triggers, quickly establish:

- Whether the user is inside a local checkout of this repository.
- Target cluster context and logical cluster name.
- Desired namespaces for collection (`contextNamespaces`).
- Deployment path: Helm + release CLI by default, source build for development.
- Diagnostic entry, if known: `Pod` or `Workload`, namespace, and name.
- Whether they want the viewer opened for human inspection.

If values are missing and a safe default exists, proceed with the default and
name it. Ask only for values that are required and cannot be inferred, such as
the target namespace/name for a diagnostic query.

## Safety Model

Explain this once when onboarding a new user:

- Runtime collection is read-only against observed Kubernetes resources.
- Helm installs this project's own Deployment, Service, ServiceAccount,
  ConfigMap, viewer, and read-only RBAC.
- The default RBAC includes `get`, `list`, and `watch`; Secret reads are enabled
  so `uses_secret` edges can be collected. Use `--set rbac.readSecrets=false`
  when Secret collection is not acceptable.
- Keep the HTTP API and viewer behind `kubectl port-forward` or a controlled
  private network. Do not expose them directly to the public internet.

## Repository Pointers

When working from a checkout, read only the files needed for the current task:

- `README.md` or `README.zh-CN.md` for project overview.
- `QUICKSTART.md` for end-to-end setup and commands.
- `AI_CONTRACT.md` for downstream agent consumption rules.
- `charts/kubernetes-ontology/values.yaml` for Helm values.
- `Makefile` for local build, server, CLI, and viewer targets.

## Recommended Onboarding Flow

Use Helm + release CLI unless the user is developing the project locally.

Only guide the user to clone the repository when the current path needs files
from the checkout, such as the local Helm chart under
`charts/kubernetes-ontology`, source development, or local viewer development.
If the user only needs CLI queries against an existing daemon, skip the clone.

When a checkout is needed and the user does not already have one, use:

```bash
git clone https://github.com/Colvin-Y/kubernetes-ontology.git
cd kubernetes-ontology
```

If the user is not ready to deploy yet, first verify prerequisites and collect
the target cluster, logical cluster name, and namespaces. Then return to the
checkout step only when deploying the chart or using source-local commands.

### 1. Verify Prerequisites

Check or ask the user to check:

```bash
kubectl config current-context
kubectl get namespace
helm version
```

If the user expects the agent to run commands, confirm before using any command
that changes cluster resources, including `helm upgrade --install`.

### 2. Deploy The Helm Chart

Use a release version and image. If the user did not specify one, use the latest
project release they selected or the version already present in the repository
docs. Do not invent a future version.

```bash
export KO_VERSION=v0.1.3
export KO_IMAGE=ghcr.io/colvin-y/kubernetes-ontology

helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --create-namespace \
  --set image.repository="${KO_IMAGE}" \
  --set image.tag="${KO_VERSION}" \
  --set cluster="your-logical-cluster" \
  --set contextNamespaces='{default,kube-system}'
```

For all namespaces, remove the `--set contextNamespaces=...` line and use the
chart default empty list. For no Secret collection:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --reuse-values \
  --set rbac.readSecrets=false
```

Wait for rollout:

```bash
kubectl -n kubernetes-ontology rollout status deploy/kubernetes-ontology
```

The viewer rollout exists when `viewer.enabled=true`, which is the chart
default:

```bash
kubectl -n kubernetes-ontology rollout status deploy/kubernetes-ontology-viewer
```

### 3. Port-Forward Server And Viewer

Use separate terminals or background sessions:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology 18080:18080
```

The viewer service exists when `viewer.enabled=true`, which is the chart
default:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology-viewer 8765:8765
```

If the user disabled the viewer, expose only the server or re-enable the viewer
later.

Default endpoints:

- Server: `http://127.0.0.1:18080`
- Viewer: `http://127.0.0.1:8765`

### 4. Download The CLI

Download `kubernetes-ontology` from GitHub Releases for the selected
`KO_VERSION`. The repository release workflow packages `kubernetes-ontology`,
`kubernetes-ontologyd`, and `kubernetes-ontology-viewer` under an archive root
named `kubernetes-ontology_${KO_VERSION}_${GOOS}_${GOARCH}`. Choose the archive
suffix by platform:

- macOS Apple Silicon: `darwin_arm64.tar.gz`
- macOS Intel: `darwin_amd64.tar.gz`
- Linux x86_64: `linux_amd64.tar.gz`
- Linux ARM64: `linux_arm64.tar.gz`
- Windows x86_64: `windows_amd64.zip`

Example for macOS Apple Silicon:

```bash
curl -LO "https://github.com/Colvin-Y/kubernetes-ontology/releases/download/${KO_VERSION}/kubernetes-ontology_${KO_VERSION}_darwin_arm64.tar.gz"
tar -tzf "kubernetes-ontology_${KO_VERSION}_darwin_arm64.tar.gz" | head
tar -xzf "kubernetes-ontology_${KO_VERSION}_darwin_arm64.tar.gz"
sudo install "kubernetes-ontology_${KO_VERSION}_darwin_arm64/kubernetes-ontology" /usr/local/bin/kubernetes-ontology
```

If the user is installing from a fork or custom release, inspect the archive
contents first and adjust the install path to the actual extracted directory.
If the user cannot use `sudo`, keep the binary in a local directory and invoke
it by path.

### 5. Confirm The Daemon Is Ready

```bash
kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

Continue only when status shows `Ready: true` or `Phase: ready`. List,
entity, relation, expand, and diagnostic responses include lowercase
`freshness.ready` metadata that agents can use after the daemon is serving
queries. If the daemon is not ready, inspect rollout logs and retry status.

## AI-Agent Automatic Troubleshooting

Use this flow when the user asks the agent to diagnose a workload or pod.

### Pod Entry

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --machine-errors \
  --diagnose-pod \
  --namespace default \
  --name my-pod
```

### Workload Entry

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --machine-errors \
  --diagnose-workload \
  --namespace default \
  --name my-deployment
```

### Agent Reasoning Rules

- Treat the response as a bounded evidence graph, not complete cluster truth.
- Index nodes by `canonicalId`; join edges by `from` and `to`.
- Prefer edge/node attributes and provenance over explanation text for hard
  conclusions.
- Use explanation text for narrative and suspicion ranking.
- If evidence is missing, report that it is missing from the current graph
  slice rather than claiming the object does not exist anywhere.
- Prefer asserted and observed facts before inferred shortcuts.
- For contract details, consult `AI_CONTRACT.md`.

Current limits to keep visible in agent reasoning:

- `explanation` content is useful but best-effort and not fully ranked.
- Traversal policy can hide valid facts outside the selected graph slice.
- CSI component correlation is configurable with `csiComponentRules`; no
  driver-specific component inference runs unless a matching rule is configured.
- A missing edge in one diagnostic response should be treated as missing
  evidence in that slice, not global absence.

### Agent Output Template

For diagnostic answers, respond with:

````markdown
## Summary
[1-3 sentence diagnosis]

## Evidence
- [node/edge/provenance fact]
- [node/edge/provenance fact]

## Next Queries
```bash
[one or two targeted CLI commands]
```

## Human Viewer
Open http://127.0.0.1:8765 and load the same Pod or Workload diagnostic graph.
````

Avoid recommending cluster mutations unless the user explicitly asks for
remediation and the evidence supports it.

## CLI Query Playbook

Status:

```bash
kubernetes-ontology --server "http://127.0.0.1:18080" --status
```

List entities:

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-entities \
  --entity-kind Pod \
  --namespace default \
  --limit 20
```

Resolve an entity and capture `entity.entityGlobalId`:

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name my-pod
```

Expand one entity:

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --expand-entity \
  --entity-id 'your/entityGlobalId' \
  --expand-depth 1 \
  --limit 100
```

List filtered relations:

```bash
kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --list-filtered-relations \
  --from 'your/entityGlobalId' \
  --relation-kind scheduled_on \
  --limit 50
```

Common stable relation kinds include:

- `controlled_by`
- `owns_pod`
- `scheduled_on`
- `selects_pod`
- `uses_config_map`
- `uses_secret`
- `uses_service_account`
- `bound_by_role_binding`
- `mounts_pvc`
- `bound_to_pv`
- `reported_by_event`
- `affected_by_webhook`
- `managed_by_csi_controller`
- `served_by_csi_node_agent`

## Human Visual Troubleshooting

Use the viewer when the user wants to inspect topology manually, validate an
agent conclusion, compare graph branches, or export a visible subgraph.

Steps:

1. Port-forward the viewer service.
2. Open `http://127.0.0.1:8765`.
3. Load live topology or a focused Pod/Workload diagnostic graph.
4. Select nodes to inspect attributes, provenance, and edge kinds.
5. Expand one hop when a relation needs more context.
6. Export the visible graph as JSON when the agent needs to continue from the
   exact human-inspected state.

The Helm chart enables the viewer by default. If the user installed with
`viewer.enabled=false`, skip the viewer rollout and port-forward commands, or
enable it with:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --reuse-values \
  --set viewer.enabled=true
```

If using the local development viewer instead of Helm:

```bash
make visualize
```

Or use the dependency-free release viewer:

```bash
kubernetes-ontology-viewer --server "http://127.0.0.1:18080"
```

## Source Development Path

Use this path for contributors or local code changes:

```bash
make build build-daemon build-viewer
cp local/kubernetes-ontology.yaml.example local/kubernetes-ontology.yaml
```

Edit `local/kubernetes-ontology.yaml`, then run in separate terminals:

```bash
make serve
make visualize
```

CLI checks:

```bash
make status-server
make list-entities-server ENTITY_KIND=Pod NAMESPACE=default LIMIT=20
make diagnose-pod-server NAMESPACE=default NAME=my-pod
```

For code changes, verify:

```bash
make verify
make live-check NAMESPACE=default NAME=my-pod
```

## Troubleshooting The Onboarding

If `entry not found`:

- Check exact namespace/name.
- List available entities in the namespace.
- Confirm `contextNamespaces` includes the target namespace, or is empty for
  all namespaces.

If status is not ready:

- Check the server rollout and pod logs.
- Confirm the ServiceAccount has read access to the needed resources.
- Increase `bootstrapTimeout` for large or slow clusters.

If graph evidence seems too small:

- Increase diagnostic depth with `--max-depth` or `--storage-max-depth`.
- Use `--expand-terminal-nodes` for deliberate fan-out.
- Expand selected viewer nodes one hop instead of loading the whole cluster.

If the viewer cannot load data:

- Confirm both port-forwards are running.
- Check `http://127.0.0.1:18080/status`.
- Restart the viewer port-forward and reload the page.
