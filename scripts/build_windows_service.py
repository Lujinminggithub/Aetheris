from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "native" / "core-service"
BUILD = ROOT / "build" / "core-service"
DIST = Path(os.environ.get("AETHERIS_DIST_DIR", ROOT / "dist")).resolve()


def sign_binary(binary: Path) -> None:
    signtool = os.environ.get("SIGNTOOL") or shutil.which("signtool.exe")
    thumbprint = os.environ.get("AETHERIS_SIGN_CERT_THUMBPRINT")
    if not signtool or not thumbprint:
        return
    result = subprocess.run([signtool, "sign", "/fd", "SHA256", "/sha1", thumbprint, str(binary)], cwd=ROOT, capture_output=True, text=True, timeout=120)
    if result.returncode != 0:
        raise RuntimeError(f"服务签名失败: {result.stdout[-1000:]}{result.stderr[-1000:]}")


def build() -> Path:
    cmake = shutil.which("cmake")
    if not cmake:
        raise RuntimeError("构建 Aetheris Core Service 需要 CMake")
    configure = subprocess.run(
        [cmake, "-S", str(SOURCE), "-B", str(BUILD), "-A", "x64"],
        cwd=ROOT, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=120,
    )
    if configure.returncode != 0:
        raise RuntimeError(configure.stdout + configure.stderr)
    compile_result = subprocess.run(
        [cmake, "--build", str(BUILD), "--config", "Release"],
        cwd=ROOT, capture_output=True, text=True, encoding="utf-8", errors="replace", timeout=300,
    )
    if compile_result.returncode != 0:
        raise RuntimeError(compile_result.stdout + compile_result.stderr)
    source = BUILD / "Release" / "AetherisCoreService.exe"
    if not source.is_file():
        raise RuntimeError("CMake 未生成 AetherisCoreService.exe")
    DIST.mkdir(parents=True, exist_ok=True)
    target = DIST / "AetherisCoreService.exe"
    shutil.copy2(source, target)
    sign_binary(target)
    return target


if __name__ == "__main__":
    print(f"created {build()}")
