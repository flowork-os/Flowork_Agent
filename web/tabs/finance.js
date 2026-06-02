// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Tab Finance gabungan (API cost/budget real finance_ledger + wallet
//   personal). Audit pass — esc semua field user, fetchJSON util, ga ada
//   innerHTML mentah. Backend shape E2E verified.
//
// finance.js — Finance dashboard (mode GABUNGAN).
//
// Nampilin data REAL yang ada di backend:
//   - API cost 7 hari (finance_ledger Section 23) — total + per-kategori
//   - Budget + % terpakai
//   - Recent calls (ledger)
//   - Wallet personal (alamat dari Settings; total saldo on-demand)
//
// Sumber data: /api/finance/snapshot (cost/budget, lokal cepat) +
// /api/settings/wallet/{addresses,portfolio} (wallet personal owner).

import { esc, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('finance.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

const CSS = `
.fn-wrap { max-width: 920px; }
.fn-head { margin-bottom: 16px; }
.fn-head h2 { margin: 0; }
.fn-head .sub { font-size: 0.82rem; color: var(--text-muted); margin-top: 4px; }
.fn-kpis { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px,1fr)); gap: 12px; margin-bottom: 18px; }
.fn-kpi { background: rgba(15,17,26,0.6); border: 1px solid var(--glass-border); border-radius: 14px; padding: 16px 18px; }
.fn-kpi .lbl { font-size: 0.72rem; text-transform: uppercase; letter-spacing: .06em; color: var(--text-muted); }
.fn-kpi .val { font-size: 1.6rem; font-weight: 700; margin-top: 4px; }
.fn-kpi .val.green { color: #86efac; } .fn-kpi .val.amber { color: #fbbf24; }
.fn-card { background: rgba(15,17,26,0.6); border: 1px solid var(--glass-border); border-radius: 14px; padding: 16px 18px; margin-bottom: 16px; }
.fn-card h3 { margin: 0 0 4px; font-size: 0.95rem; }
.fn-card .hint { font-size: 0.78rem; color: var(--text-muted); margin-bottom: 12px; }
.fn-row { display:flex; justify-content:space-between; align-items:center; padding: 8px 0; border-bottom: 1px solid var(--glass-border); font-size: 0.86rem; }
.fn-row:last-child { border-bottom: none; }
.fn-tag { font-size: 0.7rem; color: #64748b; }
.fn-bar { height: 8px; border-radius: 5px; background: rgba(148,163,184,0.15); overflow: hidden; margin-top: 6px; }
.fn-bar > span { display:block; height: 100%; background: linear-gradient(90deg,#a78bfa,#7c3aed); }
.fn-bar.warn > span { background: linear-gradient(90deg,#fbbf24,#f59e0b); }
.fn-bar.over > span { background: linear-gradient(90deg,#ef4444,#b91c1c); }
.fn-table { width: 100%; border-collapse: collapse; font-size: 0.82rem; }
.fn-table th { text-align: left; color: var(--text-muted); font-weight: 600; padding: 6px 8px; border-bottom: 1px solid var(--glass-border); font-size: 0.72rem; text-transform: uppercase; }
.fn-table td { padding: 6px 8px; border-bottom: 1px solid rgba(148,163,184,0.08); }
.fn-empty { color: var(--text-muted); font-size: 0.84rem; padding: 10px 0; }
.fn-btn { padding: 7px 13px; border-radius: 8px; background: rgba(139,92,246,0.18); border: 1px solid rgba(139,92,246,0.4); color: #c4b5fd; cursor: pointer; font-size: 0.8rem; }
.fn-mono { font-family: monospace; font-size: 0.78rem; color: #cbd5e1; }
`;

const usd = (n) => '$' + (Number(n) || 0).toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

export async function render(mainEl) {
  loadStyle('finance', CSS);
  mainEl.innerHTML = `
    <div class="fn-wrap">
      <div class="fn-head">
        <h2>${esc(L.title)}</h2>
        <div class="sub">${esc(L.sub)} <span id="fnUpd"></span></div>
      </div>
      <div class="fn-kpis" id="fnKpis"></div>
      <div class="fn-card" id="fnWallet"></div>
      <div class="fn-card" id="fnCat"></div>
      <div class="fn-card" id="fnBudget"></div>
      <div class="fn-card" id="fnRecent"></div>
    </div>
  `;
  try {
    const d = await fetchJSON('/api/finance/snapshot');
    renderKpis(mainEl, d);
    renderCategories(mainEl.querySelector('#fnCat'), d.api_cost_by_category || []);
    renderBudgets(mainEl.querySelector('#fnBudget'), d.budgets || []);
    renderRecent(mainEl.querySelector('#fnRecent'), d.recent_calls || []);
    const upd = mainEl.querySelector('#fnUpd');
    if (upd && d.updated_at) upd.textContent = esc(L.updated) + new Date(d.updated_at).toLocaleString('id-ID');
  } catch (e) {
    mainEl.querySelector('#fnCat').innerHTML = `<div class="fn-empty">${esc(L.loadFail)}${esc(String(e.message || e))}</div>`;
  }
  renderWallet(mainEl.querySelector('#fnWallet'));
}

function renderKpis(root, d) {
  const total = d.api_cost_total_usd || 0;
  const cats = (d.api_cost_by_category || []).length;
  const calls = (d.api_cost_by_category || []).reduce((s, c) => s + (c.call_count || 0), 0);
  root.querySelector('#fnKpis').innerHTML = `
    <div class="fn-kpi"><div class="lbl">${esc(L.cost7d)}</div><div class="val green">${usd(total)}</div></div>
    <div class="fn-kpi"><div class="lbl">${esc(L.totalCalls)}</div><div class="val">${calls.toLocaleString('id-ID')}</div></div>
    <div class="fn-kpi"><div class="lbl">${esc(L.category)}</div><div class="val">${cats}</div></div>
  `;
}

function renderCategories(el, cats) {
  el.innerHTML = `<h3>${esc(L.costByCat)}</h3><div class="hint">${esc(L.costByCatSub)}</div>`;
  if (!cats.length) { el.innerHTML += `<div class="fn-empty">${esc(L.noCost)}</div>`; return; }
  el.innerHTML += cats.map(c => `
    <div class="fn-row">
      <span>${esc(c.category || '—')} <span class="fn-tag">${(c.call_count || 0)} call · ${(c.input_tokens || 0)}→${(c.output_tokens || 0)} tok</span></span>
      <span class="fn-mono">${usd(c.cost_usd)}</span>
    </div>`).join('');
}

function renderBudgets(el, budgets) {
  el.innerHTML = `<h3>${esc(L.budget)}</h3><div class="hint">${esc(L.budgetHint)}</div>`;
  if (!budgets.length) { el.innerHTML += `<div class="fn-empty">${esc(L.noBudget)}</div>`; return; }
  el.innerHTML += budgets.map(b => {
    const pct = Math.max(0, Math.min(100, b.pct || 0));
    const cls = pct >= 100 ? 'over' : (pct >= (b.warning_at_pct || 80) ? 'warn' : '');
    return `
    <div style="padding:8px 0;">
      <div class="fn-row" style="border:none;padding-bottom:2px;">
        <span>${esc(b.metric_key)} ${b.enabled ? '' : '<span class="fn-tag">(off)</span>'}</span>
        <span class="fn-mono">${usd(b.spent_usd)} / ${usd(b.budget_value)} · ${pct.toFixed(0)}%</span>
      </div>
      <div class="fn-bar ${cls}"><span style="width:${pct}%"></span></div>
    </div>`;
  }).join('');
}

function renderRecent(el, rows) {
  el.innerHTML = `<h3>${esc(L.recentCalls)}</h3>`;
  if (!rows.length) { el.innerHTML += `<div class="fn-empty">${esc(L.noCalls)}</div>`; return; }
  el.innerHTML += `<table class="fn-table"><thead><tr><th>${esc(L.time)}</th><th>${esc(L.category)}</th><th>${esc(L.model)}</th><th>${esc(L.token)}</th><th>${esc(L.cost)}</th></tr></thead><tbody>${
    rows.map(r => `<tr>
      <td class="fn-tag">${r.occurred_at ? new Date(r.occurred_at).toLocaleString('id-ID', { day:'2-digit', month:'short', hour:'2-digit', minute:'2-digit' }) : '—'}</td>
      <td>${esc(r.category || '—')}</td>
      <td>${esc(r.model || r.provider || '—')}</td>
      <td class="fn-tag">${(r.input_tokens||0)}→${(r.output_tokens||0)}</td>
      <td class="fn-mono">${usd(r.cost_usd)}</td>
    </tr>`).join('')
  }</tbody></table>`;
}

async function renderWallet(el) {
  el.innerHTML = `<h3>${esc(L.wallet)}</h3><div class="hint">${esc(L.walletSub)}</div><div id="fnWList" class="fn-empty">${esc(L.loading)}</div>`;
  const list = el.querySelector('#fnWList');
  try {
    const d = await fetchJSON('/api/settings/wallet/addresses');
    const items = d.items || [];
    if (!items.length) { list.outerHTML = `<div class="fn-empty">${esc(L.noWallet)}</div>`; return; }
    list.outerHTML = `
      <div class="fn-row" style="border:none;"><span>${items.length} alamat tersimpan</span>
        <button class="fn-btn" id="fnWTotal">${esc(L.calcBalance)}</button></div>
      <div id="fnWTotalOut"></div>`;
    el.querySelector('#fnWTotal').addEventListener('click', async (ev) => {
      const out = el.querySelector('#fnWTotalOut');
      ev.target.disabled = true; out.innerHTML = `<div class="fn-empty">${esc(L.fetching)}</div>`;
      try {
        const p = await fetchJSON('/api/settings/wallet/portfolio');
        out.innerHTML = `<div class="fn-row"><span>${esc(L.totalPortfolio)}</span><span class="val green fn-mono">${usd(p.total_usd)}</span></div>${p.partial_error ? `<div class="fn-empty">${esc(p.partial_error)}</div>` : ''}`;
      } catch (e) {
        out.innerHTML = `<div class="fn-empty">${esc(L.walletErr)}${esc(String(e.message || e))}${esc(L.walletErrHint)}</div>`;
        ev.target.disabled = false;
      }
    });
  } catch (e) {
    list.outerHTML = `<div class="fn-empty">${esc(L.walletLoadFail)}${esc(String(e.message || e))}</div>`;
  }
}
