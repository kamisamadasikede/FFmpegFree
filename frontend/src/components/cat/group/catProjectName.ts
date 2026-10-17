import { catState } from '@/views/cat/catState'

export const catProjectName = (id: string): string => catState.projects.find((p) => p.id === id)?.name ?? ''
