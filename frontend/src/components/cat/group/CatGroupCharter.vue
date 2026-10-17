<template>
  <div class="cg-ch" data-testid="cat-group-charter" :data-status="c.status">
    <div class="hd">
      <FIcon name="doc" :size="12" />{{ t('cat.groupChat.charter') }}<span class="sp" />
      <template v-if="c.status === 'ok' && !editing">
        <button type="button" class="cg-btn ghost" data-testid="cat-group-charter-edit" @click="startEdit()"><FIcon name="edit" :size="12" />{{ t('cat.groupChat.edit') }}</button>
        <button type="button" class="cg-btn ghost" data-testid="cat-group-charter-regen" @click="regen()"><FIcon name="refresh" :size="12" />{{ t('cat.groupChat.regenerate') }}</button>
      </template>
    </div>

    <div v-if="c.status === 'generating'" class="cg-gen" role="status" data-testid="cat-group-charter-generating">
      <span class="cg-spin" style="margin-top: 3px" />
      <div class="bx">
        <div class="t">{{ t('cat.groupChat.generating') }}</div>
        <div class="h">{{ t('cat.groupChat.generatingHint', { name: member.name }) }}</div>
        <i class="cg-skel" style="width: 90%" /><i class="cg-skel" style="width: 70%" /><i class="cg-skel" style="width: 40%" />
      </div>
    </div>

    <div v-else-if="(c.status === 'failed' || c.status === 'none') && !editing" class="cg-gen bad" data-testid="cat-group-charter-failed">
      <span style="display: inline-flex; color: var(--cat-warn-text); margin-top: 2px"><FIcon name="warn" :size="14" /></span>
      <div class="bx">
        <div class="t">{{ t('cat.groupChat.charterFailed') }}</div>
        <div class="h">{{ t('cat.groupChat.charterFailedHint') }}</div>
        <div class="row">
          <button type="button" class="cg-btn pri" data-testid="cat-group-charter-regen" @click="regen(true)"><FIcon name="refresh" :size="12" />{{ t('cat.groupChat.regenerate') }}</button>
          <button type="button" class="cg-btn" @click="startEdit()">{{ t('cat.groupChat.edit') }}</button>
        </div>
      </div>
    </div>

    <template v-else>
      <div class="cg-fld">
        <label :for="`own-${member.id}`">{{ t('cat.groupChat.owns') }}<span v-if="isEdited('owns')" class="cg-badge ed" :title="t('cat.groupChat.editedTip')">{{ t('cat.groupChat.edited') }}</span></label>
        <textarea v-if="editing" :id="`own-${member.id}`" v-model="form.owns" rows="2" />
        <div v-else class="cg-ro">{{ c.owns }}</div>
      </div>
      <div class="cg-fld">
        <label :for="`nt-${member.id}`">{{ t('cat.groupChat.notTouch') }}<span v-if="isEdited('notTouch')" class="cg-badge ed" :title="t('cat.groupChat.editedTip')">{{ t('cat.groupChat.edited') }}</span></label>
        <textarea v-if="editing" :id="`nt-${member.id}`" v-model="form.notTouch" rows="2" />
        <div v-else class="cg-ro">{{ c.notTouch }}</div>
      </div>
      <div class="cg-fld">
        <span class="lb">{{ t('cat.groupChat.handoff') }}<span class="cg-badge acc" :title="t('cat.groupChat.handoffAutoTip')">{{ t('cat.groupChat.handoffAuto') }}</span></span>
        <div class="cg-ro" :title="t('cat.groupChat.handoffAutoTip')">{{ c.handoff || DASH }}</div>
      </div>
      <div class="cg-fld">
        <span class="lb">{{ t('cat.groupChat.dirs') }}<span v-if="isEdited('dirs')" class="cg-badge ed" :title="t('cat.groupChat.editedTip')">{{ t('cat.groupChat.edited') }}</span></span>
        <div v-if="editing" class="cg-dirs" @click="dirEl?.focus()">
          <span v-for="d in form.dirs" :key="d" class="cg-dir">{{ d }}<button type="button" :aria-label="t('cat.groupChat.removeDir', { dir: d })" @click.stop="form.dirs = form.dirs.filter((x) => x !== d)"><FIcon name="x" :size="10" /></button></span>
          <input ref="dirEl" v-model="dirDraft" :placeholder="form.dirs.length ? '' : t('cat.groupChat.dirsPh')" :aria-label="t('cat.groupChat.dirs')" @keydown.enter.prevent="addDir()" @keydown.backspace="onDirBack" @blur="addDir()">
        </div>
        <div v-else-if="c.dirs.length" class="cg-dirlist"><span v-for="d in c.dirs" :key="d" class="cg-dir">{{ d }}</span></div>
        <div v-if="editing ? !form.dirs.length : !c.dirs.length" class="cg-hint">{{ t('cat.groupChat.dirsEmpty') }}</div>
      </div>
      <div v-if="editing" class="acts">
        <button type="button" class="cg-btn" @click="editing = false">{{ t('cat.groupChat.cancel') }}</button>
        <button type="button" class="cg-btn pri" :disabled="saving" data-testid="cat-group-charter-save" @click="save()">{{ t('cat.groupChat.save') }}</button>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import FIcon from '@/components/icon/FIcon.vue'
import type { CatGroupMember } from '@/api/catGroupChat'
import { useCatGroupChat } from '@/stores/catGroupChat'

const props = defineProps<{ member: CatGroupMember; startEditing?: boolean }>()
const { t } = useI18n()
const store = useCatGroupChat()
const DASH = '—'
const c = computed(() => props.member.charter)
const isEdited = (f: string) => c.value.edited.includes(f)

const editing = ref(false)
const saving = ref(false)
const form = reactive({ owns: '', notTouch: '', dirs: [] as string[] })
const dirDraft = ref('')
const dirEl = ref<HTMLInputElement | null>(null)
function startEdit() {
  form.owns = c.value.owns
  form.notTouch = c.value.notTouch
  form.dirs = [...c.value.dirs]
  dirDraft.value = ''
  editing.value = true
}
if (props.startEditing && c.value.status === 'ok') startEdit()

/** 目录：相对项目根，去掉开头 ./ 和 /，统一 / 结尾，去重（契约 6.21.2：前缀匹配） */
function addDir() {
  const raw = dirDraft.value.trim().replace(/\\/g, '/').replace(/^\.?\/+/, '')
  dirDraft.value = ''
  if (!raw) return
  const d = raw.endsWith('/') ? raw : `${raw}/`
  if (!form.dirs.includes(d)) form.dirs.push(d)
}
function onDirBack() {
  if (!dirDraft.value) form.dirs.pop()
}
async function save() {
  addDir()
  saving.value = true
  try {
    // 契约 UpdateCatCharter 只收 owns / notTouch / dirs；handoff 由 Cat 自动维护
    await store.saveCharter(props.member.id, { owns: form.owns, notTouch: form.notTouch, dirs: form.dirs })
    editing.value = false
    ElMessage.success(t('cat.groupChat.charterSaved'))
  } catch {
    ElMessage.error(t('cat.groupChat.saveFailed'))
  } finally {
    saving.value = false
  }
}
async function regen(skipConfirm = false) {
  if (!skipConfirm && c.value.edited.length) {
    try {
      await ElMessageBox.confirm(t('cat.groupChat.regenerateConfirm'), t('cat.groupChat.regenerate'), { confirmButtonText: t('cat.groupChat.regenerate'), cancelButtonText: t('cat.groupChat.cancel') })
    } catch {
      return
    }
  }
  try {
    editing.value = false
    await store.regenerate(props.member.id)
  } catch (e) {
    const d = String((e as { detail?: string })?.detail ?? (e as Error)?.message ?? '')
    ElMessage.warning(d.includes('charter_generating') ? t('cat.groupChat.regenerating') : t('cat.groupChat.saveFailed'))
  }
}
</script>
