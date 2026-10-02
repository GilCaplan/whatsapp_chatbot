// assign-sheet.js — "Assign a persona" sheet. With a chat → pick persona;
// without → pick chat first (search) then persona.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadChats } from '../store.js';
import { openSheet, toggle, toast } from '../ui.js';
import { chatAvatar } from './avatar.js';
import { personaTiles } from './persona-picker.js';
import { createChatPicker, chatSubtitle } from './chat-picker.js';
import { openAIBuilder } from './ai-builder.js';
import { navigate } from '../router.js';

/** Assign (or re-assign) a chat. Handles 409 already_assigned by patching. */
export async function assignChat(item, personaId, approvalMode) {
  try {
    const res = await api.chats.create({ jid: item.jid, personaId, approvalMode, enabled: true }, { quiet: true });
    await loadChats();
    return res;
  } catch (e) {
    if (e.code === 'already_assigned' || e.status === 409) {
      const key = (e.data && e.data.key) || item.key;
      const res = await api.chats.patch(key, { personaId, approvalMode, enabled: true });
      await loadChats();
      return res;
    }
    toast(e.message, { type: 'error' });
    throw e;
  }
}

export function openAssignSheet({ item = null, personaId = '', openAfter = true } = {}) {
  const st = {
    item,
    personaId: personaId || (store.state.personas[0] && store.state.personas[0].id) || '',
    approval: true,
    saving: false,
  };
  let picker = null;

  const sheet = openSheet((ctl) => {
    if (!st.item && !picker) {
      picker = createChatPicker({ update: () => ctl.update(), onChange: (it) => { st.picked = it; }, autoSelectSelf: true });
      queueMicrotask(() => picker.load(true));
    }
    const target = st.item || st.picked;
    const save = async () => {
      if (!target || !st.personaId) return;
      st.saving = true; ctl.update();
      try {
        const res = await assignChat(target, st.personaId, st.approval);
        const p = store.state.personas.find((x) => x.id === st.personaId);
        toast(html`<b>${p ? p.name : 'Persona'}</b> is now handling <b>${target.isSelf ? 'your own chat' : target.name}</b>`, { type: 'success' });
        ctl.close();
        if (openAfter && res && res.key) navigate(`/chats/${encodeURIComponent(res.key)}`);
      } catch { /* toasted */ } finally { st.saving = false; if (!ctl.closed) ctl.update(); }
    };
    return html`
      <div class="sheet-head">
        ${st.item ? chatAvatar({ jid: st.item.jid, name: st.item.isSelf ? 'You' : st.item.name, kind: st.item.kind }, 48)
          : html`<div class="sheet-icon">${icon('link')}</div>`}
        <div class="grow">
          <h2>${st.item ? html`Assign a persona` : 'Assign a chat'}</h2>
          <p>${st.item ? html`<b>${st.item.isSelf ? 'You (message yourself)' : st.item.name}</b> · ${chatSubtitle(st.item)}` : 'Pick a WhatsApp chat, then who should answer it.'}</p>
        </div>
        <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
      </div>
      ${picker ? html`<div class="field mb-16"><label class="field-label">${icon('chats')}Chat</label>${picker.view()}</div>` : ''}
      <div class="field">
        <label class="field-label">${icon('personas')}Who should reply?</label>
        ${store.state.personas.length
          ? personaTiles(st.personaId, (id) => { st.personaId = id; ctl.update(); }, {
            onCreateAI: () => openAIBuilder({ onSaved: (p) => { st.personaId = p.id; ctl.update(); }, stay: true }),
          })
          : html`<div class="banner info">${icon('info')}<div>You don't have any personas yet. <a href="#/persona/new" @click=${() => ctl.close()}>Create one</a> first.</div></div>`}
      </div>
      <div class="field-row mt-16">
        <div>
          <div class="field-label">${icon('shield')}Approve before sending</div>
          <div class="field-help">Replies wait in <b>Approvals</b> until you tap Send. Recommended while you get to know a persona.</div>
        </div>
        ${toggle(st.approval, (v) => { st.approval = v; ctl.update(); }, { label: 'Approve before sending' })}
      </div>
      <div class="sheet-actions">
        <button class="btn btn-ghost" @click=${() => ctl.close()}>Cancel</button>
        <button class=${'btn btn-primary ' + (st.saving ? 'loading' : '')} ?disabled=${!target || !st.personaId || st.saving} @click=${save}>
          ${icon('check')}Assign
        </button>
      </div>`;
  }, { size: 'wide', label: 'Assign persona' });
  return sheet;
}
