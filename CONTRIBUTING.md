# Contributing

Thanks for helping improve `kubernetes-ontology`.

The project is still early, so the best contributions are small, concrete, and
easy to verify. Good examples:

- New diagnostic graph fixtures for real Kubernetes failure modes.
- Tests for graph edges, diagnostic budgets, degraded sources, or Helm evidence.
- Documentation fixes that reduce setup steps or clarify the read-only safety model.
- Bug reports with a minimal manifest, command, and observed output.

## Development Setup

Install Go, Helm, Python 3, and Docker if you want to run the full validation
path. For normal Go changes:

```bash
make test
```

For the repository CI checks:

```bash
make ci
```

For kind-based end-to-end validation:

```bash
kind create cluster --name ko-e2e --config samples/kind-helm-storage-demo/kind-config.yaml
docker build -t kubernetes-ontology:e2e .
kind load docker-image kubernetes-ontology:e2e --name ko-e2e
bash scripts/ci/verify_kind_e2e.sh
kind delete cluster --name ko-e2e
```

The kind test installs the sample Helm workload, deploys the current
`kubernetes-ontology` chart, and verifies real CLI and viewer queries against the
in-cluster daemon.

## Pull Request Checklist

- Keep the observed-cluster runtime read-only.
- Add or update tests for changed graph semantics.
- Update `AI_CONTRACT.md` or `schemas/diagnostic-subgraph.schema.json` when the
  diagnostic response contract changes.
- Update README or Quickstart docs when user-facing commands change.
- Run `make ci` before opening the PR when practical.

## Design Notes

Use the existing package boundaries:

- `internal/collect/k8s` reads Kubernetes objects.
- `internal/graph` builds and indexes graph facts.
- `internal/reconcile` applies scoped graph updates.
- `internal/query` and `internal/service/diagnostic` shape diagnostic responses.
- `internal/server` exposes the read-only HTTP API.

Prefer explicit graph semantics over clever inference. If an edge is inferred,
include provenance that tells downstream agents how much to trust it.

## Reporting Security Issues

Do not file public issues for security vulnerabilities. Follow
`SECURITY.md` instead.
