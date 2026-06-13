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
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Presets</div>
          <div class="page-subtitle">${presetsList.length} wipe configuration${presetsList.length === 1 ? '' : 's'}</div>
        </div>
        <div class="page-actions">
          <button class="btn btn-primary btn-sm" id="btn-new-preset">New Preset</button>
        </div>
      </div>

      <section class="panel">
        ${presetsList.length === 0 ? `<div class="empty-state"><div class="empty-state-title">No presets yet.</div></div>` : `
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
      </section>
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
  document.getElementById('preset-modal').onclick = (e) => { if (e.target === e.currentTarget) closePresetModal(); };
  document.addEventListener('keydown', closePresetOnEsc);
}

function closePresetOnEsc(e) {
  if (e.key === 'Escape') {
    closePresetModal();
    document.removeEventListener('keydown', closePresetOnEsc);
  }
}

function closePresetModal() {
  document.getElementById('preset-modal').classList.remove('open');
  document.removeEventListener('keydown', closePresetOnEsc);
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
import { escapeHtml, escapeAttr } from '../util.js';

export { loadAndRenderPresets, renderPresets };
