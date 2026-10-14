/**
 * 应用语言（契约 §6.20.2 / §6.20.3）。
 * 走 GetAppLanguage / SetAppLanguage（settings key app.language，默认 system）。
 * 浏览器预览（无 Wails）仍用 localStorage 兜底。
 * 繁体中文 OS → zh-CN（产品 §6.20.8，无 zh-TW 包）。
 */
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { hasWailsBackend, onEvent } from '@/services/wails'
import { FALLBACK_LOCALE, LOCALES, setI18nLocale, type AppLocale, t } from '@/i18n'
import { GetAppLanguage, SetAppLanguage } from '../../wailsjs/go/main/App'

export type LanguageSetting = 'system' | AppLocale

export interface AppLanguage {
  lang: LanguageSetting
  system: AppLocale
  effective: AppLocale
}

const LS_KEY = 'app.language'

function isLocale(v: unknown): v is AppLocale {
  return typeof v === 'string' && (LOCALES as string[]).includes(v)
}

function isSetting(v: unknown): v is LanguageSetting {
  return v === 'system' || isLocale(v)
}

/** 浏览器 / 预览：navigator.language；zh* → zh-CN（含 zh-TW），ja* → ja，其余 → en */
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

function normalize(raw: { lang?: string; system?: string; effective?: string } | null | undefined): AppLanguage | null {
  if (!raw || !isSetting(raw.lang) || !isLocale(raw.system) || !isLocale(raw.effective)) return null
  return { lang: raw.lang, system: raw.system, effective: raw.effective }
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
    if (hasWailsBackend()) {
      try {
        const fromBe = normalize(await GetAppLanguage())
        if (fromBe) apply(fromBe)
        else {
          const lang = readLocal()
          const sys = mapOsLocale()
          apply({ lang, system: sys, effective: lang === 'system' ? sys : lang })
        }
      } catch {
        const lang = readLocal()
        const sys = mapOsLocale()
        apply({ lang, system: sys, effective: lang === 'system' ? sys : lang })
      }
    } else {
      const lang = readLocal()
      const sys = mapOsLocale()
      apply({ lang, system: sys, effective: lang === 'system' ? sys : lang })
    }
    ready.value = true
    onEvent<AppLanguage>('app:language', (payload) => {
      const next = normalize(payload)
      if (next) apply(next)
    })
  }

  async function setLanguage(lang: LanguageSetting) {
    error.value = ''
    const prev = { lang: setting.value, system: system.value, effective: effective.value }
    const sys = system.value || mapOsLocale()
    apply({ lang, system: sys, effective: lang === 'system' ? sys : lang })
    if (!hasWailsBackend()) return
    try {
      const fromBe = normalize(await SetAppLanguage({ lang }))
      if (fromBe) apply(fromBe)
    } catch (e: any) {
      apply(prev)
      error.value = e?.message ? String(e.message) : t('errors.unmapped')
      throw e
    }
  }

  return { setting, system, effective, ready, error, displayName, init, setLanguage }
})
