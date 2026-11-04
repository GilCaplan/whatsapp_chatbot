// components/context/context-params.js — the knobs of a context system, drawn from
// the server's specs (GET /api/context/catalog → params): enums as selects,
// numbers with their range (0 = the spec's "zero" meaning when it has one),
// switches. A knob with showIf appears only while its parent has one of
// those values. onChange(key, value).

import { html } from '../../dom.js';
import { toggle } from '../../ui.js';
import { numberField } from '../common/controls.js';

export const PARAM_GROUPS = [
  { id: 'join', label: 'Joining in' },
  { id: 'check', label: 'Checking replies' },
  { id: 'goal', label: 'Goals' },
  { id: 'background', label: 'Background work' },
  { id: 'prompt', label: 'What it reads' },
];

const shown = (spec, params) => !spec.showIf || Object.entries(spec.showIf).every(([k, vals]) => vals.includes(params[k]));

/** One knob's control. */
export function paramControl(spec, params, onChange, { disabled = false } = {}) {
  const v = params[spec.key];
  if (spec.type === 'enum') {
    return html`<select class="select select-sm" aria-label=${spec.label} ?disabled=${disabled} @change=${(e) => onChange(spec.key, e.target.value)}>
      ${spec.options.map((o) => html`<option value=${o.value} ?selected=${o.value === v}>${o.label}</option>`)}
    </select>`;
  }
  if (spec.type === 'bool') return toggle(!!v, (on) => onChange(spec.key, on), { label: spec.label, disabled });
  const zero = spec.zero && !v;
  return html`<div class="row gap-8 cs-num">
    ${spec.zero ? html`<select class="select select-sm" aria-label=${spec.label + ' limit'} ?disabled=${disabled}
        @change=${(e) => onChange(spec.key, e.target.value === 'zero' ? 0 : (v || spec.min))}>
        <option value="zero" ?selected=${zero}>${cap(spec.zero)}</option><option value="n" ?selected=${!zero}>At most…</option>
      </select>` : ''}
    ${zero ? '' : numberField({ value: v || spec.min, min: spec.min, max: spec.max, unit: spec.unit || '', label: spec.label, width: 64, disabled,
      onChange: (n) => onChange(spec.key, n) })}
  </div>`;
}

const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);

/** One field row per knob. only: keys to show (default: all). */
export function paramRows(specs, params, onChange, { disabled = false, only = null } = {}) {
  return specs.filter((s) => (!only || only.includes(s.key)) && shown(s, params)).map((s) => html`<div class="field-row" data-key=${s.key}>
    <div class="grow"><div class="field-label">${s.label}</div>${s.help ? html`<div class="field-help">${s.help}</div>` : ''}</div>
    <div>${paramControl(s, params, onChange, { disabled })}</div>
  </div>`);
}

/** Every knob, grouped. */
export function paramsForm(specs, params, onChange, opts = {}) {
  return PARAM_GROUPS.map((g) => {
    const rows = specs.filter((s) => s.group === g.id);
    if (!rows.length) return '';
    return html`<section class="cs-pgroup" data-key=${g.id}>
      <h4 class="cs-pgroup-title">${g.label}</h4>
      ${paramRows(rows, params, onChange, opts)}
    </section>`;
  });
}
