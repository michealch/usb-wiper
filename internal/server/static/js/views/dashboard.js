// Dashboard view — instrument panel layout
import { progressMap } from '../app.js';
import { apiPost } from '../api.js';
import { openDrawer } from '../components/drawer.js';
import { showToast, showConfirm } from '../components/toast.js';
import { escapeHtml, escapeAttr, formatBytes, countUp } from '../util.js';

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
    <div class="dash-grid">
      <div class="dash-left">
        ${activeJobsPanel(running)}
        <div class="kpi-grid">
          <div class="kpi">
            <div class="kpi-value">${devices.length}</div>
            <div class="kpi-label">Devices</div>
          </div>
          <div class="kpi">
            <div class="kpi-value">${running.length}</div>
            <div class="kpi-label">Running</div>
          </div>
          <div class="kpi">
            <div class="kpi-value">${queued.length}</div>
            <div class="kpi-label">Queued</div>
          </div>
          <div class="kpi">
            <div class="kpi-value">${completedToday.length}</div>
            <div class="kpi-label">Done Today</div>
          </div>
        </div>

        ${queued.length > 0 ? `
          <div class="card">
            <div class="card-header"><h2>Pending</h2></div>
            ${queued.map(queuedJobCard).join('')}
          </div>
        ` : ''}
      </div>

      <div class="dash-right">
        ${deviceSummaryCard()}
        <div class="card">
          <div class="card-header"><h2>Recent Activity</h2></div>
          ${jobs.length === 0 ? `<div class="empty-state"><span class="empty-state-icon">&mdash;</span>No wipe jobs yet. Plug in a USB device to begin.</div>` : `
            <div class="table-scroll table-stack activity-table-wrap">
              <table>
                <thead><tr><th>Device</th><th>Scheme</th><th>Status</th><th>Progress</th><th>Created</th></tr></thead>
                <tbody>${jobs.slice(0, 30).map(activityRow).join('')}</tbody>
              </table>
            </div>
          `}
        </div>
      </div>
    </div>
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

  // Animate KPI numbers on render (snaps under reduced-motion).
  el.querySelectorAll('.kpi-value').forEach(v => {
    const target = parseInt(v.textContent, 10) || 0;
    v.textContent = '0';
    countUp(v, target);
  });
}

function deviceSummaryCard() {
  if (devices.length === 0) {
    return `
      <div class="card">
        <div class="card-header"><h2>Detected Devices</h2></div>
        <div class="empty-state"><span class="empty-state-icon">&mdash;</span>No USB devices detected.</div>
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
      <div class="table-scroll table-stack dash-device-table-wrap">
        <table>
          <thead><tr><th scope="col">Device</th><th scope="col">Model</th><th scope="col">Size</th><th scope="col">Status</th><th scope="col"></th></tr></thead>
          <tbody>${devices.map(d => {
            const job = deviceJobMap[d.path];
            return `
              <tr class="clickable dash-device-row" data-device="${escapeAttr(d.path)}">
                <td data-label="Device" style="font-family:var(--font-mono)">${escapeHtml(d.path)}</td>
                <td data-label="Model">${escapeHtml(d.model || '\u2014')}</td>
                <td data-label="Size">${formatBytes(d.sizeBytes)}</td>
                <td data-label="Status">${deviceStatusBadge(d, job)}</td>
                <td data-label="Details"><button class="btn btn-sm btn-ghost dash-smart-btn" data-device="${escapeAttr(d.path)}">SMART</button></td>
              </tr>`;
          }).join('')}</tbody>
        </table>
      </div>
    </div>`;
}

function activityRow(j) {
  const ps = progressMap.get(j.devicePath);
  const dispProgress = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.percent : (j.progress || 0);
  const dispPass = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.currentPass : j.currentPass;
  const dispTotal = (ps && (j.status === 'running' || j.status === 'verifying')) ? ps.totalPasses : j.totalPasses;
  return `
    <tr data-device="${escapeAttr(j.devicePath)}">
      <td data-label="Device" style="font-family:var(--font-mono)">${escapeHtml(j.devicePath)}</td>
      <td data-label="Scheme">${escapeHtml(j.schemeId)}</td>
      <td data-label="Status"><span class="badge ${jobBadgeClass(j.status)}">${escapeHtml(j.status)}</span></td>
      <td data-label="Progress" class="progress-cell">
        ${j.status === 'running' || j.status === 'verifying' ? `
          <div style="display:flex;align-items:center;gap:8px">
            <progress value="${dispProgress}" max="100" aria-valuemin="0" aria-valuemax="100"></progress>
            <span class="progress-pct" style="font-size:.78rem;font-weight:600">${dispProgress.toFixed(1)}%</span>
            ${dispTotal > 1 ? `<span class="progress-pass" style="font-size:.72rem;color:var(--color-text-dim)">Pass ${dispPass}/${dispTotal}</span>` : ''}
          </div>` : j.status === 'completed' ? '100%' : '\u2014'}
      </td>
      <td data-label="Created" style="font-size:.78rem;color:var(--color-text-dim)">${new Date(j.createdAt).toLocaleString()}</td>
    </tr>`;
}

function activeJobsPanel(running) {
  return `
    <div class="card active-jobs-panel">
      <div class="card-header">
        <h2>Active Wipes</h2>
        <div class="active-jobs-actions">
          <span class="badge ${running.length > 0 ? 'badge-warning' : 'badge-neutral'}">${running.length} active</span>
          ${running.length > 0 ? `<button class="btn btn-danger btn-sm" id="btn-dash-cancel-all">Cancel All</button>` : ''}
        </div>
      </div>
      ${running.length > 0 ? `<div class="stagger">${running.map((j, i) => i === 0 ? heroJobCard(j) : jobCard(j)).join('')}</div>` : `
        <div class="empty-state compact">
          <span class="empty-state-icon">&mdash;</span>
          <div class="empty-state-title">No active wipes</div>
          <div>Ready for the next device.</div>
        </div>
      `}
    </div>
  `;
}

function heroJobCard(j) {
  const ps = progressMap.get(j.devicePath);
  const dispProgress = ps ? ps.percent : (j.progress || 0);
  const dispPass = ps ? ps.currentPass : j.currentPass;
  const dispTotal = ps ? ps.totalPasses : j.totalPasses;
  return `
    <div class="job-card wipe-hero-card" data-device="${escapeAttr(j.devicePath)}">
      <div class="wipe-hero">
        <div class="wipe-gauge" data-gauge style="--pct:${dispProgress.toFixed(1)}">
          <div class="wipe-gauge-inner">
            <div class="wipe-gauge-pct progress-pct">${dispProgress.toFixed(0)}%</div>
            <div class="wipe-gauge-sub">${escapeHtml(j.status)}</div>
          </div>
        </div>
        <div class="wipe-hero-meta">
          <div class="wipe-hero-device">${escapeHtml(j.devicePath)}</div>
          <div class="wipe-hero-scheme">${escapeHtml(j.schemeId)}${dispTotal > 1 ? ` &middot; <span class="progress-pass">Pass ${dispPass}/${dispTotal}</span>` : ''}</div>
          ${j.label ? `<div class="mt-2"><span class="badge badge-info">${escapeHtml(j.label)}</span></div>` : ''}
          <div class="wipe-hero-actions">
            <button class="btn btn-danger btn-sm dash-cancel-job" data-job="${escapeAttr(j.id)}" data-device="${escapeAttr(j.devicePath)}">Cancel wipe</button>
          </div>
        </div>
      </div>
    </div>
  `;
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
          <span class="job-card-device">${escapeHtml(j.devicePath)}</span>
          <span class="job-card-scheme">${escapeHtml(j.schemeId)}${dispTotal > 1 ? ` &middot; <span class="progress-pass">Pass ${dispPass}/${dispTotal}</span>` : ''}</span>
        </div>
        <span class="badge ${jobBadgeClass(j.status)}">${escapeHtml(j.status)}</span>
      </div>
      <div class="progress-multi mb-2">
        ${dispTotal > 1 ? multiPassBarSegments(dispPass, dispTotal, dispProgress) : `<div class="progress-segment active" style="width:${Math.max(dispProgress, 1)}%"></div><div class="progress-segment pending" style="width:${100 - Math.max(dispProgress, 1)}%"></div>`}
      </div>
      <div class="progress-info">
        <span class="progress-pct">${dispProgress.toFixed(1)}%</span>
        ${j.label ? `<span class="badge badge-info">${escapeHtml(j.label)}</span>` : ''}
        <button class="btn btn-ghost btn-sm dash-cancel-job" style="margin-left:auto" data-job="${escapeAttr(j.id)}" data-device="${escapeAttr(j.devicePath)}">Cancel</button>
      </div>
    </div>
  `;
}

function queuedJobCard(j) {
  return `
    <div class="job-card" data-device="${escapeAttr(j.devicePath)}">
      <div class="job-card-header">
        <div>
          <span class="job-card-device">${escapeHtml(j.devicePath)}</span>
          <span class="job-card-scheme">${escapeHtml(j.schemeId)}${j.label ? ' &middot; ' + escapeHtml(j.label) : ''}</span>
        </div>
        <span class="badge badge-neutral">Queued</span>
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
      w = overallPercent - ((currentPass - 1) * perPass);
      w = Math.max(1, Math.min(w, perPass));
    }
    html += `<div class="progress-segment ${cls}" style="width:${w}%"></div>`;
  }
  return html;
}

function deviceStatusBadge(device, job) {
  if (job) return `<span class="badge ${jobBadgeClass(job.status)}">${escapeHtml(job.status)}</span>`;
  if (device.wipeBlocked) return `<span class="badge badge-warning" title="${escapeAttr(device.blockReason || '')}">Blocked</span>`;
  if (device.wipeHistory && device.wipeHistory.status === 'completed') {
    const ok = device.wipeHistory.verification === 'passed';
    return `<span class="badge ${ok ? 'badge-success' : 'badge-danger'}">${ok ? 'Wiped' : 'Verify failed'}</span>`;
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

export { renderDashboard, updateDashboard };
