import { render, screen } from '@testing-library/react'
import { beforeEach, expect, test, vi } from 'vitest'
import ProjectsPage from './ProjectsPage'
import { api } from '../api/client'

beforeEach(() => vi.restoreAllMocks())

test('displays logical project and location details', async () => {
  vi.spyOn(api, 'listProjects').mockResolvedValue({ projects: [{ id: 'logical-1', display_name: 'jtagent', vcs: 'git', status: 'active', metadata_revision: 3, location_count: 2, locations: [{ id: 'location-1', logical_project_id: 'logical-1', device_id: 'device-1', local_project_id: 'project-old', display_name: '主工作树', workspace_kind: 'primary', active: true, key_version: 1, metadata_revision: 3 }] }], count: 1 })
  render(<ProjectsPage />)
  expect(await screen.findByText('jtagent')).toBeInTheDocument()
  expect(screen.getByText('2 个位置')).toBeInTheDocument()
  expect(screen.getByText(/project-old/)).toBeInTheDocument()
})
