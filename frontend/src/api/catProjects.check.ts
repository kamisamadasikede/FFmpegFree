// Cat 项目接口层自检（api.check.ts 调用）：浏览器内存模拟（node 下没有 window.go）的契约规则 + 错误文案映射。
import { AppError } from './call'
import {
  CAT_PROJECT_COPY,
  createCatProject,
  deleteCatProject,
  duplicateProjectId,
  folderName,
  isRootPath,
  listCatProjects,
  pathKey,
  projectErrorText,
  relocateCatProject,
  renameCatProject,
  resetCatProjectSim,
  revealCatProject,
  revealLabel,
  setSimFolderGone,
  validProjectName,
} from './catProjects'

type Eq = (name: string, got: unknown, want: unknown) => void
const code = async (p: Promise<unknown>) => {
  try {
    await p
    return 'ok'
  } catch (e) {
    const err = e as AppError
    return err.reason ? `${err.code}:${err.reason}` : err.code
  }
}

export async function catProjectsChecks(eq: Eq): Promise<void> {
  eq('项目：文件夹名 / 路径键 / 根目录', [folderName('D:\\a\\字幕项目\\'), folderName('/Users/me/x'), pathKey('D:\\A\\B\\'), pathKey('d:/a/b'), isRootPath('C:\\'), isRootPath('/'), isRootPath('\\\\srv\\share\\'), isRootPath('D:\\a')],
    ['字幕项目', 'x', 'd:\\a\\b', 'd:\\a\\b', true, true, true, false])
  eq('项目：名字 1~60 字（按字符）', [validProjectName(''), validProjectName('  '), validProjectName('长'.repeat(60)), validProjectName('长'.repeat(61)), validProjectName('a\nb')], [false, false, true, false, false])
  resetCatProjectSim()
  eq('项目：没有项目返回空数组', await listCatProjects(), [])
  const a = await createCatProject({ path: 'D:\\工作\\字幕项目' })
  eq('项目：新建默认名 = 文件夹名、existed=false', [a.project.name, a.existed, a.project.missing], ['字幕项目', false, false])
  const b = await createCatProject({ path: 'd:\\工作\\字幕项目\\', name: '别的名字' })
  eq('项目：同一文件夹（大小写 / 末尾分隔符）→ existed=true、不改名', [b.existed, b.project.id === a.project.id, b.project.name, (await listCatProjects()).length], [true, true, '字幕项目', 1])
  eq('项目：校验 reason', [await code(createCatProject({ path: 'C:\\' })), await code(createCatProject({ path: 'rel' })), await code(renameCatProject({ id: a.project.id, name: '长'.repeat(61) })), await code(renameCatProject({ id: 'nope', name: 'x' }))],
    ['INVALID_ARGUMENT:project_root', 'INVALID_ARGUMENT:project_path', 'INVALID_ARGUMENT:project_name', 'NOT_FOUND'])
  const c = await createCatProject({ path: 'D:\\工作\\直播' })
  setSimFolderGone('D:\\工作\\字幕项目', true)
  eq('项目：文件夹不见了 → missing；显示文件夹 → CAT_PROJECT_MISSING', [(await listCatProjects()).find((p) => p.id === a.project.id)?.missing, await code(revealCatProject({ id: a.project.id }))], [true, 'CAT_PROJECT_MISSING'])
  let dup: unknown
  try {
    await relocateCatProject({ id: a.project.id, path: 'D:\\工作\\直播' })
  } catch (e) {
    dup = e
  }
  eq('项目：重新选择到别的项目的文件夹 → project_duplicate + projectId 行；本项目仍缺', [projectErrorText(dup), duplicateProjectId(dup), (await listCatProjects()).find((p) => p.id === a.project.id)?.missing], [CAT_PROJECT_COPY.existed, c.project.id, true])
  const r = await relocateCatProject({ id: a.project.id, path: 'D:\\工作\\字幕-第3季' })
  eq('项目：重新选择成功 → 路径更新、名字不变、不再缺', [r.path, r.name, r.missing], ['D:\\工作\\字幕-第3季', '字幕项目', false])
  await deleteCatProject({ id: c.project.id })
  await deleteCatProject({ id: c.project.id })
  eq('项目：删除幂等', (await listCatProjects()).map((p) => p.id), [a.project.id])
  eq('项目：错误文案映射（不出错误码 / reason）', [
    projectErrorText(new AppError('INVALID_ARGUMENT', 'x', 'reason=project_path')),
    projectErrorText(new AppError('INVALID_ARGUMENT', 'x', 'reason=project_root')),
    projectErrorText(new AppError('INVALID_ARGUMENT', 'x', 'reason=project_name')),
    projectErrorText(new AppError('TASK_CONFLICT', 'x', 'reason=turn_running')),
    projectErrorText(new AppError('CAT_PROJECT_MISSING', 'x')),
    projectErrorText(new AppError('PROCESS_FAILED', 'x')),
    duplicateProjectId(new AppError('INVALID_ARGUMENT', 'x', 'reason=project_duplicate')),
  ], ['请选择一个文件夹。', '不能把整个磁盘作为项目，请选择里面的文件夹。', '名字需要 1~60 个字。', '有对话正在回复，请先停止再换文件夹。', '项目文件夹不见了。', CAT_PROJECT_COPY.failed, ''])
  eq('项目：显示文件夹按平台', ['windows', 'darwin', 'linux'].map(revealLabel), ['在资源管理器中显示', '在访达中显示', '在文件管理器中显示'])
  resetCatProjectSim()
}
