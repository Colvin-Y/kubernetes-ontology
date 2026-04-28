# Kubernetes Ontology Access

> Guided onboarding and read-only troubleshooting workflows for `kubernetes-ontology`.

This skill helps an AI agent install, operate, and use
[`kubernetes-ontology`](https://github.com/Colvin-Y/kubernetes-ontology) for
Kubernetes topology queries, Pod and Workload diagnostics, Helm deployment,
release binary setup, and topology viewer handoff.

## Install

Install directly from GitHub:

```bash
npx skills add https://github.com/Colvin-Y/kubernetes-ontology/tree/main/skills/kubernetes-ontology-access -g --agent codex
```

Or install from the repository and select this skill:

```bash
npx skills add Colvin-Y/kubernetes-ontology -s kubernetes-ontology-access -g --agent codex
```

Restart your agent after installation so it discovers the new skill.

## Usage

Example prompts:

```text
Use the kubernetes-ontology-access skill to onboard my cluster with Helm.
```

```text
Use kubernetes-ontology to diagnose an ImagePullBackOff Pod and open the viewer path.
```

```text
Set up the release binary server and CLI for a private Kubernetes cluster.
```

## What It Covers

- Helm and release binary onboarding paths.
- Read-only daemon, CLI, and HTTP API usage.
- Pod and Workload diagnostic subgraph queries.
- Human topology viewer handoff for visual inspection.
- Cleanup guidance for short-lived troubleshooting sessions.

## Safety

The skill is designed around read-only troubleshooting. It tells agents to ask
before running cluster-changing commands such as `helm upgrade --install`, and
it keeps public HTTP API or viewer exposure out of the default path.

## Platforms

Works with agents that support the open `SKILL.md` format, including Codex,
Claude Code, GitHub Copilot agent skills, and other compatible agent runtimes.

## License

Apache-2.0, matching the repository license.
