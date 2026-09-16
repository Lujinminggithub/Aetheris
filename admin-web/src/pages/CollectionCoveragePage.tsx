import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AdapterHealthSnapshot, ApplicationCapturePolicy } from '../api/types'

const names: Record<string, string> = { browser: '浏览器', application_ocr_fallback: '应用 OCR 保底', vscode: 'VS Code', visual_studio: 'Visual Studio', claude_code: 'Claude Code', cursor: 'Cursor', codex: 'Codex', github_copilot: 'GitHub Copilot', terminal: '终端', git: 'Git' }
const states: Record<string, string> = { active: '采集中', idle: '空闲', source_missing: '数据源缺失', permission_denied: '权限不足', format_changed: '格式变化', dependency_missing: '依赖缺失', error: '采集错误', disabled: '已停用' }
const fallbackReasons: Record<string, string> = { captured: '最近采集成功', policy_disabled: '策略已停用', window_unavailable: '没有可采集前台窗口', privacy_gate_blocked: '隐私门禁已阻止', native_adapter_recent: '专属适配器近期已有数据', queue_paused: '本地队列容量保护', ocr_busy: 'OCR 正在处理其他任务', failure_backoff: '失败后退避中', device_rate_limited: '设备采集频率保护', process_rate_limited: '进程采集频率保护', ocr_timeout: 'OCR 处理超时', ocr_failed: 'OCR 处理失败', dlp_blocked: '内容安全策略已阻止', duplicate_window_content: '重复内容已跳过' }

function displaySnapshot(item: AdapterHealthSnapshot) {
  const marker = (item.detected_format || '').trim().toLowerCase()
  const approvedBrowserForeground = /^foreground:(chrome|msedge)(\.exe)?(?::|$)/.test(marker)
  const nonBrowserForeground = item.adapter_id === 'browser' && marker.startsWith('foreground:') && !approvedBrowserForeground
  if (nonBrowserForeground) {
    return { state: 'idle', label: '空闲', detectedFormat: '', stage: '未检测到浏览器前台窗口', errorCode: '' }
  }
  if (item.adapter_id === 'application_ocr_fallback') {
    const reason = marker.startsWith('ocr_fallback:') ? marker.slice('ocr_fallback:'.length) : ''
    return { state: item.state, label: states[item.state] || item.state, detectedFormat: '', stage: fallbackReasons[reason] || '', errorCode: '' }
  }
  return { state: item.state, label: states[item.state] || item.state, detectedFormat: item.detected_format || '', stage: item.error_stage || '', errorCode: item.error_code || '' }
}

export default function CollectionCoveragePage() {
  const [items, setItems] = useState<AdapterHealthSnapshot[]>([]); const [loading, setLoading] = useState(true); const [error, setError] = useState('')
  const [policy, setPolicy] = useState<ApplicationCapturePolicy | null>(null); const [savingPolicy, setSavingPolicy] = useState(false)
  useEffect(() => { api.listAdapterHealth().then(result => setItems(result.items)).catch(reason => setError(reason instanceof Error ? reason.message : '采集健康加载失败')).finally(() => setLoading(false)) }, [])
  useEffect(() => { api.getApplicationCapturePolicy().then(setPolicy).catch(() => undefined) }, [])
  async function toggleApplicationCapture() { if (!policy) return; setSavingPolicy(true); try { setPolicy(await api.updateApplicationCapturePolicy({ enabled: !policy.enabled })) } catch (reason) { setError(reason instanceof Error ? reason.message : '应用 OCR 保底策略保存失败') } finally { setSavingPolicy(false) } }
  return <section><div className="page-title"><div><p className="eyebrow">采集可观测性</p><h2>采集覆盖</h2><p className="muted">区分健康空闲、数据源缺失、格式变化和运行错误</p></div></div>{policy && <div className="policy-switch-row coverage-policy"><div><strong>应用 OCR 保底</strong><span>专属适配器连续 60 秒无有效数据后，仅采集已授权前台工作进程</span></div><label className="switch-control"><input type="checkbox" aria-label="启用应用 OCR 保底" checked={policy.enabled} disabled={savingPolicy} onChange={toggleApplicationCapture} /><span /></label></div>}{error && <div className="state error-state">{error}</div>}{loading ? <div className="state">正在加载采集健康...</div> : <div className="panel"><div className="table-wrap"><table><thead><tr><th>设备</th><th>适配器</th><th>状态</th><th>统计</th><th>阶段</th><th>错误码</th><th>最近事件</th></tr></thead><tbody>{items.map(item => { const view = displaySnapshot(item); return <tr key={`${item.device_id}-${item.adapter_id}`}><td>{item.device_id}</td><td>{names[item.adapter_id] || item.adapter_id}</td><td><span className={`status status-${view.state === 'error' ? 'down' : view.state === 'active' ? 'healthy' : 'degraded'}`}>{view.label}</span></td><td>{item.adapter_id === 'application_ocr_fallback' ? `候选 ${item.discovered} · 成功 ${item.parsed} · 跳过 ${item.skipped} · 失败 ${item.failed}` : view.detectedFormat || '-'}</td><td>{view.stage || '-'}</td><td>{view.errorCode || '-'}</td><td>{item.last_event_at ? new Date(item.last_event_at).toLocaleString('zh-CN') : '暂无'}</td></tr> })}</tbody></table></div></div>}</section>
}
