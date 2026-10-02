// drawer/banners.js — status banners shown above every tab of the chat panel
// (Needs you, Away, Paused / Revealed, persona missing, replies waiting).
// Wave 3 owner: Engineer B (hand-off "Needs you" and "Revealed" banners).

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { store, isAway } from '../../store.js';
import { busy } from '../../ui.js';
import { awayUntilText } from './shared.js';
import { catGlyph, catLabel, catReason, waLink, resumeChat } from '../handoff-meta.js';

/** The "Needs you" banner of a paused (hand-off) chat. */
export function needsYouBanner(c, p, { compact = false } = {}) {
  const h = c.handoff;
  if (!h) return '';
  const name = p ? p.name : 'The persona';
  const who = h.sender || c.name || 'Someone';
  const link = waLink(c);
  return html`<div class="needs-banner mb-16" role="status">
    <div class="nb-head">
      <span class="nb-badge">${catGlyph(h.category)}</span>
      <div class="grow" style="min-width:0">
        <div class="nb-title"><span class="nb-eyebrow">Needs you · ${catLabel(h.category)}</span>${catReason(h.category, who)}</div>
        <div class="nb-sub">${name} paused here ${h.at ? html`<rel-time datetime=${h.at}></rel-time>` : ''}${h.how === 'ai' ? ' (the AI double-checked it)' : ''}.</div>
      </div>
    </div>
    ${h.excerpt ? html`<div class="nb-quote"><span class="who">${who}</span>${h.excerpt}</div>` : ''}
    ${compact ? '' : html`<div class="nb-help">Answer from your phone. New messages are kept so ${name} knows what happened; tap Resume when you're done.</div>`}
    <div class="nb-actions">
      ${link ? html`<a class="btn btn-glass btn-sm" href=${link} target="_blank" rel="noopener">${icon('external')}Open WhatsApp</a>` : ''}
      <button class="btn btn-primary btn-sm" @click=${busy(() => resumeChat(c.key, p && p.name))}>${icon('play')}Resume ${name}</button>
    </div>
  </div>`;
}

export function drawerBanners(ctx, c, p) {
  const key = c.key;
  const away = isAway(c);
  const cb = ctx.behaviour.cb();
  const awayTail = (cb && cb.effective && cb.effective.availability && cb.effective.availability.outsideHours === 'silent')
    ? "messages that arrive meanwhile won't get a reply." : 'messages that arrive meanwhile will be answered after that.';
  const pending = store.state.approvals.filter((a) => a.chatKey === key).length;
  const name = p ? p.name : 'The persona';
  const revealed = c.revealedAt && !c.enabled;
  return html`
    ${needsYouBanner(c, p)}
    ${away ? html`<div class="banner away-banner mb-16">${icon('snooze')}<div class="grow"><strong>Away ${awayUntilText(c.snoozedUntil)}</strong> — ${awayTail}</div>
      <button class="btn btn-glass btn-sm" @click=${() => ctx.away.set(null)}>${icon('play')}I'm back</button></div>` : ''}
    ${revealed ? html`<div class="banner revealed-banner mb-16">${icon('eye')}<div class="grow"><strong>Revealed <rel-time datetime=${c.revealedAt}></rel-time>.</strong> ${c.name || 'They'} know${c.kind === 'group' ? '' : 's'} it was ${name}. It's paused here until you switch it back on.</div></div>`
      : !c.enabled ? html`<div class="banner warn mb-16">${icon('pause')}<div><strong>Paused.</strong> ${name} won't reply here until you switch it back on.</div></div>` : ''}
    ${!p ? html`<div class="banner danger mb-16">${icon('warning')}<div>The persona for this chat was deleted. Pick a new one on the Overview tab.</div></div>` : ''}
    ${pending ? html`<a class="banner info mb-16" href="#/approvals" style="text-decoration:none;color:inherit">${icon('inbox')}<div><strong>${pending} repl${pending === 1 ? 'y' : 'ies'} waiting</strong> for your approval →</div></a>` : ''}`;
}
