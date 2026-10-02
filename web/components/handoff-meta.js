// handoff-meta.js — copy and pictures for the hand-off categories ("Needs
// you"), the reply modes and the co-pilot tones. Pictures are small inline
// SVGs in the icons.js stroke style (never emoji).

import { raw } from '../dom.js';
import { api } from '../api.js';
import { store, loadChats } from '../store.js';
import { toast } from '../ui.js';

const svg = (body) => raw(`<svg class="ic cat-glyph" viewBox="0 0 24 24" aria-hidden="true" focusable="false" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`);

const GLYPHS = {
  money: '<circle cx="12" cy="12" r="8.4"/><path d="M14.6 9.2c-.5-.9-1.5-1.4-2.6-1.4-1.5 0-2.6.8-2.6 2s1.1 1.7 2.6 2.1 2.7.9 2.7 2.1-1.2 2.1-2.7 2.1c-1.2 0-2.2-.5-2.7-1.4"/><path d="M12 6.2v1.6M12 16.2v1.6"/>',
  health: '<rect x="3.6" y="3.6" width="16.8" height="16.8" rx="5"/><path d="M12 8v8M8 12h8"/>',
  meeting: '<path d="M12 21s-6.2-5.4-6.2-10.6a6.2 6.2 0 0 1 12.4 0C18.2 15.6 12 21 12 21Z"/><circle cx="12" cy="10.3" r="2.3"/>',
  distress: '<path d="M12 19.6s-7.4-4.3-7.4-9.6A4.1 4.1 0 0 1 12 7.6a4.1 4.1 0 0 1 7.4 2.4c0 5.3-7.4 9.6-7.4 9.6Z"/><path d="m12.6 9.4-1.7 3h2.4l-1.6 3"/>',
  bot: '<rect x="5" y="8" width="14" height="11" rx="3.4"/><path d="M12 8V4.8"/><circle cx="12" cy="3.9" r="1"/><circle cx="9.3" cy="13.2" r="1.1" fill="currentColor" stroke="none"/><circle cx="14.7" cy="13.2" r="1.1" fill="currentColor" stroke="none"/><path d="M9.8 16.3h4.4M3.2 12.6v2.6M20.8 12.6v2.6"/>',
  legal: '<path d="M12 4v15.6M7.6 19.6h8.8M5.4 7.2h13.2"/><path d="m6.6 7.2-2.8 6.2a3 3 0 0 0 5.6 0Z"/><path d="m17.4 7.2-2.8 6.2a3 3 0 0 0 5.6 0Z"/>',
  brief: '<path d="M4.5 12h9"/><path d="m15.2 7.4 4.3 4.6-4.3 4.6"/>',
  warm: '<path d="M12 19.2s-7-4-7-9.2A3.9 3.9 0 0 1 12 7.8a3.9 3.9 0 0 1 7 2.2c0 5.2-7 9.2-7 9.2Z"/>',
  playful: '<circle cx="12" cy="12" r="8.4"/><path d="M8.6 14.2c.8 1.4 2 2.1 3.4 2.1s2.6-.7 3.4-2.1"/><path d="M8.4 10.2c.4-.6 1.2-.6 1.6 0M14 10.2c.4-.6 1.2-.6 1.6 0"/>',
};

/** The category picture (inline SVG). */
export const catGlyph = (cat) => svg(GLYPHS[cat] || GLYPHS.bot);

/** Hand-off categories in display order: label, one-line explanation. */
export const HANDOFF_CATS = [
  { id: 'money', label: 'Money', line: 'Asking for money, payments, loans or bank details.' },
  { id: 'health', label: 'Health', line: 'Illness, injuries, hospital or an emergency.' },
  { id: 'meeting', label: 'Meeting up', line: 'Real plans to meet: a time, a place, an address.' },
  { id: 'distress', label: 'Someone struggling', line: 'Sadness, crisis or talk of hurting themselves.' },
  { id: 'bot', label: 'Is it a bot?', line: 'Asking if they are talking to an AI or the real you.' },
  { id: 'legal', label: 'Legal', line: 'Lawyers, police, courts or contracts.' },
];

export const catLabel = (id) => (HANDOFF_CATS.find((c) => c.id === id) || { label: 'Something sensitive' }).label;

/** What they did, without the name: "asked if they're talking to a bot". */
export function catPhrase(cat) {
  switch (cat) {
    case 'money': return 'brought up money';
    case 'health': return 'mentioned something about health';
    case 'meeting': return 'wants to meet up';
    case 'distress': return 'may be going through something hard';
    case 'bot': return "asked if they're talking to a bot";
    case 'legal': return 'mentioned something legal';
    default: return 'wrote something sensitive';
  }
}

/** What paused the chat, in words: "Dana asked if they're talking to a bot". */
export const catReason = (cat, who) => `${who || 'Someone'} ${catPhrase(cat)}`;

/** Reply modes (ChatAssignment.mode). */
export const MODES = [
  { value: 'auto', label: 'Auto', icon: 'bolt', line: 'Replies go out on their own, like you would.' },
  { value: 'approve', label: 'Approve', icon: 'shield', line: 'Every reply waits in Approvals until you tap Send.' },
  { value: 'copilot', label: 'Co-pilot', icon: 'sparkles', line: 'Three ideas wait in Approvals: brief, warm or playful. You pick one.' },
];

export const modeOf = (c) => (c && c.mode) || (c && c.approvalMode ? 'approve' : 'auto');
export const modeLabel = (m) => (MODES.find((x) => x.value === m) || MODES[0]).label;

/** Co-pilot tones. */
export const TONES = {
  brief: { label: 'Brief', line: 'Short and simple' },
  warm: { label: 'Warm', line: 'Kind, asks back' },
  playful: { label: 'Playful', line: 'A little tease' },
};

/** https://wa.me/<number> for a DM (null for groups and hidden numbers). */
export function waLink(c) {
  if (!c || c.kind === 'group') return null;
  for (const j of [c.jid, c.altJid]) {
    const m = /^(\d{6,15})@s\.whatsapp\.net$/.exec(j || '');
    if (m) return `https://wa.me/${m[1]}`;
  }
  return null;
}

/** The reveal message with {persona} and {me} filled in (same rule as the server). */
export function renderReveal(tpl, persona, me) {
  return String(tpl || '')
    .replaceAll('{persona}', (persona || '').trim() || 'a persona')
    .replaceAll('{me}', (me || '').trim() || 'me')
    .trim();
}

/** Lets the persona answer a paused (hand-off) chat again. */
export async function resumeChat(key, personaName) {
  const before = store.state.chats;
  store.set({ chats: before.map((c) => (c.key === key ? { ...c, handoff: null } : c)) });
  try {
    await api.chats.resumeHandoff(key);
    toast(`${personaName || 'The persona'} is back on in this chat`, { type: 'success' });
  } catch {
    store.set({ chats: before });
  } finally {
    loadChats().catch(() => {});
  }
}
