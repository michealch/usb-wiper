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

// Shared progress store — maps devicePath to latest progress state from SSE.
// All views read from this for initial render and `patchProgressDOM` updates the
// corresponding DOM elements without requiring a full re-render.
export const progressMap = new Map();

function updateProgressState(ev) {
  if (!ev.devicePath || ev.percent === undefined) return;
  progressMap.set(ev.devicePath, {
    percent: ev.percent,
    currentPass: ev.currentPass || 1,
    totalPasses: ev.totalPasses || 1,
    status: ev.status || ''
  });
}

/**
 * Patch progress elements in all views that display a progress bar for
 * the given device. Uses [data-device] selectors to find the container
 * and updates <progress>, .progress-pct, and .progress-pass inside it.
 * Works across Devices, Dashboard, and Queue views without re-rendering.
 */
function patchProgressDOM(ev) {
  if (!ev.devicePath || ev.percent === undefined) return;
  const esc = CSS.escape(ev.devicePath);
  const containers = document.querySelectorAll(`[data-device="${esc}"]`);
  containers.forEach(container => {
    const progressEl = container.querySelector('progress');
    if (progressEl) progressEl.value = ev.percent;

    const pctSpan = container.querySelector('.progress-pct');
    if (pctSpan) pctSpan.textContent = ev.percent.toFixed(1) + '%';

    const passSpan = container.querySelector('.progress-pass');
    if (passSpan && ev.currentPass && ev.totalPasses > 1) {
      passSpan.textContent = 'Pass ' + ev.currentPass + '/' + ev.totalPasses;
      passSpan.style.display = '';
    }
  });

  // Also patch segmented multi-pass bars (dashboard/queue job cards).
  containers.forEach(container => {
    const segments = container.querySelectorAll('.progress-segment');
    if (segments.length > 0 && ev.currentPass && ev.totalPasses > 1) {
      let pctSoFar = 0;
      for (let i = 0; i < ev.currentPass - 1; i++) {
        if (segments[i]) segments[i].className = 'progress-segment completed';
        pctSoFar++;
      }
      // Active segment: fill based on percent within current pass.
      const perPass = 100 / ev.totalPasses;
      const within = ev.percent - ((ev.currentPass - 1) * perPass);
      const activeIdx = ev.currentPass - 1;
      if (segments[activeIdx]) {
        segments[activeIdx].className = 'progress-segment active';
        segments[activeIdx].style.width = within + '%';
      }
      // Completed segments get full share.
      for (let i = 0; i < ev.currentPass - 1; i++) {
        if (segments[i]) segments[i].style.width = perPass + '%';
      }
      // Pending segments.
      for (let i = ev.currentPass; i < segments.length; i++) {
        if (segments[i]) {
          segments[i].className = 'progress-segment pending';
          segments[i].style.width = perPass + '%';
        }
      }
    }
  });
}

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
      // Always update shared progress store.
      updateProgressState(ev);
      // Patch DOM progress bars in all views (Devices, Dashboard, Queue).
      if (ev.devicePath && ev.percent !== undefined) {
        patchProgressDOM(ev);
      }
      // Full refresh on terminal states.
      if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled' || ev.status === 'queued') {
        refreshCurrentView();
      }
    },
    onProgress: (ev) => {
      if (ev.message) logEvent(ev.message);
      updateProgressState(ev);
      if (ev.devicePath) {
        patchProgressDOM(ev);
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
