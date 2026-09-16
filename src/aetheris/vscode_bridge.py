from __future__ import annotations

import ctypes
import json
import os
import struct
import threading
from ctypes import wintypes
from datetime import datetime, timezone
from typing import Callable

from .vscode_events import VSCodeEventRejected, project_vscode_event
from .vscode_protocol import BridgeProtocolError, MAX_FRAME_BYTES, parse_bridge_frame


PIPE_NAME = r"\\.\pipe\Aetheris.VSCode.Bridge.v1"


class VSCodeBridgeProcessor:
    def __init__(self, queue, config, *, snapshot_callback: Callable[[dict], None] | None = None):
        self.queue = queue
        self.config = config
        self.snapshot_callback = snapshot_callback
        self._snapshot = {
            "component_state": "awaiting_activation",
            "component_version": "",
            "protocol_version": 1,
            "vscode_version": "",
            "last_component_heartbeat_at": None,
            "pending_events": 0,
            "sent_events": 0,
            "dropped_events": 0,
            "accepted_events": 0,
            "rejected_events": 0,
        }
        self._desired_state = "active"
        self._clear_cache_requested = False

    def process(self, raw: bytes) -> dict:
        envelope = parse_bridge_frame(raw)
        if envelope.message_type in {"handshake", "heartbeat"}:
            metadata = envelope.metadata or {}
            state = str(metadata.get("component_state") or ("unsupported_remote_host" if metadata.get("host_kind") == "remote" else "active"))
            self._snapshot.update({
                "component_state": state,
                "component_version": str(metadata.get("extension_version", "")),
                "protocol_version": 1,
                "vscode_version": str(metadata.get("vscode_version", "")),
                "last_component_heartbeat_at": _now(),
                "pending_events": int(metadata.get("pending_events", 0)),
                "sent_events": int(metadata.get("sent_events", self._snapshot["sent_events"])),
                "dropped_events": int(metadata.get("dropped_events", self._snapshot["dropped_events"])),
            })
            self._publish_snapshot()
            return _ack([], [], self._desired_state, self._consume_clear_cache())
        accepted: list[str] = []
        rejected: list[dict] = []
        for value in envelope.events:
            event_id = str(value.get("event_id", ""))
            try:
                if self.queue is None or self.config is None:
                    raise VSCodeEventRejected("bridge_not_ready")
                event = project_vscode_event(value, self.config, session_id=envelope.session_id)
                self.queue.enqueue(event)
                accepted.append(event.event_id)
            except VSCodeEventRejected as exc:
                rejected.append({"event_id": event_id, "reason_code": str(exc)[:128]})
            except (OSError, RuntimeError, ValueError):
                rejected.append({"event_id": event_id, "reason_code": "queue_write_failed"})
        self._snapshot["accepted_events"] += len(accepted)
        self._snapshot["rejected_events"] += len(rejected)
        if accepted:
            self._snapshot["component_state"] = "active"
            self._snapshot["last_event_at"] = _now()
        self._publish_snapshot()
        return _ack(accepted, rejected, self._desired_state, self._consume_clear_cache())

    def set_desired_state(self, state: str) -> None:
        if state not in {"active", "paused_by_user"}:
            raise ValueError("vscode_control_state_invalid")
        self._desired_state = state

    def request_clear_cache(self) -> None:
        self._clear_cache_requested = True

    def _consume_clear_cache(self) -> bool:
        requested = self._clear_cache_requested
        self._clear_cache_requested = False
        return requested

    def snapshot(self) -> dict:
        return dict(self._snapshot)

    def _publish_snapshot(self) -> None:
        if self.snapshot_callback:
            self.snapshot_callback(self.snapshot())


class VSCodeBridgeServer:
    def __init__(self, processor: VSCodeBridgeProcessor, *, pipe_name: str = PIPE_NAME):
        self.processor = processor
        self.pipe_name = pipe_name
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None

    def start(self) -> None:
        if os.name != "nt":
            raise RuntimeError("vscode_bridge_requires_windows")
        if self._thread and self._thread.is_alive():
            return
        self._stop.clear()
        self._thread = threading.Thread(target=self._serve, name="aetheris-vscode-bridge", daemon=True)
        self._thread.start()

    def stop(self) -> None:
        self._stop.set()
        if os.name == "nt" and self._thread and self._thread.is_alive():
            _wake_pipe(self.pipe_name)
            self._thread.join(timeout=5)
        self._thread = None

    def snapshot(self) -> dict:
        return self.processor.snapshot()

    def _serve(self) -> None:
        descriptor = _security_descriptor()
        try:
            while not self._stop.is_set():
                pipe = _create_pipe(self.pipe_name, descriptor)
                if pipe is None:
                    return
                try:
                    kernel32 = ctypes.windll.kernel32
                    connected = kernel32.ConnectNamedPipe(pipe, None) or ctypes.get_last_error() == 535
                    if connected and not self._stop.is_set():
                        header = _read_exact(pipe, 4)
                        if header:
                            size = struct.unpack("<I", header)[0]
                            body = _read_exact(pipe, size) if 0 < size <= MAX_FRAME_BYTES else None
                            if body:
                                try:
                                    response = self.processor.process(body)
                                except BridgeProtocolError as exc:
                                    response = {"version": 1, "accepted_ids": [], "rejected": [{"event_id": "", "reason_code": str(exc)[:128]}]}
                                _write_frame(pipe, response)
                finally:
                    ctypes.windll.kernel32.FlushFileBuffers(pipe)
                    ctypes.windll.kernel32.DisconnectNamedPipe(pipe)
                    ctypes.windll.kernel32.CloseHandle(pipe)
        finally:
            ctypes.windll.kernel32.LocalFree(descriptor)


class _SECURITY_ATTRIBUTES(ctypes.Structure):
    _fields_ = [("nLength", wintypes.DWORD), ("lpSecurityDescriptor", wintypes.LPVOID), ("bInheritHandle", wintypes.BOOL)]


def _security_descriptor():
    sid = _current_user_sid()
    descriptor = wintypes.LPVOID()
    sddl = f"D:P(A;;GA;;;SY)(A;;GRGW;;;{sid})"
    if not ctypes.windll.advapi32.ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl, 1, ctypes.byref(descriptor), None):
        raise ctypes.WinError(ctypes.get_last_error())
    return descriptor


def _current_user_sid() -> str:
    token = wintypes.HANDLE()
    if not ctypes.windll.advapi32.OpenProcessToken(ctypes.windll.kernel32.GetCurrentProcess(), 0x0008, ctypes.byref(token)):
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        size = wintypes.DWORD()
        ctypes.windll.advapi32.GetTokenInformation(token, 1, None, 0, ctypes.byref(size))
        buffer = ctypes.create_string_buffer(size.value)
        if not ctypes.windll.advapi32.GetTokenInformation(token, 1, buffer, size, ctypes.byref(size)):
            raise ctypes.WinError(ctypes.get_last_error())
        sid_pointer = ctypes.cast(buffer, ctypes.POINTER(ctypes.c_void_p))[0]
        raw = wintypes.LPWSTR()
        if not ctypes.windll.advapi32.ConvertSidToStringSidW(sid_pointer, ctypes.byref(raw)):
            raise ctypes.WinError(ctypes.get_last_error())
        try:
            return raw.value
        finally:
            ctypes.windll.kernel32.LocalFree(raw)
    finally:
        ctypes.windll.kernel32.CloseHandle(token)


def _create_pipe(name: str, descriptor):
    kernel32 = ctypes.windll.kernel32
    kernel32.CreateNamedPipeW.restype = ctypes.c_void_p
    security = _SECURITY_ATTRIBUTES(ctypes.sizeof(_SECURITY_ATTRIBUTES), descriptor, False)
    handle = kernel32.CreateNamedPipeW(name, 0x00000003, 0x00000008, 1, MAX_FRAME_BYTES + 4, MAX_FRAME_BYTES + 4, 1000, ctypes.byref(security))
    return None if handle == ctypes.c_void_p(-1).value else handle


def _read_exact(pipe, size: int) -> bytes | None:
    if size < 0 or size > MAX_FRAME_BYTES:
        return None
    result = bytearray()
    while len(result) < size:
        buffer = ctypes.create_string_buffer(size - len(result))
        read = wintypes.DWORD()
        if not ctypes.windll.kernel32.ReadFile(pipe, buffer, len(buffer), ctypes.byref(read), None) or read.value == 0:
            return None
        result.extend(buffer.raw[:read.value])
    return bytes(result)


def _write_frame(pipe, value: dict) -> None:
    body = json.dumps(value, ensure_ascii=True, separators=(",", ":")).encode("utf-8")
    frame = struct.pack("<I", len(body)) + body
    buffer = ctypes.create_string_buffer(frame)
    written = wintypes.DWORD()
    ctypes.windll.kernel32.WriteFile(pipe, buffer, len(frame), ctypes.byref(written), None)


def _wake_pipe(name: str) -> None:
    kernel32 = ctypes.windll.kernel32
    kernel32.CreateFileW.restype = ctypes.c_void_p
    handle = kernel32.CreateFileW(name, 0xC0000000, 0, None, 3, 0, None)
    if handle != ctypes.c_void_p(-1).value:
        kernel32.CloseHandle(handle)


def _ack(accepted: list[str], rejected: list[dict], control_state: str = "active", clear_cache: bool = False) -> dict:
    return {
        "version": 1,
        "accepted_ids": accepted,
        "rejected": rejected,
        "control_state": control_state,
        "clear_cache": clear_cache,
    }


def _now() -> str:
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")
