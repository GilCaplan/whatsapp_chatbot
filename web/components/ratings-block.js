// ratings-block.js — the People tab's "Connection" section (groups): once a
// day with 20+ messages the persona privately scores, 0–100, how connected
// it felt to each person who wrote (GET /api/chats/{key}/ratings). Only you
// see it; nothing is sent to WhatsApp. "Rate now" scores the last 24 hours.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { seg, toast } from '../ui.js';
import { debounce, relTime } from '../util.js';
import { helpTip } from './help-tip.js';

export function createRatingsStore(key, update) {
  const st = { items: null, error: '', day: 0, rating: false };
  async function load() {
    try {
      const r = await api.chats.ratings(key, { quiet: true });
      st.items = (r && r.items) || [];
      st.error = '';
      st.day = Math.min(st.day, Math.max(0, st.items.length - 1));
    } catch (e) {
      if (!st.items) { st.items = []; st.error = (e && e.message) || "Couldn't load the ratings"; }
    }
    update();
  }
  async function rateNow() {
    st.rating = true; update();
    try {
      await api.chats.rateNow(key);
      st.day = 0;
      toast('Rated the last 24 hours', { type: 'success' });
    } catch { /* toasted */ }
    st.rating = false;
    load();
  }
  return { st, load, reloadSoon: debounce(load, 500), rateNow };
}

function dayLabel(date) {
  const d = new Date(date + 'T12:00:00');
  const today = new Date();
  const diff = Math.round((new Date(today.toDateString()) - new Date(d.toDateString())) / 86400000);
  if (diff === 0) return 'Today';
  if (diff === 1) return 'Yesterday';
  return d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
}

function change(ch) {
  if (ch == null) return html`<span class="rt-change new" title="First rating">new</span>`;
  if (ch === 0) return html`<span class="rt-change" title="Same as last time">=</span>`;
  return html`<span class=${'rt-change ' + (ch > 0 ? 'up' : 'down')} title="Since the last rating">${ch > 0 ? '+' : '−'}${Math.abs(ch)}</span>`;
}

export function ratingsBlock(rs, c, p) {
  const st = rs.st;
  const name = p ? p.name : 'The persona';
  const items = st.items;
  const d = items && items[st.day];
  return html`<section class="drawer-section" data-key="ratings">
    <h4>${icon('heart', 'ic-sm')}Connection ${helpTip('ratings')}<span class="grow"></span>
      <button class=${'btn btn-ghost btn-sm ' + (st.rating ? 'loading' : '')} ?disabled=${st.rating} @click=${rs.rateNow}
        title="Score the last 24 hours now">${icon('refresh')}Rate now</button>
    </h4>
    <div class="field-help mb-8">How connected ${name} felt to each person, 0 to 100, in its own words. Updated every day the group has at least 20 messages. Only you see this.</div>
    ${items == null ? html`<div class="skeleton" style="height:64px;border-radius:16px"></div>`
      : !items.length ? html`<div class="small muted">No ratings yet. They appear after a day with at least 20 messages, or tap Rate now.</div>`
      : html`
        ${items.length > 1 ? html`<div class="mb-8">${seg(items.slice(0, 5).map((x, i) => ({ value: i, label: dayLabel(x.date) })), st.day,
          (v) => { st.day = v; rs.load(); }, { label: 'Day', cls: 'seg-sm' })}</div>` : ''}
        <div class="tiny faint mb-8">${dayLabel(d.date)} · from ${d.messages} messages · ${relTime(d.at)}${d.onDemand ? ' · rated on request' : ''}</div>
        <ol class="rt-list">
          ${d.scores.map((s, i) => html`<li class="rt-row">
            <span class="rt-rank">${i + 1}</span>
            <div class="grow rt-main">
              <div class="rt-top"><b class="ellipsis">${s.name}</b><span class="grow"></span>${change(s.change)}<span class="rt-score">${s.score}</span></div>
              <div class="rt-track"><div class="rt-fill" style=${`width:${Math.max(2, s.score)}%`}></div></div>
              ${s.why ? html`<div class="tiny rt-why">${s.why}</div>` : ''}
            </div>
          </li>`)}
        </ol>`}
  </section>`;
}
