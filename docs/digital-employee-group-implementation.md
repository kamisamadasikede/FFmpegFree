# 数字员工协作群：实现与验收记录

日期：2026-10-16。对应 [设计方案](digital-employee-group-design.md)。本记录描述已落地代码与实际执行的检查，不把原设计中的所有后续能力标为完成。

## 已接入的使用流程

1. 在 Agent 侧栏进入「协作群」，从「数字员工」创建员工，填写名字、岗位、专长、关注点、交付标准、已确认的知识笔记，选择 CLI、模型和权限。
2. 建群并邀请员工，选择现有项目或独立群工作目录。本人自动成为人类群成员，可以编辑本人名字、身份和通知偏好。协调人可选，没有固定出场顺序或工作流模板。
3. 输入框使用 `@` 选择真实成员 ID，可一次向多人安排任务，也可选择「仅发消息」。同一个 CLI 的不同员工有各自的任务和原生会话。
4. 员工调用 `team_delegate` 创建真实子任务；调用 `team_ask` 向同事或本人提问；用 `team_yield` 释放原生轮次等待结果，再由应用恢复同一逻辑任务。打印普通的 `@名字` 不会冒充成功投递。
5. 员工独立流式回复。右栏查看任务、成员和文件，支持停止单个任务及其独占子任务、检查后继续、暂停派活、移出员工和归档。
6. 员工 @ 本人进入持久化收件箱并提交系统通知，引用答复只续接对应原问题，不随机交给协调人。阅读消息与回答问题分别记录。
7. 群中调用 `office_open` 自动打开现有 GenOffice 编辑页面；逐页 `office_page`、原生元素 `office_apply` 和 `office_save` 复用同一文档。其他 Office、PDF、HTML、Markdown 文件可通过 `team_open_file` 打开；运行中按秒发现新建或修改的可预览文件，关联任务与群内文件面板。

进入页面需要运行包含新 Go 绑定的桌面程序；单独打开 Vite 网页不会伪造 CLI 执行。

## 模块与数据

| 模块 | 内容 |
| --- | --- |
| `internal/team` | 身份、成员名册、原文消息、任务队列、协作等待、人类问答、停止恢复、幂等投递、收件箱、通知 outbox、文件产物、执行记录 |
| `internal/agentexec` | 单聊和群聊共用的原生执行入口，物理工作目录/父子目录互斥、原生会话互斥、共享 Skill/MCP 配置租约、可取消资源等待 |
| `internal/catagent` | 原适配器复用，新增 Claude stream-json 输入和 Grok ACP stdio 通道、Codex 显式 app-server 选择、每进程群 MCP 注入 |
| `app/team_service.go` | Wails 群聊、员工、成员、消息、任务、文件与预览接口 |
| `frontend/src/views/team` | 协作群、数字员工、@ 我的、成员选择、原文消息、分群草稿、任务面板与内嵌文档页面 |
| `0016_teams.sql` / `0017_team_artifacts.sql` | 增量 SQLite 迁移，不重建原单聊表；群消息序号与事件序号独立 |

群工具由当前 ffc 可执行程序的 `team-mcp` / `team-tool` 子命令提供，无需为通信另装 Python、Node 或编排框架。Office 继续使用项目已有的 GenOffice 运行时。

临时工具凭证只通过子进程环境传入，限制为 loopback、当前原生 run、群和成员代次；成员离群或执行结束后失效。Codex 仅为本应用的 `ffc_team` MCP server 配置原生授权；Claude 仅放行该 server 的工具；Grok 只接受当前应用工具白名单的单次 MCP 权限请求。其他 shell、文件与第三方 MCP 权限维持所选 CLI 模式。Office 写入还会检查本轮员工权限快照和群目录路径。

群目录和会话辅助数据存放于应用 DataRoot。绑定现有项目时，任务交付文件写入项目，运行数据库、凭证与辅助记录不写入代码仓库。独立群默认使用 DataRoot 下的独立 workspace。

## 真实输入能力

测试时安装版本：Codex CLI 0.162.1、Claude Code 2.1.300、Grok Build 1.0.52。

| CLI / 通道 | 当前实现与证据 | 保留的边界 |
| --- | --- | --- |
| Codex app-server | 原生 `turn/steer`，适配器子进程协议测试覆盖续聊、拒绝、错误、取消；已安装 CLI + 本地 Responses 中转的请求、流式回复、续聊及 MCP 调用通过 | 用户的实际第三方服务商未发送测试任务；其业务语义与所有网关组合没有一概验收 |
| Codex 自动兼容 | 保留自定义中转默认走原有 exec，仍可使用群 MCP；员工可显式选择 app-server | exec 不提供运行中引导；界面不冒充可用 |
| Claude stream-json | 真实已安装 CLI 接收第二条 user 消息并回传同一 UUID，连续两个原生结果通过；支持 `--system-prompt-snapshot off` 的版本才传入该参数 | 标为 `checkpoint`，可能在当前轮次结束后的检查点消费，不能承诺立即改变正在执行的工具 |
| Grok ACP stdio | 真实握手、创建/加载会话、隔离本地中转、流式消息和 MCP `use_tool` 通过；加载历史不重复显示 | 标为 `next_turn`，没有经过验证的 active-turn steer；不以停止重启或重复 `session/prompt` 冒充实时干预。ACP 暂不暴露未验证的 acceptEdits 模式 |

Grok 使用 `search_tool` / `use_tool` 发现并调用 `ffc_team__*`；Codex 的新版本可能把同一 server 展示为命名空间。适配遵循原生工具目录，不要求用户在聊天中执行额外的 shell 通信命令。

## 实际执行的验证

- `go test ./internal/team ./internal/agentexec ./app . -count=1`：通过。
- 单聊回归 `go test ./internal/service/cat -run '^Test(SingleChatSharedExecution|Stream|Footer|OrdinaryTurnNotify)' -count=1`：通过。覆盖共享执行入口、普通会话目录被删除后重建、流式输出、取消、错误、用量页脚及原通知行为。
- 群服务测试覆盖：指定员工派活、完整人类/员工名册、本人资料查询、Unicode 长原文按游标完整读取、同事真实委派与恢复、早到的人类答复、已失败的必要子任务、父任务取消子任务、离群凭证撤销、路径越界拒绝、未声明 finish 的原生退出、重启不重放、通知去重。
- `TestGroupNativePPTLivePreviewAndSave`：通过。经过群工具打开原生文档，启动同一 documentId 的预览，写入一页、读取、保存，并解压产出 `.pptx` 验证 `ppt/slides/*.xml` 包含可编辑文本。不是截图或 HTML 冒充 PPT。
- `FFC_TEST_LIVE_CLI=1 go test ./internal/catagent -run '^TestLive(CLI|Claude)' -count=1 -v`：通过。使用真实安装的三家 CLI、隔离配置和本地模拟 API，覆盖 Codex exec/app-server、自定义中转、Grok headless/ACP 的连续两轮请求，以及 Claude 检查点输入。
- `TestLiveTeamMCPRoundTrip`：三家真实 CLI 均调用独立构建的 ffc `team-mcp`，HTTP 工具请求确实到达本地授权桥，再将工具结果返回模型。该测试需要 `FFC_TEST_TEAM_APP` 指向临时构建的应用。
- 使用 `go build -tags desktop,production -ldflags '-H windowsgui'` 在临时目录构建桌面可执行文件，再运行上述三家 MCP 往返测试：通过，验证 Windows GUI 构建同样可作为 stdio 子进程提供群工具；未生成或运行安装包。
- `TestTeamNativePermissionScope`：通过。Grok ACP 的 `UseTool` 类型字段与应用工具内联调用可通过；其他 server、未知工具、shell 类型和文件间接调用被拒绝。`go vet ./internal/team ./internal/agentexec ./internal/catagent` 通过。
- `npm run check:agent`：通过，包含新增的群事件重复/缺口恢复、消息增量、跨群隔离、未读和草稿检查。
- `npx vue-tsc --noEmit`、前端生产构建：通过。
- 浏览器界面检查：使用临时 Wails 接口夹具渲染群名册、三位员工、原文消息和任务卡片，检查 1440×960 与 1024×768 截图，无页面运行异常。窄窗口默认收起任务面板，打开文档预览同时收起任务栏；群页面样式限定在本组件内。该检查没有调用真实模型或代替桌面系统通知验收。

全部 CLI 验证使用临时目录和本地模拟服务，没有读取真实服务商密钥或向用户的模型网关发送付费任务。此前全仓库 `go test ./...` 发现 Windows/安装状态/外部文档夹具相关的既有失败，因此不声称全仓测试全部通过。当前环境未配置 CGo，`-race` 无法运行；上述普通功能测试实际执行并通过。

实现过程中用原生测试确定了 `mcp_servers.ffc_team.default_tools_approval_mode="approve"` 仅对应用自身 server 的作用。官方文档入口请求返回 HTTP 403，因此没有把未成功读取的文档内容当作验证依据。

## 尚未关闭的设计项

- 同一物理工作目录采用串行执行保护；不同工作目录最多并行 4 个原生 run。尚未实现 Git worktree 自动拆分、交付合并和按任务可靠归因的增删行统计，不能把串行保护描述为已经支持同项目多人并行编辑。
- 本人维护的员工知识笔记已接入上下文；自动记忆候选、来源审阅和跨项目记忆管理尚未实现。
- 群内文件预览支持 Office、PDF、HTML、Markdown；通用代码编辑器、文件附件/文件 @ 候选和完整产物历史版本审阅尚待扩展。当前产物表记录最新关联与文件版本，Office 内部仍使用自身文档 revision 和冲突保护。
- 原生输入补充按上表显示实际能力；Claude/Grok 的 active-turn 全能力验收仍未关闭。
- 已接系统通知 API 和应用级通知跳转，尚未进行 Windows 前台/托盘真实横幅、系统勿扰、关闭应用后的冷启动点击及安装包回归。通知 outbox 不盲目重试结果未知的 OS 调用，重启时不会重新弹旧通知。
- 单聊与群执行的配置租约已接入；用户在其他设置页面直接改写原生共享配置时的完整冻结/延期机制尚需扩展，不能宣称所有外部配置修改都已隔离。
- 尚未进行完整桌面手工交互与视觉验收。生产构建和前端状态检查通过，不等同于所有屏幕尺寸和平台已验证。

上述条目继续按设计文档 G01–G07 验收；它们不阻止使用已实现的身份、群聊、工具派活、问答和文件预览，也不以“完整实现全部四阶段”对外表述。
