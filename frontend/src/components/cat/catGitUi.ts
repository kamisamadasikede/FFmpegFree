import { reactive } from 'vue'
import type { GitStatus } from '@/api/catGit'

/** 右栏 Git 状态摘要，供顶栏胶囊与右栏共用 */
export const catGitUi = reactive({
  convId: '',
  live: false,
  status: null as GitStatus | null,
  simBranches: false,
})

export function setCatGitUi( partial: Partial<typeof catGitUi>) {
  Object.assign(catGitUi, partial)
}
