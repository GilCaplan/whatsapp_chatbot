// recap-card.js — the daily recap: one card per chat and day (headline,
// topics, things to know, goal progress), the chat panel's recap block
// (Memory tab) and a shared "Recap now" action.
//
//   recapCard(recap, { showChat, onOpen, compact })
//   recapBlock(ctx, chat, persona)   // Memory tab: latest recap, earlier ones, "Recap now"
//   generateRecap(chatKey?)          // → [Recap] (toasts the outcome)

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, bus } from '../store.js';
import { toast, spinner } from '../ui.js';
import { clockTime } from '../util.js';

/** "Today", "Yesterday" or "Wed 30 Sep" for a recap's date (YYYY-MM-DD). */
export function recapDay(date) {
  if (!date) return '';
  const [y, m, d] = String(date).split('-').map(Number);
  const day = new Date(y, (m || 1) - 1, d || 1);
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const diff = Math.round((today - day) / 86400000);
  if (diff === 0) return 'Today';
  if (diff === 1) return 'Yesterday';
  return day.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
}

export function recapCard(r, { showChat = false, onOpen = null, compact = false } = {}) {
  const topics = r.topics || [];
  const know = r.toKnow || [];
  const open = onOpen ? () => onOpen(r) : null;
  return html`<article class=${'recap-card ' + (compact ? 'compact ' : '') + (open ? 'clickable' : '')} data-key=${r.id}
      tabindex=${open ? '0' : '-1'} role=${open ? 'link' : 'article'}
      @click=${open || (() => {})} @keydown=${(e) => { if (open && e.key === 'Enter') open(); }}>
    <div class="recap-top">
      ${showChat ? html`<span class="rc-chat ellipsis">${r.chatName || 'Chat'}</span>` : ''}
      <span class="rc-meta">${recapDay(r.date)}${r.generatedAt ? ' ' + clockTime(r.generatedAt) : ''} · ${r.messageCount === 1 ? '1 message' : `${r.messageCount || 0} messages`}</span>
      ${r.mood ? html`<span class="grow"></span><span class="chip chip-sm chip-blue">${r.mood}</span>` : ''}
    </div>
    <div class="recap-head">${r.headline}</div>
    ${topics.length && !compact ? html`<div class="recap-topics">${topics.map((t) => html`<span class="chip chip-sm">${t}</span>`)}</div>` : ''}
    ${know.length ? html`<ul class="recap-know">${(compact ? know.slice(0, 2) : know).map((k) => html`<li>${icon('check', 'ic-sm')}<span>${k}</span></li>`)}</ul>` : ''}
    ${r.goalProgress && !compact ? html`<div class="recap-goal">${icon('target', 'ic-sm')}<span>${r.goalProgress}</span></div>` : ''}
  </article>`;
}

/** Writes recaps now (one chat, or every chat with something new). */
export async function generateRecap(chatKey) {
  try {
    const res = await api.recaps.generate(chatKey || undefined);
    const items = (res && res.items) || [];
    if (!items.length) toast(chatKey ? 'Nothing to recap in this chat yet' : 'Nothing new to recap today', { type: 'info' });
    else toast(items.length === 1 ? 'Recap ready' : `${items.length} recaps ready`, { type: 'success' });
    return items;
  } catch { return null; }
}

// ── Memory tab block ───────────────────────────────────────────────────
// State is kept per chat so the block survives re-renders; recaps.changed
// (SSE) refreshes every chat panel that showed one.

const blocks = new Map(); // chatKey → { items, loaded, loading, busy, update }

bus.on('recaps.changed', () => {
  for (const [key, b] of blocks) { b.loaded = false; load(key); }
});

async function load(key) {
  const b = blocks.get(key);
  if (!b || b.loading) return;
  b.loading = true;
  try {
    const res = await api.recaps.list({ chat: key, limit: 8 }, { quiet: true });
    b.items = (res && res.items) || [];
  } catch { /* keep what we had */ }
  b.loaded = true;
  b.loading = false;
  if (b.update) b.update();
}

export function recapBlock(ctx, c, p) {
  const key = c.key;
  let b = blocks.get(key);
  if (!b) { b = { items: [], loaded: false, loading: false, busy: false }; blocks.set(key, b); }
  b.update = ctx.update;
  if (!b.loaded && !b.loading) load(key);
  const s = store.state.settings || {};
  const daily = s.recap && s.recap.enabled;
  const [latest, ...older] = b.items;
  const name = p ? p.name : 'the persona';
  async function now() {
    if (b.busy) return;
    b.busy = true; ctx.update();
    const items = await generateRecap(key);
    if (items && items.length) { b.items = [items[0], ...b.items.filter((r) => r.id !== items[0].id && !(r.date === items[0].date))]; }
    b.busy = false; ctx.update();
  }
  return html`<section class="drawer-section" data-key="recap">
    <h4>${icon('text', 'ic-sm')}Daily recap <span class="grow"></span>
      <button class=${'btn btn-ghost btn-sm ' + (b.busy ? 'loading' : '')} ?disabled=${b.busy} @click=${now}>${icon('sparkles')}Recap now</button>
    </h4>
    ${!b.loaded ? html`<div class="center" style="padding:18px">${spinner()}</div>`
      : latest ? html`${recapCard(latest)}
        ${older.length ? html`<details class="recap-older"><summary>${icon('chevron-right')}Earlier recaps (${older.length})</summary>
          ${older.map((r) => recapCard(r, { compact: true }))}</details>` : ''}`
      : html`<div class="empty empty-sm"><p class="small">${daily
        ? `A short summary of what ${name} talked about here arrives every evening at ${s.recap.time}.`
        : html`No recap yet. Tap <b>Recap now</b> for a summary of today, or switch on the daily recap in <a href="#/settings">Settings › Memory &amp; recap</a>.`}</p></div>`}
  </section>`;
}
