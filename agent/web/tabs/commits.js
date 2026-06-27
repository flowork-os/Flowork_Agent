// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30 (re-audited 2026-06-07)
// Update 2026-06-07 (owner-approved audit): fixed empty-state bug — the "no progress"
//   line used a single-quoted string, so `${esc(L.none)}` rendered LITERALLY instead
//   of the translated text; now a template literal. De-hardcoded the section header +
//   table column headers (were inline Indonesian) → i18n (commits.recent/col_*). Hash
//   cell now guards a missing value (String(c.hash||'')) and ago() output is esc()'d.
// Update (UI): re-skinned to shared glass-3D design-system (fw-* via ensureGlass) —
//   full-width, zero bespoke theme CSS, zero raw hex. ALL i18n keys / endpoint / IDs intact.
// Reason: Audit Log tab (reference copy). Audit pass — esc() on all rendered fields, table format only..

import { esc, ago, fetchJSON } from '../js/utils.js';
import { ensureGlass } from '/js/glass.js';
import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('commits.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

export async function render(mainEl) {
  ensureGlass();
  mainEl.innerHTML = `
    <div class="fw-page">
      <div class="fw-head">
        <span class="fw-glyph">📈</span>
        <div>
          <h2 class="fw-title">${esc(L.title)}</h2>
          <div class="fw-sub">${esc(L.sub)}</div>
        </div>
      </div>
      <div class="fw-card">
        <div class="fw-sec">${esc(L.recent)}</div>
        <div id="commits"><div class="fw-empty">${esc(L.running)}</div></div>
      </div>
    </div>
  `;

  try {
    const data = await fetchJSON('/api/commits');
    const el = document.getElementById('commits');

    if(!data.commits || !data.commits.length) {
      el.innerHTML = `<div class="fw-empty">${esc(L.none)}</div>`;
      return;
    }

    el.innerHTML = data.commits.map(c => `
      <div class="fw-row">
        <span class="fw-id" style="white-space:nowrap">${esc(ago(c.date))}</span>
        <span class="fw-tag">${esc(c.author)}</span>
        <span class="fw-grow fw-desc" style="margin-top:0">${esc(c.subject)}</span>
        <span class="fw-id">${esc(String(c.hash || '').substring(0,7))}</span>
      </div>
    `).join('');

  } catch(e) {
    document.getElementById('commits').innerHTML = `<div class="fw-empty">${esc(e.message)}</div>`;
  }
}
