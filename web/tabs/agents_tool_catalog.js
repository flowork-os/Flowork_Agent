// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 13 phase 3 UI — popup section Tools catalog grid +
//   checkbox subscribe/unsubscribe. Replaces simple TOOL_FLAGS list.
//   Plug-and-play module. Dictionary-only labels.
//
// agents_tool_catalog.js — render tool catalog dengan subscribe toggle.

import { t } from '/js/i18n.js';

const API_CATALOG     = '/api/agents/tools/catalog';
const API_SUBSCRIBE   = '/api/agents/tools/subscribe';
const API_UNSUBSCRIBE = '/api/agents/tools/unsubscribe';

function esc(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

export async function renderToolCatalog(hostEl, agentId) {
  hostEl.innerHTML = `<p style="color:#64748b;font-size:12px">${esc(t('menu.tab.agents.tools_catalog_loading') || 'Loading…')}</p>`;
  try {
    const resp = await fetch(`${API_CATALOG}?id=${encodeURIComponent(agentId)}`);
    const data = await resp.json();
    if (data.error) {
      hostEl.innerHTML = `<p style="color:#f87171;font-size:12px">${esc(data.error)}</p>`;
      return;
    }
    const items = data.items || [];
    if (!items.length) {
      hostEl.innerHTML = `<p style="color:#64748b;font-size:12px">${esc(t('menu.tab.agents.tools_catalog_empty') || 'No tools registered.')}</p>`;
      return;
    }
    hostEl.innerHTML = `
      <p style="color:#94a3b8;font-size:11px;margin:0 0 8px 0">
        ${esc(t('menu.tab.agents.tools_catalog_sub') || 'Subscribe = appears in suggest pool + UI listing.')}
        (${items.length}/${data.total})
      </p>
      <div style="display:grid;gap:4px;max-height:240px;overflow-y:auto">
        ${items.map((it) => `
          <label style="display:flex;align-items:flex-start;gap:8px;padding:6px;background:#1e293b;border:1px solid #334155;border-radius:6px;cursor:pointer">
            <input type="checkbox" data-tool="${esc(it.name)}" ${it.subscribed ? 'checked' : ''}>
            <div style="flex:1;min-width:0">
              <div style="color:#f1f5f9;font-family:ui-monospace,monospace;font-size:12px">${esc(it.name)}</div>
              <div style="color:#94a3b8;font-size:11px">${esc(it.capability || '(no cap)')}</div>
              <div style="color:#64748b;font-size:11px;margin-top:2px">${esc(it.description || '')}</div>
            </div>
          </label>
        `).join('')}
      </div>
      <div id="cf-tools-catalog-status" style="margin-top:6px;font-size:11px;color:#94a3b8;min-height:1.2em"></div>
    `;
    hostEl.querySelectorAll('input[data-tool]').forEach((inp) => {
      inp.addEventListener('change', async (e) => {
        const tool = e.target.dataset.tool;
        const subscribed = e.target.checked;
        const url = subscribed ? API_SUBSCRIBE : API_UNSUBSCRIBE;
        const fullUrl = `${url}?id=${encodeURIComponent(agentId)}&tool=${encodeURIComponent(tool)}${subscribed ? '&source=manual' : ''}`;
        const statusEl = hostEl.querySelector('#cf-tools-catalog-status');
        statusEl.textContent = `${tool} ${subscribed ? 'subscribing' : 'unsubscribing'}…`;
        statusEl.style.color = '#94a3b8';
        try {
          const r = await fetch(fullUrl, { method: 'POST' });
          const d = await r.json();
          if (d.error) {
            statusEl.textContent = d.error;
            statusEl.style.color = '#f87171';
            e.target.checked = !subscribed;
            return;
          }
          statusEl.textContent = `✓ ${tool} ${subscribed ? 'subscribed' : 'unsubscribed'}`;
          statusEl.style.color = '#34d399';
        } catch (err) {
          statusEl.textContent = String(err);
          statusEl.style.color = '#f87171';
          e.target.checked = !subscribed;
        }
      });
    });
  } catch (err) {
    hostEl.innerHTML = `<p style="color:#f87171;font-size:12px">${esc(String(err))}</p>`;
  }
}
