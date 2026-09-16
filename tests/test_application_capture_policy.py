import unittest
from datetime import datetime, timedelta, timezone


def at(seconds: int) -> datetime:
    return datetime(2026, 9, 15, tzinfo=timezone.utc) + timedelta(seconds=seconds)


class ApplicationCapturePolicyTests(unittest.TestCase):
    def candidate(self, **changes):
        from aetheris.application_capture_policy import CaptureCandidate

        values = {
            "identity_key": "process-1", "process_name": "designer.exe", "consent_state": "allow_global",
            "project_id": "project-1", "foreground": True, "visible": True, "minimized": False,
            "has_password_control": False, "has_client_area": True, "queue_watermark": "normal",
            "ocr_busy": False, "now": at(60),
        }
        return CaptureCandidate(**{**values, **changes})

    def test_candidate_requires_sixty_seconds_without_native_event(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()
        policy.mark_native_event("process-1", at(0))

        self.assertEqual(policy.evaluate(self.candidate(now=at(59))).reason_code, "native_adapter_recent")
        self.assertTrue(policy.evaluate(self.candidate(now=at(60))).eligible)

    def test_newly_seen_process_waits_sixty_seconds_before_first_capture(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()

        self.assertEqual(policy.evaluate(self.candidate(now=at(0))).reason_code, "native_adapter_recent")
        self.assertEqual(policy.evaluate(self.candidate(now=at(59))).reason_code, "native_adapter_recent")
        self.assertTrue(policy.evaluate(self.candidate(now=at(60))).eligible)

    def test_privacy_gate_blocks_unapproved_browser_password_and_hidden_windows(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()
        for candidate in (
            self.candidate(consent_state="pending"),
            self.candidate(process_name="chrome.exe"),
            self.candidate(has_password_control=True),
            self.candidate(foreground=False),
            self.candidate(visible=False),
            self.candidate(minimized=True),
            self.candidate(has_client_area=False),
        ):
            self.assertEqual(policy.evaluate(candidate).reason_code, "privacy_gate_blocked")

    def test_queue_and_busy_worker_skip_without_starting_capture(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()
        self.assertEqual(policy.evaluate(self.candidate(queue_watermark="pause")).reason_code, "queue_paused")
        self.assertEqual(policy.evaluate(self.candidate(ocr_busy=True)).reason_code, "ocr_busy")

    def test_device_and_process_rate_limits_are_enforced(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()
        policy.mark_result("process-1", "success", at(60))

        self.assertEqual(policy.evaluate(self.candidate(now=at(119))).reason_code, "device_rate_limited")
        self.assertEqual(policy.evaluate(self.candidate(now=at(120))).reason_code, "process_rate_limited")
        self.assertTrue(policy.evaluate(self.candidate(now=at(360))).eligible)

    def test_failure_backoff_is_bounded_and_success_resets_it(self):
        from aetheris.application_capture_policy import ApplicationCapturePolicy

        policy = ApplicationCapturePolicy()
        expected_delays = (60, 120, 300, 600, 600)
        current = at(60)
        for delay in expected_delays:
            policy.mark_result("process-1", "failure", current)
            self.assertEqual(policy.evaluate(self.candidate(now=current + timedelta(seconds=delay - 1))).reason_code, "failure_backoff")
            current += timedelta(seconds=delay)
        policy.mark_result("process-1", "success", current)
        policy.mark_result("process-1", "failure", current + timedelta(seconds=300))
        self.assertEqual(policy.evaluate(self.candidate(now=current + timedelta(seconds=359))).reason_code, "failure_backoff")
        self.assertNotEqual(policy.evaluate(self.candidate(now=current + timedelta(seconds=360))).reason_code, "failure_backoff")


if __name__ == "__main__":
    unittest.main()
