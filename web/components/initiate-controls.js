// initiate-controls.js — "Start a conversation" in the chat panel: a manual
// opener button (with an optional topic), the automatic check-in toggle and
// what is scheduled (proactive.schedule).

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

/** "15 min", "2 hours", "1 day 6 hours". */
export function minutesText(min) {
  const m = Math.max(0, Math.round(min || 0));
  if (m < 60) return `${m} min`;
  if (m < 1440) return m % 60 ? `${Math.floor(m / 60)} h ${m % 60} min` : `${m / 60} hour${m === 60 ? '' : 's'}`;
  const d = Math.floor(m / 1440);
  const h = Math.round((m % 1440) / 60);
  return `${d} day${d === 1 ? '' : 's'}${h ? ` ${h} h` : ''}`;
}

const REPEAT_TEXT = {
  fixed: (s) => `every ${minutesText(s.everyMin)}`,
  uniform: (s) => `every ${minutesText(s.everyMin)} ± ${minutesText(s.spreadMin)}`,
  normal: (s) => `about every ${minutesText(s.everyMin)} (bell curve)`,
  poisson: (s) => `at random, ${minutesText(s.everyMin)} apart on average`,
};

/** Plain-English summary of a schedule: "When it starts · 2 set times · every 2 hours". "" = nothing planned. */
export function scheduleSummary(sc) {
  if (!sc) return '';
  const bits = [];
  if (sc.onStart) bits.push('When it starts');
  const future = (sc.times || []).filter((t) => Date.parse(t) > Date.now()).length;
  if (future) bits.push(future === 1 ? '1 set time' : `${future} set times`);
  const rep = REPEAT_TEXT[sc.repeat];
  if (rep) bits.push(rep({ everyMin: sc.everyMin || 120, spreadMin: sc.spreadMin || 0 }));
  if (!bits.length) return '';
  const s = bits.join(' · ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** "today 14:00", "Tue 09:30", "12 Dec 18:00". */
export function whenText(v) {
  const d = new Date(v);
  if (isNaN(d)) return '';
  const now = new Date();
  const time = clockTime(d);
  if (d.toDateString() === now.toDateString()) return `today ${time}`;
  if (Math.abs(d - now) < 6 * 86400000) return `${d.toLocaleDateString(undefined, { weekday: 'short' })} ${time}`;
  return `${d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })} ${time}`;
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
    <div class="field-row initiate-auto">
      <div class="grow">
        <div class="field-label">Scheduled messages</div>
        <div class="field-help">${pr ? (scheduleSummary(pr.schedule) || 'None') : '…'}${pr && !fromChat ? ' · same as the defaults' : ''}
          · <button type="button" class="link-btn" @click=${onOpenSettings}>${pr && scheduleSummary(pr.schedule) ? 'Change' : 'Plan some'}</button></div>
        ${c.nextScheduledAt ? html`<div class="field-help initiate-due">${icon('calendar', 'ic-sm')}Next one ${whenText(c.nextScheduledAt)} <span class="faint">(${relTime(c.nextScheduledAt)})</span></div>` : ''}
      </div>
    </div>
    ${pr && pr.enabled ? html`<div class="banner warn small">${icon('warning')}<div><strong>Sends messages nobody asked for — use sparingly.</strong> Lots of unprompted messages can get a WhatsApp account flagged. It only ever checks in on chats that have talked before.</div></div>` : ''}
  </div>`;
}
