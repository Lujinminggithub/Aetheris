import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import BrowserPolicyPage from './BrowserPolicyPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('浏览器采集策略页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('加载并保存域名白名单', async () => {
    vi.spyOn(api, 'getBrowserPolicy').mockResolvedValue({ enabled: false, allowed_domains: [], revision: 2, updated_at: '2026-09-08T01:00:00Z' })
    const update = vi.spyOn(api, 'updateBrowserPolicy').mockResolvedValue({ enabled: true, allowed_domains: ['docs.example.com'], revision: 3, updated_at: '2026-09-08T01:01:00Z' })
    render(<BrowserPolicyPage />)

    expect(await screen.findByRole('heading', { name: '浏览器采集' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('checkbox', { name: '启用浏览器活动采集' }))
    await userEvent.type(screen.getByLabelText('允许采集的域名'), 'docs.example.com')
    await userEvent.click(screen.getByRole('button', { name: '保存策略' }))

    expect(update).toHaveBeenCalledWith({ enabled: true, allowed_domains: ['docs.example.com'] })
    expect(await screen.findByText(/策略已保存/)).toBeInTheDocument()
  })
})
