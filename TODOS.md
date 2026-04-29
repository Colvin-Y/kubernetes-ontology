# Deferred Work

These items were considered during the open-source diagnostics evolution plan
and intentionally deferred.

## P2: Persistent Backend Adapter

What: Add a non-memory graph backend after the in-memory diagnostic contract is
proven.

Why: Persistent storage helps restart recovery and long-running history, but it
adds operational cost before the core evidence graph is validated.

Context: The next phase keeps the backend in memory. Revisit this after the
Pod-to-Helm and Pod-to-Storage/CSI showcase flows have golden tests, scale
budgets, and stable output contracts.

Depends on: stable diagnostic contract, schema/version policy, scale data.

## P2: Headlamp/K9s/Backstage Integrations

What: Build external integrations or plugins that consume the diagnostic JSON
contract.

Why: Integrations may become the best distribution path, but the contract and
recipes need to be stable first.

Context: The viewer remains a reference workspace. Do not build plugins until
recipe URLs, JSON schema, and embedded viewer mode are stable.

Depends on: recipe vocabulary, shareable URL contract, embedded viewer mode.

## P2: Full Helm Release History

What: Reconstruct Helm release history and previous manifest states.

Why: History can help rollback and incident timelines, but current release
membership is enough for first ownership and blast-radius diagnosis.

Context: This phase may read current Helm release manifests when explicitly
enabled. Full history is out of scope.

Depends on: opt-in Helm Secret source, redaction, size limits, versioned schema.

## P3: Node Pressure RCA

What: Diagnose node pressure with metrics, requests/limits, taints/tolerations,
kubelet pressure history, and eviction context.

Why: Without telemetry, this project would underperform observability tools.

Context: The current phase uses `node-context`, a topology-safe recipe that does
not claim pressure root cause analysis.

Depends on: metric source decision, resource request/limit collection, node
condition history.

## P3: Public Multi-Tenant Service

What: Host `kubernetes-ontology` as a public multi-tenant service.

Why: This requires auth, tenancy, rate limiting, secret handling, and exposure
hardening that would distract from the OSS local diagnostic kernel.

Context: HTTP API and viewer remain local or controlled-network surfaces.

Depends on: security model, tenancy model, auth, rate limits, hosted ops.
