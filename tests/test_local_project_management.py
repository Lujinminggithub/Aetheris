import http.cookiejar
import json
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import HTTPCookieProcessor, Request, build_opener, urlopen


class LocalProjectManagementTests(unittest.TestCase):
    def test_local_view_exposes_vscode_status_and_actions(self):
        from aetheris.local_view import create_local_view_server

        actions = []
        server = create_local_view_server(
            lambda: [], vscode_status_provider=lambda: {"component_state": "paused_by_user"},
            vscode_events_provider=lambda: [{"event_type": "ide.file_saved", "project_name": "repo"}],
            vscode_action=lambda action: actions.append(action), control_token="control",
        )
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            jar = http.cookiejar.CookieJar(); client = build_opener(HTTPCookieProcessor(jar))
            base = f"http://127.0.0.1:{server.server_port}"
            page = client.open(base + "/").read().decode("utf-8")
            for label in ("VS Code 采集", "安装扩展", "启用", "停用", "卸载", "清除缓存"):
                self.assertIn(label, page)
            with client.open(base + "/api/vscode/status") as response:
                self.assertEqual(json.loads(response.read())["component_state"], "paused_by_user")
            request = Request(base + "/api/vscode/enable", data=b"{}", headers={"Origin": base, "X-Aetheris-Control": "local-lens"}, method="POST")
            with client.open(request) as response:
                self.assertEqual(response.status, 204)
            self.assertEqual(actions, ["enable"])
        finally:
            server.shutdown(); server.server_close()

    def test_local_view_renders_process_consent_controls(self):
        from aetheris.local_view import _local_workspace_html

        html = _local_workspace_html()
        self.assertIn("进程授权", html)
        self.assertIn("工作相关", html)
        self.assertIn("仅当前项目", html)
        self.assertIn("永久忽略", html)
        self.assertIn("decideProcess", html)
        self.assertIn("function show", html)
        self.assertIn("function addProject", html)
        self.assertIn("function loadEpisodes", html)
        self.assertIn("应用 OCR 保底", html)

    def test_local_view_marks_deleted_project_and_keeps_remove_action(self):
        from aetheris.local_view import _local_workspace_html

        html = _local_workspace_html()
        self.assertIn("目录不存在", html)
        self.assertIn("project.available===false", html)

    def test_local_view_renders_process_bulk_controls(self):
        from aetheris.local_view import _local_workspace_html

        html = _local_workspace_html()
        self.assertIn("processFilter", html)
        self.assertIn("processSearch", html)
        self.assertIn("toggleAllProcesses", html)
        self.assertIn("batchProcessDecision", html)
        self.assertIn("批量标记为工作相关", html)
        self.assertIn("批量永久忽略", html)
        self.assertIn('<option value="processed">已处理</option>', html)
        self.assertIn("process.state!=='pending'", html)

    def test_local_view_exposes_pending_process_consent(self):
        from aetheris.local_view import create_local_view_server

        server = create_local_view_server(
            lambda: [],
            process_consent_provider=lambda: [{"identity_key": "abc", "name": "tool.exe"}],
        )
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            with urlopen(f"http://127.0.0.1:{server.server_port}/api/processes/pending") as response:
                self.assertEqual(response.status, 200)
                self.assertEqual(json.loads(response.read())["processes"][0]["name"], "tool.exe")
        finally:
            server.shutdown()
            server.server_close()

    def test_local_view_accepts_process_consent_decision(self):
        from aetheris.local_view import create_local_view_server

        decisions = []
        server = create_local_view_server(
            lambda: [],
            process_consent_decision=lambda body: decisions.append(body),
            control_token="control",
        )
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            jar = http.cookiejar.CookieJar()
            opener = build_opener(HTTPCookieProcessor(jar))
            opener.open(f"http://127.0.0.1:{server.server_port}/")
            request = Request(
                f"http://127.0.0.1:{server.server_port}/api/processes/consent",
                data=json.dumps({"identity_key": "abc", "decision": "allow_global", "project_ids": []}).encode(),
                headers={"Content-Type": "application/json", "Origin": f"http://127.0.0.1:{server.server_port}", "X-Aetheris-Control": "local-lens"},
                method="POST",
            )
            with opener.open(request) as response:
                self.assertEqual(response.status, 204)
            self.assertEqual(decisions[0]["decision"], "allow_global")
        finally:
            server.shutdown()
            server.server_close()

    def test_local_view_accepts_process_consent_reset(self):
        from aetheris.local_view import create_local_view_server

        reset_keys = []
        server = create_local_view_server(
            lambda: [],
            process_consent_reset=lambda identity_key: reset_keys.append(identity_key),
            control_token="control",
        )
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            jar = http.cookiejar.CookieJar()
            opener = build_opener(HTTPCookieProcessor(jar))
            opener.open(f"http://127.0.0.1:{server.server_port}/")
            request = Request(
                f"http://127.0.0.1:{server.server_port}/api/processes/consent/identity-1",
                data=b"{}",
                headers={"Origin": f"http://127.0.0.1:{server.server_port}", "X-Aetheris-Control": "local-lens"},
                method="POST",
            )
            with opener.open(request) as response:
                self.assertEqual(response.status, 204)
            self.assertEqual(reset_keys, ["identity-1"])
        finally:
            server.shutdown()
            server.server_close()

    def test_local_view_exposes_personal_work_episodes(self):
        from aetheris.local_view import create_local_view_server

        server = create_local_view_server(lambda: [], local_episode_provider=lambda: [{"episode_id": "local-1", "objective": "修复登录"}])
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            with urlopen(f"http://127.0.0.1:{server.server_port}/api/work-episodes") as response:
                self.assertEqual(response.status, 200)
                self.assertEqual(json.loads(response.read())["episodes"][0]["objective"], "修复登录")
        finally:
            server.shutdown()
            server.server_close()

    def test_exit_action_requires_control_session(self):
        from aetheris.local_view import create_local_view_server

        exited = threading.Event()
        server = create_local_view_server(lambda: [], control_token="control-test-token", exit_action=exited.set)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        base = f"http://127.0.0.1:{server.server_port}"
        try:
            cookies = http.cookiejar.CookieJar(); client = build_opener(HTTPCookieProcessor(cookies))
            client.open(base + "/", timeout=3).read()
            request = Request(base + "/api/actions/exit", data=b"{}", headers={"Content-Type":"application/json", "Origin":base, "X-Aetheris-Control":"local-lens"}, method="POST")
            with client.open(request, timeout=3) as response:
                self.assertEqual(response.status, 202)
            self.assertTrue(exited.wait(2))
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_project_mutations_require_local_control_session_and_revision(self):
        from aetheris.local_view import create_local_view_server
        from aetheris.project_registry import ProjectRegistry

        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            config = root / "config" / "aetheris.json"; config.parent.mkdir()
            config.write_text(json.dumps({"project_revision": 0, "project_roots": [], "authorized_roots": []}), encoding="utf-8")
            project = root / "repo"; (project / ".git").mkdir(parents=True)
            registry = ProjectRegistry(config)
            server = create_local_view_server(lambda: [], project_registry=registry, control_token="control-test-token")
            thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
            base = f"http://127.0.0.1:{server.server_port}"
            try:
                body = json.dumps({"revision": 0, "path": str(project)}).encode("utf-8")
                with self.assertRaises(HTTPError) as denied:
                    urlopen(Request(base + "/api/projects/add", data=body, headers={"Content-Type": "application/json"}, method="POST"), timeout=3)
                self.assertEqual(denied.exception.code, 403)

                cookies = http.cookiejar.CookieJar()
                client = build_opener(HTTPCookieProcessor(cookies))
                with client.open(base + "/", timeout=3) as response:
                    self.assertIn("HttpOnly", response.headers.get("Set-Cookie", ""))
                headers = {
                    "Content-Type": "application/json",
                    "Origin": base,
                    "X-Aetheris-Control": "local-lens",
                }
                with client.open(Request(base + "/api/projects/add", data=body, headers=headers, method="POST"), timeout=3) as response:
                    added = json.loads(response.read())
                self.assertEqual(added["revision"], 1)
                self.assertEqual(added["projects"][0]["vcs"], "git")

                with self.assertRaises(HTTPError) as conflict:
                    client.open(Request(base + "/api/projects/pause", data=body, headers=headers, method="POST"), timeout=3)
                self.assertEqual(conflict.exception.code, 409)
                with client.open(base + "/api/projects", timeout=3) as response:
                    listed = json.loads(response.read())
                self.assertEqual(listed["projects"][0]["state"], "active")
            finally:
                server.shutdown(); server.server_close(); thread.join(timeout=2)

    def test_non_loopback_host_is_rejected(self):
        from aetheris.local_view import create_local_view_server

        server = create_local_view_server(lambda: [], control_token="control-test-token")
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            request = Request(f"http://127.0.0.1:{server.server_port}/api/status", headers={"Host": "example.com"})
            with self.assertRaises(HTTPError) as denied:
                urlopen(request, timeout=3)
            self.assertEqual(denied.exception.code, 403)
        finally:
            server.shutdown(); server.server_close(); thread.join(timeout=2)


if __name__ == "__main__":
    unittest.main()
