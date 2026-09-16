import tempfile
import unittest
from pathlib import Path


class PrefixProtector:
    def protect(self, value: bytes) -> bytes:
        return b"protected:" + value[::-1]

    def unprotect(self, value: bytes) -> bytes:
        if not value.startswith(b"protected:"):
            raise ValueError("damaged")
        return value[len(b"protected:"):][::-1]


class CredentialTests(unittest.TestCase):
    def test_store_round_trip_and_rejects_damaged_data(self):
        from aetheris.credentials import CredentialStore

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "device.credential"
            store = CredentialStore(path, PrefixProtector())
            store.write("device-token")
            self.assertEqual(store.read(), "device-token")
            self.assertNotIn(b"device-token", path.read_bytes())
            path.write_bytes(b"damaged")
            with self.assertRaises(ValueError):
                store.read()


if __name__ == "__main__":
    unittest.main()
