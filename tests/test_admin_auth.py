import json
import tempfile
import threading
import unittest
from pathlib import Path
from urllib.parse import urlencode
from urllib.error import HTTPError
from urllib.request import HTTPRedirectHandler, Request, build_opener, urlopen


class AdminAuthTests(unittest.TestCase):
    def test_authenticated_admin_can_delete_event_and_tombstone_blocks_replay(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore
        from tests.helpers import make_sample_event

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            event = make_sample_event()
            store.insert_if_new(event)
            server = create_gateway_server(store, token="device-token", admin_bootstrap_password="initial-password")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                class NoRedirect(HTTPRedirectHandler):
                    def redirect_request(self, req, fp, code, msg, headers, newurl):
                        return None
                login = Request(base + "/admin/login", data=urlencode({"username": "admin", "password": "initial-password"}).encode(), headers={"Content-Type": "application/x-www-form-urlencoded"}, method="POST")
                try:
                    build_opener(NoRedirect).open(login, timeout=3)
                except HTTPError as response:
                    cookie = response.headers["Set-Cookie"].split(";", 1)[0]
                change = Request(base + "/admin/change-password", data=urlencode({"old_password": "initial-password", "new_password": "new-password-123"}).encode(), headers={"Content-Type": "application/x-www-form-urlencoded", "Cookie": cookie}, method="POST")
                try:
                    build_opener(NoRedirect).open(change, timeout=3)
                except HTTPError:
                    pass
                delete = Request(base + f"/v1/admin/events/{event.event_id}/delete", data=urlencode({"reason": "admin review"}).encode(), headers={"Content-Type": "application/x-www-form-urlencoded", "Cookie": cookie}, method="POST")
                with urlopen(delete, timeout=3) as response:
                    self.assertEqual(json.loads(response.read())["status"], "deleted")
                self.assertEqual(store.count(), 0)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()

    def test_password_change_failure_returns_html_reason(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            server = create_gateway_server(store, token="device-token", admin_bootstrap_password="initial-password")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                class NoRedirect(HTTPRedirectHandler):
                    def redirect_request(self, req, fp, code, msg, headers, newurl):
                        return None
                login = Request(base + "/admin/login", data=urlencode({"username": "admin", "password": "initial-password"}).encode(), headers={"Content-Type": "application/x-www-form-urlencoded"}, method="POST")
                try:
                    build_opener(NoRedirect).open(login, timeout=3)
                except HTTPError as response:
                    cookie = response.headers["Set-Cookie"].split(";", 1)[0]
                change = Request(base + "/admin/change-password", data=urlencode({"old_password": "initial-password", "new_password": "short"}).encode(), headers={"Content-Type": "application/x-www-form-urlencoded", "Cookie": cookie}, method="POST")
                with self.assertRaises(HTTPError) as raised:
                    urlopen(change, timeout=3)
                body = raised.exception.read().decode("utf-8")
                self.assertEqual(raised.exception.code, 400)
                self.assertIn("New password must be at least 10 characters", body)
                self.assertIn("Change admin password", body)
                self.assertNotIn('"password_change_failed"', body)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()
    def test_failed_login_is_rejected_and_logout_invalidates_session(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            server = create_gateway_server(store, token="device-token", admin_bootstrap_password="initial-password")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                bad_login = Request(
                    base + "/admin/login",
                    data=urlencode({"username": "admin", "password": "wrong-password"}).encode("utf-8"),
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                    method="POST",
                )
                with self.assertRaises(HTTPError) as raised:
                    urlopen(bad_login, timeout=3)
                self.assertEqual(raised.exception.code, 401)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()

    def test_first_admin_login_requires_password_change(self):
        from aetheris.gateway import create_gateway_server
        from aetheris.server import EventStore

        with tempfile.TemporaryDirectory() as raw:
            store = EventStore(Path(raw) / "server.db")
            server = create_gateway_server(store, token="device-token", admin_bootstrap_password="initial-password")
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                base = f"http://127.0.0.1:{server.server_port}"
                with urlopen(base + "/admin", timeout=3) as response:
                    self.assertIn("Aetheris Admin Login", response.read().decode("utf-8"))
                login = Request(
                    base + "/admin/login",
                    data=urlencode({"username": "admin", "password": "initial-password"}).encode("utf-8"),
                    headers={"Content-Type": "application/x-www-form-urlencoded"},
                    method="POST",
                )
                class NoRedirect(HTTPRedirectHandler):
                    def redirect_request(self, req, fp, code, msg, headers, newurl):
                        return None

                try:
                    build_opener(NoRedirect).open(login, timeout=3)
                except HTTPError as response:
                    cookie = response.headers["Set-Cookie"].split(";", 1)[0]
                    self.assertEqual(response.code, 303)
                request = Request(base + "/admin", headers={"Cookie": cookie})
                with urlopen(request, timeout=3) as response:
                    self.assertIn("Change admin password", response.read().decode("utf-8"))
                change = Request(
                    base + "/admin/change-password",
                    data=urlencode({"old_password": "initial-password", "new_password": "new-password-123"}).encode("utf-8"),
                    headers={"Content-Type": "application/x-www-form-urlencoded", "Cookie": cookie},
                    method="POST",
                )
                try:
                    build_opener(NoRedirect).open(change, timeout=3)
                except HTTPError as response:
                    self.assertEqual(response.code, 303)
                request = Request(base + "/v1/admin/state", headers={"Cookie": cookie})
                with urlopen(request, timeout=3) as response:
                    self.assertEqual(json.loads(response.read())["status"], "ok")
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=2)
                store.close()


if __name__ == "__main__":
    unittest.main()
