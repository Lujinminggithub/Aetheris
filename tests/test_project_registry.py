import json
import tempfile
import unittest
from pathlib import Path


class ProjectRegistryTests(unittest.TestCase):
    def test_identity_metadata_survives_pause_and_legacy_entries_still_load(self):
        from aetheris.project_registry import ProjectRegistry

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config = root / "config" / "aetheris.json"
            config.parent.mkdir()
            identified = root / "identified"
            legacy = root / "legacy"
            identified.mkdir()
            legacy.mkdir()
            config.write_text(json.dumps({
                "project_revision": 4,
                "project_roots": [
                    {
                        "path": str(identified),
                        "vcs": "git",
                        "display_name": "服务端",
                        "local_project_id": "project-identity",
                        "remote_fingerprint": "hmac-sha256:v2:abc",
                        "root_fingerprint": "hmac-sha256:v2:def",
                        "workspace_kind": "worktree",
                        "worktree_name": "功能分支",
                        "key_version": 2,
                    },
                    {"path": str(legacy), "vcs": "git"},
                ],
                "authorized_roots": [str(identified), str(legacy)],
            }, ensure_ascii=False), encoding="utf-8")
            registry = ProjectRegistry(config)

            snapshot = registry.load()
            self.assertEqual(snapshot.projects[0].display_name, "服务端")
            self.assertEqual(snapshot.projects[0].local_project_id, "project-identity")
            self.assertEqual(snapshot.projects[1].display_name, "legacy")
            self.assertIsNone(snapshot.projects[1].remote_fingerprint)

            paused = registry.mutate(4, "pause", identified)
            project = next(item for item in paused.projects if item.path == identified.resolve())
            self.assertEqual(project.state, "paused")
            self.assertEqual(project.remote_fingerprint, "hmac-sha256:v2:abc")
            self.assertEqual(project.worktree_name, "功能分支")
            stored = json.loads(config.read_text(encoding="utf-8"))
            stored_project = next(item for item in stored["project_roots"] if item["path"] == str(identified.resolve()))
            self.assertEqual(stored_project["local_project_id"], "project-identity")
            self.assertEqual(stored_project["key_version"], 2)

    def test_zero_project_config_is_valid_and_mutations_are_revisioned(self):
        from aetheris.project_registry import ProjectRegistry, RevisionConflict

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config = root / "config" / "aetheris.json"
            config.parent.mkdir()
            config.write_text(json.dumps({
                "gateway_url": "http://server",
                "credential_file": str(config.parent / "device.credential"),
                "queue": str(root / "data" / "client.db"),
                "project_revision": 0,
                "project_roots": [],
                "authorized_roots": [],
            }), encoding="utf-8")
            project = root / "repo"
            (project / ".git").mkdir(parents=True)
            registry = ProjectRegistry(config)

            self.assertEqual(registry.load().projects, [])
            added = registry.mutate(0, "add", project)
            self.assertEqual(added.revision, 1)
            self.assertEqual(added.projects[0].vcs, "git")
            self.assertEqual(added.projects[0].state, "active")
            with self.assertRaises(RevisionConflict):
                registry.mutate(0, "pause", project)

            paused = registry.mutate(1, "pause", project)
            self.assertEqual(paused.projects[0].state, "paused")
            resumed = registry.mutate(2, "resume", project)
            self.assertEqual(resumed.projects[0].state, "active")
            removed = registry.mutate(3, "remove", project)
            self.assertEqual(removed.projects, [])

            stored = json.loads(config.read_text(encoding="utf-8"))
            self.assertEqual(stored["project_revision"], 4)
            self.assertNotIn("project_root", stored)
            actions = [json.loads(line)["action"] for line in (root / "data" / "project-audit.jsonl").read_text(encoding="utf-8").splitlines()]
            self.assertEqual(actions, ["add", "pause", "resume", "remove"])

    def test_markerless_directory_is_pending_classification(self):
        from aetheris.project_registry import ProjectRegistry

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config = root / "config" / "aetheris.json"
            config.parent.mkdir()
            config.write_text(json.dumps({"project_revision": 0, "project_roots": [], "authorized_roots": []}), encoding="utf-8")
            folder = root / "plain"
            folder.mkdir()

            snapshot = ProjectRegistry(config).mutate(0, "add", folder)

            self.assertEqual(snapshot.projects[0].vcs, "pending")
            self.assertEqual(snapshot.projects[0].classification, "pending_classification")

    def test_deleted_project_remains_visible_as_unavailable_for_removal(self):
        from aetheris.project_registry import ProjectRegistry

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            missing = root / "deleted-repo"
            config = root / "config" / "aetheris.json"; config.parent.mkdir()
            config.write_text(json.dumps({"project_revision": 2, "project_roots": [{"path": str(missing), "vcs": "git"}], "authorized_roots": [str(missing)]}), encoding="utf-8")

            snapshot = ProjectRegistry(config).load()

            self.assertEqual(len(snapshot.projects), 1)
            self.assertFalse(snapshot.projects[0].to_dict()["available"])
            removed = ProjectRegistry(config).mutate(2, "remove", missing)
            self.assertEqual(removed.projects, [])


if __name__ == "__main__":
    unittest.main()
