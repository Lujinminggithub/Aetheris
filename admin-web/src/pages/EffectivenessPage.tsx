import { useEffect, useMemo, useState } from 'react'
import { api } from '../api/client'
import type { EffectivenessReport, EffectivenessSubject, EventDetail, LogicalProject } from '../api/types'

const periods = [7, 30, 90] as const
const activityLabels: Record<string, string> = { delivery: '交付活动', coding: '编码活动', terminal: '终端操作', ai_collaboration: 'AI 协作', browser: '浏览器活动', other: '其他' }

function localDate(value: Date) { return value.toISOString().slice(0, 10) }
function range(days: number) { const to = new Date(); const from = new Date(to); from.setDate(to.getDate() - days + 1); return { from: localDate(from), to: localDate(to) } }
function minutes(value: number) { if (value < 60) return `${value} 分钟`; const hours = Math.floor(value / 60); const rest = value % 60; return rest ? `${hours} 小时 ${rest} 分钟` : `${hours} 小时` }

export default function EffectivenessPage() {
  const [days, setDays] = useState<(typeof periods)[number]>(30)
  const dates = useMemo(() => range(days), [days])
  const [subjects, setSubjects] = useState<EffectivenessSubject[]>([])
  const [projects, setProjects] = useState<LogicalProject[]>([])
  const [subjectID, setSubjectID] = useState('')
  const [report, setReport] = useState<EffectivenessReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [aiSummary, setAISummary] = useState('')
  const [detail, setDetail] = useState<EventDetail | null>(null)

  const projectNames = useMemo(() => {
    const names = new Map<string, string>()
    projects.forEach(project => {
      names.set(project.id, project.display_name)
      project.locations.forEach(location => names.set(location.local_project_id, location.display_name || project.display_name))
    })
    return names
  }, [projects])

  useEffect(() => { api.listProjects().then(result => setProjects(result.projects)).catch(() => undefined) }, [])

  useEffect(() => {
    let active = true
    setLoading(true); setError(''); setReport(null)
    api.listEffectivenessSubjects(dates.from, dates.to).then(items => {
      if (!active) return
      setSubjects(items)
      setSubjectID(current => items.some(item => item.subject_id === current) ? current : (items[0]?.subject_id || ''))
      if (!items.length) setLoading(false)
    }).catch(reason => { if (active) { setError(reason instanceof Error ? reason.message : '主体加载失败'); setLoading(false) } })
    return () => { active = false }
  }, [dates.from, dates.to])

  useEffect(() => {
    if (!subjectID) return
    let active = true
    setLoading(true); setError(''); setAISummary('')
    api.getEffectiveness(subjectID, dates.from, dates.to).then(value => { if (active) setReport(value) }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '效能数据加载失败') }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [subjectID, dates.from, dates.to])

  const maxDaily = Math.max(1, ...(report?.daily.map(day => day.active_window_minutes) || [1]))
  const activityTotal = Object.values(report?.activity_breakdown || {}).reduce((sum, value) => sum + value, 0)

  async function recompute() {
    try { const result = await api.recomputeEffectiveness(dates.from, dates.to); setNotice(`重算任务已提交：${result.job_id}`) }
    catch (reason) { setNotice(reason instanceof Error ? reason.message : '重算提交失败') }
  }
  async function summarize() {
    if (!subjectID) return
    try { const result = await api.summarizeEffectiveness(subjectID, dates.from, dates.to); setAISummary(`${result.result || '模型未返回内容'}\n${result.notice}`) }
    catch (reason) { setAISummary(reason instanceof Error ? reason.message : 'AI 总结失败') }
  }
  async function openEvidence(eventID: string) {
    try { setDetail(await api.getEvent(eventID)) } catch (reason) { setNotice(reason instanceof Error ? reason.message : '证据事件加载失败') }
  }

  return <section>
    <div className="page-title"><div><p className="eyebrow">可解释分析</p><h2>个人效能</h2><p className="muted">观察工作节奏与投入分布，每项指标均可追溯到事件证据</p></div><div className="page-actions"><button className="secondary" onClick={recompute}>重算数据</button><button onClick={summarize} disabled={!report}>生成 AI 总结</button></div></div>
    <div className="effectiveness-filters"><label>主体<select aria-label="效能主体" value={subjectID} onChange={event => setSubjectID(event.target.value)}><option value="">请选择主体</option>{subjects.map(subject => <option key={subject.subject_id} value={subject.subject_id}>{subject.display_name}</option>)}</select></label><div className="period-control" aria-label="统计周期">{periods.map(value => <button key={value} className={days === value ? 'active' : ''} onClick={() => setDays(value)}>{value} 天</button>)}</div><div className="range-label">{dates.from} 至 {dates.to}</div></div>
    {notice && <p className="notice">{notice}</p>}
    {error && <div className="state error-state"><strong>加载失败</strong><span>{error}</span></div>}
    {loading && <div className="state">正在计算个人效能...</div>}
    {!loading && !error && !report && <div className="state empty">当前范围没有可分析的主体数据</div>}
    {report && <>
      {report.coverage.insufficient && <div className="coverage-warning"><strong>数据不足，不建议进行趋势判断</strong><span>当前覆盖 {report.coverage.covered_days}/{report.coverage.period_days} 天，覆盖率 {Math.round(report.coverage.coverage_ratio * 100)}%</span></div>}
      <div className="effectiveness-kpis">
        <div><span>活跃天数</span><strong>{report.totals.active_days}</strong><small>{report.coverage.period_days} 天周期</small></div>
        <div><span>活跃时段</span><strong>{minutes(report.totals.active_window_minutes)}</strong><small>基于 5 分钟事件窗口，不等同于工时</small></div>
        <div><span>工作会话</span><strong>{report.totals.session_count}</strong><small>平均 {minutes(report.totals.average_session_minutes)}</small></div>
        <div><span>专注时段</span><strong>{report.totals.focus_block_count}</strong><small>合计 {minutes(report.totals.focus_block_minutes)}</small></div>
        <div><span>上下文切换</span><strong>{report.totals.context_switch_count}</strong><small>仅描述项目切换，不判断好坏</small></div>
        <div><span>数据覆盖率</span><strong>{Math.round(report.coverage.coverage_ratio * 100)}%</strong><small>{report.coverage.source_count} 个来源 · {report.coverage.device_count} 台设备</small></div>
      </div>
      <div className="effectiveness-grid">
        <div className="panel"><div className="panel-heading"><h3>每日趋势</h3><span className="muted">活跃时段（分钟）</span></div>{report.daily.length ? <div className="trend-chart">{report.daily.map(day => <div className="trend-column" key={day.local_date} title={`${day.local_date} · ${day.active_window_minutes} 分钟`}><div className="trend-value">{day.active_window_minutes}</div><div className="trend-bar"><span style={{ height: `${Math.max(4, day.active_window_minutes / maxDaily * 100)}%` }} /></div><time>{day.local_date.slice(5)}</time></div>)}</div> : <div className="state empty">当前周期暂无趋势数据</div>}</div>
        <div className="panel"><div className="panel-heading"><h3>活动构成</h3><span className="muted">确定性事件分类</span></div><div className="composition-list">{Object.entries(report.activity_breakdown).map(([key, value]) => <div key={key}><div><span>{activityLabels[key] || key}</span><strong>{value}</strong></div><div className="composition-track"><span style={{ width: `${activityTotal ? value / activityTotal * 100 : 0}%` }} /></div></div>)}</div></div>
      </div>
      <div className="effectiveness-grid breakdown-grid">
        <BreakdownPanel title="项目分布" values={report.project_breakdown} projectNames={projectNames} />
        <BreakdownPanel title="工作角色分布" values={report.work_role_breakdown} />
      </div>
      <div className="effectiveness-grid detail-grid"><div className="panel"><div className="panel-heading"><h3>指标口径</h3><span className="muted">定义版本 {report.metric_definition_version}</span></div><dl className="definition-list">{Object.entries(report.definitions).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{value}</dd></div>)}</dl></div><div className="panel"><div className="panel-heading"><h3>事件证据</h3><span className="muted">代表性事件 {report.evidence_event_ids.length} 条</span></div>{report.evidence_event_ids.length ? <div className="evidence-list">{report.evidence_event_ids.map(id => <button className="secondary" key={id} onClick={() => openEvidence(id)}>{id}</button>)}</div> : <div className="state empty">暂无事件证据</div>}</div></div>
      {aiSummary && <div className="ai-summary"><div className="panel-heading"><h3>AI 总结</h3><span>不作为绩效评价</span></div><p>{aiSummary}</p></div>}
    </>}
    {detail && <div className="drawer-backdrop" role="presentation" onClick={() => setDetail(null)}><aside className="drawer" onClick={event => event.stopPropagation()}><div className="panel-heading"><h3>证据事件</h3><button className="icon-button" aria-label="关闭详情" onClick={() => setDetail(null)}>×</button></div><div className="detail-list"><div><span>事件 ID</span><strong>{detail.event_id}</strong></div><div><span>事件类型</span><strong>{detail.event_type}</strong></div><div><span>发生时间</span><strong>{detail.occurred_at}</strong></div>{detail.payload && <div><span>脱敏 payload</span><pre>{JSON.stringify(detail.payload, null, 2)}</pre></div>}</div></aside></div>}
  </section>
}

function BreakdownPanel({ title, values, projectNames }: { title: string; values: Record<string, { event_count: number; active_window_minutes: number }>; projectNames?: Map<string, string> }) {
  const entries = Object.entries(values)
  const total = entries.reduce((sum, [, value]) => sum + value.active_window_minutes, 0)
  return <div className="panel"><div className="panel-heading"><h3>{title}</h3><span className="muted">按活跃时段</span></div>{entries.length ? <div className="breakdown-list">{entries.map(([key, value]) => <div key={key}><div><strong>{projectNames?.get(key) || key}</strong><span>{minutes(value.active_window_minutes)} · {value.event_count} 个事件</span></div><div className="composition-track"><span style={{ width: `${total ? value.active_window_minutes / total * 100 : 0}%` }} /></div></div>)}</div> : <div className="state empty">暂无分布数据</div>}</div>
}
