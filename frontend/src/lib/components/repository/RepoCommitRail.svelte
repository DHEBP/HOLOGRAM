<script>
  import { createEventDispatcher } from 'svelte';

  export let commits = [];
  export let loading = false;
  export let selected = null;      // commit number being viewed, or null for latest
  export let compareMode = false;
  export let compareA = null;
  export let compareB = null;

  const dispatch = createEventDispatcher();

  function isPicked(commit) {
    if (compareMode) {
      return compareA?.number === commit.number || compareB?.number === commit.number;
    }
    return selected === commit.number;
  }

  function formatHeight(height) {
    if (!height) return '';
    return height.toLocaleString();
  }
</script>

<div class="repo-rail">
  {#if loading}
    <div class="repo-rail-status">Reading history…</div>
  {:else if !commits || commits.length === 0}
    <div class="repo-rail-status">No commits indexed</div>
  {:else}
    {#each [...commits].reverse() as commit (commit.number)}
      <button
        type="button"
        class="repo-rail-item"
        class:active={isPicked(commit)}
        on:click={() => dispatch('select', commit)}
      >
        <span class="repo-rail-dot" class:current={commit.isCurrent}></span>
        <span class="repo-rail-body">
          <span class="repo-rail-line">
            <span class="repo-rail-version">v{commit.number}</span>
            {#if commit.isCurrent}
              <span class="repo-rail-latest">latest</span>
            {/if}
            {#if compareMode && compareA?.number === commit.number}
              <span class="repo-rail-pick">from</span>
            {:else if compareMode && compareB?.number === commit.number}
              <span class="repo-rail-pick">to</span>
            {/if}
          </span>
          {#if commit.label}
            <span class="repo-rail-label">{commit.label}</span>
          {/if}
          {#if commit.height}
            <span class="repo-rail-meta">block {formatHeight(commit.height)}</span>
          {/if}
        </span>
      </button>
    {/each}
  {/if}
</div>

<style>
  .repo-rail {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .repo-rail-status {
    padding: var(--s-2) var(--s-3);
    font-size: 12px;
    color: var(--text-4);
  }

  .repo-rail-item {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
    width: 100%;
    padding: var(--s-2) var(--s-3);
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    cursor: pointer;
    text-align: left;
    transition: all var(--dur-fast) ease;
  }

  .repo-rail-item:hover {
    background: var(--void-hover);
  }

  .repo-rail-item.active {
    background: rgba(34, 211, 238, 0.1);
  }

  .repo-rail-dot {
    width: 7px;
    height: 7px;
    border-radius: var(--r-full);
    background: var(--void-hover);
    margin-top: 5px;
    flex-shrink: 0;
  }

  .repo-rail-dot.current {
    background: var(--status-ok);
  }

  .repo-rail-item.active .repo-rail-dot {
    background: var(--cyan-400);
  }

  .repo-rail-body {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
    flex: 1;
  }

  .repo-rail-line {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }

  .repo-rail-version {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
  }

  .repo-rail-item.active .repo-rail-version {
    color: var(--cyan-400);
  }

  .repo-rail-latest {
    font-family: var(--font-mono);
    font-size: 9px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--status-ok);
  }

  .repo-rail-pick {
    font-family: var(--font-mono);
    font-size: 9px;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--cyan-400);
  }

  .repo-rail-label {
    font-size: 11px;
    color: var(--text-4);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-rail-meta {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-5);
  }
</style>
