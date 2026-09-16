import tempfile
import unittest
from pathlib import Path


class ProjectScanTests(unittest.TestCase):
    def test_discovers_git_directory_git_file_and_svn(self):
        from aetheris.projects import discover_projects

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "git-dir" / ".git").mkdir(parents=True)
            (root / "git-file").mkdir()
            (root / "git-file" / ".git").write_text("gitdir: elsewhere", encoding="utf-8")
            (root / "svn" / ".svn").mkdir(parents=True)
            result = discover_projects(root)
            self.assertEqual([(item.path.name, item.vcs) for item in result.projects], [("git-dir", "git"), ("git-file", "git"), ("svn", "svn")])

    def test_skips_heavy_directories_and_limits_results(self):
        from aetheris.projects import discover_projects

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "node_modules" / "ignored" / ".git").mkdir(parents=True)
            for index in range(4):
                (root / f"repo-{index}" / ".git").mkdir(parents=True)
            result = discover_projects(root, limit=2)
            self.assertEqual(len(result.projects), 2)
            self.assertTrue(result.truncated)
            self.assertNotIn("ignored", [item.path.name for item in result.projects])

    def test_respects_maximum_depth(self):
        from aetheris.projects import discover_projects

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            (root / "one" / "two" / ".git").mkdir(parents=True)
            self.assertEqual(discover_projects(root, max_depth=1).projects, [])


if __name__ == "__main__":
    unittest.main()

