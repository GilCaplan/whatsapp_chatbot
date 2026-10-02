// pages/persona-editor.js — create / edit a persona (#/persona/<id> or #/persona/new).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, loadPersonas, usageCount } from '../store.js';
import { seg, field, toast, openMenu, spinner } from '../ui.js';
import { personaAvatar, personaGradient } from '../components/avatar.js';
import { emptyState } from '../components/art.js';
import { glyph, GLYPHS, GLYPH_IDS } from '../components/glyphs.js';
import { duplicatePersona, deletePersona, resetPersona } from './personas.js';
import { providerLabel } from '../components/status.js';
import { personaGoalControls } from '../components/goal-controls.js';
import { GRADIENTS, debounce, rowsFor, copyText, initials, modKey } from '../util.js';
import { worldCard } from '../components/world-card.js';

const FAV_SUGGEST = ['😂', '🤣', '😅', '🙏', '❤️', '🔥', '✨', '💅', '👀', '😭', '🥲', '😎', '👍', '🙌', '🤔', '💀', '😘', '🥰', '🍻', '🎉'];
const MAX_UPLOAD = 5 * 1024 * 1024;

// Plain-English meaning of each setting (what the app enforces, not just asks for).
const EMOJI_HELP = {
  none: 'Never. Any emoji the AI writes are removed before sending.',
  rare: 'About one message in five, a single emoji at most, and only for jokes, good news or warmth.',
  some: 'Now and then, up to two in a message where they fit. Plenty of messages have none.',
  lots: 'Most messages get an emoji or two that match the mood, often from the favourites below.',
};
const EMOJI_NOTE = 'Whatever the setting, replies to sad or serious news never get emoji, and reactions are chosen to suit the message.';
const LENGTH_HELP = {
  short: 'A few words to one sentence, like most real texts (about 12 words at most).',
  medium: 'One to three sentences (about 30 words at most).',
  long: 'A few sentences, up to a short paragraph (about 60 words at most).',
};
const LENGTH_NOTE = 'Longer replies are asked for again or cut at a sentence end. Each chat can still make replies shorter, longer or match the other person (Reply behaviour).';

function blank() {
  const g = GRADIENTS[Math.floor(Math.random() * GRADIENTS.length)];
  return {
    id: '', builtIn: false, name: '', tagline: '',
    avatar: { kind: 'generated', gradient: g.slice(), glyph: GLYPH_IDS[Math.floor(Math.random() * GLYPH_IDS.length)], initials: '' },
    bio: '', personality: '', style: '', vocabulary: '', rules: '', language: '',
    emoji: { usage: 'some', favorites: [] },
    messageLength: 'short', goal: '', decisionHint: '', fallbackReply: '', advancedPrompt: '', llm: null,
    goalStyle: 'subtle', goalPlanAhead: true, goalAfterReached: 'relax',
    world: { city: '', country: '', timezone: '', routine: [] },
  };
}

/** World & routine in the server's field order (so unsaved-change detection is exact). */
function normWorld(w) {
  const x = w || {};
  return {
    city: x.city || '', country: x.country || '', timezone: x.timezone || '',
    routine: (Array.isArray(x.routine) ? x.routine : []).map((b) => ({ label: b.label || '', days: Array.isArray(b.days) ? b.days.slice() : [], from: b.from || '', to: b.to || '', reach: b.reach || 'normal' })),
  };
}

function normalize(p) {
  const b = blank();
  const out = { ...b, ...(p || {}) };
  out.avatar = { ...b.avatar, ...(p && p.avatar ? p.avatar : {}) };
  if (!out.avatar.gradient || out.avatar.gradient.length < 2) out.avatar.gradient = personaGradient(out);
  out.emoji = { usage: (p && p.emoji && p.emoji.usage) || 'some', favorites: (p && p.emoji && Array.isArray(p.emoji.favorites)) ? p.emoji.favorites.slice() : [] };
  out.llm = p && p.llm && p.llm.provider ? { provider: p.llm.provider, model: p.llm.model || '' } : null;
  out.goalStyle = out.goalStyle || 'subtle';
  out.goalPlanAhead = out.goalPlanAhead !== false;
  out.goalAfterReached = out.goalAfterReached || 'relax';
  out.world = normWorld(p && p.world);
  return out;
}

const snapshot = (f) => JSON.stringify({ ...f, createdAt: undefined, updatedAt: undefined });

export default function PersonaEditor(ctx) {
  const update = () => ctx.update();
  const st = {
    id: '',
    form: null,
    original: '',
    loading: true,
    notFound: false,
    saving: false,
    avatarTab: 'style',
    pendingFile: null,
    previewURL: '',
    dragOver: false,
    uploading: false,
    models: {},
    modelsLoading: '',
    advancedOpen: false,
    more: new Set(), // "More details" disclosures that are open (progressive disclosure)
    preview: null,
    previewLoading: false,
    favInput: '',
    expr: null, // expression preview: { loading, error, samples, roll, key }
    worldUI: { open: false, q: '', other: false }, // World & routine card
  };

  const isNew = () => !st.id || st.id === 'new';
  const dirty = () => !!st.form && (snapshot(st.form) !== st.original || !!st.pendingFile);

  async function init(route) {
    const id = route.param || 'new';
    if (st.form && id === st.id) return;
    st.id = id;
    st.preview = null;
    st.more = new Set();
    clearPending();
    if (id === 'new') {
      const draft = route.query.get('draft') && store.state.draftPersona;
      st.form = normalize(draft || null);
      st.original = draft ? '' : snapshot(st.form); // a fresh draft counts as unsaved
      if (draft) store.set({ draftPersona: null });
      st.loading = false;
      st.advancedOpen = !!st.form.advancedPrompt;
      update();
      return;
    }
    st.loading = true; st.notFound = false; update();
    try {
      const p = await api.personas.get(id, { quiet: true });
      st.form = normalize(p);
      st.original = snapshot(st.form);
      st.advancedOpen = !!st.form.advancedPrompt;
      loadPreview();
    } catch (e) {
      st.notFound = true;
    }
    st.loading = false;
    update();
    if (st.form && st.form.llm) loadModels(st.form.llm.provider);
  }

  function clearPending() {
    if (st.previewURL) URL.revokeObjectURL(st.previewURL);
    st.previewURL = '';
    st.pendingFile = null;
  }

  async function loadModels(provider) {
    if (!provider || st.models[provider]) return;
    st.modelsLoading = provider; update();
    try {
      const r = await api.llm.models(provider, { quiet: true });
      st.models[provider] = (r && r.models) || [];
    } catch { st.models[provider] = []; }
    st.modelsLoading = '';
    update();
  }

  async function loadPreview() {
    if (isNew()) return;
    st.previewLoading = true; update();
    try {
      const r = await api.personas.promptPreview(st.id, '', { quiet: true });
      st.preview = r && typeof r.system === 'string' ? r.system : '';
    } catch (e) { st.preview = null; }
    st.previewLoading = false; update();
  }
  const previewSoon = debounce(loadPreview, 600);

  // ── Expression preview: sample replies at the current (unsaved) emoji/length settings ──
  const exprKey = () => JSON.stringify([st.form.emoji, st.form.messageLength, st.form.style, st.form.personality, st.form.llm]);
  async function loadExpression(reroll) {
    if (st.expr && st.expr.loading) return;
    const roll = reroll && st.expr ? (st.expr.roll || 0) + 1 : (st.expr ? st.expr.roll || 0 : 0);
    st.expr = { ...(st.expr || {}), loading: true, error: '', roll };
    update();
    try {
      const body = { persona: { ...st.form, name: st.form.name.trim() }, roll, count: 3 };
      const r = await api.personas.expressionPreview(isNew() ? 'new' : st.id, body, { quiet: true });
      st.expr = { loading: false, error: '', samples: (r && r.samples) || [], roll, key: exprKey() };
    } catch (e) {
      st.expr = { loading: false, error: (e && e.message) || 'Could not write samples', samples: (st.expr && st.expr.samples) || [], roll, key: st.expr && st.expr.key };
    }
    update();
  }

  const set = (k, v) => { st.form[k] = v; update(); };
  const setAvatar = (patch) => { st.form.avatar = { ...st.form.avatar, ...patch }; update(); };

  async function save() {
    if (st.saving) return;
    const f = st.form;
    if (!f.name.trim()) { toast('Give your persona a name first', { type: 'warn' }); focusField('pe-name'); return; }
    st.saving = true; update();
    try {
      const body = { ...f, name: f.name.trim() };
      let saved;
      if (isNew()) {
        saved = await api.personas.create({ ...body, id: '' });
        if (st.pendingFile) {
          try { saved = await api.personas.uploadAvatar(saved.id, st.pendingFile); } catch { /* toasted */ }
        }
        clearPending();
        st.id = saved.id;
        st.form = normalize(saved);
        st.original = snapshot(st.form);
        ctx.navigate(`/persona/${encodeURIComponent(saved.id)}`, { replace: true });
        toast(html`<b>${saved.name}</b> created`, { type: 'success', sub: 'Assign it to a chat or try it in the Playground.' });
      } else {
        saved = await api.personas.update(st.id, body);
        st.form = normalize(saved);
        st.original = snapshot(st.form);
        toast('Saved', { type: 'success' });
      }
      loadPersonas().catch(() => {});
      previewSoon();
    } catch { /* toasted */ } finally {
      st.saving = false; update();
    }
  }

  function focusField(id) {
    requestAnimationFrame(() => { const el = document.getElementById(id); if (el) el.focus(); });
  }

  async function handleFile(file) {
    if (!file) return;
    if (!/^image\/(png|jpe?g|gif)$/i.test(file.type)) { toast('Please choose a PNG, JPG or GIF image', { type: 'error' }); return; }
    if (file.size > MAX_UPLOAD) { toast('That image is over 5 MB — try a smaller one', { type: 'error' }); return; }
    if (isNew()) {
      clearPending();
      st.pendingFile = file;
      st.previewURL = URL.createObjectURL(file);
      update();
      return;
    }
    st.uploading = true; update();
    try {
      const p = await api.personas.uploadAvatar(st.id, file);
      st.form.avatar = { ...p.avatar };
      const orig = JSON.parse(st.original); orig.avatar = { ...p.avatar }; st.original = JSON.stringify(orig);
      loadPersonas().catch(() => {});
      toast('Photo updated', { type: 'success' });
    } catch { /* toasted */ } finally { st.uploading = false; update(); }
  }

  async function removePhoto() {
    if (st.pendingFile) { clearPending(); update(); return; }
    if (isNew()) { setAvatar({ kind: 'generated' }); return; }
    try {
      const p = await api.personas.removeAvatar(st.id);
      st.form.avatar = { ...p.avatar };
      const orig = JSON.parse(st.original); orig.avatar = { ...p.avatar }; st.original = JSON.stringify(orig);
      loadPersonas().catch(() => {});
      update();
    } catch { /* toasted */ }
  }

  // ── Sections ──
  function avatarEditor() {
    const f = st.form;
    const a = f.avatar;
    const hasPhoto = !!st.previewURL || a.kind === 'upload';
    const preview = { ...f, id: isNew() ? '' : st.id };
    return html`<div class="avatar-editor">
      <div class="ae-preview">
        ${personaAvatar(preview, 104, { preview: st.previewURL })}
        ${st.uploading ? html`<span class="ae-busy">${spinner()}</span>` : ''}
      </div>
      <div class="grow col gap-12">
        ${seg([{ value: 'style', label: 'Colour & picture', icon: 'smile' }, { value: 'upload', label: 'Photo', icon: 'image' }], st.avatarTab,
          (v) => { st.avatarTab = v; update(); }, { cls: 'seg-sm', label: 'Avatar type' })}
        ${st.avatarTab === 'style' ? html`
          <div class="swatches" role="radiogroup" aria-label="Background colour">
            ${GRADIENTS.map((g) => {
              const on = a.kind !== 'upload' && a.gradient && a.gradient[0] === g[0] && a.gradient[1] === g[1];
              return html`<button type="button" class=${'swatch ' + (on ? 'on' : '')} aria-label="Gradient" aria-pressed=${String(on)}
                style=${`background:linear-gradient(135deg,${g[0]},${g[1]})`}
                @click=${() => {
                  clearPending();
                  setAvatar({ kind: a.kind === 'upload' ? 'upload' : 'generated', gradient: g.slice() });
                  if (a.kind === 'upload') toast('A photo is set — remove it to show the colour avatar', { type: 'info' });
                }}></button>`;
            })}
          </div>
          <div class="glyph-grid" role="radiogroup" aria-label="Picture" style=${`--g1:${personaGradient(f)[0]};--g2:${personaGradient(f)[1]}`}>
            <button type="button" class=${'glyph-btn initials ' + (!GLYPHS[a.glyph] ? 'on' : '')} title="Use initials" aria-pressed=${String(!GLYPHS[a.glyph])}
              @click=${() => setAvatar({ glyph: '', initials: initials(f.name) })}><span>${initials(f.name || 'A B')}</span></button>
            ${GLYPH_IDS.map((id) => html`<button type="button" class=${'glyph-btn ' + (a.glyph === id ? 'on' : '')} title=${GLYPHS[id].label} aria-label=${GLYPHS[id].label}
              aria-pressed=${String(a.glyph === id)} @click=${() => setAvatar({ glyph: id })}><span>${glyph(id)}</span></button>`)}
          </div>
          ${a.kind === 'upload' ? html`<div class="small muted">A photo is set. <button class="link-btn" @click=${removePhoto}>Remove photo</button> to use colour & picture.</div>` : ''}
        ` : html`
          <label class=${'dropzone ' + (st.dragOver ? 'over' : '')}
              @dragenter=${(e) => { e.preventDefault(); st.dragOver = true; update(); }}
              @dragover=${(e) => { e.preventDefault(); }}
              @dragleave=${(e) => { if (!e.currentTarget.contains(e.relatedTarget)) { st.dragOver = false; update(); } }}
              @drop=${(e) => { e.preventDefault(); st.dragOver = false; handleFile(e.dataTransfer.files && e.dataTransfer.files[0]); }}>
            <input type="file" accept="image/png,image/jpeg,image/gif" @change=${(e) => { handleFile(e.target.files[0]); e.target.value = ''; }}>
            ${icon('upload', 'ic-lg')}
            <span><b>Drop a photo here</b> or click to choose</span>
            <span class="tiny faint">PNG, JPG or GIF up to 5 MB · cropped to a square</span>
          </label>
          ${hasPhoto ? html`<div class="row gap-8"><span class="small muted grow">${st.pendingFile ? 'Photo will upload when you save.' : 'Photo saved.'}</span>
            <button class="btn btn-ghost btn-sm" @click=${removePhoto}>${icon('trash')}Remove photo</button></div>` : ''}
        `}
      </div>
    </div>`;
  }

  function text(id, key, { label, help, placeholder, rows = 3, ic, optional, mono = false } = {}) {
    const v = st.form[key] || '';
    return field({
      label, help, icon: ic, optional,
      children: html`<textarea id=${id} class=${'textarea ' + (mono ? 'mono' : '')} rows=${rowsFor(v, rows, 14)} placeholder=${placeholder}
        @input=${(e) => { st.form[key] = e.target.value; update(); }}>${v}</textarea>`,
    });
  }

  function line(id, key, { label, help, placeholder, ic, optional, maxlength = 120 } = {}) {
    return field({
      label, help, icon: ic, optional,
      children: html`<input id=${id} class="input" maxlength=${maxlength} placeholder=${placeholder} .value=${st.form[key] || ''}
        @input=${(e) => { st.form[key] = e.target.value; update(); }}>`,
    });
  }

  function favorites() {
    const favs = st.form.emoji.favorites;
    const toggleFav = (e) => {
      const list = favs.includes(e) ? favs.filter((x) => x !== e) : [...favs, e].slice(0, 12);
      st.form.emoji = { ...st.form.emoji, favorites: list };
      update();
    };
    const addTyped = () => {
      const parts = Array.from(typeof Intl.Segmenter === 'function' ? new Intl.Segmenter(undefined, { granularity: 'grapheme' }).segment(st.favInput) : st.favInput)
        .map((x) => (typeof x === 'string' ? x : x.segment)).map((x) => x.trim()).filter(Boolean);
      const list = favs.slice();
      for (const p of parts) if (!list.includes(p)) list.push(p);
      st.form.emoji = { ...st.form.emoji, favorites: list.slice(0, 12) };
      st.favInput = '';
      update();
    };
    return html`<div class="col gap-8">
      <div class="row row-wrap gap-6 fav-list">
        ${favs.length ? favs.map((e) => html`<button type="button" class="chip chip-lg fav on" title="Remove" @click=${() => toggleFav(e)}><span class="emoji-glyph">${e}</span>${icon('x', 'x ic-sm')}</button>`)
          : html`<span class="small faint">No favourites yet — tap some below.</span>`}
      </div>
      <div class="row row-wrap gap-4 fav-suggest">
        ${FAV_SUGGEST.filter((e) => !favs.includes(e)).map((e) => html`<button type="button" class="emoji-btn sm" @click=${() => toggleFav(e)}>${e}</button>`)}
        <input class="input input-sm" style="width:110px" placeholder="Type any…" .value=${st.favInput}
          @input=${(e) => { st.favInput = e.target.value; }}
          @keydown=${(e) => { if (e.key === 'Enter') { e.preventDefault(); addTyped(); } }}>
      </div>
    </div>`;
  }

  function expressionPreview() {
    const x = st.expr;
    const name = st.form.name.trim() || 'They';
    const stale = x && x.samples && x.samples.length && x.key !== exprKey();
    return html`<div class="expr-preview">
      <div class="row gap-8">
        <div class="grow">
          <div class="field-label">Expression preview</div>
          <div class="field-help">Three sample replies with the settings above${dirty() ? ' (including unsaved changes)' : ''}. A local model takes a few seconds.</div>
        </div>
        <button class=${'btn btn-glass btn-sm ' + (x && x.loading ? 'loading' : '')} ?disabled=${!!(x && x.loading)}
          @click=${() => loadExpression(!!(x && x.samples && x.samples.length))}>${icon('refresh')}${x && x.samples && x.samples.length ? 'Re-roll' : 'Show samples'}</button>
      </div>
      ${x && x.error ? html`<div class="banner warn small mt-8">${icon('warning')}<div>${x.error}</div></div>` : ''}
      ${stale ? html`<div class="small faint mt-8">Settings changed since these were written. Re-roll to see the difference.</div>` : ''}
      ${x && x.loading && !(x.samples && x.samples.length) ? html`<div class="mt-8"><div class="skeleton sk-line w90"></div><div class="skeleton sk-line w60"></div></div>` : ''}
      ${x && x.samples && x.samples.length ? html`<div class="expr-samples mt-8">
        ${x.samples.map((smp) => html`<div class="chat-wall expr-sample">
          <div class="bubble-row"><div class="bubble">${smp.incoming}</div></div>
          <div class="bubble-row me"><div class="bubble"><span class="who">${name}</span>${smp.reply}</div>
            <div class="bubble-meta tiny faint">${smp.words} word${smp.words === 1 ? '' : 's'} of ${smp.budget} max · ${smp.emoji ? `${smp.emoji} emoji` : 'no emoji'}${smp.tone === 'serious' ? ' · serious message: emoji held back' : ''}${smp.adjusted ? ' · adjusted to your settings' : ''}</div></div>
        </div>`)}
      </div>` : ''}
    </div>`;
  }

  function brainCard() {
    const llm = st.form.llm;
    const provider = llm ? llm.provider : '';
    const models = provider ? st.models[provider] : null;
    const s = store.state.settings && store.state.settings.llm;
    const defaultTxt = s ? `${providerLabel(s.defaultProvider)}${s.defaultModel ? ' · ' + s.defaultModel : ''}` : 'Default';
    return html`<section class="card editor-side">
      <div class="section-head"><span class="s-icon" style="--c:linear-gradient(135deg,#22d3ee,#6366f1)">${icon('brain')}</span>
        <div><h2>Brain</h2><p>Which AI writes ${st.form.name || 'this persona'}'s replies.</p></div></div>
      <div class="field">
        <label class="field-label" for="pe-provider">Provider</label>
        <select id="pe-provider" class="select" @change=${(e) => {
          const v = e.target.value;
          if (!v) st.form.llm = null;
          else { st.form.llm = { provider: v, model: '' }; loadModels(v); }
          update();
        }}>
          <option value="" ?selected=${!provider}>Use default (${defaultTxt})</option>
          <option value="ollama" ?selected=${provider === 'ollama'}>Ollama — on this computer</option>
          <option value="anthropic" ?selected=${provider === 'anthropic'}>Claude</option>
          <option value="openai" ?selected=${provider === 'openai'}>OpenAI</option>
        </select>
      </div>
      ${provider ? html`<div class="field">
        <label class="field-label" for="pe-model">Model</label>
        ${st.modelsLoading === provider ? html`<div class="row gap-8 small muted">${spinner()}Loading models…</div>`
          : models && models.length ? html`<select id="pe-model" class="select" @change=${(e) => { st.form.llm = { provider, model: e.target.value }; update(); }}>
              <option value="" ?selected=${!llm.model}>Provider default</option>
              ${models.map((m) => html`<option value=${m.id} ?selected=${m.id === llm.model}>${m.label || m.id}</option>`)}
              ${llm.model && !models.some((m) => m.id === llm.model) ? html`<option value=${llm.model} selected>${llm.model}</option>` : ''}
            </select>`
          : html`<input id="pe-model" class="input mono" placeholder="model name" .value=${llm.model || ''} @input=${(e) => { st.form.llm = { provider, model: e.target.value.trim() }; update(); }}>
            <div class="field-help">Couldn't list models${provider !== 'ollama' ? ' — is the API key set in Settings?' : ''}. You can type a model name.</div>`}
      </div>` : ''}
    </section>`;
  }

  function previewCard() {
    return html`<section class="card editor-side">
      <div class="section-head"><span class="s-icon" style="--c:linear-gradient(135deg,#a78bfa,#f472b6)">${icon('text')}</span>
        <div class="grow"><h2>Prompt preview</h2><p>What the AI is told before each reply.</p></div>
        ${!isNew() ? html`<button class="btn btn-ghost btn-icon btn-sm" title="Refresh" aria-label="Refresh preview" @click=${loadPreview}>${icon('refresh')}</button>` : ''}
      </div>
      ${isNew() ? html`<div class="small muted">Save the persona to see its prompt.</div>`
        : html`${dirty() ? html`<div class="banner info mb-8 small">${icon('info')}<div>Save to update the preview with your latest edits.</div></div>` : ''}
          ${st.previewLoading && st.preview == null ? html`<div class="skeleton sk-line w90"></div><div class="skeleton sk-line w80"></div><div class="skeleton sk-line w60"></div>`
            : st.preview != null ? html`<div class="prompt-wrap"><pre class="prompt-preview">${st.preview}</pre>
                <button class="btn btn-glass btn-sm prompt-copy" @click=${() => copyText(st.preview).then(() => toast('Copied', { type: 'success' }))}>${icon('copy')}Copy</button></div>`
            : html`<div class="small muted">Preview unavailable.</div>`}`}
    </section>`;
  }

  /** Optional fields behind a "More details" disclosure; always shown once one has content. */
  function moreDetails(id, sub, filled, body) {
    if (filled) st.more.add(id); // stays open while a field is being emptied
    const open = st.more.has(id);
    if (open) return body;
    return html`<button class="disclosure mt-12" aria-expanded="false" @click=${() => { st.more.add(id); update(); }}>
      ${icon('plus', 'ic-sm')}<span>More details</span><span class="faint small">${sub}</span>${icon('chevron-down', 'ic-sm')}
    </button>`;
  }

  function advancedCard() {
    return html`<section class="card editor-side">
      <button class="disclosure" aria-expanded=${String(st.advancedOpen)} @click=${() => { st.advancedOpen = !st.advancedOpen; update(); }}>
        ${icon('sliders', 'ic-sm')}<span>Advanced prompt</span>
        <span class="faint small">${st.form.advancedPrompt ? 'in use' : 'optional'}</span>
        ${icon(st.advancedOpen ? 'chevron-up' : 'chevron-down', 'ic-sm')}
      </button>
      ${st.advancedOpen ? html`<div class="mt-12 col gap-10">
        <div class="banner warn small">${icon('warning')}<div><strong>For power users.</strong> When this box has text, it <b>replaces</b> the identity fields on the left. Safety rules are still added.</div></div>
        <textarea class="textarea mono" rows=${rowsFor(st.form.advancedPrompt, 6, 20)} placeholder="You are … (write the full character prompt)"
          @input=${(e) => { st.form.advancedPrompt = e.target.value; update(); }}>${st.form.advancedPrompt || ''}</textarea>
      </div>` : ''}
    </section>`;
  }

  function header() {
    const f = st.form;
    const n = isNew() ? 0 : usageCount(st.id);
    const persona = !isNew() ? store.state.personas.find((p) => p.id === st.id) : null;
    const menu = (e) => openMenu(e.currentTarget, [
      { label: 'Duplicate', icon: 'copy', onClick: () => duplicatePersona({ id: st.id, name: f.name }, ctx.navigate).catch(() => {}) },
      f.builtIn ? { label: 'Reset to original', icon: 'reset', onClick: async () => { const r = await resetPersona({ id: st.id, name: f.name }).catch(() => null); if (r) { st.form = normalize(r); st.original = snapshot(st.form); previewSoon(); update(); } } } : null,
      'sep',
      { label: 'Delete', icon: 'trash', danger: true, onClick: async () => { if (await deletePersona(persona || { id: st.id, name: f.name }).catch(() => false)) { st.original = snapshot(st.form); clearPending(); ctx.navigate('/personas'); } } },
    ], { align: 'end' });
    return html`<div class="editor-head glass">
      <a class="btn btn-ghost btn-sm" href="#/personas">${icon('chevron-left')}Personas</a>
      <div class="row gap-10 grow" style="min-width:0">
        ${personaAvatar({ ...f, id: isNew() ? '' : st.id }, 34, { preview: st.previewURL })}
        <div style="min-width:0">
          <div class="h3 ellipsis">${f.name || (isNew() ? 'New persona' : 'Untitled')}</div>
          <div class="tiny faint">${isNew() ? 'Not saved yet' : n ? `Used in ${n} chat${n === 1 ? '' : 's'}` : 'Not assigned to any chat'}${f.builtIn ? ' · built-in' : ''}</div>
        </div>
        ${dirty() ? html`<span class="chip chip-sm chip-amber unsaved">Unsaved changes</span>` : ''}
      </div>
      ${!isNew() ? html`<a class="btn btn-glass btn-sm hide-sm" href=${'#/playground?persona=' + encodeURIComponent(st.id)}>${icon('playground')}Try it</a>
        <button class="btn btn-ghost btn-icon btn-sm" aria-label="More" @click=${menu}>${icon('more')}</button>` : ''}
      <button class=${'btn btn-primary ' + (st.saving ? 'loading' : '')} ?disabled=${!dirty() && !isNew()} @click=${save} title=${`Save (${modKey()}${modKey() === '⌘' ? '' : '+'}S)`}>${icon('check')}${isNew() ? 'Create' : 'Save'}</button>
    </div>`;
  }

  function view() {
    if (st.loading) return html`<div class="center" style="min-height:50vh">${spinner('spinner-lg')}</div>`;
    if (st.notFound) {
      return html`<div class="card">${emptyState({ artName: 'personas', title: 'Persona not found', body: 'It may have been deleted.', action: { label: 'Back to personas', icon: 'chevron-left', onClick: () => ctx.navigate('/personas') } })}</div>`;
    }
    const f = st.form;
    return html`<div class="editor">
      ${header()}
      <div class="editor-grid">
        <div class="editor-main">
          <section class="card section">
            <div class="section-head"><span class="s-icon">${icon('user')}</span><div><h2>Who they are</h2><p>The basics: picture, name and background.</p></div></div>
            ${avatarEditor()}
            <div class="two-col mt-16">
              ${line('pe-name', 'name', { label: 'Name', placeholder: 'e.g. Leo', help: 'What friends call them.', maxlength: 60 })}
              ${line('pe-tagline', 'tagline', { label: 'Tagline', placeholder: 'e.g. Interior architect with zero patience for bad lighting', help: 'One line that sums them up.', maxlength: 140 })}
            </div>
            <div class="mt-16">${text('pe-bio', 'bio', { label: 'Bio & background', ic: 'info', rows: 4, placeholder: 'Where they live, what they do, family, hobbies, a few specific real-feeling details…', help: 'Facts the persona can draw on. Specific details make replies feel real.' })}</div>
          </section>

          ${worldCard({ world: f.world, name: f.name.trim(), ui: st.worldUI, onChange: (w) => { st.form.world = w; update(); }, onUpdate: update })}

          <section class="card section">
            <div class="section-head"><span class="s-icon" style="--c:linear-gradient(135deg,#f472b6,#8b5cf6)">${icon('heart')}</span><div><h2>Personality & voice</h2><p>How they think and how they text.</p></div></div>
            ${text('pe-personality', 'personality', { label: 'Personality', rows: 3, placeholder: 'Warm but sarcastic, loves gossip, gets bored of small talk fast…', help: 'Their character and attitude.' })}
            <div class="mt-16">${text('pe-style', 'style', { label: 'How they text', ic: 'typing', rows: 3, placeholder: 'lowercase, short bursts, barely any punctuation, sends 2–3 quick messages instead of one long one…', help: 'Capitalisation, punctuation, slang, pace — the more specific the better.' })}</div>
            ${moreDetails('voice', 'catch-phrases, language', !!(f.vocabulary || f.language), html`
            <div class="mt-16">${text('pe-vocab', 'vocabulary', { label: 'Vocabulary & catch-phrases', ic: 'quote', rows: 2, optional: true, placeholder: 'darling, honestly, “that’s a vibe”, “no because—”', help: 'Words and phrases they use a lot.' })}</div>
            <div class="mt-16">${line('pe-lang', 'language', { label: 'Language', ic: 'globe', optional: true, placeholder: 'e.g. English, with Hebrew slang', help: 'Which language(s) to reply in. Leave empty to match the other person.', maxlength: 160 })}</div>`)}
          </section>

          <section class="card section">
            <div class="section-head"><span class="s-icon" style="--c:linear-gradient(135deg,#fbbf24,#f97316)">${icon('smile')}</span><div><h2>Emoji & length</h2><p>Small things that make a texting style recognisable.</p></div></div>
            ${field({ label: 'Emoji usage', help: EMOJI_HELP[f.emoji.usage] || EMOJI_HELP.rare, children: seg([
              { value: 'none', label: 'None' }, { value: 'rare', label: 'Rarely' }, { value: 'some', label: 'Some' }, { value: 'lots', label: 'Lots' },
            ], f.emoji.usage, (v) => { st.form.emoji = { ...st.form.emoji, usage: v }; update(); }, { label: 'Emoji usage' }) })}
            <div class="tiny faint mt-4">${EMOJI_NOTE}</div>
            ${f.emoji.usage !== 'none' ? html`<div class="mt-16">${field({ label: 'Favourite emoji', optional: true, help: 'The ones they reach for first.', children: favorites() })}</div>` : ''}
            <div class="mt-16">${field({ label: 'Message length', icon: 'ruler', help: LENGTH_HELP[f.messageLength] || LENGTH_HELP.short, children: seg([
              { value: 'short', label: 'Short' }, { value: 'medium', label: 'Medium' }, { value: 'long', label: 'Long' },
            ], f.messageLength, (v) => set('messageLength', v), { label: 'Message length' }) })}
              <div class="tiny faint mt-4">${LENGTH_NOTE}</div></div>
            <div class="mt-16">${expressionPreview()}</div>
          </section>

          <section class="card section">
            <div class="section-head"><span class="s-icon" style="--c:linear-gradient(135deg,#34d399,#0ea5e9)">${icon('target')}</span><div><h2>Behaviour</h2><p>What they're after, when they speak up, and their limits.</p></div></div>
            ${text('pe-goal', 'goal', { label: 'Goal', ic: 'target', rows: 2, optional: true, placeholder: 'e.g. Catch up and find out how their week went.', help: 'What they quietly steer conversations towards. Each chat can override this.' })}
            <div class="mt-16">${personaGoalControls(f, set)}</div>
            <div class="mt-16">${text('pe-rules', 'rules', { label: 'Hard rules', ic: 'shield', rows: 2, optional: true, placeholder: 'Never agree to meet up. Never share an address. Don’t talk about politics.', help: 'Limits the persona must always follow.' })}</div>
            ${moreDetails('behaviour', 'group chats, fallback reply', !!(f.decisionHint || f.fallbackReply), html`
            <div class="mt-16">${text('pe-hint', 'decisionHint', { label: 'When would they jump into a group chat?', ic: 'group', rows: 2, optional: true, placeholder: 'e.g. When someone mentions food, football, or them by name. Otherwise mostly lurks.', help: 'Helps decide whether to reply in groups when nobody is talking to them directly.' })}</div>
            <div class="mt-16">${line('pe-fallback', 'fallbackReply', { label: 'Fallback reply', ic: 'message', optional: true, placeholder: 'e.g. haha wait what', help: 'Sent if the AI fails or slips out of character. Keep it short and in-voice.', maxlength: 200 })}</div>`)}
          </section>
        </div>

        <aside class="editor-aside">
          ${brainCard()}
          ${previewCard()}
          ${advancedCard()}
        </aside>
      </div>
    </div>`;
  }

  const onKey = (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); if (st.form && (dirty() || isNew())) save(); }
  };
  const onBeforeUnload = (e) => { if (dirty()) { e.preventDefault(); e.returnValue = ''; } };

  return {
    title: () => (st.form && st.form.name ? st.form.name : isNew() ? 'New persona' : 'Persona'),
    pageClass: 'page-editor',
    view,
    mount() {
      init(ctx.route);
      document.addEventListener('keydown', onKey);
      window.addEventListener('beforeunload', onBeforeUnload);
      // Keep the "It's 20:11 there" preview ticking while the card is open.
      st.clock = setInterval(() => { if (st.worldUI.open && st.form) update(); }, 30000);
    },
    onParams(route) {
      if ((route.param || 'new') !== st.id) init(route);
    },
    unmount() {
      clearInterval(st.clock);
      document.removeEventListener('keydown', onKey);
      window.removeEventListener('beforeunload', onBeforeUnload);
      previewSoon.cancel();
      clearPending();
    },
    isDirty: () => dirty(),
  };
}
