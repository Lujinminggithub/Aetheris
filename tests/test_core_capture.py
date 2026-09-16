import tempfile
import unittest
from pathlib import Path


class CoreCaptureTests(unittest.TestCase):
    def test_allowed_process_and_authorized_git_root_create_event(self):
        from aetheris.core import ConsentRegistry, ProcessObservation, capture_observation

        with tempfile.TemporaryDirectory() as raw:
            tmp_path = Path(raw)
            (tmp_path / ".git").mkdir()
            registry = ConsentRegistry(allowed_names={"code.exe"})
            observation = ProcessObservation(42, "Code.exe", str(tmp_path / "Code.exe"), None)
            event = capture_observation(
                observation,
                project_path=tmp_path,
                consent_registry=registry,
                authorized_roots=[tmp_path],
            )
            self.assertIsNotNone(event)
            self.assertEqual(event.event_type, "process.observed")
            self.assertTrue(event.project_id.startswith("project-"))

    def test_unknown_process_and_unapproved_root_do_not_create_event(self):
        from aetheris.core import ConsentRegistry, ProcessObservation, capture_observation

        with tempfile.TemporaryDirectory() as raw:
            tmp_path = Path(raw)
            observation = ProcessObservation(7, "mystery.exe", None, None)
            registry = ConsentRegistry(allowed_names={"code.exe"})
            self.assertIsNone(
                capture_observation(
                    observation,
                    project_path=tmp_path,
                    consent_registry=registry,
                    authorized_roots=[],
                )
            )

    def test_known_ai_and_editor_processes_are_allowed_when_granted(self):
        from aetheris.core import ConsentRegistry, ProcessObservation

        registry = ConsentRegistry(allowed_names={"codex.exe", "cursor.exe", "claude.exe", "code.exe"})
        for name in ("codex.exe", "cursor.exe", "claude.exe", "Code.exe"):
            self.assertEqual(registry.classify(ProcessObservation(1, name, None, None)).status, "allowed")

    def test_explicitly_authorized_unmarked_directory_is_pending_but_resolvable(self):
        from aetheris.core import ProjectResolver

        with tempfile.TemporaryDirectory() as raw:
            project = ProjectResolver().resolve(Path(raw), [Path(raw)])
            self.assertIsNotNone(project)
            self.assertEqual(project.classification, "pending")


if __name__ == "__main__":
    unittest.main()
