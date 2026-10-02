// goal-controls.js — how a persona pursues its goal (persona editor + chat drawer).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { seg, toggle, field, fieldRow } from '../ui.js';
import { clockTime, relTime } from '../util.js';
import { helpTip } from './help-tip.js';

export const GOAL_STYLES = [
  { value: 'subtle', label: 'Subtle', title: 'Keeps it secret and steers slowly' },
  { value: 'balanced', label: 'Balanced', title: 'Asks about it naturally, never says why' },
  { value: 'direct', label: 'Direct', title: 'Goes for it openly' },
];

const STYLE_HELP = {
  subtle: 'Keeps the goal secret and steers slowly — sets things up so it happens on its own.',
  balanced: 'May bring the topic up with a natural question, but never says why it asks.',
  direct: 'Goes for it openly and keeps coming back to it.',
};

export const goalStyleLabel = (v) => (GOAL_STYLES.find((s) => s.value === v) || GOAL_STYLES[0]).label;
export const goalStyleHelp = (v) => STYLE_HELP[v] || STYLE_HELP.subtle;

const PLAN_HELP = 'Before each reply it privately picks its next small step. Slightly slower, much more tactful.';

/** Persona editor: style, plan ahead, what to do once reached. form/set are the editor's. */
export function personaGoalControls(f, set) {
  const style = f.goalStyle || 'subtle';
  const plan = f.goalPlanAhead !== false;
  const after = f.goalAfterReached || 'relax';
  return html`<div class="goal-controls">
    ${field({ label: 'How they pursue it', icon: 'wand', help: goalStyleHelp(style), children: seg(GOAL_STYLES, style, (v) => set('goalStyle', v), { label: 'How they pursue the goal' }) })}
    ${fieldRow({ label: html`Plan ahead ${helpTip('plan-ahead')}`, help: PLAN_HELP, control: toggle(plan, (v) => set('goalPlanAhead', v), { label: 'Plan ahead' }) })}
    ${field({ label: 'Once it happens', help: after === 'continue' ? 'Keeps gently steering towards it.' : 'Stops pursuing it and just chats.', children: seg([
      { value: 'relax', label: 'Just chat' }, { value: 'continue', label: 'Keep going' },
    ], after, (v) => set('goalAfterReached', v), { label: 'Once the goal is reached' }) })}
  </div>`;
}

/** "Working on it" / "Goal reached 20:14" pill for a chat's goal view. */
export function goalStatusPill(g) {
  if (!g) return '';
  if (g.state === 'reached') {
    const at = g.reachedAt ? clockTime(g.reachedAt) : '';
    return html`<span class="chip chip-sm chip-green goal-pill" title=${g.evidence ? `“${g.evidence}”` : 'Reached'}>${icon('check', 'ic-sm')}Goal reached${at ? ' ' + at : ''}</span>`;
  }
  return html`<span class="chip chip-sm goal-pill" title="The persona is steering towards this goal">${icon('target', 'ic-sm')}Working on it</span>`;
}

/**
 * Chat drawer: per-chat style / plan-ahead overrides plus status.
 * g = ChatGoal from GET /api/chats; c = the assignment; p = its persona.
 * onPatch(body) sends PATCH fields (goalStyle / goalPlanAhead, null = persona's); onReset() resets progress.
 */
export function chatGoalControls({ g, c, p, onPatch, onReset }) {
  const pStyle = (p && p.goalStyle) || 'subtle';
  const pPlan = !(p && p.goalPlanAhead === false);
  const style = c.goalStyle || '';
  const planOverride = c.goalPlanAhead;
  const plan = planOverride == null ? pPlan : planOverride;
  const who = p ? p.name : 'the persona';
  const styleOpts = [{ value: '', label: `Like ${who}`, title: `${goalStyleLabel(pStyle)} — ${who}'s setting` }, ...GOAL_STYLES];
  return html`<div class="goal-controls">
    ${g ? html`<div class="goal-status row gap-8 mt-8">
      ${goalStatusPill(g)}
      ${g.state === 'reached' && g.evidence ? html`<span class="small muted ellipsis grow" title=${g.evidence}>“${g.evidence}”</span>` : html`<span class="grow"></span>`}
      ${g.state === 'reached' || g.lastPlan ? html`<button class="btn btn-ghost btn-sm" @click=${onReset} title="Forget the progress and start pursuing the goal again">${icon('reset')}Reset</button>` : ''}
    </div>
    ${g.state !== 'reached' && g.lastPlan ? html`<div class="field-help goal-plan">${icon('brain', 'ic-sm')}Next move: ${g.lastPlan}${g.lastPlanAt ? html` <span class="faint">· ${relTime(g.lastPlanAt)}</span>` : ''}</div>` : ''}` : ''}
    ${field({ label: html`How to pursue it ${helpTip('goal-style')}`, help: goalStyleHelp(style || pStyle), cls: 'mt-12', children: seg(styleOpts, style, (v) => onPatch({ goalStyle: v || null }), { label: 'How to pursue the goal' }) })}
    ${fieldRow({ label: html`Plan ahead ${helpTip('plan-ahead')}`, help: planOverride == null ? `${PLAN_HELP} Following ${who} (${pPlan ? 'on' : 'off'}).` : PLAN_HELP,
      control: html`<div class="row gap-6">
        ${planOverride != null ? html`<button class="link-btn small" @click=${() => onPatch({ goalPlanAhead: null })} title=${`Use ${who}'s setting`}>Use ${who}'s</button>` : ''}
        ${toggle(plan, (v) => onPatch({ goalPlanAhead: v === pPlan ? null : v }), { label: 'Plan ahead' })}
      </div>` })}
  </div>`;
}
