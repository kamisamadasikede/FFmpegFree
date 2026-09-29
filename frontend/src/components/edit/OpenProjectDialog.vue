<template>
  <div class="ed-mask" @pointerdown.self="emit('close')">
    <div ref="box" class="ed-dlg" style="width: 480px" role="dialog" aria-modal="true" aria-labelledby="edp-t">
      <div class="ed-dh">
        <h3 id="edp-t">打开工程</h3>
        <button type="button" class="ed-x" aria-label="关闭" @click="emit('close')"><FIcon name="x" :size="16" /></button>
      </div>
      <InlineError v-if="error" code="INTERNAL" bare :description="error" />
      <div v-if="loading" class="ed-hint2">正在读取…</div>
      <div v-else-if="!list.length && !error" class="ed-hint2" data-autofocus tabindex="-1">还没有保存过的工程。</div>
      <div v-else class="ed-plist" role="list">
        <div v-for="p in list" :key="p.id" class="ed-prow" role="listitem">
          <button type="button" class="open" :data-autofocus="p === list[0] ? '' : undefined" @click="emit('open', p.id)">
            <span class="n"><b :title="p.name">{{ p.name }}</b><small>{{ formatClock(p.durationSec) }} · {{ p.clipCount }} 个片段 · {{ when(p.updatedAt) }}</small></span>
          </button>
          <button type="button" class="ed-btn sm" :aria-label="`删除工程“${p.name}”`" @click="askDelete(p)"><FIcon name="trash" :size="13" />删除</button>
        </div>
      </div>
      <div v-if="pending" class="ed-note warn" role="alert">
        <FIcon name="warn" :size="16" />
        <span>删除工程“{{ pending.name }}”？不会删除素材和已导出的文件。
          <button type="button" class="ed-lk" @click="doDelete">删除</button>
          <button type="button" class="ed-lk" style="margin-left: 12px" @click="pending = null">取消</button>
        </span>
      </div>
      <div class="ed-dfoot"><button type="button" class="ed-btn lg" @click="emit('close')">关闭</button></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import InlineError from '@/components/common/InlineError.vue'
import { deleteProject, toAppError, type EditProjectMeta } from '@/api/edit'
import { formatClock } from '@/utils/editLogic'
import { useEditor } from './editor'
import { useDialog } from './useDialog'

const emit = defineEmits<{ close: []; open: [id: string] }>()
const ed = useEditor()
const box = ref<HTMLElement | null>(null)
const list = ref<EditProjectMeta[]>([])
const loading = ref(true)
const error = ref('')
const pending = ref<EditProjectMeta | null>(null)
const when = (ms: number) => {
  const d = new Date(ms > 1e12 ? ms : ms * 1000)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}
onMounted(async () => {
  try {
    await ed.refreshProjects()
    list.value = ed.projects.value
  } catch (e) {
    error.value = toAppError(e).message
  } finally {
    loading.value = false
  }
})
function askDelete(p: EditProjectMeta) {
  pending.value = p
}
async function doDelete() {
  const p = pending.value
  if (!p) return
  try {
    await deleteProject(p.id)
    list.value = list.value.filter((x) => x.id !== p.id)
  } catch (e) {
    error.value = toAppError(e).message
  }
  pending.value = null
}
useDialog(box, () => emit('close'))
</script>
