<script>
  import { createEventDispatcher } from 'svelte';
  import { ShardFile, ConstructFromShards, SelectFile, SelectFolder } from '../../../wailsjs/go/main/App.js';
  import { toast } from '../stores/appState.js';
  
  const dispatch = createEventDispatcher();
  
  export let visible = false;
  
  let mode = 'shard';  // 'shard' or 'construct'
  let filePath = '';
  let shardPath = '';
  let compress = true;
  let loading = false;
  let result = null;
  
  async function selectInputFile() {
    try {
      const path = await SelectFile();
      if (path) {
        filePath = path;
      }
    } catch (e) {
      console.error('File selection error:', e);
    }
  }
  
  async function selectShardFolder() {
    try {
      const path = await SelectFolder();
      if (path) {
        shardPath = path;
      }
    } catch (e) {
      console.error('Folder selection error:', e);
    }
  }
  
  async function performShard() {
    if (!filePath) {
      toast.warning('Please select a file to shard');
      return;
    }
    
    loading = true;
    result = null;
    
    try {
      const res = await ShardFile(filePath, compress);
      if (res.success) {
        result = res;
        toast.success(`File sharded into ${res.shardCount} parts`);
      } else {
        toast.error(res.error || 'Sharding failed');
      }
    } catch (e) {
      toast.error(e.message || 'Sharding failed');
    } finally {
      loading = false;
    }
  }
  
  async function performConstruct() {
    if (!shardPath) {
      toast.warning('Please select a folder containing shard files');
      return;
    }
    
    loading = true;
    result = null;
    
    try {
      const res = await ConstructFromShards(shardPath);
      if (res.success) {
        result = res;
        toast.success('File reconstructed successfully');
      } else {
        toast.error(res.error || 'Construction failed');
      }
    } catch (e) {
      toast.error(e.message || 'Construction failed');
    } finally {
      loading = false;
    }
  }
  
  function close() {
    visible = false;
    result = null;
    dispatch('close');
  }
  
  function clearInputs() {
    filePath = '';
    shardPath = '';
    result = null;
  }
  
  function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
</script>

{#if visible}
  <div class="shard-backdrop" on:click={close}>
    <div class="shard-modal" on:click|stopPropagation>
      <!-- Header -->
      <div class="shard-header">
        <div class="shard-header-left">
          <span class="shard-header-icon">⬢</span>
          <h2 class="shard-title">DocShard Manager</h2>
        </div>
        <button on:click={close} class="shard-close-btn">✕</button>
      </div>
      
      <!-- Mode Selector -->
      <div class="shard-mode-bar">
        <div class="mode-tabs">
          <div class="mode-tab-group">
            <button
              on:click={() => { mode = 'shard'; clearInputs(); }}
              class="mode-tab {mode === 'shard' ? 'active' : ''}"
            >
              <span class="mode-icon">◈</span> Shard File
            </button>
            <button
              on:click={() => { mode = 'construct'; clearInputs(); }}
              class="mode-tab {mode === 'construct' ? 'active' : ''}"
            >
              <span class="mode-icon">◇</span> Reconstruct
            </button>
          </div>
        </div>
      </div>
      
      <!-- Description -->
      <div class="shard-description">
        {#if mode === 'shard'}
          <p>Split a large file into smaller DocShard pieces for deployment as multiple TELA-DOC contracts. Useful for files exceeding single-contract size limits.</p>
        {:else}
          <p>Reconstruct an original file from its DocShard pieces. Select a folder containing the shard files.</p>
        {/if}
      </div>
      
      <!-- Input Form -->
      <div class="shard-form">
        {#if mode === 'shard'}
          <div class="form-group">
            <label class="form-label">File to Shard</label>
            <div class="input-row">
              <input
                type="text"
                bind:value={filePath}
                placeholder="Select file to split into shards..."
                class="input-field"
                readonly
              />
              <button on:click={selectInputFile} class="btn-secondary">
                Browse
              </button>
            </div>
          </div>
          
          <div class="form-group">
            <label class="form-label">
              <input type="checkbox" bind:checked={compress} class="checkbox" />
              Enable GZIP Compression
            </label>
            <span class="form-hint">Reduces shard sizes but requires decompression on reconstruction</span>
          </div>
        {:else}
          <div class="form-group">
            <label class="form-label">Shard Folder</label>
            <div class="input-row">
              <input
                type="text"
                bind:value={shardPath}
                placeholder="Select folder containing shard files..."
                class="input-field"
                readonly
              />
              <button on:click={selectShardFolder} class="btn-secondary">
                Browse
              </button>
            </div>
          </div>
        {/if}
        
        <div class="form-actions">
          <button
            on:click={mode === 'shard' ? performShard : performConstruct}
            disabled={loading}
            class="btn-primary"
          >
            {#if loading}
              Processing...
            {:else if mode === 'shard'}
              Shard File
            {:else}
              Reconstruct
            {/if}
          </button>
          <button on:click={clearInputs} class="btn-secondary">
            Clear
          </button>
        </div>
      </div>
      
      <!-- Results -->
      <div class="shard-results">
        {#if result}
          <div class="result-card success">
            <div class="result-icon">✓</div>
            <div class="result-content">
              {#if mode === 'shard'}
                <h3 class="result-title">File Sharded Successfully</h3>
                <div class="result-details">
                  <div class="detail-row">
                    <span class="detail-label">Shards Created:</span>
                    <span class="detail-value">{result.shardCount}</span>
                  </div>
                  <div class="detail-row">
                    <span class="detail-label">Output Directory:</span>
                    <span class="detail-value mono">{result.outputDir}</span>
                  </div>
                  <div class="detail-row">
                    <span class="detail-label">Compression:</span>
                    <span class="detail-value">{result.compressed ? 'GZIP Enabled' : 'None'}</span>
                  </div>
                </div>
              {:else}
                <h3 class="result-title">File Reconstructed</h3>
                <div class="result-details">
                  <div class="detail-row">
                    <span class="detail-label">Output File:</span>
                    <span class="detail-value mono">{result.outputPath}</span>
                  </div>
                  <div class="detail-row">
                    <span class="detail-label">File Size:</span>
                    <span class="detail-value">{formatBytes(result.size)}</span>
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {:else}
          <div class="shard-empty-state">
            <span class="empty-icon">⬢</span>
            <p class="empty-desc">
              {#if mode === 'shard'}
                Select a file to split into DocShards
              {:else}
                Select a folder containing DocShard files
              {/if}
            </p>
          </div>
        {/if}
      </div>
      
      <!-- Info Panel -->
      <div class="shard-info">
        <div class="info-title">About DocShards</div>
        <p class="info-text">
          DocShards allow large files to be deployed across multiple TELA-DOC contracts. 
          Each shard is deployed separately and can be reconstructed client-side using the 
          shard metadata embedded in each piece.
        </p>
      </div>
    </div>
  </div>
{/if}

<style>
  /* === HOLOGRAM v6.1 DocShard Modal === */
  
  .shard-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(4px);
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-4, 16px);
  }
  
  .shard-modal {
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    width: 100%;
    max-width: 600px;
    max-height: 90vh;
    overflow: hidden;
    box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
  }
  
  /* Header */
  .shard-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4, 16px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    background: var(--void-deep, #08080e);
  }
  
  .shard-header-left {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
  }
  
  .shard-header-icon {
    font-size: 24px;
    color: var(--amber-400, #fbbf24);
  }
  
  .shard-title {
    font-family: var(--font-mono);
    font-size: 18px;
    font-weight: 700;
    color: var(--text-1, #f8f8fc);
    margin: 0;
  }
  
  .shard-close-btn {
    font-size: 20px;
    padding: var(--s-2, 8px);
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: color 200ms ease-out;
  }
  
  .shard-close-btn:hover {
    color: var(--text-1, #f8f8fc);
  }
  
  /* Mode Bar */
  .shard-mode-bar {
    padding: var(--s-4, 16px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    background: rgba(18, 18, 28, 0.5);
  }
  
  .mode-tabs {
    display: flex;
    align-items: center;
    gap: var(--s-4, 16px);
  }
  
  .mode-tab-group {
    display: flex;
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
    padding: var(--s-1, 4px);
    width: 100%;
  }
  
  .mode-tab {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
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
  
  .mode-tab:hover {
    color: var(--text-1, #f8f8fc);
  }
  
  .mode-tab.active {
    background: var(--amber-500, #f59e0b);
    color: var(--void-pure, #000000);
  }
  
  .mode-icon {
    font-size: 14px;
  }
  
  /* Description */
  .shard-description {
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: rgba(251, 191, 36, 0.05);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .shard-description p {
    margin: 0;
    font-size: 13px;
    color: var(--text-3, #707088);
    line-height: 1.5;
  }
  
  /* Form */
  .shard-form {
    padding: var(--s-4, 16px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .form-group {
    margin-bottom: var(--s-4, 16px);
  }
  
  .form-group:last-of-type {
    margin-bottom: 0;
  }
  
  .form-label {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 13px;
    color: var(--text-3, #707088);
    margin-bottom: var(--s-2, 8px);
  }
  
  .form-hint {
    display: block;
    font-size: 12px;
    color: var(--text-5, #404058);
    margin-top: var(--s-1, 4px);
    margin-left: var(--s-5, 20px);
  }
  
  .checkbox {
    width: 16px;
    height: 16px;
    accent-color: var(--amber-500, #f59e0b);
  }
  
  .input-row {
    display: flex;
    gap: var(--s-2, 8px);
  }
  
  .input-field {
    flex: 1;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-lg, 12px);
    color: var(--text-1, #f8f8fc);
    font-size: 13px;
    outline: none;
    transition: border-color 200ms ease-out;
  }
  
  .input-field::placeholder {
    color: var(--text-5, #404058);
  }
  
  .input-field:focus {
    border-color: var(--amber-500, #f59e0b);
  }
  
  .form-actions {
    display: flex;
    gap: var(--s-2, 8px);
    margin-top: var(--s-4, 16px);
  }
  
  .btn-primary {
    padding: var(--s-2, 8px) var(--s-6, 24px);
    background: var(--amber-400, #fbbf24);
    color: var(--void-pure, #000000);
    border-radius: var(--r-lg, 12px);
    font-weight: 500;
    border: none;
    cursor: pointer;
    transition: background 200ms ease-out;
  }
  
  .btn-primary:hover {
    background: var(--amber-300, #fcd34d);
  }
  
  .btn-primary:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .btn-secondary {
    padding: var(--s-2, 8px) var(--s-4, 16px);
    background: var(--void-up, #181824);
    color: var(--text-3, #707088);
    border-radius: var(--r-lg, 12px);
    border: none;
    cursor: pointer;
    transition: background 200ms ease-out;
  }
  
  .btn-secondary:hover {
    background: var(--void-surface, #1e1e2a);
  }
  
  /* Results */
  .shard-results {
    padding: var(--s-4, 16px);
    min-height: 120px;
  }
  
  .shard-empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-8, 32px);
    text-align: center;
  }
  
  .empty-icon {
    font-size: 36px;
    color: var(--text-5, #404058);
    margin-bottom: var(--s-3, 12px);
  }
  
  .empty-desc {
    color: var(--text-4, #505068);
    margin: 0;
  }
  
  .result-card {
    display: flex;
    gap: var(--s-3, 12px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
    border-left: 4px solid var(--status-ok, #34d399);
  }
  
  .result-icon {
    font-size: 24px;
    color: var(--status-ok, #34d399);
  }
  
  .result-content {
    flex: 1;
  }
  
  .result-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1, #f8f8fc);
    margin: 0 0 var(--s-3, 12px) 0;
  }
  
  .result-details {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .detail-row {
    display: flex;
    gap: var(--s-3, 12px);
    font-size: 13px;
  }
  
  .detail-label {
    color: var(--text-4, #505068);
    min-width: 120px;
  }
  
  .detail-value {
    color: var(--text-2, #a8a8b8);
  }
  
  .detail-value.mono {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    word-break: break-all;
  }
  
  /* Info Panel */
  .shard-info {
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .info-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-4, #505068);
    margin-bottom: var(--s-2, 8px);
  }
  
  .info-text {
    font-size: 12px;
    color: var(--text-5, #404058);
    line-height: 1.5;
    margin: 0;
  }
</style>

