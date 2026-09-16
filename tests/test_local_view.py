import json
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.request import urlopen

from tests.helpers import make_sample_event


class LocalViewTests(unittest.TestCase):
    def test_local_view_accepts_preferred_port(self):
        from aetheris.local_view import create_local_view_server

        server = create_local_view_server(lambda: [], port=15474)
        try:
            self.assertEqual(server.server_port, 15474)
        finally:
            server.server_close()
    def test_local_view_exposes_structured_core_status(self):
        from aetheris.local_view import create_local_view_server
        server = create_local_view_server(lambda: [], lambda: {"registration": {"state": "registered"}})
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            with urlopen(f"http://127.0.0.1:{server.server_port}/api/status", timeout=3) as response:
                self.assertEqual(json.loads(response.read())["registration"]["state"], "registered")
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_local_view_can_include_bounded_service_status(self):
        from aetheris.local_view import create_local_view_server
        server = create_local_view_server(lambda: [], lambda: {"service": {"state": "connected", "session_id": 2}})
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            with urlopen(f"http://127.0.0.1:{server.server_port}/api/status", timeout=3) as response:
                service = json.loads(response.read())["service"]
                self.assertEqual(service, {"state": "connected", "session_id": 2})
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_local_history_survives_upload_ack(self):
        from aetheris.queue import LocalQueue

        with tempfile.TemporaryDirectory() as raw:
            queue = LocalQueue(Path(raw) / "client.db")
            event = make_sample_event()
            queue.enqueue(event)
            queue.claim_batch(1)
            queue.ack([event.event_id])
            self.assertEqual(queue.history(limit=10)[0]["event_id"], event.event_id)
            queue.close()

    def test_local_view_binds_to_loopback_and_shows_recent_events(self):
        from aetheris.local_view import create_local_view_server

        server = create_local_view_server(lambda: [make_sample_event().to_dict()])
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            self.assertEqual(server.server_address[0], "127.0.0.1")
            with urlopen(f"http://127.0.0.1:{server.server_port}/", timeout=3) as response:
                self.assertIn("Aetheris Local Workspace", response.read().decode("utf-8"))
            with urlopen(f"http://127.0.0.1:{server.server_port}/api/events", timeout=3) as response:
                self.assertEqual(json.loads(response.read())["events"][0]["event_id"], "event-test-1")
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
