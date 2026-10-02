// world-card.js — the persona editor's "World & routine" card (wave 3):
// where the persona lives, its time zone (the same as yours, or another
// country's) with a live "It's 20:11 Thursday evening there" preview, and
// its daily routine (routine-editor.js).
//
//   worldCard({ world, name, ui, onChange(world), onUpdate() })
//   ui: { open, q, other } — kept by the page between renders.
//
// Everything typed here is user text: rendered through html`` (escaped).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { seg } from '../ui.js';
import { localTimeZone } from './behaviour-meta.js';
import { routineEditor, routineNow, routineSummary, zoneParts } from './routine-editor.js';

// Cities and countries people type, mapped to a time zone.
const PLACES = [
  ['Israel', 'Asia/Jerusalem'], ['Tel Aviv', 'Asia/Jerusalem'], ['Jerusalem', 'Asia/Jerusalem'], ['Haifa', 'Asia/Jerusalem'],
  ['United Kingdom', 'Europe/London'], ['UK', 'Europe/London'], ['England', 'Europe/London'], ['London', 'Europe/London'], ['Manchester', 'Europe/London'],
  ['Ireland', 'Europe/Dublin'], ['Dublin', 'Europe/Dublin'], ['Portugal', 'Europe/Lisbon'], ['Lisbon', 'Europe/Lisbon'],
  ['France', 'Europe/Paris'], ['Paris', 'Europe/Paris'], ['Germany', 'Europe/Berlin'], ['Berlin', 'Europe/Berlin'], ['Munich', 'Europe/Berlin'],
  ['Spain', 'Europe/Madrid'], ['Madrid', 'Europe/Madrid'], ['Barcelona', 'Europe/Madrid'], ['Italy', 'Europe/Rome'], ['Rome', 'Europe/Rome'], ['Milan', 'Europe/Rome'],
  ['Netherlands', 'Europe/Amsterdam'], ['Amsterdam', 'Europe/Amsterdam'], ['Greece', 'Europe/Athens'], ['Athens', 'Europe/Athens'],
  ['Cyprus', 'Asia/Nicosia'], ['Turkey', 'Europe/Istanbul'], ['Istanbul', 'Europe/Istanbul'], ['Russia', 'Europe/Moscow'], ['Moscow', 'Europe/Moscow'],
  ['Ukraine', 'Europe/Kyiv'], ['Kyiv', 'Europe/Kyiv'], ['Poland', 'Europe/Warsaw'], ['Warsaw', 'Europe/Warsaw'], ['Sweden', 'Europe/Stockholm'],
  ['UAE', 'Asia/Dubai'], ['Dubai', 'Asia/Dubai'], ['Abu Dhabi', 'Asia/Dubai'], ['Saudi Arabia', 'Asia/Riyadh'], ['Egypt', 'Africa/Cairo'], ['Cairo', 'Africa/Cairo'],
  ['India', 'Asia/Kolkata'], ['Mumbai', 'Asia/Kolkata'], ['Delhi', 'Asia/Kolkata'], ['Bangalore', 'Asia/Kolkata'], ['Thailand', 'Asia/Bangkok'], ['Bangkok', 'Asia/Bangkok'],
  ['Vietnam', 'Asia/Ho_Chi_Minh'], ['Singapore', 'Asia/Singapore'], ['China', 'Asia/Shanghai'], ['Beijing', 'Asia/Shanghai'], ['Hong Kong', 'Asia/Hong_Kong'],
  ['Japan', 'Asia/Tokyo'], ['Tokyo', 'Asia/Tokyo'], ['Osaka', 'Asia/Tokyo'], ['South Korea', 'Asia/Seoul'], ['Korea', 'Asia/Seoul'], ['Seoul', 'Asia/Seoul'],
  ['Australia', 'Australia/Sydney'], ['Sydney', 'Australia/Sydney'], ['Melbourne', 'Australia/Melbourne'], ['New Zealand', 'Pacific/Auckland'], ['Auckland', 'Pacific/Auckland'],
  ['USA', 'America/New_York'], ['United States', 'America/New_York'], ['New York', 'America/New_York'], ['NYC', 'America/New_York'], ['Boston', 'America/New_York'], ['Miami', 'America/New_York'],
  ['Chicago', 'America/Chicago'], ['Texas', 'America/Chicago'], ['Austin', 'America/Chicago'], ['Denver', 'America/Denver'],
  ['California', 'America/Los_Angeles'], ['Los Angeles', 'America/Los_Angeles'], ['LA', 'America/Los_Angeles'], ['San Francisco', 'America/Los_Angeles'], ['Seattle', 'America/Los_Angeles'],
  ['Canada', 'America/Toronto'], ['Toronto', 'America/Toronto'], ['Vancouver', 'America/Vancouver'], ['Mexico', 'America/Mexico_City'], ['Mexico City', 'America/Mexico_City'],
  ['Brazil', 'America/Sao_Paulo'], ['São Paulo', 'America/Sao_Paulo'], ['Sao Paulo', 'America/Sao_Paulo'], ['Rio', 'America/Sao_Paulo'], ['Argentina', 'America/Argentina/Buenos_Aires'], ['Buenos Aires', 'America/Argentina/Buenos_Aires'],
  ['South Africa', 'Africa/Johannesburg'], ['Cape Town', 'Africa/Johannesburg'], ['Nigeria', 'Africa/Lagos'], ['Lagos', 'Africa/Lagos'], ['Kenya', 'Africa/Nairobi'], ['Nairobi', 'Africa/Nairobi'],
];
const COUNTRIES = ['Israel', 'United Kingdom', 'Ireland', 'Portugal', 'France', 'Germany', 'Spain', 'Italy', 'Netherlands', 'Greece', 'Cyprus', 'Turkey', 'Russia', 'Ukraine', 'Poland', 'Sweden',
  'UAE', 'Saudi Arabia', 'Egypt', 'India', 'Thailand', 'Vietnam', 'Singapore', 'China', 'Japan', 'South Korea', 'Australia', 'New Zealand', 'USA', 'Canada', 'Mexico', 'Brazil', 'Argentina',
  'South Africa', 'Nigeria', 'Kenya'];
const FRI_SAT = new Set(['israel', 'il', 'saudi arabia', 'saudi', 'ksa', 'qatar', 'kuwait', 'bahrain', 'oman', 'egypt', 'jordan', 'iraq', 'libya', 'sudan', 'syria', 'yemen', 'algeria', 'bangladesh']);
const FRI_SAT_ZONES = new Set(['Asia/Jerusalem', 'Asia/Tel_Aviv', 'Asia/Riyadh', 'Asia/Qatar', 'Asia/Kuwait', 'Asia/Bahrain', 'Asia/Muscat', 'Africa/Cairo', 'Asia/Amman', 'Asia/Baghdad', 'Asia/Dhaka']);

let zoneList = null;
function zones() {
  if (zoneList) return zoneList;
  try { zoneList = Intl.supportedValuesOf('timeZone'); } catch { zoneList = []; }
  return zoneList;
}

/** "Asia/Tokyo" → "Tokyo"; "America/Argentina/Buenos_Aires" → "Buenos Aires". */
export const zoneCity = (tz) => (tz || '').split('/').pop().replace(/_/g, ' ');
const zoneArea = (tz) => (tz || '').split('/')[0];

/** Zones matching a search (city/country names first, then zone names). */
function searchZones(q) {
  const s = q.trim().toLowerCase();
  if (!s) return [];
  const out = [];
  const seen = new Set();
  const push = (tz, why) => { if (tz && !seen.has(tz)) { seen.add(tz); out.push({ tz, why }); } };
  for (const [name, tz] of PLACES) if (name.toLowerCase().startsWith(s)) push(tz, name);
  for (const [name, tz] of PLACES) if (name.toLowerCase().includes(s)) push(tz, name);
  for (const tz of zones()) if (tz.toLowerCase().replace(/_/g, ' ').includes(s)) push(tz, '');
  return out.slice(0, 8);
}

/** The time zone a typed city/country most likely means ("" = unknown). */
export function guessZone(world) {
  const w = world || {};
  for (const v of [w.city, w.country]) {
    const s = (v || '').trim().toLowerCase();
    if (!s) continue;
    const hit = PLACES.find(([name]) => name.toLowerCase() === s);
    if (hit) return hit[1];
    const z = zones().find((tz) => zoneCity(tz).toLowerCase() === s);
    if (z) return z;
  }
  return '';
}

function partOfDay(min) {
  const h = Math.floor(min / 60);
  if (h < 5) return 'late at night';
  if (h < 8) return 'early morning';
  if (h < 12) return 'morning';
  if (h < 14) return 'lunchtime';
  if (h < 18) return 'afternoon';
  if (h < 22) return 'evening';
  return 'late evening';
}

const DAY_FULL = { mon: 'Monday', tue: 'Tuesday', wed: 'Wednesday', thu: 'Thursday', fri: 'Friday', sat: 'Saturday', sun: 'Sunday' };

function weekendOf(world) {
  const c = (world.country || '').trim().toLowerCase();
  if (FRI_SAT.has(c)) return ['fri', 'sat'];
  if (!c && FRI_SAT_ZONES.has(world.timezone || localTimeZone())) return ['fri', 'sat'];
  return ['sat', 'sun'];
}

/** "Lives in Tel Aviv · same time zone as you · gym, work, sleep" */
export function worldSummary(world) {
  const w = world || {};
  const where = [w.city, w.country].filter(Boolean).join(', ');
  const bits = [where ? `Lives in ${where}` : 'No home town set'];
  bits.push(w.timezone ? `${zoneCity(w.timezone)} time` : 'same time zone as you');
  const r = w.routine || [];
  bits.push(r.length ? `routine: ${[...new Set(r.map((b) => b.label).filter(Boolean))].slice(0, 3).join(', ')}` : 'no routine');
  return bits.join(' · ');
}

/** The live "It's 20:11 on Thursday evening there" line, plus what it's doing now. */
export function worldNow(world, name = 'they', date = new Date()) {
  const w = world || {};
  const there = zoneParts(date, w.timezone || '');
  const here = zoneParts(date, '');
  const differs = !!w.timezone && (there.hm !== here.hm || there.ymd !== here.ymd);
  const wk = weekendOf(w);
  const weekend = wk.includes(there.day);
  const now = routineNow(w, date);
  return { there, here, differs, weekend, weekendDays: wk, part: partOfDay(there.minutes), day: DAY_FULL[there.day] || '', now, name };
}

function previewBox(world, name) {
  const n = worldNow(world, name);
  const who = name || 'They';
  const reach = n.now ? { normal: 'answers as usual', slow: 'slow to reply', unreachable: 'messages wait until then' }[n.now.block.reach] || '' : '';
  return html`<div class="world-preview" aria-live="polite">
    <span class="wp-clock" aria-hidden="true">${clockFace(n.there.minutes)}</span>
    <div class="grow">
      <div class="wp-main">It's <b>${n.there.hm}</b> on ${n.day} ${n.part} ${n.differs ? 'there' : 'for both of you'}.</div>
      <div class="wp-sub small muted">
        ${n.differs ? html`For you it's ${n.here.hm}${n.here.ymd !== n.there.ymd ? ' (a different day)' : ''}. ` : ''}
        ${n.weekend ? 'It is the weekend there.' : `Weekend there: ${DAY_FULL[n.weekendDays[0]]}–${DAY_FULL[n.weekendDays[1]]}.`}
      </div>
      ${n.now ? html`<div class="wp-now small"><span class=${'reach-dot r-' + n.now.block.reach}></span>${who} is busy with <b>${n.now.block.label || 'something'}</b> until ${n.now.until} — ${reach}.</div>` : ''}
    </div>
  </div>`;
}

/** A tiny analog clock (SVG) for the preview. */
function clockFace(min) {
  const h = (min / 60) % 12; const m = min % 60;
  const ha = (h / 12) * 360; const ma = (m / 60) * 360;
  const hand = (deg, len) => {
    const r = (deg - 90) * Math.PI / 180;
    return `${(20 + Math.cos(r) * len).toFixed(2)},${(20 + Math.sin(r) * len).toFixed(2)}`;
  };
  const night = min < 6 * 60 || min >= 20 * 60;
  return html`<svg viewBox="0 0 40 40" width="40" height="40" class=${night ? 'night' : 'day'}>
    <circle cx="20" cy="20" r="18" class="wp-face"></circle>
    ${[0, 90, 180, 270].map((d) => { const r = (d - 90) * Math.PI / 180; return html`<circle cx=${(20 + Math.cos(r) * 14.5).toFixed(2)} cy=${(20 + Math.sin(r) * 14.5).toFixed(2)} r="1.2" class="wp-tick"></circle>`; })}
    <polyline points=${`20,20 ${hand(ha + m / 2, 8.5)}`} class="wp-hand h"></polyline>
    <polyline points=${`20,20 ${hand(ma, 12.5)}`} class="wp-hand m"></polyline>
    <circle cx="20" cy="20" r="1.8" class="wp-pin"></circle>
  </svg>`;
}

export function worldCard({ world, name = '', ui, onChange, onUpdate }) {
  const w = { city: '', country: '', timezone: '', routine: [], ...(world || {}) };
  const set = (patch) => onChange({ ...w, ...patch });
  const other = ui.other || !!w.timezone;
  const local = localTimeZone();
  const guess = !w.timezone ? guessZone(w) : '';
  const suggest = guess && guess !== local ? guess : '';
  const results = other && ui.q ? searchZones(ui.q) : [];
  const pickZone = (tz) => {
    ui.q = '';
    const patch = { timezone: tz };
    if (!w.city && tz) patch.city = zoneCity(tz);
    set(patch);
  };
  return html`<section class="card section world-card" data-key="world">
    <button type="button" class="disclosure world-head" aria-expanded=${String(!!ui.open)} @click=${() => { ui.open = !ui.open; onUpdate(); }}>
      <span class="s-icon" style="--c:linear-gradient(135deg,#38bdf8,#22c55e)">${icon('globe')}</span>
      <span class="grow world-head-text">
        <span class="h2-like">World & routine</span>
        <span class="small muted">${ui.open ? `Where ${name || 'they'} live${name ? 's' : ''}, what time it is there, and what a normal day looks like.` : worldSummary(w)}</span>
      </span>
      ${icon(ui.open ? 'chevron-up' : 'chevron-down', 'ic-sm')}
    </button>
    ${ui.open ? html`<div class="world-body">
      <div class="two-col">
        <div class="field"><label class="field-label" for="pe-city">City <span class="faint small">optional</span></label>
          <input id="pe-city" class="input" maxlength="80" placeholder="e.g. Tel Aviv" .value=${w.city} @input=${(e) => set({ city: e.target.value })}></div>
        <div class="field"><label class="field-label" for="pe-country">Country <span class="faint small">optional</span></label>
          <input id="pe-country" class="input" maxlength="80" list="pe-countries" placeholder="e.g. Israel" .value=${w.country} @input=${(e) => set({ country: e.target.value })}>
          <datalist id="pe-countries">${COUNTRIES.map((c) => html`<option value=${c}></option>`)}</datalist></div>
      </div>

      <div class="field mt-16">
        <div class="field-label">${icon('clock', 'ic-sm')}Time zone</div>
        <div class="field-help">The persona knows what time and day it is where it lives — late at night it writes like it's late at night.</div>
        <div class="mt-8">${seg([
          { value: 'same', label: 'Same as me', icon: 'user' },
          { value: 'other', label: 'Somewhere else', icon: 'globe' },
        ], other ? 'other' : 'same', (v) => {
          if (v === 'same') { ui.other = false; ui.q = ''; set({ timezone: '' }); return; }
          ui.other = true;
          if (suggest) { set({ timezone: suggest }); return; }
          onUpdate();
          requestAnimationFrame(() => { const el = document.getElementById('pe-zone-q'); if (el) el.focus(); });
        }, { label: 'Time zone' })}</div>
        ${!other && suggest ? html`<div class="banner info small mt-8">${icon('info')}<div>${w.city || w.country} is usually on <b>${zoneCity(suggest)}</b> time.
          <button type="button" class="link-btn" @click=${() => { ui.other = true; set({ timezone: suggest }); }}>Use ${zoneCity(suggest)} time</button></div></div>` : ''}
        ${other ? html`<div class="zone-pick mt-8">
          ${w.timezone ? html`<div class="zone-chosen">${icon('globe', 'ic-sm')}<b>${zoneCity(w.timezone)}</b><span class="faint small">${zoneArea(w.timezone).replace(/_/g, ' ')} · ${w.timezone}</span>
            <span class="grow"></span><button type="button" class="link-btn small" @click=${() => { ui.q = ''; set({ timezone: '' }); ui.other = true; requestAnimationFrame(() => { const el = document.getElementById('pe-zone-q'); if (el) el.focus(); }); }}>Change</button></div>`
          : html`<div class="zone-search">
            ${icon('search', 'ic-sm')}
            <input id="pe-zone-q" class="input" autocomplete="off" placeholder="Search a city, country or time zone…" aria-label="Search time zones" .value=${ui.q || ''}
              @input=${(e) => { ui.q = e.target.value; onUpdate(); }}
              @keydown=${(e) => { if (e.key === 'Enter' && results.length) { e.preventDefault(); pickZone(results[0].tz); } }}>
          </div>
          ${results.length ? html`<div class="zone-results" role="listbox" aria-label="Time zones">
            ${results.map((r) => { const p = zoneParts(new Date(), r.tz); return html`<button type="button" role="option" class="zone-opt" @click=${() => pickZone(r.tz)}>
              <span class="grow"><b>${r.why || zoneCity(r.tz)}</b> <span class="faint small">${r.tz.replace(/_/g, ' ')}</span></span><span class="mono small">${p.hm}</span></button>`; })}
          </div>` : ui.q ? html`<div class="small muted mt-8">No match — try a big city nearby, like "London" or "Tokyo".</div>`
            : html`<div class="small faint mt-8">Try "Tokyo", "New York" or "Germany".</div>`}`}
        </div>` : ''}
      </div>

      ${previewBox(w, name)}

      <div class="field mt-16">
        <div class="field-label">${icon('calendar', 'ic-sm')}Daily routine <span class="faint small">optional</span></div>
        <div class="field-help">A normal day gives the persona a life: at the gym until 8, slow to answer at work, asleep at night. It replies late when it's busy — and may say where it was.</div>
        <div class="mt-8">${routineEditor({ routine: w.routine, onChange: (routine) => set({ routine }) })}</div>
        ${w.routine.length ? html`<div class="tiny faint mt-8">Times are ${w.timezone ? `${zoneCity(w.timezone)} time` : 'your time'}. ${routineSummary(w.routine)}</div>` : ''}
      </div>
    </div>` : ''}
  </section>`;
}
