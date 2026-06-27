// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// tabs/coder.js — ORCHESTRATOR tipis tab "AI Studio" (Pabrik Kemampuan). Layout: chat FULL
// (utama, fixed-height, ga di-bawah) + DRAWER "Siklus Hidup" 3D yg slide dari kanan via toggle.
// Nano-modular: chat = js/chatui.js · isi drawer = tabs/studio_lifecycle.js · UI 3D = js/ui/glass.js.
// Semua copy lewat kamus i18n (t('coder.*')) — nol hardcode.
import { esc } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { renderChatUI } from '/js/chatui.js';
import { renderLifecycle } from './studio_lifecycle.js';
import { ensureGlass } from '/js/glass.js';

const T = (k) => t('coder.' + k);

export async function render(mainEl) {
  ensureGlass();
  mainEl.innerHTML = `
    <div class="studio-wrap">
      <header class="studio-head">
        <span class="studio-glyph">🏭</span>
        <div class="studio-titles">
          <h1 class="studio-title">${esc(T('title'))}</h1>
          <div class="studio-note">${esc(T('factory_note'))}</div>
        </div>
        <button id="lcToggle" class="studio-lc-btn">🔄 ${esc(T('lc_title'))}</button>
      </header>
      <div id="aiStudioChat" class="studio-chat"></div>
    </div>

    <div id="lcBackdrop" class="gl-backdrop"></div>
    <aside id="lcDrawer" class="gl-drawer" aria-hidden="true">
      <div class="gl-drawer-head">
        <span style="font-size:1.1rem">🔄</span>
        <h3>${esc(T('lc_title'))}</h3>
        <button id="lcRefresh" class="gl-x" title="${esc(T('lc_refresh'))}">⟳</button>
        <button id="lcClose" class="gl-x" title="${esc(T('lc_close'))}">✕</button>
      </div>
      <div id="lcBody" class="gl-drawer-body"></div>
    </aside>

    <style>
      .studio-wrap { display:flex; flex-direction:column; height:calc(100vh - 92px); min-height:520px; padding:18px 26px 16px; }
      .studio-head { display:flex; align-items:center; gap:14px; margin-bottom:14px; }
      .studio-glyph { font-size:1.8rem; filter:drop-shadow(0 0 12px var(--accent-glow)); }
      .studio-titles { min-width:0; flex:1; }
      .studio-title { margin:0; font-size:1.5rem; font-weight:800; line-height:1.05;
        background:linear-gradient(90deg,#c4b5fd,#67e8f9 58%,#6ee7b7); -webkit-background-clip:text; background-clip:text; color:transparent; }
      .studio-note { font-size:.8rem; color:var(--text-muted); margin-top:3px; line-height:1.45; max-width:90ch; }
      .studio-lc-btn { flex-shrink:0; font:inherit; font-size:.82rem; font-weight:600; padding:9px 15px; border-radius:11px; cursor:pointer;
        color:var(--text-main); border:1px solid var(--glass-border-hover);
        background:linear-gradient(165deg, color-mix(in srgb,var(--accent) 22%, transparent), color-mix(in srgb,var(--accent) 6%, transparent));
        box-shadow:0 4px 14px rgba(0,0,0,.25), inset 0 1px 0 rgba(255,255,255,.08); transition:transform .12s, filter .15s; }
      .studio-lc-btn:hover { filter:brightness(1.12); transform:translateY(-1px); }
      .studio-chat { flex:1; min-height:0; }
      /* chat full-height di dalam studio: override tinggi default chatui biar isi shell */
      .studio-chat .cu-wrap { height:100%; }
    </style>`;

  renderChatUI(mainEl.querySelector('#aiStudioChat'));

  // ── Drawer Siklus Hidup (lazy: render isi pas pertama dibuka) ────────────────
  const drawer = mainEl.querySelector('#lcDrawer');
  const backdrop = mainEl.querySelector('#lcBackdrop');
  const bodyEl = mainEl.querySelector('#lcBody');
  let loaded = false;
  const open = () => {
    drawer.classList.add('on'); backdrop.classList.add('on'); drawer.setAttribute('aria-hidden', 'false');
    if (!loaded) { loaded = true; renderLifecycle(bodyEl); }
  };
  const close = () => { drawer.classList.remove('on'); backdrop.classList.remove('on'); drawer.setAttribute('aria-hidden', 'true'); };
  mainEl.querySelector('#lcToggle').onclick = open;
  mainEl.querySelector('#lcClose').onclick = close;
  backdrop.onclick = close;
  mainEl.querySelector('#lcRefresh').onclick = () => renderLifecycle(bodyEl);
  document.addEventListener('keydown', (e) => { if (e.key === 'Escape') close(); }, { once: false });
}
