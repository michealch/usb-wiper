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

async function apiPost(path, body = {}) {
  const res = await fetch(BASE + path, {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

async function apiPut(path, body = {}) {
  const res = await fetch(BASE + path, {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(body)
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || res.statusText);
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
    reconnectFailures = 0;
    reconnectDelay = 2000;
  };

  eventSource.onerror = () => {
    reconnectFailures++;
    if (reconnectFailures >= maxReconnectFailures) {
      logEvent('SSE connection lost after ' + maxReconnectFailures + ' attempts — refresh the page', 'error');
      return;
    }
    const delay = reconnectDelay;
    reconnectDelay = Math.min(delay * backoffFactor, maxReconnectDelay);
    setTimeout(() => connectSSE(handlers), delay);
  };

  return eventSource;
}

function closeSSE() {
  if (eventSource) { eventSource.close(); eventSource = null; }
}

function logEvent(msg, type = 'info') {
  const logBody = document.getElementById('log-body');
  if (!logBody) return;
  const time = new Date().toLocaleTimeString();
  const entry = document.createElement('div');
  entry.className = 'log-entry';
  entry.innerHTML = `<span class="ts">[${time}]</span><span class="msg">${escapeHtml(msg)}</span>`;
  logBody.prepend(entry);
  // Keep last 100 entries
  while (logBody.children.length > 100) logBody.lastChild.remove();
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

export { apiGet, apiPost, apiPut, apiDelete, connectSSE, closeSSE, logEvent };
