import {createApp} from 'vue'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import './styles/tokens.css'
import './styles/element-override.css'
import './styles/base.css'
import './styles/settings-panels.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import { createPinia } from 'pinia'
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

// pinia 先于 router 安装：路由守卫里要读 ffmpeg store，首次导航在 app.use(router) 之后就会开始
app.use(pinia)
app.use(router)
// 桌面化：控件默认 small（设计规范第 0 节）
app.use(ElementPlus, { size: 'small', locale: zhCn }) // 全站中文（分页“共 44 条”、组件自带提示等）
app.mount('#app')
app.config.errorHandler = (err, vm, info) => {
    console.error("Vue error:", err, info);
    // 可以弹窗提示用户或记录日志
};
