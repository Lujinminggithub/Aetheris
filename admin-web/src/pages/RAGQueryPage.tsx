import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { ActivitiesResult, ActivityType, EventDetail, LogicalProject, RAGQueryJob, RAGStatus } from '../api/types'

const typeLabels: Record<ActivityType, string> = { ai: 'AI 协作', terminal: '终端', ide: 'IDE', delivery: '交付', application: '应用活动', browser: '浏览器', version_control: '版本控制', other: '其他' }
const stageLabels: Record<string, string> = { queued: '等待处理', embedding: '理解问题', retrieving: '检索证据', generating: '生成回答', completed: '查询完成', failed: '查询失败' }
function localDate(value: Date) { return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}` }
function initialRange() { const to = new Date(); const from = new Date(to); from.setDate(to.getDate() - 29); return { from: localDate(from), to: localDate(to) } }

export default function RAGQueryPage() {
  const initial = initialRange(); const [from, setFrom] = useState(initial.from); const [to, setTo] = useState(initial.to)
  const [deviceID, setDeviceID] = useState(''); const [projectID, setProjectID] = useState(''); const [activityType, setActivityType] = useState<ActivityType | ''>('')
  const [question, setQuestion] = useState(''); const [status, setStatus] = useState<RAGStatus | null>(null); const [facets, setFacets] = useState<ActivitiesResult | null>(null); const [projects, setProjects] = useState<LogicalProject[]>([])
  const [job, setJob] = useState<RAGQueryJob | null>(null); const [submitting, setSubmitting] = useState(false); const [error, setError] = useState(''); const [detail, setDetail] = useState<EventDetail | null>(null); const [evidenceOpen, setEvidenceOpen] = useState(false)
  const pollTimer = useRef<number | null>(null)

  useEffect(() => { let active = true; Promise.all([api.getRAGStatus(), api.listActivities({ from, to, limit: 1, offset: 0 }), api.listProjects()]).then(([nextStatus, nextFacets, projectResult]) => { if (active) { setStatus(nextStatus); setFacets(nextFacets); setProjects(projectResult.projects) } }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '检索状态加载失败') }); return () => { active = false } }, [from, to])
  useEffect(() => () => { if (pollTimer.current !== null) window.clearTimeout(pollTimer.current) }, [])

  async function poll(queryID: string) {
    try {
      const next = await api.getRAGQuery(queryID); setJob(next)
      if (next.status !== 'completed' && next.status !== 'failed') pollTimer.current = window.setTimeout(() => poll(queryID), 1000)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '查询状态加载失败') }
  }

  async function submit() {
    setSubmitting(true); setError(''); setJob(null); setEvidenceOpen(false)
    try {
      const filters = { from, to, device_id: deviceID || undefined, logical_project_id: projectID !== '__all__' ? projectID : undefined, all_projects: projectID === '__all__', knowledge_scope: 'project_process' as const, activity_type: activityType || undefined }
      const created = await api.createRAGQuery({ question: question.trim(), filters })
      setJob({ query_id: created.query_id, question, filters, status: created.status as RAGQueryJob['status'], progress: created.progress, answer: '', citations: [], created_at: new Date().toISOString() })
      await poll(created.query_id)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '查询提交失败') } finally { setSubmitting(false) }
  }

  async function openEvidence(eventID: string) { try { setDetail(await api.getEvent(eventID)) } catch (reason) { setError(reason instanceof Error ? reason.message : '引用证据加载失败') } }

  return <section>
    <div className="page-title"><div><p className="eyebrow">本地检索增强</p><h2>智能查询</h2><p className="muted">基于清洗事实和可追溯证据</p></div><div className={`rag-health ${status?.vector_status === 'ready' ? 'ready' : ''}`}><strong>{status?.indexed || 0}</strong><span>已索引 / {status?.documents || 0}</span></div></div>
    <div className="rag-scope">
      <label>开始日期<input type="date" value={from} max={to} onChange={event => setFrom(event.target.value)} /></label><label>结束日期<input type="date" value={to} min={from} onChange={event => setTo(event.target.value)} /></label>
      <label>设备范围<select value={deviceID} onChange={event => { setDeviceID(event.target.value); setProjectID('') }}><option value="">全部设备</option>{facets?.devices.map(item => <option value={item.id} key={item.id}>{item.label}</option>)}</select></label>
      <label>项目范围<select aria-label="项目范围" value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">请选择项目</option>{projects.map(item => <option value={item.id} key={item.id}>{item.display_name}</option>)}<option value="__all__">全部项目（明确选择）</option></select></label>
      <label>活动类型<select value={activityType} onChange={event => setActivityType(event.target.value as ActivityType | '')}><option value="">全部活动</option>{(Object.keys(typeLabels) as ActivityType[]).map(type => <option value={type} key={type}>{typeLabels[type]}</option>)}</select></label>
    </div>
    <div className="rag-query-box"><label>查询问题<textarea aria-label="查询问题" value={question} maxLength={1000} rows={4} onChange={event => setQuestion(event.target.value)} /></label><button disabled={submitting || question.trim().length < 2 || !projectID || status?.vector_status !== 'ready'} onClick={submit}>开始查询</button></div>
    {error && <div className="state error-state"><strong>查询失败</strong><span>{error}</span></div>}
    {job && <div className="rag-result"><div className="rag-progress"><span>{stageLabels[job.status] || job.status}</span><strong>{job.progress}%</strong><progress aria-label="查询进度" max="100" value={job.progress} /></div>{job.status === 'failed' ? <div className="state error-state">查询执行失败</div> : job.answer && <div className="rag-answer"><div className="answer-meta"><h3>回答</h3><span className={`status status-${job.confidence === 'high' ? 'healthy' : job.confidence === 'low' ? 'degraded' : 'healthy'}`}>{job.confidence === 'high' ? '高置信度' : job.confidence === 'low' ? '低置信度' : '中等置信度'}</span></div><p>{job.answer}</p>{job.answer_mode === 'analysis' && job.details && <div className="answer-details"><h4>详细说明</h4><p>{job.details}</p></div>}{job.citations.length > 0 && <div className="evidence-disclosure"><button className="secondary" aria-expanded={evidenceOpen} onClick={() => setEvidenceOpen(value => !value)}>{evidenceOpen ? '收起依据' : `查看依据（${job.citations.length}）`}</button>{evidenceOpen && <div className="rag-citations">{job.citations.map(citation => <button className="citation" key={citation.document_id} onClick={() => citation.canonical_event_id && openEvidence(citation.canonical_event_id)}><b>证据 {citation.number}</b><span>{citation.topic || citation.project_name} · {citation.activity_type === 'process_knowledge' ? '过程知识' : typeLabels[citation.activity_type]}</span>{citation.validation_state && <em className={`knowledge-badge ${citation.validation_state}`}>{citation.validation_state}</em>}<small>{citation.excerpt}</small>{citation.applicability && <small>适用：{citation.applicability}</small>}</button>)}</div>}</div>}</div>}</div>}
    {detail && <div className="drawer-backdrop" role="presentation" onClick={() => setDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">RAG 引用</p><h3>查询引用证据</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setDetail(null)}>×</button></div><div className="detail-list"><div><span>事件 ID</span><strong>{detail.event_id}</strong></div><div><span>事件类型</span><strong>{detail.event_type}</strong></div>{detail.payload && <div><span>脱敏 payload</span><pre>{JSON.stringify(detail.payload, null, 2)}</pre></div>}</div></aside></div>}
  </section>
}
