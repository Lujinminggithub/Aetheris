import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { LogicalProject, ProcessKnowledgeClaim, ProcessKnowledgeClaimPage, ProcessKnowledgePage as KnowledgePage, ProcessKnowledgeSummary, ProcessKnowledgeUnit } from '../api/types'

const validationLabels: Record<string, string> = { verified: '已验证', partially_verified: '部分验证', unverified: '未验证', contradicted: '存在冲突' }

export default function ProcessKnowledgePage() {
  const [summary, setSummary] = useState<ProcessKnowledgeSummary | null>(null)
  const [page, setPage] = useState<KnowledgePage | null>(null)
  const [projects, setProjects] = useState<LogicalProject[]>([])
  const [projectID, setProjectID] = useState('')
  const [selected, setSelected] = useState<ProcessKnowledgeUnit | null>(null)
  const [claimPage, setClaimPage] = useState<ProcessKnowledgeClaimPage | null>(null)
  const [claimDomain, setClaimDomain] = useState('')
  const [selectedClaim, setSelectedClaim] = useState<ProcessKnowledgeClaim | null>(null)
  const [claimReason, setClaimReason] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    let active = true; setLoading(true); setError('')
    Promise.all([api.getProcessKnowledgeSummary(), api.listProcessKnowledgeUnits(projectID), api.listProjects(), api.listProcessKnowledgeClaims(claimDomain)])
      .then(([nextSummary, nextPage, projectResult, nextClaims]) => { if (active) { setSummary(nextSummary); setPage(nextPage); setProjects(projectResult.projects); setClaimPage(nextClaims) } })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '过程知识加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [projectID, claimDomain])

  async function reviewClaim(action: 'confirm' | 'reject') {
    if (!selectedClaim || claimReason.trim().length < 10) return
    setError(''); setMessage('')
    try {
      await api.reviewProcessKnowledgeClaim(selectedClaim.claim_id, action, claimReason.trim())
      setSelectedClaim(null); setClaimReason('')
      setClaimPage(await api.listProcessKnowledgeClaims(claimDomain))
      setMessage(action === 'confirm' ? '原子知识已确认，可进入公共候选审核' : '原子知识已拒绝')
    } catch (reason) { setError(reason instanceof Error ? reason.message : '原子知识审核失败') }
  }

  async function backfill() {
    if (!projectID) return
    setError(''); setMessage('')
    try { const job = await api.startProcessKnowledgeBackfill('apply', projectID, (summary?.active_version || 0) + 1); setMessage(`回填任务已提交：${job.id}`) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '回填提交失败') }
  }

  async function activate(mode: 'shadow' | 'canary' | 'active') {
    setError(''); setMessage('')
    const version = (summary?.active_version || 0) + 1
    try { await api.activateProcessKnowledgeVersion(version, mode, mode === 'canary' ? 10 : 0); setMessage(`知识版本 ${version} 已切换为 ${mode}`) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '版本切换失败') }
  }

  async function rollback() {
    if (!summary || summary.active_version <= 1) return
    try { await api.rollbackProcessKnowledgeVersion(summary.active_version - 1); setMessage(`已回滚到知识版本 ${summary.active_version - 1}`) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '版本回滚失败') }
  }

  return <section>
    <div className="page-title"><div><p className="eyebrow">人机协作沉淀</p><h2>过程知识</h2><p className="muted">问题、约束、结论与验证证据</p></div><button disabled={!projectID} onClick={backfill}>回填过程知识</button></div>
    <div className="toolbar"><label>知识项目<select aria-label="知识项目" value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">全部项目</option>{projects.map(project => <option key={project.id} value={project.id}>{project.display_name}</option>)}</select></label><span className="status status-healthy">{summary?.mode || 'shadow'}</span><button className="secondary" onClick={() => activate('shadow')}>切换为 Shadow</button><button className="secondary" onClick={() => activate('canary')}>10% 灰度</button><button className="secondary" onClick={() => activate('active')}>正式启用</button>{(summary?.active_version || 0) > 1 && <button className="secondary" onClick={rollback}>回滚上一版本</button>}</div>
    {message && <p className="notice">{message}</p>}{error && <p className="error">{error}</p>}
    <div className="effectiveness-kpis quality-metrics">{[['会话', summary?.sessions || 0], ['知识单元', summary?.units || 0], ['已验证', summary?.verified || 0], ['冲突', summary?.conflicts || 0], ['未归属', summary?.unattributed || 0], ['知识块', summary?.chunks || 0]].map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong><small>版本 {summary?.active_version || 0}</small></div>)}</div>
    <div className="panel"><div className="panel-heading"><h3>知识单元</h3><span className="muted">共 {page?.total || 0} 条</span></div>
      {loading ? <div className="state">正在加载...</div> : !page?.units.length ? <div className="state empty">当前范围暂无过程知识</div> : <div className="table-wrap"><table><thead><tr><th>主题</th><th>问题</th><th>结论</th><th>决策</th><th>验证</th><th>适用条件</th></tr></thead><tbody>{page.units.map(unit => <tr key={unit.knowledge_id} onClick={() => setSelected(unit)}><td>{unit.topic}</td><td>{unit.problem}</td><td className="content-preview">{unit.conclusion}</td><td><span className="knowledge-badge">{unit.decision_state}</span></td><td><span className={`knowledge-badge ${unit.validation_state}`}>{validationLabels[unit.validation_state] || unit.validation_state}</span></td><td>{unit.applicability || '-'}</td></tr>)}</tbody></table></div>}
    </div>
    <div className="panel claim-review-panel"><div className="panel-heading"><div><h3>原子知识审核</h3><span className="muted">先确认知识点，再串联过程知识</span></div><label>领域<select aria-label="原子知识领域" value={claimDomain} onChange={event => setClaimDomain(event.target.value)}><option value="">全部领域</option><option value="dlp">DLP</option><option value="edr">EDR</option><option value="network_transport">网络传输</option><option value="rag">RAG</option></select></label></div>
      {!claimPage?.claims.length ? <div className="state empty">当前范围没有待审核原子知识</div> : <div className="table-wrap"><table><thead><tr><th>领域</th><th>知识点</th><th>关键实体</th><th>验证</th><th>证据</th></tr></thead><tbody>{claimPage.claims.map(claim => <tr key={claim.claim_id} onClick={() => { setSelectedClaim(claim); setClaimReason('') }}><td>{claim.domain}</td><td className="content-preview">{claim.claim}</td><td>{claim.entities.join('、') || '-'}</td><td>{validationLabels[claim.validation_state] || claim.validation_state}</td><td>{claim.evidence_ids.length}</td></tr>)}</tbody></table></div>}
    </div>
    {selected && <div className="drawer-backdrop" role="presentation" onClick={() => setSelected(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">知识证据</p><h3>{selected.topic}</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setSelected(null)}>×</button></div><div className="detail-list"><div><span>问题</span><strong>{selected.problem}</strong></div><div><span>结论</span><strong>{selected.conclusion}</strong></div><div><span>适用条件</span><strong>{selected.applicability || '-'}</strong></div><div><span>证据</span><strong>{selected.evidence.length} 条</strong></div></div></aside></div>}
    {selectedClaim && <div className="drawer-backdrop" role="presentation" onClick={() => setSelectedClaim(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">原子知识 · {selectedClaim.domain}</p><h3>知识点审核</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setSelectedClaim(null)}>×</button></div><div className="detail-list"><div><span>父问题</span><strong>{selectedClaim.problem}</strong></div><div><span>知识点</span><strong>{selectedClaim.claim}</strong></div><div><span>关键实体</span><strong>{selectedClaim.entities.join('、') || '-'}</strong></div><div><span>适用条件</span><strong>{selectedClaim.applicability || '-'}</strong></div><div><span>证据 ID</span><strong>{selectedClaim.evidence_ids.join('、') || '-'}</strong></div></div><div className="knowledge-review"><label>审核理由<textarea aria-label="原子知识审核理由" value={claimReason} rows={3} onChange={event => setClaimReason(event.target.value)} /></label><div className="review-actions"><button disabled={claimReason.trim().length < 10} onClick={() => void reviewClaim('confirm')}>确认知识点</button><button className="secondary" disabled={claimReason.trim().length < 10} onClick={() => void reviewClaim('reject')}>拒绝知识点</button></div></div></aside></div>}
  </section>
}
