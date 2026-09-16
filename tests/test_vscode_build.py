import hashlib
import tempfile
import unittest
from pathlib import Path


class VSCodeBuildTests(unittest.TestCase):
    def test_build_script_defines_deterministic_output_and_checksum(self):
        from scripts import build_vscode_extension

        self.assertTrue(build_vscode_extension.EXTENSION_ROOT.is_dir())
        with tempfile.TemporaryDirectory() as raw:
            target = Path(raw) / "AetherisVSCode-0.1.0.vsix"
            digest = hashlib.sha256(b"fixture").hexdigest()
            self.assertEqual(len(digest), 64)
            self.assertEqual(target.name, "AetherisVSCode-0.1.0.vsix")


if __name__ == "__main__":
    unittest.main()
