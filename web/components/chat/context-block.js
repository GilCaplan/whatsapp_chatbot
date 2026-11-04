// context-block.js — the Memory tab's "What it reads" block: the full
// context of the persona's next reply in a chat (system prompt + messages,
// GET /api/chats/{key}/context), your instructions for the chat, the summary
// of older messages (edit, summarise now, forget, auto-compact) and where
// its history starts ("Start fresh", "Read all history again").

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { toast, confirmSheet } from '../../ui.js';
import { debounce, rowsFor, relTime } from '../../util.js';
import { helpTip } from '../common/help-tip.js';
import { patchChat } from '../drawer/shared.js';

const MAX_INSTRUCTIONS = 1000;
const MAX_SUMMARY = 4000;

/** ~4 characters per token: rough, but enough to compare. */
const tokens = (chars) => Math.round(chars / 4);

export function createContextStore(key, update) {
  const st = {
    data: null, error: '', showPrompt: true, compacting: false,
    openParts: new Set(['character']),
    instr: null, // instructions being typed
    summary: null, // summary being typed
    editing: '', draft: '', // prompt part being edited for this chat
  };
  async function load() {
    try {
      const [data, sys] = await Promise.all([api.chats.context(key, { quiet: true }), api.context.systems({ quiet: true }).catch(() => null)]);
      st.data = data;
      if (sys) st.systems = sys.items;
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

  /** Long-term memory for this chat: 0 automatic, -1 off, N every N messages. */
  async function setCompact(v) {
    if (st.data) st.data.compact = { ...st.data.compact, setting: v };
    update();
    try { await patchChat(key, { compactEvery: v }); } catch { /* toasted */ }
    load();
  }

  /** This chat's context system ("" = the persona's, else the app default). */
  async function setSystem(id) {
    try { await patchChat(key, { contextSystem: id }); toast('Context system changed for this chat', { type: 'success' }); } catch { /* toasted */ }
    load();
  }

  /** Replace one part of the prompt in this chat only (text null = back to the original). */
  async function saveEdit(part, text) {
    try {
      await patchChat(key, { promptEdits: { [part]: text } });
      toast(text ? 'Changed for this chat only' : 'Back to the original', { type: 'success' });
      st.editing = ''; st.draft = '';
    } catch { /* toasted */ }
    load();
  }

  return {
    st, load, reloadSoon, compact, forgetSummary, setFrom, setCompact, setSystem, saveEdit,
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

// Where the parts that can't be edited here come from.
const WHERE = {
  'context-open': 'Fixed wording that frames the chat context as information, not instructions.',
  'context-close': 'Fixed wording that closes the chat context.',
  people: 'Built for every reply from who is here and your notes in the People tab.',
  summary: 'Edit it in "Summary of older messages" below.',
  memories: 'Built for every reply from what it remembers (further down this tab).',
  cross: 'Built from what it knows about these people from its other chats.',
  world: "Built for every reply from the persona's time zone and daily routine.",
  late: 'Added when it answers long after a message.',
  goal: 'Change it in the Goal tab.',
  instructions: 'Edit it in "Your instructions for this chat" below.',
  language: 'Change it in the Behaviour tab › Languages.',
  guidance: "Built for every reply from the persona's message length and emoji settings.",
};

/** The prompt's parts as a stacked bar plus one row each (context parts grouped). */
function promptParts(cs, d, name, onUpdate) {
  const st = cs.st;
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
  const row = (key, label, chars, c, body, { indent = false, edited = false } = {}) => html`<div class=${'ctx-sec' + (indent ? ' indent' : '') + (open.has(key) ? ' open' : '') + (edited ? ' edited' : '')}>
    <button type="button" class="ctx-sec-h" aria-expanded=${String(open.has(key))} @click=${() => toggle(key)}>
      <span class="ctx-swatch" style=${`--c:${c}`}></span><span class="grow">${label}</span>
      ${edited ? html`<span class="chip chip-sm ctx-edited">Edited for this chat</span>` : ''}
      <span class="faint">${chars.toLocaleString()} chars · ${Math.round((chars / total) * 100)}%</span>${icon('chevron-down', 'ic-sm')}
    </button>
    ${open.has(key) ? body : ''}
  </div>`;
  const partBody = (s) => {
    if (st.editing === s.key) {
      return html`<div class="ctx-edit">
        <textarea class="textarea" rows=${rowsFor(st.draft, 6, 18)} maxlength="8000" .value=${st.draft}
          aria-label=${s.label + ' for this chat'} @input=${(e) => { st.draft = e.target.value; onUpdate(); }}></textarea>
        <div class="row gap-8 row-wrap mt-4">
          <button class="btn btn-primary btn-sm" ?disabled=${!st.draft.trim()} @click=${() => cs.saveEdit(s.key, st.draft.trim())}>${icon('check')}Save for this chat</button>
          <button class="btn btn-ghost btn-sm" @click=${() => { st.editing = ''; onUpdate(); }}>Cancel</button>
          <span class="tiny faint">Only this chat changes. ${name}'s persona stays as it is.</span>
        </div>
      </div>`;
    }
    return html`<pre class="ctx-pre">${s.text.trim()}</pre>
      <div class="ctx-sec-foot">${s.editable ? html`
        <button class="btn btn-ghost btn-sm" @click=${() => { st.editing = s.key; st.draft = s.text.trim(); onUpdate(); }}>${icon('edit')}Edit for this chat</button>
        ${s.edited ? html`<button class="btn btn-ghost btn-sm" title=${(s.original || '').trim().slice(0, 300)} @click=${() => cs.saveEdit(s.key, null)}>${icon('reset')}Back to the original</button>` : ''}`
        : html`<span class="tiny faint">${WHERE[s.key] || 'Built for every reply.'}</span>`}</div>`;
  };
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
          return html`${head}${row(s.key, s.label, s.text.length, partColour(s), partBody(s), { indent: true })}`;
        }
        return row(s.key, s.label, s.text.length, partColour(s), partBody(s), { edited: !!s.edited });
      })}
      ${row('conversation', `Conversation (${d.messages.length} messages)`, convo, PART_COLOURS.conversation, d.messages.length
        ? html`${d.messages.map((m) => html`<div class=${'ctx-msg ' + m.role}><span class="ctx-role">${roleLabel(m, name)}</span><pre class="ctx-pre">${m.content}</pre></div>`)}`
        : html`<div class="small muted">No messages yet.</div>`)}
    </div>`;
}

const COMPACT_CHOICES = [[0, 'Automatic'], [50, 'Every 50'], [100, 'Every 100'], [150, 'Every 150'], [200, 'Every 200'], [300, 'Every 300'], [-1, 'Off']];

/** Long-term memory: one choice per chat and what it means right now. */
function compactRow(cs, d) {
  const k = d.compact || {};
  const now = !k.on ? 'Off: older messages simply drop out of what it reads.'
    : `It reads every message since the last summary, at least ${k.keep}. After ${k.nextIn} more, all but the last ${k.keep} fold into the summary${k.setting === 0 ? ` (every ${k.every}${k.fast ? ': a busy chat right now' : ''})` : ''}.`;
  return html`<div class="field-row">
    <div class="grow"><div class="field-label">Long-term memory</div>
      <div class="field-help">${now}</div></div>
    <select class="select select-sm" aria-label="Long-term memory" @change=${(e) => cs.setCompact(Number(e.target.value))}>
      ${COMPACT_CHOICES.map(([v, l]) => html`<option value=${v} ?selected=${(k.setting || 0) === v}>${l}</option>`)}
    </select>
  </div>`;
}

/** Which context system the chat runs (Context page); "" = inherit. */
function systemRow(cs, d, chat) {
  const ch = d.contextSystem || {};
  const own = chat && chat.contextSystem;
  const from = ch.source === 'persona' ? "the persona's" : 'the app default';
  return html`<div class="field-row">
    <div class="grow"><div class="field-label row gap-4">Context system ${helpTip('context')}</div>
      <div class="field-help">${own ? `${d.contextSystemName}, chosen for this chat.` : `${d.contextSystemName || 'Classic'}, ${from}.`} <a href="#/context">See how it works</a></div></div>
    <select class="select select-sm" aria-label="Context system" @change=${(e) => cs.setSystem(e.target.value)}>
      <option value="" ?selected=${!own}>Default</option>
      ${(cs.st.systems || []).map((x) => html`<option value=${x.id} ?selected=${own === x.id}>${x.name}</option>`)}
    </select>
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

    <div class="row gap-8 mt-8">
      <div class="field-label grow">The prompt, part by part</div>
      <button class="btn btn-ghost btn-sm" aria-expanded=${String(st.showPrompt)} @click=${() => { st.showPrompt = !st.showPrompt; onUpdate(); }}>
        ${icon('eye')}${st.showPrompt ? 'Hide' : 'Show'}</button>
    </div>
    ${st.showPrompt ? html`<div class="ctx-full">
      ${promptParts(cs, d, name, onUpdate)}
      <div class="tiny faint">Tap a part to read it. Character, safety rules, how to read the conversation and "stay yourself" can be edited for this chat only. This is exactly what the AI gets for the next reply, apart from the private planning step for goals.</div>
    </div>` : ''}

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

    ${systemRow(cs, d, c)}
    ${compactRow(cs, d)}

    <div class="row gap-8 row-wrap mt-8">
      <button class="btn btn-ghost btn-sm" title="It forgets everything said so far in this chat (the messages stay)"
        @click=${async () => { if (await confirmSheet({ title: 'Start fresh?', body: `${name} stops reading the messages so far and the summary. Nothing is deleted.`, confirm: 'Start fresh' })) cs.setFrom('now', 'It starts fresh from here'); }}>${icon('reset')}Start fresh from now</button>
      ${d.contextFrom ? html`<button class="btn btn-ghost btn-sm" @click=${() => cs.setFrom(null, 'It reads the full history again')}>${icon('chats')}Read all history again</button>` : ''}
    </div>
  </section>`;
}
