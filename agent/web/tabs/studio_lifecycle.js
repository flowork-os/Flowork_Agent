// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// tabs/studio_lifecycle.js — ISI panel Siklus Hidup (ROADMAP_AI_STUDIO F3), di-render ke
// dalam DRAWER 3D (shell-nya di coder.js). Sumber: /api/coder/pending (Verifier) + /api/
// studio/deathletters (Death-Letter) = CEPAT (DB) → auto. /api/reaper/candidates = MAHAL
// (smoke-test = panggil LLM tiap kategori) → ON-DEMAND (tombol), biar buka drawer INSTAN +
// NOL bakar token. Pakai kit glass.js. Copy lewat kamus i18n (t('coder.lc_*')). DELETABLE.
import { esc, fetchJSON } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { ensureGlass, statTile, badge, glassBtn, row } from '/js/glass.js';

const T = (k) => t('coder.' + k);
const SEV_TONE = { healthy: 'ok', warn: 'warn', critical: 'bad' };
const VERDICT_TONE = { approved: 'ok', review: 'warn', blocked: 'bad' };

// renderLifecycle — render isi siklus-hidup ke `body`. pending+deaths auto (cepat),
// kesehatan on-demand (tombol) biar ga bakar token tiap buka.
export async function renderLifecycle(body) {
  ensureGlass();
  body.innerHTML = `<div class="gl-empty">${esc(T('lc_loading'))}</div>`;
  let pending = [], letters = [];
  try {
    const [p, l] = await Promise.all([
      fetchJSON('/api/coder/pending').catch(() => ({ pending: [] })),
      fetchJSON('/api/studio/deathletters').catch(() => ({ death_letters: [] })),
    ]);
    pending = p.pending || []; letters = l.death_letters || [];
  } catch (e) {
    body.innerHTML = `<div class="gl-empty" style="color:#f87171">${esc(T('lc_load_fail').replace('{err}', String(e.message || e)))}</div>`;
    return;
  }
  paint(body, pending, letters, null); // null = kesehatan belum dicek
}

// paint — gambar penuh. cands: null=belum dicek (tombol), []=sehat semua, [...]=ada isinya.
function paint(body, pending, letters, cands) {
  const flagged = (cands || []).filter((c) => c.flagged).length;
  const healthN = cands === null ? '·' : (cands.length - flagged);
  const tiles = `<div class="gl-tiles" style="margin-bottom:18px">
    ${statTile('⏳', pending.length, T('lc_pending_title'), '#a78bfa')}
    ${statTile('❤️', healthN, T('lc_health_title'), '#34d399')}
    ${statTile('⚠️', cands === null ? '·' : flagged, 'flagged', '#fbbf24')}
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

  let healthHTML;
  if (cands === null) {
    healthHTML = `<div class="gl-empty" style="display:flex;align-items:center;gap:10px;justify-content:space-between">
      <span>${esc(T('lc_health_ondemand'))}</span><span data-health>${glassBtn('🩺 ' + T('lc_health_check'), 'ok')}</span></div>`;
  } else if (cands.length) {
    healthHTML = cands.map((c) => {
      const sev = c.severity || 'healthy';
      const rate = c.error_rate ? ` · err ${(c.error_rate * 100).toFixed(0)}%` : '';
      const reap = c.flagged ? `<span data-reap="${esc(c.category_id)}">${glassBtn(T('lc_reap'), 'bad')}</span>` : '';
      return row(
        `<span style="flex:1;color:var(--text-main);font-weight:600">${esc(c.name || c.category_id)}` +
        `<span style="color:var(--text-muted);font-weight:400;font-size:.74rem"> (${c.done || 0}✓/${c.error || 0}✗${rate})</span></span>` +
        badge(c.reason_code || sev, SEV_TONE[sev] || 'mute') + reap);
    }).join('');
  } else {
    healthHTML = `<div class="gl-empty">${esc(T('lc_health_empty'))}</div>`;
  }

  const letterHTML = letters.length ? letters.slice(0, 30).map((d) => row(
    `<span style="flex:1;color:var(--text-muted)">⚰️ <b style="color:#cbd5e1">${esc(d.name || d.id)}</b> ${badge(d.kind || '', 'mute')}` +
    `<div style="font-size:.74rem;margin-top:3px">${esc(d.reason || '')} · ${esc((d.at || '').replace('T', ' ').replace('Z', ''))}</div></span>`)
  ).join('') : `<div class="gl-empty">${esc(T('lc_deaths_empty'))}</div>`;

  body.innerHTML = tiles +
    sect(T('lc_pending_title'), T('lc_pending_hint'), pendingHTML) +
    sect(T('lc_health_title'), T('lc_health_hint'), healthHTML) +
    sect(T('lc_deaths_title'), T('lc_deaths_hint'), letterHTML);

  // ── aksi (delegasi) ──────────────────────────────────────────────────────
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
  // cek kesehatan ON-DEMAND (baru di sini panggil reaper yg mahal)
  const hb = body.querySelector('[data-health]');
  if (hb) hb.firstElementChild.onclick = async () => {
    hb.innerHTML = `<span class="gl-empty">${esc(T('lc_health_loading'))}</span>`;
    try { const c = await fetchJSON('/api/reaper/candidates'); paint(body, pending, letters, c.candidates || []); }
    catch (e) { paint(body, pending, letters, []); }
  };
}
