// API client — fetch wrappers + SSE connection

const BASE = '';

async function apiGet(path) {
  const res = await fetch(BASE + path);
  if (!res.ok) {
    const err = await res.json().catch(() => ({error: res.statusText}));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

async function readJsonBody(res) {
  if (res.status === 204) return {};
  const text = await res.text();
  if (!text) return {};
  try { return JSON.parse(text); }
  catch (_) { return { error: text || res.statusText }; }
}

async function apiPost(path, body = {}) {
  const res = await fetch(BASE + path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  });
  const data = await readJsonBody(res);
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return data;
}

async function apiPut(path, body = {}) {
  const res = await fetch(BASE + path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  });
  const data = await readJsonBody(res);
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.status = res.status;
    throw err;
  }
  return data;
}

async function apiDelete(path) {
  const res = await fetch(BASE + path, {method: 'DELETE'});
  if (!res.ok && res.status !== 204) {
    const err = await res.json().catch(() => ({error: res.statusText}));
    throw new Error(err.error || res.statusText);
  }
  if (res.status === 204) return {};
  return res.json();
}

// SSE connection with auto-reconnect and exponential backoff
let eventSource = null;
let reconnectDelay = 2000;
const maxReconnectDelay = 30000;
const backoffFactor = 2;
let reconnectFailures = 0;
const maxReconnectFailures = 10;

function connectSSE(handlers = {}) {
  if (eventSource) eventSource.close();

  eventSource = new EventSource('/api/events');

  eventSource.addEventListener('refresh', (e) => {
    reconnectFailures = 0;
    reconnectDelay = 2000;
    try { handlers.onRefresh && handlers.onRefresh(JSON.parse(e.data)); }
    catch (_) { handlers.onRefresh && handlers.onRefresh(); }
  });

  eventSource.addEventListener('job', (e) => {
    try { handlers.onJob && handlers.onJob(JSON.parse(e.data)); }
    catch (_) {}
  });

  eventSource.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (!data.eventType || data.eventType === 'progress') {
        handlers.onProgress && handlers.onProgress(data);
      }
    } catch (_) {}
  };

  eventSource.onopen = () => {
    const wasDisconnected = reconnectFailures > 0;
    reconnectFailures = 0;
    reconnectDelay = 2000;
    if (wasDisconnected) handlers.onConnectionChange && handlers.onConnectionChange('connected');
  };

  eventSource.onerror = () => {
    reconnectFailures++;
    if (reconnectFailures === 1) {
      handlers.onConnectionChange && handlers.onConnectionChange('reconnecting');
    }
    if (reconnectFailures >= maxReconnectFailures) {
      logEvent('SSE connection lost after ' + maxReconnectFailures + ' attempts — refresh the page', 'error');
      handlers.onConnectionChange && handlers.onConnectionChange('lost');
      return;
    }
    const delay = reconnectDelay;
    reconnectDelay = Math.min(delay * backoffFactor, maxReconnectDelay);
    setTimeout(() => connectSSE(handlers), delay);
  };

  return eventSource;
}

function logEvent(msg, type = 'info') {
  const time = new Date().toLocaleTimeString();

  // Update visible status bar text
  const statusText = document.getElementById('statusbar-text');
  if (statusText) {
    statusText.textContent = msg;
    statusText.style.color = type === 'error' ? 'var(--color-danger)' : type === 'warning' ? 'var(--color-warning)' : '';
    setTimeout(() => { if (statusText) statusText.style.color = ''; }, 4000);
  }
}

export { apiGet, apiPost, apiPut, apiDelete, connectSSE, logEvent };
