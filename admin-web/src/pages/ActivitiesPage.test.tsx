import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import ActivitiesPage from './ActivitiesPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('活动记录页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('统一展示 AI、终端、IDE、浏览器和版本控制活动', async () => {
    vi.spyOn(api, 'listActivities').mockResolvedValue({
      devices: [{ id: 'device-1', label: 'DESKTOP-DEV', count: 5, last_activity_at: '2026-09-07T10:00:00Z' }],
      projects: [{ id: 'project-codex', label: 'Codex', count: 5, last_activity_at: '2026-09-07T10:00:00Z' }],
      activity_counts: { ai: 1, terminal: 1, ide: 1, browser: 1, version_control: 1, other: 0 },
      activities: [{ fact_id: 'fact-1', canonical_event_id: 'event-1', source_event_ids: ['event-1'], device_id: 'device-1', device_name: 'DESKTOP-DEV', project_id: 'project-codex', project_name: 'Codex', activity_type: 'ai', event_type: 'ai.message', source: 'core.ai.codex', actor_origin: 'human', message_role: 'user', tool: 'codex', preview: '修复登录问题', occurred_at: '2026-09-07T10:00:00Z' }],
      total: 101, limit: 50, offset: 0,
    })
    vi.spyOn(api, 'getEvent').mockResolvedValue({ event_id: 'event-1', subject_id: 'subject-1', device_id: 'device-1', event_type: 'ai.message', occurred_at: '2026-09-07T10:00:00Z', payload: { role: 'user', content: '修复登录问题' } })

    render(<ActivitiesPage />)

    expect(await screen.findByText('修复登录问题')).toBeInTheDocument()
    for (const label of ['AI 协作', '终端', 'IDE', '浏览器', '版本控制']) expect(screen.getAllByText(label).length).toBeGreaterThan(0)
    expect(screen.getAllByText('Codex').length).toBeGreaterThan(0)
    await userEvent.click(screen.getByRole('button', { name: '下一页' }))
    expect(api.listActivities).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 50 }))
    await userEvent.click(screen.getByRole('button', { name: '查看证据' }))
    expect(await screen.findByText('活动证据')).toBeInTheDocument()
  })
})
