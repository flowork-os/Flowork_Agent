// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-05-30 (RESTORED 2026-06-11 — owner-requested)
// Reason: Codemap tab (reference 966 LOC). Audit pass — esc() on node name+file_path+layer, d3 vendor for graph..
// 2026-06-11: the sidebar tab was dropped in commit 14db62b ("remove legacy
//   Finance/Protector/Codemap tabs") but the backend codemap engine + all
//   /api/codemap/* endpoints stayed wired. Owner asked for the feature back
//   (file dependency graph + zombie-code detection). Restored verbatim from the
//   last living version (commit 11d4747), re-registered 'codemap' i18n domain,
//   re-added the sidebar button. No code change beyond this header note.

/**
 * Code Map — D3.js force-directed dependency graph.
 * Style referensi: codeflow (braedonsaunders/codeflow)
 *
 * Color modes: Health | Folder | Blast (impact radius dari selected node)
 * Interaksi: klik → highlight deps, double-click → blast mode, zoom/pan, drag node
 */

import { t } from '/js/i18n.js';
const L = new Proxy({}, { get: (_, k) => t('codemap.' + String(k).replace(/[A-Z]/g, (c) => '_' + c.toLowerCase())) });

export async function render(container) {
  container.innerHTML = skeleton();
  applyStyles();

  // ── Load D3: lokal /vendor/ first, fallback graceful list view ──────────
  // Per Ayah doctrine API-FREE + sovereignty: ngga boleh tergantung CDN
  // external. Kalau /vendor/d3.min.js ngga ada, render fallback table list
  // (zombie + roots) — tab tetap functional, no graph viz.
  let d3;
  try {
    d3 = await loadD3();
  } catch (_) {
    await renderFallbackList(container);
    return;
  }

  // ── State ──────────────────────────────────────────────────────────────
  const S = {
    nodes: [], edges: [],
    semantic: {},          // R6: path → {summary,domain,role} (lapisan makna self-map)
    selected: null,
    colorMode: 'health',  // 'health' | 'folder' | 'blast'
    blastSet: new Map(),   // path → degree (1,2,3...)
    filter: '',
    simulation: null,
    status: {},
    folderPalette: {},
  };

  // ── DOM refs ───────────────────────────────────────────────────────────
  const svgEl     = container.querySelector('#cm-svg');
  const detailEl  = container.querySelector('#cm-detail');
  const zombieEl  = container.querySelector('#cm-zombie-panel');
  const searchIn  = container.querySelector('#cm-search');
  const statusTxt = container.querySelector('#cm-status');
  const reindexBtn = container.querySelector('#cm-reindex');
  const enrichBtn  = container.querySelector('#cm-enrich');
  const colorBtns  = container.querySelectorAll('.cm-color-btn');
  const zombieBtn  = container.querySelector('#cm-zombie-btn');

  // ── SVG setup ──────────────────────────────────────────────────────────
  const svg = d3.select(svgEl);
  const defs = svg.append('defs');

  // Arrow marker
  defs.append('marker')
    .attr('id', 'cm-arrow')
    .attr('viewBox', '0 -4 8 8')
    .attr('refX', 12).attr('refY', 0)
    .attr('markerWidth', 6).attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path').attr('d', 'M0,-4L8,0L0,4').attr('fill', 'rgba(100,116,139,0.5)');

  defs.append('marker')
    .attr('id', 'cm-arrow-hl')
    .attr('viewBox', '0 -4 8 8')
    .attr('refX', 12).attr('refY', 0)
    .attr('markerWidth', 6).attr('markerHeight', 6)
    .attr('orient', 'auto')
    .append('path').attr('d', 'M0,-4L8,0L0,4').attr('fill', '#a78bfa');

  // Zoom layer
  const zoomG = svg.append('g').attr('class', 'cm-zoom-g');
  const linkG = zoomG.append('g').attr('class', 'cm-link-g');
  const nodeG = zoomG.append('g').attr('class', 'cm-node-g');

  const zoom = d3.zoom().scaleExtent([0.05, 6])
    .on('zoom', (e) => zoomG.attr('transform', e.transform));
  svg.call(zoom).on('dblclick.zoom', null);

  // ── Data loading ───────────────────────────────────────────────────────

  async function loadGraph() {
    setDetail('<div class="cm-loading">⏳ Loading graph...</div>');
    try {
      const r = await fetch('/api/codemap/graph');
      if (!r.ok) throw new Error(await r.text());
      const data = await r.json();
      S.nodes = data.nodes || [];
      S.edges = data.edges || [];
      await loadSemantic();
      buildFolderPalette();
      initSimulation();
      setDetail(emptyDetail());
    } catch (e) {
      setDetail(`<div class="cm-err">❌ ${e.message}<br><small>${L.clickReindex}</small></div>`);
    }
  }

  // R6: muat lapisan makna (path → {summary,domain,role}). Fail-open (kosong = ok).
  async function loadSemantic() {
    try {
      const r = await fetch('/api/codemap/semantic');
      if (!r.ok) return;
      const d = await r.json();
      S.semantic = {};
      (d.items || []).forEach(m => { if (m.path) S.semantic[m.path] = m; });
    } catch (_) { /* fail-open: struktur tetap jalan tanpa makna */ }
  }

  // esc — escape HTML (XSS-safe) buat teks makna dari LLM.
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c =>
      ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]));
  }

  async function loadStatus() {
    try {
      const r = await fetch('/api/codemap/status');
      if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => r.statusText)}`);
      const d = await r.json();
      S.status = d;
      const p = [];
      if (d.running) p.push('⏳ indexing...');
      if (d.node_count) p.push(`${d.node_count} files`);
      if (d.edge_count) p.push(`${d.edge_count} edges`);
      statusTxt.textContent = p.join(' · ');
    } catch (_) {}
  }

  // ── Folder palette ─────────────────────────────────────────────────────

  const FOLDER_COLORS = [
    '#818cf8','#34d399','#f59e0b','#f87171','#38bdf8',
    '#a78bfa','#4ade80','#fbbf24','#fb7185','#22d3ee',
    '#c084fc','#86efac','#fcd34d','#fda4af','#67e8f9',
  ];

  function buildFolderPalette() {
    const dirs = new Set();
    S.nodes.forEach(n => {
      const parts = n.path.split('/');
      dirs.add(parts.length > 1 ? parts[0] : '__root__');
    });
    let i = 0;
    dirs.forEach(d => { S.folderPalette[d] = FOLDER_COLORS[i++ % FOLDER_COLORS.length]; });
  }

  function topDir(path) {
    const parts = path.split('/');
    return parts.length > 1 ? parts[0] : '__root__';
  }

  // ── Color logic ────────────────────────────────────────────────────────

  // 2026-05-06 (adopt Understand-Anything fitur 1): Layer color palette
  // permanen — UI / API / Data / Service / Util / Test / Core. Stable
  // mapping biar visual konsisten antar render.
  const LAYER_COLORS = {
    UI:      '#06b6d4',  // cyan
    API:     '#8b5cf6',  // violet
    Data:    '#10b981',  // emerald
    Service: '#f59e0b',  // amber
    Util:    '#94a3b8',  // slate
    Test:    '#64748b',  // gray
    Core:    '#e879f9',  // fuchsia (default fallback)
  };

  function nodeColor(n) {
    // 2026-05-06 (fitur 3 diff overlay): file di-touch commit terakhir
    // dapat outline highlight. Di-handle via stroke (separate function),
    // fill tetap by mode.
    if (S.colorMode === 'layer') {
      return LAYER_COLORS[n.layer || 'Core'] || LAYER_COLORS.Core;
    }
    if (S.colorMode === 'folder') {
      return S.folderPalette[topDir(n.path)] || '#94a3b8';
    }
    if (S.colorMode === 'blast') {
      if (!S.selected) return '#334155';
      if (n.path === S.selected.path) return '#a78bfa';
      const deg = S.blastSet.get(n.path);
      if (deg === 1) return '#ef4444';
      if (deg === 2) return '#f97316';
      if (deg === 3) return '#eab308';
      if (deg)       return '#84cc16';
      return '#1e293b';
    }
    // health mode (default)
    const h = n.health_score ?? 100;
    if (h >= 80) return '#22c55e';
    if (h >= 60) return '#eab308';
    if (h >= 40) return '#f97316';
    return '#ef4444';
  }

  // 2026-05-06 (fitur 3): stroke color buat diff overlay. Recently-touched
  // file dapat ring kuning tebal supaya kelihatan langsung di graph.
  function nodeStroke(n) {
    if (n.recently_touched) return '#fbbf24'; // amber-400
    if (S.selected && n.path === S.selected.path) return '#a78bfa';
    return '#0f172a';
  }
  function nodeStrokeWidth(n) {
    if (n.recently_touched) return 3;
    if (S.selected && n.path === S.selected.path) return 2.5;
    return 1;
  }

  function nodeOpacity(n) {
    if (S.filter) {
      const f = S.filter.toLowerCase();
      return (n.path.toLowerCase().includes(f) || n.name.toLowerCase().includes(f)) ? 1 : 0.1;
    }
    if (S.selected && S.colorMode !== 'blast') {
      if (n.path === S.selected.path) return 1;
      const connected = S.edges.some(e =>
        (e.from === S.selected.path && e.to === n.path) ||
        (e.to === S.selected.path && e.from === n.path)
      );
      return connected ? 1 : 0.18;
    }
    return 1;
  }

  function linkOpacity(e) {
    if (S.filter) return 0.05;
    if (S.selected) {
      return (e.from === S.selected.path || e.to === S.selected.path) ? 0.8 : 0.04;
    }
    return 0.25;
  }

  function linkColor(e) {
    if (!S.selected) return 'rgba(100,116,139,0.3)';
    if (e.from === S.selected.path) return 'rgba(139,92,246,0.7)';
    if (e.to   === S.selected.path) return 'rgba(34,197,94,0.6)';
    return 'rgba(100,116,139,0.06)';
  }

  function nodeRadius(n) {
    return Math.max(6, Math.min(20, 6 + (n.dependent_count || 0) * 0.8));
  }

  // ── D3 Force Simulation ────────────────────────────────────────────────

  let linkSel, nodeSel, circleSel, labelSel;

  function initSimulation() {
    const nodeMap = new Map(S.nodes.map(n => [n.path, n]));
    const links = S.edges
      .filter(e => nodeMap.has(e.from) && nodeMap.has(e.to))
      .map(e => ({ source: e.from, target: e.to, from: e.from, to: e.to }));

    // Place nodes if no position yet
    const W = svgEl.clientWidth  || 800;
    const H = svgEl.clientHeight || 600;
    S.nodes.forEach(n => {
      if (n.x == null) {
        n.x = W / 2 + (Math.random() - 0.5) * 400;
        n.y = H / 2 + (Math.random() - 0.5) * 400;
      }
    });

    if (S.simulation) S.simulation.stop();

    // 2026-05-06 (Ayah audit "tumpang tindih"): force tuning untuk codebase
    // 1000+ node. Sebelumnya charge -120 + collide r+6 → cluster dense
    // numpuk, label ngga kebaca. Naikin repulsion + collide buffer + link
    // distance proporsional ke node count biar bola-bola dapet ruang nafas.
    const N = S.nodes.length;
    const chargeStrength = N > 800 ? -350 : (N > 400 ? -240 : -160);
    const linkDistance = N > 800 ? 140 : (N > 400 ? 110 : 90);
    const collideBuffer = N > 800 ? 14 : (N > 400 ? 10 : 7);
    S.simulation = d3.forceSimulation(S.nodes)
      .force('link', d3.forceLink(links).id(d => d.path).distance(linkDistance).strength(0.25))
      .force('charge', d3.forceManyBody().strength(chargeStrength).distanceMax(450))
      .force('collision', d3.forceCollide(d => nodeRadius(d) + collideBuffer).strength(0.9))
      .force('center', d3.forceCenter(W / 2, H / 2).strength(0.05))
      .force('x', d3.forceX(W / 2).strength(0.02))
      .force('y', d3.forceY(H / 2).strength(0.02))
      .alphaDecay(0.018);

    // Draw links
    linkSel = linkG.selectAll('line')
      .data(links, d => d.from + '→' + d.to)
      .join('line')
      .attr('stroke-width', 1)
      .attr('marker-end', 'url(#cm-arrow)');

    // Draw nodes
    nodeSel = nodeG.selectAll('g.cm-n')
      .data(S.nodes, d => d.path)
      .join('g')
      .attr('class', 'cm-n')
      .style('cursor', 'pointer')
      .call(d3.drag()
        .on('start', (e, d) => { if (!e.active) S.simulation.alphaTarget(0.1).restart(); d.fx = d.x; d.fy = d.y; })
        .on('drag',  (e, d) => { d.fx = e.x; d.fy = e.y; })
        .on('end',   (e, d) => { if (!e.active) S.simulation.alphaTarget(0); d.fx = null; d.fy = null; }))
      .on('click', (e, d) => { e.stopPropagation(); selectNode(d); })
      .on('dblclick', (e, d) => { e.stopPropagation(); blastMode(d); });

    circleSel = nodeSel.append('circle')
      .attr('r', d => nodeRadius(d))
      .attr('stroke', d => nodeStroke(d))
      .attr('stroke-width', d => nodeStrokeWidth(d));

    labelSel = nodeSel.append('text')
      .attr('text-anchor', 'middle')
      .attr('font-size', '8.5px')
      .attr('fill', '#94a3b8')
      .attr('pointer-events', 'none');

    // Click background → deselect
    svg.on('click', () => { S.selected = null; S.blastSet.clear(); applyColors(); setDetail(emptyDetail()); });

    S.simulation.on('tick', tick);
    applyColors();
  }

  function tick() {
    linkSel
      .attr('x1', d => d.source.x).attr('y1', d => d.source.y)
      .attr('x2', d => d.target.x).attr('y2', d => d.target.y)
      .attr('stroke', d => linkColor(d))
      .attr('stroke-opacity', d => linkOpacity(d));

    nodeSel.attr('transform', d => `translate(${d.x ?? 0},${d.y ?? 0})`);

    circleSel.attr('r', d => nodeRadius(d));
    labelSel
      .attr('dy', d => nodeRadius(d) + 11)
      .text(d => {
        const n = d.name;
        if (d.x == null) return '';
        return n.length > 13 ? n.slice(0, 12) + '…' : n;
      });
  }

  function applyColors() {
    if (!circleSel) return;
    circleSel
      .attr('fill', d => nodeColor(d))
      .attr('stroke', d => S.selected?.path === d.path ? '#fff' : 'rgba(255,255,255,0.12)')
      .attr('fill-opacity', d => nodeOpacity(d))
      .attr('stroke-opacity', d => nodeOpacity(d));
    labelSel.attr('fill-opacity', d => nodeOpacity(d));
    linkSel
      ?.attr('stroke', d => linkColor(d))
      .attr('stroke-opacity', d => linkOpacity(d))
      .attr('marker-end', d => {
        if (S.selected && (d.from === S.selected.path || d.to === S.selected.path))
          return 'url(#cm-arrow-hl)';
        return 'url(#cm-arrow)';
      });
  }

  // ── Select node ────────────────────────────────────────────────────────

  async function selectNode(n) {
    S.selected = n;
    S.colorMode = S.colorMode === 'blast' ? 'blast' : S.colorMode;
    if (S.colorMode === 'blast') buildBlastSet(n);
    applyColors();
    renderDetail(n);
    // Bump sim a bit so highlights are clear
    S.simulation?.alphaTarget(0.02).restart();
    setTimeout(() => S.simulation?.alphaTarget(0), 500);
  }

  // ── Blast mode ─────────────────────────────────────────────────────────

  function buildBlastSet(n) {
    S.blastSet.clear();
    const queue = [n.path];
    const adjRev = new Map(); // path → who imports it
    S.edges.forEach(e => {
      if (!adjRev.has(e.to)) adjRev.set(e.to, []);
      adjRev.get(e.to).push(e.from);
    });
    for (let deg = 1; deg <= 5 && queue.length; deg++) {
      const next = [];
      queue.forEach(p => {
        (adjRev.get(p) || []).forEach(dep => {
          if (!S.blastSet.has(dep) && dep !== n.path) {
            S.blastSet.set(dep, deg);
            next.push(dep);
          }
        });
      });
      queue.length = 0;
      queue.push(...next);
    }
  }

  function blastMode(n) {
    S.colorMode = 'blast';
    S.selected  = n;
    buildBlastSet(n);
    colorBtns.forEach(b => b.classList.toggle('active', b.dataset.mode === 'blast'));
    applyColors();
    renderDetail(n);
  }

  // ── Detail panel ────────────────────────────────────────────────────────

  function renderDetail(n) {
    const hc = n.health_score >= 80 ? '#22c55e' : n.health_score >= 60 ? '#eab308' : n.health_score >= 40 ? '#f97316' : '#ef4444';
    const grade = n.health_score >= 90 ? 'A' : n.health_score >= 75 ? 'B' : n.health_score >= 60 ? 'C' : n.health_score >= 40 ? 'D' : 'F';
    const gradeColor = { A:'#22c55e', B:'#4ade80', C:'#eab308', D:'#f97316', F:'#ef4444' }[grade];

    // Go meng-import LEVEL-PAKET, bukan file. Jadi:
    //  • Imports (file ini "manggil")        = paket yang di-import file ini (outgoing, akurat per-file).
    //  • Dipakai oleh (file ini "dipanggil")  = SEMUA file dari paket LAIN yang meng-import PAKET file
    //    ini. Tanpa pendekatan paket, cuma 1 "file perwakilan" tiap paket yang punya incoming → file
    //    seperti settings.go nampak "dipakai 0×" padahal paketnya dipakai di mana-mana.
    const dirOf = (p) => { const i = p.lastIndexOf('/'); return i < 0 ? '' : p.slice(0, i); };
    const myDir = dirOf(n.path);
    const dirDeps = [...new Set(S.edges.filter(e => e.from === n.path).map(e => e.to))];
    const dirRevs = [...new Set(
      S.edges.filter(e => dirOf(e.to) === myDir && dirOf(e.from) !== myDir).map(e => e.from)
    )];
    const blastCount = S.blastSet.size;

    const issueLi = (n.issues || []).length
      ? (n.issues || []).map(i => `<li>⚠️ ${i}</li>`).join('')
      : '<li style="color:#4ade80">✅ no issues</li>';

    const pillDeps = dirDeps.slice(0, 8).map(p =>
      `<span class="cm-pill dep" data-path="${p}">${p.split('/').pop()}</span>`).join('');
    const pillRevs = dirRevs.slice(0, 8).map(p =>
      `<span class="cm-pill rev" data-path="${p}">${p.split('/').pop()}</span>`).join('');

    setDetail(`
      <div class="cm-d-top">
        <span class="cm-d-grade" style="color:${gradeColor};border-color:${gradeColor}">${grade}</span>
        <div>
          <div class="cm-d-name">${n.name}</div>
          <div class="cm-d-path">${n.path}</div>
        </div>
      </div>

      ${(() => {
        const sm = S.semantic[n.path];
        return sm && sm.summary ? `
      <div class="cm-d-sec" style="background:#1e293b;border-radius:6px;padding:8px;margin:6px 0">
        <div class="cm-d-lbl">🧠 Makna <span style="opacity:.5;font-weight:normal">(self-map R6)</span></div>
        <div style="font-size:0.82rem;line-height:1.45">${esc(sm.summary)}</div>
        <div style="margin-top:6px">
          ${sm.domain ? `<span class="cm-chip">domain: ${esc(sm.domain)}</span>` : ''}
          ${sm.role ? `<span class="cm-chip">role: ${esc(sm.role)}</span>` : ''}
        </div>
      </div>` : '';
      })()}

      <div class="cm-d-hbar-wrap">
        <div class="cm-d-hbar"><div style="width:${n.health_score}%;background:${hc}"></div></div>
        <span style="color:${hc}">${(n.health_score||0).toFixed(0)}/100</span>
      </div>

      <div class="cm-d-chips">
        <span class="cm-chip">${n.file_type?.toUpperCase() || '?'}</span>
        <span class="cm-chip">${n.line_count} baris</span>
        ${n.has_tests ? '<span class="cm-chip ok">✅ test</span>' : '<span class="cm-chip bad">❌ test</span>'}
        ${n.has_docs  ? '<span class="cm-chip ok">✅ docs</span>' : '<span class="cm-chip bad">❌ docs</span>'}
      </div>

      <div class="cm-d-sec">
        <div class="cm-d-lbl">Issues</div>
        <ul class="cm-d-ul">${issueLi}</ul>
      </div>

      <div class="cm-d-sec">
        <div class="cm-d-lbl">📥 Imports (${dirDeps.length})</div>
        <div>${pillDeps || '<em>—</em>'}${dirDeps.length > 8 ? `<span class="cm-more">+${dirDeps.length-8}</span>` : ''}</div>
      </div>

      <div class="cm-d-sec">
        <div class="cm-d-lbl">📤 Dipakai oleh (${dirRevs.length})</div>
        <div>${pillRevs || '<em>—</em>'}${dirRevs.length > 8 ? `<span class="cm-more">+${dirRevs.length-8}</span>` : ''}</div>
      </div>

      <div class="cm-d-sec">
        <div class="cm-d-lbl">💥 Blast radius</div>
        <div style="font-size:0.8rem">${blastCount > 0 ? `<span style="color:#f87171">${blastCount} files affected</span> — <button class="cm-sm-btn" id="blast-trigger" title="${L.tipBlast}">visualize</button>` : '<em>—</em>'}</div>
      </div>

      <div class="cm-d-sec" style="margin-top:8px">
        <button class="cm-action-btn" id="cm-open-docs" title="${L.tipAutodocs}">📄 Auto-Docs</button>
        <button class="cm-action-btn" id="cm-center-node" title="${L.tipCenter}">🎯 Center</button>
      </div>

      <div id="cm-docs-content" style="display:none;margin-top:12px"></div>
    `);

    // Pill clicks → select that node
    detailEl.querySelectorAll('[data-path]').forEach(el => {
      el.addEventListener('click', () => {
        const node = S.nodes.find(x => x.path === el.dataset.path);
        if (node) selectAndCenter(node);
      });
    });

    detailEl.querySelector('#blast-trigger')?.addEventListener('click', () => blastMode(n));

    detailEl.querySelector('#cm-open-docs')?.addEventListener('click', async () => {
      const dc = detailEl.querySelector('#cm-docs-content');
      if (dc.style.display !== 'none') { dc.style.display = 'none'; return; }
      dc.style.display = '';
      dc.innerHTML = '⏳ loading...';
      try {
        const r = await fetch(`/api/codemap/docs?path=${encodeURIComponent(n.path)}`);
        const txt = await r.text();
        dc.innerHTML = `<div class="cm-md">${mdToHTML(txt)}</div>`;
      } catch (e) {
        dc.innerHTML = `<span style="color:#f87171">${e.message}</span>`;
      }
    });

    detailEl.querySelector('#cm-center-node')?.addEventListener('click', () => {
      centerOnNode(n);
    });
  }

  function selectAndCenter(node) {
    selectNode(node);
    setTimeout(() => centerOnNode(node), 100);
  }

  function centerOnNode(n) {
    const W = svgEl.clientWidth, H = svgEl.clientHeight;
    const t = d3.zoomTransform(svgEl);
    svg.transition().duration(500).call(
      zoom.transform,
      d3.zoomIdentity.translate(W / 2 - t.k * n.x, H / 2 - t.k * n.y).scale(t.k)
    );
  }

  function emptyDetail() {
    return `<div class="cm-empty-d">
      <div style="font-size:2.5rem;margin-bottom:8px">🗺️</div>
      <div>${L.clickNode}</div>
      <div style="margin-top:4px;font-size:0.7rem;opacity:0.5">Double-click → blast radius mode</div>
    </div>`;
  }

  function setDetail(html) { detailEl.innerHTML = html; }

  // ── Color mode buttons ─────────────────────────────────────────────────

  colorBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      S.colorMode = btn.dataset.mode;
      colorBtns.forEach(b => b.classList.toggle('active', b.dataset.mode === S.colorMode));
      if (S.colorMode === 'blast' && S.selected) buildBlastSet(S.selected);
      applyColors();
    });
  });

  // ── Search ─────────────────────────────────────────────────────────────

  searchIn.addEventListener('input', () => {
    S.filter = searchIn.value.trim();
    applyColors();
  });

  // ── Zombie panel ───────────────────────────────────────────────────────

  zombieBtn.addEventListener('click', () => {
    const open = zombieEl.style.display !== 'none';
    zombieEl.style.display = open ? 'none' : '';
    zombieBtn.classList.toggle('active', !open);
    if (!open) loadZombies();
  });

  async function loadZombies() {
    zombieEl.innerHTML = '<div class="cm-loading">⏳ scanning...</div>';
    try {
      const r = await fetch('/api/codemap/zombies');
      if (!r.ok) throw new Error(`${r.status}: ${await r.text().catch(() => r.statusText)}`);
      const d = await r.json();
      if (!d.count) {
        zombieEl.innerHTML = `<div class="cm-z-ok">${L.noZombie}</div>`;
        return;
      }
      zombieEl.innerHTML = `
        <div class="cm-z-title">🧟 ${d.count} ${L.zombiePanel}</div>
        ${d.advisory ? `<div style="background:#3b2410;border:1px solid #b45309;border-radius:6px;padding:7px 9px;margin:6px 0;font-size:0.72rem;color:#fbbf24">⚠️ ${d.note || 'Heuristik lemah — KANDIDAT review manual, JANGAN auto-delete.'}</div>` : ''}
        <table class="cm-z-table">
          <thead><tr><th>File</th><th>Type</th><th>Lines</th><th></th></tr></thead>
          <tbody>${(d.zombies||[]).map(z => `
            <tr>
              <td title="${z.path}">${z.name}<br><span style="color:#475569;font-size:0.65rem">${z.path}</span></td>
              <td>${z.file_type.toUpperCase()}</td>
              <td>${z.line_count}</td>
              <td><button class="cm-z-go" data-path="${z.path}" title="${L.tipNav}">→</button></td>
            </tr>`).join('')}
          </tbody>
        </table>`;
      zombieEl.querySelectorAll('.cm-z-go').forEach(btn => {
        btn.addEventListener('click', () => {
          zombieEl.style.display = 'none';
          zombieBtn.classList.remove('active');
          const node = S.nodes.find(n => n.path === btn.dataset.path);
          if (node) selectAndCenter(node);
        });
      });
    } catch (e) {
      zombieEl.innerHTML = `<div class="cm-err">${e.message}</div>`;
    }
  }

  // ── Reindex ────────────────────────────────────────────────────────────

  reindexBtn.addEventListener('click', async () => {
    if (S.status.running) return;
    reindexBtn.disabled = true;
    reindexBtn.textContent = '⏳';
    try {
      const rr = await fetch('/api/codemap/reindex', { method: 'POST' });
      if (!rr.ok) throw new Error(`reindex: ${rr.status} ${await rr.text().catch(() => rr.statusText)}`);
      // Memory leak fix (Gemini Bug #17 — 2026-04-27): poll interval disimpan
      // ke window-scoped reference + di-clear kalau ada interval poll
      // sebelumnya yang masih jalan (mis. user trigger reindex 2x cepat,
      // atau pindah tab lalu balik). Tanpa cleanup, server-side hang →
      // poll jalan selamanya di background.
      if (window._cmReindexPoll) clearInterval(window._cmReindexPoll);
      window._cmReindexPoll = setInterval(async () => {
        await loadStatus();
        if (!S.status.running) {
          clearInterval(window._cmReindexPoll);
          window._cmReindexPoll = null;
          reindexBtn.disabled = false;
          reindexBtn.textContent = '🔄 Reindex';
          await loadGraph();
        }
      }, 1500);
    } catch (_) {
      reindexBtn.disabled = false;
      reindexBtn.textContent = '🔄 Reindex';
    }
  });

  // ── R6 Enrich: tambah lapisan makna (LLM) ke self-map, batch incremental ──
  enrichBtn?.addEventListener('click', async () => {
    enrichBtn.disabled = true;
    const orig = enrichBtn.textContent;
    enrichBtn.textContent = '⏳ enrich…';
    try {
      const rr = await fetch('/api/codemap/enrich?limit=20', { method: 'POST' });
      const d = await rr.json();
      if (!rr.ok || d.error) throw new Error(d.error || `enrich: ${rr.status}`);
      await loadSemantic();
      if (S.selected) renderDetail(S.selected); // refresh panel kalau ada node kepilih
      statusTxt.textContent = `🧠 enriched ${d.enriched} · sisa ${d.remaining}/${d.total_files}`;
    } catch (e) {
      statusTxt.textContent = `enrich gagal: ${e.message}`;
    } finally {
      enrichBtn.disabled = false;
      enrichBtn.textContent = orig;
    }
  });

  // ── Fit to screen ──────────────────────────────────────────────────────

  container.querySelector('#cm-fit')?.addEventListener('click', fitGraph);

  function fitGraph() {
    if (!S.nodes.length) return;
    const xs = S.nodes.map(n => n.x).filter(Boolean);
    const ys = S.nodes.map(n => n.y).filter(Boolean);
    if (!xs.length) return;
    const minX = Math.min(...xs), maxX = Math.max(...xs);
    const minY = Math.min(...ys), maxY = Math.max(...ys);
    const W = svgEl.clientWidth, H = svgEl.clientHeight;
    const pad = 60;
    const scale = Math.min(0.9, (W - pad*2) / (maxX - minX + 1), (H - pad*2) / (maxY - minY + 1));
    const tx = W/2 - scale * (minX + maxX) / 2;
    const ty = H/2 - scale * (minY + maxY) / 2;
    svg.transition().duration(600).call(zoom.transform, d3.zoomIdentity.translate(tx, ty).scale(scale));
  }

  // ── Legend ─────────────────────────────────────────────────────────────

  function updateLegend() {
    const lg = container.querySelector('#cm-legend');
    if (S.colorMode === 'health') {
      lg.innerHTML = `
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#22c55e"></span>80-100</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#eab308"></span>60-79</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#f97316"></span>40-59</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#ef4444"></span>&lt;40</span>`;
    } else if (S.colorMode === 'folder') {
      lg.innerHTML = Object.entries(S.folderPalette).slice(0, 6).map(([k, v]) =>
        `<span class="cm-lg-item"><span class="cm-lg-dot" style="background:${v}"></span>${k}</span>`
      ).join('');
    } else {
      lg.innerHTML = `
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#a78bfa"></span>selected</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#ef4444"></span>direct</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#f97316"></span>2-hop</span>
        <span class="cm-lg-item"><span class="cm-lg-dot" style="background:#eab308"></span>3-hop</span>`;
    }
  }

  // Re-run legend on mode change
  colorBtns.forEach(btn => btn.addEventListener('click', updateLegend));

  // ── Resize ─────────────────────────────────────────────────────────────

  new ResizeObserver(() => {
    if (S.simulation) {
      const W = svgEl.clientWidth, H = svgEl.clientHeight;
      S.simulation.force('center', d3.forceCenter(W/2, H/2).strength(0.05));
    }
  }).observe(svgEl);

  // ── Markdown renderer ──────────────────────────────────────────────────

  function mdToHTML(md) {
    return md
      .replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;')
      .replace(/^### (.+)$/gm,'<h3>$1</h3>').replace(/^## (.+)$/gm,'<h2>$1</h2>').replace(/^# (.+)$/gm,'<h1>$1</h1>')
      .replace(/\*\*(.+?)\*\*/g,'<strong>$1</strong>').replace(/`([^`]+)`/g,'<code>$1</code>')
      .replace(/^- (.+)$/gm,'<li>$1</li>').replace(/^---$/gm,'<hr>').replace(/\n\n/g,'<br><br>');
  }

  // ── Boot ───────────────────────────────────────────────────────────────

  setDetail(emptyDetail());
  loadStatus();
  loadGraph();
  updateLegend();

  // Memory leak fix (Gemini Bug #6 — 2026-04-26): poll status interval
  // disimpan ke window-scoped reference + cleared kalau ada interval
  // sebelumnya. Setiap kali user re-render tab Code Map, interval lama
  // di-clear sebelum bikin baru → no progressive accumulation.
  if (window._cmStatusInterval) clearInterval(window._cmStatusInterval);
  window._cmStatusInterval = setInterval(loadStatus, 15000);
}

// ── D3 loader ─────────────────────────────────────────────────────────────────
// Try local /vendor/d3.min.js first (sovereignty doctrine). Kalau ngga ada,
// reject → caller render fallback list view. CDN dihindari.

function loadD3() {
  if (window.d3) return Promise.resolve(window.d3);
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = '/vendor/d3.min.js';
    s.onload  = () => resolve(window.d3);
    s.onerror = () => reject(new Error('D3 lokal /vendor/d3.min.js missing — copy library manual'));
    document.head.appendChild(s);
  });
}

// ── Fallback list view (saat D3 unavailable) ──────────────────────────────
// Tab tetap functional via 2 panel: roots list + zombie list. No graph viz.
async function renderFallbackList(container) {
  const graphEl = container.querySelector('#cm-graph');
  graphEl.innerHTML = `
    <div class="cm-fallback-shell">
      <div class="cm-fallback-banner">
        <strong>📋 List Mode</strong> — D3.js graph view unavailable
        (<code>static/vendor/d3.min.js</code> not found). This tab still
        works as a table list. To enable the graph: drop
        <code>d3.min.js</code> into <code>static/vendor/</code> and refresh.
      </div>
      <div class="cm-fallback-grid">
        <div class="cm-fallback-panel">
          <h3>📁 Entry-point Files (roots)</h3>
          <div id="cm-fb-roots" class="cm-fb-loading">Loading…</div>
        </div>
        <div class="cm-fallback-panel">
          <h3>🧟 Zombie Files (no import in/out)</h3>
          <div id="cm-fb-zombies" class="cm-fb-loading">Loading…</div>
        </div>
      </div>
    </div>
  `;
  applyFallbackStyles();

  // Load roots + zombies via existing endpoints (auth via shared cookie).
  try {
    const r = await fetch('/api/codemap/roots');
    const data = r.ok ? await r.json() : { roots: [] };
    const list = (data.roots || data.data || []).slice(0, 50);
    if (list.length === 0) {
      document.getElementById('cm-fb-roots').innerHTML = `<div class="cm-fb-empty">${L.noRoot}</div>`;
    } else {
      document.getElementById('cm-fb-roots').innerHTML =
        '<ul class="cm-fb-list">' + list.map(n =>
          `<li><code>${(n.path || n.name || n).replace(/</g, '&lt;')}</code></li>`
        ).join('') + '</ul>';
    }
  } catch (e) {
    document.getElementById('cm-fb-roots').innerHTML = '<div class="cm-fb-err">Error: ' + e.message + '</div>';
  }

  try {
    const r = await fetch('/api/codemap/zombies');
    const data = r.ok ? await r.json() : { zombies: [] };
    const list = (data.zombies || data.data || []).slice(0, 50);
    if (list.length === 0) {
      document.getElementById('cm-fb-zombies').innerHTML = `<div class="cm-fb-empty">${L.noZombie2}</div>`;
    } else {
      document.getElementById('cm-fb-zombies').innerHTML =
        '<ul class="cm-fb-list">' + list.map(n => {
          const path = (n.path || n.name || n).replace(/</g, '&lt;');
          const conf = n.high_confidence ? ' <span class="cm-fb-tag-high">HIGH</span>' : '';
          return `<li><code>${path}</code>${conf}</li>`;
        }).join('') + '</ul>';
    }
  } catch (e) {
    document.getElementById('cm-fb-zombies').innerHTML = '<div class="cm-fb-err">Error: ' + e.message + '</div>';
  }
}

function applyFallbackStyles() {
  if (document.getElementById('cm-fb-style')) return;
  const s = document.createElement('style');
  s.id = 'cm-fb-style';
  s.textContent = `
  .cm-fallback-shell { padding: 16px; height: 100%; overflow-y: auto; }
  .cm-fallback-banner {
    background: rgba(245,158,11,0.10);
    border: 1px solid rgba(245,158,11,0.32);
    border-radius: 8px;
    padding: 12px 14px;
    color: #fcd34d;
    font-size: 0.84rem;
    margin-bottom: 16px;
    line-height: 1.5;
  }
  .cm-fallback-banner code {
    background: rgba(0,0,0,0.3);
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 0.78rem;
  }
  .cm-fallback-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
    gap: 14px;
  }
  .cm-fallback-panel {
    background: rgba(15,17,26,0.6);
    border: 1px solid rgba(139,92,246,0.18);
    border-radius: 10px;
    padding: 12px 14px;
    backdrop-filter: blur(8px);
  }
  .cm-fallback-panel h3 {
    margin: 0 0 10px 0;
    font-size: 0.86rem;
    color: #c4b5fd;
    letter-spacing: 0.04em;
  }
  .cm-fb-list {
    list-style: none;
    padding: 0;
    margin: 0;
    max-height: 50vh;
    overflow-y: auto;
  }
  .cm-fb-list li {
    padding: 4px 0;
    font-size: 0.78rem;
    border-bottom: 1px solid rgba(255,255,255,0.04);
  }
  .cm-fb-list code {
    color: #94a3b8;
    font-family: monospace;
    font-size: 0.74rem;
  }
  .cm-fb-tag-high {
    display: inline-block;
    margin-left: 6px;
    padding: 1px 6px;
    background: rgba(239,68,68,0.18);
    color: #fca5a5;
    border-radius: 4px;
    font-size: 0.66rem;
    font-weight: 700;
    letter-spacing: 0.04em;
  }
  .cm-fb-empty {
    color: var(--text-muted);
    font-style: italic;
    padding: 8px 0;
  }
  .cm-fb-err {
    color: #fca5a5;
    padding: 8px 0;
  }
  .cm-fb-loading {
    color: var(--text-muted);
    font-style: italic;
    padding: 8px 0;
  }
  `;
  document.head.appendChild(s);
}

// ── HTML skeleton ─────────────────────────────────────────────────────────────

function skeleton() {
  return `
  <div class="cmf-root">
    <div class="cmf-toolbar">
      <input id="cm-search" class="cmf-search" type="search" placeholder="${L.searchPh}" title="${L.tipSearch}" />
      <button id="cm-reindex" class="cmf-btn" title="${L.tipReindex}">🔄 Reindex</button>
      <button id="cm-enrich" class="cmf-btn" title="Tambah lapisan MAKNA (LLM) ke self-map: summary/domain/role per file (R6)">🧠 Enrich</button>
      <button id="cm-fit" class="cmf-btn" title="${L.tipFit}">⊞ Fit</button>
      <div class="cmf-color-grp">
        <button class="cm-color-btn active" data-mode="health" title="${L.tipHealth}">❤️ Health</button>
        <button class="cm-color-btn" data-mode="folder" title="${L.tipFolder}">📁 Folder</button>
        <button class="cm-color-btn" data-mode="layer" title="${L.tipLayer}">🏛️ Layer</button>
        <button class="cm-color-btn" data-mode="blast" title="${L.tipBlast2}">💥 Blast</button>
      </div>
      <button id="cm-tour-btn" class="cmf-btn" title="${L.tipTour}">🎓 Tour</button>
      <button id="cm-zombie-btn" class="cmf-btn" title="${L.tipZombie}">🧟 Zombie</button>
      <div id="cm-legend" class="cmf-legend"></div>
      <span id="cm-status" class="cmf-status"></span>
    </div>

    <div class="cmf-body">
      <div id="cm-graph" class="cmf-graph">
        <svg id="cm-svg" width="100%" height="100%"></svg>
      </div>
      <div id="cm-detail" class="cmf-detail"></div>
    </div>

    <div id="cm-zombie-panel" class="cmf-zombie" style="display:none"></div>
  </div>`;
}

// ── Styles ────────────────────────────────────────────────────────────────────

function applyStyles() {
  if (document.getElementById('cmf-style')) return;
  const s = document.createElement('style');
  s.id = 'cmf-style';
  s.textContent = `
  .cmf-root { display:flex; flex-direction:column; height:100%; color:var(--text,#e2e8f0); font-size:0.82rem; position:relative; }

  /* Toolbar */
  .cmf-toolbar { display:flex; align-items:center; gap:6px; padding:7px 12px; flex-shrink:0;
    border-bottom:1px solid rgba(139,92,246,0.2); background:rgba(8,10,20,0.8); flex-wrap:wrap; }
  .cmf-search { flex:0 0 160px; padding:4px 8px; background:rgba(255,255,255,0.05);
    border:1px solid rgba(139,92,246,0.3); border-radius:6px; color:inherit; font-size:0.79rem; }
  .cmf-btn { padding:4px 10px; background:rgba(139,92,246,0.15); border:1px solid rgba(139,92,246,0.3);
    border-radius:6px; color:#c4b5fd; cursor:pointer; font-size:0.78rem; white-space:nowrap; }
  .cmf-btn:hover:not(:disabled) { background:rgba(139,92,246,0.32); }
  .cmf-btn:disabled { opacity:0.4; cursor:default; }
  .cmf-btn.active { background:rgba(139,92,246,0.38); }
  .cmf-color-grp { display:flex; gap:1px; background:rgba(0,0,0,0.4); border-radius:7px; padding:2px; }
  .cm-color-btn { padding:3px 9px; background:transparent; border:none; color:var(--text-muted,#94a3b8);
    border-radius:5px; cursor:pointer; font-size:0.77rem; }
  .cm-color-btn.active { background:rgba(139,92,246,0.38); color:#c4b5fd; }
  .cmf-legend { display:flex; gap:8px; flex-wrap:wrap; }
  .cm-lg-item { display:flex; align-items:center; gap:4px; font-size:0.7rem; color:var(--text-muted,#94a3b8); }
  .cm-lg-dot { width:9px; height:9px; border-radius:50%; flex-shrink:0; }
  .cmf-status { margin-left:auto; font-size:0.72rem; color:var(--text-muted,#94a3b8); white-space:nowrap; }

  /* Body */
  .cmf-body { display:flex; flex:1; overflow:hidden; }
  .cmf-graph { flex:1; overflow:hidden; background:#060810; position:relative; }
  #cm-svg { display:block; }
  .cm-err-big { padding:60px 30px; text-align:center; color:#f87171; font-size:0.9rem; line-height:1.8; }

  /* D3 nodes */
  .cm-zoom-g .cm-n circle { transition:fill-opacity 0.2s, stroke 0.2s; }
  .cm-zoom-g .cm-n text   { user-select:none; }
  .cm-zoom-g .cm-link-g line { transition:stroke-opacity 0.2s; }

  /* Detail panel */
  .cmf-detail { width:260px; flex-shrink:0; overflow-y:auto; border-left:1px solid rgba(139,92,246,0.12);
    background:rgba(8,10,20,0.6); padding:14px; }
  .cm-empty-d { display:flex; flex-direction:column; align-items:center; justify-content:center;
    height:100%; text-align:center; color:var(--text-muted,#94a3b8); padding:20px; }
  .cm-d-top { display:flex; align-items:flex-start; gap:10px; margin-bottom:12px; }
  .cm-d-grade { font-size:1.6rem; font-weight:800; border:2px solid; border-radius:8px;
    width:38px; height:38px; display:flex; align-items:center; justify-content:center; flex-shrink:0; }
  .cm-d-name { font-weight:600; font-size:0.88rem; color:#c4b5fd; word-break:break-all; }
  .cm-d-path { font-size:0.67rem; color:var(--text-muted,#94a3b8); word-break:break-all; margin-top:2px; }
  .cm-d-hbar-wrap { display:flex; align-items:center; gap:8px; margin-bottom:10px; }
  .cm-d-hbar { flex:1; height:6px; background:rgba(255,255,255,0.07); border-radius:3px; overflow:hidden; }
  .cm-d-hbar > div { height:100%; border-radius:3px; transition:width 0.4s; }
  .cm-d-chips { display:flex; flex-wrap:wrap; gap:4px; margin-bottom:10px; }
  .cm-chip { padding:2px 7px; border-radius:10px; font-size:0.7rem;
    background:rgba(255,255,255,0.06); border:1px solid rgba(255,255,255,0.08); }
  .cm-chip.ok  { background:rgba(34,197,94,0.1); border-color:rgba(34,197,94,0.2); color:#4ade80; }
  .cm-chip.bad { background:rgba(239,68,68,0.08); border-color:rgba(239,68,68,0.2); color:#f87171; }
  .cm-d-sec { margin-bottom:11px; }
  .cm-d-lbl { font-size:0.68rem; font-weight:600; color:var(--text-muted,#94a3b8);
    text-transform:uppercase; letter-spacing:0.4px; margin-bottom:5px; }
  .cm-d-ul { padding-left:14px; margin:0; font-size:0.75rem; }
  .cm-d-ul li { margin:2px 0; }
  .cm-pill { display:inline-block; padding:2px 7px; border-radius:10px; font-size:0.7rem;
    cursor:pointer; max-width:130px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap;
    border:1px solid transparent; margin:2px 2px 0 0; transition:opacity 0.12s; }
  .cm-pill:hover { opacity:0.7; }
  .cm-pill.dep { background:rgba(99,102,241,0.12); border-color:rgba(99,102,241,0.3); color:#818cf8; }
  .cm-pill.rev { background:rgba(34,197,94,0.08); border-color:rgba(34,197,94,0.25); color:#4ade80; }
  .cm-more { font-size:0.7rem; color:var(--text-muted,#94a3b8); }
  .cm-sm-btn { padding:1px 6px; background:rgba(139,92,246,0.15); border:1px solid rgba(139,92,246,0.3);
    border-radius:4px; color:#c4b5fd; cursor:pointer; font-size:0.72rem; }
  .cm-action-btn { padding:4px 10px; background:rgba(139,92,246,0.12); border:1px solid rgba(139,92,246,0.25);
    border-radius:6px; color:#c4b5fd; cursor:pointer; font-size:0.75rem; margin-right:4px; }
  .cm-action-btn:hover { background:rgba(139,92,246,0.28); }
  .cm-md h1,h2,h3 { color:#c4b5fd; } .cm-md code { background:rgba(139,92,246,0.15); padding:1px 4px; border-radius:3px; }

  /* Zombie panel */
  .cmf-zombie { position:absolute; bottom:0; left:0; right:260px; max-height:260px; overflow-y:auto;
    background:rgba(10,12,22,0.97); border-top:1px solid rgba(239,68,68,0.3); padding:12px;
    z-index:10; font-size:0.8rem; }
  .cm-z-title { color:#fca5a5; margin-bottom:10px; font-weight:500; }
  .cm-z-ok { color:#4ade80; padding:12px 0; }
  .cm-z-table { width:100%; border-collapse:collapse; font-size:0.77rem; }
  .cm-z-table td { padding:4px 8px; border-bottom:1px solid rgba(255,255,255,0.04); }
  .cm-z-go { padding:2px 8px; background:rgba(139,92,246,0.12); border:1px solid rgba(139,92,246,0.3);
    border-radius:4px; color:#c4b5fd; cursor:pointer; }

  /* Utility */
  .cm-loading { padding:20px; text-align:center; color:var(--text-muted,#94a3b8); }
  .cm-err { padding:10px; background:rgba(239,68,68,0.08); border-radius:6px; color:#f87171; }
  `;
  document.head.appendChild(s);
}
