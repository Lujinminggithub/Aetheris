import tempfile
import threading
import time
import unittest
from pathlib import Path


class VerticalSliceTests(unittest.TestCase):
    def test_core_to_gateway_to_server_contains_only_redacted_content(self):
        from aetheris.cli import run_capture_and_upload
        from aetheris.core import ProcessObservation
        from aetheris.gateway import GatewayClient, create_gateway_server
        from aetheris.queue import LocalQueue
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"
            (project / ".git").mkdir(parents=True)
            queue = LocalQueue(root / "client.db")
            store = EventStore(root / "server.db")
            server = create_gateway_server(store, token="test-token")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                result = run_capture_and_upload(
                    ProcessObservation(1, "Code.exe", None, None),
                    project,
                    [project],
                    queue,
                    GatewayClient(f"http://127.0.0.1:{server.server_port}", "test-token"),
                    raw_payload={"diagnostic": "Bearer abc123", "name": "code.exe"},
                )
                self.assertEqual(result["accepted"], 1)
                row = store.fetch_all()[0]
                self.assertNotIn("Bearer abc123", row["event_json"])
                self.assertIn("[REDACTED:token]", row["event_json"])
                self.assertNotIn(str(Path.home()), row["event_json"])
                self.assertIn("[REDACTED:home_path]", row["event_json"])
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                queue.close()
                store.close()

    def test_temporary_gateway_failure_requeues_claimed_event(self):
        from aetheris.cli import run_capture_and_upload
        from aetheris.core import ProcessObservation
        from aetheris.queue import LocalQueue

        class FailingGateway:
            def send(self, events):
                raise ConnectionError("temporary outage")

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"
            (project / ".git").mkdir(parents=True)
            queue = LocalQueue(root / "client.db")
            try:
                with self.assertRaises(ConnectionError):
                    run_capture_and_upload(
                        ProcessObservation(1, "Code.exe", None, None),
                        project,
                        [project],
                        queue,
                        FailingGateway(),
                    )
                time.sleep(1.1)
                self.assertEqual(len(queue.claim_batch(10)), 1)
            finally:
                queue.close()


if __name__ == "__main__":
    unittest.main()
