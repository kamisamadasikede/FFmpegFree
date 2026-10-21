/* ==========================================================================
   ffc 原型 · 页面骨架（侧栏 + 顶栏）
   用法：
     ffcShell({ active:'convert', title:'转换', sub:'…', actions:[…], engine:{…} })
   页面里只需放 <aside id="side"> <header id="top"> 两个空容器。
   ========================================================================== */

const ICONS = {
  convert: '<path d="M4 8h11l-3-3M20 16H9l3 3"/>',
  mic: '<path d="M12 3a3 3 0 0 0-3 3v5a3 3 0 0 0 6 0V6a3 3 0 0 0-3-3Z"/><path d="M5 11a7 7 0 0 0 14 0"/><path d="M12 18v3"/>',
  doc: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5Z"/><path d="M14 3v5h5"/><path d="M8.5 13h7M8.5 16.5h4.5"/>',
  live: '<circle cx="12" cy="12" r="2.5"/><path d="M7.5 7.5a6 6 0 0 0 0 9M16.5 7.5a6 6 0 0 1 0 9"/><path d="M4.5 4.5a11 11 0 0 0 0 15M19.5 4.5a11 11 0 0 1 0 15"/>',
  code: '<path d="m9 8-4 4 4 4M15 8l4 4-4 4"/>',
  tasks: '<path d="M4 6.5h9M4 12h13M4 17.5h7"/><path d="M17.5 6.5 19 8l2.5-3"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M12 3v2.2M12 18.8V21M4.2 7.5l1.9 1.1M17.9 15.4l1.9 1.1M4.2 16.5l1.9-1.1M17.9 8.6l1.9-1.1"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m20 20-4.8-4.8"/>',
  theme: '<path d="M12 3a9 9 0 1 0 9 9 7 7 0 0 1-9-9Z"/>',
  help: '<circle cx="12" cy="12" r="8.5"/><path d="M9.8 9.5a2.3 2.3 0 1 1 3.1 2.1c-.6.2-.9.7-.9 1.3v.4"/><path d="M12 16.6h.01"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  upload: '<path d="M12 16V4m0 0L8 8m4-4 4 4"/><path d="M4 16v3a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-3"/>',
  folder: '<path d="M3 7.5A1.5 1.5 0 0 1 4.5 6H9l2 2.5h8.5A1.5 1.5 0 0 1 21 10v7.5A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5Z"/>',
  trash: '<path d="M4.5 7h15M9.5 7V5.5A1.5 1.5 0 0 1 11 4h2a1.5 1.5 0 0 1 1.5 1.5V7"/><path d="M6.5 7l.8 11.4A1.6 1.6 0 0 0 8.9 20h6.2a1.6 1.6 0 0 0 1.6-1.6L17.5 7"/>',
  eye: '<path d="M2.5 12S6 6.5 12 6.5 21.5 12 21.5 12S18 17.5 12 17.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="2.6"/>',
  play: '<path d="M8 5.5v13l11-6.5Z"/>',
  stop: '<rect x="6.5" y="6.5" width="11" height="11" rx="1.6"/>',
  more: '<circle cx="6" cy="12" r="1.3"/><circle cx="12" cy="12" r="1.3"/><circle cx="18" cy="12" r="1.3"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7"/>',
  warn: '<path d="M12 4.5 21 19.5H3Z"/><path d="M12 10v4M12 16.8h.01"/>',
  info: '<circle cx="12" cy="12" r="8.5"/><path d="M12 11v5.2M12 8.2h.01"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  refresh: '<path d="M20 12a8 8 0 1 1-2.4-5.7"/><path d="M20 4.5V10h-5.5"/>',
  cpu: '<rect x="7" y="7" width="10" height="10" rx="2"/><path d="M10 3v2.5M14 3v2.5M10 18.5V21M14 18.5V21M3 10h2.5M3 14h2.5M18.5 10H21M18.5 14H21"/>',
  external: '<path d="M14 4h6v6"/><path d="M20 4l-8.5 8.5"/><path d="M18 14.5V19a1.5 1.5 0 0 1-1.5 1.5H5A1.5 1.5 0 0 1 3.5 19V7.5A1.5 1.5 0 0 1 5 6h4.5"/>',
  screen: '<rect x="3" y="4.5" width="18" height="12" rx="1.8"/><path d="M8.5 20h7M12 16.5V20"/>',
  file: '<path d="M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5Z"/><path d="M13.5 3.5v5h5"/>',
  link: '<path d="M10 13.5a3.5 3.5 0 0 0 5 0l2.5-2.5a3.5 3.5 0 0 0-5-5l-1 1"/><path d="M14 10.5a3.5 3.5 0 0 0-5 0L6.5 13a3.5 3.5 0 0 0 5 5l1-1"/>',
  down: '<path d="m7 10 5 5 5-5"/>',
  left: '<path d="m14 7-5 5 5 5"/>',
  right: '<path d="m10 7 5 5-5 5"/>',
  layers: '<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 13 9 5 9-5"/>',
  bolt: '<path d="M13 3 5.5 13.5H11l-1 7.5L18.5 10H13Z"/>',
};

function icon(name, cls) {
  const p = ICONS[name] || ICONS.info;
  return `<svg class="${cls || ''}" viewBox="0 0 24 24" fill="none" stroke="currentColor"
    stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${p}</svg>`;
}

const NAV = [
  {
    label: '主功能',
    items: [
      { key: 'convert', icon: 'convert', label: '转换', href: '01-convert.html' },
      { key: 'voice', icon: 'mic', label: '语音工具', href: '02-voice.html' },
      { key: 'docs', icon: 'doc', label: '文档', href: '03-docs-convert.html' },
      { key: 'live', icon: 'live', label: '直播', href: '05-live-push.html' },
    ],
  },
  {
    label: '工具与系统',
    items: [
      { key: 'json', icon: 'code', label: 'JSON 工具', href: '08-tools-json.html' },
      { key: 'tasks', icon: 'tasks', label: '任务中心', href: '09-tasks.html', badge: '1' },
    ],
  },
];

const ACTION_ICONS = {
  search: ['search', '搜索'],
  theme: ['theme', '切换主题'],
  help: ['help', '帮助'],
  settings: ['settings', '设置'],
};

function ffcShell(cfg) {
  cfg = cfg || {};
  const active = cfg.active || '';
  const engine = cfg.engine || { text: '就绪', state: 'ok', path: '6.1' };

  /* ?theme=light 可直接出浅色截图 */
  const qs = new URLSearchParams(location.search).get('theme');
  if (qs) document.documentElement.dataset.theme = qs;

  /* ---- 侧栏 ---- */
  const navHtml = NAV.map((g) => `
    <div class="nav-group">
      <div class="nav-label">${g.label}</div>
      ${g.items.map((it) => `
        <a class="nav-item ${it.key === active ? 'is-active' : ''}" href="${it.href}">
          ${icon(it.icon)}
          <span>${it.label}</span>
          ${it.badge ? `<span class="badge">${it.badge}</span>` : ''}
        </a>`).join('')}
    </div>`).join('');

  document.getElementById('side').innerHTML = `
    <div class="side-brand">
      <div class="mark">ffc</div>
      <div class="name">ffc</div>
      <div class="ver">v2.4.1</div>
    </div>
    <div class="side-scroll">${navHtml}</div>
    <div class="side-foot">
      <div class="engine ${engine.state === 'warn' ? 'is-warn' : ''}">
        <span class="dot"></span>
        <span class="et"><b>${engine.title || '转换组件'}</b> · ${engine.text}</span>
        ${engine.path ? `<span class="path">${engine.path}</span>` : ''}
      </div>
      <a class="nav-item ${active === 'settings' ? 'is-active' : ''}" href="10-settings.html" style="margin-top:8px">
        ${icon('settings')}<span>设置</span>
      </a>
    </div>`;

  /* ---- 顶栏 ---- */
  const acts = (cfg.actions || ['search', 'theme', 'help'])
    .map((k) => {
      const [ic, tip] = ACTION_ICONS[k] || [k, k];
      return `<button class="c-icon-btn" title="${tip}">${icon(ic)}</button>`;
    })
    .join('');

  document.getElementById('top').innerHTML = `
    <h1>${cfg.title || ''}</h1>
    ${cfg.sub ? `<div class="sub">${cfg.sub}</div>` : ''}
    <div class="topbar-actions">${acts}${cfg.extraActions || ''}</div>`;

  /* ---- 主题开关（原型演示用） ---- */
  const themeBtn = document.querySelector('.topbar-actions [title="切换主题"]');
  if (themeBtn) {
    themeBtn.addEventListener('click', () => {
      const root = document.documentElement;
      root.dataset.theme = root.dataset.theme === 'light' ? 'dark' : 'light';
    });
  }
}
