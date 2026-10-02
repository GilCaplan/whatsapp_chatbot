// mentions.js — render "@Name" tags in message text as chips.
//
//   mentionNodes(text, names)
//
// With `names` (the message's `mentions`, exact display names) only those
// tags become chips — longest first, so "@Maya K." wins over "@Maya". Without
// names (older history), any "@word" that isn't part of a word (so not an
// e-mail address) is shown as a chip. Returns an array of plain strings and
// templates; everything is escaped by the renderer.

import { html } from '../dom.js';

const WORD = /[\p{L}\p{N}_]/u;
const FALLBACK = /@[\p{L}\p{N}_]+/gu;

const chip = (label) => html`<span class="mention">@${label}</span>`;

export function mentionNodes(text, names) {
  const s = text == null ? '' : String(text);
  if (!s.includes('@')) return [s];
  const list = (Array.isArray(names) ? names : []).filter((n) => typeof n === 'string' && n).sort((a, b) => b.length - a.length);
  const out = [];
  let last = 0;
  if (list.length) {
    for (let i = s.indexOf('@'); i >= 0; i = s.indexOf('@', i + 1)) {
      if (i < last || (i > 0 && WORD.test(s[i - 1]))) continue;
      const rest = s.slice(i + 1);
      const name = list.find((n) => rest.startsWith(n) && !WORD.test(rest.charAt(n.length) || ' '));
      if (!name) continue;
      if (i > last) out.push(s.slice(last, i));
      out.push(chip(name));
      last = i + 1 + name.length;
    }
  } else {
    for (const m of s.matchAll(FALLBACK)) {
      const i = m.index;
      if (i > 0 && WORD.test(s[i - 1])) continue;
      if (i > last) out.push(s.slice(last, i));
      out.push(chip(m[0].slice(1)));
      last = i + m[0].length;
    }
  }
  if (last < s.length) out.push(s.slice(last));
  return out.length ? out : [s];
}

/** The names in `names` that `text` still tags with "@" (for "Will tag" hints). */
export function taggedIn(text, names) {
  const s = text == null ? '' : String(text);
  return (Array.isArray(names) ? names : []).filter((n) => {
    for (let i = s.indexOf('@' + n); i >= 0; i = s.indexOf('@' + n, i + 1)) {
      const before = i > 0 ? s[i - 1] : ' ';
      const after = s.charAt(i + 1 + n.length) || ' ';
      if (!WORD.test(before) && !WORD.test(after)) return true;
    }
    return false;
  });
}
