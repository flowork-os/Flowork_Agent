// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Tab Settings (Akun/Token-Keys/Wallet Personal/Wallet AI). Audit
//   pass — esc/escAttr semua field, fetchJSON util, API key masked, label
//   via i18n. E2E verified lewat instance isolated.
//
// settings.js — halaman Settings GLOBAL (owner-level).
//
// 4 section (sub-tab internal): Akun & Keamanan, Token Crypto / API Keys,
// Wallet Personal (owner), Wallet AI (per-agent read-only).
//
// Data owner-level disimpan di flowork.db global (lewat /api/settings/*).
// AI agent TETAP terisolasi — Wallet AI cuma nampilin read-only.
//
// Semua label lewat dictionary i18n (t(...)) — no hardcode UI text.

import { t } from '/js/i18n.js';
import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';

// Chain data (proper-noun blockchain, bukan UI chrome) — selaras wallet.Supported().
const CHAINS = [
  { id: 1, name: 'Ethereum' },
  { id: 137, name: 'Polygon' },
  { id: 42161, name: 'Arbitrum' },
  { id: 10, name: 'Optimism' },
  { id: 8453, name: 'Base' },
  { id: 56, name: 'BNB Chain' },
];

const CSS = `
.set-bar { display:flex; gap:4px; margin-bottom:18px; padding:5px; flex-wrap:wrap;
  background:rgba(15,17,26,0.55); border:1px solid var(--glass-border); border-radius:12px; width:fit-content; max-width:100%; }
.set-btn { padding:8px 14px; font-size:0.82rem; font-weight:500; border-radius:8px;
  background:transparent; border:1px solid transparent; color:var(--text-muted); cursor:pointer;
  transition:background .15s,color .15s,border-color .15s; }
.set-btn:hover { background:rgba(139,92,246,0.08); color:#cbd5e1; }
.set-btn.active { background:linear-gradient(135deg,rgba(139,92,246,0.28),rgba(124,58,237,0.12));
  color:#c4b5fd; border-color:rgba(139,92,246,0.45); }
.set-panel { max-width:760px; }
.set-card { background:rgba(15,17,26,0.6); border:1px solid var(--glass-border); border-radius:14px;
  padding:18px 20px; margin-bottom:16px; }
.set-card h3 { font-size:0.95rem; margin:0 0 4px; color:#e2e8f0; }
.set-card .sub { font-size:0.8rem; color:var(--text-muted); margin-bottom:14px; line-height:1.5; }
.set-row { display:flex; gap:8px; flex-wrap:wrap; margin-bottom:10px; align-items:center; }
.set-row input, .set-row select { padding:10px 12px; border-radius:8px; background:rgba(30,41,59,0.6);
  border:1px solid rgba(148,163,184,0.2); color:#e2e8f0; font-size:0.88rem; flex:1; min-width:140px; }
.set-row input:focus, .set-row select:focus { outline:none; border-color:#a78bfa; }
.set-btn-primary { padding:10px 16px; background:linear-gradient(135deg,#a78bfa,#7c3aed); color:#fff;
  border:none; border-radius:8px; font-weight:600; font-size:0.85rem; cursor:pointer; }
.set-btn-primary:disabled { opacity:.5; cursor:not-allowed; }
.set-list { list-style:none; padding:0; margin:8px 0 0; }
.set-list li { display:flex; justify-content:space-between; align-items:center; gap:8px;
  padding:9px 12px; border:1px solid var(--glass-border); border-radius:8px; margin-bottom:6px;
  font-size:0.85rem; background:rgba(30,41,59,0.35); }
.set-list .mono { font-family:monospace; font-size:0.8rem; color:#cbd5e1; word-break:break-all; }
.set-list .rm { background:none; border:none; color:#fca5a5; cursor:pointer; font-size:0.85rem; }
.set-msg { margin-top:10px; font-size:0.82rem; min-height:1em; }
.set-msg.ok { color:#86efac; } .set-msg.err { color:#fca5a5; }
.set-empty { color:var(--text-muted); font-size:0.82rem; padding:8px 0; }
.set-tag { font-size:0.7rem; color:#64748b; }
.set-total { font-size:1.4rem; font-weight:700; color:#86efac; margin:6px 0; }
`;

const SEGMENTS = [
  { key: 'account', label: () => t('menu.tab.settings.seg_account'), render: renderAccount },
  { key: 'keys', label: () => t('menu.tab.settings.seg_keys'), render: renderKeys },
  { key: 'wallet', label: () => t('menu.tab.settings.seg_wallet'), render: renderWalletPersonal },
  { key: 'aiwallet', label: () => t('menu.tab.settings.seg_aiwallet'), render: renderAIWallet },
  { key: 'notify', label: () => t('menu.tab.settings.seg_notify'), render: renderNotify },
];

export async function render(mainEl) {
  loadStyle('settings', CSS);
  const tk = (k) => t('menu.tab.settings.' + k);
  mainEl.innerHTML = `
    <div class="sgt-header">
      <h2>⚙️ ${esc(tk('title'))}</h2>
      <div class="sub" style="font-size:0.82rem;color:var(--text-muted);margin-top:4px;">${esc(tk('desc'))}</div>
    </div>
    <div class="set-bar" id="setBar">
      ${SEGMENTS.map(s => `<button class="set-btn" data-key="${escAttr(s.key)}">${esc(s.label())}</button>`).join('')}
    </div>
    <div class="set-panel" id="setPanel"></div>
  `;
  const panel = mainEl.querySelector('#setPanel');
  const btns = mainEl.querySelectorAll('.set-btn');
  async function open(key) {
    const seg = SEGMENTS.find(s => s.key === key) || SEGMENTS[0];
    btns.forEach(b => b.classList.toggle('active', b.getAttribute('data-key') === seg.key));
    panel.innerHTML = `<div class="set-empty">${esc(t('common.loading_label').replace('{label}', seg.label()))}</div>`;
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
    <div class="set-card">
      <h3>${esc(tk('account_h'))}</h3>
      <div class="sub">${esc(tk('account_sub'))}</div>
      <div class="set-row"><input type="password" id="pwOld" placeholder="${escAttr(tk('change_pw_old_ph'))}" autocomplete="current-password"></div>
      <div class="set-row"><input type="password" id="pwNew" placeholder="${escAttr(tk('change_pw_new_ph'))}" autocomplete="new-password"></div>
      <div class="set-row"><button class="set-btn-primary" id="pwBtn">${esc(tk('change_pw_btn'))}</button></div>
      <div class="set-msg" id="pwMsg"></div>
    </div>
    <div class="set-card">
      <h3>${esc(tk('logout_h'))}</h3>
      <div class="sub">${esc(tk('logout_sub'))}</div>
      <button class="set-btn-primary" id="logoutBtn" style="background:linear-gradient(135deg,#ef4444,#b91c1c);">${esc(tk('logout_btn'))}</button>
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
    <div class="set-card">
      <h3>${esc(tk('keys_h'))}</h3>
      <div class="sub">${esc(tk('keys_sub'))}</div>
      <div class="set-row">
        <input type="text" id="kKey" placeholder="${escAttr(tk('keys_key_ph'))}" list="kKnown">
        <input type="text" id="kVal" placeholder="${escAttr(tk('keys_val_ph'))}">
        <button class="set-btn-primary" id="kAdd">${esc(t('common.btn.save'))}</button>
      </div>
      <datalist id="kKnown">
        <option value="ETHERSCAN_API_KEY"><option value="COINGECKO_API_KEY">
      </datalist>
      <div class="set-msg" id="kMsg"></div>
      <ul class="set-list" id="kList"></ul>
    </div>
  `;
  const list = panel.querySelector('#kList');
  const msg = panel.querySelector('#kMsg');
  async function reload() {
    const d = await fetchJSON('/api/settings/keys');
    const items = d.items || [];
    if (!items.length) { list.innerHTML = `<div class="set-empty">${esc(tk('keys_empty'))}</div>`; return; }
    list.innerHTML = items.map(it => `
      <li><span class="mono">${esc(it.key)}</span><span class="set-tag mono">${esc(it.masked || '—')}</span></li>
    `).join('');
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

// ── Wallet Personal (owner) ────────────────────────────────────────────────
async function renderWalletPersonal(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="set-card">
      <h3>${esc(tk('wallet_h'))}</h3>
      <div class="sub">${esc(tk('wallet_sub'))}</div>
      <div class="set-row">
        <select id="wChain">${CHAINS.map(c => `<option value="${c.id}">${esc(c.name)}</option>`).join('')}</select>
        <input type="text" id="wAddr" placeholder="${escAttr(tk('wallet_addr_ph'))}">
        <input type="text" id="wLabel" placeholder="${escAttr(tk('wallet_label_ph'))}">
        <button class="set-btn-primary" id="wAdd">${esc(t('common.btn.save'))}</button>
      </div>
      <div class="set-msg" id="wMsg"></div>
      <ul class="set-list" id="wList"></ul>
    </div>
    <div class="set-card">
      <h3>${esc(tk('wallet_portfolio_h'))}</h3>
      <div class="sub">${esc(tk('wallet_portfolio_sub'))}</div>
      <button class="set-btn-primary" id="wRefresh">${esc(tk('wallet_portfolio_btn'))}</button>
      <div id="wPortfolio" style="margin-top:12px;"></div>
    </div>
  `;
  const list = panel.querySelector('#wList');
  const msg = panel.querySelector('#wMsg');
  const chainName = (id) => (CHAINS.find(c => c.id === id) || {}).name || ('chain ' + id);
  async function reload() {
    const d = await fetchJSON('/api/settings/wallet/addresses');
    const items = d.items || [];
    if (!items.length) { list.innerHTML = `<div class="set-empty">${esc(tk('wallet_empty'))}</div>`; return; }
    list.innerHTML = items.map(it => `
      <li>
        <span><span class="set-tag">${esc(chainName(it.chain_id))}</span> <span class="mono">${esc(it.address)}</span>${it.label ? ' · ' + esc(it.label) : ''}</span>
        <button class="rm" data-chain="${it.chain_id}" data-addr="${escAttr(it.address)}">${esc(t('common.btn.delete'))}</button>
      </li>
    `).join('');
    list.querySelectorAll('.rm').forEach(b => b.addEventListener('click', async () => {
      const cid = b.getAttribute('data-chain');
      const addr = b.getAttribute('data-addr');
      await fetchJSON(`/api/settings/wallet/addresses?chain_id=${encodeURIComponent(cid)}&address=${encodeURIComponent(addr)}`, { method: 'DELETE' });
      await reload();
    }));
  }
  panel.querySelector('#wAdd').addEventListener('click', async () => {
    const chain_id = parseInt(panel.querySelector('#wChain').value, 10);
    const address = panel.querySelector('#wAddr').value.trim();
    const label = panel.querySelector('#wLabel').value.trim();
    msg.className = 'set-msg'; msg.textContent = '';
    try {
      await fetchJSON('/api/settings/wallet/addresses', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ chain_id, address, label }),
      });
      panel.querySelector('#wAddr').value = ''; panel.querySelector('#wLabel').value = '';
      await reload();
    } catch (e) { msg.className = 'set-msg err'; msg.textContent = cleanErr(e); }
  });
  panel.querySelector('#wRefresh').addEventListener('click', async () => {
    const box = panel.querySelector('#wPortfolio');
    box.innerHTML = `<div class="set-empty">${esc(tk('wallet_loading'))}</div>`;
    try {
      const p = await fetchJSON('/api/settings/wallet/portfolio');
      const holdings = (p.holdings || []).filter(h => h.amount > 0);
      box.innerHTML = `
        <div class="set-total">$${esc((p.total_usd || 0).toFixed(2))}</div>
        <ul class="set-list">${holdings.map(h => `
          <li><span><span class="set-tag">${esc(h.chain_name || '')}</span> ${esc(h.symbol)}</span>
          <span class="mono">${esc(h.amount.toFixed(4))} · $${esc((h.usd_value || 0).toFixed(2))}</span></li>
        `).join('') || `<div class="set-empty">${esc(tk('wallet_empty'))}</div>`}</ul>
        ${p.partial_error ? `<div class="set-msg err">${esc(p.partial_error)}</div>` : ''}`;
    } catch (e) {
      box.innerHTML = `<div class="set-msg err">${esc(cleanErr(e))}<br><span class="set-tag">${esc(tk('wallet_need_key'))}</span></div>`;
    }
  });
  await reload();
}

// ── Wallet AI (per-agent, read-only) ───────────────────────────────────────
async function renderAIWallet(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  const chainName = (id) => (CHAINS.find(c => c.id === id) || {}).name || ('chain ' + id);
  panel.innerHTML = `
    <div class="set-card">
      <h3>${esc(tk('aiwallet_h'))}</h3>
      <div class="sub">${esc(tk('aiwallet_sub'))}</div>
      <div id="aiList"></div>
    </div>
  `;
  const box = panel.querySelector('#aiList');
  const d = await fetchJSON('/api/settings/ai-wallets');
  const items = d.items || [];
  if (!items.length) { box.innerHTML = `<div class="set-empty">${esc(tk('aiwallet_empty'))}</div>`; return; }
  box.innerHTML = items.map(a => {
    const addrs = a.addresses || [];
    const inner = addrs.length
      ? `<ul class="set-list">${addrs.map(w => `<li><span><span class="set-tag">${esc(chainName(w.chain_id))}</span> <span class="mono">${esc(w.address)}</span>${w.label ? ' · ' + esc(w.label) : ''}</span></li>`).join('')}</ul>`
      : `<div class="set-empty">${esc(tk('aiwallet_none'))}</div>`;
    return `<div style="margin-bottom:14px;"><div style="font-weight:600;font-size:0.88rem;margin-bottom:4px;">🤖 ${esc(a.agent_id)}</div>${inner}</div>`;
  }).join('');
}

// cleanErr — buang prefix "NNN:" dari fetchJSON Error biar pesan rapi.
// ── Notifikasi (Telegram owner-level, TERPISAH dari agent) ──────────────────
async function renderNotify(panel) {
  const tk = (k) => t('menu.tab.settings.' + k);
  panel.innerHTML = `
    <div class="set-card">
      <h3>${esc(tk('notify_h'))}</h3>
      <div class="sub">${esc(tk('notify_sub'))}</div>
      <div class="set-row"><input type="password" id="ntToken" placeholder="${escAttr(tk('notify_token_ph'))}" autocomplete="off"></div>
      <div class="set-row"><input type="text" id="ntChat" placeholder="${escAttr(tk('notify_chat_ph'))}"></div>
      <div class="set-tag" style="margin-bottom:10px;">${esc(tk('notify_chat_hint'))}</div>
      <div class="set-row">
        <button class="set-btn-primary" id="ntSave">${esc(tk('notify_save'))}</button>
        <button class="set-btn-primary" id="ntTest" style="background:linear-gradient(135deg,#22c55e,#15803d);">${esc(tk('notify_test'))}</button>
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
    if (d.set && d.bot_token_masked) tokenEl.setAttribute('placeholder', d.bot_token_masked + ' (tersimpan — kosongin biar ga ganti)');
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
