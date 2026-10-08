<template>
  <div class="row od" :class="`s-${state}`">
    <div class="l">
      <div class="row-label">默认输出位置</div>
      <div id="od-desc" class="row-desc">转换后的文件默认保存到这里。</div>
    </div>

    <div class="odc">
      <div
        class="odp"
        :class="{ bad: !!error }"
        role="group"
        aria-label="默认输出位置"
        aria-describedby="od-desc"
        :aria-invalid="!!error"
        :title="shown || undefined"
      >
        <span v-if="!shown" class="ph">{{ OUTPUT_DIR_DEFAULT_TEXT }}</span>
        <template v-else>
          <span class="h">{{ pathParts.head }}</span><span class="t">{{ pathParts.tail }}</span>
        </template>
      </div>
      <div v-if="error" id="od-err" class="ferr" role="alert">
        <FIcon name="warn" :size="14" />这个文件夹不存在或没有写入权限，请换一个。
      </div>
    </div>

    <div class="odb">
      <button v-if="!shown" type="button" class="btn" :disabled="busy" @click="choose"><FIcon name="folder" :size="15" />选择文件夹…</button>
      <template v-else>
        <button type="button" class="btn" :disabled="busy" @click="choose">更改…</button>
        <button type="button" class="btn text" :disabled="busy" @click="restore">恢复默认</button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
// 设置页「转换」分组里的「默认输出位置」（设计稿 proto/pages.html ?page=settings&outdir=empty|set|error）。
// 值就是 Settings.defaultOutputDir：空字符串 = 应用的输出文件夹（<base>/output，v0.24.1）。
// 选择用 SystemService.PickDirectory，保存用 UpdateSettings；后端对不存在 / 不可写 / 非绝对路径返回 INVALID_ARGUMENT，
// 此时红框里显示被拒绝的那个路径（shown），saved 仍是后端确认过的最后一个值；下一次成功保存后红框消失。
// 挂载时会对已保存的路径再校验一次（外接硬盘拔掉了 / 文件夹被删了 → 直接进入错误态）。
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { toAppError } from '@/api/call'
import { OUTPUT_DIR_DEFAULT_TEXT, publicErrorText} from '@/errors/errorMessages'
import { getDefaultOutputDir, pickDirectory, setDefaultOutputDir } from '@/api/system'
import { hasWailsBackend, previewParams } from '@/services/wails'

/** 已保存的值（后端确认过的） */
const saved = ref('')
/** 被拒绝的路径：保存失败 / 挂载校验失败时显示在红框里（不回退到上一个成功值） */
const rejected = ref('')
const error = ref(false)
/** 输入框里显示的路径：出错时是被拒绝的路径，否则是已保存的值 */
const shown = computed(() => (error.value ? rejected.value : saved.value))
const busy = ref(false)

// 浏览器预览（没有 window.go）：?outdir=empty|set|error 直接摆出对应状态，按钮只在本地模拟
const preview = !hasWailsBackend() ? previewParams.get('outdir') : null
const PREVIEW_DIR = '/Users/me/Movies/客户项目/2026 秋季发布会/成片输出/final'

const state = computed(() => (error.value ? 'error' : shown.value ? 'set' : 'empty'))

/** 路径中间省略：最后一级文件夹名单独一段（不被省略），前面的部分放不下时用省略号 */
const pathParts = computed(() => {
  const p = shown.value.replace(/[\\/]+$/, '')
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i < 0 ? { head: '', tail: p } : { head: p.slice(0, i + 1), tail: p.slice(i + 1) }
})

onMounted(async () => {
  if (!hasWailsBackend()) {
    if (preview === 'set') saved.value = PREVIEW_DIR
    else if (preview === 'error') {
      saved.value = '/Users/me/Movies/成片输出'
      rejected.value = saved.value
      error.value = true
    }
    return
  }
  try {
    saved.value = await getDefaultOutputDir()
  } catch (e) {
    ElMessage.error(publicErrorText(toAppError(e).message))
    return
  }
  // 已保存的路径可能已经失效（外接硬盘拔了 / 文件夹被删）：用保存时同一条校验路径（UpdateSettings）再验一次
  if (saved.value) {
    try {
      await setDefaultOutputDir(saved.value)
    } catch (e) {
      if (toAppError(e).code === 'INVALID_ARGUMENT') {
        rejected.value = saved.value
        error.value = true
      }
      // 其他错误（IO 等）不打扰：只是启动时的静默校验
    }
  }
})

/** 保存；成功后 saved 才更新。INVALID_ARGUMENT → 行内错误，其他错误 → toast */
async function save(dir: string) {
  if (!hasWailsBackend()) {
    saved.value = dir
    error.value = false
    return
  }
  try {
    await setDefaultOutputDir(dir)
    saved.value = dir // 后端保存的是清理后的路径，这里再读一次以显示真实值
    error.value = false
    rejected.value = ''
    saved.value = await getDefaultOutputDir()
  } catch (e) {
    const err = toAppError(e)
    if (err.code === 'INVALID_ARGUMENT') {
      rejected.value = dir // 红框显示被拒绝的路径；saved 仍是上一次成功的值
      error.value = true
    } else ElMessage.error(publicErrorText(err.message))
  }
}

async function choose() {
  if (busy.value) return
  busy.value = true
  try {
    const dir = hasWailsBackend() ? await pickDirectory('选择默认输出位置') : PREVIEW_DIR
    if (!dir) return // 用户取消
    await save(dir)
  } catch (e) {
    ElMessage.error(publicErrorText(toAppError(e).message))
  } finally {
    busy.value = false
  }
}

async function restore() {
  if (busy.value) return
  busy.value = true
  try {
    await save('')
  } finally {
    busy.value = false
  }
}
</script>

<style scoped>
.row {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-4);
}
.l {
  flex: 1;
  padding-top: 4px;
}
.row-label {
  font-weight: 500;
}
.row-desc {
  color: var(--ff-text-2);
  font-size: var(--ff-fs-xs);
}
.odc {
  width: 360px;
  flex: none;
  display: flex;
  flex-direction: column;
}
.odp {
  height: 28px;
  display: flex;
  align-items: center;
  padding: 0 12px;
  border: 1px solid var(--ff-border);
  border-radius: var(--ff-radius-md);
  background: var(--ff-bg-surface);
  font-size: var(--ff-fs-sm);
  color: var(--ff-text-1);
  overflow: hidden;
  white-space: nowrap;
}
.odp .ph {
  color: var(--ff-text-2); /* text-3 在浅色底上只有 2.6:1，占位文字也要 ≥4.5:1 */
}
.odp .h {
  flex: 0 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.odp .t {
  flex: none;
}
.odp.bad {
  border-color: var(--ff-danger);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--ff-danger) 14%, transparent);
}
.ferr {
  display: flex;
  align-items: flex-start;
  gap: var(--ff-space-2);
  margin-top: 4px;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-danger-text);
}
.ferr svg {
  margin-top: 1px;
}
.odb {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  flex: none;
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
  font-size: var(--ff-fs-sm);
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
