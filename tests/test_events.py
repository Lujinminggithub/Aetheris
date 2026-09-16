import json
import unittest


class EventTests(unittest.TestCase):
    def test_event_rejects_invalid_field_types_and_processing_grants(self):
        from aetheris.events import AetherisEvent

        event = AetherisEvent.create(
            "process.observed", tenant_id="local-default", subject_id="local-user",
            device_id="device-test", project_id="project-a", session_id="session-a",
            source="core.process", source_version="0.1.0", payload={"name": "code.exe"},
            redaction_report={"rules": []}, processing_grants=["server_ingest"],
        )
        invalid = event.to_dict()
        invalid["processing_grants"] = "server_ingest"
        with self.assertRaises(ValueError):
            AetherisEvent.from_dict(invalid)
        invalid = event.to_dict()
        invalid["payload"] = []
        with self.assertRaises(ValueError):
            AetherisEvent.from_dict(invalid)
    def test_event_serialization_is_deterministic_and_contains_redaction_metadata(self):
        from aetheris.events import AetherisEvent

        event = AetherisEvent.create(
            "process.observed",
            tenant_id="local-default",
            subject_id="local-user",
            device_id="device-test",
            project_id="project-a",
            session_id="session-a",
            source="core.process",
            source_version="0.1.0",
            payload={"name": "code.exe"},
            redaction_report={"rules": []},
            processing_grants=["server_ingest"],
        )
        data = json.loads(event.to_json())
        self.assertEqual(data["schema_version"], 1)
        self.assertEqual(data["content_hash"], event.content_hash)
        self.assertEqual(data["redaction_report"], {"rules": []})
        self.assertEqual(AetherisEvent.from_dict(data).to_json(), event.to_json())


if __name__ == "__main__":
    unittest.main()
