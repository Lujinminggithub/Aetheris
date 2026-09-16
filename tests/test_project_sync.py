import json
import tempfile
import unittest
from pathlib import Path


class RecordingProjectClient:
    def __init__(self):
        self.batches = []

    def register_projects(self, batch):
        self.batches.append(batch)
        return {
            "registry_revision": 9,
            "projects": [
                {
                    "local_project_id": item["local_project_id"],
                    "logical_project_id": "logical-shared",
                    "display_name": item["display_name"],
                    "resolution": "remote_fingerprint",
                    "needs_review": False,
                }
                for item in batch["projects"]
            ],
        }


class ProjectSyncTests(unittest.TestCase):
    def test_builds_path_free_batch_and_preserves_location_state(self):
        from aetheris.project_identity_sync import ProjectIdentityKeys
        from aetheris.project_registry import ProjectRegistry
        from aetheris.project_sync import ProjectSync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            first = root / "first" / "repo"
            second = root / "second" / "repo"
            for project in (first, second):
                (project / ".git").mkdir(parents=True)
                (project / ".git" / "config").write_text(
                    "[remote \"origin\"]\n\turl = https://example.com/team/repo.git\n",
                    encoding="utf-8",
                )
            config = root / "config" / "aetheris.json"
            config.parent.mkdir()
            config.write_text(json.dumps({
                "project_revision": 2,
                "project_roots": [
                    {"path": str(first), "vcs": "git", "state": "active", "display_name": "主仓库"},
                    {"path": str(second), "vcs": "git", "state": "paused", "display_name": "备用工作树"},
                ],
                "authorized_roots": [str(first), str(second)],
            }, ensure_ascii=False), encoding="utf-8")
            snapshot = ProjectRegistry(config).load()

            batch = ProjectSync.build_batch(snapshot, ProjectIdentityKeys(b"t" * 32, b"r" * 32, 3))

            self.assertEqual(len(batch["projects"]), 2)
            self.assertEqual(batch["projects"][0]["remote_fingerprint"], batch["projects"][1]["remote_fingerprint"])
            self.assertNotEqual(batch["projects"][0]["root_fingerprint"], batch["projects"][1]["root_fingerprint"])
            self.assertEqual([item["active"] for item in batch["projects"]], [True, False])
            self.assertEqual([item["display_name"] for item in batch["projects"]], ["主仓库", "备用工作树"])
            serialized = json.dumps(batch, ensure_ascii=False)
            self.assertNotIn(str(root.resolve()), serialized)
            self.assertNotIn("https://", serialized)

    def test_sync_is_idempotent_until_project_or_key_revision_changes(self):
        from aetheris.project_identity_sync import ProjectIdentityKeys
        from aetheris.project_registry import ProjectRegistry
        from aetheris.project_sync import ProjectSync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"
            (project / ".git").mkdir(parents=True)
            config = root / "config" / "aetheris.json"
            config.parent.mkdir()
            config.write_text(json.dumps({
                "project_revision": 1,
                "project_roots": [{"path": str(project), "vcs": "git"}],
                "authorized_roots": [str(project)],
            }), encoding="utf-8")
            registry = ProjectRegistry(config)
            client = RecordingProjectClient()
            sync = ProjectSync(registry, client)

            first = sync.sync(ProjectIdentityKeys(b"t" * 32, b"r" * 32, 1))
            second = sync.sync(ProjectIdentityKeys(b"t" * 32, b"r" * 32, 1))
            registry.mutate(1, "pause", project)
            third = sync.sync(ProjectIdentityKeys(b"t" * 32, b"r" * 32, 1))

            self.assertEqual(first.registry_revision, 9)
            self.assertIsNone(second)
            self.assertEqual(third.registry_revision, 9)
            self.assertEqual(len(client.batches), 2)
            self.assertFalse(client.batches[-1]["projects"][0]["active"])

    def test_build_batch_skips_deleted_project_directories(self):
        from aetheris.project_identity_sync import ProjectIdentityKeys
        from aetheris.project_registry import ProjectRegistry
        from aetheris.project_sync import ProjectSync

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            missing = root / "deleted"
            available = root / "available"; (available / ".git").mkdir(parents=True)
            config = root / "config" / "aetheris.json"; config.parent.mkdir()
            config.write_text(json.dumps({
                "project_revision": 1,
                "project_roots": [{"path": str(missing), "vcs": "git"}, {"path": str(available), "vcs": "git"}],
                "authorized_roots": [str(missing), str(available)],
            }), encoding="utf-8")

            batch = ProjectSync.build_batch(ProjectRegistry(config).load(), ProjectIdentityKeys(b"t" * 32, b"r" * 32, 1))

            self.assertEqual(len(batch["projects"]), 1)
            self.assertEqual(batch["projects"][0]["display_name"], "available")


if __name__ == "__main__":
    unittest.main()
