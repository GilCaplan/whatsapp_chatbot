// drawer/log.js — the chat panel's "Log" tab: what the persona did about
// every message in this chat, why, and how long it took (GET /api/chats/{key}/log,
// kept by the engine). On top: a summary for the chosen time range — how
// many it answered, typical reply time, why it stayed quiet, who gets
// answers and how fast replies come; below: one row per message with its
// decision and, on a tap, the steps on the way.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { seg, toast, confirmSheet, skeletonRows } from '../../ui.js';
import { relTime, clockTime, duration, debounce, toDate } from '../../util.js';
import { emptyState } from '../art/art.js';
import { helpTip } from '../common/help-tip.js';
import { REASONS } from '../common/feed.js';

// How each outcome looks: label, colour token, icon.
const OUTCOMES = {
  replied: { label: 'Answered', c: 'var(--green)', icon: 'reply' },
  opener: { label: 'Wrote first', c: 'var(--teal)', icon: 'megaphone' },
  skipped: { label: 'Stayed quiet', c: 'var(--gray)', icon: 'eye' },
  reacted: { label: 'Reacted', c: 'var(--yellow)', icon: 'heart' },
  paused: { label: 'Left for you', c: 'var(--orange)', icon: 'hand' },
  blocked: { label: 'Blocked', c: 'var(--red)', icon: 'shield' },
  approval: { label: 'Waiting for you', c: 'var(--blue)', icon: 'approvals' },
  failed: { label: "Couldn't send", c: 'var(--red)', icon: 'warning' },
  pending: { label: 'On it', c: 'var(--teal)', icon: 'hourglass' },
  stalled: { label: 'No reply', c: 'var(--gray)', icon: 'minus' },
};

// A message still "pending" this long after its last step was dropped
// (app restarted mid-reply, or it ran out of time).
const STALLED_MS = 30 * 60 * 1000;

// Reply-time buckets (seconds, upper bound).
const SPEED_BUCKETS = [
  { max: 30, label: 'Under 30s' },
  { max: 120, label: '30s to 2m' },
  { max: 600, label: '2 to 10m' },
  { max: 3600, label: '10m to 1h' },
  { max: Infinity, label: 'Over 1h' },
];

const RANGES = [
  { value: 'day', label: 'Today', ms: 24 * 3600 * 1000 },
  { value: 'week', label: '7 days', ms: 7 * 24 * 3600 * 1000 },
  { value: 'all', label: 'All', ms: Infinity },
];

const STEP_ICONS = {
  decision: 'sparkles', noticing: 'eye', seen: 'check', waiting: 'hourglass', thinking: 'brain', generating: 'wand',
  typing: 'typing', sent: 'send', deferred: 'clock', reacted: 'heart', 'approval.sent': 'check', 'approval.discarded': 'x',
};

/** The outcome shown for an entry (old pending entries read "No reply"). */
function outcomeOf(d, now = Date.now()) {
  if (d.kind === 'opener') return 'opener';
  if (d.outcome !== 'pending') return d.outcome;
  const last = d.steps && d.steps.length ? d.steps[d.steps.length - 1].at : d.at;
  return now - (toDate(last) || 0) > STALLED_MS ? 'stalled' : 'pending';
}

function median(xs) {
  if (!xs.length) return 0;
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

/** Summary numbers for the entries in range (newest first). */
export function logStats(items, now = Date.now()) {
  const msgs = items.filter((d) => d.kind !== 'opener');
  const replied = msgs.filter((d) => d.outcome === 'replied');
  const delays = replied.map((d) => d.delaySeconds || 0);
  const quiet = new Map();
  const people = new Map();
  for (const d of msgs) {
    const o = outcomeOf(d, now);
    if (o !== 'replied' && o !== 'pending') {
      const k = o === 'skipped' ? (d.reason || 'other') : o;
      quiet.set(k, (quiet.get(k) || 0) + 1);
    }
    const who = d.sender || 'Someone';
    const p = people.get(who) || { name: who, total: 0, replied: 0 };
    p.total++;
    if (d.outcome === 'replied') p.replied++;
    people.set(who, p);
  }
  const speed = SPEED_BUCKETS.map((b) => ({ ...b, n: 0 }));
  for (const s of delays) speed.find((b) => s < b.max).n++;
  return {
    messages: msgs.length,
    replied: replied.length,
    openers: items.length - msgs.length,
    rate: msgs.length ? Math.round((replied.length / msgs.length) * 100) : 0,
    median: median(delays),
    longest: delays.length ? Math.max(...delays) : 0,
    quiet: [...quiet.entries()].map(([k, n]) => ({ k, n })).sort((a, b) => b.n - a.n),
    people: [...people.values()].sort((a, b) => b.total - a.total),
    speed,
  };
}

function quietLabel(k) {
  if (OUTCOMES[k]) return OUTCOMES[k].label;
  return REASONS[k] || 'Other';
}

export function createLogTab(ctx) {
  const ui = { items: null, error: '', range: 'week', filter: 'all', open: new Set(), shown: false };

  async function load() {
    try {
      const r = await api.chats.log(ctx.key, { quiet: true });
      ui.items = (r && r.items) || [];
      ui.error = '';
    } catch (err) {
      ui.error = (err && err.message) || "Couldn't load the log";
      if (!ui.items) ui.items = [];
    }
    ctx.update();
  }
  const reloadSoon = debounce(load, 700);

  async function clear() {
    const ok = await confirmSheet({
      title: 'Clear the log?', body: 'This forgets the decisions logged for this chat. The messages themselves stay.',
      confirm: 'Clear', danger: true, iconName: 'trash',
    });
    if (!ok) return;
    try { await api.chats.clearLog(ctx.key); toast('Log cleared', { type: 'success' }); } catch { /* toasted */ }
    ui.open.clear();
    load();
  }

  function toggleOpen(id) {
    if (ui.open.has(id)) ui.open.delete(id); else ui.open.add(id);
    ctx.update();
  }

  function tile(n, label, ic, c, title = '') {
    return html`<div class="stat log-stat" title=${title}><span class="stat-icon" style=${`--c:${c}`}>${icon(ic)}</span>
      <span class="stat-n">${n}</span><span class="stat-l">${label}</span></div>`;
  }

  /** Horizontal bars: rows of { label, n, c, sub? } against max. */
  function bars(rows, max) {
    return html`<div class="log-bars">${rows.map((r) => html`<div class="log-bar">
      <div class="log-bar-label ellipsis" title=${r.label}>${r.label}</div>
      <div class="log-bar-track"><div class="log-bar-fill" style=${`--c:${r.c};width:${max && r.n ? Math.max(4, (r.n / max) * 100) : 0}%`}></div></div>
      <div class="log-bar-n">${r.sub || r.n}</div>
    </div>`)}</div>`;
  }

  function summary(items, c, name) {
    const s = logStats(items);
    if (!s.messages && !s.openers) return '';
    const isGroup = c.kind === 'group';
    const quietMax = s.quiet.length ? s.quiet[0].n : 0;
    const speedMax = Math.max(0, ...s.speed.map((b) => b.n));
    const people = s.people.slice(0, 8);
    return html`
      <div class="log-stats">
        ${tile(s.messages, s.messages === 1 ? 'message to it' : 'messages to it', 'message', 'var(--blue)')}
        ${tile(s.messages ? `${s.rate}%` : '-', `answered (${s.replied})`, 'reply', 'var(--green)')}
        ${tile(s.replied ? duration(s.median) : '-', 'typical reply time', 'timer', 'var(--teal)', 'Half of the replies came faster than this')}
        ${tile(s.replied ? duration(s.longest) : '-', 'longest wait', 'hourglass', 'var(--orange)')}
      </div>
      ${s.openers ? html`<div class="tiny faint log-openers">${icon('megaphone', 'ic-sm')}${name} also wrote first ${s.openers} time${s.openers === 1 ? '' : 's'}.</div>` : ''}
      <div class="log-grid">
        ${s.quiet.length ? html`<div class="log-card">
          <div class="log-card-h">Why it didn't answer</div>
          ${bars(s.quiet.map((q) => ({ label: quietLabel(q.k), n: q.n, c: (OUTCOMES[q.k] || OUTCOMES.skipped).c })), quietMax)}
        </div>` : ''}
        ${s.replied ? html`<div class="log-card">
          <div class="log-card-h">How fast it answers</div>
          ${bars(s.speed.map((b) => ({ label: b.label, n: b.n, c: 'var(--teal)' })), speedMax)}
        </div>` : ''}
        ${isGroup && people.length > 1 ? html`<div class="log-card">
          <div class="log-card-h">Who it answers</div>
          ${bars(people.map((p) => ({ label: p.name, n: p.replied, c: 'var(--green)', sub: `${p.replied} of ${p.total}` })),
            Math.max(...people.map((p) => p.total)))}
        </div>` : ''}
      </div>`;
  }

  function steps(d) {
    const t0 = toDate(d.at);
    return html`<ol class="log-steps">
      <li><span class="log-step-t">${clockTime(d.at)}</span>${icon('message', 'ic-sm')}<span>${d.kind === 'opener' ? 'Started writing' : 'Message sent to the chat'}</span></li>
      ${(d.steps || []).map((st) => {
        const off = t0 ? ((toDate(st.at) - t0) / 1000) : 0;
        return html`<li><span class="log-step-t" title=${clockTime(st.at)}>+${duration(off)}</span>${icon(STEP_ICONS[st.type] || 'info', 'ic-sm')}<span>${st.text}</span></li>`;
      })}
    </ol>`;
  }

  function row(d, name) {
    const o = outcomeOf(d);
    const meta = OUTCOMES[o] || OUTCOMES.skipped;
    const open = ui.open.has(d.id);
    const opener = d.kind === 'opener';
    let why = d.why || '';
    if (o === 'stalled') why = why ? `${why} — then nothing was sent (the app may have restarted)` : 'Nothing was sent (the app may have restarted)';
    if (o === 'pending' && !why) why = `${name} is on it`;
    return html`<li class=${'log-row' + (open ? ' open' : '')} data-key=${d.id}>
      <button type="button" class="log-row-main" aria-expanded=${String(open)} @click=${() => toggleOpen(d.id)}>
        <span class="log-dot" style=${`--c:${meta.c}`}>${icon(meta.icon, 'ic-sm')}</span>
        <span class="grow log-row-body">
          <span class="log-row-top">
            <b class="ellipsis">${opener ? name : (d.sender || 'Someone')}</b>
            <span class="chip chip-sm log-chip" style=${`--c:${meta.c}`}>${meta.label}</span>
            ${d.repliedAt && !opener ? html`<span class="tiny log-delay" title="From their message to the first reply">${icon('timer', 'ic-sm')}${duration(d.delaySeconds || 0)}</span>` : ''}
            <span class="grow"></span>
            <span class="tiny faint" title=${toDate(d.at) ? toDate(d.at).toLocaleString() : ''}>${relTime(d.at)}</span>
          </span>
          ${opener ? '' : html`<span class="log-text">${d.text || html`<i class="faint">(no text)</i>`}</span>`}
          ${why ? html`<span class="tiny log-why">${why}</span>` : ''}
          ${d.reply ? html`<span class="log-reply">${icon('reply', 'ic-sm')}<span>${d.reply}</span></span>` : ''}
        </span>
      </button>
      ${open ? steps(d) : ''}
    </li>`;
  }

  function view(c, p) {
    const name = p ? p.name : 'The persona';
    if (ui.items == null) return html`<section class="drawer-section">${skeletonRows(4)}</section>`;
    const now = Date.now();
    const range = RANGES.find((r) => r.value === ui.range) || RANGES[1];
    const inRange = ui.items.filter((d) => now - (toDate(d.at) || 0) <= range.ms);
    const groups = {
      all: () => true,
      answered: (d) => d.kind === 'opener' || d.outcome === 'replied',
      quiet: (d) => !['replied', 'pending'].includes(outcomeOf(d, now)) && d.kind !== 'opener',
    };
    const counts = Object.fromEntries(Object.entries(groups).map(([k, f]) => [k, inRange.filter(f).length]));
    const shown = inRange.filter(groups[ui.filter] || groups.all);
    return html`
      <section class="drawer-section" data-key="log">
        <h4>${icon('activity', 'ic-sm')}What ${name} did here ${helpTip('log')}<span class="grow"></span>
          ${ui.items.length ? html`<button class="btn btn-ghost btn-sm" title="Forget the logged decisions" @click=${clear}>${icon('trash')}Clear</button>` : ''}
        </h4>
        ${ui.error ? html`<div class="tiny" style="color:var(--red)">${ui.error}</div>` : ''}
        ${!ui.items.length ? emptyState({
          artName: 'activity', small: true, title: 'Nothing logged yet',
          body: `When someone writes here, this shows whether ${name} answered, why or why not, and how long it took.`,
        }) : html`
          <div class="log-controls">
            ${seg(RANGES.map(({ value, label }) => ({ value, label })), ui.range, (v) => { ui.range = v; ctx.update(); }, { label: 'Time range' })}
          </div>
          ${inRange.length ? summary(inRange, c, name) : html`<div class="small muted">Nothing in this time range.</div>`}
          ${inRange.length ? html`
            <div class="log-controls">
              ${seg([
                { value: 'all', label: 'All', count: counts.all },
                { value: 'answered', label: 'Answered', count: counts.answered },
                { value: 'quiet', label: 'Not answered', count: counts.quiet },
              ], ui.filter, (v) => { ui.filter = v; ctx.update(); }, { label: 'Show' })}
            </div>
            <ul class="log-list">${shown.map((d) => row(d, name))}</ul>
            ${!shown.length ? html`<div class="small muted">Nothing here.</div>` : ''}` : ''}
        `}
      </section>`;
  }

  return {
    id: 'log',
    view,
    load,
    shown() { load(); },
    onBus(type, data) {
      if (type === 'activity' && data && data.chatKey === ctx.key) reloadSoon();
    },
    destroy() { reloadSoon.cancel && reloadSoon.cancel(); },
  };
}
