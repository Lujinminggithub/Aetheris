import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class AdapterHealthUploadTests(unittest.TestCase):
    def test_gateway_uploads_health_snapshots_with_device_identity(self):
        from aetheris.gateway import GatewayClient

        captured = {}
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                captured["path"] = self.path
                captured["auth"] = self.headers.get("Authorization")
                captured["body"] = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                body = b'{"accepted":1}'
                self.send_response(202); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)
            def log_message(self, *_args): return
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            result = GatewayClient(f"http://127.0.0.1:{server.server_port}", "token").upload_adapter_health([{"adapter_id": "browser", "state": "error", "error_stage": "url_read", "error_code": "uia_unavailable"}])
            self.assertEqual(result["accepted"], 1)
            self.assertEqual(captured["path"], "/api/v1/device/adapter-health")
            self.assertEqual(captured["auth"], "Bearer token")
            self.assertEqual(captured["body"]["snapshots"][0]["adapter_id"], "browser")
        finally:
            server.shutdown(); server.server_close()
