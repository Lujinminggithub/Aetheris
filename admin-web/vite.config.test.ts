import { describe, expect, it } from 'vitest'
import { ADMIN_BASE_PATH } from './vite.config'

describe('Admin Web 部署路径', () => {
  it('构建资源必须挂在 Go Server 的 /admin/ 静态路径下', () => {
    expect(ADMIN_BASE_PATH).toBe('/admin/')
  })
})

