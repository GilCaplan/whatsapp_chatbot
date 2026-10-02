// pages/dashboard.js — status at a glance: chats that need you (hand-off),
// WhatsApp, active chats, live feed and today's recaps.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, personaById, isAway, bus } from '../store.js';
import { toggle, toast, busy } from '../ui.js';
import { avatarStack, chatAvatar } from '../components/avatar.js';
import { feedRow } from '../components/feed.js';
import { emptyState } from '../components/art.js';
import { waMeta, statusDot } from '../components/status.js';
import { openWASheet, logoutAndRelink } from '../components/wa-sheet.js';
import { openAssignSheet } from '../components/assign-sheet.js';
import { openAIBuilder } from '../components/ai-builder.js';
import { patchChat, awayUntilText } from '../components/chat-drawer.js';
import { greeting, duration, toDate, prettyPhone } from '../util.js';
import { catGlyph, catLabel, catReason, catPhrase, waLink, resumeChat, modeOf } from '../components/handoff-meta.js';
import { recapCard, generateRecap } from '../components/recap-card.js';

export default function Dashboard(ctx) {
  const go = (p) => ctx.navigate(p);
  const rc = { items: [], loaded: false, busy: false };
  let offRecaps = null;

  async function loadRecaps() {
    try {
      const res = await api.recaps.list({ limit: 30 }, { quiet: true });
      rc.items = (res && res.items) || [];
    } catch { /* keep */ }
    rc.loaded = true;
    ctx.update();
  }

  // ── Needs you (hand-off) ──
  function needsStrip() {
    const list = store.state.chats.filter((c) => c.handoff);
    if (!list.length) return '';
    return html`<section class="card needs-strip needs-banner" aria-label="Chats that need you">
      <div class="card-head">
        <h2 class="h3"><span class="ns-dot" aria-hidden="true"></span>Needs you</h2>
        <span class="small muted">${list.length === 1 ? 'A persona paused and is waiting for you' : `${list.length} personas paused and are waiting for you`}</span>
      </div>
      ${list.map((c) => {
        const p = personaById(c.personaId);
        const h = c.handoff;
        const link = waLink(c);
        const open = () => go(`/chats/${encodeURIComponent(c.key)}`);
        return html`<div class="ns-row" data-key=${c.key} tabindex="0" role="link" @click=${open} @keydown=${(e) => { if (e.key === 'Enter') open(); }}>
          <span class="nb-badge" title=${catLabel(h.category)}>${catGlyph(h.category)}</span>
          <div class="ns-text">
            <div class="ns-why">${c.kind === 'group'
              ? html`<b>${c.name || c.jid}</b> · ${catReason(h.category, h.sender)}`
              : html`<b>${h.sender || c.name || c.jid}</b> ${catPhrase(h.category)}`} · <rel-time datetime=${h.at}></rel-time></div>
            ${h.excerpt ? html`<div class="ns-ex">“${h.excerpt}”</div>` : ''}
          </div>
          <div class="ns-actions" @click=${(e) => e.stopPropagation()}>
            ${link ? html`<a class="btn btn-glass btn-sm" href=${link} target="_blank" rel="noopener">${icon('external')}WhatsApp</a>` : ''}
            <button class="btn btn-primary btn-sm" @click=${busy(() => resumeChat(c.key, p && p.name))}>${icon('play')}Resume</button>
          </div>
        </div>`;
      })}
    </section>`;
  }

  // ── Daily recap ──
  function recapSection() {
    const s = store.state.settings || {};
    const daily = s.recap && s.recap.enabled;
    const since = Date.now() - 24 * 3600 * 1000;
    const seen = new Set();
    const today = rc.items.filter((r) => {
      if (Date.parse(r.generatedAt || 0) < since || seen.has(r.chatKey)) return false;
      seen.add(r.chatKey);
      return true;
    });
    if (!rc.loaded) return '';
    if (!today.length && !daily && !store.state.chats.length) return '';
    return html`<section class="card dash-recap">
      <div class="card-head">
        <h2 class="h3">Daily recap</h2>
        <span class="small muted">${daily ? `every day at ${s.recap.time}` : 'on demand'}</span>
        <span class="grow"></span>
        <button class=${'btn btn-ghost btn-sm ' + (rc.busy ? 'loading' : '')} ?disabled=${rc.busy} @click=${async () => {
          rc.busy = true; ctx.update();
          await generateRecap();
          rc.busy = false; loadRecaps();
        }}>${icon('sparkles')}Recap now</button>
      </div>
      ${today.length ? html`<div class="recap-grid">${today.map((r) => recapCard(r, {
        showChat: true, compact: true,
        onOpen: () => go(`/chats/${encodeURIComponent(r.chatKey)}/memory`),
      }))}</div>`
        : html`<p class="small muted">${daily
          ? `Nothing yet today. Around ${s.recap.time} you'll get a few lines per chat: what was said, plans made and what to follow up on.`
          : html`Get a few lines per chat at the end of the day: what was said, plans made and what to follow up on. Tap Recap now, or switch it on in <a href="#/settings">Settings</a>.`}</p>`}
    </section>`;
  }

  function waCard() {
    const wa = store.state.wa || {};
    const m = waMeta(wa.state);
    const me = wa.me || {};
    const connected = wa.state === 'connected';
    const since = toDate(wa.since);
    return html`<section class="card wa-card">
      <div class="row gap-14">
        ${connected
          ? chatAvatar({ jid: me.jid, name: me.pushName || 'Me', src: me.avatarUrl }, 56)
          : html`<span class="wa-icon" style=${`--c:${m.color}`}>${icon('phone', 'ic-lg')}</span>`}
        <div class="grow">
          <div class="eyebrow">WhatsApp</div>
          <div class="h3 ellipsis">${connected ? (me.pushName || 'Connected') : m.label}</div>
          <div class="small muted row gap-6">${statusDot(wa.state)}<span class="ellipsis">${connected
            ? html`${prettyPhone(me.phone) || 'Linked'}${since ? html` · up ${duration((Date.now() - since) / 1000)}` : ''}`
            : m.help}</span></div>
        </div>
      </div>
      ${wa.lastError && !connected ? html`<div class="small faint mt-8 clamp-2">${wa.lastError}</div>` : ''}
      <div class="row gap-8 mt-16 row-wrap">
        ${connected ? html`
          <button class="btn btn-glass btn-sm" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting…'); })}>${icon('refresh')}Reconnect</button>
          <button class="btn btn-glass btn-sm" @click=${() => openWASheet()}>${icon('qr')}Relink</button>
          <button class="btn btn-danger btn-sm" @click=${() => logoutAndRelink()}>${icon('logout')}Log out</button>
        ` : html`
          <button class="btn btn-primary btn-sm" @click=${() => openWASheet()}>${icon('qr')}${wa.state === 'awaiting_qr' || wa.state === 'logged_out' ? 'Scan QR code' : 'Open WhatsApp panel'}</button>
          <button class="btn btn-glass btn-sm" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting…'); })}>${icon('refresh')}Reconnect</button>
        `}
      </div>
    </section>`;
  }

  function stats() {
    const s = store.state;
    const active = s.chats.filter((c) => c.enabled).length;
    const today = new Date(); today.setHours(0, 0, 0, 0);
    const sent = s.activity.filter((a) => (a.type === 'sent' || a.type === 'approval.sent') && new Date(a.ts) >= today).length;
    const blocked = s.activity.filter((a) => a.type === 'blocked_injection' && new Date(a.ts) >= today).length;
    const needs = s.chats.filter((c) => c.handoff).length;
    const tile = (n, label, ic, color, href) => html`<a class="stat" href=${href} style=${`--c:${color}`}>
      <span class="stat-icon">${icon(ic)}</span>
      <span class="stat-n">${n}</span>
      <span class="stat-l">${label}</span>
    </a>`;
    return html`<section class="card stats-card">
      ${tile(active, active === 1 ? 'chat answering' : 'chats answering', 'chats', 'var(--teal)', '#/chats?tab=assigned')}
      ${tile(s.approvals.length, 'waiting for you', 'inbox', s.approvals.length ? 'var(--orange)' : 'var(--gray)', '#/approvals')}
      ${tile(sent, sent === 1 ? 'reply sent today' : 'replies sent today', 'send', 'var(--green)', '#/activity')}
      ${needs ? tile(needs, needs === 1 ? 'chat needs you' : 'chats need you', 'hand', 'var(--red)', '#/chats?tab=assigned')
        : tile(blocked, 'tricks blocked today', 'shield', 'var(--violet)', '#/activity')}
    </section>`;
  }

  function chatCards() {
    const s = store.state;
    const chats = s.chats.slice().sort((a, b) => (Date.parse(b.lastActivityAt || b.createdAt || 0) || 0) - (Date.parse(a.lastActivityAt || a.createdAt || 0) || 0));
    return html`<section class="card chats-card">
      <div class="card-head">
        <h2 class="h3">Active chats</h2>
        <span class="grow"></span>
        ${chats.length ? html`<button class="btn btn-ghost btn-sm" @click=${() => openAssignSheet()}>${icon('plus')}Assign</button>
          <a class="btn btn-ghost btn-sm" href="#/chats?tab=assigned">See all${icon('chevron-right')}</a>` : ''}
      </div>
      ${!s.chatsLoaded ? html`<div class="chat-cards">${[0, 1, 2].map(() => html`<div class="chat-card skeleton" style="height:118px"></div>`)}</div>`
        : !chats.length ? emptyState({
          artName: 'chats', small: true,
          title: 'No chats assigned yet',
          body: 'Pick a WhatsApp chat and a persona to answer it. Start with “You (message yourself)” to test safely.',
          action: { label: 'Assign a chat', icon: 'plus', onClick: () => openAssignSheet() },
        })
        : html`<div class="chat-cards stagger">${chats.slice(0, 9).map((c) => {
          const p = personaById(c.personaId);
          const pending = c.pendingCount != null ? c.pendingCount : s.approvals.filter((a) => a.chatKey === c.key).length;
          return html`<div class="chat-card clickable" data-key=${c.key} tabindex="0" role="link"
              @click=${() => go(`/chats/${encodeURIComponent(c.key)}`)}
              @keydown=${(e) => { if (e.key === 'Enter') go(`/chats/${encodeURIComponent(c.key)}`); }}>
            <div class="row gap-12">
              ${avatarStack({ jid: c.jid, name: c.name, kind: c.kind }, p, 44)}
              <div class="grow">
                <div class="title ellipsis">${c.name || c.jid}</div>
                <div class="sub ellipsis">${p ? html`as ${p.name}` : 'persona missing'}</div>
              </div>
              ${toggle(c.enabled, (v) => patchChat(c.key, { enabled: v }).catch(() => {}), { small: true, label: 'Replies on' })}
            </div>
            <div class="row gap-8 mt-12 small">
              ${c.handoff ? html`<span class="chip chip-sm chip-red">${icon('hand')}Needs you</span>`
                : c.revealedAt && !c.enabled ? html`<span class="chip chip-sm chip-violet">${icon('eye')}Revealed</span>`
                : modeOf(c) === 'copilot' ? html`<span class="chip chip-sm chip-violet">${icon('sparkles')}Co-pilot</span>`
                : modeOf(c) === 'approve' ? html`<span class="chip chip-sm">${icon('shield')}Approve</span>`
                : html`<span class="chip chip-sm chip-green">${icon('bolt')}Auto</span>`}
              ${c.kind === 'group' ? html`<span class="chip chip-sm">${icon('group')}Group</span>` : ''}
              ${isAway(c) ? html`<span class="chip chip-sm chip-away" title=${`Away ${awayUntilText(c.snoozedUntil)}`}>${icon('snooze')}Away</span>` : ''}
              <span class="grow"></span>
              ${pending ? html`<span class="badge" title="Waiting for approval">${pending}</span>` : ''}
              <span class="faint">${c.lastActivityAt ? html`<rel-time datetime=${c.lastActivityAt}></rel-time>` : 'no activity yet'}</span>
            </div>
          </div>`;
        })}</div>`}
    </section>`;
  }

  function feed() {
    const items = store.state.activity.slice(0, 8);
    return html`<section class="card feed-card">
      <div class="card-head">
        <h2 class="h3">Live feed</h2>
        <span class="live-dot" title="Live"></span>
        <span class="grow"></span>
        <a class="btn btn-ghost btn-sm" href="#/activity">Open${icon('chevron-right')}</a>
      </div>
      ${items.length ? html`<div class="feed compact">${items.map((ev) => feedRow(ev, { compact: true }))}</div>`
        : emptyState({ artName: 'activity', small: true, title: 'All quiet', body: 'Messages, decisions and replies will stream in here as they happen.' })}
    </section>`;
  }

  function quick() {
    return html`<section class="quick">
      <button class="quick-btn" @click=${() => openAssignSheet()}><span class="q-ic" style="--c:var(--teal)">${icon('link')}</span><span>Assign a chat</span></button>
      <button class="quick-btn" @click=${() => openAIBuilder()}><span class="q-ic" style="--c:var(--violet)">${icon('sparkles')}</span><span>Build persona with AI</span></button>
      <button class="quick-btn" @click=${() => go('/playground')}><span class="q-ic" style="--c:var(--pink)">${icon('playground')}</span><span>Try in Playground</span></button>
      <button class="quick-btn" @click=${() => go('/persona/new')}><span class="q-ic" style="--c:var(--blue)">${icon('plus')}</span><span>New persona</span></button>
    </section>`;
  }

  function view() {
    const s = store.state;
    const me = s.wa && s.wa.me;
    const name = me && me.pushName ? me.pushName.split(' ')[0] : '';
    return html`<div class="dash">
      <section class="dash-hero">
        <div>
          <h2 class="hero-title">${greeting()}${name ? html`, <span class="grad-text">${name}</span>` : ''}</h2>
          <p class="muted">${s.approvals.length
            ? html`${s.approvals.length} repl${s.approvals.length === 1 ? 'y is' : 'ies are'} waiting for your OK. <a href="#/approvals">Review now →</a>`
            : s.chats.some((c) => c.enabled) ? 'Your personas are on duty. Here’s what’s happening.' : 'Assign a persona to a chat to get started.'}</p>
        </div>
        ${quick()}
      </section>
      ${needsStrip()}
      <div class="dash-grid">
        ${waCard()}
        ${stats()}
        ${chatCards()}
        ${feed()}
      </div>
      ${recapSection()}
    </div>`;
  }

  return {
    title: 'Dashboard',
    view,
    mount() {
      loadRecaps();
      offRecaps = bus.on('recaps.changed', loadRecaps);
    },
    unmount() { if (offRecaps) offRecaps(); },
  };
}
