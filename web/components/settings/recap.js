// settings/recap.js — the daily recap rows of Settings › Memory & recap
// (Engineer B). h = { s(), save(partial, section), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { toggle, fieldRow } from '../../ui.js';
import { providerLabel } from '../status.js';

const KEEP = [7, 30, 90, 365];

export function recapRows(h) {
  const s = h.s();
  const r = s.recap || {};
  const n = s.notifications || {};
  const save = (recap) => h.save({ recap }, 'memory');
  const provider = (s.llm && s.llm.defaultProvider) || 'ollama';
  return html`
    ${fieldRow({
      label: html`${icon('text', 'ic-sm')}Daily recap`,
      help: 'A few lines per chat at the end of the day: what was said, plans made and what to follow up on.',
      control: toggle(r.enabled, (v) => save({ enabled: v }), { label: 'Daily recap' }),
    })}
    ${r.enabled ? html`<div class="sub-rows">
      ${fieldRow({
        label: 'Time',
        help: "In this Mac's time zone. Chats with fewer than three new messages are skipped.",
        control: html`<input class="input input-sm time-input" type="time" aria-label="Recap time" .value=${r.time || '21:00'}
          @change=${(e) => { if (/^\d{2}:\d{2}$/.test(e.target.value)) save({ time: e.target.value }); }}>`,
      })}
      ${fieldRow({
        label: 'Keep recaps for',
        control: html`<select class="select input-sm" style="width:auto" aria-label="Keep recaps for" @change=${(e) => save({ keepDays: parseInt(e.target.value, 10) })}>
          ${KEEP.map((d) => html`<option value=${d} ?selected=${r.keepDays === d}>${d === 365 ? 'a year' : `${d} days`}</option>`)}
          ${r.keepDays && !KEEP.includes(r.keepDays) ? html`<option value=${r.keepDays} selected>${r.keepDays} days</option>` : ''}
        </select>`,
      })}
      ${fieldRow({
        label: 'Notify me when it is ready',
        control: toggle(n.recap, (v) => h.save({ notifications: { recap: v } }, 'memory'), { label: 'Recap notification' }),
      })}
      ${provider !== 'ollama' ? html`<div class="field-help mt-8">${icon('info', 'ic-sm')} Recaps are written by ${providerLabel(provider)}, so it reads the day's messages of each chat.</div>` : ''}
    </div>` : ''}`;
}
