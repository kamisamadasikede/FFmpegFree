import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { ElMessage } from 'element-plus'
import * as liveApi from '@/api/live'
import { AppError, toAppError, type AppErrorCode } from '@/api/call'
import { revealInFolder } from '@/api/system'
import { actionErrorText } from '@/errors/errorMessages'
import { displayPushUrl } from '@/utils/liveUrl'
import { hasWailsBackend } from '@/services/wails'
import { rowsPreview } from '@/views/live/pushPreview'

/** 会话行的状态（对应设计稿 v0.2 §2.2）：run 运行中 · stp 正在停止… · ok 已结束推流 · cnl 已强制停止 · int 推流中断 */
export type LiveRowStatus = 'run' | 'stp' | 'ok' | 'cnl' | 'int'

export interface LiveRow {
  /** = 任务 id */
  id: string
  kind: 'file' | 'screen'
  /** 脱敏后的地址（推流码 / 口令显示 ****），可以直接显示 */
  url: string
  status: LiveRowStatus
  /** 这一路带本地存档（码率栏显示“—”） */
  archive: boolean
  /** 存档路径；终态事件里 outputPath 非空 = 存档保留 */
  outputPath: string
  startedAt: number
  /** 终态时间；0 = 还没结束 */
  endedAt: number
  bitrateKbps: number | null
  /** 屏幕推流的采集来源名（屏幕名 / 窗口标题）：只在本机界面显示，**不脱敏但不写日志、不进错误 detail**；刷新后接回的会话取不到 */
  source?: { title: string; kind: 'screen' | 'window' }
  /** 这一路开始时是否带预览（会话启动参数，运行中不能改）；刷新后接回的会话后端没告诉我们，为 undefined（按“有预览”去取，取不到会转“失败”） */
  preview?: boolean
}

export const MAX_LIVE_SESSIONS = 4

export type BeginResult = { ok: true } | { ok: false; error: AppError }

/**
 * 直播页的全局会话列表（文件 / 录屏两个页签共用，最多 4 路）。
 * - 行在收到该任务的第一条 task:progress（= 已经在推）时出现；连接阶段失败不进列表，错误由表单展示。
 * - 已结束的行在本次打开期间一直保留，用户点 [移除] 才清掉；刷新 / 重启后只接回进行中的会话（TaskService.ListActive）。
 * - “正在停止…”仍占名额，直到后端终态事件（succeeded / canceled）才释放；界面不做 16 秒超时处理。
 */
export const useLiveSessionsStore = defineStore('liveSessions', () => {
  const rows = ref<LiveRow[]>([])
  /** 预览面板当前显示的会话（任务 id）；空 = 自动取第一个进行中的会话 */
  const previewId = ref('')
  const offs = new Map<string, () => void>()
  let recovered = false

  /** n/4 的 n：运行中 + 正在停止 */
  const busyCount = computed(() => rows.value.filter((r) => r.status === 'run' || r.status === 'stp').length)

  const find = (id: string) => rows.value.find((r) => r.id === id)

  function applyProgress(id: string, p: liveApi.LiveProgress) {
    const r = find(id)
    if (!r) return
    // 有存档的会话 task:progress 不带 bitrateKbps，码率栏固定显示“—”
    if (!r.archive && p.bitrateKbps > 0) r.bitrateKbps = Math.round(p.bitrateKbps)
  }

  function applyEnd(id: string, e: liveApi.LiveTaskEnd) {
    offs.get(id)?.()
    offs.delete(id)
    const r = find(id)
    if (!r) return
    r.status = e.status === 'succeeded' ? 'ok' : e.status === 'canceled' ? 'cnl' : 'int'
    r.outputPath = e.outputPath
    r.endedAt = Date.now()
  }

  function attach(id: string) {
    offs.get(id)?.()
    offs.set(id, liveApi.watchLiveTask(id, { onProgress: (p) => applyProgress(id, p), onEnd: (e) => applyEnd(id, e) }))
  }

  /**
   * 新开的推流：订阅任务事件。第一条 progress 到了 → 加一行“运行中”并 resolve ok；
   * 还没连上就 failed → resolve 失败（错误交给表单显示，不进列表）；没连上就 succeeded / canceled / interrupted → 直接加一条终态行。
   */
  function begin(task: liveApi.ApiTask, meta: { kind: 'file' | 'screen'; redactedUrl: string; archive: boolean; source?: { title: string; kind: 'screen' | 'window' }; preview?: boolean }): Promise<BeginResult> {
    return new Promise((resolve) => {
      let connected = false
      const add = (status: LiveRowStatus, endedAt: number, outputPath = '') => {
        if (find(task.id)) return
        rows.value.unshift({
          id: task.id, kind: meta.kind, url: displayPushUrl(meta.redactedUrl), status, archive: meta.archive,
          outputPath, startedAt: Date.now(), endedAt, bitrateKbps: null, ...(meta.source ? { source: meta.source } : {}), ...(meta.preview !== undefined ? { preview: meta.preview } : {}),
        })
      }
      const off = liveApi.watchLiveTask(task.id, {
        onConnected: () => {
          connected = true
          add('run', 0)
          resolve({ ok: true })
        },
        onProgress: (p) => applyProgress(task.id, p),
        onEnd: (e) => {
          offs.delete(task.id)
          if (connected) return applyEnd(task.id, e)
          if (e.status === 'failed') {
            resolve({ ok: false, error: new AppError((e.error?.code ?? 'INTERNAL') as AppErrorCode, e.error?.message ?? '', e.error?.detail) })
            return
          }
          add(e.status === 'succeeded' ? 'ok' : e.status === 'canceled' ? 'cnl' : 'int', Date.now(), e.outputPath)
          resolve({ ok: true })
        },
      })
      offs.set(task.id, off)
    })
  }

  /** 页面刷新 / 重启后接回还在推的会话（ListActive）。只做一次；拿不到完整地址，只有后端脱敏的 params.url */
  async function recover() {
    if (recovered) return
    recovered = true
    if (rowsPreview) return seedPreview(rowsPreview)
    try {
      const running = (await liveApi.listRunning()).sort((a, b) => a.startedAt - b.startedAt)
      for (const r of running) {
        if (find(r.streamId)) continue
        rows.value.unshift({
          id: r.streamId, kind: r.type === 'live_screen_push' ? 'screen' : 'file', url: displayPushUrl(r.url), status: 'run', archive: !!r.outputPath,
          outputPath: r.outputPath, startedAt: r.startedAt || Date.now(), endedAt: 0, bitrateKbps: null,
        })
        attach(r.streamId)
      }
    } catch {
      /* 后端没起就算了 */
    }
  }

  /** [停止]：该行立刻进入“正在停止…”，等后端事件更新（最多 16 秒，界面不处理超时） */
  async function stop(id: string) {
    const r = find(id)
    if (!r || r.status !== 'run') return
    r.status = 'stp'
    if (isSeed(id)) return
    try {
      await liveApi.stopPush(id)
    } catch (e) {
      const err = toAppError(e)
      if (err.code === 'TASK_CONFLICT' || err.code === 'NOT_FOUND') return attach(id) // 已经结束：重新订阅，补取一次终态
      r.status = 'run'
      ElMessage.error(`停止失败：${actionErrorText(err.code, err.message, err.reason)}`)
    }
  }

  /** [强制停止]（只在“正在停止…”时出现） */
  async function forceStop(id: string) {
    const r = find(id)
    if (!r || r.status !== 'stp') return
    if (isSeed(id)) {
      r.status = 'cnl'
      r.endedAt = Date.now()
      r.outputPath = r.archive ? r.outputPath : ''
      return
    }
    try {
      await liveApi.forceStopPush(id)
    } catch (e) {
      const err = toAppError(e)
      if (err.code === 'TASK_CONFLICT' || err.code === 'NOT_FOUND') return attach(id)
      ElMessage.error(`停止失败：${actionErrorText(err.code, err.message, err.reason)}`)
    }
  }

  /** [移除]：只把这一行从页面列表里去掉，不删除存档、不影响任务中心历史 */
  function remove(id: string) {
    const r = find(id)
    if (!r || r.status === 'run' || r.status === 'stp') return
    rows.value = rows.value.filter((x) => x.id !== id)
  }

  /** [打开所在文件夹]（存档路径来自终态 task:status 的 outputPath） */
  async function reveal(id: string) {
    const r = find(id)
    if (!r?.outputPath) return
    if (!hasWailsBackend()) return void ElMessage.info('演示环境不能打开文件夹')
    try {
      await revealInFolder(r.outputPath)
    } catch (e) {
      const err = toAppError(e)
      ElMessage.error(actionErrorText(err.code, err.message, err.reason))
    }
  }

  // ───── 浏览器预览种子（?rows=…，真实运行不读取）─────
  const isSeed = (id: string) => id.startsWith('preview-')
  function seedPreview(name: string) {
    const tab = new URLSearchParams(window.location.search).get('tab') === 'screen' || window.location.hash.includes('/live/record') ? 'screen' : 'file'
    const now = Date.now()
    const U = {
      r1: 'rtmp://push.example.com/live/****', r2: 'rtmps://push.example.net/app/****', r1b: 'rtmp://push.example.com/live2/****',
      s1: 'srt://s.example.com:9000?passphrase=****',
    }
    const ARC = '/Users/me/Movies/FFmpegFree/直播存档/screen-20260929-200000.mp4'
    let n = 0
    const win = new URLSearchParams(window.location.search).get('src') === 'window' // 预览：?src=window 会话行显示窗口来源（含很长的标题）
    const mk = (kind: 'file' | 'screen', u: keyof typeof U, status: LiveRowStatus, sec: number, kb: number | null, arc = false): LiveRow => ({
      id: `preview-${++n}`, kind, url: U[u], status, archive: arc,
      ...(kind === 'screen' ? { source: win && n % 2 === 1 ? { title: '2026 年第三季度经营分析汇报（终稿）.pptx - PowerPoint', kind: 'window' as const } : { title: '屏幕 1（主显示器）', kind: 'screen' as const } } : {}), outputPath: arc && status !== 'run' && status !== 'stp' && status !== 'int' ? ARC : '',
      startedAt: now - sec * 1000, endedAt: status === 'run' || status === 'stp' ? 0 : now, bitrateKbps: arc ? null : kb,
    })
    const t = (h: number, m: number, s: number) => h * 3600 + m * 60 + s
    const f = (u: keyof typeof U, st: LiveRowStatus, sec: number, kb: number | null) => mk('file', u, st, sec, kb)
    const s = (u: keyof typeof U, st: LiveRowStatus, sec: number, kb: number | null, arc = false) => mk('screen', u, st, sec, kb, arc)
    const scr = tab === 'screen'
    const map: Record<string, () => LiveRow[]> = {
      running: () => (scr ? [s('r1', 'run', t(0, 12, 36), null, true)] : [f('r1', 'run', t(0, 12, 36), 4820)]),
      multi: () => (scr
        ? [s('r1', 'run', t(0, 42, 18), null, true), f('s1', 'run', t(0, 25, 8), 3960), f('r2', 'run', t(1, 3, 27), 4820)]
        : [s('r2', 'run', t(0, 4, 52), 5210), f('s1', 'run', t(0, 25, 8), 3960), f('r1', 'run', t(1, 3, 27), 4820)]),
      stopping: () => (scr ? [s('r1', 'stp', t(0, 31, 5), null, true)] : [f('r1', 'stp', t(0, 31, 5), 4790)]),
      ended: () => (scr ? [s('r1', 'ok', t(0, 31, 21), null, true)] : [f('r1', 'ok', t(0, 31, 21), 4802)]),
      canceled: () => (scr ? [s('r1', 'cnl', t(0, 31, 9), 5006)] : [f('r1', 'cnl', t(0, 31, 9), 4795)]),
      cancelarc: () => [s('r1', 'cnl', t(0, 31, 9), null, true)],
      interrupted: () => (scr ? [s('r1', 'int', t(0, 18, 44), 5120)] : [f('r1', 'int', t(0, 18, 44), 4810)]),
      max4: () => [f('r1', 'run', t(1, 3, 27), 4820), f('r2', 'run', t(0, 47, 10), 4790), f('s1', 'run', t(0, 25, 8), 3960), f('r1b', 'run', t(0, 4, 52), 5210)],
      screenlimit: () => [s('r1', 'run', t(0, 12, 36), null, true)],
      all: () => [
        f('r1', 'run', t(1, 3, 27), 4820), f('r2', 'stp', t(0, 31, 5), 4790), f('s1', 'ok', t(0, 31, 21), 3988), f('r1', 'cnl', t(0, 31, 9), 4795),
        s('r1', 'cnl', t(0, 31, 9), null, true), s('r2', 'ok', t(0, 44, 2), null, true), f('s1', 'int', t(0, 18, 44), 4810),
      ],
    }
    // cancelarc 强杀且存档保留：canceled 且 outputPath 非空
    const list = (map[name] ?? (() => []))()
    for (const r of list) if (r.status === 'cnl' && r.archive) r.outputPath = ARC
    // 开发演示：?rowspv=1,0,1 逐行指定“预览：开/关”（缺省都是开）；?pvsel=2 选中第 N 行作为当前预览行
    const pvs = (new URLSearchParams(window.location.search).get('rowspv') ?? '').split(',').filter(Boolean)
    list.forEach((r, i) => (r.preview = pvs[i] !== '0'))
    const sel = Number(new URLSearchParams(window.location.search).get('pvsel'))
    if (sel >= 1 && list[sel - 1]) previewId.value = list[sel - 1].id
    rows.value = list
  }

  const selectPreview = (id: string) => (previewId.value = id)
  return { rows, previewId, selectPreview, busyCount, begin, recover, stop, forceStop, remove, reveal }
})
