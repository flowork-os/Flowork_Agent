// === GROWTH-POINT (NON-frozen) — repurposed 2026-06-25 ===
// Owner: Aola Sahidin (Mr.Dev) · Repo: https://github.com/flowork-os/Flowork-OS
//
// agents_tool_catalog.js — DULU tab subscribe/unsubscribe tool (vestigial pasca all-tools).
// SEKARANG = panel "Agent Brain" per-agent (GUI = sumber kebenaran):
//   • SCOPE INSTING per-peran (#3 RI-5): centang domain (Room) yg boleh ke-inject ke agent ini.
//     baseline universal+tool SELALU (locked). Kosong = scope OFF (fails-open, dapet semua).
//   • DEFER / ALL-TOOLS per-agent (#2C): override ENV global (default = ikut ENV).
// Simpan → POST /api/agents/brain-config (file ~/.flowork/agent_brain_config.json) → dibaca
// router (scope insting) + host (defer policy). Export name TETAP renderToolCatalog (agents.js).

import { t } from '/js/i18n.js';

const API_CFG       = '/api/agents/brain-config';
const API_INSTINCTS = '/api/brain/instincts';
const BASELINE = ['instinct_universal', 'instinct_tool']; // selalu ke-inject (anti-starvation)
const KNOWN_DOMAINS = ['instinct_coding', 'instinct_security', 'instinct_crypto', 'instinct_bisnis'];

function esc(s) {
  return String(s ?? '').replaceAll('&', '&amp;').replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;');
}
const escAttr = esc;
const short = (r) => esc(String(r).replace(/^instinct_/, ''));

// kumpulin domain (Room) + jumlah insting dari brain. Fails-soft → KNOWN_DOMAINS.
async function loadDomains() {
  const counts = {};
  try {
    // Brain (insting) ada di ROUTER :2402; panel ini di host :1987 → proxy same-origin
    // /api/agents/brain-domains (balikin SEMUA room+count, no-cap). DINAMIS: tambah insting
    // room=instinct_<domain> di brain → domain auto muncul di sini. (fails-soft → KNOWN_DOMAINS).
    const r = await fetch('/api/agents/brain-domains');
    const d = await r.json();
    Object.assign(counts, d.rooms || {});
  } catch { /* fails-soft */ }
  // baseline (universal+tool) TETAP ditampilkan (locked/always-on) biar owner liat SEMUA domain (7), bukan ke-hide.
  const domains = new Set([...BASELINE, ...KNOWN_DOMAINS, ...Object.keys(counts)]);
  return { domains: [...domains].sort(), counts };
}

function triSelect(id, val) {
  const opt = (v, label, sel) => `<option value="${v}" ${sel ? 'selected' : ''}>${label}</option>`;
  const cur = val === true ? 'true' : val === false ? 'false' : '';
  return `<select id="${id}" style="background:#0f172a;color:#e2e8f0;border:1px solid #334155;border-radius:5px;font-size:11px;padding:2px 4px">
    ${opt('', 'default (ENV)', cur === '')}${opt('true', 'ON', cur === 'true')}${opt('false', 'OFF', cur === 'false')}
  </select>`;
}

export async function renderToolCatalog(hostEl, agentId) {
  hostEl.innerHTML = `<p style="color:#64748b;font-size:12px">Loading…</p>`;
  try {
    const [{ domains, counts }, cfgResp] = await Promise.all([
      loadDomains(),
      fetch(`${API_CFG}?id=${encodeURIComponent(agentId)}`).then((r) => r.json()).catch(() => ({})),
    ]);
    const cfg = (cfgResp && cfgResp.config) || {};
    const picked = new Set(cfg.instinct_domains || []);

    hostEl.innerHTML = `
      <p style="color:#94a3b8;font-size:11px;margin:0 0 8px 0">
        🧠 Scope insting per-peran. Baseline <b>${BASELINE.map(short).join(' + ')}</b> selalu masuk.
        Centang domain = batasi agent ke domain itu. Kosong = OFF (dapet semua). Butuh master switch
        <code>FLOWORK_INSTINCT_SCOPED=1</code>.
      </p>
      <div style="display:grid;gap:4px;max-height:200px;overflow-y:auto;margin-bottom:8px">
        ${domains.map((d) => { const base = BASELINE.includes(d); return `
          <label style="display:flex;align-items:center;gap:8px;padding:6px;background:#1e293b;border:1px solid #334155;border-radius:6px;cursor:${base ? 'default' : 'pointer'};${base ? 'opacity:.65' : ''}">
            <input type="checkbox" data-domain="${escAttr(d)}" ${base || picked.has(d) ? 'checked' : ''} ${base ? 'disabled' : ''}>
            <span style="flex:1;color:#f1f5f9;font-family:ui-monospace,monospace;font-size:12px">${short(d)}${base ? ' <span style="color:#64748b">· baseline (selalu)</span>' : ''}</span>
            <span style="color:#64748b;font-size:11px">${counts[d] ? counts[d] + ' insting' : ''}</span>
          </label>`; }).join('')}
      </div>
      <div style="display:flex;gap:14px;align-items:center;flex-wrap:wrap;font-size:11px;color:#94a3b8;margin-bottom:8px">
        <span>defer-tools ${triSelect('cf-defer', cfg.defer_tools)}</span>
        <span>all-tools ${triSelect('cf-expose', cfg.expose_all)}</span>
      </div>
      <button id="cf-brain-save" style="background:#2563eb;color:#fff;border:0;border-radius:6px;padding:6px 14px;font-size:12px;cursor:pointer">Simpan</button>
      <span id="cf-brain-status" style="margin-left:10px;font-size:11px;color:#94a3b8"></span>
      <p style="margin-top:10px;font-size:10px;color:#64748b">➕ Tambah/edit insting di tab <b>Brain → Instincts</b> (pilih domain di situ) — auto muncul di sini.</p>
    `;

    const statusEl = hostEl.querySelector('#cf-brain-status');
    hostEl.querySelector('#cf-brain-save').addEventListener('click', async () => {
      const instinct_domains = [...hostEl.querySelectorAll('input[data-domain]:checked')].map((c) => c.dataset.domain).filter((d) => !BASELINE.includes(d)); // baseline always-on di router → ga perlu disimpan
      const triVal = (id) => { const v = hostEl.querySelector('#' + id).value; return v === '' ? null : v === 'true'; };
      statusEl.textContent = 'Menyimpan…'; statusEl.style.color = '#94a3b8';
      try {
        const r = await fetch(API_CFG, {
          method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ agent: agentId, instinct_domains, defer_tools: triVal('cf-defer'), expose_all: triVal('cf-expose') }),
        });
        const d = await r.json();
        if (d.error) { statusEl.textContent = d.error; statusEl.style.color = '#f87171'; return; }
        statusEl.textContent = `✓ tersimpan (${instinct_domains.length} domain) — efek instan (router baca file)`;
        statusEl.style.color = '#34d399';
      } catch (err) { statusEl.textContent = String(err); statusEl.style.color = '#f87171'; }
    });
  } catch (err) {
    hostEl.innerHTML = `<p style="color:#f87171;font-size:12px">${esc(String(err))}</p>`;
  }
}
