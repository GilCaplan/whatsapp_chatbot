// tasks/task-sheet.js — set up a task: pick a private chat (one with a
// persona, or any contact: it is assigned on Start) → say what the
// meeting is and when it may happen → (More details) privacy, "why not?",
// confirming, booking and limits → (Advanced) the rule list. Then Start,
// Save for later, or Try it first: an AI plays the other side, nothing is
// sent or booked.
//
//   openTaskSheet({ chatKey?, spec?, save?, onDone?(run|spec) })
//   save = true: edit a saved task (no chat, no Start)

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store, personaById, assignmentFor, loadChats, loadPersonas } from '../../store.js';
import { openSheet, toast, seg, toggle } from '../../ui.js';
import { avatarStack, chatAvatar } from '../common/avatar.js';
import { createChatPicker, chatSubtitle } from '../chat/chat-picker.js';
import { assignChat } from '../chat/assign-sheet.js';
import { helpTip } from '../common/help-tip.js';
import { ruleEditor } from './rule-editor.js';
import { STRATEGIES, PROBE_THEN, BOOK, when } from './task-labels.js';

const DAYS = [['mon', 'Mon'], ['tue', 'Tue'], ['wed', 'Wed'], ['thu', 'Thu'], ['fri', 'Fri'], ['sat', 'Sat'], ['sun', 'Sun']];
const LENGTHS = [15, 30, 45, 60, 90, 120, 180];
const AHEAD = [3, 7, 14, 30];
const PREFER = [{ value: '', label: 'Any' }, { value: 'morning', label: 'Morning' }, { value: 'afternoon', label: 'Afternoon' }, { value: 'evening', label: 'Evening' }];
const STYLES = [
  { value: 'cooperative', label: 'Easy-going' }, { value: 'vague', label: 'Vague' },
  { value: 'difficult', label: 'Busy' }, { value: 'prober', label: 'Nosy' },
];

let catalog = null;
export async function loadTaskCatalog(force = false) {
  if (catalog && !force) return catalog;
  catalog = await api.tasks.list({ quiet: true });
  return catalog;
}

const clone = (x) => JSON.parse(JSON.stringify(x));
const NEW_ASSISTANT = '__assistant'; // the Assistant seed, added on Start

export function openTaskSheet({ chatKey = '', spec = null, save = false, onDone } = {}) {
  const defaults = () => (store.state.settings && store.state.settings.tasks) || {};
  const st = {
    step: save || chatKey ? 'setup' : 'chat',
    chatKey, saveOnly: save,
    spec: spec ? clone(spec) : null,
    more: !!(spec && (spec.privacy || spec.limits || spec.commit)),
    advanced: !!(spec && spec.rules && spec.rules.length),
    open: true,
    busy: '', error: '',
    try: null, tryStyle: 'cooperative', tryPersona: '',
    cals: [],
    // A contact without a persona yet: assigned on Start.
    picked: null, pickPersona: '', pickApproval: true,
  };
  let picker = null;
  let ctl = null;
  const update = () => ctl && ctl.update();

  loadTaskCatalog().then((c) => {
    if (!st.spec) st.spec = clone(((c.templates || [])[0] || {}).spec || {});
    if (!st.spec.rules || !st.spec.rules.length) st.spec.rules = clone(c.defaultRules || []);
    update();
  }).catch((e) => { st.error = (e && e.message) || "Couldn't load tasks"; update(); });

  api.calendars.list({ quiet: true }).then((r) => { st.cals = (r && r.items) || []; update(); }).catch(() => {});

  const assigned = () => (store.state.chats || []).find((c) => c.key === st.chatKey);
  // The chat the task runs in; a picked contact stands in until Start assigns it.
  const chat = () => assigned() || (st.picked && {
    key: '', jid: st.picked.jid, name: st.picked.isSelf ? 'You (message yourself)' : (st.picked.name || st.picked.phone || st.picked.jid),
    kind: st.picked.kind, personaId: st.pickPersona === NEW_ASSISTANT ? '' : st.pickPersona, enabled: true,
    mode: 'auto', // the "Approve before sending" switch above says it
  });
  const assistantId = () => ((store.state.personas || []).find((p) => p.id === 'assistant' || p.name === 'Assistant') || {}).id;
  const m = () => st.spec.meeting;
  const block = (k, def) => { if (!st.spec[k]) st.spec[k] = clone(def); return st.spec[k]; };
  const privacy = () => block('privacy', defaults().privacy || {});
  const limits = () => block('limits', defaults().limits || {});
  const commit = () => block('commit', defaults().commit || {});

  // What goes to the server: unopened blocks stay null (the app defaults).
  function payload() {
    const s = clone(st.spec);
    if (!st.more) { s.privacy = null; s.limits = null; s.commit = null; }
    if (!st.advanced) s.rules = [];
    return s;
  }

  async function start() {
    if (st.busy) return;
    st.busy = 'start'; update();
    try {
      let key = st.chatKey;
      if (!assigned() && st.picked) {
        let personaId = st.pickPersona;
        if (personaId === NEW_ASSISTANT) {
          personaId = (await api.personas.create(catalog.assistant)).id;
          st.pickPersona = personaId; // a retry reuses it
          await loadPersonas();
        }
        key = (await assignChat(st.picked, personaId, st.pickApproval)).key;
        st.chatKey = key; st.picked = null;
      } else if (assigned() && !assigned().enabled) {
        await api.chats.patch(key, { enabled: true });
        await loadChats();
      }
      const run = await api.tasks.start(key, { spec: payload(), open: st.open });
      const c = chat();
      toast(st.open ? `Task started: ${(personaById(c && c.personaId) || {}).name || 'The persona'} is writing first` : 'Task started: it answers their next message', { type: 'success' });
      if (onDone) onDone(run);
      ctl.close();
    } catch { /* toasted */ } finally { st.busy = ''; update(); }
  }

  async function saveForLater() {
    if (st.busy) return;
    st.busy = 'save'; update();
    try {
      const saved = await api.tasks.save(payload());
      st.spec.id = saved.id;
      toast('Task saved', { type: 'success' });
      if (onDone) onDone(saved);
      if (st.saveOnly) ctl.close();
    } catch { /* toasted */ } finally { st.busy = ''; update(); }
  }

  async function tryIt() {
    if (st.busy) return;
    const c = chat();
    const personaId = st.tryPersona || (c && c.personaId) || (store.state.personas.find((p) => p.id === 'assistant') || store.state.personas[0] || {}).id;
    st.busy = 'try'; st.try = { loading: true }; update();
    try {
      st.try = await api.tasks.try({ personaId, spec: payload(), style: st.tryStyle, turns: 6 });
    } catch (e) { st.try = { error: (e && e.message) || 'The try failed' }; } finally { st.busy = ''; update(); }
  }

  // ── Steps ──
  function pickChat(it) {
    const a = assignmentFor(it);
    if (a && a.kind !== 'group') { st.chatKey = a.key; st.picked = null; } else {
      st.chatKey = ''; st.picked = it;
      if (!st.pickPersona) st.pickPersona = assistantId() || (catalog && catalog.assistant ? NEW_ASSISTANT : ((store.state.personas || [])[0] || {}).id || '');
    }
    st.step = 'setup'; update();
  }

  function chatStep() {
    const chats = (store.state.chats || []).filter((c) => c.kind !== 'group');
    if (!picker) {
      picker = createChatPicker({ update, onChange: pickChat, autoSelectSelf: false, noGroups: true });
      queueMicrotask(() => picker.load(true));
    }
    return html`
      <div class="ms-title"><span class="ms-n">1</span>Who with?</div>
      <p class="small muted">Tasks run in private chats for now.</p>
      ${chats.length ? html`<div class="field-label small">Your chats</div>
        <div class="ms-chats">${chats.map((c) => {
          const p = personaById(c.personaId);
          return html`<button type="button" class="ms-chat" data-key=${'c-' + c.key} @click=${() => { st.chatKey = c.key; st.step = 'setup'; update(); }}>
            ${avatarStack(c, p, 40)}
            <span class="grow" style="min-width:0"><span class="ms-chat-name ellipsis">${c.name || c.jid}</span>
              <span class="ms-chat-sub ellipsis">${p ? p.name : 'No persona'}</span></span>
            ${c.enabled ? '' : html`<span class="chip chip-sm">off</span>`}
            ${icon('chevron-right', 'ic-sm faint')}
          </button>`;
        })}</div>` : ''}
      <div class="field-label small mt-12">${chats.length ? 'Or any other chat' : 'Pick a chat'}</div>
      ${picker.view()}`;
  }

  function chatLine() {
    const c = chat();
    if (!c) return '';
    const change = chatKey ? '' : html`<button class="link-btn" @click=${() => { st.step = 'chat'; update(); }}>Change</button>`;
    if (!assigned()) {
      const personas = store.state.personas || [];
      return html`<div class="ms-for tk-pick">${chatAvatar({ jid: st.picked.jid, name: c.name, kind: st.picked.kind }, 30)}
        <span class="grow ellipsis"><b>${c.name}</b> <span class="faint">${chatSubtitle(st.picked)}</span></span>${change}</div>
      <div class="two-col mt-8">
        <label class="field"><span class="field-label">Who writes</span>
          <select class="select input-sm" @change=${(e) => { st.pickPersona = e.target.value; update(); }}>
            ${!assistantId() && catalog && catalog.assistant ? html`<option value=${NEW_ASSISTANT} ?selected=${st.pickPersona === NEW_ASSISTANT}>Assistant (added for you)</option>` : ''}
            ${personas.map((p) => html`<option value=${p.id} ?selected=${st.pickPersona === p.id}>${p.name}</option>`)}
          </select>
          <span class="field-help">This chat gets this persona when the task starts.</span></label>
        <div class="field"><span class="field-label">Approve before sending</span>
          <span class="row gap-8">${toggle(st.pickApproval, (v) => { st.pickApproval = v; update(); }, { label: 'Approve before sending' })}
            <span class="field-help">Every message waits in Approvals until you tap Send.</span></span></div>
      </div>`;
    }
    const p = personaById(c.personaId);
    return html`<div class="ms-for">${avatarStack(c, p, 30)}<span class="grow ellipsis"><b>${p ? p.name : 'The persona'}</b> with ${c.name || c.jid}</span>${change}</div>`;
  }

  function num(label, value, lo, hi, set, unit = '') {
    return html`<label class="tk-numrow"><span class="small">${label}</span>
      <span class="row gap-4"><input class="input input-sm tk-num" type="number" min=${lo} max=${hi} .value=${String(value ?? '')}
        aria-label=${label} @change=${(e) => { set(Math.max(lo, Math.min(hi, parseInt(e.target.value, 10) || lo))); update(); }}>${unit ? html`<span class="tiny faint">${unit}</span>` : ''}</span></label>`;
  }

  function meetingFields() {
    const mt = m();
    const days = mt.days && mt.days.length ? mt.days : ['mon', 'tue', 'wed', 'thu', 'fri'];
    return html`
      <div class="tk-form">
        <label class="field"><span class="field-label">What is it about?</span>
          <input class="input" maxlength="200" placeholder="e.g. project kickoff" .value=${st.spec.about || ''} @input=${(e) => { st.spec.about = e.target.value; update(); }}>
          <span class="field-help">Said in the messages and used as the calendar title.</span></label>
        <div class="two-col">
          <label class="field"><span class="field-label">How long</span>
            <select class="select" @change=${(e) => { mt.minutes = parseInt(e.target.value, 10); update(); }}>
              ${LENGTHS.map((n) => html`<option value=${n} ?selected=${mt.minutes === n}>${n < 60 ? n + ' minutes' : n / 60 + (n === 60 ? ' hour' : ' hours')}</option>`)}
            </select></label>
          <label class="field"><span class="field-label">Where <span class="faint">(optional)</span></span>
            <input class="input" maxlength="120" placeholder="e.g. Zoom, the office" .value=${st.spec.place || ''} @input=${(e) => { st.spec.place = e.target.value; }}></label>
        </div>
        <div class="field"><span class="field-label">Which days</span>
          <span class="chip-row">${DAYS.map(([d, l]) => html`<button type="button" class="chip" aria-pressed=${String(days.includes(d))}
            @click=${() => { mt.days = days.includes(d) ? days.filter((x) => x !== d) : DAYS.map(([k]) => k).filter((k) => k === d || days.includes(k)); update(); }}>${l}</button>`)}</span></div>
        <div class="two-col">
          <div class="field"><span class="field-label">Between</span>
            <span class="row gap-8"><input class="input input-sm time-input" type="time" aria-label="From" .value=${mt.from || '09:00'} @change=${(e) => { mt.from = e.target.value; update(); }}>
              <span class="faint">and</span><input class="input input-sm time-input" type="time" aria-label="To" .value=${mt.to || '18:00'} @change=${(e) => { mt.to = e.target.value; update(); }}></span></div>
          <label class="field"><span class="field-label">Within</span>
            <select class="select" @change=${(e) => { mt.horizonDays = parseInt(e.target.value, 10); update(); }}>
              ${AHEAD.map((n) => html`<option value=${n} ?selected=${mt.horizonDays === n}>the next ${n} days</option>`)}
            </select></label>
        </div>
        <div class="field"><span class="field-label">Prefer</span>${seg(PREFER, mt.prefer || '', (v) => { mt.prefer = v; update(); }, { label: 'Preferred part of day', cls: 'seg-sm' })}</div>
        <div class="field-row">
          <div class="grow"><div class="field-label">Use my calendars ${helpTip('task-calendar')}</div>
            <div class="field-help">Takes your busy times out. The AI never sees your calendar, only the times it may offer.</div></div>
          ${toggle(mt.calendars, (v) => { mt.calendars = v; update(); }, { label: 'Use my calendars' })}
        </div>
        <div class="field"><span class="field-label">Writes</span>
          ${seg([{ value: 'as-you', label: 'As me' }, { value: 'assistant', label: 'As my assistant' }], st.spec.voice || 'as-you', (v) => { st.spec.voice = v; update(); }, { label: 'Voice', cls: 'seg-sm' })}
          <span class="field-help">${st.spec.voice === 'assistant' ? `Openly arranges it for you${(defaults().owner || {}).name ? ', ' + defaults().owner.name : ''} (your name and "About you" are in Settings › Tasks).` : 'Writes as you would, in the persona\'s voice.'}</span></div>
      </div>`;
  }

  function moreFields() {
    const p = privacy();
    const l = limits();
    const cm = commit();
    const cals = st.cals;
    return html`<div class="tk-more">
      <div class="eyebrow">How it shows your time ${helpTip('task-strategy')}</div>
      ${seg(STRATEGIES.map(({ value, label }) => ({ value, label })), p.strategy, (v) => { p.strategy = v; update(); }, { label: 'Strategy', cls: 'seg-sm' })}
      <div class="field-help">${(STRATEGIES.find((x) => x.value === p.strategy) || {}).help || ''}</div>
      <div class="tk-nums">
        ${p.strategy !== 'they-first' ? num('Times per offer', p.offers, 1, 3, (v) => { p.offers = v; }) : ''}
        ${num('"Doesn\'t work" answers before it stops', p.probeLimit, 1, 10, (v) => { p.probeLimit = v; })}
        ${num('Free time kept around busy times', p.bufferMin, 0, 60, (v) => { p.bufferMin = v; }, 'min')}
      </div>
      <div class="field"><span class="field-label small">Then</span>${seg(PROBE_THEN, p.onProbeLimit, (v) => { p.onProbeLimit = v; update(); }, { label: 'At the probe limit', cls: 'seg-sm' })}</div>

      <div class="eyebrow mt-12">When they ask "why not then?" ${helpTip('task-cover')}</div>
      ${seg([{ value: 'deflect', label: 'Brush it off' }, { value: 'lines', label: 'Use my lines' }], p.cover, (v) => { p.cover = v; update(); }, { label: 'Why not', cls: 'seg-sm' })}
      ${p.cover === 'lines' ? html`<textarea class="textarea mt-8" rows="3" placeholder="One line per row, e.g. I'm travelling a lot this month"
          @change=${(e) => { p.coverLines = e.target.value.split('\n').map((x) => x.trim()).filter(Boolean).slice(0, 10); update(); }}>${(p.coverLines || []).join('\n')}</textarea>
        <div class="field-help">Only these lines are ever sent. Settings › Tasks can draft some for you.</div>` : ''}

      <div class="eyebrow mt-12">Confirming and booking ${helpTip('task-confirm')}</div>
      ${seg([{ value: 'approve', label: 'Ask me first' }, { value: 'auto-if-free', label: 'Confirm by itself if I\'m free' }], cm.mode, (v) => { cm.mode = v; update(); }, { label: 'Confirming', cls: 'seg-sm' })}
      <div class="field mt-8"><span class="field-label small">Then</span>${seg(BOOK, cm.book, (v) => { cm.book = v; update(); }, { label: 'Booking', cls: 'seg-sm' })}</div>
      ${cm.book === 'macalendar' ? html`<div class="field-help">${cals.some((c) => c.kind === 'macalendar') ? 'Adds the meeting to MACalendar through its own API.' : 'Connect MACalendar in Settings › Calendars first.'}</div>` : ''}

      <div class="eyebrow mt-12">Limits</div>
      <div class="tk-nums">
        ${num('Messages it may send', l.maxMessages, 4, 100, (v) => { l.maxMessages = v; })}
        ${num('Days before it hands over', l.maxDays, 1, 30, (v) => { l.maxDays = v; })}
        ${num('Nudges when they go quiet', l.maxNudges, 0, 5, (v) => { l.maxNudges = v; })}
        ${num('Nudge after', l.nudgeAfterHours, 1, 168, (v) => { l.nudgeAfterHours = v; }, 'hours')}
      </div>
    </div>`;
  }

  function tryPanel() {
    const t = st.try;
    const personas = store.state.personas || [];
    const c = chat();
    return html`<div class="tk-try">
      <div class="row gap-8 row-wrap">
        <div class="grow"><div class="field-label">Try it first ${helpTip('task-try')}</div>
          <div class="field-help">An AI plays the other person. Nothing is sent, saved or booked; your calendar is only read.</div></div>
        <select class="select input-sm" style="width:auto" aria-label="Persona" @change=${(e) => { st.tryPersona = e.target.value; }}>
          ${personas.map((p) => html`<option value=${p.id} ?selected=${(st.tryPersona || (c && c.personaId) || 'assistant') === p.id}>${p.name}</option>`)}
        </select>
        ${seg(STYLES, st.tryStyle, (v) => { st.tryStyle = v; update(); }, { label: 'The other person is', cls: 'seg-sm' })}
        <button class=${'btn btn-glass btn-sm ' + (st.busy === 'try' ? 'loading' : '')} ?disabled=${!!st.busy} @click=${tryIt}>${icon('play')}Try</button>
      </div>
      ${t && t.error ? html`<div class="banner danger small mt-8">${icon('warning')}<div>${t.error}</div></div>` : ''}
      ${t && t.turns ? html`<ol class="tk-try-chat">${t.turns.map((x) => html`<li class=${'tk-bubble ' + x.speaker}>
          <span>${x.text}</span>${x.note ? html`<span class="tiny faint">${x.note}</span>` : ''}</li>`)}</ol>
        <div class="small muted">${t.result && t.result.when ? html`${icon('check', 'ic-sm')}Would agree on ${when(t.result.when)}.` : t.status === 'handed-off' ? 'It would hand over to you here.' : 'No time agreed in these turns.'}</div>` : ''}
    </div>`;
  }

  function setupStep() {
    if (st.error) return html`<div class="banner danger">${icon('warning')}<div>${st.error}</div></div>`;
    if (!st.spec || !catalog) return html`<div class="skeleton" style="height:320px;border-radius:16px"></div>`;
    const c = chat();
    return html`
      ${st.saveOnly ? '' : chatLine()}
      <div class="ms-title"><span class="ms-n">${chatKey || st.saveOnly ? 1 : 2}</span>Find a time ${helpTip('tasks')}</div>
      ${meetingFields()}
      <div class="bh-block adv-switch-row mt-12">
        <div class="grow"><div class="field-label">More details</div><div class="field-help">${st.more ? 'Just for this task. Your defaults are in Settings › Tasks.' : 'Privacy, "why not then?", confirming, booking and limits. Off = your defaults.'}</div></div>
        ${toggle(st.more, (v) => { st.more = v; update(); }, { label: 'More details' })}
      </div>
      ${st.more ? moreFields() : ''}
      <div class="bh-block adv-switch-row mt-12">
        <div class="grow"><div class="field-label">Advanced: the rules ${helpTip('task-rules')}</div><div class="field-help">${st.advanced ? 'Your own rule list for this task.' : 'Off = the template\'s rules.'}</div></div>
        ${toggle(st.advanced, (v) => { st.advanced = v; update(); }, { label: 'Edit the rules' })}
      </div>
      ${st.advanced ? ruleEditor({ rules: st.spec.rules, vocab: catalog.vocabulary, slots: st.spec.slots, defaults: catalog.defaultRules,
        onChange: (r) => { st.spec.rules = r; update(); } }) : ''}
      ${tryPanel()}
      ${c && !c.enabled ? html`<div class="banner warn small mt-8">${icon('warning')}<div>Replies are off in this chat. Starting the task turns them on.</div></div>` : ''}
      ${c && c.mode && c.mode !== 'auto' ? html`<div class="banner info small mt-8">${icon('approvals')}<div>This chat is in ${c.mode === 'copilot' ? 'co-pilot' : 'approve'} mode: every task message waits in Approvals too.</div></div>` : ''}`;
  }

  ctl = openSheet((sh) => html`
    <div class="sheet-head">
      <div class="sheet-icon tk-icon">${icon('calendar')}</div>
      <div class="grow"><h2>${st.saveOnly ? 'Edit task' : 'New task'}</h2><p>Agree on a meeting time. It only shows the times it offers, never your calendar.</p></div>
      <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => sh.close()}>${icon('x')}</button>
    </div>
    <div class="ms-body tk-body">${st.step === 'chat' ? chatStep() : setupStep()}</div>
    ${st.step === 'setup' && st.spec ? html`<div class="sheet-actions">
      ${st.saveOnly ? '' : html`<span class="row gap-8 small grow">${toggle(st.open, (v) => { st.open = v; update(); }, { label: 'Write first now', small: true })}Write first now</span>`}
      <button class=${'btn btn-ghost ' + (st.busy === 'save' ? 'loading' : '')} ?disabled=${!!st.busy} @click=${saveForLater}>${icon('pin')}${st.saveOnly ? 'Save' : 'Save for later'}</button>
      ${st.saveOnly ? '' : html`<button class=${'btn btn-primary ' + (st.busy === 'start' ? 'loading' : '')} ?disabled=${!!st.busy || !chat() || !chat().personaId && st.pickPersona !== NEW_ASSISTANT} @click=${start}>${icon('play')}Start</button>`}
    </div>` : ''}`, { size: 'wide', label: 'New task', noAutofocus: true });
  return ctl;
}
