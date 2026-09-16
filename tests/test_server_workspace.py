import json
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from tests.helpers import make_sample_event


class ServerWorkspaceTests(unittest.TestCase):
    def test_tombstone_blocks_event_resurrection(self):
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            event = make_sample_event()
            store.insert_if_new(event)
            store.delete_event(event.event_id, "admin deletion")
            self.assertEqual(store.insert_if_new(event), "tombstoned")
            self.assertEqual(store.count(), 0)
            store.close()

    def test_event_store_lists_summarizes_and_exports_structured_data(self):
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            store.insert_if_new(make_sample_event())
            self.assertEqual(len(store.list_events(limit=10)), 1)
            summary = store.summary()
            self.assertEqual(summary["total_events"], 1)
            self.assertEqual(summary["by_event_type"]["process.observed"], 1)
            self.assertIn('"event_type":"process.observed"', store.export("jsonl"))
            self.assertIn("process.observed", store.export("markdown"))
            self.assertIn('"messages"', store.export("openai_messages"))
            self.assertIn('"trajectory"', store.export("agent_trajectory"))
            store.close()

    def test_device_registration_admin_list_and_authenticated_exports(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            store.insert_if_new(make_sample_event())
            server = create_gateway_server(store, token="test-token")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                with urlopen(base + "/admin", timeout=3) as response:
                    self.assertEqual(response.status, 200)
                    self.assertIn("Aetheris Admin Login", response.read().decode("utf-8"))
                register = Request(
                    base + "/v1/devices/register",
                    data=json.dumps({"device_id": "device-test", "client_version": "0.1.0", "hostname": "devbox"}).encode("utf-8"),
                    headers={"Authorization": "Bearer test-token", "Content-Type": "application/json"},
                    method="POST",
                )
                with urlopen(register, timeout=3) as response:
                    self.assertEqual(json.loads(response.read())["status"], "registered")
                with self.assertRaises(HTTPError) as raised:
                    urlopen(base + "/v1/devices", timeout=3)
                self.assertEqual(raised.exception.code, 401)
                request = Request(base + "/v1/devices", headers={"Authorization": "Bearer test-token"})
                with urlopen(request, timeout=3) as response:
                    devices = json.loads(response.read())
                    self.assertEqual(devices["devices"][0]["device_id"], "device-test")
                request = Request(base + "/v1/summary", headers={"Authorization": "Bearer test-token"})
                with urlopen(request, timeout=3) as response:
                    summary = json.loads(response.read())
                    self.assertEqual(summary["total_events"], 1)
                request = Request(base + "/v1/export/events.jsonl", headers={"Authorization": "Bearer test-token"})
                with urlopen(request, timeout=3) as response:
                    self.assertIn("process.observed", response.read().decode("utf-8"))
                request = Request(base + "/v1/pulse", headers={"Authorization": "Bearer test-token"})
                with urlopen(request, timeout=3) as response:
                    self.assertEqual(json.loads(response.read())["metrics"]["total_events"], 1)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()


if __name__ == "__main__":
    unittest.main()
