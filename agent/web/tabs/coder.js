// coder.js — tab "AI Studio": the UNIFIED creation hub. ONE chat to build a TEAM, a
// single APP/agent, or SCHEDULE one to run automatically — like chatting with Claude to
// build. Replaces the old generate-form + approval-queue UI: the conversational
// Architect brain (server-side, /api/chat/send) designs, builds, and schedules. The
// reusable chat component is web/js/chatui.js. The /api/coder/* endpoints still exist
// (used in-process by the build_app tool); the kernel core stays untouched.
import { esc } from '../js/utils.js';
import { t } from '/js/i18n.js';
import { renderChatUI } from '/js/chatui.js';
import { renderLifecycle } from './studio_lifecycle.js';

const T = (k) => t('coder.' + k);

export async function render(mainEl) {
  mainEl.innerHTML = `
    <section style="padding:20px 26px 4px;color:#e2e8f0">
      <div style="display:flex;align-items:center;gap:13px">
        <span style="font-size:1.7rem;filter:drop-shadow(0 0 10px rgba(167,139,250,.5))">🏭</span>
        <div>
          <h1 style="margin:0;font-size:1.55rem;line-height:1.1;font-weight:700;background:linear-gradient(90deg,#c4b5fd,#67e8f9 58%,#6ee7b7);-webkit-background-clip:text;background-clip:text;color:transparent">${esc(T('title'))}</h1>
          <div style="font-size:0.86rem;color:#94a3b8;margin-top:3px;max-width:80ch;line-height:1.5">${esc(T('sub'))}</div>
          <div style="font-size:0.78rem;color:#67e8f9;margin-top:5px;max-width:80ch;line-height:1.45">${esc(T('factory_note'))}</div>
        </div>
      </div>
    </section>
    <div id="studioLifecycle" style="padding:6px 26px 0"></div>
    <div id="aiStudioChat" style="padding:14px 26px 26px"></div>`;
  renderChatUI(mainEl.querySelector('#aiStudioChat'));
  renderLifecycle(mainEl.querySelector('#studioLifecycle')); // F3 panel Siklus Hidup (async, ga blok chat)
}
