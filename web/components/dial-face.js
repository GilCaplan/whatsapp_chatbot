// dial-face.js — the custom SVG face of a vibe dial (static, trusted markup).
//
//   dialFace({ dial: 'speed'|'chattiness'|'boldness', level: 1..5, uid })
//
// A 270° arc with five notches, a gradient fill up to the level, a knob with
// a pointer that springs to the level, and a glyph per level that cross-fades.
// The DOM structure never changes between levels, so the morphing renderer
// only updates attributes and the CSS transitions run (fun.css › .df-*).

import { raw } from '../dom.js';

export const DIAL_LOOK = {
  speed: { c1: '#a78bfa', c2: '#6366f1', c3: '#38bdf8' },
  chattiness: { c1: '#34d399', c2: '#0ea5e9', c3: '#22d3ee' },
  boldness: { c1: '#fb923c', c2: '#f43f5e', c3: '#f472b6' },
};

const S = (d, w = 2.6) => `<path d="${d}" fill="none" stroke="#fff" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round"/>`;
const F = (d, o = 1) => `<path d="${d}" fill="#fff"${o < 1 ? ` fill-opacity="${o}"` : ''}/>`;
const dots = (y, o = 1) => `<g fill="#fff"${o < 1 ? ` fill-opacity="${o}"` : ''}><circle cx="14.5" cy="${y}" r="1.7"/><circle cx="20" cy="${y}" r="1.7"/><circle cx="25.5" cy="${y}" r="1.7"/></g>`;
const sparkle = (cx, cy, r) =>
  `M${cx} ${cy - r}C${cx + r * 0.14} ${cy - r * 0.3} ${cx + r * 0.3} ${cy - r * 0.14} ${cx + r} ${cy}C${cx + r * 0.3} ${cy + r * 0.14} ${cx + r * 0.14} ${cy + r * 0.3} ${cx} ${cy + r}` +
  `C${cx - r * 0.14} ${cy + r * 0.3} ${cx - r * 0.3} ${cy + r * 0.14} ${cx - r} ${cy}C${cx - r * 0.3} ${cy - r * 0.14} ${cx - r * 0.14} ${cy - r * 0.3} ${cx} ${cy - r}Z`;

// Glyphs on a 40×40 grid (centre 20,20), white on the dial's gradient.
const GLYPHS = {
  speed: [
    // 1 hourglass
    S('M12 7.5h16M12 32.5h16') + F('M14.5 9h11c0 5.5-5.5 7.8-5.5 11-0 0-5.5-5.5-5.5-11Z', 0.55) + S('M14.5 8c0 6.5 11 8.5 11 12s-11 5.5-11 12M25.5 8c0 6.5-11 8.5-11 12s11 5.5 11 12', 2.2) + F('M15.5 31.5c1.2-3 3-4.4 4.5-5 1.5.6 3.3 2 4.5 5Z'),
    // 2 clock, lazy hand
    S('M20 7.5a12.5 12.5 0 1 1 0 25a12.5 12.5 0 1 1 0-25Z') + S('M20 13v7.2l4.2 2.6') + F('M20 18.6a1.6 1.6 0 1 1 0 3.2a1.6 1.6 0 1 1 0-3.2Z'),
    // 3 typing bubble (natural)
    F('M10 10h20a5 5 0 0 1 5 5v8a5 5 0 0 1-5 5h-11l-6 5 1-5h-4a5 5 0 0 1-5-5v-8a5 5 0 0 1 5-5Z', 0.95) + `<g fill="#6366f1" fill-opacity=".9"><circle cx="13.5" cy="19" r="2"/><circle cx="20" cy="19" r="2"/><circle cx="26.5" cy="19" r="2"/></g>`,
    // 4 arrow with speed lines
    S('M13 20h17M23.5 12.5 31 20l-7.5 7.5', 3) + S('M5.5 14h6M4 20h5M5.5 26h6', 2.2),
    // 5 bolt
    F('M23.5 4.5 10.5 22.2h8.3L16.6 35.5 29.6 17.6h-8.4l2.3-13.1Z') + F(sparkle(32, 8, 3.2), 0.7),
  ],
  chattiness: [
    // 1 a quiet bubble (outline, faint dots)
    S('M11 12h18a4.5 4.5 0 0 1 4.5 4.5v6a4.5 4.5 0 0 1-4.5 4.5H19l-5 4 .8-4H11a4.5 4.5 0 0 1-4.5-4.5v-6A4.5 4.5 0 0 1 11 12Z', 2.2) + dots(19.5, 0.55),
    // 2 one bubble
    F('M11 11h18a5 5 0 0 1 5 5v7a5 5 0 0 1-5 5H19l-5.5 4.5 1-4.5H11a5 5 0 0 1-5-5v-7a5 5 0 0 1 5-5Z') + `<path d="M12 18.5h16M12 23h10" stroke="#10b981" stroke-width="2.2" stroke-linecap="round" stroke-opacity=".8"/>`,
    // 3 two bubbles
    F('M6.5 8h15a4 4 0 0 1 4 4v5a4 4 0 0 1-4 4H13l-4.5 3.5.8-3.5h-2.8a4 4 0 0 1-4-4v-5a4 4 0 0 1 4-4Z', 0.6) +
      F('M18.5 17h15a4 4 0 0 1 4 4v5a4 4 0 0 1-4 4h-2.8l.8 3.5-4.5-3.5h-8.5a4 4 0 0 1-4-4v-5a4 4 0 0 1 4-4Z'),
    // 4 two bubbles + sound arcs
    F('M5.5 12h15a4 4 0 0 1 4 4v5a4 4 0 0 1-4 4H12l-4.5 3.5.8-3.5H5.5a4 4 0 0 1-4-4v-5a4 4 0 0 1 4-4Z') +
      F('M20 22.5h12a3.5 3.5 0 0 1 3.5 3.5v3.5a3.5 3.5 0 0 1-3.5 3.5h-2l.6 3-3.8-3H20a3.5 3.5 0 0 1-3.5-3.5V26a3.5 3.5 0 0 1 3.5-3.5Z', 0.6) +
      S('M29 7.5c2.2 1.3 3.5 3.3 3.5 5.5M32.5 3.8c3.6 2.2 5.6 5.6 5.6 9.2', 2.2),
    // 5 a chorus of bubbles
    F('M3.5 6h12a3.5 3.5 0 0 1 3.5 3.5V14a3.5 3.5 0 0 1-3.5 3.5H10l-3.6 2.8.6-2.8H3.5A3.5 3.5 0 0 1 0 14V9.5A3.5 3.5 0 0 1 3.5 6Z', 0.55) +
      F('M24.5 4h12A3.5 3.5 0 0 1 40 7.5V12a3.5 3.5 0 0 1-3.5 3.5H34l.6 2.8-3.6-2.8h-6.5A3.5 3.5 0 0 1 21 12V7.5A3.5 3.5 0 0 1 24.5 4Z', 0.75) +
      F('M11 19h18a4.5 4.5 0 0 1 4.5 4.5v6A4.5 4.5 0 0 1 29 34H19l-5 4 .8-4H11a4.5 4.5 0 0 1-4.5-4.5v-6A4.5 4.5 0 0 1 11 19Z') + dots(26.6).replace('fill="#fff"', 'fill="#f43f5e" fill-opacity=".85"'),
  ],
  boldness: [
    // 1 plain: a calm ring
    S('M20 9a11 11 0 1 1 0 22a11 11 0 1 1 0-22Z', 2.6) + S('M15.5 20h9', 2.6),
    // 2 a small sparkle
    F(sparkle(20, 20, 8.5)) + F(sparkle(30, 10, 2.6), 0.6),
    // 3 sparkle + friend
    F(sparkle(18, 22, 11)) + F(sparkle(31, 9, 4.4), 0.8) + F(sparkle(31.5, 30, 2.4), 0.55),
    // 4 star + sparkles
    F('M20 5.5l4.1 8.6 9.4 1.3-6.8 6.6 1.6 9.4L20 26.9l-8.3 4.5 1.6-9.4-6.8-6.6 9.4-1.3Z') + F(sparkle(34, 30, 3.4), 0.75) + F(sparkle(6, 31, 2.4), 0.55),
    // 5 flame
    F('M21 3.5c1.4 6.1 9.8 9.4 9.8 18.5A10.8 10.8 0 0 1 20 33a10.8 10.8 0 0 1-10.8-11c0-4.3 2-7.6 4.6-9.8.2 3 1.4 5.4 3.6 6.4-.6-6.9 1.2-11.8 3.6-15.1Z') +
      `<path d="M20.5 18.5c.7 3.2 5.2 4.6 5.2 8.8A5.6 5.6 0 0 1 20 33a5.6 5.6 0 0 1-5.7-5.6c0-2.3 1-3.9 2.5-5 .3 1.6 1 2.4 2.1 2.8-.2-3 .4-5.1 1.6-6.7Z" fill="#f43f5e" fill-opacity=".85"/>` + F(sparkle(34, 8, 2.8), 0.7),
  ],
};

const SWEEP = 270;
const R = 50;
const ARC = (Math.PI * R * SWEEP) / 180;

/** Angle (degrees, 0 = up, clockwise) of a level. */
export const levelAngle = (level) => -SWEEP / 2 + ((Math.max(1, Math.min(5, level)) - 1) * SWEEP) / 4;

function polar(deg, r) {
  const a = ((deg - 90) * Math.PI) / 180;
  return [60 + r * Math.cos(a), 60 + r * Math.sin(a)];
}

/** The level nearest to a point on the face (for clicks/drags), or 0 when outside the dial. */
export function levelAt(svg, clientX, clientY) {
  const r = svg.getBoundingClientRect();
  const x = ((clientX - r.left) / r.width) * 120 - 60;
  const y = ((clientY - r.top) / r.height) * 120 - 60;
  if (Math.hypot(x, y) < 14) return 0;
  let deg = (Math.atan2(y, x) * 180) / Math.PI + 90; // 0 = up
  if (deg > 180) deg -= 360;
  if (deg < -SWEEP / 2 - 25 || deg > SWEEP / 2 + 25) return 0;
  return Math.max(1, Math.min(5, Math.round((deg + SWEEP / 2) / (SWEEP / 4)) + 1));
}

export function dialFace({ dial = 'speed', level = 3, uid = 'd' } = {}) {
  const look = DIAL_LOOK[dial] || DIAL_LOOK.speed;
  const g = `df-${uid}-${dial}`;
  const [sx, sy] = polar(-SWEEP / 2, R);
  const [ex, ey] = polar(SWEEP / 2, R);
  const track = `M${sx.toFixed(2)} ${sy.toFixed(2)}A${R} ${R} 0 1 1 ${ex.toFixed(2)} ${ey.toFixed(2)}`;
  const filled = ((level - 1) / 4) * ARC;
  const notches = [1, 2, 3, 4, 5].map((l) => {
    const [x, y] = polar(levelAngle(l), R);
    return `<circle class="df-notch${l <= level ? ' on' : ''}" cx="${x.toFixed(2)}" cy="${y.toFixed(2)}" r="${l === level ? 4.6 : 2.6}"/>`;
  }).join('');
  const glyphs = (GLYPHS[dial] || GLYPHS.speed).map((svg, i) =>
    `<g transform="translate(40 40)"><g class="df-glyph${i + 1 === level ? ' on' : ''}">${svg}</g></g>`).join('');
  return raw(`<svg class="df" viewBox="0 0 120 120" aria-hidden="true" focusable="false" xmlns="http://www.w3.org/2000/svg">
    <defs>
      <linearGradient id="${g}-arc" x1="0" y1="1" x2="1" y2="0"><stop offset="0" stop-color="${look.c1}"/><stop offset=".55" stop-color="${look.c2}"/><stop offset="1" stop-color="${look.c3}"/></linearGradient>
      <linearGradient id="${g}-face" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${look.c1}"/><stop offset="1" stop-color="${look.c2}"/></linearGradient>
      <radialGradient id="${g}-shine" cx=".35" cy=".25" r=".8"><stop offset="0" stop-color="#fff" stop-opacity=".55"/><stop offset=".5" stop-color="#fff" stop-opacity=".08"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></radialGradient>
      <filter id="${g}-sh" x="-30%" y="-30%" width="160%" height="170%"><feDropShadow dx="0" dy="5" stdDeviation="5" flood-color="${look.c2}" flood-opacity=".45"/></filter>
    </defs>
    <path class="df-track" d="${track}"/>
    <path class="df-arc" d="${track}" stroke="url(#${g}-arc)" stroke-dasharray="${ARC.toFixed(2)}" style="stroke-dashoffset:${(ARC - filled).toFixed(2)}"/>
    ${notches}
    <g class="df-knob" filter="url(#${g}-sh)">
      <circle cx="60" cy="60" r="36" fill="url(#${g}-face)"/>
      <circle cx="60" cy="60" r="36" fill="url(#${g}-shine)"/>
      <circle cx="60" cy="60" r="35.5" fill="none" stroke="#fff" stroke-opacity=".35"/>
    </g>
    <g class="df-pointer" style="transform:rotate(${levelAngle(level)}deg)"><rect x="57.6" y="25.5" width="4.8" height="9" rx="2.4" fill="#fff"/></g>
    <g class="df-glyphs">${glyphs}</g>
  </svg>`);
}
