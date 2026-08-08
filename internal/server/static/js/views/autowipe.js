// Auto Wipe panel renderer — used by the Settings screen's Auto Wipe tab.

import { apiGet, apiPut, apiDelete } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';
import { escapeHtml, formatDateTime, emptyState } from '../util.js';

let autoWipeState = null;

// renderAutoWipePanel(el) renders the Auto Wipe controls into el. All post-action
// reloads target the same element.
export async function renderAutoWipePanel(el) {
  try {
    autoWipeState = await apiGet('/api/autowipe');
  } catch (e) {
    el.innerHTML = `<p class="text-danger">Failed to load auto wipe: ${escapeHtml(e.message)}</p>`;
    return;
  }

  const s = autoWipeState || {};
  const seen = s.seen || [];
  const enabled = !!s.enabled;
  const unavailable = s.available === false;
  const queued = seen.filter(r => r.lastAction === 'queued').length;
  const skipped = seen.filter(r => r.lastAction === 'skipped' || r.lastAction === 'error' || r.lastAction === 'conflict').length;

  el.innerHTML = `
    <div class="panel-header">
      <div>
        <div class="panel-title">Auto Wipe</div>
        <div class="panel-note">New trusted disk serials use the default wipe scheme.</div>
      </div>
      <button class="btn ${enabled ? 'btn-danger' : 'btn-primary'}" id="auto-wipe-toggle" ${unavailable ? 'disabled' : ''}>
        ${enabled ? 'Disable' : 'Enable'}
      </button>
    </div>

    <div class="metric-strip">
      <div class="metric-pill"><span class="metric-label">Seen serials</span><span class="metric-value">${seen.length}</span></div>
      <div class="metric-pill"><span class="metric-label">Auto queued</span><span class="metric-value">${queued}</span></div>
    </div>

    ${unavailable ? `
      <div class="inline-alert warning">Auto wipe state storage is unavailable. The feature cannot be enabled until local persistence is healthy.</div>
    ` : `
      <div class="inline-alert warning">When enabled, already-connected drives are marked seen first. Only later attached drives with trusted identities and real serials are eligible.</div>
    `}

    <section class="panel">
      <div class="panel-header">
        <div>
          <div class="panel-title">Seen devices</div>
          <div class="panel-note">${skipped} skipped or conflicted</div>
        </div>
        <button class="btn btn-sm" id="auto-wipe-clear" ${seen.length === 0 || unavailable ? 'disabled' : ''}>Clear Seen</button>
      </div>
      ${seen.length === 0 ? `
        ${emptyState('No seen serials yet.', 'The ledger fills when Auto Wipe is enabled or a new eligible device is attached.')}` : `
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
                <td data-label="Last seen" style="font-size:var(--text-xs);color:var(--color-text-dim)">${formatDateTime(r.lastSeenAt)}</td>
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
      <div class="inline-alert">Uses the current default settings. Change the default scheme and verification policy in Settings.</div>
    </section>
  `;

  const toggleBtn = el.querySelector('#auto-wipe-toggle');
  if (toggleBtn) {
    toggleBtn.onclick = () => toggleAutoWipe(!enabled, el);
  }

  const clearBtn = el.querySelector('#auto-wipe-clear');
  if (clearBtn) {
    clearBtn.onclick = () => {
      showConfirm('Clear the Auto Wipe seen-device ledger?', {
        dangerLabel: 'Clear',
        onConfirm: async () => {
          try {
            await apiDelete('/api/autowipe/seen');
            showToast('Seen devices cleared', 'success');
            renderAutoWipePanel(el);
          } catch (e) {
            showToast(e.message, 'error');
          }
        }
      });
    };
  }
}

async function toggleAutoWipe(enabled, el) {
  try {
    const result = await apiPut('/api/autowipe', { enabled });
    showToast(enabled ? `Auto Wipe enabled. ${result.connectedMarked || 0} connected device(s) marked seen.` : 'Auto Wipe disabled', enabled ? 'warning' : 'success');
    renderAutoWipePanel(el);
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
