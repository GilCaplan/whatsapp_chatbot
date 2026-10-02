// behaviour-meta.js — field lists, fallback ranges, preset styling and small
// helpers shared by the Behaviour form, the reply timeline, Settings and the
// chat drawer. The server (GET /api/behavior/presets) is the source of truth
// for presets/ranges/enums; the fallbacks here only keep the UI usable if
// that request fails.

/** Every scalar field of a BehaviorProfile (JSON names). */
export const SCALAR_FIELDS = [
  'replyPercent', 'replyWhenNameMentioned', 'replyWhenAtMentioned', 'skipWhenOthersMentioned', 'aiJudgement',
  'chimeInPercent', 'ignoreLinks', 'reactPercent', 'pauseWhenYouReply', 'maxRepliesPerHour', 'maxRepliesPerDay',
  'cooldownSec', 'historyMessages', 'historyChars', 'staleAfterMin',
  'respondToAllMaxMembers', 'answerAnyoneWhoAddressesIt', 'maxStreak', 'triggerWords', 'muteWords',
  'noticeMinSec', 'noticeMaxSec', 'markRead', 'waitForMoreSec', 'burstCapSec', 'thinkMinSec', 'thinkMaxSec',
  'distractedPercent', 'distractedMinSec', 'distractedMaxSec', 'typingIndicator', 'typingCharsPerSec',
  'typingJitterPercent', 'typingMinSec', 'typingMaxSec',
  'splitPercent', 'splitMaxParts', 'bubbleGapMinSec', 'bubbleGapMaxSec', 'quoteReplyPercent',
  'allowMentions', 'mentionMax', 'tagReplyPercent', 'lengthBias',
  'typoPercent', 'typoFixStyle',
  'autoSendSeconds', 'injectionFilter',
];
/** Nested blocks that are overridden / reset as a whole. */
export const BLOCK_FIELDS = ['availability', 'proactive'];
export const ALL_FIELDS = [...SCALAR_FIELDS, ...BLOCK_FIELDS];

/** Fields that only mean something in group chats. */
export const GROUP_ONLY = new Set([
  'replyWhenNameMentioned', 'replyWhenAtMentioned', 'skipWhenOthersMentioned', 'aiJudgement', 'chimeInPercent', 'quoteReplyPercent',
  'allowMentions', 'mentionMax', 'tagReplyPercent', 'respondToAllMaxMembers', 'answerAnyoneWhoAddressesIt',
]);

/** List-valued fields (word lists): replaced as a whole. */
export const LIST_FIELDS = new Set(['triggerWords', 'muteWords']);

/** Fallback ranges (plan §1.4) — replaced by the server's table when loaded. */
export const FALLBACK_RANGES = {
  replyPercent: [0, 100], chimeInPercent: [0, 100], reactPercent: [0, 100], distractedPercent: [0, 100],
  splitPercent: [0, 100], quoteReplyPercent: [0, 100], tagReplyPercent: [0, 100], mentionMax: [1, 5], typingJitterPercent: [0, 80],
  typoPercent: [0, 30],
  respondToAllMaxMembers: [0, 1024], maxStreak: [0, 50],
  noticeMinSec: [0, 3600], noticeMaxSec: [0, 3600], distractedMinSec: [0, 3600], distractedMaxSec: [0, 3600], cooldownSec: [0, 3600],
  waitForMoreSec: [0, 600], thinkMinSec: [0, 600], thinkMaxSec: [0, 600], burstCapSec: [0, 1800],
  typingCharsPerSec: [1, 40], typingMinSec: [0, 60], typingMaxSec: [1, 180],
  bubbleGapMinSec: [0, 60], bubbleGapMaxSec: [0, 60], splitMaxParts: [2, 5],
  maxRepliesPerHour: [0, 600], maxRepliesPerDay: [0, 5000],
  historyMessages: [2, 500], historyChars: [500, 200000], staleAfterMin: [1, 1440], autoSendSeconds: [0, 86400],
  'availability.catchUpMaxMin': [0, 240], 'proactive.afterHours': [1, 720], 'proactive.maxPerDay': [1, 10], 'proactive.spreadMinutes': [0, 720],
};

export const FALLBACK_ENUMS = {
  injectionFilter: ['strict', 'balanced', 'off'],
  lengthBias: ['shorter', 'normal', 'longer', 'match'],
  typoFixStyle: ['correction', 'edit', 'none'],
  outsideHours: ['queue', 'silent'],
};

/** Range for a field: { min, max }. Accepts "availability.catchUpMaxMin" style paths. */
export function rangeOf(meta, field) {
  const r = meta && meta.ranges;
  const leaf = field.includes('.') ? field.split('.').pop() : field;
  const hit = r && (r[field] || r[leaf]);
  if (hit && typeof hit === 'object') {
    const min = hit.min ?? hit.Min;
    const max = hit.max ?? hit.Max;
    if (Number.isFinite(min) && Number.isFinite(max)) return { min, max };
  }
  const f = FALLBACK_RANGES[field] || FALLBACK_RANGES[leaf] || [0, 100];
  return { min: f[0], max: f[1] };
}

/** Presentation for each preset id (the server supplies label/description). */
export const PRESET_STYLE = {
  instant:  { icon: 'bolt', label: 'Instant', c1: '#fbbf24', c2: '#f97316', desc: 'Answers right away, like a bot. Good for testing.' },
  natural:  { icon: 'smile', label: 'Natural', c1: '#34d399', c2: '#0ea5e9', desc: 'Reads, thinks and types like a person who has their phone nearby.' },
  busy:     { icon: 'clock', label: 'Busy', c1: '#60a5fa', c2: '#6366f1', desc: 'Takes a while to look, sometimes misses messages, keeps it brief.' },
  slow:     { icon: 'hourglass', label: 'Slow texter', c1: '#c084fc', c2: '#f472b6', desc: 'Checks the phone now and then and types slowly.' },
  nightowl: { icon: 'moon', label: 'Night owl', c1: '#818cf8', c2: '#312e81', desc: 'Active in the evening and late at night, replies the next evening otherwise.' },
  custom:   { icon: 'sliders', label: 'Custom', c1: '#94a3b8', c2: '#475569', desc: 'Your own mix of the settings below.' },
};
export const PRESET_ORDER = ['instant', 'natural', 'busy', 'slow', 'nightowl'];

export function presetStyle(id) {
  return PRESET_STYLE[id] || PRESET_STYLE.custom;
}

/** Map a chat kind ('dm'|'group') to the settings profile key. */
export const profileKey = (kind) => (kind === 'group' ? 'group' : 'private');
/** Map a settings profile key to the API kind. */
export const apiKind = (key) => (key === 'group' ? 'group' : 'dm');

/** Stable deep equality for plain JSON values. */
export function same(a, b) {
  if (a === b) return true;
  if (a == null || b == null) return a == b; // eslint-disable-line eqeqeq
  if (typeof a !== 'object' || typeof b !== 'object') return false;
  if (Array.isArray(a) !== Array.isArray(b)) return false;
  if (Array.isArray(a)) return a.length === b.length && a.every((x, i) => same(x, b[i]));
  const ka = Object.keys(a).filter((k) => a[k] !== undefined);
  const kb = Object.keys(b).filter((k) => b[k] !== undefined);
  if (ka.length !== kb.length) return false;
  return ka.every((k) => same(a[k], b[k]));
}

/** Normalised availability for comparisons (week sorted mon..sun, empty ranges kept). */
const DAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
function normAvailability(av) {
  if (!av) return av;
  const week = DAYS.map((d) => {
    const e = (av.week || []).find((x) => x && x.day === d);
    return { day: d, ranges: (e && e.ranges ? e.ranges : []).map((r) => ({ from: r.from, to: r.to })) };
  });
  return { ...av, week, timezone: av.timezone || '' };
}

/** True when `profile` matches preset `p` for this kind on every field. */
export function matchesPreset(profile, presetProfile) {
  if (!profile || !presetProfile) return false;
  for (const f of ALL_FIELDS) {
    let a = profile[f];
    let b = presetProfile[f];
    if (f === 'availability') { a = normAvailability(a); b = normAvailability(b); }
    if (a === undefined || b === undefined) continue;
    if (!same(a, b)) return false;
  }
  return true;
}

/** Which preset id a full profile corresponds to ('custom' if none). */
export function detectPreset(profile, presets, key) {
  for (const p of presets || []) {
    if (matchesPreset(profile, p[key])) return p.id;
  }
  return 'custom';
}

/** Compact duration: 0s, 45s, 1m 30s, 12m, 1h 5m, 2d. */
export function fmtSec(sec) {
  sec = Math.max(0, Math.round(Number(sec) || 0));
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  if (m < 10 && s) return `${m}m ${s}s`;
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const mm = m % 60;
  if (h < 24) return mm ? `${h}h ${mm}m` : `${h}h`;
  const d = Math.floor(h / 24);
  return h % 24 ? `${d}d ${h % 24}h` : `${d}d`;
}

/** Spoken duration for summaries: "about 45 seconds", "about 3 minutes". */
export function sayDuration(sec) {
  sec = Math.max(0, Math.round(Number(sec) || 0));
  if (sec < 2) return 'instantly';
  if (sec < 60) return `${sec} seconds`;
  const m = sec / 60;
  if (m < 2) return 'a minute';
  if (m < 60) return `${Math.round(m)} minutes`;
  const h = m / 60;
  if (h < 2) return 'an hour';
  return `${Math.round(h)} hours`;
}

/** "1 in 3" style fraction for a 0–1 share. */
export function sayShare(share) {
  if (!(share > 0)) return 'none';
  if (share >= 0.9) return 'almost all';
  if (share >= 0.6) return 'most';
  if (share >= 0.45) return 'about half';
  const n = Math.max(2, Math.round(1 / share));
  return `about 1 in ${n}`;
}

/** The persona's IANA time zone on this computer. */
export function localTimeZone() {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone || ''; } catch { return ''; }
}

/** Count of fields a chat customises (blocks count as one). Preset label excluded. */
export function overrideCount(overrides) {
  if (!overrides) return 0;
  return ALL_FIELDS.filter((f) => overrides[f] !== undefined && overrides[f] !== null).length;
}
