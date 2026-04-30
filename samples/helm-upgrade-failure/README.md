# Helm Upgrade Runtime Failure Sample

This synthetic sample represents a failed Helm upgrade where the new rollout
reached Kubernetes, but the user no longer has Helm CLI output.

Open it with:

```bash
make visualize
```

Then browse to:

```text
http://127.0.0.1:8765/?file=samples/helm-upgrade-failure/diagnostic-graph.json
```

What this sample can support:

- observed runtime evidence from a bad Pod and Warning Event
- inferred Helm release/chart provenance from labels and annotations
- explicit caveats for missing Helm CLI output and exact manifest membership
- a preserved ownership conflict instead of choosing a silent owner

What it must not claim:

- Helm template or values root cause
- chart repository, dependency, plugin, or client-side error
- exact Helm release manifest membership
- rollback root cause hidden by `--atomic`

Use this fixture as the offline reference for the user story in
`docs/design/helm-upgrade-failure-user-story.md`.
