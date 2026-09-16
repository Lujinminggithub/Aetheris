import json
import tempfile
import unittest
from pathlib import Path


class VSCodeComponentStateTests(unittest.TestCase):
    def test_component_state_round_trips_without_paths(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "health.json"
            registry = AdapterHealthRegistry(path)
            registry.update_component("vscode_extension", {
                "component_state": "paused_by_user", "component_version": "0.1.0", "protocol_version": 1,
                "vscode_version": "1.96.0", "last_component_heartbeat_at": "2026-09-14T08:00:00Z",
                "pending_events": 2, "sent_events": 4, "dropped_events": 0,
            })

            snapshot = AdapterHealthRegistry(path).snapshot()[0]
            self.assertEqual(snapshot["state"], "disabled")
            self.assertEqual(snapshot["component_state"], "paused_by_user")
            self.assertEqual(snapshot["pending_events"], 2)
            self.assertNotIn("path", json.dumps(snapshot))

    def test_invalid_component_state_is_rejected(self):
        from aetheris.adapter_health import AdapterHealthRegistry

        with tempfile.TemporaryDirectory() as raw:
            registry = AdapterHealthRegistry(Path(raw) / "health.json")
            with self.assertRaisesRegex(ValueError, "组件状态无效"):
                registry.update_component("vscode_extension", {"component_state": "invented"})


if __name__ == "__main__":
    unittest.main()
