// Opt-in browser integration: run through TestOfficeCLIIntegration with FFC_PPT_BROWSER_QA=1.
const fs = require('node:fs'), assert = require('node:assert/strict')
const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
;(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 960 } }), errors = []
    page.on('pageerror', e => errors.push(e.message));page.on('console',m=>{if(m.type()==='error')console.log(m.text())})
    await page.goto('http://127.0.0.1:4182/?cat=chat&cat_conv=c21#/cat')
    await page.locator('textarea.ta').waitFor()
    await page.evaluate(async url => {
      window.releases = 0
      const request = async (path, values={}) => {
        const r = await fetch(url + path + '?' + new URLSearchParams(values)); if (!r.ok) throw new Error(await r.text()); return r.json()
      }
      window.runtime = { EventsOnMultiple: () => () => {} }
      window.go = { app: { CatService: {
        StartCatPptPreview: () => request('/start'),
        ReadCatPptPreview: (id, version) => request('/read', { id, version }),
        ReleaseCatPptPreview: id => { window.releases++; return request('/release', { id }) },
        GetCatPptTool: () => Promise.resolve({ state: 'ready' }),
        ListCatFiles: () => Promise.resolve({ root: 'D:/QA', entries: [{name:'live.pptx',relPath:'live.pptx',isDir:false,size:12000},{name:'second.pptx',relPath:'second.pptx',isDir:false,size:12000}], truncated: false }),
        GitCatStatus: () => Promise.resolve({ repo:false, files:[], branches:[] }),
        ListCatBranches: () => Promise.resolve({ repo:false }),
      } } }
      window.editPpt = step => request('/edit', { step })
      const vue = await import('/node_modules/.vite/deps/vue.js'), { i18n } = await import('/src/i18n/index.ts')
      const { createPinia } = await import('/node_modules/.vite/deps/pinia.js'), { default: C } = await import('/src/components/cat/CatFilesPanel.vue')
      const host = document.createElement('div'); host.id = 'qa-ppt'; host.className = 'ct'; host.style='position:fixed;top:12px;bottom:12px;right:12px;width:1320px;z-index:900;display:flex;background:var(--cat-bg)'; document.body.appendChild(host)
      window.qaApp=vue.createApp(C,{ convId: 'ppt-test', live: true });window.qaApp.use(i18n).use(createPinia()).mount(host)

    }, process.env.FFC_PPT_QA_URL)
    await page.locator('#qa-ppt .ct-node').filter({hasText:'live.pptx'}).click()
    const frame = page.frameLocator('#qa-ppt iframe.ppt-cli-frame').first()
    try { await frame.locator('.slide-container[data-slide]').first().waitFor({timeout:12000}) } catch(e) { console.log('HOST',await page.locator('#qa-ppt').innerText()); console.log('FRAMES',page.frames().length,'ERRORS',errors);if(page.frames()[1])console.log('FRAME',await page.frames()[1].evaluate(()=>({h:innerHeight,anc:[...document.querySelectorAll('html,body,.main')].map(n=>[n.outerHTML.slice(0,80),getComputedStyle(n).width,getComputedStyle(n).height,getComputedStyle(n).display,getComputedStyle(n).visibility]),root:getComputedStyle(document.documentElement).getPropertyValue('--slide-design-w'),w:innerWidth,html:document.head.innerHTML.slice(-4000),slide:document.querySelector('.slide')?.outerHTML.slice(0,1500),dims:[...document.querySelectorAll('.main,.slide,.slide-wrapper,.slide-container')].map(n=>[n.className,n.offsetWidth,n.offsetHeight,getComputedStyle(n).display])}))); console.log('HOSTDIMS',await page.locator('#qa-ppt .ct-previews,#qa-ppt .pv,#qa-ppt .ppt-preview,#qa-ppt iframe').evaluateAll(ns=>ns.map(n=>[n.className,n.offsetWidth,n.offsetHeight,getComputedStyle(n).display]))); await page.screenshot({path:'.gocache/ppt-debug.png'});throw e }
    assert.equal(await frame.locator('.slide-container[data-slide]').count(),1)
    assert.match(await frame.locator('.slide').innerText(),/External replacement/)
    const dims = await frame.locator('.slide').first().evaluate(n => ({ w:n.getBoundingClientRect().width, h:n.getBoundingClientRect().height, inner:innerWidth }))
    assert.ok(dims.w <= dims.inner - 20,JSON.stringify(dims));assert.ok(Math.abs(dims.w / dims.h - 16/9) < .01,JSON.stringify(dims))
    assert.equal(await page.locator('#qa-ppt iframe').first().getAttribute('sandbox'),'allow-scripts')
    assert.match(await frame.locator('meta[http-equiv="Content-Security-Policy"]').getAttribute('content'),/connect-src 'none'/)
    await page.evaluate(() => window.editPpt('add'))
    await frame.locator('.slide-container[data-slide="2"]').waitFor({timeout:20000})
    assert.match(await frame.locator('.slide-container[data-slide="2"]').innerText(),/Browser live update/)
    await page.evaluate(() => window.editPpt('set'))
    await frame.locator('.slide-container[data-slide="2"] .para[style*="60pt"]').waitFor({timeout:20000})
    const input = page.locator('#qa-ppt .ppt-tools input').first()
    await input.fill('1');await input.dispatchEvent('change');await page.waitForTimeout(120);assert.equal(await input.inputValue(),'1')
    await input.fill('2');await input.dispatchEvent('change');await page.waitForTimeout(120);assert.equal(await input.inputValue(),'2')
    await page.locator('#qa-ppt .ct-node').filter({hasText:'second.pptx'}).click()
    await page.locator('#qa-ppt .ct-ppt-tab').nth(1).waitFor()
    assert.equal(await page.locator('#qa-ppt iframe.ppt-cli-frame').count(),2)
    await page.locator('#qa-ppt .ct-ppt-tab').first().locator('button').first().click()
    await page.locator('#qa-ppt').screenshot({ path: '.gocache/ppt-cli-live-qa.png' })
    assert.deepEqual(errors,[])
    await page.locator('#qa-ppt .ct-ppt-tab').nth(1).locator('button').last().click()
    await page.waitForFunction(() => window.releases >= 1)
    assert.equal(await page.locator('#qa-ppt iframe.ppt-cli-frame').count(),1)
    await page.evaluate(() => window.qaApp.unmount());await page.waitForFunction(() => window.releases >= 2)
    console.log('PASS: native CLI HTML, 16:9 fit, live add/set, page navigation, sandbox/CSP, two tabs and lease release')
  } finally { await browser.close() }
})().catch(e=>{ console.error(e);process.exitCode=1 })
