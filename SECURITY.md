# Security Policy

`kubernetes-ontology` is designed as a read-only Kubernetes topology and
diagnostic service. It should not mutate observed cluster workloads or write
status, annotations, CRDs, or remediation actions.

## Supported Versions

Security fixes target the latest released version and the default branch.

| Version | Supported |
| ------- | --------- |
| latest release | Yes |
| default branch | Yes |
| older releases | Best effort |

## Reporting A Vulnerability

Please report suspected vulnerabilities privately to the maintainer before
opening a public issue.

Preferred contact:

- GitHub: open a private vulnerability report from the repository Security tab
  if available.
- Fallback: contact the repository owner, `Colvin-Y`, through GitHub.

Include:

- Affected version or commit.
- Install mode: Helm, release binary, or source.
- Minimal reproduction steps.
- Whether the issue requires cluster credentials, public HTTP exposure, or
  access to Secret-derived graph data.

## Security Boundaries

Expected behavior:

- Kubernetes collection uses `get`, `list`, and `watch`.
- The daemon and viewer should be exposed only on localhost or controlled
  internal networks unless an operator adds external protection.
- The HTTP API has no built-in authentication or TLS yet.
- Secret reads are used only to model Secret nodes and `uses_secret` edges.
  Disable them with `rbac.readSecrets=false` when that evidence is not needed.

Out of scope for the current MVP:

- Public multi-tenant hosting.
- Built-in HTTP authentication and TLS termination.
- Remediation or cluster mutation.
- Full Kubernetes authorization reasoning.

If a report shows that the runtime writes to observed resources, leaks Secret
values, or makes unsafe claims from partial graph evidence, treat it as high
priority.
