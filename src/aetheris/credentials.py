from __future__ import annotations

import ctypes
import base64
import json
import os
from ctypes import wintypes
from dataclasses import dataclass
from pathlib import Path


MAGIC = b"AETHERIS-DPAPI-1\0"


@dataclass(frozen=True)
class ProjectIdentityKeyMaterial:
    key: bytes
    version: int


class ProjectIdentityKeyStore:
    def __init__(self, path: str | Path, protector=None):
        self._store = CredentialStore(path, protector=protector)

    @property
    def path(self) -> Path:
        return self._store.path

    def write(self, material: ProjectIdentityKeyMaterial) -> None:
        if len(material.key) < 32 or material.version < 1:
            raise ValueError("项目身份密钥格式无效")
        value = json.dumps({
            "key": base64.b64encode(material.key).decode("ascii"),
            "version": material.version,
        }, separators=(",", ":"))
        self._store.write(value)

    def read(self) -> ProjectIdentityKeyMaterial:
        try:
            value = json.loads(self._store.read())
            key = base64.b64decode(value["key"], validate=True)
            version = int(value["version"])
        except (KeyError, TypeError, ValueError, json.JSONDecodeError) as exc:
            raise ValueError("项目身份密钥凭据格式无效") from exc
        if len(key) < 32 or version < 1:
            raise ValueError("项目身份密钥凭据格式无效")
        return ProjectIdentityKeyMaterial(key, version)


class CredentialStore:
    def __init__(self, path: str | Path, protector=None):
        self.path = Path(path)
        self.protector = protector or WindowsDPAPI()

    def write(self, token: str) -> None:
        if not token:
            raise ValueError("device token 不能为空")
        encrypted = self.protector.protect(token.encode("utf-8"))
        self.path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self.path.with_suffix(self.path.suffix + ".tmp")
        temporary.write_bytes(MAGIC + encrypted)
        os.chmod(temporary, 0o600)
        temporary.replace(self.path)
        _restrict_to_current_user(self.path)

    def read(self) -> str:
        raw = self.path.read_bytes()
        if not raw.startswith(MAGIC):
            raise ValueError("credential 文件格式无效")
        try:
            token = self.protector.unprotect(raw[len(MAGIC):]).decode("utf-8")
        except (OSError, UnicodeDecodeError, ValueError) as exc:
            raise ValueError("credential 无法解密") from exc
        if not token:
            raise ValueError("credential 内容为空")
        return token


class _DataBlob(ctypes.Structure):
    _fields_ = [("cbData", wintypes.DWORD), ("pbData", ctypes.POINTER(ctypes.c_byte))]


class WindowsDPAPI:
    def __init__(self):
        if os.name != "nt":
            raise RuntimeError("Windows DPAPI 只能在 Windows 上使用")

    def protect(self, value: bytes) -> bytes:
        return self._crypt(value, decrypt=False)

    def unprotect(self, value: bytes) -> bytes:
        return self._crypt(value, decrypt=True)

    @staticmethod
    def _crypt(value: bytes, *, decrypt: bool) -> bytes:
        buffer = ctypes.create_string_buffer(value)
        source = _DataBlob(len(value), ctypes.cast(buffer, ctypes.POINTER(ctypes.c_byte)))
        target = _DataBlob()
        crypt32 = ctypes.windll.crypt32
        if decrypt:
            ok = crypt32.CryptUnprotectData(ctypes.byref(source), None, None, None, None, 0, ctypes.byref(target))
        else:
            ok = crypt32.CryptProtectData(ctypes.byref(source), "Aetheris device credential", None, None, None, 0, ctypes.byref(target))
        if not ok:
            raise ctypes.WinError()
        try:
            return ctypes.string_at(target.pbData, target.cbData)
        finally:
            ctypes.windll.kernel32.LocalFree(target.pbData)


def _restrict_to_current_user(path: Path) -> None:
    if os.name != "nt":
        return
    import subprocess
    identity = os.environ.get("USERNAME", "")
    if identity:
        subprocess.run(
            ["icacls", str(path), "/inheritance:r", "/grant:r", f"{identity}:(R,W)"],
            check=False, capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW,
        )
