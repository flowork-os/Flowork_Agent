// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// tabs/studio_lifecycle.js — ISI panel Siklus Hidup (ROADMAP_AI_STUDIO F3), di-render ke
// dalam DRAWER 3D (shell-nya di coder.js). Gabung 3 sumber: /api/coder/pending (Verifier),
// /api/reaper/candidates (Reaper), /api/studio/deathletters (Death-Letter). Pakai kit glass.js
// (3D-glass share). SEMUA copy lewat kamus i18n (t('coder.lc_*')) — nol hardcode. DELETABLE.
import { esc, fetchJSON } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { ensureGlass, statTile, badge, glassBtn, row } from '/js/glass.js';

const T = (k) => t('coder.' + k);
const SEV_TONE = { healthy: 'ok', warn: 'warn', critical: 'bad' };
const VERDICT_TONE = { approved: 'ok', review: 'warn', blocked: 'bad' };

// renderLifecycle — render isi siklus-hidup ke `body` (drawer body). Async.
export async function renderLifecycle(body) {
  ensureGlass();
  body.innerHTML = `<div class="gl-empty">${esc(T('lc_loading'))}</div>`;
  let pending = [], cands = [], letters = [];
  try {
    const [p, c, l] = await Promise.all([
      fetchJSON('/api/coder/pending').catch(() => ({ pending: [] })),
      fetchJSON('/api/reaper/candidates').catch(() => ({ candidates: [] })),
      fetchJSON('/api/studio/deathletters').catch(() => ({ death_letters: [] })),
    ]);
    pending = p.pending || []; cands = c.candidates || []; letters = l.death_letters || [];
  } catch (e) {
    body.innerHTML = `<div class="gl-empty" style="color:#f87171">${esc(T('lc_load_fail').replace('{err}', String(e.message || e)))}</div>`;
    return;
  }

  // Ringkasan 3D tiles
  const flagged = cands.filter((c) => c.flagged).length;
  const tiles = `<div class="gl-tiles" style="margin-bottom:18px">
    ${statTile('⏳', pending.length, T('lc_pending_title'), '#a78bfa')}
    ${statTile('❤️', cands.length - flagged, T('lc_health_title'), '#34d399')}
    ${statTile('⚠️', flagged, 'flagged', '#fbbf24')}
    ${statTile('⚰️', letters.length, T('lc_deaths_title'), '#94a3b8')}
  </div>`;

  const sect = (title, hint, inner) =>
    `<div class="gl-sect-t">${esc(title)} <span class="gl-hint">${esc(hint)}</span></div>${inner}`;

  const pendingHTML = pending.length ? pending.map((p) => {
    const id = p.id || (p.spec && p.spec.category_id) || '';
    const name = (p.spec && p.spec.name) || id;
    const v = p.verify || {};
    return row(
      `<span style="flex:1;color:var(--text-main);font-weight:600">${esc(name)}</span>` +
      badge(`${v.status || '?'}${v.score != null ? ' ' + v.score : ''}`, VERDICT_TONE[v.status] || 'mute') +
      `<span data-approve="${esc(id)}">${glassBtn(T('lc_approve'), 'ok')}</span>` +
      `<span data-reject="${esc(id)}">${glassBtn(T('lc_reject'), 'bad')}</span>`);
  }).join('') : `<div class="gl-empty">${esc(T('lc_pending_empty'))}</div>`;

  const healthHTML = cands.length ? cands.map((c) => {
    const sev = c.severity || 'healthy';
    const rate = c.error_rate ? ` · err ${(c.error_rate * 100).toFixed(0)}%` : '';
    const reap = c.flagged ? `<span data-reap="${esc(c.category_id)}">${glassBtn(T('lc_reap'), 'bad')}</span>` : '';
    return row(
      `<span style="flex:1;color:var(--text-main);font-weight:600">${esc(c.name || c.category_id)}` +
      `<span style="color:var(--text-muted);font-weight:400;font-size:.74rem"> (${c.done || 0}✓/${c.error || 0}✗${rate})</span></span>` +
      badge(c.reason_code || sev, SEV_TONE[sev] || 'mute') + reap);
  }).join('') : `<div class="gl-empty">${esc(T('lc_health_empty'))}</div>`;

  const letterHTML = letters.length ? letters.slice(0, 30).map((d) => row(
    `<span style="flex:1;color:var(--text-muted)">⚰️ <b style="color:#cbd5e1">${esc(d.name || d.id)}</b> ${badge(d.kind || '', 'mute')}` +
    `<div style="font-size:.74rem;margin-top:3px">${esc(d.reason || '')} · ${esc((d.at || '').replace('T', ' ').replace('Z', ''))}</div></span>`)
  ).join('') : `<div class="gl-empty">${esc(T('lc_deaths_empty'))}</div>`;

  body.innerHTML = tiles +
    sect(T('lc_pending_title'), T('lc_pending_hint'), pendingHTML) +
    sect(T('lc_health_title'), T('lc_health_hint'), healthHTML) +
    sect(T('lc_deaths_title'), T('lc_deaths_hint'), letterHTML);

  // Aksi (delegasi)
  const act = async (url, confirmMsg) => {
    if (confirmMsg && !confirm(confirmMsg)) return;
    try { await fetchJSON(url, { method: 'POST' }); renderLifecycle(body); }
    catch (e) { alert(T('lc_action_fail') + (e.message || e)); }
  };
  body.querySelectorAll('[data-approve]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/coder/approve?id=${encodeURIComponent(el.dataset.approve)}`));
  body.querySelectorAll('[data-reject]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/coder/reject?id=${encodeURIComponent(el.dataset.reject)}`, T('lc_confirm_reject')));
  body.querySelectorAll('[data-reap]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/reaper/reap?category=${encodeURIComponent(el.dataset.reap)}`, T('lc_confirm_reap')));
}
