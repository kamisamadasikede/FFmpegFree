# 🎥 音视频格式转换工具

> 一个基于 Vue3 + TypeScript + Go 的跨平台桌面端音视频格式转换工具。

## 📌 简介

本项目实现了一个简单但功能齐全的音视频格式转换工具，支持多种主流音视频格式之间的相互转换。前端采用 Vue3 + TypeScript + Element Plus 构建，后端使用 Go + Gin 框架提供服务，通过 Wails 实现桌面应用打包，前后端通过 Axios 进行通信,流媒体工具实现屏幕获取拉流推流。同时支持 PDF 仿制 PPT 预览功能。

## ⚙️ 技术栈

- **前端**：Vue3 + TypeScript + Vite + Element Plus + Axios
- **后端**：Go + Gin
- **桌面端打包**：Wails
- **构建工具**：Vite + Go Modules
- **通信协议**：HTTP + JSON+SSE+WEBSOKET
- **必备工具**：FFmpeg(windows文件包已在ffmpeg目录下，构建时请复制到buildbin目录下或者执行copy-resources.ps1文件)

## 🔄 支持的格式（持续更新中）

当前已支持的音视频格式互转：

| 格式      | 类型    | 备注                          |
| ------- | ----- | --------------------------- |
| `.avi`  | 视频    | Audio Video Interleave      |
| `.mkv`  | 视频    | Matroska Video File         |
| `.mov`  | 视频    | QuickTime Movie             |
| `.flv`  | 视频    | Flash Video                 |
| `.mp4`  | 视频    | MPEG-4 Part 14              |
| `.gif`  | 视频/动画 | Graphics Interchange Format |
| `.webm` | 视频    | Web Media File              |

## 📦 安装与运行

### 前提条件

- Node.js >= 18.x
- Go >= 1.20
- Wails CLI 已安装（可通过 `go install github.com/wailsapp/wails/v2/cmd/wails@latest` 安装）

### 启动开发环境

```
# 在项目根目录下运行
wails dev
```

### 打包为桌面程序

bash

打包为桌面程序

```
# 在项目根目录下运行
wails build
```

生成的可执行文件会位于 `build/bin/` 目录下。

## 🧪 使用说明

1. 启动程序后，点击“选择文件”按钮上传需要转换的音视频文件。
2. 选择目标格式。
3. 点击“开始转换”，等待进度条完成即可下载或打开输出文件。

## 📁 项目结构

深色版本

```
project/
├── backend/          # Go + Gin 后端代码
│   └── main.go
├── frontend/         # Vue3 + TS 前端代码
│   ├── src/
│   └── ...
├── build/            # 构建输出目录
└── README.md
```

## 📡 流媒体工具模块

本模块为应用新增了强大的流媒体处理能力，支持文件推流、屏幕录制推流、直播拉流等多种音视频流操作，适用于直播、远程教学、会议分享等场景。

---

### 🔧 支持功能

#### 1. **文件推流上传**

- 用户可以选择本地音视频文件（支持主流格式），将其通过 RTMP、HLS、SRT 等协议推流到指定地址。
- 可配置目标流地址（如：`rtmp://live.example.com/stream`）。
- 支持断点续传与错误重试机制（视实现情况而定）。

#### 2. **查询当前推流任务**

- 实时展示当前正在进行的所有推流任务。
- 包括文件名、推流地址、状态（进行中/失败/完成）、进度条、开始时间等信息。
- 提供停止推流按钮，允许用户手动中断任务。

#### 3. **屏幕录制推流**

- 支持选择屏幕区域或全屏录制并实时推流。
- 可设置帧率、编码器参数等选项。
- 推流地址可自定义，便于接入第三方直播平台或私有流媒体服务器。

#### 4. **直播拉流播放（支持 FLV 格式）**

- 在应用内集成简易播放器，支持拉取并播放远程直播流。
- 完整支持 FLV 格式的实时播放，兼容 HTTP-FLV 和 WebSocket-FLV。
- 可选自动重连、缓冲控制、播放暂停等功能。

---

### 🖥️ 使用场景示例

| 场景      | 描述                             |
| ------- | ------------------------------ |
| 直播转码推流  | 将本地视频文件推送到抖音、B站、YouTube 等直播平台。 |
| 远程教学演示  | 屏幕录制 + 推流，将讲解过程实时传输至内部系统或直播服务。 |
| 监控中心查看  | 拉取多个摄像头的 FLV 流，集中显示在客户端界面。     |
| 私有流媒体测试 | 快速测试本地推流和拉流功能，调试流媒体服务器连接。      |

---

### ⚙️ 技术说明（简要）

- 推流功能基于 `ffmpeg` 命令行调用或原生 Go 音视频库实现。
- 屏幕录制使用操作系统 API（如 Windows GDI、macOS AVFoundation）捕获画面。
- FLV 拉流播放依赖于浏览器 `<video>` 标签配合 MSE（前端）或使用原生播放器组件（如通过 WebAssembly 或桌面端插件）。
- 所有流媒体操作均通过后端管理生命周期，并向前端提供状态更新接口。

---

## 项目截图：

![wechat_2025-07-03_163332_152.png](https://gitee.com/bmcbdt/FFmpegFree/raw/master/img/wechat_2025-07-03_163332_152.png)

![wechat_2025-07-03_163413_525.png](	https://gitee.com/bmcbdt/FFmpegFree/raw/master/img/wechat_2025-07-03_163413_525.png)

![wechat_2025-07-03_163434_577.png](https://gitee.com/bmcbdt/FFmpegFree/raw/master/img/wechat_2025-07-03_163434_577.png)

![wechat_2025-07-03_163442_201.png](	https://gitee.com/bmcbdt/FFmpegFree/raw/master/img/wechat_2025-07-03_163442_201.png)

## ### 🧩 后续计划（可选）
- 支持更多拉流格式（HLS、RTMP、RTSP 等）。
- 添加推流日志查看与性能监控面板。
- 支持多路并发推流与负载均衡。
- 提供简单的流媒体服务器搭建向导（如 Nginx-RTMP 一键配置）。
- 增加 office 文件转换 pdf 功能。

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！如果你有兴趣添加更多格式的支持，请 Fork 本仓库并提交你的修改。
