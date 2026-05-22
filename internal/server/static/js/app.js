// USB Wiper — main application bootstrap and hash router
import { apiGet, apiPost, connectSSE, logEvent } from './api.js';
import { loadDevices, loadJobs, loadHistory as loadHistoryState, loadPresets, loadSchemes, loadSettings, state } from './state.js';
import { initDrawer, openDrawer, deviceHealthCache, deviceHistoryCache } from './components/drawer.js';
import { showToast } from './components/toast.js';
import { renderDashboard, updateDashboard } from './views/dashboard.js';
import { renderDevices, updateDevices } from './views/devices.js';
import { renderQueue, updateQueue } from './views/queue.js';
import { loadAndRenderHistory } from './views/history.js';
import { loadAndRenderPresets } from './views/presets.js';
import { loadAndRenderSettings } from './views/settings.js';

// Current route
let currentRoute = 'dashboard';

// Initialize app
document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  initDrawer();
  initNavigation();
  initSSE();
  navigateTo(window.location.hash.replace('#/', '') || 'dashboard');
  initTopbarButtons();
});

function initTheme() {
  const saved = localStorage.getItem('theme');
  const validThemes = ['light', 'dark'];
  if (saved && validThemes.includes(saved)) {
    document.documentElement.setAttribute('data-theme', saved);
  }
}

function initNavigation() {
  document.querySelectorAll('.sidebar-link').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const route = link.dataset.route;
      window.location.hash = '#/' + route;
      navigateTo(route);
    });
  });

  window.addEventListener('hashchange', () => {
    navigateTo(window.location.hash.replace('#/', '') || 'dashboard');
  });
}

function navigateTo(route) {
  currentRoute = route;

  // Update active nav
  document.querySelectorAll('.sidebar-link').forEach(l => {
    l.classList.toggle('active', l.dataset.route === route);
  });

  // Show correct view
  document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
  const viewEl = document.getElementById('view-' + route);
  if (viewEl) viewEl.classList.add('active');

  // Update topbar title
  const title = { dashboard: 'Dashboard', devices: 'Devices', queue: 'Queue', history: 'History', presets: 'Presets', settings: 'Settings' };
  document.getElementById('topbar-title').textContent = title[route] || route;

  // Load data for view — reuse refreshCurrentView for consistency.
  switch (route) {
    case 'dashboard':
    case 'devices':
    case 'queue':
      refreshCurrentView();
      break;
    case 'history':
      loadAndRenderHistory();
      break;
    case 'presets':
      loadAndRenderPresets();
      break;
    case 'settings':
      loadAndRenderSettings();
      break;
  }
}

function initSSE() {
  connectSSE({
    onRefresh: () => {
      logEvent('Device list changed — refreshing');
      refreshCurrentView();
    },
    onJob: (ev) => {
      // Update inline progress if the event carries progress data
      if (ev.devicePath && ev.percent !== undefined) {
        updateInlineProgress(ev);
      }
      // Full refresh on terminal states or when no progress data
      if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled' || ev.status === 'queued') {
        refreshCurrentView();
      }
    },
    onProgress: (ev) => {
      if (ev.message) logEvent(ev.message);
      // Update progress bar in all views that show device rows
      if (ev.devicePath) {
        updateInlineProgress(ev);
      }
      if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled') {
        refreshCurrentView();
      }
    }
  });
}

function refreshCurrentView() {
  switch (currentRoute) {
    case 'dashboard':
    case 'devices':
      // Both views need devices + jobs — fetch once and pass to each renderer.
      Promise.all([apiGet('/api/devices'), apiGet('/api/jobs')]).then(([d, j]) => {
        const devices = d.devices || [];
        const jobs = j.jobs || [];
        if (currentRoute === 'dashboard') updateDashboard(devices, jobs);
        else updateDevices(devices, jobs);
      }).catch(() => {});
      break;
    case 'queue':
      apiGet('/api/jobs').then(j => updateQueue(j.jobs || [])).catch(() => {});
      break;
  }
}

function updateInlineProgress(ev) {
  const rows = document.querySelectorAll(`tr.clickable[data-device="${ev.devicePath}"]`);
  rows.forEach(row => {
    const progressCell = row.cells[4];
    if (progressCell && ev.percent !== undefined) {
      progressCell.innerHTML = `
        <div style="display:flex;align-items:center;gap:8px">
          <progress value="${ev.percent}" max="100"></progress>
          <span style="font-size:.78rem;font-weight:600">${ev.percent.toFixed(1)}%</span>
          ${ev.currentPass && ev.totalPasses > 1 ? `<span style="font-size:.72rem;color:var(--color-text-dim)">Pass ${ev.currentPass}/${ev.totalPasses}</span>` : ''}
        </div>`;
    }
  });
}

function initTopbarButtons() {
  document.getElementById('btn-theme-toggle').addEventListener('click', () => {
    const current = document.documentElement.getAttribute('data-theme');
    const next = current === 'light' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
    showToast('Theme: ' + next, 'info', 2000);
  });

  document.getElementById('btn-refresh-all').addEventListener('click', () => {
    refreshCurrentView();
    showToast('Refreshed', 'info', 1500);
  });
}

// Export for inline form handlers
window.startDeviceWipe = async function(device, scheme, autoFormat, verifyGB, label) {
  try {
    await apiPost('/api/wipe', { devices: [device], schemeId: scheme, autoFormat, verifySizeGB: parseInt(verifyGB), label });
    showToast('Wipe started on ' + device, 'success');
  } catch (e) { showToast(e.message, 'error'); }
};
