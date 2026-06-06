// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30 (re-audited 2026-06-07)
// Update 2026-06-07 (owner-approved audit): fixed empty-state bug — the "no progress"
//   line used a single-quoted string, so `${esc(L.none)}` rendered LITERALLY instead
//   of the translated text; now a template literal. De-hardcoded the section header +
//   table column headers (were inline Indonesian) → i18n (commits.recent/col_*). Hash
//   cell now guards a missing value (String(c.hash||'')) and ago() output is esc()'d.
// Reason: Audit Log tab (reference copy 36 LOC). Audit pass — esc() on all rendered fields, table format only..

import { esc, ago, fetchJSON } from '../js/utils.js';
import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('commits.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

export async function render(mainEl) {
  mainEl.innerHTML = `
    <h2>${esc(L.title)}</h2>
    <div class="sub">${esc(L.sub)}</div>
    <div class="card">
      <div class="ch">${esc(L.recent)}</div>
      <div class="cb" id="commits"><div class="empty">${esc(L.running)}</div></div>
    </div>
  `;

  try {
    const data = await fetchJSON('/api/commits');
    const el = document.getElementById('commits');
    
    if(!data.commits || !data.commits.length) {
      el.innerHTML = `<div class="empty">${esc(L.none)}</div>`;
      return;
    }

    el.innerHTML = `<table class="tt-table"><thead><tr><th>${esc(L.colTime)}</th><th>${esc(L.colAuthor)}</th><th>${esc(L.colMessage)}</th><th>${esc(L.colHash)}</th></tr></thead><tbody>` +
      data.commits.map(c => `
        <tr>
          <td style="white-space:nowrap;color:var(--text-muted)">${esc(ago(c.date))}</td>
          <td><b>${esc(c.author)}</b></td>
          <td>${esc(c.subject)}</td>
          <td style="font-family:monospace;color:#64748b">${esc(String(c.hash || '').substring(0,7))}</td>
        </tr>
      `).join('') + '</tbody></table>';

  } catch(e) {
    document.getElementById('commits').innerHTML = `<div class="err">Error: ${esc(e.message)}</div>`;
  }
}

