// groups.js — tab "Group" (§F2): the ant colony.
//
// A GROUP is a team of small single-job agents + a synthesizer that fuses their
// answers. Mr.Flow delegates deep analysis to a group over the loket bus → members
// work in parallel → synthesizer merges → reply. Tiny prompt per ant → a small/local
// model can run it (anti over-prompt).
//
// All copy goes through the i18n dictionary (en base + id) — no hardcoded strings.
// Look: "Jarvis" HUD — neon cyan, glass, scanline, corner brackets, animated colony.
//
// API: /api/groups (list + available members), /api/groups/{config,create,delete}.
// Roster lives in each group's loket store, read LIVE by the wasm (no restart).

import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('groups.' + String(k)) });
const fmt = (k, vars) => Object.entries(vars || {}).reduce((s, [n, v]) => s.replaceAll('{' + n + '}', v), L[k]);

const CSS = `
.gr-wrap{position:relative;max-width:1400px;padding:6px 2px 40px;
  --cy:#36e6ff;--cy2:#26ffd0;--ink:#06121a;--line:rgba(54,230,255,.22);--bad:#ff476f;--warn:#ffc24d}
/* group cards in a responsive grid: up to 3 across on a wide screen, auto-
   collapsing to 2 then 1 as the viewport narrows (auto-fit + min track width). */
#grList{display:grid;grid-template-columns:repeat(auto-fit,minmax(360px,1fr));gap:18px;align-items:start}
.gr-wrap::before{content:'';position:absolute;inset:0;z-index:0;pointer-events:none;opacity:.30;
  background:linear-gradient(rgba(54,230,255,.5),transparent);height:40px;animation:grscan 6s linear infinite}
@keyframes grscan{0%{transform:translateY(-40px);opacity:0}30%{opacity:1}100%{transform:translateY(640px);opacity:0}}
.gr-wrap *{position:relative;z-index:1}

/* ── HUD header with animated colony emblem ───────────────────────────── */
.gr-hud{display:flex;align-items:center;gap:18px;margin:6px 0 22px}
.gr-colony{width:64px;height:64px;flex:0 0 auto;position:relative}
.gr-core{position:absolute;top:50%;left:50%;width:16px;height:16px;margin:-8px 0 0 -8px;border-radius:50%;
  background:radial-gradient(circle,#aef6ff 0,var(--cy) 40%,transparent 72%);
  box-shadow:0 0 12px var(--cy),0 0 26px rgba(54,230,255,.55);animation:grpulse 2.4s ease-in-out infinite}
.gr-ring{position:absolute;inset:0;border-radius:50%;border:1px solid rgba(54,230,255,.18)}
.gr-ring.r2{inset:11px;border-color:rgba(38,255,208,.16)}
.gr-orbit{position:absolute;inset:0;animation:grspin 7s linear infinite}
.gr-orbit.o2{inset:11px;animation:grspin 4.6s linear infinite reverse}
.gr-ant{position:absolute;top:-3px;left:50%;width:6px;height:6px;margin-left:-3px;border-radius:50%;
  background:var(--cy2);box-shadow:0 0 8px var(--cy2)}
.gr-orbit.o2 .gr-ant{background:var(--cy);box-shadow:0 0 8px var(--cy)}
@keyframes grspin{to{transform:rotate(360deg)}}
@keyframes grpulse{0%,100%{transform:scale(1);filter:brightness(1)}50%{transform:scale(1.18);filter:brightness(1.35)}}
.gr-htext h2{margin:0;font-family:var(--disp,inherit);font-size:18px;letter-spacing:4px;color:#eafdff;
  text-shadow:0 0 12px rgba(54,230,255,.45)}
.gr-htext .sub{font-size:12px;color:var(--cy2);opacity:.8;margin-top:5px;max-width:640px;line-height:1.5}
.gr-htext .stat{font-size:10px;letter-spacing:2px;color:var(--cy2);margin-top:7px;display:flex;align-items:center;gap:7px}
.gr-dot{width:7px;height:7px;border-radius:50%;background:var(--cy2);box-shadow:0 0 8px var(--cy2);animation:grblink 1.6s ease-in-out infinite}
@keyframes grblink{0%,100%{opacity:1}50%{opacity:.25}}

/* ── panels with corner brackets ──────────────────────────────────────── */
.gr-panel{position:relative;background:rgba(4,14,22,.6);border:1px solid var(--line);border-radius:8px;
  padding:16px 18px;margin-bottom:16px;backdrop-filter:blur(3px)}
.gr-panel::before,.gr-panel::after{content:'';position:absolute;width:13px;height:13px;border:2px solid var(--cy);pointer-events:none;opacity:.75}
.gr-panel::before{top:-1px;left:-1px;border-right:0;border-bottom:0}
.gr-panel::after{bottom:-1px;right:-1px;border-left:0;border-top:0}
.gr-card h3{margin:0;display:flex;align-items:center;gap:9px}
.gr-tag{font-size:10px;letter-spacing:2px;color:var(--cy);border:1px solid var(--line);border-radius:3px;padding:2px 7px;background:rgba(54,230,255,.06)}
.gr-gid{font-size:11px;color:rgba(54,230,255,.5);font-family:monospace;margin:6px 0 2px}
.gr-sec{margin:14px 0 7px;font-size:10px;letter-spacing:2px;color:var(--cy);text-shadow:0 0 8px rgba(54,230,255,.3)}

/* ── inputs / chips / buttons ─────────────────────────────────────────── */
.gr-in,.gr-sel,.gr-task{background:rgba(2,10,16,.85);border:1px solid var(--line);border-radius:4px;color:var(--cy2);
  padding:8px 11px;font:inherit;font-size:13px}
.gr-in:focus,.gr-sel:focus,.gr-task:focus{outline:none;border-color:var(--cy);box-shadow:0 0 0 1px var(--cy),0 0 16px rgba(54,230,255,.22)}
.gr-name{font-size:15px;font-weight:700;color:#eafdff;min-width:240px}
.gr-task{width:100%;min-height:56px;resize:vertical;margin-top:6px}
.gr-row{display:flex;gap:11px;align-items:center;flex-wrap:wrap;margin-top:8px}
.gr-members{display:flex;flex-wrap:wrap;gap:8px}
.gr-chip{display:inline-flex;align-items:center;gap:7px;padding:6px 11px;border-radius:5px;border:1px solid var(--line);
  background:rgba(2,10,16,.6);font-size:13px;color:var(--cy2);cursor:pointer;user-select:none;transition:all .15s}
.gr-chip input{accent-color:var(--cy)}
.gr-chip.on{border-color:var(--cy);background:rgba(54,230,255,.12);color:#eafdff;box-shadow:0 0 12px rgba(54,230,255,.2)}
.gr-btn{padding:8px 15px;border-radius:4px;background:rgba(2,10,16,.8);border:1px solid var(--line);color:var(--cy);
  cursor:pointer;font:inherit;font-size:12px;letter-spacing:1.5px;font-weight:600;transition:all .15s}
.gr-btn:hover{border-color:var(--cy);color:#eafdff;background:rgba(54,230,255,.08);box-shadow:0 0 14px rgba(54,230,255,.3)}
.gr-btn:disabled{opacity:.45;cursor:default}
.gr-btn.primary{border-color:var(--cy);color:#001a22;background:linear-gradient(90deg,var(--cy),var(--cy2));font-weight:700}
.gr-btn.danger{border-color:rgba(255,71,111,.5);color:var(--bad)}
.gr-btn.danger:hover{background:rgba(255,71,111,.1);box-shadow:0 0 14px rgba(255,71,111,.3)}
.gr-save{margin-top:15px;display:flex;align-items:center;gap:12px}
.gr-msg{font-size:12px;color:var(--cy2)}
.gr-empty{color:rgba(54,230,255,.5);font-size:13px;padding:12px 0}
`;

export async function render(mainEl) {
  loadStyle('groups', CSS);
  const orbits = (n, cls) => Array.from({ length: n }, (_, i) =>
    `<div class="gr-orbit ${cls}" style="transform:rotate(${(360 / n) * i}deg)"><div class="gr-ant"></div></div>`).join('');
  mainEl.innerHTML = `
    <div class="gr-wrap">
      <div class="gr-hud">
        <div class="gr-colony">
          <div class="gr-ring"></div><div class="gr-ring r2"></div>
          ${orbits(3, '')}${orbits(2, 'o2')}
          <div class="gr-core"></div>
        </div>
        <div class="gr-htext">
          <h2>${esc(L.title)}</h2>
          <div class="sub">${esc(L.sub)}</div>
          <div class="stat"><span class="gr-dot"></span><span id="grStat">${esc(L.status_online)}</span></div>
        </div>
      </div>

      <div class="gr-panel">
        <div class="gr-sec" style="margin-top:0">${esc(L.new_title)}</div>
        <div class="gr-row">
          <input class="gr-in" id="grNewId" placeholder="${escAttr(L.new_id_ph)}" style="min-width:280px">
          <input class="gr-in" id="grNewName" placeholder="${escAttr(L.new_name_ph)}" style="min-width:220px">
          <button class="gr-btn primary gr-create">+ ${esc(L.create_btn)}</button>
          <span class="gr-msg" id="grNewMsg" style="display:none"></span>
        </div>
      </div>

      <div id="grList"><div class="gr-empty">⟳ ${esc(L.loading)}</div></div>
    </div>
  `;
  mainEl.querySelector('.gr-create').addEventListener('click', () => createGroup(mainEl));
  await load(mainEl);
}

async function load(mainEl) {
  const list = mainEl.querySelector('#grList');
  let data;
  try {
    data = await fetchJSON('/api/groups');
  } catch (e) {
    list.innerHTML = `<div class="gr-empty">${esc(L.create_fail)}${esc(String(e.message || e))}</div>`;
    return;
  }
  const groups = data.groups || [];
  const avail = data.available_agents || [];
  const stat = mainEl.querySelector('#grStat');
  if (stat) stat.textContent = `${L.status_online} · ${groups.length} ${L.count_label}`;
  if (!groups.length) {
    list.innerHTML = `<div class="gr-panel"><div class="gr-empty">${esc(L.empty)}</div></div>`;
    return;
  }
  // claimedBy maps an agent id → the group that already uses it (member or
  // synthesizer). A card then shows only ITS OWN organs + agents no other group
  // has claimed — so an agent that is not checked into this group is genuinely
  // free to add, never a leftover from another team cluttering the picker.
  const claimedBy = {};
  // 1) explicit: every organ in a group's roster (members + synth + aux roles).
  for (const g of groups) {
    for (const c of g.claims || g.members || []) claimedBy[c] = g.id;
    if (g.synthesizer) claimedBy[g.synthesizer] = g.id;
  }
  // 2) backstop: an organ named with a group's id prefix (e.g. thinking-caster)
  //    belongs to that group even if it isn't wired into the roster yet.
  for (const a of avail) {
    if (claimedBy[a.id]) continue;
    for (const g of groups) {
      if (a.id.startsWith(g.id + '-')) { claimedBy[a.id] = g.id; break; }
    }
  }
  list.innerHTML = '';
  for (const g of groups) list.appendChild(card(g, avail, claimedBy, mainEl));
}

function card(g, avail, claimedBy, mainEl) {
  const el = document.createElement('div');
  el.className = 'gr-panel gr-card';
  const members = new Set(g.members || []);
  // Show an agent only if it belongs to THIS group or is unclaimed by any other.
  const pool = avail.filter((a) => members.has(a.id) || g.synthesizer === a.id || !claimedBy[a.id] || claimedBy[a.id] === g.id);
  const chips = pool.map((a) => `
    <label class="gr-chip ${members.has(a.id) ? 'on' : ''}" data-id="${escAttr(a.id)}">
      <input type="checkbox" ${members.has(a.id) ? 'checked' : ''}> ${esc(a.display_name || a.id)}
    </label>`).join('');
  const synthOpts = [`<option value="">${esc(L.synth_none)}</option>`]
    .concat(pool.map((a) => `<option value="${escAttr(a.id)}" ${g.synthesizer === a.id ? 'selected' : ''}>${esc(a.display_name || a.id)}</option>`))
    .join('');
  el.innerHTML = `
    <h3>
      <input class="gr-sel gr-name" value="${escAttr(g.display_name || g.id)}" placeholder="${escAttr(L.name_ph)}">
      <span class="gr-tag">GROUP</span>
    </h3>
    <div class="gr-gid">${esc(g.id)}</div>

    <div class="gr-sec">${esc(L.members_label)}</div>
    <div class="gr-members">${chips || `<span class="gr-empty">${esc(L.no_agents)}</span>`}</div>

    <div class="gr-row">
      <div>
        <div class="gr-sec" style="margin-top:0">${esc(L.synth_label)}</div>
        <select class="gr-sel gr-synth">${synthOpts}</select>
      </div>
    </div>

    <div class="gr-sec">${esc(L.task_label)}</div>
    <textarea class="gr-task">${esc(g.task || '')}</textarea>

    <div class="gr-save">
      <button class="gr-btn primary gr-do">${esc(L.save_btn)}</button>
      <button class="gr-btn danger gr-del">🗑 ${esc(L.delete_btn)}</button>
      <span class="gr-msg" style="display:none"></span>
    </div>
  `;
  el.querySelectorAll('.gr-chip input').forEach((inp) =>
    inp.addEventListener('change', () => inp.closest('.gr-chip').classList.toggle('on', inp.checked)));

  el.querySelector('.gr-del').addEventListener('click', async () => {
    if (!confirm(fmt('delete_confirm', { name: g.display_name || g.id }))) return;
    try {
      await fetchJSON(`/api/groups/delete?id=${encodeURIComponent(g.id)}`, { method: 'POST' });
      setTimeout(() => load(mainEl), 600);
    } catch (e) { alert(L.delete_fail + (e.message || e)); }
  });

  const btn = el.querySelector('.gr-do');
  const msg = el.querySelector('.gr-save .gr-msg');
  btn.addEventListener('click', async () => {
    const chosen = [...el.querySelectorAll('.gr-chip input:checked')].map((i) => i.closest('.gr-chip').dataset.id);
    const payload = {
      members: chosen,
      synthesizer: el.querySelector('.gr-synth').value,
      task: el.querySelector('.gr-task').value,
      display_name: el.querySelector('.gr-name').value.trim(),
    };
    btn.disabled = true; msg.style.display = 'none';
    try {
      await fetchJSON(`/api/groups/config?id=${encodeURIComponent(g.id)}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload),
      });
      msg.style.color = 'var(--cy2)'; msg.textContent = '✓ ' + L.saved; msg.style.display = '';
      setTimeout(() => { msg.style.display = 'none'; }, 2500);
    } catch (e) {
      msg.style.color = 'var(--bad)'; msg.textContent = L.save_fail + (e.message || e); msg.style.display = '';
    } finally {
      btn.disabled = false;
    }
  });
  return el;
}

async function createGroup(mainEl) {
  const idEl = mainEl.querySelector('#grNewId');
  const nameEl = mainEl.querySelector('#grNewName');
  const msg = mainEl.querySelector('#grNewMsg');
  const btn = mainEl.querySelector('.gr-create');
  const id = (idEl.value || '').trim().toLowerCase();
  if (!/^[a-z0-9][a-z0-9-]{1,39}$/.test(id)) {
    msg.style.color = 'var(--bad)'; msg.textContent = L.id_invalid; msg.style.display = '';
    return;
  }
  btn.disabled = true; msg.style.display = 'none';
  try {
    await fetchJSON('/api/groups/create', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, display_name: nameEl.value.trim() }),
    });
    idEl.value = ''; nameEl.value = '';
    msg.style.color = 'var(--cy2)'; msg.textContent = '✓ ' + L.create_ok; msg.style.display = '';
    setTimeout(() => load(mainEl), 800);
  } catch (e) {
    msg.style.color = 'var(--bad)'; msg.textContent = L.create_fail + (e.message || e); msg.style.display = '';
  } finally {
    btn.disabled = false;
  }
}
