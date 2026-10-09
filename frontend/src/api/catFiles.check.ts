// Cat 文件树摊平自检（api.check.ts 调用）：只过滤已加载的名字，截断行跟着已展开的目录。
import { flattenCatFiles, mapFileList, type CatFileNode } from './catFiles'

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
  eq('列表映射：空和缺字段', mapFileList(null), { root: '', entries: [], truncated: false })
  eq(
    '列表映射：只留有名字的条目',
    mapFileList({ root: 'D:\\p', truncated: true, entries: [{ name: 'a.txt', relPath: 'a.txt', isDir: false, size: 3, modTime: 1 }, { relPath: 'x' }, { name: 'd', relPath: 'd', isDir: true }] }),
    { root: 'D:\\p', entries: [{ name: 'a.txt', relPath: 'a.txt', isDir: false, size: 3, modTime: 1 }, { name: 'd', relPath: 'd', isDir: true, size: 0, modTime: 0 }], truncated: true },
  )
}
