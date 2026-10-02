// people-section.js — the chat drawer's "People" section: who the persona
// answers in a group (mode + per-person switches, "always reply" stars and
// notes), or notes about the contact in a direct chat.
//
//   const people = createPeopleSection({ key, update });
//   people.view(chat)   → template            people.load()  → fetch
//   people.members()    → current members      people.destroy()

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { toggle, seg, toast } from '../ui.js';
import { chatAvatar } from './avatar.js';
import { debounce, relTime, rowsFor, prettyPhone } from '../util.js';
import { helpTip } from './help-tip.js';

const MODES = [
  { value: 'auto', label: 'Automatic', title: 'By group size' },
  { value: 'everyone', label: 'Everyone' },
  { value: 'selected', label: 'Only people I pick' },
];
const NOTES_MAX = 300;

const displayName = (m) => m.name || (m.phone ? prettyPhone(m.phone) : 'Someone');

export function createPeopleSection({ key, update }) {
  const st = {
    data: null, loading: false, error: '',
    q: '',
    notesOpen: new Set(),
    drafts: new Map(),     // jid → notes being typed
    saved: '',             // jid whose notes were just saved
    busy: false,
  };
  const timers = new Map();
  let alive = true;
  const redraw = () => { if (alive) update(); };

  async function load() {
    st.loading = true;
    try {
      st.data = await api.chats.people(key, { quiet: true });
      st.error = '';
    } catch (e) {
      if (!st.data) st.error = (e && e.message) || 'Could not load';
    }
    st.loading = false;
    redraw();
  }
  const reloadSoon = debounce(load, 400);

  async function patch(body, okMsg) {
    try {
      await api.chats.patch(key, { people: body });
      if (okMsg) toast(okMsg, { type: 'success' });
    } catch { /* toasted */ }
    reloadSoon();
  }

  // ── local (optimistic) helpers ─────────────────────────────────
  const members = () => (st.data && st.data.members) || [];
  const others = () => members().filter((m) => !m.isSelf);

  function modeDefault(d, mode) {
    const m = mode || d.mode || 'auto';
    if (m === 'everyone') return true;
    if (m === 'selected') return false;
    return !(d.memberCount > 0) || (d.threshold > 0 && d.memberCount <= d.threshold);
  }

  function setRespond(m, value) {
    const entry = { jid: m.jid, name: m.name || '', respond: value };
    if (m.priority && value !== true) { m.priority = false; entry.priority = false; } // "always reply" implies answering
    m.respond = value == null ? modeDefault(st.data) : value;
    m.respondSource = value == null ? 'mode' : 'person';
    redraw();
    patch({ people: [entry] });
  }

  function setPriority(m) {
    m.priority = !m.priority;
    if (m.priority) { m.respond = true; m.respondSource = 'person'; }
    redraw();
    patch({ people: [{ jid: m.jid, name: m.name || '', priority: m.priority }] },
      m.priority ? `${displayName(m)} always gets a reply` : '');
  }

  function setMode(mode) {
    const d = st.data;
    if (!d) return;
    d.mode = mode;
    d.effectiveMode = modeDefault(d, mode) ? 'everyone' : 'selected';
    for (const m of members()) if (m.respondSource !== 'person') m.respond = modeDefault(d, mode);
    redraw();
    patch({ mode });
  }

  function bulk(value) {
    const list = others().filter((m) => !m.left);
    if (!list.length) return;
    for (const m of list) {
      m.respond = value == null ? modeDefault(st.data) : value;
      m.respondSource = value == null ? 'mode' : 'person';
    }
    redraw();
    patch({ people: list.map((m) => ({ jid: m.jid, name: m.name || '', respond: value })) },
      value == null ? 'Everyone follows the group setting again' : value ? 'Answering everyone in this group' : 'Answering nobody unless you pick them');
  }

  function saveNotes(m, text) {
    st.drafts.set(m.jid, text);
    clearTimeout(timers.get(m.jid));
    timers.set(m.jid, setTimeout(async () => {
      timers.delete(m.jid);
      const notes = (st.drafts.get(m.jid) || '').trim();
      try {
        await api.chats.patch(key, { people: { people: [{ jid: m.jid, name: m.name || '', notes }] } });
        m.notes = notes;
        st.saved = m.jid;
        redraw();
        setTimeout(() => { if (st.saved === m.jid) { st.saved = ''; redraw(); } }, 1600);
      } catch { /* toasted */ }
    }, 700));
  }

  function flushNotes() {
    for (const [jid, t] of timers) {
      clearTimeout(t);
      const m = members().find((x) => x.jid === jid);
      if (m) api.chats.patch(key, { people: { people: [{ jid, name: m.name || '', notes: (st.drafts.get(jid) || '').trim() }] } }, { quiet: true }).catch(() => {});
    }
    timers.clear();
  }

  // ── views ──────────────────────────────────────────────────────
  function notesBox(m, placeholder) {
    const val = st.drafts.has(m.jid) ? st.drafts.get(m.jid) : (m.notes || '');
    const left = NOTES_MAX - [...val].length;
    return html`<div class="person-notes" data-key=${'notes-' + m.jid}>
      <textarea class="textarea" rows=${rowsFor(val, 2, 6)} maxlength=${NOTES_MAX} placeholder=${placeholder}
        @input=${(e) => { saveNotes(m, e.target.value); redraw(); }}>${val}</textarea>
      <div class="person-notes-foot tiny faint">
        <span class="grow">Only the persona sees this. It is never sent to anyone.</span>
        ${st.saved === m.jid ? html`<span class="saved-tick">${icon('check', 'ic-sm')}Saved</span>` : html`<span>${left} left</span>`}
      </div>
    </div>`;
  }

  function personRow(m) {
    const open = st.notesOpen.has(m.jid);
    const custom = m.respondSource === 'person';
    const name = displayName(m);
    const hasNotes = !!(m.notes && m.notes.trim());
    const sub = [];
    if (m.left) sub.push('no longer in the group');
    else if (m.lastSpokeAt) sub.push(`spoke ${relTime(m.lastSpokeAt)}`);
    else sub.push('quiet lately');
    if (m.priority) sub.push('always gets a reply');
    else if (custom) sub.push(m.respond ? 'picked' : 'not answered');
    return html`<div class=${'person ' + (m.respond ? 'is-on ' : 'is-off ') + (m.left ? 'is-left' : '')} data-key=${'p-' + m.jid}>
      <div class="person-row">
        ${chatAvatar({ jid: m.left ? '' : m.jid, name }, 34)}
        <div class="grow person-main">
          <div class="person-name"><span class="ellipsis">${name}</span>${m.isAdmin ? html`<span class="chip chip-sm person-admin">admin</span>` : ''}</div>
          <div class="person-sub ellipsis">${sub.join(' · ')}</div>
        </div>
        <button type="button" class=${'btn btn-ghost btn-icon btn-xs person-star ' + (m.priority ? 'on' : '')} aria-pressed=${String(!!m.priority)}
          title=${m.priority ? 'Always replies to them — click to turn off' : 'Always reply to them (skips the chance and AI checks)'} aria-label=${'Always reply to ' + name}
          @click=${() => setPriority(m)}>${icon('star')}</button>
        <button type="button" class=${'btn btn-ghost btn-icon btn-xs person-note-btn ' + (hasNotes ? 'has-notes' : '')} aria-expanded=${String(open)}
          title=${hasNotes ? 'Notes for the persona' : 'Add notes for the persona'} aria-label=${'Notes about ' + name}
          @click=${() => { if (open) st.notesOpen.delete(m.jid); else st.notesOpen.add(m.jid); redraw(); }}>${icon('edit')}</button>
        <span class="person-respond">
          ${custom ? html`<button type="button" class="btn btn-ghost btn-icon btn-xs reset-btn" title="Follow the group setting again" aria-label=${'Reset ' + name + ' to the group setting'}
            @click=${() => setRespond(m, null)}>${icon('reset')}</button>` : html`<span class="inherit-chip" title="Follows the group setting above">Default</span>`}
          ${toggle(m.respond, (v) => setRespond(m, v), { small: true, label: (m.respond ? 'Answering ' : 'Not answering ') + name })}
        </span>
      </div>
      ${open ? notesBox(m, `What should the persona know about ${name}?`) : ''}
    </div>`;
  }

  function groupView(d) {
    const list = others();
    const active = list.filter((m) => !m.left);
    const left = list.filter((m) => m.left);
    const q = st.q.trim().toLowerCase();
    const match = (m) => !q || displayName(m).toLowerCase().includes(q) || (m.phone || '').includes(q.replace(/\D/g, '') || '\u0000');
    const shown = active.filter(match);
    const answering = active.filter((m) => m.respond).length;
    const everyone = d.effectiveMode === 'everyone';
    const size = d.memberCount > 0 ? `${d.memberCount} members` : 'Members unknown';
    const how = d.mode === 'auto'
      ? (d.memberCount > 0 ? `${everyone ? 'small' : 'big'} group — ${everyone ? 'answering everyone' : 'answering only people you pick'}` : 'answering everyone')
      : (everyone ? 'answering everyone' : 'answering only people you pick');
    return html`
      <div class=${'people-summary ' + (everyone ? 'is-everyone' : 'is-selected')}>
        <span class="people-sum-icon">${icon(everyone ? 'group' : 'user')}</span>
        <div class="grow">
          <div class="people-sum-title"><b>${size}</b> · ${how} ${helpTip('who-answers')}</div>
          <div class="people-sum-sub">${active.length ? `Answering ${answering} of ${active.length}` : 'Nobody to show yet'}${!everyone && d.answerAnyoneWhoAddressesIt ? ' — anyone who says its name or tags it still gets an answer' : ''}${d.mode === 'auto' && d.threshold > 0 ? html` · groups up to <b>${d.threshold}</b> answer everyone` : ''}</div>
        </div>
      </div>
      ${d.membersError ? html`<div class="banner warn small mt-8">${icon('warning')}<div>${d.membersError}</div></div>` : ''}
      <div class="mt-12">${seg(MODES, d.mode || 'auto', setMode, { label: 'Who it answers', cls: 'seg-sm people-modes' })}</div>
      ${active.length ? html`
        <div class="people-tools mt-12">
          ${active.length > 8 ? html`<label class="people-search">${icon('search', 'ic-sm')}<input class="input" type="search" placeholder="Search people" .value=${st.q}
            @input=${(e) => { st.q = e.target.value; redraw(); }} aria-label="Search people"></label>` : html`<span class="grow"></span>`}
          <button type="button" class="btn btn-ghost btn-sm" @click=${() => bulk(true)} title="Answer everyone in this group">Select all</button>
          <button type="button" class="btn btn-ghost btn-sm" @click=${() => bulk(false)} title="Answer nobody unless addressed">Select none</button>
          ${list.some((m) => m.respondSource === 'person') ? html`<button type="button" class="btn btn-ghost btn-sm" @click=${() => bulk(null)} title="Everyone follows the setting above">${icon('reset')}Reset</button>` : ''}
        </div>
        <div class="people-list">
          ${shown.length ? shown.map(personRow) : html`<div class="empty empty-sm"><p class="small">Nobody matches “${st.q}”.</p></div>`}
        </div>` : ''}
      ${left.length ? html`<div class="bf-sub mt-12">${icon('logout', 'ic-sm')}No longer in the group</div>
        <div class="people-list">${left.filter(match).map(personRow)}</div>` : ''}`;
  }

  function dmView(d) {
    const m = members()[0];
    if (!m) return '';
    const name = displayName(m);
    return html`<div class="field-help mb-8">Notes about ${name} help the persona sound like it knows them. Only the persona sees them.</div>
      ${notesBox(m, `What should the persona know about ${name}?`)}`;
  }

  function view(c) {
    const isGroup = c.kind === 'group';
    const d = st.data;
    const title = isGroup ? 'People' : `About ${c.name || 'this contact'}`;
    let body;
    if (!d) {
      body = st.error ? html`<div class="banner danger small">${icon('warning')}<div><strong>Couldn't load the people in this chat.</strong> ${st.error}
          <button class="link-btn" @click=${load}>Try again</button></div></div>`
        : html`<div class="skeleton" style="height:64px;border-radius:16px"></div>`;
    } else {
      body = isGroup ? groupView(d) : dmView(d);
    }
    return html`<section class="drawer-section people-section" data-key="people">
      <h4>${icon(isGroup ? 'group' : 'user', 'ic-sm')}${title}<span class="grow"></span>
        ${isGroup ? html`<button class="btn btn-ghost btn-sm" @click=${load} title="Refresh the member list">${icon('refresh')}</button>` : ''}
      </h4>
      ${body}
    </section>`;
  }

  return {
    view,
    load,
    reloadSoon,
    members,
    destroy() { alive = false; reloadSoon.cancel(); flushNotes(); },
  };
}
