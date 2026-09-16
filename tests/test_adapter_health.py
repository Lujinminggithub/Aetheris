import json
import tempfile
import unittest
from pathlib import Path


class AdapterHealthTests(unittest.TestCase):
    def test_registry_exposes_ai_session_file_backfill_progress(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            registry = AdapterHealthRegistry(Path(raw) / "health.json")
            registry.finish(
                "codex", "active", discovered=368, parsed=500,
                total_files=368, covered_files=15, pending_files=353,
            )

            snapshot = registry.snapshot()[0]
            self.assertEqual(snapshot["total_files"], 368)
            self.assertEqual(snapshot["covered_files"], 15)
            self.assertEqual(snapshot["pending_files"], 353)

    def test_registry_distinguishes_idle_missing_source_and_stage_error(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            registry = AdapterHealthRegistry(Path(raw) / "health.json")
            registry.begin("claude_code", "source_discovery")
            registry.finish("claude_code", "idle", discovered=3, parsed=3, last_event_at="2026-09-09T09:00:00Z")
            registry.fail("vscode", "source_missing", "source_discovery", "bridge_file_missing")
            registry.fail("browser", "error", "url_read", "uia_unavailable", detail="C:\\Users\\Alice\\secret")

            snapshots = {item["adapter_id"]: item for item in registry.snapshot()}
            self.assertEqual(snapshots["claude_code"]["state"], "idle")
            self.assertEqual(snapshots["vscode"]["state"], "source_missing")
            self.assertEqual(snapshots["browser"]["error_stage"], "url_read")
            serialized = json.dumps(snapshots, ensure_ascii=False)
            self.assertNotIn("Alice", serialized)
            self.assertNotIn("secret", serialized)
            self.assertNotIn("detail", snapshots["browser"])

    def test_invalid_state_and_stage_are_rejected(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            registry = AdapterHealthRegistry(Path(raw) / "health.json")
            with self.assertRaises(ValueError):
                registry.fail("browser", "healthy", "capture", "bad")
            with self.assertRaises(ValueError):
                registry.fail("browser", "error", "unknown_stage", "bad")

    def test_snapshot_round_trip_is_bounded_and_safe(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "health.json"
            registry = AdapterHealthRegistry(path)
            registry.fail("browser", "error", "ocr", "ocr_failed")
            reloaded = AdapterHealthRegistry(path)
            self.assertEqual(reloaded.snapshot()[0]["error_code"], "ocr_failed")
            self.assertLess(path.stat().st_size, 64 * 1024)


if __name__ == "__main__":
    unittest.main()
