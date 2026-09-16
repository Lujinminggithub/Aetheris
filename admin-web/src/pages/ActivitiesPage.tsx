import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { ActivitiesResult, ActivityType, EventDetail } from '../api/types'

const activityLabels: Record<ActivityType, string> = { ai: 'AI 协作', terminal: '终端', ide: 'IDE', delivery: '交付', application: '应用活动', browser: '浏览器', version_control: '版本控制', other: '其他' }
const activityTypes = Object.keys(activityLabels) as ActivityType[]
const roleLabels: Record<string, string> = { user: '用户发送', assistant: 'AI 回复', tool: 'AI 工具执行', system: '系统消息', unknown: '未识别' }

function localDate(value: Date) { return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}` }
function initialRange() { const to = new Date(); const from = new Date(to); from.setDate(to.getDate() - 29); return { from: localDate(from), to: localDate(to) } }

export default function ActivitiesPage() {
  const initial = initialRange()
  const [from, setFrom] = useState(initial.from); const [to, setTo] = useState(initial.to)
  const [deviceID, setDeviceID] = useState(''); const [projectID, setProjectID] = useState('')
  const [activityType, setActivityType] = useState<ActivityType | ''>(''); const [messageRole, setMessageRole] = useState('')
  const [offset, setOffset] = useState(0); const [data, setData] = useState<ActivitiesResult | null>(null)
  const [detail, setDetail] = useState<EventDetail | null>(null); const [loading, setLoading] = useState(true); const [error, setError] = useState('')

  useEffect(() => {
    let active = true; setLoading(true); setError('')
    api.listActivities({ from, to, device_id: deviceID, project_id: projectID, activity_type: activityType || undefined, message_role: messageRole || undefined, limit: 50, offset })
      .then(result => { if (active) setData(result) })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '活动记录加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [from, to, deviceID, projectID, activityType, messageRole, offset])

  async function openEvidence(eventID: string) {
    setError('')
    try { setDetail(await api.getEvent(eventID)) } catch (reason) { setError(reason instanceof Error ? reason.message : '活动证据加载失败') }
  }

  const limit = data?.limit || 50
  return <section>
    <div className="page-title"><div><p className="eyebrow">统一事实视图</p><h2>活动记录</h2><p className="muted">按设备、项目和活动类型检索规范记录</p></div></div>
    <div className="toolbar activity-toolbar">
      <label>开始日期<input type="date" value={from} max={to} onChange={event => { setFrom(event.target.value); setOffset(0) }} /></label>
      <label>结束日期<input type="date" value={to} min={from} onChange={event => { setTo(event.target.value); setOffset(0) }} /></label>
      <label>设备<select value={deviceID} onChange={event => { setDeviceID(event.target.value); setProjectID(''); setOffset(0) }}><option value="">全部设备</option>{data?.devices.map(item => <option value={item.id} key={item.id}>{item.label} ({item.count})</option>)}</select></label>
      <label>项目<select value={projectID} onChange={event => { setProjectID(event.target.value); setOffset(0) }}><option value="">全部项目</option>{data?.projects.map(item => <option value={item.id} key={item.id}>{item.label} ({item.count})</option>)}</select></label>
    </div>
    <div className="activity-type-strip">
      <button className={!activityType ? 'active' : ''} onClick={() => { setActivityType(''); setOffset(0) }}>全部 <b>{data?.total || 0}</b></button>
      {activityTypes.map(type => <button className={activityType === type ? 'active' : ''} key={type} onClick={() => { setActivityType(type); setOffset(0) }}>{activityLabels[type]} <b>{data?.activity_counts[type] || 0}</b></button>)}
    </div>
    <div className="toolbar activity-role-filter"><label>AI 消息角色<select value={messageRole} onChange={event => { setMessageRole(event.target.value); setOffset(0) }}><option value="">全部角色</option><option value="user">用户发送</option><option value="assistant">AI 回复</option><option value="tool">AI 工具执行</option><option value="system">系统消息</option></select></label><span className="range-label">共 {data?.total || 0} 条规范活动</span></div>
    {error && <div className="state error-state"><strong>加载失败</strong><span>{error}</span></div>}
    <div className="panel">
      {loading ? <div className="state">正在加载活动记录...</div> : !data?.activities.length ? <div className="state empty">当前条件暂无活动</div> : <div className="table-wrap"><table><thead><tr><th>时间</th><th>活动</th><th>设备 / 项目</th><th>参与方</th><th>安全摘要</th><th>操作</th></tr></thead><tbody>{data.activities.map(item => <tr key={item.fact_id}><td>{new Date(item.occurred_at).toLocaleString('zh-CN')}</td><td><strong>{activityLabels[item.activity_type]}</strong><small>{item.event_type}</small></td><td>{item.device_name}<small>{item.project_name}</small></td><td>{item.activity_type === 'ai' ? roleLabels[item.message_role] || item.message_role : item.actor_origin}</td><td className="content-preview">{item.preview || '-'}</td><td><button className="secondary" onClick={() => openEvidence(item.canonical_event_id)}>查看证据</button></td></tr>)}</tbody></table></div>}
      <div className="pagination"><button className="secondary" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - limit))}>上一页</button><span>{data ? Math.floor(offset / limit) + 1 : 1} / {data ? Math.max(1, Math.ceil(data.total / limit)) : 1}</span><button className="secondary" disabled={loading || !data || offset + limit >= data.total} onClick={() => setOffset(offset + limit)}>下一页</button></div>
    </div>
    {detail && <div className="drawer-backdrop" role="presentation" onClick={() => setDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">事实证据链</p><h3>活动证据</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setDetail(null)}>×</button></div><div className="detail-list"><div><span>事件 ID</span><strong>{detail.event_id}</strong></div><div><span>事件类型</span><strong>{detail.event_type}</strong></div><div><span>发生时间</span><strong>{new Date(detail.occurred_at).toLocaleString('zh-CN')}</strong></div>{detail.payload && <div><span>脱敏 payload</span><pre>{JSON.stringify(detail.payload, null, 2)}</pre></div>}</div></aside></div>}
  </section>
}
