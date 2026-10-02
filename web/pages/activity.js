// pages/activity.js — live event log with filters, pause and expandable details.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadActivity } from '../store.js';
import { confirmSheet, toast, skeletonRows } from '../ui.js';
import { feedRow } from '../components/feed.js';
import { emptyState } from '../components/art.js';
import { ACT_GROUPS, actMeta } from '../components/status.js';

const SHOW_MAX = 300;

export default function Activity(ctx) {
  const update = () => ctx.update();
  const st = {
    group: 'all',
    chat: ctx.route.query.get('chat') || '',
    paused: false,
    snapshot: null,
    expanded: new Set(),
    seen: null,
  };

  function source() {
    return st.paused && st.snapshot ? st.snapshot : store.state.activity;
  }

  function filtered(list) {
    return list.filter((ev) => (st.group === 'all' || actMeta(ev.type).group === st.group) && (!st.chat || ev.chatKey === st.chat));
  }

  function chatOptions() {
    const map = new Map();
    for (const c of store.state.chats) map.set(c.key, c.name || c.key);
    for (const ev of store.state.activity) if (ev.chatKey && !map.has(ev.chatKey)) map.set(ev.chatKey, ev.chatName || ev.chatKey);
    return [...map.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }

  function togglePause() {
    st.paused = !st.paused;
    st.snapshot = st.paused ? store.state.activity.slice() : null;
    update();
  }

  async function clear() {
    const ok = await confirmSheet({ title: 'Clear the activity log?', body: 'This only clears the log. Chats, personas and memories are not affected.', confirm: 'Clear log', danger: true, iconName: 'trash' });
    if (!ok) return;
    await api.activity.clear();
    store.set({ activity: [] });
    st.snapshot = st.paused ? [] : null;
    toast('Activity cleared', { type: 'success' });
  }

  function view() {
    const s = store.state;
    const all = source();
    const list = filtered(all);
    const shown = list.slice(0, SHOW_MAX);
    let newCount = 0;
    if (st.paused && st.snapshot) {
      const ids = new Set(st.snapshot.map((x) => x.id));
      newCount = s.activity.filter((ev) => !ids.has(ev.id)).length;
    }
    if (!st.seen) st.seen = new Set(all.map((e) => e.id));
    const counts = {};
    for (const ev of all) { const g = actMeta(ev.type).group; counts[g] = (counts[g] || 0) + 1; }
    const opts = chatOptions();

    const rows = shown.map((ev) => {
      const isNew = !st.seen.has(ev.id);
      if (isNew) st.seen.add(ev.id);
      return feedRow(ev, {
        expanded: st.expanded.has(ev.id),
        isNew,
        onToggle: () => { if (st.expanded.has(ev.id)) st.expanded.delete(ev.id); else st.expanded.add(ev.id); update(); },
      });
    });

    return html`<div class="activity-page">
      <div class="toolbar">
        <div class="chip-row" role="toolbar" aria-label="Filter by type">
          ${ACT_GROUPS.map((g) => html`<button class="chip" aria-pressed=${String(st.group === g.value)} @click=${() => { st.group = g.value; update(); }}>
            ${g.value !== 'all' ? html`<span class="dot dot-sm" style=${`--c:${groupColor(g.value)}`}></span>` : ''}${g.label}
            ${g.value !== 'all' && counts[g.value] ? html`<span class="faint">${counts[g.value]}</span>` : ''}
          </button>`)}
        </div>
        <span class="grow"></span>
        <select class="select input-sm" style="width:auto;max-width:220px" aria-label="Filter by chat" @change=${(e) => { st.chat = e.target.value; update(); }}>
          <option value="" ?selected=${!st.chat}>All chats</option>
          ${opts.map(([k, n]) => html`<option value=${k} ?selected=${k === st.chat}>${n}</option>`)}
        </select>
        <button class=${'btn btn-sm ' + (st.paused ? 'btn-primary' : 'btn-glass')} @click=${togglePause}>
          ${icon(st.paused ? 'play' : 'pause')}${st.paused ? 'Resume' : 'Pause'}
        </button>
        <button class="btn btn-ghost btn-sm" ?disabled=${!s.activity.length} @click=${clear}>${icon('trash')}Clear</button>
      </div>

      ${st.paused ? html`<button class="paused-bar" @click=${togglePause}>
        ${icon('pause', 'ic-sm')}<span>Paused${newCount ? html` — <b>${newCount} new event${newCount === 1 ? '' : 's'}</b>` : ''}</span><span class="grow"></span><span class="link-btn">Resume live</span>
      </button>` : ''}

      <div class="card list-card">
        ${!s.activityLoaded ? skeletonRows(6)
          : !list.length ? (all.length
            ? emptyState({ artName: 'search', small: true, title: 'Nothing matches these filters', body: 'Try a different type or chat.', action: { label: 'Show everything', cls: 'btn-glass', onClick: () => { st.group = 'all'; st.chat = ''; update(); } } })
            : emptyState({ artName: 'activity', title: 'All quiet for now', body: 'Every incoming message, decision and reply shows up here live — handy to see why a persona did (or didn\'t) answer.' }))
          : html`<div class="feed">${rows}</div>
            ${list.length > SHOW_MAX ? html`<div class="list-end faint small">Showing the latest ${SHOW_MAX} of ${list.length}</div>` : ''}`}
      </div>
    </div>`;
  }

  function groupColor(g) {
    return { incoming: 'var(--blue)', decisions: 'var(--teal)', timing: 'var(--violet)', sent: 'var(--green)', approvals: 'var(--yellow)', blocked: 'var(--red)', errors: 'var(--orange)', system: 'var(--indigo)' }[g] || 'var(--gray)';
  }

  return {
    title: 'Activity',
    subtitle: () => (st.paused ? 'Paused' : 'Live'),
    view,
    mount() { loadActivity().catch(() => {}); },
    onParams(route) { const c = route.query.get('chat'); if (c !== null) { st.chat = c; } },
  };
}
