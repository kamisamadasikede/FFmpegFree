<script setup lang="ts">
// 文档页右栏：格式按 PDF / 文档 / 表格 / 演示分组，只显示选中文件都能转的（交集来自后端格式表 6.12.13）
import { computed, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { useDocConvertStore, DOC_TILE_SUB, type DocTile } from '@/stores/docConvert'
import { useDocComponentStore } from '@/stores/docComponent'
import { DOC_NEED_COMPONENT, DOC_SIMPLE_PDF_LABEL } from '@/utils/docV26Text'

const dc = useDocConvertStore()
const comp = useDocComponentStore()
const q = ref('')
const nSel = computed(() => dc.selectedRows.length)

function match(t: DocTile): boolean {
  const k = q.value.trim().toLowerCase()
  if (!k) return true
  return [t.ext, t.displayName, DOC_TILE_SUB[t.ext] ?? ''].some((s) => s.toLowerCase().includes(k))
}
const shown = computed(() => dc.groups.map((g) => ({ ...g, items: g.items.filter(match) })).filter((g) => g.items.length))
// 设计 v0.2 场景 02 / 03：没选文件时也按格式表置灰（组件未就绪时只剩简易 PDF / HTML / MD 可用）
// 组件检测中暂时不可用的格（pending）按中性占位显示：不置灰成「不可用」、不出禁用图标和提示，检测完平滑切到结果
const disabled = (t: DocTile) => !t.available && !t.pending
const wide = (t: DocTile) => !!t.simple && t.ext === 'pdf' && !t.pending
// 「简易转换（只保留文字）」只用于 → PDF；PDF 源的 txt / md / 简易 html 也是 simple，但格式块照常显示（说明行里讲只提取文字）
const sub = (t: DocTile) => (wide(t) ? DOC_SIMPLE_PDF_LABEL : DOC_TILE_SUB[t.ext] ?? '')
const tip = (t: DocTile) => (disabled(t) ? t.disabledReason || DOC_NEED_COMPONENT : `${t.displayName}（${sub(t)}）`)
const saveName = computed(() => {
  if (nSel.value !== 1 || !dc.target) return ''
  return `${dc.selectedRows[0].src.name.replace(/\.[^.]+$/, '')}.${dc.target}`
})
</script>

<template>
  <section class="panel cv-right" aria-label="转换设置">
    <div class="cv-ph">
      <h2>转换设置</h2>
      <span class="sp" />
      <span class="cv-selh">
        <template v-if="nSel">已选 <b>{{ nSel }}</b> 个<button type="button" class="ff-link" @click="dc.clearSelection()">取消</button></template>
        <span v-else class="none">未选择文件</span>
      </span>
    </div>
    <div class="cv-rp" style="overflow: auto">
      <label class="cv-fsearch">
        <FIcon name="search" :size="14" />
        <input v-model="q" type="text" placeholder="搜索格式，如 PDF、Word、表格" aria-label="搜索格式" autocomplete="off" spellcheck="false" />
      </label>
      <div v-if="dc.whyText" class="dc-why"><FIcon name="info" /><span>{{ dc.whyText }}</span></div>
      <div v-if="dc.matrixError" class="cv-fstate" role="alert">
        <p>格式没能加载出来。</p>
        <button type="button" class="btn" @click="dc.loadMatrix()">重试</button>
      </div>
      <div v-else-if="!dc.matrix" class="cv-presets cv-fxs" aria-busy="true" aria-label="正在加载格式">
        <div class="cv-fxg"><span v-for="i in 9" :key="i" class="cv-fxsk" /></div>
      </div>
      <div v-else class="cv-presets cv-fxs" :class="{ dim: !nSel }" role="radiogroup" aria-label="输出格式" :aria-busy="dc.checking || undefined">
        <template v-for="g in shown" :key="g.key">
          <div class="cv-fgrp">{{ g.label }}<i /></div>
          <div class="cv-fxg">
            <button
              v-for="t in g.items"
              :key="t.ext"
              type="button"
              class="preset cv-fx dc-fx"
              :class="{ on: nSel > 0 && dc.target === t.ext, off: disabled(t), 'dc-pend': t.pending, 'dc-wide': wide(t) }"
              role="radio"
              :aria-checked="nSel > 0 && dc.target === t.ext"
              :aria-disabled="disabled(t) || undefined"
              :title="tip(t)"
              :data-tip="disabled(t) ? tip(t) : undefined"
              @click="dc.pickTarget(t.ext)"
            >
              <b>{{ t.ext.toUpperCase() }}<FIcon v-if="disabled(t)" name="block" :size="12" /></b>
              <small>{{ sub(t) }}</small>
            </button>
          </div>
        </template>
      </div>
      <div v-if="dc.note" class="cv-fnote" :class="{ warn: dc.note.tone === 'warn' }">
        <FIcon name="info" />
        <span>{{ dc.note.text }}<button v-if="dc.note.download" type="button" class="lk" @click="comp.install()">下载文档组件</button></span>
      </div>
      <div class="cv-save">
        <label>保存到</label>
        <div class="cv-hint">默认保存到输出文件夹。同名文件自动加序号，不会覆盖。</div>
      </div>
    </div>
    <div class="cv-foot">
      <button type="button" class="btn pri lg" :disabled="!dc.canSubmit" :aria-disabled="!dc.canSubmit || undefined" @click="dc.submit()">
        <FIcon name="convert" />{{ nSel ? `转换 ${nSel} 个文件` : '转换' }}
      </button>
      <small v-if="dc.noneUsable">选中的文件需要文档组件才能转换</small>
      <small v-else-if="saveName">将保存为“{{ saveName }}”</small>
      <small v-else-if="nSel">每次转换都会新增一条记录</small>
      <small v-else>{{ dc.rows.length ? '勾选文件后才能转换' : '先添加文件' }}</small>
    </div>
  </section>
</template>
