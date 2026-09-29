# 前端接口层（src/api）

后端 Live / Edit / Doc 三份契约（#19 v0.10、#22 v0.11、#23 v0.12）**还没冻结、后端也没有对应绑定**。本层先把类型、服务封装和**本地模拟**做好，联调时只换这一层。
契约依据：#19 `ac1281f`、#22 `1c8035d`、#23 `c0cd5a1`（以后端最新提交为准，方法名可能小改）。

## 开关

`src/api/flags.ts`，除 `ABOUT_BACKEND_READY` 外默认全部 `false`（走模拟，不发任何网络请求）：

| 开关 | 文件 | true 时调用 |
|---|---|---|
| `LIVE_BACKEND_READY`（已打开，联调中） | `live.ts` | 生成的绑定 `wailsjs/go/app/LiveService`（直接 import，类型用 `models.live.*`），停止走 `TaskService.Cancel`，刷新后 `TaskService.ListActive` 接回。**只有 Wails 里（有 `window.go`）才走真实后端**（`liveIsReal()`），纯浏览器环境仍走模拟；“演示”提示 / 演示素材 / 任务中心“演示”标签只在模拟环境出现 |
| `EDIT_BACKEND_READY` | `edit.ts` | `window.go.app.EditService.*` |
| `DOC_BACKEND_READY` | `doc.ts` | 生成绑定 `wailsjs/go/app/DocService`（后端 #29 已合入，开关为 `true`）。纯浏览器（没有 `window.go`）保留模拟，`isDocSim()` 为 true，文档页只在这时显示“演示”提示 |
| `ABOUT_BACKEND_READY` | `about.ts` | 生成绑定 `wailsjs/go/main/App` 的 `GetAppVersion()` / `GetLicenseText(name)`（后端 #34、#36，已合入，开关为 `true`）。纯浏览器开发环境（没有 `window.go`）始终走模拟：版本“开发版”、许可文本是标注“演示文本”的 OFL 前几行 |

开关为 true 时经 `call.ts` 的 `callService(service, method, ...args)` 按名字取 `window.go`，**不 import wailsjs 生成文件**（没有绑定时 `vue-tsc` / `vite build` 也能过）。绑定不存在会抛 `UNSUPPORTED`，不会悄悄走模拟。

## 方法清单

**Live**（`live.ts`）：`startFilePush(FilePushRequest)→Task`、`startScreenPush(ScreenPushRequest)→Task`、`getCaptureCapabilities()`、`listScreens()`、`checkPushURL(url)→PushURLInfo`；停止 `stopPush(taskId)`（= `TaskService.Cancel`）；`listRunning()`（`TaskService.ListActive` 里的 live_*）；`watchLiveTask(id, handlers)`（`task:progress` / `task:status`）；素材 `pickMaterial()`（`SystemService.PickFiles` + `MediaService.Probe`）。任务类型 `live_file_push` / `live_screen_push`，Task 新增 `fps` / `bitrateKbps` / `droppedFrames`。

**Edit**（`edit.ts`）：`validateProject`、`exportProject(EditProject, EditExportOptions)→Task`（契约名 `Export`）、`getPreviewURL(path)`、`saveProject`、`loadProject`、`listProjects(limit)`、`deleteProject`；辅助 `createPreviewSource`（404 后 HEAD 探测、重新取地址）、`findTrackOverlap` / `wouldOverlap`（拖拽 / 放置时拦同轨重叠）、`newVideoClip` / `newAudioClip` / `fillOutSec`（素材加入 clip 时用探测到的时长填 `outSec`，不能是 0）、`checkStructure`（Validate / Export）、`checkSaveLimits`（Save，只查数量上限）、`sanitizeOutputName`。素材用 `system.ts` 的 `pickFiles` 和 `media.ts` 的 `probeFiles` / `thumbnailOf`。任务类型 `edit_export`。

**Doc**（`doc.ts`）：`getDocCapabilities`、`convertToPDF(inputs, outputDir)→Task[]`（`office_pdf`）、`openPDF(path)→PDFSource`、`readPDFChunk(id, offset, length)`、`readWholePDF(src)`（循环读到 eof，按原始字节处理）、`loadPDF(path)`（≤ 64 MiB 读整份；更大的返回 `/local/<token>`，先 `HEAD` 探测，404 重新 `OpenPDF` 只重试一次）、`listRecentPDFs(limit)`（limit > 200 按 200）、`removeRecentPDFs(ids)`、`listOfficeHistory()`（`TaskService.List` 的 `office_pdf` 终态任务，刷新后接回）；`isExperimental(caps)`。文档页错误文案统一走 `errors/errorMessages.ts` 的 `docErrorText`。

公共：`call.ts`（`AppError` 含 `reason` / `scheme` / `clipId` / `path`，`AppErrorCode` 全集，`BACKEND_ERROR_CODES`）、`taskTypes.ts`（Task / 事件载荷类型）、`sim.ts`（模拟任务引擎，走 `services/wails.ts` 的模拟事件总线，任务 store 已订阅，任务中心 / 角标能看到模拟任务）。

## 模拟怎么触发错误

在浏览器预览地址后加参数（只在对应开关为 false 时生效）：

- `?sim_err=<错误码>`：触发该错误码。Start\* / Export / ConvertToPDF / OpenPDF 等同步校验类的码由方法直接抛出；`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`CONVERT_DISK_FULL`、`PROCESS_FAILED`（Doc 还有 `IO_ERROR` / `INTERNAL` / `UNSUPPORTED`）让**任务**在模拟中途失败。`&sim_when=call|task` 可强制。
- `&sim_reason=max_sessions|duplicate_url|unknown`：`sim_err=TASK_CONFLICT` 时 detail 首行 `reason=<值>`（`unknown` = 一个前端不认识的值；不带 `sim_reason` = 缺 reason）。不带参数时，模拟层本来就会在“已有 4 路”“同地址重复”时抛这两种。
- Live：`?sim_err=LIVE_URL_INVALID&sim_reason=scheme_unsupported|malformed|missing_host|param_not_allowed|unknown`（不带 `sim_reason` = 缺 reason 行；不注入时地址本身的问题会按实际原因给 reason）；`?sim_err=LIVE_CONNECT_FAILED`（任务失败，detail 首行 `scheme=rtmp|rtmps|srt`，按地址协议给；`&sim_scheme=missing` = 缺首行，测兜底）；`?sim_missing=rtmp|rtmps|srt`（UNSUPPORTED，detail 单独一行 `missing=<协议名>`）、`?sim_kill=1`（停止时 5 秒强杀 → canceled）、`?sim_end=<秒>`（推满自然结束）、`?sim_err=UNSUPPORTED_PLATFORM`（Wayland）/ `SCREEN_PERMISSION_DENIED`（macOS 未授权）。
- Edit：素材文件名以 `缺失`/`missing` 开头 → NOT_FOUND，`损坏`/`broken` → PROBE_FAILED，`无声…` 放音轨 → INVALID_ARGUMENT；同轨重叠 / 越界值 / `outSec ≤ inSec`（含 0）→ INVALID_ARGUMENT，detail 首行 `clip=<id> path=<path>`（只在 Validate / Export 报；**Save 只查数量上限**）；`?sim_preview_404=1` 让第一个预览 token 立即失效。
- Doc：文件名 `加密…`、扩展名 doc/xls/ppt/csv/txt/odt/rtf → UNSUPPORTED；`损坏…` → INVALID_ARGUMENT；`缺失…` → NOT_FOUND；`超大…` → 超限；整体校验，一个不通过整批失败（detail 第一行是出错文件）。
- 模拟任务的标题带“【演示】”前缀，任务中心里显示为“演示”标签（`api/sim.ts` 的 `SIM_TITLE_PREFIX`）；开关为 false 且有 Wails 时，任务中心的活动列表 / 历史都会合并模拟任务，不会被 `ListActive` 刷新清掉。
- 自检：`npm run check:api`（esbuild 打包后在 node 跑，覆盖 reason / scheme / clipId 解析、两种 TASK_CONFLICT、LIVE_URL_INVALID 各 reason、连接失败按 scheme 出文案、未知 / 缺失兜底、停止语义、地址校验与脱敏、同轨重叠、outSec=0、Save 只查数量上限、输出名净化、模拟层主要错误）。

## 关于页（`about.ts`）

`getAppVersion()` → `GetAppVersion()`（构建时 `-ldflags` 注入，没注入返回“开发版”）；`getLicenseText(name)` → `GetLicenseText(name)`，`name` 前端只传 `"OFL"`（Noto Sans SC）；后端白名单里仍有 `"OFL-Nunito"`，但 v2 界面不使用 Nunito，关于页已去掉该入口，前端类型 `LicenseName` 只含 `"OFL"`。调用方只传 `src/config/about.ts` 里列出的常量，不传用户输入；未知名字后端返回 `INVALID_ARGUMENT`。`GetAppVersion`、`GetLicenseText` 两个绑定已在生成文件 `wailsjs/go/main/App` 里，直接 import，没有本地类型声明。

## 联调时要切换的地方

1. `src/api/flags.ts` 里对应开关改 `true`（三个可以分开切）。
2. 后端生成 wailsjs 绑定后，可选：把 `callService('X','Y',…)` 换成直接 import 生成文件（保留 `call()` 包装），并用生成的类型替换 `live.ts` / `edit.ts` / `doc.ts` 顶部的手写类型。
3. `LiveLayout.vue` 顶部的 `MigrationNotice`（直播演示提示）已改为只在模拟环境（`!liveIsReal()`）显示，真实 Wails 里不出现；剪辑页 `VideoEditor.vue`、直播页、文档页 `OfficeConvert.vue` / `PDFPreview.vue` 均已接真实后端（Wails 内），各页的 `MigrationNotice` 只在模拟环境显示。v1 兼容层 `api/index.ts`、`V1_API_READY`、`api/office`、`api/pdf`、`api/editor` 已全部删除。
4. `services/wails.ts` 的 `onSimEvent` 总线与 `stores/tasks.ts` 里对 `sim` 的分支（cancel / retry / remove / 历史）在全部开关为 true 后可删。

## 契约未冻结、可能要改的点

- Live：方法名、`PushOptions` 字段、`TASK_CONFLICT` 的 `reason` 取值（`duplicate_url` / `screen_busy` / `max_sessions` 已定）、缺 srt/rtmps 协议时 UNSUPPORTED 的 detail 写法与文案、`LIVE_*` 错误分类关键词（未用真实服务器验证）（文案已由产品定稿，见上）。
- Edit：`Export` 命名（已确认）、`EditExportOptions`、`GetPreviewURL` 的限长 206 在 Windows/WebView2 上是否可用（未验证，回退是 `edit_proxy`，接口不变）、clip 错误 detail 首行格式。
- Doc：大文件（> 64 MiB）路径在 Windows 未验证（前端已实现 HEAD 探测 + 一次重试，未在真实 Wails 里跑过）（验证不通过则 `OpenPDF` 对 > 64 MiB 返回 INVALID_ARGUMENT、`url` 恒空）；字体子集范围与 OFL 保留名。

## 契约疑问：已决与未决

### 已决（架构师 5 条决定，后端会写进契约新提交，前端已按此实现）

1. **TaskType 统一**：`convert | edit_export | office_pdf | live_file_push | live_screen_push | ffmpeg_install`。三个旧类型 id（见 `stores/tasks.ts` 的 `isLegacyTaskType`）后端保留但不再产生，任务中心一律忽略、不显示、不报错；旧类型 id 调 `Get` / `Cancel` / `Retry` / `Remove` 一律 `NOT_FOUND`（后端约定）。（原疑问 19）
2. **停止语义**：优雅停止和自然播完都是 `succeeded`，都显示“已结束推流”，不区分；停止中 5 秒内刷新看到 `running` 可接受，不加字段。（原疑问 1、2）
3. **连接失败 / 地址不合法的稳定首行**（原疑问 4、5）：
   - `LIVE_CONNECT_FAILED` 的 detail 第一行固定 `scheme=rtmp|rtmps|srt`。`AppError.scheme` 解析它；文案见下方“产品经理直播错误文案定稿”。脱敏 `params.url` 的 scheme、页面上地址的 scheme 只作首行缺失时的兜底。
   - `LIVE_URL_INVALID` 的 detail 第一行 `reason=`，取值 `scheme_unsupported` / `malformed` / `missing_host` / `param_not_allowed`（稳定枚举，只追加）。文案表 `LIVE_URL_INVALID_REASON_TEXT`，见下方定稿。
4. **`DocCapabilities.experimental`**（布尔，字段名与前端一致）；`ReadPDFChunk` 的 `Data` 按 base64 字符串解码。（原疑问 14、16）
5. **Edit**（原疑问 9、11、12、13）：预览 `/local/<token>` 支持 HEAD，token 失效返回 404，前端 HEAD 探测后重新调用 `GetPreviewURL`（`isPreviewGone` / `createPreviewSource`，已符合）；`SaveProject` 只校验数量上限、**不校验同轨重叠**（草稿可保存），重叠只在 `ValidateProject` 和 `Export` 报；`outSec` 必须大于 `inSec`，**0 不表示到结尾**，`outSec=0` 一律 `INVALID_ARGUMENT`，前端用探测到的时长填实际值（`newVideoClip` / `newAudioClip` / `fillOutSec`，素材时长未知时不能加入时间线）。

### 2026-09-29 补充决定（本轮关掉的疑问）

- **“已经在推”的判定**（原 3，已关）：**“已开始” = 第一次 `progress=continue` 且 `out_time_us>0`**。有存档时没有 `bitrateKbps`，码率列显示“—”，所以不能再用“`bitrateKbps` 有值”当已在推的依据。
- **停止 / 强杀 / 存档**（补充原 1、2，已关）：
  - 优雅停止或自然播完 = `succeeded`（“已结束推流”，不加提示）；强杀 = `canceled`（“已强制停止”）。
  - 有存档时优雅停止最多等 **16 秒**，界面显示“正在停止…”且停止按钮禁用；超时后端强杀。
  - **强杀且存档已保留** = `status=canceled` 且 `outputPath` 非空：显示“已强制停止，存档已保留，文件可能不完整”和“打开所在文件夹”。`canceled` 且 `outputPath` 为空只显示“已强制停止”。
  - 后端会先删空壳存档并清空 `outputPath`，再发终态事件。
- **SRT passphrase 长度 10–79**（新增约束，已定）。
- **`TASK_CONFLICT` 的 `reason`**（原 18，已关）：`max_sessions`=“最多同时推 4 路”，`duplicate_url`=“这个地址已经在推流”，其他 / 缺失 / Edit、Doc 的冲突=“操作冲突，请稍后再试”。屏幕推流“同时最多 1 路”是否限制、reason 叫什么，仍等产品经理（见下）。
- **带存档的屏幕推流**（原 8 的一部分）：后端已实现（#47，tee 分片 mp4），前端已放开，不再返回 `UNSUPPORTED`。
- **Doc**（无对应旧疑问，仅记录）：`PDFChunk.Data` 是 Go `string`（标准 base64，含 `=` 填充），前端直接 `atob`，不做类型转换；`Length` / `chunkBytes` 是原始字节数；超 5000 页返回 `UNSUPPORTED`；Office 转 PDF 页面显示“实验性”标签和常驻说明“仅提取文字，不保留图片和样式”；`/local/<token>` 支持 `HEAD`，失效 404。
- **Edit**（无对应旧疑问，仅记录）：
  - 转场：默认 0.5s 超过相邻较短片段一半时后端静默缩到一半，不足 0.1s 忽略并给 `transition_ignored` 警告；显式超限 `INVALID_ARGUMENT`。`EditPlan.durationSec` 已扣除转场重叠。
  - `SaveProject` 只查数量上限（素材库、100 片段、1 MiB），不查重叠；重叠只由 `ValidateProject` / `Export` 报。
  - `outSec` 必须大于 `inSec`，`outSec=0` 是 `INVALID_ARGUMENT`；小于 0.04s 的片段 `INVALID_ARGUMENT`。
  - 同轨间隙 ≤0.12s 视为相接，更大间隙导出时补黑场 / 静音。
- **task:status 事件的 `startedAt`**（后端 PR #32）：`running` 事件带 `startedAt`，四种终态事件带 `startedAt` 与 `finishedAt`，均为可选（排队中被取消则 `startedAt` 缺省）。前端优先用事件值，没有则沿用本地已记录值，仍没有就不显示“用时”。

### 2026-09-29 架构师 / 产品经理定稿（本轮关掉的疑问）

- **刷新后重连**（原 4 的后半，架构师已决）：直播任务在后端任务管理器里，刷新页面不影响它。前端刷新后重新调 `ListTasks`（`TaskService.List` / `ListActive`）重建状态，再订阅 `task:*` 事件；不做专门的重连逻辑，不需要新接口。
- **`CheckPushURL`**（原 7，架构师已决）：只校验地址格式和协议（`LIVE_URL_INVALID` 的 reason），不做冲突检查。“同地址已在推”“超过上限”都在真正开始推流（`StartFilePush` / `StartScreenPush`）时返回 `TASK_CONFLICT`，界面按 `reason` 显示。
- **`TASK_CONFLICT` reason 与判断顺序**（已统一）：**`duplicate_url` → `screen_busy` → `max_sessions`**。
  - `duplicate_url`：“这个地址已经在推流”
  - `screen_busy`：屏幕推流同一时间最多 1 路（已有进行中的屏幕推流，含排队、正在停止时，`StartScreenPush` 返回）：“屏幕推流同一时间只能有 1 路，请先停止当前的屏幕推流”（映射在 `errors/errorMessages.ts` 的 `TASK_CONFLICT_REASON_TEXT`，模拟在 `api/live.ts`，自检覆盖顺序）
  - `max_sessions`：“最多同时推 4 路”
- **屏幕推流首版**（原 8 的其余部分，架构师已决）：不做区域选择，只推整块屏幕，可选来源以契约为准（`ListScreens`），契约没写的不加。
- **屏幕推流两个错误码文案**（产品经理已定，两者不混用）：`SCREEN_PERMISSION_DENIED`：“没有获得屏幕录制权限，请在系统设置中允许 FFmpegFree 录制屏幕后重试”；`UNSUPPORTED_PLATFORM`：“当前系统暂不支持屏幕推流”（`errorMessages` 里新增了 `UNSUPPORTED_PLATFORM`）。
- **剪辑**（产品经理已定）：默认导出分辨率 **1920×1080**（前端提交时显式写宽高，`newEditProject` / `VideoEditor.vue` 已改；后端兜底值也改为 1920×1080）；素材库上限 **100 个素材文件（sources）**、片段总数上限 100，均已定稿，后端超过返回 `INVALID_ARGUMENT`；前端上限集中在 `api/edit.ts` 的 `MAX_SOURCES`（界面计数 x/100、导入拦截、`checkSaveLimits` 都读它）；一次删除 **≥5 个片段**才二次确认。

### 产品经理直播错误文案定稿（已落地，逐字）

映射都在 `errors/errorMessages.ts`，`api/live.ts` 的模拟层引用同一份；`api.check.ts` 逐条断言，并断言任何文案输出都不含传入的地址 / 口令 / 推流码。

- `LIVE_CONNECT_FAILED`（按 detail 首行 `scheme=`）：rtmp、rtmps、缺 scheme 或未知 → “连接失败，请检查推流地址和推流码是否正确，以及网络是否通畅”；srt → “连接失败，请检查地址和口令是否正确”。
- `LIVE_PUSH_REJECTED`：“服务器拒绝了推流，请检查推流码是否有效，或是否已被其他推流占用”。
- `LIVE_URL_INVALID`（按 `reason=`）：`scheme_unsupported` “暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt”；`malformed` “推流地址格式不正确，请检查后重新输入”；`missing_host` “推流地址里缺少服务器地址，请检查后重新输入”；`param_not_allowed` “推流地址里有不支持的参数，请去掉后重试”；未知 / 缺失 “推流地址不可用，请检查后重新输入”。
- SRT 口令不是 10 到 79 个字符：前端先拦，“SRT 口令需要 10 到 79 个字符”，不发给后端。校验在 `api/live.ts` 的 `isValidSrtPassphrase` / `assertSrtPassphrase`（两个 Start* 调后端和模拟之前先调；空口令 = 不加密，放行；按字符数算）。抛 `INVALID_ARGUMENT`，detail 首行 `reason=srt_passphrase_length`，message 是产品文案。页面目前没有单独的口令输入框（口令在地址的 `passphrase=` 参数里），页面把它当普通 INVALID_ARGUMENT 显示 message。
- 缺协议（UNSUPPORTED）：有具体协议名 “当前的 ffmpeg 不支持 SRT，请在设置的 ffmpeg 页面重新安装或更新”（协议名替换）；没有 “当前 ffmpeg 不支持这种推流协议，请在设置的 ffmpeg 页面重新安装或更新”。判断依据是契约 §6.10：detail 单独一行 `missing=<协议名>`（`rtmp` / `rtmps` / `srt`），严格匹配才带协议名，`missing=tee` 等其余情况用通用句（详见下方“缺协议的 UNSUPPORTED”条目，契约已冻结）。
- 已确认在映射里：`TASK_CONFLICT`（max_sessions / duplicate_url / screen_busy / 其他），`SCREEN_PERMISSION_DENIED`，`UNSUPPORTED_PLATFORM`（文案见上一节）。

### 仍未决

等**后端 / 架构师**：

- **屏幕推流本地存档**：后端已实现（#47），前端存档开关可用。`errorMessages.ts` 里仍保留 `LIVE_ARCHIVE_UNSUPPORTED_TEXT` 分支（archiveDir 非空且 UNSUPPORTED 且 detail 没有 `missing=` 行时用），作为旧后端的兜底；`api/live.ts` 的模拟层仍按旧行为对 archiveDir 抛 `UNSUPPORTED`，浏览器演示环境下如此，待直播页跟进时对齐。
- **缺协议的 UNSUPPORTED**（原 6，**契约已冻结，见契约 §6.10 与 2.2 的 detail 约定表**）：本机 ffmpeg 缺推流协议时 `Start*` 返回 `UNSUPPORTED`，`detail` 是**单独一行** `missing=<协议名>`，协议名只取 `rtmp` / `rtmps` / `srt`（对应地址 scheme，`rtmp` 是除 `rtmps` / `srt` 以外的默认）；带本地存档的会话另需 tee，缺时是 `missing=tee`；`CheckPushURL` 不返回它；`message` 是“当前 ffmpeg 不支持 <协议名>，请安装完整版 ffmpeg”（前端不显示）。其他原因的 `UNSUPPORTED`（`Retry` 直播任务、屏幕推流存档未实现）没有这一行。前端按契约严格识别（`liveMissingProtocolName` / `liveFfmpegProtocolMissingText`）：detail 按行拆开，某一行**严格等于** `missing=rtmp` / `missing=rtmps` / `missing=srt` 才显示协议名（大写）；其余一律用不带协议名的通用句，包括 `missing=tee`、大小写 / 空格不同、别的写法。`hasMissingLine` 判断有没有 `missing=` 行，用来把“缺组件的 UNSUPPORTED”和“存档未实现的 UNSUPPORTED”分开。模拟层 `?sim_missing=rtmp|rtmps|srt` 产出的 detail 就是这一行。
- **Edit 多素材预览**（原 10）：同时预览 N 个素材占 N 个 token（登记表 256 项 LRU），是否提供批量 `GetPreviewURL`。限长 206（4 MiB）的 seek 体验待 Windows 真机验证。
- **Doc 转换产物不自动进 PDF 历史**（原 15，已按契约实现）：转换记录里点“预览”才对输出路径 `OpenPDF`（此时才进“最近打开”）。
- **错误码表**（原 17）：`UNSUPPORTED_PLATFORM` 文案已定（见上）；`LIVE_PLAY_FAILED` / `LIVE_CORS_BLOCKED` 只由前端播放器产生。契约 §2 的清单是 17 个后端码。
- **敏感信息**（原 20）：前端已保证完整推流地址和口令只在输入框和调用参数里，不写 localStorage / 日志 / console，列表和标题用脱敏形式；后端 `Task.title` / `params` 已脱敏。无需契约改动，仅记录。

（关于页三项已由产品经理确认：项目地址用 GitHub、许可证句“本应用以木兰宽松许可证第 2 版发布”、许可证链接指向 `blob/master/LICENSE`，常量在 `src/config/about.ts`。）


### 文档页（2026-09-29 落地，设计稿待补）

- 布局按旧稿 `07-文档-浅色.png` 与现有设计令牌：Office 转 PDF = 标题 +“实验性”标签 + 常驻说明 12px `--ff-text-2`“仅提取文字，不保留图片和样式”、拖入区、转换记录；PDF 预览 = 缩略图 + 阅读区 + 最近打开。完整稿（含暗色、标签与说明）由设计师稍后补，出来后走查差异。
- **待产品经理确认的自拟文案**（`errors/errorMessages.ts`，已标注）：超过 5000 页 `DOC_TOO_MANY_PAGES_TEXT`、文件损坏（INVALID_ARGUMENT + OOXML）`DOC_FILE_BROKEN_TEXT`。
- 未在真实 Wails 里验证：`atob` 解码后字节与 `length` 一致（前端已校验，不一致抛 INTERNAL）；> 64 MiB 的 `/local/<token>` Range 加载与 HEAD 探测；真实 Office 转换与超 5000 页；加密 PDF 的 `onPassword` 弹出；拖入（`OnFileDrop`）落在文档页；刷新后 `ListActive` / `List` 接回进度。

### ffmpeg 安装完成后重复弹"需要安装"（2026-09-29 修复）

- 根因在前端：`stores/ffmpeg.ts` 的 `dialogOpen` 在点"下载"后一直为 true，安装完成收到 `ffmpeg:status(ready)` 时没有复位；安装对话框 `v-if` 只看 `dialogOpen`，`installing` 视图消失后就退回"需要安装 ffmpeg / 下载"视图。现在 ready 时复位 `dialogOpen`，对话框改用 `dialogVisible = dialogOpen && needsAttention`；`startInstall` / `pickPath` / `clearCustomPath` 的返回值也按事件序号丢弃过期结果。
- 自检：`npm run check:ffmpeg`（`src/stores/ffmpeg.check.ts`，假的 `window.go` 驱动真实 store）。
