// api.js — fetch wrapper for the local Doppel server.
//
// • Sends X-Doppel-Token on every mutating request (token comes from the
//   <meta name="doppel-token"> the server templates into index.html).
// • JSON bodies get Content-Type: application/json.
// • Non-2xx responses with {error, code} become ApiError and (unless quiet)
//   a toast. Chat keys are URL-encoded.

let toastFn = null;
export function setErrorToaster(fn) { toastFn = fn; }

const PLACEHOLDER = '__DOPPEL_TOKEN__';
let token = (document.querySelector('meta[name="doppel-token"]') || {}).content || '';
if (token === PLACEHOLDER) token = '';

export function setToken(t) { if (t && t !== PLACEHOLDER) token = t; }
export function hasToken() { return !!token; }

export class ApiError extends Error {
  constructor(message, { status = 0, code = 'error', data = null } = {}) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.data = data;
  }
}

const MUTATING = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);
let lastNetToast = 0;

/**
 * request(method, path, body?, opts?)
 * opts: { signal, quiet, form (FormData), timeout (ms) }
 */
export async function request(method, path, body, opts = {}) {
  const headers = {};
  let payload;
  if (opts.form) {
    payload = opts.form;
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
    payload = JSON.stringify(body);
  }
  if (MUTATING.has(method)) {
    if (!token) await ensureToken();
    headers['X-Doppel-Token'] = token;
  }

  let signal = opts.signal;
  let timer = null;
  if (opts.timeout) {
    const ctl = new AbortController();
    if (signal) signal.addEventListener('abort', () => ctl.abort(), { once: true });
    timer = setTimeout(() => ctl.abort(), opts.timeout);
    signal = ctl.signal;
  }

  let res;
  try {
    res = await fetch(path, { method, headers, body: payload, signal, credentials: 'same-origin', cache: 'no-store' });
  } catch (err) {
    clearTimeout(timer);
    if (err && err.name === 'AbortError') throw err;
    const e = new ApiError("Can't reach Doppel — is it still running?", { code: 'network' });
    if (!opts.quiet && toastFn && Date.now() - lastNetToast > 5000) {
      lastNetToast = Date.now();
      toastFn(e.message, { type: 'error' });
    }
    throw e;
  }
  clearTimeout(timer);

  if (res.status === 204) return null;
  const ct = res.headers.get('content-type') || '';
  let data = null;
  if (ct.includes('application/json')) {
    try { data = await res.json(); } catch { data = null; }
  } else {
    try { data = await res.text(); } catch { data = null; }
  }

  if (!res.ok) {
    const msg = (data && typeof data === 'object' && data.error) || `Request failed (${res.status})`;
    const code = (data && typeof data === 'object' && data.code) || `http_${res.status}`;
    const e = new ApiError(msg, { status: res.status, code, data });
    if (!opts.quiet && toastFn) toastFn(msg, { type: 'error' });
    throw e;
  }
  return data;
}

async function ensureToken() {
  try {
    const r = await fetch('/api/health', { cache: 'no-store' });
    const h = await r.json();
    if (h && h.token) setToken(h.token);
  } catch { /* the request will fail with a clear error */ }
}

const enc = encodeURIComponent;
const qs = (o) => {
  const p = new URLSearchParams();
  for (const k in o) if (o[k] !== undefined && o[k] !== null && o[k] !== '') p.set(k, o[k]);
  const s = p.toString();
  return s ? `?${s}` : '';
};

export const get = (p, o) => request('GET', p, undefined, o);
export const post = (p, b, o) => request('POST', p, b === undefined ? {} : b, o);
export const put = (p, b, o) => request('PUT', p, b, o);
export const patch = (p, b, o) => request('PATCH', p, b, o);
export const del = (p, o) => request('DELETE', p, undefined, o);

// ── Typed endpoints ────────────────────────────────────────────────────
export const api = {
  health: (o) => get('/api/health', o),

  system: {
    ports: (q = {}, o) => get('/api/system/ports' + qs({ from: 7000, to: 9999, limit: 20, ...q }), o),
    setPort: (port, o) => post('/api/system/port', { port }, o),
    quit: (o) => post('/api/system/quit', {}, o),
    openDataDir: (o) => post('/api/system/open-data-dir', {}, o),
    /** Wave 3: { ok, backend } — 501 not_implemented until notifications land. */
    /** dry: only report the backend ({ok, backend, events}) without showing anything. */
    notifyTest: (o, dry) => post('/api/system/notify-test' + (dry ? '?dry=1' : ''), {}, o),
  },

  settings: {
    get: (o) => get('/api/settings', o),
    put: (partial, o) => put('/api/settings', partial, o),
    secrets: (body, o) => put('/api/settings/secrets', body, o),
  },

  llm: {
    test: (provider, model, o) => post('/api/llm/test', model ? { provider, model } : { provider }, { timeout: 120000, ...o }),
    models: (provider, o) => get('/api/llm/models' + qs({ provider }), o),
  },

  ollama: {
    status: (o) => get('/api/ollama/status', o),
    pull: (name, o) => post('/api/ollama/pull', { name }, o),
  },

  wa: {
    status: (o) => get('/api/wa/status', o),
    pair: (o) => post('/api/wa/pair', {}, o),
    reconnect: (o) => post('/api/wa/reconnect', {}, o),
    disconnect: (o) => post('/api/wa/disconnect', {}, o),
    logout: (o) => post('/api/wa/logout', {}, o),
    chats: (q, o) => get('/api/wa/chats' + qs(q), o),
    refreshChats: (o) => post('/api/wa/chats/refresh', {}, o),
    avatarURL: (jid) => `/api/wa/avatar?jid=${enc(jid)}`,
    qrURL: () => `/api/wa/qr.png?t=${Date.now()}`,
  },

  chats: {
    list: (o) => get('/api/chats', o),
    create: (body, o) => post('/api/chats', body, o),
    patch: (key, body, o) => patch(`/api/chats/${enc(key)}`, body, o),
    remove: (key, o) => del(`/api/chats/${enc(key)}`, o),
    history: (key, limit = 100, o) => get(`/api/chats/${enc(key)}/history?limit=${limit}`, o),
    clearHistory: (key, o) => del(`/api/chats/${enc(key)}/history`, o),
    resetGoal: (key, o) => post(`/api/chats/${enc(key)}/goal/reset`, {}, o),
    initiate: (key, hint, o) => post(`/api/chats/${enc(key)}/initiate`, hint ? { hint } : {}, { timeout: 60000, ...o }),
    send: (key, text, o) => post(`/api/chats/${enc(key)}/send`, { text }, o),
    /** senderJid: groups only — which member the message comes from ('' = a random one). */
    simulate: (key, text, fromMe = false, o, senderJid = '') => post(`/api/chats/${enc(key)}/simulate`, senderJid ? { text, fromMe, senderJid } : { text, fromMe }, o),
    behavior: (key, o) => get(`/api/chats/${enc(key)}/behavior`, o),
    /** Who the persona answers: { mode, effectiveMode, memberCount, threshold, members:[…] }. */
    people: (key, o) => get(`/api/chats/${enc(key)}/people`, o),

    // ── Wave 3 (501 not_implemented until each feature lands) ──
    /** { enabled, crossEnabled, items:[Memory + shares, effectiveSensitive], pending, lastExtractedAt } */
    memories: (key, o) => get(`/api/chats/${enc(key)}/memories`, o),
    /** body: { text, person?, personJid?, pinned? } → Memory */
    addMemory: (key, body, o) => post(`/api/chats/${enc(key)}/memories`, body, o),
    /** body: { text?, pinned?, person?, scope?: 'local'|'shared'|null, sensitive? } → Memory */
    patchMemory: (key, id, body, o) => patch(`/api/chats/${enc(key)}/memories/${enc(id)}`, body, o),
    deleteMemory: (key, id, o) => del(`/api/chats/${enc(key)}/memories/${enc(id)}`, o),
    clearMemories: (key, o) => del(`/api/chats/${enc(key)}/memories`, o),
    /** → { added, updated } */
    extractMemories: (key, o) => post(`/api/chats/${enc(key)}/memories/extract`, {}, { timeout: 120000, ...o }),
    /** Cross-chat context → { enabled, kind, mode, modeSource, share, shareSource, sources:[…], brief, briefPending } */
    cross: (key, o) => get(`/api/chats/${enc(key)}/cross`, o),
    /** Rewrite this chat's summary for other chats now → Brief */
    refreshBrief: (key, o) => post(`/api/chats/${enc(key)}/cross/refresh`, {}, { timeout: 120000, ...o }),
    /** → ChatAssignment (hand-off cleared) */
    resumeHandoff: (key, o) => post(`/api/chats/${enc(key)}/handoff/resume`, {}, o),
    /** body: { text?, force? } → { ok, text } */
    reveal: (key, body = {}, o) => post(`/api/chats/${enc(key)}/reveal`, body, { timeout: 60000, ...o }),
  },

  /** Wave 3: daily recaps. */
  recaps: {
    /** q: { chat?, limit? } → { items:[Recap] } newest first */
    list: (q = {}, o) => get('/api/recaps' + qs(q), o),
    /** chatKey '' = every eligible chat → { items:[Recap] } */
    generate: (chatKey, o) => post('/api/recaps/generate', chatKey ? { chatKey } : {}, { timeout: 300000, ...o }),
  },

  /** Wave 3: missions and achievements. */
  missions: {
    /** → { templates, active, history, achievements } */
    list: (o) => get('/api/missions', o),
    /** body: { chatKey, templateId?, blanks?, goal? } → ChatAssignment */
    start: (body, o) => post('/api/missions/start', body, o),
    abandon: (chatKey, o) => post(`/api/missions/${enc(chatKey)}/abandon`, {}, o),
  },

  /** Wave 3: "Clone yourself". */
  clone: {
    /** → { enabled, count, since, preview:[text] } */
    samples: (o) => get('/api/clone/samples', o),
    deleteSamples: (o) => del('/api/clone/samples', o),
    /** body: { name?, extra?, provider?, model? } → { persona, raw, sampleCount } */
    draft: (body, o) => post('/api/clone/draft', body, { timeout: 180000, ...o }),
  },

  behavior: {
    presets: (o) => get('/api/behavior/presets', o),
    /** body: { kind: 'dm'|'group', profile, text?, samples?, approval? } */
    sample: (body, o) => post('/api/behavior/sample', body, o),
  },

  personas: {
    list: (o) => get('/api/personas', o),
    get: (id, o) => get(`/api/personas/${enc(id)}`, o),
    create: (p, o) => post('/api/personas', p, o),
    update: (id, p, o) => put(`/api/personas/${enc(id)}`, p, o),
    remove: (id, o) => del(`/api/personas/${enc(id)}`, o),
    duplicate: (id, o) => post(`/api/personas/${enc(id)}/duplicate`, {}, o),
    reset: (id, o) => post(`/api/personas/${enc(id)}/reset`, {}, o),
    uploadAvatar: (id, file, o) => {
      const form = new FormData();
      form.append('file', file);
      return request('PUT', `/api/personas/${enc(id)}/avatar`, undefined, { ...o, form });
    },
    removeAvatar: (id, o) => del(`/api/personas/${enc(id)}/avatar`, o),
    avatarURL: (id, v) => `/api/personas/${enc(id)}/avatar?v=${enc(v || 0)}`,
    draft: (body, o) => post('/api/personas/draft', body, o),
    promptPreview: (id, chatKey, o) => get(`/api/personas/${enc(id)}/prompt-preview` + qs({ chatKey }), o),
    expressionPreview: (id, body, o) => post(`/api/personas/${enc(id)}/expression-preview`, body, o),
  },

  playground: {
    start: (personaId, o) => post('/api/playground', { personaId }, o),
    /** extra: { source?, crossMode? } — "Pretend this group includes…" (a private chat of this persona; '' = none). */
    send: (id, text, group, o, goal, extra) => post(`/api/playground/${enc(id)}/messages`, { text, group: !!group, ...(goal == null ? {} : { goal }), ...(extra || {}) }, o),
    initiate: (id, group, hint, goal, o) => post(`/api/playground/${enc(id)}/messages`, { initiate: true, text: hint || '', group: !!group, ...(goal == null ? {} : { goal }) }, o),
    end: (id, o) => del(`/api/playground/${enc(id)}`, o),
  },

  approvals: {
    list: (o) => get('/api/approvals', o),
    /** draft (co-pilot): 0-based index of the chosen draft; text (if given) wins. */
    approve: (id, text, o, draft) => post(`/api/approvals/${enc(id)}/approve`, {
      ...(text == null ? {} : { text }), ...(draft == null ? {} : { draft }),
    }, o),
    regenerate: (id, o) => post(`/api/approvals/${enc(id)}/regenerate`, {}, { timeout: 180000, ...o }),
    discard: (id, o) => del(`/api/approvals/${enc(id)}`, o),
  },

  activity: {
    list: (q = {}, o) => get('/api/activity' + qs({ since: 0, limit: 200, ...q }), o),
    clear: (o) => del('/api/activity', o),
  },
};
