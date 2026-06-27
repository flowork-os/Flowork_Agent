// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-31 (re-audited 2026-06-07, re-locked 2026-06-11)
// Update 2026-06-11 (owner-approved): API Keys preset chips (per-service, pre-fill
//   the exact env var name, green when set) + new "Router & Model" segment
//   (renderRouterDefault → /api/settings/router-default) for the global default
//   model + router URL. All values esc/escAttr'd, labels via i18n, no inline secrets.
// Reason: Tab Settings (Akun/API-Keys/Notif/YouTube). Audit pass — esc/escAttr
//   semua field, fetchJSON util, API key masked, label via i18n. E2E verified
//   lewat instance isolated. 2026-06-06 (owner-approved): segmen Wallet
//   Personal + Wallet AI DIBUANG (fitur wallet dihapus).
// Update 2026-06-07 (owner-approved audit): the YouTube OAuth poll (setInterval
//   /api/settings/youtube every 2s) was never cancelled when you switched segment —
//   it kept firing in the background and could re-render YouTube over another
//   segment. Now tracked at module scope (ytPoll/stopYtPoll) and cancelled on
//   segment switch, on re-render, and before starting a new poll.
//
// Update 2026-06-16 (owner-approved, re-locked): segment "Wallet" (renderFinance →
//   /api/finance/wallet) — R8 self-finance fase-1: kredensial wallet EVM (address publik +
//   private key masked di secret store + chain/RPC/currency + hard-limit). Transaksi = fase-2.
//
// Update 2026-06-27 (owner-approved): GUI redesign → clean glass-3D full-width pakai
//   design-system share fw-* (ensureGlass dari /js/glass.js). HANYA markup/CSS yang
//   berubah — SEMUA endpoint, i18n key, element ID, data-attr, handler, sub-tab switching,
//   dan logic JS DIPERTAHANKAN apa adanya (ID = load-bearing buat baca/simpan settings).
//
// settings.js — halaman Settings GLOBAL (owner-level).
//
// Section (sub-tab internal): Akun & Keamanan, API Keys, Notifikasi, YouTube, Wallet.
//
// Data owner-level disimpan di flowork.db global (lewat /api/settings/*).
// AI agent TETAP terisolasi (warga punya store + channel sendiri).
//
// Semua label lewat dictionary i18n (t(...)) — no hardcode UI text.

import { t } from '/js/i18n.js';
import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { ensureGlass } from '/js/glass.js';

// Scoped CSS: ONLY the sub-tab nav pills + a few layout helpers that the shared
// fw-* kit doesn't cover. No raw hex — CSS variables from the design-system only.
const CSS = `
.set-bar { display:flex; gap:6px; margin-bottom:18px; padding:6px; flex-wrap:wrap;
  background:var(--bg-panel); border:1px solid var(--glass-border); border-radius:14px; width:fit-content; max-width:100%;
  box-shadow:inset 0 1px 0 rgba(255,255,255,.05); }
.set-btn { padding:8px 15px; font-size:.82rem; font-weight:600; border-radius:9px;
  background:transparent; border:1px solid transparent; color:var(--text-muted); cursor:pointer;
  transition:background .15s,color .15s,border-color .15s; }
.set-btn:hover { background:color-mix(in srgb,var(--accent) 8%, transparent); color:var(--text-main); }
.set-btn.active { background:color-mix(in srgb,var(--accent) 18%, transparent);
  color:var(--text-main); border-color:color-mix(in srgb,var(--accent) 45%, transparent); }
.set-field { display:flex; flex-direction:column; gap:6px; margin-bottom:12px; }
.set-field > label { font-size:.8rem; color:var(--text-muted); }
.set-hint { font-size:.76rem; color:var(--text-muted); margin:-4px 0 10px; line-height:1.45; }
.set-msg { margin-top:10px; font-size:.82rem; min-height:1em; color:var(--text-muted); }
.set-msg.ok { color:var(--text-main); } .set-msg.err { color:#f87171; }
.set-presets { display:flex; gap:6px; flex-wrap:wrap; margin:2px 0 10px; }
.set-preset { padding:5px 11px; font-size:.76rem; border-radius:999px; cursor:pointer;
  background:color-mix(in srgb,var(--accent) 10%, transparent); border:1px solid color-mix(in srgb,var(--accent) 30%, transparent);
  color:var(--accent); transition:background .15s,border-color .15s; }
.set-preset:hover { background:color-mix(in srgb,var(--accent) 20%, transparent); border-color:color-mix(in srgb,var(--accent) 55%, transparent); }
.set-preset.set-on { background:color-mix(in srgb,#34d399 16%, transparent); border-color:color-mix(in srgb,#34d399 45%, transparent); color:#34d399; }
.set-keyrow { display:block; padding:12px 14px; border:1px solid var(--glass-border); border-radius:12px; margin-bottom:8px;
  background:linear-gradient(165deg, rgba(255,255,255,.03), rgba(255,255,255,0) 60%), var(--bg-panel); }
.set-keyhead { display:flex; justify-content:space-between; align-items:center; gap:8px; }
.set-keyenv { font-size:.72rem; color:var(--text-muted); font-family:ui-monospace,monospace; }
.set-on-tag { font-size:.72rem; font-family:ui-monospace,monospace; color:#34d399; }
.set-changeme { font-size:.7rem; font-weight:700; color:#fbbf24; background:color-mix(in srgb,#fbbf24 14%, transparent);
  padding:2px 9px; border-radius:999px; white-space:nowrap; }
.set-keyhint { font-size:.74rem; color:var(--text-muted); margin:6px 0 8px; line-height:1.45; }
.set-keyhint a { color:var(--accent); text-decoration:none; white-space:nowrap; }
.set-keyhint a:hover { text-decoration:underline; }
.set-keyact { display:flex; gap:8px; align-items:center; }
.set-keyin { flex:1; min-width:0; }
.set-swgroup { margin-top:16px; }
.set-swcat { font-size:.72rem; text-transform:uppercase; letter-spacing:.04em; color:var(--accent); margin-bottom:6px; font-weight:700; }
.set-swrow { display:flex; align-items:center; gap:12px; padding:9px 0; border-top:1px solid var(--glass-border); }
.set-swrow .set-swmeta { flex:1; }
.set-swrow .set-swlabel { font-size:.86rem; color:var(--text-main); display:flex; align-items:center; gap:8px; }
.set-swrow .set-swdesc { font-size:.72rem; color:var(--text-muted); margin-top:2px; }
.set-swrow code { font-family:ui-monospace,monospace; opacity:.65; font-size:.92em; }
.set-swrow input[type=text] { width:130px; }
.set-srcbadge { font-size:.66rem; padding:1px 8px; border-radius:999px; font-weight:600; }
.set-saverow { margin-top:16px; display:flex; gap:12px; align-items:center; flex-wrap:wrap; }
.set-toggle { display:flex; align-items:center; gap:9px; cursor:pointer; font-size:.86rem; color:var(--text-main); margin-bottom:12px; }
.set-toggle input[type=checkbox] { width:auto; }
.set-inline { display:flex; gap:10px; align-items:center; flex-wrap:wrap; }
.set-inline label { font-size:.85rem; color:var(--text-muted); min-width:180px; display:inline-block; }
.set-inline input[type=number] { width:120px; flex:none; }
.set-path { font-size:.66rem; color:var(--text-muted); font-family:ui-monospace,monospace; }
.set-card code { font-family:ui-monospace,monospace; background:color-mix(in srgb,var(--bg-panel) 50%, #000); padding:1px 6px; border-radius:5px; font-size:.86em; }
`;

// Known integration keys — friendly platform label → env var name. Clicking a
// chip pre-fills the exact variable name so owners never have to guess it.
// Service names are proper nouns (kept verbatim); instructional text is i18n.
// Known integrations. `hint` + `url` make the keys page self-explanatory for
// non-developers: what the key is for, and exactly where to get it.
const KEY_PRESETS = [
  { label: 'Dev.to', key: 'DEVTO_API_KEY',
    hint: 'Publish articles to Dev.to. Get a key: Dev.to → Settings → Extensions → "Generate API Key".',
    url: 'https://dev.to/settings/extensions' },
  { label: 'X · auth_token', key: 'X_AUTH_TOKEN',
    hint: 'Post to X/Twitter (cookie auth). In a logged-in X tab: DevTools (F12) → Application → Cookies → copy the "auth_token" value.',
    url: 'https://x.com' },
  { label: 'X · ct0', key: 'X_CT0',
    hint: 'X/Twitter CSRF cookie, paired with auth_token. Same place: copy the "ct0" cookie value.',
    url: 'https://x.com' },
  { label: 'Facebook · Page ID', key: 'FB_PAGE_ID',
    hint: 'The numeric ID of a Facebook Page you manage. Open your Page → About → "Page transparency" → Page ID.',
    url: 'https://www.facebook.com' },
  { label: 'Facebook · Page token', key: 'FB_PAGE_TOKEN',
    hint: 'Long-lived Page access token. Get it from the Meta Graph API Explorer with the pages_manage_posts permission.',
    url: 'https://developers.facebook.com/tools/explorer/' },
  { label: 'LinkedIn', key: 'LINKEDIN_COOKIE',
    hint: 'LinkedIn session cookie for posting. In a logged-in tab: DevTools (F12) → Application → Cookies → copy "li_at".',
    url: 'https://www.linkedin.com' },
  { label: 'Telegram bot', key: 'TELEGRAM_BOT_TOKEN',
    hint: 'Token for your own Telegram bot. Chat with @BotFather → send /newbot → copy the token it gives you.',
    url: 'https://t.me/BotFather' },
  { label: 'FLOWORK_OS group id', key: 'FWOS_CHAT_ID',
    hint: 'The chat/group ID where promo posts are sent. Add @userinfobot to the group to read its ID (starts with -100…).',
    url: 'https://t.me/userinfobot' },
  { label: 'FLOWORK_OS bot', key: 'FWOS_BOT_TOKEN',
    hint: 'Bot token used for FLOWORK_OS group posts. Create it via @BotFather the same way as the Telegram bot.',
    url: 'https://t.me/BotFather' },
];

const SEGMENTS = [
  { key: 'account', label: () => t('menu.tab.settings.seg_account'), render: renderAccount },
  { key: 'keys', label: () => t('menu.tab.settings.seg_keys'), render: renderKeys },
  { key: 'switches', label: () => t('menu.tab.settings.seg_switches'), render: renderSwitches },
  { key: 'router', label: () => t('menu.tab.settings.seg_router'), render: renderRouterDefault },
  { key: 'notify', label: () => t('menu.tab.settings.seg_notify'), render: renderNotify },
  { key: 'guardian', label: () => t('menu.tab.settings.seg_guardian'), render: renderGuardian },
  { key: 'evolve', label: () => t('menu.tab.settings.seg_evolve'), render: renderEvolvePush },
  { key: 'compact', label: () => t('menu.tab.settings.seg_compact'), render: renderCompact },
];
// 2026-06-27: YouTube DICABUT sampai akar (handler + route + watcher + GUI) — owner: basi.
// Finance/wallet (crypto) dicopot dari UI; backend keujet di kernel FROZEN (loket ABI + tools +
// evolve-pillars) → dibiarin dormant (nyabut = langgar invariant "ABI cuma tumbuh"). Kelak: integration-registry.

export async function render(mainEl) {
  ensureGlass();
  loadStyle('settings', CSS);
  const tk = (k) => t('menu.tab.settings.' + k);
  mainEl.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">⚙️</span>
        <div>
          <h2 class="fw-title">${esc(tk('title'))}</h2>
          <div class="fw-sub">${esc(tk('desc'))}</div>
        </div>
      </div>
      <div class="set-bar" id="setBar">
        ${SEGMENTS.map(s => `<button class="set-btn" data-key="${escAttr(s.key)}">${esc(s.label())}</button>`).join('')}
      </div>
      <div class="set-panel" id="setPanel"></div>
    </div>
  `;
  const panel = mainEl.querySelector('#setPanel');
  const btns = mainEl.querySelectorAll('.set-btn');
  async function open(key) {
    const seg = SEGMENTS.find(s => s.key === key) || SEGMENTS[0];
    btns.forEach(b => b.classList.toggle('active', b.getAttribute('data-key') === seg.key));
    panel.innerHTML = `<div class="fw-empty">${esc(t('common.loading_label').replace('{label}', seg.label()))}</div>`;
    try {
      await seg.render(panel);
    } catch (e) {
      panel.innerHTML = `<div class="set-msg err">${esc(String(e.message || e))}</div>`;
    }
  }
  btns.forEach(b => b.addEventListener('click', () => open(b.getAttribute('data-key'))));
  open('account');
}

// ── Akun & Keamanan ──────────────────────────────────────────────────────
async function renderAccount(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('account_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('account_sub'))}</div>
      <div class="set-field"><input type="password" id="pwOld" placeholder="${escAttr(tk('change_pw_old_ph'))}" autocomplete="current-password"></div>
      <div class="set-field"><input type="password" id="pwNew" placeholder="${escAttr(tk('change_pw_new_ph'))}" autocomplete="new-password"></div>
      <button class="fw-btn" id="pwBtn">${esc(tk('change_pw_btn'))}</button>
      <div class="set-msg" id="pwMsg"></div>
    </div>
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('logout_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('logout_sub'))}</div>
      <button class="fw-btn danger" id="logoutBtn">${esc(tk('logout_btn'))}</button>
    </div>
  `;
  const msg = panel.querySelector('#pwMsg');
  panel.querySelector('#pwBtn').addEventListener('click', async () => {
    const oldPw = panel.querySelector('#pwOld').value;
    const newPw = panel.querySelector('#pwNew').value;
    msg.className = 'set-msg';
    msg.textContent = '';
    try {
      await fetchJSON('/api/auth/change-password', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ old: oldPw, new: newPw }),
      });
      msg.className = 'set-msg ok';
      msg.textContent = tk('change_pw_ok');
      panel.querySelector('#pwOld').value = '';
      panel.querySelector('#pwNew').value = '';
    } catch (e) {
      msg.className = 'set-msg err';
      msg.textContent = tk('change_pw_err') + ' ' + cleanErr(e);
    }
  });
  panel.querySelector('#logoutBtn').addEventListener('click', async () => {
    await fetch('/api/auth/logout', { method: 'POST' });
    window.location.href = '/login';
  });
}

// ── Token Crypto / API Keys ────────────────────────────────────────────────
async function renderKeys(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('keys_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('keys_sub'))}</div>
      <div class="set-inline" style="margin-bottom:10px">
        <input type="text" id="kKey" placeholder="${escAttr(tk('keys_key_ph'))}" list="kKnown" style="flex:1;min-width:160px">
        <input type="text" id="kVal" placeholder="${escAttr(tk('keys_val_ph'))}" style="flex:1;min-width:160px">
        <button class="fw-btn" id="kAdd">${esc(t('common.btn.save'))}</button>
      </div>
      <div class="set-hint">${esc(tk('keys_preset_hint'))}</div>
      <div class="set-presets" id="kPresets">
        ${KEY_PRESETS.map(p => `<span class="set-preset" data-key="${escAttr(p.key)}" title="${escAttr(p.key)}">${esc(p.label)}</span>`).join('')}
      </div>
      <datalist id="kKnown">
        ${KEY_PRESETS.map(p => `<option value="${escAttr(p.key)}">`).join('')}
      </datalist>
      <div class="set-msg" id="kMsg"></div>
      <ul class="set-list" id="kList" style="list-style:none;padding:0;margin:10px 0 0"></ul>
    </div>
  `;
  const list = panel.querySelector('#kList');
  const msg = panel.querySelector('#kMsg');
  // Clicking a preset chip pre-fills the exact env var name → owner just pastes
  // the value. Chips for keys already saved turn green so it's clear what's set.
  panel.querySelectorAll('.set-preset').forEach(c => c.addEventListener('click', () => {
    panel.querySelector('#kKey').value = c.getAttribute('data-key');
    const v = panel.querySelector('#kVal'); v.value = ''; v.focus();
    msg.className = 'set-msg'; msg.textContent = '';
  }));
  function markPresets(items) {
    const set = new Set((items || []).map(it => it.key));
    panel.querySelectorAll('.set-preset').forEach(c =>
      c.classList.toggle('set-on', set.has(c.getAttribute('data-key'))));
  }
  async function reload() {
    const d = await fetchJSON('/api/settings/keys');
    const items = d.items || [];
    markPresets(items);
    const savedMap = new Map(items.map(it => [it.key, it.masked || '••••']));
    const extra = items.filter(it => !KEY_PRESETS.some(p => p.key === it.key));
    // Guided list: EVERY known provider shown, set or not. Unset → "change me"
    // so a non-developer knows exactly what to fill, with a how-to link.
    const rowFor = (p) => {
      const isSet = savedMap.has(p.key);
      const status = isSet
        ? `<span class="set-on-tag">${esc(savedMap.get(p.key))}</span>`
        : `<span class="set-changeme">change me</span>`;
      return `
        <li class="set-keyrow">
          <div class="set-keyhead">
            <span><b>${esc(p.label || p.key)}</b> <span class="set-keyenv">${esc(p.key)}</span></span>
            ${status}
          </div>
          ${p.hint ? `<div class="set-keyhint">${esc(p.hint)}${p.url ? ` <a href="${escAttr(p.url)}" target="_blank" rel="noopener">How to get →</a>` : ''}</div>` : ''}
          <div class="set-keyact">
            <input type="text" class="set-keyin" data-key="${escAttr(p.key)}" placeholder="${escAttr(isSet ? 'leave blank to keep' : 'paste token / change me')}">
            <button class="fw-btn set-keysave" data-key="${escAttr(p.key)}">${esc(t('common.btn.save'))}</button>
            ${isSet ? `<button class="fw-btn danger rm" data-key="${escAttr(p.key)}">${esc(t('common.btn.delete'))}</button>` : ''}
          </div>
        </li>`;
    };
    list.innerHTML = KEY_PRESETS.map(rowFor).join('')
      + extra.map(it => rowFor({ key: it.key, label: it.key })).join('');
    list.querySelectorAll('.set-keysave').forEach(b => b.addEventListener('click', async () => {
      const key = b.getAttribute('data-key');
      const input = b.closest('li').querySelector('.set-keyin');
      const value = input ? input.value : '';
      if (!value.trim()) { msg.className = 'set-msg'; msg.textContent = 'Field is empty — nothing to save.'; return; }
      try {
        await fetchJSON('/api/settings/keys', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ key, value }),
        });
        msg.className = 'set-msg ok'; msg.textContent = tk('keys_saved');
        await reload();
      } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
    }));
    list.querySelectorAll('.rm').forEach(b => b.addEventListener('click', async () => {
      const key = b.getAttribute('data-key');
      if (!confirm(tk('keys_del_confirm') + ' "' + key + '"?')) return;
      try {
        await fetchJSON('/api/settings/keys?key=' + encodeURIComponent(key), { method: 'DELETE' });
        await reload();
      } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
    }));
  }
  panel.querySelector('#kAdd').addEventListener('click', async () => {
    const key = panel.querySelector('#kKey').value.trim();
    const value = panel.querySelector('#kVal').value;
    msg.className = 'set-msg'; msg.textContent = '';
    try {
      await fetchJSON('/api/settings/keys', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ key, value }),
      });
      msg.className = 'set-msg ok'; msg.textContent = tk('keys_saved');
      panel.querySelector('#kVal').value = '';
      await reload();
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
  await reload();
}

// ── Switch Fitur (plug-and-play; ganti flowork.local.env) ───────────────────
// Switch perilaku FLOWORK_* yg DULU kebawa di flowork.local.env (invisible buat user fresh).
// Sekarang dari sini → ditulis ~/.flowork/flowork_settings.json (lintas-proses, dibaca router
// :2402 + host :1987). Presedensi: GUI menang > ENV > default. Sumber tiap switch di-badge.
async function renderSwitches(panel) {
  panel.innerHTML = `<div class="fw-card"><div class="fw-sec">🎛️ Switch Fitur</div>
    <div class="fw-desc" style="margin-top:0">Toggle Flowork behavior — no more manual <code>flowork.local.env</code> edits.
    Applies to router &amp; host (live ≤3s). Badge: <b>gui</b>=set here · <b>env</b>=from ENV · <b>default</b>=built-in.</div>
    <div id="swList" class="fw-desc">Loading…</div></div>`;
  const list = panel.querySelector('#swList');
  const badge = (src) => {
    const c = src === 'gui' ? '#34d399' : src === 'env' ? '#fbbf24' : 'var(--text-muted)';
    return `<span class="set-srcbadge" style="background:color-mix(in srgb,${c} 16%, transparent);color:${c};border:1px solid color-mix(in srgb,${c} 45%, transparent)">${src}</span>`;
  };
  async function reload() {
    let d;
    try { d = await fetchJSON('/api/settings/switches'); }
    catch (e) { list.innerHTML = `<span class="set-msg err">${esc(cleanErr(e))}</span>`; return; }
    const groups = {};
    (d.switches || []).forEach(s => { (groups[s.category] = groups[s.category] || []).push(s); });
    list.innerHTML = Object.keys(groups).map(cat => `
      <div class="set-swgroup"><div class="set-swcat">${esc(cat)}</div>
      ${groups[cat].map(s => {
        const boolOn = /^(1|on|true|yes)$/i.test(s.value);
        const ctrl = s.type === 'bool'
          ? `<input type="checkbox" class="sw-in" data-key="${escAttr(s.key)}" data-type="bool" data-orig="${boolOn ? '1' : '0'}" ${boolOn ? 'checked' : ''}>`
          : `<input type="text" class="sw-in" data-key="${escAttr(s.key)}" data-type="${escAttr(s.type)}" data-orig="${escAttr(s.value)}" value="${escAttr(s.value)}">`;
        return `<div class="set-swrow">
          <div class="set-swmeta">
            <div class="set-swlabel">${esc(s.label)} ${badge(s.source)}</div>
            <div class="set-swdesc">${esc(s.desc)} <code>${esc(s.key)}</code></div>
          </div>${ctrl}</div>`;
      }).join('')}</div>`).join('')
      + `<div class="set-saverow">
          <button class="fw-btn" id="swSave">Simpan</button>
          <span id="swMsg" class="set-msg"></span>
          <span class="fw-grow"></span>
          <code class="set-path">${esc(d.path || '')}</code>
        </div>`;
    panel.querySelector('#swSave').addEventListener('click', async () => {
      const msg = panel.querySelector('#swMsg');
      const values = {};
      list.querySelectorAll('.sw-in').forEach(el => {
        const cur = el.dataset.type === 'bool' ? (el.checked ? '1' : '0') : el.value.trim();
        if (cur !== (el.dataset.orig || '')) values[el.dataset.key] = cur; // cuma yg BERUBAH → gui-pin
      });
      if (!Object.keys(values).length) { msg.className = 'set-msg'; msg.textContent = 'No changes'; return; }
      msg.className = 'set-msg'; msg.textContent = 'Menyimpan…';
      try {
        await fetchJSON('/api/settings/switches', {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ values }),
        });
        msg.className = 'set-msg ok'; msg.textContent = '✓ saved (live ≤3s to router)';
        await reload();
      } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
    });
  }
  await reload();
}

// ── Default Router & Model ──────────────────────────────────────────────────
// Global fallback for agents that don't pin their own model/router. Per-agent
// config still wins — these values only fill the blank.
async function renderRouterDefault(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('router_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('router_sub'))}</div>
      <div class="set-field"><input type="text" id="rdModel" placeholder="${escAttr(tk('router_model_ph'))}"></div>
      <div class="set-hint">${esc(tk('router_model_hint'))}</div>
      <div class="set-field"><input type="text" id="rdUrl" placeholder="${escAttr(tk('router_url_ph'))}"></div>
      <div class="set-hint">${esc(tk('router_url_hint'))}</div>
      <button class="fw-btn" id="rdSave">${esc(t('common.btn.save'))}</button>
      <div class="set-msg" id="rdMsg"></div>
    </div>
  `;
  const msg = panel.querySelector('#rdMsg');
  try {
    const d = await fetchJSON('/api/settings/router-default');
    panel.querySelector('#rdModel').value = d.model || '';
    panel.querySelector('#rdUrl').value = d.router_url || '';
  } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  panel.querySelector('#rdSave').addEventListener('click', async () => {
    const model = panel.querySelector('#rdModel').value.trim();
    const router_url = panel.querySelector('#rdUrl').value.trim();
    msg.className = 'set-msg'; msg.textContent = '';
    try {
      await fetchJSON('/api/settings/router-default', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model, router_url }),
      });
      msg.className = 'set-msg ok'; msg.textContent = tk('router_saved');
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
}

// LOCKED (soft, owner-approved 2026-06-20): GUI auto-push token. Token write-only, ga ke-commit.
// renderEvolvePush — Settings GLOBAL buat AUTO-PUSH evolusi (owner 2026-06-20: "api github
// untuk push saat selesai evolute taruh di seting global"). Token GitHub disimpan lokal di
// flowork.db (~/.flowork, GA ke-commit), write-only (server ga pernah balikin token — cuma
// has_token bool). Auto-push cuma jalan kalau: enabled + token ada + mode=auto + lolos SEMUA
// gate (karma+model+re-probe). Manual core-apply tetep STAGE (ga ke-push).
async function renderEvolvePush(panel) {
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">🧬 Evolution — Auto-Push to GitHub</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">When the evolving organism auto-commits the core (AUTO mode + all gates passed), the result is
        pushed to GitHub so Flowork stays immortal even without the owner. The token is stored locally
        (never committed) and never shown again. Manual core-apply stays STAGE (not pushed).</div>
      <label class="set-toggle">
        <input type="checkbox" id="epEnabled"> <span>Enable auto-push</span>
      </label>
      <div class="set-field">
        <input type="password" id="epToken" placeholder="GitHub token (ghp_… / fine-grained)" autocomplete="new-password">
      </div>
      <div class="set-hint" id="epTokenHint"></div>
      <div class="set-field">
        <input type="text" id="epRemote" placeholder="remote (default: origin)">
      </div>
      <div class="set-field">
        <input type="text" id="epBranch" placeholder="branch (blank = repo's active branch)">
      </div>
      <div class="set-hint">⚠️ Push uses HTTPS http.extraHeader (token never written to git config). A fine-grained
        token with minimal scope (contents:write) on just the Flowork repo is recommended — not a full-access token.</div>
      <button class="fw-btn" id="epSave">${esc(t('common.btn.save'))}</button>
      <div class="set-msg" id="epMsg"></div>
    </div>
  `;
  const msg = panel.querySelector('#epMsg');
  const tokenHint = panel.querySelector('#epTokenHint');
  try {
    const d = await fetchJSON('/api/evolve/push-config');
    panel.querySelector('#epEnabled').checked = !!d.enabled;
    panel.querySelector('#epRemote').value = d.remote || '';
    panel.querySelector('#epBranch').value = d.branch || '';
    tokenHint.textContent = d.has_token
      ? '✅ Token saved. Leave a field blank = keep the current token; fill it = replace.'
      : "⚠️ No token yet — auto-push won't run until you set it.";
  } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  panel.querySelector('#epSave').addEventListener('click', async () => {
    const enabled = panel.querySelector('#epEnabled').checked;
    const token = panel.querySelector('#epToken').value;          // JANGAN trim: biar owner bisa paste apa adanya
    const remote = panel.querySelector('#epRemote').value.trim();
    const branch = panel.querySelector('#epBranch').value.trim();
    msg.className = 'set-msg'; msg.textContent = '';
    // Body: token cuma dikirim kalau owner ngetik (field kosong = jangan sentuh token lama).
    const body = { enabled, remote, branch };
    if (token.length > 0) body.token = token;
    try {
      await fetchJSON('/api/evolve/push-config', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      panel.querySelector('#epToken').value = '';
      msg.className = 'set-msg ok'; msg.textContent = 'Saved ✓';
      const d = await fetchJSON('/api/evolve/push-config');
      tokenHint.textContent = d.has_token
        ? '✅ Token saved. Leave a field blank = keep the current token; fill it = replace.'
        : "⚠️ No token yet — auto-push won't run until you set it.";
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
}

// ── Notifikasi (Telegram owner-level, TERPISAH dari agent) ──────────────────
async function renderNotify(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('notify_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('notify_sub'))}</div>
      <div class="set-field"><input type="password" id="ntToken" placeholder="${escAttr(tk('notify_token_ph'))}" autocomplete="off"></div>
      <div class="set-field"><input type="text" id="ntChat" placeholder="${escAttr(tk('notify_chat_ph'))}"></div>
      <div class="set-hint">${esc(tk('notify_chat_hint'))}</div>
      <div class="set-inline">
        <button class="fw-btn" id="ntSave">${esc(tk('notify_save'))}</button>
        <button class="fw-btn" id="ntTest">${esc(tk('notify_test'))}</button>
      </div>
      <div class="set-msg" id="ntMsg"></div>
    </div>
  `;
  const msg = panel.querySelector('#ntMsg');
  const tokenEl = panel.querySelector('#ntToken');
  const chatEl = panel.querySelector('#ntChat');
  // Prefill: chat id + placeholder masked token kalau udah di-set.
  try {
    const d = await fetchJSON('/api/settings/notify');
    if (d.chat_id) chatEl.value = d.chat_id;
    if (d.set && d.bot_token_masked) tokenEl.setAttribute('placeholder', d.bot_token_masked + ' (saved — leave blank to keep)');
  } catch (e) { /* ignore */ }

  async function save(test) {
    msg.className = 'set-msg'; msg.textContent = '';
    const payload = { chat_id: chatEl.value.trim(), test };
    const tok = tokenEl.value.trim();
    if (tok) payload.bot_token = tok;
    try {
      const r = await fetchJSON('/api/settings/notify', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      if (test) {
        if (r.test === 'sent') { msg.className = 'set-msg ok'; msg.textContent = tk('notify_test_sent'); }
        else { msg.className = 'set-msg err'; msg.textContent = tk('notify_test_fail') + ' ' + (r.error || r.test || ''); }
      } else {
        msg.className = 'set-msg ok'; msg.textContent = tk('notify_saved');
      }
      tokenEl.value = '';
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  }
  panel.querySelector('#ntSave').addEventListener('click', () => save(false));
  panel.querySelector('#ntTest').addEventListener('click', () => save(true));
}

function cleanErr(e) {
  const m = String(e && e.message || e);
  const j = m.indexOf('{');
  if (j >= 0) { try { return JSON.parse(m.slice(j)).error || m; } catch {} }
  return m;
}

// LOCKED (soft, owner-approved 2026-06-20): GUI ambang auto-compact.
// renderCompact — Settings AUTO-COMPACT (owner 2026-06-20): ambang konteks → agent auto
// digest pengalaman ke brain + trim (anti-halu konteks panjang). Pengalaman ga ilang (pindah
// ke brain), cuma raw interaksi lama di-trim. Trigger by UKURAN, cek tiap 15 menit.
async function renderCompact(panel) {
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">🧠 Auto-Compact Context (anti-hallucination)</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">When an agent's interactions pile up, the AI can hallucinate once the context gets too long. Every 15 minutes,
        agents past the threshold auto: <b>digest experience into the brain</b> (like dreaming) →
        <b>trim</b> old interactions (keep the most recent). Experience is NOT lost — it moves to the brain,
        still recallable. Safe: only what's already in the brain gets trimmed.</div>
      <label class="set-toggle">
        <input type="checkbox" id="cmpEnabled"> <span>Enable auto-compact</span>
      </label>
      <div class="set-inline" style="margin-bottom:6px">
        <label>Threshold (interaction count):</label>
        <input type="number" id="cmpMax" min="20" step="20">
      </div>
      <div class="set-hint">Agents whose non-deleted interactions exceed this number get compacted. Default 400.</div>
      <div class="set-inline" style="margin-bottom:6px">
        <label>Keep most recent:</label>
        <input type="number" id="cmpKeep" min="0" step="10">
      </div>
      <div class="set-hint">How many of the most RECENT interactions stay intact in context (not trimmed). Default 60.</div>
      <div class="set-inline" style="margin-bottom:6px">
        <label>Digest model (optional):</label>
        <input type="text" id="cmpModel" placeholder="blank = use the LOCAL model (flowork-brain)" style="flex:1;min-width:240px" autocomplete="off">
      </div>
      <div class="set-hint">Model used to digest experience into the brain on compact (auto, Compact All, or per-agent). <b>Leave blank = use the LOCAL model (flowork-brain)</b> — free &amp; works without a subscription. Enter a capable cloud model for more reliable digests on large context (while a subscription lasts).</div>
      <button class="fw-btn" id="cmpSave">${esc(t('common.btn.save'))}</button>
      <div class="set-msg" id="cmpMsg"></div>
    </div>
  `;
  const msg = panel.querySelector('#cmpMsg');
  try {
    const d = await fetchJSON('/api/compact/config');
    panel.querySelector('#cmpEnabled').checked = d.enabled !== false;
    panel.querySelector('#cmpMax').value = d.max_interactions || 400;
    panel.querySelector('#cmpKeep').value = (d.keep_recent ?? 60);
    panel.querySelector('#cmpModel').value = (d.model || '');
  } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  panel.querySelector('#cmpSave').addEventListener('click', async () => {
    const enabled = panel.querySelector('#cmpEnabled').checked;
    const max_interactions = parseInt(panel.querySelector('#cmpMax').value, 10) || 400;
    const keep_recent = parseInt(panel.querySelector('#cmpKeep').value, 10);
    const model = panel.querySelector('#cmpModel').value.trim();
    msg.className = 'set-msg'; msg.textContent = '';
    if (keep_recent >= max_interactions) {
      msg.className = 'set-msg err'; msg.textContent = 'keep_recent must be < threshold (otherwise nothing gets trimmed)'; return;
    }
    try {
      await fetchJSON('/api/compact/config', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled, max_interactions, keep_recent: isNaN(keep_recent) ? 60 : keep_recent, model }),
      });
      msg.className = 'set-msg ok'; msg.textContent = 'Saved ✓';
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
}

// ── Self-finance wallet (R8 fase-1) ──────────────────────────────────────────
// Kredensial wallet EVM organisme: address (publik), private key (secret, masked, ga pernah
// ditampilkan), chain/RPC/currency, hard-limit (pagar belanja). Transaksi nyata = fase-2.
async function renderFinance(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">${esc(tk('fin_h'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('fin_sub'))}</div>
      <div class="set-field"><input type="text" id="finAddr" placeholder="${escAttr(tk('fin_addr_ph'))}" autocomplete="off"></div>
      <div class="set-field"><input type="password" id="finPk" placeholder="${escAttr(tk('fin_pk_ph'))}" autocomplete="off"></div>
      <div class="set-hint" style="color:#fbbf24">${esc(tk('fin_pk_hint'))}</div>
      <div class="set-field"><input type="text" id="finChain" placeholder="${escAttr(tk('fin_chain_ph'))}"></div>
      <div class="set-field"><input type="text" id="finRpc" placeholder="${escAttr(tk('fin_rpc_ph'))}"></div>
      <div class="set-inline" style="margin-bottom:12px">
        <input type="text" id="finCur" placeholder="${escAttr(tk('fin_cur_ph'))}" style="flex:1">
        <input type="number" id="finLimit" min="0" step="any" placeholder="${escAttr(tk('fin_limit_ph'))}" style="flex:1">
      </div>
      <label class="set-toggle"><input type="checkbox" id="finEnabled"> ${esc(tk('fin_enabled'))}</label>
      <div class="set-hint" id="finBalance"></div>
      <div class="set-inline">
        <button class="fw-btn" id="finSave">${esc(tk('fin_save'))}</button>
        <button class="fw-btn danger" id="finDelete">${esc(tk('fin_delete'))}</button>
      </div>
      <div class="set-msg" id="finMsg"></div>
    </div>
  `;
  const msg = panel.querySelector('#finMsg');
  const els = {
    addr: panel.querySelector('#finAddr'), pk: panel.querySelector('#finPk'),
    chain: panel.querySelector('#finChain'), rpc: panel.querySelector('#finRpc'),
    cur: panel.querySelector('#finCur'), limit: panel.querySelector('#finLimit'),
    enabled: panel.querySelector('#finEnabled'),
  };
  try {
    const d = await fetchJSON('/api/finance/wallet');
    if (d.address) els.addr.value = d.address;
    if (d.chain) els.chain.value = d.chain;
    if (d.rpc) els.rpc.value = d.rpc;
    if (d.currency) els.cur.value = d.currency;
    if (d.hard_limit) els.limit.value = d.hard_limit;
    els.enabled.checked = !!d.enabled;
    if (d.has_privkey && d.privkey_masked) els.pk.setAttribute('placeholder', d.privkey_masked + ' (saved — leave blank to keep)');
    panel.querySelector('#finBalance').textContent = d.balance || '';
  } catch (e) { /* ignore */ }

  async function save() {
    msg.className = 'set-msg'; msg.textContent = '';
    const payload = {
      address: els.addr.value.trim(), chain: els.chain.value.trim(), rpc: els.rpc.value.trim(),
      currency: els.cur.value.trim(), hard_limit: els.limit.value.trim(), enabled: els.enabled.checked,
    };
    const pk = els.pk.value.trim();
    if (pk) payload.privkey = pk;
    try {
      await fetchJSON('/api/finance/wallet', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      msg.className = 'set-msg ok'; msg.textContent = tk('fin_saved');
      els.pk.value = '';
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  }
  panel.querySelector('#finSave').addEventListener('click', save);
  panel.querySelector('#finDelete').addEventListener('click', async () => {
    if (!confirm(tk('fin_delete_confirm'))) return;
    msg.className = 'set-msg'; msg.textContent = '';
    try {
      await fetchJSON('/api/finance/wallet', { method: 'DELETE' });
      msg.className = 'set-msg ok'; msg.textContent = tk('fin_deleted');
      els.pk.value = ''; els.pk.setAttribute('placeholder', tk('fin_pk_ph')); els.enabled.checked = false;
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
}


// ── Guardian (integritas kernel) ─────────────────────────────────────────────
async function renderGuardian(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  let s;
  try { s = await fetchJSON('/api/guardian/status'); }
  catch (e) { panel.innerHTML = `<div class="set-msg err">${esc(cleanErr(e))}</div>`; return; }
  const badge = s.safe_mode
    ? `<span style="color:#f87171;font-weight:700">⛔ ${esc(tk('grd_safemode'))}</span>`
    : s.armed
      ? `<span style="color:#34d399;font-weight:700">🛡️ ${esc(tk('grd_armed'))}</span>`
      : `<span style="color:#fbbf24">○ ${esc(tk('grd_disarmed'))}</span>`;
  panel.innerHTML = `
    <div class="fw-card">
      <div class="fw-sec">🛡️ ${esc(tk('grd_title'))}</div>
      <div class="fw-desc" style="margin-top:0;margin-bottom:14px">${esc(tk('grd_desc'))}</div>
      <div class="fw-row" style="margin-bottom:8px">${badge} &nbsp;·&nbsp; ${esc(tk('grd_protected'))}: <b>${s.protected || 0}</b>${s.sealed_at ? ` &nbsp;·&nbsp; ${esc(tk('grd_sealed'))}: ${esc(s.sealed_at)}` : ''}</div>
      ${s.armed ? `<div class="fw-row" style="font-size:.8rem;margin-bottom:8px">${s.sealed
        ? `🔒 <span style="color:#34d399">${esc(tk('grd_seal_os'))}</span> <span style="color:var(--text-muted)">(${esc(s.seal_method || '')})</span>`
        : `🔓 <span style="color:#fbbf24">${esc(tk('grd_seal_detect'))}</span>`}</div>` : ''}
      <div class="set-inline" style="margin-top:12px">
        ${s.armed
          ? `<input type="password" id="grdPw" placeholder="${escAttr(tk('grd_pw_ph'))}" autocomplete="current-password" style="flex:1;min-width:160px">
             <button class="fw-btn" id="grdDisarm">${esc(tk('grd_disarm_btn'))}</button>`
          : `<button class="fw-btn" id="grdArm">${esc(tk('grd_arm_btn'))}</button>`}
      </div>
      <div class="set-msg" id="grdMsg"></div>
    </div>`;
  const msg = panel.querySelector('#grdMsg');
  const arm = panel.querySelector('#grdArm');
  if (arm) arm.addEventListener('click', async () => {
    if (!confirm(tk('grd_arm_confirm'))) return;
    msg.className = 'set-msg';
    try { await fetchJSON('/api/guardian/arm', { method: 'POST' }); renderGuardian(panel); }
    catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
  const dis = panel.querySelector('#grdDisarm');
  if (dis) dis.addEventListener('click', async () => {
    msg.className = 'set-msg';
    try {
      await fetchJSON('/api/guardian/disarm', { method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: panel.querySelector('#grdPw').value }) });
      renderGuardian(panel);
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
}
