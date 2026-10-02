// avatar.js — persona avatars (rounded squircles) and chat avatars (circles).

import { html } from '../dom.js';
import { initials, gradientFor, safeColor } from '../util.js';
import { api } from '../api.js';
import { glyph } from './glyphs.js';

/** Gradient pair for a persona, validated so server data can't inject CSS. */
export function personaGradient(p) {
  const fb = gradientFor((p && (p.id || p.name)) || '?');
  const g = (p && p.avatar && p.avatar.gradient) || [];
  return [safeColor(g[0], fb[0]), safeColor(g[1], fb[1])];
}

/**
 * personaAvatar(persona, size, { preview }) — preview: object URL for an
 * unsaved upload (editor).
 */
export function personaAvatar(p, size = 40, opts = {}) {
  const style = `--s:${size}px`;
  if (!p) return html`<span class="pav pav-empty" style=${style}>?</span>`;
  const a = p.avatar || {};
  if (opts.preview) {
    return html`<span class="pav" style=${style}><img src=${opts.preview} alt=""></span>`;
  }
  if (a.kind === 'upload' && p.id) {
    return html`<span class="pav" style=${style + ';background:var(--fill-2)'}><img src=${api.personas.avatarURL(p.id, a.version)} alt="" loading="lazy" decoding="async"></span>`;
  }
  const [c1, c2] = personaGradient(p);
  const bg = `${style};background:linear-gradient(135deg, ${c1}, ${c2})`;
  const g = glyph(a.glyph);
  if (g) return html`<span class="pav pav-glyph" style=${bg}>${g}</span>`;
  return html`<span class="pav" style=${bg}>${(a.initials || initials(p.name)).slice(0, 2)}</span>`;
}

/** <chat-avatar> element template. */
export function chatAvatar({ jid = '', name = '', kind = '', src = '' } = {}, size = 40) {
  return html`<chat-avatar jid=${jid} name=${name} kind=${kind} size=${size} src=${src}></chat-avatar>`;
}

/** Chat avatar with the persona's avatar overlapping bottom-right. */
export function avatarStack(chat, persona, size = 44) {
  return html`<span class="av-stack">
    ${chatAvatar(chat, size)}
    ${persona ? personaAvatar(persona, Math.round(size * 0.5)) : ''}
  </span>`;
}
