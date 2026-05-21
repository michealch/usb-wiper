// USB Wiper — vanilla JavaScript frontend

let selectedDevice = null;
let eventSource = null;
let wipeInProgress = false; // tracks if ANY device is wiping

// Restore selected device from sessionStorage on page load
(function restoreSelection() {
  const saved = sessionStorage.getItem('selectedDevice');
  if (saved) {
    selectedDevice = saved;
  }
})();

// ---- Device list ----

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
      return;
    }
    noDevices.style.display = 'none';

    let anyWiping = false;

    devices.forEach(d => {
      const row = tbody.insertRow();
      row.insertCell().textContent = d.path;
      row.insertCell().textContent = d.model || '—';
      row.insertCell().textContent = formatBytes(d.sizeBytes);
      row.insertCell().textContent = d.mounted ? '✓ ' + d.mountPoints.join(', ') : 'No';

      // Status column: show wipe-block reason, wiping progress, or "Ready"
      const statusCell = row.insertCell();
      if (d.wiping) {
        anyWiping = true;
        const pct = (d.wipePercent || 0).toFixed(1);
        statusCell.innerHTML = `<span style="color:#4caf50">Wiping ${pct}%</span>`;
        statusCell.title = d.wipeStatus || 'running';
      } else if (d.wipeBlocked) {
        statusCell.textContent = '⚠ Blocked';
        statusCell.title = d.blockReason || '';
        statusCell.style.color = '#e6a817';
      } else {
        statusCell.textContent = 'Ready';
        statusCell.style.color = '#4caf50';
      }

      // Action button
      const sel = row.insertCell();
      const btn = document.createElement('button');
      btn.className = 'btn';
      if (d.wiping) {
        btn.textContent = 'Wiping...';
        btn.disabled = true;
      } else if (d.wipeBlocked) {
        btn.textContent = 'Inspect';
        btn.onclick = () => selectDevice(d.path);
      } else {
        btn.textContent = 'Wipe';
        btn.className = 'btn btn-danger';
        btn.onclick = () => startDeviceWipe(d.path);
        // Click on the row itself to inspect health
        row.style.cursor = 'pointer';
        row.onclick = () => selectDevice(d.path);
      }
      sel.appendChild(btn);

      if (d.path === selectedDevice) {
        row.style.background = 'rgba(15, 52, 96, 0.4)';
      }
    });

    // Track global wipe state
    wipeInProgress = anyWiping;
    updateWipeUI();

  } catch (err) {
    logMessage('Error loading devices: ' + err.message);
  }
}

// ---- Per-device wipe ----

async function startDeviceWipe(devicePath) {
  const confirmed = confirm(
    `DESTROY ALL DATA on ${devicePath}?\n\nThis cannot be undone.`
  );
  if (!confirmed) return;

  const autoFormat = document.getElementById('auto-format').checked;

  logMessage(`Starting wipe on ${devicePath}...`);

  try {
    const res = await fetch('/api/wipe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device: devicePath, autoFormat })
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
  }

  loadDevices();
}

// ---- Device selection (for health / inspection) ----

function selectDevice(path) {
  selectedDevice = path;
  sessionStorage.setItem('selectedDevice', path);
  document.getElementById('health').style.display = 'block';
  document.getElementById('health-device').textContent = path;
  loadHealth(path);
  loadDevices(); // Re-render to highlight
}

// ---- Health ----

async function loadHealth(path) {
  const table = document.getElementById('health-table');
  table.innerHTML = '<tr><td>Loading...</td></tr>';

  try {
    const res = await fetch('/api/health?device=' + encodeURIComponent(path));
    if (!res.ok) throw new Error('Failed to load health');
    const h = await res.json();

    const rows = [
      ['Status', h.healthStatus || 'UNKNOWN'],
      ['Model', h.modelName || '—'],
      ['Serial', h.serialNumber || '—'],
      ['Firmware', h.firmwareVersion || '—'],
      ['Capacity', formatBytes(h.capacityBytes)],
      ['Power On Hours', h.powerOnHours ? h.powerOnHours.toLocaleString() : '—'],
      ['Power Cycles', h.powerCycleCount ? h.powerCycleCount.toLocaleString() : '—'],
      ['Temperature', h.temperatureC ? h.temperatureC + '°C' : '—'],
      ['Reallocated Sectors', h.reallocatedSectors ? h.reallocatedSectors.toLocaleString() : '0'],
      ['Pending Sectors', h.pendingSectors ? h.pendingSectors.toLocaleString() : '0'],
      ['Uncorrectable Errors', h.uncorrectableErrors ? h.uncorrectableErrors.toLocaleString() : '0'],
    ];

    table.innerHTML = rows.map(([k, v]) =>
      `<tr><td>${k}</td><td>${v}</td></tr>`
    ).join('');

  } catch (err) {
    table.innerHTML = '<tr><td>Health data unavailable</td></tr>';
  }
}

// ---- Global wipe bar ----

function updateWipeUI() {
  const cancelBtn = document.getElementById('cancel');

  if (wipeInProgress) {
    cancelBtn.disabled = false;
  } else {
    cancelBtn.disabled = true;
  }
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
  } catch (err) {
    logMessage('Error: ' + err.message);
  }
}

// ---- SSE connection ----

function connectSSE() {
  if (eventSource) {
    eventSource.close();
  }

  eventSource = new EventSource('/api/events');

  eventSource.onmessage = (event) => {
    try {
      const ev = JSON.parse(event.data);
      updateProgress(ev);
    } catch (e) {
      // ignore malformed events
    }
  };

  eventSource.onerror = () => {
    setTimeout(connectSSE, 2000);
  };
}

function updateProgress(ev) {
  const progress = document.getElementById('progress');
  const progressText = document.getElementById('progress-text');

  if (ev.message) {
    logMessage(ev.message);
  }

  // Update per-device progress for the device being wiped
  if (ev.totalBytes > 0 && ev.devicePath) {
    // Update the global progress bar with the latest device
    progress.value = ev.percent || 0;
    progressText.textContent = (ev.percent || 0).toFixed(1) + '%';
  }

  if (ev.status === 'completed') {
    logMessage('=== Wipe completed for ' + ev.devicePath + ' ===');
    loadDevices();
  } else if (ev.status === 'failed') {
    logMessage('=== Wipe failed for ' + ev.devicePath + ' ===');
    loadDevices();
  } else if (ev.status === 'cancelled') {
    logMessage('=== Wipe cancelled for ' + ev.devicePath + ' ===');
    loadDevices();
  }
}

// ---- Config toggle ----

document.getElementById('auto-format').addEventListener('change', async (e) => {
  try {
    await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ autoFormat: e.target.checked })
    });
  } catch (err) {
    logMessage('Config update failed: ' + err.message);
  }
});

// ---- UI helpers ----

function logMessage(msg) {
  const log = document.getElementById('log');
  const time = new Date().toLocaleTimeString();
  log.textContent = '[' + time + '] ' + msg + '\n' + log.textContent;
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) {
    size /= 1024;
    i++;
  }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

// ---- Init ----

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('refresh').addEventListener('click', loadDevices);
  document.getElementById('cancel').addEventListener('click', cancelAllWipes);

  loadDevices();
  connectSSE();

  // Restore health panel if a device was previously selected
  if (selectedDevice) {
    document.getElementById('health').style.display = 'block';
    document.getElementById('health-device').textContent = selectedDevice;
    loadHealth(selectedDevice);
  }

  // Refresh devices every 5 seconds
  setInterval(loadDevices, 5000);
});
