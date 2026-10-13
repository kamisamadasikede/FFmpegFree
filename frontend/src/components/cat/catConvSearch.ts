/**
 * 当前对话里按一问一答搜索。最新的一轮排在前面，一次只取出一页。
 */
import type { CatBlock } from '@/api/catMock'
import { userDisplayText } from './catMedia'

/** 弹窗里一次放进列表的条数。继续往下滚再取下一页。 */
export const CAT_SEARCH_PAGE = 12

export interface CatSearchHit {
  /** 对应用户消息在原列表里的位置，用来滚回去 */
  index: number
  q: string
  a: string
}

function questionOf(block: Extract<CatBlock, { kind: 'user' }>): string {
  const text = userDisplayText(block.text, block.files).trim()
  if (text) return text
  return (block.files ?? []).map((f) => f.name).filter(Boolean).join('、')
}

/** 按发言顺序配好问答。问后面的助手正文合成答，思考和工具调用不写进答。 */
export function conversationTurns(blocks: readonly CatBlock[]): CatSearchHit[] {
  const turns: CatSearchHit[] = []
  let cur: CatSearchHit | null = null
  const answers: string[] = []
  const close = () => {
    if (!cur) return
    cur.a = answers.join('\n\n').trim()
    if (cur.q || cur.a) turns.push(cur)
    cur = null
    answers.length = 0
  }
  blocks.forEach((b, i) => {
    if (b.kind === 'user') {
      close()
      cur = { index: i, q: questionOf(b), a: '' }
      return
    }
    if (!cur) return
    if (b.kind === 'a' && b.text.trim()) answers.push(b.text.trim())
  })
  close()
  return turns
}

/** 空查询返回全部。匹配问或答，不区分大小写。结果倒序，最新的在前。 */
export function searchTurns(blocks: readonly CatBlock[], query: string): CatSearchHit[] {
  const q = query.trim().toLowerCase()
  const turns = conversationTurns(blocks)
  const matched = q
    ? turns.filter((t) => t.q.toLowerCase().includes(q) || t.a.toLowerCase().includes(q))
    : turns
  return matched.reverse()
}

/** 只取前 shown 条，避免一次画出整段对话。 */
export function visibleSearchHits(hits: readonly CatSearchHit[], shown: number): CatSearchHit[] {
  if (shown <= 0) return []
  return hits.slice(0, shown)
}
