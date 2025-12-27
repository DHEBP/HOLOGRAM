<script>
  import { onMount, onDestroy } from 'svelte';
  import {
    GetEpochContributionsByApp,
    GetTopSupportedApps,
    GetMiningDailyHistory,
    GetMiningAnalyticsSummary
  } from '../../../wailsjs/go/main/App.js';
  import { HoloCard, Icons } from './holo';

  // State
  let contributions = [];
  let summary = null;
  let dailyHistory = [];
  let isLoading = true;
  let error = null;
  let refreshInterval;

  // Privacy notice shown
  let showPrivacyInfo = false;

  onMount(async () => {
    await refreshData();
    refreshInterval = setInterval(refreshData, 60000); // Refresh every minute
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
  });

  async function refreshData() {
    isLoading = true;
    error = null;
    
    try {
      const [contribRes, summaryRes, historyRes] = await Promise.all([
        GetEpochContributionsByApp(),
        GetMiningAnalyticsSummary(),
        GetMiningDailyHistory(30)
      ]);

      if (contribRes.success) {
        contributions = contribRes.contributions || [];
        // Sort by total hashes descending
        contributions.sort((a, b) => (b.TotalHashes || 0) - (a.TotalHashes || 0));
      }

      if (summaryRes.success) {
        summary = summaryRes.summary;
      }

      if (historyRes.success) {
        dailyHistory = historyRes.records || [];
      }
    } catch (e) {
      error = e.message || 'Failed to load contributions';
      console.error('Contributions error:', e);
    } finally {
      isLoading = false;
    }
  }

  // Calculate total EPOCH hashes
  $: totalEpochHashes = contributions.reduce((sum, c) => sum + (c.TotalHashes || 0), 0);
  $: totalEpochMinis = contributions.reduce((sum, c) => sum + (c.TotalMinis || 0), 0);
  $: totalRequests = contributions.reduce((sum, c) => sum + (c.RequestCount || 0), 0);
  
  // Calculate monthly trend
  $: monthlyEpochHashes = dailyHistory.reduce((sum, d) => sum + (d.epoch_hashes || 0), 0);

  function formatNumber(num) {
    if (!num) return '0';
    if (num >= 1e9) return (num / 1e9).toFixed(2) + 'B';
    if (num >= 1e6) return (num / 1e6).toFixed(2) + 'M';
    if (num >= 1e3) return (num / 1e3).toFixed(1) + 'K';
    return num.toLocaleString();
  }

  function formatScid(scid) {
    if (!scid) return '';
    return scid.substring(0, 12) + '...' + scid.substring(scid.length - 8);
  }

  function formatDate(dateStr) {
    if (!dateStr) return '';
    try {
      const date = new Date(dateStr);
      return date.toLocaleDateString();
    } catch (e) {
      return dateStr;
    }
  }

  function getTimeSince(dateStr) {
    if (!dateStr) return 'Unknown';
    try {
      const date = new Date(dateStr);
      const now = new Date();
      const diffMs = now - date;
      const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));
      if (diffDays === 0) return 'Today';
      if (diffDays === 1) return 'Yesterday';
      if (diffDays < 7) return `${diffDays} days ago`;
      if (diffDays < 30) return `${Math.floor(diffDays / 7)} weeks ago`;
      return `${Math.floor(diffDays / 30)} months ago`;
    } catch (e) {
      return 'Unknown';
    }
  }
</script>

<div class="contributions-dashboard">
  <!-- Header with Privacy Notice -->
  <div class="page-header">
    <div class="header-left">
      <h2 class="page-title">
        <Icons name="heart" size={20} />
        Your EPOCH Contributions
      </h2>
      <p class="page-desc">Apps you've supported through the EPOCH developer program</p>
    </div>
    <div class="header-right">
      <button 
        class="privacy-btn"
        on:click={() => showPrivacyInfo = !showPrivacyInfo}
        title="Privacy information"
      >
        <Icons name="shield" size={16} />
        <span>Private</span>
      </button>
    </div>
  </div>

  <!-- Privacy Info Banner -->
  {#if showPrivacyInfo}
    <div class="privacy-banner">
      <div class="privacy-icon"><Icons name="lock" size={24} /></div>
      <div class="privacy-content">
        <h4>Your Data Stays Private</h4>
        <ul>
          <li>This data is stored <strong>only on your device</strong></li>
          <li>App developers <strong>cannot see</strong> how much you contributed</li>
          <li>The blockchain <strong>doesn't record</strong> which app triggered your EPOCH contributions</li>
          <li>You are in <strong>full control</strong> of your contribution history</li>
        </ul>
      </div>
      <button class="close-btn" on:click={() => showPrivacyInfo = false}>×</button>
    </div>
  {/if}

  {#if error}
    <div class="alert alert-danger">{error}</div>
  {:else if isLoading && contributions.length === 0}
    <div class="loading-container">
      <div class="loading-spinner"></div>
      <p>Loading your contributions...</p>
    </div>
  {:else}
    <!-- Summary Stats Section -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◎</span>
          <span class="explorer-header-title">SUMMARY</span>
        </div>
      </div>
      <div class="card-content">
        <div class="summary-grid">
      <div class="summary-card highlight">
            <div class="summary-icon"><Icons name="zap" size={24} /></div>
        <div class="summary-content">
          <span class="summary-value">{formatNumber(totalEpochHashes)}</span>
          <span class="summary-label">Total Contributions</span>
        </div>
      </div>
      <div class="summary-card">
            <div class="summary-icon"><Icons name="box" size={24} /></div>
        <div class="summary-content">
          <span class="summary-value">{contributions.length}</span>
          <span class="summary-label">Apps Supported</span>
        </div>
      </div>
      <div class="summary-card">
            <div class="summary-icon"><Icons name="gem" size={24} /></div>
        <div class="summary-content">
          <span class="summary-value">{totalEpochMinis}</span>
          <span class="summary-label">Miniblocks Found</span>
        </div>
      </div>
      <div class="summary-card">
            <div class="summary-icon"><Icons name="refresh-cw" size={24} /></div>
        <div class="summary-content">
          <span class="summary-value">{formatNumber(totalRequests)}</span>
          <span class="summary-label">Total Requests</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Contribution Breakdown by App -->
    <div class="card-wrapper">
      <div class="explorer-header">
        <div class="explorer-header-left">
          <span class="explorer-header-icon">◊</span>
          <span class="explorer-header-title">APPS YOU'VE SUPPORTED</span>
        </div>
      </div>
      <div class="card-content">
      {#if contributions.length === 0}
        <div class="empty-state">
          <div class="empty-icon"><Icons name="moon" size={32} /></div>
          <h4>No Contributions Yet</h4>
          <p>When you use TELA apps with EPOCH support, your contributions will appear here.</p>
          <p class="empty-hint">This is completely private - only you can see this data.</p>
        </div>
      {:else}
        <div class="apps-list">
          {#each contributions as contrib, i}
            <div class="app-card">
              <div class="app-rank">#{i + 1}</div>
              <div class="app-info">
                <span class="app-scid" title={contrib.AppSCID}>{formatScid(contrib.AppSCID)}</span>
                {#if contrib.AppName}
                  <span class="app-name">{contrib.AppName}</span>
                {/if}
                <span class="app-meta">
                  First used: {formatDate(contrib.FirstSeen)} • Last active: {getTimeSince(contrib.LastActive)}
                </span>
              </div>
              <div class="app-stats">
                <div class="app-stat">
                  <span class="app-stat-value c-cyan">{formatNumber(contrib.TotalHashes)}</span>
                  <span class="app-stat-label">Hashes</span>
                </div>
                <div class="app-stat">
                  <span class="app-stat-value c-emerald">{contrib.TotalMinis || 0}</span>
                  <span class="app-stat-label">Minis</span>
                </div>
                <div class="app-stat">
                  <span class="app-stat-value">{contrib.RequestCount || 0}</span>
                  <span class="app-stat-label">Requests</span>
                </div>
              </div>
              <!-- Contribution Bar -->
              <div class="contribution-bar-container">
                <div 
                  class="contribution-bar"
                  style="width: {totalEpochHashes > 0 ? (contrib.TotalHashes / totalEpochHashes * 100) : 0}%"
                ></div>
              </div>
            </div>
          {/each}
        </div>
      {/if}
      </div>
    </div>

    <!-- Monthly Trend -->
    {#if dailyHistory.length > 0}
      <div class="card-wrapper">
        <div class="explorer-header">
          <div class="explorer-header-left">
            <span class="explorer-header-icon">□</span>
            <span class="explorer-header-title">30-DAY TREND</span>
          </div>
        </div>
        <div class="card-content">
        <div class="trend-stats">
          <div class="trend-stat">
            <span class="trend-value c-violet">{formatNumber(monthlyEpochHashes)}</span>
            <span class="trend-label">EPOCH Hashes This Month</span>
          </div>
          <div class="trend-stat">
            <span class="trend-value">{dailyHistory.filter(d => (d.epoch_hashes || 0) > 0).length}</span>
            <span class="trend-label">Active Days</span>
            </div>
          </div>
        </div>
      </div>
    {/if}

    <!-- Info Footer -->
    <div class="info-footer">
      <div class="info-item">
        <Icons name="info" size={14} />
        <span>EPOCH contributions help TELA developers without ads or tracking</span>
      </div>
    </div>
  {/if}
</div>

<style>
  .contributions-dashboard {
    padding: var(--s-4, 16px);
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
    color: var(--text-4);
    margin: 4px 0 0 0;
  }

  .privacy-btn {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: rgba(16, 185, 129, 0.15);
    border: 1px solid rgba(16, 185, 129, 0.3);
    border-radius: var(--r-full);
    color: var(--emerald, #10b981);
    font-size: 12px;
    cursor: pointer;
    transition: all var(--dur-med);
  }

  .privacy-btn:hover {
    background: rgba(16, 185, 129, 0.25);
  }

  /* Privacy Banner */
  .privacy-banner {
    display: flex;
    gap: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: linear-gradient(135deg, rgba(16, 185, 129, 0.1), rgba(0, 212, 170, 0.05));
    border: 1px solid rgba(16, 185, 129, 0.2);
    border-radius: 12px;
    margin-bottom: var(--s-6, 24px);
    position: relative;
  }

  .privacy-icon {
    color: var(--emerald);
  }

  .privacy-content h4 {
    margin: 0 0 8px 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--emerald);
  }

  .privacy-content ul {
    margin: 0;
    padding-left: 20px;
    font-size: 12px;
    color: var(--text-2);
    line-height: 1.6;
  }

  .privacy-content strong {
    color: var(--text-1);
  }

  .close-btn {
    position: absolute;
    top: 8px;
    right: 8px;
    background: transparent;
    border: none;
    color: var(--text-4);
    font-size: 18px;
    cursor: pointer;
    padding: 4px;
    line-height: 1;
  }

  .close-btn:hover {
    color: var(--text-2);
  }

  /* Loading */
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
    border-top-color: var(--violet, #a78bfa);
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

  /* Summary Grid */
  .summary-grid {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--s-4, 16px);
  }

  .summary-card {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
  }

  .summary-card.highlight {
    background: linear-gradient(135deg, rgba(167, 139, 250, 0.1), rgba(0, 212, 170, 0.05));
    border-color: rgba(167, 139, 250, 0.3);
  }

  .summary-icon {
    color: var(--cyan);
  }

  .summary-value {
    display: block;
    font-size: 20px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }

  .summary-label {
    display: block;
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4);
    margin-top: 2px;
  }

  /* Empty State */
  .empty-state {
    text-align: center;
    padding: var(--s-8, 32px);
    background: var(--void-deep);
    border: 1px solid var(--border-subtle);
    border-radius: 8px;
  }

  .empty-icon {
    color: var(--text-4);
    margin-bottom: var(--s-4);
  }

  .empty-state h4 {
    margin: 0 0 8px 0;
    font-size: 16px;
    color: var(--text-2);
  }

  .empty-state p {
    margin: 0;
    font-size: 13px;
    color: var(--text-4);
  }

  .empty-hint {
    margin-top: 8px !important;
    font-size: 11px !important;
    color: var(--emerald) !important;
  }

  .apps-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-3, 12px);
  }

  .app-card {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    position: relative;
  }

  .app-rank {
    font-size: 12px;
    font-weight: 600;
    color: var(--violet, #a78bfa);
    min-width: 28px;
  }

  .app-info {
    min-width: 0;
  }

  .app-scid {
    display: block;
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-3);
  }

  .app-name {
    display: block;
    font-size: 14px;
    font-weight: 500;
    color: var(--text-1);
    margin-top: 2px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .app-meta {
    display: block;
    font-size: 10px;
    color: var(--text-4);
    margin-top: 4px;
  }

  .app-stats {
    display: flex;
    gap: var(--s-4, 16px);
  }

  .app-stat {
    text-align: center;
    min-width: 60px;
  }

  .app-stat-value {
    display: block;
    font-size: 14px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }

  .app-stat-label {
    display: block;
    font-size: 9px;
    color: var(--text-4);
    margin-top: 2px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .contribution-bar-container {
    position: absolute;
    bottom: 0;
    left: 0;
    right: 0;
    height: 3px;
    background: var(--void-pure);
    border-radius: 0 0 8px 8px;
    overflow: hidden;
  }

  .contribution-bar {
    height: 100%;
    background: linear-gradient(90deg, var(--violet), var(--cyan));
    transition: width 0.5s ease-out;
  }

  /* Trend Stats */
  .trend-stats {
    display: flex;
    gap: var(--s-6, 24px);
  }

  .trend-stat {
    text-align: center;
    flex: 1;
  }

  .trend-value {
    display: block;
    font-size: 24px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1);
  }

  .trend-label {
    display: block;
    font-size: 11px;
    color: var(--text-4);
    margin-top: 4px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  /* Info Footer */
  .info-footer {
    padding: var(--s-4, 16px);
    background: var(--void-deep);
    border-radius: 8px;
  }

  .info-item {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 12px;
    color: var(--text-4);
  }

  /* Responsive */
  @media (max-width: 768px) {
    .summary-grid {
      grid-template-columns: repeat(2, 1fr);
    }

    .app-card {
      grid-template-columns: auto 1fr;
      grid-template-rows: auto auto;
    }

    .app-stats {
      grid-column: 1 / -1;
      justify-content: center;
      margin-top: var(--s-3);
      padding-top: var(--s-3);
      border-top: 1px solid var(--border-dim);
    }
  }
</style>

