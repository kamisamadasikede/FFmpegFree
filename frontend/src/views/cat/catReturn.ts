/** Cat 页「返回」的落点：进入 /cat 前所在的页面（路由 beforeEnter 记下）。直达 #/cat 时回到转换页。 */
let returnPath = '/'

export function rememberCatReturn(path: string) {
  returnPath = path.startsWith('/cat') ? '/' : path || '/'
}

export function catReturnPath(): string {
  return returnPath
}
