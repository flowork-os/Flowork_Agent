// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Finance tab (reference 417 LOC). Audit pass — esc() on ledger entry+budget+amount, no innerHTML on user fields raw..

import { esc, fetchJSON, loadStyle } from '../js/utils.js';

const CSS = `
.fn-shell { display: flex; flex-direction: column; gap: 16px; }

.fn-kpis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
}
.fn-kpi {
  position: relative;
  background:
    linear-gradient(160deg, rgba(255,255,255,0.05), rgba(255,255,255,0) 40%),
    radial-gradient(circle at 80% 120%, rgba(139,92,246,0.08), transparent 55%),
    rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  padding: 16px 14px;
  text-align: center;
  backdrop-filter: blur(14px);
  overflow: hidden;
  box-shadow: 0 6px 16px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.06);
  transition: transform 0.22s, box-shadow 0.22s;
}
.fn-kpi::after {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 2px;
  background: linear-gradient(90deg, transparent, var(--kc, #8b5cf6), transparent);
  opacity: 0.6;
}
.fn-kpi:hover { transform: translateY(-3px); box-shadow: 0 12px 26px rgba(0,0,0,0.42), inset 0 1px 0 rgba(255,255,255,0.08); }
.fn-kpi .v { font-size: 1.5rem; font-family: var(--font-heading); font-weight: 700; color: #f8fafc; line-height: 1; }
.fn-kpi .l { font-size: 0.66rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em; margin-top: 5px; font-family: monospace; }
.fn-kpi.success { --kc: #10b981; }
.fn-kpi.success .v { color: #6ee7b7; }
.fn-kpi.warn { --kc: #f59e0b; }
.fn-kpi.warn .v { color: #fcd34d; }
.fn-kpi.danger { --kc: #ef4444; }
.fn-kpi.danger .v { color: #fca5a5; }
.fn-kpi.info { --kc: #3b82f6; }
.fn-kpi.info .v { color: #93c5fd; }

.fn-card {
  position: relative;
  background:
    radial-gradient(circle at 15% 0%, rgba(139,92,246,0.06), transparent 55%),
    linear-gradient(165deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 50%),
    rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  padding: 18px;
  backdrop-filter: blur(14px);
  box-shadow: 0 6px 18px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.05);
  overflow: hidden;
}
.fn-card::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.12), transparent);
  pointer-events: none;
}
.fn-card h3 { font-family: var(--font-heading); font-size: 0.95rem; color: #e2e8f0; margin: 0 0 4px; font-weight: 600; }
.fn-card .hint { font-size: 0.72rem; color: var(--text-muted); margin-bottom: 14px; }

.fn-profile-label {
  font-size: 0.7rem;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  font-weight: 700;
  font-family: monospace;
  margin: 14px 0 8px;
}
.fn-wallets {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 10px;
}
.fn-wrow {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  background:
    linear-gradient(160deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 45%),
    rgba(15, 23, 42, 0.5);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
}
.fn-wrow:hover {
  transform: translateY(-2px);
  border-color: rgba(139,92,246,0.3);
  box-shadow: 0 8px 18px rgba(0,0,0,0.32), inset 0 1px 0 rgba(255,255,255,0.06);
}
.fn-wic {
  width: 38px; height: 38px;
  border-radius: 12px;
  display: flex; align-items: center; justify-content: center;
  font-size: 1.2rem;
  background:
    radial-gradient(circle at 30% 25%, rgba(255,255,255,0.35), transparent 55%),
    linear-gradient(135deg, var(--wc, #8b5cf6), var(--wcd, #6d28d9));
  border: 1px solid rgba(255,255,255,0.2);
  box-shadow: 0 4px 10px rgba(0,0,0,0.35), inset 0 1px 0 rgba(255,255,255,0.25);
  flex-shrink: 0;
}
.fn-wmid { flex: 1; min-width: 0; }
.fn-wsym { font-family: var(--font-heading); font-size: 0.88rem; font-weight: 700; color: #f8fafc; }
.fn-wlabel { font-size: 0.68rem; color: var(--text-muted); font-family: monospace; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.fn-wbal { font-family: monospace; font-size: 0.9rem; font-weight: 700; color: #cbd5e1; text-align: right; white-space: nowrap; flex-shrink: 0; }
.fn-wbal.stab { color: #6ee7b7; }
.fn-wtime { font-size: 0.62rem; color: #475569; text-align: right; margin-top: 2px; }

.fn-ledger { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
.fn-ledger-head-row {
  display: grid;
  grid-template-columns: 60px 1fr 1fr 1fr;
  gap: 10px;
  padding: 0 14px 4px;
}
.fn-ledger-head-row span { font-size: 0.66rem; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.06em; font-family: monospace; text-align: right; }
.fn-ledger-head-row span:first-child { text-align: left; }
.fn-ledger-row {
  display: grid;
  grid-template-columns: 60px 1fr 1fr 1fr;
  gap: 10px;
  align-items: center;
  padding: 12px 14px;
  background: rgba(15, 23, 42, 0.5);
  border: 1px solid var(--glass-border);
  border-radius: 10px;
  font-family: monospace;
  font-size: 0.82rem;
}
.fn-ledger-curr { font-weight: 700; color: #cbd5e1; }
.fn-ledger-val { text-align: right; }
.fn-rev { color: #6ee7b7; }
.fn-exp { color: #fca5a5; }
.fn-net-pos { color: #6ee7b7; font-weight: 700; }
.fn-net-neg { color: #fca5a5; font-weight: 700; }

.fn-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  background: rgba(15, 17, 26, 0.5);
  border: 1px solid var(--glass-border);
  border-radius: 10px;
  font-size: 0.72rem;
  color: var(--text-muted);
  font-family: monospace;
}
.fn-refresh-btn {
  background: linear-gradient(135deg, rgba(139,92,246,0.2), rgba(124,58,237,0.1));
  border: 1px solid rgba(139,92,246,0.4);
  color: #ddd6fe;
  padding: 6px 14px;
  border-radius: 8px;
  font-size: 0.74rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
}
.fn-refresh-btn:hover { background: rgba(139,92,246,0.35); }
.fn-refresh-btn:disabled { opacity: 0.5; cursor: not-allowed; }

.fn-empty { text-align: center; padding: 28px 20px; color: var(--text-muted); font-style: italic; font-size: 0.85rem; }
.fn-err { color: #fca5a5; padding: 12px; background: rgba(239,68,68,0.08); border: 1px solid rgba(239,68,68,0.25); border-radius: 8px; font-size: 0.82rem; }

@media (max-width: 700px) {
  .fn-wallets { grid-template-columns: 1fr; }
  .fn-ledger-row, .fn-ledger-head-row { grid-template-columns: 50px 1fr 1fr 1fr; gap: 6px; }
}
`;

const ASSET_META = {
  USDT:  { ic: '💵', c: '#26a17b', d: '#1a7356', stab: true  },
  USDC:  { ic: '💵', c: '#2775ca', d: '#1e5a9e', stab: true  },
  ETH:   { ic: '💎', c: '#627eea', d: '#3c5ad6', stab: false },
  SOL:   { ic: '⚡', c: '#9945ff', d: '#7c33d4', stab: false },
  BTC:   { ic: '₿',  c: '#f7931a', d: '#c87115', stab: false },
  BNB:   { ic: '🔶', c: '#f0b90b', d: '#c79708', stab: false },
  USD:   { ic: '💳', c: '#2775ca', d: '#1e5a9e', stab: true  },
};

function assetMeta(sym) {
  return ASSET_META[(sym || '').toUpperCase()] || { ic: '💰', c: '#64748b', d: '#475569', stab: false };
}

function parseAsset(asset) {
  const s = asset || '';
  if (s.toLowerCase().startsWith('owner_')) return { profile: 'owner', sym: s.slice(6) };
  if (s.toLowerCase().startsWith('warga_')) return { profile: 'warga', sym: s.slice(6) };
  if (s.toLowerCase().startsWith('paypal_')) return { profile: 'paypal', sym: s.slice(7) };
  return { profile: 'other', sym: s };
}

function fmtBal(n) {
  if (n == null || isNaN(n) || n === 0) return '0';
  if (n < 0.0001) return n.toExponential(2);
  if (n < 1) return n.toFixed(6);
  if (n < 10000) return n.toFixed(4);
  return n.toLocaleString('en-US', { maximumFractionDigits: 2 });
}

function fmtMoney(amount, currency) {
  if (!amount) return currency === 'IDR' ? 'Rp0' : '$0.00';
  if ((currency || '').toUpperCase() === 'IDR') return 'Rp' + Math.round(amount).toLocaleString('id-ID');
  return '$' + amount.toFixed(2);
}

function shortAddr(s) {
  if (!s) return '—';
  if (s.length <= 14) return s;
  return s.slice(0, 7) + '…' + s.slice(-5);
}

function timeAgo(iso) {
  if (!iso) return '—';
  const d = (Date.now() - new Date(iso)) / 60000;
  if (isNaN(d) || d < 0) return '—';
  if (d < 1) return 'baru saja';
  if (d < 60) return `${Math.round(d)}m lalu`;
  if (d < 1440) return `${Math.round(d / 60)}j lalu`;
  return `${Math.round(d / 1440)}hr lalu`;
}

let refreshTimer = null;

export async function render(mainEl) {
  loadStyle('finance-snap', CSS);
  if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = null; }

  mainEl.innerHTML = `
    <h2>💳 Finance Snapshot</h2>
    <div class="sub">Saldo wallet + revenue/expense 7 hari dari brain DB. Wallet di-sync tiap jam oleh wallet_sync daemon.</div>
    <div class="fn-shell">
      <div class="fn-kpis" id="fnKpis"><div class="fn-empty">Memuat…</div></div>
      <div class="fn-card" id="fnWallets"><div class="fn-empty">Memuat wallet…</div></div>
      <div class="fn-card" id="fnLedger"><div class="fn-empty">Memuat ledger…</div></div>
      <div class="fn-footer">
        <span id="fnUpdated">—</span>
        <button class="fn-refresh-btn" id="fnRefreshBtn" title="Refresh manual — Paksa reload snapshot sekarang (cache backend 30 detik). Data wallet di otak brain DB di-update oleh wallet_sync daemon tiap jam, bukan oleh tombol ini.">🔄 Refresh</button>
      </div>
    </div>
  `;

  document.getElementById('fnRefreshBtn').addEventListener('click', async (ev) => {
    const btn = ev.currentTarget;
    btn.disabled = true;
    await loadAndRender();
    setTimeout(() => { if (document.getElementById('fnRefreshBtn')) btn.disabled = false; }, 2500);
  });

  await loadAndRender();

  refreshTimer = setInterval(() => {
    if (!document.getElementById('fnKpis')) { clearInterval(refreshTimer); refreshTimer = null; return; }
    loadAndRender();
  }, 5 * 60 * 1000);
}

async function loadAndRender() {
  try {
    const d = await fetchJSON('/api/finance/snapshot');
    renderKpis(d);
    renderWallets(d.wallet_latest || []);
    renderLedger(d.revenue_7d_total || {}, d.expense_7d_total || {}, d.net_7d_by_currency || {});
    const upd = document.getElementById('fnUpdated');
    if (upd && d.updated_at) upd.textContent = 'Diperbarui: ' + new Date(d.updated_at).toLocaleString('id-ID');
  } catch (e) {
    const kpis = document.getElementById('fnKpis');
    if (kpis) kpis.innerHTML = `<div class="fn-err">Gagal muat snapshot: ${esc(e.message)}</div>`;
  }
}

function renderKpis(d) {
  const el = document.getElementById('fnKpis');
  if (!el) return;

  const wallets = d.wallet_latest || [];
  const rev = d.revenue_7d_total || {};
  const exp = d.expense_7d_total || {};
  const net = d.net_7d_by_currency || {};

  let totalStab = 0;
  wallets.forEach(w => {
    const { sym } = parseAsset(w.asset);
    if (assetMeta(sym).stab) totalStab += (w.balance || 0);
  });

  const revUSD = rev.USD || 0;
  const expUSD = exp.USD || 0;
  const netUSD = net.USD || 0;

  const monthlyExpUSD = expUSD * (30 / 7);
  const runwayMonths = monthlyExpUSD > 0 ? totalStab / monthlyExpUSD : null;
  const runwayLabel = runwayMonths == null ? '∞ mo' : (runwayMonths > 99 ? '99+ mo' : runwayMonths.toFixed(1) + ' mo');
  const runwayCls = runwayMonths == null ? '' : runwayMonths < 1 ? 'danger' : runwayMonths < 3 ? 'warn' : 'success';

  const cells = [
    { v: wallets.length || '—', l: 'Asset Tracked', cls: wallets.length > 0 ? 'info' : '' },
    { v: '$' + totalStab.toFixed(2), l: 'Stablecoin (USD)', cls: totalStab > 0 ? 'success' : '' },
    { v: '$' + revUSD.toFixed(2), l: 'Revenue 7d', cls: revUSD > 0 ? 'success' : '' },
    { v: '$' + expUSD.toFixed(2), l: 'Expense 7d', cls: expUSD > 0 ? 'warn' : '' },
    { v: (netUSD >= 0 ? '+' : '') + '$' + netUSD.toFixed(2), l: 'Net 7d (USD)', cls: netUSD >= 0 ? 'success' : 'danger' },
    { v: runwayLabel, l: 'Runway (USD)', cls: runwayCls },
  ];

  el.innerHTML = cells.map(c => `
    <div class="fn-kpi ${c.cls || ''}">
      <div class="v">${esc(String(c.v))}</div>
      <div class="l">${c.l}</div>
    </div>`).join('');
}

function renderWallets(wallets) {
  const el = document.getElementById('fnWallets');
  if (!el) return;

  const PROFILE_LABEL = {
    owner:  '👤 Owner — Ayah (Trust Wallet)',
    warga:  '🤖 Warga Pool (komunal ETH/SOL/BTC)',
    paypal: '💳 PayPal Business',
    other:  '🔧 Lainnya',
  };

  let html = `
    <h3>💰 Wallet Snapshot</h3>
    <div class="hint">Latest balance per asset dari brain DB (wallet_snapshots). Di-sync tiap jam oleh wallet_sync daemon. Beda dari tab Wallet yang real-time via Etherscan.</div>`;

  if (!wallets.length) {
    html += '<div class="fn-empty">Belum ada snapshot wallet. Akan terisi dalam ±1 jam sejak GUI pertama dijalankan.</div>';
    el.innerHTML = html;
    return;
  }

  const grouped = { owner: [], warga: [], paypal: [], other: [] };
  wallets.forEach(w => {
    const { profile } = parseAsset(w.asset);
    (grouped[profile] || grouped.other).push(w);
  });

  ['owner', 'warga', 'paypal', 'other'].forEach(profile => {
    const rows = grouped[profile];
    if (!rows.length) return;
    html += `<div class="fn-profile-label">${PROFILE_LABEL[profile]}</div><div class="fn-wallets">`;
    rows.forEach(w => {
      const { sym } = parseAsset(w.asset);
      const meta = assetMeta(sym);
      const balCls = meta.stab ? ' stab' : '';
      html += `
        <div class="fn-wrow" style="--wc:${meta.c};--wcd:${meta.d}" title="${esc(w.notes || '')}">
          <div class="fn-wic">${meta.ic}</div>
          <div class="fn-wmid">
            <div class="fn-wsym">${esc(sym)}</div>
            <div class="fn-wlabel" title="${esc(w.account_label || '')}">${esc(shortAddr(w.account_label))}</div>
          </div>
          <div>
            <div class="fn-wbal${balCls}">${fmtBal(w.balance)}</div>
            <div class="fn-wtime">${timeAgo(w.captured_at)}</div>
          </div>
        </div>`;
    });
    html += '</div>';
  });

  el.innerHTML = html;
}

function renderLedger(rev, exp, net) {
  const el = document.getElementById('fnLedger');
  if (!el) return;

  const currencies = [...new Set([...Object.keys(rev), ...Object.keys(exp), ...Object.keys(net)])];

  let html = `
    <h3>📊 Revenue vs Expense — 7 Hari Terakhir</h3>
    <div class="hint">Agregat dari revenue_log + expense_log brain DB dalam 7 hari terakhir. Append via tool finance.append_revenue / finance.append_expense (capability finance_write, Ayah toggle per warga).</div>`;

  if (!currencies.length) {
    html += '<div class="fn-empty">Belum ada entri revenue/expense dalam 7 hari terakhir.</div>';
    el.innerHTML = html;
    return;
  }

  html += `
    <div class="fn-ledger">
      <div class="fn-ledger-head-row">
        <span>Mata Uang</span>
        <span>Revenue</span>
        <span>Expense</span>
        <span>Net</span>
      </div>`;

  currencies.forEach(cur => {
    const r = rev[cur] || 0;
    const e = exp[cur] || 0;
    const n = net[cur] || 0;
    const netCls = n >= 0 ? 'fn-net-pos' : 'fn-net-neg';
    html += `
      <div class="fn-ledger-row">
        <div class="fn-ledger-curr">${esc(cur)}</div>
        <div class="fn-ledger-val fn-rev">${esc(fmtMoney(r, cur))}</div>
        <div class="fn-ledger-val fn-exp">${esc(fmtMoney(e, cur))}</div>
        <div class="fn-ledger-val ${netCls}">${n >= 0 ? '+' : ''}${esc(fmtMoney(n, cur))}</div>
      </div>`;
  });

  html += '</div>';
  el.innerHTML = html;
}
