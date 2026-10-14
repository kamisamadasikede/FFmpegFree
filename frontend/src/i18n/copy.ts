/**
 * 把文案表接到 i18n：取值时走 t()，热切换语言无需重建对象。
 * map 的 value 是 locale key 的最后一段（相对 prefix），或带插值的工厂。
 */
import { t } from '@/i18n'

type CopyValue = string | ((params?: Record<string, unknown>) => string)
type CopyInput = Record<string, CopyValue>

export function defineCopy<T extends CopyInput>(prefix: string, map: T): {
  readonly [K in keyof T]: T[K] extends (...args: any) => any ? T[K] : string
} {
  const out: Record<string, unknown> = {}
  for (const key of Object.keys(map)) {
    const spec = map[key]
    if (typeof spec === 'function') {
      out[key] = (...args: unknown[]) => {
        // 工厂自己决定怎么调 t；仍挂在 defineCopy 下方便统一前缀文档
        return (spec as Function)(...args)
      }
    } else {
      const full = `${prefix}.${key}`
      Object.defineProperty(out, key, {
        enumerable: true,
        configurable: false,
        get: () => t(full),
      })
    }
  }
  return out as any
}

/** 带插值的短助手：t(`${prefix}.${key}`, params) */
export function tp(prefix: string, key: string, params?: Record<string, unknown>): string {
  return t(`${prefix}.${key}`, params)
}
