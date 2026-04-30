# Agent Recipes

This page describes small diagnostic recipes for AI-agent clients. Recipes are
labels on the existing diagnostic subgraph response; they do not create a new
API family or change graph traversal identity.

The `--recipe` CLI flag and `recipe=...` HTTP parameter are available from the
current source branch and from releases after `v0.1.5`.

## Available v1 Recipes

| Recipe | Typical entry | Purpose |
| --- | --- | --- |
| `pod-incident` | `Pod` | Start from a bad Pod and rank runtime evidence. |
| `workload-incident` | `Workload` | Start from a Deployment/StatefulSet-style controller and inspect rollout dependencies. |
| `helm-ownership` | `HelmRelease` or `HelmChart` | Inspect label-derived Helm release/chart provenance. |
| `helm-upgrade-runtime-failure` | `Pod`, `Workload`, or `HelmRelease` | Diagnose the part of a failed Helm upgrade that reached Kubernetes when Helm CLI output is missing. |

Use the CLI:

```bash
bin/kubernetes-ontology \
  --server "http://127.0.0.1:18080" \
  --entry-kind Pod \
  --namespace default \
  --name bad-pod \
  --recipe helm-upgrade-runtime-failure
```

Or HTTP:

```text
GET /diagnostic?kind=Pod&namespace=default&name=bad-pod&recipe=helm-upgrade-runtime-failure
```

## Required Agent Behavior

For `helm-upgrade-runtime-failure`, structure the answer around:

- observed now: Events, Pod/Workload status, image/storage/config/RBAC paths
- inferred ownership: Helm release/chart edges and their confidence
- unavailable input: Helm CLI output, status/history, values, hooks, repository
  or client-side failures

Never infer Helm render, values, repository, hook, or rollback root cause from
current Kubernetes objects alone. Ask for Helm stderr/stdout or
`helm status/history` when the ranked evidence does not show a Kubernetes
runtime symptom.

## Offline Fixture

Open `samples/helm-upgrade-failure/diagnostic-graph.json` in the viewer to see
the v1 contract without a live cluster:

```text
http://127.0.0.1:8765/?file=samples/helm-upgrade-failure/diagnostic-graph.json
```

The fixture includes `schemaVersion`, `recipe`, `lanes`, `warnings`,
`degradedSources`, `budgets`, `rankedEvidence`, `conflicts`, and `freshness`.
