// settings/calendars.js — Settings › Calendars: the owner's calendars that
// personal secretaries read, read-only — a calendar link (.ics: Google,
// Outlook, iCloud…) or the MACalendar app on this Mac. Tasks and secretaries
// never show the calendar to the AI: code picks the times it may offer.
// Links are secret addresses: the server returns them masked.
// h = { sectionHead(id, title, sub, color) }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { seg, toggle, toast, confirmSheet } from '../../ui.js';
import { wordChips } from '../common/word-chips.js';

const KINDS = [{ value: 'ics', label: 'Calendar link' }, { value: 'macalendar', label: 'MACalendar app' }];
const HELP = {
  ics: html`Paste the private iCal link: <b>Google Calendar</b> › Settings › your calendar › "Secret address in iCal format"; <b>Outlook</b> › Settings › Calendar › Shared calendars › Publish › ICS link; <b>iCloud</b> › share the calendar as a public calendar.`,
  macalendar: html`Your MACalendar app's address on this Mac (usually <code>http://127.0.0.1:8080</code>) and its API key if you set one in its config.yaml.`,
};

const st = { items: null, error: '', adding: false, form: { kind: 'ics', name: '', url: '', apiKey: '' }, checks: {}, busy: '' };

async function load(update) {
  try { st.items = ((await api.calendars.list({ quiet: true })) || {}).items || []; st.error = ''; } catch (e) { st.error = (e && e.message) || "Couldn't load"; st.items = st.items || []; }
  update();
}

async function check(c, update) {
  st.busy = c.id; update();
  try { st.checks[c.id] = await api.calendars.check(c.id); } catch { /* toasted */ }
  st.busy = ''; update();
}

export function calendarsSection(h) {
  const update = h.update;
  if (st.items == null && !st.loading) { st.loading = true; load(update); }
  const f = st.form;
  const add = async () => {
    try {
      const r = await api.calendars.add({ kind: f.kind, name: f.name, url: f.url || (f.kind === 'macalendar' ? 'http://127.0.0.1:8080' : ''), apiKey: f.apiKey });
      st.items = r.items; st.adding = false; st.form = { kind: 'ics', name: '', url: '', apiKey: '' };
      const added = st.items[st.items.length - 1];
      toast('Calendar added', { type: 'success' });
      if (added) check(added, update);
    } catch { /* toasted */ }
    update();
  };
  const patchCal = async (c, body) => { try { st.items = (await api.calendars.patch(c.id, body)).items; } catch { /* toasted */ } update(); };
  const remove = async (c) => {
    if (!await confirmSheet({ title: `Remove ${c.name}?`, body: 'Secretaries stop reading it. Nothing changes in the calendar itself.', confirm: 'Remove', danger: true, iconName: 'trash' })) return;
    try { st.items = (await api.calendars.remove(c.id)).items; } catch { /* toasted */ }
    update();
  };
  const fmt = (x) => {
    const d = new Date(x.start);
    const day = d.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
    return x.allDay ? `${day} · all day` : `${day} · ${d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}`;
  };
  return html`<section class="card section" id="sec-calendars" data-section="calendars">
    ${h.sectionHead('calendars', 'Calendars', 'Calendars that tasks and secretaries use to offer times you are free. The AI never sees them: code picks the times it may offer.', 'linear-gradient(135deg,var(--indigo),var(--blue))')}
    ${st.error ? html`<div class="banner danger small">${icon('warning')}<div>${st.error}</div></div>` : ''}
    ${(st.items || []).map((c) => {
      const ck = st.checks[c.id];
      return html`<div class="cal-row" data-key=${c.id}>
        <div class="row gap-8">
          <span class="cal-ic">${icon('calendar')}</span>
          <div class="grow"><div class="field-label">${c.name}</div>
            <div class="tiny faint">${c.kind === 'macalendar' ? 'MACalendar app' : 'Calendar link'} · ${c.where}${c.hasKey ? ' · with key' : ''}</div></div>
          <button class=${'btn btn-ghost btn-sm ' + (st.busy === c.id ? 'loading' : '')} ?disabled=${st.busy === c.id} @click=${() => check(c, update)}>${icon('refresh')}Check</button>
          <button class="btn btn-ghost btn-icon btn-sm" aria-label=${'Remove ' + c.name} @click=${() => remove(c)}>${icon('trash')}</button>
          ${toggle(c.enabled, (v) => patchCal(c, { enabled: v }), { label: 'Use ' + c.name, small: true })}
        </div>
        ${ck ? html`<div class=${'cal-check small ' + (ck.ok ? '' : 'bad')}>
          ${ck.ok ? html`${icon('check', 'ic-sm')}Read fine: ${ck.events} event${ck.events === 1 ? '' : 's'} in the next three weeks.
            ${ck.upcoming && ck.upcoming.length ? html`<ul class="cal-up">${ck.upcoming.map((x) => html`<li><span class="faint">${fmt(x)}</span> ${x.title}</li>`)}</ul>
              <div class="tiny faint">Only you see the titles. The AI never sees this calendar.</div>` : ''}`
            : html`${icon('warning', 'ic-sm')}${ck.error || "Couldn't read it."}`}
        </div>` : ''}
        <div class="cal-skip">
          <div class="tiny faint">Entries with these words don't count as busy (reminders, birthdays). Entries without an end time never do.</div>
          ${wordChips({ words: c.skipWords || [], onChange: (w) => patchCal(c, { skipWords: w }), label: 'Words that don\'t count as busy', placeholder: 'e.g. reminder', max: 20, maxLen: 40 })}
        </div>
      </div>`;
    })}
    ${st.adding ? html`<div class="cal-add">
      ${seg(KINDS, f.kind, (v) => { f.kind = v; update(); }, { label: 'Kind of calendar', cls: 'seg-sm' })}
      <div class="field-help mt-8">${HELP[f.kind]}</div>
      <div class="two-col mt-8">
        <div class="field"><label class="field-label" for="cal-name">Name</label>
          <input class="input input-sm" id="cal-name" placeholder=${f.kind === 'ics' ? 'e.g. Work (Outlook)' : 'MACalendar'} .value=${f.name} @input=${(e) => { f.name = e.target.value; }}></div>
        <div class="field"><label class="field-label" for="cal-url">${f.kind === 'ics' ? 'Calendar link' : 'Address'}</label>
          <input class="input input-sm" id="cal-url" placeholder=${f.kind === 'ics' ? 'https://calendar.google.com/calendar/ical/…/basic.ics' : 'http://127.0.0.1:8080'}
            .value=${f.url} @input=${(e) => { f.url = e.target.value; }}></div>
      </div>
      ${f.kind === 'macalendar' ? html`<div class="field"><label class="field-label" for="cal-key">API key (if it has one)</label>
        <input class="input input-sm" id="cal-key" type="password" autocomplete="off" .value=${f.apiKey} @input=${(e) => { f.apiKey = e.target.value; }}></div>` : ''}
      <div class="row gap-8 mt-8"><span class="grow"></span>
        <button class="btn btn-ghost btn-sm" @click=${() => { st.adding = false; update(); }}>Cancel</button>
        <button class="btn btn-primary btn-sm" @click=${add}>${icon('check')}Add and check</button></div>
    </div>` : html`<button class="btn btn-glass btn-sm mt-8" @click=${() => { st.adding = true; update(); }}>${icon('plus')}Add a calendar</button>`}
  </section>`;
}
