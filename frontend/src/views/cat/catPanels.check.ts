// Cat 设置 / 定时任务（契约 v0.35.1~v0.35.4）：纯函数自检
import { fixThinkName } from '@/api/cat'
import { normalizeSettings, settingsPayload, normalizeCapabilities, DEFAULT_PROVIDER_CAPABILITIES } from '@/api/catSettings'
import { needsApproveConfirm, normalizeRun, notifyTitle, queueLine, runView, validateSchedule } from '@/api/catSchedules'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catPanelsChecks(eq: Eq) {
  eq('设置：notifyOnFinish 没给视为开', normalizeSettings({}).notifyOnFinish, true)
  eq('设置：notifyOnFinish=false 保留', normalizeSettings({ notifyOnFinish: false }).notifyOnFinish, false)
  eq('设置：非法批准方式清空', normalizeSettings({ defaultApprovalMode: 'x' }).defaultApprovalMode, '')
  eq('设置：旧 defaultModel/Effort 迁进 cat_build', normalizeSettings({ defaultModel: 'm1', defaultEffort: 'high' }).defaultsByAgent.cat_build, { model: 'm1', effort: 'high' })
  eq('设置：已有 cat_build 不覆盖', normalizeSettings({ defaultModel: 'old', defaultsByAgent: { cat_build: { model: 'new', effort: 'low' } } }).defaultsByAgent.cat_build, { model: 'new', effort: 'low' })
  eq('设置：非法 effort 清空', normalizeSettings({ defaultsByAgent: { cat_build: { model: 'm', effort: 'max' } } }).defaultsByAgent.cat_build.effort, '')
  eq('设置：defaultAgentKind 缺省 cat_build', normalizeSettings({}).defaultAgentKind, 'cat_build')
  eq('设置：非法 defaultAgentKind 回退', normalizeSettings({ defaultAgentKind: 'grok_build' }).defaultAgentKind, 'cat_build')
  eq('设置：providersByAgent 缺省全关', normalizeSettings({}).providersByAgent.claude_cli, { enabledCustom: false, protocol: '', apiUrl: '', apiKeySet: false })
  eq('设置：providersByAgent 读入', normalizeSettings({ providersByAgent: { claude_cli: { enabledCustom: true, protocol: 'anthropic', apiUrl: 'https://x', apiKeySet: true, apiKey: 'sk-leak' } } }).providersByAgent.claude_cli, { enabledCustom: true, protocol: 'anthropic', apiUrl: 'https://x', apiKeySet: true })
  eq('设置：非法 protocol 清空', normalizeSettings({ providersByAgent: { cat_build: { enabledCustom: true, protocol: 'foo', apiUrl: '', apiKeySet: false } } }).providersByAgent.cat_build.protocol, '')
  const payload = settingsPayload(normalizeSettings({ providersByAgent: { claude_cli: { enabledCustom: true, protocol: 'anthropic', apiUrl: 'https://x', apiKeySet: true } } }))
  eq('Save：payload 不含 apiKeySet', (payload.providersByAgent as Record<string, Record<string, unknown>>).claude_cli.apiKeySet, undefined)
  eq('Save：payload 不含 apiKey', (payload.providersByAgent as Record<string, Record<string, unknown>>).claude_cli.apiKey, undefined)
  eq('能力：空列表回落默认', normalizeCapabilities([]).map((c) => c.agentKind), DEFAULT_PROVIDER_CAPABILITIES.map((c) => c.agentKind))
  eq('能力：顺序 cat_build→claude→codex', normalizeCapabilities([{ agentKind: 'codex_cli', defaultProtocol: 'openai', allowProtocolPick: false, allowedProtocols: ['openai'] }, { agentKind: 'claude_cli', defaultProtocol: 'anthropic', allowProtocolPick: false, allowedProtocols: ['anthropic'] }, { agentKind: 'cat_build', defaultProtocol: 'openai', allowProtocolPick: false, allowedProtocols: ['openai'] }]).map((c) => c.agentKind), ['cat_build', 'claude_cli', 'codex_cli'])
  eq('强度：xhigh 显示「极高」值不变', fixThinkName({ id: 'xhigh', displayName: '超高' }), { id: 'xhigh', displayName: '极高' })
  eq('强度：其它档位不动', fixThinkName({ id: 'low', displayName: '低' }).displayName, '低')

  const base = { name: 'a', prompt: 'b', projectId: 'p1', frequency: 'daily' as const, time: '09:00', weekdays: [], runAt: 0, model: '', effort: '', approvalMode: 'plan' }
  eq('任务：名称 40 字可以', validateSchedule({ ...base, name: '字'.repeat(40) }).name, undefined)
  eq('任务：名称 41 字报错', validateSchedule({ ...base, name: '字'.repeat(41) }).name, '最多 40 个字。')
  eq('任务：空名称 / 空指令', [validateSchedule({ ...base, name: ' ' }).name, validateSchedule({ ...base, prompt: '' }).prompt], ['请填写名称。', '请填写指令。'])
  eq('任务：每周至少一天', validateSchedule({ ...base, frequency: 'weekly' }).weekdays, '请至少选一天。')
  eq('任务：一次的时间已过', validateSchedule({ ...base, frequency: 'once', runAt: 1 }).runAt, '这个时间已经过了。')
  eq('任务：改成全部批准才确认', [needsApproveConfirm('approve', null), needsApproveConfirm('approve', 'plan'), needsApproveConfirm('approve', 'approve'), needsApproveConfirm('plan', null)], [true, true, false, false])

  const run = (status: string, extra: Record<string, unknown> = {}) => normalizeRun({ id: 'r', scheduleId: 's', status, scheduledFor: 1, ...extra })!
  eq('排队：0 → 下一个就轮到它', queueLine(run('queued', { queuePosition: 0 })), '下一个就轮到它')
  eq('排队：2 → 前面还有 2 个任务', queueLine(run('queued', { queuePosition: 2 })), '前面还有 2 个任务')
  eq('排队：没有字段不显示', queueLine(run('queued')), '')
  eq('错过：closed / asleep', [runView(run('missed', { missedReason: 'closed' })).sub, runView(run('missed', { missedReason: 'asleep' })).sub], ['当时 FFmpegFree 没有打开', '当时电脑在睡眠'])
  eq('停止：列表和运行记录都写「已手动停止」、没有副行', [runView(run('stopped')).label, runView(run('stopped')).sub], ['已手动停止', ''])
  eq('跳过：已跳过 + 上一次还没结束', [runView(run('skipped')).label, runView(run('skipped')).sub], ['已跳过', '上一次还没结束'])
  eq('失败：超时原因原样显示', runView(run('failed', { error: { code: 'CAT_REPLY_FAILED', message: '超过 30 分钟没跑完。' } })).sub, '超过 30 分钟没跑完。')
  eq('通知：只有已完成 / 运行失败', ['succeeded', 'failed', 'stopped', 'missed', 'skipped', 'queued'].map((s) => notifyTitle(run(s), '每日检查')), ['定时任务已完成：每日检查', '定时任务运行失败：每日检查', '', '', '', ''])
}
