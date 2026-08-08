// Presets panel renderer — used by the Settings screen's Presets tab.

import { apiGet, apiPost, apiPut, apiDelete } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';
import { attachOverlay } from '../components/overlay.js';
import { escapeHtml, escapeAttr, emptyState, schemeOptions, verifySizeOptions } from '../util.js';

let presetsList = [];
let schemesList = [];
let editingPresetId = null;
let presetModalEl = null;
let presetModalHandle = null;

// renderPresetsPanel(el) renders the preset tiles into el. The preset modal is
// built lazily once and appended to document.body, surviving panel re-renders.
export async function renderPresetsPanel(el) {
  try {
    const [presetsData, schemesData] = await Promise.all([
      apiGet('/api/presets'),
      apiGet('/api/schemes').catch(() => ({ schemes: [] }))
    ]);
    presetsList = presetsData.presets || [];
    schemesList = schemesData.schemes || [];
  } catch (e) {
    el.innerHTML = `<p class="text-danger">Failed to load presets: ${escapeHtml(e.message)}</p>`;
    return;
  }

  el.innerHTML = `
    <div class="panel-header">
      <div>
        <div class="panel-title">Presets</div>
        <div class="panel-note">${presetsList.length} wipe configuration${presetsList.length === 1 ? '' : 's'}</div>
      </div>
      <button class="btn btn-primary btn-sm" id="btn-new-preset">New Preset</button>
    </div>
    ${presetsList.length === 0 ? emptyState('No presets yet.') : `
      <div class="preset-grid">
        ${presetsList.map(p => `
          <article class="preset-tile">
            <div class="preset-top">
              <div>
                <div class="preset-name">${escapeHtml(p.name)}</div>
                <div class="panel-note text-mono">${escapeHtml(p.schemeId)}</div>
              </div>
              <span class="badge ${p.autoFormat ? 'badge-info' : 'badge-neutral'}">${p.autoFormat ? 'Format' : 'No format'}</span>
            </div>
            <dl class="preset-meta">
              <div><dt>Verify</dt><dd>${Number(p.verifySizeGB || 0)} GiB</dd></div>
              <div><dt>Label</dt><dd>${p.labelTemplate ? escapeHtml(p.labelTemplate) : '-'}</dd></div>
            </dl>
            <div class="btn-group">
              <button class="btn btn-sm preset-edit-btn" data-id="${escapeAttr(p.id)}">Edit</button>
              <button class="btn btn-sm btn-danger preset-delete-btn" data-id="${escapeAttr(p.id)}">Delete</button>
            </div>
          </article>`).join('')}
      </div>
    `}
  `;

  document.getElementById('btn-new-preset').onclick = () => openPresetModal();

  el.querySelectorAll('.preset-edit-btn').forEach(btn => {
    btn.onclick = () => {
      const p = presetsList.find(x => x.id === btn.dataset.id);
      if (p) openPresetModal(p);
    };
  });
  el.querySelectorAll('.preset-delete-btn').forEach(btn => {
    btn.onclick = () => {
      const p = presetsList.find(x => x.id === btn.dataset.id);
      showConfirm(`Delete preset "${p.name}"?`, {
        dangerLabel: 'Delete',
        onConfirm: async () => {
          try { await apiDelete('/api/presets/' + p.id); showToast('Preset deleted', 'success'); renderPresetsPanel(el); }
          catch (e) { showToast(e.message, 'error'); }
        }
      });
    };
  });
}

function buildPresetModal() {
  presetModalEl = document.createElement('div');
  presetModalEl.className = 'overlay';
  presetModalEl.innerHTML = `
    <div class="modal" tabindex="-1">
      <h2 id="preset-modal-title">New Preset</h2>
      <div class="form-group"><label>Name</label><input type="text" id="preset-name" class="w-full" placeholder="My Preset" data-autofocus></div>
      <div class="form-group"><label>Scheme</label><select id="preset-scheme" class="w-full">${schemeOptions(schemesList, 'zero')}</select></div>
      <div class="form-group"><label class="checkbox-label"><input type="checkbox" id="preset-autoformat"> Auto-format FAT32</label></div>
      <div class="form-group"><label>Verify Size</label><select id="preset-verify" class="w-full">${verifySizeOptions(1)}</select></div>
      <div class="form-group"><label>Label Template</label><input type="text" id="preset-label" class="w-full" placeholder="e.g. RMA-{date}-{serial}"></div>
      <div class="modal-actions">
        <button class="btn" id="btn-preset-cancel">Cancel</button>
        <button class="btn btn-primary" id="btn-preset-save">Save</button>
      </div>
    </div>
  `;
  document.body.appendChild(presetModalEl);

  presetModalHandle = attachOverlay(presetModalEl, presetModalEl.querySelector('.modal'), {
    onClose: () => {
      setTimeout(() => {
        if (presetModalEl) { presetModalEl.remove(); presetModalEl = null; presetModalHandle = null; }
      }, 200);
    }
  });

  presetModalEl.querySelector('#btn-preset-cancel').onclick = closePresetModal;
  presetModalEl.querySelector('#btn-preset-save').onclick = savePreset;
}

function openPresetModal(preset = null) {
  editingPresetId = preset ? preset.id : null;
  if (!presetModalEl) buildPresetModal();
  document.getElementById('preset-modal-title').textContent = preset ? 'Edit Preset' : 'New Preset';
  document.getElementById('preset-name').value = preset ? preset.name : '';
  document.getElementById('preset-scheme').value = preset ? preset.schemeId : 'zero';
  document.getElementById('preset-autoformat').checked = preset ? preset.autoFormat : false;
  document.getElementById('preset-verify').value = preset ? preset.verifySizeGB : 1;
  document.getElementById('preset-label').value = preset ? preset.labelTemplate || '' : '';
  presetModalHandle.open();
}

function closePresetModal() {
  if (presetModalHandle) presetModalHandle.close();
}

async function savePreset() {
  const name = document.getElementById('preset-name').value.trim();
  if (!name) { showToast('Name is required', 'error'); return; }
  const body = {
    name,
    schemeId: document.getElementById('preset-scheme').value,
    autoFormat: document.getElementById('preset-autoformat').checked,
    verifySizeGB: parseInt(document.getElementById('preset-verify').value),
    labelTemplate: document.getElementById('preset-label').value
  };
  try {
    if (editingPresetId) {
      await apiPut('/api/presets/' + editingPresetId, body);
      showToast('Preset updated', 'success');
    } else {
      await apiPost('/api/presets', body);
      showToast('Preset created', 'success');
    }
    closePresetModal();
    const el = document.getElementById('view-settings').querySelector('#settings-panel-presets');
    if (el) renderPresetsPanel(el);
  } catch (e) { showToast(e.message, 'error'); }
}
