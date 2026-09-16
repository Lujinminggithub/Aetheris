import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { DataQualityFactPage, DataQualitySummary, EventDetail } from '../api/types'

function dateRange() {
  const to = new Date()
  const from = new Date(to)
  from.setDate(to.getDate() - 29)
  const format = (value: Date) => `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}-${String(value.getDate()).padStart(2, '0')}`
  return { from: format(from), to: format(to) }
}

export default function DataQualityPage() {
  const initial = dateRange()
  const [from, setFrom] = useState(initial.from)
  const [to, setTo] = useState(initial.to)
  const [quality, setQuality] = useState('')
  const [summary, setSummary] = useState<DataQualitySummary | null>(null)
  const [page, setPage] = useState<DataQualityFactPage | null>(null)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(true)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [selectedEvidence, setSelectedEvidence] = useState<EventDetail | null>(null)

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    Promise.all([api.getDataQualitySummary(from, to), api.listDataQualityFacts(from, to, quality, offset)])
      .then(([nextSummary, nextPage]) => {
        if (active) { setSummary(nextSummary); setPage(nextPage) }
      })
      .catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '数据质量加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [from, to, quality, offset])

  async function recompute() {
    setMessage('')
    setError('')
    try {
      const result = await api.recomputeDataQuality(from, to)
      setMessage(`清洗重算任务已提交：${result.job_id}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '重算提交失败')
    }
  }

  async function openEvidence(eventId: string) {
    setError('')
    try {
      setSelectedEvidence(await api.getEvent(eventId))
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '原始证据加载失败')
    }
  }

  const metrics = [
    ['原始事件', summary?.raw_events || 0], ['清洗事实', summary?.clean_facts || 0],
    ['已合并', summary?.merged_facts || 0], ['命令残片', summary?.command_fragments || 0],
    ['待确认', summary?.quarantined_facts || 0], ['已排除效能', summary?.excluded_facts || 0],
  ] as const

  return <section>
    <div className="page-title"><div><p className="eyebrow">可追溯清洗</p><h2>数据质量</h2><p className="muted">清洗原因、证据链与规则版本</p></div><button onClick={recompute}>重算清洗事实</button></div>
    <div className="toolbar">
      <label>开始日期<input type="date" value={from} onChange={event => { setFrom(event.target.value); setOffset(0) }} /></label>
      <label>结束日期<input type="date" value={to} onChange={event => { setTo(event.target.value); setOffset(0) }} /></label>
      <label>质量状态<select value={quality} onChange={event => { setQuality(event.target.value); setOffset(0) }}><option value="">全部</option><option value="accepted">已接受</option><option value="merged">已合并</option><option value="quarantined">待确认</option></select></label>
    </div>
    {message && <p className="notice">{message}</p>}{error && <p className="error">{error}</p>}
    <div className="effectiveness-kpis quality-metrics">{metrics.map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong><small>规则版本 {summary?.rule_version || 1}</small></div>)}</div>
    <div className="panel">
      <div className="panel-heading"><h3>清洗事实与证据链</h3><span className="muted">共 {page?.total || 0} 条</span></div>
      {loading ? <div className="state">正在加载...</div> : !page?.facts.length ? <div className="state empty">当前条件暂无事实</div> : <div className="table-wrap"><table>
        <thead><tr><th>时间</th><th>类型 / 状态</th><th>设备 / 项目</th><th>命令摘要</th><th>原因</th><th>源事件</th></tr></thead>
        <tbody>{page.facts.map(fact => <tr key={fact.fact_id}>
          <td>{new Date(fact.occurred_at).toLocaleString('zh-CN')}</td><td>{fact.fact_type}<small>{fact.quality_state} · {fact.confidence}</small></td><td>{fact.device_name}<small>{fact.project_name}</small></td><td className="content-preview">{fact.command_display || '-'}</td>
          <td>{fact.merge_method !== 'none' && <span className="reason-code">{fact.merge_method}</span>}{fact.reason_codes.map(code => <span className="reason-code" key={code}>{code}</span>)}</td><td>{fact.source_event_ids.map(id => <button className="evidence-link" key={id} onClick={() => openEvidence(id)}>{id}</button>)}</td>
        </tr>)}</tbody>
      </table></div>}
      <div className="pagination"><button className="secondary" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 50))}>上一页</button><button className="secondary" disabled={!page || offset + 50 >= page.total} onClick={() => setOffset(offset + 50)}>下一页</button></div>
    </div>
    {selectedEvidence && <div className="drawer-backdrop" role="presentation" onClick={() => setSelectedEvidence(null)}><aside className="drawer" onClick={event => event.stopPropagation()}>
      <div className="panel-heading"><div><p className="eyebrow">清洗证据链</p><h3>原始证据详情</h3></div><button className="icon-button" aria-label="关闭详情" onClick={() => setSelectedEvidence(null)}>×</button></div>
      <div className="detail-list"><div><span>事件 ID</span><strong>{selectedEvidence.event_id}</strong></div><div><span>事件类型</span><strong>{selectedEvidence.event_type}</strong></div><div><span>发生时间</span><strong>{new Date(selectedEvidence.occurred_at).toLocaleString('zh-CN')}</strong></div><div><span>脱敏 payload</span><pre>{JSON.stringify(selectedEvidence.payload, null, 2)}</pre></div></div>
    </aside></div>}
  </section>
}
