// USB Wiper — vanilla JavaScript frontend

let selectedDevice = null;
let eventSource = null;

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

    devices.forEach(d => {
      const row = tbody.insertRow();
      row.insertCell().textContent = d.path;
      row.insertCell().textContent = d.model || '—';
      row.insertCell().textContent = formatBytes(d.sizeBytes);
      row.insertCell().textContent = d.mounted ? '✓ ' + d.mountPoints.join(', ') : 'No';

      // Status column: show wipe-block reason or "Ready"
      const statusCell = row.insertCell();
      if (d.wipeBlocked) {
        statusCell.textContent = '⚠ Blocked';
        statusCell.title = d.blockReason || '';
        statusCell.style.color = '#e6a817';
      } else {
        statusCell.textContent = 'Ready';
        statusCell.style.color = '#4caf50';
      }

      const sel = row.insertCell();
      const btn = document.createElement('button');
      btn.className = 'btn';
      btn.textContent = d.wipeBlocked ? 'Inspect' : 'Select';
      btn.disabled = !!d.wipeBlocked;
      btn.onclick = () => selectDevice(d.path);
      sel.appendChild(btn);

      if (d.path === selectedDevice) {
        row.style.background = 'rgba(15, 52, 96, 0.4)';
      }
    });
  } catch (err) {
    logMessage('Error loading devices: ' + err.message);
  }
}

// ---- Device selection ----

function selectDevice(path) {
  selectedDevice = path;
  document.getElementById('start').disabled = false;
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

// ---- Wipe control ----

async function startWipe() {
  if (!selectedDevice) return;

  const confirmed = confirm(
    `DESTROY ALL DATA on ${selectedDevice}?\n\nThis cannot be undone.`
  );
  if (!confirmed) return;

  const autoFormat = document.getElementById('auto-format').checked;

  document.getElementById('start').disabled = true;
  document.getElementById('cancel').disabled = false;
  document.getElementById('log').textContent = 'Starting wipe...\n';

  try {
    const res = await fetch('/api/wipe', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device: selectedDevice, autoFormat })
    });

    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Failed to start wipe');
    }

    logMessage('Wipe started on ' + selectedDevice);

  } catch (err) {
    logMessage('Error: ' + err.message);
    document.getElementById('start').disabled = false;
    document.getElementById('cancel').disabled = true;
  }
}

async function cancelWipe() {
  try {
    const res = await fetch('/api/cancel', { method: 'POST' });
    if (!res.ok) {
      const err = await res.json();
      throw new Error(err.error || 'Cancel failed');
    }
    logMessage('Cancelling wipe...');
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
    // Reconnect after 2 seconds
    setTimeout(connectSSE, 2000);
  };
}

function updateProgress(ev) {
  const progress = document.getElementById('progress');
  const progressText = document.getElementById('progress-text');

  if (ev.totalBytes > 0) {
    progress.value = ev.percent || 0;
    progressText.textContent = (ev.percent || 0).toFixed(1) + '%';
  }

  if (ev.message) {
    logMessage(ev.message);
  }

  if (ev.status === 'completed') {
    document.getElementById('start').disabled = false;
    document.getElementById('cancel').disabled = true;
    logMessage('=== Wipe completed ===');
  } else if (ev.status === 'failed') {
    document.getElementById('start').disabled = false;
    document.getElementById('cancel').disabled = true;
    logMessage('=== Wipe failed ===');
  } else if (ev.status === 'cancelled') {
    document.getElementById('start').disabled = false;
    document.getElementById('cancel').disabled = true;
    logMessage('=== Wipe cancelled ===');
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
  document.getElementById('start').addEventListener('click', startWipe);
  document.getElementById('cancel').addEventListener('click', cancelWipe);

  loadDevices();
  connectSSE();

  // Refresh devices every 10 seconds
  setInterval(loadDevices, 10000);
});
