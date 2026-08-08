// Slide-in drawer component (right side)

import { apiGet } from '../api.js';
import { escapeHtml, formatBytes, formatDateTime } from '../util.js';
import { attachOverlay } from './overlay.js';
import { initTabs } from './tabs.js';

let drawerEl, drawerOverlay, drawerBody, drawerTitle;
let overlayHandle = null;
let tabsHandle = null;
let currentDevice = null;

function initDrawer() {
  drawerOverlay = document.createElement('div');
  drawerOverlay.className = 'overlay overlay-drawer';
  drawerOverlay.onclick = (e) => { if (e.target === drawerOverlay) closeDrawer(); };

  drawerEl = document.createElement('div');
  drawerEl.className = 'drawer';
  drawerEl.tabIndex = -1;

  drawerEl.innerHTML = `
    <div class="drawer-header">
      <h2 id="drawer-title">Device Details</h2>
      <button class="btn btn-ghost btn-sm" id="drawer-close" aria-label="Close drawer">×</button>
    </div>
    <div id="drawer-tablist"></div>
    <div class="drawer-body" id="drawer-body"></div>
  `;

  document.body.appendChild(drawerOverlay);
  document.body.appendChild(drawerEl);

  document.getElementById('drawer-close').onclick = closeDrawer;
  drawerTitle = document.getElementById('drawer-title');
  drawerBody = document.getElementById('drawer-body');

  overlayHandle = attachOverlay(drawerOverlay, drawerEl, { onClose: () => { currentDevice = null; } });

  tabsHandle = initTabs(document.getElementById('drawer-tablist'), {
    idPrefix: 'devinfo',
    tabs: [
      { id: 'overview', label: 'Overview' },
      { id: 'smart', label: 'SMART' },
      { id: 'history', label: 'History' }
    ],
    activeId: 'overview',
    onSelect: (id, panelEl) => loadTabContent(currentDevice, id, panelEl),
    panelsEl: drawerBody
  });
}

function openDrawer(device, initialTab = 'overview') {
  currentDevice = device;
  drawerTitle.textContent = device.name || device.path;
  // initTabs fires onSelect once at startup (before any device exists), so the
  // initial panel is empty. Re-select the requested tab now that we have a
  // device to force loadTabContent to render into it.
  tabsHandle.select(initialTab === 'overview' ? 'overview' : initialTab);
  overlayHandle.open();
}

function closeDrawer() {
  overlayHandle.close();
}

function loadTabContent(device, tab, panelEl) {
  if (!device) return;
  switch (tab) {
    case 'overview': renderOverviewTab(device, panelEl); break;
    case 'smart': renderSmartTab(device, panelEl); break;
    case 'history': renderHistoryTab(device, panelEl); break;
  }
}

function renderOverviewTab(d, el) {
  el.innerHTML = `
    <div class="form-group"><label>Device Path</label><div style="font-family:var(--font-mono)">${escapeHtml(d.path)}</div></div>
    <div class="form-group"><label>Device ID</label><div style="font-family:var(--font-mono);overflow-wrap:anywhere">${escapeHtml(d.deviceId || '—')}</div></div>
    <div class="form-group"><label>Identity</label><div>${escapeHtml(identityLabel(d))}</div></div>
    <div class="form-group"><label>Model</label><div>${escapeHtml(d.model || '—')}</div></div>
    <div class="form-group"><label>Serial</label><div>${escapeHtml(d.serial || '—')}</div></div>
    <div class="form-group"><label>Firmware</label><div>${escapeHtml(d.firmware || '—')}</div></div>
    <div class="form-group"><label>WWN</label><div>${escapeHtml(d.wwn || '—')}</div></div>
    <div class="form-group"><label>Size</label><div>${formatBytes(d.sizeBytes)}</div></div>
    <div class="form-group"><label>Removable</label><div>${d.removable ? 'Yes' : 'No'}</div></div>
    <div class="form-group"><label>USB</label><div>${d.isUSB ? 'Yes' : 'No'}</div></div>
    <div class="form-group"><label>Mounted</label><div>${d.mounted ? escapeHtml((d.mountPoints || []).join(', ')) : 'No'}</div></div>
    ${d.wipeBlocked ? `<div class="form-group"><label>Block Reason</label><div class="text-danger">${escapeHtml(d.blockReason || 'Blocked')}</div></div>` : ''}
    ${d.wipeHistory ? `<div class="form-group"><label>Last Wipe</label><div>${escapeHtml(d.wipeHistory.status)} ${d.wipeHistory.verification === 'passed' ? '\u2713' : d.wipeHistory.verification === 'failed' ? '\u2717' : ''}</div></div>` : ''}
  `;
}

async function renderSmartTab(device, el) {
  el.innerHTML = '<p class="muted">Loading SMART data...</p>';
  
  try {
    const data = await apiGet('/api/health?device=' + encodeURIComponent(device.path));
    const h = data;
    el.innerHTML = renderHealthTable(h) + await renderHealthHistory(device);
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

async function renderHistoryTab(device, el) {
  el.innerHTML = '<p class="muted">Loading history...</p>';
  
  try {
    if (!isTrustedIdentity(device)) {
      el.innerHTML = '<p class="muted">This device identity is uncertain, so prior wipe history is not attached.</p>';
      return;
    }
    const key = deviceCacheKey(device);
    const data = await apiGet('/api/history?deviceId=' + encodeURIComponent(device.deviceId));
    const history = data.history || [];
    if (history.length === 0) {
      el.innerHTML = '<p class="muted">No wipe history for this device.</p>';
      return;
    }
    el.innerHTML = `<table>
      <thead><tr><th scope="col">Status</th><th scope="col">Verification</th><th scope="col">Size</th><th scope="col">Duration</th><th scope="col">Finished</th></tr></thead>
      <tbody>${history.map(r => `
        <tr>
          <td><span class="badge ${r.status === 'completed' ? 'badge-success' : r.status === 'failed' ? 'badge-danger' : 'badge-neutral'}">${escapeHtml(r.status)}</span></td>
          <td>${escapeHtml(r.verification || '—')}</td>
          <td>${formatBytes(r.sizeBytes)}</td>
          <td>${escapeHtml(r.duration || '—')}</td>
          <td>${formatDateTime(r.finishedAt)}</td>
        </tr>`).join('')}</tbody></table>`;
  } catch (e) {
    el.innerHTML = '<p class="muted">History unavailable</p>';
  }
}

async function renderHealthHistory(device) {
  if (!isTrustedIdentity(device)) {
    return '<p class="muted mt-3">SMART history is limited to this uncertain attachment.</p>';
  }
  const data = await apiGet('/api/health-history?deviceId=' + encodeURIComponent(device.deviceId));
  const history = (data.history || []).slice(0, 8);
  if (history.length === 0) {
    return '<p class="muted mt-3">No saved SMART snapshots yet.</p>';
  }
  return `
    <h3 class="mt-4 mb-2">SMART History</h3>
    <table>
      <thead><tr><th scope="col">Captured</th><th scope="col">Status</th><th scope="col">Temp</th><th scope="col">Hours</th><th scope="col">Wear</th><th scope="col">Errors</th></tr></thead>
      <tbody>${history.map(r => `
        <tr>
          <td>${formatDateTime(r.capturedAt)}</td>
          <td>${escapeHtml(r.healthStatus || 'UNKNOWN')}</td>
          <td>${r.temperatureC ? escapeHtml(String(r.temperatureC)) + '°C' : '—'}</td>
          <td>${r.powerOnHours ? Number(r.powerOnHours).toLocaleString() : '—'}</td>
          <td>${r.enduranceUsedPct ? escapeHtml(String(r.enduranceUsedPct)) + '%' : '—'}</td>
          <td>${r.uncorrectableErrors != null ? Number(r.uncorrectableErrors).toLocaleString() : '—'}</td>
        </tr>`).join('')}</tbody>
    </table>`;
}

function deviceCacheKey(device) {
  return device.deviceId || device.path;
}

function isTrustedIdentity(device) {
  return device && (device.identityConfidence === 'high' || device.identityConfidence === 'medium') && device.deviceId;
}

function identityLabel(device) {
  const confidence = device.identityConfidence || 'unknown';
  const source = device.identitySource || 'unknown';
  if (confidence === 'low') return 'Uncertain — prior history is not attached (' + source + ')';
  return confidence.charAt(0).toUpperCase() + confidence.slice(1) + ' confidence (' + source + ')';
}

export { initDrawer, openDrawer, closeDrawer };
