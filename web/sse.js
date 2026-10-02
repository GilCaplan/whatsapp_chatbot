// sse.js — the single EventSource('/api/events') for the whole app.
//
// The browser's EventSource auto-reconnects on transient drops and resends
// Last-Event-ID itself. When it gives up (readyState CLOSED, e.g. the server
// was down at connect time) we recreate it with exponential backoff and do a
// full state refresh, since a fresh EventSource can't send Last-Event-ID.

import {
  store, bus, pushActivity, applyApproval, applyWAStatus,
  reloadChatsSoon, reloadPersonasSoon, refreshAll,
} from './store.js';

const TYPES = ['wa.status', 'wa.qr', 'activity', 'approval', 'chats.changed', 'personas.changed', 'settings.changed', 'ollama.pull', 'system',
  // Wave 3: listeners subscribe through the bus (bus.on('memories.changed', …)).
  'memories.changed', 'recaps.changed', 'missions.changed'];

let es = null;
let stopped = false;
let backoff = 1000;
let retryTimer = null;
let everOpened = false;
let needResync = false;

function parse(e) {
  try { return e.data ? JSON.parse(e.data) : {}; } catch { return {}; }
}

function handle(type, data) {
  switch (type) {
    case 'wa.status': applyWAStatus(data); break;
    case 'wa.qr': store.set({ qr: { ...data, receivedAt: Date.now() } }); break;
    case 'activity': pushActivity(data); break;
    case 'approval': applyApproval(data); break;
    case 'chats.changed': reloadChatsSoon(); break;
    case 'personas.changed': reloadPersonasSoon(); break;
    case 'settings.changed':
      if (data && typeof data === 'object') {
        const { secrets, ...settings } = data;
        store.set({ settings: { ...(store.state.settings || {}), ...settings } });
      }
      break;
    case 'ollama.pull':
      if (data && data.jobId) store.set({ pulls: { ...store.state.pulls, [data.jobId]: data } });
      break;
    case 'system': break; // handled by app.js via the bus
  }
  bus.emit(type, data);
}

function open() {
  clearTimeout(retryTimer);
  if (stopped) return;
  try { if (es) es.close(); } catch { /* ignore */ }
  es = new EventSource('/api/events');

  es.onopen = () => {
    backoff = 1000;
    const wasDown = store.state.sse !== 'open';
    store.set({ sse: 'open', sseDownSince: 0 });
    if ((everOpened && wasDown) || needResync) {
      needResync = false;
      refreshAll();
    }
    everOpened = true;
  };

  es.onerror = () => {
    if (stopped) return;
    if (store.state.sse !== 'down') store.set({ sse: 'down', sseDownSince: Date.now() });
    if (es.readyState === EventSource.CLOSED) {
      needResync = true;
      retryTimer = setTimeout(open, backoff);
      backoff = Math.min(backoff * 2, 15000);
    }
  };

  for (const t of TYPES) es.addEventListener(t, (e) => handle(t, parse(e)));
}

export function startSSE() {
  stopped = false;
  open();
  // Safari can silently drop long-lived streams when a tab sleeps.
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden && !stopped && es && es.readyState === EventSource.CLOSED) open();
  });
}

export function stopSSE() {
  stopped = true;
  clearTimeout(retryTimer);
  try { if (es) es.close(); } catch { /* ignore */ }
  es = null;
}
