import tempfile
import threading
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import urlopen


class DownloadTests(unittest.TestCase):
    def test_root_and_download_catalog_are_public_and_path_safe(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            downloads = root / "downloads"
            downloads.mkdir()
            (downloads / "AetherisSetup-0.4.15.exe").write_bytes(b"exe-fixture")
            (downloads / "AetherisSetup-0.4.15.exe.sha256").write_text("checksum", encoding="ascii")
            store = EventStore(root / "server.db")
            server = create_gateway_server(store, token="test-token", static_dir=root)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                with urlopen(base + "/", timeout=3) as response:
                    page = response.read().decode("utf-8")
                    self.assertEqual(response.status, 200)
                    self.assertIn("AetherisSetup-0.4.15.exe", page)
                with urlopen(base + "/downloads/", timeout=3) as response:
                    catalog = response.read().decode("utf-8")
                    self.assertEqual(response.status, 200)
                    self.assertIn("AetherisSetup-0.4.15.exe", catalog)
                with urlopen(base + "/downloads/AetherisSetup-0.4.15.exe", timeout=3) as response:
                    self.assertEqual(response.read(), b"exe-fixture")
                with self.assertRaises(HTTPError) as raised:
                    urlopen(base + "/downloads/../server.db", timeout=3)
                self.assertEqual(raised.exception.code, 404)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()


if __name__ == "__main__":
    unittest.main()
