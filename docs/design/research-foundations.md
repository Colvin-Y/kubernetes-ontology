# Research Foundations

The research notes in `docs/design/research/` guide the core product choices for
`kubernetes-ontology`.

## Synthesis

The strongest implementation path is not ontology-first. Kubernetes state
changes too quickly for a pure OWL/RDF runtime to be the hot path. The practical
architecture is:

1. collect Kubernetes objects with read-only list/watch access;
2. normalize them into stable entities and provenance-aware relationships;
3. maintain an in-memory dependency graph for low-latency queries;
4. expose bounded graph queries for diagnostics and AI-agent consumption;
5. add semantic mapping, OWL/RDF export, rule checks, or persistent backends only
   after the operational graph is trustworthy.

This matches both industry systems and the AICCSA 2025 Kubernetes ontology
paper: runtime dependency graphs provide freshness and update efficiency, while
ontology/rules provide semantic consistency and higher-level reasoning.

## Design Principles

- Keep Kubernetes access read-only.
- Treat the in-memory graph as the MVP runtime source of truth.
- Use canonical IDs and typed edges as the stable consumer contract.
- Preserve provenance so AI clients can distinguish asserted, observed, and
  inferred facts.
- Prefer incremental reconciliation for common change categories.
- Keep diagnostic graph slices bounded and policy-driven.
- Make ontology materialization an extension, not a prerequisite.

## Research Notes

- `research/kubernetes-ontology-research.md` surveys industry approaches:
  observability topology graphs, property graphs, semantic web systems, and
  catalog-style metadata models.
- `research/AICCSA66935.2025.11315476.md` summarizes the hybrid architecture
  from "Ontological Modeling of Kubernetes Clusters Leveraging In-Memory
  Dependency Graphs" and maps it to this repository.
