// pages/tasks.js — Tasks: errands your personas do in a chat, like agreeing
// on a meeting time. Confirmations waiting for you, tasks running (what it
// offered, what happened), your saved tasks and finished ones (with the
// calendar file). Data: GET /api/tasks, /api/tasks/runs. Sheet:
// components/tasks/task-sheet.js.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { bus, personaById, chatByKey } from '../store.js';
import { confirmSheet, toast, busy } from '../ui.js';
import { debounce, plural } from '../util.js';
import { emptyState } from '../components/art/art.js';
import { avatarStack } from '../components/common/avatar.js';
import { helpTip } from '../components/common/help-tip.js';
import { openTaskSheet, loadTaskCatalog } from '../components/tasks/task-sheet.js';
import { STATUS, STEP_LABEL, when } from '../components/tasks/task-labels.js';

export default function Tasks(ctx) {
  const st = { runs: null, saved: null, assistant: null, error: '', open: {} };

  async function load() {
    try {
      const [runs, cat] = await Promise.all([api.tasks.runs({}, { quiet: true }), loadTaskCatalog(true)]);
      st.runs = (runs && runs.items) || [];
      st.saved = (cat && cat.items) || [];
      st.assistant = (cat && cat.assistant) || null;
      st.error = '';
    } catch (e) { if (!st.runs) st.error = (e && e.message) || 'Could not load tasks'; }
    ctx.update();
  }
  const reloadSoon = debounce(load, 200);
  const offs = [];

  const newTask = (chatKey = '') => openTaskSheet({ chatKey, onDone: () => reloadSoon() });

  async function approve(run, yes) {
    if (!yes && !await confirmSheet({ title: 'Not this time?', body: `The task stops and ${run.chatName || 'the chat'} is handed over to you, paused, so you can answer yourself.`, confirm: 'Hand it to me', iconName: 'hand' })) return;
    try {
      await api.tasks.approve(run.id, yes);
      toast(yes ? 'Confirmed: it lets them know' : 'Handed over to you', { type: 'success' });
      load();
    } catch { /* toasted */ }
  }

  async function stop(run) {
    if (!await confirmSheet({ title: 'Stop this task?', body: `It stops working on "${run.spec.title}" in ${run.chatName}. Nothing more is sent for it.`, confirm: 'Stop task', danger: true, iconName: 'stop' })) return;
    try { await api.tasks.stop(run.id); toast('Task stopped', { type: 'success' }); load(); } catch { /* toasted */ }
  }

  async function removeSaved(t) {
    if (!await confirmSheet({ title: `Delete "${t.title}"?`, body: 'Only the saved task goes. Tasks already running keep going.', confirm: 'Delete', danger: true, iconName: 'trash' })) return;
    try { await api.tasks.remove(t.id); load(); } catch { /* toasted */ }
  }

  async function addAssistant() {
    try { await api.personas.create(st.assistant); toast('Assistant persona added', { type: 'success' }); st.assistant = null; ctx.update(); } catch { /* toasted */ }
  }

  function startSaved(t) {
    openTaskSheet({ spec: t, onDone: () => reloadSoon() });
  }

  // ── Views ──
  function who(run) {
    const c = chatByKey(run.chatKey) || { key: run.chatKey, name: run.chatName };
    const p = personaById(c.personaId);
    return html`${avatarStack(c, p, 30)}<span class="grow ellipsis"><b>${run.spec.title}</b> with ${run.chatName || c.name}${p ? html` <span class="faint">· ${p.name}</span>` : ''}</span>`;
  }

  function logList(run) {
    const items = (run.log || []).slice().reverse();
    return html`<ol class="tk-log">${items.map((e) => html`<li><span class="tiny faint"><rel-time datetime=${e.at}></rel-time></span><span class="small">${e.text}</span></li>`)}</ol>`;
  }

  function approvalCard(run) {
    return html`<article class="card tk-card tk-wait" data-key=${'w-' + run.id}>
      <div class="tk-card-top">${who(run)}<span class=${'chip chip-sm ' + STATUS.approval.chip}>${STATUS.approval.label}</span></div>
      <p class="tk-big">${run.chatName || 'They'} agreed to <b>${when(run.proposed)}</b>${run.spec.about ? html` for ${run.spec.about}` : ''}.</p>
      <p class="small muted">Confirm it and ${run.spec.commit && run.spec.commit.book === 'macalendar' ? 'it is booked in MACalendar, then' : ''} the persona lets them know. It checks your calendar once more first.</p>
      <div class="tk-actions">
        <button class="btn btn-primary btn-sm" @click=${busy(() => approve(run, true))}>${icon('check')}Confirm it</button>
        <button class="btn btn-ghost btn-sm" @click=${() => approve(run, false)}>${icon('hand')}I'll handle it</button>
        <a class="btn btn-ghost btn-sm" href=${'#/chats/' + encodeURIComponent(run.chatKey)}>${icon('chats')}Open chat</a>
      </div>
    </article>`;
  }

  function runningCard(run) {
    const open = !!st.open[run.id];
    return html`<article class="card tk-card" data-key=${'r-' + run.id}>
      <div class="tk-card-top">${who(run)}<span class=${'chip chip-sm ' + STATUS.active.chip}>${STEP_LABEL[run.step] || 'Running'}</span></div>
      ${run.lastOffer && run.lastOffer.length ? html`<div class="tk-times"><span class="eyebrow">Last offered</span>${run.lastOffer.map((t) => html`<span class="chip chip-sm">${icon('clock', 'ic-xs')}${when(t)}</span>`)}</div>` : ''}
      <div class="small muted">${plural(run.sent || 0, 'message')} sent${run.nudges ? ` · ${plural(run.nudges, 'nudge')}` : ''} · started <rel-time datetime=${run.startedAt}></rel-time></div>
      ${open ? logList(run) : ''}
      <div class="tk-actions">
        <button class="btn btn-ghost btn-sm" aria-expanded=${String(open)} @click=${() => { st.open[run.id] = !open; ctx.update(); }}>${icon(open ? 'chevron-up' : 'chevron-down')}${open ? 'Hide steps' : 'What happened'}</button>
        <a class="btn btn-ghost btn-sm" href=${'#/chats/' + encodeURIComponent(run.chatKey)}>${icon('chats')}Open chat</a>
        <span class="grow"></span>
        <button class="btn btn-ghost btn-sm" @click=${() => stop(run)}>${icon('stop')}Stop</button>
      </div>
    </article>`;
  }

  function finishedRow(run) {
    const s = STATUS[run.status] || STATUS.stopped;
    const r = run.result;
    const open = !!st.open[run.id];
    return html`<li class="tk-done" data-key=${'f-' + run.id}>
      <div class="tk-done-top">
        <span class="grow" style="min-width:0"><b>${run.spec.title}</b> <span class="muted">with ${run.chatName}</span>
          <span class="tiny faint">· <rel-time datetime=${run.endedAt || run.updatedAt}></rel-time></span></span>
        <span class=${'chip chip-sm ' + s.chip}>${s.label}</span>
      </div>
      ${r && r.when ? html`<div class="small">${icon('calendar', 'ic-sm')}Agreed on <b>${when(r.when)}</b>${r.booked === 'macalendar' ? ' · in MACalendar' : ''}</div>` : ''}
      ${run.summary && !(r && r.ok) ? html`<div class="small muted">${run.summary}</div>` : ''}
      <div class="row gap-8 mt-4">
        ${r && r.when ? html`<a class="btn btn-glass btn-xs" href=${api.tasks.icsURL(run.id)} download="meeting.ics">${icon('download')}Add to my calendar</a>` : ''}
        <button class="btn btn-ghost btn-xs" @click=${() => { st.open[run.id] = !open; ctx.update(); }}>${open ? 'Hide steps' : 'What happened'}</button>
      </div>
      ${open ? logList(run) : ''}
    </li>`;
  }

  function savedRow(t) {
    return html`<li class="tk-saved" data-key=${'s-' + t.id}>
      <span class="tk-saved-ic">${icon('calendar')}</span>
      <div class="grow" style="min-width:0"><b>${t.title}</b>${t.about ? html` <span class="muted">· ${t.about}</span>` : ''}
        <div class="tiny faint">${t.meeting.minutes} min · ${t.meeting.from}–${t.meeting.to} · next ${t.meeting.horizonDays} days</div></div>
      <button class="btn btn-glass btn-xs" @click=${() => startSaved(t)}>${icon('play')}Start</button>
      <button class="btn btn-ghost btn-icon btn-xs" aria-label=${'Edit ' + t.title} @click=${() => openTaskSheet({ spec: t, save: true, onDone: () => reloadSoon() })}>${icon('edit')}</button>
      <button class="btn btn-ghost btn-icon btn-xs" aria-label=${'Delete ' + t.title} @click=${() => removeSaved(t)}>${icon('trash')}</button>
    </li>`;
  }

  function view() {
    if (!st.runs && st.error) {
      return html`<div class="card">${emptyState({ artName: 'missions', title: "Couldn't load tasks", body: st.error, action: { label: 'Try again', icon: 'refresh', onClick: load } })}</div>`;
    }
    const runs = st.runs || [];
    const waiting = runs.filter((r) => r.status === 'approval');
    const running = runs.filter((r) => r.status === 'active');
    const finished = runs.filter((r) => !['active', 'approval'].includes(r.status)).slice(0, 30);
    return html`<div class="tasks">
      <section class="mi-hero">
        <div class="mi-hero-text">
          <h2 class="hero-title">Let your persona arrange things</h2>
          <p class="muted">Pick a chat and say what the meeting is. It offers times that suit you, without ever showing your calendar, and asks you before anything is confirmed. ${helpTip('tasks')}</p>
          <button class="btn btn-primary mt-12" @click=${() => newTask()}>${icon('plus')}New task</button>
        </div>
        <div class="card mi-stats">${[
          { n: st.runs ? waiting.length : '–', l: 'waiting for you', icon: 'hand', c: 'var(--amber)' },
          { n: st.runs ? running.length : '–', l: 'running', icon: 'clock', c: 'var(--teal)' },
          { n: st.runs ? runs.filter((r) => r.status === 'done').length : '–', l: 'done', icon: 'check', c: 'var(--green)' },
        ].map((t) => html`<div class="stat"><span class="stat-icon" style=${`--c:${t.c}`}>${icon(t.icon)}</span><span class="stat-n">${t.n}</span><span class="stat-l">${t.l}</span></div>`)}</div>
      </section>

      ${st.assistant ? html`<div class="banner info"><span>${icon('personas')}</span><div class="grow"><b>A neutral persona for tasks.</b>
        The Assistant writes clearly and politely, in the other person's language, with no jokes or slang. Give it a chat for errands.</div>
        <button class="btn btn-glass btn-sm" @click=${busy(addAssistant)}>${icon('plus')}Add the Assistant</button></div>` : ''}

      ${waiting.length ? html`<section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('hand', 'ic-sm')}Waiting for you</h3></div>
        <div class="mi-active-grid">${waiting.map(approvalCard)}</div>
      </section>` : ''}

      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('clock', 'ic-sm')}Running</h3>${running.length ? html`<span class="faint small">${plural(running.length, 'task')}</span>` : ''}</div>
        ${!st.runs ? html`<div class="mi-active-grid">${[1, 2].map(() => html`<div class="card skeleton" style="height:170px"></div>`)}</div>`
          : running.length ? html`<div class="mi-active-grid">${running.map(runningCard)}</div>`
          : html`<div class="card">${emptyState({ artName: 'missions', small: true, title: 'No tasks running', body: 'Start one and the persona works on it in that chat, then asks you before confirming.',
              action: { label: 'New task', icon: 'plus', onClick: () => newTask() } })}</div>`}
      </section>

      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('pin', 'ic-sm')}Saved tasks</h3></div>
        ${st.saved && st.saved.length ? html`<ul class="card tk-saved-list">${st.saved.map(savedRow)}</ul>`
          : html`<div class="card mi-empty-done"><span class="small muted">Save a task you use often (for example "30-minute call, mornings") and start it in any chat with one click.</span></div>`}
      </section>

      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('check-double', 'ic-sm')}Finished</h3></div>
        ${finished.length ? html`<ul class="card tk-done-list">${finished.map(finishedRow)}</ul>`
          : html`<div class="card mi-empty-done"><span class="small muted">Finished tasks show up here, with the time agreed and a calendar file.</span></div>`}
      </section>
    </div>`;
  }

  return {
    title: 'Tasks',
    subtitle: () => {
      const n = (st.runs || []).filter((r) => r.status === 'approval').length;
      return n ? `${plural(n, 'confirmation')} waiting for you` : '';
    },
    view,
    mount() {
      load();
      offs.push(bus.on('tasks.changed', reloadSoon), bus.on('chats.changed', reloadSoon));
    },
    unmount() { for (const off of offs) off(); reloadSoon.cancel(); },
  };
}
