// schedule.js — tab "Schedule" (fitur CORE, dipisah dari Trigger). Jadwal waktu: cron → suruh
// <agent/group> dgn <prompt> → kirim Telegram. Backend = trigger engine type "time" (udah teruji),
// tapi MENU sendiri biar ga ketuker sama Trigger (event plugin). Tema Matrix × Jarvis.
import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('schedule.' + String(k)) });
const TYPE = 'time'; // Schedule = SATU tipe core (cron). Tipe event lain → tab Trigger.

// preset cron umum (label lewat i18n, value cron baku).
const PRESETS = [
  { k: 'hourly', cron: '0 * * * *' },
  { k: 'daily9', cron: '0 9 * * *' },
  { k: 'weekday9', cron: '0 9 * * 1-5' },
  { k: 'weekly_mon', cron: '0 9 * * 1' },
];

const CSS = `
.sc{--mx:#22ff88;--cy:#36e6ff;--bad:#ff476f;--warn:#ffc24d;--line:rgba(34,255,136,.20);
  position:relative;max-width:1040px;padding:6px 2px 50px;color:#bdf5d6;
  font-family:ui-monospace,'JetBrains Mono','Cascadia Code',Consolas,monospace}
.sc::before{content:'';position:absolute;left:0;right:0;top:0;height:90px;z-index:0;pointer-events:none;opacity:.5;
  background:linear-gradient(180deg,rgba(54,230,255,.10),transparent);animation:scscan 6s linear infinite}
@keyframes scscan{0%{transform:translateY(-30px);opacity:0}30%{opacity:1}100%{transform:translateY(560px);opacity:0}}
.sc>*{position:relative;z-index:1}
.sc-hud{display:flex;align-items:center;gap:16px;margin:6px 0 20px}
.sc-orb{width:42px;height:42px;flex:0 0 auto;position:relative}
.sc-core{position:absolute;inset:38%;border-radius:50%;background:radial-gradient(circle,#aef6ff,var(--cy) 45%,transparent 72%);
  box-shadow:0 0 12px var(--cy),0 0 26px rgba(54,230,255,.5);animation:scpulse 2.4s ease-in-out infinite}
.sc-ring{position:absolute;inset:0;border-radius:50%;border:1px solid var(--line);animation:scspin 12s linear infinite}
.sc-ring::after{content:'';position:absolute;top:-2px;left:50%;width:3px;height:3px;border-radius:50%;background:var(--cy);box-shadow:0 0 8px var(--cy)}
@keyframes scspin{to{transform:rotate(360deg)}}@keyframes scpulse{0%,100%{transform:scale(1)}50%{transform:scale(1.2);filter:brightness(1.3)}}
.sc-h h2{margin:0;font-size:18px;letter-spacing:4px;color:#eafdff;text-shadow:0 0 12px rgba(54,230,255,.5)}
.sc-h .sub{font-size:12px;color:var(--cy);opacity:.8;margin-top:4px}
.sc-stat{font-size:10px;letter-spacing:2px;color:var(--cy);margin-top:6px;display:flex;align-items:center;gap:7px}
.sc-dot{width:7px;height:7px;border-radius:50%;background:var(--cy);box-shadow:0 0 8px var(--cy);animation:scblink 1.6s infinite}
@keyframes scblink{0%,100%{opacity:1}50%{opacity:.25}}
.sc-btn{padding:8px 15px;border-radius:5px;background:rgba(54,230,255,.08);border:1px solid var(--line);color:var(--cy);
  cursor:pointer;font:inherit;font-size:12px;letter-spacing:1px;font-weight:600;transition:.15s}
.sc-btn:hover{border-color:var(--cy);color:#eafdff;background:rgba(54,230,255,.16);box-shadow:0 0 14px rgba(54,230,255,.3)}
.sc-btn.primary{background:linear-gradient(90deg,var(--cy),var(--mx));color:#001a14;font-weight:700;border-color:var(--cy)}
.sc-btn.danger{color:var(--bad);border-color:rgba(255,71,111,.4)}.sc-btn.danger:hover{background:rgba(255,71,111,.12)}
.sc-btn.sm{padding:4px 10px;font-size:11px}
.sc-panel{position:relative;background:rgba(4,16,11,.6);border:1px solid var(--line);border-radius:8px;padding:14px 16px;margin-bottom:14px;backdrop-filter:blur(3px)}
.sc-panel::before,.sc-panel::after{content:'';position:absolute;width:12px;height:12px;border:2px solid var(--cy);opacity:.7;pointer-events:none}
.sc-panel::before{top:-1px;left:-1px;border-right:0;border-bottom:0}.sc-panel::after{bottom:-1px;right:-1px;border-left:0;border-top:0}
.sc-card{display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.sc-name{font-size:15px;color:#eafdff;font-weight:700}
.sc-cron{font-size:11px;letter-spacing:1px;color:var(--cy);border:1px solid var(--line);border-radius:3px;padding:2px 8px;background:rgba(54,230,255,.06)}
.sc-arrow{font-size:11px;color:#7fb9a6}
.sc-grow{flex:1 1 auto}
.sc-state{font-size:10px;letter-spacing:1px;padding:3px 9px;border-radius:3px}
.sc-state.ok{color:var(--mx);border:1px solid rgba(34,255,136,.4);background:rgba(34,255,136,.07)}
.sc-state.error{color:var(--bad);border:1px solid rgba(255,71,111,.35);background:rgba(255,71,111,.06)}
.sc-state.idle{color:var(--warn);border:1px solid rgba(255,194,77,.3)}
.sc-sw{cursor:pointer;font-size:11px;color:var(--cy)}
.sc-lbl{font-size:10px;letter-spacing:2px;color:var(--cy);text-transform:uppercase;margin:10px 0 4px}
.sc-row{display:flex;gap:10px;flex-wrap:wrap;align-items:center}
.sc-in,.sc-sel,.sc-ta{width:100%;box-sizing:border-box;background:rgba(2,12,8,.85);border:1px solid var(--line);border-radius:5px;
  color:#dffaf0;padding:9px 11px;font:inherit;font-size:13px;outline:none}
.sc-in:focus,.sc-sel:focus,.sc-ta:focus{border-color:var(--cy);box-shadow:0 0 0 1px var(--cy)}
.sc-ta{min-height:64px;resize:vertical}
.sc-pre{display:inline-block;font-size:11px;color:var(--cy);border:1px solid var(--line);border-radius:3px;padding:3px 9px;margin:3px 5px 0 0;cursor:pointer;background:rgba(54,230,255,.06)}
.sc-pre:hover{background:rgba(54,230,255,.16);border-color:var(--cy)}
.sc-pre.on{background:var(--cy);color:#001a14;font-weight:700}
.sc-chip{display:inline-block;font-size:11px;color:var(--mx);border:1px solid var(--line);border-radius:3px;padding:2px 7px;margin:3px 4px 0 0;cursor:pointer;background:rgba(34,255,136,.06)}
.sc-msg{font-size:12px;margin-top:8px;min-height:15px}.sc-msg.ok{color:var(--mx)}.sc-msg.err{color:var(--bad)}
.sc-empty{color:var(--cy);opacity:.65;text-align:center;padding:16px;font-size:13px}
.sc-hist{margin-top:8px;font-size:11px;color:#9fd9c4}.sc-hist .r{padding:4px 0;border-bottom:1px solid rgba(54,230,255,.08)}
`;

let agents = [], groups = [];

export async function render(mainEl) {
  loadStyle('schedule', CSS);
  mainEl.innerHTML = `
    <div class="sc">
      <div class="sc-hud">
        <div class="sc-orb"><div class="sc-ring"></div><div class="sc-core"></div></div>
        <div class="sc-h"><h2>⏰ ${esc(L.title)}</h2><div class="sub">${esc(L.sub)}</div>
          <div class="sc-stat"><span class="sc-dot"></span><span id="scCount">…</span></div></div>
        <span class="sc-grow"></span>
        <button class="sc-btn primary" id="scNew">＋ ${esc(L.new_btn)}</button>
      </div>
      <div id="scForm"></div>
      <div id="scList"><div class="sc-empty">⟳ ${esc(L.loading)}</div></div>
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
  catch (e) { list.innerHTML = `<div class="sc-empty">${esc(String(e.message || e))}</div>`; return; }
  const rules = (data.triggers || []).filter(r => r.type_id === TYPE); // CUMA jadwal waktu
  mainEl.querySelector('#scCount').textContent = `${rules.length} ${L.count_label}`;
  if (!rules.length) { list.innerHTML = `<div class="sc-panel"><div class="sc-empty">${esc(L.empty)}</div></div>`; return; }
  list.innerHTML = rules.map(cardHTML).join('');
  rules.forEach(r => wireCard(mainEl, r));
}

function cronOf(r) { try { return JSON.parse(r.config || '{}').cron || ''; } catch { return ''; } }

function cardHTML(r) {
  const st = r.last_status || '';
  const cls = st === 'ok' ? 'ok' : (st === 'error' || st === 'bad_config') ? 'error' : 'idle';
  return `<div class="sc-panel" data-id="${escAttr(r.id)}">
    <div class="sc-card">
      <span class="sc-name">${r.enabled ? '🟢' : '⚪'} ${esc(r.name)}</span>
      <span class="sc-cron">⏱ ${esc(cronOf(r))}</span>
      <span class="sc-arrow">→ ${esc(r.target)}</span>
      <span class="sc-grow"></span>
      <span class="sc-state ${cls}">${esc(st || L.idle)}</span>
      <span class="sc-sw" data-act="toggle">${r.enabled ? esc(L.btn_disable) : esc(L.btn_enable)}</span>
      <button class="sc-btn sm" data-act="run">▷ ${esc(L.btn_run)}</button>
      <button class="sc-btn sm" data-act="edit">✎</button>
      <button class="sc-btn sm" data-act="hist">▸</button>
      <button class="sc-btn sm danger" data-act="del">🗑</button>
    </div>
    <div class="sc-hist" data-hist style="display:none"></div>
  </div>`;
}

function wireCard(mainEl, r) {
  const card = mainEl.querySelector(`.sc-panel[data-id="${r.id}"]`); // id = slug server-validated
  if (!card) return;
  card.querySelector('[data-act="toggle"]').onclick = async () => {
    try { await fetchJSON(`/api/triggers/toggle?id=${encodeURIComponent(r.id)}&enabled=${r.enabled ? 0 : 1}`, { method: 'POST' }); await load(mainEl); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="run"]').onclick = async () => {
    try { await fetchJSON(`/api/triggers/run?id=${encodeURIComponent(r.id)}`, { method: 'POST' }); alert(L.run_ok); } catch (e) { alert(e.message); }
  };
  card.querySelector('[data-act="edit"]').onclick = () => openForm(mainEl, r);
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
      box.innerHTML = runs.length ? runs.map(x => `<div class="r">${esc(x.fired_at)} · <b>${esc(x.status)}</b> · ${esc((x.result_text || x.error_text || '').slice(0, 120))}</div>`).join('') : `<div class="r">${esc(L.no_runs)}</div>`;
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
  box.innerHTML = `<div class="sc-panel">
    <div class="sc-lbl">${esc(editing ? L.edit_title : L.new_title)}</div>
    <div class="sc-row">
      <div style="flex:1;min-width:160px"><div class="sc-lbl">${esc(L.f_id)}</div>
        <input class="sc-in" id="fId" value="${escAttr(cur.id)}" ${editing ? 'readonly' : ''} placeholder="report-pagi"></div>
      <div style="flex:1;min-width:160px"><div class="sc-lbl">${esc(L.f_name)}</div>
        <input class="sc-in" id="fName" value="${escAttr(cur.name)}" placeholder="Report Pagi"></div>
    </div>
    <div class="sc-lbl">${esc(L.f_when)}</div>
    <div id="fPresets">${PRESETS.map(p => `<span class="sc-pre" data-cron="${p.cron}">${esc(L['preset_' + p.k])}</span>`).join('')}</div>
    <div class="sc-row" style="margin-top:6px;align-items:center">
      <input class="sc-in" id="fCron" style="flex:1;min-width:160px;font-family:ui-monospace" value="${escAttr(cron)}" placeholder="0 9 * * *">
      <span style="font-size:10px;color:#7fb9a6">${esc(L.cron_help)}</span>
    </div>
    <div class="sc-lbl">${esc(L.f_target)}</div>
    <select class="sc-sel" id="fTarget">${targetOptions(selTarget)}</select>
    <div class="sc-lbl">${esc(L.f_prompt)}</div>
    <textarea class="sc-ta" id="fPrompt" placeholder="${escAttr(L.prompt_ph)}">${esc(cur.prompt)}</textarea>
    <div id="fChips"><span class="sc-chip" data-chip="{{time}}">{{time}}</span><span class="sc-chip" data-chip="{{date}}">{{date}}</span></div>
    <div class="sc-row" style="margin-top:12px">
      <button class="sc-btn primary" id="fSave">${esc(L.save)}</button>
      <button class="sc-btn" id="fCancel">${esc(L.cancel)}</button>
      <span class="sc-msg" id="fMsg"></span>
    </div>
  </div>`;
  const cronIn = box.querySelector('#fCron');
  const syncPresets = () => box.querySelectorAll('[data-cron]').forEach(p => p.classList.toggle('on', p.dataset.cron === cronIn.value.trim()));
  box.querySelectorAll('[data-cron]').forEach(p => p.onclick = () => { cronIn.value = p.dataset.cron; syncPresets(); });
  cronIn.oninput = syncPresets; syncPresets();
  box.querySelectorAll('[data-chip]').forEach(c => c.onclick = () => { const ta = box.querySelector('#fPrompt'); ta.value += c.dataset.chip; ta.focus(); });
  box.querySelector('#fCancel').onclick = () => { box.innerHTML = ''; };
  box.querySelector('#fSave').onclick = async () => {
    const msg = box.querySelector('#fMsg'); msg.className = 'sc-msg';
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
    } catch (e) { msg.className = 'sc-msg err'; msg.textContent = L.save_fail + ' ' + (e.message || e); }
  };
}
