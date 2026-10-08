// 包 20 直播表单（stores/liveForms.ts）+ 推流码遮挡自检：由 api.check.ts 调用。node 里不能挂载组件：
// “页面卸载 / 重新挂载”= 组件销毁后重新 useLiveFormsStore()（同一个 pinia）；“下次启动”= 新 pinia + 同一个 localStorage。
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'
import { LIVE_FORMS_KEY, LIVE_FORMS_SAVE_DELAY, defaultLiveForms, parseLiveForms, restoreSourceId, serializeLiveForms, useLiveFormsStore } from './liveForms'
import { SECRET_DOTS, hasPushUrlSecret, maskPushUrlSecret } from '@/utils/liveUrl'

type Eq = (name: string, got: unknown, want: unknown) => void
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

export async function liveFormsChecks(eq: Eq, readSrc: (f: string) => string): Promise<void> {
  const g = globalThis as unknown as { localStorage?: Storage }
  const prevLs = g.localStorage
  const mem = new Map<string, string>()
  let writes = 0
  g.localStorage = {
    getItem: (k: string) => (mem.has(k) ? mem.get(k)! : null),
    setItem: (k: string, v: string) => {
      writes++
      mem.set(k, String(v))
    },
  } as Storage
  try {
    // 1. 切换菜单（页面卸载再挂载）：值原样还在
    setActivePinia(createPinia())
    const a = useLiveFormsStore()
    eq('LF 初始：没有存档 → 默认值', { file: a.file, screen: a.screen, pull: a.pull }, defaultLiveForms())
    a.file.material = { name: '发布会.mp4', path: '/m/发布会.mp4', duration: '00:01:00' }
    a.file.baseUrl = 'rtmp://push.example.com/live'
    a.file.key = 'fileKEY123'
    a.screen.sourceId = 'window:42'
    a.screen.baseUrl = 'srt://srt.example.com:9000'
    a.screen.key = 'passphrase-123'
    a.screen.archiveOn = true
    a.screen.archiveDir = '/Users/me/存档'
    a.pull.url = 'http://pull.example.com/live/room.flv'
    a.pull.lowLatency = false
    a.pull.muted = true
    const b = useLiveFormsStore() // 页面重新挂载
    eq('LF 卸载再挂载：同一份表单，所有字段原样', [b === a, b.file.baseUrl, b.file.key, b.file.material?.path, b.screen.sourceId, b.screen.archiveOn, b.screen.archiveDir, b.pull.url, b.pull.lowLatency, b.pull.muted],
      [true, 'rtmp://push.example.com/live', 'fileKEY123', '/m/发布会.mp4', 'window:42', true, '/Users/me/存档', 'http://pull.example.com/live/room.flv', false, true])

    // 2. 防抖写盘：连续修改只写一次；写入的是版本化 key
    await nextTick()
    writes = 0
    a.file.baseUrl = 'rtmp://push.example.com/live2'
    await nextTick()
    a.file.baseUrl = 'rtmp://push.example.com/live3'
    await nextTick()
    eq('LF 防抖：修改后还没到时间不写', writes, 0)
    await sleep(LIVE_FORMS_SAVE_DELAY + 80)
    eq('LF 防抖：到时间只写一次，key = ffmpegfree.live.forms.v1', [writes, LIVE_FORMS_KEY, mem.has(LIVE_FORMS_KEY)], [1, 'ffmpegfree.live.forms.v1', true])

    // 3. 下次启动（新 pinia，同一个存储）：全部恢复
    a.flush()
    setActivePinia(createPinia())
    const c = useLiveFormsStore()
    eq('LF 重启恢复：三张表单全部恢复', { file: c.file, screen: c.screen, pull: c.pull }, {
      file: { material: { path: '/m/发布会.mp4', name: '发布会.mp4', duration: '00:01:00' }, baseUrl: 'rtmp://push.example.com/live3', key: 'fileKEY123' },
      screen: { sourceId: 'window:42', baseUrl: 'srt://srt.example.com:9000', key: 'passphrase-123', archiveOn: true, archiveDir: '/Users/me/存档' },
      pull: { url: 'http://pull.example.com/live/room.flv', lowLatency: false, muted: true },
    })
    eq('LF 序列化往返', parseLiveForms(serializeLiveForms({ file: c.file, screen: c.screen, pull: c.pull })), { file: { ...c.file }, screen: { ...c.screen }, pull: { ...c.pull } })

    // 4. 恢复校验：素材文件不在了 → 只清素材，其余保留；只核对一次
    let probes = 0
    await c.validateRestoredMaterial(async () => (probes++, false))
    eq('LF 恢复校验：文件不在 → 素材回到未选，地址 / 推流码保留', [c.file.material, c.file.baseUrl, c.file.key, c.screen.key], [null, 'rtmp://push.example.com/live3', 'fileKEY123', 'passphrase-123'])
    await c.validateRestoredMaterial(async () => (probes++, false))
    eq('LF 恢复校验：每次启动只核对一次', probes, 1)
    setActivePinia(createPinia())
    mem.set(LIVE_FORMS_KEY, serializeLiveForms({ ...defaultLiveForms(), file: { material: { name: 'a.mp4', path: '/m/a.mp4', duration: '' }, baseUrl: 'rtmp://h/live', key: 'k' } }))
    const d = useLiveFormsStore()
    await d.validateRestoredMaterial(async () => true)
    eq('LF 恢复校验：文件还在 → 保留', d.file.material?.path, '/m/a.mp4')
    setActivePinia(createPinia())
    const e = useLiveFormsStore()
    await e.validateRestoredMaterial(async () => {
      throw new Error('probe 失败')
    })
    eq('LF 恢复校验：核对出错按不在处理', [e.file.material, e.file.baseUrl], [null, 'rtmp://h/live'])
    setActivePinia(createPinia())
    const f = useLiveFormsStore()
    const picked = { name: 'new.mp4', path: '/m/new.mp4', duration: '' }
    await f.validateRestoredMaterial(async () => {
      f.file.material = picked // 核对期间用户换了文件
      return false
    })
    eq('LF 恢复校验：核对期间用户换了文件 → 不动用户的新选择', f.file.material?.path, '/m/new.mp4')

    // 5. 录屏来源：保存的还在就沿用；不在了退回第一个屏幕；列表空 → ''
    const list = [{ id: 'window:7', kind: 'window' as const }, { id: 'screen:0', kind: 'screen' as const }, { id: 'screen:1', kind: 'screen' as const }]
    eq('LF 来源恢复：还在 / 不在了 / 没存 / 只有窗口 / 空列表', [restoreSourceId(list, 'screen:1'), restoreSourceId(list, 'window:42'), restoreSourceId(list, ''), restoreSourceId([{ id: 'window:1', kind: 'window' }], 'x'), restoreSourceId([], 'screen:0')], ['screen:1', 'screen:0', 'screen:0', 'window:1', ''])

    // 6. 坏存档：JSON 坏 / 版本不对 → 全默认；单字段类型不对 → 只退这个字段
    eq('LF 坏存档：JSON 坏 / 版本不对 / 空', [parseLiveForms('{oops'), parseLiveForms(JSON.stringify({ v: 2, file: { baseUrl: 'x' } })), parseLiveForms(null)], [defaultLiveForms(), defaultLiveForms(), defaultLiveForms()])
    const partial = parseLiveForms(JSON.stringify({ v: 1, file: { baseUrl: 42, key: 'k1', material: { path: '' } }, screen: { archiveOn: 'yes', archiveDir: '/d' }, pull: { url: 'http://h/a.flv', lowLatency: 'no' } }))
    eq('LF 坏字段：只退回该字段，其余保留', [partial.file.baseUrl, partial.file.key, partial.file.material, partial.screen.archiveOn, partial.screen.archiveDir, partial.pull.url, partial.pull.lowLatency], ['', 'k1', null, false, '/d', 'http://h/a.flv', true])

    // 7. 隐私 / 接线（源码）：store 不打 console、不发网络；三个页面都用 store，不再各自 ref('')；预览开关不持久化
    const src = readSrc('src/stores/liveForms.ts')
    eq('LF 隐私：store 里没有 console / fetch / XMLHttpRequest / WebSocket', /console\.|fetch\(|XMLHttpRequest|WebSocket/.test(src), false)
    const fp = readSrc('src/views/live/FilePush.vue')
    const rp = readSrc('src/views/live/RecordPush.vue')
    const pp = readSrc('src/views/live/PullPlay.vue')
    eq('LF 接线：文件推流 / 录屏推流 / 拉流的输入都来自 store', [/toRefs\(forms\.file\)/.test(fp), /toRefs\(forms\.screen\)/.test(rp), /toRefs\(forms\.pull\)/.test(pp)], [true, true, true])
    eq('LF 接线：页面里不再有本地的 baseUrl / key / url / sourceId ref', [/const (baseUrl|key|material) = ref/.test(fp), /const (baseUrl|key|sourceId|archiveOn|archiveDir) = ref/.test(rp), /const (url|lowLatency|muted) = ref/.test(pp)], [false, false, false])
    eq('LF 预览开关不进存档（产品经理：不记住上次选择）', /previewOn/.test(src.replace(/\/\/.*$/gm, '')), false)
    eq('LF 推流页：推流地址输入框开了推流码遮挡', [/v-model="baseUrl" mask-key/.test(fp), /v-model="baseUrl" mask-key/.test(rp)], [true, true])
  } finally {
    g.localStorage = prevLs
  }

  // 8. 推流码遮挡（只改显示）
  const D = SECRET_DOTS
  eq('遮挡 rtmp：服务器 + 应用名原样，流名遮挡', maskPushUrlSecret('rtmp://live-push.example.com/live/abcKEY123'), `rtmp://live-push.example.com/live/${D}`)
  eq('遮挡 rtmps 多段 + 查询参数', maskPushUrlSecret('rtmps://Host.Example:443/app/sub/KEY?auth=tok1&x='), `rtmps://Host.Example:443/app/${D}/${D}?auth=${D}&x=${D}`)
  eq('遮挡 srt：查询参数值遮挡，路径不动', maskPushUrlSecret('srt://srt.example.com:9000?streamid=sidX&passphrase=passY'), `srt://srt.example.com:9000?streamid=${D}&passphrase=${D}`)
  eq('遮挡 用户信息', maskPushUrlSecret('rtmp://user:pw@h.example/live'), `rtmp://••••@h.example/live`)
  eq('没有推流码 → 原样、不显示眼睛按钮', [maskPushUrlSecret('rtmp://h.example/live'), maskPushUrlSecret('rtmp://h.example/live/'), maskPushUrlSecret('rtmp://h'), maskPushUrlSecret('rtmp:/'), maskPushUrlSecret(''), hasPushUrlSecret('rtmp://h.example/live')], ['rtmp://h.example/live', 'rtmp://h.example/live/', 'rtmp://h', 'rtmp:/', '', false])
  const secrets = ['abcKEY123', 'tok1', 'sidX', 'passY', 'pw']
  const shown = [maskPushUrlSecret('rtmp://h/live/abcKEY123'), maskPushUrlSecret('rtmp://h/live?k=tok1'), maskPushUrlSecret('srt://h:1?streamid=sidX&passphrase=passY'), maskPushUrlSecret('rtmp://u:pw@h/live')]
  eq('遮挡结果里不含任何推流码 / 口令', shown.filter((t) => secrets.some((x) => t.includes(x))), [])
  const li = readSrc('src/components/live/LiveInput.vue')
  eq('遮挡按钮：aria-label 显示 / 隐藏推流码，aria-pressed；聚焦显示完整；切走页签重新遮挡', [/'隐藏推流码' : '显示推流码'/.test(li), /:aria-pressed="revealed"/.test(li), /secretPart\.value && !revealed\.value && !focused\.value \? maskPushUrlSecret/.test(li), /onDeactivated\(\(\) => \(revealed\.value = false\)\)/.test(li)], [true, true, true, true])
  eq('遮挡只改显示：输入框 :value 显示遮挡值，model 只在聚焦编辑时写回', [/:value="shown"/.test(li), /v-model="model"/.test(li)], [true, false])
}
