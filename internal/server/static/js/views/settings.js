// Settings view

let currentSettings = {};

async function loadAndRenderSettings() {
  try {
    currentSettings = await apiGet('/api/settings');
    renderSettings();
  } catch (e) {
    document.getElementById('view-settings').innerHTML = '<div class="card"><p class="text-danger">Failed to load settings</p></div>';
  }
}

function renderSettings() {
  const el = document.getElementById('view-settings');
  const s = currentSettings;

  const safetyChecks = [
    { num: 1, name: 'Path Pattern', desc: 'Must match /dev/sd[a-z] - rejects partitions, NVMe, loop, DM' },
    { num: 2, name: 'Device Exists', desc: 'os.Stat() must succeed' },
    { num: 3, name: 'Block Device', desc: 'ModeDevice bit must be set' },
    { num: 4, name: 'Not NVMe', desc: 'NVMe devices are blocked (system disk risk)' },
    { num: 5, name: 'Not Root Device', desc: 'Checks /proc/mounts for root filesystem' },
    { num: 6, name: 'Removable Flag', desc: 'Removable == 1 in sysfs (skippable via env var)' },
    { num: 7, name: 'USB Bus', desc: 'Device symlink must resolve through /usb' },
    { num: 8, name: 'System Mount Points', desc: 'Not mounted at /, /boot, /home, /var, /usr, /etc' },
    { num: 9, name: 'Size Limit', desc: 'Max 2 TB - prevents wiping large USB-attached storage' }
  ];

  el.innerHTML = `
    <div class="view-shell">
      <div class="page-head">
        <div>
          <div class="page-title">Settings</div>
          <div class="page-subtitle">Runtime defaults and safety posture</div>
        </div>
        <div class="page-actions">
          <button class="btn btn-primary" id="btn-save-settings">Save Settings</button>
        </div>
      </div>

      <div class="settings-grid">
        <div class="settings-stack">
          <section class="settings-panel">
            <div class="panel-header"><div class="panel-title">General</div></div>
            <div class="field-grid">
              <div class="form-group">
                <label for="set-theme">Theme</label>
                <select id="set-theme" class="w-full">
                  <option value="dark" ${s.theme === 'dark' ? 'selected' : ''}>Dark</option>
                  <option value="light" ${s.theme === 'light' ? 'selected' : ''}>Light</option>
                </select>
              </div>
              <div class="form-group">
                <label for="set-scheme">Default Scheme</label>
                <select id="set-scheme" class="w-full">
                  <option value="zero" ${s.defaultSchemeId === 'zero' ? 'selected' : ''}>Zero Fill</option>
                  <option value="random" ${s.defaultSchemeId === 'random' ? 'selected' : ''}>Random Fill</option>
                  <option value="dod-3pass" ${s.defaultSchemeId === 'dod-3pass' ? 'selected' : ''}>DoD 3-Pass</option>
                  <option value="nist-clear" ${s.defaultSchemeId === 'nist-clear' ? 'selected' : ''}>NIST Clear</option>
                </select>
              </div>
              <div class="form-group">
                <label for="set-parallel">Max Parallel Jobs</label>
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
                  <option value="0" ${s.verifySizeGB === 0 ? 'selected' : ''}>Off</option>
                  <option value="1" ${s.verifySizeGB === 1 ? 'selected' : ''}>1 GiB</option>
                  <option value="2" ${s.verifySizeGB === 2 ? 'selected' : ''}>2 GiB</option>
                  <option value="4" ${s.verifySizeGB === 4 ? 'selected' : ''}>4 GiB</option>
                  <option value="16" ${s.verifySizeGB === 16 ? 'selected' : ''}>16 GiB</option>
                </select>
              </div>
              <div class="form-group">
                <label for="set-retention">History Retention</label>
                <input type="number" id="set-retention" value="${s.historyRetentionDays || 0}" min="0" class="w-full">
              </div>
            </div>
          </section>

          <section class="settings-panel">
            <div class="panel-header"><div class="panel-title">Safety Checks</div></div>
            <div class="safety-check-grid">
              ${safetyChecks.map(c => `
                <div class="safety-check">
                  <div><span class="num">#${c.num}</span> ${c.name}</div>
                  <div style="color:var(--color-text-dim);margin-top:4px;font-size:var(--text-xs)">${c.desc}</div>
                </div>`).join('')}
            </div>
          </section>
        </div>

        <div class="settings-stack">
          <section class="settings-panel">
            <div class="panel-header"><div class="panel-title">Notifications</div></div>
            <div class="setting-row">
              <div><div class="setting-label">Enable Notifications</div><div class="setting-desc">Webhook on job completion</div></div>
              <label class="checkbox-label"><input type="checkbox" id="set-notify" ${s.notificationsOn ? 'checked' : ''}></label>
            </div>
            <div class="form-group mt-3">
              <label>Webhook URL</label>
              <input type="url" id="set-webhook" class="w-full" value="${escapeAttr(s.webhookUrl || '')}" placeholder="https://hooks.example.com/wipe-complete">
            </div>
          </section>

          <section class="settings-panel">
            <div class="panel-header"><div class="panel-title">Security</div></div>
            <div id="settings-pubkey-card"></div>
            <h3 class="mt-4 mb-2">Verify certificate</h3>
            <div id="settings-verify-tool"></div>
          </section>
        </div>
      </div>
    </div>
  `;

  document.getElementById('btn-save-settings').onclick = saveSettings;

  // Theme change applies immediately
  document.getElementById('set-theme').onchange = function() {
    document.documentElement.setAttribute('data-theme', this.value);
    localStorage.setItem('theme', this.value);
  };

  renderPubkeyCard(document.getElementById('settings-pubkey-card'));
  renderVerifyTool(document.getElementById('settings-verify-tool'));
}

async function saveSettings() {
  const updates = {
    theme: document.getElementById('set-theme').value,
    defaultSchemeId: document.getElementById('set-scheme').value,
    maxParallelJobs: parseInt(document.getElementById('set-parallel').value),
    verifySizeGB: parseInt(document.getElementById('set-verify').value),
    historyRetentionDays: parseInt(document.getElementById('set-retention').value),
    notificationsOn: document.getElementById('set-notify').checked,
    webhookUrl: document.getElementById('set-webhook').value
  };
  try {
    await apiPut('/api/settings', updates);
    showToast('Settings saved', 'success');
    currentSettings = { ...currentSettings, ...updates };
  } catch (e) { showToast(e.message, 'error'); }
}

import { apiGet, apiPut } from '../api.js';
import { showToast } from '../components/toast.js';
import { renderPubkeyCard, renderVerifyTool } from '../components/cert.js';
import { escapeAttr } from '../util.js';

export { loadAndRenderSettings, renderSettings };
