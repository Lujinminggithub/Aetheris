import base64
import tempfile
import unittest
from pathlib import Path


class MemoryProtector:
    def protect(self, value: bytes) -> bytes:
        return b"protected:" + value

    def unprotect(self, value: bytes) -> bytes:
        if not value.startswith(b"protected:"):
            raise ValueError("invalid protected value")
        return value[len(b"protected:"):]


class FakeProjectIdentityClient:
    def __init__(self, response=None, error=None):
        self.response = response
        self.error = error

    def project_identity_key(self):
        if self.error:
            raise self.error
        return self.response


class ProjectIdentityKeySyncTests(unittest.TestCase):
    def test_downloads_versioned_key_and_creates_separate_device_root_key(self):
        from aetheris.credentials import ProjectIdentityKeyStore
        from aetheris.project_identity_sync import ProjectIdentityKeySync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            tenant_store = ProjectIdentityKeyStore(root / "tenant.credential", protector=MemoryProtector())
            device_store = ProjectIdentityKeyStore(root / "device.credential", protector=MemoryProtector())
            tenant_key = b"t" * 32
            sync = ProjectIdentityKeySync(
                FakeProjectIdentityClient({"key": base64.b64encode(tenant_key).decode("ascii"), "version": 4}),
                tenant_store,
                device_store,
            )

            keys = sync.sync()

            self.assertEqual(keys.remote_key, tenant_key)
            self.assertEqual(keys.key_version, 4)
            self.assertEqual(len(keys.root_key), 32)
            self.assertNotEqual(keys.remote_key, keys.root_key)
            self.assertEqual(tenant_store.read().key, tenant_key)
            self.assertEqual(device_store.read().key, keys.root_key)

    def test_network_failure_uses_valid_cached_keys(self):
        from aetheris.credentials import ProjectIdentityKeyMaterial, ProjectIdentityKeyStore
        from aetheris.project_identity_sync import ProjectIdentityKeySync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            tenant_store = ProjectIdentityKeyStore(root / "tenant.credential", protector=MemoryProtector())
            device_store = ProjectIdentityKeyStore(root / "device.credential", protector=MemoryProtector())
            tenant_store.write(ProjectIdentityKeyMaterial(b"t" * 32, 2))
            device_store.write(ProjectIdentityKeyMaterial(b"r" * 32, 1))

            keys = ProjectIdentityKeySync(
                FakeProjectIdentityClient(error=OSError("offline secret must not be logged")),
                tenant_store,
                device_store,
            ).sync()

            self.assertEqual(keys.remote_key, b"t" * 32)
            self.assertEqual(keys.root_key, b"r" * 32)
            self.assertEqual(keys.key_version, 2)

    def test_rejects_malformed_or_short_server_key(self):
        from aetheris.credentials import ProjectIdentityKeyStore
        from aetheris.project_identity_sync import ProjectIdentityKeySync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            for response in ({"key": "not-base64", "version": 1}, {"key": base64.b64encode(b"short").decode("ascii"), "version": 1}):
                sync = ProjectIdentityKeySync(
                    FakeProjectIdentityClient(response),
                    ProjectIdentityKeyStore(root / "tenant.credential", protector=MemoryProtector()),
                    ProjectIdentityKeyStore(root / "device.credential", protector=MemoryProtector()),
                )
                with self.assertRaisesRegex(ValueError, "项目身份密钥"):
                    sync.sync()


if __name__ == "__main__":
    unittest.main()
