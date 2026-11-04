// settings/compact-rows.js — Settings › Memory & recap: long-term memory
// defaults (memory.compact). Each chat reads every message since its last
// summary word for word (at least `keep`); every `every` new messages
// (`fastEvery` in a busy chat) the older ones fold into the summary. Chats
// can override it in Memory › What it reads. h = { s(), save(partial, section) }.

import { html } from '../../dom.js';
import { toggle } from '../../ui.js';
import { numberField } from '../controls.js';
import { debounce } from '../../util.js';
import { helpTip } from '../help-tip.js';

let pending = {};
const saveSoon = debounce((h) => { const p = pending; pending = {}; h.save({ memory: { compact: p } }, 'memory'); }, 600);

export function compactRows(h) {
  const k = (h.s().memory || {}).compact || {};
  const num = (key, min, max, unit, label) => numberField({ value: k[key], min, max, unit, label, width: 64,
    onChange: (n) => { pending[key] = n; saveSoon(h); } });
  return html`<div class="field-row">
      <div class="grow"><div class="field-label row gap-4">Long-term memory ${helpTip('compact')}</div>
        <div class="field-help">Older messages are folded into a short summary each chat keeps, so personas remember far back with a short prompt. Behind the scenes; each chat can change it.</div></div>
      ${toggle(!k.off, (v) => h.save({ memory: { compact: { off: !v } } }, 'memory'), { label: 'Long-term memory' })}
    </div>
    ${!k.off ? html`<div class="sub-rows">
      <div class="field-row"><div class="grow"><div class="field-label">Always reads word for word</div></div>${num('keep', 10, 200, 'messages', 'Messages always read word for word')}</div>
      <div class="field-row"><div class="grow"><div class="field-label">Summarise every</div>
        <div class="field-help">New messages before the older ones fold into the summary.</div></div>${num('every', 20, 1000, 'messages', 'Summarise every')}</div>
      <div class="field-row"><div class="grow"><div class="field-label">In a busy chat, every</div>
        <div class="field-help">Busy = at least ${k.fastMessages || 150} messages in ${k.fastHours || 3} hours: fewer, bigger summaries.</div></div>${num('fastEvery', 20, 2000, 'messages', 'In a busy chat, summarise every')}</div>
    </div>` : ''}`;
}
