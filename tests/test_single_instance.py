import unittest


class SingleInstanceTests(unittest.TestCase):
    def test_mutex_name_is_user_scoped_and_does_not_expose_sid(self):
        from aetheris.single_instance import mutex_name
        name = mutex_name("S-1-5-21-secret")
        self.assertTrue(name.startswith("Local\\AetherisCore-"))
        self.assertNotIn("secret", name)
        self.assertEqual(name, mutex_name("S-1-5-21-secret"))

    def test_existing_instance_status_url_is_read_from_status_file(self):
        import json
        import tempfile
        from pathlib import Path
        from aetheris.single_instance import existing_status_url
        with tempfile.TemporaryDirectory() as raw:
            config = Path(raw) / "config" / "aetheris.json"; config.parent.mkdir()
            data = Path(raw) / "data"; data.mkdir()
            config.write_text(json.dumps({"queue": str(data / "client.db")}), encoding="utf-8")
            (data / "core-status.json").write_text(json.dumps({"local_view_url":"http://127.0.0.1:10101/"}), encoding="utf-8")
            self.assertEqual(existing_status_url(config), "http://127.0.0.1:10101/")


if __name__ == "__main__": unittest.main()
