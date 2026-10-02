// reply-timeline.js — "How a reply unfolds": a few example runs of the reply
// pipeline for one incoming message, drawn on a shared time axis.
//
//   const tl = createTimeline({ onUpdate });       // one per page / drawer
//   tl.view({ kind: 'dm'|'group', profile })       // template; refetches (debounced) when inputs change
//   tl.reroll()                                    // fresh random samples
//
// Data comes from POST /api/behavior/sample, which runs the engine's own
// planner, so the picture is exactly what the bot will do.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { debounce } from '../util.js';
import { fmtSec, sayDuration, sayShare } from './behaviour-meta.js';

const SAMPLES = 5;

export const PHASES = {
  notice:     { label: 'Not seen yet', help: "Hasn't looked at the phone yet" },
  seen:       { label: 'Seen', help: 'Blue ticks appear', mark: true },
  wait:       { label: 'Waiting for more', help: 'Gives them a moment in case another message follows' },
  think:      { label: 'Thinking', help: 'Reading and deciding what to say' },
  distracted: { label: 'Distracted', help: 'Got pulled away for a bit' },
  typing:     { label: 'Typing', help: '"typing…" shows in the chat' },
  gap:        { label: 'Pause', help: 'Short pause between bubbles' },
  send:       { label: 'Sent', help: 'A message bubble arrives', mark: true },
};
const LEGEND = ['notice', 'seen', 'wait', 'think', 'distracted', 'typing', 'gap', 'send'];

const NICE = [1, 2, 5, 10, 15, 20, 30, 60, 120, 300, 600, 900, 1200, 1800, 3600, 7200, 10800, 21600, 43200, 86400];

function axisFor(maxSec) {
  const max = Math.max(5, maxSec);
  const step = NICE.find((s) => max / s <= 5) || 86400;
  const top = Math.ceil(max / step) * step;
  const ticks = [];
  for (let t = 0; t <= top + 1e-9; t += step) ticks.push(t);
  return { top, ticks };
}

function tickLabel(t) {
  if (t === 0) return '0';
  return fmtSec(t);
}

/** Plain-English summary under the chart. */
function sentences(summary, profile, kind) {
  const out = [];
  const s = summary || {};
  const p = profile || {};
  if (s.maxTotalSec != null) {
    const lo = fmtSec(rough(s.minTotalSec || 0));
    const hi = fmtSec(rough(s.maxTotalSec || 0));
    const range = lo === hi ? `about ${lo}` : `${lo}–${hi}`;
    const median = s.medianTotalSec != null ? ` (typically ${sayDuration(s.medianTotalSec)})` : '';
    out.push({ strong: true, text: s.maxTotalSec < 2 ? 'Replies almost instantly.' : `Usually replies ${range} after the last message${median}.` });
  }
  const extras = [];
  if (s.splitShare > 0 && p.splitMaxParts > 1) {
    const parts = p.splitMaxParts > 2 ? `2–${p.splitMaxParts}` : '2';
    extras.push(`${cap(sayShare(s.splitShare))} long replies arrive as ${parts} separate bubbles.`);
  }
  if (p.replyPercent != null && p.replyPercent < 100) {
    const miss = 100 - p.replyPercent;
    extras.push(`Leaves about ${miss}% of messages ${p.markRead ? 'on read' : 'unanswered'}.`);
  }
  if (kind === 'group' && p.chimeInPercent != null) {
    extras.push(p.chimeInPercent >= 100 && !p.aiJudgement
      ? 'Replies to every message in the group.'
      : `Joins in on about ${p.chimeInPercent}% of messages that aren't aimed at it.`);
  }
  if (p.reactPercent > 0) extras.push(`Sometimes (${p.reactPercent}%) reacts instead of replying.`);
  if (p.availability && p.availability.enabled) {
    extras.push(p.availability.outsideHours === 'silent'
      ? 'Only answers during active hours.'
      : 'Outside active hours, replies when the next active window starts.');
  }
  if (extras.length) out.push({ strong: false, text: extras.join(' ') });
  return out;
}
/** Round for prose: seconds under a minute, 10s steps under 10 min, then minutes. */
const rough = (sec) => (sec < 60 ? Math.round(sec) : sec < 600 ? Math.round(sec / 10) * 10 : Math.round(sec / 60) * 60);
const cap = (s) => s.charAt(0).toUpperCase() + s.slice(1);

function bubbleMark() {
  // A tiny chat bubble with a tail, centred on (0,0).
  return 'M-5.5 -5.5h11a2.6 2.6 0 0 1 2.6 2.6v3.6a2.6 2.6 0 0 1-2.6 2.6H-1.2L-4.6 6v-2.7h-.9a2.6 2.6 0 0 1-2.6-2.6v-3.6a2.6 2.6 0 0 1 2.6-2.6Z';
}

function sentTitle(m, s) {
  const lead = `Sent at ${fmtSec(m.at)}`;
  const quoted = s.quoted && m.bubble === (s.bubbles || [])[0] ? ' as a reply to their message' : '';
  return m.bubble && m.bubble.text ? `${lead}${quoted}: “${m.bubble.text}”` : lead + quoted;
}

function chart(samples, gen) {
  const totals = samples.map((s) => s.totalSec || (s.phases || []).reduce((a, p) => a + (p.sec || 0), 0));
  const { top, ticks } = axisFor(Math.max(...totals, 1));
  const ROW = 18;
  const GAP = 12;
  const PAD_T = 16;
  const AXIS = 22;
  const H = PAD_T + samples.length * (ROW + GAP) - GAP + AXIS;
  const X = (t) => `${Math.min(100, (t / top) * 100).toFixed(3)}%`;
  const W = (t) => `${Math.max(0, Math.min(100, (t / top) * 100)).toFixed(3)}%`;

  return html`<svg class="tl-svg" width="100%" height=${H} data-key=${'tl-' + gen} role="img"
      aria-label=${`Example reply timelines. ${totals.map((t, i) => `Try ${i + 1}: ${fmtSec(t)}`).join('. ')}`}>
    <defs>
      <pattern id="tl-hatch" width="5" height="5" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
        <rect width="5" height="5" class="tl-hatch-bg"></rect><line x1="0" y1="0" x2="0" y2="5" class="tl-hatch-line"></line>
      </pattern>
    </defs>
    <svg x="0" y="0" width="88%" height=${H} overflow="visible">
      ${ticks.map((t, i) => html`<line class="tl-grid" x1=${X(t)} x2=${X(t)} y1=${PAD_T - 8} y2=${H - AXIS + 4}></line>
        <text class="tl-tick" x=${X(t)} y=${H - 5} text-anchor=${i === 0 ? 'start' : i === ticks.length - 1 ? 'end' : 'middle'}>${tickLabel(t)}</text>`)}
      ${samples.map((s, ri) => {
        const y = PAD_T + ri * (ROW + GAP);
        let at = 0;
        const segs = [];
        const marks = [];
        let bubble = 0;
        for (const ph of s.phases || []) {
          const sec = Math.max(0, ph.sec || 0);
          if (typeof ph.startSec === 'number') at = ph.startSec;
          const meta = PHASES[ph.name] || { label: ph.name };
          if (ph.name === 'seen' || ph.name === 'send') {
            marks.push({ name: ph.name, at, sec, label: meta.label, bubble: ph.name === 'send' ? (s.bubbles || [])[bubble++] : null });
          } else if (sec > 0) {
            segs.push({ name: ph.name, at, sec, label: meta.label });
          }
          at += sec;
        }
        const delay = (t) => Math.round(ri * 70 + (t / top) * 650);
        return html`<g class="tl-row">
          <rect class="tl-track" x="0" y=${y} width="100%" height=${ROW} rx="6"></rect>
          ${segs.map((g) => html`<rect class=${'tl-seg tl-seg-' + g.name} x=${X(g.at)} y=${y} width=${W(g.sec)} height=${ROW} rx="4"
              style=${`--d:${delay(g.at)}ms`}><title>${g.label} · ${fmtSec(g.sec)} (from ${fmtSec(g.at)})</title></rect>`)}
          ${marks.map((m) => m.name === 'seen'
            ? html`<svg x=${X(m.at)} y=${y + ROW / 2} overflow="visible"><g class="tl-mark tl-seen" style=${`--d:${delay(m.at) + 120}ms`}>
                <title>Seen at ${fmtSec(m.at)} — blue ticks appear</title>
                <circle r="9" class="tl-mark-bg"></circle>
                <path d="M-5.8 .2l2.4 2.4 5-5.3M-1.3 2.3l.5.5 5-5.3" class="tl-tick-path"></path>
              </g></svg>`
            : html`<svg x=${X(m.at)} y=${y + ROW / 2} overflow="visible"><g class="tl-mark tl-send" style=${`--d:${delay(m.at) + 160}ms`}>
                <title>${sentTitle(m, s)}</title>
                <path d=${bubbleMark()} class="tl-bubble"></path>
              </g></svg>`)}
        </g>`;
      })}
    </svg>
    ${samples.map((s, ri) => html`<text class="tl-total" x="100%" y=${PAD_T + ri * (ROW + GAP) + ROW / 2 + 4} text-anchor="end">${fmtSec(totals[ri])}</text>`)}
  </svg>`;
}

export function createTimeline({ onUpdate } = {}) {
  const st = { key: '', data: null, loading: false, error: '', gen: 0, ctl: null, last: null };
  const upd = () => onUpdate && onUpdate();

  async function fetchNow() {
    const input = st.last;
    if (!input || !input.profile) return;
    if (st.ctl) st.ctl.abort();
    const ctl = new AbortController();
    st.ctl = ctl;
    st.loading = true; upd();
    try {
      // No text: the server picks a sample reply whose length follows lengthBias.
      const r = await api.behavior.sample({ kind: input.kind, profile: input.profile, samples: SAMPLES }, { quiet: true, signal: ctl.signal });
      if (ctl.signal.aborted) return;
      st.data = r && Array.isArray(r.samples) ? r : { samples: [], summary: null };
      st.error = '';
      st.gen++;
    } catch (e) {
      if (e && e.name === 'AbortError') return;
      st.error = (e && e.message) || 'Could not draw the preview';
    } finally {
      if (st.ctl === ctl) { st.loading = false; st.ctl = null; upd(); }
    }
  }
  const fetchSoon = debounce(fetchNow, 400);

  function view({ kind, profile, title = 'How a reply unfolds', compact = false }) {
    const key = kind + '|' + JSON.stringify(profile || null);
    if (profile && key !== st.key) {
      const first = !st.key;
      st.key = key;
      st.last = { kind, profile };
      if (first) fetchNow(); else fetchSoon();
    }
    const d = st.data;
    const lines = d ? sentences(d.summary, profile, kind) : [];
    return html`<div class=${'timeline ' + (compact ? 'compact ' : '') + (st.loading && d ? 'is-loading' : '')}>
      <div class="tl-head">
        <span class="tl-icon">${icon('timer')}</span>
        <div class="grow">
          <div class="tl-title">${title}</div>
          <div class="tl-sub">${compact ? `${SAMPLES} example runs for one message` : `${SAMPLES} example replies to one message — every run is a little different`}</div>
        </div>
        <button type="button" class="btn btn-glass btn-sm" ?disabled=${!profile || st.loading} title="Draw new random examples"
          @click=${() => { fetchSoon.cancel(); fetchNow(); }}>${icon('dice')}Re-roll</button>
      </div>
      ${st.error && !d ? html`<div class="banner danger small mt-12">${icon('warning')}<div>${st.error}</div></div>`
        : !d ? html`<div class="tl-skeleton">${[0, 1, 2, 3, 4].map(() => html`<div class="skeleton"></div>`)}</div>`
        : !d.samples.length ? html`<div class="small muted mt-12">No example could be drawn for these settings.</div>`
        : chart(d.samples, st.gen)}
      ${d && d.samples.length ? html`<div class="tl-legend" aria-hidden="true">
        ${LEGEND.map((k) => html`<span class="tl-key" title=${PHASES[k].help}>${k === 'seen' ? html`<span class="tl-sw-seen">${icon('check-double')}</span>` : html`<i class=${'tl-sw tl-sw-' + k}></i>`}${PHASES[k].label}</span>`)}
      </div>` : ''}
      ${lines.length ? html`<div class="tl-summary">${lines.map((l) => l.strong ? html`<p class="tl-lead">${l.text}</p>` : html`<p>${l.text}</p>`)}</div>` : ''}
    </div>`;
  }

  return {
    view,
    reroll: () => { fetchSoon.cancel(); fetchNow(); },
    destroy: () => { fetchSoon.cancel(); if (st.ctl) st.ctl.abort(); },
  };
}
