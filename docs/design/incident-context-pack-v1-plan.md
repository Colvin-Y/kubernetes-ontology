# Incident Context Pack v1 Plan

Status: implemented and locally verified on 2026-04-29.

Branch: `codex/incident-context-pack-v1`.

## Product Cut

Incident Context Pack v1 is a small, read-only diagnostic artifact for the
incident prompt:

> Helm upgrade failed, but the user no longer has Helm CLI output.

The v1 promise is intentionally narrow: show the current Kubernetes runtime
evidence, the inferred Helm/package provenance already visible in the cluster,
and the missing Helm-side evidence that cannot be reconstructed from current
objects.

This is not a new RCA engine, dashboard rewrite, Helm Secret collector, or
cluster-writing workflow.

## Agent Review Summary

Three review agents converged on the same boundary:

- Keep the first PR to one incident story rather than a broad diagnostics
  platform rewrite.
- Reuse the existing diagnostic response fields: `warnings`,
  `degradedSources`, `budgets`, `rankedEvidence`, and `conflicts`.
- Add only thin metadata for recipe identity and viewer/agent grouping.
- Fix viewer proxy gaps where the UI accepted diagnostic budgets but the Python
  proxy dropped them.
- Provide a checked-in fixture so the incident story is visible without a live
  cluster.

## Scope

Ship in this slice:

- optional diagnostic `recipe` metadata, with additive `schemaVersion` and
  `lanes`
- a v1 recipe for `helm-upgrade-runtime-failure` plus stable aliases for
  `pod-incident`, `workload-incident`, and `helm-ownership`
- CLI and HTTP threading for `--recipe` / `recipe=...`
- viewer support for HelmChart diagnostics, recipe selection, freshness/budget
  visibility, and clickable ranked evidence
- Python and Go viewer proxy forwarding for `maxNodes`, `maxEdges`, and
  `recipe`
- an offline Helm upgrade failure fixture and README
- a minimal JSON Schema documenting the diagnostic graph contract
- focused tests for recipe validation, proxy forwarding, and fixture integrity

Defer:

- reading Helm release Secrets or exact release manifests
- adding management/remediation actions
- new endpoint families or nested CLI command grammar
- full lane/timeline UI
- scale redesign for very large clusters
- persistent graph storage

## Acceptance Criteria

1. A user can open the bundled Helm upgrade failure sample and immediately see
   ranked evidence before interacting with the graph.
2. The diagnostic response includes a recipe identity and leaves existing
   clients compatible.
3. The product separates observed Kubernetes symptoms, inferred Helm ownership,
   and unavailable Helm CLI/manifest evidence.
4. The viewer's budget and recipe controls reach the daemon through
   `make visualize`.
5. The sample's node/edge references are internally consistent and include the
   v1 metadata fields.
6. Go tests and viewer tests pass locally.

## Verification Plan

- `go test ./...`
- `make visualize-check`
- fixture integrity test under `tools/visualize`
- post-implementation review by three sub-agents:
  - product/docs
  - Go API/CLI/server
  - viewer/DX/tests

## Verification Result

Completed locally on 2026-04-29:

- `go test ./...`
- `make visualize-check`
- `curl` load of `samples/helm-upgrade-failure/diagnostic-graph.json` through
  the viewer server
- headless Chrome screenshot smoke test for the Helm upgrade failure fixture
- three post-implementation sub-agent reviews, with the Go release viewer
  recipe proxy gap fixed afterward
