import unittest
from unittest.mock import patch


class FakeWindowAPI:
    def foreground_handle(self): return 42
    def process_id(self, hwnd): return 7
    def is_visible(self, hwnd): return True
    def is_minimized(self, hwnd): return False
    def title(self, hwnd): return "设计工具 - 配置"
    def client_rect(self, hwnd): return (10, 20, 410, 320)


class FakeIdentityProvider:
    def process_path(self, pid): return r"C:\Tools\Designer.exe"


class FakeImage:
    def __init__(self, size): self.size = size


class StrictGrabber:
    def __call__(self, *, bbox, all_screens):
        if bbox != (10, 20, 410, 320) or all_screens is not True:
            raise AssertionError((bbox, all_screens))
        return FakeImage((400, 300))


class Control:
    def __init__(self, children=(), password=False):
        self.children = list(children)
        self.IsPassword = password
    def GetChildren(self): return self.children


class ApplicationWindowTests(unittest.TestCase):
    def test_inspector_returns_foreground_client_area_and_safe_process_name(self):
        from aetheris.adapters.application_window import ApplicationWindowInspector

        window = ApplicationWindowInspector(FakeWindowAPI(), FakeIdentityProvider(), lambda _hwnd: Control()).inspect()

        self.assertEqual(window.hwnd, 42)
        self.assertEqual(window.pid, 7)
        self.assertEqual(window.process_name, "designer.exe")
        self.assertEqual(window.client_rect, (10, 20, 410, 320))
        self.assertFalse(window.has_password_control)

    def test_nested_password_control_is_detected_before_capture(self):
        from aetheris.adapters.application_window import ApplicationWindowInspector

        password_root = Control([Control([Control(password=True)])])
        window = ApplicationWindowInspector(FakeWindowAPI(), FakeIdentityProvider(), lambda _hwnd: password_root).inspect()

        self.assertTrue(window.has_password_control)

    def test_client_area_capture_uses_exact_bbox(self):
        from aetheris.adapters.application_window import ApplicationWindowInspector, ClientAreaCapture

        window = ApplicationWindowInspector(FakeWindowAPI(), FakeIdentityProvider(), lambda _hwnd: Control()).inspect()
        image = ClientAreaCapture(StrictGrabber()).capture(window)

        self.assertEqual(image.size, (400, 300))

    def test_capture_rejects_empty_and_oversized_client_areas(self):
        from dataclasses import replace
        from aetheris.adapters.application_window import ApplicationWindowInspector, ClientAreaCapture

        window = ApplicationWindowInspector(FakeWindowAPI(), FakeIdentityProvider(), lambda _hwnd: Control()).inspect()
        capture = ClientAreaCapture(StrictGrabber())

        with self.assertRaisesRegex(ValueError, "window_client_area_invalid"):
            capture.capture(replace(window, client_rect=(10, 10, 10, 20)))
        with self.assertRaisesRegex(ValueError, "window_client_area_too_large"):
            capture.capture(replace(window, client_rect=(0, 0, 5000, 4000)))

    def test_shared_foreground_lookup_does_not_launch_tasklist(self):
        from aetheris.adapters.window import foreground_window

        with patch("subprocess.run", side_effect=AssertionError("tasklist must not run")), \
             patch("aetheris.adapters.window.ctypes.windll.user32.GetForegroundWindow", return_value=42), \
             patch("aetheris.adapters.window.ctypes.windll.user32.GetWindowTextW", return_value=1), \
             patch("aetheris.adapters.window.ctypes.windll.user32.GetWindowThreadProcessId", side_effect=lambda _h, p: setattr(p._obj, "value", 7) or 1), \
             patch("aetheris.adapters.window.NativeProcessIdentityProvider.process_path", return_value=r"C:\Tools\Designer.exe"):
            observation = foreground_window()

        self.assertEqual(observation.app_name, "Designer.exe")


if __name__ == "__main__":
    unittest.main()
