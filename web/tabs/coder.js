// coder.js — tab "AI Studio" (CODER, roadmap 2.2). Owner minta app baru →
// CODER (Opus) rancang → VERIFIER cek → masuk Approval Queue → owner approve
// (install via pipeline) / reject.
//
// Catatan keras: verdict = DATA VIEW MENTAH (bukan LLM-summarize) · approve =
// reuse installPluginPack · GUI additive (tab baru, ga sentuh yang lama).
// Tampilan: HUD "Jarvis" — neon cyan, glass, scanline, corner-bracket.

import { esc, fetchJSON, loadStyle } from '../js/utils.js';
import { t } from '/js/i18n.js';

// i18n: semua label GUI lewat dictionary (en base + id) — NO hardcode.
const T = (k) => t('coder.' + k);
const fmt = (k, v) => T(k).replace(/{(\w+)}/g, (_, n) => (v[n] != null ? v[n] : ''));
const vbadge = (status) => T('v_' + status) || (status || '?');
const CST = { pass: '✓', warn: '!', fail: '✕' };

export async function render(mainEl) {
  // loadStyle nge-SKIP kalau <style> udah ada (cache by id) → update CSS ga
  // ke-apply pas SPA nav. Hapus dulu biar STYLE terbaru SELALU ke-inject.
  document.getElementById('tab-style-coder-style')?.remove();
  loadStyle('coder-style', STYLE);
  mainEl.innerHTML = `
    <div class="cd-wrap">
      <div class="cd-scan"></div>
      <div class="cd-head">
        <div class="cd-htitle">
          <span class="cd-core"></span>
          <div>
            <h2>${esc(T('title')).toUpperCase()} <span class="cd-tag">${esc(T('tag')).toUpperCase()}</span></h2>
            <p class="cd-sub">${esc(T('sub'))}</p>
          </div>
        </div>
        <div class="cd-status"><span class="cd-dot"></span> ${esc(T('status_online')).toUpperCase()}</div>
      </div>

      <div class="cd-gen cd-panel">
        <div class="cd-prompt">⟢ ${esc(T('design_request')).toUpperCase()}</div>
        <textarea id="cd-task" rows="2" placeholder="${esc(T('task_placeholder'))}"></textarea>
        <div class="cd-genrow">
          <select id="cd-model">
            <option value="">◇ ${esc(T('model_opus'))}</option>
            <option value="claude-sonnet-4-6">◇ ${esc(T('model_sonnet'))}</option>
            <option value="claude-haiku-4-5">◇ ${esc(T('model_haiku'))}</option>
          </select>
          <button class="cd-btn primary" id="cd-gen"><span>⚡ ${esc(T('design_btn')).toUpperCase()}</span></button>
        </div>
        <div id="cd-genmsg" class="cd-msg"></div>
      </div>

      <div class="cd-qh"><span class="cd-bar"></span>${esc(T('queue_title')).toUpperCase()} <span id="cd-count" class="cd-count">0</span></div>
      <div id="cd-queue" class="cd-queue"><div class="cd-empty">⟳ ${esc(T('loading'))}</div></div>

      <div class="cd-qh"><span class="cd-bar warn"></span>${esc(T('reaper_title')).toUpperCase()} <span id="cd-rflag" class="cd-count">0</span> <span class="cd-qhsub">${esc(T('reaper_sub'))}</span></div>
      <div id="cd-reaper" class="cd-reaper"><div class="cd-empty">⟳ ${esc(T('loading'))}</div></div>

    </div>`;

  const genBtn = mainEl.querySelector('#cd-gen');
  const taskEl = mainEl.querySelector('#cd-task');
  const modelEl = mainEl.querySelector('#cd-model');
  const msgEl = mainEl.querySelector('#cd-genmsg');

  genBtn.onclick = async () => {
    const task = taskEl.value.trim();
    if (!task) { msgEl.textContent = '⚠ ' + T('need_task'); msgEl.className = 'cd-msg warn'; return; }
    genBtn.disabled = true;
    genBtn.classList.add('busy');
    msgEl.textContent = '◢ ' + T('designing');
    msgEl.className = 'cd-msg busy';
    try {
      const body = { task };
      if (modelEl.value) body.model = modelEl.value;
      const res = await fetchJSON('/api/coder/generate', { method: 'POST', body: JSON.stringify(body) });
      msgEl.textContent = '✓ ' + fmt('designed', { name: res.spec?.name || res.pending_id, status: res.verify?.status });
      msgEl.className = 'cd-msg ok';
      taskEl.value = '';
      await loadQueue(mainEl);
    } catch (e) {
      msgEl.textContent = '✕ ' + fmt('failed', { err: e.message || e });
      msgEl.className = 'cd-msg err';
    } finally {
      genBtn.disabled = false;
      genBtn.classList.remove('busy');
    }
  };

  // Load PARALEL — tiap section ngisi independen (punya try/catch sendiri).
  // Tools + slash install GLOBAL dicabut (§E1): tools & slash sekarang per-agent
  // (lihat tab AI Agent → katalog tool + slash per kartu), bukan katalog global.
  await Promise.all([
    loadQueue(mainEl),
    loadReaper(mainEl),
  ]);
}

const RSEV = { healthy: '◉', warn: '▲', critical: '✕' };

async function loadReaper(mainEl) {
  const wrap = mainEl.querySelector('#cd-reaper');
  const flagEl = mainEl.querySelector('#cd-rflag');
  let data;
  try {
    data = await fetchJSON('/api/reaper/candidates');
  } catch (e) {
    wrap.innerHTML = `<div class="cd-empty err">${esc(fmt('health_load_fail', { err: e.message || '' }))}</div>`;
    return;
  }
  flagEl.textContent = data.flagged || 0;
  const cands = (data.candidates || []).sort((a, b) => (b.flagged - a.flagged) || (b.error_rate - a.error_rate));
  if (!cands.length) {
    wrap.innerHTML = `<div class="cd-empty">${esc(T('no_apps'))}</div>`;
    return;
  }
  wrap.innerHTML = cands.map((c) => {
    const code = c.reason_code || 'unknown';
    const reason = code === 'failing'
      ? fmt('reason_failing', { rate: (c.error_rate * 100).toFixed(0), count: c.done + c.error })
      : T('reason_' + code);
    return `
    <div class="cd-hrow sev-${esc(c.severity)} ${c.flagged ? 'flagged' : ''}">
      <span class="cd-hsev">${RSEV[c.severity] || '•'}</span>
      <span class="cd-hname">${esc(c.category_id)}</span>
      <span class="cd-hstat"><b class="ok">✓${c.done}</b> <b class="bad">✕${c.error}</b> · ${(c.error_rate * 100).toFixed(0)}% ${esc(T('err'))} · ${esc(T('smoke'))} <b>${esc(c.smoke)}</b></span>
      <span class="cd-hreason">${esc(reason)}</span>
      ${c.flagged ? `<button class="cd-btn danger sm" data-reap="${esc(c.category_id)}">⊘ ${esc(T('reap')).toUpperCase()}</button>` : ''}
    </div>`;
  }).join('');
  wrap.querySelectorAll('[data-reap]').forEach((b) => {
    b.onclick = async () => {
      const id = b.dataset.reap;
      if (!confirm(fmt('confirm_reap', { id }))) return;
      b.disabled = true; b.textContent = '⟳ ' + T('reaping');
      try {
        await fetchJSON(`/api/reaper/reap?category=${encodeURIComponent(id)}`, { method: 'POST' });
      } catch (e) { alert(fmt('reap_fail', { err: e.message || e })); }
      await loadReaper(mainEl);
    };
  });
}

async function loadQueue(mainEl) {
  const wrap = mainEl.querySelector('#cd-queue');
  const countEl = mainEl.querySelector('#cd-count');
  let data;
  try {
    data = await fetchJSON('/api/coder/pending');
  } catch (e) {
    wrap.innerHTML = `<div class="cd-empty err">${esc(fmt('queue_load_fail', { err: e.message || '' }))}</div>`;
    return;
  }
  const pending = data.pending || [];
  countEl.textContent = pending.length;
  if (!pending.length) {
    wrap.innerHTML = `<div class="cd-empty">◇ ${esc(T('queue_empty'))}</div>`;
    return;
  }
  wrap.innerHTML = pending.map((p) => cardHTML(p)).join('');
  wrap.querySelectorAll('[data-approve]').forEach((b) => {
    b.onclick = () => act(mainEl, 'approve', b.dataset.approve, b);
  });
  wrap.querySelectorAll('[data-reject]').forEach((b) => {
    b.onclick = () => act(mainEl, 'reject', b.dataset.reject, b);
  });
}

function judgeHTML(j) {
  if (!j || !j.verdict) return '';
  const flags = (j.redflags || []).map((f) => `<li>⚠ ${esc(f)}</li>`).join('');
  return `
    <div class="cd-judge v-${esc(j.verdict)}">
      <div class="cd-jhead"><span class="cd-jlabel">⟁ ${esc(T('ai_judge'))}</span>
        <span class="cd-jverdict">${esc(T('judge_' + j.verdict) || j.verdict)} · ${j.score ?? '?'}</span></div>
      <div class="cd-jreason">${esc(j.reason || '')}</div>
      ${flags ? `<ul class="cd-jflags">${flags}</ul>` : ''}
    </div>`;
}

function cardHTML(p) {
  const s = p.spec || {};
  const v = p.verify || {};
  const score = v.score ?? 0;
  const checks = (v.checks || [])
    .map((c) => `<li class="${c.status}"><span class="cd-ci">${CST[c.status] || '•'}</span> <b>${esc(c.name)}</b> <span class="cd-cdetail">${esc(c.detail || '')}</span></li>`)
    .join('');
  return `
    <div class="cd-card cd-panel v-${esc(v.status || '')}">
      <div class="cd-ctop">
        <div class="cd-cname"><span class="cd-cico">${esc(s.icon || '◰')}</span> ${esc(s.name || p.id)}</div>
        <div class="cd-verdict v-${esc(v.status || '')}">${esc(vbadge(v.status))}<span class="cd-vscore">${score}</span></div>
      </div>
      <div class="cd-cmeta"><code>${esc(p.id)}</code><span class="cd-sep">│</span>${esc(T('worker'))}: <b>${esc(s.worker_role || '-')}</b><span class="cd-sep">│</span>${esc(T('model'))}: <b>${esc(p.model || '?')}</b></div>
      <div class="cd-ctask">${esc(p.task || '')}</div>
      <div class="cd-meter"><div class="cd-meterfill" style="width:${Math.max(0, Math.min(100, score))}%"></div></div>
      ${judgeHTML(p.judge)}
      <details class="cd-cdir"><summary>${esc(T('persona_directive'))}</summary>
        <div class="cd-kv"><b>${esc(T('worker_persona'))}</b> ${esc((s.worker_persona || '').slice(0, 260))}</div>
        <div class="cd-kv"><b>${esc(T('worker_directive'))}</b> ${esc((s.worker_directive || '').slice(0, 260))}</div>
        <div class="cd-kv"><b>${esc(T('synth_directive'))}</b> ${esc((s.synth_directive || '').slice(0, 260))}</div>
      </details>
      <details class="cd-cchecks"><summary>${esc(T('checks'))} · ${(v.checks || []).length}</summary><ul>${checks}</ul></details>
      <div class="cd-cact">
        <button class="cd-btn ok" data-approve="${esc(p.id)}" data-blocked="${v.status === 'blocked' ? '1' : ''}">⏍ ${esc(T('approve')).toUpperCase()}</button>
        <button class="cd-btn ghost" data-reject="${esc(p.id)}">⊗ ${esc(T('reject')).toUpperCase()}</button>
      </div>
    </div>`;
}

async function act(mainEl, kind, id, btn) {
  if (kind === 'reject' && !confirm(fmt('confirm_reject', { id }))) return;
  // The Verifier is a real gate: a BLOCKED pack needs a conscious owner override.
  let override = '';
  if (kind === 'approve' && btn.dataset.blocked === '1') {
    if (!confirm(T('confirm_blocked') || '⛔ The Verifier BLOCKED this pack (failed safety checks).\nInstall it anyway? Only do this if you fully trust it.')) return;
    override = '&override=1';
  }
  btn.disabled = true;
  const orig = btn.textContent;
  btn.textContent = '⟳ ' + (kind === 'approve' ? T('installing') : T('removing'));
  try {
    const res = await fetchJSON(`/api/coder/${kind}?id=${encodeURIComponent(id)}${override}`, { method: 'POST' });
    if (kind === 'approve' && res.ok === false) {
      alert(fmt('install_fail', { err: res.error || JSON.stringify(res) }));
    }
  } catch (e) {
    alert(fmt('action_fail', { kind, err: e.message || e }));
    btn.disabled = false;
    btn.textContent = orig;
    return;
  }
  await loadQueue(mainEl);
}

// AI Studio HUD style — internal to this tab. (Was exported for a scanner_active.js
// that no longer exists; the active scanner now lives in scanner.js with its own .rx-*
// CSS, so the export + the tool/allowlist/triage rules below were dead and removed.)
const STYLE = `
.cd-wrap{position:relative;padding:22px 32px 60px;width:100%;box-sizing:border-box;color:#cfe9f2;
  font-family:ui-monospace,'SF Mono','Cascadia Code','JetBrains Mono','Consolas',monospace;
  --disp:ui-monospace,'SF Mono','Cascadia Code',monospace;
  background:
    radial-gradient(1200px 400px at 70% -120px,rgba(0,200,255,.10),transparent 60%),
    linear-gradient(rgba(8,18,26,.0) 0,rgba(8,18,26,.0) 100%);
  --cy:#36e6ff;--cy2:#26ffd0;--ink:#06121a;--line:rgba(54,230,255,.22);--bad:#ff476f;--warn:#ffc24d}
.cd-wrap::before{content:'';position:absolute;inset:0;z-index:0;pointer-events:none;opacity:.35;
  background-image:linear-gradient(var(--line) 1px,transparent 1px),linear-gradient(90deg,var(--line) 1px,transparent 1px);
  background-size:38px 38px;mask:radial-gradient(circle at 50% 0,#000,transparent 80%)}
.cd-wrap>*{position:relative;z-index:1}
.cd-scan{position:absolute;left:0;right:0;top:0;height:120px;z-index:0;pointer-events:none;
  background:linear-gradient(180deg,rgba(54,230,255,.10),transparent);animation:cdscan 6s linear infinite}
@keyframes cdscan{0%{transform:translateY(-40px);opacity:0}30%{opacity:1}100%{transform:translateY(620px);opacity:0}}

.cd-head{display:flex;justify-content:space-between;align-items:flex-start;gap:16px;
  border-bottom:1px solid var(--line);padding-bottom:14px;margin-bottom:20px}
.cd-htitle{display:flex;gap:14px;align-items:center}
.cd-core{width:34px;height:34px;border-radius:50%;flex:0 0 auto;position:relative;
  background:radial-gradient(circle,#aef6ff 0,var(--cy) 38%,transparent 70%);
  box-shadow:0 0 14px var(--cy),0 0 30px rgba(54,230,255,.5);animation:cdpulse 2.4s ease-in-out infinite}
.cd-core::after{content:'';position:absolute;inset:-6px;border:1px solid var(--cy);border-radius:50%;
  border-top-color:transparent;border-left-color:transparent;animation:cdspin 3s linear infinite}
@keyframes cdpulse{0%,100%{transform:scale(1);filter:brightness(1)}50%{transform:scale(1.08);filter:brightness(1.3)}}
@keyframes cdspin{to{transform:rotate(360deg)}}
.cd-head h2{margin:0;font-family:var(--disp);font-weight:800;font-size:21px;letter-spacing:3px;
  color:#eafdff;text-shadow:0 0 10px rgba(54,230,255,.6)}
.cd-tag{font-family:ui-monospace,monospace;font-size:10px;letter-spacing:2px;color:var(--ink);
  background:linear-gradient(90deg,var(--cy),var(--cy2));padding:2px 8px;border-radius:3px;vertical-align:3px;font-weight:700}
.cd-sub{color:#7fb9cc;font-size:12px;margin:5px 0 0;letter-spacing:.4px;max-width:560px}
.cd-status{font-size:11px;letter-spacing:2px;color:var(--cy2);white-space:nowrap;display:flex;align-items:center;gap:7px;
  border:1px solid var(--line);padding:5px 10px;border-radius:4px;background:rgba(38,255,208,.05)}
.cd-dot{width:8px;height:8px;border-radius:50%;background:var(--cy2);box-shadow:0 0 8px var(--cy2);animation:cdblink 1.6s ease-in-out infinite}
@keyframes cdblink{0%,100%{opacity:1}50%{opacity:.25}}

.cd-panel{position:relative;background:linear-gradient(160deg,rgba(12,28,38,.72),rgba(8,18,26,.82));
  border:1px solid var(--line);border-radius:6px;backdrop-filter:blur(6px);
  box-shadow:inset 0 0 30px rgba(0,180,255,.04),0 6px 24px rgba(0,0,0,.35)}
.cd-panel::before,.cd-panel::after{content:'';position:absolute;width:14px;height:14px;border:2px solid var(--cy);pointer-events:none;opacity:.8}
.cd-panel::before{top:-1px;left:-1px;border-right:0;border-bottom:0}
.cd-panel::after{bottom:-1px;right:-1px;border-left:0;border-top:0}

.cd-gen{margin:0 0 26px;padding:16px}
.cd-prompt{font-size:11px;letter-spacing:3px;color:var(--cy);margin-bottom:9px;text-shadow:0 0 8px rgba(54,230,255,.4)}
.cd-gen textarea{width:100%;box-sizing:border-box;background:rgba(2,10,16,.85);border:1px solid var(--line);
  border-radius:4px;color:#d6f6ff;padding:11px 12px;font:inherit;font-size:13px;resize:vertical;outline:none;transition:.2s}
.cd-gen textarea:focus{border-color:var(--cy);box-shadow:0 0 0 1px var(--cy),0 0 18px rgba(54,230,255,.25)}
.cd-gen textarea::placeholder{color:#4d7a8a}
.cd-genrow{display:flex;gap:10px;margin-top:10px;align-items:center}
.cd-genrow select{background:rgba(2,10,16,.85);border:1px solid var(--line);border-radius:4px;color:var(--cy2);
  padding:9px 10px;font:inherit;font-size:12px;letter-spacing:1px;outline:none;cursor:pointer}
.cd-btn{font-family:ui-monospace,monospace;letter-spacing:1.5px;font-size:12px;cursor:pointer;
  background:transparent;border:1px solid var(--line);color:#cfe9f2;padding:9px 15px;border-radius:4px;
  transition:.18s;position:relative;overflow:hidden}
.cd-btn:hover{border-color:var(--cy);color:#eafdff;box-shadow:0 0 14px rgba(54,230,255,.3);background:rgba(54,230,255,.07)}
.cd-btn:disabled{opacity:.45;cursor:default;box-shadow:none}
.cd-btn.primary{border-color:var(--cy);color:#001a22;background:linear-gradient(90deg,var(--cy),var(--cy2));font-weight:700}
.cd-btn.primary:hover{box-shadow:0 0 22px rgba(54,230,255,.6);filter:brightness(1.08)}
.cd-btn.primary.busy{animation:cdbusy 1s linear infinite}
@keyframes cdbusy{50%{filter:brightness(1.4)}}
.cd-btn.ok{border-color:var(--cy2);color:var(--cy2)}.cd-btn.ok:hover{background:rgba(38,255,208,.1);box-shadow:0 0 14px rgba(38,255,208,.35)}
.cd-btn.danger{border-color:var(--bad);color:var(--bad)}.cd-btn.danger:hover{background:rgba(255,71,111,.12);box-shadow:0 0 14px rgba(255,71,111,.35)}
.cd-btn.ghost{opacity:.8}
.cd-btn.sm{padding:5px 11px;font-size:11px;margin-left:auto}
.cd-msg{margin-top:10px;font-size:12px;min-height:16px;color:#7fb9cc;letter-spacing:.4px}
.cd-msg.ok{color:var(--cy2)}.cd-msg.err{color:var(--bad)}.cd-msg.warn{color:var(--warn)}
.cd-msg.busy{color:var(--cy)}

.cd-qh{margin:26px 0 12px;font-family:var(--disp);font-weight:600;font-size:13px;letter-spacing:3px;
  color:#bfeefb;display:flex;align-items:center;gap:10px}
.cd-bar{width:4px;height:16px;background:var(--cy);box-shadow:0 0 10px var(--cy);border-radius:2px}
.cd-bar.warn{background:var(--warn);box-shadow:0 0 10px var(--warn)}
.cd-qhsub{font-family:ui-monospace,monospace;font-size:10px;letter-spacing:1px;color:#5f93a6;font-weight:400}
.cd-count{font-family:ui-monospace,monospace;background:rgba(54,230,255,.12);border:1px solid var(--line);
  border-radius:3px;padding:1px 9px;font-size:12px;color:var(--cy)}

.cd-queue{display:grid;grid-template-columns:repeat(auto-fill,minmax(420px,1fr));gap:14px;align-items:start}
.cd-empty{color:#5f93a6;padding:22px;text-align:center;border:1px dashed var(--line);border-radius:6px;
  background:rgba(8,18,26,.4);font-size:12px;letter-spacing:1px}.cd-empty.err{color:var(--bad);border-color:rgba(255,71,111,.4)}

.cd-card{padding:15px 16px}
.cd-card.v-blocked{border-color:rgba(255,71,111,.4)}.cd-card.v-blocked::before,.cd-card.v-blocked::after{border-color:var(--bad)}
.cd-ctop{display:flex;justify-content:space-between;align-items:center;gap:12px}
.cd-cname{font-family:var(--disp);font-weight:600;font-size:15px;color:#eafdff;display:flex;align-items:center;gap:9px}
.cd-cico{font-size:18px;filter:drop-shadow(0 0 6px rgba(54,230,255,.5))}
.cd-verdict{font-size:11px;letter-spacing:2px;padding:4px 4px 4px 11px;border-radius:4px;white-space:nowrap;
  display:flex;align-items:center;gap:9px;border:1px solid var(--line);background:rgba(2,10,16,.6)}
.cd-vscore{font-family:ui-monospace,monospace;background:rgba(54,230,255,.14);padding:1px 8px;border-radius:3px;font-size:12px}
.cd-verdict.v-approved{color:var(--cy2);border-color:rgba(38,255,208,.45);box-shadow:0 0 12px rgba(38,255,208,.2)}
.cd-verdict.v-review{color:var(--warn);border-color:rgba(255,194,77,.45)}
.cd-verdict.v-blocked{color:var(--bad);border-color:rgba(255,71,111,.5);box-shadow:0 0 12px rgba(255,71,111,.2)}
.cd-cmeta{color:#7fb9cc;font-size:11px;margin:9px 0 7px;letter-spacing:.5px;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
.cd-cmeta code{background:rgba(2,10,16,.8);border:1px solid var(--line);padding:1px 7px;border-radius:3px;color:var(--cy)}
.cd-cmeta b{color:#cfe9f2}.cd-sep{color:#33586a}
.cd-ctask{font-size:13px;margin:4px 0 10px;color:#bfd9e4;line-height:1.5}
.cd-meter{height:3px;background:rgba(2,10,16,.8);border-radius:2px;overflow:hidden;margin-bottom:10px}
.cd-meterfill{height:100%;background:linear-gradient(90deg,var(--cy),var(--cy2));box-shadow:0 0 10px var(--cy);transition:width .6s}
.cd-cdir,.cd-cchecks{margin:7px 0;font-size:12px}
.cd-cdir summary,.cd-cchecks summary{cursor:pointer;color:var(--cy);letter-spacing:1px;list-style:none;user-select:none}
.cd-cdir summary::before,.cd-cchecks summary::before{content:'▸ ';color:var(--cy2)}
details[open] summary::before{content:'▾ '}
.cd-kv{margin:7px 0;color:#9fc4d2;line-height:1.45;font-size:12px}.cd-kv b{color:var(--cy2);display:block;font-size:10px;letter-spacing:1px;margin-bottom:2px}
.cd-cchecks ul{list-style:none;padding:0;margin:7px 0}
.cd-cchecks li{padding:4px 0;color:#9fc4d2;border-bottom:1px solid rgba(54,230,255,.07)}
.cd-cchecks li .cd-ci{display:inline-block;width:16px;color:var(--cy2)}
.cd-cchecks li.fail{color:#ffb0c2}.cd-cchecks li.fail .cd-ci{color:var(--bad)}
.cd-cchecks li.warn{color:#ffe2a8}.cd-cchecks li.warn .cd-ci{color:var(--warn)}
.cd-cdetail{color:#5f93a6}
.cd-cact{display:flex;gap:10px;margin-top:14px}
.cd-judge{margin:10px 0;padding:9px 11px;border:1px solid var(--line);border-left:3px solid var(--cy);border-radius:4px;background:rgba(2,10,16,.5);font-size:12px}
.cd-judge.v-pass{border-left-color:var(--cy2)}.cd-judge.v-review{border-left-color:var(--warn)}.cd-judge.v-fail{border-left-color:var(--bad)}
.cd-jhead{display:flex;justify-content:space-between;align-items:center;gap:8px}
.cd-jlabel{letter-spacing:2px;color:var(--cy);font-size:11px}
.cd-jverdict{font-size:11px;letter-spacing:1px;padding:2px 9px;border-radius:3px;background:rgba(54,230,255,.12);color:var(--cy2)}
.cd-judge.v-review .cd-jverdict{color:var(--warn);background:rgba(255,194,77,.12)}.cd-judge.v-fail .cd-jverdict{color:var(--bad);background:rgba(255,71,111,.12)}
.cd-jreason{color:#9fc4d2;margin-top:6px;line-height:1.45}
.cd-jflags{list-style:none;padding:0;margin:7px 0 0}.cd-jflags li{color:#ffb0c2;padding:2px 0}

.cd-reaper{display:flex;flex-direction:column;gap:8px}
.cd-hrow{display:flex;align-items:center;gap:12px;background:linear-gradient(90deg,rgba(12,28,38,.6),rgba(8,18,26,.5));
  border:1px solid var(--line);border-left:3px solid var(--cy2);border-radius:4px;padding:9px 13px;font-size:12px}
.cd-hrow.flagged{border-left-color:var(--warn);background:linear-gradient(90deg,rgba(40,30,8,.5),rgba(8,18,26,.5))}
.cd-hrow.sev-critical{border-left-color:var(--bad)}
.cd-hsev{font-size:13px;color:var(--cy2)}.cd-hrow.flagged .cd-hsev{color:var(--warn)}.cd-hrow.sev-critical .cd-hsev{color:var(--bad)}
.cd-hname{color:#eafdff;letter-spacing:1px;font-weight:700;min-width:130px}
.cd-hstat{color:#7fb9cc;font-size:11px}.cd-hstat b.ok{color:var(--cy2)}.cd-hstat b.bad{color:var(--bad)}.cd-hstat b{color:#cfe9f2}
.cd-hreason{color:#5f93a6;font-size:11px;margin-left:auto;text-align:right}
`;
