# Project Skills

This directory contains skills that can be installed into an agent environment.

## kubernetes-ontology-access

[`kubernetes-ontology-access`](kubernetes-ontology-access/SKILL.md) guides
users through Helm deployment, release CLI installation, daemon-backed topology
queries, AI-agent diagnostics, and topology viewer handoff.

Install into Codex directly from GitHub, without cloning this repository first:

```bash
npx skills add https://github.com/Colvin-Y/kubernetes-ontology/tree/main/skills/kubernetes-ontology-access -g --agent codex
```

Restart Codex after installing the skill. If the onboarding flow needs the
repository checkout, the skill will guide the user to clone it at that point.
