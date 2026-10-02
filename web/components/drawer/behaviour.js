// drawer/behaviour.js — the chat panel's "Behaviour" tab: preset chips, the
// reply timeline and the per-chat fine-tuning form (overrides).
//
// Also the panel's behaviour controller: the Goal tab (check-ins), the Away
// menu and the banners read ctx.behaviour.cb() and edit through it.
// Wave 3 owner: Engineer C (vibe dials, Advanced switch).

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { store, isAway, personaById } from '../../store.js';
import { toast, toggle } from '../../ui.js';
import { debounce } from '../../util.js';
import { behaviourForm } from '../behaviour-form.js';
import { presetChips } from '../preset-cards.js';
import { createTimeline } from '../reply-timeline.js';
import { ALL_FIELDS, detectPreset, overrideCount, presetStyle, profileKey, same } from '../behaviour-meta.js';
import { patchChat, awayUntilText } from './shared.js';
import { vibeDials } from '../vibe-dials.js';
import { helpTip } from '../help-tip.js';
import { routineSummary } from '../routine-editor.js';

/** Wave 3 (Engineer A): "Also follows Leo's routine: gym 07:00–08:30, …". */
function routineNote(c) {
  const p = personaById(c.personaId);
  const r = p && p.world && p.world.routine;
  if (!r || !r.length) return '';
  return html`<div class="bh-routine small muted mt-8">${icon('calendar', 'ic-sm')}<span>Also follows ${p.name}'s routine: ${routineSummary(r)}.
    <a class="link-btn" href=${'#/persona/' + encodeURIComponent(p.id)}>Edit routine</a></span></div>`;
}

export function createBehaviourTab(ctx) {
  const { key } = ctx;
  const st = {
    // GET /api/chats/{key}/behavior + optimistic override edits
    cb: null, cbError: '', cbLoading: false,
    bpend: {},
    formOpen: (() => { try { return sessionStorage.getItem('doppel.drawer.adv') === '1'; } catch { return false; } })(),
    bui: { open: new Set(['responsiveness', 'timing']), adv: false },
  };
  const timeline = createTimeline({ onUpdate: () => ctx.update() });

  async function loadBehavior() {
    st.cbLoading = true;
    try {
      const r = await api.chats.behavior(key, { quiet: true });
      st.cb = applyPending(r);
      st.cbError = '';
    } catch (e) {
      if (!st.cb) st.cbError = (e && e.message) || 'Could not load';
    }
    st.cbLoading = false;
    ctx.update();
  }
  const reloadBehaviorSoon = debounce(loadBehavior, 250);

  /** Re-apply edits that haven't reached the server yet on top of a fresh response. */
  function applyPending(r) {
    if (!r) return r;
    const cb = { ...r, effective: { ...(r.effective || {}) }, sources: { ...(r.sources || {}) }, overrides: { ...(r.overrides || {}) } };
    for (const [f, v] of Object.entries(st.bpend)) setLocal(cb, f, v);
    return cb;
  }

  function setLocal(cb, f, v) {
    if (v === null) {
      delete cb.overrides[f];
      if (f !== 'preset') { cb.effective[f] = cb.defaults ? cb.defaults[f] : cb.effective[f]; cb.sources[f] = 'default'; }
    } else {
      cb.overrides[f] = v;
      if (f !== 'preset') { cb.effective[f] = v; cb.sources[f] = 'chat'; }
    }
  }

  const presetsList = () => (store.state.behaviorMeta && store.state.behaviorMeta.presets) || [];

  /** The named preset this chat started from (overrides.preset), with its values for this kind. */
  function basePresetOf(cb) {
    const id = cb && cb.overrides && cb.overrides.preset;
    if (!id || id === 'custom') return null;
    const def = presetsList().find((p) => p.id === id);
    const values = def && def[profileKey(cb.kind)];
    return values ? { id, label: def.label || presetStyle(id).label, values } : null;
  }

  async function flushBehavior() {
    const body = { ...st.bpend };
    if (!Object.keys(body).length) return;
    const cb = st.cb;
    if (!('preset' in body) && cb) {
      // Keep a named starting preset while tweaking on top of it; otherwise label as custom.
      if (!overrideCount(cb.overrides)) body.preset = null;
      else if (!basePresetOf(cb)) body.preset = detectPreset(cb.effective, presetsList(), profileKey(cb.kind));
    }
    try {
      await patchChat(key, { behavior: body });
    } catch { /* toasted; the reload below shows the real state */ }
    for (const [f, v] of Object.entries(body)) if (same(st.bpend[f], v)) delete st.bpend[f];
    reloadBehaviorSoon();
  }
  const flushBehaviorSoon = debounce(flushBehavior, 500);

  function editOverride(f, v) {
    if (!st.cb) return;
    st.bpend[f] = v;
    setLocal(st.cb, f, v);
    ctx.update();
    flushBehaviorSoon();
  }

  /** Several fields at once (a vibe dial): optimistic, one debounced PATCH. */
  function editMany(fields) {
    if (!st.cb) return;
    const d = st.cb.defaults || {};
    for (const [f, v0] of Object.entries(fields)) {
      // A dial value equal to the defaults inherits them (keeps the chat's overrides tidy).
      const v = same(d[f], v0) ? null : v0;
      if (v === null && (st.cb.overrides || {})[f] === undefined) continue;
      st.bpend[f] = v; setLocal(st.cb, f, v);
    }
    ctx.update();
    flushBehaviorSoon();
  }

  function setAdvanced(on) {
    st.formOpen = on;
    try { sessionStorage.setItem('doppel.drawer.adv', on ? '1' : ''); } catch { /* ignore */ }
    ctx.update();
  }

  function applyOverrides(body, msg) {
    if (!st.cb) return;
    flushBehaviorSoon.cancel();
    Object.assign(st.bpend, body);
    for (const [f, v] of Object.entries(body)) setLocal(st.cb, f, v);
    ctx.update();
    flushBehavior().then(() => { if (msg) toast(msg, { type: 'success' }); });
  }

  function resetAll() {
    if (!st.cb) return;
    const body = {};
    for (const f of Object.keys(st.cb.overrides || {})) body[f] = null;
    if (!Object.keys(body).length) return;
    body.preset = null;
    applyOverrides(body, 'This chat now follows the defaults again');
  }

  function pickChatPreset(id) {
    if (!id) { resetAll(); return; }
    const cb = st.cb;
    const def = presetsList().find((p) => p.id === id);
    const prof = def && def[profileKey(cb && cb.kind)];
    if (!prof) return;
    const body = {};
    for (const f of ALL_FIELDS) if (prof[f] !== undefined) body[f] = prof[f];
    body.preset = id;
    applyOverrides(body, `${def.label || presetStyle(id).label} applied to this chat`);
  }

  /** Open the fine-tuning form at a section (e.g. 'proactive') and scroll to it. */
  function openForm(section) {
    st.formOpen = true;
    if (section) st.bui.open.add(section);
    ctx.update();
    requestAnimationFrame(() => {
      const el = ctx.el() && ctx.el().querySelector('.bh-form-wrap');
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }

  function behaviourSection(c) {
    const cb = st.cb;
    const isGroup = c.kind === 'group';
    const scope = isGroup ? 'Group defaults' : 'Private chat defaults';
    if (!cb) {
      return html`<section class="drawer-section" data-key="behaviour">
        <h4>${icon('sliders', 'ic-sm')}Behaviour</h4>
        ${st.cbError ? html`<div class="banner danger small">${icon('warning')}<div><strong>Couldn't load this chat's behaviour.</strong> ${st.cbError}
          <button class="link-btn" @click=${loadBehavior}>Try again</button></div></div>`
          : html`<div class="skeleton" style="height:64px;border-radius:16px"></div>`}
      </section>`;
    }
    const kindKey = profileKey(cb.kind || c.kind);
    const n = overrideCount(cb.overrides);
    const base = basePresetOf(cb);
    const tweaks = base ? ALL_FIELDS.filter((f) => cb.overrides[f] != null && !same(cb.overrides[f], base.values[f])).length : n;
    const defPreset = (cb.defaults && cb.defaults.preset) || 'custom';
    const defLabel = (presetsList().find((p) => p.id === defPreset) || {}).label || presetStyle(defPreset).label;
    const chip = !n ? '' : base ? base.id : detectPreset(cb.effective, presetsList(), kindKey);
    const look = presetStyle(base ? base.id : (chip || defPreset));
    const changes = (k) => `${k} change${k === 1 ? '' : 's'}`;
    const title = !n ? html`Using ${scope} · <b>${defLabel}</b>`
      : base ? html`Using the <b>${base.label}</b> preset${tweaks ? html` with <b>${changes(tweaks)}</b>` : ''}`
      : html`<b>${n} setting${n === 1 ? '' : 's'}</b> customised`;
    const sub = !n ? 'Change anything below to customise this chat only.'
      : base ? `Instead of the ${scope.toLowerCase()} (${defLabel}).`
      : `Everything else follows the ${scope.toLowerCase()} (${defLabel}).`;
    const tuneSub = !n ? 'everything follows the defaults' : base ? (tweaks ? `${base.label} + ${changes(tweaks)}` : `${base.label} preset`) : `${n} customised`;
    const closed = cb.available === false && !isAway(c);
    return html`<section class="drawer-section" data-key="behaviour">
      <h4>${icon('sliders', 'ic-sm')}Behaviour<span class="grow"></span>
        <button class="btn btn-glass btn-sm away-btn" @click=${ctx.away.menu} title="Pause replies in this chat for a while">${icon('snooze')}${isAway(c) ? 'Away' : 'Set away'}${icon('chevron-down', 'ic-sm')}</button>
      </h4>
      <div class="bh-summary" style=${`--c1:${look.c1};--c2:${look.c2}`}>
        <span class="bh-sum-icon" data-key=${'si-' + (base ? base.id : n ? 'custom' : defPreset)}>${icon(n && !base ? 'sliders' : look.icon)}</span>
        <div class="grow">
          <div class="bh-sum-title">${title}</div>
          <div class="bh-sum-sub">${sub}</div>
        </div>
        ${n ? html`<button class="btn btn-ghost btn-sm" @click=${resetAll} title=${`Forget this chat's changes and follow the ${scope.toLowerCase()}`}>${icon('reset')}Reset all</button>` : ''}
      </div>
      ${closed && cb.nextChangeAt ? html`<div class="banner info small mt-8">${icon('calendar')}<div>Outside active hours right now — back ${awayUntilText(cb.nextChangeAt).replace(/^until /, 'at ')}.</div></div>` : ''}
      <div class="mt-12">${presetChips(presetsList(), chip, pickChatPreset, { defaultLabel: 'Defaults' })}</div>
      <div class="mt-12">${vibeDials({ kind: cb.kind || c.kind, profile: cb.effective, meta: store.state.behaviorMeta, scope: 'chat-' + key, onChange: (fields) => editMany(fields), compact: true })}</div>
      <div class="mt-12">${timeline.view({ kind: cb.kind || c.kind, profile: cb.effective, title: 'How a reply unfolds here', compact: true })}</div>
      ${routineNote(c)}
      <div class="adv-switch-row mt-12">
        <div class="grow"><div class="field-label row gap-4">Advanced ${helpTip('advanced')}</div>
          <div class="field-help">${st.formOpen ? 'Every setting for this chat. Rows with a coloured label are set by that dial.' : html`Active hours, limits, message shape, check-ins and more <span class="faint">· ${tuneSub}</span>`}</div></div>
        ${toggle(!!st.formOpen, setAdvanced, { label: 'Show advanced settings for this chat' })}
      </div>
      ${st.formOpen ? html`<div class="bh-form-wrap adv-reveal">${behaviourForm({
        kind: cb.kind || c.kind, values: cb.effective, mode: 'overrides', sources: cb.sources || {}, defaults: cb.defaults,
        meta: store.state.behaviorMeta, ui: st.bui, onUpdate: () => ctx.update(), onChange: editOverride, scopeLabel: scope.toLowerCase(),
        basePreset: base, personaZone: ((personaById(c.personaId) || {}).world || {}).timezone || '',
      })}</div>` : ''}
    </section>`;
  }

  return {
    id: 'behaviour',
    /** The chat's behaviour view (GET /api/chats/{key}/behavior) with pending edits, or null. */
    cb: () => st.cb,
    editOverride,
    openForm,
    reloadSoon: reloadBehaviorSoon,
    /** Mirror a new Away time locally before the reload. */
    setSnoozedLocal(iso) { if (st.cb) st.cb = { ...st.cb, snoozedUntil: iso }; },
    load: loadBehavior,
    view: (c) => behaviourSection(c),
    onBus(type) {
      // Defaults or this chat changed elsewhere → refresh the effective profile.
      if (type === 'settings.changed') reloadBehaviorSoon();
      if (type === 'chats.changed' && !flushBehaviorSoon.pending()) reloadBehaviorSoon();
    },
    destroy() {
      if (flushBehaviorSoon.pending()) flushBehaviorSoon.flush();
      reloadBehaviorSoon.cancel();
      timeline.destroy();
    },
  };
}
