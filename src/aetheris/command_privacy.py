from __future__ import annotations

import hashlib
import hmac
import re
import secrets
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

from .credentials import CredentialStore
from .redaction import Redactor


@dataclass(frozen=True)
class CommandDescriptor:
    command_type: str
    command_summary: str
    command_hash: str


class CommandPrivacy:
    def __init__(self, key: bytes):
        if len(key) < 8:
            raise ValueError("command fingerprint key is too short")
        self._key = key
        self._redactor = Redactor()

    def describe(self, command: str) -> CommandDescriptor:
        redacted, _ = self._redactor.redact(command)
        canonical = re.sub(r"\s+", " ", str(redacted)).strip()
        command_type, summary = _classify(canonical)
        return CommandDescriptor(command_type, summary[:80], self._fingerprint_canonical(canonical))

    def fingerprint(self, value: str) -> str:
        redacted, _ = self._redactor.redact(value)
        canonical = re.sub(r"\s+", " ", str(redacted)).strip()
        return self._fingerprint_canonical(canonical)

    def _fingerprint_canonical(self, canonical: str) -> str:
        digest = hmac.new(self._key, canonical.encode("utf-8"), hashlib.sha256).hexdigest()
        return "hmac-sha256:" + digest


def load_or_create_command_privacy(
    path: str | Path,
    *,
    credential_store_factory: Callable[[Path], CredentialStore] = CredentialStore,
) -> CommandPrivacy:
    target = Path(path).expanduser().resolve()
    store = credential_store_factory(target)
    if target.is_file():
        encoded = store.read()
    else:
        encoded = secrets.token_hex(32)
        store.write(encoded)
    try:
        key = bytes.fromhex(encoded)
    except ValueError as exc:
        raise ValueError("command fingerprint credential is invalid") from exc
    return CommandPrivacy(key)


def _classify(command: str) -> tuple[str, str]:
    value = command.casefold()
    if re.search(r"\b(get-childitem|gci|find|findstr|select-string|rg|grep)\b", value):
        if any(flag in value for flag in ("-filter", "-recurse", "--files", "--glob", "-pattern")) or re.search(r"\b(find|findstr|select-string|rg|grep)\b", value):
            return "file.search", "递归查找文件" if "-recurse" in value else "查找文件或内容"
        return "file.list", "列出目录或文件"
    if re.search(r"(^|\s)(git|svn)(\s|$)", value):
        return "vcs", "执行版本控制操作"
    if re.search(r"\b(test|pytest|unittest|ctest|vitest|jest|go test|dotnet test)\b", value):
        return "test", "执行测试"
    if re.search(r"\b(build|compile|cmake --build|dotnet build|npm run build|makensis)\b", value):
        return "build", "执行构建"
    if re.search(r"\b(curl|wget|invoke-webrequest|test-netconnection|resolve-dnsname)\b", value):
        return "network", "执行网络检查或请求"
    if re.search(r"\b(get-process|start-process|stop-process|tasklist|taskkill)\b", value):
        return "process", "执行进程操作"
    if re.search(r"\b(pip|npm|pnpm|yarn|apt|winget|choco)\b.*\b(install|add|remove|update)\b", value):
        return "package", "执行依赖管理"
    if re.search(r"\.(ps1|py|sh|cmd|bat)(\s|$)", value):
        return "script", "执行脚本"
    return "other", "执行终端操作"
