# 🎥 音视频格式转换工具

> 一个基于 Vue3 + TypeScript + Go 的跨平台桌面端音视频格式转换工具。

## 📌 简介

本项目实现了一个简单但功能齐全的音视频格式转换工具，支持多种主流音视频格式之间的相互转换。前端采用 Vue3 + TypeScript + Element Plus 构建，后端使用 Go + Gin 框架提供服务，通过 Wails 实现桌面应用打包，前后端通过 Axios 进行通信。

## ⚙️ 技术栈

- **前端**：Vue3 + TypeScript + Vite + Element Plus + Axios
- **后端**：Go + Gin
- **桌面端打包**：Wails
- **构建工具**：Vite + Go Modules
- **通信协议**：HTTP + JSON

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

## 🤝 贡献指南

欢迎提交 Issue 和 Pull Request！如果你有兴趣添加更多格式的支持，请 Fork 本仓库并提交你的修改。
