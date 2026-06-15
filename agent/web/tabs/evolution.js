// evolution.js — R7 SELF-EVOLUTION control panel. Owner-approved 2026-06-15 (FASE 2).
// SAKLAR self-modify (OFF/STAGE/AUTO) + status gate berlapis + backlog usulan. KRUSIAL:
// owner pegang penuh. Auto-commit cuma jalan kalau mode=AUTO + karma matang + model cloud
// kuat (guard anti-LLM-lokal). SEMUA teks lewat i18n (t('evolution.*')) — en+id dict.

import { t } from '/js/i18n.js';

// L: L.someKey → t('evolution.some_key') (camelCase → snake_case, pola codemap).
const L = new Proxy({}, { get: (_, k) => t('evolution.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });
const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

export async function render(container) {
  container.innerHTML = `
    <div style="padding:18px 22px;max-width:920px;color:#e2e8f0">
      <h2 style="margin:0 0 4px">🧬 ${esc(L.title)}</h2>
      <p style="color:#94a3b8;margin:0 0 16px;font-size:0.88rem">${esc(L.intro)}</p>
      <div id="ev-status" style="background:#0f172a;border:1px solid #1e293b;border-radius:10px;padding:14px;margin-bottom:14px">⏳ ${esc(L.loading)}</div>
      <div id="ev-modes" style="display:flex;gap:10px;margin-bottom:8px"></div>
      <div id="ev-modehint" style="color:#64748b;font-size:0.78rem;margin-bottom:20px"></div>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <h3 style="margin:0">📋 ${esc(L.backlogH)}</h3>
        <button id="ev-reflect" style="background:#6366f1;color:#fff;border:0;border-radius:8px;padding:8px 14px;cursor:pointer">${esc(L.reflectBtn)}</button>
      </div>
      <div id="ev-proposals" style="margin-top:12px">⏳…</div>
    </div>`;

  const statusEl = container.querySelector('#ev-status');
  const modesEl = container.querySelector('#ev-modes');
  const hintEl = container.querySelector('#ev-modehint');
  const propEl = container.querySelector('#ev-proposals');
  const reflectBtn = container.querySelector('#ev-reflect');

  const MODES = [
    { k: 'off', label: () => L.modeOff, hint: () => L.hintOff },
    { k: 'stage', label: () => L.modeStage, hint: () => L.hintStage },
    { k: 'auto', label: () => L.modeAuto, hint: () => L.hintAuto },
  ];

  async function loadConfig() {
    try {
      const d = await (await fetch('/api/evolve/config')).json();
      if (d.error) throw new Error(d.error);
      const k = d.karma || {}, m = d.model || {};
      const yn = (b) => (b ? `<span style="color:#4ade80">${esc(L.valYes)}</span>` : `<span style="color:#f87171">${esc(L.valNo)}</span>`);
      const allow = d.autocommit_allowed;
      statusEl.innerHTML = `
        <div style="display:flex;gap:24px;flex-wrap:wrap;font-size:0.85rem">
          <div>${esc(L.lblActiveMode)}: <b style="font-size:1.05rem">${esc((d.mode || 'off').toUpperCase())}</b></div>
          <div>${esc(L.lblKarmaReady)}: ${yn(k.ready)} <span style="color:#64748b">(${Math.round(k.reflect_ok || 0)}/${k.threshold || 20} ${esc(L.suffixSuccess)})</span></div>
          <div>${esc(L.lblModelStrong)}: ${yn(m.strong)} <span style="color:#64748b">${esc(m.note || '')}</span></div>
        </div>
        <div style="margin-top:10px;padding:8px 12px;border-radius:8px;background:${allow ? '#052e16' : '#1e293b'};border:1px solid ${allow ? '#16a34a' : '#334155'}">
          ${esc(L.lblAutocommit)}: <b style="color:${allow ? '#4ade80' : '#fbbf24'}">${allow ? esc(L.autocommitOn) : esc(L.autocommitLocked)}</b>${allow ? '' : `<span style="color:#94a3b8;font-size:0.8rem">${esc(L.autocommitNeed)}</span>`}
        </div>`;
      modesEl.innerHTML = '';
      MODES.forEach((mo) => {
        const active = (d.mode || 'off') === mo.k;
        const b = document.createElement('button');
        b.textContent = mo.label();
        b.style.cssText = `flex:1;padding:12px;border-radius:10px;cursor:pointer;font-size:0.95rem;border:2px solid ${active ? '#6366f1' : '#334155'};background:${active ? '#1e1b4b' : '#0f172a'};color:#e2e8f0`;
        b.addEventListener('click', () => setMode(mo.k));
        modesEl.appendChild(b);
      });
      hintEl.textContent = (MODES.find((x) => x.k === (d.mode || 'off')) || { hint: () => '' }).hint();
    } catch (e) {
      statusEl.innerHTML = `<span style="color:#f87171">❌ ${esc(e.message)}</span>`;
    }
  }

  async function setMode(mode) {
    if (mode === 'auto' && !confirm(L.confirmAuto)) return;
    try {
      const r = await fetch('/api/evolve/config', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ mode }) });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      await loadConfig();
    } catch (e) { alert(L.errSetmode + e.message); }
  }

  async function loadProposals() {
    try {
      const d = await (await fetch('/api/evolve/proposals?limit=30')).json();
      const items = d.items || [];
      if (!items.length) { propEl.innerHTML = `<div style="color:#64748b">${esc(L.noProposals)}</div>`; return; }
      const riskColor = { low: '#4ade80', medium: '#fbbf24', high: '#f87171' };
      propEl.innerHTML = items.map((p) => `
        <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:10px 12px;margin-bottom:8px">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:4px">
            <span style="background:#1e293b;border-radius:4px;padding:1px 7px;font-size:0.72rem">${esc(p.kind || '?')}</span>
            <span style="color:${riskColor[p.risk] || '#94a3b8'};font-size:0.72rem">●${esc(p.risk || '?')}</span>
            <code style="color:#818cf8;font-size:0.76rem">${esc(p.target_file || '')}</code>
            <span style="margin-left:auto;color:#475569;font-size:0.7rem">${esc(p.status || '')}</span>
          </div>
          <div style="font-size:0.84rem;color:#cbd5e1">${esc(p.rationale || '')}</div>
        </div>`).join('');
    } catch (e) { propEl.innerHTML = `<span style="color:#f87171">❌ ${esc(e.message)}</span>`; }
  }

  reflectBtn.addEventListener('click', async () => {
    reflectBtn.disabled = true; const o = reflectBtn.textContent; reflectBtn.textContent = L.reflectBusy;
    try {
      const d = await (await fetch('/api/evolve/reflect', { method: 'POST' })).json();
      if (d.error) throw new Error(d.error);
      await loadProposals(); await loadConfig();
    } catch (e) { alert(L.errReflect + e.message); }
    finally { reflectBtn.disabled = false; reflectBtn.textContent = o; }
  });

  await loadConfig();
  await loadProposals();
}
