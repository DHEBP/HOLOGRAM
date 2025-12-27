<script>
  import { onMount } from 'svelte';
  import { walletState, toast } from '../lib/stores/appState.js';
  import { GetMODsList, GetMODInfo, GetAllMODClasses, GetMODsByClass, PrepareMODInstall } from '../../wailsjs/go/main/App.js';
  import { 
    Puzzle, Library, Palette, Zap, Database, Shield, Wrench, 
    AlertTriangle, Search, ArrowRight, Copy, ChevronRight, X, Check, Loader2
  } from 'lucide-svelte';
  
  let loading = true;
  let allMods = [];
  let filteredMods = [];
  let classes = [];
  let selectedClass = 'all';
  let searchQuery = '';
  let error = null;
  
  // Detail modal state
  let selectedMod = null;
  let modDetails = null;
  let loadingDetails = false;
  
  // Install wizard state
  let showInstallWizard = false;
  let installScid = '';
  let installLoading = false;
  let installResult = null;
  let installError = null;
  
  onMount(async () => {
    await loadData();
  });
  
  async function loadData() {
    loading = true;
    error = null;
    
    try {
      // Load all MODs
      const modsResult = await GetMODsList();
      if (modsResult.success) {
        allMods = modsResult.mods || [];
        filteredMods = [...allMods];
      } else {
        error = modsResult.error || 'Failed to load MODs';
      }
      
      // Load classes
      const classesResult = await GetAllMODClasses();
      if (classesResult.success) {
        classes = classesResult.classes || [];
      }
    } catch (e) {
      error = e.message || 'An error occurred';
    } finally {
      loading = false;
    }
  }
  
  function filterMods() {
    let result = [...allMods];
    
    // Filter by class
    if (selectedClass !== 'all') {
      result = result.filter(m => m.class === selectedClass);
    }
    
    // Filter by search query
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter(m => 
        m.name.toLowerCase().includes(query) ||
        m.tag.toLowerCase().includes(query) ||
        m.description.toLowerCase().includes(query)
      );
    }
    
    filteredMods = result;
  }
  
  $: if (selectedClass || searchQuery !== undefined) {
    filterMods();
  }
  
  async function openModDetails(mod) {
    selectedMod = mod;
    modDetails = null;
    loadingDetails = true;
    
    try {
      const result = await GetMODInfo(mod.tag);
      if (result.success) {
        modDetails = result;
      }
    } catch (e) {
      console.error('Failed to load MOD details:', e);
    } finally {
      loadingDetails = false;
    }
  }
  
  function closeModDetails() {
    selectedMod = null;
    modDetails = null;
  }
  
  function openInstallWizard() {
    showInstallWizard = true;
    installScid = '';
    installResult = null;
    installError = null;
  }
  
  function closeInstallWizard() {
    showInstallWizard = false;
    installScid = '';
    installResult = null;
    installError = null;
  }
  
  async function prepareMODInstall() {
    if (!selectedMod || !installScid || installScid.length < 64) {
      installError = 'Please enter a valid 64-character SCID';
      return;
    }
    
    installLoading = true;
    installError = null;
    installResult = null;
    
    try {
      const result = await PrepareMODInstall(installScid, selectedMod.tag);
      if (result.success) {
        installResult = result;
      } else {
        installError = result.error || 'Failed to prepare MOD installation';
      }
    } catch (e) {
      installError = e.message || 'An error occurred';
    } finally {
      installLoading = false;
    }
  }
  
  function copyToClipboard(text, label = 'Code') {
    navigator.clipboard.writeText(text);
    toast.success(`${label} copied to clipboard`, 2000);
  }
  
  // Simple code escaping - just display raw DVM BASIC code
  function escapeCode(code) {
    if (!code) return '';
    return code
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');
  }
  
  // Map class names to Lucide icon components
  const classIconMap = {
    'vs': Palette,       // Visual/Style mods
    'tx': Zap,           // Transaction mods
    'storage': Database, // Storage mods
    'auth': Shield,      // Auth mods
  };
  
  function getClassIcon(className) {
    return classIconMap[className?.toLowerCase()] || Wrench;
  }
</script>

<!-- v6.1 Unified Page Framework -->
<div class="page-layout">
  <!-- Page Header -->
  <div class="page-header">
    <div class="page-header-inner">
      <div class="page-header-left">
        <h1 class="page-header-title">
          <Puzzle size={20} class="page-header-icon" strokeWidth={1.5} />
            TELA MODs
          </h1>
        <p class="page-header-desc">Browse and install modular DVM code extensions</p>
        </div>
        
      <!-- Search in Header Actions -->
      <div class="page-header-actions">
        <div class="mods-search">
          <Search size={16} class="mods-search-icon" strokeWidth={1.5} />
          <input
            type="text"
            bind:value={searchQuery}
            placeholder="Search MODs..."
            class="mods-search-input"
          />
          <button 
            class="mods-search-btn"
            disabled={!searchQuery.trim()}
          >
            Search
          </button>
        </div>
      </div>
    </div>
  </div>
  
  <!-- Page Body: Sidebar + Content -->
  <div class="page-body">
    <!-- Sidebar - Class Filter -->
    <div class="page-sidebar">
      <div class="page-sidebar-section">Classes</div>
      <nav class="page-sidebar-nav">
      <button
        on:click={() => selectedClass = 'all'}
          class="page-sidebar-item {selectedClass === 'all' ? 'active' : ''}"
      >
          <span class="page-sidebar-item-icon">
            <Library size={16} strokeWidth={1.5} />
          </span>
          <span class="page-sidebar-item-label">All MODs</span>
          <span class="page-sidebar-item-count">({allMods.length})</span>
      </button>
      
      {#if classes.length > 0}
          {#each classes as cls}
            <button
              on:click={() => selectedClass = cls.name}
              class="page-sidebar-item {selectedClass === cls.name ? 'active' : ''}"
            >
              <span class="page-sidebar-item-icon">
                <svelte:component this={getClassIcon(cls.name)} size={16} strokeWidth={1.5} />
              </span>
              <span class="page-sidebar-item-label">{cls.name}</span>
              <span class="page-sidebar-item-count">({cls.modCount})</span>
            </button>
          {/each}
      {/if}
      </nav>
    </div>
    
    <!-- Main Content - MOD Grid -->
    <div class="page-content">
      {#if loading}
        <div class="content-state">
          <Loader2 size={32} class="spin" strokeWidth={1.5} />
          <p class="text-description">Loading MODs...</p>
        </div>
      {:else if error}
        <div class="content-state">
          <AlertTriangle size={32} strokeWidth={1.5} />
          <p class="error-text">{error}</p>
          <button on:click={loadData} class="btn btn-secondary">
            Retry
          </button>
        </div>
      {:else if filteredMods.length === 0}
        <div class="content-state">
          <Search size={32} strokeWidth={1.5} />
          <p class="text-description">No MODs found</p>
          {#if searchQuery || selectedClass !== 'all'}
            <button
              on:click={() => { searchQuery = ''; selectedClass = 'all'; }}
              class="btn btn-ghost"
            >
              Clear filters
            </button>
          {/if}
        </div>
      {:else}
        <div class="mods-grid">
          {#each filteredMods as mod}
            <div class="mod-card" on:click={() => openModDetails(mod)} role="button" tabindex="0" on:keydown={(e) => e.key === 'Enter' && openModDetails(mod)}>
              <!-- v6.1 Horizontal Layout: 48x48 icon on left -->
              <div class="mod-icon">
                <svelte:component this={getClassIcon(mod.class)} size={24} strokeWidth={1.5} />
                </div>
              <div class="mod-info">
                <div class="mod-name">{mod.name}</div>
                <div class="mod-meta">
                  <span class="badge badge-cyan">{mod.class}</span>
                  <span class="mod-tag">by {mod.tag}</span>
                </div>
                <p class="mod-desc">{mod.description || 'No description available'}</p>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>

<!-- MOD Details Modal -->
{#if selectedMod}
  <div 
    class="modal-overlay"
    on:click={closeModDetails}
  >
    <div class="modal-content" on:click|stopPropagation>
      <!-- Modal Header -->
      <div class="modal-header">
        <div class="modal-header-left">
          <div class="modal-icon">
            <svelte:component this={getClassIcon(selectedMod.class)} size={24} strokeWidth={1.5} />
          </div>
          <div>
            <h2 class="modal-title">{selectedMod.name}</h2>
            <div class="modal-meta">
              <span class="modal-tag">{selectedMod.tag}</span>
              <span class="badge badge-cyan">{selectedMod.class}</span>
            </div>
          </div>
        </div>
        <button on:click={closeModDetails} class="modal-close">
          <X size={20} strokeWidth={1.5} />
        </button>
      </div>
      
      <!-- Modal Content -->
      <div class="modal-body">
        {#if loadingDetails}
          <div class="modal-loading">
            <Loader2 size={24} class="spin" strokeWidth={1.5} />
          </div>
        {:else if modDetails}
          <!-- Description -->
          <div class="modal-section">
            <h3 class="text-label-md">Description</h3>
            <p class="text-body">{modDetails.description || selectedMod.description || 'No description available'}</p>
          </div>
          
          <!-- Functions -->
          {#if modDetails.functionNames?.length > 0}
            <div class="modal-section">
              <h3 class="text-label-md">Functions ({modDetails.functionNames.length})</h3>
              <div class="function-tags">
                {#each modDetails.functionNames as funcName}
                  <span class="function-tag">{funcName}()</span>
                {/each}
              </div>
            </div>
          {/if}
          
          <!-- Function Code -->
          {#if modDetails.functionCode}
            <div class="modal-section">
              <div class="code-header">
                <h3 class="text-label-md">DVM Code</h3>
                <button
                  on:click={() => copyToClipboard(modDetails.functionCode, 'MOD code')}
                  class="copy-btn"
                >
                  <Copy size={12} strokeWidth={1.5} />
                  <span>Copy Code</span>
                </button>
              </div>
              <pre class="code-block">{modDetails.functionCode}</pre>
            </div>
          {/if}
        {:else}
          <p class="text-description text-center py-8">No additional details available</p>
        {/if}
      </div>
      
      <!-- Modal Footer -->
      <div class="modal-footer">
        <button on:click={closeModDetails} class="btn btn-ghost">
          Close
        </button>
        
        <button
          on:click={openInstallWizard}
          disabled={!modDetails}
          class="btn btn-primary"
        >
          <Wrench size={14} strokeWidth={1.5} />
          Install to SC
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Install Wizard Modal -->
{#if showInstallWizard && selectedMod}
  <div class="modal-overlay nested" on:click={closeInstallWizard}>
    <div class="modal-content" on:click|stopPropagation>
      <!-- Wizard Header -->
      <div class="modal-header">
        <div class="modal-header-left">
          <Wrench size={20} class="modal-icon-inline" strokeWidth={1.5} />
          <div>
            <h2 class="modal-title">Install MOD: {selectedMod.name}</h2>
            <p class="text-description">Prepare this MOD for installation on a smart contract</p>
          </div>
        </div>
      </div>
      
      <!-- Wizard Content -->
      <div class="modal-body">
        {#if !installResult}
          <!-- Step 1: Enter SCID -->
          <div class="form-stack">
            <div class="form-group">
              <label class="form-label">Target Smart Contract SCID</label>
              <input
                type="text"
                bind:value={installScid}
                placeholder="Enter 64-character SCID..."
                class="input input-mono"
              />
              <p class="text-hint mt-2">You must be the owner of this SC to install MODs</p>
            </div>
            
            {#if installError}
              <div class="alert-error">
                <AlertTriangle size={14} strokeWidth={1.5} />
                <span>{installError}</span>
              </div>
            {/if}
            
            {#if !$walletState.isOpen}
              <div class="alert-warning">
                <AlertTriangle size={14} strokeWidth={1.5} />
                <span>Please open a wallet first</span>
              </div>
            {/if}
          </div>
        {:else}
          <!-- Step 2: Review & Copy -->
          <div class="form-stack">
            <div class="alert-success">
              <Check size={14} strokeWidth={2} />
              <div>
                <span class="alert-title">MOD Code Prepared!</span>
                <p class="text-description mt-1">The updated SC code with the MOD has been prepared. To install:</p>
                <ol class="install-steps">
                  <li>Copy the updated code below</li>
                  <li>Use UPDATE_SC_CODE via XSWD to update your SC</li>
                  <li>Or deploy a new SC with this code</li>
                </ol>
              </div>
            </div>
            
            <!-- Added Functions -->
            <div class="modal-section">
              <h4 class="text-label-md">Added Functions</h4>
              <div class="function-tags">
                {#each installResult.functionNames || [] as funcName}
                  <span class="function-tag">{funcName}()</span>
                {/each}
              </div>
            </div>
            
            <!-- Updated Code Preview -->
            <div class="modal-section">
              <div class="code-header">
                <h4 class="text-label-md">Updated SC Code</h4>
                <button
                  on:click={() => copyToClipboard(installResult.updatedCode)}
                  class="copy-btn"
                >
                  <Copy size={12} strokeWidth={1.5} />
                  <span>Copy Full Code</span>
                </button>
              </div>
              <pre class="code-block small">{installResult.updatedCode?.slice(-500) || ''}...</pre>
              <p class="text-hint mt-1">Showing last 500 characters</p>
            </div>
          </div>
        {/if}
      </div>
      
      <!-- Wizard Footer -->
      <div class="modal-footer">
        <button on:click={closeInstallWizard} class="btn btn-ghost">
          {installResult ? 'Done' : 'Cancel'}
        </button>
        
        {#if !installResult}
          <button
            on:click={prepareMODInstall}
            disabled={installLoading || !installScid || installScid.length < 64 || !$walletState.isOpen}
            class="btn btn-primary"
          >
            {#if installLoading}
              <Loader2 size={14} class="spin" strokeWidth={1.5} />
              Preparing...
            {:else}
              Prepare Installation
            {/if}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  /* === v6.1 MODs PAGE === */
  
  /* Header Search - Matching OmniSearch Style */
  .mods-search {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-default, rgba(255, 255, 255, 0.09));
    border-radius: var(--r-xl, 16px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    transition: all 0.2s ease;
    min-width: 300px;
  }
  
  .mods-search:focus-within {
    border-color: var(--cyan-500, #06b6d4);
    box-shadow: 0 0 15px rgba(34, 211, 238, 0.15);
  }
  
  :global(.mods-search-icon) {
    color: var(--text-4, #505068);
    flex-shrink: 0;
  }
  
  .mods-search-input {
    flex: 1;
    background: transparent;
    border: none;
    outline: none;
    color: var(--text-1, #f8f8fc);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 13px;
    min-width: 150px;
  }
  
  .mods-search-input::placeholder {
    color: var(--text-4, #505068);
  }
  
  .mods-search-btn {
    padding: var(--s-2, 8px) var(--s-4, 16px);
    background: var(--cyan-500, #06b6d4);
    border: none;
    border-radius: var(--r-md, 8px);
    color: var(--void-pure, #000);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: all 150ms;
    flex-shrink: 0;
  }
  
  .mods-search-btn:hover:not(:disabled) {
    filter: brightness(1.1);
    transform: translateY(-1px);
    box-shadow: 0 0 15px rgba(34, 211, 238, 0.3);
  }
  
  .mods-search-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  
  /* Content States (loading, error, empty) */
  .content-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-10);
    color: var(--text-4);
    gap: var(--s-4);
    min-height: 300px;
  }
  
  .content-state .error-text {
    color: var(--status-err);
  }
  
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  /* MOD Grid */
  .mods-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: var(--s-4);
  }
  
  /* Modal Styles */
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(4px);
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-4);
  }
  
  .modal-overlay.nested {
    z-index: 60;
    background: rgba(0, 0, 0, 0.7);
  }
  
  .modal-content {
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-xl);
    max-width: 700px;
    width: 100%;
    max-height: 85vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  
  .modal-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: var(--s-5);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .modal-header-left {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
  }
  
  .modal-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    background: var(--void-deep);
    border-radius: var(--r-md);
    color: var(--cyan-400);
    flex-shrink: 0;
  }
  
  :global(.modal-icon-inline) {
    color: var(--cyan-400);
    flex-shrink: 0;
  }
  
  .modal-title {
    font-size: 1rem;
    font-weight: 400;
    color: var(--text-1);
  }
  
  .modal-meta {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin-top: var(--s-1);
  }
  
  .modal-tag {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-4);
  }
  
  .modal-close {
    padding: var(--s-2);
    color: var(--text-4);
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    cursor: pointer;
    transition: all 150ms;
  }
  
  .modal-close:hover {
    color: var(--text-1);
    background: rgba(255, 255, 255, 0.05);
  }
  
  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: var(--s-5);
  }
  
  .modal-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-10);
    color: var(--text-4);
  }
  
  .modal-section {
    margin-bottom: var(--s-5);
  }
  
  .modal-section:last-child {
    margin-bottom: 0;
  }
  
  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4) var(--s-5);
    border-top: 1px solid var(--border-dim);
    background: var(--void-deep);
  }
  
  /* Function Tags */
  .function-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2);
    margin-top: var(--s-3);
  }
  
  .function-tag {
    font-size: 11px;
    font-family: var(--font-mono);
    padding: 4px 10px;
    background: rgba(167, 139, 250, 0.15);
    color: var(--violet-400);
    border-radius: var(--r-full);
  }
  
  /* Code Block */
  .code-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2);
  }
  
  .copy-btn {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    font-size: 10px;
    color: var(--text-4);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: color 150ms;
  }
  
  .copy-btn:hover {
    color: var(--cyan-400);
  }
  
  .code-block {
    padding: var(--s-4);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-md);
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.6;
    overflow-x: auto;
    max-height: 240px;
    white-space: pre-wrap;
  }
  
  .code-block.small {
    font-size: 10px;
    max-height: 160px;
  }
  
  /* Alerts */
  .alert-error,
  .alert-warning,
  .alert-success {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
    padding: var(--s-4);
    border-radius: var(--r-md);
    font-size: 12px;
  }
  
  .alert-error {
    background: rgba(248, 113, 113, 0.08);
    border: 1px solid rgba(248, 113, 113, 0.2);
    color: var(--status-err);
  }
  
  .alert-warning {
    background: rgba(251, 191, 36, 0.08);
    border: 1px solid rgba(251, 191, 36, 0.2);
    color: var(--status-warn);
  }
  
  .alert-success {
    background: rgba(52, 211, 153, 0.08);
    border: 1px solid rgba(52, 211, 153, 0.2);
    color: var(--status-ok);
  }
  
  .alert-title {
    font-weight: 500;
  }
  
  .install-steps {
    margin-top: var(--s-2);
    padding-left: var(--s-4);
    list-style-type: decimal;
    color: var(--text-3);
    font-size: 11px;
    line-height: 1.6;
  }
  
  /* Form Stack */
  .form-stack {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  
  .form-group {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .form-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
</style>
  