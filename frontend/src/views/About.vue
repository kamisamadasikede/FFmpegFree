<template>
  <div class="about spanels">
    <!-- 品牌块（原型：logo + 名称 + 版本徽标 + 右侧操作） -->
    <section class="brandcard">
      <AppLogo :size="56" decorative />
      <div class="bt">
        <h2>
          {{ APP_NAME }}
          <span class="ver">{{ version }}</span>
        </h2>
        <p>{{ t('about.licenseSummary') }}</p>
      </div>
      <div class="bact">
        <a class="btn" :href="PROJECT_URL" @click.prevent="openExternal(PROJECT_URL)">
          <FIcon name="link" :size="14" />{{ t('about.projectUrl') }}
        </a>
        <a class="btn" :href="LICENSE_URL" @click.prevent="openExternal(LICENSE_URL)">
          <FIcon name="link" :size="14" />{{ t('about.license') }}
        </a>
      </div>
    </section>

    <!-- 三卡：运行组件（复用 FFmpegPanel，只读） -->
    <FFmpegPanel heading-id="h-ffmpeg" readonly />

    <!-- 第三方许可 -->
    <section class="panel group" aria-labelledby="h-third">
      <div class="phead"><h2 id="h-third">{{ t('about.thirdParty') }}</h2></div>
      <div class="srow lic">
        <div class="l">
          <b>{{ t('about.fontSource') }}</b>
          <p v-for="lic in fontLicenses" :key="lic.name">
            {{ lic.before }}<b class="fn">{{ lic.font }}</b>{{ lic.after
            }}<a
              class="lnk sm"
              role="button"
              tabindex="0"
              @click.prevent="openLicense(lic)"
              @keydown.enter.prevent="openLicense(lic)"
              @keydown.space.prevent="openLicense(lic)"
              >{{ t('about.viewLicenseText') }}</a
            >
          </p>
        </div>
      </div>
    </section>

    <LicenseDialog v-if="current" v-model="dialogOpen" :name="current.name" :title="current.dialogTitle" />
  </div>
</template>

<script setup lang="ts">
// 「关于」页（v0.58 对齐原型）：品牌块 + 组件面板 + 第三方许可清单。
// 说明文字用 --ff-text-2（--ff-text-3 只留给占位和禁用）。
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
const fontLicenses = computed(() =>
  FONT_LICENSES.map((lic) => ({
    ...lic,
    before: t('about.fontNotoBefore'),
    after: t('about.fontNotoAfter'),
    dialogTitle: t('about.fontNotoDialog'),
  })),
)

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
  max-width: 960px;
}

/* 品牌块：与原型 .brandcard 一致（渐变辉光 + 1px 描边） */
.brandcard {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 22px var(--ff-space-6);
  border-radius: var(--ff-radius-lg);
  border: 1px solid var(--ff-border);
  background:
    radial-gradient(120% 160% at 0% 0%, color-mix(in srgb, var(--ff-primary) 16%, transparent), transparent 58%),
    radial-gradient(90% 140% at 100% 0%, rgba(123, 92, 255, 0.14), transparent 60%),
    var(--ff-bg-surface);
  min-width: 0;
}
.bt {
  min-width: 0;
  flex: 1;
}
.bt h2 {
  margin: 0;
  font-size: 20px;
  letter-spacing: -0.015em;
  display: flex;
  align-items: center;
  gap: 9px;
}
.bt .ver {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--ff-text-2);
  background: var(--ff-bg-card-2);
  border: 1px solid var(--ff-line-strong);
  border-radius: 5px;
  padding: 2px 7px;
  letter-spacing: 0;
}
.bt p {
  margin: 5px 0 0;
  font-size: 12.5px;
  line-height: 1.6;
  color: var(--ff-text-2);
}
.bact {
  display: flex;
  align-items: center;
  gap: var(--ff-space-2);
  flex: none;
}
.lnk.sm {
  margin-left: var(--ff-space-2);
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

/* 窄屏：品牌块换行，操作按钮落到底部一行 */
@media (max-width: 860px) {
  .brandcard {
    flex-wrap: wrap;
  }
  .bact {
    width: 100%;
  }
}
</style>
