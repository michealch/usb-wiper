// Queue view

let allJobs = [];

function renderQueue() {
  const el = document.getElementById('view-queue');
  const running = allJobs.filter(j => j.status === 'running' || j.status === 'verifying' || j.status === 'formatting');
  const queued = allJobs.filter(j => j.status === 'queued');
  const recent = allJobs.filter(j => j.status === 'completed' || j.status === 'failed' || j.status === 'cancelled').slice(0, 10);

  el.innerHTML = `
    <div class="card">
      <div class="card-header">
        <h2>Running (${running.length})</h2>
        ${running.length > 0 ? '<button class="btn btn-danger btn-sm" id="btn-cancel-all">Cancel All</button>' : ''}
      </div>
      ${running.length === 0 ? '<p class="muted">No active jobs.</p>' : running.map(renderJobCard).join('')}
    </div>
    <div class="card">
      <div class="card-header"><h2>Pending (${queued.length})</h2></div>
      ${queued.length === 0 ? '<p class="muted">No queued jobs.</p>' : queued.map(renderJobCard).join('')}
    </div>
    <div class="card">
      <div class="card-header"><h2>Recently Finished</h2></div>
      ${recent.length === 0 ? '<p class="muted">No completed jobs.</p>' : `
        <table>
          <thead><tr><th>Device</th><th>Scheme</th><th>Status</th><th>Time</th></tr></thead>
          <tbody>${recent.map(j => `
            <tr>
              <td style="font-family:var(--font-mono)">${j.devicePath}</td><td>${j.schemeId}</td>
              <td><span class="badge ${j.status === 'completed' ? 'badge-success' : j.status === 'failed' ? 'badge-danger' : 'badge-neutral'}">${j.status}</span></td>
              <td style="font-size:.78rem;color:var(--color-text-dim)">${j.completedAt ? new Date(j.completedAt).toLocaleString() : '—'}</td>
            </tr>`).join('')}</tbody>
        </table>
      `}
    </div>
  `;

  const btnCancel = document.getElementById('btn-cancel-all');
  if (btnCancel) {
    btnCancel.onclick = async () => {
      showConfirm('Cancel ALL active and queued jobs?', {
        dangerLabel: 'Cancel All',
        onConfirm: async () => {
          try { await apiPost('/api/cancel'); showToast('Cancelled all jobs', 'warning'); }
          catch (e) { showToast(e.message, 'error'); }
        }
      });
    };
  }

  // Bind per-job cancel buttons
  el.querySelectorAll('.job-cancel-btn').forEach(btn => {
    btn.onclick = async (e) => {
      e.stopPropagation();
      const id = btn.dataset.jobId;
      try { await apiPost('/api/jobs/' + id + '/cancel'); showToast('Job cancelled', 'warning'); }
      catch (e) { showToast(e.message, 'error'); }
    };
  });
}

function renderJobCard(j) {
  return `
    <div class="job-card">
      <div class="job-card-header">
        <div>
          <span class="job-card-device">${j.devicePath}</span>
          <span class="job-card-scheme">${j.schemeId}${j.label ? ' · ' + j.label : ''}</span>
        </div>
        <span class="badge ${j.status === 'running' ? 'badge-warning' : j.status === 'verifying' ? 'badge-info' : 'badge-neutral'}">${j.status}</span>
      </div>
      ${j.totalPasses > 1 ? `<div class="progress-multi mb-2">${multiPassSegments(j)}</div>` : `<progress value="${j.progress || 0}" max="100" style="margin-bottom:8px"></progress>`}
      <div class="progress-info">
        <span>${(j.progress || 0).toFixed(1)}%</span>
        ${j.totalPasses > 1 ? `<span>Pass ${j.currentPass}/${j.totalPasses}</span>` : ''}
        <button class="btn btn-ghost btn-sm job-cancel-btn" data-job-id="${j.id}" style="margin-left:auto">Cancel</button>
      </div>
    </div>
  `;
}

function multiPassSegments(j) {
  let h = '';
  for (let p = 1; p <= j.totalPasses; p++) {
    let cls = 'pending';
    if (p < j.currentPass) cls = 'completed';
    else if (p === j.currentPass) cls = 'active';
    h += `<div class="progress-segment ${cls}" style="width:${100/j.totalPasses}%"></div>`;
  }
  return h;
}

function updateQueue(jobs) {
  allJobs = jobs;
  renderQueue();
}

import { apiPost } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';

export { renderQueue, updateQueue };
