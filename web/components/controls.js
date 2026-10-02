// controls.js — numeric controls for the Behaviour editor.
//
//   rangePair({ lo, hi, min, max, unit, format, onChange(lo, hi), label })
//       Dual-handle slider with a filled track between the handles and two
//       exact-number inputs. Wide second ranges use a gentle curve so the
//       short end (seconds) gets most of the track.
//   percentSlider({ value, onChange, min, max, step, suffix, label, left, right })
//   numberField({ value, min, max, unit, onChange, label, width, step })
//
// All inputs use the `value` attribute (not the property) so the morph
// renderer only pushes a new value when the model changed — half-typed
// numbers are never clobbered by unrelated re-renders.

import { html } from '../dom.js';
import { fmtSec } from './behaviour-meta.js';

const clampN = (v, lo, hi) => Math.min(hi, Math.max(lo, v));
const STEPS = 1000;

/** Slider position <-> value mapping. Curved for wide ranges. */
function scaleFor(min, max, curve) {
  const span = max - min;
  const k = curve ?? (span >= 300 ? 2.6 : span >= 90 ? 1.7 : 1);
  return {
    k,
    toPos: (v) => (span <= 0 ? 0 : Math.round(STEPS * Math.pow(clampN((v - min) / span, 0, 1), 1 / k))),
    toVal: (p) => {
      const raw = min + span * Math.pow(clampN(p / STEPS, 0, 1), k);
      return clampN(snap(raw, k > 1), min, max);
    },
  };
}

function snap(v, nice) {
  if (!nice) return Math.round(v);
  if (v < 30) return Math.round(v);
  if (v < 120) return Math.round(v / 5) * 5;
  if (v < 600) return Math.round(v / 15) * 15;
  if (v < 3600) return Math.round(v / 60) * 60;
  return Math.round(v / 300) * 300;
}

const frac = (pos) => (pos / STEPS).toFixed(4);

function parseIntIn(raw, min, max) {
  const s = String(raw).trim();
  if (s === '' || !/^-?\d+$/.test(s)) return null;
  const n = parseInt(s, 10);
  if (n < min || n > max) return null;
  return n;
}

/**
 * Dual-handle range. lo/hi are the current values; onChange(lo, hi) fires on
 * every movement (callers debounce saving).
 */
export function rangePair({ lo, hi, min = 0, max = 100, unit = 'sec', format = fmtSec, onChange, label = '', curve, disabled = false }) {
  lo = clampN(Number(lo) || 0, min, max);
  hi = clampN(Number(hi) || 0, min, max);
  if (hi < lo) hi = lo;
  const sc = scaleFor(min, max, curve);
  const pLo = sc.toPos(lo);
  const pHi = sc.toPos(hi);
  // Keep the low handle grabbable when both sit at the far right.
  const loOnTop = pLo > STEPS * 0.5 && pLo >= pHi - 10;

  const paint = (wrap, a, b) => {
    if (!wrap) return;
    wrap.style.setProperty('--lo', frac(a));
    wrap.style.setProperty('--hi', frac(b));
  };

  const onLo = (e) => {
    const wrap = e.target.closest('.range-pair');
    let p = parseInt(e.target.value, 10);
    const hiPos = parseInt(wrap.querySelector('.rp-hi').value, 10);
    if (p > hiPos) { p = hiPos; e.target.value = String(p); }
    paint(wrap, p, hiPos);
    const v = sc.toVal(p);
    onChange && onChange(Math.min(v, hi), hi);
  };
  const onHi = (e) => {
    const wrap = e.target.closest('.range-pair');
    let p = parseInt(e.target.value, 10);
    const loPos = parseInt(wrap.querySelector('.rp-lo').value, 10);
    if (p < loPos) { p = loPos; e.target.value = String(p); }
    paint(wrap, loPos, p);
    const v = sc.toVal(p);
    onChange && onChange(lo, Math.max(v, lo));
  };

  const numLo = (e, commit) => {
    const n = parseIntIn(e.target.value, min, max);
    if (n == null) { if (commit) e.target.value = String(lo); return; }
    if (commit && n > hi) { onChange && onChange(n, n); return; }
    if (n <= hi) onChange && onChange(n, hi);
  };
  const numHi = (e, commit) => {
    const n = parseIntIn(e.target.value, min, max);
    if (n == null) { if (commit) e.target.value = String(hi); return; }
    if (commit && n < lo) { onChange && onChange(n, n); return; }
    if (n >= lo) onChange && onChange(lo, n);
  };

  const readout = lo === hi ? format(lo) : `${format(lo)} – ${format(hi)}`;
  return html`<div class=${'range-pair-wrap ' + (disabled ? 'is-disabled' : '')}>
    <input class="input input-sm input-number rp-num" type="number" inputmode="numeric" min=${min} max=${max} value=${String(lo)}
      aria-label=${`${label} — shortest (${unit})`} ?disabled=${disabled}
      @input=${(e) => numLo(e, false)} @change=${(e) => numLo(e, true)}>
    <div class="range-pair" style=${`--lo:${frac(pLo)};--hi:${frac(pHi)}`} title=${readout}>
      <span class="rp-track"></span><span class="rp-fill"></span>
      <input class=${'rp-lo ' + (loOnTop ? 'on-top' : '')} type="range" min="0" max=${STEPS} step="1" value=${String(pLo)}
        aria-label=${`${label} — shortest`} aria-valuetext=${format(lo)} ?disabled=${disabled} @input=${onLo}>
      <input class="rp-hi" type="range" min="0" max=${STEPS} step="1" value=${String(pHi)}
        aria-label=${`${label} — longest`} aria-valuetext=${format(hi)} ?disabled=${disabled} @input=${onHi}>
    </div>
    <input class="input input-sm input-number rp-num" type="number" inputmode="numeric" min=${min} max=${max} value=${String(hi)}
      aria-label=${`${label} — longest (${unit})`} ?disabled=${disabled}
      @input=${(e) => numHi(e, false)} @change=${(e) => numHi(e, true)}>
    <span class="rp-unit faint small">${unit}</span>
  </div>`;
}

/** Single slider with a value label (percent by default). */
export function percentSlider({ value, onChange, min = 0, max = 100, step = 1, suffix = '%', label = '', left = '', right = '', format, disabled = false }) {
  const v = clampN(Number(value) || 0, min, max);
  const fill = max > min ? ((v - min) / (max - min)) * 100 : 0;
  const show = format ? format(v) : `${v}${suffix}`;
  return html`<div class=${'range bf-slider ' + (disabled ? 'is-disabled' : '')}>
    ${left ? html`<span class="tiny faint nowrap">${left}</span>` : ''}
    <input type="range" min=${min} max=${max} step=${step} value=${String(v)} style=${`--pct:${fill}%`}
      aria-label=${label} aria-valuetext=${show} ?disabled=${disabled}
      @input=${(e) => {
        const n = parseInt(e.target.value, 10);
        e.target.style.setProperty('--pct', `${((n - min) / (max - min || 1)) * 100}%`);
        onChange && onChange(n);
      }}>
    ${right ? html`<span class="tiny faint nowrap">${right}</span>` : ''}
    <span class="range-value">${show}</span>
  </div>`;
}

/** Integer field with a unit label. onChange fires for valid, in-range input. */
export function numberField({ value, min = 0, max = 9999, unit = '', onChange, label = '', width, step = 1, disabled = false }) {
  const v = Number(value) || 0;
  return html`<span class="row gap-6 bf-num">
    <input class="input input-sm input-number" type="number" inputmode="numeric" min=${min} max=${max} step=${step}
      value=${String(v)} aria-label=${label} ?disabled=${disabled} style=${width ? `width:${width}px` : ''}
      @input=${(e) => { const n = parseIntIn(e.target.value, min, max); if (n != null && onChange) onChange(n); }}
      @change=${(e) => {
        const s = String(e.target.value).trim();
        let n = parseInt(s, 10);
        if (s === '' || isNaN(n)) { e.target.value = String(v); return; }
        n = clampN(n, min, max);
        e.target.value = String(n);
        if (n !== v && onChange) onChange(n);
      }}>
    ${unit ? html`<span class="faint small nowrap">${unit}</span>` : ''}
  </span>`;
}
