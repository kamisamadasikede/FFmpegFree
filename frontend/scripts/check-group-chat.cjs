// Browser interaction acceptance for the real Vue components using isolated fixtures.
// Run with the Vite dev server: node frontend/scripts/check-group-chat.cjs
const fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict')
const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
const root = path.resolve(__dirname, '../..'), output = path.join(root, '.gocache/group-qa')
fs.mkdirSync(output, { recursive: true })
;(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  const errors = []
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 960 } })
    page.on('pageerror', e => errors.push(e.message))
    await page.goto((process.env.FFC_FRONTEND_URL || 'http://127.0.0.1:5173') + '/?cat_group_scene=report#/cat')
    await page.locator('[data-testid="cat-group-view"]').waitFor()
    await page.evaluate(async () => {
      const { useCatGroupChat } = await import('/src/stores/catGroupChat.ts')
      window.groupQA = useCatGroupChat()
      window.groupCalls = []
      const s = window.groupQA
      s.group.name = '项目协作'
      s.hasMore = false
      s.membersOpen = false
      const now = Date.now()
      s.workflows = [{ chainId: 'qa', groupId: 'g1', originSeq: 1, request: '为任务列表增加筛选功能，并完成前后端联调和验收。', status: 'running', revision: 3, updatedAt: now, tasks: [
        { id: 'api', title: '筛选接口', memberId: 'b', instruction: 'internal instructions', kind: 'develop', status: 'done', dependsOn: [], attempts: 1, result: '筛选接口已完成。', evidence: 'node --test internal-check.cjs', turnId: 'api-turn' },
        { id: 'page', title: '筛选页面', memberId: 'f', instruction: 'internal instructions', kind: 'develop', status: 'running', dependsOn: [], attempts: 1, turnId: 'page-turn' },
        { id: 'verify', title: '联调验收', memberId: 'q', instruction: 'internal instructions', kind: 'verify', status: 'pending', dependsOn: ['api', 'page'], attempts: 0 },
      ] }]
      s.messages = [
        { seq: 1, groupId: 'g1', chainId: 'qa', kind: 'user', text: s.workflows[0].request, hop: 0, createdAt: now },
        { seq: 2, groupId: 'g1', chainId: 'qa', kind: 'system', systemKind: 'coord', text: 'internal coordinator', hop: 0, createdAt: now, route: { mode: 'ai', durationMs: 12345, memberIds: ['b','f'] } },
        { seq: 3, groupId: 'g1', chainId: 'qa', kind: 'system', systemKind: 'route', text: 'internal queue', hop: 0, createdAt: now, route: { mode: 'workflow', queued: true, memberId: 'f' } },
        { seq: 4, groupId: 'g1', chainId: 'qa', kind: 'member', memberId: 'b', text: '', hop: 1, turnId: 'api-turn', createdAt: now, report: { did: '接口已支持按状态筛选，原有查询方式可以继续使用。', parsed: true, files: [{ path: 'backend/tasks.cjs', added: 40, deleted: 2 }], footer: { outputTokens: 4200, tokensPerSec: 50, finishedAt: now } } },
        { seq: 5, groupId: 'g1', chainId: 'qa', kind: 'system', systemKind: 'no_handoff', text: 'internal done notification', hop: 1, createdAt: now },
      ]
      s.processBlocks.b = [{ id: 'qa-process', kind: 'text', text: '已检查接口返回值。\n<cat_task_result>{"status":"done","summary":"internal-result"}</cat_task_result>' }]
      s.retryTurn = async (...args) => { window.groupCalls.push(['retry', ...args]) }
      s.reassign = async (...args) => { window.groupCalls.push(['reassign', ...args]) }
      s.stopAll = async () => { window.groupCalls.push(['stopAll']) }
    })
    const group = page.locator('[data-testid="cat-group-view"]')
    const panel = page.locator('[data-testid="cat-group-workflow"]')
    assert.equal(await panel.getAttribute('open'), null, 'task list must start collapsed')
    let text = await group.innerText()
    assert.match(text, /正在处理/)
    assert.doesNotMatch(text, /token|internal|排第一|原始|工具调用|依赖|没按汇报格式|\d+\/\d+/i)
    assert.equal(await group.locator('.cli, .cg-route, .cg-chint').count(), 0)
    assert.equal(await page.locator('[data-testid="cat-group-members"]').isVisible(), false)
    assert.equal(await page.locator('[data-testid="cat-group-composer"] button:disabled').count(), 1, 'only empty send is disabled, no fake attachment control')
    await page.screenshot({ path: path.join(output, 'working-wide.png') })

    // The user can still see and act on the assignment when needed.
    await panel.locator('summary').click()
    assert.equal(await panel.locator('li').count(), 3)
    assert.match(await panel.innerText(), /等待同事完成/)
    assert.doesNotMatch(await panel.innerText(), /internal-check|dependsOn|证据|api-turn/)
    await panel.locator('li').nth(1).locator('select').selectOption('b')
    assert.deepEqual(await page.evaluate(() => window.groupCalls.at(-1)), ['reassign', 'qa', 'b', 'page'])
    await page.evaluate(() => { const w = window.groupQA.workflows[0]; w.status = 'blocked'; w.tasks[1].status = 'failed'; w.tasks[1].result = '需要确认筛选条件是否包含已归档任务。' })
    await panel.locator('li').nth(1).getByRole('button', { name: '重试', exact: true }).click()
    assert.deepEqual(await page.evaluate(() => window.groupCalls.at(-1)), ['retry', 'f', 'qa', 'page'])
    await panel.getByRole('button', { name: '全部停止', exact: true }).click()
    assert.deepEqual(await page.evaluate(() => window.groupCalls.at(-1)), ['stopAll'])
    await panel.locator('summary').click()

    // Member settings are available on demand, including at the app's minimum width.
    await page.locator('[data-testid="cat-group-members-toggle"]').click()
    await page.locator('[data-testid="cat-group-members"]').waitFor({ state: 'visible' })
    await page.locator('[data-testid="cat-group-members"] .cg-mc-row').first().click()
    assert.equal(await page.locator('[data-testid="cat-group-edit-model"]').count(), 1)
    await page.locator('[data-testid="cat-group-members"]').getByRole('button', { name: '关闭', exact: true }).click()
    await page.locator('[data-testid="cat-group-report"] .cg-msg-more').first().click()
    await page.locator('[data-testid="cat-group-process"]').waitFor()
    assert.match(await page.locator('[data-testid="cat-group-process"]').innerText(), /已检查接口返回值/)
    assert.doesNotMatch(await page.locator('[data-testid="cat-group-process"]').innerText(), /cat_task_result|internal-result/)
    await page.locator('[data-testid="cat-group-process"]').getByRole('button', { name: '关闭', exact: true }).click()

    await page.setViewportSize({ width: 1024, height: 768 })
    await page.screenshot({ path: path.join(output, 'blocked-compact.png') })
    const bounds = await group.evaluate(el => ({ left: el.getBoundingClientRect().left, right: el.getBoundingClientRect().right, viewport: innerWidth, feed: el.querySelector('.cg-feed').getBoundingClientRect().height }))
    assert.ok(bounds.right <= bounds.viewport + 1 && bounds.feed > 250, JSON.stringify(bounds))

    // A completed workflow leaves one concise delivery, without raw validation commands.
    await page.evaluate(() => {
      const s = window.groupQA, w = s.workflows[0]
      w.status = 'completed'
      for (const task of w.tasks) { task.status = 'done'; task.result = task.id === 'api' ? '筛选接口已完成。' : task.id === 'page' ? '页面支持按状态筛选。' : '前后端联调通过，筛选与空结果场景均已验证。' }
      s.messages.push({ seq: 6, groupId: 'g1', chainId: 'qa', kind: 'system', systemKind: 'workflow_summary', text: 'internal full summary with node --test', createdAt: Date.now() })
    })
    await page.locator('[data-testid="cat-group-result"]').waitFor()
    assert.equal(await page.locator('[data-testid="cat-group-workflows"]').count(), 0)
    assert.match(await page.locator('[data-testid="cat-group-result"]').innerText(), /验收通过/)
    assert.doesNotMatch(await page.locator('[data-testid="cat-group-result"]').innerText(), /node --test|internal/)
    await page.setViewportSize({ width: 1440, height: 960 })
    await page.screenshot({ path: path.join(output, 'completed-wide.png') })

    // Mentions align with the first text line, including Chinese input and wrapping.
    await page.evaluate(() => { window.groupQA.send = async (...args) => { window.groupCalls.push(['send', ...args]); return 'sent' } })
    const input = page.locator('[data-testid="cat-group-input"]')
    const composer = page.locator('[data-testid="cat-group-composer"]')
    const chip = composer.locator('.cg-chip')
    const mentionGeometry = () => composer.evaluate(el => {
      const input = el.querySelector('textarea'), box = input.getBoundingClientRect(), css = getComputedStyle(input)
      const firstLineCenter = box.top + parseFloat(css.paddingTop) + parseFloat(css.lineHeight) / 2
      const row = el.querySelector('.row').getBoundingClientRect()
      const chips = Array.from(el.querySelectorAll('.cg-chip'), el => {
        const r = el.getBoundingClientRect()
        return { left:r.left, right:r.right, top:r.top, bottom:r.bottom, center:r.top+r.height/2 }
      })
      return { firstLineCenter, input:{left:box.left, right:box.right, top:box.top}, row:{left:row.left, right:row.right}, chips }
    })
    const assertFirstLine = async () => {
      const g = await mentionGeometry()
      for (const c of g.chips) {
        assert.ok(Math.abs(c.center-g.firstLineCenter) < 1, 'mention must align with first text line: '+JSON.stringify(g))
        assert.ok(c.right < g.input.left, 'mention must not overlap editable text')
      }
    }
    await input.fill('@后端')
    const callsBeforeComposition = await page.evaluate(() => window.groupCalls.length)
    await input.evaluate(el => el.dispatchEvent(new KeyboardEvent('keydown', {key:'Enter', isComposing:true, bubbles:true, cancelable:true})))
    assert.equal(await chip.count(), 0, 'IME confirmation must not choose a mention')
    assert.equal(await input.inputValue(), '@后端')
    assert.equal(await page.evaluate(() => window.groupCalls.length), callsBeforeComposition)
    await input.press('Enter')
    assert.equal(await chip.count(), 1)
    await input.fill('速速速')
    await assertFirstLine()
    await composer.screenshot({path:path.join(output, 'mention-aligned.png')})
    await input.press('Shift+Enter')
    await input.press('a')
    assert.match(await input.inputValue(), /\n/)
    await assertFirstLine()
    await composer.screenshot({path:path.join(output, 'mention-multiline.png')})
    await input.fill('介绍一下自己')
    await input.press('Enter')
    assert.deepEqual(await page.evaluate(() => window.groupCalls.at(-1)), ['send', '@后端工程师 介绍一下自己', ['b'], []])

    // Picking/removing a member keeps the caret and typed text intact.
    await input.fill('前文后文')
    await input.evaluate(el => el.setSelectionRange(2,2))
    await composer.getByRole('button', {name:'选择成员', exact:true}).click()
    await input.press('Enter')
    assert.equal(await input.inputValue(), '前文后文')
    assert.equal(await input.evaluate(el => el.selectionStart), 2)
    await input.press('x')
    assert.equal(await input.inputValue(), '前文x后文')
    await input.fill('前文x后文@前端')
    await input.press('Tab')
    assert.equal(await chip.count(), 2)
    await assertFirstLine()
    await composer.evaluate(el => { el.style.width = '280px' })
    const wrapped = await mentionGeometry()
    assert.ok(wrapped.input.top >= wrapped.chips.at(-1).bottom, 'narrow composer wraps text below mentions')
    for (const c of wrapped.chips) assert.ok(c.left >= wrapped.row.left && c.right <= wrapped.row.right+1, 'chips must fit composer')
    assert.ok(wrapped.input.right <= wrapped.row.right+1, 'text must fit composer')
    await composer.screenshot({path:path.join(output, 'mention-wrapped.png')})
    await composer.evaluate(el => el.style.removeProperty('width'))
    await input.evaluate(el => el.setSelectionRange(0,0))
    await input.press('Backspace')
    assert.equal(await chip.count(), 1)
    assert.equal(await input.inputValue(), '前文x后文', 'backspace at start removes the chip without deleting text')
    await chip.locator('button').click()
    assert.equal(await chip.count(), 0)
    assert.equal(await input.evaluate(el => document.activeElement === el), true)
    await input.fill('大家介绍下自己')
    await input.press('Enter')
    assert.deepEqual(await page.evaluate(() => window.groupCalls.at(-1)), ['send', '大家介绍下自己', [], []])

    // Native boundaries are stubbed; the composer, persistence DTO and media
    // preview run unchanged. Real file/CLI coverage lives in the Go tests.
    await page.evaluate(() => {
      const canvas = document.createElement('canvas'); canvas.width = 240; canvas.height = 120
      const ctx = canvas.getContext('2d'); ctx.fillStyle = '#f01414'; ctx.fillRect(0,0,120,120); ctx.fillStyle = '#141ef0'; ctx.fillRect(120,0,120,120)
      window.imagePNG = canvas.toDataURL('image/png').split(',')[1]
      window.imageSaved = 0
      window.runtime = window.runtime || {}
      window.go = { app: {
        SystemService: {
          PickFiles: async () => ['C:/qa/picked.png'],
          SavePastedFile: async (name, b64) => { if (!b64) throw Error('empty upload'); return 'C:/qa/upload-' + (++window.imageSaved) + '.png' },
          ClipboardFilePaths: async () => [], ClipboardImage: async () => '',
        },
        CatService: { ReadCatMedia: async () => window.imagePNG },
      } }
    })
    const draftImages = page.locator('[data-testid="cat-group-draft-images"]')
    const sendButton = page.locator('[data-testid="cat-group-send"]')
    await page.locator('[data-testid="cat-group-add-image"]').click()
    await draftImages.locator('.cg-image').waitFor()
    await draftImages.locator('img').waitFor()
    await page.waitForFunction(() => document.querySelector('[data-testid="cat-group-draft-images"] img')?.naturalWidth === 240)
    assert.equal(await sendButton.isEnabled(), true, 'image-only messages can send')
    await draftImages.locator('.cg-image-preview').click()
    await page.locator('[data-testid="cat-attach-preview"] img').waitFor()
    await page.keyboard.press('Escape')
    await draftImages.getByRole('button', { name: /移除图片/ }).click()
    assert.equal(await sendButton.isEnabled(), false)

    await page.locator('[data-testid="cat-group-image-input"]').setInputFiles({name:'picked.png',mimeType:'image/png',buffer:Buffer.from(await page.evaluate(() => window.imagePNG),'base64')})
    await draftImages.locator('.cg-image').waitFor()
    await input.evaluate((el) => {
      const bin = atob(window.imagePNG), bytes = Uint8Array.from(bin, c => c.charCodeAt(0))
      const dt = new DataTransfer(); dt.items.add(new File([bytes], 'screenshot.png', {type:'image/png'}))
      el.dispatchEvent(new ClipboardEvent('paste', {bubbles:true,cancelable:true,clipboardData:dt}))
    })
    await page.waitForFunction(() => document.querySelectorAll('[data-testid="cat-group-draft-images"] .cg-image').length === 2)
    await page.locator('[data-testid="cat-group-composer"]').evaluate(el => {
      const dt = new DataTransfer(), file = new File(['image'], 'dropped.png', {type:'image/png'})
      Object.defineProperty(file,'path',{value:'C:/qa/dropped.png'})
      dt.items.add(file)
      el.dispatchEvent(new DragEvent('drop',{bubbles:true,cancelable:true,dataTransfer:dt}))
    })
    assert.equal(await draftImages.locator('.cg-image').count(), 3)
    await page.screenshot({ path: path.join(output,'images-wide.png') })

    // Failed sends restore text and all images; new draft input is preserved.
    await input.fill('请按图片调整页面')
    await page.evaluate(() => { window.groupQA.send = async () => { throw Error('offline') } })
    await sendButton.click()
    assert.equal(await input.inputValue(),'请按图片调整页面')
    assert.equal(await draftImages.locator('.cg-image').count(), 3)
    await page.evaluate(() => {
      window.groupQA.send = async (...args) => {
        window.groupCalls.push(['send',...args])
        window.groupQA.messages.push({seq:100,groupId:'g1',chainId:'image-message',kind:'user',text:args[0],mentions:args[1],files:args[2],hop:0,createdAt:Date.now()})
        return 'image-message'
      }
    })
    await input.fill('')
    await sendButton.click()
    const sentImages = await page.evaluate(() => window.groupCalls.at(-1))
    assert.equal(sentImages[1],'')
    assert.equal(sentImages[3].length,3)
    assert.equal(await page.locator('[data-testid="cat-group-draft-images"]').count(),0)
    await page.locator('[data-testid="cat-group-message-images"] img').first().waitFor()
    assert.equal(await page.locator('[data-testid="cat-group-message-images"] .cg-image').count(),3)
    const { normalizeMessage } = await page.evaluate(async () => {
      const {normalizeMessage} = await import('/src/api/catGroupChat.ts')
      const m = window.groupQA.messages.find(x=>x.seq===100)
      return {normalizeMessage:normalizeMessage(m)}
    })
    assert.equal(normalizeMessage.files.length,3,'history normalization retains attachments')
    await page.screenshot({ path: path.join(output,'images-sent.png') })
    assert.deepEqual(errors, [], 'no browser runtime errors')
    console.log('PASS: compact progress, hidden internals, exact task retry/reassign, stop, member settings, process, 1024/1440 layouts, final delivery; mention first-line alignment/wrapping, IME, caret, deletion, no-mention sending; image picker/upload/paste/drop/preview/removal, image-only send, failed draft recovery and history thumbnails')
    console.log('Screenshots:', output)
  } finally { await browser.close() }
})().catch(e => { console.error(e); process.exitCode = 1 })
