# Release Guide

This project publishes binaries through GitHub Releases and container images
through GitHub Container Registry (GHCR).

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

Use semantic version tags:

```bash
git tag v0.1.2
git push origin v0.1.2
```

Replace `v0.1.2` with the release tag you are publishing.

Pushing the tag starts both workflows:

- `.github/workflows/release.yml` creates a GitHub Release and uploads CLI,
  server, and viewer archives.
- `.github/workflows/docker.yml` builds and pushes a multi-architecture image:

  ```text
  ghcr.io/colvin-y/kubernetes-ontology:v0.1.2
  ghcr.io/colvin-y/kubernetes-ontology:0.1.2
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
gh release view v0.1.2
```

Pull the image:

```bash
docker pull ghcr.io/colvin-y/kubernetes-ontology:v0.1.2
```

Deploy through Helm:

```bash
helm upgrade --install kubernetes-ontology ./charts/kubernetes-ontology \
  --namespace kubernetes-ontology \
  --create-namespace \
  --set image.repository=ghcr.io/colvin-y/kubernetes-ontology \
  --set image.tag=v0.1.2 \
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
