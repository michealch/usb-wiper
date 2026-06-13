// Activity (audit log) view - read-only feed of recent events.

import { apiGet } from '../api.js';
import { escapeHtml } from '../util.js';

let events = [];
let filterType = '';
let filterText = '';

async function loadAndRenderActivity() {
  const el = document.getElementById('view-activity');
  el.innerHTML = '<div class="card"><p class="muted">Loading activity...</p></div>';
  try {
    const data = await apiGet('/api/audit');
    events = data.events || [];
    renderActivity();
  } catch (e) {
    el.innerHTML = `<div class="card"><p class="text-danger">Failed to load activity: ${escapeHtml(e.message)}</p></div>`;
  }
}

function eventField(e, ...keys) {
  for (const k of keys) {
    if (e[k] !== undefined && e[k] !== null) return e[k];
  }
  return '';
}

function renderActivity() {
  const el = document.getElementById('view-activity');

  if (events.length === 0) {
    el.innerHTML = `
      <div class="view-shell">
        <div class="page-head">
          <div>
            <div class="page-title">Activity</div>
            <div class="page-subtitle">Audit trail</div>
          </div>
        </div>
        <section class="panel">
          <div class="empty-state"><div class="empty-state-title">No activity yet.</div></div>
        </section>
      </div>`;
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
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Activity</div>
          <div class="page-subtitle">${filtered.length} of ${events.length} event${events.length === 1 ? '' : 's'}</div>
        </div>
      </div>

      <section class="panel">
        <div class="panel-header">
          <div class="panel-title">Events</div>
          <div class="audit-filter">
            <select id="activity-type-filter">
              <option value="">All event types</option>
              ${types.map(t => `<option value="${escapeHtml(t)}" ${t === filterType ? 'selected' : ''}>${escapeHtml(t)}</option>`).join('')}
            </select>
            <input type="text" id="activity-text-filter" placeholder="Filter by text..." value="${escapeHtml(filterText)}">
          </div>
        </div>
        ${filtered.length === 0 ? `<div class="empty-state"><div class="empty-state-title">No events match this filter.</div></div>` : `
          <div class="timeline">
            ${filtered.map(e => {
              const time = eventField(e, 'timestamp', 'time', 'createdAt');
              const type = eventField(e, 'eventType', 'event', 'type');
              const target = eventField(e, 'target', 'devicePath', 'device');
              const details = eventField(e, 'details', 'message', 'description');
              return `
                <div class="timeline-item">
                  <div class="timeline-time">${time ? escapeHtml(new Date(time).toLocaleString()) : '-'}</div>
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
      </section>
    </div>
  `;

  const typeSelect = el.querySelector('#activity-type-filter');
  const textInput = el.querySelector('#activity-text-filter');
  if (typeSelect) typeSelect.onchange = () => { filterType = typeSelect.value; renderActivity(); };
  if (textInput) textInput.oninput = () => { filterText = textInput.value; renderActivity(); };
}

export { loadAndRenderActivity, renderActivity };
