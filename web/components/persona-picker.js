// persona-picker.js — persona dropdown (with avatars) and selectable tiles.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { store, personaById } from '../store.js';
import { openPopover } from '../ui.js';
import { personaAvatar } from './avatar.js';

/** Dropdown-style button that opens a persona list popover. */
export function personaPicker(value, onChange, { placeholder = 'Choose a persona', allowNone = false, noneLabel = 'No persona' } = {}) {
  const p = personaById(value);
  const openList = (e) => {
    const anchor = e.currentTarget;
    openPopover(anchor, (ctl) => {
      const list = store.state.personas;
      return html`
        ${allowNone ? html`<button class="menu-item" role="menuitemradio" aria-checked=${String(!value)}
            @click=${() => { ctl.close(); onChange(''); }}>
            <span class="pav pav-empty" style="--s:30px">–</span><span class="grow">${noneLabel}</span>
            ${!value ? icon('check', 'check') : ''}</button><div class="menu-sep"></div>` : ''}
        ${list.length ? list.map((x) => html`<button class="menu-item" role="menuitemradio" aria-checked=${String(x.id === value)}
            @click=${() => { ctl.close(); if (x.id !== value) onChange(x.id); }}>
            ${personaAvatar(x, 30)}
            <span class="grow"><span class="ellipsis" style="display:block">${x.name}</span><span class="sub ellipsis">${x.tagline || ''}</span></span>
            ${x.id === value ? icon('check', 'check') : ''}
          </button>`) : html`<div class="menu-label">No personas yet</div>`}
        <div class="menu-sep"></div>
        <a class="menu-item" href="#/persona/new" @click=${() => ctl.close()}>${icon('plus')}<span class="grow">New persona…</span></a>`;
    }, { matchWidth: anchor.offsetWidth > 240 });
  };
  return html`<button type="button" class="picker-btn" aria-haspopup="menu" @click=${openList}>
    ${p ? personaAvatar(p, 32) : html`<span class="pav pav-empty" style="--s:32px">${icon('user')}</span>`}
    <span class="grow" style="min-width:0">
      <span class="name ellipsis" style="display:block">${p ? p.name : (value ? 'Unknown persona' : placeholder)}</span>
      ${p && p.tagline ? html`<span class="tag ellipsis" style="display:block">${p.tagline}</span>` : ''}
    </span>
    ${icon('chevron-down', 'ic-chev')}
  </button>`;
}

/** Grid of selectable persona tiles. */
export function personaTiles(value, onPick, { onCreateAI, limit } = {}) {
  const list = limit ? store.state.personas.slice(0, limit) : store.state.personas;
  return html`<div class="tile-grid">
    ${list.map((p) => html`<button type="button" class="tile" data-key=${p.id} aria-pressed=${String(p.id === value)} @click=${() => onPick(p.id)}>
      ${p.id === value ? html`<span class="t-check">${icon('check')}</span>` : ''}
      ${personaAvatar(p, 56)}
      <span class="t-name">${p.name}</span>
      <span class="t-tag clamp-2">${p.tagline || ''}</span>
    </button>`)}
    ${onCreateAI ? html`<button type="button" class="tile dashed" @click=${onCreateAI}>
      <span class="pav" style="--s:56px;background:var(--brand-grad)">${icon('sparkles', 'ic-lg')}</span>
      <span class="t-name">Create with AI</span>
      <span class="t-tag">Describe someone in one line</span>
    </button>` : ''}
  </div>`;
}
