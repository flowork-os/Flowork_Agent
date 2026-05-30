// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Wallet tab (reference 414 LOC). Audit pass — esc() on address+amount+holdings, fetchJSON via util..

import { esc, fetchJSON, loadStyle, validateShape } from '../js/utils.js';

const CSS = `
.wl-shell {
  display: grid;
  grid-template-columns: minmax(380px, 1fr) minmax(320px, 1fr);
  gap: 16px;
  height: calc(100vh - 220px);
  min-height: 520px;
  overflow: hidden;
}
.wl-card {
  position: relative;
  background:
    radial-gradient(circle at 15% 0%, rgba(139, 92, 246, 0.06), transparent 55%),
    linear-gradient(165deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 50%),
    rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  backdrop-filter: blur(14px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-height: 0;
  box-shadow: 0 8px 24px rgba(0,0,0,0.35), inset 0 1px 0 rgba(255,255,255,0.05);
}
.wl-card::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: linear-gradient(90deg, transparent, rgba(255,255,255,0.12), transparent);
  pointer-events: none;
  z-index: 2;
}
.wl-head {
  padding: 14px 18px;
  border-bottom: 1px solid var(--glass-border);
  background: linear-gradient(180deg, rgba(0,0,0,0.28), rgba(0,0,0,0.08));
  flex-shrink: 0;
  font-family: var(--font-heading);
  font-size: 0.82rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--text-muted);
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.wl-head .addr { font-size: 0.7rem; color: #64748b; font-family: monospace; text-transform: none; letter-spacing: 0; font-weight: 500; }
.wl-body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 20px;
  scrollbar-width: thin;
  scrollbar-color: rgba(139, 92, 246, 0.3) transparent;
}
.wl-body::-webkit-scrollbar { width: 5px; }
.wl-body::-webkit-scrollbar-thumb { background: rgba(139, 92, 246, 0.3); border-radius: 3px; }

.wl-total {
  position: relative;
  text-align: center;
  padding: 28px 16px;
  background:
    radial-gradient(ellipse 70% 50% at 50% 100%, rgba(139, 92, 246, 0.18), transparent 65%),
    linear-gradient(160deg, rgba(255,255,255,0.06), rgba(255,255,255,0) 45%),
    rgba(15, 23, 42, 0.5);
  border: 1px solid rgba(139, 92, 246, 0.25);
  border-radius: 14px;
  margin-bottom: 18px;
  box-shadow: 0 8px 20px rgba(0,0,0,0.35), inset 0 1px 0 rgba(255,255,255,0.08);
  overflow: hidden;
}
.wl-total::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: linear-gradient(90deg, transparent, rgba(196,181,253,0.5), transparent);
  pointer-events: none;
}
.wl-total .lbl {
  font-size: 0.68rem;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.15em;
  font-weight: 700;
  font-family: monospace;
}
.wl-total .val {
  font-size: 2.8rem;
  font-family: var(--font-heading);
  font-weight: 700;
  background: linear-gradient(135deg, #fff, #c4b5fd);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.02em;
  margin-top: 6px;
}
.wl-total .hint { font-size: 0.7rem; color: var(--text-muted); margin-top: 8px; font-family: monospace; }

.wl-warn {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.3);
  border-left: 3px solid #f59e0b;
  border-radius: 10px;
  padding: 10px 14px;
  font-size: 0.75rem;
  color: #fcd34d;
  margin-bottom: 16px;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
}
.wl-warn b { color: #fef3c7; }
.wl-warn ul { margin: 4px 0 0 18px; padding: 0; }
.wl-warn li { font-size: 0.72rem; margin: 2px 0; list-style: disc; }

.wl-holdings { display: flex; flex-direction: column; gap: 8px; }
.wl-holding {
  display: grid;
  grid-template-columns: 42px 1fr auto;
  gap: 12px;
  align-items: center;
  padding: 12px 14px;
  background:
    linear-gradient(160deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 45%),
    rgba(15, 23, 42, 0.5);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04);
}
.wl-holding:hover {
  transform: translateY(-2px);
  border-color: rgba(139, 92, 246, 0.3);
  box-shadow: 0 8px 18px rgba(0,0,0,0.32), inset 0 1px 0 rgba(255,255,255,0.06);
}
.wl-ic {
  position: relative;
  width: 42px; height: 42px;
  border-radius: 12px;
  background:
    radial-gradient(circle at 30% 25%, rgba(255,255,255,0.35), transparent 50%),
    linear-gradient(135deg, var(--wc, #8b5cf6), var(--wcd, #7c3aed));
  border: 1px solid rgba(255, 255, 255, 0.2);
  display: flex; align-items: center; justify-content: center;
  font-size: 1.05rem; color: #fff;
  flex-shrink: 0;
  font-weight: 700;
  font-family: var(--font-heading);
  letter-spacing: 0.02em;
  box-shadow:
    0 4px 10px rgba(0,0,0,0.35),
    inset 0 1px 0 rgba(255,255,255,0.25),
    inset 0 -2px 4px rgba(0,0,0,0.25);
  text-shadow: 0 1px 2px rgba(0,0,0,0.35);
}
.wl-ic::after {
  content: '';
  position: absolute;
  inset: 3px 3px auto 3px;
  height: 45%;
  border-radius: 10px 10px 50% 50% / 8px 8px 30% 30%;
  background: linear-gradient(180deg, rgba(255,255,255,0.3), rgba(255,255,255,0));
  pointer-events: none;
}
.wl-mid { min-width: 0; }
.wl-sym { font-family: var(--font-heading); font-size: 0.95rem; font-weight: 600; color: #f8fafc; margin-bottom: 2px; }
.wl-chain { font-size: 0.7rem; color: var(--text-muted); font-family: monospace; }
.wl-right { text-align: right; }
.wl-amt { font-size: 0.9rem; font-weight: 600; color: #cbd5e1; font-family: monospace; }
.wl-usd { font-size: 0.78rem; color: var(--text-muted); font-family: monospace; margin-top: 2px; }
.wl-usd-nonzero { color: #6ee7b7; }

.wl-empty { text-align: center; padding: 40px 20px; color: var(--text-muted); font-style: italic; font-size: 0.85rem; }

.wl-tx {
  display: grid;
  grid-template-columns: auto 1fr auto;
  gap: 10px;
  padding: 10px 12px;
  background:
    linear-gradient(160deg, rgba(255,255,255,0.03), rgba(255,255,255,0) 45%),
    rgba(15, 23, 42, 0.45);
  border: 1px solid var(--glass-border);
  border-radius: 10px;
  margin-bottom: 6px;
  font-size: 0.78rem;
}
.wl-tx-dir {
  font-size: 0.62rem;
  font-weight: 700;
  padding: 2px 8px;
  border-radius: 4px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  align-self: flex-start;
}
.wl-tx-in { background: rgba(16,185,129,0.18); color: #6ee7b7; border: 1px solid rgba(16,185,129,0.3); }
.wl-tx-out { background: rgba(239,68,68,0.18); color: #fca5a5; border: 1px solid rgba(239,68,68,0.3); }
.wl-tx-mid { min-width: 0; }
.wl-tx-amt { color: #cbd5e1; font-family: monospace; font-weight: 600; }
.wl-tx-hash { color: #64748b; font-size: 0.68rem; font-family: monospace; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 120px; }
.wl-tx-ts { font-size: 0.68rem; color: #64748b; font-family: monospace; text-align: right; align-self: flex-start; }
.wl-err { color: var(--danger); font-size: 0.82rem; padding: 12px; background: rgba(239, 68, 68, 0.08); border: 1px solid rgba(239, 68, 68, 0.25); border-radius: 8px; }

@media (max-width: 900px) {
  .wl-shell { grid-template-columns: 1fr; height: auto; }
}
`;

const CHAIN_THEME = {
  Ethereum: { c: '#627eea', d: '#3c5ad6' },
  Polygon:  { c: '#8247e5', d: '#5c2ea6' },
  Arbitrum: { c: '#28a0f0', d: '#1e74c7' },
  BSC:      { c: '#f0b90b', d: '#c79708' },
  Optimism: { c: '#ff0420', d: '#c70519' },
  Base:     { c: '#0052ff', d: '#003fbf' },
};

function chainTheme(name) {
  return CHAIN_THEME[name] || { c: '#64748b', d: '#475569' };
}

function parseWarnings(partial) {
  if (!partial) return [];
  return partial.split(';').map(s => s.trim()).filter(Boolean);
}

function shortHash(h) {
  if (!h) return '—';
  if (h.length <= 14) return h;
  return h.slice(0, 8) + '…' + h.slice(-4);
}

function fmtAmount(n) {
  if (n == null || isNaN(n)) return '0';
  if (n === 0) return '0';
  if (n < 0.0001) return n.toExponential(2);
  if (n < 1) return n.toFixed(6);
  if (n < 1000) return n.toFixed(4);
  return n.toLocaleString('en-US', { maximumFractionDigits: 2 });
}

let refreshTimer = null;

export async function render(mainEl) {
  loadStyle('wallet', CSS);
  if (refreshTimer) clearInterval(refreshTimer);

  mainEl.innerHTML = `
    <h2>💰 Keuangan AI</h2>
    <div class="sub">Pemantauan saldo dompet crypto operasional via Etherscan V2 + CoinGecko. Data riil, zero mock.</div>
    <div class="wl-shell">
      <div class="wl-card">
        <div class="wl-head">
          <span>Holdings & Saldo</span>
          <span class="addr" id="wlAddr">—</span>
        </div>
        <div class="wl-body" id="wlBal"><div class="wl-empty">Scanning blockchain…</div></div>
      </div>
      <div class="wl-card">
        <div class="wl-head"><span>Transaksi Terakhir</span></div>
        <div class="wl-body" id="wlTx"><div class="wl-empty">Memuat histori TX…</div></div>
      </div>
    </div>
  `;

  await loadAndRender();
  refreshTimer = setInterval(loadAndRender, 30000);
}

async function loadAndRender() {
  await Promise.all([loadBalance(), loadTx()]);
}

// Bug Gemini #51 fix (2026-04-27): tambah watchdog UI supaya kalau backend
// take full 20s context timeout, user tetap dapat feedback progres bukan
// stuck di "Scanning blockchain…" tanpa kabar. After 10s → text update;
// after 22s → fail message dengan diagnostic hint. Jangan abort fetch —
// backend timeout (20s) sudah handle.
function startWatchdog(elId, lateMsg, failMsg) {
  const lateTimer = setTimeout(() => {
    const el = document.getElementById(elId);
    if (el && el.querySelector('.wl-empty')) {
      el.innerHTML = `<div class="wl-empty">${lateMsg}</div>`;
    }
  }, 10000);
  const failTimer = setTimeout(() => {
    const el = document.getElementById(elId);
    if (el && el.querySelector('.wl-empty')) {
      el.innerHTML = `<div class="wl-err">${failMsg}</div>`;
    }
  }, 22000);
  return () => { clearTimeout(lateTimer); clearTimeout(failTimer); };
}

async function loadBalance() {
  const el = document.getElementById('wlBal');
  const addrEl = document.getElementById('wlAddr');
  if (!el) return;
  const cancelWatchdog = startWatchdog(
    'wlBal',
    '⏳ Network lambat — masih nunggu Etherscan response…',
    '⚠️ Timeout fetch wallet — cek <code>ETHERSCAN_API_KEY</code> di Setting + status API Etherscan.'
  );
  try {
    const balData = await fetchJSON('/api/wallet');
    cancelWatchdog();
    const bal = validateShape(balData, ['configured'], 'WalletConfig');
    if (!bal.configured) {
      el.innerHTML = `<div class="wl-err">DOMPET BELUM DIKONFIGURASI. Set env <code>TRUST_WALLET_ADDRESS</code> + <code>ETHERSCAN_API_KEY</code>.</div>`;
      return;
    }
    if (addrEl && bal.wallet) addrEl.textContent = shortHash(bal.wallet);

    const total = bal.total_usd || 0;
    const holdings = Array.isArray(bal.holdings) ? bal.holdings : [];
    const nonzero = holdings.filter(h => (h.amount || 0) > 0);

    let html = `
      <div class="wl-total">
        <div class="lbl">Total Portfolio</div>
        <div class="val">$${total.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}</div>
        <div class="hint">across ${holdings.length} chain · ${nonzero.length} asset aktif</div>
      </div>`;

    const warnings = parseWarnings(bal.partial_error);
    if (warnings.length) {
      html += `<div class="wl-warn">
        <b>Catatan fetch</b> — ${warnings.length} sumber belum lengkap (balance diasumsikan 0):
        <ul>${warnings.slice(0, 5).map(w => `<li>${esc(w)}</li>`).join('')}${warnings.length > 5 ? `<li>… +${warnings.length - 5} lagi</li>` : ''}</ul>
      </div>`;
    }

    html += '<div class="wl-holdings">';
    if (!holdings.length) {
      html += '<div class="wl-empty">Tidak ada holding terdeteksi.</div>';
    } else {
      html += holdings.map(h => {
        const theme = chainTheme(h.chain_name);
        const amt = fmtAmount(h.amount);
        const usdv = (h.usd_value || 0);
        const usdCls = usdv > 0 ? ' wl-usd-nonzero' : '';
        const tag = String(h.symbol || '??').slice(0, 4);
        return `
          <div class="wl-holding" style="--wc:${theme.c};--wcd:${theme.d}">
            <div class="wl-ic">${esc(tag)}</div>
            <div class="wl-mid">
              <div class="wl-sym">${esc(h.symbol)} ${h.is_native ? '<span style="color:#64748b;font-size:0.7rem;font-weight:500">native</span>' : ''}</div>
              <div class="wl-chain">${esc(h.chain_name)} · chain ${h.chain_id}</div>
            </div>
            <div class="wl-right">
              <div class="wl-amt">${amt}</div>
              <div class="wl-usd${usdCls}">$${usdv.toFixed(2)}</div>
            </div>
          </div>`;
      }).join('');
    }
    html += '</div>';

    el.innerHTML = html;
  } catch (e) {
    cancelWatchdog();
    el.innerHTML = `<div class="wl-err">Error: ${esc(e.message)}</div>`;
  }
}

async function loadTx() {
  const el = document.getElementById('wlTx');
  if (!el) return;
  const cancelWatchdog = startWatchdog(
    'wlTx',
    '⏳ Network lambat — masih nunggu histori TX…',
    '⚠️ Timeout fetch TX — cek <code>ETHERSCAN_API_KEY</code> + connection.'
  );
  try {
    const txData = await fetchJSON('/api/wallet/tx?limit=10');
    cancelWatchdog();
    const tx = validateShape(txData, ['txs'], 'WalletTx');
    if (tx.error) {
      el.innerHTML = `<div class="wl-err">Gagal fetch TX: ${esc(tx.error)}</div>`;
      return;
    }
    const txs = Array.isArray(tx.txs) ? tx.txs : [];
    if (!txs.length) {
      el.innerHTML = '<div class="wl-empty">Belum ada transaksi di chain manapun.</div>';
      return;
    }
    el.innerHTML = txs.slice(0, 15).map(t => {
      const myAddr = ((t.to || '') + '').toLowerCase();
      const fromAddr = ((t.from || '') + '').toLowerCase();
      const isIn = !!(t.direction ? t.direction === 'in' : (myAddr && fromAddr !== myAddr));
      const ts = t.timestamp ? new Date(t.timestamp) : null;
      const tsFmt = ts && !isNaN(ts) ? ts.toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }) : '—';
      const decimals = t.decimals || 18;
      const valRaw = t.value || '0';
      let amtNum = 0;
      try { amtNum = Number(valRaw) / Math.pow(10, decimals); } catch (_) {}
      return `
        <div class="wl-tx">
          <span class="wl-tx-dir ${isIn ? 'wl-tx-in' : 'wl-tx-out'}">${isIn ? 'IN' : 'OUT'}</span>
          <div class="wl-tx-mid">
            <div class="wl-tx-amt">${fmtAmount(amtNum)} ${esc(t.symbol || '?')}</div>
            <div class="wl-tx-hash" title="${esc(t.hash || '')}">${shortHash(t.hash)}</div>
          </div>
          <div class="wl-tx-ts">${esc(tsFmt)}</div>
        </div>`;
    }).join('');
  } catch (e) {
    cancelWatchdog();
    el.innerHTML = `<div class="wl-err">Error: ${esc(e.message)}</div>`;
  }
}
