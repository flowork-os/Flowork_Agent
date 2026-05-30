// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Monitor tab — unified dashboard untuk SEMUA fitur baru Section
//   12 phase 3 (approval queue) + 21+22 (wallet+alerts) + 23 (finance)
//   + 24 (protector) + 25 (scanner) + 26 (audit + watchdog) + 27+29
//   (codemap + zombie) + 35 (self-prompt). Collapsible panels per fitur.
//   Plug-and-play modules — tambah fitur baru = tambah section block.
//
// monitor.js — Section "Monitor" SPA tab.

import { t } from '/js/i18n.js';

const AGENT_ID = 'mr-flow'; // single-warga default. Phase 2 multi-agent selector.

function esc(s) {
  return String(s ?? '')
    .replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;').replaceAll("'", '&#39;');
}

async function getJSON(url) {
  try {
    const r = await fetch(url);
    return await r.json();
  } catch (e) {
    return { error: String(e) };
  }
}

async function postJSON(url, body) {
  try {
    const r = await fetch(url, {
      method: 'POST',
      headers: body ? { 'Content-Type': 'application/json' } : {},
      body: body ? JSON.stringify(body) : undefined,
    });
    return await r.json();
  } catch (e) {
    return { error: String(e) };
  }
}

function panel(id, titleKey, defaultIcon, contentHTML) {
  return `
    <details class="mn-panel" id="mn-${id}" style="background:#0f172a;border:1px solid #334155;border-radius:8px;margin-bottom:10px;padding:8px 12px">
      <summary style="cursor:pointer;color:#f1f5f9;font-weight:600;font-size:14px">
        ${defaultIcon} ${esc(t(titleKey) || titleKey)}
      </summary>
      <div style="margin-top:10px">${contentHTML}</div>
    </details>
  `;
}

// =============================================================================
// Section 12 phase 3 — Approval queue
// =============================================================================

async function renderApproval(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/protector/approval/queue?id=${AGENT_ID}&status=pending`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  if (!items.length) {
    host.innerHTML = '<p style="color:#64748b;font-size:12px">No pending approval.</p>';
    return;
  }
  host.innerHTML = items.map((i) => `
    <div style="background:#1e293b;border:1px solid #475569;border-radius:6px;padding:8px;margin-bottom:6px">
      <div style="display:flex;justify-content:space-between;gap:8px">
        <div style="flex:1;min-width:0">
          <div style="color:#f1f5f9;font-family:ui-monospace,monospace;font-size:12px">
            #${i.id} <strong>${esc(i.tool_name)}</strong>
          </div>
          <div style="color:#94a3b8;font-size:11px">${esc(i.reason)}</div>
          <div style="color:#64748b;font-size:11px;margin-top:4px">caller=${esc(i.caller)} · ${esc(i.requested_at)}</div>
          <pre style="color:#cbd5e1;background:#020617;padding:6px;border-radius:4px;font-size:10px;margin:4px 0 0 0;overflow-x:auto">${esc(i.args_json)}</pre>
        </div>
        <div style="display:flex;flex-direction:column;gap:4px;flex-shrink:0">
          <button class="ag-btn primary" data-approve="${i.id}" style="font-size:11px;padding:4px 8px">Approve</button>
          <button class="ag-btn danger" data-reject="${i.id}" style="font-size:11px;padding:4px 8px">Reject</button>
        </div>
      </div>
    </div>
  `).join('');
  host.querySelectorAll('[data-approve]').forEach((b) => b.onclick = async () => {
    await postJSON(`/api/agents/protector/approve_pending?id=${AGENT_ID}&queue_id=${b.dataset.approve}`);
    renderApproval(host);
  });
  host.querySelectorAll('[data-reject]').forEach((b) => b.onclick = async () => {
    await postJSON(`/api/agents/protector/reject_pending?id=${AGENT_ID}&queue_id=${b.dataset.reject}`);
    renderApproval(host);
  });
}

// =============================================================================
// Section 26 — Audit log + Watchdog
// =============================================================================

async function renderAudit(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/audit/log?id=${AGENT_ID}&limit=20`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  if (!items.length) { host.innerHTML = '<p style="color:#64748b;font-size:12px">No audit events.</p>'; return; }
  host.innerHTML = `
    <table style="width:100%;border-collapse:collapse;font-size:11px">
      <thead><tr style="background:#1e293b;color:#94a3b8">
        <th style="padding:4px 6px;text-align:left">time</th>
        <th style="padding:4px 6px;text-align:left">event</th>
        <th style="padding:4px 6px;text-align:left">sev</th>
        <th style="padding:4px 6px;text-align:left">actor</th>
      </tr></thead>
      <tbody>
        ${items.map((i) => {
          const sevColor = ({
            critical: '#f87171', error: '#fb923c', warning: '#facc15', info: '#94a3b8',
          })[i.severity] || '#94a3b8';
          return `<tr style="border-bottom:1px solid #334155">
            <td style="padding:4px 6px;color:#cbd5e1;font-family:ui-monospace,monospace">${esc(i.occurred_at).slice(11, 19)}</td>
            <td style="padding:4px 6px;color:#f1f5f9">${esc(i.event_type)}</td>
            <td style="padding:4px 6px;color:${sevColor}">${esc(i.severity)}</td>
            <td style="padding:4px 6px;color:#94a3b8">${esc(i.actor)}</td>
          </tr>`;
        }).join('')}
      </tbody>
    </table>
  `;
}

async function renderWatchdog(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/watchdog/alerts?id=${AGENT_ID}&limit=10`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  const tickBtn = '<button class="ag-btn primary" id="mn-watchdog-tick" style="font-size:11px;padding:4px 10px;margin-bottom:8px">⚡ Manual sweep</button>';
  if (!items.length) {
    host.innerHTML = tickBtn + '<p style="color:#64748b;font-size:12px">No watchdog alerts.</p>';
  } else {
    host.innerHTML = tickBtn + items.map((i) => `
      <div style="background:#1e293b;border-left:3px solid #f87171;padding:6px;margin-bottom:4px">
        <div style="color:#f1f5f9;font-size:12px"><strong>${esc(i.rule_id)}</strong> · ${esc(i.fired_at)}</div>
        <pre style="color:#cbd5e1;font-size:10px;margin:4px 0 0 0;background:#020617;padding:4px;border-radius:4px;overflow-x:auto">${esc(i.context_json)}</pre>
      </div>
    `).join('');
  }
  const btn = document.getElementById('mn-watchdog-tick');
  if (btn) btn.onclick = async () => {
    btn.disabled = true; btn.textContent = '…';
    await postJSON('/api/agents/watchdog/tick');
    renderWatchdog(host);
  };
}

// =============================================================================
// Section 25 — Scanner
// =============================================================================

async function renderScanner(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const runs = await getJSON(`/api/agents/scanner/runs?id=${AGENT_ID}&limit=10`);
  if (runs.error) { host.innerHTML = `<p style="color:#f87171">${esc(runs.error)}</p>`; return; }
  const items = runs.items || [];
  host.innerHTML = `
    <div style="display:flex;gap:6px;margin-bottom:8px">
      <input id="mn-scan-target" placeholder="target_path (e.g. mr-flow/tools/file.go)" style="flex:1;padding:4px 8px;background:#1e293b;border:1px solid #475569;border-radius:4px;color:#f1f5f9;font-size:11px">
      <button class="ag-btn primary" id="mn-scan-run" style="font-size:11px;padding:4px 10px">Scan</button>
    </div>
    ${items.length ? `<table style="width:100%;border-collapse:collapse;font-size:11px">
      <thead><tr style="background:#1e293b;color:#94a3b8">
        <th style="padding:4px 6px;text-align:left">id</th>
        <th style="padding:4px 6px;text-align:left">target</th>
        <th style="padding:4px 6px;text-align:left">findings</th>
        <th style="padding:4px 6px;text-align:left">status</th>
      </tr></thead>
      <tbody>${items.map((i) => `<tr style="border-bottom:1px solid #334155">
        <td style="padding:4px 6px;color:#cbd5e1">${i.id}</td>
        <td style="padding:4px 6px;color:#f1f5f9;font-family:ui-monospace,monospace">${esc(i.target_path)}</td>
        <td style="padding:4px 6px;color:#f1f5f9">${i.total_findings} (${i.critical_count} crit)</td>
        <td style="padding:4px 6px;color:${i.status === 'pass' ? '#34d399' : '#f87171'}">${esc(i.status)}</td>
      </tr>`).join('')}</tbody>
    </table>` : '<p style="color:#64748b;font-size:12px">No scan runs yet.</p>'}
  `;
  document.getElementById('mn-scan-run').onclick = async () => {
    const tgt = document.getElementById('mn-scan-target').value.trim();
    if (!tgt) return;
    await postJSON(`/api/agents/scanner/scan?id=${AGENT_ID}`, { target_path: tgt, scan_type: 'manual' });
    renderScanner(host);
  };
}

// =============================================================================
// Section 24 — Protector rules
// =============================================================================

async function renderProtector(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/protector/rules?id=${AGENT_ID}&include_baseline=1`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  host.innerHTML = `
    <p style="color:#94a3b8;font-size:11px;margin-bottom:6px">${items.length} rules — hardcoded immutable + custom DB</p>
    <div style="max-height:300px;overflow-y:auto">
      ${items.map((i) => `
        <div style="background:${i.source === 'hardcoded' ? '#1e293b' : '#1e3a4d'};border:1px solid #334155;border-radius:4px;padding:5px 8px;margin-bottom:3px;display:flex;justify-content:space-between;gap:6px;align-items:center">
          <div style="flex:1;min-width:0">
            <div style="color:#f1f5f9;font-size:11px;font-family:ui-monospace,monospace">${esc(i.rule_type)} · ${esc(i.pattern)}</div>
            <div style="color:#94a3b8;font-size:10px">${esc(i.action)} · ${esc(i.source)}${i.immutable ? ' 🔒' : ''}</div>
          </div>
        </div>
      `).join('')}
    </div>
  `;
}

// =============================================================================
// Section 27+29 — Codemap + Zombie
// =============================================================================

async function renderCodemap(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/codemap/nodes?id=${AGENT_ID}&limit=20`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  host.innerHTML = `
    <p style="color:#94a3b8;font-size:11px;margin-bottom:6px">${items.length} nodes indexed</p>
    <table style="width:100%;font-size:11px;border-collapse:collapse">
      <thead><tr style="background:#1e293b;color:#94a3b8"><th style="padding:4px 6px;text-align:left">type</th><th style="padding:4px 6px;text-align:left">name</th><th style="padding:4px 6px;text-align:left">file:lines</th></tr></thead>
      <tbody>${items.slice(0, 15).map((i) => `<tr style="border-bottom:1px solid #334155">
        <td style="padding:4px 6px;color:#94a3b8">${esc(i.node_type)}</td>
        <td style="padding:4px 6px;color:#f1f5f9;font-family:ui-monospace,monospace">${esc(i.name)}</td>
        <td style="padding:4px 6px;color:#cbd5e1;font-size:10px">${esc(i.file_path)}:${i.line_start}-${i.line_end}</td>
      </tr>`).join('')}</tbody>
    </table>
  `;
}

async function renderZombie(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const data = await getJSON(`/api/agents/zombie/findings?id=${AGENT_ID}&limit=20`);
  if (data.error) { host.innerHTML = `<p style="color:#f87171">${esc(data.error)}</p>`; return; }
  const items = data.items || [];
  host.innerHTML = `
    <button class="ag-btn primary" id="mn-zombie-scan" style="font-size:11px;padding:4px 10px;margin-bottom:8px">🧟 Run scan</button>
    ${items.length ? items.map((i) => `
      <div style="background:#1e293b;border-left:3px solid ${i.acknowledged ? '#34d399' : '#facc15'};padding:6px;margin-bottom:3px;font-size:11px">
        <div style="color:#f1f5f9">${esc(i.symbol_type)} <strong>${esc(i.symbol_name)}</strong> in ${esc(i.file_path)}</div>
        <div style="color:#94a3b8;font-size:10px">${esc(i.reason)}</div>
        ${!i.acknowledged ? `<button class="ag-btn" data-ack="${i.id}" style="font-size:10px;padding:2px 6px;margin-top:3px">Acknowledge</button>` : ''}
      </div>
    `).join('') : '<p style="color:#64748b;font-size:12px">No zombie findings.</p>'}
  `;
  const scanBtn = document.getElementById('mn-zombie-scan');
  if (scanBtn) scanBtn.onclick = async () => {
    scanBtn.disabled = true; scanBtn.textContent = '…';
    await postJSON(`/api/agents/zombie/scan?id=${AGENT_ID}&min_age_days=0`);
    renderZombie(host);
  };
  host.querySelectorAll('[data-ack]').forEach((b) => b.onclick = async () => {
    await postJSON(`/api/agents/zombie/ack?id=${AGENT_ID}&finding_id=${b.dataset.ack}`);
    renderZombie(host);
  });
}

// =============================================================================
// Section 21+22 — Wallet + Alerts
// =============================================================================

async function renderWallet(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const [addrs, snaps, alerts] = await Promise.all([
    getJSON(`/api/agents/wallet/addresses?id=${AGENT_ID}`),
    getJSON(`/api/agents/wallet/snapshots?id=${AGENT_ID}&limit=5`),
    getJSON(`/api/agents/wallet/alerts?id=${AGENT_ID}`),
  ]);
  host.innerHTML = `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;font-size:11px">
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Addresses (${addrs.count || 0})</h5>
        ${(addrs.items || []).map((a) => `<div style="color:#cbd5e1;font-family:ui-monospace,monospace;font-size:10px;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">${esc(a.address).slice(0, 16)}… (chain ${a.chain_id})</div>`).join('') || '<p style="color:#64748b">none</p>'}
      </div>
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Alerts (${alerts.count || 0})</h5>
        ${(alerts.items || []).map((a) => `<div style="color:#cbd5e1;font-size:10px;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">${esc(a.metric_key)} ${esc(a.comparator)} ${a.threshold_value} → ${esc(a.notify_channel)}</div>`).join('') || '<p style="color:#64748b">none</p>'}
      </div>
    </div>
    <h5 style="color:#94a3b8;margin:10px 0 4px 0;font-size:11px;font-weight:normal">Recent snapshots (${snaps.count || 0})</h5>
    ${(snaps.items || []).slice(0, 3).map((s) => `<div style="color:#cbd5e1;font-size:10px;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">$${s.total_usd?.toFixed(2)} · ${esc(s.taken_at)}</div>`).join('') || '<p style="color:#64748b;font-size:11px">no snapshots yet</p>'}
  `;
}

// =============================================================================
// Section 23 — Finance
// =============================================================================

async function renderFinance(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const [summary, budgets] = await Promise.all([
    getJSON(`/api/agents/finance/summary?id=${AGENT_ID}`),
    getJSON(`/api/agents/finance/budget?id=${AGENT_ID}`),
  ]);
  host.innerHTML = `
    <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Today's spend: <span style="color:#f1f5f9">$${(summary.total_usd || 0).toFixed(4)}</span></h5>
    ${(summary.by_category || []).map((s) => `<div style="font-size:11px;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px;color:#cbd5e1">${esc(s.category)}: $${s.cost_usd.toFixed(4)} (${s.call_count} call)</div>`).join('') || '<p style="color:#64748b;font-size:11px">no spend today</p>'}
    <h5 style="color:#94a3b8;margin:10px 0 4px 0;font-size:11px;font-weight:normal">Budgets (${budgets.count || 0})</h5>
    ${(budgets.items || []).map((b) => `<div style="font-size:11px;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px;color:#cbd5e1">${esc(b.metric_key)}: $${b.budget_value} (warn ${(b.warning_at_pct * 100).toFixed(0)}%)</div>`).join('') || '<p style="color:#64748b;font-size:11px">no budgets</p>'}
  `;
}

// =============================================================================
// Section 35 — Self-prompt slot editor
// =============================================================================

async function renderSelfPrompt(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading…</p>';
  const slots = await getJSON(`/api/agents/self-prompt?id=${AGENT_ID}`);
  if (slots.error) { host.innerHTML = `<p style="color:#f87171">${esc(slots.error)}</p>`; return; }
  const list = slots.slots || [];
  host.innerHTML = `
    <div style="margin-bottom:8px">
      <select id="mn-prompt-slot" style="padding:4px 8px;background:#1e293b;border:1px solid #475569;border-radius:4px;color:#f1f5f9;font-size:11px">
        ${['system', 'persona', 'guideline', 'task'].map((s) => `<option value="${s}">${s}</option>`).join('')}
      </select>
      <button class="ag-btn primary" id="mn-prompt-save" style="font-size:11px;padding:4px 10px">Save</button>
      <button class="ag-btn" id="mn-prompt-preview" style="font-size:11px;padding:4px 10px">📄 Render preview</button>
    </div>
    <textarea id="mn-prompt-body" placeholder="Markdown body…" style="width:100%;height:100px;padding:8px;background:#020617;color:#cbd5e1;border:1px solid #334155;border-radius:4px;font-family:ui-monospace,monospace;font-size:11px;resize:vertical"></textarea>
    <div id="mn-prompt-status" style="margin-top:4px;font-size:11px;color:#94a3b8"></div>
    <h5 style="color:#94a3b8;margin:10px 0 4px 0;font-size:11px;font-weight:normal">Active slots (${list.length})</h5>
    ${list.map((s) => `<div style="font-size:11px;color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px"><strong>${esc(s.slot)}</strong> v${s.version} (${s.body.length} bytes)</div>`).join('')}
    <pre id="mn-prompt-preview-out" style="display:none;background:#020617;color:#cbd5e1;padding:6px;border-radius:4px;font-size:10px;margin-top:8px;max-height:200px;overflow-y:auto;white-space:pre-wrap"></pre>
  `;
  document.getElementById('mn-prompt-save').onclick = async () => {
    const slot = document.getElementById('mn-prompt-slot').value;
    const body = document.getElementById('mn-prompt-body').value;
    const status = document.getElementById('mn-prompt-status');
    if (!body.trim()) { status.textContent = 'body empty'; status.style.color = '#f87171'; return; }
    const res = await postJSON(`/api/agents/self-prompt?id=${AGENT_ID}`, { slot, body });
    if (res.error) { status.textContent = res.error; status.style.color = '#f87171'; return; }
    status.textContent = `✓ saved id=${res.id}`;
    status.style.color = '#34d399';
    renderSelfPrompt(host);
  };
  document.getElementById('mn-prompt-preview').onclick = async () => {
    const data = await getJSON(`/api/agents/self-prompt/render?id=${AGENT_ID}`);
    const out = document.getElementById('mn-prompt-preview-out');
    out.style.display = 'block';
    out.textContent = data.rendered || data.error || '(empty)';
  };
}

// =============================================================================
// Router admin
// =============================================================================

async function renderRouterAdmin(host) {
  host.innerHTML = '<p style="color:#94a3b8;font-size:12px">Loading Router…</p>';
  const [chains, calls, pricing, budgets, violations, peers] = await Promise.all([
    fetch('http://127.0.0.1:2402/api/provider/chains').then((r) => r.json()).catch(() => ({ error: 'unreachable' })),
    fetch('http://127.0.0.1:2402/api/provider/calls?limit=5').then((r) => r.json()).catch(() => ({})),
    fetch('http://127.0.0.1:2402/api/pricing/rules').then((r) => r.json()).catch(() => ({})),
    fetch('http://127.0.0.1:2402/api/policy/budgets').then((r) => r.json()).catch(() => ({})),
    fetch('http://127.0.0.1:2402/api/policy/violations?limit=5').then((r) => r.json()).catch(() => ({})),
    fetch('http://127.0.0.1:2402/api/mesh/peers').then((r) => r.json()).catch(() => ({})),
  ]);
  if (chains.error) {
    host.innerHTML = `<p style="color:#f87171;font-size:12px">Router unreachable: ${esc(chains.error)}</p>`;
    return;
  }
  host.innerHTML = `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;font-size:11px">
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Chains (${chains.count || 0})</h5>
        ${(chains.items || []).map((c) => `<div style="color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px"><strong>${esc(c.chain_name)}</strong>: ${esc(c.providers_json)}</div>`).join('')}
      </div>
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Pricing rules (${pricing.count || 0})</h5>
        ${(pricing.items || []).slice(0, 5).map((p) => `<div style="color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">${esc(p.provider)}/${esc(p.model)}: $${p.input_per_1m_usd}/$${p.output_per_1m_usd} per 1M</div>`).join('')}
      </div>
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Policy budgets (${budgets.count || 0})</h5>
        ${(budgets.items || []).map((b) => `<div style="color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">${esc(b.scope)}/${esc(b.scope_key)}: ${esc(b.metric_key)} ≤$${b.budget_value}</div>`).join('')}
      </div>
      <div>
        <h5 style="color:#94a3b8;margin:0 0 4px 0;font-size:11px;font-weight:normal">Violations (${violations.count || 0})</h5>
        ${(violations.items || []).map((v) => `<div style="color:#f87171;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px">budget #${v.budget_id}: $${v.actual_value} ${esc(v.action_taken)}</div>`).join('') || '<p style="color:#64748b">none</p>'}
      </div>
    </div>
    <h5 style="color:#94a3b8;margin:10px 0 4px 0;font-size:11px;font-weight:normal">Mesh peers (${peers.count || 0})</h5>
    ${(peers.peers || []).map((p) => `<div style="color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px;font-size:11px"><strong>${esc(p.pubkey_hex).slice(0, 16)}…</strong> @ ${esc(p.ip)}:${p.port} · trust=${p.trust_score.toFixed(2)}</div>`).join('') || '<p style="color:#64748b;font-size:11px">no peers</p>'}
    <h5 style="color:#94a3b8;margin:10px 0 4px 0;font-size:11px;font-weight:normal">Recent calls (${(calls.items || []).length})</h5>
    ${(calls.items || []).slice(0, 5).map((c) => `<div style="font-size:10px;color:#cbd5e1;background:#1e293b;padding:3px 6px;border-radius:3px;margin-bottom:2px;font-family:ui-monospace,monospace">${esc(c.provider)}/${esc(c.model)}: $${c.cost_usd} · ${c.latency_ms}ms · ${esc(c.status)}</div>`).join('') || '<p style="color:#64748b;font-size:11px">no calls yet</p>'}
  `;
}

// =============================================================================
// Main render
// =============================================================================

export async function render(root) {
  root.innerHTML = `
    <div style="padding:16px;max-width:1200px;margin:0 auto">
      <h2 style="color:#f1f5f9;margin:0 0 16px 0">📊 ${esc(t('menu.tab.monitor.title') || 'Monitor — Operations Dashboard')}</h2>
      <p style="color:#94a3b8;font-size:12px;margin:0 0 16px 0">
        Real-time view dari semua fitur baru. Refresh button per panel atau auto-poll 10s.
      </p>
      ${panel('approval', 'menu.tab.monitor.approval', '🛡️', '<div id="mn-approval-body"></div>')}
      ${panel('watchdog', 'menu.tab.monitor.watchdog', '🚨', '<div id="mn-watchdog-body"></div>')}
      ${panel('audit', 'menu.tab.monitor.audit', '📋', '<div id="mn-audit-body"></div>')}
      ${panel('scanner', 'menu.tab.monitor.scanner', '🔍', '<div id="mn-scanner-body"></div>')}
      ${panel('protector', 'menu.tab.monitor.protector', '⚔️', '<div id="mn-protector-body"></div>')}
      ${panel('codemap', 'menu.tab.monitor.codemap', '🗺️', '<div id="mn-codemap-body"></div>')}
      ${panel('zombie', 'menu.tab.monitor.zombie', '🧟', '<div id="mn-zombie-body"></div>')}
      ${panel('wallet', 'menu.tab.monitor.wallet', '👛', '<div id="mn-wallet-body"></div>')}
      ${panel('finance', 'menu.tab.monitor.finance', '💰', '<div id="mn-finance-body"></div>')}
      ${panel('prompt', 'menu.tab.monitor.prompt', '✍️', '<div id="mn-prompt-body"></div>')}
      ${panel('router', 'menu.tab.monitor.router', '🛣️', '<div id="mn-router-body"></div>')}
    </div>
  `;
  // Lazy-load on first expand per panel.
  const renderers = {
    'approval': renderApproval,
    'watchdog': renderWatchdog,
    'audit': renderAudit,
    'scanner': renderScanner,
    'protector': renderProtector,
    'codemap': renderCodemap,
    'zombie': renderZombie,
    'wallet': renderWallet,
    'finance': renderFinance,
    'prompt': renderSelfPrompt,
    'router': renderRouterAdmin,
  };
  Object.entries(renderers).forEach(([key, fn]) => {
    const det = root.querySelector(`#mn-${key}`);
    const body = root.querySelector(`#mn-${key}-body`);
    if (!det || !body) return;
    let loaded = false;
    det.addEventListener('toggle', () => {
      if (det.open && !loaded) {
        loaded = true;
        fn(body);
      }
    });
  });
}
