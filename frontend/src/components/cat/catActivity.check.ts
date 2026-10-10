// 消息折叠自检：工具调用和文件查找默认收起，正文和文件改动留着，结束后不转圈。
import { diffLineKind, foldIcon, presentCatBlocks } from './catActivity'
import type { CatBlock } from '@/api/catMock'

type Eq = (name: string, got: unknown, want: unknown) => void

function foldAt(blocks: CatBlock[], index = 0) {
  const row = presentCatBlocks(blocks)[index]
  if (!row || row.kind !== 'fold') return undefined
  return row.fold
}

export function catActivityChecks(eq: Eq): void {
  const listed = presentCatBlocks([{ kind: 'run', id: 't', text: '- ListDir: .\n- Search: .\n- Read: D:\\proj\\README.md' }])
  eq(
    '查找各自一行，读取把文件放进下拉',
    listed.map((r) => (r.kind === 'fold' ? `${r.fold.title}|${r.fold.body}|${r.fold.defaultOpen}|${r.fold.streaming}` : '')),
    ['ListDir: .||false|false', 'Search: .||false|false', 'Read|D:\\proj\\README.md|false|false'],
  )

  const one = foldAt([{ kind: 'run', text: 'Read: a.go' }])
  eq('读取的文件在下拉里', one && { title: one.title, body: one.body, open: one.defaultOpen, spin: one.streaming }, {
    title: 'Read',
    body: 'a.go',
    open: false,
    spin: false,
  })

  const search = foldAt([{ kind: 'run', text: '- Search: foo\n  hit a.go\n  hit b.go' }])
  eq('查找结果默认收起，标题只留查询', search && { title: search.title, open: search.defaultOpen, body: search.body }, {
    title: 'Search: foo',
    open: false,
    body: 'hit a.go\nhit b.go',
  })

  const thought = foldAt([{ kind: 'think', id: 'h', text: '正在查看 runtime.go 里的启动顺序，确认窗口是怎么起来的。' }])
  eq('思考正文收在「思考」下', thought && { title: thought.title, open: thought.defaultOpen, body: thought.body, spin: thought.streaming }, {
    title: '思考',
    open: false,
    body: '正在查看 runtime.go 里的启动顺序，确认窗口是怎么起来的。',
    spin: false,
  })

  const status = foldAt([{ kind: 'think', text: '思考完成 · 6 秒' }])
  eq('短的思考状态不用再套一层', status && { title: status.title, body: status.body }, { title: '思考完成 · 6 秒', body: '' })

  const live = foldAt([{ kind: 'run', id: 'live', text: 'grep', streaming: true }])
  eq('搜索不转圈', live?.streaming, false)
  const thinking = foldAt([{ kind: 'think', text: '还在想', streaming: true }])
  eq('只有思考转圈', thinking?.streaming, true)

  const read = foldAt([{ kind: 'run', id: 'rf', text: 'read_file\nsrc/a.go\nsrc/b.go', streaming: true }])
  eq('read_file 不转圈，点开是加载的文件', read && { title: read.title, body: read.body, open: read.defaultOpen, spin: read.streaming }, {
    title: 'read_file · 2',
    body: 'src/a.go\nsrc/b.go',
    open: false,
    spin: false,
  })

  const change = foldAt([{ kind: 'run', text: '正在修改 3 处时间轴…' }])
  const edited = foldAt([{ kind: 'run', text: 'Edited runtime.go' }])
  eq('英文的改文件也展开', edited && { tone: edited.tone, open: edited.defaultOpen }, { tone: 'change', open: true })

  eq('文件改动默认展开', change && { tone: change.tone, open: change.defaultOpen, title: change.title, spin: change.streaming }, {
    tone: 'change',
    open: true,
    title: '正在修改 3 处时间轴…',
    spin: false,
  })

  const diff = '- Read: a.go\n- Edit: b.go\n- old line\n+ new line\n- Search: q'
  const rows = presentCatBlocks([{ kind: 'run', id: 'mix', text: diff }, { kind: 'a', id: 'ans', text: '改好了', streaming: false }])
  eq(
    '改动把查找切开，正文仍在后面',
    rows.map((r) => (r.kind === 'fold' ? `${r.fold.tone}:${r.fold.title}:${r.fold.body}:${r.fold.defaultOpen}` : r.block.kind)),
    ['tool:Read:a.go:false', 'change:Edit: b.go:- old line\n+ new line:true', 'tool:Search: q::false', 'a'],
  )
  const edit = rows[1]
  eq('改动的 diff 留在正文里', edit.kind === 'fold' && edit.fold.body, '- old line\n+ new line')

  const grouped = presentCatBlocks([
    { kind: 'run', id: 'a', text: 'Read: a.go', streaming: true },
    { kind: 'run', id: 'b', text: 'Search: q', streaming: true },
    { kind: 'run', id: 'c', text: 'Edited runtime.go', streaming: true },
  ])
  eq(
    '搜索和编辑各自直接显示，都不转圈',
    grouped.map((r) => (r.kind === 'fold' ? `${r.fold.tone}:${r.fold.title}:${r.fold.streaming}` : '')),
    ['tool:Read:false', 'tool:Search: q:false', 'change:Edited runtime.go:false'],
  )

  eq('查找用搜索图标，改动用编辑图标', [foldIcon({ tone: 'tool', title: '文件查找 · 2' }), foldIcon({ tone: 'change', title: 'Edit: a.go' }), foldIcon({ tone: 'think', title: '思考' })], ['search', 'edit', 'spark'])

  const wrote = foldAt([{ kind: 'run', text: 'Write `a.yml`\na.yml\n\n+name: nginx\n+ports:' }])
  eq('写入展开后留着新增的全文', wrote && { tone: wrote.tone, open: wrote.defaultOpen, body: wrote.body }, {
    tone: 'change',
    open: true,
    body: 'a.yml\n\n+name: nginx\n+ports:',
  })
  eq('对比行：新增绿色，删掉红色，原文和文件名不着色', ['+name: nginx', '-HTTP_PORT=8080', ' 不变', 'a.yml', '--- a.yml', '+++ a.yml'].map(diffLineKind), ['add', 'del', 'same', 'same', 'same', 'same'])
}
