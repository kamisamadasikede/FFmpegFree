import { callService, AppError } from './call'
import { hasWailsBackend } from '@/services/wails'
import type { CatConversation } from './cat'

export interface CatPrompt { id: string; name: string; content: string }
export interface CatWorkspace { projectId: string; rules: string; prompts: CatPrompt[] }
export interface ContextFile { name: string; relPath: string; path: string }
export interface ContextFiles { files: ContextFile[]; truncated: boolean }
export type ConvAction = 'rename' | 'reset_title' | 'pin' | 'archive'
const desktop = () => { if (!hasWailsBackend()) throw new AppError('UNSUPPORTED', 'Desktop only') }
export async function updateConversation(convId: string, action: ConvAction, title = '', enabled = false): Promise<CatConversation> {
  desktop(); return callService('CatService', 'UpdateCatConversation', { convId, action, title, enabled })
}
export async function exportConversation(convId: string, format: 'md' | 'json'): Promise<string> {
  desktop(); return callService('CatService', 'ExportCatConversation', convId, format)
}
export async function getWorkspace(projectId: string): Promise<CatWorkspace> {
  if (!hasWailsBackend()) return { projectId, rules: '', prompts: [] }
  return callService('CatService', 'GetCatWorkspace', projectId)
}
export async function saveProjectRules(projectId: string, rules: string): Promise<void> {
  desktop(); return callService('CatService', 'SaveCatProjectRules', { projectId, rules })
}
export async function savePrompt(p: CatPrompt): Promise<CatPrompt> {
  desktop(); return callService('CatService', 'SaveCatPrompt', p)
}
export async function deletePrompt(id: string): Promise<void> {
  desktop(); return callService('CatService', 'DeleteCatPrompt', id)
}
export async function searchContextFiles(convId: string, projectId: string, query: string): Promise<ContextFiles> {
  if (!hasWailsBackend()) return { files: [], truncated: false }
  return callService('CatService', 'SearchCatContextFiles', { convId, projectId, query })
}
