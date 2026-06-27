// connections.js — tab "Connections": the gateway gallery (clean glass-3D, full-width).
//
// Every way the outside world reaches an agent — Telegram, Discord, email, CLI,
// schedule, MCP — is a CONNECTOR: a self-contained dumb pipe in its own folder.
// This tab lists them and runs their lifecycle: install (.fwpack) · enable/disable ·
// config (token, stored only in the connector's own folder) · uninstall.
//
// All copy goes through the i18n dictionary (en base + id) — no hardcoded strings.
// UI memakai design-system fw-* share (ensureGlass) — konsisten lintas tab.
//
// API: GET /api/connections · POST /api/connections/{toggle,config,uninstall} ·
// install reuses the uniform .fwpack gerbang at /api/plugins/install (kind:channel).

import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { ensureGlass } from '/js/glass.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('connections.' + String(k)) });
const fmt = (k, vars) => Object.entries(vars || {}).reduce((s, [n, v]) => s.replaceAll('{' + n + '}', v), L[k]);

// Minimal scoped CSS — only what fw-* tidak sediakan (cat header + cfg collapse).
const CSS = `
.cx-cat { font-size:.78rem; font-weight:700; letter-spacing:.06em; color:var(--accent); margin:26px 0 12px; padding-bottom:6px; border-bottom:1px solid var(--glass-border); }
.cx-cat:first-of-type { margin-top:8px; }
.cx-cfg { margin-top:13px; padding-top:13px; border-top:1px solid var(--glass-border); display:none; }
.cx-cfg.open { display:block; }
.cx-cfg label { display:block; font-size:.74rem; color:var(--text-muted); margin:11px 0 4px; }
.cx-state { font-size:.7rem; font-weight:700; letter-spacing:.3px; padding:3px 10px; border-radius:999px; display:inline-flex; align-items:center; gap:6px; }
.cx-state.on { color:#34d399; border:1px solid color-mix(in srgb,#34d399 45%, transparent); background:color-mix(in srgb,#34d399 14%, transparent); }
.cx-state.off { color:#fbbf24; border:1px solid color-mix(in srgb,#fbbf24 45%, transparent); background:color-mix(in srgb,#fbbf24 14%, transparent); }
`;

export async function render(mainEl) {
  ensureGlass();
  loadStyle('cx-style', CSS);
  mainEl.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">🔌</span>
        <div>
          <h2 class="fw-title">${esc(L.title)}</h2>
          <div class="fw-sub">${esc(L.sub)}</div>
          <div class="fw-stat"><span class="fw-dot"></span><span id="cx-count">0 ${esc(L.count_label)}</span> · ${esc(L.status_online)}</div>
        </div>
      </div>

      <div class="cx-cat">${esc(L.cat_channels)}</div>
      <div class="fw-card">
        <div class="fw-sec">${esc(L.install_h)}</div>
        <div class="fw-drop" id="cx-drop">${esc(L.install_drop)}</div>
        <input type="file" id="cx-file" accept=".fwpack,.zip" style="display:none">
        <div class="fw-sub" style="margin-top:10px">${esc(L.install_hint)}</div>
        <div class="fw-msg" id="cx-install-msg" style="font-size:.8rem;margin-top:10px;min-height:16px"></div>
      </div>

      <div id="cx-list" class="fw-grid"></div>

      <div class="cx-cat">${esc(L.cat_mcp)}</div>
      <div class="fw-card">
        <div class="fw-sec">${esc(L.mcp_install_h)}</div>
        <div class="fw-sub">${esc(L.mcp_install_hint)}</div>
        <textarea id="cx-mcp-json" class="fw-input" style="min-height:92px;margin-top:9px;font-family:ui-monospace,monospace;resize:vertical" spellcheck="false" placeholder='{ "github": { "command": "npx", "args": ["-y","@modelcontextprotocol/server-github"], "env": { "GITHUB_TOKEN": "..." } } }'></textarea>
        <div style="margin-top:11px;display:flex;gap:10px;align-items:center"><button class="fw-btn" id="cx-mcp-install">${esc(L.mcp_install_btn)}</button>
          <span class="fw-msg" id="cx-mcp-msg" style="font-size:.8rem;margin:0"></span></div>
      </div>

      <div id="cx-mcp-list" class="fw-grid"></div>
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
    list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(String(e))}</div></div>`;
    return;
  }
  const conns = (data && data.connectors) || [];
  if (!conns.length) {
    list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(L.mcp_empty)}</div></div>`;
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
  return `<div class="fw-card" data-mcp="${escAttr(c.id)}">
    <div class="fw-row">
      <h3>${esc(c.id)} <span class="fw-tag">mcp</span></h3>
      <span class="fw-id">${esc(c.command)}${(c.env_keys && c.env_keys.length) ? ' · env: ' + esc(c.env_keys.join(',')) : ''}</span>
      <span class="fw-grow"></span>
      <span class="cx-state ${running ? 'on' : 'off'}"><span class="fw-dot"></span>${running ? esc(L.state_on) : esc(L.state_off)}</span>
      <button class="fw-btn" data-act="toggle">${on ? esc(L.btn_disable) : esc(L.btn_enable)}</button>
      <button class="fw-btn danger" data-act="uninstall">${esc(L.btn_uninstall)}</button>
    </div>
    ${(c.tools && c.tools.length) ? `<div class="fw-desc">${esc(String(c.tools.length))} ${esc(L.mcp_tools_label)}: ${esc(c.tools.join(', '))}</div>` : ''}
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
  msg.style.color = 'var(--text-muted)'; msg.textContent = L.installing;
  try {
    for (const id of ids) {
      const s = servers[id] || {};
      await fetchJSON('/api/mcp/install', { method: 'POST', body: JSON.stringify({ id: id.toLowerCase(), command: s.command, args: s.args || [], env: s.env || {} }) });
      await fetchJSON('/api/mcp/enable', { method: 'POST', body: JSON.stringify({ id: id.toLowerCase() }) });
    }
    msg.style.color = 'var(--accent)'; msg.textContent = fmt('mcp_install_ok', { n: ids.length });
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
    list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(String(e))}</div></div>`;
    return;
  }
  const conns = (data && data.connectors) || [];
  mainEl.querySelector('#cx-count').textContent = `${conns.length} ${L.count_label}`;
  if (!conns.length) {
    list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(L.empty)}</div></div>`;
    return;
  }
  list.innerHTML = conns.map(cardHTML).join('');
  conns.forEach((c) => wireCard(mainEl, c.id));
}

function cardHTML(c) {
  const on = !!c.enabled;
  const native = c.kind === 'native';
  return `<div class="fw-card" data-card="${escAttr(c.id)}">
    <div class="fw-row">
      <h3>${esc(c.name || c.id)} <span class="fw-tag">${native ? esc(L.native_label) : esc(L.kind_label)}</span></h3>
      <span class="fw-id">${esc(c.id)}${c.version ? ' · v' + esc(c.version) : ''}</span>
      <span class="fw-grow"></span>
      <span class="cx-state ${on ? 'on' : 'off'}"><span class="fw-dot"></span>${on ? esc(L.state_on) : esc(L.state_off)}</span>
      ${native ? '' : `<button class="fw-btn" data-act="toggle">${on ? esc(L.btn_disable) : esc(L.btn_enable)}</button>`}
      <button class="fw-btn" data-act="config">${esc(L.btn_config)}</button>
      ${native ? '' : `<button class="fw-btn danger" data-act="uninstall">${esc(L.btn_uninstall)}</button>`}
    </div>
    <div class="cx-cfg" data-cfg>
      <div class="fw-sub">${esc(L.cfg_note)}</div>
      ${fieldsHTML(c)}
      <div style="margin-top:11px;display:flex;gap:8px;align-items:center">
        <button class="fw-btn" data-act="save">${esc(L.cfg_save)}</button>
        <button class="fw-btn" data-act="cfgclose">${esc(L.cfg_close)}</button>
        <span class="fw-msg" data-cfgmsg style="font-size:.8rem;margin:0"></span>
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
    return `<div class="fw-sub">${esc(L.no_fields)}</div>`;
  }
  return schema.map((f) => {
    const secret = f.type === 'secret';
    const cur = vals[f.key] || '';
    const ph = secret ? (cur || esc(L.cfg_token_ph)) : (f.default || '');
    return `<label>${esc(f.label || f.key)}${f.help ? ` <span style="opacity:.6;font-weight:400">— ${esc(f.help)}</span>` : ''}</label>
      <input class="fw-input" data-key="${escAttr(f.key)}" data-secret="${secret ? 1 : 0}" type="${secret ? 'password' : 'text'}"
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
    msg.style.color = 'var(--accent)';
    msg.textContent = L.cfg_saved;
    await load(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)';
    msg.textContent = L.cfg_fail + e;
  }
}

async function install(mainEl, f) {
  const msg = mainEl.querySelector('#cx-install-msg');
  msg.style.color = 'var(--text-muted)';
  msg.textContent = L.installing;
  const fd = new FormData();
  fd.append('file', f);
  try {
    const r = await fetch('/api/plugins/install', { method: 'POST', body: fd });
    const j = await r.json();
    if (!r.ok || j.error) throw new Error(j.error || ('HTTP ' + r.status));
    msg.style.color = 'var(--accent)';
    msg.textContent = fmt('install_ok', { id: j.connector || j.plugin || '?' });
    await load(mainEl);
  } catch (e) {
    msg.style.color = 'var(--bad)';
    msg.textContent = L.install_fail + e;
  }
}
