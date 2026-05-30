import { esc, fetchJSON, loadStyle } from '../js/utils.js';

// Mr.Flow Scanner — Section 25 SGVP auditor dashboard.
// Trigger scan + browse runs history + drill ke findings detail.
// Single-warga BY DESIGN — agent_id hardcoded ke 'mr-flow'.

const AGENT_ID = 'mr-flow';

const SEVERITY_ORDER = { critical: 0, high: 1, medium: 2, low: 3, info: 4 };

const CSS = `
.sc-root {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 100px);
  min-height: 560px;
  padding: 14px 18px;
  gap: 12px;
}

/* ── Top bar ── */
.sc-topbar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  background: rgba(15, 17, 26, 0.65);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  flex-shrink: 0;
}
.sc-title {
  font-family: var(--font-heading, 'Outfit', sans-serif);
  font-size: 1.05rem;
  color: #e2e8f0;
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0;
}
.sc-icon-glow {
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(239, 68, 68, 0.22), rgba(245, 158, 11, 0.16));
  border-radius: 6px;
  border: 1px solid rgba(239, 68, 68, 0.34);
}
.sc-agent-tag {
  margin-left: auto;
  font-family: ui-monospace, monospace;
  font-size: 0.74rem;
  color: #c4b5fd;
  background: rgba(139, 92, 246, 0.14);
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid rgba(139, 92, 246, 0.26);
}

/* ── Action bar ── */
.sc-action {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 12px 16px;
  background: rgba(15, 17, 26, 0.65);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  flex-shrink: 0;
  flex-wrap: wrap;
}
.sc-action label {
  font-size: 0.78rem;
  color: #94a3b8;
}
.sc-input {
  background: rgba(15, 23, 42, 0.7);
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 7px;
  padding: 7px 12px;
  font: inherit;
  font-size: 0.84rem;
  color: #e2e8f0;
  flex: 1;
  min-width: 240px;
  font-family: ui-monospace, monospace;
}
.sc-input:focus {
  outline: none;
  border-color: #7c3aed;
  box-shadow: 0 0 0 3px rgba(124, 58, 237, 0.22);
}
.sc-btn {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.24), rgba(124, 58, 237, 0.16));
  border: 1px solid rgba(139, 92, 246, 0.42);
  color: #e2e8f0;
  padding: 7px 16px;
  border-radius: 8px;
  font-size: 0.84rem;
  cursor: pointer;
  font-family: inherit;
  font-weight: 500;
  transition: background 0.12s;
}
.sc-btn:hover { background: linear-gradient(135deg, rgba(139, 92, 246, 0.36), rgba(124, 58, 237, 0.24)); }
.sc-btn:active { transform: scale(0.97); }
.sc-btn[disabled] { opacity: 0.5; cursor: wait; }
.sc-btn.ghost {
  background: transparent;
  border: 1px solid rgba(148, 163, 184, 0.22);
  color: #94a3b8;
}
.sc-btn.ghost:hover { background: rgba(148, 163, 184, 0.10); color: #cbd5e1; }

/* ── Auditors strip ── */
.sc-auditors {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
  align-items: center;
}
.sc-auditor-chip {
  font-size: 0.7rem;
  font-family: ui-monospace, monospace;
  color: #c4b5fd;
  background: rgba(139, 92, 246, 0.12);
  border: 1px solid rgba(139, 92, 246, 0.26);
  padding: 3px 9px;
  border-radius: 5px;
}

/* ── Two-column layout ── */
.sc-layout {
  display: grid;
  grid-template-columns: 340px 1fr;
  gap: 12px;
  flex: 1;
  min-height: 0;
}
.sc-runs, .sc-findings {
  background: rgba(15, 17, 26, 0.65);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  min-width: 0;
}
.sc-pane-head {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.10);
}
.sc-pane-head h3 {
  margin: 0;
  font-size: 0.92rem;
  color: #e2e8f0;
  font-family: var(--font-heading, 'Outfit', sans-serif);
  display: flex;
  align-items: center;
  gap: 8px;
}
.sc-pane-sub {
  font-size: 0.72rem;
  color: #94a3b8;
  margin-left: auto;
  font-family: ui-monospace, monospace;
}

.sc-list {
  flex: 1;
  overflow-y: auto;
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* ── Run row ── */
.sc-run {
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.08);
  border-radius: 9px;
  padding: 10px 12px;
  cursor: pointer;
  transition: background 0.12s, border-color 0.12s;
  font-size: 0.82rem;
}
.sc-run:hover {
  background: rgba(15, 23, 42, 0.62);
  border-color: rgba(139, 92, 246, 0.22);
}
.sc-run.active {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.20), rgba(124, 58, 237, 0.08));
  border-color: rgba(139, 92, 246, 0.38);
  box-shadow: inset 3px 0 0 #8b5cf6;
}
.sc-run-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
  flex-wrap: wrap;
}
.sc-run-path {
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
  color: #93c5fd;
  margin-bottom: 4px;
  word-break: break-all;
}
.sc-run-meta {
  display: flex;
  gap: 8px;
  font-size: 0.7rem;
  color: #64748b;
  font-family: ui-monospace, monospace;
  flex-wrap: wrap;
}
.sc-sev-counts {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-top: 4px;
}
.sc-sev-badge {
  font-size: 0.64rem;
  text-transform: uppercase;
  font-family: ui-monospace, monospace;
  padding: 2px 6px;
  border-radius: 4px;
  letter-spacing: 0.04em;
  font-weight: 600;
}
.sc-sev-critical { background: rgba(220, 38, 38, 0.30);  color: #fca5a5; border: 1px solid rgba(220, 38, 38, 0.50); }
.sc-sev-high     { background: rgba(239, 68, 68, 0.22);  color: #fca5a5; }
.sc-sev-medium   { background: rgba(245, 158, 11, 0.22); color: #fcd34d; }
.sc-sev-low      { background: rgba(59, 130, 246, 0.22); color: #93c5fd; }
.sc-sev-info     { background: rgba(148, 163, 184, 0.18); color: #cbd5e1; }

.sc-tag {
  font-size: 0.64rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(139, 92, 246, 0.16);
  color: #c4b5fd;
  font-family: ui-monospace, monospace;
  font-weight: 600;
}
.sc-tag.ok   { background: rgba(16, 185, 129, 0.18); color: #6ee7b7; }
.sc-tag.warn { background: rgba(245, 158, 11, 0.20); color: #fcd34d; }
.sc-tag.err  { background: rgba(239, 68, 68, 0.22);  color: #fca5a5; }

/* ── Finding card ── */
.sc-finding {
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.10);
  border-radius: 9px;
  padding: 12px 14px;
}
.sc-finding-head {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
}
.sc-finding-msg {
  color: #e2e8f0;
  font-size: 0.92rem;
  font-weight: 600;
  margin-bottom: 6px;
  line-height: 1.4;
}
.sc-finding-file {
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
  color: #93c5fd;
  margin-bottom: 8px;
  word-break: break-all;
}
.sc-finding-file .sc-line {
  background: rgba(59, 130, 246, 0.16);
  color: #93c5fd;
  padding: 1px 6px;
  border-radius: 4px;
  margin-left: 4px;
}
.sc-snippet {
  background: rgba(2, 6, 23, 0.72);
  border: 1px solid rgba(148, 163, 184, 0.10);
  border-radius: 6px;
  padding: 10px 12px;
  font-family: ui-monospace, monospace;
  font-size: 0.78rem;
  color: #fca5a5;
  margin-bottom: 8px;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.5;
}
.sc-remediation {
  display: flex;
  gap: 8px;
  padding: 9px 12px;
  background: rgba(16, 185, 129, 0.10);
  border: 1px solid rgba(16, 185, 129, 0.25);
  border-radius: 6px;
  font-size: 0.82rem;
  color: #6ee7b7;
  line-height: 1.5;
}
.sc-remediation::before { content: '🔧'; flex-shrink: 0; }

/* ── States ── */
.sc-empty, .sc-loading, .sc-err {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 40px 20px;
  color: #94a3b8;
  font-size: 0.84rem;
  text-align: center;
}
.sc-empty-icon { font-size: 2rem; margin-bottom: 10px; opacity: 0.5; }
.sc-loading::before {
  content: '';
  width: 13px; height: 13px;
  border: 2px solid rgba(139, 92, 246, 0.32);
  border-top-color: #c4b5fd;
  border-radius: 50%;
  margin-right: 10px;
  animation: sc-spin 0.9s linear infinite;
}
@keyframes sc-spin { to { transform: rotate(360deg); } }
.sc-err {
  background: rgba(239, 68, 68, 0.08);
  border: 1px solid rgba(239, 68, 68, 0.26);
  color: #fca5a5;
  border-radius: 9px;
  padding: 14px 18px;
  margin: 14px;
  flex-direction: row;
}

@media (max-width: 880px) {
  .sc-layout { grid-template-columns: 1fr; grid-template-rows: minmax(180px, 30vh) 1fr; }
}
`;

function fmtTime(s) {
  if (!s) return '—';
  try { return new Date(s).toLocaleString('id-ID', { hour12: false, dateStyle: 'short', timeStyle: 'short' }); }
  catch { return s; }
}

function statusClass(s) {
  const v = (s || '').toLowerCase();
  if (v === 'pass' || v === 'success') return 'ok';
  if (v === 'fail' || v === 'error')    return 'err';
  if (v === 'partial' || v === 'pending') return 'warn';
  return '';
}

function renderRun(run, activeRunId) {
  const isActive = run.id === activeRunId;
  const sev = aggregateSeverity(run);
  const finished = run.finished_at ? fmtTime(run.finished_at) : '(in progress)';
  return `
    <div class="sc-run${isActive ? ' active' : ''}" data-run-id="${run.id}">
      <div class="sc-run-head">
        <span class="sc-tag ${statusClass(run.status)}">#${run.id} · ${esc(run.status || 'pending')}</span>
        <span class="sc-tag" style="background:rgba(148,163,184,0.14);color:#94a3b8">${esc(run.scan_type || 'manual')}</span>
      </div>
      <div class="sc-run-path">${esc(run.target_path || '—')}</div>
      <div class="sc-run-meta">
        <span>${esc(String(run.total_findings || 0))} findings</span>
        <span>·</span>
        <span>${finished}</span>
      </div>
      <div class="sc-sev-counts">${sev}</div>
    </div>`;
}

function aggregateSeverity(run) {
  const parts = [];
  if (run.critical_count) parts.push(`<span class="sc-sev-badge sc-sev-critical">${run.critical_count} crit</span>`);
  if (run.high_count)     parts.push(`<span class="sc-sev-badge sc-sev-high">${run.high_count} high</span>`);
  if (run.medium_count)   parts.push(`<span class="sc-sev-badge sc-sev-medium">${run.medium_count} med</span>`);
  if (run.low_count)      parts.push(`<span class="sc-sev-badge sc-sev-low">${run.low_count} low</span>`);
  return parts.join('');
}

function renderFinding(f) {
  const sev = (f.severity || 'info').toLowerCase();
  return `
    <div class="sc-finding">
      <div class="sc-finding-head">
        <span class="sc-sev-badge sc-sev-${sev}">${esc(sev)}</span>
        <span class="sc-tag" style="background:rgba(148,163,184,0.14);color:#94a3b8">${esc(f.auditor || '?')}</span>
      </div>
      <div class="sc-finding-msg">${esc(f.message || '—')}</div>
      <div class="sc-finding-file">${esc(f.file_path || '?')}<span class="sc-line">L${esc(String(f.line_number || 0))}</span></div>
      ${f.snippet ? `<div class="sc-snippet">${esc(f.snippet)}</div>` : ''}
      ${f.remediation ? `<div class="sc-remediation">${esc(f.remediation)}</div>` : ''}
    </div>`;
}

function sortFindingsBySeverity(items) {
  return items.slice().sort((a, b) => {
    const sa = SEVERITY_ORDER[(a.severity || 'info').toLowerCase()] ?? 99;
    const sb = SEVERITY_ORDER[(b.severity || 'info').toLowerCase()] ?? 99;
    if (sa !== sb) return sa - sb;
    return (a.file_path || '').localeCompare(b.file_path || '');
  });
}

export async function render(root) {
  loadStyle('scanner', CSS);

  root.innerHTML = `
    <div class="sc-root">
      <header class="sc-topbar">
        <h2 class="sc-title"><span class="sc-icon-glow">🔍</span> Mr.Flow Scanner</h2>
        <span class="sc-agent-tag">agent=${esc(AGENT_ID)}</span>
      </header>

      <div class="sc-action">
        <label>Target path:</label>
        <input class="sc-input" id="scTarget" type="text" placeholder="mr-flow/tools/bad_example.go" value="mr-flow/tools/bad_example.go">
        <button class="sc-btn" id="scScan">▶ Run Scan</button>
        <button class="sc-btn ghost" id="scRefresh">⟳ Refresh</button>
        <div class="sc-auditors" id="scAuditors"></div>
      </div>

      <div class="sc-layout">
        <section class="sc-runs">
          <div class="sc-pane-head">
            <h3>📜 Runs</h3>
            <span class="sc-pane-sub" id="scRunCount">…</span>
          </div>
          <div class="sc-list" id="scRunsList"><div class="sc-loading">Loading runs…</div></div>
        </section>

        <section class="sc-findings">
          <div class="sc-pane-head">
            <h3 id="scFindingsTitle">🐛 Findings</h3>
            <span class="sc-pane-sub" id="scFindingsSub">Pilih run di kiri</span>
          </div>
          <div class="sc-list" id="scFindingsList">
            <div class="sc-empty">
              <div class="sc-empty-icon">🛡️</div>
              <div>Klik salah satu run untuk lihat detail findings.</div>
            </div>
          </div>
        </section>
      </div>
    </div>
  `;

  let activeRunId = null;
  let runsCache = [];

  async function loadAuditors() {
    const stripEl = root.querySelector('#scAuditors');
    try {
      const data = await fetchJSON(`/api/agents/scanner/auditors?id=${encodeURIComponent(AGENT_ID)}`);
      const items = Array.isArray(data.items) ? data.items : [];
      stripEl.innerHTML = items.length === 0
        ? `<span style="color:#64748b;font-size:0.72rem">no auditors</span>`
        : items.map((a) => `<span class="sc-auditor-chip">${esc(a)}</span>`).join('');
    } catch (err) {
      stripEl.innerHTML = `<span style="color:#fca5a5;font-size:0.72rem">auditor load err: ${esc(err.message)}</span>`;
    }
  }

  async function loadRuns(selectFirst = true) {
    const listEl = root.querySelector('#scRunsList');
    const countEl = root.querySelector('#scRunCount');
    listEl.innerHTML = `<div class="sc-loading">Loading runs…</div>`;
    try {
      const data = await fetchJSON(`/api/agents/scanner/runs?id=${encodeURIComponent(AGENT_ID)}&limit=50`);
      runsCache = Array.isArray(data.items) ? data.items : [];
      countEl.textContent = `${runsCache.length} runs`;
      if (runsCache.length === 0) {
        listEl.innerHTML = `<div class="sc-empty"><div class="sc-empty-icon">📜</div><div>Belum ada scan run. Klik "Run Scan" buat trigger pertama.</div></div>`;
        return;
      }
      paintRuns();
      if (selectFirst && runsCache.length > 0) {
        selectRun(runsCache[0].id);
      }
    } catch (err) {
      countEl.textContent = 'ERR';
      listEl.innerHTML = `<div class="sc-err">Gagal load runs: ${esc(err.message)}</div>`;
    }
  }

  function paintRuns() {
    const listEl = root.querySelector('#scRunsList');
    listEl.innerHTML = runsCache.map((r) => renderRun(r, activeRunId)).join('');
    listEl.querySelectorAll('.sc-run').forEach((el) => {
      el.onclick = () => selectRun(Number(el.dataset.runId));
    });
  }

  async function selectRun(runId) {
    activeRunId = runId;
    paintRuns();
    const titleEl = root.querySelector('#scFindingsTitle');
    const subEl = root.querySelector('#scFindingsSub');
    const listEl = root.querySelector('#scFindingsList');
    titleEl.innerHTML = `🐛 Findings · run #${runId}`;
    subEl.textContent = 'Loading…';
    listEl.innerHTML = `<div class="sc-loading">Loading findings…</div>`;
    try {
      const data = await fetchJSON(`/api/agents/scanner/findings?id=${encodeURIComponent(AGENT_ID)}&run_id=${runId}&limit=200`);
      const items = sortFindingsBySeverity(Array.isArray(data.items) ? data.items : []);
      subEl.textContent = `${items.length} findings`;
      if (items.length === 0) {
        listEl.innerHTML = `<div class="sc-empty"><div class="sc-empty-icon">✅</div><div>Clean — run #${runId} ngga ada finding.</div></div>`;
        return;
      }
      listEl.innerHTML = items.map(renderFinding).join('');
    } catch (err) {
      subEl.textContent = 'ERR';
      listEl.innerHTML = `<div class="sc-err">Gagal load findings: ${esc(err.message)}</div>`;
    }
  }

  async function runScan() {
    const targetEl = root.querySelector('#scTarget');
    const btnEl = root.querySelector('#scScan');
    const target = targetEl.value.trim();
    if (!target) {
      targetEl.focus();
      return;
    }
    btnEl.disabled = true;
    btnEl.textContent = '⏳ Scanning…';
    try {
      await fetchJSON(`/api/agents/scanner/scan?id=${encodeURIComponent(AGENT_ID)}`, {
        method: 'POST',
        body: JSON.stringify({ target_path: target, scan_type: 'manual' }),
      });
      await loadRuns(true);
    } catch (err) {
      alert(`Scan gagal: ${err.message || err}`);
    } finally {
      btnEl.disabled = false;
      btnEl.textContent = '▶ Run Scan';
    }
  }

  root.querySelector('#scScan').onclick = runScan;
  root.querySelector('#scRefresh').onclick = () => {
    loadAuditors();
    loadRuns(false);
  };
  root.querySelector('#scTarget').onkeydown = (e) => {
    if (e.key === 'Enter') runScan();
  };

  loadAuditors();
  loadRuns(true);
}
