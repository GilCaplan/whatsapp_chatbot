// drawer/people.js — the chat panel's "People" tab (groups: who the persona
// answers, "always reply" stars, notes) or "Contact" tab (DMs: notes about
// the contact). The section itself is people-section.js; the shell owns the
// instance (ctx.people) because the Overview's "Test it" lists the members.
// Wave 3 owner: Engineer A (learned-memory count chip per person).

import { html } from '../../dom.js';
import { icon } from '../../icons.js';

/** "Leo remembers: Dana 3 · Josh 2" — each opens the Memory tab for that person. */
function memoryStrip(ctx, c, p) {
  const mem = ctx.memories;
  if (!mem || !mem.data || !mem.data.enabled) return '';
  const groups = mem.byPerson().filter((g) => g.items.length);
  if (!groups.length) return '';
  const open = (person) => { ctx.notify('memoryFilter', person); ctx.setTab('memory'); };
  const name = p ? p.name : 'The persona';
  const isGroup = c.kind === 'group';
  return html`<div class="mem-strip" data-key="mem-strip">
    <span class="mem-strip-label small muted">${icon('brain', 'ic-sm')}${name} remembers</span>
    ${groups.map((g) => {
      const who = g.person || (isGroup ? 'Someone' : c.name || 'Them');
      const n = g.items.length;
      return html`<button type="button" class="chip chip-sm mem-chip" title=${`See what ${name} remembers about ${who}`} @click=${() => open(g.person)}>
        ${who}<span class="mem-count">${n} ${n === 1 ? 'thing' : 'things'}</span></button>`;
    })}
  </div>`;
}

export function createPeopleTab(ctx) {
  return {
    id: 'people',
    view: (c, p) => html`${memoryStrip(ctx, c, p)}${ctx.people.view(c)}`,
  };
}
