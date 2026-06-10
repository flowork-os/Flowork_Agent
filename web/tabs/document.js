// document.js — Document tab. Placeholder shell; tutorial content is added later
// (owner-driven). Strings live in the existing `menu` i18n domain (menu.tab.document.*),
// so no new i18n domain is needed. Pattern mirrors the other read-only tabs.
import { esc } from '../js/utils.js';
import { t } from '/js/i18n.js';

const L = new Proxy({}, { get: (_, k) => t('menu.tab.document.' + String(k)) });

export async function render(mainEl) {
  mainEl.innerHTML = `
    <h2>${esc(L.title)}</h2>
    <div class="sub">${esc(L.desc)}</div>
    <div class="card">
      <div class="cb"><div class="empty">${esc(L.soon)}</div></div>
    </div>
  `;
}
