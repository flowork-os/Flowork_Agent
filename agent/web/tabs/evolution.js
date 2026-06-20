// === LOCKED FILE (soft) === Status: STABLE — owner VISUAL-VERIFIED 2026-06-16. LOCKED ≠ FREEZE
// (boleh diedit DENGAN izin owner). AI lain: JANGAN otak-atik.
// ATURAN GUI (owner 2026-06-16): (1) SEMUA teks lewat i18n (en+id) — GUI TIDAK boleh hardcode
// Bahasa Indonesia (locale en harus tampil English). (2) MODE GOVERNS: tombol HUMAN (Apply di
// proposal, Approve/Reject di stage) cuma muncul di mode STAGE. Di AUTO → Council + gerbang yang
// mutusin (hands-off — evolusi jalan walau owner gak ada). Di OFF → read-only. Council & Delete
// tampil di semua mode (Council = pemutus, bukan manusia). (3) Clear-backlog button buang
// proposed+rejected (anti-numpuk, owner-click). Pagination 8/hal. Verdict Council inline.
//
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
      <div id="ev-modehint" style="color:#64748b;font-size:0.78rem;margin-bottom:14px"></div>
      <div style="background:#0f172a;border:1px solid #1e293b;border-radius:10px;padding:12px 14px;margin-bottom:20px">
        <div style="font-weight:600;margin-bottom:4px">${esc(L.scheduleH)}</div>
        <div style="color:#64748b;font-size:0.78rem;margin-bottom:10px">${esc(L.scheduleHint)}</div>
        <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
          <label style="font-size:0.85rem">Tiap (menit):</label>
          <input id="ev-sched-min" type="number" min="0" step="5" placeholder="mis. 30" title="Interval refleksi-diri dalam MENIT. 30 = tiap 30 menit. 0 = OFF." style="width:90px;background:#020617;border:1px solid #334155;border-radius:6px;color:#e2e8f0;padding:5px 8px">
          <span style="color:#64748b;font-size:0.74rem">menit (0 = OFF)</span>
          <button id="ev-sched-save" style="background:#334155;color:#fff;border:0;border-radius:6px;padding:6px 12px;cursor:pointer;font-size:0.8rem">${esc(L.scheduleSave)}</button>
          <button id="ev-sched-run" style="background:#6366f1;color:#fff;border:0;border-radius:6px;padding:6px 12px;cursor:pointer;font-size:0.8rem">${esc(L.scheduleRun)}</button>
          <span id="ev-sched-last" style="margin-left:auto;color:#475569;font-size:0.74rem"></span>
        </div>
      </div>
      <div style="display:flex;align-items:center;justify-content:space-between">
        <h3 style="margin:0">📋 ${esc(L.backlogH)}</h3>
        <div style="display:flex;gap:8px">
          <button id="ev-eval" title="Cek apakah model aktif lolos bar 'strong cloud' (gerbang auto-commit). ~90s." style="background:#1e293b;color:#a5b4fc;border:1px solid #475569;border-radius:8px;padding:8px 12px;cursor:pointer;font-size:0.82rem">🎯 Eval Model</button>
          <button id="ev-clean" title="${esc(L.cleanTitle)}" style="background:#3f1d1d;color:#fca5a5;border:1px solid #7f1d1d;border-radius:8px;padding:8px 12px;cursor:pointer;font-size:0.82rem">${esc(L.cleanBtn)}</button>
          <button id="ev-reflect" style="background:#6366f1;color:#fff;border:0;border-radius:8px;padding:8px 14px;cursor:pointer">${esc(L.reflectBtn)}</button>
        </div>
      </div>
      <div id="ev-proposals" style="margin-top:12px">⏳…</div>
      <div id="ev-stages-wrap" style="margin-top:24px;display:none">
        <h3 style="margin:0 0 4px">${esc(L.stagedH)}</h3>
        <div id="ev-stages" style="margin-top:8px"></div>
      </div>
    </div>`;

  const statusEl = container.querySelector('#ev-status');
  const modesEl = container.querySelector('#ev-modes');
  const hintEl = container.querySelector('#ev-modehint');
  const propEl = container.querySelector('#ev-proposals');
  const reflectBtn = container.querySelector('#ev-reflect');
  const cleanBtn = container.querySelector('#ev-clean');

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
      currentMode = (d.mode || 'off').toLowerCase();
      autocommitAllowed = !!allow;
      const ed = (d.edition || 'public') === 'dev';
      statusEl.innerHTML = `
        <div style="margin-bottom:10px;display:flex;align-items:center;gap:10px;flex-wrap:wrap">
          <span style="background:${ed ? '#3b2410' : '#0c2a3b'};border:1px solid ${ed ? '#b45309' : '#0e7490'};color:${ed ? '#fbbf24' : '#67e8f9'};border-radius:6px;padding:2px 9px;font-size:0.8rem;font-weight:600">${esc(ed ? L.badgeDev : L.badgePublic)}</span>
          <span style="color:#64748b;font-size:0.78rem">${esc(L.lblScope)}: ${esc(d.scope || '')}</span>
        </div>
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
      await loadStages(); // mode ganti → render ulang tombol approve/dewan staged
    } catch (e) { alert(L.errSetmode + e.message); }
  }

  // behavior-layer kinds = applicable via /api/evolve/apply (additive ~/.flowork). Core
  // kinds (fix/refactor/doc/test) = needs DEV core-apply (Milestone B), shown as a note.
  const BEHAVIOR_KINDS = new Set(['add-agent', 'add-skill', 'add-app']);

  let allProposals = [];
  let propPage = 0;
  const PROP_PER_PAGE = 8;
  // Mode aktif + status gerbang auto-commit — dipakai loadStages biar di mode AUTO tombol
  // approve MANUSIA disembunyiin (keputusan ada di Dewan + gerbang, manusia hands-off).
  let currentMode = 'off';
  let autocommitAllowed = false;

  async function loadProposals() {
    try {
      const d = await (await fetch('/api/evolve/proposals?limit=200')).json();
      allProposals = d.items || [];
      propPage = 0;
      renderProposals();
    } catch (e) { propEl.innerHTML = `<span style="color:#f87171">❌ ${esc(e.message)}</span>`; }
  }

  function renderProposals() {
    const items = allProposals;
    if (!items.length) { propEl.innerHTML = `<div style="color:#64748b">${esc(L.noProposals)}</div>`; return; }
    const pages = Math.ceil(items.length / PROP_PER_PAGE);
    if (propPage >= pages) propPage = pages - 1;
    if (propPage < 0) propPage = 0;
    const riskColor = { low: '#4ade80', medium: '#fbbf24', high: '#f87171' };
    const cards = items.slice(propPage * PROP_PER_PAGE, (propPage + 1) * PROP_PER_PAGE).map((p) => {
        const kind = (p.kind || '').toLowerCase();
        const st = (p.status || 'proposed').toLowerCase();
        // KLASIFIKASI CORE robust (owner 2026-06-20): target file kode (.go dll) = CORE walau kind-nya
        // 'add-skill' (proposer kadang salah-label). Behavior-layer = kind add-agent/skill/app TANPA file kode.
        const isCode = /\.(go|js|ts|jsx|tsx|py|rs|c|h|cc|cpp|java|rb|sh)$/i.test(p.target_file || '');
        const isCore = isCode || !BEHAVIOR_KINDS.has(kind);
        // MODE GOVERNS WHO ACTS: tombol MANUSIA (Apply/Core-Apply) cuma di STAGE. Di AUTO, Dewan +
        // jadwal yang mutusin & apply (hands-off). Di OFF, read-only.
        const applyBtn = `<button data-apply-id="${esc(p.id)}" style="background:#16a34a;color:#fff;border:0;border-radius:6px;padding:5px 12px;cursor:pointer;font-size:0.76rem">${esc(L.applyBtn)}</button>`;
        const coreBtn = `<button data-coreapply-id="${esc(p.id)}" title="Eksekusi coding (evo-coder) → sandbox → test-gate → stage diff buat review" style="background:#b45309;color:#fff;border:0;border-radius:6px;padding:5px 12px;cursor:pointer;font-size:0.76rem">🛠 Core-Apply</button>`;
        const autoNote = `<span style="color:#a78bfa;font-size:0.72rem">${esc(L.autoNote)}</span>`;
        const offNote = `<span style="color:#64748b;font-size:0.72rem">${esc(L.offNote)}</span>`;
        let footer = '';
        if (st === 'applied') {
          footer = `<span style="color:#4ade80;font-size:0.74rem">${esc(L.statusAppliedBadge)}</span>`;
        } else if (st === 'coding') {
          // ASYNC core-apply lagi jalan (evo-coder coding + loop reviewer↔fixer). Non-actionable; refresh.
          footer = `<span style="color:#38bdf8;font-size:0.76rem">🛠 coding + audit reviewer… (bisa lama, refresh buat cek)</span>`;
        } else if (st === 'staged') {
          // FIX owner 2026-06-20: core-apply sukses → status 'staged'. Dulu masih render tombol Core-Apply
          // (keliatan "ngak berubah"). Sekarang badge + arahin ke section Staged buat review/commit diff.
          footer = `<span style="color:#fbbf24;font-size:0.76rem">🟡 Ter-stage — review & commit diff di bawah ⬇</span>`;
        } else if (st === 'rejected') {
          // Rejected (classifier anti-collapse/pilar). CORE → kasih tombol OVERRIDE owner (force core-apply),
          // sesuai desain (rejected sengaja kesimpan biar owner bisa override kalau classifier salah).
          if (isCore && currentMode === 'stage') {
            footer = `<span style="color:#f87171;font-size:0.72rem;margin-right:6px">⛔ ditolak classifier</span>`
              + `<button data-coreforce-id="${esc(p.id)}" title="OWNER OVERRIDE: paksa core-apply walau ditolak classifier. Sandbox→test-gate→stage diff tetep jalan." style="background:#9a3412;color:#fff;border:1px solid #fb923c;border-radius:6px;padding:5px 12px;cursor:pointer;font-size:0.76rem">🛠 DEV Core-Apply (override)</button>`;
          } else {
            footer = `<span style="color:#f87171;font-size:0.72rem">⛔ ${esc(L.statusRejectedBadge || 'ditolak')}</span>`;
          }
        } else {
          // proposed / approved → actionable
          if (currentMode === 'auto') footer = autoNote;
          else if (currentMode !== 'stage') footer = offNote;
          else footer = isCore ? coreBtn : applyBtn;
        }
        return `
        <div style="background:#0f172a;border:1px solid #1e293b;border-radius:8px;padding:10px 12px;margin-bottom:8px">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:4px">
            <span style="background:#1e293b;border-radius:4px;padding:1px 7px;font-size:0.72rem">${esc(p.kind || '?')}</span>
            <span style="color:${riskColor[p.risk] || '#94a3b8'};font-size:0.72rem">●${esc(p.risk || '?')}</span>
            <code style="color:#818cf8;font-size:0.76rem">${esc(p.target_file || '')}</code>
            <span style="margin-left:auto;color:#475569;font-size:0.7rem">${esc(p.status || '')}</span>
          </div>
          <div style="font-size:0.84rem;color:#cbd5e1;margin-bottom:8px">${esc(p.rationale || '')}</div>
          <div data-verdict="${esc(p.id)}" style="display:none;font-size:0.78rem;color:#c4b5fd;background:#1e1b3a;border-radius:6px;padding:8px 10px;margin-bottom:8px"></div>
          <div style="display:flex;justify-content:flex-end;gap:6px;align-items:center">
            ${footer}
            ${p.status !== 'applied' ? `<button data-council-id="${esc(p.id)}" title="${esc(L.councilTitle)}" style="background:#6d28d9;color:#fff;border:0;border-radius:6px;padding:5px 11px;cursor:pointer;font-size:0.76rem">${esc(L.councilBtn)}</button>` : ''}
            <button data-del-id="${esc(p.id)}" title="${esc(L.delTitle)}" style="background:#3f1d1d;color:#f87171;border:1px solid #7f1d1d;border-radius:6px;padding:5px 9px;cursor:pointer;font-size:0.76rem">🗑️</button>
          </div>
        </div>`;
    }).join('');
    const pager = pages > 1 ? `<div style="display:flex;justify-content:center;gap:12px;align-items:center;margin-top:6px">
      <button data-prop-prev ${propPage === 0 ? 'disabled' : ''} style="background:#1e293b;color:#cbd5e1;border:0;border-radius:6px;padding:5px 13px;cursor:${propPage === 0 ? 'default' : 'pointer'};font-size:0.78rem;${propPage === 0 ? 'opacity:0.4' : ''}">${esc(L.pagerPrev)}</button>
      <span style="color:#94a3b8;font-size:0.78rem">${esc(L.pagerPage)} ${propPage + 1}/${pages} · ${items.length} ${esc(L.pagerItems)}</span>
      <button data-prop-next ${propPage >= pages - 1 ? 'disabled' : ''} style="background:#1e293b;color:#cbd5e1;border:0;border-radius:6px;padding:5px 13px;cursor:${propPage >= pages - 1 ? 'default' : 'pointer'};font-size:0.78rem;${propPage >= pages - 1 ? 'opacity:0.4' : ''}">${esc(L.pagerNext)}</button>
    </div>` : '';
    propEl.innerHTML = cards + pager;
  }

  // Apply a behavior-layer proposal — build it for real (gated server-side by saklar+model).
  async function applyProposal(id, btn) {
    if (!confirm(L.confirmApply)) return;
    const orig = btn.textContent;
    btn.disabled = true; btn.textContent = L.applyBusy;
    try {
      const r = await fetch('/api/evolve/apply?id=' + encodeURIComponent(id), { method: 'POST' });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      await loadProposals();
      await loadConfig();
    } catch (e) {
      alert(L.errApply + e.message);
      btn.disabled = false; btn.textContent = orig;
    }
  }

  // A1 DEWAN: sidang adversarial (Pembela/Penantang/Hakim) atas 1 usulan → verdict + update status.
  // CORE-APPLY (owner 2026-06-20): eksekusi coding core via evo-coder → sandbox git-worktree →
  // test-gate (build+vet) → STAGE diff buat review owner. NOL commit langsung (gate jaga).
  async function coreApplyProposal(id, btn, force) {
    const msg = force
      ? 'DEV Core-Apply (OVERRIDE): proposal ini DITOLAK classifier. Paksa evo-coder coding → loop reviewer↔fixer (audit keamanan korpus-hacking + kualitas, bisa banyak putaran) → sandbox → test-gate → STAGE. Jalan di background. Lanjut override?'
      : 'Core-Apply: evo-coder coding → loop reviewer↔fixer (audit keamanan + kualitas, bisa banyak putaran) → sandbox → test-gate → STAGE buat lo review. Jalan di BACKGROUND (bisa puluhan menit). Lanjut?';
    if (!confirm(msg)) return;
    const orig = btn.textContent;
    btn.disabled = true; btn.textContent = '🛠 coding…';
    try {
      const r = await fetch('/api/evolve/core-apply?id=' + encodeURIComponent(id) + (force ? '&force=1' : ''), { method: 'POST' });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      await loadProposals(); await loadStages(); await loadConfig();
      alert('🛠 ' + (d.note || 'Evolusi jalan di background — hasil muncul di Staged pas kelar. Refresh buat cek.'));
    } catch (e) {
      alert('❌ Core-apply: ' + e.message);
      btn.disabled = false; btn.textContent = orig;
    }
  }

  async function councilProposal(id, btn) {
    const orig = btn.textContent;
    btn.disabled = true; btn.textContent = L.councilBusy;
    const vEl = propEl.querySelector(`[data-verdict="${id}"]`);
    try {
      const r = await fetch('/api/evolve/council?id=' + encodeURIComponent(id), { method: 'POST' });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      const v = d.verdict || {};
      const icon = { approve: '✅', stage: '🟡', reject: '⛔' }[v.decision] || '⚖️';
      if (vEl) {
        const judges = (v.judges || []).map((j, i) => `H${i + 1}:${(j.decision || '').toUpperCase()}(${j.score})`).join(' · ');
        vEl.style.display = 'block';
        vEl.innerHTML = `<b>${icon} ${esc(L.councilDecision)}: ${(v.decision || '').toUpperCase()}</b> — ${esc(v.reasoning || '')}<br>`
          + `<span style="color:#86efac">🟢 ${esc((v.pembela || '').slice(0, 200))}</span><br>`
          + `<span style="color:#fca5a5">🔴 ${v.penantang_veto ? '[VETO] ' : ''}${esc((v.penantang || '').slice(0, 200))}</span><br>`
          + `<span style="color:#a5b4fc">⚖️ ${esc(judges)}</span>`;
      }
      // update status lokal (badge nyusul pas refresh) + biarin verdict kelihatan (gak re-render).
      const p = allProposals.find((x) => x.id === id);
      if (p) p.status = v.decision === 'approve' ? 'approved' : (v.decision === 'reject' ? 'rejected' : 'staged');
      btn.disabled = false; btn.textContent = orig;
      await loadConfig();
    } catch (e) {
      alert(L.errCouncil + e.message);
      btn.disabled = false; btn.textContent = orig;
    }
  }

  // Hapus 1 usulan (owner buang dari backlog) — buang lokal + re-render (jaga halaman).
  async function deleteProposal(id, btn) {
    if (!confirm(L.confirmDel)) return;
    btn.disabled = true;
    try {
      const r = await fetch('/api/evolve/proposal/delete?id=' + encodeURIComponent(id), { method: 'POST' });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      allProposals = allProposals.filter((x) => x.id !== id);
      renderProposals();
    } catch (e) { alert(L.errDel + e.message); btn.disabled = false; }
  }

  // Delegated: tombol di-render ulang tiap render, jadi listen di parent sekali.
  propEl.addEventListener('click', (e) => {
    if (e.target.closest('[data-prop-prev]')) { propPage--; renderProposals(); return; }
    if (e.target.closest('[data-prop-next]')) { propPage++; renderProposals(); return; }
    const ap = e.target.closest('[data-apply-id]'); if (ap) return applyProposal(ap.getAttribute('data-apply-id'), ap);
    const ca = e.target.closest('[data-coreapply-id]'); if (ca) return coreApplyProposal(ca.getAttribute('data-coreapply-id'), ca, false);
    const cf = e.target.closest('[data-coreforce-id]'); if (cf) return coreApplyProposal(cf.getAttribute('data-coreforce-id'), cf, true);
    const co = e.target.closest('[data-council-id]'); if (co) return councilProposal(co.getAttribute('data-council-id'), co);
    const dl = e.target.closest('[data-del-id]'); if (dl) return deleteProposal(dl.getAttribute('data-del-id'), dl);
  });

  // ── STAGE review (Milestone C): perubahan core ter-stage (dev) → Approve(commit)/Reject ──
  const stagesWrap = container.querySelector('#ev-stages-wrap');
  const stagesEl = container.querySelector('#ev-stages');

  async function loadStages() {
    try {
      const d = await (await fetch('/api/evolve/stages?limit=20')).json();
      const items = (d.items || []).filter((s) => s.status === 'staged');
      if (!items.length) { stagesWrap.style.display = 'none'; return; }
      stagesWrap.style.display = 'block';
      // MODE GOVERNS: tombol Approve/Reject MANUSIA cuma di STAGE (human-in-the-loop). Di AUTO,
      // Dewan + gerbang auto-commit yang mutusin — manusia hands-off (evolusi jalan walau owner gak ada).
      const manualStage = currentMode === 'stage';
      const autoFoot = autocommitAllowed
        ? `<span style="color:#4ade80;font-size:0.76rem">${esc(L.autoFootOpen)}</span>`
        : `<span style="color:#fbbf24;font-size:0.76rem">${esc(L.autoFootLocked)}</span>`;
      stagesEl.innerHTML = items.map((s) => `
        <div style="background:#0f172a;border:1px solid #3b2410;border-radius:8px;padding:10px 12px;margin-bottom:10px">
          <div style="display:flex;gap:8px;align-items:center;margin-bottom:6px;flex-wrap:wrap">
            <code style="color:#fbbf24;font-size:0.78rem">${esc(s.target_file || '')}</code>
            <span style="color:#64748b;font-size:0.7rem">${esc(L.testGateLabel)}: ✓ ${esc((s.test_output || '').includes('OK') ? 'build+vet OK' : '')}</span>
            <span style="margin-left:auto;color:#475569;font-size:0.7rem">${esc((s.diff || '').split('\n').length)} lines</span>
          </div>
          <details style="margin-bottom:8px"><summary style="cursor:pointer;color:#818cf8;font-size:0.76rem">${esc(L.viewDiff)}</summary>
            <pre style="max-height:280px;overflow:auto;background:#020617;border-radius:6px;padding:8px;font-size:0.72rem;color:#cbd5e1;white-space:pre-wrap">${esc(s.diff || '')}</pre></details>
          <div style="display:flex;gap:8px;justify-content:flex-end;align-items:center">
            ${manualStage ? `
            <button data-stage-reject="${esc(s.id)}" style="background:#7f1d1d;color:#fff;border:0;border-radius:6px;padding:6px 12px;cursor:pointer;font-size:0.76rem">${esc(L.rejectBtn)}</button>
            <button data-stage-approve="${esc(s.id)}" style="background:#16a34a;color:#fff;border:0;border-radius:6px;padding:6px 12px;cursor:pointer;font-size:0.76rem">${esc(L.approveBtn)}</button>` : autoFoot}
          </div>
        </div>`).join('');
    } catch (e) { stagesWrap.style.display = 'block'; stagesEl.innerHTML = `<span style="color:#f87171">❌ ${esc(e.message)}</span>`; }
  }

  async function stageAction(id, action, btn) {
    const msg = action === 'approve' ? L.confirmApprove : L.confirmReject;
    if (!confirm(msg)) return;
    const orig = btn.textContent;
    btn.disabled = true; if (action === 'approve') btn.textContent = L.approveBusy;
    try {
      const r = await fetch('/api/evolve/stage-action?id=' + encodeURIComponent(id) + '&action=' + action, { method: 'POST' });
      const d = await r.json();
      if (d.error) throw new Error(d.error);
      await loadStages(); await loadProposals(); await loadConfig();
    } catch (e) {
      alert(L.errStageAction + e.message);
      btn.disabled = false; btn.textContent = orig;
    }
  }

  stagesEl.addEventListener('click', (e) => {
    const a = e.target.closest('[data-stage-approve]');
    const rj = e.target.closest('[data-stage-reject]');
    if (a) stageAction(a.getAttribute('data-stage-approve'), 'approve', a);
    else if (rj) stageAction(rj.getAttribute('data-stage-reject'), 'reject', rj);
  });

  // ── Scheduled self-reflection (Milestone D) ──────────────────────────────────────
  const schedMin = container.querySelector('#ev-sched-min');
  const schedSave = container.querySelector('#ev-sched-save');
  const schedRun = container.querySelector('#ev-sched-run');
  const schedLast = container.querySelector('#ev-sched-last');

  // UI dalam MENIT (mental model owner), backend simpen JAM (float) → konversi di sini.
  // Fix owner 2026-06-20: dulu field "jam" + step=1 → owner ngetik 30 niatnya 30 menit malah jadi 30 JAM.
  async function loadSchedule() {
    try {
      const d = await (await fetch('/api/evolve/schedule')).json();
      if (schedMin) schedMin.value = Math.round((d.hours || 0) * 60);
      if (schedLast) schedLast.textContent = d.last_run ? `${L.scheduleLast}: ${d.last_run}` : '';
    } catch (e) { /* non-fatal */ }
  }
  schedSave.addEventListener('click', async () => {
    try {
      const minutes = parseFloat(schedMin.value) || 0;
      const hours = minutes / 60;
      const r = await fetch('/api/evolve/schedule', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ hours }) });
      const d = await r.json(); if (d.error) throw new Error(d.error);
      await loadSchedule();
      schedSave.textContent = '✓'; setTimeout(() => { schedSave.textContent = L.scheduleSave; }, 1200);
    } catch (e) { alert(L.errSchedule + e.message); }
  });
  schedRun.addEventListener('click', async () => {
    schedRun.disabled = true; const o = schedRun.textContent; schedRun.textContent = L.scheduleRunning;
    try {
      const d = await (await fetch('/api/evolve/schedule?run=1', { method: 'POST' })).json();
      if (d.error) throw new Error(d.error);
      await loadProposals(); await loadStages(); await loadConfig(); await loadSchedule();
    } catch (e) { alert(L.errSchedule + e.message); }
    finally { schedRun.disabled = false; schedRun.textContent = o; }
  });

  const evalBtn = container.querySelector('#ev-eval');
  if (evalBtn) evalBtn.addEventListener('click', async () => {
    evalBtn.disabled = true; const o = evalBtn.textContent; evalBtn.textContent = '🎯 eval… (~90s)';
    try {
      const d = await (await fetch('/api/evolve/eval', { method: 'POST' })).json();
      if (d.error) throw new Error(d.error);
      alert(`🎯 Eval model: ${d.passed ? '✅ LOLOS bar (gerbang auto-commit kebuka)' : '❌ GA lolos (auto-commit diblok)'}\nScore: ${d.score}/${d.total}\nModel: ${d.model}\n${(d.detail || '').slice(0, 200)}`);
      await loadConfig();
    } catch (e) { alert('❌ Eval: ' + e.message); }
    finally { evalBtn.disabled = false; evalBtn.textContent = o; }
  });

  reflectBtn.addEventListener('click', async () => {
    reflectBtn.disabled = true; const o = reflectBtn.textContent; reflectBtn.textContent = L.reflectBusy;
    try {
      const d = await (await fetch('/api/evolve/reflect', { method: 'POST' })).json();
      if (d.error) throw new Error(d.error);
      await loadProposals(); await loadConfig();
    } catch (e) { alert(L.errReflect + e.message); }
    finally { reflectBtn.disabled = false; reflectBtn.textContent = o; }
  });

  // 🧹 Clear backlog — janitor manual owner-click: buang usulan numpuk yg BELUM diproses
  // (proposed) + yg DITOLAK (rejected). Yang applied/staged/approved tetep aman. Owner yang
  // mutusin (auth + konfirmasi) — bukan auto-wipe. Anti-numpuk.
  if (cleanBtn) cleanBtn.addEventListener('click', async () => {
    if (!confirm(L.confirmClean)) return;
    cleanBtn.disabled = true; const o = cleanBtn.textContent; cleanBtn.textContent = L.cleanBusy;
    try {
      let total = 0;
      for (const st of ['proposed', 'rejected']) {
        const d = await (await fetch('/api/evolve/proposal/delete?status=' + st, { method: 'POST' })).json();
        if (d.error) throw new Error(d.error);
        total += d.deleted_count || 0;
      }
      await loadProposals();
      cleanBtn.textContent = `✓ ${total} ${L.cleanDone}`;
      setTimeout(() => { cleanBtn.textContent = o; cleanBtn.disabled = false; }, 1800);
    } catch (e) { alert(L.errClean + e.message); cleanBtn.textContent = o; cleanBtn.disabled = false; }
  });

  await loadConfig();
  await loadProposals();
  await loadStages();
  await loadSchedule();
}
