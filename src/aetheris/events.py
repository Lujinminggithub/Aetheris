from __future__ import annotations

import hashlib
import json
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any


def _utc_now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def _canonical(data: dict[str, Any]) -> str:
    return json.dumps(data, ensure_ascii=True, sort_keys=True, separators=(",", ":"))


@dataclass(frozen=True)
class AetherisEvent:
    event_id: str
    schema_version: int
    event_type: str
    tenant_id: str
    subject_id: str
    device_id: str
    project_id: str
    session_id: str
    correlation_id: str
    source: str
    source_version: str
    occurred_at: str
    ingested_at: str
    payload: dict[str, Any]
    content_refs: list[dict[str, Any]] = field(default_factory=list)
    content_hash: str = ""
    provenance: dict[str, Any] = field(default_factory=dict)
    redaction_report: dict[str, Any] = field(default_factory=dict)
    consent_policy_version: str = "1"
    processing_grants: list[str] = field(default_factory=list)
    crypto_mode: str = "device-redacted"
    key_version: str | None = None
    supersedes_event_id: str | None = None

    @classmethod
    def create(
        cls,
        event_type: str,
        *,
        tenant_id: str,
        subject_id: str,
        device_id: str,
        project_id: str,
        session_id: str,
        source: str,
        source_version: str,
        payload: dict[str, Any],
        redaction_report: dict[str, Any],
        processing_grants: list[str],
        correlation_id: str | None = None,
        occurred_at: str | None = None,
        ingested_at: str | None = None,
        content_refs: list[dict[str, Any]] | None = None,
        provenance: dict[str, Any] | None = None,
        consent_policy_version: str = "1",
        crypto_mode: str = "device-redacted",
        key_version: str | None = None,
        supersedes_event_id: str | None = None,
        event_id: str | None = None,
    ) -> "AetherisEvent":
        required = {
            "event_type": event_type,
            "tenant_id": tenant_id,
            "subject_id": subject_id,
            "device_id": device_id,
            "project_id": project_id,
            "session_id": session_id,
            "source": source,
            "source_version": source_version,
        }
        missing = [name for name, value in required.items() if not value]
        if missing:
            raise ValueError(f"missing required event fields: {', '.join(missing)}")
        event = cls(
            event_id=event_id or str(uuid.uuid4()),
            schema_version=1,
            event_type=event_type,
            tenant_id=tenant_id,
            subject_id=subject_id,
            device_id=device_id,
            project_id=project_id,
            session_id=session_id,
            correlation_id=correlation_id or str(uuid.uuid4()),
            source=source,
            source_version=source_version,
            occurred_at=occurred_at or _utc_now(),
            ingested_at=ingested_at or _utc_now(),
            payload=payload,
            content_refs=content_refs or [],
            provenance=provenance or {},
            redaction_report=redaction_report,
            consent_policy_version=consent_policy_version,
            processing_grants=processing_grants,
            crypto_mode=crypto_mode,
            key_version=key_version,
            supersedes_event_id=supersedes_event_id,
        )
        return cls(**{**event.__dict__, "content_hash": event._calculate_hash()})

    def _without_hash(self) -> dict[str, Any]:
        data = self.to_dict()
        data.pop("content_hash", None)
        return data

    def _calculate_hash(self) -> str:
        return hashlib.sha256(_canonical(self._without_hash()).encode("utf-8")).hexdigest()

    def to_dict(self) -> dict[str, Any]:
        return {
            "event_id": self.event_id,
            "schema_version": self.schema_version,
            "event_type": self.event_type,
            "tenant_id": self.tenant_id,
            "subject_id": self.subject_id,
            "device_id": self.device_id,
            "project_id": self.project_id,
            "session_id": self.session_id,
            "correlation_id": self.correlation_id,
            "source": self.source,
            "source_version": self.source_version,
            "occurred_at": self.occurred_at,
            "ingested_at": self.ingested_at,
            "payload": self.payload,
            "content_refs": self.content_refs,
            "content_hash": self.content_hash,
            "provenance": self.provenance,
            "redaction_report": self.redaction_report,
            "consent_policy_version": self.consent_policy_version,
            "processing_grants": self.processing_grants,
            "crypto_mode": self.crypto_mode,
            "key_version": self.key_version,
            "supersedes_event_id": self.supersedes_event_id,
        }

    def to_json(self) -> str:
        return _canonical(self.to_dict())

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "AetherisEvent":
        required = [
            "event_id", "schema_version", "event_type", "tenant_id", "subject_id",
            "device_id", "project_id", "session_id", "correlation_id", "source",
            "source_version", "occurred_at", "ingested_at", "payload", "content_hash",
            "redaction_report", "processing_grants",
        ]
        missing = [name for name in required if name not in data]
        if missing:
            raise ValueError(f"missing event fields: {', '.join(missing)}")
        if data["schema_version"] != 1 or not isinstance(data["schema_version"], int):
            raise ValueError("schema_version must be integer 1")
        if not isinstance(data["payload"], dict):
            raise ValueError("payload must be an object")
        if not isinstance(data["processing_grants"], list) or not all(isinstance(item, str) for item in data["processing_grants"]):
            raise ValueError("processing_grants must be a list of strings")
        for field_name in ("occurred_at", "ingested_at"):
            if not isinstance(data[field_name], str):
                raise ValueError(f"{field_name} must be an RFC3339 string")
            try:
                datetime.fromisoformat(data[field_name].replace("Z", "+00:00"))
            except ValueError as exc:
                raise ValueError(f"{field_name} must be an RFC3339 string") from exc
        event = cls(
            event_id=data["event_id"],
            schema_version=int(data["schema_version"]),
            event_type=data["event_type"],
            tenant_id=data["tenant_id"],
            subject_id=data["subject_id"],
            device_id=data["device_id"],
            project_id=data["project_id"],
            session_id=data["session_id"],
            correlation_id=data["correlation_id"],
            source=data["source"],
            source_version=data["source_version"],
            occurred_at=data["occurred_at"],
            ingested_at=data["ingested_at"],
            payload=data["payload"],
            content_refs=data.get("content_refs", []),
            content_hash=data["content_hash"],
            provenance=data.get("provenance", {}),
            redaction_report=data["redaction_report"],
            consent_policy_version=data.get("consent_policy_version", "1"),
            processing_grants=data["processing_grants"],
            crypto_mode=data.get("crypto_mode", "device-redacted"),
            key_version=data.get("key_version"),
            supersedes_event_id=data.get("supersedes_event_id"),
        )
        if event._calculate_hash() != event.content_hash:
            raise ValueError("event content_hash does not match canonical payload")
        return event
