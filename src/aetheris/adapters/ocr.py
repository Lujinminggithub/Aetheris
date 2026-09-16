from __future__ import annotations

import base64
import io
import json
import os
import shutil
import subprocess
import sys
from typing import Any


class ScrollStitcher:
    @staticmethod
    def combine_sizes(frames: list[tuple[int, int]]) -> tuple[int, int]:
        if not frames:
            return (0, 0)
        return (max(width for width, _ in frames), sum(height for _, height in frames))

    @staticmethod
    def stitch(images: list[Any]) -> Any:
        if not images:
            return None
        from PIL import Image

        canvas = Image.new("RGB", ScrollStitcher.combine_sizes([image.size for image in images]), "white")
        offset = 0
        for image in images:
            canvas.paste(image, (0, offset))
            offset += image.height
        return canvas


class OcrCapture:
    """OCR adapter boundary; frames remain in memory and are never persisted."""

    def __init__(self, engine: Any):
        self.engine = engine

    def extract(self, frame: Any) -> dict:
        if self.engine is None:
            return {"state": "blocked", "reason": "ocr_engine_unavailable"}
        text = self.engine.extract(frame)
        return {"state": "captured", "text": text}


class NodeTesseractEngine:
    def __init__(self, languages: list[str] | None = None):
        self.languages = [language for language in (languages or ["eng", "chi_sim"]) if language in {"eng", "chi_sim"}]
        if not self.languages:
            raise ValueError("at least one supported OCR language is required")

    def _paths(self) -> tuple[str, str, str]:
        if getattr(sys, "frozen", False):
            root = os.path.join(getattr(sys, "_MEIPASS", os.path.dirname(sys.executable)), "runtime")
            return os.path.join(root, "node.exe"), os.path.join(root, "ocr-runtime", "worker.mjs"), os.path.join(root, "ocr-runtime", "ocr")
        root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "..", "ocr-runtime"))
        node = shutil.which("node") or ""
        return node, os.path.join(root, "worker.mjs"), os.path.join(root, "ocr")

    def extract(self, frame: Any) -> str:
        node, worker, lang_path = self._paths()
        if not node or not os.path.isfile(worker) or not os.path.isdir(lang_path):
            raise RuntimeError("OCR_RUNTIME_MISSING")
        stream = io.BytesIO()
        frame.save(stream, format="PNG")
        request = json.dumps({"image_base64": base64.b64encode(stream.getvalue()).decode("ascii"), "languages": self.languages, "lang_path": lang_path})
        kwargs = {
            "stdin": subprocess.PIPE,
            "stdout": subprocess.PIPE,
            "stderr": subprocess.PIPE,
            "text": True,
            "encoding": "utf-8",
            "errors": "replace",
            "cwd": os.path.dirname(worker),
        }
        if os.name == "nt":
            kwargs["creationflags"] = subprocess.CREATE_NO_WINDOW
        process = subprocess.Popen([node, worker], **kwargs)
        stdout, stderr = process.communicate(request + "\n", timeout=120)
        if process.returncode != 0:
            raise RuntimeError(f"OCR_WORKER_EXIT_{process.returncode}")
        result = json.loads(stdout.strip().splitlines()[-1])
        if not result.get("ok"):
            raise RuntimeError(result.get("error", "OCR_FAILED"))
        return str(result.get("text", ""))
