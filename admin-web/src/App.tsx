import { useEffect, useState, type ReactElement } from 'react'
import type { Session } from './api/types'
import LoginPage from './pages/LoginPage'
import DashboardPage from './pages/DashboardPage'
import DevicesPage from './pages/DevicesPage'
import WorkRolesPage from './pages/WorkRolesPage'
import AuditPage from './pages/AuditPage'
import EffectivenessPage from './pages/EffectivenessPage'
import ActivitiesPage from './pages/ActivitiesPage'
import RAGQueryPage from './pages/RAGQueryPage'
import DataQualityPage from './pages/DataQualityPage'
import BrowserPolicyPage from './pages/BrowserPolicyPage'
import WorkEpisodesPage from './pages/WorkEpisodesPage'
import CollectionCoveragePage from './pages/CollectionCoveragePage'
import ProjectsPage from './pages/ProjectsPage'
import VSCodeIntegrationPage from './pages/VSCodeIntegrationPage'
import ProcessKnowledgePage from './pages/ProcessKnowledgePage'
import './styles.css'

export default function App() {
  const [session, setSession] = useState<Session | null>(() => {
    try { return JSON.parse(localStorage.getItem('aetheris_session') || 'null') } catch { return null }
  })
  const [page, setPage] = useState('dashboard')

  useEffect(() => {
    const logout = () => { localStorage.removeItem('aetheris_session'); setSession(null) }
    window.addEventListener('aetheris:unauthorized', logout)
    return () => window.removeEventListener('aetheris:unauthorized', logout)
  }, [])

  if (!session) return <LoginPage onLogin={setSession} />
  const pages: Record<string, ReactElement> = {
    dashboard: <DashboardPage />, effectiveness: <EffectivenessPage />, vscode: <VSCodeIntegrationPage />, devices: <DevicesPage />,
    roles: <WorkRolesPage />, activities: <ActivitiesPage />, rag: <RAGQueryPage />, processKnowledge: <ProcessKnowledgePage />, quality: <DataQualityPage />, browserPolicy: <BrowserPolicyPage />, projects: <ProjectsPage />, episodes: <WorkEpisodesPage />, coverage: <CollectionCoveragePage />, audit: <AuditPage />,
  }
  const navigation = [['dashboard', '概览'], ['effectiveness', '个人效能'], ['vscode', 'VS Code 组件'], ['projects', '项目管理'], ['episodes', '工作片段'], ['activities', '活动记录'], ['coverage', '采集覆盖'], ['rag', '智能查询'], ['processKnowledge', '过程知识'], ['quality', '数据质量'], ['browserPolicy', '浏览器采集'], ['devices', '设备与主体'], ['roles', '工作角色'], ['audit', '审计日志']]

  return <div className="app-shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark">A</span>Aetheris</div>
      <nav>{navigation.map(([id, label]) => <button className={page === id ? 'active' : ''} key={id} onClick={() => setPage(id)}>{label}</button>)}</nav>
      <button className="logout" onClick={() => { localStorage.removeItem('aetheris_session'); setSession(null) }}>退出登录</button>
    </aside>
    <main className="content">
      <header><a className="client-download" href="/downloads/client">下载 Windows 安装程序</a><span className="user-name">{session.username}</span><span className="muted">{session.access_role}</span></header>
      {pages[page]}
    </main>
  </div>
}
