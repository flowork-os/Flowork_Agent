import { esc, fetchJSON, loadStyle } from '../js/utils.js';

// Mr.Flow Diagnostics — single-warga dashboard yang nampilin SEMUA data
// agent-scoped: interactions (chat log), decisions (audit), mistakes
// (journal), karma metrics, death-letter (wasiat warga), workspace meta
// (resource catalog), tool audit, slash invocations. By design Mr.Flow
// adalah plug-and-play single-warga — jadi semua section di sini scope
// ke agent ID default ('mr-flow'). Multi-warga view (karma cross-agent,
// mesh topology) defer ke kalau warga lain spawn.

const AGENT_ID = 'mr-flow';

const CSS = `
.dg-shell {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(420px, 1fr));
  gap: 14px;
  padding: 14px;
}
.dg-card {
  background: rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  backdrop-filter: blur(14px);
  padding: 16px;
  min-height: 220px;
  max-height: 460px;
  display: flex;
  flex-direction: column;
  position: relative;
  overflow: hidden;
}
.dg-card::before {
  content: '';
  position: absolute;
  top: -40%; right: -40%;
  width: 70%; height: 70%;
  background: radial-gradient(circle, rgba(139,92,246,0.10), transparent 70%);
  pointer-events: none;
}
.dg-card h3 {
  font-family: var(--font-heading, 'Outfit', sans-serif);
  font-size: 1.05rem;
  margin: 0 0 4px;
  color: #f1f5f9;
  display: flex;
  align-items: center;
  gap: 8px;
}
.dg-card .dg-sub {
  font-size: 0.72rem;
  color: var(--text-muted, #94a3b8);
  margin-bottom: 12px;
  letter-spacing: 0.02em;
}
.dg-card .dg-count {
  margin-left: auto;
  font-size: 0.72rem;
  color: #c4b5fd;
  font-family: monospace;
}
.dg-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-right: 4px;
}
.dg-row {
  padding: 9px 12px;
  background: rgba(15, 23, 42, 0.5);
  border: 1px solid rgba(148, 163, 184, 0.08);
  border-radius: 10px;
  font-size: 0.82rem;
  line-height: 1.45;
}
.dg-row .dg-row-head {
  display: flex;
  gap: 10px;
  align-items: baseline;
  margin-bottom: 4px;
}
.dg-row .dg-tag {
  font-size: 0.66rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  padding: 1px 7px;
  border-radius: 4px;
  background: rgba(139, 92, 246, 0.18);
  color: #c4b5fd;
  font-family: monospace;
}
.dg-row .dg-time {
  font-size: 0.66rem;
  color: var(--text-muted, #94a3b8);
  font-family: monospace;
  margin-left: auto;
}
.dg-row .dg-body {
  color: #cbd5e1;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.dg-empty {
  color: var(--text-muted, #94a3b8);
  font-size: 0.82rem;
  text-align: center;
  padding: 24px 12px;
  font-style: italic;
}
.dg-err {
  color: #fca5a5;
  font-size: 0.82rem;
  padding: 10px;
  background: rgba(239, 68, 68, 0.10);
  border-radius: 8px;
}
.dg-metric {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 9px 12px;
  background: rgba(15, 23, 42, 0.5);
  border-radius: 10px;
  font-size: 0.82rem;
}
.dg-metric .dg-metric-k { color: #cbd5e1; font-family: monospace; }
.dg-metric .dg-metric-v { color: #c4b5fd; font-family: monospace; font-weight: 600; }
.dg-bar {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 12px;
}
.dg-bar button {
  background: rgba(139, 92, 246, 0.14);
  border: 1px solid rgba(139, 92, 246, 0.32);
  color: #c4b5fd;
  padding: 5px 12px;
  border-radius: 8px;
  font-size: 0.76rem;
  cursor: pointer;
}
.dg-bar button:hover { background: rgba(139, 92, 246, 0.26); }
.dg-shell h2 {
  grid-column: 1 / -1;
  font-family: var(--font-heading, 'Outfit', sans-serif);
  font-size: 1.4rem;
  color: #e2e8f0;
  margin: 4px 4px 0;
  display: flex;
  align-items: center;
  gap: 12px;
}
.dg-shell h2 .dg-pulse {
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 12px #10b981;
  animation: pulse 1.6s infinite ease-in-out;
}
@keyframes pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}
`;

function fmtTime(s) {
  if (!s) return '—';
  try { return new Date(s).toLocaleString('id-ID', { hour12: false }); }
  catch { return s; }
}

async function loadCard(cardEl, title, endpoint, opts) {
  const listEl = cardEl.querySelector('.dg-list');
  const countEl = cardEl.querySelector('.dg-count');
  try {
    const data = await fetchJSON(endpoint);
    const items = Array.isArray(data.items) ? data.items : [];
    countEl.textContent = `${items.length} entries`;
    if (items.length === 0) {
      listEl.innerHTML = `<div class="dg-empty">${esc(opts.empty || 'Belum ada data.')}</div>`;
      return;
    }
    listEl.innerHTML = items.map(opts.renderRow).join('');
  } catch (err) {
    countEl.textContent = 'ERR';
    listEl.innerHTML = `<div class="dg-err">${esc(err.message || String(err))}</div>`;
  }
}

function renderInteractionRow(it) {
  const dir = (it.direction || '').toLowerCase();
  const tag = dir === 'in' ? 'IN' : dir === 'out' ? 'OUT' : (it.channel || '?');
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(tag)}</span>
        <span style="font-size:0.72rem;color:#94a3b8">${esc(it.channel || '')} · ${esc(it.actor || '')}</span>
        <span class="dg-time">${esc(fmtTime(it.created_at || it.ts))}</span>
      </div>
      <div class="dg-body">${esc(it.content || '')}</div>
    </div>`;
}

function renderDecisionRow(d) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(d.verdict || '?')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">${esc(d.action || '')}</span>
        <span class="dg-time">${esc(fmtTime(d.created_at))}</span>
      </div>
      <div class="dg-body">${esc(d.reason || '')}</div>
    </div>`;
}

function renderMistakeRow(m) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(m.tier || 'raw')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">hit×${esc(String(m.hit_count || 0))} · ${esc(m.signature_hash || '').slice(0, 12)}</span>
        <span class="dg-time">${esc(fmtTime(m.last_seen_at))}</span>
      </div>
      <div class="dg-body">${esc(m.summary || m.description || '')}</div>
    </div>`;
}

function renderKarmaRow(k) {
  return `
    <div class="dg-metric">
      <span class="dg-metric-k">${esc(k.metric_key)}</span>
      <span class="dg-metric-v">${esc(String(k.metric_value))}${k.metric_count > 0 ? ` <span style="color:#64748b">×${esc(String(k.metric_count))}</span>` : ''}</span>
    </div>`;
}

function renderDeathLetterRow(d) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(d.letter_type || 'reflection')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">→ ${esc(d.recipient || 'all')}</span>
        <span class="dg-time">${esc(fmtTime(d.written_at))}</span>
      </div>
      <div style="font-weight:600;color:#e2e8f0;margin-bottom:3px">${esc(d.subject || '—')}</div>
      <div class="dg-body">${esc(d.body || '')}</div>
    </div>`;
}

function renderWorkspaceRow(w) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(w.category || 'misc')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">${esc(String(w.size_bytes || 0))} B${w.shareable ? ' · shared' : ''}</span>
        <span class="dg-time">${esc(fmtTime(w.updated_at))}</span>
      </div>
      <div style="font-weight:600;color:#e2e8f0;margin-bottom:3px;font-family:monospace;font-size:0.78rem">${esc(w.path || '—')}</div>
      <div class="dg-body">${esc(w.description || '')}</div>
    </div>`;
}

function renderToolAuditRow(a) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(a.tool || a.tool_name || '?')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">${esc(a.status || a.outcome || '')}</span>
        <span class="dg-time">${esc(fmtTime(a.created_at || a.ts))}</span>
      </div>
      <div class="dg-body">${esc(a.detail || a.summary || a.arg || '')}</div>
    </div>`;
}

function renderSlashRow(s) {
  return `
    <div class="dg-row">
      <div class="dg-row-head">
        <span class="dg-tag">${esc(s.cmd || s.command || '?')}</span>
        <span style="font-size:0.72rem;color:#94a3b8">${esc(s.outcome || s.status || '')}</span>
        <span class="dg-time">${esc(fmtTime(s.created_at || s.ts))}</span>
      </div>
      <div class="dg-body">${esc(s.result || s.detail || s.arg || '')}</div>
    </div>`;
}

function makeCard(icon, title, sub) {
  return `
    <section class="dg-card">
      <h3>${icon} <span>${esc(title)}</span><span class="dg-count">…</span></h3>
      <div class="dg-sub">${esc(sub)}</div>
      <div class="dg-list"><div class="dg-empty">Loading…</div></div>
    </section>`;
}

export async function render(root) {
  loadStyle('diagnostics', CSS);

  root.innerHTML = `
    <div class="dg-shell">
      <h2><span class="dg-pulse"></span> Mr.Flow Diagnostics <span style="font-size:0.72rem;color:#94a3b8;margin-left:auto;font-family:monospace">agent=${esc(AGENT_ID)}</span></h2>
      <div style="grid-column:1/-1" class="dg-bar">
        <button id="dgRefresh">⟳ Refresh All</button>
        <span style="font-size:0.72rem;color:#64748b">Klik tombol untuk reload semua section.</span>
      </div>
      <div id="dgInteractions">${makeCard('💬', 'Interactions', 'Chat log Telegram in/out · Section 1')}</div>
      <div id="dgDecisions">${makeCard('🧭', 'Decisions', 'Audit verdict + reason · Section 3')}</div>
      <div id="dgMistakes">${makeCard('📓', 'Mistakes Journal', 'Halu/error signatures + promote tier · Section 2/7')}</div>
      <div id="dgKarma">${makeCard('⚡', 'Karma Metrics', 'Per-metric counters + averages · Section 5')}</div>
      <div id="dgDeathLetter">${makeCard('🕯️', 'Death Letter', 'Wasiat warga AI · Section 4')}</div>
      <div id="dgWorkspace">${makeCard('📁', 'Workspace Meta', 'Resource catalog · Section 6')}</div>
      <div id="dgToolAudit">${makeCard('🔧', 'Tool Audit', 'Sandbox tool calls · Section 26')}</div>
      <div id="dgSlash">${makeCard('⚡', 'Slash Invocations', 'Slash command history · Section 13')}</div>
    </div>
  `;

  async function refreshAll() {
    const qs = `id=${encodeURIComponent(AGENT_ID)}&limit=50`;
    await Promise.all([
      loadCard(root.querySelector('#dgInteractions'), 'Interactions',
        `/api/agents/interactions?${qs}`, { renderRow: renderInteractionRow, empty: 'Belum ada chat.' }),
      loadCard(root.querySelector('#dgDecisions'), 'Decisions',
        `/api/agents/decisions?${qs}`, { renderRow: renderDecisionRow, empty: 'Belum ada decision log.' }),
      loadCard(root.querySelector('#dgMistakes'), 'Mistakes',
        `/api/agents/mistakes?${qs}`, { renderRow: renderMistakeRow, empty: 'Clean — no mistakes recorded.' }),
      loadCard(root.querySelector('#dgKarma'), 'Karma',
        `/api/agents/karma?${qs}`, { renderRow: renderKarmaRow, empty: 'Belum ada metric.' }),
      loadCard(root.querySelector('#dgDeathLetter'), 'Death Letter',
        `/api/agents/death-letter?${qs}`, { renderRow: renderDeathLetterRow, empty: 'Belum ada wasiat.' }),
      loadCard(root.querySelector('#dgWorkspace'), 'Workspace',
        `/api/agents/workspace-meta?${qs}`, { renderRow: renderWorkspaceRow, empty: 'Workspace catalog kosong.' }),
      loadCard(root.querySelector('#dgToolAudit'), 'Tool Audit',
        `/api/agents/tool-audit?${qs}`, { renderRow: renderToolAuditRow, empty: 'Belum ada tool call.' }),
      loadCard(root.querySelector('#dgSlash'), 'Slash',
        `/api/agents/slash-invocations?${qs}`, { renderRow: renderSlashRow, empty: 'Belum ada slash command.' }),
    ]);
  }

  root.querySelector('#dgRefresh').onclick = refreshAll;
  await refreshAll();
}
