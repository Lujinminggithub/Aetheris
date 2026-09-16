from __future__ import annotations

import ctypes
import json
import os
import struct
import time
from dataclasses import dataclass
from typing import Callable, Protocol


PIPE_NAME = r"\\.\pipe\Aetheris.Core.Service.v1"
MAX_FRAME = 16 * 1024


class ServiceUnavailable(RuntimeError):
    pass


class PipeTransport(Protocol):
    def wait(self, timeout_ms: int) -> bool: ...
    def send(self, frame: bytes) -> None: ...


class WindowsPipeTransport:
    def __init__(self, pipe_name: str = PIPE_NAME):
        self.pipe_name = pipe_name
        self.kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
        self.kernel32.CreateFileW.restype = ctypes.c_void_p
        self.kernel32.CreateFileW.argtypes = [
            ctypes.c_wchar_p, ctypes.c_uint32, ctypes.c_uint32, ctypes.c_void_p,
            ctypes.c_uint32, ctypes.c_uint32, ctypes.c_void_p,
        ]
        self.kernel32.WriteFile.argtypes = [
            ctypes.c_void_p, ctypes.c_void_p, ctypes.c_uint32,
            ctypes.POINTER(ctypes.c_uint32), ctypes.c_void_p,
        ]
        self.kernel32.CloseHandle.argtypes = [ctypes.c_void_p]

    def wait(self, timeout_ms: int) -> bool:
        return bool(self.kernel32.WaitNamedPipeW(self.pipe_name, timeout_ms))

    def send(self, frame: bytes) -> None:
        generic_write = 0x40000000
        open_existing = 3
        handle = self.kernel32.CreateFileW(self.pipe_name, generic_write, 0, None, open_existing, 0, None)
        if handle == ctypes.c_void_p(-1).value:
            raise ServiceUnavailable("service_pipe_unavailable")
        try:
            buffer = ctypes.create_string_buffer(frame)
            written = ctypes.c_uint32()
            if not self.kernel32.WriteFile(handle, buffer, len(frame), ctypes.byref(written), None) or written.value != len(frame):
                raise ServiceUnavailable("service_pipe_write_failed")
        finally:
            self.kernel32.CloseHandle(handle)


@dataclass
class ServiceClient:
    transport: PipeTransport
    pid: int
    session_id: int
    clock: Callable[[], float] = time.monotonic
    connected: bool = False
    last_message: str = ""

    @classmethod
    def for_current_process(cls, session_id: int | None = None) -> "ServiceClient":
        if os.name != "nt":
            raise ServiceUnavailable("service_requires_windows")
        resolved_session = session_id
        if resolved_session is None:
            value = ctypes.c_uint32()
            if not ctypes.windll.kernel32.ProcessIdToSessionId(os.getpid(), ctypes.byref(value)):
                raise ServiceUnavailable("service_session_unavailable")
            resolved_session = int(value.value)
        return cls(WindowsPipeTransport(), os.getpid(), resolved_session)

    def connect(self, timeout_seconds: float = 10) -> None:
        if not self.transport.wait(max(0, int(timeout_seconds * 1000))):
            raise ServiceUnavailable("service_pipe_timeout")
        self.connected = True

    def ready(self) -> None:
        self._send("ready")

    def heartbeat(self) -> None:
        self._send("heartbeat")

    def normal_exit(self) -> None:
        self._send("normal_exit")

    def resume(self) -> None:
        self._send("resume")

    def prepare_update(self) -> None:
        self._send("update_ready")

    def status_snapshot(self) -> dict:
        return {
            "state": "connected" if self.connected else "unavailable",
            "session_id": self.session_id,
            "last_message": self.last_message,
        }

    def close(self) -> None:
        self.connected = False

    def _send(self, message_type: str) -> None:
        allowed = {"ready", "heartbeat", "normal_exit", "resume", "status_request", "update_ready"}
        if message_type not in allowed:
            raise ValueError("unsupported service message")
        body = json.dumps(
            {
                "version": 1,
                "type": message_type,
                "pid": self.pid,
                "session_id": self.session_id,
                "monotonic_ms": int(self.clock() * 1000),
            },
            ensure_ascii=True,
            separators=(",", ":"),
        ).encode("utf-8")
        if not 0 < len(body) <= MAX_FRAME:
            raise ValueError("service message too large")
        self.transport.send(struct.pack("<I", len(body)) + body)
        self.connected = True
        self.last_message = message_type
