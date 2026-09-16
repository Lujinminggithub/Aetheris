import subprocess
import unittest


class NativeProvisioningPluginTests(unittest.TestCase):
    def test_build_produces_win32_dll_with_stable_nsis_exports(self):
        from scripts.build_provisioning_plugin import build

        dll = build()
        result = subprocess.run(
            ["dumpbin.exe", "/headers", "/exports", str(dll)],
            check=False,
            capture_output=True,
            text=True,
            encoding="utf-8",
            errors="replace",
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("machine (x86)", result.stdout)
        for name in (
            "ProvisionDevice", "StartProjectScan", "PollProjectScan",
            "PopulateProjectList", "WriteConfiguration", "VerifyCoreStatus", "RevokeDevice",
            "VerifyExistingDevice", "InstallCoreService", "QueryCoreService",
            "StartCoreService", "StopCoreService", "RemoveCoreService",
            "UpdateExistingCoreVersion",
            "RequestCoreExit",
            "PrepareCoreServiceDirectory",
            "DeployCoreServiceBinary", "RestoreCoreServiceBinary",
            "VerifyCoreServiceBinaryAccess",
            "VerifyCoreServiceBinarySignature",
        ):
            self.assertIn(name, result.stdout)
        for obsolete in ("InstallStartupTask", "RemoveStartupTask", "QueryStartupTask", "RunStartupTask"):
            self.assertNotIn(obsolete, result.stdout)


if __name__ == "__main__":
    unittest.main()
