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
 * 编码设备（GPU 加速）。后端第一个 PR（#60，SystemService.ListEncoderDevices / RefreshEncoderDevices / GetEncoderPreference /
 * GetEncoderPreferenceInfo / SetEncoderPreference）已合入，设置页面板（components/encoder/EncoderDevicePanel.vue）已对着真实绑定写好，
 * **但本开关必须保持 false**：#60 只做检测和偏好，转换 / 剪辑 / 直播的编码参数还没用上所选设备，
 * 现在打开会让用户看到“能选显卡”却没有实际加速。
 * 打开条件：后端第二个 PR（转换、剪辑、直播真正按偏好使用硬件编码器，含任务的 encoder / encoderDevice / hwFallback / hwFallbackReason 字段）合入后，
 *   1) 核对 api/encoder.ts 与生成绑定；2) 接线回退提示（EncoderFallbackNotice）与任务详情里的编码器展示；3) 再把本开关改成 true。
 * false 时（不论在不在 Wails 里）正式包设置页完全不显示“编码设备”一块；仅开发 / 走查：纯浏览器（没有 window.go）地址加 `?enc=`（见 api/encoder.ts）看模拟层。
 */
export const ENCODER_BACKEND_READY: boolean = false
