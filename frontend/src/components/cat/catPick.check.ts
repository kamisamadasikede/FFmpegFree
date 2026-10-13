import { composePickSend, formatPick, newPickToken, pickLabel, placePickPop, PICK_SOURCE, pickRuntime, readPickEvent, withPickScript, type PickNote, type PickSelect } from './catPick'

type Eq = (name: string, got: unknown, want: unknown) => void

const sample: PickSelect = {
  kind: 'select',
  tag: 'button',
  id: 'save',
  classes: 'primary',
  selector: 'main > button#save',
  text: '保存',
  styles: 'color: rgb(255, 255, 255)',
  snippet: '<button id="save">保存</button>',
  box: { left: 12, top: 24, width: 80, height: 28 },
}

export async function catPickChecks(eq: Eq): Promise<void> {
  const token = 'abc123'
  const html = withPickScript('<html><body><p>你好</p></body></html>', token)
  eq('点选脚本插在 body 结束前', html.includes('<p>你好</p><script data-cat-pick="1">') && html.endsWith('</body></html>'), true)
  eq('没有 body 时脚本接在末尾', withPickScript('<p>你好</p>', token).endsWith('</scr' + 'ipt>'), true)
  eq('坏的口令不注入', withPickScript('<p></p>', '</script>'), '<p></p>')
  const runtime = pickRuntime(token)
  eq('点选脚本能通过语法检查', (() => { new Function(runtime); return true })(), true)
  eq('点选脚本自身不会提前结束', runtime.includes('</'), false)
  eq('点选脚本带上这次的口令', runtime.includes('"abc123"'), true)

  eq('不认别的口令', readPickEvent({ source: 'ffmpegfree-cat-pick', token: 'nope', kind: 'ready' }, token), null)
  eq('不认别的来源', readPickEvent({ source: 'other', token, kind: 'ready' }, token), null)
  eq('页面准备好', readPickEvent({ source: 'ffmpegfree-cat-pick', token, kind: 'ready' }, token), { kind: 'ready' })
  eq('退出点选', readPickEvent({ source: 'ffmpegfree-cat-pick', token, kind: 'esc' }, token), { kind: 'esc' })
  const picked = readPickEvent({
    source: 'ffmpegfree-cat-pick',
    token,
    kind: 'select',
    tag: 'BUTTON',
    id: 'save',
    classes: 'primary extra',
    selector: 'main > button',
    text: '保存',
    styles: 'color: red',
    snippet: '<button>保存</button>',
  }, token) as PickSelect
  eq('元素说明收下标签', picked.tag, 'button')
  eq('过长片段会被截断', (readPickEvent({ source: 'ffmpegfree-cat-pick', token, kind: 'select', tag: 'div', snippet: 'x'.repeat(800) }, token) as PickSelect).snippet.length < 700, true)
  eq('没有标签就不收', readPickEvent({ source: 'ffmpegfree-cat-pick', token, kind: 'select', tag: '!!!' }, token), null)

  const note: PickNote = { ...sample, file: 'web/index.html', intent: 'edit', note: '改成绿色' }
  const block = formatPick(note)
  eq('说明里有文件、元素和意图', block.includes('文件：web/index.html') && block.includes('元素：button#save.primary') && block.includes('意图：改变') && block.includes('说明：改成绿色'), true)
  eq('疑问会写进发送内容', formatPick({ ...note, intent: 'ask', note: '这是什么' }).includes('意图：疑问') && formatPick({ ...note, intent: 'ask', note: '这是什么' }).includes('说明：这是什么'), true)
  eq('空输入框只放说明', composePickSend('', [note]).startsWith('--- 选中 ---'), true)
  eq('草稿本身不被改写', composePickSend('改成绿色', []), '改成绿色')
  eq('发送时说明在前，草稿在后', composePickSend('再问一句', [note]).endsWith('再问一句\n') && composePickSend('再问一句', [note]).includes('说明：改成绿色'), true)
  eq('两段说明都会带上', composePickSend('好', [note, { ...note, text: '取消', note: '换成取消' }]).includes('文字：保存') && composePickSend('好', [note, { ...note, text: '取消', note: '换成取消' }]).includes('文字：取消'), true)
  eq('标题优先用元素文字', pickLabel({ tag: 'div', id: '', classes: 'a', text: '视频' }), '视频')
  eq('没有文字时用标签', pickLabel({ tag: 'button', id: 'save', classes: 'primary', text: '' }), 'button#save.primary')
  eq('弹窗优先放在元素右边', placePickPop({ left: 20, top: 30, width: 40, height: 16 }, { width: 200, height: 120 }, { width: 800, height: 600 }), { left: 68, top: 30 })
  eq('靠右时弹窗改到左边', placePickPop({ left: 700, top: 30, width: 80, height: 16 }, { width: 200, height: 120 }, { width: 800, height: 600 }).left < 700, true)
  eq('靠下时弹窗上移', placePickPop({ left: 20, top: 560, width: 40, height: 20 }, { width: 200, height: 120 }, { width: 800, height: 600 }).top <= 472, true)
  eq('每次口令都不空', newPickToken().length > 8, true)

  const mk = (globalThis as unknown as { __makeDomWindow?: () => Promise<unknown> }).__makeDomWindow
  if (!mk) return
  const win = await mk() as {
    document: Document
    addEventListener: (type: string, fn: (e: MessageEvent) => void) => void
    eval: (code: string) => unknown
    dispatchEvent: (ev: Event) => boolean
    MessageEvent: new (type: string, init: { data: unknown; source: unknown }) => Event
    MouseEvent: new (type: string, init?: MouseEventInit) => Event
    PointerEvent?: new (type: string, init?: MouseEventInit) => Event
  }
  let inPage = false
  try {
    inPage = win.eval('typeof document === "object"') === true
  } catch {
    inPage = false
  }
  if (!inPage) return
  const pageToken = 'abc123'
  const messages: unknown[] = []
  win.addEventListener('message', (e: MessageEvent) => { messages.push(e.data) })
  win.document.body.innerHTML = '<main><button id="save" class="primary">保存</button></main>'
  win.eval(pickRuntime(pageToken))
  await new Promise((r) => setTimeout(r, 30))
  eq('页面准备好会发出来', messages.some((m) => readPickEvent(m, pageToken)?.kind === 'ready'), true)
  win.dispatchEvent(new win.MessageEvent('message', {
    data: { source: PICK_SOURCE, token: pageToken, kind: 'arm', on: true },
    source: win,
  }))
  const btn = win.document.getElementById('save')
  const Ev = win.PointerEvent ?? win.MouseEvent
  btn?.dispatchEvent(new Ev('pointerdown', { bubbles: true, cancelable: true, button: 0 }))
  await new Promise((r) => setTimeout(r, 30))
  const selected = messages.map((m) => readPickEvent(m, pageToken)).find((m) => m?.kind === 'select') as PickSelect | undefined
  eq('点中的按钮会带上标签和路径', [selected?.tag, selected?.id, selected?.text, selected?.selector], ['button', 'save', '保存', 'button#save'])
  eq('点中的按钮带上位置', typeof selected?.box?.width === 'number' && typeof selected?.box?.height === 'number', true)
  const overlays = () => Array.from(win.document.querySelectorAll('[data-cat-pick]')).filter((n) => (n as HTMLElement).style.display === 'block')
  eq('点中后有选中框', overlays().length > 0, true)
  win.dispatchEvent(new win.MessageEvent('message', {
    data: { source: PICK_SOURCE, token: pageToken, kind: 'clear' },
    source: win,
  }))
  eq('添加后选中框收起', overlays().length, 0)
}
