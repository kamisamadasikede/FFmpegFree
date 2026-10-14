/**
 * vue-i18n 入口（契约 §6.20.2）。
 * 组件里用 useI18n().t；store / api / utils 用这里的 t()。
 * Node 自检（check:api）里若 Composer 尚未就绪，回退到 zh-CN.json 查表，保证文案自检仍对中文断言。
 */
import { createI18n } from 'vue-i18n'
import zhCN from '@/locales/zh-CN.json'
import en from '@/locales/en.json'
import ja from '@/locales/ja.json'

export type AppLocale = 'zh-CN' | 'en' | 'ja'
export const LOCALES: AppLocale[] = ['zh-CN', 'en', 'ja']
export const FALLBACK_LOCALE: AppLocale = 'zh-CN'

export const i18n = createI18n({
  legacy: false,
  globalInjection: true,
  locale: FALLBACK_LOCALE,
  fallbackLocale: FALLBACK_LOCALE,
  missingWarn: false,
  fallbackWarn: false,
  messages: {
    'zh-CN': zhCN,
    en,
    ja,
  },
})

function lookup(messages: Record<string, unknown>, key: string): unknown {
  return key.split('.').reduce<unknown>((o, k) => {
    if (o && typeof o === 'object' && k in (o as object)) return (o as Record<string, unknown>)[k]
    return undefined
  }, messages)
}

function interpolate(template: string, params?: Record<string, unknown>): string {
  if (!params) return template
  return template.replace(/\{(\w+)\}/g, (_, name: string) =>
    params[name] != null ? String(params[name]) : `{${name}}`,
  )
}

/** 非组件代码用的翻译函数（api / store / utils） */
export function t(key: string, params?: Record<string, unknown>): string {
  try {
    const msg = i18n.global.t(key, params as any)
    const s = typeof msg === 'string' ? msg : String(msg)
    if (s && s !== key) return s
  } catch {
    /* Node 自检等环境 Composer 未就绪时走下面兜底 */
  }
  const locale = (i18n.global.locale as { value?: string }).value ?? FALLBACK_LOCALE
  const bag =
    locale === 'en' ? en : locale === 'ja' ? ja : zhCN
  const raw = lookup(bag as Record<string, unknown>, key)
  if (typeof raw === 'string') return interpolate(raw, params)
  const fb = lookup(zhCN as Record<string, unknown>, key)
  if (typeof fb === 'string') return interpolate(fb, params)
  return key
}

export function setI18nLocale(locale: AppLocale): void {
  i18n.global.locale.value = locale
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale === 'zh-CN' ? 'zh-CN' : locale
  }
}

export function currentLocale(): AppLocale {
  const v = i18n.global.locale.value
  return LOCALES.includes(v as AppLocale) ? (v as AppLocale) : FALLBACK_LOCALE
}
