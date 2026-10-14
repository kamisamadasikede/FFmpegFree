/**
 * vue-i18n 入口（契约 §6.20.2）。
 * 组件里用 useI18n().t；store / api / utils 用这里的 t()。
 * BE GetAppLanguage / SetAppLanguage 未合入前走 localStorage 兜底（见 stores/language.ts）。
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
  missingWarn: import.meta.env.DEV,
  fallbackWarn: false,
  messages: {
    'zh-CN': zhCN,
    en,
    ja,
  },
})

/** 非组件代码用的翻译函数（api / store / utils） */
export function t(key: string, params?: Record<string, unknown>): string {
  // vue-i18n v10 Composer.t 返回 string | unknown；统一成 string
  return String(i18n.global.t(key, params as any))
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
