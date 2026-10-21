// 一致性自检：页签文案必须能命中 nav.js 的 TAB_MAP；页面里的相对链接必须存在
import { readFile, access } from 'node:fs/promises';
import { join, dirname, resolve } from 'node:path';

const ROOT = resolve(import.meta.dirname);
const nav = await readFile(join(ROOT, 'assets/nav.js'), 'utf8');

// 取出 TAB_MAP 的键
const mapBlock = nav.match(/const TAB_MAP = \{([\s\S]*?)\};/);
const keys = [...mapBlock[1].matchAll(/'([^']+)'\s*:/g)].map((m) => m[1]);
console.log('TAB_MAP keys:', keys.join(' / '));

const pages = ['01-convert', '02-voice', '03-docs-convert', '04-docs-pdf', '05-live-push',
  '06-live-record', '07-live-pull', '08-tools-json', '09-tasks', '10-settings', '11-about'];

let problems = 0;
for (const name of pages) {
  const file = join(ROOT, 'pages', `${name}.html`);
  const html = await readFile(file, 'utf8');

  // 1) 页签文案（09 任务中心是筛选页签，不参与导航映射）
  const tabArea = name === '09-tasks' ? null : html.match(/<div class="c-tabs">([\s\S]*?)<\/div>/);
  if (tabArea) {
    const labels = [...tabArea[1].matchAll(/<button[^>]*>([\s\S]*?)<\/button>/g)]
      .map((m) => m[1].replace(/<[^>]+>/g, '').replace(/\s+/g, '').trim())
      .filter(Boolean);
    const hit = labels.map((l) => `${l}${keys.includes(l) ? '✓' : '✗'}`);
    console.log(`${name.padEnd(16)} tabs:`, hit.join(' '));
    problems += hit.filter((s) => s.endsWith('✗')).length;
  }

  // 2) 相对链接逐个存在性
  const hrefs = [...html.matchAll(/href="([^"#][^"]*)"/g)].map((m) => m[1])
    .filter((h) => !h.startsWith('http') && !h.startsWith('?'));
  for (const h of new Set(hrefs)) {
    const target = join(dirname(file), h.split('?')[0]);
    try { await access(target); } catch { console.log(`  MISSING ${name} -> ${h}`); problems++; }
  }
}

console.log(problems === 0 ? 'OK: 全部页签命中，链接无断链' : `问题 ${problems} 处`);
process.exit(problems === 0 ? 0 : 1);
