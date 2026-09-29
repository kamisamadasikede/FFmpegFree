<template>
  <aside class="panel recent" aria-labelledby="h-recent">
    <div class="rhead">
      <h2 id="h-recent">{{ DOC_RECENT_TITLE }}</h2>
      <span v-if="docs.recent.length" class="cnt">{{ docs.recent.length }} 个</span>
    </div>
    <div v-if="!docs.recent.length" class="rempty">
      <template v-if="docs.loading">正在读取…</template>
      <template v-else>
        <div class="ic"><FIcon name="doc" :size="24" /></div>
        <b>{{ DOC_RECENT_EMPTY_TITLE }}</b>
        <span>{{ DOC_RECENT_EMPTY_HINT }}</span>
      </template>
    </div>
    <ul v-else class="rlist" :aria-label="DOC_RECENT_TITLE">
      <li v-for="f in docs.recent" :key="f.id" class="rr" :class="{ on: docs.currentPath === f.path, gone: !f.exists }">
        <FIcon name="doc" :size="16" class="rico" />
        <div class="main" role="button" tabindex="0" :title="f.path" @click="open(f)" @keydown.enter.prevent="open(f)" @keydown.delete.prevent="docs.removeRecent(f)">
          <MiddleEllipsis class="nm" :text="f.name" />
          <span v-if="f.exists" class="mt">{{ formatBytes(f.size) }} · {{ formatRecentTime(f.openedAt) }}</span>
          <span v-else class="mt miss"><FIcon name="warn" :size="14" />{{ DOC_RECENT_MISSING }}</span>
        </div>
        <button type="button" class="rx" :aria-label="`从列表移除 ${f.name}`" :title="DOC_RECENT_REMOVE_TIP" @click="docs.removeRecent(f)"><FIcon name="x" :size="14" /></button>
      </li>
    </ul>
    <div v-if="docs.recent.length >= MAX_RECENT_LIMIT" class="note">{{ DOC_RECENT_LIMIT_NOTE }}</div>
  </aside>
</template>

<script setup lang="ts">
// 「最近打开的 PDF」（两个 Tab 共用，设计说明 2.4）：点一行 = 用 PDF 预览打开（切到 PDF 预览 Tab）；× 只删记录不删文件；exists=false 仍可点，点开走「找不到文件」失败态。
import { useRouter } from 'vue-router'
import FIcon from '@/components/icon/FIcon.vue'
import MiddleEllipsis from '@/components/docs/MiddleEllipsis.vue'
import { MAX_RECENT_LIMIT, type PDFFile } from '@/api/doc'
import {
  DOC_RECENT_EMPTY_HINT, DOC_RECENT_EMPTY_TITLE, DOC_RECENT_LIMIT_NOTE, DOC_RECENT_MISSING, DOC_RECENT_REMOVE_TIP, DOC_RECENT_TITLE,
} from '@/errors/errorMessages'
import { useDocsStore } from '@/stores/docs'
import { formatBytes } from '@/utils/format'
import { formatRecentTime } from '@/utils/docLogic'

const docs = useDocsStore()
const router = useRouter()
function open(f: PDFFile) {
  router.push({ path: '/docs/pdf', query: { path: f.path } })
}
</script>

<style scoped>
.recent {
  width: 320px;
  flex: none;
  padding: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
@media (max-width: 1199px) {
  .recent {
    width: 304px;
  }
}
.rhead {
  flex: none;
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  padding: 12px 16px;
  border-bottom: 1px solid var(--ff-border);
  min-height: 48px;
}
.rhead h2 {
  margin: 0;
  flex: 1;
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: var(--ff-text-1);
  white-space: nowrap;
}
.cnt {
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
  white-space: nowrap;
}
.rempty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--ff-space-2);
  padding: var(--ff-space-6);
  text-align: center;
  font-size: var(--ff-fs-xs);
  color: var(--ff-text-2);
}
.rempty .ic {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--ff-primary-soft);
  color: var(--ff-primary-text);
  display: grid;
  place-items: center;
  margin-bottom: var(--ff-space-2);
}
.rempty b {
  font-size: var(--ff-fs-md);
  font-weight: 600;
  color: var(--ff-text-1);
}
.rlist {
  list-style: none;
  margin: 0;
  padding: 0;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.rr {
  display: grid;
  grid-template-columns: 16px minmax(0, 1fr) 24px;
  column-gap: 8px;
  align-items: start;
  padding: 8px 8px 8px 16px;
}
.rr:hover {
  background: var(--ff-bg-hover);
}
.rr.on {
  background: var(--ff-primary-soft);
}
.rico {
  color: var(--ff-text-2);
  margin-top: 2px;
}
.rr.on .rico,
.rr.on .nm {
  color: var(--ff-primary-text);
}
.main {
  min-width: 0;
  display: flex;
  flex-direction: column;
  cursor: pointer;
  border-radius: var(--ff-radius-sm);
}
.main:focus-visible,
.rx:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
}
.nm {
  font-size: 13px;
  font-weight: 500;
  line-height: 20px;
  color: var(--ff-text-1);
}
.rr.gone .nm {
  color: var(--ff-text-2);
}
.mt {
  font-size: var(--ff-fs-xs);
  line-height: 16px;
  color: var(--ff-text-2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
  gap: 4px;
}
.mt.miss {
  color: var(--ff-warning-text);
}
.mt svg {
  flex: none;
}
.rx {
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: var(--ff-radius-md);
  background: transparent;
  color: var(--ff-text-2);
  display: grid;
  place-items: center;
  cursor: pointer;
}
.rx:hover {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
}
.note {
  flex: none;
  padding: 8px 16px 12px;
  border-top: 1px solid var(--ff-border);
  font-size: var(--ff-fs-xs);
  line-height: 16px;
  color: var(--ff-text-2);
}
</style>
