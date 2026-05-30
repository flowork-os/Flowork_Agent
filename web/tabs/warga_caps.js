// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Tool Registry tab (reference verbatim 272 LOC). Audit pass — esc() on tool name+desc, checkbox subscribe, dataset id sanitized..

// warga_caps.js — Warga Capability Matrix tab (rc174)
// Hybrid role-default + per-warga override toggle UI.
//
// Layout: card besar per-warga (2-col grid responsive). Each card berisi:
// - Header: avatar + name + role + active toggle
// - Body: matrix checkbox grid (rows=category, cols=tools)
// - Footer: actions (Reset to role default, Copy from <warga>)
//
// Top: search bar + role filter + view toggle (per-warga / per-role).
import { esc, fetchJSON } from '../js/utils.js';

let CATALOG = [];   // [{category, tools[]}]
let ROLES = [];     // [role names]
let WARGA_LIST = []; // [{name, role, display_name, active}]
let CAPS_CACHE = {}; // {wargaName: {tool: {enabled, is_override}}}

function ensureStyle() {
  if (document.getElementById('wargaCapsStyle')) return;
  const s = document.createElement('style');
  s.id = 'wargaCapsStyle';
  s.textContent = `
    .wc-toolbar { display:flex; gap:.6rem; align-items:center; margin-bottom:1rem; flex-wrap:wrap; padding:.5rem .8rem; background:rgba(255,255,255,.04); border-radius:8px; }
    .wc-toolbar input, .wc-toolbar select { padding:.4rem .7rem; background:rgba(0,0,0,.3); border:1px solid rgba(255,255,255,.1); border-radius:6px; color:#fff; font-size:.9rem; }
    .wc-toolbar input { min-width: 220px; }
    .wc-toolbar button { padding:.4rem .9rem; background:rgba(168,85,247,.25); border:1px solid rgba(168,85,247,.5); border-radius:6px; color:#fff; cursor:pointer; font-size:.9rem; }
    .wc-toolbar button:hover { background:rgba(168,85,247,.4); }
    .wc-grid { display:grid; grid-template-columns: repeat(auto-fill, minmax(540px, 1fr)); gap:1rem; }
    .wc-card { background:rgba(255,255,255,.04); border:1px solid rgba(255,255,255,.08); border-radius:10px; padding:1rem 1.2rem; min-width: 0; }
    .wc-card-header { display:flex; justify-content:space-between; align-items:center; margin-bottom:.8rem; padding-bottom:.6rem; border-bottom:1px solid rgba(255,255,255,.08); }
    .wc-card-title { font-weight:600; font-size:1rem; }
    .wc-card-meta { font-size:.78rem; color:rgba(255,255,255,.6); margin-top:.2rem; }
    .wc-card-body { display:grid; grid-template-columns: 1fr; gap:.6rem; }
    .wc-cat { background:rgba(0,0,0,.15); border-radius:6px; padding:.55rem .7rem; }
    .wc-cat-title { font-size:.78rem; font-weight:600; text-transform:uppercase; color:rgba(255,255,255,.55); margin-bottom:.45rem; letter-spacing:.04em; }
    .wc-tools { display:grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap:.3rem .5rem; }
    .wc-tool { display:flex; align-items:center; gap:.4rem; font-size:.83rem; padding:.2rem .35rem; border-radius:4px; cursor:pointer; user-select:none; }
    .wc-tool:hover { background:rgba(255,255,255,.05); }
    .wc-tool input[type="checkbox"] { width:14px; height:14px; cursor:pointer; }
    /* Bug Gemini #58 fix (2026-04-27): override indicator dipertegas — sebelumnya
       cuma color shift + small asterisk, mudah ke-miss visual. Sekarang tambah
       background tint + border highlight + asterisk lebih besar supaya jelas
       inherited vs manual override. */
    .wc-tool.override {
      color:#a78bfa;
      font-weight:600;
      background:rgba(168,85,247,.12);
      border:1px solid rgba(168,85,247,.35);
      padding-right:.45rem;
    }
    .wc-tool.override::after {
      content:' *';
      color:#fbbf24;
      font-weight:bold;
      font-size:1.1em;
      margin-left:2px;
    }
    .wc-card-footer { display:flex; gap:.5rem; margin-top:.8rem; padding-top:.6rem; border-top:1px solid rgba(255,255,255,.08); }
    .wc-card-footer button { font-size:.78rem; padding:.3rem .7rem; background:rgba(255,255,255,.06); border:1px solid rgba(255,255,255,.1); border-radius:5px; color:#ddd; cursor:pointer; }
    .wc-card-footer button:hover { background:rgba(255,255,255,.12); }
    .wc-status { font-size:.85rem; color:rgba(255,255,255,.7); padding:.5rem .8rem; }
    .wc-empty { text-align:center; color:rgba(255,255,255,.5); padding:2rem; font-style:italic; }
    .wc-active { font-size:.7rem; padding:.15rem .4rem; border-radius:4px; }
    .wc-active.on { background:rgba(34,197,94,.2); color:#86efac; }
    .wc-active.off { background:rgba(239,68,68,.2); color:#fca5a5; }
    .wc-role-pill { font-size:.7rem; padding:.15rem .5rem; background:rgba(168,85,247,.2); color:#d8b4fe; border-radius:4px; margin-left:.5rem; }
  `;
  document.head.appendChild(s);
}

export async function render(mainEl) {
  ensureStyle();
  mainEl.innerHTML = `
    <h2>🛂 Hak & Tools per Warga</h2>
    <p>Toggle tools yang di-grant ke setiap warga (300+ ada). <code>*</code> = override per-individu (revert jadi role default kalau di-clear). Hybrid: role default + per-warga override.</p>
    <div class="wc-toolbar">
      <input id="wcSearch" placeholder="🔍 Cari warga (nama atau role)..." title="Cari Warga — Ketik nama warga atau role untuk menyaring daftar yang ditampilkan. Pencarian bersifat real-time dan langsung memfilter kartu warga yang cocok." />
      <select id="wcRoleFilter" title="Filter Berdasarkan Role — Pilih role warga AI untuk menyaring tampilan. Pilih 'Semua role' untuk melihat seluruh warga tanpa filter. Role menentukan hak akses tool default yang dimiliki warga."><option value="">Semua role</option></select>
      <select id="wcActiveFilter" title="Filter Berdasarkan Status Aktif — Saring warga berdasarkan apakah mereka sedang aktif atau tidak aktif. Warga tidak aktif tidak memproses tugas meskipun punya tool access.">
        <option value="all">Semua status</option>
        <option value="active">Active only</option>
        <option value="inactive">Inactive only</option>
      </select>
      <button id="wcSeed" title="Seed Hak Akses Default — Isi hak akses tool untuk semua warga berdasarkan role masing-masing. Operasi ini aman (idempotent) — jika Ayah sudah mengatur override manual, tidak akan ditimpa. Jalankan ini saat ada warga baru ditambahkan.">🌱 Seed Default</button>
      <span id="wcStatus" class="wc-status"></span>
    </div>
    <div id="wcGrid" class="wc-grid"><div class="wc-empty">Loading...</div></div>
  `;

  await loadInitial();
  bindToolbar();
  await renderCards();
}

async function loadInitial() {
  setStatus('Loading catalog...');
  try {
    const cat = await fetchJSON('/api/warga-caps/catalog');
    CATALOG = cat.catalog || [];
    ROLES = (cat.roles || []).sort();
    setStatus('Loading warga list...');
    const wg = await fetchJSON('/api/warga-caps/warga');
    WARGA_LIST = wg.warga || [];
    // Populate role filter
    const sel = document.getElementById('wcRoleFilter');
    if (sel) {
      const roles = [...new Set([...ROLES, ...WARGA_LIST.map(w => w.role)])].filter(r => r).sort();
      sel.innerHTML = '<option value="">Semua role</option>' + roles.map(r => `<option value="${esc(r)}">${esc(r)}</option>`).join('');
    }
    setStatus(`${WARGA_LIST.length} warga loaded`);
  } catch (e) {
    setStatus(`❌ Load failed: ${e.message}`);
    throw e;
  }
}

function bindToolbar() {
  document.getElementById('wcSearch')?.addEventListener('input', debounce(renderCards, 200));
  document.getElementById('wcRoleFilter')?.addEventListener('change', renderCards);
  document.getElementById('wcActiveFilter')?.addEventListener('change', renderCards);
  document.getElementById('wcSeed')?.addEventListener('click', async () => {
    if (!confirm('Seed default role capabilities? Idempotent — override Ayah ga di-overwrite.')) return;
    setStatus('Seeding...');
    try {
      const sr = await fetch('/api/warga-caps/seed', { method: 'POST' });
      if (!sr.ok) { const t = await sr.text().catch(() => sr.statusText); setStatus(`❌ Seed failed: ${t}`); return; }
      setStatus('✅ Seed done — refresh');
      CAPS_CACHE = {};
      await renderCards();
    } catch (e) { setStatus(`❌ Seed failed: ${e.message}`); }
  });
}

function debounce(fn, ms) {
  let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); };
}

function setStatus(msg) {
  const el = document.getElementById('wcStatus');
  if (el) el.textContent = msg;
}

async function renderCards() {
  const search = (document.getElementById('wcSearch')?.value || '').toLowerCase().trim();
  const roleFilter = document.getElementById('wcRoleFilter')?.value || '';
  const activeFilter = document.getElementById('wcActiveFilter')?.value || 'all';
  const grid = document.getElementById('wcGrid');
  if (!grid) return;

  const filtered = WARGA_LIST.filter(w => {
    if (roleFilter && w.role !== roleFilter) return false;
    if (activeFilter === 'active' && !w.active) return false;
    if (activeFilter === 'inactive' && w.active) return false;
    if (search && !(w.name.toLowerCase().includes(search) || w.role.toLowerCase().includes(search) || (w.display_name||'').toLowerCase().includes(search))) return false;
    return true;
  });

  setStatus(`${filtered.length} warga shown (dari ${WARGA_LIST.length} total)`);

  if (filtered.length === 0) {
    grid.innerHTML = '<div class="wc-empty">Ga ada warga match filter</div>';
    return;
  }

  // Render skeleton cards instantly, fill caps async
  grid.innerHTML = filtered.slice(0, 100).map(w => renderSkeletonCard(w)).join('');
  if (filtered.length > 100) {
    grid.innerHTML += `<div class="wc-empty">... +${filtered.length - 100} warga lain (filter lebih ketat untuk lihat)</div>`;
  }

  // Fetch caps for each visible warga in parallel (limited)
  const visible = filtered.slice(0, 100);
  await Promise.all(visible.map(w => loadAndPaintCaps(w.name)));
}

function renderSkeletonCard(w) {
  const activeClass = w.active ? 'on' : 'off';
  const activeLbl = w.active ? 'AKTIF' : 'OFF';
  return `
    <div class="wc-card" data-warga="${esc(w.name)}">
      <div class="wc-card-header">
        <div>
          <div class="wc-card-title">${esc(w.display_name || w.name)}<span class="wc-role-pill">${esc(w.role)}</span></div>
          <div class="wc-card-meta">@${esc(w.name)}</div>
        </div>
        <span class="wc-active ${activeClass}">${activeLbl}</span>
      </div>
      <div class="wc-card-body" id="wcBody-${esc(w.name)}"><div class="wc-empty" style="padding:1rem">Loading caps...</div></div>
      <div class="wc-card-footer">
        <button onclick="window._wcResetWarga('${esc(w.name)}')" title="Reset ke Default Role — Hapus semua pengaturan override manual untuk warga ini. Semua tool access akan kembali ke nilai default berdasarkan role-nya. Konfirmasi akan diminta sebelum reset dilakukan.">↺ Reset ke role default</button>
      </div>
    </div>
  `;
}

async function loadAndPaintCaps(wargaName) {
  try {
    const r = await fetchJSON('/api/warga-caps/effective?warga=' + encodeURIComponent(wargaName));
    const capsMap = {};
    (r.caps || []).forEach(c => { capsMap[c.tool] = { enabled: c.enabled, isOverride: c.is_override }; });
    CAPS_CACHE[wargaName] = capsMap;
    paintCardBody(wargaName);
  } catch (e) {
    const body = document.getElementById('wcBody-' + wargaName);
    if (body) body.innerHTML = `<div class="wc-empty" style="padding:1rem">❌ ${esc(e.message)}</div>`;
  }
}

function paintCardBody(wargaName) {
  const body = document.getElementById('wcBody-' + wargaName);
  if (!body) return;
  const caps = CAPS_CACHE[wargaName] || {};
  body.innerHTML = CATALOG.map(cat => {
    const tools = cat.tools.map(t => {
      const c = caps[t] || { enabled: false, isOverride: false };
      const overrideClass = c.isOverride ? ' override' : '';
      return `<label class="wc-tool${overrideClass}" title="${esc(t)} — Centang untuk mengaktifkan atau hapus centang untuk menonaktifkan akses tool ini untuk ${esc(wargaName)}. ${c.isOverride ? 'Ini adalah override manual (tanda *) yang menimpa default role.' : 'Ini adalah nilai default dari role warga ini.'} Perubahan langsung tersimpan ke database.">
        <input type="checkbox" ${c.enabled ? 'checked' : ''} data-warga="${esc(wargaName)}" data-tool="${esc(t)}" onchange="window._wcToggle(this)" title="Centang/hapus centang untuk mengaktifkan atau menonaktifkan akses tool ${esc(t)} untuk warga ${esc(wargaName)}. Perubahan langsung tersimpan dan berlaku seketika." />
        <span>${esc(t)}</span>
      </label>`;
    }).join('');
    return `<div class="wc-cat"><div class="wc-cat-title">${esc(cat.category)}</div><div class="wc-tools">${tools}</div></div>`;
  }).join('');
}

// Global handlers (attached to window for inline onclick)
window._wcToggle = async function (checkbox) {
  const warga = checkbox.dataset.warga;
  const tool = checkbox.dataset.tool;
  const enabled = checkbox.checked;
  setStatus(`Toggling ${tool} for ${warga}...`);
  try {
    const r = await fetch('/api/warga-caps/override', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ warga, tool, enabled, granted_by: 'ayah' })
    });
    if (!r.ok) throw new Error(`HTTP ${r.status}`);
    setStatus(`✅ ${enabled ? 'enabled' : 'disabled'} ${tool} for ${warga}`);
    // Update cache + repaint
    if (CAPS_CACHE[warga]) {
      CAPS_CACHE[warga][tool] = { enabled, isOverride: true };
      paintCardBody(warga);
    }
  } catch (e) {
    setStatus(`❌ Toggle failed: ${e.message}`);
    checkbox.checked = !enabled; // revert
  }
};

window._wcResetWarga = async function (warga) {
  if (!confirm(`Clear semua override untuk ${warga}? Semua tool revert ke role default.`)) return;
  const caps = CAPS_CACHE[warga] || {};
  const overrideTools = Object.keys(caps).filter(t => caps[t].isOverride);
  if (overrideTools.length === 0) {
    setStatus(`${warga} ga punya override`);
    return;
  }
  setStatus(`Clearing ${overrideTools.length} overrides...`);
  try {
    await Promise.all(overrideTools.map(tool =>
      fetch('/api/warga-caps/override', {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ warga, tool })
      })
    ));
    setStatus(`✅ Reset ${warga} done`);
    await loadAndPaintCaps(warga);
  } catch (e) {
    setStatus(`❌ Reset failed: ${e.message}`);
  }
};
