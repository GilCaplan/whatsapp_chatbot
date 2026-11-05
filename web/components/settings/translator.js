// settings/translator.js — Settings › Translator: the add-on that turns the
// persona's English into the chat's language with a dedicated translation
// model, not the AI brain. h = { s(), save, sectionHead(id, title, sub, color), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { fieldRow, toast } from '../../ui.js';
import { DEFAULT_LANGS, langName } from '../persona/language-controls.js';

const st = { status: null, loading: false, timer: null };

async function refresh(h) {
  try { st.status = await api.translator.status({ quiet: true }); } catch { st.status = st.status || {}; }
  st.loading = false;
  const busy = st.status.installing || (st.status.fetching || []).length;
  clearTimeout(st.timer);
  if (busy) st.timer = setTimeout(() => refresh(h), 3000);
  h.update();
}

// The languages this app's settings already use, so Install needs no questions.
function languagesInUse(h, available) {
  const l = h.s().behavior?.languages || {};
  const codes = [l.private, l.group].flatMap((p) => [...(p?.languages?.length ? p.languages : DEFAULT_LANGS), p?.reply]);
  return [...new Set(codes)].filter((c) => available.some((p) => p.code === c));
}

function packRow(p, s, getPacks) {
  const name = langName(p.code);
  if ((s.packs || []).includes(p.code)) return html`<span class="chip chip-sm" data-key=${p.code}>${icon('check')}${name}</span>`;
  if ((s.fetching || []).includes(p.code)) return html`<span class="chip chip-sm" data-key=${p.code}>${name}…</span>`;
  return html`<button class="btn btn-sm" data-key=${p.code} ?disabled=${s.installing} @click=${() => getPacks([p.code])}>${icon('download')}${name}, ${p.sizeMB} MB</button>`;
}

export function translatorSection(h) {
  if (!st.status && !st.loading) { st.loading = true; refresh(h); }
  const s = st.status || {};
  const available = s.available || [];
  const missing = available.filter((p) => !(s.packs || []).includes(p.code) && !(s.fetching || []).includes(p.code));
  const busy = s.installing || (s.fetching || []).length > 0;
  async function getPacks(languages) {
    try {
      st.status = await api.translator.install(languages);
      toast(s.installed ? 'Downloading. This can take a few minutes' : 'Setting up the translator. This can take a few minutes', { type: 'success' });
      refresh(h);
    } catch { /* toasted */ }
  }
  return html`<section class="card section" id="sec-translator" data-section="translator">
    ${h.sectionHead('translator', 'Translator', 'Personas always write in English. Before a reply is sent, this turns it into the chat\'s language with a dedicated translation model: fast, private, and the same text always gives the same result.', 'linear-gradient(135deg,var(--blue),var(--indigo))')}
    ${fieldRow({
      label: s.installed ? 'Installed' : 'Not installed',
      help: s.installing ? (s.step || 'Working') + '…' : s.installed
        ? 'A language also downloads by itself the first time a chat needs it. Until then the AI brain translates.'
        : 'Without it the AI brain translates, which is slower and less exact. Needs Python 3.10 or newer and about 200 MB, plus 65 to 195 MB per language. Everything stays on this computer.',
      control: s.installed ? html`<span class="chip chip-sm">Ready</span>`
        : html`<button class=${'btn btn-primary btn-sm ' + (s.installing ? 'loading' : '')} ?disabled=${busy} @click=${() => getPacks(languagesInUse(h, available))}>${icon('play')}Install</button>`,
    })}
    ${s.installed ? fieldRow({ label: 'Languages', help: (s.fetching || []).length ? `Downloading ${s.fetching.length}…` : 'Each download is checked before it is used.',
      stack: true, control: html`<span class="chip-row">${available.map((p) => packRow(p, s, getPacks))}</span>` }) : ''}
    ${s.installed && missing.length > 1 ? fieldRow({ label: 'All languages', help: `${missing.length} more, about ${(missing.reduce((n, p) => n + p.sizeMB, 0) / 1000).toFixed(1)} GB.`,
      control: html`<button class="btn btn-sm" ?disabled=${s.installing} @click=${() => getPacks(missing.map((p) => p.code))}>${icon('download')}Download all</button>` }) : ''}
    ${s.error ? html`<div class="banner danger small">${icon('warning')}<div>${s.error}</div></div>` : ''}
  </section>`;
}
