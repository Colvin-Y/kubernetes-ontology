#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KO_NAMESPACE="${KO_NAMESPACE:-kubernetes-ontology}"
APP_NAMESPACE="${APP_NAMESPACE:-payments}"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-ko-e2e}"
KO_IMAGE_REPOSITORY="${KO_IMAGE_REPOSITORY:-kubernetes-ontology}"
KO_IMAGE_TAG="${KO_IMAGE_TAG:-e2e}"
SERVER_URL=""
VIEWER_URL=""

tmpdir="$(mktemp -d)"
server_pf_pid=""
viewer_pf_pid=""

cleanup() {
  if [ -n "${viewer_pf_pid}" ]; then
    kill "${viewer_pf_pid}" 2>/dev/null || true
    wait "${viewer_pf_pid}" 2>/dev/null || true
  fi
  if [ -n "${server_pf_pid}" ]; then
    kill "${server_pf_pid}" 2>/dev/null || true
    wait "${server_pf_pid}" 2>/dev/null || true
  fi
  rm -rf "${tmpdir}"
}

diagnostics() {
  echo "kind e2e failed; collecting diagnostics" >&2
  kubectl get nodes -o wide >&2 || true
  kubectl get pods -A -o wide >&2 || true
  kubectl -n "${KO_NAMESPACE}" get events --sort-by=.lastTimestamp >&2 || true
  kubectl -n "${APP_NAMESPACE}" get events --sort-by=.lastTimestamp >&2 || true
  kubectl -n "${KO_NAMESPACE}" logs deploy/kubernetes-ontology --tail=200 >&2 || true
  kubectl -n "${KO_NAMESPACE}" logs deploy/kubernetes-ontology-viewer --tail=200 >&2 || true
  [ -f "${tmpdir}/server-port-forward.log" ] && cat "${tmpdir}/server-port-forward.log" >&2 || true
  [ -f "${tmpdir}/viewer-port-forward.log" ] && cat "${tmpdir}/viewer-port-forward.log" >&2 || true
}

trap cleanup EXIT
trap diagnostics ERR

free_port() {
  python3 - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

wait_for_http() {
  local url="$1"
  local out="$2"
  for _ in $(seq 1 120); do
    if curl -fsS "${url}" > "${out}"; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for ${url}" >&2
  return 1
}

if ! kubectl cluster-info >/dev/null 2>&1; then
  echo "kubectl cannot reach a Kubernetes cluster; create kind cluster ${CLUSTER_NAME} first" >&2
  exit 1
fi

make -C "${ROOT}" build

helm upgrade --install checkout "${ROOT}/samples/kind-helm-storage-demo/chart" \
  --namespace "${APP_NAMESPACE}" \
  --create-namespace \
  --wait \
  --timeout 4m

kubectl -n "${APP_NAMESPACE}" rollout status deploy/checkout-api --timeout=4m
POD_NAME="$(kubectl -n "${APP_NAMESPACE}" get pods \
  -l app.kubernetes.io/name=checkout-api \
  -o jsonpath='{.items[0].metadata.name}')"
if [ -z "${POD_NAME}" ]; then
  echo "checkout-api pod was not created" >&2
  exit 1
fi

helm upgrade --install kubernetes-ontology "${ROOT}/charts/kubernetes-ontology" \
  --namespace "${KO_NAMESPACE}" \
  --create-namespace \
  --set image.repository="${KO_IMAGE_REPOSITORY}" \
  --set image.tag="${KO_IMAGE_TAG}" \
  --set image.pullPolicy=IfNotPresent \
  --set cluster="${CLUSTER_NAME}" \
  --set 'contextNamespaces={payments,kube-system,local-path-storage}' \
  --set rbac.readSecrets=false \
  --wait \
  --timeout 4m

kubectl -n "${KO_NAMESPACE}" rollout status deploy/kubernetes-ontology --timeout=4m
kubectl -n "${KO_NAMESPACE}" rollout status deploy/kubernetes-ontology-viewer --timeout=4m

server_port="$(free_port)"
kubectl -n "${KO_NAMESPACE}" port-forward svc/kubernetes-ontology "${server_port}:18080" \
  > "${tmpdir}/server-port-forward.log" 2>&1 &
server_pf_pid="$!"
SERVER_URL="http://127.0.0.1:${server_port}"
wait_for_http "${SERVER_URL}/status" "${tmpdir}/server-status-http.json"

"${ROOT}/bin/kubernetes-ontology" --server "${SERVER_URL}" --status > "${tmpdir}/status.json"
python3 - "${tmpdir}/status.json" "${CLUSTER_NAME}" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["Ready"] is True, data
assert data["Phase"] == "ready", data
assert data["Cluster"] == sys.argv[2], data
assert data["NodeCount"] > 0, data
assert data["EdgeCount"] > 0, data
PY

"${ROOT}/bin/kubernetes-ontology" \
  --server "${SERVER_URL}" \
  --list-entities \
  --entity-kind Pod \
  --namespace "${APP_NAMESPACE}" \
  --limit 20 > "${tmpdir}/pods.json"
python3 - "${tmpdir}/pods.json" "${POD_NAME}" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
pod_name = sys.argv[2]
assert data["count"] >= 1, data
assert data["freshness"]["ready"] is True, data
assert any(item.get("name") == pod_name for item in data["items"]), data
PY

"${ROOT}/bin/kubernetes-ontology" \
  --server "${SERVER_URL}" \
  --diagnose-pod \
  --namespace "${APP_NAMESPACE}" \
  --name "${POD_NAME}" \
  --max-depth 3 \
  --storage-max-depth 6 \
  --max-nodes 120 \
  --max-edges 240 > "${tmpdir}/pod-diagnostic.json"
python3 - "${tmpdir}/pod-diagnostic.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
edge_kinds = {edge["kind"] for edge in data["edges"]}
node_kinds = {node["kind"] for node in data["nodes"]}
assert data["entry"]["kind"] == "Pod", data
assert data["partial"] is False, data
assert "Workload" in node_kinds, node_kinds
assert "PVC" in node_kinds, node_kinds
assert "mounts_pvc" in edge_kinds, edge_kinds
assert "managed_by_helm_release" in edge_kinds, edge_kinds
PY

"${ROOT}/bin/kubernetes-ontology" \
  --server "${SERVER_URL}" \
  --diagnose-helm-release \
  --namespace "${APP_NAMESPACE}" \
  --name checkout \
  --max-depth 3 \
  --storage-max-depth 6 \
  --max-nodes 120 \
  --max-edges 240 > "${tmpdir}/helm-diagnostic.json"
python3 - "${tmpdir}/helm-diagnostic.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
edge_kinds = {edge["kind"] for edge in data["edges"]}
node_kinds = {node["kind"] for node in data["nodes"]}
assert data["entry"]["kind"] == "HelmRelease", data
assert "HelmChart" in node_kinds, node_kinds
assert "installs_chart" in edge_kinds, edge_kinds
PY

viewer_port="$(free_port)"
kubectl -n "${KO_NAMESPACE}" port-forward svc/kubernetes-ontology-viewer "${viewer_port}:8765" \
  > "${tmpdir}/viewer-port-forward.log" 2>&1 &
viewer_pf_pid="$!"
VIEWER_URL="http://127.0.0.1:${viewer_port}"
wait_for_http "${VIEWER_URL}/" "${tmpdir}/viewer-index.html"

curl -fsS "${VIEWER_URL}/topology?entityLimit=40&relationLimit=120" > "${tmpdir}/viewer-topology.json"
python3 - "${tmpdir}/viewer-topology.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["source"] == "server", data
assert data["status"]["Ready"] is True, data
assert data["entities"]["count"] > 0, data
assert data["relations"]["count"] > 0, data
PY

curl -fsS "${VIEWER_URL}/diagnostic?kind=Pod&namespace=${APP_NAMESPACE}&name=${POD_NAME}&maxDepth=3&storageMaxDepth=6" \
  > "${tmpdir}/viewer-diagnostic.json"
python3 - "${tmpdir}/viewer-diagnostic.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["entry"]["kind"] == "Pod", data
assert data["nodeCount"] > 0, data
assert data["edgeCount"] > 0, data
PY

echo "kind e2e verification passed"
