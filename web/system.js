// system.js — full-screen system overlays: port move and "stopped".

import { html, render } from './dom.js';
import { icon } from './icons.js';
import { stopSSE } from './sse.js';
import { closeAllSheets } from './ui.js';

let moving = false;

/** Show "Moving to port N…" then reload the app on the new origin. */
export function movingTo(url) {
  if (moving || !url) return;
  moving = true;
  let port = '';
  try { port = new URL(url, location.href).port; } catch { /* ignore */ }
  closeAllSheets();
  render(document.getElementById('system-overlay'), html`<div class="sys-overlay">
    <div class="sys-card glass">
      <img src="/assets/favicon.svg" alt="">
      <h2>Moving to port ${port || 'new'}…</h2>
      <p>Doppel is switching addresses. This page will reload by itself.</p>
      <span class="spinner spinner-lg"></span>
    </div></div>`);
  stopSSE();
  setTimeout(() => {
    try {
      const u = new URL(url, location.href);
      u.hash = location.hash;
      location.replace(u.toString());
    } catch { location.replace(url); }
  }, 1100);
}

export function showStopped() {
  stopSSE();
  closeAllSheets();
  render(document.getElementById('system-overlay'), html`<div class="sys-overlay">
    <div class="sys-card glass stopped">
      <img src="/assets/favicon.svg" alt="">
      <h2>Doppel has stopped.</h2>
      <p>Open the app again to restart. Your personas, chats and settings are saved.</p>
      <button class="btn btn-glass mt-8" @click=${() => location.reload()}>${icon('refresh')}Check again</button>
    </div></div>`);
}
