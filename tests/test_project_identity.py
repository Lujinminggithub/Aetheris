import hashlib
import hmac
import tempfile
import unittest
from pathlib import Path


class ProjectIdentityTests(unittest.TestCase):
    def test_https_and_scp_style_ssh_remote_share_canonical_identity(self):
        from aetheris.project_identity import normalize_git_remote

        https = normalize_git_remote("https://user:secret@Example.COM/team/repo.git")
        ssh = normalize_git_remote("git@example.com:team/repo.git")

        self.assertEqual(https, "example.com/team/repo")
        self.assertEqual(ssh, "example.com/team/repo")
        self.assertNotIn("user", https)
        self.assertNotIn("secret", https)

    def test_remote_fingerprint_is_shared_by_tenant_key_but_root_is_device_scoped(self):
        from aetheris.project_identity import ProjectIdentityHasher

        tenant_key = b"t" * 32
        first = ProjectIdentityHasher(tenant_key, b"a" * 32, key_version=3)
        second = ProjectIdentityHasher(tenant_key, b"b" * 32, key_version=3)

        self.assertEqual(
            first.remote_fingerprint("https://example.com/team/repo.git"),
            second.remote_fingerprint("git@example.com:team/repo.git"),
        )
        self.assertNotEqual(first.root_fingerprint(Path("C:/code/repo")), second.root_fingerprint(Path("C:/code/repo")))
        self.assertTrue(first.remote_fingerprint("https://example.com/team/repo.git").startswith("hmac-sha256:v3:"))

    def test_project_identity_reads_origin_without_exposing_remote_or_absolute_root(self):
        from aetheris.project_identity import inspect_project_identity

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "repo"
            git_dir = project / ".git"
            git_dir.mkdir(parents=True)
            (git_dir / "config").write_text(
                "[remote \"origin\"]\n\turl = https://user:secret@example.com/team/repo.git\n",
                encoding="utf-8",
            )

            identity = inspect_project_identity(
                project,
                remote_key=b"t" * 32,
                root_key=b"r" * 32,
                key_version=1,
            )

            expected_project_id = "project-" + hashlib.sha256(str(project.resolve()).casefold().encode("utf-8")).hexdigest()[:16]
            expected_remote = hmac.new(b"t" * 32, b"example.com/team/repo", hashlib.sha256).hexdigest()
            self.assertEqual(identity.local_project_id, expected_project_id)
            self.assertEqual(identity.display_name, "repo")
            self.assertEqual(identity.vcs, "git")
            self.assertEqual(identity.workspace_kind, "primary")
            self.assertEqual(identity.remote_fingerprint, f"hmac-sha256:v1:{expected_remote}")
            serialized = identity.to_registration()
            self.assertNotIn(str(project.resolve()), str(serialized))
            self.assertNotIn("https://", str(serialized))
            self.assertNotIn("secret", str(serialized))

    def test_git_file_is_classified_as_worktree_and_uses_common_config(self):
        from aetheris.project_identity import inspect_project_identity

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            common = root / "main" / ".git"
            worktree_admin = common / "worktrees" / "feature"
            worktree_admin.mkdir(parents=True)
            (common / "config").write_text(
                "[remote \"origin\"]\n\turl = ssh://git@example.com/team/repo.git\n",
                encoding="utf-8",
            )
            (worktree_admin / "commondir").write_text("../..\n", encoding="utf-8")
            worktree = root / "feature"
            worktree.mkdir()
            (worktree / ".git").write_text(f"gitdir: {worktree_admin}\n", encoding="utf-8")

            identity = inspect_project_identity(worktree, remote_key=b"t" * 32, root_key=b"r" * 32, key_version=2)

            self.assertEqual(identity.workspace_kind, "worktree")
            self.assertEqual(identity.worktree_name, "feature")
            self.assertIsNotNone(identity.remote_fingerprint)


if __name__ == "__main__":
    unittest.main()
