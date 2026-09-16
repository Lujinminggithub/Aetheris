import base64
import json
import subprocess
import unittest
from unittest.mock import patch
from pathlib import Path


class OCRRuntimeTests(unittest.TestCase):
    def test_runtime_worker_contract_is_present_and_supports_languages(self):
        worker = Path("ocr-runtime/worker.mjs").read_text(encoding="utf-8")
        self.assertIn("createWorker", worker)
        self.assertIn("languages", worker)
        self.assertIn("confidence", worker)

    @unittest.skipUnless(Path("ocr-runtime/node_modules/tesseract.js").is_dir(), "OCR npm runtime not installed")
    def test_worker_recognizes_generated_text_image(self):
        from PIL import Image, ImageDraw, ImageFont
        import io

        image = Image.new("RGB", (300, 80), "white")
        ImageDraw.Draw(image).text((10, 20), "Aetheris OCR", fill="black")
        stream = io.BytesIO()
        image.save(stream, format="PNG")
        request = json.dumps({
            "image_base64": base64.b64encode(stream.getvalue()).decode("ascii"),
            "languages": ["eng"],
            "lang_path": str(Path("ocr-runtime/ocr").resolve()),
        })
        process = subprocess.Popen(["node", "ocr-runtime/worker.mjs"], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        stdout, stderr = process.communicate(request + "\n", timeout=90)
        self.assertEqual(process.returncode, 0, stderr)
        result = json.loads(stdout.strip().splitlines()[-1])
        self.assertTrue(result["ok"], result)
        self.assertIn("OCR", result["text"])

    def test_python_engine_uses_bundled_worker_contract(self):
        from PIL import Image, ImageDraw, ImageFont
        from aetheris.adapters.ocr import NodeTesseractEngine

        image = Image.new("RGB", (420, 140), "white")
        font = ImageFont.truetype(r"C:\Windows\Fonts\arial.ttf", 64)
        ImageDraw.Draw(image).text((20, 30), "OCR", fill="black", font=font)
        text = NodeTesseractEngine(["eng"]).extract(image)
        self.assertTrue(text.strip())

    def test_python_engine_decodes_node_output_as_utf8(self):
        from PIL import Image
        from aetheris.adapters.ocr import NodeTesseractEngine

        process = type("Process", (), {"returncode": 0, "communicate": lambda self, *_args, **_kwargs: ('{"ok":true,"text":"中文"}\n', "")})()
        with patch("aetheris.adapters.ocr.subprocess.Popen", return_value=process) as popen:
            text = NodeTesseractEngine(["eng"]).extract(Image.new("RGB", (10, 10), "white"))
        self.assertEqual(text, "中文")
        self.assertEqual(popen.call_args.kwargs["encoding"], "utf-8")
        self.assertEqual(popen.call_args.kwargs["errors"], "replace")
