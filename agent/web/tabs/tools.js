// tools.js — tab "Tools": plug-and-play TOOL-PACK gallery.
//
// Flowork = chatbot polos; TANGAN-nya diisi tool-pack. Tiap tool-pack = .fwpack
// (zip: plugin.json kind:tool + agents/<id>/{agent.wasm,manifest.json}) yang di-
// hot-load + RegisterDynamic. Tab ini: upload (install) · daftar ter-install ·
// uninstall. Akses per-agent diatur di tab Agent (Tools catalog/subscribe).
//
// Tool MCP (prefix "mcp_") DIKELOLA di Connections — disaring dari sini biar ga
// dobel-kelola. Semua copy lewat i18n (en base + id) — no hardcoded string.
//
// API: GET /api/tools/installed · POST /api/tools/install (multipart "file") ·
//      POST /api/tools/uninstall?tool=<name>.

import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('tools.' + String(k)) });
const fmt = (k, vars) => Object.entries(vars || {}).reduce((s, [n, v]) => s.replaceAll('{' + n + '}', v), L[k]);

const CSS = `
.tp-wrap{position:relative;padding:6px 2px 40px;
  --ty:#ffb24d;--ty2:#ffd98a;--line:rgba(255,178,77,.22);--bad:#ff476f}
.tp-wrap *{position:relative;z-index:1}
.tp-hud{display:flex;align-items:center;gap:16px;margin:6px 0 22px}
.tp-emb{width:54px;height:54px;flex:0 0 auto;position:relative}
.tp-core{position:absolute;top:50%;left:50%;width:14px;height:14px;margin:-7px 0 0 -7px;border-radius:50%;
  background:radial-gradient(circle,#fff0d6 0,var(--ty) 42%,transparent 72%);
  box-shadow:0 0 12px var(--ty),0 0 26px rgba(255,178,77,.5);animation:tppulse 2.4s ease-in-out infinite}
.tp-orbit{position:absolute;inset:0;animation:tpspin 7s linear infinite}
.tp-node{position:absolute;top:-3px;left:50%;width:6px;height:6px;margin-left:-3px;border-radius:50%;
  background:var(--ty2);box-shadow:0 0 8px var(--ty2)}
@keyframes tpspin{to{transform:rotate(360deg)}}
@keyframes tppulse{0%,100%{transform:scale(1)}50%{transform:scale(1.18);filter:brightness(1.3)}}
.tp-htext h2{margin:0;font-family:var(--disp,inherit);font-size:18px;letter-spacing:4px;color:#fff7ea;text-shadow:0 0 12px rgba(255,178,77,.45)}
.tp-htext .sub{font-size:12px;color:var(--ty2);opacity:.82;margin-top:5px;max-width:680px;line-height:1.5}
.tp-htext .stat{font-size:10px;letter-spacing:2px;color:var(--ty2);margin-top:7px;display:flex;align-items:center;gap:7px}
.tp-dot{width:7px;height:7px;border-radius:50%;background:var(--ty2);box-shadow:0 0 8px var(--ty2);animation:tpblink 1.6s ease-in-out infinite}
@keyframes tpblink{0%,100%{opacity:1}50%{opacity:.25}}
.tp-panel{position:relative;background:rgba(22,14,4,.6);border:1px solid var(--line);border-radius:8px;padding:15px 17px;margin-bottom:15px;backdrop-filter:blur(3px)}
.tp-panel::before,.tp-panel::after{content:'';position:absolute;width:12px;height:12px;border:2px solid var(--ty);opacity:.7;pointer-events:none}
.tp-panel::before{top:-1px;left:-1px;border-right:0;border-bottom:0}
.tp-panel::after{bottom:-1px;right:-1px;border-left:0;border-top:0}
.tp-sec{font-size:10px;letter-spacing:2px;color:var(--ty);margin:0 0 9px;text-shadow:0 0 8px rgba(255,178,77,.3)}
.tp-drop{border:1px dashed var(--line);border-radius:7px;padding:18px;text-align:center;color:var(--ty2);cursor:pointer;font-size:13px;transition:.2s}
.tp-drop:hover,.tp-drop.over{background:rgba(255,178,77,.07);border-color:var(--ty)}
.tp-hint{font-size:11px;color:rgba(255,178,77,.55);margin-top:8px}
.tp-msg{font-size:12px;margin-top:9px;min-height:16px}
.tp-card{display:flex;align-items:center;gap:14px;flex-wrap:wrap}
.tp-card h3{margin:0;font-size:15px;color:#fff7ea;display:flex;align-items:center;gap:9px}
.tp-tag{font-size:10px;letter-spacing:1px;color:var(--ty);border:1px solid var(--line);border-radius:3px;padding:2px 7px;background:rgba(255,178,77,.06);font-family:monospace}
.tp-id{font-size:11px;color:rgba(255,178,77,.5);font-family:monospace}
.tp-grow{flex:1 1 auto}
.tp-btn{font-size:12px;padding:6px 12px;border-radius:5px;border:1px solid var(--line);background:rgba(255,178,77,.06);color:var(--ty);cursor:pointer;transition:.15s}
.tp-btn:hover{background:rgba(255,178,77,.16)}
.tp-btn.danger{color:var(--bad);border-color:rgba(255,71,111,.35)}
.tp-btn.danger:hover{background:rgba(255,71,111,.12)}
.tp-desc{margin-top:10px;font-size:12px;color:var(--ty2);opacity:.85;line-height:1.5}
.tp-empty{font-size:13px;color:var(--ty2);opacity:.7;padding:18px;text-align:center}
`;

export async function render(mainEl) {
  loadStyle('tp-style', CSS);
  mainEl.innerHTML = `
    <div class="tp-wrap">
      <div class="tp-hud">
        <div class="tp-emb"><div class="tp-core"></div>
          <div class="tp-orbit"><div class="tp-node"></div></div>
          <div class="tp-orbit" style="animation-duration:4.4s;animation-direction:reverse"><div class="tp-node" style="background:var(--ty)"></div></div>
        </div>
        <div class="tp-htext">
          <h2>${esc(L.title)}</h2>
          <div class="sub">${esc(L.sub)}</div>
          <div class="stat"><span class="tp-dot"></span><span id="tp-count">0 ${esc(L.count_label)}</span> · ${esc(L.status_online)}</div>
        </div>
      </div>

      <div class="tp-panel">
        <div class="tp-sec">${esc(L.install_h)}</div>
        <div class="tp-drop" id="tp-drop">${esc(L.install_drop)}</div>
        <input type="file" id="tp-file" accept=".fwpack,.zip" style="display:none">
        <div class="tp-hint">${esc(L.install_hint)}</div>
        <div class="tp-msg" id="tp-install-msg"></div>
      </div>

      <div id="tp-sidecar"></div>
      <div id="tp-list"></div>
    </div>`;

  const drop = mainEl.querySelector('#tp-drop');
  const file = mainEl.querySelector('#tp-file');
  drop.onclick = () => file.click();
  drop.ondragover = (e) => { e.preventDefault(); drop.classList.add('over'); };
  drop.ondragleave = () => drop.classList.remove('over');
  drop.ondrop = (e) => { e.preventDefault(); drop.classList.remove('over'); if (e.dataTransfer.files[0]) install(mainEl, e.dataTransfer.files[0]); };
  file.onchange = () => { if (file.files[0]) install(mainEl, file.files[0]); };

  await load(mainEl);
  await loadSidecar(mainEl);
}

// loadSidecar — tampilin SIDECAR TOOLS (native, folder self-contained, akses semua agent).
// Beda dari .fwpack (sandbox upload): ini di tools/<name>/ → binary terpisah. Owner 2026-06-23.
async function loadSidecar(mainEl) {
  const el = mainEl.querySelector('#tp-sidecar');
  if (!el) return;
  let data;
  try { data = await fetchJSON('/api/tools/sidecar', { method: 'POST' }); } catch (e) { el.innerHTML = ''; return; }
  const tools = (data && data.tools) || [];
  if (!tools.length) { el.innerHTML = ''; return; }
  el.innerHTML = `<div class="tp-panel"><div class="tp-sec">⚙️ Sidecar Tools · ${tools.length}</div>`
    + `<div class="tp-hint">Native, self-contained (folder <code>tools/&lt;name&gt;/</code>, binary terpisah) — bisa diakses SEMUA agent. Tambah: taruh folder + <code>tools/build-tools.sh</code>.</div></div>`
    + tools.sort((a, b) => String(a.name).localeCompare(b.name)).map(sidecarCardHTML).join('');
}

function sidecarCardHTML(t) {
  return `<div class="tp-panel">
    <div class="tp-card">
      <h3>⚙️ ${esc(t.name)}</h3>
      <span class="tp-tag">${t.capability ? esc(t.capability) : 'semua agent'}</span>
      <span class="tp-id">${fmt('params_label', { n: t.params || 0 })}</span>
      <span class="tp-grow"></span>
      <span class="tp-id" style="opacity:.55">sidecar</span>
    </div>
    ${t.description ? `<div class="tp-desc">${esc(t.description)}</div>` : ''}
  </div>`;
}

async function load(mainEl) {
  const list = mainEl.querySelector('#tp-list');
  let data;
  try {
    data = await fetchJSON('/api/tools/installed');
  } catch (e) {
    list.innerHTML = `<div class="tp-panel"><div class="tp-empty">${esc(String(e))}</div></div>`;
    return;
  }
  // tool MCP (mcp_<id>_<name>) dikelola di Connections — saring dari sini.
  const items = ((data && data.installed) || []).filter((tl) => !String(tl.name || '').startsWith('mcp_'));
  mainEl.querySelector('#tp-count').textContent = `${items.length} ${L.count_label}`;
  if (!items.length) {
    list.innerHTML = `<div class="tp-panel"><div class="tp-empty">${esc(L.empty)}</div></div>`;
    return;
  }
  list.innerHTML = items.map(cardHTML).join('');
  items.forEach((tl) => {
    const card = mainEl.querySelector(`[data-tool="${escAttr(tl.name)}"]`);
    if (!card) return;
    const b = card.querySelector('[data-act="uninstall"]');
    if (b) b.onclick = () => uninstall(mainEl, tl.name);
  });
}

function cardHTML(tl) {
  return `<div class="tp-panel" data-tool="${escAttr(tl.name)}">
    <div class="tp-card">
      <h3>🔧 ${esc(tl.name)}</h3>
      ${tl.capability ? `<span class="tp-tag">${esc(tl.capability)}</span>` : ''}
      <span class="tp-id">${fmt('params_label', { n: tl.params || 0 })}</span>
      <span class="tp-grow"></span>
      <button class="tp-btn danger" data-act="uninstall">${esc(L.btn_uninstall)}</button>
    </div>
    ${tl.description ? `<div class="tp-desc">${esc(tl.description)}</div>` : ''}
  </div>`;
}

async function install(mainEl, f) {
  const msg = mainEl.querySelector('#tp-install-msg');
  msg.style.color = 'var(--ty2)';
  msg.textContent = L.installing;
  const fd = new FormData();
  fd.append('file', f);
  try {
    const r = await fetch('/api/tools/install', { method: 'POST', body: fd });
    const j = await r.json();
    if (!r.ok || j.error) throw new Error(j.error || ('HTTP ' + r.status));
    msg.style.color = 'var(--ty2)';
    msg.textContent = fmt('install_ok', { tool: j.tool || '?' });
    await load(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)';
    msg.textContent = L.install_fail + e;
  }
}

async function uninstall(mainEl, name) {
  if (!confirm(fmt('uninstall_confirm', { tool: name }))) return;
  try {
    await fetchJSON('/api/tools/uninstall?tool=' + encodeURIComponent(name), { method: 'POST' });
    await load(mainEl);
  } catch (e) {
    alert(L.uninstall_fail + e);
  }
}
