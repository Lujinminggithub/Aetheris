import json
import subprocess
import tempfile
import unittest
from pathlib import Path


class FakeGateway:
    def __init__(self, fail=False, heartbeat_identity=True):
        self.fail = fail
        self.heartbeat_identity = heartbeat_identity
        self.token = ""
        self.heartbeats = 0
        self.device = ""
        self.subject = ""

    def bootstrap_device(self, **kwargs):
        if self.fail:
            raise ValueError("invalid enrollment")
        self.device, self.subject = kwargs["device_id"], kwargs["subject_id"]
        return {"device_token": "issued-device-token", "device_id": self.device, "subject_id": self.subject, "tenant_id": "tenant-local"}

    def heartbeat(self):
        self.heartbeats += 1
        return {"status": "online", "device_id": self.device if self.heartbeat_identity else "wrong", "subject_id": self.subject}


class FakeCredentialStore:
    def __init__(self, path): self.path = Path(path); self.value = ""
    def write(self, value): self.value = value; self.path.parent.mkdir(parents=True, exist_ok=True); self.path.write_text("encrypted")
    def read(self): return self.value


class FakeStartup:
    def __init__(self): self.registered = None; self.removed = False
    def register(self, executable, config): self.registered = (Path(executable), Path(config))
    def remove(self): self.removed = True


class SetupAppTests(unittest.TestCase):
    def test_detects_running_versioned_core_process(self):
        from unittest.mock import patch
        from aetheris.setup_app import running_core_processes
        output = '"AetherisCore-0.3.7.exe","123","Console","1","10,000 K"\n"AetherisSetup-0.4.0.exe","124","Console","1","10,000 K"\n'
        with patch("aetheris.setup_app.subprocess.run", return_value=subprocess.CompletedProcess([], 0, output, "")):
            self.assertEqual(running_core_processes(), ["AetherisCore-0.3.7.exe"])

    def _request(self, root, project):
        from aetheris.projects import ProjectSource
        from aetheris.setup_app import InstallRequest
        return InstallRequest(Path(root) / "chosen-install", "http://server", [ProjectSource(Path(project), "git")], "DOMAIN\\Alice", "enroll", scan_root=Path(root))

    def test_invalid_enrollment_does_not_report_success_or_create_startup(self):
        from aetheris.setup_app import Installer
        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "repo"; (project / ".git").mkdir(parents=True)
            core = Path(raw) / "embedded-core.exe"; core.write_bytes(b"core")
            startup = FakeStartup()
            installer = Installer(core, lambda _: FakeGateway(fail=True), lambda path: FakeCredentialStore(path), startup, lambda *_: None, lambda *_: True)
            with self.assertRaises(ValueError): installer.install(self._request(raw, project))
            self.assertIsNone(startup.registered)
            self.assertFalse((Path(raw) / "chosen-install" / "config" / "aetheris.json").exists())

    def test_success_installs_stable_core_and_requires_heartbeat_and_status(self):
        from aetheris.setup_app import Installer
        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "repo"; (project / ".git").mkdir(parents=True)
            core = Path(raw) / "embedded-core.exe"; core.write_bytes(b"core")
            gateway = FakeGateway(); startup = FakeStartup(); stores = []
            def store_factory(path): store = FakeCredentialStore(path); stores.append(store); return store
            installer = Installer(core, lambda _: gateway, store_factory, startup, lambda *_: None, lambda *_: True, machine_id="machine-1")
            result = installer.install(self._request(raw, project))
            config = json.loads(result.config_path.read_text(encoding="utf-8"))
            self.assertEqual(result.executable.name, "AetherisCore.exe")
            self.assertEqual(result.executable.read_bytes(), b"core")
            self.assertTrue((result.executable.parent / "uninstall.ps1").is_file())
            self.assertEqual(gateway.heartbeats, 1)
            self.assertEqual(stores[0].value, "issued-device-token")
            self.assertEqual(config["credential_kind"], "device_token")
            self.assertEqual(config["tenant_id"], "tenant-local")
            self.assertEqual(config["project_roots"], [{"path": str(project.resolve()), "vcs": "git"}])
            self.assertEqual(config["scan_root"], str(Path(raw).resolve()))
            self.assertEqual(startup.registered[0], result.executable)

    def test_heartbeat_identity_mismatch_does_not_install(self):
        from aetheris.setup_app import Installer
        with tempfile.TemporaryDirectory() as raw:
            project = Path(raw) / "repo"; (project / ".git").mkdir(parents=True)
            core = Path(raw) / "embedded-core.exe"; core.write_bytes(b"core")
            startup = FakeStartup()
            installer = Installer(core, lambda _: FakeGateway(heartbeat_identity=False), lambda path: FakeCredentialStore(path), startup, lambda *_: None, lambda *_: True, machine_id="machine-1")
            with self.assertRaisesRegex(ValueError, "heartbeat"):
                installer.install(self._request(raw, project))
            self.assertIsNone(startup.registered)

    def test_install_config_enables_detected_local_ai_history_sources(self):
        from aetheris.setup_app import Installer

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            project = root / "repo"; (project / ".git").mkdir(parents=True)
            history = root / "codex-sessions"; history.mkdir()
            core = root / "embedded-core.exe"; core.write_bytes(b"core")
            installer = Installer(
                core,
                lambda _: FakeGateway(),
                lambda path: FakeCredentialStore(path),
                FakeStartup(),
                lambda *_: None,
                lambda *_: True,
                machine_id="machine-1",
                ai_roots_provider=lambda: [("codex", history)],
            )

            result = installer.install(self._request(raw, project))
            config = json.loads(result.config_path.read_text(encoding="utf-8"))

            self.assertEqual(config["ai_session_roots"], [{"tool": "codex", "path": str(history.resolve())}])


if __name__ == "__main__":
    unittest.main()
