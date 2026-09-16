import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { Spool } from '../src/spool'
import type { BehaviorEvent } from '../src/protocol'

const directories: string[] = []

async function temporarySpool(): Promise<{ spool: Spool; file: string }> {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'aetheris-vscode-spool-'))
  directories.push(directory)
  const file = path.join(directory, 'events.jsonl')
  return { spool: new Spool(file), file }
}

function event(eventId: string, occurredAt: string, fileName = 'app.ts'): BehaviorEvent {
  return { event_id: eventId, event_type: 'ide.file_opened', occurred_at: occurredAt, file_name: fileName }
}

afterEach(async () => {
  await Promise.all(directories.splice(0).map(directory => fs.rm(directory, { recursive: true, force: true })))
})

describe('扩展离线缓存', () => {
  it('按二十四小时淘汰旧事件', async () => {
    const { spool } = await temporarySpool()
    await spool.enqueue(event('old', '2026-09-13T06:00:00Z'))
    await spool.enqueue(event('new', '2026-09-14T07:00:00Z'))

    await spool.compact({ now: Date.parse('2026-09-14T08:00:00Z'), maxBytes: 8 * 1024 * 1024, maxAgeMs: 24 * 60 * 60 * 1000 })

    expect(await spool.ids()).toEqual(['new'])
  })

  it('按字节上限先进先出淘汰', async () => {
    const { spool } = await temporarySpool()
    await spool.enqueue(event('first', '2026-09-14T07:00:00Z', 'a'.repeat(120)))
    await spool.enqueue(event('second', '2026-09-14T07:01:00Z', 'b'.repeat(120)))

    await spool.compact({ now: Date.parse('2026-09-14T08:00:00Z'), maxBytes: 320, maxAgeMs: 24 * 60 * 60 * 1000 })

    expect(await spool.ids()).toEqual(['second'])
  })

  it('持久化文件不包含正文型字段', async () => {
    const { spool, file } = await temporarySpool()
    await spool.enqueue(event('safe', '2026-09-14T07:00:00Z'))
    const raw = await fs.readFile(file, 'utf8')
    expect(raw).not.toMatch(/source_text|file_content|clipboard|terminal_output/)
  })
})
