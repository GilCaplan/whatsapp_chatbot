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

/** The preferences with defaults filled in (lang.Effective). */
export function effectiveLangs(p) {
  p = p || {};
  return {
    languages: p.languages && p.languages.length ? p.languages : DEFAULT_LANGS,
    others: p.others || 'ignore',
    reply: p.reply || 'match',
  };
}

/** One-line summary: "English and Hebrew · replies in the message's language". */
export function langSummary(p) {
  const e = effectiveLangs(p);
  const names = e.languages.map(langName);
  const list = names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0];
  const reply = e.reply === 'match' ? 'replies in the language of the message' : `always replies in ${langName(e.reply)}`;
  return `${list} · ${reply}${e.others === 'answer' ? ' · answers other languages too' : ''}`;
}

/**
 * The editor for one preferences block. onChange(next) gets the whole block
 * (the API replaces it as a whole).
 */
export function languageControls(prefs, onChange, { idPrefix = 'lang' } = {}) {
  const e = effectiveLangs(prefs);
  const set = (patch) => onChange({ ...e, ...patch });
  const remove = (code) => {
    const languages = e.languages.filter((c) => c !== code);
    if (!languages.length) return;
    set({ languages, reply: e.reply === code ? 'match' : e.reply });
  };
  const add = (code) => { if (code && !e.languages.includes(code)) set({ languages: [...e.languages, code] }); };
  const rest = LANGS.filter((l) => !e.languages.includes(l.code));
  return html`<div class="lang-controls">
    <div class="field-row field-row-stack">
      <div><div class="field-label">${icon('globe', 'ic-sm')}Languages it speaks</div>
        <div class="field-help">It reads and answers these. Short messages like "ok" or "lol" always count.</div></div>
      <div class="lang-chips">
        ${e.languages.map((code) => html`<span class="chip chip-sm on lang-chip">${langName(code)}
          ${e.languages.length > 1 ? html`<button type="button" class="lang-x" aria-label=${'Remove ' + langName(code)} @click=${() => remove(code)}>${icon('x', 'ic-sm')}</button>` : ''}</span>`)}
        ${rest.length ? html`<select class="select select-sm" aria-label="Add a language" id=${idPrefix + '-add'}
            @change=${(ev) => { add(ev.target.value); ev.target.value = ''; }}>
          <option value="">Add a language…</option>
          ${rest.map((l) => html`<option value=${l.code}>${l.name}</option>`)}
        </select>` : ''}
      </div>
    </div>
    <div class="field-row">
      <div class="grow"><div class="field-label">Messages in other languages</div>
        <div class="field-help">${e.others === 'answer' ? 'Answered anyway, in the reply language below.' : 'It stays quiet (the message is still read as background).'}</div></div>
      ${seg([{ value: 'ignore', label: 'Stay quiet' }, { value: 'answer', label: 'Answer' }], e.others, (v) => set({ others: v }), { label: 'Messages in other languages', cls: 'seg-sm' })}
    </div>
    <div class="field-row">
      <div class="grow"><div class="field-label">Reply in</div>
        <div class="field-help">${e.reply === 'match' ? 'The language the person wrote in. Unclear messages follow the conversation.' : e.others === 'answer' ? 'Always this language, whatever people write. You can still pick per person in the People tab.' : 'Always this language. Messages in a language it does not speak still get no answer: set Messages in other languages to Answer to reply to those too.'}</div></div>
      <select class="select select-sm" aria-label="Reply in" @change=${(ev) => set({ reply: ev.target.value })}>
        <option value="match" ?selected=${e.reply === 'match'}>Same as the message</option>
        ${LANGS.map((l) => html`<option value=${l.code} ?selected=${e.reply === l.code}>${l.name}</option>`)}
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
