// components/context-draft-sheet.js — "Describe it": you write how a
// context system should work, the AI draws the flowchart from the building
// blocks (POST /api/context/draft), code lays it out and reads the settings
// off it. Preview, rename, then save it as one of your systems.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { openSheet, seg, toast } from '../ui.js';
import './flowchart.js';

const EXAMPLES = [
  'Only answer when someone says its name or asks the group something, check every reply in code, and keep the prompt tiny.',
  'Ask the AI whether to join in, plan the goal every three replies, and let an AI judge look at every reply before it goes out.',
];

export function openDraftSheet({ blocks, onSaved }) {
  const st = { text: '', base: 'lean', busy: false, draft: null, changes: [], name: '', saving: false, key: 0 };
  let ctl;
  const update = () => ctl && ctl.update();

  async function draw() {
    st.busy = true; update();
    try {
      const r = await api.context.draft({ text: st.text, base: st.base });
      st.draft = r.system; st.changes = r.changes || []; st.name = r.system.name; st.key++;
    } catch { /* toasted */ }
    st.busy = false; update();
  }

  async function save() {
    st.saving = true; update();
    try {
      const saved = await api.context.create({ ...st.draft, name: st.name.trim() || st.draft.name });
      toast(`Saved “${saved.name}”`, { type: 'success' });
      ctl.close();
      if (onSaved) onSaved(saved);
    } catch { /* toasted */ }
    st.saving = false; update();
  }

  ctl = openSheet((c) => html`
    <div class="sheet-head">
      <div class="sheet-icon">${icon('wand')}</div>
      <div class="grow"><h2>Describe a context system</h2><p>Say in your own words when it should think and what it should read. The AI draws it from the building blocks; you can change everything after.</p></div>
      <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => c.close()}>${icon('x')}</button>
    </div>
    <div class="cs-draft">
      <label class="field"><span class="field-label">How should it work?</span>
        <textarea class="input" rows="4" maxlength="2000" placeholder=${EXAMPLES[0]} .value=${st.text}
          @input=${(e) => { st.text = e.target.value; update(); }}></textarea></label>
      <div class="row row-wrap gap-8 small">${EXAMPLES.map((ex) => html`<button type="button" class="chip" @click=${() => { st.text = ex; update(); }}>${ex.slice(0, 48)}…</button>`)}</div>
      <div class="field"><span class="field-label">Start the settings from</span>
        ${seg([{ value: 'classic', label: 'Classic' }, { value: 'lean', label: 'Lean' }, { value: 'fast', label: 'Fast' }], st.base, (v) => { st.base = v; update(); }, { label: 'Start from' })}
        <div class="field-help">Anything the chart doesn't say keeps this system's setting.</div></div>
      <div class="row gap-8"><button class=${'btn btn-primary ' + (st.busy ? 'loading' : '')} ?disabled=${!st.text.trim() || st.busy} @click=${draw}>${icon('sparkles')}${st.draft ? 'Draw again' : 'Draw it'}</button>
        ${st.busy ? html`<span class="faint small">This can take a minute on a local model.</span>` : ''}</div>
      ${st.draft ? html`
        <label class="field"><span class="field-label">Name</span><input class="input" maxlength="60" .value=${st.name} @input=${(e) => { st.name = e.target.value; update(); }}></label>
        ${st.draft.description ? html`<p class="muted small">${st.draft.description}</p>` : ''}
        <cs-flowchart class="cs-fc-preview" .data=${{ chart: st.draft.chart, blocks, locked: true, selected: null, key: st.key }}></cs-flowchart>
        ${st.changes.length ? html`<div class="field-help">Settings read off the chart: ${st.changes.join(' · ')}</div>` : ''}` : ''}
    </div>
    ${st.draft ? html`<div class="sheet-actions">
      <button class="btn btn-ghost" @click=${() => c.close()}>Cancel</button>
      <button class=${'btn btn-primary ' + (st.saving ? 'loading' : '')} ?disabled=${st.saving} @click=${save}>${icon('check')}Save as my system</button>
    </div>` : ''}`, { size: 'wide', label: 'Describe a context system', noAutofocus: true });
  return ctl;
}
