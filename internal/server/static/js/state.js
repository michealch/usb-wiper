// Minimal shared state — the app fetches through api.js; this holds only the
// settings object the configurator reads.

import { apiGet } from './api.js';

const state = {
  settings: {}
};

function setState(updates) {
  Object.assign(state, updates);
}

function loadSettings() {
  return apiGet('/api/settings').then(data => {
    setState({ settings: data });
    return data;
  });
}

export { state, loadSettings };
