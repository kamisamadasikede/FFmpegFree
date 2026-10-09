// vite.config.js
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import { fileURLToPath } from 'url'
const __dirname = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src')
    }
  },
  // docx 编辑器（@docx-editor.dev/core，只在进入 docx 编辑时按需加载）里有顶层 await：
  // 只放行这一项语法，其余仍按默认目标转译；不支持顶层 await 的旧 WebView 进不了 docx 编辑，弹窗里显示「出了点问题，请重试。」
  esbuild: {
    supported: { 'top-level-await': true }
  },
  optimizeDeps: {
    exclude: ['pdfjs-dist', '@docx-editor.dev/core', '@docx-editor.dev/vue'],
    esbuildOptions: {
      supported: { 'top-level-await': true }
    }
  },
  build: {
    target: 'es2022', // docx-editor 顶层 await
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('pdfjs-dist')) return 'pdfjs'
          if (id.includes('@docx-editor.dev')) return 'docx-editor'
          if (id.includes('/node_modules/xlsx/')) return 'xlsx'
          if (id.includes('docx-preview') || id.includes('/jszip/')) return 'docx-preview'
        }
      }
    }
  }
})
