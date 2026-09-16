from __future__ import annotations

import ctypes
import hashlib
import os
from ctypes import wintypes
from pathlib import Path

from .process_consent import StableProcessIdentity


class NativeProcessIdentityProvider:
    """读取当前用户可访问的最小进程身份，不调用命令行解释器。"""

    def inspect(self, pid: int, name: str) -> StableProcessIdentity:
        executable = self.process_path(pid) or ""
        signature_status = self.signature_status(executable) if executable else "unknown"
        publisher = self.publisher(executable) if executable and signature_status == "valid" else ""
        binary_hash = self.binary_hash(executable) if executable else ""
        return StableProcessIdentity(name=name, executable=executable, publisher=publisher, binary_hash=binary_hash, signature_status=signature_status)

    def signature_status(self, executable: str) -> str:
        if os.name != "nt" or not executable:
            return "unknown"
        try:
            result = _win_verify_trust(executable)
        except (AttributeError, OSError, ValueError):
            return "unknown"
        return "valid" if result == 0 else "invalid"

    def process_path(self, pid: int) -> str | None:
        if os.name != "nt":
            return None
        PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
        handle = ctypes.windll.kernel32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, int(pid))
        if not handle:
            return None
        try:
            size = wintypes.DWORD(32768)
            buffer = ctypes.create_unicode_buffer(size.value)
            query = ctypes.windll.kernel32.QueryFullProcessImageNameW
            query.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
            if not query(handle, 0, buffer, ctypes.byref(size)):
                return None
            return str(Path(buffer.value[: size.value]).resolve())
        finally:
            ctypes.windll.kernel32.CloseHandle(handle)

    def binary_hash(self, executable: str) -> str:
        try:
            digest = hashlib.sha256()
            with open(executable, "rb") as handle:
                for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                    digest.update(chunk)
            return "sha256:" + digest.hexdigest()
        except OSError:
            return ""

    def publisher(self, executable: str) -> str:
        if os.name != "nt" or not executable:
            return ""
        # version.dll is a native, read-only source for CompanyName; signature
        # verification remains represented by an empty publisher when absent.
        version = ctypes.windll.version
        size = version.GetFileVersionInfoSizeW(executable, None)
        if not size:
            return ""
        buffer = ctypes.create_string_buffer(size)
        if not version.GetFileVersionInfoW(executable, 0, size, buffer):
            return ""
        value = ctypes.c_void_p()
        length = wintypes.UINT()
        for locale in ("040904b0", "040904e4"):
            sub_block = f"\\StringFileInfo\\{locale}\\CompanyName"
            if version.VerQueryValueW(buffer, sub_block, ctypes.byref(value), ctypes.byref(length)) and value.value:
                return ctypes.wstring_at(value.value, max(0, length.value - 1)).strip()
        return ""


class _GUID(ctypes.Structure):
    _fields_ = [("Data1", wintypes.DWORD), ("Data2", wintypes.WORD), ("Data3", wintypes.WORD), ("Data4", ctypes.c_ubyte * 8)]


class _WINTRUST_FILE_INFO(ctypes.Structure):
    _fields_ = [("cbStruct", wintypes.DWORD), ("pcwszFilePath", wintypes.LPCWSTR), ("hFile", wintypes.HANDLE), ("pgKnownSubject", ctypes.c_void_p)]


class _WINTRUST_DATA(ctypes.Structure):
    _fields_ = [
        ("cbStruct", wintypes.DWORD), ("pPolicyCallbackData", ctypes.c_void_p), ("pSIPClientData", ctypes.c_void_p),
        ("dwUIChoice", wintypes.DWORD), ("fdwRevocationChecks", wintypes.DWORD), ("dwUnionChoice", wintypes.DWORD),
        ("pFile", ctypes.POINTER(_WINTRUST_FILE_INFO)), ("dwStateAction", wintypes.DWORD), ("hWVTStateData", wintypes.HANDLE),
        ("pwszURLReference", wintypes.LPCWSTR), ("dwProvFlags", wintypes.DWORD), ("dwUIContext", wintypes.DWORD),
    ]


def _win_verify_trust(executable: str) -> int:
    action = _GUID(0x00AAC56B, 0xCD44, 0x11D0, (ctypes.c_ubyte * 8)(0x8C, 0xC2, 0x00, 0xC0, 0x4F, 0xC2, 0x95, 0xEE))
    file_info = _WINTRUST_FILE_INFO(ctypes.sizeof(_WINTRUST_FILE_INFO), executable, None, None)
    data = _WINTRUST_DATA()
    data.cbStruct = ctypes.sizeof(_WINTRUST_DATA)
    data.dwUIChoice = 2
    data.fdwRevocationChecks = 0
    data.dwUnionChoice = 1
    data.pFile = ctypes.pointer(file_info)
    data.dwStateAction = 0
    data.dwProvFlags = 0x00000100
    verify = ctypes.windll.wintrust.WinVerifyTrust
    verify.restype = ctypes.c_long
    return int(verify(None, ctypes.byref(action), ctypes.byref(data)))
