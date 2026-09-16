from aetheris.events import AetherisEvent


def make_sample_event() -> AetherisEvent:
    return AetherisEvent.create(
        "process.observed",
        tenant_id="local-default",
        subject_id="local-user",
        device_id="device-test",
        project_id="project-test",
        session_id="session-test",
        source="core.process",
        source_version="0.1.0",
        payload={"name": "code.exe"},
        redaction_report={"rules": [], "replacement_count": 0},
        processing_grants=["server_ingest"],
        event_id="event-test-1",
        correlation_id="correlation-test-1",
        occurred_at="2026-09-06T00:00:00Z",
        ingested_at="2026-09-06T00:00:00Z",
    )

