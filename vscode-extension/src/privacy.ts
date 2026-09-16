import path from 'node:path'

export interface UriLike {
  scheme: string
  fsPath: string
}

export interface DocumentLike {
  uri: UriLike
  languageId: string
  isDirty: boolean
}

export interface WorkspaceFolderLike {
  path: string
  name: string
}

export interface FileMetadata {
  workspace_path: string
  file_path: string
  relative_path: string
  file_name: string
  extension: string
  language_id: string
  uri_scheme: 'file'
}

export function toWorkspaceFileMetadata(document: DocumentLike, roots: readonly WorkspaceFolderLike[]): FileMetadata | null {
  if (document.uri.scheme !== 'file' || !document.uri.fsPath) return null
  const filePath = path.win32.resolve(document.uri.fsPath)
  for (const folder of roots) {
    const root = path.win32.resolve(folder.path)
    const relative = path.win32.relative(root, filePath)
    if (!relative || relative.startsWith('..' + path.win32.sep) || relative === '..' || path.win32.isAbsolute(relative)) continue
    return {
      workspace_path: root,
      file_path: filePath,
      relative_path: relative.split(path.win32.sep).join('/'),
      file_name: path.win32.basename(filePath),
      extension: path.win32.extname(filePath).toLowerCase(),
      language_id: document.languageId.slice(0, 128),
      uri_scheme: 'file',
    }
  }
  return null
}
