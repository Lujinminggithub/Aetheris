import tempfile
import unittest
from pathlib import Path


class NSISBuildTests(unittest.TestCase):
    def test_release_version_matches_package(self):
        from aetheris.version import __version__

        self.assertEqual(__version__, "0.4.15")

    def test_build_command_invokes_makensis_with_required_defines(self):
        from scripts.build_windows_setup import build_command

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            command = build_command(
                root / "makensis.exe",
                root / "AetherisSetup.nsi",
                version="0.4.9",
                core=root / "AetherisCore-0.4.9.exe",
                service=root / "AetherisCoreService.exe",
                plugin=root / "AetherisProvisioning.dll",
                output_dir=root / "dist",
                gateway_url="http://192.168.78.138:8080",
            )

        self.assertEqual(command[0], str(root / "makensis.exe"))
        self.assertIn("/DVERSION=0.4.9", command)
        self.assertIn(f"/DCORE_EXE={root / 'AetherisCore-0.4.9.exe'}", command)
        self.assertIn(f"/DSERVICE_EXE={root / 'AetherisCoreService.exe'}", command)
        self.assertIn(f"/DPLUGIN_DLL={root / 'AetherisProvisioning.dll'}", command)
        self.assertIn(f"/DPLUGIN_DIR={root}", command)
        self.assertIn(f"/DOUTPUT_DIR={root / 'dist'}", command)
        self.assertIn("/DGATEWAY_URL=http://192.168.78.138:8080", command)
        self.assertEqual(command[-1], str(root / "AetherisSetup.nsi"))
        self.assertFalse(any("pyinstaller" in item.casefold() for item in command))

    def test_find_makensis_accepts_explicit_installed_candidate(self):
        from scripts.build_windows_setup import find_makensis

        with tempfile.TemporaryDirectory() as raw:
            candidate = Path(raw) / "makensis.exe"
            candidate.write_bytes(b"fixture")

            self.assertEqual(find_makensis([candidate]), candidate.resolve())


if __name__ == "__main__":
    unittest.main()
