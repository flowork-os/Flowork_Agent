// document.js — Document tab: the in-app handbook for Flowork.
// Single scrolling page: a sticky table of contents on the left jumps to each section; all
// sections live on one page (adding more is just another entry in SECTIONS). Tone is plain and
// human; facts are grounded in the repo README + the real folder layout (nothing invented),
// brand-neutral, English (global). Tab chrome strings come from the `menu` i18n domain.
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
    id: 'brain',
    title: '🧠 The Brain',
    body: `
      <h3>Every agent carries its own mind</h3>
      <p>Each agent's memory lives in its own <code>state.db</code> — clone the folder and the memory
      comes with it. Nothing is shared with other agents unless you wire it.</p>
      <h4>How the brain is built</h4>
      <ul>
        <li><strong>Local memory (FTS5)</strong> — a fast keyword memory (SQLite FTS5 / BM25). No
        embeddings, so it's lightweight, instant, and fully offline. The agent stores what it sees with
        <code>brain_add</code> and recalls related memories with <code>brain_search</code>.</li>
        <li><strong>Wings</strong> — every memory is filed in a "wing": <em>general, experience, eureka,</em>
        or <em>constitution</em> (think labelled drawers). Duplicates are dropped by content hash.</li>
        <li><strong>Two tiers</strong> — on top of each agent's own little memory sits a big shared corpus
        (millions of entries, offline and fork-able) it can draw on.</li>
        <li><strong>Dream → Eureka</strong> — while the agent is idle, a quiet pass looks over its history
        and consolidates recurring patterns into <code>eureka</code> insights. The brain grows richer from
        its own experience — no retraining.</li>
        <li><strong>Immune system</strong> — an antibody scanner quarantines poisoned memories
        (prompt-injection, jailbreak, low-confidence junk), so the memory never gets corrupted.</li>
      </ul>
      <h4>How a turn works</h4>
      <p>A message comes in → the agent <strong>remembers</strong> it → <strong>recalls</strong> related
      memories → <strong>thinks</strong> (calls the LLM with its doctrine + the recalled context) →
      <strong>replies</strong>. Memory first, then thought.</p>
      <p><strong>Principle:</strong> a brain that never forgets, and grows from its own past.</p>`,
  },
  {
    id: 'educational-errors',
    title: '📚 Educational Errors',
    body: `
      <h3>The idea Flowork plants a flag on</h3>
      <p>This is an <strong>original design principle</strong> — written down, dated, and public in the
      repo (<code>EDUCATIONAL_ERRORS.md</code>), originated by the project's owner. Not a boast; a marker:
      here's the principle, here's the date we committed to it.</p>
      <p>Almost every AI treats a mistake as something to <em>hide</em> — suppress it, fine-tune it away,
      pretend it didn't happen. That throws away the richest learning signal there is. A child burned once
      understands fire for life; an AI that erases its mistakes erases the very thing it could grow from.</p>
      <p><strong>Flowork treats an error as EDUCATION.</strong> Every mistake is:</p>
      <ul>
        <li><strong>Captured</strong> — recorded as a real thing, not a buried log line.</li>
        <li><strong>Explained</strong> — <em>why</em> it was wrong (an error without a lesson is just noise).</li>
        <li><strong>Retained</strong> — kept as something the brain can recall, so the same wall isn't hit twice.</li>
        <li><strong>Redemptive</strong> — the agent that erred is <em>quarantined, not deleted</em>, and given
        a chance to correct. Punishment removes; education keeps and improves. A second chance, not a death
        sentence.</li>
      </ul>
      <p>The loop in one line: <strong>make a mistake → capture and explain it → keep it as a lesson →
      recall it next time → don't hit the same wall.</strong> The learning happens at runtime — no
      retraining the whole model.</p>
      <p>The pieces are real, not aspiration: a per-agent mistake store, mistake-recall before acting, a
      reversible immune quarantine, and a Reaper that only prunes on sustained failure (one error earns
      correction, not deletion).</p>
      <p><strong>Principle:</strong> errors as growth, not as shame.</p>`,
  },
  {
    id: 'router',
    title: '🔀 The Router — and why to use ours',
    body: `
      <h3>Point your agents at Flowork's own router</h3>
      <p>You can aim an agent at any LLM, but we recommend pointing it at Flowork's router
      (<code>http://127.0.0.1:2402/v1/chat/completions</code>). Here's why it's worth it:</p>
      <ul>
        <li><strong>One door, model-agnostic</strong> — every agent calls one local router; swap the model
        in one place, no API-key sprawl. It can route to a subscription model or a fully local one, picking
        by priority.</li>
        <li><strong>The anti-hallucination antibody</strong> — the real reason. Before the model answers,
        the router pulls the agent's past mistakes, ranks them by <em>karma</em> (how often that mistake has
        recurred) <strong>×</strong> <em>relevance</em> (how close it is to right now), and injects the top
        few as warnings into the prompt. So a hallucination gets <em>harder to repeat over time, by
        design</em> — deterministically, with no retraining and no GPU. You can't trust a weak model to
        remember its own mistakes on its own, so the router forces the correction at the gateway.
        Deterministic is strong; relying on the model's goodwill is fragile.</li>
        <li><strong>The constitution, every turn</strong> — the router + brain inject the agent's sacred
        doctrine on every single turn (a 5W1H gate, an identity guard, a truth rule), budget-capped so it
        never bloats. Anti-hallucination isn't a setting here — it's law.</li>
      </ul>
      <p>This is exactly where <strong>Educational Errors</strong> becomes real: the mistakes the brain
      keeps are what the router injects back as antibodies. Aim an agent at a raw third-party API and it
      gets none of this — no antibody, no constitution, no learning loop. Aim it at our router and it gets
      all three for free.</p>
      <p><strong>Principle:</strong> make correction deterministic, at the gateway — don't hope the model
      behaves.</p>`,
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
    id: 'threat-radar',
    title: '🛡️ Threat Radar (in depth)',
    body: `
      <h3>Flowork's built-in security scanner</h3>
      <p>Threat Radar is the one thing no other agent framework ships: a live dashboard that watches
      the code your agents run, and lets you scan your own code or an authorized external target.</p>

      <h4>The screen</h4>
      <p>On the left, a radar sweep with three numbers under it — <strong>runs</strong>,
      <strong>findings</strong>, and <strong>critical</strong> (red if anything critical is live, green
      if you're clean). The critical number is the worst result from the <em>latest</em> scan of each
      target, so it actually goes back down when you fix something. On the right are two panels:
      <strong>Scan Log</strong> (every scan, newest first — status, whether it was manual or automatic,
      the target, and how many hits) and <strong>Findings</strong> (click any run in the log to see
      exactly what it found). It refreshes itself every few seconds.</p>

      <h4>The buttons (top-right)</h4>
      <ul>
        <li><strong>⟳ Refresh</strong> — reload the scan list now.</li>
        <li><strong>⊕ Scan Target</strong> — open the scan form. Pick a <em>Tool</em>, a <em>Target</em>,
        optional <em>Args</em>, and a <em>Category</em> (<code>immune</code> = hardening your own code,
        <code>pentest</code> = an authorized external target), then Run. The tool list and the target
        list both come from an <strong>owner-editable allowlist</strong> — Flowork won't run a tool or
        touch a target that isn't on it, and there's no shell in the middle. Nothing runs that you
        didn't allow.</li>
        <li><strong>≣ Arsenal</strong> — the catalog of everything the scanner can use: defensive code
        auditors (the core — marked <code>CORE</code>, can't be removed), tools, and thousands of
        detection checks. Search it, and flip any pack on/off with <em>Install / Uninstall</em>. The top
        shows how many checks are installed right now.</li>
      </ul>

      <h4>For developers — make your own scanner</h4>
      <p>A scanner "check" is just a <strong>nuclei template</strong> — a small YAML file that says
      "look for this". Here's the smallest shape:</p>
      <pre><code>id: exposed-env-file
info:
  name: Exposed .env file
  author: you
  severity: high
http:
  - method: GET
    path:
      - "{{BaseURL}}/.env"
    matchers:
      - type: word
        words:
          - "DB_PASSWORD"</code></pre>
      <p>Two ways to add it:</p>
      <ol>
        <li><strong>One check at a time</strong> — POST it to <code>/api/scanner/checks/add</code> with
        <code>{ name, yaml }</code>. It runs through <code>nuclei -validate</code>; bad syntax is
        rejected, a good one lands in <code>&lt;nuclei-templates&gt;/flowork-private/</code> and shows
        up in the Arsenal right away.</li>
        <li><strong>Ship a pack</strong> (plug-and-play, like a tool) — bundle many checks into a
        <code>kind:scanner</code> <code>.fwpack</code> (a zip):
        <pre><code>my-scanner.fwpack  (zip)
├─ plugin.json   { "id": "my-scanner", "kind": "scanner",
│                  "scanner": { "name": "My Scanner", "description": "…" } }
└─ checks/
   ├─ check-1.yaml
   └─ check-2.yaml</code></pre>
        Install it with <code>/api/scanner/packs/install</code>. Flowork validates every check, drops
        any that fail, and the rest snap into the Arsenal — install/uninstall like any other module
        (<code>/api/scanner/packs/uninstall</code>, list <code>/api/scanner/packs/installed</code>).</li>
      </ol>
      <p><strong>Safety:</strong> everything is owner-only and local; every check is validated before it
      lands; templates run inert (no code execution); and scans only ever touch the tools and targets on
      your allowlist.</p>`,
  },
  {
    id: 'ai-agent',
    title: '🤖 AI Agent (in depth)',
    body: `
      <h3>Where your agents live</h3>
      <p>Each agent is its own little citizen — its own folder, its own memory, its own personality and
      rules, and its own list of what it's allowed to do. They share nothing unless you wire them to.
      Disable or delete one and nothing else even notices.</p>

      <h4>Install an agent</h4>
      <p>At the top is a drop zone: drag in a <code>.fwagent.zip</code> (or click to pick one). It must
      contain a <code>manifest.json</code> and an <code>agent.wasm</code>, max 64 MiB. Once dropped, it
      extracts to <code>~/.flowork/agents/&lt;id&gt;.fwagent/</code> and the kernel hot-loads it — no
      restart. There's also a <strong>↻ Refresh</strong> button.</p>

      <h4>The agent card &amp; its buttons</h4>
      <p>Every installed agent is a card showing its <strong>ID, Kind, Version, State,</strong> and
      <strong>Caps</strong> (the capabilities it's allowed). A switch flips it <em>Active / Disabled</em>.
      The buttons:</p>
      <ul>
        <li><strong>⚙️ Setting</strong> — the main config popup (below).</li>
        <li><strong>📊 Diagnostics</strong> — health and info for this agent.</li>
        <li><strong>📚 Educational Errors</strong> — this agent's own "doctrine" store: the mistakes it turned into lessons.</li>
        <li><strong>⧉ Duplicate</strong> — copy this agent into a new one.</li>
        <li><strong>/ Slash</strong> — a quick slash command for it.</li>
        <li><strong>⬇ Download</strong> — export it back to a <code>.fwagent.zip</code>.</li>
        <li><strong>🗑 Remove</strong> — delete it (folder + workspace + brain).</li>
      </ul>

      <h4>The Setting popup</h4>
      <p>Everything here is isolated to just this one agent:</p>
      <ul>
        <li><strong>Router</strong> — which LLM endpoint it calls, and the model name it asks for.</li>
        <li><strong>Prompt</strong> — its system prompt: who it is, its persona and rules.</li>
        <li><strong>Tools</strong> — tick what it may use: Telegram, the LLM router, a KV store, the filesystem (inside its own workspace), and outbound net fetch.</li>
        <li><strong>Schedule</strong> — recurring jobs in cron format (<code>min hr dom mon dow</code>), one-shot or repeating.</li>
        <li><strong>Skills</strong> — extra skills it can pick up.</li>
      </ul>

      <h4>For developers — make your own agent</h4>
      <p>An agent is a folder, zipped as <code>.fwagent.zip</code>. The easiest start is to copy a
      template — it's already a working "loket-native" agent that reaches every capability through one
      kernel door: <code>call(cap, args)</code>. A folder looks like:</p>
      <pre><code>my-agent.fwagent/
├─ manifest.json   the contract (below)
├─ agent.wasm      the compiled agent
├─ main.go         your logic
├─ prompt.md       its persona / system prompt
└─ doktrin.md      its "lessons" doctrine</code></pre>
      <p>The <code>manifest.json</code> is the contract:</p>
      <pre><code>{
  "id": "my-agent",
  "version": "1.0.0",
  "kind": "agent",
  "display_name": "My Agent",
  "entry": "agent.wasm",
  "abi_version": 1,
  "memory_max_mb": 16,
  "timeout_call_ms": 120000,
  "capabilities_required": [
    "net:fetch:http://127.0.0.1:1987/api/kernel/call",
    "state:read", "state:write", "time:read"
  ],
  "exposes_rpc": [
    { "name": "handle_message",
      "description": "Handle one message.",
      "input_schema": { "type": "object", "properties": {} } }
  ]
}</code></pre>
      <p><code>capabilities_required</code> is the agent's permission list — it can only do what's declared
      there. <code>exposes_rpc</code> is the functions it offers (like <code>handle_message</code>). Build
      it with plain Go — no special toolchain:</p>
      <pre><code>GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .</code></pre>
      <p>Then zip the folder to <code>my-agent.fwagent.zip</code> and drag it into this tab. It hot-loads,
      and you tune the rest (router, prompt, tools, schedule) from the Setting popup.</p>`,
  },
  {
    id: 'group',
    title: '👥 Group (in depth)',
    body: `
      <h3>A team of agents that tackle one task together</h3>
      <p>Think a colony of ants: each does one small job, then someone brings the pieces together. You
      don't build a group from scratch — you pick which agents are on the team and how their answers get
      combined. The whole idea: many small, focused agents beat one big do-everything agent.</p>

      <h4>Create a group</h4>
      <p>At the top, type an <strong>ID</strong> and a <strong>Name</strong>, then hit
      <strong>+ Create</strong>. The new group shows up below as a card.</p>

      <h4>The group card</h4>
      <ul>
        <li><strong>Name &amp; ID</strong> — the name is editable; the id is shown underneath.</li>
        <li><strong>Members</strong> — chips of available agents; tick the ones you want on the team. An
        agent can only be on one group at a time, so the picker only shows agents that are free or
        already yours.</li>
        <li><strong>Synthesizer</strong> — one agent that takes everyone's answers and stitches them into
        the final result (or "none").</li>
        <li><strong>Task</strong> — what the team should do.</li>
        <li><strong>Save</strong> — store the roster + task.</li>
        <li><strong>🗑 Delete</strong> — remove the group.</li>
      </ul>

      <h4>How it runs</h4>
      <p>When the group runs, it fans the one task out to each member over the internal "loket bus",
      collects their answers, and the synthesizer combines them into one result. Members work in
      isolation — that isolation is the point.</p>

      <h4>For developers — make your own group</h4>
      <p>The simplest group is <strong>no code</strong>: create one, tick members, pick a synthesizer,
      write the task, Save.</p>
      <p>Under the hood a group is just an <em>agent</em> built from a template — a coordinator whose
      <code>handle_message</code> routes the task to its members through the loket door
      (<code>call(cap, args)</code>) and gathers the answers. For custom orchestration — phases, specific
      roles, a gather-then-decide flow — start from <code>templates/group-template/</code> (the generic
      fan-out) or a richer real example like <code>templates/investment-group/</code> (a multi-phase
      analyst team), edit <code>main.go</code>, then build it like any agent:</p>
      <pre><code>GOOS=wasip1 GOARCH=wasm go build -o agent.wasm .</code></pre>
      <p>Members are themselves ordinary agents — so building a great group is really about writing small,
      sharp specialist agents and wiring them together.</p>`,
  },
  {
    id: 'connections',
    title: '🔌 Connections (in depth)',
    body: `
      <h3>One roof for everything coming in and out</h3>
      <p>Connections has <strong>two kinds</strong>: <strong>channels</strong> (how people talk to your
      agents) and <strong>MCP servers</strong> (tools from the outside world your agents can use).</p>

      <h4>1) Channels — human ↔ agent</h4>
      <p>The doors people use to reach an agent: Telegram, Discord, Slack, WhatsApp, CLI, and so on.</p>
      <ul>
        <li><strong>Install</strong> — drop a <code>.fwpack</code> (a <code>kind:channel</code> pack) into
        the drop zone. It validates, extracts to its own folder, and hot-loads — no restart.</li>
        <li>Each connector is a card with: its name + an on/off state, <strong>Enable / Disable</strong>,
        <strong>Config</strong> (set its token and settings — the fields come straight from the
        connector's own manifest, so secrets are masked and nothing is hardcoded; Save / Close), and
        <strong>🗑 Uninstall</strong> (deletes its folder, config + token included).</li>
        <li><em>Native</em> connectors are built in — they only have <strong>Config</strong> (no
        enable/uninstall).</li>
      </ul>

      <h4>2) MCP — external tool servers for your agents</h4>
      <p>MCP (Model Context Protocol) is a standard way to plug external tools into AI. Flowork speaks it
      <strong>both ways</strong>.</p>
      <p><strong>Using outside tools (MCP client).</strong> Paste the same <code>mcpServers</code> JSON
      you'd use in any MCP-compatible app:</p>
      <pre><code>{
  "github": {
    "command": "npx",
    "args": ["-y", "@modelcontextprotocol/server-github"],
    "env": { "GITHUB_TOKEN": "..." }
  }
}</code></pre>
      <p>Hit <strong>Install + enable</strong>. Flowork starts that server and its tools become available
      to all your agents (you can untick them per-agent later, in each agent's <em>Tools</em>). Each MCP
      card shows the tools it exposes, plus <strong>Enable / Disable</strong> and
      <strong>Uninstall</strong>.</p>
      <p><strong>Exposing your agents (MCP server).</strong> Flowork can also <em>be</em> the MCP server,
      so an outside AI app or IDE can drive your agents. It speaks MCP over stdio and exposes a small set
      of tools: <code>chat</code> (talk to an agent — the same path as Telegram or the CLI), plus
      <code>task_list</code>, <code>task_run</code>, and <code>task_result</code>. Point your external
      client at the <code>flowork-mcp</code> command; the target agent comes from the
      <code>FLOWORK_MCP_AGENT</code> setting.</p>

      <h4>For developers — make a connector</h4>
      <p>A channel connector is just another plug-and-play module: a <code>kind:channel</code>
      <code>.fwpack</code>. Its manifest declares the config fields it needs (like a bot token), and the
      Config panel renders them for you. Start from <code>templates/connector-template/</code>, fill in
      the relay logic (a dumb pipe: a message comes in → an agent handles it → a reply goes out), build
      it, and drop the <code>.fwpack</code> into this tab. For <strong>MCP</strong> you don't build
      anything — you just paste a server's config (client side), or point an external client at Flowork's
      MCP server (server side).</p>`,
  },
  {
    id: 'schedule',
    title: '⏰ Schedule (in depth)',
    body: `
      <h3>Jobs that run on a clock</h3>
      <p>Hit <strong>＋ New</strong>, pick an <em>agent</em> or a <em>group</em> to run, write what it
      should do, and give it a cron time (<code>min hr dom mon dow</code>) — e.g. <code>0 7 * * *</code>
      for 7am every day. It can repeat or fire just once.</p>
      <p>Each scheduled job is a card with:</p>
      <ul>
        <li><strong>Enable / Disable</strong> — turn it on or off.</li>
        <li><strong>▷ Run</strong> — run it right now.</li>
        <li><strong>✎ Edit</strong> — change it.</li>
        <li><strong>▸ History</strong> — its past runs.</li>
        <li><strong>🗑 Delete</strong> — remove it.</li>
      </ul>
      <p><strong>Principle:</strong> recurring work without you babysitting it. Schedule and Trigger are
      two faces of the same little engine — Schedule is the time-based half.</p>`,
  },
  {
    id: 'trigger',
    title: '⚡ Trigger (in depth)',
    body: `
      <h3>Jobs that run when something happens</h3>
      <p>Not on a clock — on an event. Hit <strong>＋ New</strong> and pick a type:</p>
      <ul>
        <li><strong>File watch</strong> — fires when a file or folder changes.</li>
        <li><strong>Webhook</strong> — fires when something POSTs to it.</li>
      </ul>
      <p>Then choose what runs (an agent or a group), what it should do, and where to deliver the result
      (e.g. Telegram). Each trigger card has <strong>Enable/Disable, ▷ Run, ✎ Edit, ▸ History, 🗑 Delete</strong>.
      A webhook trigger also shows its URL:</p>
      <pre><code>POST  http://your-host/api/triggers/hook/&lt;id&gt;?key=&lt;secret&gt;</code></pre>
      <p>Anything that can make an HTTP POST can fire it — a script, a camera, a web service — and the
      secret keeps it private.</p>
      <p><strong>Principle:</strong> the outside world pokes Flowork → an agent does something → you get
      the answer.</p>`,
  },
  {
    id: 'app',
    title: '▦ App (in depth)',
    body: `
      <h3>Little self-contained programs that live inside Flowork</h3>
      <p>Each app is <strong>both a screen you click and a set of tools your agents can use</strong> —
      "one state, two drivers". (A quant desk and a notepad ship as examples.)</p>
      <p>Two tabs at the top: <strong>Installed</strong> and <strong>Store</strong>.</p>
      <ul>
        <li><strong>Install</strong> — upload a <code>.fwpack</code>. Because an app can run a real
        program on your computer, installing asks for your consent first.</li>
        <li><strong>Open</strong> — launches the app in a locked-down sandboxed frame; it can only talk to
        Flowork through validated <em>ops</em> (it asks <code>{op, args}</code>, the host checks the op is
        declared in the app's manifest, runs it, and sends the result back).</li>
        <li><strong>Uninstall</strong> — remove it.</li>
      </ul>
      <h4>For developers — make an app</h4>
      <p>An app is a folder under <code>apps/&lt;id&gt;/</code> with three things:</p>
      <pre><code>apps/my-app/
├─ manifest.json   kind:"app" + the list of ops
├─ core.py         the headless logic (talks over stdin/stdout, line-JSON)
└─ ui/index.html   the screen (sandboxed iframe)</code></pre>
      <p>Every op you declare becomes <strong>both a GUI button and an agent tool</strong> at the same
      time.</p>
      <p><strong>Principle:</strong> write the logic once; it's driven by a human clicking and by an agent
      calling — same state, two drivers.</p>`,
  },
  {
    id: 'ai-studio',
    title: '🧬 AI Studio (in depth)',
    body: `
      <h3>The workshop where Flowork builds and prunes itself</h3>
      <ul>
        <li><strong>⚡ Design</strong> — describe what you want and it generates a module or agent.</li>
        <li><strong>Pending</strong> — anything it generated waits here for your call: <strong>⏍ Approve</strong>
        or <strong>⊗ Reject</strong>. Nothing lands without you.</li>
        <li><strong>Reaper</strong> — apoptosis. It lists modules that look dead or flagged, and you can
        <strong>⊘ Reap</strong> them (remove). Owner-gated, so the AI can't quietly delete things.</li>
      </ul>
      <p><strong>Principle:</strong> the system can grow new parts and shed dead ones, but a human always
      holds the approve button. Build with a conscience; clean up without fear.</p>`,
  },
  {
    id: 'audit-log',
    title: '📋 Audit Log (in depth)',
    body: `
      <h3>The receipt drawer</h3>
      <p>A running record of what changed in the system, newest first. It's read-only — you don't do
      anything here except look. When something changes, you can see what and when.</p>
      <p><strong>Principle:</strong> nothing happens in the dark — every change leaves a trace you can
      read.</p>`,
  },
  {
    id: 'settings',
    title: '⚙️ Settings (in depth)',
    body: `
      <h3>Your owner-level control panel</h3>
      <p>Global stuff that isn't tied to any one agent (kept in the global <code>flowork.db</code>):</p>
      <ul>
        <li><strong>Account</strong> — change your password, or log out.</li>
        <li><strong>API Keys</strong> — add, edit, or delete the keys Flowork uses (stored as secrets,
        shown masked).</li>
        <li><strong>Notifications</strong> — your own Telegram: paste a bot token + chat id, <em>Save</em>,
        and <em>Test</em> (sends you a test message). This is the token the whole system uses to ping
        you — yours, never hardcoded.</li>
        <li><strong>YouTube</strong> — connect a YouTube account by OAuth, for the automation that needs it.</li>
      </ul>
      <p><strong>Principle:</strong> one place for the owner's keys and switches, kept apart from each
      agent's private settings.</p>`,
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
    id: 'map',
    title: '🗺️ Project Map',
    body: `
      <h3>Where everything lives</h3>
      <p>The whole project is just folders. Here's the lay of the land:</p>
      <pre><code>Flowork_Agent/
├─ main.go + *.go ........ the microkernel + HTTP handlers (the "forever" core)
├─ start.sh / stop.sh / restart.sh ... run / stop / rebuild scripts
├─ go.mod ................ Go module
│
├─ internal/ ............. the core Go packages
│   ├─ kernel/ , kernelhost/ ... the WASM microkernel + capability broker
│   ├─ loket/ ............ the one "counter" every module calls through (the ABI)
│   ├─ guardian/ , protector/ .. self-protection (tamper → safe-mode)
│   ├─ floworkdb/ , agentdb/ ... databases (global + each agent's private brain)
│   ├─ tools/ ............ the built-in tools
│   ├─ scanner/ , scanapi/ , codescan/ , codemap/ ... the security scanner
│   ├─ triggers/ , scheduler/ , taskflow/ ... automation
│   ├─ connections/ , mcpclient/ , mcphub/ ... channels + MCP
│   ├─ groupsapi/ , slashcmd/ , settingsapi/ ... groups, commands, settings
│   ├─ floworkauth/ ...... owner login
│   └─ routerclient/ , marketdata/ , watchdog/ , reaper, zombie/ ...
│
├─ web/ ................. the control panel UI
│   ├─ index.html ....... the shell + sidebar
│   ├─ js/ .............. app logic (app.js router, i18n, utils)
│   ├─ tabs/ ............ one file per menu (agents.js, scanner.js, document.js …)
│   ├─ i18n/ ............ translations (en, id)
│   └─ css/ , vendor/
│
├─ apps/ ............... sandboxed apps (flowalpha, notepad)
├─ agents/ ............. installed agents (each a .fwagent folder)
├─ templates/ ......... starting points (agent, group, connector, lens, planner …)
├─ cmd/ ............... extra entry points (CLI, TUI, MCP, chat)
├─ sdk/ , doc/ , scripts/ , seeds/ , voice-backend/ ... SDK, docs, helpers, voice
└─ README.md , CHANGELOG.md ... project docs</code></pre>
      <p>The golden rule: the <strong>core</strong> (top-level <code>*.go</code> + <code>internal/kernel</code>)
      is written once and left alone. Everything you'd ever add — a tool, a scanner, a channel, an app —
      is its own folder that snaps on. Break one, fix one; the rest never notices.</p>`,
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
    <div class="doc-layout" style="display:flex;gap:16px;align-items:flex-start">
      <nav class="doc-toc" style="position:sticky;top:8px;display:flex;flex-direction:column;gap:4px;min-width:180px;max-height:calc(100vh - 120px);overflow:auto">
        <div style="opacity:.55;font-size:.8em;letter-spacing:.08em;margin:2px 6px 4px">CONTENTS</div>
        ${SECTIONS.map((s) => `<button class="doc-link" data-doc="${esc(s.id)}"
          style="text-align:left;padding:7px 12px;border-radius:8px;cursor:pointer">${esc(s.title)}</button>`).join('')}
      </nav>
      <div class="doc-body" style="flex:1;min-width:280px;line-height:1.6">
        ${SECTIONS.map((s) => `<section id="doc-${esc(s.id)}" class="card" style="padding:18px;margin-bottom:16px;scroll-margin-top:12px">${s.body}</section>`).join('')}
      </div>
    </div>`;

  // Table of contents → smooth-scroll to the section (buttons, not #hash anchors, so the
  // app's hash-based tab router is never disturbed).
  mainEl.querySelectorAll('.doc-link').forEach((b) => {
    b.onclick = () => {
      const sec = mainEl.querySelector('#doc-' + b.dataset.doc);
      if (sec) sec.scrollIntoView({ behavior: 'smooth', block: 'start' });
      mainEl.querySelectorAll('.doc-link').forEach((x) => x.classList.toggle('active', x === b));
    };
  });
}
