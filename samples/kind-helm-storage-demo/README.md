# Kind Helm Storage Demo

This sample models a two-node kind cluster running a Helm-installed checkout
workload that mounts two PVCs backed by kind's default `standard` StorageClass
and `rancher.io/local-path` provisioner.

The checked-in `diagnostic-graph.json` is deterministic so the documentation GIF
can be regenerated without a live cluster. To reproduce the object shape in kind,
start Docker Desktop or another Docker daemon first, then run:

```bash
kind create cluster --name ko-storage --config samples/kind-helm-storage-demo/kind-config.yaml
helm upgrade --install checkout samples/kind-helm-storage-demo/chart \
  --namespace payments \
  --create-namespace
kubectl get deploy,rs,pod,svc,cm,secret,sa,rolebinding,pvc,pv,storageclass,csidriver,event -A
```

Open the deterministic topology fixture in the local viewer:

```bash
python3 tools/visualize/server.py --host 127.0.0.1 --port 8765
```

Then visit:

```text
http://127.0.0.1:8765/?file=samples/kind-helm-storage-demo/diagnostic-graph.json
```

Regenerate the documentation animation:

```bash
CAPTURE_SCENARIO=kind-helm-storage CAPTURE_LOCALE=en \
  node samples/image-pull-demo/capture_viewer_gif.mjs
CAPTURE_SCENARIO=kind-helm-storage CAPTURE_LOCALE=zh-CN \
  node samples/image-pull-demo/capture_viewer_gif.mjs
```
