// 「关于」页的文案与链接常量。
// 待产品经理确认两项（见 PR 描述）：① 许可证那句话（稿面写法，来自仓库根 LICENSE）；② 项目地址用 GitHub 还是 gitee 镜像。
// 确认后只改这个文件。

/** 待产品经理确认：项目地址（稿面没写 URL；仓库现有 remote 是 GitHub，另有 gitee 镜像） */
export const PROJECT_URL = 'https://github.com/kamisamadasikede/FFmpegFree'

/** 待产品经理确认：许可证链接目标。仓库根 LICENSE；HEAD 指向默认分支，v2 成为默认分支后无需再改 */
export const LICENSE_URL = `${PROJECT_URL}/blob/HEAD/LICENSE`

/** 待产品经理确认：许可证说明句（稿面逐字，依据仓库根 LICENSE：木兰宽松许可证，第 2 版） */
export const LICENSE_SUMMARY = '本应用以木兰宽松许可证，第 2 版发布'

/** 第三方字体许可。name 是 GetLicenseText 的白名单键，只能传这里列出的值，不接受用户输入 */
export interface FontLicense {
  /** GetLicenseText 的参数（后端白名单） */
  name: 'OFL' | 'OFL-Nunito'
  font: string
  /** 面板里的一行说明（12px --ff-text-2），「查看许可文本」链接跟在末尾 */
  desc: string
  /** 弹窗标题 */
  dialogTitle: string
}

export const FONT_LICENSES: readonly FontLicense[] = [
  {
    name: 'OFL',
    font: 'Noto Sans SC',
    desc: 'Noto Sans SC 子集，遵循 SIL Open Font License 1.1，许可全文已内置在应用中。',
    dialogTitle: '字体许可：Noto Sans SC（SIL Open Font License 1.1）',
  },
  {
    // 后端 #36 合入前 GetLicenseText("OFL-Nunito") 会返回 INVALID_ARGUMENT，弹窗显示读取失败；文案待设计师定稿
    name: 'OFL-Nunito',
    font: 'Nunito',
    desc: 'Nunito，遵循 SIL Open Font License 1.1，许可全文已内置在应用中。',
    dialogTitle: '字体许可：Nunito（SIL Open Font License 1.1）',
  },
]
