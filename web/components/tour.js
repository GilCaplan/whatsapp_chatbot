// tour.js — the first-run coach-mark tour: a spotlight that glides between
// the main parts of the app with a short explanation each (copy:
// guide-content.js TOUR_STEPS). Skippable any time (Esc / Skip), replayable
// from the Guide. Starts by itself once, right after onboarding.

import { html, render } from '../dom.js';
import { icon } from '../icons.js';
import { prefersReducedMotion } from '../util.js';
import { TOUR_STEPS } from '../guide-content.js';

const DONE_KEY = 'doppel.tour';
let active = null;

export function tourDone() {
  try { return localStorage.getItem(DONE_KEY) === 'done'; } catch { return false; }
}
function markDone() {
  try { localStorage.setItem(DONE_KEY, 'done'); } catch { /* ignore */ }
}

function visible(el) {
  if (!el) return false;
  const r = el.getBoundingClientRect();
  return r.width > 0 && r.height > 0 && r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth;
}

/** Start the tour (force: even when it was finished before). */
export function startTour({ force = false } = {}) {
  if (active || (!force && tourDone())) return;
  const steps = TOUR_STEPS.filter((s) => visible(document.querySelector(s.target)));
  if (!steps.length) return;

  const layer = document.createElement('div');
  layer.className = 'tour-layer';
  layer.setAttribute('role', 'dialog');
  layer.setAttribute('aria-modal', 'true');
  layer.setAttribute('aria-label', 'Quick tour');
  document.body.append(layer);
  const prevFocus = document.activeElement;
  let i = 0;

  const close = () => {
    if (!active) return;
    active = null;
    markDone();
    window.removeEventListener('resize', place);
    document.removeEventListener('keydown', onKey, true);
    layer.classList.add('closing');
    setTimeout(() => layer.remove(), prefersReducedMotion() ? 0 : 260);
    if (prevFocus && prevFocus.focus && document.contains(prevFocus)) { try { prevFocus.focus({ preventScroll: true }); } catch { /* ignore */ } }
  };
  const go = (n) => {
    if (n < 0) return;
    if (n >= steps.length) { close(); return; }
    i = n;
    draw();
  };
  const onKey = (e) => {
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); go(i + 1); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); go(i - 1); }
  };

  function draw() {
    const s = steps[i];
    const last = i === steps.length - 1;
    render(layer, html`
      <div class="tour-scrim" @click=${close}></div>
      <div class="tour-spot" aria-hidden="true"><span class="tour-pulse"></span></div>
      <div class="tour-card" data-key=${'tc-' + i}>
        <div class="tour-step">Step ${i + 1} of ${steps.length}</div>
        <h3>${s.title}</h3>
        <p>${s.text}</p>
        <div class="tour-foot">
          <div class="tour-dots" aria-hidden="true">${steps.map((_, n) => html`<span class=${n === i ? 'on' : n < i ? 'done' : ''}></span>`)}</div>
          <span class="grow"></span>
          ${last ? '' : html`<button class="btn btn-ghost btn-sm" @click=${close}>Skip</button>`}
          ${i > 0 ? html`<button class="btn btn-glass btn-sm" aria-label="Back" @click=${() => go(i - 1)}>${icon('arrow-left', 'ic-sm')}</button>` : ''}
          <button class="btn btn-primary btn-sm tour-next" @click=${() => go(i + 1)}>${last ? html`${icon('check', 'ic-sm')}Done` : html`Next${icon('arrow-right', 'ic-sm')}`}</button>
        </div>
      </div>`);
    place();
    requestAnimationFrame(() => { const b = layer.querySelector('.tour-next'); if (b) try { b.focus({ preventScroll: true }); } catch { /* ignore */ } });
  }

  function place() {
    const s = steps[i];
    const el = document.querySelector(s.target);
    const spot = layer.querySelector('.tour-spot');
    const card = layer.querySelector('.tour-card');
    if (!spot || !card) return;
    if (!visible(el)) { spot.style.opacity = '0'; card.style.left = `${(innerWidth - card.offsetWidth) / 2}px`; card.style.top = `${innerHeight / 3}px`; return; }
    const r = el.getBoundingClientRect();
    const pad = 6;
    spot.style.opacity = '1';
    spot.style.left = `${r.left - pad}px`;
    spot.style.top = `${r.top - pad}px`;
    spot.style.width = `${r.width + pad * 2}px`;
    spot.style.height = `${r.height + pad * 2}px`;
    const cw = card.offsetWidth;
    const ch = card.offsetHeight;
    const gap = 16;
    let left;
    let top;
    let side;
    if (r.right + gap + cw < innerWidth - 12) { left = r.right + gap; top = r.top + r.height / 2 - ch / 2; side = 'right'; }
    else if (r.bottom + gap + ch < innerHeight - 12) { left = r.right - cw; top = r.bottom + gap; side = 'below'; }
    else { left = r.left + r.width / 2 - cw / 2; top = r.top - gap - ch; side = 'above'; }
    left = Math.max(12, Math.min(left, innerWidth - cw - 12));
    top = Math.max(12, Math.min(top, innerHeight - ch - 12));
    card.style.left = `${left}px`;
    card.style.top = `${top}px`;
    card.dataset.side = side;
  }

  active = { close };
  window.addEventListener('resize', place);
  document.addEventListener('keydown', onKey, true);
  draw();
  requestAnimationFrame(() => layer.classList.add('in'));
}

export function stopTour() { if (active) active.close(); }
