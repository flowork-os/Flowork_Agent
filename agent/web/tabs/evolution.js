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
// STYLING: clean glass-3D, full-width — pakai design-system fw-* share (js/glass.js).

import { t } from '/js/i18n.js';
import { ensureGlass } from '/js/glass.js';

// L: L.someKey → t('evolution.some_key') (camelCase → snake_case, pola codemap).
const L = new Proxy({}, { get: (_, k) => t('evolution.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });
const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

export async function render(container) {
  ensureGlass();
  container.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">🧬</span>
        <div>
          <h2 class="fw-title">${esc(L.title)}</h2>
          <div class="fw-sub">${esc(L.intro)}</div>
        </div>
      </div>
      <div class="fw-card" id="ev-status">⏳ ${esc(L.loading)}</div>
      <div class="fw-card">
        <div class="fw-row" id="ev-modes" style="margin-bottom:8px"></div>
        <div class="fw-desc" id="ev-modehint" style="margin-top:0"></div>
      </div>
      <div class="fw-card">
        <div class="fw-sec">${esc(L.scheduleH)}</div>
        <div class="fw-desc" style="margin-top:0;margin-bottom:10px">${esc(L.scheduleHint)}</div>
        <div class="fw-row">
          <label style="font-size:0.85rem">Tiap (menit):</label>
          <input id="ev-sched-min" type="number" min="0" step="5" placeholder="mis. 30" title="Interval refleksi-diri dalam MENIT. 30 = tiap 30 menit. 0 = OFF." class="fw-input" style="width:90px">
          <span class="fw-id">menit (0 = OFF)</span>
          <button id="ev-sched-save" class="fw-btn">${esc(L.scheduleSave)}</button>
          <button id="ev-sched-run" class="fw-btn">${esc(L.scheduleRun)}</button>
          <span class="fw-grow"></span>
          <span id="ev-sched-last" class="fw-id"></span>
        </div>
      </div>
      <div class="fw-row" style="margin-bottom:12px">
        <h3 style="margin:0;font-size:1rem;color:var(--text-main);font-weight:700">📋 ${esc(L.backlogH)}</h3>
        <span class="fw-grow"></span>
        <button id="ev-eval" title="Cek apakah model aktif lolos bar 'strong cloud' (gerbang auto-commit). ~90s." class="fw-btn">🎯 Eval Model</button>
        <button id="ev-clean" title="${esc(L.cleanTitle)}" class="fw-btn danger">${esc(L.cleanBtn)}</button>
        <button id="ev-reflect" class="fw-btn">${esc(L.reflectBtn)}</button>
      </div>
      <div id="ev-proposals">⏳…</div>
      <div id="ev-stages-wrap" style="margin-top:24px;display:none">
        <div class="fw-sec">${esc(L.stagedH)}</div>
        <div id="ev-stages"></div>
      </div>
      <div class="fw-card" id="ev-butuh-wrap" style="margin-top:24px;display:none">
        <div class="fw-row" style="margin-bottom:8px">
          <div class="fw-sec" style="margin:0">${esc(L.butuhH)}</div>
          <span class="fw-grow"></span>
          <button id="ev-butuh-clear" class="fw-btn danger">${esc(L.butuhClear)}</button>
        </div>
        <div class="fw-desc" style="margin-top:0;margin-bottom:10px">${esc(L.butuhHint)}</div>
        <div id="ev-butuh"></div>
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
        <div class="fw-row" style="margin-bottom:10px">
          <span class="fw-tag">${esc(ed ? L.badgeDev : L.badgePublic)}</span>
          <span class="fw-id">${esc(L.lblScope)}: ${esc(d.scope || '')}</span>
        </div>
        <div class="fw-row" style="gap:24px;font-size:0.85rem;color:var(--text-main)">
          <div>${esc(L.lblActiveMode)}: <b style="font-size:1.05rem">${esc((d.mode || 'off').toUpperCase())}</b></div>
          <div>${esc(L.lblKarmaReady)}: ${yn(k.ready)} <span class="fw-id">(${Math.round(k.reflect_ok || 0)}/${k.threshold || 20} ${esc(L.suffixSuccess)})</span></div>
          <div>${esc(L.lblModelStrong)}: ${yn(m.strong)} <span class="fw-id">${esc(m.note || '')}</span></div>
        </div>
        <div style="margin-top:10px;padding:8px 12px;border-radius:10px;border:1px solid var(--glass-border);background:var(--bg-panel-hover)">
          ${esc(L.lblAutocommit)}: <b style="color:${allow ? '#4ade80' : '#fbbf24'}">${allow ? esc(L.autocommitOn) : esc(L.autocommitLocked)}</b>${allow ? '' : `<span class="fw-id"> ${esc(L.autocommitNeed)}</span>`}
        </div>`;
      modesEl.innerHTML = '';
      MODES.forEach((mo) => {
        const active = (d.mode || 'off') === mo.k;
        const b = document.createElement('button');
        b.textContent = mo.label();
        b.className = 'fw-btn';
        b.style.cssText = `flex:1;padding:12px;font-size:0.95rem;${active ? 'border-color:var(--accent);box-shadow:0 0 0 1px var(--accent-glow)' : ''}`;
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
    if (!items.length) { propEl.innerHTML = `<div class="fw-empty">${esc(L.noProposals)}</div>`; return; }
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
        const applyBtn = `<button data-apply-id="${esc(p.id)}" class="fw-btn">${esc(L.applyBtn)}</button>`;
        const coreBtn = `<button data-coreapply-id="${esc(p.id)}" title="Eksekusi coding (evo-coder) → sandbox → test-gate → stage diff buat review" class="fw-btn">🛠 Core-Apply</button>`;
        const autoNote = `<span style="color:#a78bfa;font-size:0.72rem">${esc(L.autoNote)}</span>`;
        const offNote = `<span class="fw-id">${esc(L.offNote)}</span>`;
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
              + `<button data-coreforce-id="${esc(p.id)}" title="OWNER OVERRIDE: paksa core-apply walau ditolak classifier. Sandbox→test-gate→stage diff tetep jalan." class="fw-btn danger">🛠 DEV Core-Apply (override)</button>`;
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
        <div class="fw-card">
          <div class="fw-row" style="margin-bottom:4px">
            <span class="fw-tag">${esc(p.kind || '?')}</span>
            <span style="color:${riskColor[p.risk] || '#94a3b8'};font-size:0.72rem">●${esc(p.risk || '?')}</span>
            <code class="fw-id" style="color:#818cf8">${esc(p.target_file || '')}</code>
            <span class="fw-grow"></span>
            <span class="fw-id" style="opacity:.7">${esc(p.status || '')}</span>
          </div>
          <div class="fw-desc" style="margin-top:0;margin-bottom:8px;color:var(--text-main)">${esc(p.rationale || '')}</div>
          <div data-verdict="${esc(p.id)}" style="display:none;font-size:0.78rem;color:#c4b5fd;background:var(--bg-panel-hover);border:1px solid var(--glass-border);border-radius:8px;padding:8px 10px;margin-bottom:8px"></div>
          <div class="fw-row" style="justify-content:flex-end;gap:6px">
            ${footer}
            ${p.status !== 'applied' ? `<button data-council-id="${esc(p.id)}" title="${esc(L.councilTitle)}" class="fw-btn">${esc(L.councilBtn)}</button>` : ''}
            <button data-del-id="${esc(p.id)}" title="${esc(L.delTitle)}" class="fw-btn danger">🗑️</button>
          </div>
        </div>`;
    }).join('');
    const pager = pages > 1 ? `<div class="fw-row" style="justify-content:center;gap:12px;margin-top:6px">
      <button data-prop-prev ${propPage === 0 ? 'disabled' : ''} class="fw-btn" style="${propPage === 0 ? 'opacity:0.4;cursor:default' : ''}">${esc(L.pagerPrev)}</button>
      <span class="fw-id">${esc(L.pagerPage)} ${propPage + 1}/${pages} · ${items.length} ${esc(L.pagerItems)}</span>
      <button data-prop-next ${propPage >= pages - 1 ? 'disabled' : ''} class="fw-btn" style="${propPage >= pages - 1 ? 'opacity:0.4;cursor:default' : ''}">${esc(L.pagerNext)}</button>
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
        <div class="fw-card">
          <div class="fw-row" style="margin-bottom:6px">
            <code class="fw-id" style="color:#fbbf24">${esc(s.target_file || '')}</code>
            <span class="fw-id">${esc(L.testGateLabel)}: ✓ ${esc((s.test_output || '').includes('OK') ? 'build+vet OK' : '')}</span>
            <span class="fw-grow"></span>
            <span class="fw-id" style="opacity:.7">${esc((s.diff || '').split('\n').length)} lines</span>
          </div>
          <details style="margin-bottom:8px"><summary style="cursor:pointer;color:#818cf8;font-size:0.76rem">${esc(L.viewDiff)}</summary>
            <pre style="max-height:280px;overflow:auto;background:var(--bg-panel-hover);border:1px solid var(--glass-border);border-radius:8px;padding:8px;font-size:0.72rem;color:var(--text-main);white-space:pre-wrap">${esc(s.diff || '')}</pre></details>
          <div class="fw-row" style="justify-content:flex-end;gap:8px">
            ${manualStage ? `
            <button data-stage-reject="${esc(s.id)}" class="fw-btn danger">${esc(L.rejectBtn)}</button>
            <button data-stage-approve="${esc(s.id)}" class="fw-btn">${esc(L.approveBtn)}</button>` : autoFoot}
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

  // F4: antrian "butuh_tombol" — AI mentok (usulan di luar ruang saraf) → lapor ke sini,
  // BUKAN bongkar inti. Owner baca + nambah saklar (jarang, sadar) + kosongin antrian.
  async function loadButuhTombol() {
    const wrap = container.querySelector('#ev-butuh-wrap');
    const el = container.querySelector('#ev-butuh');
    if (!wrap || !el) return;
    try {
      const d = await (await fetch('/api/evolve/butuh-tombol')).json();
      const items = d.items || [];
      if (!items.length) { wrap.style.display = 'none'; return; }
      wrap.style.display = 'block';
      el.innerHTML = items.map((b) => `
        <div style="font-size:0.82rem;color:var(--text-main);background:var(--bg-panel-hover);border:1px solid var(--glass-border);border-radius:8px;padding:8px 10px;margin-bottom:6px">
          <b>${esc(L.butuhLoc)}:</b> ${esc(b.lokasi || '-')} <span class="fw-id">[${esc(b.kind || '')}→${esc(b.channel || '')}]</span><br>
          ${esc(b.alasan || '')}<span class="fw-id"> · ${esc((b.created_at || '').slice(0, 16))}</span>
        </div>`).join('');
    } catch { wrap.style.display = 'none'; }
  }
  const butuhClear = container.querySelector('#ev-butuh-clear');
  if (butuhClear) butuhClear.addEventListener('click', async () => {
    butuhClear.disabled = true;
    try { await fetch('/api/evolve/butuh-tombol', { method: 'DELETE' }); await loadButuhTombol(); }
    catch (e) { alert(esc(e.message)); }
    finally { butuhClear.disabled = false; }
  });

  await loadConfig();
  await loadProposals();
  await loadStages();
  await loadSchedule();
  await loadButuhTombol();
}
