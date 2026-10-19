const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')
const assert = require('node:assert/strict'), path = require('node:path'), fs = require('node:fs')
;(async () => {
  const browser = await chromium.launch({ channel:'msedge', headless:true })
  const output = path.resolve(__dirname,'../../.gocache/group-qa'); fs.mkdirSync(output,{recursive:true})
  try {
    const page = await browser.newPage({viewport:{width:1024,height:768}}), errors=[]
    page.on('pageerror', e=>errors.push(e.message))
    await page.goto((process.env.FFC_FRONTEND_URL || 'http://127.0.0.1:5173')+'/?cat_group_scene=report#/cat')
    await page.locator('[data-testid="cat-group-view"]').waitFor()
    await page.evaluate(async()=>{
      const {useCatGroupChat}=await import('/src/stores/catGroupChat.ts')
      const s=window.scrollQA=useCatGroupChat(); s.workflows=[]; s.hasMore=false
      s.messages=Array.from({length:32},(_,i)=>({seq:i+1,groupId:'g1',chainId:'qa',kind:'member',memberId:i%3===0?'b':'f',text:'消息 '+(i+1)+'：'+ '接口约定已同步，继续检查联调结果。'.repeat(3),hop:0,createdAt:Date.now()+i}))
      s.send=async(text,mentions,files)=>{
        // Replacement snapshot/event delivery can keep the same array length.
        s.messages=[...s.messages.slice(1),{seq:100,groupId:'g1',chainId:'sent',kind:'user',text,mentions,files,hop:0,createdAt:Date.now()}]
        return 'sent'
      }
    })
    const feed=page.locator('[data-testid="cat-group-feed"]'), input=page.locator('[data-testid="cat-group-input"]')
    const settle=()=>page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))))
    const gap=()=>feed.evaluate(el=>el.scrollHeight-el.clientHeight-el.scrollTop)
    await settle()
    assert.ok(await feed.evaluate(el=>el.scrollHeight>el.clientHeight*2),'fixture must overflow')
    await feed.evaluate(el=>{el.scrollTop=0}); await settle()
    await input.fill('这条新消息发送后应该自动下翻显示')
    await input.press('Enter'); await settle()
    assert.ok(await gap()<3,'submitting from older history must reveal the latest message; gap='+await gap())
    await page.evaluate(()=>{
      const s=window.scrollQA
      s.messages.push({seq:101,groupId:'g1',chainId:'sent',kind:'member',memberId:'b',text:'收到，正在检查。',hop:0,createdAt:Date.now()})
    }); await settle()
    assert.ok(await gap()<3,'reply should follow the latest message')
    await page.evaluate(()=>{window.scrollQA.messages.at(-1).text+='\n'+Array.from({length:16},(_,i)=>'检查结果 '+i+'：接口与页面返回一致。').join('\n')}); await settle()
    assert.ok(await gap()<3,'same-message growth should stay at the bottom; gap='+await gap())
    await page.setViewportSize({width:1024,height:640}); await settle()
    assert.ok(await gap()<3,'viewport resize should preserve bottom following')
    await feed.evaluate(el=>{el.scrollTop=0}); await settle()
    await page.evaluate(()=>{const s=window.scrollQA;s.messages.push({seq:102,groupId:'g1',chainId:'sent',kind:'member',memberId:'q',text:'验收通过。',hop:0,createdAt:Date.now()})}); await settle()
    assert.ok(await gap()>200,'reading history should not be interrupted by employee replies')
    await page.evaluate(()=>{const s=window.scrollQA;s.messages=[{seq:0,groupId:'g1',chainId:'old',kind:'member',memberId:'b',text:'更早的消息',hop:0,createdAt:Date.now()-100000},...s.messages]}); await settle()
    assert.ok(await gap()>200,'loading older messages should not jump to bottom')
    await input.fill('再发一条，回到最新记录'); await input.press('Enter'); await settle()
    assert.ok(await gap()<3,'another submission must resume bottom following')
    await page.screenshot({path:path.join(output,'chat-scroll-compact.png')})

    // The reference-style conversation keeps speakers and short messages
    // distinct, and combines consecutive replies without repeating the name.
    await page.setViewportSize({width:1440,height:960})
    await page.evaluate(()=>{
      const s=window.scrollQA
      s.group.name='项目开发群'
      const now=Date.now()
      s.messages=[
        {kind:'user',text:'大家介绍下自己，今天一起把页面联调完成。'},
        {kind:'member',memberId:'f',text:'我负责前端页面和交互，先整理组件与接口需要对齐的地方。'},
        {kind:'member',memberId:'b',text:'我负责后端接口，先把参数、返回值和错误处理约定发出来。'},
        {kind:'member',memberId:'b',text:'接口约定已经同步，前端可以开始对接。'},
        {kind:'member',memberId:'b',text:'我继续补边界情况，有问题直接在群里说。'},
        {kind:'member',memberId:'q',text:'收到，等前后端完成后，我来跑联调和验收。'},
        {kind:'user',text:'可以，完成后告诉我结果就行。'},
        {kind:'member',memberId:'f',report:{did:'筛选页面已完成，接口也已接通。',parsed:true,files:[{path:'frontend/tasks.vue'}]}},
      ].map((m,i)=>({...m,seq:200+i,groupId:'g1',chainId:'chat-style',hop:0,createdAt:now+i*1000}))
    }); await settle()
    const chats=page.locator('.cg-chat')
    assert.equal(await chats.count(),6)
    assert.equal(await chats.locator('.cg-chat-name').count(),4,'consecutive employee replies share one name')
    assert.equal(await page.locator('.cg-chat.continued').count(),2)
    const layout=await page.evaluate(()=>{
      const feed=document.querySelector('.cg-feed').getBoundingClientRect(), own=document.querySelector('.cg-task').getBoundingClientRect(), bubble=document.querySelector('.cg-short'), member=bubble.getBoundingClientRect()
      return {ownRight:own.right,memberRight:member.right,feedWidth:feed.width,bubbleWidth:member.width,border:getComputedStyle(bubble).borderWidth,shadow:getComputedStyle(bubble).boxShadow,overflow:document.documentElement.scrollWidth>innerWidth}
    })
    assert.ok(layout.ownRight>layout.memberRight,'own messages align right')
    assert.ok(layout.bubbleWidth<layout.feedWidth*.8,'short bubbles fit their content')
    assert.equal(layout.border,'0px'); assert.equal(layout.shadow,'none'); assert.equal(layout.overflow,false)
    await page.screenshot({path:path.join(output,'chat-bubbles-wide.png')})
    await page.evaluate(()=>document.documentElement.classList.add('dark')); await settle()
    await page.screenshot({path:path.join(output,'chat-bubbles-dark.png')})
    assert.deepEqual(errors,[])
    console.log('PASS: own sends reveal latest, equal-length snapshots, reply following, same-message growth, viewport resize, history reading, older-message loading; compact bubbles, own-message alignment, consecutive speaker grouping and light/dark layouts')
  } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1})
