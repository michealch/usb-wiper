// Devices view — table with expandable drawer integration

let devicesList = [];
let jobsList = [];

function renderDevices() {
  const el = document.getElementById('view-devices');
  
  if (devicesList.length === 0) {
    el.innerHTML = `
      <div class="card">
        <div class="card-header">
          <h2>Devices</h2>
          <button class="btn btn-sm" id="btn-refresh-devices">Refresh</button>
        </div>
        <div class="empty-state"><span class="icon">💾</span>No USB devices detected. Insert a USB drive and click Refresh.</div>
      </div>`;
    document.getElementById('btn-refresh-devices').onclick = refreshDevices;
    return;
  }

  const deviceJobMap = {};
  jobsList.forEach(j => {
    if (j.status === 'running' || j.status === 'verifying' || j.status === 'formatting' || j.status === 'queued') {
      deviceJobMap[j.devicePath] = j;
    }
  });

  el.innerHTML = `
    <div class="card">
      <div class="card-header">
        <h2>Devices (${devicesList.length})</h2>
        <div class="btn-group">
          <button class="btn btn-sm" id="btn-refresh-devices">Refresh</button>
          <button class="btn btn-sm btn-danger" id="btn-wipe-all" ${devicesList.filter(d => !d.wipeBlocked && !deviceJobMap[d.path]).length === 0 ? 'disabled' : ''}>
            Wipe All Eligible
          </button>
        </div>
      </div>
      <table>
        <thead>
          <tr>
            <th>Path</th>
            <th>Model</th>
            <th>Size</th>
            <th>Status</th>
            <th>Progress</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          ${devicesList.map(d => {
            const job = deviceJobMap[d.path];
            let statusHtml = '';
            if (job) {
              const badge = job.status === 'running' ? 'badge-warning' : job.status === 'verifying' ? 'badge-info' : 'badge-neutral';
              statusHtml = `<span class="badge ${badge}">${job.status}</span>`;
            } else if (d.wipeBlocked) {
              statusHtml = `<span class="badge badge-warning" title="${d.blockReason || ''}">Blocked</span>`;
            } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
              const v = d.wipeHistory.verification;
              statusHtml = `<span class="badge ${v === 'passed' ? 'badge-success' : 'badge-danger'}">${v === 'passed' ? 'Wiped ✓' : 'Wiped ✗'}</span>`;
            } else {
              statusHtml = `<span class="badge badge-success">Ready</span>`;
            }

            let progressHtml = '—';
            if (job && (job.status === 'running' || job.status === 'verifying')) {
              progressHtml = `
                <div style="display:flex;align-items:center;gap:8px">
                  <progress value="${job.progress || 0}" max="100"></progress>
                  <span class="progress-pct" style="font-size:.78rem;font-weight:600">${(job.progress || 0).toFixed(1)}%</span>
                  ${job.totalPasses > 1 ? `<span class="progress-pass" style="font-size:.72rem;color:var(--color-text-dim)">Pass ${job.currentPass}/${job.totalPasses}</span>` : ''}
                </div>`;
            } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
              progressHtml = '<span style="color:var(--color-success);font-weight:600">100%</span>';
            }

            return `
              <tr class="clickable" data-device="${escapeAttr(d.path)}">
                <td style="font-family:var(--font-mono)">${d.path}</td>
                <td>${d.model || '—'}</td>
                <td>${formatBytes(d.sizeBytes)}</td>
                <td>${statusHtml}</td>
                <td class="progress-cell">${progressHtml}</td>
                <td>
                  <div class="btn-group">
                    <button class="btn btn-sm btn-danger dev-wipe-btn" data-device="${escapeAttr(d.path)}" ${d.wipeBlocked || job ? 'disabled' : ''}>
                      ${d.wipeBlocked ? 'Blocked' : job ? 'Active' : 'Wipe'}
                    </button>
                    <button class="btn btn-sm btn-success dev-test-btn" data-device="${escapeAttr(d.path)}" ${d.wipeBlocked || job ? 'disabled' : ''}>
                      Test
                    </button>
                  </div>
                </td>
              </tr>`;
          }).join('')}
        </tbody>
      </table>
    </div>
  `;

  // Bind row clicks → open drawer
  el.querySelectorAll('tr.clickable').forEach(row => {
    row.onclick = (e) => {
      if (e.target.closest('button')) return;
      const path = row.dataset.device;
      const device = devicesList.find(d => d.path === path);
      if (device) openDrawer(device);
    };
  });

  // Bind wipe buttons
  el.querySelectorAll('.dev-wipe-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const path = btn.dataset.device;
      const device = devicesList.find(d => d.path === path);
      if (device) openDrawer(device);
    };
  });

  // Bind test buttons
  el.querySelectorAll('.dev-test-btn').forEach(btn => {
    btn.onclick = async (e) => {
      e.stopPropagation();
      const path = btn.dataset.device;
      try {
        await apiPost('/api/test-wipe', { device: path, verifySizeGB: 1 });
        showToast('Test wipe started on ' + path, 'info');
      } catch (err) { showToast(err.message, 'error'); }
    };
  });

  document.getElementById('btn-refresh-devices').onclick = refreshDevices;
  document.getElementById('btn-wipe-all').onclick = wipeAllEligible;
}

function updateDevices(d, j) {
  devicesList = d;
  jobsList = j;
  renderDevices();
}

async function refreshDevices() {
  const data = await apiGet('/api/devices');
  devicesList = data.devices || [];
  renderDevices();
}

async function wipeAllEligible() {
  const eligible = devicesList.filter(d => !d.wipeBlocked && !jobsList.some(j => j.devicePath === d.path && (j.status === 'running' || j.status === 'queued')));
  if (eligible.length === 0) {
    showToast('No eligible devices to wipe', 'warning');
    return;
  }
  const paths = eligible.map(d => d.path).join(', ');
  showConfirm(`Wipe ALL ${eligible.length} eligible devices?\n\n${paths}\n\nTHIS DESTROYS ALL DATA. Cannot be undone.`, {
    dangerLabel: 'Wipe All',
    onConfirm: async () => {
      try {
        const result = await apiPost('/api/wipe', { devices: eligible.map(d => d.path), schemeId: 'zero' });
        showToast(`Started wiping ${result.started.length} devices`, 'success');
        if (result.conflicts.length > 0) showToast(`${result.conflicts.length} devices skipped (already active)`, 'warning');
      } catch (err) { showToast(err.message, 'error'); }
    }
  });
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function escapeAttr(s) {
  return s.replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

import { apiGet, apiPost } from '../api.js';
import { openDrawer } from '../components/drawer.js';
import { showToast, showConfirm } from '../components/toast.js';

export { renderDevices, updateDevices, refreshDevices };
