import { render, screen } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import WorkEpisodesPage from './WorkEpisodesPage'
import { api } from '../api/client'

beforeEach(() => vi.restoreAllMocks())

test('displays episode objective actions validation and evidence', async () => {
  vi.spyOn(api, 'listDevices').mockResolvedValue([{ id: 'device-1', name: '开发机', subject_id: 'subject-1', status: 'online' }])
  vi.spyOn(api, 'listWorkEpisodes').mockResolvedValue({ episodes: [{ episode_id: 'episode-1', subject_id: 'subject-1', device_id: 'device-1', project_id: 'logical-1', session_id: 'session-1', title: '修复登录', objective: '修复登录超时', context: '', outcome: '验证通过', confidence: 'high', needs_review: false, started_at: '2026-09-09T09:00:00Z', ended_at: '2026-09-09T09:05:00Z', actions: [{ event_id: 'event-2', actor: 'assistant', action_type: 'ai.tool_call', summary: '修改配置', occurred_at: '2026-09-09T09:02:00Z' }], validations: [{ event_id: 'event-3', validation_type: 'ide.test', result: '通过', summary: '测试通过', occurred_at: '2026-09-09T09:04:00Z' }], evidence: [{ event_id: 'event-1', section: 'objective', reason: '用户目标' }] }], count: 1, limit: 50, offset: 0 })
  render(<WorkEpisodesPage />)
  expect(await screen.findByText('修复登录超时')).toBeInTheDocument()
  expect(screen.getByText('修改配置')).toBeInTheDocument()
  expect(screen.getByText(/测试通过/)).toBeInTheDocument()
  expect(screen.getByText('event-1')).toBeInTheDocument()
})
