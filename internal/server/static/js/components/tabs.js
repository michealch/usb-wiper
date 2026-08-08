// Tab widget — one implementation for the drawer, Records, and Settings. Renders the
// tablist (role="tablist") and the panels (role="tabpanel"), with roving tabindex and
// ArrowRight/ArrowLeft/Home/End navigation. onSelect fires on every activation
// including the initial one, so hosts do all their loading in one place.

import { escapeHtml } from '../util.js';

// initTabs(containerEl, { idPrefix, tabs, activeId, onSelect, panelsEl }) -> { select, activeId }
// tabs: [{ id: string, label: string }]
// panelsEl is optional and defaults to containerEl; when supplied the tablist is
// written into containerEl and the .tab-panels block into panelsEl (the drawer pins
// its tablist above the scrolling body this way).
export function initTabs(containerEl, { idPrefix, tabs, activeId, onSelect, panelsEl }) {
  const panelHost = panelsEl || containerEl;
  let current = tabs.some(t => t.id === activeId) ? activeId : tabs[0].id;

  const tablistHtml = `
    <div class="tabs" role="tablist">
      ${tabs.map(t => `
        <button class="tab${t.id === current ? ' active' : ''}" role="tab"
                id="${idPrefix}-tab-${t.id}" aria-controls="${idPrefix}-panel-${t.id}"
                aria-selected="${t.id === current ? 'true' : 'false'}"
                tabindex="${t.id === current ? '0' : '-1'}" data-tab="${t.id}">${escapeHtml(t.label)}</button>`).join('')}
    </div>`;
  const panelsHtml = `
    <div class="tab-panels">
      ${tabs.map(t => `
        <div class="tab-panel${t.id === current ? ' active' : ''}" role="tabpanel"
             id="${idPrefix}-panel-${t.id}" aria-labelledby="${idPrefix}-tab-${t.id}"
             tabindex="0"></div>`).join('')}
    </div>`;

  if (panelHost === containerEl) {
    // Same host: write both blocks in one pass so the second write cannot clobber
    // the first.
    containerEl.innerHTML = tablistHtml + panelsHtml;
  } else {
    containerEl.innerHTML = tablistHtml;
    panelHost.innerHTML = panelsHtml;
  }

  const tabEls = Array.from(containerEl.querySelectorAll('.tab'));
  const panelBy = new Map(tabs.map(t => [t.id, panelHost.querySelector('#' + idPrefix + '-panel-' + t.id)]));

  function select(id) {
    if (!panelBy.has(id)) return;
    if (id !== current) {
      current = id;
      tabEls.forEach(el => {
        const active = el.dataset.tab === id;
        el.classList.toggle('active', active);
        el.setAttribute('aria-selected', String(active));
        el.tabIndex = active ? 0 : -1;
      });
      panelBy.forEach((el, pid) => el.classList.toggle('active', pid === id));
    }
    if (onSelect) onSelect(id, panelBy.get(id));
  }

  tabEls.forEach((tab, idx) => {
    tab.addEventListener('click', () => {
      select(tab.dataset.tab);
      tab.focus();
    });
    tab.addEventListener('keydown', (e) => {
      let nextIdx = -1;
      if (e.key === 'ArrowRight') nextIdx = (idx + 1) % tabEls.length;
      else if (e.key === 'ArrowLeft') nextIdx = (idx - 1 + tabEls.length) % tabEls.length;
      else if (e.key === 'Home') nextIdx = 0;
      else if (e.key === 'End') nextIdx = tabEls.length - 1;
      if (nextIdx !== -1) {
        e.preventDefault();
        const next = tabEls[nextIdx];
        select(next.dataset.tab);
        next.focus();
      }
    });
  });

  if (onSelect) onSelect(current, panelBy.get(current));

  return {
    select,
    activeId: () => current
  };
}
