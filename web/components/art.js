// art.js — friendly inline-SVG illustrations for empty states (static, trusted).

import { html, raw } from '../dom.js';
import { icon } from '../icons.js';

const defs = `<defs>
  <linearGradient id="art-a" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-a1, #34d399)"/><stop offset="1" style="stop-color:var(--art-a2, #14b8c4)"/></linearGradient>
  <linearGradient id="art-b" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-b1, #a78bfa)"/><stop offset="1" style="stop-color:var(--art-b2, #f472b6)"/></linearGradient>
  <linearGradient id="art-c" x1="0" y1="0" x2="1" y2="1"><stop offset="0" style="stop-color:var(--art-c1, #22d3ee)"/><stop offset="1" style="stop-color:var(--art-c2, #8b5cf6)"/></linearGradient>
  <linearGradient id="art-d" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-opacity=".95" style="stop-color:var(--art-paper, #fff)"/><stop offset="1" stop-opacity=".7" style="stop-color:var(--art-paper, #fff)"/></linearGradient>
  <filter id="art-sh" x="-20%" y="-20%" width="140%" height="160%"><feDropShadow dx="0" dy="6" stdDeviation="6" flood-opacity=".18" style="flood-color:var(--art-ink, #0b1220)"/></filter>
</defs>`;

const ARTS = {
  chats: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <path d="M96 18h52a14 14 0 0 1 14 14v26a14 14 0 0 1-14 14h-6l2 14-16-14H96a14 14 0 0 1-14-14V32a14 14 0 0 1 14-14Z" fill="url(#art-c)"/>
      <path d="M32 46h56a14 14 0 0 1 14 14v28a14 14 0 0 1-14 14H52l-18 14 3-14h-5a14 14 0 0 1-14-14V60a14 14 0 0 1 14-14Z" fill="url(#art-d)"/>
    </g>
    <circle cx="44" cy="74" r="5" fill="url(#art-a)"/><circle cx="60" cy="74" r="5" fill="url(#art-a)" opacity=".75"/><circle cx="76" cy="74" r="5" fill="url(#art-a)" opacity=".5"/>
    <path d="M100 38h40M100 50h26" stroke-width="5" stroke-linecap="round" opacity=".85" style="stroke:var(--art-w, #fff)"/>
  </svg>`,
  personas: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <rect x="28" y="26" width="64" height="80" rx="22" fill="url(#art-b)" transform="rotate(-8 60 66)"/>
      <rect x="88" y="22" width="64" height="80" rx="22" fill="url(#art-a)" transform="rotate(7 120 62)"/>
    </g>
    <g style="fill:var(--art-w, #fff)"><circle cx="50" cy="60" r="4.5"/><circle cx="68" cy="57" r="4.5"/><circle cx="110" cy="56" r="4.5"/><circle cx="128" cy="58" r="4.5"/></g>
    <path d="M52 76c6 6 14 5 19-1M110 72c6 5 14 6 19 1" stroke-width="4" stroke-linecap="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    <path d="M150 20l3 7 7 3-7 3-3 7-3-7-7-3 7-3Z" style="fill:var(--art-o1, #fbbf24)"/>
  </svg>`,
  approvals: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <path d="M42 64 54 34a10 10 0 0 1 9-6h54a10 10 0 0 1 9 6l12 30v34a10 10 0 0 1-10 10H52a10 10 0 0 1-10-10Z" fill="url(#art-d)"/>
      <path d="M42 64h28l6 12h28l6-12h28v34a10 10 0 0 1-10 10H52a10 10 0 0 1-10-10Z" fill="url(#art-a)"/>
    </g>
    <circle cx="90" cy="46" r="15" fill="url(#art-a)"/>
    <path d="m83 46 5 5 10-10" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" fill="none" style="stroke:var(--art-w, #fff)"/>
    <path d="M30 26l2.5 6 6 2.5-6 2.5-2.5 6-2.5-6-6-2.5 6-2.5Z M150 30l2 5 5 2-5 2-2 5-2-5-5-2 5-2Z" style="fill:var(--art-b1, #a78bfa)"/>
  </svg>`,
  activity: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)"><rect x="26" y="26" width="128" height="84" rx="20" fill="url(#art-d)"/></g>
    <path d="M40 72h22l9-20 14 36 10-24 6 8h39" stroke="url(#art-c)" stroke-width="6" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
    <text x="134" y="34" font-family="-apple-system, sans-serif" font-weight="800" font-size="18" style="fill:var(--art-b1, #a78bfa)">z</text>
    <text x="146" y="22" font-family="-apple-system, sans-serif" font-weight="800" font-size="13" opacity=".7" style="fill:var(--art-b1, #a78bfa)">z</text>
  </svg>`,
  search: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)"><circle cx="82" cy="58" r="34" fill="url(#art-d)"/></g>
    <circle cx="82" cy="58" r="34" stroke="url(#art-c)" stroke-width="9" fill="none"/>
    <path d="m107 84 24 24" stroke="url(#art-c)" stroke-width="12" stroke-linecap="round"/>
    <path d="M70 52c0-7 5-12 12-12" stroke-width="5" stroke-linecap="round" fill="none" opacity=".9" style="stroke:var(--art-w, #fff)"/>
  </svg>`,
  link: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <rect x="34" y="20" width="54" height="94" rx="14" fill="url(#art-a)"/>
      <rect x="98" y="38" width="56" height="44" rx="8" fill="url(#art-c)"/>
    </g>
    <rect x="40" y="30" width="42" height="72" rx="8" opacity=".9" style="fill:var(--art-paper, #fff)"/>
    <path d="M106 90h40" stroke="url(#art-c)" stroke-width="6" stroke-linecap="round"/>
    <path d="M84 62h18" stroke-width="4" stroke-linecap="round" stroke-dasharray="2 7" style="stroke:var(--art-c2, #8b5cf6)"/>
    <g opacity=".8" style="fill:var(--art-ink, #0f172a)"><rect x="50" y="46" width="10" height="10" rx="2"/><rect x="64" y="46" width="10" height="10" rx="2"/><rect x="50" y="60" width="10" height="10" rx="2"/><rect x="66" y="62" width="6" height="6" rx="1"/></g>
  </svg>`,
  playground: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <path d="M40 30h60a12 12 0 0 1 12 12v18a12 12 0 0 1-12 12H58l-14 11 2-11h-6a12 12 0 0 1-12-12V42a12 12 0 0 1 12-12Z" fill="url(#art-d)"/>
      <path d="M140 58H86a12 12 0 0 0-12 12v16a12 12 0 0 0 12 12h40l14 10-2-10h2a12 12 0 0 0 12-12V70a12 12 0 0 0-12-12Z" fill="url(#art-b)"/>
    </g>
    <path d="M42 46h44M42 56h28" stroke-width="5" stroke-linecap="round" opacity=".6" style="stroke:var(--art-line, #94a3b8)"/>
    <path d="M88 74h46M88 84h30" stroke-width="5" stroke-linecap="round" style="stroke:var(--art-w, #fff)"/>
    <path d="M150 18l3 7 7 3-7 3-3 7-3-7-7-3 7-3Z" style="fill:var(--art-a1, #34d399)"/>
  </svg>`,
  memory: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <rect x="36" y="24" width="82" height="90" rx="16" fill="url(#art-d)" transform="rotate(-6 77 69)"/>
      <path d="M112 40h34a12 12 0 0 1 12 12v18a12 12 0 0 1-12 12h-14l-12 10 2-10h-10a12 12 0 0 1-12-12V52a12 12 0 0 1 12-12Z" fill="url(#art-b)"/>
    </g>
    <circle cx="70" cy="28" r="7" fill="url(#art-c)"/>
    <path d="M52 52h44M53 66h36M55 80h40M57 94h24" stroke-width="5" stroke-linecap="round" opacity=".55" transform="rotate(-6 77 69)" style="stroke:var(--art-line, #94a3b8)"/>
    <path d="m129 50 3.2 6.6 7.3 1-5.3 5.1 1.3 7.2-6.5-3.4-6.5 3.4 1.3-7.2-5.3-5.1 7.3-1Z" style="fill:var(--art-w, #fff)"/>
    <path d="M24 98l2 5 5 2-5 2-2 5-2-5-5-2 5-2Z M154 104l2 4 4 2-4 2-2 4-2-4-4-2 4-2Z" style="fill:var(--art-a1, #34d399)"/>
  </svg>`,
  missions: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)"><circle cx="82" cy="70" r="44" fill="url(#art-d)"/></g>
    <circle cx="82" cy="70" r="44" stroke="url(#art-c)" stroke-width="7" fill="none"/>
    <circle cx="82" cy="70" r="28" stroke="url(#art-b)" stroke-width="7" fill="none"/>
    <circle cx="82" cy="70" r="11" fill="url(#art-a)"/>
    <path d="M86 66 128 26" stroke-width="4" stroke-linecap="round" style="stroke:var(--art-ink-2, #334155)"/>
    <path d="m128 26 4-14 6 10 12 0-8 8Z" fill="url(#art-b)"/>
    <path d="M150 52l2.5 6 6 2.5-6 2.5-2.5 6-2.5-6-6-2.5 6-2.5Z M28 24l2 5 5 2-5 2-2 5-2-5-5-2 5-2Z" style="fill:var(--art-o1, #fbbf24)"/>
  </svg>`,
  guide: `<svg viewBox="0 0 180 132" xmlns="http://www.w3.org/2000/svg">${defs}
    <ellipse cx="90" cy="122" rx="62" ry="6" opacity=".07" style="fill:var(--art-ink, #0b1220)"/>
    <g filter="url(#art-sh)">
      <path d="M90 36c-16-10-36-12-56-8v76c20-4 40-2 56 8Z" fill="url(#art-d)"/>
      <path d="M90 36c16-10 36-12 56-8v76c-20-4-40-2-56 8Z" fill="url(#art-a)"/>
    </g>
    <path d="M90 36v76" stroke-opacity=".12" stroke-width="2" style="stroke:var(--art-ink, #0b1220)"/>
    <path d="M46 48c12-2 24-1 34 4M46 62c12-2 24-1 34 4M46 76c12-2 24-1 34 4" stroke-width="4" stroke-linecap="round" fill="none" opacity=".6" style="stroke:var(--art-line, #94a3b8)"/>
    <circle cx="118" cy="66" r="15" opacity=".95" style="fill:var(--art-w, #fff)"/>
    <path d="m118 55 3 8 8 3-8 3-3 8-3-8-8-3 8-3Z" fill="url(#art-c)"/>
    <path d="M150 18l2.5 6 6 2.5-6 2.5-2.5 6-2.5-6-6-2.5 6-2.5Z" style="fill:var(--art-b1, #a78bfa)"/>
  </svg>`,
};

export function art(name) {
  return raw(`<span class="art" aria-hidden="true">${ARTS[name] || ARTS.chats}</span>`);
}

/** Empty state block. action: { label, icon, onClick, cls } */
export function emptyState({ artName = 'chats', title, body, action, secondary, small = false }) {
  return html`<div class=${'empty ' + (small ? 'empty-sm' : '')}>
    ${art(artName)}
    <h3>${title}</h3>
    ${body ? html`<p>${body}</p>` : ''}
    ${action || secondary ? html`<div class="row" style="justify-content:center;flex-wrap:wrap">
      ${action ? html`<button class=${'btn ' + (action.cls || 'btn-primary')} @click=${action.onClick}>${action.icon ? icon(action.icon) : ''}${action.label}</button>` : ''}
      ${secondary ? html`<button class="btn btn-glass" @click=${secondary.onClick}>${secondary.icon ? icon(secondary.icon) : ''}${secondary.label}</button>` : ''}
    </div>` : ''}
  </div>`;
}
