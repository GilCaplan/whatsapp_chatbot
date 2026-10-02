// router.js — hash router (#/name/param?query).
//
// Each page module default-exports a factory: (ctx) => instance where
// instance = { view(), mount?(el), unmount?(), onParams?(route), isDirty?(), title?, bare? }.
// ctx = { route, update(), navigate }. Pages re-render on store changes.

import { mountView } from './dom.js';
import { prefersReducedMotion } from './util.js';

let routes = {};
let fallback = 'dashboard';
let current = null;   // { route, inst, view, el }
let host = null;
let onChange = () => {};
let pageUpdated = () => {};
let confirmLeave = async () => true;
let suppress = false;

export function parseHash(hash = location.hash) {
  const h = (hash || '').replace(/^#\/?/, '');
  const [path, qs = ''] = h.split('?');
  const segs = path.split('/').filter(Boolean);
  const name = segs[0] || '';
  let param = '';
  try { param = segs.slice(1).map(decodeURIComponent).join('/'); } catch { param = segs.slice(1).join('/'); }
  return { name, param, query: new URLSearchParams(qs), path: '/' + path, hash: '#/' + h };
}

export function navigate(path, { replace = false } = {}) {
  const target = '#' + (path.startsWith('/') ? path : '/' + path);
  if (replace) {
    history.replaceState(null, '', target);
    handle();
  } else if (location.hash === target) {
    handle();
  } else {
    location.hash = target;
  }
}

export function currentRoute() { return current && current.route; }
export function currentPage() { return current; }
export function refreshPage() { if (current) current.view.update(); }

export function initRouter({ table, defaultRoute, el, changed, beforeLeave, updated }) {
  pageUpdated = updated || pageUpdated;
  routes = table;
  fallback = defaultRoute;
  host = el;
  onChange = changed || onChange;
  confirmLeave = beforeLeave || confirmLeave;
  window.addEventListener('hashchange', () => { if (!suppress) handle(); });
}

export async function handle() {
  let route = parseHash();
  if (!routes[route.name]) {
    history.replaceState(null, '', '#/' + fallback);
    route = parseHash();
  }

  // Same page, different param/query → let the page react in place.
  if (current && current.route.name === route.name && current.inst.onParams) {
    if (current.route.param !== route.param && current.inst.isDirty && current.inst.isDirty()) {
      const back = current.route.hash;
      suppress = true;
      history.replaceState(null, '', back);
      setTimeout(() => { suppress = false; }, 0);
      if (!(await confirmLeave())) return;
      history.replaceState(null, '', route.hash);
    }
    current.route = route;
    current.inst.onParams(route);
    current.view.update();
    onChange(route, current);
    return;
  }

  if (current && current.inst.isDirty && current.inst.isDirty()) {
    const back = current.route.hash;
    suppress = true;
    history.replaceState(null, '', back);
    setTimeout(() => { suppress = false; }, 0);
    if (!(await confirmLeave())) return;
    current.inst.isDirty = () => false;
    history.replaceState(null, '', route.hash);
  }

  swap(route);
}

function swap(route) {
  const factory = routes[route.name];
  const doSwap = () => {
    if (current) {
      try { current.inst.unmount && current.inst.unmount(); } catch (e) { console.error(e); }
      current.view.destroy();
    }
    host.textContent = '';
    const el = document.createElement('div');
    host.append(el);
    const cur = { route, inst: null, view: null, el };
    const ctx = {
      get route() { return cur.route; },
      navigate,
      update: () => { if (cur.view) cur.view.update(); pageUpdated(); },
    };
    cur.inst = factory(ctx);
    el.className = 'page ' + (cur.inst.pageClass || '');
    cur.view = mountView(el, () => cur.inst.view());
    current = cur;
    host.scrollTop = 0;
    onChange(route, cur);
    try { cur.inst.mount && cur.inst.mount(el); } catch (e) { console.error(e); }
    return el;
  };

  const canVT = typeof document.startViewTransition === 'function' && !prefersReducedMotion() && current;
  if (canVT) {
    try {
      document.startViewTransition(() => { doSwap(); });
      return;
    } catch { /* fall through */ }
  }
  const el = doSwap();
  if (!prefersReducedMotion()) el.classList.add('page-enter');
}
