import subprocess
import tempfile
import unittest
from pathlib import Path


class NSISScriptTests(unittest.TestCase):
    def test_makensis_compiles_single_window_installer(self):
        from scripts.build_provisioning_plugin import build as build_plugin
        from scripts.build_windows_setup import build_command, find_makensis

        root = Path(__file__).resolve().parents[1]
        script = root / "installer" / "windows" / "nsis" / "AetherisSetup.nsi"
        plugin = build_plugin()
        with tempfile.TemporaryDirectory() as raw:
            work = Path(raw)
            core = work / "AetherisCore-0.4.9.exe"
            core.write_bytes(b"MZ fixture core")
            service = work / "AetherisCoreService.exe"
            service.write_bytes(b"MZ fixture service")
            command = build_command(
                find_makensis(), script, version="0.4.9", core=core,
                service=service, plugin=plugin, output_dir=work, gateway_url="http://127.0.0.1:18080",
            )
            result = subprocess.run(command, cwd=root, capture_output=True, text=True, encoding="utf-8", errors="replace")
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            installer = work / "AetherisSetup-0.4.9.exe"
            self.assertTrue(installer.is_file())
            self.assertGreater(installer.stat().st_size, core.stat().st_size)

        source = script.read_text(encoding="utf-8")
        enrollment = (script.parent / "pages" / "Enrollment.nsh").read_text(encoding="utf-8")
        self.assertIn("RequestExecutionLevel admin", source)
        self.assertIn("AetherisProvisioning::InstallCoreService", source)
        self.assertIn("AetherisProvisioning::StartCoreService", source)
        self.assertIn("AetherisProvisioning::RemoveCoreService", source)
        self.assertIn("AetherisProvisioning::VerifyExistingDevice", enrollment)
        self.assertIn("AetherisProvisioning::RequestCoreExit", source)
        self.assertIn("AetherisProvisioning::PrepareCoreServiceDirectory", source)
        self.assertIn("AetherisProvisioning::DeployCoreServiceBinary", source)
        self.assertIn("AetherisProvisioning::RestoreCoreServiceBinary", source)
        self.assertIn("$PROGRAMFILES64\\Aetheris\\Service", source)
        self.assertIn("$INSTDIR\\.installing\\${VERSION}", source)
        self.assertNotIn("StartupTask", source)
        self.assertNotIn("schtasks", source.casefold())
        self.assertNotIn("powershell", source.casefold())
        self.assertNotIn("cmd.exe", source.casefold())
        self.assertIn('Section /o "同时删除本地数据与项目设置"', source)
        self.assertIn('RMDir /r "$INSTDIR"', source)
        self.assertIn('RMDir /r "${SERVICE_ROOT}"', source)
        self.assertIn('RMDir /r "$PROGRAMDATA\\Aetheris"', source)
        uninstall_start = source.index('Section "Uninstall"')
        uninstall_end = source.index('Section /o "同时删除本地数据与项目设置"')
        uninstall_section = source[uninstall_start:uninstall_end]
        self.assertIn('RMDir /r "$INSTDIR"', uninstall_section)
        self.assertIn('RMDir /r "$PROGRAMDATA\\Aetheris"', uninstall_section)
        self.assertIn('"/SERVICEONLY="', source)
        self.assertIn('"/CLEANUPONLY="', source)
        self.assertIn("SetSilent silent", source)
        self.assertNotIn('WriteRegStr HKCU "Software\\Microsoft\\Windows\\CurrentVersion\\Run"', source)


if __name__ == "__main__":
    unittest.main()
