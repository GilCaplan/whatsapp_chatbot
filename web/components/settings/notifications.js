// settings/notifications.js — Settings › Notifications (desktop notifications: macOS, Windows toasts, Linux notify-send).
// Wave 3 owner: Engineer B. h = { s(), save(partial, section), sectionHead(id, title, sub, color), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { toggle, fieldRow, toast, busy } from '../../ui.js';
import { copyText, platformOf } from '../../util.js';
import { store } from '../../store.js';

const BREW = 'brew install terminal-notifier';
const LIBNOTIFY = 'sudo apt install libnotify-bin';

/** A command with a copy button. */
const copyCmd = (cmd) => html`<div class="row gap-6 mt-8"><code class="url-pill">${cmd}</code>
  <button class="btn btn-ghost btn-icon btn-sm" aria-label="Copy the command" @click=${() => copyText(cmd).then(() => toast('Copied', { type: 'success' }))}>${icon('copy')}</button></div>`;

const EVENTS = [
  { key: 'handoff', label: 'A chat needs you', help: 'Someone brought up money, health, meeting up and so on. Always with sound.' },
  { key: 'approvals', label: 'A reply waits for your OK', help: 'Approve and Co-pilot chats.' },
  { key: 'goals', label: 'A goal or mission is reached' },
  { key: 'whatsapp', label: 'WhatsApp disconnects', help: 'Only when it stays offline for more than 30 seconds.' },
  { key: 'recap', label: 'The daily recap is ready' },
];

// What this server uses to show notifications (asked once per page load).
let probe = null;
let probing = false;

function backendHint(h) {
  if (!probe && !probing) {
    probing = true;
    api.system.notifyTest({ quiet: true }, true)
      .then((r) => { probe = r || { backend: 'none' }; })
      .catch((e) => { probe = { backend: e && e.status === 501 ? 'unwired' : 'none' }; })
      .finally(() => { probing = false; h.update(); });
  }
  if (!probe) return '';
  const demo = probe.events === false
    ? html`<div class="banner info mt-8 small">${icon('info')}<div><strong>Demo mode.</strong> WhatsApp is simulated, so only test notifications are shown.</div></div>` : '';
  switch (probe.backend) {
    case 'terminal-notifier':
      return html`${demo}<div class="banner success mt-8 small">${icon('check')}<div>Using terminal-notifier: clicking a notification opens the right chat.</div></div>`;
    case 'osascript':
      return html`${demo}<div class="banner info mt-8 small notify-hint">${icon('info')}<div class="grow">
        Notifications come from <b>Script Editor</b>, so clicking one doesn't open Doppel. For clickable notifications that open the right chat, install terminal-notifier and restart Doppel:
        ${copyCmd(BREW)}
        <div class="mt-8 muted">Nothing showing up? Allow notifications for Script Editor in System Settings › Notifications.</div>
      </div></div>`;
    case 'notify-send':
      return html`${demo}<div class="banner info mt-8 small">${icon('info')}<div>Using notify-send. Clicking a notification doesn't open Doppel; open it from your app menu or browser.</div></div>`;
    case 'powershell':
      return html`${demo}<div class="banner info mt-8 small">${icon('info')}<div>Notifications appear as coming from <b>Windows PowerShell</b>; clicking one opens Doppel in your browser.
        <div class="mt-8 muted">Nothing showing up? Check Settings › System › Notifications and turn off Do not disturb (Focus assist).</div></div></div>`;
    case 'dry-run':
      return html`${demo}<div class="banner warn mt-8 small">${icon('warning')}<div>Notifications are only written to the log on this server (DOPPEL_NOTIFY=dry).</div></div>`;
    default:
      switch (platformOf(store.state.health)) {
        case 'linux':
          return html`<div class="banner warn mt-8 small notify-hint">${icon('warning')}<div class="grow">Notifications need notify-send. Install it (on Ubuntu or Debian with the command below; the package is called libnotify on most other systems) and restart Doppel:
            ${copyCmd(LIBNOTIFY)}</div></div>`;
        case 'windows':
          return html`<div class="banner warn mt-8 small">${icon('warning')}<div>Notifications need Windows PowerShell, which was not found on this PC.</div></div>`;
        default:
          return html`<div class="banner warn mt-8 small">${icon('warning')}<div>Notifications aren't available on this computer.</div></div>`;
      }
  }
}

export function notificationsSection(h) {
  const s = h.s();
  const n = s.notifications || {};
  const save = (notifications) => h.save({ notifications }, 'notifications');
  return html`<section class="card section" id="sec-notifications" data-section="notifications">
    ${h.sectionHead('notifications', 'Notifications', 'A notification on this computer when a chat needs you, a reply waits for your OK or a goal is reached.', 'linear-gradient(135deg,#f97316,#f43f5e)')}
    ${fieldRow({
      label: 'Show notifications on this computer',
      help: 'Doppel taps you on the shoulder only for things that need you.',
      control: toggle(n.enabled, (v) => save({ enabled: v }), { label: 'Show notifications' }),
    })}
    ${n.enabled ? html`<div class="sub-rows">
      ${EVENTS.map((ev) => fieldRow({
        label: ev.label, help: ev.help || '',
        control: toggle(n[ev.key], (v) => save({ [ev.key]: v }), { label: ev.label }),
      }))}
      ${fieldRow({ label: 'Play a sound', control: toggle(n.sound, (v) => save({ sound: v }), { label: 'Play a sound' }) })}
    </div>` : ''}
    <div class="field-row field-row-stack">
      <div class="row">
        <div class="grow"><div class="field-label">Try it</div><div class="field-help">Shows a test notification right now, even when notifications are off above.</div></div>
        <button class="btn btn-glass btn-sm" ?disabled=${probe && (probe.backend === 'none' || probe.backend === 'unwired')} @click=${busy(async () => {
          const r = await api.system.notifyTest();
          if (r && r.backend === 'dry-run') toast('Test written to the server log (dry run)', { type: 'info' });
          else toast(`Sent. Look at the ${{ darwin: 'top right', windows: 'bottom right' }[platformOf(store.state.health)] || 'top'} of your screen`, { type: 'success' });
        })}>${icon('megaphone')}Send a test</button>
      </div>
      ${backendHint(h)}
    </div>
  </section>`;
}
