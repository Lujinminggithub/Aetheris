import json
import tempfile
import unittest
from pathlib import Path


class VSCodeBridgeProcessorTests(unittest.TestCase):
    def test_partial_batch_acknowledges_only_enqueued_events(self):
        from aetheris.queue import LocalQueue
        from aetheris.tray import ProjectSource, TrayConfig
        from aetheris.vscode_bridge import VSCodeBridgeProcessor

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw) / "repo"; root.mkdir()
            source = root / "app.py"; source.write_text("print('private')", encoding="utf-8")
            outside = Path(raw) / "private.py"; outside.write_text("secret", encoding="utf-8")
            queue = LocalQueue(Path(raw) / "queue.db")
            config = TrayConfig(
                gateway_url="http://server", token_file=root / "token", project_root=root,
                queue=Path(raw) / "queue.db", authorized_roots=[root], project_roots=[ProjectSource(root, "git")],
                tenant_id="tenant-1", subject_id="subject-1", device_id="device-1",
            )
            processor = VSCodeBridgeProcessor(queue, config)
            raw_frame = json.dumps({
                "version": 1, "type": "events", "session_id": "session-1", "events": [
                    {"event_id": "accepted-1", "event_type": "ide.file_opened", "occurred_at": "2026-09-14T08:00:00Z", "file_path": str(source), "language_id": "python"},
                    {"event_id": "rejected-1", "event_type": "ide.file_opened", "occurred_at": "2026-09-14T08:00:00Z", "file_path": str(outside), "language_id": "python"},
                ],
            }).encode("utf-8")

            try:
                ack = processor.process(raw_frame)
                self.assertEqual(ack["accepted_ids"], ["accepted-1"])
                self.assertEqual(ack["rejected"], [{"event_id": "rejected-1", "reason_code": "path_not_authorized"}])
                self.assertEqual(queue.stats()["queued_count"], 1)
                self.assertNotIn(str(root), json.dumps(queue.history()))
            finally:
                queue.close()

    def test_heartbeat_updates_safe_component_snapshot(self):
        from aetheris.vscode_bridge import VSCodeBridgeProcessor

        processor = VSCodeBridgeProcessor(None, None)
        ack = processor.process(json.dumps({
            "version": 1, "type": "heartbeat", "session_id": "session-1",
            "extension_id": "aetheris.aetheris-vscode", "extension_version": "0.1.0",
            "vscode_version": "1.96.0", "host_kind": "local", "component_state": "active",
            "pending_events": 2, "sent_events": 4, "dropped_events": 0,
        }).encode("utf-8"))

        self.assertEqual(ack, {"version": 1, "accepted_ids": [], "rejected": [], "control_state": "active", "clear_cache": False})
        snapshot = processor.snapshot()
        self.assertEqual(snapshot["component_state"], "active")
        self.assertEqual(snapshot["pending_events"], 2)
        self.assertNotIn("path", json.dumps(snapshot))

    def test_ack_carries_local_lens_pause_state(self):
        from aetheris.vscode_bridge import VSCodeBridgeProcessor

        processor = VSCodeBridgeProcessor(None, None)
        processor.set_desired_state("paused_by_user")
        ack = processor.process(json.dumps({
            "version": 1, "type": "heartbeat", "session_id": "session-1",
            "extension_id": "aetheris.aetheris-vscode", "extension_version": "0.1.0",
            "vscode_version": "1.96.0", "host_kind": "local", "component_state": "active",
            "pending_events": 1, "sent_events": 0, "dropped_events": 0,
        }).encode("utf-8"))
        self.assertEqual(ack["control_state"], "paused_by_user")

    def test_clear_cache_control_is_delivered_once(self):
        from aetheris.vscode_bridge import VSCodeBridgeProcessor

        processor = VSCodeBridgeProcessor(None, None)
        processor.request_clear_cache()
        heartbeat = json.dumps({
            "version": 1, "type": "heartbeat", "session_id": "session-1",
            "extension_id": "aetheris.aetheris-vscode", "extension_version": "0.1.0",
            "vscode_version": "1.96.0", "host_kind": "local", "component_state": "active",
            "pending_events": 1, "sent_events": 0, "dropped_events": 0,
        }).encode("utf-8")

        self.assertTrue(processor.process(heartbeat)["clear_cache"])
        self.assertFalse(processor.process(heartbeat)["clear_cache"])


if __name__ == "__main__":
    unittest.main()
