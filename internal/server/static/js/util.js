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

// badgeClassForStatus maps a job status to its badge CSS class. The dashboard
// mapping wins: `formatting` renders as info, everything else maps explicitly.
function badgeClassForStatus(status) {
  const m = {
    running: 'badge-warning', verifying: 'badge-info', formatting: 'badge-info',
    completed: 'badge-success', failed: 'badge-danger', cancelled: 'badge-neutral', queued: 'badge-neutral'
  };
  return m[status] || 'badge-neutral';
}

// deviceStatusBadge renders the 4-branch device status badge (active job,
// blocked, wiped/verify-failed from history, or ready).
function deviceStatusBadge(device, job) {
  if (job) return `<span class="badge ${badgeClassForStatus(job.status)}">${escapeHtml(job.status)}</span>`;
  if (device.wipeBlocked) return `<span class="badge badge-warning" title="${escapeAttr(device.blockReason || '')}">Blocked</span>`;
  if (device.wipeHistory && device.wipeHistory.status === 'completed') {
    const ok = device.wipeHistory.verification === 'passed';
    return `<span class="badge ${ok ? 'badge-success' : 'badge-danger'}">${ok ? 'Wiped' : 'Verify failed'}</span>`;
  }
  return '<span class="badge badge-success">Ready</span>';
}

// resolveProgress merges live SSE progress state over the job's own fields.
function resolveProgress(job, progressMap) {
  const ps = progressMap ? progressMap.get(job.devicePath) : undefined;
  return {
    percent: ps ? ps.percent : (job.progress || 0),
    pass: ps ? ps.currentPass : job.currentPass,
    totalPasses: ps ? ps.totalPasses : job.totalPasses
  };
}

// segmentWidths returns the width percentage for each pass segment given overall
// percent. Pass 0 or 1 totalPasses yields a single segment. This is the only place
// segment geometry is computed — renderPassSegments and app.js patchProgressDOM both
// consume it.
function segmentWidths(currentPass, totalPasses, overallPercent) {
  const total = Math.max(1, totalPasses || 1);
  const cur = Math.min(Math.max(1, currentPass || 1), total);
  const perPass = 100 / total;
  const out = [];
  for (let p = 1; p <= total; p++) {
    if (p < cur) out.push({ state: 'completed', width: perPass });
    else if (p === cur) {
      const within = overallPercent - (cur - 1) * perPass;
      out.push({ state: 'active', width: Math.max(1, Math.min(within, perPass)) });
    } else out.push({ state: 'pending', width: perPass });
  }
  return out;
}

// renderPassSegments renders the multi-pass segment bar.
function renderPassSegments(currentPass, totalPasses, overallPercent) {
  return segmentWidths(currentPass, totalPasses, overallPercent)
    .map(s => `<div class="progress-segment ${s.state}" style="width:${s.width}%"></div>`)
    .join('');
}

// progressBar renders the segmented pass bar plus percent/pass readout for a job.
// Used by every job card and by the device table's progress cell. The container carries
// the progressbar role because no <progress> element remains to supply it.
function progressBar(job, progressMap) {
  const { percent, pass, totalPasses } = resolveProgress(job, progressMap);
  const total = Math.max(1, totalPasses || 1);
  return `
    <div class="progress-multi" role="progressbar" aria-valuemin="0" aria-valuemax="100"
         aria-valuenow="${Math.round(percent)}">${renderPassSegments(pass, total, percent)}</div>
    <div class="progress-info">
      <span class="progress-pct">${percent.toFixed(1)}%</span>
      ${total > 1 ? `<span class="progress-pass">Pass ${pass}/${total}</span>` : ''}
    </div>`;
}

// formatDateTime renders a localized date/time, or '—' for null/undefined/NaN.
function formatDateTime(value) {
  if (!value) return '—';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

// emptyState renders the empty-state block markup.
function emptyState(title, subtitle) {
  return `<div class="empty-state"><div class="empty-state-title">${escapeHtml(title)}</div>${subtitle ? `<div class="empty-state-subtitle">${escapeHtml(subtitle)}</div>` : ''}</div>`;
}

// activeJobByPath builds a Map(devicePath -> job) for jobs currently active or queued.
function activeJobByPath(jobs) {
  const m = new Map();
  jobs.forEach(j => {
    if (j.status === 'running' || j.status === 'verifying' || j.status === 'formatting' || j.status === 'queued') {
      m.set(j.devicePath, j);
    }
  });
  return m;
}

// schemeOptions renders <option> elements from GET /api/schemes.
function schemeOptions(schemes, selectedId) {
  return schemes.map(s => `<option value="${escapeAttr(s.id)}" ${s.id === selectedId ? 'selected' : ''}>${escapeHtml(s.displayName)} (${s.passes} pass${s.passes > 1 ? 'es' : ''})</option>`).join('');
}

// verifySizeOptions renders the 0/1/2/4/16 GiB verify-size <option> list.
function verifySizeOptions(selectedGB) {
  const options = [
    { v: 0, label: 'Off' },
    { v: 1, label: '1 GiB' },
    { v: 2, label: '2 GiB' },
    { v: 4, label: '4 GiB' },
    { v: 16, label: '16 GiB' }
  ];
  return options.map(o => `<option value="${o.v}" ${o.v === selectedGB ? 'selected' : ''}>${o.label}</option>`).join('');
}

export { escapeHtml, escapeAttr, formatBytes, prefersReducedMotion,
  badgeClassForStatus, deviceStatusBadge, resolveProgress, segmentWidths,
  renderPassSegments, progressBar, formatDateTime, emptyState, activeJobByPath,
  schemeOptions, verifySizeOptions };
