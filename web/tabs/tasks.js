// tasks.js — FASE 5: tab "Tasks" (Category Task builder + run timeline).
//
// Owner bikin/edit kategori task + crew (analis + synthesizer), jalanin, lihat
// progress per-agent live (poll /api/taskflow/run-detail) + hasil keputusan.
// Definisi di flowork.db (/api/taskflow/*). Worker tetap isolated di state.db.
//
// NOTE: label UI dipusatkan di L{} (bukan scatter-inline) — gampang di-i18n
// nanti. (Migrasi ke dictionary t() = follow-up.)

import { esc, escAttr, fetchJSON, loadStyle, openModal } from '../js/utils.js';
import { t } from '/js/i18n.js';

// Label UI lewat dictionary i18n (en base + id) — no hardcode. L.camelKey →
// t('tasks.snake_key'), di-resolve lazy (pas akses = render time = i18n loaded).
const L = new Proxy({}, {
  get: (_, k) => t('tasks.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())),
});
const ST = { pending: '⚪', running: '🔵', done: '✅', error: '❌' };

let pollTimer = null;

export async function render(mainEl) {
  loadStyle('tasks-style', STYLE);
  mainEl.innerHTML = `
    <div class="tf-wrap">
      <div class="tf-head">
        <div><h2>📋 ${esc(L.title)}</h2><p class="tf-sub">${esc(L.sub)}</p></div>
        <div class="tf-hbtns">
          <button class="tf-btn ghost" id="tf-sched">${esc(L.schedBtn)}</button>
          <button class="tf-btn ghost" id="tf-mcp">${esc(L.mcpBtn)}</button>
          <button class="tf-btn" id="tf-new">${esc(L.newCat)}</button>
        </div>
      </div>
      <div class="tf-body">
        <div class="tf-list" id="tf-list"></div>
        <div class="tf-detail" id="tf-detail"><div class="tf-empty">${esc(L.noCat)}</div></div>
      </div>
    </div>`;
  mainEl.querySelector('#tf-new').onclick = () => newCategory();
  mainEl.querySelector('#tf-mcp').onclick = () => showMCP();
  mainEl.querySelector('#tf-sched').onclick = () => showSchedules();
  await loadList();
}

// showSchedules — kelola jadwal LOOPING task (tiap hari jam X / tiap N menit →
// auto-jalanin task → kirim hasil ke Telegram).
async function showSchedules() {
  let cats = [];
  try { cats = (await fetchJSON('/api/taskflow/categories')).categories || []; } catch (e) {}
  const opts = cats.map(c => `<option value="${escAttr(c.id)}">${esc(c.name || c.id)}</option>`).join('');
  const node = document.createElement('div');
  node.className = 'tf-sched';
  node.innerHTML = `
    <p>${esc(L.schedIntro)}</p>
    <div class="tf-schform">
      <select class="tf-in" id="ts-cat">${opts}</select>
      <input class="tf-in" id="ts-subj" placeholder="${escAttr(L.subject)}"/>
      <select class="tf-in" id="ts-kind">
        <option value="daily">${esc(L.schedDaily)}</option>
        <option value="every">${esc(L.schedEvery)}</option>
      </select>
      <input class="tf-in tf-tw" id="ts-time" value="09:00" placeholder="HH:MM"/>
      <input class="tf-in tf-tw" id="ts-min" type="number" value="60" style="display:none"/>
      <input class="tf-in" id="ts-chat" placeholder="${escAttr(L.schedChatPh)}"/>
      <button class="tf-btn" id="ts-add">${esc(L.schedAdd)}</button>
    </div>
    <div id="ts-list" class="tf-schlist"></div>`;
  openModal({ title: L.schedTitle, body: node });

  const kind = node.querySelector('#ts-kind');
  const tw = node.querySelector('#ts-time'), mw = node.querySelector('#ts-min');
  kind.onchange = () => {
    const daily = kind.value === 'daily';
    tw.style.display = daily ? '' : 'none';
    mw.style.display = daily ? 'none' : '';
  };
  const reload = async () => {
    let list = [];
    try { list = (await fetchJSON('/api/taskflow/schedules')).schedules || []; } catch (e) {}
    const el = node.querySelector('#ts-list');
    if (!list.length) { el.innerHTML = `<div class="tf-empty sm">${esc(L.schedNone)}</div>`; return; }
    el.innerHTML = list.map(s => {
      const when = s.kind === 'daily' ? `${esc(L.schedDailyAt)} ${esc(s.at_time)}` : `${esc(s.every_min)} ${esc(L.schedEveryMin)}`;
      return `<div class="tf-schrow">
        <span>⏰ <b>${esc(s.category)}</b> ${esc(s.subject)} — ${when}${s.notify_chat ? ' → 📨' : ''}</span>
        <small>${esc(L.schedNext)} ${esc((s.next_run || '').slice(11, 16))}</small>
        <button class="tf-x" data-id="${s.id}">✕</button></div>`;
    }).join('');
    el.querySelectorAll('.tf-x').forEach(b => b.onclick = async () => {
      await fetchJSON('/api/taskflow/schedule/delete?id=' + b.dataset.id, { method: 'POST' });
      reload();
    });
  };
  node.querySelector('#ts-add').onclick = async () => {
    const body = {
      category: node.querySelector('#ts-cat').value,
      subject: node.querySelector('#ts-subj').value.trim(),
      kind: kind.value,
      at_time: tw.value.trim() || '09:00',
      every_min: parseInt(mw.value) || 60,
      notify_chat: node.querySelector('#ts-chat').value.trim(),
    };
    if (!body.subject) return alert(L.fillSubject);
    try {
      await fetchJSON('/api/taskflow/schedule', { method: 'POST', body: JSON.stringify(body) });
      node.querySelector('#ts-subj').value = '';
      reload();
    } catch (e) { alert(L.schedFail + e.message); }
  };
  reload();
}

// showMCP — panel config MCP siap-copas buat AI eksternal (VS Code/Cursor/Claude).
async function showMCP() {
  let d;
  try { d = await fetchJSON('/api/mcp/config'); } catch (e) { alert(L.mcpFetchFail + e.message); return; }
  const node = document.createElement('div');
  node.className = 'tf-mcp';
  node.innerHTML = `
    <p>${esc(L.mcpIntro)}</p>
    ${d.binary_exists ? '' : `<div class="tf-warn">${esc(L.mcpNobin)}<br><code>${esc(d.build_cmd)}</code></div>`}
    <pre class="tf-mcpcfg">${esc(d.config)}</pre>
    <button class="tf-btn" id="tf-mcpcopy">${esc(L.mcpCopy)}</button>
    <div class="tf-mcphelp">
      <b>${esc(L.mcpWhere)}</b><br>
      • ${esc(L.mcpHelpClaude)}<br>
      • ${esc(L.mcpHelpVscode)}<br>
      • ${esc(L.mcpHelpCursor)}<br>
      <small>${esc(L.mcpHelpNote)} (${esc(d.self_url)})</small>
    </div>`;
  openModal({ title: L.mcpTitle, body: node });
  node.querySelector('#tf-mcpcopy').onclick = (e) => {
    navigator.clipboard.writeText(d.config).then(() => {
      e.target.textContent = L.mcpCopied;
      setTimeout(() => { e.target.textContent = L.mcpCopy; }, 1500);
    }).catch(() => alert(L.mcpCopyfail));
  };
}

async function loadList(selectID) {
  const list = document.getElementById('tf-list');
  if (!list) return;
  let cats = [];
  try { cats = (await fetchJSON('/api/taskflow/categories')).categories || []; } catch (e) { cats = []; }
  if (!cats.length) { list.innerHTML = `<div class="tf-empty">${esc(L.noCat)}</div>`; return; }
  list.innerHTML = cats.map(c => `
    <div class="tf-card${c.id === selectID ? ' active' : ''}" data-id="${escAttr(c.id)}">
      <span class="tf-ic">${esc(c.icon || '📋')}</span>
      <div class="tf-cn"><b>${esc(c.name || c.id)}</b><small>${esc(c.id)}${c.enabled ? '' : ' · off'}</small></div>
    </div>`).join('');
  list.querySelectorAll('.tf-card').forEach(el => {
    el.onclick = () => { list.querySelectorAll('.tf-card').forEach(x => x.classList.remove('active')); el.classList.add('active'); openCategory(el.dataset.id); };
  });
  if (selectID) openCategory(selectID);
}

function newCategory() {
  const id = (prompt(L.newCatPrompt) || '').trim().toLowerCase();
  if (!id || !/^[a-z][a-z0-9-]*$/.test(id)) { if (id) alert(L.idInvalid); return; }
  renderDetail({ id, name: id, icon: '📋', synthesizer: '', enabled: true, crew: [] }, true);
}

async function openCategory(id) {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
  let cat;
  try { cat = await fetchJSON('/api/taskflow/category?id=' + encodeURIComponent(id)); } catch (e) { cat = null; }
  if (!cat) return;
  renderDetail(cat, false);
}

function renderDetail(cat, isNew) {
  const d = document.getElementById('tf-detail');
  if (!d) return;
  const crewRows = (cat.crew || []).map((a, i) => crewRowHTML(a, i)).join('');
  d.innerHTML = `
    <div class="tf-dwrap">
      <div class="tf-drow">
        <input class="tf-in" id="tf-name" value="${escAttr(cat.name || '')}" placeholder="${escAttr(L.catNamePh)}"/>
        <input class="tf-in tf-ic-in" id="tf-icon" value="${escAttr(cat.icon || '📋')}" maxlength="2"/>
        <label class="tf-en"><input type="checkbox" id="tf-enabled" ${cat.enabled ? 'checked' : ''}/> ${esc(L.active)}</label>
        ${isNew ? '' : `<button class="tf-btn ghost danger" id="tf-del">${esc(L.del)}</button>`}
      </div>
      <input type="hidden" id="tf-id" value="${escAttr(cat.id)}"/>

      <h4>${esc(L.crew)}</h4>
      <div id="tf-crew">${crewRows || `<div class="tf-empty sm">${esc(L.emptyCrew)}</div>`}</div>
      <button class="tf-btn ghost" id="tf-add">${esc(L.addAnalyst)}</button>

      <div class="tf-drow tf-synth">
        <label>${esc(L.synth)}:</label>
        <input class="tf-in" id="tf-synth" value="${escAttr(cat.synthesizer || '')}" placeholder="${escAttr(L.synthPh)}"/>
        <button class="tf-btn" id="tf-save">${esc(L.save)}</button>
      </div>

      <hr/>
      <div class="tf-drow">
        <input class="tf-in" id="tf-subject" placeholder="${escAttr(L.subject)}"/>
        <button class="tf-btn run" id="tf-run">${esc(L.run)}</button>
      </div>
      <div id="tf-timeline"></div>

      <h4>${esc(L.history)}</h4>
      <div id="tf-runs"><div class="tf-empty sm">—</div></div>
    </div>`;

  const addCrew = (a) => {
    const wrap = d.querySelector('#tf-crew');
    const empty = wrap.querySelector('.tf-empty'); if (empty) empty.remove();
    wrap.insertAdjacentHTML('beforeend', crewRowHTML(a || { agent_id: '', role_label: '' }, wrap.children.length));
    bindCrewRemoval(d);
  };
  d.querySelector('#tf-add').onclick = () => addCrew(null);
  bindCrewRemoval(d);
  d.querySelector('#tf-save').onclick = () => saveCategory(d);
  d.querySelector('#tf-run').onclick = () => startRun(d);
  if (d.querySelector('#tf-del')) d.querySelector('#tf-del').onclick = () => delCategory(cat.id);
  if (!isNew) loadRuns(cat.id);
}

function crewRowHTML(a, i) {
  return `<div class="tf-crow" data-i="${i}">
    <input class="tf-in agent" value="${escAttr(a.agent_id || '')}" placeholder="${escAttr(L.agentPh)}"/>
    <input class="tf-in role" value="${escAttr(a.role_label || '')}" placeholder="${escAttr(L.rolePh)}"/>
    <button class="tf-x" title="${escAttr(L.del)}">✕</button>
  </div>`;
}
function bindCrewRemoval(d) {
  d.querySelectorAll('.tf-crow .tf-x').forEach(b => b.onclick = (e) => e.target.closest('.tf-crow').remove());
}

function collectCrew(d) {
  return [...d.querySelectorAll('.tf-crow')].map(r => ({
    agent_id: r.querySelector('.agent').value.trim(),
    role_label: r.querySelector('.role').value.trim(),
  })).filter(a => a.agent_id);
}

async function saveCategory(d) {
  const body = {
    id: d.querySelector('#tf-id').value.trim(),
    name: d.querySelector('#tf-name').value.trim(),
    icon: d.querySelector('#tf-icon').value.trim() || '📋',
    synthesizer: d.querySelector('#tf-synth').value.trim(),
    enabled: d.querySelector('#tf-enabled').checked,
    crew: collectCrew(d),
  };
  if (!body.id) return alert(L.idEmpty);
  try {
    await fetchJSON('/api/taskflow/category', { method: 'POST', body: JSON.stringify(body) });
    await loadList(body.id);
  } catch (e) { alert(L.saveFail + e.message); }
}

async function delCategory(id) {
  if (!confirm(L.delConfirm + ' "' + id + '"?')) return;
  try {
    await fetchJSON('/api/taskflow/category/delete?id=' + encodeURIComponent(id), { method: 'POST' });
    document.getElementById('tf-detail').innerHTML = `<div class="tf-empty">${esc(L.noCat)}</div>`;
    await loadList();
  } catch (e) { alert(L.delFail + e.message); }
}

async function startRun(d) {
  const id = d.querySelector('#tf-id').value.trim();
  const subject = d.querySelector('#tf-subject').value.trim();
  if (!subject) return alert(L.fillSubject);
  const tl = d.querySelector('#tf-timeline');
  tl.innerHTML = `<div class="tf-run-box">${esc(L.start)} ${esc(subject)}…</div>`;
  let resp;
  try {
    resp = await fetchJSON(`/api/taskflow/run?category=${encodeURIComponent(id)}&subject=${encodeURIComponent(subject)}`, { method: 'POST' });
  } catch (e) { tl.innerHTML = `<div class="tf-run-box err">${esc(L.failed)}${esc(e.message)}</div>`; return; }
  if (resp.error) { tl.innerHTML = `<div class="tf-run-box err">${esc(resp.error)}</div>`; return; }
  pollRun(resp.run_id, tl, id);
}

function pollRun(runID, tl, catID) {
  if (pollTimer) clearInterval(pollTimer);
  const tick = async () => {
    let run;
    try { run = await fetchJSON('/api/taskflow/run-detail?id=' + runID); } catch (e) { return; }
    tl.innerHTML = timelineHTML(run);
    if (run.status !== 'running') {
      clearInterval(pollTimer); pollTimer = null;
      loadRuns(catID);
    }
  };
  tick();
  pollTimer = setInterval(tick, 2500);
}

function timelineHTML(run) {
  const steps = (run.steps || []).map(s => `
    <div class="tf-step ${esc(s.status)}">
      <span class="tf-st">${ST[s.status] || '·'}</span>
      <span class="tf-sa">${esc(s.agent_id)}</span>
      <span class="tf-sr">${esc(s.role_label || '')}</span>
      <span class="tf-sm">${s.ms ? (s.ms / 1000).toFixed(0) + 's' : (s.status === 'running' ? L.running : '')}</span>
      ${s.err ? `<span class="tf-se" title="${escAttr(s.err)}">⚠</span>` : ''}
    </div>`).join('');
  const rec = run.status === 'done' && run.summary
    ? `<div class="tf-result"><b>${esc(L.result)}:</b><div class="tf-md">${esc(run.summary)}</div></div>` : '';
  return `<div class="tf-run-box">
    <div class="tf-run-head">Run #${run.id} · ${esc(run.input_text)} · <b>${esc(run.status)}</b></div>
    ${steps}${rec}</div>`;
}

async function loadRuns(catID) {
  const el = document.getElementById('tf-runs');
  if (!el) return;
  let runs = [];
  try { runs = (await fetchJSON('/api/taskflow/runs?category=' + encodeURIComponent(catID) + '&limit=15')).runs || []; } catch (e) {}
  if (!runs.length) { el.innerHTML = `<div class="tf-empty sm">—</div>`; return; }
  el.innerHTML = runs.map(r => `
    <div class="tf-runrow" data-id="${r.id}">
      <span class="tf-rs ${esc(r.status)}">${ST[r.status] || '·'}</span>
      <span>#${r.id} ${esc(r.input_text)}</span>
      <small>${esc((r.started_at || '').replace('T', ' ').slice(0, 16))}</small>
    </div>`).join('');
  el.querySelectorAll('.tf-runrow').forEach(row => row.onclick = async () => {
    const run = await fetchJSON('/api/taskflow/run-detail?id=' + row.dataset.id);
    document.getElementById('tf-timeline').innerHTML = timelineHTML(run);
  });
}

const STYLE = `
.tf-wrap{padding:16px;height:100%;display:flex;flex-direction:column}
.tf-head{display:flex;justify-content:space-between;align-items:center;margin-bottom:12px}
.tf-head h2{margin:0;font-size:20px}.tf-sub{margin:2px 0 0;opacity:.6;font-size:12px}
.tf-body{display:flex;gap:14px;flex:1;min-height:0}
.tf-list{width:230px;flex-shrink:0;overflow:auto;display:flex;flex-direction:column;gap:6px}
.tf-card{display:flex;gap:9px;align-items:center;padding:9px 11px;border:1px solid var(--border,#2a2a35);border-radius:9px;cursor:pointer;background:var(--card,#16161c)}
.tf-card:hover{border-color:#4a7}.tf-card.active{border-color:#5b9;background:#16201a}
.tf-ic{font-size:19px}.tf-cn b{font-size:13px}.tf-cn small{display:block;opacity:.5;font-size:11px}
.tf-detail{flex:1;overflow:auto;border:1px solid var(--border,#2a2a35);border-radius:11px;background:var(--card,#16161c)}
.tf-empty{padding:40px;text-align:center;opacity:.4}.tf-empty.sm{padding:12px;font-size:12px}
.tf-dwrap{padding:16px}.tf-dwrap h4{margin:16px 0 7px;font-size:13px;opacity:.8}
.tf-drow{display:flex;gap:8px;align-items:center;margin-bottom:8px;flex-wrap:wrap}
.tf-in{flex:1;min-width:90px;padding:7px 10px;border:1px solid var(--border,#333);border-radius:7px;background:#0d0d12;color:inherit;font-size:13px}
.tf-ic-in{flex:0 0 48px;text-align:center}.tf-en{font-size:12px;opacity:.8;white-space:nowrap}
.tf-crow{display:flex;gap:6px;margin-bottom:6px;align-items:center}.tf-crow .agent{flex:0 0 40%}
.tf-x{background:none;border:none;color:#c66;cursor:pointer;font-size:14px;padding:4px}
.tf-synth{margin-top:10px}.tf-synth label{font-size:12px;opacity:.8}
.tf-btn{padding:7px 13px;border:1px solid #5b9;border-radius:7px;background:#1a2b22;color:#7ec;cursor:pointer;font-size:13px}
.tf-btn:hover{background:#234}.tf-btn.ghost{border-color:#444;background:transparent;color:inherit}
.tf-btn.danger{border-color:#a44;color:#e88}.tf-btn.run{border-color:#7a5;background:#2a2410;color:#fc9}
.tf-timeline,#tf-timeline{margin:6px 0}
.tf-run-box{border:1px solid var(--border,#333);border-radius:9px;padding:11px;background:#0d0d12;font-size:13px}
.tf-run-box.err{border-color:#a44;color:#e99}
.tf-run-head{font-size:12px;opacity:.75;margin-bottom:8px}
.tf-step{display:flex;gap:9px;align-items:center;padding:5px 0;font-size:13px;border-bottom:1px solid #1e1e26}
.tf-step.running{color:#7bf}.tf-step.error{color:#e88}.tf-step.done{color:#9d9}
.tf-sa{font-weight:600;flex:0 0 150px}.tf-sr{opacity:.6;flex:1;font-size:12px}.tf-sm{opacity:.6;font-size:12px}
.tf-se{color:#e84}.tf-result{margin-top:11px;padding-top:10px;border-top:1px solid #2a2a33}
.tf-md{white-space:pre-wrap;font-size:12px;line-height:1.5;margin-top:5px;max-height:340px;overflow:auto}
.tf-runrow{display:flex;gap:9px;align-items:center;padding:7px 9px;border-radius:7px;cursor:pointer;font-size:12px}
.tf-runrow:hover{background:#1c1c24}.tf-rs.error{color:#e88}.tf-rs.done{color:#9d9}.tf-runrow small{margin-left:auto;opacity:.45}
.tf-hbtns{display:flex;gap:8px}
.tf-mcp p{font-size:13px;line-height:1.5;margin:0 0 10px}
.tf-mcpcfg{background:#0a0a0e;border:1px solid #2a2a33;border-radius:8px;padding:12px;font-size:12px;font-family:monospace;white-space:pre;overflow:auto;max-height:240px;color:#9ec}
.tf-warn{background:#2a2410;border:1px solid #6a5;border-radius:7px;padding:9px;font-size:12px;margin-bottom:10px;color:#fc9}
.tf-warn code,.tf-mcp code{background:#000;padding:1px 5px;border-radius:4px;font-size:11px}
.tf-mcphelp{margin-top:12px;font-size:12px;line-height:1.7;opacity:.85;border-top:1px solid #2a2a33;padding-top:10px}
.tf-sched p{font-size:13px;margin:0 0 10px;opacity:.85}
.tf-schform{display:flex;flex-wrap:wrap;gap:7px;margin-bottom:12px}
.tf-schform .tf-in{flex:1;min-width:120px}.tf-tw{flex:0 0 80px!important;min-width:0!important}
.tf-schlist{border-top:1px solid #2a2a33;padding-top:8px}
.tf-schrow{display:flex;gap:8px;align-items:center;padding:7px 4px;font-size:12px;border-bottom:1px solid #1e1e26}
.tf-schrow small{margin-left:auto;opacity:.5}
`;
