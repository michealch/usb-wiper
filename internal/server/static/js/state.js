// Simple reactive pub/sub state store — no framework needed.

const state = {
  devices: [],
  jobs: [],
  history: [],
  presets: [],
  schemes: [],
  settings: {},
  queueCount: 0,
  runningCount: 0,
  connected: false
};

const listeners = new Set();

function subscribe(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

function setState(updates) {
  Object.assign(state, updates);
  listeners.forEach(fn => fn(state));
}

function loadDevices() {
  return apiGet('/api/devices').then(data => {
    setState({ devices: data.devices || [] });
    return data.devices || [];
  });
}

function loadJobs() {
  return apiGet('/api/jobs').then(data => {
    const jobs = data.jobs || [];
    const running = jobs.filter(j => j.status === 'running' || j.status === 'verifying' || j.status === 'formatting');
    const queued = jobs.filter(j => j.status === 'queued');
    setState({ jobs, queueCount: queued.length, runningCount: running.length });
    return jobs;
  });
}

function loadHistory(devicePath) {
  const q = devicePath ? '?device=' + encodeURIComponent(devicePath) : '';
  return apiGet('/api/history' + q).then(data => {
    setState({ history: data.history || [] });
    return data.history || [];
  });
}

function loadPresets() {
  return apiGet('/api/presets').then(data => {
    setState({ presets: data.presets || [] });
    return data.presets || [];
  });
}

function loadSchemes() {
  return apiGet('/api/schemes').then(data => {
    setState({ schemes: data.schemes || [] });
    return data.schemes || [];
  });
}

function loadSettings() {
  return apiGet('/api/settings').then(data => {
    setState({ settings: data });
    return data;
  });
}

function refreshAll() {
  return Promise.all([
    loadDevices(),
    loadJobs(),
    loadSettings()
  ]);
}

import { apiGet } from './api.js';

export { state, subscribe, setState, loadDevices, loadJobs, loadHistory, loadPresets, loadSchemes, loadSettings, refreshAll };
