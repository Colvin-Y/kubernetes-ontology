#!/usr/bin/env python3
import argparse
import html as html_lib
import json
import os
import socket
import urllib.parse
import urllib.error
import urllib.request
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
VIEWER = ROOT / "tools" / "visualize" / "index.html"
VERSION = str(int(VIEWER.stat().st_mtime))
DEFAULT_ONTOLOGY_SERVER = os.environ.get("ONTOLOGY_SERVER", "http://127.0.0.1:18080")
NAMESPACED_DIAGNOSTIC_KINDS = {"pod", "workload", "pvc"}
CLUSTER_SCOPED_DIAGNOSTIC_KINDS = {"pv", "storageclass", "csidriver"}


def float_env(name, default):
    try:
        value = float(os.environ.get(name, default))
    except ValueError:
        return default
    return value if value > 0 else default


UPSTREAM_TIMEOUT_SECONDS = float_env("VIEWER_UPSTREAM_TIMEOUT_SECONDS", 30)


class UpstreamHTTPError(Exception):
    def __init__(self, status, message):
        super().__init__(message)
        self.status = status
        self.message = message


class UpstreamTimeoutError(TimeoutError):
    pass


class Handler(SimpleHTTPRequestHandler):
    def end_headers(self):
        self.send_header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
        self.send_header("Pragma", "no-cache")
        self.send_header("Expires", "0")
        super().end_headers()

    def do_GET(self):
        parsed = urllib.parse.urlparse(self.path)
        if parsed.path == "/":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            html = (
                VIEWER.read_text()
                .replace("__VIEWER_VERSION__", VERSION)
                .replace("__ONTOLOGY_SERVER__", html_lib.escape(DEFAULT_ONTOLOGY_SERVER, quote=True))
            )
            self.wfile.write(html.encode())
            return
        if parsed.path == "/topology":
            qs = urllib.parse.parse_qs(parsed.query)
            server = first(qs, "server", DEFAULT_ONTOLOGY_SERVER)
            namespace = first(qs, "namespace")
            kind = first(qs, "kind")
            entity_limit = first(qs, "entityLimit", "1000")
            relation_limit = first(qs, "relationLimit", "5000")
            try:
                status = fetch_json(server, "/status")
                entity_params = {"limit": entity_limit}
                if namespace:
                    entity_params["namespace"] = namespace
                if kind:
                    entity_params["kind"] = kind
                entities = fetch_json(server, "/entities?" + urllib.parse.urlencode(entity_params))
                relations = fetch_json(server, "/relations?" + urllib.parse.urlencode({"limit": relation_limit}))
            except Exception as e:
                self._upstream_error(e)
                return
            self._json({
                "source": "server",
                "server": server,
                "status": status,
                "entities": entities,
                "relations": relations,
            }, 200)
            return
        if parsed.path == "/diagnostic":
            qs = urllib.parse.parse_qs(parsed.query)
            server = first(qs, "server", DEFAULT_ONTOLOGY_SERVER)
            kind = first(qs, "kind", "Pod")
            kind_key = kind.lower()
            namespace = first(qs, "namespace")
            name = first(qs, "name")
            max_depth = first(qs, "maxDepth", "2")
            storage_max_depth = first(qs, "storageMaxDepth", "5")
            terminal_kinds = first(qs, "terminalKinds")
            expand_terminal_nodes = first(qs, "expandTerminalNodes")
            if kind_key in CLUSTER_SCOPED_DIAGNOSTIC_KINDS:
                namespace = ""
            if not name:
                self._json({"error": "name is required"}, 400)
                return
            if not namespace and kind_key in NAMESPACED_DIAGNOSTIC_KINDS:
                self._json({"error": "namespace is required"}, 400)
                return
            params = {
                "kind": kind,
                "name": name,
                "maxDepth": max_depth,
                "storageMaxDepth": storage_max_depth,
            }
            if namespace:
                params["namespace"] = namespace
            if terminal_kinds:
                params["terminalKinds"] = terminal_kinds
            if expand_terminal_nodes:
                params["expandTerminalNodes"] = expand_terminal_nodes
            try:
                data = fetch_json(server, "/diagnostic?" + urllib.parse.urlencode(params))
            except Exception as e:
                self._upstream_error(e)
                return
            self._json(data, 200)
            return
        if parsed.path == "/expand":
            qs = urllib.parse.parse_qs(parsed.query)
            server = first(qs, "server", DEFAULT_ONTOLOGY_SERVER)
            entity_id = first(qs, "entityGlobalId", first(qs, "id"))
            depth = first(qs, "depth", "1")
            direction = first(qs, "direction")
            kind = first(qs, "kind")
            limit = first(qs, "limit")
            if not entity_id:
                self._json({"error": "entityGlobalId or id is required"}, 400)
                return
            params = {
                "entityGlobalId": entity_id,
                "depth": depth,
            }
            if direction:
                params["direction"] = direction
            if kind:
                params["kind"] = kind
            if limit:
                params["limit"] = limit
            try:
                data = fetch_json(server, "/expand?" + urllib.parse.urlencode(params))
            except Exception as e:
                self._upstream_error(e)
                return
            self._json(data, 200)
            return
        if parsed.path == "/proxy":
            qs = urllib.parse.parse_qs(parsed.query)
            server = first(qs, "server", DEFAULT_ONTOLOGY_SERVER)
            path = first(qs, "path")
            if not path.startswith("/"):
                self._json({"error": "path must start with /"}, 400)
                return
            try:
                data = fetch_bytes(server, path)
            except Exception as e:
                self._upstream_error(e)
                return
            self.send_response(200)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)
            return
        if parsed.path == "/load":
            qs = urllib.parse.parse_qs(parsed.query)
            raw = qs.get("path", [""])[0]
            path = Path(raw).expanduser()
            if not path.exists():
                self._json({"error": f"file not found: {path}"}, 404)
                return
            if not path.is_file():
                self._json({"error": f"not a file: {path}"}, 400)
                return
            try:
                with path.open() as f:
                    data = json.load(f)
            except Exception as e:
                self._json({"error": f"failed to read json: {e}"}, 400)
                return
            self._json(data, 200)
            return
        return super().do_GET()

    def _json(self, data, status):
        payload = json.dumps(data).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def _upstream_error(self, error):
        if isinstance(error, UpstreamHTTPError):
            self._json({"error": error.message}, error.status)
            return
        if isinstance(error, UpstreamTimeoutError):
            self._json({"error": str(error)}, 504)
            return
        self._json({"error": str(error)}, 502)


def first(qs, name, default=""):
    values = qs.get(name)
    if not values:
        return default
    return values[0]


def fetch_json(server, path):
    return json.loads(fetch_bytes(server, path).decode())


def fetch_bytes(server, path, timeout=None):
    parsed = urllib.parse.urlparse(server)
    if parsed.scheme not in ("http", "https"):
        raise ValueError("server must use http or https")
    if not path.startswith("/"):
        raise ValueError("path must start with /")
    url = server.rstrip("/") + path
    request = urllib.request.Request(url, headers={"Accept": "application/json"})
    try:
        with urllib.request.urlopen(request, timeout=timeout or UPSTREAM_TIMEOUT_SECONDS) as response:
            return response.read()
    except urllib.error.HTTPError as e:
        raise UpstreamHTTPError(e.code, upstream_error_message(e)) from e
    except Exception as e:
        if is_timeout_error(e):
            raise UpstreamTimeoutError(f"upstream request timed out after {timeout or UPSTREAM_TIMEOUT_SECONDS:g}s: {url}") from e
        raise


def upstream_error_message(error):
    body = error.read()
    if body:
        try:
            payload = json.loads(body.decode())
            if isinstance(payload, dict) and payload.get("error"):
                return str(payload["error"])
        except Exception:
            return body.decode(errors="replace")
    return f"upstream returned HTTP {error.code}"


def is_timeout_error(error):
    if isinstance(error, (TimeoutError, socket.timeout)):
        return True
    if isinstance(error, urllib.error.URLError):
        return isinstance(error.reason, (TimeoutError, socket.timeout))
    return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    args = parser.parse_args()

    httpd = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"viewer server listening on http://{args.host}:{args.port}")
    httpd.serve_forever()

if __name__ == "__main__":
    main()
