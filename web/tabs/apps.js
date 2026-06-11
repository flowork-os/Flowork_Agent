// apps.js — tab "App": a browser-like tabbed shell.
//
// Behaviour (Chrome-style):
//   • Tab 1 is the launcher (the app list) and can never be closed.
//   • Opening an app spawns a NEW tab running that app's sandboxed GUI; open many.
//   • An app only runs while its tab exists — closing a tab unloads the iframe and
//     stops its bridge + state poll (the app dies). No tab → not running.
//   • The shell lives in <body>, OUTSIDE #main, so switching sidebar menus only
//     HIDES it (display:none keeps every iframe loaded → apps keep working). It
//     reappears, still running, when you come back to the App menu.
//   • Open tabs persist across a page refresh (localStorage); apps reload then.
//
// State lives on `window.__fwApps` (a singleton): the router re-imports each tab
// module with a cache-buster, so module-scope would reset on every visit — the
// shell + open tabs must outlive that. The DOM shell + iframes outlive it too.
//
// Security unchanged: each app GUI is a <iframe sandbox="allow-scripts"> (no
// same-origin). Its only channel is postMessage {op,args}, validated against the
// app's manifest ops, forwarded to /api/apps/op.
import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('apps.' + String(k)) });
const LS_KEY = 'flowork_app_tabs';

// singleton state — survives the router's cache-busted re-imports.
const S = (window.__fwApps = window.__fwApps || {
  shell: null, tabs: [], activeKey: '__home', seg: 'installed', restored: false, apps: [],
});

const CSS = `
.app-shell { position:fixed; z-index:40; display:none; flex-direction:column; background:#0a0e16;
  border-left:1px solid rgba(148,163,184,0.12); }
.as-tabbar { display:flex; align-items:center; gap:3px; padding:7px 9px 0; background:rgba(15,23,42,0.7);
  border-bottom:1px solid rgba(148,163,184,0.16); overflow-x:auto; flex:0 0 auto; }
.as-tab { display:inline-flex; align-items:center; gap:8px; padding:8px 13px; border-radius:10px 10px 0 0;
  background:transparent; color:#94a3b8; cursor:pointer; font-size:0.85rem; border:1px solid transparent;
  border-bottom:none; white-space:nowrap; max-width:230px; transition:background .15s,color .15s; user-select:none; }
.as-tab:hover { background:rgba(148,163,184,0.08); color:#cbd5e1; }
.as-tab.on { background:#0e1320; color:#e2e8f0; border-color:rgba(148,163,184,0.18); }
.as-tab img { width:18px; height:18px; border-radius:4px; flex:0 0 auto; }
.as-tab .nm { overflow:hidden; text-overflow:ellipsis; }
.as-tab .cl { margin-left:2px; opacity:.55; border-radius:5px; width:18px; height:18px; line-height:16px; text-align:center; flex:0 0 auto; }
.as-tab .cl:hover { opacity:1; background:rgba(248,113,113,0.22); color:#f87171; }
.as-tab.home { font-size:1.05rem; padding:8px 14px; }
.as-body { flex:1; position:relative; overflow:hidden; }
.as-pane { position:absolute; inset:0; display:none; overflow:auto; }
.as-pane.on { display:block; }
.as-pane iframe { width:100%; height:100%; border:0; background:#06121a; display:block; }

/* ── launcher (home pane) — clean, matches the Agent/Group language ── */
.as-launch { padding:26px 30px 50px; color:#e2e8f0; }
.al-hero { padding:26px 30px; border-radius:16px; margin-bottom:22px;
  background:linear-gradient(135deg, rgba(124,58,237,0.18) 0%, rgba(14,165,233,0.14) 52%, rgba(16,185,129,0.13) 100%);
  border:1px solid rgba(148,163,184,0.2); }
.al-eyebrow { font-size:0.72rem; letter-spacing:0.3em; color:#a78bfa; text-transform:uppercase; font-weight:600; margin-bottom:7px; }
.al-h1 { margin:0; font-size:1.8rem; font-weight:700; background:linear-gradient(90deg,#c4b5fd,#67e8f9 55%,#6ee7b7);
  -webkit-background-clip:text; background-clip:text; color:transparent; }
.al-sub { margin:8px 0 0; color:#cbd5e1; font-size:0.92rem; }
.al-seg { display:flex; gap:8px; margin-bottom:18px; }
.al-segbtn { padding:7px 15px; border-radius:9px; background:rgba(2,6,18,0.4); border:1px solid rgba(148,163,184,0.2);
  color:#94a3b8; cursor:pointer; font:inherit; font-size:0.84rem; transition:all .15s; }
.al-segbtn.on { background:rgba(124,58,237,0.18); border-color:rgba(167,139,250,0.5); color:#c4b5fd; }
.al-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(140px,1fr)); gap:16px; }
.al-card { position:relative; background:rgba(15,23,42,0.6); border:1px solid rgba(148,163,184,0.18); border-radius:14px;
  padding:18px 12px; text-align:center; cursor:pointer; transition:all .15s; }
.al-card:hover { border-color:rgba(167,139,250,0.5); transform:translateY(-2px); box-shadow:0 14px 34px -24px rgba(124,58,237,0.5); }
.al-card img { width:46px; height:46px; }
.al-card .nm { font-size:0.86rem; color:#f1f5f9; margin-top:9px; word-break:break-word; }
.al-card .rt { font-size:0.66rem; letter-spacing:0.06em; color:#94a3b8; margin-top:4px; }
.al-card .x { position:absolute; top:5px; right:8px; color:#f87171; opacity:0; font-size:0.85rem; transition:opacity .15s; }
.al-card:hover .x { opacity:.7; }
.al-card .x:hover { opacity:1; }
.al-empty { color:#94a3b8; text-align:center; padding:30px; font-size:0.9rem; border:1px dashed rgba(148,163,184,0.25); border-radius:12px; }
.al-store { background:rgba(15,23,42,0.5); border:1px solid rgba(148,163,184,0.18); border-radius:12px; padding:18px 20px;
  color:#cbd5e1; font-size:0.9rem; line-height:1.65; }
.al-store code { color:#67e8f9; background:rgba(14,165,233,0.1); padding:1px 6px; border-radius:4px; }
.al-msg { font-size:0.84rem; margin-left:10px; }
`;

function topTab() { return (location.hash || '').replace(/^#\/?/, '').split('/')[0] || ''; }
function visible() { return S.shell && S.shell.style.display !== 'none'; }

function positionShell() {
  const main = document.getElementById('main');
  if (!main || !S.shell) return;
  const r = main.getBoundingClientRect();
  S.shell.style.left = r.left + 'px';
  S.shell.style.top = r.top + 'px';
  S.shell.style.width = r.width + 'px';
  S.shell.style.height = r.height + 'px';
}
function show() { if (S.shell) { S.shell.style.display = 'flex'; positionShell(); } }
function hide() { if (S.shell) S.shell.style.display = 'none'; } // iframes stay loaded → apps keep running

function ensureShell() {
  if (S.shell) return;
  const shell = document.createElement('div');
  shell.className = 'app-shell';
  shell.innerHTML = `<div class="as-tabbar" id="asTabbar"></div><div class="as-body" id="asBody"></div>`;
  document.body.appendChild(shell);
  S.shell = shell;

  // the launcher (home) tab — built once, never closed.
  const home = document.createElement('div');
  home.className = 'as-pane as-launch';
  shell.querySelector('#asBody').appendChild(home);
  S.tabs.push({ key: '__home', pane: home, home: true });

  // keep the shell glued to #main as it resizes (nav collapse, window resize).
  const main = document.getElementById('main');
  if (main && 'ResizeObserver' in window) { new ResizeObserver(() => { if (visible()) positionShell(); }).observe(main); }
  window.addEventListener('resize', () => { if (visible()) positionShell(); });
  // route awareness: only the App menu shows the shell; others hide it (apps live on).
  window.addEventListener('hashchange', () => { topTab() === 'apps' ? show() : hide(); });

  renderTabbar();
  activate('__home');
}

// ── tab bar + panes ───────────────────────────────────────────────────────────
function renderTabbar() {
  const bar = S.shell.querySelector('#asTabbar');
  bar.innerHTML = S.tabs.map((tb) => {
    if (tb.home) return `<div class="as-tab home ${S.activeKey === tb.key ? 'on' : ''}" data-key="${escAttr(tb.key)}" title="${escAttr(L.title)}">▦</div>`;
    const a = tb.app;
    return `<div class="as-tab ${S.activeKey === tb.key ? 'on' : ''}" data-key="${escAttr(tb.key)}" title="${escAttr(a.name || a.id)}">
      <img src="/api/apps/${escAttr(a.id)}/${escAttr(a.icon || 'ui/icon.svg')}" alt="" onerror="this.style.display='none'">
      <span class="nm">${esc(a.name || a.id)}</span>
      <span class="cl" title="${escAttr(L.close)}">✕</span>
    </div>`;
  }).join('');
  bar.querySelectorAll('.as-tab').forEach((el) => {
    const key = el.dataset.key;
    el.onclick = (e) => { if (e.target.classList.contains('cl')) { e.stopPropagation(); closeTab(key); } else activate(key); };
  });
}

function activate(key) {
  S.activeKey = key;
  S.tabs.forEach((tb) => tb.pane.classList.toggle('on', tb.key === key));
  renderTabbar();
  persist();
}

function closeTab(key) {
  const i = S.tabs.findIndex((tb) => tb.key === key);
  if (i < 0 || S.tabs[i].home) return; // never close the launcher
  const tb = S.tabs[i];
  if (tb.poll) clearInterval(tb.poll);
  if (tb.bridge) window.removeEventListener('message', tb.bridge);
  tb.pane.remove(); // unloads the iframe (client side)
  // stop the core PROCESS too — an app runs only while a tab is open (best-effort).
  if (tb.app) fetch('/api/apps/stop?id=' + encodeURIComponent(tb.app.id), { method: 'POST' }).catch(() => {});
  S.tabs.splice(i, 1);
  if (S.activeKey === key) activate(S.tabs[Math.max(0, i - 1)].key);
  else { renderTabbar(); persist(); }
}

// ── open an app in a tab (sandboxed iframe + bridge + state poll) ──────────────
function openApp(a) {
  const key = 'app:' + a.id;
  if (S.tabs.find((tb) => tb.key === key)) { activate(key); return; }

  const pane = document.createElement('div');
  pane.className = 'as-pane';
  const frame = document.createElement('iframe');
  frame.sandbox = 'allow-scripts';
  frame.src = `/api/apps/${a.id}/${a.gui_entry || 'ui/index.html'}`;
  pane.appendChild(frame);
  S.shell.querySelector('#asBody').appendChild(pane);

  const ops = new Set((a.operations || []).map((o) => o.name));
  const bridge = async (e) => {
    if (e.source !== frame.contentWindow) return; // only this tab's iframe
    const d = e.data || {};
    if (d.fw !== 1 || d.kind !== 'op') return;
    const reply = (extra) => frame.contentWindow.postMessage({ fw: 1, kind: 'res', reqId: d.reqId, ...extra }, '*');
    if (!ops.has(d.op)) { reply({ ok: false, error: 'op tak terdaftar' }); return; }
    try {
      const r = await fetchJSON('/api/apps/op', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ app: a.id, op: d.op, args: d.args || {} }) });
      reply({ ok: true, result: r.result });
    } catch (err) { reply({ ok: false, error: String(err.message || err) }); }
  };
  window.addEventListener('message', bridge);

  let lastVer = -1;
  const poll = setInterval(async () => {
    try { const s = await fetchJSON('/api/apps/state?id=' + encodeURIComponent(a.id)); if (s.version !== lastVer) { lastVer = s.version; frame.contentWindow.postMessage({ fw: 1, kind: 'state', version: s.version }, '*'); } } catch {}
  }, 2000);

  S.tabs.push({ key, app: a, pane, frame, bridge, poll });
  activate(key);
}

// ── persistence (survive refresh) ──────────────────────────────────────────────
function persist() {
  try {
    const open = S.tabs.filter((tb) => !tb.home).map((tb) => tb.app.id);
    localStorage.setItem(LS_KEY, JSON.stringify({ open, active: S.activeKey }));
  } catch {}
}
function restoreTabs() {
  let saved;
  try { saved = JSON.parse(localStorage.getItem(LS_KEY) || '{}'); } catch { saved = {}; }
  for (const id of saved.open || []) {
    const a = S.apps.find((x) => x.id === id);
    if (a) openApp(a);
  }
  if (saved.active && S.tabs.some((tb) => tb.key === saved.active)) activate(saved.active);
  else activate('__home');
}

// ── launcher (home pane) render ────────────────────────────────────────────────
async function refreshApps() {
  try { S.apps = (await fetchJSON('/api/apps')).apps || []; } catch { S.apps = []; }
  renderHome();
}

function renderHome() {
  const home = S.tabs.find((tb) => tb.home);
  if (!home) return;
  home.pane.innerHTML = `
    <div class="al-hero">
      <div class="al-eyebrow">FLOWORK · APPS</div>
      <h1 class="al-h1">${esc(L.title)}</h1>
      <p class="al-sub">${esc(L.sub)}</p>
    </div>
    <div class="al-seg">
      <button class="al-segbtn ${S.seg === 'installed' ? 'on' : ''}" data-seg="installed">${esc(L.installed)}</button>
      <button class="al-segbtn ${S.seg === 'store' ? 'on' : ''}" data-seg="store">${esc(L.store)}</button>
    </div>
    <div id="alBody"></div>`;
  home.pane.querySelectorAll('[data-seg]').forEach((b) => b.onclick = () => { S.seg = b.dataset.seg; renderHome(); });
  renderHomeBody(home.pane.querySelector('#alBody'));
}

function renderHomeBody(body) {
  if (S.seg === 'store') {
    body.innerHTML = `<div class="al-store">${esc(L.store_intro)}<br><br>
      <button class="al-segbtn on" id="alPick">${esc(L.store_pick)}</button>
      <input type="file" id="alFile" accept=".fwpack,.zip" style="display:none">
      <span class="al-msg" id="alMsg"></span><br><br>
      ${esc(L.store_local)} <code>apps/&lt;id&gt;/</code> (manifest.json + core + ui/).<br>
      ${esc(L.store_remote)}</div>`;
    const file = body.querySelector('#alFile');
    body.querySelector('#alPick').onclick = () => file.click();
    file.onchange = () => { if (file.files[0]) installPack(file.files[0]); };
    return;
  }
  if (!S.apps.length) { body.innerHTML = `<div class="al-empty">${esc(L.empty)}</div>`; return; }
  body.innerHTML = `<div class="al-grid">${S.apps.map(cardHTML).join('')}</div>`;
  S.apps.forEach((a) => {
    const el = body.querySelector(`[data-app="${a.id}"]`);
    el.onclick = (e) => { if (e.target.classList.contains('x')) { e.stopPropagation(); uninstallApp(a); } else openApp(a); };
  });
}

function cardHTML(a) {
  const native = a.runtime === 'process' || a.runtime === 'http';
  return `<div class="al-card" data-app="${escAttr(a.id)}">
    <span class="x" title="${escAttr(L.uninstall)}">✕</span>
    <img src="/api/apps/${escAttr(a.id)}/${escAttr(a.icon || 'ui/icon.svg')}" alt="" onerror="this.style.opacity=.3">
    <div class="nm">${esc(a.name || a.id)}</div>
    <div class="rt">${native ? '🔓 native' : '🔒 sandbox'} · ${esc(a.runtime || 'wasm')}</div>
  </div>`;
}

async function installPack(f) {
  const home = S.tabs.find((tb) => tb.home);
  const msg = home && home.pane.querySelector('#alMsg');
  if (!confirm(L.store_exec_warn)) return;
  if (msg) msg.textContent = '⟳ ' + L.installing;
  const fd = new FormData(); fd.append('file', f);
  try {
    const resp = await fetch('/api/apps/install?approve_exec=1', { method: 'POST', body: fd });
    const r = await resp.json();
    if (!resp.ok || r.error) throw new Error(r.error || ('HTTP ' + resp.status));
    if (msg) msg.textContent = '✓ ' + L.install_ok + (r.app ? ' — ' + r.app : '');
    S.seg = 'installed';
    await refreshApps();
  } catch (e) { if (msg) msg.textContent = '✕ ' + L.install_fail + ': ' + (e.message || e); }
}

async function uninstallApp(a) {
  if (!confirm(L.uninstall_confirm.replace('{name}', a.name || a.id))) return;
  try {
    closeTab('app:' + a.id); // an open app must stop before its folder is removed
    await fetchJSON('/api/apps/uninstall?id=' + encodeURIComponent(a.id), { method: 'POST' });
    await refreshApps();
  } catch (e) { alert((L.install_fail || 'failed') + ': ' + (e.message || e)); }
}

// ── entry: called by the router whenever the App menu is opened ────────────────
export async function render(mainEl) {
  loadStyle('apps', CSS);
  ensureShell();          // singleton — built once, survives menu switches
  mainEl.innerHTML = '';  // the shell overlays #main; keep #main empty
  await refreshApps();    // refresh the launcher list
  if (!S.restored) { S.restored = true; restoreTabs(); } // reopen tabs saved before a refresh
  show();
}
