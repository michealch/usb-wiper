// Certificate of Erasure — actions on completed jobs + verify tool.

import { apiGet, apiPost } from '../api.js';
import { showToast } from './toast.js';
import { escapeHtml, escapeAttr, formatBytes } from '../util.js';

// Returns markup for a "Certificate ▾" menu attached to a completed job.
function renderCertMenu(jobId) {
  const id = escapeAttr(jobId);
  return `
    <div class="cert-menu" data-cert-job="${id}">
      <button class="btn btn-ghost btn-sm cert-menu-toggle" aria-haspopup="true" aria-expanded="false">Certificate ▾</button>
      <div class="cert-menu-list" role="menu">
        <a role="menuitem" href="/api/cert/${id}/pdf" download>Download PDF</a>
        <a role="menuitem" href="/api/cert/${id}/json" download>Download JSON</a>
      </div>
    </div>
  `;
}

// Wires up open/close behavior for every `.cert-menu` inside `container`.
function initCertMenus(container) {
  container.querySelectorAll('.cert-menu-toggle').forEach(btn => {
    btn.onclick = (e) => {
      e.stopPropagation();
      const menu = btn.closest('.cert-menu').querySelector('.cert-menu-list');
      const isOpen = menu.classList.contains('open');
      container.querySelectorAll('.cert-menu-list.open').forEach(m => m.classList.remove('open'));
      menu.classList.toggle('open', !isOpen);
      btn.setAttribute('aria-expanded', String(!isOpen));
    };
  });

  if (!container.dataset.certMenuOutsideClick) {
    container.dataset.certMenuOutsideClick = '1';
    document.addEventListener('click', () => {
      container.querySelectorAll('.cert-menu-list.open').forEach(m => m.classList.remove('open'));
      container.querySelectorAll('.cert-menu-toggle[aria-expanded="true"]').forEach(b => b.setAttribute('aria-expanded', 'false'));
    });
  }
}

// Renders the "Verify Certificate" tool into `containerEl`.
function renderVerifyTool(containerEl) {
  containerEl.innerHTML = `
    <div class="form-group">
      <label for="verify-cert-file">Certificate file (.json)</label>
      <input type="file" id="verify-cert-file" accept="application/json,.json" class="w-full">
    </div>
    <div id="verify-cert-result"></div>
  `;

  const input = containerEl.querySelector('#verify-cert-file');
  const resultEl = containerEl.querySelector('#verify-cert-result');

  input.onchange = async () => {
    const file = input.files[0];
    if (!file) return;
    resultEl.innerHTML = '<p class="muted">Verifying…</p>';
    try {
      const text = await file.text();
      const cert = JSON.parse(text);
      const result = await apiPost('/api/cert/verify', cert);
      renderVerifyResult(resultEl, cert, result);
    } catch (e) {
      resultEl.innerHTML = `<div class="verify-result invalid"><div class="verify-result-title">INVALID</div><p>${escapeHtml(e.message)}</p></div>`;
    }
  };
}

function renderVerifyResult(el, cert, result) {
  const valid = !!result.valid;
  const device = cert.device || {};
  const wipe = cert.wipe || {};
  el.innerHTML = `
    <div class="verify-result ${valid ? 'valid' : 'invalid'}">
      <div class="verify-result-title">${valid ? 'VALID' : 'INVALID'}</div>
      <p>${valid ? 'This certificate signature is valid.' : escapeHtml(result.error || 'Signature verification failed.')}</p>
      <dl class="verify-result-details">
        <dt>Device</dt><dd>${escapeHtml(device.model || '—')} (${escapeHtml(device.serial || '—')})</dd>
        <dt>Size</dt><dd>${device.size ? formatBytes(device.size) : '—'}</dd>
        <dt>Scheme</dt><dd>${escapeHtml(wipe.schemeName || wipe.schemeId || '—')}</dd>
        <dt>Verification</dt><dd>${escapeHtml(cert.verification ? JSON.stringify(cert.verification) : '—')}</dd>
        <dt>Tool / Host</dt><dd>${escapeHtml(cert.tool || '—')} on ${escapeHtml(cert.host || '—')}</dd>
      </dl>
    </div>
  `;
}

// Renders the signing public key with a copy button into `containerEl`.
async function renderPubkeyCard(containerEl) {
  containerEl.innerHTML = '<p class="muted">Loading signing key…</p>';
  try {
    const data = await apiGet('/api/cert/pubkey');
    containerEl.innerHTML = `
      <div class="form-group">
        <label>Certificate signing public key</label>
        <input type="text" id="pubkey-value" class="w-full text-mono" value="${escapeAttr(data.publicKey || '')}" readonly>
      </div>
      <button class="btn btn-sm" id="pubkey-copy">Copy</button>
    `;
    containerEl.querySelector('#pubkey-copy').onclick = async () => {
      try {
        await navigator.clipboard.writeText(data.publicKey || '');
        showToast('Public key copied', 'success', 2000);
      } catch (_) {
        showToast('Copy failed — select and copy manually', 'error');
      }
    };
  } catch (e) {
    containerEl.innerHTML = `<p class="text-danger">Failed to load signing key: ${escapeHtml(e.message)}</p>`;
  }
}

export { renderCertMenu, initCertMenus, renderVerifyTool, renderPubkeyCard };
