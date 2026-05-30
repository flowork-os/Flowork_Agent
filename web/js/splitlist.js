// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Split-list pane component. Audit pass — esc() on rendered items, no eval..

import { fetchJSON, esc, mdToHtml, ago } from './utils.js';

// Chunk size — render N items per batch, sisanya via IntersectionObserver.
// 50 cukup untuk fill viewport (≈10-15 row visible) tanpa render-blocking
// di awal. Naikin kalau dataset sering <50 (misal docs/changelog ~30 entry).
const CHUNK_SIZE = 50;

// renderGenericSplitList — split-pane (list left, content right).
//
// Optimasi rc141 (2026-04-25): list panel pakai chunked rendering supaya
// kalau dataset 1000+ items, browser ga lag render semua sekaligus. Pattern:
// render 50 dulu, attach sentinel di akhir, IntersectionObserver detect saat
// user scroll mendekati bottom → render 50 berikutnya. Stop saat semua kerender.
// Content panel (kanan) tetap lazy (klik baru fetch detail) — unchanged.
// Bug Gemini #52 fix (2026-04-27): tambah optional emptyMessage param supaya
// tab spesifik (mis. changelog) bisa kasih konteks "kenapa data kosong + apa
// yang harus dilakukan", bukan generic "Data Kosong" yang misleading.
// Caller lama tanpa argumen tetap dapat default "Data Kosong." (back-compat).
export async function renderGenericSplitList(mainEl, title, subtitle, fetchEndpoint, processItemsFn, contentLoaderFn, emptyMessage) {
  mainEl.innerHTML = `
    <h2>${title}</h2>
    <div class="sub">${subtitle}</div>
    <div class="split">
      <div class="list" id="splitlist">
        <div class="empty">Memuat meta data...</div>
      </div>
      <div class="viewer" id="splitview">
        <div class="empty">Klik file di panel kiri.</div>
      </div>
    </div>
  `;

  try {
    const data = await fetchJSON(fetchEndpoint);
    const items = processItemsFn(data);
    const lst = document.getElementById('splitlist');

    if (!items.length) {
      const msg = emptyMessage || 'Data Kosong.';
      lst.innerHTML = `<div class="empty">${esc(msg)}</div>`;
      const v = document.getElementById('splitview');
      if (v) v.innerHTML = '<div class="empty">Tidak ada item untuk ditampilkan.</div>';
      return;
    }

    // State for chunked rendering — closure di sini supaya per-render
    // kebebasan, ngga ganggu render lain di tab lain.
    let renderedCount = 0;
    let observer = null;
    let sentinel = null;

    const onRowClick = async (i, row) => {
      lst.querySelectorAll('.lr').forEach(r => r.style.borderLeftColor = 'transparent');
      row.style.borderLeftColor = 'var(--accent)';
      const v = document.getElementById('splitview');
      v.innerHTML = '<div class="empty">Memuat konten via stream...</div>';
      try {
        const rawContent = await contentLoaderFn(items[i]);
        v.innerHTML = mdToHtml(rawContent);
        if (window.mermaid) {
          try { await window.mermaid.run({ querySelector: '.mermaid' }); } catch(me) { console.error('Mermaid render error:', me); }
        }
      } catch (e) {
        v.innerHTML = `<div class="err">❌ Gagal muat: ${esc(e.message)}</div>`;
      }
    };

    function renderChunk() {
      const start = renderedCount;
      const end = Math.min(start + CHUNK_SIZE, items.length);
      const fragment = document.createDocumentFragment();
      for (let i = start; i < end; i++) {
        const it = items[i];
        const row = document.createElement('div');
        row.className = 'lr';
        row.dataset.i = String(i);
        row.innerHTML = `<div class="la">${esc(it.title)}</div><div class="ls">${esc(it.sub)}</div>`;
        row.addEventListener('click', () => onRowClick(i, row));
        fragment.appendChild(row);
      }
      lst.appendChild(fragment);
      renderedCount = end;
    }

    function attachOrUpdateSentinel() {
      if (renderedCount >= items.length) {
        if (sentinel) sentinel.remove();
        if (observer) observer.disconnect();
        return;
      }
      if (!sentinel) {
        sentinel = document.createElement('div');
        sentinel.className = 'splitlist-sentinel';
        sentinel.style.cssText = 'padding: 8px; text-align: center; opacity: 0.5; font-size: 11px;';
      }
      sentinel.textContent = `… ${items.length - renderedCount} item lagi (scroll)`;
      lst.appendChild(sentinel);

      if (!observer) {
        // rootMargin 200px — pre-load before sentinel hits viewport supaya
        // render mulus, ngga ke-jeda di tepi scroll.
        observer = new IntersectionObserver(([entry]) => {
          if (!entry.isIntersecting) return;
          sentinel.remove();
          renderChunk();
          attachOrUpdateSentinel();
        }, { rootMargin: '200px' });
        observer.observe(sentinel);
      }
    }

    // Initial render
    lst.innerHTML = '';
    renderChunk();
    attachOrUpdateSentinel();
  } catch (err) {
    const lst = document.getElementById('splitlist');
    if (lst) lst.innerHTML = `<div class="err">❌ Gagal fetch API: ${esc(err.message)}</div>`;
  }
}
