import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AdapterHealthSnapshot } from '../api/types'

const stateLabels: Record<string, string> = {
  active: '运行中', awaiting_activation: '等待激活', paused_by_user: '用户已停用', install_declined: '用户未安装',
  not_installed: '未安装', pending_install: '等待安装', bridge_offline: '桥接离线', unsupported_remote_host: '远程主机不支持', incompatible: '版本不兼容', error: '组件故障',
}

export default function VSCodeIntegrationPage() {
  const [items, setItems] = useState<AdapterHealthSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  async function load() {
    setLoading(true); setError('')
    try { setItems((await api.listAdapterHealth()).items.filter(item => item.adapter_id === 'vscode_extension')) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '组件状态加载失败') }
    finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])
  return <section>
    <div className="page-title"><div><p className="eyebrow">组件管理</p><h2>VS Code 采集组件</h2><p className="muted">按设备查看扩展版本、桥接心跳和待发送缓存。用户停用不等同于组件故障。</p></div><button className="secondary" onClick={load}>刷新</button></div>
    {error && <div className="state error-state">{error}</div>}
    {loading ? <div className="state">正在加载 VS Code 组件状态...</div> : !items.length ? <div className="state empty">暂无 VS Code 组件心跳</div> : <div className="panel"><div className="table-wrap"><table><thead><tr><th>设备</th><th>组件状态</th><th>扩展版本</th><th>协议</th><th>VS Code</th><th>最近心跳</th><th>待发送</th><th>已发送</th><th>丢弃</th></tr></thead><tbody>{items.map(item => { const state = item.component_state || item.state; const fault = ['error', 'bridge_offline', 'incompatible'].includes(state); return <tr key={`${item.device_id}-${item.adapter_id}`}><td><strong>{item.device_id}</strong></td><td><span className={`status ${fault ? 'status-down' : state === 'active' ? 'status-healthy' : 'status-degraded'}`}>{stateLabels[state] || state}</span></td><td>{item.component_version || '-'}</td><td>{item.protocol_version || '-'}</td><td>{item.vscode_version || '-'}</td><td>{item.last_component_heartbeat_at ? new Date(item.last_component_heartbeat_at).toLocaleString('zh-CN') : '暂无'}</td><td>{item.pending_events ?? 0}</td><td>{item.sent_events ?? 0}</td><td>{item.dropped_events ?? 0}</td></tr> })}</tbody></table></div></div>}
  </section>
}
