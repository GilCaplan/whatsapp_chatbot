// qr-card.js — "Link WhatsApp" card: QR with scan frame + sweep + countdown
// ring, how-to steps, and a success morph once connected.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { store } from '../store.js';
import { api } from '../api.js';
import { toast, spinner, busy } from '../ui.js';
import { waMeta, statusDot } from './status.js';
import { prettyPhone } from '../util.js';

const NEEDS_PAIR = new Set(['logged_out', 'disconnected']);
const BLOCKED = new Set(['banned', 'outdated', 'replaced', 'error']);

/** createQRCard({ onConnected, showSteps }) → { view, mount, destroy } */
export function createQRCard({ showSteps = true, update = () => {} } = {}) {
  const local = { expired: false, fallback: '', lastQR: null, requested: false };
  let fallbackTimer = null;

  async function pair(quiet = true) {
    local.requested = true;
    local.expired = false;
    update();
    try { await api.wa.pair({ quiet }); } catch { /* already pairing or linked */ }
  }

  function mount() {
    const s = store.state.wa && store.state.wa.state;
    if (NEEDS_PAIR.has(s)) pair(true);
    fallbackTimer = setTimeout(() => {
      const st = store.state.wa && store.state.wa.state;
      if (!store.state.qr && st === 'awaiting_qr') { local.fallback = api.wa.qrURL(); update(); }
    }, 2500);
  }

  function destroy() { clearTimeout(fallbackTimer); }

  function steps() {
    return html`<ol class="scan-steps">
      <li><span>Open <b>WhatsApp</b> on your phone.</span></li>
      <li><span>Tap <b>Settings</b> (iPhone) or the <b>⋮ menu</b> (Android).</span></li>
      <li><span>Tap <b>Linked devices</b>, then <b>Link a device</b>.</span></li>
      <li><span>Point your phone at this screen to scan the code.</span></li>
    </ol>`;
  }

  function connectedView(wa) {
    const me = wa.me || {};
    return html`<div class="success-morph">
      <div class="me-wrap">
        <chat-avatar jid=${me.jid || ''} name=${me.pushName || 'You'} src=${me.avatarUrl || ''} size="96"></chat-avatar>
        <span class="tick"><svg class="ic check-draw" viewBox="0 0 24 24"><path d="m5 12.5 4.5 4.5L19 7.5"/></svg></span>
      </div>
      <div>
        <div class="h2">You're linked${me.pushName ? html`, ${me.pushName}` : ''}!</div>
        <div class="muted mt-4">${me.phone ? prettyPhone(me.phone) : 'WhatsApp is connected.'}</div>
      </div>
    </div>`;
  }

  function frame(wa, qr) {
    const state = wa.state;
    const src = qr ? qr.png : local.fallback;
    if (qr && qr !== local.lastQR) { local.lastQR = qr; local.expired = false; }
    const deadline = qr ? qr.receivedAt + (qr.expiresInSec || 20) * 1000 : 0;
    const showPairing = state === 'pairing';
    const waiting = !src;
    return html`<div class="qr-card">
      <div class=${'qr-frame ' + (local.expired ? 'expired' : '')}>
        <span class="corner tl"></span><span class="corner tr"></span><span class="corner bl"></span><span class="corner br"></span>
        ${waiting
          ? html`<div class="qr-skeleton"></div>`
          : html`<img src=${src} alt="WhatsApp QR code" @error=${() => { if (!qr) { local.fallback = ''; update(); } }}>`}
        ${!waiting && !local.expired && !showPairing ? html`<span class="sweep"></span>` : ''}
        ${waiting ? html`<div class="qr-overlay">
            ${NEEDS_PAIR.has(state) && local.requested === false
              ? html`<button class="btn btn-primary" @click=${() => pair(false)}>${icon('qr')}Show QR code</button>`
              : html`${spinner('spinner-lg')}<span class="small">Getting a fresh code…</span>`}
          </div>` : ''}
        ${showPairing ? html`<div class="qr-overlay">${spinner('spinner-lg')}<strong>Linking…</strong><span class="small">Keep WhatsApp open on your phone</span></div>` : ''}
        ${local.expired && !showPairing ? html`<div class="qr-overlay">
            <span class="small"><b>This code expired</b></span>
            <button class="btn btn-primary btn-sm" @click=${() => pair(false)}>${icon('refresh')}Get a new code</button>
          </div>` : ''}
      </div>
      <div class="qr-meta">
        ${qr && !local.expired && !showPairing
          ? html`<count-ring deadline=${deadline} total=${qr.expiresInSec || 20} size="30"
                @expired=${() => { local.expired = true; update(); }}></count-ring>
              <span>Code refreshes automatically</span>`
          : html`${statusDot(state)}<span>${waMeta(state).label}</span>`}
      </div>
    </div>`;
  }

  function view() {
    const wa = store.state.wa || { state: 'connecting' };
    if (wa.state === 'connected') return connectedView(wa);
    if (BLOCKED.has(wa.state)) {
      const m = waMeta(wa.state);
      return html`<div class="col gap-12" style="align-items:center;text-align:center">
        <div class="banner danger" style="text-align:left">${icon('warning')}<div><strong>${m.label}.</strong> ${m.help}
          ${wa.lastError ? html`<div class="small faint mt-4">${wa.lastError}</div>` : ''}</div></div>
        <div class="row" style="justify-content:center">
          <button class="btn btn-primary" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting…'); })}>${icon('refresh')}Reconnect</button>
          ${wa.state !== 'outdated' ? html`<button class="btn btn-glass" @click=${() => pair(false)}>${icon('qr')}Link again</button>` : ''}
        </div>
      </div>`;
    }
    return html`<div class=${showSteps ? 'qr-layout' : ''}>
      ${frame(wa, store.state.qr)}
      ${showSteps ? html`<div class="qr-steps">
        <div class="h3 mb-12">How to scan</div>
        ${steps()}
        <div class="banner info mt-16">${icon('lock')}<div class="small">Doppel links like WhatsApp Web. Your messages stay on this Mac — nothing is sent anywhere else except the AI you choose.</div></div>
      </div>` : ''}
    </div>`;
  }

  return { view, mount, destroy, pair };
}
