# FFmpegFree 项目分析报告

> 分析日期：2026-04-28 | 版本：基于 commit `4e6cb13`

---

## 1. 项目概览

**FFmpegFree** 是一个基于 **Wails v2 + Go + Vue3 + TypeScript** 的跨平台桌面端音视频处理工具，定位为 FFmpeg 的图形化外壳。后端通过 Gin 框架在 `localhost:19200` 启动 HTTP 服务，前端通过 Axios 调用后端 API，同时集成 WebSocket（推流入口）和 SSE（状态推送）。

---

## 2. 技术栈

| 层 | 技术 |
|---|---|
| 桌面框架 | Wails v2.11.0 |
| 后端语言 | Go 1.24 |
| Web 框架 | Gin v1.10.1 |
| 前端 | Vue3 + TypeScript + Vite + Element Plus |
| 通信协议 | HTTP REST + SSE + WebSocket |
| 核心依赖 | FFmpeg（内置在 `ffmpeg/` 目录）、LibreOffice（可选） |
| 其他依赖 | gorilla/websocket, goccy/go-json, go-pdf/fpdf, excelize |

---

## 3. 项目结构

```
FFmpegFree/
├── main.go                        # 入口，启动 Wails 应用 + Gin 路由
├── app.go                         # Wails App 结构体，绑定到前端
├── wails.json                     # Wails 项目配置
├── go.mod / go.sum                # Go 模块依赖
├── frontend/                      # Vue3 前端（dist/ 为构建产物，嵌入二进制）
├── ffmpeg/                        # 内置 FFmpeg 二进制（windows: ffmpeg.exe）
├── public/                        # 静态资源目录
│   ├── user/                      # 用户上传的源文件
│   ├── steam/                     # 推流用源文件
│   ├── converted/                 # 转换中的临时输出
│   ├── convertedUp/               # 转换完成的输出
│   ├── thumbs/                    # 视频缩略图
│   └── edit/                      # 视频编辑渲染输出
├── backend/
│   ├── router/
│   │   └── router.go              # 路由注册（约 40 个 API 端点）
│   ├── controllers/
│   │   ├── upload_controller.go   # 上传、格式转换、文件推流（核心模块）
│   │   ├── live_controller.go     # 增强版推流 / 转推 / 健康监控
│   │   ├── video_edit_controller.go # 视频剪辑引擎
│   │   ├── office_controller.go   # Office → PDF 转换
│   │   ├── pdf_controller.go      # PDF 管理 / 预览
│   │   ├── json_controller.go     # JSON 格式化 / 校验 / 比较
│   │   └── openclaw_controller.go # OpenClaw 第三方 API 接入
│   ├── live/
│   │   ├── global.go              # 全局 Manager 实例
│   │   ├── manager.go             # 直播状态管理器（内存态）
│   │   └── ffmpeg.go              # FFmpeg 进度解析、tee 输出构建
│   ├── sse/
│   │   └── sse.go                 # SSE 广播
│   ├── ws/
│   │   └── ws.go                  # WebSocket 推流入口
│   ├── vo/                        # 数据模型（VideoInfo, PDFInfo, OfficeInfo 等）
│   └── utils/
│       └── response.go            # 统一响应格式（Success / Fail）
```

---

## 4. 功能模块详解

### 4.1 音视频格式转换（`upload_controller.go`）

**支持格式：** `mp4`, `avi`, `mkv`, `mov`, `flv`, `gif`, `webm`

**四种预设：** `fast` / `balanced` / `quality` / `compact`

| 预设 | x264 参数 | 适用场景 |
|---|---|---|
| fast | preset=ultrafast, crf=28, tune=zerolatency | 速度优先 |
| balanced | preset=medium, crf=23 | 质量与速度平衡 |
| quality | preset=slow, crf=18 | 质量优先 |
| compact | preset=slow, crf=30 | 体积优先 |

**转换流程：**
1. 用户通过 `POST /api/upload` 上传文件到 `public/user/`
2. 通过 `POST /api/convert` 提交转换任务
3. FFmpeg 通过 `-progress pipe:1` 输出进度到 stdout
4. 后台 goroutine 解析 `out_time_ms` 计算百分比
5. 前端轮询 `GET /api/GetConvertingFiles` 获取进度
6. 转换完成后将文件从 `public/converted/` 移动到 `public/convertedUp/`

**GIF 特殊处理：** 双通道调色板模式 — 首先生成调色板（`palettegen`），再合成 GIF（`paletteuse`）。这一过程在主请求 goroutine 中同步执行，会阻塞 HTTP 响应。

### 4.2 流媒体模块（`live_controller.go` + `live/`）

项目中架构最好的部分，有三套并行的推流系统：

#### a) 旧版文件推流（`/api/steamload`）
- 简单直推：`ffmpeg -re -i input -c:v copy -c:a aac -f flv rtmp://...`
- 单目标，无归档
- `cmd.Start()` 的返回值被忽略（`live_controller.go:664`），存在僵尸记录风险

#### b) 增强版文件推流（`/api/live/stream/*`）
- 支持多目标 tee 输出 + 本地分段归档（mp4 segment）
- 编码参数：`libx264 veryfast zerolatency`, AAC 128k
- 完整生命周期管理：Start → MarkRunning → ConsumeProgress → MarkFinished

#### c) 直播转推（`/api/live/relay/*`）
- 拉取远程流并转推到多个目标
- 与文件推流共用同一套 Manager 监控与归档能力

**诊断面板（`/api/live/health`）：**
- 用 `Manager` 内存态管理所有流会话
- 解析 `ffmpeg -progress pipe:2` 获取实时指标
- 三级健康评级：healthy / warning / critical
- 阈值判定逻辑：

| 指标 | Warning | Critical |
|---|---|---|
| 延迟 | > 1800ms | > 3500ms |
| 编码速度 | < 0.95x | < 0.85x |
| 掉帧 | > 30 | > 120 |
| FPS | < 18 | — |

### 4.3 视频剪辑引擎（`video_edit_controller.go`）

最新模块（PR #3e01308），复杂度最高，实现了一个完整的非线性编辑器核心：

- **多轨道系统**：TrackID（V1/V2/A1/A2），按 TrackOrder 排序合成
- **视频片段操作**：裁剪（in/out）、变速、转场效果（8 种）、滤镜预设（4 种）、模糊
- **音频轨道**：裁剪、变速、音量、延迟
- **全局效果**：亮度 / 对比度 / 饱和度 / 锐化
- **时间线合成**：`-filter_complex` 在单个 FFmpeg 进程中完成全部合成
- **静音轨兜底**：未配置音轨时用 `anullsrc` 确保输出容器有标准音频流
- **渲染是同步阻塞的**：`CombinedOutput()` 同步等待，长渲染会挂起 HTTP 请求

### 4.4 Office 转 PDF

- 依赖外部 LibreOffice 命令行调用
- 查找路径优先级：项目目录 → 默认安装 → 32 位安装
- 异步转换，通过 `/api/getConvertedPDFiles` 查询结果

### 4.5 其他模块

- **PDF 预览/管理**：上传、列表、删除、下载已转换 PDF
- **JSON 工具**：格式化 / 校验 / 比较（三合一工具）
- **OpenClaw 一键安装**：第三方 API 接入配置（安装、鉴权检查、模型查询）

---

## 5. 架构评价

### 5.1 做得好的地方

1. **模块划分清晰**：`controllers/` 按功能拆分，`live/` 子包把流媒体逻辑单独抽离，职责明确
2. **流媒体监控系统设计合理**：Manager 的 Snapshot 模型统一了文件推流、转推、屏幕推流的状态表示，`applyHealth()` 的阈值判定逻辑清晰且可扩展
3. **视频编辑引擎完整性高**：从素材管理到时间线合成到渲染输出，覆盖了一个非线性编辑器的核心流程，转场、滤镜、音频混合都有完整实现
4. **进程生命周期管理**：`KillAllFFmpegProcesses()` 和 `KillLiveOpsProcesses()` 在应用退出时兜底清理，防止僵尸进程
5. **安全措施**：CORS 全局配置、1GB 上传限制、`filepath.Base` 路径校验（部分接口）

### 5.2 需要注意的问题

| 优先级 | 问题 | 位置 | 影响 |
|---|---|---|---|
| P0 | `cmd.Start()` 返回值被忽略 | `live_controller.go:664` | 启动失败时留下僵尸 map 记录 |
| P0 | 文件删除接口未做路径穿越防护 | `upload_controller.go` 多处 | 存在目录遍历风险 |
| P1 | GIF 转换同步阻塞 | `upload_controller.go:472-495` | 长时间阻塞 HTTP 响应 |
| P1 | 视频渲染同步阻塞 | `video_edit_controller.go:203` | 长时间阻塞 HTTP 响应 |
| P2 | 并发锁不一致 | `live_controller.go` + `live/manager.go` | 三层锁无统一加锁顺序 |
| P2 | FFmpeg 路径不统一 | 多处硬编码 `.exe` / 无后缀 | 跨平台兼容性差 |
| P2 | URL 硬编码 `localhost:19200` | 所有控制器 | 部署灵活性差 |
| P3 | 缺乏配置管理 | 全局 | 端口、路径、参数均无法外部配置 |

### 5.3 代码质量对比

```
upload_controller.go    ████████░░░░  (早期代码，重复多，锁粒度粗糙)
live_controller.go      ██████████░░  (架构合理，生命周期管理完善)
video_edit_controller.go ████████████  (最新代码，最复杂的模块)
live/manager.go         ███████████░  (设计良好，独立可测)
```

---

## 6. 改进建议

### 短期（1-2 周）

1. **修复 `live_controller.go:664` cmd.Start() 返回值被忽略的问题**
   - 添加错误检查和 TouchFailure 调用

2. **删除接口增加路径穿越防护**
   - 在 `DeleteUp`、`DeleteUpsc`、`DeletesteamVideo` 中增加 `filepath.Base` 校验

3. **统一 FFmpeg 路径获取**
   - 全部改用 `live.FFmpegBinaryPath()` 替代硬编码路径

### 中期（1-2 月）

4. **抽取配置管理**
   - 创建 `config.yaml` / `config.json`，管理端口、路径前缀、编码默认参数
   - 通过 Wails 的 `wails.json` 或自定义配置文件实现

5. **GIF 转换和视频渲染改为异步**
   - 返回 streamId，使用 SSE 推送进度，避免同步阻塞 HTTP 请求

6. **合并重复代码**
   - `sanitizeTargets` 在 `live_controller.go` 和 `live/manager.go` 中各出现一次
   - `sanitizeRelayTargets` 在 `live_controller.go` 中又重复实现

### 长期（3 月+）

7. **引入 context.Context 统一管理子进程生命周期**
   - 利用 `exec.CommandContext` 替代裸 `exec.Command`
   - 应用退出时通过 cancel 统一终止所有 FFmpeg 子进程

8. **增加单元测试**
   - `live/manager.go` 的 Manager 是纯逻辑，容易测试
   - `live/ffmpeg.go` 的 BuildTeeOutput 和 ConsumeProgress 也应测试

9. **考虑 WebSocket 推流的安全加固**
   - 增加 token 鉴权，限制连接速率，防止未授权推流

---

## 7. 部署与构建

### 开发环境

```bash
# 启动开发模式（需要安装 Wails CLI）
wails dev
```

### 生产构建

```bash
# 构建可执行文件
wails build
# 输出：build/bin/FFmpegFree.exe
```

### 前置条件

- Node.js >= 18.x
- Go >= 1.20
- Wails CLI
- FFmpeg（项目 `ffmpeg/` 目录已内置 Windows 版本）
- LibreOffice（可选，用于 Office 转 PDF）

---

## 8. API 汇总

| 分类 | 方法 | 路径 | 说明 |
|---|---|---|---|
| 上传 | POST | `/api/upload` | 上传用户文件 |
| 上传 | POST | `/api/uploadSteamup` | 上传推流文件 |
| 转换 | POST | `/api/convert` | 提交格式转换任务 |
| 转换 | GET | `/api/GetConvertingFiles` | 查询转换进度 |
| 转换 | POST | `/api/RemoveConvertingTask` | 取消转换 |
| 转换 | GET | `/api/convertup` | 列出已转换文件 |
| 转换 | GET | `/api/download` | 下载已转换文件 |
| 推流(旧) | POST | `/api/steamload` | 启动推流 |
| 推流(旧) | POST | `/api/StopStream` | 停止推流 |
| 推流(旧) | GET | `/api/GetStreamingFiles` | 查询推流列表 |
| 推流(新) | POST | `/api/live/stream/start` | 启动增强推流 |
| 推流(新) | POST | `/api/live/stream/stop` | 停止增强推流 |
| 推流(新) | GET | `/api/live/stream/list` | 列出增强推流 |
| 转推 | POST | `/api/live/relay/start` | 启动转推 |
| 转推 | POST | `/api/live/relay/stop` | 停止转推 |
| 转推 | GET | `/api/live/relay/list` | 列出转推 |
| 监控 | GET | `/api/live/health` | 健康面板 |
| 监控 | GET | `/api/live/archives` | 归档列表 |
| Office | POST | `/api/uploadOffice` | 上传 Office 文件 |
| Office | POST | `/api/convertOfficeToPDF` | Office 转 PDF |
| Office | GET | `/api/getOfficeFiles` | Office 文件列表 |
| Office | GET | `/api/getConvertedPDFiles` | 已转换 PDF 列表 |
| PDF | POST | `/api/uploadPDF` | 上传 PDF |
| PDF | GET | `/api/getPDFFiles` | PDF 文件列表 |
| PDF | POST | `/api/deletePDFFile` | 删除 PDF |
| JSON | POST | `/api/json/format` | JSON 格式化 |
| JSON | POST | `/api/json/compare` | JSON 比较 |
| JSON | POST | `/api/json/validate` | JSON 校验 |
| 编辑 | GET | `/api/edit/sources` | 编辑素材列表 |
| 编辑 | POST | `/api/edit/probe` | 探测素材元数据 |
| 编辑 | POST | `/api/edit/render` | 渲染视频 |
| 通信 | GET | `/ws` | WebSocket 推流入口 |
| 通信 | GET | `/api/sse` | SSE 事件流 |
