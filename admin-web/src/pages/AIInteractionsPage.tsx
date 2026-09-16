import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AIInteractionsResult, AIMessageRole, EventDetail } from '../api/types'

const roleLabels: Record<AIMessageRole, string> = { user: '用户发送', assistant: 'AI 回复', ai_tool: 'AI 工具执行', system: '系统消息', tool: '工具消息', unknown: '未识别' }
const roleOrder: AIMessageRole[] = ['user', 'assistant', 'ai_tool', 'system', 'tool', 'unknown']

function localDate(value: Date) {
  const year = value.getFullYear()
  const month = String(value.getMonth() + 1).padStart(2, '0')
  const day = String(value.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

function initialRange() {
  const to = new Date()
  const from = new Date(to)
  from.setDate(to.getDate() - 29)
  return { from: localDate(from), to: localDate(to) }
}

export default function AIInteractionsPage() {
  const initial = initialRange()
  const [from, setFrom] = useState(initial.from)
  const [to, setTo] = useState(initial.to)
  const [deviceID, setDeviceID] = useState('')
  const [projectID, setProjectID] = useState('')
  const [messageRole, setMessageRole] = useState<AIMessageRole | ''>('')
  const [offset, setOffset] = useState(0)
  const [data, setData] = useState<AIInteractionsResult | null>(null)
  const [detail, setDetail] = useState<EventDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    setLoading(true); setError('')
    api.listAIInteractions({ from, to, device_id: deviceID, project_id: projectID, message_role: messageRole || undefined, limit: 50, offset })
      .then(result => { if (active) setData(result) })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : 'AI 交互数据加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [from, to, deviceID, projectID, messageRole, offset])

  async function openDetail(eventID: string) {
    try { setDetail(await api.getEvent(eventID)) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '交互详情加载失败') }
  }

  const limit = data?.limit || 50
  const fullContent = typeof detail?.payload?.content === 'string' ? detail.payload.content : ''
  return <section>
    <div className="page-title"><div><p className="eyebrow">可追溯会话</p><h2>AI 交互</h2><p className="muted">按设备、项目和消息角色检索脱敏交互记录</p></div></div>
    <div className="toolbar ai-toolbar">
      <label>开始日期<input aria-label="开始日期" type="date" value={from} max={to} onChange={event => { setFrom(event.target.value); setOffset(0) }} /></label>
      <label>结束日期<input aria-label="结束日期" type="date" value={to} min={from} onChange={event => { setTo(event.target.value); setOffset(0) }} /></label>
      <span className="range-label">共 {data?.total || 0} 条匹配交互</span>
    </div>
    {error && <div className="state error-state"><strong>加载失败</strong><span>{error}</span></div>}
    <div className="ai-dimensions">
      <DimensionPanel title="1. 设备" empty="当前日期范围暂无设备">
        <DimensionButton active={!deviceID} label="全部设备" count={data?.devices.reduce((sum, item) => sum + item.event_count, 0) || 0} onClick={() => { setDeviceID(''); setProjectID(''); setOffset(0) }} />
        {data?.devices.map(device => <DimensionButton key={device.device_id} active={deviceID === device.device_id} label={device.device_name || device.device_id} detail={device.device_id} count={device.event_count} onClick={() => { setDeviceID(device.device_id); setProjectID(''); setOffset(0) }} />)}
      </DimensionPanel>
      <DimensionPanel title="2. 项目" empty="当前设备暂无项目">
        <DimensionButton active={!projectID} label="全部项目" count={data?.projects.reduce((sum, item) => sum + item.event_count, 0) || 0} onClick={() => { setProjectID(''); setOffset(0) }} />
        {data?.projects.map(project => <DimensionButton key={project.project_id} active={projectID === project.project_id} label={project.project_name} detail={project.project_id} count={project.event_count} onClick={() => { setProjectID(project.project_id); setOffset(0) }} />)}
      </DimensionPanel>
      <DimensionPanel title="3. 消息角色" empty="暂无角色统计">
        <DimensionButton active={!messageRole} label="全部角色" count={Object.values(data?.role_counts || {}).reduce((sum, value) => sum + value, 0)} onClick={() => { setMessageRole(''); setOffset(0) }} />
        {roleOrder.map(role => <DimensionButton key={role} active={messageRole === role} label={roleLabels[role]} count={data?.role_counts[role] || 0} onClick={() => { setMessageRole(role); setOffset(0) }} />)}
      </DimensionPanel>
    </div>
    <div className="panel ai-interactions-panel">
      <div className="panel-heading"><h3>交互记录</h3><span className="muted">服务端分页，只展示脱敏内容</span></div>
      {loading ? <div className="state">正在加载 AI 交互...</div> : !data?.interactions.length ? <div className="state empty">当前条件暂无交互</div> : <div className="table-wrap"><table><thead><tr><th>时间</th><th>角色</th><th>设备 / 项目</th><th>来源</th><th>脱敏摘要 / 证据</th><th>操作</th></tr></thead><tbody>{data.interactions.map(item => <tr key={item.event_id}><td>{new Date(item.occurred_at).toLocaleString('zh-CN')}</td><td><span className={`message-role role-${item.message_role}`}>{roleLabels[item.message_role]}</span></td><td><strong>{item.device_name}</strong><small>{item.project_name}</small></td><td>{item.tool}</td><td className="content-preview">{item.content_preview || '无可展示内容'}{item.source_event_ids?.map(id => <small key={id}>{id}</small>)}</td><td>{item.message_role === 'ai_tool' ? <span className="muted">仅安全摘要</span> : <button className="secondary" onClick={() => openDetail(item.event_id)}>查看完整内容</button>}</td></tr>)}</tbody></table></div>}
      <div className="pagination"><button className="secondary" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - limit))}>上一页</button><span>{data ? Math.floor(offset / limit) + 1 : 1} / {data ? Math.max(1, Math.ceil(data.total / limit)) : 1}</span><button className="secondary" disabled={loading || !data || offset + limit >= data.total} onClick={() => setOffset(offset + limit)}>下一页</button></div>
    </div>
    {detail && <div className="drawer-backdrop" role="presentation" onClick={() => setDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">AI 交互详情</p><h3>{roleLabels[(String(detail.payload?.role || 'unknown') as AIMessageRole)] || '未识别'}</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setDetail(null)}>×</button></div><div className="detail-list"><div><span>事件 ID</span><strong>{detail.event_id}</strong></div><div><span>发生时间</span><strong>{new Date(detail.occurred_at).toLocaleString('zh-CN')}</strong></div><div><span>完整脱敏内容</span><pre>{fullContent || '无可展示内容'}</pre></div></div></aside></div>}
  </section>
}

function DimensionPanel({ title, empty, children }: { title: string; empty: string; children: React.ReactNode }) {
  return <div className="dimension-panel"><div className="panel-heading"><h3>{title}</h3></div><div className="dimension-list">{children || <span className="muted">{empty}</span>}</div></div>
}

function DimensionButton({ active, label, detail, count, onClick }: { active: boolean; label: string; detail?: string; count: number; onClick: () => void }) {
  return <button className={active ? 'dimension-option active' : 'dimension-option'} onClick={onClick}><span><strong>{label}</strong>{detail && <small>{detail}</small>}</span><b>{count}</b></button>
}
