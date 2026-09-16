import tempfile
import unittest
from pathlib import Path

from tests.helpers import make_sample_event


class ForgeTests(unittest.TestCase):
    def test_groups_events_into_explainable_work_episode(self):
        from aetheris.forge import Forge
        from aetheris.events import AetherisEvent

        first = make_sample_event()
        second = AetherisEvent.create(
            "terminal.command", tenant_id=first.tenant_id, subject_id=first.subject_id,
            device_id=first.device_id, project_id=first.project_id, session_id=first.session_id,
            source="core.terminal", source_version="0.1.0", payload={"command": "git status"},
            redaction_report={"rules": [], "replacement_count": 0}, processing_grants=["server_ingest"],
            occurred_at="2026-09-06T00:05:00Z", ingested_at="2026-09-06T00:05:00Z",
        )
        episodes = Forge().build([first, second])
        self.assertEqual(len(episodes), 1)
        self.assertEqual(episodes[0]["event_ids"], [first.event_id, second.event_id])
        self.assertEqual(episodes[0]["evidence"][1]["event_id"], second.event_id)


if __name__ == "__main__":
    unittest.main()
