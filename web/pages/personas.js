// pages/personas.js — persona card grid with New / Build with AI / Duplicate / Delete / Reset.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, usageCount, loadPersonas, loadChats } from '../store.js';
import { openMenu, confirmSheet, toast } from '../ui.js';
import { personaAvatar } from '../components/avatar.js';
import { emptyState } from '../components/art.js';
import { openAIBuilder } from '../components/ai-builder.js';
import { openCloneSheet } from '../components/clone-sheet.js';
import { providerLabel } from '../components/status.js';

export async function duplicatePersona(p, navigate) {
  const copy = await api.personas.duplicate(p.id);
  await loadPersonas();
  toast(html`Made a copy: <b>${copy.name}</b>`, { type: 'success' });
  if (navigate) navigate(`/persona/${encodeURIComponent(copy.id)}`);
  return copy;
}

export async function deletePersona(p) {
  const n = usageCount(p.id);
  const ok = await confirmSheet({
    title: `Delete ${p.name}?`,
    body: n ? `${p.name} answers ${n} chat${n === 1 ? '' : 's'}. Those chats will be switched off until you pick another persona.` : 'This can\'t be undone.',
    confirm: 'Delete',
    danger: true,
    iconName: 'trash',
  });
  if (!ok) return false;
  await api.personas.remove(p.id);
  await Promise.all([loadPersonas(), loadChats()]);
  toast(`${p.name} deleted`, { type: 'success' });
  return true;
}

export async function resetPersona(p) {
  const ok = await confirmSheet({
    title: `Reset ${p.name} to the original?`,
    body: 'All your edits to this built-in persona are replaced with the default text. Chat assignments are kept.',
    confirm: 'Reset',
    danger: true,
    iconName: 'reset',
  });
  if (!ok) return null;
  const r = await api.personas.reset(p.id);
  await loadPersonas();
  toast(`${p.name} restored`, { type: 'success' });
  return r;
}

export default function Personas(ctx) {
  const st = { q: '' };

  function card(p) {
    const n = usageCount(p.id);
    const llm = p.llm && p.llm.provider ? p.llm : null;
    const open = () => ctx.navigate(`/persona/${encodeURIComponent(p.id)}`);
    const menu = (e) => {
      e.stopPropagation();
      openMenu(e.currentTarget, [
        { label: 'Edit', icon: 'edit', onClick: open },
        { label: 'Try in Playground', icon: 'playground', onClick: () => ctx.navigate(`/playground?persona=${encodeURIComponent(p.id)}`) },
        { label: 'Duplicate', icon: 'copy', onClick: () => duplicatePersona(p, ctx.navigate).catch(() => {}) },
        p.builtIn ? { label: 'Reset to original', icon: 'reset', onClick: () => resetPersona(p).catch(() => {}) } : null,
        'sep',
        { label: 'Delete', icon: 'trash', danger: true, onClick: () => deletePersona(p).catch(() => {}) },
      ], { align: 'end' });
    };
    return html`<article class="card lift clickable persona-card" data-key=${p.id} tabindex="0"
        @click=${open} @keydown=${(e) => { if (e.key === 'Enter') open(); }}>
      <div class="pc-glow" style=${`background:${glow(p)}`}></div>
      <div class="row row-top">
        ${personaAvatar(p, 64)}
        <span class="grow"></span>
        <button class="btn btn-ghost btn-icon btn-sm" aria-label=${'More actions for ' + p.name} @click=${menu}>${icon('more')}</button>
      </div>
      <div class="pc-name">${p.name}${p.builtIn ? html` <span class="chip chip-sm" title="Comes with Doppel">Built-in</span>` : ''}</div>
      <p class="pc-tag clamp-2">${p.tagline || p.bio || 'No tagline yet'}</p>
      <div class="row gap-6 row-wrap pc-chips">
        <span class=${'chip chip-sm ' + (n ? 'chip-green' : '')}>${icon('chats')}${n ? `Used in ${n} chat${n === 1 ? '' : 's'}` : 'Not in any chat'}</span>
        <span class="chip chip-sm" title="Which AI writes this persona's replies">${icon('cpu')}${llm ? `${providerLabel(llm.provider)}${llm.model ? ' · ' + llm.model : ''}` : 'Default brain'}</span>
      </div>
    </article>`;
  }

  function glow(p) {
    const g = (p.avatar && p.avatar.gradient) || [];
    const c = /^#[0-9a-f]{3,8}$/i.test(g[0] || '') ? g[0] : '#14b8c4';
    return `radial-gradient(circle at 20% 0%, ${c}55, transparent 60%)`;
  }

  function view() {
    const s = store.state;
    const q = st.q.trim().toLowerCase();
    const list = s.personas.filter((p) => !q || (p.name + ' ' + (p.tagline || '')).toLowerCase().includes(q));
    return html`<div class="personas-page">
      <div class="toolbar">
        <div class="input-wrap" style="width:min(320px,100%)">
          ${icon('search')}
          <input class="input search-input" type="search" placeholder="Search personas" .value=${st.q}
            @input=${(e) => { st.q = e.target.value; ctx.update(); }}>
        </div>
        <span class="grow"></span>
        <button class="btn btn-glass" @click=${() => ctx.navigate('/persona/new')}>${icon('plus')}New persona</button>
        <button class="btn btn-glass" data-tour="clone" @click=${() => openCloneSheet()} title="A persona that texts like you">${icon('user')}Clone yourself</button>
        <button class="btn btn-brand" @click=${() => openAIBuilder()}>${icon('sparkles')}Build with AI</button>
      </div>
      ${!s.personasLoaded ? html`<div class="persona-grid">${[0, 1, 2, 3].map(() => html`<div class="card skeleton" style="height:220px"></div>`)}</div>`
        : !s.personas.length ? html`<div class="card">${emptyState({
          artName: 'personas',
          title: 'No personas yet',
          body: 'A persona is a character with a name, a vibe and a texting style. Describe one in a sentence and let AI fill in the rest.',
          action: { label: 'Build with AI', icon: 'sparkles', onClick: () => openAIBuilder(), cls: 'btn-brand' },
          secondary: { label: 'Start from scratch', icon: 'plus', onClick: () => ctx.navigate('/persona/new') },
        })}</div>`
        : !list.length ? html`<div class="card">${emptyState({ artName: 'search', small: true, title: 'No matches', body: `No persona matches “${st.q}”.` })}</div>`
        : html`<div class="persona-grid stagger">
            ${list.map(card)}
            <button class="card new-card" @click=${() => openAIBuilder()}>
              <span class="pav" style="--s:56px;background:var(--brand-grad)">${icon('sparkles', 'ic-lg')}</span>
              <span class="h3">Build with AI</span>
              <span class="small muted">Describe someone in one line</span>
            </button>
            <button class="card new-card clone-card" @click=${() => openCloneSheet()}>
              <span class="pav" style="--s:56px;background:linear-gradient(135deg,#34d399,#0ea5e9)">${icon('user', 'ic-lg')}</span>
              <span class="h3">Clone yourself</span>
              <span class="small muted">A persona that texts like you</span>
            </button>
          </div>`}
    </div>`;
  }

  return { title: 'Personas', view };
}
