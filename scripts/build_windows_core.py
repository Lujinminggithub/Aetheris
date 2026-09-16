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
    stable = DIST / "AetherisCore.exe"
    shutil.copy2(executable, stable)
    return stable


if __name__ == "__main__":
    print(f"created {build()}")
