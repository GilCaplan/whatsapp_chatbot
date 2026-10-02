// guide-art.js — inline-SVG illustrations for the Guide (static, trusted).
// Same visual language as art.js: soft gradients, white glass cards, a drop
// shadow and a few sparkles. Never emoji.

import { raw } from '../dom.js';

const defs = `<defs>
  <linearGradient id="ga-a" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-a1, #34d399)"/><stop offset="1" style="stop-color:var(--art-a2, #14b8c4)"/></linearGradient>
  <linearGradient id="ga-b" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-b1, #a78bfa)"/><stop offset="1" style="stop-color:var(--art-b2, #f472b6)"/></linearGradient>
  <linearGradient id="ga-c" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-c1, #22d3ee)"/><stop offset="1" style="stop-color:var(--art-c2, #8b5cf6)"/></linearGradient>
  <linearGradient id="ga-o" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-o1, #fbbf24)"/><stop offset="1" style="stop-color:var(--art-o2, #f43f5e)"/></linearGradient>
  <linearGradient id="ga-w" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-opacity=".97" style="stop-color:var(--art-paper, #fff)"/><stop offset="1" stop-opacity=".74" style="stop-color:var(--art-paper, #fff)"/></linearGradient>
  <linearGradient id="ga-s" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:var(--art-s1, #e2e8f0)"/><stop offset="1" style="stop-color:var(--art-s2, #cbd5e1)"/></linearGradient>
  <filter id="ga-sh" x="-20%" y="-20%" width="140%" height="160%"><feDropShadow dx="0" dy="6" stdDeviation="6" flood-opacity=".18" style="flood-color:var(--art-ink, #0b1220)"/></filter>
</defs>`;

const shadow = (cx = 120, w = 84) => `<ellipse cx="${cx}" cy="140" rx="${w}" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>`;
const spark = (x, y, r, fill) => `<path d="M${x} ${y - r}l${r * 0.3} ${r * 0.7} ${r * 0.7} ${r * 0.3}-${r * 0.7} ${r * 0.3}-${r * 0.3} ${r * 0.7}-${r * 0.3}-${r * 0.7}-${r * 0.7}-${r * 0.3} ${r * 0.7}-${r * 0.3}Z" style="fill:${fill}"/>`;
const svg = (body) => `<svg viewBox="0 0 240 150" xmlns="http://www.w3.org/2000/svg">${defs}${body}</svg>`;
const face = (cx, cy, s = 1, col = 'var(--art-w, #fff)') => `<g style="fill:${col}"><circle cx="${cx - 7 * s}" cy="${cy - 3 * s}" r="${3.4 * s}"/><circle cx="${cx + 7 * s}" cy="${cy - 3 * s}" r="${3.4 * s}"/></g>` +
  `<path d="M${cx - 7 * s} ${cy + 6 * s}c${4 * s} ${5 * s} ${10 * s} ${5 * s} ${14 * s} 0" style="stroke:${col}" stroke-width="${3.4 * s}" stroke-linecap="round" fill="none"/>`;

const ARTS = {
  // You and your persona: two friends, one of them made of light.
  welcome: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="44" y="30" width="72" height="92" rx="30" fill="url(#ga-s)"/>
      <rect x="124" y="22" width="72" height="92" rx="30" fill="url(#ga-a)"/>
    </g>
    ${face(80, 72, 1, 'var(--art-face, #64748b)')}
    ${face(160, 64)}
    <path d="M116 46c4-8 8-8 12 0" stroke-width="3" stroke-linecap="round" stroke-dasharray="1 6" fill="none" style="stroke:var(--art-c2, #8b5cf6)"/>
    <g filter="url(#ga-sh)"><path d="M168 8h44a10 10 0 0 1 10 10v12a10 10 0 0 1-10 10h-26l-10 8 2-8h-10a10 10 0 0 1-10-10V18a10 10 0 0 1 10-10Z" fill="url(#ga-w)"/></g>
    <g fill="url(#ga-a)"><circle cx="178" cy="24" r="3.4"/><circle cx="190" cy="24" r="3.4" opacity=".7"/><circle cx="202" cy="24" r="3.4" opacity=".45"/></g>
    ${spark(214, 66, 7, 'var(--art-o1, #fbbf24)')}${spark(30, 40, 5, 'var(--art-b1, #a78bfa)')}${spark(206, 104, 4, 'var(--art-a1, #34d399)')}`),

  // Phone ↔ computer link, a brain writing the reply.
  how: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="30" y="26" width="52" height="96" rx="14" fill="url(#ga-a)"/>
      <rect x="128" y="34" width="86" height="60" rx="10" fill="url(#ga-c)"/>
      <path d="M118 100h106l-8 14H126Z" fill="url(#ga-s)"/>
    </g>
    <rect x="36" y="36" width="40" height="74" rx="7" opacity=".92" style="fill:var(--art-paper, #fff)"/>
    <path d="M42 50h22a5 5 0 0 1 5 5v2a5 5 0 0 1-5 5H48l-6 4Z" fill="url(#ga-a)" opacity=".85"/>
    <path d="M70 74H50a5 5 0 0 0-5 5v2a5 5 0 0 0 5 5h14l6 4Z" fill="url(#ga-c)" opacity=".8"/>
    <rect x="134" y="40" width="74" height="48" rx="6" opacity=".9" style="fill:var(--art-paper, #fff)"/>
    <path d="M86 70h36" stroke-width="4" stroke-linecap="round" stroke-dasharray="2 8" style="stroke:var(--art-c2, #8b5cf6)"/>
    <circle cx="104" cy="70" r="9" fill="url(#ga-b)"/><path d="m100 70 3 3 5-6" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    <g transform="translate(155 46)">
      <path d="M16 4c-5 0-8 3-8 7-3 1-4 3-4 6s2 5 4 6c0 4 3 7 8 7h1V4Z" fill="url(#ga-b)"/>
      <path d="M17 4c5 0 8 3 8 7 3 1 4 3 4 6s-2 5-4 6c0 4-3 7-8 7Z" fill="url(#ga-c)"/>
      <path d="M12 12c2 0 3 1 3 3M22 12c-2 0-3 1-3 3M11 21c2 1 4 1 6-1M23 21c-2 1-4 1-6-1" stroke-width="1.8" stroke-linecap="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    </g>
    ${spark(222, 24, 6, 'var(--art-o1, #fbbf24)')}${spark(18, 20, 4.5, 'var(--art-b1, #a78bfa)')}`),

  // A fan of character cards.
  personas: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="40" y="34" width="64" height="86" rx="20" fill="url(#ga-o)" transform="rotate(-12 72 77)"/>
      <rect x="136" y="34" width="64" height="86" rx="20" fill="url(#ga-a)" transform="rotate(12 168 77)"/>
      <rect x="88" y="22" width="64" height="88" rx="20" fill="url(#ga-b)"/>
    </g>
    ${face(120, 60)}
    <g transform="rotate(-12 72 77)">${face(72, 70, 0.8)}</g>
    <g transform="rotate(12 168 77)">${face(168, 70, 0.8)}</g>
    <path d="M100 92h40M104 100h32" stroke-width="4" stroke-linecap="round" opacity=".75" style="stroke:var(--art-w, #fff)"/>
    ${spark(206, 22, 7, 'var(--art-o1, #fbbf24)')}${spark(30, 26, 5, 'var(--art-a1, #34d399)')}`),

  // A chat list with a persona chip landing on one row.
  assign: svg(`${shadow()}
    <g filter="url(#ga-sh)"><rect x="34" y="18" width="150" height="112" rx="18" fill="url(#ga-w)"/></g>
    <g>
      <circle cx="56" cy="42" r="10" fill="url(#ga-s)"/><path d="M74 38h60M74 47h36" stroke-width="5" stroke-linecap="round" style="stroke:var(--art-s2, #cbd5e1)"/>
      <rect x="42" y="62" width="134" height="30" rx="10" opacity=".12" style="fill:var(--art-a2, #14b8c4)"/>
      <circle cx="56" cy="77" r="10" fill="url(#ga-a)"/><path d="M74 73h52M74 82h30" stroke-opacity=".45" stroke-width="5" stroke-linecap="round" style="stroke:var(--art-deep, #0f766e)"/>
      <rect x="146" y="70" width="22" height="14" rx="7" style="fill:var(--art-green, #34c759)"/><circle cx="161" cy="77" r="5" style="fill:var(--art-w, #fff)"/>
      <circle cx="56" cy="110" r="10" fill="url(#ga-s)"/><path d="M74 106h48M74 115h40" stroke-width="5" stroke-linecap="round" style="stroke:var(--art-s2, #cbd5e1)"/>
    </g>
    <g filter="url(#ga-sh)"><rect x="160" y="28" width="62" height="30" rx="15" fill="url(#ga-b)"/></g>
    <circle cx="176" cy="43" r="9" opacity=".95" style="fill:var(--art-w, #fff)"/>${face(176, 43, 0.45, 'var(--art-violet, #a855f7)')}
    <path d="M190 39h22M190 47h14" stroke-width="4" stroke-linecap="round" style="stroke:var(--art-w, #fff)"/>
    <path d="M178 60c-4 6-10 10-18 12" stroke-width="3" stroke-linecap="round" stroke-dasharray="2 6" fill="none" style="stroke:var(--art-b1, #a78bfa)"/>
    ${spark(222, 92, 6, 'var(--art-o1, #fbbf24)')}`),

  // Three dials at different levels.
  dials: svg(`${shadow()}
    ${[[52, 'var(--art-b1, #a78bfa)', 'var(--art-indigo, #6366f1)', 0.25], [120, 'var(--art-a1, #34d399)', 'var(--art-sky, #0ea5e9)', 0.6], [188, 'var(--art-orange, #fb923c)', 'var(--art-o2, #f43f5e)', 0.9]].map(([cx, a, b, f], i) => {
      const r = 28;
      const len = Math.PI * r * 1.5;
      return `<linearGradient id="ga-d${i}" x1="0" y1="1" x2="1" y2="0"><stop offset="0" style="stop-color:${a}"/><stop offset="1" style="stop-color:${b}"/></linearGradient>
      <path d="M${cx - r * 0.707} ${76 + r * 0.707}A${r} ${r} 0 1 1 ${cx + r * 0.707} ${76 + r * 0.707}" stroke-opacity=".25" stroke-width="7" stroke-linecap="round" fill="none" style="stroke:var(--art-line, #94a3b8)"/>
      <path d="M${cx - r * 0.707} ${76 + r * 0.707}A${r} ${r} 0 1 1 ${cx + r * 0.707} ${76 + r * 0.707}" stroke="url(#ga-d${i})" stroke-width="7" stroke-linecap="round" fill="none" stroke-dasharray="${len.toFixed(1)}" stroke-dashoffset="${(len * (1 - f)).toFixed(1)}"/>
      <g filter="url(#ga-sh)"><circle cx="${cx}" cy="76" r="19" fill="url(#ga-d${i})"/></g>
      <rect x="${cx - 2}" y="58" width="4" height="9" rx="2" transform="rotate(${-135 + 270 * f} ${cx} 76)" style="fill:var(--art-w, #fff)"/>`;
    }).join('')}
    <path d="M38 124h28M106 124h28M174 124h28" stroke-width="4" stroke-linecap="round" opacity=".45" style="stroke:var(--art-line, #94a3b8)"/>
    ${spark(222, 30, 6, 'var(--art-o1, #fbbf24)')}${spark(20, 34, 4.5, 'var(--art-a1, #34d399)')}`),

  // Target, flag and a medal.
  missions: svg(`${shadow()}
    <g filter="url(#ga-sh)"><circle cx="104" cy="76" r="50" fill="url(#ga-w)"/></g>
    <circle cx="104" cy="76" r="50" stroke="url(#ga-c)" stroke-width="7" fill="none"/>
    <circle cx="104" cy="76" r="32" stroke="url(#ga-b)" stroke-width="7" fill="none"/>
    <circle cx="104" cy="76" r="13" fill="url(#ga-a)"/>
    <path d="M108 72 152 28" stroke-width="4" stroke-linecap="round" style="stroke:var(--art-ink-2, #334155)"/>
    <path d="m152 28 4-15 7 11 13 0-9 9Z" fill="url(#ga-b)"/>
    <g filter="url(#ga-sh)" transform="translate(170 76)">
      <path d="M-12-24h9l3 12 3-12h9l-6 20H-6Z" fill="url(#ga-c)"/>
      <circle cx="0" cy="12" r="20" fill="url(#ga-o)"/>
      <circle cx="0" cy="12" r="14" fill="none" stroke-opacity=".6" stroke-width="2" style="stroke:var(--art-w, #fff)"/>
      <path d="m0 3 2.8 5.8 6.3.9-4.6 4.4 1.1 6.3L0 17.4l-5.6 3 1.1-6.3-4.6-4.4 6.3-.9Z" style="fill:var(--art-w, #fff)"/>
    </g>
    ${spark(36, 30, 6, 'var(--art-o1, #fbbf24)')}${spark(214, 26, 4.5, 'var(--art-b1, #a78bfa)')}`),

  // Auto, Approve, Co-pilot as three tiles.
  modes: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="20" y="40" width="62" height="74" rx="18" fill="url(#ga-a)"/>
      <rect x="89" y="28" width="62" height="86" rx="18" fill="url(#ga-w)"/>
      <rect x="158" y="40" width="62" height="74" rx="18" fill="url(#ga-b)"/>
    </g>
    <path d="M44 64v24l20-12Z" style="fill:var(--art-w, #fff)"/>
    <circle cx="120" cy="62" r="17" fill="url(#ga-a)"/><path d="m112 62 6 6 10-11" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    <path d="M104 92h32M110 101h20" stroke-width="5" stroke-linecap="round" style="stroke:var(--art-s2, #cbd5e1)"/>
    <g style="fill:var(--art-w, #fff)"><rect x="168" y="54" width="42" height="12" rx="6" opacity=".55"/><rect x="168" y="71" width="42" height="12" rx="6"/><rect x="168" y="88" width="42" height="12" rx="6" opacity=".55"/></g>
    <circle cx="176" cy="77" r="3" style="fill:var(--art-violet, #a855f7)"/>
    ${spark(120, 14, 6, 'var(--art-o1, #fbbf24)')}`),

  // The persona hands the phone back to you.
  handoff: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="92" y="22" width="56" height="100" rx="16" fill="url(#ga-c)"/>
    </g>
    <rect x="98" y="32" width="44" height="78" rx="9" opacity=".92" style="fill:var(--art-paper, #fff)"/>
    <path d="M104 44h24a5 5 0 0 1 5 5v1a5 5 0 0 1-5 5h-18l-6 4Z" style="fill:var(--art-s2, #cbd5e1)"/>
    <circle cx="120" cy="82" r="14" fill="url(#ga-o)"/><path d="M116 76v12M124 76v12" stroke-width="4" stroke-linecap="round" style="stroke:var(--art-w, #fff)"/>
    <g filter="url(#ga-sh)"><rect x="22" y="48" width="56" height="70" rx="24" fill="url(#ga-b)"/></g>${face(50, 80, 0.8)}
    <g filter="url(#ga-sh)"><rect x="162" y="48" width="56" height="70" rx="24" fill="url(#ga-s)"/></g>${face(190, 80, 0.8, 'var(--art-face, #64748b)')}
    <path d="M78 66c6-8 10-10 14-10M148 56c6 0 10 2 14 10" stroke-width="3.4" stroke-linecap="round" fill="none" style="stroke:var(--art-o2, #f43f5e)"/>
    <path d="m156 60 7 6-9 3" stroke-width="3.4" stroke-linecap="round" stroke-linejoin="round" fill="none" style="stroke:var(--art-o2, #f43f5e)"/>
    <circle cx="146" cy="24" r="8" style="fill:var(--art-red, #ff3b30)"/><path d="M146 20v5" stroke-width="2.6" stroke-linecap="round" style="stroke:var(--art-w, #fff)"/><circle cx="146" cy="28" r="1.4" style="fill:var(--art-w, #fff)"/>`),

  // A mask lifted to show the friend behind it.
  reveal: svg(`${shadow()}
    <g filter="url(#ga-sh)"><rect x="84" y="34" width="72" height="92" rx="30" fill="url(#ga-s)"/></g>
    ${face(120, 80, 1, 'var(--art-face, #64748b)')}
    <g filter="url(#ga-sh)" transform="rotate(-18 150 40)">
      <path d="M112 30c10-8 22-8 38-2 16-6 28-6 38 2 2 12-4 24-16 26-10 1-16-4-22-10-6 6-12 11-22 10-12-2-18-14-16-26Z" fill="url(#ga-b)"/>
      <ellipse cx="132" cy="38" rx="8" ry="6" opacity=".9" style="fill:var(--art-paper, #fff)"/><ellipse cx="168" cy="38" rx="8" ry="6" opacity=".9" style="fill:var(--art-paper, #fff)"/>
    </g>
    <path d="M176 92h40a8 8 0 0 1 8 8v8a8 8 0 0 1-8 8h-24l-8 7 2-7h-10a8 8 0 0 1-8-8v-8a8 8 0 0 1 8-8Z" fill="url(#ga-w)" filter="url(#ga-sh)"/>
    <path d="M186 102h26M186 110h16" stroke-width="4" stroke-linecap="round" style="stroke:var(--art-b1, #a78bfa)"/>
    ${spark(40, 40, 7, 'var(--art-o1, #fbbf24)')}${spark(60, 104, 5, 'var(--art-b2, #f472b6)')}${spark(212, 22, 5, 'var(--art-a1, #34d399)')}`),

  // A notebook of small things people said.
  memory: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="50" y="22" width="96" height="110" rx="16" fill="url(#ga-w)" transform="rotate(-5 98 77)"/>
      <path d="M150 38h44a12 12 0 0 1 12 12v18a12 12 0 0 1-12 12h-16l-12 10 2-10h-18a12 12 0 0 1-12-12V50a12 12 0 0 1 12-12Z" fill="url(#ga-b)"/>
    </g>
    <circle cx="96" cy="26" r="8" fill="url(#ga-o)"/>
    <g transform="rotate(-5 98 77)" stroke-linecap="round" stroke-width="5">
      <path d="M66 52h40" stroke="url(#ga-a)"/><path d="M66 68h56M66 84h48M66 100h34" opacity=".55" style="stroke:var(--art-line, #94a3b8)"/>
    </g>
    <path d="m172 49 3.4 7 7.6 1.1-5.5 5.4 1.3 7.6-6.8-3.6-6.8 3.6 1.3-7.6-5.5-5.4 7.6-1.1Z" style="fill:var(--art-w, #fff)"/>
    ${spark(30, 100, 6, 'var(--art-a1, #34d399)')}${spark(214, 112, 5, 'var(--art-o1, #fbbf24)')}`),

  // A bubble with a question mark.
  faq: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <path d="M60 20h86a20 20 0 0 1 20 20v40a20 20 0 0 1-20 20H96l-24 20 4-20H60a20 20 0 0 1-20-20V40a20 20 0 0 1 20-20Z" fill="url(#ga-c)"/>
      <path d="M160 62h40a14 14 0 0 1 14 14v18a14 14 0 0 1-14 14h-6l2 14-16-14h-20a14 14 0 0 1-14-14V76a14 14 0 0 1 14-14Z" fill="url(#ga-w)"/>
    </g>
    <path d="M90 50c0-9 6-14 14-14s14 5 14 12c0 10-12 10-12 20" stroke-width="8" stroke-linecap="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    <circle cx="106" cy="83" r="5.5" style="fill:var(--art-w, #fff)"/>
    <path d="M170 80h30M170 92h20" stroke="url(#ga-a)" stroke-width="5" stroke-linecap="round"/>
    ${spark(28, 34, 6, 'var(--art-o1, #fbbf24)')}${spark(220, 40, 5, 'var(--art-b1, #a78bfa)')}`),

  // A heart in a bubble between two friends.
  kind: svg(`${shadow()}
    <g filter="url(#ga-sh)">
      <rect x="28" y="54" width="56" height="70" rx="24" fill="url(#ga-a)"/>
      <rect x="156" y="54" width="56" height="70" rx="24" fill="url(#ga-b)"/>
      <path d="M96 18h48a14 14 0 0 1 14 14v26a14 14 0 0 1-14 14h-16l-8 10-8-10H96a14 14 0 0 1-14-14V32a14 14 0 0 1 14-14Z" fill="url(#ga-w)"/>
    </g>
    ${face(56, 86, 0.8)}${face(184, 86, 0.8)}
    <path d="M120 60c-14-9-20-16-20-23 0-6 4-10 10-10 4 0 8 2 10 6 2-4 6-6 10-6 6 0 10 4 10 10 0 7-6 14-20 23Z" fill="url(#ga-o)"/>
    ${spark(32, 30, 5, 'var(--art-o1, #fbbf24)')}${spark(212, 28, 6, 'var(--art-a1, #34d399)')}`),
};

/** An illustration by id (falls back to the welcome picture). */
export function guideArt(name, cls = '') {
  return raw(`<span class="guide-art ${cls}" aria-hidden="true">${ARTS[name] || ARTS.welcome}</span>`);
}

export const GUIDE_ARTS = Object.keys(ARTS);
