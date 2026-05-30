// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: i18n dictionary loader. Audit pass — local fetch /i18n/<locale>/, fallback to key string on miss, no innerHTML..

// Stable foundation — every GUI string flows through this module. Treat
// the public surface (loadI18n / t / applyI18n) as a contract. New
// requirements add keys to the JSON dictionaries, not new code here.
//
// Tiny dictionary lookup. Single source of truth for every user-visible
// string in the GUI — tooltips, menu labels, error messages, statuses.
//
// Architecture mandate: no hardcoded copy in JS/HTML. Everything goes
// through t('key.path'). Missing keys return the key itself so a typo
// or new string is immediately visible during development.
//
// Files live under /i18n/<locale>/<domain>.json. Add a domain by dropping
// a new .json there and adding its name to DOMAINS — no other changes.

const DOMAINS = ['menu', 'tooltip', 'error', 'status', 'common'];
const DEFAULT_LOCALE = 'en';

const dict = {};
let activeLocale = DEFAULT_LOCALE;
let loaded = false;

// Load every domain JSON for the requested locale in parallel. Falls back
// to the default locale if a domain is missing.
export async function loadI18n(locale = DEFAULT_LOCALE) {
  activeLocale = locale;
  const wanted = await Promise.all(
    DOMAINS.map(async (d) => {
      try {
        const r = await fetch(`/i18n/${locale}/${d}.json`, { cache: 'no-store' });
        if (!r.ok) return [d, {}];
        return [d, await r.json()];
      } catch {
        return [d, {}];
      }
    })
  );
  for (const [d, body] of wanted) dict[d] = body;
  loaded = true;
  return dict;
}

// Lookup dot-path. `t('menu.sidebar.komunikasi')` walks the JSON tree.
// Returns the key itself on miss so the UI signals "i18n hole here".
export function t(path) {
  if (!loaded) return path;
  const parts = String(path).split('.');
  let node = dict[parts[0]];
  for (let i = 1; i < parts.length; i++) {
    if (node == null || typeof node !== 'object') return path;
    node = node[parts[i]];
  }
  return typeof node === 'string' ? node : path;
}

// Walk the DOM and replace text / tooltip attrs that carry an i18n key.
//
//   <button data-t="menu.sidebar.komunikasi">…</button>
//     → element textContent = t('menu.sidebar.komunikasi'). Existing
//       child whitespace/emoji content is preserved by replacing ONLY
//       the trailing text node, so emoji prefixes stay intact.
//
//   <button data-tooltip-key="tooltip.sidebar.komunikasi">…</button>
//     → element data-tooltip attr = t(...). Original data-tooltip-key
//       stays so re-applying after locale switch works.
//
//   <span data-t-title="status.aktif">…</span>
//     → title attribute set.
export function applyI18n(root = document) {
  root.querySelectorAll('[data-t]').forEach((el) => {
    const key = el.getAttribute('data-t');
    const value = t(key);
    // If the element has emoji/icon as the first child node, keep it
    // and only replace the trailing text. Otherwise, set textContent.
    const text = el.lastChild;
    if (text && text.nodeType === Node.TEXT_NODE && el.children.length === 0 && el.childNodes.length > 1) {
      text.nodeValue = ' ' + value;
    } else {
      el.textContent = value;
    }
  });
  root.querySelectorAll('[data-tooltip-key]').forEach((el) => {
    el.setAttribute('data-tooltip', t(el.getAttribute('data-tooltip-key')));
  });
  root.querySelectorAll('[data-t-title]').forEach((el) => {
    el.setAttribute('title', t(el.getAttribute('data-t-title')));
  });
}

// Convenience for inline JS that needs the active locale code.
export function currentLocale() {
  return activeLocale;
}
