import net from 'node:net'
import { PIPE_NAME, PROTOCOL_VERSION, type BehaviorEvent, type BridgeAck, type BridgeEnvelope } from './protocol'
import { Spool } from './spool'

export interface BridgeTransport {
  send(envelope: BridgeEnvelope): Promise<BridgeAck>
}

export class NamedPipeTransport implements BridgeTransport {
  constructor(private readonly pipeName = PIPE_NAME, private readonly timeoutMs = 5_000) {}

  send(envelope: BridgeEnvelope): Promise<BridgeAck> {
    return new Promise((resolve, reject) => {
      const payload = Buffer.from(JSON.stringify(envelope), 'utf8')
      const header = Buffer.alloc(4)
      header.writeUInt32LE(payload.length, 0)
      const socket = net.createConnection(this.pipeName)
      const chunks: Buffer[] = []
      const timer = setTimeout(() => socket.destroy(new Error('bridge_timeout')), this.timeoutMs)
      socket.on('connect', () => socket.write(Buffer.concat([header, payload])))
      socket.on('data', chunk => {
        chunks.push(chunk)
        const buffer = Buffer.concat(chunks)
        if (buffer.length < 4) return
        const size = buffer.readUInt32LE(0)
        if (size <= 0 || size > 64 * 1024) return socket.destroy(new Error('ack_size_invalid'))
        if (buffer.length < size + 4) return
        clearTimeout(timer)
        try {
          const ack = JSON.parse(buffer.subarray(4, 4 + size).toString('utf8')) as BridgeAck
          socket.end()
          resolve(validateAck(ack))
        } catch (error) {
          socket.destroy()
          reject(error)
        }
      })
      socket.once('error', error => { clearTimeout(timer); reject(error) })
    })
  }
}

export class BridgeClient {
  sentEvents = 0
  droppedEvents = 0

  constructor(private readonly spool: Spool, private readonly transport: BridgeTransport, private readonly sessionId: string) {}

  async enqueue(event: BehaviorEvent): Promise<void> {
    await this.spool.enqueue(event)
  }

  async flush(): Promise<{ accepted: number; rejected: number }> {
    const events = await this.spool.claim(100)
    if (!events.length) return { accepted: 0, rejected: 0 }
    const ack = await this.transport.send({ version: PROTOCOL_VERSION, type: 'events', session_id: this.sessionId, events })
    const batchIds = new Set(events.map(event => event.event_id))
    const acceptedIds = ack.accepted_ids.filter(id => batchIds.has(id))
    await this.spool.ack(acceptedIds)
    this.sentEvents += acceptedIds.length
    return { accepted: acceptedIds.length, rejected: ack.rejected.filter(item => batchIds.has(item.event_id)).length }
  }

  async sendEnvelope(envelope: BridgeEnvelope): Promise<BridgeAck> {
    return this.transport.send(envelope)
  }

  async pendingCount(): Promise<number> {
    return this.spool.count()
  }

  async clear(): Promise<void> {
    await this.spool.clear()
  }
}

function validateAck(value: BridgeAck): BridgeAck {
  if (value?.version !== PROTOCOL_VERSION || !Array.isArray(value.accepted_ids) || !Array.isArray(value.rejected)) {
    throw new Error('ack_invalid')
  }
  return value
}
