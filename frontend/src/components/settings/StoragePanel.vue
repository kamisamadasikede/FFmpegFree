<template>
  <section :id="id" class="panel group sto" :aria-labelledby="headingId">
    <div class="phead"><h2 :id="headingId" tabindex="-1">存储</h2></div>
    <div v-for="r in rows" :key="r.kind" class="srow sd">
      <div class="l">
        <b :id="`sto-${r.kind}-l`">{{ r.label }}</b>
        <small :id="`sto-${r.kind}-d`">{{ r.desc }}</small>
      </div>
      <div class="sdc">
        <div
          class="sdp"
          :class="{ bad: !!err[r.kind] }"
          role="group"
          :aria-labelledby="`sto-${r.kind}-l`"
          :aria-describedby="`sto-${r.kind}-d`"
          :aria-invalid="!!err[r.kind] || undefined"
          :title="r.path || undefined"
        >
          <span class="h">{{ r.parts.head }}</span><span class="t">{{ r.parts.tail }}</span>
        </div>
        <div v-if="err[r.kind]" class="ferr" role="alert"><FIcon name="warn" :size="14" />{{ err[r.kind] }}</div>
        <div v-else-if="r.fellBack" class="fwarn"><FIcon name="warn" :size="14" />{{ FALLBACK_SETTINGS_NOTE }}</div>
        <div class="sdb">
          <button type="button" class="btn" :disabled="!dirs" @click="open(r.kind)"><FIcon name="folder" :size="15" />打开文件夹</button>
          <button type="button" class="btn" :disabled="busy || !dirs" @click="change(r.kind)">更改…</button>
          <button v-if="r.custom" type="button" class="btn text" :disabled="busy" @click="save(r.kind, '')">恢复默认</button>
        </div>
      </div>
    </div>
    <div class="sdnote"><FIcon name="info" :size="14" /><span>{{ STORAGE_FOOT_NOTE }}</span></div>
  </section>
</template>

<script setup lang="ts">
// 设置页“存储”（v0.24，设计说明 §八 第 45 条、§13.3 第 9 条、截图 26）：转换结果 / 上传文件两个目录。
// 每行：路径框（中间省略、保留最后两段，悬停全路径）+ 打开文件夹 · 更改… ·（自定义时）恢复默认；不能写入时路径框变红，下面显示后端 message。
// SetStorageDirs 两个字段都要传：没改的那个传当前值（自定义的传路径，默认的传 ""）；"" = 恢复默认。路径校验全交给后端（v0.24.1 §6.12）。
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { pickDirectory } from '@/api/system'
import { getStorageDirs, openStorageFolder, setStorageDirs, type StorageDirs } from '@/api/convertRecords'
import { hasWailsBackend } from '@/services/wails'
import { FALLBACK_SETTINGS_NOTE, STORAGE_CHANGED_TOAST, STORAGE_FOOT_NOTE, splitPathLastTwo } from '@/utils/convertV24Text'

withDefaults(defineProps<{ id?: string; headingId?: string }>(), { id: 'sec-storage', headingId: 'h-storage' })

type Kind = 'output' | 'uploads'
const dirs = ref<StorageDirs | null>(null)
const busy = ref(false)
const err = reactive<Record<Kind, string>>({ output: '', uploads: '' })
/** 被拒绝的路径：红框里显示它（不回退到上一个成功值） */
const rejected = reactive<Record<Kind, string>>({ output: '', uploads: '' })

const rows = computed(() => {
  const d = dirs.value
  const one = (kind: Kind, label: string, desc: string) => {
    const path = err[kind] ? rejected[kind] : (kind === 'output' ? d?.outputDir : d?.uploadsDir) ?? ''
    const custom = !!d && (kind === 'output' ? d.outputCustom : d.uploadsCustom)
    return { kind, label, desc, path, parts: splitPathLastTwo(path), custom, fellBack: !!d?.fellBack && !custom }
  }
  return [
    one('output', '转换结果', '格式转换和 Office 转 PDF 的结果都保存到这里，转换页“保存到”显示的就是这个位置。'),
    one('uploads', '上传文件', '添加文件时复制一份到这里，转换和预览都读取复制件。'),
  ]
})

onMounted(async () => {
  try {
    dirs.value = await getStorageDirs()
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
})

async function open(kind: Kind) {
  try {
    await openStorageFolder(kind)
  } catch (e) {
    ElMessage.error(toAppError(e).message)
  }
}

async function change(kind: Kind) {
  if (busy.value) return
  const title = kind === 'output' ? '选择转换结果的保存位置' : '选择上传文件的保存位置'
  const dir = hasWailsBackend() ? await pickDirectory(title).catch((e) => (ElMessage.error(toAppError(e).message), '')) : kind === 'output' ? 'D:\\Videos\\FFmpegFree' : 'E:\\FFmpegFree 素材\\uploads'
  if (dir) await save(kind, dir)
}

/** 保存一个目录；另一个传当前值（自定义的传路径，默认的传 ""） */
async function save(kind: Kind, dir: string) {
  const d = dirs.value
  if (!d || busy.value) return
  busy.value = true
  const keep = (k: Kind) => (k === 'output' ? (d.outputCustom ? d.outputDir : '') : d.uploadsCustom ? d.uploadsDir : '')
  try {
    dirs.value = await setStorageDirs({ outputDir: kind === 'output' ? dir : keep('output'), uploadsDir: kind === 'uploads' ? dir : keep('uploads') })
    err[kind] = ''
    rejected[kind] = ''
    ElMessage({ message: STORAGE_CHANGED_TOAST, type: 'info' })
  } catch (e) {
    const x = toAppError(e)
    if (x.code === 'INVALID_ARGUMENT' && dir) {
      err[kind] = x.message // 后端 message：保存位置必须是绝对路径 / 不存在 / 不是文件夹 / 无法写入（上传位置同理）
      rejected[kind] = dir
    } else ElMessage.error(x.message)
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.sd {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-4);
}
.sd .l {
  flex: 1;
  min-width: 0;
  padding-top: 4px;
}
.sd .l b {
  font-size: 13px;
  font-weight: 500;
  display: block;
}
.sd .l small {
  font-size: 12px;
  color: var(--ff-text-2);
  display: block;
  line-height: 1.5;
  margin-top: 2px;
}
.sdc {
  width: 380px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
:global(html.w1024) .sdc,
:global(.w1024) .sdc {
  width: 300px;
}
@media (max-width: 1100px) {
  .sdc {
    width: 300px;
  }
}
.sdp {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 10px;
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  font-family: var(--ff-font-mono);
  font-size: 12px;
  color: var(--ff-text-1);
  overflow: hidden;
  white-space: nowrap;
}
.sdp .h {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sdp .t {
  flex: none;
}
.sdp.bad {
  border-color: var(--ff-danger);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ff-danger) 14%, transparent);
}
.ferr,
.fwarn {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  margin-top: -4px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ff-danger-text);
}
.fwarn {
  color: var(--ff-warning-text);
}
.ferr svg,
.fwarn svg {
  margin-top: 2px;
}
.sdb {
  display: flex;
  gap: 8px;
}
.sdnote {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  padding: 10px 16px;
  font-size: 12px;
  line-height: 1.5;
  color: var(--ff-text-2);
}
.sdnote svg {
  margin-top: 2px;
  color: var(--ff-text-3);
}
.btn {
  height: 28px;
  padding: 0 12px;
  border-radius: var(--ff-radius-md);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  color: var(--ff-text-1);
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  background: var(--ff-bg-hover);
}
.btn:disabled {
  opacity: 0.45;
  cursor: default;
}
.btn.text {
  border-color: transparent;
  background: transparent;
  color: var(--ff-primary-text);
  padding: 0 8px;
}
</style>
