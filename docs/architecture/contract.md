# FFmpegFree v2 接口契约（v0.9）

v0.9 变更：ConvertService 最小版落地（单文件转换、常见格式与预设、进度、取消、重试；**不含**两遍编码 / 按目标大小压缩，见 v0.7.2）：`ConvertOptions` 新增 `crf`；新增错误码 `CONVERT_DISK_FULL`（磁盘空间不足，原来归在 `IO_ERROR`）；`Submit` 一次最多 50 个文件、先整体校验再提交；详见新增 6.9 节。

v0.8 变更：MediaService 落地（第 3、4 节 + 新增 6.7 节）：`MediaInfo` 扩展（container / fps / rotation / streams 等，见 6.7）；新增错误码 `PROBE_FAILED`；`Probe` 单个文件失败不影响整批（该项 `error` 有值）；新增 `Thumbnail`；`ThumbURL` 是 data URL（本地 HTTP 已取消）。

v0.7.4 变更：`Settings` 新增 `defaultOutputDir`（string，空字符串 = 输出到源文件所在文件夹）。`UpdateSettings` 对非空值校验：必须是绝对路径、已存在的文件夹且可写，否则 `INVALID_ARGUMENT`，整个更新不生效（与 `ffmpegPath` 一样是原子的）；保存的是清理后的路径。转换等任务的 `outputDir` 传空时使用这个默认值。

v0.7.3 变更：`SystemService.PickDirectory(title string)`（契约里原来无参，现在加 `title`，空字符串用默认标题，用户取消返回 `""` 而不是错误）与 `RevealInFolder(path)` 落地（见 6.8 节）。

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
- 拖拽文件：开启 Wails `DragAndDrop.EnableFileDrop`，由 `OnFileDrop` 拿到绝对路径后发 `app:files-dropped` 事件给前端，前端不从 WebView 的 File 对象取路径。
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

type TaskType string // convert | edit_render | office_pdf | live_file_push | live_relay | live_record_push | ffmpeg_install
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
UpdateSettings(s Settings) error      // defaultOutputDir（空=与源文件同目录）、并发数、主题、语言、ffmpegPromptDismissed、ffmpegPath
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
| `app:files-dropped` | `{ paths: string[], x, y }` | 每次拖放 |

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
- 取消：排队中的直接移出队列变 `canceled`；运行中的取消 `ctx`，ffmpeg 任务结束整个进程组；直播任务先发 `q`（有外部 stdin 时发 SIGINT），最多等 5 秒再强制结束。已结束的任务取消返回 `TASK_CONFLICT`，不存在返回 `NOT_FOUND`。Windows 上结束整个进程树。
- `Retry`：用原任务的 `type` / `params` / `title` / `inputPaths` 重新提交，生成新任务（原任务保留）；原任务仍在进行返回 `TASK_CONFLICT`。每个任务类型注册一个 Factory 才支持重试（目前只有 `ffmpeg_install`），没有 Factory 的返回 `UNSUPPORTED`。
- `Remove(ids, deleteOutput)`：任一 id 仍在进行则整体失败（`TASK_CONFLICT`）；删除记录与日志，`deleteOutput=true` 时删除成功任务的输出文件；不存在的 id 忽略；发 `task:removed`。`ClearFinished` 只删记录和日志，不删输出。
- 日志：`<数据目录>/logs/<任务ID>.log`（单个任务最多 16 MB：写满 8 MB 轮转为 `.log.1`，单行最多 8 KB 超出截断），Runner 通过 `task.LogWriter(ctx)` 写入，`GetLog(id, tailLines)` 读取末尾若干行（最多读末尾 1 MB）。
- 输出文件用 `task.RunWithPart`：选出不冲突的最终路径（重名追加 `(1)`、`(2)`），写 `<name>.part.<原扩展名>`，成功后改名，失败或取消删除 `.part`。
- `task.FFmpegRunner` + `ffmpeg.Run` 是 ffmpeg 任务的通用执行体：自动加 `-hide_banner -nostats -progress pipe:1`，解析 `out_time_us` / `speed` / `fps` / `bitrate` / `progress=end`，保留 stderr 尾部，提供错误分类钩子（直播的 `LIVE_*` 分类由直播 PR 提供）；支持两遍编码的进度映射（`ProgressBase` / `ProgressScale`）。
- 启动：`store.MarkInterrupted` 在打开数据库后立即执行，上次未结束的 `queued` / `running` 变 `interrupted`，**不会自动恢复执行**，用户可在任务中心点重试。退出：`Manager.Shutdown` 取消所有任务并等待收尾（最多 8 秒），再关闭数据库。

## 6.7 媒体探测与缩略图实现约定（v0.8）

- 门控：`ffmpeg.RequireProbe()`；缺 ffmpeg 或 ffprobe 时整个调用返回 `FFMPEG_NOT_FOUND`。
- 路径：经 `paths.Normalize`；文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读取权限 `IO_ERROR`，ffprobe 解析失败 / 无流 / 超时（30 秒）`PROBE_FAILED`（detail 是 ffprobe stderr 最后 50 行）。所有 ffmpeg / ffprobe 输入都写成 `file:<路径>`，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。
- `Probe`：`-v error -print_format json -show_format -show_streams`；`width` / `height` 是**显示尺寸**（按 `rotation` 90 / 270 交换），各流的编码尺寸在 `streams[]`；`rotation` 为逆时针角度，取自 display matrix（兼容旧 `rotate` 标签）；`duration` / `bitrate` 缺失（`N/A`）时用各流的值；封面图（`attached_pic`）不算视频画面；`fps` 取 `avg_frame_rate`，为 0 时用 `r_frame_rate`，保留 3 位小数。同一文件（同 `path_key`）再次探测保留原 `id`。失败的文件不入库。最多 4 个文件并行。
- `Thumbnail`：jpg，最大宽度 `width`（默认 320，范围 16~1920，不放大，高度自动取偶数），按旋转元数据转正；`atSec` 超出时长时退回第 0 秒；没有视频画面返回 `INVALID_ARGUMENT`。同时返回缓存文件路径和 data URL。同时最多 2 个 ffmpeg 在生成；相同参数并发只生成一次。
- 缓存：`<数据目录>/thumbs/<sha1(path_key, mtime, size, atSec 毫秒, width)>.jpg`，先写 `.part.jpg` 再改名；文件的修改时间或大小变化即失效。容量上限 1000 个文件或 200 MB（超过则按最后使用时间从旧到新删到上限的 80%），启动时清理一次并每生成 50 张清理一次，超过 1 小时的 `.part.jpg` 残留会被清掉。`RemoveRecent` 不删缓存。
- `Probe` 附带的默认缩略图取时长的 10%（最多 10 秒），宽 320；`ListRecent` 只在该缩略图已缓存时返回 `thumbUrl`，不会为此启动 ffmpeg。

## 6.9 ConvertService 实现约定（v0.9）

- 绑定：`ListPresets() []Preset`、`SavePreset(p Preset) Preset`、`DeletePreset(id)`、`Submit(inputs []string, opts ConvertOptions, outputDir string) []Task`。均返回 `INTERNAL`（"转换服务尚未初始化"）直到启动完成。
- **预设**：12 个内置（`builtIn=true`，id 形如 `builtin-mp4-h264`：MP4 H.264 / 1080p / 720p / H.265、MKV 只换封装、MOV、WebM VP9、GIF、MP3、M4A、WAV、FLAC），不能修改或删除（`INVALID_ARGUMENT`，改为"另存为"：`id` 传空新建）。`SavePreset`：`id` 空 = 新建（后端生成 id），否则更新该用户预设（不存在 `NOT_FOUND`）；名称去首尾空白后 1~60 字；`options` 必须通过校验；用户预设最多 100 个；`builtIn` 字段传什么都忽略。`ListPresets` 内置在前，用户预设按创建顺序在后。
- **容器与编码**：容器 `mp4 mkv mov webm avi flv gif mp3 aac m4a wav flac ogg opus`；视频编码 `copy h264 h265 vp9`，`""` 表示不要视频（仅对视频容器有意义，等于只保留音频）；音频编码 `copy aac mp3 opus vorbis flac pcm ac3 none`，`""` 取容器默认（mp4/mov/mkv/flv=aac，webm=opus，avi=mp3，mp3/aac/m4a/wav/flac/ogg/opus 对应各自编码），`none` 去掉音轨。每个容器只允许合适的编码组合，不合法返回 `INVALID_ARGUMENT` 并在 message 里列出可选项。音频容器不能设分辨率/帧率、不能 `none`；`gif` 没有音轨、不能设码率；`copy` 不能缩放、不能设 crf/码率。
- `width` / `height`：只给一个按比例缩放；两个都给时等比缩进该框内并补黑边；输出宽高保证是偶数。`trimStart` / `trimEnd`（秒，`trimEnd=0` 到结尾）；开始时间超过时长返回 `INVALID_ARGUMENT`。`targetSizeMb` 暂缓，>0 返回 `INVALID_ARGUMENT`。
- **Submit**：一个文件一个 `convert` 任务（batch 池按并发数排队），返回的任务与 `inputs` 一一对应；任务 `title` 形如 `a.mkv → MP4`，`inputPaths` 为源文件，`outputPath` 为预期输出（完成时以实际路径为准，重名会带 `(n)`），`params` 是 `{input, options, outputDir}` 的 JSON。
  - 一次最多 **50** 个文件（更多由前端分批），空列表 `INVALID_ARGUMENT`。
  - **先整体校验再提交**：ffmpeg 就绪（否则 `FFMPEG_NOT_FOUND`）、参数合法、每个文件都能被探测（`NOT_FOUND` / `PROBE_FAILED` / 目录 `INVALID_ARGUMENT`）且与参数兼容（如无音轨转 mp3、无画面转视频容器 → `INVALID_ARGUMENT`）。任何一个不通过整体失败，**不提交任何任务**，错误 `detail` 第一行是出错的文件路径。视频输入没有音轨转视频容器不算错误，静默不带音频。
  - **输出目录**：`outputDir` 非空用它；为空用 `Settings.defaultOutputDir`；仍为空则输出到各自源文件所在文件夹。必须是绝对路径（`INVALID_ARGUMENT`），不存在的目录会在任务开始时创建。输出名为 `<源文件名去扩展名>.<新扩展名>`，重名依次追加 `(1)`、`(2)`，绝不覆盖已有文件（含输入文件本身），走 6.6 的 `.part` + 原子改名。
  - 进度按输出时长（已扣除裁剪）计算，0~1 单调递增，完成为 1；取消后没有最终文件也没有 `.part`。
- **任务失败的错误码**（在任务的 `error` 上）：`CONVERT_DISK_FULL`（No space left on device / ENOSPC / Windows "There is not enough space on the disk" / 磁盘配额）、`IO_ERROR`（没有权限、只读、找不到路径）、`PROBE_FAILED`（输入损坏）、`PROCESS_FAILED`（缺少编码器或滤镜、编码与格式不兼容，detail 带 ffmpeg 最后 50 行）。
- **Retry**：`convert` 任务注册了重试工厂，用 `params` 重建：重新探测输入、重新校验参数（输入已删除返回 `NOT_FOUND`，不产生新任务），输出目录用原来解析好的那个（之后改 `defaultOutputDir` 不影响）。
- 所有 ffmpeg 输入输出路径都带 `file:` 前缀，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。

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

### 6.8 RevealInFolder / PickDirectory（v0.7.3）

- `RevealInFolder(path)`：path 必须是绝对路径（空 / 相对路径 → `INVALID_ARGUMENT`），必须存在（否则 `NOT_FOUND`）。Windows 执行 `explorer /select,<path>`，macOS `open -R <path>`，Linux `xdg-open <所在文件夹>`；path 是文件夹时三个平台都直接打开这个文件夹。命令启动后立即返回，启动失败 `PROCESS_FAILED`。Linux 没有统一的"选中文件"方式，所以只能打开所在文件夹。
- `PickDirectory(title)`：弹出系统选择文件夹对话框，返回绝对路径；取消返回 `""`。应用启动完成前调用返回 `INTERNAL`。
