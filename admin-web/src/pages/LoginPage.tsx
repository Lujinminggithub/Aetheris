import { FormEvent, useState } from 'react'
import { api } from '../api/client'
import type { Session } from '../api/types'

export default function LoginPage({ onLogin }: { onLogin: (session: Session) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  async function submit(event: FormEvent) { event.preventDefault(); setLoading(true); setError(''); try { const session = await api.login(username, password); localStorage.setItem('aetheris_session', JSON.stringify(session)); onLogin(session) } catch (e) { setError(e instanceof Error ? e.message : '登录失败') } finally { setLoading(false) } }
  return <main className="login-shell"><form className="login-panel" onSubmit={submit}><h1>Aetheris 管理后台</h1><p className="muted">请使用管理员账号登录</p><label>用户名<input aria-label="用户名" value={username} onChange={e => setUsername(e.target.value)} required /></label><label>密码<input aria-label="密码" type="password" value={password} onChange={e => setPassword(e.target.value)} required /></label>{error && <p className="error" role="alert">{error}</p>}<button disabled={loading}>{loading ? '登录中...' : '登录'}</button></form></main>
}
