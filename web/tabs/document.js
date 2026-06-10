// document.js — Document tab: a slim in-app index that points to the full handbook.
// The real docs live as per-menu Markdown files in doc/handbook/ (readable right after `git clone`,
// before the app is even running). This tab is just a launcher, so the .md files stay the single
// source of truth and can't drift.
import { esc } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('menu.tab.document.' + String(k)) });
const HB = 'https://github.com/flowork-os/Flowork_Agent/blob/main/doc/handbook';
const DOC = 'https://github.com/flowork-os/doc/blob/main';

const GROUPS = [
  ['Start here', [
    ['📖 Getting Started — what / why / install', HB + '/getting-started.md'],
    ['🏗️ Architecture & Technology', HB + '/architecture.md'],
    ['🧠 The Mind — Brain, Educational Errors, Router', HB + '/the-mind.md'],
  ]],
  ['Menus', [
    ['🛡️ Threat Radar', HB + '/menu-threat-radar.md'],
    ['🤖 AI Agent', HB + '/menu-ai-agent.md'],
    ['👥 Group', HB + '/menu-group.md'],
    ['🔌 Connections', HB + '/menu-connections.md'],
    ['⏰ Schedule', HB + '/menu-schedule.md'],
    ['⚡ Trigger', HB + '/menu-trigger.md'],
    ['▦ App', HB + '/menu-app.md'],
    ['🧬 AI Studio', HB + '/menu-ai-studio.md'],
    ['📋 Audit Log', HB + '/menu-audit-log.md'],
    ['⚙️ Settings', HB + '/menu-settings.md'],
  ]],
  ['Dated design blueprints', [
    ['Educational Errors', DOC + '/EDUCATIONAL_ERRORS.md'],
    ['Anti-Hallucination Antibody', DOC + '/ANTI_HALLUCINATION_ANTIBODY.md'],
    ['One State, Two Drivers', DOC + '/ONE_STATE_TWO_DRIVERS.md'],
  ]],
];

export async function render(mainEl) {
  const groups = GROUPS.map(([h, items]) => `
    <div class="card" style="padding:16px;margin-bottom:14px">
      <div class="ch">${esc(h)}</div>
      <ul style="margin:8px 0 0;padding-left:18px;line-height:1.95">
        ${items.map(([label, url]) => `<li><a href="${esc(url)}" target="_blank" rel="noopener">${esc(label)}</a></li>`).join('')}
      </ul>
    </div>`).join('');
  mainEl.innerHTML = `
    <h2>${esc(L.title)}</h2>
    <div class="sub">${esc(L.desc)}</div>
    <div class="card" style="padding:16px;margin-bottom:14px;line-height:1.6">
      <p style="margin:0 0 6px">The full handbook lives as Markdown you can read any time — even before
      the app is running — under <code>doc/handbook/</code>. Stuck on a menu? Open its page below.</p>
      <p style="margin:0;opacity:.82">Quickstart: <code>git clone … &amp;&amp; cd Flowork_Agent &amp;&amp; ./start.sh</code>
      → open <code>http://127.0.0.1:1987</code> → create your owner account.</p>
    </div>
    ${groups}`;
}
