/**
 * Cat 页开发走查开关（仅 `vite dev` + 纯浏览器 + 地址带 ?cat_sim=…）。
 *
 * - 正式包：import.meta.env.DEV 为 false，整段被裁掉，永远返回空集合
 * - Wails 里（有 window.go）：无效
 * - 默认（不带参数）：无效
 *
 * 取值（逗号分隔，可组合）：
 *   checking —— 组件状态停在「检查中」（发送置灰）
 *   stream   —— 组件状态 ready；发送后按定稿事件形状（cat:message append/done + cat:turn）推一段走查用文字，可点「停止」
 *   noproj   —— 没有项目（项目栏显示「还没有项目。」）
 *   missing  —— 「字幕项目」的文件夹不见了（项目标灰、输入框禁用）
 *   slowdel  —— 删除对话要 1.5 秒（走查「删除中」小转圈）
 *   delfail  —— 删除对话失败（走查「删除失败，请重试。」浮提示）
 *   slowconv —— 打开对话时模拟读记录 900ms（看消息骨架）
 *   browsertimeout —— 侧栏浏览器打开任意网址时按超时失败（文案同后端）
 *   caps     —— 模型 / 思考强度给一份走查用列表（含「极高」等任意档位；模型名是真实风格 ID，不叫「Cat 助手 …」），走查胶囊「默认」和选档
 *   codexcaps —— 【走查假数据，不是 Codex CLI 真实列表】Codex 会话给一份不同于 Grok 的模式 / 强度列表，走查「按 agentKind 渲染」；契约 v0.44 落地后以后端为准
 *   failonce —— 配合 stream：每条对话第一轮直接失败，走查「重试」原地重发（v0.44 RetryCatTurn）
 *   scratchfiles —— 未绑定项目的对话，对话文件夹当作有文件（右侧文件面板显示）
 *   histmissing / histreadfail —— 同 histempty，historyStatus=missing / read_failed（v0.45.2 两句空状态；histempty 无字段 → 欢迎页）
 *   retrygone —— 配合 stream,failonce：点「重试」时后端报 retry_not_allowed，按钮原地换成灰字（v0.44.5）
 *   histempty —— 打开任意对话都当作记录没找到（零轮），走查空状态
 *   md       —— 配合 stream：推一段 Markdown 回复（标题、列表、表格、代码块、引用、链接、外链图片、原始 HTML）
 *   savefail —— Cat 设置保存失败（该项回滚 +「保存失败，请重试。」）
 *   inspectslow / inspectfail / inspectempty —— 技能与扩展：加载 4 秒 / 失败 / 为空
 *   clearfail —— 清空全部对话失败
 *   keyfail —— Save/ClearCatProviderKey 失败
 *   skillsmissing —— RevealCatSkillsDir 回 skillsRootExists=false（走查「创建并打开」）
 *   setenablefail —— SetCatInspectItemEnabled 失败（开关回滚 +「保存失败，请重试。」）
 *   allagents —— 三家 agents[] 均 ready（走查自定义服务商「已安装」态）
 *   schedempty —— 定时任务为空（空状态 + 模板卡）
 *   schedfail —— 定时任务的保存 / 启停 / 立即运行 / 删除都失败
 *   mdhold   —— 配合 stream,md：推到代码块中间就停住（围栏未闭合、仍在生成），走查流式中的代码块
 *   timingpartial —— 配合 stream：v0.45 消息尾不带首字（只有总耗时），走查「缺哪项不显示哪项」
 *   timinglong —— 配合 stream：首字 12.3s、总耗时 65s（「1分05秒」）
 *   timingstop —— 配合 stream：推到一半自动按「已停止」结束（cancelled，总耗时算到停止那刻）
 *   footertip —— 消息尾悬停提示常显（截图用）
 *   nooutput —— 配合 stream：终态只有 tokenTotal，没有 outputTokens / tokensPerSec（v0.47）
 *   footertime —— 配合 stream：完成态消息尾只留「完成于 HH:mm」（默认带 token 两段）
 *   gitconflict —— 右栏 Git 模拟合并冲突列表（v0.41 走查）
 *   gitrebase —— 同上，变基中文案
 *   gitdone —— 冲突已全部解决，可完成合并
 *   gitbranch —— 分支菜单走查（补全本地/远端列表）
 *   office / officeneed —— 办公画布下载卡（需下载）
 *   officedl —— 办公画布下载中
 *   clioutdated —— 对话栏顶部多三条 Claude / Codex / Grok Build / 无原始报错「CLI 版本过旧」对话（o1–o4）
 *   officefail —— 办公画布未配置失败文案
 *   officepreview / officeready —— 办公画布 ready + ≥3 页预览壳
 *   pmcustom —— v0.46：三家自定义服务商都开着、地址和 Key 已填（模型下拉走 ListCatProviderModels 模拟）
 *   codexconv —— 对话栏顶部多一条 Codex 对话（x1「Codex 自定义服务商」）
 *   codexnoresponses —— 配合 stream：发送后按 CAT_REPLY_FAILED + reason=provider_responses_unsupported（retryable=false）失败
 *   pm_slow / pm_empty / pm_unauth / pm_unreach / pm_unsupported / pm_bad / pm_notcfg —— 配合 pmcustom：
 *            拉列表一直加载中 / ok 但空 / unauthorized / unreachable / not_supported / bad_response / not_configured（默认 = ok 有列表）
 *
 * 另：?cat_os=windows|darwin|linux —— 走查时「在…中显示」按指定平台取文案（默认按浏览器 userAgent 猜）。
 * 另：?cat_pick=<路径>[|<路径>…] —— 走查时「选择项目文件夹」依次返回这些路径（空 = 取消）；用完后返回递增的示例路径。
 */
import type { CatStreamEvent, CatTurnEvent } from '@/api/catStream'
import type { CatModel, CatThinkLevel } from '@/api/cat'

/** 走查：?cat_sim=caps 时的模型列表（第一项 = 默认模型） */
export function simModels(): CatModel[] {
  return [
    { id: 'model-4.7', displayName: 'model-4.7' },
    { id: 'model-4.7-fast', displayName: 'model-4.7-fast' },
  ]
}

/** 走查：?cat_sim=caps 时的强度列表（按后端返回顺序原样显示，不假设只有低/中/高） */
export function simThinks(): CatThinkLevel[] {
  return [
    { id: 'low', displayName: '低' },
    { id: 'medium', displayName: '中' },
    { id: 'high', displayName: '高' },
    { id: 'xhigh', displayName: '极高' },
  ]
}

export type CatSimFlag = 'checking' | 'stream' | 'noproj' | 'missing' | 'slowdel' | 'delfail' | 'caps' | 'codexcaps' | 'failonce' | 'histempty' | 'md' | 'mdhold' | 'savefail' | 'inspectslow' | 'inspectfail' | 'inspectempty' | 'clearfail' | 'keyfail' | 'skillsmissing' | 'setenablefail' | 'schedempty' | 'schedfail' | 'slowconv' | 'browsertimeout' | 'footertime' | 'allagents' | 'gitconflict' | 'gitrebase' | 'gitdone' | 'gitbranch' | 'office' | 'officeneed' | 'officedl' | 'officefail' | 'officepreview' | 'officeready' | 'clioutdated' | 'retrygone' | 'footertip' | 'timingpartial' | 'timinglong' | 'timingstop' | 'histmissing' | 'histreadfail' | 'scratchfiles' | 'pmcustom' | 'pm_slow' | 'pm_empty' | 'pm_unauth' | 'pm_unreach' | 'pm_unsupported' | 'pm_bad' | 'pm_notcfg' | 'codexnoresponses' | 'codexconv' | 'detect_checking' | 'detect_rechecking' | 'detect_partial' | 'detect_legacy' | 'detect_fail' | 'detect_ok' | 'detect_stale' | 'nooutput'
const FLAGS: readonly string[] = ['checking', 'stream', 'noproj', 'missing', 'slowdel', 'delfail', 'caps', 'codexcaps', 'failonce', 'histempty', 'md', 'mdhold', 'savefail', 'inspectslow', 'inspectfail', 'inspectempty', 'clearfail', 'keyfail', 'skillsmissing', 'setenablefail', 'schedempty', 'schedfail', 'slowconv', 'browsertimeout', 'footertime', 'allagents', 'gitconflict', 'gitrebase', 'gitdone', 'gitbranch', 'office', 'officeneed', 'officedl', 'officefail', 'officepreview', 'officeready', 'clioutdated', 'retrygone', 'footertip', 'timingpartial', 'timinglong', 'timingstop', 'histmissing', 'histreadfail', 'scratchfiles', 'pmcustom', 'pm_slow', 'pm_empty', 'pm_unauth', 'pm_unreach', 'pm_unsupported', 'pm_bad', 'pm_notcfg', 'codexnoresponses', 'codexconv', 'detect_checking', 'detect_rechecking', 'detect_partial', 'detect_legacy', 'detect_fail', 'detect_ok', 'detect_stale', 'nooutput']

export const catSim: ReadonlySet<CatSimFlag> = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return new Set<CatSimFlag>()
  if ((window as unknown as { go?: unknown }).go) return new Set<CatSimFlag>()
  const v = new URLSearchParams(window.location.search).get('cat_sim') ?? ''
  return new Set(v.split(',').map((x) => x.trim()).filter((x): x is CatSimFlag => FLAGS.includes(x)))
})()

/** 走查：?cat_pick=… 指定的选文件夹结果（仅 vite dev + 纯浏览器） */
export const catSimPicks: string[] = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return []
  if ((window as unknown as { go?: unknown }).go) return []
  const v = new URLSearchParams(window.location.search).get('cat_pick')
  return v === null ? [] : v.split('|')
})()

/** 走查：?cat_os=… 指定平台（仅 vite dev + 纯浏览器） */
export const catSimOs: 'windows' | 'darwin' | 'linux' | '' = (() => {
  if (!import.meta.env.DEV || typeof window === 'undefined') return ''
  if ((window as unknown as { go?: unknown }).go) return ''
  const v = new URLSearchParams(window.location.search).get('cat_os')
  return v === 'windows' || v === 'darwin' || v === 'linux' ? v : ''
})()

const SIM_TEXT =
  '好的，我先看一下第 4 集的字幕时间轴。整体节奏是对的，但第 12 到 18 条字幕整体比画面晚了大约 0.4 秒，' +
  '建议统一往前挪；第 31 条和第 32 条有重叠，可以把第 31 条的结束时间改到第 32 条开始之前。其余部分没有发现明显问题。'

/** 走查 ?cat_sim=stream,md：Markdown 回复（mdhold 时停在 MD_HOLD 处，代码块未闭合） */
const SIM_MD =
  '## 字幕时间轴检查结果\n\n' +
  '整体节奏是对的，有 **两处** 需要调整，详见下表：\n\n' +
  '| 字幕 | 问题 | 建议 |\n| --- | --- | --- |\n| 第 12–18 条 | 比画面晚约 0.4 秒 | 统一提前 `400ms` |\n| 第 31、32 条 | 时间重叠 | 31 条结束改到 32 条开始前 |\n\n' +
  '处理步骤：\n\n1. 备份原字幕文件\n2. 批量平移时间轴\n   - 只选第 12–18 条\n   - 偏移量填 *-0.4s*\n3. 手动修正第 31 条\n\n' +
  '> 提示：平移后建议再整体播放一遍，确认口型对齐。\n\n' +
  '参考 [字幕格式说明](https://example.com/srt) 或发邮件到 [support@example.com](mailto:support@example.com)；' +
  '示意图 ![时间轴截图](https://example.com/timeline.png) 不会自动加载，<b>原始标签</b> 按文字显示。\n\n---\n\n' +
  '可以用下面的脚本批量平移：\n\n```python\nimport pysrt\n\nsubs = pysrt.open("ep04.srt")\n' +
  'for s in subs[11:18]:\n    s.shift(milliseconds=-400)\nsubs.save("ep04.fixed.srt", encoding="utf-8")\n```\n\n改完后告诉我，我再帮你复查。'
const MD_HOLD = 'for s in subs[11:18]:\n    s.shi'

/**
 * 走查用：按定稿事件形状逐段推送（每 120ms 一段 6 字）。isStopped 为真后不再推（与真实后端「迟到文字丢弃」一致）。
 * 只在 catSim.has('stream') 时由 catState.sendMessage 调用。
 */
export function simStream(
  convId: string,
  onMessage: (e: CatStreamEvent) => void,
  onTurn: (e: CatTurnEvent) => void,
  isStopped: () => boolean,
) {
  const turnId = `sim-turn-${Date.now().toString(36)}`
  const messageId = `sim-msg-${Date.now().toString(36)}`
  onTurn({ convId, turnId, status: 'running' })
  let i = 0
  let seq = 0
  const text = catSim.has('md') ? SIM_MD : SIM_TEXT
  const holdAt = catSim.has('md') && catSim.has('mdhold') ? SIM_MD.indexOf(MD_HOLD) + MD_HOLD.length : -1
  const step = catSim.has('md') ? 14 : 6
  const tick = () => {
    if (isStopped()) return
    if (holdAt >= 0 && i >= holdAt) return // 停在未闭合代码块中间，保持「生成中」
    if (i >= text.length) {
      onMessage({ convId, turnId, messageId, seq: ++seq, op: 'done', role: 'assistant', text: '' })
      onTurn({
        convId,
        turnId,
        status: 'completed',
        messageId,
        contextUsed: 180432,
        contextWindow: 256000,
        finishedAt: Date.now(),
        ...simTiming(),
        ...(catSim.has('footertime')
          ? {}
          : catSim.has('nooutput')
            ? { tokenTotal: 12345 } // v0.47：后端拿不到输出 token → 输出与 token/s 都不给
            : { tokenTotal: 12345, outputTokens: 2100, tokensPerSec: 280 }),
      })
      return
    }
    if (catSim.has('timingstop') && i >= 60) {
      onMessage({ convId, turnId, messageId, seq: ++seq, op: 'done', role: 'assistant', text: '' })
      onTurn({ convId, turnId, status: 'cancelled', messageId, finishedAt: Date.now(), firstTokenMs: 900, totalMs: 3600, tokenTotal: 420, outputTokens: 310, tokensPerSec: 115 })
      return
    }
    const part = text.slice(i, holdAt >= 0 ? Math.min(i + step, holdAt) : i + step)
    i += part.length
    onMessage({ convId, turnId, messageId, seq: ++seq, op: 'append', role: 'assistant', text: part })
    setTimeout(tick, 120)
  }
  setTimeout(tick, 400)
}

/**
 * 【MOCK · 走查假数据】?cat_sim=caps（或 codexcaps）：三家 v0.44 原始形状（agents[].modes + ListCatModels），
 * 经 mapAgentModes / mapModelEfforts 映射；字段名按契约 #255。模式名 / 短名 / 说明按产品设计 10-14 定稿稿（短名仍是草案，数据驱动）。
 * 模型档位抄自 /workspace/notes/cli-modes-effort.md（Codex：codex debug models --bundled；Grok：内嵌模型目录）。
 * 走查用，不是运行时真实数据。
 */
type SimMode = { id: string; label: string; shortLabel: string; risk: 'safe' | 'edits' | 'danger'; description: string; writesFiles: boolean; isDefault: boolean }
const md = (id: string, label: string, shortLabel: string, risk: SimMode['risk'], description = '', isDefault = false): SimMode => ({ id, label, shortLabel, risk, description, writesFiles: risk !== 'safe', isDefault })
const BYPASS_DESC = '不做任何检查，仅建议在隔离环境使用'

export function simAgentRaw(kind: string): { modes: SimMode[]; models: { id: string; displayName: string; defaultEffort: string; efforts: string[] }[] } {
  if (kind === 'claude_cli') {
    return {
      modes: [
        md('manual', '只读', '只读', 'safe', '只看文件，不改动', true),
        md('plan', '计划', '计划', 'safe', '先分析并给出方案，不改文件'),
        md('acceptEdits', '自动改文件', '改文件', 'edits', '直接修改文件，其他命令不执行'),
        md('auto', '自动', '自动', 'edits', '自动执行，经过安全检查'),
        md('bypassPermissions', '全部放开', '全部放开', 'danger', BYPASS_DESC),
      ],
      // 【MOCK】Claude 四个别名，显示名为示意（后端将经 ListCatModels 下发）；按设计说明 §5 示例：sonnet 没有很高；haiku efforts=[] → 强度行隐藏
      models: [
        { id: 'opus', displayName: 'Opus 5.5', defaultEffort: 'high', efforts: ['low', 'medium', 'high', 'xhigh', 'max'] },
        { id: 'sonnet', displayName: 'Sonnet 5.5', defaultEffort: 'high', efforts: ['low', 'medium', 'high', 'max'] },
        { id: 'haiku', displayName: 'Haiku 5.5', defaultEffort: '', efforts: [] },
        { id: 'fable', displayName: 'Fable 5.1', defaultEffort: 'high', efforts: ['low', 'medium', 'high', 'xhigh', 'max'] },
      ],
    }
  }
  if (kind === 'codex_cli') {
    return {
      modes: [
        md('read-only', '只读', '只读', 'safe', '只读取文件，不修改，命令在只读沙箱中运行', true),
        md('workspace-write', '改项目文件', '改文件', 'edits', '可修改项目内文件，命令在沙箱中运行'),
        md('approve-for-me', '自动审查', '自动审查', 'edits', '可修改项目文件，需要批准的操作交给自动审查'),
        md('bypass', '全部放开', '全部放开', 'danger', BYPASS_DESC),
      ],
      models: [
        { id: 'gpt-6.1-sol', displayName: 'gpt-6.1-sol', defaultEffort: 'low', efforts: ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'] },
        { id: 'gpt-6-luna', displayName: 'gpt-6-luna', defaultEffort: 'medium', efforts: ['low', 'medium', 'high', 'xhigh', 'max'] },
        { id: 'gpt-5.5', displayName: 'gpt-5.5', defaultEffort: 'medium', efforts: ['low', 'medium', 'high', 'xhigh'] },
        { id: 'custom-model', displayName: 'custom-model', defaultEffort: '', efforts: [] },
      ],
    }
  }
  return {
    modes: [
      md('default', '只读', '只读', 'safe', '只用只读工具和只读命令，不修改文件', true),
      md('acceptEdits', '自动改文件', '改文件', 'edits', '直接修改文件，不再询问'),
      md('auto', '自动', '自动', 'edits', '安全检查放行的操作直接执行，其他的会被拦下'),
      md('always-approve', '全部放开', '全部放开', 'danger', BYPASS_DESC),
    ],
    models: [
      { id: 'grok-4.6', displayName: 'grok-4.6', defaultEffort: 'high', efforts: ['low', 'medium', 'high', 'xhigh'] },
      { id: 'grok-4.5', displayName: 'grok-4.5', defaultEffort: 'high', efforts: ['low', 'medium', 'high'] },
    ],
  }
}


/** 走查：v0.45 首字 / 总耗时（毫秒） */
function simTiming(): { firstTokenMs?: number; totalMs?: number } {
  if (catSim.has('timinglong')) return { firstTokenMs: 12300, totalMs: 65000 }
  if (catSim.has('timingpartial')) return { totalMs: 8400 }
  return { firstTokenMs: 1200, totalMs: 8400 }
}

/**
 * v0.47.1 走查：CLI 检测状态（?cat_sim=detect_…）。返回依次「到达」的 cat:status 原始载荷（ms 为相对延迟）。
 * detect_checking：只有初始检测中（从未检测过）；detect_rechecking：有上次结果后全量重检中（保留已安装/版本）；
 * detect_partial：只到 Claude 一家，其它保持检测中；
 * detect_legacy：先全量，再来一条没有 agents[] 的旧载荷（不得冲掉其它家）；
 * detect_fail：三家检测完都没找到；detect_ok：三家正常；detect_stale：落库的上次结果（stale）。
 */
export function simDetectPayloads(): { ms: number; raw: unknown }[] {
  const now = Date.now()
  const today = new Date(now); today.setHours(14, 32, 0, 0)
  const at = Math.min(today.getTime(), now)
  const yest = at - 86400000
  const row = (agentKind: string, o: Record<string, unknown>) => ({ agentKind, ready: false, version: '', checking: false, ...o })
  const ok = (k: string, v: string, p: string) => row(k, { ready: true, version: v, checkedAt: at, exePath: p })
  const P = {
    b: 'C:\\Users\\张三 Lee\\AppData\\Local\\Programs\\grok\\grok.exe',
    c: 'C:\\Users\\张三 Lee\\AppData\\Roaming\\npm\\claude.cmd',
    x: 'C:\\Users\\张三 Lee\\AppData\\Local\\Programs\\OpenAI\\Codex\\bin\\codex.exe',
  }
  const checkingAll = { state: 'checking', version: '', agents: ['cat_build', 'claude_cli', 'codex_cli'].map((k) => row(k, { checking: true })) }
  const full = { state: 'ready', version: '1.0', agents: [ok('cat_build', '1.0.3', P.b), ok('claude_cli', '2.1.0', P.c), ok('codex_cli', '0.46.0', P.x)] }
  if (catSim.has('detect_checking')) return [{ ms: 0, raw: checkingAll }]
  if (catSim.has('detect_rechecking')) {
    return [{
      ms: 0,
      raw: {
        state: 'checking',
        version: '1.0',
        agents: [
          { ...ok('cat_build', '1.0.3', P.b), checking: true },
          { ...ok('claude_cli', '2.1.0', P.c), checking: true },
          { ...ok('codex_cli', '0.46.0', P.x), checking: true },
        ],
      },
    }]
  }
  if (catSim.has('detect_partial')) return [{ ms: 0, raw: checkingAll }, { ms: 400, raw: { state: 'checking', version: '', agents: [ok('claude_cli', '2.1.0', P.c)] } }]
  if (catSim.has('detect_legacy')) return [{ ms: 0, raw: full }, { ms: 400, raw: { state: 'ready', version: '1.0.3' } }]
  if (catSim.has('detect_fail'))
    return [{ ms: 0, raw: checkingAll }, { ms: 400, raw: { state: 'missing', version: '', agents: ['cat_build', 'claude_cli', 'codex_cli'].map((k) => row(k, { checkedAt: at })) } }]
  if (catSim.has('detect_stale'))
    return [{ ms: 0, raw: { state: 'ready', version: '1.0', agents: [{ ...ok('cat_build', '1.0.3', P.b), stale: true, checkedAt: yest }, { ...ok('claude_cli', '2.1.0', P.c), stale: true, checkedAt: yest }, { ...ok('codex_cli', '0.46.0', P.x), stale: true }] } }]
  if (catSim.has('detect_ok')) return [{ ms: 0, raw: full }]
  return []
}
