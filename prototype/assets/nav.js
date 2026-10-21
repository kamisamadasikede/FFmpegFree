/* 原型导航：页间跳转 + 悬浮切换器（纯前端，无依赖） */
(function () {
  'use strict';

  // 由当前地址推断站点根（pages/ 下用 ../，根目录用 ./）
  const BASE = location.pathname.indexOf('/pages/') >= 0 ? '../' : '';

  const PAGES = [
    ['01', '转换', 'pages/01-convert.html'],
    ['02', '语音工具', 'pages/02-voice.html'],
    ['03', '文档 · 转换', 'pages/03-docs-convert.html'],
    ['04', '文档 · PDF 预览', 'pages/04-docs-pdf.html'],
    ['05', '直播 · 文件推流', 'pages/05-live-push.html'],
    ['06', '直播 · 录屏推流', 'pages/06-live-record.html'],
    ['07', '直播 · 拉流播放', 'pages/07-live-pull.html'],
    ['08', 'JSON 工具', 'pages/08-tools-json.html'],
    ['09', '任务中心', 'pages/09-tasks.html'],
    ['10', '设置', 'pages/10-settings.html'],
    ['11', '关于', 'pages/11-about.html'],
    ['L', '转换（浅色）', 'pages/01-convert.html?theme=light'],
  ];

  const TAB_MAP = {
    '文档转换': 'pages/03-docs-convert.html',
    'PDF预览': 'pages/04-docs-pdf.html',
    '文件推流': 'pages/05-live-push.html',
    '录屏推流': 'pages/06-live-record.html',
    '拉流播放': 'pages/07-live-pull.html',
    'JSON工具': 'pages/08-tools-json.html',
  };

  const fail = (where, err) => {
    document.documentElement.setAttribute('data-nav-error', where + ': ' + (err && err.message ? err.message : err));
  };

  /* 1. 顶部页签可点 */
  function wireTabs() {
    const tabs = document.querySelectorAll('.c-tabs button');
    for (let i = 0; i < tabs.length; i++) {
      const btn = tabs[i];
      const label = (btn.textContent || '').replace(/\s+/g, '');
      const target = TAB_MAP[label];
      if (!target) continue;
      btn.style.cursor = 'pointer';
      btn.addEventListener('click', function () { location.href = BASE + target; });
    }
  }

  /* 2. 设置页子导航：切换分节 */
  function wireSections() {
    const links = [];
    document.querySelectorAll('.subnav a[data-sect]').forEach(function (a) { links.push(a); });
    const sects = [];
    document.querySelectorAll('.sects .sect').forEach(function (s) { sects.push(s); });
    if (!links.length || !sects.length) return;

    function show(idx) {
      sects.forEach(function (s, i) { s.style.display = i === idx ? '' : 'none'; });
      links.forEach(function (a, i) { a.classList.toggle('is-on', i === idx); });
    }
    links.forEach(function (a, i) {
      a.removeAttribute('href');
      a.style.cursor = 'pointer';
      a.addEventListener('click', function (e) { e.preventDefault(); show(i); });
    });
    show(0);
  }

  /* 3. 悬浮切换器 */
  function mountSwitcher() {
    const here = location.pathname.split('/').pop() + location.search;
    const hereFile = here.split('?')[0];

    const items = PAGES.map(function (p) {
      const on = p[2].split('?')[0] === hereFile && (p[2].indexOf('?') >= 0) === (here.indexOf('?') >= 0);
      return '<a class="proto-nav-item' + (on ? ' is-on' : '') + '" href="' + BASE + p[2] + '">'
        + '<span class="n">' + p[0] + '</span>' + p[1] + '</a>';
    }).join('');

    const box = document.createElement('div');
    box.className = 'proto-nav';
    box.innerHTML =
      '<button class="proto-nav-btn" title="切换页面（P）">页面</button>' +
      '<div class="proto-nav-panel" hidden>' +
        '<div class="proto-nav-hd">原型 v1 · 11 屏</div>' +
        items +
        '<div class="proto-nav-ft">方向键 ← → 翻页 · P 开关</div>' +
      '</div>';
    document.body.appendChild(box);

    const btn = box.querySelector('.proto-nav-btn');
    const panel = box.querySelector('.proto-nav-panel');
    function toggle(open) {
      const next = typeof open === 'boolean' ? open : panel.hidden;
      panel.hidden = !next;
      btn.classList.toggle('is-on', next);
    }

    btn.addEventListener('click', function (e) { e.stopPropagation(); toggle(); });
    panel.addEventListener('click', function (e) { e.stopPropagation(); });
    document.addEventListener('click', function () { toggle(false); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') toggle(false);
      else if (e.key === 'p' || e.key === 'P') toggle();
      else if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') {
        let idx = -1;
        for (let i = 0; i < 11; i++) if (PAGES[i][2].split('?')[0] === hereFile) { idx = i; break; }
        if (idx < 0) return;
        const next = e.key === 'ArrowRight' ? (idx + 1) % 11 : (idx + 10) % 11;
        location.href = BASE + PAGES[next][2];
      }
    });

    if (new URLSearchParams(location.search).get('nav') === '1') toggle(true);
  }

  const CSS = [
    '.proto-nav{position:fixed;right:16px;bottom:44px;z-index:200}',
    '.proto-nav-btn{height:30px;padding:0 12px;border-radius:8px;background:var(--bg-card);',
    'border:1px solid var(--line-strong);color:var(--fg-2);font-size:12px;font-weight:600;',
    'box-shadow:var(--shadow-2)}',
    '.proto-nav-btn:hover,.proto-nav-btn.is-on{color:var(--fg-1);border-color:var(--brand)}',
    '.proto-nav-panel{position:absolute;right:0;bottom:38px;width:218px;background:var(--bg-card);',
    'border:1px solid var(--line-strong);border-radius:12px;padding:6px;box-shadow:var(--shadow-pop)}',
    '.proto-nav-hd{font-size:10.5px;letter-spacing:.06em;color:var(--fg-3);padding:6px 8px 8px}',
    '.proto-nav-item{display:flex;align-items:center;gap:9px;height:30px;padding:0 8px;',
    'border-radius:6px;text-decoration:none;color:var(--fg-2);font-size:12.5px}',
    '.proto-nav-item:hover{background:var(--bg-hover);color:var(--fg-1)}',
    '.proto-nav-item.is-on{background:var(--bg-active);color:var(--fg-1);font-weight:600}',
    '.proto-nav-item .n{font-size:10px;color:var(--fg-3);width:16px;font-family:var(--font-mono)}',
    '.proto-nav-ft{font-size:10px;color:var(--fg-3);padding:8px 8px 4px;border-top:1px solid var(--line);margin-top:4px}',
  ].join('');

  try {
    const style = document.createElement('style');
    style.textContent = CSS;
    document.head.appendChild(style);
    wireTabs();
    wireSections();
    mountSwitcher();
  } catch (err) {
    fail('nav', err);
  }
})();
