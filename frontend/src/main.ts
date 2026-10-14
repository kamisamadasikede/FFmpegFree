import {createApp, watch} from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import enEp from 'element-plus/es/locale/lang/en'
import jaEp from 'element-plus/es/locale/lang/ja'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import './styles/tokens.css'
import './styles/element-override.css'
import './styles/base.css'
import './styles/motion.css'
import './styles/settings-panels.css'
import './styles/cat-tokens.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import { createPinia } from 'pinia'
import { i18n, type AppLocale } from '@/i18n'
import { useLanguageStore } from '@/stores/language'

const app = createApp(App)
const pinia = createPinia()

// Polyfill for pdf.js in environments without URL.parse (e.g., Wails WebView)
if (typeof (URL as any).parse !== 'function') {
    ;(URL as any).parse = (input: string, base?: string) => {
        const resolved = base ? new URL(input, base) : new URL(input, window.location.href)
        return {
            href: resolved.href,
            protocol: resolved.protocol,
            slashes: true,
            auth: null,
            host: resolved.host,
            port: resolved.port,
            hostname: resolved.hostname,
            hash: resolved.hash,
            search: resolved.search,
            query: resolved.search ? resolved.search.substring(1) : '',
            pathname: resolved.pathname,
            path: resolved.pathname + resolved.search,
        }
    }
}
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
}

const epLocale = (lang: AppLocale) => (lang === 'en' ? enEp : lang === 'ja' ? jaEp : zhCn)

app.use(pinia)
app.use(i18n)
app.use(router)

async function boot() {
  const langStore = useLanguageStore()
  await langStore.init()
  app.use(ElementPlus, { size: 'small', locale: epLocale(langStore.effective) })
  watch(
    () => langStore.effective,
    () => {
      // A 期：语言热切换；EP 部分内置文案可能需重开面板才刷新
    },
  )
  app.mount('#app')
  app.config.errorHandler = (err, _vm, info) => {
    console.error('Vue error:', err, info)
  }
}

void boot()
