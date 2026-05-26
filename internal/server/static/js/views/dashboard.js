// Dashboard view
import { progressMap } from '../app.js';

let devices = [];
let jobs = [];

function renderDashboard() {
  const el = document.getElementById('view-dashboard');
  const running = jobs.filter(j => j.status === 'running' || j.status === 'verifying' || j.status === 'formatting');
  const queued = jobs.filter(j => j.status === 'queued');
  const completedToday = jobs.filter(j => {
    if (!j.completedAt) return false;
    const d = new Date(j.completedAt);
    const today = new Date();
    return d.toDateString() === today.toDateString() && j.status === 'completed';
  });

  el.innerHTML = `
    <div class="kpi-grid">
      <div class="kpi"><div class="kpi-value">${devices.length}</div><div class="kpi-label">Devices Detected</div></div>
      <div class="kpi"><div class="kpi-value">${running.length}</div><div class="kpi-label">Jobs Running</div></div>
      <div class="kpi"><div class="kpi-value">${queued.length}</div><div class="kpi-label">Jobs Queued</div></div>
      <div class="kpi"><div class="kpi-value">${completedToday.length}</div><div class="kpi-label">Completed Today</div></div>
    </div>
    ${deviceSummaryCard()}
    <div class="card">
      <div class="card-header"><h2>Recent Activity</h2></div>
      ${jobs.length === 0 ? '<div class="empty-state"><span class="icon">📋</span>No wipe jobs yet. Plug in a USB device to begin.</div>' : `
        <div style="max-height:400px;overflow-y:auto">
          <table>
            <thead><tr><th>Device</th><th>Scheme</th><th>Status</th><th>Progress</th><th>Created</th></tr></thead>
            <tbody>${jobs.slice(0, 20).map(j => {
              const ps = progressMap.get(j.devicePath);
              const dispProgress = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.percent : (j.progress || 0);
              const dispPass = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.currentPass : j.currentPass;
              const dispTotal = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.totalPasses : j.totalPasses;
              return `
              <tr data-device="${escapeAttr(j.devicePath)}">
                <td style="font-family:var(--font-mono)">${escapeHtml(j.devicePath)}</td>
                <td>${escapeHtml(j.schemeId)}</td>
                <td><span class="badge ${jobBadgeClass(j.status)}">${escapeHtml(j.status)}</span></td>
                <td class="progress-cell">
                  ${j.status === 'running' || j.status === 'verifying' ? `
                    <div style="display:flex;align-items:center;gap:8px">
                      <progress value="${dispProgress}" max="100"></progress>
                      <span class="progress-pct" style="font-size:.78rem;font-weight:600">${dispProgress.toFixed(1)}%</span>
                      ${dispTotal > 1 ? `<span class="progress-pass" style="font-size:.72rem;color:var(--color-text-dim)">Pass ${dispPass}/${dispTotal}</span>` : ''}
                    </div>` : j.status === 'completed' ? '100%' : '—'}
                </td>
                <td style="font-size:.78rem;color:var(--color-text-dim)">${new Date(j.createdAt).toLocaleString()}</td>
              </tr>`}).join('')}</tbody>
          </table>
        </div>
      `}
    </div>
    ${running.length > 0 ? `
      <div class="card">
        <div class="card-header">
          <h2>Active Jobs</h2>
          <button class="btn btn-danger btn-sm" id="btn-dash-cancel-all">Cancel All</button>
        </div>
        ${running.map(jobCard).join('')}
      </div>
    ` : ''}
  `;

  const btnCancel = document.getElementById('btn-dash-cancel-all');
  if (btnCancel) {
    btnCancel.onclick = async () => {
      showConfirm('Cancel ALL active wipe jobs?', {
        dangerLabel: 'Cancel All',
        onConfirm: async () => {
          try {
            await apiPost('/api/cancel');
            showToast('Cancelling all jobs...', 'warning');
          } catch (e) { showToast(e.message, 'error'); }
        }
      });
    };
  }

  el.querySelectorAll('.dash-device-row').forEach(row => {
    row.onclick = (e) => {
      if (e.target.closest('button')) return;
      const device = devices.find(d => d.path === row.dataset.device);
      if (device) openDrawer(device);
    };
  });

  el.querySelectorAll('.dash-smart-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const device = devices.find(d => d.path === btn.dataset.device);
      if (device) openDrawer(device, 'smart');
    };
  });

  el.querySelectorAll('.dash-cancel-job').forEach(btn => {
    btn.onclick = async () => {
      try {
        await apiPost('/api/jobs/' + encodeURIComponent(btn.dataset.job) + '/cancel');
        showToast('Cancelling ' + btn.dataset.device + '...', 'info');
      } catch (e) {
        showToast(e.message, 'error');
      }
    };
  });
}

function deviceSummaryCard() {
  if (devices.length === 0) {
    return `
      <div class="card">
        <div class="card-header"><h2>Detected Devices</h2></div>
        <div class="empty-state"><span class="icon">💾</span>No USB devices detected.</div>
      </div>`;
  }

  const deviceJobMap = {};
  jobs.forEach(j => {
    if (j.status === 'running' || j.status === 'verifying' || j.status === 'formatting' || j.status === 'queued') {
      deviceJobMap[j.devicePath] = j;
    }
  });

  return `
    <div class="card">
      <div class="card-header"><h2>Detected Devices</h2></div>
      <div class="table-scroll">
        <table>
          <thead><tr><th>Device</th><th>Model</th><th>Size</th><th>Status</th><th>SMART</th></tr></thead>
          <tbody>${devices.map(d => {
            const job = deviceJobMap[d.path];
            return `
              <tr class="clickable dash-device-row" data-device="${escapeAttr(d.path)}">
                <td style="font-family:var(--font-mono)">${escapeHtml(d.path)}</td>
                <td>${escapeHtml(d.model || '—')}</td>
                <td>${formatBytes(d.sizeBytes)}</td>
                <td>${deviceStatusBadge(d, job)}</td>
                <td><button class="btn btn-sm btn-ghost dash-smart-btn" data-device="${escapeAttr(d.path)}">SMART</button></td>
              </tr>`;
          }).join('')}</tbody>
        </table>
      </div>
    </div>`;
}

function jobCard(j) {
  const ps = progressMap.get(j.devicePath);
  const dispProgress = ps ? ps.percent : (j.progress || 0);
  const dispPass = ps ? ps.currentPass : j.currentPass;
  const dispTotal = ps ? ps.totalPasses : j.totalPasses;
  return `
    <div class="job-card" data-device="${escapeAttr(j.devicePath)}">
      <div class="job-card-header">
        <div>
          <span class="job-card-device">${j.devicePath}</span>
          <span class="job-card-scheme ml-2">${j.schemeId}${dispTotal > 1 ? ` · <span class="progress-pass">Pass ${dispPass}/${dispTotal}</span>` : ''}</span>
        </div>
        <span class="badge ${jobBadgeClass(j.status)}">${j.status}</span>
      </div>
      <div class="progress-multi mb-2">
        ${dispTotal > 1 ? multiPassBarSegments(dispPass, dispTotal, dispProgress) : `<div class="progress-segment active" style="width:${Math.max(dispProgress, 1)}%"></div><div class="progress-segment pending" style="width:${100 - Math.max(dispProgress, 1)}%"></div>`}
      </div>
      <div class="progress-info">
        <span class="progress-pct">${dispProgress.toFixed(1)}%</span>
        ${j.label ? `<span class="badge badge-info">${j.label}</span>` : ''}
        <button class="btn btn-ghost btn-sm dash-cancel-job" style="margin-left:auto" data-job="${escapeAttr(j.id)}" data-device="${escapeAttr(j.devicePath)}">Cancel</button>
      </div>
    </div>
  `;
}

function multiPassBarSegments(currentPass, totalPasses, overallPercent) {
  let html = '';
  const perPass = 100 / totalPasses;
  for (let p = 1; p <= totalPasses; p++) {
    let cls = 'pending';
    let w = perPass;
    if (p < currentPass) cls = 'completed';
    else if (p === currentPass) {
      cls = 'active';
      // Width is percent consumed within the current pass only.
      w = overallPercent - ((currentPass - 1) * perPass);
      w = Math.max(1, Math.min(w, perPass));
    }
    html += `<div class="progress-segment ${cls}" style="width:${w}%"></div>`;
  }
  return html;
}

function escapeAttr(s) {
  return s.replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function deviceStatusBadge(device, job) {
  if (job) return `<span class="badge ${jobBadgeClass(job.status)}">${escapeHtml(job.status)}</span>`;
  if (device.wipeBlocked) return `<span class="badge badge-warning" title="${escapeAttr(device.blockReason || '')}">Blocked</span>`;
  if (device.wipeHistory && device.wipeHistory.status === 'completed') {
    const ok = device.wipeHistory.verification === 'passed';
    return `<span class="badge ${ok ? 'badge-success' : 'badge-danger'}">${ok ? 'Wiped ✓' : 'Wiped ✗'}</span>`;
  }
  return '<span class="badge badge-success">Ready</span>';
}

function jobBadgeClass(status) {
  const m = {
    running: 'badge-warning', verifying: 'badge-info', formatting: 'badge-info',
    completed: 'badge-success', failed: 'badge-danger', cancelled: 'badge-neutral', queued: 'badge-neutral'
  };
  return m[status] || 'badge-neutral';
}

function updateDashboard(d, j) {
  devices = d;
  jobs = j;
  renderDashboard();
}

import { apiPost } from '../api.js';
import { openDrawer } from '../components/drawer.js';
import { showToast, showConfirm } from '../components/toast.js';

export { renderDashboard, updateDashboard };
