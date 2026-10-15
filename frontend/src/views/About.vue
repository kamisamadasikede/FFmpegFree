<template>
  <div class="about spanels">
    <!-- 关于 -->
    <section class="panel group" aria-labelledby="h-about">
      <div class="phead"><h2 id="h-about">{{ t('about.title') }}</h2></div>
      <div class="idrow">
        <AppLogo :size="48" decorative />
        <b>{{ APP_NAME }}</b>
      </div>
      <div class="srow">
        <div class="l"><b>{{ t('about.version') }}</b></div>
        <div class="val">{{ version }}</div>
      </div>
      <div class="srow">
        <div class="l">
          <b>{{ t('about.projectLicense') }}</b>
          <small>{{ t('about.licenseSummary') }}</small>
        </div>
        <div class="links">
          <a class="lnk" :href="PROJECT_URL" @click.prevent="openExternal(PROJECT_URL)"><FIcon name="link" :size="14" />{{ t('about.projectUrl') }}</a>
          <a class="lnk" :href="LICENSE_URL" @click.prevent="openExternal(LICENSE_URL)"><FIcon name="link" :size="14" />{{ t('about.license') }}</a>
        </div>
      </div>
    </section>

    <!-- ffmpeg：只读，与设置页共用 FFmpegPanel（不传操作插槽） -->
    <FFmpegPanel heading-id="h-ffmpeg" readonly />

    <!-- 第三方许可 -->
    <section class="panel group" aria-labelledby="h-third">
      <div class="phead"><h2 id="h-third">{{ t('about.thirdParty') }}</h2></div>
      <div class="srow lic">
        <div class="l">
          <b>{{ t('about.fontSource') }}</b>
          <!-- 每条一个 <p>，链接跟在末尾；允许在窄窗口下折成两行，不截断、不 nowrap -->
          <p v-for="lic in fontLicenses" :key="lic.name">{{ lic.before }}<b class="fn">{{ lic.font }}</b>{{ lic.after }}<a class="lnk sm" role="button" tabindex="0" @click.prevent="openLicense(lic)" @keydown.enter.prevent="openLicense(lic)" @keydown.space.prevent="openLicense(lic)">{{ t('about.viewLicenseText') }}</a></p>
        </div>
      </div>
    </section>

    <LicenseDialog v-if="current" v-model="dialogOpen" :name="current.name" :title="current.dialogTitle" />
  </div>
</template>

<script setup lang="ts">
// 「关于」页（设计说明 v0.1）：三个面板，复用设置页的 .group / .srow 样式（styles/settings-panels.css，根节点 .spanels）。
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import FIcon from '@/components/icon/FIcon.vue'
import AppLogo from '@/components/brand/AppLogo.vue'
import { APP_NAME } from '@/config/brand'
import FFmpegPanel from '@/components/settings/FFmpegPanel.vue'
import LicenseDialog from '@/components/settings/LicenseDialog.vue'
import { getAppVersion, DEV_VERSION } from '@/api/about'
import { FONT_LICENSES, LICENSE_URL, PROJECT_URL, type FontLicense } from '@/config/about'
import { openExternal } from '@/utils/openExternal'

const { t } = useI18n()
const version = ref(DEV_VERSION)
const fontLicenses = computed(() => FONT_LICENSES.map((lic) => ({
  ...lic,
  before: t('about.fontNotoBefore'),
  after: t('about.fontNotoAfter'),
  dialogTitle: t('about.fontNotoDialog'),
})))

onMounted(async () => {
  try {
    version.value = (await getAppVersion()).trim() || DEV_VERSION
  } catch (e) {
    console.error(t('cat.misc.s_df60b080be'), e)
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
