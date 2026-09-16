import os
import subprocess
import tempfile
import unittest
from pathlib import Path


class GitAdapterTests(unittest.TestCase):
    def test_collects_commit_metadata_and_diff_stat_without_patch_body(self):
        from aetheris.adapters.git import GitAdapter

        with tempfile.TemporaryDirectory() as raw:
            repo = Path(raw)
            env = os.environ.copy()
            env.update({"GIT_AUTHOR_NAME": "Aetheris Test", "GIT_AUTHOR_EMAIL": "test@example.invalid", "GIT_COMMITTER_NAME": "Aetheris Test", "GIT_COMMITTER_EMAIL": "test@example.invalid"})
            subprocess.run(["git", "init", "-q"], cwd=repo, check=True, env=env)
            subprocess.run(["git", "config", "user.name", "Aetheris Test"], cwd=repo, check=True, env=env)
            subprocess.run(["git", "config", "user.email", "test@example.invalid"], cwd=repo, check=True, env=env)
            (repo / "app.py").write_text("print('one')\n", encoding="utf-8")
            subprocess.run(["git", "add", "app.py"], cwd=repo, check=True, env=env)
            subprocess.run(["git", "commit", "-q", "-m", "initial"], cwd=repo, check=True, env=env)
            (repo / "app.py").write_text("print('two')\n", encoding="utf-8")

            records = GitAdapter().collect(repo)

            self.assertEqual([record["event_type"] for record in records], ["git.commit", "git.diff"])
            self.assertIn("subject", records[0]["payload"])
            self.assertEqual(records[1]["payload"]["files_changed"], 1)
            self.assertNotIn("print('two')", str(records))


if __name__ == "__main__":
    unittest.main()
