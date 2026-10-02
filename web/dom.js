// dom.js — a tiny rendering core.
//
//   html`<p class=${cls} @click=${fn} ?disabled=${off} .value=${v}>${text}</p>`
//
// • Every interpolated value is HTML-escaped unless wrapped in raw().
// • `@event=${fn}`  binds an event listener (any event name, incl. custom ones).
// • `?attr=${bool}` toggles a boolean attribute.
// • `.prop=${v}`    sets a DOM property after render (skipped for a focused field's value).
// • Nested templates and arrays of templates render in place.
//
// render(container, template) builds the new markup and *morphs* the live DOM
// into it (keyed by data-key), so focus, caret, scroll and CSS transitions survive
// re-renders. Children of custom elements (tags with a dash) and of elements
// marked data-keep are left alone so they can manage themselves.

const TR = Symbol.for('doppel.template');
const RAW = Symbol.for('doppel.raw');

export function html(strings, ...values) {
  return { [TR]: true, strings, values };
}

/** Mark a string as trusted HTML (never use with user/WhatsApp data). */
export function raw(s) {
  return { [RAW]: true, s: s == null ? '' : String(s) };
}

export const isTemplate = (v) => !!(v && v[TR]);

const ESC = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;', '`': '&#96;' };
export function esc(s) {
  return String(s).replace(/[&<>"'`]/g, (c) => ESC[c]);
}

// ── Template analysis (cached per call site) ────────────────────────────
const cache = new WeakMap();
const ATTR_UNQUOTED = /([@?.]?[A-Za-z_][\w:.-]*)=$/;
const ATTR_QUOTED = /([@?.]?[A-Za-z_][\w:.-]*)=(["'])$/;

function analyze(strings) {
  let a = cache.get(strings);
  if (a) return a;
  const statics = strings.slice();
  const parts = [];
  let inTag = false;
  let quote = null;
  for (let i = 0; i < strings.length; i++) {
    const s = strings[i];
    for (let j = 0; j < s.length; j++) {
      const ch = s[j];
      if (!inTag) {
        if (ch === '<' && /[A-Za-z/!]/.test(s[j + 1] || '')) { inTag = true; quote = null; }
      } else if (quote) {
        if (ch === quote) quote = null;
      } else if (ch === '"' || ch === "'") {
        quote = ch;
      } else if (ch === '>') {
        inTag = false;
      }
    }
    if (i === strings.length - 1) break;

    let part;
    if (!inTag) {
      part = { k: 'text' };
    } else {
      const m = quote ? ATTR_QUOTED.exec(statics[i]) : ATTR_UNQUOTED.exec(statics[i]);
      const wholeValue = m && (!quote || strings[i + 1][0] === quote);
      if (wholeValue) {
        const name = m[1];
        const pre = name[0];
        if (pre === '@' || pre === '?' || pre === '.') {
          statics[i] = statics[i].slice(0, statics[i].length - m[0].length);
          if (quote) statics[i + 1] = statics[i + 1].slice(1);
          part = { k: pre, name: name.slice(1) };
        } else {
          part = { k: 'attr', name: name.toLowerCase(), quoted: !!quote };
        }
      } else if (quote) {
        part = { k: 'apart' };
      } else {
        part = { k: 'tag' };
      }
    }
    parts.push(part);
  }
  a = { statics, parts };
  cache.set(strings, a);
  return a;
}

const URL_ATTRS = new Set(['href', 'src', 'action', 'formaction', 'xlink:href', 'poster']);
function attrValue(name, v) {
  if (v == null || v === false) return '';
  let s = String(v);
  if (URL_ATTRS.has(name) && /^\s*(javascript|vbscript|data:(?!image\/))/i.test(s)) s = '#';
  return esc(s);
}

function renderText(v, ctx) {
  if (v == null || v === false || v === true) return '';
  if (Array.isArray(v)) {
    let out = '';
    for (const x of v) out += renderText(x, ctx);
    return out;
  }
  if (v[TR]) return build(v, ctx);
  if (v[RAW]) return v.s;
  return esc(v);
}

function build(t, ctx) {
  const { statics, parts } = analyze(t.strings);
  let out = statics[0];
  for (let i = 0; i < parts.length; i++) {
    const p = parts[i];
    const v = t.values[i];
    switch (p.k) {
      case 'text': out += renderText(v, ctx); break;
      case 'attr': { const s = attrValue(p.name, v); out += p.quoted ? s : `"${s}"`; break; }
      case 'apart': out += v == null || v === false ? '' : esc(v); break;
      case '?': if (v) out += p.name; break;
      case '@':
        if (typeof v === 'function') {
          const id = ctx.h.push(v) - 1;
          out += `data-h data-on-${p.name.toLowerCase()}="${id}"`;
        }
        break;
      case '.': {
        const id = ctx.p.push({ name: p.name, v }) - 1;
        out += `data-h data-prop-${p.name.toLowerCase()}="${id}"`;
        break;
      }
      case 'tag': if (v && v[RAW]) out += v.s; break;
    }
    out += statics[i + 1];
  }
  return out;
}

/** Render a template to an HTML string (event/prop bindings are dropped). */
export function toHTML(t) {
  return renderText(t, { h: [], p: [] });
}

// ── Morphing ────────────────────────────────────────────────────────────
const keyOf = (n) => (n.nodeType === 1 ? n.getAttribute('data-key') : null);
const sameKind = (a, b) => a.nodeType === b.nodeType && (a.nodeType !== 1 || a.tagName === b.tagName);

function morphChildren(from, to) {
  const keyed = new Map();
  for (let c = from.firstChild; c; c = c.nextSibling) {
    const k = keyOf(c);
    if (k != null) keyed.set(k, c);
  }
  let cur = from.firstChild;
  let n = to.firstChild;
  while (n) {
    const next = n.nextSibling;
    const k = keyOf(n);
    let match = null;
    if (k != null) {
      const m = keyed.get(k);
      if (m && m.tagName === n.tagName) { match = m; keyed.delete(k); }
    } else if (cur && keyOf(cur) == null && sameKind(cur, n)) {
      match = cur;
    }
    if (match) {
      if (match === cur) cur = cur.nextSibling;
      else from.insertBefore(match, cur);
      morphNode(match, n);
    } else {
      from.insertBefore(n, cur);
    }
    n = next;
  }
  while (cur) {
    const nx = cur.nextSibling;
    from.removeChild(cur);
    cur = nx;
  }
}

function syncAttrs(a, b, skip) {
  const attrs = a.attributes;
  for (let i = attrs.length - 1; i >= 0; i--) {
    const name = attrs[i].name;
    if (skip && skip.has(name)) continue;
    if (!b.hasAttribute(name)) {
      a.removeAttribute(name);
      if (a.__on && name.startsWith('data-on-')) a.__on[name.slice(8)] = null;
    }
  }
  for (const { name, value } of b.attributes) {
    if (skip && skip.has(name)) continue;
    if (a.getAttribute(name) !== value) a.setAttribute(name, value);
  }
}

const SKIP_INPUT = new Set(['value', 'checked']);
const SKIP_DETAILS = new Set(['open']);

function morphNode(a, b) {
  if (a.nodeType !== 1) {
    if (a.nodeValue !== b.nodeValue) a.nodeValue = b.nodeValue;
    return;
  }
  const tag = a.tagName;
  if (tag === 'INPUT') {
    syncAttrs(a, b, SKIP_INPUT);
    // "Controlled when the model changes": only push a new value when the
    // template's value changed, so un-tracked typing is never clobbered.
    const nv = b.getAttribute('value');
    if (nv !== null && nv !== a.getAttribute('value')) {
      a.setAttribute('value', nv);
      if (a.value !== nv) a.value = nv;
    }
    if (a.type === 'checkbox' || a.type === 'radio') a.checked = b.hasAttribute('checked');
    return;
  }
  if (tag === 'TEXTAREA') {
    syncAttrs(a, b);
    const nv = b.textContent;
    if (nv !== a.defaultValue) {
      a.defaultValue = nv;
      if (a.value !== nv) a.value = nv;
    }
    return;
  }
  syncAttrs(a, b, tag === 'DETAILS' ? SKIP_DETAILS : null);
  if (tag.includes('-') || a.hasAttribute('data-keep')) return;
  morphChildren(a, b);
  if (tag === 'SELECT') {
    const sel = b.querySelector('option[selected]');
    if (sel && a.value !== sel.value) a.value = sel.value;
  }
}

// ── Event / property binding ────────────────────────────────────────────
function bind(root, ctx) {
  const els = root.querySelectorAll('[data-h]');
  for (const el of els) {
    for (const { name, value } of el.attributes) {
      if (name.startsWith('data-on-')) {
        const type = name.slice(8);
        const fn = ctx.h[+value];
        const on = el.__on || (el.__on = {});
        if (!(type in on)) el.addEventListener(type, (e) => { const h = el.__on[type]; if (h) h(e); });
        on[type] = fn;
      } else if (name.startsWith('data-prop-')) {
        const p = ctx.p[+value];
        if (!p) continue;
        if (el[p.name] !== p.v) el[p.name] = p.v;
      }
    }
  }
}

/** Render a template into a container, morphing existing DOM. */
export function render(container, tpl) {
  const ctx = { h: [], p: [] };
  const markup = renderText(tpl, ctx);
  const t = document.createElement('template');
  t.innerHTML = markup;
  morphChildren(container, t.content);
  bind(container, ctx);
}

/**
 * Mount a view function into a container. Returns { update, destroy, now }.
 * update() is batched to one render per microtask.
 */
export function mountView(container, viewFn, { onError } = {}) {
  let queued = false;
  let dead = false;
  const run = () => {
    if (dead) return;
    try {
      render(container, viewFn());
    } catch (err) {
      console.error('[render]', err);
      if (onError) onError(err);
      else render(container, html`<div class="banner danger"><strong>Something went wrong drawing this view.</strong> ${String(err && err.message || err)}</div>`);
    }
  };
  run();
  return {
    update() {
      if (queued || dead) return;
      queued = true;
      queueMicrotask(() => { queued = false; run(); });
    },
    now: run,
    destroy() { dead = true; },
  };
}

// ── Small helpers ───────────────────────────────────────────────────────
export function classes(...xs) {
  const out = [];
  for (const x of xs) {
    if (!x) continue;
    if (typeof x === 'string') out.push(x);
    else for (const k in x) if (x[k]) out.push(k);
  }
  return out.join(' ');
}

export function styleMap(obj) {
  let s = '';
  for (const k in obj) {
    const v = obj[k];
    if (v == null || v === false || v === '') continue;
    s += `${k}:${v};`;
  }
  return s;
}
