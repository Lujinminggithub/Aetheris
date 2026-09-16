import json
import unittest
from datetime import datetime, timedelta, timezone


class FakeImage:
    def __init__(self): self.closed = False
    def close(self): self.closed = True


class Capture:
    def __init__(self, image): self.image = image
    def capture(self, _window): return self.image


class OCR:
    def __init__(self, text): self.text = text
    def extract(self, _image): return {"state": "captured", "text": self.text}


class ApplicationOCRTests(unittest.TestCase):
    def candidate(self, now=None):
        from aetheris.adapters.application_ocr import ApplicationCaptureCandidate
        from aetheris.adapters.application_window import ApplicationWindow

        return ApplicationCaptureCandidate(
            identity_key="identity-1", project_id="project-1", now=now or datetime(2026, 9, 15, 8, 1, tzinfo=timezone.utc),
            window=ApplicationWindow(42, 7, "designer.exe", "配置 - user@example.com", True, False, False, (0, 0, 800, 600)),
        )

    def adapter(self, image, text, dlp_rules=None):
        from aetheris.adapters.application_ocr import ApplicationOcrAdapter
        from aetheris.command_privacy import CommandPrivacy
        from aetheris.dlp import DlpMatcher

        return ApplicationOcrAdapter(Capture(image), OCR(text), DlpMatcher(dlp_rules), CommandPrivacy(b"device-test-key"))

    def test_success_closes_image_and_returns_only_safe_fields(self):
        image = FakeImage()
        result = self.adapter(image, "配置账号 user@example.com").collect(self.candidate())

        self.assertTrue(image.closed)
        self.assertEqual(result.reason_code, "captured")
        self.assertEqual(result.record["event_type"], "application.activity")
        payload = result.record["payload"]
        self.assertEqual(set(payload), {"application_name", "window_context", "visible_text", "capture_method", "ocr_languages", "confidence"})
        self.assertEqual(payload["capture_method"], "ocr_fallback")
        self.assertEqual(payload["confidence"], "low")
        self.assertNotIn("user@example.com", json.dumps(result.record))

    def test_dlp_block_closes_image_and_never_returns_ocr_text(self):
        image = FakeImage()
        result = self.adapter(image, "password=secret", {"keywords": ["password"]}).collect(self.candidate())

        self.assertTrue(image.closed)
        self.assertIsNone(result.record)
        self.assertEqual(result.reason_code, "dlp_blocked")
        self.assertNotIn("secret", json.dumps(result.health))

    def test_ocr_failure_closes_image_and_returns_fixed_reason(self):
        class FailedOCR:
            def extract(self, _image): raise RuntimeError("private worker detail")

        from aetheris.adapters.application_ocr import ApplicationOcrAdapter
        from aetheris.command_privacy import CommandPrivacy
        from aetheris.dlp import DlpMatcher

        image = FakeImage()
        result = ApplicationOcrAdapter(Capture(image), FailedOCR(), DlpMatcher(), CommandPrivacy(b"device-test-key")).collect(self.candidate())
        self.assertTrue(image.closed)
        self.assertEqual(result.reason_code, "ocr_failed")
        self.assertNotIn("private worker detail", json.dumps(result.health))

    def test_same_content_is_deduplicated_within_five_minute_bucket(self):
        first_image, second_image, third_image = FakeImage(), FakeImage(), FakeImage()
        adapter = self.adapter(first_image, "设计页面")
        first = adapter.collect(self.candidate(datetime(2026, 9, 15, 8, 1, tzinfo=timezone.utc)))
        adapter.capture = Capture(second_image)
        duplicate = adapter.collect(self.candidate(datetime(2026, 9, 15, 8, 4, tzinfo=timezone.utc)))
        adapter.capture = Capture(third_image)
        next_bucket = adapter.collect(self.candidate(datetime(2026, 9, 15, 8, 5, tzinfo=timezone.utc)))

        self.assertIsNotNone(first.record)
        self.assertEqual(duplicate.reason_code, "duplicate_window_content")
        self.assertIsNotNone(next_bucket.record)
        self.assertTrue(first_image.closed and second_image.closed and third_image.closed)

    def test_command_privacy_fingerprint_is_stable_without_exposing_input(self):
        from aetheris.command_privacy import CommandPrivacy

        privacy = CommandPrivacy(b"device-test-key")
        first = privacy.fingerprint("private application context")
        second = privacy.fingerprint("private application context")

        self.assertEqual(first, second)
        self.assertTrue(first.startswith("hmac-sha256:"))
        self.assertNotIn("private", first)


if __name__ == "__main__":
    unittest.main()
