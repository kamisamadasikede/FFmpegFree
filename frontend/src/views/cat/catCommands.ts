import { t } from '@/i18n'
import type { CatPrompt } from '@/api/catWorkspace'
import type { CatConv } from '@/api/catMock'

export interface DraftTrigger { kind: 'command' | 'file'; query: string; start: number; end: number }
export interface DraftCommand { id: string; name: string; description: string; content?: string; action?: string }

export function draftTrigger(text: string, caret: number): DraftTrigger | null {
  const end = Math.max(0, Math.min(caret, text.length))
  const before = text.slice(0, end)
  if (/^\/[\p{L}\p{N}_-]*$/u.test(before) && !text.slice(end).trim()) return { kind: 'command', query: before.slice(1), start: 0, end }
  const m = /(?:^|\s)@([^\s@]*)$/.exec(before)
  if (!m) return null
  return { kind: 'file', query: m[1] ?? '', start: end - (m[1]?.length ?? 0) - 1, end }
}
export function draftCommands(prompts: CatPrompt[]): DraftCommand[] {
  const actions = {
    new: t('cat.workspace.command_new'), search: t('cat.workspace.command_search'),
    export: t('cat.workspace.command_export'), rules: t('cat.workspace.command_rules'),
    prompts: t('cat.workspace.command_prompts'), skills: t('cat.workspace.command_skills'),
  }
  const builtins = [
    { id: 'review', description: t('cat.workspace.command_review'), content: t('cat.workspace.prompt_review') },
    { id: 'test', description: t('cat.workspace.command_test'), content: t('cat.workspace.prompt_test') },
    { id: 'explain', description: t('cat.workspace.command_explain'), content: t('cat.workspace.prompt_explain') },
    { id: 'plan', description: t('cat.workspace.command_plan'), content: t('cat.workspace.prompt_plan') },
  ]
  return [
    ...Object.entries(actions).map(([action, description]) => ({ id: action, name: '/'+action, description, action })),
    ...builtins.map(c => ({ ...c, name: '/'+c.id })),
    ...prompts.map((p, i) => ({ id: 'prompt:'+p.id, name: '/p'+(i+1), description: p.name, content: p.content })),
  ]
}
/** Keep the highlighted slash row when the trigger text did not change. */
export function suggestionIndexAfterTrigger(index: number, previous: { kind: string; query: string } | null, next: { kind: string; query: string } | null): number {
  if (!next) return 0
  if (previous && previous.kind === next.kind && previous.query === next.query) return index
  return 0
}
export function filterCommands(commands: DraftCommand[], query: string) {
  const q = query.trim().toLocaleLowerCase()
  return commands.filter(c => !q || c.name.toLocaleLowerCase().includes(q) || c.description.toLocaleLowerCase().includes(q)).slice(0, 30)
}
export function visibleConversations(convs: CatConv[], archived: boolean, query: string): CatConv[] {
  const q = query.trim().toLocaleLowerCase()
  return convs.filter(c => !!c.archived === archived && (!q || c.title.toLocaleLowerCase().includes(q))).slice().sort((a,b) => Number(!!b.pinned)-Number(!!a.pinned))
}
