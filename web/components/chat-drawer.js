// chat-drawer.js — the chat panel (a centered window): header, status
// banners and a tab bar  Overview · People · Goal · Behaviour · Memory.
// Each tab is its own module under ./drawer/ so features can grow in parallel:
//
//   createXTab(ctx) → { id, view(chat, persona), load?(), shown?(),
//                       onBus?(type, data), destroy?(), <hooks for ctx.notify> }
//
// ctx (shared by every tab):
//   key, update(), el(), close(), setTab(id), notify(hook, data),
//   safePatch(body)   optimistic PATCH /api/chats/{key} (errors are toasted)
//   people            people-section.js instance (People tab + Overview "Test it")
//   behaviour         behaviour controller (cb(), editOverride(), openForm(), reloadSoon())
//   away              { set(until|null), menu(event) }
//
// The open tab is remembered for the session and is part of the URL
// (#/chats/<key>/<tab>); openChatDrawer(key, { tab, onTab }) wires that up.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { store, bus, chatByKey, personaById, loadBehaviorMeta, isAway } from '../store.js';
import { openDrawer, openMenu, toggle, toast, spinner } from '../ui.js';
import { avatarStack } from './avatar.js';
import { emptyState } from './art.js';
import { relTime } from '../util.js';
import { createPeopleSection } from './people-section.js';
import { patchChat, awayUntilText, FOREVER, tomorrowAt, pickAwayTime } from './drawer/shared.js';
import { drawerBanners } from './drawer/banners.js';
import { createOverviewTab } from './drawer/overview.js';
import { createPeopleTab } from './drawer/people.js';
import { createGoalTab } from './drawer/goal.js';
import { createBehaviourTab } from './drawer/behaviour.js';
import { createMemoryTab } from './drawer/memory.js';

// Public helpers used by other pages.
export { patchChat, bubbles, awayUntilText } from './drawer/shared.js';

/** Tab ids in order (also the URL segment: #/chats/<key>/<tab>). */
export const DRAWER_TABS = ['overview', 'people', 'goal', 'behaviour', 'memory'];

const TAB_STORE = 'doppel.drawer.tab';

function tabMeta(id, c) {
  const group = c && c.kind === 'group';
  switch (id) {
    case 'overview': return { label: 'Overview', icon: 'dashboard' };
    case 'people': return group ? { label: 'People', icon: 'group' } : { label: 'Contact', icon: 'user' };
    case 'goal': return { label: 'Goal', icon: 'target' };
    case 'behaviour': return { label: 'Behaviour', icon: 'sliders' };
    case 'memory': return { label: 'Memory', icon: 'brain' };
    default: return { label: id, icon: 'info' };
  }
}

function rememberedTab() {
  try { const t = sessionStorage.getItem(TAB_STORE); return DRAWER_TABS.includes(t) ? t : ''; } catch { return ''; }
}

function rememberTab(id) {
  try { sessionStorage.setItem(TAB_STORE, id); } catch { /* private mode */ }
}

/**
 * Open the chat panel for an assigned chat.
 * opts: { tab: initial tab ('' = last used this session), onTab(id) after the
 * user switches tabs (e.g. to update the URL), onClose() }.
 * Returns the drawer controller plus setTab(id).
 */
export function openChatDrawer(key, { onClose, tab, onTab } = {}) {
  const st = {
    tab: DRAWER_TABS.includes(tab) ? tab : (rememberedTab() || 'overview'),
    scroll: {}, // tab → scrollTop of the panel body
  };
  let drawer = null;

  const ctx = {
    key,
    update: () => drawer && drawer.update(),
    el: () => drawer && drawer.el,
    close: () => drawer && drawer.close(),
    setTab: (id) => setTab(id, { user: true }),
    notify(hook, data) {
      for (const t of Object.values(tabs)) if (typeof t[hook] === 'function') t[hook](data);
    },
    safePatch: (body) => patchChat(key, body).catch(() => {}),
    people: null,
    behaviour: null,
    away: null,
  };
  ctx.people = createPeopleSection({ key, update: ctx.update });

  // ── Away (snooze): used by the banners and the Behaviour tab ──
  async function setAway(until) {
    const iso = until == null ? null : (typeof until === 'string' ? until : until.toISOString());
    ctx.behaviour.setSnoozedLocal(iso);
    try {
      await patchChat(key, { snoozedUntil: iso });
      toast(iso ? `Away ${awayUntilText(iso)}` : 'Welcome back — replies are on again', { type: 'success' });
    } catch { /* toasted */ }
    ctx.behaviour.reloadSoon();
  }

  function awayMenu(e) {
    const c = chatByKey(key);
    const away = isAway(c);
    const h = (n) => new Date(Date.now() + n * 3600 * 1000);
    openMenu(e.currentTarget, [
      { heading: 'Away — no replies in this chat' },
      { label: 'For 1 hour', icon: 'clock', onClick: () => setAway(h(1)) },
      { label: 'For 3 hours', icon: 'clock', onClick: () => setAway(h(3)) },
      { label: 'Until tomorrow 9:00', icon: 'sun', onClick: () => setAway(tomorrowAt(9)) },
      { label: 'Until I turn it off', icon: 'snooze', onClick: () => setAway(FOREVER) },
      { label: 'Pick a time…', icon: 'calendar', onClick: async () => { const d = await pickAwayTime(); if (d) setAway(d); } },
      away ? 'sep' : null,
      away ? { label: "I'm back — end Away", icon: 'play', onClick: () => setAway(null) } : null,
    ], { align: 'end', width: 250 });
  }
  ctx.away = { set: setAway, menu: awayMenu };

  // Behaviour first: the other tabs read ctx.behaviour.
  ctx.behaviour = createBehaviourTab(ctx);
  const tabs = {
    overview: createOverviewTab(ctx),
    people: createPeopleTab(ctx),
    goal: createGoalTab(ctx),
    behaviour: ctx.behaviour,
    memory: createMemoryTab(ctx),
  };

  function body() { return drawer && drawer.el.querySelector('.drawer-body'); }

  function setTab(id, { user = false, focus = false } = {}) {
    if (!DRAWER_TABS.includes(id)) id = 'overview';
    if (id === st.tab) return;
    const b = body();
    if (b) st.scroll[st.tab] = b.scrollTop;
    st.tab = id;
    rememberTab(id);
    ctx.update();
    requestAnimationFrame(() => {
      const nb = body();
      if (nb) nb.scrollTop = st.scroll[id] || 0;
      if (focus) { const btn = drawer && drawer.el.querySelector(`#dtab-${id}`); if (btn) btn.focus(); }
      if (tabs[id] && tabs[id].shown) tabs[id].shown();
    });
    if (user && onTab) onTab(id);
  }

  function onTabKey(e) {
    const i = DRAWER_TABS.indexOf(st.tab);
    let next = null;
    if (e.key === 'ArrowRight') next = DRAWER_TABS[(i + 1) % DRAWER_TABS.length];
    else if (e.key === 'ArrowLeft') next = DRAWER_TABS[(i - 1 + DRAWER_TABS.length) % DRAWER_TABS.length];
    else if (e.key === 'Home') next = DRAWER_TABS[0];
    else if (e.key === 'End') next = DRAWER_TABS[DRAWER_TABS.length - 1];
    if (!next) return;
    e.preventDefault();
    setTab(next, { user: true, focus: true });
  }

  function tabBar(c) {
    const i = DRAWER_TABS.indexOf(st.tab);
    return html`<div class="drawer-tabs-wrap">
      <div class="drawer-tabs" role="tablist" aria-label="Chat settings" style=${`--n:${DRAWER_TABS.length};--i:${i}`} @keydown=${onTabKey}>
        ${DRAWER_TABS.map((id) => {
          const m = tabMeta(id, c);
          const on = id === st.tab;
          return html`<button type="button" role="tab" id=${'dtab-' + id} class="drawer-tab" data-tab=${id}
              aria-selected=${String(on)} aria-controls="dpanel" tabindex=${on ? '0' : '-1'}
              @click=${() => setTab(id, { user: true })}>${icon(m.icon)}<span>${m.label}</span></button>`;
        })}
      </div>
    </div>`;
  }

  drawer = openDrawer((ctl) => {
    const c = chatByKey(key);
    if (!c) {
      if (!store.state.chatsLoaded) return html`<div class="center" style="height:100%">${spinner('spinner-lg')}</div>`;
      return html`<div class="drawer-body">${emptyState({ artName: 'chats', title: 'This chat is no longer assigned', body: 'It may have been removed.', action: { label: 'Close', onClick: () => ctl.close() } })}</div>`;
    }
    const p = personaById(c.personaId);
    const isGroup = c.kind === 'group';
    const active = tabs[st.tab] || tabs.overview;

    return html`
      <div class="drawer-head">
        ${avatarStack({ jid: c.jid, name: c.name, kind: c.kind }, p, 48)}
        <div class="grow">
          <div class="h3 ellipsis">${c.name || c.jid}</div>
          <div class="small muted ellipsis">${isGroup ? 'Group' : 'Direct chat'} · ${p ? html`as <b>${p.name}</b>` : html`<span style="color:var(--red)">persona missing</span>`}${c.lastActivityAt ? html` · active ${relTime(c.lastActivityAt)}` : ''}</div>
        </div>
        ${toggle(c.enabled, (v) => ctx.safePatch({ enabled: v }), { label: c.enabled ? 'Replies on' : 'Replies off', title: 'Turn replies on or off for this chat' })}
        <button class="btn btn-ghost btn-icon" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
      </div>
      ${tabBar(c)}
      <div class="drawer-body">
        ${drawerBanners(ctx, c, p)}
        <div class="drawer-panel" id="dpanel" role="tabpanel" aria-labelledby=${'dtab-' + st.tab} data-key=${'panel-' + st.tab} tabindex="-1">
          ${active.view(c, p)}
        </div>
      </div>`;
  }, {
    label: 'Chat settings',
    noAutofocus: true,
    onClose: () => {
      for (const o of offs) o();
      for (const t of Object.values(tabs)) if (t.destroy) t.destroy();
      ctx.people.destroy();
      if (onClose) onClose();
    },
  });

  const fan = (type) => (data) => {
    for (const t of Object.values(tabs)) if (t.onBus) t.onBus(type, data);
    if (type === 'activity' && data && data.chatKey === key && data.type === 'incoming') ctx.people.reloadSoon(); // refreshes "spoke 1m ago"
  };
  const offs = ['activity', 'settings.changed', 'chats.changed', 'memories.changed', 'recaps.changed', 'missions.changed']
    .map((type) => bus.on(type, fan(type)));

  for (const t of Object.values(tabs)) if (t.load) t.load();
  ctx.people.load();
  loadBehaviorMeta().catch(() => {});

  drawer.setTab = (id) => setTab(id);
  drawer.tab = () => st.tab;
  return drawer;
}
