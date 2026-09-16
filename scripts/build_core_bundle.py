from __future__ import annotations

import hashlib
import sys
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "src"))
from aetheris.version import __version__

DIST = ROOT / "dist"


def build(dist: Path | None = None) -> tuple[Path, Path]:
    output = (dist or DIST).resolve()
    output.mkdir(parents=True, exist_ok=True)
    core_executable = output / "AetherisCore.exe"
    if not core_executable.is_file():
        from scripts.build_windows_core import build as build_core

        core_executable = build_core()

    archive_path = output / "aetheris-core-beta.zip"
    files: list[tuple[Path, str]] = [(core_executable, "AetherisCore.exe")]
    readme = ROOT / "installer" / "windows" / "README_INSTALL.md"
    if readme.is_file():
        files.append((readme, "README_INSTALL.md"))
    with zipfile.ZipFile(archive_path, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
        for source, name in files:
            bundle.write(source, name)
    digest = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    checksum_path = output / "aetheris-core-beta.sha256"
    checksum_path.write_bytes(f"{digest}  {archive_path.name}\n".encode("ascii"))
    executable_digest = hashlib.sha256(core_executable.read_bytes()).hexdigest()
    (output / "AetherisCore.exe.sha256").write_bytes(f"{executable_digest}  {core_executable.name}\n".encode("ascii"))
    (output / f"AetherisCore-{__version__}.exe.sha256").write_bytes(
        f"{executable_digest}  AetherisCore-{__version__}.exe\n".encode("ascii")
    )
    versioned = output / f"AetherisCore-{__version__}.exe"
    if not versioned.is_file():
        import shutil

        shutil.copy2(core_executable, versioned)
    return archive_path, checksum_path


if __name__ == "__main__":
    archive, checksum = build()
    print(f"created {DIST / 'AetherisCore.exe'}")
    print(f"created {DIST / 'AetherisCore.exe.sha256'}")
    print(f"created {archive}")
    print(f"created {checksum}")
