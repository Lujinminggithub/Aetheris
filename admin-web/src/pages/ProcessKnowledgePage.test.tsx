import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import ProcessKnowledgePage from './ProcessKnowledgePage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('过程知识页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('展示验证状态并按逻辑项目提交回填', async () => {
    vi.spyOn(api, 'listProjects').mockResolvedValue({ projects: [{ id: 'logical-safe', display_name: 'safe', vcs: 'git', status: 'active', metadata_revision: 1, location_count: 1, locations: [] }], count: 1 })
    vi.spyOn(api, 'getProcessKnowledgeSummary').mockResolvedValue({ sessions: 10, units: 8, verified: 3, conflicts: 1, unattributed: 2, chunks: 16, active_version: 1, mode: 'shadow' })
    vi.spyOn(api, 'listProcessKnowledgeUnits').mockResolvedValue({ units: [{ knowledge_id: 'knowledge-1', session_id: 'session-1', logical_project_id: 'logical-safe', topic: 'Windows EDR', knowledge_type: 'implementation_pattern', problem: '如何实现 EDR', intent: '形成架构', constraints: '', conclusion: '内核采集与用户态分析协作', rationale: '', alternatives: '', applicability: 'Windows', caveats: '', decision_state: 'accepted', validation_state: 'verified', lifecycle_state: 'active', occurred_at: '2026-09-17T00:00:00Z', revision: 1, version: 1, evidence: [] }], total: 1, limit: 50, offset: 0 })
    vi.spyOn(api, 'startProcessKnowledgeBackfill').mockResolvedValue({ id: 'job-1', state: 'pending' })
    vi.spyOn(api, 'activateProcessKnowledgeVersion').mockResolvedValue(undefined)
    render(<ProcessKnowledgePage />)
    expect(await screen.findByText('Windows EDR')).toBeInTheDocument()
    expect(screen.getAllByText('已验证').length).toBeGreaterThan(0)
    await userEvent.selectOptions(screen.getByLabelText('知识项目'), 'logical-safe')
    await userEvent.click(screen.getByRole('button', { name: '回填过程知识' }))
    expect(api.startProcessKnowledgeBackfill).toHaveBeenCalledWith('apply', 'logical-safe', 2)
    expect(await screen.findByText(/job-1/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '切换为 Shadow' }))
    expect(api.activateProcessKnowledgeVersion).toHaveBeenCalledWith(2, 'shadow', 0)
  })
})
