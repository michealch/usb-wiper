// Hold-to-confirm — reusable irreversible-action gate.
//
// holdConfirm(buttonEl, { durationMs, onConfirm, liveRegion })
//
// Pointer/touch/keyboard hold fills a progress overlay on the button; releasing
// before completion cancels. Under prefers-reduced-motion (or for users who
// can't sustain a hold) it falls back to an explicit two-step "Arm" -> "Confirm".

import { prefersReducedMotion } from '../util.js';

function holdConfirm(buttonEl, { durationMs = 2500, onConfirm, liveRegion } = {}) {
  if (!buttonEl) return;

  buttonEl.classList.add('hold-confirm');
  const fill = document.createElement('span');
  fill.className = 'hold-fill';
  fill.setAttribute('aria-hidden', 'true');
  buttonEl.insertBefore(fill, buttonEl.firstChild);

  const originalLabel = buttonEl.textContent.trim();
  let fired = false;

  function announce(msg) {
    if (liveRegion) liveRegion.textContent = msg;
  }

  if (prefersReducedMotion()) {
    // Two-step explicit confirm.
    let armed = false;
    let armTimer = null;

    buttonEl.addEventListener('click', () => {
      if (fired) return;
      if (!armed) {
        armed = true;
        buttonEl.classList.add('armed');
        buttonEl.textContent = 'Confirm ' + originalLabel.toLowerCase();
        announce('Armed. Click again to confirm ' + originalLabel.toLowerCase() + '.');
        clearTimeout(armTimer);
        armTimer = setTimeout(() => {
          armed = false;
          buttonEl.classList.remove('armed');
          buttonEl.textContent = originalLabel;
          announce('');
        }, 5000);
      } else {
        clearTimeout(armTimer);
        fired = true;
        announce('Confirmed.');
        onConfirm && onConfirm();
      }
    });
    return;
  }

  // Hold-to-fill interaction.
  let startTime = null;
  let rafId = null;

  function tick() {
    const elapsed = performance.now() - startTime;
    const pct = Math.min(100, (elapsed / durationMs) * 100);
    fill.style.width = pct + '%';
    if (pct >= 100) {
      if (!fired) {
        fired = true;
        buttonEl.classList.remove('holding');
        announce('Confirmed.');
        onConfirm && onConfirm();
      }
      return;
    }
    rafId = requestAnimationFrame(tick);
  }

  function start() {
    if (fired) return;
    startTime = performance.now();
    buttonEl.classList.add('holding');
    announce('Hold to confirm ' + originalLabel.toLowerCase() + '…');
    rafId = requestAnimationFrame(tick);
  }

  function cancel() {
    if (fired) return;
    if (rafId) cancelAnimationFrame(rafId);
    rafId = null;
    startTime = null;
    buttonEl.classList.remove('holding');
    fill.style.width = '0%';
    announce('');
  }

  buttonEl.addEventListener('pointerdown', start);
  buttonEl.addEventListener('pointerup', cancel);
  buttonEl.addEventListener('pointerleave', cancel);
  buttonEl.addEventListener('pointercancel', cancel);

  buttonEl.addEventListener('keydown', (e) => {
    if ((e.key === 'Enter' || e.key === ' ') && !e.repeat) {
      e.preventDefault();
      start();
    }
  });
  buttonEl.addEventListener('keyup', (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      cancel();
    }
  });
  buttonEl.addEventListener('blur', cancel);

  // Prevent the click from firing as a normal activation (no native click on hold).
  buttonEl.addEventListener('click', (e) => e.preventDefault());
}

export { holdConfirm };
