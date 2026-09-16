from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta


_BLOCKED_PROCESS_NAMES = {
    "chrome", "chrome.exe", "msedge", "msedge.exe",
    "aetheriscore.exe", "aetheriscoreservice.exe", "aetherissetup.exe",
    "system", "smss.exe", "csrss.exe", "winlogon.exe", "lsass.exe", "spoolsv.exe",
    "securityhealthservice.exe", "msmpeng.exe", "teams.exe", "slack.exe", "wechat.exe", "qq.exe",
}
_FAILURE_BACKOFF_SECONDS = (60, 120, 300, 600)


@dataclass(frozen=True)
class CaptureCandidate:
    identity_key: str
    process_name: str
    consent_state: str
    project_id: str
    foreground: bool
    visible: bool
    minimized: bool
    has_password_control: bool
    has_client_area: bool
    queue_watermark: str
    ocr_busy: bool
    now: datetime


@dataclass(frozen=True)
class CaptureDecision:
    eligible: bool
    reason_code: str


class ApplicationCapturePolicy:
    def __init__(self):
        self._native_events: dict[str, datetime] = {}
        self._eligible_since: dict[str, datetime] = {}
        self._process_success: dict[str, datetime] = {}
        self._last_device_success: datetime | None = None
        self._failure_count: dict[str, int] = {}
        self._failure_until: dict[str, datetime] = {}

    def mark_native_event(self, identity_key: str, at: datetime) -> None:
        self._native_events[identity_key] = at
        self._eligible_since[identity_key] = at

    def mark_result(self, identity_key: str, result: str, at: datetime) -> None:
        if result == "success":
            self._process_success[identity_key] = at
            self._last_device_success = at
            self._eligible_since[identity_key] = at
            self._failure_count.pop(identity_key, None)
            self._failure_until.pop(identity_key, None)
            return
        if result != "failure":
            raise ValueError("OCR 保底结果无效")
        count = self._failure_count.get(identity_key, 0)
        delay = _FAILURE_BACKOFF_SECONDS[min(count, len(_FAILURE_BACKOFF_SECONDS) - 1)]
        self._failure_count[identity_key] = count + 1
        self._failure_until[identity_key] = at + timedelta(seconds=delay)

    def evaluate(self, candidate: CaptureCandidate) -> CaptureDecision:
        if self._privacy_blocked(candidate):
            return CaptureDecision(False, "privacy_gate_blocked")
        if candidate.queue_watermark == "pause":
            return CaptureDecision(False, "queue_paused")
        if candidate.ocr_busy:
            return CaptureDecision(False, "ocr_busy")
        native_at = self._native_events.get(candidate.identity_key)
        if native_at is not None and (candidate.now - native_at).total_seconds() < 60:
            return CaptureDecision(False, "native_adapter_recent")
        failure_until = self._failure_until.get(candidate.identity_key)
        if failure_until is not None and candidate.now < failure_until:
            return CaptureDecision(False, "failure_backoff")
        if self._last_device_success is not None and (candidate.now - self._last_device_success).total_seconds() < 60:
            return CaptureDecision(False, "device_rate_limited")
        process_at = self._process_success.get(candidate.identity_key)
        if process_at is not None and (candidate.now - process_at).total_seconds() < 300:
            return CaptureDecision(False, "process_rate_limited")
        if native_at is None:
            first_seen = self._eligible_since.setdefault(candidate.identity_key, candidate.now)
            if (candidate.now - first_seen).total_seconds() < 60:
                return CaptureDecision(False, "native_adapter_recent")
        return CaptureDecision(True, "eligible")

    @staticmethod
    def _privacy_blocked(candidate: CaptureCandidate) -> bool:
        process_name = candidate.process_name.strip().casefold()
        return (
            candidate.consent_state not in {"allow_global", "allow_project"}
            or not candidate.identity_key
            or not candidate.project_id
            or process_name in _BLOCKED_PROCESS_NAMES
            or "credential" in process_name
            or "security" in process_name
            or not candidate.foreground
            or not candidate.visible
            or candidate.minimized
            or candidate.has_password_control
            or not candidate.has_client_area
        )
