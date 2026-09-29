# FFmpegFree v2 接口契约（v0.12）

v0.12 变更（DocService 契约定稿，**只有契约，尚无实现**，见 6.12 节）：`ConvertToPDF` 保持签名，格式范围如实收窄为 `docx` / `xlsx` / `pptx` **纯文本版**（与 v1 一致：无图片、表格线、样式；旧版 `doc` / `xls` / `ppt` 及其他格式一律 `UNSUPPORTED`）；`GetPDFURL` 替换为 `OpenPDF`（返回 `PDFSource`）+ `ReadPDFChunk`（分块读，走 Wails Bind，不依赖 AssetServer 行为）；新增 `GetDocCapabilities` / `ListRecentPDFs` / `RemoveRecentPDFs`；新增表 `doc_recent`；任务类型 `office_pdf` 保持不变；PDF 渲染、页数、缩略图、搜索全部在前端 pdf.js（`@tato30/vue-pdf`）完成，后端不渲染、不提供合并 / 拆分 / 旋转（v1 也没有）。


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
| TASK_CONFLICT | 任务状态不允许该操作（如取消已完成任务） |
| IO_ERROR | 读写文件失败 |
| PROBE_FAILED | 文件存在但 ffprobe 无法解析（损坏、不是音视频文件、没有可识别的流） |
| CANCELED | 调用因应用退出（根 ctx 取消）而被取消，结果作废；前端不需要提示用户 |
| UNSUPPORTED | 该操作不支持这个对象（如没有重试工厂的任务类型不能 Retry） |
| CONVERT_DISK_FULL | 转换写输出文件时磁盘空间不足（前端标题「磁盘空间不足」，可引导用户换输出目录） |
| PROCESS_FAILED | 子进程非零退出，detail 带最后 50 行日志 |
| UNSUPPORTED_PLATFORM | 当前系统或会话不支持该功能（如 Linux Wayland 下的屏幕采集） |
| LIVE_URL_INVALID | 直播地址格式不合法或协议不支持 |
| LIVE_CONNECT_FAILED | 连接推流 / 拉流目标失败（DNS、拒绝连接、超时） |
| LIVE_PUSH_REJECTED | 目标服务器拒绝推流（鉴权失败、流名冲突等） |
| LIVE_PUSH_INTERRUPTED | 推流过程中被目标服务器或网络中断 |
| SCREEN_PERMISSION_DENIED | 没有屏幕录制权限（macOS 系统授权） |
| INTERNAL | 其他；ffmpeg 异常退出时 detail 带 ffmpeg stderr 的最后若干行 |

后端返回的直播 / 录屏错误码就是上表这些。`LIVE_PLAY_FAILED`（播放器加载或解码失败）和 `LIVE_CORS_BLOCKED`（拉流地址跨域被浏览器拦截）**只在前端由播放器产生**，后端不会返回，也不在 `apperr` 里定义。

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
    Params     string     `json:"params"`     // 原始参数 JSON，用于重试
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

### LiveService
```go
StartFilePush(req FilePushRequest) (Task, error)      // 文件推流，支持循环
StartRelay(req RelayRequest) (Task, error)            // 拉流转推，多目标
StartRecordPush(req RecordPushRequest) (RecordSession, error) // 返回 wsURL + token，前端 MediaRecorder 往里写
Stop(taskID string) error
GetHealth() (LiveHealth, error)
ListArchives() ([]MediaInfo, error)
GetPlayURL(sourceURL string) (string, error)          // FLV 拉流播放经本地服务代理
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
| `task:progress` | `{ id, version, progress, speed, etaSec, outTimeSec }` | 每任务最多 4 次/秒 |
| `task:status` | `{ id, version, status, error?, outputPath?, finishedAt? }` | 状态变化时 |
| `task:removed` | `{ ids: string[] }` | 每次 |
| `live:stats` | `{ id, bitrateKbps, fps, droppedFrames, uptimeSec }` | 每秒 1 次 |
| `ffmpeg:status` | `FFmpegStatus`（见第 9 节） | 检测完成、安装状态变化时 |

`task:created` 后任务状态为 `queued`；开始执行时发 `task:status`（`running`）；结束时发 `task:status`（终态）。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

前端任务 store 规则：先 `EventsOn` 订阅并缓存事件，再 `TaskService.ListActive()` 拉取 queued 和 running 任务，拉完按 `version` 回放缓存，版本不大于本地的事件直接丢弃。历史任务只在任务中心里用 `List` 分页加载。`task:progress` 只改进度字段，不替换对象。

## 6. SQLite 表

```sql
media(id PK, path, path_key UNIQUE, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at)
tasks(id PK, type, status, title, input_paths JSON, output_path, params JSON, progress, error JSON,
      log_path, version, created_at, started_at, finished_at)
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
- 两个调度池：`batch` 池（转换、剪辑、Office、ffmpeg 下载）按设置里的并发数排队；`live` 池（三类直播任务）不排队、不占 batch 名额。
- 两遍编码 / 目标大小压缩：暂缓（v0.7.2），设计保留：每个任务用 `-passlogfile <任务专属临时目录>/pass`，进度第一遍 0~0.5，第二遍 0.5~1，结束后删临时目录。
- 输出文件先写 `<name>.part.<原扩展名>`（例如 `a.part.mp4`，保留扩展名让 ffmpeg 能识别封装格式），成功后改名；目标重名时自动追加 `(1)`、`(2)`；取消或失败删除 `.part`。
- 进度只保存在内存并通过 `task:progress` 推送，不写库；只有状态变化（开始、成功、失败、取消）时落库，避免单连接下进度写入阻塞任务中心的列表查询。
- 取消转换类任务直接强制结束进程；直播录制存档要先向 ffmpeg 发 `q`（或 SIGINT），等待最多 5 秒让它写完文件尾，超时再强制结束，否则 mp4 存档无法打开。
- `/local/<token>` 用 `http.ServeContent` 输出，支持 Range 请求，保证视频可拖动进度。

## 6.6 任务管理器实现约定（v0.7）

- 包 `internal/task`：`Manager.Submit(Spec, Runner)` 落库为 `queued` 并发 `task:created`；`batch` 池（`internal/ffmpeg` 转换 / 剪辑 / Office / 安装）按并发数 FIFO 排队，默认并发 `min(NumCPU/2, 3)` 且至少 1；`live` 池（三类直播）不排队、不占 batch 名额，且进度恒为 -1。
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
| `.doc` `.xls` `.ppt`（旧二进制格式）、`.odt` `.ods` `.odp` `.rtf` `.pages` `.numbers` `.key`、其他 | `UNSUPPORTED`，detail 写明原因；旧格式提示"请先另存为 docx / xlsx / pptx" |
| `.csv` `.txt` | **首版不支持**（架构师定，v1 也没有），`UNSUPPORTED`，detail "暂不支持该格式"；以后要加走增量契约版本 |
| 密码加密的 docx / xlsx / pptx（OLE 容器，不是 zip） | `UNSUPPORTED`，detail "加密文档不支持" |

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
- **`UNSUPPORTED` 规则保留**：内嵌字体加载失败（构建错误才会发生）**且**没有可用系统 `.ttf`，而文档又含 U+00FF 以上的字符 → 该文件 `UNSUPPORTED`，detail "没有可用的 Unicode 字体"（不输出乱码 PDF）；纯 Latin-1 文档可用内置字体。正常构建下内嵌字体总是可用，该分支基本不会触发，但校验和单元测试要覆盖。
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

- 参数校验与 6.9 同一套规则：`inputs` 非空且 ≤ 50，路径必须绝对（`INVALID_ARGUMENT`），文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读权限 `IO_ERROR`；`outputDir` 规则同 6.9（空 = `Settings.defaultOutputDir`，仍空 = 源文件所在文件夹），并在**提交时**同步校验（不放到任务里失败）：必须是绝对路径；**拒绝以 `\\?\`、`\\.\` 开头的路径**（`INVALID_ARGUMENT`）；**拒绝位于应用数据目录之内（含其本身）的路径**（`os.UserConfigDir()/FFmpegFree/`，防止把输出写进 `app.db`、`thumbs/`、`logs/` 旁边并被清理逻辑误伤，`INVALID_ARGUMENT`，`detail` 写 `outputDir 不能在应用数据目录内`；比较前对两边做 `EvalSymlinks` + 大小写按平台规则规范化）；已存在必须是目录且可写，不存在则最近的已存在上级必须是可写目录。**"可写"的判断方式（#29 实现反馈）**：在该目录里**创建一个探测文件**（`os.CreateTemp(dir, ".ffmpegfree-probe-*")`，创建成功后立即关闭并删除），**不用**权限位或 `access()` 推断（Windows 的 ACL、只读挂载、网络盘上权限位不可靠）；创建失败（含权限不足、只读、磁盘满）一律返回 `IO_ERROR`，`detail` 写系统错误文本；探测文件删除失败只记日志，不影响结果。**先整体校验再提交**，任何一个不通过整体失败、不提交任何任务，`detail` 第一行是出错文件路径。
- 整体校验里额外检查：扩展名在支持表内（否则 `UNSUPPORTED`）；文件 ≤ 100 MiB（否则 `INVALID_ARGUMENT`）；能作为 zip 打开且含必需部件（docx `word/document.xml`，xlsx `xl/workbook.xml`，pptx 至少一张 `ppt/slides/slide<n>.xml`），打不开或缺部件 `INVALID_ARGUMENT`（detail "不是有效的 OOXML 文件"）；不是 zip 而是 OLE 头（`D0 CF 11 E0`）→ `UNSUPPORTED`（加密或旧格式改了扩展名）；单个 zip 条目解压后 > 256 MiB `INVALID_ARGUMENT`（防 zip 炸弹）；字体规则见 6.12.1（需要 Unicode 字体而没有 → `UNSUPPORTED`，此项在提交时对文本做一次快速扫描，不通过整体失败）。
- **不依赖 ffmpeg**（不做 `FFMPEG_NOT_FOUND` 门控）。走 batch 池（与转换共用并发数）；`GoFuncRunner` 实际是 `task.RunnerFunc`。
- 任务：`type=office_pdf`，`title` 形如 `a.docx → PDF`，`inputPaths=[源]`，`outputPath` 为预期输出，`params={input, outputDir}` JSON。输出 `<源文件名去扩展名>.pdf`，重名追加 `(1)`、`(2)`，不覆盖，走 6.6 `RunWithPart`（`.part.pdf` → 原子改名）；取消或失败不留 `.part`。**启动时 `.part` 遗留清理是可选功能（契约"允许"，是否做由实现 PR 决定）**：若做，必须与 #22 的 6.11.3「`.part` 遗留清理」**五个条件完全一致，缺一不可**——① 文件名只能是由 `interrupted` 的 `office_pdf` 任务 `outputPath` 推出的 `<name>.part.pdf` 和 `<name>(n).part.pdf`（n=1..99）；② 位于该任务记录里登记的 `outputPath` 所在目录（不是任意目录，也不单独扫描默认输出目录）；③ 修改时间早于本次启动；④ 仅普通文件，`Lstat` 不跟随链接，符号链接和目录不动；⑤ 不递归，只看输出目录第一层，不 `ReadDir`。任务管理器统一版（覆盖 `convert`、`office_pdf`）后续单独做（见 6.11.3）。
- **进度**：按处理单元计数（docx 段落、xlsx 行、pptx 幻灯片）占总数的比例，0~1 单调，完成为 1；每处理约 100 个单元检查一次 ctx，取消响应 ≤ 1 秒（超大文件除外）。`task:progress` 载荷不变，`speed` / `etaSec` 为空。
- 页数上限 5000：**输出页数超过 5000（生成过程中累计到第 5001 页时立即停止）返回 `UNSUPPORTED`**，detail "超过 5000 页"，不产生输出文件（`.part` 删除）；xlsx 一个工作表所有行都算；xlsx 单元格文本每格最多 32 767 字符（Excel 自身上限），超出截断。
- 错误码（任务的 `error`）：`IO_ERROR`（读写失败，没有权限）、`CONVERT_DISK_FULL`（输出写盘失败且是磁盘满，判定规则同 6.9 的系统错误文本匹配；Office 转换也用这个码，前端标题相同）、`UNSUPPORTED`、`INVALID_ARGUMENT`（运行时才发现的损坏）、`INTERNAL`（fpdf / excelize 意外错误，`detail` 是错误文本）；取消是任务状态 `canceled`。
- `Retry`：注册 `office_pdf` 的重试工厂，用 `params` 重建并重新校验（输入被删除 `NOT_FOUND`，不产生新任务）。
- v1 的"按文件名防重复转换"（`officeConvertingFiles`）取消：两个任务转同一个输入是允许的，输出各自取不冲突的名字。

### 6.12.4 PDF 预览方案（不做本地流服务）

渲染**完全在前端**：沿用 v1 的 `@tato30/vue-pdf`（pdf.js），后端不渲染成图片、不提供页数 / 文本 / 缩略图接口（后端无纯 Go 的可靠 PDF 渲染器，也不打包 `pdftoppm` 之类外部程序）。后端只负责把字节交给前端：

1. `OpenPDF(path)`：路径必须绝对（`INVALID_ARGUMENT`）、存在（`NOT_FOUND`）、是文件（否则 `INVALID_ARGUMENT`）、可读（`IO_ERROR`）、扩展名 `.pdf`（不区分大小写，否则 `INVALID_ARGUMENT`）、前 1024 字节内含 `%PDF-`（否则 `INVALID_ARGUMENT`，detail "不是 PDF 文件"）、大小 ≤ 512 MiB（否则 `INVALID_ARGUMENT`）。成功后登记句柄并写入 / 更新 `doc_recent`。加密 PDF 也能打开，密码由前端 pdf.js 的 `onPassword` 弹窗处理，后端不接触密码。
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
    "version": 1, "error": null, "createdAt": 1790000000000, "startedAt": 0, "finishedAt": 0 } ]
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

AppError（转换不支持的格式；`detail` 第一行是出错文件路径）：
```json
{ "code": "UNSUPPORTED", "message": "不支持转换该格式", "detail": "C:\\Docs\\旧文档.doc\n旧版 DOC 格式暂不支持，请先另存为 docx" }
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

## 7. 本地流服务（唯一保留的 HTTP）

> **v0.5：本节的本地 FLV / WebSocket 流服务取消，不再实现。** 推流由后端 ffmpeg 直接推到用户填写的目标地址；播放由前端播放器直接拉取远端地址；后端不再监听任何本地端口，也就没有 `/ws/record`、`/flv/<id>`、token 和 `wsURL`。下面保留的原文仅供参考，其中 `/ws/record`、`/flv/<id>` 相关内容作废；第 4 节 LiveService 的 `StartRecordPush`（wsURL）、`GetPlayURL` 需随直播 PR 一并修订。

- 监听 `127.0.0.1:0`（随机端口），启动时生成随机 token，所有请求必须带 `?t=<token>`。
- 只提供：`/ws/record`（录屏二进制分片写入 ffmpeg stdin）、`/flv/<id>`（拉流播放代理）。
- 端口和 token 只通过 `LiveService` 返回，不写死在前端。
- 录屏采集按平台降级：Windows（WebView2）用 `getDisplayMedia` + `MediaRecorder` 经 `/ws/record` 写入；macOS 和 Linux 默认由后端 ffmpeg 直接采集（`avfoundation` / `x11grab`），`RecordPushRequest.captureMode` 取 `webview | native`，由 `LiveService.GetCaptureCapabilities()` 告诉前端当前平台支持哪种。native 采集输入：Windows `gdigrab`、macOS `avfoundation`（首次会弹系统"屏幕录制"授权，记在 FFmpegFree 名下）、Linux `x11grab`；Linux 检测到 `XDG_SESSION_TYPE=wayland` 时返回 `UNSUPPORTED_PLATFORM` 错误，不录黑屏。

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
