<script>
  import { createEventDispatcher } from 'svelte';
  import { GetCommitHistory, GetCommitContent, DiffCommits } from '../../../wailsjs/go/main/App.js';
  
  export let scid = '';
  export let show = false;
  
  const dispatch = createEventDispatcher();
  
  let commits = [];
  let loading = true;
  let selectedCommit = null;
  let compareMode = false;
  let compareCommitA = null;
  let compareCommitB = null;
  let diffResult = null;
  
  $: if (scid && show) {
    loadHistory();
  }
  
  async function loadHistory() {
    if (!scid) return;
    
    loading = true;
    try {
      const result = await GetCommitHistory(scid);
      if (result.success) {
        commits = result.commits || [];
      }
    } catch (error) {
      console.error('Failed to load commit history:', error);
    } finally {
      loading = false;
    }
  }
  
  async function viewCommit(commit) {
    if (compareMode) {
      if (!compareCommitA) {
        compareCommitA = commit;
      } else if (!compareCommitB) {
        compareCommitB = commit;
        await runDiff();
      }
    } else {
      selectedCommit = commit;
      try {
        const result = await GetCommitContent(scid, commit.number);
        if (result.success) {
          selectedCommit = { ...commit, content: result.content };
        }
      } catch (error) {
        console.error('Failed to get commit content:', error);
      }
    }
  }
  
  async function runDiff() {
    if (!compareCommitA || !compareCommitB) return;
    
    try {
      const result = await DiffCommits(scid, compareCommitA.number, compareCommitB.number);
      if (result.success) {
        diffResult = result;
      }
    } catch (error) {
      console.error('Diff failed:', error);
    }
  }
  
  function toggleCompareMode() {
    compareMode = !compareMode;
    compareCommitA = null;
    compareCommitB = null;
    diffResult = null;
    selectedCommit = null;
  }
  
  function clearSelection() {
    compareCommitA = null;
    compareCommitB = null;
    diffResult = null;
    selectedCommit = null;
  }
  
  function close() {
    show = false;
    dispatch('close');
  }
  
  function formatHeight(height) {
    if (!height) return 'Unknown';
    return height.toLocaleString();
  }
</script>

{#if show}
  <div class="vh-backdrop" on:click={close}>
    <div class="vh-modal" on:click|stopPropagation>
      <!-- Header -->
      <div class="vh-header">
        <div>
          <h2 class="vh-title">
            <span class="vh-icon">◎</span>
            Version History
          </h2>
          <p class="vh-scid">{scid}</p>
        </div>
        <div class="vh-header-actions">
          <button
            on:click={toggleCompareMode}
            class="btn-compare {compareMode ? 'active' : ''}"
          >
            {compareMode ? '✓ Compare Mode' : 'Compare Versions'}
          </button>
          <button on:click={close} class="btn-close">✕</button>
        </div>
      </div>
      
      <!-- Content -->
      <div class="vh-content">
        <!-- Commit Timeline -->
        <div class="vh-timeline">
          {#if loading}
            <div class="vh-loading">
              <div class="spinner"></div>
              <p class="loading-text">Loading history...</p>
            </div>
          {:else if commits.length === 0}
            <div class="vh-empty">
              <span class="empty-icon">○</span>
              <p>No version history found</p>
            </div>
          {:else}
            <div class="timeline-container">
              <!-- Timeline line -->
              <div class="timeline-line"></div>
              
              {#each commits as commit}
                <button
                  on:click={() => viewCommit(commit)}
                  class="commit-item {(compareMode && (compareCommitA?.number === commit.number || compareCommitB?.number === commit.number)) || selectedCommit?.number === commit.number ? 'active' : ''}"
                >
                  <!-- Dot -->
                  <div class="commit-dot {commit.isCurrent ? 'current' : ''}"></div>
                  
                  <div class="commit-info">
                    <div class="commit-header">
                      <span class="commit-version">v{commit.number}</span>
                      {#if commit.isCurrent}
                        <span class="badge-current">Current</span>
                      {/if}
                    </div>
                    {#if commit.height}
                      <p class="commit-meta">Block {formatHeight(commit.height)}</p>
                    {/if}
                    {#if commit.txid}
                      <p class="commit-txid">{commit.txid.substring(0, 16)}...</p>
                    {/if}
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        </div>
        
        <!-- Detail View -->
        <div class="vh-detail">
          {#if compareMode && diffResult}
            <!-- Diff View -->
            <div class="diff-header">
              <h3 class="diff-title">
                Comparing v{compareCommitA.number} → v{compareCommitB.number}
              </h3>
              <button on:click={clearSelection} class="btn-clear">
                Clear selection
              </button>
            </div>
            
            {#if diffResult.diff && diffResult.diff.length > 0}
              <div class="diff-list">
                {#each diffResult.diff as change}
                  <div class="diff-change {change.type}">
                    <span class="diff-line-num">Line {change.line}:</span>
                    {#if change.type === 'modified'}
                      <div class="diff-old">{change.oldContent}</div>
                      <div class="diff-new">{change.newContent}</div>
                    {:else}
                      <span class="diff-symbol">{change.type === 'added' ? '+' : '-'}</span>
                      {change.content}
                    {/if}
                  </div>
                {/each}
              </div>
            {:else}
              <p class="no-diff">No differences found</p>
            {/if}
          {:else if compareMode}
            <div class="vh-placeholder">
              <span class="placeholder-icon">◈</span>
              <p class="placeholder-text">Select two versions to compare</p>
              <p class="placeholder-hint">
                {#if compareCommitA}
                  Selected: v{compareCommitA.number} → Select another version
                {:else}
                  Click on a version to start
                {/if}
              </p>
            </div>
          {:else if selectedCommit}
            <!-- Single Commit View -->
            <div class="commit-detail">
              <h3 class="detail-title">
                Version {selectedCommit.number}
                {#if selectedCommit.isCurrent}
                  <span class="badge-current">Current</span>
                {/if}
              </h3>
              
              <div class="detail-grid">
                {#if selectedCommit.height}
                  <div class="detail-card">
                    <span class="detail-label">Block Height</span>
                    <span class="detail-value">{formatHeight(selectedCommit.height)}</span>
                  </div>
                {/if}
                {#if selectedCommit.txid}
                  <div class="detail-card">
                    <span class="detail-label">Transaction</span>
                    <span class="detail-value mono">{selectedCommit.txid}</span>
                  </div>
                {/if}
              </div>
              
              {#if selectedCommit.content}
                <div class="content-preview">
                  <pre>{selectedCommit.content}</pre>
                </div>
              {/if}
              
              <!-- Actions -->
              <div class="detail-actions">
                <button
                  class="btn-action"
                  on:click={() => dispatch('revert', selectedCommit)}
                >
                  ← Revert to this version
                </button>
                <button
                  class="btn-action"
                  on:click={() => dispatch('clone', selectedCommit)}
                >
                  ◇ Clone this version
                </button>
              </div>
            </div>
          {:else}
            <div class="vh-placeholder">
              <span class="placeholder-icon">◎</span>
              <p class="placeholder-text">Select a version to view details</p>
            </div>
          {/if}
        </div>
      </div>
    </div>
  </div>
{/if}

<style>
  /* === HOLOGRAM v6.1 Version History === */
  
  .vh-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
    padding: var(--s-4, 16px);
  }
  
  .vh-modal {
    background: var(--void-mid, #12121c);
    border-radius: var(--r-xl, 16px);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    width: 100%;
    max-width: 900px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
  }
  
  /* Header */
  .vh-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4, 16px) var(--s-6, 24px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .vh-title {
    font-family: var(--font-mono);
    font-size: 20px;
    font-weight: 700;
    color: var(--text-1, #f8f8fc);
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .vh-icon {
    color: var(--cyan-400, #22d3ee);
  }
  
  .vh-scid {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 13px;
    color: var(--text-5, #404058);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 400px;
  }
  
  .vh-header-actions {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
  }
  
  .btn-compare {
    padding: var(--s-1, 4px) var(--s-3, 12px);
    border-radius: var(--r-lg, 12px);
    font-size: 13px;
    font-weight: 500;
    background: var(--void-up, #181824);
    color: var(--text-3, #707088);
    border: none;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-compare:hover {
    background: var(--void-surface, #1e1e2a);
  }
  
  .btn-compare.active {
    background: var(--cyan-500, #06b6d4);
    color: var(--void-pure, #000000);
  }
  
  .btn-close {
    font-size: 20px;
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: color 200ms ease-out;
  }
  
  .btn-close:hover {
    color: var(--text-1, #f8f8fc);
  }
  
  /* Content */
  .vh-content {
    flex: 1;
    overflow: hidden;
    display: flex;
  }
  
  /* Timeline */
  .vh-timeline {
    width: 288px;
    border-right: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    overflow-y: auto;
    padding: var(--s-4, 16px);
  }
  
  .vh-loading,
  .vh-empty {
    text-align: center;
    padding: var(--s-8, 32px);
    color: var(--text-4, #505068);
  }
  
  .spinner {
    width: 32px;
    height: 32px;
    border: 2px solid var(--cyan-500, #06b6d4);
    border-top-color: transparent;
    border-radius: var(--r-full, 9999px);
    animation: spin 0.6s linear infinite;
    margin: 0 auto var(--s-2, 8px);
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  .empty-icon {
    font-size: 28px;
    display: block;
    margin-bottom: var(--s-2, 8px);
  }
  
  .timeline-container {
    position: relative;
  }
  
  .timeline-line {
    position: absolute;
    left: 16px;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--void-surface, #1e1e2a);
  }
  
  .commit-item {
    position: relative;
    width: 100%;
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
    padding: var(--s-3, 12px);
    border-radius: var(--r-lg, 12px);
    text-align: left;
    background: transparent;
    border: 1px solid transparent;
    cursor: pointer;
    transition: all 200ms ease-out;
    margin-bottom: var(--s-2, 8px);
  }
  
  .commit-item:hover {
    background: var(--void-up, #181824);
  }
  
  .commit-item.active {
    background: rgba(6, 182, 212, 0.1);
    border-color: rgba(6, 182, 212, 0.3);
  }
  
  .commit-dot {
    position: relative;
    z-index: 10;
    width: 12px;
    height: 12px;
    border-radius: var(--r-full, 9999px);
    margin-top: 4px;
    flex-shrink: 0;
    background: var(--void-hover, #262634);
  }
  
  .commit-dot.current {
    background: var(--status-ok, #34d399);
  }
  
  .commit-info {
    flex: 1;
    min-width: 0;
  }
  
  .commit-header {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .commit-version {
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .badge-current {
    padding: 2px 6px;
    font-size: 10px;
    background: rgba(52, 211, 153, 0.2);
    color: var(--status-ok, #34d399);
    border-radius: var(--r-xs, 3px);
  }
  
  .commit-meta,
  .commit-txid {
    font-size: 12px;
    color: var(--text-5, #404058);
  }
  
  .commit-txid {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  /* Detail View */
  .vh-detail {
    flex: 1;
    overflow-y: auto;
    padding: var(--s-6, 24px);
  }
  
  .vh-placeholder {
    text-align: center;
    padding: var(--s-16, 64px);
    color: var(--text-4, #505068);
  }
  
  .placeholder-icon {
    font-size: 40px;
    display: block;
    margin-bottom: var(--s-4, 16px);
    color: var(--text-5, #404058);
  }
  
  .placeholder-text {
    margin-bottom: var(--s-2, 8px);
  }
  
  .placeholder-hint {
    font-size: 13px;
    color: var(--text-5, #404058);
  }
  
  /* Diff View */
  .diff-header {
    margin-bottom: var(--s-4, 16px);
  }
  
  .diff-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-2, #a8a8b8);
    margin-bottom: var(--s-2, 8px);
  }
  
  .btn-clear {
    font-size: 13px;
    color: var(--cyan-400, #22d3ee);
    background: transparent;
    border: none;
    cursor: pointer;
    padding: 0;
    transition: color 200ms ease-out;
  }
  
  .btn-clear:hover {
    color: var(--cyan-300, #67e8f9);
  }
  
  .diff-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .diff-change {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 13px;
    padding: var(--s-2, 8px);
    border-radius: var(--r-sm, 5px);
    border-left: 2px solid;
  }
  
  .diff-change.added {
    background: rgba(52, 211, 153, 0.1);
    border-color: var(--status-ok, #34d399);
    color: var(--status-ok, #34d399);
  }
  
  .diff-change.removed {
    background: rgba(248, 113, 113, 0.1);
    border-color: var(--status-err, #f87171);
    color: var(--status-err, #f87171);
  }
  
  .diff-change.modified {
    background: rgba(251, 191, 36, 0.1);
    border-color: var(--status-warn, #fbbf24);
    color: var(--status-warn, #fbbf24);
  }
  
  .diff-line-num {
    color: var(--text-5, #404058);
    margin-right: var(--s-2, 8px);
  }
  
  .diff-old {
    color: var(--status-err, #f87171);
    text-decoration: line-through;
  }
  
  .diff-new {
    color: var(--status-ok, #34d399);
  }
  
  .diff-symbol {
    margin-right: var(--s-1, 4px);
  }
  
  .no-diff {
    text-align: center;
    padding: var(--s-8, 32px);
    color: var(--text-4, #505068);
  }
  
  /* Commit Detail */
  .commit-detail {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .detail-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-2, #a8a8b8);
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .detail-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4, 16px);
  }
  
  .detail-card {
    padding: var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
  }
  
  .detail-label {
    font-size: 12px;
    color: var(--text-5, #404058);
    display: block;
    margin-bottom: var(--s-1, 4px);
  }
  
  .detail-value {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-2, #a8a8b8);
  }
  
  .detail-value.mono {
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }
  
  .content-preview {
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
    padding: var(--s-4, 16px);
    overflow-x: auto;
  }
  
  .content-preview pre {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 13px;
    color: var(--text-3, #707088);
    margin: 0;
  }
  
  .detail-actions {
    display: flex;
    gap: var(--s-3, 12px);
    margin-top: var(--s-2, 8px);
  }
  
  .btn-action {
    padding: var(--s-2, 8px) var(--s-4, 16px);
    background: var(--void-up, #181824);
    color: var(--text-3, #707088);
    border-radius: var(--r-lg, 12px);
    border: none;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-action:hover {
    background: var(--void-surface, #1e1e2a);
    color: var(--text-2, #a8a8b8);
  }
</style>

