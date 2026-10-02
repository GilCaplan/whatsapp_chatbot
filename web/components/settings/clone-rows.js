// settings/clone-rows.js — Settings › Data: "Clone yourself" consent rows
// (wave 3, Engineer A). Off by default: with consent, Doppel keeps a private
// sample of messages you type yourself (links and numbers removed) so it can
// write a persona that texts like you. h = { s(), save(partial, section), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { toggle, toast, confirmSheet } from '../../ui.js';
import { relTime } from '../../util.js';
import { helpTip } from '../help-tip.js';

const st = { data: null, loading: false, loadedAt: 0 };

async function load(update) {
  if (st.loading) return;
  st.loading = true;
  try { st.data = await api.clone.samples({ quiet: true }); } catch { /* keep the old numbers */ }
  st.loading = false;
  st.loadedAt = Date.now();
  update();
}

export function cloneRows(h) {
  const s = h.s();
  const on = !!(s.clone && s.clone.collectSamples);
  if (!st.loading && Date.now() - st.loadedAt > 15000) load(h.update);
  const d = st.data;
  const count = d ? d.count || 0 : 0;
  async function setOn(v) {
    if (v) {
      const ok = await confirmSheet({
        title: 'Keep a sample of your messages?',
        body: 'From now on Doppel keeps a private copy of messages you type yourself on WhatsApp (links, e-mail addresses and phone numbers removed), up to the last 500. They stay on this computer and are only used when you tap Clone yourself. You can delete them any time.',
        confirm: 'Yes, keep a sample', iconName: 'lock',
      });
      if (!ok) { h.update(); return; }
    }
    if (await h.save({ clone: { collectSamples: v } }, 'data')) load(h.update);
  }
  async function del() {
    const ok = await confirmSheet({ title: 'Delete your message samples?', body: `Deletes the ${count} sample${count === 1 ? '' : 's'} kept for Clone yourself. Personas you already made stay as they are.`, confirm: 'Delete', danger: true, iconName: 'trash' });
    if (!ok) return;
    try { await api.clone.deleteSamples(); toast('Samples deleted', { type: 'success' }); } catch { /* toasted */ }
    load(h.update);
  }
  return html`
    <div class="field-row" data-key="clone-consent">
      <div class="grow">
        <div class="field-label row gap-4">Keep a sample of my messages ${helpTip('clone')}</div>
        <div class="field-help">For <b>Clone yourself</b> on the Personas page: a private sample of messages you type yourself, so Doppel can learn how you text. Stored only on this computer; links and numbers are removed. Off by default.</div>
        ${on || count ? html`<div class="tiny faint mt-4">${count ? `${count} message${count === 1 ? '' : 's'} kept${d && d.since ? ` since ${relTime(d.since)}` : ''}` : 'Nothing kept yet — chat as usual on your phone.'}</div>` : ''}
      </div>
      ${toggle(on, setOn, { label: 'Keep a sample of my messages' })}
    </div>
    ${count ? html`<div class="field-row" data-key="clone-delete">
      <div class="grow"><div class="field-label">${icon('trash', 'ic-sm')}Message samples</div><div class="field-help">Delete everything kept for Clone yourself.</div></div>
      <button class="btn btn-ghost btn-sm danger-text" @click=${del}>${icon('trash')}Delete samples</button>
    </div>` : ''}`;
}
