import { describe, expect, it } from 'vitest'
import { EditAggregator } from '../src/edit_aggregator'
import { createCollector, type EditorLike, type WorkspaceFolderLike } from '../src/collector'
import type { BehaviorEvent } from '../src/protocol'

const metadata = {
  workspace_path: 'D:\\repo', file_path: 'D:\\repo\\src\\app.ts', relative_path: 'src/app.ts',
  file_name: 'app.ts', extension: '.ts', language_id: 'typescript', uri_scheme: 'file' as const,
}

describe('编辑元数据聚合', () => {
  it('只累计插入删除长度且不保存输入文本', () => {
    const aggregator = new EditAggregator(() => 'event-1', 'session-1')
    aggregator.record('doc-1', metadata, [{ text: 'password=secret', rangeLength: 3 }], new Date('2026-09-14T08:00:00Z'))

    const event = aggregator.flush('doc-1', 'save', new Date('2026-09-14T08:00:10Z'))

    expect(event).toMatchObject({
      event_type: 'ide.file_edited', change_count: 1, inserted_chars: 15, deleted_chars: 3,
      flush_reason: 'save', edit_duration_ms: 10_000,
    })
    expect(JSON.stringify(aggregator)).not.toContain('password=secret')
    expect(JSON.stringify(event)).not.toContain('password=secret')
  })

  it('达到二百次变更时自动刷新', () => {
    const aggregator = new EditAggregator(() => 'event-200', 'session-1')
    let flushed = null
    for (let index = 0; index < 200; index += 1) {
      flushed = aggregator.record('doc-1', metadata, [{ text: 'x', rangeLength: 0 }], new Date('2026-09-14T08:00:00Z')) ?? flushed
    }
    expect(flushed).toMatchObject({ event_type: 'ide.file_edited', change_count: 200, flush_reason: 'change_limit' })
    expect(aggregator.hasPending('doc-1')).toBe(false)
  })

  it('保存时先刷新编辑聚合再生成保存事件', () => {
    const emitted: BehaviorEvent[] = []
    const root: WorkspaceFolderLike = { path: 'D:\\repo', name: 'repo' }
    const editor: EditorLike = { document: { uri: { scheme: 'file', fsPath: 'D:\\repo\\src\\app.ts' }, languageId: 'typescript', isDirty: true } }
    let id = 0
    const collector = createCollector({
      workspaceFolders: () => [root], emit: event => emitted.push(event),
      now: () => new Date('2026-09-14T08:00:00Z'), newId: () => `event-${++id}`, sessionId: 'session-1',
    })
    collector.activeEditorChanged(editor)
    collector.textDocumentChanged(editor.document, [{ text: 'secret-value', rangeLength: 2 }], new Date('2026-09-14T08:00:01Z'))

    collector.textDocumentSaved(editor.document, new Date('2026-09-14T08:00:05Z'))

    expect(emitted.map(event => event.event_type)).toEqual(['ide.file_opened', 'ide.file_edited', 'ide.file_saved'])
    expect(emitted[2]).toMatchObject({ change_count: 1, inserted_chars: 12, deleted_chars: 2 })
    expect(JSON.stringify(emitted)).not.toContain('secret-value')
  })

  it('切换编辑器和关闭文件会刷新未提交编辑', () => {
    const emitted: BehaviorEvent[] = []
    const root: WorkspaceFolderLike = { path: 'D:\\repo', name: 'repo' }
    const first: EditorLike = { document: { uri: { scheme: 'file', fsPath: 'D:\\repo\\one.ts' }, languageId: 'typescript', isDirty: true } }
    const second: EditorLike = { document: { uri: { scheme: 'file', fsPath: 'D:\\repo\\two.ts' }, languageId: 'typescript', isDirty: false } }
    let id = 0
    const collector = createCollector({ workspaceFolders: () => [root], emit: event => emitted.push(event), now: () => new Date('2026-09-14T08:00:00Z'), newId: () => `event-${++id}`, sessionId: 'session-1' })
    collector.activeEditorChanged(first)
    collector.textDocumentChanged(first.document, [{ text: 'x', rangeLength: 0 }], new Date('2026-09-14T08:00:01Z'))

    collector.activeEditorChanged(second)
    collector.textDocumentClosed(second.document, new Date('2026-09-14T08:00:03Z'))

    expect(emitted.map(event => event.event_type)).toEqual(['ide.file_opened', 'ide.file_edited', 'ide.file_opened', 'ide.file_closed'])
  })
})
