// History panel renderer — used by the Records screen's History tab.

import { apiGet } from '../api.js';
import { escapeHtml, formatBytes, formatDateTime, emptyState } from '../util.js';

let historyData = [];

// renderHistoryPanel(el) renders the wipe history table into el. The Records tab
// host owns the .view-shell/.page-head; this panel provides the metric strip and
// the record table only.
export async function renderHistoryPanel(el) {
  try {
    const data = await apiGet('/api/history');
    historyData = data.history || [];
  } catch (e) {
    el.innerHTML = `<p class="text-danger">Failed to load history: ${escapeHtml(e.message)}</p>`;
    return;
  }

  const completed = historyData.filter(r => r.status === 'completed').length;
  const failed = historyData.filter(r => r.status === 'failed').length;

  if (historyData.length === 0) {
    el.innerHTML = `
      <div class="metric-strip">
        <div class="metric-pill"><span class="metric-label">Records</span><span class="metric-value">0</span></div>
        <div class="metric-pill"><span class="metric-label">Completed</span><span class="metric-value">0</span></div>
        <div class="metric-pill"><span class="metric-label">Failed</span><span class="metric-value">0</span></div>
      </div>
      ${emptyState('No wipe history yet.')}`;
    return;
  }

  el.innerHTML = `
    <div class="metric-strip">
      <div class="metric-pill"><span class="metric-label">Records</span><span class="metric-value">${historyData.length}</span></div>
      <div class="metric-pill"><span class="metric-label">Completed</span><span class="metric-value">${completed}</span></div>
      <div class="metric-pill"><span class="metric-label">Failed</span><span class="metric-value">${failed}</span></div>
    </div>

    <div class="table-scroll table-stack">
      <table class="record-table">
        <thead><tr><th scope="col">Device</th><th scope="col">Result</th><th scope="col">Size</th><th scope="col">Duration</th><th scope="col">Date</th></tr></thead>
        <tbody>${historyData.map(r => `
          <tr>
            <td data-label="Device">
              <div class="record-primary">${escapeHtml(r.deviceModel || r.devicePath || '-')}</div>
              <div class="record-secondary">${escapeHtml(r.deviceSerial || r.deviceId || r.devicePath || '-')}</div>
            </td>
            <td data-label="Result">
              <span class="badge ${r.status === 'completed' ? 'badge-success' : r.status === 'failed' ? 'badge-danger' : 'badge-neutral'}">${escapeHtml(r.status)}</span>
              <div class="panel-note mt-1">${escapeHtml(r.verification || 'not verified')}</div>
            </td>
            <td data-label="Size">${formatBytes(r.sizeBytes)}</td>
            <td data-label="Duration">${escapeHtml(r.duration || '-')}</td>
            <td data-label="Date" style="font-size:var(--text-xs);color:var(--color-text-dim)">${formatDateTime(r.finishedAt)}</td>
          </tr>`).join('')}</tbody>
      </table>
    </div>`;
}
