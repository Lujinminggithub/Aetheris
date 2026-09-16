import json
import tempfile
import unittest
from pathlib import Path

from tests.helpers import make_sample_event


class SchemaAndQueueStatsTests(unittest.TestCase):
    def test_event_schema_declares_required_envelope_and_types(self):
        schema = json.loads(Path("schemas/aetheris-event-v1.schema.json").read_text(encoding="utf-8"))
        required = set(schema["required"])
        self.assertTrue({"event_id", "schema_version", "event_type", "payload", "content_hash", "processing_grants"}.issubset(required))
        self.assertEqual(schema["properties"]["schema_version"]["type"], "integer")

    def test_queue_reports_bytes_counts_and_watermarks(self):
        from aetheris.queue import LocalQueue

        with tempfile.TemporaryDirectory() as raw:
            queue = LocalQueue(Path(raw) / "client.db", max_bytes=100000)
            queue.enqueue(make_sample_event())
            stats = queue.stats()
            self.assertEqual(stats["queued_count"], 1)
            self.assertGreater(stats["bytes_used"], 0)
            self.assertEqual(stats["watermark"], "normal")
            queue.close()


if __name__ == "__main__":
    unittest.main()
