# Release Guide

This project publishes binaries and a packaged Helm chart through GitHub
Releases, plus container images through GitHub Container Registry (GHCR).

Docker Hub is optional. The default open-source release path does not require a
Docker Hub account, namespace, access token, or repository secret.

## One-Time GitHub Setup

1. Make sure GitHub CLI is logged in as the project owner:

   ```bash
   gh auth status
   ```

   Expected account:

   ```text
   Colvin-Y
   ```

2. Keep the repository private while reviewing the initial open-source import.
   When ready to publish, change the repository visibility to Public from
   GitHub repository settings.

3. Leave GitHub Actions enabled. The workflows declare their own minimum token
   permissions:

   - release workflow: `contents: write`
   - docker workflow: `contents: read`, `packages: write`

   If the Docker workflow fails with a package permission error, check:

   ```text
   Repository Settings -> Actions -> General -> Workflow permissions
   ```

   Use read and write workflow permissions when the repository policy blocks
   package publishing.

4. After the first image is published, check the package under:

   ```text
   https://github.com/users/Colvin-Y/packages/container/package/kubernetes-ontology
   ```

   Public GHCR packages can be pulled without authentication. If the package is
   private, change the package visibility to Public before documenting it for
   external users.

## Publish A Version

Before tagging, run local validation from a disposable kind cluster and an
explicit kubeconfig. Do not let release checks fall back to an ambient
developer kubeconfig:

```bash
export KO_KIND_KUBECONFIG=/private/tmp/kubernetes-ontology-kind-kubeconfig
kind export kubeconfig --name kind --kubeconfig "${KO_KIND_KUBECONFIG}"
KUBECONFIG="${KO_KIND_KUBECONFIG}" make ci
kubectl --kubeconfig "${KO_KIND_KUBECONFIG}" get nodes
```

Then run the automated in-cluster diagnostic e2e against that same kubeconfig:

```bash
docker build -t kubernetes-ontology:e2e .
kind load docker-image kubernetes-ontology:e2e --name kind
KIND_CLUSTER_NAME=kind bash scripts/ci/verify_kind_e2e.sh
```

The e2e installs the checked-in kind Helm storage sample, deploys the current
Helm chart with the locally built image, and verifies CLI plus viewer diagnostic
queries against the in-cluster daemon.

Use semantic version tags:

```bash
git tag v0.1.6
git push origin v0.1.6
```

Replace `v0.1.6` with the release tag you are publishing.

Pushing the tag starts both workflows:

- `.github/workflows/release.yml` creates a GitHub Release and uploads
  per-platform archives containing `kubernetes-ontology` (CLI),
  `kubernetes-ontologyd` (server), `kubernetes-ontology-viewer`, README files,
  `QUICKSTART.md`, `CHANGELOG.md`, `AI_CONTRACT.md`, `LICENSE`, `NOTICE`,
  `SECURITY.md`, the local config example, and a packaged Helm chart archive.
- `.github/workflows/docker.yml` builds and pushes a multi-architecture image:

  ```text
  ghcr.io/colvin-y/kubernetes-ontology:v0.1.6
  ghcr.io/colvin-y/kubernetes-ontology:0.1.6
  ghcr.io/colvin-y/kubernetes-ontology:latest
  ```

The Docker workflow also supports manual `workflow_dispatch` runs with an
explicit version input, which is useful for retrying image publishing.

## Verify The Release

Check the workflow runs:

```bash
gh run list --workflow Release --limit 5
gh run list --workflow Docker --limit 5
```

Check the release assets:

```bash
gh release view v0.1.6
```

Inspect one binary archive and the Helm chart archive when release packaging
changes:

```bash
gh release download v0.1.6 --pattern 'kubernetes-ontology_v0.1.6_linux_amd64.tar.gz' --clobber
tar -tzf kubernetes-ontology_v0.1.6_linux_amd64.tar.gz | grep -E 'kubernetes-ontologyd$|kubernetes-ontology$|QUICKSTART.md|local/kubernetes-ontology.yaml.example'
gh release download v0.1.6 --pattern 'kubernetes-ontology-0.1.6.tgz' --clobber
helm show chart kubernetes-ontology-0.1.6.tgz
```

Pull the image:

```bash
docker pull ghcr.io/colvin-y/kubernetes-ontology:v0.1.6
```

Deploy through Helm:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --create-namespace \
  --set image.repository=ghcr.io/colvin-y/kubernetes-ontology \
  --set image.tag=v0.1.6 \
  --set cluster=your-logical-cluster \
  --set contextNamespaces='{default,kube-system}'
```

Expose the server locally:

```bash
kubectl -n kubernetes-ontology port-forward svc/kubernetes-ontology 18080:18080
```

Query it with the release CLI:

```bash
kubernetes-ontology --server http://127.0.0.1:18080 --status
```

## Publish The Agent Skill

The skill is distributed from the repository default branch, not from release
archives. Keep marketplace pages pointed at the live repository path so users
get the newest onboarding workflow:

```text
https://github.com/Colvin-Y/kubernetes-ontology/tree/main/skills/kubernetes-ontology-access
```

For each release:

1. Keep `skills/kubernetes-ontology-access/SKILL.md` metadata aligned with the
   release you are documenting.
2. Push the skill and README changes to the default branch before refreshing
   marketplace entries.
3. Ensure GitHub repository topics include:

   ```text
   claude-skills
   claude-code-skill
   agent-skills
   codex-skills
   kubernetes
   devops
   troubleshooting
   ```

   With an authenticated GitHub CLI:

   ```bash
   gh repo edit Colvin-Y/kubernetes-ontology \
     --add-topic claude-skills \
     --add-topic claude-code-skill \
     --add-topic agent-skills \
     --add-topic codex-skills
   ```

4. Refresh the manually submitted registries:

   - `https://skills.re/submit`: submit the repository URL and select
     `skills/kubernetes-ontology-access`.
   - `https://skillhub.pm/submit`: submit as a Skill in the DevOps &
     Infrastructure category.

5. SkillsMP has no manual submit flow yet. It indexes GitHub daily, so verify
   after the next sync:

   ```bash
   curl -fsS 'https://skillsmp.com/api/v1/skills/search?q=kubernetes-ontology-access&limit=10'
   ```

## Optional Docker Hub Mirror

If you later want Docker Hub as an additional mirror:

1. Create a Docker Hub account and repository such as
   `colviny/kubernetes-ontology`.
2. Create a Docker Hub access token.
3. Add GitHub repository secrets:

   ```text
   DOCKERHUB_USERNAME
   DOCKERHUB_TOKEN
   ```

4. Extend `.github/workflows/docker.yml` with a second `docker/login-action`
   step and additional `docker.io/...` image tags.

Keep GHCR as the default path unless there is a specific reason to add the extra
registry.
