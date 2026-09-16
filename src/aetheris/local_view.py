from __future__ import annotations

import json
import threading
from http.cookies import SimpleCookie
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import Callable

from .project_registry import ProjectRegistry, RevisionConflict
from .projects import discover_projects


def create_local_view_server(
    history_provider: Callable[[], list[dict]],
    status_provider: Callable[[], dict] | None = None,
    heartbeat_action: Callable[[], object] | None = None,
    capture_action: Callable[[], object] | None = None,
    exit_action: Callable[[], object] | None = None,
    process_consent_provider: Callable[[], list[dict]] | None = None,
    process_consent_decision: Callable[[dict], object] | None = None,
    process_consent_reset: Callable[[str], object] | None = None,
    local_episode_provider: Callable[[], list[dict]] | None = None,
    vscode_status_provider: Callable[[], dict] | None = None,
    vscode_events_provider: Callable[[], list[dict]] | None = None,
    vscode_action: Callable[[str], object] | None = None,
    *,
    project_registry: ProjectRegistry | None = None,
    control_token: str | None = None,
    port: int = 0,
) -> ThreadingHTTPServer:
    class LocalViewHandler(BaseHTTPRequestHandler):
        def log_message(self, format: str, *args) -> None:
            return

        def _host_allowed(self) -> bool:
            host = self.headers.get("Host", "")
            return host in {f"127.0.0.1:{self.server.server_port}", f"localhost:{self.server.server_port}"}

        def _send_json(self, status: int, value: object) -> None:
            body = json.dumps(value, ensure_ascii=True).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            self.wfile.write(body)

        def _mutation_allowed(self) -> bool:
            if control_token is None:
                return True
            expected_origin = f"http://127.0.0.1:{self.server.server_port}"
            if self.headers.get("Origin") != expected_origin or self.headers.get("X-Aetheris-Control") != "local-lens":
                return False
            cookie = SimpleCookie(self.headers.get("Cookie", ""))
            value = cookie.get("aetheris_control")
            return value is not None and value.value == control_token

        def _read_body(self) -> dict:
            try:
                length = int(self.headers.get("Content-Length", "0"))
            except ValueError as exc:
                raise ValueError("invalid content length") from exc
            if length < 0 or length > 64 * 1024:
                raise ValueError("request body too large")
            value = json.loads(self.rfile.read(length) or b"{}")
            if not isinstance(value, dict):
                raise ValueError("request body must be an object")
            return value

        @staticmethod
        def _snapshot(value) -> dict:
            return {"revision": value.revision, "projects": [item.to_dict() for item in value.projects]}

        def do_GET(self) -> None:
            if not self._host_allowed():
                self._send_json(403, {"error": "loopback_host_required"})
                return
            if self.path == "/":
                body = _local_workspace_html().encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/html; charset=utf-8")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Cache-Control", "no-store")
                if control_token is not None:
                    self.send_header("Set-Cookie", f"aetheris_control={control_token}; HttpOnly; SameSite=Strict; Path=/")
                self.end_headers()
                self.wfile.write(body)
                return
            if self.path == "/api/events":
                self._send_json(200, {"events": history_provider()})
                return
            if self.path == "/api/status":
                self._send_json(200, status_provider() if status_provider else {})
                return
            if self.path == "/api/projects" and project_registry is not None:
                self._send_json(200, self._snapshot(project_registry.load()))
                return
            if self.path == "/api/processes/pending":
                self._send_json(200, {"processes": process_consent_provider() if process_consent_provider else []})
                return
            if self.path == "/api/work-episodes":
                self._send_json(200, {"episodes": local_episode_provider() if local_episode_provider else []})
                return
            if self.path == "/api/vscode/status":
                self._send_json(200, vscode_status_provider() if vscode_status_provider else {})
                return
            if self.path == "/api/vscode/events" or self.path.startswith("/api/vscode/events?"):
                self._send_json(200, {"events": vscode_events_provider() if vscode_events_provider else []})
                return
            self._send_json(404, {"error": "not_found"})

        def do_POST(self) -> None:
            if not self._host_allowed() or not self._mutation_allowed():
                self._send_json(403, {"error": "local_control_required"})
                return
            actions = {"/api/actions/heartbeat": heartbeat_action, "/api/actions/capture": capture_action, "/api/actions/exit": exit_action}
            action = actions.get(self.path)
            if action is not None:
                threading.Thread(target=action, daemon=True).start()
                self._send_json(202, {"status": "started"})
                return
            vscode_prefix = "/api/vscode/"
            vscode_operation = self.path[len(vscode_prefix):] if self.path.startswith(vscode_prefix) else ""
            if vscode_operation in {"install", "enable", "pause", "uninstall", "clear-cache"}:
                if vscode_action is None:
                    self._send_json(503, {"error": "vscode_control_unavailable"})
                    return
                try:
                    vscode_action(vscode_operation)
                except (OSError, RuntimeError, TypeError, ValueError) as exc:
                    self._send_json(400, {"error": "vscode_control_failed", "message": str(exc)})
                    return
                self.send_response(204)
                self.end_headers()
                return
            if self.path == "/api/processes/consent":
                if process_consent_decision is None:
                    self._send_json(503, {"error": "process_consent_unavailable"})
                    return
                try:
                    process_consent_decision(self._read_body())
                except (OSError, TypeError, ValueError) as exc:
                    self._send_json(400, {"error": "process_consent_failed", "message": str(exc)})
                    return
                self.send_response(204)
                self.end_headers()
                return
            if self.path.startswith("/api/processes/consent/"):
                if process_consent_reset is None:
                    self._send_json(503, {"error": "process_consent_unavailable"})
                    return
                identity_key = self.path.rsplit("/", 1)[-1]
                try:
                    process_consent_reset(identity_key)
                except (OSError, TypeError, ValueError) as exc:
                    self._send_json(400, {"error": "process_consent_reset_failed", "message": str(exc)})
                    return
                self.send_response(204)
                self.end_headers()
                return
            if self.path == "/api/projects/scan":
                try:
                    body = self._read_body()
                    result = discover_projects(body.get("path", ""))
                    self._send_json(200, {"projects": [item.to_dict() for item in result.projects], "truncated": result.truncated})
                except (OSError, ValueError) as exc:
                    self._send_json(400, {"error": str(exc)})
                return
            prefix = "/api/projects/"
            operation = self.path[len(prefix):] if self.path.startswith(prefix) else ""
            if operation in {"add", "pause", "resume", "remove"} and project_registry is not None:
                try:
                    body = self._read_body()
                    snapshot = project_registry.mutate(int(body.get("revision", -1)), operation, body.get("path", ""), body.get("vcs"))
                    self._send_json(200, self._snapshot(snapshot))
                except RevisionConflict as exc:
                    self._send_json(409, {"error": "revision_conflict", "reason": str(exc)})
                except (OSError, TypeError, ValueError) as exc:
                    self._send_json(400, {"error": str(exc)})
                return
            self._send_json(404, {"error": "not_found"})

    preferred = max(0, int(port))
    try:
        return ThreadingHTTPServer(("127.0.0.1", preferred), LocalViewHandler)
    except OSError:
        if preferred == 0:
            raise
        return ThreadingHTTPServer(("127.0.0.1", 0), LocalViewHandler)


def _local_workspace_html() -> str:
    html = _local_workspace_html_base()
    html = html.replace(
        "${escapeHtml(project.state)}",
        "${project.available===false?'目录不存在':escapeHtml(project.state)}",
    )
    return html.replace(
        '<button onclick="mutate(\'${project.state===',
        '<button ${project.available===false?\'hidden\':\'\'} onclick="mutate(\'${project.state===',
    )


def _local_workspace_html_base() -> str:
    return """<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Aetheris Local Workspace</title>
<style>
:root{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;color:#203234;background:#eef3f2}*{box-sizing:border-box}body{margin:0;font-size:14px}header{padding:18px 28px;background:#173f43;color:#fff;font-size:16px}main{max-width:1240px;margin:0 auto;padding:24px 28px}nav{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:24px}button,select,input{font:inherit}button{padding:8px 12px;border:1px solid #9db4b1;background:#fff;color:#173f43;border-radius:4px;cursor:pointer}button:hover{background:#edf6f4}button.primary{background:#14766d;color:#fff;border-color:#14766d}button.primary:hover{background:#0e625b}button.danger{color:#9b2c2c;border-color:#d9a5a5}.muted{color:#60706f}.toolbar,.section-head,.process-toolbar,.batch-bar{display:flex;align-items:center;flex-wrap:wrap;gap:10px;margin-bottom:14px}.section-head{justify-content:space-between}.section-head h2{margin:0 0 4px}.section-head p{margin:0}.toolbar input,.process-toolbar input,.process-toolbar select{padding:8px 10px;border:1px solid #aebdbc;background:#fff;border-radius:4px}.toolbar input{width:min(620px,70%)}table{width:100%;border-collapse:collapse;background:#fff}th,td{padding:11px 12px;border-bottom:1px solid #dfe8e6;text-align:left;vertical-align:middle}th{font-weight:600;color:#425655;background:#f8fbfa}pre{background:#fff;border:1px solid #d8e1e0;padding:12px;overflow:auto;max-height:300px}.stat-grid{display:grid;grid-template-columns:repeat(5,minmax(120px,1fr));gap:10px;margin:18px 0}.stat{background:#fff;border:1px solid #d8e4e2;padding:12px 14px;border-radius:4px}.stat strong{display:block;font-size:22px;color:#173f43}.stat span{color:#60706f}.process-toolbar{padding:12px;background:#f8fbfa;border:1px solid #d8e4e2;border-radius:4px}.process-toolbar label{display:inline-flex;align-items:center;gap:6px}.process-toolbar input[type=search]{min-width:240px}.batch-bar{padding:10px 12px;background:#e5f1ef;border:1px solid #b9d4d0;border-radius:4px}.batch-bar strong{margin-right:4px}.table-shell{overflow:auto;border:1px solid #d8e4e2;border-radius:4px}.process-table{min-width:980px;table-layout:fixed}.process-table th:nth-child(1){width:42px}.process-table th:nth-child(2){width:28%}.process-table th:nth-child(3){width:19%}.process-table th:nth-child(4){width:13%}.process-table th:nth-child(5){width:40%}.process-name strong,.process-name small{display:block}.process-name small{margin-top:3px;color:#71807f;word-break:break-all}.process-actions{display:flex;flex-wrap:wrap;gap:6px}.process-actions button{padding:6px 8px;font-size:13px;white-space:nowrap}.state{display:inline-block;padding:4px 8px;border-radius:12px;background:#edf1f0;color:#425655}.state-pending{background:#fff4d6;color:#856404}.state-allow_global,.state-allow_project{background:#e2f4ed;color:#176348}.state-deny,.state-always_ignore{background:#f8e8e8;color:#8e3030}article{background:#fff;border:1px solid #d8e4e2;border-radius:4px;padding:14px;margin-bottom:10px}article h3{margin:0 0 8px}@media(max-width:760px){main{padding:18px 14px}.stat-grid{grid-template-columns:repeat(2,1fr)}.toolbar input{width:100%}.process-toolbar input[type=search]{min-width:0;width:100%}}
</style><script>
async function loadVSCode(){
  try{
    const responses=await Promise.all([fetch('/api/vscode/status'),fetch('/api/vscode/events')]);
    const status=await responses[0].json();
    const events=(await responses[1].json()).events||[];
    const labels={active:'运行中',awaiting_activation:'等待 VS Code 激活',paused_by_user:'已停用',not_installed:'未安装',pending_install:'等待安装',error:'异常'};
    const values=[['组件状态',labels[status.component_state||status.state]||status.component_state||status.state||'未知'],['扩展版本',status.component_version||status.extension_version||'未上报'],['待发送',status.pending_events||0],['已发送',status.sent_events||0],['最后心跳',status.last_component_heartbeat_at||'暂无']];
    document.getElementById('vscodeStats').innerHTML=values.map(item=>`<div class="stat"><strong>${escapeHtml(item[1])}</strong><span>${escapeHtml(item[0])}</span></div>`).join('');
    document.getElementById('vscodeRows').innerHTML=events.map(event=>`<tr><td>${escapeHtml(event.occurred_at||'')}</td><td>${escapeHtml(event.event_type||'')}</td><td>${escapeHtml(event.project_name||event.project_id||'未归属')}</td><td>${escapeHtml(event.attributes?.language_id||'')}</td></tr>`).join('')||'<tr><td colspan="4">暂无 VS Code 行为记录</td></tr>';
  }catch(error){document.getElementById('vscodeRows').innerHTML='<tr><td colspan="4">VS Code 状态读取失败</td></tr>'}
}
async function vscodeAction(name){
  const response=await fetch('/api/vscode/'+name,{method:'POST',headers,body:'{}'});
  if(!response.ok){const data=await response.json();alert(data.message||data.error||'VS Code 操作失败');return}
  await loadVSCode();
}
document.addEventListener('DOMContentLoaded',()=>{
  document.getElementById('processFilter').insertAdjacentHTML('beforeend','<option value="processed">已处理</option>');
  visibleProcessRecords=function(){const filter=document.getElementById('processFilter').value;const query=document.getElementById('processSearch').value.trim().toLowerCase();return processRecords.filter(process=>{const stateMatch=filter==='all'||(filter==='processed'&&process.state!=='pending')||process.state===filter;const value=(process.name+' '+(process.publisher||'')+' '+(process.executable||'')).toLowerCase();return stateMatch&&(!query||value.includes(query))})};
});
</script></head>
<body><header><strong>Aetheris Lens</strong> · 本地工作区</header><main><nav><button onclick="show('projects')">项目</button><button onclick="show('episodes');loadEpisodes()">个人工作片段</button><button onclick="show('vscode');loadVSCode()">VS Code 采集</button><button onclick="show('status')">状态</button><button onclick="show('events')">事件</button><button onclick="show('processes');loadProcesses()">进程授权</button></nav>
<section id="projects"><h2>项目管理</h2><div class="toolbar"><input id="path" placeholder="输入本机项目或扫描目录"><button class="primary" onclick="addProject()">添加</button><button onclick="scanProjects()">扫描</button></div><table><thead><tr><th>目录</th><th>类型</th><th>状态</th><th>操作</th></tr></thead><tbody id="projectRows"></tbody></table></section>
<section id="status" hidden><h2>运行状态</h2><p class="muted">应用 OCR 保底仅处理已授权、无专属数据的前台工作进程。</p><button onclick="action('heartbeat')">立即 heartbeat</button> <button onclick="action('capture')">立即采集</button><pre id="statusText"></pre></section>
<section id="events" hidden><h2>最近事件</h2><pre id="eventsText"></pre></section>
<section id="episodes" hidden><h2>个人工作片段</h2><div id="episodeRows"></div></section>
<section id="vscode" hidden><div class="section-head"><div><h2>VS Code 采集</h2><p class="muted">仅记录授权项目内的文件行为和扩展状态，不采集代码正文。</p></div><button onclick="loadVSCode()">刷新</button></div><div class="toolbar"><button class="primary" onclick="vscodeAction('install')">安装扩展</button><button onclick="vscodeAction('enable')">启用</button><button onclick="vscodeAction('pause')">停用</button><button class="danger" onclick="vscodeAction('uninstall')">卸载</button><button onclick="vscodeAction('clear-cache')">清除缓存</button></div><div class="stat-grid" id="vscodeStats"></div><h3>最近行为</h3><div class="table-shell"><table><thead><tr><th>时间</th><th>行为</th><th>项目</th><th>文件类型</th></tr></thead><tbody id="vscodeRows"></tbody></table></div></section>
<section id="processes" hidden><div class="section-head"><div><h2>进程授权</h2><p class="muted">进程身份由名称和可执行路径共同确定；相同身份只保留一条记录。</p></div><button onclick="loadProcesses()">刷新</button></div><div id="processStats" class="stat-grid"></div><div class="process-toolbar"><label>状态<select id="processFilter" onchange="renderProcesses()"><option value="all">全部</option><option value="pending">待确认</option><option value="processed">已处理</option><option value="allow_global">工作相关</option><option value="allow_project">仅当前项目</option><option value="deny">不监控</option><option value="always_ignore">永久忽略</option></select></label><input id="processSearch" type="search" placeholder="搜索进程名、路径或发布者" oninput="renderProcesses()"><label><input id="selectAllProcesses" type="checkbox" onchange="toggleAllProcesses(this.checked)">全选当前结果</label><span id="selectedCount" class="muted">已选 0 条</span></div><div class="batch-bar"><strong>批量操作：</strong><button class="primary" onclick="batchProcessDecision('allow_global')">批量标记为工作相关</button><button onclick="batchProcessDecision('allow_project')">批量仅当前项目</button><button onclick="batchProcessDecision('deny')">批量不监控</button><button class="danger" onclick="batchProcessDecision('always_ignore')">批量永久忽略</button><button onclick="batchProcessDecision('reset')">批量重置</button></div><div class="table-shell"><table class="process-table"><thead><tr><th><input type="checkbox" aria-label="全选当前结果" onchange="toggleAllProcesses(this.checked)"></th><th>进程</th><th>发布者</th><th>当前决定</th><th>操作</th></tr></thead><tbody id="processRows"></tbody></table></div></section></main>
<script>let revision=0;let processRecords=[];const selectedProcessKeys=new Set();const headers={'Content-Type':'application/json','X-Aetheris-Control':'local-lens'};const consentStateLabels={pending:'待确认',allow_global:'工作相关',allow_project:'仅当前项目',deny:'不监控',always_ignore:'永久忽略'};const pathInput=()=>document.getElementById('path');function show(id){for(const section of document.querySelectorAll('section'))section.hidden=section.id!==id}function escapeHtml(value){const element=document.createElement('span');element.textContent=value??'';return element.innerHTML}function js(value){return encodeURIComponent(value??'')}function formatTime(value){return value?new Date(value*1000).toLocaleString('zh-CN'):'未设置'}async function mutate(operation,value,encoded=false){const path=encoded?decodeURIComponent(value):value;const response=await fetch('/api/projects/'+operation,{method:'POST',headers,body:JSON.stringify({revision,path})});if(!response.ok){const data=await response.json();alert(data.reason||data.error||'项目操作失败');return}render(await response.json())}function render(data){revision=data.revision;document.getElementById('projectRows').innerHTML=(data.projects||[]).map(project=>`<tr><td>${escapeHtml(project.path)}</td><td>${escapeHtml(project.vcs)}</td><td>${escapeHtml(project.state)}</td><td><button onclick="mutate('${project.state==='paused'?'resume':'pause'}','${js(project.path)}',true)">${project.state==='paused'?'恢复':'暂停'}</button><button onclick="mutate('remove','${js(project.path)}',true)">移除</button></td></tr>`).join('')||'<tr><td colspan="4">暂无授权项目</td></tr>'}async function addProject(){const value=pathInput().value.trim();if(value)await mutate('add',value)}async function scanProjects(){const value=pathInput().value.trim();if(!value)return;const response=await fetch('/api/projects/scan',{method:'POST',headers,body:JSON.stringify({path:value})});const data=await response.json();if(!response.ok){alert(data.reason||data.error||'项目扫描失败');return}for(const project of data.projects||[])await mutate('add',project.path)}async function action(name){const response=await fetch('/api/actions/'+name,{method:'POST',headers});if(!response.ok)alert('操作失败')}async function loadEpisodes(){const response=await fetch('/api/work-episodes');const data=await response.json();document.getElementById('episodeRows').innerHTML=(data.episodes||[]).map(episode=>`<article><h3>${escapeHtml(episode.objective||'未识别目标')}</h3><p>设备：${escapeHtml(episode.device_id||'当前设备')} · 项目：${escapeHtml(episode.project_name||episode.project_id||'未归属')}</p><p>事件数：${episode.event_count||0} · 动作：${(episode.actions||[]).map(item=>escapeHtml(item.summary)).join('、')||'暂无'}</p><p>证据：${(episode.event_ids||[]).slice(0,12).map(id=>`<code>${escapeHtml(id)}</code>`).join(' ')}${(episode.event_ids||[]).length>12?' …':''}</p></article>`).join('')||'<p class="muted">暂无可解释个人工作片段</p>'}function visibleProcessRecords(){const filter=document.getElementById('processFilter').value;const query=document.getElementById('processSearch').value.trim().toLowerCase();return processRecords.filter(process=>{const stateMatch=filter==='all'||filter==='processed'&&process.state!=='pending'||process.state===filter;const text=(process.name+' '+(process.publisher||'')+' '+(process.executable||'')).toLowerCase();return stateMatch&&(!query||text.includes(query))})}function renderProcessStats(){const counts={pending:0,allow_global:0,allow_project:0,deny:0,always_ignore:0};for(const process of processRecords)counts[process.state]=(counts[process.state]||0)+1;document.getElementById('processStats').innerHTML=Object.entries({pending:'待确认',allow_global:'工作相关',allow_project:'仅当前项目',deny:'不监控',always_ignore:'永久忽略'}).map(([state,label])=>`<div class="stat"><strong>${counts[state]||0}</strong><span>${label}</span></div>`).join('')}function renderProcesses(){const rows=visibleProcessRecords();document.getElementById('processRows').innerHTML=rows.map(process=>`<tr><td><input class="process-check" type="checkbox" data-identity="${process.identity_key}" ${selectedProcessKeys.has(process.identity_key)?'checked':''} aria-label="选择 ${escapeHtml(process.name)}"></td><td class="process-name"><strong>${escapeHtml(process.name)}</strong><small>${escapeHtml(process.executable||'路径未识别')}</small></td><td>${escapeHtml(process.publisher||'未知')}</td><td class="process-state"><span class="state state-${escapeHtml(process.state||'')}">${escapeHtml(consentStateLabels[process.state]||process.state||'')}</span><small>${process.state==='pending'?'首次发现 '+formatTime(process.first_seen):'决定于 '+formatTime(process.decided_at)}</small></td><td class="process-actions"><button data-process-action="allow_global" data-identity="${process.identity_key}">工作相关</button><button data-process-action="allow_project" data-identity="${process.identity_key}">仅当前项目</button><button data-process-action="deny" data-identity="${process.identity_key}">不监控</button><button class="danger" data-process-action="always_ignore" data-identity="${process.identity_key}">永久忽略</button><button data-process-action="reset" data-identity="${process.identity_key}">重置</button></td></tr>`).join('')||'<tr><td colspan="5">当前筛选条件下没有进程记录</td></tr>';const selected=rows.filter(process=>selectedProcessKeys.has(process.identity_key)).length;document.getElementById('selectedCount').textContent='已选 '+selected+' 条';document.getElementById('selectAllProcesses').checked=rows.length>0&&selected===rows.length}function toggleAllProcesses(checked){for(const process of visibleProcessRecords()){if(checked)selectedProcessKeys.add(process.identity_key);else selectedProcessKeys.delete(process.identity_key)}renderProcesses()}document.getElementById('processRows').addEventListener('change',event=>{if(!event.target.classList.contains('process-check'))return;const key=event.target.dataset.identity;if(event.target.checked)selectedProcessKeys.add(key);else selectedProcessKeys.delete(key);renderProcesses()});document.getElementById('processRows').addEventListener('click',event=>{const button=event.target.closest('button[data-process-action]');if(!button)return;const key=button.dataset.identity;const decision=button.dataset.processAction;if(decision==='reset')resetProcess(key);else decideProcess(key,decision)});async function loadProcesses(){const response=await fetch('/api/processes/pending');const data=await response.json();processRecords=data.processes||[];renderProcessStats();renderProcesses()}async function projectIdsForDecision(decision){if(decision!=='allow_project')return[];const response=await fetch('/api/projects');const data=await response.json();const current=(data.projects||[]).find(project=>project.state==='active'&&project.vcs!=='pending');if(!current)throw new Error('当前没有可授权项目');return[current.local_project_id||current.path]}async function submitProcessDecision(identityKey,decision,projectIds){const response=await fetch('/api/processes/consent',{method:'POST',headers,body:JSON.stringify({identity_key:identityKey,decision,project_ids:projectIds})});if(!response.ok){const data=await response.json();throw new Error(data.message||data.reason||data.error||'进程授权失败')}}async function decideProcess(identityKey,decision){try{await submitProcessDecision(identityKey,decision,await projectIdsForDecision(decision));selectedProcessKeys.delete(identityKey);await loadProcesses()}catch(error){alert(error.message)}}async function resetProcess(identityKey){try{const response=await fetch('/api/processes/consent/'+encodeURIComponent(identityKey),{method:'POST',headers});if(!response.ok){const data=await response.json();throw new Error(data.message||data.reason||data.error||'进程重置失败')}selectedProcessKeys.delete(identityKey);await loadProcesses()}catch(error){alert(error.message)}}async function batchProcessDecision(decision){const keys=Array.from(selectedProcessKeys);if(!keys.length){alert('请先选择进程');return}try{const projectIds=decision==='reset'?[]:await projectIdsForDecision(decision);const failures=[];for(const key of keys){try{if(decision==='reset'){const response=await fetch('/api/processes/consent/'+encodeURIComponent(key),{method:'POST',headers});if(!response.ok)throw new Error('重置失败')}else await submitProcessDecision(key,decision,projectIds)}catch(error){failures.push(key)}}selectedProcessKeys.clear();await loadProcesses();if(failures.length)alert('有 '+failures.length+' 条记录操作失败，请重试')}catch(error){alert(error.message)}}async function load(){try{const responses=await Promise.all([fetch('/api/projects'),fetch('/api/status'),fetch('/api/events')]);if(responses[0].ok)render(await responses[0].json());document.getElementById('statusText').textContent=JSON.stringify(await responses[1].json(),null,2);document.getElementById('eventsText').textContent=JSON.stringify(await responses[2].json(),null,2)}catch(error){document.getElementById('statusText').textContent='本地 Lens 暂时无法读取状态：'+error}}load();</script></body></html>"""
