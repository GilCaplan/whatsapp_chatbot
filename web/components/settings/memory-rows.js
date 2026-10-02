// settings/memory-rows.js — Settings › Memory & recap: the memory rows
// (wave 3, Engineer A). h = { s(), save(partial, section), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store } from '../../store.js';
import { toggle, toast, confirmSheet, seg } from '../../ui.js';
import { helpTip } from '../help-tip.js';
import { SENSITIVE_CATS, catGlyph } from '../handoff-meta.js';
import { numberField } from '../controls.js';
import { debounce } from '../../util.js';

const MODE_OPTS = [
  { value: 'off', label: 'Off' },
  { value: 'discreet', label: 'Discreet', icon: 'eye-off' },
  { value: 'open', label: 'Open', icon: 'message' },
];
const GROUP_HELP = {
  off: 'Groups never use what people told a persona one-to-one.',
  discreet: 'The persona knows what people told it privately and stays consistent, but never brings it up in the group unless they do.',
  open: 'It may refer to it lightly with that person, never in front of others.',
};
const DM_HELP = {
  off: 'Private chats never use what happened in groups.',
  discreet: 'It understands references to the group and keeps plans made there, but only brings the group up if they do.',
  open: 'It may mention what happened in groups you share, since you were both there.',
};

let pendingNums = {};
const saveNums = debounce((h) => { const n = pendingNums; pendingNums = {}; h.save({ memory: { cross: n } }, 'memory'); }, 600);

function crossRows(h, c) {
  const save = (partial) => h.save({ memory: { cross: partial } }, 'memory');
  const sens = c.sensitive || {};
  const num = (key, value, min, max, unit, label) => numberField({ value, min, max, unit, label, width: 64,
    onChange: (n) => { pendingNums[key] = n; saveNums(h); } });
  const kept = SENSITIVE_CATS.filter((x) => sens[x.id]).length;
  return html`<div class="sub-rows cross-settings">
    <div class="field-row field-row-stack">
      <div><div class="field-label">In groups, private chats are</div>
        <div class="field-help">${GROUP_HELP[c.groupMode] || ''}</div></div>
      ${seg(MODE_OPTS, c.groupMode || 'discreet', (v) => save({ groupMode: v }), { cls: 'seg-sm', label: 'In groups, private chats are' })}
    </div>
    <div class="field-row field-row-stack">
      <div><div class="field-label">In private chats, shared groups are</div>
        <div class="field-help">${DM_HELP[c.dmMode] || ''}</div></div>
      ${seg(MODE_OPTS, c.dmMode || 'open', (v) => save({ dmMode: v }), { cls: 'seg-sm', label: 'In private chats, shared groups are' })}
    </div>
    <div class="field-row field-row-stack">
      <div><div class="field-label">Never crosses over <span class="chip chip-sm">${kept} of ${SENSITIVE_CATS.length}</span></div>
        <div class="field-help">Things about these topics stay in the chat they were said in, whatever the mode. You can still unlock a single memory in a chat's Memory tab.</div></div>
      <div class="cat-grid">${SENSITIVE_CATS.map((x) => html`<button type="button" class="cat-tile" aria-pressed=${String(!!sens[x.id])}
          @click=${() => save({ sensitive: { [x.id]: !sens[x.id] } })}>
        <span class="cat-ic">${catGlyph(x.id)}</span>
        <span class="grow"><span class="cat-name">${x.label}</span><span class="cat-line">${x.line}</span></span>
        <span class="cat-state">${sens[x.id] ? 'Kept private' : 'May cross'}</span>
      </button>`)}</div>
    </div>
    <details class="cross-advanced">
      <summary class="small">Advanced</summary>
      <div class="field-row"><div class="grow"><div class="field-label">Only things from the last</div>
        <div class="field-help">Older notes are left behind unless pinned.</div></div>${num('freshDays', c.freshDays, 1, 90, 'days', 'Days')}</div>
      <div class="field-row"><div class="grow"><div class="field-label">People per reply</div>
        <div class="field-help">How many of the people talking it thinks about at once.</div></div>${num('maxPeople', c.maxPeople, 1, 8, '', 'People per reply')}</div>
      <div class="field-row"><div class="grow"><div class="field-label">Notes per reply</div>
        <div class="field-help">Fewer notes keep replies focused (and small models sharp).</div></div>${num('maxItems', c.maxItems, 1, 16, '', 'Notes per reply')}</div>
    </details>
  </div>`;
}

async function forgetAll() {
  const chats = store.state.chats || [];
  const ok = await confirmSheet({
    title: 'Forget all memories?',
    body: `Every persona forgets everything it learned about people in all ${chats.length === 1 ? 'your chat' : `${chats.length} chats`}, including what you added yourself. This can't be undone.`,
    confirm: 'Forget everything', danger: true, iconName: 'trash',
  });
  if (!ok) return;
  let failed = 0;
  for (const c of chats) {
    try { await api.chats.clearMemories(c.key, { quiet: true }); } catch { failed++; }
  }
  toast(failed ? `Couldn't clear ${failed} chat${failed === 1 ? '' : 's'} — try again` : 'All memories forgotten', { type: failed ? 'error' : 'success' });
}

export function memoryRows(h) {
  const s = h.s();
  const on = !!(s.memory && s.memory.enabled);
  const cross = (s.memory && s.memory.cross) || { enabled: true, groupMode: 'discreet', dmMode: 'open', sensitive: {}, freshDays: 14, maxPeople: 4, maxItems: 8 };
  return html`
    <div class="field-row">
      <div class="grow">
        <div class="field-label row gap-4">Remember what people mention ${helpTip('memory')}</div>
        <div class="field-help">Personas note lasting things people tell them — a new job, a birthday, a favourite team — and bring them up naturally later. Each chat can turn this off in its <b>Memory</b> tab, where you can also see, pin, edit and delete every memory.</div>
      </div>
      ${toggle(on, (v) => h.save({ memory: { enabled: v } }, 'memory'), { label: 'Remember what people mention' })}
    </div>
    ${on ? html`<div class="field-row">
      <div class="grow">
        <div class="field-label row gap-4">Share context between chats ${helpTip('cross')}</div>
        <div class="field-help">A persona can draw on what it learned in its other chats: in a group, what people told it privately; in a private chat, what happened in groups you share. Summaries only, never the messages themselves.</div>
      </div>
      ${toggle(cross.enabled, (v) => h.save({ memory: { cross: { enabled: v } } }, 'memory'), { label: 'Share context between chats' })}
    </div>
    ${cross.enabled ? crossRows(h, cross) : ''}` : ''}
    <div class="field-row">
      <div class="grow">
        <div class="field-label">${icon('lock', 'ic-sm')}Privacy</div>
        <div class="field-help">Memories are stored only on this computer. With Ollama nothing leaves your computer; a persona using Claude or OpenAI sends the messages it learns from to that service.</div>
      </div>
      <button class="btn btn-ghost btn-sm danger-text" @click=${forgetAll}>${icon('trash')}Forget all memories</button>
    </div>`;
}
