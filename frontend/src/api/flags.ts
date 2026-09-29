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
