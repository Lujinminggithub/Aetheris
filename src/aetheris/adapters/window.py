from __future__ import annotations

import ctypes
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import urlparse

from .browser import BrowserAllowlistAdapter
from ..processes import NativeProcessIdentityProvider


@dataclass(frozen=True)
class WindowObservation:
    pid: int
    app_name: str
    title: str
    url: str | None = None
    hwnd: int | None = None


def foreground_window() -> WindowObservation | None:
    if os.name != "nt":
        return None
    user32 = ctypes.windll.user32
    hwnd = user32.GetForegroundWindow()
    if not hwnd:
        return None
    title_buffer = ctypes.create_unicode_buffer(512)
    user32.GetWindowTextW(hwnd, title_buffer, len(title_buffer))
    pid = ctypes.c_ulong()
    user32.GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
    executable = NativeProcessIdentityProvider().process_path(pid.value) or ""
    app_name = Path(executable).name if executable else ""
    return WindowObservation(pid.value, app_name, title_buffer.value, None, int(hwnd))


def collect_visible_window(allowlist: set[str], ocr_capture=None, dlp=None, health=None) -> dict | None:
    observation = foreground_window()
    if observation is None:
        if health:
            health.finish("browser", "idle")
        return None
    if observation.app_name.casefold() not in {"chrome", "msedge", "chrome.exe", "msedge.exe"}:
        if health:
            health.finish("browser", "idle", detected_format=f"foreground:{observation.app_name.casefold() or 'unknown'}")
        return None
    if observation.url is None and ocr_capture is None:
        if health:
            health.fail("browser", "error", "url_read", "url_unavailable")
        return None
    if ocr_capture is None or dlp is None:
        record = BrowserAllowlistAdapter(allowlist).capture_page(observation.url, observation.title) if observation.url else None
        if record and health:
            hostname = str(record.get("payload", {}).get("domain", ""))
            health.finish("browser", "active", discovered=1, parsed=1, detected_format=f"foreground:{observation.app_name.casefold()}:{hostname}", last_event_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"))
        return record
    try:
        import uiautomation as auto

        observed_hwnd = getattr(observation, "hwnd", None)
        control = auto.ControlFromHandle(observed_hwnd) if observed_hwnd else auto.GetForegroundControl()
        if control is None:
            control = auto.GetForegroundControl()
        from .browser_uia import BrowserUrlReader, ScrollCapture, WindowsWindowCapture

        reader = BrowserUrlReader()
        url = reader.read(control)
        if not url:
            if health:
                health.fail("browser", "error", "url_read", "url_unavailable")
            return None
        hostname = (urlparse(url).hostname or "").casefold()
        normalized_allowlist = {domain.casefold().lstrip(".") for domain in allowlist}
        if not any(hostname == domain or hostname.endswith("." + domain) for domain in normalized_allowlist):
            if health:
                health.finish("browser", "idle", detected_format=f"foreground:{observation.app_name.casefold()}:not_allowlisted:{hostname}")
            return None
        result = ScrollCapture(reader, WindowsWindowCapture(), allowlist, max_frames=4).collect(control)
        if result is None:
            if health:
                health.fail("browser", "error", "url_read", "url_unavailable")
            return None
        record = BrowserAllowlistAdapter(allowlist).capture_frames(result["url"], result["frames"], ocr_capture, dlp)
        if record and health:
            health.finish("browser", "active", discovered=1, parsed=1, detected_format=f"foreground:{observation.app_name.casefold()}:{hostname}", last_event_at=datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"))
        return record
    except Exception:
        if health:
            health.fail("browser", "error", "capture", "window_capture_failed")
        return None
