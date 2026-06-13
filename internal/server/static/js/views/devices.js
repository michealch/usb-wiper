// Devices view — table with multi-select + drawer integration

import { apiGet, apiPost } from '../api.js';
import { openDrawer } from '../components/drawer.js';
import { openConfigurator } from '../components/configurator.js';
import { showToast } from '../components/toast.js';
import { escapeHtml, escapeAttr, formatBytes } from '../util.js';

let devicesList = [];
let jobsList = [];
let selected = new Set();

function isSelectable(d, deviceJobMap) {
  return !d.wipeBlocked && !deviceJobMap[d.path];
}

function renderDevices() {
  const el = document.getElementById('view-devices');

  if (devicesList.length === 0) {
    el.innerHTML = `
      <div class="card">
        <div class="card-header">
          <h2>Devices</h2>
          <button class="btn btn-sm" id="btn-refresh-devices">Refresh</button>
        </div>
        <div class="empty-state"><span class="empty-state-icon">—</span>No USB devices detected. Insert a USB drive and click Refresh.</div>
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

  // Prune selections that are no longer valid (removed, blocked, or now active).
  for (const path of Array.from(selected)) {
    const d = devicesList.find(dd => dd.path === path);
    if (!d || !isSelectable(d, deviceJobMap)) selected.delete(path);
  }

  const selectableDevices = devicesList.filter(d => isSelectable(d, deviceJobMap));
  const allSelected = selectableDevices.length > 0 && selectableDevices.every(d => selected.has(d.path));

  el.innerHTML = `
    <div class="device-toolbar">
      <span class="toolbar-count"><strong>${devicesList.length}</strong> device${devicesList.length !== 1 ? 's' : ''} detected</span>
      <div class="btn-group">
        <button class="btn btn-sm" id="btn-refresh-devices">Refresh</button>
        <button class="btn btn-sm btn-danger" id="btn-wipe-all" ${selectableDevices.length === 0 ? 'disabled' : ''}>
          Wipe All Eligible
        </button>
      </div>
    </div>
    <div class="card">
      <div class="card-header">
        <h2>USB Devices</h2>
      </div>
      <div class="table-scroll table-stack device-table-wrap">
      <table>
        <thead>
          <tr>
            <th scope="col" class="select-col"><input type="checkbox" class="select-all-cb" aria-label="Select all eligible devices" ${selectableDevices.length === 0 ? 'disabled' : ''} ${allSelected ? 'checked' : ''}></th>
            <th scope="col">Path</th>
            <th scope="col">Model</th>
            <th scope="col">Size</th>
            <th scope="col">Status</th>
            <th scope="col">Progress</th>
            <th scope="col">Actions</th>
          </tr>
        </thead>
        <tbody>
          ${devicesList.map(d => {
            const job = deviceJobMap[d.path];
            const selectable = isSelectable(d, deviceJobMap);
            let statusHtml = '';
            if (job) {
              const badge = job.status === 'running' ? 'badge-warning' : job.status === 'verifying' ? 'badge-info' : 'badge-neutral';
              statusHtml = `<span class="badge ${badge}">${escapeHtml(job.status)}</span>`;
            } else if (d.wipeBlocked) {
              statusHtml = `<span class="badge badge-warning" title="${escapeAttr(d.blockReason || '')}">Blocked</span>`;
            } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
              const v = d.wipeHistory.verification;
              statusHtml = `<span class="badge ${v === 'passed' ? 'badge-success' : 'badge-danger'}">${v === 'passed' ? 'Wiped' : 'Verify failed'}</span>`;
            } else {
              statusHtml = `<span class="badge badge-success">Ready</span>`;
            }

            let progressHtml = '—';
            if (job && (job.status === 'running' || job.status === 'verifying')) {
              progressHtml = `
                <div style="display:flex;align-items:center;gap:8px">
                  <progress value="${job.progress || 0}" max="100" aria-valuemin="0" aria-valuemax="100"></progress>
                  <span class="progress-pct" style="font-size:.78rem;font-weight:600">${(job.progress || 0).toFixed(1)}%</span>
                  ${job.totalPasses > 1 ? `<span class="progress-pass" style="font-size:.72rem;color:var(--color-text-dim)">Pass ${job.currentPass}/${job.totalPasses}</span>` : ''}
                </div>`;
            } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
              progressHtml = '<span style="color:var(--color-success);font-weight:600">100%</span>';
            }

            return `
              <tr class="clickable ${selected.has(d.path) ? 'row-selected' : ''}" data-device="${escapeAttr(d.path)}">
                <td data-label="Select" class="select-col"><input type="checkbox" class="row-select-cb" data-device="${escapeAttr(d.path)}" aria-label="Select ${escapeAttr(d.path)}" ${selectable ? '' : 'disabled'} ${selected.has(d.path) ? 'checked' : ''}></td>
                <td data-label="Path" style="font-family:var(--font-mono)">${escapeHtml(d.path)}</td>
                <td data-label="Model">${escapeHtml(d.model || '—')}</td>
                <td data-label="Size">${formatBytes(d.sizeBytes)}</td>
                <td data-label="Status">${statusHtml}</td>
                <td data-label="Progress" class="progress-cell">${progressHtml}</td>
                <td data-label="Actions">
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
    </div>
    ${renderSelectionBar()}
  `;

  // Bind row clicks → open drawer
  el.querySelectorAll('tr.clickable').forEach(row => {
    row.onclick = (e) => {
      if (e.target.closest('button') || e.target.closest('input')) return;
      const path = row.dataset.device;
      const device = devicesList.find(d => d.path === path);
      if (device) openDrawer(device);
    };
  });

  // Bind select-all checkbox
  const selectAllCb = el.querySelector('.select-all-cb');
  if (selectAllCb) {
    selectAllCb.onchange = () => {
      if (selectAllCb.checked) {
        selectableDevices.forEach(d => selected.add(d.path));
      } else {
        selectableDevices.forEach(d => selected.delete(d.path));
      }
      renderDevices();
    };
  }

  // Bind per-row checkboxes
  el.querySelectorAll('.row-select-cb').forEach(cb => {
    cb.onclick = (e) => e.stopPropagation();
    cb.onchange = () => {
      if (cb.checked) selected.add(cb.dataset.device);
      else selected.delete(cb.dataset.device);
      renderDevices();
    };
  });

  // Bind wipe buttons
  el.querySelectorAll('.dev-wipe-btn').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const path = btn.dataset.device;
      const device = devicesList.find(d => d.path === path);
      if (device) openConfigurator([device]);
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

  // Bind selection bar actions
  const bar = el.querySelector('.selection-bar');
  if (bar) {
    const wipeBtn = bar.querySelector('#selection-wipe-btn');
    const testBtn = bar.querySelector('#selection-test-btn');
    const clearBtn = bar.querySelector('#selection-clear-btn');
    if (wipeBtn) wipeBtn.onclick = () => {
      const devices = devicesList.filter(d => selected.has(d.path));
      openConfigurator(devices);
    };
    if (testBtn) testBtn.onclick = async () => {
      const paths = Array.from(selected);
      for (const path of paths) {
        try {
          await apiPost('/api/test-wipe', { device: path, verifySizeGB: 1 });
        } catch (err) { showToast(err.message, 'error'); }
      }
      showToast(`Test wipe started on ${paths.length} device${paths.length !== 1 ? 's' : ''}`, 'info');
    };
    if (clearBtn) clearBtn.onclick = () => {
      selected.clear();
      renderDevices();
    };
  }

  document.getElementById('btn-refresh-devices').onclick = refreshDevices;
  document.getElementById('btn-wipe-all').onclick = () => {
    const eligible = devicesList.filter(d => isSelectable(d, deviceJobMap));
    if (eligible.length === 0) {
      showToast('No eligible devices to wipe', 'warning');
      return;
    }
    openConfigurator(eligible);
  };
}

function renderSelectionBar() {
  if (selected.size === 0) return '';
  return `
    <div class="selection-bar">
      <span class="selection-count" aria-live="polite">${selected.size} selected</span>
      <div class="btn-group">
        <button class="btn btn-sm btn-success" id="selection-test-btn">Test</button>
        <button class="btn btn-sm btn-danger" id="selection-wipe-btn">Wipe</button>
        <button class="btn btn-sm" id="selection-clear-btn">Clear</button>
      </div>
    </div>
  `;
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

export { renderDevices, updateDevices, refreshDevices };
