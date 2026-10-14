/**
 * Cat 本机偏好（设计《Cat-设置与定时任务-设计说明-v0.3.1》§3 通用；契约 6.19.12.7-3：只存 localStorage，不进 CatSettings）。
 * - 新对话放在：'last' = 上次用的项目（默认）/ 'none' = 不放在项目里 / 其它 = 某个项目 id
 * - 回复逐字显示：默认开；关掉后回复按收到的段落直接出现
 */
import { reactive } from 'vue'
import { CAT_LAST_NEW_PROJECT_KEY, CAT_NEW_CONV_PLACE_KEY, CAT_TYPEWRITER_KEY } from '@/api/catStorageKeys'
import { t } from '@/i18n'

function read(key: string): string | null {
  try {
    return globalThis.localStorage?.getItem(key) ?? null
  } catch {
    return null
  }
}
function write(key: string, v: string) {
  try {
    globalThis.localStorage?.setItem(key, v)
  } catch {
    /* 隐私模式写不进去：本次运行内仍生效 */
  }
}

export const catLocalPrefs = reactive({
  newConvPlace: read(CAT_NEW_CONV_PLACE_KEY) || 'last',
  typewriter: read(CAT_TYPEWRITER_KEY) !== '0',
  lastProject: read(CAT_LAST_NEW_PROJECT_KEY) || '',
})

export function setNewConvPlace(v: string) {
  catLocalPrefs.newConvPlace = v || 'last'
  write(CAT_NEW_CONV_PLACE_KEY, catLocalPrefs.newConvPlace)
}
export function setTypewriter(on: boolean) {
  catLocalPrefs.typewriter = on
  write(CAT_TYPEWRITER_KEY, on ? '1' : '0')
}
/** 新对话真正建在某个项目里之后记一下（「上次用的项目」） */
export function rememberNewConvProject(id: string) {
  catLocalPrefs.lastProject = id
  write(CAT_LAST_NEW_PROJECT_KEY, id)
}

/** 从侧栏「新对话」进入时放在哪个项目；项目不在 / 文件夹不见了 → 不放在项目里 */
export function resolveNewConvProject(projects: readonly { id: string; missing?: boolean }[]): string {
  const p = catLocalPrefs.newConvPlace
  const want = p === 'none' ? '' : p === 'last' ? catLocalPrefs.lastProject : p
  return projects.some((x) => x.id === want && !x.missing) ? want : ''
}
