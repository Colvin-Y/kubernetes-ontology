# User Story: Diagnose Helm Upgrade Failure Without Helm CLI Output

## Status

Validated on a local kind cluster on 2026-04-29.

This story standardizes the product behavior for the common prompt:

> `helm upgrade failed. Help me diagnose it.`

The important constraint is that the user may not have saved the Helm CLI
stderr/stdout. `kubernetes-ontology` must therefore separate Kubernetes evidence
that is still observable from Helm-side evidence that is no longer available.

## Story

As an SRE or platform engineer using an AI agent, when a user reports that a
Helm upgrade failed but cannot provide Helm CLI output, I want the agent to
diagnose the current Kubernetes evidence for that Helm release, show the
affected release resources and likely runtime symptom, and explicitly ask for
Helm-side evidence only when the failure happened before Kubernetes objects
were applied.

## Inputs

Required:

- release name
- release namespace
- read-only Kubernetes API access to the target cluster

Optional but useful:

- Helm CLI stderr/stdout from the failed command
- `helm status` or `helm history` output
- chart name/version and values diff
- whether `--atomic`, hooks, or `--wait` were used

The runtime product path must not require Helm CLI output to diagnose failures
that reached the Kubernetes API.

## Primary Flow

1. The agent runs a `HelmRelease` diagnostic for the reported release.
2. The diagnostic expands probable release-owned resources through
   `managed_by_helm_release` and `installs_chart` evidence.
3. The diagnostic includes warnings for Helm evidence that is outside the
   current Kubernetes object graph.
4. If Events, Pods, Workloads, Services, images, or other runtime objects show a
   rollout symptom, the agent follows the graph into the affected Workload or
   Pod diagnostic.
5. The agent gives a scoped answer:
   - what is observable now
   - what is inferred from labels and annotations
   - what cannot be known without Helm-side output or future release history
     collection
   - the next concrete command or artifact to request from the user

## Boundary

`kubernetes-ontology` can diagnose rollout failures that reached Kubernetes,
for example:

- new Pod stuck in `ImagePullBackOff`
- Deployment rollout timed out
- Service selector mismatch caused by chart changes
- missing ConfigMap, Secret, PVC, ServiceAccount, RBAC, webhook, or CSI
  dependencies that are represented in Kubernetes objects and Events

`kubernetes-ontology` cannot reconstruct these from current cluster objects
alone:

- chart template render failures before apply
- values schema validation failures before apply
- chart repository, dependency build, or client-side plugin errors
- Helm hook failures where no surviving Kubernetes evidence remains
- exact release manifest membership when only labels and annotations are
  collected
- `--atomic` rollback root cause if failed resources and Events disappeared

In those cases the agent should stop claiming cluster-derived root cause and ask
for Helm stderr/stdout, `helm status`, `helm history`, or chart/values input.

## Evidence Contract

| Field | Required behavior |
| --- | --- |
| `warnings[].code == "helm_cli_output_not_observed"` | Present whenever Helm evidence exists in the diagnostic graph. It tells agents not to infer template, values, repository, hook, client, or rollback errors from cluster objects alone. |
| `warnings[].code == "helm_manifest_evidence_not_collected"` | Present for label-derived Helm provenance. It tells agents that default ownership is a strong hint, not exact manifest membership. |
| `degradedSources[].source == "helm_cli_output"` | Marks Helm CLI output as outside the Kubernetes API evidence source. |
| `degradedSources[].source == "helm_release_manifest"` | Marks current manifest membership as unavailable when only labels and annotations were collected. |
| `rankedEvidence[].reason == "HelmOwnershipEvidence"` | Explains resource-to-release ownership hints. |
| `rankedEvidence[].reason == "HelmChartEvidence"` | Explains release-to-chart evidence. |
| `conflicts[].code == "helm_ownership_conflict"` | Preserves contradictory Helm ownership evidence instead of silently choosing one release. |

## Acceptance Criteria

### AC1: Release Failure That Reached The Cluster

Given a release whose upgrade changed Kubernetes objects and then timed out,
when the agent runs `--diagnose-helm-release` for that release, then the result
must include:

- a `HelmRelease` entry
- probable owned Workload, Service, Pod, and `HelmChart` nodes when present
- `managed_by_helm_release` and `installs_chart` evidence
- `helm_cli_output_not_observed`
- `helm_manifest_evidence_not_collected`
- `helm_cli_output` and `helm_release_manifest` degraded sources

### AC2: Runtime Symptom Is Ranked

Given the failed upgrade produced a bad Pod, when the agent runs
`--diagnose-pod` for that Pod, then warning Events such as `ImagePullBackOff`,
`ErrImagePull`, or `Failed to pull image` must rank before informational
ownership evidence, while the same diagnostic still preserves Helm release and
chart provenance.

### AC3: Render Failure Before Apply

Given Helm failed during template rendering before any release-owned Kubernetes
object was applied, when the agent runs `--diagnose-helm-release`, then the CLI
must return a not-found diagnostic entry instead of inventing a release graph.
The agent response must ask the user for Helm stderr/stdout, `helm status`,
`helm history`, or chart/values context.

### AC4: Conflicting Ownership

Given one Kubernetes resource contains contradictory Helm release evidence,
when the agent runs a diagnostic that includes that resource, then the result
must include `helm_ownership_conflict` and must not collapse the object into a
single silent owner.

### AC5: Budget And Freshness Safety

Given the graph slice exceeds diagnostic budgets or the runtime is not ready,
when the agent receives `partial=true`, `budgets.truncated=true`, warnings, or
freshness metadata, then it must report the diagnostic as incomplete and avoid
claiming release-wide completeness.

### AC6: Safety

The product diagnostic path is read-only. It must not mutate observed
Kubernetes resources, install CRDs/controllers, or run Helm against the user's
cluster. Test fixtures may create and delete resources only in an explicit local
kind cluster using an explicit kubeconfig.

## Local Validation Matrix

All live validation used the local kind kubeconfig:
`/private/tmp/kubernetes-ontology-kind-kubeconfig`.

| Scenario | Command shape | Result |
| --- | --- | --- |
| Real upgrade reached cluster, then timed out | `helm upgrade story-release ... --wait --timeout 45s` with a missing image tag | Helm returned `context deadline exceeded`; cluster retained a running old Pod and a new `ImagePullBackOff` Pod. |
| Release diagnostic without using Helm output | `bin/kubernetes-ontology --kubeconfig /private/tmp/kubernetes-ontology-kind-kubeconfig --diagnose-helm-release --namespace helm-upgrade-story --name story-release ...` | Returned `HelmRelease`, Workload, Service, two Pods, `HelmChart`, Events, Helm warnings, degraded sources, and ranked Helm/Event evidence. |
| Pod diagnostic for the failed rollout | `bin/kubernetes-ontology --kubeconfig /private/tmp/kubernetes-ontology-kind-kubeconfig --diagnose-pod --namespace helm-upgrade-story --name story-release-ko-upgrade-story-65484cb55-kjr7h ...` | Ranked warning Events for image pull failure before Helm ownership/chart evidence. |
| Render failure before apply | `helm upgrade --install render-fail /private/tmp/ko-helm-render-fail ...` | Helm failed with a template error before release evidence existed. |
| Diagnostic after render failure | `bin/kubernetes-ontology --kubeconfig /private/tmp/kubernetes-ontology-kind-kubeconfig --diagnose-helm-release --namespace helm-render-fail --name render-fail ...` | Returned `diagnostic entry not found`, confirming the correct boundary. |

Temporary validation namespaces were deleted after the run.

## Agent Response Template

Use this shape when answering the user:

```text
I can diagnose the part of this Helm upgrade that reached Kubernetes.

Observed now:
- <top runtime symptom from ranked Events>
- <affected Workload/Pod/Service/Image>
- <Helm release/chart ownership evidence and confidence>

Limits:
- I do not have the original Helm CLI output.
- Current Helm ownership is label/annotation evidence, not exact manifest
  membership.

Next useful input:
- If the error happened before objects were created, paste the Helm stderr or
  `helm status/history`.
- If the error reached rollout, I can continue from the affected Pod/Workload
  diagnostic.
```

## Future Extensions

- Read Helm release Secrets when RBAC permits it and add exact manifest
  membership as a higher-confidence evidence tier.
- Preserve release revision and status history as read-only graph nodes.
- Add golden JSON fixtures for this story and render them in the topology
  viewer as a reference incident path.
