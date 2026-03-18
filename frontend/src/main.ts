import {createApp} from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './style.css';
import api from './api';
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

app.use(router)
app.use(pinia)
app.use(ElementPlus)
app.mount('#app')
// 可以将 api 挂载到全局，方便在组件中使用
app.config.globalProperties.$api = api;
app.config.errorHandler = (err, vm, info) => {
    console.error("Vue error:", err, info);
    // 可以弹窗提示用户或记录日志
};
