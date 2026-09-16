import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import EffectivenessPage from './EffectivenessPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('个人效能页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('展示可解释指标、趋势和数据覆盖提示且不展示排名总分', async () => {
    vi.spyOn(api, 'listEffectivenessSubjects').mockResolvedValue([{ subject_id: 's1', display_name: '研发一组', active_days: 2, active_window_minutes: 60, event_count: 8, coverage_ratio: 0.2 }])
    vi.spyOn(api, 'listProjects').mockResolvedValue({ projects: [{ id: 'logical-project-1', display_name: 'jtagent', vcs: 'git', status: 'active', metadata_revision: 1, location_count: 1, locations: [{ id: 'loc-1', logical_project_id: 'logical-project-1', device_id: 'device-1', local_project_id: 'p1', display_name: 'jtagent', workspace_kind: 'primary', active: true, key_version: 1, metadata_revision: 1 }] }], count: 1 })
    vi.spyOn(api, 'getEffectiveness').mockResolvedValue({
      subject_id: 's1', from: '2026-08-08', to: '2026-09-06', timezone: 'Asia/Shanghai', metric_definition_version: 1,
      totals: { active_days: 2, active_window_minutes: 60, session_count: 3, average_session_minutes: 20, longest_session_minutes: 30, focus_block_count: 1, focus_block_minutes: 25, context_switch_count: 2, delivery_events: 2, coding_events: 3, terminal_events: 1, ai_collaboration_events: 2, browser_events: 0, other_events: 0 },
      daily: [{ local_date: '2026-09-06', active_window_minutes: 60, session_count: 3, delivery_events: 2, coding_events: 3, terminal_events: 1, ai_collaboration_events: 2, browser_events: 0, other_events: 0 }],
      project_breakdown: { p1: { event_count: 8, active_window_minutes: 60 } }, work_role_breakdown: { '研发': { event_count: 8, active_window_minutes: 60 } },
      activity_breakdown: { delivery: 2, coding: 3, terminal: 1, ai_collaboration: 2, browser: 0, other: 0 }, trends: {},
      coverage: { covered_days: 2, period_days: 30, coverage_ratio: 0.2, source_count: 3, device_count: 1, insufficient: true },
      evidence_event_ids: ['e1'], definitions: { active_window_minutes: '有事件的不重复 5 分钟窗口，不等同于工时' },
    })
    render(<EffectivenessPage />)
    expect(await screen.findByRole('heading', { name: '个人效能' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '7 天' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '30 天' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '90 天' })).toBeInTheDocument()
    expect(await screen.findByText('活跃时段')).toBeInTheDocument()
    expect(screen.getAllByText(/不等同于工时/).length).toBeGreaterThan(0)
    expect(screen.getByText('每日趋势')).toBeInTheDocument()
    expect(screen.getByText('活动构成')).toBeInTheDocument()
    expect(screen.getByText('浏览器活动')).toBeInTheDocument()
    expect(screen.getByText(/数据不足/)).toBeInTheDocument()
    expect(screen.getByText('jtagent')).toBeInTheDocument()
    expect(screen.queryByText('p1')).not.toBeInTheDocument()
    expect(screen.queryByText(/员工排名|综合总分/)).not.toBeInTheDocument()
  })

  it('切换统计周期后重新加载且页面不会空白', async () => {
    vi.spyOn(api, 'listEffectivenessSubjects').mockResolvedValue([{ subject_id: 's1', display_name: '研发一组', active_days: 1, active_window_minutes: 30, event_count: 2, coverage_ratio: 0.1 }])
    const getEffectiveness = vi.spyOn(api, 'getEffectiveness').mockResolvedValue(reportFixture())

    render(<EffectivenessPage />)
    expect(await screen.findByText('每日趋势')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '7 天' }))

    await waitFor(() => expect(getEffectiveness).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('heading', { name: '个人效能' })).toBeInTheDocument()
    expect(screen.getByText('每日趋势')).toBeInTheDocument()
  })
})

function reportFixture() {
  return {
    subject_id: 's1', from: '2026-09-01', to: '2026-09-07', timezone: 'Asia/Shanghai', metric_definition_version: 1,
    totals: { active_days: 1, active_window_minutes: 30, session_count: 1, average_session_minutes: 30, longest_session_minutes: 30, focus_block_count: 1, focus_block_minutes: 25, context_switch_count: 0, delivery_events: 0, coding_events: 1, terminal_events: 1, ai_collaboration_events: 0, browser_events: 0, other_events: 0 },
    daily: [], project_breakdown: {}, work_role_breakdown: {}, activity_breakdown: { delivery: 0, coding: 1, terminal: 1, ai_collaboration: 0, browser: 0, other: 0 }, trends: {},
    coverage: { covered_days: 1, period_days: 7, coverage_ratio: 0.14, source_count: 1, device_count: 1, insufficient: true },
    evidence_event_ids: [], definitions: {},
  }
}
