<template>
  <Teleport to="body">
    <MotionDialog>
    <div v-if="modelValue" class="mask" @mousedown.self.prevent="close">
      <div ref="dlgRef" class="dlg lic ff-panel" role="dialog" aria-modal="true" :aria-labelledby="titleId">
        <h3 :id="titleId">{{ title }}</h3>
        <div class="lic-slot">
          <div v-if="state === 'ok'" ref="bodyRef" class="lic-body" tabindex="0" role="region" aria-label="许可全文（只读，可滚动）"><pre>{{ text }}</pre></div>
          <div v-else-if="state === 'error'" class="lic-body" role="alert">
            <div class="lic-err">
              <FIcon name="warn" :size="14" />
              <span>无法读取许可文本，请稍后重试<a role="button" tabindex="0" class="retry" @click="load" @keydown.enter.prevent="load" @keydown.space.prevent="load">重试</a></span>
            </div>
          </div>
          <div v-else class="lic-body" role="status" aria-busy="true"><span v-if="showLoading" class="lic-load">正在读取…</span></div>
        </div>
        <div class="dfoot">
          <button ref="closeRef" type="button" class="btn pri lg" @click="close">关闭</button>
        </div>
      </div>
    </div>
    </MotionDialog>
  </Teleport>
</template>

<script setup lang="ts">
// 许可文本弹窗（设计说明第 8 节）。外壳沿用安装弹框的 .mask / .dlg，只改宽高和正文区。
// title 与 name 由调用方给（name 是 GetLicenseText 的白名单键，不接受用户输入），后面加字体时只需再传一组。
// 焦点：打开后落在「关闭」；Tab 在弹窗内循环；Esc / 点遮罩 / 点按钮关闭；关闭后焦点回到打开前的元素（「查看许可文本」链接）。
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import MotionDialog from '@/components/motion/MotionDialog.vue'
import { getLicenseText, type LicenseName } from '@/api/about'

const props = defineProps<{ modelValue: boolean; name: LicenseName; title: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const titleId = `lic-title-${Math.random().toString(36).slice(2, 8)}`
const state = ref<'loading' | 'ok' | 'error'>('loading')
const text = ref('')
/** 超过 150ms 才显示“正在读取…”，避免本地读取瞬间闪一下 */
const showLoading = ref(false)
const dlgRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
const closeRef = ref<HTMLButtonElement | null>(null)

let opener: HTMLElement | null = null
let loadSeq = 0
let loadingTimer: ReturnType<typeof setTimeout> | undefined

async function load() {
  const seq = ++loadSeq
  // 点“重试”时那个链接会被卸载，焦点会丢到 body：先记下焦点是否在弹窗内，重试后收回到“关闭”按钮
  const keepFocus = !!dlgRef.value?.contains(document.activeElement)
  state.value = 'loading'
  if (keepFocus) closeRef.value?.focus()
  showLoading.value = false
  clearTimeout(loadingTimer)
  loadingTimer = setTimeout(() => (showLoading.value = true), 150)
  try {
    const t = await getLicenseText(props.name)
    if (seq !== loadSeq) return
    text.value = t
    state.value = 'ok'
  } catch (e) {
    if (seq !== loadSeq) return
    console.error('读取许可文本失败', e)
    state.value = 'error'
  } finally {
    if (seq === loadSeq) clearTimeout(loadingTimer)
  }
}

function close() {
  emit('update:modelValue', false)
}

watch(
  () => props.modelValue,
  async (open) => {
    if (open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
      document.addEventListener('keydown', onKeydown, true) // 捕获阶段挂在 document：点到弹窗空白处焦点丢失时 Esc / Tab 仍然有效
      text.value = ''
      load()
      await nextTick()
      closeRef.value?.focus()
    } else {
      document.removeEventListener('keydown', onKeydown, true)
      loadSeq++
      clearTimeout(loadingTimer)
      const el = opener
      opener = null
      await nextTick()
      el?.focus()
    }
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKeydown, true)
  loadSeq++
  clearTimeout(loadingTimer)
})

function focusables(): HTMLElement[] {
  const root = dlgRef.value
  if (!root) return []
  return Array.from(root.querySelectorAll<HTMLElement>('button:not(:disabled), [tabindex]:not([tabindex="-1"]), a[href]'))
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    close()
    return
  }
  if (e.key !== 'Tab') return
  const list = focusables()
  if (!list.length) return
  const first = list[0]
  const last = list[list.length - 1]
  const active = document.activeElement
  if (!dlgRef.value?.contains(active)) {
    e.preventDefault()
    ;(e.shiftKey ? last : first).focus()
  } else if (e.shiftKey && active === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && active === last) {
    e.preventDefault()
    first.focus()
  }
}
</script>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(0, 0, 0, 0.45);
  display: grid;
  place-items: center;
}
.dlg {
  width: 680px;
  max-width: calc(100vw - 48px);
  height: min(560px, calc(100vh - 96px));
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-3);
  padding: var(--ff-space-6);
  background: var(--ff-bg-surface);
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-xl);
  box-shadow: var(--ff-shadow-dialog);
}
h3 {
  margin: 0;
  font-size: var(--ff-fs-lg);
  font-weight: 600;
  color: var(--ff-text-1);
}
.lic-slot {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}
.lic-body {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  background: var(--ff-bg-app);
  padding: var(--ff-space-3);
  user-select: text;
}
.lic-body:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.lic-body pre {
  margin: 0;
  font-family: var(--ff-font-mono);
  font-size: var(--ff-fs-sm);
  line-height: 1.6;
  color: var(--ff-text-1);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.lic-load {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.lic-err {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-2);
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-danger-text);
}
.lic-err svg {
  margin-top: 2px;
  flex: none;
}
.retry {
  margin-left: var(--ff-space-1);
  color: var(--ff-primary-text);
  cursor: pointer;
  border-radius: 2px;
}
.retry:hover {
  text-decoration: underline;
}
.retry:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.dfoot {
  display: flex;
  justify-content: flex-end;
  margin-top: var(--ff-space-1);
}
.btn {
  height: 32px;
  padding: 0 16px;
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-badge-bg);
  background: var(--ff-badge-bg);
  color: var(--ff-on-primary);
  font: inherit;
  font-size: var(--ff-fs-sm);
  cursor: pointer;
  transition: background var(--ff-dur-fast) var(--ff-ease);
}
.btn:hover {
  background: var(--ff-primary-hover);
  border-color: var(--ff-primary-hover);
}
.btn:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
@media (prefers-reduced-motion: reduce) {
  .btn {
    transition: none;
  }
}
</style>
