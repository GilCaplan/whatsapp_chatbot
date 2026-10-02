// drawer/memory.js — the chat panel's "Memory" tab.
// Wave 3: Engineer A owns the top half (what the persona learned about the
// people here: toggle, list, "Remember this…", "Forget everything here");
// Engineer B appends the daily recap block (recapBlock() from
// web/components/recap-card.js) at the marked spot below.
//
// The memory store is shared with the People tab (ctx.memories: counts per
// person); the People tab opens this tab filtered by person via the
// `memoryFilter` notify hook.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store } from '../../store.js';
import { toggle, toast } from '../../ui.js';
import { recapBlock } from '../recap-card.js';
import { helpTip } from '../help-tip.js';
import { createMemoryStore, memoryList } from '../memory-list.js';
import { createCrossStore, crossBlock } from '../cross-block.js';
import { patchChat } from './shared.js';

export function createMemoryTab(ctx) {
  const mem = createMemoryStore(ctx.key, ctx.update);
  ctx.memories = mem;
  const cross = createCrossStore(ctx.key, ctx.update);
  const ui = { filter: '', editing: '', editText: '', newText: '', newPerson: '', adding: false, extracting: false };

  async function setMemory(c, on) {
    const app = !!(store.state.settings && store.state.settings.memory && store.state.settings.memory.enabled);
    // Back to the app setting when it matches, so Settings keeps control.
    const value = on === app ? null : on;
    try {
      await patchChat(c.key, { memory: value });
      toast(on ? 'Memory on for this chat' : 'Memory off for this chat — nothing new is learned', { type: 'success' });
    } catch { /* toasted */ }
    mem.reloadSoon();
  }

  async function extractNow() {
    ui.extracting = true; ctx.update();
    try {
      const r = await api.chats.extractMemories(ctx.key);
      const n = (r.added || 0) + (r.updated || 0);
      toast(n ? `Learned ${n} new thing${n === 1 ? '' : 's'}` : 'Nothing new to remember yet', { type: n ? 'success' : 'info' });
    } catch { /* toasted */ }
    ui.extracting = false;
    mem.reloadSoon();
  }

  function view(c, p) {
    const name = p ? p.name : 'The persona';
    const who = c.kind === 'group' ? 'people here' : (c.name || 'this contact');
    const enabled = mem.data ? !!mem.data.enabled : (c.memory != null ? !!c.memory : true);
    const own = c.memory != null;
    const pending = mem.data ? mem.data.pending || 0 : 0;
    return html`
      <section class="drawer-section" data-key="memory">
        <h4>${icon('brain', 'ic-sm')}What ${name} remembers<span class="grow"></span>
          ${enabled ? html`<button class=${'btn btn-ghost btn-sm ' + (ui.extracting ? 'loading' : '')} ?disabled=${ui.extracting}
            title="Read the latest messages now instead of waiting" @click=${extractNow}>${icon('refresh')}Update now</button>` : ''}
        </h4>
        <div class="field-row mem-toggle">
          <div class="grow">
            <div class="field-label row gap-4">Learn about ${who} ${helpTip('memory')}
              ${own ? html`<span class="inherit-chip custom" title="Set for this chat only">This chat</span>` : html`<span class="inherit-chip" title="Follows Settings › Memory & recap">Default</span>`}</div>
            <div class="field-help">${enabled
              ? `${name} picks up the little things people mention — plans, favourite things, big days — and brings them up when it fits. Stored only on this computer.`
              : `Off: ${name} doesn't learn anything new here and doesn't use what it remembers.`}</div>
          </div>
          ${toggle(enabled, (v) => setMemory(c, v), { label: 'Learn about people in this chat' })}
        </div>
        ${enabled && pending ? html`<div class="tiny faint mem-pending">${pending} new message${pending === 1 ? '' : 's'} since the last look — it checks every few messages.</div>` : ''}
        ${enabled ? crossBlock({ cross, chat: c, persona: p, onUpdate: ctx.update }) : ''}
        ${memoryList({ mem, chat: c, persona: p, ui, onUpdate: ctx.update })}
      </section>
      ${recapBlock(ctx, c, p) /* Engineer B: daily recap block (recap-card.js) */}`;
  }

  return {
    id: 'memory',
    view,
    load: () => { mem.load(); cross.load(); },
    onBus(type, data) {
      if (type === 'memories.changed' && (!data || !data.chatKey || data.chatKey === ctx.key)) mem.reloadSoon();
      if (type === 'memories.changed') cross.reloadSoon(); // other chats' notes change what this one draws on
      if (type === 'chats.changed' || type === 'settings.changed') { mem.reloadSoon(); cross.reloadSoon(); }
    },
    /** People tab → "3 things remembered" opens this tab filtered by person. */
    memoryFilter(person) { ui.filter = person || ''; ctx.update(); },
    destroy() { mem.destroy(); cross.destroy(); },
  };
}
