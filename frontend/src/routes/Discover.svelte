<script>
  import { onMount } from 'svelte';
  import { appState, navigateTo, updateStatus, setAppDiscoveryState } from '../lib/stores/appState.js';
  import { GetDiscoveredApps, SearchApps, GetAppRating, StartGnomon, SetGnomonAutostart } from '../../wailsjs/go/main/App.js';
  import deroIconFallback from '../assets/dero-icon-fallback.svg';
  
  let apps = [];
  let filteredApps = [];
  let loading = true;
  let searchQuery = '';
  let selectedCategory = 'all';
  let sortBy = 'recent';
  
  // Category filter options based on rating
  const categories = [
    { id: 'all', label: 'All Apps', icon: '◎' },
    { id: 'epoch', label: 'EPOCH Apps', icon: '◈' },
    { id: 'top', label: 'Top Rated (7+)', icon: '★' },
    { id: 'good', label: 'Good (5+)', icon: '◇' },
    { id: 'unrated', label: 'Unrated', icon: '○' },
  ];
  
  // Minimum rating filter (0-10 scale)
  let minRating = 0;
  
  // Gnomon auto-start preference
  let enableAutostart = false;
  
  // Track failed icon URLs to show fallback
  let failedIcons = new Set();
  
  // Handle icon load error - mark as failed and trigger re-render
  function handleIconError(iconUrl) {
    failedIcons.add(iconUrl);
    failedIcons = failedIcons; // Trigger Svelte reactivity
  }
  
  // Check if icon should be shown (exists and hasn't failed)
  function shouldShowIcon(iconUrl) {
    return iconUrl && !failedIcons.has(iconUrl);
  }
  
  onMount(async () => {
    await loadApps();
  });
  
  async function loadApps() {
    // Skip if Gnomon isn't running yet
    if (!$appState.gnomonRunning) {
      loading = false;
      setAppDiscoveryState({ loading: false });
      return;
    }
    
    loading = true;
    setAppDiscoveryState({ loading: true });
    try {
      const result = await GetDiscoveredApps();
      if (result.success && result.apps) {
        apps = result.apps;
        // Sort by default (recent)
        applyFilters();
      }
    } catch (error) {
      console.error('Failed to load apps:', error);
    } finally {
      loading = false;
      setAppDiscoveryState({ loading: false, loaded: true });
    }
  }
  
  // Track last indexed height for reactive reload
  let lastIndexedHeight = 0;
  
  // Reactive: reload apps when Gnomon starts running (if no apps loaded yet)
  $: if ($appState.gnomonRunning && apps.length === 0 && !loading) {
    loadApps();
  }

  // Reset discovery state when Gnomon stops
  $: if (!$appState.gnomonRunning && apps.length > 0) {
    apps = [];
    filteredApps = [];
    setAppDiscoveryState({ loading: false, loaded: false });
  }
  
  // Reactive: reload apps when Gnomon syncs more blocks (finds new apps)
  // Reload when indexed height increases by at least 1000 blocks
  $: if ($appState.gnomonRunning && $appState.gnomonIndexedHeight > lastIndexedHeight + 1000 && !loading) {
    lastIndexedHeight = $appState.gnomonIndexedHeight;
    loadApps();
  }
  
  function applyFilters() {
    let result = [...apps];
    
    // Apply search filter
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(app => {
        const name = (app.display_name || app.name || '').toLowerCase();
        const desc = (app.description || '').toLowerCase();
        const durl = (app.durl || '').toLowerCase();
        return name.includes(q) || desc.includes(q) || durl.includes(q);
      });
    }
    
    // Apply category filter
    switch (selectedCategory) {
      case 'epoch':
        // Filter for EPOCH-supporting apps only
        result = result.filter(app => app.supports_epoch === true);
        break;
      case 'top':
        result = result.filter(app => app.rating && app.rating.average >= 7);
        break;
      case 'good':
        result = result.filter(app => app.rating && app.rating.average >= 5);
        break;
      case 'unrated':
        result = result.filter(app => !app.rating || app.rating.count === 0);
        break;
      // 'all' shows everything
    }
    
    // Apply minimum rating filter (slider)
    if (minRating > 0) {
      result = result.filter(app => {
        if (!app.rating || app.rating.count === 0) return minRating === 0;
        return app.rating.average >= minRating;
      });
    }
    
    // Apply sorting
    if (sortBy === 'rating') {
      result.sort((a, b) => (b.rating?.average || 0) - (a.rating?.average || 0));
    } else if (sortBy === 'name') {
      result.sort((a, b) => (a.display_name || a.name || '').localeCompare(b.display_name || b.name || ''));
    }
    // 'recent' keeps default order (by interaction height)
    
    filteredApps = result;
  }
  
  function handleSearch() {
    applyFilters();
  }
  
  function handleCategoryChange(categoryId) {
    selectedCategory = categoryId;
    applyFilters();
  }
  
  function getRatingColor(avg) {
    if (!avg || avg === 0) return 'rating-none';
    if (avg >= 8) return 'rating-excellent';
    if (avg >= 7) return 'rating-great';
    if (avg >= 5) return 'rating-good';
    if (avg >= 3) return 'rating-fair';
    return 'rating-poor';
  }
  
  function navigateToApp(app) {
    // Set pending navigation and switch to browser tab
    const url = app.durl ? `dero://${app.durl}` : app.scid;
    navigateTo(url, app);
    // Switch to browser tab
    window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'browser' }));
  }
  
  async function startIndexer() {
    try {
      // Save auto-start preference if checkbox is checked
      if (enableAutostart) {
        await SetGnomonAutostart(true);
      }
      
      await StartGnomon();
      // Update status immediately so $appState.gnomonRunning becomes true
      await updateStatus();
      // Give Gnomon a moment to initialize
      setTimeout(() => loadApps(), 500);
    } catch (err) {
      console.error('Failed to start Gnomon:', err);
    }
  }
</script>

<div class="discover-page">
  <!-- Header -->
  <div class="discover-header">
    <div class="header-content">
      <h1 class="page-title">Discover TELA Apps</h1>
      <p class="page-desc">Browse decentralized applications on the DERO blockchain</p>
      
      <!-- Search Bar -->
      <div class="search-row">
        <div class="search-input-wrap">
          <input
            type="text"
            bind:value={searchQuery}
            on:input={handleSearch}
            placeholder="Search apps by name, description, or dURL..."
            class="search-input"
          />
          <span class="search-icon">⌕</span>
        </div>
        <select
          bind:value={sortBy}
          on:change={applyFilters}
          class="sort-select"
        >
          <option value="recent">Most Recent</option>
          <option value="rating">Highest Rated</option>
          <option value="name">Alphabetical</option>
        </select>
      </div>
      
      <!-- Rating Filter Slider -->
      <div class="rating-filter">
        <span class="filter-label">Min Rating:</span>
        <input 
          type="range" 
          min="0" 
          max="9" 
          step="1" 
          bind:value={minRating}
          on:input={applyFilters}
          class="rating-slider"
        />
        <span class="rating-value {minRating >= 7 ? 'high' : minRating >= 5 ? 'good' : minRating >= 3 ? 'fair' : ''}">
          {minRating === 0 ? 'Any' : `${minRating}+`}
        </span>
      </div>
    </div>
  </div>
  
  <!-- Category Tabs -->
  <div class="category-bar">
    <div class="category-tabs">
      {#each categories as cat}
        <button
          on:click={() => handleCategoryChange(cat.id)}
          class="category-tab {selectedCategory === cat.id ? 'active' : ''}"
        >
          <span class="tab-icon">{cat.icon}</span>
          <span>{cat.label}</span>
        </button>
      {/each}
    </div>
  </div>
  
  <!-- Content -->
  <div class="discover-content">
    <div class="content-inner">
      {#if !$appState.gnomonRunning}
        <!-- Gnomon not running -->
        <div class="empty-state">
          <span class="empty-icon">◎</span>
          <h2 class="empty-title">Gnomon Indexer Not Running</h2>
          <p class="empty-desc">Start the Gnomon indexer to discover TELA applications</p>
          <button on:click={startIndexer} class="btn-start">
            Start Gnomon Indexer
          </button>
          <label class="gnomon-autostart-option">
            <input type="checkbox" bind:checked={enableAutostart} />
            <span>Always start automatically</span>
          </label>
        </div>
      {:else if $appState.gnomonProgress < 95 && filteredApps.length === 0}
        <!-- Gnomon is syncing -->
        <div class="empty-state">
          <div class="spinner"></div>
          <h2 class="empty-title">Syncing Blockchain Index</h2>
          <p class="empty-desc">
            Indexing block {$appState.gnomonIndexedHeight.toLocaleString()} of {$appState.chainHeight.toLocaleString()}
          </p>
          <div class="gnomon-sync-progress">
            <div class="gnomon-sync-bar">
              <div class="gnomon-sync-fill" style="width: {$appState.gnomonProgress}%"></div>
            </div>
            <span class="gnomon-sync-percent">{$appState.gnomonProgress.toFixed(1)}%</span>
          </div>
          <p class="empty-hint">Apps will appear as they are discovered...</p>
        </div>
      {:else if loading}
        <!-- Loading -->
        <div class="loading-state">
          <div class="spinner"></div>
          <p class="loading-text">Loading apps from blockchain index...</p>
        </div>
      {:else if filteredApps.length === 0}
        <!-- No results -->
        <div class="empty-state">
          <span class="empty-icon">⌕</span>
          <h2 class="empty-title">No Apps Found</h2>
          <p class="empty-desc">
            {searchQuery ? 'Try a different search term' : 'No TELA apps indexed yet'}
          </p>
        </div>
      {:else}
        <!-- App Grid -->
        <div class="app-grid">
          {#each filteredApps as app}
            <button
              on:click={() => navigateToApp(app)}
              class="app-card"
            >
              <!-- Icon & Name -->
              <div class="app-header">
                <div class="app-icon-wrap">
                  {#if shouldShowIcon(app.icon)}
                    <img 
                      src={app.icon} 
                      alt="" 
                      class="app-icon-img" 
                      on:error={() => handleIconError(app.icon)}
                    />
                  {:else}
                    <img src={deroIconFallback} alt="" class="app-icon-img app-icon-fallback" />
                  {/if}
                </div>
                <div class="app-info">
                  <h3 class="app-name">
                    {app.display_name || app.name || 'Unnamed App'}
                  </h3>
                  {#if app.durl}
                    <p class="app-durl">dero://{app.durl}</p>
                  {:else}
                    <p class="app-scid">{app.scid?.substring(0, 16)}...</p>
                  {/if}
                </div>
              </div>
              
              <!-- Description -->
              <p class="app-desc">
                {app.description || 'No description available'}
              </p>
              
              <!-- Footer: Rating + EPOCH Badge -->
              <div class="app-footer">
                <div class="app-badges">
                  {#if app.supports_epoch}
                    <span class="epoch-badge" title="Supports EPOCH Developer Ecosystem">◈</span>
                  {/if}
                  {#if app.rating && app.rating.count > 0}
                    <span class="app-rating {getRatingColor(app.rating.average)}">
                      ★ {app.rating.average.toFixed(1)}/10
                    </span>
                  {:else}
                    <span class="no-rating">No ratings</span>
                  {/if}
                </div>
                {#if app.rating && app.rating.count > 0}
                  <span class="rating-count">{app.rating.count} rating{app.rating.count > 1 ? 's' : ''}</span>
                {/if}
              </div>
            </button>
          {/each}
        </div>
        
        <!-- Stats -->
        <div class="results-stats">
          Showing {filteredApps.length} of {apps.length} apps
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  /* === HOLOGRAM v6.1 Discover Page === */
  
  .discover-page {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  
  /* Header */
  .discover-header {
    padding: var(--s-6, 24px);
    background: linear-gradient(to right, rgba(6, 182, 212, 0.1), rgba(139, 92, 246, 0.1));
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .header-content {
    max-width: 1200px;
    margin: 0 auto;
  }
  
  .page-title {
    font-family: var(--font-mono);
    font-size: 24px;
    font-weight: 700;
    color: var(--text-1, #f8f8fc);
    margin-bottom: var(--s-2, 8px);
  }
  
  .page-desc {
    font-size: 14px;
    color: var(--text-4, #505068);
  }
  
  /* Search */
  .search-row {
    display: flex;
    gap: var(--s-3, 12px);
    margin-top: var(--s-4, 16px);
  }
  
  .search-input-wrap {
    flex: 1;
    position: relative;
  }
  
  .search-input {
    width: 100%;
    padding: var(--s-3, 12px) var(--s-4, 16px);
    padding-left: 40px;
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    color: var(--text-1, #f8f8fc);
    font-size: 14px;
    outline: none;
    transition: border-color 200ms ease-out;
  }
  
  .search-input::placeholder {
    color: var(--text-5, #404058);
  }
  
  .search-input:focus {
    border-color: var(--cyan-500, #06b6d4);
  }
  
  .search-icon {
    position: absolute;
    left: var(--s-3, 12px);
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-5, #404058);
    font-size: 18px;
  }
  
  .sort-select {
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    color: var(--text-1, #f8f8fc);
    font-size: 14px;
    outline: none;
  }
  
  .sort-select:focus {
    border-color: var(--cyan-500, #06b6d4);
  }
  
  /* Rating Filter */
  .rating-filter {
    display: flex;
    align-items: center;
    gap: var(--s-4, 16px);
    margin-top: var(--s-3, 12px);
  }
  
  .filter-label {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .rating-slider {
    flex: 1;
    max-width: 192px;
    accent-color: var(--cyan-500, #06b6d4);
  }
  
  .rating-value {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-4, #505068);
  }
  
  .rating-value.high {
    color: var(--cyan-400, #22d3ee);
  }
  
  .rating-value.good {
    color: var(--status-ok, #34d399);
  }
  
  .rating-value.fair {
    color: var(--status-warn, #fbbf24);
  }
  
  /* Category Bar */
  .category-bar {
    padding: var(--s-3, 12px) var(--s-6, 24px);
    background: var(--void-mid, #12121c);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .category-tabs {
    max-width: 1200px;
    margin: 0 auto;
    display: flex;
    gap: var(--s-2, 8px);
  }
  
  .category-tab {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-4, 16px);
    border-radius: var(--r-lg, 12px);
    font-size: 13px;
    font-weight: 500;
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .category-tab:hover {
    background: var(--void-up, #181824);
    color: var(--text-1, #f8f8fc);
  }
  
  .category-tab.active {
    background: rgba(6, 182, 212, 0.15);
    color: var(--cyan-400, #22d3ee);
  }
  
  .tab-icon {
    font-size: 14px;
  }
  
  /* Content */
  .discover-content {
    flex: 1;
    overflow: auto;
    padding: var(--s-6, 24px);
  }
  
  .content-inner {
    max-width: 1200px;
    margin: 0 auto;
  }
  
  /* Empty/Loading States */
  .empty-state,
  .loading-state {
    text-align: center;
    padding: var(--s-16, 64px);
  }
  
  .empty-icon {
    font-size: 56px;
    display: block;
    margin-bottom: var(--s-4, 16px);
    color: var(--text-5, #404058);
  }
  
  .empty-title {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-2, #a8a8b8);
    margin-bottom: var(--s-2, 8px);
  }
  
  .empty-desc {
    color: var(--text-4, #505068);
    margin-bottom: var(--s-6, 24px);
  }
  
  .btn-start {
    padding: var(--s-3, 12px) var(--s-6, 24px);
    background: var(--cyan-500, #06b6d4);
    color: var(--void-pure, #000000);
    font-weight: 500;
    border-radius: var(--r-lg, 12px);
    border: none;
    cursor: pointer;
    transition: background 200ms ease-out;
  }
  
  .btn-start:hover {
    background: var(--cyan-400, #22d3ee);
  }
  
  .spinner {
    width: 48px;
    height: 48px;
    border: 4px solid var(--cyan-500, #06b6d4);
    border-top-color: transparent;
    border-radius: var(--r-full, 9999px);
    animation: spin 0.6s linear infinite;
    margin: 0 auto var(--s-4, 16px);
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  .loading-text {
    color: var(--text-4, #505068);
  }
  
  /* App Grid */
  .app-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
    gap: var(--s-4, 16px);
  }
  
  .app-card {
    padding: var(--s-5, 20px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    text-align: left;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .app-card:hover {
    border-color: rgba(6, 182, 212, 0.5);
    background: var(--void-surface, #1e1e2a);
  }
  
  .app-card:hover .app-name {
    color: var(--cyan-400, #22d3ee);
  }
  
  .app-header {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-3, 12px);
  }
  
  .app-icon-wrap {
    width: 48px;
    height: 48px;
    border-radius: var(--r-xl, 16px);
    background: linear-gradient(135deg, rgba(6, 182, 212, 0.2), rgba(139, 92, 246, 0.2));
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  
  .app-icon-img {
    width: 32px;
    height: 32px;
    object-fit: contain;
  }
  
  /* Fallback DERO icon - slightly larger to fill the container */
  .app-icon-fallback {
    width: 36px;
    height: 36px;
    opacity: 0.7;
  }
  
  .app-icon-placeholder {
    font-size: 24px;
    color: var(--text-3, #707088);
  }
  
  .app-info {
    flex: 1;
    min-width: 0;
  }
  
  .app-name {
    font-weight: 600;
    color: var(--text-2, #a8a8b8);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: color 200ms ease-out;
  }
  
  .app-durl {
    font-size: 12px;
    color: rgba(6, 182, 212, 0.7);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .app-scid {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    color: var(--text-5, #404058);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .app-desc {
    font-size: 13px;
    color: var(--text-4, #505068);
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    margin-bottom: var(--s-3, 12px);
  }
  
  .app-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .app-badges {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .epoch-badge {
    font-size: 14px;
    cursor: help;
    color: var(--status-ok, #34d399);
    filter: drop-shadow(0 0 4px rgba(52, 211, 153, 0.5));
    animation: epoch-glow 2s ease-in-out infinite;
  }
  
  @keyframes epoch-glow {
    0%, 100% { filter: drop-shadow(0 0 4px rgba(52, 211, 153, 0.5)); }
    50% { filter: drop-shadow(0 0 8px rgba(52, 211, 153, 0.8)); }
  }
  
  .app-rating {
    font-size: 13px;
    font-weight: 500;
  }
  
  /* Rating Colors */
  .rating-none {
    color: var(--text-5, #404058);
  }
  
  .rating-excellent {
    color: var(--cyan-400, #22d3ee);
  }
  
  .rating-great {
    color: var(--status-ok, #34d399);
  }
  
  .rating-good {
    color: var(--status-warn, #fbbf24);
  }
  
  .rating-fair {
    color: var(--violet-400, #a78bfa);
  }
  
  .rating-poor {
    color: var(--status-err, #f87171);
  }
  
  .no-rating {
    font-size: 12px;
    color: var(--text-5, #404058);
  }
  
  .rating-count {
    font-size: 12px;
    color: var(--text-5, #404058);
  }
  
  /* Stats */
  .results-stats {
    margin-top: var(--s-8, 32px);
    text-align: center;
    font-size: 13px;
    color: var(--text-5, #404058);
  }
  
  /* Gnomon auto-start checkbox */
  .gnomon-autostart-option {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    margin-top: 16px;
    color: var(--text-4);
    font-size: 12px;
    cursor: pointer;
  }
  
  .gnomon-autostart-option input[type="checkbox"] {
    width: 14px;
    height: 14px;
    accent-color: var(--cyan-400);
    cursor: pointer;
  }
  
  .gnomon-autostart-option span {
    user-select: none;
  }
  
  /* Gnomon sync progress */
  .gnomon-sync-progress {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-top: 16px;
    width: 280px;
  }
  
  .gnomon-sync-bar {
    flex: 1;
    height: 6px;
    background: var(--void-mid);
    border-radius: 3px;
    overflow: hidden;
  }
  
  .gnomon-sync-fill {
    height: 100%;
    background: var(--cyan-400);
    border-radius: 3px;
    transition: width 0.3s ease;
  }
  
  .gnomon-sync-percent {
    font-family: var(--font-mono);
    font-size: 13px;
    color: var(--cyan-400);
    min-width: 50px;
    text-align: right;
  }
  
  .empty-hint {
    margin-top: 12px;
    font-size: 12px;
    color: var(--text-5);
    font-style: italic;
  }
</style>
