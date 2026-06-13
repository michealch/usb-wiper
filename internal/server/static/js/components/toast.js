// Toast notification system

import { escapeHtml } from '../util.js';

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
  overlay.className = 'modal-overlay';
  
  overlay.innerHTML = `
    <div class="modal">
      <h2>Confirm</h2>
      <p>${escapeHtml(msg)}</p>
      <div class="modal-actions">
        <button class="btn" id="modal-cancel">Cancel</button>
        ${dangerLabel ? `<button class="btn btn-danger" id="modal-confirm">${dangerLabel}</button>` : `<button class="btn btn-primary" id="modal-confirm">${confirmLabel}</button>`}
      </div>
    </div>
  `;
  
  document.body.appendChild(overlay);
  requestAnimationFrame(() => overlay.classList.add('open'));
  
  const close = () => {
    overlay.classList.remove('open');
    document.removeEventListener('keydown', escHandler);
    setTimeout(() => overlay.remove(), 200);
  };

  overlay.querySelector('#modal-cancel').onclick = () => { close(); onCancel && onCancel(); };
  overlay.querySelector('#modal-confirm').onclick = () => { close(); onConfirm && onConfirm(); };
  overlay.onclick = (e) => { if (e.target === overlay) { close(); onCancel && onCancel(); } };

  const escHandler = (e) => {
    if (e.key === 'Escape') { close(); onCancel && onCancel(); }
  };
  document.addEventListener('keydown', escHandler);
}

export { showToast, showConfirm };
