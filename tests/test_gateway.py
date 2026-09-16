import json
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from tests.helpers import make_sample_event


class GatewayTests(unittest.TestCase):

    def test_client_fetches_application_capture_policy_with_device_token(self):
        from aetheris.gateway import GatewayClient

        captured = {}
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_): pass
            def do_GET(self):
                captured["path"] = self.path
                captured["authorization"] = self.headers.get("Authorization")
                body = json.dumps({"enabled": True, "revision": 4}).encode()
                self.send_response(200); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            result = GatewayClient(f"http://127.0.0.1:{server.server_port}", "device-token").application_capture_policy()
            self.assertEqual(result, {"enabled": True, "revision": 4})
            self.assertEqual(captured, {"path": "/api/v1/device/application-capture-policy", "authorization": "Bearer device-token"})
        finally:
            server.shutdown(); server.server_close()
    def test_client_fetches_project_identity_key_with_device_token(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        from aetheris.gateway import GatewayClient

        requests = []

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                requests.append((self.path, self.headers.get("Authorization")))
                body = json.dumps({"key": "dGVzdA==", "version": 2}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args):
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            result = GatewayClient(f"http://127.0.0.1:{server.server_port}", "device-token").project_identity_key()
            self.assertEqual(result["version"], 2)
            self.assertEqual(requests, [("/api/v1/device/project-identity-key", "Bearer device-token")])
        finally:
            server.shutdown()
            server.server_close()

    def test_client_registers_safe_project_batch_with_device_token(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

        from aetheris.gateway import GatewayClient

        captured = {}

        class Handler(BaseHTTPRequestHandler):
            def do_PUT(self):
                captured["path"] = self.path
                captured["authorization"] = self.headers.get("Authorization")
                captured["body"] = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                body = json.dumps({"registry_revision": 1, "projects": []}).encode()
                self.send_response(200)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args):
                return

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            batch = {"projects": [{"local_project_id": "project-one"}]}
            result = GatewayClient(f"http://127.0.0.1:{server.server_port}", "device-token").register_projects(batch)
            self.assertEqual(result["registry_revision"], 1)
            self.assertEqual(captured, {"path": "/api/v1/device/projects", "authorization": "Bearer device-token", "body": batch})
        finally:
            server.shutdown()
            server.server_close()

    def test_client_fetches_browser_policy_with_device_token(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
        from aetheris.gateway import GatewayClient

        requests = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_): return
            def do_GET(self):
                requests.append((self.path, self.headers.get("Authorization")))
                data = json.dumps({"enabled": True, "allowed_domains": ["docs.example.com"], "revision": 3}).encode()
                self.send_response(200); self.send_header("Content-Length", str(len(data))); self.end_headers(); self.wfile.write(data)

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            client = GatewayClient(f"http://127.0.0.1:{server.server_port}", "issued-token")
            self.assertEqual(client.browser_policy()["revision"], 3)
            self.assertEqual(requests, [("/api/v1/device/browser-policy", "Bearer issued-token")])
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_client_bootstrap_and_heartbeat_use_v1_device_contract(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
        from aetheris.gateway import GatewayClient

        requests = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_): return
            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                body = json.loads(self.rfile.read(length) or b"{}")
                requests.append((self.path, self.headers.get("Authorization"), body))
                response = {"device_token": "issued-token", "device_id": "d1", "subject_id": "s1"} if self.path.endswith("bootstrap") else {"status": "online", "device_id": "d1", "subject_id": "s1"}
                data = json.dumps(response).encode()
                self.send_response(200); self.send_header("Content-Length", str(len(data))); self.end_headers(); self.wfile.write(data)

        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            client = GatewayClient(f"http://127.0.0.1:{server.server_port}", "")
            result = client.bootstrap_device(enrollment_secret="enroll", device_id="d1", subject_id="s1", subject_name="Alice", client_version="0.4.0")
            client.token = result["device_token"]
            heartbeat = client.heartbeat()
            self.assertEqual(heartbeat["status"], "online")
            self.assertEqual(requests[0][0], "/api/v1/device/bootstrap")
            self.assertIsNone(requests[0][1])
            self.assertEqual(requests[1][0], "/api/v1/device/heartbeat")
            self.assertEqual(requests[1][1], "Bearer issued-token")
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_gateway_requires_non_empty_device_token(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            try:
                with self.assertRaises(ValueError):
                    create_gateway_server(store, token="")
            finally:
                store.close()

    def test_gateway_can_read_token_from_file(self):
        from aetheris.gateway import _read_token_file

        with tempfile.TemporaryDirectory() as raw:
            token_file = Path(raw) / "gateway.env"
            token_file.write_text("AETHERIS_TOKEN=file-token\n", encoding="utf-8")
            self.assertEqual(_read_token_file(str(token_file)), "file-token")

    def test_gateway_can_read_bom_prefixed_token_file(self):
        from aetheris.gateway import _read_token_file

        with tempfile.TemporaryDirectory() as raw:
            token_file = Path(raw) / "gateway.env"
            token_file.write_bytes("AETHERIS_TOKEN=bom-token\n".encode("utf-8-sig"))
            self.assertEqual(_read_token_file(str(token_file)), "bom-token")

    def test_gateway_accepts_event_once_and_rejects_bad_token(self):
        from aetheris.gateway import GatewayClient, create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            server = create_gateway_server(store, token="test-token")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                client = GatewayClient(f"http://127.0.0.1:{server.server_port}", "test-token")
                sample_event = make_sample_event()
                self.assertEqual(client.send([sample_event])[0]["status"], "accepted")
                self.assertEqual(client.send([sample_event])[0]["status"], "duplicate")
                self.assertEqual(store.count(), 1)

                request = Request(
                    f"http://127.0.0.1:{server.server_port}/v1/events",
                    data=json.dumps({"events": []}).encode("utf-8"),
                    headers={"Authorization": "Bearer wrong", "Content-Type": "application/json"},
                    method="POST",
                )
                with self.assertRaises(HTTPError) as raised:
                    urlopen(request, timeout=3)
                self.assertEqual(raised.exception.code, 401)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()


if __name__ == "__main__":
    unittest.main()
