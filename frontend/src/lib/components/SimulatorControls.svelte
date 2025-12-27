<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { 
    StartSimulatorMode, 
    StopSimulatorMode, 
    GetSimulatorStatus,
    MineSimulatorBlocks,
    GenerateTestDERO,
    ResetSimulator
  } from '../../../wailsjs/go/main/App.js';
  import { DotIndicator } from './holo';
  import Icons from './Icons.svelte';
  import { 
    Play, Square, Pickaxe, Coins, 
    AlertTriangle, CheckCircle, Loader2, Trash2
  } from 'lucide-svelte';
  
  const dispatch = createEventDispatcher();
  
  // Simulator state
  let status = {
    isInitialized: false,
    isStarting: false,
    daemonRunning: false,
    walletOpen: false,
    walletAddress: '',
    balance: 0,
    balanceDERO: 0,
    blockHeight: 0
  };
  
  let isLoading = false;
  let error = '';
  let successMessage = '';
  let blocksToMine = 10;
  let targetDERO = 100;
  
  // Status polling
  let statusInterval;
  
  onMount(async () => {
    await refreshStatus();
    statusInterval = setInterval(refreshStatus, 3000);
  });
  
  onDestroy(() => {
    if (statusInterval) clearInterval(statusInterval);
  });
  
  async function refreshStatus() {
    try {
      const result = await GetSimulatorStatus();
      if (result.success) {
        status = { ...status, ...result };
      }
    } catch (e) {
      console.error('Failed to get simulator status:', e);
    }
  }
  
  async function startSimulator() {
    isLoading = true;
    error = '';
    successMessage = '';
    
    try {
      const result = await StartSimulatorMode();
      if (result.success) {
        successMessage = 'Simulator mode activated!';
        await refreshStatus();
        dispatch('simulatorStarted', result);
      } else {
        error = result.error || 'Failed to start simulator';
      }
    } catch (e) {
      error = e.message || 'Failed to start simulator';
    } finally {
      isLoading = false;
    }
  }
  
  async function stopSimulator() {
    isLoading = true;
    error = '';
    
    try {
      const result = await StopSimulatorMode();
      if (result.success) {
        successMessage = 'Simulator stopped';
        await refreshStatus();
        dispatch('simulatorStopped');
      } else {
        error = result.error || 'Failed to stop simulator';
      }
    } catch (e) {
      error = e.message || 'Failed to stop simulator';
    } finally {
      isLoading = false;
    }
  }
  
  async function mineBlocks() {
    isLoading = true;
    error = '';
    
    try {
      const result = await MineSimulatorBlocks(blocksToMine);
      if (result.success) {
        successMessage = `Mined ${result.blocksGenerated} blocks`;
        await refreshStatus();
      } else {
        error = result.error || 'Mining failed';
      }
    } catch (e) {
      error = e.message || 'Mining failed';
    } finally {
      isLoading = false;
    }
  }
  
  async function generateDERO() {
    isLoading = true;
    error = '';
    
    try {
      // Convert DERO to atomic units (1e12)
      const targetAtomic = BigInt(targetDERO) * BigInt(1e12);
      const result = await GenerateTestDERO(Number(targetAtomic));
      if (result.success) {
        successMessage = `Generated test DERO`;
        await refreshStatus();
      } else {
        error = result.error || 'Generation failed';
      }
    } catch (e) {
      error = e.message || 'Generation failed';
    } finally {
      isLoading = false;
    }
  }
  
  async function resetSim() {
    if (!confirm('This will delete all simulator data and start fresh. Continue?')) {
      return;
    }
    
    isLoading = true;
    error = '';
    
    try {
      const result = await ResetSimulator();
      if (result.success) {
        successMessage = 'Simulator reset complete';
        await refreshStatus();
      } else {
        error = result.error || 'Reset failed';
      }
    } catch (e) {
      error = e.message || 'Reset failed';
    } finally {
      isLoading = false;
    }
  }
  
  function formatAddress(addr) {
    if (!addr) return '—';
    return addr.substring(0, 12) + '...' + addr.substring(addr.length - 8);
  }
  
  function clearMessages() {
    error = '';
    successMessage = '';
  }
</script>

<div class="simulator-controls">
  <!-- Status Messages -->
  {#if error}
    <div class="message error" on:click={clearMessages}>
      <AlertTriangle size={14} />
      <span>{error}</span>
    </div>
  {/if}
  
  {#if successMessage}
    <div class="message success" on:click={clearMessages}>
      <CheckCircle size={14} />
      <span>{successMessage}</span>
    </div>
  {/if}
  
  <!-- Main Controls - Not Initialized State -->
  {#if !status.isInitialized}
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">START SIMULATOR</span>
        </div>
      </div>
      <div class="card-content">
        <p class="card-description">No real value - perfect for testing smart contracts and dApps.</p>
        
        <div class="info-list">
          <p class="info-list-title">Starting the simulator will:</p>
          <div class="info-list-items">
            <div class="info-item">
              <Icons name="server" size={14} />
              <span>Launch a local DERO daemon</span>
            </div>
            <div class="info-item">
              <Icons name="wallet" size={14} />
              <span>Create a test wallet automatically</span>
            </div>
            <div class="info-item">
              <Icons name="coins" size={14} />
              <span>Generate initial test DERO</span>
            </div>
          </div>
        </div>
        
        <button 
          class="btn-primary"
          on:click={startSimulator}
          disabled={isLoading || status.isStarting}
        >
          {#if isLoading || status.isStarting}
            <Loader2 size={14} class="spin" />
            <span>Starting Simulator...</span>
          {:else}
            <Play size={14} />
            <span>Start Simulator</span>
          {/if}
        </button>
      </div>
    </div>
  {:else}
    <!-- Simulator Status Card -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">SIMULATOR STATUS</span>
        </div>
      </div>
      <div class="card-content">
        <div class="sim-status">
          <div class="status-item">
            <span class="label">Status</span>
            <span class="value ok">Running</span>
          </div>
          <div class="status-item">
            <span class="label">Block Height</span>
            <span class="value">{status.blockHeight?.toLocaleString() || '0'}</span>
          </div>
          <div class="status-item">
            <span class="label">Test Balance</span>
            <span class="value">{status.balanceDERO?.toFixed(2) || '0'} DERO</span>
          </div>
          <div class="status-item">
            <span class="label">Wallet</span>
            <span class="value mono">{formatAddress(status.walletAddress)}</span>
          </div>
        </div>
      </div>
    </div>
    
    <!-- Quick Actions Card -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">QUICK ACTIONS</span>
        </div>
      </div>
      <div class="card-content">
        <div class="action-row">
          <div class="input-group">
            <input type="number" bind:value={blocksToMine} min="1" max="100" />
            <span class="input-suffix">blocks</span>
          </div>
          <button class="btn-secondary" on:click={mineBlocks} disabled={isLoading}>
            <Pickaxe size={14} />
            Mine
          </button>
        </div>
        
        <div class="action-row">
          <div class="input-group">
            <input type="number" bind:value={targetDERO} min="1" max="10000" />
            <span class="input-suffix">DERO</span>
          </div>
          <button class="btn-secondary" on:click={generateDERO} disabled={isLoading}>
            <Coins size={14} />
            Generate
          </button>
        </div>
      </div>
    </div>
    
    <!-- Controls Card -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">CONTROLS</span>
        </div>
      </div>
      <div class="card-content">
        <div class="controls-row">
          <button class="btn-danger" on:click={stopSimulator} disabled={isLoading}>
            <Square size={14} />
            Stop Simulator
          </button>
          <button class="btn-ghost" on:click={resetSim} disabled={isLoading}>
            <Trash2 size={14} />
            Reset All Data
          </button>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .simulator-controls {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  /* Card wrapper - Standard pattern */
  .card-wrapper {
    background: var(--void-mid);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    overflow: hidden;
  }
  
  .explorer-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-deep);
    border-bottom: 1px solid var(--border-subtle);
  }
  
  .explorer-header-left {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .explorer-header-icon {
    color: var(--cyan-400);
    font-size: 12px;
  }
  
  .explorer-header-title {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    letter-spacing: 0.1em;
    color: var(--text-1);
  }
  
  .card-content {
    padding: var(--s-4, 16px);
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .card-description {
    font-size: 13px;
    color: var(--text-3);
    margin: 0;
  }
  
  /* Info List */
  .info-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-3, 12px);
  }
  
  .info-list-title {
    font-size: 13px;
    color: var(--text-2);
    margin: 0;
  }
  
  .info-list-items {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .info-item {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 13px;
    color: var(--text-3);
  }
  
  .info-item :global(svg) {
    color: var(--text-4);
    flex-shrink: 0;
  }
  
  /* Messages */
  .message {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px);
    border-radius: var(--r-md);
    font-size: 13px;
    cursor: pointer;
  }
  
  .message.error {
    background: rgba(239, 68, 68, 0.15);
    color: var(--status-err);
    border: 1px solid rgba(239, 68, 68, 0.3);
  }
  
  .message.success {
    background: rgba(34, 197, 94, 0.15);
    color: var(--status-ok);
    border: 1px solid rgba(34, 197, 94, 0.3);
  }
  
  /* Buttons */
  .btn-primary {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--cyan);
    color: var(--void-base);
    border: none;
    border-radius: var(--r-md);
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
    transition: all var(--dur-med);
    width: fit-content;
  }
  
  .btn-primary:hover:not(:disabled) {
    filter: brightness(1.1);
  }
  
  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .btn-secondary {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-up);
    color: var(--text-2);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--dur-med);
  }
  
  .btn-secondary:hover:not(:disabled) {
    background: var(--void-hover);
    color: var(--text-1);
  }
  
  .btn-secondary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .btn-danger {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(239, 68, 68, 0.15);
    color: var(--status-err);
    border: 1px solid rgba(239, 68, 68, 0.3);
    border-radius: var(--r-md);
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    transition: all var(--dur-med);
  }
  
  .btn-danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.25);
  }
  
  .btn-danger:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .btn-ghost {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: transparent;
    color: var(--text-3);
    border: none;
    border-radius: var(--r-md);
    font-size: 13px;
    cursor: pointer;
    transition: all var(--dur-med);
  }
  
  .btn-ghost:hover:not(:disabled) {
    color: var(--text-2);
    background: var(--void-hover);
  }
  
  .btn-ghost:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  /* Simulator Status Grid */
  .sim-status {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4, 16px);
  }
  
  .status-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  
  .status-item .label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-4);
  }
  
  .status-item .value {
    font-size: 14px;
    color: var(--text-1);
    font-weight: 500;
  }
  
  .status-item .value.ok {
    color: var(--status-ok);
  }
  
  .status-item .value.mono {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  
  /* Action Rows */
  .action-row {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
  }
  
  .input-group {
    flex: 1;
    display: flex;
    align-items: center;
    background: var(--void-deep);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    overflow: hidden;
  }
  
  .input-group input {
    flex: 1;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: transparent;
    border: none;
    color: var(--text-1);
    font-size: 13px;
    font-family: var(--font-mono);
    outline: none;
    min-width: 0;
  }
  
  .input-suffix {
    padding: var(--s-2, 8px) var(--s-3, 12px);
    font-size: 12px;
    color: var(--text-4);
    background: var(--void-up);
    border-left: 1px solid var(--border-default);
  }
  
  /* Controls Row */
  .controls-row {
    display: flex;
    gap: var(--s-3, 12px);
  }
  
  /* Animations */
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>
