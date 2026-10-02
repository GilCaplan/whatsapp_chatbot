// drawer/goal.js — the chat panel's "Goal" tab: the chat's goal text, how it
// is pursued (goal-controls.js) and "Start a conversation" (initiate-controls.js).
// Wave 3 owner: Engineer C (mission picker, "See all missions").

import { html } from '../../dom.js';
import { icon } from '../../icons.js';
import { api } from '../../api.js';
import { loadChats, isAway } from '../../store.js';
import { toast } from '../../ui.js';
import { debounce, rowsFor } from '../../util.js';
import { chatGoalControls } from '../goal-controls.js';
import { initiateControls } from '../initiate-controls.js';
import { patchChat } from './shared.js';
import { openMissionSheet, loadMissionTemplates } from '../mission-sheet.js';
import { missionBadge } from '../badges.js';
import { helpTip } from '../help-tip.js';

export function createGoalTab(ctx) {
  const { key } = ctx;
  const st = {
    goal: null, goalSaved: false,
    templates: null, // mission templates (for the badge of the chat's mission)
    editWords: false, // show the goal textarea under an active mission
    initHint: '', initBusy: false, // "Start a conversation" (initiate-controls.js)
  };

  const saveGoal = debounce(async (value) => {
    try {
      await patchChat(key, { goalOverride: value });
      st.goalSaved = true; ctx.update();
      setTimeout(() => { st.goalSaved = false; ctx.update(); }, 1800);
    } catch { /* toasted */ }
  }, 700);

  async function resetGoal(p) {
    try {
      await api.chats.resetGoal(key);
      await loadChats();
      toast(`${p ? p.name : 'The persona'} will pursue the goal again`, { type: 'success' });
    } catch { /* toasted */ }
  }

  // ── Start a conversation (initiate-controls.js) ──
  async function startConversation(c, p) {
    if (st.initBusy) return;
    st.initBusy = true; ctx.update();
    try {
      const r = await api.chats.initiate(key, st.initHint.trim());
      st.initHint = '';
      const name = p ? p.name : 'The persona';
      toast(r && r.approval ? `${name}'s opener will wait in Approvals` : `${name} is writing an opener — watch the activity feed`, { type: 'success' });
      ctx.notify('historyChanged');
    } catch { /* toasted */ } finally { st.initBusy = false; ctx.update(); }
  }

  /** Check-in settings live in the Behaviour tab's fine-tuning form. */
  function openCheckInSettings() {
    ctx.setTab('behaviour');
    ctx.behaviour.openForm('proactive');
  }

  // ── Missions (mission-sheet.js) ──
  function pickMission() {
    if (saveGoal.pending()) saveGoal.flush(st.goal);
    openMissionSheet({ chatKey: key, onStarted: () => { st.goal = null; st.editWords = false; ctx.update(); } });
  }

  function missionRow(c, p) {
    if (!st.templates) {
      loadMissionTemplates().then((t) => { st.templates = t; ctx.update(); }).catch(() => { st.templates = []; });
    }
    const tpl = c.missionId && c.goalOverride && (st.templates || []).find((t) => t.id === c.missionId);
    if (tpl) {
      return html`<div class="goal-mission on">
        ${missionBadge(tpl.badge, tpl.category, { size: 40 })}
        <div class="grow" style="min-width:0"><div class="eyebrow">Mission</div><div class="goal-mission-t">${c.goalOverride}</div></div>
        <button class="btn btn-glass btn-sm" @click=${pickMission}>${icon('refresh')}Change</button>
      </div>`;
    }
    return html`<button type="button" class="goal-mission pick" @click=${pickMission}>
      <span class="goal-mission-stack" aria-hidden="true">${missionBadge('word', 'words', { size: 30 })}${missionBadge('camera', 'share', { size: 30 })}${missionBadge('calendar', 'plans', { size: 30 })}</span>
      <span class="grow"><span class="goal-mission-t">Pick a mission</span><span class="small muted">Ready-made goals, like getting them to say a word or making real plans to meet.</span></span>
      ${icon('chevron-right', 'ic-sm faint')}
    </button>`;
  }

  function view(c, p) {
    const goal = st.goal == null ? (c.goalOverride || '') : st.goal;
    const cb = ctx.behaviour.cb();
    const hasMission = !!(c.missionId && c.goalOverride);
    return html`
      <section class="drawer-section">
        <h4>${icon('target', 'ic-sm')}Goal for this chat ${helpTip('missions')}${st.goalSaved ? html`<span class="saved-tick">${icon('check', 'ic-sm')}Saved</span>` : ''}
          <span class="grow"></span><a class="link-btn small goal-all" href="#/missions">See all missions</a></h4>
        ${missionRow(c, p)}
        ${hasMission && !st.editWords ? html`<button class="link-btn small goal-edit-words" @click=${() => { st.editWords = true; ctx.update(); }}>${icon('edit', 'ic-sm')}Edit the wording</button>` : html`
        <div class="eyebrow goal-own-l">${hasMission ? 'The goal in words' : 'Or write your own'}</div>
        <textarea class="textarea" rows=${rowsFor(goal, 2, 6)} placeholder=${p && p.goal ? `Leave empty to use ${p.name}'s goal: “${p.goal}”` : 'What should the persona steer this conversation towards?'}
          @input=${(e) => { st.goal = e.target.value; saveGoal(e.target.value); }}>${goal}</textarea>`}
        ${chatGoalControls({ g: c.goal, c, p, onPatch: ctx.safePatch, onReset: () => resetGoal(p) })}
      </section>

      <section class="drawer-section" data-key="initiate">
        <h4>${icon('message', 'ic-sm')}Start a conversation</h4>
        ${initiateControls({ c, p, away: isAway(c), cb, hint: st.initHint, busy: st.initBusy,
          onHint: (v) => { st.initHint = v; }, onStart: () => startConversation(c, p),
          onToggleCheckIns: (on) => { const pr = cb && cb.effective && cb.effective.proactive; if (pr) ctx.behaviour.editOverride('proactive', { ...pr, enabled: on }); },
          onOpenSettings: openCheckInSettings })}
      </section>`;
  }

  return {
    id: 'goal',
    view,
    destroy() {
      if (saveGoal.pending()) saveGoal.flush(st.goal);
    },
  };
}
