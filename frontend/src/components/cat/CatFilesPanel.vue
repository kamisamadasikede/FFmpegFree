<template>
  <section class="ct-r" aria-label="文件">
    <div class="ct-tabs" role="tablist">
      <button type="button" role="tab" :aria-selected="tab === 'files'" :class="{ on: tab === 'files' }" @click="tab = 'files'">文件</button>
      <button type="button" role="tab" :aria-selected="tab === 'changes'" :class="{ on: tab === 'changes' }" @click="tab = 'changes'">
        变更
      </button>
      <span class="sp" />
      <span class="ib" title="打开文件夹"><FIcon name="folder" :size="15" /></span>
      <span class="ib" title="刷新"><FIcon name="refresh" :size="15" /></span>
    </div>
    <template v-if="tab === 'files'">
      <div class="ct-search"><FIcon name="search" :size="13" />按文件名搜索</div>
      <div class="ct-tree">
        <div
          v-for="(n, i) in nodes"
          :key="i"
          class="ct-node"
          :class="{ on: picked === i }"
          :style="{ paddingLeft: n[0] * 16 + (n[1] === 'f' ? 18 : 0) + 'px' }"
          @click="n[1] === 'f' && (picked = i)"
        >
          <template v-if="n[1] === 'f'"><FIcon name="file" :size="14" class="fi" /></template>
          <template v-else>
            <FIcon :name="n[1] === 'd' ? 'down' : 'right'" :size="12" class="car" /><FIcon name="folder" :size="14" />
          </template>
          <span class="nm">{{ n[2] }}</span>
        </div>
      </div>
    </template>
    <div v-else-if="changes.length" class="ct-tree chg">
      <div v-for="c in changes" :key="c.path" class="ct-node">
        <FIcon name="file" :size="14" class="fi" /><span class="nm" :title="c.path">{{ c.path }}</span>
        <span class="add">+{{ c.add }}</span><span class="del">−{{ c.del }}</span>
      </div>
    </div>
    <div v-else class="ct-empty"><FIcon name="file" :size="28" /><span>没有变更</span></div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import { mockChanges, mockFiles } from '@/api/catMock'

const props = defineProps<{ convId: string }>()
const tab = ref<'files' | 'changes'>('files')
const nodes = computed(() => mockFiles(props.convId))
const changes = computed(() => mockChanges(props.convId))
const picked = ref(-1)
watch(
  nodes,
  (list) => {
    picked.value = list.findIndex((n) => n[3])
  },
  { immediate: true },
)
</script>

<style scoped>
.ct-r {
  width: 236px;
  flex: none;
  display: flex;
  flex-direction: column;
  border-left: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  min-height: 0;
}
.ct-tabs {
  height: 44px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 0 10px;
  border-bottom: 1px solid var(--ff-border);
}
.ct-tabs button {
  font: inherit;
  font-size: 13px;
  padding: 4px 8px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--ff-text-2);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.ct-tabs button.on {
  background: var(--ff-bg-hover);
  color: var(--ff-text-1);
  font-weight: 500;
}
.ct-tabs .sp {
  flex: 1;
}
.ib {
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 5px;
  color: var(--ff-text-2);
  cursor: pointer;
}
.ib:hover {
  background: var(--ff-bg-hover);
}
.ct-search {
  margin: 10px;
  height: 30px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 10px;
  border: 1px solid var(--ff-border);
  border-radius: 6px;
  font-size: 12px;
  color: var(--ff-text-3);
}
.ct-tree {
  display: flex;
  flex-direction: column;
  padding: 0 6px;
  overflow-y: auto;
  min-height: 0;
}
.ct-tree.chg {
  padding-top: 10px;
}
.ct-node {
  height: 28px;
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ff-text-1);
  border-radius: 6px;
  white-space: nowrap;
  flex: none;
  cursor: default;
}
.ct-node > :deep(svg) {
  color: var(--ff-text-2);
}
.ct-node > :deep(svg.car) {
  color: var(--ff-text-3);
}
.ct-node > :deep(svg.fi) {
  color: #d9622b;
}
.ct-node.on {
  background: var(--ff-bg-hover);
}
.ct-node .nm {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
}
.chg .ct-node {
  padding: 0 6px;
}
.chg .nm {
  flex: 1;
}
.add {
  color: var(--ff-success-text);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.del {
  color: var(--ff-danger-text);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.ct-empty {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--ff-text-3);
  font-size: 13px;
  padding-bottom: 80px;
}
</style>
