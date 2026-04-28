#!/usr/bin/env python3
import argparse
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse


POD_ID = "ci-cluster/core/Pod/default/frontend/pod-uid/_"
NODE_ID = "ci-cluster/core/Node/_/worker-a/node-uid/_"
WORKLOAD_ID = "ci-cluster/apps/Workload/default/Deployment:frontend/deploy-uid/_"

POD = {
    "canonicalId": POD_ID,
    "kind": "Pod",
    "sourceKind": "Pod",
    "namespace": "default",
    "name": "frontend",
    "attributes": {"phase": "Running"},
}
NODE = {
    "canonicalId": NODE_ID,
    "kind": "Node",
    "sourceKind": "Node",
    "name": "worker-a",
}
WORKLOAD = {
    "canonicalId": WORKLOAD_ID,
    "kind": "Workload",
    "sourceKind": "Deployment",
    "namespace": "default",
    "name": "frontend",
}
EDGE = {
    "from": POD_ID,
    "to": NODE_ID,
    "kind": "scheduled_on",
    "provenance": {
        "sourceType": "explicit_ref",
        "state": "asserted",
        "resolver": "ci-fake-server/v1",
    },
}
FRESHNESS = {
    "ready": True,
    "phase": "ready",
    "cluster": "ci-cluster",
    "nodeCount": 3,
    "edgeCount": 1,
}


def status():
    return {
        "Phase": "ready",
        "Cluster": "ci-cluster",
        "Ready": True,
        "NodeCount": 3,
        "EdgeCount": 1,
    }


def graph_response(entry):
    return {
        "entry": entry,
        "nodes": [WORKLOAD, POD, NODE],
        "edges": [EDGE],
        "explanation": ["ci fake topology is ready"],
        "nodeCount": 3,
        "edgeCount": 1,
        "freshness": FRESHNESS,
    }


def diagnostic_entry(kind):
    if kind == "Pod":
        return {
            "kind": "Pod",
            "canonicalId": POD_ID,
            "namespace": "default",
            "name": "frontend",
        }
    if kind == "Workload":
        return {
            "kind": "Workload",
            "canonicalId": WORKLOAD_ID,
            "namespace": "default",
            "name": "frontend",
        }
    return None


def error_response(status_code, code, message, retryable=False):
    return {
        "error": message,
        "message": message,
        "code": code,
        "status": status_code,
        "retryable": retryable,
        "source": "server",
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urlparse(self.path)
        qs = parse_qs(parsed.query)
        path = parsed.path

        if path == "/healthz":
            self.write_json({"ok": True, "phase": "ready", "cluster": "ci-cluster"})
            return
        if path == "/status":
            self.write_json(status())
            return
        if path == "/entities":
            self.write_json({"items": [POD], "count": 1, "freshness": FRESHNESS})
            return
        if path == "/entity":
            if first(qs, "name") == "missing-pod":
                self.write_json(
                    error_response(404, "not_found", "entity not found"),
                    status_code=404,
                )
                return
            self.write_json({"entity": POD, "freshness": FRESHNESS})
            return
        if path == "/relations":
            self.write_json({"items": [EDGE], "count": 1, "freshness": FRESHNESS})
            return
        if path == "/neighbors":
            if not first(qs, "entityGlobalId"):
                self.write_json(
                    error_response(400, "bad_request", "entityGlobalId or id is required"),
                    status_code=400,
                )
                return
            self.write_json({"items": [EDGE], "count": 1, "freshness": FRESHNESS})
            return
        if path == "/expand":
            if not first(qs, "entityGlobalId"):
                self.write_json(
                    error_response(400, "bad_request", "entityGlobalId or id is required"),
                    status_code=400,
                )
                return
            self.write_json({
                "nodes": [POD, NODE],
                "edges": [EDGE],
                "nodeCount": 2,
                "edgeCount": 1,
                "freshness": FRESHNESS,
            })
            return
        if path == "/diagnostic":
            entry = diagnostic_entry(first(qs, "kind"))
            if entry is None:
                self.write_json(
                    error_response(400, "bad_request", "unsupported diagnostic kind"),
                    status_code=400,
                )
                return
            self.write_json(graph_response(entry))
            return
        if path == "/diagnostic/pod":
            self.write_json(graph_response(diagnostic_entry("Pod")))
            return
        if path == "/diagnostic/workload":
            self.write_json(graph_response(diagnostic_entry("Workload")))
            return

        self.write_json(error_response(404, "not_found", "not found"), status_code=404)

    def log_message(self, fmt, *args):
        return

    def write_json(self, payload, status_code=200):
        body = json.dumps(payload).encode()
        self.send_response(status_code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def first(qs, key, default=""):
    values = qs.get(key)
    return values[0] if values else default


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--port-file", required=True)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=0)
    args = parser.parse_args()

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    port_file = Path(args.port_file)
    port_file.write_text(str(server.server_address[1]))
    server.serve_forever()


if __name__ == "__main__":
    main()
