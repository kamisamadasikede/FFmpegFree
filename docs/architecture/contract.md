# FFmpegFree v2 接口契约（v0.31.2）

v0.31.2 变更（**Cat 项目口径澄清**，后端 #181 实现后架构师回写；完整规则见 **6.19.10**，冲突时以该节为准；**没有新错误码 / 迁移 / 事件 / 开关，2.1 仍是 39 个**）：① **排序最后一级按 id 倒序**：项目、对话（含无项目「对话」下）活动时间 / `createdAt` 都相同时，再按 `id` **倒序**（6.19.10.6）。② **`path_key` 大小写规则与 `paths.Normalize` 对齐**：**Windows 与 macOS 不区分大小写（键转小写），Linux 区分**；`CreateCatProject` / `RelocateCatProject` 共用。**后端 #181 目前只在 Windows 转小写，macOS 尚未对齐，需跟进修；修完后真机验：同一文件夹用不同大小写选两次 → `existed=true`**（6.19.10.2 第 2 条、6.19.10.10）。③ **`DeleteCatProject` / `DeleteCatConversation`**：有进行中的一轮时**先按 `CancelCatTurn` 取消，最多等 5 秒**等它停再删，避免回复尾巴写回库（6.19.10.2 第 4、7 条）。④ **无项目对话的读文件请求**：只在适配器内失败，说明 `这个对话没有项目文件夹，不能查看文件。`，**不进界面**，也不再用旧的「下一期开放」那句（6.19.10.5）。⑤ **说明（不绑定前端）**：后端内部 `system.Manager.OpenFolder` 被 `OpenStorageFolder` 与 `RevealCatProject` 复用，**不**暴露给前端。

v0.31.1 变更（**Cat 项目：重新选择文件夹 + 在文件管理器中显示**，产品 / 设计 10-09 定，架构师定方案；完整规则见 **6.19.10.2 第 8、9 条**，冲突时以该节为准）：① CatService 新增 **`RelocateCatProject({id, path}) → CatProject`**：换项目文件夹，**对话和消息原样保留**、名字不变；`path` 校验同 `CreateCatProject`（`reason=project_path` / `project_root`）；`path_key` 与当前相同 → 不改任何东西，返回该项目；属于**另一个**项目 → `INVALID_ARGUMENT` `reason=project_duplicate`（第二行 `projectId=<已有项目 id>`），界面提示 `这个文件夹已经建过项目了。` 并可切过去；未知 id → `NOT_FOUND`（`找不到这个项目。`）；该项目下有进行中的一轮 → `TASK_CONFLICT` `reason=turn_running`（不自动取消）。**`missing` 的项目可以重新选择**（主要用途），成功后实时重算 `missing`，变化时发 `cat:project`。选文件夹复用 `SystemService.PickDirectory("选择项目文件夹")`。**取消 v0.31 6.19.10.4 第 4 条“没有重新定位文件夹功能”**。② CatService 新增 **`RevealCatProject({id}) error`**：在系统文件管理器里打开项目文件夹本身（平台命令同 `OpenStorageFolder` / 6.8 的“path 是文件夹”分支）；只收 id、路径从表里取、不进 `RevealInFolder` 放行表；文件夹不在 → `CAT_PROJECT_MISSING`（`项目文件夹不见了。`）；**绝不写任何东西**（不建目录）。③ 名字上限维持 **60 个字符**（v0.31 已是 60，与设计稿 v0.2 一致，无改动）。④ 菜单文案按平台（非规范，前端按运行平台定）：Windows `在资源管理器中显示`、macOS `在访达中显示`、Linux `在文件管理器中显示`。⑤ **没有新错误码，2.1 仍是 39 个**；2.2 追加取值 `INVALID_ARGUMENT` `reason=project_duplicate`、`TASK_CONFLICT` `reason=turn_running`；`CAT_PROJECT_MISSING` 场景加 `RevealCatProject`。没有迁移、没有新事件、没有新开关。真机待验补两项（6.19.10.10 第 7、8 条）。

v0.31 变更（**Cat 助手 · 项目**，产品规则 / 架构 10-09 定；完整规则见新增 **6.19.10**）：① 项目 = 用户选的一个本机文件夹，名字默认文件夹名、可改名、允许重名；新类型 `CatProject{id,name,path,createdAt,updatedAt,missing}`。② CatService 新增 `ListCatProjects`、`CreateCatProject({path,name?}) → {project, existed}`（绝对路径、已存在文件夹、拒绝盘符 / 文件系统根；同一路径（Clean 后比较，Windows 不区分大小写）已建过 → 返回已有项目 `existed=true`、不报错不新建，前端提示「这个文件夹已经建过项目了。」并切过去）、`RenameCatProject({id,name})`、`DeleteCatProject({id})`（未知 id → nil；先取消其下进行中的一轮再在**显式事务**里删记录和对话，**绝不动文件夹里的文件**）；选文件夹复用 `SystemService.PickDirectory`。③ `CatConversation` 新增 `projectId`（可空），**创建时写入、之后不可变**；`CreateCatConversation` 新增可选 `projectId`；**一期没有「移动对话到项目」**，对话只能在项目下新建获得项目。v0.30 的 `projectPath` 字段作废（后端忽略）。④ `missing` 由后端 stat 实时计算（`ListCatProjects`、每次 `SendCatMessage` / `CreateCatConversation`；网络盘 2 秒超时按缺）；变化时发新事件 **`cat:project` `{id, missing}`**；前端窗口获得焦点时刷新列表；文件夹回来自动恢复；不做文件监视。⑤ 项目缺失时 `SendCatMessage` / `CreateCatConversation` 同步 **`CAT_PROJECT_MISSING`**（「项目文件夹不见了。」），不启 turn、不存用户消息。⑥ 项目路径是只读工具的沙箱根：相对路径解析，`..`、绝对路径、符号链接 / junction 逃逸一律拒绝；不属于项目的对话没有项目工具；写 / 执行仍拒绝。⑦ 排序：项目按最近对话活动倒序（无对话用 createdAt），同值按 createdAt；对话按 updatedAt。⑧ **迁移 `0011_cat_projects.sql`**（`0010` 已被 Cat 用掉）：新表 `cat_projects`（`path_key UNIQUE`）+ `cat_conversations.project_id`（可空、不加外键）。⑨ **2.1 由 38 变为 39**（+`CAT_PROJECT_MISSING`）；2.2 `INVALID_ARGUMENT` 新增 `reason=project_path` / `project_root` / `project_name`。⑩ 前端开关不变（`CAT_UI_ENABLED` 仍默认 false）；空状态「还没有项目。」。真机待验：中文 / 空格路径、网络盘、OneDrive 占位文件夹（6.19.10.10）。

v0.30.2 变更（Cat 流式回写对齐后端 #174，架构 10-09；见 **6.19.9**）：① `SendCatMessageResult` 必带 `turnId`；流式下 `assistantMessage` 为空/省略，助手内容只经 `cat:message` / `cat:turn`；同步仍带 `userMessage`；未就绪同步 `CAT_NOT_READY`（不启 turn）。② `CancelCatTurn({convId,turnId})`：`convId` 空 → `INVALID_ARGUMENT`；`turnId` 空取消该会话当前进行中一轮（兼容）；turnId 对不上 / 无进行中 / 重复取消 → nil；取消保留已出文字、无气泡后缀，先 `done`（若已有字）再一次性 `cancelled`，丢弃迟到增量。③ **用户消息不发 `cat:message`**，只助手流式用该事件。④ `seq` 从 **1** 起严格递增；`cat:turn.status` 仅 `running|completed|cancelled|failed`（无 succeeded/canceled）。⑤ **未发布/未就绪**：Send 同步 `CAT_NOT_READY`，界面仅「Cat 助手还没准备好，发布后即可使用。」，官方包不出现助手气泡、不拆字模拟回复；**24 字 append 拆分**仅作组件已就绪但 CLI 真流式未接时的临时适配器回退（切真回复，非假内容），真流式落地后只留测试。无新错误码、无迁移。

v0.30.1 变更（Cat 流式事件形态，产品 / 架构 10-09 定；**下一个包实现，不是二期**；CLI 流式协议细节待老板，见 **6.19.9**）：`cat:message` 增量（`convId`/`turnId`/`messageId`/`seq`/`op: append|replace|done`/`block?`/`textDelta?`）；`cat:turn`（`running|cancelled|failed|completed`）；`CancelCatTurn({ convId, turnId })` 立即停止、保留已出文字、界面加淡色系统行「已停止生成。」、停止按钮立即置灰；失败仍 `CAT_REPLY_FAILED`「回复没生成出来，请重试。」；reduced-motion 时前端直接追加文字、不做打字机闪烁。无新错误码、无迁移。

v0.30 变更（**Cat 助手 · 一期仅 Cat Build**，老板 10-09 定，2026-10-09；完整规则见新增 **6.19**）：① 侧栏「Cat」受 `CAT_UI_ENABLED` 控制（**默认 false**）；`CAT_BACKEND_READY` 控制是否走真绑定。② 欢迎页模式胶囊**只启用「Cat Build」**，其余禁用并提示「下一期开放」；**不**跑 Cat CLI / Cat Code / Codex CLI。③ **会话 `agentKind` 创建后锁定**（一期 `cat_build`）；Send 只走该会话适配器；应用默认 CLI **仅影响新建会话**；适配器注册表一期只注册 `cat_build`。④ Build 适配器调用老板后续发布的 CLI（入口名 / URL / SHA **TBD**）；界面只称「Cat 助手」。⑤ 无 URL/Key；未就绪「Cat 助手还没准备好，发布后即可使用。」；失败「回复没生成出来，请重试。」。⑥ 模型 / 思考强度仅来自对应 CLI；工具只读；「完全访问」置灰；无助手 / 定时任务。⑦ List/Get/Create/Delete、`SendCatMessage`、`cat:status`/`cat:message`/`cat:turn`。⑧ **2.1 由 36 变为 38**（+`CAT_NOT_READY`、`CAT_REPLY_FAILED`）。

v0.29.1 变更（产品定稿文案，2026-10-09）：`LANG_ASR_FAILED` 的面向用户文案由 `字幕识别失败，请重试。` 改为 **`识别没完成，请稍后重试。`**（6.18.8）。无接口、无新错误码、无迁移。

v0.29 变更（**语音工具 · 一期仅转字幕**，产品 / 架构 10-09 定，2026-10-09；完整规则见新增 **6.18**）：① 侧栏「语音工具」，页内**只出现「转字幕」**；任务类型 `speech_to_subtitle`，任务中心文案「转字幕」。② **语音识别组件**独立下载（默认档 **标准**约 400 MB，设置可换 **高清**约 1–1.5 GB；切换不自动下载）；状态机对齐文档组件。③ 抽音频：转换组件产出临时 **16 kHz mono WAV**（界面不展示）；ASR 池并发 **1**；高清 ASR 运行时硬件编码（NVENC/AMF/QSV）并发 **≤ 1**。④ **适配器协议**（本仓不实现 Python CLI）：组件根入口 `asr`/`asr.exe`，`--request`/`--response` JSON（输入 audioPath/language/tier，输出 cues）；URL/SHA-256 **TBD** 至老板发布包。⑤ 成功返回可编辑 cues；`ExportSubtitleCues` 硬校验（endMs>startMs、不重叠、单条≤80 字）；空识别 `LANG_ASR_EMPTY` 文案「这段音频里没有识别到有效内容。」不可重试。⑥ **2.1 由 31 变为 36**（+`LANG_ASR_NOT_READY`/`LANG_ASR_EMPTY`/`LANG_ASR_FAILED`/`LANG_DOWNLOAD_FAILED`/`LANG_CHECKSUM_FAILED`）；无迁移。⑦ 文字转语音 / 字幕进视频 / 音色克隆：**设计已定、实现延后**（6.18.9），无 UI；音色克隆组件体积占位约 1.5 GB。⑧ `OpenStorageFolder` 增加 `lang_asr`；设置键 `asrTier`。

.29 变更（**语音工具 · 一期仅转字幕**，产品 / 架构 10-09 定，2026-10-09；完整规则见新增 **6.18**）：① 侧栏「语音工具」，页内**只出现「转字幕」**；任务类型 `speech_to_subtitle`，任务中心文案「转字幕」。② **语音识别组件**独立下载（默认档 **标准**约 400 MB，设置可换 **高清**约 1–1.5 GB；切换不自动下载）；状态机对齐文档组件。③ 抽音频：转换组件产出临时 **16 kHz mono WAV**（界面不展示）；ASR 池并发 **1**；高清 ASR 运行时硬件编码（NVENC/AMF/QSV）并发 **≤ 1**。④ **适配器协议**（本仓不实现 Python CLI）：组件根入口 `asr`/`asr.exe`，`--request`/`--response` JSON（输入 audioPath/language/tier，输出 cues）；URL/SHA-256 **TBD** 至老板发布包。⑤ 成功返回可编辑 cues；`ExportSubtitleCues` 硬校验（endMs>startMs、不重叠、单条≤80 字）；空识别 `LANG_ASR_EMPTY` 文案「这段音频里没有识别到有效内容。」不可重试。⑥ **2.1 由 31 变为 36**（+`LANG_ASR_NOT_READY`/`LANG_ASR_EMPTY`/`LANG_ASR_FAILED`/`LANG_DOWNLOAD_FAILED`/`LANG_CHECKSUM_FAILED`）；无迁移。⑦ 文字转语音 / 字幕进视频 / 音色克隆：**设计已定、实现延后**（6.18.9），无 UI；音色克隆组件体积占位约 1.5 GB。⑧ `OpenStorageFolder` 增加 `lang_asr`；设置键 `asrTier`。

v0.28.3 变更（按前端 #144 / #145、后端 #146 实际合入记录，2026-10-09；见 **6.12.68**）：① `GetFormatMatrix` 在文档组件 `checking` 时**不再等最多 6 秒**，立即按当前已知状态返回：已检测到的 Office / WPS 照常可用，只靠组件的目标按未就绪返回（`componentReady` 可能为 `false`、部分格式置灰）。② 提交、重试、重转在“目标暂时不可用且组件还在检测”时仍**最多等 6 秒**（整个调用只等一次），不把“检测中”误当成没有组件。③ `GetDocComponentStatus` 检测中立即返回 `state=checking`（有 Office 时可能是 `state=ready` + `componentState=checking`）；检测结束（ready / 未安装 / 版本太旧 / 失败）**至少发一次 `doc:component`**，payload 与 `GetDocComponentStatus` 相同。④ 前端：每次收到 `doc:component` 都重拉格式表并丢弃过期的响应；检测中不显示“不可用”闪烁、不出下载入口；检测结束后 0.2 秒淡入刷新；初始化的状态 / 格式表 / 源列表三个请求并行。⑤ 后续项（未做）：组件冷启动仍要跑 `--version` 和一次冒烟转换，没有按版本缓存来跳过。**没有新增错误码（仍 31 个），没有迁移，没有新事件，接口签名不变。**


v0.28.2 变更（按前端 #141、后端 #142 实际合入记录，2026-10-09；见 **6.12.67**）：① 新 `hintKey` **`pdf_text`**，用于 pdf → txt 和 pdf → md，文案 `只提取文字，不保留排版和图片。`（产品 10-09 定）；`md_lossy` 只用于**非 PDF 源** → md；pdf → html 的提示不变（没有组件时 `simple_mode`，PDF 源文案同上）。`hintKey` 仍是字符串，**绑定不变**。取代 6.12.66 ③ 里记的 pdf → md 提示前后端不一致。② 确认：pdf → html 组件导出失败时回退纯 Go 简易 html（`simple_fallback`，`engine=go`），用户取消、加密、文件损坏、磁盘满除外（直接报错）。**没有新增错误码（仍 31 个），没有迁移，没有新事件。**


v0.28.1 变更（**按 #138 / #139 实际合入的内容记录，以实现为准**，2026-10-09；完整内容见新增的 **6.12.66**，本条只是索引）：① `DOC_ENCRYPTED` 新增 `reason=owner_only`：空用户密码能打开、但设了所有者 / 权限密码的 PDF，RC4 和 AES-128 的照常转，**AES-256 的在添加时拒绝**，不可重试，文案 `这个 PDF 设置了权限保护，暂时不能转换。`；要用户密码的不变（原 reason 和原文案）。后续项（未做）：有组件时把 AES-256 只有所有者密码的 PDF 交给组件。② 已接受的偏差：纯 Go 提取失败改用组件时，先导出 html 再取文字；pdf → html 组件导出失败回退简易 html；PDF 的转换结果可以预览。③ 前端：`DOC_V28_BACKEND_READY=true`，`KNOWN_CODE_RE` 含 `DOC_PDF_NO_TEXT`，横条 `Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转，PDF 可以提取文字转成 TXT、md 和简易网页。`；状态的 `engines` 不含 `go`，格式表的 `DocTarget.engines` 保留 `go`。④ 5 条 PDF 文案定稿（按前端标注的“产品 10-09 定稿”常量，与后端一致，见 6.12.66 ④）。**没有新增错误码（仍 31 个），没有迁移，没有新事件，没有接口签名变化**。


v0.28 变更（**PDF 作为输入转成其他格式**，用户提出，架构师定方案，文案待产品定，2026-10-09；**只有契约**；完整规则见新增的 **6.12.58~6.12.65**，本条只是索引，冲突时以新节为准）：① **取消“PDF 暂时不能转成其他格式。”**：`.pdf` 可以添加，新家族 `pdf`；目标 doc / docx / odt / rtf（Word ≥ 2013 → 文档组件 `writer_pdf_import`，不用 WPS，`hintKey=pdf_layout`）、txt / md（纯 Go 提取文字，`simple=true`，不需要组件、不占池）、html（有组件用组件，没有用纯 Go 生成简易 html）；表格、演示、图片不列出。② **纯 Go 库** `github.com/ledongthuc/pdf`（BSD-3-Clause）；提取后做**质量判定**（空，或 `U+FFFD` / 私用区 / 控制字符占非空白字符 ≥ 30%），不过时有可用组件就**自动改用组件**（`result.engine=component`），没有才报新码 **`DOC_PDF_NO_TEXT`**（`detail` 写 `quality=empty|garbled`）；Word 只用于 doc / docx / odt / rtf。txt 输出 UTF-8 不带 BOM、换行跟平台。③ **上限**：PDF 200 MiB（`INVALID_ARGUMENT` `reason=too_large`）、500 页（`reason=too_many_pages`，2.2 新增取值）；加密（要用户密码）`DOC_ENCRYPTED` 沿用现有文案，只有所有者密码的照常转。④ **2.1 由 30 个变为 31 个**（加 `DOC_PDF_NO_TEXT`；`DOC_PDF_INPUT_UNSUPPORTED` 保留不再产生）；任务类型仍 `doc_convert`，**没有迁移**，没有新事件、没有新接口；PDF 预览不变。⑤ **定稿**：另存为的目标正好是这条记录自己的源文件 / 输出时按覆盖处理并刷新记录，正在转换仍 `TASK_CONFLICT` `converting`，别的记录的文件 `IO_ERROR` `in_use`（6.12.42、6.12.49 改在原处）。⑥ **实现差异以实现为准**：Office / WPS 用一把全局锁保证并发 1（不单开池）；Office / WPS 进程暂不放 Job Object（后续加固）；组件配置 Writer `Link=0`、Calc `Link=1`（6.12.65）。⑦ 6.12.45 文案定稿：组件太旧 `文档组件版本太旧，请重新下载。` + 「更新文档组件」；「重新打开」二次确认 `重新打开会丢掉你改的内容，确定吗？`「重新打开」（危险）「取消」。


v0.27.2 变更（**docx 在预览窗口里编辑后保存**，老板定范围，架构师定方案，文案产品已定，2026-10-09；**只有契约**；完整规则见新增的 **6.12.47~6.12.57**，本条只是索引，冲突时以新节为准）：① **范围**：只放开 docx；doc / xls / xlsx / ppt / pptx / pdf 仍只能查看（`editBlock=format`，悬停 `这类文件请用默认程序打开编辑`）；前端用什么编辑器由前端定，契约只管读写。② **读取**：`DocPreview` 新增 `rawUrl`（docx 可编辑时原文件字节的 `/local/<token>` 地址；有引擎时 `kind=pdf` 只用来看，编辑用 `rawUrl`）；`editBlock`：> 20 MiB `too_large`、加密或打不开 `malformed`、原文件不在 `missing`（只能另存为）、正在转换 `converting`；`revision` 是整个文件的 SHA-256。③ **分段保存**（DocService 新方法）：`BeginDocBinarySave({sourceId|taskId, mode: overwrite|save_as, targetPath?, revision?, totalBytes})` → `{saveId, maxChunkBytes: 4194304, expiresAt}`；`AppendDocBinaryChunk({saveId, seq, data(base64)})` → `{receivedBytes}`；`CommitDocBinarySave({saveId, sha256})` → `{path, revision, sizeBytes, savedAt, backupPath?}`；`AbortDocBinarySave({saveId})`（幂等）。暂存在 `<数据目录>/tmp/docsave/<saveId>`，Begin 后 10 分钟没 Commit 自动清理，启动时清残留；同一文件同时只允许一个会话（`TASK_CONFLICT` `reason=saving`）；Commit 无论成败都结束会话。④ **完整性检查**：zip 能打开、CRC 全过、条目 ≤ 10 000、解压总量 ≤ 200 MiB、有 `[Content_Types].xml` / `_rels/.rels` / `word/document.xml` 且后者是格式良好的 XML，不过 `reason=malformed`；含 `vbaProject.bin` `reason=format`；原文件不动。⑤ **备份**（只有 overwrite）：替换前复制成同目录 `原名.bak-YYYYMMDD-HHMMSS.docx`（同秒加 `-2`），只删本应用按这个精确格式生成的、同一原名的备份，保留最近 3 份；备份失败不保存；备份不进文档列表。⑥ **2.2 新增取值**：`INVALID_ARGUMENT` 的 `chunk_order`、`checksum`、`malformed`，`NOT_FOUND` 的 `save_session`，`TASK_CONFLICT` 的 `saving`（`too_large`、`format`、`file_changed`、`converting` 等沿用）；**没有新增错误码**（2.1 仍是 30 个），没有迁移，没有新事件。⑦ **文案定稿**写进 6.12.45（原文件不在、编辑过的结果重转确认、保存 / 覆盖保存 / 另存为成功、没有权限、内容太多、docx 不完整），所有出错都保留用户编辑的内容；`checksum` / `chunk_order` / `save_session` / `saving` 映射为 `出了点问题，请重试。`。⑧ **顺带三条**：`SystemService.OpenStorageFolder` 新增 `kind="doc_component"`，打开应用下载的文档组件目录，不是正在用的下载组件时 `NOT_FOUND` `reason=component`，前端按钮只在 `componentState=ready` 且 `engines` 里组件那项 `source=downloaded` 时显示（6.12.54）；Windows / macOS 应用下载的组件低于 26.2.6 且没有别的合格候选时 `componentState=outdated`、`canDownload=true`，提交任务时 `DOC_COMPONENT_NOT_READY` 的 `detail` 第一行 `reason=outdated`（其他情况 `reason=<componentState>`），预览沿用 `kind=unavailable` + `reason=needs_component`（6.12.55）；`doc_convert` 记录必须填 `startedAt`（离开排队、真正开始处理的时刻，排队中为 0），没有迁移（6.12.56）。准备阶段的取消契约仍允许，前端可以不提供（6.12.57）。⑨ **v0.27.2 补充**（同版本、合入后追加）：CSV 提示统一为 `转成 CSV 只会保留第一个工作表。`；带宏的 docx 在 `GetDocPreview` 时就 `editable=false`、`editBlock=macro`（悬停 `这个文件带宏，请用默认程序打开编辑。`），保存时的 `vbaProject.bin` 检查保留作兜底（`reason=format`）；备份失败一律 `IO_ERROR` `reason=backup`，文案 `没法在这个文件夹留备份，文件没有保存。请另存为。` + 「另存为」。⑩ 真机待验证加三项：`ReplaceFileW` 替换 docx、备份清理、20 MiB 分段上传耗时。后端 v0.26.1 实现 PR 仍未合入，`installBytes` 按 6.12.28。


v0.27.1 变更（**文档预览里可以编辑 md、txt、csv、html**，老板定，架构师定方案，文案待产品定，2026-10-09；**只有契约**；完整规则见新增的 **6.12.37~6.12.46**，本条只是索引，冲突时以新节为准；后端的 v0.26.1 实现 PR 还没合入，`installBytes` 仍按 6.12.28 的说明）：① **能编辑的**：md（左写右预览）、txt 和 html（改源码）、csv（表格里改）；Word、Excel、PPT、PDF 不能编辑。② **`DocPreview` 新增** `editable`、`editBlock`（`format` / `missing` / `too_large` / `encoding` / `malformed` / `in_use`，只给程序用）、`revision`（读到的原始字节的 SHA-256，架构师定用内容哈希而不是大小 + 修改时间）、`encoding`（`utf8` / `utf8_bom` / `gb18030`）、`lineEnding`（`crlf` / `lf`）。text / md / html 超过 2 MiB 被截断、csv 超过 1000 行或被截断时 `too_large`；源文件行的文字类预览**改为读用户的原文件**（原文件不在时读副本、只读，`missing`）。③ **新方法 `DocService.SaveDocText({sourceId|taskId, revision, text|rows})`**：源文件行写回原文件并把同样内容写进副本（更新 `convert_copies` 的大小和修改时间，不重新复制），结果行写回输出文件并更新 `result.sizeBytes`、发 `task:status`；同目录临时文件 + 原子替换（Windows `ReplaceFileW`，保留权限和属性）；revision 不一致 `TASK_CONFLICT` `reason=file_changed`，正在转换 `TASK_CONFLICT` `reason=in_use`（副本复制中沿用 `reason=copying`），原文件不在 `NOT_FOUND` `reason=file`，没有权限 `IO_ERROR` `reason=permission`，被占用 `IO_ERROR` `reason=in_use`，超过 2 MiB / 1000 行 `INVALID_ARGUMENT` `reason=too_large`；按读取时的编码（含 BOM）和多数行的换行风格写回，GBK 按 GB18030 写；csv 只用逗号、按 RFC 4180 加引号；返回新的 `revision`。④ **新方法 `SaveDocTextAs({sourceId|taskId, targetPath, text|rows, encoding: keep|utf8})`**：不要 revision；扩展名必须同一种格式（`INVALID_ARGUMENT` `reason=format`）；目标不能在应用数据目录内（`output` 除外）和上传目录内；不新建源文件行，保存的路径登记进 `RevealInFolder` 的放行表供「打开所在文件夹」。⑤ **新方法 `SystemService.SaveFileDialog(defaultName, filters)`**：系统保存对话框，取消返回 `""`；`defaultName` 给绝对路径时用它的文件夹作初始位置；覆盖确认由对话框负责，平台不自带时后端补一个确认框。⑥ **保存原样写入**，只做换行和编码两步；html / md 的预览仍是过滤后放进 sandbox iframe。⑦ **没有新增错误码**（2.1 仍是 30 个）；2.2 新增文档编辑一行：`TASK_CONFLICT` 的 `file_changed`、`in_use`，`INVALID_ARGUMENT` 的 `too_large`、`format`，以及沿用的 `NOT_FOUND` `file`、`IO_ERROR` `permission` / `in_use` / `io`、`TASK_CONFLICT` `copying`。⑧ 没有迁移，没有新事件。

v0.27.1 补充（团队定，2026-10-09；版本号不变，规则改在 6.12.37~6.12.46 原处，冲突时以本条为准）：① **编码**：取代原来“按 GB18030 写回”。非 UTF-8 的文件读取仍按 GB18030 解码，`encoding` 取值由 `gb18030` 改名为 **`gbk`**；写回**严格按 GBK（CP936）**，不生成 GB18030 的四字节序列（中文 Windows 上的 Excel、记事本按 CP936 读）；有字符编不进 GBK（emoji、生僻字）就**拒绝保存**，`INVALID_ARGUMENT` `reason=encoding`（2.2 新取值，`detail` 后两行 `char=U+XXXX`、`line=<n>`），文案 `有些字符没法按原编码保存，请另存为 UTF-8。`，不悄悄替换；`SaveDocTextAs` 可以设 `encoding=utf8`。② **错误区分**：正在转换由 `TASK_CONFLICT` `reason=in_use` 改名为 **`reason=converting`**（`editBlock` 的 `in_use` 同样改名为 `converting`）；原文件被其他程序占用、读不了或替换不了是 `IO_ERROR` `reason=in_use`，和 `TASK_CONFLICT` `reason=file_changed` 分开；重申先写同目录临时文件再原子替换，中途失败原文件不变（补了 `ReplaceFileW` 各返回值的处理）。③ **编辑和保存的文案产品已定**（6.12.45），任何保存出错都保留用户编辑的内容。④ **v0.27 待定的三句定稿**：`DOC_ENGINE_BUSY` 转换 `请先关闭正在打开的文档，再转换。`、预览 `请先关闭正在打开的文档，再预览。`；`simple_fallback` `这次是简易转换，只保留了文字。可以稍后重转。`；文本截断 `文件太长，只显示了前面一部分。`。⑤ 仍没有新增错误码（2.1 仍是 30 个），没有迁移、没有新事件；后端 v0.26.1 实现 PR 仍未合入，`installBytes` 按 6.12.28。


v0.27 变更（**本机 Office / WPS 引擎 + 文档预览**，老板和产品经理定范围，架构师定方案，2026-10-09；**只有契约**，在 v0.26 的实现合入后接着做，不推倒重来；完整规则见新增的 **6.12.23~6.12.36**，本条只是索引，与 6.12.9~6.12.22 冲突时以新节为准）：① **引擎**：Windows 上按 `Microsoft Office` → `WPS` → 应用下载的文档组件 → 系统安装的 LibreOffice 的顺序挑（两个文档组件都有时应用下载的优先，与 v0.26 一致），四个都没有才走 v0.26 的下载引导或简易转换；macOS / Linux 不变。纯 Go 的 COM 库 `go-ole`（无 cgo，只在 `windows` build tag 下编译），调 Word / Excel / PowerPoint 或 WPS 文字 / 表格 / 演示自己的另存 / 导出 PDF；WPS 各版本的 ProgID（`KWPS` / `KET` / `KWPP` 和旧名 `WPS` / `ET` / `WPP`）需真机验证；检测只读注册表，看 `LocalServer32` 实际指向的程序（WPS 兼容模式会占用 `Word.Application`）。② **安全硬规则**：引擎只打开我们自己的临时拷贝、只读、`AutomationSecurity=3` 并读回确认（确认不了的引擎不收含宏文件）、不更新外部链接、关掉所有提示、窗口不可见、不改任何会保存下来的选项。③ **不碰用户的文档和窗口**：Word / Excel 每个任务新开自己的实例（创建前后拍进程快照，没有新进程就立刻放手换引擎），超时只结束我们自己的 PID；PowerPoint / WPS 演示只要在运行就换引擎，没有可换的返回新码 `DOC_PRESENTATION_BUSY`（可重试，任务保留）。④ **池和超时（架构师定）**：Office / WPS 单独一个池、并发 1，不占文档组件的 2 个名额（文档页最多同时 3 个转换）；单次超时 Office / WPS 3 分钟、组件 5 分钟，整个任务最多 10 分钟；超时或出错自动换下一个引擎，全不行就回退简易转换（`result.warnings` 带 `simple_fallback`）或失败；同一个程序连续 2 次启动失败或超时，本次运行内跳过。⑤ **组件状态**：`DocComponentStatus.source` 加 `office` / `wps`，`state` 变为“任何引擎可用就是 ready”（有 Office / WPS 就不出下载引导），新增 `componentState`（文档组件自己的状态）、`engines`（`DocEngineInfo{id, name, version, source?, installed, families, available}`；Windows / macOS 始终含 component 项，没下载时 `installed=false`，设置页显示 `文档组件（未下载）`，可以选，选中后前端弹确认框再调 `InstallDocComponent`，后端不自动下载），以及 v0.26.1 的 `installBytes`（1.5 GiB，Linux 0；后端实现 PR 一并给出）；前端收到 `doc:component` 一律重拉格式表。⑥ **设置** `Settings.docEngine`：`auto`（默认）/ `office` / `wps` / `component`，选中的不可用时按自动顺序，不报错。⑦ **记录**：`TaskResult.engine`（`office` / `wps` / `component` / `go` / `simple`），记录详情显示“由本机 WPS 转换”；`params` 不锁定引擎，重试和重转按当时的设置重新挑。⑧ **格式表**：有任何一个可用引擎能做就 `available=true`（能力表 6.12.27：WPS 一期不做 ODF），新增 `DocTarget.engines`。⑨ **预览**：新方法 `GetDocPreview({sourceId|taskId})`、`CancelDocPreview(previewId)`，新事件 `doc:preview`；返回值按 `kind` 区分：`pdf`（Office 类文件由引擎生成临时 PDF，或结果本身是 PDF；给 `/local/<token>` 本地地址，用转换页 localassets 的白名单，只放行这一个文件）、`text` / `md` / `html`（后端按 v0.26 编码规则转成 UTF-8 原文，上限 2 MiB，超过截断并标 `truncated`；md 由前端渲染，md / html 都经 DOMPurify 过滤后放进不带 `allow-scripts` 的 sandbox iframe + 严格 CSP，不加载任何网络资源）、`csv`（解析好的前 1000 行 `rows` + `totalRows`，前端写 `只显示前 1000 行，共 n 行。`）、`raw`（没有引擎时 docx / xlsx 的简易预览，给原文件的本地地址，不用 base64，≤ 50 MiB）、`unavailable`（`reason=too_large_for_simple` / `needs_component`，只给程序用）；生成 PDF 的缓存在 `%LocalAppData%\FFmpegFree\cache\preview`（键 = 路径 + 大小 + 修改时间 + 引擎，上限 1 GiB，按最久没打开的删），单独排队、并发 1、不进任务中心，单次 Office 90 秒 / 组件 2 分钟、整个预览最多 3 分钟；前端的缩放和工作表标签不归契约管。⑩ **新增 2 个错误码**（2.1 由 28 个变为 30 个）：`DOC_PRESENTATION_BUSY`、`DOC_ENGINE_BUSY`（架构师定，文字 / 表格类只剩会挂到用户实例上的 Word / Excel / WPS 时用，文案待产品确认），都可以重试；预览没有专门的失败码，前端除密码、被占用以外一律显示 `这个文件暂时无法预览。`。⑪ **没有迁移**（`engine` 在 `tasks.result` 的 JSON 里，设置在键值表，预览缓存不进库）；下一个迁移号是 `0010`。⑫ 未验证事项单列在 6.12.36（ProgID、只读和禁用宏、PPT 被占用判断、实例隔离、超时后进程清理、sandbox CSP 等）。


v0.26 变更（**文档多格式转换一期**，老板提出、产品经理定范围、架构师定方案，2026-10-09；**只有契约，前后端按本版并行实现**；完整规则见新增的 **6.12.9~6.12.22**，本条只是索引）：① **范围**：文字类 doc / docx / odt / rtf / txt / html / md 互转，表格类 xls / xlsx / ods / csv 互转（多工作表转 CSV 只出第一个表，带提示），演示类 ppt / pptx / odp 互转，以上都能转 PDF；跨类不转；PDF 作输入一期不支持；二期（PDF 转图片、PDF 转 Word、epub）只列为后续。② **实现**：「文档组件」= LibreOffice headless，界面只叫文档组件（唯一例外：Linux 未就绪提示 `请先在系统里安装 LibreOffice，然后重启应用。`）；md → html 用 goldmark、html → md 用 html-to-markdown（纯 Go，无 cgo）；md 转其他走 md → html → 文档组件，其他转 md 走 文档组件 → html → md；md 本地图片按 md 所在目录解析，网络图片不下载。③ **组件安装**：先检测系统安装；固定 **LibreOffice 26.2.6**，Windows 下载官方 MSI 用 `msiexec /a` 解到应用自己的目录（不弹 UAC），macOS 下载 dmg 挂载后拷出 .app，Linux 只检测；URL、大小、SHA-256 写死并已实际下载核对；断点续传，校验不过删掉重下；阶段 downloading / preparing（不给百分比，到检测通过为止）/ ready；可取消、可重试。新增 `DocComponentStatus`（`version`、`source=system|downloaded`，`path` 是 `json:"-"`）。④ **并发**：文档组件单独一个池，并发 2，与转换组件的 batch 池互不占用；md ↔ html 和简易转换不占名额、不排队；排队任务带 `queuePosition`，界面 `排队中 · 前面还有 n 项`；每个任务独立 `-env:UserInstallation`，单任务 5 分钟超时、杀整个进程树。⑤ **CSV**：输出 UTF-8 + 逗号；读入合法 UTF-8（含 BOM）按 UTF-8，否则按 GB18030 转 UTF-8，不提示（TXT、MD 同样）。⑥ **加密文件**后端自己检测（OOXML 加密容器、ODF manifest、doc / xls / ppt 加密标志），直接 `DOC_ENCRYPTED`，不交给组件。⑦ **格式表由后端给**：`DocService.GetFormatMatrix()`，每个目标带 `needsComponent` / `simple` / `available` / `hintKey` / `hint` / `disabledReason`，前端多选取交集，`doc:component` 变为 ready 时重新拉；组件未就绪时 md ↔ html 可用，docx / odt / txt → PDF 走简易转换（提示 `下载文档组件后可保留图片和排版`），doc / rtf 及其他置灰（`需要文档组件`）。⑧ **任务**：新类型 `doc_convert`，沿用转换页的记录、事件、重试、重转；`office_pdf` 只给旧记录和简易转换（输入加 odt、txt）。添加文件返回 `sheetCount`（xlsx / ods / xls 实读，csv 1，读不出 -1）。⑨ **新增 10 个错误码**（2.1 由 18 个变为 28 个）：`DOC_ENCRYPTED`、`DOC_CORRUPT`、`DOC_TIMEOUT`、`DOC_COMPONENT_CRASHED`、`DOC_COMPONENT_NOT_READY`、`DOC_DOWNLOAD_FAILED`、`DOC_CHECKSUM_FAILED`、`DOC_COMPONENT_INSTALL_FAILED`、`DOC_FORMAT_UNSUPPORTED`、`DOC_PDF_INPUT_UNSUPPORTED`，文案和是否可重试见 6.12.20；2.2 新增 `UNSUPPORTED` `reason=not_retryable`。⑩ **新方法**（DocService）：`GetFormatMatrix`、`GetDocComponentStatus`、`InstallDocComponent`、`CancelDocComponentInstall`、`RecheckDocComponent`、`AddDocSources`、`ListDocSources`、`SearchDocSources`、`SubmitDocConvert`；**新事件**：`doc:component`、`doc:component-progress`、`doc:queue`；`Task` 新增可选 `queuePosition`（只在内存）。⑪ **迁移 `0009_doc_convert.sql`**：`convert_sources` 加 `kind`（`media` / `doc`）、`sheet_count` 和索引。


v0.25.4 变更（包 24，老板实机：左下角已是“转换组件已就绪”，转换设置里格式全灰、悬停“转换组件尚未就绪”，并停在“正在加载格式…”；**没有新增错误码、事件或迁移**；唯一的接口变化是拿掉 `FFmpegStatus.path`，见第⑤⑥条）：① **根因**：转换页在冷启动时调用 `GetFormatCatalog`，那时转换组件还在检测。目录接口直接把每一项标成 `encodable=false`、`reasonCode=converter_not_ready`、`reason="转换组件尚未就绪"`，并不等检测结束。这次“未就绪”不写入能力缓存，但前端只取一次，检测完成后不再取，格式就一直灰着；左下角读的是稍后的 `ffmpeg:status`（`state=ready`），两边不是同一次判断。② **修复**：`GetFormatCatalog`（以及提交前用同一份能力判断的格式检查）**只在检测进行中**才等，最多 **6 秒**，或调用方自己的超时更短时以调用方为准。6 秒内检测变为就绪，就按真实的 muxer / encoder 返回（可输出的项 `encodable=true`）。6 秒到了仍在检测，照旧返回未就绪（不缓存），前端此时组件状态仍是 `checking`，不要当成最终结果。检测结束为未就绪（没有可用组件）时**立刻**返回未就绪，不等满 6 秒。没在检测时不等。③ **重新加载**：事件名 **`ffmpeg:status`**，payload 是完整的 `FFmpegStatus`（`state`、`version`、`source` 等，**没有 `path`**，见 9.4）。`state` 变为 **`ready`** 时前端重新调用 `GetFormatCatalog`。前端自己的加载超时必须 **大于 6 秒**（现为 8 秒）；先返回的未就绪目录在 `state` 仍是 `checking` 时继续显示加载，等这个事件再取。④ 能力探测失败、以及未就绪，都不写入缓存；缓存键仍是可执行文件路径 + 大小 + 修改时间（6.16.1），换了组件自动重新检测。见 6.16.1 / 6.16.3。⑤ **组件路径不再给前端**（#117 合入后，架构师定，仍是 v0.25.4，不另开版本号）：`FFmpegStatus` **没有 `path`**。`missing` / `outdated` 的 `error.detail` 只保留来源和失败原因（`[来源] 原因`），不含组件可执行文件路径，也不逐行列出候选路径。`SetFFmpegPath` 校验失败（`INVALID_ARGUMENT`）的 `detail` 同样不含路径。打开文件夹仍只用 `OpenStorageFolder("component")`。应用日志仍记路径。⑥ **`LIVE_PUSH_INTERRUPTED` 用 `detail` 第一行区分推流和拉流**，不靠 message：推流中途中断是 `reason=push`（没有 stderr 时 detail 就是这一行，有则第二行起仍是脱敏后的 stderr；所选窗口没了仍是 `LIVE_SOURCE_GONE`，`detail` 仍是 `kind=window`）；拉流中断是 `reason=pull`，message 仍是 `拉流被中断，请重新拉流。`。这两处 detail 不含组件可执行文件路径。前端按 `reason` 判断。

v0.25.3 变更（包 24，架构师定；**没有新增错误码、事件或迁移**；`TaskStatus` 没有新增取值（`interrupted` 早就有），新增的是它的一种来源；新增字段 `FFmpegStatus.customPathInvalid`、`FFmpegStatus.source` 新增取值 `default`、`PullSession.hasVideo` / `hasAudio`、`live:pull` 的 `hasVideo` / `hasAudio`；Wails 绑定已重新生成）：① **直播任务被中断记为 `interrupted`（N4）**：推流 / 屏幕推流**已经开始以后**（收到过第一条输出进度）非正常结束（服务器断开、进程被杀、所选窗口没了、中途显卡编码失败……），任务终态是 `interrupted`（已中断），**不再是 `failed`**；`error` 照常带上（`LIVE_PUSH_INTERRUPTED`，所选窗口没了是 `LIVE_SOURCE_GONE`），**不会是 `INTERNAL`**：已开始以后认不出原因的退出（例如进程被外部强杀，stderr 里没有可认的行）从 `INTERNAL`「推流异常退出」改成 `LIVE_PUSH_INTERRUPTED`，message `推流被中断，请回到直播页重新推流。`，detail 是脱敏后的 stderr 尾部。**开始之前**的失败（连不上、被拒绝、没有权限、启动失败）仍是 `failed`，错误码不变；用户停止仍是 `succeeded` / `canceled`；应用退出仍是 `interrupted` 且没有 `error`（所以 `interrupted` + 有 `error` = 直播中途被中断，`interrupted` + 没有 `error` = 应用退出时被中断）。`task:status` 终态事件、落库、`ListTasks` 的 `statuses` 筛选和 `total` 都按 `interrupted` 走（`tasks.status` 没有约束，不需要迁移）：按 `["failed"]` 查**不含**它，任务中心的“失败”数和“失败”页只传 `failed` 就不会算进去；“隐藏已结束”（`HideFinishedInTaskCenter`）照常把它当已结束。**重试**：直播任务不管什么状态都不能重试（不变：`Retry` 返回 `UNSUPPORTED`「直播会话不能重试，请重新开始推流」），被中断的也一样，回到直播页重新推流。转换记录的 `status="failed"` 筛选（6.14，只看 `convert` 任务）不受影响。② **拉流预览**的 `live:pull` `interrupted` 也带 `error`：`LIVE_PUSH_INTERRUPTED`（“直播连接在开始以后断开”，推流、拉流共用这个码，不新增码），message `拉流被中断，请重新拉流。`，detail 是脱敏后的 stderr 尾部。标题（“推流被中断” / “拉流被中断”）由前端定，后端除了状态和错误码不需要别的文字。③ **新总则（1.1 新增，架构师定）**：错误码和 `reason` 等 `detail` 枚举只给程序用，**界面上永远不显示**；前端按 `code`（和 2.2 的 `reason`）映射成中文，映射不到的一律显示 `出了点问题，请重试。`。后端的 `message` 必须是给用户看的中文：不含错误码、不夹英文单词或参数名、不用 `%v` 塞英文原文（原文放 `detail`）。按这条改了后端已有的 message：`limit 范围 0~N，offset 不能小于 0` → `分页参数不正确`；`status 只能是 ""、"active" 或 "failed"` → `筛选条件不正确`；`recordLimit 范围 0~N` → `每行记录条数超出范围`；`length 必须在 1 到 1 MiB 之间` / `offset 不能为负` / `offset 超出范围`（`ReadPDFChunk`）→ `读取范围不正确`；`atSec 必须是不小于 0 的数字` → `取帧时间不正确`；`编码器偏好必须是 auto、cpu 或设备 id` → `编码器设置不正确`；`下载的文件校验失败（SHA256 不一致），请重试或换镜像` → `下载的文件校验失败，请重试或换镜像`；`过滤器格式不对，应为 *.mp4 这样的通配符` → `文件类型过滤器格式不对`；`kind 只能是 output、uploads 或 component` → `不支持打开这个文件夹`；`which 只能是 "input" 或 "output"` → `参数不正确`；`未知的任务类型 "<type>"` → `不支持的任务类型`；`任务执行时发生 panic: <原文>` → `任务执行时出错，请重试`；`<type> 类型的任务不支持重试` → `这类任务不支持重试`；原来的英文 / 参数说明都挪进 `detail`，错误码不变。新加测试 `TestUserFacingMessageIsChinese` 扫描后端所有 `apperr.New` / `Wrap` 的 message 字面量（不含错误码、不夹英文单词、不用 `%v`、不出现“剪辑”；允许 `FFmpegFree`、`PDF`、`JSON`、`ID` 等产品名 / 格式名）。④ **组件状态（N5，第 9.4 节）**：`FFmpegStatus.source` **始终有值**：`ready` 时是实际在用的组件来源（`custom` / `bundled` / `system` / `legacy`）；其他状态（`checking` / `missing` / `outdated` / `installing` / `failed`）没有在用的组件，是用户的设置：手动指定了路径为 `custom`，否则为新取值 **`default`**。新增 **`customPathInvalid: boolean`**（始终输出）：用户手动指定的路径不可用（不存在、不是转换组件、版本过低）。手动路径坏了而别处（应用自带目录、系统 PATH、v1 目录）有可用的组件时**用可用的那个**：`state=ready`，`source` 是实际来源（不是 `custom`），设置里的手动路径保留不动，`customPathInvalid=true`；别处也没有时 `missing` / `outdated` + `source=custom` + `customPathInvalid=true`。这个标记只由检测结果决定，`checking` / `installing` / `failed` 时为 `false`；后端不带提示文字（文字由产品经理定、前端显示）。「恢复默认」= `SetFFmpegPath("")`（或 `UpdateSettings` 把 `ffmpegPath` 改成 `""`）：任何状态下都清掉手动路径并立即重新检测，先发 `checking` 再发结果，结果里 `customPathInvalid=false`、`source` 是实际来源或 `default`；安装进行中时清掉设置、返回并保持 `installing`，装完按新设置检测。安装成功时如果设置里还有手动路径，按检测顺序重新检测一次（手动路径能用就继续用它，不能用就用刚装好的并带上标记）。**新字段不含路径**；已有字段里 `path`（当前组件的绝对路径）和 `missing` / `outdated` 时 `error.detail`（逐行列出各候选的路径和失败原因）仍会把路径给到前端，这次没有删（见 9.4 末尾的说明，等架构师定）。⑤ **纯音频拉流（N2）**：`live:pull` 的 `playing` 一定带 **`hasVideo` / `hasAudio`**（按我们送出的 FLV 头里的音视频标志），其他状态不带；`PullSession` 也有这两个字段（同名、可选）：只有会话已经开始播放（`playing` 已发，例如同一地址重复 `StartPullPreview` 拿到已有会话）时才有，新会话刚开始还不知道时省略，前端以 `playing` 为准建播放器（纯音频不等视频）。`GetPreviewStream` 的 `hasVideo` / `hasAudio` 收到 FLV 头以后也按头里的标志。纯音频拉流端到端可用：转封装出只有音频的合法 FLV（头标志 `0x04`），GOP 缓存不等视频关键帧，后加入的客户端马上有音频；探测确认是纯音频时转封装的 `-analyzeduration` / `-probesize` 用 0.5 秒 / 500 KB（原来 RTMP 用 5 秒窗口，FLV 头说有视频，ffmpeg 把窗口等完才输出：MediaMTX 纯音频 RTMP 实测从 `StartPullPreview` 到 `playing` 由 10.7 秒降到 6.2 秒，其中 5.3 秒是探测本身；纯音频 HLS 0.2 秒）。⑥ **旧版导出（N3）**：任务中心把旧的 `edit_export` 记录显示为“旧版导出”，只能查看和删除：`Retry` 返回 `UNSUPPORTED`，message `旧版导出记录只能查看和删除，不能重试`；`ConvertService.Reconvert` / 任务层重转返回 `UNSUPPORTED`，message `旧版导出记录只能查看和删除，不能重转`；`detail` 第一行都是 `reason=feature_removed`（原来 `Retry` 的 message `剪辑功能已移除，剪辑导出记录不能重试` 含“剪辑”，已改）；后端面向用户的文字里不再出现“剪辑”。

v0.25.2 变更（包 23，老板实测：开着推流和拉流、在两个页面之间来回切换后预览冻住；**没有新增接口、错误码、事件或迁移，没有接口签名变化**；行为变化只在本机预览 HTTP 服务，6.10.3.4 / 6.10.3.5 / 6.10.3.9）：① **根因**：切页面时旧播放器的连接没有正常关闭（WebView 挂起、半开、不再读）时，后端仍把它当作正常客户端。系统的套接字缓冲会自动长到几 MB，这样的连接还能再吞几十秒的数据，写不会卡住，5 秒丢帧踢人的规则也就一直不触发；每个会话最多 4 个连接，第 4 次切回来就是 `429`，界面冻住。实测（推流 + 拉流同一路流，每次切换丢下旧连接、socket 不关）：修之前第 4~10 次切回来推流和拉流预览全部 `429`，40 秒后名额仍是 4 / 4，一个也没释放。② **满员时挤掉最早的连接，不再 `429`**（决定）：连上来的只有应用自己（Origin 白名单），最新的连接就是界面上正在显示的播放器，所以第 5 个连接到来时断开最早加入的那个（中止连接，不发结束标记），新连接照常 `200`。不按“同一 origin”区分：所有连接本来就都是 Wails 的 origin。③ **发现不读的连接**：每个播放器连接在本机一侧的发送缓冲限制为 256 KiB；每次写（含 Flush）最多等 2 秒，写不进去就断开这个连接、立即释放名额。被挤掉、太慢（连续丢帧 5 秒）的连接同样立即中止，卡在写上的那次写马上失败，不再先把队列发完。实测不读的连接在系统缓冲吞满后约 2~10 秒内被断开。④ **新连接从不从 GOP 中间开始**：GOP 缓存只从视频关键帧开始（第一个关键帧之前、缓存超限被清空之后不缓存半个 GOP）；缓存里没有 GOP 时，新连接先只收 FLV 头、metadata 和序列头，等到下一个关键帧才开始收音视频。纯音频的流照旧，不等关键帧。⑤ **地址不变（确认，不是改动）**：同一个会话反复调用 `GetPreviewStream` 返回同一个 URL（同一个 token），不会作废正在用的地址；拉流的 `previewUrl` 在会话运行期间一直有效；token 只在会话结束时作废（6.10.3.5）。⑥ **不影响推流和拉流**：分发不阻塞，卡住、被丢弃的播放器不影响推流帧率和拉流转封装。⑦ **实测**：推流到本机 MediaMTX、拉同一路流，模拟 10 次切页面（每次丢下两路的旧连接、不关 socket，再各开一个新连接，停留 4 秒）：10 次全部 `200`，0 次 `429`、0 个错误；每次都从 FLV 头 + 序列头 + 关键帧开始；首个关键帧和连续出帧都在 3 毫秒内（第一次进入拉流页 0.66 秒，等拉流出画面）；推流帧率全程 30.01 fps（源 30 fps）；每次切换后的名额（推 / 拉）最多 3 / 4，最后一次切换后约 10 秒回到 1 / 1。

v0.25.1 变更（架构师 2026-10-08 定；v0.25 实现之后的修复与补充，**没有新增错误码、事件或迁移，没有接口签名变化**；2.2 新增取值 `NOT_FOUND` `reason=component`）：① **`SystemService.OpenStorageFolder(kind)` 新增 `kind="component"`**（PM X5）：在系统文件管理器里打开当前使用的转换组件可执行文件所在的文件夹，Windows / macOS 同时选中这个文件（复用 6.8 `RevealInFolder` 的平台命令：Windows `explorer.exe /select,"<文件>"`，macOS `open -R <文件>`，Linux `xdg-open <所在文件夹>`）。转换组件不是 `ready`（检测中、安装中、没有）或文件不在：`NOT_FOUND`，message `转换组件还没有就绪。`，`detail` 只有一行 `reason=component`。**路径不回给前端**：所有错误的 `detail` 都只有 `reason=component`，不带路径；**不加进 `RevealInFolder` 的放行范围**（6.8 不变）。`output` / `uploads` 不变。签名不变，Wails 绑定不用重新生成。见 6.15.2 第 6 条。② **拉流失败有自己的分类和文字**（设计走查 G3，PM 定稿）：拉流预览在开始播放之前失败（`live:pull` 的 `failed`）不再借用推流的分类（原来会显示 `推流启动失败` / `连接推流服务器失败`）：连接类失败（DNS、拒绝连接、超时、网络不可达、远端没有这路流、读超时）是 `LIVE_CONNECT_FAILED`，`detail` 第一行 `scheme=<rtmp|rtmps|srt|http|https>`；其他是 `INTERNAL`；**两种的 message 都是 `拉流失败，请检查直播地址和网络。`**（后端一个常量 `ffmpeg.PullFailedMessage`，以后改文案只改这一处）。推流的文字不变。见 6.10.3.7。③ **HLS 拉流预览的灰色花屏（修复，行为有变化）**：根因是 HLS 一次到一整个分片——`-c copy` 转出来的 FLV 每隔一个分片时长（MediaMTX 实测约 2 秒）一次性到约 2 秒的数据，然后完全没有数据；播放器缓冲在 0 和 2 秒之间来回，旧的追帧（落后 1.5 秒就跳到最新）每分钟跳几十次，跳到 GOP 中间就是灰色花屏，一直到下一个关键帧。FLV 本身是干净的（ffmpeg 解码我们提供的 FLV 0 个错误、时间戳不倒退、分发器只在关键帧处切），不是转封装丢包。修法：**HLS 输入（地址路径以 `.m3u8` 结尾，或探测到的封装是 `hls`）的 tag 由分发器按时间戳匀速放出**（6.10.3.4 的“HLS 匀速”），并且从最新的分片开始（`-live_start_index -1`）。实测 150 秒（MediaMTX mpegts 变体）：修之前到达节奏和时间戳的偏差 3.96 秒，按旧播放器参数模拟每分钟 30.3 次跳转、9.1 秒灰屏，按新参数（6 秒才跳）不跳但每分钟卡顿 25.8 次共 6.3 秒；修之后偏差 0.5 秒，两套参数都 0 跳转、0 灰屏、0 卡顿；代价是比 RTMP 直连多约 1.8 秒延迟（中位数 1.25 → 3.0 秒），即一个分片的到达间隔，HLS 方案固有。前端不用改。④ **拉流开始阶段有上限，不会一直“正在连接…”**（包 21 实机：第一次拉流停在“正在连接…”约 30 秒）：原来拉流的 ffmpeg 输入没有任何读超时，远端接受了连接却不给数据时 ffmpeg 永远不退出（实测 rtmp / http 旧参数 40 秒还在等），会话不发 `playing` 也不发 `failed`。现在：转封装输入加 `-rw_timeout 8000000`（同探测），并且 ffmpeg 启动后 **15 秒**还没收到第一个 FLV 头就停掉它，以 `failed` 结束（`LIVE_CONNECT_FAILED`，message 同 ②，`detail` 第二行 `15 秒内没有收到数据`）；拉流的本机 TCP 等 ffmpeg 连上的时间从 15 秒改为“探测上限 12 秒 + 15 秒 + 3 秒”（原来探测慢时 TCP 已经关了，预览连不上）；播放器在 FLV 头之前连上来时 HTTP 请求最多挂 30 秒等头（原来 10 秒就 503），会话失败或预览分支没连上时立即 `503` 结束。实测远端不给数据时 12~16 秒内以 `failed` 结束。【未证实】实机那次 30 秒的具体触发点（远端当时为什么不给数据，或 Windows 第一次运行转换组件 / 探测程序时被安全软件扫描拖慢）没能复现；已排除本机 HTTP 服务懒启动（毫秒级）和 MediaMTX HLS 冷启动（首次探测 2.7 秒）。⑤ **RTMP / RTMPS 拉流的探测窗口 1 秒 / 1 MB → 5 秒 / 5 MB**（探测和转封装都改）：MediaMTX 的 RTMP 在第一个关键帧前不给 SPS/PPS，GOP 2 秒时 1 秒常常找不到编码参数（实测 12 次失败 4 次，5 秒 0 次；探测多花约 0.7 秒）。其他协议仍是 1 秒 / 1 MB：SRT（MPEG-TS）没有文件头，会一直读到窗口用完，5 秒时探测常常超过 12 秒上限（实测 6~13 秒），反而丢掉编码检查。⑥ **其他修复**：播放器断开后立即离开分发器（原来断开的连接一直占名额，重连几次就是 `429`）；后加入的客户端在同一把锁里拿开头字节并加入，tag 不重复也不遗漏。⑦ **事件顺序规则**（架构师定，前端已按此实现）：`task:status`、复制完成（`convert:copy`）、`live:pull` 这些事件**可能比创建这一行的调用先返回，也可能乱序到达**。前端对还不在列表里的 id 按 id 暂存事件，这一行加进列表时再应用；`AddSources` 返回后按这一批的 id 重新查询一次对齐。拉流会话的状态**只往终态走**，到了 `ended` 或 `interrupted` 就不再改变；**先到的事件决定状态和文字**：`ended` → `拉流已结束`，用户没按停止时再加一行 `直播已停止，或连接已断开。`；`interrupted` → `拉流被中断，请重新拉流。`；两种都提供「重新拉流」。见第 5 节末尾和 6.10.3.7。⑧ **前端文案**（记录，不是后端改动）：拉流 `preview_unavailable` 的 message 是 `这路视频暂时无法在应用内播放。`（后端已是这句），`codec` 仍是 `这路视频无法在应用内播放。`。

v0.24.6 变更（架构师 2026-10-08 定，补 v0.24.4；**是 v0.24 的修复，在 v0.25 契约之后合入，与 v0.25 无关**；**没有新增接口、错误码、reason、事件或迁移**）：**`RevealInFolder` 再放行“记录实际输出文件所在的文件夹本身”**。转换页完成横幅的“打开文件夹”，在本轮“保存到”另选了文件夹时调用 `RevealInFolder(该文件夹)`；这个文件夹可以在实际输出目录之外，原先会被拒绝。现在作为第 4 类放行：路径（先 Clean，再 `EvalSymlinks`，Windows / macOS 不区分大小写）**精确等于**某条任务登记的 `outputPath` 所在目录时可以打开。**只放行这个文件夹本身**，不放行里面任意其他文件或子文件夹（那些仍只按原来的三类判断：登记的输出文件、实际输出目录或实际上传目录之内、10 分钟内删除失败留下的那个输出文件）。所给路径本身是符号链接时不按本条放行。不在范围内时 message 仍是 `只能打开任务输出文件或默认输出文件夹里的内容`。用默认位置打开仍走 `OpenStorageFolder("output")`，本条不改（6.8、6.15.2 第 7 条）。 另外两处重转文案（架构师同日补充，错误码和 reason 不变）：① `Reconvert` 在源文件行还在复制时，message 改为 `文件还在准备中，准备好后再重转。`（`reason=copying`，其后仍有 `sourceId=`）；`SubmitSources` 仍是 `文件还在准备中，准备好后再转换。`。② `reason=format_change` 的 message 改为 `重转不能更换格式，要换格式请新转一条。`，不再出现“重新转换”（“重新转换”只留给失败 / 已取消记录的 `Retry`）。

v0.25 变更（**直播预览改成真实视频流**，老板和架构师定，包 21；**只有契约，实现在本 PR 合入后另开**；完整规则见新增的 **6.10.3**，本条只是索引）：① **删除 2 fps 图片预览**：删掉 `LiveService.GetPreview` 和 `Preview` 类型、预览这一路的 `fps=2` image2 输出（`PreviewOutputArgs`）、`PreviewProbe` 对 `mjpeg` / `image2` 的检查，以及 `<数据目录>/tmp/live-preview` 临时目录（启动时如果还在就删掉，不再重建）。6.10「预览画面（v0.17）」整节作废，只作历史记录。② **推流预览 = 推出去的那路流本身**：同一个 ffmpeg 进程，**不多编码一次**。开预览时主输出一律改成 `tee`，多一个 `onfail=ignore` 的预览分支，用 `use_fifo=1` + `drop_pkts_on_overflow=1` 做缓冲，经本机 TCP 交给后端；后端一直读，读出来分发给 HTTP 客户端。预览分支出错、看的人卡住，都不会拖慢或中断推流。带音频，前端默认静音。③ **拉流预览 = 后端转封装**：ffmpeg 读远端流 `-c copy` 成 FLV，用同样的方式提供；`PullSession` 新增 `previewUrl`。④ **本机 HTTP 服务**（v0.5 起“后端不监听任何端口”的规定**只对直播预览放开**）：只监听 `127.0.0.1` 的随机端口，每个会话一个随机 token，会话结束 token 立即作废；`Content-Type: video/x-flv`、分块传输；CORS **只回显 Wails 自己的 origin**，不用 `*`，其他 origin 和没有 `Origin` 的请求返回 403，处理 `OPTIONS` 预检。⑤ **预览开关不影响推流**（新直播页：开始前和会话面板里都有“开启预览”）：推流的预览分支**始终存在**（转换组件有 `tee` 和 `tcp` 时），开关只决定前端连不连 `GetPreviewStream`；会话中途开关预览**绝不重启、不中断推流**，也不改命令行。`FilePushRequest.preview`、`ScreenPushRequest.preview`、`PullPreviewRequest.preview` 字段保留（旧前端照传不报错），**后端忽略**。拉流预览会话总是起 ffmpeg。⑤′ **新接口 `LiveService.GetPreviewStream(sessionId) (PreviewStream, error)`**，返回 `{url, mime, hasVideo, hasAudio}`，推流会话和拉流预览会话都用它。⑥ **编码兼容**：只有 H.264 视频，以及 AAC / MP3 音频能在应用内播放。推流一定是 H.264 + AAC（或没有音频），所以总能预览；拉流时视频不是 H.264（**HEVC 默认算不支持**）返回 `UNSUPPORTED` `reason=codec`，**不转码**；只有音频不支持时去掉音频、只给画面。⑦ **结束与中断**：会话结束时正常收尾 HTTP 响应（发完分块结束标记）。推流沿用 `task:status` 区分正常结束和被中断；拉流预览新增事件 **`live:pull`**。⑧ 2.2 新增取值：`UNSUPPORTED` `reason=codec` / `reason=preview_unavailable`、`NOT_FOUND` `reason=session`。**没有新增错误码**（2.1 仍是 18 个）。⑨ 地址、推流码、token 在所有日志里脱敏。⑩ **实现 PR 补充**（不改上面的规则）：拉流被中断的文字定为 `拉流被中断，请重新拉流。`（产品经理定，6.10.3.7）；推流的转换组件硬件编码失败改用 CPU 重试时（9.7），新的 ffmpeg 进程连回**同一个**预览 TCP 端口（只在还没收到 FLV 头之前允许重连），`GetPreviewStream` 的 `url` 不变；开了预览分支时 `-progress` 的 `total_size` 是 `N/A`，推流码率改按预览分支收到的字节数计算（预览分支断开后码率显示为空，推流不受影响）。另外两条实测结论：① 预览分支方括号里 `fifo_options` 的 `:` 要写两个反斜杠（6.10.3.2 的参数原文已更正），只写一个时预览分支打不开、被 `onfail=ignore` 吞掉；② RTMP 拉流时远端停止发布、甚至推流服务器进程被杀，ffmpeg 都只看到连接正常关闭、退出码 0，所以 `live:pull` 报 `ended`（不是 `interrupted`）；`interrupted` 只在 ffmpeg 非零退出时出现（如连接被重置、读超时）。⑪ **更正 v0.24.6 的文字**（产品经理定，与直播无关，顺带改）：`Reconvert` `reason=format_change` 的 message 改为 `重转不能更换格式，要换格式请新转一条。`（6.17.1），错误码 `INVALID_ARGUMENT` 和 `reason=format_change` 不变。

v0.24.5 变更（直播推流帧率，老板：按源帧率推流；**没有新增错误码、没有接口签名变化**）：① **主输出不加任何程序自定的帧率 / 尺寸限制**（再次确认并加测试）：文件推流 `options.fps=0` 时主输出没有 `fps` 滤镜、没有 `-r`，沿用源帧率；`width/height=0` 时只补偶数（`scale=trunc(iw/2)*2:trunc(ih/2)*2`），保持源尺寸；`-g` = 2×源帧率（探测不到按 30）；用户给了 `fps` / 尺寸才加在主输出上。屏幕推流的帧率由采集端 `-framerate` 决定（`options.fps=0` 仍按 30 采集，屏幕没有“源帧率”），主输出同样没有 `fps` 滤镜。每秒 2 帧只在预览这一路。② **预览输出改为包在 `fifo` 封装里**（6.10「预览画面」）：`-map 0:v:0 -an -sn -dn -vf fps=2,scale=640:-2 -c:v mjpeg -q:v 5 -protocol_whitelist file -f fifo -fifo_format image2 -format_opts update=1:atomic_writing=1 -queue_size 4 -drop_pkts_on_overflow 1 -attempt_recovery 1 -recover_any_error 1 -recovery_wait_time 1 -max_recovery_attempts 0 file:<预览路径>`。原因（实测 ffmpeg 7.1）：同一进程里预览这一路写文件一卡住，会反压到共用的解码器，约 12 秒后推流完全停住；预览文件改名失败（Windows 上读取端正打开着预览文件时 `MoveFileEx` 会被拒绝）会让整个 ffmpeg 以 `Error muxing a packet` 退出、推流一起断掉。包了 `fifo` 后预览写慢就丢预览帧、写失败 1 秒后重试，推流不受影响。`PreviewProbe` 另要求有 `fifo` 封装，没有时这次会话不加预览（同其他降级，不报错）。拉流预览用同一个 `PreviewOutputArgs`。③ 直播推流的完整 ffmpeg 参数（推流地址、密钥脱敏）在启动时写进应用日志；`LiveService` 的日志此前没有接到应用日志，本版接上（写进 v0.24.2 的 `<数据目录>/logs/app.log`）；推流地址整段换成 `<推流地址>`，主机、端口、密钥都不写。④ 本版只是过渡：v0.25 用真实视频流替换 2 fps 图片预览。

v0.24.4 变更（架构师 2026-10-08 定，补 v0.24.1；**没有新增接口、错误码、reason、事件或迁移**。与 v0.24.1 ⑪“没有打开原文件所在位置”冲突时以本条为准）：① **“打开所在文件夹”和“用系统程序打开”都打开用户的原文件**，不打开实际上传目录里的副本（`<base>/uploads/<sourceId>/`，回退时在用户数据目录下）。`RevealSource(sourceId)` 打开 `originalPath` 所在的文件夹并选中原文件；`OpenSourceWithSystem(sourceId)` 用系统程序打开原文件。原文件已经不在（路径为空、不是绝对路径、`Stat` 失败或不是普通文件）时返回 `NOT_FOUND`（`detail` 首行 `reason=file`，message `原文件不存在，无法打开。`），**不退回副本**。扩展名白名单等其余规则不变（6.14.7）。`GetSourcePreviewURL` / `GetSourceThumbnail` 仍用显示路径（6.15.6），本条不改。② **直播本地存档留空时用实际输出目录**，跟转换、文档转 PDF 一样。`Settings.defaultOutputDir` 为空表示 `<base>/output`（6.15.1：`<base>` 默认是程序所在文件夹；该文件夹不可写时两个目录一起回退，Windows 回退到 `%LocalAppData%\FFmpegFree`，macOS / Linux 回退到应用数据目录；macOS `.app` 包直接用应用数据目录，不算回退）。直播页打开“保存存档”、用户没有另选文件夹时，存档目录**显示这个实际绝对路径**（`GetStorageDirs().outputDir`：自定义的 `defaultOutputDir` 优先，否则 `<base>/output`），不显示空的；开始推流时把该路径作为 `StartScreenPush` 的 `archiveDir` 传入。关掉“保存存档”仍传 `archiveDir=""`，表示不存档（6.10 的 `""` = 不存档不变）。用户另选了文件夹就用所选的，直到改回。 ③ **“复制中”的提示改说“准备中”（包 20 用词）**：副本还在复制时，后端给用户看的 message 不再说“复制”：`GetSourcePreviewURL` 改为 `文件还在准备中，准备好后才能预览。`；`SubmitSources` / `Reconvert` 的行还在复制（一行都没就绪时的整体错误）改为 `文件还在准备中，准备好后再转换。`；`RetryCopy` 遇到正在复制的行改为 `文件还在准备中，不需要重试。`（已复制完成的仍是 `文件已经在复制或已复制完成`）。**错误码和 `reason=copying`（及其后的 `sourceId=` 行）都不变**，前端仍按 `reason` 判断。

v0.24.3 变更（**源文件行的媒体信息在启动时补不上**，查包 18 / 19 的 Windows 实机库后修；**没有新增接口、字段、错误码、事件或迁移**）：① **根因**：实机库里 `convert_sources` 两行的 `media` 都是 `NULL`、`media_fp` 都是空（迁移 0006 已应用，0007 还没有，即最后跑的是没有 v0.24 副本的包 19），页面上的信息全靠退回关联 `media` 表（没有采样率、声道和流信息）。转换页是首页，启动时 `ListSources` 比转换组件的首次检测先到，懒探测拿到 `FFMPEG_NOT_FOUND`，按“暂时性错误不记，下次再试”什么都没写；而前端一次会话只列一次，所以每次启动都补不上（和 v0.24.2 ② 缩略图是同一个时序问题）。② **修复**：`ListSources` / `SearchSources` / `GetSource` / `GetSourcePreviewURL` / `AddSources` 的懒探测**只在有行需要探测（指纹不一致）且转换组件正在检测（`checking`）时**，先等检测有结果（最多 15 秒）再探测，同一次调用就能补上并落库；检测结果是 `missing` / `outdated` 等时立即照旧返回（不探测、不记）；没在检测时不等。**前端可见的变化**：应用刚启动、转换组件还在检测时，这几个调用可能多等到检测结束（通常 1~3 秒，最多 15 秒）才返回；返回的 `source.media` 是持久化的完整结果。持久化之后文件不变就不再探测，以后的启动不再受检测时序影响。其他依赖转换组件的接口（缩略图、提交转换等）不等，仍按 9.5 立即返回 `FFMPEG_NOT_FOUND`。

v0.24.2 变更（**默认缩略图取帧规则 + 应用日志**，包 19 Windows 实测转换页缩略图全部是类型图标后修；**没有新增接口、错误码、事件或迁移**）：① **默认缩略图改成“第一帧，太暗就取前 3 秒里第一张不黑的”**（老板、设计定，取代“时长的 10%、最多 10 秒”）：只解码前 3 秒，先缩到目标宽度（320）、转 8 位，按平均亮度 `YAVG > 32` 取第一张；前 3 秒全黑（或这一步失败）退回第一帧，仍然出图。视频和 GIF 一样。适用于 6.14.10 的 `GetRecordThumbnail` / `GetSourceThumbnail` 和 6.7 `Probe` 附带的默认缩略图（`ListRecent` 只查同一个缓存）；**`MediaService.Thumbnail(path, atSec, width)` 指定时间点的不变**；**v0.24 视频转图片的输出不走这段代码，仍按它自己的规则**。缓存键加版本号（`v2`），旧缩略图自动作废、按新规则重新生成。② **包 19 的根因：启动时转换组件还在检测（`checking`），页面已经来取缩略图**，后端只能返回 `FFMPEG_NOT_FOUND`，旧前端整个会话不再重取，全部停在类型图标。前端 #96 起失败不永久缓存、转换组件就绪（`ffmpeg:status` 变成 `ready`）时重取。接口仍是同步的：调用一直等到图生成好（每次 ffmpeg 最多 20 秒，退回第一帧时最多再一次）才返回，不返回空串；失败不缓存，下次调用重新生成。`GetSourceThumbnail` 用显示路径（6.15.6）：副本复制中 / 失败 / 已取消时截原文件，不用等副本。③ **应用日志** `<数据目录>/logs/app.log`（Windows `%AppData%\FFmpegFree\logs\app.log`）：所有后端日志同时写入（Windows 包没有控制台，之前全部丢失），超过 5 MB 启动时轮转成 `app.log.1`。缩略图每次失败记一行：`缩略图: 转换页 source|record=<id> path=… code=… msg=… detail=…`；ffmpeg 本身出错或超时另记 `缩略图: ffmpeg 失败|超时 exe=… in=… out=… at=auto|<秒> exit=<码>(0x<十六进制>) 用时=… stderr=…`；转换组件未就绪记 `缩略图: 转换组件未就绪…`；每次检测转换组件记 `转换组件检测: state=… path=… source=… version=… 用时=…`。

v0.24.1 变更（v0.24 实现 PR 定稿：架构师、PM 的决定和实现取舍；**没有新增错误码**，2.2 新增取值 `INVALID_ARGUMENT` `reason=params_locked`；本条与 v0.24 冲突时以本条为准）：① **`copyState` 名称和取值确认（不变）**：`ConvertSource.copyState`，取值 `none`（v0.24 之前没有副本的旧行，读 `originalPath`）/ `copying` / `ready` / `failed` / `canceled`，实现照此输出。② **`TaskPathCheck`（架构师定名）**：删掉 v0.24 草稿的 `canReconvert`，改为 `reconvertMode`（`"replace"` = 旧输出还在，成功后原子替换；`"regenerate"` = 旧输出不在了，按原参数重新生成；`""` = 不能重转）和 `reconvertBlock`（`reconvertMode=""` 时 `invalid_state` / `copy_not_ready` / `source_missing` / `output_moved`，否则 `""`），两个字段始终输出；不存在的 id 两个都是 `""`，不是 `convert` 的任务是 `invalid_state`。③ **`Reconvert` 同步校验顺序（架构师定）**：记录不存在 / 旧类型 → 不是 `convert` → 状态（`invalid_state`）→ 副本没就绪（`copying` / `copy_failed`）→ 源文件不在（`NOT_FOUND` `reason=file`）→ 旧输出被换掉（`output_moved`）→ `params_locked` → `format_change` → 其余参数和格式检查。④ **旧输出不在了（PM 15a）**：不再按 `output_moved` 拒绝，改为 `regenerate`：**只能用原参数**，给了 `presetId` 或 `options` 返回 `INVALID_ARGUMENT`（message `原来的输出文件不在了，只能按原来的参数重新生成`，`detail` 首行 `reason=params_locked`）；同样先写临时文件，落盘前复核目标仍不存在，**绝不覆盖**（这期间目标位置出现了文件就按 `output_moved` 失败）；失败或取消时记录仍是 `succeeded`、输出仍不在，失败写 `lastReconvertError`。**目标位置是别的文件（PM 15b）**：`TASK_CONFLICT` `reason=output_moved`（同 v0.24）。源文件不在：`NOT_FOUND` `reason=file`（同 v0.24）。⑤ **重转被退出打断（PM 13）**：退出时只删临时文件、发 `reconvertOutcome: "interrupted"` 的终态事件，**不落库**；下次启动 `RecoverReconverts`（在 `MarkInterrupted` 之前）把它恢复成 `succeeded` 并计数；新增 **`ConvertService.TakeInterruptedReconverts() int`**：第一次调用返回这个条数，之后都返回 0；前端文案 `上次退出时有 n 条重转被中断，原来的文件没有变动。`。⑥ 重转的事件不带参数（`task:status` 不带 `params`）；**重转会取消任务中心的隐藏，结束后保持不隐藏**（架构师定，6.17.7 的开放问题关闭）；旧输出被别的程序占用时**不事先提示**，替换时按 `IO_ERROR` `reason=in_use` 失败（6.17.5）。⑦ **用词**：与重转有关的用户文案一律说“**重转**”，不说“重新转换”（“重新转换”只留给失败 / 已取消记录的 `Retry` 按钮）；6.17 的 message 随之改为 `只有已完成的记录可以重转`、`这条记录正在重转`、`重转不能更换输出格式，换格式请重新添加转换`、`原来的输出文件已被移动或替换，不能重转`、`源文件不存在，无法重转`、`原来的输出文件在重转期间被移动或替换，新结果没有保存`，任务日志行 `[FFmpegFree] 重转：<paramsSummary>`。⑧ **6.12 输出目录规则改写（架构师定）**：禁止的是应用数据目录本身及其下的一切，**`<dataDir>/output` 及其子文件夹除外**（按固定的文件夹名 `output` 放行，和这次启动是否回退无关；`<dataDir>/outputs` 这类不算；比较方式不变：两边 `EvalSymlinks`，再按平台规则处理大小写）。同一条规则用于转换（`SubmitSources` / `Submit` / `Reconvert` / `PreviewOutputName` 的 `outputDir`，message `输出目录不能在应用数据目录内`）、Office 转 PDF（6.12.3），以及 `SetStorageDirs` / `UpdateSettings` 的自定义输出目录（message `保存位置不能在应用数据目录内`）。6.12.3 的“仍空 = 源文件所在文件夹”改为“仍空 = `<base>/output`”。⑨ **Windows 回退位置改为 `%LocalAppData%\FFmpegFree`**（架构师定；应用数据目录仍是 `%AppData%\FFmpegFree`）；macOS / Linux 仍回退到应用数据目录，所以 ⑧ 的例外主要影响 macOS `.app` 包和 Linux。⑩ **Office 转 PDF** 的 `outputDir` 和 `defaultOutputDir` 都空时输出到 `<base>/output`；删掉 v0.24 里“剪辑导出的默认输出目录”的说法（剪辑在 v0.23.5 已移除，没有代码为剪辑设置输出目录）。⑪ 架构师的其他确认：同一路径对应同一行（不变）；`AddSources` 只收输入列表里的扩展名；GIF 留在视频类，别名追加 `图片`（`动图, 表情包, 动画, 图片`）；没有“打开原文件所在位置”操作；名字 `FFmpegFree` 不受 1.1 的用户文字规则限制；视频转图片取第 1 秒，取不到退回第一帧；复制被退出打断的副本下次启动是 `failed`（`reason=interrupted`），可以重试，不续传。⑫ **M4R（PM 9）**：只在前端显示灰色提示 `iPhone 铃声最长 40 秒，更长的文件可能不能设为铃声。`；不阻止、不截短，也不引导用户去裁剪；后端没有变化。⑬ **编码显示名兜底（6.14.5）**：表里没有的编码去掉 `lib` 前缀后**全部大写**（`foo` → `FOO`，`libfoo` → `FOO`），不再是首字母大写。⑭ **迁移编号**：`0007_storage_copies.sql`（副本）、`0008_reconvert.sql`（重转；v0.24 原写 `0007_reconvert.sql`）。⑮ 实现取舍：格式目录的检测缓存键是“转换组件可执行文件路径 + 大小 + 修改时间”（换了组件自动失效）；检测命令失败时结果不缓存，**提交时也不因检测失败拦截**（只有确认不支持才 `UNSUPPORTED`）；视频转图片用 `-ss 1` 没有产出图片时**不论退出码**都退回第一帧（ffmpeg 7 越过结尾时以 234 退出，ffmpeg 6 正常退出但不写文件）；删副本时修改时间按 **2 秒容差**比较（文件系统的时间精度）；`DeleteSource` 遇到**被多行共享、正在复制**的副本时不取消复制，只解除这一行的引用（6.15.7 第 1 步）；图片格式的 `paramsSummary` 不写编码段（视频容器去掉画面仍写 `无画面`）。

v0.23.5 变更（**剪辑功能整体移除**，老板定 2026-10-08，应用只做转换；文档转 PDF、录屏、推流保留；**没有新增接口、错误码或事件，没有迁移**）：① **删除 `EditService` 绑定**及其全部方法（`ValidateProject`、`Export`、`GetPreviewURL`、`SaveProject`、`LoadProject`、`ListProjects`、`DeleteProject`），后端的剪辑服务、工程存储代码、`edit_export` 的导出 Runner 和重试工厂、`models.edit` 生成类型、`frontend/wailsjs/go/app/EditService.*` 一并删除；`/local/<token>` 的 `edit` 登记表随之删除（6.13 只剩 `doc`、`convert` 两张表）。② **保留**共用的部分：`MediaService`（含 `Thumbnail`、`Probe`、`ListRecent`）、`localassets`、转换页的预览 / 缩略图后端都不变。③ **旧的 `edit_export` 任务记录**：不是“旧类型”（不按 `NOT_FOUND` 处理），`TaskService.List`（含任务中心）照常返回，`TaskService.Remove` 照常可以移除（`deleteOutput` 规则同 6.6）；**`Retry` 不论什么状态都返回 `UNSUPPORTED`**，message `剪辑功能已移除，剪辑导出记录不能重试`，`detail` 第一行 `reason=feature_removed`；启动时没跑完的旧导出照常变成 `interrupted`（6.6），没有 Runner，不会被重新执行；应用里不再有任何入口产生 `edit_export` 任务。④ **数据库不动**：`edit_projects` 表和里面的工程数据原样保留（不做删表这类破坏性迁移），只是没有代码再读写它；降级到旧版本仍能看到这些工程。⑤ 可重试类型改为 `convert`、`office_pdf`、`ffmpeg_install`（6.6）；无空格的重名格式 `a(1).mp4` 现在只用于文档转 PDF（6.14.5）。⑥ 6.11 整节作废，只作历史记录保留；文中其他地方（含 v0.24 各节）出现的 `EditService` / 剪辑导出同样只是历史说明，以本条为准。

v0.23.4 变更（转换页 v2 走查 1d109f9 的后端修复，**是 v0.23 实现的修复，在 v0.24 契约之后合入、与 v0.24 的实现并行**，随实现 PR；**没有新增接口、错误码或事件**）：① **源文件行持久化完整的媒体信息**（走查 G3）：迁移 **`0006_convert_source_media.sql`**（**占用了 0006，v0.24 的 `storage_copies` 迁移顺延为 `0007`**，见 6.15.3） 给 `convert_sources` 加 `media`（`MediaInfo` 的 JSON）和 `media_fp`（探测时文件的指纹 `<大小>:<修改时间纳秒>`）两列；`ConvertSource.media` 优先用这份持久化结果，**`hasVideo` / `hasAudio` / `sampleRate` / `channels` / `container` / `fps` / `rotation` / `streams` 都可靠、重启后仍在**（此前关联 `media` 表，`hasVideo` / `hasAudio` 恒为 `false`、没有采样率和声道，重启后音频行和无声视频的“没有声音”就丢了）；写入时机：`AddSources`（**改为顺带探测**，同一次调用里同一行只探测一次，最多 4 个并发、单个 15 秒；探测失败照样有行，`media` 省略）、`MediaService.Probe` 成功时（刷新同一 `path_key` 的行）、`ListSources` / `SearchSources` / `GetSource` / `GetSourcePreviewURL` 发现文件指纹和持久化的不一致（旧行从没探测过，或文件被替换）且文件还在时懒探测补上；文件不变就不重探（探测失败也记下指纹，不反复探测）；文件不在时保留上次的结果；没有持久化结果时仍退回关联 `media` 表（这时 `hasVideo` / `hasAudio` 按编码是否为空推出）。前端可以直接用 `source.media.hasVideo` / `hasAudio` 显示“没有声音”和做冲突预检，不必为此再调 `Probe`。详见 6.14.2、6.14.3、6.14.8。② **应用内预览再按编码挡一层**（走查 G4）：`ConvertService.GetSourcePreviewURL`、`TaskService.GetPreviewURL` 在扩展名白名单之后，按探测出的编码判断：有画面的文件看第一条视频流（ProRes、DNxHD、CineForm、FFV1、HAP、QuickTime RLE、无压缩、Ut Video、MPEG-1/2、MS-MPEG4 / WMV / VC-1、MJPEG 等 WebView 解不了的编码），只有声音的文件看第一条音频流（WMA、APE、AMR、AC-3 / E-AC-3、DTS、TrueHD、MP1 / MP2 等）；命中返回 `UNSUPPORTED`（`reason=format`，前端已按“无法在应用内播放”处理）；探测不到（转换组件没就绪、文件解析失败）时不挡。名单见 6.14.7。③ **`MediaInfo` 新增 `videoCodecName` / `audioCodecName`**（走查 X3）：后端按全应用唯一的一张编码显示名表生成（`ffv1` → `FFV1`、`dnxhd` → `DNxHD`、`mjpeg` → `MJPEG`、`pcm_s16le` → `PCM`、`ac3` → `AC-3`……，与 `paramsSummary` 同一张表），编码为空时省略；前端显示编码名请用它，不再自己首字母大写（“Ffv1”就是前端兜底规则写出来的）。④ 文件读不了时的 `message` 改为 **`无法读取文件“<文件名>”，可能没有读取权限。`**（走查 X8；`IO_ERROR`，`detail` 仍是系统错误，提交时第一行加文件路径），`MediaService.Probe` 和提交转换共用这一句；提交语义不变（任一源文件读不了仍整体不提交）。

v0.24 变更（存储位置、源文件副本、格式目录、面向用户的文字；**只有契约，前后端按本版并行实现**；完整规则见新增的 **1.1、6.15、6.16、6.17**，本条只是索引）：① **面向用户的文字不出现 “ffmpeg”**（1.1，老板定）：错误的 `message`、`DeleteFailure.message`、复制错误、格式目录的不可用原因、任务标题、设置 / 状态文案一律说“**转换组件**”（如 `当前转换组件不支持输出这个格式`、`转换组件已就绪`）；代码标识符、接口名、事件名、日志、只给开发者看的 `detail` 不受限；本文档里给用户看的示例文案已一并改掉（2、2.2、9.x、LiveService）；② **存储位置**（6.15.1~6.15.2）：程序所在文件夹下建 `output`（转换结果）和 `uploads`（添加的源文件副本）；程序所在文件夹不可写（如 Program Files）时**两个一起**改用用户数据目录；macOS 在 `.app` 包里时直接用用户数据目录；默认输出目录从“与源文件同一个文件夹”改为 `<base>/output`（转换、Office 转 PDF 的 `outputDir` 传空时都用它）；`Settings` 新增 `uploadsDir`，`defaultOutputDir` 的空值含义改变；新增 `SystemService.GetStorageDirs()`（实际路径 + 是否回退）、`SetStorageDirs(StorageDirsUpdate)`、`OpenStorageFolder(kind)`；**改目录只影响之后的新文件，已有文件不搬，旧记录仍指向原位置**；③ **源文件副本**（6.15.3~6.15.8）：`AddSources` 之后后端在后台把源文件复制到 `uploads/<sourceId>/<原文件名>`，**转换只读副本**；`ConvertSource` 新增 `originalPath`、`storedPath`、`copyState`（`none` / `copying` / `ready` / `failed` / `canceled`）、`copiedBytes`、`totalBytes`、`copyError`；新事件 `convert:copy`（进度与结束）；新增 `ConvertService.CancelCopy(sourceId)`、`RetryCopy(sourceId)`；复制前检查磁盘空间（`CONVERT_DISK_FULL`，`reason=no_space`，`detail` 带 `needBytes` / `freeBytes`，message `磁盘空间不足，需要 X，剩余 Y。`）；`SubmitSources` 遇到还没复制好的行**跳过它们、只提交已就绪的**，返回值改为 `ConvertSubmitResult{tasks, skipped[]}`（**签名变化**），一个都没就绪时才返回 `TASK_CONFLICT`（`reason=copying` / `copy_failed`）；复制中的行不能预览（`GetSourcePreviewURL` 返回 `TASK_CONFLICT`，`reason=copying`）；取消复制的行保留、显示“已取消复制”，`RetryCopy` 可以从 `failed` 或 `canceled` 重来；同一个原文件（**原路径、大小、修改时间都相同**，不算哈希）复用同一份副本；新表 `convert_copies`（`ref_count` 记被几行引用）与迁移 **`0007_storage_copies.sql`**；`DeleteSource` 同时删这一行的副本（引用计数归零才删，**永远不删用户的原文件**）；v0.24 之前的旧行（`copyState=none`）继续读原路径，不补做副本；`ListSources` / `SearchSources` 的 `status=active` 把正在复制的行算进去，`status=failed` 把复制失败的行算进去；④ **格式目录**（6.16）：新增 `ConvertService.GetFormatCatalog()`：视频 / 音频 / 图片三类共 34 种输出格式（新增视频 AVI 默认参数、WMV、MPG、VOB、3GP、SWF、OGV，音频 WMA、AMR、M4R、MP2、APE、WV、Opus 正式入目录、MMF，图片 JPG、PNG（PM 已确认）、WebP、ICO、BMP、TIF、TGA），每项带 `category`、`extension`、`displayName`、`aliases`（中文搜索词）、`encodable`（按当前转换组件真实的 muxer / encoder 检测并缓存，不写死）、不可用时的 `reason` / `reasonCode`、**这个格式可用的预设 `presets[]` 和默认预设 `defaultPresetId`**（预设按格式归到“参数”下拉里；每个格式都有一个内置默认预设，内置预设从 12 个增加到 38 个）；`ConvertOptions` 的容器和编码取值随之扩充；图片输出：图片输入出图片，视频输入**取第 1 秒那一帧，不足 1 秒取第一帧**（UI 设计定），不做序列帧；不可输出的格式照样列出，提交时 `UNSUPPORTED`（`reason=format`），所选编码缺编码器时 `reason=encoder`（2.2 新值）；**模糊搜索只在前端做**；⑤ 6.14.7 预览 / 系统打开白名单按新格式扩充，**AVI、FLV 移出应用内预览**（WebView 播不了，改为“用系统程序打开”），新增可预览的 `opus`、`ogv`、`m4r`、`webp`、`bmp`、`ico`、`jpg`、`png`；APE 等只能读不能写的格式可以作为输入；⑥ **成功任务的结果警告**（走查 X1）：`TaskResult` 新增 `warnings []string`（机器码），第一个是 `short_output`（输出时长 < 预期的 90% 且短 2 秒以上，预期 = 输入时长 − 裁剪），记录仍是成功，前端显示徽标“时长偏短”、悬停“转换结果比原文件短很多，原文件可能已损坏，请预览检查。”（PM 已定），阈值由后端定义，见 6.14.6；⑦ **`Reconvert` 改为在同一条记录上原地重转（老板定，细节由 PM / 前端 / 设计定）**：不加新方法，参数改为 `ReconvertRequest{taskId, presetId?, options?}`（不给就沿用原参数快照，给了必须同格式，否则 `INVALID_ARGUMENT` `reason=format_change`）。只用于 `succeeded`，否则 `TASK_CONFLICT` `reason=invalid_state`。同一任务 id、同一输出路径和文件名（不加 “(1)”）。重转中 `reconverting=true`、显示“重转中”和进度，旧 `result` 和旧文件照常可预览 / 打开 / 显示所在文件夹。新结果写同目录临时文件 `<名>.reconvert-<taskId>.part.<扩展名>`，**成功后原子替换**并换上新的 `result` / 参数快照 / `finishedAt`。**失败或取消都恢复成 `succeeded`**，旧结果、参数快照、`finishedAt` 原样恢复，旧文件不动；失败写 `lastReconvertError{code, message, detail, at}`（前端显示“重转失败，原来的文件没有变动。”），取消不写；终态事件带 `reconvertOutcome` 区分两者。临时文件在每条路径上都清理，启动时做崩溃恢复和孤儿扫描。重转中删除先取消再删。旧输出被移动或替换时拒绝（`TASK_CONFLICT` `reason=output_moved`，界面不给入口；旧输出不在了按 v0.24.1 ④ 用原参数重新生成），源文件不在时 `NOT_FOUND` `reason=file`（界面置灰）；`TaskPathCheck` 新增 `reconvertMode` / `reconvertBlock`（v0.24.1 定名，取代草稿的 `canReconvert`）供前端提前判断；重转中删除：先取消、删临时文件，`deleteOutputs=true` 时删旧输出。新迁移 `0008_reconvert.sql`（v0.24.1 改号）。转换页在成功记录上提供入口；任务中心没有入口，`List` 显示状态。见新增的 **6.17**。**没有新增错误码**（2.1 仍是 18 个），新增的是 2.2 的 `reason=` 取值。

v0.23.3 变更（没有新增接口、错误码或事件）：**删除失败后“打开所在文件夹”在任何位置都能用**。`DeleteRecords` / `DeleteSource` 返回的 `failures` 里 `path` 非空的项（`in_use` / `permission` / `not_task_output` / `io`），在**同一次运行里、该删除调用之后 10 分钟内**可以用 `RevealInFolder(path)` 打开所在文件夹，不受 `defaultOutputDir` 范围限制（6.8 第 3 类放行路径）。后端只把**这条记录登记的输出路径本身**放进内存里的放行表（按 Clean 后的路径精确匹配，大小写规则同 6.8；符号链接不算；最多保留最新的 100 个；不落库，重启即清空）；`not_task_output` 也一样——删除只尝试记录登记的输出，所以它的 `path` 就是登记的输出路径，绝不放行任意路径。其他路径的规则不变。6.14.1 的例外说明随之更新（去掉“前端静默忽略 `INVALID_ARGUMENT`”）。

v0.23.2 变更（转换页 v2 的小补充，随实现 PR；没有新增错误码、没有新增事件）：① **`ConvertSearchFilter` 新增可选 `status`**，取值和规则与 `ConvertSourceFilter.status` **完全相同**（`""` / `"active"` / `"failed"`，`canceled` 不算失败，其他值 `INVALID_ARGUMENT`；只筛行，不筛每行内嵌的记录和 `recordCount`），与关键字是 **AND**，分页和排序不变，复用同一个 `EXISTS` 子查询（6.14.2、6.14.3）；② **6.14.1“只收 id”规则的唯一例外**：`DeleteRecords` / `DeleteSource` 返回 `failures` 后，转换页可以用第一条 `path` 非空的失败项的 `path` 调旧的 `RevealInFolder(path)`，打开没删掉的文件所在的文件夹（记录已经删了，没有 id 可用；`RevealInFolder` 只打开文件夹，不读不改文件）；**`DeleteFailure.path` 只在文件类失败（`in_use` / `permission` / `not_task_output` / `io`）时有值，`still_running` 时为空（JSON 里省略）**，代码与此一致；③ **`paramsSummary` 的视频编码显示名统一写法**：`H.265`、`ProRes`、`AV1` 等，不再出现原样大写的 `HEVC` / `PRORES`（6.14.5）。

v0.23.1 变更（转换页 v2 的补充，**随实现 PR 一起改**；没有新增错误码、没有新增事件）：① **新增 `ConvertService.RevealRecord(taskId string) error`**：在系统文件管理器里显示转换记录的输出文件（“打开所在文件夹”），路径由后端从记录取，平台命令复用 `RevealInFolder` 的实现（Windows 同样走 `reveal_windows.go`），不走 6.8 的范围白名单；不存在 / 旧类型 `NOT_FOUND`（`reason=record`），不是 `convert` `INVALID_ARGUMENT`，没有输出（不是 `succeeded`）/ 输出文件不在 / 不是普通文件 / 是符号链接 `NOT_FOUND`（`reason=file`）。**转换页不再用 `RevealInFolder(path)`**（源文件用 `RevealSource`，记录用 `RevealRecord`），`RevealInFolder` 保留给其他页面（6.8）；② **新增 `ConvertService.GetSource(sourceId string) (ConvertSourceEntry, error)`**：返回一行，与 `ListSources` 的一项**完全相同**（同样内嵌最新 20 条记录——即 `recordLimit` 的默认值——和 `recordCount`，进行中的带实时进度）；不存在 `NOT_FOUND`（`reason=record`）。用于任务中心的“在转换页查看”：前端拿任务的 `sourceId` 调它，把这一行置顶、展开并高亮那条记录；③ **`paramsSummary` 不再含容器名**（容器由预设名或输出扩展名体现，避免出现“MP4 1080p · MP4 · H.264 · 1080p”），只列视频编码、尺寸、帧率、码率等；**只给宽度时** 3840 / 2560 / 1920 / 1280 / 854 显示成 `2160p` / `1440p` / `1080p` / `720p` / `480p`，其他宽度仍是 `宽 N`，宽高都给仍是 `宽×高`；没有任何段时为 `默认参数`（永远非空）。内置 1080p / 720p 预设只设了宽度，现在显示 `H.264 · 1080p` / `H.264 · 720p`（6.14.5）；④ **三个快照字段的取值规则写死**（6.14.2）：内置预设（以及用户预设）提交的记录 `presetId`、`presetName` 非空、`paramsSummary` 有值；自定义参数提交的记录 `presetId`、`presetName` 都是 `""`、`paramsSummary` 有值；v0.23 之前的旧记录三个键都没有（按 `""` 处理），前端退回显示 `title`。**没有 `presetId: "custom"` 之类的特殊值**；⑤ 产品决定（后端不变）：`Reconvert` 在 v1 转换页**没有入口**（接口保留）；任务中心按钮改名为“**隐藏已结束**”（原“隐藏已完成”，行为不变；契约里的旧称一并改掉）；⑥ **`ListSources` 的 `ConvertSourceFilter` 新增可选 `status`**：`""`（全部，同前）/ `"active"`（至少有一条 `queued` / `running` 记录的行）/ `"failed"`（至少有一条 `failed` / `interrupted` 记录的行，`canceled` 不算），其他值 `INVALID_ARGUMENT`；只筛行，分页、排序不变，每行内嵌的最新记录和 `recordCount` **不按状态筛**；迁移 `0005`（尚未发布，直接改）加索引 `idx_tasks_source_status(source_id, status)`（6.14.3、6.14.8）；⑦ 6.14.12 记录实现 PR 对 v0.23 条文中含糊处的取舍。

v0.23 变更（转换页 v2：转换记录、原地重试、任务中心隐藏；**只有契约，实现另开 PR**；完整规则见新增的 **6.14**）：① 新表 `convert_sources`（源文件行，主键即 `sourceId`）与迁移 **`0005_convert_records.sql`**：`tasks` 新增 `source_id`（可空）、`hidden_in_task_center`（默认 0）、`result`（JSON，可空）、`output_name_key`（默认 `''`）和对应索引，旧任务由启动时的 Go 回填 `store.BackfillConvertSources` 按规范化路径建行并写 `source_id`（幂等，不丢记录）；② `Task` 新增 `sourceId`、`hiddenInTaskCenter`、`result{sizeBytes,durationSec,width,height,audioBitrateKbps}`；convert 的 `params` 新增 `presetId`、`presetName`、`paramsSummary` 三个提交时快照；③ **`canceled` 的 `progress` 保留取消那一刻的值**（不清零，含义写进第 3 节和 6.14.6），终态 `task:status` 带 `progress`，成功的 convert 任务带 `result`；④ **`TaskService.Retry` 改为原地重试，统一覆盖所有可重试的类型**（`convert`、`edit_export`、`office_pdf`、`ffmpeg_install`），允许 `failed` / `interrupted` / `canceled`，**已成功的任务调 `Retry` 返回 `TASK_CONFLICT`**：复用任务 id、`createdAt`、原输出名（被占时顺延），先删失败留下的 `.part`，重置运行字段（清单见 6.6，**不含 `params`，即不动 `presetName` / `paramsSummary`**），`version` +1，只发一次 `task:status`（`queued`，`retried: true`），不发 `task:created`；6.6 的 `NeverRanner` 说明同步更新；⑤ 任务中心：新增 `HideFinishedInTaskCenter`（**所有任务类型都只隐藏、不删除，不按类型区分**）和 `UnhideInTaskCenter(ids)`（清 `hiddenInTaskCenter`、`version` +1、发 `task:status`，幂等；“显示已隐藏”开关打开时用 `includeHidden=true` 列出并把隐藏行置灰，用法见 6.14.11），`ClearFinished` 废弃并改成同样只隐藏；`TaskFilter` 新增 `includeHidden`，`List` 默认不返回已隐藏的任务；**真删**：转换记录只在转换页（`DeleteRecords` / `DeleteSource`），非转换任务用任务中心每行的“移除”（`Remove`），`Remove` 遇到 `convert` 任务整体 `INVALID_ARGUMENT`；⑥ ConvertService 新增 `AddSources`、`ListSources`、`ListSourceRecords`、`SearchSources`、`CheckSources`、`PreviewOutputName`、`SubmitSources`、`Reconvert`、`DeleteRecords`、`DeleteSource`、`GetSourcePreviewURL`、`OpenSourceWithSystem`、`RevealSource`、`GetRecordThumbnail`、`GetSourceThumbnail`（缩略图，返回与 `MediaService.Thumbnail` 的 `dataUrl` 相同的 `data:image/jpeg;base64,...`，首次调用时生成并缓存，见 6.14.10）；TaskService 新增 `CheckPaths`、`GetPreviewURL(taskId, which)`、`OpenWithSystem(taskId, which)`、`UnhideInTaskCenter`；这些接口**只收任务 id / `sourceId`，不收路径**；**`Reconvert` 只用于已成功的记录**（新增一条，相当于“又转了一次”），对非成功记录返回 `TASK_CONFLICT`；失败 / 中断 / 已取消的记录一律走原地 `Retry`（同一个 id、同一个输出名；已取消那一行的按钮文案仍叫“重新转换”，调的是 `Retry`）（**产品经理已确认**）；⑦ 转换任务的输出名**在提交时定名并占位**（排队中的任务也占名字；当前代码只在开始运行时占位，实现须改）；**重名后缀分两种格式：只有 `convert` 用带空格的 `a (1).mp4`、`a (2).mp4`，剪辑导出、文档转 PDF、`.part` 遗留清理等其他地方保持现有的 `a(1).mp4`**（见 6.14.5）；⑧ 6.13 新增 `convert` 登记表，转换页预览白名单在 v1 列表上加 `gif`（`EditService` 不变）；⑨ **没有新增错误码**：2.2 新增 `NOT_FOUND` 的 `reason=record|file|no_app` 和 `UNSUPPORTED` 的 `reason=format`（只用于 6.14 的接口），删除结果的固定文案见 6.14.4。

v0.22 变更（走查包 12 的后端小修，**接口签名、事件、错误码都没有变**；只有 ① 一个 JSON 字段从“false 时缺失”改成“始终输出”，其余是用户可见文案和版本号显示）：① `MediaInfo.hasVideo` / `hasAudio` 去掉 `omitempty`，**始终输出** `true` / `false`（此前 false 时字段缺失，前端 `=== false` 判断永远不成立，纯音频配视频预设、无声视频配音频预设时不标“冲突”）；`ListRecent` 返回的记录里也是 `false`（不入库，同其他 Probe 字段）；② 任务日志里 FFmpegFree 自己写的显卡编码回退行 `[FFmpegFree] 硬件编码器 h264_nvenc 启动失败（nvenc_init_failed），改用 CPU 编码重试一次` → `[FFmpegFree] 显卡编码启动失败，已自动改用 CPU 重试一次`（不带编码器名；ffmpeg 自己的 stderr 原样保留）；③ `EncoderDevice.reason` / `EncoderPreferenceInfo.reason` 里 FFmpegFree 自己写的文案统一叫“显卡编码”、不带编码器名：`当前 ffmpeg 不包含 NVENC / QSV / AMF / VideoToolbox 编码器` → `当前 ffmpeg 不包含这张显卡对应的显卡编码支持`，`没有可用的硬件编码器` → `没有可用的显卡编码器`，`这张显卡没有对应的硬件编码器支持` → `这张显卡没有对应的显卡编码支持`，`系统没有响应 VideoToolbox 编码` → `系统没有响应显卡编码`，`（Quick Sync 初始化失败）` / `（AMF 初始化失败）` → `（显卡编码初始化失败）`（`试跑失败：<ffmpeg 输出最后一行>` 仍是 ffmpeg 自己的输出，原样）；④ `FFmpegStatus.version` 规范化：只取开头的数字版本，网址 / 构建信息不进这个字段：`9.0.2-https://www.martin-riedl.de` → `9.0.2`，`n7.1` → `7.1`，`7.1-static` → `7.1`，`6.1.1-3ubuntu5` → `6.1.1`；`N-12345-gabcdef` 这类没有数字版本的 git 构建保持原样，日期版去掉网址并最多保留 32 个字符（见第 9 节 `FFmpegStatus`）；主版本判断（≥6）不受影响；⑤ 没有更具体分类的 ffmpeg 非零退出（`PROCESS_FAILED`）的 `message` 从 `ffmpeg 异常退出（退出码 N）`（个别情况 `ffmpeg 执行失败`）改成 `转换被意外中断，可以重试；如果反复出现，请查看日志。`（全角标点，不含退出码）；退出码放进 `detail` 第一行 `ffmpeg 退出码 N`（-1 通常是进程被外部结束或启动后立即崩溃），后面接 stderr 最后 50 行，任务日志里也会有一行 `[FFmpegFree] ffmpeg 退出码 N`。错误码 `PROCESS_FAILED` 不变；直播任务认不出的退出仍是 `INTERNAL`（message `推流异常退出`，没有动）。

v0.21 变更（只修 Windows 上“打开输出位置”点了没反应，**接口、字段、错误码、路径白名单都没有变**；见 6.8 节）：Windows 不再用 `proc.Configure`（它设的隐藏窗口标志会被 explorer 沿用，进程启动了但窗口不显示），改为手拼命令行：文件 `explorer.exe /select,"<path>"`，文件夹 `explorer.exe "<path>"`，路径始终带双引号，含空格和中文都能识别；Windows 路径里含双引号直接 `INVALID_ARGUMENT`。其他平台不变。窗口是否真的弹出只能 Windows 真机验证。

v0.20 变更（只改设备名兜底文案，**接口、字段、错误码都没有变**；见 9.6 第 5 步）：`EncoderDevice.name` 在读不到具体显卡型号时的兜底名统一改成中文短名，界面可直接显示、不再带编码器名或驱动名：`NVIDIA GPU` → `NVIDIA 显卡`，`Intel GPU` → `Intel 显卡`，`AMD GPU` → `AMD 显卡`，`Apple GPU` / macOS 的 `Apple VideoToolbox（系统硬件编码）` → `系统显卡`，Linux 只有 sysfs 时的 `Intel GPU（i915）` / `NVIDIA GPU（nvidia）` / `AMD GPU（amdgpu）` → 同上不带驱动名的 `Intel 显卡` / `NVIDIA 显卡` / `AMD 显卡`；能读到真实型号时不变。`EncoderDevice.reason` 仍可能含 NVENC / QSV / AMF 等技术词（v0.15 起就说明“界面不必直接显示”），本次不改。

v0.19 变更（只补契约文字，**接口、字段、错误码、行为都没有变**）：① 6.6 新增 `task.NeverRanner`（任务从未真正开始执行就结束）的说明，写清触发场景、状态 / 事件 / 字段表现，以及与 v0.18 编码器字段的关系——**这类任务的 `encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 保留提交时写入的值，不会为空**（见 6.6 与 9.7）；② 第 6 节 `tasks` 表列清单补上 v0.18 迁移 `0004` 新增的四列（`encoder`、`encoder_device`、`hw_fallback`、`hw_fallback_reason`，v0.18 漏写）；③ 9.7 补一句：`hwFallbackReason` 枚举在 Go 常量（`internal/ffmpeg/hwenc.go`）、本契约、前端 `taskTypes.ts` 三处一致，并新增测试锁住这个一致性；前端 `errors/encoderMessages.ts` 现有回退文案按功能区分（转换 / 直播 / 任务行），不按原因区分。

v0.18 变更（硬件编码接入 ConvertService / EditService / LiveService，见新增的 9.7；**契约按架构师口头方案起草，如有出入以架构师为准**）：`Task` 和 `task:progress` / `task:status` 事件新增四个可选字段 `encoder`（string）、`encoderDevice`（string）、`hwFallback`（bool）、`hwFallbackReason`（string），全部 `omitempty`，没有视频编码的任务不带；`tasks` 表新增迁移 `0004_task_encoder.sql`（四列，旧行为空）；`ResolveEncoder` 的结果现在真正用于转换 / 剪辑导出 / 直播的 H.264、H.265 重编码（NVENC / QSV / AMF / VideoToolbox），`-c copy`、VP9 / GIF / 音频、按目标大小的两遍编码一律 CPU；硬件编码启动失败自动用 CPU 重试一次（`hwFallback`），取消不回退，直播只在推流建立前回退；**没有新增接口方法、没有新增错误码**；`Task.params` 不变。9.6 末段“本版不接入”作废。

v0.17 变更（LiveService 推流 / 拉流真实预览画面，见第 4 节 LiveService 和 6.10「预览画面」）：新增 `LiveService.GetPreview(sessionId) (Preview, error)`（最新一帧 base64 JPEG + 毫秒时间戳，没有画面返回空、不是错误）、`StartPullPreview(PullPreviewRequest) (PullSession, error)` / `StopPullPreview(sessionId) error`（拉流预览会话：后端 ffmpeg 读远端流只出预览，播放仍由前端播放器直接拉地址）；`FilePushRequest` / `ScreenPushRequest` 新增可选字段 `preview`（`*bool`，缺省 = true，false = 不加预览输出）；预览输出是主输出之外**独立**的一路 image2 输出（`fps=2,scale=640:-2`、`-q:v 5`、`-update 1`、`-atomic_writing 1`），不放进 tee；临时文件放 `<数据目录>/tmp/live-preview/<会话 id>.jpg`，会话结束清理，应用启动清空该目录。新增类型 `Preview`、`PullPreviewRequest`、`PullSession`；不新增错误码、不新增事件。**有硬字幕 / 视频复制（`-c copy`）的推流场景预览输出需要单独解码（额外占少量 CPU）**；当前直播主输出始终重编码，预览输出复用同一路解码结果不增加解码次数，见 6.10「预览画面」。

v0.16 变更（DocService 错误 detail 首行统一为 `reason=`，并更正 6.12.3 的措辞，只改契约的说明，接口和错误码不变，见 2.2 和 6.12.6）：**架构师定**：Doc（Office 转 PDF、PDF 预览 / 打开）面向前端的“文件本身有问题”类错误，`detail` 首行严格是 `reason=<枚举>`，枚举固定为 `too_many_pages`、`format`、`encrypted`、`no_font`、`invalid_ooxml`、`too_large`（只追加），其后可以保留原来的自由文本行（`ConvertToPDF` 整体校验失败时，出错文件路径在 reason 行**之后**，即第二行）；**`code` 和 `message` 不变**（`message` 是给用户看的短句，前端精确匹配它，精确文案列在 6.12.6 的表里）。**更正**：6.12.3 原文写 detail "超过 5000 页"，实现里"超过 5000 页"是 `message`，detail 首行是 `reason=too_many_pages`；同类的 "不是有效的 OOXML 文件" "暂不支持这种格式" "没有可用的 Unicode 字体" 也都是 `message`，不是 detail。取消、磁盘满、读写失败、路径 / 参数 / 句柄类错误**没有 reason**（6.12.6 明确列出）。实现：`internal/service/doc` 的 `reasonErr`；2.2 新增 Doc 行。

v0.15 变更（SystemService 硬件编码器检测与偏好，见第 4 节 SystemService 和 9.6；**契约按架构师口头方案起草，如有出入以架构师为准**）：新增 `ListEncoderDevices()`（返回 `EncoderDeviceList{ffmpegReady, devices[]}`，第一项永远是 CPU）、`RefreshEncoderDevices()`、`GetEncoderPreference()`（`"auto" | "cpu" | 设备 id`，默认 `"auto"`）、`GetEncoderPreferenceInfo()`（`{id, name, available, reason?}`，设置页显示「自动 / CPU / 具体显卡名」用）、`SetEncoderPreference(id)`；新增设置键 `encoderPreference`、`encoderPreferenceName`；新增纯函数 `ResolveEncoder(pref, devices, codec)`（Go 内部，不是绑定方法）。检测 = `ffmpeg -encoders` + 逐个硬件编码器实际试跑一帧（5 秒超时）+ 显卡名称枚举；结果缓存，ffmpeg 变为 ready 时失效。**本版只做检测、偏好和解析函数，转换 / 剪辑 / 直播的编码参数暂不使用它（下一版接入）**。无新增错误码。

v0.14 变更（LiveService 屏幕推流可选采集来源，见第 4 节 LiveService 和 6.10「采集来源」）：新增 `LiveService.ListCaptureSources() ([]CaptureSource, error)`；`ScreenPushRequest` 新增可选字段 `captureSourceId`（不传 = 原行为，向后兼容）；新增错误码 `LIVE_SOURCE_GONE`（`internal/apperr` 现在 18 个码，第 2.1 节清单同步），`detail` 第一行 `kind=window|screen`（2.2 表新增一行）。新增类型 `CaptureSource`。`ScreenInfo` / `ListScreens` / `GetCaptureCapabilities` 不变。

v0.13 变更（EditService 素材上限，随实现回改的小修订，见 6.11 节）：`EditProject.sources`（素材库）上限由 200 改为 100（产品经理定稿：素材 100）；`SaveProject` / `ValidateProject` / `Export` 超过 100 个返回 `INVALID_ARGUMENT`（message「素材库最多 100 个文件」，detail 第一行 `project`、第二行 `sources=<实际个数>`）。clip 总数（视频 + 音频）上限不变，仍是 100——**素材 100 / 片段 100 都是 100，是两个独立上限**。无接口签名变化。

v0.12 变更（DocService 契约定稿，**只有契约，尚无实现**，见 6.12 节）：`ConvertToPDF` 保持签名，格式范围如实收窄为 `docx` / `xlsx` / `pptx` **纯文本版**（与 v1 一致：无图片、表格线、样式；旧版 `doc` / `xls` / `ppt` 及其他格式一律 `UNSUPPORTED`）；`GetPDFURL` 替换为 `OpenPDF`（返回 `PDFSource`）+ `ReadPDFChunk`（分块读，走 Wails Bind，不依赖 AssetServer 行为）；新增 `GetDocCapabilities` / `ListRecentPDFs` / `RemoveRecentPDFs`；新增表 `doc_recent`；任务类型 `office_pdf` 保持不变；PDF 渲染、页数、缩略图、搜索全部在前端 pdf.js（`@tato30/vue-pdf`）完成，后端不渲染、不提供合并 / 拆分 / 旋转（v1 也没有）。

v0.11 变更（EditService 契约定稿，**只有契约，尚无实现**，见 6.11 节）：`Render` 改名 `Export`，任务类型 `edit_render` 改名 `edit_export`（旧名从未产生过任务，无迁移问题）；新增 `ValidateProject` / `DeleteProject` / `GetPreviewURL`；`SaveProject` 返回 `EditProjectMeta`，`LoadProject` 返回 `LoadedProject`；`EditProject` 字段与校验范围、导出参数、错误码、预览方案全部写死；预览走 AssetServer 的 `/local/<token>`（不做本地流服务，不用 `file://`），并明确 Windows 上 AssetServer 不支持流式响应、单次响应必须限长。

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
- 所有文件用**本地绝对路径**传递，选择文件用 `SystemService.PickFiles`，不经 WebView 上传。**v0.24**：转换页添加的源文件由后端在本机把它**复制**到上传目录，转换只读副本（6.15）；前端仍然只传绝对路径。
- 应用数据目录：`os.UserConfigDir()/FFmpegFree/`，下设 `app.db`、`thumbs/`、`logs/`、`bin/`、`tmp/`（不变）。**v0.24：输出目录和上传目录**默认是**程序所在文件夹**下的 `output`、`uploads`，程序所在文件夹不可写（或 macOS 的 `.app` 包）时两个一起改用应用数据目录下的 `output`、`uploads`；可在设置里分别改（6.15.1、6.15.2）。（v0.4 写的“系统视频目录下的 `FFmpegFree`”从未实现，作废；v0.7.4~v0.23 的实际默认是“与源文件同一个文件夹”，v0.24 起取消。）
- 前端预览本地文件：通过 Wails AssetServer 的 `Handler` 挂 `/local/<token>`，由后端按 token 映射真实路径，不暴露任意路径读取；协议、限长、Windows 限制、token 生命周期见 6.13。
- 时间一律 Unix 毫秒（int64），时长一律秒（float64），大小一律字节（int64）。
- ID 一律 ULID 字符串。
- 拖拽文件：前端直接使用 Wails 运行时（`OnFileDrop`）拿到绝对路径，后端不转发事件；前端不从 WebView 的 File 对象取路径。
- 路径入库前统一规范化：`filepath.Clean` + 转绝对路径；Windows 和 macOS 上额外转小写生成 `path_key` 做唯一约束，原始大小写保留在 `path` 用于显示。**库里的路径一律是绝对路径**（v0.24 写明：输出、副本、源文件都是），改设置目录或搬动程序文件夹不做迁移，找不到的文件按 `NOT_FOUND`（`reason=file`）处理（6.15.2 第 4 条）。

### 1.1 面向用户的文字（v0.24，老板定）

- **面向用户的文字里不出现 “ffmpeg”（不分大小写，含 `FFmpeg`、`ffprobe`）**，统一叫“**转换组件**”，按中文习惯组织句子，例如：`当前转换组件不支持输出这个格式`、`转换组件已就绪`、`未找到可用的转换组件，请先安装或手动指定转换组件所在位置`、`安装转换组件`（安装任务的标题）。
- **v0.29**：同样不出现 Whisper、模型文件名、Python；语音相关只叫「**语音识别组件**」（延后能力的「语音音色」「音色克隆组件」同理），见 6.18.1。
- **v0.30**：Cat 相关界面与 message 只出现「**Cat 助手**」，不出现 grok / CLI / 命令行 / 可执行文件名，见 6.19。
- **范围**（后端产生、会被前端原样显示的）：`AppError.message`；`detail` 里约定给用户看的行（目前没有：`reason=` / `scheme=` / `kind=` 是机器读的枚举，本身不含 ffmpeg）；`DeleteFailure.message`；`ConvertSource.copyError.message`；`FormatEntry.reason`；`EncoderDevice.reason` / `EncoderPreferenceInfo.reason`；任务 `title`；`FFmpegStatus.error.message`；预设名等后端生成、前端直接显示的名字。
- **不受限**：代码标识符、包名、接口名和方法名（`FFmpegStatus`、`InstallFFmpeg`、`ffmpeg:status`、`FFMPEG_NOT_FOUND`、`ffmpegPath` 等**全部不改**）、日志（任务日志、应用日志，含 `[FFmpegFree] ffmpeg 退出码 N`）、只给开发者看的 `detail`（如 `PROCESS_FAILED` 的 `ffmpeg 退出码 N` 和 stderr 尾部；`FFMPEG_NOT_FOUND` 的组件状态 detail 自 v0.25.4 起只有 `[来源] 原因`，不含路径）、ffmpeg 自己输出的原文（如 `试跑失败：<ffmpeg 输出最后一行>` 里引用的那一行）、文件路径。
- **产品名 `FFmpegFree` 不算违反**（它是应用名，例如 macOS 屏幕录制授权提示里必须写出应用名，用户才能在系统设置里找到它）；是否连产品名也要改，见 PR 的待定问题。
- 前端自己的文案同样遵守（前端负责，不在本契约里逐条列）；后端改了 `message` 不影响前端对 `code` / `reason` 的判断（前端从不按 `message` 分支，6.12.6 那几条精确匹配的 Doc 文案本来就不含 ffmpeg）。
- 新增文案时先查本条；测试里对 message 的断言同步改，并加一条测试扫描后端所有面向用户的固定文案不含 `ffmpeg` / `ffprobe`（不区分大小写）。
- **错误码和 `reason` 只给程序用（v0.25.3，架构师定）**：`code`（如 `INTERNAL`）和 `detail` 里的 `reason=` / `scheme=` / `kind=` 等枚举**界面上永远不显示**。前端按 `code`（需要时再看 2.2 的 `reason`）映射成中文；映射不到的一律显示 `出了点问题，请重试。`。后端的 `message` 是给用户看的中文：不含错误码，不夹英文单词或参数名（`limit`、`kind`、`atSec` 这类），不用 `%v` 把英文原文（Go 错误、panic）拼进去——这些放 `detail`。产品名、格式名、平台名（`FFmpegFree`、`PDF`、`JSON`、`Windows`、`macOS`、`X11`、`Office`、`ID`）可以出现。测试 `TestUserFacingMessageIsChinese`（`internal/apperr`）扫描所有 `apperr.New` / `Wrap` 的 message 字面量。


## 2. 错误约定

Bind 方法返回 `(T, error)`。error 的 message 是 JSON 字符串，前端 `api/` 层统一解析：

```json
{ "code": "FFMPEG_NOT_FOUND", "message": "未找到可用的转换组件，请先安装或手动指定转换组件所在位置", "detail": "..." }
```

| code | 含义 |
|---|---|
| INVALID_ARGUMENT | 参数不合法 |
| NOT_FOUND | 记录或文件不存在 |
| FFMPEG_NOT_FOUND | ffmpeg 缺失 |
| TASK_CONFLICT | 任务状态不允许该操作（如取消已完成任务）；直播（v0.10）：已有进行中的屏幕推流时再开一路屏幕推流（`detail` 首行 `reason=screen_busy`，**屏幕推流同一时间最多 1 路**），同一推流地址已有进行中的会话（`reason=duplicate_url`），或进行中的直播会话已达 4 个上限（`reason=max_sessions`）；判断顺序 `duplicate_url`、`screen_busy`、`max_sessions`（架构师定），稳定枚举见 2.2 与 6.10 |
| IO_ERROR | 读写文件失败 |
| PROBE_FAILED | 文件存在但 ffprobe 无法解析（损坏、不是音视频文件、没有可识别的流） |
| CANCELED | 调用因应用退出（根 ctx 取消）而被取消，结果作废；前端不需要提示用户（`ConvertService.Submit`、`LiveService.Start*` 等） |
| UNSUPPORTED | 该操作不支持这个对象（v0.24：提交了当前转换组件不能输出的格式是 `reason=format`，选了当前转换组件没有编码器的编码是 `reason=encoder`，见 6.16.6；如没有重试工厂的任务类型不能 Retry；直播会话 Retry 也是它；直播（v0.10）：本机 ffmpeg 缺少推流协议时 `Start*` 返回它，`detail` 是 `missing=<协议名>`（协议名取 `rtmp`、`rtmps`、`srt`，如 `missing=srt`），见 2.2） |
| CONVERT_DISK_FULL | 转换写输出文件时磁盘空间不足（前端标题「磁盘空间不足」，可引导用户换输出目录）；**v0.24**：复制源文件到上传目录之前检查空间不足、或复制时写满，也是它（`ConvertSource.copyError`，`detail` 首行 `reason=no_space`，后面两行 `needBytes=` / `freeBytes=`，见 2.2、6.15.4） |
| PROCESS_FAILED | 子进程非零退出，detail 第一行 `ffmpeg 退出码 N`（v0.22），后面是最后 50 行日志；message 不带退出码 |
| UNSUPPORTED_PLATFORM | 当前系统或会话不支持该功能（如 Linux Wayland 下的屏幕采集、Linux 没有 `DISPLAY`、x11grab 打不开显示） |
| LIVE_URL_INVALID | 推流地址格式不合法或协议不支持（只允许 rtmp / rtmps / srt，规则见第 4 节 LiveService）；`detail` 第一行 `reason=<值>`，见 2.2 |
| LIVE_CONNECT_FAILED | 推流**开始前**连接目标失败（DNS、拒绝连接、超时、网络不可达；SRT 的服务器未开与被拒绝无法区分，也归它）：任务在收到第一条 `task:progress` 之前就失败；`detail` 第一行 `scheme=rtmp\|rtmps\|srt`，见 2.2 |
| LIVE_PUSH_REJECTED | 目标服务器明确拒绝推流（RTMP 鉴权失败、流名冲突、握手被拒等；SRT 不会出现），**推流开始前** |
| LIVE_PUSH_INTERRUPTED | 推流**已经开始**（收到过 `task:progress`）后被目标服务器或网络中断 |
| SCREEN_PERMISSION_DENIED | 没有屏幕录制权限（macOS 系统授权），`StartScreenPush` 同步返回或任务失败 |
| LIVE_SOURCE_GONE | （v0.14）`StartScreenPush` 传了 `captureSourceId`，但所选来源此刻已不可用：窗口已关闭 / 已最小化 / 不可见，或屏幕序号不存在（显示器被拔掉）。同步返回（没有创建任务）；ffmpeg 打开窗口时才发现窗口没了（校验与打开之间的竞态）则是任务失败，码相同。`detail` 第一行 `kind=window` 或 `kind=screen`，**不带窗口标题**。前端提示「所选窗口已不可用，请重新选择」（屏幕：「所选屏幕已不可用，请重新选择」）并重新 `ListCaptureSources` |
| DOC_ENCRYPTED / DOC_CORRUPT / DOC_TIMEOUT / DOC_COMPONENT_CRASHED / DOC_COMPONENT_NOT_READY / DOC_DOWNLOAD_FAILED / DOC_CHECKSUM_FAILED / DOC_COMPONENT_INSTALL_FAILED / DOC_FORMAT_UNSUPPORTED / DOC_PDF_INPUT_UNSUPPORTED | v0.26 文档多格式转换，含义、message、是否可重试见 6.12.20 |
| DOC_PRESENTATION_BUSY / DOC_ENGINE_BUSY | v0.27 本机 Office / WPS 被占用（演示程序正在运行 / 只能挂到正在运行的实例上），可重试，见 6.12.33 |
| DOC_PDF_NO_TEXT | v0.28 PDF 转 txt / md / 简易 html 时取不出可用的文字（扫描件或乱码）且没有可用的文档组件，不可重试，见 6.12.63 |
| LANG_ASR_NOT_READY / LANG_ASR_EMPTY / LANG_ASR_FAILED / LANG_DOWNLOAD_FAILED / LANG_CHECKSUM_FAILED | v0.29 语音工具转字幕，见 6.18.8 |
| CAT_NOT_READY / CAT_REPLY_FAILED | v0.30 Cat 助手，见 6.19.7 |
| CAT_PROJECT_MISSING | v0.31 Cat 项目文件夹不见了（`SendCatMessage` / `CreateCatConversation` 同步返回；v0.31.1 加 `RevealCatProject`），可重试，见 6.19.10.9 |
| INTERNAL | 其他；直播任务里认不出的 ffmpeg 非零退出也是它（不是 `PROCESS_FAILED`），detail 带（已脱敏的）stderr 最后若干行 |

**直播 / 录屏（v0.10）用到的后端码正好是冻结的这八个：`LIVE_URL_INVALID`、`LIVE_CONNECT_FAILED`、`LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`SCREEN_PERMISSION_DENIED`、`FFMPEG_NOT_FOUND`、`UNSUPPORTED_PLATFORM`、`INTERNAL`**（v0.14 起再加 `LIVE_SOURCE_GONE`，共九个）；此外复用已有的 `INVALID_ARGUMENT`、`NOT_FOUND`、`PROBE_FAILED`（输入文件问题）、`TASK_CONFLICT`（会话上限 / 同地址冲突）、`UNSUPPORTED`（Retry）、`CANCELED`（`Start*` 因应用退出被取消，#10 已加）。**v0.10 没有新增任何错误码**，也没有 `LIVE_START_FAILED` 之类的同义码。用户主动停止不产生错误码（优雅停止成功 = `succeeded`，超时强杀 = `canceled` 状态，`error` 为空）。`LIVE_PLAY_FAILED`（播放器加载或解码失败）和 `LIVE_CORS_BLOCKED`（拉流地址跨域被浏览器拦截）**只在前端由播放器产生**，后端不会返回，也不在 `apperr` 里定义。

### 2.1 AppErrorCode 完整清单（供前端 `frontend/src/api/call.ts` 对照）

后端 `internal/apperr` 一共 39 个码（v0.14 新增 `LIVE_SOURCE_GONE`；v0.26 新增 10 个 `DOC_*`，文案和是否可重试见 6.12.20；v0.27 新增 `DOC_PRESENTATION_BUSY`、`DOC_ENGINE_BUSY`，见 6.12.33 和下表；v0.28 新增 `DOC_PDF_NO_TEXT`，见 6.12.63；v0.29 新增 5 个 `LANG_*`，见 6.18.8；v0.30 新增 2 个 `CAT_*`，见 6.19.7；v0.31 新增 `CAT_PROJECT_MISSING`，见 6.19.10.9），前端 `AppErrorCode` 必须全部包含；前端 `frontend/src/api/call.ts` 以本清单为准逐项核对补全（不在契约里写它当前缺几个，现状随前端分支变化）：

```ts
export type AppErrorCode =
  | 'INVALID_ARGUMENT' | 'NOT_FOUND' | 'FFMPEG_NOT_FOUND' | 'TASK_CONFLICT' | 'IO_ERROR'
  | 'PROBE_FAILED' | 'CANCELED' | 'UNSUPPORTED' | 'CONVERT_DISK_FULL' | 'PROCESS_FAILED'
  | 'UNSUPPORTED_PLATFORM' | 'INTERNAL'
  | 'LIVE_URL_INVALID' | 'LIVE_CONNECT_FAILED' | 'LIVE_PUSH_REJECTED' | 'LIVE_PUSH_INTERRUPTED'
  | 'SCREEN_PERMISSION_DENIED' | 'LIVE_SOURCE_GONE'
  // v0.26 文档转换（6.12.20）：
  | 'DOC_ENCRYPTED' | 'DOC_CORRUPT' | 'DOC_TIMEOUT' | 'DOC_COMPONENT_CRASHED' | 'DOC_COMPONENT_NOT_READY'
  | 'DOC_DOWNLOAD_FAILED' | 'DOC_CHECKSUM_FAILED' | 'DOC_COMPONENT_INSTALL_FAILED' | 'DOC_FORMAT_UNSUPPORTED' | 'DOC_PDF_INPUT_UNSUPPORTED'
  // v0.27 本机 Office / WPS（6.12.33）：
  | 'DOC_PRESENTATION_BUSY' | 'DOC_ENGINE_BUSY'
  // v0.28 PDF 输入（6.12.63）：
  | 'DOC_PDF_NO_TEXT'
  // v0.29 语音工具转字幕（6.18.8）：
  | 'LANG_ASR_NOT_READY' | 'LANG_ASR_EMPTY' | 'LANG_ASR_FAILED' | 'LANG_DOWNLOAD_FAILED' | 'LANG_CHECKSUM_FAILED'
  // v0.30 Cat 助手（6.19.7）：
  | 'CAT_NOT_READY' | 'CAT_REPLY_FAILED'
  // v0.31 Cat 项目（6.19.10.9）：
  | 'CAT_PROJECT_MISSING'
```

**文档类错误码能否重试（v0.27 汇总；文案以 6.12.20 / 6.12.33 为准）**：

| code | 能否重试 | 版本 |
|---|---|---|
| `DOC_ENCRYPTED` | 否（只能移出） | v0.26 |
| `DOC_CORRUPT` | 否（只能移出） | v0.26 |
| `DOC_TIMEOUT` | 是 | v0.26 |
| `DOC_COMPONENT_CRASHED` | 是 | v0.26 |
| `DOC_COMPONENT_NOT_READY` | 是（有引擎可用后） | v0.26 |
| `DOC_DOWNLOAD_FAILED` | 是 | v0.26 |
| `DOC_CHECKSUM_FAILED` | 是 | v0.26 |
| `DOC_COMPONENT_INSTALL_FAILED` | 是 | v0.26 |
| `DOC_FORMAT_UNSUPPORTED` | 否 | v0.26 |
| `DOC_PDF_INPUT_UNSUPPORTED` | 否（v0.28 起不再产生，保留给旧记录） | v0.26 |
| `DOC_PRESENTATION_BUSY` | **是**（任务保留；预览点「重试」） | v0.27 |
| `DOC_ENGINE_BUSY` | **是**（任务保留；预览点「重试」） | v0.27 |
| `DOC_PDF_NO_TEXT` | 否（装好文档组件后可以对这一行新提交） | v0.28 |
| `LANG_ASR_NOT_READY` | 是（组件就绪后） | v0.29 |
| `LANG_ASR_EMPTY` | 否（换文件再提交） | v0.29 |
| `LANG_ASR_FAILED` | 是 | v0.29 |
| `LANG_DOWNLOAD_FAILED` | 是 | v0.29 |
| `LANG_CHECKSUM_FAILED` | 是 | v0.29 |
| `CAT_NOT_READY` | 是（就绪后） | v0.30 |
| `CAT_REPLY_FAILED` | 是 | v0.30 |
| `CAT_PROJECT_MISSING` | 是（文件夹回来后） | v0.31 |

- 前端遇到不在清单里的 `code`：按 `INTERNAL` 的通用文案处理，不崩溃。
- `LIVE_PLAY_FAILED`、`LIVE_CORS_BLOCKED` 只在前端播放器里产生，**不是** `AppErrorCode`，也不出现在后端。
- 之后新增错误码：先改本表和 `internal/apperr`，再改前端；三处必须一致。

### 2.2 `detail` 第一行格式（按错误码分，稳定枚举）

| code（触发场景） | `detail` 第一行 | 取值（只追加，不改名、不改含义、不删除） | 说明 |
|---|---|---|---|
| `TASK_CONFLICT`（直播 `Start*` 的会话冲突） | `reason=<值>` | `screen_busy`（已有进行中的 `live_screen_push`，再开一路屏幕推流；文件推流不会得到它）、`duplicate_url`（同一标准化地址已有会话）、`max_sessions`（进行中的直播会话已达 4 个）；**判断顺序固定（架构师定）：`duplicate_url` → `screen_busy` → `max_sessions`**，同时满足多个条件时只返回最先命中的 | 其他 `TASK_CONFLICT`（`Cancel` 已结束的会话、`Remove` 进行中的任务等）**没有** `reason=` 行；不含任何地址、口令、streamkey |
| `LIVE_URL_INVALID` | `reason=<值>` | `scheme_unsupported`（scheme 不是 rtmp / rtmps / srt）、`malformed`（空串、超长、含非法字符、缺 scheme、端口越界或缺失、rtmp 缺应用名、SRT 参数值非法如 passphrase 长度、IPv6 括号错误等）、`missing_host`（host 为空）、`param_not_allowed`（SRT 查询参数不在白名单、`mode` 不是 `caller`、同名参数重复） | **不带地址、口令，也不带它们的任何片段**（连脱敏后的地址也不放），第二行起可以写不含地址的原因说明 |
| `UNSUPPORTED`（直播 `Start*` 时本机 ffmpeg 缺协议） | `missing=<协议名>` | `rtmp`、`rtmps`、`srt`（对应推流地址的 scheme；`rtmp` 是除 `rtmps` / `srt` 以外的默认）；带本地存档的会话另需 `tee`，缺时是 `missing=tee` | `detail` 只有这一行，没有第二行；`message` 是"当前转换组件不支持 <协议名>，请安装完整版转换组件"（v0.24 起不含 “ffmpeg”，1.1），前端据此提示安装完整版；其他原因的 `UNSUPPORTED`（如 `Retry` 直播任务、屏幕推流存档未实现）没有这一行 |
| `LIVE_CONNECT_FAILED` | `scheme=<值>` | `rtmp`、`rtmps`、`srt`（取自校验后的标准化地址，小写）；v0.25.1 起拉流预览的 `failed` 也用，另有 `http`、`https`（6.10.3.7） | 第二行起是脱敏后的 ffmpeg stderr 最后若干行；前端据此选 RTMP / SRT 的提示文案（SRT 用"连接失败，请检查地址和口令是否正确"，文案由前端负责，后端 `message` 不承载） |
| 编辑类错误（`EditService` 的 `ValidateProject` / `Export` 返回的 `INVALID_ARGUMENT`、`NOT_FOUND`、`IO_ERROR`、`PROBE_FAILED`、`UNSUPPORTED`） | **按 6.11.2 B（#22）**：`clip=<clip.id> path=<绝对路径>`，或没有 clip 的工程级错误写 `project` | 由 6.11.2 B 定义，第二行起才是原因（如 `overlaps=<clip.id>`、`path_length=<n> limit=259`、`missing=filter_complex`） | 前端用 6.11.2 B 的正则取首行；**不适用**下面"第一行只有一个 `key=value`"的统一规则；此行是 #22 合入后生效，#22 单独看时它引用的 6.11.2 B 就在该 PR 里，措辞与 #22 的 B 一致（已核对） |
| `LIVE_SOURCE_GONE`（v0.14，`StartScreenPush` 的所选来源已不可用） | `kind=<值>` | `window`（窗口已关闭 / 最小化 / 不可见）、`screen`（屏幕序号不存在）（只追加） | 只有这一行，没有第二行（不带窗口标题、不带地址）；前端用 `^kind=(window\|screen)$` 匹配（未知值按通用文案）；前端不必解析也能工作：码本身就足够提示「所选窗口已不可用」 |
| DocService 错误（v0.16，`ConvertToPDF` 的同步校验和 `office_pdf` 任务的 `error`、`OpenPDF`、`ReadPDFChunk`；只有下面取值对应的场景，其余 Doc 错误没有 reason） | `reason=<值>` | `too_many_pages`（`UNSUPPORTED`，超过 5000 页，含文字量超限）、`format`（`UNSUPPORTED` 或 `INVALID_ARGUMENT`，格式不受支持：不支持的扩展名、没有扩展名、`OpenPDF` 的扩展名不是 `.pdf` / 内容不是 PDF）、`encrypted`（`UNSUPPORTED`，加密的 Office 文档，OLE 容器；**加密 PDF 能正常打开，不会有这个错误**）、`no_font`（`UNSUPPORTED`，需要 Unicode 字体而没有）、`invalid_ooxml`（`INVALID_ARGUMENT`，不是 zip、缺必需部件、XML 损坏、zip64 目录信息无效）、`too_large`（`INVALID_ARGUMENT`，超大小或超 zip 限制：文件 > 100 MiB、PDF > 512 MiB、zip 条目数 > 100 000、中央目录 > 9 600 000 字节、单个条目解压后 > 256 MiB）（只追加，不改名、不改含义、不删除） | `code` 和 `message` 不变，`message` 是给用户看的短句（精确文案见 6.12.6）；首行之后可以有自由文本行：`ConvertToPDF` 整体校验失败时**第二行是出错文件的绝对路径**（`reason` 行永远在首行），再后面是原因说明；任务的 `error` 没有路径行。取消、磁盘满（`CONVERT_DISK_FULL`）、读写失败（`IO_ERROR`）、`NOT_FOUND`、路径 / 参数 / 输出目录 / 句柄类的 `INVALID_ARGUMENT`、`INTERNAL` **没有 reason**，走该码的通用文案 |
| 转换记录接口（v0.23，6.14：`ConvertService` 的 `ListSourceRecords` / `PreviewOutputName` / `SubmitSources` / `Reconvert` / `DeleteSource` / `GetSourcePreviewURL` / `OpenSourceWithSystem` / `RevealSource` / `GetSource` / `RevealRecord`（v0.23.1），`GetRecordThumbnail` / `GetSourceThumbnail`，`AddSources` 的单项错误，`TaskService` 的 `GetPreviewURL` / `OpenWithSystem` / `UnhideInTaskCenter`；只有下面取值对应的场景） | `reason=<值>` | `NOT_FOUND`：`record`（任务、源文件行或预设记录不存在，含旧类型 id）、`file`（记录在，但登记的文件已不存在或不是普通文件；任务不是 `succeeded` 时 `which=output` 也是它）、`no_app`（系统没有能打开这个文件的程序，message 固定 `没有找到能打开这个文件的程序`）；`UNSUPPORTED`：`format`（扩展名不在 6.14.7 的预览 / 系统打开白名单；缩略图接口上表示这个文件做不出缩略图，如纯音频，见 6.14.10）（只追加，不改名、不改含义、不删除） | `detail` 只有这一行；`SubmitSources` / `Reconvert` 只用 `reason=record`（id 不存在），**源文件本身的问题（不存在、探测失败、参数不兼容）仍按 6.9：`detail` 第一行是文件路径、没有 reason 行**；其余 6.14 的错误（`INVALID_ARGUMENT`、`TASK_CONFLICT`、`PROCESS_FAILED`、`INTERNAL`）没有 reason，走通用文案 |
| 存储与副本、格式目录、原地重转（v0.24 / v0.24.1，6.15 / 6.16 / 6.17：`ConvertSource.copyError`、`lastReconvertError.detail`（6.17.5）、`Reconvert` 的同步错误（`TASK_CONFLICT` `reason=invalid_state` / `output_moved`、`INVALID_ARGUMENT` `reason=format_change`、源文件不在时 `NOT_FOUND` `reason=file` + 第二行路径）、`AddSources` 的单项错误、`SubmitSources` / `Submit` / `Reconvert` / `Retry` / `GetSourcePreviewURL` 的同步错误、`CancelCopy` / `RetryCopy`、`OpenStorageFolder`；只有下面取值对应的场景） | `reason=<值>` | `CONVERT_DISK_FULL`：`no_space`（复制前空间不够或复制时写满；**后面固定两行 `needBytes=<整数>`、`freeBytes=<整数>`**，单位字节）；`IO_ERROR`：`in_use` / `permission` / `io`（v0.24 原地重转替换旧输出失败，出现在 `lastReconvertError.detail`，第二行是目标路径，6.17.5）、`source_changed`（复制期间原文件被修改）、`interrupted`（应用退出时还没复制完）；`TASK_CONFLICT`：`invalid_state`（`Reconvert` 的记录不是 `succeeded` 或已在重转，6.17.1）、`output_moved`（`Reconvert` 的旧输出已被移动或替换；也出现在 `lastReconvertError`，6.17.1 / 6.17.5）、`copying`（`SubmitSources` 选中的行**全部**没就绪且有正在复制的、`Reconvert` 的行在复制、`GetSourcePreviewURL` 的行在复制）、`copy_failed`（`SubmitSources` 选中的行全部复制失败或已取消、`Reconvert` 的行复制失败或已取消）（`SubmitSources` / `Reconvert` 的这两个后面一行 `sourceId=<id>`；`GetSourcePreviewURL` 只有一行）；`INVALID_ARGUMENT`：`format_change`（`Reconvert` 换了输出格式，6.17.1）、`params_locked`（v0.24.1：旧输出不在了时 `Reconvert` 给了 `presetId` / `options`，6.17.1）；`UNSUPPORTED`：`format`（格式不可输出，或 `AddSources` 的扩展名不在输入列表）、`encoder`（所选编码在当前转换组件里没有编码器）；`NOT_FOUND`：`file`（原文件不在、自定义保存位置不在）、`component`（v0.25.1：`OpenStorageFolder("component")` 时转换组件没有就绪或文件不在，只有这一行）（只追加，不改名、不改含义、不删除） | 首行之后的行只有上面写明的 `needBytes` / `freeBytes` / `sourceId`，前端按 `^(needBytes|freeBytes)=([0-9]+)$`、`^sourceId=([0-9A-Z]+)$` 取；没有 reason 的复制错误（权限、其他读写失败）走该码的通用文案。`TASK_CONFLICT` 的这两个取值是“直播会话冲突以外的 `TASK_CONFLICT` 没有 `reason=` 行”的例外 |
| 直播预览视频流（v0.25，6.10.3：`GetPreviewStream`、`live:pull` 的 `error`） | `reason=<值>` | `UNSUPPORTED`：`codec`（编码不能在应用内播放，第二行 `video=<编码名>` 或 `audio=<编码名>`）、`preview_unavailable`（这个会话没有预览视频流：转换组件缺 `tee` / `tcp`，或预览分支没连上 / 已断开）；`NOT_FOUND`：`session`（会话不存在或已结束）（只追加） | 首行之后只有 `codec` 的那一行 |
| 文档转换的不可重试记录（v0.26，`TaskService.Retry` 遇到 retryable=否 的 `doc_convert` / `office_pdf` 失败记录） | `reason=<值>` | `UNSUPPORTED`：`not_retryable`（只追加） | 只有这一行；`DOC_*` 码的 `detail` 首行（`exit=` / `msiexec=` 等）前端不解析，见 6.12.20 |
| 文档编辑（v0.27.1，6.12.41 / 6.12.42：`DocService.SaveDocText`、`SaveDocTextAs` 的同步错误） | `reason=<值>` | `TASK_CONFLICT`：`file_changed`（文件在打开之后被别的程序改过，revision 不一致）、`converting`（这一行有排队中 / 运行中的转换，或结果记录正在重转；v0.27.1 补充：原写 `in_use`，已改名）、`copying`（沿用 v0.24：副本还在复制，后面一行 `sourceId=<id>`）；`NOT_FOUND`：`record`、`file`（沿用：原文件 / 输出文件不在了）；`IO_ERROR`：`permission`（没有写权限、只读）、`in_use`（被其他程序占用、读不了或替换不了，和 `file_changed` 分开）、`io`（其他写入失败）（这三个沿用 v0.24 的取值，这里没有第二行路径）；`INVALID_ARGUMENT`：`encoding`（v0.27.1 补充：有字符编不进原编码 GBK；后面两行 `char=U+XXXX`、`line=<n>`）、`too_large`（超过 2 MiB 或 1000 行）、`format`（不能编辑的格式、另存为换了格式、html 声明了不支持的编码）（只追加） | 除 `copying`（`sourceId=`）和 `encoding`（`char=` / `line=`）外都只有这一行；另存为的目标路径不合法（应用数据目录、上传目录、非绝对路径）是没有 reason 的 `INVALID_ARGUMENT`；磁盘满 `CONVERT_DISK_FULL` 不带 reason；界面不显示错误码和 reason |
| docx 分段保存（v0.27.2，6.12.49~6.12.52：`BeginDocBinarySave`、`AppendDocBinaryChunk`、`CommitDocBinarySave` 的同步错误） | `reason=<值>` | `INVALID_ARGUMENT`：`too_large`（沿用：超过 20 MiB）、`chunk_order`（`seq` 不连续，会话不作废）、`checksum`（字节数或 SHA-256 不符）、`malformed`（完整性检查不过）、`format`（沿用：不是 docx、另存为扩展名不对、含宏）；`NOT_FOUND`：`save_session`（会话不在或已过期）、`file` / `record`（沿用）；`TASK_CONFLICT`：`saving`（同一文件已有保存会话，或会话数到上限）、`file_changed` / `converting` / `copying`（沿用 v0.27.1）；`IO_ERROR`：`backup`（v0.27.2 补充：备份失败）、`permission` / `in_use` / `io`（沿用）（只追加） | 都只有这一行；`chunk_order`、`checksum`、`save_session`、`saving` 是前端内部错误，界面一律 `出了点问题，请重试。`；界面不显示错误码和 reason |
| PDF 权限保护（v0.28.1，6.12.66 ①：添加 / 提交 / 运行时的 `DOC_ENCRYPTED`） | `reason=<值>` | `owner_only`（空用户密码能打开、只有所有者密码、AES-256，纯 Go 库解不了；要用户密码的 `DOC_ENCRYPTED` 仍没有 detail）（只追加） | 只有这一行；不可重试；界面不显示 |
| 语音工具转字幕（v0.29，6.18） | `reason=<值>` | `INVALID_ARGUMENT`：`cue_invalid`（导出硬校验）；`NOT_FOUND`：`component`（`OpenStorageFolder("lang_asr")`）（只追加） | 界面不显示 |
| PDF 输入（v0.28，6.12.60 / 6.12.63：添加 PDF 的 `AddDocSourceResult.error`、`DOC_PDF_NO_TEXT`） | `reason=<值>` | `INVALID_ARGUMENT`：`too_large`（沿用：PDF 超过 200 MiB）、`too_many_pages`（超过 500 页）；`DOC_PDF_NO_TEXT`：`no_text`（第二行 `quality=empty\|garbled` 只给开发者看）（只追加） | 只有第一行；界面不显示 |
| 文档组件未就绪（v0.27.2，6.12.55：`DOC_COMPONENT_NOT_READY` 的同步错误和任务错误，以及 `DocComponentStatus.error`） | `reason=<值>` | `checking`、`missing`、`outdated`（含 Windows / macOS 应用下载的组件低于 26.2.6）、`downloading`、`preparing`、`failed`，即 `componentState`（只追加） | 只有这一行；`OpenStorageFolder("doc_component")` 的 `NOT_FOUND` 沿用 v0.25.1 的 `reason=component` |
| `INVALID_ARGUMENT` / `NOT_FOUND` / `TASK_CONFLICT`（v0.31 Cat 项目，6.19.10：`CreateCatProject`、`RenameCatProject`；v0.31.1 加 `RelocateCatProject`） | `reason=<值>` | `INVALID_ARGUMENT`：`project_path`（不是绝对路径 / 不存在 / 不是文件夹）、`project_root`（盘符或文件系统根）、`project_name`（名字为空（改名时）、超过 60 个字符或含控制字符）、`project_duplicate`（v0.31.1：`RelocateCatProject` 的新路径已属于另一个项目）；`TASK_CONFLICT`：`turn_running`（v0.31.1：`RelocateCatProject` 时该项目下有进行中的一轮）（只追加） | 只有 `project_duplicate` 有第二行 `projectId=<已有项目 id>`，前端按 `^projectId=(\S+)$` 取（取不到就重新 `ListCatProjects`、只提示不切换）；其余只有一行。界面按 6.19.10.8 映射；`NOT_FOUND` 项目不存在无固定 detail。`turn_running` 也是“直播会话冲突以外的 `TASK_CONFLICT` 没有 `reason=` 行”的例外 |
| 其余所有码（含 `LIVE_PUSH_REJECTED`、`LIVE_PUSH_INTERRUPTED`、`INTERNAL`） | 无固定格式 | — | 前端**不得**解析（上面几行列出的码 / 场景除外） |

统一规则：第一行只有一个 `key=value`，值只含小写字母、数字、下划线（`scheme` 例外；`kind` 的取值是 `window` / `screen`，取值就是上面三个小写单词）；前端用 `^(reason|scheme|kind)=([a-z0-9_]+)$` 匹配 `detail` 的第一行；**没有第一行、格式不对、或值不认识，一律走该错误码的通用文案**，不得猜测含义、不得报错崩溃。测试必须逐码断言第一行精确等于期望值（不是包含）。

### 2.3 示例（JSON 里的数值是示意）

```json
{ "code": "LIVE_URL_INVALID", "message": "暂不支持这种推流协议", "detail": "reason=scheme_unsupported" }
```
```json
{ "code": "TASK_CONFLICT", "message": "直播会话冲突", "detail": "reason=duplicate_url\n已有会话使用同一推流地址" }
```
```json
{ "code": "TASK_CONFLICT", "message": "已有屏幕推流在进行", "detail": "reason=screen_busy" }
```
```json
{ "code": "LIVE_CONNECT_FAILED", "message": "连接推流服务器失败", "detail": "scheme=rtmp\n[tcp @ 0x7f50942c6900] Connection to tcp://127.0.0.1:1999?tcp_nodelay=0 failed: Connection refused\n[out#0/tee @ 0x557c2ff9e9c0] Could not write header (incorrect codec parameters ?): Connection refused" }
```


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
    VideoCodecName string `json:"videoCodecName,omitempty"` // v0.23.4：给人看的编码名（FFV1、DNxHD、H.265……），后端按唯一的显示名表生成；videoCodec 为空时省略
    AudioCodecName string `json:"audioCodecName,omitempty"` // v0.23.4：同上（AAC、PCM、AC-3……）；audioCodec 为空时省略
    // v0.8 扩展，只在 Probe 时填充、不入库（ListRecent 里为零值 / 省略）：
    // container, fps, rotation(0/90/180/270), sampleRate, channels, hasVideo, hasAudio（v0.22 起 hasVideo / hasAudio 始终输出，false 时是 false 不是缺失，其余字段仍是零值省略）,
    // streams[]{index,type,codec,profile,width,height,pixFmt,fps,bitrate,duration,rotation,sampleRate,channels,channelLayout,language,attachedPic},
    // probedAt, error?(批量探测时该文件的错误)
}

type TaskType string // convert | edit_export | office_pdf | live_file_push | live_screen_push | ffmpeg_install | doc_convert | speech_to_subtitle
                     // speech_to_subtitle（v0.29，6.18）：语音工具「转字幕」；界面只显示「转字幕」
                     // doc_convert（v0.26，6.12.21）：文档页的转换；office_pdf 自 v0.26 起只用于旧记录和组件未就绪时的简易转换
                     // edit_export：v0.23.5 剪辑移除后不再产生；旧记录照常列出（任务中心显示为“旧版导出”，v0.25.3）、只能查看和删除，Retry / Reconvert 一律 UNSUPPORTED（reason=feature_removed，message 不含“剪辑”），不是下面的“旧类型”
                     // 保留但不再产生：live_relay、live_record_push（v0.10 取消）、edit_render（v0.11 起改名 edit_export）。不能提交，任务中心不展示，库里的旧记录按未知类型忽略、不报错
type TaskStatus string // queued | running | succeeded | failed | canceled | interrupted
                       // interrupted（已中断，终态）：应用退出时还没结束（error 为空）；v0.25.3 起也表示直播任务开始以后被中断（error 为 LIVE_PUSH_INTERRUPTED / LIVE_SOURCE_GONE，6.10）。
                       // 它是“已结束”（HideFinishedInTaskCenter 会隐藏它），但不是 failed：ListTasks 按 ["failed"] 筛选不含它

type Task struct {
    ID         string     `json:"id"`
    Type       TaskType   `json:"type"`
    Status     TaskStatus `json:"status"`
    Title      string     `json:"title"`
    InputPaths []string   `json:"inputPaths"`
    OutputPath string     `json:"outputPath"`
    Progress   float64    `json:"progress"`   // 0~1，直播类任务恒为 -1；succeeded 为 1；canceled / failed / interrupted 保留结束那一刻的值，不清零（v0.23，见 6.14.6：canceled 表示取消时大约转到了哪里，不代表有可用的部分输出）
    Speed      string     `json:"speed"`      // 如 "2.3x"
    EtaSec     float64    `json:"etaSec"`
    // 以下三项只有直播任务在运行中才有值（v0.10），只在内存里、不落库，和 speed / etaSec 一样：
    Fps           float64 `json:"fps,omitempty"`           // 当前输出帧率
    BitrateKbps   float64 `json:"bitrateKbps,omitempty"`   // 近 5 秒的输出码率（kbit/s）；有本地存档（tee）的会话没有此值，省略
    DroppedFrames int64   `json:"droppedFrames,omitempty"` // ffmpeg 丢弃的帧数（累计），不是网络丢包
    // 以下四项（v0.18，见 9.7）：任务实际使用的视频编码器；没有视频编码的任务（纯音频转换、Office 转 PDF、ffmpeg 安装）省略；会落库（tasks 表迁移 0004）：
    Encoder          string `json:"encoder,omitempty"`          // h264_nvenc | hevc_nvenc | h264_qsv | hevc_qsv | h264_amf | hevc_amf | h264_videotoolbox | hevc_videotoolbox | libx264 | libx265 | libvpx-vp9 | gif | copy
    EncoderDevice    string `json:"encoderDevice,omitempty"`    // 设备 id（nvidia-0 之类，即 EncoderDevice.id）；CPU 编码为 "cpu"；copy 时省略
    HWFallback       bool   `json:"hwFallback,omitempty"`       // 想用硬件但实际用了 CPU：所选设备不可用，或硬件编码启动失败后自动用 CPU 重试
    HWFallbackReason string `json:"hwFallbackReason,omitempty"` // 一行短原因（固定枚举，不含路径），见 9.7
    Params     string     `json:"params"`     // 原始参数 JSON，用于重试（直播任务的 params 已脱敏，不能用来重试，见 6.10）；convert 的 params 含提交时快照 presetId / presetName / paramsSummary（v0.23，6.14.2），原地重试不改 params
    Version    int64      `json:"version"`    // 每次变更 +1，前端据此丢弃旧事件
    Error      *AppError  `json:"error,omitempty"` // 无错误时省略（不是 null）；TS 里是 error?: AppError；succeeded / canceled 一律没有该键
    CreatedAt  int64      `json:"createdAt"`
    StartedAt  int64      `json:"startedAt"`
    FinishedAt int64      `json:"finishedAt"`
    // 以下三项 v0.23（迁移 0005，见 6.14）：
    SourceID           string      `json:"sourceId,omitempty"` // 只有 convert 任务有：所属源文件行（convert_sources.id）
    HiddenInTaskCenter bool        `json:"hiddenInTaskCenter"` // 始终输出；true = 任务中心已隐藏（“隐藏已结束”），转换页照常显示；原地重试时清回 false
    Reconverting       bool            `json:"reconverting"`                 // v0.24：始终输出；true = 正在原地重转（6.17）
    LastReconvertError *ReconvertError `json:"lastReconvertError,omitempty"` // v0.24：最近一次重转失败的信息（6.17.2）
    QueuePosition      *int        `json:"queuePosition,omitempty"` // v0.26：只有文档组件池里 queued 的 doc_convert 任务有，前面还有几项（0 起），只在内存（6.12.18）
    Result             *TaskResult `json:"result,omitempty"`   // 只有成功的 convert 任务有：完成时探测输出得到 {sizeBytes, durationSec, width, height, audioBitrateKbps, warnings}，见 6.14.2（warnings 是 v0.24，6.14.6）
}

type Preset struct {
    ID       string        `json:"id"`
    Name     string        `json:"name"`
    BuiltIn  bool          `json:"builtIn"`
    Options  ConvertOptions `json:"options"`
}

type ConvertOptions struct {
    Container    string  `json:"container"`    // mp4 mkv mov webm avi flv gif mp3 aac wav flac m4a ogg opus；v0.24 加 wmv mpg vob 3gp swf ogv wma amr m4r mp2 ape wv mmf webp ico bmp tif tga（6.16.2）
    VideoCodec   string  `json:"videoCodec"`   // copy h264 h265 vp9 ""(无视频)；v0.24 加 mpeg4 mpeg2 wmv2 flv1 theora；gif 和图片格式忽略（只能是 ""）
    AudioCodec   string  `json:"audioCodec"`   // copy aac mp3 opus vorbis flac pcm ac3 none ""(容器默认)；v0.24 加 wma amr_nb mp2 wavpack adpcm_yamaha ape
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
SaveFileDialog(defaultName string, filters []FileFilter) (string, error) // v0.27.1（6.12.43）：系统保存对话框，取消返回 ""；defaultName 是绝对路径时用它的文件夹作初始位置
PickDirectory(title string) (string, error) // 取消返回 ""
RevealInFolder(path string) error
GetEnv() (EnvInfo, error)            // 系统、ffmpeg 版本、数据目录
GetSettings() (Settings, error)
UpdateSettings(s Settings) error      // defaultOutputDir（v0.24：空 = 默认输出目录 <base>/output，不再是“与源文件同目录”）、uploadsDir（v0.24，空 = <base>/uploads）、maxConcurrent（0=自动，1~8）、主题、语言、ffmpegPromptDismissed、ffmpegPath、docEngine（v0.27：auto | office | wps | component，默认 auto，6.12.29）
GetStorageDirs() (StorageDirs, error)                // v0.24（6.15.2）：实际输出 / 上传目录 + 是否回退到用户数据目录
SetStorageDirs(req StorageDirsUpdate) (StorageDirs, error) // v0.24：同时设两个目录，"" = 默认；校验失败 INVALID_ARGUMENT、整体不生效
OpenStorageFolder(kind string) error                 // v0.24：kind = "output" | "uploads"，在文件管理器里打开实际目录；v0.25.1 加 "component"（转换组件所在文件夹，6.15.2 第 6 条）；v0.27.2 加 "doc_component"（应用下载的文档组件目录，6.12.54）；v0.29 加 "lang_asr"（语音识别组件目录，6.18.3）
// LangService（v0.29，6.18；一期仅转字幕）
GetLangAsrStatus() (LangAsrStatus, error)
InstallLangAsr(tier string) (LangAsrStatus, error)
CancelLangAsrInstall() error
RecheckLangAsr() (LangAsrStatus, error)
SubmitSpeechToSubtitle(req SpeechToSubtitleRequest) (Task, error) // 批量形态见 6.18.5
ExportSubtitleCues(req ExportSubtitleRequest) (ExportSubtitleResult, error)
// CatService（v0.30，6.19；一期仅 cat_build，会话锁定 agentKind）
GetCatStatus() (CatStatus, error)
RecheckCat() (CatStatus, error)
ListCatModels(agentKind string) ([]CatModel, error)
ListCatThinkLevels(agentKind string) ([]CatThinkLevel, error)
ListCatConversations() ([]CatConversation, error)
GetCatConversation(id string) (CatConversationDetail, error)
CreateCatConversation(req CreateCatConversationRequest) (CatConversation, error)
DeleteCatConversation(id string) error
SendCatMessage(req SendCatMessageRequest) (SendCatMessageResult, error)
CancelCatTurn(req CancelCatTurnRequest) error // v0.30.2：{ convId, turnId }，见 6.19.9
// v0.31 项目（6.19.10）；CreateCatConversation 的 req 新增可选 projectId（创建后不可变）
ListCatProjects() ([]CatProject, error)                                      // missing 实时计算
CreateCatProject(req CreateCatProjectRequest) (CreateCatProjectResult, error) // {path, name?} → {project, existed}；同路径返回已有
RenameCatProject(req RenameCatProjectRequest) (CatProject, error)            // missing 也可改名
DeleteCatProject(req DeleteCatProjectRequest) error                          // 未知 id → nil；不动文件夹里的文件
RelocateCatProject(req RelocateCatProjectRequest) (CatProject, error)        // v0.31.1：{id, path} 换文件夹，对话保留；missing 也可（6.19.10.2 第 8 条）
RevealCatProject(req RevealCatProjectRequest) error                          // v0.31.1：{id} 在文件管理器里打开项目文件夹，不写任何东西（第 9 条）
ListEncoderDevices() (EncoderDeviceList, error)      // 硬件编码设备（9.6）：第一项永远是 cpu；ffmpeg 未就绪时只有 cpu 且 ffmpegReady=false，不报错
RefreshEncoderDevices() (EncoderDeviceList, error)   // 丢弃缓存重新检测
GetEncoderPreference() (string, error)               // "auto" | "cpu" | 设备 id，默认 "auto"；所选设备不可用时保持原值
GetEncoderPreferenceInfo() (EncoderPreferenceInfo, error) // {id, name, available, reason?}
SetEncoderPreference(id string) error                // 只接受 auto、cpu、ListEncoderDevices 里存在的设备 id，否则 INVALID_ARGUMENT
```

### App（main 包，非 Service）
```go
GetLicenseText(name string) (string, error) // 内嵌第三方许可全文；白名单 "OFL"（Noto Sans SC 的 SIL OFL 1.1）、"OFL-Nunito"（Nunito 的 SIL OFL 1.1），其它名称（含空串、带路径、大小写不同）→ INVALID_ARGUMENT
GetAppVersion() string                      // 构建时 -ldflags "-X FFmpegFree/internal/about.Version=..." 注入；未注入返回 "开发版"
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
Submit(inputs []string, opts ConvertOptions, outputDir string) ([]Task, error) // 批量，一个文件一个任务；v0.23 起按路径自动找到 / 创建源文件行（兼容保留，新前端用 SubmitSources）
// v0.23 转换记录（数据结构、错误码、事件见 6.14）：
AddSources(paths []string) ([]AddSourceResult, error)
ListSources(filter ConvertSourceFilter) (ConvertSourcePage, error)
GetSource(sourceID string) (ConvertSourceEntry, error)   // v0.23.1：一行，与 ListSources 的一项完全相同（任务中心“在转换页查看”）
ListSourceRecords(sourceID string, limit, offset int) (TaskPage, error)
SearchSources(filter ConvertSearchFilter) (ConvertSourcePage, error)
CheckSources(sourceIDs []string) ([]SourcePathCheck, error)
PreviewOutputName(sourceID string, opts ConvertOptions, outputDir string) (string, error)
SubmitSources(req ConvertSubmitRequest) (ConvertSubmitResult, error) // v0.24：返回 {tasks, skipped}，副本没就绪的行跳过（6.15.4 第 6 条）
Reconvert(req ReconvertRequest) (Task, error) // v0.24：同一条记录原地重转（只用于 succeeded；成功后原子替换旧输出，失败 / 取消恢复成 succeeded），参数可选、只能同格式，见 6.17
DeleteRecords(taskIDs []string, deleteOutputs bool) (DeleteResult, error)
DeleteSource(sourceID string, deleteOutputs bool) (DeleteResult, error)
GetSourcePreviewURL(sourceID string) (PreviewURL, error)
OpenSourceWithSystem(sourceID string) error
RevealSource(sourceID string) error
RevealRecord(taskID string) error        // v0.23.1：在文件管理器里显示转换记录的输出文件（转换页不再用 RevealInFolder）
GetRecordThumbnail(taskID string) (string, error)     // 转换记录输出文件的缩略图，data:image/jpeg;base64,...（6.14.10）
GetSourceThumbnail(sourceID string) (string, error)   // 源文件的缩略图，格式同上
// v0.24（6.15 / 6.16）：
CancelCopy(sourceID string) error                     // 取消这一行副本的复制
RetryCopy(sourceID string) (ConvertSource, error)     // 复制失败 / 已取消 / 副本丢失后重新复制
GetFormatCatalog() ([]FormatEntry, error)             // 格式目录（video / audio / image），前端取一次、自己做模糊搜索
```

### ~~EditService~~（v0.23.5 已移除）
剪辑功能整体移除，`EditService` 不再绑定，它的方法（`ValidateProject`、`Export`、`GetPreviewURL`、`SaveProject`、`LoadProject`、`ListProjects`、`DeleteProject`）全部删除。原来的定义见 6.11（历史记录）。素材探测、缩略图、最近素材仍在 `MediaService`。

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

v0.26 新增（文档多格式转换，详见 6.12.9~6.12.22；`ConvertToPDF` / `GetDocCapabilities` 保留给旧前端，新文档页不用）：
```go
GetFormatMatrix() (DocFormatMatrix, error)                      // 后端给的格式表：每个源格式能转成什么、要不要组件、是否简易转换、提示
GetDocComponentStatus() (DocComponentStatus, error)              // 文档组件状态（没有路径）
InstallDocComponent(mirror string) (DocComponentStatus, error)   // 下载 + 准备（"" | "cn"），幂等、断点续传；Linux UNSUPPORTED_PLATFORM
CancelDocComponentInstall() error
RecheckDocComponent() (DocComponentStatus, error)
AddDocSources(paths []string) ([]AddDocSourceResult, error)      // 带 sheetCount；被拒原因见 6.12.20
ListDocSources(filter ConvertSourceFilter) (DocSourcePage, error)
SearchDocSources(filter ConvertSearchFilter) (DocSourcePage, error)
SubmitDocConvert(req DocSubmitRequest) (ConvertSubmitResult, error) // 一个源一个 doc_convert（或简易转换的 office_pdf）任务
```

v0.27 新增（文档预览，详见 6.12.32；引擎相关的改动在 `DocComponentStatus` 和设置里，没有新方法）：
```go
GetDocPreview(req DocPreviewRequest) (DocPreview, error)        // {sourceId | taskId} 二选一；返回 previewId、kind（pdf | text | csv | html | md | raw | unavailable）、state；pdf / raw 给 /local/<token> 地址，缓存命中 / 文字类同步就绪
CancelDocPreview(previewID string) error                         // 关弹窗时调：停止生成或作废本地地址；幂等
```

v0.27.1 新增（文档编辑，详见 6.12.37~6.12.46；`DocPreview` 加 `editable` / `editBlock` / `revision` / `encoding` / `lineEnding`）：
```go
SaveDocText(req DocSaveRequest) (DocSaveResult, error)     // {sourceId|taskId, revision, text|rows}：写回原文件 / 输出文件，原子替换，返回新 revision
SaveDocTextAs(req DocSaveAsRequest) (DocSaveResult, error) // {sourceId|taskId, targetPath, text|rows, encoding: keep|utf8}：另存为，不新建源文件行
```

v0.27.2 新增（docx 编辑后分段保存，详见 6.12.47~6.12.53；`DocPreview` 加 `rawUrl`）：
```go
BeginDocBinarySave(req DocBinarySaveBegin) (DocBinarySaveSession, error)       // {sourceId|taskId, mode: overwrite|save_as, targetPath?, revision?, totalBytes} → {saveId, maxChunkBytes, expiresAt}
AppendDocBinaryChunk(req DocBinaryChunk) (DocBinaryChunkResult, error)         // {saveId, seq, data(base64，≤ 4 MiB)} → {receivedBytes}
CommitDocBinarySave(req DocBinarySaveCommit) (DocBinarySaveResult, error)      // {saveId, sha256} → {path, revision, sizeBytes, savedAt, backupPath?}
AbortDocBinarySave(req DocBinarySaveAbort) error                               // {saveId}，幂等
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
StartScreenPush(req ScreenPushRequest) (Task, error)   // 屏幕推流（可同时本地存档）；同一时间最多 1 路：已有进行中的 live_screen_push 返回 TASK_CONFLICT（detail 首行 reason=screen_busy）
GetCaptureCapabilities() (CaptureCapabilities, error)  // 屏幕采集能不能用、为什么不能用（Linux 读 XDG_SESSION_TYPE 和 DISPLAY，见 6.10「采集能力检测」）
ListCaptureSources() ([]CaptureSource, error)          // （v0.14）屏幕推流可选的采集来源：屏幕（所有平台）+ 应用窗口（只有 Windows）；不能采集屏幕的平台 / 会话返回 UNSUPPORTED_PLATFORM（同 ListScreens），见 6.10「采集来源」
GetPreviewStream(sessionID string) (PreviewStream, error) // （v0.25）推流任务 id 或拉流预览会话 id 的预览视频流：{url: http://127.0.0.1:<端口>/live/<token>.flv, mime: "video/x-flv", hasVideo, hasAudio}，见 6.10.3。会话不存在 NOT_FOUND（reason=session）；没有预览分支 UNSUPPORTED（reason=preview_unavailable）；开关预览只在前端，不重启推流；编码不能播放 UNSUPPORTED（reason=codec）
// ~~GetPreview(sessionID string) (Preview, error)~~      // v0.25 删除。原说明：（v0.17）会话（推流任务 id 或拉流预览会话 id）最新一帧预览：{data: base64 JPEG, ts: 毫秒时间戳, active}；没有画面（会话不存在 / 已结束、preview=false、还没出第一帧）返回空 data、ts=0，不是错误。前端约 500 毫秒轮询，见 6.10「预览画面」
StartPullPreview(req PullPreviewRequest) (PullSession, error) // （v0.17）拉流预览会话：后端 ffmpeg 读 rtmp / rtmps / srt / http(s) 远端流，（v0.25）`-c copy` 转封装成 FLV，经本机 HTTP 提供，返回值多 previewUrl，结束状态走 live:pull 事件（6.10.3）；同一地址幂等；同时最多 4 路
StopPullPreview(sessionID string) error                // （v0.17）停止拉流预览会话并清理预览文件；会话不存在（已结束）无操作
ListScreens() ([]ScreenInfo, error)                    // 可采集的显示器（Linux 用 xrandr --display $DISPLAY --query，必须带 --display，见 6.10「采集能力检测」）
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
    Preview    *bool       `json:"preview"`    // **v0.25 起后端忽略**（预览分支始终存在，开关只在前端，6.10.3.2a）。原说明：（v0.17）可选：nil / true = 带预览画面（GetPreview）；false = 不加预览输出。只在开始时决定（ffmpeg 已启动无法动态改输出）
    Loop       bool        `json:"loop"`       // true = 循环播放直到用户停止；false = 播完自然结束（任务 succeeded）
    Options    PushOptions `json:"options"`
}

type ScreenPushRequest struct {
    URL        string      `json:"url"`
    ScreenID   string      `json:"screenId"`   // ListScreens 返回的 id；"" = 主显示器；不存在 INVALID_ARGUMENT
    HideCursor bool        `json:"hideCursor"` // 零值 = 画面里带鼠标指针
    Audio      string      `json:"audio"`      // "none"（默认，视频流里没有音轨）| "silent"（补一路静音音轨，给要求必须有音频的服务器）；采集声音 v1 不做
    Preview    *bool       `json:"preview"`    // （v0.17）同 FilePushRequest.preview
    CaptureSourceID string `json:"captureSourceId"` // （v0.14）可选：ListCaptureSources 返回的 id（screen:<序号> | window:<hwnd 十进制>）；"" = 不传，行为同 v0.13（按 ScreenID）；非空时以它为准，ScreenID 被忽略；格式不对 INVALID_ARGUMENT；来源已不可用 LIVE_SOURCE_GONE
    ArchiveDir string      `json:"archiveDir"` // 非空 = 同时在本地存一份 mp4（绝对路径，不存在会创建；存档规则见 6.10）；"" = 不存档。v0.24.4：页面打开“保存存档”且没有另选文件夹时，前端填实际输出目录（`GetStorageDirs().outputDir`，`defaultOutputDir` 为空即 `<base>/output`）再传入，不把空字符串当成存档目录
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

// v0.25 删除 Preview（GetPreview 的返回），改用 PreviewStream（见 6.10.3.1）。以下是 v0.17 原文，只作历史记录。
// v0.17：GetPreview 的返回。没有画面时 data 为 ""、ts 为 0（不是错误）。
type Preview struct {
    Data   string `json:"data"`   // 最新一帧 JPEG 的 base64（标准编码，不带 data: 前缀）；没有画面 ""
    TS     int64  `json:"ts"`     // 这一帧写入的时间（毫秒时间戳，取文件修改时间）；没有画面 0。前端可据此判断画面是否停滞
    Active bool   `json:"active"` // 会话还在进行（推流任务未结束 / 拉流预览会话未结束）；false 时前端停止轮询
}

type PullPreviewRequest struct {
    URL     string `json:"url"`     // rtmp / rtmps / srt / http / https；ws / wss 没有对应的 ffmpeg 协议，LIVE_URL_INVALID（reason=scheme_unsupported）
    Preview *bool  `json:"preview"` // **v0.25 起后端忽略**（总是起 ffmpeg，6.10.3.2a）。原说明：nil / true = 出预览；false = 不启动 ffmpeg（GetPreview 恒为空）
}

type PullSession struct {
    ID       string `json:"id"`       // 会话 id，传给 GetPreview / StopPullPreview
    Redacted string `json:"redacted"` // 脱敏后的地址，可直接显示
    Preview  bool   `json:"preview"`  // 是否真的在出预览
    PreviewURL string `json:"previewUrl"` // （v0.25）预览视频流地址，同 GetPreviewStream(id).url；没有预览视频流（Preview=false）时 ""
}

// v0.25：GetPreviewStream 的返回（6.10.3.1）。
type PreviewStream struct {
    URL      string `json:"url"`      // http://127.0.0.1:<端口>/live/<token>.flv；token 每个会话一个，会话结束作废
    MIME     string `json:"mime"`     // 恒为 "video/x-flv"
    HasVideo bool   `json:"hasVideo"` // 推流恒为 true；拉流按探测结果
    HasAudio bool   `json:"hasAudio"` // 推流：文件推流 true、屏幕推流 audio=silent 时 true；拉流：有 AAC / MP3 音频时 true
}

// v0.14：一个可采集的来源。ListCaptureSources 返回它的列表：先是所有屏幕（顺序同 ListScreens），Windows 上再是窗口（EnumWindows 的 Z 序，最上面的在前）。
type CaptureSource struct {
    ID     string `json:"id"`     // 不透明字符串，前端原样传回 captureSourceId。screen:<序号>（序号是 ListScreens 结果里的位置，从 0 起，第 0 个不一定是主显示器）；window:<hwnd 十进制>（无符号十进制，无前导零）
    Kind   string `json:"kind"`   // screen | window。macOS / Linux 永远只有 screen，不返回 window
    Title  string `json:"title"`  // screen：ScreenInfo.Name（如 "屏幕 1（主显示器）"）；window：窗口标题原文
    Width  int    `json:"width"`  // 物理像素；window 是客户区大小（gdigrab 采的就是客户区）；查不到为 0
    Height int    `json:"height"`
}

type ScreenInfo struct {
    ID      string  `json:"id"`      // 不透明字符串，前端只原样传回：windows "monitor:<序号>"、darwin "avf:<设备序号>"、linux "x11:<输出名>" 或 "x11:desktop"
    Name    string  `json:"name"`    // 如 "屏幕 1（主显示器）"
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

**`Start*` 同步返回的错误**（此时没有创建任务）：`FFMPEG_NOT_FOUND`；`INVALID_ARGUMENT`（选项越界、输入文件没有视频、`archiveDir` 不是绝对路径、`screenId` 不存在）；`NOT_FOUND` / `PROBE_FAILED`（输入文件不存在 / 无法解析）；`LIVE_URL_INVALID`；`UNSUPPORTED`（开始前用 `ffmpeg -protocols` 检查：`Output:` 段必须有与地址 scheme 对应的协议（`srt` 地址要 `srt`，`rtmps` 地址要 `rtmps`，其余即 `rtmp` 地址要 `rtmp`），带存档时另需 `tee`，缺哪个返回它，**`detail` 就是单独一行 `missing=<协议名>`（第一行，没有第二行）**，协议名取 `rtmp`、`rtmps`、`srt`（带存档的会话缺 `tee` 时是 `missing=tee`）；`message` 是"当前转换组件不支持 <协议名>，请安装完整版转换组件"（v0.24，1.1）；`StartFilePush` 和 `StartScreenPush` 在参数校验之后、探测输入 / 枚举屏幕之前检查（`internal/service/live/service.go` 的 `checkProtocols`）；`CheckPushURL` 只校验地址、不联网、**不做协议检查**，不返回它；结果按 ffmpeg 路径缓存；7.1.5 上 `rtmp`、`rtmps`、`srt`、`tee` 都在）；`UNSUPPORTED_PLATFORM`（不能采集屏幕）；`SCREEN_PERMISSION_DENIED`（已知没有权限时）；`TASK_CONFLICT`（`detail` 首行 `reason=`，判断顺序 **`duplicate_url` → `screen_busy` → `max_sessions`**：同一个推流地址已经有进行中的会话（`duplicate_url`）；`StartScreenPush` 时已有进行中的 `live_screen_push`（屏幕推流同一时间最多 1 路，`reason=screen_busy`）；进行中的直播会话已达 4 个上限（`max_sessions`）；`StartFilePush` 只会得到 `duplicate_url` 和 `max_sessions`）。

**停止 = `TaskService.Cancel(taskID)`，不设 `StopPush`**。理由：
1. 状态机、落库、`task:status`、应用退出（`Shutdown`）走的就是同一条取消路径，直播任务的 Runner 本来就是"取消 → 先发 `q`，最多等 5 秒（有本地存档的会话 15 秒，见 6.10）让 ffmpeg 收尾，超时再强杀"（6.5、6.6）；再包一层 `StopPush` 只会多一个和 `Cancel` 语义重复、还要保持同步的入口。
2. 任务中心、通知条等所有能看到任务的地方本来就有"停止"按钮，直播会话不用特殊处理。
3. 结果语义（前端文案要区分）：优雅停止成功 → Runner 返回 nil → 任务是 **`succeeded`**（不是 `canceled`；用户点"停止直播"是直播的正常结束）；**自然播完**（文件推流 `loop=false` 播到结尾）也是 `succeeded`。**架构师定：优雅停止和自然播完都显示"已结束推流"，前端不区分，也不加任何字段；** 点"停止"之后 5 秒内（有存档的会话 15 秒内）刷新页面看到 `running` 是可接受的（`Cancel` 立即返回，ffmpeg 还在收尾），前端以 `task:status` 为准。收尾超时被强杀 → `canceled`（"已强制停止"）。**硬性规则（架构师定）：优雅停止记 `succeeded` 时任务的 `error` 必须为空；强杀记 `canceled` 时同样不带错误码（`error` 为空）；前端只看 `status` 区分"已结束推流"（`succeeded`）和"已强制停止"（`canceled`），不看 `error`。**测试必须断言这两种终态的 `Task.error == nil`，且 `task:status` 载荷不带 `error`。**已请求 `Cancel` 之后，ffmpeg 无论怎样非零退出，一律归 `canceled`、不带错误码，不得落 `LIVE_PUSH_INTERRUPTED` 等**（细则见 6.10「错误分类」）。**优雅停止的判定（架构师定）**：发 `q` 后 ffmpeg **退出码 0 才是 `succeeded`**；已请求 `Cancel` 后退出码非 0 一律 `canceled`（无错误码）。**#31 已实现**（见 6.10.2 第 1 项）：`ffmpeg.Run` 原先在发 `q` 后的宽限期内只要进程退出就返回 nil、不看退出码（`internal/ffmpeg/exec.go` 的 `exitedGracefully` 分支，对转换类任务合理，对直播不行），现在有 `ffmpeg.RunOptions.StrictGracefulExit`，直播 Runner 开启后**直播路径检查退出码**实测（7.1.5 与 9.0.2 一致）：`q` 之后无存档 / 有存档（tee）都退出码 **0**（约 0.06~0.3 秒）；而 **SIGINT 之后退出码是 255**，所以直播必须用 `q`（经 stdin 管道）而不是 SIGINT，否则优雅停止会被判成 `canceled`。已结束的会话 `Cancel` 返回 `TASK_CONFLICT`，重复点击（正在停止中）返回 nil。
4. `Retry` 对直播任务返回 `UNSUPPORTED`（没有注册重试工厂，且 params 已脱敏、拿不到密钥）；前端"重新开始"就是用表单里的值再调一次 `Start*`。

**推流地址校验规则**（`Start*` 和 `CheckPushURL` 共用，不通过一律 `LIVE_URL_INVALID`；`message` 说明原因，**`detail` 第一行固定 `reason=<值>`（枚举见 2.2），整个 `detail` 和 `message` 都不带地址、口令或它们的片段**，绝不回显原文）：
1. 先 `TrimSpace`；长度 ≤ 2048 字节；不能含空白、控制字符、`|`、`\`、`"`、`'`（`|` 是 ffmpeg tee 分隔符，其余会破坏命令行 / 日志）。
2. scheme 不区分大小写，只允许 `rtmp`、`rtmps`、`srt`；其余（`file`、`http(s)`、`rtsp`、`udp`、`tcp`、`pipe`、`concat`、`subfile`、`data` ……）一律拒绝。传给 ffmpeg 的永远是校验后的 URL 并带 `-protocol_whitelist`，不会因为用户输入变成读本地文件或打开别的协议。（不在白名单 → `reason=scheme_unsupported`；没有 `://` → `malformed`）
3. host 不能为空；端口写了必须在 1~65535；IPv6 用方括号；IDN 主机名转 punycode，转不了就拒绝。**允许**回环 / 内网地址（推到本机或局域网的 nginx-rtmp、SRS、MediaMTX 是正常用法）。 host 为空 → `reason=missing_host`；端口越界、IPv6 括号错误、IDN 转换失败 → `reason=malformed`。
4. `rtmp` / `rtmps`：路径至少要有应用名（`rtmp://host/` 不合法）；流名可以在路径里，也可以在查询参数里。
5. `srt`：必须写端口（缺失 → `malformed`）；**查询参数白名单**（键先做一次 URL 解码并转小写再比较；不在白名单、同名重复、`mode` 不是 `caller` 都是 `reason=param_not_allowed`）：`passphrase`（长度 10~79，超出 → `malformed`）、`pbkeylen`（只能 0、16、24、32）、`streamid`（≤ 512 字符）、`latency`、`connect_timeout`、`maxbw`、`pkt_size`（≤ 1456）、`mode=caller`（数值参数必须是整数，范围由实现按 `ffmpeg -h protocol=srt` 校验；单位以 7.1.5 为准：`latency` 微秒、`connect_timeout` 毫秒、`maxbw` 字节/秒）。**传给 ffmpeg 的 URL 由后端按白名单重新组装：键统一写成小写解码后的形式，值原样保留。**原因（7.1.5 实测）：ffmpeg 对参数名区分大小写、也不做百分号解码——`?PASSPHRASE=abc` 和 `?pass%70hrase=abc` 都被**悄悄忽略**（不报错，等于没加密就推出去了），（**ffmpeg 9.0.2（项目默认安装版本）行为不同：大写参数名不再被忽略，而是报 `Query string option 'PASSPHRASE' does not exist` / `Option not found`，退出码 8**；所以同一个错误地址在 7.1.5 上静默不加密、在 9.0.2 上直接失败，**不能依赖 ffmpeg 兜底**，仍然必须先解码、小写化再按白名单重组 URL）；而 `?passphrase=abc`（3 位）会报 `failed to set option SRTO_PASSPHRASE … Bad parameters`、10~80 位都能通过、81 位又报错（**长度上限取 79 是架构师定，比 ffmpeg 实测多接受的 80 位保守一位，与 SRT 规范的 10~79 一致；测试断言 9、10、79、80 位的结果分别是 `malformed`、通过、通过、`malformed`**）；`mode=listener` 会让 ffmpeg 挂起等连接。所以：小写化 + 白名单 + 重新组装是必须的，不能"原样交给 ffmpeg"。
6. 通过校验的 URL 只在内存里用；标准化形式（scheme / host 小写、去掉默认端口）用来判断"同一个地址已有进行中的会话"。

**推流密钥 / 凭据脱敏**（规则、覆盖范围和测试要求见 6.10）：地址里的用户信息、rtmp / rtmps 的流名（应用名之后的路径）、所有查询参数的值一律显示成 `***`；标题、`params`、任务日志、错误的 `message` / `detail`、所有事件 payload、后端日志都只出现脱敏后的地址；完整地址不落库、不写文件。

**任务字段**：`type` 是 `live_file_push` / `live_screen_push`；`title` 如 `文件推流：a.mp4 → rtmp://host/app/***`、`屏幕推流：屏幕 1（主显示器） → srt://host:9000?streamid=***&passphrase=***`；`inputPaths` 文件推流为 `[inputPath]`、屏幕推流为 `[]`；`outputPath` 是本地存档的最终路径，没存档为 `""`；`params` 见 6.10。

**示例（JSON 数值是示意）**

`StartFilePush` 请求 / 返回（返回是入队前的快照）：
```json
{ "inputPath": "C:\\Videos\\a.mp4", "url": "rtmp://live.example.com/app/mystreamkey", "loop": true,
  "options": { "width": 1280, "height": 720, "fps": 30, "videoBitrateKbps": 2500, "audioBitrateKbps": 128 } }
```
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "type": "live_file_push", "status": "queued",
  "title": "文件推流：a.mp4 → rtmp://live.example.com/app/***",
  "inputPaths": ["C:\\Videos\\a.mp4"], "outputPath": "", "progress": -1, "speed": "", "etaSec": 0,
  "params": "{\"kind\":\"file\",\"input\":\"C:\\\\Videos\\\\a.mp4\",\"url\":\"rtmp://live.example.com/app/***\",\"loop\":true,\"options\":{}}",
  "version": 1, "createdAt": 1790000000000, "startedAt": 0, "finishedAt": 0 }
```

`StartScreenPush` 请求（带本地存档）：
```json
{ "url": "srt://live.example.com:9000?streamid=abc&passphrase=secret-pass-1", "screenId": "monitor:0",
  "hideCursor": false, "audio": "silent", "archiveDir": "C:\\Users\\me\\Videos\\FFmpegFree",
  "options": { "fps": 30, "videoBitrateKbps": 3000 } }
```

`task:progress`（无存档的直播会话；有存档时没有 `bitrateKbps`）：
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "version": 12, "progress": -1, "speed": "1.00x", "etaSec": 0,
  "outTimeSec": 83.4, "fps": 29.97, "bitrateKbps": 2431.5, "droppedFrames": 0 }
```

`task:status`（优雅停止成功，`error` 为空、不带 `error` 键）：
```json
{ "id": "01J9Z6ZK3Q8V2M4N5P6R7S8T9V", "version": 15, "status": "succeeded",
  "outputPath": "C:\\Users\\me\\Videos\\FFmpegFree\\screen-20260929-203000.mp4", "finishedAt": 1790000090000 }
```


### TaskService
```go
type TaskFilter struct {
    Types    []TaskType   `json:"types"`    // 空 = 不过滤
    Statuses []TaskStatus `json:"statuses"` // 空 = 不过滤
    Limit    int          `json:"limit"`    // 默认 50，最大 200
    Offset   int          `json:"offset"`
    IncludeHidden bool    `json:"includeHidden"` // v0.23：默认 false = 不返回 hiddenInTaskCenter=true 的任务（total 也不算）
}
type TaskPage struct {
    Items []Task `json:"items"`  // 按 createdAt 倒序；无结果时是 []
    Total int64  `json:"total"`  // 符合过滤条件的总数，用于分页
}

ListActive() ([]Task, error)                // 全部 queued + running，供 store 启动用
List(filter TaskFilter) (TaskPage, error)   // 按类型、状态、分页，供任务中心历史用
Get(id string) (Task, error)
Cancel(id string) error
Retry(id string) (Task, error)              // v0.23：原地重试，复用任务 id（规则见 6.6）
Remove(ids []string, deleteOutput bool) error // v0.23：ids 里有 convert 任务时整体 INVALID_ARGUMENT（转换记录只在转换页删，6.14）
ClearFinished() error                       // v0.23：已废弃，等同 HideFinishedInTaskCenter，不再删除记录
GetLog(id string, tailLines int) (string, error)
// v0.23（见 6.14）：
HideFinishedInTaskCenter() (int64, error)                       // 任务中心“隐藏已结束”：只设 hiddenInTaskCenter，返回隐藏条数
UnhideInTaskCenter(ids []string) error                          // 取消隐藏：清 hiddenInTaskCenter，version +1，发 task:status；幂等
CheckPaths(taskIDs []string) ([]TaskPathCheck, error)           // inputExists / outputExists
GetPreviewURL(taskID string, which string) (PreviewURL, error)  // which = "input" | "output"
OpenWithSystem(taskID string, which string) error               // which = "input" | "output"
```

## 5. 事件（runtime.EventsEmit / EventsOn）

| 事件名 | payload | 频率 |
|---|---|---|
| `task:created` | `Task` | 每次 |
| `task:progress` | `{ id, version, progress, speed, etaSec, outTimeSec, fps?, bitrateKbps?, droppedFrames?, encoder?, encoderDevice?, hwFallback?, hwFallbackReason? }`（fps / bitrateKbps / droppedFrames 只有直播任务才有，见下；后四项 v0.18，与 `Task` 同名字段一致，见 9.7） | 每任务最多 4 次/秒 |
| `task:status` | `{ id, version, status, error?, outputPath?, startedAt?, finishedAt?, encoder?, encoderDevice?, hwFallback?, hwFallbackReason?, progress?, result?, retried?, hiddenInTaskCenter?, reconverting?, reconvertOutcome?, lastReconvertError? }`（v0.24：后三项见 6.17.2）（encoder 等四项 v0.18，见 9.7；`progress` / `result` / `retried` / `hiddenInTaskCenter` v0.23，见下） | 状态变化时 |
| `task:removed` | `{ ids: string[] }` | 每次 |
| `ffmpeg:status` | `FFmpegStatus`（见第 9 节） | 检测完成、安装状态变化时 |
| `live:pull` | `{ id, state, error?, hasVideo?, hasAudio? }`（v0.25，6.10.3.7：拉流预览会话的状态：`playing`（只发一次）/ `ended` / `interrupted` / `failed` / `unsupported`；`error` 是 AppError，地址已脱敏；v0.25.3：`playing` 一定带 `hasVideo` / `hasAudio`，其他状态不带；`interrupted` 带 `LIVE_PUSH_INTERRUPTED`，detail 第一行 `reason=pull`） | 状态变化时 |
| `convert:copy` | `{ sourceId, seq, copyState, copiedBytes, totalBytes, storedPath, error? }`（v0.24，6.15.4 第 5 条） | 开始、复制中每个副本最多 4 次/秒、结束时 |
| `doc:component` | `DocComponentStatus`（v0.26，6.12.13，没有路径） | 文档组件状态变化时 |
| `doc:component-progress` | `{ phase, receivedBytes, totalBytes, progress? }`（v0.26，preparing 不带 progress，6.12.15） | 下载中最多 4 次/秒 |
| `doc:queue` | `{ items: [{ id, queuePosition }] }`（v0.26，6.12.18） | 文档组件池队列变化时 |
| `doc:preview` | `{ previewId, state, kind, url?, error? }`（v0.27，6.12.32.1：`generating` / `ready` / `failed`；取消后不再发） | 预览状态变化时 |

**直播指标（v0.10，取代 `live:stats`）**：直播任务的 `task:progress` 除 `speed`（如 `1.00x`，持续明显小于 1 说明编码跟不上）和 `outTimeSec`（已输出的媒体时长）外，还带 `fps`（当前输出帧率）、`bitrateKbps`（**只有没有本地存档的会话才有**：**近 5 秒**平均输出码率，由 ffmpeg `total_size` 和 `out_time` 的增量算出，不用 ffmpeg 自带的 `bitrate=`，那是从开始到现在的累计平均）、`droppedFrames`（ffmpeg 累计丢帧，不是网络丢包）；**有存档的会话没有 `bitrateKbps`（架构师定）**（7.1.5 实测：tee 下 `-progress` 的 `total_size` 和 `bitrate` 恒为 `N/A`，没有可用来源；**不轮询存档文件大小来补**——文件大小含音视频分片和 moov 开销、且不是网络那一路的码率，补出来的数是误导），该字段一律省略，前端显示"—"；`fps` / `droppedFrames` / `speed` / `out_time_us` 在 tee 下正常；`progress` 恒为 -1，`etaSec` 为 0。没有单独的 `uptimeSec`：已推时长 = 现在 − `Task.startedAt`（墙钟），`outTimeSec` 是媒体时间，两者差距变大说明卡顿。这几项同时写进 `Task`（`fps` / `bitrateKbps` / `droppedFrames`，只在内存），页面刷新后 `ListActive` 能立刻显示当前值。

`task:created` 后任务状态为 `queued`；开始执行时发 `task:status`（`running`）；结束时发 `task:status`（终态）。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

`task:status` 的时间字段（Unix 毫秒，值为 0 时省略）：`running` 事件带 `startedAt`、不带 `finishedAt`；所有终态事件（`succeeded` / `failed` / `canceled` / `interrupted`）都带 `finishedAt`，跑过的任务同时带 `startedAt`（与 `Task.startedAt` / `Task.finishedAt` 及落库值一致）。任务从未进入 `running` 就结束（排队中被取消、应用退出时还在排队而被标记为 `interrupted`）时没有 `startedAt`：事件里省略该字段，`Task.startedAt` 为 0，这是正常的，前端不应把它当作错误。崩溃恢复（启动时把残留的 `queued` / `running` 置为 `interrupted`）只落库（写入 `finishedAt`，保留已有的 `startedAt`，`version` +1），不发事件；前端启动后通过 `ListActive` / `List` 拿到最新记录。`task:progress` 的 `version` 与 `task:status` 共用同一个递增序列（每次推送 +1），所以前端按 `version` 丢弃旧事件的规则对两类事件同样适用。

**v0.23 新增的四个字段**：`progress`——所有终态事件和原地重试的 `queued` 事件**一定带**（值为 0 也带，Go 用指针），`succeeded` 为 1，`canceled` / `failed` / `interrupted` 是结束那一刻的值（6.14.6），直播 -1；`running` 事件不带。`result`——只有成功的 `convert` 任务的 `succeeded` 事件带（6.14.2）。`retried: true`——只出现在 `TaskService.Retry` 原地重试发出的那一条 `queued` 事件上：前端收到时把本地这条任务的 `progress`、`speed`、`etaSec`、`startedAt`、`finishedAt`、`encoder`、`encoderDevice`、`hwFallback`、`hwFallbackReason`、`error`、`result` 先清掉、`hiddenInTaskCenter` 置 `false`，再套用事件里带的值（`status`、`version`、`progress: 0`、`outputPath`、新的编码器字段），**`title` / `params` / `inputPaths` / `sourceId` / `createdAt` 不变**；如果本地没有这条任务（例如任务中心没加载到），按 `Get` 拉一次。原地重试**不发** `task:created` / `task:removed`。**`retried` 事件同时清掉上一次运行留下的“已改用 CPU 编码”之类的回退提示**（`hwFallback` / `hwFallbackReason` 被清空，新运行再回退会在之后的事件里重新带上）。`hiddenInTaskCenter`——只出现在 `UnhideInTaskCenter` 发出的事件（值为 `false`，Go 用指针，`false` 也带）和 `retried` 事件上（同样为 `false`）；`UnhideInTaskCenter` 的事件 `status` 是任务当前状态（不变），只有 `hiddenInTaskCenter` 和 `version` 变，前端只改这两个字段。`HideFinishedInTaskCenter` 不发事件（批量，任务中心自己重新 `List`）。

**事件顺序（v0.25.1，架构师定）**：`task:status`、`convert:copy` 的结束事件、`live:pull` 可能比创建这一行的调用（`Submit*`、`AddSources`、`StartPullPreview` 等）先到，也可能彼此乱序。前端对还不在列表里的 id **按 id 暂存**事件，这一行加进列表时再应用；`AddSources` 返回后按这一批的 id 重新查询一次对齐。拉流会话的状态只往终态走，`ended` / `interrupted` 之后不再改变，先到的那个决定状态和文字（6.10.3.7）。

前端任务 store 规则：先 `EventsOn` 订阅并缓存事件，再 `TaskService.ListActive()` 拉取 queued 和 running 任务，拉完按 `version` 回放缓存，版本不大于本地的事件直接丢弃。历史任务只在任务中心里用 `List` 分页加载。`task:progress` 只改进度字段，不替换对象。

## 6. SQLite 表

```sql
media(id PK, path, path_key UNIQUE, name, size, duration, width, height, video_codec, audio_codec, bitrate, probed_at)
tasks(id PK, type, status, title, input_paths JSON, output_path, params JSON, progress, error JSON,
      log_path, version, created_at, started_at, finished_at,
      encoder, encoder_device, hw_fallback, hw_fallback_reason,   -- 这四列：迁移 0004（v0.18），见 9.7；旧行为空 / 0
      source_id, hidden_in_task_center, result JSON, output_name_key) -- 这四列：迁移 0005（v0.23），见 6.14.8；旧行 NULL / 0 / NULL / ''（回填后有 source_id 和 output_name_key）
convert_sources(id PK, path, path_key UNIQUE, name, name_key, added_at, last_activity_at,  -- 迁移 0005（v0.23），见 6.14.8
      kind, sheet_count,                                                      -- 迁移 0009（v0.26），见 6.12.21；kind = media | doc，旧行 media / 0
      media JSON, media_fp,                                                   -- 迁移 0006（v0.23.4），见 6.14.8；旧行 NULL / ''，懒探测补上
      copy_id)                                                                -- 迁移 0007（v0.24），见 6.15.3；NULL = 没有副本
convert_copies(id PK, owner_source_id, original_path, original_path_key, original_size, original_mtime_ns, stored_path,
      state, total_bytes, copied_bytes, error JSON, ref_count, pending_delete, created_at, finished_at)  -- 迁移 0007（v0.24），见 6.15.3
presets(id PK, name, built_in, options JSON, sort)
edit_projects(id PK, name, project JSON, updated_at)
doc_recent(id PK, path, path_key UNIQUE, name, size, opened_at)
cat_conversations(id PK, title, agent_kind, access_mode, project_path, created_at, updated_at,  -- 迁移 0010（v0.30），见 6.19；project_path v0.31 起不再读写
      project_id)                                                             -- 迁移 0011（v0.31），见 6.19.10.7；NULL = 不属于项目
cat_messages(id PK, conversation_id, role, content, created_at)            -- 迁移 0010（v0.30）
cat_projects(id PK, name, path, path_key UNIQUE, created_at, updated_at)   -- 迁移 0011（v0.31），见 6.19.10.7
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
- 输出文件先写 `<name>.part.<原扩展名>`（例如 `a.part.mp4`，保留扩展名让 ffmpeg 能识别封装格式），成功后改名；目标重名时自动追加 `(1)`、`(2)`（`a(1).mp4`，括号前没有空格）；取消或失败删除 `.part`。**v0.23：`convert` 任务在提交时就定名并占位（排队中也占），且重名后缀带空格（`a (1).mp4`），规则见 6.14.5；其他任务类型仍是无空格格式。**
- 进度只保存在内存并通过 `task:progress` 推送，不写库；只有状态变化（开始、成功、失败、取消）时落库，避免单连接下进度写入阻塞任务中心的列表查询。
- 取消转换类任务直接强制结束进程；直播录制存档要先向 ffmpeg 发 `q`（不用 SIGINT，见 6.10），等待它写完文件尾（无存档的直播 5 秒，有存档的 15 秒，见 6.10），超时再强制结束；存档用分片 mp4，强杀后已写出的分片仍可播放，**直播存档强杀后保留**（直接写最终文件名，不走 `RunWithPart`，6.10 实测）。
- `/local/<token>` 用 `http.ServeContent` 输出，支持 Range 请求，保证视频可拖动进度。

## 6.6 任务管理器实现约定（v0.7）

- 包 `internal/task`：`Manager.Submit(Spec, Runner)` 落库为 `queued` 并发 `task:created`；`batch` 池（`internal/ffmpeg` 转换 / 剪辑 / Office / 安装）按并发数 FIFO 排队，默认并发 `min(NumCPU/2, 3)` 且至少 1；`live` 池（两类直播）不排队、不占 batch 名额，且进度恒为 -1。
- 状态机：`queued → running → succeeded | failed | canceled | interrupted`。Runner 返回 nil 即 `succeeded`（含直播优雅停止：存档完整；直播存档在强杀 / 失败 / 中断后也保留，见 6.10）；返回被取消的错误且用户请求过取消为 `canceled`；应用退出时被停止的任务（含还在排队的）为 `interrupted`；其余为 `failed`（`error` 带错误，ffmpeg 失败时 `detail` 为 stderr 最后 50 行）。**进度（v0.23 写明）**：只有 `succeeded` 把 `progress` 置为 1；`canceled` / `failed` / `interrupted` 保留结束那一刻最后一次计算出的值，不清零，随终态落库并在终态 `task:status` 里带出（6.14.6）。
- 只有状态变化落库；进度只在内存。`task:progress` 同一任务最多 4 次/秒，被节流抑制的最后一次会在间隔到期后补发。`ListActive` / `Get` / `List` 返回运行中任务时带实时进度，`Speed` / `EtaSec` 不落库。
- 取消：排队中的直接移出队列变 `canceled`；运行中的取消 `ctx`，ffmpeg 任务结束整个进程组；直播任务发 `q`（不用 SIGINT，见 6.10），最多等 5 秒（有本地存档的直播会话 15 秒；还没连上、没有收到第一条 progress 的会话直接强杀，不发 `q`，见 6.10）再强制结束。已结束的任务取消返回 `TASK_CONFLICT`，不存在返回 `NOT_FOUND`；**旧类型（"保留但不再产生"的类型）的 id 按不存在处理，返回 `NOT_FOUND`**（6.10 确认项 ⑧）。Windows 上结束整个进程树（v0.9.2：先终结进程所在的 Job Object，失败退回 `taskkill /T /F`，再失败只结束主进程）。
- **`task.NeverRanner`（v0.19 补写，描述现有行为）**：`Runner` 可选实现的接口 `NeverRan() string`。**什么时候用**：任务在 `Run` 根本没有执行的情况下就结束时，管理器在发终态事件**之前**调用一次 `NeverRan`，返回值当作该任务的输出路径（含义与 `Run` 的返回值相同：`""` = 保留提交时的预期路径；`task.ClearOutputPath` = 把 `outputPath` 清空；绝对路径 = 采信；相对路径忽略）。目前**只有直播屏幕推流的存档 Runner**（`archiveRunner`）实现它：删掉自己创建的 0 字节占位文件并返回 `ClearOutputPath`；其余 Runner 都没实现，行为是保留预期路径（旧行为）。
  - **触发场景（`Run` 没有执行）**：排队中被 `Cancel`（`canceled`）；`Submit` 与应用退出并发、或在 `task:created` 与入队之间被取消（`interrupted` / `canceled`）；应用退出时还在排队（`interrupted`）；刚出队但 ctx 已被取消（`canceled`，应用正在退出时按“不是用户取消”规则为 `interrupted`）。直播任务不排队，只会走最后两种（刚提交就被取消）。
  - **状态与事件**：终态只可能是 `canceled` 或 `interrupted`，**不会是 `failed` / `succeeded`**，`error` 为空。**没有 `running` 事件、没有任何 `task:progress`**；终态 `task:status` **不带 `startedAt`**（`Task.startedAt` 为 0，事件里省略），带 `finishedAt`；`Task.progress` 保持入队时的值（提交或原地重试入队时都是：非直播 0，直播 -1，不会因终态变成 1；v0.23 起终态事件带 `progress`，就是这个值）。终态落库、发 `task:status` 后调用 `OnFinish`（`Finalizer`）。这类任务从未启动 ffmpeg，也没有 `.part` 文件；日志文件可能不存在（`GetLog` 返回空）。
  - **注意（非直播的例外）**：“刚出队但 ctx 已被取消”这条路径上，非直播任务失败 / 取消时管理器一律丢弃 Runner 返回的输出路径（沿用旧行为，见 `finishAfterRun`），所以 `NeverRan` 的返回值只对直播任务在这条路径上生效；前四种场景对所有类型都采信。
  - **原地重试之后（v0.23）**：`Retry` 把任务重置回 `queued`（下面的 `Retry` 条）时已清空 `startedAt` / `finishedAt` / `progress` / `error` / `result`，所以一条曾经运行过、重试后又在排队中被取消的任务，也完全符合上面的表现：没有 `startedAt`、`progress` 为 0、没有 `running` 事件。前端不能用“以前见过它运行”来推断，要以最新事件为准（`version` 规则不变）。重试入队之前删掉的是**上一次运行**留下的 `.part`，`NeverRan` 本身仍然不删任何东西（直播存档 Runner 除外）。
  - **与 v0.18 编码器字段的关系**：`encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 由 `Submit`（v0.23 起也包括原地重试）在任务落库前从 Runner 的 `EncoderReporter` 一次性写入，而 `NeverRan` 只影响输出路径，所以**从未运行的任务上这四个字段是提交时（原地重试后则是重试时）解析出来的值，不是空**（例如排队中被取消的 h264 转换任务带 `libx264` / `cpu`，所选设备当时不可用的带 `hwFallback=true`、`device_unavailable`；实现了 `EncoderReporter` 才有，纯音频转换、Office 转 PDF、ffmpeg 安装这类没有视频编码器的任务本来就为空）。因为 `Run` 没执行过，**不会发生运行中的硬件编码回退**，不会补发 `running` 事件，所以字段不会再变。前端不要把“这四个字段有值”理解为“这个任务真的编码过”，要看是否有 `startedAt`。
- `Retry`（**v0.23 改为原地重试**，取代“生成新任务”）：**复用原任务的 id**，把同一条记录重置回 `queued` 重新排队；**统一覆盖所有注册了 Factory 的类型，不按类型区分**（目前 `convert`、`office_pdf`、`ffmpeg_install`；v0.23.5 起 `edit_export` 没有 Factory，旧记录 `Retry` 一律 `UNSUPPORTED`（`reason=feature_removed`）；以后新增可重试的类型也按原地重试；v0.22 及之前这里写的“只有 `ffmpeg_install`”已过时）。
  - **允许的状态**：`failed`、`interrupted`、`canceled`（直播类型不管什么状态都是 `UNSUPPORTED`，见上；v0.25.3 起直播中途被中断的 `interrupted` 也一样）。仍在 `queued` / `running` 返回 `TASK_CONFLICT`（`任务仍在进行，请先取消`）；`succeeded` 返回 `TASK_CONFLICT`（`任务已经成功完成，不能重试`；所有类型都一样；转换任务需要重转用 `ConvertService.Reconvert`，v0.24 起是原地的，见 6.17）。没有 Factory 的类型 `UNSUPPORTED`（直播会话同前；v0.23.5：`edit_export` 的 message `剪辑功能已移除，剪辑导出记录不能重试`，`detail` 第一行 `reason=feature_removed`）；不存在 `NOT_FOUND`；**旧类型（"保留但不再产生"的类型）的 id 先判为 `NOT_FOUND`，不落 `UNSUPPORTED`**（6.10 确认项 ⑧）。
  - **步骤**：① 用 Factory 按 `params` 重建 Runner（重新探测输入、重新校验参数；失败直接返回错误，**记录原样不动、不发事件**，如输入已删除 `NOT_FOUND`）；② 走 `RunWithPart` 的类型：删掉上一次运行在原 `outputPath` 上留下的 `.part`（只删普通文件、不是符号链接、修改时间不早于上一次 `startedAt − 3 秒`；上一次从未运行则不删）；③ 重新占位**原输出名**：原 `outputPath` 仍然可用（规则同 6.14.5：磁盘上没有、没有 `.part`、没被其他未结束任务占）就继续用它，否则按该类型的重名格式顺延（`convert` 是 `a (1).mp4`，其他类型是 `a(1).mp4`，见 6.14.5）；④ 重置字段并落库；⑤ 在任务日志末尾追加一行 `[FFmpegFree] 重新开始（重试）`（日志文件和轮转规则不变）；⑥ 发**一次** `task:status`（`status: "queued"`、`retried: true`、`progress: 0`、新的 `outputPath` 和编码器字段）；⑦ 入队。`Claimer` 的 `Submitted` / `Abandoned` 调用时机与 `Submit` 相同。
  - **重置的字段**：`status` → `queued`；`progress` → 0（直播不会走到这里）；`speed` → `""`；`etaSec` → 0；`startedAt` / `finishedAt` → 0；`encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` → 先清空，再按新 Runner 的 `EncoderReporter` 写入（没有就保持空）；`error` → 无；`result` → 无；`outputPath` → 第 ③ 步的结果；`hiddenInTaskCenter` → `false`（任务又在进行了，任务中心要能看到）；`version` +1。
  - **不变的字段**：`id`、`type`、`title`、`inputPaths`、`sourceId`、`createdAt`（所以在列表里的位置不变）、**`params`（含 `presetId` / `presetName` / `paramsSummary`，是提交时的快照，重试不改）**、日志路径。源文件行的 `lastActivityAt` 不更新。
  - **不发** `task:created`，也不发 `task:removed`；前端按 5 节 `retried` 的规则原地更新。
- `Remove(ids, deleteOutput)`：任一 id 仍在进行则整体失败（`TASK_CONFLICT`）；删除记录与日志，`deleteOutput=true` 时删除成功任务的输出文件（仅当输出路径是绝对路径、所在目录及上级不含符号链接、且是普通文件；否则只删记录并在日志里说明）；不存在的 id 忽略；**但 ids 里有旧类型（"保留但不再产生"的类型）记录的 id 时整体返回 `NOT_FOUND`、不删任何记录**（6.10 确认项 ⑧，"不存在的 id 忽略"的例外）；发 `task:removed`。**v0.23**：`Remove` 是**非转换任务**真删的唯一入口（任务中心每行的“移除”，已隐藏的记录用 `List(includeHidden=true)` 才看得到）；ids 里有 `convert` 任务时整体 `INVALID_ARGUMENT`（`转换记录请在格式转换页删除`），转换记录的删除走 `ConvertService.DeleteRecords` / `DeleteSource`（先取消、尽量删、列出没删成的，见 6.14.4）。`ClearFinished` **v0.23 起不再删除任何东西**，等同 `HideFinishedInTaskCenter`（只设 `hiddenInTaskCenter`，**所有类型都一样**）；v0.22 及之前是“只删记录和日志，不删输出”。
- 日志：`<数据目录>/logs/<任务ID>.log`（单个任务最多 16 MB：写满 8 MB 轮转为 `.log.1`，单行最多 8 KB 超出截断），Runner 通过 `task.LogWriter(ctx)` 写入，`GetLog(id, tailLines)` 读取末尾若干行（最多读末尾 1 MB）。
- 输出文件用 `task.RunWithPart`：选出不冲突的最终路径（重名追加 `(1)`、`(2)`，无空格；`convert` 例外，名字在提交时已按 `a (1).mp4` 定好，见 6.14.5），写 `<name>.part.<原扩展名>`，成功后改名，失败或取消删除 `.part`。**例外：直播屏幕推流的本地存档不走 `RunWithPart`**（分片 mp4 直接写最终文件名，强杀 / 失败后保留，见 6.10）。
- `task.FFmpegRunner` + `ffmpeg.Run` 是 ffmpeg 任务的通用执行体：自动加 `-hide_banner -nostats -y -progress pipe:1`（不需要 stdin 时再加 `-nostdin`；直播优雅停止和外部 stdin 的任务不加），解析 `out_time_us` / `speed` / `fps` / `bitrate` / `progress=end`，保留 stderr 尾部，提供错误分类钩子（直播的 `LIVE_*` 分类由直播 PR 提供）；`FFmpegRunner` 目前只支持单次 ffmpeg 调用（两遍编码暂缓，见 v0.7.2），`ProgressBase` / `ProgressScale` 用来把一次调用的进度映射到任务整体进度区间（供以后多步骤任务使用）。
- **子进程回收（v0.9.2）**：所有 ffmpeg / ffprobe 子进程经 `proc.Start` / `proc.Run` 启动。Windows 上会为每个子进程创建一个 Job Object（`JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`）并把进程放进去，Job 句柄由应用进程持有：应用崩溃 / 被任务管理器结束时系统关闭句柄，ffmpeg 和它派生的进程一起被系统结束；进程正常退出后应用关闭句柄（同样结束遗留的子孙）。创建 / 加入 Job 失败不影响启动，此时结束进程树走 `taskkill /T /F`。macOS / Linux 不变（`Setpgid` 进程组；应用崩溃时 ffmpeg 不会被自动回收，读不到 stdin / 管道时通常会自己退出）。`proc.Start` 到加入 Job 之间有极短窗口，窗口内派生的孙进程不进 Job（ffmpeg 启动时不会立刻派生）。**没有 Windows 真机验证**，只有交叉编译和 Linux 上的回退顺序 / 登记表测试，Windows 专属测试文件已写但未运行。
- 启动：`store.MarkInterrupted` 在打开数据库后立即执行（v0.24：在它之前先处理 `reconverting=1` 的转换记录，恢复成 `succeeded` 或按成功收尾，不变 `interrupted`，见 6.17.5），上次未结束的 `queued` / `running` 变 `interrupted`，**不会自动恢复执行**，用户可在任务中心点重试。退出：`Manager.Shutdown` 取消所有任务并等待收尾（最多 8 秒；存在有本地存档的直播会话时最多 16 秒，见 6.10；等待期间前端显示"正在停止…"，超时走强杀），再关闭数据库。**已由 #31 实现**（见 6.10.2 第 3 项）：`app.go` 的 `shutdown` 在有带存档的直播会话时等 16 秒，否则 8 秒。

## 6.7 媒体探测与缩略图实现约定（v0.8）

- 门控：`ffmpeg.RequireProbe()`；缺 ffmpeg 或 ffprobe 时整个调用返回 `FFMPEG_NOT_FOUND`。
- 路径：经 `paths.Normalize`；文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读取权限 `IO_ERROR`，ffprobe 解析失败 / 无流 / 超时（30 秒）`PROBE_FAILED`（detail 是 ffprobe stderr 最后 50 行）。所有 ffmpeg / ffprobe 输入都写成 `file:<路径>`，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。
- `Probe`：`-v error -print_format json -show_format -show_streams`；`width` / `height` 是**显示尺寸**（按 `rotation` 90 / 270 交换），各流的编码尺寸在 `streams[]`；`rotation` 为逆时针角度，取自 display matrix（兼容旧 `rotate` 标签）；`duration` / `bitrate` 缺失（`N/A`）时用各流的值；封面图（`attached_pic`）不算视频画面；`fps` 取 `avg_frame_rate`，为 0 时用 `r_frame_rate`，保留 3 位小数。同一文件（同 `path_key`）再次探测保留原 `id`。失败的文件不入库。最多 4 个文件并行。
- `Thumbnail`：jpg，最大宽度 `width`（默认 320，范围 16~1920，不放大，高度自动取偶数），按旋转元数据转正；`atSec` 超出时长时退回第 0 秒；没有视频画面返回 `INVALID_ARGUMENT`。同时返回缓存文件路径和 data URL。同时最多 2 个 ffmpeg 在生成；相同参数并发只生成一次。
- 缓存：`<数据目录>/thumbs/<sha1(缓存版本, path_key, mtime, size, atSec 毫秒（默认缩略图是固定记号）, width)>.jpg`，先写 `.part.jpg` 再改名；文件的修改时间或大小变化即失效。容量上限 1000 个文件或 200 MB（超过则按最后使用时间从旧到新删到上限的 80%），启动时清理一次并每生成 50 张清理一次，超过 1 小时的 `.part.jpg` 残留会被清掉。`RemoveRecent` 不删缓存。
- `Probe` 附带的默认缩略图（v0.24.2 起）取第一帧，太暗时取前 3 秒里第一张不黑的（规则见文首 v0.24.2 ①），宽 320；`ListRecent` 只在该缩略图已缓存时返回 `thumbUrl`，不会为此启动 ffmpeg。

### 6.7 补充（v0.8.1，媒体服务评审修订）

- **ctx**：`App` 持有根 ctx（`NewApp` 时创建，`shutdown` 第一步取消），`MediaService.Probe` / `Thumbnail` / `ListRecent` / `RemoveRecent` 都用它；应用退出时进行中的 ffprobe / ffmpeg 被结束（`ffmpeg.NewCommand` 设了 `cmd.Cancel = proc.Kill`，取消 / 超时结束整个进程组）。
- **批量**：`Probe` 最多 500 个，超过 `INVALID_ARGUMENT`；**前端应把大批量拆成每批约 50 个**再调用（界面能更快出结果，也方便取消）。`RemoveRecent` 最多 500 个 id，超过 `INVALID_ARGUMENT`。
- **文件类型**：先 `os.Stat` 再打开；目录、FIFO、设备文件、socket 等非普通文件返回 `INVALID_ARGUMENT`（不会阻塞）。只有字幕 / 数据流（如 .srt）、或只有封面图（`attached_pic`）没有真正音频 / 视频流的文件返回 `PROBE_FAILED`；封面图 + 音频（带封面的 mp3）仍是有效的音频文件（`hasVideo=false`），对它直接调 `Thumbnail` 返回 `INVALID_ARGUMENT`（截图 `-map 0:V:0` 排除封面图）。
- **Thumbnail**：`atSec` 上限 1e7 秒（更大的按 1e7 处理，防止毫秒换算溢出）；`atSec` 超出视频长度退回第 0 秒时，缓存文件名和返回值的 `atSec` 都是 `0`（`Thumb.atSec` 永远是实际截图的时间点）；**超时不再重试**（只有"超出长度"这种 ffmpeg 失败才退回第 0 秒重试一次）；缓存被清理线程在"命中检查"和"读取"之间删掉时按未命中处理并重新生成，不返回错误。
- **输出上限**：ffprobe stdout 最多 16 MB（超过 `PROBE_FAILED`，message 为"文件的元数据太大"），stderr 只保留最后 256 KB。
- **`-pattern_type none`**：文件名带 `%` 且是 image2 支持的图片扩展名（jpg / png / bmp / webp / tiff / ppm 等，如 `a%03d.png`）时，在 `-i` 之前加 `-pattern_type none`，避免被当成序列模板；其他文件不加（ffmpeg 7.1 实测 mp4 / gif 等解封装器不认识这个选项，加了会报 "Option pattern_type not found"）。
- **media 表**：每次 `UpsertMedia` 之后只保留最近 1000 条（按 `probed_at` 倒序），更旧的记录被删除（缩略图缓存由自己的容量上限回收）。

## 6.8 RevealInFolder / PickDirectory（v0.7.3，v0.7.7 修订）

- `RevealInFolder(path)`：path 必须是绝对路径（空 / 相对路径 → `INVALID_ARGUMENT`），必须存在（否则 `NOT_FOUND`）。**范围限制（v0.7.7）**：只允许任务表里登记的输出路径（`Manager.IsTaskOutput`），或 `defaultOutputDir` 之内的路径（目录本身可以）；先 Clean 再 `EvalSymlinks`，用真实路径按目录边界比较（Windows / macOS 不区分大小写），其余一律 `INVALID_ARGUMENT`；任务输出本身是符号链接时拒绝；实际打开的是真实路径。Windows 执行 `explorer.exe /select,"<path>"`（路径带双引号，见 v0.21），macOS `open -R <path>`，Linux `xdg-open <所在文件夹>`；path 是文件夹时三个平台都直接打开这个文件夹。命令启动后立即返回，启动失败 `PROCESS_FAILED`。Linux 没有统一的"选中文件"方式，所以只能打开所在文件夹。
- **v0.23.1：格式转换页不再调用 `RevealInFolder(path)`**：源文件用 `ConvertService.RevealSource(sourceId)`，转换记录的输出用 `ConvertService.RevealRecord(taskId)`（只收 id，路径由后端从表里取，平台命令与本条相同，不走上面的范围白名单）。`RevealInFolder` 保留给其他页面，行为不变。（v0.23.2 例外：删除记录后打开“没删掉的文件”所在文件夹，见 6.14.1。）**v0.23.3：`RevealInFolder` 的放行范围加第 3 类**——`DeleteRecords` / `DeleteSource` 在 10 分钟内返回过的失败项 `path`（这条记录登记的输出文件本身，Clean 后精确匹配，符号链接不算，只在内存里、最多 100 个、同一次运行有效）；文件夹本身、同目录的其他文件仍按原来两类判断。
- **v0.24**：第 2 类改为“**实际输出目录或实际上传目录之内**”（6.15.2 第 7 条；设置为空时用默认目录，所以默认目录现在也放行）；`DeleteSource` 删不掉的副本路径同样进第 3 类的 10 分钟放行表（6.15.7）。转换页用默认位置打开“保存到”文件夹用 `OpenStorageFolder("output")`。
- **v0.24.6：第 4 类**——任务表里登记的输出文件所在的文件夹本身（`outputPath` 的目录；Clean 后再按真实路径精确相等，大小写规则同本条；路径本身是符号链接时不算）。只放行这个文件夹，不放行里面的其他文件或子文件夹。转换页完成横幅在本轮“保存到”另选了文件夹时，用 `RevealInFolder` 打开那个文件夹。不在这四类里的，message 仍是 `只能打开任务输出文件或默认输出文件夹里的内容`。
- `PickDirectory(title string)`：弹出系统选择文件夹对话框，返回绝对路径；取消返回 `""`。应用启动完成前调用返回 `INTERNAL`。**参数不能省略**：Wails v2.11 对 Go 可变参数生成 `Array<string>` 且运行时按参数个数严格检查，做不了可选参数，前端无标题时调用 `PickDirectory('')`。
- `PickFiles(filter, multiple)`：`filter = {name, patterns[]}`，patterns 形如 `["*.mp4", "*.mkv"]`（单个元素里用分号也行：`"*.mp4;*.mkv"`），只接受 `*.扩展名` 形式（扩展名限字母数字 `_ - + ? *`）和 `*` / `*.*`，其他写法 `INVALID_ARGUMENT`；patterns 为空或含 `*.*` = 不过滤；`name` 为空时用模式串当显示名。返回绝对路径（已 `Clean`、去重）；**用户取消返回空数组 `[]`，不是错误**；`multiple=false` 最多 1 个。启动完成前调用返回 `INTERNAL`。

## 6.9 ConvertService 实现约定（v0.9）

- 绑定：`ListPresets() []Preset`、`SavePreset(p Preset) Preset`、`DeletePreset(id)`、`Submit(inputs []string, opts ConvertOptions, outputDir string) []Task`；**v0.23 新增**（签名见第 4 节和 6.14.3）：`AddSources`、`ListSources`、`ListSourceRecords`、`SearchSources`、`CheckSources`、`PreviewOutputName`、`SubmitSources`、`Reconvert`、`DeleteRecords`、`DeleteSource`、`GetSourcePreviewURL`、`OpenSourceWithSystem`、`RevealSource`、`GetRecordThumbnail`、`GetSourceThumbnail`；**v0.23.1 新增** `GetSource`、`RevealRecord`（Wails 绑定 `window.go.app.ConvertService.GetSource` / `RevealRecord`，`frontend/wailsjs/go/app/ConvertService` 已重新生成）；**v0.24 新增** `CancelCopy`、`RetryCopy`、`GetFormatCatalog`（`window.go.app.ConvertService.*`），SystemService 新增 `GetStorageDirs`、`SetStorageDirs`、`OpenStorageFolder`（`window.go.app.SystemService.*`），新类型 `StorageDirs`、`StorageDirsUpdate`、`FormatEntry`、`FormatPreset`、`ConvertSubmitResult`、`SkippedSource`、`ReconvertRequest`、`ReconvertError`，`SubmitSources` 返回类型改为 `ConvertSubmitResult`，`Reconvert` 参数改为 `ReconvertRequest`，`ConvertSource` / `SourcePathCheck` / `DeleteFailure` / `Settings` 加字段，实现 PR 重新生成 `frontend/wailsjs` 并让 `check:api`、`vue-tsc` 通过。均返回 `INTERNAL`（"转换服务尚未初始化"）直到启动完成。TaskService 的 v0.23 绑定：`HideFinishedInTaskCenter`、`UnhideInTaskCenter`、`CheckPaths`、`GetPreviewURL`、`OpenWithSystem`（另有 `List` / `Retry` / `Remove` / `ClearFinished` 的行为变更）。
- **预设**：12 个内置（`builtIn=true`，id 形如 `builtin-mp4-h264`：MP4 H.264 / 1080p / 720p / H.265、MKV 只换封装、MOV、WebM VP9、GIF、MP3、M4A、WAV、FLAC；**v0.24 增加到 38 个，每个格式一个默认预设，见 6.16.2**），不能修改或删除（`INVALID_ARGUMENT`，改为"另存为"：`id` 传空新建）。`SavePreset`：`id` 空 = 新建（后端生成 id），否则更新该用户预设（不存在 `NOT_FOUND`）；名称去首尾空白后 1~60 字；`options` 必须通过校验；用户预设最多 100 个；`builtIn` 字段传什么都忽略。`ListPresets` 内置在前，用户预设按创建顺序在后。
- **容器与编码**（v0.24 扩充的容器、编码、各格式默认参数和单帧图片规则见 6.16.2 / 6.16.5，本条描述 v0.9 的部分）：容器 `mp4 mkv mov webm avi flv gif mp3 aac m4a wav flac ogg opus`；视频编码 `copy h264 h265 vp9`，`""` 表示不要视频（仅对视频容器有意义，等于只保留音频）；音频编码 `copy aac mp3 opus vorbis flac pcm ac3 none`，`""` 取容器默认（mp4/mov/mkv/flv=aac，webm=opus，avi=mp3，mp3/aac/m4a/wav/flac/ogg/opus 对应各自编码），`none` 去掉音轨。每个容器只允许合适的编码组合，不合法返回 `INVALID_ARGUMENT` 并在 message 里列出可选项。音频容器不能设分辨率/帧率、不能 `none`；`gif` 没有音轨、不能设码率；`copy` 不能缩放、不能设 crf/码率。
- `width` / `height`：只给一个按比例缩放；两个都给时等比缩进该框内并补黑边；输出宽高保证是偶数。`trimStart` / `trimEnd`（秒，`trimEnd=0` 到结尾）；开始时间超过时长返回 `INVALID_ARGUMENT`。`targetSizeMb` 暂缓，>0 返回 `INVALID_ARGUMENT`。**取值范围**（越界一律 `INVALID_ARGUMENT`）：`trimStart` / `trimEnd` ≤ 1e6 秒；`fps` 为 0（保持）或 0.1~240；`videoBitrate` 0 或 ≤ 1e9 bit/s；`audioBitrate` 0 或 8000~1000000 bit/s；`width` / `height` ≤ 8192。
- **Submit**：一个文件一个 `convert` 任务（batch 池按并发数排队），返回的任务与 `inputs` 一一对应；任务 `title` 形如 `a.mkv → MP4`，`inputPaths` 为源文件，`outputPath` 为预期输出（完成时以实际路径为准，重名会带 ` (n)`，v0.23 起带空格，见 6.14.5），`params` 是 `{input, options, outputDir}` 的 JSON（v0.23 起再加 `presetId`、`presetName`、`paramsSummary` 三个提交时快照，任务另有 `sourceId`，见 6.14；`outputPath` 在提交时就已定名并占位，见 6.14.5）。
  - 一次最多 **50** 个文件（更多由前端分批），空列表 `INVALID_ARGUMENT`。
  - **先整体校验再提交**：ffmpeg 就绪（否则 `FFMPEG_NOT_FOUND`）、参数合法、每个文件都能被探测（`NOT_FOUND` / `PROBE_FAILED` / 目录 `INVALID_ARGUMENT`）且与参数兼容（如无音轨转 mp3、无画面转视频容器 → `INVALID_ARGUMENT`）。任何一个不通过整体失败，**不提交任何任务**，错误 `detail` 第一行是出错的文件路径。视频输入没有音轨转视频容器不算错误，静默不带音频。
  - **输出目录**：`outputDir` 非空用它；为空用**实际输出目录**（v0.24：自定义的 `Settings.defaultOutputDir`，没有则默认输出目录 `<base>/output`，见 6.15.2；v0.23 及之前“仍为空则输出到各自源文件所在文件夹”不再出现）。必须是绝对路径（`INVALID_ARGUMENT`），不能在应用数据目录内（v0.24.1：规则同 6.12.3，`<dataDir>/output` 及其子文件夹允许；不通过 `INVALID_ARGUMENT`，message `输出目录不能在应用数据目录内`），不存在的目录会在任务开始时创建。输出名为 `<源文件名去扩展名>.<新扩展名>`，重名依次追加 ` (1)`、` (2)`（**v0.23 起带空格**：`a (1).mp4`；v0.22 及之前是 `a(1).mp4`，已有的旧文件不改名），绝不覆盖已有文件（含输入文件本身），走 6.6 的 `.part` + 原子改名。**v0.23：提交时定名并占位，未结束任务占着的名字也算重名（6.14.5）。**
  - 进度按输出时长（已扣除裁剪）计算，0~1 单调递增，完成为 1；取消后没有最终文件也没有 `.part`。
- **任务失败的错误码**（在任务的 `error` 上）：`CONVERT_DISK_FULL`（No space left on device / ENOSPC / Windows "There is not enough space on the disk" / 磁盘配额）、`IO_ERROR`（没有权限、只读、找不到路径）、`PROBE_FAILED`（输入损坏）、`PROCESS_FAILED`（缺少编码器或滤镜、编码与格式不兼容，detail 带 ffmpeg 最后 50 行）。
- **Retry**：`convert` 任务注册了重试工厂，用 `params` 重建：重新探测输入、重新校验参数（输入已删除返回 `NOT_FOUND`，记录不变），输出目录用原来解析好的那个（之后改 `defaultOutputDir` 不影响）。**v0.23 起是原地重试**（复用 id、原输出名，见 6.6）；“又转一次”（只用于已成功的记录）用 `ConvertService.Reconvert`（6.14）。
- **错误分类的匹配范围**（v0.9.1）：只看 stderr 尾部里去掉 `Input #` / `Output #` / `Stream mapping:` 及其缩进的元数据、`Stream #`、`Metadata:`、`Duration:` 行之后剩下的行；系统错误文本按行尾匹配；`CONVERT_DISK_FULL` 还要求同时出现写入阶段句式（`Error writing trailer`、`Error muxing packet`、`Error while writing`、`av_interleaved_write_frame`、`av_write_frame`）。所以文件名 / 标题带 `ENOSPC`、`end of file`、`permission denied` 等词不会误分类。`RunWithPart` 在创建输出目录、硬链接 / 重命名提交时遇到磁盘满（Unix ENOSPC / EDQUOT，Windows 112 / 39）也返回 `CONVERT_DISK_FULL`，其余系统错误 `IO_ERROR`。
- **提交阶段失败会保留已提交的任务**：`Submit` 校验全部通过之后才开始逐个提交；若中途某个 `Submit` 失败（例如任务管理器出错），返回值里带着已成功提交的任务列表和错误，这些任务**不回滚**，会照常运行。ctx 被取消时返回 `CANCELED`。
- 输入是文件名带 `%` 的图片（如 `a%03d.jpg`）时，命令里在 `-i` 前加 `-pattern_type none`（与缩略图 / 探测同一规则，只对 image2 图片扩展名），避免被当成序列模板。
- 所有 ffmpeg 输入输出路径都带 `file:` 前缀，以 `-` 开头、含空格、冒号、中日韩字符的文件名都安全。

## 6.10 LiveService 实现约定（v0.10 设计稿，尚未实现）

- **命令行**（`task.FFmpegRunner{Live: true}`，`GracefulStop`，不加 `-nostdin`，由 stdin 发 `q`）：
  - 文件推流：参数顺序 `-protocol_whitelist file [-re] [-stream_loop -1] -i file:<path> [-protocol_whitelist file -f lavfi -i anullsrc=r=44100:cl=stereo] -map 0:v:0 -map <0:a:0 或 1:a> <编码参数> [-shortest] -protocol_whitelist <输出侧白名单> -f <flv|mpegts> [-flvflags no_duration_filesize] <url>`（输出侧命令模板：**RTMP / RTMPS 带 `-flvflags no_duration_filesize`，SRT 不带**，见本条末尾和下面的白名单条）。源文件没有音轨时补静音（很多服务器要求有音频）；始终重编码：`-c:v libx264 -preset veryfast -tune zerolatency -pix_fmt yuv420p -b:v <k>k -maxrate <k>k -bufsize <2k>k -g <2×fps> -c:a aac -b:a <k>k -ar 44100 -ac 2`；`-re` 让 ffmpeg 按源速度读文件；`rtmp(s)` 用 `-f flv`，`srt` 用 `-f mpegts`。**RTMP / RTMPS 输出（不经 tee 时，文件推流和无存档的屏幕推流一致）额外加 `-flvflags no_duration_filesize`**（与已合并的 #31 实现一致：`internal/ffmpeg/live_args.go` 的 `BuildFilePushArgs`、`live_screen_args.go` 的 `BuildScreenPushArgs`，`Scheme != "srt"` 时加）：它只让 flv 头里不写 `duration` / `filesize` 两个占位为 0 的元数据，**只影响 flv 头，对推流无害**（ffmpeg 7.1.5 与 9.0.2 实测：`-protocol_whitelist rtmp,tcp -flvflags no_duration_filesize -f flv rtmp://127.0.0.1:1935/live/x` 推到 MediaMTX 1.21.1 退出码 0，写本地 flv 文件也退出码 0）；**SRT（`-f mpegts`）不加**。完整的 RTMP / RTMPS 输出侧命令串：`-protocol_whitelist rtmp,tcp -f flv -flvflags no_duration_filesize <url>`（RTMPS 的白名单是 `rtmps,tcp,tls,crypto`）；SRT：`-protocol_whitelist srt,udp -f mpegts <url>`。**走 tee 的存档会话**（本条之外，尚未实现，随存档一起做）以上文存档段的选项串为准，本条不改它（tee 下是否加该选项未实测，不加）。
  - **补 `anullsrc` 时必须加 `-shortest`**（架构师定；7.1.5 实测）：3 秒的无音轨源 + `anullsrc`，不加 `-shortest` 跑了 12 秒还没结束（被 `timeout` 杀掉，`anullsrc` 是无限流），加了 0 秒结束、退出码 0；`-re` 播 20 秒的源，加 `-shortest` 后 20 秒自然结束、退出码 0（任务 `succeeded`）；`loop=true` 时 `-stream_loop -1` 配 `-shortest` 仍然无限（实测 8 秒仍在运行），符合"循环直到用户停止"。源文件有音轨时不补 `anullsrc`，也不加 `-shortest`。
  - **`-protocol_whitelist` 按输入和输出分别写**（架构师定；7.1.5 实测）：这个选项**按位置生效**——写在某个 `-i` 前面只管那个输入，写在所有输入之后、输出之前只管输出。① **输入侧必须放行 `file`**：文件输入前写 `-protocol_whitelist file`；如果全局写成 `rtmp,tcp`，`file:` 输入直接失败（`Protocol 'file' not on whitelist 'rtmp,tcp'!`，退出码 234）。`anullsrc` 输入前也写 `file`（无害）；屏幕采集输入（gdigrab / avfoundation / x11grab）不涉及 URL 协议，不需要写（x11grab 前写 `file` 实测无害）。② **输出侧必须另写一次**（#31 实现确认：RTMP 输出侧 **`-protocol_whitelist rtmp,tcp`** 必须写，不得省略），只写输入侧时输出**不受限制**（实测 rtmp 输出照常打开），所以输出侧再限：无存档时 `-protocol_whitelist rtmp,tcp -f flv -flvflags no_duration_filesize <url>` / `-protocol_whitelist rtmps,tcp,tls,crypto -f flv -flvflags no_duration_filesize <url>` / `-protocol_whitelist srt,udp -f mpegts <url>`（SRT 不加 `-flvflags`）（少写 `tcp` 会报 `Protocol 'tcp' not on whitelist 'rtmp'!`，退出码 234）。③ **走 `tee` 时输出侧的 `-protocol_whitelist` 对 slave 无效**（实测：输出侧只放 `file`，rtmp slave 仍能连上），所以要把白名单写进每个 slave 的选项里：`[...:protocol_whitelist=rtmp,tcp]`、存档一路 `[...:protocol_whitelist=file]`（实测 slave 里的限制有效；值里的逗号不用转义）。rtmps 的 `rtmps,tcp,tls,crypto` 只验证了"通过白名单并走到连接被拒"，没有 TLS 服务器可以推流，**rtmps 端到端未验证**。
  - 屏幕推流输入：Windows `-f gdigrab -framerate N [-draw_mouse 0] [-offset_x X -offset_y Y -video_size WxH] -i desktop`；macOS `-f avfoundation -framerate N -capture_cursor 0|1 -i "<设备序号>:none"`；Linux `-f x11grab -framerate N -draw_mouse 0|1 [-video_size WxH] -i <DISPLAY>+<x>,<y>`。输出尺寸补偶数（`scale=trunc(iw/2)*2:trunc(ih/2)*2`），编码参数同上；无存档时输出侧与文件推流完全一致（`-protocol_whitelist <输出侧白名单> -f <flv|mpegts>`，**RTMP / RTMPS 加 `-flvflags no_duration_filesize`，SRT 不加**）；`audio=silent` 时加 `anullsrc`（屏幕采集是无限流，不需要 `-shortest`；7.1.5 + Xvfb + x11grab + `anullsrc` 端到端实测 `q` 后退出码 0、存档可完整解码，含 h264 + aac）。
  - **存档（`archiveDir` 非空，只有屏幕推流）**：用 ffmpeg `tee` muxer 一次编码同时写两个输出，并加 `-flags +global_header`：`-flags +global_header -f tee "[f=flv:onfail=abort:protocol_whitelist=rtmp,tcp]<url>|[f=mp4:onfail=abort:movflags=+frag_keyframe+empty_moov:flush_packets=1:protocol_whitelist=file]file:<存档路径>"`（SRT 时网络一路是 `[f=mpegts:onfail=abort:protocol_whitelist=srt,udp]`；URL 已经保证不含 `|`；`<url>` 和 `<存档路径>` 都要按下面的转义规则处理）。**存档不走 `RunWithPart`**（架构师定）：分片 mp4 **直接写最终文件名**（没有 `.part`、没有成功后改名），**无论 `succeeded` / `canceled`（强杀）/ `failed` / `interrupted` 都保留**（唯一例外见第 2 条：没有任何可播放分片的空壳文件会被删掉）；`outputPath` = 存档最终路径（空壳被删时清空）。逐项（ffmpeg 7.1.5 + MediaMTX 1.21.1 实测）：
    1. **网络那一路必须写 `onfail=abort`**（架构师定）：`tee` 默认 `onfail=continue`，推流地址连不上时 ffmpeg 只打一行 `Slave muxer #0 failed: Connection refused, continuing with 1/2 slaves.`，**退出码 0，存档照写**（实测存档 79 834 字节，任务会变成 `succeeded`）。加 `onfail=abort` 后：RTMP / RTMPS 连接被拒退出码 **145**（stderr 有 `[tee @ …] Slave '…': error opening: Connection refused`、`Slave muxer #0 failed, aborting`），没有存档文件；SRT 连不上退出码 **251**；RTMP 鉴权失败退出码 **255**（`Server error: authentication failed`，判 `LIVE_PUSH_REJECTED`）；推流中途服务器被杀退出码 **224**（`Broken pipe`）。这些行分类为 `LIVE_CONNECT_FAILED` / `LIVE_PUSH_REJECTED` / `LIVE_PUSH_INTERRUPTED` 的规则不变。存档那一路**也写 `onfail=abort`**（写盘失败要让整个任务失败；命令串和文字统一为 `abort`，不再有"不写 onfail"的说法）。**ffmpeg 9.0.2 复测**：`onfail=abort` 下 RTMP 连接被拒同样退出码 145、没有存档文件，带 `onfail=abort` 的 tee 命令（含存档 slave）正常推流退出码 0；另外发现 **9.0.2 里即使不写 `onfail`（默认 continue）连接被拒也退出码 145（7.1.5 上是 0、存档照写）**，也就是 7.1.5 的坑在 9.0.2 上表面消失，但契约仍然要求显式写 `onfail=abort`，不依赖版本行为。
    2. **存档用分片 mp4**（架构师定）：`movflags=+frag_keyframe+empty_moov`（不再是 `+faststart`）。实测：`-f tee` + 分片 mp4 **必须同时加 `-flags +global_header`**，否则文件里没有 H.264 参数、无法解码（解码报 404 行错误、`could not find codec parameters`）；加了之后解码 0 个错误。`flush_packets=1` 让每个分片及时落盘：实测 `kill -9` 后带 `flush_packets=1` 的存档比不带的多保留约 34%（352 192 对 262 180 字节）、解码 0 个错误（不带时最后一个分片是残缺的，解码报 `partial file`）。关键帧间隔 `-g` 已经是 2×fps，分片约 2 秒。**强杀后保留（架构师定）**：分片 mp4 在强杀后仍可播放，所以存档直接写最终文件名，**不走 `RunWithPart`、强杀 / 失败后不删**。**同名处理**：文件名带秒、同一秒内两个会话或重启后仍可能重名，最终名冲突时加 `(n)` 后缀（`screen-20260929-200000(1).mp4`、`(2)`……，规则与 `task.UniquePath` 一致），**选名时用 `O_CREATE|O_EXCL` 建一个空文件占位**（不能只 `Stat` 后再交给 ffmpeg：ffmpeg 带 `-y` 会直接覆盖已有文件，实测 5 496 字节的已有文件被覆盖；对预先建好的空文件写入则正常）。文件名用 #22 的净化函数（见第 4 条）。**空壳**：连接失败（`started=false`，`onfail=abort` 时文件是 0 字节）或过早被杀时文件里只有 moov、没有一个完整分片，不是存档——实测 `kill -9`：0.6 秒和 1.5 秒时文件 1 263 字节、`ffprobe` 读不出时长；3 秒时 32 239 字节、时长 2.08 秒、50 个视频包；6 秒时 61 271 字节、时长 4.08 秒、100 个包（分片约 2 秒，所以头约 2 秒的数据可能还没落盘）。任务结束后（任何终态）`ffprobe` 读不出时长的存档文件**删除**并把 `outputPath` 清空，读得出的保留。**这条"空壳删除"已由架构师确认。** 三条硬性约束：① **只删本任务自己用 `O_EXCL` 创建的文件**（任务里记下占位时创建成功的最终路径；`O_EXCL` 失败换名重试时没创建成功的路径不算），**绝不碰已有文件**，也不删 `archiveDir` 里的其他文件；② **删除失败只记日志（不含推流地址，路径可记）**，**不改任务状态**、不改 `error`；③ **顺序：先清理并清空 `outputPath`，再发终态事件**（**#31 已实现**，见 6.10.2 第 2 项：`task.ClearOutputPath` 是 Runner 可以返回的特殊输出路径，`entry.finish` 遇到它把 `outputPath` 清空；语义是"终态落库和 `task:status` 里的 `outputPath` 为空串"。原先 `entry.finish` 只在 output 非空时才覆盖，清不掉）（`task:status` 和落库的 `Task` 里 `outputPath` 已经是最终值，前端不会看到已经不存在的路径）。**前端规则**：判断"强杀且存档已保留" = `status == "canceled"` 且 `outputPath` 非空（`canceled` 且 `outputPath` 为空 = 强杀且没有可用存档）；正常停止（`succeeded`）的存档**不加**"文件可能不完整"之类提示。测试：强杀后 0.6 秒的空壳被删、`outputPath==""`；3 秒以上的保留、`outputPath` 非空；删除失败（只读目录）任务状态仍是 `canceled` 且 `Task.error == nil`；已有同名文件不被删。
    3. **优雅停止等待延长**（架构师定）：有存档的会话，`q` 之后最多等 **15 秒**（无存档的会话仍是 5 秒）。实测网络正常时 `q` 后 0.11 秒退出、存档完整（时长与关键帧一致、解码 0 个错误），15 秒只是上限。应用退出时同样适用：`Manager.Shutdown` 的总等待上限在存在有存档的直播会话时相应提高到 16 秒（原为 8 秒，见 6.6）。**等待期间前端显示"正在停止…"，超时走强杀**（强杀后的存档按第 2 条保留；任务终态为 `interrupted`）。
    4. **存档文件名固定为 `screen-<yyyyMMdd-HHmmss>.mp4`**（本地时间，架构师定；直接是最终文件名，冲突时加 `(n)`，见第 2 条），不再用 `<title>`，文件名里没有任何用户输入。仍要过一遍共用的文件名净化规则作为纵深防御（规则见 6.11.3「输出文件名」，直播额外禁止 `| ' [ ]`；对这个固定格式是恒等变换，测试断言"净化前后相等"）——**该规则由 #22 引入。合并顺序与实现依赖（架构师定）：#22 先合；直播存档的实现放在 #22 合入之后的提交里，文件推流部分不用等 #22；文档层面 #19 先合也无妨，这里只是引用，依赖的是实现。**用户可控的只有 `archiveDir`，它靠下面的转义规则保证安全。
- **`tee` 段转义规则**（架构师定；7.1.5 实测）：`tee` 的 slave 描述里 `|` 是 slave 分隔符，`'` `\` `[` `]` `,` `:` `=` 等是选项语法，**不转义会静默写到错误的位置或直接失败**。实测（Linux，未转义）：文件名或目录里的 `|` 把路径拆成两个 slave，`a|b.mp4` 实际创建了 `a` 和 `b.mp4` 两个文件；`'` 被吞掉（`q'x.mp4` 变成 `qx.mp4`，目录里有 `'` 则找不到目录，退出码 254）；`\` 被当转义符吞掉（`C:\Users\x\o.mp4` 变成文件 `C:Usersxo.mp4`）；目录里有 `[` 打不开（退出码 254）；文件名里的 `[` `]` `,` `=` `;` `:` 空格中日韩字符没问题，但不能靠这个判断——统一转义。规则：
  1. **路径统一**：`file:` 前缀 + 正斜杠（Windows 先把 `\` 换成 `/`，再转义）。拒绝 `\\?\`、`\\.\` 开头的路径（`INVALID_ARGUMENT`）。
  2. **转义函数**：把 `<存档路径>` 和 `<url>` 里所有**不在 `[A-Za-z0-9_./-]` 也不是非 ASCII 字符**的字符前面加一个 `\`（包括 `\ ' | [ ] , : = ; 空格 # ? % & ( )` 等，Windows 盘符冒号写成 `C\:/…`）。非 ASCII（中日韩等）不转义，实测可用。
  3. **实测**：目录 `录 屏'|[x],y=z;c:d#f?g%h&i(1)`（含以上所有特殊字符），转义后 `[f=mp4]file:<转义路径>/screen-20260929-200000.mp4` 退出码 0、文件建在期望位置；同一个路径不转义退出码 254。URL 转义后也正常（`rtmp\://127.0.0.1\:1935/live/esc\?k\=v\&x\=1` 能连上），未转义的含 `?a=b,c=d;e=f&t=1` 和 IPv6 `[::1]` 的 URL 在 tee 里也能被正确解析，但为了统一，**一律走转义函数**。
  4. **测试**：转义函数表驱动（上面每个特殊字符、中日韩、空格、Windows 盘符路径、UNC 路径）；集成测试用真实 ffmpeg 往含特殊字符的临时目录写存档，断言文件出现在期望路径且没有多余文件。
  5. **【未验证】**：Windows 真机上 `file:C\:/Users/…` 的解析（Linux 上盘符冒号只是普通字符，无法验证 Windows 的盘符语义）；UNC 路径 `//server/share/…`。这两项需要用户在 Windows 上验证。
- **采集能力检测**（`GetCaptureCapabilities` / `ListScreens` / `StartScreenPush` 共用一套逻辑）：Linux 读 `XDG_SESSION_TYPE`——`wayland`（即使有 XWayland 的 `DISPLAY`）或没有 `DISPLAY` → `supported=false`，`Start*` / `ListScreens` 返回 `UNSUPPORTED_PLATFORM`，不去录黑屏；macOS 需要"屏幕录制"授权，授权记在 FFmpegFree 名下（不是 ffmpeg 名下），查不出来时 `permission=unknown`，`Start` 时按 ffmpeg 报错分类为 `SCREEN_PERMISSION_DENIED`。显示器列表：Windows 用 `EnumDisplayMonitors`（`golang.org/x/sys/windows`，不用 cgo）；macOS 解析 `ffmpeg -f avfoundation -list_devices true -i ""` 里的 `Capture screen N`；Linux 解析 `xrandr --display $DISPLAY --query`（**必须显式带 `--display $DISPLAY`**，与已合并的 #31 实现一致（`internal/service/live/screen.go` 的 `listX11Screens`：`xrandr --display <DISPLAY> --query`）；`GetCaptureCapabilities` 用的是同一个 `DISPLAY`（为空则 `supported=false`）；不依赖子进程继承的环境变量，`$DISPLAY` 取的是与采集输入 `-i <DISPLAY>+x,y` 同一个值，保证枚举的屏幕和采集的屏幕是同一个 X server；`DISPLAY` 打不开时 `xrandr` 报 `Can't open display`，按"没有 xrandr"同样降级），没有 xrandr 或它失败时只返回一个 `x11:desktop`。
- **错误分类**：`ffmpeg.ClassifyLiveError`，规则和 6.9 一样——只看 `classifiableLines()` 剔除 `Input #` / `Output #` / `Stream mapping:` / `Metadata` / `Stream #` 段落后的行，系统错误文本按行尾匹配（tee 会话的失败行形如 `[tee @ …] Slave '[f=flv:onfail=abort:…]rtmp://…': error opening: Connection refused`，slave 描述里带完整 URL，**日志和 detail 里必须已脱敏**）。区分"连接失败"和"中断"靠 Runner 记的 `started`：**收到第一个 `progress=continue` 且 `out_time_us > 0` 的 `-progress` 块之后为 `true`**（架构师建议用 `total_size > 0`，**后来已接受改为本判据**；7.1.5 实测：**走 tee（有存档）时 `total_size` 和 `bitrate` 恒为 `N/A`**，用 `total_size` 会让有存档的会话永远"没开始"；无存档时 `total_size` 是数字（首块 4 985 字节、`out_time_us=80000`），两个判据同时成立，所以统一改用 `out_time_us`）。**`progress=end` 块不算开始**：连接失败时 ffmpeg 也会补一个 `frame=0 total_size=0 out_time_us=N/A progress=end`。Runner 在 `started` 之前不发 `task:progress`，所以前端"收到第一条 progress = 已在推"的判断与此一致。
  - `started=false`：无法解析主机名 / 连接被拒绝 / 超时 / 网络不可达 → `LIVE_CONNECT_FAILED`；服务器明确拒绝 → `LIVE_PUSH_REJECTED`。**按 ffmpeg 7.1.5 + MediaMTX 1.21.1 实测**：RTMP 鉴权失败的 stderr 是 `[rtmp @ …] Server error: authentication failed` + `Error opening output …: Operation not permitted`（可区分，判为 `LIVE_PUSH_REJECTED`）；RTMP 服务器未开 / 连接被拒是 `Connection refused`，域名解析失败是 `Failed to resolve hostname`（均为 `LIVE_CONNECT_FAILED`）；**SRT 是已知局限**：服务器未开与被拒绝（错误 passphrase / 无权限）在 ffmpeg stderr 里都只有 `Connection to srt://… failed: Input/output error`，无法区分，统一判 `LIVE_CONNECT_FAILED`（`LIVE_PUSH_REJECTED` 对 SRT 实际上不会出现）。
  - `started=true`：`Broken pipe` / `Connection reset` / `Connection timed out` / 写出时 `Input/output error`、`Error writing trailer`（网络中断）→ `LIVE_PUSH_INTERRUPTED`。**v0.25.4**：这类和认不出来的中断，`detail` 第一行都是 `reason=push`（其后是脱敏 stderr；没有 stderr 时只有这一行）。所选窗口没了仍是 `LIVE_SOURCE_GONE`（`kind=window`），不加 `reason=push`。
  - 屏幕采集：avfoundation 权限相关报错 → `SCREEN_PERMISSION_DENIED`；`x11grab` 打不开显示 → `UNSUPPORTED_PLATFORM`。
  - 认不出来的非零退出 → `INTERNAL`（不是 `PROCESS_FAILED`），stderr 最后 50 行（已脱敏）放 `detail`。**v0.25.3：只限 `started=false`**；`started=true` 时认不出来的（进程被外部强杀、崩溃）→ `LIVE_PUSH_INTERRUPTED`，message `推流被中断，请回到直播页重新推流。`，`detail` 第一行 `reason=push`（v0.25.4）。
  - **任务终态（v0.25.3，架构师定：直播任务的终态要和直播页一致）**：`started=true` 之后的非正常结束（上面任何一种，含中途显卡编码失败、所选窗口没了）→ 任务 `interrupted`（不是 `failed`），`error` 是 `LIVE_PUSH_INTERRUPTED`（所选窗口没了是 `LIVE_SOURCE_GONE`），保证不是 `INTERNAL` / `PROCESS_FAILED`。`started=false` 时的失败仍是 `failed`。实现：直播 Runner 把开始后的错误包成 `task.Interrupted(err)`，任务管理器对直播类型的这种错误记 `interrupted`（其他类型照旧 `failed`）。用户取消、应用退出的规则不变（取消优先）。
  - **已请求 `Cancel` 之后一律归 `canceled`**（架构师定）：Runner 一旦收到取消（用户 `Cancel` 或应用退出的 ctx 取消），之后 ffmpeg 无论以什么非零码退出（`Broken pipe`、`Connection reset`、被强杀……）都返回取消错误 → 任务 `canceled`（应用退出为 `interrupted`），`error` 为空，**不做错误分类**，不得落 `LIVE_PUSH_INTERRUPTED`、`LIVE_CONNECT_FAILED`、`INTERNAL`。只有"优雅停止成功且退出码 0"才是 `succeeded`。测试：取消后让假 ffmpeg 以 224 / 251 / 255 退出，断言状态是 `canceled` 且 `error == nil`。
  - **连接阶段取消直接强杀**（还没有 `started`）：不发 `q`、不等 5 秒，直接结束进程组。原因（实测）：ffmpeg 卡在连接里时不读 stdin，往不通的地址（`10.255.255.1`）推流，2 秒后发 `q`，它又过了约 3 秒才因连接超时自己退出（退出码 146）。
  - 用户主动停止：优雅退出成功 `succeeded`；超时强杀 `canceled`（没有错误码）。**具体的 ffmpeg 报错措辞（各版本、各服务器不同）必须用真实推流服务器收集样本后落成测试**，设计稿里的关键词只是起点。
- **脱敏**（`internal/live`，纯函数，必须有表驱动测试）：
  - `RedactURL(raw string) string`：用户信息 → `***@`；rtmp / rtmps 保留 host、端口和第一段路径（应用名），其后的路径（流名，可含 `/`）→ `***`；所有查询参数保留键、值 → `***`；fragment 去掉；解析失败返回 `<invalid-url>`，绝不回显原文。例：`rtmp://u:p@h:1935/live/abc123?token=xyz` → `rtmp://***@h:1935/live/***?token=***`，`srt://h:9000?streamid=a&passphrase=b` → `srt://h:9000?streamid=***&passphrase=***`。
  - `NewRedactor(rawURL string) func(line string) string`：处理一行输出——① 原始 URL、它的 URL 编码 / 解码形式、以及从中提取的每个秘密片段（用户名、密码、流名、每个查询值，长度 ≥ 3）按字面替换成 `***`；② 再用正则 `(rtmps?|srt|tcp|tls|udp)://[^\s'"<>]+` 把行里残留的任何 URL 交给 `RedactURL`（覆盖 ffmpeg 改写后的形式，如 `tcp://host:1935?tcp_nodelay=0`）。
  - **覆盖范围**：`ffmpeg.RunOptions` 增加 `Redact func(string) string`，`Run` 在 stderr 每一行进入 `TailBuffer`、`OnStderr`（任务日志）、`Classify` **之前**先过它；记录的命令行也用脱敏后的 URL；`task.Spec.Title`、`Spec.Params`、`apperr` 的 `message` / `detail`、事件 payload、后端 `logf` 全部只用脱敏后的值。完整 URL 只存在于 Runner 的内存和 ffmpeg 的命令行参数里，不落库、不写文件。
  - **`params`（脱敏，不能用来重试）**：文件推流 `{"kind":"file","input":"/abs/a.mp4","url":"rtmp://h/live/***","loop":true,"options":{...}}`；屏幕推流 `{"kind":"screen","screenId":"monitor:0","url":"srt://h:9000?streamid=***","hideCursor":false,"audio":"none","archiveDir":"","options":{...}}`。
  - **测试要求**：URL 表驱动（各协议、userinfo、多段路径、IPv6、非法串）；行脱敏用真实 ffmpeg 输出样本；端到端用假 ffmpeg 脚本把完整 URL 打到 stderr 并失败，断言 `Task.title` / `Task.params` / `error.message` / `error.detail` / 日志文件 / 全部事件 payload 里都搜不到任何秘密片段。
  - **已知限制**：ffmpeg 命令行里必须有完整 URL，同一台机器上的其他进程（任务管理器、`ps`）能看到；应用不能规避，文档里说明。
  - **日志规则**：后端**不得**输出 `cmd.Args` / `cmd.String()` / `exec.Cmd` 的任何格式化结果（含调试日志、panic 信息、`%v` / `%+v`），记录命令行只能用已脱敏的副本；发布版关闭 Wails 的调试日志（`logger.DEBUG` 级别、`options.App.LogLevel` 设为 `ERROR`/`INFO`，`Debug` 相关开关关闭），因为 Bind 调用的参数会被它记录；**前端不得把完整推流 URL（含流名、口令）存进 `localStorage` / `sessionStorage` / IndexedDB**，需要记住地址时只存脱敏后的 `PushURLInfo.redacted`，密钥由用户每次输入。
- **指标**：`task.Progress` 增加 `Fps float64`、`BitrateKbps float64`、`DroppedFrames int64`，`task.ProgressEvent` 和 `Task` 增加同名字段（`omitempty`；**#31 已实现**，见 6.10.2 第 4 项：三个字段已在 `origin/v2` 的 `task.Progress`、`task.ProgressEvent`、`Task` 上）；`FFmpegRunner` 从 `ffmpeg.ProgressUpdate`（已有 `Fps`、`Dropped`、`TotalSize`、`OutTimeSec`）填充，`BitrateKbps` **只在无存档的会话里计算**：用相邻两次 progress 的 `total_size` / `out_time` 增量做 5 秒滑动平均；有存档的会话（tee，`total_size` 恒为 `N/A`）不计算、不轮询文件大小，保持 0 → 因 `omitempty` 不出现在 JSON 里（`out_time` 不增长时沿用上一个值，不出现 NaN / Inf）。其余节流、`version`、丢弃旧事件规则不变。
- **会话与任务管理器**：新增 `TypeLiveScreenPush`，`IsLive` 包含它；旧的 `TypeLiveRelay`、`TypeLiveRecordPush` 常量**保留但不再产生**（架构师定，见下方确认项 ⑧）：`Submit` 不再接受，`IsLive` 对它们仍为 true 只是为了常量兼容。不注册重试工厂，`live_file_push` / `live_screen_push` 的 `Retry` 得到 `UNSUPPORTED`（message：直播会话不能重试，请重新开始推流）；旧类型（`live_relay`、`live_record_push`）的 id 调 `Retry` / `Get` / `Cancel` / `Remove` 一律 `NOT_FOUND`，见下方确认项 ⑧。进行中的会话同时最多 4 个；同一个标准化推流地址同时只能有一个会话；**屏幕推流同一时间最多 1 路**（都是 `TASK_CONFLICT`，`detail` 首行 `reason=max_sessions` / `duplicate_url` / `screen_busy`，判断顺序 `duplicate_url` → `screen_busy` → `max_sessions`，见下方确认项）。应用退出：`Shutdown` 取消 → 优雅停止最多 5 秒（有存档 15 秒，总等待 16 秒；前端显示"正在停止…"，超时走强杀）→ 状态 `interrupted`（有存档时存档按 6.10 保留）；应用崩溃时 ffmpeg 子进程由操作系统回收（Windows 见 Job Object 修订）。
- **【未验证】（设计稿的已知风险，实现时要真机验证，汇总见 6.10.1「真机试用清单」）**：macOS 屏幕录制授权的检测方式（不用 cgo 时只能靠 ffmpeg 报错或首帧内容判断）；Windows gdigrab 在多显示器 / 非 100% 缩放下偏移和尺寸是否等于物理像素；`x11grab` 在各桌面环境下的表现；上面所有 ffmpeg 报错关键词；RTMP / SRT 在不同服务器（nginx-rtmp、SRS、MediaMTX、常见直播平台）上的兼容性。

- **已确认项**（原待定项 ①~⑨，不再待定）：
  - ①~⑦ **产品经理和架构师已正式确认**：① 同时进行的直播会话上限 4 个、同一标准化地址只允许一个会话；**（产品经理追加）屏幕推流同一时间最多 1 路，文件推流不受影响**；② 屏幕推流首版不采集声音（只有 `none` / `silent`）；③ 始终重编码（不支持 `-c copy` 直推文件）；④ 允许推到回环 / 内网地址；⑤ 只支持 rtmp / rtmps / srt，不含 rtsp / whip / http-flv 推流；⑥ 存档只用 mp4，且只有屏幕推流有存档；⑦ 优雅停止成功记 `succeeded`、强杀记 `canceled`，前端只看 `status`（硬性规则见第 4 节「结果语义」）；**优雅停止与自然播完都是 `succeeded`，都显示"已结束推流"，不区分、不加字段**。
  - ⑧ **架构师定**：任务中心**不展示** `live_relay` 和 `live_record_push`；这两个旧类型在契约里标为"保留但不再产生"（`Submit` 不接受）；数据库里若有旧记录，一律按未知类型**忽略、不报错**（`List` / `ListActive` 等读取路径遇到类型不在当前枚举内的行时跳过，不返回错误、不影响其他记录；**#31 实现：忽略发生在 store 层，对旧类型的 id 调 `Get` 返回 `NOT_FOUND`**，就当这条记录不存在，不返回"未知类型"之类的新错误；**架构师定：旧类型（`live_relay`、`live_record_push`、`edit_render` 等一切"保留但不再产生"的类型）的任务 id，`Get`、`Cancel`、`Remove`、`Retry` 四个方法一律返回 `NOT_FOUND`**，逐个写死：
    - `Get(id)`：`NOT_FOUND`（就当这条记录不存在，不返回"未知类型"之类的新错误、不返回 `UNSUPPORTED`）。
    - `Cancel(id)`：`NOT_FOUND`（不是 `TASK_CONFLICT`；旧类型没有进行中的会话，也不会有）。
    - `Remove(ids, deleteOutput)`：ids 里**只要有库里存在的旧类型记录的 id，整体返回 `NOT_FOUND`，什么都不删**（与"任一 id 仍在进行则整体失败"同一套整体失败语义）。这是"不存在的 id 忽略"（6.6）的**例外**：真正不存在的 id 仍然忽略、不报错，只有"库里有这条记录、但类型是旧类型"的 id 才报 `NOT_FOUND`；旧记录本身留在库里不删（任务中心不展示，也没有入口删）。
    - `Retry(id)`：`NOT_FOUND`（**先于**"没有重试工厂返回 `UNSUPPORTED`"判断：旧类型按不存在处理，不落 `UNSUPPORTED`）。
    - **实现方式（与已合并的 #31 一致，`internal/store/tasks.go`、`internal/task/ops.go`；措辞以代码为准）**：**由 store 的 SQL 读取层过滤**——`legacyTypes`（目前是 `live_relay`、`live_record_push`；`edit_render` 待 #30 合入后加进同一处）拼成 `legacyTypesSQL`，所有读取和按 id 操作的 SQL 都带 `type NOT IN (…)`：`GetTask`（所以 `Manager.Get` / `Cancel` / `Retry` 拿到"没找到"，得到 `NOT_FOUND`）、`ListTasks` / `ListActive` 的 `taskWhere`（列表不含旧类型）、`DeleteTasks`（逐个 id 的 `DELETE … AND type NOT IN …`，旧类型 id 不会被删）、`DeleteFinishedTasks`（`ClearFinished` 的实现，查询和删除都带 `type NOT IN …`，**旧类型记录不被清掉**）。**`Remove` 是特例**：`Manager.Remove` 在做任何事之前先调 `Store.LegacyTaskIDs(ids)`（返回"库里有这条记录、但类型是旧类型"的 id，真正不存在的 id 不在其中），**只要有一个，整体返回 `NOT_FOUND`，什么都不删**（记录、日志、输出文件都不碰）。**`ClearFinished` 和 `DeleteTasks` 同样排除旧类型**（上面的 SQL）。（v0.23：`ClearFinished` 改为只隐藏，与 `HideFinishedInTaskCenter` 一样不再调用 `DeleteFinishedTasks`；隐藏用的 `UPDATE` 同样带 `type NOT IN (…)`，旧类型记录不受影响。）新增旧类型只改 `legacyTypes` 这一处；`IsLegacyType(t)` 用于判断。**测试**：库里插入 `live_relay` / `live_record_push` 各一条（`edit_render` 待 #30），断言 `Get` / `Cancel` / `Remove` / `Retry` 都返回 `NOT_FOUND`，`List` / `ListActive` 不含它们，`ClearFinished` 后它们仍在库里，且 `Remove` 整体失败时同批里合法的 id 没有被删
  - ⑨ **前端负责**：由前端在 `v2-fe-api-contracts` 里补全 `AppErrorCode`（`CANCELED`、八个 `LIVE_*` 相关码、`PROBE_FAILED`、`UNSUPPORTED`、`CONVERT_DISK_FULL`），并对照第 2 节契约错误码表逐项核对。后端不改动。
- **SRT 说明（架构师 / 产品定）**：SRT 连接失败**统一判 `LIVE_CONNECT_FAILED`**（原因见上文实测：服务器未开与被拒绝在 ffmpeg stderr 里无法区分）。产品文案"连接失败，请检查地址和口令是否正确"由**前端负责**，后端 `message` **不承载该文案**（后端 `message` 只描述技术原因，`detail` 是脱敏后的 stderr 尾部）。 前端据 `LIVE_CONNECT_FAILED` 的 `detail` 第一行 `scheme=srt`（RTMP 为 `scheme=rtmp` / `rtmps`）选文案，见 2.2。
- **用户可见提示（来自产品经理，仅供前端参考；后端只保证错误码和触发条件，不返回这些文案）**：
  - `TASK_CONFLICT`：进行中的直播会话已达 4 个 → 前端提示"最多同时推 4 路"；同一标准化地址已有进行中的会话 → "这个地址已经在推流"。两种触发共用同一个错误码，**用 `detail` 第一行区分（架构师已确认，稳定枚举）**；**产品经理追加：屏幕推流同一时间最多 1 路**，已有进行中的屏幕推流时再开一路 → `reason=screen_busy`（**文案由前端负责，后端不写、不返回**）。三个取值：
    - `reason=screen_busy`：**已有进行中的 `live_screen_push`，再调 `StartScreenPush`**（产品经理定"屏幕推流同一时间最多 1 路"）。只有 `StartScreenPush` 会得到它；**文件推流不受影响**（有屏幕推流在进行时仍可 `StartFilePush`，只受 4 路上限和地址唯一约束）。"进行中"指状态 `queued` / `running` 的 `live_screen_push`（含正在优雅停止、还没到终态的）。
    - `reason=max_sessions`：进行中的直播会话已达 4 个上限。
    - `reason=duplicate_url`：同一标准化推流地址已有进行中的会话。
    - **判断顺序（写死，架构师定）**：依次判断 **`duplicate_url` → `screen_busy` → `max_sessions`**，命中第一个就返回，不继续判断；`StartFilePush` 没有 `screen_busy` 这一步，只判断 `duplicate_url` → `max_sessions`。例如已有一路屏幕推流、又用**同一地址**开屏幕推流，得到的是 `duplicate_url` 而不是 `screen_busy`；已有屏幕推流、再用**不同地址**开屏幕推流，得到 `screen_busy`（即使这时总数已达 4 路也是 `screen_busy`，不是 `max_sessions`）；没有屏幕推流、总数已达 4 路时开新的推流，得到 `max_sessions`。
    - **稳定枚举规则**：`detail` 第一行固定为 `reason=<值>`，整行只有这一个键值对。以后新增取值**只能追加、不能改名、不能改含义、不能删除**；追加要走契约版本变更并在此列出。
    - 适用范围：`StartFilePush` / `StartScreenPush`（以及复用同一检查的 `CheckPushURL`，如果它做会话冲突检查）因会话冲突返回的 `TASK_CONFLICT`。`Cancel` 已结束会话、`Remove` 进行中任务等其他 `TASK_CONFLICT` 不属于这两个取值，**不带 `reason=` 行**（沿用原有 detail）。
    - **(a) 测试要求**：必须有测试分别触发三种冲突，各自断言 `detail` 第一行**精确等于** `reason=screen_busy` / `reason=duplicate_url` / `reason=max_sessions`（不是包含），**另有测试断言判断顺序**（同时满足 `duplicate_url` 和 `screen_busy` 得 `duplicate_url`；同时满足 `duplicate_url` 和 `max_sessions` 得 `duplicate_url`；同时满足 `screen_busy` 和 `max_sessions` 得 `screen_busy`），**断言文件推流在有屏幕推流进行时不返回 `screen_busy`**，并断言两者的 `code` 都是 `TASK_CONFLICT`；同时断言未触发冲突的其他 `TASK_CONFLICT`（如已结束会话再 `Cancel`）不带 `reason=`。
    - **(b) 脱敏要求**：这三种 `detail` 里**不得出现推流地址、口令、streamkey、streamid 或其任何片段**；`duplicate_url` 也不带地址（哪怕是脱敏后的地址、host 或端口），可以在第二行起写不含地址的说明（如"已有会话使用同一推流地址"）。测试要用带秘密片段的 URL 触发这两种冲突，断言 `message` / `detail` / 事件 / 日志里都搜不到秘密片段和 host。
    - **(c) 前端规则**：前端遇到**未知的 `reason` 值或没有 `reason` 行**时，显示通用冲突提示（如"操作冲突，请稍后再试"，文案由前端定），不得猜测含义、不得报错崩溃。
  - `LIVE_URL_INVALID`：协议不是 rtmp / rtmps / srt 时**后端已经是这个码**，`detail` 第一行 `reason=scheme_unsupported`，前端提示"暂不支持这种推流地址，请使用 rtmp、rtmps 或 srt"。同一个码的其他原因用 `reason=malformed` / `missing_host` / `param_not_allowed` 区分（枚举和规则见 2.2，未知值走通用文案），**不要靠 `message` 文本区分**。

- **~~预览画面（v0.17）~~ v0.25 作废，由 6.10.3 取代；下面是原文，只作历史记录。（v0.17，架构师定；实现：`internal/ffmpeg/live_preview.go`、`internal/service/live/preview.go`）**：
  - **预览输出**：推流命令在**主输出之后**追加一路独立输出 `-map 0:v:0 -an -sn -dn -vf fps=2,scale=640:-2 -q:v 5 -protocol_whitelist file -f image2 -update 1 -atomic_writing 1 file:<预览路径>`。**v0.24.5 起包在 `fifo` 封装里**（`-c:v mjpeg ... -f fifo -fifo_format image2 -format_opts update=1:atomic_writing=1 -queue_size 4 -drop_pkts_on_overflow 1 -attempt_recovery 1 -recover_any_error 1 -recovery_wait_time 1 -max_recovery_attempts 0 file:<预览路径>`，见 v0.24.5 ②），预览写慢或写失败都不会拖慢或中断推流。有自己的 `-vf`（不复用主输出的滤镜链，宽 640、高按比例取偶数、每秒 2 帧）；不影响主输出的编码参数、`-progress` 与码率统计。**带存档的屏幕推流：预览输出在 tee 之外**，仍是主输出（`-f tee`）之后单独的一路，不写进 tee 描述（测试断言 tee 描述里没有预览、且只有网络与存档两路）。文件推流、屏幕推流（含 Windows gdigrab 窗口采集、存档）用同一个 `PreviewOutputArgs`。
  - **CPU 说明**：预览输出与主输出共享同一路解码（ffmpeg 的输出端只多一个 fps + scale + mjpeg 编码，每秒 2 帧，开销很小）。当前直播主输出始终重编码（不用 `-c copy`），所以没有额外的解码次数；**如果以后加入"视频流复制（`-c copy`）"或"硬字幕"的推流场景，主输出不解码时预览输出需要单独解码（ffmpeg 会为预览输出自己起解码器），会额外占少量 CPU，那时按需要再评估默认是否关预览。** **【未验证】**高分辨率（4K）屏幕采集、Windows 真机上的预览耗时与 CPU 占用。
  - **读取与半帧**：`-atomic_writing 1` 让 ffmpeg 先写 `<路径>.tmp` 再改名，读取端不会读到半帧；`GetPreview` 读取时仍校验 JPEG：以 SOI（`FF D8`）开头、以 EOI（`FF D9`）结尾（允许末尾少量 0 填充），大小 4 字节~4 MiB，不合格（半帧、空文件、不是 JPEG）一律当没有画面返回空，不返回错误。`ts` 取文件修改时间。
  - **`GetPreview(sessionId)`**：`sessionId` 是推流任务 id（= 会话 id）或 `StartPullPreview` 返回的拉流预览会话 id。返回 `{data, ts, active}`：会话不存在 / 已结束、`preview=false`、ffmpeg 还没出第一帧、读到半帧、预览文件不存在，都是**空 data + ts=0，不是错误**；`active` = 会话是否还在进行（推流：任务没结束；拉流：会话没结束），前端在 `active=false` 或页面不可见时停止轮询，约 500 毫秒一次。预览关闭时 `GetPreview` 返回空（推流会话仍 `active=true`）。
  - **开关**：`FilePushRequest.preview` / `ScreenPushRequest.preview` / `PullPreviewRequest.preview` 是 `*bool`，缺省（nil）= true，false = 不加预览输出。**只在开始时决定**（ffmpeg 已启动无法动态改输出），没有 `SetPreviewEnabled`。
  - **降级（预览绝不能让主流失败）**：推流开始前用 `ffmpeg -encoders / -muxers / -filters` 检查 `mjpeg` 编码器、`image2` 封装、`fps` 与 `scale` 滤镜（按 ffmpeg 路径缓存）；不支持、预览目录不可用 / 不可写、`preview=false`，都**只是不加预览输出**（记日志，不报错、不改变 `Start*` 的返回）。运行中预览输出自己出错（写盘失败）会让 ffmpeg 整体退出，与主输出写失败同样处理（由主流的错误分类决定，预览不引入新错误码）。`StartPullPreview` 是拉流预览专用，没有"主流"可降级，ffmpeg 不支持时返回 `UNSUPPORTED`（`detail` 单独一行 `missing=preview`）。
  - **拉流预览会话**：`StartPullPreview` 后端起一个 ffmpeg **只读远端流、只输出预览**（不推流、不存盘），播放本身仍由前端播放器直接拉远端地址（契约 6.10 之前的"前端 mpegts.js 直接拉"不变）。地址规则：`rtmp` / `rtmps` / `srt` 复用推流地址校验（`LIVE_URL_INVALID` + `reason=`）；`http` / `https` 只做基本校验（主机必填、无空白 / 控制字符 / `|` `\` `"` `'`、≤ 2048 字节）；其他协议（含 `ws` / `wss`）`reason=scheme_unsupported`。输入侧 `-protocol_whitelist` 写在 `-i` 之前（rtmp `rtmp,tcp`、rtmps `rtmps,tcp,tls,crypto`、srt `srt,udp`、http(s) `http,https,tcp,tls,crypto`）。先用 ffprobe（`-rw_timeout 8s`，总 12 秒超时）探测有没有视频流：**纯音频没有预览**（不启动预览 ffmpeg，会话立即结束，`active` 变 `false`）；探测不出来（没有 ffprobe、连不上）按"有视频"让 ffmpeg 自己试。同一标准化地址重复调用返回同一会话（幂等）；同时最多 4 路（与推流会话上限分开计），超过 `TASK_CONFLICT`（`detail` 首行 `reason=max_pull_previews`）。会话不是任务（不进任务中心、不落库、不占 live 池）；`StopPullPreview`、远端流结束、ffmpeg 退出、应用退出（`Close`）都会结束会话并清理预览文件。地址（含口令 / 流名）只在调用参数里，日志和返回值只有脱敏形式。
  - **临时文件**：`<数据目录>/tmp/live-preview/<会话 id>.jpg`（及 ffmpeg 原子写入的 `.jpg.tmp`）。会话结束（含从未运行、排队中被取消）删除；**应用启动时清空并重建整个 `live-preview` 目录**（清理上次异常退出遗留；目录名必须是 `live-preview`，防止误删）；目录建不出来只是本次运行没有预览。
  - **前端约定**：约 500 毫秒轮询 `GetPreview`，`active=false`、任务进入终态、页面不可见时停止；`data` 转成 `data:image/jpeg;base64,<data>` 显示；`ts` 长时间不前进 = 画面停滞。浏览器模拟层（`frontend/src/api/live.ts`）最小假实现：`getPreview` 恒返回 `{data:'', ts:0, active:false}`，`startPullPreview` 返回不出画面的会话。**本版不改直播页 UI。**
  - **测试**：参数构造表驱动（文件推流有 / 无音轨、屏幕推流、带 tee 存档、拉流各协议、纯音频、`preview=false`）；`GetPreview` 半帧 / 无文件 / 关闭 / 未知会话；会话结束与启动清理；集成测试用真实 ffmpeg（7.1.5、9.0.2）+ MediaMTX 1.21.1（含 Xvfb 屏幕采集与带存档的屏幕推流）验证 2 秒内拿到宽 640 的合法 JPEG、画面不是黑屏 / 纯色（亮度方差）、主流与存档不受影响、停止后临时文件被清理。**【未验证】**Windows 真机上预览的耗时、高 CPU 占用；WebView 里 500 毫秒轮询大图 base64 的开销（每帧约几十 KB）。

- **采集来源（v0.14）**：
  - **`ListCaptureSources`**：屏幕来源 = `ListScreens` 的结果（`id` 换成 `screen:<序号>`，`title` = `ScreenInfo.Name`）；Windows 上再追加窗口来源。`ListScreens` 失败（`UNSUPPORTED_PLATFORM`，如 Wayland / 没有 `DISPLAY`）时整个方法同样失败。枚举窗口失败（`EnumWindows` 出错）只记日志、只返回屏幕，不报错。macOS / Linux 多显示器尽量列出（Linux 用 xrandr，没有 xrandr 只给一个 `x11:desktop` 默认；macOS 用 avfoundation 设备列表），**永远不返回 `window`**。
  - **窗口过滤（Windows，纯函数 `filterCaptureWindows`，有表驱动测试）**：`EnumWindows` 取顶层窗口，保留同时满足：标题非空（去空白后）；`IsWindowVisible`；不是最小化（`IsIconic`，最小化的不列出，因为 gdigrab 采不到内容；"最小化按需标记"本版选择不列出）；不是 DWM cloaked（别的虚拟桌面、挂起的 UWP 窗口）；客户区宽高都大于 0；不是本进程（FFmpegFree 自己）的窗口；不是系统壳窗口（类名 `Progman`、`WorkerW`、`Shell_TrayWnd`、`Shell_SecondaryTrayWnd`、`Windows.UI.Core.CoreWindow`，或标题 `Program Manager`）；没有 `WS_EX_TOOLWINDOW` / `WS_EX_NOACTIVATE` 且没有 owner（即被拥有的对话框、浮层不列）——带 `WS_EX_APPWINDOW` 的例外，照列。
  - **`id` 与校验**：`window:<hwnd 十进制>`，句柄在窗口关闭后会失效（也可能被复用，见未验证项）。`StartScreenPush` 传了 `captureSourceId` 时，**在占会话 / 建存档之前**重新枚举并按同一套过滤校验来源还在：格式不对（不是 `screen:<无符号十进制>` / `window:<无符号十进制>`、有前导零、带符号、十六进制）→ `INVALID_ARGUMENT`；非 Windows 传 `window:…` → `INVALID_ARGUMENT`；窗口不在过滤后的列表里（已关闭、已最小化、已不可见）→ `LIVE_SOURCE_GONE`（`kind=window`）；屏幕序号超出当前 `ListScreens` 的范围 → `LIVE_SOURCE_GONE`（`kind=screen`）。校验失败不创建任务、不占用会话。检查顺序：URL 校验 → 选项 → audio → 存档目录 → 能力检测 → 协议 / tee 检测 → **来源校验** → 会话冲突（`duplicate_url` → `screen_busy` → `max_sessions`）。
  - **ffmpeg 命令行（Windows gdigrab）**：窗口 → `-f gdigrab -framerate <fps> [-draw_mouse 0] -i title=<窗口标题>`（用**校验那一刻**的标题，标题作为**单个 argv 元素**传给 ffmpeg，不经过 shell，不加引号、不转义；gdigrab 把 `title=` 之后的全部内容当窗口标题，所以空格、引号、`=`、`&`、`|`、`%`、中日韩都原样；不带 `-offset_x` / `-offset_y` / `-video_size`，窗口大小由 gdigrab 决定，输出仍按 `PushOptions` 缩放并保证偶数）。屏幕 → `-f gdigrab -framerate <fps> [-draw_mouse 0] -offset_x <X> -offset_y <Y> -video_size <W>x<H> -i desktop`，`X/Y/W/H` 取自 `EnumDisplayMonitors` / `GetMonitorInfoW` 的显示器矩形（副屏在主屏左 / 上方时 `X` / `Y` 为负数，原样传）。macOS / Linux 屏幕来源命令行不变。
  - **窗口在校验后、ffmpeg 打开前消失**：gdigrab 报 `Can't find window '…', aborting.`——`ClassifyLiveError`（`Screen=true`）识别它 → `LIVE_SOURCE_GONE`，`detail` 只有 `kind=window`（stderr 里有窗口标题，**不放进 detail**）。这是任务失败（没有 `task:progress` 之前），不是同步错误。已开始推流之后窗口被关闭：gdigrab 行为未验证，按现有规则分类（多半是 `INTERNAL` 或 `LIVE_PUSH_INTERRUPTED`）。
  - **任务 `params`**：`kind=screen` 的 `params` 在传了 `captureSourceId` 时多一个 `"captureSourceId"` 字段（没传则不出现，旧任务不变）；任务标题 = `屏幕推流：<屏幕名 | 窗口标题> → <脱敏地址>`。`screenId` 字段仍是请求里的原值。
  - **【未验证】**（Linux 箱子只能验证参数构造、过滤、错误码；见 6.10.1 第 8～10 项）：Windows 真机上 `EnumWindows` 的实际过滤效果；gdigrab `title=` 采窗口被其他窗口遮挡 / 最小化 / 跨显示器 / 高 DPI（进程非 DPI 感知时 `GetClientRect` 与 gdigrab 的尺寸口径）；多显示器 `offset` / `video_size` 在非 100% 缩放、副屏在负坐标时是否对齐；同标题的多个窗口（`title=` 只能命中 `FindWindow` 找到的第一个，可能不是用户选的那个——**这是 `title=` 方案的固有局限**；ffmpeg 7.0+ 的 `hwnd=<十进制>` 可以精确指定窗口，但本机 ffmpeg 版本不一定支持，本版按契约用 `title=`，需要时另出契约变更）。

### 6.10.1 真机试用清单（Live，未验证项汇总）

以下项目**没有在真机上验证**（箱子是 Linux + ffmpeg 7.1.5 + MediaMTX 1.21.1），契约里已就地标"未验证"。**不阻塞实现**：实现按契约写，试用包出来后由用户逐项确认，不符再回来改契约。（Edit 的 Windows 路径 / `commitPart` / Range 续传、Doc 的 Windows 字体路径 / 大文件 Range 各在 6.11.1 / 6.11.4 / 6.12.1 / 6.12.4 有同样的清单，合并后由架构师汇总。）

| # | 未验证项 | 在哪里 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|---|
| 1 | Windows 上 `file:C\:/Users/…` 形式的存档路径能否被 ffmpeg tee 正确解析（Linux 上盘符冒号只是普通字符） | 6.10「`tee` 段转义规则」第 5 点 | Windows 上屏幕推流 + 存档到 `C:\Users\<含空格和中文的目录>`，确认文件出现在期望位置 | 改转义规则（契约变更），不改行为约定 |
| 2 | UNC 路径 `//server/share/…` 作存档目录 | 同上 | 存档到网络共享 | 同上；最坏情况在 `Start*` 拒绝 UNC 并返回 `INVALID_ARGUMENT` |
| 3 | `MoveFileEx` / 改名语义：Windows 上同名冲突时 `(n)` 后缀的占位文件（`O_CREATE\|O_EXCL`）能否被 ffmpeg 以 `-y` 写入；被杀后文件句柄释放时机是否影响随后的 `ffprobe` 与删除空壳 | 6.10 存档第 2 条 | 同一秒内连开两路存档；强杀后立即检查文件是否可读、空壳是否被删 | 空壳删除改为延迟重试；不影响主流程 |
| 4 | `rtmps`：本机 ffmpeg 的 TLS 握手与证书校验在真实服务器上的表现（7.1.5 只实测了连接被拒：退出码 145、`LIVE_CONNECT_FAILED`，没有真实 rtmps 服务器） | 6.10 命令行、错误分类 | 推到一个真实的 rtmps 地址（如常见直播平台的 rtmps 入口） | 证书错误的措辞补进分类规则；缺协议已有 `UNSUPPORTED`（`missing=rtmps`） |
| 5 | SRT 在真实公网 / 有 passphrase 的服务器上：错误 passphrase 与服务器未开确实都只有 `Input/output error`（实测于 MediaMTX，其他服务器未测） | 6.10「SRT 说明」 | 用错误口令推到 SRS / MediaMTX / 商用服务 | 仍判 `LIVE_CONNECT_FAILED`，前端文案不变 |
| 6 | macOS 屏幕录制授权检测；Windows gdigrab 多显示器 / 非 100% 缩放的偏移与尺寸；`x11grab` 在各桌面环境的表现 | 6.10「未验证」条 | 各平台真机各推一次 | 见各条 |
| 7 | 15 秒（有存档）/ 16 秒（`Shutdown`）优雅停止上限在慢网络、高负载下够不够 | 6.10 存档第 3 条 | 弱网下停止有存档的会话，看是否超时被强杀 | 调整上限（契约变更） |
| 8 | Windows：`EnumWindows` 过滤后的窗口列表是否合理（无任务栏 / 桌面 / 输入法 / 系统浮层，UWP 应用与最大化窗口能列出）；FFmpegFree 自己的窗口不出现 | 6.10「采集来源」 | Windows 上调 `ListCaptureSources`，与任务栏里的窗口对照 | 调整 `filterCaptureWindows` 的类名 / 样式过滤（不改契约结构） |
| 9 | Windows：gdigrab `title=` 采窗口——被其他窗口遮挡时内容是否正常、最小化后行为（黑屏 / 报错 / 冻结）、窗口移到副屏 / 高 DPI（125%~200% 缩放）时画面尺寸和清晰度；同标题多窗口命中哪一个；标题含引号、`&`、中文时能否找到窗口 | 同上 | 各推一次，观察播放端画面 | 遮挡 / 高 DPI 问题另出契约变更（如改用 `hwnd=`、DPI 感知清单）；同标题问题在前端提示或后端过滤 |
| 10 | Windows 多显示器：副屏在主屏左侧 / 上方（负偏移）、两块屏缩放不同时，`offset_x` / `offset_y` / `video_size` 是否对准该显示器（进程是否 DPI 感知影响 `GetMonitorInfoW` 的坐标口径） | 同上、6.10「采集能力检测」 | 双屏各选一块推流 | 改用物理像素坐标（进程声明 DPI 感知）或按缩放换算 |
| 11 | （v0.17）Windows 上预览输出的耗时（首帧时间、`GetPreview` 单次读取耗时）；高分辨率（4K）采集 / 硬字幕 / `-c copy` 场景下预览额外占用的 CPU；WebView 里每 500ms 轮询 `GetPreview`（base64 JPEG 经 IPC）的开销 | 6.10「预览画面」 | Windows 真机推流 1080p / 4K 屏幕，观察任务管理器 CPU、`GetPreview` 耗时、前端轮询时界面是否卡顿 | 降低 `fps` / 宽度常量、前端降低轮询频率，或在开销过大时默认 `preview=false`（需另出契约变更） |

### 6.10.2 实现清单（给 #31 / #30 对照；不是新接口；"现状"列已按 `origin/v2` 的 `2f0c0a4`（含已合并的 #31 第一部分）更新）

| # | 项 | 现状（`origin/v2`） | 要求 | 随哪个 PR |
|---|---|---|---|---|
| 1 | 优雅停止检查退出码 | **#31 已实现**：`ffmpeg.RunOptions.StrictGracefulExit`（`internal/ffmpeg/exec.go`），直播 Runner 开启（`internal/service/live/service.go`） | 直播路径必须检查退出码：`q` 后退出码 0 → `succeeded`；已请求 `Cancel` 后非 0 → `canceled`，无错误码；停止用 `q`（stdin 管道），不用 SIGINT（退出码 255） | #31 补一个提交 |
| 2 | 空壳存档清空 `outputPath` | **#31 已实现**：`task.ClearOutputPath` 特殊输出路径，`entry.finish` 遇到它把 `outputPath` 清空（`internal/task/`） | Runner 支持把 `outputPath` 清空（专门的清空标记 / 返回字段），先清空再发终态事件 | #31 补 |
| 3 | `Shutdown` 等待时间 | **#31 已实现**：`app.go` 的 `shutdown` 有带存档的直播会话时等 16 秒，否则 8 秒 | 有存档的直播会话运行时总等待 16 秒（6.6 / 6.10） | #31 |
| 4 | `fps` / `bitrateKbps` / `droppedFrames` | **#31 已实现**：`task.Progress` / `task.ProgressEvent` / `Task` 都有三个 `omitempty` 字段 | 三个字段（`omitempty`）；`bitrateKbps` 只在无存档会话计算 | #31 |
| 5 | 任务类型 | **#31 已实现** `live_screen_push` 和 `validType`（含 `live_file_push`）；`store/tasks.go` 已有 `TypeLiveScreenPush`；`TypeEditExport` 与 `validType` 里的 `edit_render` → `edit_export` 仍未改（随 #30） | `validType` 改为第 3 节 `TaskType` 枚举（`convert`、`edit_export`、`office_pdf`、`live_file_push`、`live_screen_push`、`ffmpeg_install`）；`Submit` **不再接受** `live_relay`、`live_record_push`（也不接受 `edit_render`）；`IsLive` 包含 `live_file_push`、`live_screen_push` | `live_screen_push` 随 #31，`edit_export` 随 #30 |
| 6 | 旧类型过滤 | **#31 已实现**（live 两个类型）：store 的 SQL 读取层过滤，`legacyTypes` / `legacyTypesSQL` / `LegacyTaskIDs`（`internal/store/tasks.go`）；`edit_render` 尚未加入（Edit 线合入前它仍是有效类型） | store 层读取时过滤旧类型行（`List` / `ListActive` 不含；`Get` / `Cancel` / `Remove` / `Retry` 一律 `NOT_FOUND`，见 6.10 确认项 ⑧） | #31（live 两个类型）、#30（`edit_render`） |
| 7 | `Task.error` 序列化 | `store.Task.Error` 是 `json:"error,omitempty"` | 与契约一致：无错误时省略该键，不输出 `null`（本契约示例已统一） | 无需改代码，测试断言 |
| 8 | 前端 `AppErrorCode` | `call.ts` 缺若干码 | 按 2.1 清单补全并逐项核对 | 前端（`v2-fe-api-contracts`） |

**ffmpeg 版本复测**：项目默认安装的是 **9.0.2**（`internal/ffmpeg/manifest.json`，martin-riedl 静态构建，SHA-256 与 manifest 一致），契约里的 ffmpeg 行为除注明外已在 **7.1.5 和 9.0.2 上都复测**：`q` 退出码 0；tee + `onfail=abort` 连接被拒退出码 145；tee 转义路径正常；`-protocol_whitelist` 按位置生效（输出侧只写 `rtmp,tcp` 时 `file:` 输入失败退出码 234）；`anullsrc` + `-shortest` 播完即结束；`-flvflags no_duration_filesize` 退出码 0；SRT passphrase 长度 9 / 81 位报 `SRTO_PASSPHRASE` 错、10 / 79 / 80 位通过。**与 7.1.5 不同的两点**：SRT 大写参数名在 9.0.2 报错而不是静默忽略；tee 默认 `onfail=continue` 连接被拒在 9.0.2 上退出码 145（7.1.5 是 0）。

### 6.10.3 直播预览视频流（v0.25，架构师定；取代 6.10「预览画面（v0.17）」）

**目标**：预览看到的就是推出去 / 拉进来的那路流，帧率、分辨率和音频都跟源一样，端到端延迟 < 1.5 秒；预览出任何问题都不能拖慢或中断推流。

#### 6.10.3.1 接口

```go
GetPreviewStream(sessionID string) (PreviewStream, error) // 推流任务 id 或拉流预览会话 id
StartPullPreview(req PullPreviewRequest) (PullSession, error) // 签名不变，PullSession 多 previewUrl
// 删除：GetPreview(sessionID) (Preview, error)、type Preview

type PreviewStream struct {
    URL      string `json:"url"`      // http://127.0.0.1:<端口>/live/<token>.flv
    MIME     string `json:"mime"`     // 恒为 "video/x-flv"
    HasVideo bool   `json:"hasVideo"` // 推流恒为 true；拉流按探测结果（纯音频流为 false）；v0.25.3：收到 FLV 头以后按头里的标志
    HasAudio bool   `json:"hasAudio"` // 推流：文件推流恒为 true（没有音轨时补的是静音），屏幕推流 audio=silent 时为 true；拉流：有 AAC / MP3 音频才为 true
}

type PullSession struct {
    ID         string `json:"id"`
    Redacted   string `json:"redacted"`
    Preview    bool   `json:"preview"`    // v0.25：有没有预览视频流（转换组件缺 tee / tcp 时 false），与请求里的 preview 无关
    PreviewURL string `json:"previewUrl"` // v0.25：同 GetPreviewStream(id).url；Preview=false 时为 ""
    HasVideo   *bool  `json:"hasVideo,omitempty"` // v0.25.3：同 PreviewStream；只有会话已经 playing 时才有（按 FLV 头），新会话省略，以 live:pull 的 playing 为准
    HasAudio   *bool  `json:"hasAudio,omitempty"` // v0.25.3：同上
}
```

- `GetPreviewStream` 的同步错误：会话不存在或已结束 → `NOT_FOUND`（`detail` 单独一行 `reason=session`）；这个会话没有预览视频流（转换组件缺 `tee` 或 `tcp`，或者预览分支 15 秒内没连上 / 已经断开）→ `UNSUPPORTED`（`reason=preview_unavailable`）；编码不能在应用内播放 → `UNSUPPORTED`（`reason=codec`，message 见 6.10.3.6）；HTTP 服务起不来（端口绑不上）→ `INTERNAL`。**推流本身永远不受这些错误影响**。
- 会话刚开始、ffmpeg 还没输出 FLV 头时就可以调用：URL 先给出去，HTTP 请求最多等 10 秒拿到 FLV 头，等不到返回 `503`（前端按“加载中 → 失败”处理，可以重试）。
- `StartPullPreview` 的探测在后台进行（同 v0.17），所以 `previewUrl` 立即返回；探测发现编码不支持时，会话以 `live:pull` 的 `unsupported` 结束（6.10.3.7），之后的 `GetPreviewStream` 返回 `NOT_FOUND`。
- 前端浏览器模拟层：`getPreviewStream` 返回 `UNSUPPORTED`（`reason=preview_unavailable`），`startPullPreview` 的 `previewUrl` 为 ""。

#### 6.10.3.2 推流：同一个 ffmpeg 进程里多一个 tee 分支

- **每个推流会话**（只要转换组件有 `tee` 封装和 `tcp` 协议）的主输出**一律用 tee**，带上预览分支，**与请求里的 `preview` 无关**（没有存档的会话原来是普通输出，现在也用 tee）。不多编码一次，各分支拿到的是同一份编码后的包：
  ```
  -flags +global_header -f tee "<网络分支>[|<存档分支>]|<预览分支>"
  网络分支  [f=<flv|mpegts>:onfail=abort:protocol_whitelist=<同 6.10>(:flvflags=no_duration_filesize，只有 flv)]<推流地址，TeeEscape>
  存档分支  同 6.10（不变）
  预览分支  [f=flv:onfail=ignore:use_fifo=1:fifo_options=queue_size=120\\:drop_pkts_on_overflow=1:flvflags=no_duration_filesize:flush_packets=1:protocol_whitelist=tcp]tcp\://127.0.0.1\:<端口>\?tcp_nodelay\=1
  ```
  （这是传给 ffmpeg 的参数原文。v0.25 实现 PR 更正：tee 先按 `|` 切分整个地址（去掉一层 `\` 转义），再按 `:` 解析方括号里的选项（又去一层），所以 `fifo_options` 里的 `:` 必须写成两个反斜杠 `\\:`；只写一个时 `drop_pkts_on_overflow` 会被当成 FLV 的选项，预览分支打开失败（`onfail=ignore`，推流照常但没有预览）。用 `use_fifo=1` 时，方括号里 tee 不认识的选项（`flvflags`、`flush_packets`、`protocol_whitelist`）由 tee 转交给 fifo 里的 FLV 封装。）
  SRT 推流的网络分支仍是 mpegts，预览分支一律是 FLV。只有转换组件缺 `tee` 或 `tcp` 时，命令行才和 v0.24 一样（没有存档就不用 tee），这个会话没有预览。
- **为什么用本机 TCP，不用 `pipe:1`**：标准输出已经给了 `-progress pipe:1`；Windows 上 `os/exec` 不能把额外的文件描述符传给子进程。
- **后端接收**：每个会话开一个只监听 `127.0.0.1:0` 的一次性 TCP 监听器，**只接受第一个连接**，并且只在 ffmpeg 启动后 15 秒内接受，接受后立即关闭监听器。读协程一直读，只做 FLV tag 切分并放进会话的分发器，**从不阻塞在客户端上**（6.10.3.4）。15 秒内没有连上，或者连接断了，预览算失败，推流照常。同一用户的其他本机进程如果抢先连上，只能往预览里塞数据，拿不到推流内容，按同用户威胁处理，不另设防。
- **不会拖慢推流**：tee 的 `onfail=ignore` 管出错（预览连接断了、写失败），`use_fifo=1` + `drop_pkts_on_overflow=1` 管变慢（队列满了丢预览分支的包，不反压编码器）；后端读 socket 永远不等 HTTP 客户端。实测（ffmpeg 7.1，30 fps，v0.25 实现 PR 复测）：预览分支的 TCP 对端完全不读（接收缓冲 4 KB）时，推流期间主输出保持 30 fps、不变慢（fifo 报 `FIFO queue full` 并丢预览的包）；但源**推完以后** ffmpeg 会等预览分支把队列写完才退出，所以后端必须一直读到 EOF 或主动关掉这个连接（关掉后 tee 报 `Slave muxer #N failed` 并照常退出）。后端的读协程从不等 HTTP 客户端，满足这一点。
- **码率统计**：tee 下 `-progress` 的 `total_size` 是 `N/A`（6.10 存档已有此情况）。带预览分支的会话改由后端按预览分支收到的字节数算 `bitrateKbps`（不管前端有没有在看）（同一份包，只差 FLV 封装开销，误差 < 2%）；预览分支断了以后不再报 `bitrateKbps`（同存档会话）。
- 能力检测：开始前检查 `tee` 封装和 `tcp` 协议（复用 `checkProtocols` 的缓存）；缺了**只是不加预览分支**（记日志），推流照常，之后 `GetPreviewStream` 返回 `UNSUPPORTED`（`reason=preview_unavailable`）。

#### 6.10.3.2a 预览开关（开始前、会话进行中都能切换）

- 新直播页在“开始推流”旁边和会话面板里每个会话上都有“开启预览”。**开关只在前端起作用**：打开 = 调 `GetPreviewStream` 拿到地址，用 mpegts.js 连上；关闭 = 销毁播放器、断开 HTTP 连接。后端看到的只是 HTTP 客户端连上或断开，分发器继续读预览分支（6.10.3.4），**推流的 ffmpeg 不重启、不中断、命令行不变，`task:status` / `task:progress` 不受影响**。一个会话可以反复开关，次数不限。重新打开时，后加入的客户端从缓存的最近关键帧开始播（6.10.3.4）。
- **开始时的 `preview` 字段**：`FilePushRequest.preview` / `ScreenPushRequest.preview` / `PullPreviewRequest.preview` 保留在结构体里，旧前端照传不报错，**后端忽略**，不影响命令行，也不影响 `GetPreviewStream`。“开始前”的开关只是前端在会话开始后要不要立即连接，由前端自己记住；后端不保存它，任务的 `params` 里也不写。
- 代价：没人看的时候预览分支也在跑。它只是把已经编码好的包经本机 TCP 交给后端，后端只做 FLV 切分，没有客户端时直接丢弃（只保留序列头和最近一个 GOP 的缓存），不另外编码，开销可以忽略。

#### 6.10.3.3 拉流预览：转封装，不转码

- `StartPullPreview`（**总是**起 ffmpeg，请求里的 `preview` 被忽略，见 6.10.3.2a）：先用 ffprobe 探测（同 v0.17：`-rw_timeout 8s`，总共 12 秒），按 6.10.3.6 决定取哪些流，再起 ffmpeg：
  ```
  -protocol_whitelist <输入白名单，同 v0.17> -fflags +nobuffer -flags low_delay -analyzeduration <N> -probesize <N> -rw_timeout 8000000 [-live_start_index -1] -i <地址>
  -map 0:v:0 [-map 0:a:0 | -an] -c copy -f flv -flvflags no_duration_filesize -flush_packets 1 -protocol_whitelist tcp tcp://127.0.0.1:<端口>?tcp_nodelay=1
  ```
  **v0.25.1**：`<N>` 是 rtmp / rtmps `5000000`，其他 `1000000`（探测和转封装相同；RTMP 要盖住一个完整 GOP，MediaMTX 的 RTMP 在第一个关键帧前不给 SPS/PPS，1 秒时 12 次失败 4 次；SRT 不加，见 v0.25.1 ⑤）；探测用 `-of json` 同时取 `format_name`；转封装加 `-rw_timeout 8000000`（单次读写最多等 8 秒）；**HLS**（地址路径以 `.m3u8` 结尾，不分大小写、忽略查询串，或探测到的 `format_name` 含 `hls`）加 `-live_start_index -1`（从最新分片开始），tag 由分发器匀速放出（6.10.3.4）。ffmpeg 启动后 15 秒还没有 FLV 头就停掉，按 `failed` 结束（6.10.3.7）。
  探测不出来（没有 ffprobe、超时）时用 `-map 0:v:0? -map 0:a:0?` 让 ffmpeg 自己试；flv 封装拒绝某个编码时（stderr 里有 `codec not currently supported in container` 之类），会话按 `unsupported` 结束。
- 接收、分发、HTTP 与推流相同（6.10.3.4、6.10.3.5）。幂等、最多 4 路、会话不是任务，这些规则不变（v0.17）。`ws` / `wss` 仍然 `LIVE_URL_INVALID`，前端照旧用 mpegts.js 直接拉。
- 拉流页的播放改用 `previewUrl`，这样 rtmp / rtmps / srt 地址也能在应用里播。拉流页的“开启预览”开关同样只决定前端连不连（6.10.3.2a）；关掉时后端的转封装会话继续运行（仍在接收远端流），直到 `StopPullPreview`。

#### 6.10.3.4 分发器（每个会话一个）

- 读协程把 FLV 切成 tag，缓存这些：FLV 头（13 字节）、最近一个 `onMetaData` 脚本 tag、AVC 序列头（`AVCPacketType=0`）、AAC 序列头（`AACPacketType=0`），以及**从最近一个视频关键帧开始的 GOP**（上限 10 秒或 8 MiB，超了就清空，等下一个关键帧）。
- **后加入的客户端**：先发 FLV 头 → metadata → 序列头 → 缓存的 GOP，然后接着发实时数据。**v0.25.2**：GOP 缓存只从视频关键帧开始；缓存里没有 GOP（第一个关键帧之前，或缓存刚被清空）时，新客户端等到下一个关键帧才开始收音视频，从不收到半个 GOP（纯音频的流不等）。时间戳不改写（mpegts.js 的直播模式能处理不从 0 开始的时间戳）。
- **慢客户端**：每个客户端一个有界队列（约 2 秒的数据，最多 4 MiB）。队列满了就**丢这个客户端的音视频 tag，一直丢到下一个视频关键帧**，再从关键帧接着发（不会发出解不了的半个 GOP）；连续 5 秒都处于丢包状态就断开它（前端重连会拿到新的 GOP）。慢客户端只影响它自己。
- 每个会话最多 4 个同时连接的 HTTP 客户端。~~超过返回 `429`~~ **v0.25.2：满员时新连接挤掉最早加入的那个**（中止它的连接，不发结束标记），新连接照常 `200`；理由见 v0.25.2 ②。每个连接本机一侧的发送缓冲 256 KiB，每次写（含 Flush）最多等 2 秒，写不进去就断开、释放名额；被挤掉、太慢的连接立即中止，不再先把队列发完。客户端断开（写失败或请求结束）时立即离开分发器、释放名额（v0.25.1）。后加入的客户端在同一把锁里取开头字节并加入队列，tag 不重复也不遗漏（v0.25.1）。
- **HLS 匀速（v0.25.1，只用于 HLS 拉流）**：HLS 一次到一整个分片，不处理时 FLV 每隔一个分片时长一次性到一整段数据，播放器缓冲忽大忽小、追帧跳到 GOP 中间花屏。读协程切出的 tag 先进一个排期队列：metadata、序列头立即放出；第一个音视频 tag 到达时定锚，之后按“锚点 + (时间戳 − 锚点时间戳)”放出；某个 tag 到达时已经晚于它的放出时间超过 40 毫秒（数据断档），锚点后移这次的晚到量（延迟随之增加）；每 10 秒看一次这段时间里最小的余量，还有富余（> 100 毫秒）就把锚点提前一点（每次最多 250 毫秒，留 50 毫秒余量），延迟不会只增不减；时间戳倒退超过 1 秒、某个 tag 要扣 8 秒以上、积压超过 30 秒或 64 MiB 时重新定锚、立即放出。ffmpeg 断开时队列里剩下的立即放完。推流和非 HLS 拉流不经过这个队列。

#### 6.10.3.5 本机 HTTP 服务

- 第一次需要时启动，应用退出时关闭；**只监听 `127.0.0.1`**（不监听 `0.0.0.0`，也不监听 `::1`），端口随机（`127.0.0.1:0`）。只有 `/live/<token>.flv` 一个路径。
- **token**：每个会话一个，32 字节 `crypto/rand`，base64url 编码（43 个字符）；会话结束时作废。正在传输的响应正常收尾，之后的新请求返回 `404`。token 不写日志，日志里只写 `/live/<已脱敏>`。不需要 Cookie，也不需要别的鉴权头。
- **方法**：`GET`、`OPTIONS`。其他方法返回 `405`。`Range` 请求头忽略，一律 `200`，从当前的直播位置开始发。
- **响应头**：`Content-Type: video/x-flv`、`Cache-Control: no-store`、`X-Content-Type-Options: nosniff`、`Access-Control-Allow-Origin: <回显的 origin>`、`Vary: Origin`。不带 `Content-Length`，用分块传输；每批 tag 写完立即 `Flush`。
- **CORS（只回显 Wails 自己的 origin，绝不用 `*`）**：允许的 origin 按 go.mod 里的 Wails 版本（**v2.11.0**）的实际取值：
  - Windows（WebView2）：`http://wails.localhost`（`internal/frontend/desktop/windows/frontend.go` 的 `startURL`；设置了 `assetserverport` 时带端口，按实际 `startURL` 算）。
  - macOS（WKWebView）、Linux（WebKitGTK）：`wails://wails`（`desktop/darwin|linux/frontend.go` 的 `startURL`）。
  - 只在开发构建（`wails dev`，`dev` build tag）里额外允许：`http://localhost:34115`（Wails 开发服务器）和 wails.json 里 `frontend:dev:serverUrl` 的 origin（Vite 开发服务器）。发布构建不放行这些。
  - `Origin` 不在允许列表里，或者**请求不带 `Origin`**，一律返回 `403`。mpegts.js 用 fetch 跨源取流，一定会带 `Origin`。
  - `OPTIONS` 预检：origin 允许时返回 `204`，带 `Access-Control-Allow-Origin`、`Access-Control-Allow-Methods: GET, OPTIONS`、`Access-Control-Allow-Headers: Range`、`Access-Control-Max-Age: 600`、`Vary: Origin`；请求带 `Access-Control-Request-Private-Network: true` 时再加 `Access-Control-Allow-Private-Network: true`（Chromium 的 Private Network Access）。不允许的 origin 返回 `403`。
- **FLV 头之前连上来的请求（v0.25.1）**：挂着等第一个 FLV 头，最多 30 秒，头到了就 `200` 开始发；会话结束、拉流失败、预览分支在等待时间内没连上（分发器关闭）时立即 `503`，不挂满 30 秒；30 秒还没有头也是 `503`。
- 不写访问日志；`http.Server` 设置 `ReadHeaderTimeout`（5 秒），不设 `WriteTimeout`（直播流是长连接）。

#### 6.10.3.6 编码兼容（v0.25 不转码）

| 流里的编码 | 处理 |
|---|---|
| 视频 H.264 | 可以播放 |
| 视频 HEVC / H.265 | **默认不支持**（WebView2 只有部分机器有 HEVC 解码，MSE 也不一定支持）→ `UNSUPPORTED` `reason=codec` |
| 视频是其他编码（AV1、VP9、MPEG-2 等） | `UNSUPPORTED` `reason=codec` |
| 音频 AAC、MP3 | 一起播放 |
| 音频是其他编码（Opus、PCM、AC-3 等） | **去掉音频，只给画面**（`hasAudio=false`，不算错误） |
| 只有音频，而且是 AAC / MP3 | 可以播放（`hasVideo=false`） |
| 只有音频，而且是其他编码 | `UNSUPPORTED` `reason=codec` |

- 推流总是重编码成 H.264（libx264 或硬件 H.264）+ AAC，或者没有音频，所以 v0.25 里推流预览**不会**出现 `reason=codec`。这条规则保留给以后的 `-c copy` 推流。
- message（给用户看，不出现技术词，1.1）：推流 `这路视频无法在应用内预览，推流不受影响。`；拉流 `这路视频无法在应用内播放。`。`detail` 第一行是 `reason=codec`，第二行是 `video=<编码名>` 或 `audio=<编码名>`（给开发者看）。
- 只有音频不支持时，选择只给画面而不是报错，理由是：画面才是预览的主要内容，丢掉音频的代价最小；前端按 `hasAudio=false` 把静音按钮置灰。

#### 6.10.3.7 结束与中断

- 会话结束时（推流任务进入终态、`StopPullPreview`、远端流结束、ffmpeg 退出、应用退出），分发器关闭，每个 HTTP 响应把已经排队的数据发完再返回，分块传输正常收尾（发出结束标记，不是直接断开连接）。前端的 mpegts.js 收到 `LOADING_COMPLETE`，再按下面的状态显示文字。
- **推流**沿用 `task:status`：`succeeded` / `canceled` → `推流已结束`；`failed`（不论错误码）→ `推流被中断，请回到直播页重新推流。`。**v0.25.3**：开始以后被中断的推流是 `interrupted`（带 `LIVE_PUSH_INTERRUPTED` 等错误，见 6.10 错误分类），直播页同样显示 `推流被中断，请回到直播页重新推流。`；`failed` 只剩开始之前的失败。
- **拉流预览**新增事件 `live:pull`（拉流预览会话不是任务，没有 `task:status`）：
  ```
  live:pull  { id, state, error?, hasVideo?, hasAudio? }
  state = "playing"      收到第一个 FLV 头（只发一次）；v0.25.3 起带 hasVideo / hasAudio（按 FLV 头里的音视频标志，纯音频 hasVideo=false）
        | "ended"        StopPullPreview，或者远端流正常结束（ffmpeg 退出码 0）
        | "interrupted"  已经 playing 之后 ffmpeg 非零退出（网络断开、远端异常）；v0.25.3 起带 error：LIVE_PUSH_INTERRUPTED（推流、拉流共用），message “拉流被中断，请重新拉流。”；v0.25.4 起 detail 第一行是 reason=pull（推流中断是 reason=push），前端按 reason 区分，不靠文案
        | "failed"       playing 之前就失败（连不上、15 秒内没有数据等），error 是分类后的 AppError（LIVE_CONNECT_FAILED / INTERNAL，v0.25.1 起 message 都是“拉流失败，请检查直播地址和网络。”）
        | "unsupported"  编码不能播放，error 为 UNSUPPORTED reason=codec
  ```
  前端文字：`ended` → `拉流已结束`；`interrupted` → `拉流被中断，请重新拉流。`（产品经理已定）；`failed` / `unsupported` 用 error 的 message。应用退出时不发。
- **拉流失败的分类（v0.25.1，设计走查 G3）**：和推流分开（`ffmpeg.ClassifyPullError`）。`playing` 之前 ffmpeg 非零退出：stderr 尾部是连接类（无法解析主机名、拒绝连接、超时、网络不可达、远端没有这路流 / 404、读超时）→ `LIVE_CONNECT_FAILED`，`detail` 第一行 `scheme=<rtmp|rtmps|srt|http|https>`，第二行起是脱敏后的 stderr 尾部；其他 → `INTERNAL`，`detail` 是 stderr 尾部。ffmpeg 启动后 15 秒还没有 FLV 头 → 停掉 ffmpeg，`LIVE_CONNECT_FAILED`，`detail` 第二行 `15 秒内没有收到数据`。**message 一律是 `拉流失败，请检查直播地址和网络。`**（后端常量 `ffmpeg.PullFailedMessage`）。推流的 `连接推流服务器失败` / `推流启动失败` 不出现在拉流里。拉流的 `preview_unavailable` message 是 `这路视频暂时无法在应用内播放。`，`codec` 是 `这路视频无法在应用内播放。`。
- **状态只往终态走（v0.25.1，架构师定，前端实现）**：事件可能乱序；到了 `ended` 或 `interrupted` 就不再改变，先到的那个决定状态和文字：`ended` → `拉流已结束`，用户没按停止时再加一行 `直播已停止，或连接已断开。`；`interrupted` → `拉流被中断，请重新拉流。`；两种都提供「重新拉流」。
- 事件和日志里的地址都用脱敏形式（同 6.10）；token 不出现在任何事件里。

#### 6.10.3.8 延迟（目标：端到端 < 1.5 秒）

- ffmpeg：预览分支 `flush_packets=1`，TCP 用 `tcp_nodelay=1`；拉流输入用 `-fflags +nobuffer -flags low_delay`，探测 1 秒 / 1 MB（v0.25.1：rtmp / rtmps 改为 5 秒 / 5 MB，实测探测多花约 0.7 秒）。**HLS 拉流**比 RTMP 直连多约一个分片的到达间隔（MediaMTX 实测中位数多 1.8 秒），这是匀速放出的代价（6.10.3.4），1.5 秒的目标不适用于 HLS。推流 libx264 是 `-tune zerolatency`（没有 B 帧、没有 lookahead），硬件编码沿用 9.7 的低延迟参数。
- 后端：读到一个 tag 就分发，HTTP 每批立即 `Flush`，不攒数据。
- 前端 mpegts.js：`{ type: 'flv', isLive: true, hasAudio, hasVideo }`，配置 `enableStashBuffer: false`、`liveBufferLatencyChasing: true`、`liveBufferLatencyMaxLatency: 1.5`、`liveBufferLatencyMinRemain: 0.3`（mpegts.js ≥ 1.7.3 可以改用 `liveSync: true`、`liveSyncMaxLatency: 1.2`、`liveSyncTargetLatency: 0.6`），`<video muted>` 默认静音。
- **GOP**：推流的 `-g` 是 2×帧率（2 秒）。后加入的客户端从缓存的最近关键帧开始，开头最多落后 2 秒，靠追帧在几秒内追到目标延迟。拉流的 GOP 由远端决定，GOP 很长（如 10 秒）的流开头会慢一些，这是转封装方案的固有限制。

#### 6.10.3.9 测试（实现 PR）

- 单元测试：tee 描述（无存档 / 有存档 / SRT，三种）、预览分支转义；HTTP 的 token、CORS 白名单、`OPTIONS`、没有 Origin 时 403、作废 token 404、第 5 个客户端挤掉最早的（v0.25.2，原为 429）、405；FLV 切分、序列头和 GOP 缓存、后加入客户端收到的开头字节；慢客户端丢到关键帧、断开；会话结束后响应正常收尾；日志里没有地址和 token。
- 集成测试（真实 ffmpeg + MediaMTX，会报数字）：①推流主输出的帧率（从 MediaMTX 拉流用 ffprobe 数 N 秒内的帧）；②预览流的帧率（HTTP 客户端读 N 秒，用 ffprobe 数）；③拉流预览的帧率；④端到端延迟（testsrc2 叠加 `drawtext` 写墙钟时间，或比较帧的 pts 和到达 HTTP 客户端的墙钟时间）；⑤预览 HTTP 客户端连上后不读，推流帧率不下降；⑥预览分支的 TCP 对端不读，推流帧率不下降；⑦停止会话时 HTTP 响应正常结束；⑧（v0.25.2）模拟 10 次切页面：每次丢下旧连接（不读、不关）再开新连接，都要 `200`、从关键帧开始、3 秒内连续出帧，推流帧率不降，名额不泄漏。
- 【未验证，Windows 真机】WebView2 里 `http://wails.localhost` → `http://127.0.0.1:<端口>` 的 fetch 与 Private Network Access 的实际表现；macOS WKWebView 和 Linux WebKitGTK 从 `wails://wails` 发出请求时 `Origin` 头的实际取值（WebKitGTK 可能是 `null`，如果是就要单独放行并在契约里写明）；Windows 防火墙对只监听 127.0.0.1 的端口是否弹窗（预期不弹）；硬件编码下的延迟。

#### 6.10.3.10 开放问题（请架构师定）

1. **HEVC**：v0.25 一律算不支持。以后可以改为前端用 `MediaSource.isTypeSupported('video/mp4; codecs="hvc1.1.6.L93.B0"')` 检测后再放行（mpegts.js 支持 Enhanced FLV 的 HEVC），这需要后端知道前端的检测结果。
2. **不支持的编码要不要转码**：v0.25 不转码（架构师定）。以后可以考虑低码率 H.264 转码预览（这样会多编码一次，要评估 CPU）。
3. **只有音频不支持时只给画面**：本版这样定（6.10.3.6），也可以改成整体报错。
4. **为什么不用 Wails AssetServer 提供流**（同源，不需要端口和 CORS）：Windows WebView2 的 AssetServer 能不能持续推送无限长的响应、能不能及时 flush，没有验证过；本机 HTTP 服务的行为是确定的。如果真机验证 AssetServer 可以流式输出，可以以后再换。
5. **预览分支始终存在**（6.10.3.2a）：这是为了让会话中途开关预览不重启推流。另一种做法是开关时重启 ffmpeg 加上或去掉分支，会让推流断一下，所以没有采用。
6. ~~拉流被中断的文字~~ 已定（产品经理）：`拉流被中断，请重新拉流。`；推流被中断仍是 `推流被中断，请回到直播页重新推流。`。

## 6.11 EditService 契约（v0.11）——**v0.23.5 已移除，本节只作历史记录**

> **v0.23.5：剪辑功能整体移除**（老板定）。`EditService` 和下面定义的全部方法、数据结构、`edit_export` 导出 Runner 都已删除，本节不再是契约。旧的 `edit_export` 任务记录怎么处理见文首 v0.23.5 条和 6.6 `Retry`；`edit_projects` 表保留不动。

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
    Sources       []string     `json:"sources"`       // 素材库：绝对路径，去重，最多 100 个（v0.13 由 200 改为 100，产品经理定：素材 100），只是列表，不保证存在
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
1. **工程级**：`videoTrack` 不能为空（v1 同）；clip 总数（视频 + 音频）≤ 100；`sources` ≤ 100 且都是绝对路径；名称 1~80 字；序列化后 ≤ 1 MiB；`schemaVersion` = 1；输出参数范围；时间线总长 ≤ 6 小时（**按 clip 自填值 `max(startSec + (outSec − inSec) / speed)` 检查，不扣转场、不探测素材**，见 E）。
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
  "progress": 0, "version": 1 }
```
```json
// task:progress（edit_export，没有 fps / bitrateKbps / droppedFrames）
{ "id": "01J9Z7A1B2C3D4E5F6G7H8J9K0", "version": 5, "progress": 0.42, "speed": "1.8x", "etaSec": 6.1, "outTimeSec": 2.1 }
```

### 6.11.3 导出任务 `edit_export`

- 走 batch 池（与转换共用并发数），不占 live 池。`title` 形如 `<outputName>.mp4`；`inputPaths` = 去重后的素材路径（按首次出现顺序）；`outputPath` = 预期输出；`params` = `{project, options, outputDir}` 的 JSON。
- 输出：`<outputDir>/<outputName>.<format>`，重名追加 `(1)`、`(2)`（无空格；v0.23 只有 `convert` 改成带空格，剪辑导出不变，见 6.14.5），绝不覆盖，走 6.6 的 `RunWithPart`（`.part.<ext>` → 原子改名）；取消 / 失败删除 `.part`。
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
- `Retry`：注册 `edit_export` 的重试工厂，用 `params` 重建：重新做 6.11.2 的校验（素材已删除 → `NOT_FOUND`，不产生新任务，原记录不变；v0.23 起 `Retry` 是原地重试，见 6.6），输出目录沿用原来解析好的那个。
- 任务创建后再改工程不影响已提交的任务（`params` 已经是快照）。

### 6.11.4 预览方案（不做本地流服务）

1. **不用 `file://`**：Wails WebView 的页面源是 `wails://` / `http://wails.localhost`，`<video src="file:///...">` 会被 WebView 拒绝（Wails 官方 issue #292）。
2. **视频 / 音频预览 = AssetServer `Handler` 挂 `/local/<token>`**，协议、限长、token 生命周期、HEAD、失效处理全部见 **6.13**（EditService 与 DocService 共用）。`GetPreviewURL(path)` 校验：绝对路径、存在、是普通文件、扩展名在 v1 允许列表 `mp4 mov avi mkv flv webm m4v mp3 wav aac m4a flac ogg` 内，否则 `INVALID_ARGUMENT`；成功后到 6.13 的 **edit 登记表**登记，返回 `PreviewURL`。（v0.23：这个列表**不加** `gif`；转换页的预览另有接口和白名单，见 6.14.7。）
3. 监视器合成（多个 `<video>` + canvas）与 clip 滤镜的预览（CSS filter / canvas 像素处理）全在前端，和导出的 ffmpeg 效果只是近似，不保证逐像素一致（v1 同）。
4. **验证不通过时的回退方案（首版不实现）**：Windows 真机 Range 续传由用户在预览包里验证；不通过时走 `edit_proxy`（低分辨率短 mp4，≤ 32 MiB，整文件加载）。**首版不做，不新增方法、错误码、任务类型。**

### 6.11.5 工程存取

- **`EditProjectMeta.durationSec`**：`SaveProject` 返回（以及 `ListProjects` 列表）的时长是**按 clip 自填值估算的**，即 `max(startSec + (outSec − inSec) / speed)`，**不探测素材、不扣转场**（6.11.2 E）；精确时长以 `ValidateProject` 的 `EditPlan.durationSec` 为准。
- 表 `edit_projects(id, name, project JSON, updated_at)` 已在第 6 节。`SaveProject`：`id` 空 = 新建（ULID），否则更新（不存在 `NOT_FOUND`）；名称重复允许。**只校验数量上限（架构师定）**：名称去首尾空白后 1~80 字、clip 总数 ≤ 100、`sources` ≤ 100、序列化后 ≤ 1 MiB、`schemaVersion` ≤ 1，超了 `INVALID_ARGUMENT`。**不校验**同轨重叠、`outSec`、`speed` 等取值范围、路径是否存在（草稿可以保存，比如正在拖动中的时间线）；这些只在 `ValidateProject` 和 `Export` 报。所以 `LoadProject` 可能读出不合法的草稿，前端要能显示，导出前再调 `ValidateProject`。
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

## 6.12 DocService 契约（v0.12，只有契约，架构师冻结前不实现；v0.26 扩展为文档多格式转换，见 6.12.9）

> **v0.26**：6.12.1~6.12.8 现在只描述**简易转换**（组件未就绪时 docx / odt / txt → PDF，纯 Go 取文字）和 PDF 预览；“不用 LibreOffice”“odt / rtf / csv / txt 不支持”等说法只对简易转换成立。文档多格式转换见 **6.12.9~6.12.22**，冲突时以后者为准；**v0.27** 的本机 Office / WPS 引擎和文档预览见 **6.12.23~6.12.36**，与前面冲突时以 v0.27 的为准。

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
| `.doc` `.xls` `.ppt`（旧二进制格式）、`.odt` `.ods` `.odp` `.rtf` `.pages` `.numbers` `.key`、其他 | `UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=format`，第二行起写明原因（提交时整体校验失败则第二行是出错文件路径）；旧格式提示"请先另存为 docx / xlsx / pptx" |
| `.csv` `.txt` | **首版不支持**（架构师定，v1 也没有），`UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=format`（说明行 "暂不支持该格式"）；以后要加走增量契约版本 |
| 密码加密的 docx / xlsx / pptx（OLE 容器，不是 zip） | `UNSUPPORTED`，`message` "暂不支持这种格式"，`detail` 首行 `reason=encrypted`（说明行 "加密文档不支持（或旧版格式改了扩展名）…"） |

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
- **`UNSUPPORTED` 规则保留**：内嵌字体加载失败（构建错误才会发生）**且**没有可用系统 `.ttf`，而文档又含 U+00FF 以上的字符 → 该文件 `UNSUPPORTED`，`message` "没有可用的 Unicode 字体"，`detail` 首行 `reason=no_font`（不输出乱码 PDF）；纯 Latin-1 文档可用内置字体。正常构建下内嵌字体总是可用，该分支基本不会触发，但校验和单元测试要覆盖。
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

- 参数校验与 6.9 同一套规则：`inputs` 非空且 ≤ 50，路径必须绝对（`INVALID_ARGUMENT`），文件不存在 `NOT_FOUND`，是目录 `INVALID_ARGUMENT`，无读权限 `IO_ERROR`；`outputDir` 规则同 6.9（空 = `Settings.defaultOutputDir`，仍空 = `<base>/output`，v0.24.1，6.15.2），并在**提交时**同步校验（不放到任务里失败）：必须是绝对路径；**拒绝以 `\\?\`、`\\.\` 开头的路径**（`INVALID_ARGUMENT`）；**拒绝位于应用数据目录之内（含其本身）的路径**（`os.UserConfigDir()/FFmpegFree/`，防止把输出写进 `app.db`、`thumbs/`、`logs/` 旁边并被清理逻辑误伤，`INVALID_ARGUMENT`，`detail` 写 `outputDir 不能在应用数据目录内`；比较前对两边做 `EvalSymlinks` + 大小写按平台规则规范化）。**v0.24.1 例外（架构师定）**：`<dataDir>/output` 及其子文件夹**允许**（按固定文件夹名 `output` 放行，与这次启动是否回退无关；`<dataDir>/outputs`、`<dataDir>/logs`、`<dataDir>` 本身仍拒绝；经符号链接指到禁止区的同样拒绝）。这条规则同样用于转换的 `outputDir` 和设置里的自定义输出目录（6.15.2）；已存在必须是目录且可写，不存在则最近的已存在上级必须是可写目录。**"可写"的判断方式（#29 实现反馈）**：在该目录里**创建一个探测文件**（`os.CreateTemp(dir, ".ffmpegfree-probe-*")`，创建成功后立即关闭并删除），**不用**权限位或 `access()` 推断（Windows 的 ACL、只读挂载、网络盘上权限位不可靠）；创建失败（含权限不足、只读、磁盘满）一律返回 `IO_ERROR`，`detail` 写系统错误文本；探测文件删除失败只记日志，不影响结果。**先整体校验再提交**，任何一个不通过整体失败、不提交任何任务；`detail` 的形式（v0.16 更正）：**有 reason 的错误首行是 `reason=<枚举>`，第二行是出错文件的绝对路径**，其后是原因说明；没有 reason 的错误（相对路径、文件不存在、目录当文件等）首行仍是出错文件路径。前端定位出错文件时在前两行里找绝对路径。
- 整体校验里额外检查：扩展名在支持表内（否则 `UNSUPPORTED`）；文件 ≤ 100 MiB（否则 `INVALID_ARGUMENT`，`message` "文件超过 100 MiB"，`detail` 首行 `reason=too_large`）；能作为 zip 打开且含必需部件（docx `word/document.xml`，xlsx `xl/workbook.xml`，pptx 至少一张 `ppt/slides/slide<n>.xml`），打不开或缺部件 `INVALID_ARGUMENT`（`message` "不是有效的 OOXML 文件"，**`detail` 首行 `reason=invalid_ooxml`**）；**zip 条目数上限 100 000**：打开压缩包之前先只读文件尾部的 EOCD（含 zip64 记录）取条目总数，超过 100 000 → `INVALID_ARGUMENT`（`message` "不是有效的 OOXML 文件"，`detail` 首行 `reason=too_large`，说明行 "压缩包条目数超过 100000"，`internal/service/doc/zipcount.go` 的 `MaxZipEntries` / `checkZipEntries`）；读不出条目数（不是 zip、被截断）就交给后面的 zip 打开报错；不是 zip 而是 OLE 头（`D0 CF 11 E0`）→ `UNSUPPORTED`（加密或旧格式改了扩展名，`message` "暂不支持这种格式"，`detail` 首行 `reason=encrypted`）；单个 zip 条目解压后 > 256 MiB `INVALID_ARGUMENT`（防 zip 炸弹，`message` "不是有效的 OOXML 文件"，`detail` 首行 `reason=too_large`）；中央目录字节数 > 9 600 000（伪造 EOCD 防护）同样是 `reason=too_large`，zip64 目录信息无效（占位符没有 zip64 记录）是 `reason=invalid_ooxml`；字体规则见 6.12.1（需要 Unicode 字体而没有 → `UNSUPPORTED`，`message` "没有可用的 Unicode 字体"，`detail` 首行 `reason=no_font`，此项在提交时对文本做一次快速扫描，不通过整体失败）。
- **不依赖 ffmpeg**（不做 `FFMPEG_NOT_FOUND` 门控）。走 batch 池（与转换共用并发数）；`GoFuncRunner` 实际是 `task.RunnerFunc`。
- 任务：`type=office_pdf`，`title` 形如 `a.docx → PDF`，`inputPaths=[源]`，`outputPath` 为预期输出，`params={input, outputDir}` JSON。输出 `<源文件名去扩展名>.pdf`，重名追加 `(1)`、`(2)`（无空格，v0.23 不变，见 6.14.5），不覆盖，走 6.6 `RunWithPart`（`.part.pdf` → 原子改名）；取消或失败不留 `.part`。**已知边界（编号最大 99）**：`.part` 遗留清理只精确拼出 `<name>.part.pdf` 与 `<name>(1..99).part.pdf` 共 100 个候选名，`(n)` 大于 99 的残留文件不会被清理（与 `internal/service/doc/cleanup.go` 一致，不通配、不扫目录）。**启动时 `.part` 遗留清理是可选功能（契约"允许"，是否做由实现 PR 决定）**：若做，必须与 #22 的 6.11.3「`.part` 遗留清理」**五个条件完全一致，缺一不可**——① 文件名只能是由 `interrupted` 的 `office_pdf` 任务 `outputPath` 推出的 `<name>.part.pdf` 和 `<name>(n).part.pdf`（n=1..99）；② 位于该任务记录里登记的 `outputPath` 所在目录（不是任意目录，也不单独扫描默认输出目录）；③ 修改时间早于本次启动；④ 仅普通文件，`Lstat` 不跟随链接，符号链接和目录不动；⑤ 不递归，只看输出目录第一层，不 `ReadDir`。任务管理器统一版（覆盖 `convert`、`office_pdf`）后续单独做（见 6.11.3）。
- **进度**：按处理单元计数（docx 段落、xlsx 行、pptx 幻灯片）占总数的比例，0~1 单调，完成为 1；每处理约 100 个单元检查一次 ctx，取消响应 ≤ 1 秒（超大文件除外）。`task:progress` 载荷不变，`speed` / `etaSec` 为空。
- 页数上限 5000：**输出页数超过 5000（生成过程中累计到第 5001 页时立即停止）返回 `UNSUPPORTED`**，**`message` "超过 5000 页"（v0.16 更正：原文写成 detail），`detail` 首行 `reason=too_many_pages`**，第二行是说明（"已排到第 N 页仍未结束"），不产生输出文件（`.part` 删除）；xlsx 一个工作表所有行都算；xlsx 单元格文本每格最多 32 767 字符（Excel 自身上限），超出截断。**页数按"正在生成的 PDF 的页码"统计，折行产生的页也算**（与 `origin/feat/doc-impl` 4d83299 的 `render.go` 一致：每处理完一个单元检查一次 `pdf.PageNo() > 5000`，写长段落时逐行也检查，超过即停止，所以不会生成超过 5000 页的 PDF）。**已知边界**：① 检查粒度是"一个单元 / 一行"，没有 Unicode 字体时（helvetica 兜底路径，只有西文文档会走到）一个超长段落用 `MultiCell` 整段排完才检查，这一段可能超过 5000 页很多再被拒绝（仍是 `UNSUPPORTED`、不产生输出）；② 提取出的文字总量超过 64 MiB 直接按"超过 5000 页"处理（`UNSUPPORTED`，`message` "超过 5000 页"，`detail` 首行 `reason=too_many_pages`，说明行 "文档文字量超过上限"），即使按这些文字排出的页数没到 5000。
- 错误码（任务的 `error`）：`IO_ERROR`（读写失败，没有权限）、`CONVERT_DISK_FULL`（输出写盘失败且是磁盘满，判定规则同 6.9 的系统错误文本匹配；Office 转换也用这个码，前端标题相同）、`UNSUPPORTED`、`INVALID_ARGUMENT`（运行时才发现的损坏，`reason=invalid_ooxml` / `too_large`）、`INTERNAL`（fpdf / excelize 意外错误，`detail` 是错误文本）；取消是任务状态 `canceled`。
- `Retry`：注册 `office_pdf` 的重试工厂，用 `params` 重建并重新校验（输入被删除 `NOT_FOUND`，原记录不变）。v0.23 起 `Retry` 是原地重试（复用 id，见 6.6）。
- v1 的"按文件名防重复转换"（`officeConvertingFiles`）取消：两个任务转同一个输入是允许的，输出各自取不冲突的名字。

### 6.12.4 PDF 预览方案（不做本地流服务）

渲染**完全在前端**：沿用 v1 的 `@tato30/vue-pdf`（pdf.js），后端不渲染成图片、不提供页数 / 文本 / 缩略图接口（后端无纯 Go 的可靠 PDF 渲染器，也不打包 `pdftoppm` 之类外部程序）。后端只负责把字节交给前端：

1. `OpenPDF(path)`：路径必须绝对（`INVALID_ARGUMENT`）、存在（`NOT_FOUND`）、是文件（否则 `INVALID_ARGUMENT`）、可读（`IO_ERROR`）、扩展名 `.pdf`（不区分大小写，否则 `INVALID_ARGUMENT`，`message` "只支持 .pdf 文件"，`detail` 首行 `reason=format`）、前 1024 字节内含 `%PDF-`（否则 `INVALID_ARGUMENT`，`message` "不是 PDF 文件"，`detail` 首行 `reason=format`，第二行是路径）、大小 ≤ 512 MiB（否则 `INVALID_ARGUMENT`，`message` "文件超过 512 MiB"，`detail` 首行 `reason=too_large`；`ReadPDFChunk` 发现文件已变大超限时同样）。成功后登记句柄并写入 / 更新 `doc_recent`。加密 PDF 也能打开，密码由前端 pdf.js 的 `onPassword` 弹窗处理，后端不接触密码。
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

**`message` 与 `detail` 首行对照（v0.16，与 `internal/service/doc` 一致；`message` 是精确文案，前端精确匹配，不得改动）**：

| 场景 | code | `message`（精确） | `detail` 首行 | 出现位置 |
|---|---|---|---|---|
| 超过 5000 页（输出页数，含折行；说明行 "已排到第 N 页仍未结束"） | `UNSUPPORTED` | `超过 5000 页` | `reason=too_many_pages` | 任务 `error` |
| 提取的文字总量超过 64 MiB（说明行 "文档文字量超过上限"） | `UNSUPPORTED` | `超过 5000 页` | `reason=too_many_pages` | 任务 `error` |
| 不支持的扩展名（doc / xls / ppt / odt / ods / odp / rtf / csv / txt / pdf / pages…）、没有扩展名 | `UNSUPPORTED` | `暂不支持这种格式` | `reason=format` | `ConvertToPDF` 同步校验 |
| 加密的 Office 文档（OLE 头 `D0 CF 11 E0`） | `UNSUPPORTED` | `暂不支持这种格式` | `reason=encrypted` | `ConvertToPDF` 同步校验 |
| 需要 Unicode 字体而没有 | `UNSUPPORTED` | `没有可用的 Unicode 字体` | `reason=no_font` | 同步校验 / 任务 `error` |
| 不是 zip、空文件、缺必需部件、XML 损坏、zip64 目录信息无效 | `INVALID_ARGUMENT` | `不是有效的 OOXML 文件` | `reason=invalid_ooxml` | 同步校验 / 任务 `error` |
| zip 条目数 > 100 000、中央目录 > 9 600 000 字节、单个条目解压后 > 256 MiB | `INVALID_ARGUMENT` | `不是有效的 OOXML 文件` | `reason=too_large` | 同步校验 / 任务 `error` |
| Office 输入文件 > 100 MiB | `INVALID_ARGUMENT` | `文件超过 100 MiB` | `reason=too_large` | `ConvertToPDF` 同步校验 |
| `OpenPDF` 扩展名不是 `.pdf` | `INVALID_ARGUMENT` | `只支持 .pdf 文件` | `reason=format` | `OpenPDF` |
| `OpenPDF` 内容不是 PDF（含空文件） | `INVALID_ARGUMENT` | `不是 PDF 文件` | `reason=format` | `OpenPDF` |
| PDF > 512 MiB（含 `ReadPDFChunk` 时文件已变大） | `INVALID_ARGUMENT` | `文件超过 512 MiB` | `reason=too_large` | `OpenPDF` / `ReadPDFChunk` |

**没有 reason 的 Doc 错误**（保持现有 code 和 `detail`，`detail` 首行不是 `reason=`）：取消（`CANCELED` "操作已取消"）；磁盘满（`CONVERT_DISK_FULL`）；读写失败 / 无权限（`IO_ERROR`）；文件 / PDF / 句柄不存在或已失效（`NOT_FOUND`）；路径非绝对、路径不合法、目录当文件、不是普通文件、`outputDir` 相关、`inputs` 为空或超过 50 个、`length` / `offset` 越界、`RemoveRecentPDFs` 超过 500 个（`INVALID_ARGUMENT`）；内部错误（`INTERNAL`）；加密 PDF 没有错误（能打开，密码由前端 pdf.js 处理），所以**没有 PDF 的 `reason=encrypted`**；LibreOffice 缺失 / 超时不适用（纯 Go 实现，不依赖外部程序）。

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
    "version": 1, "createdAt": 1790000000000, "startedAt": 0, "finishedAt": 0 } ]
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

AppError（转换不支持的格式；v0.16：`detail` 首行是 `reason=`，第二行是出错文件路径）：
```json
{ "code": "UNSUPPORTED", "message": "暂不支持这种格式", "detail": "reason=format\nC:\\Docs\\旧文档.doc\n.doc：旧版二进制格式，请先另存为 docx" }
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

## 6.12.9 文档多格式转换（v0.26，一期；架构师定，范围由老板和产品经理定）

> **v0.26 扩展 6.12**：6.12.1~6.12.8 保留，含义收窄为“**简易转换**（纯 Go 取文字转 PDF）+ PDF 预览”；本节（6.12.9~6.12.22）是新的文档多格式转换。两边冲突时以本节为准。**只有契约，前后端按本版并行实现。**

### 6.12.10 范围

| 类别（`family`） | 格式（源和目标都在这个列表里） | 同类互转 | 转 PDF |
|---|---|---|---|
| 文字 `text` | `doc` `docx` `odt` `rtf` `txt` `html`（含 `.htm`，输出一律 `.html`） `md`（含 `.markdown`，输出一律 `.md`） | 任意两种之间（不转成自己） | 都可以 |
| 表格 `sheet` | `xls` `xlsx` `ods` `csv` | 任意两种之间 | 都可以 |
| 演示 `slide` | `ppt` `pptx` `odp` | 任意两种之间 | 都可以 |

- **跨类不转**（如 docx → xlsx、csv → txt 都没有）。`pdf` 只作输出；**PDF 作为输入一期不支持**（添加时拒绝，`DOC_PDF_INPUT_UNSUPPORTED`，6.12.20）。**v0.28 取消这条限制，见 6.12.58~6.12.64**。
- **多工作表转 CSV 只输出第一个工作表**，格式表给一行提示（`hintKey=csv_first_sheet`），添加文件时给出 `sheetCount`（6.12.16）。
- **二期（只列为后续，本版不做、不留接口）**：PDF 按页转图片、PDF 转 Word、epub（读和写）。
- 输入大小上限沿用 6.12.2 `maxInputBytes`（100 MiB），一次最多添加 / 提交 50 个。

### 6.12.11 实现方式

- **文档组件 = LibreOffice，以 headless 方式运行**（`soffice --headless --convert-to`）。**界面上只叫「文档组件」**，`message`、任务标题、格式表、状态文字都不出现 `LibreOffice` / `soffice` / `ffmpeg`（1.1 扩展到文档组件）。**唯一例外**：Linux 上文档组件未就绪时的提示 `请先在系统里安装 LibreOffice，然后重启应用。`（`DOC_COMPONENT_NOT_READY` 在 Linux 上的 `message`，以及 Linux `missing` 状态的 `error.message`）。代码标识符、日志、只给开发者看的 `detail` 不受限。`TestUserFacingMessageIsChinese` 增加断言：除这一句外后端 message 字面量不含 `LibreOffice`（不区分大小写）。
- **Markdown（纯 Go，不需要 cgo，三平台照常交叉编译）**：
  - md → html：`github.com/yuin/goldmark`，开 GFM 扩展（表格、删除线、自动链接、任务列表），**不开** `html.WithUnsafe`（md 里的原始 HTML 被丢弃，防止脚本）。输出完整的 HTML 文档：`<!DOCTYPE html><html><head><meta charset="utf-8"><title>文件名</title></head><body>…</body></html>`。
  - html → md：`github.com/JohannesKaufmann/html-to-markdown/v2`（加表格插件），输出 UTF-8、`\n` 换行、无 BOM。`<script>` / `<style>` 内容丢弃。
  - **md → 其他格式**（docx / doc / odt / rtf / txt / pdf）：先按上面转成临时 HTML，再交给文档组件（输入过滤器 `HTML (StarWriter)`）。
  - **其他格式 → md**（doc / docx / odt / rtf / txt）：先由文档组件转成临时 HTML（`html:XHTML Writer File:UTF8`），再用 html-to-markdown 转 md；组件导出的图片文件丢弃（md 只保留文字和基本格式，格式表提示 `md_lossy`）。
  - **md 里的图片**：本地相对路径按 **md 文件所在目录**解析（转换页添加时会复制副本，6.15；解析时用**原文件**所在目录 `originalPath`，原文件已不在时用副本目录），绝对路径原样；解析后转成 `file:///` 绝对 URL 写进临时 HTML，文件不存在的图片换成它的 alt 文字。**网络图片（`http:` / `https:` / `//` 开头）不下载**：在交给组件之前替换成 alt 文字（没有 alt 时去掉），组件不会发出网络请求。md → html 时网络图片的 `<img src>` 原样保留（只是文本，不下载）。
- **其余转换**全部交给文档组件，每个任务一条命令（参数原文）：
  ```
  soffice --headless --invisible --norestore --nologo --nodefault --nolockcheck --nofirststartwizard
          -env:UserInstallation=file:///<临时配置目录> [--infilter=<输入过滤器>]
          --convert-to <目标> --outdir <临时输出目录> <输入文件>
  ```
  Windows 用 `program\soffice.com`（控制台版，等待转换结束并有退出码），macOS `LibreOffice.app/Contents/MacOS/soffice`，Linux `soffice`。子进程一律经 `proc.Start`（Windows Job Object、隐藏窗口；其他平台进程组），见 6.6 v0.9.2。
- **目标参数**（`--convert-to` 的值）：

  | 目标 | 文字类 | 表格类 | 演示类 |
  |---|---|---|---|
  | pdf | `pdf:writer_pdf_Export` | `pdf:calc_pdf_Export` | `pdf:impress_pdf_Export` |
  | docx / doc / odt / rtf | `docx:"MS Word 2007 XML"` / `doc:"MS Word 97"` / `odt:writer8` / `rtf:"Rich Text Format"` | — | — |
  | txt | `txt:"Text (encoded)":UTF8` | — | — |
  | html | `html:"XHTML Writer File":UTF8` | — | — |
  | xlsx / xls / ods | — | `xlsx:"Calc MS Excel 2007 XML"` / `xls:"MS Excel 97"` / `ods:calc8` | — |
  | csv | — | `csv:"Text - txt - csv (StarCalc)":44,34,76,1,,0,false,true,false,false,false,1`（逗号、双引号、UTF-8、第 12 项 `1` = 只导出第一个工作表） | — |
  | pptx / ppt / odp | — | — | `pptx:"Impress MS PowerPoint 2007 XML"` / `ppt:"MS PowerPoint 97"` / `odp:impress8` |

  输入过滤器：`txt` 用 `--infilter="Text (encoded):UTF8"`；`html` 和 md 生成的临时 HTML 用 `--infilter="HTML (StarWriter)"`（在 Writer 里打开，不进 Writer/Web）；`csv` 用 `--infilter="CSV:44,34,76,1"`（先按 6.12.17 转成 UTF-8）；其余由组件按内容识别。`txt` / `md` 输入同样先按 6.12.17 的编码规则转成 UTF-8（与 CSV 同一个函数）。
- **输出落盘**：组件写进本任务的临时输出目录，后端找到唯一的目标文件（找不到 = `DOC_COMPONENT_CRASHED`，6.12.20），再按 6.6 `RunWithPart` 的方式移到最终位置；最终名在提交时定名并占位，重名用转换页的格式 `a (1).docx`（6.14.5）。临时配置目录、临时输出目录在任务结束（任何终态）时删除。
- **最低版本**：系统安装的文档组件主版本需 **≥ 7.2**（CSV 导出选工作表的第 12 项从 7.2 起有）；新版本号 `26.2` 这类按数值比较，大于 7。

### 6.12.12 文档组件的检测、下载与安装

**固定版本：LibreOffice 26.2.6**（构建号 26.2.6.3，TDF 2026 年 10 月在 `stable/` 下的较成熟分支；26.8.x 是更新的分支，一期不用）。下面的大小和 SHA-256 **在箱子上实际下载核对过**（与官方 `.sha256` 文件一致，2026-10-09）：

| 平台 | 主地址 | 大小（字节） | SHA-256 |
|---|---|---|---|
| windows-amd64（MSI） | `https://download.documentfoundation.org/libreoffice/stable/26.2.6/win/x86_64/LibreOffice_26.2.6_Win_x86-64.msi` | 373 252 096 | `f9877032fd908beb9c0ddf06df4af5c2e85f419c42e14876c4cce5aae5fb2660` |
| darwin-amd64（dmg） | `https://download.documentfoundation.org/libreoffice/stable/26.2.6/mac/x86_64/LibreOffice_26.2.6_MacOS_x86-64.dmg` | 308 308 875 | `135b8a95b8133396d54bf8e726dbc0066145efa0d785963fc3d4592acbfcfe5b` |
| darwin-arm64（dmg） | `https://download.documentfoundation.org/libreoffice/stable/26.2.6/mac/aarch64/LibreOffice_26.2.6_MacOS_aarch64.dmg` | 297 798 926 | `94bb3248df074c225490a8a6d1d9dc87c7d6783dbb7a8e9f0d0c3d94348552af` |
| linux | 不下载，只检测 | — | — |

- **备用地址（按顺序尝试，同一个文件、同一个 SHA-256，已核对）**：① 官方归档（永久保留，`stable/` 换版本后主地址会失效）：`https://downloadarchive.documentfoundation.org/libreoffice/old/26.2.6.3/win/x86_64/LibreOffice_26.2.6.3_Win_x86-64.msi`、`…/26.2.6.3/mac/x86_64/LibreOffice_26.2.6.3_MacOS_x86-64.dmg`、`…/26.2.6.3/mac/aarch64/LibreOffice_26.2.6.3_MacOS_aarch64.dmg`；② 国内镜像 `cn`（只核对了 Windows 包，大小一致）：`https://mirrors.ustc.edu.cn/tdf/libreoffice/stable/26.2.6/<同主地址的路径>`。主地址是 TDF 的重定向器，会 302 到就近镜像。清单写进 `internal/doccomp/manifest.json`（`go:embed`），字段同 9.3 的 ffmpeg 清单（URL 列表、大小、SHA-256、版本）；**不用 latest 地址**，以后换版本走契约变更。
- **检测顺序**（启动时后台执行，不阻塞界面；文档页打开时如果还没检测过也触发）：① 应用下载的组件（`<组件目录>/doc/26.2.6/`，见下）；② 系统安装：Windows 读注册表 `HKLM\SOFTWARE\LibreOffice\UNO\InstallPath`（再看 `WOW6432Node`），再看 `%ProgramFiles%\LibreOffice\program\soffice.com`；macOS `/Applications/LibreOffice.app`、`~/Applications/LibreOffice.app`；Linux `PATH` 里的 `soffice` / `libreoffice`，再看 `/usr/lib/libreoffice/program/soffice`、`/usr/lib64/libreoffice/program/soffice`、`/opt/libreoffice*/program/soffice`（snap / flatpak 不支持）。**校验**：`soffice --headless --version`（60 秒超时，第一次运行慢）解析出版本号 ≥ 7.2，再用临时配置目录做一次 1 行 txt → pdf 的冒烟转换（120 秒超时），产出非空 PDF 才算通过。第一个通过的就是当前组件。应用下载的优先于系统安装（版本固定、已测过）。
- **组件目录**：Windows `%LocalAppData%\FFmpegFree\components\`（**不放** `%AppData%`：解包后约 1.5 GiB，漫游配置不适合），macOS `~/Library/Application Support/FFmpegFree/components/`。不改系统 PATH、不写注册表、不出现在系统的“已安装程序”里。
- **下载**（`InstallDocComponent`，不是任务，不进任务中心，进度走事件 6.12.15）：
  1. 下载前检查组件目录所在磁盘的剩余空间：需要 `包大小 + 2 GiB`，不够返回 `CONVERT_DISK_FULL`（`reason=no_space` + `needBytes` / `freeBytes`，沿用 2.2），不开始下载。
  2. 写 `<组件目录>/tmp/doc-26.2.6-<sha 前 12 位>.part`，**支持断点续传**（HTTP `Range`；服务器不支持 Range 时从头下；断线自动重试 3 次再换下一个地址）。取消、失败都**保留** `.part`，下次 `InstallDocComponent` 接着下。
  3. 下完校验 SHA-256：**不通过就删掉 `.part`、`receivedBytes` 和进度归 0，从头重下一次**（换下一个地址）；仍不通过则失败，`DOC_CHECKSUM_FAILED`。
  4. **preparing（准备）阶段**：
     - **Windows**：`msiexec /a "<msi>" /qn TARGETDIR="<组件目录>\doc\26.2.6.staging" /L*v "<组件目录>\tmp\doc-msi.log"`。管理员解包（administrative install）只是把文件解到目标目录，**不需要管理员权限、不弹 UAC**、不登记已安装程序。按 MSI 内的目录结构，`soffice.com` 预期在 `<TARGETDIR>\LibreOffice\program\`（箱子上用 msitools 看过包内结构：`LibreOffice/program/soffice.exe`、`soffice.com`、`soffice.bin` 都在；**管理员解包后的实际层级未在 Windows 真机验证**，实现在 TARGETDIR 下最多 4 层内找 `program\soffice.com`）。msiexec 退出码非 0 / 1641 / 3010 → `DOC_COMPONENT_INSTALL_FAILED`（`detail` 第一行 `msiexec=<退出码>`，后面是日志尾部，**不含路径**）。
     - **macOS**：`hdiutil attach -nobrowse -readonly -noautoopen -mountpoint <临时挂载点> <dmg>`，`ditto <挂载点>/LibreOffice.app <组件目录>/doc/26.2.6.staging/LibreOffice.app`，`hdiutil detach <挂载点>`（失败加 `-force` 再试一次）；拷完 `xattr -dr com.apple.quarantine`（我们自己下载的文件通常没有这个属性，有就清）。【未验证】dmg 是否带需要同意的许可协议（带的话 `attach` 会卡住，需加 `-noverify` 并通过 stdin 应答，实现时实测）。
     - 解包后执行上面的**检测校验**（版本 + 冒烟转换），通过后把 `.staging` 原子改名为 `doc/26.2.6/`（旧目录先改名再删，失败回滚），删掉 `.part` 和安装包，状态变 `ready`。**preparing 一直持续到检测通过**，界面显示 `正在准备文档组件…`，**不给百分比**（前端用不确定进度条）；所以不会出现“进度条到 100% 停住”。
  5. **取消**（`CancelDocComponentInstall`）：下载中取消保留 `.part`；准备中取消会结束 msiexec / hdiutil 进程树、删 `.staging`，保留安装包（下次直接从准备开始）。取消后重新检测（有系统安装就是 `ready`，否则 `missing`）。
  6. **重试**：失败（`failed`）或取消后再调 `InstallDocComponent` 即可，从断点继续。`InstallDocComponent` 幂等：已在下载或准备时直接返回当前状态，不开第二个。
  7. Linux 调 `InstallDocComponent` 返回 `UNSUPPORTED_PLATFORM`（message `请先在系统里安装 LibreOffice，然后重启应用。`）。
- 文档组件**不影响转换组件**：两者各自检测、各自下载，状态互不影响；转换页、直播不依赖文档组件，文档页不依赖转换组件（DocService 任何方法都不返回 `FFMPEG_NOT_FOUND`，9.5 不变）。

### 6.12.13 数据结构

```go
type DocComponentStatus struct {
    State         string    `json:"state"`         // checking | ready | missing | outdated | downloading | preparing | failed
    Version       string    `json:"version"`       // 当前组件的版本号（系统安装和应用下载的都读，取 --version 输出里的数字版本如 "26.2.6.3"、"7.6.4.1"）；读不出或没有组件时为 ""
    Source        string    `json:"source"`        // ready 时：system（系统安装）| downloaded（应用下载）；其他状态为 ""
    CanDownload   bool      `json:"canDownload"`   // 当前平台有下载源（Windows x64、macOS x64 / arm64 为 true，Linux 为 false）
    DownloadBytes int64     `json:"downloadBytes"` // 安装包大小（字节），没有下载源时为 0；界面“约 xxx MB”
    Phase         string    `json:"phase,omitempty"`         // downloading | preparing，只在这两个状态时有（与 state 相同，方便前端直接用）
    ReceivedBytes int64     `json:"receivedBytes,omitempty"` // downloading 时已下载字节（含续传前已有的部分）
    Error         *AppError `json:"error,omitempty"`         // missing（Linux）/ outdated / failed 时有；detail 不含路径
    Path          string    `json:"-"`                       // 当前组件可执行文件路径，只在后端用，**不给前端**（同 9.4 v0.25.4）
}

type DocFormatMatrix struct {
    ComponentReady bool             `json:"componentReady"` // 生成这张表时文档组件是否 ready
    Inputs         []string         `json:"inputs"`         // 可以添加的扩展名（不带点、小写）：doc docx odt rtf txt html htm md markdown xls xlsx ods csv ppt pptx odp
    Sources        []DocSourceFormats `json:"sources"`      // 每个源格式一项（htm / markdown 与 html / md 共用一项，见 aliases）
}
type DocSourceFormats struct {
    Ext     string      `json:"ext"`               // 源格式
    Aliases []string    `json:"aliases,omitempty"` // html → ["htm"]，md → ["markdown"]
    Family  string      `json:"family"`            // text | sheet | slide | pdf（v0.28）
    Targets []DocTarget `json:"targets"`           // 能转成的目标，固定顺序：pdf 在最前，其余按 6.12.10 表里的顺序；不含自己
}
type DocTarget struct {
    Ext            string `json:"ext"`            // pdf docx doc odt rtf txt html md xlsx xls ods csv pptx ppt odp
    DisplayName    string `json:"displayName"`    // 如 "PDF"、"Word (DOCX)"、"Word 97-2003 (DOC)"、"Markdown (MD)"、"网页 (HTML)"
    NeedsComponent bool   `json:"needsComponent"` // 这个转换要用文档组件（与组件当前状态无关；md↔html 为 false）
    Simple         bool   `json:"simple"`         // true = 这次会走简易转换（只保留文字）：只在组件未就绪时、docx / odt / txt → pdf 上为 true
    Available      bool   `json:"available"`      // 现在能选：!needsComponent || componentReady || simple
    HintKey        string `json:"hintKey,omitempty"` // 选中这个目标时显示的一行提示：csv_first_sheet | md_lossy | simple_mode | pdf_layout | pdf_text（v0.28 / v0.28.2）
    Hint           string `json:"hint,omitempty"`    // 对应文案（见下表），前端可直接显示
    DisabledReason string `json:"disabledReason,omitempty"` // available=false 时的悬停文字：`需要文档组件`
}
```

**提示文案（`hintKey` → `hint`）**：

| hintKey | 出现在 | 文案 |
|---|---|---|
| `csv_first_sheet` | xls / xlsx / ods → csv | `转成 CSV 只会保留第一个工作表。`（前端在选中文件的 `sheetCount > 1` 或 `= -1` 且目标是 CSV 时显示，6.12.16） |
| `md_lossy` | 任何格式 → md（**v0.28.2：只用于非 PDF 源**） | `转成 Markdown 只保留文字和基本格式，图片和复杂表格会丢失。` |
| `simple_mode` | 组件未就绪时 docx / odt / txt → pdf；**v0.28 / v0.28.2**：没有组件时的 pdf → html（pdf → txt 自 v0.28.2 起改用 `pdf_text`） | `下载文档组件后可保留图片和排版`；**PDF 源**：`只提取文字，不保留排版和图片。`（v0.28.1 定稿） |
| `pdf_text` | **v0.28.2**：pdf → txt、pdf → md | `只提取文字，不保留排版和图片。`（产品 10-09 定） |
| `pdf_layout` | **v0.28**：pdf → doc / docx / odt / rtf | `PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。`（v0.28.1 定稿） |

**组件未就绪时的总提示**（文档页顶部，前端常量，后端不给）：`Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转`。**v0.28.1**：打开 PDF 输入后为 `Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转，PDF 可以提取文字转成 TXT、md 和简易网页。`

### 6.12.14 接口（DocService，v0.26 新增）

```go
// 格式表与组件
GetFormatMatrix() (DocFormatMatrix, error)                 // 按当前组件状态生成；~~组件在 checking 时最多等 6 秒（同 6.16 v0.25.4），等不到按未就绪返回~~ **v0.28.3：不等，立即按当前已知状态返回（6.12.68）**
GetDocComponentStatus() (DocComponentStatus, error)
InstallDocComponent(mirror string) (DocComponentStatus, error) // mirror 只接受 "" 和 "cn"；开始或继续下载 + 准备，立即返回（downloading / preparing）；幂等；Linux UNSUPPORTED_PLATFORM
CancelDocComponentInstall() error                          // 没有在下载 / 准备时无操作
RecheckDocComponent() (DocComponentStatus, error)          // 重新检测（用户自己装了系统版本后）；下载 / 准备中返回当前状态，不打断

// 文档页的源文件行与提交（复用转换页的记录结构，6.12.16）
AddDocSources(paths []string) ([]AddDocSourceResult, error)
ListDocSources(filter ConvertSourceFilter) (DocSourcePage, error)
SearchDocSources(filter ConvertSearchFilter) (DocSourcePage, error)
SubmitDocConvert(req DocSubmitRequest) (ConvertSubmitResult, error) // 一个源一个任务；返回 {tasks, skipped}，规则同 SubmitSources
```

```go
type AddDocSourceResult struct {   // 与入参一一对应
    Path   string        `json:"path"`
    Source *DocSource    `json:"source,omitempty"`
    Error  *AppError     `json:"error,omitempty"`  // 被拒的原因，6.12.20：DOC_PDF_INPUT_UNSUPPORTED / DOC_FORMAT_UNSUPPORTED / DOC_ENCRYPTED / DOC_CORRUPT / NOT_FOUND / IO_ERROR / INVALID_ARGUMENT（> 100 MiB，reason=too_large）
}
type DocSource struct {
    ConvertSource                        // 同 6.14.2 / 6.15.3（id、originalPath、storedPath、copyState…），media 恒为空
    Ext        string `json:"ext"`        // 规范化后的源格式（htm → html，markdown → md）
    Family     string `json:"family"`     // text | sheet | slide | pdf（v0.28）
    SheetCount int    `json:"sheetCount"` // 只对表格类有意义：xlsx / ods / xls 实际读出的工作表数，csv 固定 1，读不出为 -1；其他类别为 0
}
type DocSourceEntry struct { Source DocSource `json:"source"`; Records []Task `json:"records"`; RecordCount int64 `json:"recordCount"` } // 同 ConvertSourceEntry
type DocSourcePage struct { Items []DocSourceEntry `json:"items"`; Total int64 `json:"total"` }
type DocSubmitRequest struct {
    SourceIDs []string `json:"sourceIds"` // 1~50
    Target    string   `json:"target"`    // 目标扩展名；必须对每个源都 available（多选取交集，前端保证，后端再校验）
    OutputDir string   `json:"outputDir"` // 空 = defaultOutputDir，仍空 = <base>/output（同 6.12.3 / 6.15）
}
```

- 按 id 操作的接口**直接复用 ConvertService / TaskService**，对文档页的行同样有效：`GetSource`、`ListSourceRecords`、`CheckSources`、`DeleteRecords`、`DeleteSource`、`CancelCopy`、`RetryCopy`、`RevealSource`、`RevealRecord`、`OpenSourceWithSystem`、`TaskService.Retry` / `CheckPaths` / `OpenWithSystem` / `GetLog`。`GetSourcePreviewURL` / `GetRecordThumbnail` / `GetSourceThumbnail` 对文档行返回 `UNSUPPORTED`（`reason=format`），前端不调。PDF 输出的预览用 6.12.4 的 `OpenPDF(outputPath)`。
- `ConvertService.ListSources` / `SearchSources` 只返回 `kind='media'` 的行，`DocService.ListDocSources` / `SearchDocSources` 只返回 `kind='doc'` 的行（迁移 0009，6.12.21）。两页能添加的扩展名不重叠，同一路径不会同时属于两页。
- **旧接口**：`ConvertToPDF`、`GetDocCapabilities` 保留（旧前端、旧记录），新文档页不再调用。

### 6.12.15 事件（v0.26 新增）

| 事件名 | payload | 频率 |
|---|---|---|
| `doc:component` | `DocComponentStatus`（完整，没有路径） | 状态变化时（检测完成、开始下载、进入准备、ready、failed、取消） |
| `doc:component-progress` | `{ phase, receivedBytes, totalBytes, progress? }`：`phase=downloading` 时带 `progress`（0~1）；**`phase=preparing` 时不带 `progress`**（不确定进度）；SHA-256 不通过重下时发一条 `receivedBytes=0, progress=0` | 下载中最多 4 次/秒；进入 preparing 时 1 次 |
| `doc:queue` | `{ items: [{ id, queuePosition }] }`：文档组件池里所有排队任务的当前位置（6.12.18） | 文档组件池的队列变化时（入队、出队、取消），节流 4 次/秒 |

- 前端：`doc:component` 的 `state` 变为 `ready`（或从 `ready` 变为别的）时**重新调 `GetFormatMatrix`**；页面初始化先订阅事件再调 `GetDocComponentStatus`、`GetFormatMatrix`（同第 5 节的订阅顺序）。
- 文档任务的进度和状态仍走 `task:created` / `task:progress` / `task:status`（第 5 节），事件顺序规则同 v0.25.1。

### 6.12.16 添加文件

- 只收 `DocFormatMatrix.inputs` 里的扩展名（不区分大小写）。`.pdf` → `DOC_PDF_INPUT_UNSUPPORTED`（**v0.28 起可以添加**，大小 200 MiB、500 页，见 6.12.60）；其他扩展名（含没有扩展名）→ `DOC_FORMAT_UNSUPPORTED`。
- 添加时同步做轻量检查（每个文件最多 5 秒，最多 4 个并发）：大小 ≤ 100 MiB；**加密检测**（6.12.19）命中 → `DOC_ENCRYPTED`，**不建行**；容器明显损坏（OOXML / ODF 不是 zip 或缺必需部件、OLE 头错、文件为空）→ `DOC_CORRUPT`，不建行。rtf / txt / html / md / csv 只检查非空和可读。
- 通过后建 `convert_sources` 行（`kind='doc'`），后台复制副本（6.15，事件 `convert:copy`），转换只读副本。
- **`sheetCount`**：xlsx 读 `xl/workbook.xml` 里 `<sheet>` 的个数；ods 读 `content.xml` 里 `<table:table>` 的个数（流式解析，不整份读入）；xls 数 Workbook 流里 BOUNDSHEET（0x0085）记录的个数；csv 固定 `1`；读不出（解析失败、超时）为 `-1`；文字、演示类为 `0`。持久化在行上，不重复读。
- **转 CSV 的结果警告**：源 `sheetCount > 1`（或 `-1` 且组件实际只导出了第一个）时，成功任务的 `result.warnings` 带 `csv_first_sheet_only`（6.14.6 的 warnings 机制），前端可在记录上显示同一句提示。

### 6.12.17 CSV、TXT、MD 的编码

- **输出 CSV**：固定 **UTF-8（不带 BOM）+ 逗号分隔 + 双引号包裹**，第一行原样输出（见 6.12.11 的导出参数）。
- **读入 CSV / TXT / MD**：内容是合法 UTF-8（开头有 BOM 也算，BOM 去掉）就按 UTF-8；否则按 **GB18030**（兼容 GBK / GB2312，`golang.org/x/text/encoding/simplifiedchinese`，纯 Go）解码，转成 UTF-8 写进临时文件再交给组件 / goldmark。**不提示用户**、没有设置项。判断看整份文件（≤ 100 MiB），不只看开头。
- 读入 CSV 一期只认逗号分隔；分号、制表符分隔的文件会被当成一列（后续再说）。

### 6.12.18 并发、排队与超时

- **文档组件池**：只有**真正启动文档组件进程的 `doc_convert` 任务**进这个池，**并发上限固定 2**（不跟 `Settings.maxConcurrent` 走），FIFO 排队。它是**单独的池**，和转换页 / 安装用的 batch 池（转换组件）**互不占用名额**：转换页满了不影响文档页，反之亦然。
- **不进池、不排队**：md ↔ html（纯 Go）和简易转换（纯 Go，`office_pdf`）提交后直接开始运行（各自一个 goroutine，不受 2 个名额限制，也不占 batch 池）。
- **排队位置**：文档组件池里排队的任务带 **`queuePosition`**（前面还有几项，从 0 开始：0 = 下一个就轮到它）。`Task` 新增可选字段 `queuePosition int \`json:"queuePosition,omitempty"\``（只在内存，不落库；只有文档组件池里 `queued` 的任务有，Go 用指针以便 0 也输出）；`task:created`、排队中的 `task:status`、`Get` / `List` / `ListActive` 都带；位置变化时发 `doc:queue`（6.12.15）。界面显示 **`排队中 · 前面还有 n 项`**（n = `queuePosition`；n = 0 时前端可显示 `排队中 · 下一个`，文案由产品定）。
- **每个任务独立的临时配置目录**：`<数据目录>/tmp/doc/<任务id>/profile`，以 `-env:UserInstallation=file:///<该目录，正斜杠，空格等按 URL 编码>` 传入（文档组件同一个配置目录只允许一个进程）。临时输出目录 `<数据目录>/tmp/doc/<任务id>/out`。任务结束删除整个 `<任务id>` 目录；启动时删除 `tmp/doc/` 下的残留。
- **超时**：单个任务（从启动组件到退出）**5 分钟**；到时结束**整个进程树**（Windows 先终结 Job Object，失败 `taskkill /T /F`；其他平台杀进程组；`soffice` 会派生 `soffice.bin`，必须一起结束），任务 `failed`，`DOC_TIMEOUT`。取消同样结束进程树，状态 `canceled`。md 转其他格式、其他格式转 md 的两步算一个任务、共用 5 分钟。
- **组件崩溃**：组件非零退出、被信号结束、正常退出但没有产出目标文件 → `DOC_COMPONENT_CRASHED`（`detail` 第一行 `exit=<退出码>`，后面是 stderr 尾部，不含路径）。组件能明确报告“打不开文件”的（stderr 含 `Error: source file could not be loaded`）→ `DOC_CORRUPT`。
- 进度：文档组件不报进度，任务 `progress` 在开始时 0、完成时 1，中间不发 `task:progress`（前端用不确定进度条）；md ↔ html、简易转换同样。

### 6.12.19 加密文件检测（后端自己做，不交给组件）

转换前（添加时、提交时、运行前各检查一次；运行前那次防止文件被替换）按文件内容判断，命中直接返回 **`DOC_ENCRYPTED`**，**不启动组件**（组件遇到加密文件会等密码而卡住）：

| 文件 | 判断方法 |
|---|---|
| docx / xlsx / pptx（OOXML） | 文件是 OLE 复合文档（头 `D0 CF 11 E0 A1 B1 1A E1`）且包含 `EncryptionInfo` 和 `EncryptedPackage` 两个流 |
| odt / ods / odp（ODF） | zip 里 `META-INF/manifest.xml` 含 `<manifest:encryption-data` |
| doc | OLE 里 `WordDocument` 流的 FIB：偏移 0x0A 的 16 位标志里 `fEncrypted`（0x0100）为 1 |
| xls | OLE 里 `Workbook`（或 `Book`）流的全局子流中，BOF 之后出现 `FILEPASS`（0x002F）记录 |
| ppt | OLE 里有 `EncryptedSummary` 流，或 `Current User` 流的 headerToken 是 `0xF3D1C4DF` |
| rtf / txt / html / md / csv | 不检测（没有加密格式） |

- OLE 解析用纯 Go 只读实现（如 `github.com/richardlehane/mscfb`），对越界做检查，损坏返回 `DOC_CORRUPT` 而不是 panic。
- docx / xlsx / pptx 扩展名但是 OLE 且**没有**加密流（旧格式改了扩展名）：不算加密，交给组件按内容识别。

### 6.12.20 错误码（v0.26 新增 10 个；2.1 由 18 个变为 28 个）

| code | message（后端给，前端也按 code 映射成同样的话） | 可重试（retryable） | 出现在 | 说明 |
|---|---|---|---|---|
| `DOC_ENCRYPTED` | `这个文件有密码保护，不能转换。请先去掉密码再添加。`；**v0.28.1**：`detail` 为 `reason=owner_only` 时 `这个 PDF 设置了权限保护，暂时不能转换。` | **否** | 添加、提交、任务 | 6.12.19；界面不给“重试”，只能移出；`owner_only` 见 6.12.66 ① |
| `DOC_CORRUPT` | `文件打不开，可能已损坏或不是有效的文档。` | **否**（只能移出） | 添加、任务 | 容器损坏、组件报告无法加载 |
| `DOC_TIMEOUT` | `文件处理太久没完成，可能已损坏，请检查后重试。` | 是 | 任务 | 5 分钟超时，已结束进程树 |
| `DOC_COMPONENT_CRASHED` | `文档组件意外退出，请重试。` | 是 | 任务 | `detail` 第一行 `exit=<码>` |
| `DOC_COMPONENT_NOT_READY` | `需要先下载文档组件。`；**Linux：`请先在系统里安装 LibreOffice，然后重启应用。`** | 是（组件就绪后） | 提交、任务开始时 | 提交了需要组件的转换而组件不是 `ready`（前端已置灰，这是后端兜底） |
| `DOC_DOWNLOAD_FAILED` | `文档组件下载失败，请检查网络后重试。` | 是 | `DocComponentStatus.error` | 所有地址都失败；保留 `.part` |
| `DOC_CHECKSUM_FAILED` | `下载的文档组件校验失败，请重试。` | 是 | `DocComponentStatus.error` | 重下一次仍不通过 |
| `DOC_COMPONENT_INSTALL_FAILED` | `文档组件准备失败，请重试。` | 是 | `DocComponentStatus.error` | msiexec / hdiutil 失败或解包后检测不通过；`detail` 第一行 `msiexec=<码>` / `hdiutil=<码>` / `check=<原因>` |
| `DOC_FORMAT_UNSUPPORTED` | `不支持这种文件。`（添加时）/ `不支持转成这个格式。`（提交时目标不在格式表里） | 否 | 添加、提交 | |
| `DOC_PDF_INPUT_UNSUPPORTED` | `PDF 暂时不能转成其他格式。` | 否 | 添加 | 与通用“格式不支持”分开；**v0.28 起不再产生**（6.12.63） |

- `detail` 第一行的 `exit=` / `msiexec=` / `hdiutil=` / `check=` 只给开发者看，前端**不解析**（不属于 2.2 的 `reason|scheme|kind`）。

- **沿用的码**：磁盘满 `CONVERT_DISK_FULL`（输出写满，或下载前空间不够 `reason=no_space`）；读写失败 `IO_ERROR`；源 / 记录不在 `NOT_FOUND`（`reason=record|file`）；超过 100 MiB `INVALID_ARGUMENT`（`reason=too_large`）；Linux 调下载 `UNSUPPORTED_PLATFORM`；未知错误 `INTERNAL`。
- **重试规则**：retryable=否 的失败记录，`TaskService.Retry` 返回 `UNSUPPORTED`（message `这个文件不能重试，请移出后重新添加。`，`detail` 第一行 `reason=not_retryable`），前端不显示重试按钮；其余按 6.6 原地重试。
- **界面永远不显示错误码和 `reason=` 等枚举**（1.1 v0.25.3）；前端按 code 映射上表文案，映射不到的显示 `出了点问题，请重试。`。`detail` 不含任何路径（组件路径、临时目录）；`message` 不含 `ffmpeg`，除 Linux 那一句外不含 `LibreOffice`。

### 6.12.21 任务类型、记录与迁移

- **新任务类型 `doc_convert`**：文档页的所有转换（组件转换、md ↔ html）都是它。`title` 形如 `报告.docx → PDF`；`inputPaths=[副本路径]`；`params={sourceId, input, target, outputDir, engine}`（`engine` = `component` | `go`，提交时按格式表决定）；`params` 里另有 `paramsSummary`（如 `Word → PDF`、`Markdown → 网页`），`presetId` / `presetName` 为 `""`。没有编码器字段。
- **`office_pdf` 只用于旧记录和简易转换**：组件未就绪时 docx / odt / txt → PDF 提交成 `office_pdf` 任务（带 `sourceId`，出现在文档页的记录里），走 6.12.1 的纯 Go 取文字路径；**v0.26 起 `office_pdf` 的输入扩展名加 `odt`、`txt`**（odt 读 `content.xml` 的段落文字；txt 按 6.12.17 解码后逐行），其余规则（5000 页、字体、折行）不变。组件未就绪时 doc / rtf → PDF 置灰（纯 Go 读不了）。
- **记录结构、事件、重试、重转全部沿用转换页**（6.14 / 6.15 / 6.17）：`Retry`（失败 / 中断 / 已取消，原地，6.6，`doc_convert` 和 `office_pdf` 都注册 Factory；retryable=否 的除外）；`ConvertService.Reconvert` 接受 `doc_convert` 和带 `sourceId` 的 `office_pdf` 记录（只用于 `succeeded`，同目标格式，不接受 `presetId` / `options`，给了返回 `INVALID_ARGUMENT` `reason=params_locked`；原地替换规则同 6.17）。**组件状态变化不改写旧记录**：简易转换的记录重转仍是简易转换，要保留排版需要新转一条。
- 输出名：`<源文件名去扩展名>.<目标扩展名>`，提交时定名并占位，重名 `a (1).pdf`（6.14.5 转换页格式）。
- **迁移 `0009_doc_convert.sql`**（顺延现有最大号 0008）：
  ```sql
  ALTER TABLE convert_sources ADD COLUMN kind TEXT NOT NULL DEFAULT 'media';   -- media（转换页）| doc（文档页）
  ALTER TABLE convert_sources ADD COLUMN sheet_count INTEGER NOT NULL DEFAULT 0; -- DocSource.sheetCount
  CREATE INDEX idx_convert_sources_kind_activity ON convert_sources(kind, last_activity_at);
  ```
  旧行都是 `media`。`tasks.type` 没有约束，`doc_convert` 不需要迁移。

### 6.12.22 未验证与开放问题

| # | 项目 | 说明 |
|---|---|---|
| 1 | Windows `msiexec /a` 不弹 UAC、解包后的目录层级、解包耗时 | 只看了 MSI 包内结构（msitools），未在 Windows 真机跑；解包后文件总量约 1.5 GiB（按 MSI File 表统计） |
| 2 | macOS dmg 挂载是否需要同意许可协议、拷出的 .app 能否直接 headless 运行（Gatekeeper） | 未在 macOS 真机验证 |
| 3 | 5 分钟超时和并发 2 是否合适 | 按大文件实测再调，属契约变更 |
| 4 | html 输入里的网络图片 | 一期 html 原样交给组件（组件可能去取网络图片）；md 的网络图片已在交给组件前去掉 |
| 5 | 读入 CSV 只认逗号 | 分号 / 制表符分隔后续再定 |

## 6.12.23 本机 Office / WPS 引擎与文档预览（v0.27；老板和产品经理定范围，架构师定方案）

> **v0.27 在 6.12.9~6.12.22（v0.26）上加两件事**：① Windows 上先用用户电脑里已有的 Microsoft Office 或 WPS 做转换，都没有才用文档组件；② 文档预览。**v0.26 的格式范围、格式表、源文件行、排队、加密检测、错误码、任务记录全部保留**，本节只写新增和改动；**与 6.12.9~6.12.22 冲突时以本节（6.12.23~6.12.36）为准**。**只有契约**；顺序是先把进行中的 v0.26 实现做完合入，再在它上面加本节，不推倒重来。

### 6.12.24 名词：引擎

“引擎”= 实际做排版转换的程序。界面上**只显示名字，不显示 id**：

| 引擎 id | 显示名（界面可以出现） | 平台 | 说明 |
|---|---|---|---|
| `office` | `Microsoft Office` | 只有 Windows | 本机安装的 Word / Excel / PowerPoint，通过 COM 调用 |
| `wps` | `WPS` | 只有 Windows | 本机安装的 WPS 文字 / 表格 / 演示，通过 COM 调用 |
| `component` | `文档组件` | 三个平台 | v0.26 的 LibreOffice（系统安装的或应用下载的，`source=system` / `downloaded`） |

另有两个**不是引擎**、只出现在记录里的取值：`go`（md ↔ html，纯 Go）和 `simple`（简易转换，纯 Go 取文字转 PDF）。

- `Microsoft Office`、`WPS` 是用户自己的软件，**名字可以出现在界面上**（设置页、记录详情、预览提示）。`LibreOffice` **仍然只能出现在 Linux 的两句提示里**（`请先在系统里安装 LibreOffice，然后重启应用。`、`系统里的 LibreOffice 版本太旧，请升级到 7.2 或更高版本，然后重启应用。`；后一句是 v0.26 讨论时产品定的，本版写进契约：Linux 上 `state=outdated` 时 `error.message` 就是它）。`TestUserFacingMessageIsChinese` 的 `LibreOffice` 白名单改为这两句。
- 界面上**不出现错误码、`reason=`、引擎 id，也不出现 ffmpeg**（1.1，v0.25.3），本节新增的所有文字同样遵守。

### 6.12.25 引擎顺序与选择

- **自动（`docEngine=auto`）的顺序**：`office` → `wps` → `component`（应用下载的文档组件）→ `component`（系统安装的 LibreOffice）。四个都没有：走 v0.26 的下载引导，或者简易转换（6.12.21）。macOS 和 Linux 只有文档组件，行为和 v0.26 一样。
  - 两个文档组件都有时**应用下载的优先**（与 v0.26 的 6.12.12 一致：版本固定、已测过）；它不可用时才用系统安装的（系统那个版本 < 7.2 或冒烟转换不通过就不用）。
- **用户在设置里选了某个引擎**（`office` / `wps` / `component`）：先用它，再按自动顺序试其余的。选中的引擎不存在、不可用或这个格式它做不了时**直接按自动顺序**，不报错、不提示（设置页照常显示用户选的值，下面一行显示实际在用的，6.12.28）。
- **按格式挑引擎**：每个转换（源格式 → 目标格式）只在**能做它的**引擎里按上面的顺序挑（能力表 6.12.27）。例如装了 Word 但没装 PowerPoint，演示文稿就走 WPS 或文档组件。
- **换下一个引擎的情况**（用户看不到，只在任务日志和 `result.engine` 里体现）：启动失败、实例隔离检查不通过（6.12.26 第 4 条）、演示程序被占用（6.12.26 第 5 条）、单次超时、打开或导出报错、产物为空、宏无法确认禁用而文件含宏（6.12.26 第 2 条）。**加密文件不换引擎**（运行前已检测，直接 `DOC_ENCRYPTED`）；`DOC_CORRUPT`（文档组件明确报告打不开）也不再换。
- **全部引擎都失败**：这个转换简易转换能做（docx / odt / txt → PDF）就**自动回退到简易转换**，任务成功，`result.engine=simple`，`result.warnings` 带 `simple_fallback`；做不了就失败，错误码规则见 6.12.33。
- **本次运行里跳过坏掉的引擎**：同一个引擎（按 Word / Excel / PowerPoint / WPS 文字 / 表格 / 演示分别算）**连续 2 次**启动失败或超时，本次运行内不再用它（`engines[].available=false`，发 `doc:component`），重启应用或 `RecheckDocComponent` 后恢复。理由：弹窗卡住（未激活提示等）每次都会卡满超时，不跳过的话每个任务都要白等。

### 6.12.26 本机 Office / WPS 的调用方式与硬规则

**实现**：纯 Go 的 COM 库 **`github.com/go-ole/go-ole`**（`go.mod` 里已有 v1.3.0，是 Wails 的间接依赖，本版改成直接依赖），**不用 cgo**；代码放 `internal/doceng/`，Office / WPS 部分只在 `//go:build windows` 下编译，其他平台的桩返回“没有这个引擎”。COM 调用在单独的 goroutine 里 `runtime.LockOSThread` + `CoInitializeEx(COINIT_APARTMENTTHREADED)`，一个实例的所有调用都在这个线程上；对方忙（`RPC_E_CALL_REJECTED` / `RPC_E_SERVERCALL_RETRYLATER`）时按 200 ms 退避重试，直到单次超时。

**检测**（启动时后台做，和文档组件检测并行；**只读注册表和文件版本，不启动 Word / WPS**）：

| 程序 | 试的 ProgID（按顺序） | 怎么确认是真的 |
|---|---|---|
| Word | `Word.Application` | `HKCR\<ProgID>\CLSID` → `HKCR\CLSID\{…}\LocalServer32` 指向的文件名是 `WINWORD.EXE` |
| Excel | `Excel.Application` | 同上，`EXCEL.EXE` |
| PowerPoint | `PowerPoint.Application` | 同上，`POWERPNT.EXE` |
| WPS 文字 | `KWPS.Application`、`WPS.Application` | 同上，`wps.exe`（**需真机验证**） |
| WPS 表格 | `KET.Application`、`ET.Application` | 同上，`et.exe`（**需真机验证**） |
| WPS 演示 | `KWPP.Application`、`WPP.Application` | 同上，`wpp.exe`（**需真机验证**） |

- **WPS 的“兼容模式”会把 `Word.Application` 等注册到 WPS 自己身上**：所以一律看 `LocalServer32` 实际指向的程序，`Word.Application` 指向 `wps.exe` 时**不算装了 Office**（也不拿这个 ProgID 去调 WPS，WPS 只用上表的 `K*` / 旧名 ProgID）。新版 WPS 是一体化程序，三个 ProgID 可能都指向同一个 `wps.exe`，也算（**需真机验证**）。
- **版本**：读 `LocalServer32` 指向的 exe 的文件版本（如 `16.0.17928.20114`、`12.1.0.18276`），写进 `engines[].version`；读不出为 `""`。**Office 主版本 < 14（2010 之前）不用**（导出 PDF 和 ODF 需要 2010 起）；Excel < 16 不用于转 CSV（UTF-8 CSV 的保存格式 2016 起才有）。WPS 一期不设最低版本。
- **检测不等于能用**：未激活、损坏的安装只有真正调用时才知道，靠换引擎和“连续 2 次跳过”兜底。

**硬规则（安全，Office 和 WPS 都适用；每一条实现 PR 都要有测试或真机记录）**：

1. **只读打开，且打开的是我们自己的临时拷贝**：引擎**从不直接打开用户的文件或转换页的副本**，先复制到任务临时目录 `<数据目录>/tmp/doc/<任务或预览 id>/in/<原文件名>`（复制不带 Windows 的 `Zone.Identifier` 数据流，也就不会触发受保护视图；用户正开着同一个文件时也不会撞上文件锁或在原目录生成 `~$` 锁文件）。打开参数：
   - Word `Documents.Open(FileName, ConfirmConversions:=False, ReadOnly:=True, AddToRecentFiles:=False, PasswordDocument:=<固定的假密码>, Visible:=False, OpenAndRepair:=False, NoEncodingDialog:=True)`；txt 输入加 `Encoding:=65001`。
   - Excel `Workbooks.Open(Filename, UpdateLinks:=0, ReadOnly:=True, Password:=<固定的假密码>, IgnoreReadOnlyRecommended:=True, Notify:=False, AddToMru:=False)`；**csv 输入**先按 6.12.17 转成 UTF-8，再用 `Workbooks.OpenText(Filename, Origin:=65001, DataType:=1 (xlDelimited), TextQualifier:=1, Comma:=True, Local:=False)` 打开（不让 Excel 按系统区域猜分隔符和编码）。
   - PowerPoint `Presentations.Open(FileName, ReadOnly:=msoTrue, Untitled:=msoFalse, WithWindow:=msoFalse)`。
   - WPS 用同名对象和参数（WPS 的对象模型照 Office；不认的可选参数去掉重试一次，**需真机验证**）。
   - “固定的假密码”：万一加密检测漏了，程序会直接报错而不是弹出输入密码的对话框（报错按 `DOC_ENCRYPTED` 处理）。
2. **强制禁用宏**：打开任何文件之前设 `Application.AutomationSecurity = 3`（`msoAutomationSecurityForceDisable`），设完**读回来确认是 3**。这个属性只对这个实例有效、不写进用户的设置。WPS 设同名属性。**设不上或读回来不是 3**：这个引擎对**含宏的文件**不可用（直接换下一个），不含宏的照常。含宏 = 扩展名是 `docm` / `dotm` / `xlsm` / `xlsb` / `pptm` / `potm`，或 OOXML 里有 `vbaProject.bin`，或 OLE 里有 `Macros` / `_VBA_PROJECT_CUR` / `VBA` 存储，或 ODF 里有 `Basic/` 目录。Excel 另设 `EnableEvents = False`。
3. **不更新外部链接、关掉所有提示、窗口不可见**：Word `DisplayAlerts = 0`（`wdAlertsNone`）、`Visible = False`、`ScreenUpdating = False`；Excel `DisplayAlerts = False`、`AskToUpdateLinks = False`、`Visible = False`、`ScreenUpdating = False`、`Interactive = False`、打开时 `UpdateLinks:=0`；PowerPoint `DisplayAlerts = 1`（`ppAlertsNone`），`Application.Visible` 不能设成 False（PowerPoint 不允许），靠 `WithWindow:=msoFalse` 不出窗口；WPS 设同名属性。**绝不修改会保存下来的选项**（Word `Options.*`、Excel `IgnoreRemoteRequests` 这类“选项”对话框里的设置、注册表）：只用上面这些只对本实例有效的属性。转换只调用另存 / 导出，**不调用**更新域、更新链接、刷新数据连接、运行宏之类的方法。
4. **绝不碰用户自己的文档、窗口和进程**（产品定的硬规则）：
   - **Word 和 Excel：每个任务（或预览）新开一个只属于我们的实例**（`CoCreateInstance`，Word / Excel 默认每次都起新进程），用完 `Close(SaveChanges:=False)` 关掉我们的文档，再 `Quit`。
   - **实例隔离检查**：创建实例时持一把全局锁，创建前后各拍一次进程快照（同名 exe），**多出来的那个 PID 就是我们的**；没有多出新进程（说明挂到了已经在运行的实例上，可能是用户的）时**立刻释放这个 COM 对象，不设任何属性、不打开文件、不调 `Quit`**，把这个程序当作这次不可用，换下一个引擎。WPS 文字 / 表格同样做这个检查（WPS 会不会每次起新进程**需真机验证**；会挂到用户的 WPS 上就一律换引擎）。
   - 我们的进程拿到句柄后放进一个 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` 的 Job Object（应用崩溃时系统会回收它；放不进去只记日志）。**v0.28 记录实现差异：暂未放进 Job Object，列为后续加固项，见 6.12.65。****超时只结束我们自己的那个 PID**（`TerminateProcess`，不用按名字的 `taskkill /IM`），绝不碰其他同名进程。
   - `Quit` 之前再看一眼 `Documents.Count` / `Workbooks.Count`：除了我们的还有别的（转换期间用户的文件被系统送进了这个实例）就**不 `Quit`、只释放对象**，进程留给用户（这种情况会不会发生**需真机验证**）。
   - **PowerPoint 和 WPS 演示：系统里只能有一个实例**。转换前先查进程：`POWERPNT.EXE`（用 Office）/ `wpp.exe`，以及一体化 WPS 的 `wps.exe`（用 WPS 演示）**只要在运行（包括用户开着的），这个引擎这次就不用**，自动换下一个；没有引擎可换，返回 `DOC_PRESENTATION_BUSY`（6.12.33），任务失败但**可以重试**，记录保留。我们自己的另一个任务（转换和预览之间）正在用 PowerPoint / WPS 演示时，**有下一个引擎就换，没有就排队等它用完**（应用内一把锁，不报 busy，因为那是我们自己的、马上会结束）。查进程和启动之间有极短的竞态，所以启动后同样做实例隔离检查。
5. **单次超时**（每次调用一个引擎算一次；见 6.12.29）。超时后：结束我们自己的进程（第 4 条），删掉临时文件，换下一个引擎；全都不行按 6.12.25 回退或失败。弹窗卡住（未激活提示、受保护视图、修复提示、字体缺失提示等）**一律靠超时兜底**，不去识别、不去点。
6. **任务中心看不到引擎切换**：只有任务日志写 `[FFmpegFree] 文档引擎：Word 超时，改用文档组件` 这类行（日志不受 1.1 限制，但同样不写文件内容）。

**导出参数**（输出先写进 `<数据目录>/tmp/doc/<id>/out/`，再按 6.12.11 的方式落盘；文件名用 ASCII 的临时名，避免对方程序对长路径 / 特殊字符的处理差异）：

| 目标 | Word / WPS 文字 | Excel / WPS 表格 | PowerPoint / WPS 演示 |
|---|---|---|---|
| pdf | `ExportAsFixedFormat(OutputFileName, ExportFormat:=17)`（`wdExportFormatPDF`；`OpenAfterExport:=False`、`IncludeDocProps:=False`） | `ExportAsFixedFormat(Type:=0, Filename, OpenAfterPublish:=False)`（`xlTypePDF`；整个工作簿，`IgnorePrintAreas:=False`） | `SaveAs(FileName, 32)`（`ppSaveAsPDF`） |
| docx / doc / odt / rtf | `SaveAs2(FileName, FileFormat:=16 / 0 / 23 / 6)` | — | — |
| txt | `SaveAs2(FileName, FileFormat:=7, Encoding:=65001)`（`wdFormatUnicodeText` + UTF-8） | — | — |
| html | `SaveAs2(FileName, FileFormat:=10)`（`wdFormatFilteredHTML`，先设 `WebOptions.Encoding = 65001`，只对这个文档有效）；只取 `.html` 文件，旁边的 `_files` 文件夹丢掉（同组件） | — | — |
| xlsx / xls / ods | — | `SaveAs(Filename, FileFormat:=51 / 56 / 60)` | — |
| csv | — | 先激活第一个工作表，`SaveAs(Filename, FileFormat:=62, Local:=False)`（`xlCSVUTF8`，逗号，只存当前表 = 第一个表） | — |
| pptx / ppt / odp | — | — | `SaveAs(FileName, 24 / 1 / 35)` |

- **输出统一**：csv、txt 去掉开头的 UTF-8 BOM（Excel / Word 会写 BOM；6.12.17 定的是不带 BOM）。
- **md**：md → 其他照旧先用 goldmark 转成临时 HTML，再把 HTML 交给 Word / WPS 文字（或文档组件）；其他 → md 先让引擎导出 html，再 html-to-markdown（6.12.11 不变）。
- 枚举值（16、17、62……）都是 Office 公开的常量，WPS 是否全部支持**需真机验证**。

### 6.12.27 各引擎能做的转换（能力表）

能力表写在 `internal/doceng/capabilities.go` 一处，格式表（6.12.30）和挑引擎（6.12.25）都读它；真机验证后**只改这张表**，不改接口。

| 引擎 | 文字类（源 → 目标） | 表格类 | 演示类 |
|---|---|---|---|
| `office`（Word / Excel / PowerPoint 分别检测，装了哪个算哪类） | 源：doc docx odt rtf txt html md；目标：pdf docx doc odt rtf txt html md | 源：xls xlsx ods csv；目标：pdf xlsx xls ods csv（csv 需要 Excel ≥ 16） | 源：ppt pptx odp；目标：pdf pptx ppt odp |
| `wps` | 源：doc docx rtf txt html md；目标：pdf docx doc rtf txt html md（**odt 不做**） | 源：xls xlsx csv；目标：pdf xlsx xls csv（**ods 不做**） | 源：ppt pptx；目标：pdf pptx ppt（**odp 不做**） |
| `component` | 6.12.10 的全部 | 全部 | 全部 |

- WPS 的 ODF 读写不可靠，一期不交给它（这些转换在只有 WPS 时走文档组件；没有组件就置灰，`disabledReason=需要文档组件`）。WPS 这一行**整行需真机验证**，验证后可以放宽。
- **v0.28**：能力表加 PDF 一列（`office` 只看 Word ≥ 15，pdf → doc docx odt rtf；`wps` 不做；`component` pdf → doc docx odt rtf txt md html），见 6.12.59。文档组件每个任务的临时配置里：宏禁用，外部链接不更新——Writer `Content/Update/Link=0`、Calc `Content/Update/Link=1`（以实现为准，6.12.65）。

### 6.12.28 组件状态的改动（`DocComponentStatus`，v0.26.1 / v0.27）

```go
type DocComponentStatus struct {
    State          string          `json:"state"`          // v0.27 改为“文档转换整体是否可用”：任何一个引擎可用就是 ready；都不可用时等于 componentState
    ComponentState string          `json:"componentState"` // v0.27 新增：文档组件（LibreOffice）自己的状态，取值同 v0.26 的 state（checking | ready | missing | outdated | downloading | preparing | failed）
    Version        string          `json:"version"`        // source 那个引擎的版本；读不出为 ""
    Source         string          `json:"source"`         // ready 时：office | wps（v0.27 新增）| system | downloaded；其他状态 ""。= 按设置和顺序（6.12.25）现在排第一个可用的引擎（不分格式），设置页“正在使用本机 WPS”就看它
    Engines        []DocEngineInfo `json:"engines"`        // v0.27 新增：本机检测到的全部引擎，按自动顺序；始终输出（没有时是 []）
    CanDownload    bool            `json:"canDownload"`    // 以下几项只说文档组件，含义不变
    DownloadBytes  int64           `json:"downloadBytes"`
    InstallBytes   int64           `json:"installBytes"`   // v0.26.1（后端实现 PR 一并给出）：装好后的大约占用，固定 1.5 GiB = 1610612736；没有下载源（Linux）为 0
    Phase          string          `json:"phase,omitempty"`         // 文档组件在 downloading / preparing 时有
    ReceivedBytes  int64           `json:"receivedBytes,omitempty"`
    Error          *AppError       `json:"error,omitempty"`         // 文档组件的错误（componentState 为 missing（Linux）/ outdated / failed 时）
    Path           string          `json:"-"`
}

type DocEngineInfo struct {
    ID        string   `json:"id"`        // office | wps | component
    Name      string   `json:"name"`      // Microsoft Office | WPS | 文档组件（后端给，前端直接显示）
    Version   string   `json:"version"`   // 读不出为 ""；office 取 Word 的版本（没有 Word 时取 Excel、PowerPoint）
    Source    string   `json:"source,omitempty"` // 只有已安装的 component 有：downloaded | system（两个都有时是正在用的那个，即 downloaded）
    Installed bool     `json:"installed"` // office / wps 恒为 true；component：有可用的（应用下载的或系统安装的）为 true，还没下载为 false
    Families  []string `json:"families"`  // 这个引擎能处理的类别：text | sheet | slide | pdf（office 按实际装了 Word / Excel / PowerPoint 给出；v0.28：Word ≥ 15 时 office 带 pdf，已装的 component 带 pdf，wps 不带）
    Available bool     `json:"available"` // 现在能用：installed 且没有被本次运行跳过（6.12.25 连续 2 次失败）、版本不过低；仍列出，设置页可以显示为不可用
}
```

- **`engines` 里始终有 `component` 这一项**（Windows、macOS），还没下载时 `installed=false`、`available=false`、`version=""`、没有 `source`；**Linux 没有下载源，组件没装时不列这一项**（装了系统 LibreOffice 才有）。
- **设置页下拉框**（产品定）：`自动（推荐）`，再加 `engines` 里的每一项；`installed=false` 的 component 显示为 `文档组件（未下载）`，**也可以选**。选中它时**前端先弹确认框**（下载大小、安装后占用取 `downloadBytes` / `installBytes`），用户确认后前端调 `InstallDocComponent`，并 `UpdateSettings` 保存 `docEngine=component`；**后端不会因为设置成 `component` 而自动下载**。下载完成之前按 6.12.25 退回自动顺序。

- **`installBytes`**：如果后端的 v0.26 实现 PR（契约改到 v0.26.1）合入时已经带了这个字段，以它为准；没带就按这里补上。含义：文档组件装好后的大约占用，固定 `1.5 GiB`（1 610 612 736 字节），Linux 为 0；下载引导卡片上的“安装后约占用 1.5GB 磁盘空间”取它。
- **就绪的判断**：Windows 上检测到 Office 或 WPS 中任何一个（且至少有一类能用）就是 `state=ready`，**不再出下载引导**；`componentState` 照样反映文档组件自己（可能是 `missing`）。设置页的“文档组件”一块（下载、进度、重试）看 `componentState` / `phase` / `receivedBytes`；文档页顶部的下载引导只看 `state`。Office / WPS 用户在设置页仍可以下载文档组件（用来补 WPS 不做的 ODF 格式），`InstallDocComponent` 照常可用。
- `doc:component` 事件在 `state`、`componentState`、`source`、`engines` 任何一个变化时都发（含设置里改了 `docEngine`）。**前端收到 `doc:component` 一律重新拉 `GetFormatMatrix`**（取代 v0.26 “只在 ready 变化时拉”，表很小）。
- `checking`：启动时 Office / WPS 和文档组件的检测都没完成前 `state=checking`；Office / WPS 检测只读注册表，通常几十毫秒，先完成就可以先变 `ready`（文档组件还在检测，`componentState=checking`）。**v0.28.3**：`GetDocComponentStatus` 检测中不等，立即返回；检测结束至少发一次 `doc:component`（payload 与 `GetDocComponentStatus` 相同）；`GetFormatMatrix` 也不再等（6.12.68）。
- 设置页（产品定）：`文档组件已就绪`，下面一行小字 `正在使用本机 WPS`（按 `source`：`office` → `Microsoft Office`，`wps` → `WPS`，`system` / `downloaded` → `文档组件`）；`engines` 多于一项时显示下拉框（规则见上）。

### 6.12.29 设置、单次超时与并发

- **`Settings.docEngine`**（string，settings 表键 `docEngine`）：`auto` | `office` | `wps` | `component`，**默认 `auto`**；空值按 `auto`；其他值 `UpdateSettings` 返回 `INVALID_ARGUMENT`（message `文档引擎设置不正确`，整体不生效，同 v0.7.6 的原子规则）。选了当前不存在的引擎**允许保存**（例如换了电脑），运行时按 6.12.25 退回自动顺序，不报错。macOS / Linux 也接受这四个值（只是 `office` / `wps` 永远不可用）。
- **v0.28 记录实现差异（以实现为准）**：没有单开池，是 `OfficeConverter` 的一把全局锁保证并发 1，能用 Office / WPS 的任务不进组件池、直接开始并在锁上等；语义等价，细节见 6.12.65。
- **Office / WPS 单独一个池（架构师定）**：池名“本机办公软件池”，**并发 1**（Word、Excel、PowerPoint、WPS 合在一起一次只跑一个转换任务）。**不占**文档组件那 2 个名额。理由：① Office / WPS 是完整的桌面程序，同时开两个自动化实例内存和稳定性都差，PowerPoint / WPS 演示本来就只能有一个；② 文档组件的 2 个名额是给 soffice 进程算的，Office 任务占着它没有意义，反而让需要组件的格式（如 WPS 用户的 ODF）白白排队；③ Office 失败换到文档组件时直接进组件池，不用和自己抢名额。所以文档页同时最多跑 **3 个转换**（1 个 Office / WPS + 2 个文档组件），md ↔ html 和简易转换仍不进池（6.12.18）。
- **排队**：文档页仍是**一条 FIFO 队列**，每个排队任务在派发时按 6.12.25 挑好要用的引擎，等那个引擎的池有空位；调度器按顺序找**第一个**它要的池有空位的任务启动（前面的任务等 Office 时，后面要用组件的任务可以先跑）。**不因为 Office 池忙就改用组件**：引擎按“能不能用”挑，不按“忙不忙”挑，同一种转换效果才稳定。`queuePosition` 仍是“文档队列里排在它前面的排队任务数”，`doc:queue` 不变。任务从 Office 换到组件时**插到组件池的最前面**（它已经等过一次了）。
- **转换的单次超时（架构师定）**：Office / WPS **每次 3 分钟**，文档组件每次 **5 分钟**（v0.26 不变），**整个任务（含换引擎）最多 10 分钟**，到了就停在当前引擎、按 6.12.33 失败。理由：弹窗卡住的那次永远不会自己结束，等满 5 分钟再换引擎太久；Office 转 100 MiB 以内的文件正常远少于 3 分钟（**需真机验证**，不够再调，属契约变更）。
- 预览的超时见 6.12.32 第 3 条。

### 6.12.30 格式表（`GetFormatMatrix`）的改动

- **`available`**：有任何一个**现在可用**的引擎能做这个转换（能力表 6.12.27）就是 `true`；`componentReady` 改名不改：表示 `state=ready`（任何引擎可用），字段名保留。
- **`needsComponent`** 含义不变：这个转换要不要排版引擎（Office / WPS / 文档组件中的一个）；md ↔ html 为 `false`。
- **`simple`**：只有**没有任何引擎**能做、而简易转换能做（docx / odt / txt → pdf）时为 `true`（同 v0.26）。有 Office 时 docx → pdf 的 `simple=false`。
- **`disabledReason`**：仍是 `需要文档组件`（WPS 不做 ODF、只有 WPS 时 odt 相关的项也是这句，因为装上文档组件就能做）。
- **新增 `DocTarget.engines []string`**（`omitempty`）：现在能做这个转换的引擎 id，按实际使用顺序（已考虑 `docEngine`）。只供前端排查 / 调试，界面不显示。
- md 的转换照旧走 goldmark → html → 引擎（6.12.11）。

### 6.12.31 记录：实际是哪个引擎转的

- **`TaskResult` 新增 `engine string \`json:"engine,omitempty"\``**：成功的 `doc_convert` / `office_pdf` 任务里写实际完成转换的那一个：`office` | `wps` | `component` | `go` | `simple`。落在已有的 `tasks.result`（JSON）里，**不需要迁移**。v0.27 之前的旧记录没有这个键。
- `task:status` 的 `succeeded` 事件带的 `result` 里同样有 `engine`。
- 前端在记录详情显示一行（产品定）：`office` → `由本机 Microsoft Office 转换`，`wps` → `由本机 WPS 转换`，`component` → `由文档组件转换`；`go`、`simple` 和没有 `engine` 的旧记录**不显示这一行**（简易转换另有它自己的标识）。
- **回退到简易转换**（6.12.25）：`result.engine=simple`，`result.warnings` 带新机器码 **`simple_fallback`**（6.14.6 的 warnings 机制）。前端文案（v0.27.1 补充，产品已定）：`这次是简易转换，只保留了文字。可以稍后重转。`。
- **`params.engine`**（v0.26：`component` | `go`）保留原值，`component` 的含义放宽为“要用排版引擎，运行时再挑”（不改名，旧记录不用改）。**不在 `params` 里锁定具体引擎**：`Retry` 和 `Reconvert` 每次都按**当时的**设置和检测结果重新挑，`result.engine` 随之更新。
- **迁移**：v0.27 **没有迁移**（`engine` 在 JSON 里，设置在 settings 键值表里，预览缓存不进库，6.12.32）。现有最大号是 `0009`（v0.26，`0009_doc_convert.sql`）；以后要加从 `0010` 起。

### 6.12.32 文档预览

#### 6.12.32.1 接口（DocService，v0.27 新增）

```go
GetDocPreview(req DocPreviewRequest) (DocPreview, error) // 开始（或从缓存直接拿）一个预览
CancelDocPreview(previewID string) error                 // 弹窗关掉时调：生成中就停止生成；已就绪就撤销本地地址。不存在的 id 忽略（幂等）

type DocPreviewRequest struct {
    SourceID string `json:"sourceId,omitempty"` // 文档页的源文件行（预览原文件）
    TaskID   string `json:"taskId,omitempty"`   // doc_convert / office_pdf 记录（预览转换结果）；二者必须且只能给一个
}

type DocPreview struct {
    PreviewID  string     `json:"previewId"`            // 随机 128 位十六进制，进程内有效
    Kind       string     `json:"kind"`                 // pdf | text | csv | html | md | raw | unavailable
    State      string     `json:"state"`                // generating | ready | failed（只有 kind=pdf 会经过 generating；其余同步就是 ready 或 failed）
    Name       string     `json:"name"`                 // 显示用的文件名（原文件名或输出文件名）
    Ext        string     `json:"ext"`                  // 被预览文件的扩展名（不带点、小写）；raw 时前端按它选 docx-preview 或 SheetJS
    URL        string     `json:"url,omitempty"`        // kind=pdf / raw 且 ready：本地预览地址 /local/<token>（6.13 的白名单，只放行这一个文件）
    Text       string     `json:"text,omitempty"`       // kind=text / md / html：UTF-8 原文（后端不渲染、不过滤）
    Rows       [][]string `json:"rows,omitempty"`       // kind=csv：前 1000 行（含第一行），每行是单元格数组
    TotalRows  int64      `json:"totalRows,omitempty"`  // kind=csv：文件的总行数（截断时可能是估计，见 6.12.32.4）
    Truncated  bool       `json:"truncated,omitempty"`  // text / md / html：超过 2 MiB 被截断；csv：读到 2 MiB 上限前没数完行（totalRows 是下限）
    SizeBytes  int64      `json:"sizeBytes"`            // 被预览文件的大小（字节）
    Reason     string     `json:"reason,omitempty"`     // kind=unavailable 时：needs_component | too_large_for_simple（只给程序用，界面不显示）
    Error      *AppError  `json:"error,omitempty"`      // state=failed 时有
}
```

- **事件 `doc:preview`**：`{ previewId, state, kind, url?, error? }`，只用于要引擎生成 PDF 的预览：开始排队 / 生成时发一次 `generating`，结束时发一次 `ready`（带 `url`）或 `failed`（带 `error`）；**取消后不再发**。缓存命中、结果本身是 PDF、文字类、csv、raw、unavailable 和同步就能判断的失败，`GetDocPreview` 直接返回最终状态，不发事件。事件可能比 `GetDocPreview` 先返回，前端按 `previewId` 暂存（同 v0.25.1 的事件顺序规则）。
- **同步返回的 Go 错误只有调用本身的问题**：两个 id 都给或都不给 `INVALID_ARGUMENT`；id 不存在 `NOT_FOUND`（`reason=record`）；不是文档页的行 / 不是文档任务 `UNSUPPORTED`（`reason=format`）；记录不是 `succeeded`（没有输出）或输出文件不在 `NOT_FOUND`（`reason=file`）。**文件本身的问题**（加密、损坏、生成失败、演示程序被占用）放在 `state=failed` + `error` 里返回。
- **原文件行用哪个文件**：同 6.15.6 的“显示路径”：副本已就绪用副本，复制中 / 失败用原文件（不用等复制）。**结果行**用记录的 `outputPath`。
- **本地地址（`url`）**：登记进 6.13 的 `/local/<token>` 白名单（和转换页 `GetSourcePreviewURL` 同一套 `localassets`：随机 token、登记时和每次请求都 `EvalSymlinks` + 普通文件 + `SameFile` 校验、支持 `HEAD`、单段 Range），**一个 token 只放行这一个文件**；`pdf → application/pdf`，`docx` / `xlsx` 用各自的 Office MIME。`CancelDocPreview` 或预览被释放时 token 作废，之后 `404`；前端遇到 `404` 重新 `GetDocPreview` 一次。**限长沿用 6.13**（每个 Range 响应 ≤ 4 MiB，无 Range 的整体请求 ≤ 32 MiB，更大 413）：pdf.js 本来就按 Range 加载；**raw 文件前端必须按 Range 分段取**（每段 ≤ 4 MiB，拼成整份再交给前端库），不能一次 `fetch` 整份。不再用 base64 分块接口。
- 结果本身是 PDF 的直接给 `url`，**不写 `doc_recent`**，不进 PDF 历史。
- 一次最多保留 **8 个**未释放的预览（超过时释放最早的，token 作废）。
- **v0.27.1**：`DocPreview` 另有 `editable`、`editBlock`、`revision`、`encoding`、`lineEnding`，源文件行的文字类改为读原文件，见 6.12.38~6.12.39。

#### 6.12.32.2 各种文件怎么预览

| 文件（源文件行或结果行的扩展名） | 有能做的引擎时 | 没有任何引擎时 |
|---|---|---|
| pdf（只会是结果） | `pdf`，直接给 `url` | 同左 |
| doc docx odt rtf / xls xlsx ods / ppt pptx odp | `pdf`：用引擎在后台生成一份临时 PDF（6.12.32.3），给 `url` | docx / xlsx → `raw`（≤ 50 MiB；更大 `unavailable` + `reason=too_large_for_simple`）；其余 `unavailable` + `reason=needs_component` |
| txt | `text` | 同左 |
| md / markdown | `md`（原文） | 同左 |
| html / htm | `html`（原文） | 同左 |
| csv | `csv`（解析好的前 1000 行） | 同左 |

- 挑引擎同 6.12.25（含设置、能力表、换引擎、跳过坏引擎），看“该源格式 → pdf”这一项。生成**全部失败时不回退到简易转换**（预览要看排版）：docx / xlsx 退到 `raw`（同样受 50 MiB 限制），其余 `state=failed`。
- **加密**：引擎生成前按 6.12.19 检测，命中直接 `state=failed`、`DOC_ENCRYPTED`（不交给任何引擎）；`raw` 也先检测，加密的 docx / xlsx 同样 `failed` + `DOC_ENCRYPTED`（前端库打不开加密文件）。txt / md / html / csv 不检测。
- 前端的缩放、翻页、工作表标签等显示细节**不归契约管**。

#### 6.12.32.3 用引擎生成 PDF：缓存、排队、超时

- **缓存目录**：Windows `%LocalAppData%\FFmpegFree\cache\preview\`（同组件目录，不放漫游的 `%AppData%`），macOS / Linux `<应用数据目录>/cache/preview/`。文件名 `<key>.pdf`，旁边一个 `index.json` 记每项的键、大小、最后打开时间。
- **缓存键**：`sha256(规范化路径（path_key）+ "\n" + 大小 + "\n" + 修改时间（纳秒）+ "\n" + 引擎 id)` 的十六进制。文件改了（大小或时间变）自然不命中；换了引擎也重新生成。查缓存时按**这次会用的引擎顺序**依次找，第一个命中的就用（例如上次是 Office 生成的、这次 PowerPoint 被占用，仍可以直接用 Office 那份）。
- **上限 1 GiB**（总大小），超了按“最后打开时间”从旧到新删；**正在被预览的项（token 没作废）不删**，全都在用时允许暂时超过。命中时更新“最后打开时间”。启动时删掉 `*.part`、删掉 `index.json` 里没有的文件和文件不在的项，再按上限清一次。后端不提供“清空缓存”接口（以后需要再加）。
- **单独排队，并发 1**：所有要引擎生成的预览排一条队，**一次只生成一个**；不占转换的任何名额（Office 池、组件池都不占），不进任务中心，不产生任务记录。用户关掉弹窗 → 前端调 `CancelDocPreview` → 还在排队就移出，正在生成就结束我们自己的进程（同 6.12.26 第 4 条）、删临时文件。
  - 预览用的 Office / WPS 实例和转换用的**互不共用**（Word / Excel 各开各的进程；PowerPoint / WPS 演示同一时间只能一个，按 6.12.26 第 4 条的应用内锁：被我们自己的转换占着时有别的引擎就换，没有就等）。文档组件的预览进程用自己的临时配置目录 `<数据目录>/tmp/doc/preview-<previewId>/profile`。所以最坏情况下同时有：1 个 Office / WPS 转换 + 2 个组件转换 + 1 个预览。
- **预览的超时（架构师定，调整了“2 分钟”的建议）**：Office / WPS **每次 90 秒**，文档组件每次 **2 分钟**，**整个预览最多 3 分钟**（含换引擎）。理由：用户开着弹窗在等，卡在 Office 弹窗上的那次要尽快换到下一个引擎；一个引擎 2 分钟、总共 3 分钟后基本可以认为用户已经不想等了，转换功能不受影响。
- **生成过程**：引擎把 PDF 写到 `<key>.pdf.part`，完成后改名为 `<key>.pdf`、写 `index.json`，再登记 `/local/<token>`，`state=ready`、`kind=pdf`、`url`。前端把 `url` 交给现有的 PDF 预览（pdf.js 按 Range 加载）。
- 生成的 PDF 大于 512 MiB（`MaxPDFBytes`）按失败处理（`state=failed`，`INTERNAL`）。

#### 6.12.32.4 text、md、html、csv

- **编码**：都按 6.12.17 的规则（合法 UTF-8 含 BOM → UTF-8 并去掉 BOM，否则 GB18030）转成 UTF-8；html 有 `<meta charset>` 时先按它。
- **text / md / html：后端只给 UTF-8 原文，不渲染、不过滤**（`text` 字段）。上限 **2 MiB**（转成 UTF-8 之后的字节数），超过就截在 2 MiB 内最后一个完整的 UTF-8 字符处，`truncated=true`。
- **csv**：后端读最多 2 MiB（UTF-8 之后），按 RFC 4180（逗号、双引号、引号里可以有换行）解析，`rows` 给**前 1000 行**（含第一行，不区分表头），每个单元格最多 32 767 个字符（超出截断）；`totalRows` = 总行数：2 MiB 内读完了就是准确值，`truncated=false`；没读完时继续只数行（不解析单元格内容，最多读完整个文件，≤ 100 MiB）得到准确值，数不完（超时 5 秒）时给已数到的行数并 `truncated=true`。前端在 `totalRows > 1000` 时底部写 `只显示前 1000 行，共 n 行。`（n = `totalRows`）。解析出错的行按原样放进一个单元格，不报错。
- **前端显示 md 和 html 的硬规则**（md 由前端渲染成 HTML 后同样适用）：
  1. 先用 DOMPurify 过滤：去掉 `script`、`iframe`、`frame`、`object`、`embed`、`applet`、`form`、`input`、`button`、`link`、`meta`、`base`、所有 `on*` 属性、`javascript:` / `vbscript:` / `data:`（`data:image/*` 除外）地址；`<a>` 的 `href` 去掉（只留文字）；`img` 只保留 `data:image/*` 的 `src`，其余图片换成 alt 文字；`style` 里的 `url(`、`@import`、`expression(` 去掉。md 渲染时**不允许原始 HTML 直接通过**（交给 DOMPurify 之前也要转义或丢弃）。
  2. 放进 `<iframe sandbox srcdoc="…">`，`sandbox` 属性**为空**（不加 `allow-scripts`，也不加 `allow-same-origin`、`allow-top-navigation`、`allow-popups`、`allow-forms`）。
  3. `srcdoc` 开头注入严格的 CSP：`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; form-action 'none'; base-uri 'none'">`，**不加载任何网络或本地资源**。
- **text**：用 `<pre>`（或等价的纯文本组件）显示，不当 HTML 解析。截断时前端在顶部提示一行 `文件太长，只显示了前面一部分。`（v0.27.1 补充，产品已定；md / html / csv 截断时同样用这句）。

#### 6.12.32.5 没有任何引擎时：docx、xlsx 的简易预览（`raw`）

- 条件：没有任何能做“该格式 → pdf”的可用引擎，或者有引擎但这次生成全部失败。只对 **docx 和 xlsx**，文件 **≤ 50 MiB**：返回 `kind=raw`、`state=ready`、`url`（原文件的本地地址，走 6.13 白名单，只放行这一个文件；**不用 base64**）、`ext`、`sizeBytes`。前端按 Range 分段取回（6.12.32.1），交给 `docx-preview` / SheetJS（**按需加载**，打开预览时才加载）。简易预览顶部固定一行 `简易预览，排版可能和原文件不一样。`。
- **> 50 MiB**：`kind=unavailable`、`reason=too_large_for_simple`、`state=ready`；前端显示 `文件太大，没法简易预览。下载文档组件后可以完整预览。`，带「下载文档组件」。
- **其他格式**：`kind=unavailable`、`reason=needs_component`、`state=ready`（结果就是“不能预览”，不是失败）；前端显示 `需要文档组件才能预览。`，带「下载文档组件」。Linux（`canDownload=false`）不给按钮，显示 v0.26 的 Linux 提示。
- `reason` 只给程序用，**界面不显示**。

#### 6.12.32.6 预览的文案（产品定）

| 情况 | 怎么判断 | 文案 |
|---|---|---|
| 生成中 | `state=generating` | `正在生成预览…`（关掉弹窗就取消） |
| 简易预览 | `kind=raw` | 顶部一行 `简易预览，排版可能和原文件不一样。` |
| csv 超过 1000 行 | `kind=csv` 且 `totalRows > 1000` | 底部 `只显示前 1000 行，共 n 行。` |
| 没有引擎 | `kind=unavailable` + `reason=needs_component` | `需要文档组件才能预览。` + 「下载文档组件」 |
| 太大不能简易预览 | `kind=unavailable` + `reason=too_large_for_simple` | `文件太大，没法简易预览。下载文档组件后可以完整预览。` + 「下载文档组件」 |
| 有密码 | `failed` + `DOC_ENCRYPTED` | `这个文件有密码保护，不能预览。`（不给重试） |
| 演示文稿被占用 | `failed` + `DOC_PRESENTATION_BUSY` | `请先关闭正在打开的演示文稿，再预览。` + 「重试」（重新 `GetDocPreview`） |
| 文档程序被占用 | `failed` + `DOC_ENGINE_BUSY` | `请先关闭正在打开的文档，再预览。` + 「重试」 |
| 其他所有失败 | `failed` + 其他任何码 | `这个文件暂时无法预览。` |

- 预览**没有专门的“生成失败”错误码**：后端在 `error` 里给实际原因（`DOC_TIMEOUT`、`DOC_COMPONENT_CRASHED`、`DOC_CORRUPT`、`INTERNAL`……），前端除上表点名的码以外一律显示 `这个文件暂时无法预览。`；**前端不用后端的 `message` 显示预览错误**。
- 前端未知的 `kind` / `reason` 一律按 `这个文件暂时无法预览。` 处理。

### 6.12.33 错误码（v0.27 新增 2 个；2.1 由 28 个变为 30 个）

| code | message（后端给；前端按 code 映射） | 可重试 | 出现在 | 说明 |
|---|---|---|---|---|
| `DOC_PRESENTATION_BUSY` | 转换：`请先关闭正在打开的演示文稿，再转换。`；预览：前端用 `请先关闭正在打开的演示文稿，再预览。` | **是**（任务保留，原地 `Retry`） | 任务、预览 | 演示类转换 / 预览只剩 PowerPoint 或 WPS 演示可用，而它正在运行（包括用户自己开着的），6.12.26 第 4 条 |
| `DOC_ENGINE_BUSY` | 转换：`请先关闭正在打开的文档，再转换。`；预览：前端用 `请先关闭正在打开的文档，再预览。` | **是** | 任务、预览 | **架构师定**：文字 / 表格类只剩 Word、Excel 或 WPS 可用，而实例隔离检查不通过（会挂到正在运行的实例上，6.12.26 第 4 条；主要是 WPS，**需真机验证后才知道会不会出现**）。没有新增这个码时只能借用演示文稿那句，意思不对；**文案产品已定**（v0.27.1 补充） |

- `detail` 第一行 `engine=<office|wps>`，只给开发者看，前端**不解析**（不属于 2.2 的 `reason|scheme|kind`）。
- **全部引擎都失败时报哪个码**：先看能不能回退简易转换（6.12.25，成功就不报错）；不能时：所有引擎都是“被占用”→ 有演示类的 busy 就 `DOC_PRESENTATION_BUSY`，否则 `DOC_ENGINE_BUSY`；否则报**最后一个试过的引擎**的错（超时 `DOC_TIMEOUT`，崩溃 / 报错 / 没有产物 `DOC_COMPONENT_CRASHED`，组件明确说打不开 `DOC_CORRUPT`）；一个可用的引擎都没有 `DOC_COMPONENT_NOT_READY`。`DOC_COMPONENT_CRASHED` 的文案 `文档组件意外退出，请重试。` 在 Office / WPS 出错时也用这句（界面不区分是哪个程序，`detail` 第一行 `engine=` 区分）。整个任务 10 分钟到了是 `DOC_TIMEOUT`。
- 其余沿用 6.12.20，可重试规则不变（`DOC_ENCRYPTED`、`DOC_CORRUPT` 不可重试）。
- **界面不显示错误码、`reason=`、`engine=`，不出现 ffmpeg**；Office / WPS 的名字可以出现，LibreOffice 只在 Linux 那两句里（6.12.24）。

### 6.12.34 接口、事件、字段汇总（v0.27）

- **新方法**（DocService）：`GetDocPreview`、`CancelDocPreview`。
- **新事件**：`doc:preview`。`doc:component` 的触发条件变宽（6.12.28）。
- **新 / 改字段**：`DocComponentStatus.componentState`、`engines`（`DocEngineInfo`，含未下载的 component 项 `installed=false`）、`source` 新增 `office` / `wps`、`state` 含义变为“整体可用”、`installBytes`（v0.26.1）；`Settings.docEngine`；`TaskResult.engine`；`TaskResult.warnings` 新增 `simple_fallback`；`DocTarget.engines`；新类型 `DocPreviewRequest`、`DocPreview`、`DocEngineInfo`。
- **新错误码**：`DOC_PRESENTATION_BUSY`、`DOC_ENGINE_BUSY`。
- **新依赖**：`github.com/go-ole/go-ole`（由间接改为直接，仅 Windows 代码用）；前端 `docx-preview`、`xlsx`（SheetJS）、md 渲染库、`dompurify`（按需加载）。
- **没有迁移**；设置键 `docEngine` 写进 settings 表（键值表，不用迁移）。

### 6.12.35 实现顺序与测试

1. 先合入 v0.26 的后端、前端实现（不等本节）。
2. 后端：`internal/doceng` 引擎接口（`Detect`、`Capabilities`、`Convert(ctx, in, out, target)`），把 v0.26 的 soffice 调用包成 `component` 引擎；加 Office / WPS（`windows` build tag）；调度器加 Office 池；预览服务和缓存。**Linux / macOS 上能测的都要有单测**：能力表、挑引擎顺序（用假引擎模拟超时、busy、隔离检查不通过、全部失败回退简易转换）、缓存键和 LRU、2 MiB 截断（UTF-8 边界）、csv 解析（引号内换行、1000 行、`totalRows`）、raw 的 50 MiB 边界和 `reason`、`/local/<token>` 只放行这一个文件且取消后 404。Windows 代码至少 `GOOS=windows go build`、`go vet` 通过。
3. 前端：设置页下拉框、记录详情的“由本机 … 转换”、预览弹窗各状态；md / html 的 DOMPurify 过滤要有单测（含 `<script>`、`onerror=`、`javascript:`、`<a href>`、`<img src=http>`、`<meta http-equiv=refresh>`、`<base>`、`style` 里的 `url(`），sandbox iframe 的快照里 `sandbox=""`、CSP 原文；raw 按 Range 分段取。`check:copy` 增加：`Microsoft Office`、`WPS` 允许出现；`LibreOffice` 仍只放行 Linux 那两句。

### 6.12.36 未验证事项（要在 Windows 真机上验证）

箱子是 Linux，以下**全部没有在 Windows 上跑过**。不阻塞实现：按契约写，试用包出来后由老板逐项确认；不通过就改能力表、超时或本契约（属契约变更）。

| # | 项目 | 在哪里 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|---|
| 1 | Office 各版本（2010 / 2013 / 2016 / 2019 / 2021 / 365 即点即用）的 ProgID 和 `LocalServer32` 指向；WPS 各版本（个人版 2019、新版一体化、教育版 / 企业版）实际注册了哪些 ProgID（`KWPS` / `KET` / `KWPP` / `WPS` / `ET` / `WPP`），一体化版本是否都指向 `wps.exe`；WPS 兼容模式开启时 `Word.Application` 是否被改指向 WPS | 6.12.26 检测 | 各装一台，导出注册表对应键，再实际 `CoCreateInstance` | 补 ProgID 列表 / 改判断规则 |
| 2 | 只读打开是否生效（转换后原文件修改时间不变、没有 `~$` 锁文件）；`AutomationSecurity=3` 是否生效（用一个 `Document_Open` / `Workbook_Open` / `Auto_Open` 会写文件的宏文档测试，宏**不能**执行）；WPS 是否支持 `AutomationSecurity` 并能读回 3 | 6.12.26 第 1~2 条 | 准备带宏的 docm / xlsm / pptm 和 doc / xls（老格式宏） | 读不回 3 的引擎不收含宏文件（已写进规则）；Office 本身不生效则含宏文件一律不交给 Office / WPS |
| 3 | 不更新外部链接、`DisplayAlerts` 关闭后不弹窗；未激活的 Office、首次运行向导、受保护视图、修复提示、字体替换提示是否卡住、超时后能否换引擎；临时拷贝确实没有 `Zone.Identifier` | 6.12.26 第 3、5 条 | 带外部链接的 xlsx、从网上下载的文件、未激活的 Office 虚拟机 | 调整打开参数或超时 |
| 4 | **PPT 被占用的判断**：用户开着 PowerPoint / WPS 演示时确实换引擎或返回 `DOC_PRESENTATION_BUSY`，**用户的演示文稿不受任何影响**（不关、不隐藏、不丢未保存内容）；一体化 WPS 下只开了 WPS 文字时演示是否也算被占用 | 6.12.26 第 4 条 | 开着一个未保存的演示文稿，提交 pptx → pdf | 收紧判断（宁可多报 busy） |
| 5 | **实例隔离**：Word / Excel 的 `CoCreateInstance` 每次都起新进程；WPS 文字 / 表格是否也起新进程（决定 `DOC_ENGINE_BUSY` 会不会出现）；转换期间用户双击打开 Word / Excel 文件，会不会进到我们的自动化实例里 | 6.12.26 第 4 条 | 进程快照对比；转换时双击打开文件 | 改成“这个程序在运行就不用它”（同 PowerPoint 规则） |
| 6 | **超时后进程能否清理干净**：结束我们的 PID 后没有残留的 `WINWORD.EXE` / `EXCEL.EXE` / `POWERPNT.EXE` / `wps.exe` / `et.exe` / `wpp.exe`；应用被强制结束时 Job Object 能否带走它们；COM 调用在进程被结束后能否立刻返回（不挂住我们的线程） | 6.12.26 第 4~5 条 | 用一个会弹窗的文件故意卡住，等超时；任务管理器里强制结束应用 | 补 `taskkill /PID <我们的 PID> /T /F` 兜底 |
| 7 | 导出参数（6.12.26 表里的常量）在 WPS 上是否都认；Excel `xlCSVUTF8`（62）在 2016 早期版本上是否可用；Word 导出 html 的编码 | 6.12.26、6.12.27 | 每个格式转一遍，比对输出 | 改能力表 |
| 8 | Office 转换 3 分钟、预览 90 秒的单次超时是否够用（大 pptx 导出 PDF） | 6.12.29、6.12.32.3 | 100 MiB 左右的 pptx / xlsx | 调超时 |
| 9 | 预览的 sandbox iframe + `srcdoc` 里的 CSP meta 在 WebView2 和 WKWebView 上确实生效（脚本不执行、不发任何网络请求） | 6.12.32.4 | 用带 `<script>`、`<img src=http…>` 的 html / md 预览，抓包 / 看控制台 | 改成只显示纯文本，或更严格的过滤 |
| 10 | raw（≤ 50 MiB 的 docx / xlsx）经 `/local/<token>` 按 Range 分段取回在 Windows 上正常（同 6.12.8 第 2 项的 Range 行为） | 6.12.32.1、6.12.32.5 | 预览一个 40 MiB 左右的 xlsx | 把 raw 上限降到 32 MiB 以内，整体请求一次取回 |


## 6.12.37 文档预览里的编辑（v0.27.1；老板定范围，架构师定方案，文案产品已定（v0.27.1 补充））

> 在 6.12.32（文档预览）上加编辑和保存。**只有 md、txt、csv、html 能编辑**：md 左边写、右边实时预览；csv 在表格里改；txt 和 html 改源码。Word、Excel、PPT、PDF 不能编辑。改完可以「保存」或「另存为」。与 6.12.23~6.12.36 冲突时以本节为准。**没有新增错误码**（2.1 仍是 30 个），只在 2.2 加取值；**没有迁移**。

### 6.12.38 `DocPreview` 新增的字段

```go
type DocPreview struct {
    // ……6.12.32.1 的字段不变……
    Editable   bool   `json:"editable"`             // 始终输出
    EditBlock  string `json:"editBlock,omitempty"`  // editable=false 时的原因（只给程序用，界面不显示）：format | too_large | encoding | malformed | converting | missing（v0.27.1 补充：in_use 改名为 converting）| macro（v0.27.2 补充：带宏的 docx，6.12.48）
    Revision   string `json:"revision,omitempty"`   // kind=text / md / html / csv 时有：读到的那份原始字节的 SHA-256（小写十六进制），前端不解析，保存时原样带回
    Encoding   string `json:"encoding,omitempty"`   // kind=text / md / html / csv 时有：utf8 | utf8_bom | gbk（读取时识别出的编码，6.12.40；v0.27.1 补充：原来的 gb18030 改名为 gbk，因为写回按 GBK）
    LineEnding string `json:"lineEnding,omitempty"` // 同上：crlf | lf（读取时多数行的风格，规则见 6.12.40）
}
```

**`editBlock` 的判断顺序**（命中第一个就停）：

| 取值 | 什么时候 |
|---|---|
| `format` | `kind` 是 `pdf` / `raw` / `unavailable`（Word、Excel、PPT、PDF 一律不能编辑） |
| `missing` | 源文件行的**原文件已经不在了**（这时预览读的是副本，6.12.39 第 1 条），保存不了，只能另存为 |
| `too_large` | text / md / html 被截断（`truncated=true`，超过 2 MiB）；csv `totalRows > 1000` 或 `truncated=true` |
| `encoding` | html 在 `<meta charset>` 里声明了 UTF-8 / GBK / GB2312 / GB18030 以外的编码（如 Big5、Shift_JIS），按原编码写不回去 |
| `malformed` | csv 有解析不了的行（引号不配对等，6.12.32.4 里“原样放进一个单元格”的那种）。理由：在表格里改完再按 RFC 4180 写回会把这些行改坏 |
| `converting` | 这一行有 `queued` / `running` 的记录，或结果记录正在重转（`reconverting=true`），或源文件行的副本还在复制。**打开时的判断只是提示**，保存时会再查一次（6.12.41 第 4 步） |

- `editable=false` 时前端只读显示（与 v0.27 一样）；`missing` / `too_large` / `encoding` / `malformed` 时前端可以给一行灰字说明（文案见 6.12.45）。
- **`revision` 用内容哈希，不用“大小 + 修改时间”（架构师定）**。理由：能编辑的文件最多 2 MiB，算一次 SHA-256 只要几毫秒；修改时间在 FAT / exFAT、网络盘上精度只有 2 秒甚至更差，同一秒里被别的程序改过会漏判；反过来只是被“碰了一下”（修改时间变了、内容没变）也不该报冲突。

### 6.12.39 预览读哪个文件（对 v0.27 的改动）

1. **源文件行的 text / md / html / csv**：v0.27 用“显示路径”（副本优先）。**v0.27.1 改为读用户的原文件（`originalPath`）**，因为保存写的是原文件，`revision` 必须对应原文件。原文件不在了就退回读副本（`copyState=ready` 时），`editable=false`、`editBlock=missing`；副本也没有返回 `NOT_FOUND`（`reason=file`）。v0.24 之前的旧行（`copyState=none`）本来就读原文件。Word、Excel、PPT 等引擎预览仍按 v0.27 读显示路径。
2. **结果行**：读记录的 `outputPath`（不变）。
3. 读取时一并识别 `encoding` 和 `lineEnding`，算 `revision`（对读到的**原始字节**，不是转换后的 UTF-8）。

### 6.12.40 编码与换行（读和写共用）

- **编码（读取）**：按 6.12.17：合法 UTF-8 → 开头有 BOM 是 `utf8_bom`，没有是 `utf8`；否则 `gbk`（**解码仍用 GB18030**，它兼容 GBK，老文件里偶尔有的四字节序列也能读出来）。html 先看 `<meta charset>`：声明 UTF-8 → 按 UTF-8 判断（同上）；声明 GBK / GB2312 / GB18030 → `gbk`；声明别的 → 只读，`editBlock=encoding`。
- **写回（v0.27.1 补充，团队定，取代原来“按 GB18030 写回”）**（保存，和另存为选 `keep`）按读取时的编码：`utf8` 写 UTF-8 不带 BOM，`utf8_bom` 写 UTF-8 并带 BOM，`gbk` **严格按 GBK（CP936）编码**（`golang.org/x/text/encoding/simplifiedchinese.GBK` 的编码器，**不用** GB18030，也**不包** `encoding.ReplaceUnsupported`），**不生成 GB18030 的四字节序列**。理由：中文 Windows 上的 Excel 和记事本按 CP936 读，四字节序列会变成乱码。
- **编不进 GBK 的字符**（emoji、GBK 以外的生僻字，以及原文件里本来就有的 GB18030 四字节字符）：**拒绝保存**，返回 `INVALID_ARGUMENT` `reason=encoding`，message `有些字符没法按原编码保存，请另存为 UTF-8。`；**不悄悄替换、不丢字**，原文件不动。`detail` 第二行 `char=U+XXXX`、第三行 `line=<行号，从 1 开始>`，是第一个编不进去的字符，前端**可以**用来定位（`^char=U\+([0-9A-F]{4,6})$`、`^line=([0-9]+)$`），不用也能工作。用户可以用 `SaveDocTextAs` 选 `encoding=utf8` 另存。
- **已知边界**：GB18030 和 CP936 在极少数双字节码位上的映射不同（如 `0x80`），读出再原样保存可能不是逐字节相同；普通中文文本不受影响。
- **换行风格**：读取时数 `\r\n` 和单独的 `\n` 各有几处，`\r\n` 多就是 `crlf`，否则 `lf`；**一个换行都没有**时 Windows 上是 `crlf`、其他平台 `lf`（架构师定：按用户这台电脑上新建文件的习惯）。只有单独的 `\r`（老 Mac 风格）的文件按 `lf` 处理。
- **写入时**：前端传来的文字先把 `\r\n` 和单独的 `\r` 都统一成 `\n`（编辑器给的通常就是 `\n`），再按 `lineEnding` 换成 `\r\n` 或保留 `\n`，最后按编码转成字节。**除了这两步不对内容做任何改写**：不加不去结尾的换行，不格式化，html / md 不过滤、不补全标签（6.12.44）。

### 6.12.41 `SaveDocText`：保存到原位置

```go
SaveDocText(req DocSaveRequest) (DocSaveResult, error)

type DocSaveRequest struct {
    SourceID string     `json:"sourceId,omitempty"` // 源文件行：写回用户的原文件
    TaskID   string     `json:"taskId,omitempty"`   // 结果行：写回输出文件；二者必须且只能给一个
    Revision string     `json:"revision"`           // 打开（或上次保存）时拿到的 revision；SaveDocText 必填
    Text     *string    `json:"text,omitempty"`     // txt / md / html 用
    Rows     [][]string `json:"rows,omitempty"`     // csv 用；text 和 rows 按文件类型给其中一个，给错或都给 INVALID_ARGUMENT
}

type DocSaveResult struct {
    Path      string `json:"path"`      // 实际写入的文件（原文件或输出文件的绝对路径）
    Revision  string `json:"revision"`  // 新的 revision（写入的字节的 SHA-256），前端用它继续编辑、再保存
    SizeBytes int64  `json:"sizeBytes"`
    SavedAt   int64  `json:"savedAt"`   // Unix 毫秒
}
```

**写到哪**：源文件行 → 用户的原文件 `originalPath`；结果行 → 记录的 `outputPath`。

**检查顺序**（报第一个；都是同步错误，`detail` 首行 `reason=`，见 2.2）：

1. 参数：两个 id 都给或都不给、`text` / `rows` 给错 → `INVALID_ARGUMENT`（没有 reason）；`revision` 为空 → `INVALID_ARGUMENT`。
2. 记录：id 不存在 → `NOT_FOUND` `reason=record`；不是文档页的行 / 不是 `doc_convert`、`office_pdf` 记录 → `UNSUPPORTED` `reason=format`；结果记录不是 `succeeded` → `NOT_FOUND` `reason=file`（同 6.12.32.1）。
3. 能不能编辑：扩展名不是 txt / md / markdown / html / htm / csv → `INVALID_ARGUMENT` `reason=format`；**大小上限**：`text` 按目标编码写出来的字节超过 **2 MiB**，或 `rows` 超过 **1000 行**、单元格超过 32 767 个字符、写出来超过 2 MiB → `INVALID_ARGUMENT` `reason=too_large`；html 声明了不支持的编码 → `INVALID_ARGUMENT` `reason=format`。
4. **正在转换**：源文件行有 `queued` / `running` 的记录，或结果记录正在重转 → `TASK_CONFLICT` `reason=converting`（v0.27.1 补充：原来写的 `in_use` 改名，和下面文件被占用的 `IO_ERROR` `reason=in_use` 分开）；源文件行的副本正在复制 → `TASK_CONFLICT` `reason=copying`（沿用 v0.24 的取值，后面一行 `sourceId=<id>`）。
5. 文件不在了（`Lstat` 不存在，或不是普通文件）→ `NOT_FOUND` `reason=file`。原文件是符号链接时**写它指向的真实文件**（`EvalSymlinks` 后的路径），链接本身保留。
6. **冲突**：读当前文件的全部字节算 SHA-256，和 `revision` 不同 → `TASK_CONFLICT` `reason=file_changed`（当前文件超过 2 MiB 也算被改过）。读的时候文件被锁着读不了 → `IO_ERROR` `reason=in_use`（不是 `file_changed`）。
6a. **编码**：按目标编码转字节，编不进去 → `INVALID_ARGUMENT` `reason=encoding`（6.12.40）。放在冲突检查之后，是为了先告诉用户“文件被改过”，免得另存为 UTF-8 后才发现原文件已经变了；实现可以先转字节、后比对，报错顺序按这里。
7. 写入（见下）：没有写权限、只读属性、只读的盘 → `IO_ERROR` `reason=permission`；原文件被其他程序占用、**替换不了**（Windows 共享冲突 `ERROR_SHARING_VIOLATION` / `ERROR_LOCK_VIOLATION`，`ReplaceFileW` 返回 `ERROR_UNABLE_TO_REMOVE_REPLACED`；Unix `EBUSY` / `ETXTBSY`）→ `IO_ERROR` `reason=in_use`（**和 `file_changed` 分开**：一个是“现在写不进去”，一个是“内容已经变了”）；磁盘满 → `CONVERT_DISK_FULL`（沿用，不带 reason 行）；其他写入失败 → `IO_ERROR` `reason=io`。**失败时原文件保持原样**（临时文件删掉）。

**怎么写（原子替换）**：

- **中途失败不能损坏原文件**（v0.27.1 补充，重申）：任何一步失败（写临时文件、`Sync`、替换），原文件保持保存前的字节，临时文件删掉。`ReplaceFileW` 不传备份文件名；返回 `ERROR_UNABLE_TO_MOVE_REPLACEMENT` 时原文件未被改动；返回 `ERROR_UNABLE_TO_MOVE_REPLACEMENT_2` 时实现要**复核原文件仍在、字节与保存前一致**，不一致就把内存里的原字节写回，按 `IO_ERROR` `reason=io` 报错并记应用日志。
- 在**同一个目录**建临时文件 `.<原文件名>.ffmpegfree-<随机 8 位>.tmp`，写完 `Sync`、关闭，再替换原文件。目录不能建文件（没有权限）按 `IO_ERROR` `reason=permission`（引导另存为），**不退回“直接覆盖写”**：那样写到一半出错会把原文件写坏。
- **保留原文件的属性**：Windows 用 `ReplaceFileW`（保留原文件的 ACL、属性、创建时间、备用数据流），失败再退回 `MoveFileExW(MOVEFILE_REPLACE_EXISTING)`；macOS / Linux 先把临时文件 `chmod` 成原文件的权限位、尽量 `chown` 成原来的所有者（失败忽略），再 `rename`。**已知限制**：Linux / macOS 上原文件的扩展属性、硬链接关系不保留（替换后是新文件）。
- 同一个文件的保存**串行**（按真实路径加应用内的锁）；保存和这一行的提交转换（`SubmitDocConvert`、`Retry`、`Reconvert`）互斥：保存进行中提交会等它结束（很快）。第 6 步比对和第 7 步替换之间仍有极短的空档，别的程序恰好在这时写入会被覆盖，这是接受的残余风险。

**保存成功之后**：

- **源文件行**：把同样的字节写进这一行的副本（`storedPath`，同样“临时文件 + 替换”），并把 `convert_copies` 里的 `original_size`、`original_mtime_ns` 改成原文件的新值（同一个事务），这样副本仍算 `ready`、和原文件一致，不用重新复制（架构师定，理由：内容就在手上，直接写比重新复制快，也不会出现“复制中”不能转换的空档）。副本写失败时原文件已经保存成功，照样返回成功，副本置为 `failed`（`copyError` 是 `IO_ERROR` `reason=io`），发 `convert:copy`，用户可以 `RetryCopy`。`copyState=none` 的旧行没有副本，不用管；`failed` / `canceled` 的保持原状。
- **结果行**：更新这条记录 `result.sizeBytes`，`version` +1，发 `task:status`（状态不变，带新的 `result`）。**注意**：之后对这条记录 `Reconvert` 会按新参数重新生成并替换掉编辑过的内容（6.17 的 `replace` 模式），前端在“编辑过的结果”上点重转时建议先确认（文案见 6.12.45）。
- **预览缓存**：按原文件和副本的路径删掉 6.12.32.3 里的缓存项（目前 text / md / html / csv 不生成缓存，写上是为了以后不漏）。
- 返回新的 `revision`。前端继续编辑时用它。

### 6.12.42 `SaveDocTextAs`：另存为

```go
SaveDocTextAs(req DocSaveAsRequest) (DocSaveResult, error)

type DocSaveAsRequest struct {
    SourceID   string     `json:"sourceId,omitempty"` // 二者必须且只能给一个；用来确定原格式、原编码和换行
    TaskID     string     `json:"taskId,omitempty"`
    TargetPath string     `json:"targetPath"`         // 由 SystemService.SaveFileDialog 拿到的绝对路径
    Text       *string    `json:"text,omitempty"`
    Rows       [][]string `json:"rows,omitempty"`
    Encoding   string     `json:"encoding,omitempty"` // keep（默认，空 = keep）| utf8
}
```

- **不需要 `revision`**，不比对冲突；**原文件不在也能另存为**（`editBlock=missing` 的就是靠它）。来源行 / 记录本身要在（`NOT_FOUND` `reason=record`），但不检查“正在用”（写的是别的位置）。
- **扩展名必须和原文件同一种格式**（不区分大小写）：txt ↔ txt，md ↔ md / markdown，html ↔ html / htm，csv ↔ csv；不同或没有扩展名 → `INVALID_ARGUMENT` `reason=format`。
- **目标路径的校验**（同 6.12.3 的输出目录规则）：必须是绝对路径；拒绝 `\\?\`、`\\.\` 开头；**不能在应用数据目录内（`<dataDir>/output` 及其子文件夹除外）**，也**不能在实际上传目录（6.15.1 的 `uploads`，副本所在处）内**（副本由应用管理，用户写进去会和引用计数打架）；都返回 `INVALID_ARGUMENT`（没有 reason 行，message 见 6.12.45）；所在文件夹必须已存在；目标是文件夹 → `INVALID_ARGUMENT`。比较方式同 6.12.3（两边 `EvalSymlinks`，按平台处理大小写）。
- **目标已存在**：由系统保存对话框负责确认覆盖，后端**直接覆盖**（同样“临时文件 + 替换”，保留目标原有的属性）；目标只读 / 没有权限 → `IO_ERROR` `reason=permission`，被锁 → `IO_ERROR` `reason=in_use`。
- **编码 `encoding`**：`keep` 按原文件的编码和换行写（6.12.40；原来是 `gbk` 的同样严格按 GBK，编不进去返回 `INVALID_ARGUMENT` `reason=encoding`）；**允许设 `encoding=utf8`**（这就是编码存不了时的出路）；`utf8` 写 **UTF-8 不带 BOM**，换行仍按原文件。其他值 `INVALID_ARGUMENT`。**例外**：html 选 `utf8`、而内容里 `<meta charset>` 声明的是别的编码时，写 UTF-8 **带 BOM**（按 HTML 标准 BOM 优先于 meta，浏览器才能读对；内容本身仍不改）。csv 选 `utf8` 不带 BOM 与 6.12.17 一致；Excel 打开不带 BOM 的 UTF-8 CSV 中文会乱码，所以界面上 `keep` 是默认（原来带 BOM 的会继续带）。
- 大小上限同 `SaveDocText`。
- **不新建源文件行**，只返回保存后的路径（`DocSaveResult.path`）。**v0.28 定稿（取代原来的“例外”写法）**：目标正好是**这条记录自己的**源文件或输出 → 不拒绝，按覆盖处理，等同于对该文件“保存”，保存后刷新这条记录（副本 / `result`、返回新的 `revision` 和大小）；这条记录正在转换仍是 `TASK_CONFLICT` `reason=converting`；目标是**别的记录**的源文件、输出或副本 → `IO_ERROR` `reason=in_use`。详见 6.12.65 ①。
- **“打开所在文件夹”**：保存成功的目标路径登记进 `RevealInFolder` 的内存放行表（同 v0.23.3 的做法：本次运行有效、只放行这个文件本身、最多保留最新 100 个、不落库），前端用 `RevealInFolder(path)` 打开并选中它。

### 6.12.43 `SystemService.SaveFileDialog`：系统保存对话框（新增）

现有的 `PickFiles` / `PickDirectory` 都不是保存对话框，所以新增：

```go
SaveFileDialog(defaultName string, filters []FileFilter) (string, error) // 用户取消返回 ""（不是错误）
```

- `defaultName`：只有文件名时作为默认文件名；**是绝对路径时**取它所在的文件夹作为对话框的初始位置、文件名作为默认文件名（前端传原文件的路径即可，另存为默认就在原文件旁边）。用 Wails `runtime.SaveFileDialog`（`DefaultDirectory`、`DefaultFilename`、`Filters`、`CanCreateDirectories=true`）。
- `filters`：沿用 `FileFilter{name, patterns}`（6.8），例如 `[{name: "Markdown", patterns: ["*.md", "*.markdown"]}]`；空数组 = 不过滤。
- **覆盖确认**由系统对话框负责（Windows、macOS 的保存对话框自带）。**某个平台没有自带确认**（Linux 上的 GTK 对话框是否开了覆盖确认**需真机验证**）时，后端在 `SaveFileDialog` 里发现所选文件已存在，就用 `runtime.MessageDialog` 再问一次（`“<文件名>”已存在，要替换它吗？`，选“不替换”按取消处理返回 `""`），不改接口。
- 返回的路径不登记任何放行表；只有 `SaveDocTextAs` 保存成功后才登记（6.12.42）。

### 6.12.44 安全

- 编辑时 html 和 md 的**预览**仍按 6.12.32.4：DOMPurify 过滤后放进 `sandbox` 为空的 iframe，加严格 CSP，不加载任何网络或本地资源；边写边预览时每次刷新都走同样的过滤（建议 300 毫秒防抖，前端定）。
- **保存时不对内容做任何改写，原样写入**（只做 6.12.40 的换行和编码两步）。过滤只作用于显示，**绝不**把过滤后的 HTML 存回文件。
- 写入只发生在：用户的原文件、记录的输出文件、另存为对话框选的路径。后端不接受前端直接给的任意路径做“保存”（`SaveDocText` 只收 id），另存为的路径受 6.12.42 的目录规则限制。
- 日志只记路径和字节数，不记文件内容。

### 6.12.45 文案（v0.27.1 补充：产品已定的照用，其余仍是建议）

**产品已定**（后端 `message` 和前端映射都用这几句）：

| 场景 | code / reason | 文案 | 前端动作 |
|---|---|---|---|
| 不能编辑（太大） | `editBlock=too_large` | `文件太大，只能查看，不能在这里编辑。` | — |
| 文件在别处被改过 | `TASK_CONFLICT` `file_changed` | `文件在别处被改过了，请重新打开，或另存为。` | 「重新打开」「另存为」 |
| 被其他程序占用 | `IO_ERROR` `in_use` | `文件正被其他程序占用，请关闭后再保存。` | 「重试」「另存为」 |
| 正在转换 | `TASK_CONFLICT` `converting` | `文件正在转换，转完再保存。` | 「另存为」 |
| 编码存不了 | `INVALID_ARGUMENT` `encoding` | `有些字符没法按原编码保存，请另存为 UTF-8。` | 「另存为」（选好位置后用 `encoding=utf8`） |
| 关弹窗时有没保存的改动 | — | `有改动还没保存，要保存吗？` | 「保存」「不保存」「取消」 |

- **以上任何一种出错，都保留用户编辑的内容**：前端不关弹窗、不清空编辑器、不自动重新加载文件；「重新打开」由用户自己点（点之前同样走“有改动还没保存”的确认）。后端出错时不改原文件（6.12.41）。

**v0.27.2 定稿（产品已定；文本类和 docx 都适用，与下面“建议”表冲突时以这里为准）**：

| 场景 | code / reason / 条件 | 文案 | 前端动作 |
|---|---|---|---|
| 原文件不在 | `editBlock=missing`、`NOT_FOUND` `file` | 弹窗顶部 `原文件已经不在了，改完只能另存为。` | 「保存」置灰，只能「另存为」 |
| 编辑过的结果点重转 | 结果记录在应用里保存过 | `这个文件在应用里改过。重转会按原文件重新生成一份，你改的内容不会带过去。` | 「重转」「取消」 |
| 保存成功（文本类） | — | `已保存。` | — |
| docx 覆盖保存成功 | `backupPath` 非空 | `已保存，原文件备份在同一个文件夹。` | 「打开所在文件夹」（`RevealInFolder(backupPath)`） |
| 另存为成功 | — | `已另存为“<文件名>”。` | 「打开所在文件夹」 |
| 没有权限 | `IO_ERROR` `permission` | `没有权限保存到这里，请另存到其他位置。` | 「另存为」 |
| 内容太多 | `INVALID_ARGUMENT` `too_large` | `内容太多，没法在这里保存。请用默认程序打开编辑。` | — |
| docx 不完整，拒绝保存 | `INVALID_ARGUMENT` `malformed`（含宏的 `format`） | `文件没能保存，原文件没有改动。请另存为再试。` | 「另存为」 |
| 前端内部错误 | `checksum`、`chunk_order`、`save_session`、`saving` | `出了点问题，请重试。` | — |
| 不能编辑的格式 | `editBlock=format` | 悬停 `这类文件请用默认程序打开编辑` | — |
| 带宏的 docx | `editBlock=macro` | 悬停 `这个文件带宏，请用默认程序打开编辑。` | 编辑按钮置灰 |
| 备份失败 | `IO_ERROR` `backup` | `没法在这个文件夹留备份，文件没有保存。请另存为。` | 「另存为」；保留用户内容 |

- **所有出错都保留用户编辑的内容**（同上）。
- “编辑过的结果”怎么知道（架构师定）：前端在本次运行里对这条记录保存成功过就算；跨重启不记（不加字段、不加迁移）。

**v0.28 定稿（产品已定）**：

| 场景 | code / 条件 | 文案 | 前端动作 |
|---|---|---|---|
| Windows / macOS 文档组件太旧 | `componentState=outdated`、`DOC_COMPONENT_NOT_READY` `reason=outdated` | `文档组件版本太旧，请重新下载。` | 「更新文档组件」（调 `InstallDocComponent`） |
| 场景 35：有未保存的改动时点「重新打开」 | 编辑器有改动 | `重新打开会丢掉你改的内容，确定吗？` | 「重新打开」（危险样式）「取消」 |

**仍是建议（待产品定，后端先按这些写；上面已定稿的条目以上面为准）**：

| 场景 | code / reason | 建议文案 | 前端动作 |
|---|---|---|---|
| 保存成功 | — | `已保存。` | — |
| 另存为成功 | — | `已保存到 <路径>` | 「打开所在文件夹」 |
| 副本还在准备 | `TASK_CONFLICT` `copying` | `文件还在准备中，准备好后再保存。` | — |
| 原文件不在了 | `NOT_FOUND` `file` | `原文件不在了，请另存为。` | 「另存为」 |
| 没有权限 / 只读 | `IO_ERROR` `permission` | `没有权限写入这个文件，请另存为。` | 「另存为」 |
| 其他写入失败 | `IO_ERROR` `io` | `保存失败，请重试或另存为。` | 「另存为」 |
| 磁盘满 | `CONVERT_DISK_FULL` | `磁盘空间不足，没有保存。` | — |
| 内容太多 | `INVALID_ARGUMENT` `too_large` | `内容太多，不能保存（最多 2 MB 或 1000 行）。` | — |
| 另存为换了格式 | `INVALID_ARGUMENT` `format` | `只能保存成同一种格式。` | — |
| 另存为到应用目录 | `INVALID_ARGUMENT` | `不能保存到应用自己的文件夹里，请换一个位置。` | — |
| 编辑过的结果点重转 | — | `重转会覆盖你对这个文件的修改，要继续吗？` | — |
| 不能编辑：编码 | `editBlock=encoding` | `这个网页的编码不支持编辑，只能查看。` | — |
| 不能编辑：csv 格式有问题 | `editBlock=malformed` | `这个 CSV 有格式问题，只能查看，不能编辑。` | — |
| 不能编辑：原文件不在 | `editBlock=missing` | `原文件不在了，改完只能另存为。`（这种情况前端可以允许编辑、只给「另存为」，产品定） | 「另存为」 |
| 不能编辑：正在转换 | `editBlock=converting` | `文件正在转换，转完再编辑。` | — |

- 界面**不显示错误码、`reason=`、`editBlock` 的取值**（1.1）；前端按 code + reason 映射，映射不到的显示 `出了点问题，请重试。`。

### 6.12.46 测试与未验证事项

- 单测（Linux / macOS 能跑的都要有）：三种编码 × 两种换行的往返（读出再原样保存，普通中文文本字节完全不变）；GBK 文件加 emoji 后保存返回 `reason=encoding`、原文件字节不变、`detail` 的 `char=` / `line=` 正确，改用 `SaveDocTextAs` + `encoding=utf8` 成功；输出里**没有** GB18030 四字节序列；`revision` 冲突 `file_changed`；文件被锁 `IO_ERROR` `in_use`；正在转换时 `TASK_CONFLICT` `converting`；替换中途失败（模拟）原文件不变；只读文件 `permission`；原文件不在 `NOT_FOUND`；超过 2 MiB / 1000 行 `too_large`；csv 写回（引号、逗号、引号里的换行）；另存为的扩展名、应用数据目录、上传目录、`encoding=utf8`（含 html meta 的 BOM 例外）；保存后副本内容与原文件一致、`convert_copies` 的大小和修改时间已更新；结果行保存后 `result.sizeBytes` 和 `task:status`。
- **未验证（Windows / macOS / Linux 真机）**：

| # | 项目 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|
| 1 | Windows `ReplaceFileW` 在本地盘、U 盘（FAT / exFAT）、网络盘上都能用并保留 ACL 和属性；文件被记事本 / Excel 打开时的表现（Excel 锁住 csv → `in_use`） | 各种盘上保存；Excel 开着 csv 时保存 | 只用 `MoveFileExW`，接受属性丢失 |
| 2 | 三个平台的保存对话框是否自带覆盖确认（尤其 Linux GTK） | 另存为到已存在的文件 | 后端用 `MessageDialog` 补确认（已写进 6.12.43） |
| 3 | 编辑器在 Windows 上给出的换行（`\n` 还是 `\r\n`） | 编辑一行后保存，比对字节 | 后端已统一处理，不影响结果；只是确认 |


### 6.12.47 docx 在预览窗口里编辑后保存（v0.27.2；老板定范围，架构师定方案，文案产品已定）

> 在 6.12.37~6.12.46（文本类编辑）上**只放开 docx**。doc / xls / xlsx / ppt / pptx / pdf 仍然只能查看（`editBlock=format`），编辑按钮悬停显示 `这类文件请用默认程序打开编辑`。**前端用什么编辑器由前端决定，契约只管读写**：后端给原文件字节的地址，前端编辑后把完整的新 docx 分段传回，后端校验后替换。**没有新增错误码**（2.1 仍是 30 个），只在 2.2 加取值；**没有迁移、没有新事件**。与前面冲突时以本节为准。

### 6.12.48 读取：docx 的 `DocPreview`

- **新增字段 `rawUrl string \`json:"rawUrl,omitempty"\``**：docx 且 `editable=true` 时一定有，是原文件字节的本地地址（`/local/<token>`，6.13 白名单，只放行这一个文件），前端进入编辑时用它取字节（按 Range 分段，同 6.12.32.1 的 raw 规则）。理由（架构师定）：有引擎时 docx 的 `kind` 是 `pdf`（看排版用），`url` 指向的是生成的 PDF，编辑需要的是原 docx；单独给 `rawUrl`，查看和编辑互不影响，`kind=raw` 时 `rawUrl` 与 `url` 相同。
- **读哪个文件**：同 6.12.39：源文件行读用户的原文件（`originalPath`），原文件不在时读副本、`editable=false`、`editBlock=missing`；结果行读 `outputPath`。`rawUrl` 指向的就是读到的那个文件。**只用于查看的 PDF 预览仍按 v0.27 读显示路径**。
- **`revision`**：整个文件的 SHA-256（小写十六进制），只在文件 ≤ 20 MiB 时计算和返回（更大的不能编辑，不需要）。
- **`editBlock` 的判断顺序**（命中第一个就停；v0.27.2 补充加 `macro`，排在 `malformed` 之后、`converting` 之前）：

| 取值 | docx 什么时候 |
|---|---|
| `format` | 扩展名不是 docx（doc / xls / xlsx / ppt / pptx / pdf 等） |
| `missing` | 源文件行的原文件已经不在（只能另存为） |
| `too_large` | 文件大于 **20 MiB**（20 × 1024 × 1024 = 20 971 520 字节） |
| `malformed` | 加密（6.12.19 判为加密）或打不开：不是 zip、缺 `word/document.xml`。**打开时只做这个轻量检查**，完整检查在保存时做（6.12.50） |
| `macro` | **v0.27.2 补充**：带宏——zip 里有名字以 `vbaProject.bin` 结尾的条目（不区分大小写），或 `[Content_Types].xml` 里出现 `macroEnabled` 的内容类型（只读中央目录和这一个部件，不解压别的）。悬停文案 `这个文件带宏，请用默认程序打开编辑。` |
| `converting` | 这一行有排队中 / 运行中的转换，或结果记录正在重转（保存时会再查） |

- 文本类的 `editBlock` 规则不变（6.12.38）。

### 6.12.49 分段保存的接口（DocService，v0.27.2 新增）

```go
BeginDocBinarySave(req DocBinarySaveBegin) (DocBinarySaveSession, error)
AppendDocBinaryChunk(req DocBinaryChunk) (DocBinaryChunkResult, error)
CommitDocBinarySave(req DocBinarySaveCommit) (DocBinarySaveResult, error)
AbortDocBinarySave(req DocBinarySaveAbort) error // 幂等：不存在 / 已结束的 saveId 直接返回 nil

type DocBinarySaveBegin struct {
    SourceID   string `json:"sourceId,omitempty"`   // 二者必须且只能给一个
    TaskID     string `json:"taskId,omitempty"`
    Mode       string `json:"mode"`                 // overwrite | save_as
    TargetPath string `json:"targetPath,omitempty"` // save_as 必填：SystemService.SaveFileDialog 拿到的绝对路径；overwrite 不给
    Revision   string `json:"revision,omitempty"`   // overwrite 必填：打开时拿到的 revision；save_as 不看
    TotalBytes int64  `json:"totalBytes"`           // 新 docx 的总字节数，1 ~ 20 MiB
}
type DocBinarySaveSession struct {
    SaveID        string `json:"saveId"`        // 128 位随机，十六进制
    MaxChunkBytes int    `json:"maxChunkBytes"` // 4194304（4 MiB，解码后的原始字节）；前端按返回值分段，不写死
    ExpiresAt     int64  `json:"expiresAt"`     // Unix 毫秒 = Begin 时刻 + 10 分钟
}
type DocBinaryChunk struct {
    SaveID string `json:"saveId"`
    Seq    int    `json:"seq"`  // 从 0 起连续
    Data   string `json:"data"` // base64（标准字母表，含 = 填充，同 6.12.4 的 PDFChunk.data），解码后 1 ~ maxChunkBytes 字节
}
type DocBinaryChunkResult struct {
    ReceivedBytes int64 `json:"receivedBytes"` // 累计已收的原始字节
}
type DocBinarySaveCommit struct {
    SaveID string `json:"saveId"`
    SHA256 string `json:"sha256"` // 整份新 docx 的 SHA-256（小写十六进制），前端用 crypto.subtle 算
}
type DocBinarySaveResult struct {
    Path       string `json:"path"`                 // 实际写入的文件
    Revision   string `json:"revision"`             // 新 revision = 新文件的 SHA-256
    SizeBytes  int64  `json:"sizeBytes"`
    SavedAt    int64  `json:"savedAt"`              // Unix 毫秒
    BackupPath string `json:"backupPath,omitempty"` // 只有 overwrite 成功时有：这次备份的绝对路径（6.12.51）
}
type DocBinarySaveAbort struct {
    SaveID string `json:"saveId"`
}
```

**`BeginDocBinarySave`**（检查顺序，报第一个）：

1. 参数：id 两个都给或都不给、`mode` 不是两者之一、overwrite 没有 `revision` 或给了 `targetPath`、save_as 没有 `targetPath` → `INVALID_ARGUMENT`（没有 reason）；`totalBytes` ≤ 0 → `INVALID_ARGUMENT`；**`totalBytes` > 20 MiB → `INVALID_ARGUMENT` `reason=too_large`**。
2. 记录：不存在 `NOT_FOUND` `reason=record`；不是文档页的行 / 文档记录 `UNSUPPORTED` `reason=format`；结果记录不是 `succeeded` `NOT_FOUND` `reason=file`；原文件扩展名不是 docx → `INVALID_ARGUMENT` `reason=format`。
3. **save_as 的目标**：规则同 6.12.42（绝对路径、不是 `\\?\` / `\\.\`、所在文件夹已存在、不是文件夹、**不在应用数据目录内（`output` 除外）、不在上传目录 `uploads` 内**，这几项是没有 reason 的 `INVALID_ARGUMENT`）；扩展名必须是 `.docx`（不区分大小写），否则 `INVALID_ARGUMENT` `reason=format`。**v0.28 定稿**：目标正好是这条记录自己的原文件 / 输出 → 按 overwrite 处理（不比对 `revision`，做完整性检查和备份、返回 `backupPath`），正在转换 → `TASK_CONFLICT` `reason=converting`；目标是别的记录的原文件、输出或副本 → `IO_ERROR` `reason=in_use`（6.12.65 ①）。
4. **overwrite**：正在转换 `TASK_CONFLICT` `reason=converting`（副本复制中 `reason=copying`）；原文件 / 输出文件不在 `NOT_FOUND` `reason=file`；读不了（被锁）`IO_ERROR` `reason=in_use`；**SHA-256 和 `revision` 不同 → `TASK_CONFLICT` `reason=file_changed`**（提前发现，免得白传 20 MiB）。
5. **同一文件同时只允许一个保存会话**：按要写入的真实路径（`EvalSymlinks` 后，Windows / macOS 不分大小写）加锁；已有未结束的会话（含 `SaveDocText` / `SaveDocTextAs` 正在写同一文件）→ `TASK_CONFLICT` `reason=saving`。
6. 暂存目录所在磁盘剩余 < `totalBytes` × 2 + 64 MiB（暂存一份、同目录临时文件一份的估算）→ `CONVERT_DISK_FULL`。
7. 建 `<数据目录>/tmp/docsave/<saveId>/`，返回会话。

**`AppendDocBinaryChunk`**：

- `saveId` 不存在、已结束或已过期 → `NOT_FOUND` `reason=save_session`。
- `data` 不是合法 base64、解码后为空或超过 `maxChunkBytes` → `INVALID_ARGUMENT`（没有 reason）。
- **`seq` 必须等于已收到的段数**（从 0 起连续）；不等 → `INVALID_ARGUMENT` `reason=chunk_order`，**会话不作废**（前端可以从正确的 `seq` 接着传；重复发上一段也是这个错误，不会写两次）。
- 累计字节超过 `totalBytes` 或 20 MiB → `INVALID_ARGUMENT` `reason=too_large`，**立刻删暂存文件、作废 `saveId`**。
- 写入暂存文件 `<数据目录>/tmp/docsave/<saveId>/data.part`（顺序追加），返回累计字节数。暂存目录写失败 → `IO_ERROR` `reason=io`（磁盘满 `CONVERT_DISK_FULL`），会话作废。

**`CommitDocBinarySave`**（顺序固定；**不管成功还是失败，Commit 之后会话都结束、暂存文件删掉**，失败时前端要重新 Begin 再传。理由：本地传 20 MiB 只要几秒，比维护“失败后还能继续提交”的半截状态简单可靠）：

1. `saveId` 不在 → `NOT_FOUND` `reason=save_session`。
2. **核对**：累计字节数 ≠ `totalBytes`，或暂存文件的 SHA-256 ≠ 请求里的 `sha256` → `INVALID_ARGUMENT` `reason=checksum`。
3. **overwrite 再核对一次 revision**（读当前原文件的 SHA-256）：不同 → `TASK_CONFLICT` `reason=file_changed`；原文件不在了 → `NOT_FOUND` `reason=file`；这时正在转换 → `TASK_CONFLICT` `reason=converting`。
4. **完整性检查**（6.12.50）：不通过 → `INVALID_ARGUMENT` `reason=malformed`（含宏 `reason=format`）。
5. **备份**（只有 overwrite，6.12.51）：失败就不保存，**一律 `IO_ERROR` `reason=backup`**（v0.27.2 补充；不论是没有权限、被占用、磁盘满还是别的 IO 错误，前端靠它选文案）；备份前复核发现文件被改仍是 `TASK_CONFLICT` `reason=file_changed`。
6. **写入**：把暂存文件复制到**目标所在目录**的临时文件（`.<原文件名>.ffmpegfree-<随机 8 位>.tmp`，`Sync`），再替换：
   - overwrite：Windows `ReplaceFileW`（保留 ACL、属性、创建时间；失败处理同 6.12.41：`ERROR_UNABLE_TO_REMOVE_REPLACED` / 共享冲突 → `IO_ERROR` `reason=in_use`；`ERROR_UNABLE_TO_MOVE_REPLACEMENT_2` 时复核原文件字节，不一致用备份恢复），macOS / Linux `chmod` / 尽量 `chown` 后 `rename`。
   - save_as：同目录临时文件 + 原子改名（Windows `MoveFileExW(MOVEFILE_REPLACE_EXISTING)`，Unix `rename`）；**目标已存在时直接覆盖**（`SaveFileDialog` 已确认过，6.12.43），不做备份。
   - 没有权限 → `IO_ERROR` `reason=permission`；被占用 → `IO_ERROR` `reason=in_use`；其他 → `IO_ERROR` `reason=io`；磁盘满 → `CONVERT_DISK_FULL`。**任何一步失败原文件 / 原目标都不变**，临时文件删掉；overwrite 已经做了的备份**保留**（它是原文件的完整拷贝，留着无害），但不算进“成功”，`backupPath` 不返回。
7. **之后**（同 6.12.41“保存成功之后”）：源文件行把同样字节写进副本并更新 `convert_copies` 的大小和修改时间（副本写失败只把副本置 `failed`，保存仍算成功）；结果行更新 `result.sizeBytes`、`version` +1、发 `task:status`；删掉这个文件在预览缓存（6.12.32.3）里的项；save_as 的目标正好是某一行 / 某条记录时同样刷新。写入的路径和 `backupPath` 都登记进 `RevealInFolder` 的内存放行表（同 6.12.42），供「打开所在文件夹」。
8. 返回 `DocBinarySaveResult`。**前端保存成功后重新调一次 `GetDocPreview`**：替换后旧的 `/local/<token>`（`rawUrl`、`url`）按 6.13 的 `SameFile` 校验会失效（404），要拿新地址和新的 PDF 预览。

**会话的生命周期**：

- 暂存只放 `<数据目录>/tmp/docsave/<saveId>/`，**不放用户的文件夹**（同目录临时文件只在 Commit 的第 6 步出现、马上就替换或删除）。
- **Begin 后 10 分钟没有 Commit 就自动清理**（删目录、作废 `saveId`；后台每分钟扫一次）。过期后再调 Append / Commit → `NOT_FOUND` `reason=save_session`。`AbortDocBinarySave` 立刻清理，幂等。
- **应用启动时删掉整个 `tmp/docsave/`**（上次没走完的会话）。
- 同时存在的会话最多 4 个（不同文件），第 5 个 → `TASK_CONFLICT` `reason=saving`（防止前端漏调 Abort 把磁盘占满）。

### 6.12.50 完整性检查（Commit 第 4 步；失败一律不动原文件）

按顺序：

1. **防 zip 炸弹**（先只读中央目录，不解压）：条目数 ≤ **10 000**，所有条目声明的解压后大小之和 ≤ **200 MiB**；任何一项超了 → `reason=malformed`。中央目录、EOCD / zip64 记录的防护复用 6.12.3 的 `checkZipEntries`（中央目录 ≤ 9 600 000 字节等）。
2. **zip 能打开、CRC 全过**：逐个条目完整解压一遍（只读到丢弃，不落盘），Go `archive/zip` 读到结尾时会校验 CRC-32；**实际解压出的总字节数**也按 200 MiB 封顶（防止声明的大小是假的），超了立即停止 → `reason=malformed`。
3. **必需部件**：有 `[Content_Types].xml`、`_rels/.rels`、`word/document.xml`（按 OPC 规则，名字不区分大小写）；缺任何一个 → `reason=malformed`。
4. **`word/document.xml` 是格式良好的 XML**：用 `encoding/xml` 的 `Decoder` 流式读到结尾（`Strict=true`，不展开外部实体），出错 → `reason=malformed`。只检查格式良好，不校验 schema。
5. **不含宏**（打开时已经按 `editBlock=macro` 挡住，这里是兜底：前端绕过或保存的内容自己带了宏）：任何名字以 `vbaProject.bin` 结尾的条目（不区分大小写），或 `[Content_Types].xml` 里出现 `macroEnabled` 的内容类型 → **`reason=format`**（不是 malformed：文件是好的，只是这里不允许保存带宏的 docx）。
6. 加密的 docx（OLE 容器）在第 2 步就打不开，是 `reason=malformed`。

### 6.12.51 备份（只有 overwrite）

- **时机**：完整性检查通过之后、替换之前，把**当前的原文件**复制成同目录的 `<原名去扩展名>.bak-YYYYMMDD-HHMMSS.docx`（本地时间，如 `报告.docx` → `报告.bak-20261009-110400.docx`）；同一秒已有同名文件就加 `-2`、`-3`……（`报告.bak-20261009-110400-2.docx`），直到不冲突。扩展名一律写小写 `.docx`。
- **怎么复制**：先写同目录临时文件再改名成备份名（不会留下半个备份）；修改时间设成原文件的修改时间（方便用户按时间认出是哪一版）。复制前再核对一次原文件的 SHA-256 仍等于 `revision`（第 3 步之后的极短空档被改了就按 `file_changed` 返回，不备份也不保存）。
- **失败就不保存**：备份写不了（没有权限、被占用、磁盘满等任何原因）→ **`IO_ERROR` `reason=backup`**（v0.27.2 补充，取代原来按 `permission` / `in_use` / `io` / `CONVERT_DISK_FULL` 分开报），`detail` 第二行起可以写具体原因（如 `cause=permission`，只给开发者看），原文件不动。前端文案 `没法在这个文件夹留备份，文件没有保存。请另存为。` + 「另存为」，保留用户编辑的内容。
- **只保留最近 3 份**：**替换成功之后**清理。只看同一目录里、**同一原名**、**精确匹配**本应用格式的文件：`^<原名去扩展名（按字面转义）>\.bak-[0-9]{8}-[0-9]{6}(-[0-9]+)?\.docx$`（Windows / macOS 不区分大小写），且是普通文件、不是符号链接。按文件名里的时间（同秒按 `-n` 序号）从新到旧排，第 4 份及更旧的删掉。**不碰任何不完全匹配的文件**（用户自己改过名的、别的程序的 `.bak`、别的原名的）。清理失败只记应用日志，不影响保存结果。
- 返回 `backupPath`（这次新建的备份）。**备份文件不加进文档列表**（不建源文件行），也不进预览缓存。
- 文本类（`SaveDocText`）**不做备份**，规则不变（6.12.41）。理由：文本类改动小、本身就是原样写入；docx 是前端编辑器重新生成的整份文件，丢格式的风险高，所以要留后路。

### 6.12.52 错误汇总（docx 保存；复用已有码，2.1 仍是 30 个）

| code | reason | 出现在 | 前端文案（6.12.45） |
|---|---|---|---|
| `INVALID_ARGUMENT` | `too_large` | Begin（`totalBytes` > 20 MiB）、Append（累计超了，会话作废） | `内容太多，没法在这里保存。请用默认程序打开编辑。` |
| `INVALID_ARGUMENT` | `chunk_order` | Append（`seq` 不连续，会话不作废） | `出了点问题，请重试。` |
| `INVALID_ARGUMENT` | `checksum` | Commit（字节数或 SHA-256 不符） | `出了点问题，请重试。` |
| `INVALID_ARGUMENT` | `malformed` | Commit（完整性检查不过） | `文件没能保存，原文件没有改动。请另存为再试。` |
| `INVALID_ARGUMENT` | `format` | Begin（不是 docx、另存为扩展名不对）、Commit（含宏） | `文件没能保存，原文件没有改动。请另存为再试。`（Begin 的扩展名错误前端可提示 `只能保存成同一种格式。`） |
| `INVALID_ARGUMENT` | — | 参数、base64、另存为目标在应用目录 / 上传目录 | 应用目录：`不能保存到应用自己的文件夹里，请换一个位置。`；其他 `出了点问题，请重试。` |
| `NOT_FOUND` | `save_session` | Append、Commit（会话不在 / 过期） | `出了点问题，请重试。` |
| `NOT_FOUND` | `file` / `record` | Begin、Commit | `原文件已经不在了，改完只能另存为。` |
| `TASK_CONFLICT` | `file_changed` | Begin、Commit、备份前 | `文件在别处被改过了，请重新打开，或另存为。` |
| `TASK_CONFLICT` | `converting` / `copying` | Begin、Commit | `文件正在转换，转完再保存。` / `文件还在准备中，准备好后再保存。` |
| `TASK_CONFLICT` | `saving` | Begin（同一文件已有保存会话，或会话数到上限） | `出了点问题，请重试。` |
| `IO_ERROR` | `backup` | Commit（备份失败，v0.27.2 补充） | `没法在这个文件夹留备份，文件没有保存。请另存为。` + 「另存为」 |
| `IO_ERROR` | `permission` | Commit（替换） | `没有权限保存到这里，请另存到其他位置。` |
| `IO_ERROR` | `in_use` | Begin（读不了）、Commit（替换） | `文件正被其他程序占用，请关闭后再保存。` |
| `IO_ERROR` | `io` | Append、Commit | `保存失败，请重试或另存为。` |
| `CONVERT_DISK_FULL` | — | Begin、Append、Commit | `磁盘空间不足，没有保存。` |
| `UNSUPPORTED` | `format` | Begin（不是文档页的行 / 文档记录） | `出了点问题，请重试。` |

- **所有出错都保留用户编辑的内容**（不关弹窗、不清空编辑器、不重新加载）。界面不显示错误码和 `reason=`。
- 前端在任何失败后都调一次 `AbortDocBinarySave`（幂等），关弹窗、放弃保存时也调。

### 6.12.53 测试与未验证事项

- **单测**（Linux / macOS 能跑的都要有）：分段的正常流程；`seq` 跳号 / 重复 → `chunk_order` 且会话仍可用；超 20 MiB（Begin、Append 各一）；`checksum`；过期（把时钟拨快）→ `save_session`；同一文件第二个会话 `saving`；完整性检查（截断的 zip、CRC 错、缺 `word/document.xml`、XML 不闭合、带 `vbaProject.bin`、声明 10 001 个条目、解压后超 200 MiB 的 zip 炸弹）；备份命名（同秒 `-2`）、只保留 3 份、**不删**用户自己的 `报告.bak.docx`、`报告-副本.bak-20261009-110400.docx`、别的原名的备份；备份失败不保存；替换失败原文件不变；save_as 不备份、不能写进 `uploads`；保存后副本、`result.sizeBytes`、旧 `/local/<token>` 404。
- **真机待验证**（加在 6.12.46 的表后面）：

| # | 项目 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|
| 4 | `ReplaceFileW` 替换 docx：本地盘、U 盘、网络盘；Word 正开着这个 docx 时（应返回 `in_use`，原文件不变）；替换后资源管理器里的属性、创建时间 | Windows 上各种盘保存；Word 开着时保存 | 改用 `MoveFileExW`，接受属性丢失 |
| 5 | 备份清理：连续保存 5 次只剩 3 份；同秒两次保存出现 `-2`；用户自己改名的 `.bak` 文件不被删；中文、空格、括号的原名 | 手动连续保存，看文件夹 | 修正匹配规则 |
| 6 | 20 MiB 文件分段上传耗时：4 MiB 一段（base64 后约 5.6 MB 一次 Bind 调用）在 Windows WebView2 上是否稳定、总耗时（目标 < 5 秒） | 保存一个 19 MiB 的 docx，记录每段耗时 | 后端把 `maxChunkBytes` 调小（前端按返回值分段，不用改） |


### 6.12.54 打开文档组件所在文件夹（`OpenStorageFolder("doc_component")`，v0.27.2）

- **`SystemService.OpenStorageFolder(kind)` 新增 `kind="doc_component"`**：在系统文件管理器里打开**应用下载的文档组件目录**本身——`<组件目录>/doc/<正在用的版本>/`（Windows `%LocalAppData%\FFmpegFree\components\doc\26.2.6\`，macOS `~/Library/Application Support/FFmpegFree/components/doc/26.2.6/`；组件目录见 6.12.12）。平台命令同 6.15.2 第 6 条的“打开文件夹”分支（Windows `explorer.exe "<dir>"`，macOS `open <dir>`），命令启动后立即返回。签名不变（只是多一个取值），Wails 绑定不用重新生成。
- **只在“正在用应用下载的文档组件”时可用**：`componentState=ready` 且 `engines` 里 `id=component` 那一项的 `source=downloaded`（`DocComponentStatus.source` 是“排第一个的引擎”，可能是 `office` / `wps`，所以**不看顶层 `source`**，看 `engines` 里组件那一项）。其他情况——组件检测中、没下载、下载 / 准备中、失败、版本太旧（6.12.55），或者用的是系统安装的 LibreOffice，或者目录不在了——一律 `NOT_FOUND`，message `文档组件还没有就绪。`，`detail` 只有一行 `reason=component`（沿用 v0.25.1 转换组件同一个 reason；**不带路径**）。Office / WPS 是用户自己的软件，不提供打开它们所在文件夹。
- **前端**：设置页“文档组件”一块的「打开所在文件夹」**只在 `componentState=ready` 且组件那一项 `source=downloaded` 时显示**；收到上面的错误（状态刚好变了）显示 message 即可。
- **不加进 `RevealInFolder` 的放行范围**（6.8 不变），路径不回给前端（同 v0.25.1 的 `component`）。Linux 没有应用下载的组件，恒为 `NOT_FOUND` `reason=component`（前端也不会显示这个按钮）。

### 6.12.55 应用下载的文档组件版本太旧（Windows / macOS，v0.27.2）

- **契约写死的版本是 26.2.6**（6.12.11 / 6.12.12）。检测时扫描 `<组件目录>/doc/` 下所有不带 `.staging` 的版本目录；某个目录里的组件按 6.12.12 的方式读出版本，**前三段数字小于 `26.2.6`**（`26.2.6.3` 按 `26.2.6` 比；例如以前的版本留下的 `doc/26.2.5/`）就是“版本太旧”的候选。系统安装的仍按原规则（主版本 ≥ 7.2，6.12.11），**不要求 26.2.6**。
- **什么时候是 `componentState=outdated`**：沿用 9.4 转换组件的原则——**版本过低的候选只在没有任何合格候选时才算**。所以：有合格的系统 LibreOffice 时是 `ready`（`source=system`），旧的下载组件不用；没有任何合格候选、但有太旧的下载组件时是 `componentState=outdated`。系统 LibreOffice 低于 7.2 的 `outdated`（v0.26 / v0.27 已有）不变。
- **`outdated` 时**：`canDownload=true`（Windows x64、macOS 本来就是 true；**允许调 `InstallDocComponent`**，下载 26.2.6，装好后变 `ready` 并删掉旧的 `doc/<旧版本>/` 目录，删不掉只记日志）；`version` 是那个旧版本号；`engines` 里组件那一项 `installed=true`、`available=false`、`source=downloaded`、`version` 为旧版本；`error` 为 `DOC_COMPONENT_NOT_READY`，message `文档组件版本太旧，请重新下载。`（v0.28 定稿），`detail` 第一行 `reason=outdated`。`state`（整体）仍按 6.12.28：有 Office / WPS 可用就是 `ready`，否则等于 `componentState`（`outdated`）。
- **提交任务**（`SubmitDocConvert`、任务开始时的兜底、6.12.33 的回退）：需要文档组件、又没有别的可用引擎时返回 `DOC_COMPONENT_NOT_READY`；**`detail` 第一行写 `reason=<componentState>`**（架构师定：`missing` / `outdated` / `downloading` / `preparing` / `failed` / `checking`，组件太旧时就是 `reason=outdated`），message 在 `outdated` 时是上面那句，其他情况不变（6.12.20）。
- **预览**（`GetDocPreview`）：**沿用已有定义，不改成报错**——没有可用引擎时仍是 `kind=unavailable` + `reason=needs_component`（6.12.32），不是 `DOC_COMPONENT_NOT_READY`。前端在 `componentState=outdated` 时把提示换成 `文档组件版本太旧，请重新下载。` + 「更新文档组件」（v0.28 定稿；点了调 `InstallDocComponent`）。
- 格式表（`GetFormatMatrix`）：`componentReady=false`，需要组件的目标置灰，`disabledReason` 仍是 `需要文档组件`（不新增 hintKey）。
- Linux 不变（没有下载组件）。

### 6.12.56 `doc_convert` 记录的 `startedAt`（v0.27.2）

- `Task.startedAt`（第 3 节，已有字段，`tasks.started_at` 已有列）**`doc_convert` 必须填**：任务**离开排队、真正开始处理**的时刻（Unix 毫秒）——进文档组件池 / 本机办公软件池的任务是**拿到池名额、开始挑引擎启动转换的时刻**；不进池的（md ↔ html）是开始运行的时刻。同时发 `running` 的 `task:status`，带 `startedAt`（6 节已有规则）。
- **排队中为 0**（JSON 里 `startedAt: 0`；事件里按已有规则省略）。排队中被取消、退出时还在排队而被中断的记录 `startedAt` 保持 0（同第 6 节已有规则）。
- 原地重试（`Retry`）把它清回 0，重新开始时再填；重转（6.17）按已有规则：重转期间对外仍是旧值，成功后是这次的开始时间，失败恢复旧值。
- 引擎回退（Office → WPS → 组件，6.12.25）**不重新填**：`startedAt` 是第一次开始处理的时间。
- `office_pdf`（简易转换）沿用同一规则。**没有迁移**（字段和列都已有，只是把“必须填”写死）。

### 6.12.57 准备阶段的取消（说明，v0.27.2）

- 契约**仍允许**在 `preparing`（解包、检测）时调 `CancelDocComponentInstall`（6.12.12 第 5 条，规则不变）。
- **前端可以不提供**准备阶段的取消按钮（按设计稿：准备通常一两分钟，中途取消意义不大）；下载阶段的取消照旧。后端不因为前端不调而改变任何行为。

## 6.12.58 PDF 作为输入转成其他格式（v0.28；用户提出，架构师定方案；文案 v0.28.1 定稿，见 6.12.66 ④）

> 用户原话：“需要支持 pdf 转成其他格式，现在 pdf 无法上传转换。”本节**取消** 6.12.10 / 6.12.16 / 6.12.20 里“PDF 只作输出、添加时拒绝（`DOC_PDF_INPUT_UNSUPPORTED`，`PDF 暂时不能转成其他格式。`）”的限制。任务类型仍是 `doc_convert`，**没有迁移**（现有最大号仍是 0009，下一个是 0010）。与 6.12.9~6.12.57 冲突时以本节为准。

### 6.12.59 范围与格式表

- **新家族 `pdf`**：`family` 的取值加 `pdf`（`DocSource.family`、`DocSourceFormats.family`、`DocEngineInfo.families` 都可能出现）。源格式只有 `pdf` 一个；**PDF 不算文字类**，所以不和 text 家族互转规则混在一起，目标单独列：

| 源 | 目标 | 用什么转（按顺序） | `needsComponent` | `simple` | `hintKey` |
|---|---|---|---|---|---|
| pdf | `doc` `docx` `odt` `rtf` | ① Word（Office，**Word ≥ 2013**，即主版本 ≥ 15，PDF 重排从 2013 起有）→ ② 文档组件（应用下载的 → 系统 LibreOffice，`writer_pdf_import`）。**不用 WPS**（WPS 的 PDF 转 Word 是会员功能） | `true` | `false` | `pdf_layout` |
| pdf | `txt` `md` | ① **纯 Go 提取文字**（6.12.62，不占任何池）→ ② 质量判定不过时**自动改用文档组件**（有可用的才用）。**不用 Word**（Word 只转 doc / docx / odt / rtf） | `false` | `true` | `pdf_text`（v0.28.2；原为 txt `simple_mode`、md `md_lossy`） |
| pdf | `html` | 有可用的文档组件：组件 `writer_pdf_import` 导出 html（`simple=false`）；没有：纯 Go 提取文字生成简易 html（`simple=true`）。**不用 Word** | `false` | 没有组件时 `true` | 没有组件时 `simple_mode` |

- **不支持**（矩阵里**不列出**，不是置灰）：xlsx / xls / ods / csv / pptx / ppt / odp、图片、pdf（转成自己）。PDF 转图片放以后。
- **目标顺序**：按 6.12.10 文字类的顺序 `doc` `docx` `odt` `rtf` `txt` `html` `md`（没有 pdf）。
- **`DocFormatMatrix.inputs`** 加 `pdf`；`sources` 加一项 `{ext: "pdf", family: "pdf", targets: [...]}`。
- **`available`**：doc / docx / odt / rtf 需要 Word（≥ 15）或可用的文档组件，都没有时置灰，`disabledReason=需要文档组件`；txt / md **恒为 `true`**（纯 Go）；html 恒为 `true`（没有组件就走简易）。
- **`engines`**（6.12.30 的 `DocTarget.engines`）：doc / docx / odt / rtf 为 `["office", "component"]` 中现在可用的；txt / md 为 `["go"]`，有可用组件时 `["go", "component"]`；html 有组件 `["component"]`，没有 `["go"]`。
- **`hintKey`** 新增 `pdf_layout`，文案（v0.28.1 定稿）`PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。`。`hint` 一项只放一句：**v0.28.2 起 pdf → txt、pdf → md 给 `pdf_text`**（`md_lossy` 只用于非 PDF 源），没有组件时的 pdf → html 给 `simple_mode`。**`simple_mode` 在 PDF 源上的文案**（v0.28.1 定稿）：`只提取文字，不保留排版和图片。`（与组件未就绪时的 `下载文档组件后可保留图片和排版` 区分：前者是 PDF 源，后者是其他源；后端按源给 `hint`，`hintKey` 相同）。
- **能力表**（6.12.27）加一列 **PDF**：`office` 源 pdf → 目标 doc docx odt rtf（**只看 Word，且主版本 ≥ 15**）；`wps` **不做**；`component` 源 pdf → 目标 doc docx odt rtf txt md html。纯 Go（`go`）源 pdf → txt md html 不进能力表（与 md ↔ html 一样是内置的）。`DocEngineInfo.families`：Word ≥ 15 时 `office` 带 `pdf`；`component` 已装时带 `pdf`；`wps` 不带。
- **预览**：PDF 本身的预览（6.12.32，`kind=pdf`）**不变**；PDF 仍不能编辑（`editBlock=format`）。

### 6.12.60 添加 PDF：检查、大小和页数

- `.pdf` 不再返回 `DOC_PDF_INPUT_UNSUPPORTED`，按 6.12.16 的流程建行（`kind='doc'`、`family='pdf'`、`sheetCount=0`），复制副本，转换读副本。
- **大小上限：PDF 单独定为 200 MiB**（209 715 200 字节；其他文档仍是 100 MiB）。超过 → `INVALID_ARGUMENT` `reason=too_large`，message（v0.28.1 定稿）`PDF 太大了，最多支持 200 MB。`，不建行。
- **页数上限：500 页**。添加时用纯 Go 库（6.12.62）读页数（5 秒内，和其他轻量检查同一个超时）：**> 500 → `INVALID_ARGUMENT` `reason=too_many_pages`**（新 reason），message（v0.28.1 定稿）`PDF 页数太多，最多支持 500 页。`，不建行。**读不出页数（库解析不了）不拒绝**：建行，交给引擎；运行前再读一次，仍读不出就照常转（引擎自己能读的 PDF 比纯 Go 库多），引擎的单次超时兜底。理由：纯 Go 库对部分合法 PDF（例如修复过的交叉引用表）解析不了，不能因此拒绝用户本来能转的文件。
- **基本检查**：文件开头 1024 字节内没有 `%PDF-` → `DOC_CORRUPT`，不建行。文件为空同样 `DOC_CORRUPT`。
- **加密**（加进 6.12.19 的表）：trailer（或交叉引用流的字典）里有 `/Encrypt` 时，用**空的用户密码**试打开：
  - 打得开（只有“所有者密码”、限制打印 / 复制的 PDF，很常见）→ **不算加密**，照常转；
  - 打不开（要用户密码）→ **`DOC_ENCRYPTED`**，沿用现有文案 `这个文件有密码保护，不能转换。请先去掉密码再添加。`，不建行；
  - 库不支持这种加密方式（例如它不认的加密处理程序）→ 也按 `DOC_ENCRYPTED`（宁可拒绝也不让 Word / 组件弹密码框卡住）。**需验证**：常见的 AES-256（R6）PDF 走哪一条（6.12.64）。
  - **v0.28.1（以实现为准，6.12.66 ①）**：RC4 / AES-128 只有所有者密码的照常转；AES-256（R5 / R6）只有所有者密码的添加时拒绝，`DOC_ENCRYPTED` `reason=owner_only`，文案 `这个 PDF 设置了权限保护，暂时不能转换。`；要用户密码的不变。
  - 添加、提交、运行前各检查一次（同 6.12.19）。Word 打开时仍带“固定的假密码”（6.12.26 第 1 条），组件同理不会等输入。

### 6.12.61 引擎：怎么转

**Word（Office）**，只用于 → doc / docx / odt / rtf：

- 沿用 6.12.26 的全部硬规则：**只打开我们自己的临时拷贝**、`AutomationSecurity=3`、私有实例 + 实例隔离检查、`DisplayAlerts=0`、窗口不可见、单次超时 3 分钟、超时只结束我们的 PID。
- 打开：`Documents.Open(FileName, ConfirmConversions:=False, ReadOnly:=True, AddToRecentFiles:=False, PasswordDocument:=<固定的假密码>, Visible:=False, OpenAndRepair:=False, NoEncodingDialog:=True)`（Word 2013+ 打开 PDF 时会做“PDF 重排”，转成可编辑的文档）；然后 `SaveAs2(FileName, FileFormat:=16 / 0 / 23 / 6)`（docx / doc / odt / rtf，同 6.12.26 导出表）。
- Word 重排时的“Word 现在会把 PDF 转换为可编辑文档……”提示：靠 `DisplayAlerts=0` 关掉；**不改** Word 的选项（6.12.26 第 3 条：不改会保存下来的设置）。关不掉就会卡到 3 分钟超时、换文档组件（**需真机验证**）。
- Word < 15 不用于 PDF；Word 打不开（报错、产物为空）按 6.12.25 换下一个引擎（文档组件）。

**文档组件**（应用下载的优先，再系统 LibreOffice），用于 → doc / docx / odt / rtf / html，以及 txt / md 的回退：

- `soffice --headless --infilter=writer_pdf_import --convert-to <过滤器> ...`（`writer_pdf_import` 把 PDF 当作 Writer 文档打开，版式用文本框还原，**质量比 Word 差**；不加 `--infilter` 时组件会用 Draw 打开 PDF，导不出这些格式）。导出过滤器沿用 6.12.11 文字类的写法（doc / docx / odt / rtf / html / txt）；md 先导出 html，再 html-to-markdown（6.12.26 的 md 规则）。
- 进文档组件池（并发 2）；Word 失败换到组件时的规则同 6.12.29（实际以 6.12.65 记的实现为准）。临时配置目录、宏和链接设置（6.12.27）不变。

**纯 Go**（`engine=go`），用于 → txt / md，以及没有组件时的 → html：见 6.12.62。不进任何池（同 md ↔ html，提交后直接开始运行，`startedAt` 就是开始时间）。

- **`TaskResult.engine`**（6.12.31）：实际完成的那个——Word 是 `office`，组件是 `component`，纯 Go 是 `go`（**pdf → txt / md / 简易 html 记 `go`，不记 `simple`**：`simple` 留给“排版引擎都失败后的回退”；PDF 的纯 Go 提取本来就是首选方式）。纯 Go 质量不过、改用组件成功时记 `component`。
- **`params.engine`**（6.12.21）：提交时按格式表定的首选：doc / docx / odt / rtf / 有组件的 html 为 `component`（含先试 Word，沿用 v0.27 的含义），txt / md / 没组件的 html 为 `go`。
- 超时：整个任务最多 10 分钟（6.12.29 不变）；纯 Go 提取单次 **2 分钟**（在单独的 goroutine 里跑，到时放弃这次结果按“提取失败”处理）。

### 6.12.62 纯 Go 提取文字与质量判定

- **库：`github.com/ledongthuc/pdf`**（`go.mod` 固定到 `v0.0.0-20260907135840-6c8c28e0e8a0`，即 2026-09-07 的 master，**许可证 BSD-3-Clause**，可以随应用分发、不传染）。选择理由：① 纯 Go、不用 cgo，三个平台同一份代码；② 它是 Go 官方作者 `rsc.io/pdf` 的维护分支，在其上加了按页取纯文本（`GetPlainText` / 按行按字）和页数（`NumPage`）、处理 ToUnicode 字体映射，正好覆盖“取文字”的需要；③ 体积小、没有其他依赖。没选的：`pdfcpu`（Apache-2.0，擅长改 PDF，取文字能力弱）；`unidoc/unipdf`（AGPL / 商业许可，不能用）；调用外部 `pdftotext`（要随包带 poppler，违背纯 Go）。**已知局限**：遇到损坏或少见结构的 PDF 可能报错甚至 panic（rsc.io/pdf 的传统）→ 一律在单独的 goroutine 里 `recover`，panic 和报错都按“解析失败”处理，不让应用崩溃；对越界、超大对象由库自身和 2 分钟超时兜底。
- **提取**：逐页取纯文本（最多 500 页，与上限一致）；页内按库给的行；页与页之间空一行。
- **质量判定**（每次纯 Go 提取后都做；**提取失败**的两种情况，`detail` 第二行写 `quality=empty` / `quality=garbled`，**只给开发者看**，前端不解析）：
  - **`empty`**：去掉所有空白（Unicode `White_Space`）后一个字符都没有。
  - **`garbled`**：在所有**非空白字符**里，下面三类加起来**占 30% 及以上**：替换符 `U+FFFD`；私用区字符（`U+E000–U+F8FF`、`U+F0000–U+FFFFD`、`U+100000–U+10FFFD`）；控制字符（Unicode 类别 `Cc`，换行、回车、制表符本来就算空白，不计入）。理由：没有 ToUnicode 映射的字体常被取成私用区或替换符，30% 以上时文字基本不可读，不如交给组件或直接告诉用户。
  - 解析失败（报错、panic、2 分钟超时）**不算质量问题**，按下面同样“改用组件”处理；没有组件时报 `DOC_CORRUPT`（`detail` 第一行 `engine=go`，第二行 `parse=<简短原因>`）。
- **提取失败时**（txt / md / 没组件时的 html）：
  1. **有可用的文档组件**（应用下载的或系统 LibreOffice，`componentState=ready`）→ **自动改用组件** `writer_pdf_import` 导出 txt / md / html（md 走 html → md），任务继续（日志写 `[FFmpegFree] PDF 文字提取失败（garbled），改用文档组件`），成功则 `result.engine=component`。改用组件时**进组件池**排队（插到组件池最前面，同 6.12.29“已经等过一次”的规则），不另起任务。
  2. **没有可用的组件** → 失败，**`DOC_PDF_NO_TEXT`**（新增码，6.12.63），`detail` 第一行 `reason=no_text`，第二行 `quality=empty|garbled`。
  3. 组件导出的 txt / md 去掉空白后为空（扫描件常见：组件只还原出图片框）→ 同样 `DOC_PDF_NO_TEXT`（`detail` 第二行 `quality=empty`，第三行 `engine=component`）。组件导出的 html 不做这个检查（可能只有图片，也是有效结果）。
- **输出 txt**：**UTF-8 不带 BOM**，换行 Windows 写 `\r\n`、macOS / Linux 写 `\n`。理由：① 和 6.12.17 / 6.12.26 已定的“csv、txt 输出一律不带 BOM”一致（同一个文档页，同一种 txt 不该有两种写法）；② Windows 10 1903 起的记事本、各平台编辑器默认按 UTF-8 打开不带 BOM 的文件，BOM 反而会让脚本和其他程序读到多余的字节；③ 换行跟平台走，老记事本也能正常显示。组件导出的 txt 同样去掉 BOM（6.12.26 已有）。
- **输出 md**（纯 Go）：每段文字一段，段与段之间空一行，页与页之间也空一行；行首的 Markdown 特殊字符（`#` `>` `-` `+` `*` `|`、`数字.`）和行内的 `\` `` ` `` `*` `_` `[` `]` `<` 加反斜杠转义，避免原文被当成标题、列表或链接。UTF-8 不带 BOM，换行 `\n`（md 统一用 `\n`，与 6.12.11 一致）。
- **输出简易 html**（纯 Go，没有组件时）：`<!DOCTYPE html><html><head><meta charset="utf-8"><title>原文件名</title></head><body>` + 每段一个 `<p>`（HTML 转义）+ 页与页之间 `<hr>` + `</body></html>`；`result.warnings` 带 `simple_fallback`（复用 v0.27 的 warning：记录上显示 `这次是简易转换，只保留了文字。可以稍后重转。`）。
- **不占组件槽位**：纯 Go 这一步在 PoolFree 里跑，不受文档组件池 2 个名额和 Office 锁的限制。

### 6.12.63 错误码（v0.28 新增 1 个；2.1 由 30 个变为 31 个）

| code | message | 可重试 | 出现在 | 说明 |
|---|---|---|---|---|
| `DOC_PDF_NO_TEXT` | `这个 PDF 里没有能提取的文字，可能是扫描件。`（v0.28.1 定稿） | **否** | 任务（pdf → txt / md / 简易 html） | 纯 Go 提取失败且没有可用组件，或组件导出的 txt / md 为空。`detail` 第一行 `reason=no_text`，第二行 `quality=empty\|garbled`（只给开发者看） |

- **为什么加新码而不复用**：已有的 `DOC_CORRUPT`（文件坏了，只能移出）和 `DOC_FORMAT_UNSUPPORTED`（格式不支持）意思都不对；扫描件是好文件，只是没有文字层，用户需要的是“换成转 Word / 等以后的 OCR”，文案必须单独一句。不可重试：同一份文件、同一组引擎结果不会变；用户装了文档组件后可以对这一行**新提交**一次（源文件行还在）。
- **`DOC_PDF_INPUT_UNSUPPORTED` 保留在枚举里，不再产生**（旧记录、旧前端可能还会见到它；文案不变）。
- 沿用的码：太大 `INVALID_ARGUMENT` `reason=too_large`（PDF 上限 200 MiB）；页数太多 `INVALID_ARGUMENT` `reason=too_many_pages`（**2.2 新增取值**）；加密 `DOC_ENCRYPTED`；不是 PDF / 解析失败且没有组件 `DOC_CORRUPT`；Word / 组件出错、超时同 6.12.33。
- 2.1 的 TS 联合类型加 `'DOC_PDF_NO_TEXT'`；“文档类错误码能否重试”表加一行（否，v0.28）。

### 6.12.64 测试与未验证事项

- **单测**（纯 Go 部分 Linux / macOS 都能跑）：添加 PDF 建行、`family=pdf`；201 MiB → `too_large`；501 页 → `too_many_pages`；读不出页数仍建行；只有所有者密码的 PDF 能转、要用户密码的 → `DOC_ENCRYPTED`；不是 PDF → `DOC_CORRUPT`；质量判定（全空白 → empty；30% 私用区 → garbled；29% → 通过；`\n\t` 不计入）；提取失败 + 有组件 → 改用组件、`engine=component`；提取失败 + 没组件 → `DOC_PDF_NO_TEXT` 且 `detail` 第二行正确；库 panic 不崩溃；txt 不带 BOM、换行按平台；md 转义；格式表里 pdf 的目标、`simple`、`hintKey`、`available`（有 / 没有 Word、有 / 没有组件四种组合）；WPS 不出现在 pdf 的 `engines` 里。
- **真机待验证**：

| # | 项目 | 怎么验证 | 不通过怎么办 |
|---|---|---|---|
| 1 | Word 打开 PDF 时的重排提示能否被 `DisplayAlerts=0` 关掉；Word 2013 / 2016 / 365 各一 | 转一个 PDF，看是否卡到超时 | 卡住就靠超时换组件；仍不行则 Word 不用于 PDF（改能力表） |
| 2 | Word 重排 500 页 / 200 MiB 的 PDF 耗时是否超过 3 分钟 | 准备大文件实测 | 调 PDF 的单次超时（契约变更） |
| 3 | 组件 `writer_pdf_import` 导出 docx / html / txt 的效果和耗时（26.2.6 与系统 7.x） | 同一批 PDF 对比 | 只改提示文案 |
| 4 | `ledongthuc/pdf` 对 AES-256（R6）、交叉引用流、对象流、中文 CID 字体的支持 | 一批真实 PDF（含 WPS / Word 导出的、扫描件、加密的） | 不支持的加密按 `DOC_ENCRYPTED`；取不出的靠组件回退 |
| 5 | 30% 阈值是否合适 | 真实乱码 PDF 统计 | 调阈值（契约变更） |

### 6.12.65 v0.28 顺带定稿与实现差异记录

**① 另存为的目标正好是这条记录自己的文件（定稿，改 6.12.42 和 6.12.49 第 3 步）**：

- `SaveDocTextAs` / `BeginDocBinarySave(mode=save_as)` 的 `targetPath`（按 6.12.3 的方式比较：`EvalSymlinks`，Windows / macOS 不分大小写）**正好是请求里这条记录自己的文件**——源文件行（`sourceId`）的原文件 `originalPath`，或结果记录（`taskId`）的输出 `outputPath`——时**不拒绝，按覆盖处理**，等同于对这个文件“保存”：
  - 文本类：照 6.12.41 写入（临时文件 + 原子替换，编码按请求的 `encoding`），**不比对 `revision`**（用户已在系统对话框里确认覆盖）；docx：照 6.12.49 的 overwrite 走 Commit（含完整性检查和**备份**，返回 `backupPath`），同样不比对 `revision`。
  - 保存后**刷新这条记录**：源文件行同步副本、更新 `convert_copies` 的大小和修改时间；结果记录更新 `result.sizeBytes`、`version` +1、发 `task:status`；返回新的 `revision` 和 `sizeBytes`，前端用它们替换编辑器里记着的值（之后点「保存」不会误报 `file_changed`）。
  - 这条记录**正在转换**（或副本复制中）→ 仍是 `TASK_CONFLICT` `reason=converting`（`copying`）。
- 目标是**别的记录**的源文件、输出或副本 → **`IO_ERROR` `reason=in_use`**（不管那条记录在不在转换；取代 6.12.42 原来“同样刷新那一行”的写法）。副本通常在上传目录里，先被“不能写进上传目录”的规则挡住（`INVALID_ARGUMENT`）；改过上传目录后留在旧位置的副本（6.15.2 第 4 条）才会走到这一条。
- 目标不是任何记录的文件：规则不变（直接覆盖已有文件，不备份）。

**② 记录后端实现与契约的差异（以实现为准）**：

- **Office / WPS 的并发 1**：实现**没有单开“本机办公软件池”**，而是 `OfficeConverter` 里**一把全局锁**（转换和预览共用）保证同一时刻只有一个 Office / WPS 调用；能用 Office / WPS 的 `doc_convert` 任务**不进文档组件池**（`PoolFree`），提交后直接开始、在锁上等。语义与 6.12.29 等价（不占组件的 2 个名额，同时最多 1 个 Office / WPS 调用）。对外差别：在锁上等的任务对外已是 `running`（`startedAt` 是开始等锁的时刻，没有 `queuePosition`）；6.12.56 的“离开排队”对这类任务就是这个时刻。
- **Job Object**：Office / WPS 进程**暂不放进 Job Object**（`attachJob` 现在是空的），只靠“超时只结束我们自己启动的那个 PID”（`TerminateProcess`）满足 6.12.26 第 4 条的硬规则。**列为后续加固项**：应用被强制结束时 Office / WPS 进程可能残留（6.12.36 第 6 项真机验证时记录）。
- **文档组件的链接更新设置**（6.12.27 补充，预置进每个任务临时配置目录的 `registrymodifications.xcu`）：宏 `MacroSecurityLevel=3`、`DisableMacrosExecution=true`；外部链接加载时不更新——**Writer `Content/Update/Link=0`、Calc `Content/Update/Link=1`**（两者“从不”的取值不同：Writer 0 = 从不；Calc 0 = 总是、1 = 从不、2 = 询问）。

**③ 文案定稿**：写进 6.12.45 的“v0.28 定稿”表——Windows / macOS 组件太旧 `文档组件版本太旧，请重新下载。` + 「更新文档组件」；场景 35「重新打开」的二次确认 `重新打开会丢掉你改的内容，确定吗？`，按钮「重新打开」（危险样式）「取消」。

### 6.12.66 v0.28.1：按 #138 / #139 实际合入的内容记录（以实现为准）

> 本节只记录已经合入 v2 的实现（前端 #138、后端 #139，以及 #137 的几处偏差），**没有新增错误码（2.1 仍是 31 个）、没有迁移、没有新事件、没有接口签名变化**。与 6.12.58~6.12.65 冲突时以本节为准。

**① `DOC_ENCRYPTED` 新增 `reason=owner_only`（#139）**：

- **含义**：PDF 有 `/Encrypt`，**空的用户密码能通过校验**（不要密码就能打开），但设了所有者 / 权限密码，而且**纯 Go 库解不了这种加密**。
- **哪些照常转**：库能处理的只有所有者密码的 PDF——**RC4（R2 / R3）和 AES-128（R4）**——照常转换，和 6.12.60 一样。
- **哪些拒绝**：**AES-256（R5 / R6）**只有所有者密码的 PDF，**添加文件时就拒绝**，`DOC_ENCRYPTED`，`detail` 一行 `reason=owner_only`，**不可重试**；界面文案 `这个 PDF 设置了权限保护，暂时不能转换。`。
- **真要用户密码的 PDF**：不变，`DOC_ENCRYPTED` 不带 `detail`，沿用原文案 `这个文件有密码保护，不能转换。请先去掉密码再添加。`。
- **怎么判断**（实现）：后端自己读 trailer / 交叉引用流里的 `/Encrypt`（内联字典或间接对象）和 `/ID`，按 PDF 规范校验空用户密码（R2 算法 4；R3 / R4 算法 5；R5 SHA-256；R6 算法 2.B）；读不懂的（非 `Standard` 处理程序等）仍按“要密码”处理。添加、提交、运行前、重转、库超时兜底、纯 Go 提取都走同一个判断。测试夹具是 qpdf 12.2 生成的 11 个小 PDF（R2~R6、对象流、有无用户密码）。
- 2.2 新增取值：`DOC_ENCRYPTED` 的 `owner_only`。
- **后续项（未做）**：有可用的文档组件时，把 AES-256 只有所有者密码的 PDF 交给组件转换，而不是在添加时拒绝。

**② 已接受的 PDF 实现偏差（#137）**：

- **纯 Go 提取失败、改用文档组件时**（6.12.62）：pdf → txt 不让组件直接导出 txt（`writer_pdf_import` 打开的文档直接导出 txt 拿不到文本框里的字），而是**先让组件导出 html，再从 html 里取文字**写成 txt（跳过 head / script / style，块级元素和 `<br>` 换行，连续空行合并，UTF-8 不带 BOM，换行按平台）；pdf → md 本来就是 html → md。取出来仍为空 → `DOC_PDF_NO_TEXT`（`quality=empty`，`engine=component`），同 6.12.62。
- **pdf → html 走组件但组件在导出 html 时失败**：回退到纯 Go 的简易 html（`result.engine=go`，`result.warnings` 带 `simple_fallback`），不让任务失败。（v0.28.2 已在代码中确认；用户取消、加密、损坏、磁盘满时直接报错，见 6.12.67）
- **PDF 的转换结果可以预览**：结果记录按输出格式走 6.12.32 的预览规则（docx / txt / md / html 等），PDF 源本身仍是 `kind=pdf`。前端预览按钮对所有源和成功的记录都显示（含 PDF 源和 PDF 的输出）。

**③ 前端（#138）**：

- `DOC_V28_BACKEND_READY=true`：文件选择允许 `.pdf`，格式表和记录走真实绑定；绑定没有变化。
- `KNOWN_CODE_RE` 包含 `DOC_PDF_NO_TEXT`。
- 组件未就绪时的横条（6.12.13，前端常量）改为：`Word、ODT、TXT 可以简易转 PDF，md 和网页可以互转，PDF 可以提取文字转成 TXT、md 和简易网页。`。
- **两处 `engines` 的区别**：组件状态里的 `DocComponentStatus.engines`（6.12.28）**不含 `go`**（只有 office / wps / component）；格式表的 `DocTarget.engines`（6.12.30 / 6.12.59）**保留 `go`**（如 pdf → txt 为 `["go"]` 或 `["go", "component"]`）。前端判断能不能转只看 `available` / `needsComponent` / `simple`，不读 `engines`；记录详情里 `engine=go` 不显示引擎行。
- `INVALID_ARGUMENT` 的 `too_large` / `too_many_pages`（添加 PDF 被拒）在界面上不给重试，只能移除。
- pdf → md 的提示：~~后端给 `md_lossy`、前端显示“只提取文字”的不一致~~ **已在 v0.28.2 解决**：pdf → txt / md 统一 `hintKey=pdf_text`，前后端同一句 `只提取文字，不保留排版和图片。`（6.12.67）。

**④ PDF 文案定稿**（6.12.59 / 6.12.60 / 6.12.63 原来标的“待产品定”到此为止）。来源：前端 `frontend/src/utils/docV26Text.ts` 注明“产品 10-09 定稿”的常量，与后端 `message` / `hint` 逐字一致：

| 场景 | code / hintKey | 定稿文案 |
|---|---|---|
| pdf → doc / docx / odt / rtf | `pdf_layout` | `PDF 转 Word 会尽量保留排版，复杂版式和扫描件可能会走样。` |
| pdf → txt / md（v0.28.2 起 `pdf_text`）、没有组件时的 pdf → html（`simple_mode`） | `pdf_text` / `simple_mode` | `只提取文字，不保留排版和图片。` |
| 没有可提取的文字 | `DOC_PDF_NO_TEXT` | `这个 PDF 里没有能提取的文字，可能是扫描件。` |
| PDF 超过 200 MiB | `INVALID_ARGUMENT` `too_large` | `PDF 太大了，最多支持 200 MB。` |
| PDF 超过 500 页 | `INVALID_ARGUMENT` `too_many_pages` | `PDF 页数太多，最多支持 500 页。` |
| AES-256 只有所有者密码 | `DOC_ENCRYPTED` `owner_only` | `这个 PDF 设置了权限保护，暂时不能转换。` |

### 6.12.67 v0.28.2：PDF 提示 `pdf_text`、简易 html 回退确认（按 #141 / #142 记录）

- **新 `hintKey`：`pdf_text`**（产品 10-09 定）：pdf → txt 和 pdf → md 的 `DocTarget.hintKey=pdf_text`，`hint` 为 `只提取文字，不保留排版和图片。`。
- **`md_lossy` 只用于非 PDF 源 → md**（doc / docx / odt / rtf / txt / html → md），文案不变。
- **pdf → html 不变**：有组件时没有提示；没有组件时 `simple_mode`，PDF 源上的文案是 `只提取文字，不保留排版和图片。`（6.12.59）。pdf → doc / docx / odt / rtf 仍是 `pdf_layout`。
- `hintKey` 仍是字符串（`omitempty`），**Wails 绑定不变**；前端遇到不认识的 `hintKey` 直接显示后端给的 `hint`。
- 6.12.66 ③ 记的“pdf → md 前后端提示不一致”就此解决：前后端都用 `pdf_text` 这一句。
- **确认（#142，`pdfrun.go` 的 `producePDF`）**：pdf → html 走文档组件、组件导出 html 失败时，**回退到纯 Go 的简易 html**，任务成功，`result.engine=go`，`result.warnings` 带 `simple_fallback`。**例外**（直接报错，不回退）：用户取消（`ctx` 已结束）、`DOC_ENCRYPTED`、`DOC_CORRUPT`、`CONVERT_DISK_FULL`。
- 没有新增错误码（仍 31 个），没有迁移，没有新事件。

### 6.12.68 v0.28.3：格式表和组件状态不再等检测（按 #144 / #145 / #146 记录）

> 背景：老板反馈文档页格式显示慢——`GetFormatMatrix` 在文档组件 `checking` 时会等最多 6 秒。本节取代 6.12.14 里“组件在 checking 时最多等 6 秒”的写法。**没有新增错误码（仍 31 个），没有迁移，没有新事件，接口签名和字段都不变。**

**① `GetFormatMatrix` 立即返回（#146）**：

- 组件在 `checking` 时**不再等**（去掉原来最多 6 秒的等待），按**当前已知**的引擎状态立即生成：
  - 已经检测到的 Office / WPS（只读注册表，通常几十毫秒就有结果）照常算可用，能做的目标 `available=true`；
  - **只靠文档组件**的目标按未就绪返回：`available=false`、`needsComponent=true`、`disabledReason=需要文档组件`；纯 Go 的（md ↔ html、PDF 提取文字、简易转换）照常可用。
  - `componentReady`（= 有任何引擎可用）可能是 `false`；**格式表本身没有 `componentState` 字段**，检测中这个状态要看 `GetDocComponentStatus` / `doc:component` 的 `componentState=checking`。
- 所以检测中拿到的格式表**可能有部分格式暂时置灰**；检测结束后前端按 ④ 重拉即可。

**② 需要引擎的入口仍然最多等 6 秒（#146）**：

- **提交（`SubmitDocConvert`）、重试（`TaskService.Retry` 的 doc 记录）、重转（`Reconvert` 的 doc 记录）**：目标现在不可用、而文档组件还在 `checking` 时，**最多等 6 秒**检测结果再判断一次（整个调用只等一次，不是每个文件各等 6 秒），不把“检测中”当成“没有组件”拒绝。等完仍不可用再按原规则报错（`DOC_COMPONENT_NOT_READY` 等）。
- 重试 / 重转改为和提交一样**看全部引擎的快照**（以前只看文档组件，只有 Office 的电脑重试会被误拒）。
- **添加文件（`AddDocSources`）不等**：添加只做大小、加密、损坏、页数这些检查，不依赖引擎，所以不会因为检测中被误拒。
- `GetDocComponentStatus`、`ListDocSources`、`GetDocPreview` 都不等 `checking`（预览遇到检测中按 6.12.32 当时的引擎状态处理）。

**③ `GetDocComponentStatus` 与 `doc:component`（#146）**：

- 检测中**立即返回** `state=checking`；Windows 上已检测到 Office / WPS 时可能是 `state=ready` + `componentState=checking`（6.12.28）。
- **检测结束**（变成 `ready` / `missing` / `outdated` / `failed` 中任何一个）后端**至少发一次 `doc:component`**，payload 是合并后的完整状态（经引擎注册表发出），**与 `GetDocComponentStatus` 的返回相同**。

**④ 前端规则（#144 / #145）**：

- **每次**收到 `doc:component` 都重新调 `GetFormatMatrix`（同 6.12.28），**丢弃过期的响应**（只用最后一次请求的结果，先发后到的旧响应扔掉）。
- `componentState=checking` 时**不显示“不可用”的闪烁、不出下载文档组件的入口**（置灰的格式在检测中不提示“需要文档组件”）。
- 检测结束后格式区 **0.2 秒淡入**刷新。
- 页面初始化时 `GetDocComponentStatus`、`GetFormatMatrix`、源列表（`ListDocSources`）**三个请求并行**发出（订阅事件仍在请求之前，同第 5 节的订阅顺序）。
- #145 另外调整了文档页的滚动和底部栏（只改界面，不涉及接口）。

**⑤ 后续项（未做）**：文档组件冷启动仍要跑一次 `soffice --version`（60 秒超时）和一次冒烟转换（120 秒超时，6.12.12），**没有按版本缓存检测结果**来跳过。计划：按“组件可执行文件路径 + 大小 + 修改时间 + 版本”缓存通过的结果（同 6.16.1 转换组件的做法），命中时跳过冒烟转换。

## 6.13 本地资源访问 `/local/<token>`（中立章节，DocService 与转换记录共用；由 #22 引入，#23 引用，v0.23 加 `convert` 表；v0.23.5 删除 `edit` 表）

> **章节位置**：本节编号固定为 6.13，**排在 6.12（DocService，#23 引入）之后**、`## 7.` 之前；#22 单独看时 6.12 还不存在，所以这里紧跟在 6.11 后面，#23 合入时把 6.12 插在 6.11 和本节之间（编号不变，只是位置，所有交叉引用写的都是编号，不受影响）。

> 只有契约。合并顺序（架构师最新决定）：**#22 先合**，然后 #19、#23：本节和 6.11.3 的文件名净化函数由 #22 引入，#19 的直播存档（复用净化函数）和 #23（引用本节）都依赖它；直播存档的**实现**放在 #22 合入之后，#19 的文档层面先合也无妨。

**用途**：让 WebView 用 `<video>` / `<audio>` / pdf.js 读取用户本机的文件，而不暴露任意路径读取，也不监听任何端口。挂在 Wails AssetServer 的 `Handler`（`options.App.AssetServer.Handler`，只处理静态资源之外的请求）。

1. **登记表分表**：~~`edit` 表（`EditService.GetPreviewURL`）~~（v0.23.5 随剪辑删除）、`doc` 表（`DocService.OpenPDF` 的大文件 URL）和 **`convert` 表（v0.23：`TaskService.GetPreviewURL`、`ConvertService.GetSourcePreviewURL`，见 6.14）**各自独立，**每表最多 512 项**，满了按最近使用淘汰最旧的（LRU，淘汰的 token 之后返回 404）；三张表互不挤占。删除转换记录时撤销 `convert` 表里对应路径的 token。同一路径在同一张表里复用同一个 token。
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
7. **响应头**：`Accept-Ranges: bytes`、`X-Content-Type-Options: nosniff`、`Cache-Control: no-store`、`Content-Type` 按扩展名：`mp4/m4v → video/mp4`，`mov → video/quicktime`，`mkv → video/x-matroska`，`webm → video/webm`，`avi → video/x-msvideo`，`flv → video/x-flv`，`mp3 → audio/mpeg`，`wav → audio/wav`，`aac → audio/aac`，`m4a → audio/mp4`，`flac → audio/flac`，`ogg / opus → audio/ogg`，**`gif → image/gif`（v0.23，转换页预览）**，**`ogv → video/ogg`、`m4r → audio/mp4`、`bmp → image/bmp`、`ico → image/x-icon`（v0.24，6.14.7）**，**`pdf → application/pdf`**（`png / jpg / jpeg / webp` 也有映射；v0.24 起转换页预览会放行它们，6.14.7）；表外的扩展名不会到达这里（登记时已拒绝）。
8. **token 失效与前端重试（架构师定）**：token 不存在、被淘汰、应用重启、文件变化、被 `RemoveRecent*` 撤销 → 一律 **404**（不区分原因）。前端在 `<video>` / `<audio>` 触发 `error`、或每次用旧 URL 之前，先对该 URL 发一个 **`HEAD`** 请求探测：`200`/`206` 才继续；`404` → 重新调用 `GetPreviewURL(path)`（或 `OpenPDF(path)`；转换页是 `TaskService.GetPreviewURL(taskId, which)` / `ConvertService.GetSourcePreviewURL(sourceId)`）换新 URL，**只重试一次**，仍失败则按"文件不存在或已被移动"提示。
9. **Windows 限制**：Wails v2.11.0 `pkg/assetserver/webview/responsewriter_windows.go` 把响应体缓冲在内存里，`Finish` 才一次性交给 WebView2（官方 Options 文档："Response Body Streaming：Windows ❌，macOS ✅，Linux ✅"），所以上面的 4 MiB / 32 MiB 限长在所有平台一律生效。WebView2 收到被截短的 `206` 之后是否会继续请求下一段 **未在 Windows 真机验证**，由用户在预览包里验证（社区有只发第一段的反馈，wailsapp/wails#5047 无结论）。

## 6.14 转换记录（v0.23：convert_sources、原地重试、任务中心隐藏）

> 只有契约，实现另开 PR。设计依据：《转换页 v2 设计说明 v0.1》§7.1–7.3、§八，PM 与架构师 2026-10-08 的决定。本节和第 3、4、5、6、6.6、6.9、6.13 节的 v0.23 改动是一个整体，冲突时以本节为准。

### 6.14.1 概念与总规则

> **v0.24**：源文件行新增副本（`copyState`、`storedPath` 等），转换读副本、输出目录默认 `<base>/output`、`DeleteSource` 同时删副本、`status` 筛选把复制中 / 复制失败算进去、预览白名单改写，全部见 6.15、6.16 和改写后的 6.14.7；本节其余规则不变。

- **源文件行（`ConvertSource`）**：转换页上的一行 = 一个源文件，存在新表 `convert_sources`，主键就是 `sourceId`。“添加了但还没转换”的文件也有一行，应用重启后仍在。
- **转换记录**：每次转换 = 一个 `type=convert` 的任务，`Task.sourceId` 指向它的源文件行。**分组只看 `tasks.source_id`，不按 `inputPaths[0]` 做字符串匹配**。
- **记录在任务中心和转换页之间是共享的同一条任务**：任务中心的“隐藏已结束”只设 `hiddenInTaskCenter`，**不删记录**（对所有任务类型都一样，不按类型区分）；**转换记录的真删只在转换页**（`DeleteRecords` / `DeleteSource`），**永远不删源文件**；没有回收站。非转换任务的真删用任务中心每行的“移除”（`TaskService.Remove`），`Remove` 拒绝转换任务。
- **只收 id、不收路径（架构师硬要求 5）**：存在性检查、预览、缩略图、用系统程序打开、打开所在文件夹（`RevealSource` / v0.23.1 的 `RevealRecord`）、取一行（v0.23.1 的 `GetSource`）、删除、取消隐藏，参数只有任务 id（加 `which`）或 `sourceId`；后端从表里取登记的路径，**不接受前端传来的路径**。唯一收路径的入口是 `AddSources`（登记新文件）和兼容保留的 `Submit(inputs …)`。
  - **唯一的例外（v0.23.2）**：`DeleteRecords` / `DeleteSource` 返回 `failures` 后，转换页可以取**第一条 `path` 非空**的失败项，用它的 `path` 调旧的 `SystemService.RevealInFolder(path)`，打开那个没删掉的文件所在的文件夹（例如“文件正在被使用，没有删除”之后让用户自己去处理）。原因：记录已经删了，没有 id 可用；`RevealInFolder` 只在文件管理器里打开文件夹，**不读、不改、不删文件**。`path` 为空的失败项（`still_running`，记录没删，仍可用 id 接口）不能这样用。**v0.23.3**：这些路径在**同一次运行里、该删除调用之后 10 分钟内**可以用 `RevealInFolder` 打开（后端把记录登记的输出路径本身临时放进 6.8 的放行范围，不受 `defaultOutputDir` 限制，见 6.8）；超过 10 分钟或应用重启后按 6.8 原来的规则判断（通常是 `INVALID_ARGUMENT`），文件已被移走则 `NOT_FOUND`。
- **旧类型**（6.10 确认项 ⑧ 的“保留但不再产生”的类型）的 id 在本节所有接口里一律按不存在处理（`NOT_FOUND`，`reason=record`）。
- 本节**没有新增错误码**；新增的是 2.2 里 `NOT_FOUND` / `UNSUPPORTED` 在本节接口上的 `reason=` 取值（`record`、`file`、`no_app`、`format`）。本节接口都要等启动完成，之前调用返回 `INTERNAL`（同 6.9）。

### 6.14.2 数据结构

```go
// Task 新增字段（完整定义见第 3 节）
SourceID           string      `json:"sourceId,omitempty"`  // 只有 convert 任务有；指向 convert_sources.id
HiddenInTaskCenter bool        `json:"hiddenInTaskCenter"`  // 始终输出；true = 在任务中心隐藏（转换页照常显示）
Reconverting       bool            `json:"reconverting"`                 // v0.24：始终输出；true = 正在原地重转（6.17）
LastReconvertError *ReconvertError `json:"lastReconvertError,omitempty"` // v0.24：最近一次重转失败的信息（6.17.2）
Result             *TaskResult `json:"result,omitempty"`    // 只有成功的 convert 任务有；探测失败也可能没有。v0.24：重转中（reconverting=true）仍是上一次的 result，重转失败 / 取消后原样恢复（6.17）

type TaskResult struct {           // 完成时探测输出文件写入（6.14.6），落库在 tasks.result（JSON）
    SizeBytes        int64   `json:"sizeBytes"`                  // os.Stat 的大小
    DurationSec      float64 `json:"durationSec,omitempty"`      // ffprobe format.duration
    Width            int     `json:"width,omitempty"`            // 显示尺寸（按 rotation 交换，同 6.7）；纯音频省略
    Height           int     `json:"height,omitempty"`
    AudioBitrateKbps int     `json:"audioBitrateKbps,omitempty"` // 第一条音频流的码率（kbit/s，四舍五入）；读不到时纯音频文件用 format 码率，仍读不到省略
    Warnings         []string `json:"warnings,omitempty"`        // v0.24：成功但值得提醒的情况，机器码（6.14.6），目前只有 "short_output"；没有时省略
}

// convert 任务的 params JSON（6.9）新增三个快照字段，提交时写入，之后不再变（预设改名、改参数、被删都不影响；原地重试也不动它们）：
// { input, options, outputDir, presetId, presetName, paramsSummary }
//   presetId      提交时选的预设 id（内置或用户预设），自定义参数为 ""
//   presetName    提交时该预设的名称快照，自定义参数为 ""（前端显示“自定义”）
//   paramsSummary 给人看的参数摘要快照，不含容器名，如 "H.264 · 1080p"，规则见 6.14.5；永远非空；前端只显示、不解析
// v0.23 之前的旧任务没有这三个键，前端按 "" 处理（摘要退回显示 title）。
// v0.23.1 写死的取值规则（前端据此决定记录那一行怎么显示）：
//   - 预设记录（内置预设；用户预设同样）：presetId、presetName 非空，paramsSummary 有值；
//   - 自定义参数记录：presetId、presetName 都是 ""（键存在，值为空串），paramsSummary 有值；
//   - v0.23 之前的旧记录：三个键都没有（等同 ""），前端退回显示 title。
//   没有 presetId:"custom" 之类的特殊值；兼容的 Submit(inputs …) 按自定义参数处理。

type ConvertSource struct {
    SourceID       string     `json:"sourceId"`        // ULID
    Path           string     `json:"path"`            // 规范化后的绝对路径（paths.Normalize）
    Name           string     `json:"name"`            // 文件名（含扩展名）
    AddedAt        int64      `json:"addedAt"`         // 第一次添加的时间（Unix 毫秒）
    LastActivityAt int64      `json:"lastActivityAt"`  // 最近一次添加 / 提交转换的时间，列表按它倒序
    Media          *MediaInfo `json:"media,omitempty"` // 可选的媒体信息引用：按 path_key 关联 media 表，见下
}

type ConvertSourceEntry struct {        // ListSources / SearchSources 的一项
    Source         ConvertSource `json:"source"`
    Records        []Task        `json:"records"`        // 该源文件的转换记录，按 createdAt 倒序、id 倒序，最多 recordLimit 条；没有时是 []
    RecordCount    int64         `json:"recordCount"`    // 该源文件的转换记录总数（不受 recordLimit 限制）
    NameMatched    bool          `json:"nameMatched,omitempty"`    // 只有 SearchSources：源文件名命中
    MatchedTaskIDs []string      `json:"matchedTaskIds,omitempty"` // 只有 SearchSources：输出文件名命中的记录 id（最多 200 个）
}

type ConvertSourceFilter struct {
    Limit       int `json:"limit"`       // 源文件行数，默认 50，最大 200
    Offset      int `json:"offset"`
    RecordLimit int `json:"recordLimit"` // 每行内嵌的记录数，默认 20，最大 100
    Status      string `json:"status,omitempty"` // v0.23.1，可省略："" 全部行；"active" 至少有一条 queued / running 记录的行；"failed" 至少有一条 failed / interrupted 记录的行（canceled 不算）；其他值 INVALID_ARGUMENT。只筛行，每行内嵌的记录和 recordCount 不按状态筛。v0.24：active 另含副本 copying 的行，failed 另含副本 failed 的行，副本 canceled 的行不算（6.15.6）
}
type ConvertSearchFilter struct {
    Keyword     string `json:"keyword"`  // 去首尾空白后 1~100 个字符
    Limit       int    `json:"limit"`    // 同上
    Offset      int    `json:"offset"`
    RecordLimit int    `json:"recordLimit"`
    Status      string `json:"status,omitempty"` // v0.23.2，可省略：与 ConvertSourceFilter.status 完全相同（"" | "active" | "failed"，canceled 不算失败，其他值 INVALID_ARGUMENT），与 keyword 是 AND；只筛行，不筛每行内嵌的记录和 recordCount
}
type ConvertSourcePage struct {
    Items []ConvertSourceEntry `json:"items"` // 无结果时是 []
    Total int64                `json:"total"` // 符合条件的源文件行总数
}

type AddSourceResult struct {           // 与 AddSources 入参一一对应
    Path    string         `json:"path"`             // 入参原样
    Source  *ConvertSource `json:"source,omitempty"` // 成功时有
    Existed bool           `json:"existed"`          // true = 已有这一行（同 path_key），只更新了 lastActivityAt
    Error   *AppError      `json:"error,omitempty"`  // 单个失败时有：NOT_FOUND（reason=file）/ INVALID_ARGUMENT / IO_ERROR
}

type ConvertSubmitRequest struct {
    SourceIDs []string       `json:"sourceIds"` // 1~50 个
    Options   ConvertOptions `json:"options"`   // 实际使用的参数（以它为准，不从预设重新读取）
    OutputDir string         `json:"outputDir"` // 同 Submit：空 = 实际输出目录（v0.24，6.15.2；不再有“源文件所在文件夹”）
    PresetID  string         `json:"presetId"`  // 可空；非空时后端读出它当时的名称写进 presetName 快照
}

type TaskPathCheck struct {             // 与 CheckPaths 入参一一对应
    TaskID       string `json:"taskId"`
    Found        bool   `json:"found"`        // false = 任务不存在 / 旧类型；此时两个 exists 都是 false
    InputExists  bool   `json:"inputExists"`
    OutputExists bool   `json:"outputExists"`
    ReconvertMode  string `json:"reconvertMode"`  // v0.24.1：现在调 Reconvert（不带参数）会怎么做（6.17.1）："replace" | "regenerate" | ""（不能重转）
    ReconvertBlock string `json:"reconvertBlock"` // v0.24.1：reconvertMode="" 时的原因：invalid_state | copy_not_ready | source_missing | output_moved，否则 ""；非 convert 任务是 invalid_state，found=false 时是 ""
}
type SourcePathCheck struct {           // 与 CheckSources 入参一一对应
    SourceID string `json:"sourceId"`
    Found    bool   `json:"found"`
    Exists   bool   `json:"exists"`
}

type DeleteResult struct {
    DeletedTaskIDs   []string        `json:"deletedTaskIds"`   // 实际删掉的记录；没有时是 []
    DeletedSourceIDs []string        `json:"deletedSourceIds"` // 只有 DeleteSource 可能非空
    DeletedFiles     int             `json:"deletedFiles"`     // 实际删掉的输出文件个数（不含 .part 残留）
    Failures         []DeleteFailure `json:"failures"`         // 没删成的文件 / 记录；没有时是 []
}
type DeleteFailure struct {
    TaskID  string `json:"taskId"`
    Path    string `json:"path,omitempty"` // 文件类失败（in_use / permission / not_task_output / io）时是那个输出文件的绝对路径；still_running 时为空（JSON 里省略）。v0.23.2 / v0.23.3：转换页可以拿它调 RevealInFolder（删除调用后 10 分钟内有效），见 6.14.1
    Reason  string `json:"reason"`         // 固定枚举，见 6.14.4
    Message string `json:"message"`        // 固定中文文案，见 6.14.4
}
```

- **`ConvertSource.media`（v0.23.4 起持久化）**：`convert_sources.media` 存这一行的完整探测结果（`MediaInfo`：含 `hasVideo` / `hasAudio` / `sampleRate` / `channels` / `container` / `fps` / `rotation` / `streams` / `videoCodecName` / `audioCodecName`；**不含** `thumbUrl`、`error`，`id` 为空），`media_fp` 存探测时文件的指纹 `<大小>:<修改时间纳秒>`。写入：`AddSources`（顺带探测）、`MediaService.Probe` 成功（同一 `path_key` 的行）、列表 / `GetSource` / `GetSourcePreviewURL` 时指纹不一致且文件还在（懒探测）。文件不变不重探；`PROBE_FAILED` 也记下指纹（`media` 为空），文件不变不再重探；转换组件没就绪、无权限、超时等暂时性错误不记，下次再试（v0.24.3：转换组件正在检测时先等检测结果再探测，启动时的第一次列表也能补上）。**持久化结果里 `hasVideo` / `hasAudio` 可靠**，前端可以直接用来显示“没有声音”和冲突预检。没有持久化结果时退回按 `path_key` 关联 `media` 表（v0.23 的做法；`hasVideo` / `hasAudio` 按 `videoCodec` / `audioCodec` 是否为空推出，没有采样率、声道和流信息）；都没有时省略，前端按“正在读取…”处理。
  - v0.23（已被取代）：只关联 `media` 表，`hasVideo` / `hasAudio` 恒为 `false`，重启后音频行没有采样率和声道、无声视频丢了“没有声音”（走查 G3）。
- **路径 / 名称规范化**：`path` 走 `paths.Normalize`（`filepath.Clean` + 绝对路径）；`path_key` 与 `media.path_key` 同一函数（Windows / macOS 小写）。`name_key` / `output_name_key` = `strings.ToLower(filepath.Base(path))`（Go 的 Unicode 小写，不用 SQLite 的 `lower()`，后者只管 ASCII）。本版不做 Unicode 规范化（NFC / NFD），**第二版**再考虑。

### 6.14.3 接口

```go
// ConvertService（v0.23 新增；ListPresets / SavePreset / DeletePreset 不变）
AddSources(paths []string) ([]AddSourceResult, error)
ListSources(filter ConvertSourceFilter) (ConvertSourcePage, error)
GetSource(sourceID string) (ConvertSourceEntry, error)   // v0.23.1：一行，与 ListSources 的一项完全相同（任务中心“在转换页查看”）
ListSourceRecords(sourceID string, limit, offset int) (TaskPage, error)
SearchSources(filter ConvertSearchFilter) (ConvertSourcePage, error)
CheckSources(sourceIDs []string) ([]SourcePathCheck, error)
PreviewOutputName(sourceID string, opts ConvertOptions, outputDir string) (string, error)
SubmitSources(req ConvertSubmitRequest) (ConvertSubmitResult, error) // v0.24：返回 {tasks, skipped}，副本没就绪的行跳过（6.15.4 第 6 条）
Submit(inputs []string, opts ConvertOptions, outputDir string) ([]Task, error) // 保留（兼容），按路径自动找到 / 创建源文件行
Reconvert(req ReconvertRequest) (Task, error)          // v0.24：原地重转，见 6.17（取代“新增一条”）
DeleteRecords(taskIDs []string, deleteOutputs bool) (DeleteResult, error)
DeleteSource(sourceID string, deleteOutputs bool) (DeleteResult, error)
GetSourcePreviewURL(sourceID string) (PreviewURL, error)
OpenSourceWithSystem(sourceID string) error
RevealSource(sourceID string) error
RevealRecord(taskID string) error        // v0.23.1：在文件管理器里显示转换记录的输出文件（转换页不再用 RevealInFolder）
GetRecordThumbnail(taskID string) (string, error)
GetSourceThumbnail(sourceID string) (string, error)

// TaskService（v0.23 新增 / 变更）
List(filter TaskFilter) (TaskPage, error)          // 变更：默认不含 hiddenInTaskCenter=true 的任务（TaskFilter.includeHidden）
Retry(id string) (Task, error)                     // 变更：原地重试，复用任务 id（6.6）
HideFinishedInTaskCenter() (int64, error)          // 新增：任务中心“隐藏已结束”
UnhideInTaskCenter(ids []string) error             // 新增：任务中心“取消隐藏”
ClearFinished() error                              // 变更：已废弃，等同 HideFinishedInTaskCenter，不再删除任何记录
Remove(ids []string, deleteOutput bool) error      // 变更：ids 里有 convert 任务时整体 INVALID_ARGUMENT
CheckPaths(taskIDs []string) ([]TaskPathCheck, error)
GetPreviewURL(taskID string, which string) (PreviewURL, error)   // which = "input" | "output"
OpenWithSystem(taskID string, which string) error                // which = "input" | "output"
```

逐个说明（“事件”一栏没写的就是不发事件）：

| 接口 | 行为 | 错误码 | 事件 |
|---|---|---|---|
| `AddSources(paths)` | 1~500 个（前端按约 50 个一批调用）。逐个 `paths.Normalize`、`os.Stat`：必须是普通文件。同 `path_key` 已有行 → `existed=true`，只把 `lastActivityAt` 设为现在（行移到最上面，`addedAt` 不变）；否则新建一行（`addedAt = lastActivityAt = 现在`）。同一次调用里重复的路径落到同一行。**v0.23.4：登记后顺带探测并持久化 `media`**（同一行只探测一次，文件没变不重探，最多 4 个并发、单个 15 秒，见 6.14.2），返回的 `source.media` 就是结果；探测失败的文件照样有行（`media` 省略）。单个失败放进该项 `error`，不影响其他项 | 整体：空列表或超过 500 个 `INVALID_ARGUMENT`；数据库失败 `INTERNAL`。单项：不存在 `NOT_FOUND`（`reason=file`），相对路径 / 目录 / 非普通文件 `INVALID_ARGUMENT`，无权限读取信息 `IO_ERROR` | 无 |
| `ListSources(filter)` | 按 `lastActivityAt` 倒序、`sourceId` 倒序分页列出**全部**源文件行（含没有记录的），每行内嵌最新的 `recordLimit` 条记录和 `recordCount`；记录里进行中的任务带实时进度（同 `List`）；**`hiddenInTaskCenter` 不影响这里**。**v0.23.1 `filter.status`**（可省略）：`""` 全部行（同前）；`"active"` 只返回至少有一条 `queued` / `running` 记录的行；`"failed"` 只返回至少有一条 `failed` / `interrupted` 记录的行（**`canceled` 不算**）。在 `tasks` 上用 `EXISTS` 子查询筛（只看 `type='convert'`，走 `idx_tasks_source_status`）；**分页和排序不变**，`total` 是筛选后的行数；**每行内嵌的最新记录和 `recordCount` 不按状态筛**（仍是这一行的全部记录） | 参数越界、`status` 不是这三个值 `INVALID_ARGUMENT` | 无 |
| `ListSourceRecords(sourceId, limit, offset)` | 某一行的更多记录（“展开更多”），`limit` 默认 50 最大 200，排序同上，返回 `TaskPage` | 行不存在 `NOT_FOUND`（`reason=record`）；参数越界 `INVALID_ARGUMENT` | 无 |
| `SearchSources(filter)` | 文件名搜索，覆盖全部记录：源文件名命中（`nameMatched`），或该行任一记录的输出文件名命中（`matchedTaskIds`）的行都返回；分页和每行内嵌规则同 `ListSources`。匹配方式见 6.14.5。**v0.23.2 `filter.status`**（可省略）：与 `ListSources` 的 `status` 完全相同（`""` / `"active"` / `"failed"`，`canceled` 不算失败），与关键字是 **AND**（两个条件都满足的行才返回），复用同一个 `EXISTS` 子查询；分页、排序不变，`total` 是筛选后的行数；每行内嵌的记录、`recordCount`、`nameMatched` / `matchedTaskIds` 不按状态筛 | 关键字为空 / 只有空白 / 超过 100 字 `INVALID_ARGUMENT`；`status` 不是这三个值 `INVALID_ARGUMENT` | 无 |
| `CheckSources(sourceIds)` | 1~500 个，结果一一对应：`exists` = 登记的路径现在 `os.Stat` 是普通文件；只有“不存在”算 `false`，其他 stat 错误（如无权限）算 `true`，交给后续操作报错 | 空或超过 500 `INVALID_ARGUMENT`；不存在的 id 不报错（`found=false`） | 无 |
| `PreviewOutputName(sourceId, opts, outputDir)` | “将保存为”的提示：按 6.14.5 的命名规则算出此刻会用的完整输出路径，**不占位**，真正提交时可能不同 | 行不存在 `NOT_FOUND`（`reason=record`）；参数 / 目录不合法 `INVALID_ARGUMENT` | 无 |
| `SubmitSources(req)` | 等同 `Submit`（6.9 全部规则：先整体校验再提交、最多 50 个、输出目录解析、不回滚已提交），区别只是输入来自 `convert_sources` 登记的路径：每个任务写 `sourceId`、`params.presetId / presetName / paramsSummary` 快照，并**在提交时定名并占位**（6.14.5）；被提交的行 `lastActivityAt` = 现在 | 6.9 的全部码；`sourceIds` 为空 / 超过 50 `INVALID_ARGUMENT`；任一 `sourceId` 不存在 `NOT_FOUND`（`reason=record`，整体不提交）；`presetId` 非空但不存在 `NOT_FOUND`（`reason=record`）；源文件本身的问题（已不在、探测失败、与参数不兼容）同 6.9，`detail` 第一行是文件路径、没有 reason 行 | 每个任务 `task:created` |
| `Submit(inputs, opts, outputDir)` | 兼容保留：先按 `path_key` 找到或创建源文件行（同 `AddSources`），再走 `SubmitSources` 的同一流程；`presetId` / `presetName` 为空，`paramsSummary` 照常生成 | 同 6.9 | 同上 |
| `Reconvert(req)` | **v0.24 起改为在同一条记录上原地重转，规则全部见 6.17**（取代 v0.23 / v0.23.1 的“新增一条记录、v1 转换页没有入口”）：只用于 `succeeded`，参数可选、只能同格式，重转中旧结果和旧文件照常可用，成功后原子替换；失败 / 取消恢复成 `succeeded` | 见 6.17.1（`TASK_CONFLICT` `reason=invalid_state` / `output_moved`、`NOT_FOUND` `reason=file`、`INVALID_ARGUMENT` `reason=format_change` 等） | `task:status`（`queued`、`reconverting: true`）……终态 `task:status` 带 `reconvertOutcome`；不发 `task:created`（6.17.3、6.17.5） |
| `DeleteRecords(taskIds, deleteOutputs)` | 删除转换记录，流程见 6.14.4 | 空或超过 500 `INVALID_ARGUMENT`；有不是 `convert` 的任务整体 `INVALID_ARGUMENT`（什么都不做）；有旧类型 id 整体 `NOT_FOUND`；不存在的 id 忽略；**文件没删成不是错误**，放进 `failures` | 进行中的先取消：各自的 `task:status`（`canceled`）；删完一次 `task:removed { ids }` |
| `DeleteSource(sourceId, deleteOutputs)` | 删掉这一行的全部转换记录（同 `DeleteRecords`），全部删掉后再删 `convert_sources` 行；**不删源文件**。**v0.24**：同时删我们在 `uploads` 里为这一行做的副本（引用计数归零时，6.15.7）；**永远不删用户的原文件**；输出文件只有 `deleteOutputs=true` 时才删 | 行不存在 `NOT_FOUND`（`reason=record`） | 同上 |
| `GetSourcePreviewURL(sourceId)` | 用登记的路径登记到 6.13 的 **`convert` 登记表**，返回 `PreviewURL`；扩展名白名单见 6.14.7；**v0.23.4：再按这一行的 `media`（指纹不一致时先重探）的编码挡一层**，名单见 6.14.7 | 行不存在 `NOT_FOUND`（`reason=record`）；文件不在 / 不是普通文件 `NOT_FOUND`（`reason=file`）；扩展名不在白名单、**或编码 WebView 解不了（v0.23.4）** `UNSUPPORTED`（`reason=format`，前端显示“无法在应用内播放”） | 无 |
| `OpenSourceWithSystem(sourceId)` | 用系统默认程序打开**用户的原文件**（v0.24.4：`originalPath`，不打开副本；规则其余同 `OpenWithSystem`） | 行不存在 `NOT_FOUND`（`reason=record`）；原文件不在 `NOT_FOUND`（`reason=file`，message `原文件不存在，无法打开。`）；扩展名不在白名单 `UNSUPPORTED`（`reason=format`） | 无 |
| `GetSource(sourceId)`（v0.23.1） | 返回一行 `ConvertSourceEntry`，与 `ListSources` 的一项**完全相同**：内嵌最新 20 条记录（`recordLimit` 的默认值，不能调）和 `recordCount`，记录里进行中的任务带实时进度；不带 `nameMatched` / `matchedTaskIds`。任务中心“在转换页查看”用：前端拿任务的 `sourceId` 调它，把这一行置顶、展开并高亮那条记录（记录不在内嵌的 20 条里时用 `ListSourceRecords` 继续翻） | 行不存在 `NOT_FOUND`（`reason=record`） | 无 |
| `RevealSource(sourceId)` | 在文件管理器里显示**用户的原文件**（v0.24.4：打开 `originalPath` 所在文件夹并选中原文件，不打开 `uploads` 里的副本）。平台命令、引号规则、启动失败 `PROCESS_FAILED` 全部同 6.8；路径来自表，不走 6.8 的范围白名单 | 行不存在 `NOT_FOUND`（`reason=record`）；原文件不在 `NOT_FOUND`（`reason=file`，message `原文件不存在，无法打开。`），不退回副本；Windows 路径含双引号 `INVALID_ARGUMENT` | 无 |
| `RevealRecord(taskId)`（v0.23.1） | 在文件管理器里显示转换记录的输出文件（“打开所在文件夹”），平台命令、引号规则、启动失败 `PROCESS_FAILED` 全部同 6.8（复用同一实现，Windows 走 `reveal_windows.go`）；路径来自记录，不走 6.8 的范围白名单。转换页不再用 `RevealInFolder(path)` | 不存在 / 旧类型 `NOT_FOUND`（`reason=record`）；不是 `convert` 任务 `INVALID_ARGUMENT`；不是 `succeeded`（没有输出）、输出文件不在、不是普通文件、是符号链接 `NOT_FOUND`（`reason=file`）；Windows 路径含双引号 `INVALID_ARGUMENT` | 无 |
| `GetRecordThumbnail(taskId)` / `GetSourceThumbnail(sourceId)` | 缩略图，规则见 6.14.10：返回 `data:image/jpeg;base64,...` 字符串，首次调用时生成并缓存，文件修改时间或大小变了自动重新生成；路径由后端从记录取 | 不存在 `NOT_FOUND`（`reason=record`）；文件不在 `NOT_FOUND`（`reason=file`）；做不出缩略图（如纯音频）`UNSUPPORTED`（`reason=format`）；详见 6.14.10 | 无 |
| `TaskService.List(filter)` | `TaskFilter` 新增 `includeHidden`（默认 `false`）：**默认不返回 `hiddenInTaskCenter=true` 的任务**，`total` 也不算它们；任务中心不需要传它。`ListActive` 不受影响（进行中的任务永远不是隐藏的，见 `Retry`） | 同前 | 无 |
| `TaskService.HideFinishedInTaskCenter()` | **不按类型区分**：把所有类型、所有已结束（`succeeded` / `failed` / `canceled` / `interrupted`）且未隐藏的任务设 `hiddenInTaskCenter=true`，返回本次隐藏的条数；**不删记录、不删日志、不删文件，`version` 不变**（这是任务中心的显示开关，不是任务状态）。任务中心随后自己重新 `List`。隐藏的非转换任务仍在库里，要真删用 `Remove` | 数据库失败 `INTERNAL` | 无（转换页不受影响，不需要通知） |
| `TaskService.UnhideInTaskCenter(ids)` | 取消隐藏：1~500 个 id，把其中 `hiddenInTaskCenter=true` 的清成 `false`、`version` +1、各发一次 `task:status`（当前 `status` 不变，带 `hiddenInTaskCenter: false`）。**幂等**：本来就没隐藏的 id 什么都不做（不改 version、不发事件），不算错误。**任何类型都可以取消隐藏**（含转换记录）。先整体校验再改，校验失败什么都不改 | 空或超过 500 `INVALID_ARGUMENT`；任一 id 不存在或是旧类型 `NOT_FOUND`（`reason=record`，整体不改）；数据库失败 `INTERNAL` | 每个真正被取消隐藏的任务一条 `task:status` |
| `TaskService.ClearFinished()` | **已废弃**，行为改成和 `HideFinishedInTaskCenter` 完全一样（不再删除），只为不让旧前端误删共享的记录；前端改调 `HideFinishedInTaskCenter` 后可在下个大版本删除 | 同上 | 无 |
| `TaskService.Remove(ids, deleteOutput)` | 任务中心每行的“移除”，**非转换任务真删的唯一入口**，行为不变（6.6）；**ids 里有 `convert` 任务时整体 `INVALID_ARGUMENT`**（message `转换记录请在格式转换页删除`），什么都不删——真删只在转换页 | 新增上面这一条，其余同 6.6 | 同 6.6 |
| `TaskService.CheckPaths(taskIds)` | 1~500 个，结果一一对应。`inputExists` = `inputPaths[0]` 现在是普通文件；`outputExists` = 任务是 `succeeded`、`outputPath` 是绝对路径、`os.Lstat` 是普通文件（**不是符号链接**）。只有“不存在”算 `false`，其他 stat 错误算 `true`（同 `CheckSources`）；其余状态的 `outputExists` 一律 `false`（v0.24：`reconverting=true` 的记录按 `succeeded` 算）。**v0.24.1 新增 `reconvertMode` / `reconvertBlock`**，规则见 6.17.1 | 空或超过 500 `INVALID_ARGUMENT`；不存在 / 旧类型的 id 不报错（`found=false`） | 无 |
| `TaskService.GetPreviewURL(taskId, which)` | `which=input` 取 `inputPaths[0]`；`which=output` 取 `outputPath`，**只有 `succeeded` 的任务才有输出可预览**（v0.24：`reconverting=true` 的记录也可以，预览的是旧文件，6.17.4），且不能是符号链接。登记到 `convert` 登记表（6.13），扩展名白名单见 6.14.7；**v0.23.4：扩展名通过后探测这个文件（单个 15 秒），按编码再挡一层**（名单见 6.14.7；探测不到时不挡）。不限任务类型（非媒体文件会落到 `reason=format`） | `which` 不是这两个值 `INVALID_ARGUMENT`；任务不存在 / 旧类型 `NOT_FOUND`（`reason=record`）；不是 `succeeded`、路径为空、文件不在或不是普通文件 `NOT_FOUND`（`reason=file`）；扩展名不在白名单、**或编码 WebView 解不了（v0.23.4）** `UNSUPPORTED`（`reason=format`） | 无 |
| `TaskService.OpenWithSystem(taskId, which)` | 取路径同 `GetPreviewURL`，再用系统默认程序打开：Windows `ShellExecuteW`（`open` 动词，路径作为文件参数，不经过命令行拼接）；macOS `open <path>`；Linux `xdg-open <path>`。**只允许媒体扩展名**（6.14.7 的“系统打开白名单”），防止被注入后拿它当“运行任意程序”的入口。macOS / Linux 最多等命令 5 秒拿退出码，超过 5 秒还没退出按成功 | `which` 不合法 `INVALID_ARGUMENT`；任务不存在 `NOT_FOUND`（`reason=record`）；文件不在 `NOT_FOUND`（`reason=file`）；**系统没有能打开它的程序 `NOT_FOUND`（`reason=no_app`，message 固定 `没有找到能打开这个文件的程序`）**：Windows `SE_ERR_NOASSOC` / `SE_ERR_ASSOCINCOMPLETE`，macOS `open` 退出码非 0 且 stderr 含 `No application knows how to open`，Linux `xdg-open` 退出码 3 / 4；扩展名不在白名单 `UNSUPPORTED`（`reason=format`）；命令找不到或其他启动失败 `PROCESS_FAILED` | 无 |

- **`SubmitSources` 收 `sourceId`，而不是按路径自动找 / 建（取舍）**：前端在 `AddSources` 时已经拿到 `sourceId`，提交时传 id，后端从表里取路径，符合“只收 id”；也不会出现“同一个文件在两行”的歧义（路径大小写、`..`、符号链接）。旧 `Submit(paths)` 保留并按路径自动找 / 建，保证任何途径提交的 `convert` 任务都有 `sourceId`。
- **`ListSources` 内嵌子记录，而不是另查（取舍）**：一次往返拿到完整的一屏，分页单位是源文件行，**不会出现一行的记录被分页切成两半**、也不需要前端自己按 `sourceId` 拼；单行记录很多时只内嵌最新 `recordLimit` 条，更多的用 `ListSourceRecords`。代价是每页最多 200 × 100 条记录，默认值（50 行 × 20 条）下首屏约 1000 条以内。

### 6.14.4 删除（`DeleteRecords` / `DeleteSource`）

1. **校验**（见上表），通过后按 id 去重。
2. **进行中的先取消**：`queued` / `running` 的记录先按用户取消处理（与 `Cancel` 相同，发 `task:status` `canceled`），最多一共等 10 秒到达终态；10 秒内没停下来的记录**不删**，在 `failures` 里给一条 `reason=still_running`。`DeleteSource` 只要有一条没删掉，就保留这一行（`deletedSourceIds` 不含它）。
3. **删输出文件**（`deleteOutputs=true`，前端默认不勾选）：**只删 `succeeded` 记录登记的 `outputPath`**（v0.24：重转中的记录先取消、恢复成 `succeeded` 后按这条处理，6.17.6）（第 2 步刚被取消的不算成功，本来就没有最终文件）；安全条件同 6.6 `Remove`：绝对路径、所在目录及上级不含符号链接、本身不是符号链接、是普通文件、不等于任何输入路径、修改时间不早于 `startedAt − 3 秒`。文件已经不在：跳过，不计数也不算失败。不满足安全条件或删除失败：记录照删，文件留着，`failures` 里给一条。删除前先撤销 `convert` 登记表（6.13）里这些路径的 token。
4. **`.part` 残留**：不管 `deleteOutputs`，记录的 `outputPath` 对应的 `.part` 文件（`RunWithPart` 的命名）若还在、是普通文件且修改时间不早于 `startedAt − 3 秒`，一并删掉（这是应用自己的临时文件）；不计入 `deletedFiles`，失败只记日志。
5. **源文件永远不删**：`convert_sources.path`（原文件）指向的文件不碰；`inputPaths` 指向副本时也不在这一步删，副本只由 `DeleteSource` 在引用计数归零时删（v0.24，6.15.7）。
6. 在一个事务里删记录，再删日志文件（日志删不掉只记日志），发一次 `task:removed { ids }`。`DeleteSource` 最后删 `convert_sources` 行。

`DeleteFailure.reason` / `message`（固定枚举和文案，只追加；前端汇总成“有 N 个文件正在被使用，没有删除”之类的提示）：

| reason | message | 什么时候 |
|---|---|---|
| `in_use` | `文件正在被使用，没有删除` | Windows `ERROR_SHARING_VIOLATION`（32）/ `ERROR_LOCK_VIOLATION`（33）；Unix `EBUSY` / `ETXTBSY` |
| `permission` | `没有权限删除这个文件` | 权限不足、只读 |
| `not_task_output` | `文件已被替换或移动，没有删除` | 不满足第 3 步的安全条件（是符号链接、上级目录有符号链接、不是普通文件、修改时间早于任务开始、等于输入路径） |
| `io` | `删除文件失败` | 其他错误（详细原因只进应用日志，不进 message） |
| `still_running` | `任务还没停下来，没有删除这条记录` | 第 2 步 10 秒内没到终态（`path` 为空） |

这和 6.6 的 `Remove` 不同：`Remove` 遇到进行中的任务整体 `TASK_CONFLICT`、文件删不掉返回 `IO_ERROR`；转换页删除是“先取消、尽量删、把没删成的列出来”，调用本身成功。

### 6.14.5 输出命名、占位与参数摘要

- **期望名**：`<输出目录>/<源文件名去扩展名>.<容器扩展名>`（同 6.9）。v0.24：“源文件名”是**原文件名**（`ConvertSource.name`），不是 `uploads` 里副本的名字（两者相同，但副本在 `uploads/<sourceId>/` 下，写死以免实现取错）。
- **提交时定名并占位**：`SubmitSources` / `Submit` / `Reconvert`（v0.24：`Reconvert` 不定新名，始终沿用原输出路径，见 6.17.3）在**落库之前**选出最终名并占位（占位人 = 任务 id），一直占到任务进入终态。候选名依次是期望名、`<名> (1).<扩展名>`、`<名> (2).<扩展名>`……（**括号前有一个半角空格，只有 `convert` 用这种格式**），取第一个同时满足以下条件的：磁盘上不存在（`os.Lstat`，含悬空链接）、对应的 `.part` 文件不存在、没有被**任何未结束任务**占位、不等于任何输入路径。同一次提交里的多个文件互相也算占位（`a.mkv`、`a.mov` 都转 MP4 → `a.mp4`、`a (1).mp4`）。比较按 `path_key` 规则（Windows / macOS 不区分大小写）。最大编号沿用现有实现（v0.23.1 说明：现有 `namer` 没有上限，一直往后找到空名为止；99 只是 `.part` 遗留清理精确拼候选名的范围）。
- **两种重名格式各自用在哪里（v0.23 架构师定）**：

  | 格式 | 例子 | 用在 |
  |---|---|---|
  | 带空格 ` (n)` | `a (1).mp4`，`.part` 是 `a (1).part.mp4` | **只有 `convert`**（`SubmitSources` / `Submit` / `Reconvert` 的提交时定名、原地重试的顺延、运行时顺延） |
  | 无空格 `(n)` | `a(1).mp4`，`.part` 是 `a(1).part.mp4` | 其他所有走 `RunWithPart` 的类型：**v0.23.5 起只有文档转 PDF**（6.12.3；剪辑导出已移除）；以及按这个格式精确拼候选名的 `.part` 遗留清理（6.11.3 / 6.12.3，编号 1~99），**全部保持现状不变** |

  实现上 `namer` 按任务类型（或调用方传入的后缀格式）选择格式，默认仍是无空格。`.part` 遗留清理只针对 `edit_export` / `office_pdf` 的 `outputPath`，不处理 `convert`，所以不受这次改动影响。v0.22 及之前产生的转换输出（`a(1).mp4`）不改名；它们照样占名字（磁盘上存在就跳过）。
- **现状差异（实现须改）**：当前代码（`internal/task/part.go` 的 `namer.reserve`）只在 `RunWithPart` 开始运行时才占位，排队中的任务不占名字，两个排队中的同名任务会显示同一个 `outputPath`，直到运行时才分开。v0.23 要求转换任务在提交时就占位；`RunWithPart` 运行时直接用已占的名字，只有这期间磁盘上被别的程序建了同名文件才顺延到下一个可用名，新名字随 `running` 的 `task:status.outputPath` 告知前端。剪辑导出、Office 转 PDF 本版不要求改（可以一起改）。
- 应用重启后没有未结束的任务（6.6 启动时都变成 `interrupted`），所以占位表不需要持久化。
- **`params.paramsSummary`**：提交时由后端按 `options` 生成，之后不变，**原地重试不改**；`Reconvert` 换了参数且成功时换成新快照，失败 / 取消时恢复旧的（v0.24，6.17.5）。各段用 ` · `（空格、U+00B7、空格）连接，最长 80 字符。**v0.23.1：不含容器名**（容器由预设名或输出扩展名体现；否则会出现“MP4 1080p · MP4 · H.264 · 1080p”）：
  1. 视频（只对视频容器）：`h264` → `H.264`，`h265` → `H.265`，`vp9` → `VP9`，`copy` → `原画质`，`""` → `无画面`；`gif` 和音频容器不写这一段。**v0.23.2：显示名统一写法**，从不出现原样大写的 `HEVC` / `PRORES`：`hevc` / `libx265` → `H.265`，`libx264` → `H.264`，`prores*` → `ProRes`，`av1` / `libaom-av1` / `libsvtav1` → `AV1`，`libvpx-vp9` → `VP9`（硬件编码器后缀如 `_nvenc` 去掉后同样映射），其他未知编码去掉 `lib` 前缀后**全部大写**（v0.24.1：`foo` → `FOO`，`libfoo` → `FOO`；此前是首字母大写）。v0.24 扩充的 `videoCodec` 取值见 6.16.2（`mpeg2` 显示 `MPEG-2`）；图片容器不写这一段。
  2. 尺寸：只给高 → `<高>p`（`1080p`）；**只给宽（v0.23.1）：`3840` → `2160p`、`2560` → `1440p`、`1920` → `1080p`、`1280` → `720p`、`854` → `480p`，其他宽度 → `宽 <宽>`**；都给 → `<宽>×<高>`；都没给不写。
  3. 帧率 `fps > 0` → `<fps> fps`；码率：视频容器设了 `videoBitrate` → `<Mbps> Mbps`（保留 1 位小数），音频容器设了 `audioBitrate` → `<kbps> kbps`；`audioCodec=none` → `无声`。
  4. 裁剪（`trimStart > 0` 或 `trimEnd > 0`）→ `已裁剪`。
  5. **以上都没有时为 `默认参数`**（如不设任何参数的 WAV / FLAC），所以新记录的 `paramsSummary` 永远非空。

  例：`H.264 · 1080p`（内置 1080p 预设，只设了宽 1920）、`H.264 · 720p`、`原画质`（MKV 只换封装）、`192 kbps`（MP3）、`宽 480 · 10 fps · 已裁剪`（GIF）、`H.264 · 1920×1080`。**前端只显示、不解析**；生成规则以后可以微调，只影响之后新提交的记录（v0.23.1 之前按 v0.23 规则生成、带容器名的快照不改写）。旧记录没有这个键，前端退回显示 `title`。presetId / presetName / paramsSummary 三者的取值规则见 6.14.2（v0.23.1）。

### 6.14.6 结果信息、取消时的进度、原地重试

- **`result`**：`convert` 任务的最终文件改名到位（`Run` 成功返回）后、发终态事件**之前**，对最终输出 `os.Stat` 拿 `sizeBytes`，再跑一次 ffprobe（与 6.7 相同的门控和 `file:` 前缀，超时 30 秒，可被取消）补齐其余字段，写入 `tasks.result` 并随终态 `task:status` 发出。ffprobe 失败或超时：只有 `sizeBytes`；连 stat 都失败：没有 `result`；**都不影响任务成功**，只记日志。失败、取消、中断的任务没有 `result`。实现建议：Runner 可选实现 `ResultReporter`（`Result() *TaskResult`），管理器在 `Run` 返回 nil 后调用。其他任务类型本版不写 `result`。
- **结果警告 `result.warnings`（v0.24，走查 X1）**：输入文件被截断时，转换可能“成功”得到一个只有 1 秒的输出，用户毫无察觉。成功的 `convert` 任务在算 `result` 时顺带检查，命中就往 `warnings` 里加一个**机器码**（稳定字符串，前端按码映射文案；不认识的码忽略）。任务仍然是 `succeeded`，不改 `error`、不改进度。目前只有一个码：
  - **`short_output`**：输出时长**明显短于**预期时长。
    - **预期时长** = 输入时长 − 裁剪：设了 `trimEnd`（> 0）时是 `min(trimEnd, 输入时长) − trimStart`，否则 `输入时长 − trimStart`。输入时长取这次运行算进度用的那个探测时长（6.9 提交时探测的 `format.duration`，读的是实际读取路径，v0.24 即副本）。
    - **输出时长** = `result.durationSec`。
    - **判定**（两条都满足才算，避免短文件、编码器首尾几帧的正常误差误报）：`输出时长 < 预期时长 × 0.9` **且** `预期时长 − 输出时长 > 2 秒`。例：预期 60 秒、输出 1 秒 → 命中；预期 3 秒、输出 2 秒 → 不命中（只差 1 秒）；预期 100 秒、输出 95 秒 → 不命中（95%）。
    - **不检查**（不加这个码）：输入时长未知或 ≤ 0、预期时长 ≤ 0、输出探测失败或没有时长、图片输出（6.16.5，单帧）。
    - **阈值由后端定义（以本条为准），前端只看码、不自己比较时长。**
    - **面向用户的文字（PM 已定）**：记录**仍是成功**；前端在这条记录上显示警告徽标 `时长偏短`，悬停文字 `转换结果比原文件短很多，原文件可能已损坏，请预览检查。` 后端同时在任务日志写一行 `[FFmpegFree] short_output: expected=<秒> actual=<秒>`（日志，不给用户看）。
  - `warnings` 随 `result` 一起落库（`tasks.result` JSON，不需要迁移）和随终态 `task:status` 发出；旧记录、v0.24 之前完成的任务没有这个字段。原地重试时随 `result` 一起清空（6.6），重新跑完再算。`warnings` 为空时省略（不发 `[]`）。
- **取消时的进度（v0.23 明确）**：任务被取消（`canceled`）时 **`progress` 保留取消那一刻最后一次计算出的值（0~1），不清零、不置 1**，并在终态落库（现有实现已经如此，本版写进契约并让终态事件带上它）。含义是“取消时大约转到了哪里”，**不代表有可用的部分结果**：转换类任务取消时 `.part` 已删除，没有输出文件。排队中被取消（从未运行，6.6 `NeverRanner`）是入队时的值 0；直播任务恒为 -1。`failed` / `interrupted`（应用退出时被停止）同样保留当时的值；崩溃恢复变成 `interrupted` 的记录是最后一次落库的值（进度不随时落库，通常是 0）。只有 `succeeded` 是 1。
- **原地重试与“又转一次”（产品经理已确认）**：规则见 6.6 `Retry`。失败、中断、**已取消**的记录，不管在转换页还是任务中心，都走同一个 `TaskService.Retry`（原地：**同一个任务 id、同一个输出名**，名字被占时才按 6.14.5 顺延），两边看到的记录条数始终一致。转换页上**已取消那一行的按钮文案仍是“重新转换”**，失败 / 中断行是“重试”，两者调的都是 `Retry`。**`Reconvert` 只用于已成功的记录**（v0.24 起在同一条记录上**原地**重转：同一输出路径，成功后才替换旧输出，失败 / 取消恢复成成功；转换页在成功的记录上有“重转”入口；见 6.17，取代“新增一条、v1 没有入口”）；对其他状态返回 `TASK_CONFLICT`（`reason=invalid_state`）。

### 6.14.7 预览与系统打开的扩展名白名单

**v0.24 改写**（随 6.16 的新格式扩充；v0.23 的旧列表见本节末尾）。应用内预览靠 WebView（Windows WebView2 = Chromium，macOS WKWebView = Safari 内核，Linux WebKitGTK）的 `<video>` / `<audio>` / `<img>`，**能不能播最终取决于平台和文件里的编码**，所以分三档：

| 档 | 扩展名 | 后端行为 | 前端行为 |
|---|---|---|---|
| **可以预览** | 视频 `mp4 m4v mov webm mkv ogv`；音频 `mp3 m4a m4r aac wav flac ogg opus`；图片 `gif webp png jpg jpeg bmp ico` | `GetPreviewURL` / `GetSourcePreviewURL` 返回 `/local/<token>`（6.13） | `<video>` / `<audio>` / `<img>` 加载；**触发 `error` 时（如 macOS 上的 mkv、webm、ogv、opus，或文件里是 WebView 不支持的编码）按下面“回退”处理**，不当成错误弹窗 |
| **不能预览，只能用系统程序打开** | 视频 `avi flv wmv mpg mpeg vob 3gp swf ts mts m2ts`；音频 `wma amr ape wv mmf mp2 aif aiff`；图片 `tif tiff tga` | 预览接口返回 `UNSUPPORTED`（`reason=format`）；`OpenWithSystem` / `OpenSourceWithSystem` 正常打开 | 显示“无法在应用内播放”和“**用系统程序打开**”按钮（调 `OpenWithSystem(taskId, which)` / `OpenSourceWithSystem(sourceId)`） |
| 其他 | 不在上面两档里的扩展名 | 预览和系统打开都 `UNSUPPORTED`（`reason=format`） | 只显示文件名 |

- **v0.24 的变化**：`avi`、`flv` **移出可预览档**（没有任何一个 WebView 能播，v0.23 里返回了 URL 但一定播放失败），改为只能用系统程序打开；新增可预览的 `ogv`、`m4r`、`opus`（v0.23 写的“第二版再做”提前）、`webp`、`png`、`jpg` / `jpeg`、`bmp`、`ico`；系统打开档新增 `vob swf mp2 wv mmf tif tiff tga` 和全部可预览的扩展名。
- **按编码再挡一层（v0.23.4，走查 G4）**：扩展名只看容器，ProRes 的 `.mov` 在 WebView 里是黑屏、时间照走、`<video>` 不报错。所以“可以预览”档的扩展名通过后，后端再按探测出的编码判断（名单只在后端 `internal/service/convert/playable.go` 一处，键是 ffprobe 的 `codec_name`）：
  - **有画面的文件**（封面图不算画面）看第一条视频流：`prores*`、`dnxhd` / `dnxhr`、`cfhd`、`hap`、`dxv`、`aic`、`avrp`、`ffv1`、`ffvhuff`、`huffyuv`、`utvideo`、`magicyuv`、`rawvideo`、`v210` / `v410` / `r210` / `r10k`、`qtrle`、`png`、`tiff`、`jpeg2000`、`mpeg1video`、`mpeg2video`、`msmpeg4*`、`wmv1~3`、`vc1`、`h261` / `h263` / `h263p`、`flv1`、`mjpeg` / `mjpegb`、`dvvideo`、`cinepak`、`svq1` / `svq3`、`rv10~40`、`indeo2~5`、`vp6*`。有画面时不看音频编码。
  - **只有声音的文件**看第一条音频流：`wmav1` / `wmav2` / `wmapro` / `wmalossless` / `wmavoice`、`ape`、`amr_nb` / `amr_wb`、`ac3` / `eac3`、`dts`、`truehd` / `mlp`、`mp1` / `mp2`、`wavpack`、`tta`、RealAudio、ATRAC、`qdm2` / `qdmc`、`nellymoser`、`speex`、`gsm`、常见 `adpcm_*`、`dsd_*`。
  - 命中返回 `UNSUPPORTED`（`reason=format`），和扩展名不在白名单是同一种结果；探测不到（转换组件没就绪、文件解析失败、超时）时不挡，交给前端兜底（`error`，或 `loadedmetadata` 后 `videoWidth === 0`，都按上面的“回退”处理）。`H.264`、`H.265`、`VP8` / `VP9`、`AV1`、`MPEG-4`（Part 2）、`Theora` 和 `AAC`、`MP3`、`FLAC`、`Opus`、`Vorbis`、`PCM` 不挡。“用系统程序打开”不受影响。
- **回退（前端）**：可预览档里加载失败 → 同“不能预览”档：提示“无法在应用内播放”，给“用系统程序打开”按钮；不重复请求 URL（6.13 第 8 条的 HEAD 重试只针对 404，不针对解码失败）。
- **系统打开白名单** = 前两档的并集（`OpenWithSystem`、`OpenSourceWithSystem`），仍然只放媒体扩展名，防止被注入后拿它当“运行任意程序”的入口；APE 等只能输入的格式在里面（PM 规则 4）。
- **缩略图**（6.14.10）：扩展名在系统打开白名单里才尝试生成，图片（含 `tif` / `tga` / `ico`）可以出缩略图；纯音频仍是 `UNSUPPORTED`（`reason=format`）。
- `EditService.GetPreviewURL` 的列表**不变**（剪辑时间线另有约束，见 6.11.4）。
- 6.13 第 7 条的 `Content-Type` 补上：`ogv → video/ogg`、`m4r → audio/mp4`、`bmp → image/bmp`、`ico → image/x-icon`（`opus`、`webp`、`png`、`jpg` 原来就有映射）。
- v0.23 的旧列表（作废）：预览 `mp4 mov avi mkv flv webm m4v mp3 wav aac m4a flac ogg gif`；系统打开另加 `opus wmv mpg mpeg ts mts m2ts 3gp ogv wma amr aiff ape`。

### 6.14.8 迁移 `0005_convert_records.sql` 与回填

迁移目录现有 `0001`~`0004`，本版新增 **`0005_convert_records.sql`**（已发布的迁移不改）：

```sql
CREATE TABLE convert_sources (
    id               TEXT PRIMARY KEY,          -- sourceId（ULID）
    path             TEXT NOT NULL,
    path_key         TEXT NOT NULL UNIQUE,      -- 与 media.path_key 同一规范化函数
    name             TEXT NOT NULL,
    name_key         TEXT NOT NULL,             -- Go strings.ToLower(basename)，用于搜索
    added_at         INTEGER NOT NULL,
    last_activity_at INTEGER NOT NULL
);
CREATE INDEX idx_convert_sources_activity ON convert_sources(last_activity_at DESC, id DESC);
CREATE INDEX idx_convert_sources_name_key ON convert_sources(name_key);

ALTER TABLE tasks ADD COLUMN source_id TEXT;                                   -- 可空；只有 convert 任务有
ALTER TABLE tasks ADD COLUMN hidden_in_task_center INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN result TEXT;                                      -- TaskResult JSON，可空
ALTER TABLE tasks ADD COLUMN output_name_key TEXT NOT NULL DEFAULT '';         -- Go strings.ToLower(basename(output_path))
CREATE INDEX idx_tasks_source_created ON tasks(source_id, created_at DESC) WHERE source_id IS NOT NULL;
CREATE INDEX idx_tasks_source_status ON tasks(source_id, status) WHERE source_id IS NOT NULL;   -- v0.23.1：ListSources 的 status 筛选
CREATE INDEX idx_tasks_convert_output_name ON tasks(output_name_key, source_id) WHERE type = 'convert';
CREATE INDEX idx_tasks_hidden_created ON tasks(hidden_in_task_center, created_at DESC);
```

- **默认值兼容**：新列都可空或有默认值，旧行 `source_id = NULL`、`hidden_in_task_center = 0`（任务中心照常显示）、`result = NULL`、`output_name_key = ''`。不加外键（与现有表一致），`source_id` 的一致性由删除流程（6.14.4）在同一事务里保证。迁移只加表、加列、加索引，**不改不删任何旧数据**。
- **回填在 Go 里做，不在 SQL 里做**：`path_key` 的规范化（Windows / macOS 小写、`filepath.Clean`）和 Unicode 小写 SQL 做不了，所以迁移之后由 `store.BackfillConvertSources` 回填：每次启动在迁移之后、`MarkInterrupted` 之后运行，**幂等**（只处理 `type='convert' AND source_id IS NULL` 的行和 `output_path != '' AND output_name_key = ''` 的行），每批 500 条一个事务。对每条旧 `convert` 任务：取 `inputPaths[0]` 规范化得到 `path_key`，没有对应行就建一行（`name` = 文件名，`added_at` = 这些任务里最早的 `created_at`，`last_activity_at` = 最晚的 `created_at`），再写 `source_id`；`inputPaths` 为空或 JSON 损坏的记录每条单独建一行（`path` 为空，`path_key` = `invalid:<任务 id>`，`name` 取 `title`），保证能在转换页看到并删除。同时补 `output_name_key`。
- **升级不丢记录**：回填失败只记日志、不阻止启动，下次启动再试；回填完成之前没有 `source_id` 的旧任务在任务中心照常显示，只是暂时不出现在转换页。降级到旧版本：旧版本看到更高的迁移号直接跳过，旧代码按列名读写，新列不影响；旧版本新建的 `convert` 任务没有 `source_id`，再次升级时由回填补上。
- **不新增 media 表的列**：`ConvertSource.media` 关联现有 `media` 表（6.14.2）。（v0.23.4：改为在 `convert_sources` 上持久化，见下。）
- **v0.23.4 迁移 `0006_convert_source_media.sql`**（只加列，不改不删旧数据）：

  ```sql
  ALTER TABLE convert_sources ADD COLUMN media TEXT;                      -- MediaInfo JSON（不含 thumbUrl / error，id 为空）；NULL = 没有可用结果
  ALTER TABLE convert_sources ADD COLUMN media_fp TEXT NOT NULL DEFAULT '';  -- 探测时的文件指纹 "<大小>:<修改时间纳秒>"；'' = 从没探测过
  ```

  **旧行不在启动时回填**（启动时转换组件可能还没检测完，而且大量文件一起探测会拖慢启动）：由 `ListSources` / `SearchSources` / `GetSource` 在文件还在、指纹不一致时懒探测补上（每页最多 4 个并发、单个 15 秒；v0.24.3：转换组件正在检测时先等检测结果，最多 15 秒），补上后落库，之后不再探测；文件已不在的旧行保留空值，仍退回关联 `media` 表。`media` 的 JSON 损坏时按“从没探测过”处理，下次重探。降级到旧版本：旧代码按列名读写，新列不影响。

### 6.14.9 搜索的匹配方式与索引

- **不区分大小写的子串匹配，只匹配文件名（basename），不匹配目录**：关键字去首尾空白后用 Go `strings.ToLower` 规范化，查询条件是 `instr(name_key, ?) > 0`（源文件名）或存在 `type='convert'` 的记录满足 `instr(output_name_key, ?) > 0`（输出文件名）。用 `instr` 而不是 `LIKE`，免去 `%` / `_` 转义。
- `output_name_key` 在 `outputPath` 每次变化时同步更新（提交占位、运行时顺延、原地重试重新占位、`ClearOutputPath` 时置 `''`）；`convert_sources.name_key` 随行创建。
- **索引的作用要说清楚**：子串匹配（前面不固定）用不上 B-tree 的范围查找，SQLite 会**扫描覆盖索引**（`idx_convert_sources_name_key`、`idx_tasks_convert_output_name` 已包含查询需要的列，不回表），比扫全表快很多，但仍是线性的。预期规模（几万条记录）下足够；记录数到十万级以上再考虑 SQLite FTS5 的 `trigram` 分词器（3 个字符以下的关键字仍然回退扫描），本版不做。
- 结果按源文件行的 `lastActivityAt` 倒序分页（`limit` / `offset`，与 `TaskFilter` 一致），不用游标。

### 6.14.10 缩略图（`GetRecordThumbnail` / `GetSourceThumbnail`）

- **只收 id**：`GetRecordThumbnail(taskId)` 取转换记录的输出文件（`outputPath`），`GetSourceThumbnail(sourceId)` 取源文件行登记的 `path`；路径都由后端从表里取，**不接受前端传路径**。
- **返回值**：一个字符串，就是 `MediaService.Thumbnail` 返回的 `Thumb.dataUrl`（也是 `MediaInfo.thumbUrl` 的格式）：**`data:image/jpeg;base64,<JPEG 文件的标准 base64，带 = 填充、不换行>`**，前端直接放进 `<img src>`。**不返回** `Thumb` 里的 `path`（缓存文件的绝对路径）、`atSec`、`width`：按“只收 id、不往前端暴露路径”的规则只给图片本身。
- **图片规格**（与 `Probe` 附带的默认缩略图完全相同，6.7）：JPEG，最大宽度 320（`media.DefaultThumbWidth`，源更窄时不放大），高度按比例取偶数，按旋转元数据转正；取帧（v0.24.2）：第一帧；第一帧太暗（平均亮度不超过 32）时取前 3 秒里第一张不黑的，全黑退回第一帧（见文首 v0.24.2 ①；原“时长的 10%”作废）。记录的 `result.durationSec` / `media` 表里没有时长时先探测一次，判断有没有画面。封面图（`attached_pic`）不算画面。GIF 输出有画面，可以出缩略图。
- **懒生成 + 缓存**：首次调用时才生成（同时最多 2 个 ffmpeg 在生成，相同参数并发只生成一次，单次超时 20 秒），复用 6.7 的磁盘缓存 `<数据目录>/thumbs/<sha1(缓存版本, path_key, mtime, size, atSec 毫秒（默认缩略图是固定记号）, width)>.jpg`。**缓存键必须包含文件的修改时间（mtime）和大小（size）（架构师定）**：同一路径的文件被覆盖或替换后，mtime 或 size 变了，缓存键随之变化，下次调用一定重新生成，不会返回旧文件的缩略图（每次调用都先 `os.Stat` 拿当前的 mtime / size 再算键）；容量上限和清理规则同 6.7（1000 个文件或 200 MB）。命中缓存不启动 ffmpeg。删除记录不主动删缓存，由容量清理回收。
- **不写进 `result`**，也不放进 `ListSources` / `SearchSources` 的返回值（避免一屏几十张图拖慢列表）；前端在行进入可视区域时逐个调用，失败按下面的规则显示占位。
- **错误码**：

| 情况 | code | 前端显示 |
|---|---|---|
| `taskId` / `sourceId` 不存在，或旧类型 | `NOT_FOUND`（`reason=record`） | 不显示该行（记录已被删） |
| `GetRecordThumbnail` 的任务不是 `convert` | `INVALID_ARGUMENT` | — |
| **文件不存在**（已被移动或删除），以及记录不是 `succeeded`（没有输出；v0.24 `reconverting=true` 的记录按 `succeeded` 处理，取旧文件，6.17.4）、不是普通文件、输出是符号链接 | `NOT_FOUND`（`reason=file`）（架构师定） | **显示占位图**（缺失文件的占位图，可配“文件已被移动或删除”提示）；不弹错误 |
| **没有画面，做不出缩略图**：纯音频（含只带封面图的 mp3 等），或扩展名 / 内容不是音视频 | `UNSUPPORTED`（`reason=format`） | **按类型显示图标**：音频容器（mp3 / m4a / aac / wav / flac / ogg / opus）显示音频图标，其他显示通用文件图标；不当作错误提示 |
| ffmpeg / ffprobe 缺失 | `FFMPEG_NOT_FOUND` | 类型图标 |
| 文件损坏、截图失败、超时 | `PROBE_FAILED` 或 `INTERNAL`（沿用 `MediaService.Thumbnail` 的错误） | 类型图标 |

### 6.14.11 任务中心用法（v0.23）

- **列表**：默认 `List(filter)`（`includeHidden=false`），不显示已隐藏的任务。打开“**显示已隐藏**”开关时用 `List({..., includeHidden: true})`，已隐藏的行（`hiddenInTaskCenter=true`）**置灰**显示。
- **“隐藏已结束”按钮**：调 `HideFinishedInTaskCenter()`，然后重新 `List`。所有类型都只隐藏、不删除。
- **已隐藏的行能做什么**：
  - **非转换任务**：可以“**移除**”（`Remove(ids, deleteOutput)`，真删）或“**取消隐藏**”（`UnhideInTaskCenter(ids)`）。
  - **转换记录**（`type=convert`）：**只能“取消隐藏”**；要删除去格式转换页（`DeleteRecords` / `DeleteSource`）。
- **“移除”（`Remove`）只用于非转换任务**：ids 里有 `convert` 任务时整体 `INVALID_ARGUMENT`（6.6），所以任务中心对转换记录的行不显示“移除”。
- **“重试”是原地的**（6.6 `Retry`）：不会多出一行。收到 `retried: true` 的 `task:status` 时按第 5 节清字段，**上一次运行的显卡编码回退提示（`hwFallback` / `hwFallbackReason`）随之消失**，新运行再回退会重新出现。
- `UnhideInTaskCenter` 发的 `task:status` 只改 `hiddenInTaskCenter` 和 `version`；“显示已隐藏”关闭时，任务中心收到它可以把这一行加回列表（或直接重新 `List`）。
- **“失败”数和“失败”页（v0.25.3，架构师定）**：用 `List({statuses: ["failed"]})` 的 `items` / `total`，**不含** `interrupted`（直播中途被中断、应用退出时被中断都是“已中断”，不算失败）。`interrupted` 的行照常出现在“全部”“历史”里，“隐藏已结束”会隐藏它。
- **“旧版导出”（v0.25.3）**：`edit_export` 记录只能查看和“移除”，不显示“重试”（`Retry` 返回 `UNSUPPORTED` `reason=feature_removed`）。

### 6.14.12 实现取舍（v0.23.1，随实现 PR 记录）

v0.23 条文有几处没写死，实现按最简单的理解落地，记在这里（前端可以依赖）：

- **`Retry` 的判断顺序**：不存在 `NOT_FOUND` → 进行中 `TASK_CONFLICT`（`任务仍在进行，请先取消`）→ 没有重试工厂 `UNSUPPORTED`（直播会话不论什么状态都是这一条，message `直播会话不能重试，请重新开始推流`）→ `succeeded` `TASK_CONFLICT`（`任务已经成功完成，不能重试`）→ 工厂重新校验（输入被删 `NOT_FOUND`、探测失败 `PROBE_FAILED` 等，记录不变）→ 库里的版本 / 状态与读到的不一致 `TASK_CONFLICT`（`任务状态已变化，请刷新后再试`）。
- **原地重试的输出名**：原名空着（磁盘上没有、没有 `.part`、没被别的任务占）就原样占回；被占时从“期望名”（转换：`<输出目录>/<源文件名>.<容器>`）开始按该类型的重名格式顺延；拿不到期望名时，转换先去掉原名末尾的 ` (n)` 再顺延，其他类型从原名顺延。上一次的 `.part` 只在它的修改时间不早于上次 `startedAt`（3 秒容差）、且没有别的任务占着这个名字时才删。
- **重名编号没有上限**（沿用现有 `namer`），见 6.14.5。运行时因磁盘上被抢先建了同名文件而顺延时，新名字随一条 `running` 的 `task:status`（带 `outputPath`，`version` +1）发出；名字没变不发。
- **缩略图的错误归类**：扩展名不在 6.14.7 的“系统打开白名单”（即不是音视频扩展名）时直接 `UNSUPPORTED`（`reason=format`），不启动 ffprobe；扩展名是音视频但探测出来没有画面也是 `reason=format`；ffprobe 本身失败保持 `PROBE_FAILED`。
- **`GetSource`** 的内嵌记录条数固定为 `recordLimit` 默认值 20，不带搜索字段。
- **`presetName`** 由后端在提交时按 `presetId` 查当前预设名写入（前端不传名字）；`presetId` 非空但查不到 `NOT_FOUND`（`reason=record`），整体不提交。
- **`TaskService.Remove` 的判断顺序**：有旧类型 id `NOT_FOUND` → 有 `convert` 任务（含进行中的）`INVALID_ARGUMENT` → 有进行中的 `TASK_CONFLICT`。
- **`UnhideInTaskCenter`** 先逐个校验所有 id（任一不存在 / 旧类型整体 `NOT_FOUND` `reason=record`，什么都不改），再在一个事务里改。`HideFinishedInTaskCenter` 不碰旧类型的记录。
- **回填**：`inputPaths` 损坏的记录每条一行，`name` = `title`，`name_key` = `title` 的 Go 小写；`output_name_key` 对所有类型的任务都补（搜索只用 `convert` 的）。
- **`DeleteSource`**：删完后这一行**一条记录都不剩**才删行（有 `still_running` 时行保留，`deletedSourceIds` 为空）。
- **`PreviewOutputName`** 遇到路径为空的行（回填出的损坏行）`INVALID_ARGUMENT`；`AddSources` 的相对路径 / 目录是单项 `INVALID_ARGUMENT`，不存在是单项 `NOT_FOUND`（`reason=file`）。
- **`OpenWithSystem` 的启动**：macOS / Linux 最多等 5 秒拿 `open` / `xdg-open` 的退出码（超过按成功），`xdg-open` 退出码 3 / 4、macOS stderr 含 `No application knows how to open` 归为 `reason=no_app`；Windows `ShellExecuteW` 返回 `SE_ERR_NOASSOC`（31）/ `SE_ERR_ASSOCINCOMPLETE`（27）归为 `reason=no_app`，其他失败 `PROCESS_FAILED`。

## 6.15 存储位置与源文件副本（v0.24）

> 只有契约，前后端按本节并行实现。设计依据：老板的要求（输出和上传都放在程序所在文件夹，转换只读副本）、架构师 2026-10-08 的决定（同一文件 = 原路径 + 大小 + 修改时间相同、副本放 `uploads/<sourceId>/<原文件名>`、引用计数、路径一律绝对路径不做迁移）、PM 的规则和文案。本节和第 1、3、4、5、6、6.8、6.9、6.14 节的 v0.24 改动是一个整体，冲突时以本节为准。

### 6.15.1 存储根目录 `<base>`（启动时确定一次）

1. **程序所在文件夹** `exeDir` = `filepath.Dir(filepath.EvalSymlinks(os.Executable()))`。特例：
   - **macOS**：可执行文件在 `.app` 包里（路径里有一段以 `.app` 结尾、且可执行文件在它的 `Contents/MacOS/` 下）时**不试写，直接用用户数据目录**：写进包里会破坏代码签名、升级时被整个替换，`/Applications` 对普通用户通常不可写，从下载目录直接打开时还可能被“App 转移”放到只读位置。只有不在 `.app` 包里（开发时直接运行可执行文件）才按下面的试写规则。
   - **Linux AppImage**：环境变量 `APPIMAGE` 非空且是绝对路径时，`exeDir` 取 `filepath.Dir($APPIMAGE)`（用户眼里“程序所在的文件夹”；可执行文件本身在只读的挂载点里）。
   - Windows、其他 Linux 安装方式没有特例。
2. **试写**：依次对 `<exeDir>/output`、`<exeDir>/uploads`：`os.MkdirAll(dir, 0o755)`；已存在但不是文件夹算失败；在里面 `os.CreateTemp(dir, ".ffmpegfree-write-test-*")`、写 1 个字节、`Close`、`Remove`。**两个都成功**才用 `<base> = exeDir`。任何一步失败（典型：Program Files、`/usr/bin`、只读的磁盘或网络位置）就**两个一起**回退，不会出现“output 在程序目录、uploads 在用户数据目录”的拆分；试写时新建的空文件夹回退前删掉（`os.Remove`，只删空的）。
3. **回退位置**：**Windows 是 `%LocalAppData%\FFmpegFree`**（即 `C:\Users\<用户>\AppData\Local\FFmpegFree`；v0.24.1 架构师定，取代 v0.24 的 `%AppData%`，大文件不放进漫游配置；读不到 `LOCALAPPDATA` 时退回应用数据目录）；其他平台 = 应用数据目录（第 1 节，`os.UserConfigDir()/FFmpegFree`）：macOS `~/Library/Application Support/FFmpegFree`，Linux `$XDG_CONFIG_HOME/FFmpegFree`（未设置时 `~/.config/FFmpegFree`）。在它下面同样建 `output`、`uploads`；连这里也建不出来时不阻止启动，`GetStorageDirs` 照样返回路径（`outputAvailable` / `uploadsAvailable` 为 `false`），用到时按 6.15.4 / 6.9 报 `IO_ERROR`。
4. **只在启动时判断一次**，运行期间不变，也不落库。所以同一台机器上“这次以管理员身份运行能写 Program Files、下次普通身份不能写”会让两次启动的 `<base>` 不同；这不影响旧记录（路径都是绝对路径，6.15.2 第 4 条），只是新文件去了另一个位置，页面上始终显示实际路径。
5. `<base>/output`、`<base>/uploads` 叫**默认输出目录**、**默认上传目录**；用户在设置里改过的叫**自定义目录**；正在使用的那个叫**实际目录**（自定义优先，否则默认）。
6. **回退提示（PM 文案）**：`fellBack=true` 时页面显示 `程序所在文件夹无法写入，文件已改存到：<路径>`（`<路径>` 是实际输出目录；两个目录都是自定义目录时不显示）。macOS `.app` 包按规则用用户数据目录，**不算回退**（`fellBack=false`、`baseKind="user_data"`），不显示这句话。

### 6.15.2 设置、读取与修改

```go
type StorageDirs struct {               // GetStorageDirs / SetStorageDirs 的返回值
    OutputDir         string `json:"outputDir"`         // 实际输出目录（绝对路径）：自定义优先，否则 <base>/output
    UploadsDir        string `json:"uploadsDir"`        // 实际上传目录（绝对路径）：自定义优先，否则 <base>/uploads
    OutputCustom      bool   `json:"outputCustom"`      // true = outputDir 来自设置里的自定义目录
    UploadsCustom     bool   `json:"uploadsCustom"`
    DefaultOutputDir  string `json:"defaultOutputDir"`  // <base>/output（“恢复默认”时用的值，供设置页显示）
    DefaultUploadsDir string `json:"defaultUploadsDir"` // <base>/uploads
    BaseKind          string `json:"baseKind"`          // "exe_dir" | "user_data"
    FellBack          bool   `json:"fellBack"`          // true = 程序所在文件夹不可写，<base> 回退到了用户数据目录（6.15.1 第 6 条显示提示）；macOS .app 包为 false
    OutputAvailable   bool   `json:"outputAvailable"`   // 调用时 outputDir 存在且是文件夹（只 stat，不试写）
    UploadsAvailable  bool   `json:"uploadsAvailable"`
}
type StorageDirsUpdate struct {         // SetStorageDirs 的参数：两个字段都要传
    OutputDir  string `json:"outputDir"`  // "" = 用默认输出目录；否则是自定义目录
    UploadsDir string `json:"uploadsDir"` // "" = 用默认上传目录
}

// SystemService（v0.24 新增）
GetStorageDirs() (StorageDirs, error)
SetStorageDirs(req StorageDirsUpdate) (StorageDirs, error)
OpenStorageFolder(kind string) error   // kind = "output" | "uploads" | "component"（v0.25.1）| "doc_component"（v0.27.2，6.12.54）| "lang_asr"（v0.29，6.18.3）
```

1. **一个读、一个写**：前端（设置页、转换页的“保存到”）只用 `GetStorageDirs` / `SetStorageDirs`。只改一个目录时，另一个传 `GetStorageDirs` 里的当前值（自定义的传路径，没自定义的传 `""`）。
2. **`SetStorageDirs` 校验**（两个字段先都校验，任一失败整体不生效，`INVALID_ARGUMENT`，与 v0.7.4 的 `defaultOutputDir` 校验相同）：非空时必须是绝对路径、`Clean` 后存在、是文件夹、能在里面建文件（同 6.15.1 第 2 条的试写）；保存 `Clean` 后的路径；等于对应默认目录的路径按 `""` 保存（不算自定义）。message：`保存位置必须是绝对路径` / `保存位置不存在` / `保存位置不是文件夹` / `保存位置无法写入`（上传目录把“保存位置”换成“上传位置”），`detail` 是路径。输出目录另按 6.12.3 的应用数据目录规则检查（v0.24.1：`<dataDir>/output` 及其子文件夹允许），不通过 `保存位置不能在应用数据目录内`。成功后返回新的 `StorageDirs`。
3. **`Settings`**（第 4 节 `GetSettings` / `UpdateSettings`）：`defaultOutputDir` 保留，**含义改为“自定义输出目录，`""` = 默认输出目录 `<base>/output`”**（v0.7.4~v0.23 里 `""` 表示“与源文件同一个文件夹”，这个选项 v0.24 起没有了）；新增 `uploadsDir`（自定义上传目录，`""` = `<base>/uploads`），设置表键 `uploadsDir`。`UpdateSettings` 对两个字段用第 2 条同样的校验（仍是原子的）。两套接口读写同一组设置键，结果一致。
4. **改目录只影响之后的新文件（PM 规则 1）**：已有的输出文件和副本**不搬动**，旧记录的 `outputPath`、`inputPaths`、`convert_copies.stored_path` 仍指向原来的位置；正在复制的副本照旧写完原位置；已提交（排队中）的转换任务用提交时定好的 `outputPath`。所有路径都按**绝对路径**存（6.15.3），**没有迁移**：用户改了目录、或者把整个程序文件夹搬走后，原位置找不到文件的记录按现有规则 `NOT_FOUND`（`reason=file`），`CheckSources` / `CheckPaths` 报不存在，界面显示虚线占位（6.14.10 的缺失占位）。
5. **`outputDir` 传空的地方都用实际输出目录**：`SubmitSources` / `Submit` / `Reconvert` / `PreviewOutputName`（6.9、6.14）、`DocService.ConvertToPDF`（6.12）。**v0.24.4：直播本地存档一样**——打开“保存存档”且用户没有另选文件夹时，目录是实际输出目录（`defaultOutputDir` 为空即 `<base>/output`，页面显示这个绝对路径，不显示空的）；关掉存档仍不存档（`archiveDir=""`，6.10）。“仍为空则输出到源文件所在文件夹”的分支 v0.24 起不再走到（实际输出目录永远非空）。输出目录不存在时任务开始时创建（6.9 原规则），建不出来 `IO_ERROR`。**不会**因为自定义目录暂时不可用（拔掉的 U 盘）就悄悄改用默认目录。
6. **`OpenStorageFolder(kind)`**：在系统文件管理器里**打开**实际输出 / 上传目录本身（平台命令同 6.8 的“path 是文件夹”分支：Windows `explorer.exe "<dir>"`，macOS `open <dir>`，Linux `xdg-open <dir>`），命令启动后立即返回。只收 `kind`、不收路径（符合 6.14.1 的只收 id / 枚举规则）。目录不在时：默认目录先 `MkdirAll` 再打开，建不出来 `IO_ERROR`；自定义目录不在 `NOT_FOUND`（`reason=file`，message `保存位置不存在，请在设置里重新选择`，不替用户建）。`kind` 不是这两个值 `INVALID_ARGUMENT`；启动命令失败 `PROCESS_FAILED`；Windows 路径含双引号 `INVALID_ARGUMENT`（同 6.8）。转换页“保存到”旁边和每行源文件旁边的“打开文件夹”分别用 `OpenStorageFolder("output")` 和 `RevealSource(sourceId)`（6.15.6），记录的输出用 `RevealRecord(taskId)`（不变）。 **v0.25.1：`kind="component"`**：打开当前使用的转换组件可执行文件（后端自己记下的路径，状态必须是 `ready`；`FFmpegStatus` 不再把 `path` 给前端，v0.25.4）所在的文件夹，Windows / macOS 同时选中这个文件（平台命令同 6.8 的“path 是文件”分支：Windows `explorer.exe /select,"<文件>"`，macOS `open -R <文件>`，Linux `xdg-open <所在文件夹>`）。组件不是 `ready` 或文件不在：`NOT_FOUND`，message `转换组件还没有就绪。`，`detail` 只有 `reason=component`；其他错误（启动命令失败等）码和 message 照旧，`detail` 同样只有 `reason=component`。**任何情况下都不把路径回给前端**，也不加进 `RevealInFolder` 的放行范围。设置页的「打开组件所在文件夹」用它。
7. **6.8 `RevealInFolder` 的放行范围**：第 2 类从“`defaultOutputDir` 之内”改为“**实际输出目录或实际上传目录之内**”（设置为空时也有值，所以默认目录现在也放行）。第 1、3 类不变。**v0.24.6 加第 4 类**：登记输出所在的文件夹本身（见 6.8），供完成横幅打开本轮另选的“保存到”文件夹；只放行文件夹本身。

### 6.15.3 数据：`convert_copies` 表与 `ConvertSource` 新字段

迁移 **`0007_storage_copies.sql`**（只加表、加列、加索引，不改旧数据；v0.24 契约原写 `0006`，因 v0.23.4 的 `0006_convert_source_media.sql` 先合入而顺延）：

```sql
CREATE TABLE convert_copies (
    id                TEXT PRIMARY KEY,          -- ULID（copyId，只在后端用，不出现在接口参数里）
    owner_source_id   TEXT NOT NULL,             -- 创建这份副本的源文件行（目录名 uploads/<owner_source_id>/）
    original_path     TEXT NOT NULL,             -- 复制自哪个文件（绝对路径，paths.Normalize）
    original_path_key TEXT NOT NULL,             -- 同 convert_sources.path_key 的规范化
    original_size     INTEGER NOT NULL,          -- 开始复制时原文件的大小（字节）
    original_mtime_ns INTEGER NOT NULL,          -- 开始复制时原文件的修改时间（Unix 纳秒）
    stored_path       TEXT NOT NULL,             -- 副本的绝对路径：<当时的实际上传目录>/<owner_source_id>/<原文件名>
    state             TEXT NOT NULL,             -- copying | ready | failed | canceled（见 6.15.4）
    total_bytes       INTEGER NOT NULL,          -- = original_size
    copied_bytes      INTEGER NOT NULL DEFAULT 0,
    error             TEXT,                      -- failed 时的 AppError JSON
    ref_count         INTEGER NOT NULL DEFAULT 0,-- 有多少个 convert_sources 行的 copy_id 指向它（6.15.5）
    pending_delete    INTEGER NOT NULL DEFAULT 0,-- 1 = 已没有行引用、删文件失败，等下次启动再删（6.15.7）
    created_at        INTEGER NOT NULL,
    finished_at       INTEGER                    -- 进入 ready / failed / canceled 的时间
);
CREATE INDEX idx_convert_copies_identity ON convert_copies(original_path_key, original_size, original_mtime_ns);
CREATE INDEX idx_convert_copies_state ON convert_copies(state);
ALTER TABLE convert_sources ADD COLUMN copy_id TEXT;   -- 这一行当前使用的副本；NULL = 没有副本（v0.24 之前的旧行、兼容 Submit 建的行）
CREATE INDEX idx_convert_sources_copy ON convert_sources(copy_id) WHERE copy_id IS NOT NULL;
```

```go
type ConvertSource struct {             // v0.24 在 6.14.2 的基础上新增 6 个字段
    SourceID       string     `json:"sourceId"`
    Path           string     `json:"path"`           // 不变：原文件的绝对路径（= originalPath，保留给旧前端）
    Name           string     `json:"name"`           // 原文件名；输出文件名一律按它取（不是副本名）
    AddedAt        int64      `json:"addedAt"`
    LastActivityAt int64      `json:"lastActivityAt"`
    Media          *MediaInfo `json:"media,omitempty"` // 不变：按原文件的 path_key 关联 media 表
    // v0.24：
    OriginalPath string    `json:"originalPath"`        // 原文件的绝对路径（用户的文件，后端永远不删、不改）
    StoredPath   string    `json:"storedPath"`          // 副本的绝对路径；copyState=none 时为 ""；copying / failed / canceled 时是副本将要 / 曾经所在的位置（文件不一定存在）
    CopyState    string    `json:"copyState"`           // none | copying | ready | failed | canceled
    CopiedBytes  int64     `json:"copiedBytes"`         // 已复制的字节数；ready 时 = totalBytes；none 时 0
    TotalBytes   int64     `json:"totalBytes"`          // 副本应有的大小（= 开始复制时原文件的大小）；none 时 0
    CopyError    *AppError `json:"copyError,omitempty"` // 只有 failed 有：错误码与 message 见 6.15.4
}
```

- **`copyState` 的含义**：`none` = 没有副本，转换读 `originalPath`（v0.24 之前的旧行、兼容 `Submit(inputs)` 建的行）；`copying` = 已排进复制队列或正在复制（不区分排队和进行中，看 `copiedBytes`）；`ready` = 副本完整，转换读 `storedPath`；`failed` = 复制失败（`copyError` 有值），页面显示“**复制失败**”，提供“**重试**”（`RetryCopy`）和“**从列表移除**”（`DeleteSource`）（PM 规则 3）；`canceled` = 用户调了 `CancelCopy`：**行保留**，页面显示“**已取消复制**”，同样可以“重试”（`RetryCopy`）或“从列表移除”。`copying` 的行**可以勾选**（提交时会被跳过，6.15.4 第 6 条），**不能预览**（`GetSourcePreviewURL` 返回 `TASK_CONFLICT`，`reason=copying`）。
- 一行的这些字段来自它当前的 `copy_id` 指向的 `convert_copies` 行；`ListSources` / `GetSource` / `SearchSources` / `AddSources` / `RetryCopy` 返回的都是这个视图，正在复制的行带实时的 `copiedBytes`（同 `ListActive` 带实时进度的做法，进度不落库，只有状态变化落库）。
- **所有路径都是绝对路径**，表里不存相对 `<base>` 的路径，所以改目录、搬程序文件夹都不需要迁移（6.15.2 第 4 条）。

### 6.15.4 添加与复制

1. **`AddSources(paths)`**（6.14.3 的规则全部保留：1~500 个、逐个规范化和 stat、同 `path_key` 已有行 `existed=true` 并更新 `lastActivityAt`、单项失败放进该项 `error`、不探测）。v0.24 增加：
   - **扩展名**：只接受 6.16.4 的“输入扩展名”（目录里全部 34 种扩展名，含 APE 这类只能读不能写的，加 `jpeg tiff m4v mpeg ts mts m2ts aif aiff`），其他扩展名该项 `UNSUPPORTED`（`reason=format`，message `不支持这种文件`），不建行、不复制。原因：现在添加就要复制，不能把任意大文件都拷一遍。
   - **每个成功的项**在返回之前同步做完“找副本 / 排复制”这一步（下面 2~4），复制本身在后台进行，`AddSources` **不等复制**，返回的 `source` 已带 `copyState`（通常是 `copying`，复用时是 `ready`，空间不足时直接是 `failed`）。
2. **“同一个文件”（架构师定）= 原路径（`path_key`）、大小、修改时间（纳秒）都相同，不算哈希**。添加一个文件时按顺序判断：
   1. 这个路径**本身就是某份 `ready` 副本的 `stored_path`**（用户把 `uploads` 文件夹里的副本又拖了进来）：不再复制，这一行（按它自己的 `path_key` 建或找到）直接引用那份副本，`ref_count + 1`，`copyState=ready`。这是 v0.24 里**多行共享一份副本**的情形（6.15.5）。
   2. 这一行已经有副本，且副本是 `copying`，或是 `ready` 且 `stored_path` 现在是普通文件、大小等于 `total_bytes`，并且原文件的大小和修改时间与副本记下的**都相同**：什么都不做（复用，不再复制）。
   3. 有其他 `copying` / `ready` 的副本与这个文件是“同一个文件”（`idx_convert_copies_identity` 查得到，`ready` 的同样要求文件还在且大小对）：这一行引用它，`ref_count + 1`。**说明**：目前 `convert_sources.path_key` 唯一，同一个原路径总是落到同一行，所以这一条实际上走不到（会先命中 2.2）；写出来是为了“同一个文件只存一份”的规则完整，以后如果放开“一个文件多行”可以直接生效。
   4. 否则**新做一份副本**（下面第 3 条）。这一行原来有副本（原文件变了、副本被删了、上次失败或取消了）时，先解除这一行对旧副本的引用（`ref_count − 1`，归零按 6.15.7 删掉旧副本文件）；**这一行有排队中 / 运行中的转换记录时不换副本**，该项 `TASK_CONFLICT`（message `这个文件还有正在进行的转换，请等转换结束后再添加`），行保持原样（避免正在读副本的任务被换掉文件）。v0.24 之前的旧行（`copyState=none`）被再次添加时同样新做一份副本：之后的转换读副本，**旧记录仍读它们当时的原路径**（`inputPaths` 不改）。
3. **新做副本**：
   - **位置（架构师定）**：`<实际上传目录>/<sourceId>/<原文件名>`（`sourceId` 是这一行的 id，原文件名是 `ConvertSource.name`，不改名、不加序号；每行一个目录，名字永远不会冲突）。复制时先写同目录的 `<原文件名去扩展名>.part.<原扩展名>`（规则同 6.5 的 `.part`；没有扩展名时是 `<原文件名>.part`），完成后改名为最终名。目录不存在时创建。
   - **磁盘空间检查（复制之前）**：`needBytes` = 原文件大小 + 正在排队 / 复制中的其他副本还没写完的字节数（同一卷上，`total_bytes − copied_bytes` 之和）+ 预留 64 MiB；`freeBytes` = 上传目录所在磁盘此刻可用的空间（Unix `statfs` 的 `Bavail × Bsize`，Windows `GetDiskFreeSpaceExW` 的 `lpFreeBytesAvailableToCaller`）。`freeBytes < needBytes` 时**不开始复制**，这份副本直接是 `failed`，`copyError` 为 `CONVERT_DISK_FULL`，message **`磁盘空间不足，需要 X，剩余 Y。`**（PM 文案，全角逗号和句号；X = `needBytes`、Y = `freeBytes`，按 1024 进位、最多 1 位小数、去掉 `.0`，单位 `B` / `KB` / `MB` / `GB` / `TB`，如 `磁盘空间不足，需要 4.2 GB，剩余 1.5 GB。`），`detail` **三行**：`reason=no_space`、`needBytes=<整数>`、`freeBytes=<整数>`（前端用这两行自己排版也可以）。拿不到可用空间（接口失败）时跳过检查，照常复制，写满时按下面的 ENOSPC 处理。
   - **复制**：后台复制队列，**同时最多 2 个**，先进先出；不是任务（不进任务管理器、不出现在任务中心、不占 batch 并发数、没有 `task:*` 事件）。只读打开原文件，1 MiB 缓冲顺序复制，写完 `Sync`、关闭；再 `os.Stat` 原文件：大小或修改时间和开始时不一样 → `failed`（`IO_ERROR`，`detail` 首行 `reason=source_changed`，message `复制期间原文件被修改了，请重试`）；写出的字节数不等于 `total_bytes` 同样按它处理。通过后改名为最终名（已存在同名文件时覆盖：那只可能是这一行自己的旧副本，第 2.4 步已解除引用），把副本的修改时间设成原文件的修改时间（`os.Chtimes`），状态 `ready`。
   - **复制失败**（`copyError`）：原文件不见了 `NOT_FOUND`（`reason=file`，message `原文件不存在`）；写入时磁盘满（ENOSPC / EDQUOT，Windows 112 / 39）`CONVERT_DISK_FULL`（`reason=no_space`，`needBytes` = 剩余没写的字节 + 64 MiB，`freeBytes` = 此刻可用空间，message 同上）；没有权限、只读、其他读写错误 `IO_ERROR`（message `复制文件失败`，详细原因只进应用日志）；上面的 `reason=source_changed`；应用退出时还没复制完：下次启动标成 `failed`，`IO_ERROR`，`reason=interrupted`，message `复制被中断，请重试`（**不自动续传、不自动重来**，与任务的 `interrupted` 一致）。所有失败和取消都删掉 `.part`（启动时也清一次：每个 `copying` 状态的副本目录里的 `.part`）。
4. **转换读哪个文件**（下文的“读取路径”）：`copyState=ready` 读 `storedPath`；`none` 读 `originalPath`；`copying` / `failed` / `canceled` 不能转换（第 6 条）。
5. **事件 `convert:copy`**（第 5 节）：payload `{ sourceId, seq, copyState, copiedBytes, totalBytes, storedPath, error? }`。开始复制（`copying`，`copiedBytes=0`）发一次；复制中每个副本**最多每秒 4 次**（节流抑制的最后一次在间隔到期后补发，同 `task:progress`）；结束（`ready` / `failed` / `canceled`）一定发一次，带最终的 `copiedBytes`，`failed` 带 `error`（就是 `copyError`）。**`seq` 是进程内全局递增的整数**，前端按 `sourceId` 记住最大 `seq`、丢弃更小的（与任务的 `version` 同样用法）。多行共享一份副本时，**每一行各发一条**。复用（第 2.2 条）不发事件（返回值里已经是 `ready` / `copying`）。复制队列不因前端没订阅而暂停；前端重新进入页面时以 `ListSources` 的快照为准，再接着收事件。
6. **副本没就绪时不转换：`SubmitSources` 跳过、只提交就绪的（UI 设计定）**。`SubmitSources` 返回值改为：

   ```go
   type ConvertSubmitResult struct {
       Tasks   []Task          `json:"tasks"`   // 实际提交的任务，按 sourceIds 里就绪的那些的顺序；没有时是 []
       Skipped []SkippedSource `json:"skipped"` // 因为副本没就绪而跳过的行，按 sourceIds 的顺序；没有时是 []
   }
   type SkippedSource struct {
       SourceID string `json:"sourceId"`
       Reason   string `json:"reason"`  // "copying" | "copy_failed" | "copy_canceled"
   }
   ```

   `reason` 让前端分开两类文案（UI 设计定）：`copying` = **还在复制**（“将跳过 k 个还在复制的文件”）；`copy_failed` / `copy_canceled` = **复制失败或已取消**（前端用另一条文案，两者可以合并显示）。

   - 先按 `copyState` 把选中的行分开：`ready` 和 `none` 是**就绪**；`copying`（`reason="copying"`）、`failed`（`"copy_failed"`）、`canceled`（`"copy_canceled"`）进 `skipped`。
   - **至少有一行就绪**：只对就绪的行走 6.9 的全部规则（先整体校验再提交、一个不通过整体失败、不回滚已提交的），成功时返回 `{tasks, skipped}`，**跳过不是错误**。前端提交前可以按 `copyState` 自己算出 k，还在复制的显示“**将跳过 k 个还在复制的文件**”，复制失败 / 已取消的按 `reason` 用另一条文案；提交后以返回的 `skipped` 为准。被跳过的行不更新 `lastActivityAt`。
   - **一行都没就绪**：整体 `TASK_CONFLICT`，什么都不提交：只要有一行是 `copying`，message `文件还在准备中，准备好后再转换。`（v0.24.4；原为 `文件还在复制，请等复制完成后再转换`）、`detail` 首行 `reason=copying`；否则（全是失败 / 已取消）message `文件复制没有完成，请先重试复制`、`detail` 首行 `reason=copy_failed`；第二行是第一个被跳过的 `sourceId=<id>`。
   - `ready` 但 `storedPath` 不在（用户删了 `uploads` 里的文件）**算就绪**、不跳过，按 6.9 的“文件不存在”处理（`NOT_FOUND`，`detail` 第一行是路径，整体不提交），前端提示后可以“重试”复制（`RetryCopy` 接受这种情况）。
   - **`Reconvert`**（单条，v0.24 起原地进行，6.17）：读这一行**当前**的读取路径（写进新的 `params.input`）；副本没就绪时直接 `TASK_CONFLICT`（`reason=copying` / `copy_failed`，规则同上，没有“跳过”）。**v0.24.6**：`reason=copying` 的 message 是 `文件还在准备中，准备好后再重转。`（`SubmitSources` 仍是“准备好后再转换”）。
   - **原地重试 `TaskService.Retry` 不变**：按 `params.input` 重建（6.6），读的还是当时那个路径；副本被换过时（第 2.4 条，路径相同）读到的是新副本。
   - 兼容的 `Submit(inputs …)` 返回值不变（`[]Task`），它不复制，没有跳过（第 8 条）。
7. **转换任务里记什么**：`inputPaths[0]` 和 `params.input` = 读取路径（副本路径或旧行的原路径）；`title` 用原文件名（`a.mkv → MP4`）；**输出文件名按原文件名取**（`<ConvertSource.name 去扩展名>.<容器扩展名>`，6.14.5），所以副本放在 `uploads/<sourceId>/` 下不影响输出命名；`sourceId` 照旧。`TaskService.GetPreviewURL(taskId, "input")`、`CheckPaths` 的 `inputExists` 看的是 `inputPaths[0]`（副本）。
8. **`Submit(inputs …)`（兼容保留）不复制**：仍按传入的路径找到 / 创建行（新建的行 `copyState=none`），直接读传入的路径；已有副本的行也不改。新前端不用它。

### 6.15.5 引用计数（多行共享一份副本）

- **计数在 `convert_copies.ref_count`**：= `convert_sources` 里 `copy_id` 等于它的行数。行引用副本（6.15.4 第 2.1 / 2.3 / 2.4 条）、解除引用（换副本、`DeleteSource`）都和 `copy_id` 的修改在**同一个事务**里加减 1。每次启动在迁移之后用 `SELECT copy_id, COUNT(*) … GROUP BY copy_id` 重算一遍（自愈，防止崩溃留下的不一致）。
- **归零才删**：只有 `ref_count` 变成 0 时才删副本文件和 `convert_copies` 行（6.15.7）；共享的副本只要还有一行在用就保留。**转换记录不计数**：`DeleteRecords` 不碰副本；副本被删之后，指向它的旧记录（只可能是同一行在换副本之前的记录，`DeleteSource` 时它们已一起删掉）重试时按 `NOT_FOUND`（`reason=file`）处理。
- 共享的行各自显示同一份副本的 `storedPath`、`copyState` 和进度；`CancelCopy` / `RetryCopy` 作用于副本本身，所以对共享的所有行同时生效（各行都会收到 `convert:copy`）。

### 6.15.6 新接口与改变的接口（ConvertService）

```go
// ConvertService（v0.24 新增）
CancelCopy(sourceID string) error
RetryCopy(sourceID string) (ConvertSource, error)
GetFormatCatalog() ([]FormatEntry, error)            // 见 6.16
```

| 接口 | 行为 | 错误码 | 事件 |
|---|---|---|---|
| `CancelCopy(sourceId)` | 取消这一行副本的复制：`copying` → `canceled`，停止读写、删 `.part`；**行保留**（显示“已取消复制”，可以 `RetryCopy` 或 `DeleteSource`）。已经是 `canceled` 时什么都不做（幂等，不算错误） | 行不存在 `NOT_FOUND`（`reason=record`）；没有正在复制的副本（`none` / `ready` / `failed`）`TASK_CONFLICT`（message `没有正在进行的复制`） | `convert:copy`（`canceled`） |
| `RetryCopy(sourceId)` | 重新复制（PM 规则 3 的“重试”）：**从 `failed` 或 `canceled` 都可以**，或 `ready` 但 `storedPath` 已不是普通文件 / 大小不对时，按 6.15.4 第 2.4、3 条重新做（重新检查空间、重新读原文件当前的大小和修改时间）；返回更新后的行。空间不足、原文件不在这类立即失败的情况**不是调用错误**：返回的行 `copyState=failed`、`copyError` 有值 | 行不存在 `NOT_FOUND`（`reason=record`）；`copying` `TASK_CONFLICT`（message `文件还在准备中，不需要重试。`，v0.24.4）；完好的 `ready` `TASK_CONFLICT`（message `文件已经在复制或已复制完成`）；`none`（旧行）`INVALID_ARGUMENT`（message `这个文件不需要复制`）；有排队中 / 运行中的转换记录 `TASK_CONFLICT`（同 6.15.4 第 2.4 条） | `convert:copy` |
| `AddSources(paths)` | 见 6.15.4 第 1~3 条 | 单项新增 `UNSUPPORTED`（`reason=format`，扩展名）、`TASK_CONFLICT`（换副本时有进行中的记录） | 新做副本的每一行 `convert:copy` |
| `ListSources` / `SearchSources`（`status` 筛选） | **`active` = 至少有一条 `queued` / `running` 记录，或副本 `copyState=copying` 的行**（前端要求）；**`failed` = 至少有一条 `failed` / `interrupted` 记录，或副本 `copyState=failed` 的行**（PM 已确认；`canceled` 的记录和 `canceled` 的复制都不算，与 v0.23.1 一致）；在原来的 `EXISTS` 上 `OR` 一个对 `convert_copies` 的条件，分页、排序、内嵌记录规则不变 | 同前 | 无 |
| `SubmitSources(req) (ConvertSubmitResult, error)` | **签名变化**：返回 `{tasks, skipped}`，没就绪的行跳过，见 6.15.4 第 6、7 条 | 新增：一行都没就绪时 `TASK_CONFLICT`（`reason=copying` / `copy_failed`）；其余同前 | 每个提交的任务 `task:created` |
| `Reconvert(req)` | **参数改为结构体、改为原地**（6.17）；读这一行当前的读取路径，见 6.15.4 第 6 条 | 新增 `TASK_CONFLICT`（`reason=copying` / `copy_failed` / `invalid_state` / `output_moved`）、源文件不在 `NOT_FOUND`（`reason=file`）、换格式 `INVALID_ARGUMENT`（`reason=format_change`）；替换失败记在 `lastReconvertError`（`IO_ERROR`，`reason=in_use` / `permission` / `io`，6.17.5） | 见 6.17.3、6.17.5 |
| `CheckSources(sourceIds)` | `SourcePathCheck` 新增 `originalExists`、`storedExists`（副本是普通文件；`none` 时 `false`）；**`exists` = 读取路径存在**：`ready` 看副本，其余看原文件 | 同前 | 无 |
| `GetSourcePreviewURL(sourceId)` | **`copying` 的行不能预览**（UI 设计定）；其余用“显示路径”：`ready` 用 `storedPath`，`none` / `failed` / `canceled` 用 `originalPath`；其余规则同 6.14 | 新增：`copying` 时 `TASK_CONFLICT`，message `文件还在准备中，准备好后才能预览。`（v0.24.4；原为 `文件还在复制，复制完成后才能预览`），`detail` 只有一行 `reason=copying`；其余同前 | 无 |
| `OpenSourceWithSystem` | **v0.24.4：始终打开 `originalPath`（用户的原文件）**，副本 `ready` 也不打开 `storedPath`；原文件不在时不退回副本 | 原文件不在 `NOT_FOUND`（`reason=file`，message `原文件不存在，无法打开。`）；其余同 6.14 | 无 |
| `GetSourceThumbnail` | 仍用“显示路径”：`ready` 用 `storedPath`，其余（`none` / `copying` / `failed` / `canceled`）用 `originalPath`（复制中也可以，读的是原文件）；其余规则同 6.14 | 同前 | 无 |
| `RevealSource(sourceId)` | **v0.24.4：打开 `originalPath` 所在文件夹并选中原文件**，不选中 `uploads/<sourceId>/` 里的副本；原文件不在时不退回副本。转换页每行“打开文件夹”就是它（页面上显示的名字和路径仍是原文件的） | 原文件不在 `NOT_FOUND`（`reason=file`，message `原文件不存在，无法打开。`）；其余同前 | 无 |
| `DeleteSource(sourceId, deleteOutputs)` | 见 6.15.7 | 同前 | 同前，另加 `convert:copy`（`canceled`，如果在复制） |

```go
type SourcePathCheck struct {           // v0.24 新增两个字段
    SourceID       string `json:"sourceId"`
    Found          bool   `json:"found"`
    Exists         bool   `json:"exists"`         // 读取路径存在（ready 看副本，其余看原文件）
    OriginalExists bool   `json:"originalExists"` // v0.24
    StoredExists   bool   `json:"storedExists"`   // v0.24：副本是普通文件；没有副本为 false
}
type DeleteFailure struct {             // v0.24 新增 sourceId
    TaskID   string `json:"taskId"`             // 副本删不掉时为 ""
    SourceID string `json:"sourceId,omitempty"` // v0.24：只有副本删不掉的那一条有
    Path     string `json:"path,omitempty"`
    Reason   string `json:"reason"`
    Message  string `json:"message"`
}
```

### 6.15.7 “从列表移除”（`DeleteSource`）与副本的删除

**一句话（给界面和文档用）**：从列表移除一行，会删掉这一行的转换记录和我们在 `uploads` 里为它做的副本；**永远不删用户的原文件**；输出文件只有勾选了“同时删除输出文件”（`deleteOutputs=true`）时才删。

1. 这一行的副本在复制中：先取消（同 `CancelCopy`），和第 2 步共用 10 秒的等待。**v0.24.1 实现取舍**：副本还被别的行共享（`ref_count > 1`）时**不取消**，只解除这一行的引用，复制照常给其他行做完。
2. 其余同 6.14.4：先取消进行中的记录（10 秒内没停下来的记 `still_running`、整行保留）、删记录、`deleteOutputs` 时删输出文件、删 `.part` 残留；**永远不删用户的原文件**（`originalPath`），旧行也一样。
3. 这一行的记录全部删掉、`convert_sources` 行删掉之后，在同一个事务里把它引用的副本 `ref_count − 1`。**归零**时删副本：只删 `stored_path` 这个文件本身（必须是普通文件、不是符号链接、大小等于 `total_bytes`、修改时间等于 `original_mtime_ns`（v0.24.1：相差不超过 2 秒即可，`Chtimes` 写回的时间受文件系统精度限制，如 FAT 2 秒）——即我们写出的那个文件；不满足按 `not_task_output` 记一条失败，文件不碰），再删 `.part`，再删 `uploads/<owner_source_id>/` 目录（**只在已经空了时删**，用户自己往里放了东西就留着）；然后删 `convert_copies` 行。**没归零**（还有别的行共享）：什么文件都不删。
4. 副本文件删不掉（被占用、没有权限、其他错误）：行和记录照删，`failures` 里加一条 `{taskId: "", sourceId, path: stored_path, reason: in_use | permission | io, message 同 6.14.4}`，这个 `path` 同样进 v0.23.3 的 10 分钟放行表（可以 `RevealInFolder`）；`convert_copies` 行保留并标 `pending_delete=1`、`ref_count=0`，**下次启动时再按第 3 条的条件删一次**（文件已被替换或不在了就只删表行，不再碰文件）。
5. 后端**从不扫描 `uploads` 目录删“认不出的文件”**：用户可能把上传目录设成了自己的文件夹，只删表里登记过、且确认是自己写出的文件。
6. `DeleteRecords` 不碰副本（第 6.15.5 条）。

### 6.15.8 旧数据（v0.24 之前）

- **旧行不补做副本**：迁移 `0007` 只加表和列，旧的 `convert_sources` 行 `copy_id = NULL`，即 `copyState=none`、`storedPath=""`，转换、预览、缩略图、打开都继续用原路径（`originalPath` = 原来的 `path`）。没有回填，也不在启动时后台复制。
- 旧行被用户再次添加（`AddSources` 同一路径）时才开始有副本（6.15.4 第 2.4 条）；旧记录不受影响。
- 旧记录的输出仍在它们当时的位置（多数在源文件旁边，v0.23 及之前的默认），不搬。

## 6.16 格式目录（v0.24）

> 只有契约。格式范围参照格式工厂，由老板提出、架构师确定；默认参数已用项目默认安装的转换组件（ffmpeg 9.0.2 静态版，martin-riedl）逐个实测能输出、能被 ffprobe 读回（AMR 除外：该版本没有 `libopencore_amrnb`，按规则显示为不可输出；APE 没有编码器）。

### 6.16.1 接口与数据

```go
// ConvertService（v0.24）
GetFormatCatalog() ([]FormatEntry, error)

type FormatEntry struct {
    Category       string         `json:"category"`             // "video" | "audio" | "image"
    Extension      string         `json:"extension"`            // 输出扩展名，小写、不带点，如 "mp4"、"tif"；同时就是 ConvertOptions.container 的取值，目录内唯一
    DisplayName    string         `json:"displayName"`          // 给人看的名字，如 "MP4"、"WebM"、"Opus"
    Aliases        []string       `json:"aliases"`              // 搜索用的别名（中文常用叫法为主），没有时是 []
    Encodable      bool           `json:"encodable"`            // 当前转换组件能否输出这个格式（按默认预设的参数检测，6.16.3）
    Reason         string         `json:"reason,omitempty"`     // encodable=false 时给用户看的一句话（不含 “ffmpeg”，1.1）
    ReasonCode     string         `json:"reasonCode,omitempty"` // encodable=false 时的稳定枚举：converter_not_ready | missing_muxer | missing_encoder | check_failed
    DefaultPresetID string        `json:"defaultPresetId"`      // 这个格式的默认预设（永远是内置预设，6.16.2），也在 presets 里
    Presets        []FormatPreset `json:"presets"`              // 这个格式可用的预设（“参数”下拉）：先内置（按 6.16.2 的顺序），后用户预设（按创建顺序）；至少有默认预设这一项
}
type FormatPreset struct {
    ID            string         `json:"id"`            // 即 Preset.id
    Name          string         `json:"name"`          // 即 Preset.name
    BuiltIn       bool           `json:"builtIn"`
    ParamsSummary string         `json:"paramsSummary"` // 按 6.14.5 的规则由 options 算出（如 "H.264 · 1080p"），永远非空
    Options       ConvertOptions `json:"options"`       // 提交时作为 SubmitSources 的 options，presetId 传 id
}
```

- **预设归属**：一个预设属于 `options.container` 对应的格式（内置和用户预设都一样）；`container` 不在目录里的用户预设（理论上不会有，`SavePreset` 会校验）不出现在任何格式下，`ListPresets` 照样返回。用户在“参数”下拉里改了参数又没保存时，提交时 `presetId` 传空（自定义参数，6.14.2）。
- **用户预设变了要重新取**：`SavePreset` / `DeletePreset` 成功后前端重新调一次 `GetFormatCatalog`（`encodable` 用缓存，很便宜），或自己按上面的归属规则更新本地副本。

- **调用时机**：前端进入转换页时取一次，缓存在前端；收到 `ffmpeg:status` 且 `state` 变为 `ready` 时再取一次（v0.25.4：这是冷启动重新加载的触发，payload 是完整的 `FFmpegStatus`）。后端按“转换组件可执行文件路径 + 大小 + 修改时间”缓存检测结果（v0.24.1 实现：换了组件键就变，自动重新检测）。**v0.25.4**：调用时如果转换组件正在检测，本接口最多等 **6 秒**再回答；前端自己的超时必须大于 6 秒。6 秒内仍在检测则按未就绪返回，前端看状态仍是 `checking`，等上面的事件再取，不要把这次未就绪当成最终结果。
- **不需要转换组件就绪也能调**：转换组件不是 `ready` 时照样返回完整目录，所有项 `encodable=false`、`reasonCode="converter_not_ready"`、`reason="转换组件尚未就绪"`，不报错（前端照样能显示格式列表）。**v0.25.4**：这只适用于检测已经结束、或等满 6 秒仍在检测；检测进行中会先等（见上一条），检测在 6 秒内完成就按真实能力返回。检测命令本身失败（超时、不能运行）时同样全部 `encodable=false`，`reasonCode="check_failed"`、`reason="暂时无法确认转换组件是否支持这个格式"`，这次结果**不缓存**（未就绪同样不缓存）；提交时的格式检查（6.16.6）遇到检测失败**不拦截**，照常提交（v0.24.1）。
- **顺序固定**：按 `category`（video、audio、image）分组，组内按 6.16.2 表格的顺序；前端可以直接按这个顺序显示。
- **不可输出的格式照样列出**（PM 规则 4）：前端置灰，悬停提示 `当前转换组件不支持输出这个格式`（就是 `missing_muxer` / `missing_encoder` 的 `reason`）。
- **搜索只在前端做**（前端要求）：前端对 `extension`、`displayName`、`aliases` 做模糊匹配（不区分大小写，算法由前端定），后端没有搜索接口；搜不到时的空状态文案也由前端负责。
- 错误：只有启动完成前调用返回 `INTERNAL`（同 6.9）；其余情况不返回错误。

### 6.16.2 目录内容与默认参数

`ConvertOptions` 的取值在 v0.24 扩充（第 3 节同步）：`container` 增加 `wmv mpg vob 3gp swf ogv wma amr m4r mp2 ape wv mmf webp ico bmp tif tga`；`videoCodec` 增加 `mpeg4`（MPEG-4 Part 2 / Xvid，编码器 `mpeg4`）、`mpeg2`（`mpeg2video`）、`wmv2`（`wmv2`）、`flv1`（Sorenson H.263，编码器 `flv`）、`theora`（`libtheora`）；`audioCodec` 增加 `wma`（`wmav2`）、`amr_nb`（`libopencore_amrnb`）、`mp2`（`mp2`）、`wavpack`（`wavpack`）、`adpcm_yamaha`（`adpcm_yamaha`）、`ape`（**转换组件里没有这个编码器**，只为让 APE 走“不可输出”而不是“参数不合法”）。下表“默认参数”就是这个格式**默认预设**的 `options`（没写的字段为 0 / `""`），默认预设的 id 写在“默认预设”一列（6.16.2 末尾列出全部内置预设）；“固定参数”是后端按格式加的、用户改不了的参数；所有输出都显式加 `-f <muxer>`（不靠扩展名猜，输出写的是 `.part` 文件）。

**视频**（`category=video`）

| extension | displayName | aliases | 默认参数（默认预设的 options） | 固定参数 / 说明 | 检测用 muxer / encoder | 允许的视频 / 音频编码 |
|---|---|---|---|---|---|---|
| `mp4` | MP4 | 通用视频, 手机视频, MPEG-4 | `h264` + `aac` | 同 6.9（`+faststart`） | mp4 / libx264, aac | copy h264 h265 vp9 / copy aac mp3 opus ac3 |
| `mkv` | MKV | 高清, 蓝光, Matroska | `h264` + `aac` | 同 6.9 | matroska / libx264, aac | copy h264 h265 vp9 / copy aac mp3 opus vorbis flac ac3 pcm |
| `mov` | MOV | 苹果, 苹果视频, QuickTime | `h264` + `aac` | 同 6.9 | mov / libx264, aac | copy h264 h265 / copy aac mp3 ac3 pcm |
| `webm` | WebM | 网页视频 | `vp9` + `opus` | 同 6.9 | webm / libvpx-vp9, libopus | copy vp9 / copy opus vorbis |
| `avi` | AVI | 老式视频, Xvid, DivX | `mpeg4` + `mp3` | `mpeg4`：`-vtag xvid`，没设码率时 `-q:v 4`（v0.24 新增编码；原来允许的 `h264` 保留） | avi / mpeg4, libmp3lame | copy h264 mpeg4 / copy mp3 aac ac3 pcm |
| `flv` | FLV | Flash 视频, 网络视频 | `h264` + `aac` | 同 6.9 | flv / libx264, aac | copy h264 / copy aac mp3 |
| `gif` | GIF | 动图, 表情包, 动画, 图片 | 宽 480、12 帧/秒 | 同 6.9（调色板，**保持动画**，不按 6.16.5 取单帧，所以归在视频类） | gif / gif | —（不带音轨） |
| `wmv` | WMV | Windows 视频, 微软视频 | `wmv2` + `wma` | 没设码率时 `-q:v 4`；`wma` 默认 192 kbps | asf / wmv2, wmav2 | wmv2 / wma |
| `mpg` | MPG | MPEG, MPEG-2, VCD | `mpeg2` + `mp2` | 没设码率时 `-q:v 3`；`mp2` 默认 224 kbps | mpeg / mpeg2video, mp2 | mpeg2 / mp2 ac3 |
| `vob` | VOB | DVD | `mpeg2` + `ac3` | 同上；`ac3` 默认 192 kbps；**不强制 DVD 分辨率**（720×480 / 576 由用户自己设） | vob / mpeg2video, ac3 | mpeg2 / ac3 mp2 |
| `3gp` | 3GP | 老手机, 手机视频 | `h264` + `aac`，音频 96 kbps | `-profile:v baseline`；不用 AMR 音频（多数转换组件没有它的编码器） | 3gp / libx264, aac | h264 / aac amr_nb |
| `swf` | SWF | Flash, Flash 动画 | `flv1` + `mp3` | 没设码率时 `-q:v 5`；音频固定 `-ar 44100`（swf 只认 44.1 / 22.05 / 11.025 kHz）；Flash 已停止支持，多数播放器打不开，仍按格式工厂列出 | swf / flv, libmp3lame | flv1 / mp3 |
| `ogv` | OGV | Ogg 视频, Theora | `theora` + `vorbis` | 没设码率时 `-q:v 7`；vorbis 同 6.9（`-q:a 5`） | ogg / libtheora, libvorbis | theora / vorbis opus |

**音频**（`category=audio`；音频规则同 6.9：不能设分辨率 / 帧率，只取第一条音轨）

| extension | displayName | aliases | 默认参数 | 固定参数 / 说明 | 检测用 muxer / encoder | 允许的音频编码 |
|---|---|---|---|---|---|---|
| `mp3` | MP3 | 音乐, 歌曲, 通用音频 | `mp3`，192 kbps | 同 6.9 | mp3 / libmp3lame | mp3 copy |
| `m4a` | M4A | 苹果, 苹果音频, AAC | `aac`，192 kbps | 同 6.9 | ipod / aac | aac copy |
| `aac` | AAC | 高级音频编码 | `aac` | 同 6.9（ADTS） | adts / aac | aac copy |
| `wav` | WAV | 无损, 波形, CD 音质 | `pcm` | 同 6.9 | wav / pcm_s16le | pcm |
| `flac` | FLAC | 无损, 无损压缩 | `flac` | 同 6.9 | flac / flac | flac |
| `ogg` | OGG | Vorbis, Ogg 音频 | `vorbis` | 同 6.9 | ogg / libvorbis | vorbis opus copy |
| `opus` | Opus | 语音, 网络音频 | `opus`，128 kbps | 同 6.9（原来就能输出，v0.24 正式进目录） | opus / libopus | opus copy |
| `wma` | WMA | Windows 音频, 微软音频 | `wma`，192 kbps | — | asf / wmav2 | wma |
| `amr` | AMR | 录音, 手机录音, 语音 | `amr_nb` | 固定 `-ar 8000 -ac 1`，码率 12.2 kbps（`-b:a 12200`，用户设的码率忽略）；**依赖转换组件内置 `libopencore_amrnb`**，项目默认安装的版本没有，显示为不可输出 | amr / libopencore_amrnb | amr_nb |
| `m4r` | M4R | 苹果铃声, 铃声, iPhone 铃声 | `aac`，192 kbps | `-f ipod -movflags +faststart`（就是改了扩展名的 M4A）；**不自动截短、不阻止**；前端只显示灰色提示 `iPhone 铃声最长 40 秒，更长的文件可能不能设为铃声。`（v0.24.1 PM 9，不引导去裁剪） | ipod / aac | aac copy |
| `mp2` | MP2 | MPEG 音频, 广播 | `mp2`，224 kbps | 采样率由转换组件自动换成编码器支持的值 | mp2 / mp2 | mp2 |
| `ape` | APE | 无损, Monkey's Audio | `ape` | **转换组件没有 APE 编码器（也没有 APE muxer），永远不可输出**，靠检测得出、不写死；APE 文件可以作为输入（6.16.4） | ape / ape | ape |
| `wv` | WV | 无损, WavPack | `wavpack` | — | wv / wavpack | wavpack |
| `mmf` | MMF | 手机铃声, 彩铃 | `adpcm_yamaha` | 固定 `-ar 22050 -ac 1`（mmf 只支持单声道和 4 / 8 / 11.025 / 22.05 / 44.1 kHz，实测立体声和 48 kHz 都失败） | mmf / adpcm_yamaha | adpcm_yamaha |

**图片**（`category=image`；单帧规则见 6.16.5）：输入是视频时，把**第 1 秒那一帧**存成图片（视频不足 1 秒时取第一帧）；输入是图片时直接转格式。不出序列帧。

| extension | displayName | aliases | 默认参数 | 固定参数 / 说明 | 检测用 muxer / encoder |
|---|---|---|---|---|---|
| `jpg` | JPG | 照片, 图片, JPEG | 无（保持原尺寸） | `-c:v mjpeg -q:v 2` | image2 / mjpeg |
| `png` | PNG | 透明图片, 截图, 图片 | 无 | `-c:v png` | image2 / png |
| `webp` | WebP | 网页图片, 图片 | 无（保持原尺寸） | `-c:v libwebp -quality 80` | image2 / libwebp |
| `ico` | ICO | 图标 | 无 | 缩到 256×256 以内（`scale='min(256,iw)':'min(256,ih)':force_original_aspect_ratio=decrease`，不放大），`format=rgba`，`-c:v png -f ico`；用户设的宽高也不能超过 256（`INVALID_ARGUMENT`） | ico / png |
| `bmp` | BMP | 位图, 图片 | 无 | `-c:v bmp` | image2 / bmp |
| `tif` | TIF | TIFF, 印刷, 图片 | 无 | `-c:v tiff -compression_algo lzw` | image2 / tiff |
| `tga` | TGA | Targa, 游戏贴图 | 无 | `-c:v targa` | image2 / targa |

- **别名**（PM 规则 5）：表里的 `aliases` 就是返回值（顺序照抄）；`无损` 出现在 FLAC、APE、WV、WAV 上，`苹果` 在 MOV、M4A 上，`苹果铃声` 在 M4R 上，`动图`、`图片` 在 GIF 上（v0.24.1 架构师定：GIF 仍归视频类，加 `图片` 方便搜到），`网页图片` 在 WebP 上。别名以后可以只追加。
- **`paramsSummary`**（6.14.5）新编码的显示名：`mpeg4` → `MPEG-4`，`mpeg2` → `MPEG-2`，`wmv2` → `WMV`，`flv1` → `FLV`，`theora` → `Theora`；图片格式的摘要是 `单帧`（加尺寸段，如 `单帧 · 宽 256`），没有其他段。
- **内置预设（v0.24：从 12 个增加到 38 个）**：原来的 12 个 id、名称、参数都不变；新增 26 个，保证**每个格式都有一个内置默认预设**（即使格式当前不可输出，如 APE，下拉里也有它，整体置灰）。`ListPresets` 返回全部（内置在前，按下表顺序；用户预设在后），`GetFormatCatalog` 按格式分组返回。内置预设仍不能修改或删除（6.9）。提交时前端把选中预设的 `options`（用户改过的话是改后的值）作为 `SubmitSources` 的 `options` 传，选的是预设且没改时 `presetId` 传它的 id（快照规则同 6.14.2）。

  | 格式 | 内置预设（**粗体是默认预设**；id：名称） |
  |---|---|
  | mp4 | **`builtin-mp4-h264`：MP4（H.264 + AAC，通用）**；`builtin-mp4-h264-1080p`：MP4 1080p（H.264 + AAC）；`builtin-mp4-h264-720p`：MP4 720p（H.264 + AAC）；`builtin-mp4-h265`：MP4（H.265 + AAC，体积更小） |
  | mkv | **`builtin-mkv-h264`：MKV（H.264 + AAC）**（新增）；`builtin-mkv-copy`：MKV（不重新编码，只换封装） |
  | mov | **`builtin-mov-h264`：MOV（H.264 + AAC）** |
  | webm | **`builtin-webm-vp9`：WebM（VP9 + Opus）** |
  | gif | **`builtin-gif`：GIF 动图（宽 480，12 帧/秒）** |
  | mp3 / m4a / wav / flac | **`builtin-mp3`：MP3（192 kbps）**；**`builtin-m4a`：M4A（AAC 192 kbps）**；**`builtin-wav`：WAV（无损 PCM）**；**`builtin-flac`：FLAC（无损压缩）** |
  | 其余 25 个格式（新增） | 各一个，id 为 **`builtin-<extension>`**，参数就是上表的“默认参数”：`builtin-avi` AVI（Xvid + MP3）、`builtin-flv` FLV（H.264 + AAC）、`builtin-wmv` WMV（WMV2 + WMA）、`builtin-mpg` MPG（MPEG-2 + MP2）、`builtin-vob` VOB（MPEG-2 + AC-3）、`builtin-3gp` 3GP（H.264 + AAC）、`builtin-swf` SWF（Flash 视频）、`builtin-ogv` OGV（Theora + Vorbis）、`builtin-aac` AAC（192 kbps）、`builtin-ogg` OGG（Vorbis）、`builtin-opus` Opus（128 kbps）、`builtin-wma` WMA（192 kbps）、`builtin-amr` AMR（语音 12.2 kbps）、`builtin-m4r` M4R 铃声（AAC 192 kbps）、`builtin-mp2` MP2（224 kbps）、`builtin-ape` APE（无损）、`builtin-wv` WV（WavPack 无损）、`builtin-mmf` MMF（手机铃声）、`builtin-jpg` JPG 图片、`builtin-png` PNG 图片、`builtin-webp` WebP 图片、`builtin-ico` ICO 图标（最大 256×256）、`builtin-bmp` BMP 图片、`builtin-tif` TIF 图片、`builtin-tga` TGA 图片 |

  新内置预设随升级写进 `presets` 表（现有机制：按 id 更新内置行，`sort` 按上表顺序），不需要新迁移。

### 6.16.3 `encodable` 的检测

- 后端运行 `ffmpeg -hide_banner -muxers` 和 `ffmpeg -hide_banner -encoders`（每个 10 秒超时，经 `ffmpeg.NewCommand`，不占任务槽位），解析出 muxer 名集合（`-muxers` 每行第二列，逗号分隔的每个名字都算）和编码器名集合（同 9.6 的 `parseEncodersList`）。
- 每个格式按 6.16.2 表里的“检测用 muxer / encoder”判断（就是默认预设实际要用的那些）：muxer 不在 → `encodable=false`、`reasonCode="missing_muxer"`；muxer 在但任一编码器不在 → `reasonCode="missing_encoder"`；两种的 `reason` 都是 `当前转换组件不支持输出这个格式`（PM 文案）。**不写死哪个格式能用**：APE、AMR 是否可用完全由检测结果决定。
- 只检测默认预设要用的编码器（同一格式下其他预设和用户预设若用了别的编码，提交时按第 6.16.6 条单独检查，目录里不逐个标）；用户把编码改成别的（例如 MKV 选 VP9）时，提交前按第 6.16.6 条单独检查。
- 结果按“转换组件可执行文件路径 + 大小 + 修改时间”缓存（6.16.1）；检测在第一次 `GetFormatCatalog` 时进行（同时只跑一次，并发调用等同一个结果）。**v0.25.4**：若此时检测还在进行，先等检测结束（最多 6 秒，见 6.16.1）再跑上面两条命令；探测失败或仍未就绪的结果不写入缓存，组件路径、大小或修改时间变化后缓存失效。

### 6.16.4 输入：哪些文件能添加

- **输入扩展名** = 目录里全部 34 个 `extension`（**含不可输出的 APE，以及 AMR 等可能不可输出的格式**：只要转换组件能解码就能当输入，PM 规则 4）+ `jpeg tiff m4v mpeg ts mts m2ts aif aiff`。`AddSources` 按它过滤（6.15.4 第 1 条），前端的 `PickFiles` 过滤器用同一份列表（前端写死这份列表并与本条保持一致；单独再提供一个“所有文件 `*`”选项也可以，选进来不在列表里的文件由 `AddSources` 逐项拒绝）。
- 能不能真的转换由提交时的探测决定（6.9：探测失败 `PROBE_FAILED`，与参数不兼容 `INVALID_ARGUMENT`）。
- 图片作为输入：只能转成图片格式（6.16.5）；图片转视频 / 音频容器不在本版范围（按 6.9 原规则：没有音轨转音频容器是 `INVALID_ARGUMENT`；图片转视频容器的行为不变、不保证）。

### 6.16.5 图片输出（单帧，不做序列帧）

- **图片输入**（输入扩展名是 `jpg jpeg png bmp webp tif tiff tga ico gif` 之一）→ 取**第 0 帧**（动图 GIF / 动态 WebP 只取第一帧）。
- **视频输入**（其他有画面的输入）→ **取第 1 秒那一帧；视频不足 1 秒时取第一帧**（UI 设计定，取代本版草稿里“同缩略图规则”的写法）。时长从探测结果取：时长 ≥ 1 秒用 `-ss 1`（放在 `-i` 前面），时长 < 1 秒用第 0 帧；时长未知时先按 `-ss 1` 取；用 `-ss 1` 没有产出图片时，**不论退出码**都按第 0 帧重试一次（v0.24.1 实测：ffmpeg 6 定位超过结尾时正常退出但不写文件，ffmpeg 7 以 `Error while opening encoder`、退出码 234 失败），第 0 帧也失败时报第二次的错误。按旋转元数据转正（ffmpeg 默认的 autorotate）。封面图（`attached_pic`）不算画面，只有封面的音频文件转图片是 `INVALID_ARGUMENT`（`输入文件没有视频画面，无法转成 <格式>`，同 6.9）。
- **只出一个文件**：`-frames:v 1`，image2 输出另加 `-update 1`（文件名里带 `%` 也不会被当成序列模板）。**v1 不做图片序列**（每隔几秒一张之类），也没有“导出全部帧”。
- **允许的参数**：只有 `width` / `height`（同 6.9 的缩放规则；ICO 另有 256 上限）；`trimStart`、`fps`、`videoBitrate`、`audioBitrate`、`crf`、`trimEnd`、`videoCodec`、`audioCodec`（只能是 `""`，`audioCodec` 也可以是 `none`）设了都是 `INVALID_ARGUMENT`（`图片格式不能设置裁剪、帧率、码率或画质`）。取帧时间固定，不能调（v1）。
- 进度：单帧输出没有可用的时长进度，任务从 0 直接到 1（成功）；`result` 只有 `sizeBytes`、`width`、`height`。
- 输出命名同 6.14.5：`<原文件名去扩展名>.<图片扩展名>`，重名 ` (n)`。

### 6.16.6 提交时的格式检查（`SubmitSources` / `Submit` / `Reconvert`）

在 6.9 的“先整体校验再提交”里，`ValidateConvertOptions` 通过之后、探测文件之前（对整个请求只查一次，与副本是否就绪无关；所以格式不可输出时即使有就绪的行也整体失败，不走 6.15.4 第 6 条的“跳过”）：
1. `options.container` 对应的格式 `encodable=false` → 整体 `UNSUPPORTED`，`detail` 只有一行 `reason=format`，message `当前转换组件不支持输出这个格式`（含 `converter_not_ready` 的情况：此时 6.9 本来就先返回 `FFMPEG_NOT_FOUND`）。
2. 格式本身可输出，但用户选的编码（`videoCodec` / `audioCodec` 改成了非默认值）在当前转换组件里没有对应编码器（如没有 `libvpx-vp9` 时选 VP9）→ 整体 `UNSUPPORTED`，`detail` 只有一行 `reason=encoder`（2.2 新值），message `当前转换组件不支持所选的编码`。硬件编码器不在这里检查（9.7 照旧自动回退 CPU）。
3. 用的是第 6.16.3 条的同一份缓存；缓存没有时先检测一次。`SavePreset` 不做这项检查（预设只是数据，换了转换组件可能又能用了）。
4. 原地重试（`TaskService.Retry`）同样在重建 Runner 时做这两项检查，失败时记录原样不动（6.6）。

## 6.17 原地重转（`Reconvert`，v0.24，老板定；细节由 PM、前端、设计定；v0.24.1 定稿）

取代 v0.23 / v0.23.1 的“`Reconvert` 新增一条记录、v1 转换页没有入口”。**不新增方法，沿用 `Reconvert`，只改语义**：在**同一条记录**上原地重转，任务 id 不变，输出路径和文件名也不变（**不加 “(1)”**）。新结果先写同目录的临时文件，**成功后才原子替换**旧输出。转换页在**已成功**的记录上提供“重转”入口。失败和取消的记录仍用 `TaskService.Retry`（按钮文案“重新转换”，6.14.6），不变。**用词（v0.24.1）**：与重转有关的用户文案一律说“重转”，“重新转换”只留给 `Retry`。

### 6.17.1 接口与同步校验

```go
Reconvert(req ReconvertRequest) (Task, error)   // v0.24：参数由 taskID string 改为结构体（v1 前端没有调用方）；返回更新后的记录

type ReconvertRequest struct {
    TaskID   string          `json:"taskId"`
    PresetID string          `json:"presetId,omitempty"` // 可选：选了同格式的某个预设
    Options  *ConvertOptions `json:"options,omitempty"`  // 可选：自定义参数（或改过的预设参数）
}
```

- **参数可选**：`presetId` 和 `options` 都没给时，沿用记录当前的参数快照（`params.options` / `presetId` / `presetName` / `paramsSummary` 全部不变）。只给 `presetId` 时，用这个预设的参数，快照记这个预设。给了 `options` 时用它；这时 `presetId` 的含义同 `SubmitSources`：选了预设且没改就传它的 id，自定义参数传 `""`，快照按 6.14.2 / 6.14.5 重新生成。
- **只能用同一种格式**：新参数的 `container`（也就是输出扩展名）必须和记录当前的相同，否则返回 `INVALID_ARGUMENT`，message `重转不能更换格式，要换格式请新转一条。`（v0.25 产品经理定，更正 v0.24.6 的写法），`detail` 首行 `reason=format_change`。`presetId` 指向别的格式的预设也是这个错误；预设不存在时返回 `NOT_FOUND`（`reason=record`，同 `SubmitSources`）。**前端可以给的预设** = `GetFormatCatalog()` 里这个扩展名那一项的 `presets[]`（6.16.1，内置在前、用户预设在后，含默认预设）。
- **只允许 `succeeded` 的 `convert` 记录**，并且它当前不在重转（`reconverting=false`）。其他状态（`queued` / `running` / `failed` / `canceled` / `interrupted`），或已经在重转，都返回 **`TASK_CONFLICT`，`detail` 首行 `reason=invalid_state`**，message 分别是 `只有已完成的记录可以重转` 和 `这条记录正在重转`。错误码沿用现有的 18 个，没有新增 `INVALID_STATE`（v0.24.1 架构师定）。不是 `convert` 任务返回 `INVALID_ARGUMENT`；不存在或是旧类型返回 `NOT_FOUND`（`reason=record`）。
- **旧输出决定模式（设计定；v0.24.1 PM 15 改写）**：
  - `outputPath` 上**有文件**，且仍是这条记录的输出（6.14.4 第 3 步的安全条件：绝对路径、上级不含符号链接、本身不是符号链接、是普通文件、不等于任何输入路径、修改时间不早于这条记录 `startedAt − 3 秒`）→ **`replace` 模式**：成功后原子替换旧文件（6.17.5）。
  - `outputPath` 上**没有文件**（被移动或删除了，`Lstat` 报不存在）→ **`regenerate` 模式（PM 15a）**：按**原来的参数**重新生成到同一个路径。只能用原参数：请求里给了 `presetId` 或 `options` 返回 **`INVALID_ARGUMENT`，`detail` 首行 `reason=params_locked`**，message `原来的输出文件不在了，只能按原来的参数重新生成`。同样先写临时文件，落盘前复核目标**仍不存在**，**绝不覆盖**（这期间目标位置出现了文件，按 `output_moved` 失败，6.17.5）。失败或取消时记录仍是 `succeeded`、输出仍不在，失败写 `lastReconvertError`。
  - 其余情况（目标位置是**别的文件**（PM 15b）、是符号链接或目录、`outputPath` 不是绝对路径、`Lstat` 出别的错）→ **`TASK_CONFLICT`，`detail` 首行 `reason=output_moved`**，message `原来的输出文件已被移动或替换，不能重转`。**后端不会另写新文件**。界面上这种记录不提供“重转”。
- **源文件不在时不能重转（设计定）**：检查这一行**当前**的读取路径（副本 `ready` 用 `storedPath`，`none` 用 `originalPath`，6.15.4 第 4 条）。不是普通文件时返回 **`NOT_FOUND`，`detail` 首行 `reason=file`，第二行是那个路径**，message `源文件不存在，无法重转`。这里沿用 2.2 已有的 `reason=file`，不新增 `reason=source`：`Reconvert` 里输出文件的问题都走上一条的 `output_moved`，所以 `reason=file` 只会指源文件。界面上置灰入口。
- **前端怎么提前知道**（用来置灰或隐藏入口）：`TaskService.CheckPaths` 的 `TaskPathCheck` 新增两个字段（v0.24.1 架构师定名，取代 v0.24 草稿的 `canReconvert`，后者已删除）：`reconvertMode string`（`"replace"` / `"regenerate"` / `""`，`""` = 不能重转）和 `reconvertBlock string`（`reconvertMode=""` 时的原因：`invalid_state` / `copy_not_ready` / `source_missing` / `output_moved`，否则 `""`）。两个字段始终输出；`found=false` 时都是 `""`；不是 `convert` 的任务是 `invalid_state`。判断规则和顺序同 `Reconvert` 的同步校验，只是不校验参数。`output_moved` 时界面**不显示**入口，`source_missing` / `copy_not_ready` 时**置灰**（设计定）；`regenerate` 时前端只能不带参数调用。
- **校验顺序**（多个问题同时存在时，报第一个；v0.24.1 架构师定）：记录不存在 / 旧类型（`NOT_FOUND` `reason=record`）→ 不是 `convert` → 状态（`invalid_state`）→ 副本没就绪（`copying` / `copy_failed`）→ 源文件（`NOT_FOUND` `reason=file`）→ 旧输出（`output_moved`）→ 旧输出不在时改了参数（`params_locked`）→ 换格式（`format_change`）→ 其余参数和格式检查。
- 其余同步校验同 `SubmitSources`：参数校验（6.9）、格式检查（6.16.6，`UNSUPPORTED` `reason=format` / `encoder`）、读取路径与副本状态（读这一行**当前**的读取路径；副本没就绪时 `TASK_CONFLICT`，`reason=copying` / `copy_failed`，6.15.4 第 6 条）。**任何同步错误都不改记录、不发事件。**

### 6.17.2 新字段

`Task`（第 3 节）新增，只有 `convert` 任务会用到：

```go
Reconverting       bool                `json:"reconverting"`                 // 始终输出；true = 正在原地重转（status 是 queued / running），界面显示“重转中”和进度
LastReconvertError *ReconvertError     `json:"lastReconvertError,omitempty"` // 最近一次重转失败的信息；重转成功、被取消、或又开始一次重转时清空

type ReconvertError struct {
    Code    string `json:"code"`    // AppError.code，如 PROCESS_FAILED、IO_ERROR、CONVERT_DISK_FULL
    Message string `json:"message"` // AppError.message（面向用户，1.1）
    Detail  string `json:"detail,omitempty"` // AppError.detail（2.2 规则；替换失败时首行是 reason=in_use / permission / io）
    At      int64  `json:"at"`      // 失败时间，Unix 毫秒
}
```

`task:status` 事件新增两个字段：`reconverting`（凡是 `convert` 任务的 `task:status` 都带）和 `reconvertOutcome`。后者**只在重转结束那一次**的终态事件里出现，取值 `"succeeded"` / `"failed"` / `"canceled"` / `"interrupted"`，其余事件省略。

库：迁移 **`0008_reconvert.sql`**（v0.24 原写 `0007`；`0007` 是 6.15.3 的 `storage_copies`）：

```sql
ALTER TABLE tasks ADD COLUMN reconverting INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN reconvert_prev TEXT;          -- 重转开始前的快照 JSON（6.17.3），重转中才有，结束后清空
ALTER TABLE tasks ADD COLUMN reconvert_pending TEXT;       -- 替换前写入的“成功后的新状态” JSON（6.17.5 第 3 步），替换完清空
ALTER TABLE tasks ADD COLUMN last_reconvert_error TEXT;    -- ReconvertError JSON，可空
```

旧行都是 0 / NULL，`reconverting=false`。

### 6.17.3 开始重转时（`Reconvert` 通过校验后，一个事务）

1. **保存快照** `reconvert_prev`：`params`、`inputPaths`、`outputPath`、`result`、`progress`、`startedAt`、`finishedAt`、`encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason`，以及旧输出文件的 `{sizeBytes, mtimeNs}`（6.17.1 已确认它是这条记录的输出；`regenerate` 模式没有旧文件，记下模式本身）。
2. **目标就是同一个 `outputPath`**：`replace` 成功后替换旧文件，`regenerate` 成功后在目标仍不存在时落盘。6.17.1 已经排除了“目标被换成别的文件”的情况，所以不会另定新名。这个名字本来就由这条记录持有，重转期间继续按 6.14.5 占位（占位人是这个任务 id）。
3. **更新记录**：`reconverting=true`、`status=queued`、`progress=0`、`speed=""`、`etaSec=0`、`error` 无、`lastReconvertError` 清空，编码器字段按新 Runner 写入，`hiddenInTaskCenter=false`（任务中心 `List` 能看到它在跑；结束后保持不隐藏，见 6.17.7），`version` +1。行的 `lastActivityAt` = 现在。
4. **不变**（重转期间对外仍是旧的）：`id`、`type`、`title`、`sourceId`、`createdAt`、**`params`（对外仍是旧快照，新参数只在内部 Runner 和 `reconvert_pending` 里）**、`inputPaths`、**`outputPath`**、**`result`**、**`finishedAt`**（旧的完成时间）。`startedAt` 在真正开始运行时改成这次的开始时间（同普通任务）。
5. 日志末尾追加一行 `[FFmpegFree] 重转：<新 paramsSummary>`。发**一次** `task:status`（`status: "queued"`、`reconverting: true`、`progress: 0`、编码器字段；`retried` 不带），然后入队。**不发** `task:created`。

### 6.17.4 重转期间

- 状态照常走 `queued` → `running`，`task:progress` 照常发（界面显示“重转中”和进度）。可以 `TaskService.Cancel`。
- **旧结果和旧文件照常可用**：`result`、`outputPath`、`finishedAt` 都是旧的。`TaskService.GetPreviewURL(taskId, "output")`、`TaskService.OpenWithSystem(taskId, "output")`、`RevealRecord`、`GetRecordThumbnail` 对 `reconverting=true` 的记录按 `succeeded` 处理，作用在**旧文件**上，直到替换完成（这几个接口的“只有 `succeeded`”条件放宽为“`succeeded` 或 `reconverting`”）。
- **临时文件**：`<目标所在目录>/<目标文件名去扩展名>.reconvert-<taskId>.part.<扩展名>`，例如 `D:\out\婚礼.reconvert-01J….part.mp4`。和目标同一目录，保证改名是原子的；`.part.<扩展名>` 结尾让 ffmpeg 按扩展名选封装（同 `RunWithPart`）。名字里带 `taskId`，同一时刻每条记录最多一个。

### 6.17.5 结束：成功、失败、取消、中断

**成功**（ffmpeg 正常结束）：

1. 探测临时文件，得到新的 `result`（6.14.6，含 `warnings`；探测失败只有 `sizeBytes`）。
2. **复核目标**：确认 `outputPath` 仍与快照的 `{sizeBytes, mtimeNs}` 一致、仍是普通文件、不是符号链接。不一致或不在了（重转期间被用户移动或替换），就**不覆盖、也不另写新文件**，按“失败”处理：`lastReconvertError` 的 `code` 是 `TASK_CONFLICT`，`detail` 首行 `reason=output_moved`，message `原来的输出文件在重转期间被移动或替换，新结果没有保存`。**`regenerate` 模式**的复核条件是目标**仍不存在**；这期间目标位置出现了任何文件，同样按这条失败，**不覆盖**。
3. 把“成功后的新状态”（新 `params` 快照、新 `inputPaths`、新 `result`；`outputPath` 不变）写进 `reconvert_pending` 并落库。
4. **原子替换**：同目录 rename，把临时文件改名到目标（Unix `rename(2)`；Windows `MoveFileExW(MOVEFILE_REPLACE_EXISTING)`，即 Go 的 `os.Rename`）。`regenerate` 模式改用“不覆盖”的落盘（同 `RunWithPart` 的提交：目标已存在就失败，按第 2 步的 `output_moved` 处理）。旧输出被占用时**不事先提示**，直接按下面的 `in_use` 失败（v0.24.1 架构师定）。
5. 一个事务里：应用 `reconvert_pending`，`status=succeeded`、`progress=1`、`finishedAt=现在`、`startedAt` 为这次的开始时间，编码器字段为这次的；清空 `reconverting` / `reconvert_prev` / `reconvert_pending` / `lastReconvertError`；`version` +1。
6. 发终态 `task:status`（`status: "succeeded"`、`reconverting: false`、`reconvertOutcome: "succeeded"`、`outputPath`，新的 `result` / `finishedAt`）。参数可能变了，前端需要时用 `TaskService.Get` 或 `ListSourceRecords` 重新取 `params`（`task:status` 不带 `params`）。

**失败**（ffmpeg 失败，或第 2～4 步出错，包括复核不通过和替换失败）：

- 删掉临时文件。**旧文件不碰。**
- **整条记录恢复成 `succeeded`**：按 `reconvert_prev` 原样恢复 `params`、`inputPaths`、`outputPath`、`result`、`progress`（即 1）、`startedAt`、`finishedAt`、编码器字段。清空 `reconverting` / `reconvert_prev` / `reconvert_pending`，`error` 为空（记录本身仍是成功的）。
- **`lastReconvertError` = 这次的错误** `{code, message, detail, at}`，`version` +1。
- 发终态 `task:status`：`status: "succeeded"`、`reconverting: false`、**`reconvertOutcome: "failed"`**，带 `lastReconvertError`。前端显示“**重转失败，原来的文件没有变动。**”（前端文案），详情可以用 `lastReconvertError.message`。
- 替换失败时的错误是 `IO_ERROR`，`detail` 首行 `reason=<值>`，第二行是目标路径：

  | reason | message | 什么时候 |
  |---|---|---|
  | `in_use` | `输出文件正被其他程序使用，新结果没能替换进去` | 同 6.14.4 的判断（Windows 32 / 33，Unix `EBUSY` / `ETXTBSY`） |
  | `permission` | `没有权限替换输出文件` | 权限不足、只读 |
  | `io` | `替换输出文件失败` | 其他错误（详细原因只写日志） |

  ffmpeg 本身失败时的错误同普通转换（6.9 的分类）。

**取消**（`TaskService.Cancel`，或删除前的取消，6.17.6）：

- 同失败的恢复步骤，区别只有两点：**`lastReconvertError` 清空，不写错误**；终态事件里是 **`reconvertOutcome: "canceled"`**。前端看到 `canceled` 什么都不显示。

**中断**（应用退出时重转还在跑）：

- 退出收尾时**只删临时文件**，发终态事件 `reconvertOutcome: "interrupted"`，**不落库**（v0.24.1 PM 13）：库里仍是 `reconverting=1`，下次启动由下面的崩溃恢复恢复成 `succeeded`（`lastReconvertError` 不写）并**计数**。
- **`ConvertService.TakeInterruptedReconverts() int`**（v0.24.1 新增）：返回本次启动恢复的“上次退出时被中断的重转”条数，**第一次调用返回这个数，之后都返回 0**。前端在转换页显示一次 `上次退出时有 n 条重转被中断，原来的文件没有变动。`（n = 返回值，0 时不显示）。

**两者怎么分**：失败和取消都回到 `succeeded`，前端只看终态事件的 `reconvertOutcome`，或者看记录上有没有 `lastReconvertError`（重新加载列表时用这个；`failed` 有、`canceled` / `interrupted` 没有）。

**启动时的清理（崩溃恢复）**：在 `store.MarkInterrupted` **之前**处理所有 `reconverting=1` 的行（它们不进“`running` → `interrupted`”的规则）：

1. 有 `reconvert_pending`，临时文件**不在**、目标文件在：说明第 4 步的改名已经做了，就按成功收尾（第 5 步），只是不发事件。
2. 其余情况（没有 `reconvert_pending`，或临时文件还在）：删掉临时文件（只删普通文件、不是符号链接），按 `reconvert_prev` 恢复成 `succeeded`，`lastReconvertError` 不写（同中断）。这一类的条数就是 `TakeInterruptedReconverts` 返回的数（第 1 类已经成功，不算）。
3. **孤儿临时文件扫描**：对每个 `convert` 记录 `outputPath` 所在目录（去重），以及实际输出目录（6.15.1），删掉匹配 `*.reconvert-<taskId>.part.*` 的普通文件，条件是 `<taskId>` 不是“正在重转”的记录（启动时还没有这样的记录，所以全部可删）。不递归，不碰别的文件，失败只记日志。

### 6.17.6 删除、重试与其他接口

- **重转中删除**（`DeleteRecords` / `DeleteSource`）按这个顺序（设计定）：① 沿用 6.14.4 第 2 步，**先取消**；② 取消按 6.17.5 **删掉临时文件**，并把记录恢复成 `succeeded`；③ `deleteOutputs=true` 时删**旧输出**，安全条件按恢复后的 `outputPath` 和 `startedAt` 判断，不满足时是 `not_task_output`；④ 删记录。删除时还会兜底再删一次这条记录的临时文件 `*.reconvert-<taskId>.part.*`，不管 `deleteOutputs`，也不计入 `deletedFiles`，失败只记日志（同 6.14.4 第 4 步对 `.part` 的处理）。10 秒内没停下来的照旧是 `still_running`，这时临时文件和旧输出都不动。
- **`TaskService.Retry` 不变**：只用于 `failed` / `canceled` / `interrupted` 的记录（转换页按钮“重新转换”）。重转结束后记录总是 `succeeded`，所以 `Retry` 和重转不会碰到同一条记录。
- 名字顺延和占位的其余规则不变（6.14.5）；`PreviewOutputName` 与本节无关。

### 6.17.7 任务中心

- v1 任务中心**不提供**“重转”入口。
- `TaskService.List` 照常返回这条记录，带 `reconverting` 和实时的 `status` / `progress`。重转开始时 `hiddenInTaskCenter` 置为 `false`，所以默认列表能看到它在跑。结束后**不恢复**原来的隐藏状态（v0.24.1 架构师定：重转会取消隐藏，并保持不隐藏），用户可以再隐藏。

## 6.18 语音工具（v0.29，一期仅转字幕；产品 / 架构 10-09 定）

> **一期只实现转字幕**（`speech_to_subtitle`）。侧栏一级入口名 **「语音工具」**，页内**只出现「转字幕」这一个标签**（另外三个标签**完全不出现**，不是置灰）。  
> **语音识别的 Python CLI / 模型由老板另行构建与发布**，本仓库**不实现、不打包**该 CLI；后端只实现 **适配器**（下载组件、按约定工作目录调用入口二进制、解析 JSON 结果）。  
> 文字转语音 / 字幕进视频 / 音色克隆：**设计已定、实现延后**（6.18.9），本期无 UI 入口、无任务提交。  
> 文案与命名必须逐字使用「语音工具」「语音识别组件」等（见 6.18.1）；界面与 `message` **不出现** Whisper、模型文件名、`ffmpeg`、Python、错误码（同 1.1）。

### 6.18.1 定稿命名

| 用途 | 定稿文案 | 禁止出现 |
|---|---|---|
| 侧栏 / 顶栏 | **语音工具** | 「语言」「语言页」 |
| ASR 下载包 | **语音识别组件** | Whisper、模型文件名、引擎名、`语言组件` |
| 任务中心类型（一期） | **转字幕** | 英文 `speech_to_subtitle`、Whisper、ffmpeg |
| （延后，命名先占位） | **语音音色** / **音色克隆组件**；任务文案「文字转语音」「字幕进视频」「音色克隆」 | 同上技术词 |

侧栏顺序：转换 → **语音工具** → 文档 → 直播 → 工具 → 任务中心。

### 6.18.2 一期范围：`speech_to_subtitle`

| 项 | 规则 |
|---|---|
| 任务类型 | `Task.type = speech_to_subtitle`（库内英文；界面只显示「转字幕」） |
| 依赖 | **语音识别组件** + **转换组件**（抽音频）；缺转换组件 → `FFMPEG_NOT_FOUND`；缺识别组件 → `LANG_ASR_NOT_READY` |
| 抽音频 | 转换组件产出临时 **16 kHz、单声道 WAV**，落在 `<数据目录>/tmp/lang/<taskId>/`；**永不**出现在界面 / 任务标题 / 「打开所在文件夹」；任务终态删除 |
| 识别 | 适配器调用组件内入口二进制（6.18.4）；进 **ASR 池，并发固定 1** |
| 高清 × 硬件编码 | 正在跑 **高清**档 ASR 时，本机 NVENC / AMF / QSV 视频编码并发 **≤ 1** |
| 成功 | `TaskResult` 带可编辑 `cues`（`id` / `text` / `startMs` / `endMs`）；用户可改字、改起止时间，再 `ExportSubtitleCues`；**不做边听边改** |
| 空识别 | `LANG_ASR_EMPTY`，**不可重试**，文案 `这段音频里没有识别到有效内容。`；用户换文件再提交 |

### 6.18.3 语音识别组件：状态、档位、下载

形态对齐文档组件状态机（`checking` / `ready` / `missing` / `outdated` / `downloading` / `preparing` / `failed`），可断点续传、SHA-256、下前磁盘检查（空间不足 `CONVERT_DISK_FULL` `reason=no_space`）。

```go
type LangAsrStatus struct {
    State         string    `json:"state"`
    Version       string    `json:"version"`
    Source        string    `json:"source"`        // ready 时 downloaded；一期无系统安装来源
    Tier          string    `json:"tier"`          // standard | hd，与 Settings.asrTier 一致
    CanDownload   bool      `json:"canDownload"`
    DownloadBytes int64     `json:"downloadBytes"` // 当前档安装包字节；界面「约 xxx MB」
    InstallBytes  int64     `json:"installBytes,omitempty"`
    Phase         string    `json:"phase,omitempty"`
    ReceivedBytes int64     `json:"receivedBytes,omitempty"`
    Error         *AppError `json:"error,omitempty"`
    Path          string    `json:"-"`
}
```

| 档位 | `Settings.asrTier` | 引导体积（写死在界面，发布前可改数字） | 说明 |
|---|---|---|---|
| **标准**（**默认**） | `standard` | **约 400 MB** | 默认档 |
| **高清** | `hd` | **约 1–1.5 GB** | **只在设置里切换**；**只改下载引导体积与 Install 所选包，切换本身不自动下载** |

- 组件目录：Windows `%LocalAppData%\FFmpegFree\components\lang\asr\<tier>\<version>\`；macOS `~/Library/Application Support/FFmpegFree/components/lang/asr/<tier>/<version>/`（Linux 路径同应用数据目录约定，**TBD** 是否提供下载包）。  
- 下载 URL / SHA-256 / 精确字节数：**TBD（老板发布组件包后写入）**；契约先留占位，实现用配置或常量，未配置时 `InstallLangAsr` 返回明确错误（建议 `LANG_DOWNLOAD_FAILED`，message `暂时无法下载语音识别组件，请稍后重试。`）。  
- 引导文案（产品）：标题 `下载语音识别组件，才能转字幕`；正文示例 `需要下载语音识别组件（约 400 MB），现在下载吗？当前是「标准」档；设置里可换「高清」。`；按钮「稍后」「下载」。  
- `OpenStorageFolder("lang_asr")`：打开当前档已安装目录；不可用 → `NOT_FOUND`，`detail` 一行 `reason=component`（不带路径）。仅 `state=ready` 时前端显示按钮。

### 6.18.4 ASR 适配器协议（本仓只实现调用方）

> **不**实现、**不**捆绑 Python / 模型。老板发布的「语音识别组件」包解压后须满足下列布局与协议；适配器按此调用。

**布局（解压后的 `<组件根>` = 上节 `…/asr/<tier>/<version>/`）**：

| 路径 | 含义 |
|---|---|
| `<组件根>/asr.exe`（Windows）或 `<组件根>/asr`（macOS / Linux） | **入口二进制**（名字固定 `asr` / `asr.exe`） |
| `<组件根>/` 其余文件 | 由发布方自定（模型、运行时等）；适配器**不解释** |

**工作目录**：每次识别在任务临时目录运行，`WorkDir = <数据目录>/tmp/lang/<taskId>/asr/`（先建空目录）。进程的 cwd 设为该目录；组件根只读，通过参数传入。

**调用**（超时、取消随任务 `ctx`；超时建议 **TBD**，可先 30 分钟）：

```text
asr --component-root <组件根绝对路径> --request <request.json 绝对路径> --response <response.json 绝对路径>
```

- 退出码 `0`：读 `response.json`。  
- 非 0：失败；`detail` 可带 `exit=<码>`（只给开发者看）；用户文案走 `LANG_ASR_FAILED` 或归入空结果 / 损坏等（见 6.18.8）。  
- stderr 写入任务日志（受 1.1 限制：展示给用户的 message 仍不得出现技术词）。

**`request.json`（UTF-8）**：

```json
{
  "version": 1,
  "audioPath": "<16kHz mono wav 绝对路径>",
  "language": "auto",
  "tier": "standard"
}
```

| 字段 | 说明 |
|---|---|
| `version` | 协议版本，一期固定 `1` |
| `audioPath` | 抽好的临时 WAV |
| `language` | `auto` \| `zh` \| `en` \| …（一期允许的取值 **TBD**，至少支持 `auto`） |
| `tier` | `standard` \| `hd`，与当前组件档一致 |

**`response.json`（UTF-8，成功时）**：

```json
{
  "version": 1,
  "cues": [
    { "id": "01J…", "text": "你好", "startMs": 0, "endMs": 1200 }
  ]
}
```

| 字段 | 说明 |
|---|---|
| `cues` | 可空数组；**空或全无有效文本** → 任务失败 `LANG_ASR_EMPTY` |
| `cues[].id` | 非空字符串；CLI 可自生成；后端若发现空 id 则补 ULID |
| `cues[].text` | 字幕文本 |
| `cues[].startMs` / `endMs` | 毫秒；须 `endMs > startMs`；导出前再经 6.18.6 硬校验 |

适配器把 `cues` 写入 `TaskResult`（JSON，**无迁移**）。进度：CLI 若在 stderr 打进度行，格式 **TBD**；一期可只靠不确定进度 + 任务 `running`。

### 6.18.5 接口与事件（一期）

```go
// LangService（名称可微调；Wails Bind）
GetLangAsrStatus() (LangAsrStatus, error)
InstallLangAsr(tier string) (LangAsrStatus, error) // "" = Settings.asrTier；standard|hd
CancelLangAsrInstall() error
RecheckLangAsr() (LangAsrStatus, error)

SubmitSpeechToSubtitle(req SpeechToSubtitleRequest) (Task /* 或批量结果，TBD */, error)
ExportSubtitleCues(req ExportSubtitleRequest) (ExportSubtitleResult, error)

type SpeechToSubtitleRequest struct {
    Paths     []string `json:"paths"`
    Language  string   `json:"language,omitempty"` // 默认 auto
    Format    string   `json:"format"`             // 导出意向 srt|vtt
    OutputDir string   `json:"outputDir,omitempty"`
}
type SubtitleCue struct {
    ID      string `json:"id"`
    Text    string `json:"text"`
    StartMs int64  `json:"startMs"`
    EndMs   int64  `json:"endMs"`
}
type ExportSubtitleRequest struct {
    TaskID     string        `json:"taskId"`
    Cues       []SubtitleCue `json:"cues"`
    Format     string        `json:"format"` // srt|vtt
    TargetPath string        `json:"targetPath,omitempty"`
}
```

事件：

| 事件 | payload | 何时 |
|---|---|---|
| `lang:asr` | `LangAsrStatus` | 状态变化；检测结束至少一次 |
| `lang:asr-progress` | `{ phase, receivedBytes, totalBytes, progress? }` | 下载中；preparing 可不带 progress |
| `task:created` / `task:progress` / `task:status` | 现有第 5 节 | 转字幕任务 |

设置键：`asrTier` = `standard`（默认）\| `hd`；非法值 `INVALID_ARGUMENT`；切换不触发下载。

### 6.18.6 `ExportSubtitleCues` 硬校验

失败 → `INVALID_ARGUMENT`（建议 `reason=cue_invalid`），**不**部分写入、**不**警告放行：

1. 每条 `endMs > startMs`  
2. 任意两条时间轴**不得重叠**  
3. 每条 `text` 最多 **80** 个 Unicode 字符  

通过后写 SRT 或 VTT：**UTF-8 不带 BOM**；同步写盘，不新建 task type。

### 6.18.7 任务中心

- 类型列：「转字幕」。  
- `title` 示例：`采访.mp4 → 字幕`（最终格式可微调，不得出现技术词）。  
- 进度 / 失败 / 重试走现有任务中心；`LANG_ASR_EMPTY` 不可重试。

### 6.18.8 错误码（v0.29 新增 5 个；2.1 由 31 变为 36）

| code | message | 可重试 | 说明 |
|---|---|---|---|
| `LANG_ASR_NOT_READY` | `需要先下载语音识别组件。` | 是（就绪后） | 提交 / 开始识别时组件非 ready |
| `LANG_ASR_EMPTY` | `这段音频里没有识别到有效内容。` | **否** | 无有效 cues；换文件再提交 |
| `LANG_ASR_FAILED` | `识别没完成，请稍后重试。` | 是 | CLI 非 0、响应无法解析、超时等（不是空结果） |
| `LANG_DOWNLOAD_FAILED` | `语音识别组件下载失败，请检查网络后重试。` | 是 | 含 URL/包尚未配置、解包/准备失败（`detail` 可带 `phase=preparing`，界面不显示） |
| `LANG_CHECKSUM_FAILED` | `下载的语音识别组件校验失败，请重试。` | 是 | SHA-256 不符 |

沿用：`FFMPEG_NOT_FOUND`、`CONVERT_DISK_FULL`、`INVALID_ARGUMENT`（含导出 cue 校验、参数）、`CANCELED`、`IO_ERROR`、`NOT_FOUND`（`reason=component` 等）。

### 6.18.9 设计已定、实现延后（无 UI 入口）

下列名称与规则**保留**，避免日后扯皮；**本期不出现标签、不下载、不提交任务**。

| 能力 | 任务类型 | 组件名 | 要点（已定） |
|---|---|---|---|
| 文字转语音 | `tts` | **语音音色**（约 80 MB，2 个中文音色） | 整段合成再播；URL 同转换页预览白名单；文稿最多 10000 字，超限 `文字太长，最多 1 万字。`；TTS 池并发 1 |
| 字幕进视频 | `subtitle_embed` | （仅转换组件） | 软 / 硬字幕各一默认样式；无字体颜色字号设置 |
| 音色克隆 | `voice_clone` | **音色克隆组件**（引导 **约 1.5 GB**，句式 `需要下载约 1.5 GB。`；发布前可用实测替换） | 仅本机；引导句与授权句见产品定稿；采样 10s–10min、wav/mp3/m4a；`ListTtsVoices` 含 `source=cloned` |

完整字段与接口以仓库外草稿 `/workspace/ffmpegfree/docs-draft/language-contract-v0.1.md`（v0.1.2）为准，实现延期时再开契约修订合入。

### 6.18.10 未决（不阻塞一期适配器）

1. 老板发布后的标准 / 高清包 URL、SHA-256、精确 `downloadBytes` / `installBytes`。  
2. Linux 是否提供识别组件包。  
3. `language` 枚举全集；CLI 进度协议。  
4. 转字幕输入扩展名与时长 / 大小上限。  
5. `SubmitSpeechToSubtitle` 单文件还是批量返回形态。  
6. `LANG_ASR_FAILED` 最终文案产品确认。


## 6.19 Cat 助手（v0.30，一期仅 Cat Build；产品 / 老板 10-09 定）

> **一期只适配 Grok Build / Cat Build 路径。** 欢迎页模式胶囊里**只有「Cat Build」可点**；「Cat CLI」「Cat Code」「Codex CLI」「更多」等**一律置灰**，悬停 / 旁注文案 **`下一期开放`**（产品可再改字，本期先用这句）。**不**适配 Cat CLI / Cat Code / Codex CLI 的运行时。  
> 界面与 `message` **只出现「Cat 助手」**；**永不**出现：`grok`、`Grok`、`CLI`、`命令行`、可执行文件名、模型厂商名（同 1.1）。  
> 本仓实现 **适配器注册表 + Build 适配器**（下载 / 检测组件、按约定调用入口二进制、转发消息与只读工具结果）；**入口二进制名与发布包 URL/SHA-256 TBD**（老板后续提供 grok-build 相关 CLI）。  
> **会话创建时锁定 `agentKind`**：之后改应用默认 CLI **不得**影响已有会话；Send / 流式始终走该会话自己的适配器（6.19.2）。  
> 开关：前端 `CAT_UI_ENABLED` **默认 `false`**（侧栏「Cat」入口隐藏；为 true 时出现在「语音工具」后，可带「新」角标）；`CAT_BACKEND_READY` 为 false 时走本地假数据 / 不调真绑定（与前端 flags 对齐）。

### 6.19.1 一期范围与明确不做

| 做 | 不做（一期） |
|---|---|
| 侧栏进入 Cat 全屏聊天（收起主侧栏，返回原页） | Cat CLI / Cat Code / Codex CLI **运行时**（胶囊可见但禁用） |
| 欢迎页 + 会话列表 + 发送 / 回复（事件推进） | URL / API Key 设置页或设置项 |
| **仅 `cat_build`（Cat Build）** 可创建并运行 | 「助手」「定时任务」入口与功能 |
| 模型名 / 思考强度**只从该会话 `agentKind` 对应 CLI 的能力列表读取** | 前端写死第三方模型名 |
| 工具调用：**只读**（读文件 / 列目录等，白名单 TBD） | **完全访问**（入口置灰；不可落到 full） |
| 组件未就绪文案见下 | 任意写盘、执行任意命令（非只读） |

**未就绪**（组件未发布 / 未安装 / 不可用）：`Cat 助手还没准备好，发布后即可使用。`（一期通常无下载按钮，直至 `canDownload=true`）。  
**回复失败**：`回复没生成出来，请重试。`（`CAT_REPLY_FAILED`，可重试）。

### 6.19.2 `agentKind` 锁定与适配器注册表（多 CLI 扩展预备）

| `agentKind` | 欢迎胶囊（展示名） | 一期 |
|---|---|---|
| `cat_build` | Cat Build | **唯一可创建 / 可发送** |
| `cat_cli` | Cat CLI | 禁用「下一期开放」；注册表可不注册 |
| `cat_code` | Cat Code | 同上 |
| `codex_cli` | Codex CLI | 同上 |

**规则（架构硬性）**：

1. **`CatConversation.agentKind` 在创建时写入，之后不可变**（无 Update 接口改 kind；库内也不允许改）。  
2. **`CreateCatConversation`（或首条 `SendCatMessage` 隐式建会话）**必须带上用户在欢迎页选中的模式对应的 `agentKind`；一期前端只能传 `cat_build`，其它值 → `INVALID_ARGUMENT`。  
3. **`SendCatMessage` / 流式 / 取消**一律按**该会话已存的 `agentKind`** 查注册表路由，**绝不**使用「当前应用默认 CLI」。  
4. **应用级默认** `Settings.catDefaultAgentKind`（默认 `cat_build`）**只作用于新建会话**的预填；改设置不影响旧会话。  
5. **适配器注册表**（后端进程内）：`agentKind` → `{ executablePath, protocolVersion, … }`。一期**只注册 `cat_build`**；未注册 kind 的会话若因脏数据出现 → `CAT_NOT_READY` 或 `UNSUPPORTED`（实现选一，message 仍用未就绪 / 通用失败口径，不暴露可执行名）。  
6. **`ListCatConversations` / `GetCatConversation` 返回 `agentKind`**，供 UI 徽章或内部逻辑；**展示给用户仍是「Cat 助手」**，不把 `cat_build` / grok / cli 字样直接打在气泡上（可用中性「Build」等产品另定的短标，**TBD**；禁止技术名）。

### 6.19.3 Build 适配器（`cat_build`）

- 组件目录建议：`%LocalAppData%\FFmpegFree\components\cat\build\<version>\`（macOS 对应 Application Support）；每轮工作目录 `<数据目录>/tmp/cat/<conversationIdOrTurnId>/`。  
- 入口二进制：**名字 TBD**（占位 `cat-build` / `cat-build.exe`，以老板发布包为准）。路径只进注册表，**不**回前端。  
- 能力探测：CLI 输出 JSON → `models[]` / `thinkLevels[]`（展示名须为「Cat 助手 …」口径，或后端映射后只把展示名给前端）。**按 agentKind 缓存**；不同 kind 不得混用能力列表。  
- 一轮对话：写 request（消息、modelId、thinkLevelId、只读工具结果、可选只读项目根）→ 调该 kind 的可执行文件 → 读 response；写类工具请求拒绝。  
- 协议字段、超时、取消细节：**TBD**（稳定后回写）。  
- **无** Base URL / API Key 设置。

### 6.19.4 数据结构

```go
type CatStatus struct {
    State       string    `json:"state"` // checking | ready | missing | failed
    Version     string    `json:"version"`
    CanDownload bool      `json:"canDownload"`
    Error       *AppError `json:"error,omitempty"`
}

type CatModel struct {
    ID          string `json:"id"`
    DisplayName string `json:"displayName"` // 如「Cat 助手 1.0」
}
type CatThinkLevel struct {
    ID          string `json:"id"`
    DisplayName string `json:"displayName"` // 如「高」
}

type CatConversation struct {
    ID        string `json:"id"`
    Title     string `json:"title"`
    AgentKind string `json:"agentKind"` // 创建后不可变；一期仅 cat_build
    CreatedAt int64  `json:"createdAt"`
    UpdatedAt int64  `json:"updatedAt"`
}

type CatMessage struct {
    ID        string `json:"id"`
    Role      string `json:"role"` // user | assistant | system
    Content   string `json:"content"`
    CreatedAt int64  `json:"createdAt"`
}
```

会话表 / JSON 落库须持久化 `agentKind`；**尽量无现有键值或新表，迁移号顺延（下一号以仓库为准，可能为 0010+）**——若实现选 JSON 旁路字段且无迁移，须在实现 PR 注明。

### 6.19.5 接口与事件

```go
GetCatStatus() (CatStatus, error)
RecheckCat() (CatStatus, error)
ListCatModels(agentKind string) ([]CatModel, error)           // 一期只接受 cat_build
ListCatThinkLevels(agentKind string) ([]CatThinkLevel, error)

ListCatConversations() ([]CatConversation, error) // 每项含 agentKind
GetCatConversation(id string) (CatConversationDetail, error)
CreateCatConversation(req CreateCatConversationRequest) (CatConversation, error)
// req 必填 agentKind（一期 = cat_build）；可选 title
DeleteCatConversation(id string) error

SendCatMessage(req SendCatMessageRequest) (SendCatMessageResult, error)
// req: conversationId, content, modelId, thinkLevelId；可选 projectPath（只读根）
// 路由：用会话已存 agentKind，忽略「当前默认」
// 返回：必带 turnId + userMessage；流式下 assistantMessage 空，见 6.19.9
CancelCatTurn(req CancelCatTurnRequest) error // v0.30.2：{ convId, turnId }，见 6.19.9
// v0.31：ListCatProjects / CreateCatProject / RenameCatProject / DeleteCatProject，见 6.19.10；
// CreateCatConversation 新增可选 projectId；SendCatMessage 的 projectPath 作废（忽略）
```

| 事件 | 含义 |
|---|---|
| `cat:status` | `CatStatus` 变化 |
| `cat:message` | **仅助手流式增量**（用户消息不发）；形态见 6.19.9 |
| `cat:turn` | 一轮状态 `{ convId, turnId, status }`（`running|completed|cancelled|failed`），见 6.19.9 |
| `cat:project` | v0.31：项目 `missing` 变化 `{ id, missing }`，见 6.19.10.3 |

设置：`catDefaultAgentKind` 默认 `cat_build`，**仅新建会话预填**。

### 6.19.6 UI 冻结项

- 欢迎页：**仅「Cat Build」可选**；其余禁用 + 「下一期开放」。选中 Build 后 `Create`/`Send` 带 `agentKind=cat_build`。  
- 合一胶囊：「{模型展示名} · {思考强度展示名}」，选项来自**该会话 agentKind** 的 List API。  
- 「完全访问」置灰。无「助手」「定时任务」。无 URL/Key 设置。

### 6.19.7 错误码（v0.30 新增 2 个；2.1 由 36 变为 38）

| code | message | 可重试 |
|---|---|---|
| `CAT_NOT_READY` | `Cat 助手还没准备好，发布后即可使用。` | 是（就绪后） |
| `CAT_REPLY_FAILED` | `回复没生成出来，请重试。` | 是 |

非法 `agentKind`、改已有会话 kind 等 → `INVALID_ARGUMENT`。沿用 `NOT_FOUND` / `CANCELED` / `IO_ERROR` 等。

### 6.19.8 延后

`cat_cli` / `cat_code` / `codex_cli` 注册与运行；完全访问；助手；定时任务；URL/Key；组件下载专节。（流式事件已提前到 6.19.9，下一个包做。）扩展时**只加注册表项与胶囊启用**，会话锁定规则不变。


### 6.19.9 流式事件与取消（v0.30.1；v0.30.2 对齐 #174）

> 本节定**应用内事件与绑定形态**；Build CLI 侧流式输出协议（stdout 分帧、如何映射到下面的 op）**待老板发布 CLI 后补**，后端适配器负责把 CLI 输出翻译成本节事件。**不是二期**，排在下一个包。

**`SendCatMessage` / `SendCatMessageResult`（v0.30.2）**

```go
type SendCatMessageResult struct {
    UserMessage      CatMessage `json:"userMessage"`
    TurnID           string     `json:"turnId"`                     // 必填；供 CancelCatTurn / 停止按钮
    AssistantMessage *CatMessage `json:"assistantMessage,omitempty"` // 流式下为空/省略
}
```

- 同步路径：校验 + 落库用户消息后返回 `{ userMessage, turnId }`；助手回复在后台跑，**全靠事件推送**。
- **流式下 `assistantMessage` 为空或省略**；助手正文只经 `cat:message` / `cat:turn`。
- **用户消息不发 `cat:message`**（只在同步 `userMessage` 里带）；`cat:message` **仅用于助手流式**。
- **未就绪**（组件/CLI 未发布或 `state != ready`）：同步返回 `CAT_NOT_READY`，**不启 turn**、不发流式事件；界面只展示「Cat 助手还没准备好，发布后即可使用。」。**官方包禁止**在未就绪时造助手气泡、禁止拆字/模拟回复。
- **临时适配器回退**（组件已就绪、但 CLI 真流式尚未接线）：可将**真实整段回复**按约 24 字拆成多次 `append` 再 `done`（切真内容，非假文案）。真 CLI 流式落地后，该拆分**只留测试**，不得进官方包运行路径。

**`cat:message`（增量；仅助手）**

```ts
type CatMessageEvent = {
  convId: string
  turnId: string
  messageId: string                 // 助手消息入库 id，与事件一致
  seq: number                       // 同一 messageId 内从 1 起严格递增
  op: 'append' | 'replace' | 'done'
  block?: { type: string; text?: string } // replace 时携带整块内容（如 text / tool_result 等，type 枚举随实现扩展）
  textDelta?: string                // append 时携带追加的文本片段
}
```

| op | 含义 |
|---|---|
| `append` | 在该 `messageId` 末尾追加 `textDelta`；`seq` 递增 |
| `replace` | 用 `block` 替换该消息当前内容（或当前块，实现定口径须稳定） |
| `done` | 该消息结束，之后不再有同 `messageId` 事件 |

- 典型顺序：若干 `append`（`seq` = 1, 2, …），最后一次 `done`。
- 前端按 `(messageId, seq)` 去重 / 丢弃乱序旧包；同一轮可有多条 `messageId`（如助手正文 + 工具块）。

**`cat:turn`**

```ts
type CatTurnEvent = {
  convId: string
  turnId: string
  status: 'running' | 'completed' | 'cancelled' | 'failed'  // 无 succeeded / canceled
}
```

- 一轮开始发 `running`；终态只发一次 `completed` | `cancelled` | `failed`。
- `failed` 时用户可见文案仍是 **`CAT_REPLY_FAILED`** → `回复没生成出来，请重试。`（错误详情可走同步返回或附加字段，界面不显示 code）。

**`CancelCatTurn`**

```go
type CancelCatTurnRequest struct {
    ConvID string `json:"convId"`
    TurnID string `json:"turnId"`
}
```

- `convId` 为空 → `INVALID_ARGUMENT`。
- `turnId` 为空：取消该会话**当前进行中**的一轮（兼容旧调用）。
- `turnId` 对不上 / 没有进行中的一轮 / 重复取消 → 返回 **nil**（不报错）。
- 取消时：**已流出文字保留入库**，气泡末尾**不加**后缀；若已出过字则先发该消息的 `done`，再发一次 `cat:turn` `cancelled`；迟到增量丢弃。
- 界面在对话里追加一条**淡色系统行**：`已停止生成。`；停止按钮在点击后**立即置灰**（不等后端确认）。

**无障碍**：`prefers-reduced-motion: reduce` 时，前端**直接追加**收到的文本，不做逐字打字机闪烁动画。

### 6.19.10 项目（v0.31；v0.31.1 补重新选择文件夹 / 在文件管理器中显示；v0.31.2 口径澄清；产品规则 10-09 定，架构师定方案）

> 项目 = 用户选的**一个本机文件夹**；对话可以属于一个项目，也可以不属于任何项目（显示在「对话」下）。项目文件夹同时是该项目下对话的**只读工具沙箱根**。本节与 6.19.1~6.19.9 冲突时以本节为准；会话 `agentKind` 锁定、流式事件、未就绪规则不变。

**产品规则（产品经理定，原样执行）**

1. 项目就是用户选的一个本机文件夹；名字默认取文件夹名，可改名。
2. 一个对话只属于一个项目，也可以不属于任何项目，放在「对话」下。
3. 删除项目**只删应用里的记录和它下面的对话**，**绝不动用户文件夹里的任何文件**；删之前确认，文案：`删除项目会同时删除它下面的对话，文件夹里的文件不受影响。`（删除按钮红色，属 UI）。
4. 文件夹被移走或删掉时，项目标灰，提示 `项目文件夹不见了。`；对话仍能看，**不能继续发消息**。

#### 6.19.10.1 数据结构

```go
type CatProject struct {
    ID        string `json:"id"`
    Name      string `json:"name"`      // 去首尾空白后 1~60 个字符（按 Unicode 字符计）
    Path      string `json:"path"`      // 用户选的绝对路径（Clean 后原样保存，用于显示与沙箱根）
    CreatedAt int64  `json:"createdAt"` // Unix 毫秒
    UpdatedAt int64  `json:"updatedAt"` // 改名时更新；不参与排序
    Missing   bool   `json:"missing"`   // 后端实时计算（6.19.10.4），不落库；始终输出
}

type CreateCatProjectRequest struct {
    Path string `json:"path"`           // 必填：绝对路径、已存在的文件夹、不能是盘符 / 文件系统根
    Name string `json:"name,omitempty"` // 可选：空 = 文件夹名
}
type CreateCatProjectResult struct {
    Project CatProject `json:"project"`
    Existed bool       `json:"existed"` // true = 这个路径已经建过项目，返回的是已有的那个，什么都没新建
}

type RenameCatProjectRequest struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}
type DeleteCatProjectRequest struct {
    ID string `json:"id"`
}

// v0.31.1
type RelocateCatProjectRequest struct {
    ID   string `json:"id"`
    Path string `json:"path"` // 新文件夹；规则同 CreateCatProjectRequest.Path
}
type RevealCatProjectRequest struct {
    ID string `json:"id"`
}

type CatProjectEvent struct { // 事件 cat:project
    ID      string `json:"id"`
    Missing bool   `json:"missing"`
}
```

`CatConversation`（6.19.4）新增：

```go
    ProjectID string `json:"projectId,omitempty"` // 所属项目；空 / 省略 = 不属于任何项目（显示在「对话」下）；库里 NULL
```

`CreateCatConversationRequest` 新增可选 `projectId`（string，空 = 不属于任何项目）。

**规则**：

1. **`projectId` 在创建对话时写入，之后不可变**（与 `agentKind` 锁定同一精神，6.19.2）：没有任何接口能改对话的项目，库里也不允许改。
2. **一期没有「移动对话到项目」/「移出项目」操作**；对话只能通过「在某个项目下新建」（`CreateCatConversation` 带 `projectId`）获得项目。界面不提供拖拽、菜单等任何移动入口。
3. **旧字段作废**：v0.30 的 `CreateCatConversationRequest.projectPath`、`SendCatMessageRequest.projectPath`、`CatConversation.projectPath`（库列 `cat_conversations.project_path`）从 v0.31 起**后端一律忽略、不再写入、不再读取**，字段暂留只为绑定兼容（下一次整份重生成绑定时删除）。只读工具的根只来自对话所属项目（6.19.10.5）。旧会话 `project_id` 为 NULL，统统显示在「对话」下。

#### 6.19.10.2 接口（CatService，v0.31 新增 / 修改）

```go
ListCatProjects() ([]CatProject, error)
CreateCatProject(req CreateCatProjectRequest) (CreateCatProjectResult, error)
RenameCatProject(req RenameCatProjectRequest) (CatProject, error)
DeleteCatProject(req DeleteCatProjectRequest) error
RelocateCatProject(req RelocateCatProjectRequest) (CatProject, error) // v0.31.1
RevealCatProject(req RevealCatProjectRequest) error                   // v0.31.1
// 修改：CreateCatConversation(req) 的 req 新增可选 projectId
// 修改：ListCatConversations() 每项带 projectId（不属于项目的省略）
```

选文件夹**复用** `SystemService.PickDirectory(title)`（6.8），前端传 `选择项目文件夹`；用户取消返回 `""`，前端什么都不做、不调 `CreateCatProject` / `RelocateCatProject`（v0.31.1：「重新选择文件夹」用同一个对话框和标题）。不新增对话框方法。

1. **`ListCatProjects()`**：返回全部项目，每项的 `missing` 在本次调用里实时计算（6.19.10.4）。没有项目返回空数组（不是 null）。排序见 6.19.10.6。
2. **`CreateCatProject({path, name?})`**：
   1. `path` 校验：去首尾空白后必须是**绝对路径**、**存在**且是**文件夹**；不满足 → `INVALID_ARGUMENT` `reason=project_path`。是**盘符根或文件系统根**（`filepath.Dir(p) == p`，含 `C:\`、`/`、UNC 共享根 `\\server\share\`）→ `INVALID_ARGUMENT` `reason=project_root`。
   2. **路径去重**：比较键 `path_key` = `filepath.Clean` 后去掉末尾分隔符；**大小写规则与仓库已有 `paths.Normalize` / `media.path_key` 一致**：**Windows 与 macOS 不区分大小写（键转小写），Linux 区分（原样）**。不解析符号链接、不展开短文件名（8.3）。同一 `path_key` 已有项目 → **不报错、不新建、不改名**（忽略本次 `name`），返回 `{project: 已有项目, existed: true}`；前端提示 `这个文件夹已经建过项目了。` 并切到该项目。新建成功 `existed=false`。**实现注意（v0.31.2）**：后端 #181 的 `CreateCatProject` / `RelocateCatProject` 目前**只在 Windows 转小写**，macOS 尚未与 `paths.Normalize` 对齐，需跟进修；修完后按 6.19.10.10 真机验。
   3. **名字**：`name` 去首尾空白；为空则取文件夹名（`filepath.Base`），文件夹名超过 60 个字符时截到前 60 个；用户给的 `name` 超过 60 个字符或含换行 / 控制字符 → `INVALID_ARGUMENT` `reason=project_name`。**名字允许重复**（不同文件夹可以同名）。
   4. 新建的项目 `missing=false`，`createdAt = updatedAt = 当前时间`。
3. **`RenameCatProject({id, name})`**：`name` 规则同上，但**不能为空**（去空白后为空 → `INVALID_ARGUMENT` `reason=project_name`）；`id` 不存在 → `NOT_FOUND`（`找不到这个项目。`）。**`missing` 的项目也可以改名**。只改名字和 `updatedAt`，不改排序。返回改名后的项目（`missing` 实时计算）。
4. **`DeleteCatProject({id})`**：
   1. `id` 为空 → `INVALID_ARGUMENT`；`id` 不存在 → 返回 **nil**（幂等，可重复删）。
   2. **`missing` 的项目也可以删**。
   3. 该项目下任何对话有进行中的一轮：**先按 `CancelCatTurn` 的规则取消**（6.19.9：已出文字先 `done`，再一次性发 `cat:turn` `cancelled`），**最多等 5 秒**等这一轮停掉再删，避免回复尾巴写回库；超时仍删（迟到文字丢弃，与 6.19.9 取消口径一致）。
   4. 在**一个显式事务**里删除：该项目下所有对话的 `cat_messages` → 这些 `cat_conversations` → `cat_projects` 这一行；任何一步失败整体回滚，返回 `IO_ERROR`（`删除项目失败，请重试。`）。**不依赖外键级联**（见 6.19.10.7）。
   5. **绝不访问用户文件夹**：不删、不改、不建任何文件；删除前后都不 stat 也可以。
   6. 删除不发事件；前端在调用成功后从本地列表移除项目及其对话。
5. **`CreateCatConversation` 带 `projectId`**：`projectId` 非空且不存在 → `NOT_FOUND`（`找不到这个项目。`）；项目 `missing` → `CAT_PROJECT_MISSING`，不创建对话。`agentKind` 校验先于项目校验。
6. **`SendCatMessage`**：校验顺序：会话存在 → `CAT_NOT_READY`（6.19.9，全局未就绪优先）→ 会话有 `projectId` 时实时检查项目文件夹，`missing` → **同步返回 `CAT_PROJECT_MISSING`**：**不启 turn、不存用户消息、不发任何 `cat:message` / `cat:turn`**；状态由“不缺”变“缺”时同时发 `cat:project`。
7. **`GetCatConversation` / `DeleteCatConversation`** 对属于 `missing` 项目的对话照常可用（对话能看、能删）。**`DeleteCatConversation`**（v0.31.2 明确）：有进行中的一轮时同样**先按 `CancelCatTurn` 取消，最多等 5 秒**再删记录，避免回复尾巴写回库；`id` 不存在 → nil（幂等）。
8. **`RelocateCatProject({id, path})`（v0.31.1，「重新选择文件夹」）**：把项目换到另一个文件夹。**主要用途是 `missing` 的项目**（文件夹搬了家），正常项目也可以用。
   1. **校验顺序**（命中即返回，什么都不改）：
      1. `id` 为空 → `INVALID_ARGUMENT`（无 reason）；`id` 不存在 → `NOT_FOUND`（`找不到这个项目。`）。
      2. `path` 校验**与 `CreateCatProject` 完全相同**（第 2 条第 1 款）：不是绝对路径 / 不存在 / 不是文件夹 → `INVALID_ARGUMENT` `reason=project_path`；盘符根、文件系统根、UNC 共享根 → `INVALID_ARGUMENT` `reason=project_root`。stat 同样最多等 2 秒（6.19.10.4 第 5 条），超时按 `project_path`。
      3. 新路径算 `path_key`（第 2 条第 2 款同一规则：`filepath.Clean`、去末尾分隔符、**Windows / macOS 转小写、Linux 原样**；不解析符号链接、不展开 8.3）。**与本项目当前 `path_key` 相同 → 不改库、不改 `path` 的显示写法、不改 `updatedAt`**，直接返回本项目（`missing` 实时重算；由缺变不缺时照常发 `cat:project`）。
      4. `path_key` 已属于**另一个**项目 → `INVALID_ARGUMENT` `reason=project_duplicate`，`detail` 第二行 `projectId=<那个项目的 id>`，message `这个文件夹已经建过项目了。`。前端提示这句并**可以**切到那个项目（取不到 `projectId` 行就重新 `ListCatProjects`、只提示不切换）；本项目不变（仍是 `missing` 就仍是 `missing`）。**不合并两个项目的对话**。
      5. 该项目下**任何对话有进行中的一轮** → `TASK_CONFLICT` `reason=turn_running`，**不自动取消**（与 `DeleteCatProject` 不同：换文件夹不是要丢弃回复，由用户先点「停止」）。缺失项目一般不会有进行中的一轮（6.19.10.4 第 6 条的情况除外）。
   2. **更新**：在一个事务里改这一行的 `path`（Clean 后原样保存）、`path_key`、`updated_at = 当前时间`；**`name` 不变**（即使原名是旧文件夹名，也不自动改成新文件夹名）。撞 `path_key UNIQUE`（并发）→ 按第 4 款返回 `project_duplicate`。落库失败 → `IO_ERROR`。
   3. **对话和消息原样保留**：不改 `cat_conversations` / `cat_messages` 任何一行（`projectId` 不变，`updatedAt` 不变，所以排序不变——项目 `updatedAt` 本来就不参与排序，6.19.10.6）。历史消息里提到的旧路径不改写。之后的每一轮只读工具以**新路径**为沙箱根（6.19.10.5）。
   4. **绝不访问新旧两个文件夹里的文件**：不复制、不移动、不删除、不创建任何东西；只对新路径做 stat。
   5. 返回更新后的项目，`missing` 实时重算（刚校验过，一般为 `false`）；与后端内存里上一次的值不同（典型：`true → false`）时发 **`cat:project` `{id, missing}`**，与 6.19.10.3 同一规则。其下对话随即可以继续发消息。
9. **`RevealCatProject({id})`（v0.31.1，「在资源管理器中显示」等）**：在系统文件管理器里**打开项目文件夹本身**。
   1. **复用 `SystemService.OpenStorageFolder` 的“打开目录”做法**（6.15.2 第 6 条 = 6.8 的“path 是文件夹”分支）：Windows `explorer.exe "<dir>"`，macOS `open <dir>`，Linux `xdg-open <dir>`；命令启动后立即返回；启动失败 `PROCESS_FAILED`；Windows 路径含双引号 `INVALID_ARGUMENT`（同 6.8）。**实现说明（v0.31.2，不绑定前端）**：后端内部新增 `system.Manager.OpenFolder`，由 `OpenStorageFolder` 与 `RevealCatProject` 共用同一段打开目录代码；**不**暴露给前端 Bind。
   2. **只收 id、不收路径**：路径由后端从 `cat_projects` 取；**不走、也不加入 `RevealInFolder` 的放行范围**（6.8）。不把路径回给前端以外的任何地方（前端本来就有 `CatProject.path`）。
   3. `id` 为空 → `INVALID_ARGUMENT`；`id` 不存在 → `NOT_FOUND`（`找不到这个项目。`）；文件夹不在（按 6.19.10.4 实时 stat，2 秒超时按缺）→ **`CAT_PROJECT_MISSING`**（`项目文件夹不见了。`），状态变化时发 `cat:project`。
   4. **绝不写任何东西**：不 `MkdirAll`、不建 / 改 / 删文件、不改库（与 `OpenStorageFolder` 默认目录会 `MkdirAll` 不同）。

#### 6.19.10.3 事件

| 事件 | payload | 何时发 |
|---|---|---|
| `cat:project` | `{ id, missing }` | 某个项目的 `missing` 计算结果与后端内存里上一次的值**不同**时（变缺或恢复都发）；同一结果不重复发 |

- 后端内存为每个项目记上一次的 `missing`；进程启动后第一次计算只记录、**不发事件**（结果已在 `ListCatProjects` 返回值里）。
- 新建项目时记 `missing=false`；删除项目时丢弃记录。

#### 6.19.10.4 `missing` 的计算（不用文件监视）

1. `missing = true` 当且仅当对 `path` 的 `os.Stat` 失败（不存在、无权限、网络断开等任何错误）或结果不是文件夹。只看文件夹本身，不读里面的内容。
2. **何时计算**：每次 `ListCatProjects`（逐个项目）、每次 `SendCatMessage` 和 `CreateCatConversation`（只算该对话 / 请求的项目）、`RenameCatProject` 返回前（只算该项目）；v0.31.1 加 `RelocateCatProject`、`RevealCatProject`（只算该项目）。**不做文件监视**。
3. **前端在窗口重新获得焦点时**（以及进入 Cat 页、刷新列表时）调 `ListCatProjects`；变化经返回值和 `cat:project` 同步到界面。这就是“应用使用过程中状态变化”的全部来源。
4. **自动恢复**：文件夹回来（同一路径重新存在且是文件夹）后下一次计算 `missing=false`，发 `cat:project`，项目恢复正常、可以继续发消息；不需要用户操作。**v0.31.1 起**文件夹搬了家时用户也可以用「重新选择文件夹」（`RelocateCatProject`，6.19.10.2 第 8 条）把项目指到新位置，对话保留。
5. **超时**（网络盘）：每个路径的 stat 最多等 **2 秒**，超时按 `missing=true` 算（后台 stat 结束后丢弃结果，下一次重新算）；`ListCatProjects` 里各项目并行计算，整个调用不超过约 2 秒。
6. 一轮已经开始后文件夹才消失：**不中断这一轮**；之后只读工具读不到文件按工具失败返回给助手；下一次 Send 时才报 `CAT_PROJECT_MISSING`。

#### 6.19.10.5 只读工具的沙箱根

1. 对话属于项目时，**项目 `path` 就是只读项目工具（读文件 / 列目录等，6.19.1）的唯一根**；对话不属于项目时，**没有项目根，所有项目工具请求一律拒绝**。
2. 工具请求里的路径按**相对项目根**解析：绝对路径、含 `..` 跳出根、以及解析符号链接 / 目录联接（junction）/ 快捷方式后真实路径（`EvalSymlinks`）不在根的真实路径之内的，**一律拒绝**；比较按目录边界（Windows / macOS 不区分大小写），与 6.8 同口径。
3. **写文件、执行命令仍一律拒绝**（6.19.1，一期不变）。
4. 被拒绝的工具请求作为工具失败结果回给适配器，**不**是面向用户的错误码，不中断这一轮。
5. **无项目对话（v0.31.2）**：读文件 / 列目录等项目工具请求**只在适配器内**失败，失败说明固定为 `这个对话没有项目文件夹，不能查看文件。`；**不进界面、不弹横幅、不发面向用户的错误码**；**不再**使用旧的「下一期开放」那句。

#### 6.19.10.6 排序

1. **项目**：按“最近对话活动”倒序——项目的活动时间 = 其下对话 `updatedAt` 的最大值；没有对话的项目用自己的 `createdAt`。活动时间相同按 `createdAt` 倒序，再相同按 **`id` 倒序**（v0.31.2 明确：最后一级是 id 倒序）。
2. **项目内的对话**、以及「对话」（无项目）下的对话：按 `updatedAt` 倒序，相同按 `createdAt` 倒序，再相同按 **`id` 倒序**（与 v0.30 `ListCatConversations` 一致；v0.31.2 明确最后一级 id 倒序）。
3. 改名不改变排序（项目 `updatedAt` 不参与）。后端按此排好；前端分组时保持顺序。

#### 6.19.10.7 存储与迁移 `0011_cat_projects.sql`

下一个空闲迁移号以仓库为准：`0010_cat.sql` 已被 v0.30 用掉，本节用 **`0011`**。

```sql
CREATE TABLE cat_projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL,
    path_key   TEXT NOT NULL UNIQUE,   -- 6.19.10.2 第 2 条的比较键
    created_at INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL DEFAULT 0
);
ALTER TABLE cat_conversations ADD COLUMN project_id TEXT;   -- NULL = 不属于任何项目；不加外键
CREATE INDEX idx_cat_conversations_project ON cat_conversations (project_id, updated_at DESC);
```

- **删除用显式事务，不用外键级联**（架构师定）：`project_id` 不声明 `REFERENCES`，删除顺序和原子性由 `DeleteCatProject` 的事务保证（6.19.10.2 第 4 条）；`cat_messages` 对 `cat_conversations` 已有的 `ON DELETE CASCADE` 保留，但事务里仍显式先删消息，不依赖 `foreign_keys` 开关。
- `missing` 不落库。旧列 `cat_conversations.project_path` 保留不删（SQLite 不便删列），不再读写。
- `CreateCatProject` 的去重靠 `path_key UNIQUE`：插入撞唯一键时改为读出已有行，返回 `existed=true`（并发同时创建同一路径也只会有一行）。

#### 6.19.10.8 UI 冻结项（文案产品已定）

| 场景 | 文案 / 表现 |
|---|---|
| 没有项目 | 一行淡灰 `还没有项目。` |
| 选文件夹对话框标题 | `选择项目文件夹` |
| 同一文件夹已建过项目（`existed=true`） | 提示 `这个文件夹已经建过项目了。`，切到该项目 |
| 删除项目确认 | `删除项目会同时删除它下面的对话，文件夹里的文件不受影响。`（删除按钮红色） |
| 项目文件夹不见了（`missing=true` 或 `CAT_PROJECT_MISSING`） | 项目标灰，提示 `项目文件夹不见了。`；对话可看、输入框禁用；改名 / 删除仍可用；不能在其下新建对话；v0.31.1：提供「重新选择文件夹」（`PickDirectory("选择项目文件夹")` → `RelocateCatProject`），「在…中显示」可隐藏或置灰（调用会得到 `CAT_PROJECT_MISSING`） |
| 重新选择文件夹选到别的项目的文件夹（v0.31.1，`project_duplicate`） | 提示 `这个文件夹已经建过项目了。`，可切到 `projectId` 那个项目；本项目不变 |
| 重新选择文件夹时有对话正在回复（v0.31.1，`TASK_CONFLICT` `reason=turn_running`） | `有对话正在回复，请先停止再换文件夹。`（架构师建议文案，产品可改） |
| 在文件管理器中显示（v0.31.1，`RevealCatProject`；**非规范，前端按运行平台定**） | Windows `在资源管理器中显示`、macOS `在访达中显示`、Linux `在文件管理器中显示` |
| `INVALID_ARGUMENT` `reason=project_path` | `请选择一个文件夹。` |
| `INVALID_ARGUMENT` `reason=project_root` | `不能把整个磁盘作为项目，请选择里面的文件夹。` |
| `INVALID_ARGUMENT` `reason=project_name` | `名字需要 1~60 个字。`（v0.31.1 确认：上限 60 个字符，契约、后端、设计稿 v0.2 一致） |

（后三行是架构师建议文案，产品可改；前端按 `reason` 映射，1.1。）

前端开关**不变**：`CAT_UI_ENABLED` 仍默认 `false`，`CAT_BACKEND_READY` 不变；本版不新增开关。

#### 6.19.10.9 错误码（v0.31 新增 1 个；2.1 由 38 变为 39；v0.31.1 不新增，仍 39）

| code | message | 可重试 |
|---|---|---|
| `CAT_PROJECT_MISSING` | `项目文件夹不见了。` | 是（文件夹回来后） |

- 场景：`SendCatMessage`、`CreateCatConversation` 遇到 `missing` 的项目；v0.31.1 加 `RevealCatProject`（同步返回，无 `detail` 固定格式）。`RelocateCatProject` **不**返回它（新路径不在是 `INVALID_ARGUMENT` `reason=project_path`）。
- 沿用：`INVALID_ARGUMENT`（新增 `reason=project_path` / `project_root` / `project_name`；v0.31.1 加 `project_duplicate`，见 2.2）、`NOT_FOUND`（项目不存在，`找不到这个项目。`）、`IO_ERROR`（落库失败）；v0.31.1：`TASK_CONFLICT` `reason=turn_running`（`RelocateCatProject` 遇到进行中的一轮）、`PROCESS_FAILED`（`RevealCatProject` 启动文件管理器失败）。

#### 6.19.10.10 真机待验证（Windows；第 4、8 条另含 macOS）

1. 路径含**中文、空格**、括号的文件夹：建项目、去重、只读工具读文件。
2. **网络盘**（映射盘符和 UNC `\\server\share\dir`）：在线 / 断开时的 `missing`、2 秒超时、恢复后自动清除；UNC 共享根被拒绝。
3. **OneDrive 占位文件夹**（仅联机 / 按需文件）：stat 是否触发下载、只读工具读占位文件时的行为与耗时。
4. 同一文件夹用不同大小写 / 末尾带不带 `\` 选两次 → `existed=true`（**Windows**；**macOS** 在后端按 v0.31.2 对齐 `paths.Normalize` 之后真机验：同一文件夹不同大小写选两次 → `existed=true`；Linux 大小写不同算两个项目）。
5. 项目里的 junction / 符号链接指向项目外 → 被拒。
6. 删除项目后文件夹里文件一个不少（对比删除前后的文件清单）。
7. **重新选择文件夹**（v0.31.1）：把项目文件夹剪切到新位置后重新选择 → 对话、消息都在，可继续发消息，`cat:project` 恢复；新位置分别试 **OneDrive 文件夹**（含仅联机占位）、**网络盘**（映射盘符和 UNC 子目录）；选到别的项目的文件夹（含大小写不同）→ 重复提示且本项目不变；选同一文件夹 → 无变化；新旧文件夹里的文件一个不少。
8. **在文件管理器中显示**（v0.31.1）：Windows 资源管理器、macOS 访达各开一次（中文 / 空格路径、网络盘路径）；文件夹不在时返回 `项目文件夹不见了。` 且不会新建文件夹。

## 7. 本地流服务（已取消）

> **v0.25 补充**：直播预览新增只监听 `127.0.0.1` 随机端口的本机 HTTP 服务（只提供 `/live/<token>.flv`，见 6.10.3.5）；下面“后端不监听任何端口”的规定只对它放开，其他仍然有效。
>
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
- 失败时保留已下载部分，提示"重试"或"手动选择转换组件所在位置"（离线用户的出路；v0.24 起面向用户的文字不含 “ffmpeg”，1.1）。安装任务的 `title` 是 `安装转换组件`（v0.23 及之前是 `安装 ffmpeg`）。

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
    Version   string `json:"version"`   // 规范化后的数字版本，如 "9.0.2"、"7.1.5"（v0.22：不含 n 前缀、-static / 发行版后缀和网址）；git 主干构建等没有数字版本时是原样的前 32 个字符
    Source    string `json:"source"`    // v0.25.3 起始终有值：ready 时是实际在用的来源 custom | bundled | system | legacy；其他状态是用户的设置：custom（手动指定了路径）| default（没有手动指定）
    TaskID    string `json:"taskId,omitempty"` // installing 时对应的安装任务；无值时不输出，TS 中为 taskId?: string
    FFprobeMissing bool `json:"ffprobeMissing"`   // ready 但没有 ffprobe（v1 的 ffmpeg/ 目录），前端提示补全；探测 / 缩略图用 ffmpeg.RequireProbe() 门控
    CustomPathInvalid bool `json:"customPathInvalid"` // v0.25.3，始终输出：手动指定的路径不可用。ready + source≠custom = 用的是别处找到的组件、手动路径保留；missing / outdated = 手动路径也不能用。checking / installing / failed 时为 false。后端不带文字
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

**v0.25.3（N5）**：
- `source` 始终有值、`customPathInvalid` 的含义见上面的注释。检测顺序不变（9.1：手动路径优先），所以 `ready` 而 `source` 不是 `custom` 且设置里有手动路径，只能是手动路径不可用、退到了别处。
- **「恢复默认」用 `SetFFmpegPath("")`**（`UpdateSettings` 把 `ffmpegPath` 改成 `""` 等价）：任何状态（`ready` + 标记、`missing`、`outdated`、`failed`）都清掉手动路径并立即重新检测，发 `checking` 再发结果，返回值和最后一个 `ffmpeg:status` 一致，`customPathInvalid=false`。安装进行中时：清掉设置，返回当前的 `installing`，装完按新设置检测。
- 安装成功时设置里还有手动路径：按检测顺序重新检测一次（不直接标成 `bundled`），手动路径能用就继续用，不能用就用刚装好的并带标记。
- 前端提示文字由产品经理定，后端不带文字。
- **路径（v0.25.4，架构师定）**：`FFmpegStatus` 没有 `path`。`missing` / `outdated` 时 `error.detail` 逐行是 `[来源] 原因`，不含组件可执行文件路径，也不列出候选路径。`SetFFmpegPath` 校验失败的 `detail` 同样不含路径。打开组件所在文件夹只用 `OpenStorageFolder("component")`（内部用后端自己记下的路径，不回给前端）。应用日志仍写路径。

`missing` / `outdated` 时 `error` 为 `FFMPEG_NOT_FOUND`（v0.24 起 `message` 不含 “ffmpeg”：`未找到可用的转换组件` / `转换组件版本过低，需要 6 或更高`，1.1），`detail` 逐行是 `[来源] 原因`（给开发者看，可以含 ffmpeg 这个词，**不含路径**，v0.25.4）；`ready` 时为 null。`GetSettings` / `UpdateSettings`（第 4 节）中 `ffmpegPath`、`ffmpegPromptDismissed` 两项已实现，`UpdateSettings` 改 `ffmpegPath` 时同样校验，失败返回 `INVALID_ARGUMENT` 且整体不生效；其余字段随后续 PR 补充。

### 9.5 功能门控

- 依赖 ffmpeg 的：转换、剪辑、直播、媒体探测和缩略图。`state != ready` 时这些入口可以进，但操作按钮禁用，顶部显示提示条和"一键安装"按钮；后端对应 Service 统一返回 `FFMPEG_NOT_FOUND`，双保险：入口处调用 `ffmpeg.Require()`，未就绪返回该错误，就绪则返回 ffmpeg / ffprobe 的绝对路径，子进程一律用这个路径启动。
- 不依赖 ffmpeg 的：Office 转 PDF、PDF 预览、JSON 工具，始终可用（DocService 任何方法都不返回 `FFMPEG_NOT_FOUND`）。
- 首次启动检测到 `missing` 时弹一次确认框（"安装"或"稍后"），选"稍后"后写入 `Settings.ffmpegPromptDismissed = true`，之后只保留提示条，不再弹窗；ffmpeg 变为 ready 后该标记重置。
- 前端不轮询：检测完成、安装进度导致的 state 变化、手动指定路径、重新检测，都会推送 `ffmpeg:status`，payload 为完整 `FFmpegStatus`。

### 9.6 硬件编码器检测与偏好（v0.15，契约按架构师口头方案起草，如有出入以架构师为准）

```go
type EncoderNames struct {
    H264 string `json:"h264"` // 该设备上的 h264 编码器名，如 "h264_nvenc"；不支持为 ""
    HEVC string `json:"hevc"` // 如 "hevc_nvenc"；不支持为 ""
}
type EncoderDevice struct {
    ID        string       `json:"id"`        // "cpu"，或 "<vendor>-<序号>"，如 nvidia-0、intel-0、amd-0、apple-0；偏好里存它
    Name      string       `json:"name"`      // 给人看的名字，CPU 是 "CPU（软件编码）"
    Vendor    string       `json:"vendor"`    // nvidia | intel | amd | apple | unknown
    Kind      string       `json:"kind"`      // gpu | cpu
    Discrete  bool         `json:"discrete"`  // 独立显卡（auto 时独显优先于集显）；CPU 恒为 false
    Encoders  EncoderNames `json:"encoders"`
    Available bool         `json:"available"` // 试跑成功才为 true
    Reason    string       `json:"reason,omitempty"` // available=false 时的一行原因（可能偏技术，界面不必直接显示）
}
type EncoderDeviceList struct {
    FFmpegReady bool            `json:"ffmpegReady"` // false = ffmpeg 未就绪，没有做检测，devices 只有 cpu
    Devices     []EncoderDevice `json:"devices"`     // 第一项永远是 cpu（id "cpu"，encoders 为 libx264 / libx265，available=true）
}
type EncoderPreferenceInfo struct {
    ID        string `json:"id"`                // auto | cpu | 设备 id
    Name      string `json:"name"`              // "自动" | "CPU（软件编码）" | 设备名；设备不可用 / 不存在时用保存偏好时记下的名字（没记过为 ""）
    Available bool   `json:"available"`         // auto、cpu 恒为 true
    Reason    string `json:"reason,omitempty"`
}
```

**检测流程**（`ListEncoderDevices`）：
1. ffmpeg 状态不是 `ready`：直接返回 `{ffmpegReady:false, devices:[cpu]}`，不检测、不报错。
2. `ffmpeg -hide_banner -encoders`，解析出视频编码器集合。这一步失败（命令失败且没有输出）：返回仅 cpu，**不缓存**、不报错。
3. 枚举显卡名称（失败一律降级为空列表，不报错）：Windows 用 PowerShell `Get-CimInstance Win32_VideoController | Select-Object Name,PNPDeviceID | ConvertTo-Json`（wmic 已废弃）；macOS 用 `system_profiler SPDisplaysDataType -json`；Linux 先 `lspci -nn`，没有或没输出时读 `/sys/class/drm/card*/device/vendor`（只有厂商名）。虚拟适配器（Microsoft Basic Display / Remote Display、Hyper-V、VMware、VirtualBox、QXL 等）忽略。独显判定：NVIDIA 恒为独显；AMD 的 `Radeon Graphics` / `Vega N` / `xxxM` 是集显，其余（RX、Pro）是独显；Intel 只有 Arc 是独显。
4. 试跑：平台上每个厂商的编码器（nvidia：`h264_nvenc` / `hevc_nvenc`；intel：`h264_qsv` / `hevc_qsv`；amd：`h264_amf` / `hevc_amf`；macOS：`h264_videotoolbox` / `hevc_videotoolbox`；Linux vaapi 本版不做），**只试 ffmpeg 里存在的**；显卡枚举到了就只试有对应显卡的厂商，枚举不出来就全试（试跑才是真相）。命令：`ffmpeg -hide_banner -loglevel error -nostdin -f lavfi -i color=c=black:s=256x256:d=0.1 -frames:v 1 -c:v <enc> -f null -`，**每个编码器 5 秒超时**，同时最多 2 个试跑。某厂商任一编码器试跑成功即 `available=true`，`encoders` 只填成功的那个（h264 成功、hevc 失败则 `hevc` 为 `""`）；都失败则 `available=false`、`reason` 是归类后的一行原因（转换组件不含该编码器 / 无可用显卡或驱动缺失 / 试跑超时 / 其他；v0.24 起文案不含 “ffmpeg”，如 `当前转换组件不包含这张显卡对应的显卡编码支持`，1.1）。
5. 组装：`devices[0]` 是 cpu；随后按厂商顺序（nvidia、amd、intel；macOS 只有 apple）每张显卡一项，同厂商多张 id 序号递增。试跑成功但没枚举到名字（lspci 缺失等）给一个只有厂商名的设备，名字是中文短名：`NVIDIA 显卡`、`Intel 显卡`、`AMD 显卡`；macOS 读不到型号时是 `系统显卡`（v0.20 起；此前是 `NVIDIA GPU` / `Apple VideoToolbox（系统硬件编码）` / Linux 只有 sysfs 时的 `Intel GPU（i915）`）。兜底名不含编码器名（NVENC / QSV / AMF / VideoToolbox）、驱动名（i915 / amdgpu / nvidia）和括号后缀，界面可以直接显示；能读到真实型号（如 `NVIDIA GeForce RTX 4060`）时保持原样。枚举到但厂商没有显卡编码支持的显卡（如 unknown）也列出，`available=false`。
6. 探测子进程一律经 `ffmpeg.NewCommand`（Windows 隐藏控制台窗口、单独进程组），不占用任务管理器的槽位，不影响正在运行的任务。应用根 ctx 取消时中断并返回 `CANCELED`。

**缓存**：按 `ffmpeg 路径 + 版本` 缓存整个结果。ffmpeg 状态每次变化（安装完成 / 手动指定 / 重新检测，即每次 `ffmpeg:status`）都使缓存失效；检测过程中发生失效，这次结果不写入缓存。`RefreshEncoderDevices()` 强制重测。有编码器试跑超时的结果**不缓存**（驱动可能只是一时没响应）。没有显卡的机器：`devices` 只有 cpu，不报错，也不试跑。

**偏好**：`"auto" | "cpu" | 设备 id`，存 settings 表键 `encoderPreference`，默认 `"auto"`；同时把设备名记在 `encoderPreferenceName`（设备之后不可用时，设置页仍能显示选的是哪张卡）。`SetEncoderPreference(id)`：`auto`、`cpu` 直接保存；其他值必须符合 `^[a-z0-9][a-z0-9_-]{0,31}$` 且在当前 `ListEncoderDevices` 里存在（存在但 `available=false` 的允许保存），否则 `INVALID_ARGUMENT` 且不改动原值。`GetEncoderPreference` 永远返回保存的原值，不因设备消失而改写；设备不存在或不可用时，`ListEncoderDevices` 在列表**末尾**追加一项 `available=false` 的占位（`id` 为偏好值，`name` 为记下的名字，`reason` 说明），偏好为 `auto` / `cpu` 时不追加。

**`ResolveEncoder(pref, devices, codec) (encoderName, deviceID string, fallback bool)`**（Go 纯函数，`internal/service/system`）：`codec` 为 `h264` 或 `hevc`（接受 `h265`）。
- `auto`（或空）：从 `available` 且有该 codec 编码器的显卡里选第一张，排序为 **独显优先于集显**，同为独显时 nvidia、amd 在前，其后 intel Arc，同级保持列表顺序；没有则 cpu。auto 落到 cpu **不算回退**（`fallback=false`）。
- `cpu`：cpu，`fallback=false`。
- 设备 id：该设备存在、`available` 且有该 codec 编码器则用它；否则回退 cpu，`fallback=true`。
- 不认识的 `codec`：返回 `("", "", false)`。

**~~本版不接入~~（v0.18 起作废，已在 9.7 接入）**：v0.15 时 `ConvertService` / `EditService` / `LiveService` 的编码参数仍是软件编码；`ResolveEncoder` 只是提供给下一版接入用。**未在真机验证**：真实 NVIDIA / Intel / AMD / VideoToolbox 试跑、Windows 显卡名称枚举（PowerShell 输出格式按文档与常见样例解析，用纯函数表驱动测试覆盖）。

### 9.7 硬件编码接入（v0.18，契约按架构师口头方案起草，如有出入以架构师为准）

**没有新增接口方法、没有新增错误码。** 只是让转换、剪辑导出、直播真正使用 9.6 的 `ResolveEncoder` 结果。

**何时解析**：任务**提交时**（Retry 重新提交时同样）用 `ListEncoderDevices` 的缓存结果（没有缓存时先检测一次）+ 当前偏好 + `ResolveEncoder` 解析一次，结果写进 `Task.encoder` / `encoderDevice`（所以 `task:created`、落库、第一条 `task:status` 就带着）。直播在 `StartFilePush` / `StartScreenPush` 时解析。解析器由 `system.Manager.EncoderResolver()` 提供，注入各服务的 `Config.Encoder`（nil = 一律 CPU）。

**各功能用哪个编码器**

| 功能 | 什么时候可以用硬件 | 其余一律 CPU（`hwFallback` 为 false，不算回退） |
|---|---|---|
| 格式转换 `convert` | 真正重编码 H.264（`videoCodec=h264` → `h264_*`）或 H.265（`h265` → `hevc_*`），容器 mp4 / mov / mkv / avi / flv | `-c copy`（`encoder="copy"`，无设备）；VP9（`libvpx-vp9`）；GIF（`gif`）；纯音频转换和无视频输出（不带 `encoder`）；`targetSizeMb > 0` 的两遍编码（目前 `ValidateConvertOptions` 仍拒绝）；H.264 且输出宽或高 > 4096（NVENC / AMF / QSV 的 H.264 上限） |
| 剪辑导出 `edit_export` | mp4 / mov / mkv 导出的 H.264（`h264_*`） | webm（VP9，恒 CPU）；输出宽或高 > 4096 |
| 直播 `live_file_push` / `live_screen_push`（含带存档的 tee） | 视频恒为 H.264 重编码，用 `h264_*` | 输出宽或高 > 4096 |

音频编码、滤镜（`scale` / `pad` / `fps` / `crop` / 剪辑 filtergraph）、封装参数不变，滤镜仍在 CPU 上跑，只把编码交给显卡。像素格式：nvenc / amf / videotoolbox 用 `yuv420p`，QSV 用 `nv12`；转换 / 剪辑本来就把输出降到 8bit 4:2:0，所以 10bit 输入不存在“编码器不兼容”的情况。

**参数映射**（集中在 `internal/ffmpeg/hwenc.go` 的纯函数，表驱动测试；CPU 参数与原来逐字一致）。`eq` = “x264 等价 CRF”：H.264 直接用 CRF（转换默认 23，用户设了 `crf` 就用它，剪辑导出 20），H.265 用 `CRF - 5`（默认 28 → 23，因为 x265 的 CRF 比 x264 约高 5 才是相近画质），限制在 1~51。设置了 `videoBitrate` 时按码率而不是质量。

| 编码器 | 质量（CRF 模式） | 码率模式（`videoBitrate` > 0） |
|---|---|---|
| libx264 / libx265（CPU，现状） | `-preset medium -crf N` | `-b:v` |
| `*_nvenc` | `-preset p4 -rc vbr -cq <eq> -b:v 0` | `-preset p4 -rc vbr -b:v <b>` |
| `*_qsv` | `-preset medium -global_quality <eq>` | `-preset medium -b:v <b>` |
| `*_amf` | `-quality balanced -rc cqp -qp_i <eq> -qp_p <eq>` | `-quality balanced -rc vbr_peak -b:v <b>` |
| `*_videotoolbox` | `-q:v <108 - 2×eq>`（1~100；18→72、23→62、28→52） | `-b:v <b>` |

H.265 在 mp4 / mov 里照旧加 `-tag:v hvc1`。**直播**（H.264，码率控制）：`-b:v <k>k -maxrate <k>k -bufsize <2k>k -g <2×帧率> -bf 0` 在每个硬件编码器上都给，另加各家的低延迟项：nvenc `-preset p4 -tune ll -rc cbr`；qsv `-preset veryfast -async_depth 1`；amf `-usage lowlatency -rc cbr`；videotoolbox `-realtime 1`；CPU 仍是 `-preset veryfast -tune zerolatency`（不加 `-bf 0`，与现状一致）。这些映射是经验值，**没有在真机上校准画质**。

**回退规则（硬件编码启动失败 → 自动用 CPU 重试一次，不算任务失败）**
- 触发条件（任一）：ffmpeg 非零退出，且 stderr 命中所选厂商的硬件初始化失败特征（NVENC：`No NVENC capable devices`、`Cannot load libcuda`、`Driver does not support the required nvenc API`、`OpenEncodeSessionEx failed` 等；QSV：`Error initializing an MFX session` 等；AMF：`DLL amfrt64.dll failed to open`、`AMF failed` 等；VideoToolbox：`VTCompressionSessionCreate` 等）或通用特征（`Unknown encoder`、`Error while opening encoder`）；或**起始阶段崩溃**：还没有任何进度、输出临时文件不存在或为空，且 stderr 有一行提到该硬件编码器名和错误字样。与硬件无关的失败（输入损坏、磁盘满、推流连接被拒等）**不**触发回退，按原样失败。
- 回退动作：日志里写一行说明，`Task.encoder` / `encoderDevice` 改成 CPU 编码器（`libx264` / `libx265`，设备 `cpu`），`hwFallback=true`，`hwFallbackReason` 设为下表枚举，落库，并**补发一条 `task:status`（status 仍为 `running`）**带这四个字段；然后用 CPU 参数重新运行同一个 ffmpeg 命令（进度从头开始，进度条只增不减）。**最多重试一次**；重试也失败则任务失败，错误取 CPU 那一次的错误。
- **取消不触发回退**：ctx 已取消时一律按取消处理，不启动重试。
- **直播的回退窗口**：只在推流尚未建立（还没有第一条 `task:progress`，即 ReportGate 之前）时失败才回退；推流已建立后中途失败（包括硬件编码器中途报错）**不自动重试**，任务按原有规则失败。
- **提交时**所选设备不可用（`ResolveEncoder` 返回 `fallback=true`）：直接用 CPU，`hwFallback=true`，`hwFallbackReason="device_unavailable"`，不算错误。auto 落到 CPU、偏好 `cpu`、以及上表“一律 CPU”的场景都**不**算回退。

`hwFallbackReason` 取值（固定枚举，一行，不含路径；前端自行翻译文案。v0.19：Go 常量 `internal/ffmpeg/hwenc.go`、本行、前端 `taskTypes.ts` 三处枚举一致，`TestHWFallbackReasonEnumConsistent` 锁定；前端 `errors/encoderMessages.ts` 的回退文案按功能区分，不按原因区分）：`device_unavailable`、`nvenc_init_failed`、`qsv_init_failed`、`amf_init_failed`、`videotoolbox_failed`、`encoder_unavailable`（ffmpeg 里没有该编码器）、`encoder_start_failed`（其他打开编码器失败 / 起始崩溃）。

**事件与落库**：`Task.encoder` / `encoderDevice` / `hwFallback` / `hwFallbackReason` 与 `task:progress`、`task:status` 里的同名字段一致（`omitempty`）：`task:progress` 每条都带（前端可只在变化时取用）；`task:status` 的 `running` 事件、回退时补发的 `running` 事件、所有终态事件都带；`Get` / `List` 从库里读到的任务也有（迁移 `0004_task_encoder.sql`，旧行为空）。Retry（v0.23 起原地重试）同样重新解析编码器。`Task.params` 不变（不含编码器信息）。

**已在沙箱验证**（ffmpeg 7.1.5，有 `h264_nvenc` 编码器但没有 GPU，libcuda 缺失）：转换 / 剪辑导出 / 文件推流到 MediaMTX 的 CPU 路径端到端；选 NVIDIA 时真实 NVENC 初始化失败（`Cannot load libcuda.so.1` / `Error while opening encoder`）→ 自动 CPU 重试成功，输出 h264，任务带 `hwFallback`；假 ffmpeg 夹具覆盖：初始化失败回退成功、重试也失败、与硬件无关的失败不回退、取消不回退、直播推流前失败回退、推流中途失败不回退。
**未在真机验证**：真实 NVENC / QSV / AMF / VideoToolbox 的画质与码率（CRF → 质量映射是经验值）；各厂商 stderr 失败特征的覆盖度（尤其 QSV、AMF、VideoToolbox 的真实报错文案）；`-rc cqp` / `vbr_peak` / `-async_depth` / `-realtime` 等参数在各驱动版本上的可用性；4096 的尺寸上限只是保守取值（新显卡的 H.264 可能支持更大）；直播在硬件编码器上的延迟与 `-bf 0` 效果。
