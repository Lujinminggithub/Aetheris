import { describe, expect, it } from 'vitest'
import { createComponentHeartbeat, diffExtensions, snapshotExtensions } from '../src/extension_inventory'

describe('VS Code 扩展状态', () => {
  it('只保留扩展标识版本和激活状态', () => {
    const snapshot = snapshotExtensions([{
      id: 'vendor.tool', isActive: true,
      packageJSON: { version: '1.2.3', configuration: { secret: 'must-not-leak' } },
      exports: { token: 'private' },
    }])

    expect(snapshot).toEqual([{ id: 'vendor.tool', version: '1.2.3', is_active: true }])
    expect(JSON.stringify(snapshot)).not.toContain('must-not-leak')
    expect(JSON.stringify(snapshot)).not.toContain('private')
  })

  it('区分安装更新移除和激活变化', () => {
    const previous = [
      { id: 'vendor.updated', version: '1.0.0', is_active: false },
      { id: 'vendor.removed', version: '2.0.0', is_active: true },
      { id: 'vendor.activation', version: '1.0.0', is_active: false },
    ]
    const current = [
      { id: 'vendor.updated', version: '1.1.0', is_active: true },
      { id: 'vendor.installed', version: '1.0.0', is_active: false },
      { id: 'vendor.activation', version: '1.0.0', is_active: true },
    ]

    expect(diffExtensions(previous, current, () => 'event-id', () => new Date('2026-09-14T08:00:00Z')).map(event => event.change)).toEqual([
      'installed', 'removed', 'updated', 'activation_changed',
    ])
  })

  it('心跳不包含路径或文件信息', () => {
    const heartbeat = createComponentHeartbeat({
      sessionId: 'session-1', extensionVersion: '0.1.0', vscodeVersion: '1.96.0', hostKind: 'local',
      pendingEvents: 3, sentEvents: 7, droppedEvents: 1,
    })

    expect(heartbeat).toMatchObject({ type: 'heartbeat', version: 1, extension_id: 'aetheris.aetheris-vscode', pending_events: 3 })
    expect(JSON.stringify(heartbeat)).not.toMatch(/path|file|content/i)
  })
})
