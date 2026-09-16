import json
import tempfile
import unittest
from pathlib import Path


class CoreStatusTests(unittest.TestCase):
    def test_project_identity_status_contains_version_but_never_key_material(self):
        from aetheris.status import CoreStatus

        with tempfile.TemporaryDirectory() as raw:
            status = CoreStatus(Path(raw) / "status.json", version="1.0")
            status.update_project_identity("available", key_version=4)

            snapshot = status.snapshot()
            self.assertEqual(snapshot["project_identity"], {"state": "available", "key_version": 4, "last_error": ""})
            serialized = json.dumps(snapshot).casefold()
            self.assertNotIn("remote_key", serialized)
            self.assertNotIn("root_key", serialized)
            self.assertNotIn("dgvuyw50", serialized)

    def test_project_sync_status_tracks_revision_and_count(self):
        from aetheris.status import CoreStatus

        with tempfile.TemporaryDirectory() as raw:
            status = CoreStatus(Path(raw) / "status.json", version="1.0")
            status.update_project_sync("registered", registry_revision=8, project_count=3)

            self.assertEqual(status.snapshot()["project_sync"], {
                "state": "registered", "registry_revision": 8, "project_count": 3, "last_error": "",
            })

    def test_capture_success_does_not_clear_registration_error(self):
        from aetheris.status import CoreStatus
        with tempfile.TemporaryDirectory() as raw:
            status = CoreStatus(Path(raw) / "status.json", version="0.4.0")
            status.update_registration("credential_invalid", error="HTTP 401")
            status.update_capture("running", project_count=3)
            data = json.loads((Path(raw) / "status.json").read_text())
            self.assertEqual(data["registration"]["state"], "credential_invalid")
            self.assertEqual(data["registration"]["last_error"], "HTTP 401")
            self.assertEqual(data["capture"]["state"], "running")

    def test_capture_status_exposes_duplicate_terminal_suppression_count(self):
        from aetheris.status import CoreStatus

        with tempfile.TemporaryDirectory() as raw:
            status = CoreStatus(Path(raw) / "status.json", version="0.1.0")
            status.update_capture("running", project_count=1, suppressed_duplicate_terminal=3)
            self.assertEqual(status.snapshot()["capture"]["suppressed_duplicate_terminal"], 3)

    def test_backoff_caps_at_five_minutes_and_resets(self):
        from aetheris.status import HeartbeatBackoff
        backoff = HeartbeatBackoff()
        values = [backoff.failure() for _ in range(10)]
        self.assertEqual(values[:4], [5, 10, 20, 40])
        self.assertEqual(values[-1], 300)
        self.assertEqual(backoff.success(), 60)
        self.assertEqual(backoff.failure(), 5)


if __name__ == "__main__": unittest.main()
