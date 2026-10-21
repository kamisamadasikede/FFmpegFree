# 前端视觉改造 v0.58（对齐 `prototype/` 原型）

范围：**除 Agent（Cat）外的全部页面**。Cat 页视觉已冻结，与本轮无关（见文末验证）。

## 一、改了什么

### 1. 设计系统（全局）

| 文件 | 变化 |
|---|---|
| `styles/tokens.css` | 重做配色与尺度：三层背景 `bg-app / bg-panel / bg-surface`（+`bg-card-2`）、`border`/`line`/`line-strong` 三档描边、圆角 6/8/12/16、新增 `--ff-panel-w`/`--ff-page-pad-x|y`；命名仍是 `--ff-*`，避免全站大改 |
| `styles/base.css` | 新增通用件 `.ff-card-head` `.ff-pill` `.ff-bar` `.ff-note` `.ff-empty` `.ff-stack`；细滚动条悬停显形；三档响应式变量 |
| `styles/element-override.css` | 输入框内凹描边、表头小字化、tag 药丸化、主/次按钮对齐原型、主按钮禁用态不再"半透明假可点" |
| `styles/settings-panels.css` | 设置/关于面板：14px/620 面板头、20px 行内边距、药丸状态标、32px 按钮、等宽路径框 |

### 2. 外壳

- `AppSidebar.vue`：分组标题（主功能 / 工具与系统）、选中态左 3px 短条、导航区独立滚动、引擎状态块独立于底部；**窄屏自动收成图标轨**
- `AppTitlebar.vue`：17px 标题 + 副标题；窄屏隐藏副标题
- `SidebarItem.vue`：选中/置灰/角标对齐原型

### 3. 页面

| 页面 | 改动 |
|---|---|
| 转换 | 继承新 token；统计条、失败条、右栏分组随新配色统一 |
| 任务中心 | 统计卡数字 20→26px 紧排、卡面描边、窄屏 4→2→1 列 |
| 文档 / 直播 | 页签从深底段控改为卡片式（选中卡片面 + 1px 描边）；PDF 预览、推流/录屏/拉流三页同步 |
| JSON | 工具条独立成条带底部分隔线；面板头 620 字重；代码/树形段控加描边 |
| 设置 / 关于 | 子导航软底+主色文字；面板头、行、标签、按钮、路径框全部按原型规格 |

### 4. 媒体预览按格式自适应（重点）

新增 `utils/mediaKind.ts` 统一判定，把原来只有 `video / audio / gif` 三类扩到 **7 类**：
`video` `audio` `image` `gif` `pdf` `text` `other`。

- **图片** → `<img>` 直接显示（改造前会被判成"无法在应用内播放"）
- **PDF** → `<iframe>` 内嵌预览
- **文本** → `<pre>` 读取展示，超 512 KB 截断并提示
- 控制条按类型裁剪：图片/PDF/文本不再出现无意义的播放键与音量条

判定顺序：MIME 优先 → 扩展名 → 探测结果（`audioOnly`/`hasVideo`）→ 兜底 video。

## 二、验证

| 项 | 结果 |
|---|---|
| `vue-tsc --noEmit` | ✅ 0 error |
| `vite build` | ✅ 成功（21s） |
| `npm run check:api` | ✅ **0 失败**（改造前 7 条，本次同时修正了 G4/X5 两条随实现变化的断言） |
| `npm run check:i18n` | ✅ 1720 keys 通过 |
| `npm run check:json` | ✅ 通过 |
| `check:ffmpeg` | ⚠️ 70 条，**与改造前的基线完全相同**（全部落在 Cat 相关文件，属既有问题） |
| Cat 页视觉 | ✅ 逐像素比对：`max channel diff = 2`，超阈值像素 **0** |
| 窄屏 1024 / 753 | ✅ 侧栏收图标轨，正文不塌 |

截图见同目录 `overview.png`（11 屏拼图）与 `01`–`11`、`narrow-*.png`。

## 四、自适应（本轮补做，11 页全部覆盖）

第一轮只统一了视觉（token / 外壳 / 卡片规格），页面的**版式**还是固定列宽：窗口一窄，右栏照旧占 300 多像素，正文被压到 300 出头，失败原因竖排逐字换行、操作按钮直接被裁。本轮把三档断点真正落到每一页。

### 4.1 根因（两个，都在外壳）

| 问题 | 位置 | 后果 |
|---|---|---|
| Wails 窗口 `MinWidth: 1024` | `main.go` | 窗口根本拖不窄，CSS 里写的 1024 / 720 断点永远跑不到 |
| `.app-shell { min-width: 1024px }` | `App.vue` | 即使窗口变窄，壳仍撑 1024，整页横向溢出被裁 |

两处下限都降到 720（`min-height` 560），并把 `.app-content` 的写死内边距 `20/24/24` 改为走 `--ff-page-pad-x/y`；`.app-shell` 自身的 `min-width` 归零（窗口下限由 `main.go` 保证），否则 720 窗口扣掉滚动条后壳仍撑 720 会溢出。

### 4.2 三档断点

| 断点 | 变化 |
|---|---|
| ≤1240 | 侧栏 236→212；页边距 24→20；各页右栏 -24~-36 |
| ≤1024 | 侧栏 64px 图标轨；顶栏隐藏副标题；**所有两栏收单列**，右栏取消吸顶、整宽落到正文下方 |
| ≤720 | 侧栏 56px；页边距 12；卡头折行；按钮/字号/内边距收紧 |

另在 900 加了一档「单列真正生效」的阈值（1024 收窄、720 紧凑，单列放在 900 是因为 1024 宽时两栏还勉强能读，900 以下才必须堆叠）。JSON 的两个编辑器用同一阈值改上下两栏。

### 4.3 逐页

| 页面 | 文件 | 窄窗口行为 |
|---|---|---|
| 转换 | `convert-v2.css` | ≤900 单列，记录列表限高自滚、右栏整宽；≤720 栏头折行、行骨架去勾选框、缩略图 52×30 |
| 语音 | `components/voice/voice.css` | ≤900 单列（设置面板落到字幕时间轴下面）；≤720 分段行改「编号+时间码 / 文本」两行 |
| 文档转换 | 复用 `convert-v2.css` | 同转换；空态卡在单列下按内容撑高（原 `flex:1` 会塌成 0，随后被右栏盖住） |
| PDF 预览 | `views/PDFPreview.vue` + `views/docs/DocsLayout.vue` | 工具栏折行、文件名独占一行；104px 缩略图轨 → 阅读区上方 92px 横向滑动条；「最近打开」右栏 ≤900 落到下面横铺；无 PDF 时缩略图轨不留空白 |
| 文件推流 / 录屏推流 / 拉流 | `LiveTabFrame.vue`、`LivePanel.vue`、`LiveLayout.vue` | ≤900 预览在上、设置面板整宽在下；预览给 `clamp(220px,42vh,420px)` 高度（预览与播放器都是 `flex:1;min-height:0`，列容器若也 `flex:none` 会把它塌成 0）；指标卡 4→2 列；顶部提示条允许换行 |
| JSON | `views/JsonTools.vue` | ≤900 上下两栏各占一半；工具条折行（原来「去转义 / 复制」被裁掉） |
| 任务中心 | `views/TaskCenter.vue` | 筛选条折行；≤900 表格最小宽 660 + 横向滚动，保住任务名与列语义 |
| 设置 | `settings/SettingsLayout.vue`、`settings-panels.css` | ≤900 176px 竖排子导航 → 顶部横向可滚胶囊；设置行折行、路径框整宽 |
| 关于 | 复用 `settings-panels.css` | 品牌块、许可行随设置行规则折行 |

### 4.4 自检

新增 `npm run check:responsive`（`scripts/check-responsive.mjs`）：把探针注入 dist 的临时副本，用 Chrome headless 在 **720 / 760 / 900 / 1024 / 1280** 五个宽度逐个打开 11 个页面，扫主内容区里「右边界超出视口且自身不能横向滚动」的元素。

结果：**55 组全部 OK，0 处横向溢出**。截图见 `.shots/responsive/`（每页 720 / 1024 / 1560 三档）。

## 五、Cat 页面为什么没变

新 token 通过 `--ff-*` 会波及 Cat。为不动已定稿的原型，在 `styles/cat-tokens.css` 末尾追加了 `.cat-root` 作用域变量冻结块，把 Cat 依赖的 `--ff-*`（及其派生的 `--cat-*`）按改造前的值钉死，深色主题同步一份。要统一 Cat 视觉时删掉该块即可。

本轮改动后再验一次：Cat 页与基线**逐像素完全一致**（`max channel diff = 0`）。

## 六、验证汇总

| 项 | 结果 |
|---|---|
| `go build ./...` | ✅ |
| `vue-tsc --noEmit` | ✅ 0 error |
| `vite build` | ✅ |
| `npm run check:responsive` | ✅ 55/55 无横向溢出 |
| `npm run check:api`（含 i18n 1720 keys） | ✅ |
| `npm run check:json` | ✅ |
| `npm run check:ffmpeg` | ⚠️ 69 条，全部落在 Cat 页（`catDevSim` / `catState` / `CatCLIUpdateNotice` 等），属既有问题，与本次无关 |
| Cat 页像素 | ✅ `max diff = 0` |

## 七、遗留（未擅自处理）

1. `check:ffmpeg` 的 69 条违规（规则禁止 Cat 页出现 `Claude/Codex/CLI` 等词，与当前产品现状冲突），需产品确认后调整规则或文案。
2. 工作区里存在**非本次改造**的未提交改动（`App.vue` 关窗收尾、`internal/service/cat/*` 群聊协调等），本次未触碰。
