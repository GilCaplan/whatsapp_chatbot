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
    others: p.others || 'ignore',
    reply: p.reply || 'match',
  };
}

/** One-line summary: "English and Hebrew · replies in the message's language". */
export function langSummary(p) {
  const e = effectiveLangs(p);
  const names = e.languages.map(langName);
  const list = e.others === 'answer' ? 'All languages' : names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0];
  const reply = e.reply === 'match' ? 'replies in the language of the message' : `always replies in ${langName(e.reply)}`;
  return `${list} · ${reply}`;
}

/**
 * The editor for one preferences block. onChange(next) gets the whole block
 * (the API replaces it as a whole).
 */
export function languageControls(prefs, onChange, { idPrefix = 'lang' } = {}) {
  const e = effectiveLangs(prefs);
  const set = (patch) => onChange({ ...e, ...patch });
  const all = e.others === 'answer';
  const toggle = (code) => {
    const languages = e.languages.includes(code) ? e.languages.filter((c) => c !== code) : [...e.languages, code];
    if (!languages.length) return;
    set({ languages, reply: languages.includes(e.reply) || e.reply === 'match' ? e.reply : 'match' });
  };
  const picked = LANGS.filter((l) => e.languages.includes(l.code)).map((l) => l.name).join(', ');
  return html`<div class="lang-controls">
    <div class="field-row field-row-stack">
      <div><div class="field-label">${icon('globe', 'ic-sm')}Languages it speaks</div>
        <div class="field-help">${all ? 'It answers messages in any language.' : 'It answers these and stays quiet on others, which it still reads as background. Short messages like "ok" or "lol" always count.'}</div></div>
      ${seg([{ value: 'all', label: 'All languages' }, { value: 'some', label: 'Choose specific' }], all ? 'all' : 'some', (v) => set({ others: v === 'all' ? 'answer' : 'ignore' }), { label: 'Languages it speaks', cls: 'seg-sm' })}
      ${all ? '' : html`<details class="lang-menu" id=${idPrefix + '-menu'} ?open=${openMenus.has(idPrefix)}
          @toggle=${(ev) => { if (ev.target.open) openMenus.add(idPrefix); else openMenus.delete(idPrefix); }}>
        <summary class="select select-sm"><span class="ellipsis">${picked}</span></summary>
        <div class="lang-menu-list" role="group" aria-label="Languages it speaks">
          ${LANGS.map((l) => {
            const on = e.languages.includes(l.code);
            return html`<label class="lang-menu-item"><input type="checkbox" .checked=${on} ?disabled=${on && e.languages.length === 1} @change=${() => toggle(l.code)}>${l.name}</label>`;
          })}
        </div>
      </details>`}
    </div>
    <div class="field-row">
      <div class="grow"><div class="field-label">Reply in</div>
        <div class="field-help">${e.reply === 'match' ? 'The language the person wrote in. Unclear messages follow the conversation.' : all ? 'Always this language, whatever people write. You can still pick per person in the People tab.' : 'Always this language. Messages in a language it does not speak still get no answer: choose All languages to reply to those too.'}</div></div>
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
