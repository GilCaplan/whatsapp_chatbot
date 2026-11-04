// notes-card.js — "Messages for you" (Dashboard): what personal secretaries
// wrote down for you (GET /api/secretary/notes), urgent first. Mark one done
// or open the chat it came from. Hidden while there is nothing open.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { debounce, relTime } from '../../util.js';

export function createNotesStore(update) {
  const st = { items: null, showDone: false };
  async function load() {
    try { st.items = ((await api.notes.list({ quiet: true })) || {}).items || []; } catch { st.items = st.items || []; }
    update();
  }
  async function setDone(n, done) {
    n.done = done;
    update();
    try { await api.notes.patch(n.id, { done }); } catch { /* toasted */ }
  }
  async function clearDone() {
    try { await api.notes.remove('done'); } catch { /* toasted */ }
    load();
  }
  return { st, load, reloadSoon: debounce(load, 400), setDone, clearDone };
}

export function notesCard(ns, go) {
  const st = ns.st;
  if (!st.items) return '';
  const open = st.items.filter((n) => !n.done).sort((a, b) => (b.urgent - a.urgent) || (new Date(b.at) - new Date(a.at)));
  const done = st.items.filter((n) => n.done);
  if (!open.length && !st.showDone) return '';
  const row = (n) => html`<div class=${'note-row' + (n.urgent ? ' urgent' : '') + (n.done ? ' done' : '')} data-key=${n.id}>
    <span class="note-ic">${icon(n.urgent ? 'warning' : 'inbox', 'ic-sm')}</span>
    <div class="grow note-main">
      <div class="note-top"><b>${n.from || 'Someone'}</b><span class="faint small">${n.chatName ? `in ${n.chatName} · ` : ''}${relTime(n.at)}</span>
        ${n.urgent ? html`<span class="chip chip-sm note-urgent">Urgent</span>` : ''}</div>
      <div class="note-text">${n.text}</div>
      ${n.callback || n.meeting ? html`<div class="tiny faint">${n.callback ? `Get back: ${n.callback}` : ''}${n.callback && n.meeting ? ' · ' : ''}${n.meeting ? `Meeting: ${n.meeting}` : ''}</div>` : ''}
    </div>
    <div class="note-actions">
      <button class="btn btn-ghost btn-sm" @click=${() => go(`/chats/${encodeURIComponent(n.chatKey)}`)}>${icon('chats')}Chat</button>
      <button class="btn btn-glass btn-sm" @click=${() => ns.setDone(n, !n.done)}>${icon(n.done ? 'reset' : 'check')}${n.done ? 'Open again' : 'Done'}</button>
    </div>
  </div>`;
  return html`<section class="card notes-card" aria-label="Messages for you">
    <div class="card-head">
      <h2 class="h3">${icon('inbox', 'ic-sm')}Messages for you</h2>
      <span class="small muted">${open.length ? `${open.length} open${open.some((n) => n.urgent) ? ', urgent first' : ''}` : 'All done'}</span>
      <span class="grow"></span>
      ${done.length ? html`<button class="btn btn-ghost btn-sm" @click=${() => { st.showDone = !st.showDone; ns.load(); }}>${st.showDone ? 'Hide done' : `Show done (${done.length})`}</button>` : ''}
      ${st.showDone && done.length ? html`<button class="btn btn-ghost btn-sm" @click=${ns.clearDone}>${icon('trash')}Clear done</button>` : ''}
    </div>
    ${open.map(row)}
    ${st.showDone ? done.slice(0, 20).map(row) : ''}
  </section>`;
}
