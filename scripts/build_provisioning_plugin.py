from __future__ import annotations

import shutil
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "native" / "provisioning"
BUILD = ROOT / "build" / "native-provisioning"


def build() -> Path:
    cmake = shutil.which("cmake")
    if not cmake:
        raise RuntimeError("构建 provisioning 插件需要 CMake")
    configure = subprocess.run(
        [cmake, "-S", str(SOURCE), "-B", str(BUILD), "-A", "Win32"],
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
    dll = BUILD / "Release" / "AetherisProvisioning.dll"
    if not dll.is_file():
        raise RuntimeError("CMake 未生成 AetherisProvisioning.dll")
    return dll


if __name__ == "__main__":
    print(f"created {build()}")
