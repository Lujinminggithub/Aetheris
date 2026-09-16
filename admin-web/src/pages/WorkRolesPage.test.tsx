import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import WorkRolesPage from './WorkRolesPage'

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client')
  return { ...actual, api: { ...actual.api } }
})

describe('工作角色页面', () => {
  afterEach(() => { cleanup(); vi.restoreAllMocks() })

  it('从服务端主体列表选择用户并提交内部主体 ID', async () => {
    vi.spyOn(api, 'listWorkRoles').mockResolvedValue([])
    vi.spyOn(api, 'listSubjects').mockResolvedValue([{ id: 'subject-1', name: 'DESKTOP\\LuJinming', device_count: 1 }])
    const assign = vi.spyOn(api, 'assignWorkRole').mockResolvedValue({ id: 'assignment-1', subject_id: 'subject-1', role: '研发' })

    render(<WorkRolesPage />)
    expect(await screen.findByRole('option', { name: /DESKTOP\\LuJinming/ })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '分配角色' }))

    expect(assign).toHaveBeenCalledWith({ subject_id: 'subject-1', role: '研发' })
    expect(await screen.findByText('工作角色已保存')).toBeInTheDocument()
  })
})
