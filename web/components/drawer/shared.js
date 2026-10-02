// drawer/shared.js — helpers shared by the chat panel shell, its tabs and
// other pages (chats, dashboard, approvals): optimistic PATCH, chat bubbles
// and the "Away" (snooze) helpers. chat-drawer.js re-exports the public ones.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store, loadChats } from '../../store.js';
import { openSheet, toast } from '../../ui.js';
import { clockTime } from '../../util.js';
import { mentionNodes } from '../mentions.js';

/** Merge a `behavior` patch (absent = keep, null = inherit) into overrides. */
function mergeOverrides(cur, patch) {
  if (patch === null) return {};
  const out = { ...(cur || {}) };
  for (const [k, v] of Object.entries(patch || {})) {
    if (v === null) delete out[k]; else out[k] = v;
  }
  return out;
}

/** Optimistic PATCH of an assignment. */
export async function patchChat(key, body) {
  const before = store.state.chats;
  store.set({ chats: before.map((c) => {
    if (c.key !== key) return c;
    const next = { ...c, ...body };
    if ('behavior' in body) next.behavior = mergeOverrides(c.behavior, body.behavior);
    return next;
  }) });
  try {
    await api.chats.patch(key, body);
  } catch (e) {
    store.set({ chats: before });
    throw e;
  } finally {
    loadChats().catch(() => {});
  }
}

export function bubbles(messages, { group = false } = {}) {
  let prev = null;
  return messages.map((m) => {
    const me = m.speaker === 'me';
    const same = prev && prev.speaker === m.speaker && (prev.name || '') === (m.name || '');
    prev = m;
    return html`<div class=${'bubble-row ' + (me ? 'me ' : '') + (same ? 'same' : '')} data-key=${'m' + (m.id || m.ts)}>
      <div class="bubble">${group && !me && m.name && !same ? html`<span class="who">${m.name}</span>` : ''}${mentionNodes(m.text, m.mentions)}<span class="meta">${m.crossUsed && m.crossUsed.length ? html`<span class="cross-mark" title=${'Used what it knows about ' + m.crossUsed.join(', ') + ' from other chats'} aria-label="Used context from other chats">${icon('link', 'ic-sm')}</span>` : ''}${m.kind === 'edited' ? html`<span class="edited-mark" title="Fixed a typo by editing the message">Edited</span>` : ''}${m.fromBot ? icon('sparkles', 'ic-sm') : ''}${clockTime(m.ts)}</span></div>
    </div>`;
  });
}

// ── Away (snooze) ─────────────────────────────────────────────────────
export const FOREVER = '2100-01-01T00:00:00Z';
const isForever = (iso) => { const d = new Date(iso); return !isNaN(d) && d.getFullYear() >= 2099; };

/** "until you turn it off" / "until 15:30" / "until tomorrow 09:00" / "until Fri 12 Oct, 09:00" */
export function awayUntilText(iso) {
  if (!iso) return '';
  if (isForever(iso)) return 'until you turn it off';
  const d = new Date(iso);
  const now = new Date();
  const t = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  const day = (x) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(d) - day(now)) / 86400000);
  if (diff <= 0) return `until ${t}`;
  if (diff === 1) return `until tomorrow ${t}`;
  if (diff < 7) return `until ${d.toLocaleDateString(undefined, { weekday: 'long' })} ${t}`;
  return `until ${d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}, ${t}`;
}

export function tomorrowAt(h) {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  d.setHours(h, 0, 0, 0);
  return d;
}

const pad2 = (n) => String(n).padStart(2, '0');
const localInput = (d) => `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}T${pad2(d.getHours())}:${pad2(d.getMinutes())}`;

/** Ask for a custom "away until" time. Resolves to a Date or null. */
export function pickAwayTime() {
  return new Promise((resolve) => {
    let value = localInput(new Date(Date.now() + 2 * 3600 * 1000));
    let result = null;
    openSheet((ctl) => html`
      <div class="sheet-head">
        <div class="sheet-icon">${icon('snooze')}</div>
        <div class="grow"><h2>Away until…</h2><p>The persona won't reply in this chat until then.</p></div>
      </div>
      <input class="input" type="datetime-local" value=${value} min=${localInput(new Date())} autofocus aria-label="Away until"
        @input=${(e) => { value = e.target.value; }}>
      <div class="sheet-actions">
        <button class="btn btn-ghost" @click=${() => ctl.close()}>Cancel</button>
        <button class="btn btn-primary" @click=${() => {
          const d = new Date(value);
          if (isNaN(d) || d.getTime() <= Date.now()) { toast('Pick a time in the future', { type: 'warn' }); return; }
          result = d; ctl.close();
        }}>${icon('snooze')}Set away</button>
      </div>`, { size: 'sm', label: 'Away until', onClose: () => resolve(result) });
  });
}
