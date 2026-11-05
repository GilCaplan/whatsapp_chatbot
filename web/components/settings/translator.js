// settings/translator.js — Settings › Translator: the add-on that turns the
// persona's English into the chat's language with a dedicated translation
// model, not the AI brain. h = { s(), save, sectionHead(id, title, sub, color), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { fieldRow, toast } from '../../ui.js';
import { langName } from '../persona/language-controls.js';

const st = { status: null, loading: false, timer: null };

async function refresh(h) {
  try { st.status = await api.translator.status({ quiet: true }); } catch { st.status = st.status || {}; }
  st.loading = false;
  const busy = st.status.installing || (st.status.fetching || []).length;
  clearTimeout(st.timer);
  if (busy) st.timer = setTimeout(() => refresh(h), 3000);
  h.update();
}

export function translatorSection(h) {
  if (!st.status && !st.loading) { st.loading = true; refresh(h); }
  const s = st.status || {};
  const packs = s.packs || [];
  const busy = s.installing || (s.fetching || []).length > 0;
  async function install() {
    try {
      st.status = await api.translator.install(['ru', 'he']);
      toast('Setting up the translator. This can take a few minutes', { type: 'success' });
      refresh(h);
    } catch { /* toasted */ }
  }
  return html`<section class="card section" id="sec-translator" data-section="translator">
    ${h.sectionHead('translator', 'Translator', 'Personas always write in English. Before a reply is sent, this turns it into the chat\'s language with a dedicated translation model: fast, private, and the same text always gives the same result.', 'linear-gradient(135deg,var(--blue),var(--indigo))')}
    ${fieldRow({
      label: s.installed ? 'Installed' : 'Not installed',
      help: s.installing ? (s.step || 'Working') + '…' : s.installed
        ? 'New languages download by themselves the first time a chat needs them. Until then the AI brain translates.'
        : 'Without it the AI brain translates, which is slower and less exact. Needs Python and about 500 MB, plus 100 to 200 MB per language.',
      control: s.installed ? html`<span class="chip chip-sm">Ready</span>`
        : html`<button class=${'btn btn-primary btn-sm ' + (s.installing ? 'loading' : '')} ?disabled=${busy} @click=${install}>${icon('play')}Install</button>`,
    })}
    ${s.installed ? fieldRow({ label: 'Languages ready', help: (s.fetching || []).length ? `Downloading ${s.fetching.length} more…` : '',
      control: html`<span class="chip-row">${packs.length ? packs.map((c) => html`<span class="chip chip-sm" data-key=${c}>${langName(c)}</span>`) : html`<span class="tiny faint">None yet</span>`}</span>` }) : ''}
    ${s.error ? html`<div class="banner danger small">${icon('warning')}<div>${s.error}</div></div>` : ''}
  </section>`;
}
