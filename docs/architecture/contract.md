# FFmpegFree v2 接口契约（v0.19）

v0.19 变更（只补契约文字，**接口、字段、错误码、行为都没有变**）：① 6.6 新增 `task.NeverRanner`（任务从未真正开始执行就结束）的说明，写清触发场景、状态 / 事件 / 字段表现，以及与 v0.18 编码器字段的关系——**这类任务的 `encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 保留提交时写入的值，不会为空**（见 6.6 与 9.7）；② 第 6 节 `tasks` 表列清单补上 v0.18 迁移 `0004` 新增的四列（`encoder`、`encoder_device`、`hw_fallback`、`hw_fallback_reason`，v0.18 漏写）；③ 9.7 补一句：`hwFallbackReason` 枚举在 Go 常量（`internal/ffmpeg/hwenc.go`）、本契约、前端 `taskTypes.ts` 三处一致，并新增测试锁住这个一致性；前端 `errors/encoderMessages.ts` 现有回退文案按功能区分（转换 / 直播 / 任务行），不按原因区分。

v0.18 变更（硬件编码接入 ConvertService / EditService / LiveService，见新增的 9.7；**契约按架构师口头方案起草，如有出入以架构师为准**）：`Task` 和 `task:progress` / `task:status` 事件新增四个可选字段 `encoder`（string）、`encoderDevice`（string）、`hwFallback`（bool）、`hwFallbackReason`（string），全部 `omitempty`，没有视频编码的任务不带；`tasks` 表新增迁移 `0004_task_encoder.sql`（四列，旧行为空）；`ResolveEncoder` 的结果现在真正用于转换 / 剪辑导出 / 直播的 H.264、H.265 重编码（NVENC / QSV / AMF / VideoToolbox），`-c copy`、VP9 / GIF / 音频、按目标大小的两遍编码一律 CPU；硬件编码启动失败自动用 CPU 重试一次（`hwFallback`），取消不回退，直播只在推流建立前回退；**没有新增接口方法、没有新增错误码**；`Task.params` 不变。9.6 末段“本版不接入”作废。

v0.17 变更（LiveService 推流 / 拉流真实预览画面，见第 4 节 LiveService 和 6.10「预览画面」）：新增 `LiveService.GetPreview(sessionId) (Preview, error)`（最新一帧 base64 JPEG + 毫秒时间戳，没有画面返回空、不是错误）、`StartPullPreview(PullPreviewRequest) (PullSession, error)` / `StopPullPreview(sessionId) error`（拉流预览会话：后端 ffmpeg 读远端流只出预览，播放仍由前端播放器直接拉地址）；`FilePushRequest` / `ScreenPushRequest` 新增可选字段 `preview`（`*bool`，缺省 = true，false = 不加预览输出）；预览输出是主输出之外**独立**的一路 image2 输出（`fps=2,scale=640:-2`、`-q:v 5`、`-update 1`、`-atomic_writing 1`），不放进 tee；临时文件放 `<数据目录>/tmp/live-preview/<会话 id>.jpg`，会话结束清理，应用启动清空该目录。新增类型 `Preview`、`PullPreviewRequest`、`PullSession`；不新增错误码、不新增事件。**有硬字幕 / 视频复制（`-c copy`）的推流场景预览输出需要单独解码（额外占少量 CPU）**；当前直播主输出始终重编码，预览输出复用同一路解码结果不增加解码次数，见 6.10「预览画面」。

v0.16 变更（DocService 错误 detail 首行统一为 `reason=`，并更正 6.12.3 的措辞，只改契约的说明，接口和错误码不变，见 2.2 和 6.12.6）：**架构师定**：Doc（Office 转 PDF、PDF 预览 / 打开）面向前端的“文件本身有问题”类错误，`detail` 首行严格是 `reason=<枚举>`，枚举固定为 `too_many_pages`、`format`、`encrypted`、`no_font`、`invalid_ooxml`、`too_large`（只追加），其后可以保留原来的自由文本行（`ConvertToPDF` 整体校验失败时，出错文件路径在 reason 行**之后**，即第二行）；**`code` 和 `message` 不变**（`message` 是给用户看的短句，前端精确匹配它，精确文案列在 6.12.6 的表里）。**更正**：6.12.3 原文写 detail "超过 5000 页"，实现里"超过 5000 页"是 `message`，detail 首行是 `reason=too_many_pages`；同类的 "不是有效的 OOXML 文件" "暂不支持这种格式" "没有可用的 Unicode 字体" 也都是 `message`，不是 detail。取消、磁盘满、读写失败、路径 / 参数 / 句柄类错误**没有 reason**（6.12.6 明确列出）。实现：`internal/service/doc` 的 `reasonErr`；2.2 新增 Doc 行。

v0.15 变更（SystemService 硬件编码器检测与偏好，见第 4 节 SystemService 和 9.6；**契约按架构师口头方案起草，如有出入以架构师为准**）：新增 `ListEncoderDevices()`（返回 `EncoderDeviceList{ffmpegReady, devices[]}`，第一项永远是 CPU）、`RefreshEncoderDevices()`、`GetEncoderPreference()`（`"auto" | "cpu" | 设备 id`，默认 `"auto"`）、`GetEncoderPreferenceInfo()`（`{id, name, available, reason?}`，设置页显示「自动 / CPU / 具体显卡名」用）、`SetEncoderPreference(id)`；新增设置键 `encoderPreference`、`encoderPreferenceName`；新增纯函数 `ResolveEncoder(pref, devices, codec)`（Go 内部，不是绑定方法）。检测 = `ffmpeg -encoders` + 逐个硬件编码器实际试跑一帧（5 秒超时）+ 显卡名称枚举；结果缓存，ffmpeg 变为 ready 时失效。**本版只做检测、偏好和解析函数，转换 / 剪辑 / 直播的编码参数暂不使用它（下一版接入）**。无新增错误码。

v0.14 变更（LiveService 屏幕推流可选采集来源，见第 4 节 LiveService 和 6.10「采集来源」）：新增 `LiveService.ListCaptureSources() ([]CaptureSource, error)`；`ScreenPushRequest` 新增可选字段 `captureSourceId`（不传 = 原行为，向后兼容）；新增错误码 `LIVE_SOURCE_GONE`（`internal/apperr` 现在 18 个码，第 2.1 节清单同步），`detail` 第一行 `kind=window|screen`（2.2 表新增一行）。新增类型 `CaptureSource`。`ScreenInfo` / `ListScreens` / `GetCaptureCapabilities` 不变。

v0.13 变更（EditService 素材上限，随实现回改的小修订，见 6.11 节）：`EditProject.sources`（素材库）上限由 200 改为 100（产品经理定稿：素材 100）；`SaveProject` / `ValidateProject` / `Export` 超过 100 个返回 `INVALID_ARGUMENT`（message「素材库最多 100 个文件」，detail 第一行 `project`、第二行 `sources=<实际个数>`）。clip 总数（视频 + 音频）上限不变，仍是 100——**素材 100 / 片段 100 都是 100，是两个独立上限**。无接口签名变化。

v0.12 变更（DocService 契约定稿，**只有契约，尚无实现**，见 6.12 节）：`ConvertToPDF` 保持签名，格式范围如实收窄为 `docx` / `xlsx` / `pptx` **纯文本版**（与 v1 一致：无图片、表格线、样式；旧版 `doc` / `xls` / `ppt` 及其他格式一律 `UNSUPPORTED`）；`GetPDFURL` 替换为 `OpenPDF`（返回 `PDFSource`）+ `ReadPDFChunk`（分块读，走 Wails Bind，不依赖 AssetServer 行为）；新增 `GetDocCapabilities` / `ListRecentPDFs` / `RemoveRecentPDFs`；新增表 `doc_recent`；任务类型 `office_pdf` 保持不变；PDF 渲染、页数、缩略图、搜索全部在前端 pdf.js（`@tato30/vue-pdf`）完成，后端不渲染、不提供合并 / 拆分 / 旋转（v1 也没有）。

v0.11 变更（EditService 契约定稿，**只有契约，尚无实现**，见 6.11 节）：`Render` 改名 `Export`，任务类型 `edit_render` 改名 `edit_export`（旧名从未产生过任务，无迁移问题）；新增 `ValidateProject` / `DeleteProject` / `GetPreviewURL`；`SaveProject` 返回 `EditProjectMeta`，`LoadProject` 返回 `LoadedProject`；`EditProject` 字段与校验范围、导出参数、错误码、预览方案全部写死；预览走 AssetServer 的 `/local/<token>`（不做本地流服务，不用 `file://`），并明确 Windows 上 AssetServer 不支持流式响应、单次响应必须限长。

v0.10 变更（LiveService 设计稿，**只是契约，尚未实现**，见第 4 节 LiveService 和 6.10 节）：重写 LiveService——删除 `StartRelay`、`StartRecordPush(wsURL)`、`Stop`、`GetHealth`、`ListArchives`、`GetPlayURL`，新增 `StartFilePush`、`StartScreenPush`、`GetCaptureCapabilities`、`ListScreens`、`CheckPushURL`；停止用 `TaskService.Cancel`；`TaskType` 的直播类型改为 `live_file_push | live_screen_push`；`task:progress`（和 `Task`）增加 `fps`、`bitrateKbps`、`droppedFrames`，删除 `live:stats` 事件；推流地址校验规则、推流密钥 / 凭据在标题、params、日志、错误、事件里的脱敏规则；直播错误分类沿用 6.9 的"先剔除元数据段落"做法；第 7 节（本地流服务）删除，只留说明。

v0.9.2 变更（Windows 子进程回收，见 6.6 节）：Windows 上 ffmpeg / ffprobe 子进程改为放进 Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`），应用崩溃或被强制结束时系统会回收 ffmpeg 及其子孙进程；结束进程树先终结 Job，失败退回 `taskkill /T /F`，再退回只结束主进程；其他平台行为不变。无接口变化。

v0.9.1 变更（#10 架构师审查修订，见 6.9 节）：转换错误分类先剔除 ffmpeg stderr 里的 `Input #` / `Output #` / `Metadata` / `Stream #` 段落（文件名、标题里的关键词不再误分类），磁盘满只在写入阶段句式（`Error writing trailer` / `Error muxing packet` 等）上判定；`RunWithPart` 创建输出目录或提交输出失败包成 `IO_ERROR`，磁盘满（ENOSPC、Windows 错误码 112 / 39）为 `CONVERT_DISK_FULL`；`ConvertOptions` 增加上下限（trim ≤ 1e6 秒、fps ≥ 0.1、videoBitrate ≤ 1e9、audioBitrate 8000~1000000、宽高 ≤ 8192）；新增错误码 `CANCELED`（调用因应用退出而被取消）；`ConvertService` 使用应用根 ctx；`app:files-dropped` 事件删除（前端直接用 Wails `OnFileDrop`）。

v0.9 变更：ConvertService 最小版落地（单文件转换、常见格式与预设、进度、取消、重试；**不含**两遍编码 / 按目标大小压缩，见 v0.7.2）：`ConvertOptions` 新增 `crf`；新增错误码 `CONVERT_DISK_FULL`（磁盘空间不足，原来归在 `IO_ERROR`）；`Submit` 一次最多 50 个文件、先整体校验再提交；详见新增 6.9 节。

v0.8.1 变更（媒体服务评审修订，见 6.7 节）：`Probe` / `Thumbnail` 使用应用根 ctx，应用退出时取消并结束 ffprobe / ffmpeg；`Probe` 一次最多 500 个，**前端应每批不超过约 50 个**；非普通文件（FIFO、设备）`INVALID_ARGUMENT`；`Thumbnail.atSec` 封顶 1e7，退回第 0 秒时缓存名和返回的 `atSec` 都按 0；超时不重试；只有字幕 / 数据流（.srt）或只有封面图的文件 `PROBE_FAILED`；带封面的 mp3 直接 `Thumbnail` 返回 `INVALID_ARGUMENT`；`media` 表只保留最近 1000 条；`RemoveRecent` 一次最多 500 个 id；ffprobe 输出有大小上限。

v0.8 变更：MediaService 落地（第 3、4 节 + 新增 6.7 节）：`MediaInfo` 扩展（container / fps / rotation / streams 等，见 6.7）；新增错误码 `PROBE_FAILED`；`Probe` 单个文件失败不影响整批（该项 `error` 有值）；新增 `Thumbnail`；`ThumbURL` 是 data URL（本地 HTTP 已取消）。

v0.7.7 变更（架构师合并前修订）：`RevealInFolder` 只允许两类路径，其余返回 `INVALID_ARGUMENT`：任务表里登记的输出路径，或当前 `defaultOutputDir` 之内的路径（目录本身也可以）；判断前先 Clean（折叠 `..`）再 `EvalSymlinks`，用真实路径比较（Windows / macOS 不区分大小写），符号链接逃逸和被换成链接的任务输出都会被拒绝。`PickDirectory` 签名**不变**仍是 `PickDirectory(title string)`：Wails v2.11 对可变参数生成 `Array<string>` 且运行时严格检查参数个数，无法做可选参数；前端无标题时要调用 `PickDirectory('')`（`frontend/src/stores/ffmpeg.ts` 的 `pickPath` 目前是无参调用，需要改）。

v0.7.6 变更：`Settings` 新增 `maxConcurrent`（int，batch 池同时运行的任务数）：`0` = 自动（CPU 核数的一半，限制在 1~3），手动 `1~8`，其他值（负数、大于 8）`INVALID_ARGUMENT`，`UpdateSettings` 原子（任何字段校验失败都整体不生效）。保存后立即应用到任务管理器（`Manager.SetConcurrency`）：调大让排队任务马上补位，调小**不打断**运行中的任务，只是暂停取新任务，直到运行数低于新上限；应用启动时应用上次保存的值。

v0.7.5 变更：`SystemService.PickFiles(filter FileFilter, multiple bool)` 落地，`FileFilter` 为 `{name string, patterns []string}`（见 6.8 节）。

v0.7.4 变更：`Settings` 新增 `defaultOutputDir`（string，空字符串 = 输出到源文件所在文件夹）。`UpdateSettings` 对非空值校验：必须是绝对路径、已存在的文件夹且可写，否则 `INVALID_ARGUMENT`，整个更新不生效（与 `ffmpegPath` 一样是原子的）；保存的是清理后的路径。转换等任务的 `outputDir` 传空时使用这个默认值。

v0.7.3 变更：`SystemService.PickDirectory(title string)`（契约里原来无参，现在加 `title`，空字符串用默认标题，用户取消返回 `""` 而不是错误）与 `RevealInFolder(path)` 落地（见 6.8 节）。

v0.7.2.1 变更（任务管理器合并前小修，只改 6.6 节的描述与实现细节，无接口变化）：`Submit` 返回的是**入队前**取的快照（`queued`、`version=1`，与 `task:created` 一致），之后的变化只走事件；`Runner` 返回的输出路径必须是绝对路径，相对路径不采信（保留提交时的预期路径）；`Remove(deleteOutput)` 还要求输出所在目录（含上级）不含符号链接（`EvalSymlinks` 后不变），否则只删记录不删文件；日志一次写入大块多行数据时按容量拆分写入并轮转，单文件不会超过 8 MB；`RunWithPart` 里 `produce` panic 时也会清理 `.part`。

v0.7.2 变更：两遍编码与按目标大小压缩（`TargetSizeMB`）**暂缓**，不在近期实现；`FFmpegRunner` 只支持单次 ffmpeg 调用，`ConvertOptions.targetSizeMb` 暂不生效（传大于 0 的值返回 `INVALID_ARGUMENT`），下面 v0.7.1 中关于两遍编码的内容作废。

v0.7.1 变更（任务管理器评审修订，见 6.6 节）：输出文件名在管理器内登记占用，最终提交用 `os.Link` 不覆盖；所有 ffmpeg 命令带 `-y`（不需要 stdin 的还带 `-nostdin`）；任务日志上限 16 MB（轮转）、单行上限 8 KB；两遍编码（已在 v0.7.2 撤回，暂缓）；Windows 结束整个进程树（`taskkill /T /F`）；`task:created` 一定先于该任务的 `task:status`；Retry 工厂在造 Runner 时就占位（安装不会重复提交）；`Remove(deleteOutput)` 不删与输入相同、修改时间早于任务开始、符号链接的文件，删除失败如实返回 `IO_ERROR`；`Spec.ID` 只能是字母数字、路径必须是绝对路径；应用退出时被停止的任务（含直播优雅停止）为 `interrupted`，排队任务同样调用 `OnFinish`；进度只增不减（`out_time=N/A` 忽略）；没有重试工厂的任务类型 `Retry` 返回新错误码 `UNSUPPORTED`。

v0.7 变更：任务管理器 `internal/task` 落地（第 4、5 节补充，新增 6.6 节实现约定；6.5 里的 `GoFuncRunner` / `DownloadRunner` 在代码里是 `task.RunnerFunc`）：新增 `TaskFilter` / `TaskPage` 定义；`task:progress` 也递增 `version`；`InstallFFmpeg` 返回真正的 `Task`（第 9 节 v0.6 的轻量 `InstallTask` 取消）。

v0.6.1 变更：镜像不再悄悄回退。`InstallFFmpeg(mirror)` 只接受 `""` 和当前平台真正有的镜像，其他值（含没有镜像的平台传 `"cn"`）返回 `INVALID_ARGUMENT`，detail 列出可选镜像；新增 `SystemService.GetInstallOptions()`（第 9.4 节）。

v0.6 变更：第 9 节补充安装实现（下载清单、`InstallFFmpeg(mirror)` 只接受 `""` / `"cn"`、`CancelFFmpegInstall`、安装期间的事件）；`FFmpegStatus.error` / `taskId` 无值时不输出（TS 中为可选字段）；`FFmpegStatus` 增加 `ffprobeMissing`。

v0.5 变更：错误码补充直播 / 录屏相关码（第 2 节）；第 7 节的本地流服务取消；第 9 节补充检测实现细节（git 构建取舍、`SetFFmpegPath` 空串、`ffmpeg.Require()` 门控）。

v0.4 变更：默认输出目录按平台区分；`.part` 改为 `<name>.part.<原扩展名>`；进度不落库；直播存档优雅停止。

v0.3 变更：`InstallFFmpeg` 幂等；macOS ad-hoc 签名；`Settings.ffmpegPromptDismissed`；录屏 native 采集补 Windows gdigrab、macOS 授权和 Wayland 限制。

v0.2 变更：新增 ffmpeg 环境检测与自动安装（第 9 节）；合入后端、前端第一轮评审意见（任务版本号、双池调度、Runner 接口、两遍编码、Range 预览、临时文件、路径规范化、文件拖放、录屏采集降级）。

范围：保留格式转换、视频剪辑、直播工具、Office 转 PDF、PDF 预览、JSON 工具，新增任务中心；删除 OpenClaw。
技术栈：Wails v2 + Vue 3 + TS 5 + Vite 5，Go 端 SQLite（modernc.org/sqlite，免 CGO）。

## 1. 总体约定

- 除二进制流以外，前后端一律通过 Wails Bind 调用，不再有 `localhost:19200`。
- 所有文件用**本地绝对路径**传递，选择文件用 `SystemService.PickFiles`，不再上传拷贝。
- 应用数据目录：`os.UserConfigDir()/FFmpegFree/`，下设 `app.db`、`thumbs/`、`logs/`；输出目录默认是系统"视频"目录下的 `FFmpegFree`：Windows 为 `%USERPROFILE%\Videos`，macOS 为 `~/Movies`，Linux 读 `XDG_VIDEOS_DIR`（读不到用 `~/Videos`），可在设置里改。
- 前端预览本地文件：通过 Wails AssetServer 的 `Handler` 挂 `/local/<token>`，由后端按 token 映射真实路径，不暴露任意路径读取；协议、限长、Windows 限制、token 生命周期见 6.13。
- 时间一律 Unix 毫秒（int64），时长一律秒（float64），大小一律字节（int64）。
- ID 一律 ULID 字符串。
- 拖拽文件：前端直接使用 Wails 运行时（`OnFileDrop`）拿到绝对路径，后端不转发事件；前端不从 WebView 的 File 对象取路径。
- 路径入库前统一规范化：`filepath.Clean` + 转绝对路径；Windows 和 macOS 上额外转小写生成 `path_key` 做唯一约束，原始大小写保留在 `path` 用于显示。

## 2. 错误约定

Bind 方法返回 `(T, error)`。error 的 message 是 JSON 字符串，前端 `api/` 层统一解析：

```json
{ "code": "FFMPEG_NOT_FOUND", "message": "未找到 ffmpeg 可执行文件", "detail": "..." }
```

| code | 含义 |
|---|---|
| INVALID_ARGUMENT | 参数不合法 |
| NOT_FOUND | 记录或文件不存在 |
| FFMPEG_NOT_FOUND | ffmpeg 缺失 |
| TASK_CONFLICT | 任务状态不允许该操作（如取消已完成任务）；直播（v0.10）：已有进行中的屏幕推流时再开一路屏幕推流（`detail` 首行 `reason=screen_busy`，**屏幕推流同一时间最多 1 路**），同一推流地址已有进行中的会话（`reason=duplicate_url`），或进行中的直播会话已达 4 个上限（`reason=max_sessions`）；判断顺序 `duplicate_url`、`screen_busy`、`max_sessions`（架构师定），稳定枚举见 2.2 与 6.10 |
| IO_ERROR | 读写文件失败 |
| PROBE_FAILED | 文件存在但 ffprobe 无法解析（损坏、不是音视频文件、没有可识别的流） |
| CANCELED | 调用因应用退出（根 ctx 取消）而被取消，结果作废；前端不需要提示用户（`ConvertService.Submit`、`LiveService.Start*` 等） |
| UNSUPPORTED | 该操作不支持这个对象（如没有重试工厂的任务类型不能 Retry；直播会话 Retry 也是它；直播（v0.10）：本机 ffmpeg 缺少推流协议时 `Start*` 返回它，`detail` 是 `missing=<协议名>`（协议名取 `rtmp`、`rtmps`、`srt`，如 `missing=srt`），见 2.2） |
| CONVERT_DISK_FULL | 转换写输出文件时磁盘空间不足（前端标题「磁盘空间不足」，可引导用户换输出目录） |
| PROCESS_FAILED | 子进程非零退出，detail 带最后 50 行日志 |
| UNSUPPORTED_PLATFORM | 当前系统或会话不支持该功能（如 Linux Wayland 下的屏幕采集、Linux 没有 `DISPLAY`、x11grab 打不开显示） |
| LIVE_URL_INVALID | 推流地址格式不合法或协议不支持（只允许 rtmp / rtmps / srt，规则见第 4 节 LiveService）；`detail` 第一行 `reason=<值>`，见 2.2 |
| LIVE_CONNECT_FAILED | 推流**开始前**连接目标失败（DNS、拒绝连接、超时、网络不可达；SRT 的服务器未开与被拒绝无法区分，也归它）：任务在收到第一条 `task:progress` 之前就失败；`detail` 第一行 `scheme=rtmp\|rtmps\|srt`，见 2.2 |
| LIVE_PUSH_REJECTED | 目标服务器明确拒绝推流（RTMP 鉴权失败、流名冲突、握手被拒等；SRT 不会出现），**推流开始前** |
| LIVE_PUSH_INTERRUPTED | 推流**已经开始**（收到过 `task:progress`）后被目标服务器或网络中断 |
| SCREEN_PERMISSION_DENIED | 没有屏幕录制权限（macOS 系统授权），`StartScreenPush` 同步返回或任务失败 |
| LIVE_SOURCE_GONE | （v0.14）`StartScreenPush` 传了 `captureSourceId`，但所选来源此刻已不可用：窗口已关闭 / 已最小化 / 不可见，或屏幕序号不存在（显示器被拔掉）。同步返回（没有创建任务）；ffmpeg 打开窗口时才发现窗口没了（校验与打开之间的竞态）则是任务失败，码相同。`detail` 第一行 `kind=window` 或 `kind=screen`，**不带窗口标题**。前端提示「所选窗口已不可用，请重新选择」（屏幕：「所选屏幕已不可用，请重新选择」）并重新 `ListCaptureSources` |
| INTERNAL | 其他；直播任务里认不出的 ffmpeg 非零退出也是它（不是 `PROCESS_FAILED`），detail 带（已脱敏的）stderr 最后若干行 |

**直播 / 录屏（v0.10）用到的后端码正好是冻结的这八个：`LIVE_URL_INVALID`、`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`SCREEN_PERMISSION_DENIED`、`FFMPEG_NOT_FOUND`、`UNSUPPORTED_PLATFORM`、`INTERNAL`**（v0.14 起再加 `LIVE_SOURCE_GONE`，共九个）；此外复用已有的 `INVALID_ARGUMENT`、`NOT_FOUND`、`PROBE_FAILED`（输入文件问题）、`TASK_CONFLICT`（会话上限 / 同地址冲突）、`UNSUPPORTED`（Retry）、`CANCELED`（`Start*` 因应用退出被取消，#10 已加）。**v0.10 没有新增任何错误码**，也没有 `LIVE_START_FAILED` 之类的同义码。用户主动停止不产生错误码（优雅停止成功 = `succeeded`，超时强杀 = `canceled` 状态，`error` 为空）。`LIVE_PLAY_FAILED`（播放器加载或解码失败）和 `LIVE_CORS_BLOCKED`（拉流地址跨域被浏览器拦截）**只在前端由播放器产生**，后端不会返回，也不在 `apperr` 里定义。

### 2.1 AppErrorCode 完整清单（供前端 `frontend/src/api/call.ts` 对照）

后端 `internal/apperr` 一共 18 个码（v0.14 新增 `LIVE_SOURCE_GONE`），前端 `AppErrorCode` 必须全部包含；前端 `frontend/src/api/call.ts` 以本清单为准逐项核对补全（不在契约里写它当前缺几个，现状随前端分支变化）：

```ts
export type AppErrorCode =
  | 'INVALID_ARGUMENT' | 'NOT_FOUND' | 'FFMPEG_NOT_FOUND' | 'TASK_CONFLICT' | 'IO_ERROR'
  | 'PROBE_FAILED' | 'CANCELED' | 'UNSUPPORTED' | 'CONVERT_DISK_FULL' | 'PROCESS_FAILED'
  | 'UNSUPPORTED_PLATFORM' | 'INTERNAL'
  | 'LIVE_URL_INVALID' | 'LIVE_CONNECT_FAILED' | 'LIVE_PUSH_REJECTED' | 'LIVE_PUSH_INTERRUPTED'
  | 'SCREEN_PERMISSION_DENIED' | 'LIVE_SOURCE_GONE'
```

- 前端遇到不在清单里的 `code`：按 `INTERNAL` 的通用文案处理，不崩溃。
- `LIVE_PLAY_FAILED`、`LIVE_CORS_BLOCKED` 只在前端播放器里产生，**不是** `AppErrorCode`，也不出现在后端。
- 之后新增错误码：先改本表和 `internal/apperr`，再改前端；三处必须一致。

### 2.2 `detail` 第一行格式（按错误码分，稳定枚举）

| code（触发场景） | `detail` 第一行 | 取值（只追加，不改名、不改含义、不删除） | 说明 |
|---|---|---|---|
| `TASK_CONFLICT`（直播 `Start*` 的会话冲突） | `reason=<值>` | `screen_busy`（已有进行中的 `live_screen_push`，再开一路屏幕推流；文件推流不会得到它）、`duplicate_url`（同一标准化地址已有会话）、`max_sessions`（进行中的直播会话已达 4 个）；**判断顺序固定（架构师定）：`duplicate_url` → `screen_busy` → `max_sessions`**，同时满足多个条件时只返回最先命中的 | 其他 `TASK_CONFLICT`（`Cancel` 已结束的会话、`Remove` 进行中的任务等）**没有** `reason=` 行；不含任何地址、口令、streamkey |
| `LIVE_URL_INVALID` | `reason=<值>` | `scheme_unsupported`（scheme 不是 rtmp / rtmps / srt）、`malformed`（空串、超长、含非法字符、缺 scheme、端口越界或缺失、rtmp 缺应用名、SRT 参数值非法如 passphrase 长度、IPv6 括号错误等）、`missing_host`（host 为空）、`param_not_allowed`（SRT 查询参数不在白名单、`mode` 不是 `caller`、同名参数重复） | **不带地址、口令，也不带它们的任何片段**（连脱敏后的地址也不放），第二行起可以写不含地址的原因说明 |
| `UNSUPPORTED`（直播 `Start*` 时本机 ffmpeg 缺协议） | `missing=<协议名>` | `rtmp`、`rtmps`、`srt`（对应推流地址的 scheme；`rtmp` 是除 `rtmps` / `srt` 以外的默认）；带本地存档的会话另需 `tee`，缺时是 `missing=tee` | `detail` 只有这一行，没有第二行；`message` 是"当前 ffmpeg 不支持 <协议名>，请安装完整版 ffmpeg"，前端据此提示安装完整版；其他原因的 `UNSUPPORTED`（如 `Retry` 直播任务、屏幕推流存档未实现）没有这一行 |
| `LIVE_CONNECT_FAILED` | `scheme=<值>` | `rtmp`、`rtmps`、`srt`（取自校验后的标准化地址，小写） | 第二行起是脱敏后的 ffmpeg stderr 最后若干行；前端据此选 RTMP / SRT 的提示文案（SRT 用"连接失败，请检查地址和口令是否正确"，文案由前端负责，后端 `message` 不承载） |
| 编辑类错误（`EditService` 的 `ValidateProject` / `Export` 返回的 `INVALID_ARGUMENT`、`NOT_FOUND`、`IO_ERROR`、`PROBE_FAILED`、`UNSUPPORTED`） | **按 6.11.2 B（#22）**：`clip=<clip.id> path=<绝对路径>`，或没有 clip 的工程级错误写 `project` | 由 6.11.2 B 定义，第二行起才是原因（如 `overlaps=<clip.id>`、`path_length=<n> limit=259`、`missing=filter_complex`） | 前端用 6.11.2 B 的正则取首行；**不适用**下面"第一行只有一个 `key=value`"的统一规则；此行是 #22 合入后生效，#22 单独看时它引用的 6.11.2 B 就在该 PR 里，措辞与 #22 的 B 一致（已核对） |
| `LIVE_SOURCE_GONE`（v0.14，`StartScreenPush` 的所选来源已不可用） | `kind=<值>` | `window`（窗口已关闭 / 最小化 / 不可见）、`screen`（屏幕序号不存在）（只追加） | 只有这一行，没有第二行（不带窗口标题、不带地址）；前端用 `^kind=(window\|screen)$` 匹配（未知值按通用文案）；前端不必解析也能工作：码本身就足够提示「所选窗口已不可用」 |
| DocService 错误（v0.16，`ConvertToPDF` 的同步校验和 `office_pdf` 任务的 `error`、`OpenPDF`、`ReadPDFChunk`；只有下面取值对应的场景，其余 Doc 错误没有 reason） | `reason=<值>` | `too_many_pages`（`UNSUPPORTED`，超过 5000 页，含文字量超限）、`format`（`UNSUPPORTED` 或 `INVALID_ARGUMENT`，格式不受支持：不支持的扩展名、没有扩展名、`OpenPDF` 的扩展名不是 `.pdf` / 内容不是 PDF）、`encrypted`（`UNSUPPORTED`，加密的 Office 文档，OLE 容器；**加密 PDF 能正常打开，不会有这个错误**）、`no_font`（`UNSUPPORTED`，需要 Unicode 字体而没有）、`invalid_ooxml`（`INVALID_ARGUMENT`，不是 zip、缺必需部件、XML 损坏、zip64 目录信息无效）、`too_large`（`INVALID_ARGUMENT`，超大小或超 zip 限制：文件 > 100 MiB、PDF > 512 MiB、zip 条目数 > 100 000、中央目录 > 9 600 000 字节、单个条目解压后 > 256 MiB）（只追加，不改名、不改含义、不删除） | `code` 和 `message` 不变，`message` 是给用户看的短句（精确文案见 6.12.6）；首行之后可以有自由文本行：`ConvertToPDF` 整体校验失败时**第二行是出错文件的绝对路径**（`reason` 行永远在首行），再后面是原因说明；任务的 `error` 没有路径行。取消、磁盘满（`CONVERT_DISK_FULL`）、读写失败（`IO_ERROR`）、`NOT_FOUND`、路径 / 参数 / 输出目录 / 句柄类的 `INVALID_ARGUMENT`、`INTERNAL` **没有 reason**，走该码的通用文案 |
| 其余所有码（含 `LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`INTERNAL`） | 无固定格式 | — | 前端**不得**解析（上面几行列出的码 / 场景除外） |

统一规则：第一行只有一个 `key=value`，值只含小写字母、数字、下划线（`scheme` 例外；`kind` 的取值是 `window` / `screen`，取值就是上面三个小写单词）；前端用 `^(reason|scheme|kind)=([a-z0-9_]+)$` 匹配 `detail` 的第一行；**没有第一行、格式不对、或值不认识，一律走该错误码的通用文案**，不得猜测含义、不得报错崩溃。测试必须逐码断言第一行精确等于期望值（不是包含）。

### 2.3 示例（JSON 里的数值是示意）

```json
{ "code": "LIVE_URL_INVALID", "message": "暂不支持这种推流协议", "detail": "reason=scheme_unsupported" }
```
```json
{ "code": "TASK_CONFLICT", "message": "直播会话冲突", "detail": "reason=duplicate_url\n已有会话使用同一推流地址" }
```
```json
{ "code": "TASK_CONFLICT", "message": "已有屏幕推流在进行", "detail": "reason=screen_busy" }
```
```json
{ "code": "LIVE_CONNECT_FAILED", "message": "连接推流服务器失败", "detail": "scheme=rtmp\n[tcp @ 0x7f50942c6900] Connection to tcp://127.0.0.1:1999?tcp_nodelay=0 failed: Connection refused\n[out#0/tee @ 0x557c2ff9e9c0] Could not write header (incorrect codec parameters ?): Connection refused" }
```


## 3. 数据模型（Go struct，Wails 自动生成 models.ts）

```go
type MediaInfo struct {
    ID         string  `json:"id"`
    Path       string  `json:"path"`
    Name       string  `json:"name"`
    Size       int64   `json:"size"`
    Duration   float64 `json:"duration"`
    Width      int     `json:"width"`
    Height     int     `json:"height"`
    VideoCodec string  `json:"videoCodec"`
    AudioCodec string  `json:"audioCodec"`
    Bitrate    int64   `json:"bitrate"`
    ThumbURL   string  `json:"thumbUrl"`   // data:image/jpeg;base64,...（v0.8）；无视频画面或生成失败为 ""
    // v0.8 扩展，只在 Probe 时填充、不入库（ListRecent 里为零值 / 省略）：
    // container, fps, rotation(0/90/180/270), sampleRate, channels, hasVideo, hasAudio,
    // streams[]{index,type,codec,profile,width,height,pixFmt,fps,bitrate,duration,rotation,sampleRate,channels,channelLayout,language,attachedPic},
    // probedAt, error?(批量探测时该文件的错误)
}

type TaskType string // convert | edit_export | office_pdf | live_file_push | live_screen_push | ffmpeg_install
                     // 保留但不再产生：live_relay、live_record_push（v0.10 取消）、edit_render（v0.11 起改名 edit_export）。不能提交，任务中心不展示，库里的旧记录按未知类型忽略、不报错
type TaskStatus string // queued | running | succeeded | failed | canceled | interrupted

type Task struct {
    ID         string     `json:"id"`
    Type       TaskType   `json:"type"`
    Status     TaskStatus `json:"status"`
    Title      string     `json:"title"`
    InputPaths []string   `json:"inputPaths"`
    OutputPath string     `json:"outputPath"`
    Progress   float64    `json:"progress"`   // 0~1，直播类任务恒为 -1
    Speed      string     `json:"speed"`      // 如 "2.3x"
    EtaSec     float64    `json:"etaSec"`
    // 以下三项只有直播任务在运行中才有值（v0.10），只在内存里、不落库，和 speed / etaSec 一样：
    Fps           float64 `json:"fps,omitempty"`           // 当前输出帧率
    BitrateKbps   float64 `json:"bitrateKbps,omitempty"`   // 近 5 秒的输出码率（kbit/s）；有本地存档（tee）的会话没有此值，省略
    DroppedFrames int64   `json:"droppedFrames,omitempty"` // ffmpeg 丢弃的帧数（累计），不是网络丢包
    // 以下四项（v0.18，见 9.7）：任务实际使用的视频编码器；没有视频编码的任务（纯音频转换、Office 转 PDF、ffmpeg 安装）省略；会落库（tasks 表迁移 0004）：
    Encoder          string `json:"encoder,omitempty"`          // h264_nvenc | hevc_nvenc | h264_qsv | hevc_qsv | h264_amf | hevc_amf | h264_videotoolbox | hevc_videotoolbox | libx264 | libx265 | libvpx-vp9 | gif | copy
    EncoderDevice    string `json:"encoderDevice,omitempty"`    // 设备 id（nvidia-0 之类，即 EncoderDevice.id）；CPU 编码为 "cpu"；copy 时省略
    HWFallback       bool   `json:"hwFallback,omitempty"`       // 想用硬件但实际用了 CPU：所选设备不可用，或硬件编码启动失败后自动用 CPU 重试
    HWFallbackReason string `json:"hwFallbackReason,omitempty"` // 一行短原因（固定枚举，不含路径），见 9.7
    Params     string     `json:"params"`     // 原始参数 JSON，用于重试（直播任务的 params 已脱敏，不能用来重试，见 6.10）
    Version    int64      `json:"version"`    // 每次变更 +1，前端据此丢弃旧事件
    Error      *AppError  `json:"error,omitempty"` // 无错误时省略（不是 null）；TS 里是 error?: AppError；succeeded / canceled 一律没有该键
    CreatedAt  int64      `json:"createdAt"`
    StartedAt  int64      `json:"startedAt"`
    FinishedAt int64      `json:"finishedAt"`
}

type Preset struct {
    ID       string        `json:"id"`
    Name     string        `json:"name"`
    BuiltIn  bool          `json:"builtIn"`
    Options  ConvertOptions `json:"options"`
}

type ConvertOptions struct {
    Container    string  `json:"container"`    // mp4 mkv mov webm avi flv gif mp3 aac wav flac m4a ogg
    VideoCodec   string  `json:"videoCodec"`   // copy h264 h265 vp9 ""(无视频)
    AudioCodec   string  `json:"audioCodec"`
    Width        int     `json:"width"`        // 0 保持
    Height       int     `json:"height"`
    Fps          float64 `json:"fps"`
    VideoBitrate int64   `json:"videoBitrate"` // 0 自动
    AudioBitrate int64   `json:"audioBitrate"`
    Crf          int     `json:"crf"`          // 0 用编码器默认（h264 23、h265 28、vp9 32）；范围 0~63（h264/h265 最大 51）；设了码率时忽略；copy 不能设
    TargetSizeMB float64 `json:"targetSizeMb"` // 暂缓：>0 时按目标大小反推码率（两遍编码），目前传 >0 返回 INVALID_ARGUMENT
    TrimStart    float64 `json:"trimStart"`
    TrimEnd      float64 `json:"trimEnd"`
}
```

## 4. Service 方法

### SystemService
```go
PickFiles(filter FileFilter, multiple bool) ([]string, error)
PickDirectory(title string) (string, error) // 取消返回 ""
RevealInFolder(path string) error
GetEnv() (EnvInfo, error)            // 系统、ffmpeg 版本、数据目录
GetSettings() (Settings, error)
UpdateSettings(s Settings) error      // defaultOutputDir（空=与源文件同目录）、maxConcurrent（0=自动，1~8）、主题、语言、ffmpegPromptDismissed、ffmpegPath
ListEncoderDevices() (EncoderDeviceList, error)      // 硬件编码设备（9.6）：第一项永远是 cpu；ffmpeg 未就绪时只有 cpu 且 ffmpegReady=false，不报错
RefreshEncoderDevices() (EncoderDeviceList, error)   // 丢弃缓存重新检测
GetEncoderPreference() (string, error)               // "auto" | "cpu" | 设备 id，默认 "auto"；所选设备不可用时保持原值
GetEncoderPreferenceInfo() (EncoderPreferenceInfo, error) // {id, name, available, reason?}
SetEncoderPreference(id string) error                // 只接受 auto、cpu、ListEncoderDevices 里存在的设备 id，否则 INVALID_ARGUMENT
```

### App（main 包，非 Service）
```go
GetLicenseText(name string) (string, error) // 内嵌第三方许可全文；白名单 "OFL"（Noto Sans SC 的 SIL OFL 1.1）、"OFL-Nunito"（Nunito 的 SIL OFL 1.1），其它名称（含空串、带路径、大小写不同）→ INVALID_ARGUMENT
GetAppVersion() string                      // 构建时 -ldflags "-X FFmpegFree/internal/about.Version=..." 注入；未注入返回 "开发版"
```

### MediaService
```go
Probe(paths []string) ([]MediaInfo, error)   // 批量探测（最多 500 个），结果写入 media 表；返回值与入参一一对应，单个失败时该项 error 有值
Thumbnail(path string, atSec float64, width int) (Thumb, error) // Thumb{path, dataUrl, atSec, width}；带磁盘缓存
ListRecent(limit int) ([]MediaInfo, error)    // 默认 20，最大 200
RemoveRecent(ids []string) error              // 只删记录，不删文件
```

### ConvertService
```go
ListPresets() ([]Preset, error)
SavePreset(p Preset) (Preset, error)
DeletePreset(id string) error
Submit(inputs []string, opts ConvertOptions, outputDir string) ([]Task, error) // 批量，一个文件一个任务
```

### EditService（多轨时间线，v0.11 契约，详见 6.11）
```go
ValidateProject(project EditProject) (EditPlan, error)                  // 不落盘、不启动导出；探测素材并做全部校验（同 Export 的 6.11.2），返回时长与结构化警告 EditWarning[]
Export(project EditProject, opts EditExportOptions) (Task, error)       // 提交一个 edit_export 任务；进度走 task:progress，取消走 TaskService.Cancel
GetPreviewURL(path string) (PreviewURL, error)                          // 预览用的 /local/<token>，见 6.11.4 与 6.13
SaveProject(project EditProject) (EditProjectMeta, error)               // id 空 = 新建；只校验数量上限（草稿可保存，重叠 / 越界只在 ValidateProject 和 Export 报）；不做后端自动保存
LoadProject(id string) (LoadedProject, error)                           // 返回工程 + 已丢失的素材路径
ListProjects(limit int) ([]EditProjectMeta, error)                      // 默认 50，最大 200，按 updatedAt 倒序
DeleteProject(id string) error                                          // 不存在 NOT_FOUND；不删素材和导出文件
```
素材管理不在 EditService：选文件 `SystemService.PickFiles`，探测 `MediaService.Probe`，缩略图 `MediaService.Thumbnail`，最近素材 `MediaService.ListRecent`。素材列表随工程保存在 `EditProject.sources`。

### DocService（Office 转 PDF + PDF 预览，v0.12 契约，详见 6.12）
```go
GetDocCapabilities() (DocCapabilities, error)                       // 支持的格式、字体状态、experimental 标志、上限；不依赖 ffmpeg，随时可调
ConvertToPDF(inputs []string, outputDir string) ([]Task, error)     // 批量，一个文件一个 office_pdf 任务；先整体校验再提交
OpenPDF(path string) (PDFSource, error)                             // 校验并登记一个 PDF，返回句柄；同时写入 doc_recent
ReadPDFChunk(id string, offset int64, length int) (PDFChunk, error) // 按句柄分块读 PDF 字节（Bind，Data 是 base64 字符串），length ≤ 1 MiB
ListRecentPDFs(limit int) ([]PDFFile, error)                        // 默认 20（limit ≤ 0 取 20），最大 200：limit > 200 **静默截断到 200**，不报错；按 openedAt 倒序
RemoveRecentPDFs(ids []string) error                                // 一次最多 500 个；只删记录、不删文件；同时撤销句柄和 /local/<token>
```
`GetPDFURL(path) string` 在 v0.12 删除（未实现过，无迁移）。

### JsonService（纯函数，不落库）
```go
Format(req JsonFormatRequest) (JsonFormatResponse, error)
Compare(req JsonCompareRequest) (JsonCompareResponse, error)
Validate(req JsonValidateRequest) (JsonValidateResponse, error)
```
结构沿用现有 `vo/JsonInfo.go`。

### LiveService（v0.10 设计稿，尚未实现）

没有本地流服务、没有 WebSocket：ffmpeg 直接把流推到用户填的地址，播放由前端播放器（mpegts.js）直接拉远端地址。会话就是 `live` 池里的一个 live 类型任务（不排队、不占 batch 名额），**会话 id = 任务 id**，`progress` 恒为 -1，指标走 `task:progress`。所有方法依赖 ffmpeg（缺失返回 `FFMPEG_NOT_FOUND`），启动完成之前返回 `INTERNAL`；`Start*` 用应用根 ctx，被取消返回 `CANCELED`。

```go
StartFilePush(req FilePushRequest) (Task, error)       // 文件推流（可循环）
StartScreenPush(req ScreenPushRequest) (Task, error)   // 屏幕推流（可同时本地存档）；同一时间最多 1 路：已有进行中的 live_screen_push 返回 TASK_CONFLICT（detail 首行 reason=screen_busy）
GetCaptureCapabilities() (CaptureCapabilities, error)  // 屏幕采集能不能用、为什么不能用（Linux 读 XDG_SESSION_TYPE 和 DISPLAY，见 6.10「采集能力检测」）
ListCaptureSources() ([]CaptureSource, error)          // （v0.14）屏幕推流可选的采集来源：屏幕（所有平台）+ 应用窗口（只有 Windows）；不能采集屏幕的平台 / 会话返回 UNSUPPORTED_PLATFORM（同 ListScreens），见 6.10「采集来源」
GetPreview(sessionID string) (Preview, error)          // （v0.17）会话（推流任务 id 或拉流预览会话 id）最新一帧预览：{data: base64 JPEG, ts: 毫秒时间戳, active}；没有画面（会话不存在 / 已结束、preview=false、还没出第一帧）返回空 data、ts=0，不是错误。前端约 500 毫秒轮询，见 6.10「预览画面」
StartPullPreview(req PullPreviewRequest) (PullSession, error) // （v0.17）拉流预览会话：后端 ffmpeg 读 rtmp / rtmps / srt / http(s) 远端流，只输出预览；同一地址幂等；同时最多 4 路
StopPullPreview(sessionID string) error                // （v0.17）停止拉流预览会话并清理预览文件；会话不存在（已结束）无操作
ListScreens() ([]ScreenInfo, error)                    // 可采集的显示器（Linux 用 xrandr --display $DISPLAY --query，必须带 --display，见 6.10「采集能力检测」）
CheckPushURL(url string) (PushURLInfo, error)          // 只校验地址并返回脱敏后的显示文本，不联网
// 停止：TaskService.Cancel(taskID)，没有单独的 StopPush（理由见下）
// 查询会话：TaskService.ListActive / Get / List（type = live_*），实时指标看 task:progress
```

```go
// 两个 Start* 共用的编码选项。全部可省略（零值 = 默认）；越界 INVALID_ARGUMENT。
type PushOptions struct {
    Width            int     `json:"width"`            // 0 = 保持（文件）/ 采集分辨率（屏幕）；上限 8192，输出保证偶数
    Height           int     `json:"height"`           // 同上；只给一个按比例缩放
    Fps              float64 `json:"fps"`              // 0 = 保持源帧率（文件）/ 30（屏幕）；范围 1~60
    VideoBitrateKbps int     `json:"videoBitrateKbps"` // 0 = 2500；范围 100~50000（kbit/s）
    AudioBitrateKbps int     `json:"audioBitrateKbps"` // 0 = 128；范围 32~512
}

type FilePushRequest struct {
    InputPath  string      `json:"inputPath"`  // 绝对路径的普通文件，必须有视频画面（否则 INVALID_ARGUMENT）
    URL        string      `json:"url"`        // 推流地址，规则见下
    Preview    *bool       `json:"preview"`    // （v0.17）可选：nil / true = 带预览画面（GetPreview）；false = 不加预览输出。只在开始时决定（ffmpeg 已启动无法动态改输出）
    Loop       bool        `json:"loop"`       // true = 循环播放直到用户停止；false = 播完自然结束（任务 succeeded）
    Options    PushOptions `json:"options"`
}

type ScreenPushRequest struct {
    URL        string      `json:"url"`
    ScreenID   string      `json:"screenId"`   // ListScreens 返回的 id；"" = 主显示器；不存在 INVALID_ARGUMENT
    HideCursor bool        `json:"hideCursor"` // 零值 = 画面里带鼠标指针
    Audio      string      `json:"audio"`      // "none"（默认，视频流里没有音轨）| "silent"（补一路静音音轨，给要求必须有音频的服务器）；采集声音 v1 不做
    Preview    *bool       `json:"preview"`    // （v0.17）同 FilePushRequest.preview
    CaptureSourceID string `json:"captureSourceId"` // （v0.14）可选：ListCaptureSources 返回的 id（screen:<序号> | window:<hwnd 十进制>）；"" = 不传，行为同 v0.13（按 ScreenID）；非空时以它为准，ScreenID 被忽略；格式不对 INVALID_ARGUMENT；来源已不可用 LIVE_SOURCE_GONE
    ArchiveDir string      `json:"archiveDir"` // 非空 = 同时在本地存一份 mp4（绝对路径，不存在会创建；存档规则见 6.10）；"" = 不存档
    Options    PushOptions `json:"options"`
}

type CaptureCapabilities struct {
    Supported    bool   `json:"supported"`    // 当前系统 / 会话能否采集屏幕
    Platform     string `json:"platform"`     // windows | darwin | linux
    Backend      string `json:"backend"`      // gdigrab | avfoundation | x11grab；不支持时 ""
    SessionType  string `json:"sessionType"`  // 只对 linux 有意义：x11 | wayland | unknown；其他平台 ""
    Permission   string `json:"permission"`   // granted | denied | unknown | notRequired（macOS 屏幕录制授权；查不出来是 unknown）
    AudioCapture bool   `json:"audioCapture"` // v1 恒为 false
    Reason       string `json:"reason"`       // 不支持时给用户看的中文原因，支持时 ""
}

// v0.17：GetPreview 的返回。没有画面时 data 为 ""、ts 为 0（不是错误）。
type Preview struct {
    Data   string `json:"data"`   // 最新一帧 JPEG 的 base64（标准编码，不带 data: 前缀）；没有画面 ""
    TS     int64  `json:"ts"`     // 这一帧写入的时间（毫秒时间戳，取文件修改时间）；没有画面 0。前端可据此判断画面是否停滞
    Active bool   `json:"active"` // 会话还在进行（推流任务未结束 / 拉流预览会话未结束）；false 时前端停止轮询
}

type PullPreviewRequest struct {
    URL     string `json:"url"`     // rtmp / rtmps / srt / http / https；ws / wss 没有对应的 ffmpeg 协议，LIVE_URL_INVALID（reason=scheme_unsupported）
    Preview *bool  `json:"preview"` // nil / true = 出预览；false = 不启动 ffmpeg（GetPreview 恒为空）
}

type PullSession struct {
    ID       string `json:"id"`       // 会话 id，传给 GetPreview / StopPullPreview
    Redacted string `json:"redacted"` // 脱敏后的地址，可直接显示
    Preview  bool   `json:"preview"`  // 是否真的在出预览
}

// v0.14：一个可采集的来源。ListCaptureSources 返回它的列表：先是所有屏幕（顺序同 ListScreens），Windows 上再是窗口（EnumWindows 的 Z 序，最上面的在前）。
type CaptureSource struct {
    ID     string `json:"id"`     // 不透明字符串，前端原样传回 captureSourceId。screen:<序号>（序号是 ListScreens 结果里的位置，从 0 起，第 0 个不一定是主显示器）；window:<hwnd 十进制>（无符号十进制，无前导零）
    Kind   string `json:"kind"`   // screen | window。macOS / Linux 永远只有 screen，不返回 window
    Title  string `json:"title"`  // screen：ScreenInfo.Name（如 "屏幕 1（主显示器）"）；window：窗口标题原文
    Width  int    `json:"width"`  // 物理像素；window 是客户区大小（gdigrab 采的就是客户区）；查不到为 0
    Height int    `json:"height"`
}

type ScreenInfo struct {
    ID      string  `json:"id"`      // 不透明字符串，前端只原样传回：windows "monitor:<序号>"、darwin "avf:<设备序号>"、linux "x11:<输出名>" 或 "x11:desktop"
    Name    string  `json:"name"`    // 如 "屏幕 1（主显示器）"
    Primary bool    `json:"primary"`
    X       int     `json:"x"`       // 在虚拟桌面里的位置，物理像素；查不到为 0
    Y       int     `json:"y"`
    Width   int     `json:"width"`   // 物理像素；查不到为 0
    Height  int     `json:"height"`
    Scale   float64 `json:"scale"`   // 系统缩放倍数（1、1.5、2…）；查不到为 1
}

type PushURLInfo struct {
    Scheme   string `json:"scheme"`   // rtmp | rtmps | srt
    Host     string `json:"host"`
    Port     int    `json:"port"`     // 地址里没写用默认值：rtmp 1935、rtmps 443；srt 必须写端口
    Redacted string `json:"redacted"` // 脱敏后的地址，可以直接显示：rtmp://host/app/***
}
```

**返回值**：`Start*` 立即返回，不等连接成功。返回的 `Task` 是入队前取的快照（`status=queued`、`version=1`，与 `task:created` 一致）；紧接着 `task:status(running)`；连接 / 鉴权失败以任务 `failed` + `error` 体现，不是 `Start*` 的返回错误。前端判断"已经在推了"：`running` 且已收到该任务的第一条 `task:progress`（ffmpeg 有输出才会有）；`running` 但还没有 progress = "连接中"。

**`Start*` 同步返回的错误**（此时没有创建任务）：`FFMPEG_NOT_FOUND`；`INVALID_ARGUMENT`（选项越界、输入文件没有视频、`archiveDir` 不是绝对路径、`screenId` 不存在）；`NOT_FOUND` / `PROBE_FAILED`（输入文件不存在 / 无法解析）；`LIVE_URL_INVALID`；`UNSUPPORTED`（开始前用 `ffmpeg -protocols` 检查：`Output:` 段必须有与地址 scheme 对应的协议（`srt` 地址要 `srt`，`rtmps` 地址要 `rtmps`，其余即 `rtmp` 地址要 `rtmp`），带存档时另需 `tee`，缺哪个返回它，**`detail` 就是单独一行 `missing=<协议名>`（第一行，没有第二行）**，协议名取 `rtmp`、`rtmps`、`srt`（带存档的会话缺 `tee` 时是 `missing=tee`）；`message` 是"当前 ffmpeg 不支持 <协议名>，请安装完整版 ffmpeg"；`StartFilePush` 和 `StartScreenPush` 在参数校验之后、探测输入 / 枚举屏幕之前检查（`internal/service/live/service.go` 的 `checkProtocols`）；`CheckPushURL` 只校验地址、不联网、**不做协议检查**，不返回它；结果按 ffmpeg 路径缓存；7.1.5 上 `rtmp`、`rtmps`、`srt`、`tee` 都在）；`UNSUPPORTED_PLATFORM`（不能采集屏幕）；`SCREEN_PERMISSION_DENIED`（已知没有权限时）；`TASK_CONFLICT`（`detail` 首行 `reason=`，判断顺序 **`duplicate_url` → `screen_busy` → `max_sessions`**：同一个推流地址已经有进行中的会话（`duplicate_url`）；`StartScreenPush` 时已有进行中的 `live_screen_push`（屏幕推流同一时间最多 1 路，`reason=screen_busy`）；进行中的直播会话已达 4 个上限（`max_sessions`）；`StartFilePush` 只会得到 `duplicate_url` 和 `max_sessions`）。

**停止 = `TaskService.Cancel(taskID)`，不设 `StopPush`**。理由：
1. 状态机、落库、`task:status`、应用退出（`Shutdown`）走的就是同一条取消路径，直播任务的 Runner 本来就是"取消 → 先发 `q`，最多等 5 秒（有本地存档的会话 15 秒，见 6.10）让 ffmpeg 收尾，超时再强杀"（6.5、6.6）；再包一层 `StopPush` 只会多一个和 `Cancel` 语义重复、还要保持同步的入口。
2. 任务中心、通知条等所有能看到任务的地方本来就有"停止"按钮，直播会话不用特殊处理。
3. 结果语义（前端文案要区分）：优雅停止成功 → Runner 返回 nil → 任务是 **`succeeded`**（不是 `canceled`；用户点"停止直播"是直播的正常结束）；**自然播完**（文件推流 `loop=false` 播到结尾）也是 `succeeded`。**架构师定：优雅停止和自然播完都显示"已结束推流"，前端不区分，也不加任何字段；** 点"停止"之后 5 秒内（有存档的会话 15 秒内）刷新页面看到 `running` 是可接受的（`Cancel` 立即返回，ffmpeg 还在收尾），前端以 `task:status` 为准。收尾超时被强杀 → `canceled`（"已强制停止"）。**硬性规则（架构师定）：优雅停止记 `succeeded` 时任务的 `error` 必须为空；强杀记 `canceled` 时同样不带错误码（`error` 为空）；前端只看 `status` 区分"已结束推流"（`succeeded`）和"已强制停止"（`canceled`），不看 `error`。**测试必须断言这两种终态的 `Task.error == nil`，且 `task:status` 载荷不带 `error`。**已请求 `Cancel` 之后，ffmpeg 无论怎样非零退出，一律归 `canceled`、不带错误码，不得落 `LIVE_PUSH_INTERRUPTED` 等**（细则见 6.10「错误分类」）。**优雅停止的判定（架构师定）**：发 `q` 后 ffmpeg **退出码 0 才是 `succeeded`**；已请求 `Cancel` 后退出码非 0 一律 `canceled`（无错误码）。**#31 已实现**（见 6.10.2 第 1 项）：`ffmpeg.Run` 原先在发 `q` 后的宽限期内只要进程退出就返回 nil、不看退出码（`internal/ffmpeg/exec.go` 的 `exitedGracefully` 分支，对转换类任务合理，对直播不行），现在有 `ffmpeg.RunOptions.StrictGracefulExit`，直播 Runner 开启后**直播路径检查退出码**实测（7.1.5 与 9.0.2 一致）：`q` 之后无存档 / 有存档（tee）都退出码 **0**（约 0.06~0.3 秒）；而 **SIGINT 之后退出码是 255**，所以直播必须用 `q`（经 stdin 管道）而不是 SIGINT，否则优雅停止会被判成 `canceled`。已结束的会话 `Cancel` 返回 `TASK_CONFLICT`，重复点击（正在停止中）返回 nil。
4. `Retry` 对直播任务返回 `UNSUPPORTED`（没有注册重试工厂，且 params 已脱敏、拿不到密钥）；前端"重新开始"就是用表单里的值再调一次 `Start*`。

**推流地址校验规则**（`Start*` 和 `CheckPushURL` 共用，不通过一律 `LIVE_URL_INVALID`；`message` 说明原因，**`detail` 第一行固定 `reason=<值>`（枚举见 2.2），整个 `detail` 和 `message` 都不带地址、口令或它们的片段**，绝不回显原文）：
1. 先 `TrimSpace`；长度 ≤ 2048 字节；不能含空白、控制字符、`|`、`\`、`"`、`'`（`|` 是 ffmpeg tee 分隔符，其余会破坏命令行 / 日志）。
2. scheme 不区分大小写，只允许 `rtmp`、`rtmps`、`srt`；其余（`file`、`http(s)`、`rtsp`、`udp`、`tcp`、`pipe`、`concat`、`subfile`、`data` ……）一律拒绝。传给 ffmpeg 的永远是校验后的 URL 并带 `-protocol_whitelist`，不会因为用户输入变成读本地文件或打开别的协议。（不在白名单 → `reason=scheme_unsupported`；没有 `://` → `malformed`）
3. host 不能为空；端口写了必须在 1~65535；IPv6 用方括号；IDN 主机名转 punycode，转不了就拒绝。**允许**回环 / 内网地址（推到本机或局域网的 nginx-rtmp、SRS、MediaMTX 是正常用法）。 host 为空 → `reason=missing_host`；端口越界、IPv6 括号错误、IDN 转换失败 → `reason=malformed`。
4. `rtmp` / `rtmps`：路径至少要有应用名（`rtmp://host/` 不合法）；流名可以在路径里，也可以在查询参数里。
5. `srt`：必须写端口（缺失 → `malformed`）；**查询参数白名单**（键先做一次 URL 解码并转小写再比较；不在白名单、同名重复、`mode` 不是 `caller` 都是 `reason=param_not_allowed`）：`passphrase`（长度 10~79，超出 → `malformed`）、`pbkeylen`（只能 0、16、24、32）、`streamid`（≤ 512 字符）、`latency`、`connect_timeout`、`maxbw`、`pkt_size`（≤ 1456）、`mode=caller`（数值参数必须是整数，范围由实现按 `ffmpeg -h protocol=srt` 校验；单位以 7.1.5 为准：`latency` 微秒、`connect_timeout` 毫秒、`maxbw` 字节/秒）。**传给 ffmpeg 的 URL 由后端按白名单重新组装：键统一写成小写解码后的形式，值原样保留。**原因（7.1.5 实测）：ffmpeg 对参数名区分大小写、也不做百分号解码——`?PASSPHRASE=abc` 和 `?pass%70hrase=abc` 都被**悄悄忽略**（不报错，等于没加密就推出去了），（**ffmpeg 9.0.2（项目默认安装版本）行为不同：大写参数名不再被忽略，而是报 `Query string option 'PASSPHRASE' does not exist` / `Option not found`，退出码 8**；所以同一个错误地址在 7.1.5 上静默不加密、在 9.0.2 上直接失败，**不能依赖 ffmpeg 兜底**，仍然必须先解码、小写化再按白名单重组 URL）；而 `?passphrase=abc`（3 位）会报 `failed to set option SRTO_PASSPHRASE … Bad parameters`、10~80 位都能通过、81 位又报错（**长度上限取 79 是架构师定，比 ffmpeg 实测多接受的 80 位保守一位，与 SRT 规范的 10~79 一致；测试断言 9、10、79、80 位的结果分别是 `malformed`、通过、通过、`malformed`**）；`mode=listener` 会让 ffmpeg 挂起等连接。所以：小写化 + 白名单 + 重新组装是必须的，不能"原样交给 ffmpeg"。
6. 通过校验的 URL 只在内存里用；标准化形式（scheme / host 小写、去掉默认端口）用来判断"同一个地址已有进行中的会话"。

**推流密钥 / 凭据脱敏**（规则、覆盖范围和测试要求见 6.10）：地址里的用户信息、rtmp / rtmps 的流名（应用名之后的路径）、所有查询参数的值一律显示成 `***`；标题、`params`、任务日志、错误的 `message` / `detail`、所有事件 payload、后端日志都只出现脱敏后的地址；完整地址不落库、不写文件。

**任务字段**：`type` 是 `live_file_push` / `live_screen_push`；`title` 如 `文件推流：a.mp4 → rtmp://host/app/***`、`屏幕推流：屏幕 1（主显示器） → srt://host:9000?streamid=***&passphrase=***`；`inputPaths` 文件推流为 `[inputPath]`、屏幕推流为 `[]`；`outputPath` 是本地存档的最终路径，没存档为 `""`；`params` 见 6.10。

**示例（JSON 数值是示意）**

`StartFilePush` 请求 / 返回（返回是入队前的快照）：
```json
{ "inputPath": "C:\\Videos\\a.mp4", "url": "rtmp://live.example.com/app/mystreamkey", "loop": true,
  "options": { "width": 1280, "height": 720, "fps": 30, "videoBitrateKbps": 2500, "audioBitrateKbps": 128 } }
```
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "type": "live_file_push", "status": "queued",
  "title": "文件推流：a.mp4 → rtmp://live.example.com/app/***",
  "inputPaths": ["C:\\Videos\\a.mp4"], "outputPath": "", "progress": -1, "speed": "", "etaSec": 0,
  "params": "{\"kind\":\"file\",\"input\":\"C:\\\\Videos\\\\a.mp4\",\"url\":\"rtmp://live.example.com/app/***\",\"loop\":true,\"options\":{}}",
  "version": 1, "createdAt": 1790000000000, "startedAt": 0, "finishedAt": 0 }
```

`StartScreenPush` 请求（带本地存档）：
```json
{ "url": "srt://live.example.com:9000?streamid=abc&passphrase=secret-pass-1", "screenId": "monitor:0",
  "hideCursor": false, "audio": "silent", "archiveDir": "C:\\Users\\me\\Videos\\FFmpegFree",
  "options": { "fps": 30, "videoBitrateKbps": 3000 } }
```

`task:progress`（无存档的直播会话；有存档时没有 `bitrateKbps`）：
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "version": 12, "progress": -1, "speed": "1.00x", "etaSec": 0,
  "outTimeSec": 83.4, "fps": 29.97, "bitrateKbps": 2431.5, "droppedFrames": 0 }
```

`task:status`（优雅停止成功，`error` 为空、不带 `error` 键）：
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "version": 15, "status": "succeeded",
  "outputPath": "C:\\Users\\me\\Videos\\FFmpegFree\\screen-20260929-203000.mp4", "finishedAt": 1790000090000 }
```


### TaskService
```go
type TaskFilter struct {
    Types    []TaskType   `json:"types"`    // 空 = 不过滤
    Statuses []TaskStatus `json:"statuses"` // 空 = 不过滤
    Limit    int          `json:"limit"`    // 默认 50，最大 200
    Offset   int          `json:"offset"`
}
type TaskPage struct {
    Items []Task `json:"items"`  // 按 createdAt 倒序；无结果时是 []
    Total int64  `json:"total"`  // 符合过滤条件的总数，用于分页
}

ListActive() ([]Task, error)                // 全部 queued + running，供 store 启动用
List(filter TaskFilter) (TaskPage, error)   // 按类型、状态、分页，供任务中心历史用
Get(id string) (Task, error)
Cancel(id string) error
Retry(id string) (Task, error)              // 用 Params 重新提交，生成新任务
Remove(ids []string, deleteOutput bool) error
ClearFinished() error
GetLog(id string, tailLines int) (string, error)
```

## 5. 事件（runtime.EventsEmit / EventsOn）

| 事件名 | payload | 频率 |
|---|---|---|
| `task:created` | `Task` | 每次 |
| `task:progress` | `{ id, version, progress, speed, etaSec, outTimeSec, fps?, bitrateKbps?, droppedFrames?, encoder?, encoderDevice?, hwFallback?, hwFallbackReason? }`（fps / bitrateKbps / droppedFrames 只有直播任务才有，见下；后四项 v0.18，与 `Task` 同名字段一致，见 9.7） | 每任务最多 4 次/秒 |
| `task:status` | `{ id, version, status, error?, outputPath?, startedAt?, finishedAt?, encoder?, encoderDevice?, hwFallback?, hwFallbackReason? }`（后四项 v0.18，见 9.7） | 状态变化时 |
| `task:removed` | `{ ids: string[] }` | 每次 |
| `ffmpeg:status` | `FFmpegStatus`（见第 9 节） | 检测完成、安装状态变化时 |

**直播指标（v0.10，取代 `live:stats`）**：直播任务的 `task:progress` 除 `speed`（如 `1.00x`，持续明显小于 1 说明编码跟不上）和 `outTimeSec`（已输出的媒体时长）外，还带 `fps`（当前输出帧率）、`bitrateKbps`（**只有没有本地存档的会话才有**：**近 5 秒**平均输出码率，由 ffmpeg `total_size` 和 `out_time` 的增量算出，不用 ffmpeg 自带的 `bitrate=`，那是从开始到现在的累计平均）、`droppedFrames`（ffmpeg 累计丢帧，不是网络丢包）；**有存档的会话没有 `bitrateKbps`（架构师定）**（7.1.5 实测：tee 下 `-progress` 的 `total_size` 和 `bitrate` 恒为 `N/A`，没有可用来源；**不轮询存档文件大小来补**——文件大小含音视频分片和 moov 开销、且不是网络那一路的码率，补出来的数是误导），该字段一律省略，前端显示"—"；`fps` / `droppedFrames` / `speed` / `out_time_us` 在 tee 下正常；`progress` 恒为 -1，`etaSec` 为 0。没有单独的 `uptimeSec`：已推时长 = 现在 − `Task.startedAt`（墙钟），`outTimeSec` 是媒体时间，两者差距变大说明卡顿。这几项同时写进 `Task`（`fps` / `bitrateKbps` / `droppedFrames`，只在内存），页面刷新后 `ListActive` 能立刻显示当前值。

`task:created` 后任务状态为 `queued`；开始执行时发 `task:status`（`running`）；结束时发 `task:status`（终态）。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

`task:status` 的时间字段（Unix 毫秒，值为 0 时省略）：`running` 事件带 `startedAt`、不带 `finishedAt`；所有终态事件（`succeeded` / `failed` / `canceled` / `interrupted`）都带 `finishedAt`，跑过的任务同时带 `startedAt`（与 `Task.startedAt` / `Task.finishedAt` 及落库值一致）。任务从未进入 `running` 就结束（排队中被取消、应用退出时还在排队而被标记为 `interrupted`）时没有 `startedAt`：事件里省略该字段，`Task.startedAt` 为 0，这是正常的，前端不应把它当作错误。崩溃恢复（启动时把残留的 `queued` / `running` 置为 `interrupted`）只落库（写入 `finishedAt`，保留已有的 `startedAt`，`version` +1），不发事件；前端启动后通过 `ListActive` / `List` 拿到最新记录。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

前端任务 store 规则：先 `EventsOn` 订阅并缓存事件，再 `TaskService.ListActive()` 拉取 queued 和 running 任务，拉完按 `version` 回放缓存，版本不大于本地的事件直接丢弃。历史任务只在任务中心里用 `List` 分页加载。`task:progress` 只改进度字段，不替换对象。

## 6. SQLite 表

```sql
media(id PK, path, path_key UNIQUE, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at)
tasks(id PK, type, status, title, input_paths JSON, output_path, params JSON, progress, error JSON,
      log_path, version, created_at, started_at, finished_at,
      encoder, encoder_device, hw_fallback, hw_fallback_reason)   -- 后四列：迁移 0004（v0.18），见 9.7；旧行为空 / 0
presets(id PK, name, built_in, options JSON, sort)
edit_projects(id PK, name, project JSON, updated_at)
doc_recent(id PK, path, path_key UNIQUE, name, size, opened_at)
settings(key PK, value JSON)
schema_migrations(version PK, applied_at)
```
启动时把 `status in (queued, running)` 的任务改为 `interrupted`。

## 6.5 任务管理器实现约定

- 通用接口，不绑定 ffmpeg：
  ```go
  type Runner interface {
      Run(ctx context.Context, report func(p Progress)) (outputPath string, err error)
  }
  ```
  ffmpeg 类任务用 `FFmpegRunner`，Office 转 PDF 用 `GoFuncRunner`，ffmpeg 下载用 `DownloadRunner`。
- 两个调度池：`batch` 池（转换、剪辑、Office、ffmpeg 下载）按设置里的并发数排队；`live` 池（直播任务：`live_file_push`、`live_screen_push`）不排队、不占 batch 名额。
- 两遍编码 / 目标大小压缩：暂缓（v0.7.2），设计保留：每个任务用 `-passlogfile <任务专属临时目录>/pass`，进度第一遍 0~0.5，第二遍 0.5~1，结束后删临时目录。
- 输出文件先写 `<name>.part.<原扩展名>`（例如 `a.part.mp4`，保留扩展名让 ffmpeg 能识别封装格式），成功后改名；目标重名时自动追加 `(1)`、`(2)`；取消或失败删除 `.part`。
- 进度只保存在内存并通过 `task:progress` 推送，不写库；只有状态变化（开始、成功、失败、取消）时落库，避免单连接下进度写入阻塞任务中心的列表查询。
- 取消转换类任务直接强制结束进程；直播录制存档要先向 ffmpeg 发 `q`（不用 SIGINT，见 6.10），等待它写完文件尾（无存档的直播 5 秒，有存档的 15 秒，见 6.10），超时再强制结束；存档用分片 mp4，强杀后已写出的分片仍可播放，**直播存档强杀后保留**（直接写最终文件名，不走 `RunWithPart`，6.10 实测）。
- `/local/<token>` 用 `http.ServeContent` 输出，支持 Range 请求，保证视频可拖动进度。

## 6.6 任务管理器实现约定（v0.7）

- 包 `internal/task`：`Manager.Submit(Spec, Runner)` 落库为 `queued` 并发 `task:created`；`batch` 池（`internal/ffmpeg` 转换 / 剪辑 / Office / 安装）按并发数 FIFO 排队，默认并发 `min(NumCPU/2, 3)` 且至少 1；`live` 池（两类直播）不排队、不占 batch 名额，且进度恒为 -1。
- 状态机：`queued → running → succeeded | failed | canceled | interrupted`。Runner 返回 nil 即 `succeeded`（含直播优雅停止：存档完整；直播存档在强杀 / 失败 / 中断后也保留，见 6.10）；返回被取消的错误且用户请求过取消为 `canceled`；应用退出时被停止的任务（含还在排队的）为 `interrupted`；其余为 `failed`（`error` 带错误，ffmpeg 失败时 `detail` 为 stderr 最后 50 行）。
- 只有状态变化落库；进度只在内存。`task:progress` 同一任务最多 4 次/秒，被节流抑制的最后一次会在间隔到期后补发。`ListActive` / `Get` / `List` 返回运行中任务时带实时进度，`Speed` / `EtaSec` 不落库。
- 取消：排队中的直接移出队列变 `canceled`；运行中的取消 `ctx`，ffmpeg 任务结束整个进程组；直播任务发 `q`（不用 SIGINT，见 6.10），最多等 5 秒（有本地存档的直播会话 15 秒；还没连上、没有收到第一条 progress 的会话直接强杀，不发 `q`，见 6.10）再强制结束。已结束的任务取消返回 `TASK_CONFLICT`，不存在返回 `NOT_FOUND`；**旧类型（"保留但不再产生"的类型）的 id 按不存在处理，返回 `NOT_FOUND`**（6.10 确认项 ⑧）。Windows 上结束整个进程树（v0.9.2：先终结进程所在的 Job Object，失败退回 `taskkill /T /F`，再失败只结束主进程）。
- **`task.NeverRanner`（v0.19 补写，描述现有行为）**：`Runner` 可选实现的接口 `NeverRan() string`。**什么时候用**：任务在 `Run` 根本没有执行的情况下就结束时，管理器在发终态事件**之前**调用一次 `NeverRan`，返回值当作该任务的输出路径（含义与 `Run` 的返回值相同：`""` = 保留提交时的预期路径；`task.ClearOutputPath` = 把 `outputPath` 清空；绝对路径 = 采信；相对路径忽略）。目前**只有直播屏幕推流的存档 Runner**（`archiveRunner`）实现它：删掉自己创建的 0 字节占位文件并返回 `ClearOutputPath`；其余 Runner 都没实现，行为是保留预期路径（旧行为）。
  - **触发场景（`Run` 没有执行）**：排队中被 `Cancel`（`canceled`）；`Submit` 与应用退出并发、或在 `task:created` 与入队之间被取消（`interrupted` / `canceled`）；应用退出时还在排队（`interrupted`）；刚出队但 ctx 已被取消（`canceled`，应用正在退出时按“不是用户取消”规则为 `interrupted`）。直播任务不排队，只会走最后两种（刚提交就被取消）。
  - **状态与事件**：终态只可能是 `canceled` 或 `interrupted`，**不会是 `failed` / `succeeded`**，`error` 为空。**没有 `running` 事件、没有任何 `task:progress`**；终态 `task:status` **不带 `startedAt`**（`Task.startedAt` 为 0，事件里省略），带 `finishedAt`；`Task.progress` 保持提交时的值（非直播 0，直播 -1，不会因终态变成 1）。终态落库、发 `task:status` 后调用 `OnFinish`（`Finalizer`）。这类任务从未启动 ffmpeg，也没有 `.part` 文件；日志文件可能不存在（`GetLog` 返回空）。
  - **注意（非直播的例外）**：“刚出队但 ctx 已被取消”这条路径上，非直播任务失败 / 取消时管理器一律丢弃 Runner 返回的输出路径（沿用旧行为，见 `finishAfterRun`），所以 `NeverRan` 的返回值只对直播任务在这条路径上生效；前四种场景对所有类型都采信。
  - **与 v0.18 编码器字段的关系**：`encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 由 `Submit` 在任务落库前从 Runner 的 `EncoderReporter` 一次性写入，而 `NeverRan` 只影响输出路径，所以**从未运行的任务上这四个字段是提交时解析出来的值，不是空**（例如排队中被取消的 h264 转换任务带 `libx264` / `cpu`，所选设备当时不可用的带 `hwFallback=true`、`device_unavailable`；实现了 `EncoderReporter` 才有，纯音频转换、Office 转 PDF、ffmpeg 安装这类没有视频编码器的任务本来就为空）。因为 `Run` 没执行过，**不会发生运行中的硬件编码回退**，不会补发 `running` 事件，所以字段不会再变。前端不要把“这四个字段有值”理解为“这个任务真的编码过”，要看是否有 `startedAt`。
- `Retry`：用原任务的 `type` / `params` / `title` / `inputPaths` 重新提交，生成新任务（原任务保留）；原任务仍在进行返回 `TASK_CONFLICT`。每个任务类型注册一个 Factory 才支持重试（目前只有 `ffmpeg_install`），没有 Factory 的返回 `UNSUPPORTED`；**旧类型（"保留但不再产生"的类型）的 id 先判为 `NOT_FOUND`，不落 `UNSUPPORTED`**（6.10 确认项 ⑧）。
- `Remove(ids, deleteOutput)`：任一 id 仍在进行则整体失败（`TASK_CONFLICT`）；删除记录与日志，`deleteOutput=true` 时删除成功任务的输出文件（仅当输出路径是绝对路径、所在目录及上级不含符号链接、且是普通文件；否则只删记录并在日志里说明）；不存在的 id 忽略；**但 ids 里有旧类型（"保留但不再产生"的类型）记录的 id 时整体返回 `NOT_FOUND`、不删任何记录**（6.10 确认项 ⑧，"不存在的 id 忽略"的例外）；发 `task:removed`。`ClearFinished` 只删记录和日志，不删输出。
- 日志：`<数据目录>/logs/<任务ID>.log`（单个任务最多 16 MB：写满 8 MB 轮转为 `.log.1`，单行最多 8 KB 超出截断），Runner 通过 `task.LogWriter(ctx)` 写入，`GetLog(id, tailLines)` 读取末尾若干行（最多读末尾 1 MB）。
- 输出文件用 `task.RunWithPart`：选出不冲突的最终路径（重名追加 `(1)`、`(2)`），写 `<name>.part.<原扩展名>`，成功后改名，失败或取消删除 `.part`。**例外：直播屏幕推流的本地存档不走 `RunWithPart`**（分片 mp4 直接写最终文件名，强杀 / 失败后保留，见 6.10）。
- `task.FFmpegRunner` + `ffmpeg.Run` 是 ffmpeg 任务的通用执行体：自动加 `-hide_banner -nostats -y -progress pipe:1`（不需要 stdin 时再加 `-nostdin`；直播优雅停止和外部 stdin 的任务不加），解析 `out_time_us` / `speed` / `fps` / `bitrate` / `progress=end`，保留 stderr 尾部，提供错误分类钩子（直播的 `LIVE_*` 分类由直播 PR 提供）；`FFmpegRunner` 目前只支持单次 ffmpeg 调用（两遍编码暂缓，见 v0.7.2），`ProgressBase` / `ProgressScale` 用来把一次调用的进度映射到任务整体进度区间（供以后多步骤任务使用）。
- **子进程回收（v0.9.2）**：所有 ffmpeg / ffprobe 子进程经 `proc.Start` / `proc.Run` 启动。Windows 上会为每个子进程创建一个 Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）并把进程放进去，Job 句柄由应用进程持有：应用崩溃 / 被任务管理器结束时系统关闭句柄，ffmpeg 和它派生的进程一起被系统结束；进程正常退出后应用关闭句柄（同样结束遗留的子孙）。创建 / 加入 Job 失败不影响启动，此时结束进程树走 `taskkill /T /F`。macOS / Linux 不变（`Setpgid` 进程组；应用崩溃时 ffmpeg 不会被自动回收，读不到 stdin / 管道时通常会自己退出）。`proc.Start` 到加入 Job 之间有极短窗口，窗口内派生的孙进程不进 Job（ffmpeg 启动时不会立刻派生）。**没有 Windows 真机验证**，只有交叉编译和 Linux 上的回退顺序 / 登记表测试，Windows 专属测试文件已写但未运行。
- 启动：`store.MarkInterrupted` 在打开数据库后立即执行，上次未结束的 `queued` / `running` 变 `interrupted`，**不会自动恢复执行**，用户可在任务中心点重试。退出：`Manager.Shutdown` 取消所有任务并等待收尾（最多 8 秒；存在有本地存档的直播会话时最多 16 秒，见 6.10；等待期间前端显示"正在停止…"，超时走强杀），再关闭数据库。**已由 #31 实现**（见 6.10.2 第 3 项）：`app.go` 的 `shutdown` 在有带存档的直播会话时等 16 秒，否则 8 秒。

## 6.7 媒体探测与缩略图实现约定（v0.8）

- 门控：`ffmpeg.RequireProbe()`；缺 ffmpeg 或 ffprobe 时整个调用返回 `FFMPEG_NOT_FOUND`。
- 路径：经 `paths.Normalize`；文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读取权限 `IO_ERROR`，ffprobe 解析失败 / 无流 / 超时（30 秒）`PROBE_FAILED`（detail 是 ffprobe stderr 最后 50 行）。所有 ffmpeg / ffprobe 输入都写成 `file:<路径>`，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。
- `Probe`：`-v error -print_format json -show_format -show_streams`；`width` / `height` 是**显示尺寸**（按 `rotation` 90 / 270 交换），各流的编码尺寸在 `streams[]`；`rotation` 为逆时针角度，取自 display matrix（兼容旧 `rotate` 标签）；`duration` / `bitrate` 缺失（`N/A`）时用各流的值；封面图（`attached_pic`）不算视频画面；`fps` 取 `avg_frame_rate`，为 0 时用 `r_frame_rate`，保留 3 位小数。同一文件（同 `path_key`）再次探测保留原 `id`。失败的文件不入库。最多 4 个文件并行。
- `Thumbnail`：jpg，最大宽度 `width`（默认 320，范围 16~1920，不放大，高度自动取偶数），按旋转元数据转正；`atSec` 超出时长时退回第 0 秒；没有视频画面返回 `INVALID_ARGUMENT`。同时返回缓存文件路径和 data URL。同时最多 2 个 ffmpeg 在生成；相同参数并发只生成一次。
- 缓存：`<数据目录>/thumbs/<sha1(path_key, mtime, size, atSec 毫秒, width)>.jpg`，先写 `.part.jpg` 再改名；文件的修改时间或大小变化即失效。容量上限 1000 个文件或 200 MB（超过则按最后使用时间从旧到新删到上限的 80%），启动时清理一次并每生成 50 张清理一次，超过 1 小时的 `.part.jpg` 残留会被清掉。`RemoveRecent` 不删缓存。
- `Probe` 附带的默认缩略图取时长的 10%（最多 10 秒），宽 320；`ListRecent` 只在该缩略图已缓存时返回 `thumbUrl`，不会为此启动 ffmpeg。

### 6.7 补充（v0.8.1，媒体服务评审修订）

- **ctx**：`App` 持有根 ctx（`NewApp` 时创建，`shutdown` 第一步取消），`MediaService.Probe` / `Thumbnail` / `ListRecent` / `RemoveRecent` 都用它；应用退出时进行中的 ffprobe / ffmpeg 被结束（`ffmpeg.NewCommand` 设了 `cmd.Cancel = proc.Kill`，取消 / 超时结束整个进程组）。
- **批量**：`Probe` 最多 500 个，超过 `INVALID_ARGUMENT`；**前端应把大批量拆成每批约 50 个**再调用（界面能更快出结果，也方便取消）。`RemoveRecent` 最多 500 个 id，超过 `INVALID_ARGUMENT`。
- **文件类型**：先 `os.Stat` 再打开；目录、FIFO、设备文件、socket 等非普通文件返回 `INVALID_ARGUMENT`（不会阻塞）。只有字幕 / 数据流（如 .srt）、或只有封面图（`attached_pic`）没有真正音频 / 视频流的文件返回 `PROBE_FAILED`；封面图 + 音频（带封面的 mp3）仍是有效的音频文件（`hasVideo=false`），对它直接调 `Thumbnail` 返回 `INVALID_ARGUMENT`（截图 `-map 0:V:0` 排除封面图）。
- **Thumbnail**：`atSec` 上限 1e7 秒（更大的按 1e7 处理，防止毫秒换算溢出）；`atSec` 超出视频长度退回第 0 秒时，缓存文件名和返回值的 `atSec` 都是 `0`（`Thumb.atSec` 永远是实际截图的时间点）；**超时不再重试**（只有"超出长度"这种 ffmpeg 失败才退回第 0 秒重试一次）；缓存被清理线程在"命中检查"和"读取"之间删掉时按未命中处理并重新生成，不返回错误。
- **输出上限**：ffprobe stdout 最多 16 MB（超过 `PROBE_FAILED`，message 为"文件的元数据太大"），stderr 只保留最后 256 KB。
- **`-pattern_type none`**：文件名带 `%` 且是 image2 支持的图片扩展名（jpg / png / bmp / webp / tiff / ppm 等，如 `a%03d.png`）时，在 `-i` 之前加 `-pattern_type none`，避免被当成序列模板；其他文件不加（ffmpeg 7.1 实测 mp4 / gif 等解封装器不认识这个选项，加了会报 "Option pattern_type not found"）。
- **media 表**：每次 `UpsertMedia` 之后只保留最近 1000 条（按 `probed_at` 倒序），更旧的记录被删除（缩略图缓存由自己的容量上限回收）。

## 6.8 RevealInFolder / PickDirectory（v0.7.3，v0.7.7 修订）

- `RevealInFolder(path)`：path 必须是绝对路径（空 / 相对路径 → `INVALID_ARGUMENT`），必须存在（否则 `NOT_FOUND`）。**范围限制（v0.7.7）**：只允许任务表里登记的输出路径（`Manager.IsTaskOutput`），或 `defaultOutputDir` 之内的路径（目录本身可以）；先 Clean 再 `EvalSymlinks`，用真实路径按目录边界比较（Windows / macOS 不区分大小写），其余一律 `INVALID_ARGUMENT`；任务输出本身是符号链接时拒绝；实际打开的是真实路径。Windows 执行 `explorer /select,<path>`，macOS `open -R <path>`，Linux `xdg-open <所在文件夹>`；path 是文件夹时三个平台都直接打开这个文件夹。命令启动后立即返回，启动失败 `PROCESS_FAILED`。Linux 没有统一的"选中文件"方式，所以只能打开所在文件夹。
- `PickDirectory(title string)`：弹出系统选择文件夹对话框，返回绝对路径；取消返回 `""`。应用启动完成前调用返回 `INTERNAL`。**参数不能省略**：Wails v2.11 对 Go 可变参数生成 `Array<string>` 且运行时按参数个数严格检查，做不了可选参数，前端无标题时调用 `PickDirectory('')`。
- `PickFiles(filter, multiple)`：`filter = {name, patterns[]}`，patterns 形如 `["*.mp4", "*.mkv"]`（单个元素里用分号也行：`"*.mp4;*.mkv"`），只接受 `*.扩展名` 形式（扩展名限字母数字 `_ - + ? *`）和 `*` / `*.*`，其他写法 `INVALID_ARGUMENT`；patterns 为空或含 `*.*` = 不过滤；`name` 为空时用模式串当显示名。返回绝对路径（已 `Clean`、去重）；**用户取消返回空数组 `[]`，不是错误**；`multiple=false` 最多 1 个。启动完成前调用返回 `INTERNAL`。

## 6.9 ConvertService 实现约定（v0.9）

- 绑定：`ListPresets() []Preset`、`SavePreset(p Preset) Preset`、`DeletePreset(id)`、`Submit(inputs []string, opts ConvertOptions, outputDir string) []Task`。均返回 `INTERNAL`（"转换服务尚未初始化"）直到启动完成。
- **预设**：12 个内置（`builtIn=true`，id 形如 `builtin-mp4-h264`：MP4 H.264 / 1080p / 720p / H.265、MKV 只换封装、MOV、WebM VP9、GIF、MP3、M4A、WAV、FLAC），不能修改或删除（`INVALID_ARGUMENT`，改为"另存为"：`id` 传空新建）。`SavePreset`：`id` 空 = 新建（后端生成 id），否则更新该用户预设（不存在 `NOT_FOUND`）；名称去首尾空白后 1~60 字；`options` 必须通过校验；用户预设最多 100 个；`builtIn` 字段传什么都忽略。`ListPresets` 内置在前，用户预设按创建顺序在后。
- **容器与编码**：容器 `mp4 mkv mov webm avi flv gif mp3 aac m4a wav flac ogg opus`；视频编码 `copy h264 h265 vp9`，`""` 表示不要视频（仅对视频容器有意义，等于只保留音频）；音频编码 `copy aac mp3 opus vorbis flac pcm ac3 none`，`""` 取容器默认（mp4/mov/mkv/flv=aac，webm=opus，avi=mp3，mp3/aac/m4a/wav/flac/ogg/opus 对应各自编码），`none` 去掉音轨。每个容器只允许合适的编码组合，不合法返回 `INVALID_ARGUMENT` 并在 message 里列出可选项。音频容器不能设分辨率/帧率、不能 `none`；`gif` 没有音轨、不能设码率；`copy` 不能缩放、不能设 crf/码率。
- `width` / `height`：只给一个按比例缩放；两个都给时等比缩进该框内并补黑边；输出宽高保证是偶数。`trimStart` / `trimEnd`（秒，`trimEnd=0` 到结尾）；开始时间超过时长返回 `INVALID_ARGUMENT`。`targetSizeMb` 暂缓，>0 返回 `INVALID_ARGUMENT`。**取值范围**（越界一律 `INVALID_ARGUMENT`）：`trimStart` / `trimEnd` ≤ 1e6 秒；`fps` 为 0（保持）或 0.1~240；`videoBitrate` 0 或 ≤ 1e9 bit/s；`audioBitrate` 0 或 8000~1000000 bit/s；`width` / `height` ≤ 8192。
- **Submit**：一个文件一个 `convert` 任务（batch 池按并发数排队），返回的任务与 `inputs` 一一对应；任务 `title` 形如 `a.mkv → MP4`，`inputPaths` 为源文件，`outputPath` 为预期输出（完成时以实际路径为准，重名会带 `(n)`），`params` 是 `{input, options, outputDir}` 的 JSON。
  - 一次最多 **50** 个文件（更多由前端分批），空列表 `INVALID_ARGUMENT`。
  - **先整体校验再提交**：ffmpeg 就绪（否则 `FFMPEG_NOT_FOUND`）、参数合法、每个文件都能被探测（`NOT_FOUND` / `PROBE_FAILED` / 目录 `INVALID_ARGUMENT`）且与参数兼容（如无音轨转 mp3、无画面转视频容器 → `INVALID_ARGUMENT`）。任何一个不通过整体失败，**不提交任何任务**，错误 `detail` 第一行是出错的文件路径。视频输入没有音轨转视频容器不算错误，静默不带音频。
  - **输出目录**：`outputDir` 非空用它；为空用 `Settings.defaultOutputDir`；仍为空则输出到各自源文件所在文件夹。必须是绝对路径（`INVALID_ARGUMENT`），不存在的目录会在任务开始时创建。输出名为 `<源文件名去扩展名>.<新扩展名>`，重名依次追加 `(1)`、`(2)`，绝不覆盖已有文件（含输入文件本身），走 6.6 的 `.part` + 原子改名。
  - 进度按输出时长（已扣除裁剪）计算，0~1 单调递增，完成为 1；取消后没有最终文件也没有 `.part`。
- **任务失败的错误码**（在任务的 `error` 上）：`CONVERT_DISK_FULL`（No space left on device / ENOSPC / Windows "There is not enough space on the disk" / 磁盘配额）、`IO_ERROR`（没有权限、只读、找不到路径）、`PROBE_FAILED`（输入损坏）、`PROCESS_FAILED`（缺少编码器或滤镜、编码与格式不兼容，detail 带 ffmpeg 最后 50 行）。
- **Retry**：`convert` 任务注册了重试工厂，用 `params` 重建：重新探测输入、重新校验参数（输入已删除返回 `NOT_FOUND`，不产生新任务），输出目录用原来解析好的那个（之后改 `defaultOutputDir` 不影响）。
- **错误分类的匹配范围**（v0.9.1）：只看 stderr 尾部里去掉 `Input #` / `Output #` / `Stream mapping:` 及其缩进的元数据、`Stream #`、`Metadata:`、`Duration:` 行之后剩下的行；系统错误文本按行尾匹配；`CONVERT_DISK_FULL` 还要求同时出现写入阶段句式（`Error writing trailer`、`Error muxing packet`、`Error while writing`、`av_interleaved_write_frame`、`av_write_frame`）。所以文件名 / 标题带 `ENOSPC`、`end of file`、`permission denied` 等词不会误分类。`RunWithPart` 在创建输出目录、硬链接 / 重命名提交时遇到磁盘满（Unix ENOSPC / EDQUOT，Windows 112 / 39）也返回 `CONVERT_DISK_FULL`，其余系统错误 `IO_ERROR`。
- **提交阶段失败会保留已提交的任务**：`Submit` 校验全部通过之后才开始逐个提交；若中途某个 `Submit` 失败（例如任务管理器出错），返回值里带着已成功提交的任务列表和错误，这些任务**不回滚**，会照常运行。ctx 被取消时返回 `CANCELED`。
- 输入是文件名带 `%` 的图片（如 `a%03d.jpg`）时，命令里在 `-i` 前加 `-pattern_type none`（与缩略图 / 探测同一规则，只对 image2 图片扩展名），避免被当成序列模板。
- 所有 ffmpeg 输入输出路径都带 `file:` 前缀，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。

## 6.10 LiveService 实现约定（v0.10 设计稿，尚未实现）

- **命令行**（`task.FFmpegRunner{Live: true}`，`GracefulStop`，不加 `-nostdin`，由 stdin 发 `q`）：
  - 文件推流：参数顺序 `-protocol_whitelist file [-re] [-stream_loop -1] -i file:<path> [-protocol_whitelist file -f lavfi -i anullsrc=r=44100:cl=stereo] -map 0:v:0 -map <0:a:0 或 1:a> <编码参数> [-shortest] -protocol_whitelist <输出侧白名单> -f <flv|mpegts> [-flvflags no_duration_filesize] <url>`（输出侧命令模板：**RTMP / RTMPS 带 `-flvflags no_duration_filesize`，SRT 不带**，见本条末尾和下面的白名单条）。源文件没有音轨时补静音（很多服务器要求有音频）；始终重编码：`-c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -b:v <k>k -maxrate <k>k -bufsize <2k>k -g <2×fps> -c:a aac -b:a <k>k -ar 44100 -ac 2`；`-re` 让 ffmpeg 按源速度读文件；`rtmp(s)` 用 `-f flv`，`srt` 用 `-f mpegts`。**RTMP / RTMPS 输出（不经 tee 时，文件推流和无存档的屏幕推流一致）额外加 `-flvflags no_duration_filesize`**（与已合并的 #31 实现一致：`internal/ffmpeg/live_args.go` 的 `BuildFilePushArgs`、`live_screen_args.go` 的 `BuildScreenPushArgs`，`Scheme != "srt"` 时加）：它只让 flv 头里不写 `duration` / `filesize` 两个占位为 0 的元数据，**只影响 flv 头，对推流无害**（ffmpeg 7.1.5 与 9.0.2 实测：`-protocol_whitelist rtmp,tcp -flvflags no_duration_filesize -f flv rtmp://127.0.0.1:1935/live/x` 推到 MediaMTX 1.21.1 退出码 0，写本地 flv 文件也退出码 0）；**SRT（`-f mpegts`）不加**。完整的 RTMP / RTMPS 输出侧命令串：`-protocol_whitelist rtmp,tcp -f flv -flvflags no_duration_filesize <url>`（RTMPS 的白名单是 `rtmps,tcp,tls,crypto`）；SRT：`-protocol_whitelist srt,udp -f mpegts <url>`。**走 tee 的存档会话**（本条之外，尚未实现，随存档一起做）以上文存档段的选项串为准，本条不改它（tee 下是否加该选项未实测，不加）。
  - **补 `anullsrc` 时必须加 `-shortest`**（架构师定；7.1.5 实测）：3 秒的无音轨源 + `anullsrc`，不加 `-shortest` 跑了 12 秒还没结束（被 `timeout` 杀掉，`anullsrc` 是无限流），加了 0 秒结束、退出码 0；`-re` 播 20 秒的源，加 `-shortest` 后 20 秒自然结束、退出码 0（任务 `succeeded`）；`loop=true` 时 `-stream_loop -1` 配 `-shortest` 仍然无限（实测 8 秒仍在运行），符合"循环直到用户停止"。源文件有音轨时不补 `anullsrc`，也不加 `-shortest`。
  - **`-protocol_whitelist` 按输入和输出分别写**（架构师定；7.1.5 实测）：这个选项**按位置生效**——写在某个 `-i` 前面只管那个输入，写在所有输入之后、输出之前只管输出。① **输入侧必须放行 `file`**：文件输入前写 `-protocol_whitelist file`；如果全局写成 `rtmp,tcp`，`file:` 输入直接失败（`Protocol 'file' not on whitelist 'rtmp,tcp'!`，退出码 234）。`anullsrc` 输入前也写 `file`（无害）；屏幕采集输入（gdigrab / avfoundation / x11grab）不涉及 URL 协议，不需要写（x11grab 前写 `file` 实测无害）。② **输出侧必须另写一次**（#31 实现确认：RTMP 输出侧 **`-protocol_whitelist rtmp,tcp`** 必须写，不得省略），只写输入侧时输出**不受限制**（实测 rtmp 输出照常打开），所以输出侧再限：无存档时 `-protocol_whitelist rtmp,tcp -f flv -flvflags no_duration_filesize <url>` / `-protocol_whitelist rtmps,tcp,tls,crypto -f flv -flvflags no_duration_filesize <url>` / `-protocol_whitelist srt,udp -f mpegts <url>`（SRT 不加 `-flvflags`）（少写 `tcp` 会报 `Protocol 'tcp' not on whitelist 'rtmp'!`，退出码 234）。③ **走 `tee` 时输出侧的 `-protocol_whitelist` 对 slave 无效**（实测：输出侧只放 `file`，rtmp slave 仍能连上），所以要把白名单写进每个 slave 的选项里：`[...:protocol_whitelist=rtmp,tcp]`、存档一路 `[...:protocol_whitelist=file]`（实测 slave 里的限制有效；值里的逗号不用转义）。rtmps 的 `rtmps,tcp,tls,crypto` 只验证了"通过白名单并走到连接被拒"，没有 TLS 服务器可以推流，**rtmps 端到端未验证**。
  - 屏幕推流输入：Windows `-f gdigrab -framerate N [-draw_mouse 0] [-offset_x X -offset_y Y -video_size WxH] -i desktop`；macOS `-f avfoundation -framerate N -capture_cursor 0|1 -i "<设备序号>:none"`；Linux `-f x11grab -framerate N -draw_mouse 0|1 [-video_size WxH] -i <DISPLAY>+<x>,<y>`。输出尺寸补偶数（`scale=trunc(iw/2)*2:trunc(ih/2)*2`），编码参数同上；无存档时输出侧与文件推流完全一致（`-protocol_whitelist <输出侧白名单> -f <flv|mpegts>`，**RTMP / RTMPS 加 `-flvflags no_duration_filesize`，SRT 不加**）；`audio=silent` 时加 `anullsrc`（屏幕采集是无限流，不需要 `-shortest`；7.1.5 + Xvfb + x11grab + `anullsrc` 端到端实测 `q` 后退出码 0、存档可完整解码，含 h264 + aac）。
  - **存档（`archiveDir` 非空，只有屏幕推流）**：用 ffmpeg `tee` muxer 一次编码同时写两个输出，并加 `-flags +global_header`：`-flags +global_header -f tee "[f=flv:onfail=abort:protocol_whitelist=rtmp,tcp]<url>|[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]file:<存档路径>"`（SRT 时网络一路是 `[f=mpegts:onfail=abort:protocol_whitelist=srt,udp]`；URL 已经保证不含 `|`；`<url>` 和 `<存档路径>` 都要按下面的转义规则处理）。**存档不走 `RunWithPart`**（架构师定）：分片 mp4 **直接写最终文件名**（没有 `.part`、没有成功后改名），**无论 `succeeded` / `canceled`（强杀）/ `failed` / `interrupted` 都保留**（唯一例外见第 2 条：没有任何可播放分片的空壳文件会被删掉）；`outputPath` = 存档最终路径（空壳被删时清空）。逐项（ffmpeg 7.1.5 + MediaMTX 1.21.1 实测）：
    1. **网络那一路必须写 `onfail=abort`**（架构师定）：`tee` 默认 `onfail=continue`，推流地址连不上时 ffmpeg 只打一行 `Slave muxer #0 failed: Connection refused, continuing with 1/2 slaves.`，**退出码 0，存档照写**（实测存档 79 834 字节，任务会变成 `succeeded`）。加 `onfail=abort` 后：RTMP / RTMPS 连接被拒退出码 **145**（stderr 有 `[tee @ …] Slave '…': error opening: Connection refused`、`Slave muxer #0 failed, aborting`），没有存档文件；SRT 连不上退出码 **251**；RTMP 鉴权失败退出码 **255**（`Server error: authentication failed`，判 `LIVE_PUSH_REJECTED`）；推流中途服务器被杀退出码 **224**（`Broken pipe`）。这些行分类为 `LIVE_CONNECT_FAILED` / `LIVE_PUSH_REJECTED` / `LIVE_PUSH_INTERRUPTED` 的规则不变。存档那一路**也写 `onfail=abort`**（写盘失败要让整个任务失败；命令串和文字统一为 `abort`，不再有"不写 onfail"的说法）。**ffmpeg 9.0.2 复测**：`onfail=abort` 下 RTMP 连接被拒同样退出码 145、没有存档文件，带 `onfail=abort` 的 tee 命令（含存档 slave）正常推流退出码 0；另外发现 **9.0.2 里即使不写 `onfail`（默认 continue）连接被拒也退出码 145（7.1.5 上是 0、存档照写）**，也就是 7.1.5 的坑在 9.0.2 上表面消失，但契约仍然要求显式写 `onfail=abort`，不依赖版本行为。
    2. **存档用分片 mp4**（架构师定）：`movflags=+frag_keyframe+empty_moov`（不再是 `+faststart`）。实测：`-f tee` + 分片 mp4 **必须同时加 `-flags +global_header`**，否则文件里没有 H.264 参数、无法解码（解码报 404 行错误、`could not find codec parameters`）；加了之后解码 0 个错误。`flush_packets=1` 让每个分片及时落盘：实测 `kill -9` 后带 `flush_packets=1` 的存档比不带的多保留约 34%（352 192 对 262 180 字节）、解码 0 个错误（不带时最后一个分片是残缺的，解码报 `partial file`）。关键帧间隔 `-g` 已经是 2×fps，分片约 2 秒。**强杀后保留（架构师定）**：分片 mp4 在强杀后仍可播放，所以存档直接写最终文件名，**不走 `RunWithPart`、强杀 / 失败后不删**。**同名处理**：文件名带秒、同一秒内两个会话或重启后仍可能重名，最终名冲突时加 `(n)` 后缀（`screen-20260929-200000(1).mp4`、`(2)`……，规则与 `task.UniquePath` 一致），**选名时用 `O_CREATE|O_EXCL` 建一个空文件占位**（不能只 `Stat` 后再交给 ffmpeg：ffmpeg 带 `-y` 会直接覆盖已有文件，实测 5 496 字节的已有文件被覆盖；对预先建好的空文件写入则正常）。文件名用 #22 的净化函数（见第 4 条）。**空壳**：连接失败（`started=false`，`onfail=abort` 时文件是 0 字节）或过早被杀时文件里只有 moov、没有一个完整分片，不是存档——实测 `kill -9`：0.6 秒和 1.5 秒时文件 1 263 字节、`ffprobe` 读不出时长；3 秒时 32 239 字节、时长 2.08 秒、50 个视频包；6 秒时 61 271 字节、时长 4.08 秒、100 个包（分片约 2 秒，所以头约 2 秒的数据可能还没落盘）。任务结束后（任何终态）`ffprobe` 读不出时长的存档文件**删除**并把 `outputPath` 清空，读得出的保留。**这条"空壳删除"已由架构师确认。** 三条硬性约束：① **只删本任务自己用 `O_EXCL` 创建的文件**（任务里记下占位时创建成功的最终路径；`O_EXCL` 失败换名重试时没创建成功的路径不算），**绝不碰已有文件**，也不删 `archiveDir` 里的其他文件；② **删除失败只记日志（不含推流地址，路径可记）**，**不改任务状态**、不改 `error`；③ **顺序：先清理并清空 `outputPath`，再发终态事件**（**#31 已实现**，见 6.10.2 第 2 项：`task.ClearOutputPath` 是 Runner 可以返回的特殊输出路径，`entry.finish` 遇到它把 `outputPath` 清空；语义是"终态落库和 `task:status` 里的 `outputPath` 为空串"。原先 `entry.finish` 只在 output 非空时才覆盖，清不掉）（`task:status` 和落库的 `Task` 里 `outputPath` 已经是最终值，前端不会看到已经不存在的路径）。**前端规则**：判断"强杀且存档已保留" = `status == "canceled"` 且 `outputPath` 非空（`canceled` 且 `outputPath` 为空 = 强杀且没有可用存档）；正常停止（`succeeded`）的存档**不加**"文件可能不完整"之类提示。测试：强杀后 0.6 秒的空壳被删、`outputPath==""`；3 秒以上的保留、`outputPath` 非空；删除失败（只读目录）任务状态仍是 `canceled` 且 `Task.error == nil`；已有同名文件不被删。
    3. **优雅停止等待延长**（架构师定）：有存档的会话，`q` 之后最多等 **15 秒**（无存档的会话仍是 5 秒）。实测网络正常时 `q` 后 0.11 秒退出、存档完整（时长与关键帧一致、解码 0 个错误），15 秒只是上限。应用退出时同样适用：`Manager.Shutdown` 的总等待上限在存在有存档的直播会话时相应提高到 16 秒（原为 8 秒，见 6.6）。**等待期间前端显示"正在停止…"，超时走强杀**（强杀后的存档按第 2 条保留；任务终态为 `interrupted`）。
    4. **存档文件名固定为 `screen-<yyyyMMdd-HHmmss>.mp4`**（本地时间，架构师定；直接是最终文件名，冲突时加 `(n)`，见第 2 条），不再用 `<title>`，文件名里没有任何用户输入。仍要过一遍共用的文件名净化规则作为纵深防御（规则见 6.11.3「输出文件名」，直播额外禁止 `| ' [ ]`；对这个固定格式是恒等变换，测试断言"净化前后相等"）——**该规则由 #22 引入。合并顺序与实现依赖（架构师定）：#22 先合；直播存档的实现放在 #22 合入之后的提交里，文件推流部分不用等 #22；文档层面 #19 先合也无妨，这里只是引用，依赖的是实现。**用户可控的只有 `archiveDir`，它靠下面的转义规则保证安全。
- **`tee` 段转义规则**（架构师定；7.1.5 实测）：`tee` 的 slave 描述里 `|` 是 slave 分隔符，`'` `\` `[` `]` `,` `:` `=` 等是选项语法，**不转义会静默写到错误的位置或直接失败**。实测（Linux，未转义）：文件名或目录里的 `|` 把路径拆成两个 slave，`a|b.mp4` 实际创建了 `a` 和 `b.mp4` 两个文件；`'` 被吞掉（`q'x.mp4` 变成 `qx.mp4`，目录里有 `'` 则找不到目录，退出码 254）；`\` 被当转义符吞掉（`C:\Users\x\o.mp4` 变成文件 `C:Usersxo.mp4`）；目录里有 `[` 打不开（退出码 254）；文件名里的 `[` `]` `,` `=` `;` `:` 空格中日韩字符没问题，但不能靠这个判断——统一转义。规则：
  1. **路径统一**：`file:` 前缀 + 正斜杠（Windows 先把 `\` 换成 `/`，再转义）。拒绝 `\\?\`、`\\.\` 开头的路径（`INVALID_ARGUMENT`）。
  2. **转义函数**：把 `<存档路径>` 和 `<url>` 里所有**不在 `[A-Za-z0-9_./-]` 也不是非 ASCII 字符**的字符前面加一个 `\`（包括 `\ ' | [ ] , : = ; 空格 # ? % & ( )` 等，Windows 盘符冒号写成 `C\:/…`）。非 ASCII（中日韩等）不转义，实测可用。
  3. **实测**：目录 `录 屏'|[x],y=z;c:d#f?g%h&i(1)`（含以上所有特殊字符），转义后 `[f=mp4]file:<转义路径>/screen-20260929-200000.mp4` 退出码 0、文件建在期望位置；同一个路径不转义退出码 254。URL 转义后也正常（`rtmp\://127.0.0.1\:1935/live/esc\?k\=v\&x\=1` 能连上），未转义的含 `?a=b,c=d;e=f&t=1` 和 IPv6 `[::1]` 的 URL 在 tee 里也能被正确解析，但为了统一，**一律走转义函数**。
  4. **测试**：转义函数表驱动（上面每个特殊字符、中日韩、空格、Windows 盘符路径、UNC 路径）；集成测试用真实 ffmpeg 往含特殊字符的临时目录写存档，断言文件出现在期望路径且没有多余文件。
  5. **【未验证】**：Windows 真机上 `file:C\:/Users/…` 的解析（Linux 上盘符冒号只是普通字符，无法验证 Windows 的盘符语义）；UNC 路径 `//server/share/…`。这两项需要用户在 Windows 上验证。
- **采集能力检测**（`GetCaptureCapabilities` / `ListScreens` / `StartScreenPush` 共用一套逻辑）：Linux 读 `XDG_SESSION_TYPE`——`wayland`（即使有 XWayland 的 `DISPLAY`）或没有 `DISPLAY` → `supported=false`，`Start*` / `ListScreens` 返回 `UNSUPPORTED_PLATFORM`，不去录黑屏；macOS 需要"屏幕录制"授权，授权记在 FFmpegFree 名下（不是 ffmpeg 名下），查不出来时 `permission=unknown`，`Start` 时按 ffmpeg 报错分类为 `SCREEN_PERMISSION_DENIED`。显示器列表：Windows 用 `EnumDisplayMonitors`（`golang.org/x/sys/windows`，不用 cgo）；macOS 解析 `ffmpeg -f avfoundation -list_devices true -i ""` 里的 `Capture screen N`；Linux 解析 `xrandr --display $DISPLAY --query`（**必须显式带 `--display $DISPLAY`**，与已合并的 #31 实现一致（`internal/service/live/screen.go` 的 `listX11Screens`：`xrandr --display <DISPLAY> --query`）；`GetCaptureCapabilities` 用的是同一个 `DISPLAY`（为空则 `supported=false`）；不依赖子进程继承的环境变量，`$DISPLAY` 取的是与采集输入 `-i <DISPLAY>+x,y` 同一个值，保证枚举的屏幕和采集的屏幕是同一个 X server；`DISPLAY` 打不开时 `xrandr` 报 `Can't open display`，按"没有 xrandr"同样降级），没有 xrandr 或它失败时只返回一个 `x11:desktop`。
- **错误分类**：`ffmpeg.ClassifyLiveError`，规则和 6.9 一样——只看 `classifiableLines()` 剔除 `Input #` / `Output #` / `Stream mapping:` / `Metadata` / `Stream #` 段落后的行，系统错误文本按行尾匹配（tee 会话的失败行形如 `[tee @ …] Slave '[f=flv:onfail=abort:…]rtmp://…': error opening: Connection refused`，slave 描述里带完整 URL，**日志和 detail 里必须已脱敏**）。区分"连接失败"和"中断"靠 Runner 记的 `started`：**收到第一个 `progress=continue` 且 `out_time_us > 0` 的 `-progress` 块之后为 `true`**（架构师建议用 `total_size > 0`，**后来已接受改为本判据**；7.1.5 实测：**走 tee（有存档）时 `total_size` 和 `bitrate` 恒为 `N/A`**，用 `total_size` 会让有存档的会话永远"没开始"；无存档时 `total_size` 是数字（首块 4 985 字节、`out_time_us=80000`），两个判据同时成立，所以统一改用 `out_time_us`）。**`progress=end` 块不算开始**：连接失败时 ffmpeg 也会补一个 `frame=0 total_size=0 out_time_us=N/A progress=end`。Runner 在 `started` 之前不发 `task:progress`，所以前端"收到第一条 progress = 已在推"的判断与此一致。
  - `started=false`：无法解析主机名 / 连接被拒绝 / 超时 / 网络不可达 → `LIVE_CONNECT_FAILED`；服务器明确拒绝 → `LIVE_PUSH_REJECTED`。**按 ffmpeg 7.1.5 + MediaMTX 1.21.1 实测**：RTMP 鉴权失败的 stderr 是 `[rtmp @ …] Server error: authentication failed` + `Error opening output …: Operation not permitted`（可区分，判为 `LIVE_PUSH_REJECTED`）；RTMP 服务器未开 / 连接被拒是 `Connection refused`，域名解析失败是 `Failed to resolve hostname`（均为 `LIVE_CONNECT_FAILED`）；**SRT 是已知局限**：服务器未开与被拒绝（错误 passphrase / 无权限）在 ffmpeg stderr 里都只有 `Connection to srt://… failed: Input/output error`，无法区分，统一判 `LIVE_CONNECT_FAILED`（`LIVE_PUSH_REJECTED` 对 SRT 实际上不会出现）。
  - `started=true`：`Broken pipe` / `Connection reset` / `Connection timed out` / 写出时 `Input/output error`、`Error writing trailer`（网络中断）→ `LIVE_PUSH_INTERRUPTED`。
  - 屏幕采集：avfoundation 权限相关报错 → `SCREEN_PERMISSION_DENIED`；`x11grab` 打不开显示 → `UNSUPPORTED_PLATFORM`。
  - 认不出来的非零退出 → `INTERNAL`（不是 `PROCESS_FAILED`），stderr 最后 50 行（已脱敏）放 `detail`。
  - **已请求 `Cancel` 之后一律归 `canceled`**（架构师定）：Runner 一旦收到取消（用户 `Cancel` 或应用退出的 ctx 取消），之后 ffmpeg 无论以什么非零码退出（`Broken pipe`、`Connection reset`、被强杀……）都返回取消错误 → 任务 `canceled`（应用退出为 `interrupted`），`error` 为空，**不做错误分类**，不得落 `LIVE_PUSH_INTERRUPTED`、`LIVE_CONNECT_FAILED`、`INTERNAL`。只有"优雅停止成功且退出码 0"才是 `succeeded`。测试：取消后让假 ffmpeg 以 224 / 251 / 255 退出，断言状态是 `canceled` 且 `error == nil`。
  - **连接阶段取消直接强杀**（还没有 `started`）：不发 `q`、不等 5 秒，直接结束进程组。原因（实测）：ffmpeg 卡在连接里时不读 stdin，往不通的地址（`10.255.255.1`）推流，2 秒后发 `q`，它又过了约 3 秒才因连接超时自己退出（退出码 146）。
  - 用户主动停止：优雅退出成功 `succeeded`；超时强杀 `canceled`（没有错误码）。**具体的 ffmpeg 报错措辞（各版本、各服务器不同）必须用真实推流服务器收集样本后落成测试**，设计稿里的关键词只是起点。
- **脱敏**（`internal/live`，纯函数，必须有表驱动测试）：
  - `RedactURL(raw string) string`：用户信息 → `***@`；rtmp / rtmps 保留 host、端口和第一段路径（应用名），其后的路径（流名，可含 `/`）→ `***`；所有查询参数保留键、值 → `***`；fragment 去掉；解析失败返回 `<invalid-url>`，绝不回显原文。例：`rtmp://u:p@h:1935/live/abc123?token=xyz` → `rtmp://***@h:1935/live/***?token=***`，`srt://h:9000?streamid=a&passphrase=b` → `srt://h:9000?streamid=***&passphrase=***`。
  - `NewRedactor(rawURL string) func(line string) string`：处理一行输出——① 原始 URL、它的 URL 编码 / 解码形式、以及从中提取的每个秘密片段（用户名、密码、流名、每个查询值，长度 ≥ 3）按字面替换成 `***`；② 再用正则 `(rtmps?|srt|tcp|tls|udp)://[^\s'"<>]+` 把行里残留的任何 URL 交给 `RedactURL`（覆盖 ffmpeg 改写后的形式，如 `tcp://host:1935?tcp_nodelay=0`）。
  - **覆盖范围**：`ffmpeg.RunOptions` 增加 `Redact func(string) string`，`Run` 在 stderr 每一行进入 `TailBuffer`、`OnStderr`（任务日志）、`Classify` **之前**先过它；记录的命令行也用脱敏后的 URL；`task.Spec.Title`、`Spec.Params`、`apperr` 的 `message` / `detail`、事件 payload、后端 `logf` 全部只用脱敏后的值。完整 URL 只存在于 Runner 的内存和 ffmpeg 的命令行参数里，不落库、不写文件。
  - **`params`（脱敏，不能用来重试）**：文件推流 `{"kind":"file","input":"/abs/a.mp4","url":"rtmp://h/live/***","loop":true,"options":{...}}`；屏幕推流 `{"kind":"screen","screenId":"monitor:0","url":"srt://h:9000?streamid=***","hideCursor":false,"audio":"none","archiveDir":"","options":{...}}`。
  - **测试要求**：URL 表驱动（各协议、userinfo、多段路径、IPv6、非法串）；行脱敏用真实 ffmpeg 输出样本；端到端用假 ffmpeg 脚本把完整 URL 打到 stderr 并失败，断言 `Task.title` / `Task.params` / `error.message` / `error.detail` / 日志文件 / 全部事件 payload 里都搜不到任何秘密片段。
  - **已知限制**：ffmpeg 命令行里必须有完整 URL，同一台机器上的其他进程（任务管理器、`ps`）能看到；应用不能规避，文档里说明。
  - **日志规则**：后端**不得**输出 `cmd.Args` / `cmd.String()` / `exec.Cmd` 的任何格式化结果（含调试日志、panic 信息、`%v` / `%+v`），记录命令行只能用已脱敏的副本；发布版关闭 Wails 的调试日志（`logger.DEBUG` 级别、`options.App.LogLevel` 设为 `ERROR`/`INFO`，`Debug` 相关开关关闭），因为 Bind 调用的参数会被它记录；**前端不得把完整推流 URL（含流名、口令）存进 `localStorage` / `sessionStorage` / IndexedDB**，需要记住地址时只存脱敏后的 `PushURLInfo.redacted`，密钥由用户每次输入。
- **指标**：`task.Progress` 增加 `Fps float64`、`BitrateKbps float64`、`DroppedFrames int64`，`task.ProgressEvent` 和 `Task` 增加同名字段（`omitempty`；**#31 已实现**，见 6.10.2 第 4 项：三个字段已在 `origin/v2` 的 `task.Progress`、`task.ProgressEvent`、`Task` 上）；`FFmpegRunner` 从 `ffmpeg.ProgressUpdate`（已有 `Fps`、`Dropped`、`TotalSize`、`OutTimeSec`）填充，`BitrateKbps` **只在无存档的会话里计算**：用相邻两次 progress 的 `total_size` / `out_time` 增量做 5 秒滑动平均；有存档的会话（tee，`total_size` 恒为 `N/A`）不计算、不轮询文件大小，保持 0 → 因 `omitempty` 不出现在 JSON 里（`out_time` 不增长时沿用上一个值，不出现 NaN / Inf）。其余节流、`version`、丢弃旧事件规则不变。
- **会话与任务管理器**：新增 `TypeLiveScreenPush`，`IsLive` 包含它；旧的 `TypeLiveRelay`、`TypeLiveRecordPush` 常量**保留但不再产生**（架构师定，见下方确认项 ⑧）：`Submit` 不再接受，`IsLive` 对它们仍为 true 只是为了常量兼容。不注册重试工厂，`live_file_push` / `live_screen_push` 的 `Retry` 得到 `UNSUPPORTED`（message：直播会话不能重试，请重新开始推流）；旧类型（`live_relay`、`live_record_push`）的 id 调 `Retry` / `Get` / `Cancel` / `Remove` 一律 `NOT_FOUND`，见下方确认项 ⑧。进行中的会话同时最多 4 个；同一个标准化推流地址同时只能有一个会话；**屏幕推流同一时间最多 1 路**（都是 `TASK_CONFLICT`，`detail` 首行 `reason=max_sessions` / `duplicate_url` / `screen_busy`，判断顺序 `duplicate_url` → `screen_busy` → `max_sessions`，见下方确认项）。应用退出：`Shutdown` 取消 → 优雅停止最多 5 秒（有存档 15 秒，总等待 16 秒；前端显示"正在停止…"，超时走强杀）→ 状态 `interrupted`（有存档时存档按 6.10 保留）；应用崩溃时 ffmpeg 子进程由操作系统回收（Windows 见 Job Object 修订）。
- **【未验证】（设计稿的已知风险，实现时要真机验证，汇总见 6.10.1「真机试用清单」）**：macOS 屏幕录制授权的检测方式（不用 cgo 时只能靠 ffmpeg 报错或首帧内容判断）；Windows gdigrab 在多显示器 / 非 100% 缩放下偏移和尺寸是否等于物理像素；`x11grab` 在各桌面环境下的表现；上面所有 ffmpeg 报错关键词；RTMP / SRT 在不同服务器（nginx-rtmp、SRS、MediaMTX、常见直播平台）上的兼容性。

- **已确认项**（原待定项 ①~⑨，不再待定）：
  - ①~⑦ **产品经理和架构师已正式确认**：① 同时进行的直播会话上限 4 个、同一标准化地址只允许一个会话；**（产品经理追加）屏幕推流同一时间最多 1 路，文件推流不受影响**；② 屏幕推流首版不采集声音（只有 `none` / `silent`）；③ 始终重编码（不支持 `-c copy` 直推文件）；④ 允许推到回环 / 内网地址；⑤ 只支持 rtmp / rtmps / srt，不含 rtsp / whip / http-flv 推流；⑥ 存档只用 mp4，且只有屏幕推流有存档；⑦ 优雅停止成功记 `succeeded`、强杀记 `canceled`，前端只看 `status`（硬性规则见第 4 节「结果语义」）；**优雅停止与自然播完都是 `succeeded`，都显示"已结束推流"，不区分、不加字段**。
  - ⑧ **架构师定**：任务中心**不展示** `live_relay` 和 `live_record_push`；这两个旧类型在契约里标为"保留但不再产生"（`Submit` 不接受）；数据库里若有旧记录，一律按未知类型**忽略、不报错**（`List` / `ListActive` 等读取路径遇到类型不在当前枚举内的行时跳过，不返回错误、不影响其他记录；**#31 实现：忽略发生在 store 层，对旧类型的 id 调 `Get` 返回 `NOT_FOUND`**，就当这条记录不存在，不返回"未知类型"之类的新错误；**架构师定：旧类型（`live_relay`、`live_record_push`、`edit_render` 等一切"保留但不再产生"的类型）的任务 id，`Get`、`Cancel`、`Remove`、`Retry` 四个方法一律返回 `NOT_FOUND`**，逐个写死：
    - `Get(id)`：`NOT_FOUND`（就当这条记录不存在，不返回"未知类型"之类的新错误、不返回 `UNSUPPORTED`）。
    - `Cancel(id)`：`NOT_FOUND`（不是 `TASK_CONFLICT`；旧类型没有进行中的会话，也不会有）。
    - `Remove(ids, deleteOutput)`：ids 里**只要有库里存在的旧类型记录的 id，整体返回 `NOT_FOUND`，什么都不删**（与"任一 id 仍在进行则整体失败"同一套整体失败语义）。这是"不存在的 id 忽略"（6.6）的**例外**：真正不存在的 id 仍然忽略、不报错，只有"库里有这条记录、但类型是旧类型"的 id 才报 `NOT_FOUND`；旧记录本身留在库里不删（任务中心不展示，也没有入口删）。
    - `Retry(id)`：`NOT_FOUND`（**先于**"没有重试工厂返回 `UNSUPPORTED`"判断：旧类型按不存在处理，不落 `UNSUPPORTED`）。
    - **实现方式（与已合并的 #31 一致，`internal/store/tasks.go`、`internal/task/ops.go`；措辞以代码为准）**：**由 store 的 SQL 读取层过滤**——`legacyTypes`（目前是 `live_relay`、`live_record_push`；`edit_render` 待 #30 合入后加进同一处）拼成 `legacyTypesSQL`，所有读取和按 id 操作的 SQL 都带 `type NOT IN (…)`：`GetTask`（所以 `Manager.Get` / `Cancel` / `Retry` 拿到"没找到"，得到 `NOT_FOUND`）、`ListTasks` / `ListActive` 的 `taskWhere`（列表不含旧类型）、`DeleteTasks`（逐个 id 的 `DELETE … AND type NOT IN …`，旧类型 id 不会被删）、`DeleteFinishedTasks`（`ClearFinished` 的实现，查询和删除都带 `type NOT IN …`，**旧类型记录不被清掉**）。**`Remove` 是特例**：`Manager.Remove` 在做任何事之前先调 `Store.LegacyTaskIDs(ids)`（返回"库里有这条记录、但类型是旧类型"的 id，真正不存在的 id 不在其中），**只要有一个，整体返回 `NOT_FOUND`，什么都不删**（记录、日志、输出文件都不碰）。**`ClearFinished` 和 `DeleteTasks` 同样排除旧类型**（上面的 SQL）。新增旧类型只改 `legacyTypes` 这一处；`IsLegacyType(t)` 用于判断。**测试**：库里插入 `live_relay` / `live_record_push` 各一条（`edit_render` 待 #30），断言 `Get` / `Cancel` / `Remove` / `Retry` 都返回 `NOT_FOUND`，`List` / `ListActive` 不含它们，`ClearFinished` 后它们仍在库里，且 `Remove` 整体失败时同批里合法的 id 没有被删
  - ⑨ **前端负责**：由前端在 `v2-fe-api-contracts` 里补全 `AppErrorCode`（`CANCELED`、八个 `LIVE_*` 相关码、`PROBE_FAILED`、`UNSUPPORTED`、`CONVERT_DISK_FULL`），并对照第 2 节契约错误码表逐项核对。后端不改动。
- **SRT 说明（架构师 / 产品定）**：SRT 连接失败**统一判 `LIVE_CONNECT_FAILED`**（原因见上文实测：服务器未开与被拒绝在 ffmpeg stderr 里无法区分）。产品文案"连接失败，请检查地址和口令是否正确"由**前端负责**，后端 `message` **不承载该文案**（后端 `message` 只描述技术原因，`detail` 是脱敏后的 stderr 尾部）。 前端据 `LIVE_CONNECT_FAILED` 的 `detail` 第一行 `scheme=srt`（RTMP 为 `scheme=rtmp` / `rtmps`）选文案，见 2.2。
- **用户可见提示（来自产品经理，仅供前端参考；后端只保证错误码和触发条件，不返回这些文案）**：
  - `TASK_CONFLICT`：进行中的直播会话已达 4 个 → 前端提示"最多同时推 4 路"；同一标准化地址已有进行中的会话 → "这个地址已经在推流"。两种触发共用同一个错误码，**用 `detail` 第一行区分（架构师已确认，稳定枚举）**；**产品经理追加：屏幕推流同一时间最多 1 路**，已有进行中的屏幕推流时再开一路 → `reason=screen_busy`（**文案由前端负责，后端不写、不返回**）。三个取值：
    - `reason=screen_busy`：**已有进行中的 `live_screen_push`，再调 `StartScreenPush`**（产品经理定"屏幕推流同一时间最多 1 路"）。只有 `StartScreenPush` 会得到它；**文件推流不受影响**（有屏幕推流在进行时仍可 `StartFilePush`，只受 4 路上限和地址唯一约束）。"进行中"指状态 `queued` / `running` 的 `live_screen_push`（含正在优雅停止、还没到终态的）。
    - `reason=max_sessions`：进行中的直播会话已达 4 个上限。
    - `reason=duplicate_url`：同一标准化推流地址已有进行中的会话。
    - **判断顺序（写死，架构师定）**：依次判断 **`duplicate_url` → `screen_busy` → `max_sessions`**，命中第一个就返回，不继续判断；`StartFilePush` 没有 `screen_busy` 这一步，只判断 `duplicate_url` → `max_sessions`。例如已有一路屏幕推流、又用**同一地址**开屏幕推流，得到的是 `duplicate_url` 而不是 `screen_busy`；已有屏幕推流、再用**不同地址**开屏幕推流，得到 `screen_busy`（即使这时总数已达 4 路也是 `screen_busy`，不是 `max_sessions`）；没有屏幕推流、总数已达 4 路时开新的推流，得到 `max_sessions`。
    - **稳定枚举规则**：`detail` 第一行固定为 `reason=<值>`，整行只有这一个键值对。以后新增取值**只能追加、不能改名、不能改含义、不能删除**；追加要走契约版本变更并在此列出。
    - 适用范围：`StartFilePush` / `StartScreenPush`（以及复用同一检查的 `CheckPushURL`，如果它做会话冲突检查）因会话冲突返回的 `TASK_CONFLICT`。`Cancel` 已结束会话、`Remove` 进行中任务等其他 `TASK_CONFLICT` 不属于这两个取值，**不带 `reason=` 行**（沿用原有 detail）。
    - **(a) 测试要求**：必须有测试分别触发三种冲突，各自断言 `detail` 第一行**精确等于** `reason=screen_busy` / `reason=duplicate_url` / `reason=max_sessions`（不是包含），**另有测试断言判断顺序**（同时满足 `duplicate_url` 和 `screen_busy` 得 `duplicate_url`；同时满足 `duplicate_url` 和 `max_sessions` 得 `duplicate_url`；同时满足 `screen_busy` 和 `max_sessions` 得 `screen_busy`），**断言文件推流在有屏幕推流进行时不返回 `screen_busy`**，并断言两者的 `code` 都是 `TASK_CONFLICT`；同时断言未触发冲突的其他 `TASK_CONFLICT`（如已结束会话再 `Cancel`）不带 `reason=`。
    - **(b) 脱敏要求**：这三种 `detail` 里**不得出现推流地址、口令、streamkey、streamid 或其任何片段**；`duplicate_url` 也不带地址（哪怕是脱敏后的地址、host 或端口），可以在第二行起写不含地址的说明（如"已有会话使用同一推流地址"）。测试要用带秘密片段的 URL 触发这两种冲突，断言 `message` / `detail` / 事件 / 日志里都搜不到秘密片段和 host。
    - **(c) 前端规则**：前端遇到**未知的 `reason` 值或没有 `reason` 行**时，显示通用冲突提示（如"操作冲突，请稍后再试"，文案由前端定），不得猜测含义、不得报错崩溃。
  - `LIVE_URL_INVALID`：协议不是 rtmp / rtmps / srt 时**后端已经是这个码**，`detail` 第一行 `reason=scheme_unsupported`，前端提示"暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt"。同一个码的其他原因用 `reason=malformed` / `missing_host` / `param_not_allowed` 区分（枚举和规则见 2.2，未知值走通用文案），**不要靠 `message` 文本区分**。

- **预览画面（v0.17，架构师定；实现：`internal/ffmpeg/live_preview.go`、`internal/service/live/preview.go`）**：
  - **预览输出**：推流命令在**主输出之后**追加一路独立输出 `-map 0:v:0 -an -sn -dn -vf fps=2,scale=640:-2 -q:v 5 -protocol_whitelist file -f image2 -update 1 -atomic_writing 1 file:<预览路径>`。有自己的 `-vf`（不复用主输出的滤镜链，宽 640、高按比例取偶数、每秒 2 帧）；不影响主输出的编码参数、`-progress` 与码率统计。**带存档的屏幕推流：预览输出在 tee 之外**，仍是主输出（`-f tee`）之后单独的一路，不写进 tee 描述（测试断言 tee 描述里没有预览、且只有网络与存档两路）。文件推流、屏幕推流（含 Windows gdigrab 窗口采集、存档）用同一个 `PreviewOutputArgs`。
  - **CPU 说明**：预览输出与主输出共享同一路解码（ffmpeg 的输出端只多一个 fps + scale + mjpeg 编码，每秒 2 帧，开销很小）。当前直播主输出始终重编码（不用 `-c copy`），所以没有额外的解码次数；**如果以后加入"视频流复制（`-c copy`）"或"硬字幕"的推流场景，主输出不解码时预览输出需要单独解码（ffmpeg 会为预览输出自己起解码器），会额外占少量 CPU，那时按需要再评估默认是否关预览。** **【未验证】**高分辨率（4K）屏幕采集、Windows 真机上的预览耗时与 CPU 占用。
  - **读取与半帧**：`-atomic_writing 1` 让 ffmpeg 先写 `<路径>.tmp` 再改名，读取端不会读到半帧；`GetPreview` 读取时仍校验 JPEG：以 SOI（`FF D8`）开头、以 EOI（`FF D9`）结尾（允许末尾少量 0 填充），大小 4 字节~4 MiB，不合格（半帧、空文件、不是 JPEG）一律当没有画面返回空，不返回错误。`ts` 取文件修改时间。
  - **`GetPreview(sessionId)`**：`sessionId` 是推流任务 id（= 会话 id）或 `StartPullPreview` 返回的拉流预览会话 id。返回 `{data, ts, active}`：会话不存在 / 已结束、`preview=false`、ffmpeg 还没出第一帧、读到半帧、预览文件不存在，都是**空 data + ts=0，不是错误**；`active` = 会话是否还在进行（推流：任务没结束；拉流：会话没结束），前端在 `active=false` 或页面不可见时停止轮询，约 500 毫秒一次。预览关闭时 `GetPreview` 返回空（推流会话仍 `active=true`）。
  - **开关**：`FilePushRequest.preview` / `ScreenPushRequest.preview` / `PullPreviewRequest.preview` 是 `*bool`，缺省（nil）= true，false = 不加预览输出。**只在开始时决定**（ffmpeg 已启动无法动态改输出），没有 `SetPreviewEnabled`。
  - **降级（预览绝不能让主流失败）**：推流开始前用 `ffmpeg -encoders / -muxers / -filters` 检查 `mjpeg` 编码器、`image2` 封装、`fps` 与 `scale` 滤镜（按 ffmpeg 路径缓存）；不支持、预览目录不可用 / 不可写、`preview=false`，都**只是不加预览输出**（记日志，不报错、不改变 `Start*` 的返回）。运行中预览输出自己出错（写盘失败）会让 ffmpeg 整体退出，与主输出写失败同样处理（由主流的错误分类决定，预览不引入新错误码）。`StartPullPreview` 是拉流预览专用，没有"主流"可降级，ffmpeg 不支持时返回 `UNSUPPORTED`（`detail` 单独一行 `missing=preview`）。
  - **拉流预览会话**：`StartPullPreview` 后端起一个 ffmpeg **只读远端流、只输出预览**（不推流、不存盘），播放本身仍由前端播放器直接拉远端地址（契约 6.10 之前的"前端 mpegts.js 直接拉"不变）。地址规则：`rtmp` / `rtmps` / `srt` 复用推流地址校验（`LIVE_URL_INVALID` + `reason=`）；`http` / `https` 只做基本校验（主机必填、无空白 / 控制字符 / `|` `\` `"` `'`、≤ 2048 字节）；其他协议（含 `ws` / `wss`）`reason=scheme_unsupported`。输入侧 `-protocol_whitelist` 写在 `-i` 之前（rtmp `rtmp,tcp`、rtmps `rtmps,tcp,tls,crypto`、srt `srt,udp`、http(s) `http,https,tcp,tls,crypto`）。先用 ffprobe（`-rw_timeout 8s`，总 12 秒超时）探测有没有视频流：**纯音频没有预览**（不启动预览 ffmpeg，会话立即结束，`active` 变 `false`）；探测不出来（没有 ffprobe、连不上）按"有视频"让 ffmpeg 自己试。同一标准化地址重复调用返回同一会话（幂等）；同时最多 4 路（与推流会话上限分开计），超过 `TASK_CONFLICT`（`detail` 首行 `reason=max_pull_previews`）。会话不是任务（不进任务中心、不落库、不占 live 池）；`StopPullPreview`、远端流结束、ffmpeg 退出、应用退出（`Close`）都会结束会话并清理预览文件。地址（含口令 / 流名）只在调用参数里，日志和返回值只有脱敏形式。
  - **临时文件**：`<数据目录>/tmp/live-preview/<会话 id>.jpg`（及 ffmpeg 原子写入的 `.jpg.tmp`）。会话结束（含从未运行、排队中被取消）删除；**应用启动时清空并重建整个 `live-preview` 目录**（清理上次异常退出遗留；目录名必须是 `live-preview`，防止误删）；目录建不出来只是本次运行没有预览。
  - **前端约定**：约 500 毫秒轮询 `GetPreview`，`active=false`、任务进入终态、页面不可见时停止；`data` 转成 `data:image/jpeg;base64,<data>` 显示；`ts` 长时间不前进 = 画面停滞。浏览器模拟层（`frontend/src/api/live.ts`）最小假实现：`getPreview` 恒返回 `{data:'', ts:0, active:false}`，`startPullPreview` 返回不出画面的会话。**本版不改直播页 UI。**
  - **测试**：参数构造表驱动（文件推流有 / 无音轨、屏幕推流、带 tee 存档、拉流各协议、纯音频、`preview=false`）；`GetPreview` 半帧 / 无文件 / 关闭 / 未知会话；会话结束与启动清理；集成测试用真实 ffmpeg（7.1.5、9.0.2）+ MediaMTX 1.21.1（含 Xvfb 屏幕采集与带存档的屏幕推流）验证 2 秒内拿到宽 640 的合法 JPEG、画面不是黑屏 / 纯色（亮度方差）、主流与存档不受影响、停止后临时文件被清理。**【未验证】**Windows 真机上预览的耗时、高 CPU 占用；WebView 里 500 毫秒轮询大图 base64 的开销（每帧约几十 KB）。

- **采集来源（v0.14）**：
  - **`ListCaptureSources`**：屏幕来源 = `ListScreens` 的结果（`id` 换成 `screen:<序号>`，`title` = `ScreenInfo.Name`）；Windows 上再追加窗口来源。`ListScreens` 失败（`UNSUPPORTED_PLATFORM`，如 Wayland / 没有 `DISPLAY`）时整个方法同样失败。枚举窗口失败（`EnumWindows` 出错）只记日志、只返回屏幕，不报错。macOS / Linux 多显示器尽量列出（Linux 用 xrandr，没有 xrandr 只给一个 `x11:desktop` 默认；macOS 用 avfoundation 设备列表），**永远不返回 `window`**。
  - **窗口过滤（Windows，纯函数 `filterCaptureWindows`，有表驱动测试）**：`EnumWindows` 取顶层窗口，保留同时满足：标题非空（去空白后）；`IsWindowVisible`；不是最小化（`IsIconic`，最小化的不列出，因为 gdigrab 采不到内容；"最小化按需标记"本版选择不列出）；不是 DWM cloaked（别的虚拟桌面、挂起的 UWP 窗口）；客户区宽高都大于 0；不是本进程（FFmpegFree 自己）的窗口；不是系统壳窗口（类名 `Progman`、`WorkerW`、`Shell_TrayWnd`、`Shell_SecondaryTrayWnd`、`Windows.UI.Core.CoreWindow`，或标题 `Program Manager`）；没有 `WS_EX_TOOLWINDOW` / `WS_EX_NOACTIVATE` 且没有 owner（即被拥有的对话框、浮层不列）——带 `WS_EX_APPWINDOW` 的例外，照列。
  - **`id` 与校验**：`window:<hwnd 十进制>`，句柄在窗口关闭后会失效（也可能被复用，见未验证项）。`StartScreenPush` 传了 `captureSourceId` 时，**在占会话 / 建存档之前**重新枚举并按同一套过滤校验来源还在：格式不对（不是 `screen:<无符号十进制>` / `window:<无符号十进制>`、有前导零、带符号、十六进制）→ `INVALID_ARGUMENT`；非 Windows 传 `window:…` → `INVALID_ARGUMENT`；窗口不在过滤后的列表里（已关闭、已最小化、已不可见）→ `LIVE_SOURCE_GONE`（`kind=window`）；屏幕序号超出当前 `ListScreens` 的范围 → `LIVE_SOURCE_GONE`（`kind=screen`）。校验失败不创建任务、不占用会话。检查顺序：URL 校验 → 选项 → audio → 存档目录 → 能力检测 → 协议 / tee 检测 → **来源校验** → 会话冲突（`duplicate_url` → `screen_busy` → `max_sessions`）。
  - **ffmpeg 命令行（Windows gdigrab）**：窗口 → `-f gdigrab -framerate <fps> [-draw_mouse 0] -i title=<窗口标题>`（用**校验那一刻**的标题，标题作为**单个 argv 元素**传给 ffmpeg，不经过 shell，不加引号、不转义；gdigrab 把 `title=` 之后的全部内容当窗口标题，所以空格、引号、`=`、`&`、`|`、`%`、中日韩都原样；不带 `-offset_x` / `-offset_y` / `-video_size`，窗口大小由 gdigrab 决定，输出仍按 `PushOptions` 缩放并保证偶数）。屏幕 → `-f gdigrab -framerate <fps> [-draw_mouse 0] -offset_x <X> -offset_y <Y> -video_size <W>x<H> -i desktop`，`X/Y/W/H` 取自 `EnumDisplayMonitors` / `GetMonitorInfoW` 的显示器矩形（副屏在主屏左 / 上方时 `X` / `Y` 为负数，原样传）。macOS / Linux 屏幕来源命令行不变。
  - **窗口在校验后、ffmpeg 打开前消失**：gdigrab 报 `Can't find window '…', aborting.`——`ClassifyLiveError`（`Screen=true`）识别它 → `LIVE_SOURCE_GONE`，`detail` 只有 `kind=window`（stderr 里有窗口标题，**不放进 detail**）。这是任务失败（没有 `task:progress` 之前），不是同步错误。已开始推流之后窗口被关闭：gdigrab 行为未验证，按现有规则分类（多半是 `INTERNAL` 或 `LIVE_PUSH_INTERRUPTED`）。
  - **任务 `params`**：`kind=screen` 的 `params` 在传了 `captureSourceId` 时多一个 `"captureSourceId"` 字段（没传则不出现，旧任务不变）；任务标题 = `屏幕推流：<屏幕名 | 窗口标题> → <脱敏地址>`。`screenId` 字段仍是请求里的原值。
  - **【未验证】**（Linux 箱子只能验证参数构造、过滤、错误码；见 6.10.1 第 8～10 项）：Windows 真机上 `EnumWindows` 的实际过滤效果；gdigrab `title=` 采窗口被其他窗口遮挡 / 最小化 / 跨显示器 / 高 DPI（进程非 DPI 感知时 `GetClientRect` 与 gdigrab 的尺寸口径）；多显示器 `offset` / `video_size` 在非 100% 缩放、副屏在负坐标时是否对齐；同标题的多个窗口（`title=` 只能命中 `FindWindow` 找到的第一个，可能不是用户选的那个——**这是 `title=` 方案的固有局限**；ffmpeg 7.0+ 的 `hwnd=<十进制>` 可以精确指定窗口，但本机 ffmpeg 版本不一定支持，本版按契约用 `title=`，需要时另出契约变更）。

### 6.10.1 真机试用清单（Live，未验证项汇总）

以下项目**没有在真机上验证**（箱子是 Linux + ffmpeg 7.1.5 + MediaMTX 1.21.1），契约里已就地标"未验证"。**不阻塞实现**：实现按契约写，试用包出来后由用户逐项确认，不符再回来改契约。（Edit 的 Windows 路径 / `commitPart` / Range 续传、Doc 的 Windows 字体路径 / 大文件 Range 各在 6.11.1 / 6.11.4 / 6.12.1 / 6.12.4 有同样的清单，合并后由架构师汇总。）

| # | 未验证项 | 在哪里 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|---|
| 1 | Windows 上 `file:C\:/Users/…` 形式的存档路径能否被 ffmpeg tee 正确解析（Linux 上盘符冒号只是普通字符） | 6.10「`tee` 段转义规则」第 5 点 | Windows 上屏幕推流 + 存档到 `C:\Users\<含空格和中文的目录>`，确认文件出现在期望位置 | 改转义规则（契约变更），不改行为约定 |
| 2 | UNC 路径 `//server/share/…` 作存档目录 | 同上 | 存档到网络共享 | 同上；最坏情况在 `Start*` 拒绝 UNC 并返回 `INVALID_ARGUMENT` |
| 3 | `MoveFileEx` / 改名语义：Windows 上同名冲突时 `(n)` 后缀的占位文件（`O_CREATE\|O_EXCL`）能否被 ffmpeg 以 `-y` 写入；被杀后文件句柄释放时机是否影响随后的 `ffprobe` 与删除空壳 | 6.10 存档第 2 条 | 同一秒内连开两路存档；强杀后立即检查文件是否可读、空壳是否被删 | 空壳删除改为延迟重试；不影响主流程 |
| 4 | `rtmps`：本机 ffmpeg 的 TLS 握手与证书校验在真实服务器上的表现（7.1.5 只实测了连接被拒：退出码 145、`LIVE_CONNECT_FAILED`，没有真实 rtmps 服务器） | 6.10 命令行、错误分类 | 推到一个真实的 rtmps 地址（如常见直播平台的 rtmps 入口） | 证书错误的措辞补进分类规则；缺协议已有 `UNSUPPORTED`（`missing=rtmps`） |
| 5 | SRT 在真实公网 / 有 passphrase 的服务器上：错误 passphrase 与服务器未开确实都只有 `Input/output error`（实测于 MediaMTX，其他服务器未测） | 6.10「SRT 说明」 | 用错误口令推到 SRS / MediaMTX / 商用服务 | 仍判 `LIVE_CONNECT_FAILED`，前端文案不变 |
| 6 | macOS 屏幕录制授权检测；Windows gdigrab 多显示器 / 非 100% 缩放的偏移与尺寸；`x11grab` 在各桌面环境的表现 | 6.10「未验证」条 | 各平台真机各推一次 | 见各条 |
| 7 | 15 秒（有存档）/ 16 秒（`Shutdown`）优雅停止上限在慢网络、高负载下够不够 | 6.10 存档第 3 条 | 弱网下停止有存档的会话，看是否超时被强杀 | 调整上限（契约变更） |
| 8 | Windows：`EnumWindows` 过滤后的窗口列表是否合理（无任务栏 / 桌面 / 输入法 / 系统浮层，UWP 应用与最大化窗口能列出）；FFmpegFree 自己的窗口不出现 | 6.10「采集来源」 | Windows 上调 `ListCaptureSources`，与任务栏里的窗口对照 | 调整 `filterCaptureWindows` 的类名 / 样式过滤（不改契约结构） |
| 9 | Windows：gdigrab `title=` 采窗口——被其他窗口遮挡时内容是否正常、最小化后行为（黑屏 / 报错 / 冻结）、窗口移到副屏 / 高 DPI（125%~200% 缩放）时画面尺寸和清晰度；同标题多窗口命中哪一个；标题含引号、`&`、中文时能否找到窗口 | 同上 | 各推一次，观察播放端画面 | 遮挡 / 高 DPI 问题另出契约变更（如改用 `hwnd=`、DPI 感知清单）；同标题问题在前端提示或后端过滤 |
| 10 | Windows 多显示器：副屏在主屏左侧 / 上方（负偏移）、两块屏缩放不同时，`offset_x` / `offset_y` / `video_size` 是否对准该显示器（进程是否 DPI 感知影响 `GetMonitorInfoW` 的坐标口径） | 同上、6.10「采集能力检测」 | 双屏各选一块推流 | 改用物理像素坐标（进程声明 DPI 感知）或按缩放换算 |
| 11 | （v0.17）Windows 上预览输出的耗时（首帧时间、`GetPreview` 单次读取耗时）；高分辨率（4K）采集 / 硬字幕 / `-c copy` 场景下预览额外占用的 CPU；WebView 里每 500ms 轮询 `GetPreview`（base64 JPEG 经 IPC）的开销 | 6.10「预览画面」 | Windows 真机推流 1080p / 4K 屏幕，观察任务管理器 CPU、`GetPreview` 耗时、前端轮询时界面是否卡顿 | 降低 `fps` / 宽度常量、前端降低轮询频率，或在开销过大时默认 `preview=false`（需另出契约变更） |

### 6.10.2 实现清单（给 #31 / #30 对照；不是新接口；"现状"列已按 `origin/v2` 的 `2f0c0a4`（含已合并的 #31 第一部分）更新）

| # | 项 | 现状（`origin/v2`） | 要求 | 随哪个 PR |
|---|---|---|---|---|
| 1 | 优雅停止检查退出码 | **#31 已实现**：`ffmpeg.RunOptions.StrictGracefulExit`（`internal/ffmpeg/exec.go`），直播 Runner 开启（`internal/service/live/service.go`） | 直播路径必须检查退出码：`q` 后退出码 0 → `succeeded`；已请求 `Cancel` 后非 0 → `canceled`，无错误码；停止用 `q`（stdin 管道），不用 SIGINT（退出码 255） | #31 补一个提交 |
| 2 | 空壳存档清空 `outputPath` | **#31 已实现**：`task.ClearOutputPath` 特殊输出路径，`entry.finish` 遇到它把 `outputPath` 清空（`internal/task/`） | Runner 支持把 `outputPath` 清空（专门的清空标记 / 返回字段），先清空再发终态事件 | #31 补 |
| 3 | `Shutdown` 等待时间 | **#31 已实现**：`app.go` 的 `shutdown` 有带存档的直播会话时等 16 秒，否则 8 秒 | 有存档的直播会话运行时总等待 16 秒（6.6 / 6.10） | #31 |
| 4 | `fps` / `bitrateKbps` / `droppedFrames` | **#31 已实现**：`task.Progress` / `task.ProgressEvent` / `Task` 都有三个 `omitempty` 字段 | 三个字段（`omitempty`）；`bitrateKbps` 只在无存档会话计算 | #31 |
| 5 | 任务类型 | **#31 已实现** `live_screen_push` 和 `validType`（含 `live_file_push`）；`store/tasks.go` 已有 `TypeLiveScreenPush`；`TypeEditExport` 与 `validType` 里的 `edit_render` → `edit_export` 仍未改（随 #30） | `validType` 改为第 3 节 `TaskType` 枚举（`convert`、`edit_export`、`office_pdf`、`live_file_push`、`live_screen_push`、`ffmpeg_install`）；`Submit` **不再接受** `live_relay`、`live_record_push`（也不接受 `edit_render`）；`IsLive` 包含 `live_file_push`、`live_screen_push` | `live_screen_push` 随 #31，`edit_export` 随 #30 |
| 6 | 旧类型过滤 | **#31 已实现**（live 两个类型）：store 的 SQL 读取层过滤，`legacyTypes` / `legacyTypesSQL` / `LegacyTaskIDs`（`internal/store/tasks.go`）；`edit_render` 尚未加入（Edit 线合入前它仍是有效类型） | store 层读取时过滤旧类型行（`List` / `ListActive` 不含；`Get` / `Cancel` / `Remove` / `Retry` 一律 `NOT_FOUND`，见 6.10 确认项 ⑧） | #31（live 两个类型）、#30（`edit_render`） |
| 7 | `Task.error` 序列化 | `store.Task.Error` 是 `json:"error,omitempty"` | 与契约一致：无错误时省略该键，不输出 `null`（本契约示例已统一） | 无需改代码，测试断言 |
| 8 | 前端 `AppErrorCode` | `call.ts` 缺若干码 | 按 2.1 清单补全并逐项核对 | 前端（`v2-fe-api-contracts`） |

**ffmpeg 版本复测**：项目默认安装的是 **9.0.2**（`internal/ffmpeg/manifest.json`，martin-riedl 静态构建，SHA-256 与 manifest 一致），契约里的 ffmpeg 行为除注明外已在 **7.1.5 和 9.0.2 上都复测**：`q` 退出码 0；tee + `onfail=abort` 连接被拒退出码 145；tee 转义路径正常；`-protocol_whitelist` 按位置生效（输出侧只写 `rtmp,tcp` 时 `file:` 输入失败退出码 234）；`anullsrc` + `-shortest` 播完即结束；`-flvflags no_duration_filesize` 退出码 0；SRT passphrase 长度 9 / 81 位报 `SRTO_PASSPHRASE` 错、10 / 79 / 80 位通过。**与 7.1.5 不同的两点**：SRT 大写参数名在 9.0.2 报错而不是静默忽略；tee 默认 `onfail=continue` 连接被拒在 9.0.2 上退出码 145（7.1.5 是 0）。

## 6.11 EditService 契约（v0.11，只有契约，架构师冻结前不实现）

依据：v1 `master` 上 `backend/contollers/video_edit_controller.go`（`/api/edit/sources`、`/api/edit/probe`、`/api/edit/render`）与 `frontend/src/views/VideoEditor.vue`。v1 **没有**：撤销 / 重做、工程保存 / 打开、自动保存、切割（blade）工具、字幕、转场之外的关键帧；v2 首版也不做撤销 / 重做和切割（切割 = 前端把一个 clip 拆成两个 `inSec` / `outSec` 不同的 clip，不需要后端方法）。v1 有的：素材列表（按视频 / 音频过滤）、多视频轨 + 多音轨时间线、拖动 / 边缘裁剪 / 吸附 / 逐帧、按 clip 的速度 / 滤镜预设 / 模糊 / 转场、全局亮度对比度饱和度锐化、canvas 多 `<video>` 合成监视器、导出 mp4 / mov / mkv / webm。

> **架构师新增决定（写死，逐条对应下文）**：① 同轨不重叠单独成条（6.11.2 A）；② Windows 路径上限 259 按最终文件全路径算，含扩展名，为 `.part` 和最坏 `(99)` 预留，超长在 `Export` 提交时同步 `INVALID_ARGUMENT`（6.11.3「输出路径长度」）；③ `ValidateProject` 的 `warnings` 是稳定结构 `EditWarning{code, clipId?, message}`，`code` 枚举只追加（6.11.2 D）；④ `edit_proxy` 首版不做，契约只保留一句回退说明（6.11.4 第 4 点）；⑤ 导出期间后端允许再提交导出、走 batch 池排队，前端在同一工程导出中禁用"导出视频"按钮（6.11.3「并发提交」）；⑥ `/local/<token>` 必须支持 `HEAD`，token 失效返回 404，前端用 `HEAD` 探测后重新 `GetPreviewURL`（6.13）；⑦ `SaveProject` 只校验数量上限，`outSec` 必须大于 `inSec`，`outSec=0` 一律 `INVALID_ARGUMENT`（0 不再表示"到结尾"）。

> **#30 实现反馈回改**：`.part` 遗留清理首版只在 EditService 对 `edit_export` 做（6.11.3）；默认转场超限的处理、`durationSec` 公式、`SaveProject` 的时长估算、clip 最短时长、`outSec` 取整不报警告、`outputName` 空白与净化后为空的区别、各错误的 `detail` 行格式、探测缓存 key（6.11.1 / 6.11.2 / 6.11.3 / 6.11.5）；POSIX 改名残余竞态写进 6.11.7。

> **架构师已确认（v0.11 定稿）**：`Render` → `Export`、`edit_render` → `edit_export` 确定；数值 / 枚举越界一律 `INVALID_ARGUMENT`（不再静默截断）；filtergraph 走文件（`-/filter_complex <file>`，不支持时退 `-filter_complex_script <file>`，功能探测择一，见 6.11.2 第 0 条；9.0 已移除 `-filter_complex_script`）；输出文件名规则见 6.11.3；预览回退方案见 6.11.4 第 4 点。

### 6.11.1 数据结构

```go
type EditProject struct {
    SchemaVersion int          `json:"schemaVersion"` // 当前 1；大于 1 的工程 LoadProject 返回 UNSUPPORTED
    ID            string       `json:"id"`            // 新建时空
    Name          string       `json:"name"`          // 去首尾空白后 1~80 字
    Sources       []string     `json:"sources"`       // 素材库：绝对路径，去重，最多 100 个（v0.13 由 200 改为 100，产品经理定：素材 100），只是列表，不保证存在
    Output        EditOutput   `json:"output"`
    VideoTrack    []VideoClip  `json:"videoTrack"`    // 沿用 v1 的平铺结构，用 trackId 区分轨道
    AudioTrack    []AudioClip  `json:"audioTrack"`
    Effects       GlobalEffects `json:"effects"`
    UpdatedAt     int64        `json:"updatedAt"`     // 只读，Save 时由后端写
}
type EditOutput struct {
    Format string  `json:"format"` // mp4 | mov | mkv | webm，空 = mp4
    Width  int     `json:"width"`  // 16~7680，空(0) = 1920（产品经理定，默认导出 1920×1080；前端提交时会显式写宽高，兜底值只给绕过前端的调用）；导出时向下取偶数
    Height int     `json:"height"` // 16~4320，空(0) = 1080（与 width 的兜底配套：宽高都为 0 时是 1920×1080）
    Fps    float64 `json:"fps"`    // (0,120]，空(0) = 30
}
type VideoClip struct {
    ID                    string  `json:"id"`      // 前端生成，字符集 [A-Za-z0-9_-]，1~64 字符，同一工程内视频 / 音频 clip 合起来唯一；错误 detail 第一行用它定位（正则见 6.11.2 C）
    Path                  string  `json:"path"`    // 绝对路径（v1 的 fileName + scope 在 v2 删除）
    TrackID               string  `json:"trackId"` // V1~V8，编号大的盖在上面
    StartSec              float64 `json:"startSec"`
    InSec                 float64 `json:"inSec"`
    OutSec                float64 `json:"outSec"`  // 必须 > inSec；0 一律 INVALID_ARGUMENT（0 不表示"到结尾"，前端加 clip 时用探测到的素材时长填）
    Speed                 float64 `json:"speed"`   // 0.25~4，空(0) = 1
    EffectPreset          string  `json:"effectPreset"`          // none | grayscale | sepia | vintage | cinematic，空 = none
    TransitionToNext      string  `json:"transitionToNext"`      // none | fade | wipeleft | wiperight | slideleft | slideright | circleopen | circleclose | dissolve，空 = none
    TransitionDurationSec float64 `json:"transitionDurationSec"` // 0 = 默认 0.5；显式设置范围 0.1~2，且不超过相邻两个 clip 中较短者的一半（显式超限 INVALID_ARGUMENT；默认值超限静默缩短，见 6.11.2 第 2 条）
    Blur                  float64 `json:"blur"`    // 0~4
}
type AudioClip struct {
    ID       string  `json:"id"`
    Path     string  `json:"path"`
    TrackID  string  `json:"trackId"` // A1~A8
    StartSec float64 `json:"startSec"`
    InSec    float64 `json:"inSec"`
    OutSec   float64 `json:"outSec"`   // 同 VideoClip：必须 > inSec，0 无效
    Speed    float64 `json:"speed"`   // 0.25~4
    Volume   float64 `json:"volume"`  // 0~4，0 = 静音；空值不可区分，前端必须显式传 1
}
type GlobalEffects struct {
    Brightness float64 `json:"brightness"` // -0.5~0.5
    Contrast   float64 `json:"contrast"`   // 0.5~2，0 视为 1
    Saturation float64 `json:"saturation"` // 0~2，0 视为 1（v1 行为；要完全去色用 clip 的 grayscale）
    Sharpen    float64 `json:"sharpen"`    // 0~2
}
type EditExportOptions struct {
    OutputName string `json:"outputName"` // 不含扩展名；空或纯空白 = 工程名；净化规则见 6.11.3「输出文件名」，最长 100 字符；净化后才变空则直接用 "edit"（不回退工程名）
    OutputDir  string `json:"outputDir"`  // 规则同 6.9：空 = Settings.defaultOutputDir，仍空 = 第一个 clip 所在文件夹；必须是绝对路径
}
type EditPlan struct {
    DurationSec float64       `json:"durationSec"` // 导出的实际时间线总长：扣除转场重叠之后（见 6.11.2 E），视频与音频一起算；进度分母用它
    ClipCount   int           `json:"clipCount"`
    Inputs      []string      `json:"inputs"`      // 去重后的素材路径
    HasAudio    bool          `json:"hasAudio"`    // 音轨是否非空；false 时导出静音音轨
    Warnings    []EditWarning `json:"warnings"`    // 结构化警告，没有时是 []，不是 null
}
type EditWarning struct {
    Code    string `json:"code"`              // 稳定枚举，只追加、不改名、不改含义、不删除，取值见 6.11.2 D；前端按 code 出文案
    ClipID  string `json:"clipId,omitempty"`  // 与警告有关的 clip（gap 类指"后一个"clip）；工程级警告省略
    Message string `json:"message"`           // 给日志 / 开发者看的中文说明，前端不得解析、不得直接展示为主文案
}
type EditProjectMeta struct {
    ID string `json:"id"`; Name string `json:"name"`; DurationSec float64 `json:"durationSec"`; ClipCount int `json:"clipCount"`; UpdatedAt int64 `json:"updatedAt"`
}
type LoadedProject struct {
    Project      EditProject `json:"project"`
    MissingPaths []string    `json:"missingPaths"` // 工程里引用但磁盘上已不存在的素材（sources 与 clip 的并集）
}
type PreviewURL struct {
    URL  string `json:"url"`  // 形如 /local/<token>，直接给 <video src> / <audio src>
    Mime string `json:"mime"`
    Size int64  `json:"size"`
}
```

### 6.11.2 校验（`ValidateProject` 与 `Export` 共用，先整体校验再提交，任何一项失败不产生任务）

**顺序（决定"第一个校验失败的片段"是谁，实现必须按此顺序，测试逐条断言）**：0 环境 → 1 工程级 → 2 逐 clip 字段（先 `videoTrack` 再 `audioTrack`，各自按数组顺序）→ 3 逐 clip 路径与探测（同上顺序；同一素材的失败记在按顺序第一个用到它的 clip 上）→ 4 同轨重叠（A）→ 5 输出（名字、目录、路径长度，见 6.11.3）。第一个失败就返回，不累计。

0. **环境**：ffmpeg / ffprobe 就绪，否则 `FFMPEG_NOT_FOUND`；**filtergraph 文件选项的功能探测**（导出方式依赖它，见 6.11.3「命令行长度」）。**背景（实测）**：项目默认安装的是 **ffmpeg 9.0.2**（`internal/ffmpeg/manifest.json`），**9.0 已移除 `-filter_complex_script`**（`Unrecognized option 'filter_complex_script'`，退出码 8）；同一份滤镜文件用 **`-/filter_complex <file>`** 在 9.0.2 和 7.1.5 上都是退出码 0。所以**功能探测择一、先新后旧**：① **先探 `-/filter_complex`**（7.0 起）；② 不支持再探 `-filter_complex_script`（6.x）；③ **两个都不支持才返回 `UNSUPPORTED`**，`detail` 第一行 `project`、第二行 **`missing=filter_complex`**（不再是 `missing=filter_complex_script`），**不落 `PROCESS_FAILED`**。探测方式是功能探测而不是解析帮助文本或按版本号判断：`ffmpeg -hide_banner -nostdin -loglevel error -f lavfi -i nullsrc=s=32x32:r=5:d=0.4 <选项> <临时文件，内容 [0:v]scale=16:16[v]> -map [v] -f null -`，`<选项>` 依次是 `-/filter_complex`、`-filter_complex_script`；退出码 0 = 可用；不认识的选项退出码 8、stderr `Unrecognized option`。**实测**：9.0.2（martin-riedl 静态构建，SHA-256 与 manifest 一致）`-/filter_complex` 退出码 0（约 0.01 秒）、`-filter_complex_script` 退出码 8；7.1.5 两个都是 0；滤镜文件含换行（`scale=16:16,\nsetsar=1`）时 `-/filter_complex` 在 9.0.2 和 7.1.5 上也是 0。**探测结果（选中了哪个选项，或都不支持）按 ffmpeg 二进制缓存到进程内，key = 路径 + 文件大小 + 修改时间**（换 ffmpeg 即失效）；`Export` 用探测选中的那个选项。**未验证**：6.x 上 `-/filter_complex` 不可用、`-filter_complex_script` 可用（架构师给的版本边界，箱子上没有 6.x）；8.x 两个选项各自的状态未测。
1. **工程级**：`videoTrack` 不能为空（v1 同）；clip 总数（视频 + 音频）≤ 100；`sources` ≤ 100 且都是绝对路径；名称 1~80 字；序列化后 ≤ 1 MiB；`schemaVersion` = 1；输出参数范围；时间线总长 ≤ 6 小时（**按 clip 自填值 `max(startSec + (outSec − inSec) / speed)` 检查，不扣转场、不探测素材**，见 E）。
2. **逐 clip 字段**（数值越界一律 `INVALID_ARGUMENT`，v1 是悄悄截断，v2 改为报错）：`id` 匹配 `^[A-Za-z0-9_-]{1,64}$` 且工程内唯一；`trackId` 匹配 `V1~V8`（视频）/ `A1~A8`（音频）；所有数值必须是有限数（拒绝 NaN / Inf）；`startSec ≥ 0`；`inSec ≥ 0`；**`outSec > inSec`，`outSec = 0` 或 `outSec ≤ inSec` 一律 `INVALID_ARGUMENT`**；`speed` 0.25~4（空(0)= 1）；`volume` 0~4；`blur` 0~4；`effectPreset` / `transitionToNext` 不在枚举内；`transitionDurationSec`：**显式设置**（> 0）时范围 0.1~2，且不超过相邻两个 clip 中较短者的一半（clip 时长 = `(outSec − inSec) / speed`），越界 `INVALID_ARGUMENT`；**空(0)= 默认 0.5 秒**，默认值超过较短者的一半时**不报错**：① **静默缩短到较短者时长的一半**；② 一半**不足 0.1 秒**（即较短者 < 0.2 秒）则该转场**不生效**（这两个 clip 直接 `concat`），并给警告 `transition_ignored`（6.11.2 D）。显式值不做缩短，也不因为"一半不足 0.1 秒"放行（显式 0.1 起步，超限就是超限）。**clip 折算后的时长（`(outSec − inSec) / speed`）不足 0.04 秒 → `INVALID_ARGUMENT`**（否则导出 0 帧；素材截断到素材时长之后再算一次，见第 3 条）。
3. **逐 clip 路径**：必须绝对路径且不含控制字符（含换行，否则会破坏 `detail` 的行格式）——**含控制字符的路径直接 `INVALID_ARGUMENT`，不探测、不访问文件系统**（`detail` 第一行的 `path=` 里控制字符一律替换成 `?`，保证仍是单行）；不存在 `NOT_FOUND`；是目录 `INVALID_ARGUMENT`；无读权限 `IO_ERROR`；ffprobe 失败 `PROBE_FAILED`。视频 clip 的素材必须有视频流，音频 clip 的素材必须有音频流，否则 `INVALID_ARGUMENT`。`inSec ≥ 素材时长` → `INVALID_ARGUMENT`；`outSec` 超过素材时长 **0.05 秒以内静默取整到素材时长，不报警告**（浮点 / 毫秒吸附误差，不值得打扰用户）；更大则截断到素材时长并记警告 `out_truncated`。截断 / 取整之后 clip 时长仍要满足第 2 条的 ≥ 0.04 秒，否则 `INVALID_ARGUMENT`。素材探测复用 `MediaService` 的探测实现与缓存（超时 30 秒、根 ctx 取消返回 `CANCELED`）；**探测结果缓存的 key 含路径、文件大小和修改时间**（文件被替换或改动即失效，不会拿旧的时长去校验）。

**A. 同一轨道上的 clip 不得重叠（架构师定，独立一条）**
- 适用于每条视频轨（`V1~V8`）和每条音频轨（`A1~A8`）。不同轨道之间可以重叠（画中画 / 混音请用不同轨道）。
- 判定：同一 `trackId` 的 clip 按 `startSec` 升序（相同则按数组下标）排序，取相邻的两个 clip；前一个的结束时间 `end = startSec + (outSec − inSec) / speed`；**后一个的 `startSec` 早于前一个的 `end` 即重叠**，返回 `INVALID_ARGUMENT`。两个值比较前都先四舍五入到毫秒（`round(x*1000)`），避免浮点误差（这是本条的实现细则：前端时间也应吸附到毫秒整数）。
- **间隙**：`后一个.startSec − 前一个.end ≤ 0.12 秒`视为首尾相接（可以带转场，沿用 v1 阈值）；`> 0.12 秒`视为空隙——**导出时空隙补黑场（视频）和静音（音频）**，不报错，只在 `ValidateProject` 的 `warnings` 里给 `clip_gap`（见 D）。第一个 clip 的 `startSec > 0.12` 同理是片头黑场，警告 `leading_gap`。
- **实测（ffmpeg 7.1.5，箱子上用 6.11.3 的滤镜图）**：`V1` 上 0~2 秒一个 clip、3~5 秒一个 clip，6 秒时间线在 2.5 秒和 5.0 秒处的亮度均值 `YAVG=16`（黑），1.0 / 3.5 秒处 122.9 / 125.9（有画面）；`A1` 上 0~1 秒、3~4 秒各一个 clip，1~3 秒 `mean_volume` −90.3 dB、4~6 秒 −80.8 dB（静音，编码底噪），0~1 / 3~4 秒 −21.0 dB。也就是空隙自然是黑场 / 静音，不需要额外补丁。
- **同一轨道重叠时 ffmpeg 不会报错**（实测：把第二个 clip 平移到 1 秒，命令退出码 0，后一个盖在前一个上面），所以必须由契约这一条在提交前拦住，不能靠 ffmpeg 兜底。
- `detail` 第一行的 `clip=` 指**后一个** clip（即 `startSec` 更晚、被判定为"压到前一个"的那个）；**第二行固定是 `overlaps=<前一个 clip.id>`**（例如 `clip=c2 path=C:\Videos\a.mp4` 换行 `overlaps=c1`，见 F 的示例）。

**B. `detail` 第一行格式（写死，前端用正则取）**
- 有 clip：`clip=<clip.id> path=<绝对路径>`；没有 clip 的工程级错误：`project`。正则：`^(?:clip=([A-Za-z0-9_-]{1,64}) path=(.*)|project)$`（`path` 取到行尾，路径里可以有空格；路径不含换行，见上）。
- 第二行起才是原因，前端不得解析；`clip=` 永远指**第一个校验失败的 clip**（顺序见上），重叠指后一个。

**C. clip id**：字符集 `[A-Za-z0-9_-]`、1~64 字符（前端用 ULID 或自增串即可）。原因：id 出现在 `detail` 第一行，含空格 / 换行 / `=` 会让上面的正则失效。

**D. `warnings` 稳定结构与首批 code**（`EditWarning{code, clipId?, message}`；`code` 是稳定枚举，**只追加**，新增要走契约版本变更并在此列出；前端按 `code` 出文案，未知 `code` 走通用文案"存在提示"，不得报错）：

| code | 触发（`ValidateProject` 与 `Export` 前的校验相同；`Export` 不因警告失败） | `clipId` |
|---|---|---|
| `clip_gap` | 同一轨道相邻两个 clip 之间的空隙 > 0.12 秒（导出补黑场 / 静音） | 后一个 clip |
| `leading_gap` | 视频轨或音频轨上第一个 clip 的 `startSec` > 0.12 秒（片头黑场 / 静音） | 该 clip |
| `no_audio_track` | `audioTrack` 为空，导出静音音轨；**不会**回退用视频自带音频 | 省略 |
| `out_truncated` | `outSec` 超过素材时长 0.05 秒以上，已截断到素材时长 | 该 clip |
| `transition_ignored` | clip 设了 `transitionToNext` 但转场不生效：① 它后面没有同轨首尾相接的 clip（空隙 > 0.12 秒或它是最后一个）；② 用的是默认时长（`transitionDurationSec` = 0），且相邻较短 clip 的一半不足 0.1 秒（无法缩到最小转场时长） | 该 clip |

**E. 时长口径（架构师定）**
- **`EditPlan.durationSec`（`ValidateProject` 返回）= 扣除转场重叠后的实际时长**：clip 时长 `d = (outSec − inSec) / speed`（`outSec` 已按素材时长截断）；同轨一串首尾相接的 clip 用 `xfade` 连接时，这一串的长度 = 第一个 clip 的 `startSec` + Σ`d` − Σ（实际生效的转场时长，即上面缩短后的值，不生效的转场不扣）；其余 clip 是 `startSec + d`；`durationSec` 取所有视频、音频串 / clip 的最大值。导出进度 `outTimeSec / durationSec` 用它（6.11.3）。
- **工程级 6 小时上限（6.11.2 第 1 条）按 clip 自填值检查、不扣转场**：即 `max(startSec + (outSec − inSec) / speed)`，用 clip 里填的 `outSec`（未按素材截断，不探测）。所以一个不扣转场为 6 小时零几秒、扣掉转场后不足 6 小时的工程仍然 `INVALID_ARGUMENT`。
- **`SaveProject` 返回的 `EditProjectMeta.durationSec` 是按 clip 自填值估算的**（同上一条的 `max(startSec + (outSec − inSec) / speed)`，**不探测素材、不扣转场**），只用于工程列表显示，可能与 `ValidateProject` 的 `durationSec` 不同（6.11.5）。

**F. 示例（数值是示意）**
```json
// ValidateProject 请求（节选）
{ "schemaVersion": 1, "id": "", "name": "旅行 vlog", "sources": ["C:\\Videos\\a.mp4"],
  "output": { "format": "mp4", "width": 1920, "height": 1080, "fps": 30 },
  "videoTrack": [
    { "id": "c1", "path": "C:\\Videos\\a.mp4", "trackId": "V1", "startSec": 0, "inSec": 0, "outSec": 2, "speed": 1,
      "effectPreset": "none", "transitionToNext": "none", "transitionDurationSec": 0, "blur": 0 },
    { "id": "c2", "path": "C:\\Videos\\a.mp4", "trackId": "V1", "startSec": 3, "inSec": 2, "outSec": 4, "speed": 1,
      "effectPreset": "none", "transitionToNext": "none", "transitionDurationSec": 0, "blur": 0 } ],
  "audioTrack": [], "effects": { "brightness": 0, "contrast": 1, "saturation": 1, "sharpen": 0 } }
```
```json
// ValidateProject 返回
{ "durationSec": 5, "clipCount": 2, "inputs": ["C:\\Videos\\a.mp4"], "hasAudio": false,
  "warnings": [ { "code": "clip_gap", "clipId": "c2", "message": "V1 上 c1 与 c2 之间有 1 秒空隙，导出时补黑场" },
                { "code": "no_audio_track", "message": "音轨为空，导出为静音" } ] }
```
```json
// 同轨重叠：AppError（c2 的 startSec 早于 c1 的结束）
{ "code": "INVALID_ARGUMENT", "message": "同一轨道上的片段重叠",
  "detail": "clip=c2 path=C:\\Videos\\a.mp4\noverlaps=c1" }
```
```json
// Export 请求 / 返回
{ "project": { "...": "同上" }, "options": { "outputName": "旅行 vlog", "outputDir": "C:\\Users\\me\\Videos\\FFmpegFree" } }
{ "id": "01J9Z7A1B2C3D4E5F6G7H8J9K0", "type": "edit_export", "status": "queued", "title": "旅行 vlog.mp4",
  "inputPaths": ["C:\\Videos\\a.mp4"], "outputPath": "C:\\Users\\me\\Videos\\FFmpegFree\\旅行 vlog.mp4",
  "progress": 0, "version": 1 }
```
```json
// task:progress（edit_export，没有 fps / bitrateKbps / droppedFrames）
{ "id": "01J9Z7A1B2C3D4E5F6G7H8J9K0", "version": 5, "progress": 0.42, "speed": "1.8x", "etaSec": 6.1, "outTimeSec": 2.1 }
```

### 6.11.3 导出任务 `edit_export`

- 走 batch 池（与转换共用并发数），不占 live 池。`title` 形如 `<outputName>.mp4`；`inputPaths` = 去重后的素材路径（按首次出现顺序）；`outputPath` = 预期输出；`params` = `{project, options, outputDir}` 的 JSON。
- 输出：`<outputDir>/<outputName>.<format>`，重名追加 `(1)`、`(2)`，绝不覆盖，走 6.6 的 `RunWithPart`（`.part.<ext>` → 原子改名）；取消 / 失败删除 `.part`。
- 命令：一个 filtergraph（见下「命令行长度」），语义**沿用 v1**：黑色底画布 → 每个 clip `trim` + `setpts=(PTS-STARTPTS)/speed` + `fps` + `scale`（等比缩进 + 黑边）+ 预设 / 全局效果 + `boxblur` → 同轨且首尾相接（间隙 ≤ 0.12 秒）的 clip 用 `xfade`（有转场）或 `concat`（无转场），其余按 `startSec` 平移后 `overlay` 到画布，轨道编号大的在上；音频：`atrim` + `atempo`（速度 > 2 或 < 0.5 链式拆分）+ `volume` + `adelay` → `amix`（`normalize=0`）→ 截到时间线总长；音轨为空时导出静音（`anullsrc`），**不会**回退使用视频自带音频（v1 行为；想用视频原声，前端把同一素材再加进音轨）。
- **命令行长度**：filtergraph 写入任务专属临时目录里的 UTF-8 文本文件（滤镜图里的 `\n` 换行在 7.1.5 和 9.0.2 上实测可用，退出码 0），**用 `-/filter_complex <file>` 传给 ffmpeg，探测到它不可用才用 `-filter_complex_script <file>`**（择一由 6.11.2 第 0 条的功能探测决定，不按 ffmpeg 版本号判断；**9.0 已移除 `-filter_complex_script`**，项目默认安装 9.0.2，所以主路径是 `-/filter_complex`），避免 Windows 命令行 32 K 上限；任务结束后删除该目录。素材路径仍按 6.9 规则写成 `file:<路径>`。已知：ffmpeg 7.1.5 上 `-filter_complex_script` 会在 stderr 打印一行 `-filter_complex_script is deprecated, use -/filter_complex … instead`（`-/filter_complex` 没有这行），功能正常；该警告行不参与错误分类，日志里保留即可。两个选项都不可用返回 `UNSUPPORTED`（`missing=filter_complex`），见 6.11.2 第 0 条。
- **输出文件名**（`outputName` 净化，架构师定；放开中日韩，不再限制为 `[a-zA-Z0-9_-]`；净化函数 `SanitizeFileName` 是**共用函数**，直播存档（6.10）也调用它）：① `outputName` **为空或纯空白（去首尾空白后为空）→ 用工程名**（工程名也空白 → `edit`）；② 先做 **Unicode NFC 规范化**，再删除：控制字符（U+0000~U+001F、U+007F~U+009F）、**Unicode 格式类字符**（U+200B~U+200F、U+202A~U+202E、U+2066~U+2069、U+FEFF；零宽字符和双向控制符会让文件名"看起来一样"或反向显示）、路径分隔符和 Windows 非法字符 `\ / : * ? " < > |`；③ 去掉首尾空白和**尾部的点与空格**（Windows 会静默吞掉它们）；④ Windows 保留设备名一律避开，**取名字里第一个 `.` 之前的部分**（`NUL.foo`、`con.tar.gz` 也命中），不区分大小写，保留名为 `CON PRN AUX NUL COM0~COM9 LPT0~LPT9`（含上标数字变体 `COM¹ COM² COM³ LPT¹ LPT² LPT³`）：命中时在整个名字前加下划线（`CON` → `_CON`，`NUL.foo` → `_NUL.foo`）；⑤ 长度上限：先按 Unicode 字符（rune）截断到 **100 个字符**，再检查 **UTF-8 字节数 ≤ 200**，超了就从末尾逐个 rune 删到 ≤ 200（**不得切开一个字符**），然后再做一次 ③④；⑥ 以上处理后为空（**净化之后才变空**，例如名字全是 `?:*` 或零宽字符）→ **直接用 `edit`，不回退工程名**（与①的"输入为空用工程名"是两回事：用户写了名字但全被净化掉，用工程名会让文件名与用户输入毫无关系）。直播存档额外禁止 `| ' [ ]`（替换为 `_`）。所有平台使用同一套规则（避免工程在 Mac 上导出、拷到 Windows 出问题）。最终文件名 = `<净化名>.<format>`，重名再追加 `(1)`、`(2)`（6.6）。**净化是静默处理，不报错；路径超长才报错，见下一条。**
- **输出路径长度（Windows，架构师定）**：在 **`Export` 提交时**（同步返回，**不放到任务里失败**）计算并校验，只在 Windows 上启用（其他平台只受上一条的字节数限制）。长度按 **UTF-16 码元数**算，上限 **259**（`MAX_PATH` 260 含结尾 NUL）。计算：`len(输出目录, 已 Clean 的绝对路径) + 1（分隔符，目录以分隔符结尾则不加）+ len(净化名) + len(扩展名含点) + 4（最坏情况的 "(99)" 后缀，见 `task.UniquePath` 的 `%s(%d)%s` 格式，没有空格）+ 5（".part"，`PartPath` 把它插在扩展名前）≤ 259`。也就是说**为 `.part` 和 `(99)` 一律预留 9 个字符**，不管这次实际会不会重名。超出返回 `INVALID_ARGUMENT`，`detail` 第一行 `project`，第二行 `path_length=<实际计算值> limit=259`；**不自动截断名字**（名字的截断只由上一条的 100 字符 / 200 字节规则完成）。拒绝以 `\\?\`、`\\.\` 开头的 `outputDir`（`INVALID_ARGUMENT`）。UNC 路径（`\\server\share\…`）整条计入。**未在 Windows 真机验证**（Linux 箱子只能验证算式，实测：目录 `C:\Users\someone\Videos\FFmpegFree`（34 字符）+ `.webm` 时名字最多 209 字符，被 100 字符规则先挡住）。
- **输出目录在提交时就校验**：`outputDir`（含从"第一个 clip 所在文件夹"推出的）必须是绝对路径（`INVALID_ARGUMENT`）；已存在则必须是目录（`INVALID_ARGUMENT`）且可写（在其中创建再删除一个临时文件，失败 `IO_ERROR`）；不存在则最近的已存在上级必须是可写目录，任务开始时再创建。这样磁盘 / 权限错误在 `Export` 返回，而不是任务开始后才失败。
- **并发提交（架构师定）**：导出期间后端**允许再次提交** `Export`（同一工程或别的工程），新任务走 batch 池 FIFO 排队，与转换共用并发数；同名输出靠 `RunWithPart` 的占用登记各取不冲突的名字。**后端不判断"同一工程正在导出"**（`EditProject.id` 可能为空，草稿也能导出）。**前端职责**：同一工程有 `queued` / `running` 的 `edit_export` 任务时，禁用"导出视频"按钮（按前端自己记录的工程 id → taskId 对应关系，任务进入终态后恢复）。
- **`.part` 遗留清理（#30 实现反馈修订，架构师收紧；以 feat/edit-impl 头 `5d12670` 的实现为准）**：**首版只在 EditService 里对 `edit_export` 做；任务管理器统一版（覆盖 `convert`、`office_pdf`）后续单独做。** 启动时（`MarkInterrupted` 之后）逐个处理 `status=interrupted` 的 `edit_export` 任务记录，**下面五个条件缺一不可，任何一个不满足就不删**：
  1. **文件名符合本应用 `edit_export` 产生 `.part` 的命名**：`task.PartPath` 的规则是"最终文件名去掉扩展名，加 `.part`，再加回原扩展名"（`a.mp4` → `a.part.mp4`，`.part` 插在扩展名前，不是追加在末尾）。候选文件**只有**由该任务的 `outputPath`（`<dir>/<name>.<ext>`）推出的这 100 个：`<dir>/<name>.part.<ext>`，以及重名时 `UniquePath` 可能占用的 `<dir>/<name>(n).part.<ext>`（`n = 1..99`，与 6.11.3 的 `(99)` 预留一致）。**不是"目录里所有 `*.part*`"**：不做通配匹配、不按后缀扫描。
  2. **位于应用登记过的输出目录**：这里"登记"的含义是——该目录是某个 `interrupted` 的 `edit_export` **任务记录里登记的 `outputPath` 所在目录**（`outputPath` 是提交时 `resolveOutputDir` 解析出的最终输出目录 + 文件名，`params` 里另存 `outputDir`；来源可能是 `Export` 的 `opts.outputDir`、`Settings.defaultOutputDir`，或第一个视频 clip 所在文件夹，**解析结果已落库，清理只认落库的这个值**）。**不是任意目录，也不会单独去扫描 `Settings.defaultOutputDir`**：默认输出目录只有在某个中断任务实际用过它时才会被涉及。`outputPath` 为空或不是绝对路径的记录跳过。
  3. **修改时间早于本次启动**：文件 `ModTime` 早于 `EditService` 本次构造时记下的启动时间（避免误删本次运行刚建的）。
  4. **仅普通文件**：对候选路径用 `os.Lstat`（**不跟随符号链接**）判断，`Mode().IsRegular()` 才删；符号链接、目录、设备文件一律不动。
  5. **不递归，只看输出目录第一层**：只处理上面 1 里精确推出的候选路径（都在 `outputPath` 的同一层目录里），不 `ReadDir`、不 `WalkDir`、不进子目录，不碰用户其它文件。
  另：候选不存在不算错误；删除失败只记日志（路径可记）；单次启动最多处理 5000 条中断任务记录（每页 200）。测试必须覆盖：命中候选被删；最终文件（无 `.part`）、名字相近的 `other.part.mp4`、`convert` 任务的输出不被删；`ModTime` 晚于启动的不删；符号链接不删；`outputPath` 是相对路径的记录被跳过。**已知边界**：`Lstat` 只保证候选文件本身不是链接，不检查 `outputPath` 的上级目录是否含符号链接（上级链接会被跟随）；首版接受，因为候选路径来自本应用自己落库的记录。
- **提交阶段 `os.Link` 不可用时的回退**：FAT / exFAT / 部分网络盘不支持硬链接，`commitPart` 已经是"`os.Link` 失败且**目标不存在**才 `Rename`，目标已存在返回 `errTargetExists` 换下一个名字"，本契约要求保持这一点。已知的残余竞态：检查和 `Rename` 之间目标被别的程序创建，在 Windows 上 `os.Rename` 会**覆盖**它（`MoveFileEx` 带 `REPLACE_EXISTING`）；实现时 Windows 的回退应改用不带 `REPLACE_EXISTING` 的 `MoveFileEx`（`golang.org/x/sys/windows`，已在 go.mod），使"目标存在"变成失败。**此点未在 Windows 真机验证。**
- 编码：mp4 / mov / mkv = `libx264 -preset medium -crf 20` + `aac 192k`（mp4 加 `+faststart`）；webm = `libvpx-vp9 -b:v 2M` + `libopus 128k`。缺少编码器由 ffmpeg 报错，按 6.9 归为 `PROCESS_FAILED`。
- 进度：`outTimeSec / durationSec`，0~1 单调，完成为 1；`task:progress` 载荷不变（`progress / speed / etaSec / outTimeSec`）。**不新增事件**。
- 任务失败错误码：`CONVERT_DISK_FULL`、`IO_ERROR`、`PROCESS_FAILED`（detail 带 ffmpeg 最后 50 行，分类规则同 6.9 / v0.9.1）、`PROBE_FAILED`、`CANCELED` 走任务状态 `canceled`。
- `Retry`：注册 `edit_export` 的重试工厂，用 `params` 重建：重新做 6.11.2 的校验（素材已删除 → `NOT_FOUND`，不产生新任务），输出目录沿用原来解析好的那个。
- 任务创建后再改工程不影响已提交的任务（`params` 已经是快照）。

### 6.11.4 预览方案（不做本地流服务）

1. **不用 `file://`**：Wails WebView 的页面源是 `wails://` / `http://wails.localhost`，`<video src="file:///...">` 会被 WebView 拒绝（Wails 官方 issue #292）。
2. **视频 / 音频预览 = AssetServer `Handler` 挂 `/local/<token>`**，协议、限长、token 生命周期、HEAD、失效处理全部见 **6.13**（EditService 与 DocService 共用）。`GetPreviewURL(path)` 校验：绝对路径、存在、是普通文件、扩展名在 v1 允许列表 `mp4 mov avi mkv flv webm m4v mp3 wav aac m4a flac ogg` 内，否则 `INVALID_ARGUMENT`；成功后到 6.13 的 **edit 登记表**登记，返回 `PreviewURL`。
3. 监视器合成（多个 `<video>` + canvas）与 clip 滤镜的预览（CSS filter / canvas 像素处理）全在前端，和导出的 ffmpeg 效果只是近似，不保证逐像素一致（v1 同）。
4. **验证不通过时的回退方案（首版不实现）**：Windows 真机 Range 续传由用户在预览包里验证；不通过时走 `edit_proxy`（低分辨率短 mp4，≤ 32 MiB，整文件加载）。**首版不做，不新增方法、错误码、任务类型。**

### 6.11.5 工程存取

- **`EditProjectMeta.durationSec`**：`SaveProject` 返回（以及 `ListProjects` 列表）的时长是**按 clip 自填值估算的**，即 `max(startSec + (outSec − inSec) / speed)`，**不探测素材、不扣转场**（6.11.2 E）；精确时长以 `ValidateProject` 的 `EditPlan.durationSec` 为准。
- 表 `edit_projects(id, name, project JSON, updated_at)` 已在第 6 节。`SaveProject`：`id` 空 = 新建（ULID），否则更新（不存在 `NOT_FOUND`）；名称重复允许。**只校验数量上限（架构师定）**：名称去首尾空白后 1~80 字、clip 总数 ≤ 100、`sources` ≤ 100、序列化后 ≤ 1 MiB、`schemaVersion` ≤ 1，超了 `INVALID_ARGUMENT`。**不校验**同轨重叠、`outSec`、`speed` 等取值范围、路径是否存在（草稿可以保存，比如正在拖动中的时间线）；这些只在 `ValidateProject` 和 `Export` 报。所以 `LoadProject` 可能读出不合法的草稿，前端要能显示，导出前再调 `ValidateProject`。
- `LoadProject` 不因素材丢失而失败，缺失路径放 `missingPaths`；`SchemaVersion` 大于 1 → `UNSUPPORTED`。
- 后端不做自动保存，也不做撤销栈；前端需要时自行防抖调用 `SaveProject`。

### 6.11.6 错误码对照（EditService 全部沿用现有码，无新增）

| 场景 | code |
|---|---|
| 参数 / 范围 / 枚举不合法、`outSec` ≤ `inSec`（含 0）、同轨重叠、clip 与素材流不匹配、目录当文件、`outputDir` 非绝对、Windows 输出路径超长 | `INVALID_ARGUMENT` |
| 素材文件或工程 id 不存在 | `NOT_FOUND` |
| ffmpeg / ffprobe 缺失 | `FFMPEG_NOT_FOUND` |
| 素材无法解析 | `PROBE_FAILED` |
| 读写文件失败、无权限 | `IO_ERROR` |
| 输出磁盘满（任务错误） | `CONVERT_DISK_FULL` |
| ffmpeg 非零退出、缺编码器 / 滤镜 | `PROCESS_FAILED` |
| 工程 `schemaVersion` 过新；本机 ffmpeg 既不支持 `-/filter_complex` 也不支持 `-filter_complex_script`（`detail` 第一行 `project`、第二行 `missing=filter_complex`） | `UNSUPPORTED` |
| 应用退出导致调用中断 | `CANCELED` |
| 其他 | `INTERNAL` |

### 6.11.7 真机试用清单（Edit，未验证项汇总）

以下项目**没有在真机上验证**（箱子是 Linux + ffmpeg 7.1.5），契约里已就地标"未验证"。**不阻塞实现**：实现按契约写，试用包出来后由用户逐项确认。

| # | 未验证项 | 在哪里 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|---|
| 1 | Windows 输出路径 259 字符上限的算式（UTF-16 码元数、预留 `.part` 和 `(99)` 共 9 个字符）与真实 `MAX_PATH` 行为；UNC 路径整条计入 | 6.11.3「输出路径长度」 | Windows 上用接近上限的目录导出，确认不超长的能成功、超长的在提交时返回 `INVALID_ARGUMENT` | 调整预留长度（契约变更） |
| 2 | `MoveFileEx`：Windows 回退改名用不带 `REPLACE_EXISTING` 的 `MoveFileEx`，"目标存在"变失败（**已交叉编译，未真机验证**；**POSIX 上 `rename` 会覆盖已存在的目标，检查与改名之间的竞态仍有残余**，首版接受并在实现里注明） | 6.11.3「`os.Link` 不可用时的回退」 | 在 FAT / exFAT U 盘或网络盘上导出两次同名文件，确认第二次得到 `(1)` 后缀而不是覆盖 | 保持 `os.Rename`，接受残余竞态并记录 |
| 3 | WebView2 收到被截短到 4 MiB 的 `206` 之后是否继续请求后续 Range（Range 续传） | 6.11.4 第 4 点、6.13 第 9 点 | 预览包里播放 > 32 MiB 的视频并拖动进度 | **验证不通过时的回退方案（首版不实现）**：走 `edit_proxy`，见 6.11.4 第 4 点 |
| 4 | `HEAD` 请求在 WebView2 里的实际表现，`token` 失效 404 后前端重新 `GetPreviewURL` 的流程 | 6.13 | 预览包里让 token 失效（删除素材后）再播放 | 前端改为直接重新 `GetPreviewURL` 不探测 |

## 6.12 DocService 契约（v0.12，只有契约，架构师冻结前不实现）

依据：v1 `master` 上 `backend/contollers/office_controller.go`、`pdf_controller.go`、`frontend/src/views/OfficeConvert.vue`、`PDFPreview.vue`。v1 真实功能：Office → PDF（**纯 Go**，`archive/zip` + `encoding/xml` + `excelize` + `go-pdf/fpdf`，不用 LibreOffice）、PDF 上传 / 列表 / 删除、PDF 预览（前端 `@tato30/vue-pdf`：缩放、翻页、缩略图侧栏、历史列表）。v1 **没有** PDF 合并 / 拆分 / 旋转 / 提取 / 加水印 / 文本提取 / OCR，v2 首版同样不做。

> **合并顺序（架构师最新决定，三个 PR 说明一致）**：**#22 先合**，然后 #19、#23；本节引用的 6.13 由 #22 引入，所以 #23 必须在 #22 之后合入。

> **架构师新增决定（写死，逐条对应下文）**：① `DocCapabilities` 增加 `experimental`（bool，后端给出，前端据此显示"实验性"，6.12.2）；② `PDFChunk.data` 的 Go 字段类型就是 `string`，由后端显式 base64 编码，生成的 `models.ts` 里也是 `string`，前端直接 `atob`（6.12.4 第 2 点）；③ `TaskType` 与 #19 / #22 统一（第 3 节）；④ `/local/<token>` 的共用规则移到中立章节 6.13，本节引用；⑤ 字体合规：子集 name 表改名、CI 断言、字体目录 README（6.12.1）。

> **架构师已确认（v0.12 定稿）**：内嵌 Noto Sans SC `.ttf` 子集为主路径（6.12.1）；大文件预览的 Windows 验证与回退（6.12.4 第 3 点）；Office 转 PDF 标"实验性"；CSV / TXT 首版不支持。

### 6.12.1 Office 转 PDF：格式范围（如实）

| 扩展名（不区分大小写） | v2 行为 |
|---|---|
| `.docx` | 支持，**仅文本**：`word/document.xml` 里每个 `<w:p>` 的 `<w:t>` 拼成一段，按顺序输出，折行分页（折行方式见 6.12.1「折行」） |
| `.xlsx` | 支持，**仅单元格文本**：每个工作表先输出 `Sheet: <名称>` 标题，再**按行流式读取**逐行输出（`excelize.OpenReader` + `Rows()` 迭代器，一次只读一行，`rows.Columns()` 取单元格的显示文本，公式取缓存值；**不用 `GetRows`**——它会把整个工作表一次读进内存），单元格间 4 个空格分隔；每个工作表后换页 |
| `.pptx` | 支持，**仅文本**：每张幻灯片一个标题 `Slide <n>` + 该页所有 `<a:t>` 文本按段落输出，每页幻灯片换页；按数字顺序处理（v1 按字符串排序会把 slide10 排在 slide2 前，v2 修正） |
| `.doc` `.xls` `.ppt`（旧二进制格式）、`.odt` `.ods` `.odp` `.rtf` `.pages` `.numbers` `.key`、其他 | `UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=format`，第二行起写明原因（提交时整体校验失败则第二行是出错文件路径）；旧格式提示"请先另存为 docx / xlsx / pptx" |
| `.csv` `.txt` | **首版不支持**（架构师定，v1 也没有），`UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=format`（说明行 "暂不支持该格式"）；以后要加走增量契约版本 |
| 密码加密的 docx / xlsx / pptx（OLE 容器，不是 zip） | `UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=encrypted`（说明行 "加密文档不支持（或旧版格式改了扩展名）…"） |

**明确不支持（输出里没有）**：图片、图表、形状、SmartArt、表格边框与合并单元格、页眉页脚、脚注、批注、修订、字体 / 字号 / 颜色 / 加粗等样式、页面大小与方向（一律 A4 纵向）、分栏、超链接（只保留文字）、公式的重新计算、幻灯片母版与动画、xlsx 的图表与条件格式。这是"提取文字后重排"，**不是**版式保真转换；想要版式保真需要 LibreOffice 或商业库，不在本项目范围（纯 Go 没有可用的开源保真实现）。**Office 转 PDF 在界面上标"实验性"**（架构师定）：转换页标题 / 入口带"实验性"标签，并常驻一条说明"仅提取文字重新排版，不保留图片和样式"，文案由前端定。

**字体**（影响是否能转换；架构师定：**内嵌字体为主路径，系统字体为补充**）：`fpdf` 只能嵌入 `.ttf`（TrueType 轮廓），**不能加载 `.ttc`**（箱子上实测用系统 `NotoSansCJK-Regular.ttc` 报 `get metrics Error: not supported`）；只用 Helvetica 等内置字体时，任何 U+00FF 以上的字符（含中日韩）会变成乱码（箱子上实测 `你好` 输出为 `ä½ å¥½`）。规则：
- **内嵌字体（主路径）**：程序用 `go:embed` 内嵌 **Noto Sans SC 子集，必须是 `.ttf`（TrueType 轮廓 `glyf`，不得使用 `.otf` / `.ttc`，也不得是可变字体——`fvar` 表要实例化掉）**，字重 Regular（wght 400），通过 `fpdf.AddUTF8FontFromBytes` 加载，不落盘、不依赖系统。文件放 `internal/service/doc/fonts/NotoSansSC-Regular-subset.ttf`，**同目录必须随包带 SIL OFL 1.1 协议文件 `OFL.txt`（原样，不改一个字节）**，并在应用的"关于 / 开源许可"里列出。
- **字体合规（保守做法，不是法律结论）**：下载到的 `OFL.txt` 声明 `Copyright 2014-2021 Adobe … with Reserved Font Name 'Source'`，子集化 / 实例化算修改，所以**子集文件里除版权声明外不得出现 `Source`**：
  1. **name 表**：保留 nameID **0**（版权，仍含 `Reserved Font Name 'Source'` 原文）和 nameID **13 / 14**（OFL 许可文本与 URL）原样；把 nameID **1 / 4 / 6 / 16 / 17** 改成不含 `Source` 的名字（家族名 `FFmpegFree CJK Subset`，全名 `FFmpegFree CJK Subset Regular`，PostScript 名 `FFmpegFreeCJKSubset-Regular`；16 / 17 在样品里本来就不存在，规则是"有就改、没有不加"）；nameID **5 / 7 / 10** 清理（版本串改为 `Version 1.0; subset of Noto Sans SC 2.004 wght=400`，商标 / 描述删除）；nameID 3 改为 `FFmpegFreeCJKSubset-Regular;subset`；nameID 8 / 9 / 11 / 12（厂商 / 设计师 / URL）随子集化一并删除，**设计者署名靠 nameID 0 的版权声明保留**。只保留 Windows 平台英文（platformID 3，langID 1033）记录，去掉 Mac 平台记录。
  2. **CI 单测**（必须写）：读取嵌入的字体，断言 nameID 1 / 4 / 6 / 16 / 17 以及 5 / 7 / 10 里（不区分大小写）不含 `source`；断言 nameID 0 仍含原版权声明和 `Reserved Font Name 'Source'`；断言 nameID 13 存在；断言无 `fvar`、有 `glyf`、无 `CFF `；断言文件 SHA-256 与 README 里记录的一致。Go 侧**不新增 `golang.org/x/image` 依赖**（架构师采纳 #29 实现方案）：由实现自写的最小只读 sfnt 解析读 `name` 表和 `cmap` 表（只读、不写字体、不依赖第三方库；`go.mod` 不因字体而变），CI 单测和缺字统计共用这份解析。解析必须对越界偏移、表长度、`numTables` 等做边界检查，遇到损坏字体返回错误而不是 panic（嵌入字体是构建时固定的，运行时加载的系统字体来自外部文件，更要防）。
  3. **字体目录 README**（必须写，`internal/service/doc/fonts/README.md`）：记录来源文件、来源仓库的**提交 SHA**、来源文件和产物的 SHA-256、fontTools 版本、生成命令 / 脚本、码点集、包体大小、`OFL.txt` 的来源与 SHA-256。**下面是我在箱子上实际做过的，如实记录，实现 PR 直接照抄并复测**：
     - 来源：`https://github.com/google/fonts`，文件 `ofl/notosanssc/NotoSansSC[wght].ttf`，最后修改该文件的提交 `2894aab31764f10f29c421bdfd2340d3b382d384`（2022-12-09，`Noto Sans SC hotfix2 (#5533)`；用 `gh api` 查得）；在该提交下载的文件 SHA-256 = `a3041811a78c361b1de50f953c805e0244951c21c5bd412f7232ef0d899af0da`（17 772 300 字节，与 `main` 上下载的一致）；`OFL.txt` SHA-256 = `1c05c68c34f9708415aada51f17e1b0092d2cea709bf4a94cd38114f9e73d7d9`（4 388 字节，两处一致）。
     - 工具：Python 3.13.5，**fontTools 4.66.0**，brotli 1.2.0（`python3 -m venv v && v/bin/pip install fonttools brotli`）。
     - 命令：`v/bin/python mkfont.py NotoSansSC[wght].ttf NotoSansSC-Regular-subset.ttf`，`mkfont.py` 的做法：`instancer.instantiateVariableFont(f, {"wght": 400}, updateFontNames=False)` → `subset.Subsetter`（`layout_features=["kern","vert"]`、`hinting=False`、`notdef_outline=True`、`desubroutinize=True`、`name_IDs` 保留 0~14/16/17，码点集见下条）→ 改 name 表（上面第 1 条）→ `save`。**脚本本身要随 README 一起提交**，不能只写"用 fontTools 做过"。
     - 产物：**大小和 SHA-256 以 `internal/service/doc/fonts/README.md` 记录的为准，本契约不再写死**（早期样品 2 355 692 字节 / `48c44ed1…`、#29 首版的 2 355 692 字节 / `7907e8b2…` 都已过时，字符集扩充后重新生成）。**当前实现 = 2 741 704 字节，SHA-256 = `b96fad9e311f2f0254f2b3dc4db8ac0fb791c4f7a2211e2f04d02e68dd1f20ae`**（仅供参考，同样以 README 为准；CI 单测断言的是 README 里的值）。`glyf`、无 `fvar`、无 `CFF `；name 表里含 `Source` 的只有 nameID 0（脚本内断言实测：`{0}`）。fpdf v0.9.0 加载并输出 `Hello 你好，世界！こんにちは Àé`，`pdftotext` 取回一致。
- **子集范围**（箱子上已做出样品，见下方实测）：GB2312 全部 6763 个汉字 + GB2312 符号区 + ASCII + Latin-1 + 通用标点（U+2000~206F）+ CJK 标点（U+3000~303F）+ 平假名 / 片假名（U+3040~30FF）+ 全角形式（U+FF00~FFEF）+ 箭头 / 数学符号 / 几何图形（U+2190~21FF、2200~22FF、25A0~25FF）+ **JIS X 0208 第一水准汉字（2965 字，日文常用汉字，含 GB2312 里没有的日本汉字）**，保留 `kern` / `vert` 特性，去 hinting。**不覆盖**：繁体中文专用字、GB2312 与 JIS X 0208 第一水准之外的生僻字（**第二水准生僻字仍显示方框，属刻意取舍**，为控制包体）、谚文、emoji。**字体里没有的字符输出为该字体的 `.notdef` 方框，不视为失败**（实测：GB2312 / JIS 第一水准之外的字确实显示为方框）。
- **包体增量（实测）**：当前实现的字体 `NotoSansSC-Regular-subset.ttf` = **2 741 704 字节（约 2.61 MiB）**（含 JIS X 0208 第一水准；早期只含 GB2312 的样品是 2 355 692 字节 / 约 2.25 MiB，已过时），`OFL.txt` = 4 388 字节；`go:embed` 不压缩，所以可执行文件增加约 **2.61 MiB**（安装包会压缩，早期样品 gzip -9 后约 1.4 MiB，当前版本 7z / NSIS 压缩率**未测**）。**大小和 SHA 以字体目录 README 为准**，实现 PR 描述里必须再报一次包体增量。
- **缺字统计（建议，已写入）**：转换时统计"文档里出现、但当前主用字体没有的字符"（按去重码点计数），任务日志（`task.LogWriter`）末尾写一行 `missing_glyphs=<去重码点数> total=<出现次数> sample=U+XXXX,U+XXXX,…（最多 20 个）`，**只记码点，不记文档文字内容**（避免把用户文档内容写进日志）；没有缺字不写这一行。判断"有没有这个字"以嵌入字体的 cmap 为准（用上面自写的只读 `cmap` 解析，不引入 `x/image/font/sfnt`）。缺字仍输出 `.notdef` 方框，不视为失败。
- **系统字体（补充）**：后端按顺序找第一个存在且可加载的 `.ttf`：Windows `C:/Windows/Fonts/simhei.ttf`、`simsun.ttf`（`msyh.ttf` 仅在旧系统存在；新版 Windows 自带的雅黑通常是 `msyh.ttc`，`fpdf` 加载不了，**未在 Windows 真机验证**），macOS `/Library/Fonts/Arial Unicode.ttf`，Linux `/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`。**按文档选字体、不做逐字回退**（`fpdf` 一份文档一个当前字体）：先用内嵌字体；若文档含内嵌字体 cmap 未覆盖的字符，且系统字体 cmap 覆盖了这些字符（如繁体字用 Arial Unicode），则整份文档改用该系统字体；否则仍用内嵌字体（缺字为方框）。
- **`UNSUPPORTED` 规则保留**：内嵌字体加载失败（构建错误才会发生）**且**没有可用系统 `.ttf`，而文档又含 U+00FF 以上的字符 → 该文件 `UNSUPPORTED`，`message` "没有可用的 Unicode 字体"，`detail` 首行 `reason=no_font`（不输出乱码 PDF）；纯 Latin-1 文档可用内置字体。正常构建下内嵌字体总是可用，该分支基本不会触发，但校验和单元测试要覆盖。
- **折行（自行折行，#29 实现）**：**有 Unicode 字体（内嵌字体或可加载的系统字体，即 `DocFont.available=true`）时由后端自己折行**：按字体度量逐字符累计宽度，超出版心宽度就换行；CJK 文字可在任意两个字符之间断行，拉丁文字 / 数字按空格断词（单词超宽才强制断开）；**简化的行首行尾禁则**：行首不得出现的标点（`，。、；：？！）》」』】〕…—` 及半角 `,.;:?!)]}` 等）和行尾不得出现的标点（`（《「『【〔([{` 等）不落在不允许的位置，违反时把该标点与相邻字符一起挪到同一行（**只处理单个标点，不做连续标点、完整 JIS X 4051 避头尾规则**）。**没有 Unicode 字体时**（内嵌字体加载失败且无系统 `.ttf`，只能用内置 Helvetica，文档只能含 Latin-1，见下条 `UNSUPPORTED` 规则）**退回 `fpdf` 自带的 `MultiCell`**（无禁则）。这是实现细节，不改任何接口；测试：含长中文段落、行首标点的文档，`pdftotext` 取回文字无丢失、行首不出现 `，。`。
- DejaVu Sans 不含 CJK 字形，只作为系统补充里的拉丁 / 希腊 / 西里尔备选，不再是 CJK 的判据。`GetDocCapabilities.font` 的语义相应调整：`available` = 内嵌字体或系统字体至少一个可用；`name` = 主用字体（`noto-sans-sc-embedded` 或系统字体名）；`cjk` = 主用字体是否覆盖 GB2312 汉字（内嵌字体为 `true`）。

### 6.12.2 数据结构

```go
type DocCapabilities struct {
    Formats      []DocFormat `json:"formats"`      // 固定列表：docx xlsx pptx（supported=true，fidelity="text-only"）+ doc xls ppt odt ods odp rtf csv txt（supported=false，reason 给出原因；csv / txt 的 reason 是"暂不支持该格式"，首版不做）
    Font         DocFont     `json:"font"`
    Limits       DocLimits   `json:"limits"`
    Experimental bool        `json:"experimental"` // 后端给出：Office 转 PDF 是否在界面上显示"实验性"；当前恒为 true（仅提取文字重排，不保留版式），以后版式保真了改成 false，前端不写死
}
type DocFormat struct {
    Ext       string `json:"ext"`       // 不带点，小写；docx xlsx pptx doc xls ppt odt ods odp rtf csv txt
    Supported bool   `json:"supported"`
    Fidelity  string `json:"fidelity"`  // "text-only"（支持的三种）| ""
    Reason    string `json:"reason"`    // 不支持时的原因
}
type DocFont struct {
    Available bool   `json:"available"` // 是否找到可用 .ttf
    Name      string `json:"name"`      // 主用字体：noto-sans-sc-embedded（内嵌，正常构建下恒为它）| simhei | msyh | simsun | arialunicode | dejavu | ""（都不可用）
    Cjk       bool   `json:"cjk"`       // 主用字体是否覆盖 GB2312 汉字；noto-sans-sc-embedded 为 true
}
type DocLimits struct {
    MaxInputsPerSubmit int   `json:"maxInputsPerSubmit"` // 50
    MaxInputBytes      int64 `json:"maxInputBytes"`      // 100 MiB
    MaxPages           int   `json:"maxPages"`           // 5000
    MaxPDFBytes        int64 `json:"maxPdfBytes"`        // 512 MiB（OpenPDF）
    ChunkBytes         int   `json:"chunkBytes"`         // 1 MiB（ReadPDFChunk 的 length 上限，按**原始字节**计；base64 编码后一块约 1.4 MiB）
    WholeLoadBytes     int64 `json:"wholeLoadBytes"`     // 64 MiB（前端整份读入内存的上限，见 6.12.4）
}
type PDFSource struct {
    ID   string `json:"id"`   // 句柄（128 位随机，进程内有效，重启失效）；同一路径复用同一 id
    Path string `json:"path"`
    Name string `json:"name"`
    Size int64  `json:"size"` // 字节
    URL  string `json:"url"`  // /local/<32 位十六进制 token>（6.13 的 doc 登记表），仅在 size > WholeLoadBytes 时前端使用，见 6.12.4；token 与 id 是两回事；被 RemoveRecentPDFs 撤销或失效后 404
}
type PDFChunk struct {
    Offset int64  `json:"offset"`
    Length int    `json:"length"` // 实际读到的**原始字节数**（base64 解码后的长度，不是 Data 的字符数）；单块上限 1 MiB
    EOF    bool   `json:"eof"`    // offset+length >= 文件当前大小
    Size   int64  `json:"size"`   // 本次读取时文件的当前大小；与 OpenPDF 返回的 size 不同说明文件读取期间被改动，前端应重新 OpenPDF
    Data   string `json:"data"`   // Go 字段类型就是 string：后端对读到的原始字节显式 base64 编码（标准字母表 base64.StdEncoding，含 = 填充）；models.ts 里同为 string，前端直接 atob。Length 是原始字节数，不是 Data 的字符数
}
type PDFFile struct {
    ID       string `json:"id"`       // doc_recent.id（ULID）
    Path     string `json:"path"`
    Name     string `json:"name"`
    Size     int64  `json:"size"`
    OpenedAt int64  `json:"openedAt"` // Unix 毫秒
    Exists   bool   `json:"exists"`   // 列表时 stat 的结果，文件已删为 false（记录保留，用户手动移除）
}
```
表 `doc_recent(id PK, path, path_key UNIQUE, name, size, opened_at)`（第 6 节补一行，迁移新文件）。**只保留最近 1000 条**：`OpenPDF` 在同一事务里插入 / 更新后，按 `opened_at` 倒序删除第 1000 条之后的记录（只删记录，不删文件，也不撤销仍在使用的句柄之外的东西）；`path_key` 规则同第 1 节（Windows / macOS 小写）。

### 6.12.3 `ConvertToPDF`：任务 `office_pdf`

- 参数校验与 6.9 同一套规则：`inputs` 非空且 ≤ 50，路径必须绝对（`INVALID_ARGUMENT`），文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读权限 `IO_ERROR`；`outputDir` 规则同 6.9（空 = `Settings.defaultOutputDir`，仍空 = 源文件所在文件夹），并在**提交时**同步校验（不放到任务里失败）：必须是绝对路径；**拒绝以 `\\?\`、`\\.\` 开头的路径**（`INVALID_ARGUMENT`）；**拒绝位于应用数据目录之内（含其本身）的路径**（`os.UserConfigDir()/FFmpegFree/`，防止把输出写进 `app.db`、`thumbs/`、`logs/` 旁边并被清理逻辑误伤，`INVALID_ARGUMENT`，`detail` 写 `outputDir 不能在应用数据目录内`；比较前对两边做 `EvalSymlinks` + 大小写按平台规则规范化）；已存在必须是目录且可写，不存在则最近的已存在上级必须是可写目录。**"可写"的判断方式（#29 实现反馈）**：在该目录里**创建一个探测文件**（`os.CreateTemp(dir, ".ffmpegfree-probe-*")`，创建成功后立即关闭并删除），**不用**权限位或 `access()` 推断（Windows 的 ACL、只读挂载、网络盘上权限位不可靠）；创建失败（含权限不足、只读、磁盘满）一律返回 `IO_ERROR`，`detail` 写系统错误文本；探测文件删除失败只记日志，不影响结果。**先整体校验再提交**，任何一个不通过整体失败、不提交任何任务；`detail` 的形式（v0.16 更正）：**有 reason 的错误首行是 `reason=<枚举>`，第二行是出错文件的绝对路径**，其后是原因说明；没有 reason 的错误（相对路径、文件不存在、目录当文件等）首行仍是出错文件路径。前端定位出错文件时在前两行里找绝对路径。
- 整体校验里额外检查：扩展名在支持表内（否则 `UNSUPPORTED`）；文件 ≤ 100 MiB（否则 `INVALID_ARGUMENT`，`message` "文件超过 100 MiB"，`detail` 首行 `reason=too_large`）；能作为 zip 打开且含必需部件（docx `word/document.xml`，xlsx `xl/workbook.xml`，pptx 至少一张 `ppt/slides/slide<n>.xml`），打不开或缺部件 `INVALID_ARGUMENT`（`message` "不是有效的 OOXML 文件"，**`detail` 首行 `reason=invalid_ooxml`**）；**zip 条目数上限 100 000**：打开压缩包之前先只读文件尾部的 EOCD（含 zip64 记录）取条目总数，超过 100 000 → `INVALID_ARGUMENT`（`message` "不是有效的 OOXML 文件"，`detail` 首行 `reason=too_large`，说明行 "压缩包条目数超过 100000"，`internal/service/doc/zipcount.go` 的 `MaxZipEntries` / `checkZipEntries`）；读不出条目数（不是 zip、被截断）就交给后面的 zip 打开报错；不是 zip 而是 OLE 头（`D0 CF 11 E0`）→ `UNSUPPORTED`（加密或旧格式改了扩展名，`message` "暂不支持这种格式"，`detail` 首行 `reason=encrypted`）；单个 zip 条目解压后 > 256 MiB `INVALID_ARGUMENT`（防 zip 炸弹，`message` "不是有效的 OOXML 文件"，`detail` 首行 `reason=too_large`）；中央目录字节数 > 9 600 000（伪造 EOCD 防护）同样是 `reason=too_large`，zip64 目录信息无效（占位符没有 zip64 记录）是 `reason=invalid_ooxml`；字体规则见 6.12.1（需要 Unicode 字体而没有 → `UNSUPPORTED`，`message` "没有可用的 Unicode 字体"，`detail` 首行 `reason=no_font`，此项在提交时对文本做一次快速扫描，不通过整体失败）。
- **不依赖 ffmpeg**（不做 `FFMPEG_NOT_FOUND` 门控）。走 batch 池（与转换共用并发数）；`GoFuncRunner` 实际是 `task.RunnerFunc`。
- 任务：`type=office_pdf`，`title` 形如 `a.docx → PDF`，`inputPaths=[源]`，`outputPath` 为预期输出，`params={input, outputDir}` JSON。输出 `<源文件名去扩展名>.pdf`，重名追加 `(1)`、`(2)`，不覆盖，走 6.6 `RunWithPart`（`.part.pdf` → 原子改名）；取消或失败不留 `.part`。**已知边界（编号最大 99）**：`.part` 遗留清理只精确拼出 `<name>.part.pdf` 与 `<name>(1..99).part.pdf` 共 100 个候选名，`(n)` 大于 99 的残留文件不会被清理（与 `internal/service/doc/cleanup.go` 一致，不通配、不扫目录）。**启动时 `.part` 遗留清理是可选功能（契约"允许"，是否做由实现 PR 决定）**：若做，必须与 #22 的 6.11.3「`.part` 遗留清理」**五个条件完全一致，缺一不可**——① 文件名只能是由 `interrupted` 的 `office_pdf` 任务 `outputPath` 推出的 `<name>.part.pdf` 和 `<name>(n).part.pdf`（n=1..99）；② 位于该任务记录里登记的 `outputPath` 所在目录（不是任意目录，也不单独扫描默认输出目录）；③ 修改时间早于本次启动；④ 仅普通文件，`Lstat` 不跟随链接，符号链接和目录不动；⑤ 不递归，只看输出目录第一层，不 `ReadDir`。任务管理器统一版（覆盖 `convert`、`office_pdf`）后续单独做（见 6.11.3）。
- **进度**：按处理单元计数（docx 段落、xlsx 行、pptx 幻灯片）占总数的比例，0~1 单调，完成为 1；每处理约 100 个单元检查一次 ctx，取消响应 ≤ 1 秒（超大文件除外）。`task:progress` 载荷不变，`speed` / `etaSec` 为空。
- 页数上限 5000：**输出页数超过 5000（生成过程中累计到第 5001 页时立即停止）返回 `UNSUPPORTED`**，**`message` "超过 5000 页"（v0.16 更正：原文写成 detail），`detail` 首行 `reason=too_many_pages`**，第二行是说明（"已排到第 N 页仍未结束"），不产生输出文件（`.part` 删除）；xlsx 一个工作表所有行都算；xlsx 单元格文本每格最多 32 767 字符（Excel 自身上限），超出截断。**页数按"正在生成的 PDF 的页码"统计，折行产生的页也算**（与 `origin/feat/doc-impl` 4d83299 的 `render.go` 一致：每处理完一个单元检查一次 `pdf.PageNo() > 5000`，写长段落时逐行也检查，超过即停止，所以不会生成超过 5000 页的 PDF）。**已知边界**：① 检查粒度是"一个单元 / 一行"，没有 Unicode 字体时（helvetica 兜底路径，只有西文文档会走到）一个超长段落用 `MultiCell` 整段排完才检查，这一段可能超过 5000 页很多再被拒绝（仍是 `UNSUPPORTED`、不产生输出）；② 提取出的文字总量超过 64 MiB 直接按"超过 5000 页"处理（`UNSUPPORTED`，`message` "超过 5000 页"，`detail` 首行 `reason=too_many_pages`，说明行 "文档文字量超过上限"），即使按这些文字排出的页数没到 5000。
- 错误码（任务的 `error`）：`IO_ERROR`（读写失败，没有权限）、`CONVERT_DISK_FULL`（输出写盘失败且是磁盘满，判定规则同 6.9 的系统错误文本匹配；Office 转换也用这个码，前端标题相同）、`UNSUPPORTED`、`INVALID_ARGUMENT`（运行时才发现的损坏，`reason=invalid_ooxml` / `too_large`）、`INTERNAL`（fpdf / excelize 意外错误，`detail` 是错误文本）；取消是任务状态 `canceled`。
- `Retry`：注册 `office_pdf` 的重试工厂，用 `params` 重建并重新校验（输入被删除 `NOT_FOUND`，不产生新任务）。
- v1 的"按文件名防重复转换"（`officeConvertingFiles`）取消：两个任务转同一个输入是允许的，输出各自取不冲突的名字。

### 6.12.4 PDF 预览方案（不做本地流服务）

渲染**完全在前端**：沿用 v1 的 `@tato30/vue-pdf`（pdf.js），后端不渲染成图片、不提供页数 / 文本 / 缩略图接口（后端无纯 Go 的可靠 PDF 渲染器，也不打包 `pdftoppm` 之类外部程序）。后端只负责把字节交给前端：

1. `OpenPDF(path)`：路径必须绝对（`INVALID_ARGUMENT`）、存在（`NOT_FOUND`）、是文件（否则 `INVALID_ARGUMENT`）、可读（`IO_ERROR`）、扩展名 `.pdf`（不区分大小写，否则 `INVALID_ARGUMENT`，`message` "只支持 .pdf 文件"，`detail` 首行 `reason=format`）、前 1024 字节内含 `%PDF-`（否则 `INVALID_ARGUMENT`，`message` "不是 PDF 文件"，`detail` 首行 `reason=format`，第二行是路径）、大小 ≤ 512 MiB（否则 `INVALID_ARGUMENT`，`message` "文件超过 512 MiB"，`detail` 首行 `reason=too_large`；`ReadPDFChunk` 发现文件已变大超限时同样）。成功后登记句柄并写入 / 更新 `doc_recent`。加密 PDF 也能打开，密码由前端 pdf.js 的 `onPassword` 弹窗处理，后端不接触密码。
2. **主路径（size ≤ 64 MiB）：`ReadPDFChunk` 读整份**。前端循环调用 `ReadPDFChunk(id, offset, chunk)` 直到 `eof`，拼成 `Uint8Array` 交给 `usePDF`。只用 Wails Bind，**不依赖 AssetServer 在 Windows 上缓冲响应的行为**（见 6.13 第 9 点）。`chunk` 取 `min(GetDocCapabilities().limits.chunkBytes, 1 MiB)`（1 MiB 的 base64 约 1.4 MiB；Windows 上 Bind 返回值大小是否有上限**未验证**，需要时后端把 `chunkBytes` 调小，前端不用改）。
   - **`Data` 的编码与解码（架构师定）**：`PDFChunk.data` 在 Go 结构体里**直接是 `string`**（不再是 `[]byte`），由后端**显式 `base64.StdEncoding.EncodeToString`**（标准字母表，含 `=` 填充）；因此 Wails 生成的 `frontend/wailsjs/go/models.ts` 里 `PDFChunk.data` 就是 `string`，**前端拿到后直接 `atob`，不需要类型断言**；`length` 是解码后的原始字节数，单块上限 1 MiB，`atob` 解出的字节数必须等于 `length`：`const bin = atob(chunk.data); const bytes = new Uint8Array(bin.length); for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)`（不要用 `Uint8Array.fromBase64`，WebView2 / WebKit 版本不一定有）。**大小口径**：`length` 参数、`chunkBytes`（1 MiB）、`PDFChunk.length` 都按**原始字节**计；base64 编码后 `data` 约为原始的 4/3，一块最多约 **1.4 MiB**（1 MiB → 1 398 104 个字符）。`bytes.length` 必须等于 `chunk.length`（前端校验，见伪代码）。**【未验证】**：没有在真实 Wails 环境里跑过，见 6.12.8 联调项。**实现 PR 仍以生成出来的 `models.ts` 为准**核对字段类型，与这里不一致要回来改契约。
   - `ReadPDFChunk` 每次调用的校验（顺序即优先级，架构师定）：
     1. `length` 范围 1~`chunkBytes`（≤ 1 MiB），越界 `INVALID_ARGUMENT`；`offset < 0` `INVALID_ARGUMENT`；**`offset > math.MaxInt64 − length` 一律 `INVALID_ARGUMENT`**（防 `offset+length` 的 int64 溢出，比较写成减法形式，不写 `offset+length > x`）。
     2. **`id` 只查登记表，不拼路径**：`id` 必须是 `OpenPDF` 返回的句柄（32 位十六进制）；格式不对 / 查不到（重启后失效、被 `RemoveRecentPDFs` 撤销）→ `NOT_FOUND`。真实路径只来自登记表里 `OpenPDF` 时记录的值，任何来自前端的字符串都不参与路径拼接。
     3. **每次重新校验文件**：`EvalSymlinks` 与登记时的真实路径一致，`os.Stat` 仍是**普通文件**，`os.SameFile` 与登记时相同（文件被删 / 被换成链接 / 被换成别的文件 → `NOT_FOUND`）；**当前大小 ≤ `MaxPDFBytes`**（登记后文件被追加变大 → `INVALID_ARGUMENT`，detail "文件超过 512 MiB"）；无读权限 / 读失败 `IO_ERROR`。
     4. `offset ≥ 当前大小` 返回 `length=0, eof=true`；否则读 `min(length, 当前大小 − offset)` 字节。`size` 字段填本次读取时的当前大小。
   - **前端读取循环伪代码**（架构师要求）：
     ```ts
     async function loadPdf(path: string): Promise<Uint8Array> {
       const src = await call(DocService.OpenPDF(path))            // 失败：抛 AppError
       const caps = await call(DocService.GetDocCapabilities())
       if (src.size > caps.limits.wholeLoadBytes) return loadByUrl(src)   // 大文件：走 6.12.4 第 3 点，不在这里读
       const chunkLen = Math.min(caps.limits.chunkBytes, 1 << 20)
       const out = new Uint8Array(src.size)                        // 按 OpenPDF 的 size 预分配；size 变了就重来
       let offset = 0, retried = false
       while (offset < src.size) {
         const c = await call(DocService.ReadPDFChunk(src.id, offset, chunkLen))
         if (c.size !== src.size) {                                // 读取期间文件被改动
           if (retried) throw new AppError('IO_ERROR', 'PDF 在读取时被修改')
           retried = true; return loadPdf(path)                    // 只重来一次
         }
         const bytes = b64ToBytes(c.data)
         if (bytes.length !== c.length) throw new AppError('INTERNAL', '分块长度不一致')
         out.set(bytes, offset); offset += c.length
         if (c.eof) break
         if (c.length === 0) throw new AppError('INTERNAL', '读到 0 字节但未结束')   // 防死循环
         onProgress?.(offset / src.size)
       }
       if (offset !== src.size) throw new AppError('IO_ERROR', 'PDF 读取不完整')
       return out
     }
     ```
     `NOT_FOUND`（句柄失效）时前端重新 `OpenPDF(path)` 换句柄，**只重试一次**。
3. **大文件（64 MiB < size ≤ 512 MiB）**：前端用 `PDFSource.url`（`/local/<token>`）交给 pdf.js 按 Range 加载；协议、限长、`HEAD` 探测和失效重试都按 **6.13**（每个 Range 响应 ≤ 4 MiB；无 Range 的整体请求 ≤ 32 MiB，更大 413，因此大文件必须走 Range）。**此路径在 Windows 上未经验证**（箱子是 Linux），**Windows 真机 Range 续传由用户在预览包里验证**（架构师定）。**Doc 的具体行为（#29 已有测试覆盖，这里写死）**：size > 64 MiB 的 PDF，`OpenPDF` 返回的 `PDFSource.url` 非空；对该 URL 的 **`HEAD` 返回 `200`，带完整的 `Content-Length`（等于文件大小，不是 4 MiB 截断值）、`Accept-Ranges: bytes`、`Content-Type: application/pdf`，没有正文**；`RemoveRecentPDFs` 撤销该 id 之后，**`GET` 和 `HEAD` 都返回 `404`**（token 同时作废，不是只作废句柄）；文件被替换或不再是同一个普通文件时同样 `404`。前端遇到 `404` 重新 `OpenPDF` 取新 `url`。**验证不通过时的回退方案（首版不实现，不新增方法或错误码）**：大文件上限降为 64 MiB，即 `OpenPDF` 对 size > 64 MiB 的文件返回 `INVALID_ARGUMENT`（detail "文件超过 64 MiB"），`PDFSource.url` 恒为空；`WholeLoadBytes` 与 `MaxPDFBytes` 都变成 64 MiB。该降级只改 `OpenPDF` 的一个阈值和文档，不改方法签名。`url` 在 size ≤ 64 MiB 时也会返回，但前端不应使用。
4. 不用 `file://`（WebView 拒绝，同 6.13）；不把整份 PDF 作为 base64 一次返回（会撞 IPC 体积与内存峰值）。
5. 内存：主路径峰值 = 文件大小 × 约 2（分块拼接 + pdf.js 解析），64 MiB 上限据此设定，**阈值是估计值，需真机调**。
6. 历史列表：`ListRecentPDFs` 取代 v1 的"服务器上传目录列表"；不再复制 PDF 到应用目录（v1 上传会拷贝），列表只存路径，文件被移动 / 删除时 `exists=false`。`OpenPDF` 是唯一的写入点；转换产出的 PDF 不自动进历史，前端在转换完成后需要预览时调用 `OpenPDF(outputPath)`。只保留最近 1000 条（6.12.2）。**`ListRecentPDFs(limit)`：`limit ≤ 0` 取默认 20；`limit > 200` 静默截断到 200，不返回 `INVALID_ARGUMENT`**（列表只是取最近若干条，超出上限不算调用错误；测试断言 `limit=201`、`limit=100000` 都返回至多 200 条且不报错）。
7. **`RemoveRecentPDFs(ids)`**：`ids` 是 `PDFFile.id`（`doc_recent.id`，ULID）；**一次最多 500 个**，超过 `INVALID_ARGUMENT`；空列表直接返回 nil；**不存在的 id 忽略，不报错**（重复调用幂等）；只删记录，不删文件。**同时撤销对应的句柄（`ReadPDFChunk` 用的 `PDFSource.id`）和 `/local/<token>`（6.13 的 doc 登记表里同一路径的项）**——按记录的 `path_key` 找到并删除，撤销后 `ReadPDFChunk` 返回 `NOT_FOUND`、`/local/<token>` 返回 404；正在进行中的 HTTP 请求不强制中断。
8. **共用规则**：`PDFSource.url` 走 6.13 的 `doc` 登记表（512 项 LRU、`crypto/rand` token、登记与每次请求都 `EvalSymlinks` + 普通文件 + `SameFile` 校验、支持 `HEAD`、Range 单段 + 4 MiB 限长、`pdf → application/pdf`、token 失效 404）。本节不再重复。

### 6.12.5 事件

不新增事件。转换进度走 `task:created` / `task:progress` / `task:status`，与转换、剪辑一致；预览没有事件。

### 6.12.6 错误码对照（全部沿用现有码，无新增）

| 场景 | code |
|---|---|
| 参数不合法、路径非绝对、`outputDir` 是 `\\?\` / `\\.\` 或在应用数据目录内、目录当文件、不是 PDF、不是有效 OOXML、文件超限（含读取期间变大）、`length` / `offset` 越界或溢出、`RemoveRecentPDFs` 超过 500 个 | `INVALID_ARGUMENT` |
| 输入 / PDF / 句柄不存在或已失效（含被 `RemoveRecentPDFs` 撤销、文件被替换） | `NOT_FOUND` |
| 不支持的格式（旧版 Office、odt、rtf、加密文档）、缺 Unicode 字体、超过 5000 页 | `UNSUPPORTED` |
| 读写失败、无权限 | `IO_ERROR` |
| 输出磁盘满（任务错误） | `CONVERT_DISK_FULL` |
| 应用退出导致调用中断 | `CANCELED` |
| 库内部意外错误 | `INTERNAL` |

**`message` 与 `detail` 首行对照（v0.16，与 `internal/service/doc` 一致；`message` 是精确文案，前端精确匹配，不得改动）**：

| 场景 | code | `message`（精确） | `detail` 首行 | 出现位置 |
|---|---|---|---|---|
| 超过 5000 页（输出页数，含折行；说明行 "已排到第 N 页仍未结束"） | `UNSUPPORTED` | `超过 5000 页` | `reason=too_many_pages` | 任务 `error` |
| 提取的文字总量超过 64 MiB（说明行 "文档文字量超过上限"） | `UNSUPPORTED` | `超过 5000 页` | `reason=too_many_pages` | 任务 `error` |
| 不支持的扩展名（doc / xls / ppt / odt / ods / odp / rtf / csv / txt / pdf / pages…）、没有扩展名 | `UNSUPPORTED` | `暂不支持这种格式` | `reason=format` | `ConvertToPDF` 同步校验 |
| 加密的 Office 文档（OLE 头 `D0 CF 11 E0`） | `UNSUPPORTED` | `暂不支持这种格式` | `reason=encrypted` | `ConvertToPDF` 同步校验 |
| 需要 Unicode 字体而没有 | `UNSUPPORTED` | `没有可用的 Unicode 字体` | `reason=no_font` | 同步校验 / 任务 `error` |
| 不是 zip、空文件、缺必需部件、XML 损坏、zip64 目录信息无效 | `INVALID_ARGUMENT` | `不是有效的 OOXML 文件` | `reason=invalid_ooxml` | 同步校验 / 任务 `error` |
| zip 条目数 > 100 000、中央目录 > 9 600 000 字节、单个条目解压后 > 256 MiB | `INVALID_ARGUMENT` | `不是有效的 OOXML 文件` | `reason=too_large` | 同步校验 / 任务 `error` |
| Office 输入文件 > 100 MiB | `INVALID_ARGUMENT` | `文件超过 100 MiB` | `reason=too_large` | `ConvertToPDF` 同步校验 |
| `OpenPDF` 扩展名不是 `.pdf` | `INVALID_ARGUMENT` | `只支持 .pdf 文件` | `reason=format` | `OpenPDF` |
| `OpenPDF` 内容不是 PDF（含空文件） | `INVALID_ARGUMENT` | `不是 PDF 文件` | `reason=format` | `OpenPDF` |
| PDF > 512 MiB（含 `ReadPDFChunk` 时文件已变大） | `INVALID_ARGUMENT` | `文件超过 512 MiB` | `reason=too_large` | `OpenPDF` / `ReadPDFChunk` |

**没有 reason 的 Doc 错误**（保持现有 code 和 `detail`，`detail` 首行不是 `reason=`）：取消（`CANCELED` "操作已取消"）；磁盘满（`CONVERT_DISK_FULL`）；读写失败 / 无权限（`IO_ERROR`）；文件 / PDF / 句柄不存在或已失效（`NOT_FOUND`）；路径非绝对、路径不合法、目录当文件、不是普通文件、`outputDir` 相关、`inputs` 为空或超过 50 个、`length` / `offset` 越界、`RemoveRecentPDFs` 超过 500 个（`INVALID_ARGUMENT`）；内部错误（`INTERNAL`）；加密 PDF 没有错误（能打开，密码由前端 pdf.js 处理），所以**没有 PDF 的 `reason=encrypted`**；LibreOffice 缺失 / 超时不适用（纯 Go 实现，不依赖外部程序）。

`FFMPEG_NOT_FOUND` / `PROBE_FAILED` / `PROCESS_FAILED` 不会由 DocService 返回。

### 6.12.7 示例（JSON 数值是示意；`data` 为节选）

`GetDocCapabilities` 返回：
```json
{ "formats": [
    { "ext": "docx", "supported": true,  "fidelity": "text-only", "reason": "" },
    { "ext": "xlsx", "supported": true,  "fidelity": "text-only", "reason": "" },
    { "ext": "pptx", "supported": true,  "fidelity": "text-only", "reason": "" },
    { "ext": "doc",  "supported": false, "fidelity": "", "reason": "旧版二进制格式，请先另存为 docx" },
    { "ext": "csv",  "supported": false, "fidelity": "", "reason": "暂不支持该格式" },
    { "ext": "txt",  "supported": false, "fidelity": "", "reason": "暂不支持该格式" } ],
  "font": { "available": true, "name": "noto-sans-sc-embedded", "cjk": true },
  "limits": { "maxInputsPerSubmit": 50, "maxInputBytes": 104857600, "maxPages": 5000, "maxPdfBytes": 536870912, "chunkBytes": 1048576, "wholeLoadBytes": 67108864 },
  "experimental": true }
```

`ConvertToPDF` 请求 / 返回（每个输入一个任务）：
```json
{ "inputs": ["C:\\Docs\\报告.docx"], "outputDir": "C:\\Users\\me\\Documents\\PDF" }
```
```json
[ { "id": "01J9Z8B1C2D3E4F5G6H7J8K9M0", "type": "office_pdf", "status": "queued", "title": "报告.docx → PDF",
    "inputPaths": ["C:\\Docs\\报告.docx"], "outputPath": "C:\\Users\\me\\Documents\\PDF\\报告.pdf",
    "progress": 0, "speed": "", "etaSec": 0, "params": "{\"input\":\"C:\\\\Docs\\\\报告.docx\",\"outputDir\":\"C:\\\\Users\\\\me\\\\Documents\\\\PDF\"}",
    "version": 1, "createdAt": 1790000000000, "startedAt": 0, "finishedAt": 0 } ]
```

`task:progress`（`office_pdf`，没有 `fps` / `bitrateKbps` / `droppedFrames`，`speed` 为空）：
```json
{ "id": "01J9Z8B1C2D3E4F5G6H7J8K9M0", "version": 4, "progress": 0.62, "speed": "", "etaSec": 0, "outTimeSec": 0 }
```

`OpenPDF` 返回 / `ReadPDFChunk` 请求与返回：
```json
{ "id": "9f3a1c5e7b2d4a6c8e0f1b3d5a7c9e1f", "path": "C:\\Docs\\a.pdf", "name": "a.pdf", "size": 2411724,
  "url": "/local/3c9d0e1f2a4b6c8d0e1f3a5b7c9d1e2f" }
```
```json
{ "id": "9f3a1c5e7b2d4a6c8e0f1b3d5a7c9e1f", "offset": 0, "length": 1048576 }
```
```json
{ "offset": 0, "length": 1048576, "eof": false, "size": 2411724, "data": "JVBERi0xLjcKJeLjz9MK…" }
```

AppError（转换不支持的格式；v0.16：`detail` 首行是 `reason=`，第二行是出错文件路径）：
```json
{ "code": "UNSUPPORTED", "message": "暂不支持这种格式", "detail": "reason=format\nC:\\Docs\\旧文档.doc\n.doc：旧版二进制格式，请先另存为 docx" }
```
AppError（句柄失效）：
```json
{ "code": "NOT_FOUND", "message": "PDF 句柄已失效，请重新打开" }
```

### 6.12.8 真机试用清单（Doc，未验证项汇总）

以下项目**没有在真机上验证**（箱子是 Linux），契约里已就地标"未验证"。**不阻塞实现**：实现按契约写，试用包出来后由用户逐项确认。

| # | 未验证项 | 在哪里 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|---|
| 1 | Windows 系统字体路径：`simhei.ttf` / `simsun.ttf` 是否存在、新版 Windows 的 `msyh.ttc` 确实加载不了；系统字体作补充路径时的表现 | 6.12.1「系统字体（补充）」 | Windows 10 / 11 上转一份含繁体字的 docx | 内嵌字体是主路径，系统字体只是补充，无系统字体也不影响简体 |
| 2 | 大文件（> 64 MiB）经 `/local/<token>` 的 Range 续传：WebView2 收到被截短的 `206` 后是否继续请求；`HEAD` 探测与失效重试 | 6.12.4 第 3 点、6.13 | 预览包里打开 100 MiB 左右的 PDF 并翻页 | **验证不通过时的回退方案（首版不实现）**：大文件限 64 MiB，超过返回 `INVALID_ARGUMENT`（不新增方法、错误码） |
| 3 | 64 MiB 整份读入阈值（峰值内存约文件大小 × 2）是估计值 | 6.12.4 第 5 点 | 真机打开 60 MiB 左右的 PDF，看内存和耗时 | 调整阈值（契约变更） |
| 4 | Windows 上 `outputDir` 拒绝 `\\?\` / `\\.\` 与数据目录内路径的判断；输出的 `.part` 原子改名（`os.Link` 失败回退到不带 `REPLACE_EXISTING` 的 `MoveFileEx`，同 6.11.3） | 6.12.3、6.12.6 | Windows 上把输出目录设到 U 盘（FAT/exFAT）、网络盘、数据目录内 | 保持 `os.Rename`，接受残余竞态并记录 |
| 5 | **联调项**：`ReadPDFChunk` 的 `data`（Go 字段 `string`，后端 base64 编码）在真实 Wails 运行时经前端 `atob` 解码后字节正确（**未验证**，没有在真实 Wails 环境跑过） | 6.12.4 第 2 点 | 在 Wails 开发模式下打开一份 PDF，核对拼出的字节以 `%PDF-` 开头即可 | 不符则回来改契约（例如 `models.ts` 的类型与预期不一致） |

## 6.13 本地资源访问 `/local/<token>`（中立章节，EditService 与 DocService 共用；由 #22 引入，#23 引用）

> **章节位置**：本节编号固定为 6.13，**排在 6.12（DocService，#23 引入）之后**、`## 7.` 之前；#22 单独看时 6.12 还不存在，所以这里紧跟在 6.11 后面，#23 合入时把 6.12 插在 6.11 和本节之间（编号不变，只是位置，所有交叉引用写的都是编号，不受影响）。

> 只有契约。合并顺序（架构师最新决定）：**#22 先合**，然后 #19、#23：本节和 6.11.3 的文件名净化函数由 #22 引入，#19 的直播存档（复用净化函数）和 #23（引用本节）都依赖它；直播存档的**实现**放在 #22 合入之后，#19 的文档层面先合也无妨。

**用途**：让 WebView 用 `<video>` / `<audio>` / pdf.js 读取用户本机的文件，而不暴露任意路径读取，也不监听任何端口。挂在 Wails AssetServer 的 `Handler`（`options.App.AssetServer.Handler`，只处理静态资源之外的请求）。

1. **登记表分表**：`edit` 表（`EditService.GetPreviewURL`）和 `doc` 表（`DocService.OpenPDF` 的大文件 URL）各自独立，**每表最多 512 项**，满了按最近使用淘汰最旧的（LRU，淘汰的 token 之后返回 404）；两张表互不挤占。同一路径在同一张表里复用同一个 token。
2. **token**：**`crypto/rand` 生成 16 字节，十六进制 32 字符**（不用 `math/rand`、不用时间 / 计数器）；URL 形如 `/local/<32 位十六进制>`，进程内有效，**应用重启后全部失效**。Handler 只按 token 查表，不接受任何路径参数或查询参数。
3. **登记时**：`filepath.EvalSymlinks` 得到真实路径，`os.Stat` 必须是**普通文件**（不是目录、设备、管道），记录真实路径和当时的 `os.FileInfo`。
4. **每次请求**：重新 `EvalSymlinks` 并与登记的真实路径比较，再 `os.Stat`，要求仍是普通文件且 `os.SameFile(登记时的 FileInfo, 现在的)` 为真；任何一项不满足（文件被删、被替换成链接 / 目录、被换成另一个文件）→ **404**。
5. **方法**：只允许 `GET` 和 **`HEAD`**（架构师定，必须支持），其他方法 `405` 并带 `Allow: GET, HEAD`。`HEAD` 与 `GET` 的状态码和头完全一致，只是没有正文；不带 `Range` 的 `HEAD` 对大于 32 MiB 的文件也返回 `200` 和完整 `Content-Length`（前端用它探测存在性和大小）。
6. **Range（原型已实测 12 种请求）**：
   - 只接受**单段** `bytes=`；多段（含 `,`）→ `416`。
   - 每个 `206` 响应**最多 4 MiB**：`bytes=a-b` 超长按 4 MiB 截断；开区间 `bytes=a-` 也按 4 MiB 截断；后缀 `bytes=-n` 先把 `n` 限制到文件大小，再按 4 MiB 截断（返回被请求区间的**开头** 4 MiB，`Content-Range` 如实反映）；截断后的长度小于请求长度是 HTTP 允许的，播放器会接着请求下一段。
   - 起点 ≥ 文件大小、起点大于终点、无法解析（`bytes=abc`）→ `416`，带 `Content-Range: bytes */<文件大小>`。
   - 不带 `Range` 的 `GET`：文件 ≤ 32 MiB 返回 `200` 整体，更大返回 `413`。
   - 用 `http.ServeContent` 输出，但**在调用前把请求头里的 `Range` 改写为校验后的单段区间，并删除 `If-None-Match` / `If-Modified-Since` / `If-Range`**，且不设 `Last-Modified` / `ETag`（WebView2 对 304 有已知问题，会让后续请求挂起——Wails 源码里对 304 有专门的降级为 500 的处理）。
   - 原型（Go `httptest`，20~40 MiB 稀疏文件）实测结果：`bytes=0-` → `206`，长 4 194 304，`Content-Range: bytes 0-4194303/…`；`bytes=0-99` → 100 字节；`bytes=-100` → 尾部 100 字节；`bytes=-10000000` → 4 194 304 字节；`bytes=99999999-` → `416`；`bytes=0-1,5-9` → `416`；`bytes=abc` → `416`；`POST` → `405`；`HEAD` + `bytes=0-9` → `206` 无正文；40 MiB 文件不带 Range 的 `GET` → `413`。
7. **响应头**：`Accept-Ranges: bytes`、`X-Content-Type-Options: nosniff`、`Cache-Control: no-store`、`Content-Type` 按扩展名：`mp4/m4v → video/mp4`，`mov → video/quicktime`，`mkv → video/x-matroska`，`webm → video/webm`，`avi → video/x-msvideo`，`flv → video/x-flv`，`mp3 → audio/mpeg`，`wav → audio/wav`，`aac → audio/aac`，`m4a → audio/mp4`，`flac → audio/flac`，`ogg → audio/ogg`，**`pdf → application/pdf`**；表外的扩展名不会到达这里（登记时已拒绝）。
8. **token 失效与前端重试（架构师定）**：token 不存在、被淘汰、应用重启、文件变化、被 `RemoveRecent*` 撤销 → 一律 **404**（不区分原因）。前端在 `<video>` / `<audio>` 触发 `error`、或每次用旧 URL 之前，先对该 URL 发一个 **`HEAD`** 请求探测：`200`/`206` 才继续；`404` → 重新调用 `GetPreviewURL(path)`（或 `OpenPDF(path)`）换新 URL，**只重试一次**，仍失败则按"文件不存在或已被移动"提示。
9. **Windows 限制**：Wails v2.11.0 `pkg/assetserver/webview/responsewriter_windows.go` 把响应体缓冲在内存里，`Finish` 才一次性交给 WebView2（官方 Options 文档："Response Body Streaming：Windows ❌，macOS ✅，Linux ✅"），所以上面的 4 MiB / 32 MiB 限长在所有平台一律生效。WebView2 收到被截短的 `206` 之后是否会继续请求下一段 **未在 Windows 真机验证**，由用户在预览包里验证（社区有只发第一段的反馈，wailsapp/wails#5047 无结论）。

## 7. 本地流服务（已取消）

> **v0.5 起取消，v0.10 删除原文。** 没有本地 FLV / WebSocket 流服务，后端不监听任何端口（也就没有 `/ws/record`、`/flv/<id>`、token、`wsURL`）：推流由后端 ffmpeg 直接推到用户填写的地址，播放由前端播放器直接拉取远端地址，屏幕由后端 ffmpeg 直接采集。应用里唯一保留的"HTTP"是 Wails AssetServer 的 `/local/<token>`（本地文件预览，见第 1 节）。直播的设计见第 4 节 LiveService 和 6.10。

## 8. 迁移步骤

1. 删除 OpenClaw：`openclaw_controller.go`、`vo/OpenClawInfo.go`、`OpenClawInstall.vue`、`api/openclaw/`、路由和菜单项。
2. 建 `internal/ffmpeg`、`internal/store`、`internal/task`，把 controller 里的业务逻辑抽到 `internal/service`；gin handler 暂时改为调用 service。
3. 按页面逐个切换到 Bind：JSON 工具（最简单，先验证链路）、Office/PDF、格式转换、剪辑、直播。
4. 全部切完后删除 gin、SSE、`/api` 路由、`public/` 目录逻辑。
5. 三端打包：Windows 用 NSIS，macOS 出 .app/.dmg，Linux 出 AppImage；安装包不再内置 ffmpeg，由第 9 节的检测与安装流程处理。

## 9. ffmpeg 环境检测与自动安装

### 9.1 检测顺序（启动时后台执行，不阻塞界面）

1. 设置里用户手动指定的路径
2. 应用自带目录：`<数据目录>/bin/ffmpeg(.exe)`、`ffprobe(.exe)`
3. 系统 PATH（`exec.LookPath`）
4. 兼容 v1：程序同级的 `ffmpeg/` 目录

每个候选执行 `ffmpeg -version` 和 `ffprobe -version` 校验：能运行、主版本不低于 6、`-encoders` 里包含 libx264 和 aac。第一个通过的即为当前 ffmpeg。

实现细节（`internal/ffmpeg`）：
- 手动指定的路径可以是目录（也会看它下面的 `bin/`）或 ffmpeg 可执行文件本身；指定的路径失效时不报错，继续往后找，原因记在 `Error.detail`。
- 版本过低但能运行的候选，只在没有任何合格候选时才作为 `outdated` 返回；`outdated` 不再检查编码器。
- 无法解析主版本号的构建（`N-12345-gabc` 这类 git 主干构建、`2024-05-20-git-...` 日期版）视为版本可接受，因为通常比最新发行版还新，但编码器检查照常执行。
- v1 的 `ffmpeg/` 目录只带 ffmpeg 没有 ffprobe，ffprobe 缺失时退回 PATH 里的（同样校验）。
- 每个检测命令 10 秒超时，Windows 下隐藏控制台窗口。

### 9.2 安装位置

装到应用自己的 `<数据目录>/bin/`，**不修改系统 PATH**。原因：改 PATH 在 Windows 要管理员权限、在 macOS 和 Linux 要改 shell 配置，还可能和用户已有的 ffmpeg 冲突，卸载也清不干净。应用内部统一用绝对路径调用，效果和装进环境一样。

### 9.3 下载流程

- 下载源做成按平台和架构（windows-amd64、darwin-arm64、darwin-amd64、linux-amd64、linux-arm64）的清单，清单里写 URL、SHA256、版本号；清单本身放在项目仓库的 release 里，可以随时换源，并支持配置国内镜像。
- 作为 `ffmpeg_install` 任务进 batch 池，复用任务进度事件；支持断点续传（HTTP Range），下载完先校验 SHA256，再解压到临时目录，二次运行 `-version` 验证通过后原子改名到 `bin/`。
- macOS 和 Linux 解压后 `chmod 0755`；macOS 上若 `-version` 运行失败，先执行 `codesign -s - <文件>` 做 ad-hoc 签名再验证一次。
- `InstallFFmpeg` 幂等：已有进行中的安装任务时直接返回该任务，不开第二个下载。
- 失败时保留已下载部分，提示"重试"或"手动选择 ffmpeg 所在位置"（离线用户的出路）。

**v0.6 实现说明（`internal/ffmpeg`：manifest / download / extract / install）：**
- 清单是内置的 `internal/ffmpeg/manifest.json`（`go:embed`），每个平台一项，含 URL、SHA256、大小、压缩包类型（zip / tar.xz）、要提取的文件；只用带版本号的固定地址，不用 `latest` 滚动地址，SHA256 都经实际下载核对。某平台没有可验证的固定源时标记 `available:false`，`InstallFFmpeg` 返回 `UNSUPPORTED_PLATFORM`。
- `mirror` 参数只接受 `""`（默认源）和 `"cn"`。清单里某个压缩包有 `mirrors.cn` 时先用镜像、失败再退回原地址；没有 `cn` 条目的平台（目前是 macOS / Linux 的 martin-riedl.de）**不会悄悄退回默认源**：`InstallFFmpeg("cn")` 返回 `INVALID_ARGUMENT`，detail 说明该平台没有镜像、可用默认源（v0.6.1）；一个平台的所有压缩包都有该镜像才算"可用"。前端用 `GetInstallOptions().mirrors` 决定是否显示镜像开关。不编造镜像地址。镜像必须与原地址返回完全相同的文件（SHA256 相同）。
- 下载写到 `<数据目录>/tmp/ffmpeg-<版本>-<sha前缀>.part`，断线自动重试并用 `Range` 续传；失败或取消保留 `.part`，SHA256 不符则删除（内容已坏）。校验通过才解压，只提取 ffmpeg / ffprobe 到暂存目录，用与检测相同的规则校验，通过后改名进 `bin/`（新旧文件整体替换，中途失败回滚）。成功后才删除 `.part`。
- 安装期间状态为 `installing`（`taskId` 为安装任务 ID），依赖 ffmpeg 的门控保持关闭；期间 `RecheckFFmpeg` 保持 `installing`，`SetFFmpegPath` 返回 `TASK_CONFLICT`。结束后 `ready`；失败为 `failed`（`error` 有值，`taskId` 保留）；取消后重新检测。
- 进度不塞进 `ffmpeg:status`：安装是任务管理器里的 `ffmpeg_install` 任务（v0.7 起），由任务管理器发第 5 节的 `task:created` / `task:progress`（每秒最多 4 次）/ `task:status`，payload 与契约一致（`Task.type = ffmpeg_install`）。`InstallFFmpeg` 返回的就是任务管理器里的 `Task`（v0.7 起；v0.6 的轻量 `InstallTask` 已取消，JSON 形状不变）。`progress` 0~1 覆盖整个流程：下载占 0~0.9，解压 0.9~0.94，校验 0.94~0.98，安装完成 1。
- 下载可用镜像 / 平台清单在契约外，随版本更新清单文件即可；macOS 上校验失败会先 `codesign -s -` 再校验一次。

### 9.4 接口

```go
type FFmpegStatus struct {
    State     string `json:"state"`     // checking | ready | missing | outdated | installing | failed
    Path      string `json:"path"`
    Version   string `json:"version"`
    Source    string `json:"source"`    // custom | bundled | system | legacy
    TaskID    string `json:"taskId,omitempty"` // installing 时对应的安装任务；无值时不输出，TS 中为 taskId?: string
    FFprobeMissing bool `json:"ffprobeMissing"`   // ready 但没有 ffprobe（v1 的 ffmpeg/ 目录），前端提示补全；探测 / 缩略图用 ffmpeg.RequireProbe() 门控
    Error     *AppError `json:"error,omitempty"`  // 无值时不输出，TS 中为 error?: AppError
}

// SystemService
GetFFmpegStatus() (FFmpegStatus, error)
InstallFFmpeg(mirror string) (Task, error)   // 提交 ffmpeg_install 任务（batch 池）；mirror 只接受 "" 和 GetInstallOptions().mirrors 里的名字，其他值返回 INVALID_ARGUMENT（detail 列出可选镜像）
GetInstallOptions() (InstallOptions, error)  // {platform, supported, mirrors[]}：当前平台是否有下载源、可用镜像（不含默认源），没有时 mirrors 是 []
CancelFFmpegInstall() error                  // 取消进行中的安装，保留已下载部分；没有安装在进行时无操作
SetFFmpegPath(dir string) (FFmpegStatus, error) // 手动指定，校验失败返回 INVALID_ARGUMENT；传空串清除手动指定并重新检测
RecheckFFmpeg() (FFmpegStatus, error)
```

`missing` / `outdated` 时 `error` 为 `FFMPEG_NOT_FOUND`，`detail` 逐行列出各候选失败原因；`ready` 时为 null。`GetSettings` / `UpdateSettings`（第 4 节）中 `ffmpegPath`、`ffmpegPromptDismissed` 两项已实现，`UpdateSettings` 改 `ffmpegPath` 时同样校验，失败返回 `INVALID_ARGUMENT` 且整体不生效；其余字段随后续 PR 补充。

### 9.5 功能门控

- 依赖 ffmpeg 的：转换、剪辑、直播、媒体探测和缩略图。`state != ready` 时这些入口可以进，但操作按钮禁用，顶部显示提示条和"一键安装"按钮；后端对应 Service 统一返回 `FFMPEG_NOT_FOUND`，双保险：入口处调用 `ffmpeg.Require()`，未就绪返回该错误，就绪则返回 ffmpeg / ffprobe 的绝对路径，子进程一律用这个路径启动。
- 不依赖 ffmpeg 的：Office 转 PDF、PDF 预览、JSON 工具，始终可用（DocService 任何方法都不返回 `FFMPEG_NOT_FOUND`）。
- 首次启动检测到 `missing` 时弹一次确认框（"安装"或"稍后"），选"稍后"后写入 `Settings.ffmpegPromptDismissed = true`，之后只保留提示条，不再弹窗；ffmpeg 变为 ready 后该标记重置。
- 前端不轮询：检测完成、安装进度导致的 state 变化、手动指定路径、重新检测，都会推送 `ffmpeg:status`，payload 为完整 `FFmpegStatus`。

### 9.6 硬件编码器检测与偏好（v0.15，契约按架构师口头方案起草，如有出入以架构师为准）

```go
type EncoderNames struct {
    H264 string `json:"h264"` // 该设备上的 h264 编码器名，如 "h264_nvenc"；不支持为 ""
    HEVC string `json:"hevc"` // 如 "hevc_nvenc"；不支持为 ""
}
type EncoderDevice struct {
    ID        string       `json:"id"`        // "cpu"，或 "<vendor>-<序号>"，如 nvidia-0、intel-0、amd-0、apple-0；偏好里存它
    Name      string       `json:"name"`      // 给人看的名字，CPU 是 "CPU（软件编码）"
    Vendor    string       `json:"vendor"`    // nvidia | intel | amd | apple | unknown
    Kind      string       `json:"kind"`      // gpu | cpu
    Discrete  bool         `json:"discrete"`  // 独立显卡（auto 时独显优先于集显）；CPU 恒为 false
    Encoders  EncoderNames `json:"encoders"`
    Available bool         `json:"available"` // 试跑成功才为 true
    Reason    string       `json:"reason,omitempty"` // available=false 时的一行原因（可能偏技术，界面不必直接显示）
}
type EncoderDeviceList struct {
    FFmpegReady bool            `json:"ffmpegReady"` // false = ffmpeg 未就绪，没有做检测，devices 只有 cpu
    Devices     []EncoderDevice `json:"devices"`     // 第一项永远是 cpu（id "cpu"，encoders 为 libx264 / libx265，available=true）
}
type EncoderPreferenceInfo struct {
    ID        string `json:"id"`                // auto | cpu | 设备 id
    Name      string `json:"name"`              // "自动" | "CPU（软件编码）" | 设备名；设备不可用 / 不存在时用保存偏好时记下的名字（没记过为 ""）
    Available bool   `json:"available"`         // auto、cpu 恒为 true
    Reason    string `json:"reason,omitempty"`
}
```

**检测流程**（`ListEncoderDevices`）：
1. ffmpeg 状态不是 `ready`：直接返回 `{ffmpegReady:false, devices:[cpu]}`，不检测、不报错。
2. `ffmpeg -hide_banner -encoders`，解析出视频编码器集合。这一步失败（命令失败且没有输出）：返回仅 cpu，**不缓存**、不报错。
3. 枚举显卡名称（失败一律降级为空列表，不报错）：Windows 用 PowerShell `Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID | ConvertTo-Json`（wmic 已废弃）；macOS 用 `system_profiler SPDisplaysDataType -json`；Linux 先 `lspci -nn`，没有或没输出时读 `/sys/class/drm/card*/device/vendor`（只有厂商名）。虚拟适配器（Microsoft Basic Display / Remote Display、Hyper-V、VMware、VirtualBox、QXL 等）忽略。独显判定：NVIDIA 恒为独显；AMD 的 `Radeon Graphics` / `Vega N` / `xxxM` 是集显，其余（RX、Pro）是独显；Intel 只有 Arc 是独显。
4. 试跑：平台上每个厂商的编码器（nvidia：`h264_nvenc` / `hevc_nvenc`；intel：`h264_qsv` / `hevc_qsv`；amd：`h264_amf` / `hevc_amf`；macOS：`h264_videotoolbox` / `hevc_videotoolbox`；Linux vaapi 本版不做），**只试 ffmpeg 里存在的**；显卡枚举到了就只试有对应显卡的厂商，枚举不出来就全试（试跑才是真相）。命令：`ffmpeg -hide_banner -loglevel error -nostdin -f lavfi -i color=c=black:s=256x256:d=0.1 -frames:v 1 -c:v <enc> -f null -`，**每个编码器 5 秒超时**，同时最多 2 个试跑。某厂商任一编码器试跑成功即 `available=true`，`encoders` 只填成功的那个（h264 成功、hevc 失败则 `hevc` 为 `""`）；都失败则 `available=false`、`reason` 是归类后的一行原因（ffmpeg 不含该编码器 / 无可用显卡或驱动缺失 / 试跑超时 / 其他）。
5. 组装：`devices[0]` 是 cpu；随后按厂商顺序（nvidia、amd、intel；macOS 只有 apple）每张显卡一项，同厂商多张 id 序号递增。试跑成功但没枚举到名字（lspci 缺失等）给一个只有厂商名的设备（如 `NVIDIA GPU`）。枚举到但厂商没有硬件编码器支持的显卡（如 unknown）也列出，`available=false`。
6. 探测子进程一律经 `ffmpeg.NewCommand`（Windows 隐藏控制台窗口、单独进程组），不占用任务管理器的槽位，不影响正在运行的任务。应用根 ctx 取消时中断并返回 `CANCELED`。

**缓存**：按 `ffmpeg 路径 + 版本` 缓存整个结果。ffmpeg 状态每次变化（安装完成 / 手动指定 / 重新检测，即每次 `ffmpeg:status`）都使缓存失效；检测过程中发生失效，这次结果不写入缓存。`RefreshEncoderDevices()` 强制重测。有编码器试跑超时的结果**不缓存**（驱动可能只是一时没响应）。没有显卡的机器：`devices` 只有 cpu，不报错，也不试跑。

**偏好**：`"auto" | "cpu" | 设备 id`，存 settings 表键 `encoderPreference`，默认 `"auto"`；同时把设备名记在 `encoderPreferenceName`（设备之后不可用时，设置页仍能显示选的是哪张卡）。`SetEncoderPreference(id)`：`auto`、`cpu` 直接保存；其他值必须符合 `^[a-z0-9][a-z0-9_-]{0,31}$` 且在当前 `ListEncoderDevices` 里存在（存在但 `available=false` 的允许保存），否则 `INVALID_ARGUMENT` 且不改动原值。`GetEncoderPreference` 永远返回保存的原值，不因设备消失而改写；设备不存在或不可用时，`ListEncoderDevices` 在列表**末尾**追加一项 `available=false` 的占位（`id` 为偏好值，`name` 为记下的名字，`reason` 说明），偏好为 `auto` / `cpu` 时不追加。

**`ResolveEncoder(pref, devices, codec) (encoderName, deviceID string, fallback bool)`**（Go 纯函数，`internal/service/system`）：`codec` 为 `h264` 或 `hevc`（接受 `h265`）。
- `auto`（或空）：从 `available` 且有该 codec 编码器的显卡里选第一张，排序为 **独显优先于集显**，同为独显时 nvidia、amd 在前，其后 intel Arc，同级保持列表顺序；没有则 cpu。auto 落到 cpu **不算回退**（`fallback=false`）。
- `cpu`：cpu，`fallback=false`。
- 设备 id：该设备存在、`available` 且有该 codec 编码器则用它；否则回退 cpu，`fallback=true`。
- 不认识的 `codec`：返回 `("", "", false)`。

**~~本版不接入~~（v0.18 起作废，已在 9.7 接入）**：v0.15 时 `ConvertService` / `EditService` / `LiveService` 的编码参数仍是软件编码；`ResolveEncoder` 只是提供给下一版接入用。**未在真机验证**：真实 NVIDIA / Intel / AMD / VideoToolbox 试跑、Windows 显卡名称枚举（PowerShell 输出格式按文档与常见样例解析，用纯函数表驱动测试覆盖）。

### 9.7 硬件编码接入（v0.18，契约按架构师口头方案起草，如有出入以架构师为准）

**没有新增接口方法、没有新增错误码。** 只是让转换、剪辑导出、直播真正使用 9.6 的 `ResolveEncoder` 结果。

**何时解析**：任务**提交时**（Retry 重新提交时同样）用 `ListEncoderDevices` 的缓存结果（没有缓存时先检测一次）+ 当前偏好 + `ResolveEncoder` 解析一次，结果写进 `Task.encoder` / `encoderDevice`（所以 `task:created`、落库、第一条 `task:status` 就带着）。直播在 `StartFilePush` / `StartScreenPush` 时解析。解析器由 `system.Manager.EncoderResolver()` 提供，注入各服务的 `Config.Encoder`（nil = 一律 CPU）。

**各功能用哪个编码器**

| 功能 | 什么时候可以用硬件 | 其余一律 CPU（`hwFallback` 为 false，不算回退） |
|---|---|---|
| 格式转换 `convert` | 真正重编码 H.264（`videoCodec=h264` → `h264_*`）或 H.265（`h265` → `hevc_*`），容器 mp4 / mov / mkv / avi / flv | `-c copy`（`encoder="copy"`，无设备）；VP9（`libvpx-vp9`）；GIF（`gif`）；纯音频转换和无视频输出（不带 `encoder`）；`targetSizeMb > 0` 的两遍编码（目前 `ValidateConvertOptions` 仍拒绝）；H.264 且输出宽或高 > 4096（NVENC / AMF / QSV 的 H.264 上限） |
| 剪辑导出 `edit_export` | mp4 / mov / mkv 导出的 H.264（`h264_*`） | webm（VP9，恒 CPU）；输出宽或高 > 4096 |
| 直播 `live_file_push` / `live_screen_push`（含带存档的 tee） | 视频恒为 H.264 重编码，用 `h264_*` | 输出宽或高 > 4096 |

音频编码、滤镜（`scale` / `pad` / `fps` / `crop` / 剪辑 filtergraph）、封装参数不变，滤镜仍在 CPU 上跑，只把编码交给显卡。像素格式：nvenc / amf / videotoolbox 用 `yuv420p`，QSV 用 `nv12`；转换 / 剪辑本来就把输出降到 8bit 4:2:0，所以 10bit 输入不存在“编码器不兼容”的情况。

**参数映射**（集中在 `internal/ffmpeg/hwenc.go` 的纯函数，表驱动测试；CPU 参数与原来逐字一致）。`eq` = “x264 等价 CRF”：H.264 直接用 CRF（转换默认 23，用户设了 `crf` 就用它，剪辑导出 20），H.265 用 `CRF - 5`（默认 28 → 23，因为 x265 的 CRF 比 x264 约高 5 才是相近画质），限制在 1~51。设置了 `videoBitrate` 时按码率而不是质量。

| 编码器 | 质量（CRF 模式） | 码率模式（`videoBitrate` > 0） |
|---|---|---|
| libx264 / libx265（CPU，现状） | `-preset medium -crf N` | `-b:v` |
| `*_nvenc` | `-preset p4 -rc vbr -cq <eq> -b:v 0` | `-preset p4 -rc vbr -b:v <b>` |
| `*_qsv` | `-preset medium -global_quality <eq>` | `-preset medium -b:v <b>` |
| `*_amf` | `-quality balanced -rc cqp -qp_i <eq> -qp_p <eq>` | `-quality balanced -rc vbr_peak -b:v <b>` |
| `*_videotoolbox` | `-q:v <108 - 2×eq>`（1~100；18→72、23→62、28→52） | `-b:v <b>` |

H.265 在 mp4 / mov 里照旧加 `-tag:v hvc1`。**直播**（H.264，码率控制）：`-b:v <k>k -maxrate <k>k -bufsize <2k>k -g <2×帧率> -bf 0` 在每个硬件编码器上都给，另加各家的低延迟项：nvenc `-preset p4 -tune ll -rc cbr`；qsv `-preset veryfast -async_depth 1`；amf `-usage lowlatency -rc cbr`；videotoolbox `-realtime 1`；CPU 仍是 `-preset veryfast -tune zerolatency`（不加 `-bf 0`，与现状一致）。这些映射是经验值，**没有在真机上校准画质**。

**回退规则（硬件编码启动失败 → 自动用 CPU 重试一次，不算任务失败）**
- 触发条件（任一）：ffmpeg 非零退出，且 stderr 命中所选厂商的硬件初始化失败特征（NVENC：`No NVENC capable devices`、`Cannot load libcuda`、`Driver does not support the required nvenc API`、`OpenEncodeSessionEx failed` 等；QSV：`Error initializing an MFX session` 等；AMF：`DLL amfrt64.dll failed to open`、`AMF failed` 等；VideoToolbox：`VTCompressionSessionCreate` 等）或通用特征（`Unknown encoder`、`Error while opening encoder`）；或**起始阶段崩溃**：还没有任何进度、输出临时文件不存在或为空，且 stderr 有一行提到该硬件编码器名和错误字样。与硬件无关的失败（输入损坏、磁盘满、推流连接被拒等）**不**触发回退，按原样失败。
- 回退动作：日志里写一行说明，`Task.encoder` / `encoderDevice` 改成 CPU 编码器（`libx264` / `libx265`，设备 `cpu`），`hwFallback=true`，`hwFallbackReason` 设为下表枚举，落库，并**补发一条 `task:status`（status 仍为 `running`）**带这四个字段；然后用 CPU 参数重新运行同一个 ffmpeg 命令（进度从头开始，进度条只增不减）。**最多重试一次**；重试也失败则任务失败，错误取 CPU 那一次的错误。
- **取消不触发回退**：ctx 已取消时一律按取消处理，不启动重试。
- **直播的回退窗口**：只在推流尚未建立（还没有第一条 `task:progress`，即 ReportGate 之前）时失败才回退；推流已建立后中途失败（包括硬件编码器中途报错）**不自动重试**，任务按原有规则失败。
- **提交时**所选设备不可用（`ResolveEncoder` 返回 `fallback=true`）：直接用 CPU，`hwFallback=true`，`hwFallbackReason="device_unavailable"`，不算错误。auto 落到 CPU、偏好 `cpu`、以及上表“一律 CPU”的场景都**不**算回退。

`hwFallbackReason` 取值（固定枚举，一行，不含路径；前端自行翻译文案。v0.19：Go 常量 `internal/ffmpeg/hwenc.go`、本行、前端 `taskTypes.ts` 三处枚举一致，`TestHWFallbackReasonEnumConsistent` 锁定；前端 `errors/encoderMessages.ts` 的回退文案按功能区分，不按原因区分）：`device_unavailable`、`nvenc_init_failed`、`qsv_init_failed`、`amf_init_failed`、`videotoolbox_failed`、`encoder_unavailable`（ffmpeg 里没有该编码器）、`encoder_start_failed`（其他打开编码器失败 / 起始崩溃）。

**事件与落库**：`Task.encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 与 `task:progress`、`task:status` 里的同名字段一致（`omitempty`）：`task:progress` 每条都带（前端可只在变化时取用）；`task:status` 的 `running` 事件、回退时补发的 `running` 事件、所有终态事件都带；`Get` / `List` 从库里读到的任务也有（迁移 `0004_task_encoder.sql`，旧行为空）。Retry 生成的新任务重新解析编码器。`Task.params` 不变（不含编码器信息）。

**已在沙箱验证**（ffmpeg 7.1.5，有 `h264_nvenc` 编码器但没有 GPU，libcuda 缺失）：转换 / 剪辑导出 / 文件推流到 MediaMTX 的 CPU 路径端到端；选 NVIDIA 时真实 NVENC 初始化失败（`Cannot load libcuda.so.1` / `Error while opening encoder`）→ 自动 CPU 重试成功，输出 h264，任务带 `hwFallback`；假 ffmpeg 夹具覆盖：初始化失败回退成功、重试也失败、与硬件无关的失败不回退、取消不回退、直播推流前失败回退、推流中途失败不回退。
**未在真机验证**：真实 NVENC / QSV / AMF / VideoToolbox 的画质与码率（CRF → 质量映射是经验值）；各厂商 stderr 失败特征的覆盖度（尤其 QSV、AMF、VideoToolbox 的真实报错文案）；`-rc cqp` / `vbr_peak` / `-async_depth` / `-realtime` 等参数在各驱动版本上的可用性；4096 的尺寸上限只是保守取值（新显卡的 H.264 可能支持更大）；直播在硬件编码器上的延迟与 `-bf 0` 效果。
