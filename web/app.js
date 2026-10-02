// app.js — boot, shell (sidebar + topbar), routing table, system overlays.

import { html, render, mountView } from './dom.js';
import { installIcons, icon } from './icons.js';
import { defineElements } from './components/elements.js';
import { setErrorToaster, setToken, hasToken } from './api.js';
import { store, bus, loadHealth, refreshAll } from './store.js';
import { startSSE } from './sse.js';
import { initRouter, handle, navigate, currentPage, parseHash } from './router.js';
import { toast, confirmSheet } from './ui.js';
import { statusPill } from './components/status.js';
import { openWASheet } from './components/wa-sheet.js';
import { sleep } from './util.js';
import { movingTo, showStopped } from './system.js';
import { PAGE_SECTION } from './guide-content.js';
import { startTour, tourDone } from './components/tour.js';
import { applyAppearance, watchSystemMode } from './skins.js';

import Onboarding from './pages/onboarding.js';
import Dashboard from './pages/dashboard.js';
import Chats from './pages/chats.js';
import Personas from './pages/personas.js';
import PersonaEditor from './pages/persona-editor.js';
import Playground from './pages/playground.js';
import Activity from './pages/activity.js';
import Approvals from './pages/approvals.js';
import Settings from './pages/settings.js';
import Missions from './pages/missions.js';
import Guide from './pages/guide.js';

const ROUTES = {
  welcome: Onboarding,
  dashboard: Dashboard,
  chats: Chats,
  personas: Personas,
  persona: PersonaEditor,
  playground: Playground,
  activity: Activity,
  approvals: Approvals,
  settings: Settings,
  missions: Missions,
  guide: Guide,
};

const NAV = [
  { name: 'dashboard', label: 'Dashboard', icon: 'dashboard' },
  { name: 'chats', label: 'Chats', icon: 'chats' },
  { name: 'personas', label: 'Personas', icon: 'personas', also: ['persona'] },
  { name: 'missions', label: 'Missions', icon: 'target' },
  { name: 'playground', label: 'Playground', icon: 'playground' },
  { name: 'activity', label: 'Activity', icon: 'activity' },
  { name: 'approvals', label: 'Approvals', icon: 'approvals', badge: () => store.state.approvals.length },
  { name: 'settings', label: 'Settings', icon: 'settings' },
  { name: 'guide', label: 'Guide', icon: 'info' },
];

const appEl = document.getElementById('app');
const pageHost = document.getElementById('page');

// ── Shell views ────────────────────────────────────────────────────────
function sidebarView() {
  const route = parseHash();
  const s = store.state;
  const health = s.health || {};
  return html`
    <a class="brand" href="#/dashboard" aria-label="Doppel home">
      <img src="/assets/favicon.svg" alt="">
      <span class="brand-name"><strong>Doppel</strong><span>for WhatsApp</span></span>
    </a>
    <nav class="nav">
      ${NAV.map((n) => {
        const active = route.name === n.name || (n.also && n.also.includes(route.name));
        const badge = n.badge ? n.badge() : 0;
        return html`<a class="nav-item" href=${'#/' + n.name} aria-current=${active ? 'page' : 'false'} title=${n.label} data-key=${n.name}>
          ${icon(n.icon)}<span class="label">${n.label}</span>
          ${badge ? html`<span class="nav-badge" aria-label=${badge + ' pending'}>${badge > 99 ? '99+' : badge}</span>` : ''}
        </a>`;
      })}
    </nav>
    <div class="sidebar-foot">
      <div class="conn" title=${s.sse === 'open' ? 'Live updates connected' : 'Reconnecting to Doppel…'}>
        <span class="dot dot-sm" style=${`--c:${s.sse === 'open' ? 'var(--green)' : 'var(--orange)'}`}></span>
        <span class="conn-text">${s.sse === 'open' ? 'Live' : 'Reconnecting…'}</span>
      </div>
      ${health.version ? html`<span class="ver">v${health.version}${health.fakeWA || health.fakeLLM ? ' · demo mode' : ''}</span>` : ''}
    </div>`;
}

function topbarView() {
  const cur = currentPage();
  const inst = cur && cur.inst;
  const s = store.state;
  const title = (inst && (typeof inst.title === 'function' ? inst.title() : inst.title)) || '';
  const sub = inst && inst.subtitle ? inst.subtitle() : '';
  const down = s.sse === 'down' && s.sseDownSince && Date.now() - s.sseDownSince > 3000;
  return html`
    <div class="grow" style="min-width:0">
      <h1>${title}</h1>
      ${sub ? html`<div class="sub">${sub}</div>` : ''}
    </div>
    ${down ? html`<span class="offline-banner">${icon('wifi', 'ic-sm')}Reconnecting…</span>` : ''}
    ${inst && inst.actions ? inst.actions() : ''}
    ${helpButton()}
    ${statusPill(s.wa, () => openWASheet())}`;
}

// ── Help: topbar "?" (opens the Guide at the section for this page) + tour ──
function helpButton() {
  const r = parseHash();
  if (r.name === 'guide' || r.name === 'welcome') return '';
  const sec = PAGE_SECTION[r.name] || 'welcome';
  return html`<a class="btn btn-glass btn-icon help-btn" href=${'#/guide/' + sec} data-tour="help"
    title="Help for this page" aria-label="Open the guide for this page">?</a>`;
}

// The tour starts by itself once, right after onboarding finishes in this
// browser (localStorage remembers it; the Guide can replay it).
let wasOnboarded = null;
function maybeStartTour(route) {
  const done = !!(store.state.settings && store.state.settings.onboardingCompleted);
  if (wasOnboarded === false && done && route && route.name !== 'welcome' && !tourDone()) {
    setTimeout(() => startTour(), 900);
  }
  if (route && route.name !== 'welcome') wasOnboarded = done;
}

let sidebar = null;
let topbar = null;

function updateShell() {
  if (sidebar) sidebar.update();
  if (topbar) topbar.update();
  const cur = currentPage();
  const title = cur && cur.inst && (typeof cur.inst.title === 'function' ? cur.inst.title() : cur.inst.title);
  const n = store.state.approvals.length;
  document.title = `${n ? `(${n}) ` : ''}${title ? title + ' · ' : ''}Doppel`;
}

// ── System overlays (port change, quit) ────────────────────────────────
bus.on('system', (d) => {
  if (!d) return;
  if (d.kind === 'port_changed') movingTo(d.url);
  else if (d.kind === 'quitting') showStopped();
});

// ── Boot ───────────────────────────────────────────────────────────────
function splash(msg, retry) {
  render(pageHost, html`<div class="splash">
    <img src="/assets/favicon.svg" alt="" width="72" height="72">
    <p>${msg}</p>
    ${retry ? html`<button class="btn btn-glass" @click=${retry}>${icon('refresh')}Try again</button>` : ''}
  </div>`);
}

async function boot() {
  installIcons();
  defineElements();
  setErrorToaster(toast);

  let health = null;
  for (let attempt = 0; attempt < 6 && !health; attempt++) {
    try { health = await loadHealth(); } catch { await sleep(600 * (attempt + 1)); }
  }
  if (!health) {
    splash("Can't reach Doppel. Make sure the app is running.", () => location.reload());
    return;
  }
  if (!hasToken() && health.token) setToken(health.token);

  await refreshAll();
  applyAppearance(store.state.settings || {}, { animate: false });
  watchSystemMode(() => store.state.settings || {}, () => { const cur = currentPage(); if (cur) cur.view.update(); });
  store.set({ booted: true });

  sidebar = mountView(document.getElementById('sidebar'), sidebarView);
  topbar = mountView(document.getElementById('topbar'), topbarView);

  store.subscribe((s) => {
    if (s.settings) applyAppearance(s.settings);
    updateShell();
    const cur = currentPage();
    if (cur) cur.view.update();
  });

  // Keep the "Reconnecting…" banner honest even without store changes.
  setInterval(() => { if (store.state.sse === 'down') updateShell(); }, 2000);

  initRouter({
    table: ROUTES,
    defaultRoute: 'dashboard',
    el: pageHost,
    changed: (route, cur) => {
      appEl.dataset.layout = cur && cur.inst && cur.inst.bare ? 'bare' : '';
      updateShell();
      maybeStartTour(route);
    },
    updated: () => { if (topbar) topbar.update(); },
    beforeLeave: () => confirmSheet({
      title: 'Discard unsaved changes?',
      body: 'You have edits that haven\'t been saved yet.',
      confirm: 'Discard',
      cancel: 'Keep editing',
      danger: true,
    }),
  });

  const onboarded = store.state.settings ? store.state.settings.onboardingCompleted : health.onboardingCompleted;
  wasOnboarded = !!onboarded;
  const r = parseHash();
  if (!onboarded && r.name !== 'welcome') navigate('/welcome', { replace: true });
  else if (!r.name) navigate('/dashboard', { replace: true });
  else handle();

  startSSE();
}

boot().catch((e) => {
  console.error(e);
  splash('Something went wrong while starting. ' + (e && e.message ? e.message : ''), () => location.reload());
});

