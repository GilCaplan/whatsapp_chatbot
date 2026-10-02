// status.js — WhatsApp connection state + activity type presentation.

import { html } from '../dom.js';
import { icon } from '../icons.js';

const WA = {
  connected:    { label: 'Connected', color: 'var(--green)', pulse: false, breathe: true, help: 'Your WhatsApp is linked and Doppel is listening.' },
  connecting:   { label: 'Connecting…', color: 'var(--amber)', pulse: true, help: 'Talking to WhatsApp…' },
  awaiting_qr:  { label: 'Waiting for scan', color: 'var(--blue)', pulse: true, help: 'Scan the QR code with your phone to link WhatsApp.' },
  pairing:      { label: 'Linking…', color: 'var(--teal)', pulse: true, help: 'Almost there — finishing the link with your phone.' },
  disconnected: { label: 'Disconnected', color: 'var(--gray)', pulse: false, help: 'Doppel is not connected to WhatsApp right now.' },
  logged_out:   { label: 'Not linked', color: 'var(--gray)', pulse: false, help: 'No WhatsApp account is linked. Scan a QR code to link one.' },
  error:        { label: 'Connection error', color: 'var(--red)', pulse: false, help: 'Something went wrong talking to WhatsApp.' },
  replaced:     { label: 'Opened elsewhere', color: 'var(--orange)', pulse: false, help: 'Another copy of this session connected and took over. Close other bots using this account, then reconnect.' },
  banned:       { label: 'Account restricted', color: 'var(--red)', pulse: false, help: 'WhatsApp has temporarily restricted this account.' },
  outdated:     { label: 'Update needed', color: 'var(--red)', pulse: false, help: 'WhatsApp rejected this version of Doppel. Update the app, then reconnect.' },
};

export function waMeta(state) {
  return WA[state] || { label: state || 'Unknown', color: 'var(--gray)', pulse: false, help: '' };
}

export function waLabel(wa) {
  if (!wa) return 'Starting…';
  if (wa.state === 'connected' && wa.me && wa.me.pushName) return `Connected as ${wa.me.pushName}`;
  return waMeta(wa.state).label;
}

export function statusDot(state, extra = '') {
  const m = waMeta(state);
  return html`<span class=${'status-dot ' + (m.pulse ? 'pulse ' : '') + (m.breathe ? 'breathe ' : '') + extra} style=${`--c:${m.color}`}></span>`;
}

export function statusPill(wa, onClick) {
  return html`<button class="status-pill glass" @click=${onClick} title=${waMeta(wa && wa.state).help}>
    ${statusDot(wa && wa.state)}
    <span class="label">${waLabel(wa)}</span>
    ${icon('chevron-down')}
  </button>`;
}

// ── Activity types ─────────────────────────────────────────────────────
export const ACT = {
  'incoming':           { label: 'Message in', color: 'var(--blue)', group: 'incoming', icon: 'message' },
  'decision.skip':      { label: 'Stayed quiet', color: 'var(--gray)', group: 'decisions', icon: 'hand' },
  'decision.reply':     { label: 'Decided to reply', color: 'var(--teal)', group: 'decisions', icon: 'bolt' },
  'blocked_injection':  { label: 'Blocked trick', color: 'var(--red)', group: 'blocked', icon: 'shield' },
  'noticing':           { label: 'Not seen yet', color: 'var(--gray)', group: 'timing', icon: 'eye' },
  'seen':               { label: 'Seen', color: 'var(--blue)', group: 'timing', icon: 'check-double' },
  'waiting':            { label: 'Waiting for more', color: 'var(--amber)', group: 'timing', icon: 'hourglass' },
  'thinking':           { label: 'Thinking', color: 'var(--violet)', group: 'timing', icon: 'brain' },
  'planning':           { label: 'Planning', color: 'var(--violet)', group: 'timing', icon: 'target' },
  'goal.reached':       { label: 'Goal reached', color: 'var(--green)', group: 'decisions', icon: 'target' },
  'distracted':         { label: 'Distracted', color: 'var(--orange)', group: 'timing', icon: 'coffee' },
  'typing':             { label: 'Typing', color: 'var(--teal)', group: 'timing', icon: 'typing' },
  'deferred':           { label: 'Reply later', color: 'var(--amber)', group: 'timing', icon: 'clock' },
  'generating':         { label: 'Writing reply', color: 'var(--violet)', group: 'decisions', icon: 'sparkles' },
  'sent':               { label: 'Sent', color: 'var(--green)', group: 'sent', icon: 'send' },
  'reacted':            { label: 'Reacted', color: 'var(--pink)', group: 'sent', icon: 'heart' },
  'proactive':          { label: 'Checking in', color: 'var(--indigo)', group: 'decisions', icon: 'wand' },
  'approval.queued':    { label: 'Needs approval', color: 'var(--yellow)', group: 'approvals', icon: 'inbox' },
  'approval.sent':      { label: 'Approved & sent', color: 'var(--green)', group: 'approvals', icon: 'check' },
  'approval.discarded': { label: 'Discarded', color: 'var(--gray)', group: 'approvals', icon: 'trash' },
  'error':              { label: 'Error', color: 'var(--red)', group: 'errors', icon: 'warning' },
  'wa.status':          { label: 'WhatsApp', color: 'var(--cyan)', group: 'system', icon: 'phone' },
  'system':             { label: 'System', color: 'var(--indigo)', group: 'system', icon: 'server' },
  // Wave 3
  'handoff':            { label: 'Needs you', color: 'var(--red)', group: 'decisions', icon: 'hand' },
  'handoff.resumed':    { label: 'Back on', color: 'var(--green)', group: 'decisions', icon: 'play' },
  'reveal':             { label: 'Revealed', color: 'var(--violet)', group: 'sent', icon: 'eye' },
  'memory':             { label: 'Remembered', color: 'var(--indigo)', group: 'system', icon: 'brain' },
  'recap':              { label: 'Daily recap', color: 'var(--cyan)', group: 'system', icon: 'text' },
};

export function actMeta(type) {
  return ACT[type] || { label: type || 'Event', color: 'var(--gray)', group: 'system', icon: 'info' };
}

export const ACT_GROUPS = [
  { value: 'all', label: 'All' },
  { value: 'incoming', label: 'Incoming' },
  { value: 'decisions', label: 'Decisions' },
  { value: 'timing', label: 'Timing' },
  { value: 'sent', label: 'Sent' },
  { value: 'approvals', label: 'Approvals' },
  { value: 'blocked', label: 'Blocked' },
  { value: 'errors', label: 'Errors' },
  { value: 'system', label: 'System' },
];

const PROVIDERS = { ollama: 'Ollama', anthropic: 'Claude', openai: 'OpenAI' };
export const providerLabel = (p) => PROVIDERS[p] || p || '';
