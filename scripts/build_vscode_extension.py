from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
EXTENSION_ROOT = ROOT / "vscode-extension"
DIST = Path(os.environ.get("AETHERIS_DIST_DIR", ROOT / "dist"))


def build(dist: Path | None = None) -> Path:
    output = (dist or DIST).resolve()
    output.mkdir(parents=True, exist_ok=True)
    npm = shutil.which("npm.cmd") or shutil.which("npm")
    node = shutil.which("node.exe") or shutil.which("node")
    vsce = EXTENSION_ROOT / "node_modules" / "@vscode" / "vsce" / "vsce"
    if not npm or not node or not vsce.is_file():
        raise RuntimeError("需要 Node.js、npm 和 vscode-extension 的 @vscode/vsce")
    build_result = subprocess.run([npm, "run", "build"], cwd=EXTENSION_ROOT, capture_output=True, text=True, timeout=120)
    if build_result.returncode != 0:
        raise RuntimeError(f"VS Code 扩展构建失败: {build_result.stderr[-1000:]}")
    version = _version()
    target = output / f"AetherisVSCode-{version}.vsix"
    package_result = subprocess.run([node, str(vsce), "package", "--no-dependencies", "--out", str(target)], cwd=EXTENSION_ROOT, capture_output=True, text=True, timeout=180)
    if package_result.returncode != 0 or not target.is_file():
        raise RuntimeError(f"VSIX 打包失败: {package_result.stdout[-500:]}{package_result.stderr[-1000:]}")
    digest = hashlib.sha256(target.read_bytes()).hexdigest()
    (output / f"AetherisVSCode-{version}.vsix.sha256").write_text(f"{digest}  {target.name}\n", encoding="ascii", newline="")
    shutil.copy2(target, output / "AetherisVSCode.vsix")
    shutil.copy2(output / f"AetherisVSCode-{version}.vsix.sha256", output / "AetherisVSCode.vsix.sha256")
    return target


def _version() -> str:
    import json

    return str(json.loads((EXTENSION_ROOT / "package.json").read_text(encoding="utf-8"))["version"])


if __name__ == "__main__":
    print(f"created {build()}")
