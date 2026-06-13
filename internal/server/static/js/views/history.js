// History view

import { apiGet } from '../api.js';
import { renderVerifyTool } from '../components/cert.js';
import { escapeHtml, formatBytes } from '../util.js';

let historyData = [];

async function loadAndRenderHistory() {
  try {
    const data = await apiGet('/api/history');
    historyData = data.history || [];
    renderHistory();
  } catch (e) {
    document.getElementById('view-history').innerHTML = '<div class="card"><p class="text-danger">Failed to load history: ' + escapeHtml(e.message) + '</p></div>';
  }
}

function renderHistory() {
  const el = document.getElementById('view-history');
  const completed = historyData.filter(r => r.status === 'completed').length;
  const failed = historyData.filter(r => r.status === 'failed').length;

  const verifyPanel = `
    <section class="panel">
      <div class="panel-header">
        <div class="panel-title">Verify certificate</div>
      </div>
      <div id="history-verify-tool"></div>
    </section>
  `;

  if (historyData.length === 0) {
    el.innerHTML = `
      <div class="view-shell">
        <div class="page-head">
          <div>
            <div class="page-title">History</div>
            <div class="page-subtitle">Completed wipe records and certificates</div>
          </div>
        </div>
        <div class="split-grid">
          <section class="panel"><div class="empty-state"><div class="empty-state-title">No wipe history yet.</div></div></section>
          ${verifyPanel}
        </div>
      </div>`;
    renderVerifyTool(el.querySelector('#history-verify-tool'));
    return;
  }

  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">History</div>
          <div class="page-subtitle">${historyData.length} record${historyData.length === 1 ? '' : 's'} saved</div>
        </div>
      </div>

      <div class="metric-strip">
        <div class="metric-pill"><span class="metric-label">Records</span><span class="metric-value">${historyData.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Completed</span><span class="metric-value">${completed}</span></div>
        <div class="metric-pill"><span class="metric-label">Failed</span><span class="metric-value">${failed}</span></div>
      </div>

      <div class="split-grid">
        <section class="panel">
          <div class="panel-header">
            <div class="panel-title">Wipe Records</div>
            <div class="panel-note">Newest first</div>
          </div>
          <div class="table-scroll table-stack">
            <table class="record-table">
              <thead><tr><th scope="col">Device</th><th scope="col">Result</th><th scope="col">Size</th><th scope="col">Duration</th><th scope="col">Date</th></tr></thead>
              <tbody>${historyData.map(r => `
                <tr class="clickable">
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
                  <td data-label="Date" style="font-size:var(--text-xs);color:var(--color-text-dim)">${r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '-'}</td>
                </tr>`).join('')}</tbody>
            </table>
          </div>
        </section>
        ${verifyPanel}
      </div>
    </div>
  `;

  renderVerifyTool(el.querySelector('#history-verify-tool'));
}

export { loadAndRenderHistory, renderHistory };
