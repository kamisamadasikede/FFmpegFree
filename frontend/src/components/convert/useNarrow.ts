// 转换页 1024 布局（设计稿 html.w1024）：窗口宽度 ≤1100px 时为 true。页面根和弹窗外层据此加 .w1024
import { onScopeDispose, ref } from 'vue'

const QUERY = '(max-width: 1100px)'
const narrow = ref(false)
let users = 0
let mql: MediaQueryList | null = null
const onChange = () => (narrow.value = !!mql?.matches)

export function useNarrow() {
  if (typeof window !== 'undefined' && window.matchMedia) {
    if (!users++) {
      mql = window.matchMedia(QUERY)
      onChange()
      mql.addEventListener('change', onChange)
    }
    onScopeDispose(() => {
      if (!--users && mql) {
        mql.removeEventListener('change', onChange)
        mql = null
      }
    })
  }
  return narrow
}
