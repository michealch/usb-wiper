// Records screen — History and Activity as tabs.

import { initTabs } from '../components/tabs.js';
import { renderHistoryPanel } from './history.js';
import { renderActivityPanel } from './activity.js';

// renderRecords(tab) builds the Records shell once per navigation and lets tabs.js
// drive which panel loads. tab is 'history' | 'activity'.
export function renderRecords(tab) {
  const el = document.getElementById('view-records');
  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Records</div>
          <div class="page-subtitle">Wipe history and audit trail</div>
        </div>
      </div>
      <section class="panel" id="records-tabhost"></section>
    </div>
  `;

  initTabs(document.getElementById('records-tabhost'), {
    idPrefix: 'records',
    tabs: [
      { id: 'history', label: 'History' },
      { id: 'activity', label: 'Activity' }
    ],
    activeId: tab,
    onSelect: (id) => {
      // Linkable tab — replaceState so we don't re-enter navigateTo via hashchange.
      history.replaceState(null, '', '#/records/' + id);
      if (id === 'history') renderHistoryPanel(document.getElementById('records-panel-history'));
      else renderActivityPanel(document.getElementById('records-panel-activity'));
    }
  });
}
