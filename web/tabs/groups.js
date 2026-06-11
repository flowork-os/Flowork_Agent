// groups.js — tab "Group": the ant colony.
//
// A GROUP is a team of small single-job agents + a synthesizer that fuses their
// answers. Mr.Flow delegates deep analysis to a group over the loket bus → members
// work in parallel → synthesizer merges → reply. Tiny prompt per ant → a small/local
// model can run it (anti over-prompt).
//
// Look: matches the AI Agent tab — calm glass cards, soft violet→cyan→emerald
// gradient, deterministic agent avatars for each member. No HUD animation: AI-native,
// tidy, professional. All copy via the i18n dictionary (en base + id) — no hardcode.
//
// API: /api/groups (list + available members), /api/groups/{config,create,delete}.
// Roster lives in each group's loket store, read LIVE by the wasm (no restart).

import { esc, escAttr, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('groups.' + String(k)) });
const fmt = (k, vars) => Object.entries(vars || {}).reduce((s, [n, v]) => s.replaceAll('{' + n + '}', v), L[k]);

// ── deterministic agent avatar (same vocabulary as the AI Agent tab) ──────────
const AV_PALETTES = [
  ['#7c3aed', '#a855f7'], ['#0ea5e9', '#22d3ee'], ['#10b981', '#34d399'], ['#f59e0b', '#fbbf24'],
  ['#ec4899', '#f472b6'], ['#6366f1', '#818cf8'], ['#ef4444', '#f87171'], ['#14b8a6', '#5eead4'],
];
const AV_FACES = ['🤖', '🪄', '🦊', '🛸', '👾', '🐙', '🦉', '🐉', '🐺', '🪐', '⚡', '🧊'];
function hashStr(s) {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0; }
  return h >>> 0;
}
function avatarFor(id) {
  const h = hashStr(id || 'unknown');
  return { face: AV_FACES[h % AV_FACES.length], palette: AV_PALETTES[(h >>> 8) % AV_PALETTES.length] };
}
function avatar(id, size) {
  const { face, palette } = avatarFor(id);
  return `<span class="gr-av" style="width:${size}px;height:${size}px;font-size:${Math.round(size * 0.5)}px;
    background:radial-gradient(circle at 30% 25%, ${palette[1]} 0%, ${palette[0]} 72%, #0f172a 115%);
    box-shadow:0 0 0 1px ${palette[1]}55, inset 0 0 10px rgba(255,255,255,0.08)">${face}</span>`;
}

const CSS = `
.gr-tab { padding:24px 32px 60px; color:#e2e8f0; max-width:1320px; }

/* ── hero (matches AI Agent) ── */
.gr-hero { position:relative; overflow:hidden; padding:30px 36px; border-radius:18px; margin-bottom:24px;
  background:linear-gradient(135deg, rgba(124,58,237,0.20) 0%, rgba(14,165,233,0.16) 52%, rgba(16,185,129,0.15) 100%);
  border:1px solid rgba(148,163,184,0.22); display:grid; grid-template-columns:1fr auto; align-items:center; gap:22px;
  box-shadow:0 18px 60px -30px rgba(124,58,237,0.4); }
.gr-eyebrow { font-size:0.72rem; letter-spacing:0.32em; color:#a78bfa; text-transform:uppercase; font-weight:600; margin-bottom:8px; }
.gr-h1 { margin:0; font-size:2.1rem; line-height:1.1; font-weight:700;
  background:linear-gradient(90deg,#c4b5fd,#67e8f9 55%,#6ee7b7); -webkit-background-clip:text; background-clip:text; color:transparent; }
.gr-hsub { margin:10px 0 0; color:#cbd5e1; max-width:72ch; line-height:1.55; font-size:0.95rem; }
.gr-stat { background:rgba(15,23,42,0.6); border:1px solid rgba(148,163,184,0.2); padding:14px 22px; border-radius:14px; text-align:center; min-width:104px; backdrop-filter:blur(4px); }
.gr-stat b { display:block; font-size:1.9rem; font-weight:700; color:#c4b5fd; line-height:1; }
.gr-stat label { font-size:0.66rem; letter-spacing:0.18em; text-transform:uppercase; color:#94a3b8; margin-top:5px; display:block; }

/* ── create bar ── */
.gr-create-bar { display:flex; gap:10px; flex-wrap:wrap; align-items:center; margin-bottom:24px;
  background:rgba(15,23,42,0.5); border:1px solid rgba(148,163,184,0.16); border-radius:14px; padding:14px 16px; }
.gr-create-bar .gr-in { flex:1; min-width:200px; }

/* ── inputs ── */
.gr-in, .gr-sel, .gr-task { background:rgba(2,6,18,0.55); border:1px solid rgba(148,163,184,0.2); border-radius:9px;
  color:#e2e8f0; padding:9px 12px; font:inherit; font-size:0.9rem; transition:border-color .15s; }
.gr-in:focus, .gr-sel:focus, .gr-task:focus { outline:none; border-color:#a78bfa; }
.gr-in::placeholder, .gr-task::placeholder { color:#64748b; }

/* ── grid of colony cards ── */
#grList { display:grid; grid-template-columns:repeat(auto-fill,minmax(380px,1fr)); gap:20px; align-items:start; }
.gr-card { background:rgba(15,23,42,0.6); border:1px solid rgba(148,163,184,0.18); border-radius:16px; padding:20px 22px;
  backdrop-filter:blur(4px); transition:border-color .18s, transform .18s, box-shadow .18s; }
.gr-card:hover { border-color:rgba(167,139,250,0.5); transform:translateY(-2px); box-shadow:0 16px 40px -26px rgba(124,58,237,0.5); }

.gr-head { display:flex; align-items:center; gap:13px; }
.gr-av { display:inline-flex; align-items:center; justify-content:center; border-radius:50%; flex:0 0 auto; }
.gr-head-text { flex:1; min-width:0; }
.gr-name { width:100%; font-size:1.05rem; font-weight:600; color:#f1f5f9; background:transparent; border:1px solid transparent; border-radius:7px; padding:4px 7px; margin:-4px -7px 0; }
.gr-name:hover { border-color:rgba(148,163,184,0.2); } .gr-name:focus { background:rgba(2,6,18,0.55); border-color:#a78bfa; outline:none; }
.gr-gid { font-family:ui-monospace,monospace; font-size:0.74rem; color:#64748b; margin-top:3px; padding-left:1px; }
.gr-tag { font-size:0.6rem; letter-spacing:0.16em; font-weight:700; color:#a78bfa; border:1px solid rgba(167,139,250,0.4);
  background:rgba(124,58,237,0.12); border-radius:999px; padding:3px 9px; align-self:flex-start; }

.gr-sec { font-size:0.64rem; letter-spacing:0.18em; text-transform:uppercase; color:#94a3b8; margin:18px 0 8px; }

/* ── member avatar chips ── */
.gr-members { display:flex; flex-wrap:wrap; gap:7px; }
.gr-chip { display:inline-flex; align-items:center; gap:7px; padding:4px 11px 4px 4px; border-radius:999px;
  border:1px solid rgba(148,163,184,0.2); background:rgba(2,6,18,0.4); color:#94a3b8; cursor:pointer; user-select:none;
  font-size:0.82rem; transition:all .15s; }
.gr-chip input { display:none; }
.gr-chip:hover { border-color:rgba(148,163,184,0.4); color:#cbd5e1; }
.gr-chip.on { border-color:rgba(103,232,249,0.55); background:rgba(14,165,233,0.14); color:#e2e8f0; box-shadow:0 0 0 1px rgba(103,232,249,0.25); }
.gr-chip.on .gr-av { box-shadow:0 0 0 2px rgba(103,232,249,0.6); }

.gr-task { width:100%; min-height:52px; resize:vertical; margin-top:2px; box-sizing:border-box; }
.gr-sched { font-size:0.78rem; color:#7dd3fc; margin-top:14px; display:flex; align-items:center; gap:6px; }

/* ── buttons ── */
.gr-foot { display:flex; align-items:center; gap:10px; margin-top:18px; }
.gr-btn { padding:8px 16px; border-radius:9px; font:inherit; font-size:0.84rem; font-weight:600; cursor:pointer;
  border:1px solid rgba(148,163,184,0.25); background:rgba(56,189,248,0.14); color:#7dd3fc; transition:all .15s; }
.gr-btn:hover { background:rgba(56,189,248,0.24); border-color:rgba(125,211,252,0.5); }
.gr-btn:disabled { opacity:.5; cursor:default; }
.gr-btn.primary { background:linear-gradient(90deg,#7c3aed,#0ea5e9); border-color:transparent; color:#fff; }
.gr-btn.primary:hover { filter:brightness(1.12); }
.gr-btn.danger { background:transparent; border-color:rgba(248,113,113,0.4); color:#f87171; }
.gr-btn.danger:hover { background:rgba(248,113,113,0.12); }
.gr-msg { font-size:0.8rem; }
.gr-empty { color:#64748b; font-size:0.86rem; padding:10px 0; }
`;

export async function render(mainEl) {
  loadStyle('groups', CSS);
  mainEl.innerHTML = `
    <section class="gr-tab">
      <header class="gr-hero">
        <div>
          <div class="gr-eyebrow">FLOWORK · ANT COLONY</div>
          <h1 class="gr-h1">${esc(L.title)}</h1>
          <p class="gr-hsub">${esc(L.sub)}</p>
        </div>
        <div class="gr-stat"><b id="grStatN">·</b><label>${esc(L.count_label)}</label></div>
      </header>

      <div class="gr-create-bar">
        <input class="gr-in" id="grNewId" placeholder="${escAttr(L.new_id_ph)}">
        <input class="gr-in" id="grNewName" placeholder="${escAttr(L.new_name_ph)}">
        <button class="gr-btn primary gr-create">+ ${esc(L.create_btn)}</button>
        <span class="gr-msg" id="grNewMsg" style="display:none"></span>
      </div>

      <div id="grList"><div class="gr-empty">${esc(L.loading)}</div></div>
    </section>
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
  const statN = mainEl.querySelector('#grStatN');
  if (statN) statN.textContent = groups.length;
  if (!groups.length) {
    list.innerHTML = `<div class="gr-card"><div class="gr-empty">${esc(L.empty)}</div></div>`;
    return;
  }
  // claimedBy maps an agent id → the group that already uses it (member or
  // synthesizer), so a card only offers its own organs + agents no other group
  // has claimed.
  const claimedBy = {};
  for (const g of groups) {
    for (const c of g.claims || g.members || []) claimedBy[c] = g.id;
    if (g.synthesizer) claimedBy[g.synthesizer] = g.id;
  }
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
  el.className = 'gr-card';
  const members = new Set(g.members || []);
  const pool = avail.filter((a) => members.has(a.id) || g.synthesizer === a.id || !claimedBy[a.id] || claimedBy[a.id] === g.id);
  const chips = pool.map((a) => `
    <label class="gr-chip ${members.has(a.id) ? 'on' : ''}" data-id="${escAttr(a.id)}">
      ${avatar(a.id, 22)}<span class="gr-chip-name">${esc(a.display_name || a.id)}</span>
      <input type="checkbox" ${members.has(a.id) ? 'checked' : ''}>
    </label>`).join('');
  const synthOpts = [`<option value="">${esc(L.synth_none)}</option>`]
    .concat(pool.map((a) => `<option value="${escAttr(a.id)}" ${g.synthesizer === a.id ? 'selected' : ''}>${esc(a.display_name || a.id)}</option>`))
    .join('');
  el.innerHTML = `
    <div class="gr-head">
      ${avatar(g.id, 46)}
      <div class="gr-head-text">
        <input class="gr-name" value="${escAttr(g.display_name || g.id)}" placeholder="${escAttr(L.name_ph)}">
        <div class="gr-gid">${esc(g.id)}</div>
      </div>
      <span class="gr-tag">GROUP</span>
    </div>

    <div class="gr-sec">${esc(L.members_label)}</div>
    <div class="gr-members">${chips || `<span class="gr-empty">${esc(L.no_agents)}</span>`}</div>

    <div class="gr-sec">${esc(L.synth_label)}</div>
    <select class="gr-sel gr-synth" style="width:100%">${synthOpts}</select>

    <div class="gr-sec">${esc(L.task_label)}</div>
    <textarea class="gr-task" placeholder="${escAttr(L.task_label)}">${esc(g.task || '')}</textarea>

    <div class="gr-foot">
      <button class="gr-btn primary gr-do">${esc(L.save_btn)}</button>
      <button class="gr-btn danger gr-del">${esc(L.delete_btn)}</button>
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
  const msg = el.querySelector('.gr-foot .gr-msg');
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
      msg.style.color = '#6ee7b7'; msg.textContent = '✓ ' + L.saved; msg.style.display = '';
      setTimeout(() => { msg.style.display = 'none'; }, 2500);
    } catch (e) {
      msg.style.color = '#f87171'; msg.textContent = L.save_fail + (e.message || e); msg.style.display = '';
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
    msg.style.color = '#f87171'; msg.textContent = L.id_invalid; msg.style.display = '';
    return;
  }
  btn.disabled = true; msg.style.display = 'none';
  try {
    await fetchJSON('/api/groups/create', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, display_name: nameEl.value.trim() }),
    });
    idEl.value = ''; nameEl.value = '';
    msg.style.color = '#6ee7b7'; msg.textContent = '✓ ' + L.create_ok; msg.style.display = '';
    setTimeout(() => load(mainEl), 800);
  } catch (e) {
    msg.style.color = '#f87171'; msg.textContent = L.create_fail + (e.message || e); msg.style.display = '';
  } finally {
    btn.disabled = false;
  }
}
