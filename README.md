# FFmpegFree

FFmpegFree 是一个基于 ffmpeg 的桌面工具，技术栈为 **Wails v2 + Vue 3 + Go**。目前提供格式转换、任务中心、JSON 工具等功能；视频剪辑、直播推流、Office 转 PDF、PDF 预览仍在开发合入中（见下文“功能与当前状态”）。

> **分支说明**：`master` 是 v1，保持不动；v2 的开发集成在 `v2` 分支，本 README 描述 v2。v1 的原始说明归档在 [docs/v1.md](docs/v1.md)。

## v2 相对 v1 的变化

- **去掉 OpenClaw**：相关后端、页面、接口和菜单项已删除。
- **去掉 Gin 与本地 HTTP 中间层（进行中）**：前端通过 Wails 直接绑定调用 Go 的 Service，不再走 Axios + 本地 HTTP。已迁移的功能（转换、JSON 工具、系统与 ffmpeg、任务、媒体探测）都已走 Wails 绑定；剪辑、直播、Office/PDF 的后端尚未合入，v2 上还保留着 v1 的 `backend/`（Gin 路由与控制器）给这些功能占位，随对应后端 PR 合入后再删除。
- **SQLite 存任务和设置**：任务记录、设置、最近文件等保存在用户数据目录下的 SQLite（`app.db`），应用退出或崩溃时未结束的任务在下次启动被标记为“已中断”。
- **ffmpeg 不再内置**：启动时后台检测；缺失时经用户确认后自动下载安装到数据目录，不修改系统 PATH；检测不到（或版本过旧）时，依赖 ffmpeg 的功能被禁用并给出提示。
- **统一的任务管理器与任务中心**：转换等耗时操作都作为任务提交，有排队、并发上限、进度、取消、重试和日志，界面里可在任务中心统一查看。

## 功能与当前状态

以下按 v2 分支上**实际已合入的代码**填写（截至本 README 提交时）。

| 功能 | 状态 | 说明 |
|---|---|---|
| 格式转换 | ✅ 已完成 | `ConvertService` + `MediaService`（探测、缩略图）已合入，转换页已接入真实后端 |
| 任务中心 | ✅ 已完成 | `TaskService` + `internal/task`，界面为“任务中心”页 |
| ffmpeg 检测与安装 | ✅ 已完成 | `SystemService`：检测、手动指定、下载安装、下载源切换 |
| 设置 | ✅ 已完成 | 输出位置、同时转换数量、ffmpeg 路径等 |
| JSON 工具 | ✅ 已完成 | `JsonService` |
| 视频剪辑 | ⏳ 待合入 | 后端 EditService 在审查中（契约 #22，实现 #30）；界面接口层已就绪，暂用演示数据，页面按钮当前禁用 |
| 直播推流 | ⏳ 待合入 | 后端 LiveService 在审查中（契约 #19，实现 #31）；界面接口层已就绪，页面暂用演示数据 |
| Office 转 PDF | ⏳ 待合入 | 后端 DocService 在审查中（契约 #23，实现 #29）；页面暂不可用 |
| PDF 预览 | ⏳ 待合入 | 同上（DocService） |

## 架构与目录

```
app/                 Wails 绑定层：按领域拆分的 Service（convert / media / system / task / json），
                     只做参数校验与转发。新增 Service 后在 main.go 的 Bind 列表注册
internal/
  service/           业务逻辑（convert、media、system、jsontool）
  task/              统一任务管理器（调度、并发、取消、重试、日志、事件）
  store/             SQLite 存储与迁移（任务、设置、媒体记录、预设）
  ffmpeg/            ffmpeg 定位与校验、下载安装、参数生成、进度解析
  proc/              子进程启动与回收（含 Windows Job Object）
  apperr/            统一错误码与错误类型
  paths/ id/         数据目录与 ID 工具
frontend/            Vue 3 + TypeScript + Vite + Element Plus + Pinia
  src/api/           前端接口封装层（见下）
  wailsjs/           Wails 生成的绑定（wails generate module）
docs/architecture/   接口契约
backend/ ffmpeg/     v1 遗留：Gin 后端与内置 ffmpeg（Windows），待清理
```

**前端接口层**位于 `frontend/src/api`，页面和 store 只调用这一层。其中有三个“后端就绪开关”（`frontend/src/api/flags.ts`）：

| 开关 | 对应后端 | 默认 |
|---|---|---|
| `LIVE_BACKEND_READY` | LiveService（直播） | `false`：走本地模拟 |
| `EDIT_BACKEND_READY` | EditService（剪辑） | `false`：走本地模拟 |
| `DOC_BACKEND_READY` | DocService（Office / PDF） | `false`：走本地模拟 |

对应后端合入并生成绑定后，把开关改成 `true` 即可联调；细节见 [frontend/src/api/README.md](frontend/src/api/README.md)。

## ffmpeg 检测顺序

启动时依次查找，第一个通过校验（ffmpeg 与 ffprobe 可运行、主版本不低于 6、含 libx264 与 aac）的为准：

1. 设置里手动指定的路径
2. `<数据目录>/bin`（自动安装的位置）
3. 系统 `PATH`
4. 程序同级的 `ffmpeg/` 目录（兼容 v1）

数据目录为系统用户配置目录下的 `FFmpegFree`（Windows `%AppData%\FFmpegFree`，macOS `~/Library/Application Support/FFmpegFree`，Linux `~/.config/FFmpegFree`）。详见契约第 9 节。

## 开发与构建

依赖：

- Go（`go.mod` 声明 1.24）
- Node.js（Vite 5，建议 18 及以上）与 npm
- [Wails CLI](https://wails.io/)：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`

常用命令：

```bash
# 开发（热重载）
wails dev

# 构建 Windows 版本
wails build -platform windows/amd64
```

### 注入版本号

“关于”页显示的版本号（绑定 `GetAppVersion`）在构建时用 `-ldflags` 写入 `internal/about.Version`；不注入（空串）则显示“开发版”。目前仓库里没有别处注入版本号，`wails.json`、`scripts/build.ps1` 也没有版本相关配置：

```bash
# wails 构建
wails build -platform windows/amd64 -ldflags "-X FFmpegFree/internal/about.Version=1.2.3"

# 纯 go 构建（需要先构建前端，见下方注意事项）
go build -ldflags "-X FFmpegFree/internal/about.Version=1.2.3" .
```

测试与检查：

```bash
go test ./...                 # Go 单元测试
cd frontend
npm run check:api             # 接口层自检
npm run check:json            # JSON 工具文本处理自检
npx vue-tsc --noEmit          # 类型检查
```

> **注意**：根包用 `//go:embed all:frontend/dist` 嵌入前端产物，`frontend/dist` 不在版本库里。直接 `go build ./...`、`go vet ./...` 或 `go test ./...` 之前，需要先构建前端：
> `cd frontend && npm ci && npx vite build`。`wails dev` / `wails build` 会自动构建前端。

## 接口契约文档

- v2 已合入的契约：[docs/architecture/contract.md](docs/architecture/contract.md)（数据模型、Service 方法、事件、SQLite 表、任务管理器、ffmpeg 检测与安装等）。
- Live / Edit / Doc 三份契约尚未合入 v2，合入后位于同一文件 `docs/architecture/contract.md` 中；在此之前见对应 PR：直播 #19（分支 `feat/live-contract`）、剪辑 #22（`feat/edit-contract`）、Office/PDF #23（`feat/doc-contract`）。

## 已知限制

- Windows / macOS 真机上尚未验证的项，见各契约的“真机试用清单”（如 Live 契约的“真机试用清单”一节，以及 v2 契约中标注“未验证”的部分）。目前主要在 Linux 上做了测试和交叉编译。
- 剪辑、直播、Office 转 PDF、PDF 预览暂不可用或仅为演示（见上表）。
- 转换暂不支持“按目标体积压缩”（两遍编码已暂缓，见契约 v0.7.2）。
- 仓库里仍保留 v1 遗留的 Gin 后端与 `ffmpeg/` 目录，待相关功能迁移完成后清理。

## 许可与第三方

- 本项目使用木兰宽松许可证第 2 版，见 [LICENSE](LICENSE)。
- **ffmpeg 不随安装包分发**：由用户机器上已有的 ffmpeg，或应用在用户确认后从清单中的下载源下载安装（清单见 `internal/ffmpeg/manifest.json`）。ffmpeg 自身的许可证与使用条款请以其官方说明为准。
- DocService 计划内嵌 Noto Sans SC 子集字体，遵循 SIL OFL；该字体尚未合入 v2，合入后见 `fonts/README.md` 与 `OFL.txt`（位置以合入后的实际路径为准）。

## 贡献

欢迎提交 Issue 和 Pull Request。v2 相关的 PR 请以 `v2` 分支为目标。
