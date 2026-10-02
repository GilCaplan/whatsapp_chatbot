// vibe-dials.js — Speed, Chattiness and Boldness: three five-notch dials in
// front of the full behaviour form. A dial is derived from the profile (never
// stored): moving it writes its fields from the server's dial table
// (GET /api/behavior/presets → dials), reading places the profile on the
// nearest level (same rule as behavior.ReadDials in Go).
//
//   vibeDials({ kind: 'dm'|'group', profile, meta, scope, onChange(fields, dialId, level), compact })
//
// scope ('settings:private', 'chat:<key>') remembers which of several equal
// levels the user picked (e.g. Chattiness 3–5 are the same in private chats).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { dialFace, levelAt, DIAL_LOOK } from './dial-face.js';
import { helpTip } from './help-tip.js';

export const DIAL_ORDER = ['speed', 'chattiness', 'boldness'];
const DIAL_ICON = { speed: 'timer', chattiness: 'bubbles', boldness: 'sparkles' };
const UNLIMITED_ZERO = new Set(['maxRepliesPerHour', 'maxRepliesPerDay']);

const kindKey = (kind) => (kind === 'group' ? 'group' : 'private');

/** The dial definitions from the presets response, in display order. */
export function dialDefs(meta) {
  const d = meta && meta.dials;
  if (!d) return [];
  const order = (meta.dialOrder && meta.dialOrder.length ? meta.dialOrder : DIAL_ORDER).filter((id) => d[id]);
  return order.map((id) => d[id]);
}

/** field → dial id, for the dots in the Advanced form. */
export function dialFieldMap(meta, kind) {
  const out = {};
  for (const d of dialDefs(meta)) {
    const first = d.levels && d.levels[0] && d.levels[0][kindKey(kind)];
    for (const f of Object.keys(first || {})) out[f] = d.id;
  }
  return out;
}

export const dialColor = (id) => (DIAL_LOOK[id] || DIAL_LOOK.speed).c2;
export const dialLabel = (meta, id) => ((meta && meta.dials && meta.dials[id]) || {}).label || id;

function rangeMax(meta, f) {
  const r = meta && meta.ranges && meta.ranges[f];
  return r && Number.isFinite(r.max) ? r.max : 100;
}

function distance(meta, f, have, want) {
  if (typeof want === 'boolean') return !!have === want ? 0 : 1;
  const a = Number(have) || 0;
  const b = Number(want) || 0;
  if (a === b) return 0;
  const top = rangeMax(meta, f);
  const conv = (n) => (n === 0 && UNLIMITED_ZERO.has(f) ? Math.log1p(top) : Math.log1p(Math.max(0, n)));
  return Math.min(1, Math.abs(conv(a) - conv(b)) / Math.log1p(top));
}

/** { [dialId]: { level, exact, matches } } — mirrors behavior.ReadDials. */
export function readDials(profile, kind, meta) {
  const out = {};
  for (const d of dialDefs(meta)) {
    let best = Infinity;
    let level = 3;
    const matches = [];
    for (const l of d.levels) {
      const vals = l[kindKey(kind)] || {};
      let dist = 0;
      for (const [f, want] of Object.entries(vals)) dist += distance(meta, f, profile ? profile[f] : undefined, want);
      if (dist === 0) matches.push(l.level);
      if (dist < best - 1e-9 || (Math.abs(dist - best) < 1e-9 && Math.abs(l.level - 3) < Math.abs(level - 3))) {
        best = dist; level = l.level;
      }
    }
    out[d.id] = { level, exact: best === 0, matches };
  }
  return out;
}

/** The field values one level of a dial sets for a kind. */
export function dialValues(meta, id, kind, level) {
  const d = meta && meta.dials && meta.dials[id];
  const l = d && d.levels.find((x) => x.level === level);
  return l ? { ...(l[kindKey(kind)] || {}) } : null;
}

// Remembered picks among equal levels (per scope + dial), per browser.
const picks = new Map();
function pickKey(scope, id) { return `doppel.dial.${scope}.${id}`; }
function rememberPick(scope, id, level) {
  picks.set(pickKey(scope, id), level);
  try { localStorage.setItem(pickKey(scope, id), String(level)); } catch { /* ignore */ }
}
function rememberedPick(scope, id) {
  const k = pickKey(scope, id);
  if (picks.has(k)) return picks.get(k);
  try { const v = parseInt(localStorage.getItem(k) || '', 10); if (v) { picks.set(k, v); return v; } } catch { /* ignore */ }
  return 0;
}

/** Which level to show: the user's last pick when it still matches exactly. */
function shownLevel(pos, scope, id) {
  const pick = rememberedPick(scope, id);
  if (pick && pos.matches && pos.matches.includes(pick)) return pick;
  return pos.level;
}

export function vibeDials({ kind = 'dm', profile, meta, scope = 'default', onChange, compact = false, disabled = false }) {
  const defs = dialDefs(meta);
  if (!defs.length || !profile) {
    return html`<div class="vibe-dials skeleton-dials" aria-busy="true">${DIAL_ORDER.map(() => html`<div class="vd-card skeleton"></div>`)}</div>`;
  }
  const pos = readDials(profile, kind, meta);
  const set = (d, level, cur) => {
    level = Math.max(1, Math.min(5, level));
    rememberPick(scope, d.id, level);
    if (level === cur && pos[d.id].exact) return;
    const vals = dialValues(meta, d.id, kind, level);
    if (vals && onChange) onChange(vals, d.id, level);
  };
  return html`<div class=${'vibe-dials ' + (compact ? 'compact' : '')} role="group" aria-label="Vibe dials">
    ${defs.map((d) => {
      const p = pos[d.id] || { level: 3, exact: true, matches: [] };
      const level = shownLevel(p, scope, d.id);
      const lv = d.levels.find((x) => x.level === level) || d.levels[2];
      const blurb = kind !== 'group' && lv.privateBlurb ? lv.privateBlurb : lv.blurb;
      const look = DIAL_LOOK[d.id] || DIAL_LOOK.speed;
      const onFace = (e) => {
        if (disabled) return;
        const svg = e.currentTarget.querySelector('svg');
        const l = svg && levelAt(svg, e.clientX, e.clientY);
        if (l) set(d, l, level);
      };
      return html`<div class=${'vd-card ' + (p.exact ? '' : 'is-tuned')} data-key=${'vd-' + d.id} data-dial=${d.id} data-level=${level}
          style=${`--c1:${look.c1};--c2:${look.c2};--c3:${look.c3}`}>
        <div class="vd-head">
          <span class="vd-ic">${icon(DIAL_ICON[d.id] || 'sliders', 'ic-sm')}</span>
          <span class="vd-name">${d.label}</span>
          ${helpTip('dial-' + d.id)}
          <span class="grow"></span>
          ${p.exact ? '' : html`<span class="vd-tuned" title="Advanced settings changed some of this dial's values. Move the dial to snap back to a level.">${icon('sliders', 'ic-sm')}tuned</span>`}
        </div>
        <div class="vd-face" @pointerdown=${onFace} title=${`${d.label}: ${lv.label}`}>${dialFace({ dial: d.id, level, uid: scope.replace(/[^a-z0-9]/gi, '') })}</div>
        <div class="vd-level" aria-live="polite"><b data-key=${'vl-' + level}>${lv.label}</b></div>
        <p class="vd-blurb">${blurb}</p>
        <div class="vd-range">
          <input type="range" min="1" max="5" step="1" value=${String(level)} ?disabled=${disabled}
            aria-label=${d.label} aria-valuetext=${lv.label} style=${`--p:${((level - 1) / 4) * 100}%`}
            @input=${(e) => set(d, parseInt(e.target.value, 10), level)}>
          <div class="vd-ticks" aria-hidden="true">${d.levels.map((x) => html`<span class=${x.level === level ? 'on' : x.level < level ? 'lit' : ''}></span>`)}</div>
          <div class="vd-ends" aria-hidden="true"><span>${d.levels[0].label}</span><span>${d.levels[4].label}</span></div>
        </div>
      </div>`;
    })}
  </div>`;
}

/** A small coloured dot marking a form row whose value a dial sets. */
export function dialDot(meta, id) {
  if (!id) return '';
  const label = dialLabel(meta, id);
  return html`<span class="dial-dot" style=${`--c:${dialColor(id)}`} title=${`Set by the ${label} dial`} aria-label=${`Set by the ${label} dial`}>${label}</span>`;
}
