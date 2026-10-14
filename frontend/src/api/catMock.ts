/**
 * Cat 聊天页的布局用模拟数据（纯浏览器走查：会话列表 / 欢迎页示意；Wails 里走真实 CatService）。
 * 模型与思考强度不在这里造假——由 api/cat.ts 的 List* 返回（未就绪时为空）。
 * 不调用任何 Wails 绑定。
 */

import type { CatAgentKind } from '@/api/cat'
import { CAT_AGENT_BUILD, CAT_AGENT_CLAUDE, CAT_AGENT_CODEX } from '@/api/cat'
import { CAT_CLI_OUTDATED_COPY, parseCliOutdated } from '@/components/cat/catCliOutdated'
import { t } from '@/i18n'

export type CatConvState = 'ok' | 'idle' | 'run' | 'bad'

/** 一段消息内容。p / li 里的 `反引号` 渲染成行内代码；code 的 parts 第二项为 true 时按关键字上色 */
export type CatBlock =
  | { kind: 'user'; text: string; failed?: boolean; files?: import('@/components/cat/catMedia').CatShownFile[] }
  /** 超时重试。streaming 时转圈，回复继续后收起 */
  | { kind: 'status'; text: string; id?: string; streaming?: boolean }
  | { kind: 'think'; text: string; id?: string; streaming?: boolean }
  | { kind: 'p'; text: string }
  | { kind: 'ul'; items: string[] }
  | { kind: 'code'; lang: string; parts: Array<[string, boolean?]>; more?: string }
  | { kind: 'run'; text: string; id?: string; streaming?: boolean }
  /** 助手回复（真实会话）：streaming=true 时文字仍在增长。reveal：这一轮新出现的，要逐字打出来 */
  | {
      kind: 'a'
      id: string
      text: string
      streaming: boolean
      reveal?: boolean
      /** v0.37.1 消息尾：有 finishedAt 才显示；进行中勿带 */
      finishedAt?: number
      tokenTotal?: number
      /** v0.47 */
      outputTokens?: number
      tokensPerSec?: number
      /** v0.45 首字 / 总耗时（毫秒） */
      firstTokenMs?: number
      totalMs?: number
    }
  /** 浅灰系统行：已停止生成 / 未就绪 / 回复失败 */
  | { kind: 'sys'; text: string; tone?: 'err'; outdated?: import('@/components/cat/catCliOutdated').CatCliOutdated; /** v0.46 服务商类失败（如 provider_responses_unsupported）：同 cli_outdated 的错误行样式 */ providerErr?: true }

export interface CatConv {
  id: string
  title: string
  /** 创建时锁定，之后不可改（契约 6.19.2） */
  agentKind: CatAgentKind
  /** 所属项目（创建时决定，之后不可改；空 = 「对话」下） */
  projectId?: string
  /** 项目对话：第二行摘要、时间、状态点、是否「主要」 */
  sub?: string
  time?: string
  st?: CatConvState
  main?: boolean
}

/** 侧栏里的项目（契约 6.19.10.1 的 CatProject + 界面状态） */
export interface CatProject {
  id: string
  name: string
  /** 用户选的文件夹（tooltip 显示完整路径以区分同名项目） */
  path: string
  /** 文件夹不见了：项目标灰、对话可看不可发、不能在其下新建对话 */
  missing: boolean
  createdAt: number
  open: boolean
  branch: string
  convs: CatConv[]
}

/** 文件树节点：[缩进层级, 类型 d=展开的文件夹 / c=收起的文件夹 / f=文件, 名字, 是否选中] */
export type CatTreeNode = [number, 'd' | 'c' | 'f', string, boolean?]

/** 变更文件：名字、增加行数、删除行数 */
export type CatChange = { path: string; add: number; del: number }

export interface CatAccess { id: 'normal' | 'plan' | 'auto' | 'approve'; name: string; short: string; desc: string; icon: 'hand' | 'list' | 'shield' | 'alert'; enabled: boolean }
export interface CatMode {
  id: string
  name: string
  /** 通用 FIcon；三家主推用 CatAgentMark，此时 icon 可忽略 */
  icon: 'tool' | 'spark' | 'clock' | 'refresh' | 'bot' | 'term'
  /** 创建会话时写入的 agentKind；v0.36 三家可点 */
  agentKind: CatAgentKind
  enabled: boolean
  /** 有值时欢迎页画几何标，不画 FIcon */
  mark?: 'cat_build' | 'claude_cli' | 'codex_cli'
}

export const CAT_ACCESS: CatAccess[] = [
  { id: 'normal', name: t('cat.misc.s_7173ff3411'), short: t('cat.misc.s_35242cff12'), desc: t('cat.misc.s_a65ff5be32'), icon: 'hand', enabled: true },
  { id: 'plan', name: 'Plan', short: 'Plan', desc: t('cat.misc.s_09b7fcb097'), icon: 'list', enabled: true },
  { id: 'auto', name: t('cat.misc.s_321aca51be'), short: t('cat.misc.s_321aca51be'), desc: t('cat.misc.s_3871da4cc0'), icon: 'shield', enabled: true },
  { id: 'approve', name: t('cat.misc.s_35a0b2e6c7'), short: t('cat.misc.s_35a0b2e6c7'), desc: t('cat.misc.s_ecefab6e97'), icon: 'alert', enabled: true },
]

/** 欢迎页：v0.36 解禁 Grok Build / Claude / Codex；Cat CLI / Cat Code 仍灰（契约 6.19.6） */
export const CAT_MODES: CatMode[] = [
  { id: 'cat_build', name: 'Grok Build', icon: 'spark', agentKind: CAT_AGENT_BUILD, enabled: true, mark: 'cat_build' },
  { id: 'claude_cli', name: 'Claude', icon: 'bot', agentKind: CAT_AGENT_CLAUDE, enabled: true, mark: 'claude_cli' },
  { id: 'codex_cli', name: 'Codex', icon: 'term', agentKind: CAT_AGENT_CODEX, enabled: true, mark: 'codex_cli' },
  { id: 'cat_cli', name: 'Cat CLI', icon: 'tool', agentKind: 'cat_cli', enabled: false },
  { id: 'cat_code', name: 'Cat Code', icon: 'spark', agentKind: 'cat_code', enabled: false },
]

export const CAT_TRY = [
  t('cat.misc.s_a83ff639cb'),
  t('cat.misc.s_c4e57b98e5'),
  t('cat.misc.s_50b4bcf29e'),
]

export function mockProjects(): CatProject[] {
  return [
    {
      id: 'p1', name: t('cat.misc.s_6f82dc15ba'), path: 'D:\\素材\\视频素材整理', missing: false, createdAt: 3, open: false, branch: 'main',
      convs: [
        { id: 'c11', title: t('cat.misc.s_172202c7b2'), agentKind: CAT_AGENT_BUILD, main: true, sub: t('cat.misc.s_124c9e0ef8'), time: '2d', st: 'idle' },
        { id: 'c12', title: t('cat.misc.s_bc07b2ace2'), agentKind: CAT_AGENT_BUILD, sub: t('cat.misc.s_04d7ae12b2'), time: '3d', st: 'idle' },
      ],
    },
    {
      id: 'p2', name: t('cat.misc.s_b055793df4'), path: 'D:\\工作\\直播推流测试', missing: false, createdAt: 2, open: true, branch: 'live',
      convs: [
        { id: 'c21', title: t('cat.misc.s_fc141724b1'), agentKind: CAT_AGENT_BUILD, main: true, sub: t('cat.misc.s_4a5f88a092'), time: '58m', st: 'ok' },
        { id: 'c22', title: t('cat.misc.s_c946a66672'), agentKind: CAT_AGENT_BUILD, sub: t('cat.misc.s_ce6ffe19c6'), time: '3h', st: 'idle' },
      ],
    },
    {
      id: 'p3', name: '字幕项目', path: 'D:\\工作\\字幕项目', missing: false, createdAt: 1, open: true, branch: 'test',
      convs: [{ id: 'c31', title: t('cat.misc.s_b972e5622a'), agentKind: CAT_AGENT_BUILD, main: true, sub: t('cat.misc.s_0babf8b30f'), time: '12m', st: 'run' }],
    },
  ]
}

export function mockPlainConvs(): CatConv[] {
  return [
    { id: 'd1', title: t('cat.misc.s_11def59d87'), agentKind: CAT_AGENT_BUILD },
    { id: 'd2', title: t('cat.misc.s_6c1bf7b7ad'), agentKind: CAT_AGENT_BUILD },
    { id: 'd3', title: t('cat.misc.s_62602a1d9e'), agentKind: CAT_AGENT_BUILD },
  ]
}

/** 走查消息尾用的完成时刻（本地 22:15） */
const FOOTER_AT = new Date(2026, 9, 13, 22, 15, 0).getTime()

const SUB3: CatBlock[] = [
  { kind: 'user', text: t('cat.misc.s_0ced2a2855') },
  { kind: 'think', text: t('cat.misc.s_6fd714d6b7') },
  { kind: 'p', text: t('cat.misc.s_17f2d14490') },
  {
    kind: 'code', lang: 'srt', more: t('cat.misc.s_086b81a565'),
    parts: [['118', true], [t('cat.misc.s_d8e85e6c39')], ['119', true], [t('cat.misc.s_4fe6122367')]],
  },
  { kind: 'p', text: t('cat.misc.s_39e3eb799e') },
  { kind: 'ul', items: [t('cat.misc.s_eb202f8e59'), t('cat.misc.s_3b349d0d41'), t('cat.misc.s_14bc57581b')] },
  { kind: 'run', text: t('cat.misc.s_3fda6effa8') },
  // v0.37.1 消息尾走查：有 usage（设计场景 23）
  { kind: 'a', id: 'a-c31', text: t('cat.misc.s_99c153e62e'), streaming: false, finishedAt: FOOTER_AT, firstTokenMs: 1200, totalMs: 8400, tokenTotal: 12345, outputTokens: 2100, tokensPerSec: 280 },
  // v0.47：无 outputTokens（只有合计）→ 不显示「输出」与 token/s
  { kind: 'a', id: 'a-c32', text: t('cat.misc.s_c38625d1fb'), streaming: false, finishedAt: FOOTER_AT, firstTokenMs: 1200, totalMs: 8400, tokenTotal: 12345 },
]
const LIVE: CatBlock[] = [
  { kind: 'user', text: t('cat.misc.s_a3105db781') },
  { kind: 'think', text: t('cat.misc.s_d149f9fde4') },
  { kind: 'p', text: t('cat.misc.s_7958198a49') },
  { kind: 'ul', items: [t('cat.misc.s_b54a2e1ae5'), t('cat.misc.s_28d3b53928'), t('cat.misc.s_69aca32d6d')] },
  // v0.37.1 消息尾走查：仅时间（设计场景 24）
  { kind: 'a', id: 'a-c21', text: t('cat.misc.s_986b0e147f'), streaming: false, finishedAt: FOOTER_AT },
]
const RUST: CatBlock[] = [
  { kind: 'p', text: t('cat.misc.s_90b4f2355d') },
  { kind: 'code', lang: 'go', more: t('cat.misc.s_417b6c1be9'), parts: [['type', true], [' SystemHandler '], ['struct', true], [' {\n    Provider *ProviderService\n}']] },
  { kind: 'p', text: t('cat.misc.s_f9e282c16e') },
  { kind: 'user', text: t('cat.misc.s_2856a33998') },
  { kind: 'think', text: t('cat.misc.s_2917d29fdf') },
  { kind: 'p', text: t('cat.misc.s_68d720907e') },
  { kind: 'ul', items: [t('cat.misc.s_8198b6339a'), t('cat.misc.s_919b8a6592'), t('cat.misc.s_5c2ebeefff')] },
]

/** 走查 ?cat_sim=clioutdated：三家各一条 CLI 版本过旧（o2 原始报错超长；o3 不带版本号；o4 不带原始报错） */
export function mockCliOutdatedConvs(): CatConv[] {
  return [
    { id: 'o1', title: t('cat.misc.s_2e0a6afa0e'), agentKind: CAT_AGENT_CLAUDE },
    { id: 'o2', title: t('cat.misc.s_ce7a1ff932'), agentKind: CAT_AGENT_CODEX },
    { id: 'o3', title: t('cat.misc.s_dff0b862fb'), agentKind: CAT_AGENT_BUILD },
    { id: 'o4', title: t('cat.misc.s_f862f7fcab'), agentKind: CAT_AGENT_CLAUDE },
  ]
}
const OUTDATED_ASK: CatBlock = { kind: 'user', text: t('cat.misc.s_504f668012') }
/** 契约 6.19.17 的 detail 原样（四行 key=value，raw 里换行是字面 \n） */
const OUTDATED_DETAIL: Record<string, string> = {
  o1: "reason=cli_outdated\nagentKind=claude_cli\ncliVersion=2.0.1\nraw=error: option '--permission-mode <mode>' argument 'manual' is invalid.\\nAllowed choices are acceptEdits, bypassPermissions, default, plan.",
  o2: "reason=cli_outdated\nagentKind=codex_cli\ncliVersion=0.98.0\nraw=error: unexpected argument '-c' found\\n\\nUsage: codex exec resume [OPTIONS] [SESSION_ID] [PROMPT]\\n  at runExec (node:internal/cli:100:0) frame 0\\n  at runExec (node:internal/cli:101:3) frame 1\\n  at runExec (node:internal/cli:102:6) frame 2\\n  at runExec (node:internal/cli:103:9) frame 3\\n  at runExec (node:internal/cli:104:12) frame 4\\n  at runExec (node:internal/cli:105:15) frame 5\\n  at runExec (node:internal/cli:106:18) frame 6\\n  at runExec (node:internal/cli:107:21) frame 7\\n  at runExec (node:internal/cli:108:24) frame 8\\n  at runExec (node:internal/cli:109:27) frame 9\\n  at runExec (node:internal/cli:110:30) frame 10\\n  at runExec (node:internal/cli:111:33) frame 11\\n  at runExec (node:internal/cli:112:36) frame 12\\n  at runExec (node:internal/cli:113:39) frame 13\\n  at runExec (node:internal/cli:114:42) frame 14\\n  at runExec (node:internal/cli:115:45) frame 15\\n  at runExec (node:internal/cli:116:48) frame 16\\n  at runExec (node:internal/cli:117:51) frame 17",
  o3: "reason=cli_outdated\nagentKind=cat_build\ncliVersion=\nraw=error: unknown option '--output-format'",
  o4: "reason=cli_outdated\nagentKind=claude_cli\ncliVersion=1.9.4\nraw=",
}
const OUTDATED: Record<string, CatBlock[]> = Object.fromEntries(
  Object.entries(OUTDATED_DETAIL).map(([id, d]) => {
    const od = parseCliOutdated(d)!
    return [id, [OUTDATED_ASK, { kind: 'sys', tone: 'err', text: CAT_CLI_OUTDATED_COPY.body(od.cli), outdated: od } as CatBlock]]
  }),
)

/** 某条对话的示意记录（每次调用返回新数组，页面可以往里追加本地消息） */
export function mockMessages(id: string, title: string): CatBlock[] {
  const base = ({ c31: SUB3, c21: LIVE, d1: RUST, ...OUTDATED } as Record<string, CatBlock[]>)[id]
  if (base) return base.map((b) => ({ ...b }))
  return [
    { kind: 'user', text: title },
    { kind: 'think', text: t('cat.misc.s_d3296a1236') },
    { kind: 'p', text: t('cat.misc.s_7dfc661aa1') },
  ]
}

export function mockFiles(id: string): CatTreeNode[] {
  if (id === 'c31') return [[0, 'd', '字幕项目'], [1, 'd', t('cat.misc.s_43760bbbb2')], [2, 'f', t('cat.misc.s_74ee209834'), true], [2, 'f', t('cat.misc.s_e49ae8652c')], [1, 'c', t('cat.misc.s_66b6a04613')], [1, 'c', t('cat.misc.s_4e6e24d35b')], [0, 'f', t('cat.misc.s_ec635b7430')]]
  if (id === 'c21') return [[0, 'd', t('cat.misc.s_b055793df4')], [1, 'f', t('cat.misc.s_259ff4d60a'), true], [1, 'f', t('cat.misc.s_b66f3d798a')]]
  return [[0, 'c', 'cat-temp-a1b2'], [0, 'd', 'hello-world'], [1, 'd', 'src'], [2, 'f', 'main.rs', true], [1, 'c', 'target'], [1, 'f', 'Cargo.toml']]
}

export function mockChanges(id: string): CatChange[] {
  if (id === 'c31') return [{ path: t('cat.misc.s_b8a44ede86'), add: 3, del: 3 }]
  if (id === 'c21') return [{ path: t('cat.misc.s_259ff4d60a'), add: 6, del: 1 }]
  return []
}

/** 首条消息生成对话标题 */
export function titleFrom(text: string): string {
  const t = text.trim().replace(/\s+/g, ' ')
  return t.length > 18 ? t.slice(0, 18) + '…' : t || t('cat.misc.s_1ac07a4bb2')
}
