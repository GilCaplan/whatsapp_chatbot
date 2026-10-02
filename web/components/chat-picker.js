// chat-picker.js — searchable WhatsApp chat chooser (server-side search,
// paged, lazy avatars). Used by onboarding and "Assign a chat".

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { seg, spinner, skeletonRows } from '../ui.js';
import { chatAvatar } from './avatar.js';
import { debounce, prettyPhone, plural } from '../util.js';
import { assignmentFor } from '../store.js';

const PAGE = 40;

export function chatSubtitle(it) {
  if (it.isSelf) return 'Your own chat — perfect for testing';
  if (it.kind === 'group') return it.participantCount ? `Group · ${plural(it.participantCount, 'member')}` : 'Group';
  return prettyPhone(it.phone) || 'Contact';
}

/** createChatPicker({ update, onChange, autoSelectSelf }) */
export function createChatPicker({ update = () => {}, onChange = () => {}, autoSelectSelf = true, hideAssigned = false } = {}) {
  const st = { q: '', tab: 'recent', items: [], total: 0, loading: false, error: '', selected: null, seq: 0, ctl: null };

  async function load(reset = true) {
    const seq = ++st.seq;
    if (st.ctl) st.ctl.abort();
    const ctl = new AbortController();
    st.ctl = ctl;
    st.loading = true;
    st.error = '';
    if (reset) { st.items = []; st.total = 0; }
    update();
    try {
      const r = await api.wa.chats({ q: st.q, tab: st.tab, limit: PAGE, offset: reset ? 0 : st.items.length }, { quiet: true, signal: ctl.signal });
      if (seq !== st.seq) return;
      const items = (r && r.items) || [];
      st.items = reset ? items : st.items.concat(items);
      st.total = (r && r.total) || st.items.length;
      if (autoSelectSelf && !st.selected) {
        const self = st.items.find((i) => i.isSelf);
        if (self) { st.selected = self; onChange(self); }
      }
    } catch (e) {
      if (e.name === 'AbortError') return;
      if (seq === st.seq) st.error = e.message || 'Could not load chats';
    } finally {
      if (seq === st.seq) { st.loading = false; update(); }
    }
  }

  const search = debounce(() => load(true), 200);

  function pick(it) {
    st.selected = it;
    onChange(it);
    update();
  }

  function view() {
    const items = hideAssigned ? st.items.filter((i) => !i.assigned) : st.items;
    const more = st.items.length < st.total;
    return html`<div class="chat-picker col gap-10">
      <div class="row gap-8 row-wrap">
        <div class="input-wrap grow" style="min-width:200px">
          ${icon('search')}
          <input class="input search-input" type="search" placeholder="Search chats" .value=${st.q}
            @input=${(e) => { st.q = e.target.value; update(); search(); }}
            @keydown=${(e) => { if (e.key === 'Enter') { e.preventDefault(); search.flush(); } }}>
        </div>
        ${seg([
          { value: 'recent', label: 'Recent' },
          { value: 'contacts', label: 'Contacts' },
          { value: 'groups', label: 'Groups' },
        ], st.tab, (v) => { st.tab = v; load(true); }, { cls: 'seg-sm', label: 'Chat type' })}
      </div>
      <div class="picker-list glass">
        ${st.error ? html`<div class="banner danger" style="margin:8px">${icon('warning')}<div>${st.error} <button class="link-btn" @click=${() => load(true)}>Try again</button></div></div>` : ''}
        ${!st.items.length && st.loading ? skeletonRows(4) : ''}
        ${!st.items.length && !st.loading && !st.error ? html`<div class="empty empty-sm"><p>${st.q ? html`No chats match “${st.q}”.` : 'No chats here yet. Once WhatsApp syncs, they will show up.'}</p></div>` : ''}
        ${items.map((it) => {
          const sel = st.selected && st.selected.key === it.key;
          const asg = assignmentFor(it);
          return html`<div class=${'list-row clickable ' + (sel ? 'selected' : '')} data-key=${it.key} role="option" aria-selected=${String(!!sel)} tabindex="0"
              @click=${() => pick(it)} @keydown=${(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); pick(it); } }}>
            ${chatAvatar({ jid: it.jid, name: it.isSelf ? 'You' : it.name, kind: it.kind }, 38)}
            <div class="grow">
              <div class="title ellipsis">${it.isSelf ? 'You (message yourself)' : (it.name || prettyPhone(it.phone) || it.jid)}</div>
              <div class="sub ellipsis">${chatSubtitle(it)}${asg ? html` · <span class="faint">already assigned</span>` : ''}</div>
            </div>
            ${sel ? html`<span class="sel-check">${icon('check')}</span>` : ''}
          </div>`;
        })}
        ${more ? html`<load-more token=${st.items.length} @visible=${() => { if (!st.loading) load(false); }}></load-more>` : ''}
        ${st.items.length && st.loading ? html`<div class="center" style="padding:10px">${spinner()}</div>` : ''}
      </div>
    </div>`;
  }

  return { view, load, get selected() { return st.selected; }, state: st };
}
