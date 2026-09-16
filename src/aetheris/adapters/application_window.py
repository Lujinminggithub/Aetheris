from __future__ import annotations

import ctypes
import os
from ctypes import wintypes
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

from ..processes import NativeProcessIdentityProvider


MAX_CLIENT_PIXELS = 16_000_000
_CREDENTIAL_CLASSES = {"credential dialog xaml host", "credentialdialogxamlhost", "credprovhost"}


@dataclass(frozen=True)
class ApplicationWindow:
    hwnd: int
    pid: int
    process_name: str
    title: str
    visible: bool
    minimized: bool
    has_password_control: bool
    client_rect: tuple[int, int, int, int]


class ApplicationWindowInspector:
    def __init__(self, window_api=None, identity_provider=None, control_provider: Callable[[int], object | None] | None = None):
        self.window_api = window_api or NativeWindowAPI()
        self.identity_provider = identity_provider or NativeProcessIdentityProvider()
        self.control_provider = control_provider or _control_from_handle

    def inspect(self) -> ApplicationWindow | None:
        hwnd = int(self.window_api.foreground_handle() or 0)
        if not hwnd:
            return None
        pid = int(self.window_api.process_id(hwnd) or 0)
        if not pid:
            return None
        executable = self.identity_provider.process_path(pid) or ""
        process_name = Path(executable).name.casefold() if executable else ""
        title = str(self.window_api.title(hwnd) or "")[:512]
        class_name = str(getattr(self.window_api, "class_name", lambda _hwnd: "")(hwnd) or "").casefold()
        control = self.control_provider(hwnd)
        password = class_name in _CREDENTIAL_CLASSES or "credential" in class_name or _contains_password_control(control)
        return ApplicationWindow(
            hwnd=hwnd,
            pid=pid,
            process_name=process_name,
            title=title,
            visible=bool(self.window_api.is_visible(hwnd)),
            minimized=bool(self.window_api.is_minimized(hwnd)),
            has_password_control=password,
            client_rect=self.window_api.client_rect(hwnd),
        )


class ClientAreaCapture:
    def __init__(self, grabber=None):
        self.grabber = grabber or _image_grab

    def capture(self, window: ApplicationWindow):
        left, top, right, bottom = window.client_rect
        width, height = right - left, bottom - top
        if width <= 0 or height <= 0:
            raise ValueError("window_client_area_invalid")
        if width * height > MAX_CLIENT_PIXELS:
            raise ValueError("window_client_area_too_large")
        return self.grabber(bbox=window.client_rect, all_screens=True)


class _POINT(ctypes.Structure):
    _fields_ = [("x", wintypes.LONG), ("y", wintypes.LONG)]


class NativeWindowAPI:
    def _user32(self):
        if os.name != "nt":
            raise RuntimeError("windows_only")
        return ctypes.windll.user32

    def foreground_handle(self) -> int:
        return int(self._user32().GetForegroundWindow())

    def process_id(self, hwnd: int) -> int:
        pid = wintypes.DWORD()
        self._user32().GetWindowThreadProcessId(hwnd, ctypes.byref(pid))
        return int(pid.value)

    def is_visible(self, hwnd: int) -> bool:
        return bool(self._user32().IsWindowVisible(hwnd))

    def is_minimized(self, hwnd: int) -> bool:
        return bool(self._user32().IsIconic(hwnd))

    def title(self, hwnd: int) -> str:
        buffer = ctypes.create_unicode_buffer(512)
        self._user32().GetWindowTextW(hwnd, buffer, len(buffer))
        return buffer.value

    def class_name(self, hwnd: int) -> str:
        buffer = ctypes.create_unicode_buffer(256)
        self._user32().GetClassNameW(hwnd, buffer, len(buffer))
        return buffer.value

    def client_rect(self, hwnd: int) -> tuple[int, int, int, int]:
        rect = wintypes.RECT()
        if not self._user32().GetClientRect(hwnd, ctypes.byref(rect)):
            return (0, 0, 0, 0)
        origin = _POINT(rect.left, rect.top)
        end = _POINT(rect.right, rect.bottom)
        if not self._user32().ClientToScreen(hwnd, ctypes.byref(origin)) or not self._user32().ClientToScreen(hwnd, ctypes.byref(end)):
            return (0, 0, 0, 0)
        return (origin.x, origin.y, end.x, end.y)


def _control_from_handle(hwnd: int):
    try:
        import uiautomation as auto

        return auto.ControlFromHandle(hwnd)
    except Exception:
        return None


def _contains_password_control(control, depth: int = 8) -> bool:
    if control is None or depth < 0:
        return False
    try:
        if bool(getattr(control, "IsPassword", False)):
            return True
    except Exception:
        return True
    try:
        children = control.GetChildren()
    except Exception:
        children = []
    return any(_contains_password_control(child, depth - 1) for child in children or [])


def _image_grab(*, bbox, all_screens):
    from PIL import ImageGrab

    return ImageGrab.grab(bbox=bbox, all_screens=all_screens)
