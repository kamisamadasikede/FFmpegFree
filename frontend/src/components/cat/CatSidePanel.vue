<template>
  <section class="ct-l" aria-label="会话">
    <div class="ct-brand">
      <div class="lg"><FIcon name="cat" :size="16" /></div>
      <b>Cat</b>
      <button type="button" class="ct-back" title="返回原页面并展开侧栏" @click="emit('back')"><FIcon name="left" :size="14" />返回</button>
    </div>
    <button type="button" class="ct-it new" :class="{ on: catState.sel === NEW_CONV }" @click="select(NEW_CONV)">
      <FIcon name="plus" :size="16" /><span class="t">新对话</span><span class="r"><FIcon name="list" :size="16" /></span>
    </button>
    <button type="button" class="ct-it disabled" aria-disabled="true" :title="laterTip" @click="onLater">
      <FIcon name="bot" :size="16" /><span class="t">助手</span>
    </button>
    <button type="button" class="ct-it disabled" aria-disabled="true" :title="laterTip" @click="onLater">
      <FIcon name="clock" :size="16" /><span class="t">定时任务</span>
    </button>

    <div class="ct-scroll">
      <div class="ct-sec">
        <span>项目</span>
        <span class="acts">
          <span class="ib" title="通知"><FIcon name="bell" :size="14" /></span>
          <span class="ib" title="筛选"><FIcon name="filter" :size="14" /></span>
          <span class="ib" title="新建项目文件夹"><FIcon name="folder-plus" :size="14" /></span>
          <span class="ib" title="新建项目对话" role="button" tabindex="0" @click="select(NEW_CONV)" @keydown.enter="select(NEW_CONV)"><FIcon name="plus" :size="14" /></span>
        </span>
      </div>
      <div class="pjs">
        <div v-for="p in catState.projects" :key="p.id" class="pj" :class="{ open: p.open }">
          <div class="pj-row" role="button" tabindex="0" :aria-expanded="p.open" @click="toggle(p, $event)" @keydown.enter="toggle(p, $event)">
            <FIcon name="right" :size="12" class="car" />
            <FIcon :name="p.open ? 'folder-open' : 'folder'" :size="16" />
            <span class="t" :title="p.name">{{ p.name }}</span>
            <span class="pj-act">
              <span class="ib" title="更多"><FIcon name="more" :size="14" /></span>
              <span class="ib" title="在这个项目里新建对话" @click.stop="select(NEW_CONV)"><FIcon name="plus" :size="14" /></span>
            </span>
          </div>
          <div class="pj-kids">
            <div class="pj-in">
              <div
                v-for="c in p.convs"
                :key="c.id"
                class="pc"
                :class="{ on: c.id === catState.sel }"
                role="button"
                tabindex="0"
                @click="select(c.id)"
                @keydown.enter="select(c.id)"
              >
                <div class="l1">
                  <i class="st" :class="c.st" aria-hidden="true" />
                  <span class="t" :title="c.title">{{ c.title }}</span>
                  <span v-if="c.main" class="main-tag">主要</span>
                </div>
                <div class="l2"><span class="t" :title="c.sub">{{ c.sub }}</span><span class="tm">{{ c.time }}</span></div>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div class="ct-sec"><span>对话</span></div>
      <button
        v-for="c in catState.plain"
        :key="c.id"
        type="button"
        class="ct-it dc"
        :class="{ on: c.id === catState.sel }"
        @click="select(c.id)"
      >
        <FIcon name="chat" :size="16" /><span class="t" :title="c.title">{{ c.title }}</span>
      </button>
    </div>
    <div class="ct-it"><FIcon name="set" :size="16" /><span class="t">设置</span></div>
  </section>
</template>

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import { CAT_COPY } from '@/api/cat'
import type { CatProject } from '@/api/catMock'
import { catState, NEW_CONV } from '@/views/cat/catState'

const emit = defineEmits<{ back: [] }>()
const laterTip = CAT_COPY.later

function select(id: string) {
  catState.sel = id
}
function toggle(p: CatProject, e: Event) {
  if ((e.target as HTMLElement).closest('.pj-act')) return
  p.open = !p.open
}
function onLater() {
  ElMessage.info(CAT_COPY.later)
}
</script>

<style scoped>
.ct-l {
  width: 248px;
  flex: none;
  display: flex;
  flex-direction: column;
  background: var(--ff-bg-sidebar);
  border-right: 1px solid var(--ff-border);
  padding: 10px 8px 12px;
  min-height: 0;
}
.ct-brand {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 4px 12px;
  --wails-draggable: drag;
}
.ct-brand .lg {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: var(--ff-text-1);
  color: var(--ff-bg-surface);
  display: grid;
  place-items: center;
  flex: none;
}
.ct-brand b {
  font-size: 14px;
  font-weight: 600;
  flex: 1;
}
.ct-back {
  height: 28px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 8px;
  border-radius: 6px;
  font: inherit;
  font-size: 12px;
  color: var(--ff-text-2);
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  cursor: pointer;
  --wails-draggable: no-drag;
}
.ct-back:hover {
  color: var(--ff-text-1);
  background: var(--ff-bg-hover);
}
.ct-it {
  width: 100%;
  height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  border-radius: 6px;
  border: none;
  background: transparent;
  font: inherit;
  font-size: 13px;
  color: var(--ff-text-1);
  text-align: left;
  flex: none;
}
.ct-it > :deep(svg) {
  color: var(--ff-text-2);
}
.ct-it .r {
  margin-left: auto;
  color: var(--ff-text-3);
  display: flex;
}
button.ct-it {
  cursor: pointer;
}
button.ct-it:hover,
.pj-row:hover,
.pc:hover {
  background: var(--ff-bg-hover);
}
.ct-it.on {
  background: var(--ff-bg-hover);
  font-weight: 500;
}
.ct-it .t {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
}
.ct-it.new {
  border: 1px solid var(--ff-border);
  background: var(--ff-bg-surface);
  margin-bottom: 4px;
  font-weight: 500;
}
.ct-it.new.on {
  border-color: var(--ff-primary);
  color: var(--ff-primary-text);
}
.ct-sec {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--ff-text-2);
  padding: 14px 8px 4px;
}
.ct-sec .acts {
  display: flex;
  gap: 2px;
}
.ct-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  margin: 0 -8px;
  padding: 0 8px;
  scrollbar-width: thin;
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
  color: var(--ff-text-1);
}
.pj-row {
  height: 32px;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 6px 0 4px;
  border-radius: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--ff-text-1);
  cursor: pointer;
}
.pj-row > :deep(svg) {
  color: var(--ff-text-2);
}
.pj-row > :deep(svg.car) {
  color: var(--ff-text-3);
  transition: transform 180ms ease;
}
.pj.open .pj-row > :deep(svg.car) {
  transform: rotate(90deg);
}
.pj-row .t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pj-act {
  display: flex;
  gap: 2px;
  opacity: 0;
  transition: opacity var(--ff-dur-quick);
}
.pj-row:hover .pj-act,
.pj-row:focus-within .pj-act,
.pj-row:focus-visible .pj-act {
  opacity: 1;
}
.pj-kids {
  display: grid;
  grid-template-rows: 0fr;
  transition: grid-template-rows 200ms ease;
}
.pj.open .pj-kids {
  grid-template-rows: 1fr;
}
.pj-in {
  overflow: hidden;
  min-height: 0;
  padding-left: 14px;
}
.pc {
  padding: 5px 8px 6px;
  border-radius: 8px;
  margin: 2px 0;
  border: 1px solid transparent;
  cursor: pointer;
}
.pc.on {
  background: var(--ff-bg-surface);
  border-color: var(--ff-border);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
}
html.dark .pc.on {
  background: var(--ff-bg-elevated);
}
.pc .l1 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--ff-text-1);
  font-weight: 500;
  height: 20px;
}
.pc .l2 {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  color: var(--ff-text-3);
  padding-left: 14px;
  height: 18px;
}
.pc .t {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.pc .l1 .t {
  flex: 0 1 auto;
}
.pc .tm {
  flex: none;
  font-variant-numeric: tabular-nums;
}
.st {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
  background: var(--ff-text-3);
}
.st.ok {
  background: var(--ff-success);
}
.st.idle {
  background: transparent;
  border: 1.5px solid var(--ff-text-3);
  width: 7px;
  height: 7px;
}
.st.run {
  background: transparent;
  border: 2px solid var(--ff-warning);
  border-right-color: transparent;
  animation: ct-spin 1s linear infinite;
}
@keyframes ct-spin {
  to { transform: rotate(360deg); }
}
.main-tag {
  flex: none;
  font-size: 10.5px;
  line-height: 16px;
  padding: 0 5px;
  border-radius: 4px;
  border: 1px solid var(--ff-border);
  color: var(--ff-text-2);
  background: var(--ff-bg-hover);
  font-weight: 500;
}
button.ct-it:focus-visible,
.pj-row:focus-visible,
.pc:focus-visible,
.ct-back:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: -2px;
}
.ct-it.disabled,
.ct-it.disabled:hover {
  color: var(--ff-text-3);
  background: transparent;
  cursor: not-allowed;
  opacity: 0.55;
}
.ct-it.disabled > :deep(svg) {
  color: var(--ff-text-3);
}
</style>
