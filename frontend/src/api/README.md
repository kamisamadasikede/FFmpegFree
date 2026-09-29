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

**Edit**（`edit.ts`）：`validateProject`、`exportProject(EditProject, EditExportOptions)→Task`（契约名 `Export`）、`getPreviewURL(path)`、`saveProject`、`loadProject`、`listProjects(limit)`、`deleteProject`；辅助 `createPreviewSource`（404 后 HEAD 探测、重新取地址）、`findTrackOverlap` / `wouldOverlap`（拖拽 / 放置时拦同轨重叠）、`newVideoClip` / `newAudioClip` / `fillOutSec`（素材加入 clip 时用探测到的时长填 `outSec`，不能是 0）、`checkStructure`（Validate / Export）、`checkSaveLimits`（Save，只查数量上限）、`sanitizeOutputName`。素材用 `system.ts` 的 `pickFiles` 和 `media.ts` 的 `probeFiles` / `thumbnailOf`。任务类型 `edit_export`。

**Doc**（`doc.ts`）：`getDocCapabilities`、`convertToPDF(inputs, outputDir)→Task[]`（`office_pdf`）、`openPDF(path)→PDFSource`、`readPDFChunk(id, offset, length)`、`readWholePDF(src)`（循环读到 eof）、`listRecentPDFs(limit)`、`removeRecentPDFs(ids)`；`isExperimental(caps)`。

公共：`call.ts`（`AppError` 含 `reason` / `scheme` / `clipId` / `path`，`AppErrorCode` 全集，`BACKEND_ERROR_CODES`）、`taskTypes.ts`（Task / 事件载荷类型）、`sim.ts`（模拟任务引擎，走 `services/wails.ts` 的模拟事件总线，任务 store 已订阅，任务中心 / 角标能看到模拟任务）。

## 模拟怎么触发错误

在浏览器预览地址后加参数（只在对应开关为 false 时生效）：

- `?sim_err=<错误码>`：触发该错误码。Start\* / Export / ConvertToPDF / OpenPDF 等同步校验类的码由方法直接抛出；`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`CONVERT_DISK_FULL`、`PROCESS_FAILED`（Doc 还有 `IO_ERROR` / `INTERNAL` / `UNSUPPORTED`）让**任务**在模拟中途失败。`&sim_when=call|task` 可强制。
- `&sim_reason=max_sessions|duplicate_url|unknown`：`sim_err=TASK_CONFLICT` 时 detail 首行 `reason=<值>`（`unknown` = 一个前端不认识的值；不带 `sim_reason` = 缺 reason）。不带参数时，模拟层本来就会在“已有 4 路”“同地址重复”时抛这两种。
- Live：`?sim_err=LIVE_URL_INVALID&sim_reason=scheme_unsupported|malformed|missing_host|param_not_allowed|unknown`（不带 `sim_reason` = 缺 reason 行；不注入时地址本身的问题会按实际原因给 reason）；`?sim_err=LIVE_CONNECT_FAILED`（任务失败，detail 首行 `scheme=rtmp|rtmps|srt`，按地址协议给；`&sim_scheme=missing` = 缺首行，测兜底）；`?sim_missing=srt|rtmps`（UNSUPPORTED，detail 写缺哪个）、`?sim_kill=1`（停止时 5 秒强杀 → canceled）、`?sim_end=<秒>`（推满自然结束）、`?sim_err=UNSUPPORTED_PLATFORM`（Wayland）/ `SCREEN_PERMISSION_DENIED`（macOS 未授权）。
- Edit：素材文件名以 `缺失`/`missing` 开头 → NOT_FOUND，`损坏`/`broken` → PROBE_FAILED，`无声…` 放音轨 → INVALID_ARGUMENT；同轨重叠 / 越界值 / `outSec ≤ inSec`（含 0）→ INVALID_ARGUMENT，detail 首行 `clip=<id> path=<path>`（只在 Validate / Export 报；**Save 只查数量上限**）；`?sim_preview_404=1` 让第一个预览 token 立即失效。
- Doc：文件名 `加密…`、扩展名 doc/xls/ppt/csv/txt/odt/rtf → UNSUPPORTED；`损坏…` → INVALID_ARGUMENT；`缺失…` → NOT_FOUND；`超大…` → 超限；整体校验，一个不通过整批失败（detail 第一行是出错文件）。
- 模拟任务的标题带“【演示】”前缀，任务中心里显示为“演示”标签（`api/sim.ts` 的 `SIM_TITLE_PREFIX`）；开关为 false 且有 Wails 时，任务中心的活动列表 / 历史都会合并模拟任务，不会被 `ListActive` 刷新清掉。
- 自检：`npm run check:api`（esbuild 打包后在 node 跑，覆盖 reason / scheme / clipId 解析、两种 TASK_CONFLICT、LIVE_URL_INVALID 各 reason、连接失败按 scheme 出文案、未知 / 缺失兜底、停止语义、地址校验与脱敏、同轨重叠、outSec=0、Save 只查数量上限、输出名净化、模拟层主要错误）。

## 联调时要切换的地方

1. `src/api/flags.ts` 里对应开关改 `true`（三个可以分开切）。
2. 后端生成 wailsjs 绑定后，可选：把 `callService('X','Y',…)` 换成直接 import 生成文件（保留 `call()` 包装），并用生成的类型替换 `live.ts` / `edit.ts` / `doc.ts` 顶部的手写类型。
3. `LiveLayout.vue` 顶部的 `MigrationNotice`（直播仍是演示）联调完成后删除；剪辑页 `VideoEditor.vue` 与 Office 页 `OfficeConvert.vue` / `PDFPreview.vue` 目前仍是 v1 的 `V1_API_READY=false`，**本次没有改这三个页面的逻辑**（接口层已备好，页面接入是后续工作）。
4. `services/wails.ts` 的 `onSimEvent` 总线与 `stores/tasks.ts` 里对 `sim` 的分支（cancel / retry / remove / 历史）在全部开关为 true 后可删。

## 契约未冻结、可能要改的点

- Live：方法名、`PushOptions` 字段、`TASK_CONFLICT` 的 `reason` 取值（现有 `max_sessions` / `duplicate_url`，屏幕推流“同时最多 1 路”的 reason 待产品定）、缺 srt/rtmps 协议时 UNSUPPORTED 的 detail 写法与文案、`LIVE_*` 错误分类关键词（未用真实服务器验证）、RTMP 连接失败与 `malformed` / `missing_host` / `param_not_allowed` 的文案（待产品定稿）。
- Edit：`Export` 命名（已确认）、`EditExportOptions`、`GetPreviewURL` 的限长 206 在 Windows/WebView2 上是否可用（未验证，回退是 `edit_proxy`，接口不变）、clip 错误 detail 首行格式。
- Doc：大文件（> 64 MiB）路径在 Windows 未验证（验证不通过则 `OpenPDF` 对 > 64 MiB 返回 INVALID_ARGUMENT、`url` 恒空）；字体子集范围与 OFL 保留名。

## 契约疑问：已决与未决

### 已决（架构师 5 条决定，后端会写进契约新提交，前端已按此实现）

1. **TaskType 统一**：`convert | edit_export | office_pdf | live_file_push | live_screen_push | ffmpeg_install`；`live_relay`、`live_record_push`、`edit_render` 只是“保留但不再产生”的旧类型，任务中心继续忽略、不报错。（原疑问 19）
2. **停止语义**：优雅停止和自然播完都是 `succeeded`，都显示“已结束推流”，不区分；停止中 5 秒内刷新看到 `running` 可接受，不加字段。（原疑问 1、2）
3. **连接失败 / 地址不合法的稳定首行**（原疑问 4、5）：
   - `LIVE_CONNECT_FAILED` 的 detail 第一行固定 `scheme=rtmp|rtmps|srt`。`AppError.scheme` 解析它；SRT 文案“连接失败，请检查地址和口令是否正确”，RTMP / RTMPS 文案“连接失败，请检查推流地址是否正确、服务器是否在线”（**待产品定稿**）。脱敏 `params.url` 的 scheme、页面上地址的 scheme 只作首行缺失时的兜底。
   - `LIVE_URL_INVALID` 的 detail 第一行 `reason=`，取值 `scheme_unsupported` / `malformed` / `missing_host` / `param_not_allowed`（稳定枚举，只追加）。文案表 `LIVE_URL_INVALID_REASON_TEXT`：`scheme_unsupported`→“暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt”；其余三个先用“推流地址格式不正确”（**待产品定稿**）；未知值 / 缺失 reason →“推流地址不正确”。
4. **`DocCapabilities.experimental`**（布尔，字段名与前端一致）；`ReadPDFChunk` 的 `Data` 按 base64 字符串解码。（原疑问 14、16）
5. **Edit**（原疑问 9、11、12、13）：预览 `/local/<token>` 支持 HEAD，token 失效返回 404，前端 HEAD 探测后重新调用 `GetPreviewURL`（`isPreviewGone` / `createPreviewSource`，已符合）；`SaveProject` 只校验数量上限、**不校验同轨重叠**（草稿可保存），重叠只在 `ValidateProject` 和 `Export` 报；`outSec` 必须大于 `inSec`，**0 不表示到结尾**，`outSec=0` 一律 `INVALID_ARGUMENT`，前端用探测到的时长填实际值（`newVideoClip` / `newAudioClip` / `fillOutSec`，素材时长未知时不能加入时间线）。

### 仍未决

- **“已经在推”的判定**（原 3）：刷新后 `ListActive` 返回的 Task 里 `fps` / `bitrateKbps` 有值即视为已在推；running 但这三项都缺省时无法区分“连接中”和“刚开始”。建议契约写明：首条 progress 之前 `bitrateKbps` 缺省。
- **刷新后重连**（原 4 的后半）：`params` 已脱敏，刷新后拿不到完整地址，“断线自动重连”只能在页面不刷新时用。可接受的话请确认。
- **缺协议的 UNSUPPORTED**（原 6）：detail 写“缺哪个”，格式没定，模拟层暂按 `ffmpeg 缺少协议：srt`；且与直播会话 Retry 的 UNSUPPORTED 同码，建议也用 `reason=` 首行区分。
- **`CheckPushURL` 是否做会话冲突检查**（原 7）：前端当纯校验用。
- **屏幕推流**（原 8）：没有区域 / 窗口选择；`ArchiveDir` 是否需要 `Settings.archiveDir` 默认值。
- **Edit 多素材预览**（原 10）：同时预览 N 个素材占 N 个 token（登记表 256 项 LRU），是否提供批量 `GetPreviewURL`。限长 206（4 MiB）的 seek 体验待 Windows 真机验证。
- **Doc 转换产物不自动进 PDF 历史**（原 15）：预览时才 `OpenPDF`，请确认是预期。
- **错误码表**（原 17）：`UNSUPPORTED_PLATFORM` 没有专属用户文案，走兜底；`LIVE_PLAY_FAILED` / `LIVE_CORS_BLOCKED` 只由前端播放器产生。契约 §2 的清单是 17 个后端码。
- **`TASK_CONFLICT` 的 reason**（原 18）：Edit / Doc 没有定义，统一“操作冲突，请稍后再试”；屏幕推流“同时最多 1 路”的 reason 待产品定，文案表里留了追加位。
- **敏感信息**（原 20）：前端已保证完整推流地址和口令只在输入框和调用参数里，不写 localStorage / 日志 / console，列表和标题用脱敏形式；后端 `Task.title` / `params` 已脱敏。无需契约改动，仅记录。
