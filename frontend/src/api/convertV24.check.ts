// 转换页 v0.24 / v0.24.1（契约 §6.15–6.17）自检：由 api.check.ts 调用。测真实绑定的包装（假 window.go）、模拟层、纯函数和 store 里的 X6 删除框。
import { CONVERT_V24_BACKEND_READY } from './flags'
import * as cr from './convertRecords'
import * as mock from './convertRecordsMock'
import { mockFormatCatalog } from './formatCatalogMock'
import { toAppError } from './call'
import { createPinia, setActivePinia } from 'pinia'
import { useConvertRecordsStore } from '@/stores/convertRecords'
import { isRetiredType, RETIRED_TASK_TYPES } from '@/stores/tasks'
import { coverKindOf } from '@/utils/convertText'
import * as t24 from '@/utils/convertV24Text'

type Eq = (name: string, got: unknown, want: unknown) => void
type Win = Record<string, unknown> & { location: { search: string } }

export async function convertV24Checks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  const win = (globalThis as unknown as { window: Win }).window
  // ---- 开关：后端 #95 已合入，v0.24 联调打开 ----
  eq('v0.24 开关：CONVERT_V24_BACKEND_READY 为 true（后端 #95 已合入）', CONVERT_V24_BACKEND_READY, true)
  eq('纯浏览器（无 window.go）：v0.24 界面打开，走模拟', [cr.convertV24On(), cr.convertV24IsReal()], [true, false])
  eq('旧的按名字调用（convertV24Binding.ts）已删除，v0.24 真实调用都在 convertRecordsBinding.ts（生成的绑定）', [/convertV24Binding/.test(readSrc('src/api/convertRecords.ts')), /wailsjs\/go\/app\/SystemService/.test(readSrc('src/api/convertRecordsBinding.ts'))], [false, true])

  // ---- 真实绑定的包装（假 window.go）：形状按 v0.24.1 ----
  {
    const calls: [string, unknown][] = []
    const copyCbs: ((e: unknown) => void)[] = []
    const rec = (name: string, ret: unknown) => async (a?: unknown) => (calls.push([name, a === undefined ? null : JSON.parse(JSON.stringify(a))]), ret)
    const task = { id: 't1', type: 'convert', status: 'queued', inputPaths: ['C:\\a.mp4'], outputPath: 'D:\\out\\a.mp4', params: '{}', createdAt: 1, reconverting: true }
    win.go = {
      app: {
        ConvertService: {
          SubmitSources: rec('SubmitSources', { tasks: [task], skipped: [{ sourceId: 's2', reason: 'copying' }, { sourceId: 's3', reason: 'copy_failed' }] }),
          Reconvert: rec('Reconvert', task),
          CancelCopy: rec('CancelCopy', null),
          RetryCopy: rec('RetryCopy', { sourceId: 's2', path: 'C:\\b.mp4', name: 'b.mp4', addedAt: 1, lastActivityAt: 1, originalPath: 'C:\\b.mp4', storedPath: 'D:\\up\\s2\\b.mp4', copyState: 'copying', copiedBytes: 0, totalBytes: 10 }),
          GetFormatCatalog: rec('GetFormatCatalog', [{ category: 'image', extension: 'png', displayName: 'PNG 图片', aliases: null, encodable: true, defaultPresetId: 'builtin-png', presets: null }]),
          TakeInterruptedReconverts: rec('TakeInterruptedReconverts', 2),
        },
        SystemService: {
          GetStorageDirs: rec('GetStorageDirs', { outputDir: 'D:\\FF\\output', uploadsDir: 'D:\\FF\\uploads', outputCustom: false, uploadsCustom: false, defaultOutputDir: 'D:\\FF\\output', defaultUploadsDir: 'D:\\FF\\uploads', fellBack: false }),
          SetStorageDirs: rec('SetStorageDirs', { outputDir: 'E:\\o', uploadsDir: 'D:\\FF\\uploads', outputCustom: true, uploadsCustom: false, defaultOutputDir: 'D:\\FF\\output', defaultUploadsDir: 'D:\\FF\\uploads', fellBack: false }),
          OpenStorageFolder: rec('OpenStorageFolder', null),
        },
        TaskService: {
          CheckPaths: rec('CheckPaths', [{ taskId: 't1', found: true, inputExists: true, outputExists: false, reconvertMode: 'regenerate', reconvertBlock: '' }]),
        },
      },
    }
    win.runtime = { EventsOnMultiple: (ev: string, cb: (e: unknown) => void) => (ev === 'convert:copy' && copyCbs.push(cb), () => undefined) }
    try {
      eq('有 Wails：v0.24 走真实绑定', [cr.convertV24IsReal(), cr.convertV24On()], [true, true])
      const sub = await cr.submitSources({ sourceIds: ['s1', 's2', 's3'], presetId: 'builtin-mp4', outputDir: '' } as unknown as Parameters<typeof cr.submitSources>[0])
      eq('SubmitSources：返回 {tasks, skipped}，skipped 原样带 sourceId / reason', [sub.tasks.length, sub.tasks[0].reconverting, sub.skipped], [1, true, [{ sourceId: 's2', reason: 'copying' }, { sourceId: 's3', reason: 'copy_failed' }]])
      calls.length = 0
      await cr.reconvert({ taskId: 't1' })
      await cr.reconvert({ taskId: 't1', presetId: 'builtin-mp4-hevc' })
      eq('Reconvert：传 ReconvertRequest；沿用参数时只有 taskId，换预设时带 presetId', calls.map((c) => c[1]), [{ taskId: 't1' }, { taskId: 't1', presetId: 'builtin-mp4-hevc' }])
      calls.length = 0
      await cr.cancelCopy('s2')
      const rs = await cr.retryCopy('s2')
      eq('CancelCopy / RetryCopy：按 sourceId；RetryCopy 返回行（copyState=copying）', [calls.map((c) => c[0]), calls.map((c) => c[1]), rs.copyState], [['CancelCopy', 'RetryCopy'], ['s2', 's2'], 'copying'])
      const cat = await cr.getFormatCatalog()
      eq('GetFormatCatalog：aliases / presets 为 null 时补 []', [cat[0].aliases, cat[0].presets, cat[0].category], [[], [], 'image'])
      eq('TakeInterruptedReconverts：返回条数；提示文字说“重转”', [await cr.takeInterruptedReconverts(), t24.interruptedReconvertsText(2)], [2, '上次退出时有 2 条重转被中断，原来的文件没有变动。'])
      calls.length = 0
      const sd = await cr.getStorageDirs()
      await cr.setStorageDirs({ outputDir: 'E:\\o', uploadsDir: '' })
      await cr.openStorageFolder('uploads')
      eq('存储目录：GetStorageDirs / SetStorageDirs（两个都传，"" = 默认）/ OpenStorageFolder("uploads")', [sd.defaultOutputDir, calls.map((c) => c[0]), calls[1][1], calls[2][1]], ['D:\\FF\\output', ['GetStorageDirs', 'SetStorageDirs', 'OpenStorageFolder'], { outputDir: 'E:\\o', uploadsDir: '' }, 'uploads'])
      const pc = await cr.checkPaths(['t1'])
      eq('CheckPaths：reconvertMode / reconvertBlock（取代 canReconvert）', [pc[0].reconvertMode, pc[0].reconvertBlock, 'canReconvert' in pc[0]], ['regenerate', '', false])
      const got: unknown[] = []
      const off = cr.onCopyEvent((e) => got.push(e))
      copyCbs.forEach((cb) => cb({ sourceId: 's2', seq: 7, copyState: 'ready', copiedBytes: 10, totalBytes: 10, storedPath: 'D:\\up\\s2\\b.mp4' }))
      off()
      eq('convert:copy：订阅真实事件', [copyCbs.length, (got[0] as { seq: number })?.seq], [1, 7])
    } finally {
      delete win.go
      delete win.runtime
    }
    eq('去掉 window.go 后回到模拟', cr.convertV24IsReal(), false)
  }

  // ---- 格式目录与模糊搜索（§八 第 35–38 条） ----
  {
    const cat = mockFormatCatalog(mock.mockParamsSummary)
    const exts = (q: string) => t24.searchFormats(cat, q).flatMap((g) => g.items.map((h) => h.entry.extension))
    eq('格式目录（模拟）：视频 / 音频 / 图片三类；图片 7 个', [new Set(cat.map((f) => f.category)).size, cat.filter((f) => f.category === 'image').map((f) => f.extension).sort()], [3, ['bmp', 'ico', 'jpg', 'png', 'tga', 'tif', 'webp']])
    eq('搜索：不区分大小写，扩展名完全相同排第一', exts('MP4')[0], 'mp4')
    eq('搜索：扩展名开头 > 显示名 > 别名', [t24.formatRank({ extension: 'mp4', displayName: 'MP4', aliases: [], category: 'video', encodable: true }, 'mp'), t24.formatRank({ extension: 'm4a', displayName: 'M4A 音频', aliases: ['苹果音频'], category: 'audio', encodable: true }, '苹果')], [1, 3])
    eq('搜索“动图”：GIF 在视频分类里（别名命中）', t24.searchFormats(cat, '动图').map((g) => [g.category, g.items.map((h) => h.entry.extension)]).find((g) => (g[1] as string[]).includes('gif'))?.[0], 'video')
    const apple = t24.searchFormats(cat, '苹果')
    const m4r = apple.flatMap((g) => g.items).find((h) => h.entry.extension === 'm4r')
    eq('搜索“苹果”：M4R 命中，第二行换成命中的别名', [!!m4r, m4r?.sub.includes('苹果')], [true, true])
    eq('搜索：空关键字 → 不分组；没有命中 → []（显示“没有找到…”）', [t24.searchFormats(cat, '  '), exts('rmvb'), t24.formatNoneText('rmvb')], [[], [], '没有找到和“rmvb”相关的格式'])
    const off = cat.filter((f) => !f.encodable)
    eq('不可输出的格式仍在搜索结果里（置灰），有原因；原因不出现 ffmpeg', [off.length > 0, off.every((f) => exts(f.extension).includes(f.extension)), off.every((f) => !!f.reason && !/ffmpeg/i.test(f.reason))], [true, true, true])
    eq('高亮：按关键字切段，不区分大小写', t24.highlightParts('MP4 视频', 'mp'), [{ t: 'MP', hit: true }, { t: '4 视频', hit: false }])
  }

  // ---- 封面：格式目录里“图片”分类的格式用图片封面（包 20 的 image 封面） ----
  eq('封面：目录分类 image → 图片封面（tga 也是）；GIF 在视频分类 → 胶片；音频分类 → 音符', [coverKindOf('tga', null, 'image'), coverKindOf('png'), coverKindOf('gif', null, 'video'), coverKindOf('m4r', null, 'audio')], ['image', 'image', 'video', 'audio'])

  // ---- 提交跳过（§八 第 53 条） ----
  eq('部分跳过：提示开始的个数 + 复制中 / 失败分开说', t24.submitSkipToast(2, [{ reason: 'copying' }, { reason: 'copy_failed' }, { reason: 'copy_canceled' }]), '已开始转换 2 个文件。有 1 个文件还在复制，已跳过，复制完成后再转换。有 2 个文件没有复制成功，已跳过。')
  eq('没有跳过：不提示', t24.submitSkipToast(3, []), '')
  {
    mock.resetConvertMock('mixed')
    const r = await mock.SubmitSources({ sourceIds: [], presetId: 'builtin-mp4', outputDir: '' } as unknown as Parameters<typeof mock.SubmitSources>[0]).catch((e) => toAppError(e))
    eq('模拟 SubmitSources：空列表报 INVALID_ARGUMENT', (r as { code?: string }).code, 'INVALID_ARGUMENT')
  }

  // ---- 重转（§八 第 58、59、64、65、70 条；v0.24.1 reconvertMode） ----
  {
    const st = t24.reconvertState
    eq('reconvertMode=replace / regenerate → 可以重转', [st({ reconvertMode: 'replace', reconvertBlock: '' }, { sourceGone: false, outputGone: false }).mode, st({ reconvertMode: 'regenerate', reconvertBlock: '' }, { sourceGone: false, outputGone: true }).mode], ['replace', 'regenerate'])
    eq('reconvertMode="" → 置灰 + 原因（output_moved / copy_not_ready / invalid_state）', ['output_moved', 'copy_not_ready', 'invalid_state'].map((b) => st({ reconvertMode: '', reconvertBlock: b }, { sourceGone: false, outputGone: false })), [{ mode: '', tip: '原来的位置已经有别的文件，不能重转' }, { mode: '', tip: '文件复制完成后才能重转' }, { mode: '', tip: '这条记录现在不能重转' }])
    eq('源文件不在优先', st({ reconvertMode: 'replace', reconvertBlock: '' }, { sourceGone: true, outputGone: false }), { mode: '', tip: '源文件已不存在，不能重转' })
    eq('还没检查：按本地推断（输出不在 → regenerate）', st(undefined, { sourceGone: false, outputGone: true }).mode, 'regenerate')
    eq('重新生成框文字 / 成功、失败提示', [t24.reconvertBody('regenerate', 'a.mp4'), t24.reconvertDoneToast('a.mp4'), t24.reconvertFailToast('regenerate'), t24.reconvertFailToast('replace', { detail: 'reason=in_use' })], ['原来的文件已经不在了，会按原来的参数重新生成“a.mp4”。', '已重转“a.mp4”。', '重转失败，请稍后重试。', '重转失败，原来的文件没有变动。文件可能正在被其他程序使用，请关闭后再重转。'])
    const errs = [
      { code: 'TASK_CONFLICT', message: '', detail: 'reason=invalid_state' }, { code: 'TASK_CONFLICT', message: '', detail: 'reason=output_moved' }, { code: 'TASK_CONFLICT', message: '', detail: 'reason=copying' },
      { code: 'NOT_FOUND', message: '', detail: 'reason=file' }, { code: 'NOT_FOUND', message: '', detail: 'reason=record' }, { code: 'INVALID_ARGUMENT', message: '不能重新转换为其他格式', detail: 'reason=format_change' }, { code: 'UNSUPPORTED', message: '', detail: 'reason=format' },
    ].map(t24.reconvertErrorText)
    eq('Reconvert 错误文字：都说“重转”，不说“重新转换”（后端 format_change 的 message 不直接显示）', [errs.every((s) => s.includes('重转')), errs.some((s) => s.includes('重新转换'))], [true, false])
    const v24Strings = Object.values(t24).filter((v) => typeof v === 'string').join('\n') + JSON.stringify(t24.RECONVERT_BLOCK_TIP)
    eq('v0.24 文案里没有“重新转换”（只留给失败 / 已取消记录的按钮）、没有 ffmpeg', [/重新转换/.test(v24Strings), /ffmpeg/i.test(v24Strings)], [false, false])
    const srcs = ['src/components/convert/ConvertReconvertDialog.vue', 'src/components/convert/ConvertKid.vue', 'src/stores/convertRecords.ts', 'src/api/convertRecordsMock.ts', 'src/api/sim.ts'].map(readSrc).join('\n')
    eq('重转相关源码：没有“重新转换”配“重转”语境的旧文字（正在重新转换 / 不能重新转换）', /正在重新转换|不能重新转换|重新转换失败/.test(srcs), false)
    mock.resetConvertMock('done')
    const recs = await mock.List({ types: ['convert'], statuses: ['succeeded'], limit: 5, offset: 0, includeHidden: true })
    const id = recs.items[0]?.id ?? ''
    const [c] = await mock.CheckPaths([id])
    eq('模拟 CheckPaths：完成的记录 reconvertMode=replace，reconvertBlock=""', [c.reconvertMode, c.reconvertBlock], ['replace', ''])
    const t = await mock.Reconvert({ taskId: id })
    eq('模拟 Reconvert：同一条记录进入重转（reconverting=true）', [t.id, t.reconverting], [id, true])
    const again = await mock.Reconvert({ taskId: id }).catch((e) => toAppError(e))
    eq('重转中再重转：TASK_CONFLICT reason=invalid_state', [(again as { code?: string }).code, /reason=invalid_state/.test((again as { detail?: string }).detail ?? '')], ['TASK_CONFLICT', true])
    const [c2] = await mock.CheckPaths([id])
    eq('重转中：reconvertMode="" reconvertBlock=invalid_state', [c2.reconvertMode, c2.reconvertBlock], ['', 'invalid_state'])
  }

  // ---- 时长偏短 / 改存横条 / 复制状态 ----
  eq('short_output：只看 result.warnings', [t24.hasShortOutput({ warnings: ['short_output'] }), t24.hasShortOutput({ warnings: [] }), t24.hasShortOutput(null)], [true, false, false])
  eq('改存横条：fellBack 且两个目录不都是自定义才显示', [t24.showFallbackBanner({ fellBack: true, outputCustom: false, uploadsCustom: false }), t24.showFallbackBanner({ fellBack: true, outputCustom: true, uploadsCustom: true }), t24.showFallbackBanner({ fellBack: false, outputCustom: false, uploadsCustom: false })], [true, false, false])
  eq('复制标签：复制中（还没开始是“等待复制”）/ 失败 / 已取消；ready、none 没有标签', [t24.copyTag('copying', 1)?.text, t24.copyTag('copying', 0)?.text, t24.copyTag('failed')?.text, t24.copyTag('canceled')?.text, t24.copyTag('ready'), t24.copyTag('none')], ['复制中', '等待复制', '复制失败', '已取消复制', null, null])
  eq('复制进度：百分比向下取整、0–100', [t24.copyPct(1, 3), t24.copyPct(5, 0), t24.copyPct(11, 10)], [33, 0, 100])
  eq('空间不足：CONVERT_DISK_FULL 或 reason=no_space', [t24.isNoSpace({ code: 'CONVERT_DISK_FULL' }), t24.isNoSpace({ code: 'IO_ERROR', detail: 'reason=no_space\nneedBytes=1' }), t24.isNoSpace({ code: 'IO_ERROR', detail: 'reason=interrupted' })], [true, true, false])

  // ---- X6：没有记录的行的删除框（场景 33 / 33b） ----
  {
    eq('X6 正文按 copyState：none → 只从列表移除；其余 → 只删除程序里的复制件', [t24.sourceRemoveEmptyBody('none'), t24.sourceRemoveEmptyBody(undefined), t24.sourceRemoveEmptyBody('ready'), t24.sourceRemoveEmptyBody('copying')], [{ text: '只从列表移除，', bold: '不删除原文件' }, { text: '只从列表移除，', bold: '不删除原文件' }, { text: '只删除程序里的复制件，', bold: '不删除原文件' }, { text: '只删除程序里的复制件，', bold: '不删除原文件' }])
    setActivePinia(createPinia())
    const cv = useConvertRecordsStore()
    mock.resetConvertMock('old-none')
    await cv.init()
    const rows = Object.values(cv.sources)
    const old = rows.find((r) => r.copyState === 'none' && !r.recordCount)
    const withRecs = rows.find((r) => r.recordCount > 0)
    const a = old ? cv.deleteAsk('source', old.sourceId) : null
    eq('X6 旧行（copyState=none，没有记录）：标题“从列表移除这个文件”，count=0（只显示文件名，不显示“0 条转换记录”）', [a?.title, a?.count, a?.copyState], ['从列表移除这个文件', 0, 'none'])
    const b = withRecs ? cv.deleteAsk('source', withRecs.sourceId) : null
    eq('有记录的行：标题不变（不是“从列表移除这个文件”），count > 0', [b?.title !== '从列表移除这个文件', (b?.count ?? 0) > 0], [true, true])
    const dlg = readSrc('src/components/convert/ConvertDeleteDialog.vue')
    eq('删除框：count=0 时不显示“条转换记录”和“同时删除输出文件”', [/a\.count > 0/.test(dlg), /sourceRemoveEmptyBody/.test(dlg)], [true, true])
  }

  // ---- 其它：剪辑任务不能重试；没有剪辑默认输出目录；N3 ----
  eq('edit_export 仍在 RETIRED_TASK_TYPES（重试按钮隐藏）', [RETIRED_TASK_TYPES.includes('edit_export'), isRetiredType('edit_export')], [true, true])
  eq('没有剪辑的默认输出目录（契约 v0.24.1 已去掉）', /editOutputDir|EditOutputDir|defaultEditOutput/.test(readSrc('src/api/system.ts') + readSrc('src/views/Settings.vue') + readSrc('src/components/settings/StoragePanel.vue')), false)
  eq('N3：任务中心 1024 进度列用短格式（速度不截断、用时一行）', /narrowRun|shortClock/.test(readSrc('src/views/TaskCenter.vue')), true)
  // D5（产品 10-08）：复制失败的行有“重试”，提示改成“请在列表里点“重试””；再次添加同一个文件在同一行重新复制
  {
    const cs = await import('@/utils/convertSubmit')
    eq('D5 文案：全部失败 / 部分失败 / 跟在“准备中”后面', [cs.SUBMIT_COPY_FAILED_TEXT, cs.skippedNotice([{ sourceId: 'a', reason: 'copy_failed' }, { sourceId: 'b', reason: 'copy_canceled' }]), cs.skippedNotice([{ sourceId: 'a', reason: 'copying' }, { sourceId: 'b', reason: 'copy_failed' }])],
      ['文件没能准备好，请在列表里点“重试”后再转换。', '有 2 个文件没能准备好，已转换其余文件。请在列表里点“重试”后再转换。', '有 1 个文件还在准备中，已转换其余文件。准备好后再点转换。另有 1 个文件没能准备好，请在列表里点“重试”。'])
    mock.resetConvertMock('copyfail')
    const before = (await mock.ListSources({ limit: 100, offset: 0 } as unknown as Parameters<typeof mock.ListSources>[0])).items.map((e) => e.source)
    const bad = before.find((x) => x.copyState === 'failed' || x.copyState === 'canceled')
    if (bad) {
      const [r] = await mock.AddSources([bad.originalPath || bad.path])
      const after = (await mock.ListSources({ limit: 100, offset: 0 } as unknown as Parameters<typeof mock.ListSources>[0])).items
      eq('再次添加复制失败的文件：同一行（同一个 sourceId、existed=true）重新复制，不新建行', [r.source?.sourceId === bad.sourceId, r.existed, r.source?.copyState, after.length === before.length], [true, true, 'copying', true])
    } else eq('copyfail 场景里有复制失败的行', false, true)
  }
  // v0.24.3：打开原文件 / 所在文件夹失败不改去复制件；直播存档空目录显示解析后的真实路径
  eq('打开原文件不在：提示“原文件不存在，无法打开。”', [t24.ORIGINAL_MISSING_OPEN, /ORIGINAL_MISSING_OPEN/.test(readSrc('src/components/convert/ConvertSourceRow.vue') + readSrc('src/components/convert/ConvertPreviewDialog.vue')), /storedPath/.test(readSrc('src/components/convert/ConvertSourceRow.vue').split('revealSrc')[1] ?? '')], ['原文件不存在，无法打开。', true, false])
  eq('直播本地存档：空的 defaultOutputDir 用 GetStorageDirs 解析出的路径（getOutputDirShown）', /getOutputDirShown/.test(readSrc('src/views/live/RecordPush.vue')), true)
}

