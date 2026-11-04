// tasks/rule-editor.js — the when/then rule list of a task (Advanced). The
// first rule that matches wins. Conditions and actions come from a fixed
// list (GET /api/tasks → vocabulary), so every rule can be checked.
//
//   ruleEditor({ rules, vocab, slots, defaults, onChange(rules) })

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { condLabel, actLabel, STEPS, EFFECTS } from './task-labels.js';

export function ruleEditor({ rules, vocab, slots, defaults, onChange }) {
  const list = rules || [];
  const set = (next) => onChange(next.map((r) => ({ ...r, do: [...r.do] })));
  const edit = (i, patch) => set(list.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const move = (i, d) => {
    const j = i + d;
    if (j < 0 || j >= list.length) return;
    const next = [...list];
    [next[i], next[j]] = [next[j], next[i]];
    set(next);
  };
  const says = (r) => r.do.filter((a) => !EFFECTS.includes(a)).length;
  const conds = (vocab && vocab.conditions) || [];
  const acts = (vocab && vocab.actions) || [];

  const row = (r, i) => html`<li class="tk-rule" data-key=${'rule-' + i}>
    <span class="tk-rule-n" aria-hidden="true">${i + 1}</span>
    <div class="tk-rule-body">
      <div class="tk-rule-line">
        <span class="tk-rule-w">When</span>
        <select class="select input-sm" aria-label=${'Rule ' + (i + 1) + ': when'} @change=${(e) => edit(i, { when: e.target.value })}>
          ${conds.map((c) => html`<option value=${c} ?selected=${r.when === c}>${condLabel(c, slots)}</option>`)}
        </select>
        <select class="select input-sm" aria-label=${'Rule ' + (i + 1) + ': step'} @change=${(e) => edit(i, { step: e.target.value || undefined })}>
          ${STEPS.map((s) => html`<option value=${s.value} ?selected=${(r.step || '') === s.value}>${s.label}</option>`)}
        </select>
      </div>
      <div class="tk-rule-line">
        <span class="tk-rule-w">Then</span>
        <span class="chip-row">${r.do.map((a, k) => html`<span class=${'chip chip-sm ' + (EFFECTS.includes(a) ? 'chip-violet' : 'chip-blue')}>${actLabel(a, slots)}
          <button type="button" class="tk-chip-x" aria-label=${'Remove ' + actLabel(a, slots)} ?disabled=${r.do.length === 1}
            @click=${() => edit(i, { do: r.do.filter((_, x) => x !== k) })}>${icon('x', 'ic-xs')}</button></span>`)}</span>
        ${r.do.length < 4 ? html`<select class="select input-sm tk-add-act" aria-label=${'Rule ' + (i + 1) + ': add an action'}
            @change=${(e) => { const a = e.target.value; e.target.value = ''; if (a) edit(i, { do: [...r.do, a] }); }}>
          <option value="">Add…</option>
          ${acts.filter((a) => !r.do.includes(a) && (EFFECTS.includes(a) || says(r) === 0)).map((a) => html`<option value=${a}>${actLabel(a, slots)}</option>`)}
        </select>` : ''}
      </div>
      ${r.do.includes('commit') && r.step !== 'confirming' ? html`<div class="tiny tk-rule-warn">${icon('warning', 'ic-xs')}Confirming only works while confirming: set the step to "While confirming".</div>` : ''}
    </div>
    <div class="tk-rule-tools">
      <label class="tiny faint tk-max" title="0 = no limit">Max
        <input class="input tk-num" type="number" min="0" max="50" .value=${String(r.maxTimes || 0)} aria-label=${'Rule ' + (i + 1) + ': most times it fires'}
          @change=${(e) => edit(i, { maxTimes: Math.max(0, Math.min(50, parseInt(e.target.value, 10) || 0)) || undefined })}></label>
      <button type="button" class="btn btn-ghost btn-icon btn-xs" aria-label="Move up" ?disabled=${i === 0} @click=${() => move(i, -1)}>${icon('chevron-up', 'ic-sm')}</button>
      <button type="button" class="btn btn-ghost btn-icon btn-xs" aria-label="Move down" ?disabled=${i === list.length - 1} @click=${() => move(i, 1)}>${icon('chevron-down', 'ic-sm')}</button>
      <button type="button" class="btn btn-ghost btn-icon btn-xs" aria-label="Delete rule" @click=${() => set(list.filter((_, j) => j !== i))}>${icon('trash', 'ic-sm')}</button>
    </div>
  </li>`;

  return html`<div class="tk-rules">
    <p class="small muted">From the top down, the first rule that fits what just happened decides the next message. Code checks every time it offers; the AI only reads their messages and writes yours.</p>
    <ol class="tk-rule-list">${list.map(row)}</ol>
    <div class="row gap-8 mt-8">
      <button type="button" class="btn btn-glass btn-sm" ?disabled=${list.length >= 60} @click=${() => set([...list, { when: conds[0] || 'started', do: ['chat'] }])}>${icon('plus')}Add a rule</button>
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost btn-sm" @click=${() => set(defaults || [])}>${icon('reset')}Back to the template's rules</button>
    </div>
  </div>`;
}
