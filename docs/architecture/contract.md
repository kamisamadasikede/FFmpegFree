# FFmpegFree v2 接口契约（v0.10）

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
- 前端预览本地文件：通过 Wails AssetServer 的 `Handler` 挂 `/local/<token>`，由后端按 `media_id` 映射真实路径，不暴露任意路径读取。
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
| TASK_CONFLICT | 任务状态不允许该操作（如取消已完成任务）；直播（v0.10）：同一推流地址已有进行中的会话，或进行中的直播会话已达 4 个上限 |
| IO_ERROR | 读写文件失败 |
| PROBE_FAILED | 文件存在但 ffprobe 无法解析（损坏、不是音视频文件、没有可识别的流） |
| CANCELED | 调用因应用退出（根 ctx 取消）而被取消，结果作废；前端不需要提示用户（`ConvertService.Submit`、`LiveService.Start*` 等） |
| UNSUPPORTED | 该操作不支持这个对象（如没有重试工厂的任务类型不能 Retry；直播会话 Retry 也是它） |
| CONVERT_DISK_FULL | 转换写输出文件时磁盘空间不足（前端标题「磁盘空间不足」，可引导用户换输出目录） |
| PROCESS_FAILED | 子进程非零退出，detail 带最后 50 行日志 |
| UNSUPPORTED_PLATFORM | 当前系统或会话不支持该功能（如 Linux Wayland 下的屏幕采集、Linux 没有 `DISPLAY`、x11grab 打不开显示） |
| LIVE_URL_INVALID | 推流地址格式不合法或协议不支持（只允许 rtmp / rtmps / srt，规则见第 4 节 LiveService） |
| LIVE_CONNECT_FAILED | 推流**开始前**连接目标失败（DNS、拒绝连接、超时、网络不可达）：任务在收到第一条 `task:progress` 之前就失败 |
| LIVE_PUSH_REJECTED | 目标服务器明确拒绝推流（鉴权失败、流名冲突、握手被拒等），推流开始前 |
| LIVE_PUSH_INTERRUPTED | 推流**已经开始**（收到过 `task:progress`）后被目标服务器或网络中断 |
| SCREEN_PERMISSION_DENIED | 没有屏幕录制权限（macOS 系统授权），`StartScreenPush` 同步返回或任务失败 |
| INTERNAL | 其他；直播任务里认不出的 ffmpeg 非零退出也是它（不是 `PROCESS_FAILED`），detail 带（已脱敏的）stderr 最后若干行 |

**直播 / 录屏（v0.10）用到的后端码正好是冻结的这八个：`LIVE_URL_INVALID`、`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`SCREEN_PERMISSION_DENIED`、`FFMPEG_NOT_FOUND`、`UNSUPPORTED_PLATFORM`、`INTERNAL`**；此外复用已有的 `INVALID_ARGUMENT`、`NOT_FOUND`、`PROBE_FAILED`（输入文件问题）、`TASK_CONFLICT`（会话上限 / 同地址冲突）、`UNSUPPORTED`（Retry）、`CANCELED`（`Start*` 因应用退出被取消，#10 已加）。**v0.10 没有新增任何错误码**，也没有 `LIVE_START_FAILED` 之类的同义码。用户主动停止不产生错误码（优雅停止成功 = `succeeded`，超时强杀 = `canceled` 状态，`error` 为空）。`LIVE_PLAY_FAILED`（播放器加载或解码失败）和 `LIVE_CORS_BLOCKED`（拉流地址跨域被浏览器拦截）**只在前端由播放器产生**，后端不会返回，也不在 `apperr` 里定义。

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

type TaskType string // convert | edit_render | office_pdf | live_file_push | live_screen_push | ffmpeg_install
                     // （v0.10：live_relay、live_record_push 保留但不再产生：不能提交，任务中心不展示，库里的旧记录按未知类型忽略、不报错，见 6.10）
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
    BitrateKbps   float64 `json:"bitrateKbps,omitempty"`   // 近 5 秒的输出码率（kbit/s）
    DroppedFrames int64   `json:"droppedFrames,omitempty"` // ffmpeg 丢弃的帧数（累计），不是网络丢包
    Params     string     `json:"params"`     // 原始参数 JSON，用于重试（直播任务的 params 已脱敏，不能用来重试，见 6.10）
    Version    int64      `json:"version"`    // 每次变更 +1，前端据此丢弃旧事件
    Error      *AppError  `json:"error"`
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

### EditService（保留现有多轨时间线能力）
```go
Render(project EditProject) (Task, error)    // EditProject 沿用现有 VideoClip/AudioClip/GlobalEffects 结构，
                                             // 把 fileName+scope 换成绝对 path
SaveProject(project EditProject) (string, error)
LoadProject(id string) (EditProject, error)
ListProjects() ([]EditProjectMeta, error)
```

### DocService（Office 转 PDF + PDF 预览）
```go
ConvertToPDF(inputs []string, outputDir string) ([]Task, error) // docx/xlsx/pptx，纯 Go 实现，不依赖 LibreOffice
GetPDFURL(path string) (string, error)                            // 返回 /local/<token> 供预览
```

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
StartScreenPush(req ScreenPushRequest) (Task, error)   // 屏幕推流（可同时本地存档）
GetCaptureCapabilities() (CaptureCapabilities, error)  // 屏幕采集能不能用、为什么不能用
ListScreens() ([]ScreenInfo, error)                    // 可采集的显示器
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
    Loop       bool        `json:"loop"`       // true = 循环播放直到用户停止；false = 播完自然结束（任务 succeeded）
    Options    PushOptions `json:"options"`
}

type ScreenPushRequest struct {
    URL        string      `json:"url"`
    ScreenID   string      `json:"screenId"`   // ListScreens 返回的 id；"" = 主显示器；不存在 INVALID_ARGUMENT
    HideCursor bool        `json:"hideCursor"` // 零值 = 画面里带鼠标指针
    Audio      string      `json:"audio"`      // "none"（默认，视频流里没有音轨）| "silent"（补一路静音音轨，给要求必须有音频的服务器）；采集声音 v1 不做
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

type ScreenInfo struct {
    ID      string  `json:"id"`      // 不透明字符串，前端只原样传回：windows "monitor:<序号>"、darwin "avf:<设备序号>"、linux "x11:<输出名>" 或 "x11:desktop"
    Name    string  `json:"name"`    // 如 "显示器 1（主）"
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

**`Start*` 同步返回的错误**（此时没有创建任务）：`FFMPEG_NOT_FOUND`；`INVALID_ARGUMENT`（选项越界、输入文件没有视频、`archiveDir` 不是绝对路径、`screenId` 不存在）；`NOT_FOUND` / `PROBE_FAILED`（输入文件不存在 / 无法解析）；`LIVE_URL_INVALID`；`UNSUPPORTED_PLATFORM`（不能采集屏幕）；`SCREEN_PERMISSION_DENIED`（已知没有权限时）；`TASK_CONFLICT`（同一个推流地址已经有进行中的会话；或进行中的直播会话已达 4 个上限）。

**停止 = `TaskService.Cancel(taskID)`，不设 `StopPush`**。理由：
1. 状态机、落库、`task:status`、应用退出（`Shutdown`）走的就是同一条取消路径，直播任务的 Runner 本来就是"取消 → 先发 `q`，最多等 5 秒让 ffmpeg 收尾，超时再强杀"（6.5、6.6）；再包一层 `StopPush` 只会多一个和 `Cancel` 语义重复、还要保持同步的入口。
2. 任务中心、通知条等所有能看到任务的地方本来就有"停止"按钮，直播会话不用特殊处理。
3. 结果语义（前端文案要区分）：优雅停止成功 → Runner 返回 nil → 任务是 **`succeeded`**（不是 `canceled`；用户点"停止直播"是直播的正常结束，存档完整）；5 秒内没退出被强杀 → `canceled`，存档不保证可用。`Cancel` 本身立即返回，不等 ffmpeg 退出；前端以 `task:status` 为准。**硬性规则（架构师定）：优雅停止记 `succeeded` 时任务的 `error` 必须为空；强杀记 `canceled` 时同样不带错误码（`error` 为空）；前端只看 `status` 区分"已结束推流"（`succeeded`）和"已强制停止"（`canceled`），不看 `error`。**测试必须断言这两种终态的 `Task.error == nil`，且 `task:status` 载荷不带 `error`。已结束的会话 `Cancel` 返回 `TASK_CONFLICT`，重复点击（正在停止中）返回 nil。
4. `Retry` 对直播任务返回 `UNSUPPORTED`（没有注册重试工厂，且 params 已脱敏、拿不到密钥）；前端"重新开始"就是用表单里的值再调一次 `Start*`。

**推流地址校验规则**（`Start*` 和 `CheckPushURL` 共用，不通过一律 `LIVE_URL_INVALID`，`message` 说明原因，`detail` 只带脱敏后的地址，绝不回显原文）：
1. 先 `TrimSpace`；长度 ≤ 2048 字节；不能含空白、控制字符、`|`、`\`、`"`、`'`（`|` 是 ffmpeg tee 分隔符，其余会破坏命令行 / 日志）。
2. scheme 不区分大小写，只允许 `rtmp`、`rtmps`、`srt`；其余（`file`、`http(s)`、`rtsp`、`udp`、`tcp`、`pipe`、`concat`、`subfile`、`data` ……）一律拒绝。传给 ffmpeg 的永远是校验后的 URL 并带 `-protocol_whitelist`，不会因为用户输入变成读本地文件或打开别的协议。
3. host 不能为空；端口写了必须在 1~65535；IPv6 用方括号；IDN 主机名转 punycode，转不了就拒绝。**允许**回环 / 内网地址（推到本机或局域网的 nginx-rtmp、SRS、MediaMTX 是正常用法）。
4. `rtmp` / `rtmps`：路径至少要有应用名（`rtmp://host/` 不合法）；流名可以在路径里，也可以在查询参数里。
5. `srt`：必须写端口；只允许 caller 模式——查询参数里 `mode=listener` / `mode=rendezvous` 拒绝（listener 会在本机开监听端口，与"后端不监听任何端口"矛盾）；`passphrase`、`streamid` 等参数原样交给 ffmpeg。
6. 通过校验的 URL 只在内存里用；标准化形式（scheme / host 小写、去掉默认端口）用来判断"同一个地址已有进行中的会话"。

**推流密钥 / 凭据脱敏**（规则、覆盖范围和测试要求见 6.10）：地址里的用户信息、rtmp / rtmps 的流名（应用名之后的路径）、所有查询参数的值一律显示成 `***`；标题、`params`、任务日志、错误的 `message` / `detail`、所有事件 payload、后端日志都只出现脱敏后的地址；完整地址不落库、不写文件。

**任务字段**：`type` 是 `live_file_push` / `live_screen_push`；`title` 如 `文件推流：a.mp4 → rtmp://host/app/***`、`屏幕推流：显示器 1（主） → srt://host:9000?streamid=***&passphrase=***`；`inputPaths` 文件推流为 `[inputPath]`、屏幕推流为 `[]`；`outputPath` 是本地存档的最终路径，没存档为 `""`；`params` 见 6.10。

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
| `task:progress` | `{ id, version, progress, speed, etaSec, outTimeSec, fps?, bitrateKbps?, droppedFrames? }`（后三项只有直播任务才有，见下） | 每任务最多 4 次/秒 |
| `task:status` | `{ id, version, status, error?, outputPath?, finishedAt? }` | 状态变化时 |
| `task:removed` | `{ ids: string[] }` | 每次 |
| `ffmpeg:status` | `FFmpegStatus`（见第 9 节） | 检测完成、安装状态变化时 |

**直播指标（v0.10，取代 `live:stats`）**：直播任务的 `task:progress` 除 `speed`（如 `1.00x`，持续明显小于 1 说明编码跟不上）和 `outTimeSec`（已输出的媒体时长）外，还带 `fps`（当前输出帧率）、`bitrateKbps`（**近 5 秒**平均输出码率，由 ffmpeg `total_size` 和 `out_time` 的增量算出，不用 ffmpeg 自带的 `bitrate=`，那是从开始到现在的累计平均）、`droppedFrames`（ffmpeg 累计丢帧，不是网络丢包）；`progress` 恒为 -1，`etaSec` 为 0。没有单独的 `uptimeSec`：已推时长 = 现在 − `Task.startedAt`（墙钟），`outTimeSec` 是媒体时间，两者差距变大说明卡顿。这几项同时写进 `Task`（`fps` / `bitrateKbps` / `droppedFrames`，只在内存），页面刷新后 `ListActive` 能立刻显示当前值。

`task:created` 后任务状态为 `queued`；开始执行时发 `task:status`（`running`）；结束时发 `task:status`（终态）。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

前端任务 store 规则：先 `EventsOn` 订阅并缓存事件，再 `TaskService.ListActive()` 拉取 queued 和 running 任务，拉完按 `version` 回放缓存，版本不大于本地的事件直接丢弃。历史任务只在任务中心里用 `List` 分页加载。`task:progress` 只改进度字段，不替换对象。

## 6. SQLite 表

```sql
media(id PK, path, path_key UNIQUE, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at)
tasks(id PK, type, status, title, input_paths JSON, output_path, params JSON, progress, error JSON,
      log_path, version, created_at, started_at, finished_at)
presets(id PK, name, built_in, options JSON, sort)
edit_projects(id PK, name, project JSON, updated_at)
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
- 取消转换类任务直接强制结束进程；直播录制存档要先向 ffmpeg 发 `q`（或 SIGINT），等待最多 5 秒让它写完文件尾，超时再强制结束，否则 mp4 存档无法打开。
- `/local/<token>` 用 `http.ServeContent` 输出，支持 Range 请求，保证视频可拖动进度。

## 6.6 任务管理器实现约定（v0.7）

- 包 `internal/task`：`Manager.Submit(Spec, Runner)` 落库为 `queued` 并发 `task:created`；`batch` 池（`internal/ffmpeg` 转换 / 剪辑 / Office / 安装）按并发数 FIFO 排队，默认并发 `min(NumCPU/2, 3)` 且至少 1；`live` 池（两类直播）不排队、不占 batch 名额，且进度恒为 -1。
- 状态机：`queued → running → succeeded | failed | canceled | interrupted`。Runner 返回 nil 即 `succeeded`（含直播优雅停止：存档完整）；返回被取消的错误且用户请求过取消为 `canceled`；应用退出时被停止的任务（含还在排队的）为 `interrupted`；其余为 `failed`（`error` 带错误，ffmpeg 失败时 `detail` 为 stderr 最后 50 行）。
- 只有状态变化落库；进度只在内存。`task:progress` 同一任务最多 4 次/秒，被节流抑制的最后一次会在间隔到期后补发。`ListActive` / `Get` / `List` 返回运行中任务时带实时进度，`Speed` / `EtaSec` 不落库。
- 取消：排队中的直接移出队列变 `canceled`；运行中的取消 `ctx`，ffmpeg 任务结束整个进程组；直播任务先发 `q`（有外部 stdin 时发 SIGINT），最多等 5 秒再强制结束。已结束的任务取消返回 `TASK_CONFLICT`，不存在返回 `NOT_FOUND`。Windows 上结束整个进程树（v0.9.2：先终结进程所在的 Job Object，失败退回 `taskkill /T /F`，再失败只结束主进程）。
- `Retry`：用原任务的 `type` / `params` / `title` / `inputPaths` 重新提交，生成新任务（原任务保留）；原任务仍在进行返回 `TASK_CONFLICT`。每个任务类型注册一个 Factory 才支持重试（目前只有 `ffmpeg_install`），没有 Factory 的返回 `UNSUPPORTED`。
- `Remove(ids, deleteOutput)`：任一 id 仍在进行则整体失败（`TASK_CONFLICT`）；删除记录与日志，`deleteOutput=true` 时删除成功任务的输出文件（仅当输出路径是绝对路径、所在目录及上级不含符号链接、且是普通文件；否则只删记录并在日志里说明）；不存在的 id 忽略；发 `task:removed`。`ClearFinished` 只删记录和日志，不删输出。
- 日志：`<数据目录>/logs/<任务ID>.log`（单个任务最多 16 MB：写满 8 MB 轮转为 `.log.1`，单行最多 8 KB 超出截断），Runner 通过 `task.LogWriter(ctx)` 写入，`GetLog(id, tailLines)` 读取末尾若干行（最多读末尾 1 MB）。
- 输出文件用 `task.RunWithPart`：选出不冲突的最终路径（重名追加 `(1)`、`(2)`），写 `<name>.part.<原扩展名>`，成功后改名，失败或取消删除 `.part`。
- `task.FFmpegRunner` + `ffmpeg.Run` 是 ffmpeg 任务的通用执行体：自动加 `-hide_banner -nostats -y -progress pipe:1`（不需要 stdin 时再加 `-nostdin`；直播优雅停止和外部 stdin 的任务不加），解析 `out_time_us` / `speed` / `fps` / `bitrate` / `progress=end`，保留 stderr 尾部，提供错误分类钩子（直播的 `LIVE_*` 分类由直播 PR 提供）；`FFmpegRunner` 目前只支持单次 ffmpeg 调用（两遍编码暂缓，见 v0.7.2），`ProgressBase` / `ProgressScale` 用来把一次调用的进度映射到任务整体进度区间（供以后多步骤任务使用）。
- **子进程回收（v0.9.2）**：所有 ffmpeg / ffprobe 子进程经 `proc.Start` / `proc.Run` 启动。Windows 上会为每个子进程创建一个 Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）并把进程放进去，Job 句柄由应用进程持有：应用崩溃 / 被任务管理器结束时系统关闭句柄，ffmpeg 和它派生的进程一起被系统结束；进程正常退出后应用关闭句柄（同样结束遗留的子孙）。创建 / 加入 Job 失败不影响启动，此时结束进程树走 `taskkill /T /F`。macOS / Linux 不变（`Setpgid` 进程组；应用崩溃时 ffmpeg 不会被自动回收，读不到 stdin / 管道时通常会自己退出）。`proc.Start` 到加入 Job 之间有极短窗口，窗口内派生的孙进程不进 Job（ffmpeg 启动时不会立刻派生）。**没有 Windows 真机验证**，只有交叉编译和 Linux 上的回退顺序 / 登记表测试，Windows 专属测试文件已写但未运行。
- 启动：`store.MarkInterrupted` 在打开数据库后立即执行，上次未结束的 `queued` / `running` 变 `interrupted`，**不会自动恢复执行**，用户可在任务中心点重试。退出：`Manager.Shutdown` 取消所有任务并等待收尾（最多 8 秒），再关闭数据库。

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
  - 文件推流：`[-re] [-stream_loop -1] -i file:<path> [-f lavfi -i anullsrc=r=44100:cl=stereo]`（源文件没有音轨时补静音，很多服务器要求有音频）`-map 0:v:0 -map <音频>`，始终重编码：`-c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -b:v <k>k -maxrate <k>k -bufsize <2k>k -g <2×fps> -c:a aac -b:a <k>k -ar 44100 -ac 2`；`-re` 让 ffmpeg 按源速度读文件；`rtmp(s)` 用 `-f flv`，`srt` 用 `-f mpegts`。`-protocol_whitelist` 只放当前协议需要的（`rtmp,tcp` / `rtmps,tcp,tls,crypto` / `srt,udp`）。
  - 屏幕推流输入：Windows `-f gdigrab -framerate N [-draw_mouse 0] [-offset_x X -offset_y Y -video_size WxH] -i desktop`；macOS `-f avfoundation -framerate N -capture_cursor 0|1 -i "<设备序号>:none"`；Linux `-f x11grab -framerate N -draw_mouse 0|1 [-video_size WxH] -i <DISPLAY>+<x>,<y>`。输出尺寸补偶数（`scale=trunc(iw/2)*2:trunc(ih/2)*2`），编码参数同上；`audio=silent` 时加 `anullsrc`。
  - 存档（`archiveDir` 非空，只有屏幕推流）：用 ffmpeg `tee` muxer 一次编码同时写两个输出——`[f=flv]<url>|[f=mp4:movflags=+faststart]<archiveDir>/<title>-<yyyyMMdd-HHmmss>.part.mp4`（URL 已经保证不含 `|`）；文件走 `RunWithPart`（`.part` → 成功后改名），所以**只有 `succeeded`（含优雅停止）才留存档**，失败 / 中断按现有规则删 `.part`；`outputPath` = 存档最终路径。优雅停止必须成功写完 mp4 尾部，所以停止一定走 `q`。
- **采集能力检测**（`GetCaptureCapabilities` / `ListScreens` / `StartScreenPush` 共用一套逻辑）：Linux 读 `XDG_SESSION_TYPE`——`wayland`（即使有 XWayland 的 `DISPLAY`）或没有 `DISPLAY` → `supported=false`，`Start*` / `ListScreens` 返回 `UNSUPPORTED_PLATFORM`，不去录黑屏；macOS 需要"屏幕录制"授权，授权记在 FFmpegFree 名下（不是 ffmpeg 名下），查不出来时 `permission=unknown`，`Start` 时按 ffmpeg 报错分类为 `SCREEN_PERMISSION_DENIED`。显示器列表：Windows 用 `EnumDisplayMonitors`（`golang.org/x/sys/windows`，不用 cgo）；macOS 解析 `ffmpeg -f avfoundation -list_devices true -i ""` 里的 `Capture screen N`；Linux 解析 `xrandr --query`，没有 xrandr 时只返回一个 `x11:desktop`。
- **错误分类**：`ffmpeg.ClassifyLiveError`，规则和 6.9 一样——只看 `classifiableLines()` 剔除 `Input #` / `Output #` / `Stream mapping:` / `Metadata` / `Stream #` 段落后的行，系统错误文本按行尾匹配。区分"连接失败"和"中断"靠 Runner 记的 `started`（收到过第一条 progress）：
  - `started=false`：无法解析主机名 / 连接被拒绝 / 超时 / 网络不可达 → `LIVE_CONNECT_FAILED`；服务器明确拒绝 → `LIVE_PUSH_REJECTED`。**按 ffmpeg 7.1.5 + MediaMTX 1.21.1 实测**：RTMP 鉴权失败的 stderr 是 `[rtmp @ …] Server error: authentication failed` + `Error opening output …: Operation not permitted`（可区分，判为 `LIVE_PUSH_REJECTED`）；RTMP 服务器未开 / 连接被拒是 `Connection refused`，域名解析失败是 `Failed to resolve hostname`（均为 `LIVE_CONNECT_FAILED`）；**SRT 是已知局限**：服务器未开与被拒绝（错误 passphrase / 无权限）在 ffmpeg stderr 里都只有 `Connection to srt://… failed: Input/output error`，无法区分，统一判 `LIVE_CONNECT_FAILED`（`LIVE_PUSH_REJECTED` 对 SRT 实际上不会出现）。
  - `started=true`：`Broken pipe` / `Connection reset` / `Connection timed out` / 写出时 `Input/output error`、`Error writing trailer`（网络中断）→ `LIVE_PUSH_INTERRUPTED`。
  - 屏幕采集：avfoundation 权限相关报错 → `SCREEN_PERMISSION_DENIED`；`x11grab` 打不开显示 → `UNSUPPORTED_PLATFORM`。
  - 认不出来的非零退出 → `INTERNAL`（不是 `PROCESS_FAILED`），stderr 最后 50 行（已脱敏）放 `detail`。
  - 用户主动停止：优雅退出成功 `succeeded`；超时强杀 `canceled`（没有错误码）。**具体的 ffmpeg 报错措辞（各版本、各服务器不同）必须用真实推流服务器收集样本后落成测试**，设计稿里的关键词只是起点。
- **脱敏**（`internal/live`，纯函数，必须有表驱动测试）：
  - `RedactURL(raw string) string`：用户信息 → `***@`；rtmp / rtmps 保留 host、端口和第一段路径（应用名），其后的路径（流名，可含 `/`）→ `***`；所有查询参数保留键、值 → `***`；fragment 去掉；解析失败返回 `<invalid-url>`，绝不回显原文。例：`rtmp://u:p@h:1935/live/abc123?token=xyz` → `rtmp://***@h:1935/live/***?token=***`，`srt://h:9000?streamid=a&passphrase=b` → `srt://h:9000?streamid=***&passphrase=***`。
  - `NewRedactor(rawURL string) func(line string) string`：处理一行输出——① 原始 URL、它的 URL 编码 / 解码形式、以及从中提取的每个秘密片段（用户名、密码、流名、每个查询值，长度 ≥ 3）按字面替换成 `***`；② 再用正则 `(rtmps?|srt|tcp|tls|udp)://[^\s'"<>]+` 把行里残留的任何 URL 交给 `RedactURL`（覆盖 ffmpeg 改写后的形式，如 `tcp://host:1935?tcp_nodelay=0`）。
  - **覆盖范围**：`ffmpeg.RunOptions` 增加 `Redact func(string) string`，`Run` 在 stderr 每一行进入 `TailBuffer`、`OnStderr`（任务日志）、`Classify` **之前**先过它；记录的命令行也用脱敏后的 URL；`task.Spec.Title`、`Spec.Params`、`apperr` 的 `message` / `detail`、事件 payload、后端 `logf` 全部只用脱敏后的值。完整 URL 只存在于 Runner 的内存和 ffmpeg 的命令行参数里，不落库、不写文件。
  - **`params`（脱敏，不能用来重试）**：文件推流 `{"kind":"file","input":"/abs/a.mp4","url":"rtmp://h/live/***","loop":true,"options":{...}}`；屏幕推流 `{"kind":"screen","screenId":"monitor:0","url":"srt://h:9000?streamid=***","hideCursor":false,"audio":"none","archiveDir":"","options":{...}}`。
  - **测试要求**：URL 表驱动（各协议、userinfo、多段路径、IPv6、非法串）；行脱敏用真实 ffmpeg 输出样本；端到端用假 ffmpeg 脚本把完整 URL 打到 stderr 并失败，断言 `Task.title` / `Task.params` / `error.message` / `error.detail` / 日志文件 / 全部事件 payload 里都搜不到任何秘密片段。
  - **已知限制**：ffmpeg 命令行里必须有完整 URL，同一台机器上的其他进程（任务管理器、`ps`）能看到；应用不能规避，文档里说明。
- **指标**：`task.Progress` 增加 `Fps float64`、`BitrateKbps float64`、`DroppedFrames int64`，`task.ProgressEvent` 和 `Task` 增加同名字段（`omitempty`）；`FFmpegRunner` 从 `ffmpeg.ProgressUpdate`（已有 `Fps`、`Dropped`、`TotalSize`、`OutTimeSec`）填充，`BitrateKbps` 用相邻两次 progress 的 `total_size` / `out_time` 增量做 5 秒滑动平均（`out_time` 不增长时沿用上一个值，不出现 NaN / Inf）。其余节流、`version`、丢弃旧事件规则不变。
- **会话与任务管理器**：新增 `TypeLiveScreenPush`，`IsLive` 包含它；旧的 `TypeLiveRelay`、`TypeLiveRecordPush` 常量**保留但不再产生**（架构师定，见下方确认项 ⑧）：`Submit` 不再接受，`IsLive` 对它们仍为 true 只是为了常量兼容。不注册重试工厂，`Retry` 得到 `UNSUPPORTED`（message：直播会话不能重试，请重新开始推流）。进行中的会话同时最多 4 个；同一个标准化推流地址同时只能有一个会话（都是 `TASK_CONFLICT`）。应用退出：`Shutdown` 取消 → 优雅停止最多 5 秒 → 状态 `interrupted`；应用崩溃时 ffmpeg 子进程由操作系统回收（Windows 见 Job Object 修订）。
- **未验证（设计稿的已知风险，实现时要真机验证）**：macOS 屏幕录制授权的检测方式（不用 cgo 时只能靠 ffmpeg 报错或首帧内容判断）；Windows gdigrab 在多显示器 / 非 100% 缩放下偏移和尺寸是否等于物理像素；`x11grab` 在各桌面环境下的表现；上面所有 ffmpeg 报错关键词；RTMP / SRT 在不同服务器（nginx-rtmp、SRS、MediaMTX、常见直播平台）上的兼容性。

- **合并顺序（架构师定）**：#19（Live，本节）→ #22（Edit，6.11）→ #23（Doc，6.12），三份合并后的最终契约版本是 **v0.12**；每个 PR 的头部版本号只在合并时按"保留最高版本号、各自 vX 变更段和小节都保留"处理，本 PR 头部保持 v0.10。
- **已确认项**（原待定项 ①~⑨，不再待定）：
  - ①~⑦ **产品经理和架构师已正式确认**：① 同时进行的直播会话上限 4 个、同一标准化地址只允许一个会话；② 屏幕推流首版不采集声音（只有 `none` / `silent`）；③ 始终重编码（不支持 `-c copy` 直推文件）；④ 允许推到回环 / 内网地址；⑤ 只支持 rtmp / rtmps / srt，不含 rtsp / whip / http-flv 推流；⑥ 存档只用 mp4，且只有屏幕推流有存档；⑦ 优雅停止成功记 `succeeded`、强杀记 `canceled`，前端文案按此区分（硬性规则见 6.10 前文「结果语义」）。
  - ⑧ **架构师定**：任务中心**不展示** `live_relay` 和 `live_record_push`；这两个旧类型在契约里标为"保留但不再产生"（`Submit` 不接受）；数据库里若有旧记录，一律按未知类型**忽略、不报错**（`List` / `ListActive` / `Get` 等读取路径遇到类型不在当前枚举内的行时跳过，不返回错误、不影响其他记录）。目前没有任何代码产生过这两种记录。
  - ⑨ **前端负责**：由前端在 `v2-fe-api-contracts` 里补全 `AppErrorCode`（`CANCELED`、八个 `LIVE_*` 相关码、`PROBE_FAILED`、`UNSUPPORTED`、`CONVERT_DISK_FULL`），并对照第 2 节契约错误码表逐项核对。后端不改动。
- **SRT 说明（架构师 / 产品定）**：SRT 连接失败**统一判 `LIVE_CONNECT_FAILED`**（原因见上文实测：服务器未开与被拒绝在 ffmpeg stderr 里无法区分）。产品文案"连接失败，请检查地址和口令是否正确"由**前端负责**，后端 `message` **不承载该文案**（后端 `message` 只描述技术原因，`detail` 是脱敏后的 stderr 尾部）。
- **用户可见提示（来自产品经理，仅供前端参考；后端只保证错误码和触发条件，不返回这些文案）**：
  - `TASK_CONFLICT`：进行中的直播会话已达 4 个 → 前端提示"最多同时推 4 路"；同一标准化地址已有进行中的会话 → "这个地址已经在推流"。两种触发共用同一个错误码 `TASK_CONFLICT`。**（建议，待架构师确认，非已拍板）**：为方便前端区分文案，`detail` 第一行固定写 `reason=max_sessions`（达到上限）或 `reason=duplicate_url`（地址重复），其后才是脱敏说明；不确认则前端只能按 `message` 文本区分，不稳定。
  - `LIVE_URL_INVALID`：协议不是 rtmp / rtmps / srt 时**后端已经是这个码**（第 2 节错误码表与本节「推流地址校验规则」第 2 条一致：scheme 只允许 `rtmp`、`rtmps`、`srt`，其余一律 `LIVE_URL_INVALID`），前端提示"暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt"。同一个码还覆盖地址格式不合法、端口越界、srt listener / rendezvous 模式等，前端如需区分靠 `message`，不要靠猜测。

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
- 不依赖 ffmpeg 的：Office 转 PDF、PDF 预览、JSON 工具，始终可用。
- 首次启动检测到 `missing` 时弹一次确认框（"安装"或"稍后"），选"稍后"后写入 `Settings.ffmpegPromptDismissed = true`，之后只保留提示条，不再弹窗；ffmpeg 变为 ready 后该标记重置。
- 前端不轮询：检测完成、安装进度导致的 state 变化、手动指定路径、重新检测，都会推送 `ffmpeg:status`，payload 为完整 `FFmpegStatus`。
