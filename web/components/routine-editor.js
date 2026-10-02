// routine-editor.js — a persona's daily routine (wave 3, World & routine):
// rows of "label · days · from–to · how reachable", quick templates to start
// from, and helpers to describe a routine in plain English.
//
//   routineEditor({ routine, onChange(routine), disabled })
//   routineSummary(routine)          → "gym 07:00–08:00, work 09:00–17:00"
//   routineNow(world, date)          → { block, until } | null (what it's doing now)
//
// Times are in the persona's own time zone. "to" earlier than "from" wraps
// past midnight (sleep 23:30–07:00).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { seg } from '../ui.js';
import { helpTip } from './help-tip.js';

export const MAX_BLOCKS = 12;
export const DAY_IDS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
const DAY_LETTER = { mon: 'M', tue: 'T', wed: 'W', thu: 'T', fri: 'F', sat: 'S', sun: 'S' };
const DAY_LONG = { mon: 'Monday', tue: 'Tuesday', wed: 'Wednesday', thu: 'Thursday', fri: 'Friday', sat: 'Saturday', sun: 'Sunday' };
const WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri'];

export const REACH = [
  { value: 'normal', label: 'Reachable', icon: 'message', help: 'Answers as usual.' },
  { value: 'slow', label: 'Slow', icon: 'clock', help: 'Notices messages later (about three times slower).' },
  { value: 'unreachable', label: 'Unreachable', icon: 'snooze', help: 'Messages wait until this is over; it may say where it was.' },
];

const B = (label, days, from, to, reach) => ({ label, days, from, to, reach });
export const TEMPLATES = [
  { id: '9to5', label: '9-to-5', blocks: () => [
    B('work', WEEKDAYS, '09:00', '17:30', 'slow'),
    B('commute', WEEKDAYS, '08:15', '09:00', 'slow'),
    B('dinner', [], '19:30', '20:30', 'slow'),
    B('sleep', [], '23:30', '07:00', 'unreachable'),
  ] },
  { id: 'student', label: 'Student', blocks: () => [
    B('classes', ['mon', 'tue', 'wed', 'thu'], '10:00', '15:00', 'slow'),
    B('library', ['mon', 'wed'], '16:00', '19:00', 'slow'),
    B('sleep', [], '01:30', '09:30', 'unreachable'),
  ] },
  { id: 'night', label: 'Night shift', blocks: () => [
    B('work', ['sun', 'mon', 'tue', 'wed', 'thu'], '22:00', '06:00', 'slow'),
    B('sleep', [], '07:30', '14:30', 'unreachable'),
  ] },
  { id: 'gym', label: 'Gym rat', blocks: () => [
    B('gym', ['mon', 'wed', 'fri'], '07:00', '08:30', 'unreachable'),
    B('work', WEEKDAYS, '09:30', '18:00', 'slow'),
    B('gym', ['sat'], '10:00', '11:30', 'unreachable'),
    B('sleep', [], '23:00', '06:30', 'unreachable'),
  ] },
];

const toMin = (t) => {
  const m = /^(\d{1,2}):(\d{2})/.exec(t || '');
  return m ? Math.min(1440, (+m[1]) * 60 + (+m[2])) : 0;
};

/** "every day" / "Mon–Fri" / "weekends" / "Mon, Wed, Fri" */
export function daysText(days) {
  const d = (days || []).filter((x) => DAY_IDS.includes(x));
  if (!d.length || d.length === 7) return 'every day';
  const sorted = DAY_IDS.filter((x) => d.includes(x));
  if (sorted.join() === WEEKDAYS.join()) return 'Mon–Fri';
  if (sorted.join() === 'sat,sun') return 'weekends';
  if (sorted.join() === 'mon,tue,wed,thu,sun') return 'Sun–Thu';
  return sorted.map((x) => x[0].toUpperCase() + x.slice(1)).join(', ');
}

/** "gym 07:00–08:00, work 09:00–17:00 (+2 more)" */
export function routineSummary(routine, max = 3) {
  const r = routine || [];
  if (!r.length) return '';
  const parts = r.slice(0, max).map((b) => `${b.label} ${b.from}–${b.to}`);
  return parts.join(', ') + (r.length > max ? ` (+${r.length - max} more)` : '');
}

/** Wall-clock parts of `date` in a time zone ("" = this browser's). */
export function zoneParts(date, tz) {
  const opts = { weekday: 'short', hour: '2-digit', minute: '2-digit', hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit' };
  if (tz) opts.timeZone = tz;
  let parts;
  try { parts = new Intl.DateTimeFormat('en-GB', opts).formatToParts(date); } catch { parts = new Intl.DateTimeFormat('en-GB', { ...opts, timeZone: undefined }).formatToParts(date); }
  const get = (t) => (parts.find((p) => p.type === t) || {}).value || '';
  const wd = get('weekday').toLowerCase().slice(0, 3);
  return { day: wd, minutes: (+get('hour') % 24) * 60 + (+get('minute')), hm: `${get('hour')}:${get('minute')}`, ymd: `${get('year')}-${get('month')}-${get('day')}` };
}

const RANK = { normal: 0, slow: 1, unreachable: 2 };

/** The routine block running at `date` in the persona's zone (least reachable wins), or null. */
export function routineNow(world, date = new Date()) {
  const w = world || {};
  const r = w.routine || [];
  if (!r.length) return null;
  const now = zoneParts(date, w.timezone || '');
  const di = DAY_IDS.indexOf(now.day);
  const yesterday = DAY_IDS[(di + 6) % 7];
  let best = null;
  for (const b of r) {
    const f = toMin(b.from); const t = toMin(b.to);
    if (f === t) continue;
    const on = (d) => !(b.days && b.days.length) || b.days.includes(d);
    let hit = false;
    if (t > f) hit = on(now.day) && now.minutes >= f && now.minutes < t;
    else hit = (on(now.day) && now.minutes >= f) || (on(yesterday) && now.minutes < t);
    if (hit && (!best || (RANK[b.reach] || 0) > (RANK[best.reach] || 0))) best = b;
  }
  return best ? { block: best, until: best.to } : null;
}

function dayChips(days, onChange, disabled) {
  const d = days && days.length ? days : DAY_IDS.slice();
  const toggleDay = (id) => {
    let next = d.includes(id) ? d.filter((x) => x !== id) : [...d, id];
    if (!next.length) next = [id];
    next = DAY_IDS.filter((x) => next.includes(x));
    onChange(next.length === 7 ? [] : next);
  };
  return html`<span class="re-days" role="group" aria-label="Days">
    ${DAY_IDS.map((id) => html`<button type="button" class=${'re-day ' + (d.includes(id) ? 'on' : '')} aria-pressed=${String(d.includes(id))}
      title=${DAY_LONG[id]} aria-label=${DAY_LONG[id]} ?disabled=${disabled} @click=${() => toggleDay(id)}>${DAY_LETTER[id]}</button>`)}
  </span>`;
}

export function routineEditor({ routine, onChange, disabled = false }) {
  const r = (routine || []).map((b) => ({ ...b, days: (b.days || []).slice() }));
  const set = (i, patch) => { const next = r.map((b) => ({ ...b })); next[i] = { ...next[i], ...patch }; onChange(next); };
  const remove = (i) => onChange(r.filter((_, j) => j !== i));
  const add = () => {
    const last = r[r.length - 1];
    onChange([...r, last ? { label: '', days: last.days.slice(), from: last.to, to: last.to === '23:00' ? '23:59' : '23:00', reach: 'normal' }
      : { label: '', days: [], from: '09:00', to: '17:00', reach: 'slow' }]);
  };
  return html`<div class=${'routine-editor ' + (disabled ? 'is-disabled' : '')}>
    <div class="row row-wrap gap-6 re-templates">
      <span class="small muted">Start from</span>
      ${TEMPLATES.map((t) => html`<button type="button" class="chip chip-sm" ?disabled=${disabled}
        title=${`Fill in a typical ${t.label.toLowerCase()} day, then change anything`} @click=${() => onChange(t.blocks())}>${icon('wand')}${t.label}</button>`)}
    </div>
    ${r.length ? html`<div class="re-list">
      ${r.map((b, i) => {
        const wraps = toMin(b.to) <= toMin(b.from);
        return html`<div class="re-row" data-key=${'rb' + i}>
          <div class="re-top">
            <input class="input input-sm re-label" maxlength="24" placeholder="e.g. gym" aria-label="What they're doing" .value=${b.label || ''} ?disabled=${disabled}
              @change=${(e) => set(i, { label: e.target.value.trim() })}>
            <span class="re-time">
              <input type="time" class="we-time" step="300" aria-label="From" .value=${b.from || '09:00'} ?disabled=${disabled} @change=${(e) => e.target.value && set(i, { from: e.target.value })}>
              <span class="we-dash">–</span>
              <input type="time" class="we-time" step="300" aria-label="Until" .value=${b.to || '17:00'} ?disabled=${disabled} @change=${(e) => e.target.value && set(i, { to: e.target.value })}>
              ${wraps ? html`<span class="we-next" title="Ends the next morning">+1</span>` : ''}
            </span>
            <button type="button" class="btn btn-ghost btn-icon btn-sm" aria-label=${`Remove ${b.label || 'this block'}`} title="Remove" ?disabled=${disabled} @click=${() => remove(i)}>${icon('trash')}</button>
          </div>
          <div class="re-bottom">
            ${dayChips(b.days, (days) => set(i, { days }), disabled)}
            ${seg(REACH.map((x) => ({ value: x.value, label: x.label })), b.reach || 'normal', (v) => set(i, { reach: v }), { cls: 'seg-sm re-reach', label: 'How reachable' })}
          </div>
        </div>`;
      })}
    </div>` : html`<div class="re-empty small muted">No routine yet — the persona is around all day. Pick a template above or add a block.</div>`}
    <div class="row gap-8 re-foot">
      ${r.length < MAX_BLOCKS ? html`<button type="button" class="btn btn-glass btn-sm" ?disabled=${disabled} @click=${add}>${icon('plus')}Add a block</button>` : html`<span class="small faint">That's the maximum of ${MAX_BLOCKS} blocks.</span>`}
      <span class="grow"></span>
      <span class="small muted row gap-4">How reachable? ${helpTip('reach')}</span>
    </div>
  </div>`;
}
