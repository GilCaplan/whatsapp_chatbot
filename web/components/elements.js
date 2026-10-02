// elements.js — small self-managing custom elements. The morph renderer leaves
// their children alone, so they can update themselves (timers, lazy loading).
//
//   <chat-avatar jid name size kind src>   lazy WhatsApp avatar with gradient-initials fallback
//   <rel-time datetime>                    "2m ago", refreshes itself
//   <count-ring deadline total size>       countdown ring (deadline = epoch ms); fires "expired"
//   <load-more token>                      fires "visible" when scrolled into view

import { esc } from '../dom.js';
import { initials, gradientFor, relTime, toDate } from '../util.js';

// ── <chat-avatar> ──────────────────────────────────────────────────────
const avatarState = new Map(); // url -> 'ok' | 'none'
const inflight = new Set();
const io = 'IntersectionObserver' in window
  ? new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (e.isIntersecting) { io.unobserve(e.target); e.target._load(); }
    }
  }, { rootMargin: '240px' })
  : null;

const GROUP_SVG = '<svg class="ic" aria-hidden="true"><use href="#i-group"></use></svg>';
const USER_SVG = '<svg class="ic" aria-hidden="true"><use href="#i-user"></use></svg>';

class ChatAvatar extends HTMLElement {
  static get observedAttributes() { return ['jid', 'name', 'size', 'kind', 'src']; }

  connectedCallback() { this._render(); }
  disconnectedCallback() { if (io) io.unobserve(this); }
  attributeChangedCallback() { if (this.isConnected) this._render(); }

  _url() {
    const src = this.getAttribute('src');
    if (src) return src;
    const jid = this.getAttribute('jid');
    return jid ? `/api/wa/avatar?jid=${encodeURIComponent(jid)}` : '';
  }

  _render() {
    const name = this.getAttribute('name') || '';
    const jid = this.getAttribute('jid') || '';
    const size = parseInt(this.getAttribute('size'), 10) || 40;
    const kind = this.getAttribute('kind') || '';
    const url = this._url();
    const sig = `${name}|${jid}|${size}|${kind}|${url}`;
    if (sig === this._sig) return;
    this._sig = sig;
    const [c1, c2] = gradientFor(jid || name);
    const label = name.trim()
      ? `<span class="av-txt">${esc(initials(name))}</span>`
      : (kind === 'group' ? GROUP_SVG : USER_SVG);
    this.innerHTML = `<span class="av" style="--s:${size}px;background:linear-gradient(135deg,${c1},${c2})">${label}</span>`;
    if (!url) return;
    const st = avatarState.get(url);
    if (st === 'none') return;
    if (st === 'ok' || !io) this._load();
    else io.observe(this);
  }

  _load() {
    const url = this._url();
    if (!url || avatarState.get(url) === 'none') return;
    const wrap = this.firstElementChild;
    if (!wrap || wrap.querySelector('img')) return;
    const img = new Image();
    img.alt = '';
    img.decoding = 'async';
    img.onload = () => { avatarState.set(url, 'ok'); inflight.delete(url); img.classList.add('ok'); };
    img.onerror = () => { avatarState.set(url, 'none'); inflight.delete(url); img.remove(); };
    inflight.add(url);
    img.src = url;
    wrap.append(img);
  }
}

/** Forget cached "no avatar" results (e.g. after relinking). */
export function resetAvatarCache() { avatarState.clear(); }

// ── <rel-time> ─────────────────────────────────────────────────────────
const relNodes = new Set();
setInterval(() => { for (const n of relNodes) n._u(); }, 15000);

class RelTime extends HTMLElement {
  static get observedAttributes() { return ['datetime']; }
  connectedCallback() { relNodes.add(this); this._u(); }
  disconnectedCallback() { relNodes.delete(this); }
  attributeChangedCallback() { if (this.isConnected) this._u(); }
  _u() {
    const v = this.getAttribute('datetime');
    const txt = relTime(v);
    if (txt === this._t) return;
    this._t = txt;
    const d = toDate(v);
    this.innerHTML = d ? `<span title="${esc(d.toLocaleString())}">${esc(txt)}</span>` : '';
  }
}

// ── <count-ring> ───────────────────────────────────────────────────────
const rings = new Set();
let ringTimer = null;
function tickRings() {
  for (const r of rings) r._tick();
  if (!rings.size) { clearInterval(ringTimer); ringTimer = null; }
}

class CountRing extends HTMLElement {
  static get observedAttributes() { return ['deadline', 'total', 'size']; }
  connectedCallback() {
    rings.add(this);
    if (!ringTimer) ringTimer = setInterval(tickRings, 250);
    this._build();
  }
  disconnectedCallback() { rings.delete(this); }
  attributeChangedCallback() { if (this.isConnected) this._build(); }
  _build() {
    const size = parseInt(this.getAttribute('size'), 10) || 40;
    const stroke = Math.max(3, Math.round(size / 11));
    const r = (size - stroke) / 2;
    this._c = 2 * Math.PI * r;
    this._fired = false;
    this.innerHTML = `<span class="ring-wrap" style="width:${size}px;height:${size}px">
      <svg class="ring" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">
        <circle class="track" cx="${size / 2}" cy="${size / 2}" r="${r}" stroke-width="${stroke}"></circle>
        <circle class="bar" cx="${size / 2}" cy="${size / 2}" r="${r}" stroke-width="${stroke}" stroke-dasharray="${this._c.toFixed(2)}"></circle>
      </svg><span class="ring-label" style="font-size:${Math.max(10, Math.round(size * 0.3))}px"></span></span>`;
    this._bar = this.querySelector('.bar');
    this._label = this.querySelector('.ring-label');
    this._tick();
  }
  _tick() {
    if (!this._bar) return;
    const deadline = Number(this.getAttribute('deadline')) || 0;
    const total = Math.max(1, Number(this.getAttribute('total')) || 1);
    const left = Math.max(0, (deadline - Date.now()) / 1000);
    const frac = Math.min(1, left / total);
    this._bar.setAttribute('stroke-dashoffset', (this._c * (1 - frac)).toFixed(2));
    const secs = Math.ceil(left);
    const txt = secs >= 60 ? `${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, '0')}` : String(secs);
    if (this._label.textContent !== txt) this._label.textContent = txt;
    if (left <= 0 && !this._fired) {
      this._fired = true;
      this.dispatchEvent(new CustomEvent('expired', { bubbles: true }));
    }
  }
}

// ── <load-more> ────────────────────────────────────────────────────────
const sentinelIO = 'IntersectionObserver' in window
  ? new IntersectionObserver((entries) => {
    for (const e of entries) if (e.isIntersecting) e.target.dispatchEvent(new CustomEvent('visible'));
  }, { rootMargin: '400px' })
  : null;

class LoadMore extends HTMLElement {
  static get observedAttributes() { return ['token']; }
  connectedCallback() { if (sentinelIO) sentinelIO.observe(this); }
  disconnectedCallback() { if (sentinelIO) sentinelIO.unobserve(this); }
  attributeChangedCallback() {
    // Re-observe so a still-visible sentinel fires again after more rows load.
    if (this.isConnected && sentinelIO) { sentinelIO.unobserve(this); sentinelIO.observe(this); }
  }
}

export function defineElements() {
  if (!customElements.get('chat-avatar')) customElements.define('chat-avatar', ChatAvatar);
  if (!customElements.get('rel-time')) customElements.define('rel-time', RelTime);
  if (!customElements.get('count-ring')) customElements.define('count-ring', CountRing);
  if (!customElements.get('load-more')) customElements.define('load-more', LoadMore);
}
