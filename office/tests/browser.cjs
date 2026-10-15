// Run via TestEmbeddedCanvasBrowser; the Go host and real Vue component are used.
const fs = require('node:fs'), path = require('node:path'), assert = require('node:assert/strict')
const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
;(async () => {
  const preview = JSON.parse(process.env.FFC_OFFICE_QA_PREVIEW)
  const control = process.env.FFC_OFFICE_QA_CONTROL
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 960 } }), errors = []
    page.on('pageerror', e => { errors.push(e.message); console.error('pageerror:', e.message) })
    await page.goto(process.env.FFC_FRONTEND_QA_URL || 'http://127.0.0.1:4182/?cat=chat&cat_conv=c21#/cat')
    await page.locator('textarea.ta').waitFor({ timeout: 20000 })
    await page.evaluate(async p => {
      window.go = { app: { CatService: {
        StartCatPptPreview: () => Promise.resolve(p), ReleaseCatPptPreview: () => Promise.resolve(),
        GetCatPptTool: () => Promise.resolve({ state: 'ready' }),
      } } }
      const vue = await import('/node_modules/.vite/deps/vue.js'), { i18n } = await import('/src/i18n/index.ts')
      const { createPinia } = await import('/node_modules/.vite/deps/pinia.js'), { default: C } = await import('/src/components/cat/CatPptxPreview.vue')
      const host = document.createElement('div'); host.id = 'qa-office'; host.className = 'ct'
      host.style = 'position:fixed;inset:20px;z-index:9999;display:flex;background:white'; document.body.appendChild(host)
      window.qaApp = vue.createApp(C, { convId: 'qa', relPath: 'live.pptx' }); window.qaApp.use(createPinia());window.qaApp.use(i18n);window.qaApp.mount(host)
    }, preview)
    const frameElement = page.locator('#qa-office iframe')
    await frameElement.waitFor()
    const frame = page.frameLocator('#qa-office iframe')
    try { await frame.locator('.slide-list .thumb').first().waitFor({ timeout: 15000 }) }
    catch (error) {
      console.error('Editor diagnostics:', await frame.locator('body').innerText())
      await page.locator('#qa-office').screenshot({ path: path.resolve(__dirname, '../../.gocache/genoffice-preview-failed.png') })
      throw error
    }
    for (const label of ['Genspark AI', 'AI 美化', 'AI 事实核查', 'AI 配图']) {
      assert.equal(await frame.getByText(label, { exact: true }).count(), 0, label + ' should be removed')
    }
    assert.equal(await frame.locator('.stage-ai-bar').count(), 0)
    assert.equal(await page.locator('#qa-office .ppt-hostbar').count(), 0, 'no extra ffc toolbar')
    const edit = async step => {
      const response = await fetch(control + '?step=' + step)
      if (!response.ok) throw new Error(await response.text())
      return response.json()
    }
    await edit('1')
    await frame.locator('.slide-list .thumb').first().waitFor()
    await frame.locator('.slide-text-editor').count()
    const canvas = frame.locator('.stage-wrap canvas').first()
    const size = await canvas.boundingBox()
    assert.ok(size && size.width > 500 && size.height > 200, 'canvas must have real layout')
    await page.waitForTimeout(450)
    await edit('2')
    await frame.locator('.slide-list .thumb').nth(1).waitFor()
    assert.equal(await frame.locator('.slide-list .thumb').count(), 2)
    await edit('edit')
    await page.waitForTimeout(350)
    await frame.locator('.slide-list .thumb').first().click()
    await page.waitForTimeout(250)
    await page.locator('#qa-office').screenshot({ path: path.resolve(__dirname, '../../.gocache/genoffice-preview.png') })
    const bounds = await frame.locator('.stage-rel').boundingBox(), scale = bounds.width / 1280
    await page.mouse.dblclick(bounds.x + 160 * scale, bounds.y + 145 * scale)
    await frame.locator('.slide-text-editor').waitFor()
    assert.match(await frame.locator('.slide-text-editor').innerText(), /标题实时更新/)
    await frame.locator('.slide-text-editor').fill('ffc · 官方编辑器修改完成')
    await frame.locator('.slide-list .thumb').first().click()
    await frame.locator('.slide-text-editor').waitFor({ state: 'hidden' })
    await page.waitForTimeout(350)
    await frame.getByRole('button', { name: '撤销', exact: true }).click()
    await page.waitForTimeout(300)
    await page.mouse.dblclick(bounds.x + 160 * scale, bounds.y + 145 * scale)
    await frame.locator('.slide-text-editor').waitFor()
    assert.match(await frame.locator('.slide-text-editor').innerText(), /标题实时更新/)
    await frame.locator('.slide-list .thumb').first().click()
    // User draft must survive an Agent edit made while the native overlay is open.
    await page.mouse.dblclick(bounds.x + 160 * scale, bounds.y + 145 * scale)
    await frame.locator('.slide-text-editor').waitFor()
    await frame.locator('.slide-text-editor').fill('手动草稿应保留')
    await edit('conflict')
    await page.waitForTimeout(300)
    assert.match(await frame.locator('.slide-text-editor').innerText(), /手动草稿应保留/)
    await frame.locator('.qa-btn[data-tip^="保存"]').click()
    await frame.getByText('文档已变化，请重新读取后修改', { exact: false }).waitFor()
    assert.match(await frame.locator('.slide-text-editor').innerText(), /手动草稿应保留/)
    await frame.locator('.slide-text-editor').press('Escape')
    await frame.locator('.slide-text-editor').waitFor({ state: 'hidden' })
    await page.mouse.dblclick(bounds.x + 160 * scale, bounds.y + 145 * scale)
    await frame.locator('.slide-text-editor').waitFor()
    assert.match(await frame.locator('.slide-text-editor').innerText(), /Agent 已写入新版本/)
    await frame.locator('.slide-list .thumb').first().click()
    // Use the official ribbon for slide creation; the native host must persist it.
    const homeTab = frame.locator('.ribbon-tab').filter({ hasText: /^开始$/ })
    if (!(await homeTab.evaluate(e => e.classList.contains('active')))) await homeTab.click()
    await frame.locator('[data-rbgroup="slides"] .rb-big').first().click()
    await frame.getByRole('button', { name: '新建幻灯片', exact: true }).click()
    await frame.locator('.slide-list .thumb').nth(2).waitFor()
    await frame.locator('.slide-list .thumb').first().click()
    const beforeZoom = await frame.locator('.stage-rel').boundingBox()
    await frame.locator('.zoom-btn').first().click()
    await page.waitForTimeout(180)
    assert.ok((await frame.locator('.stage-rel').boundingBox()).width < beforeZoom.width)
    // A hidden/re-shown tab must regain its width, fixing the old iframe zero-size failure.
    await page.locator('#qa-office').evaluate(e => { e.style.display = 'none' })
    await page.waitForTimeout(100)
    await page.locator('#qa-office').evaluate(e => { e.style.display = 'flex'; e.style.right = '320px' })
    await page.waitForTimeout(300)
    assert.ok((await canvas.boundingBox()).width > 300)
    await frame.getByText('文档已变化，请重新读取后修改', { exact: false }).waitFor({ state: 'hidden', timeout: 12000 })
    const output = path.resolve(__dirname, '../../.gocache/genoffice-preview.png')
    fs.mkdirSync(path.dirname(output), { recursive: true })
    await page.locator('#qa-office').screenshot({ path: output })
    assert.deepEqual(errors, [], 'browser should have no runtime errors')
    console.log('Vue + GenOffice QA passed: removed AI entries, incremental pages, live edits, native text editing, preserved conflict drafts, undo, slide creation, zoom and tab resizing')
  } finally { await browser.close() }
})().catch(e => { console.error(e);process.exitCode=1 })
