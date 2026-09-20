from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
import os
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))
from aetheris.version import __version__

DIST = Path(os.environ.get("AETHERIS_DIST_DIR", ROOT / "dist")).resolve()
CORE_NAME = f"AetherisCore-{__version__}"


def find_signtool() -> Path | None:
    configured = os.environ.get("SIGNTOOL")
    candidates = [Path(configured)] if configured else []
    candidates.extend([
        Path(r"D:\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe"),
        Path(r"C:\Program Files (x86)\Windows Kits\10\bin\10.0.26100.0\x64\signtool.exe"),
    ])
    discovered = shutil.which("signtool.exe")
    if discovered:
        candidates.insert(0, Path(discovered))
    return next((candidate.resolve() for candidate in candidates if candidate and candidate.is_file()), None)


def signature_sign_command(binary: Path, signtool: Path, thumbprint: str) -> list[str]:
    return [str(signtool), "sign", "/fd", "SHA256", "/sha1", thumbprint, str(binary)]


def signature_verify_command(binary: Path, signtool: Path) -> list[str]:
    return [str(signtool), "verify", "/pa", "/v", str(binary)]


def build() -> Path:
    pyinstaller = shutil.which("pyinstaller")
    if not pyinstaller:
        raise RuntimeError("PyInstaller is required; install the build dependencies first")
    node = shutil.which("node")
    ocr_runtime = ROOT / "ocr-runtime"
    if not node or not (ocr_runtime / "node_modules" / "tesseract.js").is_dir():
        raise RuntimeError("Node.js and ocr-runtime/node_modules/tesseract.js are required to build the client")
    DIST.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix=f"aetheris-pyi-{CORE_NAME}-"))
    command = [
        pyinstaller,
        "--noconfirm",
        "--clean",
        "--onefile",
        "--windowed",
        "--name",
        CORE_NAME,
        "--distpath",
        str(DIST),
        "--workpath",
        str(work),
        "--specpath",
        str(work),
        "--paths",
        str(ROOT / "src"),
        "--add-binary",
        f"{node};runtime",
        "--add-data",
        f"{ocr_runtime};runtime/ocr-runtime",
        str(ROOT / "scripts" / "tray_entry.py"),
    ]
    try:
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, timeout=300)
        if result.returncode != 0:
            raise RuntimeError(f"PyInstaller failed with exit code {result.returncode}: {result.stderr[-1000:]}")
    finally:
        shutil.rmtree(work, ignore_errors=True)
    executable = DIST / f"{CORE_NAME}.exe"
    if not executable.is_file():
        raise RuntimeError("PyInstaller completed without creating AetherisCore.exe")
    thumbprint = os.environ.get("AETHERIS_SIGN_CERT_THUMBPRINT", "").strip()
    signtool = find_signtool()
    if not thumbprint or not signtool:
        raise RuntimeError("Core 发布构建必须配置代码签名证书和 signtool")
    signed = subprocess.run(signature_sign_command(executable, signtool, thumbprint), cwd=ROOT, capture_output=True, text=True, timeout=120)
    if signed.returncode != 0:
        raise RuntimeError(f"Core 签名失败: {signed.stdout[-1000:]}{signed.stderr[-1000:]}")
    verified = subprocess.run(signature_verify_command(executable, signtool), cwd=ROOT, capture_output=True, text=True, timeout=60)
    if verified.returncode != 0:
        raise RuntimeError(f"Core 未通过 Authenticode 验证: {verified.stdout[-1000:]}{verified.stderr[-1000:]}")
    stable = DIST / "AetherisCore.exe"
    shutil.copy2(executable, stable)
    return stable


if __name__ == "__main__":
    print(f"created {build()}")
