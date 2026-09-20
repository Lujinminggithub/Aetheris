import json
import os
import tempfile
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest.mock import patch


class TrayTests(unittest.TestCase):

    def test_service_ready_precedes_optional_integration_install(self):
        from aetheris.tray import _announce_ready_and_start_optional

        calls = []
        service = type("Service", (), {"ready": lambda self: calls.append("ready")})()
        app = type("App", (), {"start_optional_integrations": lambda self: calls.append("optional")})()

        _announce_ready_and_start_optional(app, service)

        self.assertEqual(calls, ["ready", "optional"])

    def test_core_applies_application_capture_policy_and_persists_revision(self):
        from aetheris.tray import TrayConfig, TrayCore

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config_path = root / "aetheris.json"
            config_path.write_text(json.dumps({"application_capture_enabled": True, "application_capture_policy_revision": 0}), encoding="utf-8")
            core = TrayCore.__new__(TrayCore)
            core.config = TrayConfig("http://server", root / "token", root, root / "queue.db", config_path=config_path)

            self.assertTrue(core.apply_application_capture_policy({"enabled": False, "revision": 3}))
            self.assertFalse(core.config.application_capture_enabled)
            self.assertEqual(core.config.application_capture_policy_revision, 3)
            persisted = json.loads(config_path.read_text(encoding="utf-8"))
            self.assertEqual(persisted["application_capture_policy_revision"], 3)
            self.assertFalse(persisted["application_capture_enabled"])
    def test_server_event_count_decrease_resets_history_replay_state(self):
        from aetheris.tray import TrayCore

        class Adapter:
            def __init__(self): self.reset_calls = 0
            def reset_history(self): self.reset_calls += 1

        with tempfile.TemporaryDirectory() as raw:
            core = TrayCore.__new__(TrayCore)
            core._server_sync_path = Path(raw) / "server-sync.json"
            core.ai_adapters = [Adapter(), Adapter()]
            core._seen_records = {"old-fingerprint"}
            core._log = lambda _message: None
            core._server_sync_path.write_text(json.dumps({"server_event_count": 500, "data_generation": "500:old"}), encoding="utf-8")

            core._sync_server_history({"server_event_count": 12, "data_generation": "12:new"})

            self.assertEqual([adapter.reset_calls for adapter in core.ai_adapters], [1, 1])
            self.assertEqual(core._seen_records, set())

    def test_project_registration_updates_independent_safe_status(self):
        from aetheris.project_sync import ProjectSyncResult
        from aetheris.tray import TrayCore

        class Sync:
            def __init__(self, error=None):
                self.error = error

            def sync(self, _keys):
                if self.error:
                    raise self.error
                return ProjectSyncResult(7, [{"local_project_id": "project-one"}])

        class Status:
            def update_project_sync(self, *args, **values):
                self.args = args
                self.values = values

        core = TrayCore.__new__(TrayCore)
        core.project_sync = Sync()
        core.project_identity_keys = object()
        core.status = Status()
        core._log = lambda _message: None

        self.assertTrue(core.sync_projects())
        self.assertEqual(core.status.args, ("registered",))
        self.assertEqual(core.status.values, {"registry_revision": 7, "project_count": 1})

        core.project_sync = Sync(OSError("private server detail"))
        self.assertFalse(core.sync_projects())
        self.assertEqual(core.status.args, ("error",))
        self.assertEqual(core.status.values["error"], "OSError")

    def test_project_identity_sync_updates_safe_status_without_exposing_failure_message(self):
        from aetheris.project_identity_sync import ProjectIdentityKeys
        from aetheris.tray import TrayCore

        class Sync:
            def __init__(self, result=None, error=None):
                self.result = result
                self.error = error

            def sync(self):
                if self.error:
                    raise self.error
                return self.result

        class Status:
            def update_project_identity(self, *args, **values):
                self.args = args
                self.values = values

        core = TrayCore.__new__(TrayCore)
        core.project_identity_key_sync = Sync(ProjectIdentityKeys(b"t" * 32, b"r" * 32, 3))
        core.status = Status()
        core._log = lambda _message: None

        self.assertTrue(core.sync_project_identity_keys())
        self.assertEqual(core.project_identity_keys.key_version, 3)
        self.assertEqual(core.status.args, ("available",))
        self.assertEqual(core.status.values["key_version"], 3)

        core.project_identity_key_sync = Sync(error=OSError("private network detail"))
        self.assertFalse(core.sync_project_identity_keys())
        self.assertEqual(core.status.args, ("error",))
        self.assertEqual(core.status.values["error"], "OSError")

    def test_core_applies_new_browser_policy_and_persists_revision(self):
        from aetheris.tray import TrayConfig, TrayCore

        class Status:
            def update_browser_policy(self, **values): self.values = values

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config_path = root / "aetheris.json"
            config_path.write_text(json.dumps({"browser_allowlist": [], "browser_policy_revision": 0}), encoding="utf-8")
            core = TrayCore.__new__(TrayCore)
            core.config = TrayConfig("http://server", root / "token", root, root / "queue.db", config_path=config_path)
            core.status = Status()
            core.browser_ocr = None

            changed = core.apply_browser_policy({"enabled": True, "allowed_domains": ["docs.example.com"], "revision": 4})

            self.assertTrue(changed)
            self.assertEqual(core.config.browser_allowlist, {"docs.example.com"})
            self.assertEqual(core.config.browser_policy_revision, 4)
            self.assertIsNotNone(core.browser_ocr)
            persisted = json.loads(config_path.read_text(encoding="utf-8"))
            self.assertEqual(persisted["browser_policy_revision"], 4)
            self.assertEqual(persisted["browser_allowlist"], ["docs.example.com"])
            self.assertEqual(core.status.values["state"], "enabled")

    def test_browser_policy_sync_failure_keeps_last_valid_policy(self):
        from aetheris.tray import TrayConfig, TrayCore

        class Client:
            def browser_policy(self): raise OSError("offline")
        class Status:
            def update_browser_policy(self, **values): self.values = values

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            core = TrayCore.__new__(TrayCore)
            core.config = TrayConfig("http://server", root / "token", root, root / "queue.db", browser_allowlist={"docs.example.com"}, browser_policy_revision=2)
            core.client = Client(); core.status = Status(); core.browser_ocr = object(); core._log = lambda _: None

            self.assertFalse(core.sync_browser_policy())
            self.assertEqual(core.config.browser_allowlist, {"docs.example.com"})
            self.assertEqual(core.config.browser_policy_revision, 2)
            self.assertIn("offline", core.status.values["error"])

    def test_core_ignores_stale_browser_policy_revision(self):
        from aetheris.tray import TrayConfig, TrayCore

        class Status:
            def update_browser_policy(self, **values): self.values = values

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            core = TrayCore.__new__(TrayCore)
            core.config = TrayConfig("http://server", root / "token", root, root / "queue.db", browser_allowlist={"docs.example.com"}, browser_policy_revision=4)
            core.status = Status(); core.browser_ocr = object()

            self.assertFalse(core.apply_browser_policy({"enabled": True, "allowed_domains": ["old.example.com"], "revision": 3}))
            self.assertEqual(core.config.browser_allowlist, {"docs.example.com"})
            self.assertEqual(core.config.browser_policy_revision, 4)

    def test_browser_record_fingerprint_repeats_once_per_five_minute_window(self):
        from aetheris.tray import record_fingerprint

        record = {"event_type": "browser.page_view", "payload": {"domain": "docs.example.com"}}
        first = record_fingerprint(record, "core.browser", Path("repo"), datetime(2026, 9, 8, 9, 1, tzinfo=timezone.utc))
        same_window = record_fingerprint(record, "core.browser", Path("repo"), datetime(2026, 9, 8, 9, 4, tzinfo=timezone.utc))
        next_window = record_fingerprint(record, "core.browser", Path("repo"), datetime(2026, 9, 8, 9, 5, tzinfo=timezone.utc))

        self.assertEqual(first, same_window)
        self.assertNotEqual(first, next_window)

    def test_upload_events_splits_backlog_into_bounded_batches(self):
        from aetheris.events import AetherisEvent
        from aetheris.queue import LocalQueue
        from aetheris.tray import TrayCore

        class Client:
            def __init__(self):
                self.batch_sizes = []

            def send(self, events):
                self.batch_sizes.append(len(events))
                return [{"event_id": event.event_id, "status": "accepted"} for event in events]

        class Status:
            def update_queue(self, **_):
                pass

        with tempfile.TemporaryDirectory() as raw:
            core = TrayCore.__new__(TrayCore)
            core.queue = LocalQueue(Path(raw) / "queue.db")
            core.client = Client()
            core.status = Status()
            events = [AetherisEvent.create(
                "ai.message", tenant_id="tenant-1", subject_id="subject-1", device_id="device-1",
                project_id="project-1", session_id="session-1", source="core.ai.codex", source_version="0.4.6",
                payload={"role": "user", "content": str(index)}, redaction_report={}, processing_grants=["server_ingest"],
                event_id=f"event-{index}", correlation_id=f"event-{index}",
            ) for index in range(205)]

            try:
                accepted = core._upload_events(events)

                self.assertEqual(accepted, 205)
                self.assertEqual(core.client.batch_sizes, [100, 100, 5])
                self.assertEqual(core.queue.stats()["queued_count"], 0)
            finally:
                core.queue.close()

    def test_native_setup_config_auto_adds_detected_ai_sources(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"; (project / ".git").mkdir(parents=True)
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            codex = root / "codex-sessions"; codex.mkdir()
            config_path = root / "aetheris.json"
            config_path.write_text(json.dumps({
                "gateway_url": "http://server", "credential_file": str(credential),
                "project_root": str(project), "project_roots": [{"path": str(project), "vcs": "git"}],
                "authorized_roots": [str(project)], "queue": str(root / "client.db"),
            }), encoding="utf-8")

            with patch("aetheris.tray.default_ai_session_roots", return_value=[("codex", codex)]):
                config = TrayConfig.from_file(config_path)

            self.assertEqual(config.ai_session_roots, [("codex", codex.resolve())])
            persisted = json.loads(config_path.read_text(encoding="utf-8"))
            self.assertEqual(persisted["ai_session_roots"], [{"tool": "codex", "path": str(codex.resolve())}])

    def test_existing_config_adds_newly_detected_ai_source(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            codex = root / "codex-sessions"; codex.mkdir()
            claude = root / "claude"; claude.mkdir()
            config_path = root / "aetheris.json"
            config_path.write_text(json.dumps({
                "gateway_url": "http://server", "credential_file": str(credential),
                "queue": str(root / "client.db"),
                "ai_session_roots": [{"tool": "codex", "path": str(codex)}],
            }), encoding="utf-8")

            with patch("aetheris.tray.default_ai_session_roots", return_value=[("codex", codex), ("claude_code", claude)]):
                config = TrayConfig.from_file(config_path)

            self.assertEqual(config.ai_session_roots, [("codex", codex.resolve()), ("claude_code", claude.resolve())])
            self.assertEqual(json.loads(config_path.read_text(encoding="utf-8"))["core_version"], "0.4.16")

    def test_existing_config_auto_enables_real_vscode_state_database(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            appdata = root / "AppData" / "Roaming"
            database = appdata / "Code" / "User" / "globalStorage" / "state.vscdb"
            database.parent.mkdir(parents=True)
            database.write_bytes(b"SQLite format 3\0")
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            config_path = root / "aetheris.json"
            config_path.write_text(json.dumps({
                "gateway_url": "http://server", "credential_file": str(credential), "queue": str(root / "client.db"),
            }), encoding="utf-8")

            with patch.dict(os.environ, {"APPDATA": str(appdata)}):
                config = TrayConfig.from_file(config_path)

            self.assertEqual(config.vscode_state_path, database.resolve())

    def test_tray_config_loads_multiple_project_roots_and_credential_file(self):
        from aetheris.tray import TrayConfig
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw); git = root / "git"; svn = root / "svn"
            (git / ".git").mkdir(parents=True); (svn / ".svn").mkdir(parents=True)
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            path = root / "aetheris.json"
            path.write_text(json.dumps({"gateway_url":"http://server","credential_file":str(credential),"credential_kind":"device_token","project_root":str(git),"project_roots":[{"path":str(git),"vcs":"git"},{"path":str(svn),"vcs":"svn"}],"queue":str(root/"client.db")}), encoding="utf-8")
            config = TrayConfig.from_file(path)
            self.assertEqual([(item.path, item.vcs) for item in config.project_roots], [(git.resolve(), "git"), (svn.resolve(), "svn")])
            self.assertEqual(config.credential_file, credential.resolve())

    def test_tray_config_allows_zero_projects(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            control = root / "control.credential"; control.write_bytes(b"encrypted-control")
            path = root / "aetheris.json"
            path.write_text(json.dumps({
                "gateway_url": "http://server",
                "credential_file": str(credential),
                "control_credential_file": str(control),
                "queue": str(root / "client.db"),
                "project_revision": 0,
                "project_roots": [],
                "authorized_roots": [],
            }), encoding="utf-8")

            config = TrayConfig.from_file(path)

            self.assertIsNone(config.project_root)
            self.assertEqual(config.project_roots, [])
            self.assertEqual(config.authorized_roots, [])
            self.assertEqual(config.control_credential_file, control.resolve())

    def test_deleted_primary_project_uses_next_available_registered_project(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            deleted = root / "deleted"
            available = root / "available"; (available / ".git").mkdir(parents=True)
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            path = root / "aetheris.json"
            path.write_text(json.dumps({
                "gateway_url": "http://server", "credential_file": str(credential), "queue": str(root / "client.db"),
                "project_root": str(deleted),
                "project_roots": [{"path": str(deleted), "vcs": "git"}, {"path": str(available), "vcs": "git"}],
                "authorized_roots": [str(deleted), str(available)],
            }), encoding="utf-8")

            config = TrayConfig.from_file(path)

            self.assertEqual(config.project_root, available.resolve())
            self.assertEqual([(item.path, item.vcs) for item in config.project_roots], [(available.resolve(), "git")])
            self.assertEqual(config.authorized_roots, [available.resolve()])
            stored = json.loads(path.read_text(encoding="utf-8"))
            self.assertEqual(stored["project_root"], str(available.resolve()))
            self.assertEqual(len(stored["project_roots"]), 2)

    def test_all_deleted_projects_start_in_waiting_state_without_losing_registry(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            deleted = root / "deleted"
            credential = root / "device.credential"; credential.write_bytes(b"encrypted")
            path = root / "aetheris.json"
            path.write_text(json.dumps({
                "gateway_url": "http://server", "credential_file": str(credential), "queue": str(root / "client.db"),
                "project_root": str(deleted), "project_roots": [{"path": str(deleted), "vcs": "git"}],
                "authorized_roots": [str(deleted)],
            }), encoding="utf-8")

            config = TrayConfig.from_file(path)

            self.assertIsNone(config.project_root)
            self.assertEqual(config.project_roots, [])
            self.assertEqual(config.authorized_roots, [])
            stored = json.loads(path.read_text(encoding="utf-8"))
            self.assertNotIn("project_root", stored)
            self.assertEqual(stored["project_roots"], [{"path": str(deleted), "vcs": "git"}])

    def test_running_core_hot_reloads_added_project(self):
        from aetheris.project_registry import ProjectRegistry
        from aetheris.tray import TrayConfig, TrayCore

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"; (project / ".git").mkdir(parents=True)
            config_path = root / "config" / "aetheris.json"; config_path.parent.mkdir()
            config_path.write_text(json.dumps({"project_revision": 0, "project_roots": [], "authorized_roots": []}), encoding="utf-8")
            config = TrayConfig("http://server", root / "token", None, root / "client.db", config_path=config_path)
            core = TrayCore.__new__(TrayCore)
            core.config = config
            core.project_registry = ProjectRegistry(config_path)
            core.visual_studio_adapters = []

            core.project_registry.mutate(0, "add", project)

            self.assertTrue(core.reload_projects_if_changed())
            self.assertEqual(core.config.project_revision, 1)
            self.assertEqual(core.config.project_root, project.resolve())
            self.assertEqual([(item.path, item.vcs) for item in core.config.project_roots], [(project.resolve(), "git")])

    def test_tray_config_migrates_legacy_windows_paths_with_single_backslashes(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw)
            (project / ".git").mkdir()
            path = project / "aetheris.json"
            path.write_text(
                '{"gateway_url":"http://127.0.0.1:8080","token_file":"C:\\Users\\dev\\gateway.env",'
                f'"project_root":"{str(project).replace(chr(92), chr(92) * 1)}","queue":"C:\\Users\\dev\\client.db"}}',
                encoding="utf-8",
            )
            config = TrayConfig.from_file(path)
            self.assertEqual(config.gateway_url, "http://127.0.0.1:8080")
            self.assertEqual(config.project_root, project.resolve())
            self.assertTrue(config.device_id.startswith("device-"))

    def test_default_config_path_is_user_local_and_config_data_is_complete(self):
        from aetheris.tray import default_config_path, build_config_data

        with tempfile.TemporaryDirectory() as raw:
            config_path = default_config_path(Path(raw))
            data = build_config_data(
                gateway_url="http://127.0.0.1:8080",
                token_file=Path(raw) / "gateway.env",
                project_root=Path(raw),
                queue=Path(raw) / "client.db",
            )
            self.assertEqual(config_path, Path(raw) / "config" / "aetheris.json")
            self.assertEqual(data["project_root"], str(Path(raw).resolve()))
            self.assertEqual(data["executable"], "AetherisCore.exe")

    def test_build_config_embeds_gateway_without_setup_prompt(self):
        from aetheris.tray import build_config_data, DEFAULT_GATEWAY_URL

        with tempfile.TemporaryDirectory() as raw:
            data = build_config_data(gateway_url=DEFAULT_GATEWAY_URL, token_file=Path(raw) / "token", project_root=Path(raw), queue=Path(raw) / "q.db")
            self.assertEqual(data["gateway_url"], DEFAULT_GATEWAY_URL)
            setup_text = Path("src/aetheris/tray.py").read_text(encoding="utf-8")
            self.assertNotIn('askstring(\n            "Aetheris Core setup",\n            "Gateway URL:', setup_text)

    def test_frozen_client_defaults_config_next_to_executable(self):
        import sys
        from aetheris.tray import default_config_path

        old_frozen = getattr(sys, "frozen", None)
        old_executable = sys.executable
        try:
            sys.frozen = True
            sys.executable = r"D:\Aetheris\AetherisCore.exe"
            self.assertEqual(default_config_path(), Path(r"D:\Aetheris\config\aetheris.json"))
        finally:
            sys.executable = old_executable
            if old_frozen is None:
                delattr(sys, "frozen")
            else:
                sys.frozen = old_frozen

    def test_record_conversion_creates_project_attributed_event(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw)
            (project / ".git").mkdir()
            config = TrayConfig(
                gateway_url="http://127.0.0.1:8080",
                token_file=project / "gateway.env",
                project_root=project,
                queue=project / "client.db",
                authorized_roots=[project],
            )
            event = event_from_record(
                {"event_type": "terminal.command", "payload": {"command": "git status"}},
                config,
                "core.terminal",
            )
            self.assertEqual(event.event_type, "terminal.command")
            self.assertTrue(event.project_id.startswith("project-"))

    def test_record_conversion_uses_enrolled_tenant_and_subject(self):
        from aetheris.tray import TrayConfig, event_from_record
        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw); (project / ".git").mkdir()
            config = TrayConfig("http://server", project / "token", project, project / "queue.db", tenant_id="tenant-enrolled", subject_id="subject-enrolled", authorized_roots=[project])
            event = event_from_record({"event_type":"git.diff","payload":{}}, config, "core.git")
            self.assertEqual((event.tenant_id, event.subject_id), ("tenant-enrolled", "subject-enrolled"))

    def test_record_conversion_uses_existing_ai_project_hint(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "repo"
            (project / ".git").mkdir(parents=True)
            config = TrayConfig("http://127.0.0.1:8080", project / "gateway.env", project, project / "client.db", authorized_roots=[project])
            event = event_from_record({"event_type": "ai.message", "payload": {"project_path": str(project), "role": "user", "content": "hello"}}, config, "core.ai")
            self.assertEqual(event.provenance["project_root"], str(project.resolve()))

    def test_ai_project_hint_takes_precedence_over_default_project(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            first = Path(raw) / "first"; (first / ".git").mkdir(parents=True)
            second = Path(raw) / "second"; (second / ".git").mkdir(parents=True)
            config = TrayConfig("http://server", Path(raw) / "token", first, Path(raw) / "queue.db", authorized_roots=[first, second])

            event = event_from_record(
                {"event_type": "ai.message", "payload": {"project": str(second), "role": "user", "content": "hello"}},
                config, "core.ai.codex", first,
            )

            self.assertEqual(event.provenance["project_root"], str(second.resolve()))

    def test_ai_record_uses_original_rfc3339_timestamp(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw); (project / ".git").mkdir()
            config = TrayConfig("http://server", project / "token", project, project / "queue.db", authorized_roots=[project])

            event = event_from_record({
                "event_type": "ai.message",
                "payload": {"role": "user", "content": "hello", "timestamp": "2026-09-06T10:20:30Z"},
            }, config, "core.ai.codex")

            self.assertEqual(event.occurred_at, "2026-09-06T10:20:30Z")

    def test_record_conversion_uses_source_position_for_stable_unique_ids(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw)
            config = TrayConfig("http://127.0.0.1:8080", project / "gateway.env", project, project / "client.db", authorized_roots=[project])
            payload = {"role": "user", "content": "repeat"}
            first = event_from_record({"event_type": "ai.message", "payload": payload, "provenance": {"source_file": "s.jsonl", "source_offset": 10}}, config, "core.ai")
            second = event_from_record({"event_type": "ai.message", "payload": payload, "provenance": {"source_file": "s.jsonl", "source_offset": 20}}, config, "core.ai")

            self.assertNotEqual(first.event_id, second.event_id)
            self.assertEqual(first.provenance["source_offset"], 10)

    def test_record_batch_keeps_ai_fallback_and_authorized_project_records(self):
        from aetheris.tray import TrayConfig, events_from_records

        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "authorized"
            project.mkdir()
            outside = Path(raw) / "outside"
            outside.mkdir()
            config = TrayConfig("http://127.0.0.1:8080", project / "gateway.env", project, project / "client.db", authorized_roots=[project])
            records = [
                ({"event_type": "ai.message", "payload": {"project": str(outside), "role": "user", "content": "ignore"}}, "core.ai.codex"),
                ({"event_type": "ai.message", "payload": {"project": str(project), "role": "user", "content": "keep"}}, "core.ai.codex"),
            ]

            events = events_from_records(records, config)

            self.assertEqual(len(events), 2)
            self.assertEqual(events[0].payload["project_label"], "Codex")
            self.assertEqual(events[1].payload["content"], "keep")

    def test_ai_record_with_unauthorized_cwd_uses_tool_fallback_without_path(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            authorized = root / "authorized"; (authorized / ".git").mkdir(parents=True)
            outside = root / "private" / "workspace"; outside.mkdir(parents=True)
            config = TrayConfig("http://server", root / "token", authorized, root / "queue.db", authorized_roots=[authorized], tenant_id="tenant-1")

            event = event_from_record({
                "event_type": "ai.message",
                "payload": {"tool": "codex", "project": str(outside), "role": "user", "content": "hello"},
                "provenance": {"source_file": "session.jsonl", "source_offset": 10},
            }, config, "core.ai.codex")

            serialized = event.to_dict()
            self.assertEqual(event.payload["project_label"], "Codex")
            self.assertEqual(event.payload["project_attribution"], "tool_fallback")
            self.assertNotIn(str(outside), str(serialized))
            self.assertNotIn("project", event.payload)
            self.assertIsNotNone(event.supersedes_event_id)
            self.assertNotEqual(event.event_id, event.supersedes_event_id)

            repeated = event_from_record({
                "event_type": "ai.message",
                "payload": {"tool": "codex", "project": str(outside), "role": "user", "content": "hello"},
                "provenance": {"source_file": "session.jsonl", "source_offset": 10},
            }, config, "core.ai.codex")
            self.assertEqual(repeated.event_id, event.event_id)

    def test_non_ai_record_with_unauthorized_hint_is_still_rejected(self):
        from aetheris.tray import TrayConfig, event_from_record

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            authorized = root / "authorized"; authorized.mkdir()
            outside = root / "outside"; outside.mkdir()
            config = TrayConfig("http://server", root / "token", authorized, root / "queue.db", authorized_roots=[authorized])

            with self.assertRaises(ValueError):
                event_from_record({"event_type": "terminal.command", "payload": {"project": str(outside), "command": "pwd"}}, config, "core.terminal")

    def test_tray_config_loads_required_paths_and_defaults_interval(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "aetheris.json"
            path.write_text(
                json.dumps({
                    "gateway_url": "http://127.0.0.1:8080",
                    "token_file": str(Path(raw) / "gateway.env"),
                    "project_root": raw,
                    "queue": str(Path(raw) / "client.db"),
                }),
                encoding="utf-8",
            )
            config = TrayConfig.from_file(path)
            self.assertEqual(config.gateway_url, "http://127.0.0.1:8080")
            self.assertEqual(config.interval_seconds, 15)
            self.assertEqual(config.authorized_roots, [Path(raw).resolve()])

    def test_tray_config_rejects_missing_project_root(self):
        from aetheris.tray import TrayConfig

        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "aetheris.json"
            path.write_text(json.dumps({"gateway_url": "http://127.0.0.1:8080"}), encoding="utf-8")
            with self.assertRaises(ValueError):
                TrayConfig.from_file(path)


if __name__ == "__main__":
    unittest.main()
