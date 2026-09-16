import unittest
from pathlib import Path
import subprocess


class SetupBuildTests(unittest.TestCase):
    def test_signature_verification_command_uses_authenticode_policy(self):
        from scripts.build_windows_setup import signature_verify_command

        command = signature_verify_command(Path(r"C:\AetherisCoreService.exe"), Path(r"C:\\signtool.exe"))
        self.assertEqual(command[:3], [r"C:\signtool.exe", "verify", "/pa"])
        self.assertEqual(command[-1], r"C:\AetherisCoreService.exe")

    def test_installer_signature_command_uses_configured_certificate(self):
        from scripts.build_windows_setup import signature_sign_command

        command = signature_sign_command(
            Path(r"C:\AetherisSetup.exe"),
            Path(r"C:\signtool.exe"),
            "certificate-thumbprint",
        )
        self.assertEqual(
            command,
            [
                r"C:\signtool.exe",
                "sign",
                "/fd",
                "SHA256",
                "/sha1",
                "certificate-thumbprint",
                r"C:\AetherisSetup.exe",
            ],
        )
    def test_setup_build_uses_shared_version_and_nsis(self):
        from scripts import build_windows_setup
        from aetheris.version import __version__

        self.assertEqual(build_windows_setup.SETUP_NAME, f"AetherisSetup-{__version__}")
        self.assertEqual(build_windows_setup.CORE_NAME, f"AetherisCore-{__version__}.exe")
        command = build_windows_setup.build_command(
            Path("makensis.exe"), Path("setup.nsi"), version=__version__,
            core=Path(build_windows_setup.CORE_NAME), service=Path("AetherisCoreService.exe"),
            plugin=Path("AetherisProvisioning.dll"),
            output_dir=Path("dist"), gateway_url="http://server",
        )
        self.assertEqual(command[0], "makensis.exe")
        self.assertFalse(any("pyinstaller" in item.casefold() for item in command))

    def test_setup_entry_supports_noninteractive_version(self):
        result = subprocess.run(["python", "scripts/setup_entry.py", "--version"], capture_output=True, text=True, check=False)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "0.4.15")

    def test_nsis_keeps_provisioning_plugin_loaded_and_shows_scan_progress(self):
        root = Path("installer/windows/nsis")
        setup = (root / "AetherisSetup.nsi").read_text(encoding="utf-8")
        projects = (root / "pages/Projects.nsh").read_text(encoding="utf-8")
        self.assertIn("AetherisProvisioning::StartProjectScan /NOUNLOAD", projects)
        self.assertIn("AetherisProvisioning::PollProjectScan /NOUNLOAD", projects)
        self.assertIn("AetherisProvisioning::WriteConfiguration /NOUNLOAD", projects)
        self.assertIn("ProjectsProgress", projects)
        self.assertIn("已扫描目录", projects)
        self.assertIn("PBM_SETMARQUEE", projects)


if __name__ == "__main__": unittest.main()
