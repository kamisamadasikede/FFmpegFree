/**
 * 接口层开关：每个后端服务一个。false = 走 api/*.ts 里的本地模拟（定时器推进任务进度，走 services/wails.ts 的模拟事件总线）；
 * true = 调用 Wails 绑定（window.go.app.<Service>.<Method>，见 api/call.ts 的 callService）。
 * 后端服务落地并生成绑定后，逐个改成 true 即可，页面 / store 不用动。
 */
/** 直播：后端 LiveService（#31）已合入，联调打开。纯浏览器环境（无 window.go）仍走模拟，见 api/live.ts 的 liveIsReal() */
export const LIVE_BACKEND_READY: boolean = true
/** 剪辑：后端 EditService（#30）已合入、契约已冻结，联调打开。纯浏览器环境（无 window.go）仍走模拟，见 api/edit.ts 的 editIsReal() */
export const EDIT_BACKEND_READY: boolean = true
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
 * 转换页 v2（转换记录）：契约 v0.23 §6.14 已合入 v2（da6c6eb），v0.23.1 的 RevealRecord / GetSource 随后端实现 PR 加；后端实现还没合入。
 * 接口名按契约写在 api/convertRecordsBinding.ts 的 V023_METHODS。**false**：转换页的记录列表、源文件、删除、预览地址、任务中心“隐藏已结束”都走
 * api/convertRecordsMock.ts 的模拟（复用 api/sim.ts 的模拟任务引擎和事件总线）；后端实现合入、绑定生成并联调后改成 true 即可，页面 / store 不用动。
 */
export const CONVERT_V2_BACKEND_READY: boolean = false
