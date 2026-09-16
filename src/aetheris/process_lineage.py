from __future__ import annotations

import ctypes
import os
from ctypes import wintypes
from dataclasses import dataclass


AI_PROCESSES = {"codex.exe", "claude.exe", "cursor.exe"}


@dataclass(frozen=True)
class ProcessLineage:
    pid: int
    parent_pid: int
    ancestor_processes: tuple[str, ...]
    origin: str

    def to_payload(self) -> dict:
        return {
            "parent_pid": self.parent_pid,
            "ancestor_processes": list(self.ancestor_processes),
            "origin": self.origin,
        }


def classify_shell_origin(process_name: str, ancestors: list[str] | tuple[str, ...]) -> str:
    normalized = {name.casefold() for name in ancestors[1:]}
    return "ai" if normalized.intersection(AI_PROCESSES) else "unknown"


class NativeProcessLineageProvider:
    """Read a bounded Windows process ancestry snapshot without shell commands."""

    def inspect(self, pid: int, name: str, *, max_depth: int = 8) -> ProcessLineage:
        if os.name != "nt":
            return ProcessLineage(pid, 0, (name.casefold(),), "unknown")
        try:
            entries = _process_snapshot()
        except (AttributeError, OSError, ValueError):
            entries = {}
        return build_lineage(pid, name, entries, max_depth=max_depth)


def build_lineage(pid: int, name: str, entries: dict[int, tuple[int, str]], *, max_depth: int = 8) -> ProcessLineage:
    ancestors = [name.casefold()]
    current = int(pid)
    parent_pid = entries.get(current, (0, ""))[0]
    seen = {current}
    for _ in range(max(0, min(max_depth, 16))):
        parent = entries.get(current, (0, ""))[0]
        if parent <= 0 or parent in seen:
            break
        seen.add(parent)
        parent_name = entries.get(parent, (0, ""))[1]
        if parent_name:
            ancestors.append(parent_name.casefold())
        current = parent
    return ProcessLineage(pid, parent_pid, tuple(ancestors), classify_shell_origin(name, ancestors))


class _PROCESSENTRY32W(ctypes.Structure):
    _fields_ = [
        ("dwSize", wintypes.DWORD), ("cntUsage", wintypes.DWORD), ("th32ProcessID", wintypes.DWORD),
        ("th32DefaultHeapID", ctypes.POINTER(ctypes.c_ulong)), ("th32ModuleID", wintypes.DWORD),
        ("cntThreads", wintypes.DWORD), ("th32ParentProcessID", wintypes.DWORD),
        ("pcPriClassBase", ctypes.c_long), ("dwFlags", wintypes.DWORD), ("szExeFile", wintypes.WCHAR * 260),
    ]


def _process_snapshot() -> dict[int, tuple[int, str]]:
    kernel32 = ctypes.windll.kernel32
    snapshot = kernel32.CreateToolhelp32Snapshot(0x00000002, 0)
    if snapshot in {0, ctypes.c_void_p(-1).value}:
        return {}
    entries: dict[int, tuple[int, str]] = {}
    try:
        value = _PROCESSENTRY32W()
        value.dwSize = ctypes.sizeof(_PROCESSENTRY32W)
        if not kernel32.Process32FirstW(snapshot, ctypes.byref(value)):
            return entries
        while True:
            entries[int(value.th32ProcessID)] = (int(value.th32ParentProcessID), value.szExeFile)
            if not kernel32.Process32NextW(snapshot, ctypes.byref(value)):
                break
        return entries
    finally:
        kernel32.CloseHandle(snapshot)
