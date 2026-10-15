import * as Binding from '../../wailsjs/go/app/CatService'
import { cat } from '../../wailsjs/go/models'
import { call, AppError } from '@/api/call'
import { hasWailsBackend } from '@/services/wails'

export interface SharedItem { name: string; description: string; enabled: boolean }
export interface SharedTarget {
  agentKind: string; skillsPath: string; configPath: string
  skillCount: number; mcpCount: number; state: string; warnings: string[]
}
export interface SharedExtensions {
  skillsRoot: string; mcpConfigPath: string; skills: SharedItem[]
  mcpServers: SharedItem[]; targets: SharedTarget[]; warnings: string[]
}
// Browser previews are explicitly empty and never claim a real native sync.
function preview(): SharedExtensions {
  return { skillsRoot: '', mcpConfigPath: '', skills: [], mcpServers: [], targets: [], warnings: [] }
}
function requireBackend() { if (!hasWailsBackend()) throw new AppError('UNSUPPORTED', 'Desktop only') }
export async function getShared(projectId: string): Promise<SharedExtensions> {
  if (!hasWailsBackend()) return preview()
  return call(Binding.GetCatSharedExtensions(cat.SkillsDirRequest.createFrom({ projectId })))
}
export async function syncShared(projectId: string): Promise<SharedExtensions> {
  requireBackend()
  return call(Binding.SyncCatSharedExtensions(cat.SkillsDirRequest.createFrom({ projectId })))
}
export async function importSharedSkill(projectId: string, sourcePath: string): Promise<SharedExtensions> {
  requireBackend()
  return call(Binding.ImportCatSharedSkill(cat.SharedImportRequest.createFrom({ projectId, sourcePath })))
}
export async function toggleShared(projectId: string, category: string, name: string, enabled: boolean): Promise<SharedExtensions> {
  requireBackend()
  return call(Binding.SetCatSharedItemEnabled(cat.SetInspectItemEnabledRequest.createFrom({ projectId, category, name, enabled })))
}
export async function readSharedMCP(projectId: string): Promise<string> {
  requireBackend()
  return call(Binding.ReadCatSharedMCP(cat.SkillsDirRequest.createFrom({ projectId })))
}
export async function saveSharedMCP(projectId: string, json: string): Promise<SharedExtensions> {
  requireBackend()
  return call(Binding.SaveCatSharedMCP(cat.SharedMCPRequest.createFrom({ projectId, json })))
}
export async function revealShared(projectId: string, kind: 'skills' | 'mcp'): Promise<void> {
  requireBackend()
  return call(Binding.RevealCatSharedDirectory(cat.SharedDirectoryRequest.createFrom({ projectId, kind })))
}
