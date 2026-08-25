<script>
  import { createEventDispatcher } from 'svelte';
  import { HighlightSource, RenderTELAMarkdown } from '../../../../wailsjs/go/main/App.js';
  import { Icons } from '../holo';
  import SignatureBadge from './SignatureBadge.svelte';

  export let entry = null;
  export let signature = null;
  export let showSignature = true;

  const dispatch = createEventDispatcher();

  let renderedHtml = '';
  // Per-line HTML from HighlightSource. Each entry is chroma output or escaped
  // text for exactly one source line; absent on older backends and past the
  // backend's line cap (each entry becomes a table row, so a newline-dense
  // file must not arrive per line), in which case the single-block <pre>
  // below is the whole view.
  let lines = [];
  let language = '';
  let highlighted = false;
  let reason = '';
  let loading = false;
  let renderError = '';
  let asMarkdown = false;
  let loadToken = 0;

  $: isMarkdown = /\.md$/i.test(entry?.name || '');
  // Gutter width in ch, sized once for the largest line number so the column
  // never shifts while scrolling.
  $: gutterCh = String(lines.length || 1).length;
  $: hasBody = entry?.kind === 'doc' && typeof entry?.content === 'string' && entry.content.length > 0;

  // A new file resets the view mode before anything is fetched, so a slow
  // response cannot paint the previous file's body under the new file's name.
  $: if (entry) resetFor(entry);

  function resetFor(next) {
    renderedHtml = '';
    lines = [];
    language = '';
    highlighted = false;
    reason = '';
    renderError = '';
    asMarkdown = /\.md$/i.test(next?.name || '');
    load();
  }

  async function load() {
    const token = ++loadToken;
    const target = entry;

    if (!target || target.kind !== 'doc' || typeof target.content !== 'string' || target.content.length === 0) {
      loading = false;
      return;
    }

    loading = true;
    try {
      if (asMarkdown) {
        const result = await RenderTELAMarkdown(target.content);
        if (token !== loadToken) return;
        if (result?.success) {
          renderedHtml = result.html || '';
        } else {
          // Fall back to source rather than a blank pane. The renderer refuses
          // oversized or binary input, which is a reason to show the bytes, not
          // to show nothing.
          renderError = result?.error || 'This document could not be rendered as markdown.';
          asMarkdown = false;
          await load();
          return;
        }
      } else {
        const result = await HighlightSource(target.name || '', target.content);
        if (token !== loadToken) return;
        renderedHtml = result?.html || '';
        lines = Array.isArray(result?.lines) ? result.lines : [];
        language = result?.language || '';
        highlighted = !!result?.highlighted;
        reason = result?.reason || '';
      }
    } catch (error) {
      if (token !== loadToken) return;
      renderError = 'This file could not be read.';
      renderedHtml = '';
      lines = [];
    } finally {
      if (token === loadToken) loading = false;
    }
  }

  function toggleMarkdown() {
    asMarkdown = !asMarkdown;
    renderError = '';
    load();
  }

  // Rendered markdown carries real anchors. Left alone, a click would navigate
  // the app's own webview away from HOLOGRAM. The sanitiser already restricted
  // schemes to http/https/mailto and dropped relative URLs, so anything that
  // survives is an external address - it goes to the host's confirm-and-open
  // path, never straight out.
  function interceptLink(event) {
    const anchor = event.target?.closest?.('a');
    if (!anchor) return;
    event.preventDefault();
    const href = anchor.getAttribute('href');
    if (href) dispatch('external', { url: href });
  }

  function formatBytes(n) {
    if (typeof n !== 'number' || n < 0) return '';
    if (n < 1024) return `${n} B`;
    return `${(n / 1024).toFixed(1)} KB`;
  }
</script>

{#if !entry}
  <div class="repo-view-empty">
    <div class="repo-view-empty-icon">◎</div>
    <p class="repo-view-empty-title">Select a file</p>
    <p class="repo-view-empty-text">Every file in this repository is a contract on chain.</p>
  </div>
{:else}
  <div class="repo-view">
    <div class="repo-view-header">
      <span class="repo-view-icon"><Icons name="file-code" size={13} /></span>
      <span class="repo-view-name">{entry.name}</span>

      <div class="repo-view-meta">
        {#if language && !asMarkdown && hasBody}
          <span class="repo-view-chip">{language}</span>
        {/if}
        {#if entry.bytes}
          <span class="repo-view-chip">{formatBytes(entry.bytes)}</span>
        {/if}
        {#if showSignature && signature}
          <SignatureBadge state={signature.state} signer={signature.signer} showSigner={true} />
        {/if}
        {#if isMarkdown && hasBody}
          <button type="button" class="repo-view-toggle" on:click={toggleMarkdown}>
            {asMarkdown ? 'View source' : 'View rendered'}
          </button>
        {/if}
      </div>
    </div>

    {#if entry.scid}
      <div class="repo-view-scid" title={entry.scid}>{entry.scid}</div>
    {/if}

    {#if renderError}
      <div class="repo-view-note">{renderError}</div>
    {/if}

    {#if entry.kind === 'index'}
      <div class="repo-view-note">
        This entry is a nested INDEX, not a single document. TELA shards a large
        file this way; its own entries carry the contents and the signatures.
      </div>
    {:else if entry.kind === 'unreadable'}
      <!-- No reason is appended. The backend used to set one that restated this
           exact sentence, so it printed the same fact twice, the second time in
           parentheses as though it were extra detail. -->
      <div class="repo-view-note">
        This contract parses as neither a TELA INDEX nor a DOC, so its contents
        cannot be shown.
      </div>
    {:else if entry.reason && !hasBody}
      <div class="repo-view-note">{entry.reason}</div>
      {#if entry.bytes}
        <div class="repo-view-note-sub">Stored size: {formatBytes(entry.bytes)}</div>
      {/if}
    {:else if !hasBody}
      <div class="repo-view-note">This file is empty.</div>
    {:else if loading}
      <div class="repo-view-loading">
        <span class="repo-view-spinner"></span>
        <span>Rendering…</span>
      </div>
    {:else if asMarkdown}
      <!-- Sanitised by renderMarkdownSafe in Go. Nothing is concatenated onto
           it here; the guarantee covers exactly what the backend returned. -->
      <div class="repo-markdown" on:click={interceptLink} role="presentation">
        {@html renderedHtml}
      </div>
    {:else}
      {#if reason}
        <div class="repo-view-note-sub">{reason}</div>
      {/if}
      <!-- HighlightSource escapes every path that is not chroma output; that
           covers both the whole-file html and every entry in lines. -->
      {#if lines.length > 0}
        <div class="repo-source repo-source-numbered" class:plain={!highlighted}>
          <table class="repo-source-table">
            <tbody>
              {#each lines as line, i}
                <tr>
                  <td class="repo-line-num" style="min-width: {gutterCh}ch">{i + 1}</td>
                  <!-- The '&nbsp;' constant keeps an empty line at full row
                       height; it is never taken from the response. -->
                  <td class="repo-line-code">{@html line || '&nbsp;'}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {:else}
        <pre class="repo-source" class:plain={!highlighted}>{@html renderedHtml}</pre>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .repo-view {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .repo-view-header {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-3) var(--s-4);
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-md) var(--r-md) 0 0;
  }

  .repo-view-icon {
    display: flex;
    align-items: center;
    color: var(--text-4);
    flex-shrink: 0;
  }

  .repo-view-name {
    font-family: var(--font-mono);
    font-size: 13px;
    color: var(--text-1);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-view-meta {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin-left: auto;
    flex-shrink: 0;
  }

  .repo-view-chip {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-4);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    padding: 1px var(--s-2);
    white-space: nowrap;
  }

  .repo-view-toggle {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--cyan-400);
    background: transparent;
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    padding: 2px var(--s-2);
    cursor: pointer;
    transition: all var(--dur-fast) ease;
  }

  .repo-view-toggle:hover {
    border-color: var(--border-accent);
  }

  .repo-view-scid {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-5);
    padding: var(--s-1) var(--s-4);
    background: var(--void-deep);
    border-left: 1px solid var(--border-dim);
    border-right: 1px solid var(--border-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-view-note {
    padding: var(--s-4);
    font-size: 13px;
    color: var(--text-3);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-top: none;
    border-radius: 0 0 var(--r-md) var(--r-md);
    line-height: 1.6;
  }

  .repo-view-note-sub {
    padding: var(--s-2) var(--s-4);
    font-size: 11px;
    color: var(--text-4);
    background: var(--void-deep);
    border-left: 1px solid var(--border-dim);
    border-right: 1px solid var(--border-dim);
  }

  .repo-view-loading {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-6);
    color: var(--text-4);
    font-size: 13px;
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-top: none;
    border-radius: 0 0 var(--r-md) var(--r-md);
  }

  .repo-view-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid var(--cyan-500);
    border-top-color: transparent;
    border-radius: var(--r-full);
    animation: repo-spin 0.6s linear infinite;
  }

  @keyframes repo-spin {
    to { transform: rotate(360deg); }
  }

  .repo-source {
    font-family: var(--font-mono);
    font-size: 12.5px;
    line-height: 1.65;
    color: var(--text-2);
    margin: 0;
    padding: var(--s-4);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-top: none;
    border-radius: 0 0 var(--r-md) var(--r-md);
    white-space: pre;
    overflow: auto;
    max-height: none;
    tab-size: 2;
  }

  .repo-source.plain {
    color: var(--text-3);
  }

  /* Numbered variant. The container keeps .repo-source's scroll behaviour;
     rows must not wrap, so a long line widens the table and scrolls with it. */
  .repo-source-numbered {
    padding: var(--s-4) 0;
  }

  .repo-source-table {
    border-collapse: collapse;
    border-spacing: 0;
    width: 100%;
  }

  .repo-line-num {
    font-family: var(--font-mono);
    color: var(--text-5);
    text-align: right;
    vertical-align: top;
    user-select: none;
    padding: 0 var(--s-3);
    border-right: 1px solid var(--border-dim);
    white-space: pre;
  }

  .repo-line-code {
    padding: 0 var(--s-4);
    white-space: pre;
    vertical-align: top;
    width: 100%;
  }

  .repo-view-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-16) var(--s-6);
    text-align: center;
  }

  .repo-view-empty-icon {
    font-size: 36px;
    color: var(--text-5);
    margin-bottom: var(--s-3);
  }

  .repo-view-empty-title {
    font-family: var(--font-mono);
    font-size: 14px;
    color: var(--text-2);
    margin: 0 0 var(--s-1) 0;
  }

  .repo-view-empty-text {
    font-size: 12px;
    color: var(--text-4);
    margin: 0;
  }

  /* Rendered markdown. Every selector is :global because the HTML comes from
     the backend and carries no Svelte scoping attribute. */
  .repo-markdown {
    padding: var(--s-5) var(--s-6);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-top: none;
    border-radius: 0 0 var(--r-md) var(--r-md);
    color: var(--text-2);
    font-size: 13.5px;
    line-height: 1.7;
    overflow-wrap: break-word;
  }

  .repo-markdown :global(h1),
  .repo-markdown :global(h2),
  .repo-markdown :global(h3),
  .repo-markdown :global(h4),
  .repo-markdown :global(h5),
  .repo-markdown :global(h6) {
    font-family: var(--font-mono);
    color: var(--text-1);
    font-weight: 600;
    letter-spacing: 0.02em;
    margin: var(--s-6) 0 var(--s-3) 0;
    line-height: 1.35;
  }

  .repo-markdown :global(h1) { font-size: 20px; }
  .repo-markdown :global(h2) { font-size: 17px; }
  .repo-markdown :global(h3) { font-size: 15px; }
  .repo-markdown :global(h4),
  .repo-markdown :global(h5),
  .repo-markdown :global(h6) { font-size: 13px; color: var(--text-2); }

  .repo-markdown :global(h1),
  .repo-markdown :global(h2) {
    padding-bottom: var(--s-2);
    border-bottom: 1px solid var(--border-dim);
  }

  .repo-markdown :global(*:first-child) { margin-top: 0; }

  .repo-markdown :global(p),
  .repo-markdown :global(ul),
  .repo-markdown :global(ol),
  .repo-markdown :global(blockquote) {
    margin: 0 0 var(--s-4) 0;
  }

  .repo-markdown :global(ul),
  .repo-markdown :global(ol) { padding-left: var(--s-6); }
  .repo-markdown :global(li) { margin-bottom: var(--s-1); }

  .repo-markdown :global(a) {
    color: var(--cyan-400);
    text-decoration: none;
    border-bottom: 1px solid rgba(34, 211, 238, 0.3);
  }

  .repo-markdown :global(a:hover) {
    border-bottom-color: var(--cyan-400);
  }

  .repo-markdown :global(code) {
    font-family: var(--font-mono);
    font-size: 12px;
    background: var(--void-up);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-xs);
    padding: 1px 5px;
    color: var(--text-2);
  }

  .repo-markdown :global(pre) {
    background: var(--void-base);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-md);
    padding: var(--s-4);
    overflow-x: auto;
    margin: 0 0 var(--s-4) 0;
  }

  .repo-markdown :global(pre code) {
    background: transparent;
    border: none;
    padding: 0;
    font-size: 12.5px;
    line-height: 1.6;
  }

  .repo-markdown :global(blockquote) {
    border-left: 2px solid var(--border-strong);
    padding-left: var(--s-4);
    color: var(--text-3);
  }

  .repo-markdown :global(hr) {
    border: none;
    border-top: 1px solid var(--border-dim);
    margin: var(--s-6) 0;
  }

  .repo-markdown :global(table) {
    border-collapse: collapse;
    margin: 0 0 var(--s-4) 0;
    font-size: 12.5px;
    display: block;
    overflow-x: auto;
  }

  .repo-markdown :global(th),
  .repo-markdown :global(td) {
    border: 1px solid var(--border-dim);
    padding: var(--s-2) var(--s-3);
    text-align: left;
  }

  .repo-markdown :global(th) {
    background: var(--void-mid);
    color: var(--text-2);
    font-weight: 600;
  }

  .repo-markdown :global(img) {
    max-width: 100%;
    border-radius: var(--r-sm);
  }

  .repo-markdown :global(input[type='checkbox']) {
    margin-right: var(--s-2);
    accent-color: var(--cyan-500);
  }
</style>
