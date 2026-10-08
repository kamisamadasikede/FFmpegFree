// 转换页 v2（契约 v0.23 §6.14 + v0.23.1）自检：由 api.check.ts 调用。只测数据层 / 模拟层 / 纯函数和几处源码约定，不渲染组件。
import { toAppError, type AppError } from './call'
import { recordOf, parseParams } from './convertRecords'
import * as mock from './convertRecordsMock'
import { cancelSimTask, getSimTask, hideSimFinished, listSimFinished, retrySimTask, unhideSimTasks } from './sim'
import { onSimEvent } from '@/services/wails'
import { createPinia, setActivePinia } from 'pinia'
import { useTaskStore } from '@/stores/tasks'
import { recordParamsText, recordLine, deleteResultText, deleteFailureText, deleteFailureNotice, revealDeleteFailureText, sourceRemovedText, sourceRemovedParts, conflictReason, CONFLICT_NO_AUDIO, CONFLICT_NO_VIDEO, CONFLICT_TITLE, formatRecordTime } from '@/utils/convertText'
import { codecName } from '@/utils/mediaText'

type Eq = (name: string, got: unknown, want: unknown) => void
async function rejects(p: Promise<unknown>): Promise<AppError | null> {
  try {
    await p
    return null
  } catch (e) {
    return toAppError(e)
  }
}

export async function convertV2Checks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  // ---- 第 2 行（设计 §7.3 第 14 条）：一个格式化函数，三处共用 ----
  const P = { presetId: 'builtin-mp4-h264-1080p', presetName: 'MP4 1080p（H.264 + AAC）', paramsSummary: 'H.264 · 1080p', title: 'a.mov → MP4' }
  eq('第 2 行：presetId 非空 → 预设名（卡片标题），悬停 = paramsSummary', recordParamsText(P), { text: 'MP4 1080p', tip: 'H.264 · 1080p', preset: true })
  eq('第 2 行：presetId / presetName 为空、有摘要 → 自定义 · 摘要', recordParamsText({ presetId: '', presetName: '', paramsSummary: 'H.264 · 1080p · 8.0 Mbps', title: 't' }).text, '自定义 · H.264 · 1080p · 8.0 Mbps')
  eq('第 2 行：三个快照都为空（旧任务）→ title', [recordParamsText({ title: 'a.mov → MP4' }).text, recordParamsText({ presetId: '', presetName: '', paramsSummary: '', title: 'b' }).text], ['a.mov → MP4', 'b'])
  eq('recordLine：时间 · 预设名，title = paramsSummary', recordLine(P, ['今天 11:48']), { text: '今天 11:48 · MP4 1080p', title: 'H.264 · 1080p' })
  eq('recordLine：自定义 → title = 整行', recordLine({ paramsSummary: 'H.264 · 1080p', title: 't' }, ['今天 10:05']), { text: '今天 10:05 · 自定义 · H.264 · 1080p', title: '今天 10:05 · 自定义 · H.264 · 1080p' })
  eq('recordLine：任务中心不带时间（head 为空）', recordLine({ title: 'a.mov → MP4' }, []).text, 'a.mov → MP4')
  eq('recordLine：预览底栏带“… 转换”前缀和设备后缀', recordLine(P, ['今天 11:48 转换'], ['显卡']).text, '今天 11:48 转换 · MP4 1080p · 显卡')
  // ---- 参数摘要（v0.23.1：不带容器名；只给宽的写 p 值）----
  const S = mock.mockParamsSummary
  eq('摘要：只给宽 3840/2560/1920/1280/854 → 2160p/1440p/1080p/720p/480p', [3840, 2560, 1920, 1280, 854].map((w) => S({ container: 'mp4', videoCodec: 'h264', width: w })), ['H.264 · 2160p', 'H.264 · 1440p', 'H.264 · 1080p', 'H.264 · 720p', 'H.264 · 480p'])
  eq('摘要：其它宽度“宽 N”；宽高都有“宽×高”；只给高“高p”', [S({ container: 'mp4', videoCodec: 'h264', width: 1000 }), S({ container: 'mp4', videoCodec: 'h264', width: 1920, height: 800 }), S({ container: 'mp4', videoCodec: 'h264', height: 1080, videoBitrate: 8_000_000 })], ['H.264 · 宽 1000', 'H.264 · 1920×800', 'H.264 · 1080p · 8.0 Mbps'])
  eq('摘要：不含容器名；音频写码率', [S({ container: 'mp3', audioCodec: 'mp3', audioBitrate: 192000 }), /MP4|WEBM|MP3/.test(S({ container: 'webm', videoCodec: 'vp9' }))], ['192 kbps', false])
  // ---- 删除结果 / 冲突 ----
  {
    const F = (path: string) => ({ reason: path ? 'in_use' : 'still_running', message: '', path })
    eq('删除失败：只有文件没删成 → 文件句 + 打开所在文件夹（第一个非空路径）', deleteFailureNotice([F('D:\\a.mp4'), F('D:\\b.mp4')]), { text: '有 2 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。', path: 'D:\\a.mp4' })
    eq('删除失败：只有 still_running（路径为空）→ 转换没停下来，没有文件夹操作', deleteFailureNotice([F(''), F('')]), { text: '有 2 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。', path: '' })
    eq('删除失败：混合 → 两句都有，文件夹用第一个非空路径', deleteFailureNotice([F(''), F('D:\\c.mp4')]), { text: '有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。有 1 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。', path: 'D:\\c.mp4' })
    eq('删除单条记录 toast：已删除 n 条记录。+ 失败句', [deleteResultText({ deletedTaskIds: ['a'], failures: [F('D:\\a.mp4')] }), deleteFailureText([])], ['已删除 1 条记录。有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。', ''])
    eq('toast：警告 8 秒（所有失败情况）；有路径才给“打开所在文件夹”', [/type: fail \? 'warning' : 'success', duration: fail \? WARN_TOAST_MS : TOAST_MS/.test(readSrc('src/views/ConvertPage.vue')), /fail && path \? h\('button'/.test(readSrc('src/views/ConvertPage.vue'))], [true, true])
  }
  eq('移除 toast：有记录 / 删了输出 / 没有记录（不出现“0 条记录”）', [sourceRemovedText('launch-4k.mov', 3, 0), sourceRemovedText('launch-4k.mov', 3, 1), sourceRemovedText('新文件.mov', 0, 0)], ['已从列表移除“launch-4k.mov”和 3 条记录。', '已从列表移除“launch-4k.mov”和 3 条记录，并删除了 1 个文件。', '已从列表移除“新文件.mov”。'])
  eq('移除 toast：文件名单独一段（页面放进可省略、带 title 的 span）', sourceRemovedParts('a.mov', 2, 0), { before: '已从列表移除“', name: 'a.mov', after: '”和 2 条记录。' })
  eq('编码显示名：H.265 / ProRes，不出现 HEVC / PRORES；其余首字母大写', ['hevc', 'h265', 'prores', 'h264', 'av1', 'pcm_s16le', 'cinepak', 'copy'].map((c) => codecName(c)), ['H.265', 'H.265', 'ProRes', 'H.264', 'AV1', 'PCM', 'Cinepak', '原编码'])
  eq('删除结果：没有失败', deleteResultText({ deletedTaskIds: ['a'], failures: [] }), '已删除 1 条记录。')
  eq('冲突：无画面配视频 / 无声配音频 / 读取中不判断 / 兼容', [conflictReason({ hasAudio: true }, true, 'mp4'), conflictReason({ hasVideo: true }, true, 'mp3'), conflictReason({}, false, 'mp4'), conflictReason({ hasVideo: true, hasAudio: true }, true, 'mp4')], [CONFLICT_NO_VIDEO, CONFLICT_NO_AUDIO, null, null])
  eq('冲突文案定稿（不显示错误码）', [CONFLICT_TITLE, CONFLICT_NO_VIDEO, CONFLICT_NO_AUDIO], ['这个文件不能用当前预设', '没有画面，不能转成视频格式。请换一个音频预设，或取消勾选。', '没有声音，不能转成音频格式。请换一个视频预设，或取消勾选。'])
  {
    const now = new Date(2026, 9, 8, 13, 0).getTime()
    eq('记录时间：今天 / 昨天', [formatRecordTime(new Date(2026, 9, 8, 11, 48).getTime(), now), formatRecordTime(new Date(2026, 9, 7, 21, 14).getTime(), now)], ['今天 11:48', '昨天 21:14'])
  }
  // ---- 模拟层：GetSource / RevealRecord（v0.23.1）----
  mock.resetConvertMock('missing')
  const e = await mock.GetSource('mock-src-wed')
  eq('GetSource：返回一个 ConvertSourceEntry（同 ListSources 的一项）', [e.source.sourceId, e.recordCount, e.records.length, e.records.map((r) => r.id)], ['mock-src-wed', 2, 2, ['simcv-wedGif', 'simcv-wedMp4']])
  eq('GetSource 不存在：NOT_FOUND reason=record', [(await rejects(mock.GetSource('mock-src-nope')))?.code, (await rejects(mock.GetSource('mock-src-nope')))?.reason], ['NOT_FOUND', 'record'])
  eq('RevealRecord：输出已不存在 → NOT_FOUND reason=file；记录不存在 → reason=record；正常 → 成功', [(await rejects(mock.RevealRecord('simcv-wedGif')))?.reason, (await rejects(mock.RevealRecord('simcv-nope')))?.reason, await rejects(mock.RevealRecord('simcv-wedMp4'))], ['file', 'record', null])
  mock.resetConvertMock('dup')
  {
    const recs = (await mock.GetSource('mock-src-iv')).records.map(recordOf)
    const [ivNew, ivOld] = recs
    eq('模拟记录：预设记录 presetId 非空，摘要按宽写 720p；自定义记录 presetId 为空', [ivNew.presetId, ivNew.paramsSummary, ivOld.presetId, ivOld.presetName, ivOld.paramsSummary], ['builtin-mp4-h264-720p', 'H.264 · 720p', '', '', 'H.264 · 1080p · 8.0 Mbps'])
    eq('模拟记录第 2 行：预设名 / 自定义 · 摘要', [recordParamsText(ivNew).text, recordParamsText(ivOld).text], ['MP4 720p', '自定义 · H.264 · 1080p · 8.0 Mbps'])
    eq('parseParams：旧任务 params 为空 → 三个快照 undefined', [parseParams('').presetId, parseParams('{"options":{"container":"mp4"}}').paramsSummary], [undefined, undefined])
    eq('Reconvert：只接受成功的记录（v1 没有界面入口，只保留接口）', (await rejects(mock.Reconvert('simcv-nope')))?.code, 'NOT_FOUND')
  }
  // ---- ListSources status（v0.23.1）：EXISTS 语义，内嵌记录 / recordCount 不过滤，分页排序不变 ----
  {
    const ids = async (status: '' | 'active' | 'failed', limit = 50, offset = 0) => {
      const p = await mock.ListSources({ limit, offset, recordLimit: 20, status })
      return { ids: p.items.map((e) => e.source.sourceId.replace('mock-src-', '')), total: p.total, items: p.items }
    }
    mock.resetConvertMock('mixed')
    const all = await ids('')
    const act = await ids('active')
    const fail = await ids('failed')
    eq('status 筛选（mixed）：全部 5 行；进行中 / 失败都只有 launch', [all.ids, act.ids, fail.ids, act.total, fail.total], [['launch', 'iv', 'rec', 'pod', 'wed'], ['launch'], ['launch'], 1, 1])
    eq('status 筛选：行内嵌记录和 recordCount 不按状态过滤（含已完成那条）', [fail.items[0].recordCount, fail.items[0].records.map((r) => r.status)], [3, ['failed', 'running', 'succeeded']])
    eq('status 缺省 = 全部', (await mock.ListSources({ limit: 50, offset: 0, recordLimit: 20 })).total, all.total)
    mock.resetConvertMock('canceled')
    eq('status 筛选（canceled）：已取消不算失败，也不算进行中', [(await ids('failed')).ids, (await ids('active')).ids], [[], []])
    mock.resetConvertMock('running')
    const r1 = await ids('active')
    const r2 = await ids('active', 1, 1)
    eq('status 筛选（running）：排队中 / 进行中都算；排序按最近活动；分页不变', [r1.ids, r1.total, r2.ids, r2.total], [['rec', 'launch'], 2, ['launch'], 2])
    eq('status 筛选（running）：没有失败行', (await ids('failed')).ids, [])
    const bad = await rejects(mock.ListSources({ limit: 50, offset: 0, recordLimit: 20, status: 'canceled' as never }))
    eq('status 其它值 → INVALID_ARGUMENT', bad?.code, 'INVALID_ARGUMENT')
    eq('status 失败 = failed / interrupted（模拟层定义）', /failed: \['failed', 'interrupted'\]/.test(readSrc('src/api/convertRecordsMock.ts')), true)
    eq('绑定层：ListSources / SearchSources 都总是带 status（缺省 \'\'）', [/'ListSources', \{ \.\.\.f, status: f\.status \?\? '' \}/.test(readSrc('src/api/convertRecordsBinding.ts')), /'SearchSources', \{ \.\.\.f, status: f\.status \?\? '' \}/.test(readSrc('src/api/convertRecordsBinding.ts')), /interface ConvertSearchFilter extends ConvertSourceFilter \{/.test(readSrc('src/api/convertRecords.ts'))], [true, true, true])
    const st = readSrc('src/stores/convertRecords.ts')
    eq('store：筛选走 ListSources(status)；搜索带同样的 status（v0.23.2），有关键字时切筛选重新搜索', [/listSources\(\{[^}]*status \}\)/.test(st), /searchSources\(\{[^}]*, status \}\)/.test(st), /if \(kw\) return search\(keyword\.value\)/.test(st)], [true, true, true])
    eq('store：筛选“失败”时行默认展开（ListSources / SearchSources 两处）', (st.match(/status === 'failed'\) for \(const id of order\) if \(!known\.has\(id\)\) foldSession\[id\] = true|sh\.status === 'failed'\) foldSession/g) ?? []).length, 2)
    const pg = readSrc('src/views/ConvertPage.vue')
    eq('页面：没有“已加载的记录里…”提示，筛选为空时是普通空状态（设计 §四 11 文案）；搜索时筛选不置灰', [/已加载的记录里/.test(pg), pg.includes('没有进行中的记录'), pg.includes('没有失败的记录'), /filterOff|搜索时显示全部状态/.test(pg)], [false, true, true, false])
    // 契约 6.14.4：10 秒内没停下来的记录不删（still_running，path 为空）；DeleteSource 有一条没删就保留这一行
    {
      const win = (globalThis as unknown as { window: { location: { search: string } } }).window
      const prev = win.location.search
      try {
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=still_running'
        const r = await mock.DeleteSource('mock-src-launch', true)
        const left = await mock.GetSource('mock-src-launch')
        eq('still_running：进行中那条不删、path 为空；其余照删；行保留（deletedSourceIds 为空）', [r.failures.map((f) => [f.reason, f.path]), r.deletedTaskIds.length, r.deletedSourceIds, left.recordCount, left.records.map((x) => x.status)], [[['still_running', '']], 2, [], 1, ['running']])
        eq('still_running 的提示：只有转换句，没有文件夹操作', deleteFailureNotice(r.failures), { text: '有 1 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。', path: '' })
        mock.resetConvertMock('mixed')
        win.location.search = '?cv_delfail=in_use,still_running'
        const r3 = await mock.DeleteSource('mock-src-launch', true)
        const n3 = deleteFailureNotice(r3.failures)
        eq('两种失败都有：文件句在前、转换句在后，各自计数；文件夹取第一条非空 path', [n3.text, !!n3.path, n3.path === r3.failures.find((f) => f.path)?.path], ['有 1 个文件没能删除，可能正在被其他程序使用，请关闭后手动删除。有 1 条转换没能及时停止，它们的输出文件没有删除，请稍后手动删除。', true, true])
        // v0.23.3：打开所在文件夹失败——移走了 NOT_FOUND →“找不到这个文件。”；超过 10 分钟 / 重启 INVALID_ARGUMENT → 普通错误提示
        win.location.search = '?cv_revealfail=NOT_FOUND'
        const nf = await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))
        win.location.search = '?cv_revealfail=INVALID_ARGUMENT'
        const ia = await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))
        win.location.search = prev
        eq('打开所在文件夹失败的提示：NOT_FOUND 固定文案，其他用错误本身的文案；正常不报错', [nf && revealDeleteFailureText(nf), ia && revealDeleteFailureText(ia), await rejects(mock.revealDeleteFailureMock('D:\\x.mp4'))], ['找不到这个文件。', '不允许打开这个位置', null])
        eq('页面：打开所在文件夹失败走 revealDeleteFailureText，不再静默忽略 INVALID_ARGUMENT', [/revealDeleteFailure\(path\)\.catch\(\(e\) => ElMessage\.error\(revealDeleteFailureText\(toAppError\(e\)\)\)\)/.test(readSrc('src/views/ConvertPage.vue')), /INVALID_ARGUMENT/.test(readSrc('src/views/ConvertPage.vue'))], [true, false])
        mock.resetConvertMock('mixed')
        win.location.search = prev
        const r2 = await mock.DeleteSource('mock-src-launch', false)
        eq('不注入：进行中的先取消再删，行一起删掉', [r2.failures.length, r2.deletedSourceIds], [0, ['mock-src-launch']])
      } finally {
        win.location.search = prev
      }
    }
    // v0.23.2：SearchSources 的 status 与 ListSources 同语义
    mock.resetConvertMock('mixed')
    const sr = async (keyword: string, status: '' | 'active' | 'failed') => (await mock.SearchSources({ keyword, limit: 50, offset: 0, recordLimit: 20, status })).items.map((e) => e.source.sourceId.replace('mock-src-', ''))
    eq('SearchSources status：关键字 + 失败 / 进行中 / 全部', [await sr('mov', ''), await sr('mov', 'failed'), await sr('mov', 'active'), await sr('采访', 'failed')], [['launch', 'wed'], ['launch'], ['launch'], []])
    eq('SearchSources status 其它值 → INVALID_ARGUMENT', (await rejects(mock.SearchSources({ keyword: 'a', limit: 50, offset: 0, recordLimit: 20, status: 'x' as never })))?.code, 'INVALID_ARGUMENT')
  }
  // ---- 原地重试（含已取消）、隐藏 / 取消隐藏 ----
  mock.resetConvertMock('canceled')
  {
    const seen: { id: string; retried?: boolean; status: string }[] = []
    const off = onSimEvent<{ id: string; retried?: boolean; status: string }>('task:status', (p) => { if (p.id === 'simcv-launchCx') seen.push(p) })
    const t = retrySimTask('simcv-launchCx')
    eq('已取消的转换原地重试：同一个 id、回到排队、进度清零、发 task:status retried:true', [t.id, t.status, t.progress, seen[0]?.retried, seen[0]?.status], ['simcv-launchCx', 'queued', 0, true, 'queued'])
    off()
    if (getSimTask('simcv-launchCx')) cancelSimTask('simcv-launchCx')
    const visibleBefore = listSimFinished(false).map((x) => x.id)
    const n = hideSimFinished()
    eq('隐藏已结束：已结束的都隐藏，记录仍在（GetSource 照常返回）', [n > 0, listSimFinished(false).some((x) => x.id === 'simcv-launchWebmOk'), (await mock.GetSource('mock-src-launch')).records.some((r) => r.id === 'simcv-launchWebmOk')], [true, false, true])
    {
      setActivePinia(createPinia())
      const ts = useTaskStore()
      await ts.loadStats()
      const off3 = [ts.finishedTotal, ts.failedTotal, ts.todayDone]
      ts.historyFilter.includeHidden = true
      await ts.loadStats()
      const on3 = [ts.finishedTotal, ts.failedTotal, ts.todayDone]
      ts.historyFilter.includeHidden = false
      eq('任务中心计数：隐藏后页签计数不含已隐藏；“显示已隐藏”打开时含（用 total）；今日完成不受开关影响', [off3[0] < on3[0], on3[0] >= listSimFinished(true).filter((x) => x.type === 'convert').length, off3[2] === on3[2], on3[2] > 0], [true, true, true, true])
    }
    const un: { id: string; hiddenInTaskCenter?: boolean }[] = []
    const off2 = onSimEvent<{ id: string; hiddenInTaskCenter?: boolean }>('task:status', (p) => un.push(p))
    unhideSimTasks(['simcv-launchWebmOk'])
    off2()
    eq('取消隐藏：发 task:status hiddenInTaskCenter:false，回到默认列表', [un.length, un[0]?.hiddenInTaskCenter, listSimFinished(false).some((x) => x.id === 'simcv-launchWebmOk')], [1, false, true])
    unhideSimTasks(visibleBefore) // 其它自检的模拟任务恢复原样
    eq('UnhideInTaskCenter：未知 id → NOT_FOUND reason=record', (await rejects(mock.UnhideInTaskCenter(['nope'])))?.reason, 'record')
  }
  mock.resetConvertMock('mixed')
  // ---- 源码约定 ----
  const tc = readSrc('src/views/TaskCenter.vue')
  const page = readSrc('src/views/ConvertPage.vue')
  const panel = readSrc('src/components/convert/ConvertSettingsPanel.vue')
  const del = readSrc('src/components/convert/ConvertDeleteDialog.vue')
  const row = readSrc('src/components/convert/ConvertSourceRow.vue')
  const kid = readSrc('src/components/convert/ConvertKid.vue')
  const pv = readSrc('src/components/convert/ConvertPreviewDialog.vue')
  eq('任务中心：按钮“隐藏已结束”+ 说明 tooltip；“显示已隐藏”每次进入默认关', [/HIDE_FINISHED_LABEL = '隐藏已结束'/.test(tc), tc.includes('只从任务中心隐藏，转换记录仍保留在转换页'), /tasks\.historyFilter\.includeHidden = false/.test(tc), /localStorage[^\n]*includeHidden|includeHidden[^\n]*localStorage/.test(tc)], [true, true, true, false])
  eq('任务中心：转换行不给“移除”，给“在转换页查看”（先 GetSource，已删时提示）', [tc.includes('转换记录请在转换页删除'), /getSource\(t\.sourceId/.test(tc), tc.includes('这条转换记录已被删除')], [true, true, true])
  eq('任务中心转换行第二行用共用格式化函数', /recordParamsText\(/.test(tc), true)
  eq('转换页：没有“再转一个”/ startOver，没有 Reconvert 入口', [/再转一个|startOver/.test(page + panel + row + kid + pv), /\breconvert\(/.test(page + panel + row + kid + pv)], [false, false])
  eq('转换页：“转换 N 个文件”按钮', /转换 \$\{[^}]+\} 个文件/.test(panel), true)
  eq('删除确认：只删记录；勾选“同时删除输出文件”才提示无法恢复', [/只删除记录，(<b>)?不删除磁盘上的文件/.test(del), del.includes('同时删除输出文件'), del.includes('删除后无法恢复。')], [true, true, true])
  {
    const st = readSrc('src/stores/convertRecords.ts')
    eq('源文件行删除定稿：标题 / 按钮提示 / 菜单项“从列表移除”；没有旧说法', [/SOURCE_REMOVE_TITLE = '从列表移除这个文件和它的全部记录'/.test(st), /SOURCE_REMOVE_LABEL = '从列表移除'/.test(st), /:aria-label="SOURCE_REMOVE_LABEL" :title="SOURCE_REMOVE_LABEL"/.test(row), /\{\{ SOURCE_REMOVE_LABEL \}\}<\/button>/.test(row), /删除源文件和全部记录/.test(row + del + st + page)], [true, true, true, true, false])
    eq('源文件行确认按钮：普通“移除”，勾选后红色“移除并删除文件”', [/withOutput\.value \? '移除并删除文件' : '移除'/.test(del), /a\.value\?\.kind === 'record' \|\| withOutput\.value \? 'danger' : 'pri'/.test(del)], [true, true])
    eq('移除弹窗：标题下文件行（名称 · n 条转换记录）；“移除时会先取消它”；未勾选图标中性灰', [/class="cv-delfile"/.test(del) && /\{\{ a\.count \}\} 条转换记录/.test(del), /'移除' : '删除'\}时会先取消它/.test(del), /:class="\{ neutral: !danger \}"/.test(del)], [true, true, true])
    eq('toast：普通 4 秒、警告 8 秒；有失败时带“打开所在文件夹”（RevealInFolder(failures[0].path)）', [/TOAST_MS = 4000/.test(page), /WARN_TOAST_MS = 8000/.test(page), page.includes('打开所在文件夹'), /revealInFolder\(path\)/.test(readSrc('src/api/convertRecords.ts'))], [true, true, true, true])
  }
  eq('三处第 2 行都走 recordLine / recordParamsText', [/recordLine\(/.test(kid), /recordLine\(/.test(pv)], [true, true])
  eq('预览：快捷键空格 / ←→ 5 秒 / F / Esc；token 404 重新取一次地址', [/' '|'Space'/.test(pv), /ArrowLeft/.test(pv) && /ArrowRight/.test(pv), /'f'|'F'/.test(pv), /Escape/.test(pv), /404/.test(pv)], [true, true, true, true, true])
  eq('记录用 RevealRecord（不再用 RevealInFolder(path)）', [/revealRecord\(/.test(readSrc('src/stores/convertRecords.ts')), /RevealInFolder|revealInFolder/.test(readSrc('src/api/convertRecordsBinding.ts'))], [true, false])
  eq('第 2 行 / 预览页脚只用快照 presetName，不读当前预设卡片', [/presetLabel|presetTitle/.test(kid + row + pv)], [false])
  eq('失败卡片：ErrorLine 卡片版（操作左、错误码右，同一行）', [(kid.match(/actions-row/g) ?? []).length, /class="arow-line"/.test(readSrc('src/components/common/ErrorLine.vue'))], [2, true])
  eq('任务中心：固定列宽，操作列按内容估算，任务名优先；窄窗口“演示”只在 title', [/\.tbl \{ table-layout: fixed; \}/.test(tc), /'--ops-w': opsWidth \+ 'px'/.test(tc), /\.simtag \{ display: none; \}/.test(tc)], [true, true, true])
  eq('任务中心：页签计数跟“显示已隐藏”（failedTotal / finishedTotal），失败卡片用 failedCard', [/tasks\.failedCard/.test(tc), /void loadStats\(\) \/\/ 页签计数跟着开关/.test(readSrc('src/stores/tasks.ts'))], [true, true])
  let oldGone = true
  try { readSrc('src/stores/convert.ts'); oldGone = false } catch { /* 已删除 */ }
  try { readSrc('src/components/convert/ConvertFileRow.vue'); oldGone = false } catch { /* 已删除 */ }
  eq('旧转换页 store / 行组件已删除', oldGone, true)
}
