<template>
  <div v-if="ed.dialog.value" class="ed-mask" @pointerdown.self="cancel">
    <div ref="box" class="ed-dlg sm" role="alertdialog" aria-modal="true" :aria-labelledby="'edd-t'" :aria-describedby="'edd-d'">
      <div class="ed-dh"><h3 id="edd-t">{{ title }}</h3></div>
      <p id="edd-d">{{ body }}</p>
      <div class="ed-dfoot">
        <template v-if="d.kind === 'unsaved'">
          <button type="button" class="ed-btn lg" @click="discard">不保存</button>
          <button type="button" class="ed-btn lg" @click="cancel">取消</button>
          <button type="button" class="ed-btn pri lg" data-autofocus @click="saveThen">保存</button>
        </template>
        <template v-else>
          <button type="button" class="ed-btn lg" data-autofocus @click="cancel">取消</button>
          <button type="button" class="ed-btn danger lg" @click="confirm">{{ d.kind === 'removeSource' ? '移除并删除片段' : '删除' }}</button>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useEditor } from './editor'
import { useDialog } from './useDialog'

const ed = useEditor()
const box = ref<HTMLElement | null>(null)
const d = computed(() => ed.dialog.value!)
const title = computed(() => (d.value.kind === 'unsaved' ? '工程有未保存的更改，要保存吗？' : d.value.kind === 'removeSource' ? '移除这个素材？' : `删除所选的 ${(d.value as { ids: string[] }).ids.length} 个片段？`))
const body = computed(() =>
  d.value.kind === 'unsaved'
    ? '不保存会丢失这次的修改。'
    : d.value.kind === 'removeSource'
      ? `这个素材还在时间线上用到 ${(d.value as { n: number }).n} 次，移除后这些片段也会被删除。`
      : '此操作无法撤销。',
)
function cancel() {
  const v = ed.dialog.value
  ed.dialog.value = null
  if (v && v.kind === 'unsaved') v.cancel?.()
}
function confirm() {
  const v = d.value
  ed.dialog.value = null
  if (v.kind === 'deleteClips') {
    ed.removeClips(v.ids)
    ed.say(`已删除 ${v.ids.length} 个片段`)
  } else if (v.kind === 'removeSource') ed.removeSource(v.path, true)
}
function discard() {
  const v = d.value
  ed.dialog.value = null
  if (v.kind === 'unsaved') v.then()
}
async function saveThen() {
  const v = d.value
  if (v.kind !== 'unsaved') return
  if (await ed.save()) {
    ed.dialog.value = null
    v.then()
  }
}
useDialog(box, cancel)
</script>
