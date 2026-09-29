<template>
  <div class="about spanels">
    <!-- 关于 -->
    <section class="panel group" aria-labelledby="h-about">
      <div class="phead"><h2 id="h-about">关于</h2></div>
      <div class="idrow">
        <div class="logo" aria-hidden="true"><FIcon name="play" :size="20" :stroke="2.2" /></div>
        <b>FFmpegFree</b>
      </div>
      <div class="srow">
        <div class="l"><b>版本</b></div>
        <div class="val">{{ version }}</div>
      </div>
      <div class="srow">
        <div class="l">
          <b>项目与许可证</b>
          <small>{{ LICENSE_SUMMARY }}</small>
        </div>
        <div class="links">
          <a class="lnk" :href="PROJECT_URL" @click.prevent="openExternal(PROJECT_URL)"><FIcon name="link" :size="14" />项目地址</a>
          <a class="lnk" :href="LICENSE_URL" @click.prevent="openExternal(LICENSE_URL)"><FIcon name="link" :size="14" />许可证</a>
        </div>
      </div>
    </section>

    <!-- ffmpeg：只读，与设置页共用 FFmpegPanel（不传操作插槽） -->
    <FFmpegPanel heading-id="h-ffmpeg" readonly />

    <!-- 第三方许可 -->
    <section class="panel group" aria-labelledby="h-third">
      <div class="phead"><h2 id="h-third">第三方许可</h2></div>
      <div class="srow lic">
        <div class="l">
          <b>字体来源和许可</b>
          <!-- 每条一个 <p>，链接跟在末尾；允许在窄窗口下折成两行，不截断、不 nowrap -->
          <p v-for="lic in FONT_LICENSES" :key="lic.name">{{ lic.before }}<b class="fn">{{ lic.font }}</b>{{ lic.after }}<a class="lnk sm" role="button" tabindex="0" :aria-label="`查看许可文本：${lic.font}`" @click.prevent="openLicense(lic)" @keydown.enter.prevent="openLicense(lic)" @keydown.space.prevent="openLicense(lic)">查看许可文本</a></p>
        </div>
      </div>
    </section>

    <LicenseDialog v-if="current" v-model="dialogOpen" :name="current.name" :title="current.dialogTitle" />
  </div>
</template>

<script setup lang="ts">
// 「关于」页（设计说明 v0.1）：三个面板，复用设置页的 .group / .srow 样式（styles/settings-panels.css，根节点 .spanels）。
import { onMounted, ref } from 'vue'
import FIcon from '@/components/icon/FIcon.vue'
import FFmpegPanel from '@/components/settings/FFmpegPanel.vue'
import LicenseDialog from '@/components/settings/LicenseDialog.vue'
import { getAppVersion, DEV_VERSION } from '@/api/about'
import { FONT_LICENSES, LICENSE_SUMMARY, LICENSE_URL, PROJECT_URL, type FontLicense } from '@/config/about'
import { openExternal } from '@/utils/openExternal'

const version = ref(DEV_VERSION)
onMounted(async () => {
  try {
    version.value = (await getAppVersion()).trim() || DEV_VERSION
  } catch (e) {
    console.error('读取版本号失败', e)
    version.value = DEV_VERSION
  }
})

const current = ref<FontLicense | null>(null)
const dialogOpen = ref(false)
function openLicense(lic: FontLicense) {
  current.value = lic
  dialogOpen.value = true
}
</script>

<style scoped>
.about {
  display: flex;
  flex-direction: column;
  gap: var(--ff-space-4);
}
/* 关于页的说明是正式信息，用 --ff-text-2（--ff-text-3 只留给占位和禁用） */
.idrow {
  display: flex;
  align-items: center;
  gap: var(--ff-space-3);
  padding: var(--ff-space-4);
  border-bottom: 1px solid var(--ff-border);
}
.logo {
  width: 40px;
  height: 40px;
  flex: none;
  border-radius: 10px;
  background: linear-gradient(135deg, #3b6ef5, #8b5cf6); /* 与侧栏 logo 同渐变 */
  color: #fff;
  display: grid;
  place-items: center;
}
.idrow b {
  font-size: var(--ff-fs-lg);
  font-weight: 600;
}
.val {
  flex: none;
  font-weight: 500;
  color: var(--ff-text-1);
}
.links {
  display: flex;
  align-items: center;
  gap: var(--ff-space-4);
  flex: none;
}
.lnk {
  display: inline-flex;
  align-items: center;
  gap: var(--ff-space-1);
  color: var(--ff-primary-text);
  font-size: var(--ff-fs-sm);
  cursor: pointer;
  text-decoration: none;
}
.lnk:hover {
  text-decoration: underline;
}
.lnk:focus-visible {
  outline: 2px solid var(--ff-primary);
  outline-offset: 2px;
  border-radius: 2px;
}
.lnk.sm {
  font-size: var(--ff-fs-xs);
}
.srow.lic .l p {
  margin: 0;
  font-size: var(--ff-fs-xs);
  line-height: 1.5;
  color: var(--ff-text-2);
}
.srow.lic .l p + p {
  margin-top: var(--ff-space-2);
}
.srow.lic .l p .lnk {
  margin-left: var(--ff-space-2);
}
/* 字体名：同字号加粗，--ff-text-1（.srow .l b 是 display:block，这里要行内） */
.srow.lic .l p .fn {
  display: inline;
  font-weight: 600;
  color: var(--ff-text-1);
}
</style>
