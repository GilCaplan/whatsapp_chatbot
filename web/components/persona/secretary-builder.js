// secretary-builder.js — build (or change) a personal secretary persona: a
// short form (your name, its name, tone, what it does, when people can meet
// you, what it may say and must never do) plus "More details" (questions it
// may answer, VIPs, an away message). It always says it is your assistant.
// The persona's texts are written from the answers; the secretary block
// (persona.secretary) drives its job in the prompt (prompt/secretary.go).

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store } from '../../store.js';
import { openSheet, toast, seg, toggle } from '../../ui.js';
import { navigate } from '../../router.js';
import { rowsFor } from '../../util.js';

const DAYS = [['mon', 'Mon'], ['tue', 'Tue'], ['wed', 'Wed'], ['thu', 'Thu'], ['fri', 'Fri'], ['sat', 'Sat'], ['sun', 'Sun']];
const LENGTHS = [15, 30, 45, 60, 90];
const DUTIES = [
  ['takeMessages', 'Take messages for me', 'Finds out who, what, how urgent and how to get back to them; you see it in Messages for you.'],
  ['flagUrgent', 'Flag urgent things', 'Urgent messages and your VIPs notify you right away.'],
  ['answerFaqs', 'Answer simple questions', 'From the facts you give it below; anything else it checks with you.'],
  ['meetings', 'Help set up meetings', 'Offers times you are free (never confirms them; you do).'],
];

const cleanName = (s) => (s || '').replace(/[^\p{L}\p{N} '.-]/gu, '').trim();

/** The persona texts written from the answers. */
function personaTexts(f) {
  const owner = f.ownerName.trim() || 'the owner';
  const formal = f.tone === 'formal';
  return {
    tagline: `${owner}'s personal assistant`,
    bio: `${owner}'s personal assistant on WhatsApp. Looks after ${owner}'s messages while ${owner} is busy.`,
    personality: formal ? 'Polite, organised and precise. Professional and discreet.' : 'Warm, organised and quick. Helpful without being pushy; keeps things light.',
    style: formal ? 'Clear, complete sentences; courteous; no slang; one question at a time.' : 'Short, natural messages with a friendly tone; one question at a time.',
    rules: `Always says it is ${owner}'s assistant. Never makes commitments on ${owner}'s behalf.`,
    goal: `Help people reach ${owner}: understand what they need and pass it on clearly.`,
  };
}

/**
 * Open the builder. With a persona that already is a secretary, it changes
 * that persona (texts and secretary block); otherwise it creates a new one.
 */
export function openSecretaryBuilder({ persona = null, onSaved = null } = {}) {
  const s0 = store.state;
  const sec = (persona && persona.secretary) || {};
  const f = {
    ownerName: sec.ownerName || cleanName(s0.wa && s0.wa.me && (s0.wa.me.pushName || s0.wa.me.name)) || '',
    name: persona ? persona.name : 'Assistant',
    tone: sec.tone || 'friendly',
    takeMessages: sec.takeMessages ?? true,
    flagUrgent: sec.flagUrgent ?? true,
    answerFaqs: sec.answerFaqs ?? true,
    meetings: sec.meetings ?? true,
    meetDays: sec.meetDays || ['mon', 'tue', 'wed', 'thu', 'fri'],
    meetFrom: sec.meetFrom || '09:00',
    meetTo: sec.meetTo || '18:00',
    meetingMinutes: sec.meetingMinutes || 30,
    useCalendars: sec.useCalendars ?? true,
    shareable: sec.shareable || '',
    never: sec.never || '',
    awayMessage: sec.awayMessage || '',
    faqs: (sec.faqs || []).map((x) => ({ ...x })),
    vips: (sec.vips || []).join(', '),
    more: !!((sec.faqs && sec.faqs.length) || (sec.vips && sec.vips.length) || sec.awayMessage),
  };
  const st = { saving: false, error: '', calendars: null };
  api.calendars.list({ quiet: true }).then((r) => { st.calendars = (r && r.items) || []; sheet.update(); }).catch(() => { st.calendars = []; });

  async function save(ctl) {
    if (!f.ownerName.trim()) { st.error = 'Write your name: it says it is your assistant.'; ctl.update(); return; }
    if (!f.name.trim()) { st.error = 'Give it a name.'; ctl.update(); return; }
    st.saving = true; st.error = ''; ctl.update();
    const secretary = {
      ownerName: f.ownerName.trim(), tone: f.tone,
      takeMessages: f.takeMessages, flagUrgent: f.flagUrgent, answerFaqs: f.answerFaqs, meetings: f.meetings,
      meetDays: f.meetDays, meetFrom: f.meetFrom, meetTo: f.meetTo, meetingMinutes: Number(f.meetingMinutes), useCalendars: f.useCalendars,
      shareable: f.shareable.trim(), never: f.never.trim(), awayMessage: f.awayMessage.trim(),
      faqs: f.faqs.filter((x) => x.q.trim() && x.a.trim()),
      vips: f.vips.split(/[,\n]/).map((x) => x.trim()).filter(Boolean),
    };
    const texts = personaTexts(f);
    try {
      let saved;
      if (persona) {
        saved = await api.personas.update(persona.id, { ...persona, name: f.name.trim(), tagline: texts.tagline, secretary });
        toast('Secretary updated', { type: 'success' });
      } else {
        saved = await api.personas.create({
          name: f.name.trim(), ...texts, messageLength: 'short',
          emoji: { usage: f.tone === 'formal' ? 'none' : 'rare', favorites: [] },
          avatar: { kind: 'generated', glyph: 'book' }, secretary,
        });
        toast(`${saved.name} is ready. Assign it to a chat in Chats.`, { type: 'success' });
      }
      ctl.close();
      if (onSaved && saved) onSaved(saved);
      if (!persona && saved && saved.id) navigate(`/persona/${encodeURIComponent(saved.id)}`);
    } catch (e) {
      st.error = (e && e.message) || "Couldn't save it";
      st.saving = false;
      ctl.update();
    }
  }

  const set = (ctl, k, v) => { f[k] = v; ctl.update(); };
  const calCount = () => (st.calendars || []).filter((c) => c.enabled).length;

  const sheet = openSheet((ctl) => html`
    <div class="sheet-head">
      <div class="sheet-icon">${icon('inbox')}</div>
      <div class="grow">
        <h2>${persona ? `Change ${persona.name}` : 'Personal secretary'}</h2>
        <p>Answers for you while you're busy, takes messages, flags what's urgent and helps people find a time with you. It always says it is your assistant.</p>
      </div>
      <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
    </div>
    ${st.error ? html`<div class="banner danger mb-16">${icon('warning')}<div>${st.error}</div></div>` : ''}
    <div class="sec-form">
      <div class="two-col">
        <div class="field"><label class="field-label" for="sec-owner">Your name</label>
          <input class="input" id="sec-owner" .value=${f.ownerName} placeholder="e.g. Alex" @input=${(e) => { f.ownerName = e.target.value; }}>
          <div class="field-help">It introduces itself as "${(f.ownerName || 'your')}${f.ownerName ? "'s" : ''} assistant".</div></div>
        <div class="field"><label class="field-label" for="sec-name">Its name</label>
          <input class="input" id="sec-name" .value=${f.name} placeholder="e.g. Maya" @input=${(e) => { f.name = e.target.value; }}></div>
      </div>
      <div class="field-row"><div class="grow"><div class="field-label">Tone</div></div>
        ${seg([{ value: 'friendly', label: 'Friendly' }, { value: 'formal', label: 'Formal' }], f.tone, (v) => set(ctl, 'tone', v), { label: 'Tone', cls: 'seg-sm' })}</div>

      <div class="field-label mt-12">What it does</div>
      <div class="sec-duties">
        ${DUTIES.map(([k, label, help]) => html`<label class=${'sec-duty' + (f[k] ? ' on' : '')}>
          <input type="checkbox" .checked=${f[k]} @change=${(e) => set(ctl, k, e.target.checked)}>
          <span class="grow"><span class="sec-duty-t">${label}</span><span class="sec-duty-h">${help}</span></span>
        </label>`)}
      </div>

      ${f.meetings ? html`<div class="sec-block">
        <div class="field-label">When people can meet you</div>
        <div class="sec-days">${DAYS.map(([d, l]) => html`<button type="button" class="chip chip-sm" aria-pressed=${String(f.meetDays.includes(d))}
          @click=${() => set(ctl, 'meetDays', f.meetDays.includes(d) ? f.meetDays.filter((x) => x !== d) : [...f.meetDays, d])}>${l}</button>`)}</div>
        <div class="row gap-8 row-wrap mt-8">
          <label class="small">From <input class="input input-sm sec-time" type="time" .value=${f.meetFrom} @change=${(e) => set(ctl, 'meetFrom', e.target.value)}></label>
          <label class="small">to <input class="input input-sm sec-time" type="time" .value=${f.meetTo} @change=${(e) => set(ctl, 'meetTo', e.target.value)}></label>
          <label class="small">meetings of
            <select class="select select-sm" @change=${(e) => set(ctl, 'meetingMinutes', Number(e.target.value))}>
              ${LENGTHS.map((n) => html`<option value=${n} ?selected=${Number(f.meetingMinutes) === n}>${n} min</option>`)}
            </select></label>
        </div>
        <div class="field-row mt-8">
          <div class="grow"><div class="field-label">${icon('calendar', 'ic-sm')}Check my calendars</div>
            <div class="field-help">${st.calendars == null ? 'Looking…' : calCount()
              ? html`${calCount()} connected, read-only: it only sees when you're busy, never what. <a href="#/settings" @click=${() => ctl.close()}>Manage in Settings › Calendars</a>.`
              : html`None connected yet: it offers any time in the hours above. <a href="#/settings" @click=${() => ctl.close()}>Add one in Settings › Calendars</a> (your MACalendar app, Google, Outlook, iCloud).`}</div></div>
          ${toggle(f.useCalendars, (v) => set(ctl, 'useCalendars', v), { label: 'Check my calendars' })}
        </div>
      </div>` : ''}

      <div class="field"><label class="field-label" for="sec-share">Things it may tell people</label>
        <textarea class="textarea" id="sec-share" rows=${rowsFor(f.shareable, 2, 6)} placeholder="e.g. I'm travelling until Sunday. The office is closed on Fridays."
          .value=${f.shareable} @input=${(e) => { f.shareable = e.target.value; }}></textarea></div>
      <div class="field"><label class="field-label" for="sec-never">Things it must never do or say</label>
        <textarea class="textarea" id="sec-never" rows=${rowsFor(f.never, 2, 6)} placeholder="e.g. give out my address, agree to prices, say where I am"
          .value=${f.never} @input=${(e) => { f.never = e.target.value; }}></textarea></div>

      <button type="button" class="disclosure" aria-expanded=${String(f.more)} @click=${() => set(ctl, 'more', !f.more)}>
        ${icon(f.more ? 'chevron-up' : 'chevron-down', 'ic-sm')}<span>More details</span><span class="faint small">questions it may answer, VIPs, an away message</span></button>
      ${f.more ? html`<div class="sec-block">
        <div class="field-label">Questions it may answer</div>
        ${f.faqs.map((q, i) => html`<div class="sec-faq">
          <input class="input input-sm" placeholder="Question, e.g. Where is the office?" .value=${q.q} @input=${(e) => { q.q = e.target.value; }}>
          <input class="input input-sm" placeholder="Your answer" .value=${q.a} @input=${(e) => { q.a = e.target.value; }}>
          <button type="button" class="btn btn-ghost btn-icon btn-xs" aria-label="Remove this question" @click=${() => set(ctl, 'faqs', f.faqs.filter((_, j) => j !== i))}>${icon('x')}</button>
        </div>`)}
        ${f.faqs.length < 20 ? html`<button type="button" class="btn btn-ghost btn-sm" @click=${() => set(ctl, 'faqs', [...f.faqs, { q: '', a: '' }])}>${icon('plus')}Add a question</button>` : ''}
        <div class="field mt-12"><label class="field-label" for="sec-vips">VIPs</label>
          <input class="input" id="sec-vips" placeholder="Names or numbers, separated by commas" .value=${f.vips} @input=${(e) => { f.vips = e.target.value; }}>
          <div class="field-help">Their messages always count as urgent.</div></div>
        <div class="field"><label class="field-label" for="sec-away">Away message</label>
          <input class="input" id="sec-away" placeholder="e.g. Alex is on holiday until 12 Nov and checks messages in the evening." .value=${f.awayMessage}
            @input=${(e) => { f.awayMessage = e.target.value; }}></div>
      </div>` : ''}
    </div>
    <div class="sheet-actions">
      <button class="btn btn-ghost" @click=${() => ctl.close()}>Cancel</button>
      <button class=${'btn btn-primary ' + (st.saving ? 'loading' : '')} ?disabled=${st.saving} @click=${() => save(ctl)}>${icon('check')}${persona ? 'Save changes' : 'Create secretary'}</button>
    </div>`, { label: 'Personal secretary', size: 'lg' });
  return sheet;
}
