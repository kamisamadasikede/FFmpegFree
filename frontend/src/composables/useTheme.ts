import { computed, ref, watch } from 'vue'

export type ThemeMode = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'ff-theme'
// TODO(v2): 接上 SettingsService 后改为读写 Settings.theme，localStorage 只作首屏兜底
const mode = ref<ThemeMode>((localStorage.getItem(STORAGE_KEY) as ThemeMode) || 'system')
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
    localStorage.setItem(STORAGE_KEY, m)
    apply()
  })
}

export function useTheme() {
  start()
  return { mode, isDark }
}
