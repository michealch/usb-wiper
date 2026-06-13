// Queue view
import { progressMap } from '../app.js';
import { apiPost } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';
import { renderCertMenu, initCertMenus } from '../components/cert.js';
import { escapeHtml, escapeAttr } from '../util.js';

let allJobs = [];

function renderQueue() {
  const el = document.getElementById('view-queue');
  const running = allJobs.filter(j => j.status === 'running' || j.status === 'verifying' || j.status === 'formatting');
  const queued = allJobs.filter(j => j.status === 'queued');
  const recent = allJobs.filter(j => j.status === 'completed' || j.status === 'failed' || j.status === 'cancelled').slice(0, 10);

  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Queue</div>
          <div class="page-subtitle">${running.length + queued.length} active workload${running.length + queued.length === 1 ? '' : 's'}</div>
        </div>
        <div class="page-actions">
          ${running.length + queued.length > 0 ? '<button class="btn btn-danger btn-sm" id="btn-cancel-all">Cancel All</button>' : ''}
        </div>
      </div>

      <div class="metric-strip">
        <div class="metric-pill"><span class="metric-label">Running</span><span class="metric-value">${running.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Pending</span><span class="metric-value">${queued.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Finished</span><span class="metric-value">${recent.length}</span></div>
      </div>

      <div class="split-grid">
        <div class="stack">
          <section class="panel">
            <div class="panel-header"><div class="panel-title">Running</div></div>
            ${running.length === 0 ? emptyState('No active jobs.') : running.map(renderJobCard).join('')}
          </section>
          <section class="panel">
            <div class="panel-header"><div class="panel-title">Pending</div></div>
            ${queued.length === 0 ? emptyState('No queued jobs.') : queued.map(renderJobCard).join('')}
          </section>
        </div>

        <section class="panel">
          <div class="panel-header">
            <div class="panel-title">Recently Finished</div>
            <div class="panel-note">${recent.length} shown</div>
          </div>
          ${recent.length === 0 ? emptyState('No completed jobs.') : `
            <div class="table-scroll table-stack">
              <table class="record-table">
                <thead><tr><th scope="col">Device</th><th scope="col">Status</th><th scope="col">Time</th><th scope="col"></th></tr></thead>
                <tbody>${recent.map(j => `
                  <tr>
                    <td data-label="Device">
                      <div class="record-primary text-mono">${escapeHtml(j.devicePath)}</div>
                      <div class="record-secondary">${escapeHtml(j.schemeId)}${j.label ? ' · ' + escapeHtml(j.label) : ''}</div>
                    </td>
                    <td data-label="Status"><span class="badge ${statusBadge(j.status)}">${escapeHtml(j.status)}</span></td>
                    <td data-label="Time" style="font-size:var(--text-xs);color:var(--color-text-dim)">${j.completedAt ? new Date(j.completedAt).toLocaleString() : '-'}</td>
                    <td data-label=""><div class="record-actions">${j.status === 'completed' ? renderCertMenu(j.id) : ''}</div></td>
                  </tr>`).join('')}</tbody>
              </table>
            </div>
          `}
        </section>
      </div>
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

  initCertMenus(el);
}

function emptyState(text) {
  return `<div class="empty-state"><div class="empty-state-title">${escapeHtml(text)}</div></div>`;
}

function statusBadge(status) {
  if (status === 'completed') return 'badge-success';
  if (status === 'failed') return 'badge-danger';
  if (status === 'running' || status === 'verifying' || status === 'formatting') return 'badge-warning';
  return 'badge-neutral';
}

function renderJobCard(j) {
  const ps = progressMap.get(j.devicePath);
  const dispProgress = ps ? ps.percent : (j.progress || 0);
  const dispPass = ps ? ps.currentPass : j.currentPass;
  const dispTotal = ps ? ps.totalPasses : j.totalPasses;
  return `
    <div class="job-card" data-device="${escapeAttr(j.devicePath)}">
      <div class="job-card-header">
        <div>
          <span class="job-card-device">${escapeHtml(j.devicePath)}</span>
          <span class="job-card-scheme">${escapeHtml(j.schemeId)}${j.label ? ' · ' + escapeHtml(j.label) : ''}</span>
        </div>
        <span class="badge ${j.status === 'running' ? 'badge-warning' : j.status === 'verifying' ? 'badge-info' : 'badge-neutral'}">${escapeHtml(j.status)}</span>
      </div>
      ${dispTotal > 1 ? `<div class="progress-multi mb-2">${multiPassSegments(dispPass, dispTotal, dispProgress)}</div>` : `<progress value="${dispProgress}" max="100" aria-valuemin="0" aria-valuemax="100" style="margin-bottom:8px"></progress>`}
      <div class="progress-info">
        <span class="progress-pct">${dispProgress.toFixed(1)}%</span>
        ${dispTotal > 1 ? `<span class="progress-pass">Pass ${dispPass}/${dispTotal}</span>` : ''}
        <button class="btn btn-ghost btn-sm job-cancel-btn" data-job-id="${escapeAttr(j.id)}" style="margin-left:auto">Cancel</button>
      </div>
    </div>
  `;
}

function multiPassSegments(currentPass, totalPasses, overallPercent) {
  const perPass = 100 / totalPasses;
  let h = '';
  for (let p = 1; p <= totalPasses; p++) {
    let cls = 'pending';
    let w = perPass;
    if (p < currentPass) cls = 'completed';
    else if (p === currentPass) {
      cls = 'active';
      w = overallPercent - ((currentPass - 1) * perPass);
      w = Math.max(1, Math.min(w, perPass));
    }
    h += `<div class="progress-segment ${cls}" style="width:${w}%"></div>`;
  }
  return h;
}

function updateQueue(jobs) {
  allJobs = jobs;
  renderQueue();
}

export { renderQueue, updateQueue };
