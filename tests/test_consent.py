import tempfile
import unittest
from pathlib import Path


class ConsentTests(unittest.TestCase):
    def test_project_grant_allows_matching_identity_and_revokes_on_hash_change(self):
        from aetheris.consent import ConsentGrant, ConsentStore, ProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            store = ConsentStore(Path(raw) / "consent.db")
            identity = ProcessIdentity("code.exe", "C:/Code.exe", "publisher", "hash-a")
            store.save(ConsentGrant(identity, "project", "project-a", "allow"))
            self.assertEqual(store.decide(identity, project_id="project-a"), "allowed")
            changed = ProcessIdentity("code.exe", "C:/Code.exe", "publisher", "hash-b")
            self.assertEqual(store.decide(changed, project_id="project-a"), "pending")
            store.close()

    def test_always_ignore_is_terminal_for_identity(self):
        from aetheris.consent import ConsentGrant, ConsentStore, ProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            store = ConsentStore(Path(raw) / "consent.db")
            identity = ProcessIdentity("unknown.exe", "C:/unknown.exe", "", "hash-a")
            store.save(ConsentGrant(identity, "global", "", "always_ignore"))
            self.assertEqual(store.decide(identity, project_id="project-a"), "excluded")
            store.close()


if __name__ == "__main__":
    unittest.main()
