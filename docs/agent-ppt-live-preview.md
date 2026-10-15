# Agent 原生 PPT 生成与实时预览

> 历史 OfficeCLI 方案，已被 [GenOffice 集成与改造方案](genoffice-integration-plan.md) 替代。当前 PPT 主路由直接嵌入 GenOffice 官方编辑页面；本文不再代表当前实现状态。

## 1. 整体架构设计

组件图（文字）：

```text
用户聊天请求 → 现有 Codex / Claude Code / Grok Build Agent
                         ↓ 本轮 developer rules 引导读取内置 Skill
                ffc-ppt-live/SKILL.md + office.cjs
                         ↓ 原生 OfficeCLI add / set / remove / move
                  可编辑 .pptx（按页/小批次落盘）
                         ↓
           OfficeCLI watch --port 0（仅监听 loopback）
                         ↓ SSE update + CLI HTML renderer
              Go officecli.Manager（多文件缓存、租约、权限重验）
                         ↑ 外部磁盘写入：400ms 版本观察 + CLI view html
                         ↓ Wails ReadCatPptPreview（版本相同时不传 HTML）
         Agent 文件面板 / 多个 PPT 标签 / CatPptxPreview
                         ↓ 安全 srcdoc + 隔离 iframe
                   幻灯片布局、翻页、缩放、跟随修改
```

应用管理 Watch，不让 Agent 启动互相冲突的 Watch。预览生命周期独立于单轮任务：任务结束后预览仍可用；关闭最后一个同文件标签或应用退出时释放进程和内存。标签切换保留已打开预览。

生成前 Skill 的 `prepare` 调用本轮 HTTP 桥，提前打开等待面板；每页/小批次修改保存后，OfficeCLI 推送 SSE，应用读取最新 HTML，用户在同一个应用内看见生成进度。其他工具直接写入 PPTX 时，用有效且稳定的 ZIP 版本触发 CLI 渲染补足闭环。这里的实时单位是一次有效文件修改，而不是模型 token。

## 2. 技术选型与理由

- **OfficeCLI v1.0.157**：官方原生可执行文件，支持路径编辑、可编辑形状/文字、HTML 渲染、SSE Watch；无需安装 .NET 或 PowerPoint。使用 Apache-2.0 项目能力，通过独立进程集成，无复制上游实现。
- **Go 管理器**：复用已有 Wails 后端与进程生命周期管理，跨平台私有安装，固定版本官方 SHA256 校验，下载镜像失败后尝试 GitHub。没有全局 npm 安装、远程脚本执行或 PATH 修改。
- **后端消费 SSE + Wails 读取缓存**：上游存在编辑、切换文件等 HTTP API，所以不直接把整个 Watch 服务暴露给 iframe。Go 只读 root/events；前端读取受授权缓存。
- **Vue iframe**：展示 CLI 已渲染的 HTML/CSS，浏览器不解析 OOXML。删除上游脚本及活动文档内容，只注入自己的视口控制代码；iframe opaque origin，CSP 禁止网络请求和文档脚本。
- **跨 CLI Skill**：内置应用目录中的 SKILL.md 与 Node 包装脚本；每轮通过规则提供真实路径与短期能力凭证，所有底层 CLI 使用同一个 PPT 引擎。用户自定义共享 Skill 不会被覆盖。

已实际核对官方 README、v1.0.157 Watch 命令与 SSE 实现、官方 npm 安装器、发布资产摘要，并用 Windows x64 二进制实测 create/add/set/watch。尤其注意：上游 Watch 明确不检测外部文件编辑，不能单凭 Watch 实现所有修改刷新。

来源：[OfficeCLI](https://github.com/iOfficeAI/OfficeCLI)、[固定发布版本](https://github.com/iOfficeAI/OfficeCLI/releases/tag/v1.0.157)、[官方 Skill](https://officecli.ai/SKILL.md)。AionUi 是体验参考，本次未复制或声称已完成其源码功能对照。

## 3. 分步实现计划与当前落地

1. 核对原生命令、输出、保存行为与本地 Watch：已实现固定版本工具管理与 HTML 渲染。
2. 管理 Watch：已实现同文件复用、独立租约、SSE 接收、无变化省略 HTML、外部写入稳定检测、失败 CLI 静态重渲染。
3. 编写内置 Skill：已实现大纲→生成前打开→逐页原生写入→定位编辑→验证/保存；Node 包装脚本透传参数，不拼 shell 命令。
4. 本轮桥接：已实现 `/ppt` 的 `tool/prepare`，复用短期 Bearer，禁浏览器 Origin，限制会话工作区，允许尚未生成的 PPTX 路径。
5. 前端面板：已实现多 PPT 标签、默认适应宽度、缩放/翻页、跟随修改开关、旧画面保留、重试与静态 Office 预览入口。
6. 自动发现：本轮新增及修改的办公文件可自动打开；Skill 显式打开可比目录扫描更早出现。
7. 验证：纯单元测试无需安装工具；原生集成测试通过环境变量启用，覆盖真实生成、编辑、move/remove、外部替换、原生 ZIP 与形状。浏览器脚本连接测试管理器读取真实 CLI HTML，验证尺寸、翻页、刷新、标签关闭释放及隔离。

未将大文件压测、所有平台实测、所有复杂 Office 特性的视觉一致性描述为已完成。验收记录随测试结果补充。

## 4. 核心文件与模块

| 文件 | 职责 |
| --- | --- |
| `internal/officecli/tool.go` | 固定版本、双源下载与 SHA256、原生进程、CLI HTML 渲染 |
| `internal/officecli/manager.go` | 工具后台安装、Watch/SSE、磁盘变更、缓存、租约与释放 |
| `internal/officecli/html.go` | CLI HTML 活动内容清理 |
| `internal/service/cat/ppt_preview.go` | 会话路径授权、预览绑定与生成前打开 |
| `internal/service/cat/ppt_skill.go` | go:embed 与内置 Skill 部署 |
| `internal/service/cat/pptskill/SKILL.md` | 多 Agent 操作规范与视觉检查建议 |
| `internal/service/cat/pptskill/scripts/office.cjs` | 能力桥、工具就绪等待、参数安全透传 |
| `frontend/src/api/catPptx.ts` | 预览 API |
| `frontend/src/components/cat/CatPptxPreview.vue` | 状态、工具栏、重试、静态降级 |
| `frontend/src/components/docs/preview/PvPptxHtml.vue` | 隔离 iframe 与消息通道 |
| `frontend/src/components/docs/preview/pptxFrameRuntime.js` | 只负责缩放、翻页、变更定位 |
| `internal/officecli/manager_test.go` | 安全/租约/ZIP/真实原生链路验证 |
| `frontend/scripts/check-ppt-live.cjs` | 真实 CLI HTML 的浏览器验收 |

已有 CatFilesPanel、CatFilePreview、CatDocumentPreview、DocPreviewModal、文档服务、应用关闭处理与 turn workspace observer 做对应接入。

## 5. 关键代码与调用方式

Skill 使用的调用形式（真实 endpoint/token 来自当前任务规则，不写入产物）：

```text
node ABSOLUTE_OFFICE_SCRIPT --endpoint TURN_ENDPOINT --token TURN_TOKEN -- create output/topic.pptx
node ABSOLUTE_OFFICE_SCRIPT --endpoint TURN_ENDPOINT --token TURN_TOKEN -- add output/topic.pptx / --type slide --prop title=Summary
node ABSOLUTE_OFFICE_SCRIPT --endpoint TURN_ENDPOINT --token TURN_TOKEN -- set output/topic.pptx /slide[3]/shape[1] --prop size=48
```

Watch 集成骨架（实际实现见 manager.go）：

```go
watch := exec.CommandContext(ctx, verifiedExecutable, "watch", file, "--port", "0")
// 从 stdout 获取实际 loopback 端口；使用 proc.Start/Cancel 统一回收进程树。
// 后端消费 GET /events。update → 获取最新 GET / HTML → SafeHTML → 增加应用版本。
// 文件变化且上游无 SSE → CLI view file html -o temp → SafeHTML → 增加应用版本。
// Start/Read 验证会话工作区、符号链接及 junction；Release 撤销租约。
```

前端闭环骨架（实际实现见 CatPptxPreview.vue）：

```ts
const lease = await startPptPreview(convId, relPath)
const next = await readPptPreview(lease.id, lastVersion)
if (next.html) postToSandbox({ type: 'html', html: next.html, slide: next.slide })
// 相同版本没有 HTML，避免重复传输大页资源。关闭组件立即 release。
```

iframe 的 `sandbox="allow-scripts"` 不包含 same-origin。只运行随机 nonce 授权的内部视口脚本；`connect-src 'none'`，图片/字体限 data URL，禁任意网页、远程脚本、下载及编辑 API。双向消息校验 iframe window 和随机 channel。

## 6. 风险与应对

| 风险 | 当前措施与边界 |
| --- | --- |
| 首次安装慢或网络不可用 | 后台安装，预览不阻塞聊天/关闭标签；明确错误与重试；官方镜像/GitHub 双源、固定 SHA256 |
| Watch 进程启动失败/退出 | CLI 静态 HTML 渲染继续跟踪文件；可切换 Office/WPS PDF 预览，未安装排版引擎时显示设置入口 |
| 其他工具保存不通知 Watch | 400ms 文件版本观察，连续稳定后调用 CLI，不在浏览器解析 XML |
| Resident 导致磁盘内容延迟 | 包装脚本禁自动 resident、设置每次 flush；提供 save；外部已有 resident 混用需先 close 保存，不能自动关闭用户未保存的外部会话 |
| Agent 一次性生成导致黑盒 | Skill 先 prepare，再按页/小批次写入；文件操作边界才可见，无法展示不存在的中间 Office 状态 |
| 不完整 ZIP、资源过大 | 保留最后有效预览，等待下次稳定保存；100MiB 文件、48MiB HTML、512MiB 解压/256MiB 单资源、30,000 ZIP 条目、8 个文件并行上限 |
| 文件路径越界/链接换向 | 会话根校验、禁止 traversal、每个现存路径段检查符号链接/junction；读取和渲染重验。仍依赖本机账户权限，非跨用户安全沙箱 |
| HTML 主动内容或上游编辑 API | 后端清理 + iframe 隔离 + CSP + 随机消息通道；不直接嵌入可写 Watch API |
| 大文稿与图片传输 | 多版本合并、版本未变不发 HTML、缓存同文件复用；当前更改仍发送完整 HTML，后续可做受控逐页 patch |
| PowerPoint 完整视觉兼容 | OfficeCLI CSS 保留原生布局，但缺字体、SmartArt、3D、视频、复杂效果可能存在差异；隔离策略禁远程素材/脚本，使用静态 Office/WPS 对照 |
| Morph/动画表现 | 产物可保留原生编辑能力；当前 HTML 控件为静态页预览，不承诺转场/所有动画播放 |
| 多 Agent 同时编辑同一文件 | 预览共享最新状态，但不提供写入合并或多人协作锁；并行任务应使用独立文件，用户要求同文件编辑时需顺序保存 |

本地 Watch 由上游绑定 loopback 并自带 Host 校验。应用前端不获得其端口或编辑凭证。本机同账户进程仍可能访问上游端口；这里防的是网页与工作区越界，不是对同账户恶意进程的隔离。

## 7. 后续扩展

- 受控逐页 HTML patch、图像资源去重、按需加载长文稿，减少多页大图的完整 HTML 传输。
- Shape stable ID 与 `!!` 命名、原生 Morph 生成与播放验收；提供元素点选后直接添加自然语言修改上下文。
- CLI PNG/SVG 截图→多模态视觉检查→溢出/对齐自动修复，限制轮数与修改范围。
- 复用同一只读预览管理器接入 OfficeCLI DOCX/XLSX；仍保留已有 Word/Excel 编辑保护与排版引擎。
- CLI 撤销/重做、每页历史、导出 PDF 与演讲模式；共享 MCP 工具接口可在已有扩展架构内复用，无需改每家 Agent 的专有协议。
- 自动组件升级策略、其他平台运行测试、复杂文稿视觉基准与长期性能压测。
