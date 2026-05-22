// History view

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
  
  if (historyData.length === 0) {
    el.innerHTML = '<div class="card"><div class="empty-state"><span class="icon">📜</span>No wipe history yet.</div></div>';
    return;
  }

  el.innerHTML = `
    <div class="card">
      <div class="card-header"><h2>Wipe History (${historyData.length})</h2></div>
      <table>
        <thead><tr><th>Device</th><th>Status</th><th>Verification</th><th>Size</th><th>Duration</th><th>Date</th></tr></thead>
        <tbody>${historyData.map(r => `
          <tr class="clickable">
            <td style="font-family:var(--font-mono)">${r.devicePath}</td>
            <td><span class="badge ${r.status === 'completed' ? 'badge-success' : r.status === 'failed' ? 'badge-danger' : 'badge-neutral'}">${r.status}</span></td>
            <td>${r.verification || '—'}</td>
            <td>${formatBytes(r.sizeBytes)}</td>
            <td>${r.duration || '—'}</td>
            <td style="font-size:.78rem">${r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—'}</td>
          </tr>`).join('')}</tbody>
      </table>
    </div>
  `;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function escapeHtml(s) { const d = document.createElement('div'); d.textContent = s; return d.innerHTML; }

import { apiGet } from '../api.js';

export { loadAndRenderHistory, renderHistory };
