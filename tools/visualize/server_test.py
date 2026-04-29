import importlib.util
import json
import threading
import time
import unittest
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path


SPEC = importlib.util.spec_from_file_location("visualize_server", Path(__file__).with_name("server.py"))
visualize_server = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(visualize_server)


class VisualizeServerTest(unittest.TestCase):
    def test_root_injects_default_ontology_server(self):
        previous = visualize_server.DEFAULT_ONTOLOGY_SERVER
        visualize_server.DEFAULT_ONTOLOGY_SERVER = "http://kubernetes-ontology:18080"
        viewer = running_server(visualize_server.Handler)
        try:
            body = urllib.request.urlopen(viewer.url + "/").read().decode()
            self.assertIn('value="http://kubernetes-ontology:18080"', body)
            self.assertNotIn(
                "el.serverUrl.value === 'http://kubernetes-ontology:18080'",
                body,
            )
        finally:
            viewer.close()
            visualize_server.DEFAULT_ONTOLOGY_SERVER = previous

    def test_diagnostic_requires_namespace_before_upstream_request(self):
        hits = []
        upstream = running_server(make_upstream(hits=hits))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/diagnostic?" + urllib.parse.urlencode({
                "server": upstream.url,
                "kind": "Pod",
                "name": "frontend",
            })
            with self.assertRaises(urllib.error.HTTPError) as raised:
                urllib.request.urlopen(url)
            self.assertEqual(raised.exception.code, 400)
            self.assertEqual(hits, [])
        finally:
            viewer.close()
            upstream.close()

    def test_diagnostic_preserves_upstream_status_and_error(self):
        upstream = running_server(make_upstream(status=404, payload={"error": "entry not found"}))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/diagnostic?" + urllib.parse.urlencode({
                "server": upstream.url,
                "kind": "Pod",
                "namespace": "default",
                "name": "frontend",
            })
            with self.assertRaises(urllib.error.HTTPError) as raised:
                urllib.request.urlopen(url)
            self.assertEqual(raised.exception.code, 404)
            payload = json.loads(raised.exception.read().decode())
            self.assertEqual(payload["error"], "entry not found")
        finally:
            viewer.close()
            upstream.close()

    def test_diagnostic_allows_cluster_scoped_kind_without_namespace(self):
        hits = []
        upstream = running_server(make_upstream(hits=hits))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/diagnostic?" + urllib.parse.urlencode({
                "server": upstream.url,
                "kind": "PV",
                "name": "pv-data",
            })
            body = urllib.request.urlopen(url).read()
            self.assertTrue(json.loads(body))
            self.assertEqual(len(hits), 1)
            parsed = urllib.parse.urlparse(hits[0])
            self.assertEqual(parsed.path, "/diagnostic")
            qs = urllib.parse.parse_qs(parsed.query)
            self.assertEqual(qs["kind"][0], "PV")
            self.assertEqual(qs["name"][0], "pv-data")
            self.assertNotIn("namespace", qs)
        finally:
            viewer.close()
            upstream.close()

    def test_diagnostic_omits_namespace_for_cluster_scoped_kind(self):
        hits = []
        upstream = running_server(make_upstream(hits=hits))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/diagnostic?" + urllib.parse.urlencode({
                "server": upstream.url,
                "kind": "StorageClass",
                "namespace": "default",
                "name": "fast",
            })
            body = urllib.request.urlopen(url).read()
            self.assertTrue(json.loads(body))
            parsed = urllib.parse.urlparse(hits[0])
            qs = urllib.parse.parse_qs(parsed.query)
            self.assertEqual(qs["kind"][0], "StorageClass")
            self.assertEqual(qs["name"][0], "fast")
            self.assertNotIn("namespace", qs)
        finally:
            viewer.close()
            upstream.close()

    def test_diagnostic_timeout_returns_504(self):
        previous = visualize_server.UPSTREAM_TIMEOUT_SECONDS
        visualize_server.UPSTREAM_TIMEOUT_SECONDS = 0.05
        upstream = running_server(make_upstream(delay=0.2))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/diagnostic?" + urllib.parse.urlencode({
                "server": upstream.url,
                "kind": "Pod",
                "namespace": "default",
                "name": "frontend",
            })
            with self.assertRaises(urllib.error.HTTPError) as raised:
                urllib.request.urlopen(url)
            self.assertEqual(raised.exception.code, 504)
            payload = json.loads(raised.exception.read().decode())
            self.assertIn("timed out", payload["error"])
        finally:
            viewer.close()
            upstream.close()
            visualize_server.UPSTREAM_TIMEOUT_SECONDS = previous

    def test_expand_proxies_entity_and_depth(self):
        hits = []
        upstream = running_server(make_upstream(hits=hits))
        viewer = running_server(visualize_server.Handler)
        try:
            url = viewer.url + "/expand?" + urllib.parse.urlencode({
                "server": upstream.url,
                "entityGlobalId": "cluster/core/Pod/default/frontend/p1/_",
                "depth": "1",
                "direction": "both",
                "limit": "20",
            })
            body = urllib.request.urlopen(url).read()
            self.assertTrue(json.loads(body))
            self.assertEqual(len(hits), 1)
            parsed = urllib.parse.urlparse(hits[0])
            self.assertEqual(parsed.path, "/expand")
            qs = urllib.parse.parse_qs(parsed.query)
            self.assertEqual(qs["entityGlobalId"][0], "cluster/core/Pod/default/frontend/p1/_")
            self.assertEqual(qs["depth"][0], "1")
            self.assertEqual(qs["direction"][0], "both")
            self.assertEqual(qs["limit"][0], "20")
        finally:
            viewer.close()
            upstream.close()


def make_upstream(status=200, payload=None, delay=0, hits=None):
    payload = payload or {
        "entry": {"kind": "Pod", "namespace": "default", "name": "frontend", "canonicalId": "pod-id"},
        "nodes": [],
        "edges": [],
        "explanation": [],
    }

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if hits is not None:
                hits.append(self.path)
            if delay:
                time.sleep(delay)
            body = json.dumps(payload).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try:
                self.wfile.write(body)
            except BrokenPipeError:
                pass

        def log_message(self, format, *args):
            return

    return Handler


class running_server:
    def __init__(self, handler):
        self.httpd = ThreadingHTTPServer(("127.0.0.1", 0), handler)
        host, port = self.httpd.server_address
        self.url = f"http://{host}:{port}"
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        self.httpd.shutdown()
        self.httpd.server_close()
        self.thread.join(timeout=1)


if __name__ == "__main__":
    unittest.main()
