// glyphs.js — custom avatar graphics (used instead of emoji).
// Each glyph is drawn on a 32×32 grid in three tones that sit on top of the
// persona's gradient: solid white, soft white, and a translucent dark "cut".
// Keep GLYPHS ids in sync with persona.Glyphs (internal/persona/avatar.go).

import { raw } from '../dom.js';

const M = (d) => `<path d="${d}" fill="#fff"/>`;
const S = (d) => `<path d="${d}" fill="#fff" fill-opacity=".55"/>`;
const C = (d) => `<path d="${d}" fill="#0a0f1e" fill-opacity=".22"/>`;
const circ = (cx, cy, r, tone = 'M') => {
  const fill = tone === 'C' ? 'fill="#0a0f1e" fill-opacity=".22"' : tone === 'S' ? 'fill="#fff" fill-opacity=".55"' : 'fill="#fff"';
  return `<circle cx="${cx}" cy="${cy}" r="${r}" ${fill}/>`;
};
const rect = (x, y, w, h, rx, tone = 'M') => {
  const fill = tone === 'C' ? 'fill="#0a0f1e" fill-opacity=".22"' : tone === 'S' ? 'fill="#fff" fill-opacity=".55"' : 'fill="#fff"';
  return `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${rx}" ${fill}/>`;
};
const line = (d, tone = 'M', w = 2.6) => {
  const stroke = tone === 'C' ? 'stroke="#0a0f1e" stroke-opacity=".22"' : tone === 'S' ? 'stroke="#fff" stroke-opacity=".55"' : 'stroke="#fff"';
  return `<path d="${d}" fill="none" ${stroke} stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round"/>`;
};
const sparkle = (cx, cy, r) =>
  `M${cx} ${cy - r}c${r * 0.12} ${r * 0.7} ${r * 0.3} ${r * 0.88} ${r} ${r}c${-r * 0.7} ${r * 0.12} ${-r * 0.88} ${r * 0.3} ${-r} ${r}` +
  `c${-r * 0.12} ${-r * 0.7} ${-r * 0.3} ${-r * 0.88} ${-r} ${-r}c${r * 0.7} ${-r * 0.12} ${r * 0.88} ${-r * 0.3} ${r} ${-r}z`;

export const GLYPHS = {
  spark: {
    label: 'Sparkle',
    svg: M('M16 3.5c.9 6.6 5.3 11.3 12.5 12.5-7.2 1.2-11.6 5.9-12.5 12.5-.9-6.6-5.3-11.3-12.5-12.5C10.7 14.8 15.1 10.1 16 3.5z') +
      S(sparkle(25.5, 6.5, 3.5)) + circ(6.5, 26, 1.6, 'S'),
  },
  rocket: {
    label: 'Rocket',
    svg: `<g transform="translate(16 16) scale(1.12) rotate(40) translate(-16 -16)">` +
      S('M10.4 13.8C7.8 15.2 6.5 17.6 6.5 21l4-2.5z') + S('M21.6 13.8c2.6 1.4 3.9 3.8 3.9 7.2l-4-2.5z') +
      S('M12.5 20h7c0 3.4-1.4 6.3-3.5 8.5-2.1-2.2-3.5-5.1-3.5-8.5z') +
      M('M16 3c4.6 3.6 6.6 8.6 6 15.5H10C9.4 11.6 11.4 6.6 16 3z') +
      circ(16, 11.5, 3, 'C') + circ(16, 11.5, 1.7, 'S') + `</g>`,
  },
  crystal: {
    label: 'Crystal ball',
    svg: S('M8.5 28.5h15l-1.8-4.4H10.3z') + circ(16, 14, 10) +
      C('M24.6 9.6A10 10 0 0 1 11 23.2a9 9 0 0 0 13.6-13.6z') + C(sparkle(16, 14, 4.6)),
  },
  cash: {
    label: 'Banknote',
    svg: `<rect x="5" y="5.5" width="22" height="13" rx="2.5" fill="#fff" fill-opacity=".55" transform="rotate(-10 16 12)"/>` +
      rect(4, 11, 24, 15, 3) + circ(16, 18.5, 3.6, 'C') + circ(8.5, 18.5, 1.2, 'C') + circ(23.5, 18.5, 1.2, 'C'),
  },
  dumbbell: {
    label: 'Dumbbell',
    svg: rect(2.5, 11.5, 3.5, 9, 1.5, 'S') + rect(26, 11.5, 3.5, 9, 1.5, 'S') +
      rect(8, 14.5, 16, 3, 1.5) + rect(6, 8.5, 4.5, 15, 2) + rect(21.5, 8.5, 4.5, 15, 2),
  },
  martini: {
    label: 'Martini',
    svg: line('M13 10.5 20.5 2.8', 'S', 1.4) +
      `<path d="M4.5 6.5h23L16 18.5z" fill="#fff" stroke="#fff" stroke-width="1.2" stroke-linejoin="round"/>` +
      rect(15, 18, 2, 7.5, 1) + rect(10, 25, 12, 2.5, 1.25) + circ(13, 10.5, 2.3, 'C'),
  },
  sofa: {
    label: 'Sofa',
    svg: rect(5, 7, 22, 11, 3.5, 'S') + rect(6, 15, 20, 7, 2.5) + rect(2.5, 12, 6, 11, 3) + rect(23.5, 12, 6, 11, 3) +
      rect(5.5, 23, 2.2, 3, 1, 'S') + rect(24.3, 23, 2.2, 3, 1, 'S') + rect(15.5, 15.5, 1, 6, 0.5, 'C'),
  },
  palette: {
    label: 'Palette',
    svg: M('M16 4C9.4 4 4 9.2 4 15.7 4 22.4 9.3 28 15.6 28c2.5 0 3.7-1.7 3-3.8-.7-2 .3-3.7 2.4-3.7h2.2c2.8 0 4.8-2.1 4.8-5C28 9 22.6 4 16 4z') +
      circ(10.5, 12.5, 2.1, 'C') + circ(15.5, 9, 2.1, 'C') + circ(21, 11.2, 2.1, 'C') + circ(9.8, 18.6, 2.1, 'C'),
  },
  coffee: {
    label: 'Coffee',
    svg: line('M11 4.5c-1.5 1.6 1.5 2.8 0 4.6', 'S', 1.8) + line('M16.5 4.5c-1.5 1.6 1.5 2.8 0 4.6', 'S', 1.8) +
      M('M5.5 12.5h17V19a7 7 0 0 1-7 7h-3a7 7 0 0 1-7-7z') + line('M22.5 14.5h1.8a3 3 0 0 1 0 6h-2', 'M', 2.4) +
      rect(3.5, 27, 21, 2, 1, 'S'),
  },
  headphones: {
    label: 'Headphones',
    svg: line('M6.5 19v-3a9.5 9.5 0 0 1 19 0v3', 'M', 3) + rect(3.5, 17, 7, 11, 3) + rect(21.5, 17, 7, 11, 3) +
      rect(7, 19.5, 1.6, 6, 0.8, 'C') + rect(23.4, 19.5, 1.6, 6, 0.8, 'C'),
  },
  leaf: {
    label: 'Leaf',
    svg: line('M8 26.5 4.8 29.6', 'S', 2.2) + M('M27 5C15.5 4.5 6 10 6 20.5c0 2.4.7 4.4 2 6 11-.3 19-8 19-21.5z') +
      line('M8.5 26C12.5 19.8 17.3 14.6 23 9.8', 'C', 1.6),
  },
  moon: {
    label: 'Moon',
    svg: M('M19.5 4.2A11.5 11.5 0 1 0 27.8 21 9.6 9.6 0 0 1 19.5 4.2z') + S(sparkle(25.5, 7, 3)) + circ(23, 14, 1.1, 'S'),
  },
  sun: {
    label: 'Sun',
    svg: ['M16 6.5V3', 'M22.7 9.3l2.5-2.5', 'M25.5 16H29', 'M22.7 22.7l2.5 2.5', 'M16 25.5V29', 'M9.3 22.7l-2.5 2.5', 'M6.5 16H3', 'M9.3 9.3 6.8 6.8']
      .map((d) => line(d, 'S', 2.6)).join('') + circ(16, 16, 6.4),
  },
  wave: {
    label: 'Waves',
    svg: line('M3.5 9.5c2.2 0 2.8-2.5 5-2.5s2.8 2.5 5 2.5 2.8-2.5 5-2.5 2.8 2.5 5 2.5 2.8-2.5 5-2.5', 'M', 2.8) +
      line('M3.5 16.5c2.2 0 2.8-2.5 5-2.5s2.8 2.5 5 2.5 2.8-2.5 5-2.5 2.8 2.5 5 2.5 2.8-2.5 5-2.5', 'M', 2.8) +
      line('M3.5 23.5c2.2 0 2.8-2.5 5-2.5s2.8 2.5 5 2.5 2.8-2.5 5-2.5 2.8 2.5 5 2.5 2.8-2.5 5-2.5', 'S', 2.8),
  },
  flame: {
    label: 'Flame',
    svg: M('M16 3c.6 4.7 4.4 7 6.6 10.4 1.3 2 2 4.1 2 6.3A8.6 8.6 0 0 1 16 28.3a8.6 8.6 0 0 1-8.6-8.6c0-3.4 1.6-6 3.6-8 .3 2 1.3 3.4 2.7 4.2C13.3 11.2 13.9 6.4 16 3z') +
      C('M16 16.5c1.6 2.2 3.8 3.3 3.8 6.1a3.8 3.8 0 0 1-7.6 0c0-1.6.7-2.8 1.7-3.8.5 1 1.1 1.4 1.6 1.6-.1-1.4.1-2.7.5-3.9z'),
  },
  heart: {
    label: 'Heart',
    svg: M('M16 27.5C6.8 21.6 3 16.4 3 11.4 3 7.6 5.9 4.8 9.5 4.8c2.7 0 4.9 1.5 6.5 3.9 1.6-2.4 3.8-3.9 6.5-3.9 3.6 0 6.5 2.8 6.5 6.6 0 5-3.8 10.2-13 16.1z') +
      line('M7.3 11.2a3 3 0 0 1 2.6-2.6', 'C', 1.6),
  },
  book: {
    label: 'Book',
    svg: S('M16 8.5c3.5-2.6 8-3.2 12-2v18.6c-4-1-8.5-.4-12 2.4z') + M('M16 8.5c-3.5-2.6-8-3.2-12-2v18.6c4-1 8.5-.4 12 2.4z') +
      line('M7 11.5c2-.4 4-.2 6 .8', 'C', 1.3) + line('M7 15.5c2-.4 4-.2 6 .8', 'C', 1.3) + line('M7 19.5c2-.4 4-.2 6 .8', 'C', 1.3),
  },
  camera: {
    label: 'Camera',
    svg: M('M6 10h3.5l2-3.2h9l2 3.2H26a3 3 0 0 1 3 3v11a3 3 0 0 1-3 3H6a3 3 0 0 1-3-3V13a3 3 0 0 1 3-3z') +
      circ(16, 18, 5.8, 'C') + circ(16, 18, 3.6) + circ(16, 18, 1.4, 'C') + rect(23, 12.5, 3, 1.8, 0.9, 'C'),
  },
  crown: {
    label: 'Crown',
    svg: circ(4, 10, 1.6, 'S') + circ(16, 6.4, 1.6, 'S') + circ(28, 10, 1.6, 'S') +
      M('M4 10.5l6.2 5.3L16 7l5.8 8.8 6.2-5.3-2.6 13.5H6.6z') + rect(6.6, 25, 18.8, 3, 1.2) +
      circ(16, 19, 1.6, 'C') + circ(10.5, 20.2, 1.2, 'C') + circ(21.5, 20.2, 1.2, 'C'),
  },
  paw: {
    label: 'Paw',
    svg: M('M16 16.5c-4.2 0-8 4.3-8 8 0 2.2 1.6 3.5 3.6 3.5 1.8 0 2.8-1 4.4-1s2.6 1 4.4 1c2 0 3.6-1.3 3.6-3.5 0-3.7-3.8-8-8-8z') +
      `<g fill="#fff"><ellipse cx="7.5" cy="14" rx="2.4" ry="3" transform="rotate(-20 7.5 14)"/>` +
      `<ellipse cx="12.5" cy="8.5" rx="2.5" ry="3.2" transform="rotate(-8 12.5 8.5)"/>` +
      `<ellipse cx="19.5" cy="8.5" rx="2.5" ry="3.2" transform="rotate(8 19.5 8.5)"/>` +
      `<ellipse cx="24.5" cy="14" rx="2.4" ry="3" transform="rotate(20 24.5 14)"/></g>`,
  },
  flower: {
    label: 'Flower',
    svg: [[16, 8.5], [22.2, 13], [19.8, 20.3], [12.2, 20.3], [9.8, 13]].map(([x, y]) => circ(x, y, 4.7)).join('') +
      circ(16, 15, 3.4, 'C'),
  },
  bolt: {
    label: 'Lightning',
    svg: `<path d="M18.5 3 7 18h8l-2 11 12-15.5h-8.2z" fill="#fff" stroke="#fff" stroke-width="1.2" stroke-linejoin="round"/>` +
      S(sparkle(25.5, 24.5, 2.8)),
  },
  chat: {
    label: 'Chat',
    svg: S('M12 4.5h12a5 5 0 0 1 5 5v5a5 5 0 0 1-5 5h-1v3.5L19 19.5h-7a5 5 0 0 1-5-5v-5a5 5 0 0 1 5-5z') +
      M('M8 11h11a5 5 0 0 1 5 5v5a5 5 0 0 1-5 5h-7l-4.5 3.5V26H8a5 5 0 0 1-5-5v-5a5 5 0 0 1 5-5z') +
      circ(9.5, 18.5, 1.4, 'C') + circ(13.5, 18.5, 1.4, 'C') + circ(17.5, 18.5, 1.4, 'C'),
  },
  globe: {
    label: 'Globe',
    svg: circ(16, 16, 12) +
      `<ellipse cx="16" cy="16" rx="5" ry="12" fill="none" stroke="#0a0f1e" stroke-opacity=".22" stroke-width="1.6"/>` +
      line('M4 16h24', 'C', 1.6) + line('M5.6 10h20.8', 'C', 1.6) + line('M5.6 22h20.8', 'C', 1.6),
  },
};

export const GLYPH_IDS = Object.keys(GLYPHS);

/** SVG markup for a glyph id, or '' when unknown (callers fall back to initials). */
export function glyph(id, cls = 'glyph') {
  const g = GLYPHS[id];
  if (!g) return '';
  return raw(`<svg class="${cls}" viewBox="0 0 32 32" aria-hidden="true" focusable="false">${g.svg}</svg>`);
}
