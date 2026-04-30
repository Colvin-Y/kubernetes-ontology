# Samples

Keep checked-in samples synthetic.

Do not commit:
- real kubeconfig paths
- private cluster names
- real namespace or workload names
- node names, volume handles, or incident transcripts

Available samples:

- `image-pull-demo/`: a synthetic ImagePullBackOff diagnostic demo with
  Kubernetes manifests, an offline viewer graph, and a Chinese walkthrough.
- `kind-helm-storage-demo/`: a kind-focused Helm workload sample with a
  deterministic topology fixture covering PVC, PV, StorageClass, CSIDriver, and
  local-path provisioner evidence.
- `helm-upgrade-failure/`: a synthetic Incident Context Pack v1 fixture for a
  Helm upgrade failure that reached Kubernetes when Helm CLI output is missing.

For internal debugging outputs, use `/tmp` or ignored files under `local/`.
