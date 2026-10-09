// @docx-editor.dev/i18n 的语言子路径只在 exports 里声明；项目的 moduleResolution=node 认不出，这里补类型（Vite 按 exports 正常解析）
declare module '@docx-editor.dev/i18n/zh-CN' {
  const zhCN: Record<string, unknown>
  export default zhCN
}
