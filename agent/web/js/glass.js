// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// js/ui/glass.js — KIT UI 3D-GLASS share (nano-module, reusable lintas-tab). Primitive
// clean+3D ala "Autonomy/Nervous System" + "Threat Radar": tile gradient + inset-shadow +
// hover-lift, pakai CSS var design-system (--accent/--glass-border/--text-main). NOL hardcode
// warna mentah — tema light/dark ikut otomatis. Dipake AI Studio + tab lain.
import { esc, loadStyle } from './utils.js';

// ensureGlass — inject CSS kit sekali (idempotent). Panggil di awal render tab.
export function ensureGlass() {
  loadStyle('fw-glass', GLASS_CSS);
}

// statTile — kartu statistik 3D (angka gede + label). tone = warna aksen (opsional).
export function statTile(emoji, n, label, tone) {
  const t = tone ? ` style="--gl-accent:${tone}"` : '';
  return `<div class="gl-tile"${t}>
    <div class="gl-tile-n">${n | 0}</div>
    <div class="gl-tile-l">${esc(emoji)} ${esc(label)}</div>
  </div>`;
}

// badge — pil status kecil. tone: 'ok'|'warn'|'bad'|'mute' (atau hex via 4th arg).
const BADGE_TONE = { ok: '#34d399', warn: '#fbbf24', bad: '#f87171', mute: '#94a3b8' };
export function badge(text, tone = 'mute') {
  const c = BADGE_TONE[tone] || tone || BADGE_TONE.mute;
  return `<span class="gl-badge" style="--gl-bc:${c}">${esc(text)}</span>`;
}

// glassBtn — tombol kecil ber-aksen (label sudah di-esc oleh caller kalau perlu).
export function glassBtn(label, tone = 'mute') {
  const c = BADGE_TONE[tone] || tone || BADGE_TONE.mute;
  return `<button class="gl-btn" style="--gl-bc:${c}">${esc(label)}</button>`;
}

// row — baris item dalam panel (flex, glass tipis).
export function row(inner) {
  return `<div class="gl-row">${inner}</div>`;
}

const GLASS_CSS = `
.gl-tiles { display:grid; grid-template-columns:repeat(auto-fit,minmax(96px,1fr)); gap:10px; }
.gl-tile {
  position:relative; text-align:center; padding:14px 10px; border-radius:14px;
  border:1px solid var(--glass-border);
  background:
    radial-gradient(circle at 22% 0%, color-mix(in srgb, var(--gl-accent,var(--accent)) 16%, transparent), transparent 60%),
    linear-gradient(165deg, rgba(255,255,255,.05), rgba(255,255,255,0) 55%),
    var(--bg-panel);
  box-shadow:0 6px 18px rgba(0,0,0,.28), inset 0 1px 0 rgba(255,255,255,.06);
  transition:transform .2s ease, border-color .2s ease, box-shadow .2s ease;
}
.gl-tile:hover { transform:translateY(-3px); border-color:var(--glass-border-hover); box-shadow:0 10px 26px rgba(0,0,0,.34), inset 0 1px 0 rgba(255,255,255,.08); }
.gl-tile-n { font-size:1.7rem; font-weight:800; line-height:1; color:var(--text-main); text-shadow:0 2px 10px color-mix(in srgb, var(--gl-accent,var(--accent)) 30%, transparent); }
.gl-tile-l { font-size:.74rem; color:var(--text-muted); margin-top:6px; }

.gl-badge { padding:2px 9px; border-radius:999px; font-size:.7rem; font-weight:700; letter-spacing:.3px;
  color:var(--gl-bc); background:color-mix(in srgb, var(--gl-bc) 15%, transparent); border:1px solid color-mix(in srgb, var(--gl-bc) 45%, transparent); }
.gl-btn { font:inherit; font-size:.74rem; font-weight:700; padding:4px 11px; border-radius:8px; cursor:pointer;
  color:var(--gl-bc); background:color-mix(in srgb, var(--gl-bc) 12%, transparent); border:1px solid color-mix(in srgb, var(--gl-bc) 40%, transparent);
  transition:filter .15s, transform .1s; }
.gl-btn:hover { filter:brightness(1.18); } .gl-btn:active { transform:translateY(1px); }

.gl-row { display:flex; align-items:center; gap:10px; padding:9px 11px; margin-bottom:7px; border-radius:11px;
  border:1px solid var(--glass-border);
  background:linear-gradient(165deg, rgba(255,255,255,.04), rgba(255,255,255,0) 60%), var(--bg-panel);
  box-shadow:inset 0 1px 0 rgba(255,255,255,.05); transition:border-color .18s, background .18s; }
.gl-row:hover { border-color:var(--glass-border-hover); }

/* ── Drawer 3D (slide dari kanan) ───────────────────────────────────────── */
.gl-backdrop { position:fixed; inset:0; background:rgba(2,6,18,.5); backdrop-filter:blur(3px);
  opacity:0; pointer-events:none; transition:opacity .25s ease; z-index:40; }
.gl-backdrop.on { opacity:1; pointer-events:auto; }
.gl-drawer { position:fixed; top:0; right:0; height:100vh; width:min(440px,94vw); z-index:41;
  display:flex; flex-direction:column;
  background:linear-gradient(180deg, color-mix(in srgb, var(--bg-panel) 92%, #0b1220), color-mix(in srgb, var(--bg-core) 96%, #000));
  border-left:1px solid var(--glass-border-hover); box-shadow:-24px 0 60px rgba(0,0,0,.5);
  transform:translateX(102%); transition:transform .3s cubic-bezier(.4,0,.2,1); }
.gl-drawer.on { transform:translateX(0); }
.gl-drawer-head { display:flex; align-items:center; gap:10px; padding:18px 18px 14px; border-bottom:1px solid var(--glass-border); }
.gl-drawer-head h3 { margin:0; font-size:1rem; color:var(--text-main); font-weight:700; }
.gl-drawer-body { flex:1; overflow-y:auto; padding:16px 18px; scrollbar-width:none; }
.gl-drawer-body::-webkit-scrollbar { display:none; }
.gl-x { margin-left:auto; width:30px; height:30px; border-radius:9px; cursor:pointer; font-size:1rem; line-height:1;
  color:var(--text-muted); background:var(--bg-panel); border:1px solid var(--glass-border); transition:.15s; }
.gl-x:hover { color:var(--text-main); border-color:var(--glass-border-hover); }

.gl-sect-t { font-size:.78rem; font-weight:700; color:var(--accent); margin:18px 0 8px; letter-spacing:.02em; }
.gl-sect-t:first-child { margin-top:0; }
.gl-sect-t .gl-hint { color:var(--text-muted); font-weight:400; font-size:.72rem; }
.gl-empty { color:#64748b; font-size:.8rem; padding:8px 11px; font-style:italic; }

/* ── DESIGN-SYSTEM "fw-*" : kelas page/card share buat SEMUA tab (clean glass-3D, full-width).
   Tab tinggal ensureGlass() + pakai kelas ini → konsisten, nano-modular, nol duplikat CSS. ── */
.fw-page { padding:18px 26px 40px; }
.fw-head { display:flex; align-items:center; gap:14px; margin-bottom:20px; flex-wrap:wrap; }
.fw-glyph { font-size:1.8rem; filter:drop-shadow(0 0 12px var(--accent-glow)); }
.fw-title { margin:0; font-size:1.5rem; font-weight:800; line-height:1.05;
  background:linear-gradient(90deg,#c4b5fd,#67e8f9 58%,#6ee7b7); -webkit-background-clip:text; background-clip:text; color:transparent; }
.fw-sub { font-size:.82rem; color:var(--text-muted); margin-top:3px; max-width:96ch; line-height:1.45; }
.fw-stat { font-size:.74rem; color:var(--text-muted); margin-top:6px; display:flex; align-items:center; gap:7px; }
.fw-dot { width:7px; height:7px; border-radius:50%; background:#34d399; box-shadow:0 0 8px #34d399; animation:fwblink 1.8s ease-in-out infinite; }
@keyframes fwblink { 0%,100%{opacity:1} 50%{opacity:.3} }
.fw-grow { flex:1 1 auto; }

.fw-card { position:relative; border-radius:16px; padding:18px 20px; margin-bottom:14px; transition:transform .2s, border-color .2s, box-shadow .2s;
  border:1px solid var(--glass-border);
  background:
    radial-gradient(circle at 18% 0%, color-mix(in srgb,var(--accent) 9%, transparent), transparent 55%),
    linear-gradient(165deg, rgba(255,255,255,.045), rgba(255,255,255,0) 52%), var(--bg-panel);
  box-shadow:0 10px 30px rgba(0,0,0,.34), inset 0 1px 0 rgba(255,255,255,.06); }
.fw-card:hover { border-color:var(--glass-border-hover); transform:translateY(-2px); box-shadow:0 18px 42px rgba(0,0,0,.44), inset 0 1px 0 rgba(255,255,255,.09); }
.fw-sec { font-size:.78rem; font-weight:700; letter-spacing:.03em; color:var(--accent); margin:0 0 12px; }
.fw-row { display:flex; align-items:center; gap:14px; flex-wrap:wrap; }
.fw-row h3 { margin:0; font-size:1rem; color:var(--text-main); font-weight:700; display:flex; align-items:center; gap:9px; }
.fw-tag { font-size:.7rem; font-weight:700; letter-spacing:.4px; color:var(--accent); border-radius:999px; padding:2px 10px;
  border:1px solid color-mix(in srgb,var(--accent) 40%, transparent); background:color-mix(in srgb,var(--accent) 13%, transparent); }
.fw-id { font-size:.74rem; color:var(--text-muted); font-family:ui-monospace,monospace; }
.fw-desc { margin-top:11px; font-size:.82rem; color:var(--text-muted); line-height:1.5; }
.fw-empty { font-size:.85rem; color:var(--text-muted); padding:22px; text-align:center; font-style:italic; }
.fw-btn { font:inherit; font-size:.76rem; font-weight:700; padding:7px 15px; border-radius:10px; cursor:pointer; transition:filter .15s, transform .1s;
  color:var(--text-main); border:1px solid var(--glass-border-hover); background:color-mix(in srgb,var(--accent) 13%, transparent); box-shadow:0 4px 12px rgba(0,0,0,.2); }
.fw-btn:hover { filter:brightness(1.16); transform:translateY(-1px); }
.fw-btn.danger { color:#f87171; border-color:rgba(248,113,113,.42); background:rgba(248,113,113,.11); }
.fw-drop { border:1.5px dashed var(--glass-border-hover); border-radius:13px; padding:28px; text-align:center; cursor:pointer;
  color:var(--text-muted); font-size:.9rem; background:color-mix(in srgb,var(--bg-panel) 60%, transparent); transition:.2s; }
.fw-drop:hover, .fw-drop.over { background:color-mix(in srgb,var(--accent) 9%, transparent); border-color:var(--accent); color:var(--text-main); }
.fw-input, .fw-card input[type=text], .fw-card input[type=number], .fw-card input[type=password], .fw-card select, .fw-card textarea {
  width:100%; box-sizing:border-box; background:var(--bg-panel); border:1px solid var(--glass-border); color:var(--text-main);
  padding:9px 12px; border-radius:10px; font:inherit; font-size:.85rem; transition:.15s; }
.fw-input:focus, .fw-card input:focus, .fw-card select:focus, .fw-card textarea:focus { outline:none; border-color:var(--accent); box-shadow:0 0 0 2px var(--accent-glow); }
.fw-grid { display:grid; grid-template-columns:repeat(auto-fill, minmax(340px, 1fr)); gap:14px; }
`;
