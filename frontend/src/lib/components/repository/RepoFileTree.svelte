<script>
  import { createEventDispatcher } from 'svelte';
  import { Icons } from '../holo';
  import SignatureBadge from './SignatureBadge.svelte';
  import { baseName, groupByFolder } from './repoFiles.js';

  // entries: [{ name, scid, docType, kind, content, bytes, reason }]
  export let entries = [];
  export let signatures = {};
  export let selected = '';
  export let loading = false;
  export let showSignatures = true;
  export let signaturesLoading = false;

  const dispatch = createEventDispatcher();

  $: groups = groupByFolder(entries);

  // Only icon names the Icons component actually maps. An unmapped name falls
  // back to a circle, which reads as a missing file type rather than a file.
  function iconFor(entry) {
    if (entry?.kind === 'index') return 'package';
    // Neutral on purpose: an entry we cannot parse is not a fault to flag.
    if (entry?.kind === 'unreadable') return 'circle';
    const ext = (entry?.name || '').split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'html': return 'globe';
      case 'js': return 'zap';
      case 'json': return 'code';
      case 'css': return 'layers';
      case 'md': return 'book';
      case 'svg': return 'file-code';
      default: return 'file';
    }
  }
</script>

<div class="repo-tree">
  {#if loading}
    <div class="repo-tree-status">Reading contracts…</div>
  {:else if !entries || entries.length === 0}
    <div class="repo-tree-status">No files</div>
  {:else}
    {#each groups as group (group.dir)}
      {#if group.dir}
        <div class="repo-tree-folder">
          <Icons name="folder" size={12} />
          <span class="repo-tree-folder-name">{group.dir}</span>
        </div>
      {/if}
      <div class="repo-tree-group" class:nested={group.dir}>
        {#each group.files as entry (entry.scid || entry.name)}
          <button
            type="button"
            class="repo-tree-item"
            class:active={selected === entry.name}
            title={entry.name}
            on:click={() => dispatch('select', entry)}
          >
            <span class="repo-tree-icon"><Icons name={iconFor(entry)} size={13} /></span>
            <span class="repo-tree-name">{baseName(entry.name)}</span>
            {#if showSignatures}
              <SignatureBadge
                state={signatures[entry.scid]?.state || ''}
                pending={signaturesLoading && !!entry.scid}
              />
            {/if}
          </button>
        {/each}
      </div>
    {/each}
  {/if}
</div>

<style>
  .repo-tree {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .repo-tree-status {
    padding: var(--s-2) var(--s-3);
    font-size: 12px;
    color: var(--text-4);
  }

  .repo-tree-folder {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3) var(--s-1);
    color: var(--text-4);
    font-size: 11px;
    letter-spacing: 0.06em;
  }

  .repo-tree-folder-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .repo-tree-group.nested {
    padding-left: var(--s-3);
  }

  .repo-tree-item {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    width: 100%;
    padding: var(--s-2) var(--s-3);
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-3);
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    cursor: pointer;
    text-align: left;
    transition: all var(--dur-fast) ease;
  }

  .repo-tree-item:hover {
    background: var(--void-hover);
    color: var(--text-1);
  }

  .repo-tree-item.active {
    background: rgba(34, 211, 238, 0.1);
    color: var(--cyan-400);
  }

  .repo-tree-icon {
    display: flex;
    align-items: center;
    opacity: 0.7;
    flex-shrink: 0;
  }

  .repo-tree-item.active .repo-tree-icon {
    opacity: 1;
  }

  .repo-tree-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
