#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLI="${CLI:-${ROOT}/bin/kubernetes-ontology}"
PYTHON="${PYTHON:-python3}"
POD_ID="ci-cluster/core/Pod/default/frontend/pod-uid/_"

tmpdir="$(mktemp -d)"
server_pid=""
cleanup() {
  if [ -n "${server_pid}" ]; then
    kill "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

"${PYTHON}" "${ROOT}/scripts/ci/fake_ontology_server.py" --port-file "${tmpdir}/port" &
server_pid="$!"

for _ in $(seq 1 100); do
  if [ -s "${tmpdir}/port" ]; then
    break
  fi
  sleep 0.05
done

if [ ! -s "${tmpdir}/port" ]; then
  echo "fake ontology server did not start" >&2
  exit 1
fi

server_url="http://127.0.0.1:$(cat "${tmpdir}/port")"

"${CLI}" --server "${server_url}" --status > "${tmpdir}/status.json"
"${PYTHON}" - "${tmpdir}/status.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["Ready"] is True, data
assert data["Phase"] == "ready", data
assert data["Cluster"] == "ci-cluster", data
PY

"${CLI}" \
  --server "${server_url}" \
  --list-entities \
  --entity-kind Pod \
  --namespace default \
  --limit 20 > "${tmpdir}/entities.json"
"${PYTHON}" - "${tmpdir}/entities.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["count"] == 1, data
assert data["items"][0]["kind"] == "Pod", data
assert data["freshness"]["ready"] is True, data
PY

"${CLI}" \
  --server "${server_url}" \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name frontend > "${tmpdir}/entity.json"
"${PYTHON}" - "${tmpdir}/entity.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["entity"]["name"] == "frontend", data
assert data["entity"]["canonicalId"], data
PY

"${CLI}" \
  --server "${server_url}" \
  --list-filtered-relations \
  --from "${POD_ID}" \
  --relation-kind scheduled_on \
  --limit 50 > "${tmpdir}/relations.json"
"${PYTHON}" - "${tmpdir}/relations.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["count"] == 1, data
assert data["items"][0]["kind"] == "scheduled_on", data
PY

"${CLI}" \
  --server "${server_url}" \
  --neighbors \
  --entity-id "${POD_ID}" \
  --direction out > "${tmpdir}/neighbors.json"
"${PYTHON}" - "${tmpdir}/neighbors.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["count"] == 1, data
assert data["items"][0]["to"].endswith("/worker-a/node-uid/_"), data
PY

"${CLI}" \
  --server "${server_url}" \
  --expand-entity \
  --entity-id "${POD_ID}" \
  --expand-depth 1 > "${tmpdir}/expand.json"
"${PYTHON}" - "${tmpdir}/expand.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["nodeCount"] == 2, data
assert data["edgeCount"] == 1, data
PY

"${CLI}" \
  --server "${server_url}" \
  --diagnose-pod \
  --namespace default \
  --name frontend > "${tmpdir}/diagnostic-pod.json"
"${PYTHON}" - "${tmpdir}/diagnostic-pod.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["entry"]["kind"] == "Pod", data
assert data["nodeCount"] == 3, data
assert data["freshness"]["ready"] is True, data
PY

"${CLI}" \
  --server "${server_url}" \
  --diagnose-workload \
  --namespace default \
  --name frontend > "${tmpdir}/diagnostic-workload.json"
"${PYTHON}" - "${tmpdir}/diagnostic-workload.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["entry"]["kind"] == "Workload", data
assert data["nodeCount"] == 3, data
PY

if "${CLI}" \
  --server "${server_url}" \
  --machine-errors \
  --resolve-entity \
  --entity-kind Pod \
  --namespace default \
  --name missing-pod > "${tmpdir}/missing.out" 2> "${tmpdir}/missing.err"; then
  echo "expected missing entity query to fail" >&2
  exit 1
fi
"${PYTHON}" - "${tmpdir}/missing.err" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["code"] == "not_found", data
assert data["status"] == 404, data
assert data["source"] == "server", data
PY

echo "client verification passed"
