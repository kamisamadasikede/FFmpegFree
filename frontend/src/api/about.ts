import { GetAppVersion, GetLicenseText } from '../../wailsjs/go/main/App'
import { call } from '@/api/call'
import { ABOUT_BACKEND_READY } from '@/api/flags'
import { hasWailsBackend } from '@/services/wails'

/** 后端没有注入版本号时的显示文本（与后端 DevVersion 一致），也是纯浏览器预览的模拟值 */
export const DEV_VERSION = '开发版'

/** GetLicenseText 的白名单键；其它值后端返回 INVALID_ARGUMENT，所以只允许调用方传这两个常量值 */
export type LicenseName = 'OFL' | 'OFL-Nunito'

/** 只有 Wails 里且开关打开才走真实绑定 */
const live = () => ABOUT_BACKEND_READY && hasWailsBackend()

/** 应用版本号；构建时 -ldflags 注入，没有注入时后端返回“开发版” */
export async function getAppVersion(): Promise<string> {
  if (!live()) return DEV_VERSION
  return await call(GetAppVersion())
}

/** 模拟文本：明确标注“演示文本”，只放 OFL 的开头几行，不冒充完整许可文本 */
const MOCK_LICENSE = `【演示文本】这不是完整的许可文本，仅用于浏览器预览。真实文本由应用内置（后端 GetLicenseText）。

-----------------------------------------------------------
SIL OPEN FONT LICENSE Version 1.1 - 26 February 2007
-----------------------------------------------------------

PREAMBLE
The goals of the Open Font License (OFL) are to stimulate worldwide
development of collaborative font projects, to support the font creation
efforts of academic and linguistic communities, and to provide a free and
open framework in which fonts may be shared and improved in partnership
with others.

……（演示文本到此为止）`

/** 读取内置许可全文，返回后端原文（不改动、不重排）。失败抛 AppError（未知名字 INVALID_ARGUMENT） */
export async function getLicenseText(name: LicenseName): Promise<string> {
  if (!live()) {
    await new Promise((r) => setTimeout(r, 120))
    return MOCK_LICENSE
  }
  return await call(GetLicenseText(name))
}
