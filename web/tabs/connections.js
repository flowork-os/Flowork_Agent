// connections.js — tab "Connections": the gateway gallery.
//
// Every way the outside world reaches an agent — Telegram, Discord, email, CLI,
// schedule, MCP — is a CONNECTOR: a self-contained dumb pipe in its own folder.
// This tab lists them and runs their lifecycle: install (.fwpack) · enable/disable ·
// config (token, stored only in the connector's own folder) · uninstall.
//
// All copy goes through the i18n dictionary (en base + id) — no hardcoded strings.
// Look: "Jarvis" HUD to match the rest of the app.
//
// API: GET /api/connections · POST /api/connections/{toggle,config,uninstall} ·
// install reuses the uniform .fwpack gerbang at /api/plugins/install (kind:channel).

import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('connections.' + String(k)) });
const fmt = (k, vars) => Object.entries(vars || {}).reduce((s, [n, v]) => s.replaceAll('{' + n + '}', v), L[k]);

const CSS = `
.cx-wrap{position:relative;max-width:1000px;padding:6px 2px 40px;
  --cy:#36e6ff;--cy2:#26ffd0;--line:rgba(54,230,255,.22);--bad:#ff476f;--warn:#ffc24d}
.cx-wrap *{position:relative;z-index:1}
.cx-hud{display:flex;align-items:center;gap:16px;margin:6px 0 22px}
.cx-emb{width:54px;height:54px;flex:0 0 auto;position:relative}
.cx-core{position:absolute;top:50%;left:50%;width:14px;height:14px;margin:-7px 0 0 -7px;border-radius:50%;
  background:radial-gradient(circle,#aef6ff 0,var(--cy) 42%,transparent 72%);
  box-shadow:0 0 12px var(--cy),0 0 26px rgba(54,230,255,.5);animation:cxpulse 2.4s ease-in-out infinite}
.cx-orbit{position:absolute;inset:0;animation:cxspin 7s linear infinite}
.cx-node{position:absolute;top:-3px;left:50%;width:6px;height:6px;margin-left:-3px;border-radius:50%;
  background:var(--cy2);box-shadow:0 0 8px var(--cy2)}
@keyframes cxspin{to{transform:rotate(360deg)}}
@keyframes cxpulse{0%,100%{transform:scale(1)}50%{transform:scale(1.18);filter:brightness(1.3)}}
.cx-htext h2{margin:0;font-family:var(--disp,inherit);font-size:18px;letter-spacing:4px;color:#eafdff;text-shadow:0 0 12px rgba(54,230,255,.45)}
.cx-htext .sub{font-size:12px;color:var(--cy2);opacity:.82;margin-top:5px;max-width:680px;line-height:1.5}
.cx-htext .stat{font-size:10px;letter-spacing:2px;color:var(--cy2);margin-top:7px;display:flex;align-items:center;gap:7px}
.cx-dot{width:7px;height:7px;border-radius:50%;background:var(--cy2);box-shadow:0 0 8px var(--cy2);animation:cxblink 1.6s ease-in-out infinite}
@keyframes cxblink{0%,100%{opacity:1}50%{opacity:.25}}
.cx-panel{position:relative;background:rgba(4,14,22,.6);border:1px solid var(--line);border-radius:8px;padding:15px 17px;margin-bottom:15px;backdrop-filter:blur(3px)}
.cx-panel::before,.cx-panel::after{content:'';position:absolute;width:12px;height:12px;border:2px solid var(--cy);opacity:.7;pointer-events:none}
.cx-panel::before{top:-1px;left:-1px;border-right:0;border-bottom:0}
.cx-panel::after{bottom:-1px;right:-1px;border-left:0;border-top:0}
.cx-sec{font-size:10px;letter-spacing:2px;color:var(--cy);margin:0 0 9px;text-shadow:0 0 8px rgba(54,230,255,.3)}
.cx-drop{border:1px dashed var(--line);border-radius:7px;padding:18px;text-align:center;color:var(--cy2);cursor:pointer;font-size:13px;transition:.2s}
.cx-drop:hover,.cx-drop.over{background:rgba(54,230,255,.07);border-color:var(--cy)}
.cx-hint{font-size:11px;color:rgba(54,230,255,.55);margin-top:8px}
.cx-msg{font-size:12px;margin-top:9px;min-height:16px}
.cx-card{display:flex;align-items:center;gap:14px;flex-wrap:wrap}
.cx-card h3{margin:0;font-size:15px;color:#eafdff;display:flex;align-items:center;gap:9px}
.cx-tag{font-size:10px;letter-spacing:1px;color:var(--cy);border:1px solid var(--line);border-radius:3px;padding:2px 7px;background:rgba(54,230,255,.06)}
.cx-id{font-size:11px;color:rgba(54,230,255,.5);font-family:monospace}
.cx-state{font-size:10px;letter-spacing:2px;padding:3px 9px;border-radius:3px;display:inline-flex;align-items:center;gap:6px}
.cx-state.on{color:var(--cy2);border:1px solid rgba(38,255,208,.4);background:rgba(38,255,208,.07)}
.cx-state.off{color:var(--warn);border:1px solid rgba(255,194,77,.35);background:rgba(255,194,77,.06)}
.cx-grow{flex:1 1 auto}
.cx-btn{font-size:12px;padding:6px 12px;border-radius:5px;border:1px solid var(--line);background:rgba(54,230,255,.06);color:var(--cy);cursor:pointer;transition:.15s}
.cx-btn:hover{background:rgba(54,230,255,.16)}
.cx-btn.danger{color:var(--bad);border-color:rgba(255,71,111,.35)}
.cx-btn.danger:hover{background:rgba(255,71,111,.12)}
.cx-cfg{margin-top:13px;padding-top:13px;border-top:1px solid var(--line);display:none}
.cx-cfg.open{display:block}
.cx-cfg label{display:block;font-size:11px;color:var(--cy2);margin:9px 0 4px;letter-spacing:1px}
.cx-cfg input{width:100%;box-sizing:border-box;padding:8px 10px;border-radius:5px;border:1px solid var(--line);background:rgba(2,10,16,.7);color:#dffaff;font-size:13px}
.cx-cfg .note{font-size:11px;color:rgba(54,230,255,.55);margin-top:7px}
.cx-empty{font-size:13px;color:var(--cy2);opacity:.7;padding:18px;text-align:center}
.cx-cat{font-size:11px;letter-spacing:3px;color:var(--cy);margin:26px 0 12px;padding-bottom:6px;border-bottom:1px solid var(--line);text-shadow:0 0 8px rgba(54,230,255,.3)}
.cx-cat:first-child{margin-top:8px}
.cx-ta{width:100%;box-sizing:border-box;min-height:92px;margin-top:9px;padding:10px;border-radius:6px;border:1px solid var(--line);background:rgba(2,10,16,.7);color:#dffaff;font:12px/1.5 monospace;resize:vertical}
.cx-tools{font-size:11px;color:var(--cy2);opacity:.8;margin-top:8px;word-break:break-word}
`;

export async function render(mainEl) {
  loadStyle('cx-style', CSS);
  mainEl.innerHTML = `
    <div class="cx-wrap">
      <div class="cx-hud">
        <div class="cx-emb"><div class="cx-core"></div>
          <div class="cx-orbit"><div class="cx-node"></div></div>
          <div class="cx-orbit" style="animation-duration:4.4s;animation-direction:reverse"><div class="cx-node" style="background:var(--cy)"></div></div>
        </div>
        <div class="cx-htext">
          <h2>${esc(L.title)}</h2>
          <div class="sub">${esc(L.sub)}</div>
          <div class="stat"><span class="cx-dot"></span><span id="cx-count">0 ${esc(L.count_label)}</span> · ${esc(L.status_online)}</div>
        </div>
      </div>

      <div class="cx-cat">${esc(L.cat_channels)}</div>
      <div class="cx-panel">
        <div class="cx-sec">${esc(L.install_h)}</div>
        <div class="cx-drop" id="cx-drop">${esc(L.install_drop)}</div>
        <input type="file" id="cx-file" accept=".fwpack,.zip" style="display:none">
        <div class="cx-hint">${esc(L.install_hint)}</div>
        <div class="cx-msg" id="cx-install-msg"></div>
      </div>

      <div id="cx-list"></div>

      <div class="cx-cat">${esc(L.cat_mcp)}</div>
      <div class="cx-panel">
        <div class="cx-sec">${esc(L.mcp_install_h)}</div>
        <div class="cx-hint">${esc(L.mcp_install_hint)}</div>
        <textarea id="cx-mcp-json" class="cx-ta" spellcheck="false" placeholder='{ "github": { "command": "npx", "args": ["-y","@modelcontextprotocol/server-github"], "env": { "GITHUB_TOKEN": "..." } } }'></textarea>
        <div style="margin-top:9px"><button class="cx-btn" id="cx-mcp-install">${esc(L.mcp_install_btn)}</button>
          <span class="cx-msg" id="cx-mcp-msg" style="margin:0 0 0 10px"></span></div>
      </div>

      <div id="cx-mcp-list"></div>
    </div>`;

  const drop = mainEl.querySelector('#cx-drop');
  const file = mainEl.querySelector('#cx-file');
  drop.onclick = () => file.click();
  drop.ondragover = (e) => { e.preventDefault(); drop.classList.add('over'); };
  drop.ondragleave = () => drop.classList.remove('over');
  drop.ondrop = (e) => { e.preventDefault(); drop.classList.remove('over'); if (e.dataTransfer.files[0]) install(mainEl, e.dataTransfer.files[0]); };
  file.onchange = () => { if (file.files[0]) install(mainEl, file.files[0]); };

  mainEl.querySelector('#cx-mcp-install').onclick = () => mcpInstall(mainEl);

  await load(mainEl);
  await loadMCP(mainEl);
}

// ── MCP connectors (Jenis 2: external MCP servers as agent tool-sources) ──────
async function loadMCP(mainEl) {
  const list = mainEl.querySelector('#cx-mcp-list');
  let data;
  try {
    data = await fetchJSON('/api/mcp');
  } catch (e) {
    list.innerHTML = `<div class="cx-empty">${esc(String(e))}</div>`;
    return;
  }
  const conns = (data && data.connectors) || [];
  if (!conns.length) {
    list.innerHTML = `<div class="cx-panel"><div class="cx-empty">${esc(L.mcp_empty)}</div></div>`;
    return;
  }
  list.innerHTML = conns.map(mcpCardHTML).join('');
  conns.forEach((c) => {
    const card = mainEl.querySelector(`[data-mcp="${c.id}"]`);
    if (!card) return;
    card.querySelector('[data-act="toggle"]').onclick = () => mcpToggle(mainEl, c.id, !c.enabled);
    card.querySelector('[data-act="uninstall"]').onclick = () => mcpUninstall(mainEl, c.id);
  });
}

function mcpCardHTML(c) {
  const on = !!c.enabled;
  const running = !!c.running;
  return `<div class="cx-panel" data-mcp="${escAttr(c.id)}">
    <div class="cx-card">
      <h3>${esc(c.id)} <span class="cx-tag">mcp</span></h3>
      <span class="cx-id">${esc(c.command)}${(c.env_keys && c.env_keys.length) ? ' · env: ' + esc(c.env_keys.join(',')) : ''}</span>
      <span class="cx-grow"></span>
      <span class="cx-state ${running ? 'on' : 'off'}"><span class="cx-dot"></span>${running ? esc(L.state_on) : esc(L.state_off)}</span>
      <button class="cx-btn" data-act="toggle">${on ? esc(L.btn_disable) : esc(L.btn_enable)}</button>
      <button class="cx-btn danger" data-act="uninstall">${esc(L.btn_uninstall)}</button>
    </div>
    ${(c.tools && c.tools.length) ? `<div class="cx-tools">${esc(String(c.tools.length))} ${esc(L.mcp_tools_label)}: ${esc(c.tools.join(', '))}</div>` : ''}
  </div>`;
}

async function mcpInstall(mainEl) {
  const msg = mainEl.querySelector('#cx-mcp-msg');
  const raw = mainEl.querySelector('#cx-mcp-json').value.trim();
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (e) {
    msg.style.color = 'var(--bad)'; msg.textContent = L.mcp_bad_json + e; return;
  }
  // accept either {mcpServers:{...}} or {...} directly
  const servers = parsed.mcpServers || parsed;
  const ids = Object.keys(servers || {});
  if (!ids.length) { msg.style.color = 'var(--bad)'; msg.textContent = L.mcp_bad_json; return; }
  msg.style.color = 'var(--cy2)'; msg.textContent = L.installing;
  try {
    for (const id of ids) {
      const s = servers[id] || {};
      await fetchJSON('/api/mcp/install', { method: 'POST', body: JSON.stringify({ id: id.toLowerCase(), command: s.command, args: s.args || [], env: s.env || {} }) });
      await fetchJSON('/api/mcp/enable', { method: 'POST', body: JSON.stringify({ id: id.toLowerCase() }) });
    }
    msg.style.color = 'var(--cy2)'; msg.textContent = fmt('mcp_install_ok', { n: ids.length });
    mainEl.querySelector('#cx-mcp-json').value = '';
    await loadMCP(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)'; msg.textContent = L.mcp_install_fail + e;
  }
}

async function mcpToggle(mainEl, id, enable) {
  try {
    await fetchJSON('/api/mcp/' + (enable ? 'enable' : 'disable'), { method: 'POST', body: JSON.stringify({ id }) });
    await loadMCP(mainEl);
  } catch (e) { alert(L.toggle_fail + e); }
}

async function mcpUninstall(mainEl, id) {
  if (!confirm(fmt('uninstall_confirm', { id }))) return;
  try {
    await fetchJSON('/api/mcp/uninstall', { method: 'POST', body: JSON.stringify({ id }) });
    await loadMCP(mainEl);
  } catch (e) { alert(L.uninstall_fail + e); }
}

async function load(mainEl) {
  const list = mainEl.querySelector('#cx-list');
  let data;
  try {
    data = await fetchJSON('/api/connections');
  } catch (e) {
    list.innerHTML = `<div class="cx-empty">${esc(String(e))}</div>`;
    return;
  }
  const conns = (data && data.connectors) || [];
  mainEl.querySelector('#cx-count').textContent = `${conns.length} ${L.count_label}`;
  if (!conns.length) {
    list.innerHTML = `<div class="cx-panel"><div class="cx-empty">${esc(L.empty)}</div></div>`;
    return;
  }
  list.innerHTML = conns.map(cardHTML).join('');
  conns.forEach((c) => wireCard(mainEl, c.id));
}

function cardHTML(c) {
  const on = !!c.enabled;
  const native = c.kind === 'native';
  return `<div class="cx-panel" data-card="${escAttr(c.id)}">
    <div class="cx-card">
      <h3>${esc(c.name || c.id)} <span class="cx-tag">${native ? esc(L.native_label) : esc(L.kind_label)}</span></h3>
      <span class="cx-id">${esc(c.id)}${c.version ? ' · v' + esc(c.version) : ''}</span>
      <span class="cx-grow"></span>
      <span class="cx-state ${on ? 'on' : 'off'}"><span class="cx-dot"></span>${on ? esc(L.state_on) : esc(L.state_off)}</span>
      ${native ? '' : `<button class="cx-btn" data-act="toggle">${on ? esc(L.btn_disable) : esc(L.btn_enable)}</button>`}
      <button class="cx-btn" data-act="config">${esc(L.btn_config)}</button>
      ${native ? '' : `<button class="cx-btn danger" data-act="uninstall">${esc(L.btn_uninstall)}</button>`}
    </div>
    <div class="cx-cfg" data-cfg>
      <div class="note">${esc(L.cfg_note)}</div>
      ${fieldsHTML(c)}
      <div style="margin-top:11px;display:flex;gap:8px;align-items:center">
        <button class="cx-btn" data-act="save">${esc(L.cfg_save)}</button>
        <button class="cx-btn" data-act="cfgclose">${esc(L.cfg_close)}</button>
        <span class="cx-msg" data-cfgmsg style="margin:0"></span>
      </div>
    </div>
  </div>`;
}

// fieldsHTML renders a connector's config inputs FROM ITS MANIFEST SCHEMA — the
// kernel/GUI never hardcode a connector's keys. Secret fields render empty with the
// masked current value as placeholder (so the owner sees it's set but only sends a
// new one when they type); text fields prefill with the current value.
function fieldsHTML(c) {
  const schema = c.config || [];
  const vals = c.values || {};
  if (!schema.length) {
    return `<div class="note">${esc(L.no_fields)}</div>`;
  }
  return schema.map((f) => {
    const secret = f.type === 'secret';
    const cur = vals[f.key] || '';
    const ph = secret ? (cur || esc(L.cfg_token_ph)) : (f.default || '');
    return `<label>${esc(f.label || f.key)}${f.help ? ` <span style="opacity:.6;font-weight:400">— ${esc(f.help)}</span>` : ''}</label>
      <input data-key="${escAttr(f.key)}" data-secret="${secret ? 1 : 0}" type="${secret ? 'password' : 'text'}"
        value="${secret ? '' : escAttr(cur)}" placeholder="${escAttr(ph)}" autocomplete="off">`;
  }).join('');
}

function wireCard(mainEl, id) {
  // id is validated to [a-z0-9_-] server-side, so it is safe in an attribute selector.
  const card = mainEl.querySelector(`[data-card="${id}"]`);
  if (!card) return;
  const on = card.querySelector('.cx-state').classList.contains('on');
  const toggleBtn = card.querySelector('[data-act="toggle"]');
  if (toggleBtn) toggleBtn.onclick = () => toggle(mainEl, id, !on);
  const uninstallBtn = card.querySelector('[data-act="uninstall"]');
  if (uninstallBtn) uninstallBtn.onclick = () => uninstall(mainEl, id);
  const cfg = card.querySelector('[data-cfg]');
  card.querySelector('[data-act="config"]').onclick = () => cfg.classList.toggle('open');
  card.querySelector('[data-act="cfgclose"]').onclick = () => cfg.classList.remove('open');
  card.querySelector('[data-act="save"]').onclick = () => saveCfg(mainEl, id, card);
}

async function toggle(mainEl, id, enabled) {
  try {
    await fetchJSON('/api/connections/toggle', { method: 'POST', body: JSON.stringify({ id, enabled }) });
    await load(mainEl);
  } catch (e) { alert(L.toggle_fail + e); }
}

async function uninstall(mainEl, id) {
  if (!confirm(fmt('uninstall_confirm', { id }))) return;
  try {
    await fetchJSON('/api/connections/uninstall', { method: 'POST', body: JSON.stringify({ id }) });
    await load(mainEl);
  } catch (e) { alert(L.uninstall_fail + e); }
}

async function saveCfg(mainEl, id, card) {
  // Collect schema fields. A secret left blank means "unchanged" — never send it, or
  // we'd overwrite the real token with the masked placeholder.
  const cfg = {};
  card.querySelectorAll('[data-key]').forEach((inp) => {
    const v = inp.value.trim();
    const isSecret = inp.getAttribute('data-secret') === '1';
    if (isSecret && v === '') return;
    cfg[inp.getAttribute('data-key')] = v;
  });
  const msg = card.querySelector('[data-cfgmsg]');
  try {
    await fetchJSON('/api/connections/config', { method: 'POST', body: JSON.stringify({ id, config: cfg }) });
    msg.style.color = 'var(--cy2)';
    msg.textContent = L.cfg_saved;
    await load(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)';
    msg.textContent = L.cfg_fail + e;
  }
}

async function install(mainEl, f) {
  const msg = mainEl.querySelector('#cx-install-msg');
  msg.style.color = 'var(--cy2)';
  msg.textContent = L.installing;
  const fd = new FormData();
  fd.append('file', f);
  try {
    const r = await fetch('/api/plugins/install', { method: 'POST', body: fd });
    const j = await r.json();
    if (!r.ok || j.error) throw new Error(j.error || ('HTTP ' + r.status));
    msg.style.color = 'var(--cy2)';
    msg.textContent = fmt('install_ok', { id: j.connector || j.plugin || '?' });
    await load(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)';
    msg.textContent = L.install_fail + e;
  }
}
