// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 7 phase 2 UI — Browse Router Catalog modal. Plug-and-play
//   module (import default), dictionary-only labels. Phase 3 (favorite,
//   tag filter, recently-used) → tambah file baru, JANGAN modify ini.
//
// agents_router_skills.js — Browse Router skill catalog modal.
//
// Flow:
//   1. User klik tombol "Browse Router Catalog" di skill section
//   2. Modal terbuka, fetch GET /api/agents/router-skills/list?id=<agent>
//   3. Render list summary (name + description)
//   4. Klik "Use this skill" → fetch GET /api/agents/router-skills/get?
//      id=<agent>&name=<skill> → callback dengan full skill doc
//   5. Caller di agents.js push ke skills[] + close modal

import { t } from '/js/i18n.js';

const API_LIST = '/api/agents/router-skills/list';
const API_GET  = '/api/agents/router-skills/get';

// esc — minimal HTML escape (defense XSS dari Router response).
function esc(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

// fillTpl — replace {count} {total} placeholder dengan nilai actual.
function fillTpl(s, vars) {
  return String(s).replace(/\{(\w+)\}/g, (_, k) => esc(vars[k] ?? ''));
}

export function openRouterSkillBrowser(agentId, onChoose) {
  // Modal root.
  const root = document.createElement('div');
  root.className = 'ag-modal-bg';
  root.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;display:flex;align-items:center;justify-content:center;padding:20px';
  root.innerHTML = `
    <div class="ag-modal" style="max-width:680px;width:100%;max-height:80vh;display:flex;flex-direction:column;background:#0f172a;border:1px solid #334155;border-radius:12px;padding:18px;overflow:hidden">
      <h3 style="margin:0 0 12px 0;color:#f1f5f9">${esc(t('menu.tab.agents.skills_router_modal_h'))}</h3>
      <input id="rsb-search" placeholder="${esc(t('menu.tab.agents.skills_router_search_ph'))}"
             style="padding:8px 12px;background:#1e293b;border:1px solid #475569;border-radius:6px;color:#f1f5f9;margin-bottom:10px">
      <div id="rsb-status" style="color:#94a3b8;font-size:13px;margin-bottom:8px"></div>
      <div id="rsb-list" style="flex:1;overflow-y:auto;display:flex;flex-direction:column;gap:6px"></div>
      <div style="display:flex;justify-content:flex-end;margin-top:12px">
        <button id="rsb-close" class="ag-btn ghost">${esc(t('menu.tab.agents.skills_router_close_btn'))}</button>
      </div>
    </div>
  `;
  document.body.appendChild(root);

  const $ = (sel) => root.querySelector(sel);
  const close = () => { try { root.remove(); } catch (_) {} };

  $('#rsb-close').onclick = close;
  // Click backdrop = close. Click inner = ignore.
  root.addEventListener('click', (e) => { if (e.target === root) close(); });

  let allItems = []; // cached list dari last fetch
  let pendingTotal = 0;

  const render = (items) => {
    const listEl = $('#rsb-list');
    if (!items.length) {
      listEl.innerHTML = `<p style="color:#64748b;text-align:center;padding:18px">${esc(t('menu.tab.agents.skills_router_empty'))}</p>`;
      return;
    }
    listEl.innerHTML = items.map((it) => `
      <div style="background:#1e293b;border:1px solid #334155;border-radius:8px;padding:10px;display:flex;justify-content:space-between;gap:10px;align-items:flex-start">
        <div style="flex:1;min-width:0">
          <div style="color:#f1f5f9;font-weight:600;font-family:ui-monospace,monospace;font-size:13px">${esc(it.name)}</div>
          <div style="color:#94a3b8;font-size:12px;margin-top:4px">${esc(it.description)}</div>
        </div>
        <button class="ag-btn primary" data-name="${esc(it.name)}" style="flex-shrink:0">${esc(t('menu.tab.agents.skills_router_use_btn'))}</button>
      </div>
    `).join('');
    listEl.querySelectorAll('button[data-name]').forEach((b) => {
      b.onclick = async () => {
        b.disabled = true;
        b.textContent = '...';
        try {
          const url = `${API_GET}?id=${encodeURIComponent(agentId)}&name=${encodeURIComponent(b.dataset.name)}`;
          const resp = await fetch(url);
          const data = await resp.json();
          if (data.error) {
            $('#rsb-status').textContent = data.error;
            $('#rsb-status').style.color = '#f87171';
            b.disabled = false;
            b.textContent = t('menu.tab.agents.skills_router_use_btn');
            return;
          }
          onChoose(data);
          close();
        } catch (err) {
          $('#rsb-status').textContent = String(err);
          $('#rsb-status').style.color = '#f87171';
          b.disabled = false;
          b.textContent = t('menu.tab.agents.skills_router_use_btn');
        }
      };
    });
  };

  const fetchList = async (search) => {
    $('#rsb-status').textContent = t('menu.tab.agents.skills_router_fetching');
    $('#rsb-status').style.color = '#94a3b8';
    $('#rsb-list').innerHTML = '';
    try {
      const url = `${API_LIST}?id=${encodeURIComponent(agentId)}&search=${encodeURIComponent(search || '')}`;
      const resp = await fetch(url);
      const data = await resp.json();
      if (data.error) {
        $('#rsb-status').textContent = data.error || t('menu.tab.agents.skills_router_error');
        $('#rsb-status').style.color = '#f87171';
        return;
      }
      allItems = data.items || [];
      pendingTotal = data.total || allItems.length;
      $('#rsb-status').textContent = fillTpl(
        t('menu.tab.agents.skills_router_count'),
        { count: allItems.length, total: pendingTotal },
      );
      render(allItems);
    } catch (err) {
      $('#rsb-status').textContent = t('menu.tab.agents.skills_router_error');
      $('#rsb-status').style.color = '#f87171';
    }
  };

  // Debounced search.
  let searchTimer = null;
  $('#rsb-search').addEventListener('input', (e) => {
    clearTimeout(searchTimer);
    const q = e.target.value;
    searchTimer = setTimeout(() => fetchList(q), 300);
  });

  // Initial load.
  fetchList('');
}
