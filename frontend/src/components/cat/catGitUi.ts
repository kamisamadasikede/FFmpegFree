import { reactive } from 'vue'
import type { GitStatus } from '@/api/catGit'
import { t } from '@/i18n'

/** 右栏 Git 状态摘要，供顶栏胶囊与右栏共用 */
export const catGitUi = reactive({
  convId: '',
  live: false,
  status: null as GitStatus | null,
  simBranches: false,
  /** 合并出冲突后 +1：文件栏切到 Git 页并刷新状态（v0.50） */
  conflictNav: 0,
  /** 合并成功后 +1：只刷新 Git 状态 */
  gitRefresh: 0,
})

export function setCatGitUi( partial: Partial<typeof catGitUi>) {
  Object.assign(catGitUi, partial)
}
