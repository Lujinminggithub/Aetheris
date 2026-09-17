import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { LogicalProject, ProcessKnowledgePage as KnowledgePage, ProcessKnowledgeSummary, ProcessKnowledgeUnit } from '../api/types'

const validationLabels: Record<string, string> = { verified: '已验证', partially_verified: '部分验证', unverified: '未验证', contradicted: '存在冲突' }

export default function ProcessKnowledgePage() {
  const [summary, setSummary] = useState<ProcessKnowledgeSummary | null>(null)
  const [page, setPage] = useState<KnowledgePage | null>(null)
  const [projects, setProjects] = useState<LogicalProject[]>([])
  const [projectID, setProjectID] = useState('')
  const [selected, setSelected] = useState<ProcessKnowledgeUnit | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    let active = true; setLoading(true); setError('')
    Promise.all([api.getProcessKnowledgeSummary(), api.listProcessKnowledgeUnits(projectID), api.listProjects()])
      .then(([nextSummary, nextPage, projectResult]) => { if (active) { setSummary(nextSummary); setPage(nextPage); setProjects(projectResult.projects) } })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '过程知识加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [projectID])

  async function backfill() {
    if (!projectID) return
    setError(''); setMessage('')
    try { const job = await api.startProcessKnowledgeBackfill('apply', projectID, summary?.active_version || 1); setMessage(`回填任务已提交：${job.id}`) }
    catch (reason) { setError(reason instanceof Error ? reason.message : '回填提交失败') }
  }

  return <section>
    <div className="page-title"><div><p className="eyebrow">人机协作沉淀</p><h2>过程知识</h2><p className="muted">问题、约束、结论与验证证据</p></div><button disabled={!projectID} onClick={backfill}>回填过程知识</button></div>
    <div className="toolbar"><label>知识项目<select aria-label="知识项目" value={projectID} onChange={event => setProjectID(event.target.value)}><option value="">全部项目</option>{projects.map(project => <option key={project.id} value={project.id}>{project.display_name}</option>)}</select></label><span className="status status-healthy">{summary?.mode || 'shadow'}</span></div>
    {message && <p className="notice">{message}</p>}{error && <p className="error">{error}</p>}
    <div className="effectiveness-kpis quality-metrics">{[['会话', summary?.sessions || 0], ['知识单元', summary?.units || 0], ['已验证', summary?.verified || 0], ['冲突', summary?.conflicts || 0], ['未归属', summary?.unattributed || 0], ['知识块', summary?.chunks || 0]].map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong><small>版本 {summary?.active_version || 0}</small></div>)}</div>
    <div className="panel"><div className="panel-heading"><h3>知识单元</h3><span className="muted">共 {page?.total || 0} 条</span></div>
      {loading ? <div className="state">正在加载...</div> : !page?.units.length ? <div className="state empty">当前范围暂无过程知识</div> : <div className="table-wrap"><table><thead><tr><th>主题</th><th>问题</th><th>结论</th><th>决策</th><th>验证</th><th>适用条件</th></tr></thead><tbody>{page.units.map(unit => <tr key={unit.knowledge_id} onClick={() => setSelected(unit)}><td>{unit.topic}</td><td>{unit.problem}</td><td className="content-preview">{unit.conclusion}</td><td><span className="knowledge-badge">{unit.decision_state}</span></td><td><span className={`knowledge-badge ${unit.validation_state}`}>{validationLabels[unit.validation_state] || unit.validation_state}</span></td><td>{unit.applicability || '-'}</td></tr>)}</tbody></table></div>}
    </div>
    {selected && <div className="drawer-backdrop" role="presentation" onClick={() => setSelected(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><div><p className="eyebrow">知识证据</p><h3>{selected.topic}</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setSelected(null)}>×</button></div><div className="detail-list"><div><span>问题</span><strong>{selected.problem}</strong></div><div><span>结论</span><strong>{selected.conclusion}</strong></div><div><span>适用条件</span><strong>{selected.applicability || '-'}</strong></div><div><span>证据</span><strong>{selected.evidence.length} 条</strong></div></div></aside></div>}
  </section>
}
