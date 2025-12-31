<script>
  import { createEventDispatcher, onMount } from 'svelte';
  
  const dispatch = createEventDispatcher();
  
  export let isOpen = false;
  
  let searches = [];
  let pinnedSearches = [];
  let filter = ''; // Filter by query
  
  const STORAGE_KEY = 'recentSearches';
  const PINNED_KEY = 'pinnedSearches';
  const MAX_HISTORY = 50;
  
  onMount(() => {
    loadSearches();
  });
  
  function loadSearches() {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      searches = stored ? JSON.parse(stored).slice(0, MAX_HISTORY) : [];
      
      const pinned = localStorage.getItem(PINNED_KEY);
      pinnedSearches = pinned ? JSON.parse(pinned) : [];
    } catch (e) {
      searches = [];
      pinnedSearches = [];
    }
  }
  
  function saveSearches() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(searches));
      localStorage.setItem(PINNED_KEY, JSON.stringify(pinnedSearches));
    } catch (e) {
      // Ignore storage errors
    }
  }
  
  function isPinned(query) {
    return pinnedSearches.includes(query);
  }
  
  function togglePin(search) {
    if (isPinned(search.query)) {
      pinnedSearches = pinnedSearches.filter(q => q !== search.query);
    } else {
      pinnedSearches = [search.query, ...pinnedSearches];
    }
    saveSearches();
  }
  
  function removeSearch(search) {
    searches = searches.filter(s => s.query !== search.query);
    pinnedSearches = pinnedSearches.filter(q => q !== search.query);
    saveSearches();
  }
  
  function clearAll() {
    searches = [];
    // Keep pinned items
    saveSearches();
  }
  
  function clearUnpinned() {
    searches = searches.filter(s => isPinned(s.query));
    saveSearches();
  }
  
  function selectSearch(search) {
    dispatch('select', search);
    close();
  }
  
  function close() {
    isOpen = false;
    dispatch('close');
  }
  
  function getTypeIcon(type) {
    switch (type) {
      case 'block': return 'B';
      case 'tx': return '💸';
      case 'scid': return '📜';
      case 'hash': return '#';
      case 'durl': return '@';
      case 'address': return 'A';
      default: return '?';
    }
  }
  
  function formatTimestamp(ts) {
    if (!ts) return '';
    const date = new Date(ts);
    const now = new Date();
    const diff = now - date;
    
    if (diff < 60000) return 'Just now';
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
    if (diff < 604800000) return `${Math.floor(diff / 86400000)}d ago`;
    return date.toLocaleDateString();
  }
  
  function truncateQuery(query, maxLen = 40) {
    if (query.length <= maxLen) return query;
    return query.slice(0, maxLen / 2 - 2) + '...' + query.slice(-(maxLen / 2 - 2));
  }
  
  // Filtered and sorted searches
  $: filteredSearches = searches
    .filter(s => !filter || s.query.toLowerCase().includes(filter.toLowerCase()))
    .sort((a, b) => {
      // Pinned items first
      const aPinned = isPinned(a.query);
      const bPinned = isPinned(b.query);
      if (aPinned && !bPinned) return -1;
      if (!aPinned && bPinned) return 1;
      // Then by timestamp
      return (b.timestamp || 0) - (a.timestamp || 0);
    });
  
  $: pinnedCount = searches.filter(s => isPinned(s.query)).length;
  $: unpinnedCount = searches.length - pinnedCount;
</script>

{#if isOpen}
  <div class="overlay" on:click={close}>
    <div class="modal" on:click|stopPropagation>
      <div class="modal-header">
        <h2>Search History</h2>
        <button class="close-btn" on:click={close}>×</button>
      </div>
      
      <div class="modal-toolbar">
        <div class="filter-input">
          <span class="filter-icon"><Search size={12} /></span>
          <input 
            type="text" 
            placeholder="Filter searches..."
            bind:value={filter}
          />
        </div>
        <div class="toolbar-actions">
          {#if unpinnedCount > 0}
            <button class="action-btn" on:click={clearUnpinned} title="Clear unpinned">
              Clear Unpinned
            </button>
          {/if}
          {#if searches.length > 0}
            <button class="action-btn danger" on:click={clearAll} title="Clear all history">
              Clear All
            </button>
          {/if}
        </div>
      </div>
      
      <div class="modal-body">
        {#if filteredSearches.length === 0}
          <div class="empty-state">
            <span class="empty-icon">📭</span>
            <p>{filter ? 'No matching searches found' : 'No search history yet'}</p>
            <p class="empty-hint">Your searches will appear here</p>
          </div>
        {:else}
          <div class="search-list">
            {#each filteredSearches as search}
              <div class="search-item" class:pinned={isPinned(search.query)}>
                <button class="pin-btn" on:click={() => togglePin(search)} title={isPinned(search.query) ? 'Unpin' : 'Pin'}>
                  {isPinned(search.query) ? '📌' : '📍'}
                </button>
                
                <button class="search-content" on:click={() => selectSearch(search)}>
                  <span class="search-icon">{getTypeIcon(search.type)}</span>
                  <span class="search-query">{truncateQuery(search.query)}</span>
                  <span class="search-time">{formatTimestamp(search.timestamp)}</span>
                </button>
                
                <button class="remove-btn" on:click={() => removeSearch(search)} title="Remove">
                  ×
                </button>
              </div>
            {/each}
          </div>
        {/if}
      </div>
      
      <div class="modal-footer">
        <span class="stats">
          {searches.length} searches • {pinnedCount} pinned
        </span>
        <button class="btn-primary" on:click={close}>Done</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    animation: overlay-fade 0.2s ease;
  }
  
  @keyframes overlay-fade {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  
  .modal {
    background: linear-gradient(135deg, rgba(30, 30, 40, 0.98) 0%, rgba(20, 20, 30, 0.98) 100%);
    border: 1px solid rgba(82, 200, 219, 0.3);
    border-radius: 16px;
    width: 90%;
    max-width: 600px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
    animation: modal-appear 0.2s ease;
  }
  
  @keyframes modal-appear {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(-10px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }
  
  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1.25rem 1.5rem;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  }
  
  .modal-header h2 {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 600;
    color: #fff;
  }
  
  .close-btn {
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    color: rgba(255, 255, 255, 0.6);
    font-size: 1.25rem;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .close-btn:hover {
    background: rgba(239, 68, 68, 0.2);
    border-color: rgba(239, 68, 68, 0.4);
    color: #ef4444;
  }
  
  .modal-toolbar {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1rem 1.5rem;
    background: rgba(0, 0, 0, 0.2);
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  }
  
  .filter-input {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.75rem;
    background: rgba(0, 0, 0, 0.3);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
  }
  
  .filter-icon {
    font-size: 0.85rem;
    opacity: 0.5;
  }
  
  .filter-input input {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    color: #fff;
    font-size: 0.9rem;
  }
  
  .filter-input input::placeholder {
    color: rgba(255, 255, 255, 0.3);
  }
  
  .toolbar-actions {
    display: flex;
    gap: 0.5rem;
  }
  
  .action-btn {
    padding: 0.5rem 0.75rem;
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 6px;
    color: rgba(255, 255, 255, 0.6);
    font-size: 0.75rem;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .action-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    color: #fff;
  }
  
  .action-btn.danger:hover {
    background: rgba(239, 68, 68, 0.15);
    border-color: rgba(239, 68, 68, 0.3);
    color: #ef4444;
  }
  
  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 1rem;
  }
  
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 3rem 1rem;
    text-align: center;
  }
  
  .empty-icon {
    font-size: 3rem;
    margin-bottom: 1rem;
    opacity: 0.6;
  }
  
  .empty-state p {
    margin: 0;
    color: rgba(255, 255, 255, 0.5);
  }
  
  .empty-hint {
    font-size: 0.85rem;
    margin-top: 0.5rem !important;
    color: rgba(255, 255, 255, 0.3) !important;
  }
  
  .search-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }
  
  .search-item {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem;
    background: rgba(0, 0, 0, 0.2);
    border: 1px solid rgba(255, 255, 255, 0.06);
    border-radius: 10px;
    transition: all 0.15s ease;
  }
  
  .search-item:hover {
    background: rgba(0, 0, 0, 0.3);
    border-color: rgba(255, 255, 255, 0.1);
  }
  
  .search-item.pinned {
    background: rgba(251, 191, 36, 0.05);
    border-color: rgba(251, 191, 36, 0.2);
  }
  
  .pin-btn, .remove-btn {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.15s ease;
    flex-shrink: 0;
  }
  
  .pin-btn {
    font-size: 0.85rem;
    opacity: 0.5;
  }
  
  .pin-btn:hover {
    background: rgba(251, 191, 36, 0.1);
    opacity: 1;
  }
  
  .search-item.pinned .pin-btn {
    opacity: 1;
  }
  
  .remove-btn {
    font-size: 1.1rem;
    color: rgba(255, 255, 255, 0.3);
  }
  
  .remove-btn:hover {
    background: rgba(239, 68, 68, 0.15);
    color: #ef4444;
  }
  
  .search-content {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem;
    background: transparent;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.15s ease;
    text-align: left;
    min-width: 0;
  }
  
  .search-content:hover {
    background: rgba(82, 200, 219, 0.1);
  }
  
  .search-icon {
    font-size: 1rem;
    flex-shrink: 0;
  }
  
  .search-query {
    flex: 1;
    font-size: 0.9rem;
    color: #fff;
    font-family: monospace;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .search-time {
    font-size: 0.7rem;
    color: rgba(255, 255, 255, 0.3);
    flex-shrink: 0;
  }
  
  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 1rem 1.5rem;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(0, 0, 0, 0.2);
  }
  
  .stats {
    font-size: 0.8rem;
    color: rgba(255, 255, 255, 0.4);
  }
  
  .btn-primary {
    padding: 0.6rem 1.25rem;
    background: linear-gradient(135deg, #52c8db 0%, #3ba8bc 100%);
    border: none;
    border-radius: 8px;
    color: #000;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
  }
  
  .btn-primary:hover {
    background: linear-gradient(135deg, #6dd4e5 0%, #4bb8cc 100%);
    transform: translateY(-1px);
  }
  
  /* Scrollbar */
  .modal-body::-webkit-scrollbar {
    width: 6px;
  }
  
  .modal-body::-webkit-scrollbar-track {
    background: rgba(0, 0, 0, 0.2);
    border-radius: 3px;
  }
  
  .modal-body::-webkit-scrollbar-thumb {
    background: rgba(82, 200, 219, 0.3);
    border-radius: 3px;
  }
  
  .modal-body::-webkit-scrollbar-thumb:hover {
    background: rgba(82, 200, 219, 0.5);
  }
</style>

