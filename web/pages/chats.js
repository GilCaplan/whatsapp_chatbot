// pages/chats.js — browse WhatsApp chats (server-side search + paging),
// assign personas, and open a chat's drawer (#/chats/<key>).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, personaById, assignmentFor, isAway } from '../store.js';
import { seg, toggle, toast, spinner, skeletonRows } from '../ui.js';
import { chatAvatar, avatarStack, personaAvatar } from '../components/avatar.js';
import { emptyState } from '../components/art.js';
import { chatSubtitle } from '../components/chat-picker.js';
import { openAssignSheet } from '../components/assign-sheet.js';
import { openChatDrawer, patchChat, awayUntilText, DRAWER_TABS } from '../components/chat-drawer.js';
import { debounce, prettyPhone } from '../util.js';
import { openWASheet } from '../components/wa-sheet.js';

const PAGE = 50;
const TABS = ['recent', 'assigned', 'contacts', 'groups'];

export default function Chats(ctx) {
  const update = () => ctx.update();
  const q0 = ctx.route.query.get('tab');
  const st = {
    tab: TABS.includes(q0) ? q0 : 'recent',
    q: '',
    items: [],
    total: 0,
    loading: false,
    error: '',
    seq: 0,
    abort: null,
    refreshing: false,
  };
  let drawer = null;
  let drawerKey = '';

  async function load(reset = true) {
    if (st.tab === 'assigned') { st.loading = false; update(); return; }
    const seq = ++st.seq;
    if (st.abort) st.abort.abort();
    st.abort = new AbortController();
    st.loading = true;
    st.error = '';
    if (reset) { st.items = []; st.total = 0; }
    update();
    try {
      const r = await api.wa.chats({ q: st.q.trim(), tab: st.tab, limit: PAGE, offset: reset ? 0 : st.items.length },
        { quiet: true, signal: st.abort.signal });
      if (seq !== st.seq) return;
      const items = (r && r.items) || [];
      st.items = reset ? items : st.items.concat(items.filter((x) => !st.items.some((y) => y.key === x.key)));
      st.total = (r && typeof r.total === 'number') ? r.total : st.items.length;
    } catch (e) {
      if (e.name === 'AbortError') return;
      if (seq === st.seq) st.error = e.message || 'Could not load chats';
    } finally {
      if (seq === st.seq) { st.loading = false; update(); }
    }
  }
  const search = debounce(() => load(true), 200);

  /** Route param "<key>" or "<key>/<drawer tab>" (keys never contain "/"). */
  function splitParam(param) {
    const [key = '', tab = ''] = (param || '').split('/');
    return { key, tab: DRAWER_TABS.includes(tab) ? tab : '' };
  }

  const chatPath = (key, drawerTab) => `/chats${key ? '/' + encodeURIComponent(key) + (drawerTab ? '/' + drawerTab : '') : ''}${st.tab !== 'recent' ? '?tab=' + st.tab : ''}`;

  function setTab(t) {
    st.tab = t;
    const { key, tab } = splitParam(ctx.route.param);
    history.replaceState(null, '', '#' + chatPath(key, tab));
    load(true);
  }

  async function refresh() {
    st.refreshing = true; update();
    try {
      await api.wa.refreshChats();
      toast('Refreshing contacts and groups from WhatsApp…');
      await load(true);
    } catch { /* toasted */ } finally { st.refreshing = false; update(); }
  }

  function syncDrawer(route) {
    const { key, tab } = splitParam(route.param);
    if (key === drawerKey) {
      if (key && tab && drawer && !drawer.closed && drawer.tab() !== tab) drawer.setTab(tab);
      return;
    }
    if (drawer && !drawer.closed) { const d = drawer; drawer = null; d.close(); }
    drawerKey = key;
    if (key) {
      drawer = openChatDrawer(key, {
        tab,
        // Keep the URL deep-linkable (#/chats/<key>/<tab>) without a history entry per tab.
        onTab: (t) => { if (drawerKey === key) ctx.navigate(chatPath(key, t), { replace: true }); },
        onClose: () => {
          if (drawerKey === key) {
            drawerKey = '';
            drawer = null;
            const r = ctx.route;
            if (r.name === 'chats' && splitParam(r.param).key === key) ctx.navigate(chatPath(''));
          }
        },
      });
    }
  }

  const openChat = (key) => ctx.navigate(chatPath(key));

  function assignedControls(a) {
    const p = personaById(a.personaId);
    return html`
      ${isAway(a) ? html`<span class="chip chip-sm chip-away" title=${`Away ${awayUntilText(a.snoozedUntil)}`}>${icon('snooze')}Away</span>` : ''}
      <span class="persona-chip" title=${p ? p.name : 'Persona missing'}>${personaAvatar(p, 22)}<span class="ellipsis">${p ? p.name : 'Missing'}</span></span>
      <span class="mini-switch" title=${a.mode === 'copilot' ? 'Co-pilot: you pick one of three drafts' : "Approve each reply before it's sent"} @click=${(e) => e.stopPropagation()}>
        <span class="faint tiny">${a.mode === 'copilot' ? 'Co-pilot' : 'Approve'}</span>
        ${toggle(a.approvalMode, (v) => patchChat(a.key, { approvalMode: v }).catch(() => {}), { small: true, label: 'Approve before sending' })}
      </span>
      <span class="mini-switch" title="Replies on/off" @click=${(e) => e.stopPropagation()}>
        <span class="faint tiny">On</span>
        ${toggle(a.enabled, (v) => patchChat(a.key, { enabled: v }).catch(() => {}), { small: true, label: 'Replies on' })}
      </span>
      ${icon('chevron-right', 'faint')}`;
  }

  function row(it) {
    const a = assignmentFor(it);
    const p = a ? personaById(a.personaId) : null;
    const name = it.isSelf ? 'You (message yourself)' : (it.name || prettyPhone(it.phone) || it.jid);
    return html`<div class=${'list-row chat-row ' + (a ? 'clickable assigned' : '')} data-key=${it.key}
        @click=${a ? () => openChat(a.key) : null}>
      ${a ? avatarStack({ jid: it.jid, name: it.isSelf ? 'You' : it.name, kind: it.kind }, p, 42)
        : chatAvatar({ jid: it.jid, name: it.isSelf ? 'You' : it.name, kind: it.kind }, 42)}
      <div class="grow">
        <div class="title ellipsis">${name}${it.isSelf ? html` <span class="chip chip-sm chip-green">test here</span>` : ''}</div>
        <div class="sub ellipsis">${chatSubtitle(it)}</div>
      </div>
      ${it.lastMessageAt ? html`<rel-time class="faint small nowrap hide-sm" datetime=${it.lastMessageAt}></rel-time>` : ''}
      ${a ? assignedControls(a)
        : html`<button class="btn btn-glass btn-sm" @click=${(e) => { e.stopPropagation(); openAssignSheet({ item: it }); }}>${icon('plus')}Assign</button>`}
    </div>`;
  }

  function assignedRows() {
    const q = st.q.trim().toLowerCase();
    const list = store.state.chats.filter((c) => !q || (c.name || '').toLowerCase().includes(q) || (c.jid || '').includes(q));
    if (!store.state.chatsLoaded) return skeletonRows(4);
    if (!list.length) {
      return q ? emptyState({ artName: 'search', small: true, title: 'No matches', body: `No assigned chat matches “${st.q}”.` })
        : emptyState({ artName: 'chats', title: 'No personas on duty yet', body: 'Assign a persona to a chat and it will show up here.', action: { label: 'Assign a chat', icon: 'plus', onClick: () => openAssignSheet() } });
    }
    return list.map((c) => {
      const p = personaById(c.personaId);
      return html`<div class="list-row chat-row clickable assigned" data-key=${c.key} @click=${() => openChat(c.key)}>
        ${avatarStack({ jid: c.jid, name: c.name, kind: c.kind }, p, 42)}
        <div class="grow">
          <div class="title ellipsis">${c.name || c.jid}</div>
          <div class="sub ellipsis">${c.kind === 'group' ? 'Group' : 'Direct chat'}${c.goalOverride ? html` · goal: ${c.goalOverride}${c.goal && c.goal.state === 'reached' ? ' (reached)' : ''}` : ''}</div>
        </div>
        ${c.pendingCount ? html`<span class="badge" title="Waiting for approval">${c.pendingCount}</span>` : ''}
        ${c.lastActivityAt ? html`<rel-time class="faint small nowrap hide-sm" datetime=${c.lastActivityAt}></rel-time>` : ''}
        ${assignedControls(c)}
      </div>`;
    });
  }

  function listBody() {
    if (st.tab === 'assigned') return assignedRows();
    if (st.error) {
      return html`<div class="banner danger" style="margin:10px">${icon('warning')}<div><strong>Couldn't load chats.</strong> ${st.error}
        <button class="link-btn" @click=${() => load(true)}>Try again</button></div></div>`;
    }
    if (!st.items.length && st.loading) return skeletonRows(7);
    if (!st.items.length) {
      const wa = store.state.wa && store.state.wa.state;
      if (st.q) return emptyState({ artName: 'search', small: true, title: 'Nothing found', body: `No ${st.tab === 'groups' ? 'groups' : 'chats'} match “${st.q}”. Try part of a name or a phone number.` });
      if (wa !== 'connected') return emptyState({ artName: 'link', title: 'WhatsApp isn\'t linked', body: 'Link your WhatsApp to see your chats here.', action: { label: 'Link WhatsApp', icon: 'qr', onClick: () => openWASheet() } });
      return emptyState({ artName: 'chats', small: true, title: 'Nothing here yet', body: 'Chats appear as WhatsApp syncs. You can also refresh contacts and groups.', action: { label: 'Refresh from WhatsApp', icon: 'refresh', onClick: refresh, cls: 'btn-glass' } });
    }
    const more = st.items.length < st.total;
    return html`${st.items.map(row)}
      ${more ? html`<load-more token=${st.items.length} @visible=${() => { if (!st.loading) load(false); }}></load-more>` : ''}
      ${st.loading ? html`<div class="center" style="padding:14px">${spinner()}</div>` : ''}
      ${!more && st.items.length > 12 ? html`<div class="list-end faint small">That's everything · ${st.total} ${st.tab === 'groups' ? 'groups' : 'chats'}</div>` : ''}`;
  }

  function view() {
    const assignedCount = store.state.chats.length;
    return html`<div class="chats-page">
      <div class="toolbar">
        ${seg([
          { value: 'recent', label: 'Recent', icon: 'clock' },
          { value: 'assigned', label: 'Assigned', icon: 'personas', count: assignedCount || null },
          { value: 'contacts', label: 'Contacts', icon: 'user' },
          { value: 'groups', label: 'Groups', icon: 'group' },
        ], st.tab, setTab, { label: 'Chat list' })}
        <div class="input-wrap grow" style="min-width:220px">
          ${icon('search')}
          <input class="input search-input" type="search" placeholder=${st.tab === 'groups' ? 'Search groups' : st.tab === 'contacts' ? 'Search 1000s of contacts by name or number' : 'Search chats'}
            .value=${st.q} @input=${(e) => { st.q = e.target.value; if (st.tab === 'assigned') update(); else search(); }}
            @keydown=${(e) => { if (e.key === 'Escape' && st.q) { st.q = ''; e.target.value = ''; search.flush(); } }}>
        </div>
        <button class=${'btn btn-glass btn-icon ' + (st.refreshing ? 'loading' : '')} title="Refresh contacts & groups from WhatsApp" aria-label="Refresh" @click=${refresh}>${icon('refresh')}</button>
        <button class="btn btn-primary" @click=${() => openAssignSheet()}>${icon('plus')}<span class="hide-sm">Assign</span></button>
      </div>
      <div class="card list-card">
        <div class="list">${listBody()}</div>
      </div>
    </div>`;
  }

  return {
    title: 'Chats',
    view,
    mount() { load(true); syncDrawer(ctx.route); },
    onParams(route) {
      const t = route.query.get('tab');
      if (t && TABS.includes(t) && t !== st.tab) { st.tab = t; load(true); }
      syncDrawer(route);
    },
    unmount() {
      if (st.abort) st.abort.abort();
      search.cancel();
      if (drawer && !drawer.closed) { drawerKey = ''; const d = drawer; drawer = null; d.close(); }
    },
  };
}
