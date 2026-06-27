// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-20 (+ target system:compact-all).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner).
//
// triggers.js — tab "Trigger" (ROADMAP 3): otomasi event→aksi, ala Google Tag Manager.
// KALAU <event> MAKA suruh <agent/group> dgn <prompt {{payload}}> → kirim Telegram.
// Clean glass-3D, full-width (design-system fw-*). Semua label lewat i18n (dictionary), no hardcode.
import { esc, escAttr, fetchJSON } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { ensureGlass } from '/js/glass.js';

const L = new Proxy({}, { get: (_, k) => t('triggers.' + String(k)) });
const fmt = (k, v) => Object.entries(v || {}).reduce((s, [n, val]) => s.replaceAll('{' + n + '}', val), L[k]);

let agents = [], groups = [], types = [];

export async function render(mainEl) {
  ensureGlass();
  mainEl.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">⚡</span>
        <div>
          <h2 class="fw-title">${esc(L.title)}</h2>
          <div class="fw-sub">${esc(L.sub)}</div>
          <div class="fw-stat"><span class="fw-dot"></span><span id="tgCount">…</span></div>
        </div>
        <span class="fw-grow"></span>
        <button class="fw-btn" id="tgNew">＋ ${esc(L.new_btn)}</button>
      </div>
      <div id="tgForm"></div>
      <div id="tgList"><div class="fw-card"><div class="fw-empty">⟳ ${esc(L.loading)}</div></div></div>
    </div>`;
  mainEl.querySelector('#tgNew').addEventListener('click', () => openForm(mainEl, null));
  await loadRefs();
  await load(mainEl);
}

async function loadRefs() {
  // Schedule (type "time") punya menu SENDIRI → di sini cuma event PLUGIN (webhook/file-watch/…).
  try { const d = await fetchJSON('/api/triggers/types'); types = (d.types || []).filter(t => t.id !== 'time'); } catch { types = []; }
  try { const d = await fetchJSON('/api/kernel/agents'); agents = (d.plugins || []).filter(a => a.id && a.kind !== 'channel'); } catch { agents = []; }
  try { const d = await fetchJSON('/api/groups'); groups = d.groups || []; } catch { groups = []; }
}

async function load(mainEl) {
  const list = mainEl.querySelector('#tgList');
  let data;
  try { data = await fetchJSON('/api/triggers'); }
  catch (e) { list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(String(e.message || e))}</div></div>`; return; }
  const rules = (data.triggers || []).filter(r => r.type_id !== 'time'); // jadwal waktu → tab Schedule
  mainEl.querySelector('#tgCount').textContent = `${rules.length} ${L.count_label}`;
  if (!rules.length) { list.innerHTML = `<div class="fw-card"><div class="fw-empty">${esc(L.empty)}</div></div>`; return; }
  list.innerHTML = `<div class="fw-grid">${rules.map(cardHTML).join('')}</div>`;
  rules.forEach(r => wireCard(mainEl, r));
}

function typeOf(id) { return types.find(t => t.id === id) || { id, name: id, mode: 'poll' }; }
function cfgSummary(r) {
  try { const c = JSON.parse(r.config || '{}'); return Object.entries(c).map(([k, v]) => `${k}=${v}`).join(' · '); } catch { return ''; }
}

function cardHTML(r) {
  const st = r.last_status || '';
  const cls = st === 'ok' ? 'ok' : (st === 'error' || st === 'bad_config' || st === 'type_removed') ? 'error' : 'idle';
  const ty = typeOf(r.type_id);
  return `<div class="fw-card" data-id="${escAttr(r.id)}">
    <div class="fw-row">
      <h3>${r.enabled ? '🟢' : '⚪'} ${esc(r.name)}</h3>
      <span class="fw-tag">${esc(ty.name)}</span>
      <span class="fw-grow"></span>
      <span class="fw-tag tg-state ${cls}">${esc(st || L.idle)}</span>
    </div>
    <div class="fw-desc"><span class="fw-id">${esc(cfgSummary(r))} → ${esc(r.target)}</span></div>
    <div class="fw-row" style="margin-top:11px">
      <span class="fw-grow"></span>
      <button class="fw-btn" data-act="toggle">${r.enabled ? esc(L.btn_disable) : esc(L.btn_enable)}</button>
      <button class="fw-btn" data-act="run">▷ ${esc(L.btn_run)}</button>
      <button class="fw-btn" data-act="edit">✎</button>
      <button class="fw-btn" data-act="dup" title="Duplicate (new copy, disabled first)">⧉</button>
      <button class="fw-btn" data-act="hist">▸</button>
      <button class="fw-btn danger" data-act="del">🗑</button>
    </div>
    ${ty.mode === 'webhook' && r.webhook_secret ? `<div class="fw-desc fw-id" data-url style="word-break:break-all">POST ${location.origin}/api/triggers/hook/${esc(r.id)}?key=${esc(r.webhook_secret)}</div>` : ''}
    <div class="fw-desc" data-hist style="display:none"></div>
  </div>`;
}

function wireCard(mainEl, r) {
  const card = mainEl.querySelector(`.fw-card[data-id="${r.id}"]`); // id = slug server-validated, aman
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
    if (!confirm(fmt('del_confirm', { name: r.name }))) return;
    try { await fetchJSON(`/api/triggers/delete?id=${encodeURIComponent(r.id)}`, { method: 'POST' }); await load(mainEl); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="hist"]').onclick = async () => {
    const box = card.querySelector('[data-hist]');
    if (box.style.display === 'block') { box.style.display = 'none'; return; }
    box.style.display = 'block'; box.innerHTML = '…';
    try {
      const d = await fetchJSON(`/api/triggers/runs?id=${encodeURIComponent(r.id)}&limit=15`);
      const runs = d.runs || [];
      box.innerHTML = runs.length ? runs.map(x => `<div>${esc(x.fired_at)} · <b>${esc(x.status)}</b> · ${esc(x.trigger)} · ${esc((x.result_text || x.error_text || '').slice(0, 120))}</div>`).join('') : `<div>${esc(L.no_runs)}</div>`;
    } catch (e) { box.innerHTML = esc(e.message); }
  };
}

function targetOptions(sel) {
  const a = agents.map(x => `<option value="agent:${escAttr(x.id)}" ${sel === 'agent:' + x.id ? 'selected' : ''}>🤖 ${esc(x.display_name || x.id)}</option>`).join('');
  const g = groups.map(x => `<option value="group:${escAttr(x.id)}" ${sel === 'group:' + x.id ? 'selected' : ''}>👥 ${esc(x.display_name || x.id)}</option>`).join('');
  // Aksi SISTEM (owner 2026-06-20 "all compact ke triger"): jadwalin Compact All via trigger.
  const sys = `<option value="system:compact-all" ${sel === 'system:compact-all' ? 'selected' : ''}>🧠 Compact All (semua agent → brain)</option>`;
  return `<optgroup label="Agents">${a}</optgroup>${g ? `<optgroup label="Groups">${g}</optgroup>` : ''}<optgroup label="Sistem">${sys}</optgroup>`;
}

function openForm(mainEl, r) {
  const box = mainEl.querySelector('#tgForm');
  const editing = !!r;
  const cur = r || { id: '', name: '', type_id: types[0] ? types[0].id : 'webhook', config: '{}', target: '', target_kind: 'agent', prompt: '' };
  let cfg = {}; try { cfg = JSON.parse(cur.config || '{}'); } catch {}
  const selTarget = cur.target ? cur.target_kind + ':' + cur.target : '';
  box.innerHTML = `<div class="fw-card">
    <div class="fw-sec">${esc(editing ? L.edit_title : L.new_title)}</div>
    <div class="fw-grid" style="grid-template-columns:repeat(auto-fit,minmax(160px,1fr))">
      <div><div class="fw-sec">${esc(L.f_id)}</div>
        <input class="fw-input" id="fId" value="${escAttr(cur.id)}" ${editing ? 'readonly' : ''} placeholder="report-saham"></div>
      <div><div class="fw-sec">${esc(L.f_name)}</div>
        <input class="fw-input" id="fName" value="${escAttr(cur.name)}" placeholder="Report Saham A"></div>
      <div><div class="fw-sec">${esc(L.f_type)}</div>
        <select class="fw-input" id="fType">${types.map(t => `<option value="${escAttr(t.id)}" ${t.id === cur.type_id ? 'selected' : ''}>${esc(t.name)}</option>`).join('')}</select></div>
    </div>
    <div id="fCfg"></div>
    <div class="fw-sec" style="margin-top:14px">${esc(L.f_target)}</div>
    <select class="fw-input" id="fTarget">${targetOptions(selTarget)}</select>
    <div class="fw-sec" style="margin-top:14px">${esc(L.f_prompt)}</div>
    <textarea class="fw-input" id="fPrompt" style="min-height:64px;resize:vertical" placeholder="${escAttr(L.prompt_ph)}">${esc(cur.prompt)}</textarea>
    <div id="fChips"></div>
    <div class="fw-row" style="margin-top:14px">
      <button class="fw-btn" id="fSave">${esc(L.save)}</button>
      <button class="fw-btn" id="fCancel">${esc(L.cancel)}</button>
      <span class="fw-id" id="fMsg"></span>
    </div>
  </div>`;
  const renderCfg = () => {
    const ty = typeOf(box.querySelector('#fType').value);
    const fields = ty.config_schema || [];
    box.querySelector('#fCfg').innerHTML = fields.length ? fields.map(f =>
      `<div class="fw-sec" style="margin-top:14px">${esc(f.label || f.key)}${f.required ? ' *' : ''}${f.help ? ` <span style="color:var(--text-muted);font-weight:400">— ${esc(f.help)}</span>` : ''}</div>
       <input class="fw-input" data-cfg="${escAttr(f.key)}" value="${escAttr(cfg[f.key] != null ? cfg[f.key] : (f.default || ''))}">` ).join('') : '';
    box.querySelector('#fChips').innerHTML = (ty.payload_keys || []).map(k => `<span class="fw-tag" data-chip="{{${esc(k)}}}" style="cursor:pointer;margin:6px 4px 0 0;display:inline-block">{{${esc(k)}}}</span>`).join('');
    box.querySelectorAll('[data-chip]').forEach(c => c.onclick = () => {
      const ta = box.querySelector('#fPrompt'); ta.value += c.dataset.chip; ta.focus();
    });
  };
  box.querySelector('#fType').onchange = () => { cfg = {}; renderCfg(); };
  renderCfg();
  box.querySelector('#fCancel').onclick = () => { box.innerHTML = ''; };
  box.querySelector('#fSave').onclick = async () => {
    const msg = box.querySelector('#fMsg'); msg.style.color = 'var(--text-muted)';
    const conf = {};
    box.querySelectorAll('[data-cfg]').forEach(i => { const v = i.value.trim(); if (v) conf[i.getAttribute('data-cfg')] = v; });
    const tv = box.querySelector('#fTarget').value; const ti = tv.indexOf(':');
    const payload = {
      id: box.querySelector('#fId').value.trim().toLowerCase(),
      name: box.querySelector('#fName').value.trim(),
      type_id: box.querySelector('#fType').value,
      config: JSON.stringify(conf),
      target: ti >= 0 ? tv.slice(ti + 1) : tv,
      target_kind: ti >= 0 ? tv.slice(0, ti) : 'agent',
      prompt: box.querySelector('#fPrompt').value,
      enabled: true,
    };
    try {
      await fetchJSON('/api/triggers', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) });
      box.innerHTML = ''; await load(mainEl);
    } catch (e) { msg.style.color = '#f87171'; msg.textContent = L.save_fail + ' ' + (e.message || e); }
  };
}
