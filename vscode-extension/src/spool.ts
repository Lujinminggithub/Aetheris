import fs from 'node:fs/promises'
import path from 'node:path'
import type { BehaviorEvent } from './protocol'

const DEFAULT_MAX_BYTES = 8 * 1024 * 1024
const DEFAULT_MAX_AGE_MS = 24 * 60 * 60 * 1000
const EVENT_KEYS = new Set([
  'event_id', 'event_type', 'occurred_at', 'session_id', 'workspace_path', 'file_path', 'relative_path',
  'file_name', 'extension', 'language_id', 'uri_scheme', 'read_only', 'change_count', 'inserted_chars',
  'deleted_chars', 'undo_count', 'redo_count', 'flush_reason', 'edit_started_at', 'edit_ended_at',
  'edit_duration_ms', 'had_pending_edit', 'workspace_kind', 'added_workspace_paths', 'removed_workspace_paths',
  'extension_id', 'version', 'previous_version', 'change', 'is_active', 'previous_is_active',
])

export interface CompactOptions {
  now: number
  maxBytes: number
  maxAgeMs: number
}

export class Spool {
  constructor(private readonly file: string) {}

  async enqueue(event: BehaviorEvent): Promise<void> {
    const events = await this.read()
    events.push(sanitize(event))
    await this.write(boundedByBytes(events, DEFAULT_MAX_BYTES))
  }

  async claim(limit = 100): Promise<BehaviorEvent[]> {
    return (await this.read()).slice(0, Math.max(0, Math.min(100, limit)))
  }

  async ack(eventIds: readonly string[]): Promise<void> {
    const accepted = new Set(eventIds)
    await this.write((await this.read()).filter(event => !accepted.has(event.event_id)))
  }

  async compact(options: CompactOptions): Promise<void> {
    await this.write(compacted(await this.read(), options))
  }

  async ids(): Promise<string[]> {
    return (await this.read()).map(event => event.event_id)
  }

  async count(): Promise<number> {
    return (await this.read()).length
  }

  async clear(): Promise<void> {
    await this.write([])
  }

  private async read(): Promise<BehaviorEvent[]> {
    try {
      const raw = await fs.readFile(this.file, 'utf8')
      return raw.split('\n').filter(Boolean).flatMap(line => {
        try {
          const value = JSON.parse(line) as BehaviorEvent
          return value && typeof value.event_id === 'string' && typeof value.event_type === 'string' ? [sanitize(value)] : []
        } catch {
          return []
        }
      })
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') return []
      throw error
    }
  }

  private async write(events: readonly BehaviorEvent[]): Promise<void> {
    await fs.mkdir(path.dirname(this.file), { recursive: true })
    const temporary = `${this.file}.tmp`
    const body = events.map(event => JSON.stringify(sanitize(event))).join('\n') + (events.length ? '\n' : '')
    await fs.writeFile(temporary, body, { encoding: 'utf8', mode: 0o600 })
    await fs.rename(temporary, this.file)
  }
}

function sanitize(event: BehaviorEvent): BehaviorEvent {
  return Object.fromEntries(Object.entries(event).filter(([key]) => EVENT_KEYS.has(key))) as unknown as BehaviorEvent
}

function compacted(events: readonly BehaviorEvent[], options: CompactOptions): BehaviorEvent[] {
  const minimumTime = options.now - options.maxAgeMs
  const retained = events.filter(event => {
    const occurredAt = Date.parse(event.occurred_at)
    return Number.isFinite(occurredAt) && occurredAt >= minimumTime
  })
  return boundedByBytes(retained, options.maxBytes)
}

function boundedByBytes(events: readonly BehaviorEvent[], maxBytes: number): BehaviorEvent[] {
  const retained = [...events]
  while (retained.length && byteLength(retained) > maxBytes) retained.shift()
  return retained
}

function byteLength(events: readonly BehaviorEvent[]): number {
  return Buffer.byteLength(events.map(event => JSON.stringify(event)).join('\n') + (events.length ? '\n' : ''), 'utf8')
}
