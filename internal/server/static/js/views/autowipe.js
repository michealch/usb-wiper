// Auto Wipe view - controls automatic wipes for newly attached trusted serials.

import { apiGet, apiPut, apiDelete } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';
import { escapeHtml } from '../util.js';

let autoWipeState = null;

async function loadAndRenderAutoWipe() {
  const el = document.getElementById('view-autowipe');
  el.innerHTML = '<div class="panel"><p class="muted">Loading auto wipe...</p></div>';
  try {
    autoWipeState = await apiGet('/api/autowipe');
    renderAutoWipe();
  } catch (e) {
    el.innerHTML = `<div class="panel"><p class="text-danger">Failed to load auto wipe: ${escapeHtml(e.message)}</p></div>`;
  }
}

function renderAutoWipe() {
  const el = document.getElementById('view-autowipe');
  const s = autoWipeState || {};
  const seen = s.seen || [];
  const enabled = !!s.enabled;
  const unavailable = s.available === false;
  const queued = seen.filter(r => r.lastAction === 'queued').length;
  const skipped = seen.filter(r => r.lastAction === 'skipped' || r.lastAction === 'error' || r.lastAction === 'conflict').length;

  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Auto Wipe</div>
          <div class="page-subtitle">New trusted disk serials use the default wipe scheme.</div>
        </div>
        <div class="page-actions">
          <button class="btn ${enabled ? 'btn-danger' : 'btn-primary'}" id="auto-wipe-toggle" ${unavailable ? 'disabled' : ''}>
            ${enabled ? 'Disable' : 'Enable'}
          </button>
        </div>
      </div>

      <div class="metric-strip">
        <div class="metric-pill"><span class="metric-label">Mode</span><span class="metric-value">${enabled ? 'On' : 'Off'}</span></div>
        <div class="metric-pill"><span class="metric-label">Default scheme</span><span class="metric-value">${escapeHtml(s.defaultSchemeName || s.defaultSchemeId || 'zero')}</span></div>
        <div class="metric-pill"><span class="metric-label">Seen serials</span><span class="metric-value">${seen.length}</span></div>
        <div class="metric-pill"><span class="metric-label">Auto queued</span><span class="metric-value">${queued}</span></div>
      </div>

      ${unavailable ? `
        <div class="inline-alert warning">Auto wipe state storage is unavailable. The feature cannot be enabled until local persistence is healthy.</div>
      ` : `
        <div class="inline-alert warning">When enabled, already-connected drives are marked seen first. Only later attached drives with trusted identities and real serials are eligible.</div>
      `}

      <div class="split-grid">
        <section class="panel">
          <div class="panel-header">
            <div>
              <div class="panel-title">Seen devices</div>
              <div class="panel-note">${skipped} skipped or conflicted</div>
            </div>
            <button class="btn btn-sm" id="auto-wipe-clear" ${seen.length === 0 || unavailable ? 'disabled' : ''}>Clear Seen</button>
          </div>
          ${seen.length === 0 ? `
            <div class="empty-state">
              <div class="empty-state-title">No seen serials yet.</div>
              <div class="empty-state-subtitle">The ledger fills when Auto Wipe is enabled or a new eligible device is attached.</div>
            </div>` : `
            <div class="table-scroll table-stack">
              <table class="record-table">
                <thead><tr><th scope="col">Device</th><th scope="col">Identity</th><th scope="col">Last action</th><th scope="col">Last seen</th></tr></thead>
                <tbody>${seen.map(r => `
                  <tr>
                    <td data-label="Device">
                      <div class="record-primary">${escapeHtml(r.model || 'Unknown disk')}</div>
                      <div class="record-secondary">${escapeHtml(r.serial || 'No serial')}</div>
                    </td>
                    <td data-label="Identity">
                      <span class="badge">${escapeHtml(r.identityConfidence || '-')}</span>
                      <div class="record-secondary">${escapeHtml(r.identitySource || r.deviceId || '-')}</div>
                    </td>
                    <td data-label="Last action">
                      <div class="record-primary">${escapeHtml(actionLabel(r.lastAction))}</div>
                      ${r.lastMessage ? `<div class="record-secondary">${escapeHtml(r.lastMessage)}</div>` : ''}
                      ${r.lastJobId ? `<div class="record-secondary">Job ${escapeHtml(r.lastJobId)}</div>` : ''}
                    </td>
                    <td data-label="Last seen" style="font-size:var(--text-xs);color:var(--color-text-dim)">${escapeHtml(formatDate(r.lastSeenAt))}</td>
                  </tr>`).join('')}</tbody>
              </table>
            </div>`}
        </section>

        <section class="panel">
          <div class="panel-header"><div class="panel-title">Default action</div></div>
          <dl class="preset-meta mb-4">
            <dt>Scheme</dt><dd>${escapeHtml(s.defaultSchemeName || s.defaultSchemeId || 'zero')}</dd>
            <dt>Auto format</dt><dd>${s.autoFormat ? 'On' : 'Off'}</dd>
            <dt>Verify</dt><dd>${Number(s.verifySizeGB || 0) > 0 ? `${Number(s.verifySizeGB)} GiB` : 'Off'}</dd>
            <dt>Identity</dt><dd>Trusted serial</dd>
          </dl>
          <div class="stack">
            <div class="inline-alert">Uses the current default settings. Change the default scheme and verification policy in Settings.</div>
            <button class="btn ${enabled ? 'btn-danger' : 'btn-primary'} w-full" id="auto-wipe-toggle-secondary" ${unavailable ? 'disabled' : ''}>
              ${enabled ? 'Disable Auto Wipe' : 'Enable Auto Wipe'}
            </button>
          </div>
        </section>
      </div>
    </div>
  `;

  el.querySelectorAll('#auto-wipe-toggle, #auto-wipe-toggle-secondary').forEach(btn => {
    btn.onclick = () => toggleAutoWipe(!enabled);
  });

  const clearBtn = el.querySelector('#auto-wipe-clear');
  if (clearBtn) {
    clearBtn.onclick = () => {
      showConfirm('Clear the Auto Wipe seen-device ledger?', {
        dangerLabel: 'Clear',
        onConfirm: async () => {
          try {
            await apiDelete('/api/autowipe/seen');
            showToast('Seen devices cleared', 'success');
            loadAndRenderAutoWipe();
          } catch (e) {
            showToast(e.message, 'error');
          }
        }
      });
    };
  }
}

async function toggleAutoWipe(enabled) {
  try {
    const result = await apiPut('/api/autowipe', { enabled });
    showToast(enabled ? `Auto Wipe enabled. ${result.connectedMarked || 0} connected device(s) marked seen.` : 'Auto Wipe disabled', enabled ? 'warning' : 'success');
    loadAndRenderAutoWipe();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

function actionLabel(action) {
  const labels = {
    observed_on_enable: 'Seen on enable',
    observed_on_startup: 'Seen on startup',
    queued: 'Queued wipe',
    skipped: 'Skipped',
    conflict: 'Already queued',
    error: 'Error'
  };
  return labels[action] || action || 'Seen';
}

function formatDate(value) {
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '-';
  return d.toLocaleString();
}

export { loadAndRenderAutoWipe, renderAutoWipe };
