// ai-builder.js — "Build with AI": one-line description (+ optional sample
// messages) → POST /api/personas/draft → open the filled editor (or save in
// place when used inside onboarding / the assign sheet).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadPersonas } from '../store.js';
import { openSheet, toast } from '../ui.js';
import { personaAvatar } from './avatar.js';
import { navigate } from '../router.js';
import { rowsFor } from '../util.js';

const LINES = [
  'Interviewing your doppelgänger…',
  'Teaching them your inside jokes…',
  'Picking their profile picture…',
  'Practising their texting voice…',
  'Deciding how often they say “lol”…',
  'Giving them strong opinions about pizza…',
  'Writing a tiny backstory…',
  'Rehearsing witty comebacks…',
  'Almost there — local models take a minute…',
];

const IDEAS = [
  'My sarcastic older brother who works in tech and loves football',
  'A warm grandma who sends voice-note energy in text and too many hearts',
  'A chill surfer from Tel Aviv, replies in Hebrew slang, very laid back',
  'An over-organised PA who always pushes to lock in a date',
];

/** openAIBuilder({ onSaved(persona), stay }) */
export function openAIBuilder({ onSaved, stay = false } = {}) {
  const st = { description: '', samples: '', showSamples: false, provider: '', phase: 'form', error: '', draft: null, started: 0, line: 0, saving: false };
  let ticker = null;
  let ctlAbort = null;

  const stopTicker = () => { clearInterval(ticker); ticker = null; };

  async function draft(ctl) {
    if (!st.description.trim()) return;
    st.phase = 'loading'; st.error = ''; st.started = Date.now(); st.line = 0;
    ctl.update();
    ticker = setInterval(() => ctl.update(), 1000);
    ctlAbort = new AbortController();
    try {
      const body = { description: st.description.trim(), samples: st.samples.trim() };
      if (st.provider) body.provider = st.provider;
      const res = await api.personas.draft(body, { quiet: true, signal: ctlAbort.signal, timeout: 300000 });
      const p = (res && res.persona) || null;
      if (!p) throw new Error('The AI did not return a persona. Try again with a bit more detail.');
      st.draft = p;
      st.phase = stay ? 'preview' : 'done';
      if (!stay) {
        store.set({ draftPersona: p });
        ctl.close();
        navigate('/persona/new?draft=1');
        toast('Draft ready — tweak anything you like, then Save.', { type: 'success' });
        return;
      }
    } catch (e) {
      if (e.name === 'AbortError') { st.phase = 'form'; }
      else { st.phase = 'form'; st.error = e.message || 'Drafting failed'; }
    } finally {
      stopTicker();
      if (!ctl.closed) ctl.update();
    }
  }

  async function saveDraft(ctl) {
    st.saving = true; ctl.update();
    try {
      const p = await api.personas.create({ ...st.draft, id: '' });
      await loadPersonas();
      toast(html`<b>${p.name}</b> joined your personas`, { type: 'success' });
      ctl.close();
      if (onSaved) onSaved(p);
    } catch { /* toasted */ } finally { st.saving = false; if (!ctl.closed) ctl.update(); }
  }

  const sheet = openSheet((ctl) => {
    if (st.phase === 'loading') {
      const secs = Math.floor((Date.now() - st.started) / 1000);
      st.line = Math.floor(secs / 3.5) % LINES.length;
      return html`
        <div class="sheet-head">
          <div class="sheet-icon builder-spark">${icon('sparkles')}</div>
          <div class="grow"><h2>Drafting your persona</h2><p class="builder-line" data-key=${'l' + st.line}>${LINES[st.line]}</p></div>
        </div>
        <div class="builder-skeleton glass">
          <div class="row gap-12"><div class="skeleton" style="width:64px;height:64px;border-radius:20px"></div>
            <div class="grow"><div class="skeleton sk-line w40" style="height:16px"></div><div class="skeleton sk-line w80"></div></div></div>
          <div class="skeleton sk-line w90 mt-16"></div><div class="skeleton sk-line w80"></div><div class="skeleton sk-line w60"></div>
          <div class="row gap-8 mt-16"><div class="skeleton" style="width:70px;height:26px;border-radius:99px"></div><div class="skeleton" style="width:90px;height:26px;border-radius:99px"></div><div class="skeleton" style="width:60px;height:26px;border-radius:99px"></div></div>
        </div>
        <div class="sheet-actions">
          <span class="faint small grow" style="align-self:center">${secs}s · local models can take up to a minute</span>
          <button class="btn btn-ghost" @click=${() => { if (ctlAbort) ctlAbort.abort(); }}>Cancel</button>
        </div>`;
    }

    if (st.phase === 'preview' && st.draft) {
      const p = st.draft;
      return html`
        <div class="sheet-head">
          ${personaAvatar(p, 56)}
          <div class="grow"><h2>${p.name || 'New persona'}</h2><p>${p.tagline || ''}</p></div>
        </div>
        <div class="col gap-12">
          ${p.bio ? html`<div><div class="field-label">About</div><p class="muted mt-4">${p.bio}</p></div>` : ''}
          ${p.personality ? html`<div><div class="field-label">Personality</div><p class="muted mt-4">${p.personality}</p></div>` : ''}
          ${p.style ? html`<div><div class="field-label">How they text</div><p class="muted mt-4">${p.style}</p></div>` : ''}
        </div>
        <div class="sheet-actions">
          <button class="btn btn-ghost" @click=${() => { st.phase = 'form'; ctl.update(); }}>${icon('arrow-left')}Back</button>
          <span class="grow"></span>
          <button class="btn btn-glass" @click=${() => { store.set({ draftPersona: p }); ctl.close(); navigate('/persona/new?draft=1'); }}>${icon('edit')}Edit details</button>
          <button class=${'btn btn-primary ' + (st.saving ? 'loading' : '')} @click=${() => saveDraft(ctl)}>${icon('check')}Save persona</button>
        </div>`;
    }

    return html`
      <div class="sheet-head">
        <div class="sheet-icon builder-spark">${icon('sparkles')}</div>
        <div class="grow">
          <h2>Build a persona with AI</h2>
          <p>Describe who they are in a sentence. You can tweak everything afterwards.</p>
        </div>
        <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
      </div>
      ${st.error ? html`<div class="banner danger mb-16">${icon('warning')}<div><strong>Couldn't draft that.</strong> ${st.error}</div></div>` : ''}
      <div class="field">
        <label class="field-label" for="ai-desc">Who are they?</label>
        <textarea id="ai-desc" class="textarea" rows=${rowsFor(st.description, 2, 5)} autofocus
          placeholder="e.g. My sarcastic older brother who works in tech and loves football"
          @input=${(e) => { st.description = e.target.value; ctl.update(); }}
          @keydown=${(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); draft(ctl); } }}>${st.description}</textarea>
        <div class="row row-wrap gap-6 mt-4">
          ${IDEAS.map((idea) => html`<button type="button" class="chip chip-sm" @click=${() => { st.description = idea; ctl.update(); }}>${idea.length > 42 ? idea.slice(0, 40) + '…' : idea}</button>`)}
        </div>
      </div>
      <div class="field">
        ${st.showSamples ? html`
          <label class="field-label" for="ai-samples">Sample messages <span class="opt">· optional</span></label>
          <textarea id="ai-samples" class="textarea" rows="5" placeholder=${'Paste a few real messages they wrote, one per line:\nhaha no way\nu coming tonight or what'}
            @input=${(e) => { st.samples = e.target.value; }}>${st.samples}</textarea>
          <div class="field-help">Real examples help the AI copy their spelling, slang and emoji habits. They're only used for this draft.</div>`
          : html`<button type="button" class="link-btn" @click=${() => { st.showSamples = true; ctl.update(); }}>${'+ Add sample messages (makes it more realistic)'}</button>`}
      </div>
      <div class="sheet-actions">
        <label class="row gap-8 small muted" style="margin-right:auto">
          Brain
          <select class="select input-sm" style="width:auto" @change=${(e) => { st.provider = e.target.value; }}>
            <option value="" ?selected=${!st.provider}>Default</option>
            <option value="ollama" ?selected=${st.provider === 'ollama'}>Ollama (this Mac)</option>
            <option value="anthropic" ?selected=${st.provider === 'anthropic'} ?disabled=${!store.state.secrets.anthropic || !store.state.secrets.anthropic.set}>Claude</option>
            <option value="openai" ?selected=${st.provider === 'openai'} ?disabled=${!store.state.secrets.openai || !store.state.secrets.openai.set}>OpenAI</option>
          </select>
        </label>
        <button class="btn btn-ghost" @click=${() => ctl.close()}>Cancel</button>
        <button class="btn btn-brand" ?disabled=${!st.description.trim()} @click=${() => draft(ctl)}>${icon('sparkles')}Draft persona</button>
      </div>`;
  }, {
    size: 'wide',
    label: 'Build a persona with AI',
    onClose: () => { stopTicker(); if (ctlAbort) ctlAbort.abort(); },
  });
  return sheet;
}
