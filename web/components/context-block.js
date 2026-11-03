// context-block.js — the Memory tab's "What it reads" block: the full
// context of the persona's next reply in a chat (system prompt + messages,
// GET /api/chats/{key}/context), your instructions for the chat, the summary
// of older messages (edit, summarise now, forget, auto-compact) and where
// its history starts ("Start fresh", "Read all history again").

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { toggle, toast, confirmSheet } from '../ui.js';
import { debounce, rowsFor, relTime } from '../util.js';
import { helpTip } from './help-tip.js';
import { patchChat } from './drawer/shared.js';

const MAX_INSTRUCTIONS = 1000;
const MAX_SUMMARY = 1500;

/** ~4 characters per token: rough, but enough to compare. */
const tokens = (chars) => Math.round(chars / 4);

export function createContextStore(key, update) {
  const st = {
    data: null, error: '', showPrompt: false, compacting: false,
    instr: null, // instructions being typed
    summary: null, // summary being typed
  };
  async function load() {
    try {
      st.data = await api.chats.context(key, { quiet: true });
      st.error = '';
    } catch (e) {
      if (!st.data) st.error = (e && e.message) || "Couldn't load what it reads";
    }
    update();
  }
  const reloadSoon = debounce(load, 600);

  const saveInstructions = debounce(async () => {
    const v = (st.instr || '').trim();
    try { await patchChat(key, { instructions: v }); st.instr = null; reloadSoon(); } catch { /* toasted */ }
  }, 800);
  const saveSummary = debounce(async () => {
    const v = (st.summary || '').trim();
    if (!v) return;
    try { await api.chats.putSummary(key, { text: v }); st.summary = null; reloadSoon(); } catch { /* toasted */ }
  }, 900);

  async function compact() {
    st.compacting = true; update();
    try {
      const sm = await api.chats.compact(key, {});
      toast(`Summarised ${sm.messages || 'the older'} messages`, { type: 'success' });
    } catch { /* toasted */ }
    st.compacting = false;
    load();
  }

  async function forgetSummary() {
    if (!await confirmSheet({ title: 'Forget the summary?', body: 'It stops reading the summary of older messages. The messages themselves stay where they are.', confirm: 'Forget', danger: true, iconName: 'trash' })) return;
    try { await api.chats.deleteSummary(key); st.summary = null; toast('Summary forgotten', { type: 'success' }); } catch { /* toasted */ }
    load();
  }

  async function setFrom(value, msg) {
    try { await patchChat(key, { contextFrom: value }); toast(msg, { type: 'success' }); } catch { /* toasted */ }
    load();
  }

  async function setAuto(on) {
    if (st.data) st.data.autoCompact = on;
    update();
    try { await patchChat(key, { autoCompact: on }); } catch { /* toasted */ }
    load();
  }

  return {
    st, load, reloadSoon, compact, forgetSummary, setFrom, setAuto,
    typeInstructions(v) { st.instr = v; update(); saveInstructions(); },
    typeSummary(v) { st.summary = v; update(); saveSummary(); },
    destroy() {
      reloadSoon.cancel();
      if (saveInstructions.pending()) saveInstructions.flush();
      if (saveSummary.pending()) saveSummary.flush();
    },
  };
}

// Colour of each part of the prompt (tokens only; the context parts share one).
const PART_COLOURS = {
  character: 'var(--indigo)', safety: 'var(--red)', context: 'var(--teal)', history: 'var(--gray)', goal: 'var(--orange)',
  instructions: 'var(--pink)', stay: 'var(--indigo)', language: 'var(--mint)', guidance: 'var(--yellow)', conversation: 'var(--blue)',
};
const partColour = (s) => PART_COLOURS[s.group || s.key] || 'var(--gray)';

/** The prompt's parts as a stacked bar plus one row each (context parts grouped). */
function promptParts(d, name, st, onUpdate) {
  const sections = d.sections && d.sections.length ? d.sections : [{ key: 'system', label: 'System prompt', text: d.system }];
  const convo = d.messages.reduce((n, m) => n + m.content.length, 0);
  const total = Math.max(1, d.chars);
  // The bar: the chat-context parts count as one block.
  const blocks = [];
  for (const s of sections) {
    const key = s.group || s.key;
    const last = blocks[blocks.length - 1];
    if (s.group && last && last.key === key) last.n += s.text.length;
    else blocks.push({ key, label: s.group === 'context' ? 'Chat context' : s.label, n: s.text.length, c: partColour(s) });
  }
  blocks.push({ key: 'conversation', label: 'Conversation', n: convo, c: PART_COLOURS.conversation });
  const open = st.openParts || (st.openParts = new Set());
  const toggle = (k) => { if (open.has(k)) open.delete(k); else open.add(k); onUpdate(); };
  const row = (key, label, chars, c, body, indent = false) => html`<div class=${'ctx-sec' + (indent ? ' indent' : '') + (open.has(key) ? ' open' : '')}>
    <button type="button" class="ctx-sec-h" aria-expanded=${String(open.has(key))} @click=${() => toggle(key)}>
      <span class="ctx-swatch" style=${`--c:${c}`}></span><span class="grow">${label}</span>
      <span class="faint">${chars.toLocaleString()} chars · ${Math.round((chars / total) * 100)}%</span>${icon('chevron-down', 'ic-sm')}
    </button>
    ${open.has(key) ? body : ''}
  </div>`;
  let ctxShown = false;
  return html`
    <div class="ctx-bar" role="img" aria-label="How the prompt is split">
      ${blocks.map((b) => html`<span style=${`--c:${b.c};flex:${Math.max(b.n, 1)}`} title=${`${b.label}: ${b.n.toLocaleString()} characters`}></span>`)}
    </div>
    <div class="ctx-legend">${blocks.map((b) => html`<span><span class="ctx-swatch" style=${`--c:${b.c}`}></span>${b.label}</span>`)}</div>
    <div class="ctx-secs">
      ${sections.map((s) => {
        if (s.group === 'context') {
          const head = ctxShown ? '' : html`<div class="ctx-group-h"><span class="ctx-swatch" style=${`--c:${PART_COLOURS.context}`}></span>Chat context <span class="faint">· who is here, notes, memories, time</span></div>`;
          ctxShown = true;
          return html`${head}${row(s.key, s.label, s.text.length, partColour(s), html`<pre class="ctx-pre">${s.text.trim()}</pre>`, true)}`;
        }
        return row(s.key, s.label, s.text.length, partColour(s), html`<pre class="ctx-pre">${s.text.trim()}</pre>`);
      })}
      ${row('conversation', `Conversation (${d.messages.length} messages)`, convo, PART_COLOURS.conversation, d.messages.length
        ? html`${d.messages.map((m) => html`<div class=${'ctx-msg ' + m.role}><span class="ctx-role">${roleLabel(m, name)}</span><pre class="ctx-pre">${m.content}</pre></div>`)}`
        : html`<div class="small muted">No messages yet.</div>`)}
    </div>`;
}

function roleLabel(m, name) {
  return m.role === 'assistant' ? name : 'Chat';
}

export function contextBlock({ ctx: cs, chat: c, persona: p, onUpdate }) {
  const st = cs.st;
  const name = p ? p.name : 'The persona';
  const d = st.data;
  if (!d) {
    return html`<section class="drawer-section" data-key="context">
      <h4>${icon('text', 'ic-sm')}What ${name} reads</h4>
      ${st.error ? html`<div class="banner danger small">${icon('warning')}<div>${st.error} <button class="link-btn" @click=${cs.load}>Try again</button></div></div>`
        : html`<div class="skeleton" style="height:64px;border-radius:16px"></div>`}
    </section>`;
  }
  const instr = st.instr != null ? st.instr : (d.instructions || '');
  const sm = d.summary;
  const summary = st.summary != null ? st.summary : (sm ? sm.text : '');
  const fromNote = d.contextFrom
    ? html`It reads messages from <b>${relTime(d.contextFrom)}</b> on${sm ? ', plus the summary of what came before' : ''}.`
    : html`It reads the last <b>${d.window}</b> messages.`;
  return html`<section class="drawer-section" data-key="context">
    <h4>${icon('text', 'ic-sm')}What ${name} reads ${helpTip('context')}<span class="grow"></span>
      <button class="btn btn-ghost btn-sm" title="Refresh" @click=${cs.load}>${icon('refresh')}</button>
    </h4>
    <div class="ctx-stats">
      <span><b>${d.historyKept}</b> of ${d.historyTotal} messages</span>
      <span title="The whole prompt, about 4 characters per token">~<b>${tokens(d.chars).toLocaleString()}</b> tokens</span>
      ${sm ? html`<span>summary of ${sm.messages} older</span>` : ''}
    </div>
    <div class="field-help mt-4">${fromNote}</div>

    <div class="field-row field-row-stack mt-12">
      <div><div class="field-label">${icon('edit', 'ic-sm')}Your instructions for this chat</div>
        <div class="field-help">Added to its prompt here only, e.g. "keep it about the trip" or "don't bring up work". Who it is stays in the persona: <a href=${'#/persona/' + encodeURIComponent(c.personaId)}>edit ${name}</a>.</div></div>
      <textarea class="textarea" rows=${rowsFor(instr, 2, 8)} maxlength=${MAX_INSTRUCTIONS} placeholder="Nothing extra" .value=${instr}
        aria-label="Instructions for this chat" @input=${(e) => cs.typeInstructions(e.target.value)}></textarea>
      <div class="tiny faint">${instr.length}/${MAX_INSTRUCTIONS}${st.instr != null ? ' · saving…' : ''}</div>
    </div>

    <div class="field-row field-row-stack">
      <div><div class="field-label">${icon('brain', 'ic-sm')}Summary of older messages</div>
        <div class="field-help">${sm
          ? html`Covers ${sm.messages} messages up to ${relTime(sm.through)}${sm.edited ? ' · edited by you' : ''}. Fix anything it got wrong; it keeps building on your version.`
          : 'None yet. Summarising folds older messages into a short note it reads instead, so it remembers more with a shorter prompt.'}</div></div>
      ${sm ? html`<textarea class="textarea" rows=${rowsFor(summary, 3, 10)} maxlength=${MAX_SUMMARY} .value=${summary}
        aria-label="Summary of older messages" @input=${(e) => cs.typeSummary(e.target.value)}></textarea>` : ''}
      <div class="row gap-8 row-wrap">
        <button class=${'btn btn-glass btn-sm ' + (st.compacting ? 'loading' : '')} ?disabled=${st.compacting} @click=${cs.compact}
          title="Summarise everything but the last 8 messages now">${icon('wand')}Summarise now</button>
        ${sm ? html`<button class="btn btn-ghost btn-sm" @click=${cs.forgetSummary}>${icon('trash')}Forget summary</button>` : ''}
      </div>
    </div>

    <div class="field-row">
      <div class="grow"><div class="field-label">Summarise automatically</div>
        <div class="field-help">When its history is full, the older half is folded into the summary instead of being dropped.</div></div>
      ${toggle(!!d.autoCompact, (v) => cs.setAuto(v), { label: 'Summarise automatically' })}
    </div>

    <div class="row gap-8 row-wrap mt-8">
      <button class="btn btn-ghost btn-sm" title="It forgets everything said so far in this chat (the messages stay)"
        @click=${async () => { if (await confirmSheet({ title: 'Start fresh?', body: `${name} stops reading the messages so far and the summary. Nothing is deleted.`, confirm: 'Start fresh' })) cs.setFrom('now', 'It starts fresh from here'); }}>${icon('reset')}Start fresh from now</button>
      ${d.contextFrom ? html`<button class="btn btn-ghost btn-sm" @click=${() => cs.setFrom(null, 'It reads the full history again')}>${icon('chats')}Read all history again</button>` : ''}
      <span class="grow"></span>
      <button class="btn btn-ghost btn-sm" aria-expanded=${String(st.showPrompt)} @click=${() => { st.showPrompt = !st.showPrompt; onUpdate(); }}>
        ${icon('eye')}${st.showPrompt ? 'Hide' : 'Show'} the prompt, part by part</button>
    </div>
    ${st.showPrompt ? html`<div class="ctx-full">
      ${promptParts(d, name, st, onUpdate)}
      <div class="tiny faint">Tap a part to read it. This is exactly what goes to the AI for the next reply, apart from the private planning step for goals.</div>
    </div>` : ''}
  </section>`;
}
