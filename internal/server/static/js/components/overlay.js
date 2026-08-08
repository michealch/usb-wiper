// Overlay behaviour — shared by the drawer, configurator, confirm modal, and preset
// modal. Owns focus management (record/restore, autofocus, trap) and the Escape /
// backdrop close rules; callers keep owning their markup.

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

// attachOverlay(backdropEl, panelEl, { onClose }) -> { open, close, isOpen, destroy }
export function attachOverlay(backdropEl, panelEl, { onClose } = {}) {
  let restoreTarget = null;

  function open() {
    restoreTarget = document.activeElement;
    backdropEl.classList.add('open');
    panelEl.classList.add('open');
    const autofocus = panelEl.querySelector('[data-autofocus]');
    const first = autofocus || panelEl.querySelector(FOCUSABLE);
    (first || panelEl).focus();
  }

  function close() {
    if (!isOpen()) return;
    backdropEl.classList.remove('open');
    panelEl.classList.remove('open');
    if (restoreTarget && restoreTarget.isConnected && restoreTarget.focus) {
      restoreTarget.focus();
    }
    if (onClose) onClose();
  }

  function isOpen() {
    return backdropEl.classList.contains('open');
  }

  function onKeydown(e) {
    if (!isOpen()) return;
    if (e.key === 'Escape') {
      close();
      return;
    }
    if (e.key === 'Tab') {
      const focusables = panelEl.querySelectorAll(FOCUSABLE);
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === panelEl)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  }

  function onBackdropClick(e) {
    if (e.target === backdropEl) close();
  }

  backdropEl.addEventListener('click', onBackdropClick);
  document.addEventListener('keydown', onKeydown);

  return {
    open,
    close,
    isOpen,
    destroy() {
      backdropEl.removeEventListener('click', onBackdropClick);
      document.removeEventListener('keydown', onKeydown);
    }
  };
}
