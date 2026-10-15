<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { t as tGlobal } from '@/i18n'
const { t } = useI18n()

/**
 * 办公画布薄预览壳（路线 A）：按 IR deck.slides[] 逐页渲染 + 导出 PPTX。
 * 不嵌入 OnlyOffice；不用 LibreOffice PDF 冒充实时画布。
 */
import { computed } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useOfficeCanvasStore } from '@/stores/officeCanvas'

const emit = defineEmits<{ close: [] }>()
const store = useOfficeCanvasStore()
const C = store.copy

const slides = computed(() => store.deck?.slides ?? [])
const streaming = computed(() => store.artifact?.status === 'streaming')
const title = computed(() => store.deck?.title || C.toolLabel)
</script>

<template>
  <aside class="canvas-panel" data-testid="cat-office-canvas-panel" :aria-label="t('cat.misc.s_311632453c')">
    <div class="canvas-hd">
      <FIcon name="slide" :size="16" />
      <b>{{ title }}</b>
      <span class="sp" />
      <button
        type="button"
        class="btn pri"
        :disabled="store.exportBusy || !slides.length"
        data-testid="cat-office-export"
        @click="store.exportPptx()"
      >
        <FIcon name="export" :size="14" />{{ C.exportBtn }}
      </button>
      <button
        v-if="store.ready"
        type="button"
        class="ibtn"
        :title="C.openFolder"
        :aria-label="C.openFolder"
        data-testid="cat-office-folder"
        @click="store.openFolder()"
      >
        <FIcon name="folder" :size="14" />
      </button>
      <button type="button" class="ibtn" :aria-label="t('cat.misc.s_b15d91274e')" data-testid="cat-office-canvas-close" @click="emit('close')">
        <FIcon name="x" :size="14" />
      </button>
    </div>

    <div class="canvas-bd">
      <div
        v-for="(s, i) in slides"
        :key="s.id || i"
        class="slide"
        :class="{ gen: streaming && i === slides.length - 1 }"
        :data-testid="'cat-office-slide-' + (i + 1)"
      >
        <div class="sh">第 {{ i + 1 }} 页</div>
        <div class="sb">
          <h4>{{ s.title || t('cat.misc.s_5ec4e11eca') }}</h4>
          <ul v-if="s.bullets?.length">
            <li v-for="(b, j) in s.bullets" :key="j">{{ b }}</li>
          </ul>
        </div>
        <span v-if="streaming && i === slides.length - 1" class="badge-gen">{{ C.generating }}</span>
      </div>
      <div v-if="streaming" class="slide ghost" data-testid="cat-office-slide-ghost">{{ C.ghost }}</div>
    </div>

    <div class="canvas-ft">
      <span class="meta">{{ t('cat.misc.s_ff655a5c10', { n: slides.length }) }}</span>
      <span class="sp" />
      <span v-if="store.exportOk" class="st ok" data-testid="cat-office-export-ok">
        <FIcon name="check" :size="14" />已导出 {{ store.exportOk }}
      </span>
      <button
        v-else
        type="button"
        class="btn pri"
        :disabled="store.exportBusy || !slides.length"
        data-testid="cat-office-export-ft"
        @click="store.exportPptx()"
      >
        {{ C.exportBtn }}
      </button>
    </div>
  </aside>
</template>

<style scoped>
.canvas-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-width: 0;
  background: var(--cat-panel);
  border-left: 1px solid var(--cat-divider);
}
.canvas-hd {
  height: 46px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  border-bottom: 1px solid var(--cat-divider);
  background: var(--cat-surface);
  flex: none;
}
.canvas-hd b {
  font-size: 13.5px;
  font-weight: 650;
}
.canvas-hd .sp {
  flex: 1;
}
.btn {
  height: 30px;
  padding: 0 11px;
  font-size: 12.5px;
  border-radius: 8px;
  border: 1px solid var(--cat-border);
  background: var(--cat-elevated, var(--cat-surface));
  color: var(--cat-text);
  display: inline-flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
}
.btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.btn.pri {
  height: 32px;
  padding: 0 14px;
  background: var(--cat-accent);
  border-color: transparent;
  color: #fff;
  box-shadow: 0 1px 2px color-mix(in srgb, var(--cat-accent) 30%, transparent);
}
.ibtn {
  width: 28px;
  height: 28px;
  border: none;
  background: transparent;
  color: var(--cat-muted);
  border-radius: 8px;
  display: grid;
  place-items: center;
  cursor: pointer;
}
.ibtn:hover {
  background: var(--cat-hover, rgba(0, 0, 0, 0.04));
  color: var(--cat-text);
}
.canvas-bd {
  flex: 1;
  overflow: auto;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  background: var(--cat-sunken, var(--cat-bg));
}
.slide {
  border: 1px solid var(--cat-border);
  border-radius: var(--r-md, 10px);
  background: var(--cat-surface);
  overflow: hidden;
  box-shadow: var(--cat-shadow-sm);
  aspect-ratio: 16 / 9;
  display: flex;
  flex-direction: column;
  position: relative;
  transition: box-shadow 0.15s, border-color 0.15s;
}
.slide:hover {
  box-shadow: var(--cat-shadow-md);
  border-color: var(--cat-border-strong, var(--cat-border));
}
.slide .sh {
  height: 24px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  font-size: 11px;
  color: var(--cat-muted);
  background: var(--cat-elevated, var(--cat-panel));
  border-bottom: 1px solid var(--cat-divider);
  font-weight: 550;
  letter-spacing: 0.2px;
}
.slide .sb {
  flex: 1;
  padding: 16px 18px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 7px;
}
.slide .sb h4 {
  margin: 0;
  font-size: 15.5px;
  font-weight: 650;
  letter-spacing: 0.2px;
}
.slide .sb ul {
  margin: 0;
  padding-left: 18px;
  font-size: 12.5px;
  color: var(--cat-text-2);
  line-height: 1.55;
}
.slide.gen .sb {
  opacity: 0.72;
}
.badge-gen {
  position: absolute;
  top: 30px;
  right: 10px;
  font-size: 10.5px;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--cat-active);
  color: var(--cat-active-text);
  font-weight: 600;
  border: 1px solid color-mix(in srgb, var(--cat-accent) 30%, transparent);
}
.slide.ghost {
  border: 1px solid var(--cat-divider);
  background: var(--cat-surface);
  min-height: 72px;
  aspect-ratio: auto;
  display: grid;
  place-items: center;
  color: var(--cat-muted);
  font-size: 12.5px;
  box-shadow: none;
}
.canvas-ft {
  padding: 12px;
  border-top: 1px solid var(--cat-divider);
  display: flex;
  gap: 8px;
  align-items: center;
  background: var(--cat-surface);
  flex: none;
}
.canvas-ft .meta {
  font-size: 12px;
  color: var(--cat-muted);
}
.canvas-ft .sp {
  flex: 1;
}
.st.ok {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12.5px;
  color: var(--cat-success, #0a0);
}
</style>
