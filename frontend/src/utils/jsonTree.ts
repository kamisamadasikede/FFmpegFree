// 树形视图的数据结构：直接在文本上做一遍解析（输入已经通过校验），数字保留原文，不丢精度。
// 树只保存节点，可见行由 flatten 按展开集合按需生成，配合组件里的虚拟滚动，几十万节点也只渲染屏幕内的行。

export type JKind = 'object' | 'array' | 'string' | 'number' | 'boolean' | 'null'

export interface JNode {
  id: number
  depth: number
  kind: JKind
  /** 对象键的原文（带引号）或数组下标 */
  key?: string
  /** 键是否是数组下标 */
  isIndex?: boolean
  /** 标量值的原文 */
  raw?: string
  children?: JNode[]
}

export function parseTree(text: string): JNode | null {
  let i = 0
  let id = 0
  const n = text.length
  const ws = () => {
    while (i < n) {
      const c = text.charCodeAt(i)
      if (c === 32 || c === 10 || c === 13 || c === 9) i++
      else break
    }
  }
  const str = () => {
    const s = i
    i++
    while (i < n && text[i] !== '"') i += text[i] === '\\' ? 2 : 1
    i++
    return text.slice(s, i)
  }
  const value = (depth: number, key?: string, isIndex?: boolean): JNode => {
    ws()
    const c = text[i]
    const node: JNode = { id: id++, depth, kind: 'null', key, isIndex }
    if (c === '{' || c === '[') {
      const arr = c === '['
      node.kind = arr ? 'array' : 'object'
      node.children = []
      i++
      ws()
      if (text[i] === (arr ? ']' : '}')) {
        i++
        return node
      }
      for (;;) {
        ws()
        if (arr) {
          node.children.push(value(depth + 1, String(node.children.length), true))
        } else {
          const k = str()
          ws()
          i++ // :
          node.children.push(value(depth + 1, k))
        }
        ws()
        if (text[i] === ',') {
          i++
          continue
        }
        i++ // } or ]
        return node
      }
    }
    if (c === '"') {
      node.kind = 'string'
      node.raw = str()
      return node
    }
    const s = i
    while (i < n && !/[\s,\]}]/.test(text[i])) i++
    node.raw = text.slice(s, i)
    node.kind = node.raw === 'null' ? 'null' : node.raw === 'true' || node.raw === 'false' ? 'boolean' : 'number'
    return node
  }
  try {
    return value(0)
  } catch {
    return null // 嵌套过深等极端情况
  }
}

/** 默认展开：根和第一层。 */
export function defaultExpanded(root: JNode): Set<number> {
  const s = new Set<number>()
  if (root.children) {
    s.add(root.id)
    for (const c of root.children) if (c.children && root.children.length <= 200) s.add(c.id)
  }
  return s
}

/** 按展开集合展开成可见行（迭代，不递归）。 */
export function flatten(root: JNode, expanded: Set<number>): JNode[] {
  const rows: JNode[] = []
  const stack: JNode[] = [root]
  while (stack.length) {
    const nd = stack.pop()!
    rows.push(nd)
    if (nd.children && expanded.has(nd.id)) {
      for (let k = nd.children.length - 1; k >= 0; k--) stack.push(nd.children[k])
    }
  }
  return rows
}
