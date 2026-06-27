// schedule.js — tab "Schedule" (fitur CORE, dipisah dari Trigger). Jadwal waktu: cron → suruh
// <agent/group> dgn <prompt> → kirim Telegram. Backend = trigger engine type "time" (udah teruji),
// tapi MENU sendiri biar ga ketuker sama Trigger (event plugin). Clean glass-3D, full-width.
import { esc, escAttr, fetchJSON } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { ensureGlass } from '/js/glass.js';

const L = new Proxy({}, { get: (_, k) => t('schedule.' + String(k)) });
const TYPE = 'time'; // Schedule = SATU tipe core (cron). Tipe event lain → tab Trigger.

// preset cron umum (label lewat i18n, value cron baku).
const PRESETS = [
  { k: 'hourly', cron: '0 * * * *' },
  { k: 'daily9', cron: '0 9 * * *' },
  { k: 'weekday9', cron: '0 9 * * 1-5' },
  { k: 'weekly_mon', cron: '0 9 * * 1' },
];

let agents = [], groups = [];

export async function render(mainEl) {
  ensureGlass();
  mainEl.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">⏰</span>
        <div>
          <h2 class="fw-title">${esc(L.title)}</h2>
          <div class="fw-sub">${esc(L.sub)}</div>
          <div class="fw-stat"><span class="fw-dot"></span><span id="scCount">…</span></div>
        </div>
        <span class="fw-grow"></span>
        <button class="fw-btn" id="scNew">＋ ${esc(L.new_btn)}</button>
      </div>
      <div id="scForm"></div>
      <div id="scList"><div class="fw-card"><div class="fw-empty">⟳ ${esc(L.loading)}</div></div></div>
    </div>`;
  mainEl.querySelector('#scNew').addEventListener('click', () => openForm(mainEl, null));
  await loadRefs();
  await load(mainEl);
}

async function loadRefs() {
  try { const d = await fetchJSON('/api/kernel/agents'); agents = (d.plugins || []).filter(a => a.id && a.kind !== 'channel'); } catch { agents = []; }
  try { const d = await fetchJSON('/api/groups'); groups = d.groups || []; } catch { groups = []; }
}

async function load(mainEl) {
  const list = mainEl.querySelector('#scList');
  let data;
  try { data = await fetchJSON('/api/triggers'); }
  catch (e) { list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(String(e.message || e))}</div></div>`; return; }
  const rules = (data.triggers || []).filter(r => r.type_id === TYPE); // CUMA jadwal waktu
  mainEl.querySelector('#scCount').textContent = `${rules.length} ${L.count_label}`;
  if (!rules.length) { list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(L.empty)}</div></div>`; return; }
  list.innerHTML = rules.map(cardHTML).join('');
  rules.forEach(r => wireCard(mainEl, r));
}

function cronOf(r) { try { return JSON.parse(r.config || '{}').cron || ''; } catch { return ''; } }

function cardHTML(r) {
  const st = r.last_status || '';
  const tone = st === 'ok' ? 'ok' : (st === 'error' || st === 'bad_config') ? 'danger' : 'idle';
  return `<div class="fw-card" data-id="${escAttr(r.id)}">
    <div class="fw-row">
      <h3>${r.enabled ? '🟢' : '⚪'} ${esc(r.name)}</h3>
      <span class="fw-tag">⏱ ${esc(cronOf(r))}</span>
      <span class="fw-id">→ ${esc(r.target)}</span>
      <span class="fw-grow"></span>
      <span class="fw-tag" data-tone="${tone}">${esc(st || L.idle)}</span>
      <button class="fw-btn" data-act="toggle">${r.enabled ? esc(L.btn_disable) : esc(L.btn_enable)}</button>
      <button class="fw-btn" data-act="run">▷ ${esc(L.btn_run)}</button>
      <button class="fw-btn" data-act="edit">✎</button>
      <button class="fw-btn" data-act="dup" title="Duplicate (new copy, disabled first)">⧉</button>
      <button class="fw-btn" data-act="hist">▸</button>
      <button class="fw-btn danger" data-act="del">🗑</button>
    </div>
    <div class="fw-desc" data-hist style="display:none"></div>
  </div>`;
}

function wireCard(mainEl, r) {
  const card = mainEl.querySelector(`.fw-card[data-id="${r.id}"]`); // id = slug server-validated
  if (!card) return;
  card.querySelector('[data-act="toggle"]').onclick = async () => {
    try { await fetchJSON(`/api/triggers/toggle?id=${encodeURIComponent(r.id)}&enabled=${r.enabled ? 0 : 1}`, { method: 'POST' }); await load(mainEl); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="run"]').onclick = async () => {
    try { await fetchJSON(`/api/triggers/run?id=${encodeURIComponent(r.id)}`, { method: 'POST' }); alert(L.run_ok); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="edit"]').onclick = () => openForm(mainEl, r);
  card.querySelector('[data-act="dup"]').onclick = async () => {
    try { const d = await fetchJSON(`/api/triggers/duplicate?id=${encodeURIComponent(r.id)}`, { method: 'POST' }); alert('Duplicated → ' + d.id + ' (disabled — enable when ready)'); await load(mainEl); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="del"]').onclick = async () => {
    if (!confirm(L.del_confirm.replace('{name}', r.name))) return;
    try { await fetchJSON(`/api/triggers/delete?id=${encodeURIComponent(r.id)}`, { method: 'POST' }); await load(mainEl); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="hist"]').onclick = async () => {
    const box = card.querySelector('[data-hist]');
    if (box.style.display === 'block') { box.style.display = 'none'; return; }
    box.style.display = 'block'; box.innerHTML = '…';
    try {
      const d = await fetchJSON(`/api/triggers/runs?id=${encodeURIComponent(r.id)}&limit=15`);
      const runs = d.runs || [];
      box.innerHTML = runs.length ? runs.map(x => `<div class="fw-row">${esc(x.fired_at)} · <b>${esc(x.status)}</b> · ${esc((x.result_text || x.error_text || '').slice(0, 120))}</div>`).join('') : `<div class="fw-empty">${esc(L.no_runs)}</div>`;
    } catch (e) { box.innerHTML = esc(e.message); }
  };
}

function targetOptions(sel) {
  const a = agents.map(x => `<option value="agent:${escAttr(x.id)}" ${sel === 'agent:' + x.id ? 'selected' : ''}>🤖 ${esc(x.display_name || x.id)}</option>`).join('');
  const g = groups.map(x => `<option value="group:${escAttr(x.id)}" ${sel === 'group:' + x.id ? 'selected' : ''}>👥 ${esc(x.display_name || x.id)}</option>`).join('');
  return `<optgroup label="Agents">${a}</optgroup>${g ? `<optgroup label="Groups">${g}</optgroup>` : ''}`;
}

function openForm(mainEl, r) {
  const box = mainEl.querySelector('#scForm');
  const editing = !!r;
  const cur = r || { id: '', name: '', config: '{}', target: '', target_kind: 'agent', prompt: '' };
  let cron = cronOf(cur) || '0 9 * * *';
  const selTarget = cur.target ? cur.target_kind + ':' + cur.target : '';
  box.innerHTML = `<div class="fw-card">
    <div class="fw-sec">${esc(editing ? L.edit_title : L.new_title)}</div>
    <div class="fw-grid">
      <div><div class="fw-sec">${esc(L.f_id)}</div>
        <input class="fw-input" id="fId" value="${escAttr(cur.id)}" ${editing ? 'readonly' : ''} placeholder="report-pagi"></div>
      <div><div class="fw-sec">${esc(L.f_name)}</div>
        <input class="fw-input" id="fName" value="${escAttr(cur.name)}" placeholder="Report Pagi"></div>
    </div>
    <div class="fw-sec" style="margin-top:14px">${esc(L.f_when)}</div>
    <div class="fw-row" id="fPresets">${PRESETS.map(p => `<span class="fw-tag" data-cron="${p.cron}" style="cursor:pointer">${esc(L['preset_' + p.k])}</span>`).join('')}</div>
    <div class="fw-row" style="margin-top:8px;align-items:center">
      <input class="fw-input fw-grow" id="fCron" style="min-width:160px;font-family:ui-monospace,monospace" value="${escAttr(cron)}" placeholder="0 9 * * *">
      <span class="fw-id">${esc(L.cron_help)}</span>
    </div>
    <div class="fw-sec" style="margin-top:14px">${esc(L.f_target)}</div>
    <select class="fw-input" id="fTarget">${targetOptions(selTarget)}</select>
    <div class="fw-sec" style="margin-top:14px">${esc(L.f_prompt)}</div>
    <textarea class="fw-input" id="fPrompt" style="min-height:64px;resize:vertical" placeholder="${escAttr(L.prompt_ph)}">${esc(cur.prompt)}</textarea>
    <div class="fw-row" id="fChips" style="margin-top:8px"><span class="fw-tag" data-chip="{{time}}" style="cursor:pointer">{{time}}</span><span class="fw-tag" data-chip="{{date}}" style="cursor:pointer">{{date}}</span></div>
    <div class="fw-row" style="margin-top:14px">
      <button class="fw-btn" id="fSave">${esc(L.save)}</button>
      <button class="fw-btn" id="fCancel">${esc(L.cancel)}</button>
      <span class="fw-id" id="fMsg"></span>
    </div>
  </div>`;
  const cronIn = box.querySelector('#fCron');
  const syncPresets = () => box.querySelectorAll('[data-cron]').forEach(p => p.classList.toggle('on', p.dataset.cron === cronIn.value.trim()));
  box.querySelectorAll('[data-cron]').forEach(p => p.onclick = () => { cronIn.value = p.dataset.cron; syncPresets(); });
  cronIn.oninput = syncPresets; syncPresets();
  box.querySelectorAll('[data-chip]').forEach(c => c.onclick = () => { const ta = box.querySelector('#fPrompt'); ta.value += c.dataset.chip; ta.focus(); });
  box.querySelector('#fCancel').onclick = () => { box.innerHTML = ''; };
  box.querySelector('#fSave').onclick = async () => {
    const msg = box.querySelector('#fMsg');
    const tv = box.querySelector('#fTarget').value; const ti = tv.indexOf(':');
    const payload = {
      id: box.querySelector('#fId').value.trim().toLowerCase(),
      name: box.querySelector('#fName').value.trim(),
      type_id: TYPE,
      config: JSON.stringify({ cron: cronIn.value.trim() }),
      target: ti >= 0 ? tv.slice(ti + 1) : tv,
      target_kind: ti >= 0 ? tv.slice(0, ti) : 'agent',
      prompt: box.querySelector('#fPrompt').value,
      enabled: true,
    };
    try {
      await fetchJSON('/api/triggers', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      box.innerHTML = ''; await load(mainEl);
    } catch (e) { msg.textContent = L.save_fail + ' ' + (e.message || e); }
  };
}
