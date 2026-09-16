import tempfile
import unittest
import zipfile
from pathlib import Path


class CoreBundleTests(unittest.TestCase):
    def test_builder_creates_installable_bundle_without_credentials(self):
        from scripts.build_core_bundle import build

        with tempfile.TemporaryDirectory() as raw:
            output = Path(raw)
            (output / "AetherisCore.exe").write_bytes(b"MZ core fixture")
            archive, checksum = build(output)
            self.assertTrue(archive.is_file())
            self.assertTrue(checksum.is_file())
            with zipfile.ZipFile(archive) as bundle:
                names = set(bundle.namelist())
            self.assertNotIn("install.ps1", names)
            self.assertNotIn("install.cmd", names)
            self.assertNotIn("run-core.ps1", names)
            self.assertNotIn("run-core.cmd", names)
            self.assertIn("AetherisCore.exe", names)
            self.assertIn("README_INSTALL.md", names)
            self.assertNotIn("gateway.env", names)
            archive_text = archive.read_bytes().decode("latin1")
            self.assertNotIn("AETHERIS_TOKEN=", archive_text)
            self.assertNotIn(b"\r", checksum.read_bytes())


if __name__ == "__main__":
    unittest.main()
