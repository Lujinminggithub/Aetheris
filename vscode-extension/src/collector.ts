import type { BehaviorEvent } from './protocol'
import { toWorkspaceFileMetadata, type DocumentLike, type WorkspaceFolderLike } from './privacy'
import { EditAggregator, type TextChangeLike } from './edit_aggregator'

export type { DocumentLike, WorkspaceFolderLike } from './privacy'

export interface EditorLike {
  document: DocumentLike
}

export interface CollectorOptions {
  workspaceFolders: () => readonly WorkspaceFolderLike[]
  emit: (event: BehaviorEvent) => void
  now: () => Date
  newId: () => string
  sessionId: string
}

export interface Collector {
  activeEditorChanged(editor: EditorLike | null): BehaviorEvent | null
  workspaceFoldersChanged(added: readonly WorkspaceFolderLike[], removed: readonly WorkspaceFolderLike[]): BehaviorEvent | null
  textDocumentChanged(document: DocumentLike, changes: readonly TextChangeLike[], at?: Date, reason?: 'undo' | 'redo'): BehaviorEvent | null
  textDocumentSaved(document: DocumentLike, at?: Date): BehaviorEvent | null
  textDocumentClosed(document: DocumentLike, at?: Date): BehaviorEvent | null
  flushIdle(at?: Date): BehaviorEvent[]
  clear(): void
}

export function createCollector(options: CollectorOptions): Collector {
  const edits = new EditAggregator(options.newId, options.sessionId)
  let activeDocumentId = ''

  function activeEditorChanged(editor: EditorLike | null): BehaviorEvent | null {
    const at = options.now()
    if (activeDocumentId) emitIfPresent(edits.flush(activeDocumentId, 'editor_switch', at))
    activeDocumentId = ''
    if (!editor) return null
    const metadata = toWorkspaceFileMetadata(editor.document, options.workspaceFolders())
    if (!metadata) return null
    activeDocumentId = documentId(editor.document)
    const event: BehaviorEvent = {
      event_id: options.newId(),
      event_type: 'ide.file_opened',
      occurred_at: at.toISOString(),
      session_id: options.sessionId,
      ...metadata,
    }
    options.emit(event)
    return event
  }

  function workspaceFoldersChanged(added: readonly WorkspaceFolderLike[], removed: readonly WorkspaceFolderLike[]): BehaviorEvent | null {
    if (!added.length && !removed.length) return null
    const event: BehaviorEvent = {
      event_id: options.newId(),
      event_type: 'ide.workspace_changed',
      occurred_at: options.now().toISOString(),
      session_id: options.sessionId,
      workspace_kind: options.workspaceFolders().length > 1 ? 'multi_root' : 'single_root',
      added_workspace_paths: added.map(folder => pathValue(folder)),
      removed_workspace_paths: removed.map(folder => pathValue(folder)),
    }
    options.emit(event)
    return event
  }

  function textDocumentChanged(document: DocumentLike, changes: readonly TextChangeLike[], at = options.now(), reason?: 'undo' | 'redo'): BehaviorEvent | null {
    const metadata = toWorkspaceFileMetadata(document, options.workspaceFolders())
    if (!metadata) return null
    const event = edits.record(documentId(document), metadata, changes, at, reason)
    emitIfPresent(event)
    return event
  }

  function textDocumentSaved(document: DocumentLike, at = options.now()): BehaviorEvent | null {
    const metadata = toWorkspaceFileMetadata(document, options.workspaceFolders())
    if (!metadata) return null
    const edited = edits.flush(documentId(document), 'save', at)
    emitIfPresent(edited)
    const event: BehaviorEvent = {
      event_id: options.newId(), event_type: 'ide.file_saved', occurred_at: at.toISOString(), session_id: options.sessionId,
      ...metadata,
      change_count: edited?.change_count ?? 0,
      inserted_chars: edited?.inserted_chars ?? 0,
      deleted_chars: edited?.deleted_chars ?? 0,
      edit_duration_ms: edited?.edit_duration_ms ?? 0,
    }
    options.emit(event)
    return event
  }

  function textDocumentClosed(document: DocumentLike, at = options.now()): BehaviorEvent | null {
    const metadata = toWorkspaceFileMetadata(document, options.workspaceFolders())
    if (!metadata) return null
    const id = documentId(document)
    const hadPendingEdit = edits.hasPending(id)
    emitIfPresent(edits.flush(id, 'close', at))
    if (activeDocumentId === id) activeDocumentId = ''
    const event: BehaviorEvent = {
      event_id: options.newId(), event_type: 'ide.file_closed', occurred_at: at.toISOString(), session_id: options.sessionId,
      ...metadata, had_pending_edit: hadPendingEdit,
    }
    options.emit(event)
    return event
  }

  function flushIdle(at = options.now()): BehaviorEvent[] {
    const events = edits.flushIdle(at)
    events.forEach(options.emit)
    return events
  }

  function clear(): void {
    edits.clearAll()
    activeDocumentId = ''
  }

  function emitIfPresent(event: BehaviorEvent | null): void {
    if (event) options.emit(event)
  }

  return { activeEditorChanged, workspaceFoldersChanged, textDocumentChanged, textDocumentSaved, textDocumentClosed, flushIdle, clear }
}

function pathValue(folder: WorkspaceFolderLike): string {
  return folder.path
}

function documentId(document: DocumentLike): string {
  return `${document.uri.scheme}:${document.uri.fsPath.toLowerCase()}`
}
