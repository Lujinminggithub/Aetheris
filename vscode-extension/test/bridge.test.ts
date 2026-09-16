import fs from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, expect, it } from 'vitest'
import { BridgeClient, type BridgeTransport } from '../src/bridge'
import { Spool } from '../src/spool'
import type { BehaviorEvent, BridgeEnvelope } from '../src/protocol'

const directories: string[] = []

async function fixture(transport: BridgeTransport) {
  const directory = await fs.mkdtemp(path.join(os.tmpdir(), 'aetheris-vscode-bridge-'))
  directories.push(directory)
  const spool = new Spool(path.join(directory, 'events.jsonl'))
  return { spool, client: new BridgeClient(spool, transport, 'session-1') }
}

function event(id: string): BehaviorEvent {
  return { event_id: id, event_type: 'ide.file_opened', occurred_at: '2026-09-14T08:00:00Z', file_name: 'app.ts' }
}

afterEach(async () => {
  await Promise.all(directories.splice(0).map(directory => fs.rm(directory, { recursive: true, force: true })))
})

describe('命名管道客户端', () => {
  it('部分确认只删除服务端已接受事件', async () => {
    const sent: BridgeEnvelope[] = []
    const transport: BridgeTransport = { send: async envelope => {
      sent.push(envelope)
      return { version: 1, accepted_ids: ['event-1'], rejected: [{ event_id: 'event-2', reason_code: 'path_not_authorized' }] }
    } }
    const { spool, client } = await fixture(transport)
    await spool.enqueue(event('event-1'))
    await spool.enqueue(event('event-2'))

    const result = await client.flush()

    expect(result).toEqual({ accepted: 1, rejected: 1 })
    expect(sent[0].events?.map(item => item.event_id)).toEqual(['event-1', 'event-2'])
    expect(await spool.ids()).toEqual(['event-2'])
  })

  it('连接失败时保留全部事件', async () => {
    const transport: BridgeTransport = { send: async () => { throw new Error('offline') } }
    const { spool, client } = await fixture(transport)
    await spool.enqueue(event('event-1'))

    await expect(client.flush()).rejects.toThrow('offline')
    expect(await spool.ids()).toEqual(['event-1'])
  })

  it('单批最多发送一百条', async () => {
    let batchSize = 0
    const transport: BridgeTransport = { send: async envelope => {
      batchSize = envelope.events?.length ?? 0
      return { version: 1, accepted_ids: envelope.events?.map(item => item.event_id) ?? [], rejected: [] }
    } }
    const { spool, client } = await fixture(transport)
    for (let index = 0; index < 105; index += 1) await spool.enqueue(event(`event-${index}`))

    await client.flush()

    expect(batchSize).toBe(100)
    expect((await spool.ids()).length).toBe(5)
  })
})
