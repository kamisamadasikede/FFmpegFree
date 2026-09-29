import { computed, ref, watch } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'ff-theme'
// TODO(v2): 接上 SettingsService 后改为读写 Settings.theme，localStorage 只作首屏兜底
// 浏览器预览（没有 window.go）时地址里的 ?theme=dark|light 优先，方便截图；真实运行不读取
const previewTheme = !(window as any).go ? new URLSearchParams(window.location.search).get('theme') : null
const mode = ref<ThemeMode>(
  previewTheme === 'dark' || previewTheme === 'light' ? previewTheme : (localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'system',
)
const media = window.matchMedia('(prefers-color-scheme: dark)')
const systemDark = ref(media.matches)
const isDark = computed(() => mode.value === 'dark' || (mode.value === 'system' && systemDark.value))

function apply() {
  systemDark.value = media.matches
  document.documentElement.classList.toggle('dark', isDark.value)
}

let started = false
function start() {
  if (started) return
  started = true
  apply()
  media.addEventListener('change', apply)
  watch(mode, (m) => {
    if (!previewTheme) localStorage.setItem(STORAGE_KEY, m)
    apply()
  })
}

export function useTheme() {
  start()
  return { mode, isDark }
}
