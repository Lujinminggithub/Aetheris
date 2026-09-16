import json
import unittest


class VSCodeProtocolTests(unittest.TestCase):
    def test_parses_valid_behavior_event_batch(self):
        from aetheris.vscode_protocol import parse_bridge_frame

        envelope = parse_bridge_frame(json.dumps({
            "version": 1,
            "type": "events",
            "session_id": "session-1",
            "events": [{
                "event_id": "vscode-event-1",
                "event_type": "ide.file_edited",
                "occurred_at": "2026-09-14T01:00:00Z",
                "workspace_path": r"D:\repo",
                "file_path": r"D:\repo\src\app.py",
                "language_id": "python",
                "change_count": 3,
                "inserted_chars": 12,
                "deleted_chars": 4,
                "flush_reason": "save",
            }],
        }).encode("utf-8"))

        self.assertEqual(envelope.message_type, "events")
        self.assertEqual(envelope.session_id, "session-1")
        self.assertEqual(envelope.events[0]["event_id"], "vscode-event-1")

    def test_rejects_source_text_anywhere_in_frame(self):
        from aetheris.vscode_protocol import BridgeProtocolError, parse_bridge_frame

        raw = json.dumps({
            "version": 1,
            "type": "events",
            "session_id": "session-1",
            "events": [{
                "event_id": "vscode-event-1",
                "event_type": "ide.file_edited",
                "occurred_at": "2026-09-14T01:00:00Z",
                "file_path": r"D:\repo\app.py",
                "source_text": "password=secret",
            }],
        }).encode("utf-8")

        with self.assertRaisesRegex(BridgeProtocolError, "forbidden_field"):
            parse_bridge_frame(raw)

    def test_rejects_oversized_frame_and_batch(self):
        from aetheris.vscode_protocol import BridgeProtocolError, parse_bridge_frame

        with self.assertRaisesRegex(BridgeProtocolError, "frame_size_invalid"):
            parse_bridge_frame(b"x" * (64 * 1024 + 1))
        raw = json.dumps({
            "version": 1,
            "type": "events",
            "session_id": "session-1",
            "events": [{
                "event_id": f"event-{index}",
                "event_type": "ide.file_opened",
                "occurred_at": "2026-09-14T01:00:00Z",
                "file_path": r"D:\repo\app.py",
            } for index in range(101)],
        }).encode("utf-8")
        with self.assertRaisesRegex(BridgeProtocolError, "batch_size_invalid"):
            parse_bridge_frame(raw)

    def test_rejects_unknown_version_type_and_event(self):
        from aetheris.vscode_protocol import BridgeProtocolError, parse_bridge_frame

        for value, reason in (
            ({"version": 2, "type": "heartbeat", "session_id": "s"}, "version_unsupported"),
            ({"version": 1, "type": "unknown", "session_id": "s"}, "message_type_invalid"),
            ({"version": 1, "type": "events", "session_id": "s", "events": [{"event_id": "e", "event_type": "ide.unknown", "occurred_at": "2026-09-14T01:00:00Z"}]}, "event_type_invalid"),
        ):
            with self.subTest(reason=reason), self.assertRaisesRegex(BridgeProtocolError, reason):
                parse_bridge_frame(json.dumps(value).encode("utf-8"))


if __name__ == "__main__":
    unittest.main()
