// settings/safety.js — Settings › Safety: hand-off (which topics pause the
// persona and ask you to take over) and the reveal message.
// Wave 3 owner: Engineer B. h = { s(), save(partial, section), sectionHead(id, title, sub, color), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { store } from '../../store.js';
import { toggle, fieldRow } from '../../ui.js';
import { debounce, rowsFor } from '../../util.js';
import { helpTip } from '../help-tip.js';
import { HANDOFF_CATS, catGlyph, renderReveal } from '../handoff-meta.js';

const MAX_TEMPLATE = 600;
const DEFAULT_TEMPLATE = "Quick confession: for a while now you've been chatting with {persona}, an AI persona I set up. It was me behind it, and I'm taking over from here. Hope it was fun, tell me what you thought!";

// The reveal template while you type (saved after a pause).
let draft = null;
let saveDraft = null;

export function safetySection(h) {
  const s = h.s();
  const ho = (s.safety && s.safety.handoff) || {};
  const tpl = (s.safety && s.safety.reveal && s.safety.reveal.template) || '';
  const saveHandoff = (handoff) => h.save({ safety: { handoff } }, 'safety');
  if (!saveDraft) {
    saveDraft = debounce(async (value) => {
      if (value.trim() && value.length <= MAX_TEMPLATE && await h.save({ safety: { reveal: { template: value } } }, 'safety') && draft === value) {
        draft = null; h.update();
      }
    }, 700);
  }
  const text = draft != null ? draft : tpl;
  const on = HANDOFF_CATS.filter((c) => ho[c.id]).length;
  const persona = (store.state.personas[0] && store.state.personas[0].name) || 'Leo';
  const me = (store.state.wa && store.state.wa.me && store.state.wa.me.pushName) || '';
  return html`<section class="card section" id="sec-safety" data-section="safety">
    ${h.sectionHead('safety', 'Safety', 'When a message is about money, health, meeting up or asks if it is a bot, the persona pauses and lets you take over.', 'linear-gradient(135deg,#ef4444,#a855f7)')}
    ${fieldRow({
      label: html`Hand over sensitive messages to me ${helpTip('handoff')}`,
      help: 'The persona goes quiet in that chat, keeps reading, and Doppel shows "Needs you" until you tap Resume.',
      control: toggle(ho.enabled, (v) => saveHandoff({ enabled: v }), { label: 'Hand-off' }),
    })}
    ${ho.enabled ? html`<div class="field-row field-row-stack">
      <div><div class="field-label">Topics that pause the persona <span class="chip chip-sm">${on} of ${HANDOFF_CATS.length}</span></div>
        <div class="field-help">Tap a topic to switch it on or off. English and Hebrew are understood.</div></div>
      <div class="cat-grid">${HANDOFF_CATS.map((c) => html`<button type="button" class="cat-tile" aria-pressed=${String(!!ho[c.id])}
          @click=${() => saveHandoff({ [c.id]: !ho[c.id] })}>
        <span class="cat-ic">${catGlyph(c.id)}</span>
        <span class="grow"><span class="cat-name">${c.label}</span><span class="cat-line">${c.line}</span></span>
        <span class="cat-state">${ho[c.id] ? 'On' : 'Off'}</span>
      </button>`)}</div>
    </div>
    ${fieldRow({
      label: 'Ask the AI when unsure',
      help: 'Some words are only sometimes serious ("I owe you one", "meet you at the gym lol"). A quick private check with your AI decides, so jokes don\'t pause the chat. Off: only clear-cut messages pause it.',
      control: toggle(ho.aiCheck, (v) => saveHandoff({ aiCheck: v }), { label: 'Ask the AI when unsure' }),
    })}` : ''}

    <div class="field-row field-row-stack reveal-tpl">
      <div><div class="field-label">${icon('eye', 'ic-sm')}Reveal message ${helpTip('reveal')}</div>
        <div class="field-help">Sent from your WhatsApp when you tap Reveal… in a chat; the persona then pauses there. You can still change it before it goes.</div></div>
      <textarea class="textarea" aria-label="Reveal message" rows=${rowsFor(text, 3, 8)} maxlength=${MAX_TEMPLATE} .value=${text}
        @input=${(e) => { draft = e.target.value; h.update(); saveDraft(draft); }}></textarea>
      <div class="reveal-tpl-foot">
        <span><code>{persona}</code> becomes the persona's name, <code>{me}</code> your WhatsApp name.</span>
        <span class="grow"></span>
        <span class=${text.length > MAX_TEMPLATE - 40 ? 'danger-text' : ''}>${text.length}/${MAX_TEMPLATE}</span>
        ${text !== DEFAULT_TEMPLATE ? html`<button class="link-btn small" @click=${() => { draft = DEFAULT_TEMPLATE; h.update(); saveDraft(draft); }}>Use the default</button>` : ''}
      </div>
      <div class="reveal-preview" aria-label="Preview">
        <div class="bubble-row me"><div class="bubble">${renderReveal(text, persona, me) || html`<span class="muted">Write a message first.</span>`}</div></div>
        <div class="tiny faint mt-4">Preview with ${persona}${me ? ` and ${me}` : ''}.</div>
      </div>
    </div>
  </section>`;
}
