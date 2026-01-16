<script>
  import {
    AlertTriangle,
    CheckCircle,
    FolderOpen,
    Layers,
    Loader2,
    Scissors
  } from 'lucide-svelte';

  export let shardError = '';
  export let shardResult = null;
  export let shardMode = 'shard';
  export let shardFilePath = '';
  export let shardFolderPath = '';
  export let shardCompress = false;
  export let shardLoading = false;
  export let formatShardBytes = () => '';
  export let resetShard = () => {};
  export let selectShardFile = () => {};
  export let selectShardFolder = () => {};
  export let performShard = () => {};
  export let performReconstruct = () => {};
</script>

<div class="content-section">
  <h2 class="content-section-title">DocShard Manager</h2>
  <p class="content-section-desc">Split large files into shards for deployment, or reconstruct files from their shards.</p>
  
  <!-- Error Display -->
  {#if shardError}
    <div class="alert alert-error" style="margin-bottom: var(--s-4);">
      <AlertTriangle size={16} />
      <span>{shardError}</span>
    </div>
  {/if}
  
  <!-- Success Display -->
  {#if shardResult}
    <div class="clone-success-card">
      <div class="clone-success-header">
        <CheckCircle size={24} class="clone-success-icon" />
        <div>
          {#if shardResult.mode === 'shard'}
            <h3 class="clone-success-title">File Sharded Successfully!</h3>
            <p class="clone-success-subtitle">{shardResult.shardCount} shards created</p>
          {:else}
            <h3 class="clone-success-title">File Reconstructed!</h3>
            <p class="clone-success-subtitle">Original file restored</p>
          {/if}
        </div>
      </div>
      
      <div class="clone-result-details">
        {#if shardResult.mode === 'shard'}
          <div class="clone-detail-row">
            <span class="clone-detail-label">Shards Created</span>
            <span class="clone-detail-value">{shardResult.shardCount}</span>
          </div>
          <div class="clone-detail-row">
            <span class="clone-detail-label">Output Directory</span>
            <code class="clone-detail-value">{shardResult.outputDir}</code>
          </div>
          <div class="clone-detail-row">
            <span class="clone-detail-label">Compression</span>
            <span class="clone-detail-value">{shardResult.compressed ? 'GZIP Enabled' : 'None'}</span>
          </div>
        {:else}
          <div class="clone-detail-row">
            <span class="clone-detail-label">Output File</span>
            <code class="clone-detail-value">{shardResult.outputPath}</code>
          </div>
          <div class="clone-detail-row">
            <span class="clone-detail-label">File Size</span>
            <span class="clone-detail-value">{formatShardBytes(shardResult.size)}</span>
          </div>
        {/if}
      </div>
      
      <div class="clone-actions">
        <button class="btn btn-ghost" on:click={resetShard}>
          {shardMode === 'shard' ? 'Shard Another File' : 'Reconstruct Another'}
        </button>
      </div>
    </div>
  {:else}
    <!-- Mode Selector Tabs -->
    <div class="shard-mode-tabs">
      <button 
        class="shard-mode-tab" 
        class:active={shardMode === 'shard'}
        on:click={() => { shardMode = 'shard'; resetShard(); }}
      >
        <Scissors size={16} />
        Shard File
      </button>
      <button 
        class="shard-mode-tab" 
        class:active={shardMode === 'reconstruct'}
        on:click={() => { shardMode = 'reconstruct'; resetShard(); }}
      >
        <Layers size={16} />
        Reconstruct
      </button>
    </div>
    
    {#if shardMode === 'shard'}
      <!-- Shard File Card -->
      <div class="content-card">
        <div class="content-card-header">
          <Scissors size={32} class="content-card-icon" />
          <p class="content-card-title">Shard File</p>
          <p class="content-card-text">Split a large file into smaller DocShard pieces for deployment as multiple TELA-DOC contracts. Useful for files exceeding single-contract size limits.</p>
        </div>
        
        <div class="form-group" style="margin-top: var(--s-4);">
          <label class="form-label">File to Shard</label>
          <div class="shard-input-row">
            <input
              type="text"
              bind:value={shardFilePath}
              placeholder="Select file to split into shards..."
              class="input input-mono"
              readonly
            />
            <button class="btn btn-secondary" on:click={selectShardFile}>
              Browse
            </button>
          </div>
        </div>
        
        <div class="form-group" style="margin-top: var(--s-3);">
          <label class="checkbox-wrap">
            <input type="checkbox" bind:checked={shardCompress} class="checkbox" />
            <span class="checkbox-label">Enable GZIP Compression</span>
          </label>
          <span class="form-hint">Reduces shard sizes but requires decompression on reconstruction</span>
        </div>
        
        <button 
          class="btn btn-primary btn-block" 
          style="margin-top: var(--s-4);"
          on:click={performShard}
          disabled={shardLoading || !shardFilePath}
        >
          {#if shardLoading}
            <Loader2 size={16} class="spinner" />
            Sharding...
          {:else}
            <Scissors size={16} />
            Shard File
          {/if}
        </button>
      </div>
    {:else}
      <!-- Reconstruct Card -->
      <div class="content-card">
        <div class="content-card-header">
          <Layers size={32} class="content-card-icon" />
          <p class="content-card-title">Reconstruct File</p>
          <p class="content-card-text">Reconstruct an original file from its DocShard pieces. Select a folder containing the shard files.</p>
        </div>
        
        <div class="form-group" style="margin-top: var(--s-4);">
          <label class="form-label">Shard Folder</label>
          <div class="shard-input-row">
            <input
              type="text"
              bind:value={shardFolderPath}
              placeholder="Select folder containing shard files..."
              class="input input-mono"
              readonly
            />
            <button class="btn btn-secondary" on:click={selectShardFolder}>
              <FolderOpen size={14} />
              Browse
            </button>
          </div>
        </div>
        
        <button 
          class="btn btn-primary btn-block" 
          style="margin-top: var(--s-4);"
          on:click={performReconstruct}
          disabled={shardLoading || !shardFolderPath}
        >
          {#if shardLoading}
            <Loader2 size={16} class="spinner" />
            Reconstructing...
          {:else}
            <Layers size={16} />
            Reconstruct File
          {/if}
        </button>
      </div>
    {/if}
    
    <!-- Info Panel -->
    <div class="info-panel" style="margin-top: var(--s-4);">
      <div class="info-panel-icon">◎</div>
      <div class="info-panel-content">
        <p class="info-panel-title">About DocShards</p>
        <ul class="info-list">
          <li>DocShards allow large files to be deployed across multiple TELA-DOC contracts</li>
          <li>Each shard is deployed separately and can be reconstructed client-side</li>
          <li>Shard metadata is embedded in each piece for seamless reconstruction</li>
        </ul>
      </div>
    </div>
  {/if}
</div>
