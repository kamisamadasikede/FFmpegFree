// 浏览器截图 / 走查：?lpv= 画出设计说明 §八 的 16 个场景。真实运行（有 window.go）不读。
import { hasWailsBackend, previewParams } from '@/services/wails'

export type LpPhase = 'empty' | 'connecting' | 'playing' | 'buffering' | 'unsupported' | 'ended' | 'interrupted'

export interface LpVisual {
  phase: LpPhase
  kind: 'push' | 'pull'
  muted: boolean
  hint: boolean
  vol: boolean
  volume: number
  idle: boolean
  lag: number | null
  aspect: number
  full: boolean
  panel: boolean
  fake: '' | 'a' | 'b' | 'c'
  clock: string
  /** codec = 编码不支持；unavailable = 预览流做不出来（同一句推流文案） */
  reason: '' | 'codec' | 'unavailable'
  tab: 'push' | 'pull'
}

const base = (p: Partial<LpVisual> & Pick<LpVisual, 'phase' | 'kind' | 'tab'>): LpVisual => ({
  muted: p.kind === 'push', hint: false, vol: false, volume: 70, idle: false, lag: null, aspect: 16 / 9, full: false, panel: false,
  fake: '', clock: p.kind === 'push' ? '00:12:36' : '00:42:18', reason: '', ...p,
})

const MAP: Record<string, LpVisual> = {
  muted: base({ phase: 'playing', kind: 'push', tab: 'push', hint: true, fake: 'a' }),
  sound: base({ phase: 'playing', kind: 'push', tab: 'push', muted: false, vol: true, fake: 'a' }),
  idle: base({ phase: 'playing', kind: 'push', tab: 'push', idle: true, fake: 'a' }),
  play: base({ phase: 'playing', kind: 'pull', tab: 'pull', muted: false, fake: 'b' }),
  lag: base({ phase: 'playing', kind: 'pull', tab: 'pull', muted: false, lag: 4, aspect: 9 / 16, fake: 'c' }),
  connecting: base({ phase: 'connecting', kind: 'pull', tab: 'pull' }),
  buffering: base({ phase: 'buffering', kind: 'pull', tab: 'pull', muted: false, fake: 'b' }),
  unsup: base({ phase: 'unsupported', kind: 'push', tab: 'push', reason: 'codec', clock: '00:12:36' }),
  'pull-unsup': base({ phase: 'unsupported', kind: 'pull', tab: 'pull', reason: 'codec' }),
  ended: base({ phase: 'ended', kind: 'push', tab: 'push', fake: 'a' }),
  'pull-ended': base({ phase: 'ended', kind: 'pull', tab: 'pull', fake: 'b' }),
  broken: base({ phase: 'interrupted', kind: 'push', tab: 'push', fake: 'a' }),
  full: base({ phase: 'playing', kind: 'pull', tab: 'pull', muted: false, full: true, fake: 'b' }),
  'pull-broken': base({ phase: 'interrupted', kind: 'pull', tab: 'pull', muted: false, fake: 'b' }),
  'full-broken': base({ phase: 'interrupted', kind: 'push', tab: 'push', full: true, fake: 'a' }),
  panel: base({ phase: 'playing', kind: 'push', tab: 'push', hint: true, fake: 'a', panel: true }),
  empty: base({ phase: 'empty', kind: 'push', tab: 'push', panel: true, clock: '' }),
}

export const lpVisual: LpVisual | null = hasWailsBackend() ? null : MAP[previewParams.get('lpv') ?? ''] ?? null
