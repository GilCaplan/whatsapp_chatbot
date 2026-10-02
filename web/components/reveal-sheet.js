// reveal-sheet.js — "Reveal…": shows the exact message that will be sent
// ("it was an AI persona all along"), lets you tweak it, then sends it as
// your own message and pauses the persona in that chat.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadChats } from '../store.js';
import { openSheet, toast } from '../ui.js';
import { rowsFor } from '../util.js';
import { helpTip } from './help-tip.js';
import { renderReveal } from './handoff-meta.js';

export function openRevealSheet(c, p) {
  const s = store.state.settings || {};
  const tpl = (s.safety && s.safety.reveal && s.safety.reveal.template) || '';
  const me = (store.state.wa && store.state.wa.me && store.state.wa.me.pushName) || '';
  const name = p ? p.name : 'the persona';
  const st = { text: renderReveal(tpl, p && p.name, me), busy: false, again: !!c.revealedAt };

  async function send(ctl) {
    const text = st.text.trim();
    if (!text || st.busy) return;
    st.busy = true; ctl.update();
    try {
      await api.chats.reveal(c.key, { text, force: st.again });
      toast(`Revealed ${name} to ${c.name || 'this chat'} and paused it`, { type: 'success' });
      ctl.close(true);
      loadChats().catch(() => {});
    } catch (e) {
      if (e && e.code === 'already_revealed') { st.again = true; }
      st.busy = false; ctl.update();
    }
  }

  return openSheet((ctl) => html`
    <div class="sheet-head">
      <div class="sheet-icon" style="background:linear-gradient(135deg,#a78bfa,#7c3aed)">${icon('eye')}</div>
      <div class="grow">
        <h2>Reveal ${name} to ${c.name || 'this chat'}? ${helpTip('reveal')}</h2>
        <p>This is the exact message that will be sent from your WhatsApp. You can change it here.</p>
      </div>
    </div>
    ${st.again ? html`<div class="banner warn mb-16">${icon('warning')}<div>You already revealed ${name} here <rel-time datetime=${c.revealedAt}></rel-time>. Sending again posts the message a second time.</div></div>` : ''}
    <div class="reveal-wall">
      <div class="bubble-row me">
        <textarea class="textarea" aria-label="Reveal message" rows=${rowsFor(st.text, 3, 9)} maxlength="2000" .value=${st.text}
          @input=${(e) => { st.text = e.target.value; ctl.update(); }}></textarea>
      </div>
    </div>
    <div class="reveal-steps">
      <div>${icon('send')}<span>Sent right away as your own message: no typing delay, no typos.</span></div>
      <div>${icon('pause')}<span>${name} stops replying in this chat. Replies waiting for approval are dropped.</span></div>
      <div>${icon('edit')}<span>Change the default wording in Settings › Safety.</span></div>
    </div>
    <div class="sheet-actions">
      <button class="btn btn-ghost" @click=${() => ctl.close(false)}>Cancel</button>
      <button class=${'btn btn-primary ' + (st.busy ? 'loading' : '')} ?disabled=${!st.text.trim() || st.busy} @click=${() => send(ctl)}>
        ${icon('eye')}${st.again ? 'Send again' : 'Send and pause'}</button>
    </div>`, { label: `Reveal ${name}` });
}
