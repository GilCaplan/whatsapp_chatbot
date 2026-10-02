// pages/missions.js — Missions: playful goals your personas are working on,
// what they pulled off (with the words that proved it) and the badges earned.
// Data: GET /api/missions. "New mission" sheet: components/mission-sheet.js.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { bus, personaById, chatByKey } from '../store.js';
import { confirmSheet, toast, busy } from '../ui.js';
import { debounce, plural } from '../util.js';
import { emptyState } from '../components/art.js';
import { avatarStack } from '../components/avatar.js';
import { missionBadge, medal } from '../components/badges.js';
import { helpTip } from '../components/help-tip.js';
import { openMissionSheet, CATEGORY_LABELS } from '../components/mission-sheet.js';

const categoryLabel = (id) => (CATEGORY_LABELS.find((c) => c.id === id) || { label: 'A' }).label;

const HOW = {
  said_word: { label: 'They said the word', icon: 'quote' },
  ai: { label: 'Spotted in the chat', icon: 'eye' },
  media: { label: 'They sent it', icon: 'image' },
};

const fmtDay = (iso) => {
  try { return new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: new Date(iso).getFullYear() === new Date().getFullYear() ? undefined : 'numeric' }); } catch { return ''; }
};

export default function Missions(ctx) {
  const st = { data: null, error: '', loading: false };

  async function load() {
    st.loading = true;
    try { st.data = await api.missions.list({ quiet: true }); st.error = ''; }
    catch (e) { if (!st.data) st.error = (e && e.message) || 'Could not load missions'; }
    st.loading = false;
    ctx.update();
  }
  const reloadSoon = debounce(load, 200);
  const offs = [];

  const templateOf = (id) => (st.data && st.data.templates || []).find((t) => t.id === id) || null;

  function openRecord(chatKey) {
    return (st.data && st.data.history || []).find((r) => r.chatKey === chatKey && !r.reachedAt) || null;
  }

  async function abandon(a) {
    const ok = await confirmSheet({
      title: 'Stop this mission?',
      body: `${a.personaName || 'The persona'} stops working on “${a.goal.text}” in ${a.chatName}.`,
      confirm: 'Stop mission', danger: true, iconName: 'stop',
    });
    if (!ok) return;
    try { await api.missions.abandon(a.chatKey); toast('Mission stopped', { type: 'success' }); load(); } catch { /* toasted */ }
  }

  async function startNow(a) {
    try {
      const r = await api.chats.initiate(a.chatKey, '');
      toast(r && r.approval ? `${a.personaName}'s opener will wait in Approvals` : `${a.personaName} is writing an opener`, { type: 'success' });
    } catch { /* toasted */ }
  }

  // ── Views ──
  function hero(d) {
    const done = d ? d.history.filter((r) => r.reachedAt).length : 0;
    const badges = d ? d.achievements.filter((a) => a.unlocked).length : 0;
    const tiles = [
      { n: d ? d.active.length : '–', l: 'in progress', icon: 'target', c: 'var(--teal)' },
      { n: d ? done : '–', l: 'completed', icon: 'check', c: 'var(--green)' },
      { n: d ? `${badges}/${d.achievements.length}` : '–', l: 'badges earned', icon: 'star', c: 'var(--amber)' },
    ];
    return html`<section class="mi-hero">
      <div class="mi-hero-text">
        <h2 class="hero-title">Give your personas something to aim for</h2>
        <p class="muted">Pick a playful goal, like getting a friend to say a word or making real plans to meet. The persona works on it quietly, in its own words. ${helpTip('missions')}</p>
        <button class="btn btn-primary mt-12" data-tour="new-mission" @click=${() => openMissionSheet({ onStarted: () => reloadSoon() })}>${icon('plus')}New mission</button>
      </div>
      <div class="card mi-stats">${tiles.map((t) => html`<div class="stat"><span class="stat-icon" style=${`--c:${t.c}`}>${icon(t.icon)}</span>
        <span class="stat-n">${t.n}</span><span class="stat-l">${t.l}</span></div>`)}</div>
    </section>`;
  }

  function activeCard(a) {
    const c = chatByKey(a.chatKey) || { key: a.chatKey, name: a.chatName, jid: a.jid, kind: a.kind };
    const p = personaById(a.personaId);
    const tpl = a.missionId ? templateOf(a.missionId) : null;
    const rec = openRecord(a.chatKey);
    const g = a.goal || {};
    const fromPersona = g.source !== 'chat';
    return html`<article class="card mi-active lift" data-key=${'a-' + a.chatKey}>
      <div class="mi-active-top">
        ${tpl ? missionBadge(tpl.badge, tpl.category, { size: 44 }) : html`<span class="mi-free">${icon('target')}</span>`}
        <div class="grow" style="min-width:0">
          <div class="mi-goal">${g.text}</div>
          <div class="small muted mi-meta">
            ${tpl ? html`<span>${categoryLabel(tpl.category)} mission</span>` : fromPersona ? html`<span>${a.personaName}'s own goal</span>` : html`<span>Your own goal</span>`}
            ${rec ? html` · started <rel-time datetime=${rec.startedAt}></rel-time>` : ''}
          </div>
        </div>
        ${fromPersona ? '' : html`<button class="btn btn-ghost btn-xs btn-icon mi-stop" title="Stop this mission" aria-label="Stop this mission" @click=${() => abandon(a)}>${icon('x', 'ic-sm')}</button>`}
      </div>
      <div class="mi-who">
        ${avatarStack(c, p, 30)}
        <span class="grow ellipsis"><b>${a.personaName || (p && p.name) || 'Persona'}</b> in ${a.chatName}</span>
        ${c.enabled === false ? html`<span class="chip chip-sm chip-amber" title="Replies are off in this chat">Paused</span>` : ''}
        <span class="chip chip-sm chip-violet">${{ subtle: 'Subtle', balanced: 'Balanced', direct: 'Direct' }[g.style] || 'Subtle'}</span>
      </div>
      ${g.lastPlan ? html`<div class="mi-plan"><span class="eyebrow">Next move</span><span>${g.lastPlan}</span></div>` : html`<div class="mi-plan dim"><span class="eyebrow">Next move</span><span>Waiting for the conversation to get going.</span></div>`}
      <div class="mi-actions">
        <button class="btn btn-glass btn-sm" ?disabled=${c.enabled === false} @click=${busy(() => startNow(a))} title=${c.enabled === false ? 'Turn replies on in this chat first' : 'Send an opener now'}>${icon('message')}Start now</button>
        <button class="btn btn-ghost btn-sm" @click=${() => openMissionSheet({ chatKey: a.chatKey, onStarted: () => reloadSoon() })}>${icon('refresh')}Change</button>
        <a class="btn btn-ghost btn-sm" href=${'#/chats/' + encodeURIComponent(a.chatKey) + '/goal'}>${icon('chats')}Open chat</a>
      </div>
    </article>`;
  }

  function completedRow(r) {
    const tpl = r.templateId ? templateOf(r.templateId) : null;
    const how = HOW[r.how] || HOW.ai;
    const [who, ...rest] = String(r.evidence || '').split(': ');
    const MEDIA = { '[photo]': 'Sent a photo', '[voice note]': 'Sent a voice note', '[video]': 'Sent a video', '[sticker]': 'Sent a sticker' };
    const said = rest.length ? rest.join(': ') : who;
    const quote = MEDIA[said.trim().toLowerCase()] || said;
    const speaker = rest.length ? who : '';
    return html`<li class="mi-done" data-key=${'h-' + r.id}>
      <span class="mi-done-dot" aria-hidden="true"></span>
      <div class="mi-done-card">
        <div class="mi-done-top">
          ${tpl ? missionBadge(tpl.badge, tpl.category, { size: 34 }) : html`<span class="mi-free sm">${icon('check')}</span>`}
          <div class="grow" style="min-width:0">
            <div class="mi-done-goal">${r.goal}</div>
            <div class="small muted">${r.personaName || 'Persona'} in ${r.chatName || r.chatKey} · ${fmtDay(r.reachedAt)}</div>
          </div>
          <span class="chip chip-sm chip-green">${icon(how.icon)}${how.label}</span>
        </div>
        ${quote ? html`<blockquote class="mi-quote">${icon(quote !== said ? 'image' : 'quote', 'ic-sm')}<span>${quote}</span>${speaker ? html`<cite>${speaker}</cite>` : ''}</blockquote>` : ''}
      </div>
    </li>`;
  }

  function achievementTile(a) {
    return html`<div class=${'mi-badge ' + (a.unlocked ? 'on' : '')} data-key=${'ach-' + a.id} title=${a.blurb}>
      ${medal(a, { size: 84 })}
      <div class="mi-badge-t">${a.title}</div>
      <div class="mi-badge-s">${a.unlocked ? `Earned ${fmtDay(a.unlockedAt)}` : a.target > 1 ? `${a.progress} of ${a.target}` : 'Not yet'}</div>
      <div class="mi-badge-b">${a.blurb}</div>
    </div>`;
  }

  function view() {
    const d = st.data;
    if (!d && st.error) {
      return html`<div class="card">${emptyState({ artName: 'missions', title: "Couldn't load missions", body: st.error, action: { label: 'Try again', icon: 'refresh', onClick: load } })}</div>`;
    }
    const done = d ? d.history.filter((r) => r.reachedAt) : [];
    return html`<div class="missions">
      ${hero(d)}
      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('target', 'ic-sm')}In progress</h3>${d && d.active.length ? html`<span class="faint small">${plural(d.active.length, 'chat')}</span>` : ''}</div>
        ${!d ? html`<div class="mi-active-grid">${[1, 2].map(() => html`<div class="card skeleton" style="height:190px"></div>`)}</div>`
          : d.active.length ? html`<div class="mi-active-grid">${d.active.map(activeCard)}</div>`
          : html`<div class="card">${emptyState({ artName: 'missions', small: true, title: 'No missions running', body: 'Start one and your persona begins steering the chat towards it, without giving the game away.',
              action: { label: 'New mission', icon: 'plus', onClick: () => openMissionSheet({ onStarted: () => reloadSoon() }) } })}</div>`}
      </section>

      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('star', 'ic-sm')}Badges</h3>${d ? html`<span class="faint small">${d.achievements.filter((a) => a.unlocked).length} of ${d.achievements.length}</span>` : ''}</div>
        <div class="card mi-badges">${d ? d.achievements.map(achievementTile) : html`<div class="skeleton" style="height:140px;border-radius:16px"></div>`}</div>
      </section>

      <section class="mi-section">
        <div class="mi-sec-head"><h3>${icon('check-double', 'ic-sm')}Completed</h3>${done.length ? html`<span class="faint small">${plural(done.length, 'mission')}</span>` : ''}</div>
        ${!d ? '' : done.length ? html`<ol class="mi-timeline">${done.map(completedRow)}</ol>`
          : html`<div class="card mi-empty-done"><span class="small muted">Completed missions show up here, with the words that proved it.</span></div>`}
      </section>
    </div>`;
  }

  return {
    title: 'Missions',
    subtitle: () => (st.data ? `${plural(st.data.active.length, 'mission')} in progress` : ''),
    view,
    mount() {
      load();
      offs.push(bus.on('missions.changed', reloadSoon), bus.on('chats.changed', reloadSoon));
    },
    unmount() { for (const off of offs) off(); reloadSoon.cancel(); },
  };
}
