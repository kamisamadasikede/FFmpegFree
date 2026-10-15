// Opens the official docs, PDF, HTML, Markdown, and Sheets pages against the loopback host.
const fs = require('node:fs'), os = require('node:os'), path = require('node:path')
const { chromium } = require(process.env.FFC_PLAYWRIGHT || 'C:/Users/Administrator/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/node_modules/playwright')

function sheetSummary(requestText, status, bodyText) {
  let command = ''
  let range = ''
  try {
    const req = JSON.parse(requestText || '{}')
    command = String(req.command || '')
    if (req.range) range = JSON.stringify(req.range)
  } catch { command = 'unparsed' }
  let brief = ''
  try {
    const body = JSON.parse(bodyText || '{}')
    if (typeof body.error === 'string') brief = 'error=' + body.error.slice(0, 80)
    else if (body.error) brief = 'error'
    else if (Array.isArray(body.cells)) {
      const first = body.cells[0]
      brief = 'cells=' + body.cells.length + ' through=' + body.indexedThroughRow + ' done=' + body.indexingComplete + (first ? ' first=' + String(first.value).slice(0, 40) : '')
    } else if (Array.isArray(body.sheets)) {
      const sheet = body.sheets[0] || {}
      brief = 'sheets=' + body.sheets.length + ' rows=' + sheet.rowCount + ' cols=' + sheet.columnCount + ' values=' + sheet.valueCellCount
    } else brief = 'keys=' + Object.keys(body).slice(0, 8).join(',')
  } catch { brief = 'nonjson ' + String(bodyText || '').slice(0, 80) }
  return command + ' ' + status + ' ' + range + ' ' + brief
}

;(async () => {
  const pages = JSON.parse(process.env.FFC_OFFICE_QA_PAGES || '[]')
  if (!pages.length) throw new Error('no official pages to open')
  const browser = await chromium.launch({ channel: 'msedge', headless: true })
  const problems = []
  const waitMs = Number(process.env.FFC_OFFICE_QA_WAIT_MS || 90000)
  try {
    for (const item of pages) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
      const pageErrors = []
      const sheetLog = []
      page.on('pageerror', error => pageErrors.push(error.stack || error.message))
      page.on('console', msg => { if (msg.type() === 'error') pageErrors.push('console: ' + msg.text()) })
      page.on('response', async response => {
        if (!response.url().includes('/sheets')) return
        const requestText = response.request().postData() || ''
        let bodyText = ''
        try { bodyText = await response.text() } catch { bodyText = '' }
        sheetLog.push(sheetSummary(requestText, response.status(), bodyText))
      })
      if (item.name === 'sheets') {
        await page.addInitScript(() => {
          const proto = CanvasRenderingContext2D.prototype
          if (proto.__ffcFill) return
          proto.__ffcFill = true
          const orig = proto.fillText
          proto.fillText = function (text, x, y, maxWidth) {
            if (text === '官方表格已打开') window.__sawSheetText = true
            return orig.call(this, text, x, y, maxWidth)
          }
        })
      }
      await page.goto(item.url, { waitUntil: 'load', timeout: 90000 })
      await page.evaluate(payload => {
        window.postMessage({ type: 'ffc-office-connect', id: payload.id, token: payload.token, locale: 'zh' }, location.origin)
      }, { id: item.id, token: item.token })
      const textOf = async () => {
        const parts = []
        for (const frame of page.frames()) {
          parts.push(await frame.locator('body').innerText({ timeout: 1000 }).catch(() => ''))
          parts.push(await frame.locator('body').evaluate(body => {
            const bits = []
            for (const el of body.querySelectorAll('input, textarea')) if (el.value) bits.push(el.value)
            const bar = body.querySelector('[data-u-comp="formula-bar"]')
            if (bar) bits.push(bar.textContent || '')
            return bits.join('\n')
          }).catch(() => ''))
        }
        return parts.join('\n')
      }
      const sheetInk = async () => page.evaluate(() => {
        const host = document.querySelector('#univer-container')
        const canvas = [...(host ? host.querySelectorAll('canvas') : [])].sort((a, b) => b.width * b.height - a.width * a.height)[0]
        if (!canvas) return 'no-canvas saw=' + Boolean(window.__sawSheetText)
        const ctx = canvas.getContext('2d', { willReadFrequently: true })
        if (!ctx) return 'no-ctx'
        const rect = canvas.getBoundingClientRect()
        const sx = canvas.width / Math.max(1, rect.width)
        const sy = canvas.height / Math.max(1, rect.height)
        const count = (x, y, w, h) => {
          const data = ctx.getImageData(Math.round(x * sx), Math.round(y * sy), Math.round(w * sx), Math.round(h * sy)).data
          let n = 0
          for (let i = 0; i < data.length; i += 4) if (data[i] < 90 && data[i + 1] < 90 && data[i + 2] < 90 && data[i + 3] > 180) n += 1
          return n
        }
        let cell = 0
        let bands = ''
        try {
          cell = count(50, 26, 180, 48)
          for (let y = 0; y < 240; y += 40) bands += y + ':' + count(48, y, 180, 40) + ' '
        } catch (error) { bands = error.message }
        return 'cell=' + cell + ' ' + bands + 'box=' + Math.round(rect.width) + 'x' + Math.round(rect.height) + ' saw=' + Boolean(window.__sawSheetText)
      }).catch(error => error.message)
      let text = ''
      let ink = ''
      const sheetShown = async () => {
        ink = await sheetInk()
        const cell = Number((ink.match(/cell=(\d+)/) || [])[1] || 0)
        return cell > 30 && ink.includes('saw=true')
      }
      const shown = async () => {
        text = await textOf()
        if (item.name === 'sheets') return sheetShown()
        return text.includes(item.expect)
      }
      const deadline = Date.now() + waitMs
      let ok = false
      while (Date.now() < deadline) {
        ok = await shown()
        if (ok) break
        await page.waitForTimeout(250)
      }
      const banned = ['Genspark AI', 'AI 总结', 'AI 润色', 'AI 排版', 'AI 思维导图', 'AI 要点', 'AI 总结审阅', 'AI 处理便签', 'AI 填表', 'AI 处理批注', 'AI 总结修订', 'AI 校验', 'AI 分析']
      if (ok) {
        for (const name of ['审阅', '批注', '视图']) {
          const tab = page.getByRole('button', { name, exact: true })
          if (await tab.count()) await tab.first().click().catch(() => undefined)
        }
        await page.waitForTimeout(300)
        text = await textOf()
        if (item.name === 'sheets') ok = await sheetShown()
        else ok = text.includes(item.expect)
        const visible = banned.filter(label => text.includes(label))
        if (visible.length) problems.push(item.name + ' still shows built-in AI: ' + visible.join(', '))
      }
      if (!ok) {
        const shot = path.join(os.tmpdir(), 'ffc-office-open-' + item.name + '.png')
        await page.screenshot({ path: shot, fullPage: true }).catch(() => undefined)
        const pixels = item.name === 'sheets' ? await page.evaluate(() => {
          const canvases = [...document.querySelectorAll('canvas')]
          const samples = canvases.slice(0, 6).map(canvas => {
            const rect = canvas.getBoundingClientRect()
            let dark = -1
            try {
              const ctx = canvas.getContext('2d', { willReadFrequently: true })
              if (ctx && canvas.width && canvas.height) {
                dark = 0
                const step = 8
                const data = ctx.getImageData(0, 0, Math.min(canvas.width, 400), Math.min(canvas.height, 200)).data
                const width = Math.min(canvas.width, 400)
                const height = Math.min(canvas.height, 200)
                for (let y = 0; y < height; y += step) {
                  for (let x = 0; x < width; x += step) {
                    const i = (y * width + x) * 4
                    if (data[i] < 80 && data[i + 1] < 80 && data[i + 2] < 80 && data[i + 3] > 200) dark += 1
                  }
                }
              }
            } catch (error) { dark = -1 }
            return Math.round(rect.x) + ',' + Math.round(rect.y) + ' ' + Math.round(rect.width) + 'x' + Math.round(rect.height) + ' bmp=' + canvas.width + 'x' + canvas.height + ' dark=' + dark
          })
          const boxes = ['.app-shell', '.sheet-body', '.sheet-main', '.workbook-area', '#univer-container', '[data-u-comp="formula-bar"]'].map(selector => {
            const node = document.querySelector(selector)
            if (!node) return selector + '=missing'
            const rect = node.getBoundingClientRect()
            return selector + '=' + Math.round(rect.x) + ',' + Math.round(rect.y) + ' ' + Math.round(rect.width) + 'x' + Math.round(rect.height)
          })
          return 'canvases=' + canvases.length + ' ' + samples.join(' | ') + ' || ' + boxes.join(' ; ')
        }).catch(error => 'pixels ' + error.message) : ''
        problems.push(item.name + ' did not show the file\n' + shot + '\n' + text.slice(0, 400) + '\n' + ink + '\n' + sheetLog.slice(-6).join('\n') + '\n' + pixels + '\n' + pageErrors.slice(0, 4).join('\n'))
        await page.close()
        continue
      }
      await page.close()
    }
  } finally {
    await browser.close()
  }
  if (problems.length) {
    fs.writeFileSync(path.join(os.tmpdir(), 'ffc-office-open-fail.txt'), problems.join('\n\n'), 'utf8')
    throw new Error(problems.join('\n\n'))
  }
  console.log('opened ' + pages.map(item => item.name).join(', '))
})().catch(error => {
  console.error(error && error.stack ? error.stack : error)
  process.exit(1)
})
