# Project Skills

This directory contains skills that can be installed into an agent environment.

## kubernetes-ontology-access

[`kubernetes-ontology-access`](kubernetes-ontology-access/SKILL.md) guides
users through Helm deployment, release CLI installation, daemon-backed topology
queries, AI-agent diagnostics, and topology viewer handoff.

See the skill package README for marketplace-friendly metadata, example
prompts, and install variants:
[`kubernetes-ontology-access/README.md`](kubernetes-ontology-access/README.md).

Install into Codex directly from GitHub, without cloning this repository first:

```bash
npx skills add https://github.com/Colvin-Y/kubernetes-ontology/tree/main/skills/kubernetes-ontology-access -g --agent codex
```

Or install from the repository and select this skill:

```bash
npx skills add Colvin-Y/kubernetes-ontology -s kubernetes-ontology-access -g --agent codex
```

These install commands are meant to track the repository's default branch.
Release tags are used for the daemon, CLI, viewer binaries, images, and Helm
chart versions; the skill itself should expose the latest onboarding workflow.

Restart Codex after installing the skill. If the onboarding flow needs the
repository checkout, the skill will guide the user to clone it at that point.
