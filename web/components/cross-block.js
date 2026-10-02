// cross-block.js — "What it knows from other chats" in the chat panel's
// Memory tab (cross-chat context). A persona can use what it learned in its
// other chats with the people here: in a group, what someone told it
// one-to-one; in a private chat, what happened in groups you share.
//
//   const cross = createCrossStore(key, update)   cross.load(), cross.data
//   crossBlock({ cross, chat, persona, onUpdate })
//
// Chat and people names come from WhatsApp: always rendered through html``.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store } from '../store.js';
import { seg, toggle, toast, openSheet } from '../ui.js';
import { debounce, relTime, plural } from '../util.js';
import { helpTip } from './help-tip.js';
import { patchChat } from './drawer/shared.js';

export function createCrossStore(key, update) {
  const st = { data: null, loading: false, error: '', alive: true, refreshing: false };
  async function load() {
    st.loading = true;
    try {
      st.data = await api.chats.cross(key, { quiet: true });
      st.error = '';
    } catch (e) {
      if (!st.data) st.error = (e && e.message) || 'Could not load';
    }
    st.loading = false;
    if (st.alive) update();
  }
  const reloadSoon = debounce(load, 300);
  return {
    get data() { return st.data; },
    get loading() { return st.loading; },
    get error() { return st.error; },
    get refreshing() { return st.refreshing; },
    set refreshing(v) { st.refreshing = v; },
    load, reloadSoon,
    destroy() { st.alive = false; reloadSoon.cancel(); },
  };
}

const firstName = (s) => ((s || '').trim().split(/\s+/)[0] || s || '');

/** App defaults for a chat kind (Settings › Memory & recap). */
function defaults(isGroup) {
  const c = (store.state.settings && store.state.settings.memory && store.state.settings.memory.cross) || {};
  return { enabled: c.enabled !== false, mode: (isGroup ? c.groupMode : c.dmMode) || (isGroup ? 'discreet' : 'open') };
}

/** Plain-English help per mode. */
export function modeHelp(mode, isGroup, contact) {
  const who = firstName(contact) || 'them';
  if (isGroup) {
    return {
      off: 'Never uses what people told it one-to-one.',
      discreet: 'Knows what people told it privately and stays consistent, but never brings it up here unless they do.',
      open: 'May refer to it lightly with that person, never in front of others.',
    }[mode] || '';
  }
  return {
    off: `Never uses what happened in groups you share with ${who}.`,
    discreet: `Understands what ${who} means and keeps plans made there, but doesn't bring the group up unless ${who} does.`,
    open: `May mention what happened in your shared groups, since you were both there.`,
  }[mode] || '';
}

const BLOCKED = {
  handoff: 'paused — needs you',
  revealed: 'revealed',
  share_off: 'sharing off there',
  person_off: 'turned off for this person',
  memory_off: 'learning off there',
  mode_off: 'not used',
};

/** Small custom picture: two chat bubbles joined by a dotted line. */
function crossArt() {
  return html`<svg class="cross-art" viewBox="0 0 56 40" width="56" height="40" aria-hidden="true">
    <rect x="2" y="4" width="22" height="15" rx="6" fill="currentColor" opacity="0.18"></rect>
    <rect x="32" y="21" width="22" height="15" rx="6" fill="currentColor" opacity="0.32"></rect>
    <path d="M18 19c0 7 8 9 14 9" fill="none" stroke="currentColor" stroke-width="1.8" stroke-dasharray="2 3" stroke-linecap="round"></path>
    <circle cx="9" cy="11.5" r="1.8" fill="currentColor" opacity="0.6"></circle><circle cx="15" cy="11.5" r="1.8" fill="currentColor" opacity="0.6"></circle>
  </svg>`;
}

function previewSheet(chat, persona) {
  let text = null;
  let err = '';
  openSheet((ctl) => {
    if (text === null && !err) {
      text = '';
      api.personas.promptPreview(chat.personaId, chat.key, { quiet: true })
        .then((r) => { text = typeof r === 'string' ? r : (r && (r.prompt || r.system || r.text)) || ''; ctl.update(); })
        .catch((e) => { err = (e && e.message) || 'Could not load the preview'; ctl.update(); });
    }
    const m = /\n\nWHAT YOU KNOW FROM (?:PRIVATE CHATS|GROUPS YOU SHARE)[\s\S]*?(?=\n\n[A-Z][A-Z ]+[:(]|\n\nRIGHT NOW|\n\nYOUR PRIVATE AGENDA|\n\nGUIDANCE|$)/.exec(text || '');
    const part = m ? m[0].trim() : '';
    return html`
      <div class="sheet-head">
        <div class="sheet-icon">${icon('link')}</div>
        <div class="grow"><h2>What ${persona ? persona.name : 'it'} gets from other chats</h2>
          <p>Exactly what goes into the next reply here, right now. Only short notes, never the messages themselves.</p></div>
      </div>
      ${err ? html`<div class="banner danger small">${icon('warning')}<div>${err}</div></div>`
        : text === '' ? html`<div class="skeleton" style="height:120px;border-radius:14px"></div>`
        : part ? html`<pre class="cross-preview">${part}</pre>`
        : html`<div class="muted small">Nothing right now. It only draws on other chats about the people taking part in the conversation, so this changes as people talk.</div>`}
      <div class="sheet-actions"><button class="btn btn-primary" @click=${() => ctl.close()}>Done</button></div>`;
  }, { size: 'md', label: 'Context from other chats' });
}

export function crossBlock({ cross, chat, persona, onUpdate }) {
  const isGroup = chat.kind === 'group';
  const name = persona ? persona.name : 'The persona';
  const d = cross.data;
  const def = defaults(isGroup);
  const own = !!(chat.cross && chat.cross.mode);
  const mode = d ? d.mode : (own ? chat.cross.mode : def.mode);
  const shareOwn = !!(chat.cross && chat.cross.share != null);
  const share = d ? d.share : (shareOwn ? chat.cross.share : true);
  const contact = chat.name || 'this contact';

  if (d && !d.enabled) {
    return html`<div class="cross-block off" data-key="cross">
      <div class="cross-head">${crossArt()}<div class="grow">
        <div class="field-label">What it knows from other chats ${helpTip('cross')}</div>
        <div class="field-help">Turned off for every chat in Settings › Memory &amp; recap: each chat keeps to itself.</div></div>
        <a class="btn btn-ghost btn-sm" href="#/settings">${icon('settings')}Settings</a>
      </div>
    </div>`;
  }

  async function setMode(v) {
    const body = { cross: { mode: v === def.mode ? null : v, share: chat.cross ? chat.cross.share ?? null : null } };
    try {
      await patchChat(chat.key, body);
      toast({ off: 'Off — other chats are not used here', discreet: 'Discreet — knows, never tells', open: 'Open — may mention it with that person' }[v], { type: 'success' });
    } catch { /* toasted */ }
    cross.reloadSoon();
  }
  async function setShare(v) {
    try {
      await patchChat(chat.key, { cross: { mode: (chat.cross && chat.cross.mode) || null, share: v ? null : false } });
      toast(v ? 'Other chats may use what it learns here' : 'Nothing from this chat is used anywhere else', { type: 'success' });
    } catch { /* toasted */ }
    cross.reloadSoon();
  }
  async function refresh() {
    cross.refreshing = true; onUpdate();
    try { await api.chats.refreshBrief(chat.key); toast('Summary updated', { type: 'success' }); } catch { /* toasted */ }
    cross.refreshing = false;
    cross.reloadSoon();
  }

  const sources = (d && d.sources) || [];
  const usable = sources.filter((s) => !s.blocked);
  const sourceLabel = (s) => (s.kind === 'group' ? s.name || 'A group' : `Private chat with ${firstName(s.person || s.name)}`);
  const sourceDetail = (s) => {
    if (s.blocked) return BLOCKED[s.blocked] || 'not used';
    const bits = [];
    if (s.items) bits.push(plural(s.items, 'thing'));
    if (s.hasBrief) bits.push('recent summary');
    return bits.join(' · ') || 'nothing to carry yet';
  };
  const brief = d && d.brief;

  return html`<div class="cross-block" data-key="cross">
    <div class="cross-head">${crossArt()}<div class="grow">
      <div class="field-label row gap-4">What it knows from other chats ${helpTip('cross')}
        ${own ? html`<span class="inherit-chip custom" title="Set for this chat only">This chat</span>` : html`<span class="inherit-chip" title="Follows Settings › Memory & recap">Default</span>`}</div>
      <div class="field-help">${isGroup ? `When someone here also texts ${name} one-to-one.` : `When ${firstName(contact)} is in a group with ${name} too.`}</div>
    </div></div>
    <div class="cross-mode">${seg([
      { value: 'off', label: 'Off' },
      { value: 'discreet', label: 'Discreet', icon: 'eye-off' },
      { value: 'open', label: 'Open', icon: 'message' },
    ], mode, setMode, { cls: 'seg-sm cross-seg', label: isGroup ? 'Private chats with people here' : 'Groups you share' })}
      <div class="field-help cross-mode-help">${modeHelp(mode, isGroup, contact)}</div></div>

    ${mode !== 'off' ? html`<div class="cross-sources" aria-label="Draws on">
      <span class="tiny faint">Draws on</span>
      ${!d && cross.loading ? html`<span class="skeleton" style="height:22px;width:140px;border-radius:11px"></span>`
        : !sources.length ? html`<span class="tiny muted">${isGroup
          ? `Nobody here has a private chat with ${name} yet.`
          : `${firstName(contact)} isn't in any group ${name} is in.`}</span>`
        : sources.map((s) => html`<span class=${'chip chip-sm cross-src ' + (s.blocked ? 'blocked' : '')} data-key=${s.chatKey}
            title=${s.blocked ? `${sourceLabel(s)}: ${sourceDetail(s)}` : `${sourceLabel(s)}: ${sourceDetail(s)}`}>
            ${icon(s.kind === 'group' ? 'group' : 'user', 'ic-sm')}${sourceLabel(s)}<span class="faint">· ${sourceDetail(s)}</span></span>`)}
    </div>` : ''}
    ${mode !== 'off' && usable.length ? html`<div class="tiny faint cross-note">${icon('lock', 'ic-sm')}Money, health, relationships, secrets and anything that paused a chat never cross over.</div>` : ''}

    <details class="cross-more">
      <summary class="tiny">More options</summary>
      <div class="field-row">
        <div class="grow">
          <div class="field-label">Let other chats use what it learns here
            ${shareOwn ? html`<span class="inherit-chip custom">This chat</span>` : ''}</div>
          <div class="field-help">${share ? `${name}'s other chats with these people may draw on short notes from here. Off: nothing from this chat is used anywhere else.` : 'Off: nothing from this chat is used anywhere else.'}</div>
        </div>
        ${toggle(share, setShare, { label: 'Let other chats use what it learns here' })}
      </div>
      <div class="row gap-8 cross-foot">
        <span class="tiny faint grow">${brief && brief.generatedAt ? `Summary for other chats updated ${relTime(brief.generatedAt)}` : 'No summary of this chat yet'}${d && d.briefPending ? ` · ${plural(d.briefPending, 'new message')}` : ''}</span>
        ${share ? html`<button class=${'btn btn-ghost btn-sm ' + (cross.refreshing ? 'loading' : '')} ?disabled=${cross.refreshing} @click=${refresh}>${icon('refresh')}Update now</button>` : ''}
        <button class="btn btn-ghost btn-sm" @click=${() => previewSheet(chat, persona)}>${icon('eye')}Preview what it gets</button>
      </div>
    </details>
    ${cross.error && !d ? html`<div class="tiny danger-text">${cross.error} <button class="link-btn" @click=${() => cross.load()}>Try again</button></div>` : ''}
  </div>`;
}
