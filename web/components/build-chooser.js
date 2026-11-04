// build-chooser.js — "Build a persona": one button on the Personas page opens
// this chooser — like you (Clone yourself), a personal secretary, from a
// description (AI builder) or from scratch (the editor).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { openSheet } from '../ui.js';
import { navigate } from '../router.js';
import { openCloneSheet } from './clone-sheet.js';
import { openAIBuilder } from './ai-builder.js';
import { openSecretaryBuilder } from './secretary-builder.js';

const KINDS = [
  { id: 'me', icon: 'user', title: 'Like me', line: 'A persona that texts like you, learned from messages you type yourself.', c1: 'var(--teal)', c2: 'var(--blue)' },
  { id: 'secretary', icon: 'inbox', title: 'Personal secretary', line: 'Answers for you while you are busy, takes messages, flags what is urgent and helps people find a time with you.', c1: 'var(--indigo)', c2: 'var(--blue)' },
  { id: 'ai', icon: 'sparkles', title: 'From a description', line: 'Describe someone in one line and the AI writes the whole persona.', c1: 'var(--green)', c2: 'var(--teal)' },
  { id: 'scratch', icon: 'edit', title: 'From scratch', line: 'Fill in every field yourself.', c1: 'var(--indigo)', c2: 'var(--pink)' },
];

/** Open the chooser; picking a kind closes it and opens that builder. */
export function openBuildChooser() {
  const go = (ctl, id) => {
    ctl.close();
    if (id === 'me') openCloneSheet();
    else if (id === 'secretary') openSecretaryBuilder();
    else if (id === 'ai') openAIBuilder();
    else navigate('/persona/new');
  };
  return openSheet((ctl) => html`
    <div class="sheet-head">
      <div class="sheet-icon">${icon('plus')}</div>
      <div class="grow"><h2>Build a persona</h2><p>Pick where to start. You can change everything afterwards.</p></div>
      <button class="btn btn-ghost btn-icon sheet-close" aria-label="Close" @click=${() => ctl.close()}>${icon('x')}</button>
    </div>
    <div class="build-grid">
      ${KINDS.map((k) => html`<button type="button" class="build-tile" style=${`--c1:${k.c1};--c2:${k.c2}`} @click=${() => go(ctl, k.id)}>
        <span class="build-ic">${icon(k.icon)}</span>
        <span class="build-title">${k.title}</span>
        <span class="build-line">${k.line}</span>
      </button>`)}
    </div>`, { label: 'Build a persona', size: 'lg' });
}
