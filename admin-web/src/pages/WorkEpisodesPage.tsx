import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { WorkEpisode } from '../api/types'

export default function WorkEpisodesPage() {
  const [episodes, setEpisodes] = useState<WorkEpisode[]>([])
  const [devices, setDevices] = useState<{ id: string; name: string }[]>([])
  const [deviceID, setDeviceID] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => { api.listDevices().then(items => { setDevices(items.map(item => ({ id: item.id, name: item.name }))); if (items[0]) setDeviceID(items[0].id) }).catch(() => setDevices([])) }, [])
  useEffect(() => { setLoading(true); api.listWorkEpisodes(deviceID ? { device_id: deviceID } : {}).then(result => setEpisodes(result.episodes)).catch(reason => setError(reason instanceof Error ? reason.message : '工作片段加载失败')).finally(() => setLoading(false)) }, [deviceID])
  return <section><div className="page-title"><div><p className="eyebrow">可解释工作成果</p><h2>工作片段</h2><p className="muted">按设备查看目标、动作、验证和证据</p></div></div><div className="toolbar"><label>设备<select value={deviceID} onChange={event => setDeviceID(event.target.value)}><option value="">全部设备</option>{devices.map(device => <option key={device.id} value={device.id}>{device.name || device.id}</option>)}</select></label></div>{error && <div className="state error-state">{error}</div>}{loading ? <div className="state">正在加载工作片段...</div> : episodes.length === 0 ? <div className="state empty">当前设备暂无工作片段</div> : <div className="episode-list">{episodes.map(episode => <article className="panel episode-card" key={episode.episode_id}><div className="panel-heading"><div><h3>{episode.title || '未命名工作片段'}</h3><span className="muted">{episode.device_id} · {episode.project_name || episode.project_id || '未归属'} · {new Date(episode.started_at).toLocaleString('zh-CN')}</span></div><span className={`status status-${episode.needs_review ? 'degraded' : 'healthy'}`}>{episode.needs_review ? '待复核' : '已确认'}</span></div><dl className="definition-list"><div><dt>目标</dt><dd>{episode.objective || '未识别'}</dd></div><div><dt>动作</dt><dd>{episode.actions.length ? episode.actions.map(action => <span className="reason-code" key={action.event_id}>{action.summary}</span>) : '暂无'}</dd></div><div><dt>验证</dt><dd>{episode.validations.length ? episode.validations.map(validation => <span className="reason-code" key={validation.event_id}>{validation.summary} · {validation.result}</span>) : '暂无明确验证'}</dd></div><div><dt>结果</dt><dd>{episode.outcome || '未形成结论'}</dd></div><div><dt>证据</dt><dd>{episode.evidence.length ? episode.evidence.map(item => <code key={`${item.section}-${item.event_id}`}>{item.event_id}</code>) : '暂无'}</dd></div></dl></article>)}</div>}</section>
}
