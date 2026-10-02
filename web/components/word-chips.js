// word-chips.js — a small "type a word, press Enter" list editor.
//
//   wordChips({ words, onChange(words), placeholder, label, max = 20, maxLen = 40, tone })
//
// Enter or comma adds the typed word (trimmed, duplicates ignored
// case-insensitively), Backspace in an empty field removes the last chip,
// the x on a chip removes it. tone: '' | 'green' | 'red' (chip colour).

import { html } from '../dom.js';
import { icon } from '../icons.js';

export function wordChips({ words, onChange, placeholder = 'Type a word and press Enter', label = 'Words', max = 20, maxLen = 40, tone = '' }) {
  const list = Array.isArray(words) ? words : [];
  const emit = (next) => onChange && onChange(next);
  const add = (raw) => {
    const w = String(raw || '').replace(/\s+/g, ' ').trim().slice(0, maxLen);
    if (!w || list.length >= max || list.some((x) => x.toLowerCase() === w.toLowerCase())) return false;
    emit([...list, w]);
    return true;
  };
  const onKey = (e) => {
    const el = e.target;
    if ((e.key === 'Enter' || e.key === ',') && !e.isComposing) {
      e.preventDefault();
      if (add(el.value)) el.value = '';
    } else if (e.key === 'Backspace' && !el.value && list.length) {
      emit(list.slice(0, -1));
    }
  };
  const onBlur = (e) => { if (add(e.target.value)) e.target.value = ''; };
  const full = list.length >= max;
  return html`<div class=${'word-chips ' + (tone ? 'tone-' + tone : '')} role="group" aria-label=${label}>
    ${list.map((w, i) => html`<span class="word-chip" data-key=${'w' + i + w}>${w}<button type="button" class="word-x" aria-label=${'Remove ' + w}
      @click=${() => emit(list.filter((_, j) => j !== i))}>${icon('x')}</button></span>`)}
    <input class="word-input" type="text" maxlength=${maxLen} placeholder=${full ? `Up to ${max}` : list.length ? 'Add another…' : placeholder}
      ?disabled=${full} aria-label=${label} @keydown=${onKey} @blur=${onBlur}>
  </div>`;
}
