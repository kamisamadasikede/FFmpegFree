import type { CatBlock } from '@/api/catMock'
import { CAT_SEARCH_PAGE, searchTurns, visibleSearchHits } from './catConvSearch'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catConvSearchChecks(eq: Eq): void {
  const blocks: CatBlock[] = [
    { kind: 'user', text: '你好' },
    { kind: 'think', text: '在想' },
    { kind: 'a', id: 'a1', text: '你好，我在。', streaming: false },
    { kind: 'user', text: '1+1' },
    { kind: 'run', text: 'Read: a.go' },
    { kind: 'a', id: 'a2', text: '等于 2', streaming: false },
    { kind: 'user', text: 'D:\\pic.png\n图片', files: [{ name: 'pic.png', path: 'D:\\pic.png', kind: 'image', data: 'aa' }] },
    { kind: 'a', id: 'a3', text: '这是一张照片', streaming: false },
  ]
  const all = searchTurns(blocks, '')
  eq('搜索：最新的问排在前面', all.map((t) => t.q), ['pic.png', '1+1', '你好'])
  eq('搜索：图片不把路径和占位写进问', all[0] && [all[0].q, all[0].a], ['pic.png', '这是一张照片'])
  eq('搜索：答里不带思考和工具', all[1]?.a, '等于 2')
  eq('搜索：输入框能按问或答筛选', searchTurns(blocks, '等于').map((t) => t.q), ['1+1'])
  eq('搜索：大小写不限', searchTurns([{ kind: 'user', text: 'Hello' }, { kind: 'a', id: 'x', text: 'OK', streaming: false }], 'hello').length, 1)
  eq('搜索：一次只拿出一页', visibleSearchHits(all, CAT_SEARCH_PAGE).length, all.length)
  const many = Array.from({ length: 30 }, (_, i): CatBlock => ({ kind: 'user', text: `第${i}句` }))
  eq('搜索：下翻之前不把全部画出来', visibleSearchHits(searchTurns(many, ''), CAT_SEARCH_PAGE).length, CAT_SEARCH_PAGE)
}
