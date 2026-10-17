// 截图：node shot.mjs [场景...]  （2x，浅暗两版）→ shots/<场景>-{light,dark}.png
// 需要 playwright-core（或 playwright）；默认用系统 Chrome，可用 CHROME=/path 覆盖
import { createRequire } from 'module';
import path from 'path';
import { fileURLToPath } from 'url';
import fs from 'fs';
const require = createRequire(import.meta.url);
let pw;
for (const m of ['playwright-core', 'playwright', process.env.PW_PATH].filter(Boolean)) { try { pw = require(m); break; } catch {} }
if (!pw) { console.error('找不到 playwright-core，请先 npm i playwright-core 或设置 PW_PATH'); process.exit(1); }
const dir = path.dirname(fileURLToPath(import.meta.url));
const ALL = ['list', 'create', 'rolegen', 'main', 'members', 'mention', 'working', 'report', 'error', 'dispatch', 'swap', 'overstep', 'overstep-undo', 'overstep-result', 'session', 'settings', 'narrow-drawer', 'narrow-single'];
const VP = { 'narrow-drawer': { width: 1000, height: 820 }, 'narrow-single': { width: 720, height: 900 } };
const scenes = process.argv.slice(2).length ? process.argv.slice(2) : ALL;
fs.mkdirSync(path.join(dir, 'shots'), { recursive: true });
const browser = await pw.chromium.launch({ executablePath: process.env.CHROME || '/usr/bin/google-chrome', args: ['--no-sandbox', '--font-render-hinting=none'] });
for (const s of scenes) for (const theme of ['light', 'dark']) {
  const page = await browser.newPage({ viewport: VP[s] || { width: 1440, height: 900 }, deviceScaleFactor: 2 });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('file://' + path.join(dir, 'index.html') + `?s=${s}&theme=${theme}&shot=1`);
  await page.waitForTimeout(250);
  const out = path.join(dir, 'shots', `${s}-${theme}.png`);
  await page.screenshot({ path: out });
  await page.close();
  console.log(out);
}
await browser.close();
