// USB Wiper — main application bootstrap and hash router
import { apiGet, connectSSE, logEvent } from './api.js';
import { loadSettings } from './state.js';
import { initDrawer } from './components/drawer.js';
import { showToast } from './components/toast.js';
import { segmentWidths } from './util.js';
import { updateWipe } from './views/wipe.js';
import { renderRecords } from './views/records.js';
import { renderSettings } from './views/settings.js';

// Current route
let currentRoute = 'wipe';
let currentTab = null;

const ROUTES = ['wipe', 'records', 'settings'];
const TITLES = { wipe: 'Wipe', records: 'Records', settings: 'Settings' };
const DEFAULT_TAB = { records: 'history', settings: 'general' };
const ROUTE_ALIASES = {
  dashboard: 'wipe',
  devices: 'wipe',
  queue: 'wipe',
  history: 'records/history',
  activity: 'records/activity',
  autowipe: 'settings/autowipe',
  presets: 'settings/presets'
};

// parseHash maps any hash — canonical, legacy, or junk — onto a live route.
// `rewrite` marks hashes that must be canonicalised in the address bar.
function parseHash() {
  const raw = window.location.hash.replace(/^#\/?/, '');
  const [first, second] = raw.split('/');
  if (!first) return { route: 'wipe', tab: null, rewrite: false };
  if (ROUTE_ALIASES[first]) {
    const [route, tab] = ROUTE_ALIASES[first].split('/');
    return { route, tab: tab || null, rewrite: true };
  }
  if (!ROUTES.includes(first)) return { route: 'wipe', tab: null, rewrite: true };
  return { route: first, tab: second || null, rewrite: false };
}

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

function patchProgressDOM(ev) {
  if (!ev.devicePath || ev.percent === undefined) return;
  const containers = document.querySelectorAll(`[data-device="${CSS.escape(ev.devicePath)}"]`);
  const widths = segmentWidths(ev.currentPass, ev.totalPasses, ev.percent);
  containers.forEach(container => {
    const pct = container.querySelector('.progress-pct');
    if (pct) pct.textContent = ev.percent.toFixed(1) + '%';

    const passEl = container.querySelector('.progress-pass');
    if (passEl && widths.length > 1) {
      passEl.textContent = 'Pass ' + Math.min(ev.currentPass || 1, widths.length) + '/' + widths.length;
    }

    const bar = container.querySelector('.progress-multi');
    if (bar) bar.setAttribute('aria-valuenow', String(Math.round(ev.percent)));

    const segments = container.querySelectorAll('.progress-segment');
    if (segments.length === widths.length) {
      widths.forEach((w, i) => {
        segments[i].className = 'progress-segment ' + w.state;
        segments[i].style.width = w.width + '%';
      });
    }
  });
}

function setSSEBanner(state) {
  const banner = document.getElementById('sse-banner');
  if (!banner) return;
  if (state === 'reconnecting') {
    banner.textContent = 'Reconnecting…';
    banner.className = 'sse-banner visible';
  } else if (state === 'lost') {
    banner.textContent = 'Connection lost — refresh the page to resume live updates.';
    banner.className = 'sse-banner lost visible';
  } else {
    banner.className = 'sse-banner';
  }
}

// Initialize app
document.addEventListener('DOMContentLoaded', () => {
  initTheme();
  initDrawer();
  initNavigation();
  initSSE();
  loadSettings().catch(() => {});
  navigateTo(parseHash());
  initTopbarButtons();
});

function initTheme() {
  const saved = localStorage.getItem('theme');
  const validThemes = ['light', 'dark'];
  if (saved && validThemes.includes(saved)) {
    document.documentElement.setAttribute('data-theme', saved);
    return;
  }
  // Auto-detect OS preference if no saved theme
  if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
    document.documentElement.setAttribute('data-theme', 'dark');
  } else if (window.matchMedia('(prefers-color-scheme: light)').matches) {
    document.documentElement.setAttribute('data-theme', 'light');
  }
}

function initNavigation() {
  document.querySelectorAll('.sidebar-link').forEach(link => {
    link.addEventListener('click', (e) => {
      e.preventDefault();
      const route = link.dataset.route;
      if (route === currentRoute) {
        // Same route: navigate in place. Writing location.hash would queue a
        // hashchange that re-enters navigateTo with a stale tab and clobbers
        // the just-selected tab.
        navigateTo(parseHash());
        return;
      }
      window.location.hash = '#/' + route;
      navigateTo(parseHash());
    });
  });

  window.addEventListener('hashchange', () => {
    const p = parseHash();
    const pTab = p.tab || DEFAULT_TAB[p.route] || null;
    // A pending hashchange can land after an in-page replaceState (tab select,
    // alias rewrite) has already canonicalised the hash. If the parsed route
    // and tab match the live state, the navigation already happened — skip the
    // re-render rather than clobber the just-activated tab.
    if (p.route === currentRoute && pTab === currentTab) return;
    navigateTo(p);
  });
}

function navigateTo({ route, tab, rewrite }) {
  currentRoute = route;
  currentTab = tab || DEFAULT_TAB[route] || null;

  if (rewrite) {
    // Canonicalise legacy/junk hashes without adding a history entry.
    history.replaceState(null, '', '#/' + route + (currentTab ? '/' + currentTab : ''));
  }

  // Update active nav
  document.querySelectorAll('.sidebar-link').forEach(l => {
    l.classList.toggle('active', l.dataset.route === route);
  });

  // Show correct view
  document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
  const viewEl = document.getElementById('view-' + route);
  if (viewEl) viewEl.classList.add('active');

  // Update topbar title
  document.getElementById('topbar-title').textContent = TITLES[route] || route;

  // Load data for view.
  refreshCurrentView();
}

function refreshIfLive() {
  if (currentRoute === 'wipe') refreshCurrentView();
}

function initSSE() {
  connectSSE({
    onRefresh: () => {
      logEvent('Device list changed — refreshing');
      refreshIfLive();
    },
    onJob: (ev) => {
      // Always update shared progress store.
      updateProgressState(ev);
      // Patch DOM progress bars in all views.
      if (ev.devicePath && ev.percent !== undefined) {
        patchProgressDOM(ev);
      }
      // Full refresh on terminal states (gated to the Wipe route so an SSE
      // terminal event never discards in-progress edits on other screens).
      if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled' || ev.status === 'queued') {
        refreshIfLive();
      }
    },
    onProgress: (ev) => {
      if (ev.message) logEvent(ev.message);
      updateProgressState(ev);
      if (ev.devicePath) {
        patchProgressDOM(ev);
      }
      if (ev.status === 'completed' || ev.status === 'failed' || ev.status === 'cancelled') {
        refreshIfLive();
      }
    },
    onConnectionChange: (state) => setSSEBanner(state)
  });
}

export function refreshCurrentView() {
  switch (currentRoute) {
    case 'wipe':
      loadDevicesAndJobs().then(({ devices, jobs }) => updateWipe(devices, jobs));
      break;
    case 'records':
      renderRecords(currentTab || DEFAULT_TAB.records);
      break;
    case 'settings':
      renderSettings(currentTab || DEFAULT_TAB.settings);
      break;
  }
}

async function loadDevicesAndJobs() {
  const [deviceResult, jobResult] = await Promise.allSettled([
    apiGet('/api/devices'),
    apiGet('/api/jobs')
  ]);

  let devices = [];
  let jobs = [];

  if (deviceResult.status === 'fulfilled') {
    devices = deviceResult.value.devices || [];
  } else {
    logEvent('Device list unavailable: ' + deviceResult.reason.message, 'error');
  }

  if (jobResult.status === 'fulfilled') {
    jobs = jobResult.value.jobs || [];
  } else {
    logEvent('Job list unavailable: ' + jobResult.reason.message, 'error');
  }

  return { devices, jobs };
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
    const svg = document.querySelector('#btn-refresh-all svg');
    if (svg) {
      svg.classList.add('spin');
      setTimeout(() => svg.classList.remove('spin'), 500);
    }
    refreshCurrentView();
    showToast('Refreshed', 'info', 1500);
  });

  // Keyboard shortcuts
  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT' || e.target.tagName === 'TEXTAREA') return;
    if (e.key === 'r' && !e.ctrlKey && !e.metaKey) {
      refreshCurrentView();
      showToast('Refreshed', 'info', 1000);
    }
  });
}
