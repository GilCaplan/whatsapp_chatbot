// drawer/overview.js — the chat panel's "Overview" tab: persona, reply mode
// (Auto / Approve / Co-pilot), quick actions (Away, Reveal…, Recap now), the
// latest messages (whole conversation on demand), "Test it" and removing the
// assignment.
// Wave 3 owner: Engineer B.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store, loadChats, isAway } from '../../store.js';
import { seg, confirmSheet, toast, spinner, busy } from '../../ui.js';
import { debounce } from '../../util.js';
import { personaPicker } from '../persona-picker.js';
import { helpTip } from '../help-tip.js';
import { MODES, modeOf } from '../handoff-meta.js';
import { openRevealSheet } from '../reveal-sheet.js';
import { generateRecap } from '../recap-card.js';
import { bubbles } from './shared.js';

export function createOverviewTab(ctx) {
  const { key } = ctx;
  const st = {
    history: [], historyLoaded: false, historyLoading: false,
    sim: '', simFromMe: false, simBusy: false, simFrom: '',
    send: '', sendBusy: false,
    showAll: false, // the whole conversation instead of the latest messages
  };
  const LATEST = 3;

  const scrollWall = () => requestAnimationFrame(() => {
    const w = ctx.el() && ctx.el().querySelector('.chat-wall');
    if (w) w.scrollTop = w.scrollHeight;
  });

  async function loadHistory() {
    st.historyLoading = true;
    ctx.update();
    try {
      const list = await api.chats.history(key, 100, { quiet: true });
      st.history = Array.isArray(list) ? list : [];
    } catch { /* leave as is */ }
    st.historyLoaded = true;
    st.historyLoading = false;
    ctx.update();
    scrollWall();
  }
  const reloadHistorySoon = debounce(loadHistory, 500);

  async function doSimulate() {
    const text = st.sim.trim();
    if (!text || st.simBusy) return;
    st.simBusy = true; ctx.update();
    try {
      await api.chats.simulate(key, text, st.simFromMe, undefined, st.simFromMe ? '' : st.simFrom);
      st.sim = '';
      toast('Simulated message injected — watch the activity feed', { type: 'success' });
      reloadHistorySoon();
    } catch { /* toasted */ } finally { st.simBusy = false; ctx.update(); }
  }

  async function doSend() {
    const text = st.send.trim();
    if (!text || st.sendBusy) return;
    st.sendBusy = true; ctx.update();
    try {
      await api.chats.send(key, text);
      st.send = '';
      toast('Sent', { type: 'success' });
      reloadHistorySoon();
    } catch { /* toasted */ } finally { st.sendBusy = false; ctx.update(); }
  }

  function modeSection(c) {
    const mode = modeOf(c);
    const m = MODES.find((x) => x.value === mode) || MODES[0];
    return html`<section class="drawer-section mode-block">
      <h4>${icon('shield', 'ic-sm')}Reply mode ${helpTip('mode')}</h4>
      ${seg(MODES.map((x) => ({ value: x.value, label: x.label, icon: x.icon })), mode,
        (v) => ctx.safePatch({ mode: v }), { label: 'Reply mode' })}
      <div class="mode-line">${icon(m.icon)}<span>${m.line}</span></div>
      ${mode === 'copilot' ? html`<div class="mode-note">Co-pilot takes a little longer: it writes three replies at once. Nothing is sent until you pick one.</div>` : ''}
    </section>`;
  }

  async function recapNow() {
    const items = await generateRecap(key);
    if (items && items.length) ctx.setTab('memory');
  }

  function quickActions(c, p) {
    const away = isAway(c);
    return html`<section class="drawer-section">
      <h4>${icon('bolt', 'ic-sm')}Quick actions</h4>
      <div class="quick-actions">
        <button class="btn btn-glass btn-sm" @click=${(e) => ctx.away.menu(e)}>${icon('snooze')}${away ? 'Away' : 'Set away'}${icon('chevron-down')}</button>
        <button class="btn btn-glass btn-sm" ?disabled=${!p} title="Tell them it was an AI persona, then pause this chat"
          @click=${() => openRevealSheet(c, p)}>${icon('eye')}Reveal…</button>
        <button class="btn btn-glass btn-sm" @click=${busy(recapNow)}>${icon('text')}Recap now</button>
      </div>
    </section>`;
  }

  function view(c, p) {
    const isGroup = c.kind === 'group';
    const s = store.state.settings || {};
    const prefix = (s.behavior && s.behavior.triggerPrefix) || '';
    const people = ctx.people;
    return html`
      <section class="drawer-section">
        <h4>${icon('personas', 'ic-sm')}Persona</h4>
        ${personaPicker(c.personaId, (id) => ctx.safePatch({ personaId: id }))}
      </section>

      ${modeSection(c)}
      ${quickActions(c, p)}

      <section class="drawer-section">
        <h4>${icon('chats', 'ic-sm')}${st.showAll ? 'Conversation' : 'Latest'} <span class="grow"></span>
          <button class="btn btn-ghost btn-sm" @click=${loadHistory} title="Refresh">${icon('refresh')}</button>
          ${st.history.length ? html`<button class="btn btn-ghost btn-sm" @click=${async () => {
            if (!await confirmSheet({ title: 'Clear this chat\'s memory?', body: 'The persona forgets the conversation so far. Nothing is deleted from WhatsApp.', confirm: 'Clear memory', danger: true, iconName: 'trash' })) return;
            await api.chats.clearHistory(key); st.history = []; ctx.update(); toast('Memory cleared', { type: 'success' });
          }}>${icon('trash')}Clear</button>` : ''}
        </h4>
        <div class="chat-wall drawer-wall">
          ${!st.historyLoaded ? html`<div class="center" style="padding:24px">${spinner()}</div>`
            : st.history.length ? bubbles(st.showAll ? st.history : st.history.slice(-LATEST), { group: isGroup })
            : html`<div class="empty empty-sm"><p class="small">No messages remembered yet. ${prefix ? html`From your phone, send <code>${prefix} hi</code> in this chat to test.` : ''}</p></div>`}
        </div>
        ${st.history.length > LATEST ? html`<button class="link-btn small mt-8" @click=${() => { st.showAll = !st.showAll; ctx.update(); scrollWall(); }}>
          ${st.showAll ? 'Show only the latest messages' : `Show the whole conversation (${st.history.length} messages)`}</button>` : ''}
      </section>

      <section class="drawer-section">
        <h4>${icon('playground', 'ic-sm')}Test it</h4>
        <div class="composer">
          <input class="input" placeholder="Simulate an incoming message…" .value=${st.sim}
            @input=${(e) => { st.sim = e.target.value; }}
            @keydown=${(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); doSimulate(); } }}>
          <button class=${'btn btn-glass ' + (st.simBusy ? 'loading' : '')} @click=${doSimulate} title="Inject a fake incoming message into the real pipeline">${icon('arrow-up')}Simulate</button>
        </div>
        <div class="row row-wrap gap-10 mt-8">
          <label class="toggle-row small muted">
            <input type="checkbox" ?checked=${st.simFromMe} @change=${(e) => { st.simFromMe = e.target.checked; ctx.update(); }}>
            Pretend I sent it (uses the trigger prefix rules)
          </label>
          ${isGroup && !st.simFromMe && people.members().some((m) => !m.isSelf && !m.left) ? html`<label class="sim-from small muted">From
            <select class="select select-sm" aria-label="Who the simulated message comes from" @change=${(e) => { st.simFrom = e.target.value; }}>
              <option value="" ?selected=${!st.simFrom}>Someone at random</option>
              ${people.members().filter((m) => !m.isSelf && !m.left).map((m) => html`<option value=${m.jid} ?selected=${st.simFrom === m.jid}>${m.name || m.phone || m.jid}</option>`)}
            </select></label>` : ''}
        </div>
        <div class="composer mt-12">
          <input class="input" placeholder=${`Send as ${p ? p.name : 'the persona'}…`} .value=${st.send}
            @input=${(e) => { st.send = e.target.value; }}
            @keydown=${(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); doSend(); } }}>
          <button class=${'btn btn-primary ' + (st.sendBusy ? 'loading' : '')} @click=${doSend}>${icon('send')}Send</button>
        </div>
        <div class="field-help mt-4">“Send” really sends this text on WhatsApp, written by you but shown as the persona's message in its memory.</div>
      </section>

      <section class="drawer-section danger-zone">
        <button class="btn btn-danger" @click=${async () => {
          if (!await confirmSheet({ title: 'Remove this assignment?', body: `${p ? p.name : 'The persona'} will stop replying in ${c.name || 'this chat'}. Its memory of the chat is kept.`, confirm: 'Remove', danger: true, iconName: 'trash' })) return;
          await api.chats.remove(key); await loadChats(); toast('Assignment removed', { type: 'success' }); ctx.close();
        }}>${icon('trash')}Remove assignment</button>
      </section>`;
  }

  return {
    id: 'overview',
    view,
    load: loadHistory,
    /** Re-show the conversation when this tab becomes visible again (the wall scrolls to the end). */
    shown: scrollWall,
    historyChanged: reloadHistorySoon,
    onBus(type, ev) {
      if (type === 'activity' && ev && ev.chatKey === key && (ev.type === 'incoming' || ev.type === 'sent' || ev.type === 'approval.sent')) reloadHistorySoon();
    },
    destroy() { reloadHistorySoon.cancel(); },
  };
}
