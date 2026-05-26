// Slide-in drawer component (right side)

let drawerEl, drawerOverlay, drawerBody, drawerTitle;
let currentDevice = null;
let activeTab = 'overview';
let deviceHealthCache = {};
let deviceHistoryCache = {};

function initDrawer() {
  drawerOverlay = document.createElement('div');
  drawerOverlay.className = 'drawer-overlay';
  drawerOverlay.onclick = closeDrawer;
  
  drawerEl = document.createElement('div');
  drawerEl.className = 'drawer';
  
  drawerEl.innerHTML = `
    <div class="drawer-header">
      <h2 id="drawer-title">Device Details</h2>
      <button class="btn btn-ghost btn-sm" id="drawer-close">&times;</button>
    </div>
    <div class="drawer-tabs" id="drawer-tabs">
      <button class="drawer-tab active" data-tab="overview">Overview</button>
      <button class="drawer-tab" data-tab="smart">SMART</button>
      <button class="drawer-tab" data-tab="wipe">Wipe</button>
      <button class="drawer-tab" data-tab="history">History</button>
    </div>
    <div class="drawer-body" id="drawer-body">
      <div class="drawer-tab-content active" id="tab-overview"></div>
      <div class="drawer-tab-content" id="tab-smart"></div>
      <div class="drawer-tab-content" id="tab-wipe"></div>
      <div class="drawer-tab-content" id="tab-history"></div>
    </div>
  `;
  
  document.body.appendChild(drawerOverlay);
  document.body.appendChild(drawerEl);
  
  document.getElementById('drawer-close').onclick = closeDrawer;
  drawerTitle = document.getElementById('drawer-title');
  drawerBody = document.getElementById('drawer-body');
  
  // Tab switching
  document.querySelectorAll('.drawer-tab').forEach(tab => {
    tab.onclick = () => {
      document.querySelectorAll('.drawer-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      activeTab = tab.dataset.tab;
      document.querySelectorAll('.drawer-tab-content').forEach(c => c.classList.remove('active'));
      document.getElementById('tab-' + activeTab).classList.add('active');
      if (currentDevice) loadTabContent(currentDevice, activeTab);
    };
  });
}

function openDrawer(device, initialTab = 'overview') {
  currentDevice = device;
  drawerTitle.textContent = device.name || device.path;
  activeTab = initialTab;
  
  // Reset tabs
  document.querySelectorAll('.drawer-tab').forEach(t => t.classList.remove('active'));
  document.querySelector(`.drawer-tab[data-tab="${activeTab}"]`).classList.add('active');
  document.querySelectorAll('.drawer-tab-content').forEach(c => c.classList.remove('active'));
  document.getElementById('tab-' + activeTab).classList.add('active');
  
  drawerOverlay.classList.add('open');
  drawerEl.classList.add('open');
  
  loadTabContent(device, activeTab);
}

function closeDrawer() {
  drawerOverlay.classList.remove('open');
  drawerEl.classList.remove('open');
  currentDevice = null;
}

function loadTabContent(device, tab) {
  switch (tab) {
    case 'overview': renderOverviewTab(device); break;
    case 'smart': renderSmartTab(device); break;
    case 'wipe': renderWipeTab(device); break;
    case 'history': renderHistoryTab(device); break;
  }
}

function renderOverviewTab(d) {
  const el = document.getElementById('tab-overview');
  el.innerHTML = `
    <div class="form-group"><label>Device Path</label><div style="font-family:var(--font-mono)">${escapeHtml(d.path)}</div></div>
    <div class="form-group"><label>Model</label><div>${escapeHtml(d.model || '—')}</div></div>
    <div class="form-group"><label>Serial</label><div>${escapeHtml(d.serial || '—')}</div></div>
    <div class="form-group"><label>Size</label><div>${formatBytes(d.sizeBytes)}</div></div>
    <div class="form-group"><label>Removable</label><div>${d.removable ? 'Yes' : 'No'}</div></div>
    <div class="form-group"><label>USB</label><div>${d.isUSB ? 'Yes' : 'No'}</div></div>
    <div class="form-group"><label>Mounted</label><div>${d.mounted ? escapeHtml(d.mountPoints.join(', ')) : 'No'}</div></div>
    ${d.wipeBlocked ? `<div class="form-group"><label>Block Reason</label><div class="text-danger">${escapeHtml(d.blockReason || 'Blocked')}</div></div>` : ''}
    ${d.wipeHistory ? `<div class="form-group"><label>Last Wipe</label><div>${escapeHtml(d.wipeHistory.status)} ${d.wipeHistory.verification === 'passed' ? '✓' : d.wipeHistory.verification === 'failed' ? '✗' : ''}</div></div>` : ''}
  `;
}

async function renderSmartTab(device) {
  const el = document.getElementById('tab-smart');
  el.innerHTML = '<p class="muted">Loading SMART data...</p>';
  
  try {
    if (!deviceHealthCache[device.path]) {
      const data = await apiGet('/api/health?device=' + encodeURIComponent(device.path));
      deviceHealthCache[device.path] = data;
    }
    const h = deviceHealthCache[device.path];
    el.innerHTML = renderHealthTable(h);
  } catch (e) {
    el.innerHTML = `<p class="muted">SMART data unavailable</p><p class="muted">${escapeHtml(e.message)}</p>`;
  }
}

function renderHealthTable(h) {
  const nvmeLog = getNVMeLog(h);
  const isNVMe = h.deviceType === 'nvme' || !!nvmeLog;
  const rows = [
    ['Status', h.healthStatus || 'UNKNOWN'],
    ['Bridge Mode', h.deviceType || (h.raw && h.raw.usb_wiper_smartctl_type) || 'auto'],
    ['Model', h.modelName || '—'],
    ['Serial', h.serialNumber || '—'],
    ['Firmware', h.firmwareVersion || '—'],
    ['Capacity', formatBytes(h.capacityBytes)],
    ['Power On Hours', h.powerOnHours ? h.powerOnHours.toLocaleString() : '—'],
    ['Power Cycles', h.powerCycleCount ? h.powerCycleCount.toLocaleString() : '—'],
    ['Temperature', h.temperatureC ? h.temperatureC + '°C' : '—'],
  ];

  if (isNVMe) {
    rows.push(
      ['Spare Available', formatPercent(h.availableSparePct, nvmeLog && nvmeLog.available_spare)],
      ['Endurance Used', formatPercent(h.enduranceUsedPct, nvmeLog && nvmeLog.percentage_used)],
      ['Data Read', h.readLBAs ? formatNVMeDataUnits(h.readLBAs) : '—'],
      ['Data Written', h.writeLBAs ? formatNVMeDataUnits(h.writeLBAs) : '—'],
      ['Media Errors', h.uncorrectableErrors != null ? h.uncorrectableErrors.toLocaleString() : '—']
    );
  } else {
    rows.push(
      ['Reallocated', h.reallocatedSectors ? h.reallocatedSectors.toLocaleString() : '0'],
      ['Pending', h.pendingSectors ? h.pendingSectors.toLocaleString() : '0'],
      ['Uncorrectable', h.uncorrectableErrors ? h.uncorrectableErrors.toLocaleString() : '0']
    );
  }

  if (h.raw && h.raw.error) {
    rows.push(['Probe Message', h.raw.error]);
  }

  return `<table class="smart-table">${rows.map(([k, v]) => `
    <tr>
      <td>${escapeHtml(k)}</td>
      <td>${escapeHtml(String(v))}</td>
    </tr>`).join('')}</table>`;
}

function getNVMeLog(h) {
  if (!h || !h.raw) return null;
  return h.raw.nvme_smart_health_log || h.raw.nvme_smart_health_information_log || null;
}

function formatPercent(normalized, rawValue) {
  const value = normalized || rawValue;
  return value !== undefined && value !== null && value !== '' ? value + '%' : '—';
}

function formatNVMeDataUnits(units) {
  return formatBytes(units * 512000);
}

async function renderWipeTab(device) {
  const el = document.getElementById('tab-wipe');
  el.innerHTML = '<p class="muted">Loading schemes...</p>';

  try {
    const schemes = await apiGet('/api/schemes');
    const presets = await apiGet('/api/presets');

    el.innerHTML = `
      <div class="form-group">
        <label>Wipe Scheme</label>
        <select id="wipe-scheme" class="w-full">
          ${schemes.schemes.map(s => `<option value="${s.id}">${s.displayName} (${s.passes} pass${s.passes>1?'es':''})</option>`).join('')}
        </select>
      </div>
      <div class="form-group">
        <label>Preset</label>
        <select id="wipe-preset" class="w-full">
          <option value="">— Manual —</option>
          ${presets.presets.map(p => `<option value="${p.id}">${p.name} (${p.schemeId}, ${p.verifySizeGB}GiB verify)</option>`).join('')}
        </select>
      </div>
      <div class="form-group">
        <label class="checkbox-label">
          <input type="checkbox" id="wipe-autoformat">Auto-format FAT32 after wipe
        </label>
      </div>
      <div class="form-group">
        <label>Verification</label>
        <select id="wipe-verify" class="w-full">
          <option value="0">Off</option>
          <option value="1" selected>1 GiB</option>
          <option value="2">2 GiB</option>
          <option value="4">4 GiB</option>
          <option value="16">16 GiB</option>
        </select>
      </div>
      <div class="form-group">
        <label>Label</label>
        <input type="text" id="wipe-label" class="w-full" placeholder="Optional label (e.g. RMA-2026-05)">
      </div>
      <div class="btn-group" style="margin-top:var(--space-4)">
        <button class="btn btn-danger" id="btn-start-wipe">Start Wipe</button>
        <button class="btn btn-success" id="btn-test-wipe">Test Wipe (Read-Only)</button>
      </div>
    `;
    
    // Preset selection fills form
    document.getElementById('wipe-preset').onchange = function() {
      const preset = presets.presets.find(p => p.id === this.value);
      if (preset) {
        document.getElementById('wipe-scheme').value = preset.schemeId;
        document.getElementById('wipe-autoformat').checked = preset.autoFormat;
        document.getElementById('wipe-verify').value = preset.verifySizeGB;
      }
    };
    
    document.getElementById('btn-start-wipe').onclick = () => {
      const scheme = document.getElementById('wipe-scheme').value;
      const autoFormat = document.getElementById('wipe-autoformat').checked;
      const verify = parseInt(document.getElementById('wipe-verify').value);
      const label = document.getElementById('wipe-label').value;
      
      showConfirm(`DESTROY ALL DATA on ${device.path}?\n\nScheme: ${scheme}\nThis cannot be undone.`, {
        dangerLabel: 'Wipe Device',
        onConfirm: async () => {
          try {
            await apiPost('/api/wipe', {
              devices: [device.path],
              schemeId: scheme,
              autoFormat,
              verifySizeGB: verify,
              label
            });
            showToast('Wipe started on ' + device.path, 'success');
            closeDrawer();
          } catch (e) {
            showToast(e.message, 'error');
          }
        }
      });
    };
    
    document.getElementById('btn-test-wipe').onclick = async () => {
      const verify = parseInt(document.getElementById('wipe-verify').value);
      try {
        await apiPost('/api/test-wipe', { device: device.path, verifySizeGB: verify });
        showToast('Test wipe started on ' + device.path, 'info');
        closeDrawer();
      } catch (e) {
        showToast(e.message, 'error');
      }
    };
  } catch (e) {
    el.innerHTML = '<p class="text-danger">Failed to load schemes</p>';
  }
}

async function renderHistoryTab(device) {
  const el = document.getElementById('tab-history');
  el.innerHTML = '<p class="muted">Loading history...</p>';
  
  try {
    if (!deviceHistoryCache[device.path]) {
      const data = await apiGet('/api/history?device=' + encodeURIComponent(device.path));
      deviceHistoryCache[device.path] = data.history || [];
    }
    const history = deviceHistoryCache[device.path];
    if (history.length === 0) {
      el.innerHTML = '<p class="muted">No wipe history for this device.</p>';
      return;
    }
    el.innerHTML = `<table>
      <thead><tr><th>Status</th><th>Verification</th><th>Size</th><th>Duration</th><th>Finished</th></tr></thead>
      <tbody>${history.map(r => `
        <tr>
          <td><span class="badge ${r.status === 'completed' ? 'badge-success' : r.status === 'failed' ? 'badge-danger' : 'badge-neutral'}">${r.status}</span></td>
          <td>${r.verification || '—'}</td>
          <td>${formatBytes(r.sizeBytes)}</td>
          <td>${r.duration || '—'}</td>
          <td>${r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—'}</td>
        </tr>`).join('')}</tbody></table>`;
  } catch (e) {
    el.innerHTML = '<p class="muted">History unavailable</p>';
  }
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

import { apiGet, apiPost } from '../api.js';
import { showToast, showConfirm } from './toast.js';

export { initDrawer, openDrawer, closeDrawer, deviceHealthCache, deviceHistoryCache };
