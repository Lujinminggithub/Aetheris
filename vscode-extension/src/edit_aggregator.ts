import type { BehaviorEvent } from './protocol'
import type { FileMetadata } from './privacy'

export interface TextChangeLike {
  text: string
  rangeLength: number
}

interface Aggregate {
  metadata: FileMetadata
  startedAt: Date
  endedAt: Date
  changeCount: number
  insertedChars: number
  deletedChars: number
  undoCount: number
  redoCount: number
}

export class EditAggregator {
  private readonly aggregates = new Map<string, Aggregate>()

  constructor(private readonly newId: () => string, private readonly sessionId: string) {}

  record(documentId: string, metadata: FileMetadata, changes: readonly TextChangeLike[], at: Date, reason?: 'undo' | 'redo'): BehaviorEvent | null {
    if (!changes.length) return null
    const aggregate = this.aggregates.get(documentId) ?? {
      metadata, startedAt: at, endedAt: at, changeCount: 0,
      insertedChars: 0, deletedChars: 0, undoCount: 0, redoCount: 0,
    }
    aggregate.metadata = metadata
    aggregate.endedAt = at
    for (const change of changes) {
      aggregate.changeCount += 1
      aggregate.insertedChars += change.text.length
      aggregate.deletedChars += Math.max(0, change.rangeLength)
    }
    if (reason === 'undo') aggregate.undoCount += 1
    if (reason === 'redo') aggregate.redoCount += 1
    this.aggregates.set(documentId, aggregate)
    return aggregate.changeCount >= 200 ? this.flush(documentId, 'change_limit', at) : null
  }

  flush(documentId: string, reason: string, at: Date): BehaviorEvent | null {
    const aggregate = this.aggregates.get(documentId)
    if (!aggregate) return null
    this.aggregates.delete(documentId)
    return {
      event_id: this.newId(), event_type: 'ide.file_edited', occurred_at: at.toISOString(), session_id: this.sessionId,
      ...aggregate.metadata,
      change_count: aggregate.changeCount,
      inserted_chars: aggregate.insertedChars,
      deleted_chars: aggregate.deletedChars,
      undo_count: aggregate.undoCount,
      redo_count: aggregate.redoCount,
      flush_reason: reason,
      edit_started_at: aggregate.startedAt.toISOString(),
      edit_ended_at: aggregate.endedAt.toISOString(),
      edit_duration_ms: Math.max(0, at.getTime() - aggregate.startedAt.getTime()),
    }
  }

  flushIdle(at: Date, idleMs = 30_000): BehaviorEvent[] {
    const events: BehaviorEvent[] = []
    for (const [documentId, aggregate] of this.aggregates) {
      if (at.getTime() - aggregate.endedAt.getTime() >= idleMs) {
        const event = this.flush(documentId, 'idle', at)
        if (event) events.push(event)
      }
    }
    return events
  }

  hasPending(documentId: string): boolean {
    return this.aggregates.has(documentId)
  }

  clearAll(): void {
    this.aggregates.clear()
  }
}
