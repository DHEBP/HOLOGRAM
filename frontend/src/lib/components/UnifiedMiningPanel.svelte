<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    GetUnifiedMiningStatus,
    GetMiningRecommendations,
    ApplyMiningPowerMode,
    StartMining,
    StopMining,
    SetEpochEnabled,
    GetCPUCores
  } from '../../../wailsjs/go/main/App.js';
  import { HoloCard, Icons } from './holo';

  // State
  let unifiedStatus = null;
  let recommendations = null;
  let maxCores = 4;
  let isLoading = true;
  let error = null;
  let refreshInterval;

  // Config state
  let miningAddress = '';
  let miningThreads = 2;
  let selectedPowerMode = 'Balanced';

  onMount(async () => {
    maxCores = await GetCPUCores();
    miningThreads = Math.max(1, Math.floor(maxCores / 2));
    await refreshAll();
    refreshInterval = setInterval(refreshStatus, 3000);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });

  async function refreshAll() {
    isLoading = true;
    try {
      const [statusRes, recsRes] = await Promise.all([
        GetUnifiedMiningStatus(),
        GetMiningRecommendations()
      ]);

      if (statusRes.success) {
        unifiedStatus = statusRes.state;
        if (unifiedStatus?.WalletAddress) {
          miningAddress = unifiedStatus.WalletAddress;
        }
        if (unifiedStatus?.MinerThreads > 0) {
          miningThreads = unifiedStatus.MinerThreads;
        }
      }

      if (recsRes.success) {
        recommendations = recsRes;
      }
    } catch (e) {
      error = e.message;
    } finally {
      isLoading = false;
    }
  }

  async function refreshStatus() {
    try {
      const res = await GetUnifiedMiningStatus();
      if (res.success) {
        unifiedStatus = res.state;
      }
    } catch (e) {
      console.error('Status refresh error:', e);
    }
  }

  async function handleToggleMiner() {
    error = null;
    try {
      if (unifiedStatus?.MinerRunning) {
        const result = await StopMining();
        if (!result.success) {
          error = result.error;
        }
      } else {
        if (!miningAddress) {
          error = 'Please enter a wallet address';
          return;
        }
        const result = await StartMining(miningAddress, miningThreads);
        if (!result.success) {
          error = result.error;
        }
      }
      await refreshStatus();
    } catch (e) {
      error = e.message;
    }
  }

  async function handleToggleEpoch() {
    error = null;
    try {
      const newState = !unifiedStatus?.EpochEnabled;
      await SetEpochEnabled(newState);
      await refreshStatus();
    } catch (e) {
      error = e.message;
    }
  }

  async function applyPowerMode(modeName) {
    error = null;
    try {
      selectedPowerMode = modeName;
      const result = await ApplyMiningPowerMode(modeName);
      if (!result.success) {
        error = result.error;
      } else {
        await refreshAll();
      }
    } catch (e) {
      error = e.message;
    }
  }

  function formatHashrate(rate) {
    if (!rate) return '0 H/s';
    if (rate >= 1e6) return (rate / 1e6).toFixed(2) + ' MH/s';
    if (rate >= 1e3) return (rate / 1e3).toFixed(2) + ' KH/s';
    return rate.toFixed(0) + ' H/s';
  }

  function formatNumber(num) {
    if (!num) return '0';
    return num.toLocaleString();
  }
</script>

<div class="unified-mining-panel">
  <!-- Header -->
  <div class="page-header">
    <div class="header-left">
      <h2 class="page-title">
        <Icons name="cpu" size={20} />
        Mining Control
      </h2>
      <p class="page-desc">Unified mining and developer support management</p>
    </div>
    <div class="header-right">
      {#if unifiedStatus?.IsActive}
        <span class="status-pill active">
          <span class="status-dot"></span>
          {unifiedStatus.PrimaryMode === 'both' ? 'Both Active' : 
           unifiedStatus.MinerRunning ? 'Mining' : 'EPOCH Active'}
        </span>
      {:else}
        <span class="status-pill inactive">Inactive</span>
      {/if}
    </div>
  </div>

  {#if error}
    <div class="alert alert-danger">{error}</div>
  {/if}

  {#if isLoading && !unifiedStatus}
    <div class="loading-container">
      <div class="loading-spinner"></div>
      <p>Loading mining status...</p>
    </div>
  {:else}
    <!-- Quick Power Modes Section -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">QUICK MODES</span>
        </div>
      </div>
      <div class="card-content">
      <div class="power-modes">
        {#if recommendations?.power_modes}
          {#each recommendations.power_modes as mode}
            <button
              class="power-mode-btn"
              class:selected={selectedPowerMode === mode.name}
              on:click={() => applyPowerMode(mode.name)}
            >
              <span class="mode-icon">
                  {#if mode.name === 'Eco'}
                    <Icons name="zap" size={20} />
                  {:else if mode.name === 'Balanced'}
                    <Icons name="gauge" size={20} />
                  {:else if mode.name === 'Performance'}
                    <Icons name="activity" size={20} />
                  {:else}
                    <Icons name="cpu" size={20} />
                  {/if}
              </span>
              <span class="mode-name">{mode.name}</span>
              <span class="mode-desc">{mode.description}</span>
            </button>
          {/each}
        {/if}
        </div>
      </div>
    </div>

    <!-- Live Stats Section -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◊</span>
          <span class="explorer-header-title">LIVE STATUS</span>
        </div>
      </div>
      <div class="card-content">
      <div class="stats-row">
        <!-- Miner Stats -->
        <div class="stat-card" class:active={unifiedStatus?.MinerRunning}>
          <div class="stat-header">
              <span class="stat-icon"><Icons name="pickaxe" size={18} /></span>
            <span class="stat-title">Background Miner</span>
            <button
              class="toggle-btn"
              class:running={unifiedStatus?.MinerRunning}
              on:click={handleToggleMiner}
            >
              {unifiedStatus?.MinerRunning ? 'Stop' : 'Start'}
            </button>
          </div>
          {#if unifiedStatus?.MinerRunning}
            <div class="stat-values">
              <div class="stat-value-item">
                <span class="value c-cyan">{unifiedStatus.MinerHashRateStr || '0 H/s'}</span>
                <span class="label">Hashrate</span>
              </div>
              <div class="stat-value-item">
                <span class="value">{formatNumber(unifiedStatus.MinerBlocks)}</span>
                <span class="label">Blocks</span>
              </div>
              <div class="stat-value-item">
                <span class="value">{formatNumber(unifiedStatus.MinerMinis)}</span>
                <span class="label">Minis</span>
              </div>
            </div>
            <div class="stat-meta">
              <span>Threads: {unifiedStatus.MinerThreads}/{maxCores}</span>
              <span>Uptime: {unifiedStatus.MinerUptime}</span>
            </div>
          {:else}
            <p class="stat-inactive">Mining is stopped</p>
          {/if}
        </div>
        </div>
      </div>
    </div>

    <!-- Configuration Section (when not running) -->
    {#if !unifiedStatus?.MinerRunning}
      <div class="card-wrapper">
        <div class="explorer-header">
          <div class="explorer-header-left">
            <span class="explorer-header-icon">□</span>
            <span class="explorer-header-title">MINING CONFIGURATION</span>
          </div>
        </div>
        <div class="card-content">
          <div class="settings-row">
            <div class="settings-row-info">
              <span class="settings-row-label">Wallet Address</span>
              <span class="settings-row-desc">Mining rewards will be sent to this address</span>
            </div>
          </div>
            <input
              type="text"
              bind:value={miningAddress}
              placeholder="dero1qy..."
              class="input"
            />

          <div class="settings-row" style="margin-top: 16px;">
            <div class="settings-row-info">
              <span class="settings-row-label">Mining Threads</span>
              <span class="settings-row-desc">CPU threads to use for mining</span>
            </div>
              <span class="slider-value c-cyan">{miningThreads} / {maxCores}</span>
            </div>
            <input
              type="range"
              min="1"
              max={maxCores}
              bind:value={miningThreads}
              class="slider"
            />
        </div>
      </div>
    {/if}

    <!-- Today's Activity Section -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">⬡</span>
          <span class="explorer-header-title">TODAY'S ACTIVITY</span>
        </div>
      </div>
      <div class="card-content">
      <div class="today-stats">
        <div class="today-stat">
          <span class="today-value c-cyan">{formatNumber(unifiedStatus?.TotalHashesToday || 0)}</span>
          <span class="today-label">Total Hashes</span>
        </div>
        <div class="today-stat">
          <span class="today-value c-emerald">{formatNumber(unifiedStatus?.TotalBlocksToday || 0)}</span>
          <span class="today-label">Blocks</span>
        </div>
        <div class="today-stat">
          <span class="today-value">{formatNumber(unifiedStatus?.TotalMinisToday || 0)}</span>
          <span class="today-label">Miniblocks</span>
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .unified-mining-panel {
    padding: 0;
  }

  /* Page Header */
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    margin-bottom: var(--s-6, 24px);
  }

  .page-title {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 20px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0;
  }

  .page-desc {
    font-size: 13px;
    color: var(--text-4, #505068);
    margin: 4px 0 0 0;
  }

  .status-pill {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    border-radius: var(--r-full);
    font-size: 12px;
    font-weight: 500;
  }

  .status-pill.active {
    background: rgba(16, 185, 129, 0.15);
    color: var(--emerald, #10b981);
  }

  .status-pill.inactive {
    background: var(--void-mid, #12121a);
    color: var(--text-4, #505068);
  }

  .status-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
    animation: pulse 2s infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }

  .loading-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: var(--s-12, 48px);
    color: var(--text-4);
  }

  .loading-spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--border-dim);
    border-top-color: var(--cyan);
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-bottom: var(--s-4);
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  /* Card Wrapper - Explorer Style */
  .card-wrapper {
    background: var(--void-mid);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    overflow: hidden;
    margin-bottom: var(--s-6, 24px);
  }

  .explorer-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 24px;
    background: var(--void-deep);
    border-bottom: 1px solid var(--border-subtle);
  }

  .explorer-header-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .explorer-header-icon {
    font-size: 16px;
    color: var(--cyan-400);
    line-height: 1;
  }

  .explorer-header-title {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-1);
  }

  .card-content {
    padding: 24px;
  }

  /* Settings Row - v6.1 Pattern */
  .settings-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 8px;
  }

  .settings-row-info {
    flex: 1;
  }

  .settings-row-label {
    display: block;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
    margin-bottom: 2px;
  }

  .settings-row-desc {
    display: block;
    font-size: 11px;
    color: var(--text-4, #505068);
  }

  /* Power Modes */
  .power-modes {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--s-3, 12px);
  }

  .power-mode-btn {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .power-mode-btn:hover {
    background: var(--void-up, #1a1a24);
    border-color: var(--border-strong, rgba(255, 255, 255, 0.12));
  }

  .power-mode-btn.selected {
    background: rgba(0, 212, 170, 0.1);
    border-color: var(--cyan, #00d4aa);
  }

  .mode-icon {
    font-size: 20px;
    margin-bottom: 4px;
  }

  .mode-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-1);
  }

  .mode-desc {
    font-size: 10px;
    color: var(--text-4);
    text-align: center;
  }

  /* Stats Row */
  .stats-row {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-4, 16px);
  }

  .stat-card {
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    padding: var(--s-4, 16px);
    transition: all 0.2s;
  }

  .stat-card.active {
    border-color: rgba(0, 212, 170, 0.4);
    background: linear-gradient(135deg, rgba(0, 212, 170, 0.05), transparent);
  }

  .stat-header {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-bottom: var(--s-4, 16px);
  }

  .stat-icon {
    font-size: 18px;
  }

  .stat-title {
    flex: 1;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-2);
  }

  .toggle-btn {
    padding: 4px 12px;
    font-size: 11px;
    font-weight: 500;
    border-radius: 5px;
    border: none;
    cursor: pointer;
    transition: all 0.2s;
    background: var(--void-up);
    color: var(--text-3);
  }

  .toggle-btn:hover {
    background: var(--cyan);
    color: var(--void-dark);
  }

  .toggle-btn.running {
    background: rgba(239, 68, 68, 0.2);
    color: var(--err, #ef4444);
  }

  .toggle-btn.running:hover {
    background: var(--err);
    color: white;
  }

  .stat-values {
    display: flex;
    gap: var(--s-4, 16px);
    margin-bottom: var(--s-3, 12px);
  }

  .stat-value-item {
    flex: 1;
    text-align: center;
  }

  .stat-value-item .value {
    display: block;
    font-size: 16px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }

  .stat-value-item .label {
    display: block;
    font-size: 10px;
    color: var(--text-4);
    margin-top: 2px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .stat-meta {
    display: flex;
    justify-content: space-between;
    font-size: 11px;
    color: var(--text-4);
    padding-top: var(--s-3, 12px);
    border-top: 1px solid var(--border-dim);
  }

  .stat-inactive {
    font-size: 12px;
    color: var(--text-4);
    text-align: center;
    padding: var(--s-4, 16px) 0;
    margin: 0;
  }

  /* Input */
  .input {
    width: 100%;
    padding: 10px 14px;
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    color: var(--text-1);
    font-family: var(--font-mono);
    font-size: 13px;
    outline: none;
    transition: border-color 0.2s;
    box-sizing: border-box;
  }

  .input:focus {
    border-color: var(--cyan);
  }

  .slider-value {
    font-size: 13px;
    font-family: var(--font-mono);
  }

  .slider {
    width: 100%;
    height: 6px;
    -webkit-appearance: none;
    background: var(--void-deep, #08080e);
    border-radius: 3px;
    outline: none;
    margin-top: 8px;
  }

  .slider::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 16px;
    height: 16px;
    background: var(--cyan-400, #22d3ee);
    border-radius: 50%;
    cursor: pointer;
  }

  /* Today Stats */
  .today-stats {
    display: flex;
    gap: var(--s-6, 24px);
  }

  .today-stat {
    flex: 1;
    text-align: center;
  }

  .today-value {
    display: block;
    font-size: 22px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }

  .today-label {
    display: block;
    font-size: 10px;
    color: var(--text-4);
    margin-top: 4px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  /* Responsive */
  @media (max-width: 768px) {
    .power-modes {
      grid-template-columns: repeat(2, 1fr);
    }

    .stats-row {
      grid-template-columns: 1fr;
    }
  }
</style>

