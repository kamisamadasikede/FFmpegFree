# FFmpegFree v2 接口契约（v0.11）

v0.11 变更（EditService 契约定稿，**只有契约，尚无实现**，见 6.11 节）：`Render` 改名 `Export`，任务类型 `edit_render` 改名 `edit_export`（旧名从未产生过任务，无迁移问题）；新增 `ValidateProject` / `DeleteProject` / `GetPreviewURL`；`SaveProject` 返回 `EditProjectMeta`，`LoadProject` 返回 `LoadedProject`；`EditProject` 字段与校验范围、导出参数、错误码、预览方案全部写死；预览走 AssetServer 的 `/local/<token>`（不做本地流服务，不用 `file://`），并明确 Windows 上 AssetServer 不支持流式响应、单次响应必须限长。


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
    Sources       []string     `json:"sources"`       // 素材库：绝对路径，去重，最多 200 个，只是列表，不保证存在
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
1. **工程级**：`videoTrack` 不能为空（v1 同）；clip 总数（视频 + 音频）≤ 100；`sources` ≤ 200 且都是绝对路径；名称 1~80 字；序列化后 ≤ 1 MiB；`schemaVersion` = 1；输出参数范围；时间线总长 ≤ 6 小时（**按 clip 自填值 `max(startSec + (outSec − inSec) / speed)` 检查，不扣转场、不探测素材**，见 E）。
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
  "progress": 0, "version": 1, "error": null }
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
- 表 `edit_projects(id, name, project JSON, updated_at)` 已在第 6 节。`SaveProject`：`id` 空 = 新建（ULID），否则更新（不存在 `NOT_FOUND`）；名称重复允许。**只校验数量上限（架构师定）**：名称去首尾空白后 1~80 字、clip 总数 ≤ 100、`sources` ≤ 200、序列化后 ≤ 1 MiB、`schemaVersion` ≤ 1，超了 `INVALID_ARGUMENT`。**不校验**同轨重叠、`outSec`、`speed` 等取值范围、路径是否存在（草稿可以保存，比如正在拖动中的时间线）；这些只在 `ValidateProject` 和 `Export` 报。所以 `LoadProject` 可能读出不合法的草稿，前端要能显示，导出前再调 `ValidateProject`。
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
- 不依赖 ffmpeg 的：Office 转 PDF、PDF 预览、JSON 工具，始终可用。
- 首次启动检测到 `missing` 时弹一次确认框（"安装"或"稍后"），选"稍后"后写入 `Settings.ffmpegPromptDismissed = true`，之后只保留提示条，不再弹窗；ffmpeg 变为 ready 后该标记重置。
- 前端不轮询：检测完成、安装进度导致的 state 变化、手动指定路径、重新检测，都会推送 `ffmpeg:status`，payload 为完整 `FFmpegStatus`。
