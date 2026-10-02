// ui.js — shared UI primitives: toasts, sheets, drawers, popovers, confirm,
// and small template helpers (switch, segmented control, field, rings…).

import { html, render, mountView, isTemplate } from './dom.js';
import { icon } from './icons.js';
import { store } from './store.js';

// ── Toasts ────────────────────────────────────────────────────────────
const TOAST_ICON = { success: 'check', error: 'x', info: 'info', warn: 'warning' };

/**
 * toast('Saved', { type: 'success'|'error'|'info'|'warn', sub, timeout })
 * msg may be a string (escaped) or an html`` template.
 */
export function toast(msg, opts = {}) {
  const host = document.getElementById('toasts');
  if (!host) return;
  const type = opts.type || 'info';
  const el = document.createElement('div');
  el.className = `toast ${type}`;
  el.setAttribute('role', type === 'error' ? 'alert' : 'status');
  render(el, html`
    <span class="t-icon">${icon(TOAST_ICON[type] || 'info')}</span>
    <span class="t-msg">${isTemplate(msg) ? msg : String(msg)}${opts.sub ? html`<small>${opts.sub}</small>` : ''}</span>`);
  host.append(el);
  while (host.children.length > 4) host.firstElementChild.remove();
  const close = () => {
    if (el.classList.contains('closing')) return;
    el.classList.add('closing');
    setTimeout(() => el.remove(), 260);
  };
  el.addEventListener('click', close);
  const ms = opts.timeout ?? (type === 'error' ? 6500 : opts.sub ? 6000 : 3800);
  if (ms > 0) setTimeout(close, ms);
  return close;
}

// ── Overlay stack (sheets, drawers) ───────────────────────────────────
const stack = [];

document.addEventListener('keydown', (e) => {
  if (e.key !== 'Escape') return;
  if (openPop) { openPop.close(); e.preventDefault(); return; }
  const top = stack[stack.length - 1];
  if (top && top.dismissable) { top.close(); e.preventDefault(); }
});

store.subscribe(() => { for (const o of stack) o.update(); if (openPop) openPop.update(); });

/**
 * openSheet(viewFn, { kind: 'sheet'|'drawer', size: 'sm'|'wide', dismissable, onClose, label })
 * viewFn receives the controller ({ close, update }) and returns a template.
 */
export function openSheet(viewFn, opts = {}) {
  const host = document.getElementById('overlays');
  const kind = opts.kind || 'sheet';
  const layer = document.createElement('div');
  layer.className = `layer ${kind === 'drawer' ? 'drawer-layer' : ''}`;
  const scrim = document.createElement('div');
  scrim.className = 'scrim';
  const panel = document.createElement('div');
  panel.className = kind === 'drawer' ? 'drawer' : `sheet ${opts.size ? 'sheet-' + opts.size : ''}`;
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-modal', 'true');
  if (opts.label) panel.setAttribute('aria-label', opts.label);
  layer.append(scrim, panel);
  host.append(layer);

  const prevFocus = document.activeElement;
  let closed = false;
  const ctl = {
    dismissable: opts.dismissable !== false,
    el: panel,
    update: () => view.update(),
    close(result) {
      if (closed) return;
      closed = true;
      const i = stack.indexOf(ctl);
      if (i >= 0) stack.splice(i, 1);
      layer.classList.add('closing');
      view.destroy();
      setTimeout(() => layer.remove(), 240);
      if (prevFocus && prevFocus.focus && document.contains(prevFocus)) {
        try { prevFocus.focus({ preventScroll: true }); } catch { /* ignore */ }
      }
      if (opts.onClose) opts.onClose(result);
    },
    get closed() { return closed; },
  };
  const view = mountView(panel, () => viewFn(ctl));
  scrim.addEventListener('click', () => { if (ctl.dismissable) ctl.close(); });
  stack.push(ctl);

  requestAnimationFrame(() => {
    const f = panel.querySelector('[autofocus]') || panel.querySelector('input:not([type=checkbox]),textarea,select') || panel.querySelector('button');
    if (f && !opts.noAutofocus) { try { f.focus({ preventScroll: true }); } catch { /* ignore */ } }
  });
  return ctl;
}

export const openDrawer = (viewFn, opts = {}) => openSheet(viewFn, { ...opts, kind: 'drawer' });

export function closeAllSheets() {
  for (const o of stack.slice()) o.close();
}

/** Promise<boolean> confirmation sheet. */
export function confirmSheet({ title, body = '', confirm = 'Confirm', cancel = 'Cancel', danger = false, iconName } = {}) {
  return new Promise((resolve) => {
    let result = false;
    openSheet((ctl) => html`
      <div class="sheet-head">
        <div class=${'sheet-icon ' + (danger ? 'danger' : '')}>${icon(iconName || (danger ? 'warning' : 'info'))}</div>
        <div class="grow">
          <h2>${title}</h2>
          ${body ? html`<p>${body}</p>` : ''}
        </div>
      </div>
      <div class="sheet-actions">
        <button class="btn btn-ghost" @click=${() => ctl.close()}>${cancel}</button>
        <button class=${'btn ' + (danger ? 'btn-danger-solid' : 'btn-primary')} autofocus
          @click=${() => { result = true; ctl.close(); }}>${confirm}</button>
      </div>`, { size: 'sm', onClose: () => resolve(result), label: title });
  });
}

// ── Popovers / menus ──────────────────────────────────────────────────
let openPop = null;

/**
 * openPopover(anchorEl, viewFn, { align: 'start'|'end', width, onClose })
 * viewFn(ctl) → template. Closes on outside click, Esc, resize.
 */
export function openPopover(anchor, viewFn, opts = {}) {
  if (openPop) openPop.close();
  const el = document.createElement('div');
  el.className = 'popover';
  el.setAttribute('role', 'menu');
  if (opts.width) el.style.width = typeof opts.width === 'number' ? `${opts.width}px` : opts.width;
  document.body.append(el);

  let closed = false;
  const ctl = {
    el,
    update: () => view.update(),
    close() {
      if (closed) return;
      closed = true;
      if (openPop === ctl) openPop = null;
      document.removeEventListener('pointerdown', outside, true);
      window.removeEventListener('resize', ctl.close);
      el.classList.add('closing');
      view.destroy();
      setTimeout(() => el.remove(), 150);
      if (opts.onClose) opts.onClose();
    },
  };
  const view = mountView(el, () => viewFn(ctl));

  const place = () => {
    const r = anchor.getBoundingClientRect();
    const pw = el.offsetWidth;
    const ph = el.offsetHeight;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let left = opts.align === 'end' ? r.right - pw : r.left;
    if (opts.matchWidth) { el.style.width = `${r.width}px`; left = r.left; }
    left = Math.max(12, Math.min(left, vw - pw - 12));
    let top = r.bottom + 6;
    let origin = 'top';
    if (top + ph > vh - 12 && r.top - ph - 6 > 12) { top = r.top - ph - 6; origin = 'bottom'; }
    el.style.left = `${left}px`;
    el.style.top = `${Math.max(12, top)}px`;
    el.style.setProperty('--origin', `${origin} ${opts.align === 'end' ? 'right' : 'left'}`);
  };
  place();

  const outside = (e) => { if (!el.contains(e.target) && !anchor.contains(e.target)) ctl.close(); };
  setTimeout(() => document.addEventListener('pointerdown', outside, true), 0);
  window.addEventListener('resize', ctl.close);
  openPop = ctl;
  requestAnimationFrame(() => {
    const f = el.querySelector('input,[aria-checked="true"],button');
    if (f) try { f.focus({ preventScroll: true }); } catch { /* ignore */ }
  });
  return ctl;
}

/** items: [{ label, icon, sub, danger, checked, onClick } | 'sep' | { heading }] */
export function openMenu(anchor, items, opts = {}) {
  return openPopover(anchor, (ctl) => html`${items.filter(Boolean).map((it) => {
    if (it === 'sep') return html`<div class="menu-sep"></div>`;
    if (it.heading) return html`<div class="menu-label">${it.heading}</div>`;
    return html`<button class=${'menu-item ' + (it.danger ? 'danger' : '')} role="menuitem"
      @click=${() => { ctl.close(); it.onClick && it.onClick(); }}>
      ${it.icon ? icon(it.icon) : ''}
      <span class="grow">${it.label}${it.sub ? html`<span class="sub">${it.sub}</span>` : ''}</span>
      ${it.checked ? icon('check', 'check') : ''}
    </button>`;
  })}`, opts);
}

// ── Template helpers ──────────────────────────────────────────────────

/** iOS switch. onChange(checked, event) */
export function toggle(checked, onChange, { label = '', small = false, disabled = false, title = '' } = {}) {
  return html`<label class=${'switch ' + (small ? 'switch-sm' : '')} title=${title || label}>
    <input type="checkbox" role="switch" ?checked=${!!checked} ?disabled=${disabled} aria-label=${label || title}
      @change=${(e) => onChange && onChange(e.target.checked, e)} @click=${(e) => e.stopPropagation()}>
    <span class="track"></span><span class="thumb"></span>
  </label>`;
}

/** Segmented control. options: [{ value, label, icon, count }] */
export function seg(options, value, onChange, { cls = '', label = '' } = {}) {
  const i = Math.max(0, options.findIndex((o) => o.value === value));
  const none = !options.some((o) => o.value === value);
  return html`<div class=${'seg ' + cls + (none ? ' no-sel' : '')} role="radiogroup" aria-label=${label}
      style=${`--n:${options.length};--i:${i}`}>
    ${options.map((o) => html`<button type="button" role="radio" aria-checked=${String(o.value === value)} aria-pressed=${String(o.value === value)}
        title=${o.title || ''} @click=${() => { if (o.value !== value) onChange(o.value); }}>
      ${o.icon ? icon(o.icon) : ''}<span>${o.label}</span>${o.count != null ? html`<span class="count">${o.count}</span>` : ''}
    </button>`)}
  </div>`;
}

export function field({ label, help, icon: ic, optional, children, cls = '' }) {
  return html`<div class=${'field ' + cls}>
    ${label ? html`<label class="field-label">${ic ? icon(ic) : ''}${label}${optional ? html` <span class="opt">· optional</span>` : ''}</label>` : ''}
    ${children}
    ${help ? html`<div class="field-help">${help}</div>` : ''}
  </div>`;
}

/** A settings-style row: label + help on the left, control on the right. */
export function fieldRow({ label, help, control, stack = false }) {
  return html`<div class=${'field-row ' + (stack ? 'field-row-stack' : '')}>
    <div class="grow">
      <div class="field-label">${label}</div>
      ${help ? html`<div class="field-help">${help}</div>` : ''}
    </div>
    <div>${control}</div>
  </div>`;
}

export const spinner = (cls = '') => html`<span class=${'spinner ' + cls} role="progressbar" aria-label="Loading"></span>`;

/** Static progress ring (percent 0–100). */
export function progressRing(percent, { size = 44, stroke = 4, label = true } = {}) {
  const r = (size - stroke) / 2;
  const c = 2 * Math.PI * r;
  const p = Math.max(0, Math.min(100, percent || 0));
  return html`<span class="ring-wrap" style=${`width:${size}px;height:${size}px`}>
    <svg class="ring" width=${size} height=${size} viewBox=${`0 0 ${size} ${size}`}>
      <circle class="track" cx=${size / 2} cy=${size / 2} r=${r} stroke-width=${stroke}></circle>
      <circle class="bar" cx=${size / 2} cy=${size / 2} r=${r} stroke-width=${stroke}
        stroke-dasharray=${c.toFixed(2)} stroke-dashoffset=${(c * (1 - p / 100)).toFixed(2)}></circle>
    </svg>
    ${label ? html`<span class="ring-label" style=${`font-size:${Math.max(10, size * 0.24)}px`}>${Math.round(p)}%</span>` : ''}
  </span>`;
}

export function skeletonRows(n = 5) {
  return html`${Array.from({ length: n }, (_, i) => html`<div class="list-row" style="pointer-events:none">
    <div class="skeleton" style="width:42px;height:42px;border-radius:50%"></div>
    <div class="grow"><div class=${'skeleton sk-line ' + (i % 2 ? 'w40' : 'w60')}></div><div class="skeleton sk-line w80" style="height:9px"></div></div>
  </div>`)}`;
}

/** Wrap an async click handler: disables the button and shows a spinner. */
export function busy(fn) {
  return async (e) => {
    const btn = e && e.currentTarget;
    if (btn && btn.classList) { btn.classList.add('loading'); btn.disabled = true; }
    try { return await fn(e); }
    catch (err) { if (err && err.name !== 'ApiError' && err.name !== 'AbortError') { console.error(err); toast(err.message || 'Something went wrong', { type: 'error' }); } }
    finally { if (btn && btn.classList) { btn.classList.remove('loading'); btn.disabled = false; } }
  };
}
