// week-editor.js — active hours per weekday (Mon–Sun), each day with any
// number of "from–to" ranges (to <= from wraps past midnight), plus quick
// fills and a time-zone picker.
//
//   weekEditor({ week, timezone, onWeek(week), onTimezone(tz), disabled, personaZone })
//
// timezone "persona" (wave 3) follows the persona's own time zone (World &
// routine in the persona editor); personaZone is an optional hint shown
// next to that choice, e.g. "Asia/Tokyo".
//
// The full 7-day array is always handed back (the settings PUT replaces
// arrays wholesale).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { localTimeZone } from './behaviour-meta.js';

export const DAYS = [
  { id: 'mon', short: 'Mon', long: 'Monday' },
  { id: 'tue', short: 'Tue', long: 'Tuesday' },
  { id: 'wed', short: 'Wed', long: 'Wednesday' },
  { id: 'thu', short: 'Thu', long: 'Thursday' },
  { id: 'fri', short: 'Fri', long: 'Friday' },
  { id: 'sat', short: 'Sat', long: 'Saturday' },
  { id: 'sun', short: 'Sun', long: 'Sunday' },
];

const R = (from, to) => ({ from, to });
const QUICK = [
  { label: 'Every day 9:00–22:00', week: () => DAYS.map((d) => ({ day: d.id, ranges: [R('09:00', '22:00')] })) },
  { label: 'Weekdays 8:00–23:00', week: () => DAYS.map((d, i) => ({ day: d.id, ranges: i < 5 ? [R('08:00', '23:00')] : [R('10:00', '23:30')] })), sub: 'weekends 10:00–23:30' },
  { label: 'Evenings 18:00–02:30', week: () => DAYS.map((d) => ({ day: d.id, ranges: [R('18:00', '02:30')] })) },
];

/** Always 7 entries, mon..sun, ranges copied. */
export function normWeek(week) {
  return DAYS.map((d) => {
    const e = (week || []).find((x) => x && x.day === d.id);
    return { day: d.id, ranges: (e && Array.isArray(e.ranges) ? e.ranges : []).map((r) => ({ from: r.from || '00:00', to: r.to || '00:00' })) };
  });
}

const toMin = (t) => {
  const m = /^(\d{1,2}):(\d{2})/.exec(t || '');
  return m ? Math.min(24 * 60, (+m[1]) * 60 + (+m[2])) : 0;
};

/** Total open minutes in a week (for summaries). */
export function weekMinutes(week) {
  let total = 0;
  for (const d of normWeek(week)) {
    for (const r of d.ranges) {
      const a = toMin(r.from); const b = toMin(r.to);
      total += b > a ? b - a : 24 * 60 - a + b;
    }
  }
  return total;
}

/** "Every day 18:00–02:30" / "Mon–Fri 08:00–23:00, weekends off" / "Custom week" */
export function describeWeek(week) {
  const w = normWeek(week);
  const sig = (d) => d.ranges.map((r) => `${r.from}–${r.to}`).join(', ');
  const sigs = w.map(sig);
  if (sigs.every((s) => s === '')) return 'No hours set';
  if (sigs.every((s) => s === sigs[0])) return `Every day ${sigs[0]}`;
  const wk = sigs.slice(0, 5); const we = sigs.slice(5);
  if (wk.every((s) => s === wk[0]) && we.every((s) => s === we[0])) {
    return `Weekdays ${wk[0] || 'off'} · weekends ${we[0] || 'off'}`;
  }
  return 'Custom hours';
}

let zoneCache = null;
function zones() {
  if (zoneCache) return zoneCache;
  try { zoneCache = Intl.supportedValuesOf('timeZone'); } catch { zoneCache = []; }
  return zoneCache;
}

function strip(ranges) {
  const segs = [];
  for (const r of ranges) {
    const a = toMin(r.from); const b = toMin(r.to);
    if (b > a) segs.push([a, b]);
    else { segs.push([a, 1440]); if (b > 0) segs.push([0, b]); }
  }
  return html`<span class="we-strip" aria-hidden="true">${segs.map(([a, b]) =>
    html`<span style=${`left:${(a / 1440) * 100}%;width:${((b - a) / 1440) * 100}%`}></span>`)}</span>`;
}

export function weekEditor({ week, timezone = '', onWeek, onTimezone, disabled = false, personaZone = '' }) {
  const w = normWeek(week);
  const set = (fn) => { const next = normWeek(w); fn(next); onWeek && onWeek(next); };
  const local = localTimeZone();
  const list = zones();
  const tzKnown = !timezone || timezone === 'persona' || list.includes(timezone);

  return html`<div class=${'week-editor ' + (disabled ? 'is-disabled' : '')}>
    <div class="we-quick chip-row">
      ${QUICK.map((q) => html`<button type="button" class="chip chip-sm" title=${q.sub || q.label} ?disabled=${disabled}
        @click=${() => onWeek && onWeek(q.week())}>${icon('wand')}${q.label}</button>`)}
    </div>
    <div class="we-grid" role="table" aria-label="Active hours by day">
      <div class="we-row we-head" role="row" aria-hidden="true">
        <span></span>
        <span class="we-scale"><span>00</span><span>06</span><span>12</span><span>18</span><span>24</span></span>
        <span></span>
      </div>
      ${w.map((d, i) => {
        const meta = DAYS[i];
        return html`<div class=${'we-row ' + (d.ranges.length ? '' : 'off')} role="row" data-key=${'we-' + d.day}>
          <span class="we-day" role="rowheader" title=${meta.long}>${meta.short}</span>
          <div class="we-cell" role="cell">
            ${strip(d.ranges)}
            <div class="we-ranges">
              ${d.ranges.length ? d.ranges.map((r, ri) => {
                const wraps = toMin(r.to) <= toMin(r.from);
                return html`<span class="we-range" data-key=${`r${ri}`}>
                  <input type="time" class="we-time" value=${r.from} step="300" aria-label=${`${meta.long} from`} ?disabled=${disabled}
                    @change=${(e) => { const v = e.target.value; if (v) set((n) => { n[i].ranges[ri].from = v; }); }}>
                  <span class="we-dash">–</span>
                  <input type="time" class="we-time" value=${r.to} step="300" aria-label=${`${meta.long} until`} ?disabled=${disabled}
                    @change=${(e) => { const v = e.target.value; if (v) set((n) => { n[i].ranges[ri].to = v; }); }}>
                  ${wraps ? html`<span class="we-next" title="Ends the next morning">+1</span>` : ''}
                  <button type="button" class="we-x" aria-label=${`Remove ${meta.long} ${r.from}–${r.to}`} ?disabled=${disabled}
                    @click=${() => set((n) => { n[i].ranges.splice(ri, 1); })}>${icon('x')}</button>
                </span>`;
              }) : html`<span class="we-off">Off all day</span>`}
              ${d.ranges.length < 4 ? html`<button type="button" class="we-add" ?disabled=${disabled} aria-label=${`Add hours on ${meta.long}`}
                title="Add a time range" @click=${() => set((n) => {
                  const last = n[i].ranges[n[i].ranges.length - 1];
                  n[i].ranges.push(last ? { from: last.to, to: last.to === '23:00' ? '23:59' : '23:00' } : { from: '09:00', to: '22:00' });
                })}>${icon('plus')}${d.ranges.length ? '' : html`<span>Add hours</span>`}</button>` : ''}
            </div>
          </div>
          <button type="button" class="btn btn-ghost btn-icon btn-sm we-copy" title=${`Copy ${meta.long}'s hours to every day`}
            aria-label=${`Copy ${meta.long}'s hours to every day`} ?disabled=${disabled}
            @click=${() => set((n) => { for (const x of n) x.ranges = d.ranges.map((r) => ({ ...r })); })}>${icon('copy')}</button>
        </div>`;
      })}
    </div>
    <div class="we-tz">
      <span class="field-label">${icon('globe')}Time zone</span>
      <select class="select input-sm" aria-label="Time zone" ?disabled=${disabled} data-keep data-key=${'tz:' + timezone} @change=${(e) => onTimezone && onTimezone(e.target.value)}>
        <option value="persona" ?selected=${timezone === 'persona'}>Same as the persona${personaZone ? ` (${personaZone.replace(/_/g, ' ')})` : ''}</option>
        <option value="" ?selected=${!timezone}>This computer's time zone${local ? ` (${local.replace(/_/g, ' ')})` : ''}</option>
        ${!tzKnown ? html`<option value=${timezone} selected>${timezone}</option>` : ''}
        ${list.map((z) => html`<option value=${z} ?selected=${z === timezone}>${z.replace(/_/g, ' ')}</option>`)}
      </select>
    </div>
  </div>`;
}
