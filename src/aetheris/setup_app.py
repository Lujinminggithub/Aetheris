from __future__ import annotations

import json
import csv
import io
import os
import shutil
import socket
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Callable

from .credentials import CredentialStore
from .gateway import GatewayClient
from .identity import default_user_identifier, device_id, machine_guid, subject_id
from .projects import ProjectSource, discover_projects
from .version import __version__


DEFAULT_GATEWAY_URL = "http://192.168.78.138:8080"


@dataclass(frozen=True)
class InstallRequest:
    install_dir: Path
    gateway_url: str
    projects: list[ProjectSource]
    user_identifier: str
    enrollment_code: str
    start_with_windows: bool = True
    scan_root: Path | None = None


@dataclass(frozen=True)
class InstallResult:
    executable: Path
    config_path: Path
    device_id: str
    subject_id: str


class Installer:
    def __init__(
        self,
        core_source: str | Path,
        gateway_factory: Callable[[str], GatewayClient] = GatewayClient,
        credential_store_factory: Callable[[Path], CredentialStore] = CredentialStore,
        startup=None,
        launcher=None,
        status_probe=None,
        *,
        machine_id: str | None = None,
        ai_roots_provider=None,
    ):
        self.core_source = Path(core_source)
        self.gateway_factory = gateway_factory
        self.credential_store_factory = credential_store_factory
        self.startup = startup or WindowsStartupRegistrar()
        self.launcher = launcher or launch_core
        self.status_probe = status_probe or wait_for_core_status
        self.machine_id = machine_id or machine_guid()
        if ai_roots_provider is None:
            from .tray import default_ai_session_roots

            ai_roots_provider = default_ai_session_roots
        self.ai_roots_provider = ai_roots_provider

    def install(self, request: InstallRequest) -> InstallResult:
        if not request.projects:
            raise ValueError("至少选择一个 Git 或 SVN 项目")
        if not request.enrollment_code.strip():
            raise ValueError("enrollment code 不能为空")
        if not self.core_source.is_file():
            raise ValueError("安装包中缺少 AetherisCore.exe")
        install_dir = request.install_dir.expanduser().resolve()
        config_dir, data_dir, logs_dir = install_dir / "config", install_dir / "data", install_dir / "logs"
        for directory in (install_dir, config_dir, data_dir, logs_dir):
            directory.mkdir(parents=True, exist_ok=True)
        status_path = install_dir / "install-status.json"
        log_path = logs_dir / "setup.log"

        def status(state: str, stage: str, message: str) -> None:
            record = {"state": state, "stage": stage, "message": message, "timestamp": time.time(), "version": __version__}
            _atomic_json(status_path, record)
            with log_path.open("a", encoding="utf-8") as handle:
                handle.write(f"{record['timestamp']} [{state}] {stage} - {message}\n")

        normalized_identifier = request.user_identifier.strip()
        resolved_subject_id = subject_id(normalized_identifier)
        resolved_device_id = device_id(normalized_identifier, self.machine_id)
        status("running", "bootstrap_device", "正在注册设备")
        gateway = self.gateway_factory(request.gateway_url.rstrip("/"))
        try:
            enrollment = gateway.bootstrap_device(
                enrollment_secret=request.enrollment_code.strip(),
                device_id=resolved_device_id,
                subject_id=resolved_subject_id,
                subject_name=normalized_identifier,
                client_version=__version__,
            )
            if enrollment.get("device_id") != resolved_device_id or enrollment.get("subject_id") != resolved_subject_id or not enrollment.get("device_token") or not enrollment.get("tenant_id"):
                raise ValueError("服务端返回的设备身份不一致")
            credential_path = config_dir / "device.credential"
            credential_store = self.credential_store_factory(credential_path)
            credential_store.write(enrollment["device_token"])
            if credential_store.read() != enrollment["device_token"]:
                raise ValueError("device token 加密校验失败")
            gateway.token = enrollment["device_token"]
            heartbeat = gateway.heartbeat()
            if heartbeat.get("status") != "online" or heartbeat.get("device_id") != resolved_device_id or heartbeat.get("subject_id") != resolved_subject_id:
                raise ValueError("设备 heartbeat 身份校验失败")
        except Exception as exc:
            status("failed", "bootstrap_device", str(exc))
            raise

        executable = install_dir / "AetherisCore.exe"
        temporary_executable = install_dir / "AetherisCore.exe.new"
        shutil.copy2(self.core_source, temporary_executable)
        temporary_executable.replace(executable)
        _write_uninstaller(install_dir)
        config = {
            "config_version": 2,
            "core_version": __version__,
            "gateway_url": request.gateway_url.rstrip("/"),
            "tenant_id": enrollment["tenant_id"],
            "credential_kind": "device_token",
            "credential_file": str(credential_path),
            "scan_root": str((request.scan_root or Path(os.path.commonpath([str(item.path) for item in request.projects]))).expanduser().resolve()),
            "project_root": str(request.projects[0].path.resolve()),
            "project_roots": [item.to_dict() for item in request.projects],
            "authorized_roots": [str(item.path.resolve()) for item in request.projects],
            "queue": str(data_dir / "client.db"),
            "device_id": resolved_device_id,
            "subject_id": resolved_subject_id,
            "subject_name": normalized_identifier,
            "executable": str(executable),
            "interval_seconds": 15,
            "heartbeat_seconds": 60,
            "show_status_on_first_run": True,
            "browser_allowlist": [],
            "browser_policy_revision": 0,
            "ai_session_roots": [
                {"tool": str(tool), "path": str(Path(path).expanduser().resolve())}
                for tool, path in self.ai_roots_provider()
            ],
        }
        config_path = config_dir / "aetheris.json"
        _atomic_json(config_path, config)
        _restrict_file(config_path)
        if request.start_with_windows:
            self.startup.register(executable, config_path)
        try:
            self.launcher(executable, config_path)
            if not self.status_probe(data_dir / "core-status.json", 45):
                raise RuntimeError("Core 未在规定时间内写入运行状态")
        except Exception as exc:
            if request.start_with_windows:
                self.startup.remove()
            status("failed", "launch_core", str(exc))
            raise
        legacy_token = config_dir / "gateway.env"
        if legacy_token.is_file():
            legacy_token.unlink()
        status("success", "complete", "安装、设备注册和 Core 启动验证完成")
        return InstallResult(executable, config_path, resolved_device_id, resolved_subject_id)


class WindowsStartupRegistrar:
    def register(self, executable: Path, config_path: Path) -> None:
        if os.name != "nt":
            return
        import winreg
        command = f'"{executable}" --config "{config_path}"'
        with winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Microsoft\Windows\CurrentVersion\Run", 0, winreg.KEY_SET_VALUE) as key:
            winreg.SetValueEx(key, "AetherisCore", 0, winreg.REG_SZ, command)

    def remove(self) -> None:
        if os.name != "nt":
            return
        import winreg
        try:
            with winreg.OpenKey(winreg.HKEY_CURRENT_USER, r"Software\Microsoft\Windows\CurrentVersion\Run", 0, winreg.KEY_SET_VALUE) as key:
                winreg.DeleteValue(key, "AetherisCore")
        except FileNotFoundError:
            pass


def launch_core(executable: Path, config_path: Path) -> None:
    kwargs = {"cwd": executable.parent}
    if os.name == "nt":
        kwargs["creationflags"] = subprocess.DETACHED_PROCESS | subprocess.CREATE_NO_WINDOW
    subprocess.Popen([str(executable), "--config", str(config_path)], **kwargs)


def wait_for_core_status(path: Path, timeout_seconds: int) -> bool:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
            if data.get("version") == __version__ and data.get("registration", {}).get("state") == "registered":
                return True
        except (OSError, json.JSONDecodeError):
            pass
        time.sleep(0.5)
    return False


def _atomic_json(path: Path, value: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=True, indent=2), encoding="utf-8")
    temporary.replace(path)


def _restrict_file(path: Path) -> None:
    os.chmod(path, 0o600)
    if os.name == "nt":
        identity = os.environ.get("USERNAME", "")
        if identity:
            subprocess.run(["icacls", str(path), "/inheritance:r", "/grant:r", f"{identity}:(R,W)"], check=False, capture_output=True, creationflags=subprocess.CREATE_NO_WINDOW)


def _write_uninstaller(install_dir: Path) -> None:
    script = r'''$ErrorActionPreference = "Stop"
$installDir = Split-Path -Parent $MyInvocation.MyCommand.Path
Remove-ItemProperty -Path "HKCU:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "AetherisCore" -ErrorAction SilentlyContinue
Get-Process -Name "AetherisCore" -ErrorAction SilentlyContinue | Stop-Process
foreach ($name in @("AetherisCore.exe", "config", "data", "logs", "install-status.json")) {
    $target = Join-Path $installDir $name
    if (Test-Path -LiteralPath $target) { Remove-Item -LiteralPath $target -Recurse -Force }
}
Write-Host "Aetheris Core 已卸载。安装目录中的其他文件未被删除。"
'''
    (install_dir / "uninstall.ps1").write_text(script, encoding="utf-8")


def embedded_core_path() -> Path:
    root = Path(getattr(sys, "_MEIPASS", Path(__file__).resolve().parents[2]))
    candidates = [root / "payload" / "AetherisCore.exe", root / "AetherisCore.exe"]
    for candidate in candidates:
        if candidate.is_file():
            return candidate
    raise FileNotFoundError("安装包中缺少 AetherisCore.exe")


def run_setup_gui() -> None:
    import tkinter as tk
    from tkinter import filedialog, messagebox, simpledialog

    root = tk.Tk()
    root.withdraw()
    try:
        running = running_core_processes()
        if running:
            raise RuntimeError("检测到正在运行的旧版 Core，请先从通知区域或任务管理器退出后重新运行 Setup：" + ", ".join(running))
        default_install = Path(os.environ.get("LOCALAPPDATA", Path.home())) / "Aetheris"
        install_dir = filedialog.askdirectory(title="选择 Aetheris 安装目录", initialdir=default_install.parent, mustexist=False, parent=root)
        if not install_dir:
            return
        scan_root = filedialog.askdirectory(title="选择 Git/SVN 项目扫描根目录", parent=root)
        if not scan_root:
            raise ValueError("必须选择项目扫描根目录")
        scan = discover_projects(scan_root)
        if not scan.projects:
            raise ValueError("所选目录 8 层内没有发现 Git 或 SVN 项目")
        projects = _select_projects(root, scan.projects, scan.truncated)
        if not projects:
            raise ValueError("至少选择一个项目")
        identifier = simpledialog.askstring("Aetheris Setup", "用户标识：", initialvalue=default_user_identifier(), parent=root)
        if not identifier:
            raise ValueError("用户标识不能为空")
        enrollment = simpledialog.askstring("Aetheris Setup", "Enrollment code：", show="*", parent=root)
        if not enrollment:
            raise ValueError("Enrollment code 不能为空")
        result = Installer(embedded_core_path()).install(InstallRequest(Path(install_dir), DEFAULT_GATEWAY_URL, projects, identifier, enrollment, scan_root=Path(scan_root)))
        messagebox.showinfo("Aetheris Setup", f"安装完成\n\nCore: {result.executable}\n已完成服务端注册和 heartbeat。", parent=root)
    except Exception as exc:
        messagebox.showerror("Aetheris Setup 失败", str(exc), parent=root)
        raise
    finally:
        root.destroy()


def _select_projects(parent, projects: list[ProjectSource], truncated: bool) -> list[ProjectSource]:
    import tkinter as tk

    dialog = tk.Toplevel(parent)
    dialog.title("选择允许采集的项目")
    dialog.geometry("760x460")
    label = "已发现以下 Git/SVN 项目。请选择允许采集的项目。"
    if truncated:
        label += " 结果已达到 100 项上限。"
    tk.Label(dialog, text=label, anchor="w").pack(fill="x", padx=16, pady=(16, 8))
    listbox = tk.Listbox(dialog, selectmode=tk.MULTIPLE)
    listbox.pack(fill="both", expand=True, padx=16, pady=8)
    for item in projects:
        listbox.insert(tk.END, f"[{item.vcs.upper()}] {item.path}")
    listbox.select_set(0, tk.END)
    selected: list[int] = []
    def confirm():
        selected.extend(int(index) for index in listbox.curselection())
        dialog.destroy()
    tk.Button(dialog, text="确认项目", command=confirm).pack(pady=(4, 16))
    dialog.transient(parent)
    dialog.grab_set()
    parent.wait_window(dialog)
    return [projects[index] for index in selected]


def running_core_processes() -> list[str]:
    if os.name != "nt":
        return []
    result = subprocess.run(
        ["tasklist", "/FO", "CSV", "/NH"],
        check=False,
        capture_output=True,
        text=True,
        timeout=10,
        creationflags=subprocess.CREATE_NO_WINDOW,
    )
    names = []
    for row in csv.reader(io.StringIO(result.stdout)):
        if not row:
            continue
        name = row[0]
        folded = name.casefold()
        if folded.startswith("aetheriscore") and folded.endswith(".exe"):
            names.append(name)
    return sorted(set(names), key=str.casefold)
