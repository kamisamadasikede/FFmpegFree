// Real page reloads and Vue navigation; only the group data API is a fixture.
// Run against Vite: node frontend/scripts/check-group-restore.cjs
const assert = require('node:assert/strict')
const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
const LAST = 'ffmpegfree.cat.lastGroup', POSITIONS = 'ffmpegfree.cat.groupPositions'
const url = (process.env.FFC_FRONTEND_URL || 'http://127.0.0.1:5173') + '/#/cat'
;(async () => {
  const browser = await chromium.launch({channel:'msedge',headless:true})
  const errors = []
  try {
    const page = await browser.newPage({viewport:{width:1024,height:768}})
    page.on('pageerror', e => errors.push(e.message))
    await page.route('**/src/api/catGroupChatMock.ts', async route => {
      const response = await route.fetch()
      await route.fulfill({response, body:await response.text()+`
        const fixtureGetGroup = groupMock.getGroup.bind(groupMock)
        groupMock.getGroup = async id => {
          const delay = Number(sessionStorage.getItem('qa-delay-'+id)) || 0
          if (delay) await new Promise(resolve => setTimeout(resolve, delay))
          if (id === 'deleted') throw Error(JSON.stringify({code:'NOT_FOUND',message:'群聊已删除'}))
          if (sessionStorage.getItem('qa-offline')) throw Error(JSON.stringify({code:'IO_ERROR',message:'暂时无法读取群聊'}))
          return fixtureGetGroup(id)
        }
        groupMock.listMessages = async (id, before=0, limit=50) => {
          const total = Number(sessionStorage.getItem('qa-count')) || 150
          const all = Array.from({length:total}, (_, i) => ({seq:i+1, groupId:id, chainId:'qa', kind:'member', memberId:i%2?'b':'f', text:'历史记录 '+(i+1)+'：接口和页面约定已同步，继续检查联调结果。'.repeat(2), hop:0, createdAt:1720000000000+i*1000})).filter(m => !before || m.seq<before)
          return {messages:all.slice(-limit),hasMore:all.length>limit}
        }
      `})
    })
    await page.goto(url)
    const group = page.locator('[data-testid="cat-group-view"]')
    const feed = page.locator('[data-testid="cat-group-feed"]')
    const settle = () => page.evaluate(() => new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r))))
    const ready = async id => {
      await group.waitFor()
      await page.evaluate(async () => {
        const {useCatGroupChat} = await import('/src/stores/catGroupChat.ts')
        window.restoreQA = useCatGroupChat()
      })
      await page.waitForFunction(id => window.restoreQA.group?.id === id && !window.restoreQA.loading && window.restoreQA.messages.length > 0, id)
      await page.waitForFunction(()=>document.querySelector('[data-testid="cat-group-view"]')?.getAttribute('aria-busy') === 'false')
      await settle()
    }
    const open = id => page.locator('[data-testid="cat-group-item"]').filter({hasText:id==='g1'?'群聊开发':'发版检查'}).click()
    const gap = () => feed.evaluate(el => el.scrollHeight-el.clientHeight-el.scrollTop)
    const reading = () => feed.evaluate(el => {
      const top = el.getBoundingClientRect().top
      const row = Array.from(el.querySelectorAll('[data-message-seq]')).find(row => row.getBoundingClientRect().bottom>top)
      return {seq:Number(row?.dataset.messageSeq), offset:row?.getBoundingClientRect().top-top}
    })
    const readAt = async seq => {
      while (!(await feed.locator('[data-message-seq="'+seq+'"]').count())) await feed.locator('.cg-feed-in > button').click()
      await feed.evaluate((el, seq) => {
        el.scrollTop += el.querySelector('[data-message-seq="'+seq+'"]').getBoundingClientRect().top-el.getBoundingClientRect().top+9
      }, seq)
      await settle()
      return reading()
    }
    await page.evaluate(async () => {
      const {catState} = await import('/src/views/cat/catState.ts')
      catState.projects.find(p=>p.id==='p3').open=true
      localStorage.setItem('ffmpegfree.cat.lastConv','previous-ordinary-chat')
    })
    await page.locator('[data-testid="cat-group-item"]').filter({hasText:'群聊开发'}).click()
    await ready('g1')
    assert.equal(await page.evaluate(key=>localStorage.getItem(key),LAST),'g1','opening the group immediately persists it')
    const old = await readAt(22)
    assert.equal(old.seq,22)
    // Refresh immediately: pagehide must save even before the debounce fires.
    await page.evaluate(()=>sessionStorage.setItem('qa-count','170'))
    await page.reload()
    await ready('g1')
    const restored = await reading()
    assert.equal(restored.seq,old.seq,'refresh restores an anchor outside the latest 50 messages')
    assert.ok(Math.abs(restored.offset-old.offset)<2,JSON.stringify({old,restored}))
    assert.ok(await gap()>1000,'new messages must not pull history readers to the bottom')
    assert.equal(await page.locator('[data-testid="cat-group-item"].on').isVisible(),true,'restored group expands its sidebar project')
    assert.equal(await page.evaluate(()=>localStorage.getItem('ffmpegfree.cat.lastConv')),'previous-ordinary-chat','group selection preserves ordinary conversation history')

    await open('g2'); await ready('g2')
    const second = await readAt(45)
    await open('g1'); await ready('g1')
    assert.equal((await reading()).seq,22,'switching groups restores the first group position')
    await open('g2'); await ready('g2')
    assert.equal((await reading()).seq,second.seq,'each group has a separate reading position')
    await page.reload(); await ready('g2')
    assert.equal((await reading()).seq,second.seq,'the last selected group survives a real reload')

    await feed.evaluate(el=>{el.scrollTop=el.scrollHeight}); await settle()
    await page.evaluate(()=>sessionStorage.setItem('qa-count','180'))
    await page.reload(); await ready('g2')
    assert.ok(await gap()<3,'readers at the bottom follow the newest message after refresh')
    await page.getByTestId('cat-nav-settings').click()
    await group.waitFor({state:'detached'})
    await page.reload(); await ready('g2')

    await page.locator('[data-testid="cat-conv-row"][data-cid="c31"]').click()
    await group.waitFor({state:'detached'})
    assert.equal(await page.evaluate(key=>localStorage.getItem(key),LAST),null)
    await page.reload()
    await page.locator('[data-testid="cat-page"] .ct-c').waitFor()
    assert.equal(await group.count(),0,'ordinary conversation navigation clears the group restore target')
    await open('g1'); await ready('g1')
    await page.locator('.ct-it.new').click()
    await group.waitFor({state:'detached'})
    await page.reload()
    await page.locator('[data-testid="cat-page"] .ct-c').waitFor()
    assert.equal(await group.count(),0,'explicit new-chat selection survives refresh')

    await page.evaluate(([last,positions])=>{localStorage.setItem(last,'deleted');localStorage.setItem(positions,JSON.stringify([['deleted',{atBottom:false,seq:22,offset:-9}]]))},[LAST,POSITIONS])
    await page.reload()
    await page.waitForFunction(key=>localStorage.getItem(key)===null,LAST)
    await page.locator('[data-testid="cat-page"] .ct-c').waitFor()
    assert.equal(await group.count(),0,'a deleted group falls back to the ordinary chat')
    assert.equal(await page.evaluate(key=>JSON.parse(localStorage.getItem(key)).some(([id])=>id==='deleted'),POSITIONS),false)

    await page.evaluate(([last,positions])=>{localStorage.setItem(last,'g1');localStorage.setItem(positions,'corrupt-json');sessionStorage.setItem('qa-offline','1')},[LAST,POSITIONS])
    await page.reload()
    await page.getByRole('alert').filter({hasText:'暂时无法读取群聊'}).waitFor()
    assert.equal(await page.getByTestId('cat-group-send').isEnabled(),false,'unloaded groups cannot send with stale members')
    assert.equal(await page.evaluate(key=>localStorage.getItem(key),LAST),'g1','transient API failure must retain the group restore target')
    await page.evaluate(()=>sessionStorage.removeItem('qa-offline'))
    await page.reload(); await ready('g1')
    assert.ok(await gap()<3,'corrupt saved positions safely fall back to the latest message')

    // A late request for another group cannot finish the current load early.
    await page.evaluate(()=>{sessionStorage.setItem('qa-delay-g1','80');sessionStorage.setItem('qa-delay-g2','350')})
    await open('g2'); await open('g1'); await open('g2')
    await page.waitForTimeout(140)
    assert.equal(await page.evaluate(async()=>{const {useCatGroupChat}=await import('/src/stores/catGroupChat.ts');return useCatGroupChat().loading}),true)
    await ready('g2')
    await page.waitForTimeout(100)
    assert.equal(await page.evaluate(async()=>{const {useCatGroupChat}=await import('/src/stores/catGroupChat.ts');return useCatGroupChat().group.id}),'g2')

    // Disabled local storage must not prevent normal navigation or reading.
    await page.addInitScript(() => {
      for (const method of ['getItem','setItem','removeItem']) {
        const original = Storage.prototype[method]
        Storage.prototype[method] = function(key, ...args) {
          if (key==='ffmpegfree.cat.lastGroup' || key==='ffmpegfree.cat.groupPositions') throw new DOMException('disabled','SecurityError')
          return original.call(this,key,...args)
        }
      }
    })
    await page.reload()
    await page.locator('[data-testid="cat-page"] .ct-c').waitFor()
    await open('g1'); await ready('g1')
    assert.ok(await gap()<3)
    await readAt(40)
    await page.evaluate(()=>window.dispatchEvent(new Event('pagehide')))
    assert.deepEqual(errors,[],'no uncaught browser errors during refresh/restoration')
    console.log('PASS: real refresh, last selected group, per-group reading anchors across pagination, new replies while away, latest following, sidebar expansion, ordinary/new chat navigation, settings roundtrip, deleted group, transient failure, corrupt/disabled storage and rapid group switches')
  } finally { await browser.close() }
})().catch(e=>{console.error(e);process.exitCode=1})
