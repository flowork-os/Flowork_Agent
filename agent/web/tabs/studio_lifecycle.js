// studio_lifecycle.js — PANEL SIKLUS HIDUP (ROADMAP_AI_STUDIO F3). 1 layar liat semua
// kemampuan + kesehatannya + tombol periksa/setujui/buang. Gabung 3 sumber yang UDAH ada:
//   /api/coder/pending      → nunggu approve (verdict VERIFIER nempel)   [Coder→Verifier]
//   /api/reaper/candidates  → kesehatan tiap kemampuan (sehat/sakit/mati) [Reaper]
//   /api/studio/deathletters→ surat kematian (apa yg mati + kenapa)       [Death-Letter]
//
// Owner: Aola Sahidin (Mr.Dev) · github.com/flowork-os/Flowork-OS · floworkos.com (white-label)
// Self-contained + DELETABLE: hapus file ini + 1 import di coder.js → panel ilang, chat utuh.
import { esc, fetchJSON } from '../js/utils.js';

const SEV_COLOR = { healthy: '#34d399', warn: '#fbbf24', critical: '#f87171' };
const VERDICT_COLOR = { approved: '#34d399', review: '#fbbf24', blocked: '#f87171' };

function badge(text, color) {
  return `<span style="font-size:.7rem;font-weight:700;padding:2px 8px;border-radius:999px;background:${color}22;color:${color};border:1px solid ${color}55">${esc(text)}</span>`;
}

function row(inner) {
  return `<div style="display:flex;align-items:center;gap:10px;padding:8px 10px;border:1px solid #ffffff14;border-radius:9px;background:#ffffff06;margin-bottom:6px">${inner}</div>`;
}

function btn(label, color) {
  return `<button style="font-size:.74rem;font-weight:700;padding:4px 11px;border-radius:7px;border:1px solid ${color}66;background:${color}1a;color:${color};cursor:pointer">${esc(label)}</button>`;
}

// renderLifecycle — mount panel ke `container` (dipanggil dari tab AI Studio).
export async function renderLifecycle(container) {
  container.innerHTML = `<div style="color:#94a3b8;font-size:.85rem;padding:10px">Memuat siklus hidup…</div>`;
  let pending = [], cands = [], letters = [];
  try {
    const [p, c, l] = await Promise.all([
      fetchJSON('/api/coder/pending').catch(() => ({ pending: [] })),
      fetchJSON('/api/reaper/candidates').catch(() => ({ candidates: [] })),
      fetchJSON('/api/studio/deathletters').catch(() => ({ death_letters: [] })),
    ]);
    pending = p.pending || [];
    cands = c.candidates || [];
    letters = l.death_letters || [];
  } catch (e) {
    container.innerHTML = row(`<span style="color:#f87171">Gagal memuat: ${esc(String(e.message || e))}</span>`);
    return;
  }

  // ── Nunggu Persetujuan (Coder → Verifier) ───────────────────────────────────
  const pendingHTML = pending.length ? pending.map((p) => {
    const id = p.id || (p.spec && p.spec.category_id) || '';
    const name = (p.spec && p.spec.name) || id;
    const v = p.verify || {};
    const vb = badge(`${v.status || '?'}${v.score != null ? ' ' + v.score : ''}`, VERDICT_COLOR[v.status] || '#94a3b8');
    return row(
      `<span style="flex:1;color:#e2e8f0;font-weight:600">${esc(name)}</span>${vb}` +
      `<span data-approve="${esc(id)}">${btn('Setujui', '#34d399')}</span>` +
      `<span data-reject="${esc(id)}">${btn('Tolak', '#f87171')}</span>`
    );
  }).join('') : `<div style="color:#64748b;font-size:.8rem;padding:6px 10px">Nol antrian — ga ada kemampuan nunggu approve.</div>`;

  // ── Kesehatan (Reaper) ───────────────────────────────────────────────────────
  const healthHTML = cands.length ? cands.map((c) => {
    const sev = c.severity || 'healthy';
    const hb = badge(c.reason_code || sev, SEV_COLOR[sev] || '#94a3b8');
    const rate = c.error_rate ? ` · err ${(c.error_rate * 100).toFixed(0)}%` : '';
    const reap = c.flagged ? `<span data-reap="${esc(c.category_id)}">${btn('Buang', '#f87171')}</span>` : '';
    return row(
      `<span style="flex:1;color:#e2e8f0;font-weight:600">${esc(c.name || c.category_id)}` +
      `<span style="color:#64748b;font-weight:400;font-size:.76rem">  (${c.done || 0}✓/${c.error || 0}✗${rate})</span></span>${hb}${reap}`
    );
  }).join('') : `<div style="color:#64748b;font-size:.8rem;padding:6px 10px">Belum ada kemampuan terpasang.</div>`;

  // ── Surat Kematian (Death-Letter) ───────────────────────────────────────────
  const letterHTML = letters.length ? letters.slice(0, 30).map((d) => row(
    `<span style="flex:1;color:#cbd5e1">⚰️ <b>${esc(d.name || d.id)}</b> <span style="color:#64748b;font-size:.76rem">${esc(d.kind || '')}</span>` +
    `<div style="color:#94a3b8;font-size:.76rem;margin-top:2px">${esc(d.reason || '')} · ${esc((d.at || '').replace('T', ' ').replace('Z', ''))}</div></span>`
  )).join('') : `<div style="color:#64748b;font-size:.8rem;padding:6px 10px">Belum ada yang mati. (Surat kematian dicatat pas kemampuan dibuang.)</div>`;

  const section = (title, hint, body) =>
    `<div style="margin-bottom:16px">
       <div style="font-size:.82rem;font-weight:700;color:#a5b4fc;margin-bottom:6px">${esc(title)} <span style="color:#64748b;font-weight:400">${esc(hint)}</span></div>
       ${body}
     </div>`;

  container.innerHTML = `
    <div style="background:#0b1220;border:1px solid #ffffff12;border-radius:13px;padding:16px 18px;margin-bottom:18px">
      <div style="display:flex;align-items:center;gap:10px;margin-bottom:14px">
        <span style="font-size:1.1rem">🔄</span>
        <b style="color:#e2e8f0;font-size:.98rem">Siklus Hidup Kemampuan</b>
        <span style="color:#64748b;font-size:.78rem">Coder → Verifier → Reaper → Death-Letter</span>
        <button id="lcRefresh" style="margin-left:auto;font-size:.74rem;padding:4px 11px;border-radius:7px;border:1px solid #ffffff22;background:#ffffff10;color:#cbd5e1;cursor:pointer">⟳ Segarkan</button>
      </div>
      ${section('Nunggu Persetujuan', '(Verifier udah periksa — owner setujui/tolak)', pendingHTML)}
      ${section('Kesehatan', '(Reaper: sehat / sakit / mati — owner yang mutusin buang)', healthHTML)}
      ${section('Surat Kematian', '(apa yang mati + kenapa)', letterHTML)}
    </div>`;

  // ── Aksi (delegasi klik) ────────────────────────────────────────────────────
  container.querySelector('#lcRefresh').onclick = () => renderLifecycle(container);
  const act = async (url, confirmMsg) => {
    if (confirmMsg && !confirm(confirmMsg)) return;
    try {
      await fetchJSON(url, { method: 'POST' });
      renderLifecycle(container);
    } catch (e) {
      alert('Gagal: ' + (e.message || e));
    }
  };
  container.querySelectorAll('[data-approve]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/coder/approve?id=${encodeURIComponent(el.dataset.approve)}`));
  container.querySelectorAll('[data-reject]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/coder/reject?id=${encodeURIComponent(el.dataset.reject)}`, 'Tolak & buang kemampuan pending ini?'));
  container.querySelectorAll('[data-reap]').forEach((el) =>
    el.firstElementChild.onclick = () => act(`/api/reaper/reap?category=${encodeURIComponent(el.dataset.reap)}`, 'Buang kemampuan ini? (akan dicatat di Surat Kematian)'));
}
