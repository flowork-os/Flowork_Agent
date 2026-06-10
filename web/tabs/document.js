// document.js — Document tab: the in-app handbook for Flowork.
// Tone: plain, human, like explaining it to a friend — not a spec sheet. Facts are grounded in
// the repo README + product lineage (nothing invented), brand-neutral, English (global audience).
// Tab chrome strings live in the existing `menu` i18n domain (menu.tab.document.*); the long-form
// sections are authored content kept here as data. Layout: left section nav + a content pane.
import { esc } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('menu.tab.document.' + String(k)) });

// Each section's `body` is trusted, author-written HTML (never user input).
const SECTIONS = [
  {
    id: 'history',
    title: '📜 History',
    body: `
      <h3>Flowork didn't get built once. It got rebuilt 12 times.</h3>
      <p>Twelve times in about a year and a half. That's not someone who can't make up their mind —
      it's someone chasing one idea until it finally fits.</p>
      <p>It started as <strong>floworkos.com</strong>: a website where you dragged boxes around to
      automate things, written in <strong>Python</strong>, a bit like n8n. The catch? <em>You</em> did
      all the wiring. You were the one stitching everything together by hand.</p>
      <p>So the whole thing flipped. Instead of you running the machine, the <strong>AI agents run
      themselves</strong> — and you just own them. Moving from Python to Go wasn't for fun: you simply
      can't ship one no-dependencies binary, or safely box off each agent, in Python.</p>
      <p>Through all 12 rebuilds, four things never changed:</p>
      <ul>
        <li>It's always an <strong>operating system</strong>, never just a "tool".</li>
        <li>Your <strong>data is always yours</strong> — offline, no cloud, no tracking.</li>
        <li>Everything <strong>plugs in and out cleanly</strong>.</li>
        <li><strong>Privacy comes first.</strong></li>
      </ul>`,
  },
  {
    id: 'install',
    title: '⬇️ Install',
    body: `
      <h3>No Docker. No accounts. No cloud. Just one command.</h3>
      <pre><code>git clone https://github.com/flowork-os/Flowork_Agent.git
cd Flowork_Agent
./start.sh</code></pre>
      <p>That builds Flowork and opens the control panel at <code>http://127.0.0.1:1987</code>. You'll
      need <strong>Go 1.25+</strong> installed. The first time you open it, make your
      <strong>owner account</strong> on the login screen — that's you, the boss.</p>
      <ul>
        <li>Works on <strong>Linux, macOS, and Windows</strong>.</li>
        <li>Stop it with <code>./stop.sh</code>, restart with <code>./restart.sh</code>.</li>
        <li>Everything runs on your machine and talks to nothing outside unless you tell it to.</li>
      </ul>`,
  },
  {
    id: 'why',
    title: '⭐ Why Flowork',
    body: `
      <h3>Most AI forgets you the second you close the tab. Flowork doesn't.</h3>
      <p>You pay, you chat, and when the session ends, everything you taught it is gone. That's renting.</p>
      <p>A Flowork agent is something you <strong>own</strong>. It lives in a folder on your computer.
      It remembers. It follows its own rules. It learns from its mistakes. And it keeps working even
      when the internet's down. Copy that folder to a USB stick and the whole "brain" comes with it.</p>
      <p>Why people pick it:</p>
      <ul>
        <li><strong>It's yours.</strong> Runs on your machine, your data never leaves. No cloud, no tracking, no lock-in.</li>
        <li><strong>It snaps together.</strong> Drop in a <code>.fwpack</code> and it just works — nothing to rebuild.</li>
        <li><strong>It gets smarter.</strong> Agents learn from what they got wrong, and treat mistakes as lessons, not something to hide.</li>
        <li><strong>It watches its own back.</strong> A real security scanner checks the code your agents run.</li>
      </ul>
      <p>There are similar tools out there (one's built on Node, another on Python). What makes Flowork
      different: <strong>one small Go program that runs anywhere</strong>, every agent
      <strong>boxed off safely on its own</strong>, and a <strong>built-in security scanner</strong> the
      others don't have.</p>`,
  },
  {
    id: 'menus',
    title: '🧭 Menus',
    body: `
      <h3>What each thing in the left menu is for</h3>
      <ul>
        <li>🛡️ <strong>Threat Radar</strong> — your security scanner. Point it at your code or a target and see what's risky, sorted by how bad it is.</li>
        <li>🤖 <strong>AI Agent</strong> — install and manage your agents. Every agent card has a <em>Setting</em> button for its brain, prompt, tools, and schedule.</li>
        <li>👥 <strong>Group</strong> — a team of small specialist agents that work on something together (think a colony of ants, each doing one small job well).</li>
        <li>🔌 <strong>Connections</strong> — one place for all the ways things come in and out: Telegram, Discord, Slack, WhatsApp, CLI, schedules, MCP.</li>
        <li>⏰ <strong>Schedule</strong> — make agents run on a timer.</li>
        <li>⚡ <strong>Trigger</strong> — make something happen on an event or webhook: something fires → an agent runs → you get the result.</li>
        <li>▦ <strong>App</strong> — install and run little self-contained apps. Each one is both a screen you click and a tool your agents can use.</li>
        <li>🧬 <strong>AI Studio</strong> — the workshop for building and editing agents and modules.</li>
        <li>📋 <strong>Audit Log</strong> — a history of what changed in the system.</li>
        <li>📖 <strong>Document</strong> — this handbook.</li>
        <li>⚙️ <strong>Settings</strong> — your owner-level stuff: API keys, notifications (your own Telegram), and more.</li>
      </ul>`,
  },
  {
    id: 'tech',
    title: '🔧 Technology',
    body: `
      <h3>For the curious, here's what's under the hood</h3>
      <ul>
        <li><strong>Go 1.25</strong> — the whole thing is one small program. No Docker, no extra runtime. Linux, macOS, Windows.</li>
        <li><strong>A tiny "forever" core (microkernel)</strong> — written once and never touched again. Everything else clips onto one fixed contract. If a plugin breaks, you fix that one folder and nothing else cares.</li>
        <li><strong>Each agent in its own box (WASM)</strong> — agents run as sandboxed WebAssembly via <em>wazero</em>, and can only do what you've allowed them to.</li>
        <li><strong>Memory in SQLite</strong> — fast full-text search, and every agent gets its own private brain file.</li>
        <li><strong>MCP, both ways</strong> — use outside MCP tools, and let outside apps use your agents.</li>
        <li><strong>It guards itself</strong> — the core watches for tampering and drops into safe-mode if anything's off.</li>
        <li><strong>The web screen is baked in</strong> — no separate website to host.</li>
      </ul>`,
  },
  {
    id: 'features',
    title: '✨ Features',
    body: `
      <h3>The short version of what you get</h3>
      <ul>
        <li><strong>Agents you own</strong> — each in its own folder with its own personality, rules, tools, schedule, and memory.</li>
        <li><strong>A memory that learns</strong> — agents remember what went wrong and turn it into a lesson, not something to hide.</li>
        <li><strong>117 built-in tools and 9 quick commands.</strong></li>
        <li><strong>Everything plugs in</strong> — tools, commands, scanners, channels, MCP servers, apps — all drop-in <code>.fwpack</code> files.</li>
        <li><strong>A security scanner built in</strong> — a 16,000+ check arsenal (Threat Radar).</li>
        <li><strong>Talk to it anywhere</strong> — Telegram, Discord, Slack, WhatsApp, CLI.</li>
        <li><strong>Your own voice</strong> — offline speech-to-text and free text-to-speech.</li>
        <li><strong>Teamwork</strong> — many small agents working together on bigger jobs.</li>
        <li><strong>It pings your phone</strong> — alerts straight to your own Telegram; the token is yours, set in Settings, never baked into the code.</li>
        <li><strong>Works fully offline.</strong></li>
      </ul>`,
  },
];

export async function render(mainEl) {
  mainEl.innerHTML = `
    <h2>${esc(L.title)}</h2>
    <div class="sub">${esc(L.desc)}</div>
    <div class="doc-layout" style="display:flex;gap:16px;align-items:flex-start;flex-wrap:wrap">
      <nav class="doc-nav" style="display:flex;flex-direction:column;gap:6px;min-width:170px">
        ${SECTIONS.map((s, i) => `<button class="doc-link${i === 0 ? ' active' : ''}" data-doc="${esc(s.id)}"
          style="text-align:left;padding:8px 12px;border-radius:8px;cursor:pointer">${esc(s.title)}</button>`).join('')}
      </nav>
      <div class="card doc-body" id="docBody" style="flex:1;min-width:280px;line-height:1.6;padding:18px"></div>
    </div>`;

  const body = mainEl.querySelector('#docBody');
  const show = (id) => {
    const s = SECTIONS.find((x) => x.id === id) || SECTIONS[0];
    body.innerHTML = s.body;
    mainEl.querySelectorAll('.doc-link').forEach((b) => b.classList.toggle('active', b.dataset.doc === s.id));
  };
  mainEl.querySelectorAll('.doc-link').forEach((b) => { b.onclick = () => show(b.dataset.doc); });
  show(SECTIONS[0].id);
}
