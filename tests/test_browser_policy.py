import unittest


class BrowserPolicyTests(unittest.TestCase):
    def test_allowlist_accepts_only_visible_text_for_configured_domains(self):
        from aetheris.adapters.browser import BrowserAllowlistAdapter

        adapter = BrowserAllowlistAdapter({"learn.microsoft.com", "stackoverflow.com"})
        allowed = adapter.capture_page("https://learn.microsoft.com/en-us/windows", "token=Bearer abc123")
        blocked = adapter.capture_page("https://example.com", "private page")
        self.assertEqual(allowed["event_type"], "browser.page_view")
        self.assertNotIn("abc123", str(allowed))
        self.assertIsNone(blocked)

    def test_capture_frames_requires_ocr_and_dlp_clearance_before_event(self):
        from PIL import Image
        from aetheris.adapters.browser import BrowserAllowlistAdapter
        from aetheris.adapters.ocr import OcrCapture
        from aetheris.dlp import DlpMatcher

        class Engine:
            def extract(self, frame):
                return "public documentation"

        adapter = BrowserAllowlistAdapter({"learn.microsoft.com"})
        event = adapter.capture_frames(
            "https://learn.microsoft.com/docs",
            [Image.new("RGB", (10, 5), "white"), Image.new("RGB", (10, 7), "white")],
            OcrCapture(Engine()),
            DlpMatcher({"keywords": ["secret"]}),
        )
        self.assertEqual(event["event_type"], "browser.page_view")
        self.assertEqual(event["payload"]["ocr_state"], "captured")


if __name__ == "__main__":
    unittest.main()
