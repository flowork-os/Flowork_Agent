// Prompt Library tab — CRUD canonical prompt templates. Agent assign template
// via dropdown di Warga tab (no retype prompt per agent). Plug-and-play.
import { esc, fetchJSON, loadStyle } from '../js/utils.js';

const CSS = `
.pl-shell { padding:16px; display:flex; flex-direction:column; gap:14px; min-height:70vh; }
.pl-head { display:flex; justify-content:space-between; align-items:center; gap:12px; flex-wrap:wrap; }
.pl-head h2 { margin:0; font-size:1.2rem; }
.pl-sub { color:#94a3b8; font-size:0.8rem; }
.pl-actions { display:flex; gap:8px; }
.pl-btn-new {
  padding:8px 14px; background:#10b981; color:#fff; border:none; border-radius:6px;
  cursor:pointer; font-weight:600; font-size:0.85rem;
}
.pl-btn-new:hover { background:#059669; }

.pl-grid {
  display:grid; grid-template-columns:repeat(auto-fill, minmax(280px, 1fr));
  gap:12px;
}

.pl-card {
  padding:12px; border-radius:8px; background:rgba(15,17,26,0.5);
  border:1px solid rgba(148,163,184,0.18);
  display:flex; flex-direction:column; gap:8px;
}
.pl-card-head { display:flex; justify-content:space-between; align-items:center; gap:8px; }
.pl-card-name { font-weight:600; color:#e2e8f0; font-size:0.95rem; }
.pl-card-usage {
  padding:2px 8px; background:rgba(59,130,246,0.12); color:#60a5fa;
  border-radius:10px; font-size:0.7rem;
}
.pl-card-preview {
  font-size:0.75rem; color:#94a3b8;
  max-height:84px; overflow:hidden;
  background:rgba(0,0,0,0.2); padding:6px 8px; border-radius:4px;
  font-family:monospace; line-height:1.4;
  white-space:pre-wrap;
}
.pl-card-meta { display:flex; gap:12px; color:#64748b; font-size:0.7rem; }
.pl-card-actions { display:flex; gap:6px; justify-content:flex-end; }
.pl-card-btn {
  padding:4px 10px; font-size:0.72rem; border:none; border-radius:4px;
  cursor:pointer; font-weight:500;
}
.pl-btn-view  { background:#1e293b; color:#cbd5e1; }
.pl-btn-edit  { background:#2563eb; color:#fff; }
.pl-btn-delete { background:#7f1d1d; color:#fecaca; }

.pl-modal {
  position:fixed; inset:0; background:rgba(0,0,0,0.75); display:flex;
  align-items:center; justify-content:center; z-index:9999;
  backdrop-filter:blur(4px);
}
.pl-modal-box {
  background:#0f172a; border:1px solid #334155; border-radius:12px;
  padding:20px; max-width:720px; width:92%; max-height:88vh;
  display:flex; flex-direction:column; gap:12px; color:#e2e8f0;
}
.pl-modal-head { display:flex; justify-content:space-between; align-items:center; }
.pl-modal-head h3 { margin:0; font-size:1.05rem; }
.pl-modal-close {
  background:transparent; border:none; color:#94a3b8; font-size:1.4rem; cursor:pointer;
}
.pl-field { display:flex; flex-direction:column; gap:4px; }
.pl-field label { font-size:0.8rem; color:#cbd5e1; }
.pl-field input, .pl-field textarea {
  padding:8px; background:#1e293b; border:1px solid #334155;
  border-radius:6px; color:#e2e8f0; font-family:monospace; font-size:0.82rem;
}
.pl-field textarea { resize:vertical; min-height:320px; line-height:1.5; }
.pl-modal-footer {
  display:flex; justify-content:space-between; align-items:center; gap:10px;
  border-top:1px solid #334155; padding-top:12px;
}
.pl-hint { font-size:0.72rem; color:#64748b; }
.pl-btn-save { padding:8px 16px; background:#10b981; color:#fff; border:none; border-radius:6px; cursor:pointer; font-weight:600; }
.pl-btn-cancel { padding:8px 16px; background:#334155; color:#e2e8f0; border:none; border-radius:6px; cursor:pointer; }

.pl-empty { text-align:center; color:#64748b; padding:40px; font-size:0.9rem; }
`;

export async function render(root) {
  loadStyle('prompt', CSS);
  root.innerHTML = `
    <div class="pl-shell">
      <div class="pl-head">
        <div>
          <h2>📝 Prompt Library</h2>
          <div class="pl-sub">Canonical prompt templates — shared across warga via dropdown. Edit sekali, propagate ke semua agen yang pake.</div>
        </div>
        <div class="pl-actions">
          <button class="pl-btn-new" id="plNew" title="Buat Template Baru — Buka editor untuk bikin prompt template baru yang bisa di-share ke banyak warga via dropdown di tab Warga. Edit sekali, semua warga yang pakai auto-update di sesi berikutnya. Contoh: bikin 'merpati-casual' dengan content 'Lo merpati, bahasa santai, fokus Telegram bridge' → assign ke warga merpati + warga ombak yang juga handle social. Logic: POST /api/brain/prompt-templates {name, content} → handler validate regex name (^[a-z0-9][a-z0-9_-]{1,63}$) + min 10 chars content → INSERT INTO prompt_templates (PK: name) di flowork-brain.sqlite. Daemon warga restart dibutuhkan untuk reload template ke memory cache (system prompt cached saat boot).">+ Template Baru</button>
        </div>
      </div>
      <div id="plGrid" class="pl-grid"></div>
    </div>
  `;
  document.getElementById('plNew').onclick = () => openEditor(null);
  await loadList();
}

async function loadList() {
  const grid = document.getElementById('plGrid');
  grid.innerHTML = '<div class="pl-empty">Memuat templates...</div>';
  try {
    const d = await fetchJSON('/api/brain/prompt-templates');
    const templates = d.templates || [];
    if (templates.length === 0) {
      grid.innerHTML = '<div class="pl-empty">Belum ada template. Klik "+ Template Baru" untuk mulai.</div>';
      return;
    }
    grid.innerHTML = templates.map(t => cardHTML(t)).join('');
    grid.querySelectorAll('[data-action="view"]').forEach(b => b.onclick = () => openViewer(b.dataset.name));
    grid.querySelectorAll('[data-action="edit"]').forEach(b => b.onclick = () => openEditor(b.dataset.name));
    grid.querySelectorAll('[data-action="delete"]').forEach(b => b.onclick = () => handleDelete(b.dataset.name, Number(b.dataset.used)));
  } catch (e) {
    grid.innerHTML = `<div class="pl-empty">❌ Gagal load: ${esc(e.message)}</div>`;
  }
}

function cardHTML(t) {
  const usage = t.usage_count > 0
    ? `<span class="pl-card-usage">${t.usage_count} warga pake</span>`
    : `<span class="pl-card-usage" style="background:rgba(100,116,139,0.12);color:#94a3b8">unused</span>`;
  const updated = t.updated_at ? t.updated_at.split('T')[0] : '—';
  return `
    <div class="pl-card">
      <div class="pl-card-head">
        <span class="pl-card-name">${esc(t.name)}</span>
        ${usage}
      </div>
      <div class="pl-card-preview">${esc(t.preview || '').slice(0, 300)}</div>
      <div class="pl-card-meta">
        <span>${t.content_size} chars</span>
        <span>updated ${esc(updated)}</span>
      </div>
      <div class="pl-card-actions">
        <button class="pl-card-btn pl-btn-view"   data-action="view"   data-name="${esc(t.name)}" title="Lihat Template — Tampilkan isi lengkap prompt template ${esc(t.name)} + daftar warga yang reference dia (used_by). Read-only, modal akan muncul. Contoh: sebelum lo edit template, klik View dulu untuk audit konten + lihat siapa yang affected (kalau used_by 5 warga, semua mereka akan dapat update). Logic: GET /api/brain/prompt-templates/detail?name=${esc(t.name)} → JOIN dengan agents.prompt_template untuk used_by list. Modal show full content (no truncation) + size info.">👁️ View</button>
        <button class="pl-card-btn pl-btn-edit"   data-action="edit"   data-name="${esc(t.name)}" title="Edit Template — Buka editor untuk modifikasi prompt template ${esc(t.name)}. Nama LOCKED (cegah breakage FK ke agents.prompt_template), cuma content editable. Contoh: tweak tone template 'aksara-coder' dari formal ke casual lo/gw → semua warga yang pakai template ini auto-adopt setelah daemon restart. Logic: POST /api/brain/prompt-templates/update {name, content} → handler UPDATE prompt_templates SET content=?, updated_at=now WHERE name=?. Cache invalidation di sisi daemon (warga reload prompt saat next boot).">✏️ Edit</button>
        <button class="pl-card-btn pl-btn-delete" data-action="delete" data-name="${esc(t.name)}" data-used="${t.usage_count}" title="Hapus Template — Hapus prompt template ${esc(t.name)} dari database. Kalau masih dipakai (usage_count>0), confirmation muncul + force=true bakal clear FK reference di agents.prompt_template (warga affected fall back ke default prompt). Contoh: hapus template lama 'merpati-v1' yang sudah deprecated, replace dengan 'merpati-v2'. Logic: POST /api/brain/prompt-templates/delete {name, force?} → handler kalau usage>0 + force=false return 409, kalau force=true UPDATE agents SET prompt_template=NULL WHERE prompt_template=name + DELETE FROM prompt_templates.">🗑️</button>
      </div>
    </div>`;
}

async function openViewer(name) {
  try {
    const d = await fetchJSON('/api/brain/prompt-templates/detail?name=' + encodeURIComponent(name));
    const usedList = (d.used_by || []).map(u => `@${u.name}`).join(', ') || '(tidak dipake warga manapun)';
    showModal(`👁️ ${d.name}`, `
      <div class="pl-field">
        <label>Content (${d.content.length} chars)</label>
        <textarea readonly>${esc(d.content)}</textarea>
      </div>
      <div class="pl-field">
        <label>Dipakai oleh (${d.used_count} warga)</label>
        <div style="padding:8px;background:#1e293b;border-radius:4px;font-size:0.82rem;">${esc(usedList)}</div>
      </div>
    `, []);
  } catch (e) {
    alert('Gagal load detail: ' + e.message);
  }
}

async function openEditor(name) {
  let existing = { name: '', content: '' };
  if (name) {
    try {
      const d = await fetchJSON('/api/brain/prompt-templates/detail?name=' + encodeURIComponent(name));
      existing = { name: d.name, content: d.content };
    } catch (e) {
      alert('Gagal load: ' + e.message); return;
    }
  }
  const isNew = !name;
  showModal(isNew ? '+ Template Baru' : `✏️ Edit ${existing.name}`, `
    <div class="pl-field">
      <label>Nama (lowercase, dash/underscore OK, 2-64 chars)${isNew ? '' : ' — LOCKED'}</label>
      <input type="text" id="plName" value="${esc(existing.name)}" ${isNew ? '' : 'readonly'} placeholder="contoh: merpati-casual, bughunter-verbose" title="Nama Template — Nama unik untuk template ini (lowercase, bisa pakai dash/underscore, 2-64 karakter). Contoh: merpati-casual, bughunter-verbose. Nama tidak bisa diubah setelah dibuat.">
    </div>
    <div class="pl-field">
      <label>Content (markdown)</label>
      <textarea id="plContent" placeholder="Lo adalah ..." title="Konten Prompt — Teks lengkap prompt sistem yang akan digunakan warga AI. Tulis dalam bahasa Indonesia informal (lo/gw) sesuai doktrin Flowork. Template ini dibaca sebagai system prompt di awal setiap sesi.">${esc(existing.content)}</textarea>
    </div>
  `, [
    { label: 'Batal', class: 'pl-btn-cancel', action: 'close' },
    { label: isNew ? 'Create' : 'Update', class: 'pl-btn-save', action: async (close) => {
      const nm = document.getElementById('plName').value.trim().toLowerCase();
      const ct = document.getElementById('plContent').value.trim();
      if (!nm || ct.length < 10) { alert('Nama wajib + content min 10 chars'); return; }
      try {
        if (isNew) {
          await fetchJSON('/api/brain/prompt-templates', {
            method: 'POST', body: JSON.stringify({ name: nm, content: ct })
          });
        } else {
          await fetchJSON('/api/brain/prompt-templates/update', {
            method: 'POST', body: JSON.stringify({ name: nm, content: ct })
          });
        }
        close();
        await loadList();
      } catch (e) { alert('Error: ' + e.message); }
    }},
  ]);
}

async function handleDelete(name, usedCount) {
  const warn = usedCount > 0
    ? `⚠️  Template "${name}" masih dipake ${usedCount} warga. Delete dengan force=true akan clear reference dari agent mereka.`
    : `Yakin hapus template "${name}"?`;
  if (!confirm(warn)) return;
  const force = usedCount > 0;
  try {
    await fetchJSON('/api/brain/prompt-templates/delete', {
      method: 'POST', body: JSON.stringify({ name, force })
    });
    await loadList();
  } catch (e) { alert('Error: ' + e.message); }
}

function showModal(title, bodyHTML, buttons) {
  const modal = document.createElement('div');
  modal.className = 'pl-modal';
  const close = () => modal.remove();
  modal.innerHTML = `
    <div class="pl-modal-box">
      <div class="pl-modal-head">
        <h3>${esc(title)}</h3>
        <button class="pl-modal-close" id="plClose">×</button>
      </div>
      ${bodyHTML}
      <div class="pl-modal-footer">
        <div class="pl-hint">Daemon restart diperlukan untuk apply ke agent yang running.</div>
        <div style="display:flex;gap:8px;" id="plButtons"></div>
      </div>
    </div>`;
  document.body.appendChild(modal);
  document.getElementById('plClose').onclick = close;
  modal.onclick = (e) => { if (e.target === modal) close(); };
  const btnRoot = document.getElementById('plButtons');
  for (const b of buttons) {
    const el = document.createElement('button');
    el.className = b.class;
    el.textContent = b.label;
    el.onclick = async () => {
      if (b.action === 'close') close();
      else if (typeof b.action === 'function') await b.action(close);
    };
    btnRoot.appendChild(el);
  }
  // Always render a close button if no buttons provided
  if (buttons.length === 0) {
    const el = document.createElement('button');
    el.className = 'pl-btn-cancel';
    el.textContent = 'Tutup';
    el.onclick = close;
    btnRoot.appendChild(el);
  }
}
