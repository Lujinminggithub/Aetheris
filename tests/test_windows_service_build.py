import tempfile
import unittest
from pathlib import Path


class WindowsServiceBuildTests(unittest.TestCase):
    def test_service_build_produces_x64_gui_executable(self):
        import subprocess
        from scripts.build_windows_service import build

        service = build()
        self.assertTrue(service.is_file())
        result = subprocess.run(
            ["dumpbin.exe", "/headers", str(service)], capture_output=True, text=True,
            encoding="utf-8", errors="replace", check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("machine (x64)", result.stdout)
        self.assertIn("subsystem (Windows GUI)", result.stdout)
        dependencies = subprocess.run(
            ["dumpbin.exe", "/dependents", str(service)], capture_output=True, text=True,
            encoding="utf-8", errors="replace", check=False,
        )
        self.assertNotIn("MSVCP140.dll", dependencies.stdout)
        self.assertNotIn("VCRUNTIME140.dll", dependencies.stdout)


if __name__ == "__main__":
    unittest.main()
