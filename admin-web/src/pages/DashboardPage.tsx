import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AdminSummary, ProviderHealth } from '../api/types'

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="state error-state"><strong>暂时无法加载</strong><span>{message}</span><button className="secondary" onClick={onRetry}>重试</button></div>
}
function HealthStatus({ status }: { status: string }) {
  const label = status === 'healthy' ? '正常' : status === 'degraded' ? '降级' : status === 'down' ? '不可用' : status
  return <span className={`status status-${status}`}>{label}</span>
}
export default function DashboardPage() {
  const [summary, setSummary] = useState<AdminSummary | null>(null)
  const [providers, setProviders] = useState<ProviderHealth[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  async function load() { setLoading(true); setError(''); try { const [nextSummary, nextProviders] = await Promise.all([api.getSummary(), api.getProviderHealth()]); setSummary(nextSummary); setProviders(nextProviders) } catch (e) { setError(e instanceof Error ? e.message : '请求失败') } finally { setLoading(false) } }
  useEffect(() => { load() }, [])
  if (loading) return <section><div className="page-title"><div><p className="eyebrow">运行态势</p><h2>总览</h2></div></div><div className="state">正在加载总览...</div></section>
  if (error) return <section><div className="page-title"><div><p className="eyebrow">运行态势</p><h2>总览</h2></div></div><ErrorState message={error} onRetry={load} /></section>
  if (!summary) return <section><div className="state empty">暂无总览数据</div></section>
  return <section>
    <div className="page-title"><div><p className="eyebrow">运行态势</p><h2>总览</h2><p className="muted">实时掌握主体、设备与模型服务状态</p></div><button className="secondary" onClick={load}>刷新数据</button></div>
    <div className="metrics"><div className="metric"><span>主体总数</span><strong>{summary.total_subjects}</strong><small>已纳入管理的主体</small></div><div className="metric"><span>设备总数</span><strong>{summary.total_devices}</strong><small>已注册采集设备</small></div><div className="metric"><span>活跃设备</span><strong>{summary.active_devices}</strong><small>最近有心跳的设备</small></div><div className="metric"><span>今日事件</span><strong>{summary.events_today}</strong><small>UTC+08:00 当日累计</small></div></div>
    <div className="dashboard-grid"><div className="panel"><div className="panel-heading"><h3>最近活动</h3><span className="muted">最新事件</span></div>{summary.activity?.length ? <div className="activity-list">{summary.activity.slice(0, 6).map(item => <div className="activity-item" key={item.event_id || item.id}><span className="activity-dot" /><div><strong>{item.event_type}</strong><p>{item.summary || `${item.subject_id || '未知主体'} · ${item.device_id || '未知设备'}`}</p></div><time>{new Date(item.occurred_at).toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit' })}</time></div>)}</div> : <div className="state empty">暂无活动记录</div>}</div><div className="panel"><div className="panel-heading"><h3>模型服务健康</h3><span className="muted">实时探针</span></div>{providers.length ? <div className="health-list">{providers.map(provider => <div className="health-item" key={`${provider.provider}-${provider.name}`}><div><strong>{provider.name}</strong><p>{provider.provider}{provider.message ? ` · ${provider.message}` : ''}</p></div><div className="health-meta"><HealthStatus status={provider.status} />{provider.latency_ms != null && <small>{provider.latency_ms} ms</small>}</div></div>)}</div> : <div className="state empty">暂无模型健康数据</div>}</div></div>
  </section>
}
