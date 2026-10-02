// help-tip.js — a small "?" next to a complex control that opens a friendly
// explanation (copy: guide-content.js TIPS) with "Learn more" into the Guide.
//
//   helpTip('mode')                     // known tip id
//   helpTip('mode', { label: 'Modes' }) // custom aria label

import { html, raw, esc } from '../dom.js';
import { icon } from '../icons.js';
import { openPopover } from '../ui.js';
import { navigate } from '../router.js';
import { TIPS, GUIDE_SECTIONS } from '../guide-content.js';

/** Escape text and render **bold** spans (the only markup guide copy uses). */
export function richText(s) {
  return raw(esc(s || '').replace(/\*\*(.+?)\*\*/g, '<b>$1</b>'));
}

function open(anchor, id) {
  const tip = TIPS[id];
  if (!tip) return;
  const sec = GUIDE_SECTIONS.find((x) => x.id === tip.section);
  const pop = openPopover(anchor, (ctl) => html`<div class="help-pop" role="dialog" aria-label=${tip.title}>
    <div class="help-pop-head"><span class="help-pop-ic">${icon('info', 'ic-sm')}</span><b>${tip.title}</b></div>
    <p>${richText(tip.text)}</p>
    ${sec ? html`<button type="button" class="link-btn help-more" @click=${() => { ctl.close(); navigate('/guide/' + sec.id); }}>
      Learn more: ${sec.title}${icon('arrow-right', 'ic-sm')}</button>` : ''}
  </div>`, { width: 300, onClose: () => window.removeEventListener('hashchange', closeOnNav) });
  // Popovers outlive page changes otherwise.
  const closeOnNav = () => pop.close();
  window.addEventListener('hashchange', closeOnNav);
}

export function helpTip(id, { label = '' } = {}) {
  const tip = TIPS[id];
  if (!tip) return '';
  return html`<button type="button" class="help-tip" aria-label=${label || `What is ${tip.title}?`} title=${`What is ${tip.title}?`}
    @click=${(e) => { e.preventDefault(); e.stopPropagation(); open(e.currentTarget, id); }}>?</button>`;
}
