// Cat 文件树摊平自检（api.check.ts 调用）：只过滤已加载的名字，截断行跟着已展开的目录。
import { fileEntryStamp, flattenCatFiles, mapFileList, mapSearchResult, mergeFileNodes, type CatFileEntry, type CatFileNode } from './catFiles'

type Eq = (name: string, got: unknown, want: unknown) => void

function node(partial: Partial<CatFileNode> & Pick<CatFileNode, 'name' | 'relPath' | 'isDir'>): CatFileNode {
  return {
    depth: 0,
    expanded: false,
    loading: false,
    truncated: false,
    children: null,
    ...partial,
  }
}

export function catFilesChecks(eq: Eq): void {
  const tree: CatFileNode[] = [
    node({
      name: 'src',
      relPath: 'src',
      isDir: true,
      expanded: true,
      children: [
        node({ name: 'main.go', relPath: 'src/main.go', isDir: false, depth: 1 }),
        node({ name: 'skip.txt', relPath: 'src/skip.txt', isDir: false, depth: 1 }),
      ],
    }),
    node({ name: 'README', relPath: 'README', isDir: false }),
  ]
  eq('不过滤：已展开的层都在', flattenCatFiles(tree, '').map((r) => r.name), ['src', 'main.go', 'skip.txt', 'README'])
  eq('按文件名过滤并保留上级目录', flattenCatFiles(tree, 'Main').map((r) => r.name), ['src', 'main.go'])
  eq('没有匹配就是空', flattenCatFiles(tree, 'zzz'), [])
  const cut = [node({ name: 'src', relPath: 'src', isDir: true, expanded: true, truncated: true, children: [] })]
  eq('截断提示挂在已展开的目录下', flattenCatFiles(cut, '').map((r) => r.kind), ['entry', 'note'])
  eq('没展开的目录不显示截断行', flattenCatFiles([node({ name: 'src', relPath: 'src', isDir: true, truncated: true })], '').map((r) => r.kind), ['entry'])
  const kept = [node({ name: 'src', relPath: 'src', isDir: true, expanded: true, children: [node({ name: 'a.go', relPath: 'src/a.go', isDir: false, depth: 1 })] })]
  const merged = mergeFileNodes(kept, [
    { name: 'src', relPath: 'src', isDir: true, size: 0, modTime: 1 },
    { name: 'note.txt', relPath: 'note.txt', isDir: false, size: 4, modTime: 2 },
  ] satisfies CatFileEntry[], 0)
  eq('实时刷新保留已展开目录，并补上新文件', [merged[0] === kept[0], merged[0].children?.[0]?.name, merged.map((n) => n.name)], [true, 'a.go', ['src', 'note.txt']])
  const same = { name: 'a', relPath: 'a', isDir: false, size: 1, modTime: 1 }
  eq('文件改动会改变列表签名', fileEntryStamp([same]) === fileEntryStamp([{ ...same, modTime: 2 }]), false)
  eq('列表映射：空和缺字段', mapFileList(null), { root: '', entries: [], truncated: false })
  eq(
    '列表映射：只留有名字的条目',
    mapFileList({ root: 'D:\\p', truncated: true, entries: [{ name: 'a.txt', relPath: 'a.txt', isDir: false, size: 3, modTime: 1 }, { relPath: 'x' }, { name: 'd', relPath: 'd', isDir: true }] }),
    { root: 'D:\\p', entries: [{ name: 'a.txt', relPath: 'a.txt', isDir: false, size: 3, modTime: 1 }, { name: 'd', relPath: 'd', isDir: true, size: 0, modTime: 0 }], truncated: true },
  )
  eq('查找映射：空和有行号的命中', mapSearchResult({ truncated: true, hits: [{ name: 'a.go', relPath: 'src/a.go', line: 2, text: 'func Hello' }, {}] }), { hits: [{ name: 'a.go', relPath: 'src/a.go', isDir: false, line: 2, text: 'func Hello' }], truncated: true })
}
