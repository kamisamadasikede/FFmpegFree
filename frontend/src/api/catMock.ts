/**
 * Cat 聊天页的布局用模拟数据（纯浏览器走查：会话列表 / 欢迎页示意；Wails 里走真实 CatService）。
 * 模型与思考强度不在这里造假——由 api/cat.ts 的 List* 返回（未就绪时为空）。
 * 不调用任何 Wails 绑定。
 */

import type { CatAgentKind } from '@/api/cat'
import { CAT_AGENT_BUILD, CAT_AGENT_CLAUDE, CAT_AGENT_CODEX } from '@/api/cat'
import { CAT_CLI_OUTDATED_COPY, parseCliOutdated } from '@/components/cat/catCliOutdated'

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
      tokensPerSec?: number
    }
  /** 浅灰系统行：已停止生成 / 未就绪 / 回复失败 */
  | { kind: 'sys'; text: string; tone?: 'err'; outdated?: import('@/components/cat/catCliOutdated').CatCliOutdated }

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
  { id: 'normal', name: '普通模式', short: '普通', desc: '按平时的规则询问。这个窗口点不到确认，卡住时请按停止。', icon: 'hand', enabled: true },
  { id: 'plan', name: 'Plan', short: 'Plan', desc: '先整理计划再动手，这一阶段只看不改。', icon: 'list', enabled: true },
  { id: 'auto', name: '自动审查', short: '自动审查', desc: '自己判断能不能做。被拦住的步骤会失败并写在对话里，不会一直等确认。', icon: 'shield', enabled: true },
  { id: 'approve', name: '全部批准', short: '全部批准', desc: '写文件、跑命令都直接执行，不再询问。', icon: 'alert', enabled: true },
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
  '了解一下这个项目，用大白话告诉我它是做什么的，从哪里入手',
  '帮我找出这个文件夹里重复和没用的文件，列出来让我确认',
  '帮我做一个精美的个人主页网页，做好后直接打开给我看',
]

export function mockProjects(): CatProject[] {
  return [
    {
      id: 'p1', name: '视频素材整理', path: 'D:\\素材\\视频素材整理', missing: false, createdAt: 3, open: false, branch: 'main',
      convs: [
        { id: 'c11', title: '按拍摄日期归档素材', agentKind: CAT_AGENT_BUILD, main: true, sub: '素材 / 2026-10', time: '2d', st: 'idle' },
        { id: 'c12', title: '找出重复的片段', agentKind: CAT_AGENT_BUILD, sub: '去重清单', time: '3d', st: 'idle' },
      ],
    },
    {
      id: 'p2', name: '直播推流测试', path: 'D:\\工作\\直播推流测试', missing: false, createdAt: 2, open: true, branch: 'live',
      convs: [
        { id: 'c21', title: '推流地址轮换方案', agentKind: CAT_AGENT_BUILD, main: true, sub: '我想让三路推流轮流切换地址…', time: '58m', st: 'ok' },
        { id: 'c22', title: '断线重连日志分析', agentKind: CAT_AGENT_BUILD, sub: '重连 4 次，平均 6 秒', time: '3h', st: 'idle' },
      ],
    },
    {
      id: 'p3', name: '字幕项目', path: 'D:\\工作\\字幕项目', missing: false, createdAt: 1, open: true, branch: 'test',
      convs: [{ id: 'c31', title: '校对第 3 集时间轴', agentKind: CAT_AGENT_BUILD, main: true, sub: '你先帮我把时间轴整体后移 0.4 秒…', time: '12m', st: 'run' }],
    },
  ]
}

export function mockPlainConvs(): CatConv[] {
  return [
    { id: 'd1', title: '用 Go 背景逐步教 Rust 语法', agentKind: CAT_AGENT_BUILD },
    { id: 'd2', title: 'Docker 命令大全', agentKind: CAT_AGENT_BUILD },
    { id: 'd3', title: '整理本周会议纪要要点和待办事项', agentKind: CAT_AGENT_BUILD },
  ]
}

/** 走查消息尾用的完成时刻（本地 22:15） */
const FOOTER_AT = new Date(2026, 9, 13, 22, 15, 0).getTime()

const SUB3: CatBlock[] = [
  { kind: 'user', text: '你先帮我把时间轴整体后移 0.4 秒，再检查一下有没有字幕重叠。' },
  { kind: 'think', text: '思考完成 · 6 秒' },
  { kind: 'p', text: '好的。第 3 集一共 412 条字幕，已经整体后移 0.4 秒。检查下来有 3 处重叠：' },
  {
    kind: 'code', lang: 'srt', more: '查看更多（2 处）',
    parts: [['118', true], ['\n00:04:12,300 --> 00:04:15,100\n你先别急，听我说完。\n'], ['119', true], ['\n00:04:14,900 --> 00:04:16,200\n好，你说。']],
  },
  { kind: 'p', text: '处理建议：' },
  { kind: 'ul', items: ['第 118 条结束时间改成 `00:04:14,850`，留 50 毫秒间隔。', '第 205、366 条同样处理。', '改完我会再整体检查一遍，确认没有新的重叠。'] },
  { kind: 'run', text: '正在修改 3 处时间轴…' },
  // v0.37.1 消息尾走查：有 usage（设计场景 23）
  { kind: 'a', id: 'a-c31', text: '三处重叠已改完，没有发现新的重叠。', streaming: false, finishedAt: FOOTER_AT, tokenTotal: 12480, tokensPerSec: 186 },
]
const LIVE: CatBlock[] = [
  { kind: 'user', text: '我想让三路推流轮流切换地址，每路 30 分钟换一次，怎么安排比较稳？' },
  { kind: 'think', text: '思考完成 · 12 秒' },
  { kind: 'p', text: '可以按“错开切换”的思路来，避免三路同时断开：' },
  { kind: 'ul', items: ['第 1 路在整点切换，第 2 路在 10 分，第 3 路在 20 分。', '切换前先连上新地址，连上后再断开旧地址。', '任意一路连续失败 2 次，就停止轮换并提醒你。'] },
  // v0.37.1 消息尾走查：仅时间（设计场景 24）
  { kind: 'a', id: 'a-c21', text: '要我把这个方案写成一份可以直接导入的配置吗？', streaming: false, finishedAt: FOOTER_AT },
]
const RUST: CatBlock[] = [
  { kind: 'p', text: '把服务放进 handler 结构体是同一件事：' },
  { kind: 'code', lang: 'go', more: '查看更多（4 行）', parts: [['type', true], [' SystemHandler '], ['struct', true], [' {\n    Provider *ProviderService\n}']] },
  { kind: 'p', text: 'Rust 这里没有写成方法接收者，而是写成了参数 `State(state)`。`state.provider` 就相当于 Go 里的 `h.Provider`。' },
  { kind: 'user', text: '我大致了解了，本质上和其他语言没什么区别，只是在控制方面需要自己更严格。' },
  { kind: 'think', text: '思考完成 · 9 秒' },
  { kind: 'p', text: '对。多出来的是编译器要你把几件事写明：' },
  { kind: 'ul', items: ['一个值归谁，借出去用还是直接交出去。', '这个变量能不能改。', '这一步可能失败，所以用 `Result`，失败就用 `?` 返回。'] },
]

/** 走查 ?cat_sim=clioutdated：三家各一条 CLI 版本过旧（o2 原始报错超长；o3 不带版本号；o4 不带原始报错） */
export function mockCliOutdatedConvs(): CatConv[] {
  return [
    { id: 'o1', title: 'Claude 版本过旧', agentKind: CAT_AGENT_CLAUDE },
    { id: 'o2', title: 'Codex 版本过旧', agentKind: CAT_AGENT_CODEX },
    { id: 'o3', title: 'Grok Build 版本过旧', agentKind: CAT_AGENT_BUILD },
    { id: 'o4', title: 'Claude 版本过旧（无原始报错）', agentKind: CAT_AGENT_CLAUDE },
  ]
}
const OUTDATED_ASK: CatBlock = { kind: 'user', text: '帮我把这个目录下的字幕文件统一转成 UTF-8。' }
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
    { kind: 'think', text: '思考完成 · 5 秒' },
    { kind: 'p', text: '这里是示意内容。正式版会显示这条对话的完整记录。' },
  ]
}

export function mockFiles(id: string): CatTreeNode[] {
  if (id === 'c31') return [[0, 'd', '字幕项目'], [1, 'd', '第3集'], [2, 'f', '第3集.srt', true], [2, 'f', '第3集-校对前.srt'], [1, 'c', '第2集'], [1, 'c', '第1集'], [0, 'f', '术语表.txt']]
  if (id === 'c21') return [[0, 'd', '直播推流测试'], [1, 'f', '推流地址.txt', true], [1, 'f', '重连日志.log']]
  return [[0, 'c', 'cat-temp-a1b2'], [0, 'd', 'hello-world'], [1, 'd', 'src'], [2, 'f', 'main.rs', true], [1, 'c', 'target'], [1, 'f', 'Cargo.toml']]
}

export function mockChanges(id: string): CatChange[] {
  if (id === 'c31') return [{ path: '第3集/第3集.srt', add: 3, del: 3 }]
  if (id === 'c21') return [{ path: '推流地址.txt', add: 6, del: 1 }]
  return []
}

/** 首条消息生成对话标题 */
export function titleFrom(text: string): string {
  const t = text.trim().replace(/\s+/g, ' ')
  return t.length > 18 ? t.slice(0, 18) + '…' : t || '新对话'
}
