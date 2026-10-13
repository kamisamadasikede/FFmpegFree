// Cat 本机偏好：新对话放在 / 回复逐字显示
import { catLocalPrefs, rememberNewConvProject, resolveNewConvProject, setNewConvPlace, setTypewriter } from './catLocalPrefs'

type Eq = (name: string, got: unknown, want: unknown) => void

export function catLocalPrefsChecks(eq: Eq) {
  const saved = { ...catLocalPrefs }
  const ps = [{ id: 'p1' }, { id: 'p2', missing: true }, { id: 'p3' }]
  try {
    eq('本机偏好：默认「上次用的项目」、逐字显示开', [saved.newConvPlace, saved.typewriter], ['last', true])
    setNewConvPlace('last')
    rememberNewConvProject('p3')
    eq('新对话放在：上次用的项目', resolveNewConvProject(ps), 'p3')
    rememberNewConvProject('p9')
    eq('新对话放在：上次的项目没了 → 不放在项目里', resolveNewConvProject(ps), '')
    setNewConvPlace('none')
    eq('新对话放在：不放在项目里', resolveNewConvProject(ps), '')
    setNewConvPlace('p1')
    eq('新对话放在：指定项目', resolveNewConvProject(ps), 'p1')
    setNewConvPlace('p2')
    eq('新对话放在：文件夹不见了 → 不放', resolveNewConvProject(ps), '')
    setTypewriter(false)
    eq('逐字显示：可以关掉', catLocalPrefs.typewriter, false)
  } finally {
    Object.assign(catLocalPrefs, saved)
  }
}
