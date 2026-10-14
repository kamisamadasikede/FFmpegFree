import { ref } from 'vue'
import { t } from '@/i18n'

/** 顶栏胶囊与右栏分支钮共用：同一时刻只开一处 */
export type CatBranchMenuSource = 'top' | 'panel' | null

export const catBranchMenuOpen = ref<CatBranchMenuSource>(null)

export function openCatBranchMenu(source: Exclude<CatBranchMenuSource, null>) {
  catBranchMenuOpen.value = source
}

export function closeCatBranchMenu() {
  catBranchMenuOpen.value = null
}

export function toggleCatBranchMenu(source: Exclude<CatBranchMenuSource, null>) {
  catBranchMenuOpen.value = catBranchMenuOpen.value === source ? null : source
}
