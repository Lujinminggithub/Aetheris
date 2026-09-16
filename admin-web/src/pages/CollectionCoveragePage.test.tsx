import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import CollectionCoveragePage from './CollectionCoveragePage'
import { api } from '../api/client'

beforeEach(() => vi.restoreAllMocks())
afterEach(() => cleanup())

test('displays adapter state stage and safe error code', async () => {
  vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [{ device_id: 'device-1', adapter_id: 'browser', state: 'error', capability_version: '1', detected_format: '', discovered: 0, parsed: 0, skipped: 0, failed: 1, lag_seconds: 0, error_code: 'uia_unavailable', error_stage: 'url_read' }], count: 1 })
  render(<CollectionCoveragePage />)
  expect(await screen.findByText('浏览器')).toBeInTheDocument()
  expect(screen.getByText('采集错误')).toBeInTheDocument()
  expect(screen.getByText('url_read')).toBeInTheDocument()
  expect(screen.getByText('uia_unavailable')).toBeInTheDocument()
})

test('treats a non-browser foreground marker as idle instead of a URL error', async () => {
  vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [{ device_id: 'device-1', adapter_id: 'browser', state: 'error', capability_version: '1', detected_format: 'foreground:vmware.exe', discovered: 0, parsed: 0, skipped: 0, failed: 1, lag_seconds: 0, error_code: 'url_unavailable', error_stage: 'url_read' }], count: 1 })
  const view = render(<CollectionCoveragePage />)
  expect(await view.findByText('浏览器')).toBeInTheDocument()
  expect(view.getByText('空闲')).toBeInTheDocument()
  expect(view.getByText('未检测到浏览器前台窗口')).toBeInTheDocument()
  expect(view.container).not.toHaveTextContent('vmware.exe')
  expect(view.container.querySelector('.status-down')).toBeNull()
})

test('shows only the approved Edge browser marker and captured domain', async () => {
  vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [{ device_id: 'device-1', adapter_id: 'browser', state: 'active', capability_version: '1', detected_format: 'foreground:msedge.exe:baidu.com', discovered: 1, parsed: 1, skipped: 0, failed: 0, lag_seconds: 0 }], count: 1 })
  const view = render(<CollectionCoveragePage />)
  expect(await view.findByText('浏览器')).toBeInTheDocument()
  expect(view.getByText('采集中')).toBeInTheDocument()
  expect(view.getByText('foreground:msedge.exe:baidu.com')).toBeInTheDocument()
})

test('shows application OCR fallback counters and controls tenant policy', async () => {
  vi.spyOn(api, 'listAdapterHealth').mockResolvedValue({ items: [{ device_id: 'device-1', adapter_id: 'application_ocr_fallback', state: 'active', capability_version: '1', detected_format: 'ocr_fallback:captured', discovered: 4, parsed: 2, skipped: 1, failed: 1, lag_seconds: 0 }], count: 1 })
  vi.spyOn(api, 'getApplicationCapturePolicy').mockResolvedValue({ enabled: true, revision: 3 })
  const update = vi.spyOn(api, 'updateApplicationCapturePolicy').mockResolvedValue({ enabled: false, revision: 4 })

  const view = render(<CollectionCoveragePage />)

  expect((await screen.findAllByText('应用 OCR 保底')).length).toBe(2)
  expect(screen.getByText('候选 4 · 成功 2 · 跳过 1 · 失败 1')).toBeInTheDocument()
  expect(view.container).not.toHaveTextContent('ocr_fallback:captured')
  await userEvent.click(screen.getByRole('checkbox', { name: '启用应用 OCR 保底' }))
  expect(update).toHaveBeenCalledWith({ enabled: false })
})
