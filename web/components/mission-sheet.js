// mission-sheet.js — "New mission": pick a chat → pick a mission → fill in the
// blanks → preview → Start, then offer "Start the conversation now".
//
//   openMissionSheet({ chatKey?, onStarted?(chat) })   // chatKey skips the chat step

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, personaById, chatByKey, loadChats } from '../store.js';
import { openSheet, toast } from '../ui.js';
import { avatarStack } from './avatar.js';
import { missionBadge } from './badges.js';
import { helpTip } from './help-tip.js';

export const CATEGORY_LABELS = [
  { id: 'words', label: 'Wordplay' },
  { id: 'vibes', label: 'Good vibes' },
  { id: 'curious', label: 'Curious' },
  { id: 'share', label: 'Show and tell' },
  { id: 'plans', label: 'Make plans' },
];
const DIFFICULTY = ['', 'Easy', 'Medium', 'Tricky'];
const MAX_BLANK = 60;

let templatesCache = null;
export async function loadMissionTemplates(force = false) {
  if (templatesCache && !force) return templatesCache;
  const r = await api.missions.list({ quiet: true });
  templatesCache = (r && r.templates) || [];
  return templatesCache;
}

/** Fill a template string's {blanks} (empty → "…"). */
export function fillText(s, blanks) {
  return String(s || '').replace(/\{([a-z]+)\}/g, (_, k) => {
    const v = String((blanks && blanks[k]) || '').trim().replace(/^["'“”‘’]+|["'“”‘’]+$/g, '');
    return v || '…';
  });
}

const firstName = (name) => String(name || '').trim().split(/\s+/)[0] || '';

export function difficultyDots(n) {
  return html`<span class="diff" title=${DIFFICULTY[n] || ''} aria-label=${'Difficulty: ' + (DIFFICULTY[n] || '')}>
    ${[1, 2, 3].map((i) => html`<span class=${i <= n ? 'on' : ''}></span>`)}<span class="diff-l">${DIFFICULTY[n] || ''}</span></span>`;
}

export function openMissionSheet({ chatKey = '', onStarted } = {}) {
  const st = {
    step: chatKey ? 'pick' : 'chat',
    fixedChat: !!chatKey,
    chatKey,
    templates: templatesCache,
    loadError: '',
    tpl: null,            // template or { id: '', custom: true }
    blanks: {},
    custom: '',
    members: null,
    busy: false,
    started: null,        // ChatAssignment after Start
    initBusy: false, initDone: false,
  };
  let ctl = null;
  const update = () => ctl && ctl.update();

  if (!st.templates) {
    loadMissionTemplates().then((t) => { st.templates = t; update(); })
      .catch((e) => { st.loadError = (e && e.message) || 'Could not load missions'; update(); });
  }

  const chat = () => chatByKey(st.chatKey);

  async function loadMembers(c) {
    st.members = null;
    if (!c || c.kind !== 'group') return;
    try {
      const r = await api.chats.people(c.key, { quiet: true });
      st.members = ((r && r.members) || []).filter((m) => !m.isSelf && !m.left && m.name).map((m) => m.name).slice(0, 10);
    } catch { st.members = []; }
    update();
  }
  if (chatKey) loadMembers(chatByKey(chatKey));

  function pickChat(c) {
    st.chatKey = c.key;
    st.step = 'pick';
    loadMembers(c);
    update();
  }

  function pickTemplate(t) {
    st.tpl = t;
    const c = chat();
    st.blanks = {};
    if (t && !t.custom && c && c.kind !== 'group') st.blanks.name = firstName(c.name);
    st.step = 'fill';
    update();
    requestAnimationFrame(() => {
      const el = ctl && ctl.el.querySelector('.ms-fill input:not([value]), .ms-fill input[value=""], .ms-fill textarea');
      if (el) try { el.focus(); } catch { /* ignore */ }
    });
  }

  const goalText = () => (st.tpl && !st.tpl.custom ? fillText(st.tpl.goal, st.blanks) : st.custom.trim());
  const missing = () => {
    if (!st.tpl) return true;
    if (st.tpl.custom) return !st.custom.trim();
    return (st.tpl.blanks || []).some((b) => !String(st.blanks[b.key] || '').trim());
  };

  async function start() {
    if (st.busy || missing()) return;
    st.busy = true; update();
    try {
      const body = st.tpl.custom ? { chatKey: st.chatKey, goal: st.custom.trim() }
        : { chatKey: st.chatKey, templateId: st.tpl.id, blanks: st.blanks };
      st.started = await api.missions.start(body);
      st.step = 'done';
      loadChats().catch(() => {});
      if (onStarted) onStarted(st.started);
    } catch { /* toasted */ } finally { st.busy = false; update(); }
  }

  async function startNow() {
    const c = st.started || chat();
    if (!c || st.initBusy) return;
    st.initBusy = true; update();
    try {
      const r = await api.chats.initiate(c.key, '');
      st.initDone = true;
      const p = personaById(c.personaId);
      toast(r && r.approval ? `${p ? p.name : 'The persona'}'s opener will wait in Approvals` : `${p ? p.name : 'The persona'} is writing an opener`, { type: 'success' });
      ctl.close();
    } catch { /* toasted */ } finally { st.initBusy = false; update(); }
  }

  // ── Steps ──
  function chatStep() {
    const chats = store.state.chats || [];
    return html`
      <div class="ms-title"><span class="ms-n">1</span>Which chat?</div>
      ${!chats.length ? html`<div class="banner info">${icon('info')}<div>Assign a persona to a chat first (Chats page). Then it can go on missions.</div></div>` : html`
        <div class="ms-chats">${chats.map((c) => {
          const p = personaById(c.personaId);
          return html`<button type="button" class="ms-chat" data-key=${'c-' + c.key} @click=${() => pickChat(c)}>
            ${avatarStack(c, p, 40)}
            <span class="grow" style="min-width:0">
              <span class="ms-chat-name ellipsis">${c.name || c.jid}</span>
              <span class="ms-chat-sub ellipsis">${p ? p.name : 'No persona'}${c.goal && c.goal.text ? html` · working on: ${c.goal.text}` : ''}</span>
            </span>
            ${c.enabled ? '' : html`<span class="chip chip-sm">off</span>`}
            ${icon('chevron-right', 'ic-sm faint')}
          </button>`;
        })}</div>`}`;
  }

  function chatLine() {
    const c = chat();
    if (!c) return '';
    const p = personaById(c.personaId);
    return html`<div class="ms-for">
      ${avatarStack(c, p, 30)}<span class="grow ellipsis"><b>${p ? p.name : 'The persona'}</b> in ${c.name || c.jid}</span>
      ${st.fixedChat ? '' : html`<button class="link-btn" @click=${() => { st.step = 'chat'; update(); }}>Change</button>`}
    </div>`;
  }

  function pickStep() {
    const c = chat();
    const who = c && c.kind !== 'group' ? firstName(c.name) : 'someone';
    if (st.loadError) return html`<div class="banner danger">${icon('warning')}<div>${st.loadError}</div></div>`;
    if (!st.templates) return html`<div class="ms-grid">${[1, 2, 3, 4].map(() => html`<div class="ms-card skeleton" style="height:120px"></div>`)}</div>`;
    return html`
      ${chatLine()}
      <div class="ms-title"><span class="ms-n">${st.fixedChat ? 1 : 2}</span>Pick a mission ${helpTip('missions')}</div>
      ${CATEGORY_LABELS.map((cat) => {
        const list = st.templates.filter((t) => t.category === cat.id);
        if (!list.length) return '';
        return html`<div class="ms-cat" data-key=${'cat-' + cat.id}>
          <div class="eyebrow ms-cat-l">${cat.label}</div>
          <div class="ms-grid">${list.map((t) => html`<button type="button" class="ms-card" data-key=${'t-' + t.id} @click=${() => pickTemplate(t)}>
            ${missionBadge(t.badge, t.category, { size: 40 })}
            <span class="ms-card-t">${fillText(t.title, { name: who })}</span>
            <span class="ms-card-b">${t.blurb}</span>
            ${difficultyDots(t.difficulty)}
          </button>`)}</div>
        </div>`;
      })}
      <button type="button" class="ms-card ms-own" @click=${() => pickTemplate({ id: '', custom: true })}>
        <span class="ms-own-ic">${icon('edit')}</span>
        <span class="grow"><span class="ms-card-t">Write your own</span><span class="ms-card-b">Any goal you like, in your own words.</span></span>
        ${icon('chevron-right', 'ic-sm faint')}
      </button>`;
  }

  function fillStep() {
    const c = chat();
    const t = st.tpl;
    const p = c && personaById(c.personaId);
    const mode = c && (c.mode || (c.approvalMode ? 'approve' : 'auto'));
    const current = c && c.goal && c.goal.text;
    return html`
      ${chatLine()}
      <div class="ms-title"><span class="ms-n">${st.fixedChat ? 2 : 3}</span>${t.custom ? 'Describe the goal' : 'Fill in the blanks'}</div>
      <div class="ms-fill">
        ${t.custom ? html`<textarea class="textarea" rows="3" maxlength="500" placeholder="e.g. Find out where Dana is going on holiday" aria-label="Goal"
            @input=${(e) => { st.custom = e.target.value; update(); }}>${st.custom}</textarea>`
          : html`<div class="ms-fill-head">${missionBadge(t.badge, t.category, { size: 48 })}<div><b>${fillText(t.title, st.blanks)}</b><div class="small muted">${t.blurb}</div></div></div>
            ${(t.blanks || []).map((b) => html`<label class="field ms-blank" data-key=${'bl-' + b.key}>
              <span class="field-label">${b.label}</span>
              <input class="input" maxlength=${MAX_BLANK} placeholder=${b.placeholder} value=${st.blanks[b.key] || ''}
                @input=${(e) => { st.blanks[b.key] = e.target.value; update(); }}
                @keydown=${(e) => { if (e.key === 'Enter') { e.preventDefault(); start(); } }}>
              ${b.key === 'name' && st.members && st.members.length ? html`<span class="chip-row mt-4">${st.members.map((m) => html`<button type="button"
                  class="chip chip-sm" aria-pressed=${String(st.blanks.name === firstName(m))} @click=${() => { st.blanks.name = firstName(m); update(); }}>${m}</button>`)}</span>` : ''}
            </label>`)}`}
      </div>
      <div class="ms-preview" aria-live="polite">
        <div class="eyebrow">${p ? p.name : 'The persona'} will quietly work towards</div>
        <p>${goalText() || '…'}</p>
        ${current && current !== goalText() ? html`<div class="ms-replace small muted">${icon('reset', 'ic-sm')}<span>Replaces the current goal: ${current}</span></div>` : ''}
      </div>
      ${c && !c.enabled ? html`<div class="banner warn small mt-8">${icon('warning')}<div>Replies are off in this chat. Turn them on in the chat panel so ${p ? p.name : 'the persona'} can work on it.</div></div>` : ''}
      ${mode && mode !== 'auto' ? html`<div class="banner info small mt-8">${icon('approvals')}<div>${mode === 'copilot' ? 'Co-pilot' : 'Approve'} mode: replies wait in Approvals for your OK.</div></div>` : ''}`;
  }

  function doneStep() {
    const c = st.started || chat();
    const p = c && personaById(c.personaId);
    return html`<div class="ms-done">
      <div class="ms-done-burst" aria-hidden="true">${st.tpl && !st.tpl.custom ? missionBadge(st.tpl.badge, st.tpl.category, { size: 72 }) : html`<span class="tick-badge">${icon('check')}</span>`}</div>
      <h2>Mission started</h2>
      <div class="ms-done-goal">${c ? c.goalOverride : ''}</div>
      <p class="muted">${p ? p.name : 'The persona'} is on it in ${c ? (c.name || c.jid) : 'this chat'}. You'll see it in the activity feed (and on the Missions page) when it happens.</p>
      <div class="ms-done-start">
        <div class="grow"><b>Don't want to wait for them?</b><div class="small muted">${p ? p.name : 'The persona'} can open the conversation right now.</div></div>
        <button class=${'btn btn-primary ' + (st.initBusy ? 'loading' : '')} ?disabled=${!c || !c.enabled || st.initBusy} @click=${startNow}>${icon('message')}Start now</button>
      </div>
    </div>`;
  }

  ctl = openSheet((c) => {
    const step = st.step;
    return html`
      <div class="sheet-head">
        <div class="sheet-icon ms-icon">${icon('target')}</div>
        <div class="grow"><h2>${step === 'done' ? 'Nice one' : 'New mission'}</h2><p>${step === 'done' ? 'Good luck to your persona.' : 'A playful goal your persona works on quietly, in its own words.'}</p></div>
        <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => c.close()}>${icon('x')}</button>
      </div>
      <div class="ms-body" data-step=${step}>
        ${step === 'chat' ? chatStep() : step === 'pick' ? pickStep() : step === 'fill' ? fillStep() : doneStep()}
      </div>
      ${step === 'fill' ? html`<div class="sheet-actions">
        <button class="btn btn-ghost" @click=${() => { st.step = 'pick'; update(); }}>${icon('arrow-left')}Back</button>
        <button class=${'btn btn-primary ' + (st.busy ? 'loading' : '')} ?disabled=${missing() || st.busy} @click=${start}>${icon('target')}Start mission</button>
      </div>` : step === 'done' ? html`<div class="sheet-actions"><button class="btn btn-glass" @click=${() => c.close()}>Done</button></div>` : ''}`;
  }, { size: 'wide', label: 'New mission', noAutofocus: true });
  return ctl;
}
