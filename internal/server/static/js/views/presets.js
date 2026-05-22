// Presets view

let presetsList = [];

async function loadAndRenderPresets() {
  try {
    const data = await apiGet('/api/presets');
    presetsList = data.presets || [];
    renderPresets();
  } catch (e) {
    document.getElementById('view-presets').innerHTML = '<div class="card"><p class="text-danger">Failed to load presets</p></div>';
  }
}

function renderPresets() {
  const el = document.getElementById('view-presets');
  el.innerHTML = `
    <div class="card">
      <div class="card-header">
        <h2>Wipe Presets (${presetsList.length})</h2>
        <button class="btn btn-primary btn-sm" id="btn-new-preset">+ New Preset</button>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(260px,1fr));gap:var(--space-4)">
        ${presetsList.map(p => `
          <div class="card" style="margin:0">
            <h3 style="margin-bottom:8px">${p.name}</h3>
            <div style="font-size:.82rem;color:var(--color-text-dim)">
              <div>Scheme: <strong>${p.schemeId}</strong></div>
              <div>Auto-format: ${p.autoFormat ? 'Yes' : 'No'}</div>
              <div>Verify: ${p.verifySizeGB} GiB</div>
              ${p.labelTemplate ? `<div>Label: ${p.labelTemplate}</div>` : ''}
            </div>
            <div class="btn-group mt-3">
              <button class="btn btn-sm preset-edit-btn" data-id="${p.id}">Edit</button>
              <button class="btn btn-sm btn-danger preset-delete-btn" data-id="${p.id}">Delete</button>
            </div>
          </div>`).join('')}
      </div>
    </div>
    <div class="modal-overlay" id="preset-modal">
      <div class="modal">
        <h2 id="preset-modal-title">New Preset</h2>
        <div class="form-group"><label>Name</label><input type="text" id="preset-name" class="w-full" placeholder="My Preset"></div>
        <div class="form-group"><label>Scheme</label><select id="preset-scheme" class="w-full"><option value="zero">Zero Fill</option><option value="random">Random Fill</option><option value="dod-3pass">DoD 3-Pass</option><option value="nist-clear">NIST Clear</option></select></div>
        <div class="form-group"><label class="checkbox-label"><input type="checkbox" id="preset-autoformat"> Auto-format FAT32</label></div>
        <div class="form-group"><label>Verify Size</label><select id="preset-verify" class="w-full"><option value="0">Off</option><option value="1" selected>1 GiB</option><option value="2">2 GiB</option><option value="4">4 GiB</option><option value="16">16 GiB</option></select></div>
        <div class="form-group"><label>Label Template</label><input type="text" id="preset-label" class="w-full" placeholder="e.g. RMA-{date}-{serial}"></div>
        <div class="modal-actions">
          <button class="btn" id="btn-preset-cancel">Cancel</button>
          <button class="btn btn-primary" id="btn-preset-save">Save</button>
        </div>
      </div>
    </div>
  `;

  document.getElementById('btn-new-preset').onclick = () => openPresetModal();
  document.getElementById('btn-preset-cancel').onclick = closePresetModal;
  document.getElementById('btn-preset-save').onclick = savePreset;

  el.querySelectorAll('.preset-edit-btn').forEach(btn => {
    btn.onclick = () => {
      const p = presetsList.find(x => x.id === btn.dataset.id);
      if (p) openPresetModal(p);
    };
  });
  el.querySelectorAll('.preset-delete-btn').forEach(btn => {
    btn.onclick = async () => {
      const p = presetsList.find(x => x.id === btn.dataset.id);
      showConfirm(`Delete preset "${p.name}"?`, {
        dangerLabel: 'Delete',
        onConfirm: async () => {
          try { await apiDelete('/api/presets/' + p.id); showToast('Preset deleted', 'success'); loadAndRenderPresets(); }
          catch (e) { showToast(e.message, 'error'); }
        }
      });
    };
  });
}

let editingPresetId = null;
function openPresetModal(preset = null) {
  editingPresetId = preset ? preset.id : null;
  document.getElementById('preset-modal-title').textContent = preset ? 'Edit Preset' : 'New Preset';
  document.getElementById('preset-name').value = preset ? preset.name : '';
  document.getElementById('preset-scheme').value = preset ? preset.schemeId : 'zero';
  document.getElementById('preset-autoformat').checked = preset ? preset.autoFormat : false;
  document.getElementById('preset-verify').value = preset ? preset.verifySizeGB : 1;
  document.getElementById('preset-label').value = preset ? preset.labelTemplate || '' : '';
  document.getElementById('preset-modal').classList.add('open');
}

function closePresetModal() {
  document.getElementById('preset-modal').classList.remove('open');
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
    loadAndRenderPresets();
  } catch (e) { showToast(e.message, 'error'); }
}

import { apiGet, apiPost, apiPut, apiDelete } from '../api.js';
import { showToast, showConfirm } from '../components/toast.js';

export { loadAndRenderPresets, renderPresets };
