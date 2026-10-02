// settings/memory-rows.js — Settings › Memory & recap: the memory rows
// (wave 3, Engineer A). h = { s(), save(partial, section), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store } from '../../store.js';
import { toggle, toast, confirmSheet } from '../../ui.js';
import { helpTip } from '../help-tip.js';

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
  return html`
    <div class="field-row">
      <div class="grow">
        <div class="field-label row gap-4">Remember what people mention ${helpTip('memory')}</div>
        <div class="field-help">Personas note lasting things people tell them — a new job, a birthday, a favourite team — and bring them up naturally later. Each chat can turn this off in its <b>Memory</b> tab, where you can also see, pin, edit and delete every memory.</div>
      </div>
      ${toggle(on, (v) => h.save({ memory: { enabled: v } }, 'memory'), { label: 'Remember what people mention' })}
    </div>
    <div class="field-row">
      <div class="grow">
        <div class="field-label">${icon('lock', 'ic-sm')}Privacy</div>
        <div class="field-help">Memories are stored only on this Mac. With Ollama nothing leaves your Mac; a persona using Claude or OpenAI sends the messages it learns from to that service.</div>
      </div>
      <button class="btn btn-ghost btn-sm danger-text" @click=${forgetAll}>${icon('trash')}Forget all memories</button>
    </div>`;
}
