/**
 * Cat 聊天页的布局用模拟数据（CAT_BACKEND_READY=false：会话列表 / 欢迎页示意）。
 * 模型与思考强度不在这里造假——由 api/cat.ts 的 List* 返回（未就绪时为空）。
 * 不调用任何 Wails 绑定。
 */

import type { CatAgentKind } from '@/api/cat'
import { CAT_AGENT_BUILD } from '@/api/cat'

export type CatConvState = 'ok' | 'idle' | 'run'

/** 一段消息内容。p / li 里的 `反引号` 渲染成行内代码；code 的 parts 第二项为 true 时按关键字上色 */
export type CatBlock =
  | { kind: 'user'; text: string }
  | { kind: 'think'; text: string }
  | { kind: 'p'; text: string }
  | { kind: 'ul'; items: string[] }
  | { kind: 'code'; lang: string; parts: Array<[string, boolean?]>; more?: string }
  | { kind: 'run'; text: string }

export interface CatConv {
  id: string
  title: string
  /** 创建时锁定，之后不可改（契约 6.19.2） */
  agentKind: CatAgentKind
  /** 项目对话：第二行摘要、时间、状态点、是否「主要」 */
  sub?: string
  time?: string
  st?: CatConvState
  main?: boolean
}

export interface CatProject {
  id: string
  name: string
  open: boolean
  branch: string
  convs: CatConv[]
}

/** 文件树节点：[缩进层级, 类型 d=展开的文件夹 / c=收起的文件夹 / f=文件, 名字, 是否选中] */
export type CatTreeNode = [number, 'd' | 'c' | 'f', string, boolean?]

/** 变更文件：名字、增加行数、删除行数 */
export type CatChange = { path: string; add: number; del: number }

export interface CatAccess { id: 'ask' | 'full'; name: string; short: string; desc: string; icon: 'hand' | 'alert'; enabled: boolean }
export interface CatMode {
  id: string
  name: string
  icon: 'tool' | 'spark' | 'clock' | 'refresh'
  /** 创建会话时写入的 agentKind；一期只有 build 可点 */
  agentKind: CatAgentKind
  enabled: boolean
}

export const CAT_ACCESS: CatAccess[] = [
  { id: 'ask', name: '请求批准', short: '请求批准', desc: '编辑外部文件和使用互联网时始终询问', icon: 'hand', enabled: true },
  { id: 'full', name: '完全访问权限', short: '完全访问', desc: '可不受限制地访问互联网和你电脑上的任何文件', icon: 'alert', enabled: false },
]

/** 欢迎页模式：仅 Cat Build 可选；其余灰掉「下一期开放。」（产品 / 契约 6.19.6） */
export const CAT_MODES: CatMode[] = [
  { id: 'cat_cli', name: 'Cat CLI', icon: 'tool', agentKind: 'cat_cli', enabled: false },
  { id: 'cat_code', name: 'Cat Code', icon: 'spark', agentKind: 'cat_code', enabled: false },
  { id: 'cat_agent', name: 'Cat Agent', icon: 'clock', agentKind: 'cat_agent', enabled: false },
  { id: 'cat_build', name: 'Cat Build', icon: 'refresh', agentKind: CAT_AGENT_BUILD, enabled: true },
]

export const CAT_TRY = [
  '了解一下这个项目，用大白话告诉我它是做什么的，从哪里入手',
  '帮我找出这个文件夹里重复和没用的文件，列出来让我确认',
  '帮我做一个精美的个人主页网页，做好后直接打开给我看',
]

export function mockProjects(): CatProject[] {
  return [
    {
      id: 'p1', name: '视频素材整理', open: false, branch: 'main',
      convs: [
        { id: 'c11', title: '按拍摄日期归档素材', agentKind: CAT_AGENT_BUILD, main: true, sub: '素材 / 2026-10', time: '2d', st: 'idle' },
        { id: 'c12', title: '找出重复的片段', agentKind: CAT_AGENT_BUILD, sub: '去重清单', time: '3d', st: 'idle' },
      ],
    },
    {
      id: 'p2', name: '直播推流测试', open: true, branch: 'live',
      convs: [
        { id: 'c21', title: '推流地址轮换方案', agentKind: CAT_AGENT_BUILD, main: true, sub: '我想让三路推流轮流切换地址…', time: '58m', st: 'ok' },
        { id: 'c22', title: '断线重连日志分析', agentKind: CAT_AGENT_BUILD, sub: '重连 4 次，平均 6 秒', time: '3h', st: 'idle' },
      ],
    },
    {
      id: 'p3', name: '字幕项目', open: true, branch: 'test',
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
]
const LIVE: CatBlock[] = [
  { kind: 'user', text: '我想让三路推流轮流切换地址，每路 30 分钟换一次，怎么安排比较稳？' },
  { kind: 'think', text: '思考完成 · 12 秒' },
  { kind: 'p', text: '可以按“错开切换”的思路来，避免三路同时断开：' },
  { kind: 'ul', items: ['第 1 路在整点切换，第 2 路在 10 分，第 3 路在 20 分。', '切换前先连上新地址，连上后再断开旧地址。', '任意一路连续失败 2 次，就停止轮换并提醒你。'] },
  { kind: 'p', text: '要我把这个方案写成一份可以直接导入的配置吗？' },
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

/** 某条对话的示意记录（每次调用返回新数组，页面可以往里追加本地消息） */
export function mockMessages(id: string, title: string): CatBlock[] {
  const base = ({ c31: SUB3, c21: LIVE, d1: RUST } as Record<string, CatBlock[]>)[id]
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
