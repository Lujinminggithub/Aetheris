import crypto from 'node:crypto'
import path from 'node:path'
import * as vscode from 'vscode'
import { BridgeClient, NamedPipeTransport } from './bridge'
import { createCollector } from './collector'
import { createComponentHeartbeat, diffExtensions, snapshotExtensions, type ExtensionSnapshot } from './extension_inventory'
import type { BehaviorEvent } from './protocol'
import { PROTOCOL_VERSION } from './protocol'
import { Spool } from './spool'

let clearCollector: (() => void) | undefined
let stopRuntime: (() => Promise<void>) | undefined

export function activate(context: vscode.ExtensionContext): void {
  const sessionId = crypto.randomUUID()
  const spool = new Spool(path.join(context.globalStorageUri.fsPath, 'events.jsonl'))
  const bridge = new BridgeClient(spool, new NamedPipeTransport(), sessionId)
  let paused = context.globalState.get<boolean>('pausedByUser', false)
  const remote = Boolean(vscode.env.remoteName)
  let operation = Promise.resolve()
  const queueEvent = (event: BehaviorEvent) => {
    if (paused || remote) return
    operation = operation.then(() => bridge.enqueue(event)).then(() => bridge.flush()).then(() => undefined).catch(() => undefined)
  }
  const folders = () => (vscode.workspace.workspaceFolders ?? []).map(folder => ({ path: folder.uri.fsPath, name: folder.name }))
  const collector = createCollector({
    workspaceFolders: folders,
    emit: queueEvent,
    now: () => new Date(),
    newId: () => `vscode-${crypto.randomUUID()}`,
    sessionId,
  })
  clearCollector = () => collector.clear()
  let extensionSnapshot = context.globalState.get<ExtensionSnapshot[]>('extensionSnapshot', [])
  const updateExtensionSnapshot = async () => {
    const current = snapshotExtensions(vscode.extensions.all)
    diffExtensions(extensionSnapshot, current, () => `vscode-${crypto.randomUUID()}`, () => new Date()).forEach(queueEvent)
    extensionSnapshot = current
    await context.globalState.update('extensionSnapshot', current)
  }
  const idleTimer = setInterval(() => collector.flushIdle(), 5_000)
  const sendHeartbeat = async () => {
    const envelope = createComponentHeartbeat({
      sessionId, extensionVersion: String(context.extension.packageJSON.version), vscodeVersion: vscode.version,
      hostKind: remote ? 'remote' : 'local', pendingEvents: await bridge.pendingCount(),
      sentEvents: bridge.sentEvents, droppedEvents: bridge.droppedEvents,
      componentState: remote ? 'unsupported_remote_host' : paused ? 'paused_by_user' : 'active',
    })
    const ack = await bridge.sendEnvelope(envelope)
    if (ack.clear_cache) {
      collector.clear()
      await bridge.clear()
    }
    if (ack.control_state === 'paused_by_user' && !paused) {
      paused = true
      collector.clear()
      await bridge.clear()
      await context.globalState.update('pausedByUser', true)
    } else if (ack.control_state === 'active' && paused) {
      paused = false
      await context.globalState.update('pausedByUser', false)
    }
  }
  const heartbeatTimer = setInterval(() => { void sendHeartbeat().catch(() => undefined) }, 30_000)
  const flushTimer = setInterval(() => { if (!paused && !remote) void bridge.flush().catch(() => undefined) }, 5_000)
  context.subscriptions.push(
    vscode.window.onDidChangeActiveTextEditor(editor => collector.activeEditorChanged(editor ? { document: editor.document } : null)),
    vscode.workspace.onDidChangeWorkspaceFolders(event => collector.workspaceFoldersChanged(
      event.added.map(folder => ({ path: folder.uri.fsPath, name: folder.name })),
      event.removed.map(folder => ({ path: folder.uri.fsPath, name: folder.name })),
    )),
    vscode.workspace.onDidChangeTextDocument(event => collector.textDocumentChanged(
      event.document,
      event.contentChanges,
      new Date(),
      event.reason === vscode.TextDocumentChangeReason.Undo ? 'undo' : event.reason === vscode.TextDocumentChangeReason.Redo ? 'redo' : undefined,
    )),
    vscode.workspace.onDidSaveTextDocument(document => collector.textDocumentSaved(document)),
    vscode.workspace.onDidCloseTextDocument(document => collector.textDocumentClosed(document)),
    vscode.extensions.onDidChange(() => { void updateExtensionSnapshot() }),
    { dispose: () => clearInterval(idleTimer) },
    { dispose: () => clearInterval(heartbeatTimer) },
    { dispose: () => clearInterval(flushTimer) },
    vscode.commands.registerCommand('aetheris.pauseCollection', async () => {
      paused = true
      collector.clear()
      await bridge.clear()
      await context.globalState.update('pausedByUser', true)
      await sendHeartbeat().catch(() => undefined)
    }),
    vscode.commands.registerCommand('aetheris.resumeCollection', async () => {
      paused = false
      await context.globalState.update('pausedByUser', false)
      await sendHeartbeat().catch(() => undefined)
    }),
    vscode.commands.registerCommand('aetheris.clearPendingEvents', () => bridge.clear()),
  )
  void updateExtensionSnapshot()
  void bridge.sendEnvelope({
    version: PROTOCOL_VERSION, type: 'handshake', session_id: sessionId,
    extension_id: 'aetheris.aetheris-vscode', extension_version: String(context.extension.packageJSON.version),
    vscode_version: vscode.version, host_kind: remote ? 'remote' : 'local',
  }).then(() => sendHeartbeat()).catch(() => undefined)
  if (!paused && !remote && vscode.window.activeTextEditor) collector.activeEditorChanged({ document: vscode.window.activeTextEditor.document })
  stopRuntime = async () => {
    clearInterval(idleTimer)
    clearInterval(heartbeatTimer)
    clearInterval(flushTimer)
    collector.clear()
    await operation
  }
}

export async function deactivate(): Promise<void> {
  clearCollector?.()
  clearCollector = undefined
  await stopRuntime?.()
  stopRuntime = undefined
}
