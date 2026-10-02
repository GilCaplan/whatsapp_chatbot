// pages/onboarding.js — first-run setup: Brain → WhatsApp → Persona → First chat.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadSettings, loadPersonas } from '../store.js';
import { seg, toggle, toast, progressRing, spinner } from '../ui.js';
import { createQRCard } from '../components/qr-card.js';
import { personaTiles, personaPicker } from '../components/persona-picker.js';
import { createChatPicker } from '../components/chat-picker.js';
import { assignChat } from '../components/assign-sheet.js';
import { openAIBuilder } from '../components/ai-builder.js';
import { fmtMs, fmtBytes } from '../util.js';

const STEPS = [
  { key: 'brain', label: 'Brain', icon: 'brain' },
  { key: 'wa', label: 'WhatsApp', icon: 'phone' },
  { key: 'persona', label: 'Persona', icon: 'personas' },
  { key: 'chat', label: 'First chat', icon: 'chats' },
];

const RECOMMENDED_MODEL = 'llama3.1:8b';

export default function Onboarding(ctx) {
  const update = () => ctx.update();
  const settings = store.state.settings || {};
  const llm = settings.llm || {};

  const st = {
    step: 0,
    dir: 'right',
    tab: ['ollama', 'anthropic', 'openai'].includes(llm.defaultProvider) ? llm.defaultProvider : 'ollama',
    ollama: null,
    ollamaLoading: false,
    pullJob: '',
    key: { anthropic: '', openai: '' },
    showKey: false,
    testing: false,
    test: null,
    personaId: '',
    chatPersonaId: '',
    approval: true,
    picked: null,
    finishing: false,
  };

  const qr = createQRCard({ showSteps: true, update });
  let qrMounted = false;
  const picker = createChatPicker({ update, onChange: (it) => { st.picked = it; } });
  let pickerLoaded = false;

  // ── Brain ──
  async function checkOllama() {
    st.ollamaLoading = true; update();
    try { st.ollama = await api.ollama.status({ quiet: true }); }
    catch (e) { st.ollama = { reachable: false, error: e.message, models: [] }; }
    st.ollamaLoading = false; update();
  }

  async function chooseOllamaModel(id) {
    await api.settings.put({ llm: { defaultProvider: 'ollama', ollamaModel: id } });
    await loadSettings();
    update();
  }

  async function pull(name) {
    try {
      const r = await api.ollama.pull(name);
      st.pullJob = r && r.jobId;
      update();
    } catch { /* toasted */ }
  }

  async function saveAndTest(provider) {
    const value = st.key[provider].trim();
    st.testing = true; st.test = null; update();
    try {
      if (value) {
        const secrets = await api.settings.secrets(provider === 'anthropic' ? { anthropicKey: value } : { openaiKey: value });
        store.set({ secrets });
        st.key[provider] = '';
      }
      st.test = await api.llm.test(provider);
      if (st.test && st.test.ok) {
        await api.settings.put({ llm: { defaultProvider: provider } });
        await loadSettings();
      }
    } catch (e) {
      st.test = { ok: false, error: e.message };
    } finally {
      st.testing = false; update();
    }
  }

  function brainReady() {
    if (st.tab === 'ollama') return !!(st.ollama && st.ollama.reachable && st.ollama.models && st.ollama.models.length);
    const s = store.state.secrets[st.tab];
    return !!(s && s.set);
  }

  function ollamaPanel() {
    const o = st.ollama;
    const job = st.pullJob ? store.state.pulls[st.pullJob] : null;
    if (job && job.done && !job.error && !st._pullHandled) {
      st._pullHandled = true;
      queueMicrotask(async () => { await checkOllama(); await chooseOllamaModel(job.name); toast(`${job.name} is ready`, { type: 'success' }); });
    }
    if (!o || st.ollamaLoading) {
      return html`<div class="row gap-12 muted" style="padding:18px 4px">${spinner()}<span>Looking for Ollama on this computer…</span></div>`;
    }
    if (!o.reachable) {
      return html`<div class="col gap-12">
        <div class="banner warn">${icon('warning')}<div><strong>Ollama isn't running on this computer.</strong>
          <div class="small mt-4">Ollama runs AI models privately on your computer — free, and nothing leaves it.</div></div></div>
        <ol class="scan-steps">
          <li><span>Download it from <a href="https://ollama.com/download" target="_blank" rel="noopener noreferrer">ollama.com/download</a> and open it.</span></li>
          <li><span>Come back here and press <b>Check again</b>.</span></li>
        </ol>
        <div class="row gap-8 row-wrap">
          <button class="btn btn-primary" @click=${checkOllama}>${icon('refresh')}Check again</button>
          <button class="btn btn-ghost" @click=${() => { st.tab = 'anthropic'; update(); }}>Use Claude instead</button>
        </div>
      </div>`;
    }
    const models = o.models || [];
    const current = (store.state.settings && store.state.settings.llm && store.state.settings.llm.ollamaModel) || '';
    return html`<div class="col gap-12">
      <div class="banner success">${icon('check')}<div><strong>Ollama is running</strong>${o.version ? html` <span class="faint">· v${o.version}</span>` : ''}. Replies are generated privately on this computer.</div></div>
      ${models.length ? html`
        <div class="field-label">Pick the model your personas use</div>
        <div class="row row-wrap gap-8">
          ${models.map((m) => html`<button class="chip chip-lg" aria-pressed=${String(m.id === current)} @click=${() => chooseOllamaModel(m.id)}>
            ${m.id === current ? icon('check') : icon('cpu')}${m.label || m.id}
            ${m.meta && m.meta.size ? html`<span class="faint">${fmtBytes(m.meta.size)}</span>` : ''}
          </button>`)}
        </div>` : ''}
      ${job && !job.done ? html`<div class="pull-card glass">
          ${progressRing(job.percent || 0, { size: 56, stroke: 5 })}
          <div class="grow"><div class="h3">Downloading ${job.name}</div>
            <div class="small muted">${job.status || 'Starting…'}${job.total ? html` · ${fmtBytes(job.completed)} of ${fmtBytes(job.total)}` : ''}</div></div>
        </div>`
      : job && job.error ? html`<div class="banner danger">${icon('warning')}<div><strong>Download failed.</strong> ${job.error}</div></div>`
      : !models.length ? html`<div class="pull-card glass">
          <span class="pav" style="--s:52px;background:var(--brand-grad)">${icon('download', 'ic-lg')}</span>
          <div class="grow"><div class="h3">No chat models yet</div><div class="small muted">Download Llama 3.1 (8B) — about 4.9 GB, a good all-rounder.</div></div>
          <button class="btn btn-primary" @click=${() => pull(RECOMMENDED_MODEL)}>${icon('download')}Download</button>
        </div>` : ''}
    </div>`;
  }

  function keyPanel(provider) {
    const sec = store.state.secrets[provider] || {};
    const name = provider === 'anthropic' ? 'Claude' : 'OpenAI';
    const where = provider === 'anthropic'
      ? html`Get one at <a href="https://console.anthropic.com/settings/keys" target="_blank" rel="noopener noreferrer">console.anthropic.com</a> → API keys.`
      : html`Get one at <a href="https://platform.openai.com/api-keys" target="_blank" rel="noopener noreferrer">platform.openai.com</a> → API keys.`;
    const t = st.test;
    return html`<div class="col gap-12">
      <div class="field">
        <label class="field-label" for="onb-key">${icon('key')}${name} API key</label>
        <div class="input-wrap">
          <input id="onb-key" class="input has-suffix mono" type=${st.showKey ? 'text' : 'password'} autocomplete="off" spellcheck="false"
            placeholder=${sec.set ? `Saved key ${sec.hint || ''} — paste a new one to replace` : (provider === 'anthropic' ? 'sk-ant-…' : 'sk-…')}
            .value=${st.key[provider]} @input=${(e) => { st.key[provider] = e.target.value; update(); }}
            @keydown=${(e) => { if (e.key === 'Enter') saveAndTest(provider); }}>
          <button class="btn btn-ghost btn-icon btn-sm input-suffix" type="button" aria-label=${st.showKey ? 'Hide key' : 'Show key'}
            @click=${() => { st.showKey = !st.showKey; update(); }}>${icon(st.showKey ? 'eye-off' : 'eye')}</button>
        </div>
        <div class="field-help">${where} Your key is stored only on this computer.</div>
      </div>
      <div class="row gap-10 row-wrap">
        <button class=${'btn btn-primary ' + (st.testing ? 'loading' : '')} ?disabled=${!st.key[provider].trim() && !sec.set}
          @click=${() => saveAndTest(provider)}>${icon('bolt')}${st.key[provider].trim() ? 'Save & test' : 'Test'}</button>
        ${sec.set ? html`<span class="chip chip-green">${icon('lock')}Key saved ${sec.hint || ''}</span>` : ''}
      </div>
      ${t ? (t.ok
        ? html`<div class="test-ok">
            <span class="tick-badge"><svg class="ic check-draw" viewBox="0 0 24 24"><path d="m5 12.5 4.5 4.5L19 7.5"/></svg></span>
            <div class="grow"><div class="h3">It works!</div><div class="small muted">${name} answered in ${fmtMs(t.latencyMs)}${t.model ? html` using <code>${t.model}</code>` : ''}</div>
              ${t.sample ? html`<div class="bubble mt-8" style="display:inline-block">${t.sample}</div>` : ''}</div>
          </div>`
        : html`<div class="banner danger">${icon('warning')}<div><strong>That didn't work.</strong> ${t.error || 'Check the key and try again.'}</div></div>`) : ''}
    </div>`;
  }

  function stepBrain() {
    return html`
      <div class="onb-hero">
        <div class="onb-badge">${icon('brain', 'ic-lg')}</div>
        <h2>Choose your persona's brain</h2>
        <p>This is the AI that writes the replies. Run it free on this computer, or use Claude or OpenAI with your own key.</p>
      </div>
      ${seg([
        { value: 'ollama', label: 'This computer (Ollama)', icon: 'cpu' },
        { value: 'anthropic', label: 'Claude', icon: 'sparkles' },
        { value: 'openai', label: 'OpenAI', icon: 'bolt' },
      ], st.tab, (v) => { st.tab = v; st.test = null; st.showKey = false; update(); if (v === 'ollama' && !st.ollama) checkOllama(); }, { cls: 'seg-lg seg-block', label: 'AI provider' })}
      <div class="onb-panel" data-key=${'tab-' + st.tab}>
        ${st.tab === 'ollama' ? ollamaPanel() : keyPanel(st.tab)}
      </div>`;
  }

  function stepWA() {
    const connected = store.state.wa && store.state.wa.state === 'connected';
    return html`
      <div class="onb-hero">
        <div class="onb-badge">${icon('phone', 'ic-lg')}</div>
        <h2>${connected ? 'WhatsApp is linked' : 'Link your WhatsApp'}</h2>
        <p>${connected ? 'Doppel can now see your chats and reply as the personas you choose.' : 'Doppel connects as a linked device — just like WhatsApp Web. Your phone stays in charge.'}</p>
      </div>
      ${qr.view()}`;
  }

  function stepPersona() {
    if (!st.personaId && store.state.personas.length) st.personaId = store.state.personas[0].id;
    return html`
      <div class="onb-hero">
        <div class="onb-badge">${icon('personas', 'ic-lg')}</div>
        <h2>Meet your personas</h2>
        <p>A persona is a character with a name, a vibe and a way of texting. Pick one to start — you can edit it or make more later.</p>
      </div>
      ${store.state.personasLoaded ? personaTiles(st.personaId, (id) => { st.personaId = id; update(); }, {
        onCreateAI: () => openAIBuilder({ stay: true, onSaved: async (p) => { await loadPersonas(); st.personaId = p.id; update(); } }),
      }) : html`<div class="center" style="padding:30px">${spinner('spinner-lg')}</div>`}`;
  }

  function stepChat() {
    if (!pickerLoaded) { pickerLoaded = true; queueMicrotask(() => picker.load(true)); }
    const connected = store.state.wa && store.state.wa.state === 'connected';
    const pid = st.chatPersonaId || st.personaId || (store.state.personas[0] && store.state.personas[0].id) || '';
    return html`
      <div class="onb-hero">
        <div class="onb-badge">${icon('chats', 'ic-lg')}</div>
        <h2>Pick a first chat</h2>
        <p>We suggest <b>You (message yourself)</b> — a safe place to try it out where nobody else sees the replies.</p>
      </div>
      ${!connected ? html`<div class="banner warn mb-12">${icon('warning')}<div>WhatsApp isn't linked yet, so your chats can't load. You can finish now and assign a chat later.</div></div>` : ''}
      <div class="onb-chat-grid">
        <div class="field">
          <label class="field-label">${icon('chats')}Chat</label>
          ${picker.view()}
        </div>
        <div class="col gap-16">
          <div class="field">
            <label class="field-label">${icon('personas')}Persona</label>
            ${personaPicker(pid, (id) => { st.chatPersonaId = id; update(); })}
          </div>
          <div class="field-row" style="border:0;padding:0">
            <div>
              <div class="field-label">${icon('shield')}Approve before sending</div>
              <div class="field-help">You'll check each reply before it goes out. Turn this off once you trust the persona.</div>
            </div>
            ${toggle(st.approval, (v) => { st.approval = v; update(); }, { label: 'Approve before sending' })}
          </div>
        </div>
      </div>`;
  }

  async function finish() {
    if (st.finishing) return;
    st.finishing = true; update();
    const pid = st.chatPersonaId || st.personaId;
    let assigned = false;
    try {
      if (st.picked && pid) { await assignChat(st.picked, pid, st.approval); assigned = true; }
      await api.settings.put({ onboardingCompleted: true });
      await loadSettings();
      ctx.navigate('/dashboard');
      const prefix = (store.state.settings && store.state.settings.behavior && store.state.settings.behavior.triggerPrefix) || '1';
      if (assigned) {
        toast(html`You're all set! From your phone, send <code>${prefix} hi</code> in that chat to test.`, { type: 'success', timeout: 12000 });
      } else {
        toast('Setup finished. Assign a chat whenever you are ready.', { type: 'success' });
      }
    } catch { /* toasted */ } finally {
      st.finishing = false; update();
    }
  }

  async function skip() {
    try {
      await api.settings.put({ onboardingCompleted: true });
      await loadSettings();
      ctx.navigate('/dashboard');
      toast('Setup skipped — everything is available from the sidebar and Settings.');
    } catch { /* toasted */ }
  }

  async function next() {
    if (st.step === 0 && brainReady()) {
      const s = store.state.settings && store.state.settings.llm;
      if (s && s.defaultProvider !== st.tab) {
        try { await api.settings.put({ llm: { defaultProvider: st.tab } }); await loadSettings(); } catch { /* toasted */ }
      }
    }
    if (st.step < STEPS.length - 1) { st.dir = 'right'; st.step++; enter(); update(); }
    else finish();
  }

  function back() {
    if (st.step > 0) { st.dir = 'left'; st.step--; enter(); update(); }
  }

  function enter() {
    if (STEPS[st.step].key === 'wa' && !qrMounted) { qrMounted = true; qr.mount(); }
    const host = document.getElementById('page');
    if (host) host.scrollTop = 0;
  }

  function continueLabel() {
    const k = STEPS[st.step].key;
    if (k === 'brain') return brainReady() ? 'Continue' : 'Skip for now';
    if (k === 'wa') return store.state.wa && store.state.wa.state === 'connected' ? 'Continue' : "I'll do this later";
    if (k === 'chat') return st.picked ? 'Finish setup' : 'Finish without a chat';
    return 'Continue';
  }

  function view() {
    const k = STEPS[st.step].key;
    return html`<div class="onb">
      <header class="onb-top">
        <div class="row gap-10">
          <img src="/assets/favicon.svg" alt="" width="34" height="34" style="border-radius:10px">
          <strong class="h3">Doppel</strong>
        </div>
        <ol class="onb-dots" aria-label="Setup progress">
          ${STEPS.map((s, i) => html`<li class=${(i === st.step ? 'on ' : '') + (i < st.step ? 'done' : '')} aria-current=${i === st.step ? 'step' : 'false'}>
            <button type="button" ?disabled=${i > st.step} @click=${() => { if (i < st.step) { st.dir = 'left'; st.step = i; enter(); update(); } }}>
              <span class="pip">${i < st.step ? icon('check') : i + 1}</span><span class="pl">${s.label}</span>
            </button></li>`)}
        </ol>
        <button class="btn btn-ghost btn-sm" @click=${skip}>Skip setup</button>
      </header>
      <div class="onb-stage">
        <section class=${'onb-card glass ' + (st.dir === 'left' ? 'in-left' : 'in-right')} data-key=${'step-' + k}>
          ${k === 'brain' ? stepBrain() : k === 'wa' ? stepWA() : k === 'persona' ? stepPersona() : stepChat()}
          <footer class="onb-nav">
            ${st.step > 0 ? html`<button class="btn btn-ghost" @click=${back}>${icon('chevron-left')}Back</button>` : html`<span></span>`}
            <span class="faint small">Step ${st.step + 1} of ${STEPS.length}</span>
            <button class=${'btn btn-lg ' + (k === 'chat' ? 'btn-brand ' : 'btn-primary ') + (st.finishing ? 'loading' : '')} @click=${next}>
              ${continueLabel()}${k === 'chat' ? icon('check') : icon('arrow-right')}
            </button>
          </footer>
        </section>
      </div>
    </div>`;
  }

  return {
    bare: true,
    title: 'Welcome',
    view,
    mount() { checkOllama(); },
    unmount() { qr.destroy(); },
  };
}
