/**
 * Cat 页的本地状态（单例，离开再进来保持选择）。CAT_BACKEND_READY=false：数据全部来自 api/catMock.ts，
 * 发消息只追加一条本地示意回复，不调任何接口。
 */
import { computed, reactive } from 'vue'
import {
  CAT_ACCESS, CAT_MODELS, CAT_THINKS, fakeReply, mockMessages, mockPlainConvs, mockProjects, titleFrom,
  type CatBlock, type CatConv, type CatProject,
} from '@/api/catMock'

export const NEW_CONV = 'new'

export const catState = reactive({
  model: 'v10',
  think: 'high',
  access: 'full' as 'ask' | 'full',
  mode: 'build',
  workInProject: true,
  sel: 'c31',
  projects: mockProjects() as CatProject[],
  plain: mockPlainConvs() as CatConv[],
  /** 已打开过的对话记录（含本地追加的消息） */
  messages: {} as Record<string, CatBlock[]>,
  /** 正在等示意回复的对话 id */
  pending: '' as string,
})

export const modelName = computed(() => CAT_MODELS.find((m) => m.id === catState.model)?.name ?? CAT_MODELS[0].name)
export const thinkName = computed(() => CAT_THINKS.find((t) => t.id === catState.think)?.name ?? '高')
export const accessShort = computed(() => CAT_ACCESS.find((a) => a.id === catState.access)?.short ?? '')

export function findConv(id: string): { project?: CatProject; conv: CatConv } | null {
  for (const p of catState.projects) {
    const c = p.convs.find((x) => x.id === id)
    if (c) return { project: p, conv: c }
  }
  const d = catState.plain.find((x) => x.id === id)
  return d ? { conv: d } : null
}

export function messagesOf(id: string): CatBlock[] {
  if (!catState.messages[id]) {
    const f = findConv(id)
    catState.messages[id] = f ? mockMessages(id, f.conv.title) : []
  }
  return catState.messages[id]
}

let seq = 0
let replyTimer: ReturnType<typeof setTimeout> | undefined

/** 发送：新对话时先在「对话」里建一条；本地追加用户消息，约 0.8 秒后追加示意回复 */
export function sendMessage(text: string) {
  const t = text.trim()
  if (!t || catState.pending) return
  let id = catState.sel
  if (id === NEW_CONV) {
    id = `local-${Date.now().toString(36)}-${++seq}`
    catState.plain.unshift({ id, title: titleFrom(t) })
    catState.messages[id] = []
    catState.sel = id
  }
  const list = messagesOf(id)
  // 原型里「正在修改…」是进行中的示意，发新消息时先去掉
  for (let i = list.length - 1; i >= 0; i--) if (list[i].kind === 'run') list.splice(i, 1)
  list.push({ kind: 'user', text: t })
  catState.pending = id
  const name = modelName.value
  clearTimeout(replyTimer)
  replyTimer = setTimeout(() => {
    messagesOf(id).push(...fakeReply(t, name))
    catState.pending = ''
  }, 800)
}
