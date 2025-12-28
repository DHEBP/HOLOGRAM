<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { walletState, settingsState, toast } from '../stores/appState.js';
  import { ScanFolder, EstimateBatchGas, DeployTELABatch, IsInSimulatorMode } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { BrowserOpenURL, ClipboardSetText } from '../../../wailsjs/runtime/runtime.js';
  
  export let folderPath = '';
  
  const dispatch = createEventDispatcher();
  
  let files = [];
  let loading = false;
  let deploying = false;
  let deployProgress = { current: 0, total: 0, status: '', fileName: '', phase: 'idle' };
  let error = null;
  let totalSize = 0;
  let totalGas = 0;
  
  // Track per-file deployment status
  let fileStatuses = {}; // { filename: 'pending' | 'deploying' | 'completed' | 'failed' }
  
  // INDEX metadata
  let indexName = '';
  let indexDURL = '';
  let indexDescription = '';
  let indexIcon = '';
  
  // New: Ringsize, compression, and confirmation
  let ringsize = 2; // 2 = updateable, 16+ = immutable
  let enableCompression = false; // Global compression toggle (matching tela-cli)
  let isSimulator = false;
  let showConfirmModal = false;
  
  // Deployment result
  let deploymentResult = null; // { indexScid, deployedDocs, durl }
  
  // Event subscriptions
  let unsubscribeStart, unsubscribeProgress, unsubscribeComplete, unsubscribeError;
  
  onMount(async () => {
    // Check if we're in simulator mode
    try {
      const result = await IsInSimulatorMode();
      isSimulator = result === true;
    } catch (e) {
      isSimulator = false;
    }
    
    // Subscribe to deployment events
    unsubscribeStart = EventsOn('tela:deploy:start', (data) => {
      deployProgress = { 
        current: 0, 
        total: data.totalFiles, 
        status: 'Starting deployment...', 
        fileName: '',
        phase: 'starting'
      };
      // Initialize all files as pending
      fileStatuses = {};
      files.forEach(f => fileStatuses[f.name] = 'pending');
    });
    
    unsubscribeProgress = EventsOn('tela:deploy:progress', (data) => {
      deployProgress = {
        current: data.current,
        total: data.total,
        status: data.status === 'deploying' ? `Deploying ${data.fileName}...` :
                data.status === 'completed' ? `Deployed ${data.fileName}` :
                data.status === 'creating_index' ? 'Creating INDEX...' : data.status,
        fileName: data.fileName,
        phase: data.status
      };
      
      // Update file status
      if (data.status === 'deploying') {
        fileStatuses[data.fileName] = 'deploying';
      } else if (data.status === 'completed') {
        fileStatuses[data.fileName] = 'completed';
      }
      fileStatuses = fileStatuses; // Trigger reactivity
    });
    
    unsubscribeComplete = EventsOn('tela:deploy:complete', (data) => {
      deployProgress = {
        current: data.totalFiles,
        total: data.totalFiles,
        status: 'Deployment complete!',
        fileName: '',
        phase: 'complete'
      };
      deploying = false;
      
      // Store the deployment result for display
      deploymentResult = {
        indexScid: data.indexScid,
        deployedDocs: data.deployedDocs,
        durl: data.durl,
      };
      
      // Show success toast
      toast.success(`Deployment complete! INDEX: ${data.indexScid?.substring(0, 16)}...`);
      
      dispatch('complete', {
        indexScid: data.indexScid,
        deployedDocs: data.deployedDocs,
        durl: data.durl,
      });
    });
    
    unsubscribeError = EventsOn('tela:deploy:error', (data) => {
      error = data.error;
      deploying = false;
      if (data.fileName) {
        fileStatuses[data.fileName] = 'failed';
        fileStatuses = fileStatuses;
      }
      deployProgress.phase = 'error';
    });
  });
  
  onDestroy(() => {
    if (unsubscribeStart) unsubscribeStart();
    if (unsubscribeProgress) unsubscribeProgress();
    if (unsubscribeComplete) unsubscribeComplete();
    if (unsubscribeError) unsubscribeError();
  });
  
  $: if (folderPath) {
    scanFolder();
  }
  
  async function scanFolder() {
    if (!folderPath) return;
    
    loading = true;
    error = null;
    
    try {
      const result = await ScanFolder(folderPath);
      if (result.success) {
        files = result.files || [];
        totalSize = result.totalSize || 0;
        totalGas = result.totalGas || 0;
        
        // Auto-populate index name from folder name
        if (!indexName) {
          const parts = folderPath.split(/[/\\]/);
          indexName = parts[parts.length - 1] || 'My TELA App';
        }
      } else {
        error = result.error;
      }
    } catch (err) {
      error = err.message;
    } finally {
      loading = false;
    }
  }
  
  function updateDocType(index, newType) {
    files[index].docType = newType;
    files = files; // Trigger reactivity
  }
  
  function removeFile(index) {
    files = files.filter((_, i) => i !== index);
    recalculateTotals();
  }
  
  function recalculateTotals() {
    totalSize = files.reduce((sum, f) => sum + (f.size || 0), 0);
    totalGas = files.reduce((sum, f) => sum + (f.gasEstimate || 0), 0);
  }
  
  // Prepare deployment - show confirmation for Mainnet, auto-deploy for Simulator
  function prepareDeploy() {
    // Check wallet - allow in simulator mode or with wallet open
    if (!$walletState.isOpen && !isSimulator) {
      error = 'Please open a wallet first';
      return;
    }
    
    if (!indexName) {
      error = 'Please enter an application name';
      return;
    }
    
    if (files.length === 0) {
      error = 'No files to deploy';
      return;
    }
    
    error = null;
    
    // In simulator mode, deploy immediately (free transactions)
    if (isSimulator) {
      deploy();
    } else {
      // Show confirmation modal for Mainnet
      showConfirmModal = true;
    }
  }
  
  function cancelDeploy() {
    showConfirmModal = false;
  }
  
  async function confirmDeploy() {
    showConfirmModal = false;
    await deploy();
  }
  
  async function deploy() {
    deploying = true;
    error = null;
    deploymentResult = null;
    deployProgress = { current: 0, total: files.length, status: 'Preparing...', fileName: '', phase: 'preparing' };
    
    // Initialize file statuses
    fileStatuses = {};
    files.forEach(f => fileStatuses[f.name] = 'pending');
    
    try {
      const batchData = {
        files: files.map(f => ({
          name: f.name,
          path: f.path,
          subDir: f.subDir,
          docType: f.docType,
          size: f.size,
          compressed: enableCompression && f.canCompress, // Only compress if toggle enabled AND file is compressible
          ringsize: ringsize,
        })),
        indexName: indexName,
        indexDurl: indexDURL,
        description: indexDescription,
        iconUrl: indexIcon,
        ringsize: ringsize,
      };
      
      // Note: Events will handle progress updates and completion
      const result = await DeployTELABatch(JSON.stringify(batchData));
      
      // Fallback if events didn't fire (shouldn't happen)
      if (!result.success && !error) {
        error = result.error;
        deploying = false;
      }
    } catch (err) {
      error = err.message;
      deploying = false;
    }
  }
  
  // Copy SCID to clipboard
  function copyScid(scid) {
    ClipboardSetText(scid);
  }
  
  // Preview deployed INDEX in browser
  function previewIndex(scid) {
    dispatch('preview', { scid, type: 'index' });
  }
  
  // Update SubDir for a file
  function updateSubDir(index, newSubDir) {
    files[index].subDir = newSubDir;
    files = files; // Trigger reactivity
  }
  
  // Reset to deploy another batch
  function resetDeployment() {
    deploymentResult = null;
    deployProgress = { current: 0, total: 0, status: '', fileName: '', phase: 'idle' };
    fileStatuses = {};
  }
  
  function getFileStatus(fileName) {
    return fileStatuses[fileName] || 'pending';
  }
  
  function getStatusIcon(status) {
    switch(status) {
      case 'pending': return '○';
      case 'deploying': return '◎';
      case 'completed': return '✓';
      case 'failed': return '✗';
      default: return '○';
    }
  }
  
  function getStatusColor(status) {
    switch(status) {
      case 'pending': return 'status-pending';
      case 'deploying': return 'status-deploying';
      case 'completed': return 'status-completed';
      case 'failed': return 'status-failed';
      default: return 'status-pending';
    }
  }
  
  function getDocTypeIcon(docType) {
    const icons = {
      // TELA types (from backend)
      'TELA-HTML-1': '◇',
      'TELA-CSS-1': '◈',
      'TELA-JS-1': '⬡',
      'TELA-JSON-1': '□',
      'TELA-MD-1': '◊',
      'TELA-GO-1': '◆',
      'TELA-STATIC-1': '○',
      // Standard MIME types (fallback)
      'text/html': '◇',
      'text/css': '◈',
      'application/javascript': '⬡',
      'application/json': '□',
      'image/svg+xml': '◎',
      'image/png': '◎',
      'image/jpeg': '◎',
      'image/gif': '◎',
      'image/webp': '◎',
      'text/markdown': '◊',
    };
    return icons[docType] || '○';
  }
  
  function getDocTypeLabel(docType) {
    // TELA uses specific type constants like "TELA-CSS-1"
    const labels = {
      // TELA types (from backend)
      'TELA-HTML-1': 'HTML',
      'TELA-CSS-1': 'CSS',
      'TELA-JS-1': 'JS',
      'TELA-JSON-1': 'JSON',
      'TELA-MD-1': 'MD',
      'TELA-GO-1': 'GO',
      'TELA-STATIC-1': 'FILE',
      // Standard MIME types (fallback)
      'text/html': 'HTML',
      'text/css': 'CSS',
      'application/javascript': 'JS',
      'application/json': 'JSON',
      'image/svg+xml': 'SVG',
      'image/png': 'PNG',
      'image/jpeg': 'JPEG',
      'image/gif': 'GIF',
      'image/webp': 'WebP',
      'text/markdown': 'MD',
      'application/octet-stream': 'BIN',
    };
    return labels[docType] || 'FILE';
  }
  
  function getDocTypeClass(docType) {
    // Return CSS class for color-coding file types
    const classes = {
      'TELA-HTML-1': 'type-html',
      'TELA-CSS-1': 'type-css',
      'TELA-JS-1': 'type-js',
      'TELA-JSON-1': 'type-json',
      'TELA-MD-1': 'type-md',
      'TELA-GO-1': 'type-go',
      'TELA-STATIC-1': 'type-static',
    };
    return classes[docType] || 'type-static';
  }
  
  // #2: Determine if file type detection is confident (extension matches type)
  function isConfidentDetection(filename, docType) {
    const ext = filename.split('.').pop()?.toLowerCase();
    const confidentMappings = {
      'html': 'TELA-HTML-1',
      'htm': 'TELA-HTML-1',
      'css': 'TELA-CSS-1',
      'js': 'TELA-JS-1',
      'mjs': 'TELA-JS-1',
      'json': 'TELA-JSON-1',
      'md': 'TELA-MD-1',
      'markdown': 'TELA-MD-1',
      'go': 'TELA-GO-1',
    };
    return confidentMappings[ext] === docType;
  }
  
  // #3: Get tooltip text explaining why a file is marked as entry point
  function getEntryPointTooltip(filename) {
    const name = filename.toLowerCase();
    if (name === 'index.html' || name === 'index.htm') {
      return 'Auto-detected: index.html is the default TELA entry point';
    }
    return 'Marked as application entry point';
  }
  
  function formatSize(bytes) {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(2) + ' MB';
  }
  
  // Check if any files can benefit from compression (text-based types)
  function hasCompressibleFiles() {
    return files.some(f => f.canCompress === true);
  }
</script>

<div class="batch-upload">
  <!-- Folder Info -->
  <div class="folder-info">
    <div class="folder-info-content">
      <div>
        <span class="folder-label">Source Folder</span>
        <p class="folder-path">{folderPath}</p>
      </div>
      <button 
        on:click={scanFolder}
        disabled={loading}
        class="btn-rescan"
      >
        {loading ? 'Scanning...' : '↻ Rescan'}
      </button>
    </div>
  </div>
  
  <!-- Error Display -->
  {#if error}
    <div class="alert-error">
      <span class="alert-icon">!</span> {error}
    </div>
  {/if}
  
  <!-- Files List -->
  {#if loading}
    <div class="loading-state">
      <div class="spinner"></div>
      <p class="loading-text">Scanning folder...</p>
    </div>
  {:else if files.length > 0}
    <div class="files-container">
      <div class="files-header">
        <span class="files-count">{files.length} files • {formatSize(totalSize)}</span>
        <span class="files-gas">~{totalGas.toLocaleString()} gas</span>
      </div>
      
      <div class="files-list">
        {#each files as file, i}
          {@const status = getFileStatus(file.name)}
          <div class="file-row {status === 'deploying' ? 'highlight-deploying' : status === 'completed' ? 'highlight-completed' : ''}">
            {#if deploying}
              <span class="file-icon {getStatusColor(status)}">{getStatusIcon(status)}</span>
            {:else}
              <span class="file-icon">{getDocTypeIcon(file.docType)}</span>
            {/if}
            
            <div class="file-info">
              <div class="file-name-row">
                <span class="file-name">{file.name}</span>
                {#if file.isEntryPoint}
                  <span class="badge-entry" title={getEntryPointTooltip(file.name)}>
                    <span class="entry-icon">◇</span> Entry
                  </span>
                {/if}
                {#if deploying && status === 'deploying'}
                  <span class="badge-deploying">Deploying</span>
                {/if}
              </div>
              <div class="file-meta">
                {#if !deploying}
                  <input
                    type="text"
                    value={file.subDir}
                    on:input={(e) => updateSubDir(i, e.target.value)}
                    class="subdir-input"
                    placeholder="/"
                    title="SubDir path"
                  />
                {:else}
                  <span class="subdir-display">{file.subDir || '/'}</span>
                {/if}
                <span class="file-size">• {formatSize(file.size)}</span>
              </div>
            </div>
            
            <!-- #5: Compression indicator -->
            {#if enableCompression && file.canCompress}
              <span class="compress-badge" title="Will be gzip compressed for smaller on-chain size">
                <span class="compress-icon">Z</span>
              </span>
            {/if}
            
            <!-- #2: Only show dropdown for non-confident detections -->
            {#if isConfidentDetection(file.name, file.docType)}
              <!-- Confident detection: just show badge, no dropdown -->
              <span 
                class="file-type-badge {getDocTypeClass(file.docType)} confident" 
                title="Auto-detected from .{file.name.split('.').pop()} extension"
              >
                {getDocTypeLabel(file.docType)}
              </span>
            {:else}
              <!-- Ambiguous: show badge + dropdown -->
              <div class="file-type-wrapper">
                <span 
                  class="file-type-badge {getDocTypeClass(file.docType)}" 
                  title="Type may need verification"
                >
                  {getDocTypeLabel(file.docType)}
                </span>
                <select
                  value={file.docType}
                  on:change={(e) => updateDocType(i, e.target.value)}
                  disabled={deploying}
                  class="file-type-select"
                  title="Select correct file type"
                >
                  <option value="TELA-HTML-1">HTML</option>
                  <option value="TELA-CSS-1">CSS</option>
                  <option value="TELA-JS-1">JavaScript</option>
                  <option value="TELA-JSON-1">JSON</option>
                  <option value="TELA-MD-1">Markdown</option>
                  <option value="TELA-GO-1">Go</option>
                  <option value="TELA-STATIC-1">Static/Binary</option>
                </select>
              </div>
            {/if}
            
            <button
              on:click={() => removeFile(i)}
              disabled={deploying}
              class="file-remove-btn"
            >✕</button>
          </div>
        {/each}
      </div>
    </div>
    
    <!-- Deployment Progress Bar -->
    {#if deploying}
      <div class="progress-card">
        <div class="progress-header">
          <span class="progress-status">{deployProgress.status}</span>
          <span class="progress-count">{deployProgress.current}/{deployProgress.total}</span>
        </div>
        <div class="progress-bar-bg">
          <div 
            class="progress-bar-fill"
            style="width: {deployProgress.total > 0 ? (deployProgress.current / deployProgress.total) * 100 : 0}%"
          ></div>
        </div>
        {#if deployProgress.phase === 'creating_index'}
          <p class="progress-note success">Creating INDEX smart contract...</p>
        {:else if deployProgress.phase === 'complete'}
          <p class="progress-note success">✓ All files deployed successfully!</p>
        {/if}
      </div>
    {/if}
    
    <!-- INDEX Configuration -->
    <div class="config-card {deploying ? 'disabled' : ''}">
      <h3 class="config-title">INDEX Configuration</h3>
      
      <div class="config-grid">
        <div class="config-field">
          <label class="config-label">Application Name <span class="required">*</span></label>
          <input
            type="text"
            bind:value={indexName}
            placeholder="My TELA App"
            disabled={deploying}
            class="config-input"
          />
        </div>
        
        <div class="config-field">
          <label class="config-label">dURL (optional)</label>
          <input
            type="text"
            bind:value={indexDURL}
            placeholder="my-app"
            disabled={deploying}
            class="config-input"
          />
        </div>
      </div>
      
      <div class="config-field">
        <label class="config-label">Description</label>
        <textarea
          bind:value={indexDescription}
          placeholder="Describe your application..."
          rows="2"
          disabled={deploying}
          class="config-textarea"
        ></textarea>
      </div>
      
      <div class="config-field">
        <label class="config-label">Icon URL (optional)</label>
        <input
          type="text"
          bind:value={indexIcon}
          placeholder="https://example.com/icon.png"
          disabled={deploying}
          class="config-input"
        />
      </div>
      
      <!-- Ringsize Selector -->
      <div class="config-field">
        <label class="config-label">Content Type</label>
        <div class="ringsize-selector">
          <button
            type="button"
            class="ringsize-btn {ringsize === 2 ? 'active' : ''}"
            on:click={() => ringsize = 2}
            disabled={deploying}
          >
            <span class="ringsize-icon">↻</span>
            <span class="ringsize-label">Updateable</span>
            <span class="ringsize-desc">Ring 2 • Can be modified later</span>
          </button>
          <button
            type="button"
            class="ringsize-btn {ringsize === 16 ? 'active immutable' : ''}"
            on:click={() => ringsize = 16}
            disabled={deploying}
          >
            <span class="ringsize-icon">◆</span>
            <span class="ringsize-label">Immutable</span>
            <span class="ringsize-desc">Ring 16 • Permanent, cannot change</span>
          </button>
        </div>
      </div>
      
      <!-- Compression Toggle (matching tela-cli) -->
      {#if hasCompressibleFiles()}
        <div class="config-field">
          <label class="config-label">Compression</label>
          <button 
            type="button"
            class="compression-toggle {enableCompression ? 'active' : ''}"
            on:click={() => enableCompression = !enableCompression}
            disabled={deploying}
          >
            <div class="compression-track">
              <div class="compression-thumb"></div>
            </div>
            <div class="compression-content">
              <span class="compression-label">{enableCompression ? 'Enabled' : 'Disabled'}</span>
              <span class="compression-desc">
                {enableCompression 
                  ? 'Text files will be gzip compressed (smaller on-chain size)' 
                  : 'Files stored uncompressed'}
              </span>
            </div>
          </button>
          {#if enableCompression}
            <p class="compression-note">
              ◈ HTML, CSS, JS, JSON, MD, and Go files will be gzip compressed before deployment
            </p>
          {/if}
        </div>
      {/if}
    </div>
    
    <!-- Deploy Button -->
    <div class="deploy-row">
      <div class="deploy-cost">
        {#if isSimulator}
          <span class="simulator-badge">SIMULATOR</span>
          <span class="free-badge">FREE</span>
        {:else}
          Estimated cost: <span class="cost-value">~{totalGas.toLocaleString()} gas</span>
        {/if}
      </div>
      
      <button
        on:click={prepareDeploy}
        disabled={deploying || (!$walletState.isOpen && !isSimulator) || !indexName}
        class="btn-deploy"
      >
        {#if deploying}
          <div class="btn-spinner"></div>
          Deploying... ({deployProgress.current}/{deployProgress.total})
        {:else}
          Deploy {files.length} Files + INDEX
        {/if}
      </button>
    </div>
    
    {#if !$walletState.isOpen && !isSimulator && !deploying}
      <p class="wallet-warning">
        <span class="warn-icon">!</span> Please open a wallet to deploy
      </p>
    {/if}
    
    <!-- Deployment Success Display -->
    {#if deploymentResult}
      <div class="success-card">
        <div class="success-header">
          <span class="success-icon">✓</span>
          <h3 class="success-title">Deployment Complete!</h3>
        </div>
        
        <div class="result-section">
          <div class="result-label">INDEX SCID</div>
          <div class="result-scid-row">
            <code class="result-scid">{deploymentResult.indexScid}</code>
            <button class="btn-copy" on:click={() => {
              copyScid(deploymentResult.indexScid);
              toast.success('INDEX SCID copied to clipboard');
            }} title="Copy full SCID">
              ◎
            </button>
            <button class="btn-preview" on:click={() => previewIndex(deploymentResult.indexScid)} title="Preview">
              ▶
            </button>
          </div>
        </div>
        
        {#if deploymentResult.durl}
          <div class="result-section">
            <div class="result-label">dURL</div>
            <code class="result-durl">{deploymentResult.durl}</code>
          </div>
        {/if}
        
        <div class="result-section">
          <div class="result-label">Deployed DOCs ({deploymentResult.deployedDocs?.length || 0})</div>
          <div class="deployed-docs-list">
            {#each (deploymentResult.deployedDocs || []) as doc}
              <div class="deployed-doc-row">
                <span class="deployed-doc-name">{doc.name}</span>
                <code class="deployed-doc-scid" title={doc.scid}>{doc.scid?.substring(0, 32)}...</code>
                <button class="btn-copy-sm" on:click={() => {
                  copyScid(doc.scid);
                  toast.success(`Copied ${doc.name} SCID`);
                }} title="Copy full SCID">
                  ◎
                </button>
              </div>
            {/each}
          </div>
        </div>
        
        <div class="success-actions">
          <button class="btn-reset" on:click={resetDeployment}>
            Deploy Another Batch
          </button>
          <button class="btn-copy-index" on:click={() => copyScid(deploymentResult.indexScid)} title="Copy INDEX SCID">
            Copy INDEX SCID
          </button>
          {#if deploymentResult.durl}
            <button class="btn-preview-index" on:click={() => previewIndex(deploymentResult.indexScid)} title="Preview in Browser">
              Preview in Browser
            </button>
          {/if}
        </div>
      </div>
    {/if}
  {:else}
    <div class="empty-state">
      <span class="empty-icon">◇</span>
      <p class="empty-text">No files found in folder</p>
    </div>
  {/if}
</div>

<!-- Confirmation Modal for Mainnet Deployment -->
{#if showConfirmModal}
  <div class="modal-overlay" on:click={cancelDeploy}>
    <div class="modal-content" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-icon">⚠</span>
        <h3 class="modal-title">Confirm Deployment</h3>
      </div>
      
      <div class="modal-body">
        <p class="modal-warning">
          You are about to deploy to <strong>Mainnet</strong>. This transaction is <strong>permanent</strong> and will consume DERO.
        </p>
        
        <div class="modal-details">
          <div class="modal-detail-row">
            <span class="detail-label">Files</span>
            <span class="detail-value">{files.length} DOCs + 1 INDEX</span>
          </div>
          <div class="modal-detail-row">
            <span class="detail-label">Total Size</span>
            <span class="detail-value">{formatSize(totalSize)}</span>
          </div>
          <div class="modal-detail-row">
            <span class="detail-label">Estimated Gas</span>
            <span class="detail-value cost">~{totalGas.toLocaleString()}</span>
          </div>
          <div class="modal-detail-row">
            <span class="detail-label">Type</span>
            <span class="detail-value">{ringsize === 2 ? 'Updateable (Ring 2)' : 'Immutable (Ring 16)'}</span>
          </div>
        </div>
      </div>
      
      <div class="modal-actions">
        <button class="btn-cancel" on:click={cancelDeploy}>Cancel</button>
        <button class="btn-confirm" on:click={confirmDeploy}>Deploy to Mainnet</button>
      </div>
    </div>
  </div>
{/if}

<style>
  /* === HOLOGRAM v6.1 Batch Upload === */
  
  .batch-upload {
    display: flex;
    flex-direction: column;
    gap: var(--s-6, 24px);
  }
  
  /* Folder Info */
  .folder-info {
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
  }
  
  .folder-info-content {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .folder-label {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .folder-path {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 13px;
    color: var(--text-2, #a8a8b8);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .btn-rescan {
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-up, #181824);
    color: var(--text-3, #707088);
    border-radius: var(--r-md, 8px);
    border: none;
    font-size: 13px;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-rescan:hover {
    background: var(--void-surface, #1e1e2a);
    color: var(--text-2, #a8a8b8);
  }
  
  .btn-rescan:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  /* Alert Error */
  .alert-error {
    padding: var(--s-4, 16px);
    background: rgba(248, 113, 113, 0.1);
    border: 1px solid rgba(248, 113, 113, 0.3);
    border-radius: var(--r-lg, 12px);
    color: var(--status-err, #f87171);
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .alert-icon {
    font-weight: 700;
  }
  
  /* Loading State */
  .loading-state {
    text-align: center;
    padding: var(--s-8, 32px);
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
  
  .loading-text {
    color: var(--text-4, #505068);
  }
  
  /* Files Container */
  .files-container {
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-lg, 12px);
    overflow: hidden;
  }
  
  .files-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-2, 8px) var(--s-4, 16px);
    background: var(--void-mid, #12121c);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .files-count {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .files-gas {
    font-size: 13px;
    color: var(--cyan-400, #22d3ee);
  }
  
  .files-list {
    max-height: 256px;
    overflow-y: auto;
  }
  
  /* File Row */
  .file-row {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    padding: var(--s-3, 12px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    transition: background 200ms ease-out;
  }
  
  .file-row:last-child {
    border-bottom: none;
  }
  
  .file-row:hover {
    background: rgba(18, 18, 28, 0.5);
  }
  
  .file-row.highlight-deploying {
    background: rgba(251, 191, 36, 0.05);
  }
  
  .file-row.highlight-completed {
    background: rgba(52, 211, 153, 0.05);
  }
  
  .file-icon {
    font-size: 16px;
    color: var(--text-3, #707088);
    width: 24px;
    text-align: center;
  }
  
  .file-icon.status-pending {
    color: var(--text-5, #404058);
  }
  
  .file-icon.status-deploying {
    color: var(--status-warn, #fbbf24);
  }
  
  .file-icon.status-completed {
    color: var(--status-ok, #34d399);
  }
  
  .file-icon.status-failed {
    color: var(--status-err, #f87171);
  }
  
  .file-info {
    flex: 1;
    min-width: 0;
  }
  
  .file-name-row {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .file-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  /* #3: Entry Point Badge - improved with icon and tooltip */
  .badge-entry {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    font-size: 10px;
    font-weight: 600;
    background: rgba(6, 182, 212, 0.15);
    border: 1px solid rgba(6, 182, 212, 0.3);
    color: var(--cyan-400, #22d3ee);
    border-radius: var(--r-xs, 3px);
    cursor: help;
  }
  
  .entry-icon {
    font-size: 10px;
    opacity: 0.8;
  }
  
  .badge-deploying {
    padding: 2px 6px;
    font-size: 10px;
    background: rgba(251, 191, 36, 0.2);
    color: var(--status-warn, #fbbf24);
    border-radius: var(--r-xs, 3px);
    animation: pulse 1.5s ease-in-out infinite;
  }
  
  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  
  .file-meta {
    font-size: 12px;
    color: var(--text-5, #404058);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  /* File Type Select - HOLOGRAM Design System Compliant */
  .file-type-select {
    padding: var(--s-1, 4px) var(--s-2, 8px);
    padding-right: 28px; /* Room for dropdown arrow */
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 11px;
    color: var(--text-3, #707088);
    background: var(--void-deep, #08080e);
    /* Custom dropdown arrow - HOLOGRAM standard */
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'%3E%3Cpath fill='%23707088' d='M2 4l4 4 4-4'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 6px center;
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-sm, 5px);
    /* CRITICAL: Remove native OS styling */
    appearance: none;
    -webkit-appearance: none;
    -moz-appearance: none;
    cursor: pointer;
    outline: none;
    transition: all 150ms ease;
  }
  
  .file-type-select:hover {
    border-color: var(--border-subtle, rgba(255, 255, 255, 0.06));
  }
  
  .file-type-select:focus {
    border-color: var(--cyan-500, #06b6d4);
    box-shadow: 0 0 0 2px rgba(34, 211, 238, 0.15);
  }
  
  .file-type-select:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    background-color: var(--void-mid, #12121c);
  }
  
  .file-type-select option {
    background: var(--void-deep, #08080e);
    color: var(--text-1, #f8f8fc);
  }
  
  /* File Type Wrapper - shows badge + dropdown */
  .file-type-wrapper {
    display: flex;
    align-items: center;
    gap: var(--s-1, 4px);
  }
  
  /* File Type Badge - color-coded by type */
  .file-type-badge {
    padding: 3px 8px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-radius: var(--r-xs, 3px);
    cursor: help;
    white-space: nowrap;
  }
  
  /* Type-specific colors */
  .file-type-badge.type-html {
    background: rgba(34, 211, 238, 0.15);
    border: 1px solid rgba(34, 211, 238, 0.3);
    color: var(--cyan-400, #22d3ee);
  }
  
  .file-type-badge.type-css {
    background: rgba(167, 139, 250, 0.15);
    border: 1px solid rgba(167, 139, 250, 0.3);
    color: var(--violet-400, #a78bfa);
  }
  
  .file-type-badge.type-js {
    background: rgba(251, 191, 36, 0.15);
    border: 1px solid rgba(251, 191, 36, 0.3);
    color: var(--status-warn, #fbbf24);
  }
  
  .file-type-badge.type-json {
    background: rgba(52, 211, 153, 0.12);
    border: 1px solid rgba(52, 211, 153, 0.25);
    color: var(--emerald-400, #34d399);
  }
  
  .file-type-badge.type-md {
    background: rgba(168, 168, 184, 0.1);
    border: 1px solid rgba(168, 168, 184, 0.2);
    color: var(--text-2, #a8a8b8);
  }
  
  .file-type-badge.type-go {
    background: rgba(34, 211, 238, 0.12);
    border: 1px solid rgba(34, 211, 238, 0.25);
    color: var(--cyan-300, #67e8f9);
  }
  
  .file-type-badge.type-static {
    background: rgba(80, 80, 104, 0.15);
    border: 1px solid rgba(80, 80, 104, 0.3);
    color: var(--text-4, #505068);
  }
  
  /* #2: Confident detection badge - subtle checkmark indicator */
  .file-type-badge.confident {
    position: relative;
    cursor: help;
  }
  
  .file-type-badge.confident::after {
    content: '✓';
    margin-left: 4px;
    font-size: 9px;
    opacity: 0.6;
  }
  
  /* #5: Compression eligibility badge */
  .compress-badge {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    background: rgba(6, 182, 212, 0.12);
    border: 1px solid rgba(6, 182, 212, 0.25);
    border-radius: var(--r-xs, 3px);
    cursor: help;
  }
  
  .compress-icon {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 10px;
    font-weight: 700;
    color: var(--cyan-400, #22d3ee);
    line-height: 1;
  }
  
  .file-remove-btn {
    color: var(--text-5, #404058);
    background: transparent;
    border: none;
    cursor: pointer;
    padding: var(--s-1, 4px);
    transition: color 200ms ease-out;
  }
  
  .file-remove-btn:hover {
    color: var(--status-err, #f87171);
  }
  
  .file-remove-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }
  
  /* Progress Card */
  .progress-card {
    padding: var(--s-5, 20px);
    background: var(--void-mid, #12121c);
    border: 1px solid rgba(6, 182, 212, 0.3);
    border-radius: var(--r-xl, 16px);
  }
  
  .progress-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2, 8px);
  }
  
  .progress-status {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .progress-count {
    font-size: 13px;
    color: var(--cyan-400, #22d3ee);
  }
  
  .progress-bar-bg {
    width: 100%;
    height: 12px;
    background: var(--void-deep, #08080e);
    border-radius: var(--r-full, 9999px);
    overflow: hidden;
  }
  
  .progress-bar-fill {
    height: 100%;
    background: var(--cyan-500, #06b6d4);
    border-radius: var(--r-full, 9999px);
    transition: width 300ms ease-out;
  }
  
  .progress-note {
    font-size: 12px;
    text-align: center;
    margin-top: var(--s-2, 8px);
  }
  
  .progress-note.success {
    color: var(--status-ok, #34d399);
  }
  
  /* Config Card */
  .config-card {
    padding: var(--s-5, 20px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .config-card.disabled {
    opacity: 0.5;
    pointer-events: none;
  }
  
  .config-title {
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .config-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4, 16px);
  }
  
  .config-field {
    display: flex;
    flex-direction: column;
    gap: var(--s-1, 4px);
  }
  
  .config-label {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .config-input,
  .config-textarea {
    width: 100%;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-lg, 12px);
    color: var(--text-1, #f8f8fc);
    font-size: 13px;
    outline: none;
    transition: border-color 200ms ease-out;
  }
  
  .config-input::placeholder,
  .config-textarea::placeholder {
    color: var(--text-5, #404058);
  }
  
  .config-input:focus,
  .config-textarea:focus {
    border-color: var(--cyan-500, #06b6d4);
  }
  
  .config-input:disabled,
  .config-textarea:disabled {
    opacity: 0.5;
  }
  
  .config-textarea {
    resize: none;
  }
  
  /* Deploy Row */
  .deploy-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .deploy-cost {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .cost-value {
    color: var(--cyan-400, #22d3ee);
    font-weight: 500;
  }
  
  .btn-deploy {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px) var(--s-6, 24px);
    background: var(--cyan-500, #06b6d4);
    color: var(--void-pure, #000000);
    border-radius: var(--r-lg, 12px);
    font-weight: 600;
    border: none;
    cursor: pointer;
    transition: background 200ms ease-out;
  }
  
  .btn-deploy:hover {
    background: var(--cyan-400, #22d3ee);
  }
  
  .btn-deploy:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .btn-spinner {
    width: 16px;
    height: 16px;
    border: 2px solid var(--void-pure, #000000);
    border-top-color: transparent;
    border-radius: var(--r-full, 9999px);
    animation: spin 0.6s linear infinite;
  }
  
  /* Wallet Warning */
  .wallet-warning {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2, 8px);
    font-size: 13px;
    color: var(--status-warn, #fbbf24);
    text-align: center;
  }
  
  .warn-icon {
    font-weight: 700;
  }
  
  /* Empty State */
  .empty-state {
    text-align: center;
    padding: var(--s-8, 32px);
    color: var(--text-4, #505068);
  }
  
  .empty-icon {
    font-size: 40px;
    display: block;
    margin-bottom: var(--s-2, 8px);
  }
  
  .empty-text {
    font-size: 13px;
  }
  
  /* SubDir Input */
  .subdir-input {
    width: 80px;
    padding: 2px 6px;
    font-size: 11px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xs, 3px);
    color: var(--text-3, #707088);
    outline: none;
  }
  
  .subdir-input:focus {
    border-color: var(--cyan-500, #06b6d4);
  }
  
  .subdir-display {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-5, #404058);
  }
  
  .file-size {
    color: var(--text-5, #404058);
  }
  
  .file-meta {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  /* Required field indicator */
  .required {
    color: var(--status-err, #f87171);
  }
  
  /* Ringsize Selector */
  .ringsize-selector {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3, 12px);
  }
  
  .ringsize-btn {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-lg, 12px);
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .ringsize-btn:hover {
    border-color: var(--border-subtle, rgba(255, 255, 255, 0.06));
    background: var(--void-up, #181824);
  }
  
  .ringsize-btn.active {
    border-color: var(--cyan-500, #06b6d4);
    background: rgba(6, 182, 212, 0.1);
  }
  
  .ringsize-btn.active.immutable {
    border-color: var(--violet-400, #a78bfa);
    background: rgba(167, 139, 250, 0.1);
  }
  
  .ringsize-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .ringsize-icon {
    font-size: 20px;
    margin-bottom: var(--s-1, 4px);
    color: var(--text-3, #707088);
  }
  
  .ringsize-btn.active .ringsize-icon {
    color: var(--cyan-400, #22d3ee);
  }
  
  .ringsize-btn.active.immutable .ringsize-icon {
    color: var(--violet-400, #a78bfa);
  }
  
  .ringsize-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .ringsize-desc {
    font-size: 11px;
    color: var(--text-5, #404058);
    text-align: center;
  }
  
  /* Compression Toggle (matching tela-cli) */
  .compression-toggle {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    width: 100%;
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-lg, 12px);
    cursor: pointer;
    transition: all 200ms ease-out;
    text-align: left;
  }
  
  .compression-toggle:hover {
    border-color: var(--border-subtle, rgba(255, 255, 255, 0.06));
    background: var(--void-up, #181824);
  }
  
  .compression-toggle.active {
    border-color: var(--cyan-500, #06b6d4);
    background: rgba(6, 182, 212, 0.08);
  }
  
  .compression-toggle:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .compression-track {
    width: 36px;
    height: 20px;
    background: var(--void-surface, #1e1e2a);
    border-radius: 10px;
    position: relative;
    transition: background 200ms ease-out;
    flex-shrink: 0;
  }
  
  .compression-toggle.active .compression-track {
    background: var(--cyan-500, #06b6d4);
  }
  
  .compression-thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    background: var(--text-4, #505068);
    border-radius: 50%;
    transition: all 200ms ease-out;
  }
  
  .compression-toggle.active .compression-thumb {
    left: 18px;
    background: #fff;
  }
  
  .compression-content {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .compression-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .compression-toggle.active .compression-label {
    color: var(--cyan-400, #22d3ee);
  }
  
  .compression-desc {
    font-size: 11px;
    color: var(--text-5, #404058);
  }
  
  .compression-note {
    margin-top: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(6, 182, 212, 0.05);
    border-radius: var(--r-md, 8px);
    font-size: 11px;
    color: var(--cyan-400, #22d3ee);
  }
  
  /* Simulator & Free Badges */
  .simulator-badge {
    padding: 2px 8px;
    font-size: 11px;
    font-weight: 600;
    background: rgba(251, 191, 36, 0.2);
    color: var(--status-warn, #fbbf24);
    border-radius: var(--r-sm, 5px);
    margin-right: var(--s-2, 8px);
  }
  
  .free-badge {
    padding: 2px 8px;
    font-size: 11px;
    font-weight: 600;
    background: rgba(52, 211, 153, 0.2);
    color: var(--status-ok, #34d399);
    border-radius: var(--r-sm, 5px);
  }
  
  /* Success Card */
  .success-card {
    padding: var(--s-5, 20px);
    background: var(--void-mid, #12121c);
    border: 1px solid rgba(52, 211, 153, 0.3);
    border-radius: var(--r-xl, 16px);
  }
  
  .success-header {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-4, 16px);
  }
  
  .success-icon {
    font-size: 24px;
    color: var(--status-ok, #34d399);
  }
  
  .success-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1, #f8f8fc);
  }
  
  .result-section {
    margin-bottom: var(--s-4, 16px);
  }
  
  .result-label {
    font-size: 12px;
    color: var(--text-4, #505068);
    margin-bottom: var(--s-1, 4px);
  }
  
  .result-scid-row {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .result-scid {
    flex: 1;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    color: var(--cyan-400, #22d3ee);
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .result-durl {
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    color: var(--text-2, #a8a8b8);
  }
  
  .btn-copy, .btn-preview {
    padding: var(--s-2, 8px);
    background: var(--void-up, #181824);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-md, 8px);
    color: var(--text-3, #707088);
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-copy:hover, .btn-preview:hover {
    background: var(--void-surface, #1e1e2a);
    color: var(--cyan-400, #22d3ee);
  }
  
  .deployed-docs-list {
    max-height: 150px;
    overflow-y: auto;
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    padding: var(--s-2, 8px);
  }
  
  .deployed-doc-row {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-1, 4px) var(--s-2, 8px);
    border-radius: var(--r-sm, 5px);
  }
  
  .deployed-doc-row:hover {
    background: rgba(18, 18, 28, 0.5);
  }
  
  .deployed-doc-name {
    flex: 1;
    font-size: 12px;
    color: var(--text-2, #a8a8b8);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .deployed-doc-scid {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 11px;
    color: var(--text-4, #505068);
  }
  
  .btn-copy-sm {
    padding: 2px 6px;
    background: transparent;
    border: none;
    color: var(--text-5, #404058);
    cursor: pointer;
    font-size: 12px;
    transition: color 200ms ease-out;
  }
  
  .btn-copy-sm:hover {
    color: var(--cyan-400, #22d3ee);
  }
  
  .success-actions {
    display: flex;
    gap: var(--s-2, 8px);
    margin-top: var(--s-4, 16px);
    flex-wrap: wrap;
  }
  
  .btn-reset {
    flex: 1;
    min-width: 140px;
    padding: var(--s-3, 12px);
    background: var(--void-up, #181824);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-lg, 12px);
    color: var(--text-2, #a8a8b8);
    font-size: 13px;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-reset:hover {
    background: var(--void-surface, #1e1e2a);
    border-color: var(--cyan-500, #06b6d4);
    color: var(--cyan-400, #22d3ee);
  }
  
  .btn-copy-index,
  .btn-preview-index {
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-up, #181824);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-lg, 12px);
    color: var(--text-2, #a8a8b8);
    font-size: 13px;
    cursor: pointer;
    transition: all 200ms ease-out;
    white-space: nowrap;
  }
  
  .btn-copy-index:hover,
  .btn-preview-index:hover {
    background: var(--void-surface, #1e1e2a);
    border-color: var(--cyan-500, #06b6d4);
    color: var(--cyan-400, #22d3ee);
  }
  
  /* Confirmation Modal */
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.8);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    backdrop-filter: blur(4px);
  }
  
  .modal-content {
    width: 100%;
    max-width: 420px;
    padding: var(--s-6, 24px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-default, rgba(255, 255, 255, 0.09));
    border-radius: var(--r-xl, 16px);
  }
  
  .modal-header {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-4, 16px);
  }
  
  .modal-icon {
    font-size: 24px;
    color: var(--status-warn, #fbbf24);
  }
  
  .modal-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-1, #f8f8fc);
  }
  
  .modal-body {
    margin-bottom: var(--s-5, 20px);
  }
  
  .modal-warning {
    font-size: 13px;
    color: var(--text-3, #707088);
    line-height: 1.5;
    margin-bottom: var(--s-4, 16px);
  }
  
  .modal-warning strong {
    color: var(--text-1, #f8f8fc);
  }
  
  .modal-details {
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
    padding: var(--s-3, 12px);
  }
  
  .modal-detail-row {
    display: flex;
    justify-content: space-between;
    padding: var(--s-2, 8px) 0;
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .modal-detail-row:last-child {
    border-bottom: none;
  }
  
  .detail-label {
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .detail-value {
    font-size: 13px;
    color: var(--text-2, #a8a8b8);
  }
  
  .detail-value.cost {
    color: var(--cyan-400, #22d3ee);
    font-weight: 500;
  }
  
  .modal-actions {
    display: flex;
    gap: var(--s-3, 12px);
  }
  
  .btn-cancel {
    flex: 1;
    padding: var(--s-3, 12px);
    background: var(--void-up, #181824);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-lg, 12px);
    color: var(--text-3, #707088);
    font-size: 13px;
    cursor: pointer;
    transition: all 200ms ease-out;
  }
  
  .btn-cancel:hover {
    background: var(--void-surface, #1e1e2a);
    color: var(--text-2, #a8a8b8);
  }
  
  .btn-confirm {
    flex: 1;
    padding: var(--s-3, 12px);
    background: var(--cyan-500, #06b6d4);
    border: none;
    border-radius: var(--r-lg, 12px);
    color: var(--void-pure, #000000);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: background 200ms ease-out;
  }
  
  .btn-confirm:hover {
    background: var(--cyan-400, #22d3ee);
  }
</style>

