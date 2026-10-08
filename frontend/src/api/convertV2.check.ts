// 转换页 v2（契约 v0.23 §6.14 + v0.23.1）自检：由 api.check.ts 调用。只测数据层 / 模拟层 / 纯函数和几处源码约定，不渲染组件。
import { toAppError, type AppError } from './call'
import { recordOf, parseParams } from './convertRecords'
import * as mock from './convertRecordsMock'
import { cancelSimTask, getSimTask, hideSimFinished, listSimFinished, retrySimTask, unhideSimTasks } from './sim'
import { onSimEvent } from '@/services/wails'
import { recordParamsText, recordLine, deleteResultText, conflictReason, CONFLICT_NO_AUDIO, CONFLICT_NO_VIDEO, CONFLICT_TITLE, formatRecordTime } from '@/utils/convertText'

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
  eq('删除结果：按原因汇总', deleteResultText({ deletedTaskIds: ['a', 'b'], failures: [{ reason: 'in_use', message: '' }, { reason: 'in_use', message: '' }, { reason: 'permission', message: '' }] }), '已删除 2 条记录。有 2 个文件正在被使用，没有删除。有 1 个文件没有权限删除。')
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
  eq('三处第 2 行都走 recordLine / recordParamsText', [/recordLine\(/.test(kid), /recordLine\(/.test(pv)], [true, true])
  eq('预览：快捷键空格 / ←→ 5 秒 / F / Esc；token 404 重新取一次地址', [/' '|'Space'/.test(pv), /ArrowLeft/.test(pv) && /ArrowRight/.test(pv), /'f'|'F'/.test(pv), /Escape/.test(pv), /404/.test(pv)], [true, true, true, true, true])
  eq('记录用 RevealRecord（不再用 RevealInFolder(path)）', [/revealRecord\(/.test(readSrc('src/stores/convertRecords.ts')), /RevealInFolder|revealInFolder/.test(readSrc('src/api/convertRecordsBinding.ts'))], [true, false])
  let oldGone = true
  try { readSrc('src/stores/convert.ts'); oldGone = false } catch { /* 已删除 */ }
  try { readSrc('src/components/convert/ConvertFileRow.vue'); oldGone = false } catch { /* 已删除 */ }
  eq('旧转换页 store / 行组件已删除', oldGone, true)
}
