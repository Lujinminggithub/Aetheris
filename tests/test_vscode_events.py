import json
import tempfile
import unittest
from pathlib import Path


class VSCodeEventProjectionTests(unittest.TestCase):
    def _config(self, root: Path):
        from aetheris.tray import ProjectSource, TrayConfig

        return TrayConfig(
            gateway_url="http://server", token_file=root / "token", project_root=root,
            queue=root / "queue.db", authorized_roots=[root], project_roots=[ProjectSource(root, "git")],
            tenant_id="tenant-1", subject_id="subject-1", device_id="device-1",
        )

    def test_projects_authorized_file_and_removes_absolute_paths(self):
        from aetheris.vscode_events import project_vscode_event

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            source = root / "src" / "app.py"
            source.parent.mkdir()
            source.write_text("AETHERIS_FORBIDDEN_SOURCE_SENTINEL_7F31", encoding="utf-8")
            event = project_vscode_event({
                "event_id": "vscode-event-1", "event_type": "ide.file_opened",
                "occurred_at": "2026-09-14T08:00:00Z", "session_id": "session-1",
                "workspace_path": str(root), "file_path": str(source), "relative_path": "wrong/path.py",
                "file_name": "app.py", "extension": ".py", "language_id": "python", "uri_scheme": "file",
            }, self._config(root))

            serialized = event.to_json()
            self.assertEqual(event.payload["relative_path"], "src/app.py")
            self.assertEqual(event.payload["file_name"], "app.py")
            self.assertEqual(event.source, "core.vscode.extension")
            self.assertNotIn(str(root), serialized)
            self.assertNotIn("AETHERIS_FORBIDDEN_SOURCE_SENTINEL_7F31", serialized)

    def test_rejects_file_outside_authorized_roots(self):
        from aetheris.vscode_events import VSCodeEventRejected, project_vscode_event

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw) / "repo"; root.mkdir()
            outside = Path(raw) / "private" / "secret.py"; outside.parent.mkdir(); outside.write_text("secret", encoding="utf-8")
            with self.assertRaisesRegex(VSCodeEventRejected, "path_not_authorized"):
                project_vscode_event({
                    "event_id": "vscode-event-2", "event_type": "ide.file_edited",
                    "occurred_at": "2026-09-14T08:00:00Z", "file_path": str(outside),
                    "change_count": 1, "inserted_chars": 3, "deleted_chars": 0,
                }, self._config(root))

    def test_extension_change_uses_project_without_exposing_paths(self):
        from aetheris.vscode_events import project_vscode_event

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw).resolve()
            event = project_vscode_event({
                "event_id": "vscode-event-3", "event_type": "ide.extension_changed",
                "occurred_at": "2026-09-14T08:00:00Z", "extension_id": "vendor.tool",
                "version": "1.2.3", "change": "installed", "is_active": False,
            }, self._config(root), session_id="session-1")
            self.assertEqual(event.payload["extension_id"], "vendor.tool")
            self.assertNotIn(str(root), json.dumps(event.to_dict()))


if __name__ == "__main__":
    unittest.main()
