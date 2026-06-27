// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// js/ratelimit_badge.js — BADGE SADAR-KUOTA di header (statusline ala Claude Code). Polling
// /api/router/ratelimit (proxy → router :2402, 1 state share) → nampilin "5h X%" + warna
// (ijo<80 / kuning<95 / merah>=95 atau surpassed). Hijau-kuning-merah = sinyal kapan ngerem.
// Self-contained + DELETABLE: hapus file + 1 import di app.js → badge ilang, app utuh.

let timer = null;

function colorFor(util, surpassed) {
  if (surpassed || util >= 0.95) return '#f87171';
  if (util >= 0.80) return '#fbbf24';
  return '#34d399';
}

async function poll(el) {
  try {
    const r = await fetch('/api/router/ratelimit', { cache: 'no-store' });
    const d = await r.json();
    if (!d || !d.seen) { el.style.display = 'none'; return; }
    const u = Math.round((d.util_5h || 0) * 100);
    const c = colorFor(d.util_5h || 0, d.surpassed_5h);
    el.style.display = '';
    el.style.color = c;
    el.style.borderColor = c + '66';
    el.style.background = c + '1a';
    el.title = `Kuota langganan 5-jam: ${u}%` + (d.util_7d ? ` · 7-hari: ${Math.round(d.util_7d * 100)}%` : '') +
      (d.surpassed_5h ? ' · LEWAT AMBANG (router auto-rem ke fallback)' : '');
    el.innerHTML = `🔋 5h ${u}%`;
  } catch { el.style.display = 'none'; }
}

// initRateLimitBadge — sisipin badge ke header + mulai polling (30 dtk). Idempotent.
export function initRateLimitBadge() {
  const header = document.querySelector('header');
  if (!header || document.getElementById('rlBadge')) return;
  const el = document.createElement('span');
  el.id = 'rlBadge';
  el.style.cssText = 'display:none;font-size:.74rem;font-weight:700;padding:5px 11px;border-radius:999px;' +
    'border:1px solid transparent;letter-spacing:.3px;margin-left:4px;white-space:nowrap;cursor:default';
  // taruh sebelum tombol logout kalau ada, kalau ga di ujung header.
  const logout = header.querySelector('#logout-link') || header.querySelector('.header-logout');
  if (logout) header.insertBefore(el, logout); else header.appendChild(el);
  poll(el);
  if (timer) clearInterval(timer);
  timer = setInterval(() => poll(el), 30000);
}
