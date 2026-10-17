import { demoProcessBlocks, memberIdFromGroupConv, groupIdFromGroupConv } from './catGroupProcess'

function eq(name: string, a: unknown, b: unknown) {
  const ok = JSON.stringify(a) === JSON.stringify(b)
  if (!ok) {
    console.error('FAIL', name, '\n got ', a, '\n want', b)
    process.exitCode = 1
  } else console.log('ok', name)
}

const raw = "先看一眼。\n\n[search] systemKind === 'failed'\n[read] GroupStream.vue\n+ line\n\n类型检查通过。"
const blocks = demoProcessBlocks(raw)
eq('demo: 正文 + 两个工具 + 收尾', blocks.map((b) => ({ kind: b.kind, text: b.text })), [
  { kind: 'text', text: '先看一眼。' },
  { kind: 'tool', text: "search systemKind === 'failed'" },
  { kind: 'tool', text: 'read GroupStream.vue\n+ line' },
  { kind: 'text', text: '类型检查通过。' },
])
eq('convId member', memberIdFromGroupConv('group:g1:m9'), 'm9')
eq('convId group', groupIdFromGroupConv('group:g1:m9'), 'g1')
eq('convId ignore', memberIdFromGroupConv('c-normal'), null)
console.log(process.exitCode ? 'FAILED' : 'ALL OK')
