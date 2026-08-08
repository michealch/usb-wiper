// Wipe view — the single screen for attaching, selecting, and wiping devices.
// Replaces the old Dashboard / Devices / Queue screens: every wipe flows through
// openConfigurator(), and progress is rendered as the segmented pass bar.

import { apiGet, apiPost } from '../api.js';
import { progressMap, refreshCurrentView } from '../app.js';
import { openDrawer } from '../components/drawer.js';
import { openConfigurator } from '../components/configurator.js';
import { showToast, showConfirm } from '../components/toast.js';
import { renderCertMenu, initCertMenus } from '../components/cert.js';
import { escapeHtml, escapeAttr, formatBytes, formatDateTime, badgeClassForStatus,
  deviceStatusBadge, progressBar, emptyState, activeJobByPath } from '../util.js';

let devicesList = [];
let jobsList = [];
let selected = new Set();

function isSelectable(d, deviceJobMap) {
  return !d.wipeBlocked && !deviceJobMap.get(d.path);
}

function healthBadges(d) {
  const h = d.healthLatest;
  if (!h) return '';
  let out = '';
  if (h.healthStatus === 'FAILED') out += '<span class="badge badge-danger">SMART failed</span>';
  if (Number(h.enduranceUsedPct) >= 90) out += `<span class="badge badge-warning">Wear ${Number(h.enduranceUsedPct)}%</span>`;
  return out ? `<div class="mt-1">${out}</div>` : '';
}

function jobCard(j) {
  return `
    <div class="job-card" data-device="${escapeAttr(j.devicePath)}">
      <div class="job-card-header">
        <div>
          <span class="job-card-device">${escapeHtml(j.devicePath)}</span>
          <span class="job-card-scheme">${escapeHtml(j.schemeId)}${j.label ? ' · ' + escapeHtml(j.label) : ''}</span>
        </div>
        <span class="badge ${badgeClassForStatus(j.status)}">${escapeHtml(j.status)}</span>
      </div>
      ${progressBar(j, progressMap)}
      <div class="btn-group">
        <button class="btn btn-ghost btn-sm job-cancel" data-job="${escapeAttr(j.id)}"
                data-device="${escapeAttr(j.devicePath)}">Cancel</button>
      </div>
    </div>`;
}

function renderSelectionBar() {
  if (selected.size === 0) return '';
  return `
    <div class="selection-bar">
      <span class="selection-count" aria-live="polite">${selected.size} selected</span>
      <div class="btn-group">
        <button class="btn btn-sm btn-danger" id="sel-wipe">Wipe ${selected.size} device${selected.size !== 1 ? 's' : ''}</button>
        <button class="btn btn-sm" id="sel-clear">Clear</button>
      </div>
    </div>`;
}

function renderWipe() {
  const el = document.getElementById('view-wipe');

  const active   = jobsList.filter(j => j.status === 'running' || j.status === 'verifying' || j.status === 'formatting');
  const queued   = jobsList.filter(j => j.status === 'queued');
  const finished = jobsList.filter(j => j.status === 'completed' || j.status === 'failed' || j.status === 'cancelled').slice(0, 10);
  const doneToday = jobsList.filter(j => j.status === 'completed' && j.completedAt &&
    new Date(j.completedAt).toDateString() === new Date().toDateString());
  const deviceJobMap = activeJobByPath(jobsList);

  // Prune selections that are no longer valid (removed, blocked, or now active).
  for (const path of Array.from(selected)) {
    const d = devicesList.find(dd => dd.path === path);
    if (!d || !isSelectable(d, deviceJobMap)) selected.delete(path);
  }

  const selectableDevices = devicesList.filter(d => isSelectable(d, deviceJobMap));
  const allSelected = selectableDevices.length > 0 && selectableDevices.every(d => selected.has(d.path));
  const eligible = devicesList.filter(d => isSelectable(d, deviceJobMap));

  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Wipe</div>
          <div class="page-subtitle">${devicesList.length} attached · ${eligible.length} eligible</div>
        </div>
        <div class="page-actions">
          <button class="btn btn-sm" id="wipe-refresh">Refresh</button>
          ${active.length + queued.length > 0 ? '<button class="btn btn-sm btn-danger" id="wipe-cancel-all">Cancel all</button>' : ''}
        </div>
      </div>

      <div class="metric-strip">
        <div class="metric-pill"><span class="metric-label">Attached</span><span class="metric-value">${devicesList.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Running</span><span class="metric-value">${active.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Queued</span><span class="metric-value">${queued.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Done today</span><span class="metric-value">${doneToday.length}</span></div>
      </div>

      ${active.length + queued.length > 0 ? `
        <section class="panel">
          <div class="panel-header">
            <div class="panel-title">In progress</div>
            <div class="panel-note">${active.length} running · ${queued.length} queued</div>
          </div>
          <div class="stagger">
            ${active.map(jobCard).join('')}
            ${queued.map(jobCard).join('')}
          </div>
        </section>
      ` : ''}

      <section class="panel">
        <div class="panel-header">
          <div class="panel-title">Devices</div>
          <div class="panel-note">Click a row for details</div>
        </div>
        ${devicesList.length === 0 ? emptyState('No USB devices detected.', 'Attach a drive, then press Refresh.') : `
          <div class="table-scroll table-stack">
            <table class="record-table">
              <thead>
                <tr>
                  <th scope="col" class="select-col"><input type="checkbox" class="sel-all-cb" aria-label="Select all eligible devices" ${selectableDevices.length === 0 ? 'disabled' : ''} ${allSelected ? 'checked' : ''}></th>
                  <th scope="col">Device</th>
                  <th scope="col">Size</th>
                  <th scope="col">Status</th>
                  <th scope="col">Progress</th>
                  <th scope="col">Action</th>
                </tr>
              </thead>
              <tbody>
                ${devicesList.map(d => {
                  const job = deviceJobMap.get(d.path);
                  const selectable = isSelectable(d, deviceJobMap);
                  let progressHtml = '—';
                  if (job && (job.status === 'running' || job.status === 'verifying' || job.status === 'formatting')) {
                    progressHtml = progressBar(job, progressMap);
                  } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
                    progressHtml = '<span class="text-success">100%</span>';
                  }
                  return `
                    <tr class="clickable ${selected.has(d.path) ? 'row-selected' : ''}" data-device="${escapeAttr(d.path)}">
                      <td data-label="Select" class="select-col"><input type="checkbox" class="row-select" data-device="${escapeAttr(d.path)}" aria-label="Select ${escapeAttr(d.path)}" ${selectable ? '' : 'disabled'} ${selected.has(d.path) ? 'checked' : ''}></td>
                      <td data-label="Device">
                        <div class="record-primary">${escapeHtml(d.model || 'Unknown device')}</div>
                        <div class="record-secondary">${escapeHtml(d.path)} · ${escapeHtml(d.serial || 'no serial')}</div>
                        ${healthBadges(d)}
                      </td>
                      <td data-label="Size">${formatBytes(d.sizeBytes)}</td>
                      <td data-label="Status">${deviceStatusBadge(d, job)}</td>
                      <td data-label="Progress" class="progress-cell">${progressHtml}</td>
                      <td data-label="Action">
                        <button class="btn btn-sm btn-danger row-wipe" data-device="${escapeAttr(d.path)}" ${d.wipeBlocked || job ? 'disabled' : ''}>
                          ${d.wipeBlocked ? 'Blocked' : job ? 'Active' : 'Wipe'}
                        </button>
                      </td>
                    </tr>`;
                }).join('')}
              </tbody>
            </table>
          </div>
        `}
      </section>

      ${renderSelectionBar()}

      ${finished.length > 0 ? `
        <section class="panel">
          <div class="panel-header">
            <div class="panel-title">Recently finished</div>
            <div class="panel-note">Last ${finished.length}</div>
          </div>
          <div class="table-scroll table-stack">
            <table class="record-table">
              <thead><tr><th scope="col">Device</th><th scope="col">Status</th><th scope="col">Time</th><th scope="col"></th></tr></thead>
              <tbody>${finished.map(j => `
                <tr>
                  <td data-label="Device">
                    <div class="record-primary text-mono">${escapeHtml(j.devicePath)}</div>
                    <div class="record-secondary">${escapeHtml(j.schemeId)}${j.label ? ' · ' + escapeHtml(j.label) : ''}</div>
                  </td>
                  <td data-label="Status"><span class="badge ${badgeClassForStatus(j.status)}">${escapeHtml(j.status)}</span></td>
                  <td data-label="Time" style="font-size:var(--text-xs);color:var(--color-text-dim)">${formatDateTime(j.completedAt)}</td>
                  <td data-label=""><div class="record-actions">${j.status === 'completed' ? renderCertMenu(j.id) : ''}</div></td>
                </tr>`).join('')}</tbody>
            </table>
          </div>
        </section>
      ` : ''}
    </div>
  `;

  // Refresh
  document.getElementById('wipe-refresh').onclick = refreshCurrentView;

  // Cancel all
  const cancelAll = document.getElementById('wipe-cancel-all');
  if (cancelAll) {
    cancelAll.onclick = () => {
      showConfirm('Cancel every running and queued wipe?', {
        dangerLabel: 'Cancel all',
        onConfirm: async () => {
          try { await apiPost('/api/cancel'); showToast('Cancelled all jobs', 'warning'); }
          catch (e) { showToast(e.message, 'error'); }
        }
      });
    };
  }

  // Per-job cancel
  el.querySelectorAll('.job-cancel').forEach(btn => {
    btn.onclick = async (e) => {
      e.stopPropagation();
      try {
        await apiPost('/api/jobs/' + encodeURIComponent(btn.dataset.job) + '/cancel');
        showToast('Cancelling ' + btn.dataset.device + '…', 'info');
      } catch (e) { showToast(e.message, 'error'); }
    };
  });

  // Row clicks → open drawer
  el.querySelectorAll('tr.clickable').forEach(row => {
    row.onclick = (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      const device = devicesList.find(d => d.path === row.dataset.device);
      if (device) openDrawer(device);
    };
  });

  // Header checkbox
  const selectAllCb = el.querySelector('.sel-all-cb');
  if (selectAllCb) {
    selectAllCb.onchange = () => {
      if (selectAllCb.checked) {
        selectableDevices.forEach(d => selected.add(d.path));
      } else {
        selectableDevices.forEach(d => selected.delete(d.path));
      }
      renderWipe();
    };
  }

  // Per-row checkboxes
  el.querySelectorAll('.row-select').forEach(cb => {
    cb.onclick = (e) => e.stopPropagation();
    cb.onchange = () => {
      if (cb.checked) selected.add(cb.dataset.device);
      else selected.delete(cb.dataset.device);
      renderWipe();
    };
  });

  // Row wipe buttons — the one per-device entry point into the configurator.
  el.querySelectorAll('.row-wipe').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const device = devicesList.find(d => d.path === btn.dataset.device);
      if (device) openConfigurator([device]);
    };
  });

  // Selection bar
  const bar = el.querySelector('.selection-bar');
  if (bar) {
    bar.querySelector('#sel-wipe').onclick = () => {
      const devices = devicesList.filter(d => selected.has(d.path));
      openConfigurator(devices);
    };
    bar.querySelector('#sel-clear').onclick = () => {
      selected.clear();
      renderWipe();
    };
  }

  initCertMenus(el);
}

function updateWipe(d, j) {
  devicesList = d;
  jobsList = j;
  renderWipe();
}

export { updateWipe };
