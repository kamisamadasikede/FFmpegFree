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
| `ENCODER_BACKEND_READY`（**`true`**） | `encoder.ts` | `SystemService.ListEncoderDevices()` / `RefreshEncoderDevices()` / `GetEncoderPreference()` / `GetEncoderPreferenceInfo()` / `SetEncoderPreference(id)`（经 `callService`，后端 #60 已合入，绑定在 `wailsjs/go/app/SystemService`）。**已打开**（后端第二个 PR #67 / #68 与前端回退提示接线 #69 都已合入）：Wails 里设置页显示“编码设备”一块并调用真实绑定；纯浏览器（没有 `window.go`）默认不显示，只有地址带 `?enc=` 才显示模拟层（仅开发用）。应急回滚：改回 `false`，设置页和任务里的编码设备界面整体消失 |

开关为 true 时经 `call.ts` 的 `callService(service, method, ...args)` 按名字取 `window.go`，**不 import wailsjs 生成文件**（没有绑定时 `vue-tsc` / `vite build` 也能过）。绑定不存在会抛 `UNSUPPORTED`，不会悄悄走模拟。

## 方法清单

**Live**（`live.ts`）：`startFilePush(FilePushRequest)→Task`、`startScreenPush(ScreenPushRequest)→Task`、`getCaptureCapabilities()`、`listScreens()`、`listCaptureSources()`（v0.14，屏幕 + Windows 窗口；`ScreenPushRequest.captureSourceId` 可选，失效 → `LIVE_SOURCE_GONE`，`?sim_source_gone=1` 让「演示文稿」窗口在列表被拉过一次之后消失（页面选中它 → 开始 → LIVE_SOURCE_GONE → 自动刷新后它不在了），`?sim_sources=loading|fail|empty|screens` 模拟加载中 / 失败 / 空 / 仅屏幕，`?sim_err=LIVE_SOURCE_GONE&sim_reason=window|screen` 注入）、`checkPushURL(url)→PushURLInfo`；停止 `stopPush(taskId)`（= `TaskService.Cancel`）；`listRunning()`（`TaskService.ListActive` 里的 live_*）；`watchLiveTask(id, handlers)`（`task:progress` / `task:status`）；素材 `pickMaterial()`（`SystemService.PickFiles` + `MediaService.Probe`）。任务类型 `live_file_push` / `live_screen_push`，Task 新增 `fps` / `bitrateKbps` / `droppedFrames`。

**Edit**（`edit.ts`）：`validateProject`、`exportProject(EditProject, EditExportOptions)→Task`（契约名 `Export`）、`getPreviewURL(path)`、`saveProject`、`loadProject`、`listProjects(limit)`、`deleteProject`；辅助 `createPreviewSource`（404 后 HEAD 探测、重新取地址）、`findTrackOverlap` / `wouldOverlap`（拖拽 / 放置时拦同轨重叠）、`newVideoClip` / `newAudioClip` / `fillOutSec`（素材加入 clip 时用探测到的时长填 `outSec`，不能是 0）、`checkStructure`（Validate / Export）、`checkSaveLimits`（Save，只查数量上限）、`sanitizeOutputName`。素材用 `system.ts` 的 `pickFiles` 和 `media.ts` 的 `probeFiles` / `thumbnailOf`。任务类型 `edit_export`。

**Doc**（`doc.ts`）：`getDocCapabilities`、`convertToPDF(inputs, outputDir)→Task[]`（`office_pdf`）、`openPDF(path)→PDFSource`、`readPDFChunk(id, offset, length)`、`readWholePDF(src)`（循环读到 eof，按原始字节处理）、`loadPDF(path)`（≤ 64 MiB 读整份；更大的返回 `/local/<token>`，先 `HEAD` 探测，404 重新 `OpenPDF` 只重试一次）、`listRecentPDFs(limit)`（limit > 200 按 200）、`removeRecentPDFs(ids)`、`listOfficeHistory()`（`TaskService.List` 的 `office_pdf` 终态任务，刷新后接回）；`isExperimental(caps)`。文档页错误文案统一走 `errors/errorMessages.ts` 的 `docErrorText`。

**编码设备**（`encoder.ts`，字段以后端 #60 的真实绑定 / 契约 v0.15 §9.6 为准）：`listEncoderDevices()` / `refreshEncoderDevices()` → `{ffmpegReady, devices[]}`（`RefreshEncoderDevices` 强制重测；设置页“重新检测”走它），设备 `id` / `name` / `vendor`（nvidia|intel|amd|apple|unknown）/ `kind`（gpu|cpu）/ `discrete`（独显）/ `available` / 可选 `reason`，第一项永远是 `id="cpu"`；ffmpeg 未就绪时只有 cpu 且 `ffmpegReady=false`（不报错）。**前端排序**（`sortDevices`，在 `normalizeList` 里）：cpu 第一，显卡里独显在前、集显在后，同级保持后端顺序，与后端 `ResolveEncoder` 的 auto 选择一致（下拉第一张可用显卡 = 自动会选的那张）。后端返回的 `encoders{h264,hevc}`（内部编码器名）在 `normalizeList` 里丢弃，界面不显示 NVENC / QSV 等（自检锁定）。`getEncoderPreference()` → `auto` | `cpu` | 设备 id；`getEncoderPreferenceInfo()` → `{id, name, available, reason?}`（设置页“当前选择”据此显示：自动 / CPU / 具体显卡名；所选设备不可用或已不存在时 `name` 仍是保存偏好时记下的名字，后端在列表**末尾**追加一项 `available=false` 占位）；`setEncoderPreference(id)`（非法或不在当前列表 → `INVALID_ARGUMENT`，不改原值；存在但不可用的允许保存），成功后面板重读 `getEncoderPreferenceInfo()`；重新检测 / 选择用事件序号（`createSeq`，沿用 ffmpeg store 的做法）丢弃过期返回。状态推导在 `encoderView.ts`，面板 `components/encoder/EncoderDevicePanel.vue`（挂在设置页 ffmpeg 面板之后，不新增左侧分类），文案集中在 `errors/encoderMessages.ts`（**全部待产品经理确认**）。

**编码设备显示规则**：`ENCODER_BACKEND_READY=true`：Wails 里显示整块并走真实绑定，不出现模拟显卡名；`?enc=` 只在纯浏览器（没有 window.go）里有效（Wails 里无效）；纯浏览器没有 `?enc=` 时不显示。标志为 false 时（应急回滚）正式包不渲染整块。浏览器演示用 `?enc=<场景>`：`found`（默认）| `found-open` | `found-gpu` | `multi` / `multi-open`（多显卡：Intel 集显故意排在后端顺序前面，验证前端把独显排前）| `none` | `none-open` | `unavail` | `fail` | `noff` | `detecting`；`&fb=1` 额外显示“回退提示”三种展示的预览（仅展示）。

**编码设备 · 回退提示与设备名（契约 v0.18 §9.7 / v0.19，已接线；`ENCODER_BACKEND_READY=true` 后 Wails 里生效，为 false 时整体不显示）**：
- 字段：`Task` / `task:progress` / `task:status` 的 `encoder`（不展示）、`encoderDevice`、`hwFallback`、`hwFallbackReason`，全部 omitempty。合并在 `api/encoderTask.ts` 的 `mergeEncoderFields`：**逐字段、缺省不覆盖**（`hwFallback` 只有 true 才写，后端 false 不会出现在事件里）；回退时后端补发的 `running` status 带全四个字段，据此更新。终态快照（`FinalState`）也带这四个字段，转换页 / 剪辑导出结果处要用。
- 显示规则（`encoderTask.ts`，自检在 `api.check.ts`）：只有 `startedAt > 0`（真的运行过）才显示回退提示和“使用的设备”——契约 v0.19：从未运行的任务（排队中取消、退出时还在排队）四字段仍是提交时解析的值，不能据此显示；`hwFallback=true` 才显示提示，`-c copy`（`encoder="copy"`）、两遍编码、VP9/GIF、宽或高超 4096 走 CPU 后端都不置 `hwFallback`，前端不猜。
- 设备名：`encoderDevice` 在 `ListEncoderDevices` 里查 `name`；`cpu` → “CPU”；查不到 → “显卡”；**不显示 id，不显示 `encoder`，界面任何位置不出现编码器名**。
- 接入点：转换页（`ConvertPage.vue` 进度面板上方的 `variant="convert"` 提示条，整批一条；进度面板 / 完成条里的“编码设备 xxx”）、剪辑导出（`ExportStrip.vue` 的提示条 + “编码设备 xxx”，文案 `ENCODER_FALLBACK_EXPORT`）、任务中心（该行下方 `variant="row"` 一行，“查看日志”打开日志面板；日志面板头显示设备名，回退时加 `hwFallbackReason` 对应的一句次要说明，未知枚举用通用兜底句）。文案在 `errors/encoderMessages.ts`（**待产品经理确认**），枚举与契约 9.7、后端 `hwenc.go` 一一对应。
- **直播变体（`variant="live"`，`views/live/*`）暂未接线**：`v2-fe-live-preview` 分支另有人在改 `views/live/*`，等预览合入后再接（数据已就绪：直播任务的 `hwFallback` 在 store 里，直播只在第一条 progress 之前回退；任务中心行里直播任务的提示已用直播文案）。
- 开发预览：`?enc=gpu-task | fb-nvenc | fb-unavail | fb-unknown | gpu-gone | copy-task | cpu-task`（仅纯浏览器，配合 `?convert=running` / `?tasks=5` / 剪辑导出）；设备列表场景（`found` / `multi` / …）不改任务。

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

- **屏幕推流本地存档**（后端 #47 已实现，前端已放开）：`archiveDir` 非空时任务的 `outputPath` = 存档路径（分片 mp4）；有存档的会话 `task:progress` 不带 `bitrateKbps`（码率栏显示“—”）；优雅停止最多等 16 秒（页面显示“正在停止…”，不做超时处理）；强杀且存档保留 = `status=canceled` 且 `outputPath` 非空 → “已强制停止，存档已保留，文件可能不完整” + “打开所在文件夹”；`watchLiveTask.onEnd` 现在带终态的 `outputPath`。`UNSUPPORTED` 一律按缺 ffmpeg 组件处理（`missing=tee` 用通用句）；`LIVE_ARCHIVE_UNSUPPORTED_TEXT` 已删除。模拟层同步（存档路径、无 bitrateKbps、强杀保留、空壳清空）。
- **缺协议的 UNSUPPORTED**（原 6，**契约已冻结，见契约 §6.10 与 2.2 的 detail 约定表**）：本机 ffmpeg 缺推流协议时 `Start*` 返回 `UNSUPPORTED`，`detail` 是**单独一行** `missing=<协议名>`，协议名只取 `rtmp` / `rtmps` / `srt`（对应地址 scheme，`rtmp` 是除 `rtmps` / `srt` 以外的默认）；带本地存档的会话另需 tee，缺时是 `missing=tee`；`CheckPushURL` 不返回它；`message` 是“当前 ffmpeg 不支持 <协议名>，请安装完整版 ffmpeg”（前端不显示）。其他原因的 `UNSUPPORTED`（`Retry` 直播任务、屏幕推流存档未实现）没有这一行。前端按契约严格识别（`liveMissingProtocolName` / `liveFfmpegProtocolMissingText`）：detail 按行拆开，某一行**严格等于** `missing=rtmp` / `missing=rtmps` / `missing=srt` 才显示协议名（大写）；其余一律用不带协议名的通用句，包括 `missing=tee`、大小写 / 空格不同、别的写法。`hasMissingLine` 判断有没有 `missing=` 行。模拟层 `?sim_missing=rtmp|rtmps|srt` 产出的 detail 就是这一行。
- **Edit 多素材预览**（原 10）：同时预览 N 个素材占 N 个 token（登记表 256 项 LRU），是否提供批量 `GetPreviewURL`。限长 206（4 MiB）的 seek 体验待 Windows 真机验证。
- **Doc 转换产物不自动进 PDF 历史**（原 15，已按契约实现）：转换记录里点“预览”才对输出路径 `OpenPDF`（此时才进“最近打开”）。
- **错误码表**（原 17）：`UNSUPPORTED_PLATFORM` 文案已定（见上）；`LIVE_PLAY_FAILED` / `LIVE_CORS_BLOCKED` 只由前端播放器产生。契约 §2 的清单是 17 个后端码。
- **敏感信息**（原 20）：前端已保证完整推流地址和口令只在输入框和调用参数里，不写 localStorage / 日志 / console，列表和标题用脱敏形式；后端 `Task.title` / `params` 已脱敏。无需契约改动，仅记录。

（关于页三项已由产品经理确认：项目地址用 GitHub、许可证句“本应用以木兰宽松许可证第 2 版发布”、许可证链接指向 `blob/master/LICENSE`，常量在 `src/config/about.ts`。）


### 文档页（2026-09-30 对齐设计师完整稿 v0.1）

- 结构：`views/docs/DocsLayout.vue`（分段控件 240×28 + `KeepAlive` + 两个 Tab 共用的右侧「最近打开的 PDF」`components/docs/RecentPanel.vue`，320 / ≤1199px 304）；`stores/docs.ts` 放共用的最近列表；Wails 只有一个 `OnFileDrop` 入口，由 DocsLayout 注册一次，按当前 Tab 分发（各页在 activated / deactivated 时登记 / 撤销）。纯函数（中间省略、缩放档位、缩略图窗口、时间格式）在 `utils/docLogic.ts`，有 `check:api` 断言。
- **Doc 错误的 `reason=`（契约 v0.16，2.2 / 6.12.6）**：后端已让 Doc 的“文件本身有问题”类错误 detail 首行严格是 `reason=<枚举>`（`too_many_pages` / `format` / `encrypted` / `no_font` / `invalid_ooxml` / `too_large`），`code` 和 `message` 不变；`ConvertToPDF` 整体校验失败时第二行才是出错文件路径。`call.ts` 已提供类型 `DocErrorReason`、常量 `DOC_ERROR_REASONS` 和 `docErrorReason(err.reason)`（`AppError.reason` 本来就会解析首行）。**页面和 `errors/errorMessages.ts` 已切换到 `reason`**（2026-09-30，后端 #63 / 契约 v0.16）：判断顺序是先看 `reason`，reason 缺失（老后端 / 未列入的错误）才退回下面那条的 message 精确相等 / detail 首行匹配作兜底；`reason` 与 `message` 不一致时听 `reason`。映射：`too_many_pages`→5000 页文案，`format`→不支持的格式，`encrypted`→加密的 Office 文档，`no_font`→缺字体，`invalid_ooxml`→文件已损坏，`too_large`→文件太大；`OpenPDF` 的 `format`（不是有效的 PDF）、`too_large`（超 512 MiB，按 limits 拼）同样按 reason。`docErrorPath` 现在在前两行里找路径，跳过 `reason=` 行。无 reason 的错误（取消、磁盘满、读写失败、`NOT_FOUND`、路径参数类、`INTERNAL`）保持原处理；任务中心里超页数和缺字体仍沿用后端 message。重试按钮显隐没改。模拟层（`api/doc.ts`）的错误已改成 v0.16 形态（`reason=…\n路径\n说明`），`check:api` 有断言。
- **Doc 错误判断不再用正则匹配 message**。后端真实取值（`internal/service/doc`）：契约 2.2 的“detail 第一行”稳定枚举里**没有** Doc 的码；`ConvertToPDF` 整体校验失败时 detail 第一行是出错文件路径（`withPath`），运行时任务失败的 detail 没有路径行。超过 5000 页 = `UNSUPPORTED` + **message** `超过 5000 页`（detail 是 `已排到第 N 页仍未结束` / `文档文字量超过上限`；契约 6.12.3 写成“detail『超过 5000 页』”，与实现不一致）；损坏 = `INVALID_ARGUMENT` + message `不是有效的 OOXML 文件`（detail 是 zip 库错误等自由文本，不稳定）。所以前端只做“错误码 + message 精确相等 / detail 首行（先剥掉路径行）精确匹配”，认不出的一律回落后端 message 或通用文案。（架构师已按此冻结 `reason` 枚举并更正 6.12.3，见上一条；以上是 reason 缺失时的兜底依据。）
- **待产品经理确认的自拟文案**（`errors/errorMessages.ts`，已标注）：`DOC_TOO_MANY_PAGES_TEXT`「文档太长，超过 5000 页，无法转换」（页数取 `limits.maxPages` 拼）、`DOC_FILE_BROKEN_TEXT`「这个文件已损坏，或不是有效的 Word、Excel、PowerPoint 文档」。设计说明和截图 127/147 里是另一版措辞（「文档超过 5000 页，暂不支持转换」「文件已损坏或格式不正确，无法转换」），设计师倾向保持前端现有两句。新增三句（reason 补齐，同样待确认）：`DOC_ENCRYPTED_TEXT`「这是加密的 Office 文档（或旧版格式改了扩展名），暂不支持，请先去掉密码，或另存为 docx、xlsx、pptx」、`DOC_NO_FONT_TEXT`「没有可用的字体，暂时无法转换这个文档」、`DOC_TOO_LARGE_TEXT`「这个文件太大，无法转换」。另有 `DOC_BATCH_INVALID_HINT`、PDF 预览失败卡片 / 密码卡片文案（设计说明 5.3，均待确认），都集中在该文件。
- **最近列表（架构师已定）**：标题「最近打开的 PDF」（常量 `DOC_RECENT_TITLE`）。`doc_recent` 只由 `OpenPDF` 写入，前端不在转换成功后自动调 `OpenPDF`，转换产物不进列表；点「打开 PDF」/ 预览时才 `OpenPDF`。待转换行不显示文件大小，只显示“等待开始”，不加接口。
- 设计有、后端 / 前端拿不到所以**没做**的：待转换行的文件大小（前端没有 stat 接口，只显示“等待开始”）；已完成行的“PDF 86 KB”（任务没有产物大小，且不为取大小去调 `OpenPDF`）；磁盘满的“更换输出位置”按钮（设计稿没画）；PDF 全屏演示模式（契约没有）；100 MiB 大小预检（拿不到文件大小，由后端校验）。
- 取保守方案的第 8 节未决项：4 失败行按设计显示「重试」（含 UNSUPPORTED / INVALID_ARGUMENT）；5 已中断行单独画；6 `experimental=false` 时标签和常驻说明一起隐藏；8 缩放 50%~200% 步进 25%、不记忆、不做适合宽度；10 不支持的扩展名先加入列表，提交时整体校验并标红（.txt/.csv 是否拒收待产品）；13 文件名尾部固定 6 个字符 + 扩展名。
- 模拟环境预览参数（真实环境不读）：`sim_seed=pending|invalid|invalid2|running|done|failed|canceled|all`（Office 页预置各状态行）、`sim_hover=1`（拖入悬停）、`sim_recent=sample|full|none`（最近列表 8 条示例 / 200 条 / 空）、`sim_view=loading|pass|wrong`（PDF 页加载中 / 需要密码 / 密码错误）；文件名 `多页…` = 12 页，`解析失败…`、`无权限…`、`被修改…`、`非pdf…`、`超大…`、`缺失…` 触发对应失败卡片。
- 未在真实 Wails 里验证：`atob` 解码后字节与 `length` 一致；> 64 MiB 的 `/local/<token>` Range 加载与 HEAD 探测、以及此时用 pdf.js `onProgress` 显示读取进度；真实 Office 转换与超 5000 页；加密 PDF 的 `onPassword`（NEED_PASSWORD=1 / INCORRECT_PASSWORD=2）弹出；拖入（`OnFileDrop`）在 KeepAlive 下按 Tab 分发；刷新后 `ListActive` / `List` 接回进度；5000 页 PDF 上缩略图窗口化的表现；文件选择过滤器。

### 侧栏 ffmpeg 状态对齐设计稿（2026-09-30）

- 按《编码设备-设计说明-v0.1》第一节：整行 32px、内边距 8px 12px、8px 圆点 + 13px 文字、间距 8px；已就绪 `--ff-success` 点 + `--ff-text-2`；未就绪 `--ff-warning` 点 + `--ff-warning-text`，整行是 `<button>`，点开安装对话框（#53 的 `dialogVisible` 逻辑没动）；安装中 12px 转圈（`--ff-primary`，减少动效下不转）、不可点；外层 `role="status"` + `aria-label`；折叠只留圆点，hover / 键盘聚焦显示气泡（12px、`--ff-bg-elevated`、`--ff-shadow-dialog`）。
- 设计师定稿的两处调整（2026-09-30）：启动检测中 = 中性灰点 + “ffmpeg 检测中…”，不可点，`role="status"`；安装中整行可点，点开就是安装对话框（里面有进度，不跳任务中心），`aria-label` / `title` 为“ffmpeg 安装中，点击查看进度”，有 hover 和焦点环，折叠时圆点（转圈）加气泡，气泡文字同 `aria-label`。映射抽在 `components/ffmpeg/statusView.ts`，`check:api` 有文案 / 可点性断言。折叠时侧栏 `overflow: visible`，气泡才能显示在侧栏外。

### ffmpeg 安装完成后重复弹"需要安装"（2026-09-29 修复）

- 根因在前端：`stores/ffmpeg.ts` 的 `dialogOpen` 在点"下载"后一直为 true，安装完成收到 `ffmpeg:status(ready)` 时没有复位；安装对话框 `v-if` 只看 `dialogOpen`，`installing` 视图消失后就退回"需要安装 ffmpeg / 下载"视图。现在 ready 时复位 `dialogOpen`，对话框改用 `dialogVisible = dialogOpen && needsAttention`；`startInstall` / `pickPath` / `clearCustomPath` 的返回值也按事件序号丢弃过期结果。
- 自检：`npm run check:ffmpeg`（`src/stores/ffmpeg.check.ts`，假的 `window.go` 驱动真实 store）。

### 直播 v1.1 采集来源选择器（2026-09-30，设计稿未出，按 v0.2 风格先做）

`views/live/RecordPush.vue` 用 `listCaptureSources()` 取代 `listScreens()`，组件 `components/live/CaptureSourcePicker.vue`（“屏幕 / 应用窗口”两个分组，仅屏幕时不出“应用窗口”标题；加载中 / 空 / 失败三态 + “刷新列表”；名称过长省略，title 带完整名称和分辨率）。默认选第一个屏幕；开始推流传 `captureSourceId`（`screenId` 传空，后端以 captureSourceId 为准）。`LIVE_SOURCE_GONE`（detail 首行 `kind=window|screen`）→ 来源选择器下方显示错误（`liveSourceGoneText`），取消已选来源并自动刷新列表；`INVALID_ARGUMENT` 沿用通用文案。会话行显示这一路的来源名（`LiveRow.source`），不脱敏但只在本机界面显示，不写日志、不进错误 detail；刷新后接回的会话取不到来源名。文案集中在 `errors/errorMessages.ts`（`LIVE_SOURCE_*`，**全部待产品经理确认**）。预览参数：`?sim_sources=…`、`?form=window|srcgone|srcgonescreen`、`?src=window`（会话行窗口来源）。预览见下一节。

## 直播预览（后端 PR #65，契约 v0.17；前端 `v2-fe-live-preview`）

**接口**（`api/live.ts`）：`getPreview(sessionId)`（`{data: base64 JPEG, ts: 毫秒, active}`；没有画面返回空、不是错误；`active=false` = 会话已结束）、`startPullPreview({url, preview?})→{id, redacted, preview}`（同地址幂等，最多 4 路，超限 `TASK_CONFLICT` reason=max_pull_previews）、`stopPullPreview(id)`；`FilePushRequest` / `ScreenPushRequest` 有可选 `preview`（缺省开）。

**开关语义（产品经理已定）**：预览是**会话启动参数**，只能在开始前选。推流表单“开始推流”上方是“开启预览”开关（默认开，**不记住上次选择**，每次打开表单默认开；ffmpeg 未就绪、正在开始推流时和“开始推流”一起置灰）；会话行第二行只读“预览：开 / 关”；拉流的开关在“拉流设置”地址框下方，播放中置灰，控制条只剩只读“预览：开”。`preview:false` 原样透传给后端。

**取帧规则**（`api/livePreviewPoller.ts`，不依赖 Vue，可注入假时钟）：约 500ms 一次、请求不重叠、事件序号丢弃过期返回；空帧只算“还没画面”；首帧 10 秒仍没有转失败；连续 5 次取帧出错转失败（成功一次清零）；点“重试”重新计时 / 计数；`active=false` 转“会话已结束”（末帧灰化）；页面不可见 / 切页 / 卸载停止，重新可见立即取一次；预览关闭的会话不轮询。画面用 `data:image/jpeg;base64,` 直接显示，不建 blob。
**拉流预览会话**（`api/pullPreviewSession.ts`）：start 成功后一定 stop（出错 / 点停止 / 离开页面 / 卸载 / start 在途被取消 / 后端返回 `preview=false`），重复开始不重复 Start。KeepAlive 切走会 Stop，回来仍在播放则重开。ws / wss 地址后端不支持预览，舞台显示“不支持”提示（自拟文案）。
**多路会话**：预览面板只显示一路（当前预览行）；其他预览为开的运行中行显示“查看预览”，预览为关的行不显示，被选中的关闭行显示“当前选中”。
**文案**集中在 `errors/livePreviewMessages.ts`（“已定”与“待产品经理确认”在文件里逐条标注）；主屏统一为“屏幕 1（主显示器）”（后端 `ListScreens` 名称、模拟数据、契约同步）。
**浏览器演示参数**（只在没有 window.go 时有效）：`?preview=ok|slow|never|error|flaky`（模拟 JPEG 帧 / 5 秒后首帧 / 一直没画面 / 每次出错 / 偶尔出错）；`?rows=…&rowspv=1,0,1`（逐行“预览：开/关”）、`&pvsel=N`（选中第 N 行为当前预览行）；拉流页 `?live=running&pvon=0`（预览关）。不加 `?preview` 时模拟层恒返回空帧（不假装有画面）。

