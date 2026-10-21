/**
 * 自适应自检：在多个窗口宽度下打开每个页面，检查有没有元素横向溢出主内容区。
 *
 * 做法：把探针脚本注进 dist/index.html 的临时副本（dist/probe.html），
 * 用 Chrome headless --dump-dom 跑一遍，读出脚本写进 #ffcprobe 的结果。
 *
 * 依赖：先 `node node_modules/vite/bin/vite.js build`，再起 vite preview（默认 5290）。
 * 用法：node scripts/check-responsive.mjs [baseUrl]
 */
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { tmpdir } from 'node:os'

const here = dirname(fileURLToPath(import.meta.url))
const dist = resolve(here, '..', 'dist')
const BASE = process.argv[2] || 'http://127.0.0.1:5290'

const CHROME_CANDIDATES = [
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Microsoft/Edge/Application/msedge.exe',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
]
const chrome = CHROME_CANDIDATES.find((p) => existsSync(p))
if (!chrome) {
  console.error('找不到 Chrome / Edge，无法做自适应自检')
  process.exit(2)
}

const PAGES = [
  ['convert', '#/'],
  ['voice', '#/voice'],
  ['docs', '#/docs/office'],
  ['pdf', '#/docs/pdf'],
  ['live-push', '#/live/push'],
  ['live-record', '#/live/record'],
  ['live-pull', '#/live/pull'],
  ['json', '#/tools/json'],
  ['tasks', '#/tasks'],
  ['settings', '#/settings/general'],
  ['about', '#/settings/about'],
]
// 720 是窗口最小宽度；760/900 覆盖单列断点两侧；1024/1280 是常见工作宽度
const WIDTHS = [720, 760, 900, 1024, 1280]

const PROBE = `
<div id="ffcprobe" style="display:none"></div>
<script>
(function(){
  function run(){
    var out=['win='+innerWidth+'x'+innerHeight];
    var root=document.querySelector('.app-content')||document.body;
    var els=root.querySelectorAll('*');
    for(var i=0;i<els.length;i++){
      var e=els[i], r=e.getBoundingClientRect();
      if(r.width<=0||r.height<=0) continue;
      if(e.closest('[class*=monaco]')) continue;           /* 编辑器内部节点与视口无关 */
      if(r.right<=innerWidth+1) continue;
      var cs=getComputedStyle(e);
      if(cs.position==='fixed') continue;
      if(cs.overflowX==='auto'||cs.overflowX==='scroll') continue; /* 自己可横向滚的不算溢出 */
      var cls=(e.className&&e.className.toString?e.className.toString().split(/\\s+/).slice(0,2).join('.'):'');
      out.push(e.tagName.toLowerCase()+'.'+cls+' right='+Math.round(r.right)+' w='+Math.round(r.width));
      if(out.length>12) break;
    }
    document.getElementById('ffcprobe').textContent='DIAG::'+out.join(' || ');
  }
  setTimeout(run, 2600);
})();
</script>`

const indexHtml = readFileSync(resolve(dist, 'index.html'), 'utf8')
const probeFile = resolve(dist, 'probe.html')
writeFileSync(probeFile, indexHtml.replace('</body>', `${PROBE}\n</body>`), 'utf8')

let bad = 0
for (const w of WIDTHS) {
  for (const [name, hash] of PAGES) {
    const profile = resolve(tmpdir(), `ffc-probe-${process.pid}-${w}-${name}`)
    let dom = ''
    try {
      dom = execFileSync(
        chrome,
        [
          '--headless=new', '--disable-gpu', '--no-first-run', '--hide-scrollbars',
          `--window-size=${w},860`, '--virtual-time-budget=5000',
          `--user-data-dir=${profile}`, '--dump-dom', `${BASE}/probe.html${hash}`,
        ],
        { encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'], maxBuffer: 64 * 1024 * 1024 },
      )
    } catch {
      dom = ''
    }
    const m = dom.match(/DIAG::([^<]*)</)
    if (!m) {
      console.log(`${String(w).padEnd(5)} ${name.padEnd(12)} 读取不到探针结果（页面没起来？）`)
      bad++
      continue
    }
    const parts = m[1].split(' || ')
    const win = (parts[0] || '').replace('win=', '')
    const over = parts.slice(1)
    if (!over.length) {
      console.log(`${String(w).padEnd(5)} ${name.padEnd(12)} OK  (${win})`)
    } else {
      bad++
      console.log(`${String(w).padEnd(5)} ${name.padEnd(12)} 溢出 ${over.length} 处 (${win})`)
      for (const o of over.slice(0, 4)) console.log(`        ${o}`)
    }
  }
}

try {
  unlinkSync(probeFile)
} catch {
  /* ignore */
}

console.log(bad === 0 ? '\n自适应自检通过：所有页面在各宽度下都没有横向溢出' : `\n自适应自检失败：${bad} 项`)
process.exit(bad === 0 ? 0 : 1)
