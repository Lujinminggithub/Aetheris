from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime

from ..command_privacy import CommandPrivacy
from ..dlp import DlpMatcher
from ..redaction import Redactor
from .application_window import ApplicationWindow


MAX_OCR_TEXT_LENGTH = 20_000


@dataclass(frozen=True)
class ApplicationCaptureCandidate:
    identity_key: str
    project_id: str
    now: datetime
    window: ApplicationWindow


@dataclass(frozen=True)
class ApplicationCaptureResult:
    record: dict | None
    reason_code: str
    health: dict


class ApplicationOcrAdapter:
    def __init__(self, capture, ocr, dlp: DlpMatcher, privacy: CommandPrivacy):
        self.capture = capture
        self.ocr = ocr
        self.dlp = dlp
        self.privacy = privacy
        self.redactor = Redactor()
        self._seen: dict[str, int] = {}

    def collect(self, candidate: ApplicationCaptureCandidate) -> ApplicationCaptureResult:
        image = None
        try:
            image = self.capture.capture(candidate.window)
            ocr_result = self.ocr.extract(image)
            if not isinstance(ocr_result, dict) or ocr_result.get("state") != "captured":
                return self._failure("ocr_failed")
            raw_text = str(ocr_result.get("text", ""))[:MAX_OCR_TEXT_LENGTH]
            if not raw_text.strip():
                return self._failure("ocr_failed")
            dlp_result = self.dlp.evaluate(raw_text)
            if dlp_result.get("blocked"):
                return ApplicationCaptureResult(None, "dlp_blocked", {
                    "reason_code": "dlp_blocked",
                    "dlp_match_count": len(dlp_result.get("matches", [])),
                })
            safe, report = self.redactor.redact({
                "window_context": candidate.window.title[:160],
                "visible_text": raw_text,
            })
            bucket = int(candidate.now.timestamp()) // 300
            fingerprint = self.privacy.fingerprint("\0".join((
                candidate.identity_key,
                candidate.project_id,
                str(safe["window_context"]),
                str(safe["visible_text"]),
                str(bucket),
            )))
            self._seen = {key: value for key, value in self._seen.items() if value >= bucket - 1}
            if fingerprint in self._seen:
                return self._failure("duplicate_window_content")
            self._seen[fingerprint] = bucket
            record = {
                "event_type": "application.activity",
                "payload": {
                    "application_name": candidate.window.process_name[:128],
                    "window_context": safe["window_context"],
                    "visible_text": safe["visible_text"],
                    "capture_method": "ocr_fallback",
                    "ocr_languages": ["eng", "chi_sim"],
                    "confidence": "low",
                },
                "redaction_report": report,
            }
            return ApplicationCaptureResult(record, "captured", {"reason_code": "captured", "parsed": 1})
        except TimeoutError:
            return self._failure("ocr_timeout")
        except (OSError, RuntimeError, TypeError, ValueError):
            return self._failure("ocr_failed")
        finally:
            close = getattr(image, "close", None)
            if close:
                close()

    @staticmethod
    def _failure(reason_code: str) -> ApplicationCaptureResult:
        return ApplicationCaptureResult(None, reason_code, {"reason_code": reason_code})
