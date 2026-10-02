// skins.js — the looks ("skins") the app offers and how one is applied.
// Keep in sync with config.Skins (Go) and the one-mode map in boot.js ONLY;
// colours live in styles/tokens.css (glass) and styles/skins.css.

import { prefersReducedMotion } from './util.js';

export const SKINS = [
  { id: 'glass', name: 'Liquid Glass', blurb: 'Frosted cards over soft colour.', modes: ['light', 'dark'] },
  { id: 'midnight', name: 'Midnight', blurb: 'Pure black, quiet and sharp.', modes: ['dark'] },
  { id: 'daylight', name: 'Daylight', blurb: 'Pure white, crisp and minimal.', modes: ['light'] },
  { id: 'classic', name: 'Classic', blurb: 'Clean and flat, one blue accent.', modes: ['light', 'dark'] },
  { id: 'vintage', name: 'Vintage', blurb: 'Paper, ink and antique gold.', modes: ['light', 'dark'], darkName: 'Candlelit study' },
  { id: 'ocean', name: 'Ocean', blurb: 'Deep teal and sea glass.', modes: ['light', 'dark'] },
  { id: 'forest', name: 'Forest', blurb: 'Sage, moss and a touch of gold.', modes: ['light', 'dark'] },
  { id: 'neon', name: 'Neon', blurb: 'Night drive: cyan, magenta, glow.', modes: ['dark'] },
  { id: 'contrast', name: 'High contrast', blurb: 'Solid colours and strong borders.', modes: ['light', 'dark'] },
];

export const skinById = (id) => SKINS.find((s) => s.id === id) || SKINS[0];

/** The mode a look actually shows: one-mode looks force theirs; the rest follow theme (system → the OS). */
export function resolveMode(skinId, theme, systemDark) {
  const s = skinById(skinId);
  if (s.modes.length === 1) return s.modes[0];
  if (theme === 'light' || theme === 'dark') return theme;
  return systemDark ? 'dark' : 'light';
}

const darkQuery = () => (window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null);
export const systemDark = () => { const q = darkQuery(); return !!(q && q.matches); };

let last = '';

/**
 * Apply settings {skin, theme} to <html data-skin data-theme> (theme is the resolved
 * mode), remember it for boot.js, and cross-fade when it changes (unless reduced motion).
 */
export function applyAppearance(settings, { animate = true } = {}) {
  const skin = skinById(settings && settings.skin).id;
  const raw = settings && (settings.theme === 'light' || settings.theme === 'dark') ? settings.theme : 'system';
  const mode = resolveMode(skin, raw, systemDark());
  try {
    localStorage.setItem('doppel.skin', skin);
    localStorage.setItem('doppel.theme', raw);
  } catch { /* storage unavailable */ }
  const key = skin + '/' + mode;
  if (key === last) return;
  const first = !last;
  last = key;
  const root = document.documentElement;
  const swap = () => {
    root.setAttribute('data-skin', skin);
    root.setAttribute('data-theme', mode);
    const bg = getComputedStyle(root).getPropertyValue('--bg').trim();
    if (bg) document.querySelectorAll('meta[name="theme-color"]').forEach((m) => m.setAttribute('content', bg));
  };
  const same = root.getAttribute('data-skin') === skin && root.getAttribute('data-theme') === mode;
  if (first || same || !animate || prefersReducedMotion() || typeof document.startViewTransition !== 'function') {
    swap();
    return;
  }
  root.classList.add('skin-switch');
  const vt = document.startViewTransition(swap);
  vt.finished.finally(() => root.classList.remove('skin-switch'));
}

/** Re-apply when the OS switches light/dark (only matters for theme "system"). */
export function watchSystemMode(getSettings, onChange) {
  const q = darkQuery();
  if (!q) return;
  const on = () => { applyAppearance(getSettings()); if (onChange) onChange(); };
  if (q.addEventListener) q.addEventListener('change', on);
  else if (q.addListener) q.addListener(on);
}
