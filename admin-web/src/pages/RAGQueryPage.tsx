import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { EventDetail, PublicKnowledgeUnit, RAGCitation, RAGQueryJob, RAGStatus } from '../api/types'

const stageLabels: Record<string, string> = { queued: '等待处理', embedding: '理解问题', retrieving: '检索知识', generating: '生成回答', completed: '查询完成', failed: '查询失败' }
const sourceLabels: Record<string, string> = { tenant_private: '本租户私有知识', platform_public: '平台公共知识', model_general: '模型通用知识', process_knowledge: '本租户私有知识', activity_fact: '活动证据' }

export default function RAGQueryPage() {
  const [question, setQuestion] = useState('')
  const [status, setStatus] = useState<RAGStatus | null>(null)
  const [job, setJob] = useState<RAGQueryJob | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [eventDetail, setEventDetail] = useState<EventDetail | null>(null)
  const [publicDetail, setPublicDetail] = useState<PublicKnowledgeUnit | null>(null)
  const [evidenceOpen, setEvidenceOpen] = useState(false)
  const pollTimer = useRef<number | null>(null)

  useEffect(() => {
    let active = true
    api.getRAGStatus().then(next => { if (active) setStatus(next) }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '检索状态加载失败') })
    return () => { active = false }
  }, [])
  useEffect(() => () => { if (pollTimer.current !== null) window.clearTimeout(pollTimer.current) }, [])

  async function poll(queryID: string) {
    try {
      const next = await api.getRAGQuery(queryID); setJob(next)
      if (next.status !== 'completed' && next.status !== 'failed') pollTimer.current = window.setTimeout(() => void poll(queryID), 1000)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '查询状态加载失败') }
  }

  async function submit() {
    const normalized = question.trim()
    setSubmitting(true); setError(''); setJob(null); setEvidenceOpen(false)
    try {
      const created = await api.createRAGQuery({ question: normalized, knowledge_scope: 'tenant_and_public' })
      setJob({ query_id: created.query_id, question: normalized, filters: { knowledge_scope: 'tenant_and_public' }, status: created.status as RAGQueryJob['status'], progress: created.progress, answer: '', citations: [], created_at: new Date().toISOString() })
      await poll(created.query_id)
    } catch (reason) { setError(reason instanceof Error ? reason.message : '查询提交失败') }
    finally { setSubmitting(false) }
  }

  async function openCitation(citation: RAGCitation) {
    try {
      if (citation.source_scope === 'platform_public' && citation.public_knowledge_id) {
        setPublicDetail(await api.getPublicKnowledgeUnit(citation.public_knowledge_id)); return
      }
      if (citation.canonical_event_id) setEventDetail(await api.getEvent(citation.canonical_event_id))
    } catch (reason) { setError(reason instanceof Error ? reason.message : '引用详情加载失败') }
  }

  const ready = status?.vector_status === 'ready'
  return <section>
    <div className="page-title"><div><p className="eyebrow">全局过程知识</p><h2>智能查询</h2></div><div className={`rag-health ${ready ? 'ready' : ''}`}><strong>{status?.indexed || 0}</strong><span>已索引 / {status?.documents || 0}</span></div></div>
    <div className="rag-query-box"><label>查询问题<textarea aria-label="查询问题" value={question} maxLength={1000} rows={4} onChange={event => setQuestion(event.target.value)} /></label><button aria-busy={submitting} title={question.trim().length < 2 ? '请输入至少两个字符' : !ready ? '检索服务尚未就绪' : ''} disabled={submitting || question.trim().length < 2 || !ready} onClick={() => void submit()}>{submitting ? '提交中...' : '开始查询'}</button></div>
    {error && <div className="state error-state"><strong>查询失败</strong><span>{error}</span></div>}
    {job && <div className="rag-result"><div className="rag-progress"><span>{stageLabels[job.status] || job.status}</span><strong>{job.progress}%</strong><progress aria-label="查询进度" max="100" value={job.progress} /></div>{job.status === 'failed' ? <div className="state error-state">查询执行失败</div> : job.answer && <div className="rag-answer"><div className="answer-meta"><h3>回答</h3><span className={`status status-${job.confidence === 'low' ? 'degraded' : 'healthy'}`}>{job.confidence === 'high' ? '高置信度' : job.confidence === 'low' ? '低置信度' : '中等置信度'}</span></div><p>{job.answer}</p>{job.answer_mode === 'analysis' && job.details && <div className="answer-details"><h4>详细说明</h4><p>{job.details}</p></div>}{job.citations.length > 0 && <div className="evidence-disclosure"><button className="secondary" aria-expanded={evidenceOpen} onClick={() => setEvidenceOpen(value => !value)}>{evidenceOpen ? '收起依据' : `查看依据（${job.citations.length}）`}</button>{evidenceOpen && <div className="rag-citations">{job.citations.map(citation => {
      const source = citation.source_scope || citation.source_kind || 'activity_fact'
      const canOpen = !!citation.canonical_event_id || (source === 'platform_public' && !!citation.public_knowledge_id)
      return <button className="citation" key={`${citation.document_id}-${citation.number}`} disabled={!canOpen} onClick={() => void openCitation(citation)}><b>证据 {citation.number}</b><span>{citation.topic || '通用知识'} · {sourceLabels[source] || source}</span>{citation.validation_state && <em className={`knowledge-badge ${citation.validation_state}`}>{citation.validation_state}</em>}<small>{citation.excerpt}</small>{citation.applicability && <small>适用：{citation.applicability}</small>}{source === 'platform_public' && <small>{citation.anonymous_source_tenant_count || 0} 个匿名租户来源</small>}</button>
    })}</div>}</div>}</div>}</div>}
    {eventDetail && <div className="drawer-backdrop" role="presentation" onClick={() => setEventDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">本租户证据</p><h3>查询引用证据</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setEventDetail(null)}>×</button></div><div className="detail-list"><div><span>事件 ID</span><strong>{eventDetail.event_id}</strong></div><div><span>事件类型</span><strong>{eventDetail.event_type}</strong></div>{eventDetail.payload && <div><span>脱敏 payload</span><pre>{JSON.stringify(eventDetail.payload, null, 2)}</pre></div>}</div></aside></div>}
    {publicDetail && <div className="drawer-backdrop" role="presentation" onClick={() => setPublicDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">平台公共知识</p><h3>{publicDetail.canonical_topic}</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setPublicDetail(null)}>×</button></div><div className="detail-list"><div><span>公共知识 ID</span><strong>{publicDetail.public_knowledge_id}</strong></div><div><span>规范结论</span><strong>{publicDetail.current.conclusion}</strong></div><div><span>适用条件</span><strong>{publicDetail.current.applicability || '-'}</strong></div><div><span>匿名来源</span><strong>{publicDetail.current.anonymous_source_tenant_count} 个租户 / {publicDetail.current.independent_session_count} 个会话</strong></div></div></aside></div>}
  </section>
}
