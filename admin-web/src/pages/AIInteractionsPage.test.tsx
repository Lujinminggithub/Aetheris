import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import AIInteractionsPage from './AIInteractionsPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('AI 交互页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('按设备、项目和消息角色展示交互并打开脱敏详情', async () => {
    vi.spyOn(api, 'listAIInteractions').mockResolvedValue({
      devices: [{ device_id: 'device-1', device_name: 'DESKTOP-DEV', event_count: 15, last_interaction_at: '2026-09-07T10:00:00Z' }],
      projects: [{ project_id: 'project-1', project_name: 'jtagent', event_count: 15, last_interaction_at: '2026-09-07T10:00:00Z' }],
      role_counts: { user: 6, assistant: 9, ai_tool: 1, system: 0, tool: 0, unknown: 0 },
      interactions: [{ event_id: 'event-1', device_id: 'device-1', device_name: 'DESKTOP-DEV', project_id: 'project-1', project_name: 'jtagent', message_role: 'assistant', interaction_kind: 'message', actor_origin: 'ai', tool: 'codex', occurred_at: '2026-09-07T10:00:00Z', content_preview: '已完成修复' }],
      total: 15, limit: 50, offset: 0,
    })
    vi.spyOn(api, 'getEvent').mockResolvedValue({ event_id: 'event-1', subject_id: 'subject-1', device_id: 'device-1', event_type: 'ai.message', occurred_at: '2026-09-07T10:00:00Z', payload: { role: 'assistant', content: '已完成修复，这是完整脱敏内容' } })

    render(<AIInteractionsPage />)

    expect((await screen.findAllByText('DESKTOP-DEV')).length).toBeGreaterThan(0)
    expect(screen.getAllByText('jtagent').length).toBeGreaterThan(0)
    expect(screen.getByText('用户发送')).toBeInTheDocument()
    expect(screen.getAllByText('AI 回复').length).toBeGreaterThan(0)
    expect(screen.getByText('已完成修复')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '查看完整内容' }))
    expect(await screen.findByText('已完成修复，这是完整脱敏内容')).toBeInTheDocument()
  })

  it('AI 工具执行只展示安全摘要和证据链', async () => {
    vi.spyOn(api, 'listAIInteractions').mockResolvedValue({
      devices: [], projects: [], role_counts: { user: 0, assistant: 0, ai_tool: 1, system: 0, tool: 0, unknown: 0 },
      interactions: [{ event_id: 'ai-tool-1', device_id: 'device-1', device_name: 'DESKTOP-DEV', project_id: 'project-1', project_name: 'jtagent', message_role: 'ai_tool', interaction_kind: 'tool_call', actor_origin: 'ai', tool: 'codex', occurred_at: '2026-09-07T10:00:00Z', content_preview: '执行版本控制操作', source_event_ids: ['ai-tool-1', 'terminal-1'] }],
      total: 1, limit: 50, offset: 0,
    })

    render(<AIInteractionsPage />)

    expect((await screen.findAllByText('AI 工具执行')).length).toBeGreaterThan(0)
    expect(screen.getByText('执行版本控制操作')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '查看完整内容' })).not.toBeInTheDocument()
    expect(screen.getByText('terminal-1')).toBeInTheDocument()
  })
})
