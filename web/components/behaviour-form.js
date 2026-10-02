// behaviour-form.js — the one Behaviour editor used by Settings (app-wide
// Private / Group profiles) and by the chat drawer (per-chat overrides), so
// the two can never drift apart.
//
//   behaviourForm({
//     kind: 'dm' | 'group',
//     values,                 // full profile (Settings) or the chat's *effective* profile (drawer)
//     mode: 'profile' | 'overrides',
//     sources,                // overrides mode: { field: 'chat' | 'default' }
//     defaults,               // overrides mode: the inherited profile (for "Reply to everything" off)
//     meta,                   // /api/behavior/presets response (ranges, enums, defaults)
//     ui, onUpdate,           // persistent UI state { open:Set, adv:bool } + re-render callback
//     onChange(field, value | null),   // null = inherit (overrides mode only)
//     scopeLabel,             // "Group defaults" — used in the "Default" chip tooltip
//     basePreset,             // overrides mode: { label, values } when the chat started from a preset —
//                             // fields still matching it are labelled with the preset's name
//     personaZone,            // the chat persona's time zone, shown next to "Same as the persona"
//   })
//
// Blocks (availability, proactive) are always changed or reset as a whole.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { seg, toggle } from '../ui.js';
import { rangePair, percentSlider, numberField } from './controls.js';
import { weekEditor, describeWeek, normWeek } from './week-editor.js';
import { rangeOf, fmtSec, GROUP_ONLY, FALLBACK_ENUMS, same } from './behaviour-meta.js';
import { wordChips } from './word-chips.js';
import { dialFieldMap, dialDot } from './vibe-dials.js';

export const FORM_GROUPS = [
  { id: 'responsiveness', label: 'Responsiveness', icon: 'gauge', c: 'linear-gradient(135deg,#38bdf8,#2563eb)', sub: 'Whether it answers at all, and how often',
    fields: ['replyPercent', 'replyWhenNameMentioned', 'replyWhenAtMentioned', 'skipWhenOthersMentioned', 'aiJudgement', 'chimeInPercent', 'ignoreLinks', 'reactPercent', 'pauseWhenYouReply', 'maxRepliesPerHour', 'maxRepliesPerDay', 'cooldownSec', 'historyMessages', 'historyChars', 'staleAfterMin'] },
  { id: 'people', label: 'Who it answers', icon: 'group', c: 'linear-gradient(135deg,#fbbf24,#ea580c)', sub: 'Who gets a reply, and words that always or never get one',
    fields: ['respondToAllMaxMembers', 'answerAnyoneWhoAddressesIt', 'maxStreak', 'triggerWords', 'muteWords'] },
  { id: 'timing', label: 'Timing', icon: 'timer', c: 'linear-gradient(135deg,#a78bfa,#7c3aed)', sub: 'Noticing, reading, thinking and typing',
    fields: ['noticeMinSec', 'noticeMaxSec', 'markRead', 'waitForMoreSec', 'burstCapSec', 'thinkMinSec', 'thinkMaxSec', 'distractedPercent', 'distractedMinSec', 'distractedMaxSec', 'typingIndicator', 'typingCharsPerSec', 'typingJitterPercent', 'typingMinSec', 'typingMaxSec'] },
  { id: 'availability', label: 'Active hours', icon: 'calendar', c: 'linear-gradient(135deg,#818cf8,#4338ca)', sub: 'When the persona is around to answer',
    fields: ['availability'] },
  { id: 'shape', label: 'Message shape', icon: 'bubbles', c: 'linear-gradient(135deg,#2dd4bf,#0d9488)', sub: 'One bubble or several, quoting, length',
    fields: ['splitPercent', 'splitMaxParts', 'bubbleGapMinSec', 'bubbleGapMaxSec', 'quoteReplyPercent', 'allowMentions', 'mentionMax', 'tagReplyPercent', 'lengthBias', 'typoPercent', 'typoFixStyle'] },
  { id: 'proactive', label: 'Start conversations', icon: 'megaphone', c: 'linear-gradient(135deg,#f472b6,#db2777)', sub: 'Message first after a quiet spell',
    fields: ['proactive'] },
  { id: 'safety', label: 'Safety', icon: 'shield', c: 'linear-gradient(135deg,#34d399,#059669)', sub: 'Approvals and trick protection',
    fields: ['autoSendSeconds', 'injectionFilter'] },
];

const FILTER_HELP = {
  strict: 'Blocks anything that looks even a little like someone trying to trick the persona. Safest, but may block some harmless messages.',
  balanced: 'Blocks clear attempts to make the persona reveal its instructions or break character. Recommended.',
  off: 'No filtering. The persona may be talked into breaking character.',
};
const OUTSIDE_HELP = {
  queue: 'Messages that arrive outside active hours stay unread and get an answer once the next active window starts.',
  silent: 'Messages outside active hours are remembered but never answered.',
};
const BIAS_LABEL = { shorter: 'Shorter', normal: 'Normal', longer: 'Longer', match: 'Match theirs' };
const TYPO_FIX_LABEL = { correction: 'Send *fix', edit: 'Edit it', none: 'Leave it' };
const TYPO_FIX_HELP = {
  correction: 'Right after, it sends the right word with a star, like *tomorrow — the classic way.',
  edit: 'It quietly edits the message a few seconds later. WhatsApp shows a small "Edited" label.',
  none: 'The typo stays. People usually understand anyway.',
};
const OUTSIDE_LABEL = { queue: 'Reply later', silent: 'Stay silent' };
const FILTER_LABEL = { strict: 'Strict', balanced: 'Balanced', off: 'Off' };

const plural = (n, one, many = one + 's') => `${n} ${n === 1 ? one : many}`;

/** Collapsed-header summary for each group. */
function groupSummary(id, v, kind) {
  switch (id) {
    case 'responsiveness': {
      const bits = [`Answers ${v.replyPercent ?? 100}% of messages`];
      if (kind === 'group') bits.push(v.chimeInPercent >= 100 && !v.aiJudgement ? 'replies to everything' : `chimes in ${v.chimeInPercent ?? 0}%`);
      if (v.maxRepliesPerHour || v.maxRepliesPerDay) bits.push('limits on');
      return bits.join(' · ');
    }
    case 'people': {
      const bits = [];
      if (kind === 'group') bits.push(v.respondToAllMaxMembers ? `Everyone in groups up to ${v.respondToAllMaxMembers}` : 'Only people you pick');
      bits.push(v.maxStreak ? `stops after ${v.maxStreak} in a row` : 'no limit in a row');
      const tw = (v.triggerWords || []).length;
      const mw = (v.muteWords || []).length;
      if (tw) bits.push(plural(tw, 'trigger word'));
      if (mw) bits.push(plural(mw, 'mute word'));
      return bits.join(' · ');
    }
    case 'timing':
      return `Notices in ${fmtSec(v.noticeMinSec)}–${fmtSec(v.noticeMaxSec)} · thinks ${fmtSec(v.thinkMinSec)}–${fmtSec(v.thinkMaxSec)} · types ${v.typingCharsPerSec ?? 0} chars/s`;
    case 'availability': {
      const a = v.availability || {};
      return a.enabled ? `${describeWeek(a.week)} · ${a.outsideHours === 'silent' ? 'silent outside' : 'replies later outside'}` : 'Always around';
    }
    case 'shape':
      return `${v.splitPercent ? `Splits ${v.splitPercent}% of long replies` : 'One bubble per reply'} · ${v.lengthBias === 'match' ? 'length matches theirs' : `${(BIAS_LABEL[v.lengthBias] || 'Normal').toLowerCase()} length`}${kind === 'group' && v.allowMentions ? ' · tags people' : ''}${v.typoPercent ? ` · typos ${v.typoPercent}%` : ''}`;
    case 'proactive': {
      const p = v.proactive || {};
      return p.enabled ? `After ${plural(p.afterHours || 0, 'hour')} of quiet · up to ${p.maxPerDay || 1}/day` : 'Off';
    }
    case 'safety':
      return `Trick protection ${(FILTER_LABEL[v.injectionFilter] || 'Balanced').toLowerCase()} · ${v.autoSendSeconds ? `auto-send after ${fmtSec(v.autoSendSeconds)}` : 'no auto-send'}`;
    default: return '';
  }
}

export function behaviourForm(opts) {
  const {
    kind = 'dm', values: v0, mode = 'profile', sources = {}, defaults = null, meta = null,
    ui = { open: new Set(['responsiveness', 'timing']), adv: false }, onUpdate = () => {}, onChange, scopeLabel = 'defaults',
    basePreset = null, personaZone = '',
  } = opts;
  const v = v0 || {};
  const isGroup = kind === 'group';
  const dialOf = dialFieldMap(meta, kind); // field → vibe dial that sets it (coloured label)
  const ov = mode === 'overrides';
  const E = (meta && meta.enums) || {};
  const enums = {
    ...FALLBACK_ENUMS, ...E,
    outsideHours: E['availability.outsideHours'] || E.outsideHours || FALLBACK_ENUMS.outsideHours,
  };
  const set = (f, val) => onChange && onChange(f, val);
  const R = (f) => rangeOf(meta, f);
  const fromChat = (fields) => fields.some((f) => sources[f] === 'chat');
  const fromPreset = (fields) => !!basePreset && fields.every((f) => sources[f] !== 'chat' || same(v[f], basePreset.values[f]));

  // ── Row chrome ────────────────────────────────────────────────────
  function chrome(fields) {
    if (!ov) return '';
    if (fromChat(fields)) {
      const p = fromPreset(fields);
      return html`<span class=${'inherit-chip custom ' + (p ? 'preset' : '')} title=${p ? `From the ${basePreset.label} preset, for this chat only` : 'Set for this chat only'}>${icon(p ? 'sliders' : 'edit')}${p ? basePreset.label : 'This chat'}</span>
        <button type="button" class="btn btn-ghost btn-icon btn-xs reset-btn" title=${`Go back to the ${scopeLabel}`} aria-label=${`Reset to ${scopeLabel}`}
          @click=${() => { for (const f of fields) set(f, null); }}>${icon('reset')}</button>`;
    }
    return html`<span class="inherit-chip" title=${`Same as the ${scopeLabel}. Change it to customise this chat.`}>Default</span>`;
  }

  function row({ fields, label, help, control, stack = false, groupOnly = false, key, cls = '', readout = '', derived = false }) {
    if (groupOnly && !isGroup) return '';
    const custom = ov && !derived && fromChat(fields);
    return html`<div class=${'field-row bf-row ' + (stack ? 'field-row-stack ' : '') + (custom ? 'is-custom ' : '') + cls} data-key=${key || fields.join('+')}>
      <div class="grow">
        <div class="field-label">${label}${dialDot(meta, fields.map((f) => dialOf[f]).find(Boolean))}${derived ? '' : chrome(fields)}${readout ? html`<span class="bf-readout">${readout}</span>` : ''}</div>
        ${help ? html`<div class="field-help">${help}</div>` : ''}
      </div>
      ${stack ? html`<div class="bf-control">${control}</div>` : html`<div>${control}</div>`}
    </div>`;
  }

  const slider = (f, o = {}) => percentSlider({ value: v[f], min: R(f).min, max: R(f).max, step: o.step || 5, label: o.label || f, suffix: o.suffix ?? '%', left: o.left, right: o.right, format: o.format, onChange: (n) => set(f, n) });
  const num = (f, o = {}) => numberField({ value: v[f], min: R(f).min, max: R(f).max, unit: o.unit || '', label: o.label || f, width: o.width, onChange: (n) => set(f, n) });
  const pair = (a, b, label, o = {}) => rangePair({ lo: v[a], hi: v[b], min: Math.min(R(a).min, R(b).min), max: Math.max(R(a).max, R(b).max), unit: o.unit || 'sec', label, onChange: (lo, hi) => { if (lo !== v[a]) set(a, lo); if (hi !== v[b]) set(b, hi); } });
  const span = (a, b) => (v[a] === v[b] ? fmtSec(v[a]) : `${fmtSec(v[a])} – ${fmtSec(v[b])}`);
  const sw = (f, label) => toggle(!!v[f], (c) => set(f, c), { label });

  // ── Groups ───────────────────────────────────────────────────────
  function responsiveness() {
    const everything = (v.chimeInPercent ?? 0) >= 100 && v.aiJudgement === false;
    const setEverything = (on) => {
      if (on) { set('chimeInPercent', 100); set('aiJudgement', false); return; }
      const d = defaults || (meta && meta.defaults && meta.defaults.group) || {};
      const dEverything = (d.chimeInPercent ?? 0) >= 100 && d.aiJudgement === false;
      if (ov && !dEverything) { set('chimeInPercent', null); set('aiJudgement', null); return; }
      set('chimeInPercent', dEverything || d.chimeInPercent == null ? 40 : d.chimeInPercent);
      set('aiJudgement', true);
    };
    return html`
      ${row({ fields: ['replyPercent'], stack: true, label: 'Reply chance',
        help: html`How often it answers a message at all. Below 100%, some messages are ${v.markRead ? 'seen but never answered (left on read)' : 'never answered'} — like a real person.`,
        control: slider('replyPercent', { label: 'Reply chance' }) })}
      ${isGroup ? html`<div class="bf-sub">${icon('group', 'ic-sm')}In groups</div>` : ''}
      ${row({ groupOnly: true, derived: true, fields: ['chimeInPercent', 'aiJudgement'], key: 'everything', label: 'Reply to everything',
        help: 'Shortcut: answer every single message in the group. Turns the chime-in chance to 100% and the AI judgement off.',
        control: toggle(everything, setEverything, { label: 'Reply to everything' }) })}
      ${row({ groupOnly: true, fields: ['replyWhenNameMentioned'], label: 'Reply when its name comes up', help: "Always answers when someone writes the persona's name.", control: sw('replyWhenNameMentioned', 'Reply when its name comes up') })}
      ${row({ groupOnly: true, fields: ['replyWhenAtMentioned'], label: 'Reply when you are @mentioned', help: 'Always answers when someone tags you with @.', control: sw('replyWhenAtMentioned', 'Reply when @mentioned') })}
      ${row({ groupOnly: true, fields: ['skipWhenOthersMentioned'], label: 'Stay out when others are @mentioned', help: 'If a message tags somebody else, let them answer it.', control: sw('skipWhenOthersMentioned', 'Stay out when others are mentioned') })}
      ${!everything ? html`
        ${row({ groupOnly: true, fields: ['aiJudgement'], label: 'Let the AI decide when to chime in', help: 'For everything else, the AI reads along and joins in only when it makes sense. Off = decided by the chance below alone.', control: sw('aiJudgement', 'Let the AI decide') })}
        ${row({ groupOnly: true, fields: ['chimeInPercent'], stack: true, label: 'Chime-in chance', help: "Of the messages that aren't aimed at the persona, how often it joins in.", control: slider('chimeInPercent', { label: 'Chime-in chance' }) })}` : ''}
      ${isGroup ? html`<div class="bf-sub">${icon('message', 'ic-sm')}Every chat</div>` : ''}
      ${row({ fields: ['reactPercent'], stack: true, label: 'React instead of replying', help: "When it decides not to answer, sometimes leave a reaction on the message instead (picked from the persona's favourite emoji).", control: slider('reactPercent', { label: 'React instead of replying' }) })}
      ${row({ fields: ['ignoreLinks'], label: 'Ignore links', help: "Don't reply to messages that are only a link.", control: sw('ignoreLinks', 'Ignore links') })}
      ${row({ fields: ['pauseWhenYouReply'], label: 'Stand down when you answer yourself', help: 'If you reply from your phone while it is getting ready, it drops its own reply.', control: sw('pauseWhenYouReply', 'Stand down when you answer') })}
      <div class="bf-sub" id=${'bf-limits-' + kind}>${icon('hand', 'ic-sm')}Limits</div>
      ${row({ fields: ['maxRepliesPerHour'], label: 'Most replies per hour', help: '0 = no limit.', control: num('maxRepliesPerHour', { label: 'Most replies per hour' }) })}
      ${row({ fields: ['maxRepliesPerDay'], label: 'Most replies per day', help: '0 = no limit. Once reached it stays quiet until tomorrow.', control: num('maxRepliesPerDay', { label: 'Most replies per day' }) })}
      ${row({ fields: ['cooldownSec'], label: 'Minimum gap between replies', help: 'Never sends two replies closer together than this.', control: num('cooldownSec', { unit: 'sec', label: 'Minimum gap between replies' }) })}
      <button type="button" class="disclosure bf-adv" aria-expanded=${String(!!ui.adv)} @click=${() => { ui.adv = !ui.adv; onUpdate(); }}>
        ${icon('brain', 'ic-sm')}<span>Memory and old messages</span><span class="faint small">${ui.adv ? '' : `reads the last ${v.historyMessages ?? 0} messages`}</span>${icon(ui.adv ? 'chevron-up' : 'chevron-down', 'ic-sm')}
      </button>
      ${ui.adv ? html`
        ${row({ fields: ['historyMessages'], label: 'Memory: messages', help: 'How many recent messages it reads before replying.', control: num('historyMessages', { label: 'Memory: messages' }) })}
        ${row({ fields: ['historyChars'], label: 'Memory: characters', help: 'A cap on how much text that is in total (keeps local models fast).', control: num('historyChars', { label: 'Memory: characters', width: 110 }) })}
        ${row({ fields: ['staleAfterMin'], label: 'Treat old messages as background', help: 'Messages older than this (for example while Doppel was off) are remembered but not answered.', control: num('staleAfterMin', { unit: 'min', label: 'Old message threshold' }) })}` : ''}`;
  }

  function people() {
    return html`
      ${isGroup ? html`<div class="bf-sub">${icon('group', 'ic-sm')}In groups</div>` : ''}
      ${row({ groupOnly: true, fields: ['respondToAllMaxMembers'], label: 'Answer everyone in groups up to',
        help: html`Smaller groups answer everyone by default. In bigger ones it only answers the people you pick in each chat's <b>People</b> list. 0 = always only the people you pick.`,
        control: num('respondToAllMaxMembers', { unit: 'members', label: 'Answer everyone in groups up to' }) })}
      ${row({ groupOnly: true, fields: ['answerAnyoneWhoAddressesIt'], label: 'Still answer anyone who says its name or tags it', help: "People you didn't pick still get an answer when they talk to the persona directly.", control: sw('answerAnyoneWhoAddressesIt', 'Still answer anyone who addresses it') })}
      ${isGroup ? html`<div class="bf-sub">${icon('message', 'ic-sm')}Every chat</div>` : ''}
      ${row({ fields: ['maxStreak'], label: 'Stop after this many replies in a row', help: 'Replies to the same person while nobody else speaks. Stops two bots from answering each other forever; starts over when someone else speaks or after 30 minutes of quiet. 0 = no limit.', control: num('maxStreak', { unit: 'replies', label: 'Stop after this many replies in a row' }) })}
      ${row({ fields: ['triggerWords'], stack: true, label: 'Always reply to these words', help: 'A message containing one of these (whole words, any case) gets an answer without the chance rolls — from anyone the persona answers in that chat.',
        control: wordChips({ words: v.triggerWords, onChange: (w) => set('triggerWords', w), label: 'Trigger words', tone: 'green', placeholder: 'e.g. pizza, game night — press Enter' }) })}
      ${row({ fields: ['muteWords'], stack: true, label: 'Never reply to these words', help: 'Messages containing one of these are remembered but never answered. These win over the words above.',
        control: wordChips({ words: v.muteWords, onChange: (w) => set('muteWords', w), label: 'Mute words', tone: 'red', placeholder: 'e.g. spoiler — press Enter' }) })}`;
  }

  function timing() {
    return html`
      ${row({ fields: ['noticeMinSec', 'noticeMaxSec'], stack: true, readout: span('noticeMinSec', 'noticeMaxSec'), label: 'Time before it notices', help: 'How long until it looks at the phone after a message arrives — a random time in this range.', control: pair('noticeMinSec', 'noticeMaxSec', 'Time before it notices') })}
      ${row({ fields: ['markRead'], label: 'Show “seen” ticks', help: "Marks messages as read (blue ticks) when it looks. If read receipts are off in your WhatsApp privacy settings, nobody sees them anyway.", control: sw('markRead', 'Show seen ticks') })}
      ${row({ fields: ['waitForMoreSec'], label: 'Wait for more messages', help: 'After seeing a message, waits this long in case another follows, so a burst of messages gets one reply.', control: num('waitForMoreSec', { unit: 'sec', label: 'Wait for more messages' }) })}
      ${row({ fields: ['burstCapSec'], label: 'Never wait longer than', help: 'Even if they keep sending messages, start replying after this long.', control: num('burstCapSec', { unit: 'sec', label: 'Never wait longer than' }) })}
      ${row({ fields: ['thinkMinSec', 'thinkMaxSec'], stack: true, readout: span('thinkMinSec', 'thinkMaxSec'), label: 'Think time', help: 'A pause before typing starts, as if reading and deciding what to say. The AI writes the reply during this time.', control: pair('thinkMinSec', 'thinkMaxSec', 'Think time') })}
      ${row({ fields: ['distractedPercent', 'distractedMinSec', 'distractedMaxSec'], stack: true, readout: v.distractedPercent ? `${v.distractedPercent}% · +${span('distractedMinSec', 'distractedMaxSec')}` : 'Never', label: 'Gets distracted',
        help: 'Sometimes gets pulled away before answering and comes back a bit later.',
        control: html`${slider('distractedPercent', { label: 'Chance of getting distracted', left: 'Never', right: 'Often' })}
          <div class=${'bf-nested ' + (v.distractedPercent ? '' : 'is-dim')}>
            <div class="bf-nested-label">Extra delay when it happens</div>
            ${pair('distractedMinSec', 'distractedMaxSec', 'Extra delay when distracted')}
          </div>` })}
      ${row({ fields: ['typingIndicator'], label: 'Show “typing…”', help: 'They see you typing while the reply is being written.', control: sw('typingIndicator', 'Show typing') })}
      ${row({ fields: ['typingCharsPerSec'], stack: true, label: 'Typing speed', help: 'Characters per second. Most people manage 4–8 on a phone.',
        control: slider('typingCharsPerSec', { step: 1, label: 'Typing speed', suffix: '', format: (n) => `${n} chars/s`, left: 'Slow', right: 'Fast' }) })}
      ${row({ fields: ['typingJitterPercent'], stack: true, label: 'Speed variation', help: 'How much the typing speed changes from one message to the next.',
        control: slider('typingJitterPercent', { label: 'Speed variation', left: 'Steady', right: 'Erratic' }) })}
      ${row({ fields: ['typingMinSec', 'typingMaxSec'], stack: true, readout: span('typingMinSec', 'typingMaxSec'), label: 'Typing time limits', help: 'However long or short the reply, typing lasts at least and at most this long.', control: pair('typingMinSec', 'typingMaxSec', 'Typing time limits') })}`;
  }

  function availability() {
    const a = v.availability || {};
    const setA = (patch) => set('availability', { ...a, week: normWeek(a.week), ...patch });
    const outs = enums.outsideHours || ['queue', 'silent'];
    return html`
      ${row({ fields: ['availability'], label: 'Active hours', help: 'Only answer at certain times of day, like someone who sleeps and works. Turn off to be around 24/7.',
        control: toggle(!!a.enabled, (c) => setA({ enabled: c }), { label: 'Active hours' }) })}
      ${a.enabled ? html`
        <div class="bf-block">
          ${weekEditor({ week: a.week, timezone: a.timezone || '', personaZone, onWeek: (week) => setA({ week }), onTimezone: (timezone) => setA({ timezone }) })}
        </div>
        <div class="field-row field-row-stack bf-row" data-key="outside">
          <div class="grow"><div class="field-label">Outside active hours</div><div class="field-help">${OUTSIDE_HELP[a.outsideHours] || OUTSIDE_HELP.queue}</div></div>
          <div>${seg(outs.map((o) => ({ value: o, label: OUTSIDE_LABEL[o] || o, icon: o === 'queue' ? 'clock' : 'snooze' })), a.outsideHours || 'queue', (x) => setA({ outsideHours: x }), { label: 'Outside active hours' })}</div>
        </div>
        ${a.outsideHours !== 'silent' ? html`<div class="field-row bf-row" data-key="catchup">
          <div class="grow"><div class="field-label">Catch-up delay</div><div class="field-help">When the next active window starts, wait up to this long before answering — nobody replies at exactly 09:00.</div></div>
          ${numberField({ value: a.catchUpMaxMin, ...R('availability.catchUpMaxMin'), unit: 'min', label: 'Catch-up delay', onChange: (n) => setA({ catchUpMaxMin: n }) })}
        </div>` : ''}` : ''}`;
  }

  function shape() {
    const parts = [];
    for (let i = R('splitMaxParts').min; i <= R('splitMaxParts').max; i++) parts.push({ value: i, label: String(i) });
    const bias = enums.lengthBias || ['shorter', 'normal', 'longer'];
    const tagMax = [];
    for (let i = R('mentionMax').min; i <= R('mentionMax').max; i++) tagMax.push({ value: i, label: String(i) });
    return html`
      ${row({ fields: ['splitPercent'], stack: true, label: 'Split long replies', help: 'How often a longer reply is sent as a few short messages instead of one block. Short replies are never split.', control: slider('splitPercent', { label: 'Split long replies' }) })}
      ${v.splitPercent ? html`
        ${row({ fields: ['splitMaxParts'], label: 'At most', help: 'The most bubbles one reply is split into.', control: html`<span class="row gap-8">${seg(parts, v.splitMaxParts, (n) => set('splitMaxParts', n), { cls: 'seg-sm', label: 'At most bubbles' })}<span class="faint small">bubbles</span></span>` })}
        ${row({ fields: ['bubbleGapMinSec', 'bubbleGapMaxSec'], stack: true, readout: span('bubbleGapMinSec', 'bubbleGapMaxSec'), label: 'Pause between bubbles', help: 'Time between the separate messages, on top of typing each one.', control: pair('bubbleGapMinSec', 'bubbleGapMaxSec', 'Pause between bubbles') })}` : ''}
      ${row({ groupOnly: true, fields: ['quoteReplyPercent'], stack: true, label: 'Quote the message it answers', help: "In a busy group, sometimes reply to a specific message so it's clear what the answer is about.", control: slider('quoteReplyPercent', { label: 'Quote the message it answers' }) })}
      ${row({ groupOnly: true, fields: ['allowMentions'], label: 'Tag people when relevant', help: 'Lets it write @Name to talk to someone directly. They get a notification, like a real tag. It tags rarely and never everyone.', control: sw('allowMentions', 'Tag people when relevant') })}
      ${isGroup && v.allowMentions ? html`
        ${row({ groupOnly: true, fields: ['mentionMax'], label: 'At most', help: 'The most people one reply may tag.',
          control: html`<span class="row gap-8">${seg(tagMax, v.mentionMax, (n) => set('mentionMax', n), { cls: 'seg-sm', label: 'At most people tagged' })}<span class="faint small">${v.mentionMax === 1 ? 'person' : 'people'}</span></span>` })}
        ${row({ groupOnly: true, fields: ['tagReplyPercent'], stack: true, label: 'Tag the person it answers', help: "When several people are talking, sometimes starts with @Name so it's clear who it's answering.", control: slider('tagReplyPercent', { label: 'Tag the person it answers' }) })}` : ''}
      ${row({ fields: ['lengthBias'], label: 'Reply length', help: 'Shorter or longer than the persona normally writes, or Match theirs: about as long as the message it answers (a word gets a few words, a long message a longer reply).',
        control: seg(bias.map((b) => ({ value: b, label: BIAS_LABEL[b] || b })), v.lengthBias || 'normal', (x) => set('lengthBias', x), { label: 'Reply length' }) })}
      ${row({ fields: ['typoPercent'], stack: true, label: 'Typos now and then', help: 'Chance that a reply has one small slip — two letters swapped or a missed key — like real thumbs. Only in automatic replies, never in ones you approve.',
        control: slider('typoPercent', { step: 1, label: 'Typos now and then', left: 'Never', right: 'Clumsy' }) })}
      ${v.typoPercent ? row({ fields: ['typoFixStyle'], stack: true, label: 'After a typo', help: TYPO_FIX_HELP[v.typoFixStyle] || TYPO_FIX_HELP.correction,
        control: seg((enums.typoFixStyle || ['correction', 'edit', 'none']).map((x) => ({ value: x, label: TYPO_FIX_LABEL[x] || x })), v.typoFixStyle || 'correction', (x) => set('typoFixStyle', x), { label: 'After a typo' }) }) : ''}`;
  }

  function proactive() {
    const p = v.proactive || {};
    const setP = (patch) => set('proactive', { ...p, ...patch });
    return html`
      <div class="banner warn bf-warn">${icon('warning')}<div><strong>Sends messages nobody asked for — use sparingly.</strong> Lots of unprompted messages can get a WhatsApp account flagged. It only ever checks in on chats that have talked before.</div></div>
      ${row({ fields: ['proactive'], label: 'Start conversations', help: 'If a chat has been quiet for a while, the persona sends a natural opener, the way a friend checks in.',
        control: toggle(!!p.enabled, (c) => setP({ enabled: c }), { label: 'Start conversations' }) })}
      ${p.enabled ? html`
        <div class="field-row bf-row" data-key="after"><div class="grow"><div class="field-label">After a quiet spell of</div><div class="field-help">Counted from the last message by anyone in the chat.</div></div>
          ${numberField({ value: p.afterHours, ...R('proactive.afterHours'), unit: 'hours', label: 'After a quiet spell of', onChange: (n) => setP({ afterHours: n }) })}</div>
        <div class="field-row bf-row" data-key="maxday"><div class="grow"><div class="field-label">At most per day</div><div class="field-help">A hard cap on check-ins in this kind of chat.</div></div>
          ${numberField({ value: p.maxPerDay, ...R('proactive.maxPerDay'), unit: 'per day', label: 'At most per day', onChange: (n) => setP({ maxPerDay: n }) })}</div>
        <div class="field-row bf-row" data-key="spread"><div class="grow"><div class="field-label">Random extra wait</div><div class="field-help">Adds up to this much so check-ins never happen at an exact, robotic time.</div></div>
          ${numberField({ value: p.spreadMinutes, ...R('proactive.spreadMinutes'), unit: 'min', label: 'Random extra wait', onChange: (n) => setP({ spreadMinutes: n }) })}</div>` : ''}`;
  }

  function safety() {
    const filters = enums.injectionFilter || ['strict', 'balanced', 'off'];
    return html`
      ${row({ fields: ['autoSendSeconds'], label: 'Auto-send approvals after', help: 'If a reply waits in Approvals this long, it is sent anyway. 0 = never.', control: num('autoSendSeconds', { unit: 'sec', label: 'Auto-send approvals after', width: 104 }) })}
      ${row({ fields: ['injectionFilter'], stack: true, label: 'Trick protection', help: FILTER_HELP[v.injectionFilter] || FILTER_HELP.balanced,
        control: seg(filters.map((f) => ({ value: f, label: FILTER_LABEL[f] || f, icon: f === 'strict' ? 'shield' : '' })), v.injectionFilter || 'balanced', (x) => set('injectionFilter', x), { label: 'Trick protection' }) })}
      <div class="bf-note small muted">${icon('info', 'ic-sm')}<span>Daily reply limits live under <button type="button" class="link-btn" @click=${() => {
        ui.open.add('responsiveness'); onUpdate();
        requestAnimationFrame(() => { const el = document.getElementById('bf-limits-' + kind); if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' }); });
      }}>Responsiveness → Limits</button>.</span></div>`;
  }

  const BODY = { responsiveness, people, timing, availability, shape, proactive, safety };

  return html`<div class=${'behaviour-form ' + (ov ? 'mode-overrides' : 'mode-profile')}>
    ${FORM_GROUPS.map((g) => {
      const open = ui.open.has(g.id);
      const relevant = g.fields.filter((f) => isGroup || !GROUP_ONLY.has(f));
      const customised = ov ? relevant.filter((f) => sources[f] === 'chat' && !fromPreset([f])).length : 0;
      const fromP = ov && basePreset ? relevant.filter((f) => sources[f] === 'chat' && fromPreset([f])).length : 0;
      return html`<section class=${'behaviour-group ' + (open ? 'open' : '')} data-key=${'bg-' + g.id}>
        <button type="button" class="bg-head" aria-expanded=${String(open)} @click=${() => { if (open) ui.open.delete(g.id); else ui.open.add(g.id); onUpdate(); }}>
          <span class="bg-icon" style=${`--c:${g.c}`}>${icon(g.icon)}</span>
          <span class="grow bg-text">
            <span class="bg-title">${g.label}${customised ? html`<span class="inherit-chip custom">${customised} customised</span>` : ''}${fromP ? html`<span class="inherit-chip custom preset">${basePreset.label}</span>` : ''}</span>
            <span class="bg-sum">${open ? g.sub : groupSummary(g.id, v, kind)}</span>
          </span>
          ${icon('chevron-down', 'bg-chev')}
        </button>
        ${open ? html`<div class="bg-body">${BODY[g.id]()}</div>` : ''}
      </section>`;
    })}
  </div>`;
}
