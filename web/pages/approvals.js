// pages/approvals.js — review generated replies before they're sent.

import { html } from '../dom.js';
import { icon } from '../icons.js';
import { api } from '../api.js';
import { store, personaById, chatByKey, loadApprovals } from '../store.js';
import { toast } from '../ui.js';
import { avatarStack } from '../components/avatar.js';
import { bubbles } from '../components/chat-drawer.js';
import { emptyState } from '../components/art.js';
import { providerLabel } from '../components/status.js';
import { rowsFor, modKey } from '../util.js';
import { taggedIn } from '../components/mentions.js';
import { TONES, catGlyph, modeOf, catLabel } from '../components/handoff-meta.js';

export default function Approvals(ctx) {
  const update = () => ctx.update();
  const edits = new Map();     // id (or "id#draft" in co-pilot) -> edited text
  const picked = new Map();    // id -> chosen co-pilot draft (index)
  const busyIds = new Map();   // id -> 'approve' | 'regen' | 'discard'
  const leaving = new Set();

  function removeSoon(id) {
    leaving.add(id);
    update();
    setTimeout(() => {
      leaving.delete(id);
      forget(id);
      store.set({ approvals: store.state.approvals.filter((a) => a.id !== id) });
    }, 280);
  }

  /** Drops the local edits and draft choice of an approval. */
  function forget(id) {
    for (const k of [...edits.keys()]) if (k === id || k.startsWith(id + '#')) edits.delete(k);
    picked.delete(id);
  }

  const hasDrafts = (p) => Array.isArray(p.drafts) && p.drafts.length > 0;
  const draftIdx = (p) => (hasDrafts(p) ? Math.min(picked.get(p.id) || 0, p.drafts.length - 1) : -1);
  const editKey = (p) => (hasDrafts(p) ? `${p.id}#${draftIdx(p)}` : p.id);
  const baseText = (p) => (hasDrafts(p) ? p.drafts[draftIdx(p)].text : p.text);

  async function approve(p) {
    if (busyIds.has(p.id)) return;
    const k = editKey(p);
    const edited = edits.has(k) ? edits.get(k) : null;
    const text = edited != null && edited.trim() !== baseText(p) ? edited.trim() : null;
    if (edited != null && !edited.trim()) { toast('The reply is empty — write something or discard it', { type: 'warn' }); return; }
    const draft = hasDrafts(p) ? draftIdx(p) : undefined;
    busyIds.set(p.id, 'approve'); update();
    try {
      await api.approvals.approve(p.id, text, undefined, draft);
      toast(html`Sent to <b>${p.chatName || 'chat'}</b>`, { type: 'success' });
      removeSoon(p.id);
    } catch (e) {
      if (e.status === 404) { removeSoon(p.id); }
    } finally { busyIds.delete(p.id); update(); }
  }

  async function regenerate(p) {
    if (busyIds.has(p.id)) return;
    busyIds.set(p.id, 'regen'); update();
    try {
      const np = await api.approvals.regenerate(p.id);
      forget(p.id);
      if (np && np.id) store.set({ approvals: store.state.approvals.map((a) => (a.id === np.id ? np : a)) });
    } catch (e) {
      if (e.status === 404) removeSoon(p.id);
    } finally { busyIds.delete(p.id); update(); }
  }

  async function discard(p) {
    if (busyIds.has(p.id)) return;
    busyIds.set(p.id, 'discard'); update();
    try {
      await api.approvals.discard(p.id);
      removeSoon(p.id);
      toast('Discarded');
    } catch (e) {
      if (e.status === 404) removeSoon(p.id);
    } finally { busyIds.delete(p.id); update(); }
  }

  function card(p) {
    const chat = chatByKey(p.chatKey);
    const persona = personaById(p.personaId);
    const busy = busyIds.get(p.id);
    const k = editKey(p);
    const base = baseText(p);
    const text = edits.has(k) ? edits.get(k) : base;
    const edited = edits.has(k) && edits.get(k) !== base;
    const drafts = hasDrafts(p);
    const mode = modeOf(chat);
    const copilot = drafts || mode === 'copilot';
    const autoAt = p.autoSendAt ? Date.parse(p.autoSendAt) : 0;
    // Auto-send delay is per behaviour profile / chat now, so derive it from the item itself.
    const total = autoAt && p.createdAt ? Math.max(1, Math.round((autoAt - Date.parse(p.createdAt)) / 1000)) : 30;
    const ctxMsgs = (p.context || []).slice(-4);
    const willTag = taggedIn(text, p.mentions);
    return html`<article class=${'card approval-card ' + (leaving.has(p.id) ? 'leaving ' : '') + (p.stale ? 'is-stale' : '')} data-key=${p.id}>
      ${p.stale ? html`<div class="stale-ribbon">${icon('warning', 'ic-sm')}Stale — new messages arrived since this was written. Consider regenerating.</div>` : ''}
      <header class="ap-head">
        ${avatarStack({ jid: chat ? chat.jid : '', name: p.chatName || (chat && chat.name) || '?', kind: chat ? chat.kind : '' }, persona, 46)}
        <div class="grow" style="min-width:0">
          <div class="h3 ellipsis">${p.chatName || (chat && chat.name) || p.chatKey}</div>
          <div class="small muted ellipsis">as <b>${p.personaName || (persona && persona.name) || 'persona'}</b>${copilot ? html` <span class="chip chip-sm chip-violet ap-mode">${icon('sparkles')}Co-pilot</span>` : ''} · <rel-time datetime=${p.createdAt}></rel-time>
            ${p.provider ? html` · ${providerLabel(p.provider)}${p.model ? ' ' + p.model : ''}` : ''}</div>
        </div>
        ${autoAt ? html`<div class="auto-send" title="Sends the original reply automatically unless you act">
          <count-ring deadline=${autoAt} total=${total} size="42"></count-ring><span class="tiny faint">auto-send</span></div>` : ''}
      </header>

      ${ctxMsgs.length ? html`<div class="chat-wall ap-context">${bubbles(ctxMsgs, { group: chat && chat.kind === 'group' })}</div>` : ''}

      <div class="ap-reply">
        ${chat && chat.handoff ? html`<div class="banner danger mb-12 small">${catGlyph(chat.handoff.category)}<div><strong>Needs you · ${catLabel(chat.handoff.category)}.</strong> ${p.personaName || 'The persona'} is paused in this chat, so this won't send by itself. You can still send it if it fits.</div></div>` : ''}
        ${drafts ? html`<div class="tone-tiles" role="radiogroup" aria-label="Pick a reply">${p.drafts.map((d, i) => {
          const t = TONES[d.tone] || { label: `Idea ${i + 1}`, line: '' };
          const on = i === draftIdx(p);
          const dk = `${p.id}#${i}`;
          const dt = edits.has(dk) ? edits.get(dk) : d.text;
          return html`<button type="button" class=${'tone-tile tone-' + (d.tone || 'brief')} role="radio" aria-checked=${String(on)} aria-pressed=${String(on)}
              ?disabled=${!!busy} @click=${() => { picked.set(p.id, i); update(); }}>
            <span class="tone-head">${catGlyph(d.tone || 'brief')}${t.label}<span class="tone-sub">${t.line}</span>
              ${edits.has(dk) && edits.get(dk) !== d.text ? html`<span class="chip chip-sm">edited</span>` : ''}</span>
            <span class="tone-text">${dt}</span>
          </button>`;
        })}</div>` : ''}
        <label class="field-label" for=${'ap-' + p.id}>${icon('sparkles', 'ic-sm')}${drafts ? `Your pick: ${(TONES[p.drafts[draftIdx(p)].tone] || { label: 'this one' }).label.toLowerCase()}` : 'Suggested reply'} ${edited ? html`<span class="chip chip-sm">edited</span>` : ''}
          ${edited && autoAt ? html`<span class="chip chip-sm chip-amber" title="The timer sends the original text">${icon('clock')}Send your edit before the timer runs out</span>` : ''}</label>
        <div class=${'ap-bubble ' + (busy === 'regen' ? 'regen' : '')}>
          <textarea id=${'ap-' + p.id} class="textarea" rows=${rowsFor(text, 2, 10)} ?disabled=${busy === 'regen'} .value=${text}
            @input=${(e) => { edits.set(k, e.target.value); update(); }}
            @keydown=${(e) => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); approve(p); } }}></textarea>
          ${busy === 'regen' ? html`<div class="ap-regen-overlay"><span class="typing"><span></span><span></span><span></span></span></div>` : ''}
        </div>
        ${willTag.length ? html`<div class="ap-tags small muted" title="Tagged people get a WhatsApp notification">${icon('at', 'ic-sm')}Will tag ${willTag.map((n, i) => html`${i ? ', ' : ''}<span class="mention">@${n}</span>`)}</div>` : ''}
        ${copilot && !drafts ? html`<div class="ap-note">${icon('info')}<span>Only one idea came out this time. Tap <b>More ideas</b> for three to pick from.</span></div>` : ''}
      </div>

      <footer class="ap-actions">
        <button class=${'btn btn-ghost btn-sm danger-text ' + (busy === 'discard' ? 'loading' : '')} ?disabled=${!!busy} @click=${() => discard(p)}>${icon('trash')}Discard</button>
        <span class="grow"></span>
        <span class="tiny faint hide-sm"><kbd>${modKey()}</kbd> <kbd>↵</kbd> to send</span>
        <button class=${'btn btn-glass btn-sm ' + (busy === 'regen' ? 'loading' : '')} ?disabled=${!!busy} @click=${() => regenerate(p)}>${icon('refresh')}${copilot ? 'More ideas' : 'Regenerate'}</button>
        <button class=${'btn btn-primary ' + (busy === 'approve' ? 'loading' : '')} ?disabled=${!!busy} @click=${() => approve(p)}>${icon('send')}${edited ? 'Send edited' : drafts ? 'Send this one' : 'Approve & send'}</button>
      </footer>
    </article>`;
  }

  function view() {
    const s = store.state;
    const list = s.approvals.slice().sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
    return html`<div class="approvals-page">
      ${!s.approvalsLoaded ? html`<div class="card skeleton" style="height:240px"></div>`
        : !list.length ? html`<div class="card">${emptyState({
          artName: 'approvals',
          title: 'All caught up',
          body: 'When a chat is in Approve or Co-pilot mode, replies wait here for your OK.',
          action: { label: 'Manage chats', icon: 'chats', cls: 'btn-glass', onClick: () => ctx.navigate('/chats?tab=assigned') },
        })}</div>`
        : html`<div class="approval-list">${list.map(card)}</div>`}
    </div>`;
  }

  return {
    title: 'Approvals',
    subtitle: () => { const n = store.state.approvals.length; return n ? `${n} waiting` : ''; },
    view,
    mount() { loadApprovals().catch(() => {}); },
  };
}
