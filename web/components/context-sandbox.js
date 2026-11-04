// components/context-sandbox.js — the context-system sandbox on the Context
// page: pick a chat and up to three systems; the server answers the chat's
// last messages from them again with each (POST /api/context/replay, nothing
// is sent) and the run fills in live (SSE context.changed {replayId}).
// Shows AI calls per step, prompt size, time and the replies side by side.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store } from '../store.js';
import { seg, spinner, toast } from '../ui.js';
import { plural } from '../util.js';
import { helpTip } from './help-tip.js';

const STEPS = [
  { id: 'join', label: 'Join in?', c: 'var(--blue)' },
  { id: 'plan', label: 'Goal plan', c: 'var(--violet)' },
  { id: 'card', label: 'Persona card', c: 'var(--pink)' },
  { id: 'reply', label: 'Reply', c: 'var(--teal)' },
  { id: 'check', label: 'Judge', c: 'var(--amber)' },
  { id: 'rewrite', label: 'Rewrite', c: 'var(--orange)' },
];

const secs = (ms) => (ms >= 10000 ? Math.round(ms / 1000) + ' s' : (ms / 1000).toFixed(1) + ' s');

export function createSandbox(update) {
  const st = { chatKey: '', picked: ['classic', 'lean'], messages: 8, run: null, starting: false };
  let poll = null;

  async function refresh() {
    if (!st.run) return;
    try { st.run = await api.context.replayGet(st.run.id, { quiet: true }); } catch { /* keep the last view */ }
    if (st.run.status !== 'running' && poll) { clearInterval(poll); poll = null; }
    update();
  }

  async function start() {
    st.starting = true; update();
    try {
      st.run = await api.context.replay({ chatKey: st.chatKey, systems: st.picked, messages: st.messages });
      if (poll) clearInterval(poll);
      poll = setInterval(refresh, 4000); // SSE does the live part; this is the safety net
    } catch { /* toasted */ }
    st.starting = false; update();
  }

  async function stop() {
    if (!st.run) return;
    try { await api.context.replayStop(st.run.id); toast('Stopping the sandbox'); } catch { /* toasted */ }
  }

  function togglePick(id) {
    if (st.picked.includes(id)) st.picked = st.picked.filter((x) => x !== id);
    else if (st.picked.length < 3) st.picked = [...st.picked, id];
    else toast('Pick up to three systems');
    update();
  }

  function stepBar(r) {
    const total = Math.max(1, r.calls);
    return html`<div class="cs-bar" role="img" aria-label=${`${r.calls} AI calls`}>
      ${STEPS.filter((s) => r.callsByStep && r.callsByStep[s.id]).map((s) => html`<span style=${`flex:${r.callsByStep[s.id] / total};background:${s.c}`}
        title=${`${s.label}: ${r.callsByStep[s.id]}`}></span>`)}
    </div>
    <div class="cs-legend">${STEPS.filter((s) => r.callsByStep && r.callsByStep[s.id]).map((s) => html`<span><i style=${`background:${s.c}`}></i>${s.label} ${r.callsByStep[s.id]}</span>`)}</div>`;
  }

  function resultCard(r, maxCalls) {
    const per = r.replied ? (r.calls / r.replied).toFixed(1) : '–';
    return html`<div class="card cs-res" data-key=${r.systemId}>
      <div class="cs-res-head"><strong>${r.systemName}</strong><span class="faint small">${r.replied} answered · ${r.skipped} quiet</span></div>
      <div class="cs-res-stats">
        <div><span class="cs-big">${r.calls}</span><span class="faint small">AI calls</span></div>
        <div><span class="cs-big">${per}</span><span class="faint small">per answer</span></div>
        <div><span class="cs-big">${r.promptChars ? (r.promptChars / 1000).toFixed(1) + 'k' : '–'}</span><span class="faint small">prompt size</span></div>
        <div><span class="cs-big">${secs(r.totalMs)}</span><span class="faint small">AI time</span></div>
      </div>
      <div class="cs-res-bar" style=${`width:${Math.max(8, (r.calls / Math.max(1, maxCalls)) * 100)}%`}>${stepBar(r)}</div>
      ${r.background ? html`<div class="faint small mt-8">Plus about ${plural(r.background, 'background call')} for these messages.</div>` : ''}
    </div>`;
  }

  function compare(run) {
    const res = run.results || [];
    if (!res.length || !res[0].steps.length) return '';
    const rows = res[0].steps.map((_, i) => i);
    return html`<div class="cs-compare" style=${`--cols:${res.length}`}>
      <div class="cs-crow cs-chead"><div>Their message</div>${res.map((r) => html`<div>${r.systemName}</div>`)}</div>
      ${rows.map((i) => {
        const first = res[0].steps[i];
        return html`<div class="cs-crow" data-key=${'r' + i}>
          <div class="cs-them"><span class="faint small">${first.from}</span><div>${first.text}</div>
            ${first.real ? html`<div class="cs-real"><span class="faint small">It really said</span> ${first.real}</div>` : ''}</div>
          ${res.map((r) => {
            const s = r.steps[i];
            if (!s) return html`<div class="cs-cell cs-pending">${spinner('spinner-sm')}</div>`;
            return html`<div class=${'cs-cell ' + (s.decision === 'reply' ? 'cs-yes' : 'cs-no')}>
              ${s.decision === 'reply' ? html`<div class="cs-bubble">${s.reply}</div>` : html`<div class="muted small">${icon('minus', 'ic-xs')} Stayed quiet</div>`}
              <div class="faint small">${s.why} · ${plural(s.calls, 'call')} · ${secs(s.ms)}</div>
              ${s.checks && s.checks.length ? html`<div class="cs-checks small">${icon('shield', 'ic-xs')} ${s.checks.join('; ')}</div>` : ''}
              ${s.plan ? html`<div class="faint small">Plan: ${s.plan}</div>` : ''}
            </div>`;
          })}
        </div>`;
      })}
    </div>`;
  }

  function view(systems) {
    const chats = store.state.chats.filter((c) => c.personaId);
    if (!st.chatKey && chats.length) st.chatKey = (chats.find((c) => c.kind === 'group') || chats[0]).key;
    const run = st.run;
    const running = run && run.status === 'running';
    const maxCalls = run ? Math.max(1, ...run.results.map((r) => r.calls)) : 1;
    return html`<section class="card cs-sandbox">
      <div class="cs-sec-head"><h3>${icon('playground', 'ic-sm')}Sandbox ${helpTip('cs-sandbox')}</h3>
        <span class="faint small">Answers a chat's last messages again with each system. Nothing is sent.</span></div>
      ${!chats.length ? html`<p class="muted small">Assign a persona to a chat first.</p>` : html`
      <div class="cs-sb-form">
        <label class="field"><span class="field-label">Chat</span>
          <select class="select" @change=${(e) => { st.chatKey = e.target.value; update(); }}>
            ${chats.map((c) => html`<option value=${c.key} ?selected=${c.key === st.chatKey}>${c.name || c.key}</option>`)}
          </select></label>
        <div class="field"><span class="field-label">Systems (up to 3)</span>
          <div class="cs-chips">${systems.map((s) => html`<button type="button" class=${'chip ' + (st.picked.includes(s.id) ? 'chip-on' : '')}
            aria-pressed=${String(st.picked.includes(s.id))} @click=${() => togglePick(s.id)}>${s.name}</button>`)}</div></div>
        <div class="field"><span class="field-label">Their last</span>
          ${seg([4, 8, 12].map((n) => ({ value: n, label: `${n} messages` })), st.messages, (n) => { st.messages = n; update(); }, { label: 'Messages to replay' })}</div>
        <div class="cs-sb-go">
          ${running ? html`<button class="btn btn-glass" @click=${stop}>${icon('stop')}Stop</button>`
            : html`<button class=${'btn btn-primary ' + (st.starting ? 'loading' : '')} ?disabled=${!st.picked.length || !st.chatKey || st.starting} @click=${start}>${icon('play')}Run</button>`}
        </div>
      </div>`}
      ${run ? html`<div class="cs-sb-status small">
          ${running ? html`${spinner('spinner-sm')} Replaying ${run.done} of ${run.total}…` : run.status === 'done' ? html`${icon('check', 'ic-xs')} Done: ${run.chatName}` : html`${icon('warning', 'ic-xs')} ${run.status === 'cancelled' ? 'Stopped' : 'Stopped: it took too long'}`}
          <progress max=${run.total} value=${run.done}></progress>
        </div>
        <div class="cs-results">${run.results.map((r) => resultCard(r, maxCalls))}</div>
        ${compare(run)}` : ''}
    </section>`;
  }

  return {
    view,
    onEvent(d) { if (d && d.replayId && st.run && d.replayId === st.run.id) refresh(); },
    destroy() { if (poll) clearInterval(poll); },
  };
}
