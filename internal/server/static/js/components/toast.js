// Toast notification system

import { escapeHtml } from '../util.js';
import { attachOverlay } from './overlay.js';

const container = document.createElement('div');
container.className = 'toast-container';
document.body.appendChild(container);

const TOAST_ICONS = {
  success: '✓',
  error: '✕',
  warning: '⚠',
  info: 'ⓘ'
};

function showToast(msg, type = 'info', duration = 5000) {
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  toast.innerHTML = `<span class="toast-icon" aria-hidden="true">${TOAST_ICONS[type] || TOAST_ICONS.info}</span><span class="toast-msg">${escapeHtml(msg)}</span><button class="toast-close" aria-label="Dismiss">&times;</button>`;
  
  toast.querySelector('.toast-close').onclick = () => {
    toast.style.animation = 'slideOut .2s ease forwards';
    setTimeout(() => toast.remove(), 200);
  };

  container.appendChild(toast);
  
  if (duration > 0) {
    setTimeout(() => {
      if (toast.parentNode) {
        toast.style.animation = 'slideOut .2s ease forwards';
        setTimeout(() => toast.remove(), 200);
      }
    }, duration);
  }
  
  return toast;
}

function showConfirm(msg, {confirmLabel = 'Confirm', dangerLabel = '', onConfirm, onCancel} = {}) {
  const overlay = document.createElement('div');
  overlay.className = 'overlay';

  overlay.innerHTML = `
    <div class="modal" tabindex="-1">
      <h2>Confirm</h2>
      <p>${escapeHtml(msg)}</p>
      <div class="modal-actions">
        <button class="btn" id="modal-cancel" data-autofocus>Cancel</button>
        ${dangerLabel ? `<button class="btn btn-danger" id="modal-confirm">${dangerLabel}</button>` : `<button class="btn btn-primary" id="modal-confirm">${confirmLabel}</button>`}
      </div>
    </div>
  `;

  document.body.appendChild(overlay);
  const handle = attachOverlay(overlay, overlay.querySelector('.modal'), {
    onClose: () => {
      setTimeout(() => overlay.remove(), 200);
    }
  });
  handle.open();

  overlay.querySelector('#modal-cancel').onclick = () => { handle.close(); onCancel && onCancel(); };
  overlay.querySelector('#modal-confirm').onclick = () => { handle.close(); onConfirm && onConfirm(); };
}

export { showToast, showConfirm };
