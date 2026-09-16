import json
import tempfile
import unittest
from pathlib import Path

from tests.helpers import make_sample_event


class RedactionAndQueueTests(unittest.TestCase):
    def test_redaction_removes_secret_before_event_hashing(self):
        from aetheris.redaction import Redactor

        value, report = Redactor().redact({"token": "Bearer abc123", "note": "hello"})
        self.assertEqual(value["token"], "[REDACTED:token]")
        self.assertEqual(report["replacement_count"], 1)
        self.assertNotIn("abc123", json.dumps(value))

    def test_redaction_removes_jsonb_incompatible_control_characters(self):
        from aetheris.redaction import Redactor

        value, report = Redactor().redact("before\x00\x01after\nkept")

        self.assertEqual(value, "before[REDACTED:control]after\nkept")
        self.assertIn({"rule_id": "control_character", "count": 1}, report["rules"])

    def test_queue_round_trip_and_ack(self):
        from aetheris.queue import LocalQueue

        with tempfile.TemporaryDirectory() as raw:
            queue = LocalQueue(Path(raw) / "queue.db", max_bytes=1024 * 1024)
            sample_event = make_sample_event()
            queue.enqueue(sample_event)
            claimed = queue.claim_batch(10)
            self.assertEqual([item.event_id for item in claimed], [sample_event.event_id])
            queue.ack([sample_event.event_id])
            self.assertEqual(queue.claim_batch(10), [])
            queue.close()

    def test_release_requeues_temporary_failure_and_reject_keeps_local_record(self):
        from aetheris.queue import LocalQueue

        with tempfile.TemporaryDirectory() as raw:
            queue = LocalQueue(Path(raw) / "queue.db", max_bytes=1024 * 1024)
            sample_event = make_sample_event()
            queue.enqueue(sample_event)
            queue.claim_batch(10)
            queue.release([sample_event.event_id])
            self.assertEqual([item.event_id for item in queue.claim_batch(10)], [sample_event.event_id])
            queue.reject([sample_event.event_id], "schema rejected")
            self.assertEqual(queue.claim_batch(10), [])
            self.assertEqual(queue.rejected_count(), 1)
            queue.close()

    def test_release_delay_prevents_immediate_reclaim(self):
        from aetheris.queue import LocalQueue

        with tempfile.TemporaryDirectory() as raw:
            queue = LocalQueue(Path(raw) / "queue.db", max_bytes=1024 * 1024)
            event = make_sample_event()
            queue.enqueue(event)
            queue.claim_batch(1)
            queue.release([event.event_id], delay_seconds=60)
            self.assertEqual(queue.claim_batch(1), [])
            queue.close()


if __name__ == "__main__":
    unittest.main()
