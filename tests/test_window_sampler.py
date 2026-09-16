import unittest
from unittest.mock import patch


class WindowSamplerTests(unittest.TestCase):
    def test_non_allowlisted_window_is_ignored(self):
        from aetheris.adapters.window import WindowObservation, collect_visible_window

        with patch("aetheris.adapters.window.foreground_window", return_value=WindowObservation(1, "Chrome", "Example - Chrome", "https://example.com")):
            self.assertIsNone(collect_visible_window({"learn.microsoft.com"}))

    def test_allowlisted_window_returns_metadata_without_screenshot(self):
        from aetheris.adapters.window import WindowObservation, collect_visible_window

        with patch("aetheris.adapters.window.foreground_window", return_value=WindowObservation(1, "Chrome", "Docs - Chrome", "https://learn.microsoft.com/docs")):
            record = collect_visible_window({"learn.microsoft.com"})
        self.assertEqual(record["event_type"], "browser.page_view")
        self.assertNotIn("screenshot", record["payload"])


if __name__ == "__main__":
    unittest.main()
