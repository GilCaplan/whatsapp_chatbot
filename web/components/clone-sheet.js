// clone-sheet.js — "Clone yourself" (wave 3): a persona that texts like you,
// written from a private sample of messages you typed yourself. Collecting
// samples needs your consent (Settings › Data); the draft opens in the
// persona editor to review before anything is saved.
//
//   openCloneSheet()

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadSettings } from '../store.js';
import { openSheet, toast, progressRing, spinner } from '../ui.js';
import { navigate } from '../router.js';
import { relTime, rowsFor } from '../util.js';
import { providerLabel } from './status.js';
import { helpTip } from './help-tip.js';

const MIN = 20;
const LINES = [
  'Reading how you text…',
  'Counting your "haha"s…',
  'Noticing your favourite words…',
  'Checking how you use capital letters…',
  'Working out your emoji habits…',
  'Writing your persona…',
  'Almost there — local models take a minute…',
];

function cloneArt() {
  return html`<svg viewBox="0 0 120 72" width="120" height="72" aria-hidden="true">
    <defs>
      <linearGradient id="clg1" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#34d399"></stop><stop offset="1" stop-color="#0ea5e9"></stop></linearGradient>
      <linearGradient id="clg2" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#a78bfa"></stop><stop offset="1" stop-color="#f472b6"></stop></linearGradient>
    </defs>
    <circle cx="36" cy="26" r="13" fill="url(#clg1)"></circle>
    <path d="M14 66c2-14 11-22 22-22s20 8 22 22z" fill="url(#clg1)"></path>
    <circle cx="84" cy="26" r="13" fill="url(#clg2)" opacity="0.9"></circle>
    <path d="M62 66c2-14 11-22 22-22s20 8 22 22z" fill="url(#clg2)" opacity="0.9"></path>
    <path d="M52 30h16m-5-5 5 5-5 5" stroke="currentColor" stroke-width="2.5" fill="none" stroke-linecap="round" stroke-linejoin="round" opacity="0.55"></path>
  </svg>`;
}

export function openCloneSheet() {
  const s0 = store.state;
  const st = {
    phase: 'loading', data: null, error: '',
    name: (s0.wa && s0.wa.me && (s0.wa.me.pushName || s0.wa.me.name)) || '',
    extra: '', provider: '', showSamples: false, line: 0, started: 0, enabling: false,
  };
  let ticker = null;
  let abort = null;
  const stop = () => { clearInterval(ticker); ticker = null; };

  async function load(ctl) {
    try {
      st.data = await api.clone.samples({ quiet: true });
      st.phase = 'ready';
    } catch (e) {
      st.phase = 'ready';
      st.error = (e && e.message) || 'Could not load your samples';
    }
    ctl.update();
  }

  async function enable(ctl) {
    st.enabling = true; ctl.update();
    try {
      await api.settings.put({ clone: { collectSamples: true } });
      loadSettings().catch(() => {});
      toast("Doppel will now keep a sample of the messages you type", { type: 'success' });
      await load(ctl);
    } catch { /* toasted */ }
    st.enabling = false; ctl.update();
  }

  async function clone(ctl) {
    st.phase = 'drafting'; st.error = ''; st.started = Date.now(); st.line = 0;
    ctl.update();
    ticker = setInterval(() => { st.line++; ctl.update(); }, 2500);
    abort = new AbortController();
    try {
      const body = { name: st.name.trim(), extra: st.extra.trim() };
      if (st.provider) body.provider = st.provider;
      const res = await api.clone.draft(body, { quiet: true, signal: abort.signal });
      stop();
      if (!res || !res.persona) throw new Error('The AI did not return a persona. Try again.');
      store.set({ draftPersona: res.persona });
      ctl.close();
      navigate('/persona/new?draft=1');
      toast('Your clone is ready — check it over, then Save.', { type: 'success', sub: `Written from ${res.sampleCount} of your messages.` });
    } catch (e) {
      stop();
      if (e && e.name === 'AbortError') { st.phase = 'ready'; ctl.update(); return; }
      st.phase = 'ready';
      st.error = (e && e.message) || 'Cloning failed';
      ctl.update();
    }
  }

  const sheet = openSheet((ctl) => {
    if (st.phase === 'loading') {
      return html`<div class="center" style="min-height:220px">${spinner('spinner-lg')}</div>`;
    }
    const d = st.data || { enabled: false, count: 0, preview: [] };
    const count = d.count || 0;
    const settings = store.state.settings || {};
    const def = settings.llm ? settings.llm.defaultProvider : 'ollama';
    const prov = st.provider || def || 'ollama';
    const cloud = prov !== 'ollama';
    const head = html`<div class="sheet-head">
        <div class="sheet-icon clone-icon">${icon('user')}</div>
        <div class="grow">
          <h2>Clone yourself ${helpTip('clone')}</h2>
          <p>A persona that texts like you — your length, slang, emoji and habits — written from messages you typed yourself.</p>
        </div>
        <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
      </div>`;

    if (st.phase === 'drafting') {
      const secs = Math.round((Date.now() - st.started) / 1000);
      return html`${head}
        <div class="clone-busy">
          <div class="clone-art">${cloneArt()}</div>
          <div class="h3">${LINES[Math.min(st.line, LINES.length - 1)]}</div>
          <div class="small muted">${secs}s · reading up to 150 of your messages</div>
        </div>
        <div class="sheet-actions"><span class="grow"></span><button class="btn btn-ghost" @click=${() => { if (abort) abort.abort(); }}>Cancel</button></div>`;
    }

    if (!d.enabled && count === 0) {
      return html`${head}
        <div class="clone-intro">
          <div class="clone-art">${cloneArt()}</div>
          <ol class="clone-steps">
            <li><b>Turn on a private sample.</b> Doppel keeps a copy of messages you type yourself on WhatsApp — links, e-mail addresses and phone numbers removed, at most the last 500.</li>
            <li><b>Chat as usual.</b> After about ${MIN} messages there is enough to learn from.</li>
            <li><b>Clone.</b> The AI writes a persona that texts like you. You review and edit it before it's saved.</li>
          </ol>
          <div class="banner info small">${icon('lock')}<div>The sample stays on this computer and is only used when you tap Clone. Delete it any time in Settings › Data.</div></div>
        </div>
        <div class="sheet-actions">
          <button class="btn btn-ghost" @click=${() => ctl.close()}>Not now</button>
          <button class=${'btn btn-primary ' + (st.enabling ? 'loading' : '')} ?disabled=${st.enabling} @click=${() => enable(ctl)}>${icon('check')}Start keeping a sample</button>
        </div>`;
    }

    const preview = (d.preview || []).slice(-8);
    const samplesBox = html`<div class="clone-samples">
      <button type="button" class="disclosure" aria-expanded=${String(st.showSamples)} @click=${() => { st.showSamples = !st.showSamples; ctl.update(); }}>
        ${icon('quote', 'ic-sm')}<span>What it learns from</span><span class="faint small">${count} message${count === 1 ? '' : 's'}${d.since ? ` since ${relTime(d.since)}` : ''}</span>${icon(st.showSamples ? 'chevron-up' : 'chevron-down', 'ic-sm')}
      </button>
      ${st.showSamples ? html`<div class="chat-wall clone-wall">${preview.map((t) => html`<div class="bubble-row me"><div class="bubble">${t}</div></div>`)}</div>
        <div class="tiny faint">The latest few. Links and numbers were removed before saving.</div>` : ''}
    </div>`;

    if (count < MIN) {
      return html`${head}
        <div class="clone-progress">
          ${progressRing((count / MIN) * 100, { size: 64, stroke: 6, label: false })}
          <div class="grow">
            <div class="h3">${count} of ${MIN} messages</div>
            <div class="small muted">${d.enabled ? 'Keep chatting on your phone as usual — Doppel adds the messages you type. Come back in a while.' : 'Sample collection is off. Turn it back on to keep collecting.'}</div>
          </div>
        </div>
        ${count ? samplesBox : ''}
        <div class="sheet-actions">
          <button class="btn btn-ghost" @click=${() => ctl.close()}>Close</button>
          ${!d.enabled ? html`<button class=${'btn btn-primary ' + (st.enabling ? 'loading' : '')} @click=${() => enable(ctl)}>${icon('check')}Keep collecting</button>` : ''}
        </div>`;
    }

    return html`${head}
      ${st.error ? html`<div class="banner danger mb-16">${icon('warning')}<div><strong>Couldn't clone you.</strong> ${st.error}</div></div>` : ''}
      <div class="two-col">
        <div class="field"><label class="field-label" for="cl-name">Name</label>
          <input id="cl-name" class="input" maxlength="60" placeholder="What friends call you" .value=${st.name} @input=${(e) => { st.name = e.target.value; ctl.update(); }}>
          <div class="field-help">The persona's name.</div></div>
        <div class="field"><label class="field-label" for="cl-brain">Brain</label>
          <select id="cl-brain" class="select" @change=${(e) => { st.provider = e.target.value; ctl.update(); }}>
            <option value="" ?selected=${!st.provider}>Default (${providerLabel(def || 'ollama')})</option>
            <option value="ollama" ?selected=${st.provider === 'ollama'}>Ollama — on this computer</option>
            <option value="anthropic" ?selected=${st.provider === 'anthropic'} ?disabled=${!store.state.secrets.anthropic || !store.state.secrets.anthropic.set}>Claude</option>
            <option value="openai" ?selected=${st.provider === 'openai'} ?disabled=${!store.state.secrets.openai || !store.state.secrets.openai.set}>OpenAI</option>
          </select></div>
      </div>
      <div class="field mt-12"><label class="field-label" for="cl-extra">Anything else about you? <span class="opt">· optional</span></label>
        <textarea id="cl-extra" class="textarea" rows=${rowsFor(st.extra, 2, 5)} maxlength="600" placeholder="e.g. 29, product designer in Tel Aviv, into climbing and bad reality TV"
          @input=${(e) => { st.extra = e.target.value; }}>${st.extra}</textarea>
        <div class="field-help">Helps fill in the background. Your texting style comes from the samples.</div></div>
      ${samplesBox}
      ${cloud ? html`<div class="banner warn small mt-8">${icon('lock')}<div><b>${providerLabel(prov)}</b> will read your ${Math.min(count, 150)} sample messages to write the persona. Pick Ollama to keep them on this computer.</div></div>`
        : html`<div class="tiny faint mt-8 row gap-6">${icon('lock', 'ic-sm')}<span>Runs on this computer with Ollama — your messages don't leave it.</span></div>`}
      <div class="sheet-actions">
        <button class="btn btn-ghost" @click=${() => ctl.close()}>Cancel</button>
        <button class="btn btn-brand" ?disabled=${!st.name.trim()} @click=${() => clone(ctl)}>${icon('sparkles')}Clone me</button>
      </div>`;
  }, {
    size: 'wide',
    label: 'Clone yourself',
    onClose: () => { stop(); if (abort) abort.abort(); },
  });
  load(sheet);
  return sheet;
}
