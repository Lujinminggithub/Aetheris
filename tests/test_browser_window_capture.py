import unittest
from unittest.mock import patch


class BrowserWindowCaptureTests(unittest.TestCase):
    def test_browser_collection_reports_failure_stage_without_exception_text(self):
        from aetheris.adapters.window import collect_visible_window

        class Health:
            def __init__(self): self.calls = []
            def fail(self, adapter_id, state, stage, error_code, **_): self.calls.append((adapter_id, state, stage, error_code))

        with patch("aetheris.adapters.window.foreground_window", return_value=type("Window", (), {"app_name": "chrome.exe", "url": None})()):
            health = Health()
            result = collect_visible_window({"example.com"}, None, None, health=health)
        self.assertIsNone(result)
        self.assertEqual(health.calls, [("browser", "error", "url_read", "url_unavailable")])

    def test_non_browser_foreground_reports_idle_browser_health(self):
        from aetheris.adapters.window import collect_visible_window

        class Health:
            def __init__(self): self.calls = []
            def finish(self, *args, **kwargs): self.calls.append((args, kwargs))

        with patch("aetheris.adapters.window.foreground_window", return_value=type("Window", (), {"app_name": "notepad.exe", "url": None})()):
            health = Health()
            self.assertIsNone(collect_visible_window({"example.com"}, None, None, health=health))
        self.assertEqual(health.calls[0][0], ("browser", "idle"))
        self.assertEqual(health.calls[0][1]["detected_format"], "foreground:notepad.exe")

    def test_browser_collection_uses_observed_window_handle(self):
        from aetheris.adapters.window import collect_visible_window

        class Health:
            def finish(self, *args, **kwargs): pass
            def fail(self, *args, **kwargs): pass

        with patch("aetheris.adapters.window.foreground_window", return_value=type("Window", (), {"app_name": "msedge.exe", "url": None, "hwnd": 42})()), \
             patch("uiautomation.ControlFromHandle", return_value="observed-window") as from_handle, \
             patch("uiautomation.GetForegroundControl") as foreground, \
             patch("aetheris.adapters.browser_uia.BrowserUrlReader.read", return_value="https://example.com"), \
             patch("aetheris.adapters.browser_uia.ScrollCapture.collect", return_value={"url": "https://example.com", "frames": []}), \
             patch("aetheris.adapters.window.BrowserAllowlistAdapter.capture_frames", return_value={"event_type": "browser.page_view"}):
            result = collect_visible_window({"example.com"}, object(), object(), health=Health())
        self.assertEqual(result["event_type"], "browser.page_view")
        from_handle.assert_called_once_with(42)
        foreground.assert_not_called()

    def test_window_capture_scroll_uses_uiautomation_page_down_name(self):
        from aetheris.adapters.browser_uia import WindowsWindowCapture

        with patch("uiautomation.SendKeys") as send_keys:
            WindowsWindowCapture().scroll(object())
        send_keys.assert_called_once_with("{PAGEDOWN}")

    def test_browser_collection_reports_success_and_domain(self):
        from aetheris.adapters.window import collect_visible_window

        class Health:
            def __init__(self): self.calls = []
            def finish(self, *args, **kwargs): self.calls.append((args, kwargs))
            def fail(self, *args, **kwargs): self.calls.append((args, kwargs))

        with patch("aetheris.adapters.window.foreground_window", return_value=type("Window", (), {"app_name": "msedge.exe", "url": None, "hwnd": 42})()), \
             patch("uiautomation.ControlFromHandle", return_value="observed-window"), \
             patch("aetheris.adapters.browser_uia.BrowserUrlReader.read", return_value="https://example.com/docs"), \
             patch("aetheris.adapters.browser_uia.ScrollCapture.collect", return_value={"url": "https://example.com/docs", "frames": []}), \
             patch("aetheris.adapters.window.BrowserAllowlistAdapter.capture_frames", return_value={"event_type": "browser.page_view", "payload": {"domain": "example.com"}}):
            health = Health()
            result = collect_visible_window({"example.com"}, object(), object(), health=health)
        self.assertEqual(result["event_type"], "browser.page_view")
        self.assertEqual(health.calls[-1][0][0:2], ("browser", "active"))
        self.assertEqual(health.calls[-1][1]["detected_format"], "foreground:msedge.exe:example.com")
    def test_url_reader_accepts_address_bar_value_and_rejects_non_http(self):
        from aetheris.adapters.browser_uia import BrowserUrlReader, FakeControl

        control = FakeControl([FakeControl([], control_type="EditControl", value="https://learn.microsoft.com/docs")])
        self.assertEqual(BrowserUrlReader().read(control), "https://learn.microsoft.com/docs")
        bad = FakeControl([FakeControl([], control_type="EditControl", value="file:///secret")])
        self.assertIsNone(BrowserUrlReader().read(bad))

    def test_url_reader_reaches_real_edge_address_bar_depth(self):
        from aetheris.adapters.browser_uia import BrowserUrlReader, FakeControl

        control = FakeControl([], value="edge")
        for _ in range(8):
            control = FakeControl([control])
        address_bar = FakeControl([], control_type="EditControl", value="https://www.baidu.com/s?wd=aetheris")
        control.children[0].children[0].children[0].children[0].children[0].children[0].children[0].children.append(address_bar)
        self.assertEqual(BrowserUrlReader().read(control), "https://www.baidu.com/s?wd=aetheris")

    def test_scroll_capture_stops_at_allowlisted_url_and_keeps_frames_in_memory(self):
        from aetheris.adapters.browser_uia import ScrollCapture

        class Reader:
            def read(self, control):
                return "https://learn.microsoft.com/docs"

        class Capture:
            def __init__(self): self.calls = 0
            def capture(self, control): self.calls += 1; return f"frame-{self.calls}"
            def scroll(self, control): self.calls += 1

        capture = Capture()
        result = ScrollCapture(Reader(), capture, {"learn.microsoft.com"}, max_frames=3).collect(object())
        self.assertEqual(result["url"], "https://learn.microsoft.com/docs")
        self.assertEqual(result["frames"], ["frame-1", "frame-3", "frame-5"])
        self.assertEqual(capture.calls, 5)


if __name__ == "__main__":
    unittest.main()
