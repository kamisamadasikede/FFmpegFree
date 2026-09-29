# FFmpegFree v2 接口契约（v0.4）

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
| PROCESS_FAILED | 子进程非零退出，detail 带最后 50 行日志 |
| UNSUPPORTED_PLATFORM | 当前系统或会话不支持该功能 |
| INTERNAL | 其他 |

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
    ThumbURL   string  `json:"thumbUrl"`
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
    TargetSizeMB float64 `json:"targetSizeMb"` // >0 时按目标大小反推码率（两遍编码）
    TrimStart    float64 `json:"trimStart"`
    TrimEnd      float64 `json:"trimEnd"`
}
```

## 4. Service 方法

### SystemService
```go
PickFiles(filter FileFilter, multiple bool) ([]string, error)
PickDirectory() (string, error)
RevealInFolder(path string) error
GetEnv() (EnvInfo, error)            // 系统、ffmpeg 版本、数据目录
GetSettings() (Settings, error)
UpdateSettings(s Settings) error      // 输出目录、并发数、主题、语言、ffmpegPromptDismissed、ffmpegPath
```

### MediaService
```go
Probe(paths []string) ([]MediaInfo, error)   // 批量探测，结果写入 media 表
ListRecent(limit int) ([]MediaInfo, error)
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
- 两遍编码：每个任务用 `-passlogfile <任务临时目录>/pass`，进度第一遍映射到 0~0.5，第二遍映射到 0.5~1，结束后删临时目录。
- 输出文件先写 `<name>.part.<原扩展名>`（例如 `a.part.mp4`，保留扩展名让 ffmpeg 能识别封装格式），成功后改名；目标重名时自动追加 `(1)`、`(2)`；取消或失败删除 `.part`。
- 进度只保存在内存并通过 `task:progress` 推送，不写库；只有状态变化（开始、成功、失败、取消）时落库，避免单连接下进度写入阻塞任务中心的列表查询。
- 取消转换类任务直接强制结束进程；直播录制存档要先向 ffmpeg 发 `q`（或 SIGINT），等待最多 5 秒让它写完文件尾，超时再强制结束，否则 mp4 存档无法打开。
- `/local/<token>` 用 `http.ServeContent` 输出，支持 Range 请求，保证视频可拖动进度。

## 7. 本地流服务（唯一保留的 HTTP）

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

### 9.2 安装位置

装到应用自己的 `<数据目录>/bin/`，**不修改系统 PATH**。原因：改 PATH 在 Windows 要管理员权限、在 macOS 和 Linux 要改 shell 配置，还可能和用户已有的 ffmpeg 冲突，卸载也清不干净。应用内部统一用绝对路径调用，效果和装进环境一样。

### 9.3 下载流程

- 下载源做成按平台和架构（windows-amd64、darwin-arm64、darwin-amd64、linux-amd64、linux-arm64）的清单，清单里写 URL、SHA256、版本号；清单本身放在项目仓库的 release 里，可以随时换源，并支持配置国内镜像。
- 作为 `ffmpeg_install` 任务进 batch 池，复用任务进度事件；支持断点续传（HTTP Range），下载完先校验 SHA256，再解压到临时目录，二次运行 `-version` 验证通过后原子改名到 `bin/`。
- macOS 和 Linux 解压后 `chmod 0755`；macOS 上若 `-version` 运行失败，先执行 `codesign -s - <文件>` 做 ad-hoc 签名再验证一次。
- `InstallFFmpeg` 幂等：已有进行中的安装任务时直接返回该任务，不开第二个下载。
- 失败时保留已下载部分，提示"重试"或"手动选择 ffmpeg 所在位置"（离线用户的出路）。

### 9.4 接口

```go
type FFmpegStatus struct {
    State     string `json:"state"`     // checking | ready | missing | outdated | installing | failed
    Path      string `json:"path"`
    Version   string `json:"version"`
    Source    string `json:"source"`    // custom | bundled | system | legacy
    TaskID    string `json:"taskId"`    // installing 时对应的安装任务
    Error     *AppError `json:"error"`
}

// SystemService
GetFFmpegStatus() (FFmpegStatus, error)
InstallFFmpeg(mirror string) (Task, error)   // mirror 为空用默认源
SetFFmpegPath(dir string) (FFmpegStatus, error) // 手动指定，校验失败返回 INVALID_ARGUMENT
RecheckFFmpeg() (FFmpegStatus, error)
```

### 9.5 功能门控

- 依赖 ffmpeg 的：转换、剪辑、直播、媒体探测和缩略图。`state != ready` 时这些入口可以进，但操作按钮禁用，顶部显示提示条和"一键安装"按钮；后端对应 Service 统一返回 `FFMPEG_NOT_FOUND`，双保险。
- 不依赖 ffmpeg 的：Office 转 PDF、PDF 预览、JSON 工具，始终可用。
- 首次启动检测到 `missing` 时弹一次确认框（"安装"或"稍后"），选"稍后"后写入 `Settings.ffmpegPromptDismissed = true`，之后只保留提示条，不再弹窗；ffmpeg 变为 ready 后该标记重置。
- 前端不轮询：检测完成、安装进度导致的 state 变化、手动指定路径、重新检测，都会推送 `ffmpeg:status`，payload 为完整 `FFmpegStatus`。
