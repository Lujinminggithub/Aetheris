import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './client'

describe('admin API client', () => {
  afterEach(() => vi.restoreAllMocks())

  it('请求总览、模型健康和事件详情接口', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => new Response(JSON.stringify({}), { status: 200, headers: { 'content-type': 'application/json' } }))
    await api.getSummary()
    await api.getProviderHealth()
    await api.getEvent('evt/1')
    expect(fetchMock.mock.calls.map(([input]) => String(input))).toEqual([
      '/api/v1/admin/summary',
      '/api/v1/admin/health/providers',
      '/api/v1/admin/events/evt%2F1',
    ])
  })

  it('读取并更新浏览器采集策略', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify({ enabled: false, allowed_domains: [], revision: 0 }), { status: 200, headers: { 'content-type': 'application/json' } }))
    await api.getBrowserPolicy()
    await api.updateBrowserPolicy({ enabled: true, allowed_domains: ['docs.example.com'] })
    expect(fetchMock.mock.calls.map(([input, init]) => [String(input), init?.method || 'GET'])).toEqual([
      ['/api/v1/admin/browser-policy', 'GET'],
      ['/api/v1/admin/browser-policy', 'PUT'],
    ])
  })

  it('登录凭据错误时显示明确的中文原因', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(
      JSON.stringify({ error: 'invalid_credentials', message: '用户名或密码错误' }),
      { status: 401, headers: { 'content-type': 'application/json' } },
    ))
    await expect(api.login('admin', 'wrong')).rejects.toThrow('用户名或密码错误')
  })

  it('将事件中的角色快照转换为管理端可直接展示的角色名', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      events: [{
        event_id: 'evt-1', subject_id: 'subject-1', device_id: 'device-1', event_type: 'terminal.command',
        occurred_at: '2026-09-07T10:00:00+08:00', work_role: { role_id: 'role-dev', code: '研发', version: 1, source: 'admin_assignment' },
      }],
      count: 1,
    }), { status: 200, headers: { 'content-type': 'application/json' } }))

    await expect(api.listEvents()).resolves.toMatchObject([{ event_id: 'evt-1', work_role: '研发' }])
  })
})
