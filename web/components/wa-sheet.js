// wa-sheet.js — WhatsApp connection sheet (opened from the status pill,
// the dashboard and settings).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { store } from '../store.js';
import { api } from '../api.js';
import { openSheet, confirmSheet, toast, busy } from '../ui.js';
import { createQRCard } from './qr-card.js';
import { waMeta, statusDot } from './status.js';
import { resetAvatarCache } from './elements.js';
import { duration, toDate } from '../util.js';

export async function logoutAndRelink() {
  const ok = await confirmSheet({
    title: 'Log out of WhatsApp?',
    body: 'Doppel will stop replying and this Mac will be removed from your phone\'s Linked devices. You can link again right away with a new QR code.',
    confirm: 'Log out',
    danger: true,
    iconName: 'logout',
  });
  if (!ok) return false;
  await api.wa.logout();
  resetAvatarCache();
  toast('Logged out. Scan the new code to link again.', { type: 'success' });
  try { await api.wa.pair({ quiet: true }); } catch { /* server may start pairing itself */ }
  return true;
}

export function openWASheet() {
  let qr = null;
  const sheet = openSheet((ctl) => {
    const wa = store.state.wa || {};
    const m = waMeta(wa.state);
    const connected = wa.state === 'connected';
    const since = toDate(wa.since);
    if (!qr) { qr = createQRCard({ showSteps: true, update: () => ctl.update() }); queueMicrotask(() => qr.mount()); }
    return html`
      <div class="sheet-head">
        <div class="sheet-icon">${icon('phone')}</div>
        <div class="grow">
          <h2>WhatsApp</h2>
          <p class="row gap-8">${statusDot(wa.state)}<span>${m.label}${connected && since ? html` · for ${duration((Date.now() - since) / 1000)}` : ''}</span></p>
        </div>
        <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
      </div>
      ${connected
        ? html`<div class="col gap-16" style="align-items:center">
            ${qr.view()}
            ${wa.me && wa.me.jid ? html`<dl class="kv" style="align-self:stretch">
              <dt>Account</dt><dd>${wa.me.jid}</dd>
            </dl>` : ''}
          </div>`
        : qr.view()}
      <div class="sheet-actions">
        ${connected ? html`
          <button class="btn btn-danger" @click=${busy(async () => { if (await logoutAndRelink()) ctl.update(); })}>${icon('logout')}Log out & relink</button>
          <span class="grow"></span>
          <button class="btn btn-glass" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting to WhatsApp…'); })}>${icon('refresh')}Reconnect</button>
        ` : html`
          <button class="btn btn-glass" @click=${busy(async () => { await api.wa.reconnect(); toast('Reconnecting to WhatsApp…'); })}>${icon('refresh')}Try reconnecting</button>
        `}
        <button class="btn btn-primary" @click=${() => ctl.close()}>Done</button>
      </div>`;
  }, { size: 'wide', label: 'WhatsApp connection', onClose: () => qr && qr.destroy() });
  return sheet;
}
