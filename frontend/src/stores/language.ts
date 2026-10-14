/**
 * 应用语言（契约 §6.20.2 / §6.20.3）。
 * BE GetAppLanguage / SetAppLanguage 未合入前：localStorage 兜底，标记 I18N_BE_PENDING，合入后只改本文件。
 * 繁体中文 OS → zh-CN（产品 §6.20.8，无 zh-TW 包）。
 */
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { hasWailsBackend, onEvent } from '@/services/wails'
import { FALLBACK_LOCALE, LOCALES, setI18nLocale, type AppLocale, t } from '@/i18n'

export type LanguageSetting = 'system' | AppLocale

export interface AppLanguage {
  lang: LanguageSetting
  system: AppLocale
  effective: AppLocale
}

const LS_KEY = 'app.language'
/** 后端绑定尚未合入时为 true；合入后改为 false 并走 wailsjs */
export const I18N_BE_PENDING = true

function isLocale(v: unknown): v is AppLocale {
  return typeof v === 'string' && (LOCALES as string[]).includes(v)
}

function isSetting(v: unknown): v is LanguageSetting {
  return v === 'system' || isLocale(v)
}

/** 浏览器 / 预览：用 navigator.language；zh* → zh-CN（含 zh-TW），ja* → ja，其余 → en */
export function mapOsLocale(raw?: string | null): AppLocale {
  const s = (raw ?? (typeof navigator !== 'undefined' ? navigator.language : '') ?? '').toLowerCase()
  if (s.startsWith('zh')) return 'zh-CN'
  if (s.startsWith('ja')) return 'ja'
  return 'en'
}

function readLocal(): LanguageSetting {
  try {
    const v = localStorage.getItem(LS_KEY)
    if (isSetting(v)) return v
  } catch { /* ignore */ }
  return 'system'
}

function writeLocal(lang: LanguageSetting) {
  try { localStorage.setItem(LS_KEY, lang) } catch { /* ignore */ }
}

async function beGet(): Promise<AppLanguage | null> {
  if (I18N_BE_PENDING || !hasWailsBackend()) return null
  try {
    const go = (window as any).go?.main?.App ?? (window as any).go?.app?.App
    if (!go?.GetAppLanguage) return null
    const r = await go.GetAppLanguage()
    if (!r || !isSetting(r.lang) || !isLocale(r.system) || !isLocale(r.effective)) return null
    return { lang: r.lang, system: r.system, effective: r.effective }
  } catch {
    return null
  }
}

async function beSet(lang: LanguageSetting): Promise<AppLanguage | null> {
  if (I18N_BE_PENDING || !hasWailsBackend()) return null
  try {
    const go = (window as any).go?.main?.App ?? (window as any).go?.app?.App
    if (!go?.SetAppLanguage) return null
    const r = await go.SetAppLanguage({ lang })
    if (!r || !isSetting(r.lang) || !isLocale(r.system) || !isLocale(r.effective)) return null
    return { lang: r.lang, system: r.system, effective: r.effective }
  } catch (e) {
    throw e
  }
}

export const useLanguageStore = defineStore('language', () => {
  const setting = ref<LanguageSetting>('system')
  const system = ref<AppLocale>(mapOsLocale())
  const effective = ref<AppLocale>(FALLBACK_LOCALE)
  const ready = ref(false)
  const error = ref('')

  const displayName = computed(() => {
    const map: Record<AppLocale, string> = {
      'zh-CN': t('settings.language.zhCN'),
      en: t('settings.language.en'),
      ja: t('settings.language.ja'),
    }
    return map[effective.value]
  })

  function apply(next: AppLanguage) {
    setting.value = next.lang
    system.value = next.system
    effective.value = next.effective
    setI18nLocale(next.effective)
    writeLocal(next.lang)
  }

  async function init() {
    const fromBe = await beGet()
    if (fromBe) {
      apply(fromBe)
    } else {
      const lang = readLocal()
      const sys = mapOsLocale()
      const eff = lang === 'system' ? sys : lang
      apply({ lang, system: sys, effective: eff })
    }
    ready.value = true
    onEvent<AppLanguage>('app:language', (payload) => {
      if (!payload || !isSetting(payload.lang) || !isLocale(payload.effective)) return
      apply({
        lang: payload.lang,
        system: isLocale(payload.system) ? payload.system : mapOsLocale(),
        effective: payload.effective,
      })
    })
  }

  async function setLanguage(lang: LanguageSetting) {
    error.value = ''
    const prev = { lang: setting.value, system: system.value, effective: effective.value }
    const sys = system.value || mapOsLocale()
    const optimistic: AppLanguage = {
      lang,
      system: sys,
      effective: lang === 'system' ? sys : lang,
    }
    apply(optimistic)
    try {
      const fromBe = await beSet(lang)
      if (fromBe) apply(fromBe)
    } catch (e: any) {
      apply(prev)
      error.value = e?.message ? String(e.message) : t('errors.unmapped')
      throw e
    }
  }

  return { setting, system, effective, ready, error, displayName, init, setLanguage }
})
