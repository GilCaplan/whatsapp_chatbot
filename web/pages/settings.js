// pages/settings.js — server, AI, behaviour, notifications, safety, memory & recap,
// appearance, WhatsApp, data, about. Wave 3 sections live in components/settings/*.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadSettings, loadBehaviorMeta } from '../store.js';
import { seg, fieldRow, toast, confirmSheet, progressRing, spinner, busy, toggle } from '../ui.js';
import { statusDot, waMeta, providerLabel } from '../components/status.js';
import { openWASheet, logoutAndRelink } from '../components/wa-sheet.js';
import { movingTo, showStopped } from '../system.js';
import { debounce, fmtMs, fmtBytes, copyText, prettyPhone } from '../util.js';
import { behaviourForm } from '../components/behaviour-form.js';
import { presetCards } from '../components/preset-cards.js';
import { createTimeline } from '../components/reply-timeline.js';
import { detectPreset, apiKind, same } from '../components/behaviour-meta.js';
import { vibeDials } from '../components/vibe-dials.js';
import { helpTip } from '../components/help-tip.js';
import { notificationsSection } from '../components/settings/notifications.js';
import { safetySection } from '../components/settings/safety.js';
import { memorySection } from '../components/settings/memory.js';
import { cloneRows } from '../components/settings/clone-rows.js';

const SECTIONS = [
  { id: 'server', label: 'Server', icon: 'server' },
  { id: 'ai', label: 'AI brain', icon: 'brain' },
  { id: 'behaviour', label: 'Behaviour', icon: 'sliders' },
  { id: 'notifications', label: 'Notifications', icon: 'megaphone' },
  { id: 'safety', label: 'Safety', icon: 'shield' },
  { id: 'memory', label: 'Memory & recap', icon: 'pin' },
  { id: 'appearance', label: 'Appearance', icon: 'sun' },
  { id: 'whatsapp', label: 'WhatsApp', icon: 'phone' },
  { id: 'data', label: 'Data', icon: 'folder' },
  { id: 'about', label: 'About', icon: 'info' },
];

const ANTHROPIC_FALLBACK = [
  { id: 'claude-sonnet-5-5', label: 'Claude Sonnet 5.5' },
  { id: 'claude-opus-5-5', label: 'Claude Opus 5.5' },
  { id: 'claude-haiku-4-5', label: 'Claude Haiku 4.5' },
];

export default function Settings(ctx) {
  const update = () => ctx.update();
  const st = {
    saved: {},            // section -> timestamp of last save
    drafts: {},           // path -> raw input value while typing
    ports: null, portsLoading: false, pickedPort: null, customPort: '', switching: false,
    ollama: null, ollamaLoading: false,
    models: {}, modelsLoading: {},
    tests: {}, testing: {},
    keys: { anthropic: '', openai: '' }, showKey: { anthropic: false, openai: false },
    pullName: '', pullJob: '',
    active: 'server',
    llmAdvanced: false,
    bkind: 'private',     // Behaviour tab: private | group
    bui: {
      private: { open: new Set(['responsiveness', 'timing']), adv: false },
      group: { open: new Set(['responsiveness', 'timing']), adv: false },
    },
    bpend: { private: {}, group: {} },   // optimistic edits not yet confirmed by the server
    bhAdvanced: (() => { try { return sessionStorage.getItem('doppel.bh.adv') === '1'; } catch { return false; } })(),
  };
  const timeline = createTimeline({ onUpdate: () => update() });
  const timers = {};

  const s = () => store.state.settings || {};
  const llm = () => s().llm || {};
  const beh = () => s().behavior || {};

  async function save(partial, section) {
    try {
      const res = await api.settings.put(partial);
      if (res) {
        const { secrets, ...settings } = res;
        store.set({ settings, ...(secrets ? { secrets } : {}) });
      }
      st.saved[section] = Date.now();
      update();
      clearTimeout(timers[section]);
      timers[section] = setTimeout(() => { delete st.saved[section]; update(); }, 1800);
      return true;
    } catch { return false; }
  }

  const debouncers = {};
  /** Draft-aware debounced save for text/number inputs. */
  function draftInput(path, section, build, parse = (v) => v) {
    return (e) => {
      const raw = e.target.value;
      st.drafts[path] = raw;
      if (!debouncers[path]) {
        debouncers[path] = debounce(async (value) => {
          const parsed = parse(value);
          if (parsed === undefined) return;
          const ok = await save(build(parsed), section);
          if (ok && st.drafts[path] === value) { delete st.drafts[path]; update(); }
        }, 600);
      }
      debouncers[path](raw);
    };
  }
  const dv = (path, value) => (path in st.drafts ? st.drafts[path] : (value ?? ''));
  const intParse = (min, max) => (v) => {
    if (String(v).trim() === '') return undefined;
    const n = parseInt(v, 10);
    if (isNaN(n)) return undefined;
    return Math.max(min, Math.min(max, n));
  };

  function savedTick(section) {
    return st.saved[section] ? html`<span class="saved-tick">${icon('check', 'ic-sm')}Saved</span>` : '';
  }

  function sectionHead(id, title, sub, color) {
    const sec = SECTIONS.find((x) => x.id === id);
    return html`<div class="section-head">
      <span class="s-icon" style=${color ? `--c:${color}` : ''}>${icon(sec.icon)}</span>
      <div class="grow"><h2>${title}</h2>${sub ? html`<p>${sub}</p>` : ''}</div>
      ${savedTick(id)}
    </div>`;
  }

  // ── Server ──
  async function findPorts() {
    st.portsLoading = true; update();
    try { st.ports = await api.system.ports({ limit: 12 }); } catch { /* toasted */ }
    st.portsLoading = false; update();
  }

  async function switchPort() {
    const port = st.pickedPort || parseInt(st.customPort, 10);
    if (!port) return;
    st.switching = true; update();
    try {
      const r = await api.system.setPort(port);
      if (r && r.url) movingTo(r.url);
    } catch { st.switching = false; update(); }
  }

  function serverSection() {
    const h = store.state.health || {};
    const port = h.port || s().port;
    const url = `${location.protocol}//${location.hostname}:${port || location.port}`;
    const ports = st.ports;
    return html`<section class="card section" id="sec-server" data-section="server">
      ${sectionHead('server', 'Server', 'Doppel runs a small private web server on this Mac.', 'linear-gradient(135deg,#64748b,#334155)')}
      <div class="field-row">
        <div><div class="field-label">Address</div><div class="field-help">Only this Mac can open it.</div></div>
        <div class="row gap-6"><code class="url-pill">${url}</code>
          <button class="btn btn-ghost btn-icon btn-sm" aria-label="Copy address" @click=${() => copyText(url).then(() => toast('Copied', { type: 'success' }))}>${icon('copy')}</button></div>
      </div>
      <div class="field-row field-row-stack">
        <div class="row">
          <div class="grow"><div class="field-label">Port</div><div class="field-help">Change it if another app needs port ${port}. The page will move to the new address automatically.</div></div>
          <button class=${'btn btn-glass btn-sm ' + (st.portsLoading ? 'loading' : '')} @click=${findPorts}>${icon('search')}Find free ports</button>
        </div>
        ${ports ? html`<div class="row row-wrap gap-6 mt-8">
            ${(ports.free || []).length ? ports.free.map((p) => html`<button class="chip" aria-pressed=${String(st.pickedPort === p)} @click=${() => { st.pickedPort = p; st.customPort = ''; update(); }}>${p}</button>`)
              : html`<span class="small muted">No free ports found in that range.</span>`}
            <input class="input input-sm input-number" type="number" min="1024" max="65535" placeholder="Other…" .value=${st.customPort}
              @input=${(e) => { st.customPort = e.target.value; st.pickedPort = null; update(); }}>
            <span class="grow"></span>
            <button class=${'btn btn-primary btn-sm ' + (st.switching ? 'loading' : '')} ?disabled=${!(st.pickedPort || parseInt(st.customPort, 10) >= 1024)} @click=${switchPort}>
              ${icon('arrow-right')}Switch to ${st.pickedPort || st.customPort || '…'}
            </button>
          </div>` : ''}
      </div>
    </section>`;
  }

  // ── AI ──
  async function checkOllama() {
    st.ollamaLoading = true; update();
    try { st.ollama = await api.ollama.status({ quiet: true }); } catch (e) { st.ollama = { reachable: false, error: e.message }; }
    st.ollamaLoading = false; update();
  }

  async function loadModels(provider, force = false) {
    if (!force && st.models[provider]) return;
    st.modelsLoading[provider] = true; update();
    try {
      const r = await api.llm.models(provider, { quiet: true });
      st.models[provider] = (r && r.models) || [];
    } catch { st.models[provider] = []; }
    st.modelsLoading[provider] = false; update();
  }

  async function test(provider) {
    st.testing[provider] = true; st.tests[provider] = null; update();
    try { st.tests[provider] = await api.llm.test(provider); } catch (e) { st.tests[provider] = { ok: false, error: e.message }; }
    st.testing[provider] = false; update();
  }

  async function saveKey(provider) {
    const value = st.keys[provider].trim();
    if (!value) return;
    try {
      const secrets = await api.settings.secrets(provider === 'anthropic' ? { anthropicKey: value } : { openaiKey: value });
      store.set({ secrets });
      st.keys[provider] = '';
      st.models[provider] = null;
      toast(`${providerLabel(provider)} key saved`, { type: 'success' });
      loadModels(provider, true);
      test(provider);
    } catch { /* toasted */ }
  }

  async function removeKey(provider) {
    const ok = await confirmSheet({ title: `Remove the ${providerLabel(provider)} key?`, body: 'Personas using this provider will stop replying until a key is added again.', confirm: 'Remove key', danger: true, iconName: 'key' });
    if (!ok) return;
    try {
      const secrets = await api.settings.secrets(provider === 'anthropic' ? { anthropicKey: '' } : { openaiKey: '' });
      store.set({ secrets });
      st.tests[provider] = null;
      update();
    } catch { /* toasted */ }
  }

  async function pull() {
    const name = st.pullName.trim();
    if (!name) return;
    try {
      const r = await api.ollama.pull(name);
      st.pullJob = r && r.jobId;
      st.pullName = '';
      update();
    } catch { /* toasted */ }
  }

  function testLine(provider) {
    const t = st.tests[provider];
    if (st.testing[provider]) return html`<div class="row gap-8 small muted mt-8">${spinner()}Asking ${providerLabel(provider)} to say hi…</div>`;
    if (!t) return '';
    return t.ok
      ? html`<div class="banner success mt-8 small">${icon('check')}<div><strong>Works</strong> · ${fmtMs(t.latencyMs)}${t.model ? html` · <code>${t.model}</code>` : ''}${t.sample ? html`<div class="muted mt-4">“${t.sample}”</div>` : ''}</div></div>`
      : html`<div class="banner danger mt-8 small">${icon('warning')}<div><strong>Didn't work.</strong> ${t.error || ''}</div></div>`;
  }

  function modelSelect(provider, value, onPick, fallback = []) {
    const list = st.models[provider];
    const loading = st.modelsLoading[provider];
    const models = list && list.length ? list : fallback;
    if (loading && !models.length) return html`<div class="row gap-8 small muted">${spinner()}Loading…</div>`;
    if (!models.length) {
      return html`<input class="input input-sm mono" style="width:220px" placeholder="model name" .value=${value || ''}
        @change=${(e) => onPick(e.target.value.trim())}>`;
    }
    return html`<select class="select input-sm" style="width:auto;max-width:260px" @change=${(e) => onPick(e.target.value)}>
      ${!value ? html`<option value="" selected>Choose a model</option>` : ''}
      ${models.map((m) => html`<option value=${m.id} ?selected=${m.id === value}>${m.label || m.id}</option>`)}
      ${value && !models.some((m) => m.id === value) ? html`<option value=${value} selected>${value}</option>` : ''}
    </select>`;
  }

  function keyRow(provider) {
    const sec = store.state.secrets[provider] || {};
    return html`<div class="field-row field-row-stack">
      <div class="field-label">${icon('key')}API key ${sec.set ? html`<span class="chip chip-sm chip-green">${icon('lock')}saved ${sec.hint || ''}</span>` : html`<span class="chip chip-sm">not set</span>`}</div>
      <div class="row gap-8 row-wrap">
        <div class="input-wrap grow" style="min-width:220px">
          <input class="input has-suffix mono" type=${st.showKey[provider] ? 'text' : 'password'} autocomplete="off" spellcheck="false"
            placeholder=${sec.set ? `Paste a new key to replace ${sec.hint || ''}` : (provider === 'anthropic' ? 'sk-ant-…' : 'sk-…')}
            .value=${st.keys[provider]} @input=${(e) => { st.keys[provider] = e.target.value; update(); }}
            @keydown=${(e) => { if (e.key === 'Enter') saveKey(provider); }}>
          <button class="btn btn-ghost btn-icon btn-sm input-suffix" type="button" aria-label="Show or hide key"
            @click=${() => { st.showKey[provider] = !st.showKey[provider]; update(); }}>${icon(st.showKey[provider] ? 'eye-off' : 'eye')}</button>
        </div>
        <button class="btn btn-primary btn-sm" ?disabled=${!st.keys[provider].trim()} @click=${() => saveKey(provider)}>Save key</button>
        ${sec.set ? html`<button class="btn btn-glass btn-sm" @click=${() => test(provider)}>${icon('bolt')}Test</button>
          <button class="btn btn-ghost btn-sm danger-text" @click=${() => removeKey(provider)}>Remove</button>` : ''}
      </div>
      <div class="field-help">Stored only on this Mac, never shown again after saving.</div>
      ${testLine(provider)}
    </div>`;
  }

  function aiSection() {
    const L = llm();
    const o = st.ollama;
    const job = st.pullJob ? store.state.pulls[st.pullJob] : null;
    const temp = Number(dv('llm.temperature', L.temperature ?? 0.8));
    return html`<section class="card section" id="sec-ai" data-section="ai">
      ${sectionHead('ai', 'AI brain', 'Which AI writes the replies, and how.', 'linear-gradient(135deg,#22d3ee,#6366f1)')}
      <div class="field-row">
        <div><div class="field-label">Default provider</div><div class="field-help">Used by every persona unless it picks its own.</div></div>
        ${seg([
          { value: 'ollama', label: 'Ollama', icon: 'cpu' },
          { value: 'anthropic', label: 'Claude', icon: 'sparkles' },
          { value: 'openai', label: 'OpenAI', icon: 'bolt' },
        ], L.defaultProvider, (v) => save({ llm: { defaultProvider: v } }, 'ai'), { label: 'Default provider' })}
      </div>
      <div class="field-row field-row-stack">
        <div class="row"><div class="grow"><div class="field-label">Creativity</div>
          <div class="field-help">Lower = predictable and consistent. Higher = more playful and surprising.</div></div></div>
        <div class="range">
          <span class="tiny faint">Focused</span>
          <input type="range" min="0" max="1.5" step="0.05" .value=${String(temp)} style=${`--pct:${(temp / 1.5) * 100}%`}
            aria-label="Temperature"
            @input=${(e) => { st.drafts['llm.temperature'] = e.target.value; update(); }}
            @change=${async (e) => { const v = parseFloat(e.target.value); if (await save({ llm: { temperature: v } }, 'ai')) { delete st.drafts['llm.temperature']; update(); } }}>
          <span class="tiny faint">Creative</span>
          <span class="range-value">${temp.toFixed(2)}</span>
        </div>
      </div>

      <div class="provider-card glass" data-key="p-ollama">
        <div class="pc-head">${icon('cpu')}<b>Ollama</b><span class="faint small">on this Mac · free & private</span><span class="grow"></span>
          ${L.defaultProvider === 'ollama' ? html`<span class="chip chip-sm chip-green">default</span>` : ''}</div>
        <div class="field-row">
          <div><div class="field-label">Status</div>
            <div class="field-help">${st.ollamaLoading ? 'Checking…' : !o ? '' : o.reachable ? html`<span class="row gap-6">${statusDot('connected')}Running${o.version ? ` · v${o.version}` : ''} · ${(o.models || []).length} model${(o.models || []).length === 1 ? '' : 's'}</span>`
              : html`<span class="row gap-6">${statusDot('error')}Not reachable${o.error ? ` — ${o.error}` : ''}</span>`}</div></div>
          <button class=${'btn btn-glass btn-sm ' + (st.ollamaLoading ? 'loading' : '')} @click=${() => { checkOllama(); loadModels('ollama', true); }}>${icon('refresh')}Check</button>
        </div>
        <div class="field-row">
          <div><div class="field-label">Address</div><div class="field-help">Usually http://127.0.0.1:11434</div></div>
          <input class="input input-sm mono" style="width:240px" .value=${dv('llm.ollamaURL', L.ollamaURL)}
            @input=${draftInput('llm.ollamaURL', 'ai', (v) => ({ llm: { ollamaURL: v } }), (v) => v.trim() || undefined)}>
        </div>
        <div class="field-row">
          <div><div class="field-label">Model</div><div class="field-help">Installed chat models.</div></div>
          <div class="row gap-6">${modelSelect('ollama', L.ollamaModel, (v) => v && save({ llm: { ollamaModel: v } }, 'ai'))}
            <button class="btn btn-glass btn-sm" @click=${() => test('ollama')}>${icon('bolt')}Test</button></div>
        </div>
        ${testLine('ollama')}
        <div class="field-row field-row-stack">
          <div><div class="field-label">Download a model</div><div class="field-help">Type a name from <a href="https://ollama.com/library" target="_blank" rel="noopener noreferrer">ollama.com/library</a>, e.g. <code>llama3.1:8b</code> or <code>qwen2.5:7b</code>.</div></div>
          <div class="row gap-8">
            <input class="input input-sm mono" style="max-width:260px" placeholder="llama3.1:8b" .value=${st.pullName}
              @input=${(e) => { st.pullName = e.target.value; update(); }} @keydown=${(e) => { if (e.key === 'Enter') pull(); }}>
            <button class="btn btn-primary btn-sm" ?disabled=${!st.pullName.trim() || (job && !job.done)} @click=${pull}>${icon('download')}Download</button>
          </div>
          ${job ? html`<div class="pull-card mt-8">
            ${job.done && !job.error ? html`<span class="tick-badge sm">${icon('check')}</span>` : progressRing(job.percent || 0, { size: 46, stroke: 4 })}
            <div class="grow"><div class="small"><b>${job.name}</b> — ${job.error ? html`<span style="color:var(--red)">${job.error}</span>` : job.done ? 'ready' : (job.status || 'starting…')}</div>
              ${job.total && !job.done ? html`<div class="tiny faint">${fmtBytes(job.completed)} of ${fmtBytes(job.total)}</div>` : ''}</div>
            ${job.done ? html`<button class="btn btn-ghost btn-sm" @click=${() => { st.pullJob = ''; loadModels('ollama', true); checkOllama(); }}>OK</button>` : ''}
          </div>` : ''}
        </div>
      </div>

      <div class="provider-card glass" data-key="p-anthropic">
        <div class="pc-head">${icon('sparkles')}<b>Claude</b><span class="faint small">by Anthropic</span><span class="grow"></span>
          ${L.defaultProvider === 'anthropic' ? html`<span class="chip chip-sm chip-green">default</span>` : ''}</div>
        ${keyRow('anthropic')}
        <div class="field-row">
          <div><div class="field-label">Model</div><div class="field-help">Sonnet is a great balance; Haiku is fastest.</div></div>
          ${modelSelect('anthropic', L.anthropicModel, (v) => v && save({ llm: { anthropicModel: v } }, 'ai'), ANTHROPIC_FALLBACK)}
        </div>
      </div>

      <div class="provider-card glass" data-key="p-openai">
        <div class="pc-head">${icon('bolt')}<b>OpenAI</b><span class="faint small">or any compatible API</span><span class="grow"></span>
          ${L.defaultProvider === 'openai' ? html`<span class="chip chip-sm chip-green">default</span>` : ''}</div>
        ${keyRow('openai')}
        <div class="field-row">
          <div><div class="field-label">Model</div></div>
          ${modelSelect('openai', L.openaiModel, (v) => v && save({ llm: { openaiModel: v } }, 'ai'))}
        </div>
        <div class="field-row">
          <div><div class="field-label">Base URL</div><div class="field-help">Change only for OpenAI-compatible services.</div></div>
          <input class="input input-sm mono" style="width:260px" .value=${dv('llm.openaiBaseURL', L.openaiBaseURL)}
            @input=${draftInput('llm.openaiBaseURL', 'ai', (v) => ({ llm: { openaiBaseURL: v } }), (v) => v.trim() || undefined)}>
        </div>
      </div>

      <button class="disclosure mt-12" aria-expanded=${String(st.llmAdvanced)} @click=${() => { st.llmAdvanced = !st.llmAdvanced; update(); }}>
        ${icon('sliders', 'ic-sm')}<span>Advanced</span>${icon(st.llmAdvanced ? 'chevron-up' : 'chevron-down', 'ic-sm')}</button>
      ${st.llmAdvanced ? html`
        ${fieldRow({ label: 'Max reply length (tokens)', help: 'Upper limit for one reply. Real texts are short; 400 is plenty.', control: html`<input class="input input-sm input-number" type="number" min="32" max="4096"
          .value=${String(dv('llm.replyMaxTokens', L.replyMaxTokens))} @input=${draftInput('llm.replyMaxTokens', 'ai', (v) => ({ llm: { replyMaxTokens: v } }), intParse(32, 4096))}>` })}
        ${fieldRow({ label: 'Ollama context size', help: 'How much text a local model can consider at once. Bigger uses more memory.', control: html`<input class="input input-sm input-number" type="number" min="1024" max="131072" step="1024"
          .value=${String(dv('llm.ollamaNumCtx', L.ollamaNumCtx))} @input=${draftInput('llm.ollamaNumCtx', 'ai', (v) => ({ llm: { ollamaNumCtx: v } }), intParse(1024, 131072))}>` })}
      ` : ''}
    </section>`;
  }

  // ── Behaviour ──
  const presetsList = () => (store.state.behaviorMeta && store.state.behaviorMeta.presets) || [];
  /** Server profile with not-yet-confirmed edits on top. */
  const profileFor = (key) => ({ ...(beh()[key] || {}), ...st.bpend[key] });
  const activePreset = (key) => {
    const p = profileFor(key);
    return presetsList().length ? detectPreset(p, presetsList(), key) : (p.preset || 'custom');
  };

  async function flushBehavior() {
    const sent = {};
    const body = {};
    for (const key of ['private', 'group']) {
      const pend = st.bpend[key];
      if (!Object.keys(pend).length) continue;
      sent[key] = { ...pend };
      body[key] = { ...pend, preset: activePreset(key) };
    }
    if (!Object.keys(body).length) return;
    const ok = await save({ behavior: body }, 'behaviour');
    for (const key of Object.keys(sent)) {
      for (const [f, val] of Object.entries(sent[key])) {
        // Drop confirmed (or rejected) edits; keep anything changed since.
        if (same(st.bpend[key][f], val)) delete st.bpend[key][f];
      }
    }
    if (!ok) update();
  }
  const flushSoon = debounce(flushBehavior, 500);

  function editBehavior(key, field, value) {
    st.bpend[key][field] = value;
    update();
    flushSoon();
  }

  function pickPreset(key, id) {
    const def = presetsList().find((p) => p.id === id);
    if (!def || !def[key]) return;
    st.bpend[key] = { ...def[key], preset: id };
    update();
    flushSoon.cancel();
    flushBehavior().then(() => toast(`${def.label || id} applied to ${key === 'group' ? 'group chats' : 'private chats'}`, { type: 'success' }));
  }

  /** Several fields at once (a vibe dial): one optimistic edit, one save. */
  function editMany(key, fields) {
    Object.assign(st.bpend[key], fields);
    update();
    flushSoon();
  }

  function setAdvanced(on) {
    st.bhAdvanced = on;
    try { sessionStorage.setItem('doppel.bh.adv', on ? '1' : ''); } catch { /* ignore */ }
    update();
  }

  function behaviourSection() {
    const B = beh();
    const key = st.bkind;
    const prof = profileFor(key);
    const meta = store.state.behaviorMeta;
    const hasProfile = B[key] && Object.keys(B[key]).length;
    const adv = !!st.bhAdvanced;
    return html`<section class="card section" id="sec-behaviour" data-section="behaviour">
      ${sectionHead('behaviour', 'Behaviour', 'How personas notice, read, think, type and reply. Any chat can have its own on top of these.', 'linear-gradient(135deg,#34d399,#0ea5e9)')}
      <div class="bh-tabs">
        ${seg([
          { value: 'private', label: 'Private chats', icon: 'user' },
          { value: 'group', label: 'Group chats', icon: 'group' },
        ], key, (v) => { st.bkind = v; update(); }, { label: 'Which chats', cls: 'seg-lg' })}
        <span class="small muted">${key === 'group' ? 'Used in every group a persona is assigned to.' : 'Used in every one-to-one chat a persona is assigned to.'}</span>
      </div>
      ${!hasProfile ? html`<div class="center" style="min-height:120px">${spinner()}</div>` : html`
        <div class="bh-block">
          <div class="eyebrow mb-8 row gap-4">Start from a preset ${helpTip('presets')}</div>
          ${presetCards(presetsList(), activePreset(key), (id) => pickPreset(key, id))}
        </div>
        <div class="bh-block">
          <div class="eyebrow mb-8">Or turn the dials</div>
          ${vibeDials({ kind: apiKind(key), profile: prof, meta, scope: 'settings-' + key, onChange: (fields) => editMany(key, fields) })}
        </div>
        <div class="bh-block">${timeline.view({ kind: apiKind(key), profile: prof })}</div>
        <div class="bh-block adv-switch-row">
          <div class="grow"><div class="field-label row gap-4">Advanced ${helpTip('advanced')}</div>
            <div class="field-help">${adv ? 'Every setting, grouped. Rows with a coloured label are set by that dial.' : 'Active hours, message shape, limits, check-ins, trick protection and the test trigger.'}</div></div>
          ${toggle(adv, setAdvanced, { label: 'Show advanced settings' })}
        </div>
        ${adv ? html`<div class="bh-block adv-reveal">
          ${fieldRow({ label: 'Test trigger', help: html`When <b>you</b> send a message starting with this in an assigned chat, the persona answers you straight away — handy for testing. E.g. <code>${B.triggerPrefix || '1'} hi</code>`,
            control: html`<input class="input input-sm" style="width:90px;text-align:center" maxlength="8" .value=${dv('behavior.triggerPrefix', B.triggerPrefix)}
              aria-label="Test trigger" @input=${draftInput('behavior.triggerPrefix', 'behaviour', (v) => ({ behavior: { triggerPrefix: v } }))}>` })}
          <div class="mt-12">${behaviourForm({ kind: apiKind(key), values: prof, mode: 'profile', meta, ui: st.bui[key], onUpdate: update,
            onChange: (f, val) => editBehavior(key, f, val) })}</div>
        </div>` : ''}`}
    </section>`;
  }

  // Context for sections that live in their own modules (components/settings/*).
  const sectionCtx = { s, save, sectionHead, update, savedTick };

  function appearanceSection() {
    return html`<section class="card section" id="sec-appearance" data-section="appearance">
      ${sectionHead('appearance', 'Appearance', '', 'linear-gradient(135deg,#fbbf24,#f472b6)')}
      ${fieldRow({ label: 'Theme', help: 'System follows your Mac’s light/dark setting.',
        control: seg([
          { value: 'system', label: 'System', icon: 'system' },
          { value: 'light', label: 'Light', icon: 'sun' },
          { value: 'dark', label: 'Dark', icon: 'moon' },
        ], s().theme || 'system', (v) => {
          store.set({ settings: { ...s(), theme: v } });
          save({ theme: v }, 'appearance');
        }, { label: 'Theme' }) })}
    </section>`;
  }

  function whatsappSection() {
    const wa = store.state.wa || {};
    const m = waMeta(wa.state);
    const me = wa.me || {};
    return html`<section class="card section" id="sec-whatsapp" data-section="whatsapp">
      ${sectionHead('whatsapp', 'WhatsApp', '', 'linear-gradient(135deg,#25d366,#128c7e)')}
      <div class="field-row">
        <div class="row gap-12">
          ${wa.state === 'connected' ? html`<chat-avatar jid=${me.jid || ''} name=${me.pushName || 'Me'} src=${me.avatarUrl || ''} size="44"></chat-avatar>` : ''}
          <div><div class="field-label">${statusDot(wa.state)}${wa.state === 'connected' ? (me.pushName || 'Connected') : m.label}</div>
            <div class="field-help">${wa.state === 'connected' ? prettyPhone(me.phone) : m.help}</div></div>
        </div>
        <div class="row gap-8 row-wrap" style="justify-content:flex-end">
          <button class="btn btn-glass btn-sm" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting…'); })}>${icon('refresh')}Reconnect</button>
          <button class="btn btn-glass btn-sm" @click=${() => openWASheet()}>${icon('qr')}${wa.state === 'connected' ? 'Details' : 'Link'}</button>
          ${wa.state === 'connected' ? html`<button class="btn btn-danger btn-sm" @click=${() => logoutAndRelink().then((ok) => { if (ok) openWASheet(); })}>${icon('logout')}Log out & relink</button>` : ''}
        </div>
      </div>
    </section>`;
  }

  function dataSection() {
    const h = store.state.health || {};
    return html`<section class="card section" id="sec-data" data-section="data">
      ${sectionHead('data', 'Data', 'Personas, chats, memories, keys and your WhatsApp session live in one folder on this Mac.', 'linear-gradient(135deg,#60a5fa,#a78bfa)')}
      <div class="field-row">
        <div class="grow" style="min-width:0"><div class="field-label">Data folder</div><div class="field-help mono ellipsis" title=${h.dataDir || ''}>${h.dataDir || '—'}</div></div>
        <button class="btn btn-glass btn-sm" @click=${busy(() => api.system.openDataDir())}>${icon('folder')}Open in Finder</button>
      </div>
      ${cloneRows(sectionCtx) /* Clone yourself consent (settings/clone-rows.js) */}
    </section>`;
  }

  function aboutSection() {
    const h = store.state.health || {};
    return html`<section class="card section" id="sec-about" data-section="about">
      ${sectionHead('about', 'About', '', 'var(--brand-grad)')}
      <div class="about-row">
        <img src="/assets/favicon.svg" alt="" width="56" height="56">
        <div class="grow">
          <div class="h3">WhatsApp Doppel ${h.version ? html`<span class="faint">v${h.version}</span>` : ''}</div>
          <div class="small muted">Running since ${h.startedAt ? html`<rel-time datetime=${h.startedAt}></rel-time>` : '—'}
            ${h.fakeWA ? html` · <span class="chip chip-sm chip-amber">fake WhatsApp</span>` : ''}${h.fakeLLM ? html` · <span class="chip chip-sm chip-amber">fake AI</span>` : ''}</div>
        </div>
      </div>
      <div class="row gap-8 mt-16 row-wrap">
        <button class="btn btn-glass btn-sm" @click=${async () => { if (await save({ onboardingCompleted: false }, 'about')) ctx.navigate('/welcome'); }}>${icon('sparkles')}Run setup again</button>
        <span class="grow"></span>
        <button class="btn btn-danger" @click=${quit}>${icon('power')}Quit Doppel</button>
      </div>
    </section>`;
  }

  async function quit() {
    const ok = await confirmSheet({
      title: 'Quit Doppel?',
      body: 'Personas stop replying until you open the app again. Nothing is deleted.',
      confirm: 'Quit',
      danger: true,
      iconName: 'power',
    });
    if (!ok) return;
    try { await api.system.quit(); } catch { /* the server may close the connection first */ }
    showStopped();
  }

  function view() {
    if (!store.state.settings) return html`<div class="center" style="min-height:40vh">${spinner('spinner-lg')}</div>`;
    return html`<div class="settings">
      <nav class="settings-nav glass" aria-label="Settings sections">
        ${SECTIONS.map((x) => html`<button class=${'snav ' + (st.active === x.id ? 'on' : '')} @click=${() => {
          st.active = x.id; update();
          const el = document.getElementById('sec-' + x.id);
          if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }}>${icon(x.icon)}<span>${x.label}</span></button>`)}
      </nav>
      <div class="settings-body">
        ${serverSection()}
        ${aiSection()}
        ${behaviourSection()}
        ${notificationsSection(sectionCtx)}
        ${safetySection(sectionCtx)}
        ${memorySection(sectionCtx)}
        ${appearanceSection()}
        ${whatsappSection()}
        ${dataSection()}
        ${aboutSection()}
      </div>
    </div>`;
  }

  let scrollHost = null;
  const onScroll = debounce(() => {
    if (!scrollHost) return;
    const top = scrollHost.getBoundingClientRect().top;
    let best = 'server';
    for (const x of SECTIONS) {
      const el = document.getElementById('sec-' + x.id);
      if (el && el.getBoundingClientRect().top - top < 140) best = x.id;
    }
    if (best !== st.active) { st.active = best; update(); }
  }, 60);

  return {
    title: 'Settings',
    view,
    mount() {
      loadSettings().catch(() => {});
      loadBehaviorMeta().catch(() => {});
      checkOllama();
      loadModels('ollama');
      if (store.state.secrets.anthropic && store.state.secrets.anthropic.set) loadModels('anthropic');
      if (store.state.secrets.openai && store.state.secrets.openai.set) loadModels('openai');
      scrollHost = document.getElementById('page');
      if (scrollHost) scrollHost.addEventListener('scroll', onScroll, { passive: true });
    },
    unmount() {
      if (scrollHost) scrollHost.removeEventListener('scroll', onScroll);
      for (const k in debouncers) if (debouncers[k].pending()) debouncers[k].flush(st.drafts[k]);
      if (flushSoon.pending()) flushSoon.flush();
      timeline.destroy();
      for (const k in timers) clearTimeout(timers[k]);
    },
  };
}
