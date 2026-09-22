import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { PublicKnowledgePublicationState, PublicKnowledgeSummary, PublicKnowledgeUnit, PublicKnowledgeValidationState, Session } from '../api/types'

type QueueID = 'source' | 'verify' | 'corroborated' | 'certify' | 'published' | 'withdrawn'
type ReviewAction = 'confirm' | 'verify' | 'certify' | 'reject' | 'suspend' | 'withdraw' | 'republish'

const queueFilters: Record<QueueID, { publication_state: PublicKnowledgePublicationState; validation_state?: PublicKnowledgeValidationState }> = {
  source: { publication_state: 'candidate', validation_state: 'unverified' },
  verify: { publication_state: 'pending_review', validation_state: 'source_confirmed' },
  corroborated: { publication_state: 'pending_review', validation_state: 'cross_tenant_corroborated' },
  certify: { publication_state: 'pending_review', validation_state: 'evidence_verified' },
  published: { publication_state: 'published', validation_state: 'platform_certified' },
  withdrawn: { publication_state: 'withdrawn' },
}

const queueLabels: Record<QueueID, string> = { source: '待来源确认', verify: '待证据验证', corroborated: '跨租户印证', certify: '待平台认证', published: '已发布', withdrawn: '冲突与撤回' }
const validationLabels: Record<PublicKnowledgeValidationState, string> = { unverified: '未验证', source_confirmed: '来源已确认', evidence_verified: '证据已验证', cross_tenant_corroborated: '跨租户印证', platform_certified: '平台已认证', contradicted: '存在冲突' }

function hasPermission(session: Session, permission: string) {
  if (session.permissions?.[permission]) return true
  if (session.access_role === 'platform_admin' || session.access_role === 'admin') return true
  if (permission === 'knowledge:diagnose') return ['tenant_admin', 'analyst'].includes(session.access_role)
  if (permission === 'knowledge:confirm_source' || permission === 'knowledge:verify') return ['tenant_admin', 'analyst'].includes(session.access_role)
  return false
}

export default function PublicKnowledgePage({ session }: { session: Session }) {
  const [summary, setSummary] = useState<PublicKnowledgeSummary | null>(null)
  const [queue, setQueue] = useState<QueueID>('certify')
  const [units, setUnits] = useState<PublicKnowledgeUnit[]>([])
  const [selected, setSelected] = useState<PublicKnowledgeUnit | null>(null)
  const [reason, setReason] = useState('')
  const [loading, setLoading] = useState(true)
  const [acting, setActing] = useState<ReviewAction | ''>('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  async function load() {
    setLoading(true); setError('')
    try {
      const [nextSummary, page] = await Promise.all([api.getPublicKnowledgeSummary(), api.listPublicKnowledgeUnits({ ...queueFilters[queue], limit: 50, offset: 0 })])
      setSummary(nextSummary); setUnits(page.units)
    } catch (cause) { setError(cause instanceof Error ? cause.message : '公共知识加载失败') }
    finally { setLoading(false) }
  }

  useEffect(() => { void load() }, [queue])

  async function openUnit(id: string) {
    setError(''); setReason('')
    try { setSelected(await api.getPublicKnowledgeUnit(id)) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '公共知识详情加载失败') }
  }

  async function review(action: ReviewAction) {
    if (!selected || !reason.trim()) return
    setActing(action); setError(''); setNotice('')
    try {
      const updated = await api.reviewPublicKnowledge(selected.public_knowledge_id, action, { expected_revision: selected.current_revision, reason: reason.trim() })
      setSelected(updated); setReason(''); setNotice('知识状态已更新'); await load()
    } catch (cause) { setError(cause instanceof Error ? cause.message : '知识审核失败') }
    finally { setActing('') }
  }

  async function startBuild() {
    setError(''); setNotice('')
    try { const job = await api.startPublicKnowledgeJob('build_candidates'); setNotice(`候选生成任务已提交：${job.id}`) }
    catch (cause) { setError(cause instanceof Error ? cause.message : '候选任务提交失败') }
  }

  const queueCount = (id: QueueID) => {
    if (!summary) return 0
    if (id === 'source') return summary.candidate
    if (id === 'published') return summary.published
    if (id === 'withdrawn') return summary.withdrawn + summary.open_conflicts
    return summary.pending_review
  }

  return <section>
    <div className="page-title"><div><p className="eyebrow">平台知识域</p><h2>知识治理</h2></div>{hasPermission(session, 'knowledge:certify_public') && <button onClick={startBuild}>生成公共候选</button>}</div>
    {notice && <p className="notice">{notice}</p>}{error && <p className="error">{error}</p>}
    <div className="resource-summary public-knowledge-summary">
      <div><strong>{summary?.published || 0}</strong><span>已发布</span></div><div><strong>{summary?.pending_review || 0}</strong><span>待审核</span></div><div><strong>{summary?.open_conflicts || 0}</strong><span>开放冲突</span></div><div><strong>{summary?.active_jobs || 0}</strong><span>运行任务</span></div>
    </div>
    <div className="knowledge-queue" role="tablist">{(Object.keys(queueLabels) as QueueID[]).map(id => <button role="tab" aria-selected={queue === id} className={queue === id ? 'active' : ''} key={id} onClick={() => setQueue(id)}><span>{queueLabels[id]}</span><b>{queueCount(id)}</b></button>)}</div>
    <div className="panel public-knowledge-table"><div className="panel-heading"><h3>{queueLabels[queue]}</h3><span className="muted">{units.length} 条</span></div>
      {loading ? <div className="state">正在加载...</div> : units.length === 0 ? <div className="state empty">当前队列没有知识</div> : <div className="table-wrap"><table><thead><tr><th>主题</th><th>规范结论</th><th>验证状态</th><th>来源覆盖</th><th>适用条件</th></tr></thead><tbody>{units.map(unit => <tr key={unit.public_knowledge_id} onClick={() => void openUnit(unit.public_knowledge_id)}><td><strong>{unit.canonical_topic}</strong><small>revision {unit.current_revision}</small></td><td className="content-preview">{unit.current.conclusion}</td><td><span className={`knowledge-badge ${unit.current.validation_state}`}>{validationLabels[unit.current.validation_state]}</span></td><td>{unit.current.anonymous_source_tenant_count} 个租户 / {unit.current.independent_session_count} 个会话</td><td>{unit.current.applicability || '-'}</td></tr>)}</tbody></table></div>}
    </div>
    {selected && <div className="drawer-backdrop" role="presentation" onClick={() => setSelected(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">公共知识 revision {selected.current_revision}</p><h3>{selected.canonical_topic}</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setSelected(null)}>×</button></div>
      <div className="detail-list"><div><span>领域范围</span><strong>{selected.domains.length ? selected.domains.join('、') : '未分类'} · {selected.scope_state}</strong></div><div><span>关键实体</span><strong>{selected.entities.length ? selected.entities.join('、') : '-'}</strong></div><div><span>规范结论</span><strong>{selected.current.conclusion}</strong></div><div><span>依据</span><strong>{selected.current.rationale || '-'}</strong></div><div><span>适用条件</span><strong>{selected.current.applicability || '-'}</strong></div><div><span>限制与反例</span><strong>{selected.current.caveats || '-'}</strong></div><div><span>匿名来源</span><strong>{selected.current.anonymous_source_tenant_count} 个租户 / {selected.current.independent_session_count} 个会话</strong></div>{selected.private_evidence && selected.private_evidence.length > 0 && <div><span>本租户证据</span><strong>{selected.private_evidence.map(item => `${item.knowledge_id} r${item.revision}`).join('、')}</strong></div>}</div>
      <div className="knowledge-review"><label>审核理由<textarea aria-label="审核理由" value={reason} rows={3} onChange={event => setReason(event.target.value)} /></label><div className="review-actions">
        {hasPermission(session, 'knowledge:confirm_source') && selected.publication_state === 'candidate' && <button disabled={!reason.trim() || !!acting} onClick={() => void review('confirm')}>来源确认</button>}
        {hasPermission(session, 'knowledge:verify') && selected.publication_state === 'pending_review' && selected.current.validation_state !== 'platform_certified' && <button disabled={!reason.trim() || !!acting} onClick={() => void review('verify')}>证据验证</button>}
        {hasPermission(session, 'knowledge:certify_public') && selected.scope_state === 'classified' && selected.domains.length > 0 && selected.publication_state === 'pending_review' && ['source_confirmed', 'evidence_verified', 'cross_tenant_corroborated'].includes(selected.current.validation_state) && <button disabled={reason.trim().length < 20 || !!acting} onClick={() => void review('certify')}>平台认证</button>}
        {hasPermission(session, 'knowledge:certify_public') && !['published', 'withdrawn'].includes(selected.publication_state) && <button className="secondary" disabled={!reason.trim() || !!acting} onClick={() => void review('reject')}>驳回</button>}
        {hasPermission(session, 'knowledge:withdraw_public') && selected.publication_state === 'published' && <button className="secondary" disabled={!reason.trim() || !!acting} onClick={() => void review('suspend')}>暂停</button>}
        {hasPermission(session, 'knowledge:withdraw_public') && selected.publication_state !== 'withdrawn' && <button className="text-button" disabled={!reason.trim() || !!acting} onClick={() => void review('withdraw')}>撤回</button>}
        {hasPermission(session, 'knowledge:withdraw_public') && selected.publication_state === 'suspended' && <button disabled={!reason.trim() || !!acting} onClick={() => void review('republish')}>重新发布</button>}
      </div></div>
    </aside></div>}
  </section>
}
