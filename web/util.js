// util.js — small pure helpers.

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
export const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

export function debounce(fn, ms) {
  let t = null;
  const d = (...args) => {
    clearTimeout(t);
    t = setTimeout(() => { t = null; fn(...args); }, ms);
  };
  d.cancel = () => { clearTimeout(t); t = null; };
  d.flush = (...args) => { clearTimeout(t); t = null; fn(...args); };
  d.pending = () => t !== null;
  return d;
}

export const plural = (n, one, many = one + 's') => `${n} ${n === 1 ? one : many}`;

export function toDate(v) {
  if (!v) return null;
  const d = v instanceof Date ? v : new Date(v);
  return isNaN(d) || d.getFullYear() < 2000 ? null : d;
}

/** "just now", "2m ago", "3h ago", "yesterday", "Mon", "12 Mar" */
export function relTime(v, now = Date.now()) {
  const d = toDate(v);
  if (!d) return '';
  const s = Math.round((now - d.getTime()) / 1000);
  if (s < -30) {
    const f = -s;
    if (f < 60) return `in ${f}s`;
    if (f < 3600) return `in ${Math.round(f / 60)}m`;
    return `in ${Math.round(f / 3600)}h`;
  }
  if (s < 45) return 'just now';
  if (s < 3600) return `${Math.max(1, Math.round(s / 60))}m ago`;
  if (s < 86400) return `${Math.round(s / 3600)}h ago`;
  const days = Math.floor(s / 86400);
  if (days === 1) return 'yesterday';
  if (days < 7) return d.toLocaleDateString(undefined, { weekday: 'short' });
  const sameYear = d.getFullYear() === new Date(now).getFullYear();
  return d.toLocaleDateString(undefined, sameYear ? { day: 'numeric', month: 'short' } : { day: 'numeric', month: 'short', year: 'numeric' });
}

export function duration(sec) {
  sec = Math.max(0, Math.round(sec));
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h ${m % 60}m`;
  return `${Math.floor(h / 24)}d ${h % 24}h`;
}

export function clockTime(v) {
  const d = toDate(v);
  return d ? d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }) : '';
}

export function fmtMs(ms) {
  if (ms == null || isNaN(ms)) return '';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)} s`;
}

export function fmtBytes(n) {
  if (!n) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(u.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i > 1 ? 1 : 0)} ${u[i]}`;
}

/** First grapheme of the first two words, uppercased. Handles emoji/Hebrew. */
export function initials(name) {
  const all = String(name || '').replace(/[()[\]{}"'.,]/g, ' ').trim().split(/\s+/).filter(Boolean);
  const words = all.filter((w) => /^[\p{L}\p{N}]/u.test(w));
  if (!words.length && all.length) return Array.from(all[0])[0] || '?';
  if (!words.length) return '?';
  const first = (w) => Array.from(w)[0] || '';
  const s = words.length > 1 ? first(words[0]) + first(words[1]) : first(words[0]);
  return s.toLocaleUpperCase();
}

export function hash(str) {
  let h = 2166136261;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export const GRADIENTS = [
  ['#34d399', '#0ea5e9'],
  ['#f472b6', '#8b5cf6'],
  ['#fbbf24', '#f97316'],
  ['#22d3ee', '#6366f1'],
  ['#a3e635', '#10b981'],
  ['#fb7185', '#f59e0b'],
  ['#818cf8', '#c084fc'],
  ['#2dd4bf', '#3b82f6'],
  ['#f9a8d4', '#fb923c'],
  ['#60a5fa', '#a78bfa'],
  ['#4ade80', '#14b8a6'],
  ['#f43f5e', '#a855f7'],
];

export function gradientFor(seed) {
  return GRADIENTS[hash(String(seed || '?')) % GRADIENTS.length];
}

const COLOR_RE = /^(#[0-9a-f]{3,8}|rgba?\([\d\s.,%]+\)|hsla?\([\d\s.,%deg]+\))$/i;
export function safeColor(c, fallback) {
  return typeof c === 'string' && COLOR_RE.test(c.trim()) ? c.trim() : fallback;
}

export function uid() {
  return Math.random().toString(36).slice(2, 10);
}

export const prefersReducedMotion = () =>
  window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches;

export function copyText(text) {
  if (navigator.clipboard && navigator.clipboard.writeText) return navigator.clipboard.writeText(text);
  const ta = document.createElement('textarea');
  ta.value = text;
  document.body.append(ta);
  ta.select();
  try { document.execCommand('copy'); } finally { ta.remove(); }
  return Promise.resolve();
}

/** Rows for an auto-growing textarea without JS-managed style. */
export function rowsFor(text, min = 1, max = 8) {
  const lines = String(text || '').split('\n').length;
  return clamp(lines, min, max);
}

export function greeting(d = new Date()) {
  const h = d.getHours();
  if (h < 5) return 'Up late';
  if (h < 12) return 'Good morning';
  if (h < 18) return 'Good afternoon';
  return 'Good evening';
}

const CC2 = new Set(['20', '27', '30', '31', '32', '33', '34', '36', '39', '40', '41', '43', '44', '45', '46', '47', '48', '49',
  '51', '52', '53', '54', '55', '56', '57', '58', '60', '61', '62', '63', '64', '65', '66', '81', '82', '84', '86', '90', '91', '92', '93', '94', '95', '98']);

/** Show a phone number nicely: 972501234567 → +972 50 123 4567, 15551234567 → +1 555 123 4567 (best effort). */
export function prettyPhone(p) {
  if (!p) return '';
  const d = String(p).replace(/\D/g, '');
  if (d.length < 8) return d;
  const cc = d[0] === '1' || d[0] === '7' ? d[0] : CC2.has(d.slice(0, 2)) ? d.slice(0, 2) : d.slice(0, 3);
  const r = d.slice(cc.length);
  let parts;
  if (r.length === 10) parts = [r.slice(0, 3), r.slice(3, 6), r.slice(6)];
  else if (r.length === 9) parts = [r.slice(0, 2), r.slice(2, 5), r.slice(5)];
  else if (r.length === 8) parts = [r.slice(0, 4), r.slice(4)];
  else parts = r.match(/.{1,4}/g) || [r];
  return `+${cc} ${parts.join(' ')}`;
}
