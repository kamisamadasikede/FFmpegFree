/**
 * 「全部放开」（risk=danger）每个项目第一次切换时要确认；确认过就记住（本机 localStorage，按项目路径）。
 * 只通过这两个函数读写，以后要挪到后端只改这里。
 */
import { CAT_DANGER_ACK_KEY } from '@/api/catStorageKeys'
import { t } from '@/i18n'

const NO_PROJECT = '__no_project__'
const mem: Record<string, true> = {}

function load(): Record<string, true> {
  try {
    const raw = globalThis.localStorage?.getItem(CAT_DANGER_ACK_KEY)
    const v = raw ? JSON.parse(raw) : {}
    return v && typeof v === 'object' ? v : {}
  } catch {
    return { ...mem }
  }
}

export function dangerAcked(projectPath: string | undefined): boolean {
  const k = projectPath || NO_PROJECT
  return !!load()[k] || !!mem[k]
}

export function ackDanger(projectPath: string | undefined) {
  const k = projectPath || NO_PROJECT
  mem[k] = true
  try {
    globalThis.localStorage?.setItem(CAT_DANGER_ACK_KEY, JSON.stringify({ ...load(), [k]: true }))
  } catch {
    /* 存不了就只在这次运行里记 */
  }
}
