from __future__ import annotations

import argparse
from pathlib import Path

from .core import ConsentRegistry, ProcessObservation, capture_observation
from .events import AetherisEvent
from .gateway import GatewayClient, _read_token_file
from .queue import LocalQueue
from .redaction import Redactor


def _redacted_event(event: AetherisEvent) -> AetherisEvent:
    safe_fields, report = Redactor().redact(
        {
            "payload": event.payload,
            "provenance": event.provenance,
            "content_refs": event.content_refs,
        }
    )
    return AetherisEvent.create(
        event.event_type,
        tenant_id=event.tenant_id,
        subject_id=event.subject_id,
        device_id=event.device_id,
        project_id=event.project_id,
        session_id=event.session_id,
        source=event.source,
        source_version=event.source_version,
        payload=safe_fields["payload"],
        redaction_report=report,
        processing_grants=event.processing_grants,
        correlation_id=event.correlation_id,
        occurred_at=event.occurred_at,
        ingested_at=event.ingested_at,
        content_refs=safe_fields["content_refs"],
        provenance=safe_fields["provenance"],
        consent_policy_version=event.consent_policy_version,
        crypto_mode=event.crypto_mode,
        key_version=event.key_version,
        supersedes_event_id=event.supersedes_event_id,
        event_id=event.event_id,
    )


def run_capture_and_upload(
    observation: ProcessObservation,
    project_path: str | Path,
    authorized_roots: list[str | Path],
    queue: LocalQueue,
    gateway_client: GatewayClient,
    raw_payload: dict | None = None,
    device_id: str = "device-local",
    tenant_id: str = "local-default",
    subject_id: str = "local-user",
) -> dict[str, int]:
    event = capture_observation(
        observation,
        project_path=project_path,
        consent_registry=ConsentRegistry(allowed_names={"code.exe", "devenv.exe", "powershell.exe"}),
        authorized_roots=authorized_roots,
        payload=raw_payload or {"pid": observation.pid, "name": observation.name.casefold()},
        device_id=device_id,
        tenant_id=tenant_id,
        subject_id=subject_id,
    )
    if event is None:
        return {"captured": 0, "accepted": 0, "duplicate": 0, "rejected": 0}
    safe_event = _redacted_event(event)
    queue.enqueue(safe_event)
    claimed = queue.claim_batch(100)
    try:
        results = gateway_client.send(claimed)
    except Exception:
        queue.release_with_backoff([item.event_id for item in claimed])
        raise
    accepted = sum(result["status"] == "accepted" for result in results)
    duplicate = sum(result["status"] == "duplicate" for result in results)
    rejected = sum(result["status"] == "rejected" for result in results)
    queue.ack([item.event_id for item, result in zip(claimed, results) if result["status"] in {"accepted", "duplicate"}])
    for item, result in zip(claimed, results):
        if result["status"] == "rejected":
            queue.reject([item.event_id], result.get("reason", "server rejected event"))
    return {"captured": 1, "accepted": accepted, "duplicate": duplicate, "rejected": rejected}


def main() -> None:
    parser = argparse.ArgumentParser(description="Run a safe Aetheris Core demo capture")
    subparsers = parser.add_subparsers(dest="command", required=True)
    demo = subparsers.add_parser("demo")
    demo.add_argument("--server-url", required=True)
    demo.add_argument("--token")
    demo.add_argument("--token-file")
    demo.add_argument("--queue", default="aetheris-client.db")
    demo.add_argument("--project", type=Path, default=Path.cwd())
    args = parser.parse_args()
    if args.command == "demo":
        token = args.token or (_read_token_file(args.token_file) if args.token_file else "")
        if not token:
            parser.error("a non-empty --token or --token-file is required")
        queue = LocalQueue(args.queue)
        try:
            result = run_capture_and_upload(
                ProcessObservation(1, "Code.exe", None, None),
                args.project,
                [args.project],
                queue,
                GatewayClient(args.server_url, token),
            )
            print(result)
        finally:
            queue.close()


if __name__ == "__main__":
    main()
