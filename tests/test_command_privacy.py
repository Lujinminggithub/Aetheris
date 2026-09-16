import unittest
import tempfile
from pathlib import Path


class CommandPrivacyTests(unittest.TestCase):
    def test_load_or_create_persists_encrypted_device_key(self):
        from aetheris.command_privacy import load_or_create_command_privacy
        from aetheris.credentials import CredentialStore

        class Protector:
            def protect(self, value): return b"encrypted:" + value[::-1]
            def unprotect(self, value): return value[len(b"encrypted:"):][::-1]

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "command-fingerprint.credential"
            factory = lambda target: CredentialStore(target, Protector())
            first = load_or_create_command_privacy(path, credential_store_factory=factory)
            second = load_or_create_command_privacy(path, credential_store_factory=factory)

            self.assertEqual(first.describe("git status").command_hash, second.describe("git status").command_hash)
            self.assertNotIn(b"command-fingerprint-key", path.read_bytes())
    def test_descriptor_is_stable_per_device_and_contains_no_command_text(self):
        from aetheris.command_privacy import CommandPrivacy

        command = 'Get-ChildItem "D:\\secret\\build" -Recurse -Filter private.dll'
        first = CommandPrivacy(b"device-key-a").describe(command)
        same = CommandPrivacy(b"device-key-a").describe(command)
        other_device = CommandPrivacy(b"device-key-b").describe(command)

        self.assertEqual(first.command_hash, same.command_hash)
        self.assertNotEqual(first.command_hash, other_device.command_hash)
        self.assertEqual(first.command_type, "file.search")
        self.assertEqual(first.command_summary, "递归查找文件")
        self.assertNotIn("secret", str(first))
        self.assertFalse(hasattr(first, "command"))


if __name__ == "__main__":
    unittest.main()
