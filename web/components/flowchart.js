// components/flowchart.js — <cs-flowchart>: an interactive flowchart of a
// context system (drag blocks, wire them, select, delete; pan and zoom).
// It draws its own SVG (the page's morphing renderer leaves custom-element
// children alone) and reports edits as events:
//   fc-change {chart}  after any edit (drag end, wire, delete, add)
//   fc-select {kind:'node'|'edge'|null, id|index}
// Set .data = {chart, blocks, locked, selected, key}; a new chart object
// redraws, a new key also fits the view to it.
// Built-in charts are passed locked: pan, zoom and select only.

const NS = 'http://www.w3.org/2000/svg';
const W = 200, H = 56, GRID = 20;
const KIND_TAG = { ai: 'AI call', code: 'code', data: 'reads', io: '', note: 'yours' };

const el = (tag, attrs = {}, parent) => {
  const n = document.createElementNS(NS, tag);
  for (const [k, v] of Object.entries(attrs)) if (v != null) n.setAttribute(k, v);
  if (parent) parent.appendChild(n);
  return n;
};

// Two lines of at most ~26 characters.
function wrap(label) {
  const words = String(label || '').split(/\s+/);
  const lines = [''];
  for (const w of words) {
    const cur = lines[lines.length - 1];
    if ((cur + ' ' + w).trim().length > 26 && cur) {
      if (lines.length === 2) { lines[1] = (lines[1] + ' ' + w).trim(); continue; }
      lines.push(w);
    } else lines[lines.length - 1] = (cur + ' ' + w).trim();
  }
  return lines.map((l) => (l.length > 30 ? l.slice(0, 29) + '…' : l));
}

// Where a wire leaves a and enters b: bottom→top when b is below, else the
// facing sides.
function anchors(a, b) {
  const ac = { x: a.x + W / 2, y: a.y + H / 2 }, bc = { x: b.x + W / 2, y: b.y + H / 2 };
  const dx = bc.x - ac.x, dy = bc.y - ac.y;
  if (Math.abs(dy) > H * 0.9 && Math.abs(dy) >= Math.abs(dx) * 0.35) {
    return dy > 0
      ? [{ x: ac.x, y: a.y + H }, { x: bc.x, y: b.y }, 'v']
      : [{ x: ac.x, y: a.y }, { x: bc.x, y: b.y + H }, 'v'];
  }
  return dx > 0
    ? [{ x: a.x + W, y: ac.y }, { x: b.x, y: bc.y }, 'h']
    : [{ x: a.x, y: ac.y }, { x: b.x + W, y: bc.y }, 'h'];
}

function wirePath(p, q, dir) {
  const k = dir === 'v' ? Math.max(30, Math.abs(q.y - p.y) / 2) : Math.max(30, Math.abs(q.x - p.x) / 2);
  const c1 = dir === 'v' ? { x: p.x, y: p.y + Math.sign(q.y - p.y || 1) * k } : { x: p.x + Math.sign(q.x - p.x || 1) * k, y: p.y };
  const c2 = dir === 'v' ? { x: q.x, y: q.y - Math.sign(q.y - p.y || 1) * k } : { x: q.x - Math.sign(q.x - p.x || 1) * k, y: q.y };
  return `M${p.x} ${p.y} C${c1.x} ${c1.y} ${c2.x} ${c2.y} ${q.x} ${q.y}`;
}

class CtxFlowchart extends HTMLElement {
  constructor() {
    super();
    this._data = { chart: { nodes: [], edges: [] }, blocks: {}, locked: true, selected: null };
    this._view = { x: 0, y: 0, k: 1 };
    this._fitted = false;
  }

  set data(d) {
    const prev = this._data;
    this._data = { ...prev, ...d };
    if (!this._svg) return;
    // A new chart redraws; a new key (another system, a tidy-up) also re-fits the view.
    if (d.chart !== prev.chart || d.key !== prev.key) {
      if (d.key !== prev.key) this._fitted = false;
      this.draw();
      return;
    }
    if (d.locked !== prev.locked || d.selected !== prev.selected) this.draw();
  }
  get data() { return this._data; }

  connectedCallback() {
    if (this._svg) return;
    this.classList.add('fc');
    this._svg = el('svg', { class: 'fc-svg', role: 'img', 'aria-label': 'Flowchart of the context system', tabindex: '0' }, this);
    const defs = el('defs', {}, this._svg);
    const m = el('marker', { id: 'fc-arrow', viewBox: '0 0 10 10', refX: '9', refY: '5', markerWidth: '7', markerHeight: '7', orient: 'auto-start-reverse' }, defs);
    el('path', { d: 'M0 0 L10 5 L0 10 z', class: 'fc-arrowhead' }, m);
    this._g = el('g', {}, this._svg);
    this._svg.addEventListener('pointerdown', (e) => this.onDown(e));
    this._svg.addEventListener('wheel', (e) => this.onWheel(e), { passive: false });
    this._svg.addEventListener('keydown', (e) => this.onKey(e));
    this._ro = new ResizeObserver(() => { if (!this._fitted) this.fit(); });
    this._ro.observe(this);
    this.draw();
  }

  disconnectedCallback() { if (this._ro) this._ro.disconnect(); }

  get chart() { return this._data.chart; }

  emit(type, detail) { this.dispatchEvent(new CustomEvent(type, { detail, bubbles: true })); }
  changed() { this.emit('fc-change', { chart: this.chart }); }
  select(sel) { this._data.selected = sel; this.emit('fc-select', sel); this.draw(); }

  // ── Viewport ──
  apply() { this._g.setAttribute('transform', `translate(${this._view.x} ${this._view.y}) scale(${this._view.k})`); }
  fit() {
    const { nodes } = this.chart;
    const r = this.getBoundingClientRect();
    if (!r.width || !nodes.length) { this.apply(); return; }
    const xs = nodes.map((n) => n.x), ys = nodes.map((n) => n.y);
    const minX = Math.min(...xs) - 30, minY = Math.min(...ys) - 30;
    const w = Math.max(...xs) + W + 30 - minX, h = Math.max(...ys) + H + 30 - minY;
    // Fit the width (readable text); a tall chart starts at the top and scrolls.
    const k = Math.max(0.55, Math.min(1.1, r.width / w));
    const y = h * k <= r.height ? (r.height - h * k) / 2 - minY * k : 12 - minY * k;
    const x = w * k <= r.width ? (r.width - w * k) / 2 - minX * k : 12 - minX * k;
    this._view = { k, x, y };
    this._fitted = true;
    this.apply();
  }
  zoom(f, cx, cy) {
    const r = this.getBoundingClientRect();
    cx = cx ?? r.width / 2; cy = cy ?? r.height / 2;
    const v = this._view, k = Math.max(0.25, Math.min(2.2, v.k * f));
    this._view = { k, x: cx - (cx - v.x) * (k / v.k), y: cy - (cy - v.y) * (k / v.k) };
    this.apply();
  }
  toChart(e) {
    const r = this._svg.getBoundingClientRect(), v = this._view;
    return { x: (e.clientX - r.left - v.x) / v.k, y: (e.clientY - r.top - v.y) / v.k };
  }
  center() {
    const r = this.getBoundingClientRect(), v = this._view;
    return { x: (r.width / 2 - v.x) / v.k - W / 2, y: (r.height / 2 - v.y) / v.k - H / 2 };
  }

  // ── Editing API (the page's palette and inspector call these) ──
  addBlock(block, label) {
    const ids = new Set(this.chart.nodes.map((n) => n.id));
    let i = this.chart.nodes.length + 1;
    while (ids.has('n' + i)) i++;
    const c = this.center();
    const off = (this.chart.nodes.length % 5) * GRID;
    const n = { id: 'n' + i, block: block.id, label: label || block.label, x: Math.round((c.x + off) / GRID) * GRID, y: Math.round((c.y + off) / GRID) * GRID };
    this.chart.nodes.push(n);
    this.changed();
    this.select({ kind: 'node', id: n.id });
  }
  removeSelected() {
    const s = this._data.selected;
    if (!s || this._data.locked) return;
    if (s.kind === 'node') {
      this.chart.nodes = this.chart.nodes.filter((n) => n.id !== s.id);
      this.chart.edges = this.chart.edges.filter((e) => e.from !== s.id && e.to !== s.id);
    } else if (s.kind === 'edge') {
      this.chart.edges.splice(s.index, 1);
    }
    this.changed();
    this.select(null);
  }
  redraw() { this.draw(); }

  // ── Pointer ──
  onDown(e) {
    if (e.button !== 0) return;
    this._svg.focus({ preventScroll: true });
    const t = e.target.closest('[data-node],[data-edge],[data-port]');
    const locked = this._data.locked;
    const start = { x: e.clientX, y: e.clientY };
    if (t && t.dataset.port != null && !locked) { // new wire
      const from = t.dataset.port;
      const ghost = el('path', { class: 'fc-wire fc-ghost' }, this._g);
      const a = this.chart.nodes.find((n) => n.id === from);
      const move = (ev) => {
        const p = this.toChart(ev);
        ghost.setAttribute('d', `M${a.x + W / 2} ${a.y + H} L${p.x} ${p.y}`);
      };
      const up = (ev) => {
        window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up);
        ghost.remove();
        const p = this.toChart(ev);
        const to = this.chart.nodes.find((n) => p.x >= n.x && p.x <= n.x + W && p.y >= n.y && p.y <= n.y + H);
        if (to && to.id !== from && !this.chart.edges.some((x) => x.from === from && x.to === to.id)) {
          const fb = this._data.blocks[a.block] || {}, tb = this._data.blocks[to.block] || {};
          const dashed = fb.kind === 'data' || fb.stage === 'background' || tb.kind === 'data';
          this.chart.edges.push({ from, to: to.id, label: '', dashed });
          this.changed();
          this.select({ kind: 'edge', index: this.chart.edges.length - 1 });
        }
      };
      window.addEventListener('pointermove', move); window.addEventListener('pointerup', up);
      e.preventDefault();
      return;
    }
    if (t && t.dataset.node != null) {
      const n = this.chart.nodes.find((x) => x.id === t.dataset.node);
      const orig = { x: n.x, y: n.y };
      let moved = false;
      const move = (ev) => {
        if (locked) return;
        const dx = (ev.clientX - start.x) / this._view.k, dy = (ev.clientY - start.y) / this._view.k;
        if (!moved && Math.hypot(dx, dy) < 3) return;
        moved = true;
        n.x = Math.round((orig.x + dx) / GRID) * GRID;
        n.y = Math.round((orig.y + dy) / GRID) * GRID;
        this.draw();
      };
      const up = () => {
        window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up);
        if (moved) this.changed();
        this.select({ kind: 'node', id: n.id });
      };
      window.addEventListener('pointermove', move); window.addEventListener('pointerup', up);
      e.preventDefault();
      return;
    }
    if (t && t.dataset.edge != null) { this.select({ kind: 'edge', index: +t.dataset.edge }); return; }
    // Background: pan.
    const v0 = { ...this._view };
    let moved = false;
    const move = (ev) => {
      const dx = ev.clientX - start.x, dy = ev.clientY - start.y;
      if (!moved && Math.hypot(dx, dy) < 3) return;
      moved = true;
      this._view = { ...v0, x: v0.x + dx, y: v0.y + dy };
      this.apply();
    };
    const up = () => {
      window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up);
      if (!moved && this._data.selected) this.select(null);
    };
    window.addEventListener('pointermove', move); window.addEventListener('pointerup', up);
  }

  onWheel(e) {
    e.preventDefault();
    const r = this._svg.getBoundingClientRect();
    if (e.ctrlKey || e.metaKey) { this.zoom(Math.exp(-e.deltaY * 0.01), e.clientX - r.left, e.clientY - r.top); return; }
    this._view = { ...this._view, x: this._view.x - e.deltaX, y: this._view.y - e.deltaY };
    this.apply();
  }

  onKey(e) {
    if ((e.key === 'Delete' || e.key === 'Backspace') && this._data.selected && !this._data.locked) {
      e.preventDefault();
      this.removeSelected();
    } else if (e.key === 'Escape' && this._data.selected) {
      this.select(null);
    } else if (e.key === '+' || e.key === '=') this.zoom(1.2);
    else if (e.key === '-') this.zoom(1 / 1.2);
  }

  // ── Drawing ──
  draw() {
    if (!this._g) return;
    const { chart, blocks, locked, selected } = this._data;
    const g = this._g;
    g.replaceChildren();
    this.classList.toggle('fc-locked', !!locked);
    const byId = new Map(chart.nodes.map((n) => [n.id, n]));
    const wires = el('g', { class: 'fc-wires' }, g);
    const labels = el('g', {}, g);
    chart.edges.forEach((e, i) => {
      const a = byId.get(e.from), b = byId.get(e.to);
      if (!a || !b) return;
      const [p, q, dir] = anchors(a, b);
      const d = wirePath(p, q, dir);
      const sel = selected && selected.kind === 'edge' && selected.index === i;
      el('path', { d, class: 'fc-hit', 'data-edge': i }, wires);
      el('path', { d, class: 'fc-wire' + (e.dashed ? ' fc-dashed' : '') + (sel ? ' fc-sel' : ''), 'marker-end': 'url(#fc-arrow)', 'data-edge': i }, wires);
      if (e.label) {
        const mx = (p.x + q.x) / 2, my = (p.y + q.y) / 2;
        const lg = el('g', { class: 'fc-label', 'data-edge': i }, labels);
        const w = e.label.length * 6.4 + 12;
        el('rect', { x: mx - w / 2, y: my - 10, width: w, height: 20, rx: 10 }, lg);
        el('text', { x: mx, y: my + 4, 'text-anchor': 'middle' }, lg).textContent = e.label;
      }
    });
    for (const n of chart.nodes) {
      const b = blocks[n.block] || { kind: 'note' };
      const sel = selected && selected.kind === 'node' && selected.id === n.id;
      const ng = el('g', { class: `fc-node fc-${b.kind}${sel ? ' fc-sel' : ''}`, 'data-node': n.id, transform: `translate(${n.x} ${n.y})` }, g);
      const t = el('title', {}, ng);
      t.textContent = (n.label || b.label || '') + (b.description ? ' — ' + b.description : '') + (n.note ? '\n' + n.note : '');
      el('rect', { width: W, height: H, rx: b.kind === 'io' ? H / 2 : 12, class: 'fc-box' }, ng);
      const lines = wrap(n.label || b.label);
      const tag = KIND_TAG[b.kind] || '';
      const two = lines.length > 1;
      const first = tag ? (two ? 31 : 37) : (two ? 25 : 33);
      if (tag) el('text', { x: 14, y: 15, class: 'fc-tag' }, ng).textContent = tag;
      lines.forEach((l, i) => { el('text', { x: 14, y: first + i * 15, class: 'fc-text' }, ng).textContent = l; });
      if (n.note) el('circle', { cx: W - 12, cy: 12, r: 4, class: 'fc-notedot' }, ng);
      if (!locked) el('circle', { cx: W / 2, cy: H, r: 6, class: 'fc-port', 'data-port': n.id }, ng);
    }
    this.apply();
    if (!this._fitted) this.fit();
  }
}

if (!customElements.get('cs-flowchart')) customElements.define('cs-flowchart', CtxFlowchart);
