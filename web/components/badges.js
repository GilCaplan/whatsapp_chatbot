// badges.js — custom SVG badges for missions and achievement medals (static,
// trusted markup; never emoji). Ids match internal/mission (Badge fields).
//
//   badgeGlyph(id)                          → white glyph on a 24×24 grid (raw SVG string)
//   missionBadge(id, category, { size })    → rounded tile for a mission template
//   medal(achievement, { size })            → medal with ribbon; locked = desaturated + progress ring

import { raw } from '../dom.js';

const st = (d, w = 2) => `<path d="${d}" fill="none" stroke="#fff" stroke-width="${w}" stroke-linecap="round" stroke-linejoin="round"/>`;
const fl = (d, o = 1) => `<path d="${d}" fill="#fff"${o < 1 ? ` fill-opacity="${o}"` : ''}/>`;
const star = (cx, cy, r) => {
  let d = '';
  for (let k = 0; k < 10; k++) {
    const rr = k % 2 ? r * 0.45 : r;
    const a = (Math.PI / 5) * k - Math.PI / 2;
    d += `${k ? 'L' : 'M'}${(cx + rr * Math.cos(a)).toFixed(2)} ${(cy + rr * Math.sin(a)).toFixed(2)}`;
  }
  return d + 'Z';
};
const sparkle = (cx, cy, r) =>
  `M${cx} ${cy - r}C${cx + r * 0.15} ${cy - r * 0.3} ${cx + r * 0.3} ${cy - r * 0.15} ${cx + r} ${cy}C${cx + r * 0.3} ${cy + r * 0.15} ${cx + r * 0.15} ${cy + r * 0.3} ${cx} ${cy + r}` +
  `C${cx - r * 0.15} ${cy + r * 0.3} ${cx - r * 0.3} ${cy + r * 0.15} ${cx - r} ${cy}C${cx - r * 0.3} ${cy - r * 0.15} ${cx - r * 0.15} ${cy - r * 0.3} ${cx} ${cy - r}Z`;

const GLYPHS = {
  word: fl('M4 17.5c0-5 1.8-8.6 5.6-10.6l1 1.6C8.4 10 7.6 11.8 7.5 13.5H10V18H4Z') + fl('M13 17.5c0-5 1.8-8.6 5.6-10.6l1 1.6c-2.2 1.5-3 3.3-3.1 5H19V18h-6Z'),
  laugh: st('M12 3.5a8.5 8.5 0 1 1 0 17a8.5 8.5 0 1 1 0-17Z') + st('M7.8 10.2l1.6-1.4 1.6 1.4M13 10.2l1.6-1.4 1.6 1.4', 1.8) + fl('M7.6 13.2h8.8c-.4 2.7-2.2 4.3-4.4 4.3s-4-1.6-4.4-4.3Z'),
  heart: fl('M12 20.2C6.2 16.4 3.5 13.3 3.5 9.6 3.5 7 5.4 5 7.9 5c1.7 0 3.1.9 4.1 2.4C13 5.9 14.4 5 16.1 5c2.5 0 4.4 2 4.4 4.6 0 3.7-2.7 6.8-8.5 10.6Z'),
  sun: fl('M12 7.5a4.5 4.5 0 1 1 0 9a4.5 4.5 0 1 1 0-9Z') + st('M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4'),
  tag: fl('M3.5 5.5v5.6c0 .5.2 1 .6 1.4l7.5 7.5c.8.8 2 .8 2.8 0l5.4-5.4c.8-.8.8-2 0-2.8L12.3 4.3c-.4-.4-.9-.6-1.4-.6H5.3c-1 0-1.8.8-1.8 1.8Z') + `<circle cx="8" cy="8.3" r="1.7" fill="#0b1220" fill-opacity=".28"/>`,
  spark: fl(sparkle(10.5, 12.5, 7.5)) + fl(sparkle(18.5, 5.5, 3), 0.7),
  magnifier: st('M10.5 4a6.5 6.5 0 1 1 0 13a6.5 6.5 0 1 1 0-13Z', 2.4) + st('M15.5 15.5 20.5 20.5', 3) + st('M7.6 9.2c.4-1.4 1.5-2.3 2.9-2.4', 1.6),
  chat: fl('M5 4.5h14a2.5 2.5 0 0 1 2.5 2.5v8a2.5 2.5 0 0 1-2.5 2.5h-8l-4.5 3.5.7-3.5H5A2.5 2.5 0 0 1 2.5 15V7A2.5 2.5 0 0 1 5 4.5Z') +
    `<path d="M6.5 9h11M6.5 13h7" stroke="#0b1220" stroke-opacity=".28" stroke-width="1.8" stroke-linecap="round"/>`,
  star: fl(star(12, 12.6, 9.2)),
  camera: fl('M4.5 7.5h3l1.6-2.2c.3-.4.7-.6 1.2-.6h3.4c.5 0 .9.2 1.2.6l1.6 2.2h3a2 2 0 0 1 2 2v8.5a2 2 0 0 1-2 2h-15a2 2 0 0 1-2-2V9.5a2 2 0 0 1 2-2Z') +
    `<circle cx="12" cy="13.6" r="4" fill="#0b1220" fill-opacity=".28"/><circle cx="12" cy="13.6" r="2.3" fill="#fff"/>`,
  mic: fl('M12 2.8a3.2 3.2 0 0 1 3.2 3.2v5.6a3.2 3.2 0 0 1-6.4 0V6A3.2 3.2 0 0 1 12 2.8Z') + st('M6.3 11.2a5.7 5.7 0 0 0 11.4 0M12 17v3.8M9 20.8h6'),
  music: fl('M9 17.5a2.8 2.8 0 1 1-2.8-2.8c.5 0 1 .1 1.4.3V5.6l11-2.1v11.9a2.8 2.8 0 1 1-2.8-2.8c.5 0 1 .1 1.4.3V8.2L9 9.6Z'),
  calendar: fl('M5 5.5h14a2 2 0 0 1 2 2v11.5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7.5a2 2 0 0 1 2-2Z') +
    `<rect x="3" y="9" width="18" height="1.6" fill="#0b1220" fill-opacity=".25"/>` + st('M8 3.2v4M16 3.2v4', 2.2) +
    `<path d="m9 15 2 2 4-4.2" stroke="#0b1220" stroke-opacity=".45" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>`,
  film: fl('M4 8.5h16v10a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2Z') + fl('m3.6 4.8 15.5-2.6.5 2.9-15.5 2.6Z', 0.75) +
    `<path d="m7.2 4.3 2 2.6M11.4 3.6l2 2.6M15.6 2.9l2 2.6" stroke="#0b1220" stroke-opacity=".3" stroke-width="1.6"/><path d="m10.3 11.8 4.8 2.7-4.8 2.7Z" fill="#0b1220" fill-opacity=".3"/>`,
  handshake: fl('M5 4.5h14a2.5 2.5 0 0 1 2.5 2.5v8a2.5 2.5 0 0 1-2.5 2.5h-8l-4.5 3.5.7-3.5H5A2.5 2.5 0 0 1 2.5 15V7A2.5 2.5 0 0 1 5 4.5Z') +
    `<path d="m8 11 2.8 2.8L16.3 8.3" stroke="#0b1220" stroke-opacity=".38" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" fill="none"/>`,
  first: fl('M10.3 6.6 13.6 4h1.9v15.5h-3V8.2l-2.2 1.6Z') + fl(sparkle(6.5, 15.5, 3), 0.7) + fl(sparkle(19.5, 9, 2.2), 0.6),
  hattrick: fl(star(7, 15.5, 4.6)) + fl(star(17, 15.5, 4.6)) + fl(star(12, 7.4, 5.2)),
  butterfly: fl('M11.4 11.3C9.2 6.4 6.1 4.2 4 4.6c-2.2.4-1.6 4.6.6 6.6.9.8 2.2 1.1 3.5 1-1.8.6-3.1 2.1-2.7 3.9.6 2.6 3.6 3 5.4.3Z') +
    fl('M12.6 11.3c2.2-4.9 5.3-7.1 7.4-6.7 2.2.4 1.6 4.6-.6 6.6-.9.8-2.2 1.1-3.5 1 1.8.6 3.1 2.1 2.7 3.9-.6 2.6-3.6 3-5.4.3Z', 0.8) + st('M12 9v9', 1.8) + st('M12 9c-.6-2-1.6-3.3-2.8-4M12 9c.6-2 1.6-3.3 2.8-4', 1.3),
  mask: fl('M2.8 8.6c2.6-1.6 5.6-2 9.2-.4 3.6-1.6 6.6-1.2 9.2.4.4 3.6-1.2 7.4-4.6 7.8-2.2.3-3.5-.9-4.6-2.6-1.1 1.7-2.4 2.9-4.6 2.6-3.4-.4-5-4.2-4.6-7.8Z') +
    `<ellipse cx="7.6" cy="11.3" rx="2" ry="1.5" fill="#0b1220" fill-opacity=".35"/><ellipse cx="16.4" cy="11.3" rx="2" ry="1.5" fill="#0b1220" fill-opacity=".35"/>`,
  moon: fl('M15.5 3.6A8.6 8.6 0 1 0 20.4 15a7 7 0 0 1-4.9-11.4Z') + fl(sparkle(18.6, 6.4, 2.6), 0.8),
  flame: fl('M12.6 2.6c.8 3.6 5.8 5.6 5.8 11a6.4 6.4 0 0 1-12.8 0c0-2.6 1.2-4.5 2.7-5.8.1 1.8.8 3.2 2.1 3.8-.3-4.1.8-7 2.2-9Z') +
    `<path d="M12.3 12.2c.4 1.9 3.1 2.7 3.1 5.2a3.4 3.4 0 0 1-6.8 0c0-1.4.6-2.3 1.5-3 .2 1 .6 1.4 1.3 1.7-.1-1.8.2-3 .9-3.9Z" fill="#0b1220" fill-opacity=".25"/>`,
  crown: fl('M3.5 8.2 8 12l4-6.5 4 6.5 4.5-3.8-1.8 10H5.3Z') + `<rect x="5.3" y="18.6" width="13.4" height="2.2" rx="1.1" fill="#fff"/>` +
    `<circle cx="12" cy="13.8" r="1.4" fill="#0b1220" fill-opacity=".3"/>`,
};

/** Category palettes for mission tiles. */
const CAT = {
  words: ['#a78bfa', '#6366f1'],
  vibes: ['#fbbf24', '#f97316'],
  curious: ['#22d3ee', '#3b82f6'],
  share: ['#f472b6', '#e11d48'],
  plans: ['#34d399', '#0d9488'],
};
export const categoryColors = (cat) => CAT[cat] || CAT.curious;

/** Medal palettes per achievement badge. */
const MEDAL = {
  first: ['#34d399', '#059669'], hattrick: ['#fbbf24', '#ea580c'], word: ['#a78bfa', '#6d28d9'],
  butterfly: ['#f9a8d4', '#db2777'], mask: ['#818cf8', '#3730a3'], moon: ['#60a5fa', '#1e3a8a'],
  camera: ['#f472b6', '#be123c'], calendar: ['#2dd4bf', '#0f766e'], flame: ['#fb923c', '#dc2626'], crown: ['#fde047', '#ca8a04'],
};

export function badgeGlyph(id) {
  return GLYPHS[id] || GLYPHS.star;
}

/** A rounded gradient tile with the mission's glyph. */
export function missionBadge(id, category, { size = 44 } = {}) {
  const [a, b] = categoryColors(category);
  const g = `mb-${id}-${category}`;
  return raw(`<span class="mission-badge" style="width:${size}px;height:${size}px;--c1:${a};--c2:${b}" aria-hidden="true">
    <svg viewBox="0 0 48 48" width="${size}" height="${size}" xmlns="http://www.w3.org/2000/svg">
      <defs><linearGradient id="${g}" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${a}"/><stop offset="1" stop-color="${b}"/></linearGradient>
      <radialGradient id="${g}s" cx=".3" cy=".2" r=".9"><stop offset="0" stop-color="#fff" stop-opacity=".45"/><stop offset=".6" stop-color="#fff" stop-opacity="0"/></radialGradient></defs>
      <rect x="1" y="1" width="46" height="46" rx="15" fill="url(#${g})"/>
      <rect x="1" y="1" width="46" height="46" rx="15" fill="url(#${g}s)"/>
      <rect x="1.5" y="1.5" width="45" height="45" rx="14.5" fill="none" stroke="#fff" stroke-opacity=".35"/>
      <g transform="translate(12 12)">${badgeGlyph(id)}</g>
    </svg></span>`);
}

/**
 * Achievement medal. a = { badge, unlocked, progress, target, title }.
 * Locked medals are desaturated (CSS) with a ring showing progress.
 */
export function medal(a, { size = 84 } = {}) {
  const [c1, c2] = MEDAL[a.badge] || MEDAL.first;
  const g = `md-${a.badge}`;
  const locked = !a.unlocked;
  const pct = a.target ? Math.max(0, Math.min(1, (a.progress || 0) / a.target)) : 0;
  const R = 41;
  const C = 2 * Math.PI * R;
  return raw(`<span class="medal${locked ? ' locked' : ''}" style="width:${size}px;height:${size}px" aria-hidden="true">
    <svg viewBox="0 0 100 100" width="${size}" height="${size}" xmlns="http://www.w3.org/2000/svg">
      <defs>
        <linearGradient id="${g}" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/></linearGradient>
        <linearGradient id="${g}r" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#fff" stop-opacity=".95"/><stop offset=".5" stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/></linearGradient>
        <radialGradient id="${g}s" cx=".32" cy=".22" r=".85"><stop offset="0" stop-color="#fff" stop-opacity=".6"/><stop offset=".55" stop-color="#fff" stop-opacity="0"/></radialGradient>
      </defs>
      <g class="medal-art">
        <path d="M35 8h12l6 26-9 4Z" fill="${c2}" opacity=".9"/><path d="M65 8H53l-6 26 9 4Z" fill="${c1}"/>
        <circle cx="50" cy="56" r="31" fill="url(#${g}r)"/>
        <circle cx="50" cy="56" r="25.5" fill="url(#${g})"/>
        <circle cx="50" cy="56" r="25.5" fill="url(#${g}s)"/>
        <circle cx="50" cy="56" r="22" fill="none" stroke="#fff" stroke-opacity=".4" stroke-dasharray="1.5 3"/>
        <g transform="translate(35 41) scale(1.25)">${badgeGlyph(a.badge)}</g>
      </g>
      ${locked ? `<circle class="medal-track" cx="50" cy="56" r="${R}" fill="none" stroke-width="4" transform="rotate(-90 50 56)"/>
        <circle class="medal-progress" cx="50" cy="56" r="${R}" fill="none" stroke="${c2}" stroke-width="4" stroke-linecap="round"
          stroke-dasharray="${C.toFixed(1)}" stroke-dashoffset="${(C * (1 - pct)).toFixed(1)}" transform="rotate(-90 50 56)"/>
        <g class="medal-lock" transform="translate(76 82)"><circle r="10" fill="var(--glass-solid, #fff)"/><path d="M-3.6-1.2v-2a3.6 3.6 0 0 1 7.2 0v2" stroke="#64748b" stroke-width="1.8" fill="none"/><rect x="-5" y="-1.4" width="10" height="7.4" rx="2" fill="#64748b"/></g>` : ''}
    </svg></span>`);
}
