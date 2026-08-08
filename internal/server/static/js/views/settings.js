// Settings screen — General, Auto Wipe, Presets, and Security tabs.

import { apiGet, apiPut } from '../api.js';
import { showToast } from '../components/toast.js';
import { initTabs } from '../components/tabs.js';
import { renderPubkeyCard, renderVerifyTool } from '../components/cert.js';
import { escapeHtml, escapeAttr, schemeOptions, verifySizeOptions } from '../util.js';
import { renderAutoWipePanel } from './autowipe.js';
import { renderPresetsPanel } from './presets.js';

let currentSettings = {};
let schemesList = [];

// renderSettings(tab) builds the Settings shell and delegates each tab to its panel
// renderer. tab is 'general' | 'autowipe' | 'presets' | 'security'.
export function renderSettings(tab) {
  const el = document.getElementById('view-settings');
  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Settings</div>
          <div class="page-subtitle">Runtime defaults, automation, and signing keys</div>
        </div>
      </div>
      <section class="panel" id="settings-tabhost"></section>
    </div>
  `;

  initTabs(document.getElementById('settings-tabhost'), {
    idPrefix: 'settings',
    tabs: [
      { id: 'general', label: 'General' },
      { id: 'autowipe', label: 'Auto Wipe' },
      { id: 'presets', label: 'Presets' },
      { id: 'security', label: 'Security' }
    ],
    activeId: tab,
    onSelect: (id) => {
      // Linkable tab — replaceState so we don't re-enter navigateTo via hashchange.
      history.replaceState(null, '', '#/settings/' + id);
      const panelEl = document.getElementById('settings-panel-' + id);
      if (id === 'general') renderGeneralPanel(panelEl);
      else if (id === 'autowipe') renderAutoWipePanel(panelEl);
      else if (id === 'presets') renderPresetsPanel(panelEl);
      else if (id === 'security') renderSecurityPanel(panelEl);
    }
  });
}

async function renderGeneralPanel(el) {
  el.innerHTML = '<p class="muted">Loading settings...</p>';
  try {
    const [settings, schemesData] = await Promise.all([
      apiGet('/api/settings'),
      apiGet('/api/schemes').catch(() => ({ schemes: [] }))
    ]);
    currentSettings = settings;
    schemesList = schemesData.schemes || [];
  } catch (e) {
    el.innerHTML = `<p class="text-danger">Failed to load settings: ${escapeHtml(e.message)}</p>`;
    return;
  }

  const s = currentSettings;

  // If the scheme list failed to load, fall back to the current value only.
  const schemeOptionsHtml = schemesList.length > 0
    ? schemeOptions(schemesList, s.defaultSchemeId)
    : `<option value="${escapeAttr(s.defaultSchemeId || 'zero')}" selected>${escapeHtml(s.defaultSchemeId || 'zero')}</option>`;

  el.innerHTML = `
    <div class="panel-header"><div class="panel-title">General</div></div>
    <div class="field-grid">
      <div class="form-group">
        <label for="set-scheme">Default Scheme</label>
        <select id="set-scheme" class="w-full" ${schemesList.length === 0 ? 'disabled' : ''}>
          ${schemeOptionsHtml}
        </select>
      </div>
      <div class="form-group">
        <label for="set-parallel">Max Parallel Jobs (applies on restart)</label>
        <select id="set-parallel" class="w-full">
          <option value="1" ${s.maxParallelJobs === 1 ? 'selected' : ''}>1</option>
          <option value="2" ${s.maxParallelJobs === 2 ? 'selected' : ''}>2</option>
          <option value="3" ${s.maxParallelJobs === 3 ? 'selected' : ''}>3</option>
          <option value="4" ${s.maxParallelJobs === 4 ? 'selected' : ''}>4</option>
        </select>
      </div>
      <div class="form-group">
        <label for="set-verify">Default Verify Size</label>
        <select id="set-verify" class="w-full">
          ${verifySizeOptions(s.verifySizeGB)}
        </select>
      </div>
    </div>
    <div class="btn-group mt-3">
      <button class="btn btn-primary" id="btn-save-settings">Save Settings</button>
    </div>
  `;

  document.getElementById('btn-save-settings').onclick = saveSettings;
}

function renderSecurityPanel(el) {
  el.innerHTML = '<div id="settings-pubkey"></div><h3 class="mt-4 mb-2">Verify certificate</h3><div id="settings-verify"></div>';
  renderPubkeyCard(document.getElementById('settings-pubkey'));
  renderVerifyTool(document.getElementById('settings-verify'));
}

async function saveSettings() {
  const updates = {
    defaultSchemeId: document.getElementById('set-scheme').value,
    maxParallelJobs: parseInt(document.getElementById('set-parallel').value),
    verifySizeGB: parseInt(document.getElementById('set-verify').value)
  };
  try {
    await apiPut('/api/settings', updates);
    showToast('Settings saved', 'success');
    currentSettings = { ...currentSettings, ...updates };
  } catch (e) { showToast(e.message, 'error'); }
}
