# 前端接口层（src/api）

后端 Live / Edit / Doc 三份契约（#19 v0.10、#22 v0.11、#23 v0.12）**还没冻结、后端也没有对应绑定**。本层先把类型、服务封装和**本地模拟**做好，联调时只换这一层。
契约依据：#19 `ac1281f`、#22 `1c8035d`、#23 `c0cd5a1`（以后端最新提交为准，方法名可能小改）。

## 开关

`src/api/flags.ts`，默认全部 `false`（走模拟，不发任何网络请求）：

| 开关 | 文件 | true 时调用 |
|---|---|---|
| `LIVE_BACKEND_READY` | `live.ts` | `window.go.app.LiveService.*`，停止走 `TaskService.Cancel` |
| `EDIT_BACKEND_READY` | `edit.ts` | `window.go.app.EditService.*` |
| `DOC_BACKEND_READY` | `doc.ts` | `window.go.app.DocService.*` |

开关为 true 时经 `call.ts` 的 `callService(service, method, ...args)` 按名字取 `window.go`，**不 import wailsjs 生成文件**（没有绑定时 `vue-tsc` / `vite build` 也能过）。绑定不存在会抛 `UNSUPPORTED`，不会悄悄走模拟。

## 方法清单

**Live**（`live.ts`）：`startFilePush(FilePushRequest)→Task`、`startScreenPush(ScreenPushRequest)→Task`、`getCaptureCapabilities()`、`listScreens()`、`checkPushURL(url)→PushURLInfo`；停止 `stopPush(taskId)`（= `TaskService.Cancel`）；`listRunning()`（`TaskService.ListActive` 里的 live_*）；`watchLiveTask(id, handlers)`（`task:progress` / `task:status`）；素材 `pickMaterial()`（`SystemService.PickFiles` + `MediaService.Probe`）。任务类型 `live_file_push` / `live_screen_push`，Task 新增 `fps` / `bitrateKbps` / `droppedFrames`。

**Edit**（`edit.ts`）：`validateProject`、`exportProject(EditProject, EditExportOptions)→Task`（契约名 `Export`）、`getPreviewURL(path)`、`saveProject`、`loadProject`、`listProjects(limit)`、`deleteProject`；辅助 `createPreviewSource`（404 后重新取地址）、`findTrackOverlap` / `wouldOverlap`（拖拽 / 放置时拦同轨重叠）、`checkStructure`、`sanitizeOutputName`。素材用 `system.ts` 的 `pickFiles` 和 `media.ts` 的 `probeFiles` / `thumbnailOf`。任务类型 `edit_export`。

**Doc**（`doc.ts`）：`getDocCapabilities`、`convertToPDF(inputs, outputDir)→Task[]`（`office_pdf`）、`openPDF(path)→PDFSource`、`readPDFChunk(id, offset, length)`、`readWholePDF(src)`（循环读到 eof）、`listRecentPDFs(limit)`、`removeRecentPDFs(ids)`；`isExperimental(caps)`。

公共：`call.ts`（`AppError` 含 `reason` / `clipId` / `path`，`AppErrorCode` 全集，`BACKEND_ERROR_CODES`）、`taskTypes.ts`（Task / 事件载荷类型）、`sim.ts`（模拟任务引擎，走 `services/wails.ts` 的模拟事件总线，任务 store 已订阅，任务中心 / 角标能看到模拟任务）。

## 模拟怎么触发错误

在浏览器预览地址后加参数（只在对应开关为 false 时生效）：

- `?sim_err=<错误码>`：触发该错误码。Start\* / Export / ConvertToPDF / OpenPDF 等同步校验类的码由方法直接抛出；`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`CONVERT_DISK_FULL`、`PROCESS_FAILED`（Doc 还有 `IO_ERROR` / `INTERNAL` / `UNSUPPORTED`）让**任务**在模拟中途失败。`&sim_when=call|task` 可强制。
- `&sim_reason=max_sessions|duplicate_url|unknown`：`sim_err=TASK_CONFLICT` 时 detail 首行 `reason=<值>`（`unknown` = 一个前端不认识的值；不带 `sim_reason` = 缺 reason）。不带参数时，模拟层本来就会在“已有 4 路”“同地址重复”时抛这两种。
- Live：`?sim_missing=srt|rtmps`（UNSUPPORTED，detail 写缺哪个）、`?sim_kill=1`（停止时 5 秒强杀 → canceled）、`?sim_end=<秒>`（推满自然结束）、`?sim_err=UNSUPPORTED_PLATFORM`（Wayland）/ `SCREEN_PERMISSION_DENIED`（macOS 未授权）。
- Edit：素材文件名以 `缺失`/`missing` 开头 → NOT_FOUND，`损坏`/`broken` → PROBE_FAILED，`无声…` 放音轨 → INVALID_ARGUMENT；同轨重叠 / 越界值 → INVALID_ARGUMENT，detail 首行 `clip=<id> path=<path>`；`?sim_preview_404=1` 让第一个预览 token 立即失效。
- Doc：文件名 `加密…`、扩展名 doc/xls/ppt/csv/txt/odt/rtf → UNSUPPORTED；`损坏…` → INVALID_ARGUMENT；`缺失…` → NOT_FOUND；`超大…` → 超限；整体校验，一个不通过整批失败（detail 第一行是出错文件）。
- 自检：`npm run check:api`（esbuild 打包后在 node 跑，覆盖 reason / clipId 解析、两种 TASK_CONFLICT、未知 reason 兜底、停止语义、地址校验与脱敏、同轨重叠、输出名净化、模拟层主要错误）。

## 联调时要切换的地方

1. `src/api/flags.ts` 里对应开关改 `true`（三个可以分开切）。
2. 后端生成 wailsjs 绑定后，可选：把 `callService('X','Y',…)` 换成直接 import 生成文件（保留 `call()` 包装），并用生成的类型替换 `live.ts` / `edit.ts` / `doc.ts` 顶部的手写类型。
3. `LiveLayout.vue` 顶部的 `MigrationNotice`（直播仍是演示）联调完成后删除；剪辑页 `VideoEditor.vue` 与 Office 页 `OfficeConvert.vue` / `PDFPreview.vue` 目前仍是 v1 的 `V1_API_READY=false`，**本次没有改这三个页面的逻辑**（接口层已备好，页面接入是后续工作）。
4. `services/wails.ts` 的 `onSimEvent` 总线与 `stores/tasks.ts` 里对 `sim` 的分支（cancel / retry / remove / 历史）在全部开关为 true 后可删。

## 契约未冻结、可能要改的点

- Live：方法名、`PushOptions` 字段、`TASK_CONFLICT` 的 `reason` 取值（现有 `max_sessions` / `duplicate_url`，屏幕推流“同时最多 1 路”的 reason 待产品定）、缺 srt/rtmps 协议时 UNSUPPORTED 的 detail 写法与文案、`LIVE_*` 错误分类关键词（未用真实服务器验证）。
- Edit：`Export` 命名（已确认）、`EditExportOptions`、`GetPreviewURL` 的限长 206 在 Windows/WebView2 上是否可用（未验证，回退是 `edit_proxy`，接口不变）、clip 错误 detail 首行格式。
- Doc：`DocCapabilities.experimental` 尚未写进契约；大文件（> 64 MiB）路径在 Windows 未验证（验证不通过则 `OpenPDF` 对 > 64 MiB 返回 INVALID_ARGUMENT、`url` 恒空）；字体子集范围与 OFL 保留名。

## 对契约的疑问

1. **Live 优雅停止 vs 强杀**：只看 status（succeeded=已结束推流，canceled=已强制停止），这点已定。但“停止中”这个中间状态 Task 里没有——`Cancel` 立即返回，任务仍是 `running`，直到 ffmpeg 退出（最多 5 秒）。前端只能自己在点击后本地显示“正在停止…”，页面刷新后就丢了。建议 Task 或 `task:progress` 带一个 `stopping` 标志，或者接受刷新后看不到。
2. **Live 自然结束 vs 用户停止**：非循环文件推流播完也是 `succeeded`（error 为空），和“用户优雅停止”无法区分，都会显示“已结束推流”。可接受，但如果产品想区分“播放完成”，需要额外字段。
3. **“已经在推”的判定**：契约说 running 且收到第一条 `task:progress`。刷新页面后 `ListActive` 返回的 Task 里 `fps` / `bitrateKbps` 有值即可视为已在推，但 running 且这三项都为 0/缺省时无法区分“连接中”和“刚开始”。建议契约明确：`Task.startedAt` 之后、首条 progress 之前 `bitrateKbps` 缺省。
4. **推流地址脱敏后无法“接回”并自动重连**：`params` 已脱敏，刷新后拿不到完整地址，所以“断线自动重连”只能在页面不刷新时用（用内存里的地址再调 Start\*）。SRT 连接失败统一 `LIVE_CONNECT_FAILED`，前端文案“连接失败，请检查地址和口令是否正确”，但 RTMP 的 `LIVE_CONNECT_FAILED` 用另一句——前端靠 scheme 区分，而任务中心失败行只能靠脱敏后的 `params.url` 判断 scheme，比较脆弱；建议 `Task` 带 `scheme` 或 `error.detail` 首行 `scheme=srt`。
5. **`LIVE_URL_INVALID` 一个码覆盖协议不支持、格式错、端口越界、srt listener**：前端在失焦时自己校验（`utils/liveUrl.ts`）并对“协议不支持”显示产品文案，但后端返回的同一个码只能靠 `message` 区分，契约说“不要靠猜测”。地址框提示对后端返回的 `LIVE_URL_INVALID` 目前显示后端 `message`。建议增加 `reason=protocol_unsupported|format|port|srt_mode` 的稳定首行，与 TASK_CONFLICT 同一套。
6. **UNSUPPORTED（缺协议）**：detail 写“缺哪个”，格式没定。模拟层按 `ffmpeg 缺少协议：srt`；前端目前对所有 Start\* 的 UNSUPPORTED 显示同一句“当前 ffmpeg 不支持这种推流协议”。同一个码在 Retry（直播会话不能重试）里含义完全不同，建议也用 `reason=` 首行区分。
7. **`CheckPushURL` 是否做会话冲突检查**：契约括号里写“如果它做会话冲突检查”，没定。前端把它当纯校验用。
8. **屏幕推流**：`ScreenPushRequest` 没有分辨率上限之外的“区域/窗口”选择，UI 上“窗口”“摄像头”来源暂不支持（页面提示）。`captureMode` 之类 v0.4 字段已删，OK。`ArchiveDir` 空表示不存档，前端在勾选存档时用默认输出位置或让用户选文件夹——是否应有 `Settings.archiveDir`？
9. **Edit：GetPreviewURL 的使用方式**：token 进程内有效、登记表最多 256 项 LRU，所以同时预览很多素材时旧 token 会被挤掉（404）。前端已做“404 → 重新 GetPreviewURL”，但 `<video>` 遇到 404 时浏览器只给 `MediaError`（无状态码），只能用 HEAD 探一次确认；HEAD 是否被 Handler 允许（契约写 GET/HEAD 允许）请后端保证。另外限长 206（4 MiB）会让 `<video>` 频繁发 Range 请求，seek 体验待真机验证。
10. **Edit：多个 `<video>` 合成监视器**同时预览 N 个素材会占用 N 个 token；建议契约说明是否支持批量 `GetPreviewURL`。
11. **Edit：`outSec = 0` 的重叠判定**：同轨重叠要看 clip 时间线长度，`outSec=0` 时需要素材时长；后端有探测结果，前端只在探测过（`MediaService.Probe`）后才能准确拦截，未探测的按 0 长度不拦。契约没说这种情况后端是否会判重叠（应该会，因为探测后总长已知）。
12. **Edit：`SaveProject` 只做结构校验，不查“同轨重叠”？** 契约 6.11.5 说只做 6.11.2 的前两条（结构与范围）。同轨重叠在哪一条里不明确；前端假定 Save 也拒绝（与模拟层一致），若后端允许保存重叠工程，`LoadProject` 载入后要能显示并让用户修复。
13. **Edit：`EditProject.updatedAt` / `schemaVersion=0`**：新建工程时前端传 `schemaVersion=1`，契约没说传 0 是否等价于 1。
14. **Doc：`experimental` 字段**未在契约里；名称与位置（`DocCapabilities.experimental`？）请确认。前端缺省按 true。
15. **Doc：转换完成后自动进 PDF 历史？** 契约 6.12.4 第 6 点：不自动进，前端需要预览时对 outputPath 调 `OpenPDF`（会写入最近列表）。“打开输出”和“预览”会因此改变最近列表，请确认是预期。
16. **Doc：`ReadPDFChunk` 的 base64**：Go `[]byte` 在 Wails 里是 `string`（base64）还是 `number[]`？生成的 TS 类型会告诉我们，`decodeChunk` 目前按 base64 字符串处理。
17. **错误码表**：`LIVE_PLAY_FAILED` / `LIVE_CORS_BLOCKED` 只由前端播放器产生，`AppErrorCode` 里保留但不在 `BACKEND_ERROR_CODES` 里；契约 §2 的清单是 17 个后端码（#19/#22/#23 三份一致）。`UNSUPPORTED_PLATFORM` 和 `UNSUPPORTED` 在文档页、直播页的用户文案没有专门约定，目前 `UNSUPPORTED_PLATFORM` 走 `resolveError` 兜底“出错了”+ 后端 message。
18. **`TASK_CONFLICT` 的其他触发**（Edit/Doc 没有 reason 定义）：一律显示“操作冲突，请稍后再试”。
19. **旧类型**：`edit_render`（Edit 改名前）与 `live_relay` / `live_record_push` 前端一律忽略；`#22`/`#23` 头部的 `TaskType` 注释里仍写着 `live_relay | live_record_push`、#19 里仍写着 `edit_render`，合并三份契约时请统一（当前三份互相不一致）。
20. **敏感信息**：前端保证完整推流地址和口令只在输入框和调用参数里，不写 localStorage / 日志 / console，任务标题 / 日志行只用 `redactPushUrl` 的结果；后端 `Task.title` / `params` 已脱敏，这条已满足。
