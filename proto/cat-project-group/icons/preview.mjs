import fs from 'fs'; import path from 'path'; import { createRequire } from 'module';
const require = createRequire('/workspace/design-tools/');
const pw = require('playwright-core');
const dir = '/workspace/design-ffc/proto/cat-project-group/icons';
const ts = fs.readFileSync('/workspace/_fe4da/frontend/src/components/icon/icons.ts','utf8');
const get = n => { const m = ts.match(new RegExp(`'${n}':\\s*(["'])((?:\\\\.|(?!\\1).)*)\\1`)); return m ? m[2].replace(/\\"/g,'"') : ''; };
const inner = f => fs.readFileSync(path.join(dir,f),'utf8').replace(/^[\s\S]*?<svg[^>]*>/,'').replace(/<\/svg>\s*$/,'');
const NEW = [['group','群',inner('group.svg')],['swap','换人',inner('swap.svg')],['dispatch','分派',inner('dispatch.svg')]];
const OLD = [['chat','（临时·群）'],['convert','（临时·换人）'],['branch','（临时·分派）']].map(([n,l])=>[n,l,get(n)]);
const CTX = ['stop','refresh','warn','info','edit','plus'].map(n=>[n,n,get(n)]);
const svg = (p,s,sw) => `<svg viewBox="0 0 24 24" width="${s}" height="${s}" fill="none" stroke="currentColor" stroke-width="${sw}" stroke-linecap="round" stroke-linejoin="round">${p}</svg>`;
const row = (list, s, sw) => list.map(([n,l,p])=>`<div class="c">${svg(p,s,sw)}<span>${l}</span></div>`).join('');
const panel = (th) => `<section class="${th}"><h2>${th==='light'?'浅色':'暗色'}</h2>
${[16,20].map(s=>`<h3>${s}px</h3><div class="r"><b>正式</b>${row(NEW,s,1.75)}</div><div class="r"><b>前端临时</b>${row(OLD,s,1.8)}</div><div class="r"><b>现有并排</b>${row(NEW.slice(0,1),s,1.75)}${row(CTX,s,1.8)}${row(NEW.slice(1),s,1.75)}</div>`).join('')}
<h3>场景</h3>
<div class="ctx"><span class="o">${svg(NEW[0][2],14,1.75)}</span> v0.55 群聊开发 <i class="av a1">架</i><i class="av a2">后</i><i class="av a3">前</i></div>
<div class="ctx g">${svg(NEW[2][2],14,1.75)} 交给 架构师：先出方案再拆给前后端 <span class="act">${svg(NEW[1][2],14,1.75)}换人</span></div>
<div class="ctx g">${svg(NEW[2][2],14,1.75)} 已转交给 测试（第 2/6 次）</div>
<h3>48px 放大</h3><div class="r big">${row(NEW,48,1.75)}</div></section>`;
const html = `<!doctype html><meta charset=utf8><style>
body{margin:0;font:13px/1.5 "Noto Sans CJK SC","PingFang SC",sans-serif;display:flex}
section{flex:1;padding:20px 28px}
.light{background:#fff;color:#1f2329;--t2:#646a73;--o:#e8710a;--a:#3370ff;--b:#e5e6eb}
.dark{background:#1b1c1f;color:#e6e7ea;--t2:#a2a6ad;--o:#fb923c;--a:#6b95ff;--b:#2e3036}
h2{margin:0 0 6px;font-size:15px} h3{margin:16px 0 6px;font-size:12px;color:var(--t2);font-weight:500}
.r{display:flex;align-items:center;gap:6px;margin:4px 0} .r b{width:64px;font-weight:500;font-size:12px;color:var(--t2)}
.c{display:flex;align-items:center;gap:5px;padding:4px 8px;border:1px solid var(--b);border-radius:6px} .c span{white-space:nowrap;font-size:11px;color:var(--t2)}
.big .c{flex-direction:column;padding:10px 16px}
.ctx{display:flex;align-items:center;gap:6px;margin:6px 0;font-size:13px}.ctx.g{font-size:12px;color:var(--t2)}
.o{color:var(--o);display:inline-flex}.act{color:var(--a);display:inline-flex;align-items:center;gap:3px}
.av{font-style:normal;width:18px;height:18px;border-radius:50%;color:#fff;font-size:10px;display:inline-flex;align-items:center;justify-content:center;margin-left:-6px}.av.a1{margin-left:4px;background:#3370ff}.av.a2{background:#0f9d6c}.av.a3{background:#c2570c}
</style><body>${panel('light')}${panel('dark')}</body>`;
fs.writeFileSync('/workspace/_review/icons-preview.html', html);
const b = await pw.chromium.launch({ executablePath:'/usr/bin/google-chrome', args:['--no-sandbox'] });
const p = await b.newPage({ viewport:{ width:1500, height:600 }, deviceScaleFactor:2 });
await p.goto('file:///workspace/_review/icons-preview.html');
await p.screenshot({ path: path.join(dir,'icons-preview.png'), fullPage:true });
await b.close(); console.log('ok');
