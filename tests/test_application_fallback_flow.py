import tempfile
import unittest
from concurrent.futures import Future
from datetime import datetime, timedelta, timezone
from pathlib import Path


class ImmediateExecutor:
    def submit(self, function, *args):
        future = Future()
        try: future.set_result(function(*args))
        except Exception as exc: future.set_exception(exc)
        return future


class Health:
    def __init__(self): self.snapshots = []
    def finish(self, adapter_id, state, **values): self.snapshots.append((adapter_id, state, values))


class ApplicationFallbackFlowTests(unittest.TestCase):
    def core(self, root: Path):
        from aetheris.adapters.application_ocr import ApplicationCaptureResult
        from aetheris.adapters.application_window import ApplicationWindow
        from aetheris.application_capture_policy import ApplicationCapturePolicy
        from aetheris.core import ProjectResolver
        from aetheris.process_consent import ConsentOutcome, StableProcessIdentity
        from aetheris.queue import LocalQueue
        from aetheris.tray import ProjectSource, TrayConfig, TrayCore

        project = root / "repo"; project.mkdir(); (project / ".git").mkdir()
        core = TrayCore.__new__(TrayCore)
        core.config = TrayConfig("http://server", root / "token", project, root / "queue.db", authorized_roots=[project], project_roots=[ProjectSource(project, "git")], tenant_id="tenant-1", subject_id="subject-1", device_id="device-1")
        core.queue = LocalQueue(root / "queue.db")
        core.application_capture_policy = ApplicationCapturePolicy()
        core.application_capture_policy.mark_native_event("identity-1", datetime(2026, 9, 15, 8, 0, tzinfo=timezone.utc))
        window = ApplicationWindow(42, 7, "designer.exe", "配置页面", True, False, False, (0, 0, 800, 600))
        core.application_window_inspector = type("Inspector", (), {"inspect": lambda self: window})()
        identity = StableProcessIdentity("designer.exe", str(root / "Designer.exe"), "Vendor", "sha256:1", "valid")
        core.process_identity_provider = type("Identity", (), {"inspect": lambda self, pid, name: identity})()
        core.process_consent = type("Consent", (), {"observe": lambda self, value: ConsentOutcome("allow_global", False, "identity-1")})()
        core.application_ocr = type("OCR", (), {"collect": lambda self, value: ApplicationCaptureResult({"event_type": "application.activity", "payload": {"application_name": "designer.exe", "window_context": "配置页面", "visible_text": "已脱敏文本", "capture_method": "ocr_fallback", "ocr_languages": ["eng", "chi_sim"], "confidence": "low"}, "redaction_report": {"rules": [], "replacement_count": 0}}, "captured", {"parsed": 1})})()
        core.application_executor = ImmediateExecutor()
        core._application_future = None
        core._application_future_identity = ""
        core._application_fallback_counts = {"eligible": 0, "captured": 0, "parsed": 0, "emitted": 0, "skipped": 0, "failed": 0}
        core.adapter_health = Health()
        core.project_resolver = ProjectResolver()
        return core, project

    def test_authorized_foreground_process_falls_back_after_sixty_seconds(self):
        from aetheris.core import ProcessObservation

        with tempfile.TemporaryDirectory() as raw:
            core, project = self.core(Path(raw))
            observation = ProcessObservation(7, "designer.exe", None, None)
            try:
                self.assertEqual(core.capture_application_fallback([observation], set(), datetime(2026, 9, 15, 8, 0, 59, tzinfo=timezone.utc)), 0)
                self.assertEqual(core.capture_application_fallback([observation], set(), datetime(2026, 9, 15, 8, 1, tzinfo=timezone.utc)), 1)
                event = core.queue.history()[0]
                self.assertEqual(event["event_type"], "application.activity")
                self.assertEqual(event["project_id"], core.project_resolver.resolve(project, [project]).project_id)
                self.assertNotIn(str(project), str(event))
            finally:
                core.queue.close()

    def test_native_event_resets_fallback_timer(self):
        from aetheris.core import ProcessObservation

        with tempfile.TemporaryDirectory() as raw:
            core, _ = self.core(Path(raw))
            observation = ProcessObservation(7, "designer.exe", None, None)
            try:
                count = core.capture_application_fallback([observation], {"identity-1"}, datetime(2026, 9, 15, 8, 1, tzinfo=timezone.utc))
                self.assertEqual(count, 0)
                self.assertEqual(core.queue.history(), [])
            finally:
                core.queue.close()

    def test_unapproved_process_never_submits_ocr(self):
        from aetheris.core import ProcessObservation
        from aetheris.process_consent import ConsentOutcome

        with tempfile.TemporaryDirectory() as raw:
            core, _ = self.core(Path(raw))
            core.process_consent = type("Consent", (), {"observe": lambda self, value: ConsentOutcome("pending", False, "identity-1")})()
            try:
                self.assertEqual(core.capture_application_fallback([ProcessObservation(7, "designer.exe", None, None)], set(), datetime(2026, 9, 15, 8, 2, tzinfo=timezone.utc)), 0)
                self.assertIsNone(core._application_future)
            finally:
                core.queue.close()


if __name__ == "__main__":
    unittest.main()
