from __future__ import annotations

from urllib.parse import urlparse

from ..redaction import Redactor
from ..dlp import DlpMatcher
from .ocr import OcrCapture, ScrollStitcher


class BrowserAllowlistAdapter:
    def __init__(self, allowlist: set[str]):
        self.allowlist = {domain.casefold().lstrip(".") for domain in allowlist}
        self.redactor = Redactor()

    def capture_page(self, url: str, visible_text: str) -> dict | None:
        parsed = urlparse(url)
        hostname = (parsed.hostname or "").casefold()
        if not hostname or not any(hostname == domain or hostname.endswith("." + domain) for domain in self.allowlist):
            return None
        safe_text, report = self.redactor.redact(visible_text[:20000])
        return {
            "event_type": "browser.page_view",
            "payload": {"domain": hostname, "path": parsed.path[:512], "visible_text": safe_text},
            "redaction_report": report,
        }

    def capture_frames(self, url: str, frames: list, ocr: OcrCapture, dlp: DlpMatcher) -> dict | None:
        parsed = urlparse(url)
        hostname = (parsed.hostname or "").casefold()
        if not hostname or not any(hostname == domain or hostname.endswith("." + domain) for domain in self.allowlist):
            return None
        stitched = ScrollStitcher.stitch(frames)
        result = ocr.extract(stitched)
        if result.get("state") != "captured":
            return {"event_type": "browser.page_view", "payload": {"domain": hostname, "ocr_state": result.get("state")}, "redaction_report": {"rules": []}}
        text = str(result.get("text", ""))
        safe_text, report = self.redactor.redact(text[:20000])
        dlp_result = dlp.evaluate(safe_text)
        if dlp_result["blocked"]:
            return {"event_type": "browser.page_view", "payload": {"domain": hostname, "ocr_state": "blocked", "dlp_matches": dlp_result["matches"]}, "redaction_report": report}
        return {"event_type": "browser.page_view", "payload": {"domain": hostname, "path": parsed.path[:512], "visible_text": safe_text, "ocr_state": "captured"}, "redaction_report": report}
