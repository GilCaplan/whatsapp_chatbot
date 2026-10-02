// settings/memory.js — Settings › Memory & recap.
// Wave 3: Engineer A owns the memory rows, Engineer B the daily recap rows.
// h = { s(), save(partial, section), sectionHead(id, title, sub, color), update() }.

import { html } from '../../dom.js';
import { memoryRows } from './memory-rows.js';
import { recapRows } from './recap.js';

export function memorySection(h) {
  return html`<section class="card section" id="sec-memory" data-section="memory">
    ${h.sectionHead('memory', 'Memory & recap', 'Personas remember what people tell them, and you can get a short recap of each chat at the end of the day.', 'linear-gradient(135deg,#6366f1,#22d3ee)')}
    ${memoryRows(h) /* Engineer A: memory rows (settings/memory-rows.js) */}
    ${recapRows(h) /* Engineer B: daily recap rows (settings/recap.js) */}
  </section>`;
}
