import { describe, expect, it } from 'vitest'
import { createCollector, type EditorLike, type WorkspaceFolderLike } from '../src/collector'

const root: WorkspaceFolderLike = { path: 'D:\\repo', name: 'repo' }

function editor(path: string, scheme = 'file'): EditorLike {
  return { document: { uri: { scheme, fsPath: path }, languageId: 'typescript', isDirty: false } }
}

describe('VS Code 行为采集器', () => {
  it('只为活动编辑器中的授权本机文件生成打开事件', () => {
    const emitted: unknown[] = []
    const collector = createCollector({
      workspaceFolders: () => [root], emit: event => emitted.push(event),
      now: () => new Date('2026-09-14T08:00:00Z'), newId: () => 'event-1', sessionId: 'session-1',
    })

    const event = collector.activeEditorChanged(editor('D:\\repo\\src\\app.ts'))

    expect(event).toMatchObject({
      event_id: 'event-1', event_type: 'ide.file_opened', workspace_path: 'D:\\repo',
      file_path: 'D:\\repo\\src\\app.ts', relative_path: 'src/app.ts', file_name: 'app.ts',
      extension: '.ts', language_id: 'typescript', uri_scheme: 'file',
    })
    expect(JSON.stringify(event)).not.toContain('source_text')
    expect(emitted).toHaveLength(1)
  })

  it('忽略非文件 URI 和授权根目录之外的文件', () => {
    const collector = createCollector({
      workspaceFolders: () => [root], emit: () => undefined,
      now: () => new Date('2026-09-14T08:00:00Z'), newId: () => 'event-1', sessionId: 'session-1',
    })

    expect(collector.activeEditorChanged(editor('output', 'output'))).toBeNull()
    expect(collector.activeEditorChanged(editor('D:\\private\\secret.ts'))).toBeNull()
  })

  it('记录工作区增加和移除但不读取文件', () => {
    const emitted: unknown[] = []
    const collector = createCollector({
      workspaceFolders: () => [root], emit: event => emitted.push(event),
      now: () => new Date('2026-09-14T08:00:00Z'), newId: () => 'event-2', sessionId: 'session-1',
    })

    const event = collector.workspaceFoldersChanged([{ path: 'D:\\repo2', name: 'repo2' }], [root])

    expect(event).toMatchObject({
      event_type: 'ide.workspace_changed',
      added_workspace_paths: ['D:\\repo2'], removed_workspace_paths: ['D:\\repo'],
    })
    expect(JSON.stringify(emitted)).not.toContain('file_content')
  })
})
