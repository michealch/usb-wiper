// Activity (audit log) panel renderer — used by the Records screen's Activity tab.

import { apiGet } from '../api.js';
import { escapeHtml, formatDateTime, emptyState } from '../util.js';

let events = [];
let filterType = '';
let filterText = '';

function eventField(e, ...keys) {
  for (const k of keys) {
    if (e[k] !== undefined && e[k] !== null) return e[k];
  }
  return '';
}

// renderActivityPanel(el) renders the audit feed into el. Module-level filter state
// survives across re-renders; the filter controls re-render into the same element.
export async function renderActivityPanel(el) {
  try {
    const data = await apiGet('/api/audit');
    events = data.events || [];
  } catch (e) {
    el.innerHTML = `<p class="text-danger">Failed to load activity: ${escapeHtml(e.message)}</p>`;
    return;
  }

  if (events.length === 0) {
    el.innerHTML = emptyState('No activity yet.');
    return;
  }

  const types = Array.from(new Set(events.map(e => String(eventField(e, 'eventType', 'event', 'type'))))).filter(Boolean).sort();

  const filtered = events.filter(e => {
    const type = String(eventField(e, 'eventType', 'event', 'type'));
    if (filterType && type !== filterType) return false;
    if (filterText) {
      const haystack = JSON.stringify(e).toLowerCase();
      if (!haystack.includes(filterText.toLowerCase())) return false;
    }
    return true;
  });

  el.innerHTML = `
    <div class="panel-header">
      <div class="panel-title">Events</div>
      <div class="panel-note">${filtered.length} of ${events.length} event${events.length === 1 ? '' : 's'}</div>
      <div class="audit-filter">
        <select id="activity-type-filter">
          <option value="">All event types</option>
          ${types.map(t => `<option value="${escapeHtml(t)}" ${t === filterType ? 'selected' : ''}>${escapeHtml(t)}</option>`).join('')}
        </select>
        <input type="text" id="activity-text-filter" placeholder="Filter by text..." value="${escapeHtml(filterText)}">
      </div>
    </div>
    ${filtered.length === 0 ? emptyState('No events match this filter.') : `
      <div class="timeline">
        ${filtered.map(e => {
          const time = eventField(e, 'timestamp', 'time', 'createdAt');
          const type = eventField(e, 'eventType', 'event', 'type');
          const target = eventField(e, 'target', 'devicePath', 'device');
          const details = eventField(e, 'details', 'message', 'description');
          return `
            <div class="timeline-item">
              <div class="timeline-time">${formatDateTime(time)}</div>
              <div class="timeline-body">
                <div class="timeline-title">
                  <span class="badge badge-neutral">${escapeHtml(String(type) || '-')}</span>
                  ${target ? `<span class="text-mono">${escapeHtml(String(target))}</span>` : ''}
                </div>
                <div class="timeline-meta">${escapeHtml(typeof details === 'object' ? JSON.stringify(details) : String(details || '-'))}</div>
              </div>
            </div>`;
        }).join('')}
      </div>
    `}
  `;

  const typeSelect = el.querySelector('#activity-type-filter');
  const textInput = el.querySelector('#activity-text-filter');
  if (typeSelect) typeSelect.onchange = () => { filterType = typeSelect.value; renderActivityPanel(el); };
  if (textInput) textInput.oninput = () => { filterText = textInput.value; renderActivityPanel(el); };
}
