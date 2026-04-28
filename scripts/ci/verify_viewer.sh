#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VIEWER="${VIEWER:-${ROOT}/bin/kubernetes-ontology-viewer}"
PYTHON="${PYTHON:-python3}"
POD_ID="ci-cluster/core/Pod/default/frontend/pod-uid/_"

tmpdir="$(mktemp -d)"
server_pid=""
viewer_pid=""
cleanup() {
  if [ -n "${viewer_pid}" ]; then
    kill "${viewer_pid}" 2>/dev/null || true
    wait "${viewer_pid}" 2>/dev/null || true
  fi
  if [ -n "${server_pid}" ]; then
    kill "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

free_port() {
  "${PYTHON}" - <<'PY'
import socket

sock = socket.socket()
sock.bind(("127.0.0.1", 0))
print(sock.getsockname()[1])
sock.close()
PY
}

urlencode() {
  "${PYTHON}" - "$1" <<'PY'
import sys
import urllib.parse

print(urllib.parse.quote(sys.argv[1], safe=""))
PY
}

"${PYTHON}" "${ROOT}/scripts/ci/fake_ontology_server.py" --port-file "${tmpdir}/server-port" &
server_pid="$!"

for _ in $(seq 1 100); do
  if [ -s "${tmpdir}/server-port" ]; then
    break
  fi
  sleep 0.05
done
if [ ! -s "${tmpdir}/server-port" ]; then
  echo "fake ontology server did not start" >&2
  exit 1
fi

server_url="http://127.0.0.1:$(cat "${tmpdir}/server-port")"
viewer_port="$(free_port)"
viewer_url="http://127.0.0.1:${viewer_port}"

"${VIEWER}" \
  --host 127.0.0.1 \
  --port "${viewer_port}" \
  --server "${server_url}" \
  --upstream-timeout 2s > "${tmpdir}/viewer.log" 2>&1 &
viewer_pid="$!"

for _ in $(seq 1 100); do
  if curl -fsS "${viewer_url}/" > "${tmpdir}/index.html"; then
    break
  fi
  sleep 0.05
done
if [ ! -s "${tmpdir}/index.html" ]; then
  echo "viewer did not serve index" >&2
  cat "${tmpdir}/viewer.log" >&2 || true
  exit 1
fi

grep -q "Load topology" "${tmpdir}/index.html"
grep -q "${server_url}" "${tmpdir}/index.html"

curl -fsS "${viewer_url}/topology?entityLimit=20&relationLimit=20" > "${tmpdir}/topology.json"
"${PYTHON}" - "${tmpdir}/topology.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["source"] == "server", data
assert data["status"]["Ready"] is True, data
assert data["entities"]["count"] == 1, data
assert data["relations"]["count"] == 1, data
PY

curl -fsS "${viewer_url}/diagnostic?kind=Pod&namespace=default&name=frontend" > "${tmpdir}/diagnostic.json"
"${PYTHON}" - "${tmpdir}/diagnostic.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["entry"]["kind"] == "Pod", data
assert data["nodeCount"] == 3, data
PY

encoded_pod_id="$(urlencode "${POD_ID}")"
curl -fsS "${viewer_url}/expand?entityGlobalId=${encoded_pod_id}&depth=1" > "${tmpdir}/expand.json"
"${PYTHON}" - "${tmpdir}/expand.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["nodeCount"] == 2, data
assert data["edgeCount"] == 1, data
PY

sample_path="$(urlencode "${ROOT}/samples/image-pull-demo/diagnostic-graph.json")"
curl -fsS "${viewer_url}/load?path=${sample_path}" > "${tmpdir}/sample.json"
"${PYTHON}" - "${tmpdir}/sample.json" <<'PY'
import json
import sys

data = json.load(open(sys.argv[1]))
assert data["source"] == "sample/image-pull-demo", data
assert data["entry"]["kind"] == "Pod", data
assert len(data["nodes"]) > 0, data
assert len(data["edges"]) > 0, data
PY

echo "viewer verification passed"
