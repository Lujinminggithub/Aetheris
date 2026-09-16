import hashlib
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


class ProcessIdentityTests(unittest.TestCase):
    def test_identity_provider_hashes_binary_and_preserves_publisher(self):
        from aetheris.processes import NativeProcessIdentityProvider

        with tempfile.TemporaryDirectory() as raw:
            binary = Path(raw) / "tool.exe"
            binary.write_bytes(b"aetheris-process")
            provider = NativeProcessIdentityProvider()
            with patch.object(provider, "process_path", return_value=str(binary)), patch.object(provider, "signature_status", return_value="valid"), patch.object(provider, "publisher", return_value="Acme"):
                identity = provider.inspect(123, "tool.exe")

            self.assertEqual(identity.executable, str(binary.resolve()))
            self.assertEqual(identity.publisher, "Acme")
            self.assertEqual(identity.signature_status, "valid")
            self.assertEqual(identity.binary_hash, "sha256:" + hashlib.sha256(b"aetheris-process").hexdigest())

    def test_missing_path_is_safe_and_does_not_raise(self):
        from aetheris.processes import NativeProcessIdentityProvider

        provider = NativeProcessIdentityProvider()
        with patch.object(provider, "process_path", return_value=None), patch.object(provider, "signature_status", return_value="unknown"), patch.object(provider, "publisher", return_value=""):
            identity = provider.inspect(456, "unknown.exe")
        self.assertEqual(identity.name, "unknown.exe")
        self.assertEqual(identity.executable, "")
        self.assertEqual(identity.publisher, "")
        self.assertEqual(identity.binary_hash, "")
        self.assertEqual(identity.signature_status, "unknown")


if __name__ == "__main__":
    unittest.main()
