// Wipe Configurator — first-class destructive flow for 1..N devices.

import { apiGet, apiPost } from '../api.js';
import { showToast } from './toast.js';
import { holdConfirm } from './hold-confirm.js';
import { escapeHtml, escapeAttr, formatBytes } from '../util.js';
import { state } from '../state.js';

let overlay = null;
let lastFocused = null;

function buildOverlay() {
  overlay = document.createElement('div');
  overlay.className = 'configurator-overlay';
  overlay.innerHTML = `
    <div class="configurator" role="dialog" aria-modal="true" aria-labelledby="configurator-title">
      <div class="configurator-header">
        <h2 id="configurator-title">Configure wipe</h2>
        <button class="btn btn-ghost btn-sm" id="configurator-close" aria-label="Close">×</button>
      </div>
      <div class="configurator-body" id="configurator-body"></div>
      <div class="configurator-footer" id="configurator-footer"></div>
    </div>
  `;
  document.body.appendChild(overlay);

  overlay.querySelector('#configurator-close').onclick = closeConfigurator;
  overlay.onclick = (e) => { if (e.target === overlay) closeConfigurator(); };
  document.addEventListener('keydown', escHandler);
}

function escHandler(e) {
  if (e.key === 'Escape' && overlay && overlay.classList.contains('open')) {
    closeConfigurator();
  }
}

function closeConfigurator() {
  if (!overlay) return;
  overlay.classList.remove('open');
  setTimeout(() => {
    if (overlay) { overlay.remove(); overlay = null; }
  }, 200);
  document.removeEventListener('keydown', escHandler);
  if (lastFocused && lastFocused.focus) lastFocused.focus();
}

async function openConfigurator(devices) {
  lastFocused = document.activeElement;
  if (!overlay) buildOverlay();

  const body = overlay.querySelector('#configurator-body');
  const footer = overlay.querySelector('#configurator-footer');
  body.innerHTML = '<p class="muted">Loading schemes…</p>';
  footer.innerHTML = '';

  requestAnimationFrame(() => overlay.classList.add('open'));

  let schemes = [];
  let presets = [];
  try {
    [schemes, presets] = await Promise.all([
      apiGet('/api/schemes').then(d => d.schemes || []),
      apiGet('/api/presets').then(d => d.presets || [])
    ]);
  } catch (e) {
    body.innerHTML = `<p class="text-danger">Failed to load wipe schemes: ${escapeHtml(e.message)}</p>`;
    return;
  }

  renderConfigurator(devices, schemes, presets);

  const closeBtn = overlay.querySelector('#configurator-close');
  if (closeBtn) closeBtn.focus();
}

function renderConfigurator(devices, schemes, presets) {
  const body = overlay.querySelector('#configurator-body');
  const footer = overlay.querySelector('#configurator-footer');
  const title = overlay.querySelector('#configurator-title');

  const eligible = devices.filter(d => !d.wipeBlocked);
  const blocked = devices.filter(d => d.wipeBlocked);
  const totalBytes = eligible.reduce((sum, d) => sum + (d.sizeBytes || 0), 0);

  title.textContent = devices.length === 1 ? 'Configure wipe' : `Configure wipe — ${devices.length} devices`;

  const settings = state.settings || {};
  const defaultScheme = settings.defaultSchemeId || (schemes[0] && schemes[0].id) || 'zero';
  const defaultVerify = settings.verifySizeGB != null ? settings.verifySizeGB : 1;

  body.innerHTML = `
    <div class="configurator-section">
      <h3>Targets</h3>
      <div class="target-list">
        ${devices.map(d => `
          <div class="target-item ${d.wipeBlocked ? 'excluded' : ''}">
            <div>
              <div class="target-item-name">${escapeHtml(d.model || 'Unknown device')}</div>
              <div class="target-item-meta">${escapeHtml(d.path)} · ${escapeHtml(d.serial || '—')}</div>
            </div>
            <div style="text-align:right">
              <div>${formatBytes(d.sizeBytes)}</div>
              ${d.wipeBlocked ? `<div class="text-danger" style="font-size:var(--text-xs)">${escapeHtml(d.blockReason || 'Blocked')}</div>` : ''}
            </div>
          </div>
        `).join('')}
      </div>
      <div class="target-summary">
        ${eligible.length === 0
          ? `<span class="text-danger">No eligible devices — all selected devices are blocked.</span>`
          : `Erasing <strong>${eligible.length} device${eligible.length !== 1 ? 's' : ''} · ${formatBytes(totalBytes)}</strong>${blocked.length ? ` (${blocked.length} excluded)` : ''}`}
      </div>
    </div>

    <div class="configurator-section">
      <h3>Scheme</h3>
      <div class="form-group">
        <label for="cfg-scheme">Wipe scheme</label>
        <select id="cfg-scheme" class="w-full">
          ${schemes.map(s => `<option value="${escapeAttr(s.id)}" ${s.id === defaultScheme ? 'selected' : ''}>${escapeHtml(s.displayName)} (${s.passes} pass${s.passes > 1 ? 'es' : ''})</option>`).join('')}
        </select>
      </div>
      <div class="form-group">
        <label for="cfg-preset">Preset</label>
        <select id="cfg-preset" class="w-full">
          <option value="">— Manual —</option>
          ${presets.map(p => `<option value="${escapeAttr(p.id)}">${escapeHtml(p.name)} (${escapeHtml(p.schemeId)}, ${p.verifySizeGB}GiB verify)</option>`).join('')}
        </select>
      </div>
    </div>

    <div class="configurator-section">
      <h3>Options</h3>
      <div class="form-group">
        <label class="checkbox-label"><input type="checkbox" id="cfg-autoformat"> Auto-format FAT32 after wipe</label>
      </div>
      <div class="form-group">
        <label for="cfg-verify">Verification</label>
        <select id="cfg-verify" class="w-full">
          <option value="0" ${defaultVerify === 0 ? 'selected' : ''}>Off</option>
          <option value="1" ${defaultVerify === 1 ? 'selected' : ''}>1 GiB</option>
          <option value="2" ${defaultVerify === 2 ? 'selected' : ''}>2 GiB</option>
          <option value="4" ${defaultVerify === 4 ? 'selected' : ''}>4 GiB</option>
          <option value="16" ${defaultVerify === 16 ? 'selected' : ''}>16 GiB</option>
        </select>
      </div>
      <div class="form-group">
        <label for="cfg-label">Label (optional)</label>
        <input type="text" id="cfg-label" class="w-full" placeholder="e.g. RMA-2026-05">
      </div>
    </div>

    <div class="configurator-section">
      <div class="danger-summary">
        <strong>This permanently destroys all data</strong> on ${eligible.length} device${eligible.length !== 1 ? 's' : ''}. This cannot be undone.
      </div>
      <div aria-live="assertive" class="sr-only" id="cfg-live" style="position:absolute;width:1px;height:1px;overflow:hidden"></div>
      <button class="btn btn-danger w-full" id="cfg-confirm" ${eligible.length === 0 ? 'disabled' : ''}>
        Erase ${eligible.length} device${eligible.length !== 1 ? 's' : ''}
      </button>
      <div class="hold-confirm-hint">Press and hold (or Enter/Space) to confirm</div>
    </div>
  `;

  footer.innerHTML = `<button class="btn w-full" id="cfg-cancel">Cancel</button>`;
  footer.querySelector('#cfg-cancel').onclick = closeConfigurator;

  const presetSelect = body.querySelector('#cfg-preset');
  presetSelect.onchange = function () {
    const preset = presets.find(p => p.id === this.value);
    if (preset) {
      body.querySelector('#cfg-scheme').value = preset.schemeId;
      body.querySelector('#cfg-autoformat').checked = !!preset.autoFormat;
      body.querySelector('#cfg-verify').value = String(preset.verifySizeGB);
    }
  };

  if (eligible.length > 0) {
    const confirmBtn = body.querySelector('#cfg-confirm');
    const liveRegion = body.querySelector('#cfg-live');
    holdConfirm(confirmBtn, {
      durationMs: 2500,
      liveRegion,
      onConfirm: async () => {
        confirmBtn.disabled = true;
        try {
          const result = await apiPost('/api/wipe', {
            devices: eligible.map(d => d.path),
            schemeId: body.querySelector('#cfg-scheme').value,
            autoFormat: body.querySelector('#cfg-autoformat').checked,
            verifySizeGB: parseInt(body.querySelector('#cfg-verify').value),
            label: body.querySelector('#cfg-label').value
          });
          const started = Array.isArray(result.started) ? result.started : eligible.map(d => d.path);
          const conflicts = Array.isArray(result.conflicts) ? result.conflicts : [];
          showToast(`Wipe started on ${started.length} device${started.length !== 1 ? 's' : ''}`, 'success');
          if (conflicts.length > 0) showToast(`${conflicts.length} device(s) skipped — already active`, 'warning');
          closeConfigurator();
        } catch (e) {
          showToast(e.message, 'error');
          confirmBtn.disabled = false;
        }
      }
    });
  }
}

export { openConfigurator, closeConfigurator };
