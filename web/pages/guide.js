// pages/guide.js — the Guide: how Doppel works, explained with pictures.
// Copy: guide-content.js. Pictures: components/guide-art.js. Deep links:
// #/guide/<section> (the topbar "?" opens the section for the current page).

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { debounce, prefersReducedMotion } from '../util.js';
import { GUIDE_SECTIONS } from '../guide-content.js';
import { guideArt } from '../components/guide-art.js';
import { art } from '../components/art.js';
import { richText } from '../components/help-tip.js';
import { startTour } from '../components/tour.js';

function block(b, i) {
  if (b.p) return html`<p class="gd-p" data-key=${'b' + i}>${richText(b.p)}</p>`;
  if (b.note) return html`<div class="gd-note" data-key=${'b' + i}>${icon('info', 'ic-sm')}<span>${richText(b.note)}</span></div>`;
  if (b.points) {
    return html`<ul class="gd-points" data-key=${'b' + i}>${b.points.map((t) => html`<li><span class="gd-bullet" aria-hidden="true"></span><span>${richText(t)}</span></li>`)}</ul>`;
  }
  if (b.steps) {
    return html`<ol class="gd-steps" data-key=${'b' + i}>${b.steps.map((t, n) => html`<li><span class="gd-step-n" aria-hidden="true">${n + 1}</span><span>${richText(t)}</span></li>`)}</ol>`;
  }
  if (b.figure) {
    return html`<div class="gd-figure" data-key=${'b' + i}>${guideArt(b.figure, 'sm')}<div><h4>${b.title}</h4><p>${richText(b.text)}</p></div></div>`;
  }
  if (b.qa) {
    return html`<div class="gd-qa" data-key=${'b' + i}>${b.qa.map(([q, a]) => html`<details class="gd-q">
      <summary><span>${q}</span>${icon('chevron-down', 'ic-sm gd-q-chev')}</summary>
      <p>${richText(a)}</p>
    </details>`)}</div>`;
  }
  return '';
}

export default function Guide(ctx) {
  const st = { active: GUIDE_SECTIONS[0].id };
  let scrollHost = null;

  function go(id, smooth = true) {
    const el = document.getElementById('gd-' + id);
    if (!el) return;
    st.active = id;
    ctx.update();
    el.scrollIntoView({ behavior: smooth && !prefersReducedMotion() ? 'smooth' : 'auto', block: 'start' });
    el.classList.remove('gd-flash');
    void el.offsetWidth; // restart the highlight animation
    el.classList.add('gd-flash');
  }

  function view() {
    return html`<div class="guide">
      <nav class="guide-toc glass" aria-label="Guide sections">
        <div class="eyebrow gd-toc-title">In this guide</div>
        ${GUIDE_SECTIONS.map((s) => html`<a class=${'gd-toc-item ' + (st.active === s.id ? 'on' : '')} href=${'#/guide/' + s.id}
            aria-current=${st.active === s.id ? 'true' : 'false'} data-key=${'toc-' + s.id}>
          ${icon(s.icon, 'ic-sm')}<span class="grow"><span class="gd-toc-t">${s.title}</span><span class="gd-toc-s">${s.short}</span></span>
        </a>`)}
      </nav>
      <div class="guide-body">
        <section class="card gd-hero">
          <span class="guide-art hero">${art('guide')}</span>
          <div class="grow">
            <div class="eyebrow">Guide</div>
            <h2 class="hero-title">Doppel in five minutes</h2>
            <p class="muted">Everything explained in plain words, with pictures. Jump to any part, or take the quick tour of the app.</p>
            <div class="row gap-8 row-wrap mt-12">
              <button class="btn btn-primary" @click=${() => startTour({ force: true })}>${icon('play')}Take the tour</button>
              <button class="btn btn-glass" @click=${() => go('how-it-works')}>${icon('link')}How it works</button>
            </div>
          </div>
        </section>
        ${GUIDE_SECTIONS.map((s, n) => html`<section class="card gd-section" id=${'gd-' + s.id} data-key=${'sec-' + s.id}>
          <div class="gd-head">
            ${guideArt(s.art)}
            <div class="grow">
              <div class="gd-num">${String(n + 1).padStart(2, '0')}</div>
              <h3>${s.title}</h3>
              <p class="gd-lead">${richText(s.lead)}</p>
            </div>
          </div>
          <div class="gd-blocks">${s.blocks.map(block)}</div>
          ${n < GUIDE_SECTIONS.length - 1 ? html`<button class="link-btn gd-next" @click=${() => go(GUIDE_SECTIONS[n + 1].id)}>
            Next: ${GUIDE_SECTIONS[n + 1].title}${icon('arrow-right', 'ic-sm')}</button>` : ''}
        </section>`)}
      </div>
    </div>`;
  }

  const onScroll = debounce(() => {
    if (!scrollHost) return;
    const top = scrollHost.getBoundingClientRect().top;
    let best = GUIDE_SECTIONS[0].id;
    for (const s of GUIDE_SECTIONS) {
      const el = document.getElementById('gd-' + s.id);
      if (el && el.getBoundingClientRect().top - top < 160) best = s.id;
    }
    if (best !== st.active) { st.active = best; ctx.update(); }
  }, 60);

  function openParam(route, smooth) {
    const id = route && route.param;
    if (id && GUIDE_SECTIONS.some((s) => s.id === id)) requestAnimationFrame(() => go(id, smooth));
  }

  return {
    title: 'Guide',
    subtitle: () => 'How Doppel works, in plain words',
    view,
    onParams(route) { openParam(route, true); },
    mount() {
      scrollHost = document.getElementById('page');
      if (scrollHost) scrollHost.addEventListener('scroll', onScroll, { passive: true });
      setTimeout(() => openParam(ctx.route, false), 60);
    },
    unmount() {
      if (scrollHost) scrollHost.removeEventListener('scroll', onScroll);
      onScroll.cancel();
    },
  };
}
