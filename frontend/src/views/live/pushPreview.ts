// 浏览器预览（没有 window.go）专用的推流页状态种子，方便截图和走查设计稿 v0.2 的每一屏；真实运行完全不读这些参数。
//   ?rows=running|multi|stopping|ended|canceled|cancelarc|interrupted|max4|screenlimit|all   预置会话列表
//   ?form=empty|filled|scheme|malformed|host|param|unknown|srtpass|connfail|connfailsrt|rejected|same|max4|conflictunk|screen1|perm|unsupported|nosrt|noproto   预置表单状态
// 没有 ?form= 但有 ?rows= 时表单是“已填写”。
import { hasWailsBackend, previewParams } from '@/services/wails'

const on = !hasWailsBackend()
export const rowsPreview: string | null = on ? previewParams.get('rows') : null
export const formPreview: string | null = on ? previewParams.get('form') ?? (rowsPreview ? 'filled' : null) : null
