
import { t } from '@/i18n'/** 输入框一次最多带这么多个文件。前四个直接摆在输入框上，剩下的收进加号旁边。 */
export const CAT_ATTACH_SHOW = 4
export const CAT_ATTACH_MAX = 20

export interface CatOutgoingFile {
  name: string
  path: string
}

export function splitAttach<T>(files: readonly T[]): { shown: T[]; extra: T[] } {
  return { shown: files.slice(0, CAT_ATTACH_SHOW), extra: files.slice(CAT_ATTACH_SHOW) }
}

export function fileBaseName(path: string): string {
  const s = path.replace(/\\/g, '/')
  const i = s.lastIndexOf('/')
  const name = (i >= 0 ? s.slice(i + 1) : s).trim()
  return name || path
}
