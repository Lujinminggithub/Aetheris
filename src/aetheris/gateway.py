from __future__ import annotations

import argparse
from html import escape
import json
import platform
import secrets
import socket
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlparse
from urllib.request import Request, urlopen
from urllib.error import HTTPError

from .events import AetherisEvent
from .version import __version__
from .server import EventStore, serialize_response


def create_gateway_server(
    store: EventStore,
    token: str,
    host: str = "127.0.0.1",
    port: int = 0,
    static_dir: str | Path | None = None,
    admin_bootstrap_password: str | None = None,
) -> ThreadingHTTPServer:
    if not token:
        raise ValueError("a non-empty device token is required")

    static_root = Path(static_dir).expanduser().resolve() if static_dir else None
    allowed_downloads = {f"AetherisSetup-{__version__}.exe", f"AetherisSetup-{__version__}.exe.sha256"}
    store.ensure_admin(admin_bootstrap_password)
    sessions: dict[str, dict] = {}

    class GatewayHandler(BaseHTTPRequestHandler):
        server_version = "AetherisGateway/0.1"

        def log_message(self, format: str, *args) -> None:
            return

        def _send(self, status: int, payload: dict) -> None:
            _, body = serialize_response(payload, status)
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _send_bytes(self, status: int, body: bytes, content_type: str) -> None:
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(body)

        def _send_html(self, body: str) -> None:
            self._send_html_status(200, body)

        def _send_html_status(self, status: int, body: str) -> None:
            self._send_bytes(status, body.encode("utf-8"), "text/html; charset=utf-8")

        def _redirect(self, location: str) -> None:
            self.send_response(303)
            self.send_header("Location", location)
            self.send_header("Content-Length", "0")
            self.end_headers()

        def _session(self) -> tuple[str, dict] | None:
            raw = self.headers.get("Cookie", "")
            for part in raw.split(";"):
                name, separator, value = part.strip().partition("=")
                if separator and name == "aetheris_session" and value in sessions:
                    return value, sessions[value]
            return None

        def _admin_session(self, require_changed: bool = True) -> dict | None:
            current = self._session()
            if not current:
                return None
            _, state = current
            if require_changed and state.get("must_change"):
                return None
            return state

        def _form(self) -> dict[str, str]:
            length = int(self.headers.get("Content-Length", "0"))
            raw = self.rfile.read(min(length, 64 * 1024)).decode("utf-8", errors="replace")
            return {key: values[0] for key, values in parse_qs(raw, keep_blank_values=True).items()}

        def _send_download_page(self) -> None:
            links = []
            download_root = static_root / "downloads" if static_root else None
            if download_root and download_root.is_dir():
                for filename in sorted(allowed_downloads):
                    if (download_root / filename).is_file():
                        links.append(f'<li><a href="/downloads/{filename}">{filename}</a></li>')
            body = (
                "<!doctype html><html><head><meta charset=\"utf-8\">"
                "<title>Aetheris Downloads</title></head><body>"
                "<h1>Aetheris Windows Setup</h1>"
                "<p>Windows installer download service</p><ul>"
                + "".join(links)
                + "</ul><p><a href=\"/dashboard\">Personal workspace</a> · <a href=\"/healthz\">Service health</a></p></body></html>"
            ).encode("utf-8")
            self._send_bytes(200, body, "text/html; charset=utf-8")

        def _send_download(self, filename: str) -> None:
            if not static_root or filename not in allowed_downloads:
                self._send(404, {"error": "not_found"})
                return
            target = (static_root / "downloads" / filename).resolve()
            download_root = (static_root / "downloads").resolve()
            if download_root not in target.parents or not target.is_file():
                self._send(404, {"error": "not_found"})
                return
            content_type = "application/zip" if filename.endswith(".zip") else (
                "application/vnd.microsoft.portable-executable" if filename.endswith(".exe") else "text/plain; charset=utf-8"
            )
            self._send_bytes(200, target.read_bytes(), content_type)

        def _authorized(self) -> bool:
            return self.headers.get("Authorization") == f"Bearer {token}"

        def do_GET(self) -> None:
            if self.path == "/healthz":
                self._send(200, {"status": "ok"})
                return
            if self.path in {"/admin", "/dashboard"}:
                current_session = self._session()
                if not current_session:
                    self._send_html(
                        "<!doctype html><html><head><meta charset=\"utf-8\"><title>Aetheris Admin Login</title>"
                        "<style>body{font:16px system-ui;max-width:520px;margin:70px auto;padding:0 20px}"
                        "main{border:1px solid #ddd;padding:28px;border-radius:8px}input,button{font:inherit;padding:10px;margin:6px 0;width:100%;box-sizing:border-box}"
                        "button{background:#1769aa;color:white;border:0;border-radius:4px;cursor:pointer}</style></head>"
                        "<body><main><h1>Aetheris Admin Login</h1><form method=\"post\" action=\"/admin/login\">"
                        "<label>Username<br><input name=\"username\" autocomplete=\"username\" required></label>"
                        "<label>Password<br><input type=\"password\" name=\"password\" autocomplete=\"current-password\" required></label>"
                        "<button type=\"submit\">Sign in</button></form></main></body></html>"
                    )
                    return
                _, state = current_session
                if state.get("must_change"):
                    self._send_html(
                        "<!doctype html><html><head><meta charset=\"utf-8\"><title>Change admin password</title>"
                        "<style>body{font:16px system-ui;max-width:520px;margin:70px auto;padding:0 20px}main{border:1px solid #ddd;padding:28px;border-radius:8px}input,button{font:inherit;padding:10px;margin:6px 0;width:100%;box-sizing:border-box}button{background:#1769aa;color:white;border:0;border-radius:4px}</style></head>"
                        "<body><main><h1>Change admin password</h1><p>First login requires a new password.</p>"
                        "<form method=\"post\" action=\"/admin/change-password\"><label>Current password<br><input type=\"password\" name=\"old_password\" required></label>"
                        "<label>New password (10+ characters)<br><input type=\"password\" name=\"new_password\" required></label>"
                        "<button type=\"submit\">Change password</button></form></main></body></html>"
                    )
                    return
                self._send_html(
                    "<!doctype html><html><head><meta charset=\"utf-8\"><title>Aetheris Device Management</title>"
                    "<style>body{font:16px system-ui;max-width:1180px;margin:30px auto;padding:0 18px;color:#1f2933}header{display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid #ddd;padding-bottom:14px}"
                    "nav button{width:auto;background:#1769aa;color:white;border:0;padding:8px 12px;margin:3px;border-radius:4px;cursor:pointer}.panel{display:none;padding-top:20px}.panel.active{display:block}.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}.card{border:1px solid #d8dee4;border-radius:6px;padding:16px;background:#fff}.card b{font-size:26px;display:block;margin-top:4px}table{border-collapse:collapse;width:100%;margin-top:12px}th,td{border-bottom:1px solid #e5e7eb;text-align:left;padding:9px}pre{background:#f6f8fa;padding:12px;overflow:auto}</style></head>"
                    "<body><header><div><h1>Aetheris Device Management</h1><div>Registered clients, terminal activity, effectiveness data, and exports</div></div><form method=\"post\" action=\"/admin/logout\"><button>Sign out</button></form></header>"
                    "<nav><button onclick=\"show('overview')\">Effectiveness dashboard</button><button onclick=\"show('devices')\">Terminal management</button><button onclick=\"show('downloads')\">Data downloads</button></nav>"
                    "<section id=\"overview\" class=\"panel active\"><div id=\"cards\" class=\"cards\"></div><h2>Event organization</h2><pre id=\"types\"></pre></section>"
                    "<section id=\"devices\" class=\"panel\"><h2>Registered clients</h2><table><thead><tr><th>Device</th><th>Host</th><th>Platform</th><th>Last seen</th></tr></thead><tbody id=\"deviceRows\"></tbody></table><h2>Recent terminal activity</h2><pre id=\"terminal\"></pre></section>"
                    "<section id=\"downloads\" class=\"panel\"><h2>Data downloads</h2><p>Exports are generated from server-stored redacted events.</p><button onclick=\"download('jsonl')\">JSONL</button><button onclick=\"download('markdown')\">Markdown</button><button onclick=\"download('openai_messages')\">OpenAI messages</button><button onclick=\"download('agent_trajectory')\">Agent trajectory</button></section>"
                    "<script>async function load(){let h={};let s=await fetch('/v1/admin/state');if(s.status===403){location.reload();return}if(!s.ok){return}let d=await s.json();document.getElementById('cards').innerHTML='<div class=\"card\">Total events<b>'+d.summary.total_events+'</b></div><div class=\"card\">Devices<b>'+d.devices.length+'</b></div><div class=\"card\">Projects<b>'+Object.keys(d.summary.by_project).length+'</b></div>';document.getElementById('types').textContent=JSON.stringify(d.summary.by_event_type,null,2);document.getElementById('deviceRows').innerHTML=d.devices.map(x=>'<tr><td>'+x.device_id+'</td><td>'+x.hostname+'</td><td>'+x.platform+'</td><td>'+new Date(x.last_seen*1000).toLocaleString()+'</td></tr>').join('');document.getElementById('terminal').textContent=JSON.stringify(d.terminal_events,null,2)}function show(id){document.querySelectorAll('.panel').forEach(x=>x.classList.remove('active'));document.getElementById(id).classList.add('active')}async function download(f){let r=await fetch('/v1/export/events.'+f);if(!r.ok){alert('Unavailable');return}let b=await r.blob(),a=document.createElement('a');a.href=URL.createObjectURL(b);a.download='aetheris-'+f+'.txt';a.click()}load();</script></body></html>"
                )
                return
            if self.path == "/" or self.path == "/downloads/":
                self._send_download_page()
                return
            parsed = urlparse(self.path)
            if parsed.path == "/v1/admin/state":
                state = self._admin_session(require_changed=False)
                if not state:
                    self._send(401, {"error": "admin_login_required"})
                    return
                if state.get("must_change"):
                    self._send(403, {"error": "password_change_required"})
                    return
                self._send(
                    200,
                    {
                        "status": "ok",
                        "summary": store.summary(),
                        "devices": store.list_devices(),
                        "episodes": store.episodes(),
                        "terminal_events": store.list_events(limit=200, event_type="terminal.command"),
                        "pulse": store.pulse(),
                        "daily_log": store.daily_log(),
                    },
                )
                return
            if parsed.path in {"/v1/summary", "/v1/events", "/v1/devices", "/v1/episodes", "/v1/pulse"} or parsed.path.startswith("/v1/export/"):
                if not self._authorized():
                    self._send(401, {"error": "unauthorized"})
                    return
                query = parse_qs(parsed.query)
                if parsed.path == "/v1/summary":
                    self._send(200, store.summary())
                    return
                if parsed.path == "/v1/events":
                    events = store.list_events(
                        int(query.get("limit", [100])[0]),
                        project_id=query.get("project_id", [None])[0],
                        event_type=query.get("event_type", [None])[0],
                    )
                    self._send(200, {"events": events, "count": len(events)})
                    return
                if parsed.path == "/v1/devices":
                    self._send(200, {"devices": store.list_devices(), "count": len(store.list_devices())})
                    return
                if parsed.path == "/v1/episodes":
                    self._send(200, {"episodes": store.episodes(), "count": len(store.episodes())})
                    return
                if parsed.path == "/v1/pulse":
                    self._send(200, store.pulse())
                    return
                format_name = parsed.path.removeprefix("/v1/export/")
                if format_name.startswith("events."):
                    format_name = format_name.removeprefix("events.")
                try:
                    body = store.export(format_name).encode("utf-8")
                except ValueError as exc:
                    self._send(400, {"error": str(exc)})
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/jsonl; charset=utf-8" if format_name != "markdown" else "text/markdown; charset=utf-8")
                self.send_header("Content-Disposition", f"attachment; filename=aetheris-{format_name}.txt")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            if self.path.startswith("/downloads/"):
                filename = unquote(self.path[len("/downloads/"):]).split("?", 1)[0]
                self._send_download(filename)
                return
            self._send(404, {"error": "not_found"})

        def do_POST(self) -> None:
            if self.path.startswith("/v1/admin/events/") and self.path.endswith("/delete"):
                state = self._admin_session()
                if not state:
                    self._send(401, {"error": "admin_login_required"})
                    return
                event_id = unquote(self.path[len("/v1/admin/events/"):-len("/delete")])
                form = self._form()
                store.delete_event(event_id, form.get("reason", "admin deletion"))
                self._send(200, {"event_id": event_id, "status": "deleted"})
                return
            if self.path == "/admin/login":
                form = self._form()
                auth = store.authenticate_admin(form.get("username", ""), form.get("password", ""))
                if not auth:
                    self._send(401, {"error": "invalid_credentials"})
                    return
                session_id = secrets.token_urlsafe(32)
                sessions[session_id] = auth
                self.send_response(303)
                self.send_header("Location", "/admin")
                self.send_header("Set-Cookie", f"aetheris_session={session_id}; HttpOnly; SameSite=Strict; Path=/")
                self.send_header("Content-Length", "0")
                self.end_headers()
                return
            if self.path == "/admin/change-password":
                current = self._session()
                if not current:
                    self._send(401, {"error": "admin_login_required"})
                    return
                session_id, state = current
                form = self._form()
                old_password = form.get("old_password", "")
                new_password = form.get("new_password", "")
                if store.authenticate_admin(state["username"], old_password) is None:
                    reason = "Current password is incorrect."
                elif len(new_password) < 10:
                    reason = "New password must be at least 10 characters."
                elif not store.change_admin_password(state["username"], old_password, new_password):
                    reason = "Password change failed; try again."
                else:
                    state["must_change"] = False
                    self._redirect("/admin")
                    return
                self._send_html_status(
                    400,
                    "<!doctype html><html><head><meta charset=\"utf-8\"><title>Change admin password</title>"
                    "<style>body{font:16px system-ui;max-width:520px;margin:70px auto;padding:0 20px}main{border:1px solid #ddd;padding:28px;border-radius:8px}input,button{font:inherit;padding:10px;margin:6px 0;width:100%;box-sizing:border-box}button{background:#1769aa;color:white;border:0;border-radius:4px}.error{color:#b42318}</style></head>"
                    f"<body><main><h1>Change admin password</h1><p class=\"error\">{escape(reason)}</p><form method=\"post\" action=\"/admin/change-password\">"
                    "<label>Current password<br><input type=\"password\" name=\"old_password\" required></label>"
                    "<label>New password (10+ characters)<br><input type=\"password\" name=\"new_password\" required></label>"
                    "<button type=\"submit\">Change password</button></form></main></body></html>",
                )
                return
            if self.path == "/admin/logout":
                current = self._session()
                if current:
                    sessions.pop(current[0], None)
                self.send_response(303)
                self.send_header("Location", "/admin")
                self.send_header("Set-Cookie", "aetheris_session=; Max-Age=0; HttpOnly; SameSite=Strict; Path=/")
                self.send_header("Content-Length", "0")
                self.end_headers()
                return
            if self.path == "/v1/devices/register":
                if not self._authorized():
                    self._send(401, {"error": "unauthorized"})
                    return
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                    body = json.loads(self.rfile.read(length))
                    result = store.register_device(
                        str(body.get("device_id", "")),
                        client_version=str(body.get("client_version", "")),
                        hostname=str(body.get("hostname", "")),
                        platform=str(body.get("platform", "")),
                        metadata=body.get("metadata") if isinstance(body.get("metadata"), dict) else {},
                    )
                    self._send(200, result)
                except (ValueError, TypeError, json.JSONDecodeError) as exc:
                    self._send(400, {"error": "invalid_request", "reason": str(exc)})
                return
            if self.path != "/v1/events":
                self._send(404, {"error": "not_found"})
                return
            if not self._authorized():
                self._send(401, {"error": "unauthorized"})
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if length <= 0 or length > 10 * 1024 * 1024:
                    raise ValueError("invalid content length")
                body = json.loads(self.rfile.read(length))
                raw_events = body.get("events")
                if not isinstance(raw_events, list) or len(raw_events) > 1000:
                    raise ValueError("events must be a list of at most 1000 items")
                results = []
                for raw in raw_events:
                    try:
                        event = AetherisEvent.from_dict(raw)
                        results.append({"event_id": event.event_id, "status": store.insert_if_new(event)})
                    except (TypeError, ValueError, KeyError) as exc:
                        results.append({"event_id": raw.get("event_id") if isinstance(raw, dict) else None, "status": "rejected", "reason": str(exc)})
                self._send(202, {"results": results})
            except (ValueError, json.JSONDecodeError) as exc:
                self._send(400, {"error": "invalid_request", "reason": str(exc)})

    server = ThreadingHTTPServer((host, port), GatewayHandler)
    server.aetheris_store = store
    server.aetheris_static_dir = static_root
    return server


def _read_token_file(path: str) -> str:
    values = {}
    with open(path, "r", encoding="utf-8-sig") as handle:
        for line in handle:
            key, separator, value = line.rstrip("\n").partition("=")
            if separator:
                values[key] = value
    return values.get("AETHERIS_TOKEN", "")


def _read_admin_bootstrap_file(path: str) -> str:
    return Path(path).read_text(encoding="utf-8-sig").strip()


class GatewayClient:
    def __init__(self, base_url: str, token: str = ""):
        self.base_url = base_url.rstrip("/")
        self.token = token

    def bootstrap_device(
        self,
        *,
        enrollment_secret: str,
        device_id: str,
        subject_id: str,
        subject_name: str,
        client_version: str,
    ) -> dict:
        payload = json.dumps({
            "enrollment_secret": enrollment_secret,
            "device_id": device_id,
            "subject_id": subject_id,
            "subject_name": subject_name,
            "client_version": client_version,
            "hostname": socket.gethostname(),
        }, separators=(",", ":")).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/v1/device/bootstrap",
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def heartbeat(self) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        request = Request(
            f"{self.base_url}/api/v1/device/heartbeat",
            data=b"{}",
            headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def browser_policy(self) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        request = Request(
            f"{self.base_url}/api/v1/device/browser-policy",
            headers={"Authorization": f"Bearer {self.token}"},
            method="GET",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def application_capture_policy(self) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        request = Request(
            f"{self.base_url}/api/v1/device/application-capture-policy",
            headers={"Authorization": f"Bearer {self.token}"},
            method="GET",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def project_identity_key(self) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        request = Request(
            f"{self.base_url}/api/v1/device/project-identity-key",
            headers={"Authorization": f"Bearer {self.token}"},
            method="GET",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def register_projects(self, batch: dict) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        payload = json.dumps(batch, separators=(",", ":")).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/v1/device/projects",
            data=payload,
            headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
            method="PUT",
        )
        with urlopen(request, timeout=15) as response:
            return json.loads(response.read())

    def upload_adapter_health(self, snapshots: list[dict]) -> dict:
        if not self.token:
            raise ValueError("device token 不能为空")
        payload = json.dumps({"snapshots": snapshots}, separators=(",", ":")).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/v1/device/adapter-health",
            data=payload,
            headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
            method="POST",
        )
        with urlopen(request, timeout=10) as response:
            return json.loads(response.read())

    def register_device(self, device_id: str, client_version: str = __version__) -> dict:
        payload = json.dumps(
            {
                "device_id": device_id,
                "client_version": client_version,
                "hostname": socket.gethostname(),
                "platform": platform.platform(),
            },
            separators=(",", ":"),
        ).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/v1/devices/register",
            data=payload,
            headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urlopen(request, timeout=10) as response:
                return json.loads(response.read())
        except HTTPError as exc:
            if exc.code not in {404, 405}:
                raise
            fallback = Request(
                f"{self.base_url}/v1/devices/register",
                data=payload,
                headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
                method="POST",
            )
            with urlopen(fallback, timeout=10) as response:
                return json.loads(response.read())

    def send(self, events: list[AetherisEvent]) -> list[dict]:
        payload = json.dumps({"events": [event.to_dict() for event in events]}, separators=(",", ":")).encode("utf-8")
        request = Request(
            f"{self.base_url}/api/v1/ingest",
            data=payload,
            headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urlopen(request, timeout=10) as response:
                body = json.loads(response.read())
                return body.get("results", body.get("accepted", []))
        except HTTPError as exc:
            if exc.code not in {404, 405}:
                raise
            fallback = Request(
                f"{self.base_url}/v1/events",
                data=payload,
                headers={"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"},
                method="POST",
            )
            with urlopen(fallback, timeout=10) as response:
                return json.loads(response.read())["results"]


def main() -> None:
    parser = argparse.ArgumentParser(description="Run the Aetheris Gateway")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8080)
    parser.add_argument("--database", default="aetheris-server.db")
    parser.add_argument("--token")
    parser.add_argument("--token-file")
    parser.add_argument("--static-dir")
    parser.add_argument("--admin-bootstrap-file")
    args = parser.parse_args()
    token = args.token or (_read_token_file(args.token_file) if args.token_file else "")
    if not token:
        parser.error("a non-empty --token or --token-file is required")
    store = EventStore(args.database)
    admin_password = _read_admin_bootstrap_file(args.admin_bootstrap_file) if args.admin_bootstrap_file else None
    server = create_gateway_server(store, token, args.host, args.port, args.static_dir, admin_password)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.shutdown()
        store.close()


if __name__ == "__main__":
    main()
