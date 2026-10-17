import { draftTrigger, visibleConversations, draftCommands, filterCommands, suggestionIndexAfterTrigger } from './catCommands'
import type { CatConv } from '@/api/catMock'
export function catCommandsChecks(eq: (name: string, got: unknown, want: unknown) => void) {
  eq('slash begins at input start', draftTrigger('/re', 3)?.query, 're')
  eq('saved prompts searchable in Chinese', draftTrigger('/数据库', 4)?.query, '数据库')
  eq('paths are not commands', draftTrigger('src/file.ts', 11), null)
  eq('multiline slash is text', draftTrigger('hello\n/re', 9), null)
  eq('unselected command suffix stays text', draftTrigger('/re draft', 3), null)
  eq('email is not file reference', draftTrigger('a@b.com', 7), null)
  eq('file mention after unicode words', draftTrigger('看一下 @src/main', 13), { kind: 'file', query: 'src/main', start: 4, end: 13 })
  eq('mention before following text', draftTrigger('@src code', 4), { kind: 'file', query: 'src', start: 0, end: 4 })
  eq('mention at beginning', draftTrigger('@', 1)?.kind, 'file')
  eq('whitespace ends mention', draftTrigger('@src ', 5), null)
  eq('slash highlight stays when the same command text is refreshed', suggestionIndexAfterTrigger(2, { kind: 'command', query: '' }, { kind: 'command', query: '' }), 2)
  eq('slash highlight returns to the first row when the query changes', suggestionIndexAfterTrigger(2, { kind: 'command', query: '' }, { kind: 'command', query: 're' }), 0)
  const convs: CatConv[] = [{id:'a',title:'Active',agentKind:'codex_cli'}, {id:'b',title:'Pinned',agentKind:'codex_cli',pinned:true}, {id:'c',title:'Archived',agentKind:'codex_cli',archived:true}]
  eq('active pinned first', visibleConversations(convs, false, '').map(c=>c.id), ['b','a'])
  eq('archives preserved', visibleConversations(convs, true, '').map(c=>c.id), ['c'])
  eq('filter is insensitive', visibleConversations(convs, false, 'ACTIVE').map(c=>c.id), ['a'])
  eq('filter does not mutate source', convs.map(c=>c.id), ['a','b','c'])
  const commands = draftCommands([{id:'custom',name:'Inspect database',content:'Read schema first.'}])
  eq('saved prompt discoverable by title', filterCommands(commands,'database').map(c=>c.content), ['Read schema first.'])
  eq('built-in review only drafts prompt', commands.find(c=>c.id==='review')?.action, undefined)
  eq('review requires evidence', !!commands.find(c=>c.id==='review')?.content, true)
  eq('unknown native commands never advertised', filterCommands(commands,'compact').length, 0)
  eq('no destructive rewind advertised', filterCommands(commands,'rewind').length, 0)
}
