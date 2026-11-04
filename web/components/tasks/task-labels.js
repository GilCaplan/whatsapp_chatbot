// tasks/task-labels.js — plain-English words for the task rule vocabulary
// (conditions, actions, steps), run statuses and what a turn did, plus time
// formatting. The vocabulary itself comes from GET /api/tasks.

const COND = {
  started: 'The task starts',
  'time:works': 'A time they gave works',
  'time:no': "A time they gave doesn't work",
  'time:day': 'They give only a day or part of a day',
  unclear: "They agree, but it isn't clear to which time",
  ready: 'A time works and everything is filled in',
  probe_limit: 'Too many "doesn\'t work" answers',
  no_reply: "They haven't answered for a while",
  out_of_nudges: 'Still no answer after every nudge',
  limit: 'The message limit is reached',
  'intent:accept': 'They agree',
  'intent:reject': 'They turn the times down',
  'intent:propose': 'They suggest a time',
  'intent:ask_availability': 'They ask when you are free',
  'intent:ask_reason': 'They ask why a time doesn\'t work',
  'intent:question': 'They ask something else',
  'intent:defer': "They'll get back to you",
  'intent:off_topic': 'They change the subject',
  'intent:money': 'They bring up money',
  'intent:bot_menu': 'An automated menu answers',
  'intent:other': 'Anything else they say',
};

const ACT = {
  strategy: 'Follow the privacy strategy',
  offer: 'Offer a few times',
  offer_windows: 'Offer parts of days',
  offer_on_day: 'Offer times on their day',
  answer_time: "Say it doesn't work, offer another",
  ask_their_times: 'Ask when suits them',
  ask_confirm: 'Ask them to confirm',
  ask_which: 'Ask which time',
  commit: 'Confirm (asks you first)',
  cover: 'Answer "why not?" without a reason',
  chat: 'Answer, then steer back',
  nudge: 'Send a friendly nudge',
  wait: 'Say there is no rush',
  on_probe_limit: 'Do what the probe limit says',
  handoff: 'Hand over to me',
  'finish:ok': 'End: done',
  'finish:fail': 'End: no time found',
  notify: 'Tell me',
};

export const STEPS = [
  { value: '', label: 'Any time' },
  { value: 'open', label: 'At the start' },
  { value: 'negotiate', label: 'While agreeing a time' },
  { value: 'confirming', label: 'While confirming' },
];

const slotLabel = (slots, key) => ((slots || []).find((s) => s.key === key) || { label: key }).label;

export function condLabel(c, slots) {
  if (c.startsWith('missing:')) return `${slotLabel(slots, c.slice(8))} isn't known yet`;
  return COND[c] || c;
}

export function actLabel(a, slots) {
  if (a.startsWith('ask:')) return `Ask for ${slotLabel(slots, a.slice(4)).toLowerCase()}`;
  return ACT[a] || a;
}

/** Actions that only have an effect (they write nothing). */
export const EFFECTS = ['handoff', 'finish:ok', 'finish:fail', 'notify'];

export const STATUS = {
  active: { label: 'Running', chip: 'chip-blue' },
  approval: { label: 'Waiting for you', chip: 'chip-amber' },
  done: { label: 'Done', chip: 'chip-green' },
  failed: { label: 'No time found', chip: '' },
  'handed-off': { label: 'Handed over to you', chip: 'chip-violet' },
  stopped: { label: 'Stopped', chip: '' },
};

export const STEP_LABEL = {
  open: 'Getting started', negotiate: 'Agreeing on a time', confirming: 'Waiting for their yes',
  committing: 'Confirming', ended: 'Ended',
};

export const STRATEGIES = [
  { value: 'they-first', label: 'They go first', help: 'Asks when suits them and offers one match. Shows the least of your week.' },
  { value: 'offers', label: 'A few times', help: 'Offers one to three times it picked. Round times, never right next to something busy.' },
  { value: 'windows', label: 'Parts of days', help: 'Offers "Tuesday morning or Thursday afternoon", then narrows down.' },
];

export const PROBE_THEN = [
  { value: 'handoff', label: 'Hand over to me' },
  { value: 'offer', label: 'Send a few options' },
  { value: 'ask', label: 'Ask for their times' },
];

export const BOOK = [
  { value: 'off', label: "Don't book" },
  { value: 'ics', label: 'Calendar file' },
  { value: 'macalendar', label: 'MACalendar' },
];

const opts = { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' };

/** "Tue 10 Nov, 10:00" in this browser's language and zone. */
export function when(iso) {
  try { return new Date(iso).toLocaleString(undefined, opts); } catch { return ''; }
}
