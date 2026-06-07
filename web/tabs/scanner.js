// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Scanner tab — radar besar + scan log + findings. Audit pass: esc()
//   semua field, encodeURIComponent run_id, auto-poll interval di-clear tiap
//   render (no leak).
// Update 2026-06-06 (owner approved, final pre-prod audit): kabel-putus fix —
//   GUI dulu baca r.high_count/medium_count/low_count yang GA ADA di kontrak
//   /api/agents/scanner/runs (run row cuma punya critical_count + total_findings).
//   Akibatnya dot run penuh-HIGH nyamar 'info', dan poll 8s nge-downgrade radar.
//   Sekarang: dot = critical/has-findings/clean; preview count-based naro critical
//   presisi + sisanya 'medium'; poll ga ngebangun-ulang radar run terpilih
//   (immutable, udah presisi dari fetch findings). NO backend/schema change.
// Update 2026-06-05 (owner approved): + manual scan modal (⊕ Scan Target) —
//   allowlist-driven dropdown → POST /api/scanner/run (gated-exec). Hasil mirror
//   ke radar yang SAMA. Run via fetch mentah (bukan fetchJSON) biar 403 'denied'
//   allowlist ga salah-trigger prompt password. Semua string lewat dictionary.
//
// scanner.js — Mr.Flow "Threat Radar". Layout: radar gede (kiri) + scan log
// stream & findings detail (kanan). Background codescan engine isi log otomatis.

import { esc, escAttr, fetchJSON, loadStyle, openModal } from '../js/utils.js';
import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('scanner.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

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
.rx-stat .n { font-size:1.7rem; font-weight:700; color:var(--neon); line-height:1; font-variant-numeric:tabular-nums; }
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

/* ── manual scan form (modal) ── */
.rx-form { display:flex; flex-direction:column; gap:5px;
  font-family:'JetBrains Mono',ui-monospace,monospace; color:#bdf5d6; }
.rx-fl { font-size:0.62rem; letter-spacing:.12em; text-transform:uppercase; color:#7fb89c; margin-top:8px; }
.rx-in { background:rgba(0,0,0,0.45); border:1px solid rgba(34,255,136,0.28); border-radius:8px;
  color:#e6fff2; padding:9px 11px; font-family:inherit; font-size:0.82rem; outline:none; }
.rx-in:focus { border-color:var(--neon); box-shadow:0 0 0 2px rgba(34,255,136,0.14); }
.rx-warn { font-size:0.76rem; color:#ffd11a; padding:8px 11px; margin-top:4px;
  border:1px dashed rgba(255,209,26,0.4); border-radius:8px; }
.rx-result { margin-top:12px; font-size:0.78rem; min-height:18px; word-break:break-word; }
.rx-result .rx-ok { color:var(--neon); }
.rx-result .rx-err { color:#ff8080; }
.rx-result .rx-run { color:#5eead4; }

/* ── scanner arsenal (catalog: scroll + install/uninstall) ── */
.rx-arsenal { display:flex; flex-direction:column; gap:10px;
  font-family:'JetBrains Mono',ui-monospace,monospace; }
.rx-ars-head { display:flex; gap:10px; align-items:center; }
.rx-ars-head .rx-in { flex:1; }
.rx-ars-total { font-size:0.74rem; color:var(--neon); white-space:nowrap; font-weight:700;
  font-variant-numeric:tabular-nums; }
.rx-ars-list { max-height:54vh; overflow-y:auto; display:flex; flex-direction:column; gap:2px; padding-right:4px; }
.rx-ars-plane { margin-bottom:8px; }
.rx-ars-plabel { font-size:0.6rem; letter-spacing:.1em; text-transform:uppercase; color:#5eead4;
  padding:8px 4px 5px; position:sticky; top:0; background:rgba(15,17,26,0.97); z-index:1; }
.rx-ars-pc { color:#5c8a73; }
.rx-ars-row { display:flex; align-items:center; gap:10px; padding:7px 10px;
  border:1px solid rgba(34,255,136,0.10); border-radius:7px; background:rgba(0,0,0,0.25); }
.rx-ars-row.dim { opacity:0.42; }
.rx-ars-name { flex:1; font-size:0.8rem; color:#cfeede; word-break:break-all; }
.rx-ars-cnt { font-size:0.66rem; color:#7fb89c; white-space:nowrap; font-variant-numeric:tabular-nums; }
.rx-ars-core { font-size:0.58rem; color:#5c8a73; border:1px solid rgba(92,138,115,0.4);
  border-radius:5px; padding:2px 7px; letter-spacing:.05em; text-transform:uppercase; }
.rx-ars-toggle { font-size:0.66rem; font-weight:700; padding:4px 12px; border-radius:6px; cursor:pointer;
  font-family:inherit; letter-spacing:.04em; border:1px solid; }
.rx-ars-toggle.on { background:rgba(255,59,59,0.10); color:#ff8080; border-color:rgba(255,59,59,0.35); }
.rx-ars-toggle.off { background:rgba(34,255,136,0.12); color:var(--neon); border-color:rgba(34,255,136,0.4); }
.rx-ars-toggle:hover { filter:brightness(1.15); }
`;

export async function render(root) {
  loadStyle('scanner-radar', CSS);
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  selectedRun = null;

  root.innerHTML = `
    <div class="rx">
      <div class="rx-top">
        <h2 class="rx-title">⌖ Threat Radar</h2>
        <span class="rx-live"><span class="dot"></span>${esc(L.watchActive)}</span>
        <span class="rx-spacer"></span>
        <span style="font-size:0.66rem;color:#5c8a73;letter-spacing:.06em">agent=${esc(AGENT_ID)}</span>
        <button class="rx-btn" id="rxArsenal">≣ ${esc(L.arsenal)}</button>
        <button class="rx-btn" id="rxScan">⊕ ${esc(L.scanTarget)}</button>
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
            <div class="rx-phead">▚ SCAN LOG <span style="color:#5c8a73;letter-spacing:0">${esc(L.logSub)}</span></div>
            <div class="rx-pbody" id="rxLog"><div class="rx-empty">loading…</div></div>
          </div>
          <div class="rx-panel find">
            <div class="rx-phead" id="rxFindHead">⚠ FINDINGS</div>
            <div class="rx-pbody" id="rxFindings"><div class="rx-empty">${esc(L.pickEntry)}</div></div>
          </div>
        </div>
      </div>
    </div>`;

  root.querySelector('#rxRefresh').addEventListener('click', () => loadAll(root));
  root.querySelector('#rxScan').addEventListener('click', () => openScanModal(root));
  root.querySelector('#rxArsenal').addEventListener('click', () => openArsenalModal());
  await loadAll(root);
  pollTimer = setInterval(() => { loadRuns(root).catch(() => {}); }, 8000);
}

async function loadAll(root) {
  await Promise.all([loadAuditors(root), loadRuns(root)]);
}

async function loadAuditors(root) {
  try {
    const d = await fetchJSON(`/api/agents/scanner/auditors?id=${AGENT_ID}`);
    root.querySelector('#rxAud').textContent = `${(d.items || []).length} ${L.audSummary}`;
  } catch { /* ignore */ }
}

async function loadRuns(root) {
  const d = await fetchJSON(`/api/agents/scanner/runs?id=${AGENT_ID}&limit=60`);
  const runs = d.items || [];
  renderLog(root, runs);
  // CRITICAL = posture SEKARANG di SELURUH tubuh Flowork: ambil run TERBARU per
  // target (runs urut terbaru dulu), kritis = MAX di antara mereka. Bukan jumlah
  // 60 run (balon, ga turun walau udah di-fix), bukan cuma baseline agent (ga
  // nampilin repo lain kayak Router / body-scan).
  const latestPerTarget = {};
  for (const r of runs) {
    const key = (r.scan_type || '') + '|' + (r.target_path || '');
    if (!(key in latestPerTarget)) latestPerTarget[key] = r;
  }
  const critNow = Math.max(0, ...Object.values(latestPerTarget).map((r) => r.critical_count || 0));
  renderStats(root, {
    runs: runs.length,
    findings: runs.reduce((s, r) => s + (r.total_findings || 0), 0),
    critical: critNow,
  });
  if (!selectedRun && runs.length) {
    // default: pilih run PALING PARAH (critical terbanyak) biar radar langsung
    // nampilin ancaman tubuh-Flowork tanpa harus diklik; semua bersih → terbaru.
    const worst = runs.reduce((a, b) => ((b.critical_count || 0) > (a.critical_count || 0) ? b : a), runs[0]);
    selectRun(root, worst);
  } else if (!selectedRun) {
    updateRadar(root, [], 'SECURE');
  }
  // selectedRun set → radarnya udah ke-render PRESISI per-severity dari fetch
  // findings di selectRun(), dan sebuah run itu immutable sekali selesai — jadi
  // poll 8 detik cukup nge-refresh log + stats di atas; radar JANGAN dibangun
  // ulang dari count (itu bakal nurunin blip high/low jadi perkiraan kasar).
}

// compactNum — angka gede dipadetin biar lebar box tetep (layout ga goyang pas
// temuan numpuk): <1rb apa adanya, ribuan → "16k+", jutaan → "2M+".
function compactNum(n) {
  n = Number(n) || 0;
  if (n < 1000) return String(n);
  if (n < 1000000) return Math.floor(n / 1000) + 'k+';
  return Math.floor(n / 1000000) + 'M+';
}

function renderStats(root, s) {
  root.querySelector('#rxStats').innerHTML = `
    <div class="rx-stat"><div class="n" title="${s.runs}">${compactNum(s.runs)}</div><div class="l">runs</div></div>
    <div class="rx-stat"><div class="n" title="${s.findings}">${compactNum(s.findings)}</div><div class="l">findings</div></div>
    <div class="rx-stat"><div class="n" title="${s.critical}" style="color:${s.critical ? '#ff3b3b' : '#22ff88'}">${compactNum(s.critical)}</div><div class="l">critical</div></div>`;
}

function renderLog(root, runs) {
  const el = root.querySelector('#rxLog');
  if (!runs.length) { el.innerHTML = `<div class="rx-empty">${esc(L.noScan)}</div>`; return; }
  el.innerHTML = runs.map(r => {
    const auto = (r.scan_type || '').startsWith('auto');
    const on = selectedRun && selectedRun.id === r.id;
    // The runs API persists only critical_count + total_findings per run (no per-
    // severity breakdown), so the dot reflects exactly what we have: red = has a
    // critical, amber = has findings, dim = clean. (Don't read high/medium/low_count
    // — those fields don't exist on the run row and would always read undefined.)
    const dotColor = r.critical_count ? SEV_COLOR.critical : (r.total_findings || 0) ? SEV_COLOR.medium : '#2f5f47';
    return `<div class="rx-log-row${on ? ' on' : ''}" data-run="${r.id}">
      <span class="rx-dot" style="background:${dotColor};color:${dotColor}"></span>
      <span class="rx-id">#${r.id}</span>
      <span class="rx-tag ${r.status === 'fail' ? 'fail' : 'pass'}">${esc(r.status || '·')}</span>
      <span class="rx-tag ${auto ? 'auto' : 'manual'}">${esc((r.scan_type || 'manual').replace('auto:', ''))}</span>
      <span class="rx-path" title="${escAttr(r.target_path || '')}">${esc(r.target_path || '—')}</span>
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
  if (!items.length) { el.innerHTML = `<div class="rx-empty ok">${esc(L.clear)}</div>`; return; }
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

// Count-based INSTANT preview saat klik log row (sebelum findings ke-fetch). Run
// row cuma nyimpen critical_count + total_findings (ga ada per-severity breakdown),
// jadi preview ini cuma bisa naro blip critical presisi; sisanya jadi kontak
// 'medium' generik. Radar per-severity yang AUTHORITATIVE dateng dari fetch
// findings di selectRun() (updateRadar dengan item asli).
function updateRadarFromRun(root, r) {
  const pseudo = [];
  const push = (n, sev) => { for (let i = 0; i < (n || 0); i++) pseudo.push({ severity: sev }); };
  const crit = r.critical_count || 0;
  push(crit, 'critical');
  push(Math.max(0, (r.total_findings || 0) - crit), 'medium');
  updateRadar(root, pseudo, statusFromCounts(r));
}
function statusFromCounts(r) {
  if (r.critical_count) return 'THREAT';
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

// ── manual scan (owner) ─────────────────────────────────────────────────────
// Run an allowlisted tool against an in-scope target. Backend /api/scanner/run
// is GATED-EXEC: blocklist + allowlist exec + allowlist target + no-shell. The
// tool/target dropdowns come FROM the allowlist (owner-editable gate) — we never
// hardcode a tool list. Results mirror into this same radar (scan log + findings).
async function openScanModal(root) {
  let execList = [];
  let targetList = [];
  try {
    const [ex, tg] = await Promise.all([
      fetchJSON('/api/scanner/allowlist?kind=exec'),
      fetchJSON('/api/scanner/allowlist?kind=target'),
    ]);
    execList = (ex.allowlist || []).map((a) => a.value);
    targetList = (tg.allowlist || []).map((a) => a.value);
  } catch { /* allowlist unreachable → warn shown below */ }

  const box = document.createElement('div');
  box.className = 'rx-form';
  box.innerHTML = `
    <label class="rx-fl">${esc(L.fTool)}</label>
    ${execList.length
      ? `<select class="rx-in" id="scBin">${execList.map((v) => `<option value="${escAttr(v)}">${esc(v)}</option>`).join('')}</select>`
      : `<div class="rx-warn">${esc(L.noExec)}</div>`}
    <label class="rx-fl">${esc(L.fTarget)}</label>
    <input class="rx-in" id="scTarget" list="scTgtList" placeholder="${escAttr(L.fTargetHint)}" autocomplete="off">
    <datalist id="scTgtList">${targetList.map((v) => `<option value="${escAttr(v)}"></option>`).join('')}</datalist>
    <label class="rx-fl">${esc(L.fArgs)}</label>
    <input class="rx-in" id="scArgs" placeholder="${escAttr(L.fArgsHint)}" autocomplete="off">
    <label class="rx-fl">${esc(L.fCategory)}</label>
    <select class="rx-in" id="scCat">
      <option value="immune">${esc(L.catImmune)}</option>
      <option value="pentest">${esc(L.catPentest)}</option>
    </select>
    <div class="rx-result" id="scResult"></div>`;

  openModal({
    title: L.modalTitle,
    body: box,
    buttons: [
      { label: L.cancel, variant: 'secondary', onClick: ({ modal }) => modal.close() },
      { label: L.runBtn, onClick: ({ modal }) => runScan(root, box, modal) },
    ],
  });
}

async function runScan(root, box, modal) {
  const binSel = box.querySelector('#scBin');
  const binary = binSel ? binSel.value.trim() : '';
  const target = box.querySelector('#scTarget').value.trim();
  const argsRaw = box.querySelector('#scArgs').value.trim();
  const category = box.querySelector('#scCat').value;
  const resEl = box.querySelector('#scResult');
  if (!binary) { resEl.innerHTML = `<span class="rx-err">${esc(L.noExec)}</span>`; return; }
  const args = argsRaw ? argsRaw.split(/\s+/) : [];
  modal.setBusy(true);
  resEl.innerHTML = `<span class="rx-run">${esc(L.running)}</span>`;
  // Raw fetch (not fetchJSON): backend returns 403 {denied} for allowlist misses;
  // fetchJSON would treat 403 as auth and pop a password prompt. We want the body.
  try {
    const r = await fetch('/api/scanner/run', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ binary, args, target, category }),
    });
    const d = await r.json().catch(() => ({ error: 'bad response' }));
    if (d.denied) {
      resEl.innerHTML = `<span class="rx-err">⛔ ${esc(d.denied)}</span>`;
    } else if (d.error) {
      resEl.innerHTML = `<span class="rx-err">${esc(d.error)}</span>`;
    } else {
      const n = d.findings_count != null ? d.findings_count : (d.findings || []).length;
      resEl.innerHTML = `<span class="rx-ok">✓ ${esc(L.scanDone)} · ${esc(String(n))} ${esc(L.findingsWord)} · run #${esc(String(d.run_id || ''))}</span>`;
      await loadRuns(root);
    }
  } catch (e) {
    resEl.innerHTML = `<span class="rx-err">${esc(String(e.message || e))}</span>`;
  } finally {
    modal.setBusy(false);
  }
}

// ── scanner arsenal (catalog: scroll + install/uninstall) ────────────────────
// Full inventory: code auditors (defensive core) + tools + nuclei template packs
// (offensive, 1 file = 1 check). Owner install/uninstall pack nuclei (POST
// /api/scanner/registry/toggle, persist flowork.db); core defensif ga di-copot.
// Total = yang kepasang → "ribuan" jadi KELIHATAN + bisa di-kurasi.
async function openArsenalModal() {
  const box = document.createElement('div');
  box.className = 'rx-arsenal';
  box.innerHTML = `
    <div class="rx-ars-head">
      <input class="rx-in" id="arsSearch" placeholder="${escAttr(L.arsenalSearch)}" autocomplete="off">
      <span class="rx-ars-total" id="arsTotal">…</span>
    </div>
    <div class="rx-ars-list" id="arsList"><div class="rx-empty">loading…</div></div>`;
  openModal({
    title: L.arsenalTitle,
    body: box,
    buttons: [{ label: L.cancel, variant: 'secondary', onClick: ({ modal }) => modal.close() }],
  });
  await loadArsenal(box);
  box.querySelector('#arsSearch').addEventListener('input', (e) => filterArsenal(box, e.target.value));
}

async function loadArsenal(box) {
  try {
    const d = await fetchJSON('/api/scanner/registry');
    box.querySelector('#arsTotal').textContent = `${Number(d.total_installed || 0).toLocaleString()} ${L.arsenalTotalWord}`;
    renderArsenal(box, d.planes || []);
  } catch (e) {
    box.querySelector('#arsList').innerHTML = `<div class="rx-empty">gagal: ${esc(String(e.message || e))}</div>`;
  }
}

function renderArsenal(box, planes) {
  const el = box.querySelector('#arsList');
  if (!planes.length) { el.innerHTML = `<div class="rx-empty">—</div>`; return; }
  el.innerHTML = planes.map((pl) => `
    <div class="rx-ars-plane">
      <div class="rx-ars-plabel">${esc(pl.label)} <span class="rx-ars-pc">${(pl.items || []).length}</span></div>
      ${(pl.items || []).map((it) => arsenalRow(pl, it)).join('')}
    </div>`).join('');
  el.querySelectorAll('.rx-ars-toggle').forEach((b) =>
    b.addEventListener('click', () => toggleArsenal(box, b.dataset.id, b.dataset.installed === '1')));
}

function arsenalRow(pl, it) {
  const cnt = it.count > 1 ? `<span class="rx-ars-cnt">${Number(it.count).toLocaleString()} ${esc(L.checksWord)}</span>` : '';
  const ctrl = pl.removable
    ? `<button class="rx-ars-toggle ${it.installed ? 'on' : 'off'}" data-id="${escAttr(it.id)}" data-installed="${it.installed ? 1 : 0}">${it.installed ? esc(L.uninstall) : esc(L.install)}</button>`
    : `<span class="rx-ars-core">${esc(L.coreBadge)}</span>`;
  return `<div class="rx-ars-row${it.installed ? '' : ' dim'}" data-name="${escAttr(String(it.name || '').toLowerCase())}">
    <span class="rx-ars-name">${esc(it.name)}</span>${cnt}${ctrl}</div>`;
}

async function toggleArsenal(box, id, currentlyInstalled) {
  try {
    const r = await fetch('/api/scanner/registry/toggle', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, installed: !currentlyInstalled }),
    });
    const d = await r.json().catch(() => ({}));
    if (d.ok) await loadArsenal(box);
  } catch { /* state unchanged on failure */ }
}

function filterArsenal(box, q) {
  q = (q || '').trim().toLowerCase();
  box.querySelectorAll('.rx-ars-row').forEach((row) => {
    row.style.display = (!q || (row.dataset.name || '').includes(q)) ? '' : 'none';
  });
}
