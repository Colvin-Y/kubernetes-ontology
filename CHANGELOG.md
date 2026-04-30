# Changelog

## v0.1.6 - 2026-04-30

### Added

- Added the professional Cytoscape-backed topology renderer as the default
  viewer experience while keeping the legacy SVG renderer available.
- Added a deterministic kind Helm storage sample that covers Helm provenance,
  PVC, PV, StorageClass, CSIDriver, local-path provisioner, Node, and Event
  evidence.
- Added refreshed English and Chinese documentation GIFs generated from the
  real viewer.

### Changed

- Updated README, Quickstart, release guide, homepage, and access skill
  examples for `v0.1.6`.
- Packaged the vendored Cytoscape viewer asset in the embedded Go viewer and
  the local Python visualizer.

### Validation

- Local CI, viewer checks, Helm template validation, and release packaging
  checks are run before tagging.

## v0.1.5 - 2026-04-29

### Added

- Added bounded diagnostic response metadata for AI-agent consumers:
  `partial`, `warnings`, `budgets`, `rankedEvidence`, `degradedSources`, and
  `conflicts`.
- Added Helm provenance modeling from standard Helm labels and annotations,
  including `HelmRelease`, `HelmChart`, `managed_by_helm_release`, and
  `installs_chart`.
- Added viewer rendering support for diagnostic warnings, budget status,
  evidence ranking, degraded sources, conflicts, and Helm provenance nodes.
- Expanded the Kubernetes ontology OWL schema and access skill guidance for the
  new diagnostic and Helm graph contract.

### Changed

- Refreshed installation examples, Helm chart metadata, and release guide
  examples for `v0.1.5`.

### Validation

- Local CI, visualization checks, and live Kubernetes diagnostic checks are run
  against a disposable kind cluster with an explicit kubeconfig.

## v0.1.4 - 2026-04-28

- Published release binaries and GHCR images for the open-source MVP.
