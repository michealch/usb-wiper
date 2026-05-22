// USB Wiper — vanilla JavaScript frontend
// Each device row is expandable with per-device settings, health, and wipe history.
// Fully push-driven: SSE delivers progress + refresh events. No polling needed.

let eventSource = null;
let deviceStates = {};     // devicePath -> { wiping, percent, speed, status, verified }
let expandedDevice = null; // currently expanded device path
let deviceHealth = {};     // devicePath -> cached health data
let deviceHistory = {};    // devicePath -> cached history records
let perDeviceSettings = {}; // devicePath -> { autoFormat: bool, verifySizeGB: int }

// Restore expanded device from sessionStorage
(function restoreExpanded() {
  expandedDevice = sessionStorage.getItem('expandedDevice') || null;
})();

// Refresh triggered by SSE refresh event — clear caches and reload
function handleRefreshEvent() {
  deviceHealth = {};
  deviceHistory = {};
  loadDevices();
}

// ---- Load full device list ----

async function loadDevices() {
  const tbody = document.querySelector('#device-table tbody');
  const noDevices = document.getElementById('no-devices');
  tbody.innerHTML = '';

  try {
    const res = await fetch('/api/devices');
    if (!res.ok) throw new Error('Failed to load devices');
    const data = await res.json();
    const devices = data.devices || [];

    if (devices.length === 0) {
      noDevices.style.display = 'block';
      document.getElementById('device-table').style.display = 'none';
      return;
    }
    noDevices.style.display = 'none';
    document.getElementById('device-table').style.display = 'table';

    let anyWiping = false;

    devices.forEach(d => {
      if (d.wiping) anyWiping = true;

      // ---- Main row ----
      const row = tbody.insertRow();
      row.setAttribute('data-device', d.path);
      row.className = 'device-row';
      if (d.path === expandedDevice) row.classList.add('expanded');
      row.onclick = () => toggleDeviceExpand(d.path);

      row.insertCell().textContent = d.path;
      row.insertCell().textContent = d.model || '—';
      row.insertCell().textContent = formatBytes(d.sizeBytes);

      // Status cell
      const statusCell = row.insertCell();
      statusCell.className = 'status-cell';
      if (d.wiping) {
        statusCell.innerHTML = '<span class="status-wiping">Wiping</span>';
      } else if (d.wipeBlocked) {
        statusCell.innerHTML = '<span class="status-blocked">⚠ Blocked</span>';
        statusCell.title = d.blockReason || '';
      } else if (d.wipeHistory && d.wipeHistory.status === 'completed') {
        const v = d.wipeHistory.verification;
        if (v === 'passed') statusCell.innerHTML = '<span class="status-passed">✓ Wiped</span>';
        else if (v === 'failed') statusCell.innerHTML = '<span class="status-failed">✗ VerFail</span>';
        else statusCell.innerHTML = '<span class="status-passed">✓ Wiped</span>';
      } else if (d.wipeHistory && d.wipeHistory.status === 'failed') {
        statusCell.innerHTML = '<span class="status-failed">Failed</span>';
      } else if (d.wipeHistory && d.wipeHistory.status === 'cancelled') {
        statusCell.innerHTML = '<span class="status-cancelled">Cancelled</span>';
      } else {
        statusCell.innerHTML = '<span class="status-ready">Ready</span>';
      }

      // Progress cell — per-device inline bar
      const progressCell = row.insertCell();
      progressCell.className = 'progress-cell';
      const st = deviceStates[d.path];
      if (d.wiping || (st && st.wiping)) {
        const pct = st ? st.percent : (d.wipePercent || 0);
        const speed = st ? st.speed : 0;
        progressCell.innerHTML =
          '<div class="mini-progress">' +
          '<progress value="' + pct + '" max="100"></progress>' +
          '<span class="mini-pct">' + pct.toFixed(1) + '%</span>' +
          (speed > 0 ? '<span class="mini-speed">' + formatBytes(speed) + '/s</span>' : '') +
          '</div>';
      } else if (d.wipeHistory && d.wipeHistory.status === 'completed' && d.wipeHistory.verification === 'passed') {
        progressCell.innerHTML = '<span class="mini-done">100%</span>';
      } else {
        progressCell.textContent = '—';
      }

      // Wipe + Test Wipe buttons
      const btnCell = row.insertCell();
      btnCell.className = 'btn-cell';

      const btnWipe = document.createElement('button');
      if (d.wiping) {
        btnWipe.textContent = 'Cancel';
        btnWipe.className = 'btn btn-cancel';
        btnWipe.onclick = (e) => { e.stopPropagation(); cancelDeviceWipe(d.path); };
      } else if (d.wipeBlocked) {
        btnWipe.textContent = 'Blocked';
        btnWipe.className = 'btn';
        btnWipe.disabled = true;
        btnWipe.title = d.blockReason || '';
      } else {
        btnWipe.textContent = 'Wipe';
        btnWipe.className = 'btn btn-danger';
        btnWipe.onclick = (e) => { e.stopPropagation(); startDeviceWipe(d.path); };
      }
      btnCell.appendChild(btnWipe);

      // Test Wipe button — non-destructive verification only
      const btnTest = document.createElement('button');
      if (d.wiping) {
        btnTest.textContent = '—';
        btnTest.className = 'btn';
        btnTest.disabled = true;
      } else if (d.wipeBlocked) {
        btnTest.textContent = '—';
        btnTest.className = 'btn';
        btnTest.disabled = true;
      } else {
        btnTest.textContent = 'Test';
        btnTest.className = 'btn btn-test';
        btnTest.title = 'Non-destructive: reads random chunks to verify all zeros';
        btnTest.onclick = (e) => { e.stopPropagation(); startTestWipe(d.path); };
      }
      btnCell.appendChild(btnTest);

      // ---- Expanded row (health + settings + history) ----
      if (d.path === expandedDevice) {
        const expandRow = tbody.insertRow();
        expandRow.className = 'expand-row';
        expandRow.setAttribute('data-expand-for', d.path);
        const cell = expandRow.insertCell();
        cell.colSpan = 6;
        cell.innerHTML =
          '<div class="expand-content" id="expand-' + d.path.replace(/\//g, '-') + '">' +
          '  <div class="expand-section health-area"><h3>Health</h3><div class="health-table-wrap">Loading...</div></div>' +
          '  <div class="expand-section settings-area">' +
          '    <h3>Wipe Settings</h3>' +
          '    <label class="checkbox-label"><input type="checkbox" class="dev-auto-format" data-device="' + d.path + '"' +
               (perDeviceSettings[d.path] && perDeviceSettings[d.path].autoFormat ? ' checked' : '') +
               '> Auto-format FAT32</label>' +
          '    <label class="verify-label">Verify ' +
          '      <select class="dev-verify-size" data-device="' + d.path + '">' +
          '        <option value="0">Off</option>' +
          '        <option value="1"' + (!perDeviceSettings[d.path] || perDeviceSettings[d.path].verifySizeGB !== 0 ? ' selected' : '') + '>1 GiB</option>' +
          '        <option value="2">2 GiB</option>' +
          '        <option value="4">4 GiB</option>' +
          '      </select>' +
          '    </label>' +
          '  </div>' +
          '  <div class="expand-section history-area"><h3>Wipe History</h3><div class="history-table-wrap">Loading...</div></div>' +
          '</div>';

        // Load health and history
        loadDeviceHealth(d.path);
        loadDeviceHistory(d.path);
      }
    });

    updateCancelButton(anyWiping);

    // Bind per-device setting listeners after DOM is updated
    bindPerDeviceSettingListeners();

  } catch (err) {
    logMessage('Error loading devices: ' + err.message);
  }
}

// ---- Expand / collapse device ----

function toggleDeviceExpand(devicePath) {
  if (expandedDevice === devicePath) {
    // Collapse
    expandedDevice = null;
    sessionStorage.removeItem('expandedDevice');
  } else {
    expandedDevice = devicePath;
    sessionStorage.setItem('expandedDevice', devicePath);
  }
  loadDevices();
}

function bindPerDeviceSettingListeners() {
  document.querySelectorAll('.dev-auto-format').forEach(cb => {
    cb.onchange = function() {
      const dev = this.getAttribute('data-device');
      if (!perDeviceSettings[dev]) perDeviceSettings[dev] = { autoFormat: false, verifySizeGB: 1 };
      perDeviceSettings[dev].autoFormat = this.checked;
    };
  });
  document.querySelectorAll('.dev-verify-size').forEach(sel => {
    sel.onchange = function() {
      const dev = this.getAttribute('data-device');
      if (!perDeviceSettings[dev]) perDeviceSettings[dev] = { autoFormat: false, verifySizeGB: 1 };
      perDeviceSettings[dev].verifySizeGB = parseInt(this.value);
    };
  });
}

// ---- Per-device health ----

async function loadDeviceHealth(devicePath) {
  const wrapDiv = document.querySelector('#expand-' + devicePath.replace(/\//g, '-') + ' .health-table-wrap');
  if (!wrapDiv) return;
  wrapDiv.textContent = 'Loading...';

  try {
    if (!deviceHealth[devicePath]) {
      const res = await fetch('/api/health?device=' + encodeURIComponent(devicePath));
      if (!res.ok) throw new Error('Failed');
      deviceHealth[devicePath] = await res.json();
    }
    const h = deviceHealth[devicePath];
    const rows = [
      ['Status', h.healthStatus || 'UNKNOWN'],
      ['Model', h.modelName || '—'],
      ['Serial', h.serialNumber || '—'],
      ['Firmware', h.firmwareVersion || '—'],
      ['Capacity', formatBytes(h.capacityBytes)],
      ['Power On Hours', h.powerOnHours ? h.powerOnHours.toLocaleString() : '—'],
      ['Power Cycles', h.powerCycleCount ? h.powerCycleCount.toLocaleString() : '—'],
      ['Temperature', h.temperatureC ? h.temperatureC + '°C' : '—'],
      ['Reallocated', h.reallocatedSectors ? h.reallocatedSectors.toLocaleString() : '0'],
      ['Pending', h.pendingSectors ? h.pendingSectors.toLocaleString() : '0'],
      ['Uncorr.', h.uncorrectableErrors ? h.uncorrectableErrors.toLocaleString() : '0'],
    ];
    wrapDiv.innerHTML = '<table class="mini-table">' +
      rows.map(([k, v]) => '<tr><td>' + k + '</td><td>' + v + '</td></tr>').join('') +
      '</table>';
  } catch (e) {
    wrapDiv.textContent = 'Health data unavailable';
  }
}

// ---- Per-device history ----

async function loadDeviceHistory(devicePath) {
  const wrapDiv = document.querySelector('#expand-' + devicePath.replace(/\//g, '-') + ' .history-table-wrap');
  if (!wrapDiv) return;
  wrapDiv.textContent = 'Loading...';

  try {
    if (!deviceHistory[devicePath]) {
      const res = await fetch('/api/history?device=' + encodeURIComponent(devicePath));
      if (!res.ok) throw new Error('Failed');
      deviceHistory[devicePath] = (await res.json()).history || [];
    }
    const history = deviceHistory[devicePath];
    if (history.length === 0) {
      wrapDiv.textContent = 'No wipe history for this device.';
      return;
    }
    let html = '<table class="mini-table history-mini"><thead><tr><th>Status</th><th>Verified</th><th>Size</th><th>Duration</th><th>Finished</th></tr></thead><tbody>';
    history.forEach(r => {
      let stBadge = r.status;
      if (r.status === 'completed') stBadge = '<span class="status-passed">Completed</span>';
      else if (r.status === 'failed') stBadge = '<span class="status-failed">Failed</span>';
      else if (r.status === 'cancelled') stBadge = '<span class="status-cancelled">Cancelled</span>';

      let vBadge = r.verification || '—';
      if (r.verification === 'passed') vBadge = '<span class="status-passed">✓ Passed</span>';
      else if (r.verification === 'failed') vBadge = '<span class="status-failed">✗ Failed</span>';

      html += '<tr title="' + (r.error || '') + '">' +
        '<td>' + stBadge + '</td>' +
        '<td>' + vBadge + '</td>' +
        '<td>' + formatBytes(r.sizeBytes) + '</td>' +
        '<td>' + (r.duration || '—') + '</td>' +
        '<td>' + (r.finishedAt ? new Date(r.finishedAt).toLocaleString() : '—') + '</td>' +
        '</tr>';
    });
    html += '</tbody></table>';
    wrapDiv.innerHTML = html;
  } catch (e) {
    wrapDiv.textContent = 'History unavailable.';
  }
}

// ---- Per-device wipe ----

async function startDeviceWipe(devicePath) {
  const confirmed = confirm(
    `DESTROY ALL DATA on ${devicePath}?\n\nThis cannot be undone.`
  );
  if (!confirmed) return;

  const settings = perDeviceSettings[devicePath] || { autoFormat: false, verifySizeGB: 1 };
  logMessage(`Starting wipe on ${devicePath} (autoFormat=${settings.autoFormat}, verify=${settings.verifySizeGB}GiB)...`);

  deviceStates[devicePath] = { wiping: true, percent: 0, speed: 0, status: 'running' };

  // Update the expanded row's history cache so it re-fetches
  delete deviceHistory[devicePath];

  loadDevices();

  try {
    const res = await fetch('/api/wipe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        device: devicePath,
        autoFormat: settings.autoFormat,
        verifySizeGB: settings.verifySizeGB
      })
    });

    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Failed to start wipe');
    }

    const data = await res.json();
    data.started.forEach(d => logMessage('Wipe started on ' + d));
    if (data.conflicts && data.conflicts.length > 0) {
      data.conflicts.forEach(d => logMessage('Skipped ' + d + ': already wiping'));
    }
  } catch (err) {
    logMessage('Error: ' + err.message);
    delete deviceStates[devicePath];
  }

  loadDevices();
}

async function startTestWipe(devicePath) {
  const confirmed = confirm(
    `Test wipe on ${devicePath}?\n\nThis is NON-DESTRUCTIVE — only reads random chunks\nto verify the device is all zeros.`
  );
  if (!confirmed) return;

  const settings = perDeviceSettings[devicePath] || { autoFormat: false, verifySizeGB: 1 };
  logMessage(`Starting test wipe on ${devicePath} (verify=${settings.verifySizeGB}GiB)...`);

  deviceStates[devicePath] = { wiping: true, percent: 0, speed: 0, status: 'verifying' };
  loadDevices();

  try {
    const res = await fetch('/api/test-wipe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        device: devicePath,
        verifySizeGB: settings.verifySizeGB
      })
    });

    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Failed to start test wipe');
    }

    logMessage('Test wipe started on ' + devicePath);
  } catch (err) {
    logMessage('Error: ' + err.message);
    delete deviceStates[devicePath];
  }

  loadDevices();
}

async function cancelDeviceWipe(devicePath) {
  const confirmed = confirm('Cancel wipe on ' + devicePath + '?');
  if (!confirmed) return;

  try {
    const res = await fetch('/api/cancel?device=' + encodeURIComponent(devicePath), { method: 'POST' });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Cancel failed');
    }
    logMessage('Cancelling wipe on ' + devicePath + '...');
  } catch (err) {
    logMessage('Error: ' + err.message);
  }
}

// ---- Cancel all ----

function updateCancelButton(anyWiping) {
  document.getElementById('cancel-all').disabled = !anyWiping;
}

async function cancelAllWipes() {
  const confirmed = confirm('Cancel ALL active wipes?');
  if (!confirmed) return;

  try {
    const res = await fetch('/api/cancel', { method: 'POST' });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Cancel failed');
    }
    logMessage('Cancelling all wipes...');
    deviceStates = {};
    loadDevices();
  } catch (err) {
    logMessage('Error: ' + err.message);
  }
}

// ---- SSE connection ----

function connectSSE() {
  if (eventSource) eventSource.close();

  eventSource = new EventSource('/api/events');

  eventSource.onmessage = (event) => {
    try {
      updateProgress(JSON.parse(event.data));
    } catch (e) { /* ignore */ }
  };

  eventSource.onerror = () => {
    setTimeout(connectSSE, 2000);
  };
}

function updateProgress(ev) {
  // Handle refresh event — full reload of device list and history
  if (ev.eventType === 'refresh') {
    handleRefreshEvent();
    return;
  }

  if (ev.message) logMessage(ev.message);
  if (!ev.devicePath) return;

  if (ev.status === 'running' || ev.status === 'verifying') {
    deviceStates[ev.devicePath] = {
      wiping: true,
      percent: ev.percent || 0,
      speed: ev.speed || 0,
      status: ev.status,
      totalBytes: ev.totalBytes,
      bytesWritten: ev.bytesWritten
    };
    updatePerDeviceRow(ev.devicePath);
  } else if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled') {
    deviceStates[ev.devicePath] = {
      wiping: false,
      percent: 100,
      speed: 0,
      status: ev.status,
      verified: ev.verified,
      bytesVerified: ev.bytesVerified
    };
    updatePerDeviceRow(ev.devicePath);

    // Clear cached history so expanded row re-fetches on next refresh
    delete deviceHistory[ev.devicePath];
  }
}

function updatePerDeviceRow(devicePath) {
  const st = deviceStates[devicePath];
  if (!st) return;

  const row = document.querySelector(`tr.device-row[data-device="${devicePath}"]`);
  if (!row) return;

  // Status (cell 3)
  if (row.cells[3]) {
    if (st.status === 'verifying') {
      row.cells[3].innerHTML = '<span class="status-verifying">Verifying</span>';
    } else if (st.wiping) {
      row.cells[3].innerHTML = '<span class="status-wiping">Wiping</span>';
    } else if (st.status === 'completed') {
      row.cells[3].innerHTML = '<span class="status-passed">✓ Wiped</span>';
    } else if (st.status === 'failed') {
      row.cells[3].innerHTML = '<span class="status-failed">Failed</span>';
    } else if (st.status === 'cancelled') {
      row.cells[3].innerHTML = '<span class="status-cancelled">Cancelled</span>';
    }
  }

  // Progress (cell 4)
  if (row.cells[4]) {
    if (st.wiping || st.status === 'verifying') {
      row.cells[4].innerHTML =
        '<div class="mini-progress">' +
        '<progress value="' + st.percent + '" max="100"></progress>' +
        '<span class="mini-pct">' + st.percent.toFixed(1) + '%</span>' +
        (st.speed > 0 ? '<span class="mini-speed">' + formatBytes(st.speed) + '/s</span>' : '') +
        '</div>';
    } else {
      row.cells[4].innerHTML = '<span class="mini-done">100%</span>';
    }
  }

  // Wipe + Test buttons (cell 5 — now has two buttons)
  if (row.cells[5] && row.cells[5].childNodes.length >= 2) {
    const btnWipe = row.cells[5].childNodes[0];
    const btnTest = row.cells[5].childNodes[1];
    if (st.wiping) {
      btnWipe.textContent = 'Cancel';
      btnWipe.className = 'btn btn-cancel';
      btnWipe.onclick = (e) => { e.stopPropagation(); cancelDeviceWipe(devicePath); };
      btnTest.textContent = '—';
      btnTest.className = 'btn';
      btnTest.disabled = true;
    } else {
      btnWipe.textContent = 'Wipe';
      btnWipe.className = 'btn btn-danger';
      btnWipe.onclick = (e) => { e.stopPropagation(); startDeviceWipe(devicePath); };
      btnTest.textContent = 'Test';
      btnTest.className = 'btn btn-test';
      btnTest.disabled = false;
      btnTest.onclick = (e) => { e.stopPropagation(); startTestWipe(devicePath); };
    }
  }

  updateCancelButton(Object.values(deviceStates).some(s => s.wiping));
}

// ---- Helpers ----

function logMessage(msg) {
  const log = document.getElementById('log');
  const time = new Date().toLocaleTimeString();
  log.textContent = '[' + time + '] ' + msg + '\n' + log.textContent;
  const lines = log.textContent.split('\n');
  if (lines.length > 50) log.textContent = lines.slice(0, 50).join('\n');
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

// ---- Init ----

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('refresh').addEventListener('click', () => {
    // Clear caches on manual refresh
    deviceHealth = {};
    deviceHistory = {};
    loadDevices();
  });
  document.getElementById('cancel-all').addEventListener('click', cancelAllWipes);

  loadDevices();
  connectSSE();
  // No polling — SSE push events keep the UI up to date in real time.
  // Use the manual Refresh button to detect newly plugged/unplugged devices.
  // Periodic refresh (respect expand state)
});
