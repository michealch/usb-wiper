// Shared helpers — escaping, formatting

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s == null ? '' : String(s);
  return d.innerHTML;
}

function escapeAttr(s) {
  return String(s == null ? '' : s).replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function formatBytes(bytes) {
  if (!bytes || bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return size.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
}

function prefersReducedMotion() {
  return window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

// Animate an element's text from its current numeric value up to `to`.
// Snaps instantly under prefers-reduced-motion.
function countUp(el, to, { duration = 600 } = {}) {
  if (!el) return;
  to = Number(to) || 0;
  const from = Number(String(el.textContent).replace(/[^\d.-]/g, '')) || 0;
  if (prefersReducedMotion() || from === to || duration <= 0) {
    el.textContent = String(to);
    return;
  }
  const start = performance.now();
  function frame(now) {
    const t = Math.min(1, (now - start) / duration);
    // easeOutCubic
    const eased = 1 - Math.pow(1 - t, 3);
    el.textContent = String(Math.round(from + (to - from) * eased));
    if (t < 1) requestAnimationFrame(frame);
    else el.textContent = String(to);
  }
  requestAnimationFrame(frame);
}

// Add the `.stagger` class to a container so its direct children cascade in.
// No-op under reduced motion (CSS still renders them fully visible).
function applyStagger(containerEl) {
  if (!containerEl || prefersReducedMotion()) return;
  containerEl.classList.add('stagger');
}

export { escapeHtml, escapeAttr, formatBytes, countUp, applyStagger, prefersReducedMotion };
