# Design Guide

This directory is the design home for `kubernetes-ontology`.

The project follows a graph-first, AI-agent-first architecture: maintain a
read-only, provenance-aware Kubernetes dependency graph in memory, expose stable
query contracts, and leave heavier ontology materialization or persistent graph
storage as later extensions.

Recommended reading order:

1. `research-foundations.md` summarizes the research and industry direction
   behind the design.
2. `current-state-and-next-steps.md` records the current implementation state.
3. `open-source-mvp-plan.md` defines the open-source MVP boundary.
4. `kubernetes-semantic-kernel-evolution.md` gives the long-term architecture.
5. `helm-upgrade-failure-user-story.md` standardizes the Helm upgrade failure
   scenario where the user no longer has Helm CLI output.
6. `incident-context-pack-v1-plan.md` records the scoped implementation plan for
   the first evidence-first incident pack.
7. `continuous-runtime-technical-design.md` and
   `continuous-runtime-progress-snapshot.md` capture runtime design history.

Detailed research notes are kept under `research/` so the design rationale and
source material live together.
