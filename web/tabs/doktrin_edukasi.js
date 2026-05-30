// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Doktrin Edukasi tab (reference 310 LOC). Audit pass — esc() on error_code+title+message_template+evolution_hint..

import { esc, fetchJSON, loadStyle } from '../js/utils.js';

// Doktrin Edukasi (Educational Errors) — list & edit pesan error edukatif
// yang AI terima saat melanggar batasan. Per Ayah 2026-04-25: cuma R + U.
// Tambah/hapus kode error = perubahan kode di brain/db/educational_errors_seed.go.

const CSS = `
.de-shell {
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: calc(100vh - 220px);
  min-height: 520px;
  overflow: hidden;
}
.de-bar {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
  padding: 12px 16px;
  background: rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  backdrop-filter: blur(14px);
  flex-shrink: 0;
}
.de-bar input {
  padding: 7px 12px !important;
  font-size: 0.82rem !important;
  border-radius: 8px !important;
  min-width: 220px;
  flex: 1;
  max-width: 400px;
}
.de-bar .stat { margin-left: auto; font-size: 0.72rem; color: var(--text-muted); font-family: monospace; display: flex; gap: 10px; }
.de-bar .stat b { color: #cbd5e1; font-weight: 700; }

.de-panel {
  background: rgba(15, 17, 26, 0.6);
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  backdrop-filter: blur(14px);
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.de-row {
  display: grid;
  grid-template-columns: minmax(220px, 0.8fr) 1fr auto;
  gap: 14px;
  align-items: start;
  padding: 12px 14px;
  background: rgba(15, 23, 42, 0.4);
  border: 1px solid var(--glass-border);
  border-radius: 10px;
  cursor: pointer;
  transition: border-color 0.18s, background 0.18s;
}
.de-row:hover {
  background: rgba(30, 34, 56, 0.65);
  border-color: rgba(139, 92, 246, 0.35);
}
.de-row.hidden { display: none; }
.de-code {
  font-family: monospace;
  font-size: 0.85rem;
  font-weight: 700;
  color: #c4b5fd;
}
.de-title {
  font-size: 0.7rem;
  color: var(--text-muted);
  margin-top: 4px;
}
.de-preview {
  font-size: 0.78rem;
  color: #cbd5e1;
  line-height: 1.5;
  word-break: break-word;
}
.de-preview .pre-hint {
  display: block;
  color: #94a3b8;
  font-style: italic;
  margin-top: 4px;
  font-size: 0.72rem;
}
.de-edit-btn {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.2), rgba(124, 58, 237, 0.1));
  border: 1px solid rgba(139, 92, 246, 0.4);
  color: #ddd6fe;
  padding: 6px 14px;
  border-radius: 8px;
  font-size: 0.74rem;
  font-weight: 600;
  cursor: pointer;
  align-self: center;
}
.de-edit-btn:hover { background: rgba(139, 92, 246, 0.35); }

.de-modal {
  position: fixed; inset: 0;
  background: rgba(0, 0, 0, 0.55);
  display: flex; align-items: center; justify-content: center;
  z-index: 1000;
}
.de-modal-card {
  background: #11131a;
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  padding: 22px;
  width: min(640px, 92vw);
  max-height: 88vh;
  overflow-y: auto;
  display: flex; flex-direction: column; gap: 14px;
  box-shadow: 0 20px 60px rgba(0,0,0,0.6);
}
.de-modal-card h3 { margin: 0; color: #c4b5fd; }
.de-modal-card label {
  display: block;
  font-size: 0.72rem;
  color: var(--text-muted);
  margin-bottom: 4px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.de-modal-card input,
.de-modal-card textarea {
  width: 100%;
  padding: 8px 12px;
  font-size: 0.82rem;
  background: rgba(0,0,0,0.4);
  border: 1px solid var(--glass-border);
  border-radius: 8px;
  color: #e2e8f0;
  font-family: inherit;
}
.de-modal-card input[readonly] { color: #64748b; cursor: not-allowed; }
.de-modal-card textarea { min-height: 110px; resize: vertical; line-height: 1.55; }
.de-modal-actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 6px; }
.de-modal-actions button {
  padding: 8px 18px;
  font-size: 0.78rem;
  font-weight: 600;
  border-radius: 8px;
  cursor: pointer;
}
.de-btn-cancel { background: transparent; border: 1px solid var(--glass-border); color: var(--text-muted); }
.de-btn-save {
  background: linear-gradient(135deg, #8b5cf6, #7c3aed);
  border: none; color: white;
}
.de-modal-hint {
  font-size: 0.7rem;
  color: var(--text-muted);
  font-style: italic;
  padding: 6px 10px;
  background: rgba(139, 92, 246, 0.06);
  border-left: 2px solid rgba(139, 92, 246, 0.4);
  border-radius: 4px;
}
.de-empty { text-align: center; padding: 40px 20px; color: var(--text-muted); font-style: italic; }
.de-err { color: var(--danger); padding: 12px; background: rgba(239,68,68,0.08); border: 1px solid rgba(239,68,68,0.25); border-radius: 8px; }
`;

export async function render(mainEl) {
  loadStyle('doktrin_edukasi', CSS);

  mainEl.innerHTML = `
    <h2>📚 Doktrin Edukasi (Educational Errors)</h2>
    <div class="sub">Pesan error edukatif yang AI terima saat melanggar batasan. Edit untuk tune tone &amp; doktrin tanpa restart daemon. Tambah/hapus kode = perubahan di kode (<code>brain/db/educational_errors_seed.go</code>).</div>
    <div class="de-shell">
      <div class="de-bar">
        <input type="text" id="deSearch" placeholder="Cari kode/title (mis: TOOL, PROTECTED)…" autocomplete="off" title="Cari Doktrin Edukasi — Filter daftar pesan error edukatif berdasarkan kode (ERR_*) atau title. Berguna saat warga AI report error spesifik dan lo mau audit/edit pesan yang dia terima. Contoh: ketik 'TOOL' untuk semua kode terkait pemakaian tool (ERR_TOOL_NOT_ALLOWED, dll); ketik 'PROTECTED' untuk pesan terkait Protected Core. Logic: client-side filter array entries cached dari /api/settings/educational-errors → setiap keystroke iterate row + match substring case-insensitive di error_code/title fields → toggle hidden class. No backend round-trip.">
        <div class="stat">
          <span><b id="deCount">0</b> kode error</span>
        </div>
      </div>
      <div class="de-panel" id="dePanel">
        <div class="de-empty">Memuat doktrin edukasi…</div>
      </div>
    </div>
  `;

  let entries = [];
  try {
    const resp = await fetchJSON('/api/settings/educational-errors');
    entries = Array.isArray(resp.data) ? resp.data : [];
  } catch (e) {
    document.getElementById('dePanel').innerHTML = `<div class="de-err">Error: ${esc(e.message)}</div>`;
    return;
  }

  document.getElementById('deCount').textContent = entries.length;

  const panel = document.getElementById('dePanel');
  if (!entries.length) {
    panel.innerHTML = '<div class="de-empty">Belum ada entry. Cek seed di <code>brain/db/educational_errors_seed.go</code>.</div>';
    return;
  }

  function renderRows(filter = '') {
    const q = filter.trim().toLowerCase();
    panel.innerHTML = entries.map((e, i) => {
      const matches = !q || e.error_code.toLowerCase().includes(q) || (e.title || '').toLowerCase().includes(q);
      return `
        <div class="de-row${matches ? '' : ' hidden'}" data-idx="${i}">
          <div>
            <div class="de-code">${esc(e.error_code)}</div>
            <div class="de-title">${esc(e.title || '')}</div>
          </div>
          <div class="de-preview">
            ${esc(e.message_template || '')}
            <span class="pre-hint">${esc(e.evolution_hint || '')}</span>
          </div>
          <button class="de-edit-btn" data-idx="${i}" title="Edit Doktrin Edukasi — Buka modal untuk tune teks message_template + evolution_hint dari kode error ini. error_code + title LOCKED (cuma bisa di-edit di educational_errors_seed.go source code). Contoh: kalau warga selalu salah pasif saat dapat ERR_TOOL_NOT_ALLOWED, ubah message_template ke yang lebih actionable + tambah hint 'baca capability matrix di Tasking → Hak & Tools dulu sebelum tool call'. Logic: PUT /api/settings/educational-errors {error_code, message_template, evolution_hint} → handler UPDATE educational_errors di flowork-settings.sqlite. Hot-reload, warga AI dapat pesan baru di error berikutnya tanpa daemon restart.">Edit ✏️</button>
        </div>
      `;
    }).join('');

    panel.querySelectorAll('.de-edit-btn, .de-row').forEach(el => {
      el.addEventListener('click', (ev) => {
        ev.stopPropagation();
        const idx = parseInt(ev.currentTarget.getAttribute('data-idx'), 10);
        if (!isNaN(idx)) openEditModal(entries[idx]);
      });
    });
  }

  function openEditModal(entry) {
    const modal = document.createElement('div');
    modal.className = 'de-modal';
    modal.innerHTML = `
      <div class="de-modal-card">
        <h3>Edit ${esc(entry.error_code)}</h3>
        <div class="de-modal-hint">Title &amp; error_code fixed di kode (read-only). Lo cuma bisa tune teks pesan + hint evolusi yang dilihat AI.</div>
        <div>
          <label>Error Code</label>
          <input type="text" value="${esc(entry.error_code)}" readonly title="Kode Error — Identifier unik error ini (tidak bisa diubah). Kode ini dipakai di source code untuk mapping ke pesan edukatif. Perubahan harus dilakukan di educational_errors_seed.go.">
        </div>
        <div>
          <label>Title (skenario)</label>
          <input type="text" value="${esc(entry.title || '')}" readonly title="Judul Skenario — Deskripsi singkat situasi yang memicu error ini (tidak bisa diubah di GUI). Diubah di source code educational_errors_seed.go.">
        </div>
        <div>
          <label>Message Template (pesan utama, support <code>%s</code> placeholder)</label>
          <textarea id="deMsg" title="Pesan Error — Teks pesan yang diterima warga AI saat melanggar aturan ini. Tulis dengan bahasa edukatif dan jelas. Gunakan %s sebagai placeholder untuk nilai dinamis (contoh: nama file, nama tool).">${esc(entry.message_template || '')}</textarea>
        </div>
        <div>
          <label>Evolution Hint (arahan langkah keluar dari kesalahan)</label>
          <textarea id="deHint" title="Evolution Hint — Panduan langkah konkret yang diberikan ke warga AI untuk keluar dari kesalahan ini dan berkembang. Tulis secara actionable: apa yang harus dilakukan berbeda di langkah berikutnya.">${esc(entry.evolution_hint || '')}</textarea>
        </div>
        <div class="de-modal-actions">
          <button class="de-btn-cancel" id="deCancel" title="Batal — Tutup form tanpa menyimpan perubahan. Teks pesan error tidak berubah.">Batal</button>
          <button class="de-btn-save" id="deSave" title="Simpan — Simpan perubahan pesan error dan evolution hint ke database. Perubahan langsung aktif: warga AI akan menerima teks baru ini saat melanggar aturan yang sama.">Simpan</button>
        </div>
      </div>
    `;
    document.body.appendChild(modal);

    const close = () => modal.remove();
    modal.addEventListener('click', (ev) => { if (ev.target === modal) close(); });
    modal.querySelector('#deCancel').addEventListener('click', close);
    modal.querySelector('#deSave').addEventListener('click', async () => {
      const msg = modal.querySelector('#deMsg').value.trim();
      const hint = modal.querySelector('#deHint').value.trim();
      if (!msg || !hint) {
        alert('Message dan hint tidak boleh kosong.');
        return;
      }
      try {
        const r = await fetch('/api/settings/educational-errors', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            error_code: entry.error_code,
            message_template: msg,
            evolution_hint: hint,
          }),
        });
        // BUG 21 fix: cek r.ok dulu sebelum parse JSON.
        if (!r.ok) {
          const txt = await r.text().catch(() => r.statusText);
          let errMsg = txt;
          try { errMsg = JSON.parse(txt).error || txt; } catch (_) {}
          alert('Error: ' + errMsg);
          return;
        }
        const j = await r.json();
        entry.message_template = msg;
        entry.evolution_hint = hint;
        close();
        renderRows(document.getElementById('deSearch').value);
      } catch (e) {
        alert('Error: ' + e.message);
      }
    });
  }

  renderRows();

  document.getElementById('deSearch').addEventListener('input', (ev) => {
    renderRows(ev.target.value);
  });
}
