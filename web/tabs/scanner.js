// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Scanner tab — radar besar + scan log + findings (no manual target
//   input; full background-watch). Audit pass: esc() semua field, encodeURIComponent
//   run_id, auto-poll interval di-clear tiap render (no leak).
//
// scanner.js — Mr.Flow "Threat Radar". Layout: radar gede (kiri) + scan log
// stream & findings detail (kanan). Background codescan engine isi log otomatis.

import { esc, fetchJSON, loadStyle } from '../js/utils.js';

const AGENT_ID = 'mr-flow';
const SEV_ORDER = { critical: 0, high: 1, medium: 2, low: 3, info: 4 };
const SEV_COLOR = { critical: '#ff3b3b', high: '#ff8c1a', medium: '#ffd11a', low: '#22ff88', info: '#5eead4' };
const SEV_RADIUS = { critical: 0.30, high: 0.50, medium: 0.68, low: 0.84, info: 0.92 };

let pollTimer = null;
let selectedRun = null;

const CSS = `
.rx { --neon:#22ff88; --neon2:#5eead4; --line:rgba(34,255,136,0.16);
  font-family:'JetBrains Mono','SFMono-Regular',ui-monospace,Menlo,Consolas,monospace;
  color:#bdf5d6; position:relative; height:calc(100vh - 150px); min-height:560px;
  display:flex; flex-direction:column; }
.rx-top { display:flex; align-items:center; gap:14px; flex-wrap:wrap; margin-bottom:14px; flex:0 0 auto; }
.rx-title { font-size:1.2rem; font-weight:700; letter-spacing:.2em; text-transform:uppercase;
  color:var(--neon); text-shadow:0 0 14px rgba(34,255,136,0.5); margin:0; }
.rx-live { display:inline-flex; align-items:center; gap:7px; font-size:0.68rem; letter-spacing:.12em;
  padding:4px 12px; border:1px solid rgba(34,255,136,0.4); border-radius:20px; color:var(--neon);
  background:rgba(34,255,136,0.06); }
.rx-live .dot { width:8px;height:8px;border-radius:50%;background:var(--neon);
  box-shadow:0 0 8px var(--neon); animation:rx-blink 1.2s ease-in-out infinite; }
@keyframes rx-blink { 0%,100%{opacity:1} 50%{opacity:.2} }
.rx-spacer { flex:1; }
.rx-btn { padding:9px 16px; border-radius:8px; border:1px solid rgba(34,255,136,0.35); cursor:pointer;
  background:rgba(34,255,136,0.10); color:var(--neon); font-family:inherit; font-weight:600;
  font-size:0.78rem; letter-spacing:.06em; transition:all .15s; }
.rx-btn:hover { background:rgba(34,255,136,0.22); box-shadow:0 0 14px rgba(34,255,136,0.3); }

.rx-grid { display:grid; grid-template-columns: 460px 1fr; gap:18px; flex:1 1 auto; min-height:0; }
@media(max-width:980px){ .rx-grid{ grid-template-columns:1fr; } }

/* ── RADAR (kiri, gede) ── */
.rx-left { display:flex; flex-direction:column; gap:16px; align-items:center;
  padding:24px 20px; border:1px solid var(--line); border-radius:18px;
  background:radial-gradient(circle at 50% 38%, rgba(34,255,136,0.06), rgba(0,0,0,0.45)); }
.rx-radar { position:relative; width:400px; height:400px; max-width:100%; aspect-ratio:1; border-radius:50%;
  background:radial-gradient(circle, rgba(0,52,30,0.6), rgba(0,12,7,0.96));
  border:1px solid rgba(34,255,136,0.5);
  box-shadow:inset 0 0 60px rgba(34,255,136,0.12), 0 0 34px rgba(34,255,136,0.14); overflow:hidden; }
.rx-radar::before { content:''; position:absolute; inset:0; border-radius:50%;
  background:repeating-radial-gradient(circle, transparent 0 49px, rgba(34,255,136,0.13) 49px 50px); }
.rx-cross-v { position:absolute; left:50%; top:0; width:1px; height:100%; background:rgba(34,255,136,0.12); }
.rx-cross-h { position:absolute; left:0; top:50%; width:100%; height:1px; background:rgba(34,255,136,0.12); }
.rx-sweep { position:absolute; inset:0; border-radius:50%;
  background:conic-gradient(from 0deg, rgba(34,255,136,0) 0deg 300deg, rgba(34,255,136,0.08) 322deg, rgba(34,255,136,0.5) 358deg, rgba(34,255,136,0.72) 360deg);
  animation:rx-spin 3.4s linear infinite; }
@keyframes rx-spin { to { transform:rotate(360deg); } }
.rx-core { position:absolute; left:50%; top:50%; transform:translate(-50%,-50%); text-align:center; z-index:3; pointer-events:none; }
.rx-core .st { font-size:1.5rem; font-weight:700; letter-spacing:.14em; text-shadow:0 0 16px currentColor; }
.rx-core .ct { font-size:0.72rem; color:#7fb89c; margin-top:4px; letter-spacing:.1em; }
.rx-blip { position:absolute; width:12px; height:12px; border-radius:50%; transform:translate(-50%,-50%);
  z-index:2; box-shadow:0 0 12px currentColor; animation:rx-pulse 1.7s ease-out infinite; }
@keyframes rx-pulse { 0%{box-shadow:0 0 0 0 currentColor; opacity:1} 70%{box-shadow:0 0 0 12px transparent; opacity:.65} 100%{opacity:1} }

.rx-stats { display:flex; gap:12px; flex-wrap:wrap; justify-content:center; }
.rx-stat { text-align:center; padding:10px 18px; border:1px solid var(--line); border-radius:12px;
  background:rgba(0,0,0,0.3); min-width:78px; }
.rx-stat .n { font-size:1.7rem; font-weight:700; color:var(--neon); line-height:1; }
.rx-stat .l { font-size:0.6rem; letter-spacing:.12em; color:#7fb89c; text-transform:uppercase; margin-top:6px; }
.rx-aud { font-size:0.66rem; color:#5c8a73; letter-spacing:.05em; text-align:center; }

/* ── KANAN: log + findings ── */
.rx-right { display:flex; flex-direction:column; gap:16px; min-height:0; }
.rx-panel { border:1px solid var(--line); border-radius:14px; background:rgba(0,9,5,0.55);
  display:flex; flex-direction:column; min-height:0; overflow:hidden; }
.rx-panel.log { flex:0 0 42%; }
.rx-panel.find { flex:1 1 auto; }
.rx-phead { padding:11px 16px; border-bottom:1px solid var(--line); font-size:0.7rem; letter-spacing:.16em;
  text-transform:uppercase; color:var(--neon2); display:flex; align-items:center; gap:8px; flex:0 0 auto;
  background:linear-gradient(180deg, rgba(34,255,136,0.05), transparent); }
.rx-pbody { overflow-y:auto; flex:1 1 auto; min-height:0; }

.rx-log-row { display:flex; align-items:center; gap:9px; padding:9px 16px; cursor:pointer;
  border-bottom:1px solid rgba(34,255,136,0.06); font-size:0.74rem; transition:background .12s; }
.rx-log-row:hover { background:rgba(34,255,136,0.06); }
.rx-log-row.on { background:rgba(34,255,136,0.11); border-left:2px solid var(--neon); }
.rx-id { color:#5c8a73; min-width:34px; }
.rx-path { flex:1; color:#cfeede; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
.rx-hits { color:#7fb89c; font-size:0.68rem; }
.rx-tag { font-size:0.6rem; padding:2px 7px; border-radius:5px; letter-spacing:.04em; white-space:nowrap; }
.rx-tag.auto { background:rgba(94,234,212,0.14); color:#5eead4; }
.rx-tag.manual { background:rgba(148,163,184,0.12); color:#94a3b8; }
.rx-tag.pass { background:rgba(34,255,136,0.14); color:var(--neon); }
.rx-tag.fail { background:rgba(255,59,59,0.16); color:#ff8080; }
.rx-dot { width:7px;height:7px;border-radius:50%; flex:0 0 auto; box-shadow:0 0 6px currentColor; }

.rx-find { padding:13px 16px; border-bottom:1px solid rgba(34,255,136,0.06); }
.rx-find-h { display:flex; gap:8px; align-items:center; margin-bottom:6px; flex-wrap:wrap; }
.rx-sev { font-size:0.6rem; padding:2px 8px; border-radius:5px; font-weight:700; letter-spacing:.04em; }
.rx-find-msg { font-size:0.86rem; color:#e6fff2; line-height:1.45; }
.rx-find-file { font-size:0.7rem; color:#7fb89c; margin-top:5px; }
.rx-find-file .ln { color:var(--neon); margin-left:6px; }
.rx-snip { margin-top:8px; padding:8px 11px; background:rgba(0,0,0,0.5); border-left:2px solid rgba(255,59,59,0.4);
  font-size:0.72rem; color:#ffb3b3; white-space:pre-wrap; word-break:break-all; border-radius:0 6px 6px 0; }
.rx-rem { margin-top:6px; font-size:0.72rem; color:#9fe6c4; } .rx-rem b{ color:var(--neon); }
.rx-empty { padding:26px; text-align:center; color:#5c8a73; font-size:0.82rem; }
.rx-empty.ok { color:var(--neon); }
`;

export async function render(root) {
  loadStyle('scanner-radar', CSS);
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  selectedRun = null;

  root.innerHTML = `
    <div class="rx">
      <div class="rx-top">
        <h2 class="rx-title">⌖ Threat Radar</h2>
        <span class="rx-live"><span class="dot"></span>LIVE · BACKGROUND WATCH AKTIF</span>
        <span class="rx-spacer"></span>
        <span style="font-size:0.66rem;color:#5c8a73;letter-spacing:.06em">agent=${esc(AGENT_ID)}</span>
        <button class="rx-btn" id="rxRefresh">⟳ REFRESH</button>
      </div>
      <div class="rx-grid">
        <div class="rx-left">
          <div class="rx-radar" id="rxRadar">
            <div class="rx-cross-v"></div><div class="rx-cross-h"></div>
            <div class="rx-sweep"></div>
            <div class="rx-core" id="rxCore"><div class="st">—</div><div class="ct">init…</div></div>
          </div>
          <div class="rx-stats" id="rxStats"></div>
          <div class="rx-aud" id="rxAud">auditor: …</div>
        </div>
        <div class="rx-right">
          <div class="rx-panel log">
            <div class="rx-phead">▚ SCAN LOG <span style="color:#5c8a73;letter-spacing:0">· file kode yang ke-scan otomatis</span></div>
            <div class="rx-pbody" id="rxLog"><div class="rx-empty">loading…</div></div>
          </div>
          <div class="rx-panel find">
            <div class="rx-phead" id="rxFindHead">⚠ FINDINGS</div>
            <div class="rx-pbody" id="rxFindings"><div class="rx-empty">pilih entri di scan log buat lihat detail</div></div>
          </div>
        </div>
      </div>
    </div>`;

  root.querySelector('#rxRefresh').addEventListener('click', () => loadAll(root));
  await loadAll(root);
  pollTimer = setInterval(() => { loadRuns(root).catch(() => {}); }, 8000);
}

async function loadAll(root) {
  await Promise.all([loadAuditors(root), loadRuns(root)]);
}

async function loadAuditors(root) {
  try {
    const d = await fetchJSON(`/api/agents/scanner/auditors?id=${AGENT_ID}`);
    root.querySelector('#rxAud').textContent = `${(d.items || []).length} auditor aktif · auto-scan tiap file kode berubah`;
  } catch { /* ignore */ }
}

async function loadRuns(root) {
  const d = await fetchJSON(`/api/agents/scanner/runs?id=${AGENT_ID}&limit=60`);
  const runs = d.items || [];
  renderLog(root, runs);
  renderStats(root, {
    runs: runs.length,
    findings: runs.reduce((s, r) => s + (r.total_findings || 0), 0),
    critical: runs.reduce((s, r) => s + (r.critical_count || 0), 0),
  });
  if (!selectedRun && runs.length) {
    selectRun(root, runs[0]);
  } else if (selectedRun) {
    const cur = runs.find(r => r.id === selectedRun.id);
    if (cur) updateRadarFromRun(root, cur);
  } else {
    updateRadar(root, [], 'SECURE');
  }
}

function renderStats(root, s) {
  root.querySelector('#rxStats').innerHTML = `
    <div class="rx-stat"><div class="n">${s.runs}</div><div class="l">runs</div></div>
    <div class="rx-stat"><div class="n">${s.findings}</div><div class="l">findings</div></div>
    <div class="rx-stat"><div class="n" style="color:${s.critical ? '#ff3b3b' : '#22ff88'}">${s.critical}</div><div class="l">critical</div></div>`;
}

function renderLog(root, runs) {
  const el = root.querySelector('#rxLog');
  if (!runs.length) { el.innerHTML = `<div class="rx-empty">belum ada scan. edit file kode apa aja → auto-scan jalan.</div>`; return; }
  el.innerHTML = runs.map(r => {
    const auto = (r.scan_type || '').startsWith('auto');
    const on = selectedRun && selectedRun.id === r.id;
    const sev = r.critical_count ? 'critical' : r.high_count ? 'high' : r.medium_count ? 'medium' : r.low_count ? 'low' : 'info';
    const dotColor = (r.total_findings || 0) ? SEV_COLOR[sev] : '#2f5f47';
    return `<div class="rx-log-row${on ? ' on' : ''}" data-run="${r.id}">
      <span class="rx-dot" style="background:${dotColor};color:${dotColor}"></span>
      <span class="rx-id">#${r.id}</span>
      <span class="rx-tag ${r.status === 'fail' ? 'fail' : 'pass'}">${esc(r.status || '·')}</span>
      <span class="rx-tag ${auto ? 'auto' : 'manual'}">${esc((r.scan_type || 'manual').replace('auto:', ''))}</span>
      <span class="rx-path" title="${esc(r.target_path || '')}">${esc(r.target_path || '—')}</span>
      <span class="rx-hits">${esc(String(r.total_findings || 0))} hit</span>
    </div>`;
  }).join('');
  el.querySelectorAll('.rx-log-row').forEach(d => d.addEventListener('click', () => {
    const r = runs.find(x => String(x.id) === d.dataset.run);
    if (r) selectRun(root, r);
  }));
}

async function selectRun(root, run) {
  selectedRun = run;
  root.querySelectorAll('.rx-log-row').forEach(d => d.classList.toggle('on', d.dataset.run === String(run.id)));
  updateRadarFromRun(root, run);
  root.querySelector('#rxFindHead').innerHTML = `⚠ FINDINGS · <span style="color:#cfeede">${esc(run.target_path || '#' + run.id)}</span>`;
  const el = root.querySelector('#rxFindings');
  el.innerHTML = `<div class="rx-empty">memuat run #${run.id}…</div>`;
  try {
    const d = await fetchJSON(`/api/agents/scanner/findings?id=${AGENT_ID}&run_id=${encodeURIComponent(run.id)}`);
    const items = (d.items || []).sort((a, b) =>
      (SEV_ORDER[(a.severity || 'info').toLowerCase()] ?? 9) - (SEV_ORDER[(b.severity || 'info').toLowerCase()] ?? 9));
    renderFindings(el, items);
    updateRadar(root, items, statusFromItems(items));
  } catch (e) {
    el.innerHTML = `<div class="rx-empty">gagal: ${esc(String(e.message || e))}</div>`;
  }
}

function renderFindings(el, items) {
  if (!items.length) { el.innerHTML = `<div class="rx-empty ok">✓ CLEAR — ga ada temuan di run ini</div>`; return; }
  el.innerHTML = items.map(f => {
    const sev = (f.severity || 'info').toLowerCase();
    const c = SEV_COLOR[sev] || '#5eead4';
    return `<div class="rx-find">
      <div class="rx-find-h">
        <span class="rx-sev" style="background:${c}22;color:${c}">${esc(sev)}</span>
        <span class="rx-tag manual">${esc(f.auditor || '?')}</span>
      </div>
      <div class="rx-find-msg">${esc(f.message || '—')}</div>
      <div class="rx-find-file">${esc(f.file_path || '?')}<span class="ln">L${esc(String(f.line_number || 0))}</span></div>
      ${f.snippet ? `<div class="rx-snip">${esc(f.snippet)}</div>` : ''}
      ${f.remediation ? `<div class="rx-rem"><b>fix:</b> ${esc(f.remediation)}</div>` : ''}
    </div>`;
  }).join('');
}

function statusFromItems(items) {
  if (items.some(f => (f.severity || '').toLowerCase() === 'critical')) return 'THREAT';
  if (items.some(f => (f.severity || '').toLowerCase() === 'high')) return 'WARNING';
  if (items.length) return 'NOTED';
  return 'SECURE';
}

function updateRadarFromRun(root, r) {
  const pseudo = [];
  const push = (n, sev) => { for (let i = 0; i < (n || 0); i++) pseudo.push({ severity: sev }); };
  push(r.critical_count, 'critical'); push(r.high_count, 'high'); push(r.medium_count, 'medium'); push(r.low_count, 'low');
  updateRadar(root, pseudo, statusFromCounts(r));
}
function statusFromCounts(r) {
  if (r.critical_count) return 'THREAT';
  if (r.high_count) return 'WARNING';
  if ((r.total_findings || 0) > 0) return 'NOTED';
  return 'SECURE';
}

function updateRadar(root, items, status) {
  const radar = root.querySelector('#rxRadar');
  const core = root.querySelector('#rxCore');
  if (!radar || !core) return;
  radar.querySelectorAll('.rx-blip').forEach(b => b.remove());
  const list = (items || []).slice(0, 56);
  list.forEach((f, i) => {
    const sev = (f.severity || 'info').toLowerCase();
    const color = SEV_COLOR[sev] || '#5eead4';
    const rr = (SEV_RADIUS[sev] ?? 0.9) * 47;
    const ang = (i * 137.508) * Math.PI / 180;
    const x = 50 + rr * Math.cos(ang);
    const y = 50 + rr * Math.sin(ang);
    const b = document.createElement('div');
    b.className = 'rx-blip';
    b.style.cssText = `left:${x}%;top:${y}%;background:${color};color:${color};animation-delay:${(i % 8) * 0.12}s`;
    radar.appendChild(b);
  });
  const stColor = status === 'THREAT' ? '#ff3b3b' : status === 'WARNING' ? '#ff8c1a' : status === 'NOTED' ? '#ffd11a' : '#22ff88';
  core.innerHTML = `<div class="st" style="color:${stColor}">${status}</div><div class="ct">${list.length} contact${list.length === 1 ? '' : 's'}</div>`;
}
