#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

make -C "${ROOT}" build build-daemon build-viewer

for binary in \
  "${ROOT}/bin/kubernetes-ontology" \
  "${ROOT}/bin/kubernetes-ontologyd" \
  "${ROOT}/bin/kubernetes-ontology-viewer"; do
  if [ ! -x "${binary}" ]; then
    echo "missing executable binary: ${binary}" >&2
    exit 1
  fi
  name="$(basename "${binary}")"
  "${binary}" --help > "${tmpdir}/${name}.help" 2>&1
  grep -q "Usage of" "${tmpdir}/${name}.help"
  go version -m "${binary}" > "${tmpdir}/${name}.version"
  grep -q "path.*github.com/Colvin-Y/kubernetes-ontology/cmd/${name}" "${tmpdir}/${name}.version"
done

echo "binary verification passed"
