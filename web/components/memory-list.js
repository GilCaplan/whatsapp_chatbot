// memory-list.js — what a persona remembers about the people in one chat
// (wave 3, Memory of people): a small per-drawer store and the list UI
// (grouped by person; pin, edit, delete; "Remember this…").
//
//   const mem = createMemoryStore(key, update)   mem.load(), mem.data, mem.byPerson()
//   memoryList({ mem, chat, persona, ui, onUpdate })
//
// Memory texts and names come from chats: always rendered through html``.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store } from '../store.js';
import { toast, confirmSheet } from '../ui.js';
import { debounce, relTime, plural } from '../util.js';
import { providerLabel } from './status.js';

export const KIND_LABEL = { fact: 'Fact', preference: 'Likes', event: 'Coming up', relationship: 'People', other: 'Note' };
/** Sensitive topics that never cross into other chats on their own. */
export const SENSITIVE_LABEL = { money: 'Money', health: 'Health', meeting: 'Meeting up', distress: 'Struggling', bot: 'Bot question', legal: 'Legal', romance: 'Relationships', secret: 'Told in confidence' };
const MAX_TEXT = 160;

export function createMemoryStore(key, update) {
  const st = { data: null, loading: false, error: '', alive: true };
  async function load() {
    st.loading = true;
    try {
      st.data = await api.chats.memories(key, { quiet: true });
      st.error = '';
    } catch (e) {
      if (!st.data) st.error = (e && e.message) || 'Could not load memories';
    }
    st.loading = false;
    if (st.alive) update();
  }
  const reloadSoon = debounce(load, 250);
  const items = () => (st.data && st.data.items) || [];
  /** [{ person, items }] — people in first-seen order, "" (the contact / unknown) last. */
  function byPerson() {
    const groups = new Map();
    for (const m of items()) {
      const p = m.person || '';
      if (!groups.has(p)) groups.set(p, []);
      groups.get(p).push(m);
    }
    const out = [...groups.entries()].map(([person, list]) => ({ person, items: list }));
    out.sort((a, b) => (a.person === '') - (b.person === ''));
    return out;
  }
  /** Count of memories about a person (by name, first-name match). */
  function countFor(name) {
    const n = (name || '').trim().toLowerCase();
    if (!n) return 0;
    const first = n.split(/\s+/)[0];
    return items().filter((m) => {
      const p = (m.person || '').toLowerCase();
      return p === n || (p && p.split(/\s+/)[0] === first);
    }).length;
  }
  return {
    get data() { return st.data; },
    get loading() { return st.loading; },
    get error() { return st.error; },
    load, reloadSoon, items, byPerson, countFor,
    destroy() { st.alive = false; reloadSoon.cancel(); },
  };
}

/** The AI that writes memories for this chat ("" = runs on this computer). */
function cloudProvider(persona) {
  const s = store.state.settings && store.state.settings.llm;
  const prov = (persona && persona.llm && persona.llm.provider) || (s && s.defaultProvider) || 'ollama';
  return prov === 'ollama' ? '' : providerLabel(prov);
}

function whenText(m) {
  if (m.kind === 'event' && m.expiresAt) {
    const d = new Date(m.expiresAt);
    if (!isNaN(d)) {
      const past = d < new Date();
      return `${past ? 'was' : 'on'} ${d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' })}`;
    }
  }
  return m.source === 'user' ? `added by you ${relTime(m.createdAt)}` : `learned ${relTime(m.createdAt)}`;
}

export function memoryList({ mem, chat, persona, ui, onUpdate }) {
  const key = chat.key;
  const isGroup = chat.kind === 'group';
  const name = persona ? persona.name : 'The persona';
  const data = mem.data;
  const groups = mem.byPerson();
  const filter = ui.filter || '';
  const shown = filter ? groups.filter((g) => g.person === filter) : groups;

  async function patchMem(m, body, ok) {
    Object.assign(m, body); onUpdate();
    try { await api.chats.patchMemory(key, m.id, body); if (ok) toast(ok, { type: 'success' }); } catch { /* toasted */ }
    mem.reloadSoon();
  }
  async function remove(m) {
    if (data) data.items = data.items.filter((x) => x.id !== m.id);
    onUpdate();
    try { await api.chats.deleteMemory(key, m.id); toast('Forgotten', { type: 'success' }); } catch { /* toasted */ }
    mem.reloadSoon();
  }
  async function saveEdit(m) {
    const text = (ui.editText || '').trim();
    ui.editing = '';
    if (!text || text === m.text) { onUpdate(); return; }
    await patchMem(m, { text }, 'Saved');
  }
  async function add() {
    const text = (ui.newText || '').trim();
    if (!text) return;
    if (text.length > MAX_TEXT) { toast(`Keep it to one line (${MAX_TEXT} characters)`, { type: 'warn' }); return; }
    ui.adding = true; onUpdate();
    try {
      await api.chats.addMemory(key, { text, person: isGroup ? (ui.newPerson || '').trim() : (chat.name || ''), pinned: false });
      ui.newText = '';
      toast(`${name} will remember that`, { type: 'success' });
    } catch { /* toasted */ }
    ui.adding = false;
    mem.reloadSoon();
  }
  async function forgetAll() {
    const ok = await confirmSheet({
      title: 'Forget everything here?',
      body: `${name} forgets everything it learned in this chat, including what you added. This can't be undone.`,
      confirm: 'Forget everything', danger: true, iconName: 'trash',
    });
    if (!ok) return;
    try { await api.chats.clearMemories(key); toast('Forgotten', { type: 'success' }); } catch { /* toasted */ }
    ui.filter = '';
    mem.reloadSoon();
  }

  const crossOn = !!(data && data.crossEnabled);
  const elsewhere = isGroup ? 'private chats' : 'groups';
  /** One badge about where a memory may be used (local, sensitive, unlocked, shared). */
  function crossBadge(m) {
    const sens = m.effectiveSensitive || '';
    if (m.scope === 'local') return html`<span class="mem-badge local" title="Never used in other chats">${icon('lock', 'ic-sm')}Private to this chat</span>`;
    if (sens && m.scope !== 'shared') {
      return html`<span class="chip chip-sm chip-amber mem-badge" title="Sensitive things stay in this chat unless you unlock them">${icon('lock', 'ic-sm')}Sensitive · ${SENSITIVE_LABEL[sens] || 'Private'}</span>`;
    }
    if (m.scope === 'shared') return html`<span class="mem-badge unlocked" title="You let other chats use this">${icon('unlock', 'ic-sm')}Unlocked by you</span>`;
    if (m.shares) return html`<span class="mem-badge shares" title=${`${name} may use this in its ${elsewhere} with ${m.person || 'them'}`}>${icon('link', 'ic-sm')}Shared with ${elsewhere}</span>`;
    return '';
  }
  async function toggleLock(m) {
    const sens = m.effectiveSensitive || '';
    if (m.scope === 'local' || m.scope === 'shared') {
      await patchMem(m, { scope: null }, m.scope === 'local' ? (sens ? 'Sensitive: it still stays in this chat' : 'Other chats may use this again') : 'Kept in this chat again');
      return;
    }
    if (sens) {
      const who = (m.person || chat.name || 'they').trim().split(/\s+/)[0];
      const ok = await confirmSheet({
        title: 'Share this with other chats?',
        body: `This looks like it is about ${(SENSITIVE_LABEL[sens] || 'something private').toLowerCase()}. If you unlock it, ${name} may use it in other chats with ${who} — discreetly by default. Only do this if ${who} wouldn't mind.`,
        confirm: 'Unlock', danger: true, iconName: 'lock',
      });
      if (ok) await patchMem(m, { scope: 'shared' }, 'Unlocked: other chats may use it');
      return;
    }
    await patchMem(m, { scope: 'local' }, 'Kept in this chat only');
  }
  function lockButton(m) {
    if (!crossOn && !m.scope) return '';
    const sens = m.effectiveSensitive || '';
    const locked = m.scope === 'local' || (sens && m.scope !== 'shared');
    const title = m.scope === 'local' ? 'Private to this chat. Click to let other chats use it'
      : locked ? 'Sensitive: kept in this chat. Click to share it anyway'
      : m.scope === 'shared' ? 'Unlocked by you. Click to keep it in this chat'
      : 'Keep this in this chat only';
    return html`<button class=${'btn btn-ghost btn-icon btn-sm ' + (locked ? 'on' : '')} aria-pressed=${String(!!locked)} title=${title} aria-label=${title}
      @click=${() => toggleLock(m)}>${icon(locked ? 'lock' : 'unlock')}</button>`;
  }

  const cloud = cloudProvider(persona);
  const people = groups.map((g) => g.person).filter(Boolean);

  function row(m) {
    const editing = ui.editing === m.id;
    return html`<li class=${'mem-row ' + (m.pinned ? 'pinned' : '')} data-key=${m.id}>
      ${editing ? html`<div class="mem-edit">
          <input class="input input-sm" maxlength=${MAX_TEXT} aria-label="Edit memory" .value=${ui.editText}
            @input=${(e) => { ui.editText = e.target.value; }}
            @keydown=${(e) => { if (e.key === 'Enter') { e.preventDefault(); saveEdit(m); } if (e.key === 'Escape') { ui.editing = ''; onUpdate(); } }}>
          <button class="btn btn-primary btn-sm" @click=${() => saveEdit(m)}>${icon('check')}Save</button>
          <button class="btn btn-ghost btn-sm" @click=${() => { ui.editing = ''; onUpdate(); }}>Cancel</button>
        </div>`
        : html`<div class="mem-main">
          <div class="mem-text">${m.text}</div>
          <div class="mem-meta tiny faint">
            <span class=${'mem-kind k-' + (m.kind || 'other')}>${KIND_LABEL[m.kind] || 'Note'}</span>
            <span>${whenText(m)}</span>
            ${m.evidence ? html`<span class="mem-evidence" title=${`From: “${m.evidence}”`}>${icon('quote', 'ic-sm')}source</span>` : ''}
            ${crossOn || m.scope ? crossBadge(m) : ''}
          </div>
        </div>
        <div class="mem-actions">
          <button class=${'btn btn-ghost btn-icon btn-sm ' + (m.pinned ? 'on' : '')} aria-pressed=${String(!!m.pinned)}
            title=${m.pinned ? 'Pinned — always kept and always in mind. Click to unpin' : 'Pin: always keep this and always keep it in mind'}
            aria-label=${m.pinned ? 'Unpin' : 'Pin'} @click=${() => patchMem(m, { pinned: !m.pinned }, m.pinned ? 'Unpinned' : 'Pinned')}>${icon('pin')}</button>
          ${lockButton(m)}
          <button class="btn btn-ghost btn-icon btn-sm" title="Edit" aria-label="Edit" @click=${() => { ui.editing = m.id; ui.editText = m.text; onUpdate(); requestAnimationFrame(() => { const el = document.querySelector('.mem-edit input'); if (el) el.focus(); }); }}>${icon('edit')}</button>
          <button class="btn btn-ghost btn-icon btn-sm" title="Forget this" aria-label="Forget this" @click=${() => remove(m)}>${icon('trash')}</button>
        </div>`}
    </li>`;
  }

  return html`<div class="memory-list">
    ${cloud ? html`<div class="banner warn small mem-cloud">${icon('lock')}<div>Memories here are written by <b>${cloud}</b>, which reads the messages it learns from. Switch this persona's Brain to Ollama to keep everything on this computer.</div></div>` : ''}
    ${groups.length > 1 ? html`<div class="chip-row mem-filter" role="group" aria-label="Show memories about">
      <button type="button" class=${'chip chip-sm ' + (!filter ? 'on' : '')} aria-pressed=${String(!filter)} @click=${() => { ui.filter = ''; onUpdate(); }}>Everyone</button>
      ${groups.map((g) => html`<button type="button" class=${'chip chip-sm ' + (filter === g.person ? 'on' : '')} aria-pressed=${String(filter === g.person)}
        @click=${() => { ui.filter = filter === g.person ? '' : g.person; onUpdate(); }}>${g.person || (isGroup ? 'Someone' : chat.name || 'Them')} <span class="faint">${g.items.length}</span></button>`)}
    </div>` : ''}
    ${!data && mem.loading ? html`<div class="skeleton" style="height:72px;border-radius:14px"></div>`
      : mem.error ? html`<div class="banner danger small">${icon('warning')}<div>${mem.error} <button class="link-btn" @click=${() => mem.load()}>Try again</button></div></div>`
      : !groups.length ? html`<div class="mem-empty">
          <span class="mem-empty-art" aria-hidden="true">${memoryArt()}</span>
          <div><b>Nothing yet.</b> <span class="muted">After a few messages ${name} starts noting things ${isGroup ? 'people here' : chat.name || 'they'} mention — a new job, a trip, a favourite team.</span></div>
        </div>`
      : shown.map((g) => html`<div class="mem-group" data-key=${'g:' + g.person}>
          <div class="mem-person">${personDot(g.person || chat.name || '?')}<b>${g.person || (isGroup ? 'Someone' : chat.name || 'Them')}</b><span class="faint small">${plural(g.items.length, 'thing')}</span></div>
          <ul class="mem-items">${g.items.map(row)}</ul>
        </div>`)}

    <div class="mem-add">
      <div class="field-label">${icon('plus', 'ic-sm')}Remember this…</div>
      <div class="mem-add-row">
        ${isGroup ? html`<input class="input input-sm mem-who" list=${'mem-people-' + key} maxlength="60" placeholder="Who" aria-label="Who it's about" .value=${ui.newPerson || ''}
          @input=${(e) => { ui.newPerson = e.target.value; }}>
          <datalist id=${'mem-people-' + key}>${people.map((p) => html`<option value=${p}></option>`)}</datalist>` : ''}
        <input class="input input-sm grow" maxlength=${MAX_TEXT} placeholder=${isGroup ? 'e.g. is training for the Tel Aviv marathon' : `e.g. ${chat.name ? chat.name.split(' ')[0] + ' is' : 'is'} allergic to cats`} aria-label="Something to remember"
          .value=${ui.newText || ''} @input=${(e) => { ui.newText = e.target.value; onUpdate(); }} @keydown=${(e) => { if (e.key === 'Enter') { e.preventDefault(); add(); } }}>
        <button class=${'btn btn-glass btn-sm ' + (ui.adding ? 'loading' : '')} ?disabled=${!(ui.newText || '').trim() || ui.adding} @click=${add}>${icon('check')}Add</button>
      </div>
      <div class="tiny faint">Things you add are never rewritten by the AI.</div>
    </div>

    ${groups.length ? html`<div class="row gap-8 mem-foot">
      <span class="tiny faint grow">${data && data.lastExtractedAt ? `Last updated ${relTime(data.lastExtractedAt)}` : ''}</span>
      <button class="btn btn-ghost btn-sm danger-text" @click=${forgetAll}>${icon('trash')}Forget everything here</button>
    </div>` : ''}
  </div>`;
}

function personDot(name) {
  const ch = (name || '?').trim().charAt(0).toUpperCase() || '?';
  return html`<span class="mem-dot" aria-hidden="true">${ch}</span>`;
}

/** Small custom illustration: a speech bubble with a bookmark. */
function memoryArt() {
  return html`<svg viewBox="0 0 64 56" width="64" height="56">
    <defs><linearGradient id="memg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#818cf8"></stop><stop offset="1" stop-color="#22d3ee"></stop></linearGradient></defs>
    <path d="M10 6h44a6 6 0 0 1 6 6v24a6 6 0 0 1-6 6H26l-10 9v-9h-6a6 6 0 0 1-6-6V12a6 6 0 0 1 6-6z" fill="url(#memg)" opacity="0.9"></path>
    <path d="M40 6h10v20l-5-4-5 4z" fill="#fff" opacity="0.85"></path>
    <rect x="13" y="17" width="20" height="3.5" rx="1.75" fill="#fff" opacity="0.9"></rect>
    <rect x="13" y="25" width="28" height="3.5" rx="1.75" fill="#fff" opacity="0.7"></rect>
  </svg>`;
}
