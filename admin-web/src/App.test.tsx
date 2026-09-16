import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App'
import { api } from './api/client'

vi.mock('./api/client', async () => {
  const actual = await vi.importActual<typeof import('./api/client')>('./api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('Admin Web', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })
  afterEach(() => cleanup())

  it('未登录时显示登录页', () => {
    window.history.pushState({}, '', '/')
    render(<App />)
    expect(screen.getByRole('heading', { name: 'Aetheris 管理后台' })).toBeInTheDocument()
    expect(screen.getByLabelText('用户名')).toBeInTheDocument()
  })

  it('登录后显示总览 KPI、活动和模型健康状态', async () => {
    localStorage.setItem('aetheris_session', JSON.stringify({ user_id: 'u1', username: '管理员', access_role: 'admin' }))
    vi.spyOn(api, 'getSummary').mockResolvedValue({ total_subjects: 4, total_devices: 8, events_today: 12, active_devices: 7, activity: [{ id: 'e1', event_type: '检查完成', occurred_at: '2026-09-06T10:00:00Z' }] })
    vi.spyOn(api, 'getProviderHealth').mockResolvedValue([{ name: '主模型', provider: 'internal', status: 'healthy', latency_ms: 120 }])
    vi.spyOn(api, 'listActivities').mockResolvedValue({ devices: [], projects: [], activity_counts: { ai: 0, terminal: 0, ide: 0, browser: 0, version_control: 0, other: 0 }, activities: [], total: 0, limit: 50, offset: 0 })
    render(<App />)
    expect(await screen.findByText('主体总数')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
    expect(screen.getByText('主模型')).toBeInTheDocument()
    expect(screen.getByText('检查完成')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '下载 Windows 安装程序' })).toHaveAttribute('href', '/downloads/client')
    expect(screen.getByRole('button', { name: '智能查询' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '浏览器采集' })).toBeInTheDocument()
  })

  it('只提供统一活动记录入口并打开活动证据', async () => {
    localStorage.setItem('aetheris_session', JSON.stringify({ user_id: 'u1', username: '管理员', access_role: 'admin' }))
    vi.spyOn(api, 'getSummary').mockResolvedValue({ total_subjects: 0, total_devices: 0, events_today: 0, active_devices: 0, activity: [] })
    vi.spyOn(api, 'getProviderHealth').mockResolvedValue([])
    vi.spyOn(api, 'listActivities').mockResolvedValue({ devices: [], projects: [], activity_counts: { ai: 0, terminal: 0, ide: 0, browser: 0, version_control: 0, other: 1 }, activities: [{ fact_id: 'fact-1', canonical_event_id: 'evt-1', source_event_ids: ['evt-1'], device_id: 'dev-1', device_name: 'DESKTOP', project_id: 'project-1', project_name: 'project-1', activity_type: 'other', event_type: '异常告警', source: 'core.process', actor_origin: 'unknown', message_role: 'unknown', preview: '温度过高', occurred_at: '2026-09-06T10:00:00Z' }], total: 1, limit: 50, offset: 0 })
    vi.spyOn(api, 'getEvent').mockResolvedValue({ event_id: 'evt-1', subject_id: 'sub-1', device_id: 'dev-1', event_type: '异常告警', occurred_at: '2026-09-06T10:00:00Z', work_role: '研发', summary: '温度过高', payload: { temperature: 88 } })
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: '活动记录' })).toBeInTheDocument())
    expect(screen.queryByRole('button', { name: 'AI 交互' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '事件' })).not.toBeInTheDocument()
    screen.getByRole('button', { name: '活动记录' }).click()
    expect(await screen.findByText('异常告警')).toBeInTheDocument()
    screen.getByRole('button', { name: '查看证据' }).click()
    expect(await screen.findByText('温度过高')).toBeInTheDocument()
    expect(screen.getByText('脱敏 payload')).toBeInTheDocument()
    const drawer = document.querySelector<HTMLElement>('.drawer')
    expect(drawer).not.toBeNull()
  })
})
