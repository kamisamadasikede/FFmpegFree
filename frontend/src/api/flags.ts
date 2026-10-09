/**
 * 接口层开关：每个后端服务一个。false = 走 api/*.ts 里的本地模拟（定时器推进任务进度，走 services/wails.ts 的模拟事件总线）；
 * true = 调用 Wails 绑定（生成的 wailsjs/go/app/<Service>；还没生成绑定的用 api/call.ts 的 callService 按名字调）。
 * 后端服务落地并生成绑定后，逐个改成 true 即可，页面 / store 不用动。
 */
/** 直播：后端 LiveService（#31）已合入，联调打开。纯浏览器环境（无 window.go）仍走模拟，见 api/live.ts 的 liveIsReal() */
export const LIVE_BACKEND_READY: boolean = true
/** 文档：后端 DocService（#29）已合入，联调打开。纯浏览器环境（无 window.go）仍走模拟，见 api/doc.ts 的 isDocSim() */
export const DOC_BACKEND_READY: boolean = true
/**
 * 关于页：GetAppVersion / GetLicenseText（后端 #34，绑定在 wailsjs/go/main/App）。
 * true = 在 Wails 里调用真实绑定；纯浏览器开发环境（没有 window.go）始终走 api/about.ts 里的模拟。
 */
export const ABOUT_BACKEND_READY: boolean = true
/**
 * 编码设备（GPU 加速）。**已打开（true）**：后端第一个 PR（#60，SystemService.ListEncoderDevices / RefreshEncoderDevices / GetEncoderPreference /
 * GetEncoderPreferenceInfo / SetEncoderPreference）和第二个 PR（#67 / #68，契约 v0.18 / v0.19 §9.7：转换 / 剪辑 / 直播按偏好真正使用硬件编码器，
 * 任务带 encoder / encoderDevice / hwFallback / hwFallbackReason）都已合入；设置页面板（components/encoder/EncoderDevicePanel.vue）、
 * 回退提示与任务里的设备名（EncoderFallbackNotice、api/encoderTask.ts）都已对着真实绑定 / 事件接好（#64、#69）。
 * true 且在 Wails 里：设置页显示“编码设备”一块，调用真实绑定，任务里显示回退提示和使用的设备名。
 * 纯浏览器（没有 window.go）：默认仍不显示；地址加 `?enc=` 才显示模拟层（仅开发 / 走查用，见 api/encoder.ts）；在 Wails 里 `?enc=` 无效。
 * 直播页的回退提示变体待 v2-fe-live-preview 合入后再接（见 api/README.md）。
 */
export const ENCODER_BACKEND_READY: boolean = true
/**
 * 转换页 v2（转换记录）：后端 #83 / #84 / #85（契约 v0.23–v0.23.3 §6.14）已合入 v2（1c4c913），绑定已生成（wailsjs/go/app/ConvertService、TaskService），联调打开。
 * true 且在 Wails 里：转换页的记录列表、源文件、删除、预览地址、缩略图、任务中心“隐藏已结束 / 显示已隐藏”都调真实绑定（api/convertRecordsBinding.ts）。
 * 纯浏览器（没有 window.go，previewMode / 截图 / 走查）仍走 api/convertRecordsMock.ts 的模拟，见 api/convertRecords.ts 的 convertV2IsReal()。
 */
export const CONVERT_V2_BACKEND_READY: boolean = true
/**
 * 转换页 v0.24（契约 v0.24.1 §6.15–6.17：存储目录与源文件副本、格式目录与搜索、原地重转、时长偏短）。
 * 后端 #95 已合入 v2（04c45d4），绑定已重新生成，联调打开（true）：在 Wails 里调真实绑定（api/convertRecordsBinding.ts）。
 * 改回 false 且在 Wails 里：转换页、设置页回到 v0.23 的样子（预设卡片、旧的“保存到”、没有“存储”页、没有重转入口），不调 v0.24 新增的接口。
 * 纯浏览器（没有 window.go，走查 / 截图）：v0.24 的界面全部打开，数据来自模拟，见 api/convertRecords.ts 的 convertV24On()。
 */
export const CONVERT_V24_BACKEND_READY: boolean = true
/**
 * 直播预览 v0.25（包 21，契约 6.10.3；后端 #109 已实现，绑定已重新生成）：GetPreviewStream → { url, mime, hasVideo, hasAudio }，
 * PullSession.previewUrl，事件 live:pull。播放器用 mpegts.js，旧的 2 fps 图片预览已删除。
 * true（联调打开）：在 Wails 里调真实绑定。纯浏览器（没有 window.go）仍走模拟：GetPreviewStream 返回 UNSUPPORTED reason=preview_unavailable，
 * 拉流页对 http(s) / ws(s) 直接播放用户填的地址。改回 false：Wails 里也按模拟处理（推流预览显示“暂时无法预览”）。
 */
export const LIVE_PREVIEW_V25_BACKEND_READY: boolean = true
