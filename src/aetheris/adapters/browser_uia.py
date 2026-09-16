from __future__ import annotations

import ctypes
from ctypes import wintypes
import os
import time
from dataclasses import dataclass
from urllib.parse import urlparse


@dataclass
class FakeControl:
    children: list
    control_type: str = "WindowControl"
    value: str = ""

    @property
    def ControlTypeName(self):
        return self.control_type

    @property
    def Name(self):
        return self.value

    def GetChildren(self):
        return self.children

    def GetValuePattern(self):
        return type("ValuePattern", (), {"Value": self.value})()


class BrowserUrlReader:
    def read(self, root=None) -> str | None:
        if root is None:
            try:
                import uiautomation as auto

                root = auto.GetForegroundControl()
            except Exception:
                return None
        # Chromium address bars are nested below several provider/group nodes;
        # Edge currently places the editable URL at depth 8.
        for control in self._walk(root, depth=12):
            if getattr(control, "ControlTypeName", "") != "EditControl":
                continue
            value = ""
            try:
                value = control.GetValuePattern().Value
            except Exception:
                value = getattr(control, "Name", "")
            if isinstance(value, str) and urlparse(value).scheme in {"http", "https"} and urlparse(value).hostname:
                return value
        return None

    def _walk(self, control, depth: int):
        if depth < 0:
            return
        yield control
        try:
            children = control.GetChildren()
        except Exception:
            children = getattr(control, "children", [])
        for child in children or []:
            yield from self._walk(child, depth - 1)


class WindowsWindowCapture:
    def capture(self, control):
        from PIL import ImageGrab

        hwnd = getattr(control, "NativeWindowHandle", None) or getattr(control, "hwnd", None)
        if not hwnd or os.name != "nt":
            raise RuntimeError("window_handle_unavailable")
        rect = wintypes.RECT()
        if not ctypes.windll.user32.GetWindowRect(hwnd, ctypes.byref(rect)):
            raise RuntimeError("window_rect_unavailable")
        return ImageGrab.grab(bbox=(rect.left, rect.top, rect.right, rect.bottom), all_screens=True)

    def scroll(self, control):
        if os.name != "nt":
            raise RuntimeError("windows_only")
        import uiautomation as auto

        auto.SetCursorPos(0, 0) if False else None
        auto.SendKeys("{PAGEDOWN}")
        time.sleep(0.25)


class ScrollCapture:
    def __init__(self, url_reader, window_capture, allowlist: set[str], max_frames: int = 4):
        self.url_reader = url_reader
        self.window_capture = window_capture
        self.allowlist = {domain.casefold().lstrip(".") for domain in allowlist}
        self.max_frames = max(1, min(max_frames, 8))

    def collect(self, control) -> dict | None:
        url = self.url_reader.read(control)
        host = (urlparse(url).hostname or "").casefold() if url else ""
        if not url or not any(host == domain or host.endswith("." + domain) for domain in self.allowlist):
            return None
        frames = []
        for index in range(self.max_frames):
            frames.append(self.window_capture.capture(control))
            if index + 1 < self.max_frames:
                self.window_capture.scroll(control)
        return {"url": url, "frames": frames}
