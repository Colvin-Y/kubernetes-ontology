# Project Skills

This directory contains project-local skills that can be installed into an
agent environment.

## kubernetes-ontology-access

[`kubernetes-ontology-access`](kubernetes-ontology-access/SKILL.md) guides
users through Helm deployment, release CLI installation, daemon-backed topology
queries, AI-agent diagnostics, and topology viewer handoff.

Install into Codex:

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills"
cp -R skills/kubernetes-ontology-access "${CODEX_HOME:-$HOME/.codex}/skills/"
```

Restart Codex after installing the skill.
