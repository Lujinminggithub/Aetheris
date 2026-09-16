import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import RAGQueryPage from './RAGQueryPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('智能查询页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('异步查询并通过引用打开活动证据', async () => {
    vi.spyOn(api, 'getRAGStatus').mockResolvedValue({ documents: 100, indexed: 96, pending: 4, failed: 0, embedding_model: 'embeddinggemma', vector_status: 'ready' })
    vi.spyOn(api, 'listActivities').mockResolvedValue({ devices: [{ id: 'device-1', label: 'DESKTOP', count: 10, last_activity_at: '2026-09-07T10:00:00Z' }], projects: [{ id: 'project-codex', label: 'Codex', count: 10, last_activity_at: '2026-09-07T10:00:00Z' }], activity_counts: { ai: 10, terminal: 0, ide: 0, browser: 0, version_control: 0, other: 0 }, activities: [], total: 10, limit: 1, offset: 0 })
    vi.spyOn(api, 'createRAGQuery').mockResolvedValue({ query_id: 'rag-1', status: 'queued', progress: 0 })
    vi.spyOn(api, 'getRAGQuery').mockResolvedValue({ query_id: 'rag-1', question: '最近修复了什么？', filters: { from: '2026-09-01', to: '2026-09-07' }, status: 'completed', progress: 100, answer: '修复了登录问题 [1]。', citations: [{ number: 1, document_id: 'doc-1', fact_id: 'fact-1', canonical_event_id: 'event-1', source_event_ids: ['event-1'], project_id: 'project-codex', project_name: 'Codex', activity_type: 'ai', excerpt: '修复登录问题', score: 0.9, occurred_at: '2026-09-07T10:00:00Z' }], created_at: '2026-09-07T10:00:00Z' })
    vi.spyOn(api, 'getEvent').mockResolvedValue({ event_id: 'event-1', subject_id: 'subject-1', device_id: 'device-1', event_type: 'ai.message', occurred_at: '2026-09-07T10:00:00Z', payload: { content: '修复登录问题' } })

    render(<RAGQueryPage />)
    expect(await screen.findByText(/已索引/)).toBeInTheDocument()
    await userEvent.type(screen.getByLabelText('查询问题'), '最近修复了什么？')
    await userEvent.selectOptions(screen.getByLabelText('项目范围'), 'project-codex')
    await userEvent.click(screen.getByRole('button', { name: '开始查询' }))
    expect(await screen.findByText('修复了登录问题 [1]。')).toBeInTheDocument()
    expect(api.createRAGQuery).toHaveBeenCalledWith(expect.objectContaining({ filters: expect.objectContaining({ project_id: 'project-codex' }) }))
    await userEvent.click(screen.getByRole('button', { name: '查看依据（1）' }))
    await userEvent.click(screen.getByRole('button', { name: /证据 1/ }))
    expect(await screen.findByText('查询引用证据')).toBeInTheDocument()
  })

  it('默认只显示简洁答案并折叠技术证据', async () => {
    vi.spyOn(api, 'listActivities').mockResolvedValue({ devices: [], projects: [], activity_counts: { ai: 1, terminal: 0, ide: 0, browser: 0, version_control: 0, other: 0 }, activities: [], total: 1, limit: 1, offset: 0 })
    vi.spyOn(api, 'getRAGStatus').mockResolvedValue({ documents: 1, indexed: 1, pending: 0, failed: 0, embedding_model: 'embeddinggemma', vector_status: 'ready' })
    vi.spyOn(api, 'createRAGQuery').mockResolvedValue({ query_id: 'query-direct', status: 'completed', progress: 100 })
    vi.spyOn(api, 'getRAGQuery').mockResolvedValue({ query_id: 'query-direct', question: '线路有没有流量限制', filters: { from: '2026-08-08', to: '2026-09-06' }, status: 'completed', progress: 100, answer: '线路存在流量限制。', answer_mode: 'direct', confidence: 'high', details: '', citation_numbers: [1], citations: [{ number: 1, document_id: 'doc-1', fact_id: 'fact-1', canonical_event_id: 'event-1', source_event_ids: ['event-1'], project_id: 'project-1', project_name: '线路', activity_type: 'ai', excerpt: 'Qdrant 向量索引技术细节', score: 0.9, occurred_at: '2026-09-06T10:00:00Z' }], created_at: '2026-09-06T10:00:00Z' })
    render(<RAGQueryPage />)
    await userEvent.type(await screen.findByLabelText('查询问题'), '线路有没有流量限制')
    await userEvent.click(screen.getByRole('button', { name: '开始查询' }))
    expect(await screen.findByText('线路存在流量限制。')).toBeInTheDocument()
    expect(screen.getByText('高置信度')).toBeInTheDocument()
    expect(screen.queryByText('Qdrant 向量索引技术细节')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '查看依据（1）' }))
    expect(screen.getByText('Qdrant 向量索引技术细节')).toBeInTheDocument()
    expect(screen.queryByText('embeddinggemma')).not.toBeInTheDocument()
  })
})
