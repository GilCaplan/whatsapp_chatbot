// language-controls.js — which languages a persona speaks and the language of
// its replies (settings.behavior.languages.private|group, a chat's own
// `language` block, a person's `language`). LANGS mirrors lang.All in
// internal/lang/lang.go; defaults mirror lang.Effective.

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { seg } from '../../ui.js';

export const LANGS = [
  ['en', 'English'], ['he', 'Hebrew'], ['ar', 'Arabic'], ['ru', 'Russian'], ['uk', 'Ukrainian'],
  ['es', 'Spanish'], ['fr', 'French'], ['de', 'German'], ['it', 'Italian'], ['pt', 'Portuguese'],
  ['nl', 'Dutch'], ['tr', 'Turkish'], ['el', 'Greek'], ['hi', 'Hindi'], ['th', 'Thai'],
  ['zh', 'Chinese'], ['ja', 'Japanese'], ['ko', 'Korean'],
].map(([code, name]) => ({ code, name }));

export const DEFAULT_LANGS = ['en', 'he'];

export const langName = (code) => (LANGS.find((l) => l.code === code) || {}).name || code;

// Stays open across re-renders while you tick several.
const openMenus = new Set();

/** The preferences with defaults filled in (lang.Effective). */
export function effectiveLangs(p) {
  p = p || {};
  return {
    languages: p.languages && p.languages.length ? p.languages : DEFAULT_LANGS,
    understands: p.understands || [],
    others: p.others || 'ignore',
    reply: p.reply || 'match',
  };
}

/** One-line summary: "English and Hebrew · replies in the message's language". */
export function langSummary(p) {
  const e = effectiveLangs(p);
  const names = e.languages.map(langName);
  const list = e.others === 'answer' ? 'Replies to all languages' : `Replies to ${names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0]}`;
  const reply = e.reply === 'match' ? 'replies in the language of the message' : `always replies in ${langName(e.reply)}`;
  return `${list} · ${reply}`;
}

/** All, or a tick list in a dropdown. list = null means all. */
function langPicker(id, label, list, defaults, options, onChange) {
  const toggle = (code) => {
    const next = list.includes(code) ? list.filter((c) => c !== code) : [...list, code];
    if (next.length) onChange(next);
  };
  const picked = list ? LANGS.filter((l) => list.includes(l.code)).map((l) => l.name).join(', ') : '';
  return html`${seg([{ value: 'all', label: 'All' }, { value: 'some', label: 'Choose specific' }], list ? 'some' : 'all',
    (v) => onChange(v === 'all' ? null : list || defaults), { label, cls: 'seg-sm' })}
    ${list ? html`<details class="lang-menu" id=${id} ?open=${openMenus.has(id)}
      @toggle=${(ev) => { if (ev.target.open) openMenus.add(id); else openMenus.delete(id); }}>
      <summary class="select select-sm"><span class="ellipsis">${picked}</span></summary>
      <div class="lang-menu-list" role="group" aria-label=${label}>
        ${options.map((l) => {
          const on = list.includes(l.code);
          return html`<label class="lang-menu-item"><input type="checkbox" .checked=${on} ?disabled=${on && list.length === 1} @change=${() => toggle(l.code)}>${l.name}</label>`;
        })}
      </div>
    </details>` : ''}`;
}

/**
 * The editor for one preferences block. onChange(next) gets the whole block
 * (the API replaces it as a whole).
 */
export function languageControls(prefs, onChange, { idPrefix = 'lang' } = {}) {
  const e = effectiveLangs(prefs);
  const set = (patch) => onChange({ ...e, ...patch });
  const known = e.understands.length ? LANGS.filter((l) => e.understands.includes(l.code)) : LANGS;
  const all = e.others === 'answer';
  const setKnown = (list) => {
    if (!list) return set({ understands: [] });
    const languages = e.languages.filter((c) => list.includes(c));
    set({ understands: list, languages: languages.length ? languages : [list[0]], reply: e.reply === 'match' || list.includes(e.reply) ? e.reply : 'match' });
  };
  const setReplyTo = (list) => (list ? set({ others: 'ignore', languages: list }) : set({ others: 'answer' }));
  return html`<div class="lang-controls">
    <div class="field-row field-row-stack">
      <div><div class="field-label">${icon('globe', 'ic-sm')}Understands</div>
        <div class="field-help">${e.understands.length ? 'Messages in any other language are skipped completely, as if it could not read them.' : 'It can read messages in any language.'}</div></div>
      ${langPicker(idPrefix + '-known', 'Understands', e.understands.length ? e.understands : null, e.languages, LANGS, setKnown)}
    </div>
    <div class="field-row field-row-stack">
      <div><div class="field-label">Replies to</div>
        <div class="field-help">${all ? 'It answers everything it understands.' : 'It answers these. Other messages it understands stay as background. Short messages like "ok" or "lol" always count.'}</div></div>
      ${langPicker(idPrefix + '-to', 'Replies to', all ? null : e.languages, e.languages, known, setReplyTo)}
    </div>
    <div class="field-row">
      <div class="grow"><div class="field-label">Replies in</div>
        <div class="field-help">${e.reply === 'match' ? 'The language the person wrote in. Unclear messages follow the conversation.' : 'Always this language. You can still pick per person in the People tab.'}</div></div>
      <select class="select select-sm" aria-label="Replies in" @change=${(ev) => set({ reply: ev.target.value })}>
        <option value="match" ?selected=${e.reply === 'match'}>Same as the message</option>
        ${known.map((l) => html`<option value=${l.code} ?selected=${e.reply === l.code}>${l.name}</option>`)}
      </select>
    </div>
  </div>`;
}

/** A person's reply language: "" = the chat's choice, "match" or a code. */
export function personLanguageSelect(value, onChange, label = 'Reply language') {
  return html`<select class="select select-sm" aria-label=${label} @change=${(ev) => onChange(ev.target.value)}>
    <option value="" ?selected=${!value}>As the chat is set</option>
    <option value="match" ?selected=${value === 'match'}>Same as their message</option>
    ${LANGS.map((l) => html`<option value=${l.code} ?selected=${value === l.code}>${l.name}</option>`)}
  </select>`;
}
