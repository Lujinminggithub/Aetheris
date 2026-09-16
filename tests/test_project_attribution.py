import tempfile
import unittest
from pathlib import Path


class ProjectAttributionTests(unittest.TestCase):
    def test_authorized_cwd_uses_repository_leaf(self):
        from aetheris.project_attribution import attribute_ai_project

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "service-api"
            (project / ".git").mkdir(parents=True)

            result = attribute_ai_project("codex", project / "src", [root], namespace="tenant-1")

            self.assertEqual(result.project_label, "service-api")
            self.assertEqual(result.attribution, "authorized_root")
            self.assertEqual(result.project_root, project.resolve())

    def test_unauthorized_cwd_uses_tenant_scoped_codex_project(self):
        from aetheris.project_attribution import attribute_ai_project

        with tempfile.TemporaryDirectory() as raw:
            outside = Path(raw) / "private" / "temporary-workspace"
            outside.mkdir(parents=True)

            result = attribute_ai_project("codex", outside, [], namespace="tenant-1")
            other_tenant = attribute_ai_project("codex", outside, [], namespace="tenant-2")

            self.assertEqual(result.project_label, "Codex")
            self.assertEqual(result.attribution, "tool_fallback")
            self.assertIsNone(result.project_root)
            self.assertNotEqual(result.project_id, other_tenant.project_id)

    def test_known_ai_tool_labels_are_stable(self):
        from aetheris.project_attribution import attribute_ai_project

        cases = {
            "codex": "Codex",
            "claude_code": "Claude Code",
            "cursor": "Cursor",
            "github_copilot": "GitHub Copilot",
        }
        for tool, label in cases.items():
            with self.subTest(tool=tool):
                self.assertEqual(attribute_ai_project(tool, None, [], namespace="tenant-1").project_label, label)


if __name__ == "__main__":
    unittest.main()
