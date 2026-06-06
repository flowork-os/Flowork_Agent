// apps.js — tab "App" (ROADMAP 4): launcher ala Android. Grid ikon app terinstall; klik →
// buka GUI app DI DALAM Flowork (iframe TER-SANDBOX). App = program dipakai MANUSIA (GUI) &
// AGENT (tool) di state yang SAMA. Core app LINTAS BAHASA. Tema Matrix × Jarvis.
//
// KEAMANAN: GUI app pihak-ketiga dimuat di <iframe sandbox="allow-scripts"> TANPA
// allow-same-origin → tak bisa baca session/DOM Flowork. Satu-satunya kanal = postMessage
// {op,args} yang DIVALIDASI host (op terdaftar di manifest) lalu diteruskan ke /api/apps/op.
import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('apps.' + String(k)) });

const CSS = `
.ap{--mx:#22ff88;--cy:#36e6ff;--line:rgba(34,255,136,.20);position:relative;max-width:1040px;padding:6px 2px 50px;
  color:#bdf5d6;font-family:ui-monospace,'JetBrains Mono',Consolas,monospace}
.ap-hud{display:flex;align-items:center;gap:14px;margin:6px 0 20px}
.ap-orb{width:40px;height:40px;position:relative;flex:0 0 auto}
.ap-core{position:absolute;inset:38%;border-radius:50%;background:radial-gradient(circle,#aef6ff,var(--mx) 45%,transparent 72%);box-shadow:0 0 12px var(--mx);animation:appulse 2.4s ease-in-out infinite}
.ap-ring{position:absolute;inset:0;border-radius:50%;border:1px solid var(--line);animation:apspin 7s linear infinite}
@keyframes apspin{to{transform:rotate(360deg)}}@keyframes appulse{0%,100%{transform:scale(1)}50%{transform:scale(1.25);filter:brightness(1.3)}}
.ap-h h2{margin:0;font-size:18px;letter-spacing:4px;color:#eafdff;text-shadow:0 0 12px rgba(34,255,136,.5)}
.ap-h .sub{font-size:12px;color:var(--mx);opacity:.8;margin-top:4px}
.ap-seg{display:flex;gap:6px;margin-bottom:16px}
.ap-segbtn{padding:7px 14px;border-radius:6px;background:transparent;border:1px solid var(--line);color:#7fb9a6;cursor:pointer;font:inherit;font-size:12px;letter-spacing:1px}
.ap-segbtn.on{background:rgba(34,255,136,.12);border-color:var(--mx);color:var(--mx)}
.ap-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:14px}
.ap-icon{position:relative;background:rgba(4,16,11,.6);border:1px solid var(--line);border-radius:12px;padding:14px 10px;text-align:center;cursor:pointer;transition:.15s}
.ap-icon:hover{border-color:var(--mx);box-shadow:0 0 16px rgba(34,255,136,.25);transform:translateY(-2px)}
.ap-icon img{width:48px;height:48px}
.ap-icon .nm{font-size:12px;color:#eafdff;margin-top:8px;word-break:break-word}
.ap-icon .rt{font-size:9px;letter-spacing:1px;color:#7fb9a6;margin-top:3px}
.ap-icon .x{position:absolute;top:4px;right:7px;color:#ff476f;opacity:.6;font-size:13px;display:none}
.ap-icon:hover .x{display:block}
.ap-empty{color:var(--mx);opacity:.65;text-align:center;padding:24px;font-size:13px;border:1px dashed var(--line);border-radius:10px}
.ap-store{background:rgba(4,16,11,.5);border:1px solid var(--line);border-radius:10px;padding:16px;color:#9fd9c4;font-size:13px;line-height:1.6}
.ap-store code{color:var(--cy);background:rgba(54,230,255,.08);padding:1px 6px;border-radius:3px}
/* jendela app (iframe) */
.ap-win{position:fixed;inset:0;z-index:500;background:#06121a;display:flex;flex-direction:column}
.ap-bar{display:flex;align-items:center;gap:12px;padding:9px 16px;border-bottom:1px solid var(--line);background:rgba(4,16,11,.9)}
.ap-bar .t{color:var(--mx);letter-spacing:2px;font-size:13px}.ap-bar .tag{font-size:10px;color:#7fb9a6;border:1px solid var(--line);border-radius:3px;padding:2px 6px}
.ap-bar button{margin-left:auto;background:rgba(34,255,136,.1);border:1px solid var(--line);color:var(--mx);border-radius:5px;padding:6px 14px;cursor:pointer;font:inherit}
.ap-win iframe{flex:1;width:100%;border:0;background:#06121a}
`;

let apps = [];
let seg = 'installed';
let bridgeListener = null, poll = null;

export async function render(mainEl) {
  loadStyle('apps', CSS);
  try { const d = await fetchJSON('/api/apps'); apps = d.apps || []; } catch { apps = []; }
  mainEl.innerHTML = `
    <div class="ap">
      <div class="ap-hud">
        <div class="ap-orb"><div class="ap-ring"></div><div class="ap-core"></div></div>
        <div class="ap-h"><h2>▦ ${esc(L.title)}</h2><div class="sub">${esc(L.sub)}</div></div>
      </div>
      <div class="ap-seg">
        <button class="ap-segbtn ${seg === 'installed' ? 'on' : ''}" data-seg="installed">${esc(L.installed)}</button>
        <button class="ap-segbtn ${seg === 'store' ? 'on' : ''}" data-seg="store">${esc(L.store)}</button>
      </div>
      <div id="apBody"></div>
    </div>`;
  mainEl.querySelectorAll('[data-seg]').forEach(b => b.onclick = () => { seg = b.dataset.seg; render(mainEl); });
  renderBody(mainEl);
}

function renderBody(mainEl) {
  const body = mainEl.querySelector('#apBody');
  if (seg === 'store') {
    body.innerHTML = `<div class="ap-store">${esc(L.store_intro)}<br><br>
      ${esc(L.store_local)} <code>apps/&lt;id&gt;/</code> (manifest.json + core + ui/).<br>
      ${esc(L.store_remote)}</div>`;
    return;
  }
  if (!apps.length) { body.innerHTML = `<div class="ap-empty">${esc(L.empty)}</div>`; return; }
  body.innerHTML = `<div class="ap-grid">${apps.map(iconHTML).join('')}</div>`;
  apps.forEach(a => {
    const el = body.querySelector(`[data-app="${CSS.escape(a.id)}"]`);
    el.querySelector('.open').onclick = () => openApp(a);
  });
}

function iconHTML(a) {
  const native = a.runtime === 'process' || a.runtime === 'http';
  return `<div class="ap-icon" data-app="${escAttr(a.id)}">
    <div class="open">
      <img src="/api/apps/${escAttr(a.id)}/${escAttr(a.icon || 'ui/icon.svg')}" alt="" onerror="this.style.opacity=.3">
      <div class="nm">${esc(a.name || a.id)}</div>
      <div class="rt">${native ? '🔓 native' : '🔒 sandbox'} · ${esc(a.runtime || 'wasm')}</div>
    </div>
  </div>`;
}

// openApp — buka GUI app di iframe ter-sandbox + pasang bridge postMessage + poll state.
function openApp(a) {
  closeWin();
  const ops = new Set((a.operations || []).map(o => o.name)); // validasi op dari iframe
  const win = document.createElement('div');
  win.className = 'ap-win'; win.id = 'apWin';
  win.innerHTML = `
    <div class="ap-bar"><span class="t">▦ ${esc(a.name || a.id)}</span>
      <span class="tag">${a.runtime === 'process' ? '🔓 native' : '🔒 sandbox'}</span>
      <button id="apClose">✕ ${esc(L.close)}</button></div>
    <iframe id="apFrame" sandbox="allow-scripts" src="/api/apps/${escAttr(a.id)}/${escAttr(a.gui_entry || 'ui/index.html')}"></iframe>`;
  document.body.appendChild(win);
  const frame = win.querySelector('#apFrame');
  win.querySelector('#apClose').onclick = closeWin;

  // bridge: iframe → host (validasi op) → /api/apps/op → balas ke iframe.
  bridgeListener = async (e) => {
    if (e.source !== frame.contentWindow) return;
    const d = e.data || {};
    if (d.fw !== 1 || d.kind !== 'op') return;
    const reply = (extra) => frame.contentWindow.postMessage({ fw: 1, kind: 'res', reqId: d.reqId, ...extra }, '*');
    if (!ops.has(d.op)) { reply({ ok: false, error: 'op tak terdaftar' }); return; }
    try {
      const r = await fetchJSON('/api/apps/op', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ app: a.id, op: d.op, args: d.args || {} }) });
      reply({ ok: true, result: r.result });
    } catch (err) { reply({ ok: false, error: String(err.message || err) }); }
  };
  window.addEventListener('message', bridgeListener);

  // poll state: kalau agent mengubah (version naik) → kasih tau iframe biar re-render.
  let lastVer = -1;
  poll = setInterval(async () => {
    try { const s = await fetchJSON('/api/apps/state?id=' + encodeURIComponent(a.id)); if (s.version !== lastVer) { lastVer = s.version; frame.contentWindow.postMessage({ fw: 1, kind: 'state', version: s.version }, '*'); } } catch {}
  }, 2000);
}

function closeWin() {
  if (poll) { clearInterval(poll); poll = null; }
  if (bridgeListener) { window.removeEventListener('message', bridgeListener); bridgeListener = null; }
  const w = document.getElementById('apWin'); if (w) w.remove();
}
