// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Shared utils (529 LOC): esc, fetchJSON, loadStyle, tryAutoVerify. Audit pass — esc() proper HTML escape, fetchJSON throws on non-2xx, tryAutoVerify graceful on error..

import { t } from './i18n.js';

export const A = window.location.origin;
// Single warga: mr.dev (eks-merpati, di-rename setelah cleanup).
// Static fallback — will be overridden by dynamic DB load.
let _wargaCache = ['all', 'mr.dev'];
export const WARGA_ALL = _wargaCache;

// Dynamic loader — call once at app boot. Currently /api/brain/agents
// di mock-server return empty list; ketika nanti wired ke flow_router,
// dynamic list akan menggantikan static.
export async function loadWargaFromBrain() {
  try {
    const r = await fetch(A + '/api/brain/agents');
    const d = await r.json();
    if (d.data && d.data.length > 0) {
      const names = d.data.map(a => a.name);
      _wargaCache.length = 0;
      _wargaCache.push('all', ...names);
    }
  } catch (e) { /* fallback to static list */ }
}

export function esc(s) {
  return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// escAttr — safe untuk inject ke HTML attribute value="...". Escape juga
// kutip ganda + tunggal supaya tidak pecahin attribute boundary.
// Pakai ini buat title="..." / data-foo="..." / value="..." dll.
// Bug fix 2026-04-27: setting.js tooltip mengandung kutip ganda ("Compact
// Memory") yang sebelumnya cuma di-esc → break attribute → tab navigation
// rusak + JS parse error 'Unexpected identifier "data"'.
export function escAttr(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

export function ts(s) {
  if (!s) return '';
  const d = new Date(s);
  return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
}

export function ago(s) {
  if (!s) return '-';
  const diff = (Date.now() - new Date(s)) / 60000;
  if (diff < 1) return 'baru saja';
  if (diff < 60) return Math.round(diff) + ' menit lalu';
  if (diff < 1440) return Math.round(diff / 60) + ' jam lalu';
  return Math.round(diff / 1440) + ' hari lalu';
}

const OWNER_PWD_KEY = 'flowork_owner_pwd';

function getOwnerPassword() {
  try { return sessionStorage.getItem(OWNER_PWD_KEY) || ''; } catch { return ''; }
}
function setOwnerPassword(pwd) {
  try { sessionStorage.setItem(OWNER_PWD_KEY, pwd); } catch {}
}
function clearOwnerPassword() {
  try { sessionStorage.removeItem(OWNER_PWD_KEY); } catch {}
}

async function promptOwnerPassword(reason) {
  const pwd = window.prompt(reason || 'Owner password (FLOWORK_OWNER_PASSWORD):');
  if (pwd) setOwnerPassword(pwd);
  return pwd;
}

async function doFetch(url, opts) {
  const o = { ...(opts || {}) };
  o.headers = { ...(o.headers || {}) };
  if (o.body && !o.headers['Content-Type']) {
    o.headers['Content-Type'] = 'application/json';
  }
  const isMutative = o.method && o.method.toUpperCase() !== 'GET' && o.method.toUpperCase() !== 'OPTIONS';
  if (isMutative) {
    const pwd = getOwnerPassword();
    if (pwd) o.headers['X-Flowork-Password'] = pwd;
  }
  return fetch(A + url, o);
}

let _autoVerifyPromise = null;
function tryAutoVerify() {
  if (_autoVerifyPromise) return _autoVerifyPromise;
  _autoVerifyPromise = fetch(A + '/api/owner/auto-verify')
    .catch(() => null);
  return _autoVerifyPromise;
}

export async function fetchJSON(url, opts) {
  await tryAutoVerify();
  let r = await doFetch(url, opts);
  if (r.status === 401 || r.status === 403) {
    clearOwnerPassword();
    const pwd = await promptOwnerPassword('Password diminta untuk action ini. Masukkan FLOWORK_OWNER_PASSWORD:');
    if (pwd) {
      r = await doFetch(url, opts);
    }
  }
  if (!r.ok) {
    const text = await r.text().catch(() => r.statusText);
    throw new Error(`${r.status}: ${text}`);
  }
  return r.json();
}

export function validateShape(data, expectedKeys, context = "API") {
  if (!data) return data;
  const missing = expectedKeys.filter(k => !(k in data));
  if (missing.length > 0) {
    console.error(`[Shape Validator] ${context} kehilangan field: ${missing.join(', ')}`, data);
  }
  return data;
}

export function loadStyle(id, css) {
  const tagId = 'tab-style-' + id;
  if (document.getElementById(tagId)) return;
  const el = document.createElement('style');
  el.id = tagId;
  el.textContent = css;
  document.head.appendChild(el);
}

export function segmentedTab(mainEl, spec) {
  loadStyle('sgt', SEG_CSS);
  mainEl.innerHTML = `
    <div class="sgt-header">
      <h2>${esc(spec.emoji || '')} ${esc(spec.title)}</h2>
      <div class="sub">${esc(spec.desc || '')}</div>
    </div>
    <div class="sgt-bar">
      ${spec.segments.map(s => `<button class="sgt-btn" data-key="${escAttr(s.key)}"${s.tooltip ? ` data-tooltip="${escAttr(s.tooltip)}"` : ''}>${esc(s.emoji || '')} ${esc(s.label)}</button>`).join('')}
    </div>
    <div class="sgt-content" id="sgtContent"></div>
  `;
  const content = document.getElementById('sgtContent');
  const btns = mainEl.querySelectorAll('.sgt-btn');
  const knownKeys = new Set(spec.segments.map(s => s.key));
  const storeKey = spec.storeKey;

  function currentHashSub() {
    const raw = (location.hash || '').replace(/^#\/?/, '');
    const parts = raw.split('/').filter(Boolean);
    return parts[1] || '';
  }

  function persistKey(key) {
    if (storeKey) {
      try { localStorage.setItem(storeKey, key); } catch {}
    }
    // Mirror into URL hash so reload restores the same sub-tab. The top
    // portion of the hash is whatever app.js set; we only touch the suffix.
    if (window.flowworkRoute && typeof window.flowworkRoute.setSub === 'function') {
      window.flowworkRoute.setSub(key);
    }
  }

  async function loadSegment(key) {
    const seg = spec.segments.find(s => s.key === key);
    if (!seg) return;
    btns.forEach(b => b.classList.toggle('active', b.getAttribute('data-key') === key));
    content.innerHTML = `<div class="empty">${esc(t('common.loading_label').replace('{label}', seg.label))}</div>`;
    try {
      const cb = Date.now();
      const mod = await import(`/tabs/${seg.tab}.js?t=${cb}`);
      if (typeof mod.render !== 'function') throw new Error('module ' + seg.tab + ' has no render()');
      await mod.render(content);
    } catch (e) {
      const failed = t('common.load_failed_label').replace('{label}', seg.label) + ' ' + e.message;
      content.innerHTML = `<div class="err">${esc(failed)}</div>`;
    }
  }

  btns.forEach(b => b.addEventListener('click', () => {
    const key = b.getAttribute('data-key');
    persistKey(key);
    loadSegment(key);
  }));

  // Resolution order: URL hash sub > persisted last-pick > spec.defaultKey > first segment.
  let initial = null;
  const fromHash = currentHashSub();
  if (fromHash && knownKeys.has(fromHash)) initial = fromHash;
  if (!initial && storeKey) {
    try {
      const saved = localStorage.getItem(storeKey);
      if (saved && knownKeys.has(saved)) initial = saved;
    } catch {}
  }
  if (!initial) initial = knownKeys.has(spec.defaultKey) ? spec.defaultKey : spec.segments[0].key;
  // Make sure the hash reflects the segment we're about to render so a
  // mid-session reload here stays put even if it wasn't already in the URL.
  if (window.flowworkRoute && typeof window.flowworkRoute.setSub === 'function') {
    window.flowworkRoute.setSub(initial);
  }
  loadSegment(initial);
}

// Public hook for outside code (e.g. another tab) to switch this hub's
// active segment programmatically. Looks for the rendered .sgt-btn in the
// document and triggers a real click so all the existing wiring runs.
export function switchSubTab(key) {
  const btn = document.querySelector(`.sgt-btn[data-key="${key}"]`);
  if (btn) btn.click();
}

const SEG_CSS = `
.sgt-header { margin-bottom: 14px; }
.sgt-bar {
  display: flex; gap: 4px; margin-bottom: 16px; padding: 5px;
  background: rgba(15, 17, 26, 0.55); border: 1px solid var(--glass-border);
  border-radius: 12px; flex-wrap: wrap; width: fit-content; max-width: 100%;
}
.sgt-btn {
  padding: 8px 14px !important; font-size: 0.82rem !important;
  font-weight: 500 !important; border-radius: 8px !important;
  background: transparent !important; border: 1px solid transparent !important;
  color: var(--text-muted) !important; box-shadow: none !important;
  transition: background 0.15s, color 0.15s, border-color 0.15s !important;
}
.sgt-btn:hover { background: rgba(139, 92, 246, 0.08) !important; color: #cbd5e1 !important; transform: none !important; }
.sgt-btn.active {
  background: linear-gradient(135deg, rgba(139, 92, 246, 0.28), rgba(124, 58, 237, 0.12)) !important;
  color: #c4b5fd !important; border-color: rgba(139, 92, 246, 0.45) !important;
}
.sgt-content { min-height: 300px; }
.sgt-content h2 { display: none; }
.sgt-content > .sub { display: none; }
`;

// ── mdToHtml — line-by-line Markdown → HTML renderer ────────────────
// Processes text line-by-line to reliably handle tables, code blocks,
// blockquotes, headings, lists, and inline formatting.
export function mdToHtml(s) {
  if (!s) return '';
  // Normalize line endings
  const lines = s.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n');
  const out = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trimEnd();

    // ── Fenced code block ───
    if (trimmed.startsWith('```')) {
      const lang = trimmed.slice(3).trim();
      const codeLines = [];
      i++;
      while (i < lines.length && !lines[i].trimEnd().startsWith('```')) {
        codeLines.push(lines[i]);
        i++;
      }
      i++; // skip closing ```
      
      if (lang.toLowerCase() === 'mermaid') {
        // esc() the source: entities decode back to literal chars in textContent
        // (which mermaid reads), but <script>/<img onerror> become inert text →
        // no XSS from agent-authored mermaid fences. Mermaid syntax (-->) survives.
        out.push('<div class="mermaid" style="background:var(--panel-bg);border:1px solid var(--glass-border);border-radius:8px;padding:16px;margin:16px 0;display:flex;justify-content:center;">\n' + esc(codeLines.join('\n')) + '\n</div>');
        continue;
      }
      
      const lb = lang ? '<span class="md-code-lang">' + esc(lang) + '</span>' : '';
      out.push('<div class="md-codeblock">' + lb + '<pre><code>' + esc(codeLines.join('\n')) + '</code></pre></div>');
      continue;
    }

    // ── GFM Table ───
    if (trimmed.startsWith('|') && trimmed.endsWith('|') && i + 1 < lines.length) {
      const nextTrimmed = lines[i + 1].trimEnd();
      // Check if next line is separator (|---|---|)
      if (nextTrimmed.startsWith('|') && /^[|\s\-:]+$/.test(nextTrimmed)) {
        const tableRows = [trimmed];
        tableRows.push(nextTrimmed); // separator
        let j = i + 2;
        while (j < lines.length && lines[j].trimEnd().startsWith('|') && lines[j].trimEnd().endsWith('|')) {
          tableRows.push(lines[j].trimEnd());
          j++;
        }
        const pr = r => r.split('|').slice(1, -1).map(c => c.trim());
        const hd = pr(tableRows[0]);
        const dr = tableRows.slice(2).map(pr);
        let html = '<div class="md-table-wrap"><table class="md-table"><thead><tr>';
        hd.forEach(h => { html += '<th>' + inlinemd(h) + '</th>'; });
        html += '</tr></thead><tbody>';
        dr.forEach(r => { html += '<tr>'; r.forEach(c => { html += '<td>' + inlinemd(c) + '</td>'; }); html += '</tr>'; });
        html += '</tbody></table></div>';
        out.push(html);
        i = j;
        continue;
      }
    }

    // ── Blockquote ───
    if (trimmed.startsWith('> ')) {
      const bqLines = [];
      while (i < lines.length && lines[i].trimEnd().startsWith('> ')) {
        bqLines.push(lines[i].trimEnd().slice(2));
        i++;
      }
      out.push('<blockquote class="md-bq">' + bqLines.map(l => inlinemd(l)).join('<br>') + '</blockquote>');
      continue;
    }

    // ── Heading ───
    if (trimmed.startsWith('#### ')) { out.push('<h4>' + inlinemd(trimmed.slice(5)) + '</h4>'); i++; continue; }
    if (trimmed.startsWith('### ')) { out.push('<h3>' + inlinemd(trimmed.slice(4)) + '</h3>'); i++; continue; }
    if (trimmed.startsWith('## ')) { out.push('<h2>' + inlinemd(trimmed.slice(3)) + '</h2>'); i++; continue; }
    if (trimmed.startsWith('# ')) { out.push('<h1>' + inlinemd(trimmed.slice(2)) + '</h1>'); i++; continue; }

    // ── Horizontal rule ───
    if (/^-{3,}$/.test(trimmed)) { out.push('<hr>'); i++; continue; }

    // ── Unordered list ───
    if (/^[-*] /.test(trimmed)) {
      const items = [];
      while (i < lines.length && /^[-*] /.test(lines[i].trimEnd())) {
        items.push(lines[i].trimEnd().replace(/^[-*] /, ''));
        i++;
      }
      out.push('<ul>' + items.map(it => '<li>' + inlinemd(it) + '</li>').join('') + '</ul>');
      continue;
    }

    // ── Ordered list ───
    if (/^\d+\. /.test(trimmed)) {
      const items = [];
      while (i < lines.length && /^\d+\. /.test(lines[i].trimEnd())) {
        items.push(lines[i].trimEnd().replace(/^\d+\. /, ''));
        i++;
      }
      out.push('<ol>' + items.map(it => '<li>' + inlinemd(it) + '</li>').join('') + '</ol>');
      continue;
    }

    // ── Empty line → paragraph break ───
    if (trimmed === '') { out.push(''); i++; continue; }

    // ── Regular paragraph (collect contiguous non-empty lines) ───
    const pLines = [];
    while (i < lines.length && lines[i].trimEnd() !== '' &&
           !lines[i].trimEnd().startsWith('#') &&
           !lines[i].trimEnd().startsWith('```') &&
           !lines[i].trimEnd().startsWith('> ') &&
           !lines[i].trimEnd().startsWith('---') &&
           !(lines[i].trimEnd().startsWith('|') && lines[i].trimEnd().endsWith('|')) &&
           !/^[-*] /.test(lines[i].trimEnd()) &&
           !/^\d+\. /.test(lines[i].trimEnd())) {
      pLines.push(lines[i].trimEnd());
      i++;
    }
    if (pLines.length > 0) {
      out.push('<p>' + pLines.map(l => inlinemd(l)).join('<br>') + '</p>');
    }
  }

  return out.filter(x => x !== '').join('\n');
}

// hunting_bug 2026-04-30 BUG-024 fix: sanitize URL protocol untuk prevent
// XSS via javascript:/data:/vbscript: in markdown link/image. Sebelumnya
// agent-generated markdown (bug reports, brain content, dll) bisa inject
// `[click](javascript:alert(document.cookie))` → stored XSS di dashboard.
function sanitizeURL(url) {
  if (typeof url !== 'string') return '#blocked';
  // Decode + lowercase + trim untuk catch obfuscated protocol (e.g. "JaVaScRiPt:")
  let decoded;
  try { decoded = decodeURIComponent(url); } catch (_) { decoded = url; }
  const lower = decoded.trim().toLowerCase();
  // Block dangerous protocols (case-insensitive, allow whitespace prefix)
  if (/^\s*(javascript|data|vbscript|file)\s*:/.test(lower)) {
    return '#blocked-unsafe-url';
  }
  return url;
}

// Inline markdown: images, links, bold, italic, inline code
function inlinemd(s) {
  let h = esc(s);
  // esc() above already neutralized <>& for the whole string, but it does NOT
  // escape quotes. url/alt get injected into double-quoted attributes (src/href/
  // alt), so a `"` in agent/brain markdown would break out → onerror=/onfocus=
  // XSS. Escape the quote in attribute-bound captures (don't re-escape & → dobel).
  const aq = (x) => String(x).replace(/"/g, '&quot;');
  h = h.replace(/!\[(.*?)\]\((.*?)\)/g, (_, alt, url) => {
    return '<img src="' + aq(sanitizeURL(url)) + '" alt="' + aq(alt) + '" class="md-img" />';
  });
  h = h.replace(/\[(.*?)\]\((.*?)\)/g, (_, text, url) => {
    return '<a href="' + aq(sanitizeURL(url)) + '" target="_blank" rel="noopener">' + text + '</a>';
  });
  h = h.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  h = h.replace(/\*(.+?)\*/g, '<em>$1</em>');
  h = h.replace(/`([^`]+)`/g, '<code class="md-inline">$1</code>');
  return h;
}

// ── Modal helper ──────────────────────────────────────────────────────────
// One source of truth for the centred-card-over-backdrop pattern. Callers
// pass title + body element + footer buttons; this wires ESC + backdrop
// click + a managed close() handle.
//
//   const m = openModal({
//     title: 'Edit persona',
//     body: textareaEl,
//     buttons: [
//       { label: 'Cancel', onClick: () => m.close(), variant: 'secondary' },
//       { label: 'Save',   onClick: async () => { ... m.close(); } },
//     ],
//   });

loadStyle('flowork-modal', `
.fm-backdrop {
  position: fixed; inset: 0; z-index: 9000;
  background: rgba(2, 6, 23, 0.72);
  backdrop-filter: blur(4px);
  display: flex; align-items: center; justify-content: center;
  animation: fmFade 0.14s ease-out;
}
@keyframes fmFade { from { opacity: 0; } to { opacity: 1; } }
.fm-card {
  background: linear-gradient(155deg, rgba(30,17,52,0.95), rgba(15,17,26,0.95));
  border: 1px solid rgba(168, 85, 247, 0.32);
  border-radius: 14px;
  box-shadow: 0 20px 60px rgba(0,0,0,0.55), 0 0 0 1px rgba(255,255,255,0.05) inset;
  width: min(640px, calc(100vw - 32px));
  max-height: calc(100vh - 64px);
  display: flex; flex-direction: column;
}
.fm-head {
  display: flex; align-items: center; gap: 10px;
  padding: 14px 18px; border-bottom: 1px solid rgba(255,255,255,0.08);
}
.fm-title { font-weight: 700; color: #f8fafc; font-size: 1rem; flex: 1; }
.fm-x {
  background: transparent; border: none; color: #94a3b8; font-size: 1.4rem;
  cursor: pointer; padding: 0 6px; border-radius: 6px;
}
.fm-x:hover { background: rgba(255,255,255,0.08); color: #f8fafc; }
.fm-body { padding: 16px 18px; overflow-y: auto; color: #e2e8f0; font-size: 0.88rem; }
.fm-foot {
  display: flex; gap: 8px; justify-content: flex-end;
  padding: 12px 18px; border-top: 1px solid rgba(255,255,255,0.08);
}
.fm-btn {
  padding: 8px 16px; border-radius: 8px; font-size: 0.85rem; font-weight: 600;
  cursor: pointer; border: 1px solid transparent;
}
.fm-btn.primary {
  background: linear-gradient(135deg, #a78bfa, #7c3aed); color: #fff;
}
.fm-btn.primary:hover { filter: brightness(1.08); }
.fm-btn.secondary {
  background: rgba(255,255,255,0.06); color: #cbd5e1;
  border-color: rgba(255,255,255,0.12);
}
.fm-btn.secondary:hover { background: rgba(255,255,255,0.12); }
.fm-btn:disabled { opacity: 0.5; cursor: wait; }
`);

export function openModal({ title = '', body = null, buttons = [], onClose } = {}) {
  const backdrop = document.createElement('div');
  backdrop.className = 'fm-backdrop';

  const card = document.createElement('div');
  card.className = 'fm-card';
  card.setAttribute('role', 'dialog');
  card.setAttribute('aria-modal', 'true');

  const head = document.createElement('div');
  head.className = 'fm-head';
  const titleEl = document.createElement('div');
  titleEl.className = 'fm-title';
  titleEl.textContent = title;
  const xBtn = document.createElement('button');
  xBtn.className = 'fm-x';
  xBtn.setAttribute('aria-label', 'Close');
  xBtn.textContent = '×';
  head.append(titleEl, xBtn);

  const bodyEl = document.createElement('div');
  bodyEl.className = 'fm-body';
  if (body instanceof Node) bodyEl.appendChild(body);
  else if (typeof body === 'string') bodyEl.innerHTML = body;

  const foot = document.createElement('div');
  foot.className = 'fm-foot';
  const btnEls = [];
  for (const b of buttons) {
    const el = document.createElement('button');
    el.className = 'fm-btn ' + (b.variant === 'secondary' ? 'secondary' : 'primary');
    el.textContent = b.label;
    el.onclick = async () => {
      if (typeof b.onClick === 'function') await b.onClick({ button: el, modal: handle });
    };
    foot.appendChild(el);
    btnEls.push(el);
  }

  card.append(head, bodyEl, foot);
  backdrop.appendChild(card);
  document.body.appendChild(backdrop);

  let closed = false;
  function close() {
    if (closed) return;
    closed = true;
    document.removeEventListener('keydown', onKey, true);
    backdrop.remove();
    if (typeof onClose === 'function') onClose();
  }
  function onKey(e) {
    if (e.key === 'Escape') { e.preventDefault(); close(); }
  }
  document.addEventListener('keydown', onKey, true);
  backdrop.addEventListener('mousedown', (e) => {
    if (e.target === backdrop) close();
  });
  xBtn.onclick = close;

  const handle = {
    close,
    setBusy(busy) { btnEls.forEach((el) => (el.disabled = busy)); },
    bodyEl,
    cardEl: card,
  };
  return handle;
}
