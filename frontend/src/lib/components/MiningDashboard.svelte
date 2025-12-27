<script>
  import { onMount, onDestroy } from 'svelte';
  import { 
    GetMiningAnalyticsSummary,
    GetMiningDailyHistory,
    GetEpochContributionsByApp,
    GetTopSupportedApps,
    GetMinerStats,
    GetEpochStats
  } from '../../../wailsjs/go/main/App.js';
  import { HoloCard, Icons } from './holo';
  
  // Dashboard data
  let summary = null;
  let dailyHistory = [];
  let topApps = [];
  let minerStats = { is_running: false };
  let epochStats = { active: false, enabled: true };
  let isLoading = true;
  let error = null;
  let refreshInterval;
  
  // Chart dimensions
  const chartHeight = 120;
  const chartPadding = 20;
  
  onMount(async () => {
    await refreshAllData();
    // Refresh every 30 seconds
    refreshInterval = setInterval(refreshAllData, 30000);
  });
  
  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });
  
  async function refreshAllData() {
    isLoading = true;
    error = null;
    
    try {
      // Fetch all data in parallel
      const [summaryRes, historyRes, appsRes, minerRes, epochRes] = await Promise.all([
        GetMiningAnalyticsSummary(),
        GetMiningDailyHistory(7),
        GetTopSupportedApps(5),
        GetMinerStats(),
        GetEpochStats()
      ]);
      
      if (summaryRes.success) {
        summary = summaryRes.summary;
      }
      
      if (historyRes.success) {
        dailyHistory = historyRes.records || [];
      }
      
      if (appsRes.success) {
        topApps = appsRes.apps || [];
      }
      
      minerStats = minerRes;
      epochStats = epochRes;
    } catch (e) {
      error = e.message || 'Failed to load mining data';
      console.error('Mining dashboard error:', e);
    } finally {
      isLoading = false;
    }
  }
  
  // Calculate max hash count for chart scaling
  $: maxHashes = Math.max(...dailyHistory.map(d => d.total_hashes || 0), 1);
  
  // Format large numbers
  function formatNumber(num) {
    if (!num) return '0';
    if (num >= 1e9) return (num / 1e9).toFixed(2) + 'B';
    if (num >= 1e6) return (num / 1e6).toFixed(2) + 'M';
    if (num >= 1e3) return (num / 1e3).toFixed(1) + 'K';
    return num.toLocaleString();
  }
  
  // Format date for display
  function formatDate(dateStr) {
    if (!dateStr) return '';
    const [year, month, day] = dateStr.split('-');
    const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    return `${months[parseInt(month) - 1]} ${parseInt(day)}`;
  }
  
  // Format SCID for display
  function formatScid(scid) {
    if (!scid) return '';
    return scid.substring(0, 12) + '...' + scid.substring(scid.length - 8);
  }
</script>

<div class="mining-dashboard">
  <!-- Header -->
  <div class="page-header">
    <div class="header-left">
      <h2 class="page-title">
        <Icons name="trending" size={20} />
        Mining Analytics
      </h2>
      <p class="page-desc">Track your mining and EPOCH contributions</p>
    </div>
    <div class="header-right">
      <button class="btn btn-ghost btn-sm" on:click={refreshAllData} disabled={isLoading}>
        <Icons name="refresh" size={14} class={isLoading ? 'animate-spin' : ''} />
        Refresh
      </button>
    </div>
  </div>
  
  {#if error}
    <div class="alert alert-danger">
      {error}
    </div>
  {:else if isLoading && !summary}
    <div class="loading-container">
      <div class="loading-spinner"></div>
      <p>Loading analytics...</p>
    </div>
  {:else}
    <!-- Live Status Row -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">LIVE STATUS</span>
        </div>
      </div>
      <div class="card-content">
        <div class="status-row">
          <!-- Miner Status -->
          <div class="status-card" class:active={minerStats.is_running}>
            <div class="status-icon"><Icons name="pickaxe" size={24} /></div>
            <div class="status-content">
              <span class="status-label">Miner</span>
              <span class="status-value">
                {#if minerStats.is_running}
                  <span class="c-emerald">{minerStats.hash_rate_str || '0 H/s'}</span>
                {:else}
                  <span class="c-dim">Stopped</span>
                {/if}
              </span>
            </div>
            {#if minerStats.is_running}
              <div class="status-badge c-emerald">RUNNING</div>
            {/if}
          </div>
          
          <!-- EPOCH Status -->
          <div class="status-card" class:active={epochStats.active}>
            <div class="status-icon"><Icons name="sparkles" size={24} /></div>
            <div class="status-content">
              <span class="status-label">EPOCH</span>
              <span class="status-value">
                {#if epochStats.active}
                  <span class="c-cyan">{epochStats.hashes_str || '0'}</span>
                {:else if epochStats.enabled}
                  <span class="c-warn">Waiting</span>
                {:else}
                  <span class="c-dim">Disabled</span>
                {/if}
              </span>
            </div>
            {#if epochStats.active}
              <div class="status-badge c-cyan">ACTIVE</div>
            {/if}
          </div>
        </div>
      </div>
    </div>
    
    <!-- Today's Stats -->
    {#if summary?.today}
      <div class="card-wrapper">
        <div class="explorer-header">
          <div class="explorer-header-left">
            <span class="explorer-header-icon">◊</span>
            <span class="explorer-header-title">TODAY'S ACTIVITY</span>
          </div>
        </div>
        <div class="card-content">
        <div class="stats-grid">
          <div class="stat-card">
            <span class="stat-label">Total Hashes</span>
            <span class="stat-value c-cyan">{formatNumber(summary.today.total_hashes)}</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Miner</span>
            <span class="stat-value">{formatNumber(summary.today.miner_hashes)}</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">EPOCH</span>
            <span class="stat-value">{formatNumber(summary.today.epoch_hashes)}</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Blocks</span>
            <span class="stat-value c-emerald">{summary.today.blocks_found || 0}</span>
          </div>
          <div class="stat-card">
            <span class="stat-label">Miniblocks</span>
            <span class="stat-value">{(summary.today.minis_found || 0) + (summary.today.epoch_minis || 0)}</span>
            </div>
          </div>
        </div>
      </div>
    {/if}
    
    <!-- Weekly Chart -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">□</span>
          <span class="explorer-header-title">LAST 7 DAYS</span>
        </div>
      </div>
      <div class="card-content">
        {#if dailyHistory.length > 0}
          <svg width="100%" height={chartHeight + chartPadding * 2} class="history-chart">
            <!-- Y-axis labels -->
            <text x="0" y={chartPadding} class="chart-label" dominant-baseline="middle">
              {formatNumber(maxHashes)}
            </text>
            <text x="0" y={chartHeight + chartPadding} class="chart-label" dominant-baseline="middle">
              0
            </text>
            
            <!-- Bars -->
            {#each dailyHistory.reverse() as day, i}
              {@const barWidth = (100 / dailyHistory.length) - 2}
              {@const minerHeight = ((day.miner_hashes || 0) / maxHashes) * chartHeight}
              {@const epochHeight = ((day.epoch_hashes || 0) / maxHashes) * chartHeight}
              {@const totalHeight = minerHeight + epochHeight}
              {@const xPos = (i / dailyHistory.length) * 100 + 5}
              
              <!-- Miner portion (bottom) -->
              <rect
                x="{xPos}%"
                y={chartHeight - totalHeight + chartPadding}
                width="{barWidth}%"
                height={minerHeight}
                class="bar-miner"
                rx="2"
              >
                <title>{day.date}: {formatNumber(day.miner_hashes)} miner hashes</title>
              </rect>
              
              <!-- EPOCH portion (top) -->
              <rect
                x="{xPos}%"
                y={chartHeight - totalHeight + chartPadding + minerHeight}
                width="{barWidth}%"
                height={epochHeight}
                class="bar-epoch"
                rx="2"
              >
                <title>{day.date}: {formatNumber(day.epoch_hashes)} EPOCH hashes</title>
              </rect>
              
              <!-- Date label -->
              <text
                x="{xPos + barWidth/2}%"
                y={chartHeight + chartPadding + 15}
                class="chart-date"
                text-anchor="middle"
              >
                {formatDate(day.date)}
              </text>
            {/each}
            
            <!-- Baseline -->
            <line 
              x1="4%" 
              y1={chartHeight + chartPadding} 
              x2="100%" 
              y2={chartHeight + chartPadding} 
              class="chart-baseline"
            />
          </svg>
          
          <!-- Legend -->
          <div class="chart-legend">
            <div class="legend-item">
              <span class="legend-dot miner"></span>
              <span>Miner</span>
            </div>
            <div class="legend-item">
              <span class="legend-dot epoch"></span>
              <span>EPOCH</span>
            </div>
          </div>
        {:else}
          <div class="chart-empty">
            <p>No mining history yet</p>
          </div>
        {/if}
      </div>
    </div>
    
    <!-- Period Stats -->
    {#if summary?.week}
      <div class="period-grid">
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">⬡</span>
              <span class="explorer-header-title">THIS WEEK</span>
            </div>
          </div>
          <div class="card-content">
          <div class="period-stats">
            <div class="period-stat">
              <span class="period-stat-value c-cyan">{formatNumber(summary.week.TotalHashes)}</span>
              <span class="period-stat-label">Total Hashes</span>
            </div>
            <div class="period-stat">
              <span class="period-stat-value c-emerald">{summary.week.BlocksFound || 0}</span>
              <span class="period-stat-label">Blocks</span>
            </div>
            <div class="period-stat">
              <span class="period-stat-value">{(summary.week.MinisFound || 0) + (summary.week.EpochMinis || 0)}</span>
              <span class="period-stat-label">Miniblocks</span>
              </div>
            </div>
          </div>
        </div>
        
        {#if summary?.month}
          <div class="card-wrapper">
            <div class="explorer-header">
              <div class="explorer-header-left">
                <span class="explorer-header-icon">◎</span>
                <span class="explorer-header-title">THIS MONTH</span>
              </div>
            </div>
            <div class="card-content">
            <div class="period-stats">
              <div class="period-stat">
                <span class="period-stat-value c-cyan">{formatNumber(summary.month.TotalHashes)}</span>
                <span class="period-stat-label">Total Hashes</span>
              </div>
              <div class="period-stat">
                <span class="period-stat-value c-emerald">{summary.month.BlocksFound || 0}</span>
                <span class="period-stat-label">Blocks</span>
              </div>
              <div class="period-stat">
                <span class="period-stat-value">{(summary.month.MinisFound || 0) + (summary.month.EpochMinis || 0)}</span>
                <span class="period-stat-label">Miniblocks</span>
                </div>
              </div>
            </div>
          </div>
        {/if}
      </div>
    {/if}
    
    <!-- Top Supported Apps -->
    {#if topApps.length > 0}
      <div class="apps-section">
        <h3 class="section-title">Top Supported Apps</h3>
        <div class="apps-list">
          {#each topApps as app, i}
            <div class="app-row">
              <span class="app-rank">#{i + 1}</span>
              <div class="app-info">
                <span class="app-scid">{formatScid(app.AppSCID)}</span>
                {#if app.AppName}
                  <span class="app-name">{app.AppName}</span>
                {/if}
              </div>
              <div class="app-stats">
                <span class="app-hashes">{formatNumber(app.TotalHashes)}</span>
                <span class="app-minis">{app.TotalMinis || 0} minis</span>
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/if}
    
    <!-- Earnings Summary -->
    {#if summary?.earnings}
      <div class="earnings-section">
        <h3 class="section-title">Mining Earnings</h3>
        <div class="earnings-card">
          <div class="earnings-amount">
            <span class="earnings-value">{summary.earnings.formatted || '0.00000 DERO'}</span>
            <span class="earnings-label">Total Earned</span>
          </div>
          <div class="earnings-count">
            <span class="earnings-count-value">{summary.earnings.total_count || 0}</span>
            <span class="earnings-count-label">Rewards</span>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>

<style>
  .mining-dashboard {
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
  
  .loading-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-12, 48px);
    color: var(--text-4);
  }
  
  .loading-spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--border-dim);
    border-top-color: var(--cyan, #00d4aa);
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
  
  /* Status Row */
  .status-row {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-4, 16px);
  }
  
  .status-card {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255,255,255,0.06));
    border-radius: 8px;
    position: relative;
  }
  
  .status-card.active {
    border-color: rgba(0, 212, 170, 0.4);
    background: linear-gradient(135deg, rgba(0, 212, 170, 0.05), transparent);
  }
  
  .status-icon {
    color: var(--cyan);
  }
  
  .status-content {
    flex: 1;
  }
  
  .status-label {
    display: block;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4, #505068);
  }
  
  .status-value {
    font-size: 16px;
    font-weight: 600;
    font-family: var(--font-mono);
  }
  
  .status-badge {
    position: absolute;
    top: 8px;
    right: 8px;
    font-size: 9px;
    font-weight: 600;
    padding: 4px 8px;
    border-radius: var(--r-xs);
    background: currentColor;
    color: var(--void-dark) !important;
    opacity: 0.9;
  }
  
  /* Stats Grid */
  .stats-grid {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: var(--s-3, 12px);
  }
  
  .stat-card {
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    padding: var(--s-4, 16px);
    text-align: center;
  }
  
  .stat-label {
    display: block;
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4, #505068);
    margin-bottom: 4px;
  }
  
  .stat-value {
    display: block;
    font-size: 16px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1, #e8e8f0);
  }
  
  .history-chart {
    display: block;
  }
  
  .chart-label {
    fill: var(--text-4, #505068);
    font-size: 10px;
    font-family: var(--font-mono);
  }
  
  .chart-date {
    fill: var(--text-4, #505068);
    font-size: 9px;
  }
  
  .chart-baseline {
    stroke: var(--border-dim);
    stroke-width: 1;
  }
  
  .bar-miner {
    fill: var(--emerald, #10b981);
    opacity: 0.8;
  }
  
  .bar-epoch {
    fill: var(--cyan, #00d4aa);
    opacity: 0.8;
  }
  
  .chart-legend {
    display: flex;
    justify-content: center;
    gap: var(--s-6, 24px);
    margin-top: var(--s-4, 16px);
    padding-top: var(--s-3, 12px);
    border-top: 1px solid var(--border-dim);
  }
  
  .legend-item {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 11px;
    color: var(--text-3, #707088);
  }
  
  .legend-dot {
    width: 10px;
    height: 10px;
    border-radius: 2px;
  }
  
  .legend-dot.miner { background: var(--emerald, #10b981); }
  .legend-dot.epoch { background: var(--cyan, #00d4aa); }
  
  .chart-empty {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 120px;
    color: var(--text-4);
    font-size: 13px;
  }
  
  /* Period Grid */
  .period-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-4, 16px);
  }
  
  .period-stats {
    display: flex;
    gap: var(--s-4, 16px);
  }
  
  .period-stat {
    flex: 1;
    text-align: center;
  }
  
  .period-stat-value {
    display: block;
    font-size: 18px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }
  
  .period-stat-label {
    display: block;
    font-size: 10px;
    color: var(--text-4);
    margin-top: 2px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  /* Apps list */
  .apps-section {
    margin-bottom: var(--s-6, 24px);
  }
  
  .apps-list {
    background: var(--void-mid, #12121a);
    border: 1px solid var(--border-dim);
    border-radius: 12px;
    overflow: hidden;
  }
  
  .app-row {
    display: flex;
    align-items: center;
    gap: var(--s-4, 16px);
    padding: var(--s-3, 12px) var(--s-4, 16px);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .app-row:last-child {
    border-bottom: none;
  }
  
  .app-rank {
    font-size: 11px;
    font-weight: 600;
    color: var(--cyan, #00d4aa);
    min-width: 24px;
  }
  
  .app-info {
    flex: 1;
    min-width: 0;
  }
  
  .app-scid {
    display: block;
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-3, #707088);
  }
  
  .app-name {
    display: block;
    font-size: 13px;
    color: var(--text-1);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .app-stats {
    text-align: right;
  }
  
  .app-hashes {
    display: block;
    font-size: 13px;
    font-family: var(--font-mono);
    color: var(--text-1);
  }
  
  .app-minis {
    display: block;
    font-size: 10px;
    color: var(--text-4);
  }
  
  /* Earnings */
  .earnings-section {
    margin-bottom: var(--s-4, 16px);
  }
  
  .earnings-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: linear-gradient(135deg, rgba(16, 185, 129, 0.1), rgba(0, 212, 170, 0.05));
    border: 1px solid var(--emerald-dim, rgba(16, 185, 129, 0.2));
    border-radius: 12px;
    padding: var(--s-5, 20px);
  }
  
  .earnings-amount {
    display: flex;
    flex-direction: column;
  }
  
  .earnings-value {
    font-size: 24px;
    font-weight: 700;
    font-family: var(--font-mono);
    color: var(--emerald, #10b981);
  }
  
  .earnings-label {
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-4);
    margin-top: 4px;
  }
  
  .earnings-count {
    text-align: right;
  }
  
  .earnings-count-value {
    display: block;
    font-size: 20px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }
  
  .earnings-count-label {
    display: block;
    font-size: 10px;
    color: var(--text-4);
  }
  
  /* Responsive */
  @media (max-width: 768px) {
    .stats-grid {
      grid-template-columns: repeat(3, 1fr);
    }
    
    .status-row {
      grid-template-columns: 1fr;
    }
    
    .period-section {
      grid-template-columns: 1fr;
    }
  }
  
  @media (max-width: 480px) {
    .stats-grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>

