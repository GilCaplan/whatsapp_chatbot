// feed.js — activity row rendering (dashboard mini feed + Activity page).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { actMeta, providerLabel } from './status.js';
import { fmtMs, clockTime, relTime } from '../util.js';
import { fmtSec } from './behaviour-meta.js';
import { mentionNodes } from './mentions.js';

const META_LABELS = {
  reason: 'Why',
  delaySeconds: 'Delay',
  typingSeconds: 'Typing for',
  chars: 'Characters',
  part: 'Bubble',
  count: 'Messages',
  quoted: 'Quoted their message',
  resumeAt: 'Resumes',
  limit: 'Limit',
  window: 'Per',
  silenceHours: 'Quiet for',
  stage: 'Stage',
  dueAt: 'Due',
  readReceipts: 'Read receipts',
  proactive: 'Check-in',
  manual: 'Sent by you',
  percent: 'Chance',
  messages: 'Messages read',
  provider: 'Brain',
  model: 'Model',
  latencyMs: 'Took',
  score: 'Score',
  matches: 'Matched',
  resetCount: 'Timer resets',
  waitSeconds: 'Waited',
  mentions: 'Tagged',
  sender: 'From',
  word: 'Word',
  plan: 'Next move',
  hint: 'Topic',
  goal: 'Goal',
  how: 'How',
  evidence: 'What they said',
  afterReached: 'Afterwards',
  goalRewritten: 'Rewritten to keep the goal secret',
  crossContext: 'Context from other chats',
  crossRewritten: 'Rewritten to keep other chats private',
  crossDropped: 'Written without context from other chats',
  lengthRetried: 'Asked again for a shorter reply',
  trimmed: 'Trimmed to the length setting',
  emojiRemoved: 'Emoji removed (emoji setting)',
  emojiAdded: 'Emoji added (emoji setting)',
  tone: 'Read the message as',
  // Wave 3
  category: 'About',
  excerpt: 'Their message',
  typo: 'Made a typo',
  fix: 'Corrected it',
  edited: 'Edited the message',
  drafts: 'Drafts',
  block: 'Routine',
  added: 'New things learned',
  updated: 'Things updated',
  chats: 'Chats',
};

// Never shown: reactions are described in words, not with the emoji itself.
const HIDDEN = new Set(['emoji', 'parts', 'messageId', 'distracted', 'jid', 'sentAt', 'missionId']);

/** decision.skip / deferred reason codes in plain English. */
export const REASONS = {
  reply_chance: 'Reply chance said no — left it on read',
  outside_hours: 'Outside active hours',
  snoozed: 'Chat is set to Away',
  rate_limit: 'Reached the reply limit',
  you_replied: 'You answered yourself',
  chime_in: 'Chimed in by chance',
  chime_out: 'Chose not to chime in',
  random_override: 'Chimed in by chance',
  new_messages: 'New messages arrived — starting over',
  cooldown: 'Too soon after the last reply',
  burst_cap: 'Waited long enough — replying now',
  link: 'Only a link',
  stale: 'Old message — kept as background',
  mentions_other: 'Someone else was @mentioned',
  name_mentioned: 'Its name came up',
  mentioned_me: 'You were @mentioned',
  always_reply: 'Replies to everything here',
  llm_yes: 'The AI thought it should join in',
  llm_no: "The AI thought it wasn't needed",
  llm_error: "Couldn't ask the AI, so it stayed quiet",
  llm_error_random: "Couldn't ask the AI, so it went by chance",
  not_selected: "Not one of the people it answers here",
  priority_person: 'Always replies to this person',
  trigger_word: 'The message has a trigger word',
  mute_word: 'The message has a mute word',
  streak_limit: 'Too many replies in a row to one person',
  // Wave 3
  handoff: 'Paused until you take over',
  routine: 'Busy with its daily routine',
};

// Wave 3: hand-off categories and how they were spotted.
const HANDOFF_CATEGORIES = {
  money: 'Money', health: 'Health', meeting: 'Meeting up', distress: 'Someone may be struggling',
  bot: 'Asked if it is a bot', legal: 'Legal matters',
};
const HANDOFF_HOW = { keyword: 'Matched a keyword', ai: 'The AI double-checked it' };

// Activity types whose text is a chat message (tags are shown as chips).
const MESSAGE_TYPES = new Set(['incoming', 'sent', 'approval.sent', 'approval.queued', 'approval.discarded']);

const STAGES = { scheduled: 'Scheduled', starting: 'Writing the opener', sent: 'Sent', queued: 'Waiting for approval', skipped: 'Skipped', plan: 'Planning the next move', manual: 'You asked for it', cross: 'Using context from other chats' };
const GOAL_HOW = { said_word: 'They said the word', ai: 'Spotted in the conversation', media: 'They sent it' };
const GOAL_AFTER = { relax: 'Stops pursuing it and just chats', continue: 'Keeps gently pursuing it' };

const TONES = { serious: 'serious or sad', vent: 'venting', good: 'good news', love: 'thanks or affection', funny: 'a joke', surprise: 'surprising news', plan: 'a plan', question: 'a question', greeting: 'a hello', neutral: 'everyday chat' };

function metaValue(k, v, meta) {
  if (v == null) return '';
  switch (k) {
    case 'latencyMs': return fmtMs(v);
    case 'provider': return providerLabel(v);
    case 'waitSeconds':
    case 'delaySeconds':
    case 'typingSeconds': return typeof v === 'number' ? fmtSec(v) : String(v);
    case 'score': return typeof v === 'number' ? v.toFixed(2) : String(v);
    case 'reason': return REASONS[v] || String(v);
    case 'quoted': return v ? 'Yes' : 'No';
    case 'part': return meta && meta.parts ? `${v} of ${meta.parts}` : String(v);
    case 'readReceipts': return v ? 'On' : 'Off — nobody sees blue ticks';
    case 'percent': return `${v}%`;
    case 'proactive':
    case 'manual': return v ? 'Yes' : 'No';
    case 'dueAt':
    case 'resumeAt': { const t = clockTime(v); const r = relTime(v); return t ? `${t}${r ? ` (${r})` : ''}` : String(v); }
    case 'silenceHours': return typeof v === 'number' ? (v >= 48 ? `${Math.round(v / 24)} days` : `${Math.round(v)} hours`) : String(v);
    case 'stage': return STAGES[v] || String(v);
    case 'how': return (meta && meta.category ? HANDOFF_HOW[v] : GOAL_HOW[v]) || String(v);
    case 'category': return HANDOFF_CATEGORIES[v] || String(v);
    case 'typo':
    case 'fix':
    case 'edited': return v ? 'Yes' : 'No';
    case 'drafts': return typeof v === 'number' ? `${v} to choose from` : String(v);
    case 'afterReached': return GOAL_AFTER[v] || String(v);
    case 'crossContext': {
      const n = Number(v.items) || 0;
      const people = Array.isArray(v.people) && v.people.length ? ` about ${v.people.join(', ')}` : '';
      const how = { discreet: 'discreet', open: 'open' }[v.mode] || String(v.mode || '');
      return `${n} ${n === 1 ? 'note' : 'notes'}${people}${how ? ` (${how})` : ''}`;
    }
    case 'crossRewritten':
    case 'crossDropped':
    case 'goalRewritten':
    case 'lengthRetried':
    case 'trimmed':
    case 'emojiAdded': return v ? 'Yes' : 'No';
    case 'tone': return TONES[v] || String(v);
    case 'window': return v === 'hour' ? 'hour' : v === 'day' ? 'day' : String(v);
    default:
      if (Array.isArray(v)) return v.join(', ');
      if (typeof v === 'object') return JSON.stringify(v);
      return String(v);
  }
}

export function metaEntries(meta) {
  if (!meta) return [];
  const keys = Object.keys(meta).filter((k) => !HIDDEN.has(k));
  const order = Object.keys(META_LABELS);
  keys.sort((a, b) => {
    const ia = order.indexOf(a); const ib = order.indexOf(b);
    return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib);
  });
  return keys.filter((k) => meta[k] !== '' && meta[k] != null).map((k) => [META_LABELS[k] || k, metaValue(k, meta[k], meta)]);
}

const PICTO = /[\p{Extended_Pictographic}\u{1F1E6}-\u{1F1FF}\u{FE0F}\u{200D}\u{1F3FB}-\u{1F3FF}]/gu;
/** System-written texts never show emoji (user/persona message text is left alone). */
function displayText(ev) {
  if (!ev.text) return '';
  if (ev.type === 'reacted') {
    const t = ev.text.replace(PICTO, '').replace(/\s{2,}/g, ' ').trim();
    return t || 'Reacted to their message instead of replying';
  }
  return ev.text;
}

function quickLine(ev) {
  const m = ev.meta;
  if (!m) return '';
  if (Array.isArray(m.mentions) && m.mentions.length && (ev.type === 'sent' || ev.type === 'approval.sent' || ev.type === 'approval.queued')) {
    return `Tagged ${m.mentions.join(', ')}`;
  }
  if (m.reason) return REASONS[m.reason] || m.reason;
  if (m.plan) return `Next move: ${m.plan}`;
  if (ev.type === 'goal.reached' && m.evidence) return `“${m.evidence}”`;
  if (ev.type === 'sent' && m.parts > 1) return `Bubble ${m.part || 1} of ${m.parts}${m.quoted ? ' · quoted their message' : ''}`;
  if (m.quoted) return 'Quoted their message';
  if (m.resumeAt) return `Resumes ${metaValue('resumeAt', m.resumeAt)}`;
  if (m.latencyMs) return fmtMs(m.latencyMs);
  return '';
}

/** One activity row. opts: { expanded, onToggle, compact, isNew } */
export function feedRow(ev, opts = {}) {
  // A distraction is reported as a "thinking" event flagged meta.distracted.
  const m = actMeta(ev.type === 'thinking' && ev.meta && ev.meta.distracted ? 'distracted'
    : ev.type === 'thinking' && ev.meta && ev.meta.stage === 'plan' ? 'planning'
    : ev.type === 'thinking' && ev.meta && ev.meta.stage === 'cross' ? 'crossing' : ev.type);
  const entries = metaEntries(ev.meta);
  const canExpand = !opts.compact && entries.length > 0;
  const quick = quickLine(ev);
  const text = displayText(ev);
  return html`<div class=${'feed-row ' + (opts.compact ? 'compact ' : '') + (canExpand ? 'expandable ' : '') + (opts.expanded ? 'open ' : '') + (opts.isNew ? 'row-new' : '')}
      data-key=${'a' + ev.id} @click=${canExpand ? opts.onToggle : null}>
    <span class="feed-icon" style=${`--c:${m.color}`}>${icon(m.icon)}</span>
    <div class="grow">
      <div class="feed-head">
        <span class="feed-type">${m.label}</span>
        ${ev.chatName ? html`<span class="feed-chat ellipsis">${ev.chatName}</span>` : ''}
        ${ev.personaName ? html`<span class="chip chip-sm">${ev.personaName}</span>` : ''}
        <span class="grow"></span>
        <rel-time class="feed-time" datetime=${ev.ts}></rel-time>
      </div>
      ${text ? html`<div class=${'feed-text ' + (opts.compact ? 'clamp-2' : '')}>${MESSAGE_TYPES.has(ev.type) ? mentionNodes(text, ev.meta && ev.meta.mentions) : text}</div>` : ''}
      ${!opts.expanded && quick && !opts.compact ? html`<div class="feed-quick faint small ellipsis">${quick}</div>` : ''}
      ${opts.expanded ? html`<dl class="kv feed-meta">${entries.map(([k, v]) => html`<dt>${k}</dt><dd>${v}</dd>`)}</dl>` : ''}
    </div>
    ${canExpand ? html`<span class="feed-chev">${icon(opts.expanded ? 'chevron-up' : 'chevron-down')}</span>` : ''}
  </div>`;
}
