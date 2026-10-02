// initiate-controls.js — "Start a conversation" in the chat panel: a manual
// opener button (with an optional topic) and the automatic check-in toggle.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { toggle } from '../ui.js';
import { clockTime, relTime } from '../util.js';

const hours = (h) => (h >= 48 && h % 24 === 0 ? `${h / 24} days` : `${h} hour${h === 1 ? '' : 's'}`);

/** Plain-English summary of a proactive block: "After 48 hours of quiet · at most 1 per day". */
export function checkInSummary(pr) {
  if (!pr) return '';
  const max = pr.maxPerDay || 1;
  return `After ${hours(pr.afterHours || 0)} of quiet · at most ${max} per day`;
}

/**
 * opts: {
 *   c: ChatAssignment view (enabled, approvalMode, nextCheckInAt, kind), p: persona,
 *   away: bool, cb: chat behaviour view (effective.proactive, sources.proactive) or null,
 *   hint, busy, onHint(text), onStart(), onToggleCheckIns(on), onOpenSettings()
 * }
 */
export function initiateControls({ c, p, away, cb, hint, busy, onHint, onStart, onToggleCheckIns, onOpenSettings }) {
  const name = p ? p.name : 'the persona';
  const disabled = !c.enabled || !p;
  const label = c.approvalMode ? 'Write an opener for approval' : 'Start the conversation';
  const pr = cb && cb.effective ? cb.effective.proactive : null;
  const fromChat = cb && cb.sources && cb.sources.proactive === 'chat';
  const due = c.nextCheckInAt;
  return html`<div class="initiate">
    <p class="small muted initiate-lead">${name} writes the first message now — a natural opener that fits the chat${c.goal && c.goal.state !== 'reached' ? ' and quietly works towards the goal' : ''}.</p>
    <div class="composer">
      <input class="input" placeholder="What should it be about? (optional)" maxlength="300" .value=${hint || ''}
        ?disabled=${disabled} aria-label="What should the opener be about"
        @input=${(e) => onHint(e.target.value)}
        @keydown=${(e) => { if (e.key === 'Enter' && !e.shiftKey && !disabled) { e.preventDefault(); onStart(); } }}>
      <button class=${'btn btn-primary ' + (busy ? 'loading' : '')} ?disabled=${disabled || busy} @click=${onStart}
        title=${c.approvalMode ? 'The opener waits in Approvals until you send it' : `${name} types and sends an opener now`}>${icon('message')}${label}</button>
    </div>
    ${!c.enabled ? html`<div class="banner warn small mt-8">${icon('pause')}<div>Replies are off for this chat. Turn them on to let ${name} start a conversation.</div></div>`
      : away ? html`<div class="banner info small mt-8">${icon('snooze')}<div>Away is on — tapping the button still sends this one opener, because you asked.</div></div>`
      : c.approvalMode ? html`<div class="field-help mt-4">Approval is on: the opener waits in Approvals until you send it.</div>` : ''}

    <div class="field-row initiate-auto mt-12">
      <div class="grow">
        <div class="field-label">Check in by itself when it's been quiet</div>
        <div class="field-help">${pr ? (pr.enabled ? checkInSummary(pr) : 'Off') : '…'}${pr && !fromChat ? ' · same as the defaults' : ''}
          · <button type="button" class="link-btn" @click=${onOpenSettings}>Settings</button></div>
        ${pr && pr.enabled && due ? html`<div class="field-help initiate-due">${icon('clock', 'ic-sm')}Next check-in around ${clockTime(due)} <span class="faint">(${relTime(due)})</span></div>` : ''}
      </div>
      ${toggle(!!(pr && pr.enabled), (v) => onToggleCheckIns(v), { label: 'Check in by itself when it has been quiet', disabled: !pr })}
    </div>
    ${pr && pr.enabled ? html`<div class="banner warn small">${icon('warning')}<div><strong>Sends messages nobody asked for — use sparingly.</strong> Lots of unprompted messages can get a WhatsApp account flagged. It only ever checks in on chats that have talked before.</div></div>` : ''}
  </div>`;
}
