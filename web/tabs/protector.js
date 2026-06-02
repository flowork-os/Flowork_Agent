// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Protector tab (reference 425 LOC). Audit pass — esc() on rule+pattern+reason, POST CSRF via same-origin only..

import { esc, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('protector.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

const CSS = `
.pr-shell {
  display: grid;
  grid-template-columns: 1fr 320px;
  gap: 16px;
  height: calc(100vh - 220px);
  min-height: 540px;
  overflow: hidden;
}
.pr-main {
  position: relative;
  background:
    radial-gradient(circle at 15% 0%, rgba(16, 185, 129, 0.07), transparent 55%),
    linear-gradient(165deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 50%),
    rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  padding: 18px;
  backdrop-filter: blur(14px);
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  box-shadow: 0 6px 18px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.05);
}
.pr-main::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: linear-gradient(90deg, transparent, rgba(16,185,129,0.25), transparent);
  pointer-events: none;
}
.pr-side {
  background:
    radial-gradient(circle at 85% 0%, rgba(16, 185, 129, 0.05), transparent 55%),
    linear-gradient(165deg, rgba(255,255,255,0.04), rgba(255,255,255,0) 50%),
    rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  padding: 16px;
  backdrop-filter: blur(14px);
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-width: thin;
  scrollbar-color: rgba(16,185,129,0.3) transparent;
  box-shadow: 0 6px 18px rgba(0,0,0,0.3), inset 0 1px 0 rgba(255,255,255,0.05);
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.pr-side::-webkit-scrollbar { width: 4px; }
.pr-side::-webkit-scrollbar-thumb { background: rgba(16,185,129,0.3); border-radius: 2px; }

.pr-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: 14px; flex-shrink: 0; }
.pr-head h3 { font-family: var(--font-heading); font-size: 1rem; color: #e2e8f0; font-weight: 600; }
.pr-stat { display: inline-flex; gap: 10px; margin-left: auto; font-size: 0.72rem; color: var(--text-muted); font-family: monospace; }
.pr-stat b { font-family: var(--font-heading); }
.pr-stat .st-total b { color: #10b981; }
.pr-stat .st-hard b { color: #f59e0b; }
.pr-stat .st-custom b { color: #3b82f6; }
.pr-stat .st-off b { color: #ef4444; }

.pr-filter-bar { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 12px; flex-shrink: 0; }
.pr-filter-bar button {
  font-size: 0.72rem; padding: 4px 10px; border-radius: 999px;
  border: 1px solid var(--glass-border); background: rgba(15,23,42,0.5);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s;
}
.pr-filter-bar button:hover { background: rgba(30,34,56,0.6); color: #e2e8f0; }
.pr-filter-bar button.active { background: rgba(16,185,129,0.15); border-color: #10b981; color: #10b981; }

.pr-list {
  flex: 1 1 auto; min-height: 0; overflow-y: auto; overscroll-behavior: contain;
  scrollbar-width: thin; scrollbar-color: rgba(16,185,129,0.3) transparent;
}
.pr-list::-webkit-scrollbar { width: 5px; }
.pr-list::-webkit-scrollbar-thumb { background: rgba(16,185,129,0.3); border-radius: 3px; }

.pr-rule {
  display: grid;
  grid-template-columns: 28px 1fr auto auto auto;
  gap: 10px;
  align-items: center;
  padding: 10px 12px;
  background: linear-gradient(160deg, rgba(255,255,255,0.03), rgba(255,255,255,0) 45%), rgba(15,23,42,0.5);
  border: 1px solid var(--glass-border);
  border-left: 3px solid var(--rule-color, #64748b);
  border-radius: 10px;
  margin-bottom: 6px;
  transition: background 0.15s, transform 0.15s;
}
.pr-rule:hover { background: rgba(30,34,56,0.7); transform: translateX(2px); }
.pr-rule.is-disabled { opacity: 0.45; }

.pr-rule-icon { font-size: 1rem; text-align: center; }
.pr-rule-path {
  font-family: monospace; font-size: 0.78rem; color: #e2e8f0;
  white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
}
.pr-rule-badge {
  font-size: 0.65rem; padding: 2px 8px; border-radius: 999px;
  font-weight: 600; font-family: monospace; white-space: nowrap;
}
.pr-badge-hardcoded { background: rgba(245,158,11,0.15); color: #f59e0b; border: 1px solid rgba(245,158,11,0.3); }
.pr-badge-custom { background: rgba(59,130,246,0.15); color: #3b82f6; border: 1px solid rgba(59,130,246,0.3); }
.pr-badge-cat {
  font-size: 0.62rem; padding: 2px 6px; border-radius: 999px;
  background: rgba(255,255,255,0.06); color: var(--text-muted); border: 1px solid var(--glass-border);
}

.pr-actions { display: flex; gap: 4px; }
.pr-actions button {
  font-size: 0.78rem; padding: 4px 8px; border-radius: 6px;
  border: 1px solid var(--glass-border); background: rgba(15,23,42,0.6);
  cursor: pointer; transition: all 0.15s; color: var(--text-muted);
}
.pr-actions button:hover { background: rgba(30,34,56,0.8); color: #e2e8f0; }
.pr-actions .btn-danger:hover { background: rgba(239,68,68,0.2); color: #ef4444; border-color: #ef4444; }
.pr-actions .btn-toggle.is-on { color: #10b981; }
.pr-actions .btn-toggle.is-off { color: #ef4444; }

/* Side panels */
.pr-panel {
  background: rgba(15,23,42,0.4);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  padding: 14px;
}
.pr-panel h4 { font-family: var(--font-heading); font-size: 0.85rem; color: #e2e8f0; margin-bottom: 10px; font-weight: 600; }
.pr-panel .hint { font-size: 0.68rem; color: var(--text-muted); margin-bottom: 10px; }

.pr-input {
  width: 100%; padding: 8px 10px; border-radius: 8px;
  border: 1px solid var(--glass-border); background: rgba(15,23,42,0.8);
  color: #e2e8f0; font-family: monospace; font-size: 0.78rem;
  margin-bottom: 8px; outline: none; transition: border-color 0.15s;
}
.pr-input:focus { border-color: #10b981; }

.pr-select {
  width: 100%; padding: 8px 10px; border-radius: 8px;
  border: 1px solid var(--glass-border); background: rgba(15,23,42,0.8);
  color: #e2e8f0; font-size: 0.78rem; margin-bottom: 8px; outline: none;
}

.pr-btn-add {
  width: 100%; padding: 10px; border-radius: 10px; border: none;
  background: linear-gradient(135deg, #10b981, #059669);
  color: #fff; font-weight: 700; font-size: 0.82rem; cursor: pointer;
  transition: opacity 0.15s, transform 0.1s;
  font-family: var(--font-heading);
}
.pr-btn-add:hover { opacity: 0.9; transform: translateY(-1px); }
.pr-btn-add:active { transform: translateY(0); }

.pr-test-result {
  padding: 10px; border-radius: 8px; font-size: 0.82rem;
  font-weight: 600; text-align: center; margin-top: 8px;
}
.pr-test-protected { background: rgba(16,185,129,0.15); color: #10b981; border: 1px solid rgba(16,185,129,0.3); }
.pr-test-unprotected { background: rgba(239,68,68,0.15); color: #ef4444; border: 1px solid rgba(239,68,68,0.3); }

@media (max-width: 1000px) {
  .pr-shell { grid-template-columns: 1fr; }
}
`;

const CAT_ICONS = {
  secrets: '🔑', core: '⚙️', doktrin: '📜', entry: '🚪',
  docs: '📄', infra: '🏗️', security: '🛡️', config: '⚙️',
  custom: '🟢', other: '📦'
};
const CAT_COLORS = {
  secrets: '#ef4444', core: '#8b5cf6', doktrin: '#f59e0b', entry: '#3b82f6',
  docs: '#06b6d4', infra: '#a855f7', security: '#10b981', config: '#64748b',
  custom: '#3b82f6', other: '#94a3b8'
};

let rules = [];
let stats = {};
let filterCat = null;
let refreshTimer = null;

export async function render(mainEl) {
  loadStyle('protector', CSS);
  if (refreshTimer) clearInterval(refreshTimer);

  mainEl.innerHTML = `
    <h2>${esc(L.title)}</h2>
    <div class="sub">${esc(L.sub)}</div>
    <div class="pr-shell">
      <div class="pr-main">
        <div class="pr-head">
          <div>
            <h3>${esc(L.rulesH)}</h3>
          </div>
          <div class="pr-stat">
            <span class="st-total"><b id="prTotal">0</b> ${esc(L.statTotal)}</span>
            <span class="st-hard"><b id="prHard">0</b> ${esc(L.statHard)}</span>
            <span class="st-custom"><b id="prCustom">0</b> ${esc(L.statCustom)}</span>
            <span class="st-off"><b id="prOff">0</b> ${esc(L.statOff)}</span>
          </div>
        </div>
        <div class="pr-filter-bar" id="prFilters"></div>
        <div class="pr-list" id="prList"><div class="empty">${esc(L.loading)}</div></div>
      </div>
      <aside class="pr-side">
        <div class="pr-panel">
          <h4>${esc(L.addH)}</h4>
          <div class="hint">${esc(L.addHint)}</div>
          <input class="pr-input" id="prAddPath" placeholder="${esc(L.addPh)}" title="${esc(L.addTip)}" />
          <select class="pr-select" id="prAddType" title="${esc(L.typeTip)}">
            <option value="suffix">${esc(L.optSuffix)}</option>
            <option value="basename">${esc(L.optBasename)}</option>
          </select>
          <select class="pr-select" id="prAddCat" title="${esc(L.catTip)}">
            <option value="custom">${esc(L.catCustom)}</option>
            <option value="secrets">${esc(L.catSecrets)}</option>
            <option value="core">${esc(L.catCore)}</option>
            <option value="doktrin">${esc(L.catDoktrin)}</option>
            <option value="entry">${esc(L.catEntry)}</option>
            <option value="docs">${esc(L.catDocs)}</option>
            <option value="config">${esc(L.catConfig)}</option>
          </select>
          <button class="pr-btn-add" id="prAddBtn" title="${esc(L.addBtnTip)}">${esc(L.addBtn)}</button>
          <div id="prAddMsg"></div>
        </div>
        <div class="pr-panel">
          <h4>${esc(L.testH)}</h4>
          <div class="hint">${esc(L.testHint)}</div>
          <input class="pr-input" id="prTestPath" placeholder="${esc(L.testPh)}" title="${esc(L.testTip)}" />
          <button class="pr-btn-add" id="prTestBtn" style="background:linear-gradient(135deg,#3b82f6,#2563eb)" title="${esc(L.testBtnTip)}">${esc(L.testBtn)}</button>
          <div id="prTestResult"></div>
        </div>
      </aside>
    </div>
  `;

  document.getElementById('prAddBtn').addEventListener('click', handleAdd);
  document.getElementById('prTestBtn').addEventListener('click', handleTest);
  document.getElementById('prAddPath').addEventListener('keydown', e => { if (e.key === 'Enter') handleAdd(); });
  document.getElementById('prTestPath').addEventListener('keydown', e => { if (e.key === 'Enter') handleTest(); });

  await loadData();
  refreshTimer = setInterval(loadData, 15000);
}

async function loadData() {
  try {
    const d = await fetchJSON('/api/protector');
    rules = Array.isArray(d.rules) ? d.rules : [];
    stats = d.stats || {};
    renderStats();
    renderFilters();
    renderList();
  } catch (e) {
    const el = document.getElementById('prList');
    if (el) el.innerHTML = `<div class="err">Gagal muat: ${esc(e.message)}</div>`;
  }
}

function renderStats() {
  const s = (id, v) => { const el = document.getElementById(id); if (el) el.textContent = v; };
  s('prTotal', stats.total || 0);
  s('prHard', stats.hardcoded || 0);
  s('prCustom', stats.custom || 0);
  s('prOff', stats.disabled || 0);
}

function renderFilters() {
  const el = document.getElementById('prFilters');
  if (!el) return;
  const cats = {};
  rules.forEach(r => { cats[r.category] = (cats[r.category] || 0) + 1; });
  const entries = Object.entries(cats).sort((a, b) => b[1] - a[1]);

  el.innerHTML = `<button data-cat="" class="${!filterCat ? 'active' : ''}">Semua (${rules.length})</button>` +
    entries.map(([cat, n]) =>
      `<button data-cat="${esc(cat)}" class="${filterCat === cat ? 'active' : ''}">${CAT_ICONS[cat] || '📦'} ${esc(cat)} (${n})</button>`
    ).join('');

  el.querySelectorAll('button').forEach(btn => {
    btn.addEventListener('click', () => {
      filterCat = btn.dataset.cat || null;
      renderFilters();
      renderList();
    });
  });
}

function renderList() {
  const el = document.getElementById('prList');
  if (!el) return;
  let visible = rules;
  if (filterCat) visible = visible.filter(r => r.category === filterCat);

  // Sort: active first, then hardcoded before custom, then by path
  visible.sort((a, b) => {
    if (a.active !== b.active) return a.active ? -1 : 1;
    if (a.source !== b.source) return a.source === 'hardcoded' ? -1 : 1;
    return a.path.localeCompare(b.path);
  });

  if (!visible.length) {
    el.innerHTML = '<div class="empty">${esc(L.noMatch)}</div>';
    return;
  }

  el.innerHTML = visible.map(r => {
    const col = CAT_COLORS[r.category] || '#64748b';
    const icon = CAT_ICONS[r.category] || '📦';
    const dis = !r.active ? ' is-disabled' : '';
    const isHard = r.source === 'hardcoded';
    const toggleIcon = r.active ? '🟢' : '🔴';
    const toggleClass = r.active ? 'is-on' : 'is-off';

    return `<div class="pr-rule${dis}" style="--rule-color:${col}" data-path="${esc(r.path)}">
      <span class="pr-rule-icon">${icon}</span>
      <span class="pr-rule-path" title="${esc(r.path)}">${esc(r.path)}</span>
      <span class="pr-badge-cat">${esc(r.category)}</span>
      <span class="pr-rule-badge ${isHard ? 'pr-badge-hardcoded' : 'pr-badge-custom'}">${isHard ? 'HARDCODED' : 'CUSTOM'}</span>
      <span class="pr-actions">
        <button class="btn-toggle ${toggleClass}" data-action="toggle" data-path="${esc(r.path)}" data-active="${r.active}" title="${r.active ? 'Disable' : 'Enable'}">${toggleIcon}</button>
        ${!isHard ? `<button class="btn-danger" data-action="remove" data-path="${esc(r.path)}" title="${esc(L.delConfirm)}">🗑️</button>` : ''}
      </span>
    </div>`;
  }).join('');

  el.querySelectorAll('.btn-toggle').forEach(btn => {
    btn.addEventListener('click', () => handleToggle(btn.dataset.path, btn.dataset.active === 'true'));
  });
  el.querySelectorAll('.btn-danger').forEach(btn => {
    btn.addEventListener('click', () => handleRemove(btn.dataset.path));
  });
}

async function handleAdd() {
  const pathEl = document.getElementById('prAddPath');
  const typeEl = document.getElementById('prAddType');
  const catEl = document.getElementById('prAddCat');
  const msgEl = document.getElementById('prAddMsg');
  const path = pathEl.value.trim();
  if (!path) { msgEl.innerHTML = '<div style="color:#ef4444;font-size:0.75rem;margin-top:6px">${esc(L.pathRequired)}</div>'; return; }
  try {
    const res = await fetch('/api/protector/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, type: typeEl.value, category: catEl.value })
    });
    if (!res.ok) {
      const txt = await res.text().catch(() => res.statusText);
      let errMsg = txt;
      try { errMsg = JSON.parse(txt).message || txt; } catch (_) {}
      msgEl.innerHTML = `<div style="color:#ef4444;font-size:0.75rem;margin-top:6px">${esc(errMsg)}</div>`;
      return;
    }
    await res.json();
    msgEl.innerHTML = '<div style="color:#10b981;font-size:0.75rem;margin-top:6px">${esc(L.ruleAdded)}</div>';
    pathEl.value = '';
    await loadData();
  } catch (e) {
    msgEl.innerHTML = `<div style="color:#ef4444;font-size:0.75rem;margin-top:6px">Error: ${esc(e.message)}</div>`;
  }
}

async function handleToggle(path, currentActive) {
  // Bug Gemini #15 fix (2026-04-27): cek r.ok dulu sebelum await loadData(),
  // alert kalau backend reject (mis. rule hardcoded yang ngga boleh ditoggle).
  // Sebelumnya fire-and-forget — UI reload seolah sukses padahal server reject.
  try {
    const r = await fetch('/api/protector/toggle', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, active: !currentActive })
    });
    if (!r.ok) {
      const errText = await r.text().catch(() => r.statusText);
      alert(`${L.toggleFail} "${path}": ${errText || 'HTTP ' + r.status}`);
      return;
    }
    await loadData();
  } catch (e) {
    console.error('toggle error:', e);
    alert(`${L.toggleErr}${e.message}`);
  }
}

async function handleRemove(path) {
  if (!confirm(`${L.delConfirm} "${path}"?`)) return;
  // Bug Gemini #15 fix: same pattern dengan handleToggle.
  try {
    const r = await fetch('/api/protector/remove', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path })
    });
    if (!r.ok) {
      const errText = await r.text().catch(() => r.statusText);
      alert(`${L.delFail} "${path}": ${errText || 'HTTP ' + r.status}`);
      return;
    }
    await loadData();
  } catch (e) {
    console.error('remove error:', e);
    alert(`${L.removeErr}${e.message}`);
  }
}

async function handleTest() {
  const pathEl = document.getElementById('prTestPath');
  const resultEl = document.getElementById('prTestResult');
  const path = pathEl.value.trim();
  if (!path) { resultEl.innerHTML = ''; return; }
  try {
    const d = await fetchJSON(`/api/protector/test?path=${encodeURIComponent(path)}`);
    if (d.protected) {
      resultEl.innerHTML = '<div class="pr-test-result pr-test-protected">${esc(L.protected)}</div>';
    } else {
      resultEl.innerHTML = '<div class="pr-test-result pr-test-unprotected">${esc(L.notProtected)}</div>';
    }
  } catch (e) {
    resultEl.innerHTML = `<div style="color:#ef4444;font-size:0.75rem">Error: ${esc(e.message)}</div>`;
  }
}
