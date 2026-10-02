// settings/soon.js — the "arriving soon" row used by Settings sections whose
// feature hasn't landed yet (wave 3 placeholders).

import { html } from '../../dom.js';
import { icon } from '../../icons.js';

export function soonRow(text) {
  return html`<div class="field-row soon-row">
    <div class="grow"><div class="field-label">${icon('sparkles', 'ic-sm')}Arriving soon</div><div class="field-help">${text}</div></div>
  </div>`;
}
