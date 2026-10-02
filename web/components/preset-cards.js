// preset-cards.js — pick a Behaviour preset (Settings) or a compact chip row
// (chat drawer). Icons only — never emoji.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { PRESET_ORDER, presetStyle } from './behaviour-meta.js';

function ordered(presets) {
  const list = Array.isArray(presets) && presets.length
    ? presets.slice()
    : PRESET_ORDER.map((id) => ({ id }));
  list.sort((a, b) => {
    const ia = PRESET_ORDER.indexOf(a.id); const ib = PRESET_ORDER.indexOf(b.id);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
  });
  return list.filter((p) => p.id !== 'custom');
}

const labelOf = (p) => p.label || presetStyle(p.id).label;
const descOf = (p) => p.description || presetStyle(p.id).desc;

/**
 * Large selectable cards. activeId may be 'custom' (then the Custom card is lit).
 * onPick(id) is called for named presets only.
 */
export function presetCards(presets, activeId, onPick, { disabled = false } = {}) {
  const list = ordered(presets);
  const custom = presetStyle('custom');
  const isCustom = !list.some((p) => p.id === activeId);
  return html`<div class="preset-grid" role="radiogroup" aria-label="Behaviour preset">
    ${list.map((p) => {
      const st = presetStyle(p.id);
      const on = p.id === activeId;
      return html`<button type="button" class="tile preset-card" role="radio" aria-checked=${String(on)} aria-pressed=${String(on)}
          data-key=${'pc-' + p.id} ?disabled=${disabled} style=${`--c1:${st.c1};--c2:${st.c2}`}
          @click=${() => { if (!on && onPick) onPick(p.id); }}>
        ${on ? html`<span class="t-check">${icon('check')}</span>` : ''}
        <span class="pc-icon">${icon(st.icon)}</span>
        <span class="t-name">${labelOf(p)}</span>
        <span class="t-tag">${descOf(p)}</span>
      </button>`;
    })}
    <div class=${'tile preset-card is-custom ' + (isCustom ? 'on' : '')} data-key="pc-custom" aria-current=${String(isCustom)}
        style=${`--c1:${custom.c1};--c2:${custom.c2}`}
        title=${isCustom ? 'You changed some settings below' : 'Change any setting below to make your own mix'}>
      ${isCustom ? html`<span class="t-check">${icon('check')}</span>` : ''}
      <span class="pc-icon">${icon(custom.icon)}</span>
      <span class="t-name">${custom.label}</span>
      <span class="t-tag">${isCustom ? 'Your own mix of the settings below.' : 'Change any setting below to make your own.'}</span>
    </div>
  </div>`;
}

/**
 * Compact chips for the chat drawer. First chip = "Default" (inherit).
 * activeId: '' for default, a preset id, or 'custom'.
 */
export function presetChips(presets, activeId, onPick, { defaultLabel = 'Default' } = {}) {
  const list = ordered(presets);
  return html`<div class="chip-row preset-chips" role="radiogroup" aria-label="Behaviour preset for this chat">
    <button type="button" class="chip" role="radio" aria-checked=${String(!activeId)} aria-pressed=${String(!activeId)}
      @click=${() => { if (activeId) onPick(''); }}>${icon('reset')}${defaultLabel}</button>
    ${list.map((p) => {
      const st = presetStyle(p.id);
      const on = p.id === activeId;
      return html`<button type="button" class="chip preset-chip" role="radio" aria-checked=${String(on)} aria-pressed=${String(on)}
          style=${`--c1:${st.c1};--c2:${st.c2}`} title=${descOf(p)} @click=${() => { if (!on) onPick(p.id); }}>
        <span class="pchip-ic">${icon(st.icon)}</span>${labelOf(p)}
      </button>`;
    })}
    ${activeId === 'custom' ? html`<span class="chip on">${icon('sliders')}Custom</span>` : ''}
  </div>`;
}
