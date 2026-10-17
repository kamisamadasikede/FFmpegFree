/* Cat 项目群聊 高保真原型 v0.2.1（对齐契约 6.21.12–6.21.16）
   用法：index.html?s=<场景>&theme=dark[&shot=1]
   场景：list create rolegen main members mention working report error dispatch swap overstep overstep-undo overstep-result session settings narrow-drawer narrow-single */
const Q = new URLSearchParams(location.search);
const S = Q.get('s') || 'main';
if (Q.get('theme') === 'dark') document.documentElement.classList.add('dark');
if (Q.get('shot')) document.documentElement.classList.add('shot');

const P = {
  users:'<path d="M16 19v-1.5a3.5 3.5 0 0 0-3.5-3.5h-5A3.5 3.5 0 0 0 4 17.5V19"/><circle cx="10" cy="7.5" r="3.5"/><path d="M20 19v-1.5a3.5 3.5 0 0 0-2.5-3.35M15.5 4.2a3.5 3.5 0 0 1 0 6.6"/>',
  plus:'<path d="M12 5v14M5 12h14"/>',
  folder:'<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  chat:'<path d="M5 5h14v10H9l-4 4z"/>',
  chevD:'<path d="m6 9 6 6 6-6"/>', chevR:'<path d="m9 6 6 6-6 6"/>',
  at:'<circle cx="12" cy="12" r="4"/><path d="M16 8v5a3 3 0 0 0 6 0v-1a10 10 0 1 0-4 8"/>',
  send:'<path d="M12 19V5M6 11l6-6 6 6"/>',
  stop:'<rect x="7" y="7" width="10" height="10" rx="1.5"/>',
  grip:'<circle cx="9" cy="6" r=".9"/><circle cx="15" cy="6" r=".9"/><circle cx="9" cy="12" r=".9"/><circle cx="15" cy="12" r=".9"/><circle cx="9" cy="18" r=".9"/><circle cx="15" cy="18" r=".9"/>',
  file:'<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  warn:'<path d="M10.3 3.9 2.6 17.5A2 2 0 0 0 4.3 20.5h15.4a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/><path d="M12 9v4M12 17h.01"/>',
  check:'<path d="m5 12 5 5 9-10"/>', x:'<path d="M6 6l12 12M18 6 6 18"/>',
  swap:'<path d="M7 7h12l-3-3M17 17H5l3 3"/>',
  refresh:'<path d="M20 11a8 8 0 0 0-14.3-4.9L4 8M4 13a8 8 0 0 0 14.3 4.9L20 16"/><path d="M4 4v4h4M20 20v-4h-4"/>',
  edit:'<path d="M4 20h4L19 9l-4-4L4 16z"/>', eye:'<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  gear:'<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z"/>',
  menu:'<path d="M4 6h16M4 12h16M4 18h16"/>', info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  route:'<path d="M5 12h12M13 6l6 6-6 6"/>', panel:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M15 4v16"/>',
  term:'<path d="m5 8 4 4-4 4M12 16h7"/>', search:'<circle cx="11" cy="11" r="6"/><path d="m20 20-4-4"/>',
  trash:'<path d="M4 7h16M10 11v6M14 11v6M6 7l1 12a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-12M9 7V4h6v3"/>',
  sparkle:'<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z"/>',
  undo:'<path d="M9 14 4 9l5-5"/><path d="M4 9h11a5 5 0 0 1 0 10h-3"/>',
};
const ic = (n, c = '') => `<svg class="i ${c}" viewBox="0 0 24 24">${P[n]}</svg>`;
const CLI = { g: ['Grok Build', 'G', 'cli-g'], c: ['Claude', 'C', 'cli-c'], x: ['Codex', 'X', 'cli-x'] };
const M = {
  a: { n: '架构师', t: '架构师', cli: 'c', model: 'Claude Opus 4.5', c: 1 },
  b: { n: '后端工程师', t: '后端工程师', cli: 'x', model: 'gpt-5.2-codex', c: 2 },
  f: { n: '前端工程师', t: '前端工程师', cli: 'g', model: 'grok-code-fast', c: 3 },
  q: { n: '测试', t: '测试工程师', cli: 'c', model: 'Claude Sonnet 4.5', c: 4 },
};
const ORDER = ['a', 'b', 'f', 'q'];
const av = (k, cls = '') => `<div class="av m${M[k].c} ${cls}">${M[k].n[0]}</div>`;
const cliIco = k => `<span class="cliico ${CLI[k][2]}">${CLI[k][1]}</span>`;
const cliChip = k => `<span class="chip-cli">${cliIco(k)}${CLI[k][0]}</span>`;
const at = k => `<span class="at">@${M[k].n}</span>`;
const atchip = k => `<span class="atchip">${av(k)}@${M[k].n}</span>`;

/* ---------- 左栏 ---------- */
function rail(o = {}) {
  const g = o.group || 'dev';
  const item = (id, name, members, dot) => `<div class="ritem ${g === id ? 'on' : ''}">${ic('users', 's gi')}<span class="nm">${name}</span>${dot ? `<span class="dot ${dot}"></span>` : ''}<span class="stack">${members.slice(0, 3).map(k => av(k)).join('')}${members.length > 3 ? `<span class="more">+${members.length - 3}</span>` : ''}</span></div>`;
  return `<aside class="rail">
  <div class="rail-top"><span class="logo">C</span>Cat<span class="sp"></span><button class="ibtn">${ic('search', 's')}</button></div>
  <button class="rail-new">${ic('plus', 's')}新对话</button>
  <div class="rail-sec">
    <div class="proj">
      <div class="proj-h">${ic('chevD', 'xs')}${ic('folder', 's')}FFmpegFree</div>
      <div class="grp-h"><span>群聊 · 2</span><button class="plus ${o.plusHl ? 'hl' : ''}" title="新建群聊">${ic('plus', 's')}</button>${o.plusHl ? '<div class="tip" style="right:-4px;top:30px">新建群聊</div>' : ''}</div>
      ${item('dev', 'v0.55 群聊开发', ORDER, o.devDot || 'busy')}
      ${item('rel', '发版检查', ['a', 'q'], o.relDot || '')}
      <div class="rail-sub">会话</div>
      <div class="ritem">${ic('chat', 's sess')}<span class="nm">修 Codex 登录检测</span></div>
      <div class="ritem">${ic('chat', 's sess')}<span class="nm">直播页暗色对比度</span></div>
    </div>
    <div class="proj">
      <div class="proj-h">${ic('chevD', 'xs')}${ic('folder', 's')}ai-canvas</div>
      <div class="grp-h"><span>群聊 · 1</span><button class="plus">${ic('plus', 's')}</button></div>
      <div class="ritem">${ic('users', 's gi')}<span class="nm">画布重构</span><span class="dot red"></span><span class="stack">${av('a')}${av('f')}</span></div>
      <div class="ritem">${ic('users', 's gi')}<span class="nm">导出 PDF</span><span class="dot org"></span><span class="stack">${av('b')}${av('f')}</span></div>
    </div>
    <div class="proj"><div class="proj-h">${ic('chevR', 'xs')}${ic('folder', 's')}notes-sync</div></div>
  </div></aside>`;
}

/* ---------- 中栏 ---------- */
function chead(o = {}) {
  return `<div class="chead">
  <button class="ibtn hamb">${ic('menu')}</button>
  ${ic('users', 's')}<span class="t">${o.title || 'v0.55 群聊开发'}</span><span class="sub">FFmpegFree · ${o.count || 4} 位成员</span>
  <span class="sp"></span>
  <span class="stack">${ORDER.map(k => av(k)).join('')}</span>
  ${o.memBtn ? `<button class="ibtn bd ${o.memOn ? 'hl' : ''}">${ic('users', 's')}成员</button>` : ''}
  <button class="ibtn ${o.setOn ? 'hl' : ''}" title="群设置">${ic('gear', 's')}</button>
  </div>`;
}
const task = (html, tm = '17:08', extra = '') => `<div class="task"><div class="task-h"><div class="av u">何</div><b>何こと優しい 何</b><span class="tg">任务</span>${extra}<span class="tm">${tm}</span></div><p>${html}</p></div>`;
const gl = (html, o = {}) => `<div class="gl ${o.cls || ''}">${o.spin ? '<span class="spin"></span>' : ic(o.icon || 'route', 'xs')}<span class="${o.lk ? 'lk' : ''}">${html}</span>${o.tmr ? `<span class="tmr">${o.tmr}</span>` : ''}${(o.acts || []).join('')}${o.after || ''}</div>`;
const swapBtn = (hl) => `<button class="act ${hl ? 'hl' : ''}">${ic('swap', 'xs')}换人</button>`;

function fileRows(files, o = {}) {
  return files.map(f => {
    const nm = f.p.split('/'), base = nm.pop();
    const tag = f.tag === 'ov' ? '<span class="tag ov">越界</span>' : f.tag === 'unk' ? '<span class="tag unk">来源不明</span>' : f.tag === 'rev' ? '<span class="tag ok">已撤销</span>' : f.tag === 'conf' ? '<span class="tag bad">没撤销</span>' : '';
    const cls = f.tag === 'ov' ? 'ov' : f.tag === 'conf' ? 'conf' : f.tag === 'rev' ? 'rev' : '';
    return `<div class="frow ${cls}">${ic('file', 'xs fi')}<span class="p"><span class="d">${nm.join('/')}${nm.length ? '/' : ''}</span>${base}</span>${f.a ? `<span class="add">+${f.a}</span>` : ''}${f.d ? `<span class="del">−${f.d}</span>` : ''}${tag}${f.tip ? `<div class="tip" style="right:0;top:32px">这段时间有多人在干活，不确定是谁改的</div>` : ''}</div>`;
  }).join('');
}
function report(o) {
  const m = M[o.m];
  const files = o.files || [];
  const A = files.reduce((s, f) => s + (f.a || 0), 0), D = files.reduce((s, f) => s + (f.d || 0), 0);
  let h = `<div class="rep ${o.over ? 'over' : ''}">
  <div class="rep-h">${av(o.m)}<span class="nm">${m.n}</span><span class="ttl">${m.t}</span>${cliChip(m.cli)}<span class="tm">${o.tm || '17:20'}</span></div>
  <div class="sec f"><div class="sec-t">完成了什么</div><div class="sec-b"><p>${o.did}</p></div></div>`;
  if (o.parsed === false) {
    h += `<div class="note">${ic('info', 'xs')}没按汇报格式写，只显示最后一段回复</div>`;
  } else {
    if (files.length) {
      h += `<div class="sec"><div class="sec-t">改了哪些文件</div>
      <div class="files-h">${ic(o.open ? 'chevD' : 'chevR', 'xs')}<span class="fc">${files.length} 个文件</span><span class="add">+${A}</span><span class="del">−${D}</span>${o.over ? `<span class="tag ov">${o.over.n} 个越界</span>` : ''}</div>
      ${o.open ? `<div class="flist">${fileRows(files)}</div>` : ''}</div>`;
    }
    if (o.ovbox) h += o.ovbox;
    if (o.attn) h += `<div class="sec attn"><div class="sec-t">${ic('warn', 'xs')}需要注意</div><div class="sec-b"><p>${o.attn}</p></div></div>`;
    if (o.give && o.give.length) h += `<div class="sec"><div class="sec-t">交给谁</div><div class="sec-b">${o.give.map(g => `<div class="give">${atchip(g[0])}<span>${g[1]}</span></div>`).join('')}</div></div>`;
  }
  const f = o.foot || ['17:20', '3.2k', '41'];
  h += `<div class="rep-f">完成于 ${f[0]}<span>·</span>输出 ${f[1]} token<span>·</span>${f[2]} token/s<span class="sp"></span><button class="lnk">${ic('eye', 'xs')}查看完整过程</button></div></div>`;
  return h;
}

/* 主线数据 */
const T1 = '群聊失败灰字旁边加「重试」：同一任务再跑一轮，hop 不加。先出方案，再分给前后端。';
const R_ARCH = (o = {}) => report({ m: 'a', tm: '17:12', did: '重试方案定了：新增绑定 <code>RetryCatGroupMemberTurn</code>，按同一 chainId 把该成员的任务再跑一轮，hop 不加；失败灰字旁加「重试」按钮。已写进契约 6.21.15。', files: [{ p: 'docs/architecture/contract.md', a: 38, d: 2 }], give: [['b', '实现绑定和单测，失败/停止后才允许重试'], ['f', '失败灰字旁接「重试」按钮，点了乐观显示「正在安排…」']], foot: ['17:12', '2.1k', '38'], ...o });
const R_BE = (o = {}) => report({ m: 'b', tm: '17:19', did: '<code>RetryCatGroupMemberTurn</code> 做好了：校验该成员本链最后一轮是失败或停止，复用原任务再跑一轮，hop 不变。单测 9 个全过。', files: [{ p: 'internal/catagent/group_retry.go', a: 86 }, { p: 'internal/catagent/group_retry_test.go', a: 124 }, { p: 'app_cat_group.go', a: 14, d: 2 }], attn: '成员已被移出群时返回 <code>member_not_found</code>，前端按现有错误文案显示即可。', give: [['q', '跑 6.21.11 ④ 的重试用例，三家各一次']], foot: ['17:19', '4.6k', '52'], ...o });

function composer(o = {}) {
  let wb = '';
  if (o.working && o.working.length) {
    wb = `<div class="wbar">${o.working.map(w => `<div class="wrow"><span class="pulse"></span>${av(w[0])}<span class="tx"><b>${M[w[0]].n}</b> 正在干活 · 已用 ${w[1]}${w[2] ? ` · ${w[2]} 个工具` : ''}</span><button class="lnkbtn ${w[3] === 'view' ? 'hl' : ''}">查看过程</button><button class="lnkbtn stop">停止</button></div>`).join('')}
    ${o.working.length > 1 ? `<div class="wsum">${o.working.length} 人在干活<span class="sp"></span><button class="lnkbtn dng">全部停止</button></div>` : ''}</div>`;
  }
  const inner = o.text || '<span class="ph">给群里派活，@ 成员直接交给他；不 @ 时自动安排</span>';
  return `<div class="bottom"><div class="col">${o.pop || ''}${wb}
  <div class="comp ${o.focus ? 'focus' : ''}"><div class="in">${inner}</div>
  <div class="tools"><button class="ibtn" title="添加文件">${ic('plus')}</button><button class="ibtn ${o.atHl ? 'hl' : ''}" title="@ 成员">${ic('at')}</button><span class="sp"></span><button class="send ${o.text ? '' : 'off'}">${ic('send', 's')}</button></div></div>
  ${o.hint ? `<div class="hint">${o.hint}</div>` : ''}</div></div>`;
}
const centerWrap = (head, body, bottom, extra = '') => `<main class="center">${head}<div class="stream"><div class="col">${body}</div></div>${bottom}${extra}</main>`;

/* ---------- 右栏 ---------- */
const ST = { idle: '<span class="st idle">空闲</span>', run: '<span class="st run"><span class="pulse"></span>正在干活</span>', q: '<span class="st q">排队中</span>', ov: '<span class="st ov">越界待处理</span>', stop: '<span class="st stop">已停止</span>', fail: '<span class="st fail">没完成</span>' };
function memRow(k, st, o = {}) {
  const m = M[k];
  return `<div class="mem ${o.sel ? 'sel' : ''} ${o.drag ? 'drag' : ''}">${ic('grip', 's grip')}<div class="av m${m.c}">${m.n[0]}${st === 'ov' ? '<span class="od"></span>' : ''}</div>
  <div class="info"><div class="l1">${m.n}${o.first ? '<span class="first">排第一</span>' : ''}</div><div class="l2">${m.t} · ${CLI[m.cli][0]}</div></div>
  <div class="mst">${ST[st]}${st === 'run' || st === 'q' ? `<button class="stopb" title="停止">${ic('stop', 'xs')}</button>` : ''}</div></div>`;
}
function charter(k, o = {}) {
  const m = M[k];
  const head = `<div class="ch-h">${av(k)}<span class="nm">${m.n}</span><span class="muted" style="font-size:12px">职责规范</span><span class="sp"></span>${o.state === 'ok' || !o.state ? `<button class="btn ghost sm">${ic('edit', 'xs')}编辑</button><button class="btn ghost sm">${ic('refresh', 'xs')}重新生成</button>` : ''}</div>`;
  if (o.state === 'gen') return `<div class="ch">${head}<div class="genning"><span class="spin"></span>正在生成职责…</div><div class="skel" style="width:92%"></div><div class="skel" style="width:76%"></div><div class="skel" style="width:58%"></div></div>`;
  if (o.state === 'fail') return `<div class="ch">${head}<div class="chfail">${ic('info', 'xs')}职责还没生成<span class="sp"></span><button class="btn sm">${ic('refresh', 'xs')}重新生成</button></div><div class="info-line" style="margin-top:4px">成员照样可以干活；没有职责时不做越界检查。</div></div>`;
  const C = CH[k];
  const ed = s => (C.edited || []).includes(s) ? '<span class="edited">已手动修改</span>' : '';
  const row = (lbl, key, v) => `<div class="ch-r"><div class="k">${lbl}${ed(key)}</div><div class="v">${v}</div></div>`;
  return `<div class="ch">${head}
  ${row('负责', 'owns', C.owns)}${row('不碰', 'notTouch', C.notTouch)}
  ${row('负责的目录', 'dirs', C.dirs.length ? C.dirs.map(d => `<span class="dirchip">${d}</span>`).join('') : '<span class="muted">没写目录，不做越界检查</span>')}
  ${row('做完 @', 'handoff', C.handoff)}${row('交付物', 'deliver', C.deliver)}${row('完成标准', 'done', C.done)}</div>`;
}
const CH = {
  a: { owns: '契约与技术方案，拆任务、定接口和验收口径', notTouch: '不写业务代码，不改前端页面', dirs: [], handoff: `方案定了 → ${at('b')} ${at('f')}`, deliver: '契约章节（docs/architecture/contract.md）', done: '接口、事件、文案都写死，前后端无需再猜' },
  b: { owns: 'Go 后端：群聊存储、派活、汇报解析、越界检查', notTouch: '不改 frontend/ 和契约', dirs: ['internal/catagent/', 'internal/store/', 'app_cat_*.go'], handoff: `接口好了 → ${at('f')}；要验证 → ${at('q')}`, deliver: '实现 + 单测 + 绑定', done: 'go test ./... 通过，拼参有单测', edited: ['dirs'] },
  f: { owns: 'Vue 页面：群消息流、汇报卡、干活条、查看过程', notTouch: '不改 Go 代码和契约', dirs: ['frontend/src/'], handoff: `页面好了 → ${at('q')}`, deliver: '页面 + 多语言 key', done: '浅暗两版对设计稿，vue-tsc 通过' },
  q: { owns: '按 6.21.11 跑验收用例，报缺陷', notTouch: '不改业务代码', dirs: ['tests/'], handoff: `有缺陷 → 对应负责人`, deliver: '用例结果与复现步骤', done: '6.21.11 七项都跑到' },
};
function right(o = {}) {
  const st = o.st || { a: 'idle', b: 'idle', f: 'run', q: 'q' };
  let body = '';
  const order = o.order || ORDER;
  order.forEach((k, i) => {
    if (o.dropAt === i) body += '<div class="dropline"></div>';
    body += memRow(k, st[k], { first: i === 0, sel: o.sel === k, drag: o.drag === k });
    if (o.sel === k && o.charter) body += charter(k, { state: o.charterState });
  });
  return `<aside class="right ${o.cls || ''}"><div class="tabs"><div class="tab on">成员<span class="cnt">${order.length}</span></div><div class="tab">文件<span class="cnt">7</span></div><span class="sp"></span><button class="ibtn" title="添加成员">${ic('plus', 's')}</button></div>
  <div class="rbody">${body}</div>
  <div class="rfoot">${ic('info', 'xs')}<span>拖动调整顺序。排第一的成员负责兜底：没判断出该谁接、或关了自动派活时交给他。</span></div></aside>`;
}

/* ---------- 弹窗 ---------- */
const modal = (title, sub, body, foot, cls = '') => `<div class="scrim"><div class="modal ${cls}"><div class="mh">${title}<span class="sp"></span><button class="ibtn">${ic('x', 's')}</button></div>${sub ? `<div class="msub">${sub}</div>` : ''}<div class="mb">${body}</div><div class="mf">${foot}</div></div></div>`;

function createModal() {
  const row = (k, cli, title, name, model, o = {}) => `<div class="mrow">${ic('grip', 's grip')}${k ? av(k) : '<div class="av" style="background:var(--cat-sunken);color:var(--cat-muted)">?</div>'}
  <div class="inp">${cliIco(cli)}${CLI[cli][0]}<span class="ca">${ic('chevD', 'xs')}</span></div>
  <div class="inp">${title}</div>
  <div class="inp"><span class="${model ? '' : 'ph'}">${model || '默认模型'}</span><span class="ca">${ic('chevD', 'xs')}</span></div>
  <div><div class="inp ${o.err ? 'err foc' : ''}"><span class="muted">@</span>${name}${o.err ? '<span class="caret"></span>' : ''}</div>${o.err ? `<div class="errmsg">群里已经有叫这个名字的成员了</div><div class="sugg">试试 <b>后端工程师 2</b></div>` : ''}</div>
  <button class="del">${ic('x', 's')}</button></div>`;
  const body = `<div class="lbl">群名</div><div class="inp" style="height:36px;font-size:13.5px">v0.55 群聊开发</div>
  <div class="lbl" style="margin-top:16px">成员<span class="muted">· 拖动排序，排第一的成员负责兜底</span></div>
  <div class="mrows">
    <div class="mrow hd"><span></span><span></span><span>CLI</span><span>职位</span><span>模型（可选）</span><span>群里叫什么</span><span></span></div>
    ${row('a', 'c', '架构师', '架构师', 'Claude Opus 4.5')}
    ${row('b', 'x', '后端工程师', '后端工程师', '')}
    ${row('', 'x', '后端工程师', '后端工程师', '', { err: 1 })}
    ${row('f', 'g', '前端工程师', '前端工程师', 'grok-code-fast')}
    <div class="addrow">${ic('plus', 's')}添加成员</div>
  </div>
  <div class="info-line">${ic('info', 'xs')}<span>所有成员共用项目文件夹 <span style="font-family:var(--ff-font-mono)">~/code/FFmpegFree</span>，不设默认协调者。创建后 Cat 会用每位成员自己的 CLI 生成职责规范，期间成员照样可以干活。</span></div>`;
  return modal('新建群聊', '在「FFmpegFree」下建一个群，成员各用一家 CLI，Cat 负责转交消息。', body, `<span class="lft">4 位成员</span><button class="btn">取消</button><button class="btn pri dis">创建</button>`);
}
function rolegenModal() {
  const body = charter('a') + charter('b') + charter('f', { state: 'gen' }) + charter('q', { state: 'fail' });
  return modal('职责规范', 'Cat 按职位和项目情况自动生成，每轮都会带给成员。手改过的段落不会被自动覆盖；加人、减人只更新「做完 @」。', body, `<span class="lft">3 / 4 已生成</span><button class="btn pri">好的</button>`);
}
function settingsModal() {
  const body = `<div class="lbl">群名</div><div class="inp" style="height:36px">v0.55 群聊开发</div>
  <div class="lbl" style="margin-top:16px">成员排序<span class="muted">· 排第一的成员负责兜底</span></div>
  ${ORDER.map((k, i) => `<div class="smem">${ic('grip', 's grip')}${av(k)}<span>${M[k].n}</span><span class="muted">${M[k].t} · ${CLI[M[k].cli][0]}</span><span class="sp"></span>${i === 0 ? '<span class="first">排第一</span>' : ''}</div>`).join('')}
  <div style="margin-top:12px">
  <div class="setrow"><div class="tx">没 @ 人时自动派活<small>开：Cat 用排第一成员的 CLI 判断一次该谁接（约 20 秒内）。关：直接交给排第一的成员。</small></div><span class="switch"></span></div>
  <div class="setrow"><div class="tx">删除群聊<small>先停下群里所有人的活；成员自己的会话不会删除。</small></div><button class="btn sm" style="color:var(--cat-danger-text)">${ic('trash', 'xs')}删除</button></div></div>`;
  return modal('群设置', '', body, `<button class="btn">取消</button><button class="btn pri">保存</button>`);
}

/* ---------- 场景 ---------- */
const mainStream = (o = {}) => `<div class="daysep">今天</div>
  ${task(T1, '17:08')}
  ${gl('交给 架构师：先出方案再拆给前后端', { lk: 1, acts: [swapBtn()] })}
  ${R_ARCH(o.arch)}
  ${gl('已转交给 后端工程师（第 1/6 次）', { icon: 'route' })}
  ${gl('已转交给 前端工程师（第 1/6 次）', { icon: 'route' })}
  ${R_BE(o.be)}
  ${gl('已转交给 测试（第 2/6 次）', { icon: 'route' })}
  ${o.tail || ''}`;
const WORK = [['f', '2 分 13 秒', 14], ['q', '38 秒', 0]];

const ovFiles = [{ p: 'frontend/src/views/cat/GroupStream.vue', a: 64, d: 8 }, { p: 'frontend/src/views/cat/RetryButton.vue', a: 41 }, { p: 'internal/catagent/group_retry.go', a: 6, d: 2, tag: 'ov' }, { p: 'app_cat_group.go', a: 3, d: 1, tag: 'ov' }, { p: 'frontend/src/i18n/ja.json', a: 2, tag: 'unk' }];
function feReport(o = {}) {
  const ovRows = o.result ? [{ p: 'internal/catagent/group_retry.go', a: 6, d: 2, tag: 'rev' }, { p: 'app_cat_group.go', a: 3, d: 1, tag: 'conf' }] : ovFiles.filter(f => f.tag === 'ov');
  const ovbox = `<div class="ovbox"><div class="ovbox-h">${ic('warn', 's')}改了不属于自己的文件（2 个）${ic('chevD', 'xs')}<span class="sp"></span>${o.result ? '<button class="btn sm dis">撤销这些改动</button>' : '<button class="btn sm org">' + ic('undo', 'xs') + '撤销这些改动</button>'}</div>
    <div class="flist">${fileRows(ovRows)}</div>
    ${o.result ? `<div class="errt">${ic('warn', 'xs')}<span>app_cat_group.go 之后又被改过，不能自动撤销，请手动处理。</span></div>` : ''}</div>`;
  const files = ovFiles.map(f => ({ ...f, tip: o.tip && f.tag === 'unk' }));
  return report({ m: 'f', tm: '17:24', over: { n: 2 }, open: true, did: '失败灰字旁的「重试」接上了：点了先显示「正在安排…」，成员状态回到正在干活；中英日文案都加了。', files, ovbox, attn: '为了联调方便顺手改了两处后端文件（重试的错误码映射），不在我负责的目录里，请后端确认或撤销。', give: [['q', '前端好了，可以连同后端一起跑重试用例']], foot: ['17:24', '5.8k', '47'] });
}

const SC = {};
SC.list = () => ({
  rail: { group: 'rel', plusHl: 1, relDot: '' },
  center: centerWrap(chead({ title: '发版检查', count: 2 }), `<div class="empty"><div class="ill">${ic('users')}</div><h3>群里还没有消息</h3><div>这里只显示成员的工作汇报，干活过程留在各自的会话里。</div>
    <ul class="steps"><li><span class="n">1</span>@ 成员，直接交给他（@ 多人会同时开工）</li><li><span class="n">2</span>不 @ 人时，Cat 自动安排谁来接</li><li><span class="n">3</span>成员干完发一张汇报卡，需要的话 @ 下一位接着做</li></ul></div>`, composer()),
  right: right({ order: ['a', 'q'], st: { a: 'idle', q: 'idle' } }),
});
SC.create = () => ({ ...SC.main(), overlay: createModal() });
SC.rolegen = () => ({ ...SC.main(), overlay: rolegenModal() });
SC.main = () => ({
  center: centerWrap(chead(), mainStream(), composer({ working: [['q', '38 秒', 3]] })),
  right: right({ st: { a: 'idle', b: 'idle', f: 'idle', q: 'run' } }),
});
SC.members = () => ({
  center: centerWrap(chead(), mainStream(), composer({ working: WORK })),
  right: right({ st: { a: 'idle', b: 'idle', f: 'ov', q: 'run' }, order: ['a', 'f', 'b', 'q'], drag: 'f', dropAt: 1, sel: 'b', charter: 1 }),
});
SC.mention = () => {
  const pop = `<div class="pop" style="left:60px;bottom:118px;width:300px"><div class="pop-t">@ 成员</div>
    ${ORDER.map(k => `<div class="pop-i ${k === 'b' ? 'on' : ''}">${av(k)}<span>${M[k].n}</span><span class="ttl">${M[k].t}</span>${{ a: ST.idle, b: ST.idle, f: ST.idle, q: ST.run }[k]}</div>`).join('')}</div>`;
  const text = `让 <span class="inchip">${av('f')}@前端工程师</span> 和 <span class="inchip">${av('q')}@测试</span> 一起把重试流程过一遍，<span class="at">@后</span><span class="caret"></span>`;
  return { center: centerWrap(chead(), mainStream(), composer({ text, focus: 1, pop, atHl: 1, hint: '@ 多人会同时开工 · 已识别的 @ 会变成整体标签，退格整个删除' })), right: right({ st: { a: 'idle', b: 'idle', f: 'idle', q: 'run' } }) };
};
SC.working = () => ({
  center: centerWrap(chead(), mainStream(), composer({ working: WORK })),
  right: right({ st: { a: 'idle', b: 'idle', f: 'run', q: 'run' } }),
});
SC.report = () => ({
  focus: '.rep', focusIdx: 1,
  center: centerWrap(chead(), `<div class="daysep">今天</div>
    ${R_ARCH()}
    ${gl('已转交给 后端工程师（第 1/6 次）')}${gl('已转交给 前端工程师（第 1/6 次）')}
    ${R_BE({ open: true })}
    ${gl('已转交给 测试（第 2/6 次）')}
    ${report({ m: 'q', tm: '17:31', parsed: false, did: '三家都跑了一遍重试：Claude 和 Codex 失败后点重试都能接着跑，hop 没变；Grok Build 登录过期那次重试仍然失败，报错文案正常。另外停止后重试也正常。用例记录我放在 tests/group/retry.md 了。', foot: ['17:31', '1.9k', '36'] })}
    ${gl('测试 已完成，没有交给下一位', { icon: 'check' })}`, composer()),
  right: right({ st: { a: 'idle', b: 'idle', f: 'idle', q: 'idle' } }),
});
SC.error = () => ({
  scroll: 'top',
  rail: { devDot: 'red' },
  center: centerWrap(chead(), `<div class="daysep">今天</div>
    ${R_BE()}
    ${gl('已转交给 前端工程师（第 2/6 次）')}${gl('已转交给 测试（第 2/6 次）')}
    ${gl('前端工程师 没完成：Grok Build 登录已过期，请到设置里重新登录', { icon: 'x', cls: 'fail', acts: ['<span class="pipe">|</span><button class="act">' + ic('refresh', 'xs') + '重试</button>', '<button class="act">查看过程</button>'] })}
    ${gl('测试 已停止', { icon: 'stop' })}
    ${task(`${at('a')} 把评审意见也加进契约，然后继续交下去`, '17:40')}
    ${R_ARCH({ tm: '17:43', did: '评审意见已补进契约 6.21.15。', files: [{ p: 'docs/architecture/contract.md', a: 6 }], give: [['b', '照新口径复核一遍']] })}
    ${gl('已转交给 后端工程师（第 6/6 次）')}
    ${gl('已连续转交 6 次，先停在这里。需要继续请 @ 对应成员。', { icon: 'info' })}`, composer()),
  right: right({ st: { a: 'idle', b: 'idle', f: 'fail', q: 'stop' } }),
});
SC.dispatch = () => ({
  scroll: 'top',
  center: centerWrap(chead(), `<div class="daysep">今天</div>
    ${task(T1, '17:08')}
    ${gl('正在安排谁来接…', { spin: 1, cls: 'faded' }).replace('class="gl faded"', 'class="gl" style="opacity:.55"')}
    ${gl('交给 架构师：先出方案再拆给前后端', { lk: 1, tmr: '· 4.2 秒', acts: [swapBtn()] })}
    <div class="rdet"><h5>派活判断</h5>
      <div class="kv"><span>用的 CLI</span><span>Claude · Claude Opus 4.5（排第一的成员）</span><span>耗时</span><span>4.2 秒 · 不留会话</span><span>看了什么</span><span>这条任务 + 最近 10 条群消息 + 4 位成员的职责</span></div>
      <h5>原样输出</h5><pre>{
  "memberId": "m_arch_01",
  "reason": "先出方案再拆给前后端"
}</pre></div>
    ${R_ARCH()}
    ${task('再改一下，按钮文案用「重试」两个字就行', '17:15')}
    ${gl('交给 架构师：接着上一条', { acts: [swapBtn()] })}
    ${task('发版前把更新日志整理一下', '17:33')}
    ${gl('交给 架构师：没判断出该谁接，先交给排第一的成员', { lk: 1, acts: [swapBtn()] })}
    ${task('顺便看下 Windows 打包脚本', '17:36', '<span class="muted">· 自动派活已关</span>')}
    ${gl('交给 架构师：没开自动派活，交给排第一的成员', { acts: [swapBtn()] })}
    ${task('README 里补一段群聊用法', '17:38')}
    ${gl('正在安排谁来接…', { spin: 1 })}`, composer({ working: [['a', '1 分 05 秒', 6]] })),
  right: right({ st: { a: 'run', b: 'idle', f: 'idle', q: 'idle' } }),
});
SC.swap = () => {
  const pop = `<div class="pop" style="left:250px;top:24px;width:260px"><div class="pop-t">换给谁接这条</div>
    ${ORDER.map(k => `<div class="pop-i ${k === 'f' ? 'on' : ''} ${k === 'b' ? 'dis' : ''}">${av(k)}<span>${M[k].n}</span><span class="ttl">${M[k].t}</span><span class="st ${k === 'b' ? 'run' : 'idle'}" style="margin-left:auto">${k === 'b' ? '当前' : '空闲'}</span></div>`).join('')}
    <div class="info-line" style="padding:2px 8px 4px">原来的人会先停下，换人不计入转交次数。</div></div>`;
  return {
    scroll: 'top',
    center: centerWrap(chead(), `<div class="daysep">今天</div>
      ${task('重试按钮点了以后，成员卡片要回到「正在干活」状态', '17:46')}
      ${gl('交给 后端工程师：涉及成员状态推送', { lk: 1, acts: [swapBtn(1)], after: pop })}
      <div style="height:236px"></div>
      ${task('前端工程师那边做过类似的，交给他吧', '17:47')}
      ${gl('已换人：后端工程师 → 前端工程师', { icon: 'swap' })}
      ${gl('后端工程师 已停止（换人时停下的）', { icon: 'stop' })}
      ${gl('交给 前端工程师：你指定的', { acts: [swapBtn()] })}`, composer({ working: [['f', '12 秒', 1]] })),
    right: right({ st: { a: 'idle', b: 'stop', f: 'run', q: 'idle' } }),
  };
};
SC.overstep = () => ({
  focus: '.rep.over',
  rail: { devDot: 'org' },
  center: centerWrap(chead(), `<div class="daysep">今天</div>${R_BE()}${gl('已转交给 前端工程师（第 2/6 次）')}${feReport({ tip: 1 })}${gl('已转交给 测试（第 3/6 次）')}`, composer({ working: [['q', '21 秒', 2]] })),
  right: right({ st: { a: 'idle', b: 'idle', f: 'ov', q: 'run' } }),
});
SC['overstep-undo'] = () => ({
  ...SC.overstep(),
  overlay: modal('撤销 前端工程师 的越界改动？', '只撤销这 2 个文件在这一轮的改动，其他文件不受影响。', `<div class="flist">${fileRows(ovFiles.filter(f => f.tag === 'ov'))}</div><div class="info-line">${ic('info', 'xs')}<span>「来源不明」的文件不在撤销范围内。</span></div>`, `<button class="btn">取消</button><button class="btn dng">撤销</button>`, 'sm'),
});
SC['overstep-result'] = () => ({
  focus: '.rep.over',
  rail: { devDot: 'org' },
  center: centerWrap(chead(), `<div class="daysep">今天</div>${R_BE()}${gl('已转交给 前端工程师（第 2/6 次）')}${feReport({ result: 1 })}${gl('已转交给 测试（第 3/6 次）')}`, composer(), `<div class="toast">${ic('check', 's')}已撤销 1 个文件的改动</div>`),
  right: right({ st: { a: 'idle', b: 'idle', f: 'ov', q: 'idle' } }),
});
function sessionDrawer() {
  return `<div class="dscrim"></div><div class="drawer">
  <div class="dh">${av('f')}<div><div class="t">前端工程师 · 查看过程</div><div class="s">${CLI.g[0]} · grok-code-fast · 第 2/6 次</div></div><span class="sp"></span><button class="ibtn">${ic('x', 's')}</button></div>
  <div class="dbanner">${ic('eye', 'xs')}只看过程：这里的内容不会发进群里，干完后群里只出汇报卡。</div>
  <div class="dbody">
    <div class="stxt">先看一下现有的失败灰字组件和群消息流的事件订阅。</div>
    <div class="tool"><div class="tool-h">${ic('search', 'xs')}<span class="k">搜索</span><span class="a">systemKind === 'failed' in frontend/src</span><span class="ok">${ic('check', 'xs')}</span></div></div>
    <div class="tool"><div class="tool-h">${ic('file', 'xs')}<span class="k">读取</span><span class="a">frontend/src/views/cat/GroupStream.vue</span><span class="ok">${ic('check', 'xs')}</span></div></div>
    <div class="tool"><div class="tool-h">${ic('edit', 'xs')}<span class="k">编辑</span><span class="a">frontend/src/views/cat/RetryButton.vue</span><span class="add">+41</span><span class="ok">${ic('check', 'xs')}</span></div>
    <pre>+ const onRetry = () =&gt; RetryCatGroupMemberTurn({ groupId, memberId, chainId })
+ // 乐观更新：先显示「正在安排谁来接…」</pre></div>
    <div class="tool"><div class="tool-h">${ic('term', 'xs')}<span class="k">运行</span><span class="a">npm run typecheck</span><span class="ok">${ic('check', 'xs')}</span></div><pre>vue-tsc --noEmit  ✓ 0 errors</pre></div>
    <div class="stxt">类型检查通过。接下来把中英日三套文案补上，然后写汇报<span class="caret"></span></div>
  </div>
  <div class="dfoot"><span class="pulse"></span>正在干活 · 已用 2 分 13 秒 · 14 个工具<span class="sp"></span><button class="btn sm">${ic('stop', 'xs')}停止</button></div></div>`;
}
SC.session = () => ({
  center: centerWrap(chead(), mainStream(), composer({ working: [['f', '2 分 13 秒', 14, 'view'], ['q', '38 秒', 0]] }), sessionDrawer()),
  right: right({ st: { a: 'idle', b: 'idle', f: 'run', q: 'run' } }),
});
SC.settings = () => ({ center: centerWrap(chead({ setOn: 1 }), mainStream(), composer()), right: right({ st: { a: 'idle', b: 'idle', f: 'idle', q: 'idle' } }), overlay: settingsModal() });
SC['narrow-drawer'] = () => ({
  layout: 'no-right',
  center: centerWrap(chead({ memBtn: 1, memOn: 1 }), mainStream(), composer({ working: WORK }), `<div class="dscrim"></div><div class="drawer mem-d">${right({ st: { a: 'idle', b: 'idle', f: 'run', q: 'run' }, cls: 'in-drawer' }).replace('<aside class="right in-drawer">', '<div class="right" style="height:100%;border:0">').replace('</aside>', '</div>')}</div>`),
});
SC['narrow-single'] = () => ({
  layout: 'single',
  center: centerWrap(chead({ memBtn: 1 }), mainStream(), composer({ working: WORK })),
});
SC.narrow = SC['narrow-drawer'];

/* ---------- 渲染 ---------- */
function render() {
  const sc = (SC[S] || SC.main)();
  const layout = sc.layout || '';
  const html = `<div class="shell ${layout}">${layout === 'single' ? '' : rail(sc.rail)}${sc.center}${layout ? '' : (sc.right || '')}${sc.overlay || ''}</div><div class="slabel">s=${S}</div>`;
  document.getElementById('app').innerHTML = html;
  document.title = `Cat 群聊 · ${S}`;
  const st = document.querySelector('.stream');
  if (st) {
    if (sc.focus) { const el = st.querySelectorAll(sc.focus)[sc.focusIdx || 0]; if (el) st.scrollTop = el.offsetTop - st.offsetTop - 12; }
    else if (sc.scroll !== 'top') st.scrollTop = st.scrollHeight;
  }
}
render();
