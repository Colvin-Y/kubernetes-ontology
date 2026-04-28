#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHART="${CHART:-${ROOT}/charts/kubernetes-ontology}"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT
touch "${tmpdir}/kubeconfig"
chmod 600 "${tmpdir}/kubeconfig"
export KUBECONFIG="${tmpdir}/kubeconfig"

helm lint "${CHART}"

helm template kubernetes-ontology "${CHART}" \
  --namespace kubernetes-ontology \
  --set image.repository=example.com/kubernetes-ontology \
  --set image.tag=ci \
  --set cluster=ci-cluster \
  --set 'contextNamespaces={default,kube-system}' > "${tmpdir}/default.yaml"

grep -q "kind: ServiceAccount" "${tmpdir}/default.yaml"
grep -q "name: kubernetes-ontology-config" "${tmpdir}/default.yaml"
grep -q "cluster: \"ci-cluster\"" "${tmpdir}/default.yaml"
grep -q -- "- default" "${tmpdir}/default.yaml"
grep -q -- "- kube-system" "${tmpdir}/default.yaml"
grep -q "image: \"example.com/kubernetes-ontology:ci\"" "${tmpdir}/default.yaml"
grep -q "name: kubernetes-ontology-viewer" "${tmpdir}/default.yaml"
grep -q "/kubernetes-ontology-viewer" "${tmpdir}/default.yaml"

helm template kubernetes-ontology "${CHART}" \
  --namespace kubernetes-ontology \
  --set image.repository=example.com/kubernetes-ontology \
  --set image.tag=ci \
  --set viewer.enabled=false > "${tmpdir}/viewer-disabled.yaml"

if grep -q "kubernetes-ontology-viewer" "${tmpdir}/viewer-disabled.yaml"; then
  echo "viewer resources rendered when viewer.enabled=false" >&2
  exit 1
fi

helm template kubernetes-ontology "${CHART}" \
  --namespace kubernetes-ontology \
  --set image.repository=example.com/kubernetes-ontology \
  --set image.tag=ci \
  --set rbac.readSecrets=false > "${tmpdir}/no-secrets.yaml"

if grep -q -- "- secrets" "${tmpdir}/no-secrets.yaml"; then
  echo "secret reads rendered when rbac.readSecrets=false" >&2
  exit 1
fi

echo "helm verification passed"
