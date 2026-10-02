// store.js — app-wide reactive state + loaders.
//
// A single plain object; set() merges and notifies subscribers once per
// microtask. Views read store.state directly and re-render on notify.

import { api } from './api.js';
import { debounce } from './util.js';

const subs = new Set();
let queued = false;

export const store = {
  state: {
    booted: false,
    health: null,            // /api/health
    settings: null,          // Settings (no secrets)
    secrets: { anthropic: { set: false }, openai: { set: false } },
    wa: { state: 'connecting', me: null, lastError: '', since: null },
    qr: null,                // { png, expiresInSec, receivedAt }
    personas: [],
    personasLoaded: false,
    chats: [],               // ChatAssignment & {personaName, pendingCount, historyCount}
    chatsLoaded: false,
    approvals: [],
    approvalsLoaded: false,
    activity: [],            // newest first
    activityLoaded: false,
    pulls: {},               // jobId -> PullProgress
    sse: 'connecting',       // connecting | open | down
    sseDownSince: 0,
    draftPersona: null,      // from the AI builder, consumed by the editor
    behaviorMeta: null,      // /api/behavior/presets → { presets, ranges, enums, defaults }
  },

  set(patch) {
    Object.assign(this.state, typeof patch === 'function' ? patch(this.state) : patch);
    this.notify();
  },

  notify() {
    if (queued) return;
    queued = true;
    queueMicrotask(() => {
      queued = false;
      for (const fn of subs) {
        try { fn(this.state); } catch (e) { console.error(e); }
      }
    });
  },

  subscribe(fn) {
    subs.add(fn);
    return () => subs.delete(fn);
  },
};

// ── Simple event bus for SSE fan-out to pages ─────────────────────────
const listeners = new Map();
export const bus = {
  on(type, fn) {
    if (!listeners.has(type)) listeners.set(type, new Set());
    listeners.get(type).add(fn);
    return () => listeners.get(type).delete(fn);
  },
  emit(type, data) {
    const s = listeners.get(type);
    if (s) for (const fn of s) { try { fn(data); } catch (e) { console.error(e); } }
  },
};

// ── Lookups ────────────────────────────────────────────────────────────
export const personaById = (id) => store.state.personas.find((p) => p.id === id) || null;
export const chatByKey = (key) => store.state.chats.find((c) => c.key === key) || null;
export function assignmentFor(item) {
  if (!item) return null;
  const cs = store.state.chats;
  return cs.find((c) => c.key === item.key)
    || cs.find((c) => item.jid && (c.jid === item.jid || c.altJid === item.jid))
    || cs.find((c) => item.altJid && (c.jid === item.altJid || c.altJid === item.altJid))
    || null;
}
export const usageCount = (personaId) => store.state.chats.filter((c) => c.personaId === personaId).length;

// ── Loaders ────────────────────────────────────────────────────────────
const quiet = { quiet: true };

export async function loadHealth() {
  const health = await api.health(quiet);
  store.set({ health });
  return health;
}

export async function loadSettings() {
  const s = await api.settings.get(quiet);
  const { secrets, ...settings } = s || {};
  store.set({ settings, secrets: secrets || store.state.secrets });
  return settings;
}

export async function loadPersonas() {
  const list = await api.personas.list(quiet);
  store.set({ personas: Array.isArray(list) ? list : [], personasLoaded: true });
}

export async function loadChats() {
  const list = await api.chats.list(quiet);
  store.set({ chats: Array.isArray(list) ? list : [], chatsLoaded: true });
}

export async function loadApprovals() {
  const list = await api.approvals.list(quiet);
  store.set({ approvals: Array.isArray(list) ? list : [], approvalsLoaded: true });
}

export async function loadActivity() {
  const r = await api.activity.list({ limit: 200 }, quiet);
  const items = (r && r.items) || [];
  store.set({ activity: items.slice().reverse(), activityLoaded: true });
}

let behaviorMetaReq = null;
/** Presets, ranges and enums for the Behaviour editor (fetched once, cached). */
export function loadBehaviorMeta(force = false) {
  if (store.state.behaviorMeta && !force) return Promise.resolve(store.state.behaviorMeta);
  if (!behaviorMetaReq || force) {
    behaviorMetaReq = api.behavior.presets(quiet)
      .then((meta) => { store.set({ behaviorMeta: meta || null }); return meta; })
      .catch((e) => { behaviorMetaReq = null; throw e; });
  }
  return behaviorMetaReq;
}

/** True when an assignment is snoozed ("Away") right now. */
export function isAway(chat, now = Date.now()) {
  if (!chat || !chat.snoozedUntil) return false;
  const t = Date.parse(chat.snoozedUntil);
  return !isNaN(t) && t > now;
}

export async function loadWA() {
  const wa = await api.wa.status(quiet);
  if (wa) store.set({ wa });
}

export const reloadChatsSoon = debounce(() => loadChats().catch(() => {}), 120);
export const reloadPersonasSoon = debounce(() => loadPersonas().catch(() => {}), 120);

export async function refreshAll() {
  await Promise.allSettled([loadSettings(), loadPersonas(), loadChats(), loadApprovals(), loadActivity(), loadWA()]);
}

// ── Reducers for live events ───────────────────────────────────────────
const ACTIVITY_CAP = 500;

export function pushActivity(ev) {
  if (!ev) return;
  const list = store.state.activity;
  if (ev.id != null && list.length && list.some((x) => x.id === ev.id)) return;
  const next = [ev, ...list];
  if (next.length > ACTIVITY_CAP) next.length = ACTIVITY_CAP;
  store.set({ activity: next });
}

export function applyApproval({ action, pending } = {}) {
  if (!pending || !pending.id) return;
  let list = store.state.approvals.filter((p) => p.id !== pending.id);
  if (action !== 'removed') {
    const idx = store.state.approvals.findIndex((p) => p.id === pending.id);
    if (idx >= 0) { list = store.state.approvals.slice(); list[idx] = pending; }
    else list = [...list, pending];
  }
  store.set({ approvals: list });
}

export function applyWAStatus(wa) {
  const patch = { wa };
  if (wa && wa.state === 'connected') patch.qr = null;
  store.set(patch);
}
