// pages/context.js — Context: how personas decide when to use the AI and
// what they read. Three built-in systems (Classic, Lean, Fast; locked) and
// yours: pick one, see its flowchart, tune its settings, and choose which
// chats and personas run it (chat → persona → app default). Your own
// systems have an editable flowchart (<cs-flowchart>): add blocks from the
// palette, drag, wire, label, note, delete; "Match settings to chart" reads
// the settings off the drawing. Also: the sandbox (context-sandbox.js),
// "Describe it" (context-draft-sheet.js) and the persona cards.
// Data: /api/context/* (docs/API.md › Context systems).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { bus, store, reloadPersonasSoon } from '../store.js';
import { confirmSheet, toast, seg, spinner } from '../ui.js';
import { debounce, plural, relTime } from '../util.js';
import { helpTip } from '../components/common/help-tip.js';
import { paramsForm, paramRows } from '../components/context/context-params.js';
import { createSandbox } from '../components/context/context-sandbox.js';
import { openDraftSheet } from '../components/context/context-draft-sheet.js';
import '../components/context/flowchart.js';

const STAGE_LABELS = {
  arrive: 'Arrives', gate: 'Rules', join: 'Joining in', goal: 'Goals', prompt: 'What it reads',
  write: 'Writing', check: 'Checks', out: 'Out', background: 'Background', custom: 'Your own',
};
const KIND_LABEL = { ai: 'AI call', code: 'Decided in code', data: 'Part of the prompt', io: 'In or out', note: 'Your own' };
const clone = (o) => JSON.parse(JSON.stringify(o));

export default function Context(ctx) {
  const st = {
    catalog: null, systems: null, appDefault: 'classic', instances: [], error: '',
    sel: '', tab: 'chart', edit: null, key: 0, fcSel: null, saving: false, savedAt: 0,
  };
  const offs = [];
  const sandbox = createSandbox(() => ctx.update());
  const fc = () => document.querySelector('cs-flowchart.cs-fc');

  async function load() {
    try {
      const [cat, sys, inst] = await Promise.all([
        st.catalog ? Promise.resolve(st.catalog) : api.context.catalog({ quiet: true }),
        api.context.systems({ quiet: true }), api.context.instances({ quiet: true }),
      ]);
      st.catalog = cat; st.systems = sys.items; st.appDefault = sys.appDefault; st.instances = inst.items; st.error = '';
      if (!st.systems.some((s) => s.id === st.sel)) pick(st.appDefault);
      else if (!st.edit || st.edit.id !== st.sel) pick(st.sel);
    } catch (e) { if (!st.systems) st.error = (e && e.message) || 'Could not load context systems'; }
    ctx.update();
  }
  const loadSoon = debounce(load, 250);

  const blocksById = () => Object.fromEntries((st.catalog ? st.catalog.blocks : []).map((b) => [b.id, b]));
  const current = () => (st.systems || []).find((s) => s.id === st.sel) || null;
  const nameOf = (id) => ((st.systems || []).find((s) => s.id === id) || { name: 'Classic' }).name;

  function pick(id) {
    const s = (st.systems || []).find((x) => x.id === id);
    if (!s) return;
    st.sel = id; st.edit = clone(s); st.key++; st.fcSel = null;
    ctx.update();
  }

  // ── Saving your own systems (debounced; the flowchart edits st.edit in place) ──
  const saveSoon = debounce(async () => {
    const e = st.edit;
    if (!e || e.builtIn) return;
    st.saving = true; ctx.update();
    try {
      const saved = await api.context.save(e.id, e, { quiet: false });
      st.savedAt = Date.now();
      const i = st.systems.findIndex((s) => s.id === saved.id);
      if (i >= 0) st.systems[i] = { ...st.systems[i], ...saved };
      loadSoon(); // fresh estimates
    } catch { /* toasted */ }
    st.saving = false; ctx.update();
  }, 700);
  const touched = () => { saveSoon(); ctx.update(); };

  function setParam(key, value) {
    if (!st.edit || st.edit.builtIn) return;
    st.edit.params = { ...st.edit.params, [key]: value };
    touched();
  }

  async function copy(id, name) {
    try {
      const saved = await api.context.create({ copyFrom: id, name: name || '' });
      toast(`Copied — you can edit “${saved.name}” now`, { type: 'success' });
      await load();
      pick(saved.id);
    } catch { /* toasted */ }
  }

  async function remove(s) {
    const ok = await confirmSheet({ title: `Delete “${s.name}”?`, body: s.chats || s.personas
      ? `${plural(s.chats, 'chat')} and ${plural(s.personas, 'persona')} use it; they go back to their default.` : 'Nothing uses it right now.',
    confirm: 'Delete', danger: true, iconName: 'trash' });
    if (!ok) return;
    try { await api.context.remove(s.id); toast('Deleted', { type: 'success' }); st.sel = st.appDefault; st.edit = null; load(); } catch { /* toasted */ }
  }

  async function tidy() {
    try {
      st.edit.chart = await api.context.layout(st.edit.chart);
      st.key++; st.fcSel = null; touched();
    } catch { /* toasted */ }
  }

  async function matchSettings() {
    try {
      const r = await api.context.infer({ chart: st.edit.chart, params: st.edit.params, base: st.edit.base });
      if (!r.changes.length) { toast('The settings already match the chart'); return; }
      const ok = await confirmSheet({ title: 'Match the settings to the chart?', body: r.changes.join(' · '), confirm: 'Change settings', iconName: 'sliders' });
      if (!ok) return;
      st.edit.params = r.params; touched();
      toast('Settings changed', { type: 'success' });
    } catch { /* toasted */ }
  }

  async function setApp(id) {
    try { await api.settings.put({ context: { system: id } }); toast(`App default: ${nameOf(id)}`, { type: 'success' }); load(); } catch { /* toasted */ }
  }
  async function setChat(key, id) {
    try { await api.chats.patch(key, { contextSystem: id }); load(); } catch { /* toasted */ }
  }
  async function setPersona(pid, id) {
    try { await api.context.setPersona(pid, id); load(); } catch { /* toasted */ }
  }

  // ── Views ──
  function systemCard(s) {
    const on = s.id === st.sel;
    return html`<button type="button" class=${'card cs-sys ' + (on ? 'cs-sys-on' : '')} aria-pressed=${String(on)} data-key=${s.id} @click=${() => pick(s.id)}>
      <div class="cs-sys-top"><strong>${s.name}</strong>
        ${s.builtIn ? html`<span class="badge badge-soft cs-badge">${icon('lock', 'ic-xs')}Built in</span>` : html`<span class="badge cs-badge badge-accent">Yours</span>`}
        ${s.id === st.appDefault ? html`<span class="badge cs-badge badge-green">App default</span>` : ''}</div>
      <p class="muted small cs-sys-desc">${s.description || 'Your own context system.'}</p>
      <div class="cs-sys-meta small">
        <span title="AI calls one after another before a group reply with a goal (an estimate)">${icon('bolt', 'ic-xs')}~${s.estimate.perAnswer} calls per reply</span>
        <span>${icon('chats', 'ic-xs')}${plural(s.chats, 'chat')}</span>
      </div>
    </button>`;
  }

  function palette(blocks) {
    const byStage = {};
    for (const b of blocks) (byStage[b.stage] = byStage[b.stage] || []).push(b);
    return html`<aside class="cs-palette" aria-label="Building blocks">
      <div class="faint small cs-pal-hint">Click a block to add it</div>
      ${st.catalog.stages.filter((s) => byStage[s]).map((s) => html`<div class="cs-pal-group" data-key=${s}>
        <div class="cs-pal-title">${STAGE_LABELS[s] || s}</div>
        ${byStage[s].map((b) => html`<button type="button" class=${'cs-pal-item fc-' + b.kind} title=${b.description}
          @click=${() => fc()?.addBlock(b)}><i></i>${b.label}</button>`)}
      </div>`)}
    </aside>`;
  }

  function inspector(locked) {
    const sel = st.fcSel, chart = st.edit.chart, blocks = blocksById();
    if (!sel) {
      return html`<aside class="cs-insp"><div class="faint small">${locked
        ? 'Select a block to see what it does. Copy the system to change it.'
        : 'Select a block or a wire to edit it. Drag blocks to move them, drag from the dot under a block to wire it to another, press Delete to remove.'}</div>
        <div class="cs-legend-kinds small">${Object.entries(KIND_LABEL).map(([k, l]) => html`<span><i class=${'fc-' + k}></i>${l}</span>`)}</div></aside>`;
    }
    if (sel.kind === 'edge') {
      const e = chart.edges[sel.index];
      if (!e) return html`<aside class="cs-insp"></aside>`;
      const a = chart.nodes.find((n) => n.id === e.from), b = chart.nodes.find((n) => n.id === e.to);
      return html`<aside class="cs-insp">
        <div class="cs-insp-kind small faint">Wire</div>
        <div class="small">${a ? a.label : e.from} ${icon('arrow-right', 'ic-xs')} ${b ? b.label : e.to}</div>
        <label class="field"><span class="field-label">Label</span><input class="input input-sm" maxlength="30" ?disabled=${locked} .value=${e.label || ''}
          placeholder="yes, no, unsure…" @input=${(ev) => { e.label = ev.target.value; fc()?.redraw(); touched(); }}></label>
        <div class="field-row"><div class="grow"><div class="field-label">Feeds in the background</div><div class="field-help">Dashed: a prompt part or a background job.</div></div>
          <label class="switch"><input type="checkbox" role="switch" ?checked=${!!e.dashed} ?disabled=${locked}
            @change=${(ev) => { e.dashed = ev.target.checked; fc()?.redraw(); touched(); }}><span class="track"></span><span class="thumb"></span></label></div>
        ${locked ? '' : html`<button class="btn btn-glass btn-sm" @click=${() => fc()?.removeSelected()}>${icon('trash')}Remove wire</button>`}
      </aside>`;
    }
    const n = chart.nodes.find((x) => x.id === sel.id);
    if (!n) return html`<aside class="cs-insp"></aside>`;
    const b = blocks[n.block] || {};
    const specs = st.catalog.params.filter((p) => (b.params || []).includes(p.key));
    return html`<aside class="cs-insp">
      <div class=${'cs-insp-kind small fc-' + b.kind}><i></i>${KIND_LABEL[b.kind] || ''}</div>
      <p class="small muted">${b.description}</p>
      <label class="field"><span class="field-label">Label</span><input class="input input-sm" maxlength="60" ?disabled=${locked} .value=${n.label || ''}
        @input=${(ev) => { n.label = ev.target.value; fc()?.redraw(); touched(); }}></label>
      ${locked ? '' : html`<label class="field"><span class="field-label">Block</span>
        <select class="select select-sm" @change=${(ev) => { n.block = ev.target.value; fc()?.redraw(); touched(); }}>
          ${st.catalog.blocks.map((x) => html`<option value=${x.id} ?selected=${x.id === n.block}>${STAGE_LABELS[x.stage]} · ${x.label}</option>`)}
        </select></label>`}
      <label class="field"><span class="field-label">Note</span><textarea class="input" rows="3" maxlength="300" ?disabled=${locked} .value=${n.note || ''}
        placeholder="Why it's here, what to try…" @input=${(ev) => { n.note = ev.target.value; fc()?.redraw(); touched(); }}></textarea></label>
      ${specs.length ? html`<div class="cs-insp-params"><div class="field-label">Settings it stands for</div>
        ${paramRows(specs, st.edit.params, setParam, { disabled: locked })}</div>` : ''}
      ${locked ? '' : html`<button class="btn btn-glass btn-sm" @click=${() => fc()?.removeSelected()}>${icon('trash')}Remove block</button>`}
    </aside>`;
  }

  function chartTab(s) {
    const locked = !!s.builtIn;
    return html`<div class="cs-chart-wrap">
      <div class="cs-toolbar">
        ${locked ? html`<span class="small muted row gap-4">${icon('lock', 'ic-xs')} Built-in charts are drawn from the code and can't be changed.</span>
            <button class="btn btn-primary btn-sm" @click=${() => copy(s.id)}>${icon('copy')}Copy to edit</button>`
          : html`<button class="btn btn-glass btn-sm" @click=${tidy} title="Lay the blocks out again">${icon('layout')}Tidy up</button>
            <button class="btn btn-glass btn-sm" @click=${matchSettings} title="Read the settings off the blocks in the chart">${icon('sliders')}Match settings to chart</button>
            <span class="faint small">${st.saving ? html`${spinner('spinner-sm')} Saving…` : st.savedAt ? 'Saved' : ''}</span>`}
        <span class="grow"></span>
        <button class="btn btn-ghost btn-icon btn-sm" aria-label="Zoom out" @click=${() => fc()?.zoom(1 / 1.2)}>${icon('minus')}</button>
        <button class="btn btn-ghost btn-icon btn-sm" aria-label="Zoom in" @click=${() => fc()?.zoom(1.2)}>${icon('plus')}</button>
        <button class="btn btn-ghost btn-sm" @click=${() => fc()?.fit()}>Fit</button>
      </div>
      <div class=${'cs-editor ' + (locked ? 'cs-editor-locked' : '')}>
        ${locked ? '' : palette(st.catalog.blocks)}
        <cs-flowchart class="cs-fc" .data=${{ chart: st.edit.chart, blocks: blocksById(), locked, selected: st.fcSel, key: st.sel + ':' + st.key }}
          @fc-select=${(e) => { st.fcSel = e.detail; ctx.update(); }}
          @fc-change=${() => touched()}></cs-flowchart>
        ${inspector(locked)}
      </div>
      ${locked ? '' : html`<p class="faint small">The chart is your design: only the settings change how chats run. Blocks for real settings edit them from the panel on the right.</p>`}
    </div>`;
  }

  function settingsTab(s) {
    const locked = !!s.builtIn;
    return html`<div class="cs-settings">
      ${locked ? html`<div class="banner info small">${icon('lock', 'ic-sm')}<span>Built-in settings are fixed. Copy this system to tune it.</span>
          <button class="btn btn-glass btn-sm" @click=${() => copy(s.id)}>${icon('copy')}Copy to edit</button></div>`
        : html`<div class="cs-name-row">
          <label class="field grow"><span class="field-label">Name</span><input class="input" maxlength="60" .value=${st.edit.name}
            @input=${(e) => { st.edit.name = e.target.value; touched(); }}></label>
          <label class="field grow"><span class="field-label">Description</span><input class="input" maxlength="400" .value=${st.edit.description || ''}
            @input=${(e) => { st.edit.description = e.target.value; touched(); }}></label>
        </div>`}
      ${paramsForm(st.catalog.params, st.edit.params, setParam, { disabled: locked })}
      ${locked ? '' : html`<div class="row gap-8 mt-12"><span class="faint small grow">Made from ${nameOf(s.base)}${s.updatedAt ? ` · changed ${relTime(s.updatedAt)}` : ''}</span>
        <button class="btn btn-danger btn-sm" @click=${() => remove(s)}>${icon('trash')}Delete system</button></div>`}
    </div>`;
  }

  function sysSelect(value, onChange, inheritLabel, label) {
    return html`<select class="select select-sm" aria-label=${label} @change=${(e) => onChange(e.target.value)}>
      <option value="" ?selected=${!value}>${inheritLabel}</option>
      ${(st.systems || []).map((s) => html`<option value=${s.id} ?selected=${s.id === value}>${s.name}</option>`)}
    </select>`;
  }

  function usedByTab() {
    const personas = store.state.personas;
    const pSys = (id) => (personas.find((p) => p.id === id) || {}).contextSystem || '';
    return html`<div class="cs-used">
      <div class="field-row"><div class="grow"><div class="field-label">App default</div><div class="field-help">Every chat whose persona and chat don't pick one.</div></div>
        <select class="select select-sm" aria-label="App default system" @change=${(e) => setApp(e.target.value)}>
          ${(st.systems || []).map((s) => html`<option value=${s.id} ?selected=${s.id === st.appDefault}>${s.name}</option>`)}
        </select></div>
      <h4 class="cs-pgroup-title">Personas</h4>
      ${personas.map((p) => html`<div class="field-row" data-key=${'p-' + p.id}><div class="grow"><div class="field-label">${p.name}</div></div>
        ${sysSelect(p.contextSystem || '', (v) => setPersona(p.id, v), `App default (${nameOf(st.appDefault)})`, `${p.name}'s system`)}</div>`)}
      <h4 class="cs-pgroup-title">Chats</h4>
      ${st.instances.length ? st.instances.map((i) => {
        const inherit = pSys(i.personaId) ? `${i.personaName}'s (${nameOf(pSys(i.personaId))})` : `App default (${nameOf(st.appDefault)})`;
        return html`<div class="field-row cs-inst" data-key=${i.chatKey}>
          <div class="grow"><div class="field-label">${icon(i.kind === 'group' ? 'group' : 'user', 'ic-xs')} ${i.name || i.chatKey}
            ${i.enabled ? '' : html`<span class="badge badge-soft cs-badge">Off</span>`}</div>
            <div class="field-help">${i.personaName || 'No persona'} · runs ${i.systemName}${i.choice.source !== 'chat' ? ` (from the ${i.choice.source === 'persona' ? 'persona' : 'app default'})` : ''}</div></div>
          ${sysSelect(i.chatSystem, (v) => setChat(i.chatKey, v), inherit, `${i.name}'s system`)}</div>`;
      }) : html`<p class="muted small">No chats assigned yet.</p>`}
    </div>`;
  }

  function cardsSection() {
    const personas = store.state.personas;
    return html`<section class="card cs-cards">
      <div class="cs-sec-head"><h3>${icon('text', 'ic-sm')}Persona cards ${helpTip('cs-card')}</h3>
        <span class="faint small">Short versions of each persona for systems with "Short persona card".</span></div>
      ${personas.map((p) => html`<div class="cs-card-row" data-key=${p.id}>
        <div class="cs-card-who"><strong>${p.name}</strong><span class="faint small">${p.card ? `${p.card.length} characters${p.cardAt ? ' · ' + relTime(p.cardAt) : ''}` : 'Not made yet: it is written the first time it is needed'}</span></div>
        <textarea class="input" rows="3" maxlength="2000" .value=${p.card || ''} placeholder="Written by the AI when first needed, or write your own."
          @change=${async (e) => { try { await api.context.saveCard(p.id, e.target.value); toast('Card saved', { type: 'success' }); reloadPersonasSoon(); } catch { /* toasted */ } }}></textarea>
        <button class="btn btn-glass btn-sm" @click=${async (e) => {
          const b = e.currentTarget; b.classList.add('loading');
          try { await api.context.makeCard(p.id); toast(`New card for ${p.name}`, { type: 'success' }); reloadPersonasSoon(); } catch { /* toasted */ }
          b.classList.remove('loading');
        }}>${icon('sparkles')}${p.card ? 'Write again' : 'Write now'}</button>
      </div>`)}
    </section>`;
  }

  function view() {
    if (st.error) return html`<div class="card"><p>${st.error}</p><button class="btn btn-glass" @click=${load}>${icon('refresh')}Try again</button></div>`;
    if (!st.systems || !st.catalog || !st.edit) return html`<div class="card skeleton" style="height:420px"></div>`;
    const s = current() || st.edit;
    const mine = st.systems.filter((x) => !x.builtIn);
    return html`<div class="cs-page">
      <section class="cs-hero">
        <div class="grow">
          <h2 class="hero-title">How your personas think</h2>
          <p class="muted">A context system decides when a persona asks the AI and what it reads before it writes. Fewer calls reply faster; checks in code catch nonsense. ${helpTip('context')}</p>
        </div>
        <div class="row row-wrap gap-8">
          <button class="btn btn-glass" @click=${() => openDraftSheet({ blocks: blocksById(), onSaved: async (x) => { await load(); pick(x.id); } })}>${icon('wand')}Describe one</button>
          <button class="btn btn-primary" @click=${() => copy(st.sel)}>${icon('copy')}Copy “${s.name}”</button>
        </div>
      </section>

      <div class="cs-systems">${st.systems.filter((x) => x.builtIn).map(systemCard)}${mine.map(systemCard)}</div>

      <section class="card cs-detail">
        <div class="cs-detail-head">
          <h3>${s.name}</h3>
          ${seg([{ value: 'chart', label: 'Flowchart', icon: 'flow' }, { value: 'settings', label: 'Settings', icon: 'sliders' }, { value: 'used', label: 'Who uses it', icon: 'chats' }],
            st.tab, (v) => { st.tab = v; ctx.update(); }, { label: 'View' })}
        </div>
        ${st.tab === 'chart' ? chartTab(s) : st.tab === 'settings' ? settingsTab(s) : usedByTab()}
      </section>

      ${sandbox.view(st.systems)}
      ${cardsSection()}
    </div>`;
  }

  return {
    title: 'Context',
    subtitle: () => (st.systems ? `${plural(st.systems.length, 'system')} · app default ${nameOf(st.appDefault)}` : ''),
    view,
    mount() {
      load();
      offs.push(
        bus.on('context.changed', (d) => { if (d && d.replayId) sandbox.onEvent(d); else loadSoon(); }),
        bus.on('chats.changed', loadSoon), bus.on('personas.changed', loadSoon), bus.on('settings.changed', loadSoon),
      );
    },
    unmount() { for (const off of offs) off(); loadSoon.cancel(); if (saveSoon.pending()) saveSoon.flush(); sandbox.destroy(); },
  };
}
