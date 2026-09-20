from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))

from aetheris.version import __version__


DIST = Path(os.environ.get("AETHERIS_DIST_DIR", ROOT / "dist")).resolve()
SETUP_NAME = f"AetherisSetup-{__version__}"
CORE_NAME = f"AetherisCore-{__version__}.exe"
DEFAULT_GATEWAY_URL = "http://192.168.78.138:8080"


def find_signtool() -> Path | None:
    configured = os.environ.get("SIGNTOOL")
    candidates = [Path(configured)] if configured else []
    candidates.extend([
        Path(r"C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe"),
        Path(r"C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe"),
    ])
    discovered = shutil.which("signtool.exe")
    if discovered:
        candidates.insert(0, Path(discovered))
    return next((candidate.resolve() for candidate in candidates if candidate and candidate.is_file()), None)


def signature_verify_command(service: Path, signtool: Path) -> list[str]:
    return [str(signtool), "verify", "/pa", "/v", str(service)]


def signature_sign_command(binary: Path, signtool: Path, thumbprint: str) -> list[str]:
    return [str(signtool), "sign", "/fd", "SHA256", "/sha1", thumbprint, str(binary)]


def find_makensis(candidates: list[Path] | None = None) -> Path:
    configured = os.environ.get("MAKENSIS")
    search = candidates or [
        Path(configured) if configured else Path(""),
        Path(r"C:\Program Files (x86)\NSIS\makensis.exe"),
        Path(r"C:\Program Files\NSIS\makensis.exe"),
    ]
    discovered = shutil.which("makensis") or shutil.which("makensis.exe")
    if discovered:
        search.insert(0, Path(discovered))
    for candidate in search:
        if candidate and candidate.is_file():
            return candidate.resolve()
    raise RuntimeError("未找到 NSIS makensis；请安装 NSIS 3.x Unicode")


def build_command(
    makensis: Path,
    script: Path,
    *,
    version: str,
    core: Path,
    service: Path,
    plugin: Path,
    output_dir: Path,
    gateway_url: str,
    vsix: Path | None = None,
    vsix_sha256: str | None = None,
) -> list[str]:
    command = [
        str(makensis),
        "/V3",
        "/INPUTCHARSET",
        "UTF8",
        f"/DVERSION={version}",
        f"/DCORE_EXE={core}",
        f"/DSERVICE_EXE={service}",
        f"/DPLUGIN_DLL={plugin}",
        f"/DPLUGIN_DIR={plugin.parent}",
        f"/DOUTPUT_DIR={output_dir}",
        f"/DGATEWAY_URL={gateway_url}",
        str(script),
    ]
    if vsix:
        command.insert(-1, f"/DVSIX_FILE={vsix}")
    if vsix_sha256:
        command.insert(-1, f"/DVSIX_SHA256={vsix_sha256}")
    return command


def build() -> Path:
    core = DIST / CORE_NAME
    service = DIST / "AetherisCoreService.exe"
    plugin = ROOT / "build" / "native-provisioning" / "Release" / "AetherisProvisioning.dll"
    vsix = DIST / "AetherisVSCode.vsix"
    script = ROOT / "installer" / "windows" / "nsis" / "AetherisSetup.nsi"
    for required in (core, service, plugin, script, vsix):
        if not required.is_file():
            raise RuntimeError(f"NSIS 构建输入不存在: {required}")
    signtool = find_signtool()
    require_signed_service = os.environ.get("AETHERIS_REQUIRE_SIGNED_SERVICE", "1") == "1"
    if signtool and require_signed_service:
        verified = subprocess.run(signature_verify_command(service, signtool), cwd=ROOT, capture_output=True, text=True, timeout=60)
        if verified.returncode != 0:
            raise RuntimeError(f"Core Service 未通过 Authenticode 验证: {verified.stdout[-1000:]}{verified.stderr[-1000:]}")
    elif require_signed_service:
        raise RuntimeError("未找到 signtool，无法验证 Core Service 签名")
    DIST.mkdir(parents=True, exist_ok=True)
    command = build_command(
        find_makensis(), script, version=__version__, core=core.resolve(),
        service=service.resolve(), plugin=plugin.resolve(), output_dir=DIST, gateway_url=DEFAULT_GATEWAY_URL,
        vsix=vsix.resolve(), vsix_sha256=hashlib.sha256(vsix.read_bytes()).hexdigest(),
    )
    result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=300)
    if result.returncode != 0:
        raise RuntimeError(f"makensis 构建失败，退出码 {result.returncode}: {result.stdout[-2000:]}{result.stderr[-1000:]}")
    executable = DIST / f"{SETUP_NAME}.exe"
    if not executable.is_file():
        raise RuntimeError("makensis 未生成 Setup EXE")
    thumbprint = os.environ.get("AETHERIS_SIGN_CERT_THUMBPRINT", "").strip()
    if thumbprint:
        if not signtool:
            raise RuntimeError("已配置安装器签名证书，但未找到 signtool")
        signed = subprocess.run(
            signature_sign_command(executable, signtool, thumbprint),
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=120,
        )
        if signed.returncode != 0:
            raise RuntimeError(f"安装器签名失败: {signed.stdout[-1000:]}{signed.stderr[-1000:]}")
        verified = subprocess.run(
            signature_verify_command(executable, signtool),
            cwd=ROOT,
            capture_output=True,
            text=True,
            timeout=60,
        )
        if verified.returncode != 0:
            raise RuntimeError(f"安装器未通过 Authenticode 验证: {verified.stdout[-1000:]}{verified.stderr[-1000:]}")
    digest = hashlib.sha256(executable.read_bytes()).hexdigest()
    (DIST / f"{SETUP_NAME}.exe.sha256").write_text(f"{digest}  {SETUP_NAME}.exe\n", encoding="ascii", newline="")
    shutil.copy2(executable, DIST / "AetherisSetup.exe")
    return executable


if __name__ == "__main__":
    print(f"created {build()}")
