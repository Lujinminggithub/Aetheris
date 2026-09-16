import sqlite3
import tempfile
import unittest
from pathlib import Path


class ProcessConsentFlowTests(unittest.TestCase):
    def test_capture_gate_accepts_only_matching_project_grant(self):
        from aetheris.process_consent import ConsentOutcome, can_capture

        self.assertFalse(can_capture(ConsentOutcome("pending", False, "id"), "logical-1"))
        self.assertTrue(can_capture(ConsentOutcome("allow_global", False, "id"), "logical-2"))
        self.assertTrue(can_capture(ConsentOutcome("allow_project", False, "id", ("logical-1",)), "logical-1"))
        self.assertFalse(can_capture(ConsentOutcome("allow_project", False, "id", ("logical-1",)), "logical-2"))

    def test_unknown_identity_is_pending_once_and_persists_user_decision(self):
        from aetheris.process_consent import ProcessConsentCoordinator, StableProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            coordinator = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            identity = StableProcessIdentity("tool.exe", r"C:\\Tools\\tool.exe", "Acme", "sha256:abc")

            first = coordinator.observe(identity)
            second = coordinator.observe(identity)
            coordinator.decide(identity, "allow_project", ["logical-1"])
            after = coordinator.observe(identity)

            self.assertEqual(first.state, "pending")
            self.assertTrue(first.should_notify)
            self.assertEqual(second.state, "pending")
            self.assertFalse(second.should_notify)
            self.assertEqual(after.state, "allow_project")
            self.assertFalse(after.should_notify)
            self.assertEqual(coordinator.pending(), [])
            coordinator.close()

            reloaded = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            self.assertEqual(reloaded.observe(identity).state, "allow_project")
            reloaded.close()

    def test_publisher_or_binary_hash_change_keeps_name_path_decision(self):
        from aetheris.process_consent import ProcessConsentCoordinator, StableProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            coordinator = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            original = StableProcessIdentity("tool.exe", r"C:\\Tools\\tool.exe", "Acme", "sha256:abc")
            changed = StableProcessIdentity("tool.exe", r"C:\\Tools\\tool.exe", "Other", "sha256:def")
            coordinator.decide(original, "deny", [])
            result = coordinator.observe(changed)
            coordinator.close()
            self.assertEqual(original.key, changed.key)
            self.assertEqual(result.state, "deny")
            self.assertFalse(result.should_notify)

    def test_process_identity_normalizes_windows_name_and_path(self):
        from aetheris.process_consent import StableProcessIdentity

        first = StableProcessIdentity(" TOOL.EXE ", r"C:/Tools/../Tools/tool.exe", "A", "sha256:a")
        second = StableProcessIdentity("tool.exe", r"c:\\tools\\tool.exe", "B", "sha256:b", "valid")

        self.assertEqual(first.key, second.key)

    def test_existing_hash_based_rows_are_collapsed_to_latest_explicit_decision(self):
        from aetheris.process_consent import ProcessConsentCoordinator

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "consent.db"
            coordinator = ProcessConsentCoordinator(path, user_sid="S-1-5-21-test")
            coordinator.close()
            connection = sqlite3.connect(path)
            rows = [
                ("old-a", "Tool.exe", r"C:\\Tools\\tool.exe", "Publisher A", "sha256:a", "always_ignore", "", 10.0, 20.0, 1, 15.0),
                ("old-b", "tool.exe", r"c:/tools/tool.exe", "Publisher B", "sha256:b", "allow_global", "", 12.0, 30.0, 1, 25.0),
                ("old-c", "tool.exe", r"C:\\TOOLS\\tool.exe", "Publisher C", "sha256:c", "pending", "", 14.0, 40.0, 1, None),
            ]
            connection.executemany(
                "INSERT INTO process_consents(user_sid,identity_key,name,executable,publisher,binary_hash,state,project_ids,first_seen,last_seen,prompt_shown,decided_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
                [("S-1-5-21-test", *row) for row in rows],
            )
            connection.commit(); connection.close()

            migrated = ProcessConsentCoordinator(path, user_sid="S-1-5-21-test")
            records = migrated.list_all()
            migrated.close()

            self.assertEqual(len(records), 1)
            self.assertEqual(records[0]["state"], "allow_global")
            self.assertEqual(records[0]["publisher"], "Publisher C")
            self.assertEqual(records[0]["binary_hash"], "sha256:c")
            self.assertEqual(records[0]["first_seen"], 10.0)
            self.assertEqual(records[0]["last_seen"], 40.0)
            self.assertEqual(records[0]["decided_at"], 25.0)
            migrated.close()

    def test_default_excluded_process_never_enters_pending_queue(self):
        from aetheris.process_consent import ProcessConsentCoordinator, StableProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            coordinator = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            result = coordinator.observe(StableProcessIdentity("spoolsv.exe", r"C:\\Windows\\System32\\spoolsv.exe", "Microsoft", "sha256:spool"))
            self.assertEqual(result.state, "default_excluded")
            self.assertFalse(result.should_notify)
            self.assertEqual(coordinator.pending(), [])
            coordinator.close()

    def test_decision_can_be_updated_by_stable_identity_key(self):
        from aetheris.process_consent import ProcessConsentCoordinator, StableProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            coordinator = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            identity = StableProcessIdentity("tool.exe", r"C:\\Tools\\tool.exe", "Acme", "sha256:abc")
            coordinator.observe(identity)
            coordinator.decide_key(identity.key, "allow_global", [])
            self.assertEqual(coordinator.observe(identity).state, "allow_global")
            coordinator.reset_key(identity.key)
            self.assertEqual(coordinator.observe(identity).state, "pending")
            coordinator.close()

    def test_list_all_exposes_pending_and_decided_processes_for_lens(self):
        from aetheris.process_consent import ProcessConsentCoordinator, StableProcessIdentity

        with tempfile.TemporaryDirectory() as raw:
            coordinator = ProcessConsentCoordinator(Path(raw) / "consent.db", user_sid="S-1-5-21-test")
            identity = StableProcessIdentity("tool.exe", r"C:\\Tools\\tool.exe", "Acme", "sha256:abc")
            coordinator.observe(identity)
            self.assertEqual(coordinator.list_all()[0]["state"], "pending")
            coordinator.decide(identity, "deny", [])
            decided = coordinator.list_all()[0]
            coordinator.close()
            self.assertEqual(decided["state"], "deny")
            self.assertIsInstance(decided["decided_at"], float)


if __name__ == "__main__":
    unittest.main()
