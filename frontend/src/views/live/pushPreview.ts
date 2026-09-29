// 浏览器预览（没有 window.go）专用的推流页状态种子，方便截图和走查设计稿 v0.2 的每一屏；真实运行完全不读这些参数。
//   ?rows=running|multi|stopping|ended|canceled|cancelarc|interrupted|max4|screenlimit|all   预置会话列表
//   来源选择器：?sim_sources=loading|fail|empty|screens（加载中 / 失败 / 空 / 仅屏幕）；?sim_sources=stale|refreshing|nowin（刷新失败保留旧列表 / 刷新中 / Windows 没有窗口）；?sim_win=many（5 个窗口含长标题）；?sim_os=mac|linux（屏幕单选列表；linux 三块屏，需同时 sim_sources=screens）；?form=window（选中一个窗口）|srcgone|srcgonescreen|srcgoneopen（来源消失提示 / 点“刷新列表”后展开）
//   ?form=empty|filled|scheme|malformed|host|param|unknown|srtpass|connfail|connfailsrt|rejected|same|max4|conflictunk|screen1|perm|unsupported|nosrt|noproto   预置表单状态
// 没有 ?form= 但有 ?rows= 时表单是“已填写”。
import { hasWailsBackend, previewParams } from '@/services/wails'

const on = !hasWailsBackend()
export const rowsPreview: string | null = on ? previewParams.get('rows') : null
export const formPreview: string | null = on ? previewParams.get('form') ?? (rowsPreview ? 'filled' : null) : null
