// settings/tasks.js — Settings › Tasks: the defaults every task starts
// from (settings.tasks): about you, how it shows your time, "why not
// then?" lines (with "Help me write these"), how messages sound,
// confirming and booking, limits. h = { s(), save(partial, section),
// sectionHead(), update() }.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { seg, fieldRow, toast } from '../../ui.js';
import { helpTip } from '../common/help-tip.js';
import { STRATEGIES, PROBE_THEN, BOOK } from '../tasks/task-labels.js';

const st = { describe: '', drafts: [], drafting: false };

export function tasksSection(h) {
  const t = h.s().tasks || {};
  const p = t.privacy || {};
  const l = t.limits || {};
  const c = t.commit || {};
  const tone = t.tone || {};
  const owner = t.owner || {};
  const save = (part) => h.save({ tasks: part }, 'tasks');
  const lines = p.coverLines || [];
  const setLines = (next) => save({ privacy: { coverLines: next } });

  const num = (label, value, lo, hi, set, unit = '') => fieldRow({
    label,
    control: html`<span class="row gap-4"><input class="input input-sm tk-num" type="number" min=${lo} max=${hi} .value=${String(value ?? '')} aria-label=${label}
      @change=${(e) => set(Math.max(lo, Math.min(hi, parseInt(e.target.value, 10) || lo)))}>${unit ? html`<span class="tiny faint">${unit}</span>` : ''}</span>`,
  });

  async function draft() {
    if (!st.describe.trim() || st.drafting) return;
    st.drafting = true; h.update();
    try { st.drafts = ((await api.tasks.coverLines(st.describe)) || {}).lines || []; } catch { /* toasted */ }
    st.drafting = false; h.update();
  }

  return html`<section class="card section" id="sec-tasks" data-section="tasks">
    ${h.sectionHead('tasks', 'Tasks', 'The defaults for every task, like finding a meeting time. Each task can change them under More details.', 'linear-gradient(135deg,var(--teal),var(--blue))')}

    <div class="eyebrow">About you</div>
    <div class="two-col">
      <label class="field"><span class="field-label">Your name</span>
        <input class="input input-sm" maxlength="60" placeholder="e.g. Alex" .value=${owner.name || ''} @change=${(e) => save({ owner: { name: e.target.value.trim() } })}></label>
      <label class="field"><span class="field-label">About you <span class="faint">(optional)</span></span>
        <input class="input input-sm" maxlength="300" placeholder="e.g. product designer, based in Tel Aviv" .value=${owner.about || ''} @change=${(e) => save({ owner: { about: e.target.value.trim() } })}>
        <span class="field-help">Only used when a task writes as your assistant, and only if it helps.</span></label>
    </div>

    <div class="eyebrow mt-12">How it shows your time ${helpTip('task-strategy')}</div>
    ${fieldRow({ label: 'Strategy', help: (STRATEGIES.find((x) => x.value === p.strategy) || {}).help || '',
      control: seg(STRATEGIES.map(({ value, label }) => ({ value, label })), p.strategy, (v) => save({ privacy: { strategy: v } }), { label: 'Strategy', cls: 'seg-sm' }) })}
    ${num('Times per offer', p.offers, 1, 3, (v) => save({ privacy: { offers: v } }))}
    ${num('"Doesn\'t work" answers before it stops', p.probeLimit, 1, 10, (v) => save({ privacy: { probeLimit: v } }))}
    ${fieldRow({ label: 'Then', control: seg(PROBE_THEN, p.onProbeLimit, (v) => save({ privacy: { onProbeLimit: v } }), { label: 'At the probe limit', cls: 'seg-sm' }) })}
    ${num('Free time kept around busy times', p.bufferMin, 0, 60, (v) => save({ privacy: { bufferMin: v } }), 'min')}

    <div class="eyebrow mt-12">When they ask "why not then?" ${helpTip('task-cover')}</div>
    ${fieldRow({ label: 'Answer', help: p.cover === 'lines' ? 'One of your lines below, picked at random. Nothing else is ever said about it.' : 'Lightly says something else is on then, no details.',
      control: seg([{ value: 'deflect', label: 'Brush it off' }, { value: 'lines', label: 'Use my lines' }], p.cover, (v) => save({ privacy: { cover: v } }), { label: 'Why not', cls: 'seg-sm' }) })}
    ${p.cover === 'lines' ? html`<div class="sub-rows">
      <ul class="tk-lines">${lines.map((x, i) => html`<li data-key=${'cl-' + i}><span class="grow">${x}</span>
        <button class="btn btn-ghost btn-icon btn-xs" aria-label="Remove line" @click=${() => setLines(lines.filter((_, j) => j !== i))}>${icon('x', 'ic-sm')}</button></li>`)}</ul>
      ${lines.length < 10 ? html`<input class="input input-sm" maxlength="200" placeholder="Type a line and press Enter"
        @keydown=${(e) => { if (e.key === 'Enter' && e.target.value.trim()) { setLines([...lines, e.target.value.trim()]); e.target.value = ''; } }}>` : ''}
      <div class="tk-draft mt-8">
        <div class="field-label small">Help me write these</div>
        <div class="row gap-8"><input class="input input-sm grow" maxlength="400" placeholder="e.g. I travel a lot this month, keep it casual" .value=${st.describe}
          @input=${(e) => { st.describe = e.target.value; }} @keydown=${(e) => { if (e.key === 'Enter') draft(); }}>
          <button class=${'btn btn-glass btn-sm ' + (st.drafting ? 'loading' : '')} ?disabled=${st.drafting} @click=${draft}>${icon('wand')}Write</button></div>
        ${st.drafts.length ? html`<ul class="tk-lines mt-8">${st.drafts.map((x) => html`<li><span class="grow">${x}</span>
          <button class="btn btn-ghost btn-xs" ?disabled=${lines.includes(x) || lines.length >= 10}
            @click=${() => { setLines([...lines, x]); toast('Line added', { type: 'success' }); }}>${icon('plus')}${lines.includes(x) ? 'Added' : 'Add'}</button></li>`)}</ul>` : ''}
      </div>
    </div>` : ''}

    <div class="eyebrow mt-12">How task messages sound</div>
    ${fieldRow({ label: 'Formality', control: seg([{ value: 'casual', label: 'Casual' }, { value: 'neutral', label: 'Neutral' }, { value: 'formal', label: 'Formal' }], tone.formality, (v) => save({ tone: { formality: v } }), { label: 'Formality', cls: 'seg-sm' }) })}
    ${fieldRow({ label: 'Directness', control: seg([{ value: 'soft', label: 'Gentle' }, { value: 'balanced', label: 'Balanced' }, { value: 'direct', label: 'Direct' }], tone.directness, (v) => save({ tone: { directness: v } }), { label: 'Directness', cls: 'seg-sm' }) })}
    ${fieldRow({ label: 'Warmth', control: seg([{ value: 'reserved', label: 'Brief' }, { value: 'friendly', label: 'Friendly' }, { value: 'warm', label: 'Warm' }], tone.warmth, (v) => save({ tone: { warmth: v } }), { label: 'Warmth', cls: 'seg-sm' }) })}
    ${fieldRow({ label: 'Dates', help: 'How days are written in messages.', control: seg([{ value: 'dmy', label: 'Tue 10 Nov' }, { value: 'mdy', label: 'Tue Nov 10' }], tone.dateStyle, (v) => save({ tone: { dateStyle: v } }), { label: 'Date style', cls: 'seg-sm' }) })}
    ${fieldRow({ label: 'Clock', control: seg([{ value: '24h', label: '15:00' }, { value: '12h', label: '3 PM' }], tone.clock, (v) => save({ tone: { clock: v } }), { label: 'Clock', cls: 'seg-sm' }) })}

    <div class="eyebrow mt-12">Confirming and booking ${helpTip('task-confirm')}</div>
    ${fieldRow({ label: 'Confirming', help: c.mode === 'auto-if-free' ? 'Confirms by itself when the time is still free in your calendar, and tells you.' : 'Every confirmation waits for your OK on the Tasks page (with a notification).',
      control: seg([{ value: 'approve', label: 'Ask me first' }, { value: 'auto-if-free', label: 'By itself if free' }], c.mode, (v) => save({ commit: { mode: v } }), { label: 'Confirming', cls: 'seg-sm' }) })}
    ${fieldRow({ label: 'Booking', help: c.book === 'macalendar' ? 'Adds it to MACalendar through its own API (Settings › Calendars).' : c.book === 'ics' ? 'A calendar file you open to add it to any calendar app.' : 'Nothing is booked; you see the agreed time on the Tasks page.',
      control: seg(BOOK, c.book, (v) => save({ commit: { book: v } }), { label: 'Booking', cls: 'seg-sm' }) })}

    <div class="eyebrow mt-12">Limits</div>
    ${num('Messages a task may send', l.maxMessages, 4, 100, (v) => save({ limits: { maxMessages: v } }))}
    ${num('Days before it hands over', l.maxDays, 1, 30, (v) => save({ limits: { maxDays: v } }))}
    ${num('Nudges when they go quiet', l.maxNudges, 0, 5, (v) => save({ limits: { maxNudges: v } }))}
    ${num('Nudge after', l.nudgeAfterHours, 1, 168, (v) => save({ limits: { nudgeAfterHours: v } }), 'hours')}
  </section>`;
}
