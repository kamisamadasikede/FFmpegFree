// 「关于」页的文案与链接常量（产品经理已确认，2026-09-29）。改文案或链接只改这个文件。

/** 项目地址：GitHub（产品经理已定，不放 gitee 镜像） */
export const PROJECT_URL = 'https://github.com/kamisamadasikede/FFmpegFree'

/** 许可证链接：仓库根 LICENSE（master 分支），用系统浏览器打开（产品经理已定） */
export const LICENSE_URL = `${PROJECT_URL}/blob/master/LICENSE`

/** 许可证说明句（产品经理已定；“许可证”后没有逗号） */
export const LICENSE_SUMMARY = '本应用以木兰宽松许可证第 2 版发布'

/** 第三方字体许可。name 是 GetLicenseText 的白名单键，只能传这里列出的值，不接受用户输入 */
export interface FontLicense {
  /** GetLicenseText 的参数（后端白名单） */
  name: 'OFL'
  /** 字体名（渲染时加粗，600 / --ff-text-1） */
  font: string
  /** 面板里一段说明（12px --ff-text-2）= before + 加粗的 font + after，「查看许可文本」链接跟在末尾（设计说明 §5、§8.5） */
  before: string
  after: string
  /** 弹窗标题 */
  dialogTitle: string
}

export const FONT_LICENSES: readonly FontLicense[] = [
  {
    name: 'OFL',
    font: 'Noto Sans SC',
    before: '文档页 Office 转 PDF 内置 ',
    after: ' 子集字体，遵循 SIL Open Font License 1.1，许可全文已内置在应用中。',
    dialogTitle: '字体许可：Noto Sans SC（SIL Open Font License 1.1）',
  },
]
