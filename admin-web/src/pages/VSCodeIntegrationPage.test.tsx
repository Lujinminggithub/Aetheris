import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import VSCodeIntegrationPage from './VSCodeIntegrationPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('VS Code 组件管理页', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('区分用户停用和组件故障并展示设备维度', async () => {
    vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [
      { device_id: 'd1', adapter_id: 'vscode_extension', component_state: 'paused_by_user', component_version: '0.1.0', protocol_version: 1, state: 'disabled', capability_version: '1', discovered: 0, parsed: 0, skipped: 0, failed: 0, lag_seconds: 0, pending_events: 0 },
      { device_id: 'd2', adapter_id: 'vscode_extension', component_state: 'bridge_offline', component_version: '0.1.0', protocol_version: 1, state: 'error', capability_version: '1', discovered: 0, parsed: 0, skipped: 0, failed: 1, lag_seconds: 180, pending_events: 3 },
    ], count: 2 })

    render(<VSCodeIntegrationPage />)

    expect(await screen.findByRole('heading', { name: 'VS Code 采集组件' })).toBeInTheDocument()
    expect(screen.getByText('用户已停用')).toBeInTheDocument()
    expect(screen.getByText('桥接离线')).toBeInTheDocument()
    expect(screen.getByText('d1')).toBeInTheDocument()
    expect(screen.getByText('d2')).toBeInTheDocument()
  })
})
