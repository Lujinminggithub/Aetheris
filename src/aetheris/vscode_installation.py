from __future__ import annotations

import json
import os
import subprocess
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable


EXTENSION_ID = "aetheris.aetheris-vscode"
USER_STATES = {"install_declined", "paused_by_user", "not_installed", "awaiting_activation", "active", "error"}
RETRY_DELAY_SECONDS = 300


@dataclass(frozen=True)
class InstallationState:
    state: str
    code_path: str = ""
    extension_version: str = ""
    reason_code: str = ""
    retry_after: float = 0


@dataclass(frozen=True)
class OperationResult:
    state: str
    reason_code: str = ""


class VSCodeInstallation:
    def __init__(
        self,
        state_path: str | Path,
        vsix_path: str | Path,
        *,
        code_path: str | Path | None = None,
        runner: Callable | None = None,
        clock: Callable[[], float] | None = None,
    ):
        self.state_path = Path(state_path)
        self.vsix_path = Path(vsix_path)
        self.code_path = Path(code_path) if code_path else None
        self.runner = runner or subprocess.run
        self.clock = clock or time.time

    def inspect(self) -> InstallationState:
        persisted = self._read_state()
        state = str(persisted.get("state", ""))
        if state in {"install_declined", "paused_by_user", "not_installed", "error", "pending_install"}:
            return InstallationState(state, str(self._find_code() or ""), str(persisted.get("extension_version", "")), str(persisted.get("reason_code", "")), float(persisted.get("retry_after", 0) or 0))
        code = self._find_code()
        if code is None:
            return InstallationState("pending_install")
        if state in {"active", "awaiting_activation"}:
            return InstallationState(state, str(code), str(persisted.get("extension_version", "")))
        return InstallationState("awaiting_activation" if self._installed_version() else "not_installed", str(code), self._installed_version())

    def install(self) -> OperationResult:
        code = self._find_code()
        if code is None:
            self._write_state("pending_install", reason_code="vscode_not_found", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("pending_install", "vscode_not_found")
        if not self.vsix_path.is_file():
            self._write_state("error", reason_code="vsix_missing", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("error", "vsix_missing")
        cli = self._find_cli(code)
        if cli is None:
            self._write_state("error", reason_code="vscode_cli_missing", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("error", "vscode_cli_missing")
        try:
            result = self._run([str(code), str(cli), "--install-extension", str(self.vsix_path), "--force"])
        except subprocess.TimeoutExpired:
            self._write_state("error", reason_code="extension_install_timeout", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("error", "extension_install_timeout")
        except (OSError, subprocess.SubprocessError):
            self._write_state("error", reason_code="extension_install_failed", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("error", "extension_install_failed")
        if result.returncode != 0:
            self._write_state("error", reason_code="extension_install_failed", retry_after=self.clock() + RETRY_DELAY_SECONDS)
            return OperationResult("error", "extension_install_failed")
        self._write_state("awaiting_activation")
        return OperationResult("awaiting_activation")

    def uninstall(self) -> OperationResult:
        code = self._find_code()
        if code is not None:
            cli = self._find_cli(code)
            if cli is None:
                return OperationResult("error", "vscode_cli_missing")
            try:
                result = self._run([str(code), str(cli), "--uninstall-extension", EXTENSION_ID])
            except (OSError, subprocess.SubprocessError):
                return OperationResult("error", "extension_uninstall_failed")
            if result.returncode != 0:
                return OperationResult("error", "extension_uninstall_failed")
        self._write_state("not_installed")
        return OperationResult("not_installed")

    def decline(self) -> OperationResult:
        self._write_state("install_declined")
        return OperationResult("install_declined")

    def pause(self) -> OperationResult:
        self._write_state("paused_by_user")
        return OperationResult("paused_by_user")

    def enable(self) -> OperationResult:
        state = "awaiting_activation" if self._find_code() else "pending_install"
        self._write_state(state)
        return OperationResult(state)

    def should_auto_install(self) -> bool:
        if not self.vsix_path.is_file() or self._installed_version():
            return False
        persisted = self._read_state()
        if not persisted:
            return self._find_code() is not None
        if str(persisted.get("state", "")) not in {"error", "pending_install"}:
            return False
        return float(persisted.get("retry_after", 0) or 0) <= self.clock() and self._find_code() is not None

    def mark_active(self, extension_version: str) -> None:
        self._write_state("active", extension_version=extension_version)

    def _run(self, argv: list[str]):
        environment = dict(os.environ)
        environment["ELECTRON_RUN_AS_NODE"] = "1"
        kwargs = {
            "check": False,
            "capture_output": True,
            "text": True,
            "encoding": "utf-8",
            "errors": "replace",
            "timeout": 30,
            "shell": False,
            "env": environment,
        }
        if os.name == "nt":
            kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
        else:
            kwargs["creationflags"] = 0
        return self.runner(argv, **kwargs)

    @staticmethod
    def _find_cli(code: Path) -> Path | None:
        direct = code.parent / "resources" / "app" / "out" / "cli.js"
        if direct.is_file():
            return direct.resolve()
        candidates = [path for path in code.parent.glob("*/resources/app/out/cli.js") if path.is_file()]
        if not candidates:
            return None
        return max(candidates, key=lambda path: path.stat().st_mtime_ns).resolve()

    def _find_code(self) -> Path | None:
        if self.code_path and self.code_path.is_file():
            return self.code_path.resolve()
        candidates = []
        local = os.environ.get("LOCALAPPDATA")
        program_files = os.environ.get("ProgramFiles")
        if local:
            candidates.append(Path(local) / "Programs" / "Microsoft VS Code" / "Code.exe")
        if program_files:
            candidates.append(Path(program_files) / "Microsoft VS Code" / "Code.exe")
        try:
            import winreg
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Microsoft\Windows\CurrentVersion\App Paths\Code.exe") as key:
                candidates.insert(0, Path(str(winreg.QueryValueEx(key, "")[0])))
        except (ImportError, OSError):
            pass
        return next((candidate.resolve() for candidate in candidates if candidate.is_file()), None)

    def _installed_version(self) -> str:
        root = Path.home() / ".vscode" / "extensions"
        matches = sorted(root.glob(f"{EXTENSION_ID}-*"), reverse=True) if root.is_dir() else []
        return matches[0].name.removeprefix(f"{EXTENSION_ID}-") if matches else ""

    def _read_state(self) -> dict:
        try:
            value = json.loads(self.state_path.read_text(encoding="utf-8"))
            return value if isinstance(value, dict) else {}
        except (OSError, json.JSONDecodeError):
            return {}

    def _write_state(self, state: str, *, extension_version: str = "", reason_code: str = "", retry_after: float = 0) -> None:
        if state not in USER_STATES and state != "pending_install":
            raise ValueError("VS Code 组件状态无效")
        self.state_path.parent.mkdir(parents=True, exist_ok=True)
        temporary = self.state_path.with_suffix(self.state_path.suffix + ".tmp")
        temporary.write_text(json.dumps({"version": 1, "state": state, "extension_version": extension_version, "reason_code": reason_code[:128], "retry_after": max(0, retry_after)}, ensure_ascii=True, separators=(",", ":")), encoding="utf-8")
        temporary.replace(self.state_path)
