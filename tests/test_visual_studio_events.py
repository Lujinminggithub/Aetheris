import json
import tempfile
import unittest
from pathlib import Path


class VisualStudioEventAdapterTests(unittest.TestCase):
    def test_parses_debug_build_and_test_events(self):
        from aetheris.adapters.visual_studio_events import VisualStudioEventAdapter

        with tempfile.TemporaryDirectory() as raw:
            log = Path(raw) / "activity.jsonl"
            log.write_text("\n".join([
                json.dumps({"event": "debug", "action": "start", "solution": "C:/repo/Demo.sln"}),
                json.dumps({"event": "build", "configuration": "Debug", "success": True, "duration_ms": 1200}),
                json.dumps({"event": "test", "name": "Demo.Tests", "outcome": "passed", "duration_ms": 44}),
            ]) + "\n", encoding="utf-8")
            records = VisualStudioEventAdapter(log).collect()
            self.assertEqual([record["event_type"] for record in records], ["ide.debug", "ide.build", "ide.test"])
            self.assertEqual(records[2]["payload"]["outcome"], "passed")


if __name__ == "__main__":
    unittest.main()
