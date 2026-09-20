import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import PublicKnowledgePage from './PublicKnowledgePage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

const unit = {
  public_knowledge_id: 'public-1', canonical_topic: 'Windows EDR', knowledge_type: 'implementation_pattern' as const,
  publication_state: 'pending_review' as const, current_revision: 2,
  current: {
    revision: 2, problem_pattern: '如何实现 EDR', conclusion: '内核侧轻量采集，用户态完成关联分析。', rationale: '集成测试通过',
    applicability: 'Windows 11', caveats: '高 IRQL 路径禁止阻塞', alternatives: '', validation_state: 'evidence_verified' as const,
    anonymous_source_tenant_count: 3, independent_session_count: 7, canonical_hash: 'hash', extractor_version: 'v1', redaction_version: 'v1', review_policy_version: 'v1', created_at: '2026-09-20T10:00:00Z',
  }, reviews: [], private_evidence: [], created_at: '2026-09-20T10:00:00Z', updated_at: '2026-09-20T10:00:00Z',
}

describe('知识治理页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('显示治理队列并由平台管理员认证知识', async () => {
    vi.spyOn(api, 'getPublicKnowledgeSummary').mockResolvedValue({ candidate: 1, pending_review: 2, published: 3, suspended: 0, withdrawn: 1, open_conflicts: 1, active_jobs: 0 })
    vi.spyOn(api, 'listPublicKnowledgeUnits').mockResolvedValue({ units: [unit], total: 1, limit: 50, offset: 0 })
    vi.spyOn(api, 'getPublicKnowledgeUnit').mockResolvedValue(unit)
    vi.spyOn(api, 'reviewPublicKnowledge').mockResolvedValue({ ...unit, current: { ...unit.current, validation_state: 'platform_certified' } })

    render(<PublicKnowledgePage session={{ user_id: 'admin', username: '管理员', access_role: 'platform_admin', permissions: { 'knowledge:certify_public': true, 'knowledge:diagnose': true } }} />)

    expect(await screen.findByText('Windows EDR')).toBeInTheDocument()
    for (const label of ['待来源确认', '待证据验证', '跨租户印证', '待平台认证', '已发布', '冲突与撤回']) {
      expect(screen.getByRole('tab', { name: new RegExp(label) })).toBeInTheDocument()
    }
    expect(screen.getByText('3 个租户 / 7 个会话')).toBeInTheDocument()
    await userEvent.click(screen.getByText('Windows EDR'))
    await userEvent.type(screen.getByLabelText('审核理由'), '证据、适用条件和隐私边界均已复核')
    await userEvent.click(screen.getByRole('button', { name: '平台认证' }))
    expect(api.reviewPublicKnowledge).toHaveBeenCalledWith('public-1', 'certify', { expected_revision: 2, reason: '证据、适用条件和隐私边界均已复核' })
  })
})
