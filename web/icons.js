// icons.js — inline SVG sprite of SF-Symbols-style stroke icons (24×24 grid).
// installIcons() injects one hidden <svg> with <symbol id="i-NAME">; use via icon('name').

import { html } from './dom.js';

function gear() {
  const cx = 12, cy = 12, ro = 9.6, ri = 7.4, teeth = 8;
  const pt = (r, deg) => {
    const a = (deg * Math.PI) / 180;
    return `${(cx + r * Math.cos(a)).toFixed(2)} ${(cy + r * Math.sin(a)).toFixed(2)}`;
  };
  let d = '';
  for (let i = 0; i < teeth; i++) {
    const a = (360 / teeth) * i;
    const t0 = a - 13, t1 = a - 8, t2 = a + 8, t3 = a + 13, n0 = a + 360 / teeth - 13;
    d += (i === 0 ? `M${pt(ri, t0)}` : '') + ` L${pt(ro, t1)} L${pt(ro, t2)} L${pt(ri, t3)} A${ri} ${ri} 0 0 1 ${pt(ri, n0)}`;
  }
  return `<path d="${d} Z"/><circle cx="12" cy="12" r="3.1"/>`;
}

const dot = (x, y, r = 1.35) => `<circle cx="${x}" cy="${y}" r="${r}" fill="currentColor" stroke="none"/>`;

const ICONS = {
  dashboard: '<rect x="3.5" y="3.5" width="7.2" height="7.2" rx="2.2"/><rect x="13.3" y="3.5" width="7.2" height="7.2" rx="2.2"/><rect x="3.5" y="13.3" width="7.2" height="7.2" rx="2.2"/><rect x="13.3" y="13.3" width="7.2" height="7.2" rx="2.2"/>',
  chats: '<path d="M14.5 3.8h-9A2.7 2.7 0 0 0 2.8 6.5v5.2a2.7 2.7 0 0 0 2.7 2.7H6v3.1l3.6-3.1h4.9a2.7 2.7 0 0 0 2.7-2.7V6.5a2.7 2.7 0 0 0-2.7-2.7Z"/><path d="M17.2 8.6h1.3a2.7 2.7 0 0 1 2.7 2.7v5.1a2.7 2.7 0 0 1-2.7 2.7H18v2.8l-3.3-2.8h-3.4a2.7 2.7 0 0 1-2.6-2"/>',
  personas: '<circle cx="9" cy="8" r="3.6"/><path d="M2.6 19.6c.6-3.5 3.3-5.7 6.4-5.7s5.8 2.2 6.4 5.7"/><path d="M15.4 4.6a3.5 3.5 0 0 1 0 6.8"/><path d="M17.6 14.2c2.2.7 3.6 2.6 3.9 5.4"/>',
  playground: '<path d="M11.2 3.2 12.9 8l4.8 1.7-4.8 1.7-1.7 4.8-1.7-4.8L4.7 9.7l4.8-1.7Z"/><path d="m18.2 13.6.9 2.4 2.4.9-2.4.9-.9 2.4-.9-2.4-2.4-.9 2.4-.9Z"/><path d="m5.6 15.8.6 1.5 1.5.6-1.5.6-.6 1.5-.6-1.5-1.5-.6 1.5-.6Z"/>',
  sparkles: '<path d="M11.2 3.2 12.9 8l4.8 1.7-4.8 1.7-1.7 4.8-1.7-4.8L4.7 9.7l4.8-1.7Z"/><path d="m18.2 13.6.9 2.4 2.4.9-2.4.9-.9 2.4-.9-2.4-2.4-.9 2.4-.9Z"/><path d="m5.6 15.8.6 1.5 1.5.6-1.5.6-.6 1.5-.6-1.5-1.5-.6 1.5-.6Z"/>',
  activity: '<path d="M2.5 12.5h3.8l2.6-6.5 4.2 12 2.7-7.2 1.4 1.7h4.3"/>',
  approvals: '<path d="M3.2 13.6 5.7 6.4A2.2 2.2 0 0 1 7.8 5h8.4a2.2 2.2 0 0 1 2.1 1.4l2.5 7.2v4.2a2.2 2.2 0 0 1-2.2 2.2H5.4a2.2 2.2 0 0 1-2.2-2.2Z"/><path d="M3.4 13.6h4.8l1.4 2.4h4.8l1.4-2.4h4.8"/><path d="m9.4 9.4 1.9 1.8 3.4-3.5"/>',
  settings: gear(),
  plus: '<path d="M12 5v14M5 12h14"/>',
  minus: '<path d="M5 12h14"/>',
  x: '<path d="M6.5 6.5l11 11M17.5 6.5l-11 11"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  'chevron-right': '<path d="m9.5 5.5 6.5 6.5-6.5 6.5"/>',
  'chevron-left': '<path d="M14.5 5.5 8 12l6.5 6.5"/>',
  'chevron-down': '<path d="m5.5 9.5 6.5 6.5 6.5-6.5"/>',
  'chevron-up': '<path d="m5.5 14.5 6.5-6.5 6.5 6.5"/>',
  'arrow-up': '<path d="M12 19.5v-15M5.5 11 12 4.5 18.5 11"/>',
  'arrow-right': '<path d="M4.5 12h15M13 5.5l6.5 6.5-6.5 6.5"/>',
  'arrow-left': '<path d="M19.5 12h-15M11 5.5 4.5 12l6.5 6.5"/>',
  search: '<circle cx="10.6" cy="10.6" r="6.6"/><path d="m15.6 15.6 4.9 4.9"/>',
  trash: '<path d="M4 6.6h16M9.4 6.6V4.9c0-.8.6-1.4 1.4-1.4h2.4c.8 0 1.4.6 1.4 1.4v1.7M6 6.6l.9 12.1a2 2 0 0 0 2 1.8h6.2a2 2 0 0 0 2-1.8L18 6.6M10 10.6v5.8M14 10.6v5.8"/>',
  copy: '<rect x="8.2" y="8.2" width="12.3" height="12.3" rx="2.8"/><path d="M15.8 8.2V6.3a2.8 2.8 0 0 0-2.8-2.8H6.3a2.8 2.8 0 0 0-2.8 2.8V13a2.8 2.8 0 0 0 2.8 2.8h1.9"/>',
  refresh: '<path d="M19.8 12.4a7.9 7.9 0 1 1-2.5-6.2"/><path d="M19.6 3.8v4.8h-4.8"/>',
  reset: '<path d="M4.2 12.4a7.9 7.9 0 1 0 2.5-6.2"/><path d="M4.4 3.8v4.8h4.8"/>',
  send: '<path d="M20.8 3.2 10.2 13.8"/><path d="m20.8 3.2-6.4 17.6-4.2-7-7-4.2Z"/>',
  edit: '<path d="M4 20h4.2L19.3 8.9a2.9 2.9 0 0 0-4.2-4.2L4 15.8Z"/><path d="m13.6 6.2 4.2 4.2"/>',
  more: dot(5.5, 12, 1.6) + dot(12, 12, 1.6) + dot(18.5, 12, 1.6),
  qr: '<rect x="3.5" y="3.5" width="6.6" height="6.6" rx="1.6"/><rect x="13.9" y="3.5" width="6.6" height="6.6" rx="1.6"/><rect x="3.5" y="13.9" width="6.6" height="6.6" rx="1.6"/><path d="M13.9 13.9h2.8v2.8h-2.8zM17.7 17.7h2.8v2.8h-2.8zM17.7 13.9h2.8M13.9 17.7v2.8"/>',
  phone: '<rect x="6.5" y="2.5" width="11" height="19" rx="2.8"/><path d="M10.4 18.4h3.2"/>',
  link: '<path d="M10 14a4.6 4.6 0 0 0 6.5 0l3-3a4.6 4.6 0 0 0-6.5-6.5l-1.3 1.3"/><path d="M14 10a4.6 4.6 0 0 0-6.5 0l-3 3a4.6 4.6 0 0 0 6.5 6.5l1.3-1.3"/>',
  logout: '<path d="M14 4h3.4A2.6 2.6 0 0 1 20 6.6v10.8a2.6 2.6 0 0 1-2.6 2.6H14"/><path d="M9.8 16.5 5.3 12l4.5-4.5M5.3 12H15"/>',
  power: '<path d="M12 3.2v8"/><path d="M6.6 6.8a7.6 7.6 0 1 0 10.8 0"/>',
  folder: '<path d="M3 7.6A2.6 2.6 0 0 1 5.6 5h3.6l2 2h7.2A2.6 2.6 0 0 1 21 9.6v7.8a2.6 2.6 0 0 1-2.6 2.6H5.6A2.6 2.6 0 0 1 3 17.4Z"/>',
  server: '<rect x="3.5" y="4" width="17" height="7" rx="2.2"/><rect x="3.5" y="13" width="17" height="7" rx="2.2"/><path d="M7 7.5h.01M7 16.5h.01M10 7.5h.01M10 16.5h.01"/>',
  cpu: '<rect x="6" y="6" width="12" height="12" rx="2.8"/><rect x="9.4" y="9.4" width="5.2" height="5.2" rx="1.2"/><path d="M9.5 2.8V6M14.5 2.8V6M9.5 18v3.2M14.5 18v3.2M2.8 9.5H6M2.8 14.5H6M18 9.5h3.2M18 14.5h3.2"/>',
  key: '<circle cx="8" cy="15.2" r="4.6"/><path d="m11.3 11.9 8.4-8.4M17.2 6l2.6 2.6M14.6 8.6l2 2"/>',
  eye: '<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/>',
  'eye-off': '<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/><path d="M4 4l16 16"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2.6v1.8M12 19.6v1.8M2.6 12h1.8M19.6 12h1.8M5.4 5.4l1.3 1.3M17.3 17.3l1.3 1.3M5.4 18.6l1.3-1.3M17.3 6.7l1.3-1.3"/>',
  moon: '<path d="M19.8 14.6A8.3 8.3 0 0 1 9.4 4.2a8.3 8.3 0 1 0 10.4 10.4Z"/>',
  system: '<circle cx="12" cy="12" r="8.5"/><path d="M12 3.5v17a8.5 8.5 0 0 0 0-17Z" fill="currentColor"/>',
  upload: '<path d="M12 15.5V4.2M7.5 8.7 12 4.2l4.5 4.5"/><path d="M4 14.5V18a2.5 2.5 0 0 0 2.5 2.5h11A2.5 2.5 0 0 0 20 18v-3.5"/>',
  download: '<path d="M12 4.2v11.3M7.5 11 12 15.5l4.5-4.5"/><path d="M4 14.5V18a2.5 2.5 0 0 0 2.5 2.5h11A2.5 2.5 0 0 0 20 18v-3.5"/>',
  image: '<rect x="3" y="4.5" width="18" height="15" rx="2.8"/><circle cx="8.6" cy="9.6" r="1.8"/><path d="m3.6 17.2 5-5 4 4 2.8-2.8 5 5"/>',
  smile: '<circle cx="12" cy="12" r="8.6"/><path d="M8.4 14.2a4.6 4.6 0 0 0 7.2 0"/>' + dot(9.2, 9.8, 1.1) + dot(14.8, 9.8, 1.1),
  group: '<circle cx="12" cy="8.2" r="3.1"/><path d="M6.6 19.2c.5-3 2.7-4.9 5.4-4.9s4.9 1.9 5.4 4.9"/><circle cx="5.1" cy="10.2" r="2.2"/><circle cx="18.9" cy="10.2" r="2.2"/><path d="M1.9 17.6c.3-1.8 1.4-3 3-3.3M22.1 17.6c-.3-1.8-1.4-3-3-3.3"/>',
  user: '<circle cx="12" cy="8.4" r="4"/><path d="M4.6 20c.8-4 3.8-6.4 7.4-6.4s6.6 2.4 7.4 6.4"/>',
  bolt: '<path d="M13.2 2.6 4.6 13.4h6.6l-1.2 8 8.6-10.8H12Z"/>',
  shield: '<path d="M12 3 4.6 5.9v5.6c0 4.6 3.1 8.2 7.4 9.5 4.3-1.3 7.4-4.9 7.4-9.5V5.9Z"/><path d="m9 12 2.2 2.2L15.3 10"/>',
  pause: '<path d="M9 5.5v13M15 5.5v13"/>',
  play: '<path d="M7.2 4.8v14.4l11.6-7.2Z"/>',
  filter: '<path d="M4 7h16M7 12h10M10 17h4"/>',
  clock: '<circle cx="12" cy="12" r="8.6"/><path d="M12 7.4V12l3.1 2"/>',
  warning: '<path d="M10.3 4.3 2.9 17.1a2 2 0 0 0 1.7 3h14.8a2 2 0 0 0 1.7-3L13.7 4.3a2 2 0 0 0-3.4 0Z"/><path d="M12 9.6v4"/>' + dot(12, 17, 1.1),
  info: '<circle cx="12" cy="12" r="8.6"/><path d="M12 11v5.4"/>' + dot(12, 7.9, 1.15),
  lock: '<rect x="5" y="10.5" width="14" height="10" rx="2.6"/><path d="M8 10.5V7.6a4 4 0 0 1 8 0v2.9"/>',
  unlock: '<rect x="5" y="10.5" width="14" height="10" rx="2.6"/><path d="M8 10.5V7.6a4 4 0 0 1 7.7-1.6"/>',
  external: '<path d="M14 4h6v6M20 4l-8.5 8.5"/><path d="M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4"/>',
  sliders: '<path d="M4 7h9M17 7h3M4 17h3M11 17h9"/><circle cx="15" cy="7" r="2.1"/><circle cx="9" cy="17" r="2.1"/>',
  message: '<path d="M5.6 4.5h12.8A2.6 2.6 0 0 1 21 7.1v7.8a2.6 2.6 0 0 1-2.6 2.6H10L5.5 21v-3.5A2.6 2.6 0 0 1 3 14.9V7.1a2.6 2.6 0 0 1 2.6-2.6Z"/>',
  wand: '<path d="m4 20 10.5-10.5"/><path d="m12.8 8 3.2 3.2"/><path d="M17.5 2.8v2.8M16.1 4.2h2.8M20.2 7.6v2M19.2 8.6h2M9 3v1.6M8.2 3.8h1.6"/>',
  globe: '<circle cx="12" cy="12" r="8.6"/><path d="M3.4 12h17.2M12 3.4c2.4 2.5 3.5 5.4 3.5 8.6s-1.1 6.1-3.5 8.6c-2.4-2.5-3.5-5.4-3.5-8.6s1.1-6.1 3.5-8.6Z"/>',
  target: '<circle cx="12" cy="12" r="8.6"/><circle cx="12" cy="12" r="5"/>' + dot(12, 12, 1.6),
  text: '<path d="M6 6.5h12M6 11.5h12M6 16.5h7.5"/>',
  quote: '<path d="M9.5 7.5H6.4A1.9 1.9 0 0 0 4.5 9.4v2.8c0 1 .9 1.9 1.9 1.9H8.8v.6c0 1.6-.9 2.6-2.6 3.1M19.5 7.5h-3.1a1.9 1.9 0 0 0-1.9 1.9v2.8c0 1 .9 1.9 1.9 1.9h2.4v.6c0 1.6-.9 2.6-2.6 3.1"/>',
  ruler: '<rect x="2.8" y="7.5" width="18.4" height="9" rx="2"/><path d="M7 7.5v3M11 7.5v4.5M15 7.5v3M19 7.5v4.5"/>',
  heart: '<path d="M12 20s-7.8-4.6-7.8-10.4A4.3 4.3 0 0 1 12 7a4.3 4.3 0 0 1 7.8 2.6C19.8 15.4 12 20 12 20Z"/>',
  star: '<path d="m12 3.6 2.6 5.3 5.8.8-4.2 4.1 1 5.8L12 16.9l-5.2 2.7 1-5.8-4.2-4.1 5.8-.8Z"/>',
  at: '<circle cx="12" cy="12" r="3.6"/><path d="M15.6 12v1.5a2.6 2.6 0 0 0 5.2 0V12a8.8 8.8 0 1 0-3.5 7"/>',
  brain: '<path d="M9.2 4.2a2.8 2.8 0 0 0-2.8 2.7 2.8 2.8 0 0 0-2.4 4.3 3 3 0 0 0 .8 5 2.9 2.9 0 0 0 4.4 2.9 2.4 2.4 0 0 0 2.8-.8V5.6a2.4 2.4 0 0 0-2.8-1.4Z"/><path d="M14.8 4.2a2.8 2.8 0 0 1 2.8 2.7 2.8 2.8 0 0 1 2.4 4.3 3 3 0 0 1-.8 5 2.9 2.9 0 0 1-4.4 2.9 2.4 2.4 0 0 1-2.8-.8"/><path d="M8.6 9.6c.9 0 1.6.5 2 1.2M15.4 9.6c-.9 0-1.6.5-2 1.2M8.8 14.4a2 2 0 0 0 2-1.4M15.2 14.4a2 2 0 0 1-2-1.4"/>',
  typing: dot(7, 12, 1.5) + dot(12, 12, 1.5) + dot(17, 12, 1.5) + '<path d="M5.6 5h12.8A2.6 2.6 0 0 1 21 7.6v7.8a2.6 2.6 0 0 1-2.6 2.6H10L5.5 21.5V18A2.6 2.6 0 0 1 3 15.4V7.6A2.6 2.6 0 0 1 5.6 5Z"/>',
  hand: '<path d="M8 12.5V5.8a1.6 1.6 0 0 1 3.2 0v5.2M11.2 10.6V4.4a1.6 1.6 0 0 1 3.2 0v6.2M14.4 10.8V6a1.6 1.6 0 0 1 3.2 0v7.5c0 4-2.6 7-6.2 7-2.6 0-4-1-5.4-3.2l-2.3-3.7a1.6 1.6 0 0 1 2.6-1.9L8 13.6"/>',
  stop: '<rect x="6" y="6" width="12" height="12" rx="2.4"/>',
  inbox: '<path d="M3.2 13.6 5.7 6.4A2.2 2.2 0 0 1 7.8 5h8.4a2.2 2.2 0 0 1 2.1 1.4l2.5 7.2v4.2a2.2 2.2 0 0 1-2.2 2.2H5.4a2.2 2.2 0 0 1-2.2-2.2Z"/><path d="M3.4 13.6h4.8l1.4 2.4h4.8l1.4-2.4h4.8"/>',
  wifi: '<path d="M2.5 9a14 14 0 0 1 19 0M5.5 12.4a9.5 9.5 0 0 1 13 0M8.6 15.7a5 5 0 0 1 6.8 0"/>' + dot(12, 19, 1.4),
  pin: '<path d="M12 21s6.5-5.6 6.5-11a6.5 6.5 0 0 0-13 0c0 5.4 6.5 11 6.5 11Z"/><circle cx="12" cy="10" r="2.4"/>',
  'check-double': '<path d="m2.6 12.9 4 4L15.4 7.6"/><path d="m11.4 16.1.8.8 8.8-9.3"/>',
  snooze: '<path d="M17.4 16.6A7.4 7.4 0 0 1 8.2 7.4a7.4 7.4 0 1 0 9.2 9.2Z"/><path d="M14.4 3.4h5.1l-5.1 5.4h5.1"/>',
  calendar: '<rect x="3.5" y="5" width="17" height="15.5" rx="3"/><path d="M3.5 10h17M8 3v4M16 3v4"/>' + dot(8.2, 14.2, 1.05) + dot(12, 14.2, 1.05) + dot(15.8, 14.2, 1.05),
  coffee: '<path d="M4.5 9.5h12v5.2a5 5 0 0 1-5 5h-2a5 5 0 0 1-5-5Z"/><path d="M16.5 11h1.6a2.4 2.4 0 0 1 0 4.8h-1.9"/><path d="M8.6 3.4c-.9 1 .9 1.9 0 3M12.6 3.4c-.9 1 .9 1.9 0 3"/>',
  reply: '<path d="M9.6 6.2 4.2 11.6 9.6 17"/><path d="M4.6 11.6h8.9a6.3 6.3 0 0 1 6.3 6.3v.9"/>',
  dice: '<rect x="3.8" y="3.8" width="16.4" height="16.4" rx="4.2"/>' + dot(8.6, 8.6, 1.3) + dot(12, 12, 1.3) + dot(15.4, 15.4, 1.3),
  timer: '<circle cx="12" cy="13.6" r="7.4"/><path d="M12 9.6v4.2l2.6 1.6M9.4 2.8h5.2M18.4 6.6l1.5-1.5"/>',
  bubbles: '<path d="M4.6 3.6h8.6a2.5 2.5 0 0 1 2.5 2.5v3a2.5 2.5 0 0 1-2.5 2.5H8.3l-3.2 2.6v-2.6h-.5a2.5 2.5 0 0 1-2.5-2.5v-3a2.5 2.5 0 0 1 2.5-2.5Z"/><path d="M11 14.2h8.4a2.5 2.5 0 0 1 2.5 2.5v1.2a2.5 2.5 0 0 1-2.5 2.5h-.3v2.3l-3-2.3H11a2.5 2.5 0 0 1-2.5-2.5v-1.2a2.5 2.5 0 0 1 2.5-2.5Z"/>',
  megaphone: '<path d="M3.6 10.1v3.8A1.6 1.6 0 0 0 5.2 15.5h2.1l7.6 4.2V4.3L7.3 8.5H5.2a1.6 1.6 0 0 0-1.6 1.6Z"/><path d="M18.2 9a4.2 4.2 0 0 1 0 6"/><path d="m7.6 15.6 1.3 4.6"/>',
  gauge: '<path d="M3.9 17.2a8.6 8.6 0 1 1 16.2 0"/><path d="m12 13.4 4.2-4.6"/>' + dot(12, 13.4, 1.5),
  hourglass: '<path d="M6.5 3.5h11M6.5 20.5h11M7.5 3.5c0 4.3 4.5 5.5 4.5 8.5s-4.5 4.2-4.5 8.5M16.5 3.5c0 4.3-4.5 5.5-4.5 8.5s4.5 4.2 4.5 8.5"/>',
};

export function installIcons() {
  if (document.getElementById('doppel-sprite')) return;
  const symbols = Object.entries(ICONS)
    .map(([k, v]) => `<symbol id="i-${k}" viewBox="0 0 24 24">${v}</symbol>`)
    .join('');
  // Not display:none — gradients inside display:none SVGs don't paint in Chromium.
  const svg = `<svg id="doppel-sprite" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" style="position:absolute;width:0;height:0;overflow:hidden">
    <defs>
      <linearGradient id="ring-grad" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" style="stop-color:var(--brand-1)"/><stop offset=".55" style="stop-color:var(--brand-2)"/><stop offset="1" style="stop-color:var(--brand-3)"/>
      </linearGradient>
      <linearGradient id="brand-grad" x1="0" y1="0" x2="1" y2="1">
        <stop offset="0" style="stop-color:var(--brand-1)"/><stop offset=".55" style="stop-color:var(--brand-2)"/><stop offset="1" style="stop-color:var(--brand-3)"/>
      </linearGradient>
    </defs>${symbols}</svg>`;
  document.body.insertAdjacentHTML('afterbegin', svg);
}

export function icon(name, cls = '') {
  return html`<svg class=${'ic ' + cls} aria-hidden="true" focusable="false"><use href=${'#i-' + name}></use></svg>`;
}

export const ICON_NAMES = Object.keys(ICONS);
