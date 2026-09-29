// 不引入测试框架的自检：npm run check:edit（scripts/check-edit.mjs 用 esbuild 打包后在 node 里跑）。
import { newAudioClip, newEditProject, newVideoClip, type VideoClip } from '@/api/edit'
import {
  DELETE_CONFIRM_MIN, MAX_CLIPS, MAX_TIMELINE_SEC, clipEnd, deleteNeedsConfirm, exceedsTimeline, exportErrorView, exportNameError, formatClock, formatTC, gapKind,
  maxTransitionSec, nameLength, nextTouching, normalizeWarnings, overlapWith, parseTC, pathTooLong, placeProblem, resolveSplitTarget, snapStart, splitBlockReason, splitClip,
  timelineDuration, warningText, TEXT, LIMIT_TEXT, MAX_SOURCES, clampTransitionInput, clampSilently, transitionFits, hasTransitionIgnored, isSilentWarning,
} from './editLogic'

export function runEditChecks(): string[] {
  const fails: string[] = []
  const eq = (name: string, got: unknown, want: unknown) => {
    const a = JSON.stringify(got)
    const b = JSON.stringify(want)
    if (a !== b) fails.push(`✗ ${name}: 得到 ${a}，期望 ${b}`)
  }
  const v = (id: string, start: number, len: number, track = 'V1', extra: Partial<VideoClip> = {}): VideoClip => ({
    ...newVideoClip({ id, path: `/m/${id}.mp4`, durationSec: 600, trackId: track, startSec: start }), inSec: 0, outSec: len, ...extra,
  })

  // 限制常量
  eq('片段上限 100', MAX_CLIPS, 100)
  eq('素材库上限先按 100', MAX_SOURCES, 100)
  eq('6 小时', MAX_TIMELINE_SEC, 21600)
  {
    const d = newEditProject('x').output
    eq('新建工程默认 1920×1080、30fps', [d.width, d.height, d.fps], [1920, 1080, 30])
    eq('转场输入超限：限制为上限并提示', clampTransitionInput(1.0, 0.8), { value: 0.8, over: true })
    eq('转场输入合法：不提示', clampTransitionInput(0.5, 0.8), { value: 0.5, over: false })
    eq('默认 0.5 超上限：静默缩短', clampSilently(0.5, 0.3), 0.3)
    eq('上限不足 0.1：放不下', [transitionFits(0.09), transitionFits(0.1)], [false, true])
    eq('transition_ignored 字符串与结构化都识别', [hasTransitionIgnored(normalizeWarnings(['transition_ignored'])), hasTransitionIgnored(normalizeWarnings([{ code: 'transition_ignored', message: 'x' }]))], [true, true])
    eq('clip_gap / leading_gap 不提示', normalizeWarnings(['clip_gap', { code: 'leading_gap', message: '' }]).map(isSilentWarning), [true, true])
    eq('间隙 0.12 相接、0.13 空隙', [gapKind(10, 10.12), gapKind(10, 10.13)], ['touch', 'gap'])
  }
  {
    const pj = { videoTrack: [v('cs', 0, 5)], audioTrack: [] }
    const ev = exportErrorView({ code: 'INVALID_ARGUMENT', message: '片段太短（按速度折算后不足 0.04 秒）', detail: 'clip=cs path=/m/cs.mp4\n片段太短' }, pj)
    eq('片段太短：固定文案、定位 + 日志、无重试', [ev.text, ev.actions, ev.clipId], ['片段太短，请调整后再导出。', ['locate', 'log'], 'cs'])
    eq('转场没生效文案', TEXT.transitionIgnored, '有片段太短，转场没有生效')
  }
  eq('限制小字', LIMIT_TEXT, '限制：视频轨 8 条、音频轨 8 条，片段 100 个，时间线 6 小时')
  eq('删除 5 个才确认', [deleteNeedsConfirm(4), deleteNeedsConfirm(DELETE_CONFIRM_MIN), deleteNeedsConfirm(9)], [false, true, true])

  // 新片段 outSec 必须是探测到的时长，不是 0
  const nv = newVideoClip({ path: '/a.mp4', durationSec: 84 })
  eq('newVideoClip outSec = 探测时长', [nv.inSec, nv.outSec, nv.speed], [0, 84, 1])
  eq('newAudioClip volume 显式 1', newAudioClip({ path: '/a.mp3', durationSec: 10 }).volume, 1)

  // 重叠：后一个开始早于前一个结束才算
  const a = v('a', 0, 10)
  eq('首尾相接不算重叠', overlapWith([a], v('b', 10, 5)), null)
  eq('差 1ms 内不算重叠', overlapWith([a], v('b', 9.9995, 5)), null)
  eq('重叠返回冲突片段', overlapWith([a], v('b', 9, 5)), 'a')
  eq('不同轨道不算重叠（画中画）', overlapWith([a], v('b', 5, 5, 'V2')), null)
  eq('忽略自己（拖动自己）', overlapWith([a], { ...a, startSec: 3 }), null)
  eq('速度变小导致变长撞到后一个', overlapWith([a, v('b', 12, 5)], { ...a, speed: 0.5 }), 'b')
  eq('速度变大变短不撞', overlapWith([a, v('b', 12, 5)], { ...a, speed: 2 }), null)

  // 0.12 秒间隙
  eq('间隙 0 = 相接', gapKind(10, 10), 'touch')
  eq('间隙 0.12 = 相接', gapKind(10, 10.12), 'touch')
  eq('间隙 0.121 = 空隙', gapKind(10, 10.121), 'gap')
  eq('间隙 1 = 空隙', gapKind(10, 11), 'gap')
  eq('开始早于结束 = 重叠', gapKind(10, 9.5), 'overlap')
  eq('nextTouching 相接', nextTouching([a, v('b', 10.1, 5)], a)?.id, 'b')
  eq('nextTouching 空隙没有转场', nextTouching([a, v('b', 10.5, 5)], a), null)
  eq('转场时长上限 = 较短者的一半', maxTransitionSec(v('x', 0, 3), v('y', 3, 8)), 1.5)
  eq('转场时长上限最多 2 秒', maxTransitionSec(v('x', 0, 30), v('y', 30, 30)), 2)

  // 时长与 6 小时
  const clips = [v('a', 0, 10), v('c', 20, 5, 'V1', { speed: 2 }), { ...newAudioClip({ path: '/b.mp3', durationSec: 100 }), id: 'au', outSec: 100 }]
  eq('时间线总长（音视频一起，速度换算）', timelineDuration(clips), 100)
  eq('clipEnd 含速度', clipEnd(v('s', 4, 10, 'V1', { speed: 2 })), 9)
  eq('放在 6 小时内', exceedsTimeline([], v('z', MAX_TIMELINE_SEC - 10, 10)), false)
  eq('超过 6 小时', exceedsTimeline([], v('z', MAX_TIMELINE_SEC - 10, 10.5)), true)
  eq('placeProblem 片段数已满优先', placeProblem(Array.from({ length: 100 }, (_, i) => v('k' + i, i * 10, 5, 'V' + ((i % 8) + 1))), v('n', 0, 1, 'V1'), { adding: true, kindOk: true }), 'count')
  eq('placeProblem 轨道类型不符', placeProblem([], v('n', 0, 1), { adding: true, kindOk: false }), 'track')
  eq('placeProblem 重叠', placeProblem([a], v('n', 5, 10), { adding: true, kindOk: true }), 'overlap')
  eq('placeProblem 超 6 小时', placeProblem([], v('n', MAX_TIMELINE_SEC, 1), { adding: true, kindOk: true }), 'timeline')
  eq('placeProblem 可放', placeProblem([a], v('n', 10, 5), { adding: true, kindOk: true }), null)

  // 切割
  const s = v('s', 10, 20, 'V1', { inSec: 5, outSec: 45, speed: 2, transitionToNext: 'fade', transitionDurationSec: 1 }) // 时间线 10~30，素材 5~45
  const parts = splitClip(s, 20, ['s1', 's2'])!
  eq('切割：前一个 outSec = 切点（素材时刻）', [parts[0].inSec, parts[0].outSec, parts[0].startSec], [5, 25, 10])
  eq('切割：后一个 inSec = 切点，startSec 顺延', [parts[1].inSec, parts[1].outSec, parts[1].startSec], [25, 45, 20])
  eq('切割：两段首尾连续', clipEnd(parts[0]), parts[1].startSec)
  eq('切割：总长不变', clipEnd(parts[1]), clipEnd(s))
  eq('切割：转场留在后一个', [parts[0].transitionToNext, parts[1].transitionToNext], ['none', 'fade'])
  eq('切割：新 id', [parts[0].id, parts[1].id], ['s1', 's2'])
  eq('切割：离起点 < 0.1 秒不行', splitClip(s, 10.05, ['x', 'y']), null)
  eq('切割：离终点 < 0.1 秒不行', splitClip(s, 29.95, ['x', 'y']), null)
  eq('切割：正好 0.1 秒可以', splitClip(s, 10.1, ['x', 'y']) !== null, true)
  eq('切割原因：没有片段', splitBlockReason([], null, 0), TEXT.noClips)
  eq('切割原因：没选中且播放头下两个片段', splitBlockReason([v('p', 0, 10), v('q', 0, 10, 'V2')], null, 5), TEXT.splitNeedSelect)
  eq('切割：没选中但播放头下只有一个片段', resolveSplitTarget([v('p', 0, 10), v('q', 20, 10, 'V2')], null, 5)?.id, 'p')
  eq('切割：选中但播放头不在片段内', splitBlockReason([v('p', 0, 10)], 'p', 15), TEXT.splitNeedSelect)
  eq('切割：可用', splitBlockReason([v('p', 0, 10)], 'p', 5), null)
  eq('切割原因：已满 100', splitBlockReason(Array.from({ length: 100 }, (_, i) => v('k' + i, i * 10, 5, 'V' + ((i % 8) + 1))), 'k0', 2), TEXT.splitFull)

  // 吸附
  eq('吸附到相邻片段终点', snapStart(10.05, 5, [0, 10], 0.1), { start: 10, at: 10 })
  eq('吸附：终点靠近别的起点', snapStart(4.95, 5, [10], 0.1), { start: 5, at: 10 })
  eq('吸附：超出阈值不吸', snapStart(10.5, 5, [10], 0.1), { start: 10.5, at: null })
  eq('吸附：不会吸到负数', snapStart(0.05, 5, [-0.1], 0.2).start >= 0, true)

  // 时间码
  eq('formatTC 帧', formatTC(12.267), '00:12.08')
  eq('formatTC 进位', formatTC(59.999), '01:00.00')
  eq('formatTC 小时', formatTC(3725.5), '01:02:05.15')
  eq('formatClock', [formatClock(52), formatClock(21600, true), formatClock(3725)], ['00:52', '06:00:00', '01:02:05'])
  eq('parseTC', [parseTC('12'), parseTC('1:02.5'), parseTC('01:02:03'), parseTC('abc'), parseTC('')], [12, 62.5, 3723, null, null])

  // 文件名长度（按字符，不是字节）
  const long = '我的旅行短片'.repeat(17).slice(0, 101)
  eq('101 个字符', nameLength(long), 101)
  eq('101/100 报错', exportNameError(long, '/Users/me', 'mp4', false)?.text, '文件名或保存位置的路径太长，请缩短')
  eq('100 个字符可以', exportNameError(long.slice(0, 100), '/Users/me', 'mp4', false), null)
  eq('emoji 按一个字符算', nameLength('😀'.repeat(100)), 100)
  eq('Windows 整条路径 > 259', pathTooLong('C:\\' + 'a'.repeat(200), 'b'.repeat(50), 'mp4', true), true)
  eq('非 Windows 不算路径长度', pathTooLong('/' + 'a'.repeat(300), 'b', 'mp4', false), false)
  eq('Windows 路径错误标红保存位置', exportNameError('x', 'C:\\' + 'a'.repeat(300), 'mp4', true)?.path, true)

  // 校验提示：按 code 出文案，未知 code 用通用文案
  const ws = normalizeWarnings([{ code: 'OUT_TRUNCATED', clipId: 'c3', message: 'x' }, { code: 'FUTURE_CODE', message: '后端新增' }, 'clip k1 outSec 超过素材时长，已截断', '别的话'])
  eq('warnings 结构', ws.map((w) => [w.code, w.clipId]), [['OUT_TRUNCATED', 'c3'], ['FUTURE_CODE', undefined], ['OUT_TRUNCATED', 'k1'], ['UNKNOWN', undefined]])
  eq('已知 code 文案', warningText(ws[0], '片段 3'), '片段 3 的出点超过素材时长，导出时会截到素材结尾。')
  eq('未知 code 通用文案', warningText(ws[1]), TEXT.warningGeneric)

  // 导出错误定位
  const proj = { videoTrack: [v('c1', 0, 8), v('c2', 8, 8), v('c3', 16, 8)], audioTrack: [] }
  const e1 = exportErrorView({ code: 'INVALID_ARGUMENT', message: '入点超出素材时长。', detail: 'clip=c3 path=/m/c3.mp4\ninSec 超出' }, proj)
  eq('定位：片段 N 出错', [e1.title, e1.text, e1.clipId, e1.actions], ['导出失败', '片段 3 出错：入点超出素材时长。', 'c3', ['locate', 'retry', 'log']])
  const e2 = exportErrorView({ code: 'INVALID_ARGUMENT', message: 'm', detail: 'clip=zzz path=/x' }, proj)
  eq('定位：id 找不到不拼、没有定位', [e2.text, e2.clipId, e2.actions], ['m', null, ['retry', 'log']])
  eq('定位：project 不拼', exportErrorView({ code: 'INVALID_ARGUMENT', message: 'm', detail: 'project\n视频轨不能为空' }, proj).clipId, null)
  const multi = { videoTrack: [v('c1', 0, 8), v('c9', 0, 8, 'V2')], audioTrack: [{ ...newAudioClip({ path: '/a.mp3', durationSec: 9, id: 'a1' }) }] }
  eq('定位：多条有片段的轨道写轨名', exportErrorView({ code: 'PROCESS_FAILED', message: 'm', detail: 'clip=c9 path=/x' }, multi).text, 'V2 片段 1 出错：m')
  const e3 = exportErrorView({ code: 'CONVERT_DISK_FULL', message: 'x', detail: 'clip=c1 path=/x' }, proj)
  eq('磁盘不足：专门标题、更换输出位置', [e3.title, e3.actions], ['磁盘空间不足', ['retry', 'changeOutput', 'log']])
  return fails
}
