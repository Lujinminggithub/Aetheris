import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { BrowserCapturePolicy } from '../api/types'

export default function BrowserPolicyPage() {
  const [policy, setPolicy] = useState<BrowserCapturePolicy | null>(null)
  const [enabled, setEnabled] = useState(false)
  const [domains, setDomains] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  useEffect(() => {
    let active = true
    api.getBrowserPolicy().then(value => {
      if (!active) return
      setPolicy(value); setEnabled(value.enabled); setDomains(value.allowed_domains.join('\n'))
    }).catch(reason => { if (active) setError(reason instanceof Error ? reason.message : '浏览器采集策略加载失败') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  async function save() {
    setSaving(true); setError(''); setNotice('')
    const allowedDomains = domains.split(/\r?\n/).map(value => value.trim()).filter(Boolean)
    try {
      const value = await api.updateBrowserPolicy({ enabled, allowed_domains: allowedDomains })
      setPolicy(value); setEnabled(value.enabled); setDomains(value.allowed_domains.join('\n'))
      setNotice(`策略已保存，版本 ${value.revision}`)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '浏览器采集策略保存失败')
    } finally {
      setSaving(false)
    }
  }

  return <section>
    <div className="page-title"><div><p className="eyebrow">采集策略</p><h2>浏览器采集</h2><p className="muted">租户级 Chrome / Edge 域名白名单</p></div></div>
    {error && <div className="state error-state"><strong>请求失败</strong><span>{error}</span></div>}
    {notice && <p className="notice">{notice}</p>}
    {loading ? <div className="state">正在加载浏览器采集策略...</div> : <div className="browser-policy-layout">
      <div className="panel browser-policy-panel">
        <div className="policy-switch-row">
          <div><strong>浏览器活动采集</strong><span>{enabled ? '已启用' : '已停用'}</span></div>
          <label className="switch-control"><input type="checkbox" aria-label="启用浏览器活动采集" checked={enabled} onChange={event => setEnabled(event.target.checked)} /><span /></label>
        </div>
        <label className="domain-editor">允许采集的域名<textarea aria-label="允许采集的域名" value={domains} onChange={event => setDomains(event.target.value)} rows={9} placeholder={'docs.example.com\nportal.example.com'} /></label>
        <div className="policy-actions"><span className="muted">当前策略版本 {policy?.revision || 0}</span><button onClick={save} disabled={saving}>{saving ? '保存中...' : '保存策略'}</button></div>
      </div>
      <div className="panel privacy-boundary"><div className="panel-heading"><h3>数据边界</h3><span className="status status-healthy">白名单限定</span></div><dl className="definition-list"><div><dt>采集范围</dt><dd>仅当前台显示的白名单域名页面</dd></div><div><dt>上传字段</dt><dd>域名、脱敏路径和脱敏可见内容</dd></div><div><dt>明确排除</dt><dd>浏览历史、Cookie、凭据和非白名单页面</dd></div></dl></div>
    </div>}
  </section>
}
