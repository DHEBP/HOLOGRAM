<script>
  import { Icons } from '../holo';

  // result: the map DiffCommits returns — { summary, fileDiffs[], diff[] }
  export let result = null;
  export let fromLabel = '';
  export let toLabel = '';
  export let loading = false;

  // The line list is flat and uncapped upstream, so a whole-file rewrite of a
  // large DOC would render thousands of rows. Cap the DOM and say so, rather
  // than freeze the UI thread.
  const MAX_LINES_SHOWN = 400;

  function statusClass(status) {
    switch (status) {
      case 'added': return 'is-added';
      case 'removed': return 'is-removed';
      case 'modified': return 'is-modified';
      default: return '';
    }
  }

  function iconFor(filename) {
    const ext = (filename || '').split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'html': return 'globe';
      case 'js': return 'zap';
      case 'json': return 'code';
      case 'css': return 'layers';
      case 'md': return 'book';
      default: return 'file';
    }
  }
</script>

<div class="repo-diff">
  <div class="repo-diff-header">
    <h3 class="repo-diff-title">
      {fromLabel} <span class="repo-diff-arrow">→</span> {toLabel}
    </h3>
    {#if result?.summary}
      <span class="repo-diff-summary">{result.summary}</span>
    {/if}
  </div>

  {#if loading}
    <div class="repo-diff-status">
      <span class="repo-diff-spinner"></span>
      <span>Reconstructing both versions from chain…</span>
    </div>
  {:else if !result}
    <div class="repo-diff-status">Pick two versions to compare.</div>
  {:else if result.fileDiffs && result.fileDiffs.length > 0}
    {#each result.fileDiffs as fileDiff (fileDiff.fileName)}
      <div class="repo-diff-file">
        <div class="repo-diff-file-header {statusClass(fileDiff.status)}">
          <span class="repo-diff-file-icon"><Icons name={iconFor(fileDiff.fileName)} size={13} /></span>
          <span class="repo-diff-file-name">{fileDiff.fileName}</span>
          <span class="repo-diff-file-status {statusClass(fileDiff.status)}">{fileDiff.status}</span>
        </div>

        {#if fileDiff.lineDiffs && fileDiff.lineDiffs.length > 0}
          <div class="repo-diff-lines">
            {#each fileDiff.lineDiffs.slice(0, MAX_LINES_SHOWN) as change}
              {#if change.type === 'modified'}
                <div class="repo-diff-row removed">
                  <span class="repo-diff-num">{change.oldLine || change.line || ''}</span>
                  <span class="repo-diff-sign">-</span>
                  <span class="repo-diff-text">{change.oldContent}</span>
                </div>
                <div class="repo-diff-row added">
                  <span class="repo-diff-num">{change.newLine || change.line || ''}</span>
                  <span class="repo-diff-sign">+</span>
                  <span class="repo-diff-text">{change.newContent}</span>
                </div>
              {:else}
                <div class="repo-diff-row {change.type}">
                  <span class="repo-diff-num">{change.type === 'added' ? (change.newLine || change.line || '') : (change.oldLine || change.line || '')}</span>
                  <span class="repo-diff-sign">{change.type === 'added' ? '+' : '-'}</span>
                  <span class="repo-diff-text">{change.content}</span>
                </div>
              {/if}
            {/each}
            {#if fileDiff.lineDiffs.length > MAX_LINES_SHOWN}
              <div class="repo-diff-truncated">
                {fileDiff.lineDiffs.length - MAX_LINES_SHOWN} more changed line{fileDiff.lineDiffs.length - MAX_LINES_SHOWN !== 1 ? 's' : ''} not shown
              </div>
            {/if}
          </div>
        {:else}
          <p class="repo-diff-none">
            {#if fileDiff.status === 'added'}
              New file
            {:else if fileDiff.status === 'removed'}
              File removed
            {:else}
              No line changes
            {/if}
          </p>
        {/if}
      </div>
    {/each}
  {:else}
    <div class="repo-diff-status">No differences between these versions.</div>
  {/if}
</div>

<style>
  .repo-diff {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }

  .repo-diff-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
  }

  .repo-diff-title {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    letter-spacing: 0.06em;
    color: var(--text-1);
    margin: 0;
  }

  .repo-diff-arrow {
    color: var(--text-4);
  }

  .repo-diff-summary {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-3);
    padding: 2px var(--s-3);
    background: var(--void-up);
    border-radius: var(--r-sm);
    white-space: nowrap;
  }

  .repo-diff-status {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-8);
    justify-content: center;
    color: var(--text-4);
    font-size: 13px;
  }

  .repo-diff-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid var(--cyan-500);
    border-top-color: transparent;
    border-radius: var(--r-full);
    animation: repo-diff-spin 0.6s linear infinite;
  }

  @keyframes repo-diff-spin {
    to { transform: rotate(360deg); }
  }

  .repo-diff-file {
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-md);
    overflow: hidden;
  }

  .repo-diff-file-header {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    background: var(--void-mid);
    border-bottom: 1px solid var(--border-dim);
  }

  .repo-diff-file-icon {
    display: flex;
    align-items: center;
    color: var(--text-4);
  }

  .repo-diff-file-name {
    flex: 1;
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-diff-file-status {
    font-family: var(--font-mono);
    font-size: 9px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    padding: 1px var(--s-2);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    color: var(--text-4);
  }

  .repo-diff-file-status.is-added { color: var(--status-ok); border-color: var(--status-ok); }
  .repo-diff-file-status.is-removed { color: var(--status-err); border-color: var(--status-err); }
  .repo-diff-file-status.is-modified { color: var(--status-warn); border-color: var(--status-warn); }

  .repo-diff-lines {
    max-height: 420px;
    overflow: auto;
    padding: var(--s-2) 0;
  }

  .repo-diff-row {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    padding: 1px var(--s-3);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.55;
  }

  .repo-diff-row.added {
    background: rgba(52, 211, 153, 0.08);
    color: var(--status-ok);
  }

  .repo-diff-row.removed {
    background: rgba(248, 113, 113, 0.08);
    color: var(--status-err);
  }

  .repo-diff-num {
    width: 44px;
    flex-shrink: 0;
    text-align: right;
    color: var(--text-5);
    user-select: none;
  }

  .repo-diff-sign {
    width: 10px;
    flex-shrink: 0;
    user-select: none;
  }

  .repo-diff-text {
    flex: 1;
    min-width: 0;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .repo-diff-truncated {
    padding: var(--s-3);
    text-align: center;
    font-size: 11px;
    color: var(--text-4);
    border-top: 1px solid var(--border-dim);
  }

  .repo-diff-none {
    padding: var(--s-3);
    margin: 0;
    text-align: center;
    font-size: 12px;
    font-style: italic;
    color: var(--text-5);
  }
</style>
