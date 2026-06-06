// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 17 phase 2 Web UI slash input modal. API stable:
//   openSlashModal(agentId). Plug-and-play module. Dictionary-only
//   labels. Phase 3 (history dropdown, autocomplete from /slash/registry,
//   pipe to file) → tambah file baru, JANGAN modify ini.
//
// agents_slash_modal.js — quick slash input modal per kartu agent.

import { t } from '/js/i18n.js';

const API_RUN = '/api/agents/slash/run';

function esc(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

export function openSlashModal(agentId) {
  const root = document.createElement('div');
  root.className = 'ag-modal-bg';
  root.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.6);z-index:9999;display:flex;align-items:center;justify-content:center;padding:20px';
  root.innerHTML = `
    <div class="ag-modal" style="max-width:640px;width:100%;max-height:80vh;display:flex;flex-direction:column;background:#0f172a;border:1px solid #334155;border-radius:12px;padding:18px;overflow:hidden">
      <h3 style="margin:0 0 8px 0;color:#f1f5f9">
        ${esc(t('menu.tab.agents.slash_modal_h') || 'Slash command')}
        <span style="color:#94a3b8;font-size:13px;font-weight:normal">— @${esc(agentId)}</span>
      </h3>
      <p style="margin:0 0 12px 0;color:#94a3b8;font-size:12px">
        ${esc(t('menu.tab.agents.slash_modal_sub') || 'Type /command or click hint. Enter = run, Esc = close.')}
      </p>
      <input id="sl-input" placeholder="/help"
             autocomplete="off" autocapitalize="off" spellcheck="false"
             style="padding:10px 12px;background:#1e293b;border:1px solid #475569;border-radius:6px;color:#f1f5f9;font-family:ui-monospace,monospace;margin-bottom:8px">
      <div id="sl-hints" style="margin-bottom:8px;color:#64748b;font-size:12px"></div>
      <div id="sl-status" style="color:#94a3b8;font-size:13px;margin-bottom:8px"></div>
      <div id="sl-output" style="flex:1;overflow-y:auto;background:#020617;color:#cbd5e1;font-family:ui-monospace,monospace;font-size:13px;padding:10px;border-radius:6px;border:1px solid #1e293b;white-space:pre-wrap;min-height:80px;max-height:300px"></div>
      <div style="display:flex;justify-content:flex-end;gap:8px;margin-top:12px">
        <button id="sl-close" class="ag-btn ghost">${esc(t('common.btn.close') || 'Close')}</button>
        <button id="sl-run" class="ag-btn primary">${esc(t('menu.tab.agents.slash_run_btn') || 'Run')}</button>
      </div>
    </div>
  `;
  document.body.appendChild(root);

  const $ = (sel) => root.querySelector(sel);
  const close = () => { try { root.remove(); } catch (_) {} };

  $('#sl-close').onclick = close;
  root.addEventListener('click', (e) => { if (e.target === root) close(); });
  document.addEventListener('keydown', escHandler, { once: false });
  function escHandler(e) {
    if (e.key === 'Escape') {
      close();
      document.removeEventListener('keydown', escHandler);
    }
  }

  // Common slash hint chips (clickable).
  const hints = ['/help', '/version', '/tools', '/stats', '/now', '/tool_search '];
  $('#sl-hints').innerHTML = hints.map((h) => `
    <button class="ag-btn" data-hint="${esc(h)}" style="font-size:11px;padding:3px 8px;margin:2px">${esc(h)}</button>
  `).join('');
  $('#sl-hints').querySelectorAll('button[data-hint]').forEach((b) => {
    b.onclick = () => {
      $('#sl-input').value = b.dataset.hint;
      $('#sl-input').focus();
    };
  });

  const run = async () => {
    const text = $('#sl-input').value.trim();
    if (!text) return;
    if (!text.startsWith('/')) {
      $('#sl-status').textContent = t('menu.tab.agents.slash_must_start') || 'Command must start with /';
      $('#sl-status').style.color = '#f87171';
      return;
    }
    $('#sl-status').textContent = t('menu.tab.agents.slash_running') || 'Running…';
    $('#sl-status').style.color = '#94a3b8';
    $('#sl-output').textContent = '';
    try {
      const url = `${API_RUN}?id=${encodeURIComponent(agentId)}`;
      const resp = await fetch(url, {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ text, caller: 'web-ui' }),
      });
      const data = await resp.json();
      if (data.error) {
        $('#sl-status').textContent = data.error;
        $('#sl-status').style.color = '#f87171';
        return;
      }
      $('#sl-status').textContent = `${data.command} · ${data.duration_ms ?? 0}ms`;
      $('#sl-status').style.color = '#34d399';
      $('#sl-output').textContent = data.result?.text || '(empty)';
    } catch (err) {
      $('#sl-status').textContent = String(err);
      $('#sl-status').style.color = '#f87171';
    }
  };

  $('#sl-run').onclick = run;
  $('#sl-input').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') { e.preventDefault(); run(); }
  });
  // Focus + select after frame so cursor lands.
  setTimeout(() => $('#sl-input').focus(), 30);
}
