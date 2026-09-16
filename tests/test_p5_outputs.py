import unittest

from tests.helpers import make_sample_event


class P5OutputsTests(unittest.TestCase):
    def test_pulse_is_explainable_and_daily_log_keeps_event_ids(self):
        from aetheris.logs import DailyLog
        from aetheris.pulse import Pulse

        event = make_sample_event()
        pulse = Pulse().summarize([event])
        self.assertEqual(pulse["metrics"]["coding_events"], 1)
        self.assertIn(event.event_id, pulse["evidence_event_ids"])
        log = DailyLog().build([event])
        self.assertIn(event.event_id, log)

    def test_dataset_manifest_contains_hash_and_rights(self):
        from aetheris.dataset import DatasetExporter

        event = make_sample_event()
        result = DatasetExporter().export([event])
        self.assertIn("manifest", result)
        self.assertEqual(result["manifest"]["rights"]["dataset_export"], "required")
        self.assertEqual(len(result["manifest"]["event_ids"]), 1)
        self.assertEqual(len(result["jsonl"].splitlines()), 1)


if __name__ == "__main__":
    unittest.main()
