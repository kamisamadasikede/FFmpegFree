import { liveTurnPlan, transcriptChanged } from './catLive'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catLiveChecks(eq: Eq): void {
  const plan = liveTurnPlan(
    [{ convId: 'local', adopted: false }, { convId: 'back', adopted: true }],
    [{ convId: 'back', turnId: 't1' }, { convId: 'orphan', turnId: '' }, { convId: 'local', turnId: 't2' }],
  )
  eq('接回：新的接上，还在的刷新，本页自己的一轮不动', [plan.adopt.map((t) => t.convId), plan.refresh, plan.release], [['orphan'], ['back'], []])
  const gone = liveTurnPlan([{ convId: 'back', adopted: true }], [])
  eq('接回：进程没了就放开', gone.release, ['back'])
  eq('接回：正文没变不替换', transcriptChanged([{ kind: 'a', text: '你好' }], [{ kind: 'a', text: '你好' }]), false)
  eq('接回：多了一句就要换上', transcriptChanged([{ kind: 'a', text: '你好' }], [{ kind: 'a', text: '你好' }, { kind: 'run', text: '跑完了' }]), true)
}
