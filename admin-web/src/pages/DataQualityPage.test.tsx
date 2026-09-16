import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import DataQualityPage from './DataQualityPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('数据质量页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('展示可解释质量数量、原因码和证据链', async () => {
    vi.spyOn(api, 'getDataQualitySummary').mockResolvedValue({ raw_events: 100, clean_facts: 95, merged_facts: 3, command_fragments: 2, quarantined_facts: 2, excluded_facts: 2, rule_version: 1 })
    vi.spyOn(api, 'listDataQualityFacts').mockResolvedValue({ facts: [{ fact_id: 'fact-1', fact_type: 'command_fragment', actor_origin: 'unknown', project_id: 'project-1', project_name: 'jtagent', device_id: 'device-1', device_name: 'DESKTOP-DEV', occurred_at: '2026-09-07T10:00:00Z', quality_state: 'quarantined', confidence: 'low', merge_method: 'none', reason_codes: ['orphan_parameter_fragment'], source_event_ids: ['event-a', 'event-b'], command_display: '-Filter *.dll', excluded_from_effectiveness: true }], total: 1, limit: 50, offset: 0 })
    vi.spyOn(api, 'recomputeDataQuality').mockResolvedValue({ job_id: 'cleaning-1', status: 'queued' })
    vi.spyOn(api, 'getEvent').mockResolvedValue({ event_id: 'event-a', event_type: 'terminal.command', device_id: 'device-1', subject_id: 'subject-1', occurred_at: '2026-09-07T10:00:00Z', work_role: '研发', payload: { command: '-Filter *.dll' } })

    render(<DataQualityPage />)

    expect(await screen.findByText('原始事件')).toBeInTheDocument()
    expect(screen.getByText('100')).toBeInTheDocument()
    expect(screen.getByText('orphan_parameter_fragment')).toBeInTheDocument()
    expect(screen.getByText('event-a')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'event-a' }))
    expect(await screen.findByText('原始证据详情')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '重算清洗事实' }))
    expect(await screen.findByText(/cleaning-1/)).toBeInTheDocument()
    expect(screen.queryByText(/质量总分|排名/)).not.toBeInTheDocument()
  })
})
