<script>
  import { onMount, onDestroy } from 'svelte';
  import { settingsState, appState, consoleLogs, clearConsoleLogs, syncNetworkMode, saveSetting, loadSettings } from '../lib/stores/appState.js';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';
  import { 
    SetSetting, StartGnomon, StopGnomon, ResyncGnomon,
    GetSearchExclusions, AddSearchExclusion, RemoveSearchExclusion, ClearSearchExclusions, SetSearchMinLikes,
    DetectRunningNode, CheckDerodStatus, GetLatestDerodRelease,
    DownloadDerodFromGitHub, StartNode, StopNode, GetNodeStatus, GetSyncProgress,
    GetConnectedApps, RevokeAppPermissions, RevokeAppPermission, GetPermissionTypes,
    SetCypherpunkMode, GetNetworkFilterStatus, AddAllowedHost, RemoveAllowedHost,
    GetConnectionLog, ClearConnectionLog, GetActiveConnections,
    IsEpochEnabled, SetEpochEnabled, GetEpochStats, SetEpochConfig, InitializeEpoch,
    GetDevSupportStatus, GetDevSupportStats, SetDevSupportEnabled, IsDevSupportEnabled,
    SetNodeAdvancedConfig, GetNodeAdvancedConfig,
    StartSimulatorMode, StopSimulatorMode, GetSimulatorStatus, ResetSimulator,
    GetConsoleLogs, SetNetworkMode, GetAppInfo,
    ClearConsoleLogs as ClearBackendLogs,
    SetGnomonAutostart, GetGnomonAutostart
  } from '../../wailsjs/go/main/App.js';
  import OfflineCacheManager from '../lib/components/OfflineCacheManager.svelte';
  import SyncManager from '../lib/components/SyncManager.svelte';
  import SafeBrowsingSettings from '../lib/components/SafeBrowsingSettings.svelte';
import { HoloCard, DotIndicator, HoloBadge, Icons } from '../lib/components/holo';
  import ServerManager from '../lib/components/ServerManager.svelte';
  import { Settings as SettingsIcon } from 'lucide-svelte';

  let activeSection = 'general';
  
  const sections = [
    { id: 'general', label: 'General', iconName: 'settings' },
    { id: 'node', label: 'Node', iconName: 'server' },
    { id: 'simulator', label: 'Simulator', iconName: 'gamepad' },
    { id: 'servers', label: 'TELA Servers', iconName: 'globe' },
    { id: 'offline-cache', label: 'Offline Cache', iconName: 'download' },
    { id: 'sync-manager', label: 'Sync Manager', iconName: 'refresh-cw' },
    { id: 'safe-browsing', label: 'Safe Browsing', iconName: 'shield' },
    { id: 'network', label: 'Network', iconName: 'globe' },
    { id: 'gnomon', label: 'Gnomon', iconName: 'database' },
    { id: 'connected-apps', label: 'Connected Apps', iconName: 'link' },
    { id: 'privacy', label: 'Privacy Mode', iconName: 'lock' },
    { id: 'console', label: 'Console', iconName: 'terminal' },
    { id: 'developer-support', label: 'Developer Support', iconName: 'heart' },
    { id: 'about', label: 'About', iconName: 'info' },
  ];
  
  // App info state
  let appInfo = {
    name: 'Hologram',
    version: '',
    buildDate: '',
    gitCommit: '',
    description: ''
  };
  
  // Connected Apps state
  let connectedApps = [];
  let permissionTypes = [];
  let isLoadingApps = false;
  let selectedApp = null;
  
  // Node state
  let nodeStatus = { isRunning: false };
  let derodStatus = { installed: false };
  let latestRelease = null;
  let downloadProgress = null;
  let isDownloading = false;
  let nodeDataDir = '';
  let syncProgress = { progress: 0, isSynced: false };
  let statusInterval;
  
  // Privacy Mode state
  let privacyModeEnabled = false;
  let allowedHosts = [];
  let connectionLog = [];
  let activeConnections = [];
  let newAllowedHost = '';
  let privacyLoading = false;
  
  // Node detection state
  let detecting = false;
  let detectionMessage = '';
  
  // Gnomon resync state
  let resyncingGnomon = false;
  let gnomonAutostart = false;
  
  // Search exclusions state
  let searchExclusions = [];
  let searchMinLikes = 0;
  let showExclusionModal = false;
  let newExclusionFilter = '';
  
  // Developer Support (EPOCH + Passive Hashing) state
  let epochStats = { 
    enabled: true, 
    active: false, 
    paused: false,
    pause_reason: '',
    worker_running: false,
    hashes: 0, 
    miniblocks: 0,
    total_hashes: 0,
    total_hashes_str: '0',
    total_miniblocks: 0,
    uptime_seconds: 0
  };
  let epochEnabled = true;
  let epochMaxHashes = 100;
  let epochMaxThreads = 2;
  
  // Simulator state
  let simulatorStatus = {
    isInitialized: false,
    isStarting: false,
    daemonRunning: false,
    walletOpen: false,
    walletAddress: '',
    balance: 0,
    balanceDERO: 0,
    blockHeight: 0
  };
  let simulatorLoading = false;
  let simulatorError = '';
  let simulatorSuccess = '';
  let simulatorStatusInterval;
  let epochStatsInterval;
  let consoleLogsInterval;
  let epochError = '';
  let devSupportStats = null;
  
  // Console viewport for auto-scroll
  let consoleViewport;
  let consoleUserScrolled = false; // Track if user has scrolled up
  let previousLogCount = 0; // Track log count to detect new logs
  
  // Advanced Node Options state
  let fastSyncEnabled = false;
  let pruneHistory = 0;
  let advancedNodeLoading = false;
  
  onMount(async () => {
    // Sync network mode from backend first
    await syncNetworkMode();
    await refreshNodeStatus();
    
    // Subscribe to network mode changes
    EventsOn('network-mode-changed', async () => {
      await syncNetworkMode();
    });
    await loadPermissionTypes();
    await initEpochPanel();
    await loadAdvancedNodeConfig();
    await loadAppInfo();
    
    // Listen for section navigation from status indicators
    const handleNavigateSection = (e) => {
      const { section } = e.detail;
      if (section && sections.find(s => s.id === section)) {
        activeSection = section;
      }
    };
    window.addEventListener('navigate-section', handleNavigateSection);
    
    // Store handler for cleanup in onDestroy
    window._settingsNavigateHandler = handleNavigateSection;
  });
  
  // Load app version info
  async function loadAppInfo() {
    try {
      const info = await GetAppInfo();
      appInfo = { ...appInfo, ...info };
    } catch (e) {
      console.error('Failed to load app info:', e);
    }
  }
  
  // EPOCH (Developer Support) functions
  async function initEpochPanel() {
    try {
      epochEnabled = await IsDevSupportEnabled();
      await refreshEpochStats();
      await refreshDevSupportStats();
    } catch (e) {
      console.error('Failed to init Developer Support panel:', e);
    }
  }
  
  async function refreshEpochStats() {
    try {
      const stats = await GetEpochStats();
      epochStats = { ...epochStats, ...stats };
      epochEnabled = stats.enabled;
      epochMaxHashes = stats.max_hashes || 100;
      epochMaxThreads = stats.max_threads || 2;
    } catch (e) {
      console.error('Failed to get EPOCH stats:', e);
    }
  }
  
  async function refreshDevSupportStats() {
    try {
      const stats = await GetDevSupportStats();
      if (stats.success) {
        devSupportStats = stats;
      }
    } catch (e) {
      console.error('Failed to get Developer Support stats:', e);
    }
  }
  
  async function handleToggleEpoch() {
    epochError = '';
    try {
      // Note: epochEnabled is already updated by bind:checked before this handler runs
      // So we use the current value, not !epochEnabled
      const result = await SetDevSupportEnabled(epochEnabled);
      if (!result.success && result.error && !result.error.includes('No wallet')) {
        epochError = result.error;
        // Revert the toggle on error
        epochEnabled = !epochEnabled;
      }
      await refreshEpochStats();
      await refreshDevSupportStats();
      
      if (epochEnabled) {
        startEpochStatsPolling();
      } else {
        stopEpochStatsPolling();
      }
    } catch (e) {
      epochError = e.message || 'Failed to toggle developer support';
      // Revert the toggle on error
      epochEnabled = !epochEnabled;
    }
  }
  
  async function handleUpdateEpochConfig() {
    epochError = '';
    try {
      const result = await SetEpochConfig(epochMaxHashes, epochMaxThreads);
      if (!result.success) {
        epochError = result.error || 'Failed to update configuration';
        return;
      }
      await refreshEpochStats();
    } catch (e) {
      epochError = e.message || 'Failed to update configuration';
    }
  }
  
  async function handleRetryEpoch() {
    epochError = '';
    try {
      const result = await InitializeEpoch();
      if (!result.success) {
        epochError = result.error || 'Failed to connect';
        return;
      }
      await refreshEpochStats();
      startEpochStatsPolling();
    } catch (e) {
      epochError = e.message || 'Failed to connect';
    }
  }
  
  function startEpochStatsPolling() {
    if (epochStatsInterval) return;
    epochStatsInterval = setInterval(async () => {
      await refreshEpochStats();
      await refreshDevSupportStats();
    }, 5000);
  }
  
  function stopEpochStatsPolling() {
    if (epochStatsInterval) {
      clearInterval(epochStatsInterval);
      epochStatsInterval = null;
    }
  }
  
  // Format uptime seconds to human readable
  function formatUptime(seconds) {
    if (!seconds || seconds < 60) {
      return `${seconds || 0}s`;
    } else if (seconds < 3600) {
      const m = Math.floor(seconds / 60);
      const s = seconds % 60;
      return `${m}m ${s}s`;
    } else {
      const h = Math.floor(seconds / 3600);
      const m = Math.floor((seconds % 3600) / 60);
      return `${h}h ${m}m`;
    }
  }
  
  // Advanced Node Options functions
  async function loadAdvancedNodeConfig() {
    try {
      const config = await GetNodeAdvancedConfig();
      if (config.success !== false) {
        fastSyncEnabled = config.fastSync || false;
        pruneHistory = config.pruneHistory || 0;
      }
    } catch (e) {
      console.error('Failed to load advanced node config:', e);
    }
  }
  
  async function saveAdvancedNodeConfig() {
    advancedNodeLoading = true;
    try {
      const result = await SetNodeAdvancedConfig(fastSyncEnabled, pruneHistory);
      if (!result.success) {
        console.error('Failed to save:', result.error);
      }
    } catch (e) {
      console.error('Failed to save advanced node config:', e);
    } finally {
      advancedNodeLoading = false;
    }
  }
  
  // Simulator functions
  async function refreshSimulatorStatus() {
    try {
      const result = await GetSimulatorStatus();
      if (result.success) {
        simulatorStatus = { ...simulatorStatus, ...result };
      }
    } catch (e) {
      console.error('Failed to get simulator status:', e);
    }
  }
  
  async function startSimulator() {
    simulatorLoading = true;
    simulatorError = '';
    simulatorSuccess = '';
    
    try {
      const result = await StartSimulatorMode();
      if (result.success) {
        simulatorSuccess = 'Simulator started successfully';
        await refreshSimulatorStatus();
      } else {
        simulatorError = result.error || 'Failed to start simulator';
      }
    } catch (e) {
      simulatorError = e.message || 'Failed to start simulator';
    } finally {
      simulatorLoading = false;
    }
  }
  
  async function stopSimulator() {
    simulatorLoading = true;
    simulatorError = '';
    
    try {
      const result = await StopSimulatorMode();
      if (result.success) {
        simulatorSuccess = 'Simulator stopped';
        await refreshSimulatorStatus();
      } else {
        simulatorError = result.error || 'Failed to stop simulator';
      }
    } catch (e) {
      simulatorError = e.message || 'Failed to stop simulator';
    } finally {
      simulatorLoading = false;
    }
  }
  
  async function resetSimulator() {
    if (!confirm('This will delete all simulator data and start fresh. Continue?')) {
      return;
    }
    
    simulatorLoading = true;
    simulatorError = '';
    
    try {
      const result = await ResetSimulator();
      if (result.success) {
        simulatorSuccess = 'Simulator reset complete';
        await refreshSimulatorStatus();
      } else {
        simulatorError = result.error || 'Reset failed';
      }
    } catch (e) {
      simulatorError = e.message || 'Reset failed';
    } finally {
      simulatorLoading = false;
    }
  }
  
  function formatSimulatorAddress(addr) {
    if (!addr) return '—';
    return addr.substring(0, 12) + '...' + addr.substring(addr.length - 8);
  }
  
  function clearSimulatorMessages() {
    simulatorError = '';
    simulatorSuccess = '';
  }
  
  function startSimulatorPolling() {
    if (simulatorStatusInterval) return;
    simulatorStatusInterval = setInterval(refreshSimulatorStatus, 3000);
  }
  
  function stopSimulatorPolling() {
    if (simulatorStatusInterval) {
      clearInterval(simulatorStatusInterval);
      simulatorStatusInterval = null;
    }
  }
  
  // Start polling when EPOCH section becomes active
  $: if (activeSection === 'developer-support') {
    refreshEpochStats();
    if (epochEnabled) {
      startEpochStatsPolling();
    }
  }
  
  // Start polling when Simulator section becomes active
  $: if (activeSection === 'simulator') {
    refreshSimulatorStatus();
    startSimulatorPolling();
  } else {
    stopSimulatorPolling();
  }
  
  // Console log functions
  async function loadConsoleLogs() {
    try {
      const logs = await GetConsoleLogs();
      consoleLogs.set(logs.map(log => ({
        timestamp: log.timestamp || log.Timestamp || new Date().toLocaleTimeString(),
        message: log.message || log.Message || '',
        level: log.level || log.Level || 'info'
      })));
    } catch (e) {
      console.error('Failed to load console logs:', e);
    }
  }
  
  function startConsoleLogsPolling() {
    if (consoleLogsInterval) return;
    loadConsoleLogs(); // Load immediately
    consoleLogsInterval = setInterval(loadConsoleLogs, 1000); // Poll every second
  }
  
  function stopConsoleLogsPolling() {
    if (consoleLogsInterval) {
      clearInterval(consoleLogsInterval);
      consoleLogsInterval = null;
    }
  }
  
  // Copy recent console logs to clipboard
  async function copyRecentLogs(lineCount) {
    const logs = $consoleLogs.slice(-lineCount);
    const text = logs.map(log => `[${log.timestamp}] ${log.message}`).join('\n');
    try {
      await navigator.clipboard.writeText(text);
    } catch (e) {
      console.error('Failed to copy logs:', e);
    }
  }
  
  // Clear console logs (both frontend and backend)
  async function handleClearLogs() {
    try {
      await ClearBackendLogs();
      clearConsoleLogs(); // Also clear frontend store
      previousLogCount = 0; // Reset count so auto-scroll works again
      consoleUserScrolled = false; // Reset scroll state
    } catch (e) {
      console.error('Failed to clear logs:', e);
    }
  }
  
  // Check if user is at bottom of console
  function handleConsoleScroll() {
    if (!consoleViewport) return;
    const { scrollTop, scrollHeight, clientHeight } = consoleViewport;
    // Consider "at bottom" if within 100px of the bottom (more generous threshold)
    const distanceFromBottom = scrollHeight - scrollTop - clientHeight;
    consoleUserScrolled = distanceFromBottom > 100;
  }
  
  // Auto-scroll console to bottom ONLY when new logs are added AND user is at bottom
  // This prevents the "fighting" behavior where scroll keeps jumping back
  $: if ($consoleLogs && consoleViewport) {
    const currentLogCount = $consoleLogs.length;
    // Only scroll if new logs were added (not just on any reactive trigger)
    if (currentLogCount > previousLogCount && !consoleUserScrolled) {
      // Use requestAnimationFrame for smoother scrolling
      requestAnimationFrame(() => {
        if (consoleViewport && !consoleUserScrolled) {
          consoleViewport.scrollTop = consoleViewport.scrollHeight;
        }
      });
    }
    previousLogCount = currentLogCount;
  }
  
  // Start polling when Console section becomes active
  $: if (activeSection === 'console') {
    startConsoleLogsPolling();
  } else {
    stopConsoleLogsPolling();
  }
  
  async function loadPermissionTypes() {
    try {
      permissionTypes = await GetPermissionTypes();
    } catch (e) {
      console.error('Failed to load permission types:', e);
    }
  }
  
  async function loadConnectedApps() {
    isLoadingApps = true;
    try {
      connectedApps = await GetConnectedApps();
    } catch (e) {
      console.error('Failed to load connected apps:', e);
      connectedApps = [];
    } finally {
      isLoadingApps = false;
    }
  }
  
  async function revokeAllPermissions(origin) {
    try {
      await RevokeAppPermissions(origin);
      await loadConnectedApps();
      selectedApp = null;
    } catch (e) {
      console.error('Failed to revoke permissions:', e);
    }
  }
  
  async function revokePermission(origin, permission) {
    try {
      await RevokeAppPermission(origin, permission);
      await loadConnectedApps();
    } catch (e) {
      console.error('Failed to revoke permission:', e);
    }
  }
  
  function formatTimestamp(ts) {
    if (!ts) return 'Unknown';
    const date = new Date(ts * 1000);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString();
  }
  
  function getPermissionLabel(permId) {
    const perm = permissionTypes.find(p => p.id === permId);
    return perm?.name || permId;
  }
  
  // Load apps when section becomes active
  $: if (activeSection === 'connected-apps') {
    loadConnectedApps();
  }
  
  // Load privacy mode data when section becomes active
  $: if (activeSection === 'privacy') {
    loadPrivacyModeData();
  }
  
  async function loadPrivacyModeData() {
    privacyLoading = true;
    try {
      const status = await GetNetworkFilterStatus();
      if (status.success) {
        privacyModeEnabled = status.enabled;
        allowedHosts = status.allowedHosts || [];
      }
      
      const log = await GetConnectionLog(50);
      if (log.success) {
        connectionLog = log.log || [];
      }
      
      const connections = await GetActiveConnections();
      if (connections.success) {
        activeConnections = connections.connections || [];
      }
    } catch (e) {
      console.error('Failed to load privacy mode data:', e);
    } finally {
      privacyLoading = false;
    }
  }
  
  async function togglePrivacyMode() {
    try {
      const result = await SetCypherpunkMode(!privacyModeEnabled);
      if (result.success) {
        privacyModeEnabled = result.enabled;
      }
    } catch (e) {
      console.error('Failed to toggle privacy mode:', e);
    }
  }
  
  async function addHost() {
    if (!newAllowedHost.trim()) return;
    try {
      const result = await AddAllowedHost(newAllowedHost.trim());
      if (result.success) {
        await loadPrivacyModeData();
        newAllowedHost = '';
      }
    } catch (e) {
      console.error('Failed to add host:', e);
    }
  }
  
  async function removeHost(host) {
    try {
      const result = await RemoveAllowedHost(host);
      if (result.success) {
        await loadPrivacyModeData();
      }
    } catch (e) {
      console.error('Failed to remove host:', e);
    }
  }
  
  async function clearLog() {
    try {
      await ClearConnectionLog();
      connectionLog = [];
    } catch (e) {
      console.error('Failed to clear log:', e);
    }
  }
  
  onDestroy(() => {
    if (statusInterval) clearInterval(statusInterval);
    if (simulatorStatusInterval) clearInterval(simulatorStatusInterval);
    if (epochStatsInterval) clearInterval(epochStatsInterval);
    if (consoleLogsInterval) clearInterval(consoleLogsInterval);
    EventsOff('network-mode-changed');
    if (window._settingsNavigateHandler) {
      window.removeEventListener('navigate-section', window._settingsNavigateHandler);
      window._settingsNavigateHandler = null;
    }
  });
  
  async function refreshNodeStatus() {
    try {
      // Check derod installation status
      derodStatus = await CheckDerodStatus();
      
      // Check for running node
      const nodeCheck = await GetNodeStatus();
      nodeStatus = nodeCheck;
      
      // If node is running, start polling for sync status
      if (nodeStatus.isRunning) {
        startSyncPolling();
      }
    } catch (error) {
      console.error('Failed to refresh node status:', error);
    }
  }
  
  function startSyncPolling() {
    if (statusInterval) clearInterval(statusInterval);
    statusInterval = setInterval(async () => {
      if (nodeStatus.isRunning) {
        syncProgress = await GetSyncProgress();
      }
    }, 5000);
  }
  
  async function checkLatestRelease() {
    latestRelease = await GetLatestDerodRelease();
  }
  
  async function downloadDerod() {
    isDownloading = true;
    try {
      const result = await DownloadDerodFromGitHub();
      if (result.success) {
        derodStatus = await CheckDerodStatus();
      } else {
        console.error('Download failed:', result.error);
      }
    } catch (error) {
      console.error('Download error:', error);
    } finally {
      isDownloading = false;
    }
  }
  
  // Node start/stop controls moved to Network page
  
  async function detectExternalNode() {
    detecting = true;
    detectionMessage = '';
    try {
      const result = await DetectRunningNode();
      if (result.found) {
        // Update settings with detected endpoint
        await updateSetting('daemonEndpoint', result.endpoint);
        detectionMessage = `Found node at ${result.endpoint}`;
      } else {
        detectionMessage = 'ℹ️ No running node detected';
      }
    } catch (error) {
      detectionMessage = `Error: ${error.message || 'Failed to detect node'}`;
    } finally {
      detecting = false;
      // Clear message after 5 seconds
      setTimeout(() => detectionMessage = '', 5000);
    }
  }
  
  async function updateSetting(key, value) {
    // Use the centralized saveSetting which handles key mapping
    await saveSetting(key, value);
  }
  
  // Handle network mode change - syncs with backend
  async function handleNetworkChange(newNetwork) {
    try {
      // Update backend network mode
      const result = await SetNetworkMode(newNetwork);
      if (result.success) {
        // Sync network mode from backend (this updates appState and settingsState)
        await syncNetworkMode();
        // Also update settings for compatibility
        await updateSetting('network', newNetwork);
      } else {
        console.error('Failed to set network mode:', result.error);
      }
    } catch (error) {
      console.error('Failed to change network mode:', error);
    }
  }
  
  async function handleGnomonToggle() {
    if ($appState.gnomonRunning) {
      await StopGnomon();
    } else {
      await StartGnomon();
    }
  }
  
  async function handleResyncGnomon() {
    if ($appState.gnomonRunning) {
      console.warn('[Gnomon] Cannot resync while running');
      return;
    }
    
    resyncingGnomon = true;
    try {
      const result = await ResyncGnomon();
      if (result.success) {
        console.log('[Gnomon] Resync started:', result.message);
      } else {
        console.error('[Gnomon] Resync failed:', result.error);
      }
    } catch (e) {
      console.error('[Gnomon] Resync error:', e);
    } finally {
      resyncingGnomon = false;
    }
  }
  
  // Search exclusions functions
  async function loadSearchExclusions() {
    try {
      const result = await GetSearchExclusions();
      if (result.success) {
        searchExclusions = result.exclusions || [];
        searchMinLikes = result.minLikes || 0;
      }
    } catch (e) {
      console.error('[Exclusions] Load failed:', e);
    }
  }
  
  async function addExclusion() {
    const filter = newExclusionFilter.trim();
    if (!filter) return;
    
    try {
      const result = await AddSearchExclusion(filter);
      if (result.success) {
        searchExclusions = result.exclusions || [];
        newExclusionFilter = '';
      }
    } catch (e) {
      console.error('[Exclusions] Add failed:', e);
    }
  }
  
  async function removeExclusion(filter) {
    try {
      const result = await RemoveSearchExclusion(filter);
      if (result.success) {
        searchExclusions = result.exclusions || [];
      }
    } catch (e) {
      console.error('[Exclusions] Remove failed:', e);
    }
  }
  
  async function clearAllExclusions() {
    if (!confirm('Clear all search exclusion filters?')) return;
    
    try {
      const result = await ClearSearchExclusions();
      if (result.success) {
        searchExclusions = [];
      }
    } catch (e) {
      console.error('[Exclusions] Clear failed:', e);
    }
  }
  
  async function updateMinLikes() {
    try {
      await SetSearchMinLikes(searchMinLikes);
    } catch (e) {
      console.error('[Exclusions] Set min likes failed:', e);
    }
  }
  
  // Handle Gnomon auto-start toggle
  async function handleGnomonAutostartToggle() {
    gnomonAutostart = !gnomonAutostart;
    try {
      await SetGnomonAutostart(gnomonAutostart);
    } catch (e) {
      console.error('[Gnomon] Failed to save auto-start setting:', e);
      gnomonAutostart = !gnomonAutostart; // Revert on error
    }
  }
  
  // Load autostart setting
  async function loadGnomonAutostart() {
    try {
      gnomonAutostart = await GetGnomonAutostart();
    } catch (e) {
      console.error('[Gnomon] Failed to load auto-start setting:', e);
    }
  }
  
  // Load settings on gnomon section activation
  $: if (activeSection === 'gnomon') {
    loadSearchExclusions();
    loadGnomonAutostart();
  }
  
</script>

<div class="page-layout">
  <!-- v5.6 Page Header -->
  <div class="page-header">
    <div class="page-header-inner">
      <div class="page-header-left">
        <h1 class="page-header-title">
          <SettingsIcon size={18} class="page-header-icon" strokeWidth={1.5} />
          Settings
        </h1>
        <p class="page-header-desc">Configure application preferences</p>
      </div>
    </div>
  </div>
  
  <!-- v5.6 Unified Page Body -->
  <div class="page-body">
    <!-- Sidebar -->
    <div class="page-sidebar">
      <div class="page-sidebar-section">SECTIONS</div>
      <nav class="page-sidebar-nav">
      {#each sections as section}
        <button
          on:click={() => activeSection = section.id}
            class="page-sidebar-item"
          class:active={activeSection === section.id}
        >
            <span class="page-sidebar-item-icon">
              <Icons name={section.iconName} size={14} />
          </span>
            <span class="page-sidebar-item-label">{section.label}</span>
        </button>
      {/each}
    </nav>
  </div>
  
    <!-- Content Area -->
    <div class="page-content">
      {#if activeSection === 'general'}
        <!-- Content Filtering -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">CONTENT FILTERING</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row" style="flex-direction: column; align-items: stretch;">
              <div class="form-group" style="margin-bottom: var(--s-4);">
              <div class="slider-header">
                <span class="form-label">Minimum Content Rating</span>
                <span class="slider-value c-cyan">{$settingsState.minRating}</span>
              </div>
              <input
                type="range"
                min="0"
                max="99"
                bind:value={$settingsState.minRating}
                on:change={(e) => updateSetting('minRating', parseInt(e.target.value))}
                class="slider"
              />
              </div>
            </div>
            
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Block Malware</div>
                <div class="settings-row-desc">Block content rated 0-9 (potentially harmful)</div>
              </div>
                <input
                  type="checkbox"
                  bind:checked={$settingsState.blockMalware}
                  on:change={(e) => updateSetting('blockMalware', e.target.checked)}
                class="toggle"
                />
            </div>
            
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Show NSFW Content</div>
                <div class="settings-row-desc">Display adult content when available</div>
              </div>
                <input
                  type="checkbox"
                  bind:checked={$settingsState.showNSFW}
                  on:change={(e) => updateSetting('showNSFW', e.target.checked)}
                class="toggle"
                />
            </div>
          </div>
        </div>
        
        <!-- Connection -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◆</span>
              <span class="explorer-header-title">CONNECTION</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Auto-connect XSWD</div>
                <div class="settings-row-desc">Automatically connect to XSWD wallet service on startup</div>
              </div>
                <input
                  type="checkbox"
                  bind:checked={$settingsState.autoConnectXSWD}
                  on:change={(e) => updateSetting('autoConnectXSWD', e.target.checked)}
                class="toggle"
                />
            </div>

            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Integrated Wallet Modal</div>
                <div class="settings-row-desc">Handle dApp connections and signing directly in Hologram</div>
              </div>
                <input
                  type="checkbox"
                  bind:checked={$settingsState.integratedWallet}
                  on:change={(e) => updateSetting('integratedWallet', e.target.checked)}
                class="toggle"
                />
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'node'}
        <!-- Node Status -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">NODE STATUS</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Status</div>
                <div class="settings-row-desc">
                {#if nodeStatus.isRunning}
                  Running - {syncProgress.isSynced ? 'Synced' : `Syncing ${syncProgress.progress?.toFixed(1) || 0}%`}
                {:else}
                  Not running
                {/if}
            </div>
              </div>
              <span class="settings-hint-label">
                <Icons name="server" size={12} />
                Node controls below
              </span>
          </div>
          
          {#if nodeStatus.isRunning}
              <div class="settings-row" style="flex-direction: column; align-items: stretch;">
                <div class="sync-header">
                  <span class="settings-row-label">Sync Progress</span>
                  <span class="sync-progress-value">{syncProgress.progress?.toFixed(2) || 0}%</span>
              </div>
                <div class="progress mb-4">
                  <div class="progress-bar" style="width: {syncProgress.progress || 0}%"></div>
              </div>
                <div class="stat-grid">
                  <div class="stat-block">
                    <span class="stat-label">Height</span>
                    <span class="stat-value">{(syncProgress.topoHeight || 0).toLocaleString()}</span>
                </div>
                  <div class="stat-block">
                    <span class="stat-label">Peers</span>
                    <span class="stat-value">{syncProgress.peers || 0}</span>
                </div>
              </div>
            </div>
          {/if}
          </div>
        </div>
        
        <!-- Node Binary -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">▣</span>
              <span class="explorer-header-title">NODE BINARY</span>
            </div>
          </div>
          <div class="card-content">
          {#if derodStatus.installed}
              <div class="settings-row">
                <div class="settings-row-info">
                  <div class="settings-row-label">DERO Node (derod)</div>
                  <div class="settings-row-desc installed-info">
                    <span class="badge badge-ok">Installed</span>
                    <span class="version-text">{derodStatus.version || 'Unknown version'}</span>
              </div>
                </div>
                <button on:click={checkLatestRelease} class="btn btn-secondary btn-sm">Check for Updates</button>
            </div>
            {#if latestRelease && latestRelease.tagName !== derodStatus.version}
                <div class="alert alert-info mt-3">
                  <p class="update-notice">New version available: {latestRelease.tagName}</p>
                  <button on:click={downloadDerod} disabled={isDownloading} class="btn btn-primary btn-sm mt-2">
                  {isDownloading ? 'Downloading...' : 'Update'}
                </button>
              </div>
            {/if}
          {:else}
              <div class="settings-row" style="flex-direction: column; align-items: stretch;">
                <div class="settings-row-info" style="margin-bottom: var(--s-4);">
                  <div class="settings-row-label">DERO Node Binary</div>
                  <div class="settings-row-desc">The DERO node binary (derod) is required to run a local node.</div>
                </div>
                <button on:click={downloadDerod} disabled={isDownloading} class="btn btn-primary">
              {isDownloading ? 'Downloading...' : 'Download DERO Node'}
            </button>
            {#if isDownloading}
                  <p class="form-hint mt-3">This may take a few minutes (~50MB download)</p>
            {/if}
              </div>
          {/if}
          </div>
        </div>
        
        <!-- Data Directory -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◆</span>
              <span class="explorer-header-title">DATA DIRECTORY</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row" style="flex-direction: column; align-items: stretch;">
              <div class="form-group" style="margin-bottom: 0;">
                <label class="form-label">Blockchain Data Location</label>
            <input
              type="text"
              bind:value={nodeDataDir}
              placeholder="~/.dero/mainnet (default)"
              disabled={nodeStatus.isRunning}
                  class="input"
            />
                <p class="form-hint">Leave empty to use default location based on network</p>
              </div>
            </div>
          </div>
        </div>
        
        <!-- Advanced Options -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">⬢</span>
              <span class="explorer-header-title">ADVANCED OPTIONS</span>
            </div>
          </div>
          <div class="card-content">
            <p class="form-hint mb-4">These settings apply on the next node start</p>
          
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Fast Sync</div>
                <div class="settings-row-desc">Skip full block validation during initial sync (faster but less secure)</div>
              </div>
              <input 
                type="checkbox" 
                bind:checked={fastSyncEnabled}
                on:change={saveAdvancedNodeConfig}
                disabled={nodeStatus.isRunning}
                class="toggle"
              />
              </div>
            
            <div class="settings-row" style="flex-direction: column; align-items: stretch;" class:opacity-50={nodeStatus.isRunning}>
              <div class="form-group" style="margin-bottom: 0;">
                <label class="form-label">Prune History (blocks)</label>
              <div class="prune-input-row">
                <input
                  type="number"
                  bind:value={pruneHistory}
                  on:change={saveAdvancedNodeConfig}
                  min="0"
                  max="100000"
                  step="1000"
                  disabled={nodeStatus.isRunning}
                    class="input"
                    style="width: 150px;"
                />
                  <span class="form-hint">0 = keep all blocks</span>
              </div>
                <p class="form-hint">Remove blocks older than N to save disk space (minimum 1000 if enabled)</p>
              </div>
            </div>
            
            {#if nodeStatus.isRunning}
              <p class="form-hint c-warn mt-2">Stop the node to change these settings</p>
            {/if}
          </div>
        </div>
        
        <!-- External Node -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">EXTERNAL NODE</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Connect to External Node</div>
                <div class="settings-row-desc">Connect to an external DERO node instead of running your own</div>
              </div>
              <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 8px;">
                <button on:click={detectExternalNode} class="btn btn-secondary" disabled={detecting}>
                  {detecting ? 'Detecting...' : 'Detect Running Node'}
                </button>
                {#if detectionMessage}
                  <span style="font-size: 11px; color: var(--text-3); white-space: nowrap;">
                    {detectionMessage}
                  </span>
                {/if}
              </div>
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'simulator'}
        <!-- Simulator Status -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">SIMULATOR STATUS</span>
            </div>
            <div class="explorer-header-right">
              <DotIndicator status={simulatorStatus.isInitialized ? 'ok' : (simulatorStatus.isStarting ? 'warn' : 'err')} />
              <span class="explorer-header-meta">{simulatorStatus.isInitialized ? 'Running' : (simulatorStatus.isStarting ? 'Starting' : 'Stopped')}</span>
            </div>
          </div>
          <div class="card-content">
            {#if simulatorError}
              <div class="alert alert-danger" on:click={clearSimulatorMessages}>{simulatorError}</div>
            {/if}
            {#if simulatorSuccess}
              <div class="alert alert-success" on:click={clearSimulatorMessages}>{simulatorSuccess}</div>
            {/if}
            
            {#if !simulatorStatus.isInitialized}
              <div class="settings-row" style="flex-direction: column; align-items: stretch;">
                <div class="settings-row-info" style="margin-bottom: var(--s-4);">
                  <div class="settings-row-label">Local Test Environment</div>
                  <div class="settings-row-desc">Perfect for testing smart contracts and dApps. No real value.</div>
                </div>
                <button 
                  class="btn btn-primary"
                  on:click={startSimulator}
                  disabled={simulatorLoading || simulatorStatus.isStarting}
                >
                  {simulatorLoading || simulatorStatus.isStarting ? 'Starting Simulator...' : 'Start Simulator'}
                </button>
              </div>
            {:else}
              <div class="settings-row">
                <div class="settings-row-info">
                  <div class="settings-row-label">Status</div>
                  <div class="settings-row-desc c-emerald">Running</div>
                </div>
              </div>
              <div class="stat-grid">
                <div class="stat-block">
                  <span class="stat-label">BLOCK HEIGHT</span>
                  <span class="stat-value">{simulatorStatus.blockHeight?.toLocaleString() || '0'}</span>
                </div>
              </div>
              {#if simulatorStatus.walletAddress}
                <div class="settings-row" style="flex-direction: column; align-items: stretch; gap: var(--s-2);">
                  <div class="settings-row-info">
                    <div class="settings-row-label">Wallet</div>
                    <div class="settings-row-desc mono">{formatSimulatorAddress(simulatorStatus.walletAddress)}</div>
                  </div>
                  <div class="sim-wallet-balance-row">
                    <span class="sim-wallet-balance-label">Balance:</span>
                    <span class="sim-wallet-balance-value">{simulatorStatus.balanceDERO?.toFixed(5) || '0'} DERO</span>
                    <span class="sim-wallet-balance-hint">(receives mining rewards)</span>
                  </div>
                </div>
              {/if}
            {/if}
          </div>
        </div>
        
        {#if simulatorStatus.isInitialized}
          <!-- Controls -->
          <div class="card-wrapper">
            <div class="explorer-header">
              <div class="explorer-header-left">
                <span class="explorer-header-icon">▣</span>
                <span class="explorer-header-title">CONTROLS</span>
              </div>
            </div>
            <div class="card-content">
              <div class="settings-row">
                <div class="settings-row-info">
                  <div class="settings-row-label">Stop Simulator</div>
                  <div class="settings-row-desc">Shut down the local test environment</div>
                </div>
                <button class="btn btn-danger btn-sm" on:click={stopSimulator} disabled={simulatorLoading}>Stop</button>
              </div>
              <div class="settings-row">
                <div class="settings-row-info">
                  <div class="settings-row-label">Reset All Data</div>
                  <div class="settings-row-desc">Delete all simulator data and start fresh</div>
                </div>
                <button class="btn btn-ghost btn-sm" on:click={resetSimulator} disabled={simulatorLoading}>Reset</button>
              </div>
            </div>
          </div>
        {:else}
          <!-- About Simulator -->
          <div class="card-wrapper">
            <div class="explorer-header">
              <div class="explorer-header-left">
                <span class="explorer-header-icon">?</span>
                <span class="explorer-header-title">ABOUT SIMULATOR</span>
              </div>
            </div>
            <div class="card-content">
              <div class="info-list">
                <p class="info-item">
                  <strong class="c-cyan">Local Daemon</strong> - Launches a private DERO blockchain for testing.
                </p>
                <p class="info-item">
                  <strong class="c-cyan">Test Wallet</strong> - Creates a wallet automatically with test funds.
                </p>
                <p class="info-item">
                  <strong class="c-cyan">No Real Value</strong> - Perfect for development without risking real DERO.
                </p>
              </div>
            </div>
          </div>
        {/if}
      
      {:else if activeSection === 'servers'}
        <!-- TELA Servers Management -->
        <ServerManager />
      
      {:else if activeSection === 'offline-cache'}
        <!-- Offline Cache Manager -->
        <OfflineCacheManager />
      
      {:else if activeSection === 'sync-manager'}
        <!-- Sync Manager - Batch prefetch & updates -->
        <SyncManager />
      
      {:else if activeSection === 'safe-browsing'}
        <!-- Safe Browsing Settings -->
        <SafeBrowsingSettings />
      
      {:else if activeSection === 'developer-support'}
        <!-- Developer Support (Unified EPOCH + Passive Hashing) Section -->
        
        <!-- Status Card (with enable toggle in header) -->
        <div class="card-wrapper" style="margin-bottom: var(--s-6);">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">DEVELOPER SUPPORT</span>
            </div>
            <div class="explorer-header-right">
              <label class="toggle-label">
                <span class="toggle-text">{epochEnabled ? 'Enabled' : 'Disabled'}</span>
                <input
                  type="checkbox"
                  bind:checked={epochEnabled}
                  on:change={handleToggleEpoch}
                  class="toggle"
                />
              </label>
            </div>
          </div>
          <div class="card-content">
            <div class="dev-support-status">
              <div class="status-indicator" class:active={epochEnabled && epochStats.worker_running && !epochStats.paused} class:paused={epochStats.paused} class:disabled={!epochEnabled}>
                <span class="status-dot"></span>
                <span class="status-text">
                  {#if !epochEnabled}
                    Disabled
                  {:else if epochStats.paused}
                    Paused
                  {:else if epochStats.worker_running}
                    Actively Supporting
                  {:else if epochStats.active}
                    Ready
                  {:else}
                    Connecting...
                  {/if}
                </span>
              </div>
              
              {#if epochStats.paused && epochStats.pause_reason}
                <div class="pause-reason-box">
                  <p class="pause-reason">
                    <Icons name="info" size={14} />
                    {epochStats.pause_reason}
                  </p>
                  {#if epochStats.pause_reason.includes('node')}
                    <button class="btn btn-sm btn-outline" on:click={() => activeSection = 'node'}>
                      <Icons name="server" size={14} />
                      Go to Node Settings
                    </button>
                  {/if}
                </div>
              {:else if epochEnabled && epochStats.worker_running}
                <p class="active-info">
                  <Icons name="zap" size={14} />
                  50 hashes every 5 seconds • 1-2 threads
                </p>
              {/if}
            </div>
            
            {#if epochError}
              <div class="alert alert-warn" style="margin-top: var(--s-4);">
                {epochError}
                <button on:click={handleRetryEpoch} class="btn btn-sm btn-outline mt-2">
                  Retry Connection
                </button>
              </div>
            {/if}
          </div>
        </div>
        
        <!-- Your Contributions Card -->
        <div class="card-wrapper" style="margin-bottom: var(--s-6);">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◆</span>
              <span class="explorer-header-title">YOUR CONTRIBUTIONS</span>
            </div>
          </div>
          <div class="card-content">
            <div class="contributions-grid">
              <div class="contribution-stat">
                <span class="contribution-value c-cyan">
                  {epochStats.total_hashes_str || devSupportStats?.total_hashes_str || '0'}
                </span>
                <span class="contribution-label">Total Hashes</span>
              </div>
              <div class="contribution-stat">
                <span class="contribution-value c-emerald">
                  {epochStats.total_miniblocks || devSupportStats?.miniblocks_found || 0}
                </span>
                <span class="contribution-label">Miniblocks Found</span>
              </div>
              <div class="contribution-stat">
                <span class="contribution-value">
                  {devSupportStats?.uptime_formatted || formatUptime(epochStats.uptime_seconds || 0)}
                </span>
                <span class="contribution-label">Support Time</span>
              </div>
            </div>
            
            {#if devSupportStats?.total_sessions}
              <p class="sessions-info">
                Across {devSupportStats.total_sessions} session{devSupportStats.total_sessions !== 1 ? 's' : ''}
              </p>
            {/if}
          </div>
        </div>
        
        <!-- How It Works Card -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">?</span>
              <span class="explorer-header-title">HOW IT WORKS</span>
            </div>
          </div>
          <div class="card-content">
            <div class="info-grid">
              <div class="info-item">
                <div class="info-icon"><Icons name="zap" size={18} /></div>
                <div class="info-content">
                  <strong>Passive Support</strong>
                  <p>Light background hashing runs continuously while Hologram is open, contributing to development.</p>
                </div>
              </div>
              <div class="info-item">
                <div class="info-icon"><Icons name="cpu" size={18} /></div>
                <div class="info-content">
                  <strong>Minimal Impact</strong>
                  <p>Only 50 hashes every 5 seconds using 1-2 threads. Automatically pauses when you're mining or on battery.</p>
                </div>
              </div>
              <div class="info-item">
                <div class="info-icon"><Icons name="globe" size={18} /></div>
                <div class="info-content">
                  <strong>TELA App Support</strong>
                  <p>When you use TELA dApps, they can also request small contributions to support their developers.</p>
                </div>
              </div>
              <div class="info-item">
                <div class="info-icon"><Icons name="shield" size={18} /></div>
                <div class="info-content">
                  <strong>Privacy First</strong>
                  <p>No personal data collected. Only anonymous proof-of-work. Disable anytime.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'network'}
        <!-- Network Settings Section -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">NETWORK SETTINGS</span>
            </div>
          </div>
          <div class="card-content">
            <!-- Network Radio Cards -->
            <div class="radio-card-group">
              <!-- Mainnet -->
              <button
                on:click={() => handleNetworkChange('mainnet')}
                class="radio-card"
                class:selected={$appState.network === 'mainnet'}
              >
                <div class="radio-card-radio"></div>
                <div class="radio-card-content">
                  <div class="radio-card-title">Mainnet</div>
                  <div class="radio-card-desc">Live blockchain</div>
                  <span class="radio-card-badge warn">Permanent • Costs DERO</span>
                </div>
              </button>
              
              <!-- Testnet -->
              <button
                on:click={() => handleNetworkChange('testnet')}
                class="radio-card"
                class:selected={$appState.network === 'testnet'}
              >
                <div class="radio-card-radio"></div>
                <div class="radio-card-content">
                  <div class="radio-card-title">Testnet</div>
                  <div class="radio-card-desc">Test blockchain</div>
                  <span class="radio-card-badge test">Use testnet DERO</span>
                </div>
              </button>
              
              <!-- Simulator -->
              <button
                on:click={() => handleNetworkChange('simulator')}
                class="radio-card"
                class:selected={$appState.network === 'simulator'}
              >
                <div class="radio-card-radio"></div>
                <div class="radio-card-content">
                  <div class="radio-card-title">Simulator</div>
                  <div class="radio-card-desc">Local simulation</div>
                  <span class="radio-card-badge safe">Safe • No Cost</span>
                </div>
              </button>
            </div>
            
            <!-- Network Info -->
            <div class="setting-row" style="background: transparent; border: none; padding: var(--s-3); margin: 0;">
              {#if $appState.network === 'mainnet'}
                <span style="color: var(--text-4); font-size: 12px;"><strong class="c-err">Mainnet:</strong> All transactions are permanent and cost real DERO.</span>
              {:else if $appState.network === 'testnet'}
                <span style="color: var(--text-4); font-size: 12px;"><strong class="c-warn">Testnet:</strong> Test blockchain with testnet DERO.</span>
              {:else if $appState.network === 'simulator'}
                <span style="color: var(--text-4); font-size: 12px;"><strong class="c-ok">Simulator:</strong> Local blockchain simulation. No cost.</span>
              {:else}
                <span style="color: var(--text-4); font-size: 12px;">Network: {$appState.network || 'mainnet'}</span>
              {/if}
            </div>
          </div>
        </div>
        
        <!-- Daemon Connection Section -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◆</span>
              <span class="explorer-header-title">DAEMON CONNECTION</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <span class="settings-row-label">Daemon Endpoint</span>
                <span class="settings-row-desc">Default ports: Mainnet (10102), Testnet (40402), Simulator (20000)</span>
              </div>
            </div>
              <input
                type="text"
                bind:value={$settingsState.daemonEndpoint}
                on:blur={(e) => updateSetting('daemonEndpoint', e.target.value)}
                class="input"
              style="margin-top: 8px;"
            />
            
            <div class="settings-row" style="margin-top: 16px;">
              <div class="settings-row-info">
                <span class="settings-row-label">Connection Status</span>
              </div>
              <div class="connection-badge {$appState.nodeConnected ? 'connected' : 'disconnected'}">
                <span class="connection-dot"></span>
                {$appState.nodeConnected ? 'Connected' : 'Disconnected'}
              </div>
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'gnomon'}
        <!-- Gnomon Indexer Section -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">GNOMON INDEXER</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Indexer Status</div>
                <div class="settings-row-desc">
                  {#if $appState.gnomonRunning}
                    <span class="c-cyan">{$appState.gnomonIndexedHeight.toLocaleString()}</span>
                    <span style="color: var(--text-4);"> / </span>
                    <span>{$appState.chainHeight.toLocaleString()} blocks</span>
                  {:else}
                    <span style="color: var(--text-4);">Not running</span>
                  {/if}
                </div>
              </div>
              <button
                on:click={handleGnomonToggle}
                class="btn {$appState.gnomonRunning ? 'btn-danger' : 'btn-primary'}"
              >
                <Icons name={$appState.gnomonRunning ? 'x' : 'play'} size={14} />
                {$appState.gnomonRunning ? 'Stop' : 'Start'}
              </button>
            </div>
            
            <!-- Auto-start Setting -->
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Auto-start on Launch</div>
                <div class="settings-row-desc">
                  Automatically start Gnomon when HOLOGRAM opens
                </div>
              </div>
              <button
                class="toggle-switch"
                class:active={gnomonAutostart}
                on:click={handleGnomonAutostartToggle}
                aria-pressed={gnomonAutostart}
              >
                <span class="toggle-slider"></span>
              </button>
            </div>
            
            {#if $appState.gnomonRunning}
              <div class="progress-section">
                <div class="progress-header">
                  <span class="form-label">Progress</span>
                  <span class="progress-value c-cyan">{$appState.gnomonProgress.toFixed(2)}%</span>
                </div>
                <div class="progress">
                  <div 
                    class="progress-bar"
                    style="width: {$appState.gnomonProgress}%"
                  ></div>
                </div>
              </div>
            {/if}
            
            <!-- Database Management -->
            <div class="settings-row" style="margin-top: var(--s-4); border-top: 1px solid var(--border-subtle); padding-top: var(--s-4);">
              <div class="settings-row-info">
                <div class="settings-row-label">Database Management</div>
                <div class="settings-row-desc">
                  Reset the Gnomon database to re-sync from scratch.
                  <span style="color: var(--status-warn);">This will delete all indexed data.</span>
                </div>
              </div>
              <button
                on:click={handleResyncGnomon}
                disabled={$appState.gnomonRunning || resyncingGnomon}
                class="btn btn-secondary"
                title={$appState.gnomonRunning ? 'Stop Gnomon first' : 'Resync Gnomon database'}
              >
                <Icons name={resyncingGnomon ? 'loader' : 'refresh'} size={14} />
                {resyncingGnomon ? 'Resyncing...' : 'Resync Database'}
              </button>
            </div>
            
            <!-- Search Tips -->
            <div class="settings-row" style="margin-top: var(--s-3);">
              <div class="settings-row-info">
                <div class="settings-row-label">Search Tips</div>
                <div class="settings-row-desc" style="font-family: var(--font-mono); font-size: 12px; line-height: 1.6;">
                  <strong>key:</strong>owner - Search by SC key<br>
                  <strong>value:</strong>TELA - Search by SC value<br>
                  <strong>code:</strong>STORE - Search SC code
                </div>
              </div>
            </div>
            
            <!-- Search Exclusions Filter -->
            <div class="settings-row" style="margin-top: var(--s-4); border-top: 1px solid var(--border-subtle); padding-top: var(--s-4);">
              <div class="settings-row-info">
                <div class="settings-row-label">Search Exclusions</div>
                <div class="settings-row-desc">
                  Filter out content containing specific text in dURL.
                  <span style="color: var(--text-muted);">({searchExclusions.length} active filters)</span>
                </div>
              </div>
              <div class="settings-row-actions">
                <button
                  on:click={() => showExclusionModal = true}
                  class="btn btn-secondary"
                >
                  <Icons name="filter" size={14} />
                  Manage
                </button>
                {#if searchExclusions.length > 0}
                  <button
                    on:click={clearAllExclusions}
                    class="btn btn-danger-outline"
                    title="Clear all exclusions"
                  >
                    <Icons name="trash-2" size={14} />
                  </button>
                {/if}
              </div>
            </div>
            
            <!-- Minimum Likes Filter -->
            <div class="settings-row" style="margin-top: var(--s-3);">
              <div class="settings-row-info">
                <div class="settings-row-label">Minimum Likes %</div>
                <div class="settings-row-desc">
                  Filter search results by minimum like ratio. 0 = show all.
                </div>
              </div>
              <div class="settings-row-actions">
                <input
                  type="number"
                  min="0"
                  max="100"
                  bind:value={searchMinLikes}
                  on:change={updateMinLikes}
                  class="form-input"
                  style="width: 70px; text-align: center;"
                />
                <span class="input-suffix">%</span>
              </div>
            </div>
            
            <!-- Active Exclusions List -->
            {#if searchExclusions.length > 0}
              <div class="exclusions-list" style="margin-top: var(--s-3);">
                <div class="exclusions-header">Active Filters:</div>
                <div class="exclusions-tags">
                  {#each searchExclusions as filter}
                    <span class="exclusion-tag">
                      {filter}
                      <button
                        class="exclusion-remove"
                        on:click={() => removeExclusion(filter)}
                        title="Remove filter"
                      >×</button>
                    </span>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        </div>
        
        <!-- Search Exclusions Modal -->
        {#if showExclusionModal}
          <div class="modal-overlay" on:click={() => showExclusionModal = false}>
            <div class="modal-content" on:click|stopPropagation>
              <div class="modal-header">
                <h3 class="modal-title">Search Exclusions</h3>
                <button class="modal-close" on:click={() => showExclusionModal = false}>
                  <Icons name="x" size={20} />
                </button>
              </div>
              <div class="modal-body">
                <p class="modal-desc">
                  Add text filters to exclude SCIDs containing these patterns in their dURL.
                  Useful for filtering out unwanted or low-quality content.
                </p>
                
                <div class="exclusion-input-row">
                  <input
                    type="text"
                    bind:value={newExclusionFilter}
                    placeholder="Enter filter text..."
                    class="form-input"
                    on:keydown={(e) => e.key === 'Enter' && addExclusion()}
                  />
                  <button
                    class="btn btn-primary"
                    on:click={addExclusion}
                    disabled={!newExclusionFilter.trim()}
                  >
                    <Icons name="plus" size={14} />
                    Add
                  </button>
                </div>
                
                {#if searchExclusions.length > 0}
                  <div class="exclusions-modal-list">
                    {#each searchExclusions as filter}
                      <div class="exclusion-item">
                        <span class="exclusion-text">{filter}</span>
                        <button
                          class="btn btn-sm btn-danger-outline"
                          on:click={() => removeExclusion(filter)}
                        >
                          <Icons name="trash-2" size={12} />
                        </button>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div class="empty-exclusions">
                    <Icons name="filter" size={32} />
                    <p>No exclusion filters set</p>
                  </div>
                {/if}
              </div>
              <div class="modal-footer">
                <button class="btn btn-secondary" on:click={() => showExclusionModal = false}>
                  Close
                </button>
              </div>
            </div>
          </div>
        {/if}
      
      {:else if activeSection === 'connected-apps'}
        <!-- Connected Apps -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">CONNECTED APPS</span>
            </div>
          </div>
          <div class="card-content">
            <p class="section-desc">
          Manage dApps that have connected to your wallet via XSWD. You can view and revoke permissions for each app.
        </p>
        
        {#if isLoadingApps}
              <div class="loading-container">
                <div class="loading-spinner"></div>
          </div>
        {:else if connectedApps.length === 0}
              <div class="empty-state-card">
                <div class="empty-state-icon">
                  <Icons name="lock" size={48} />
                </div>
                <h3 class="empty-state-title">No Connected Apps</h3>
                <p class="empty-state-desc">
              Apps that connect to your wallet via XSWD will appear here.
                  You'll be able to manage their permissions.
            </p>
          </div>
        {:else}
              <div class="apps-list">
            {#each connectedApps as app}
                  <div class="app-card">
                <!-- App Header -->
                <button
                  on:click={() => selectedApp = selectedApp === app.origin ? null : app.origin}
                      class="app-card-header"
                >
                      <div class="app-card-info">
                        <div class="app-card-icon">
                          <Icons name="link" size={24} />
                    </div>
                        <div class="app-card-details">
                          <div class="app-card-name-row">
                            <span class="app-card-name">{app.name || 'Unknown App'}</span>
                        {#if app.isActive}
                              <span class="app-status-badge connected">Connected</span>
                        {/if}
                      </div>
                          <p class="app-card-origin">{app.origin}</p>
                    </div>
                  </div>
                      <div class="app-card-meta">
                        <span class="app-perm-count">
                        {app.permissions?.length || 0} permission{app.permissions?.length !== 1 ? 's' : ''}
                      </span>
                        <Icons name={selectedApp === app.origin ? 'chevron-up' : 'chevron-down'} size={20} />
                  </div>
                </button>
                
                <!-- Expanded Details -->
                {#if selectedApp === app.origin}
                      <div class="app-card-expanded">
                    <!-- Timestamps -->
                        <div class="app-timestamps">
                          <div class="app-timestamp">
                            <span class="timestamp-label">First Connected</span>
                            <span class="timestamp-value">{formatTimestamp(app.grantedAt)}</span>
                      </div>
                          <div class="app-timestamp">
                            <span class="timestamp-label">Last Activity</span>
                            <span class="timestamp-value">{formatTimestamp(app.lastAccessed)}</span>
                      </div>
                    </div>
                    
                    <!-- Permissions -->
                        <div class="app-permissions">
                          <span class="permissions-label">Granted Permissions</span>
                      {#if app.permissions && app.permissions.length > 0}
                            <div class="permissions-list">
                          {#each app.permissions as perm}
                                <span class="permission-tag">
                                  <span class="permission-name">{getPermissionLabel(perm)}</span>
                              <button
                                on:click|stopPropagation={() => revokePermission(app.origin, perm)}
                                    class="permission-revoke"
                                title="Revoke this permission"
                              >
                                    <Icons name="x" size={14} />
                              </button>
                            </span>
                          {/each}
                        </div>
                      {:else}
                            <p class="no-permissions">No permissions granted</p>
                      {/if}
                    </div>
                    
                    <!-- Actions -->
                        <div class="app-actions">
                      <button
                        on:click|stopPropagation={() => revokeAllPermissions(app.origin)}
                            class="btn btn-danger"
                      >
                        Revoke All & Disconnect
                      </button>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
          
          <!-- Revoke All Button -->
          {#if connectedApps.length > 1}
                <div class="revoke-all-section">
              <button
                on:click={async () => {
                  for (const app of connectedApps) {
                    await RevokeAppPermissions(app.origin);
                  }
                  await loadConnectedApps();
                }}
                    class="btn btn-danger"
              >
                Revoke All Apps
              </button>
            </div>
          {/if}
        {/if}
          </div>
        </div>
        
        <!-- Permission Reference -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">▣</span>
              <span class="explorer-header-title">PERMISSION REFERENCE</span>
            </div>
          </div>
          <div class="card-content">
            <div class="permission-ref-list">
            {#each permissionTypes as perm}
                <div class="permission-ref-item">
                  <div class="permission-ref-icon">
                    {#if perm.id === 'view_address'}
                      <Icons name="eye" size={16} />
                    {:else if perm.id === 'view_balance'}
                      <Icons name="wallet" size={16} />
                    {:else if perm.id === 'sign_transaction'}
                      <Icons name="send" size={16} />
                    {:else}
                      <Icons name="lock" size={16} />
                    {/if}
                </div>
                  <div class="permission-ref-content">
                    <div class="permission-ref-header">
                      <span class="permission-ref-name">{perm.name}</span>
                    {#if perm.alwaysAsk}
                        <span class="permission-ref-badge">Always Asks</span>
                    {/if}
                  </div>
                    <p class="permission-ref-desc">{perm.description}</p>
                </div>
              </div>
            {/each}
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'privacy'}
        <!-- Privacy & Security -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">PRIVACY & SECURITY</span>
            </div>
          </div>
          <div class="card-content">
            <div class="settings-row">
              <div class="settings-row-info">
                <div class="settings-row-label">Privacy Mode</div>
                <div class="settings-row-desc">Block all non-DERO network connections</div>
            </div>
                <input
                  type="checkbox"
                  class="toggle"
                  checked={privacyModeEnabled}
                  on:change={togglePrivacyMode}
                />
          </div>
          
          {#if privacyModeEnabled}
              <div class="alert alert-success">Only DERO network and whitelisted hosts are allowed</div>
          {:else}
              <div class="alert alert-info">All network connections are allowed</div>
          {/if}
          </div>
        </div>
        
        <!-- Allowed Hosts -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">▣</span>
              <span class="explorer-header-title">ALLOWED HOSTS</span>
            </div>
          </div>
          <div class="card-content">
            <p class="form-hint" style="margin-bottom: var(--s-4);">
            These hosts are always allowed, even when Privacy Mode is enabled.
          </p>
          
          <!-- Add Host Form -->
          <div class="add-host-form">
            <input
              type="text"
              bind:value={newAllowedHost}
              placeholder="Enter hostname or IP..."
                class="input host-input"
              on:keydown={(e) => e.key === 'Enter' && addHost()}
            />
            <button
              on:click={addHost}
              disabled={!newAllowedHost.trim()}
                class="btn btn-primary"
            >
              Add
            </button>
          </div>
          
          <!-- Hosts List -->
          <div class="host-list">
            {#each allowedHosts as host}
              <div class="host-item">
                <span class="host-dot"></span>
                <span class="host-name">{host}</span>
                {#if !['127.0.0.1', 'localhost', '::1', '0.0.0.0'].includes(host)}
                  <button
                    on:click={() => removeHost(host)}
                    class="btn btn-ghost btn-sm"
                    style="margin-left: auto;"
                    title="Remove host"
                  >
                    ✕
                  </button>
                {:else}
                  <span class="connection-protocol" style="margin-left: auto;">Required</span>
                {/if}
              </div>
            {/each}
            {#if allowedHosts.length === 0}
              <div class="host-item" style="justify-content: center; color: var(--text-4);">
                No hosts configured
              </div>
            {/if}
          </div>
          </div>
        </div>
        
        <!-- Active Connections -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◆</span>
              <span class="explorer-header-title">ACTIVE CONNECTIONS</span>
            </div>
          </div>
          <div class="card-content">
            <p class="form-hint" style="margin-bottom: var(--s-4);">Current network connections used by Hologram</p>
          
          <div class="connection-list">
            {#each activeConnections as conn}
              <div class="connection-item">
                <span class="connection-dot" class:connection-dot-off={!conn.connected}></span>
                <span class="connection-name">{conn.name}</span>
                <span class="connection-protocol">{conn.type}</span>
                <span class="connection-url">{conn.endpoint}</span>
              </div>
            {/each}
            {#if activeConnections.length === 0}
              <div class="connection-item">
                <span class="connection-dot" class:connection-dot-off={!$appState.xswdConnected}></span>
                <span class="connection-name">XSWD (Wallet)</span>
                <span class="connection-protocol">WebSocket</span>
                <span class="connection-url">ws://127.0.0.1:44326</span>
                </div>
              <div class="connection-item">
                <span class="connection-dot" class:connection-dot-off={!$appState.nodeConnected}></span>
                <span class="connection-name">Daemon RPC</span>
                <span class="connection-protocol">HTTP</span>
                <span class="connection-url">{$appState.currentEndpoint || $settingsState.daemonEndpoint}</span>
              </div>
            {/if}
          </div>
          </div>
        </div>
        
        <!-- Connection Log -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">⬢</span>
              <span class="explorer-header-title">CONNECTION LOG</span>
            </div>
          </div>
          <div class="card-content">
            <div class="section-header">
              <p class="form-hint">Recent connection attempts</p>
            {#if connectionLog.length > 0}
              <button
                on:click={clearLog}
                  class="btn btn-ghost btn-sm"
              >
                Clear Log
              </button>
            {/if}
          </div>
          
          <div class="connection-log">
            {#if connectionLog.length === 0}
              <p class="connection-log-empty">No connection attempts logged</p>
            {:else}
              {#each [...connectionLog].reverse().slice(0, 20) as entry}
                <div class="connection-log-entry">
                  <span class="log-dot {entry.allowed ? 'log-dot-ok' : 'log-dot-err'}"></span>
                  <div class="log-entry-info">
                    <span class="log-entry-host">{entry.host || entry.url}</span>
                    <span class="log-entry-reason">{entry.reason}</span>
                  </div>
                  <span class="log-entry-status {entry.allowed ? 'c-emerald' : 'c-err'}">
                    {entry.allowed ? 'Allowed' : 'Blocked'}
                  </span>
                </div>
              {/each}
            {/if}
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'console'}
        <!-- Application Logs -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">APPLICATION LOGS</span>
            </div>
            <div class="explorer-header-right">
              <div class="console-actions">
                <button on:click={() => copyRecentLogs(25)} class="btn btn-ghost btn-sm" title="Copy last 25 lines">
                  Copy 25
                </button>
                <button on:click={() => copyRecentLogs(50)} class="btn btn-ghost btn-sm" title="Copy last 50 lines">
                  Copy 50
                </button>
                <button on:click={handleClearLogs} class="btn btn-ghost btn-sm" title="Clear all logs">
                  Clear
                </button>
              </div>
            </div>
          </div>
          <div class="card-content console-content">
            <div class="console-viewport" bind:this={consoleViewport} on:scroll={handleConsoleScroll}>
              {#if $consoleLogs.length === 0}
                <p class="console-empty">No logs yet. Application output will appear here.</p>
              {:else}
                {#each $consoleLogs as log}
                  <div class="console-line {log.level === 'error' ? 'level-error' : log.level === 'warn' ? 'level-warn' : ''}">
                    <span class="console-timestamp">[{log.timestamp}]</span>
                    <span class="console-message">{log.message}</span>
                  </div>
                {/each}
              {/if}
            </div>
          </div>
        </div>
      
      {:else if activeSection === 'about'}
        <!-- About Hologram -->
        <div class="card-wrapper">
          <div class="explorer-header">
            <div class="explorer-header-left">
              <span class="explorer-header-icon">◎</span>
              <span class="explorer-header-title">ABOUT HOLOGRAM</span>
            </div>
          </div>
          <div class="card-content">
            <div class="about-details">
              <div class="about-row">
                <span class="about-label">Version</span>
                <span class="about-value mono">{appInfo.version}</span>
              </div>
              <div class="about-row">
                <span class="about-label">Build Date</span>
                <span class="about-value mono">{appInfo.buildDate === 'dev' ? 'Development Build' : appInfo.buildDate}</span>
              </div>
              {#if appInfo.gitCommit && appInfo.gitCommit !== 'unknown'}
              <div class="about-row">
                <span class="about-label">Commit</span>
                <span class="about-value mono">{appInfo.gitCommit}</span>
              </div>
              {/if}
              <div class="about-row">
                <span class="about-label">DERO Node</span>
                <span class="about-value mono">{$appState.nodeVersion || 'Not connected'}</span>
              </div>
              <div class="about-row">
                <span class="about-label">Network</span>
                <span class="about-value">
                  <span class="about-network-badge {$settingsState.network}">
                    {$settingsState.network || 'mainnet'}
                  </span>
                </span>
              </div>
            </div>
            
            <div class="about-footer">
              <p class="about-copyright">© 2026 {appInfo.author || 'Hologram Contributors'}</p>
              <p class="about-tagline">Explore the DERO Decentralized Web</p>
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  /* === HOLOGRAM v7.0 Settings Page Styles === */
  /* Strict compliance with HOLOGRAM-DESIGN-SYSTEM.md */
  /* Utilitarian Card Headers (Explorer Style) */
  
  /* === Card Wrapper === */
  .card-wrapper {
    background: var(--void-mid);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    overflow: hidden;
    margin-bottom: var(--s-6, 24px);
  }
  
  /* === Explorer-style Headers === */
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
  
  .explorer-header-right {
    display: flex;
    align-items: center;
    gap: 16px;
  }
  
  .explorer-header-meta {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-4);
  }
  
  /* === Card Content === */
  .card-content {
    padding: 24px;
  }
  
  /* === Settings Row (Individual Setting) === */
  .settings-row {
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    margin-bottom: var(--s-3, 12px);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4, 16px);
  }
  
  .settings-row:last-child {
    margin-bottom: 0;
  }
  
  .settings-row-info {
    flex: 1;
  }
  
  .settings-row-actions {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex-shrink: 0;
  }
  
  .settings-row-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
    margin-bottom: 2px;
  }
  
  .settings-row-desc {
    font-size: 11px;
    color: var(--text-4, #505068);
    line-height: 1.5;
  }
  
  .settings-row-control {
    flex-shrink: 0;
  }
  
  .settings-hint-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 11px;
    color: var(--text-5);
    font-style: italic;
  }
  
  /* === Settings Stats Grid === */
  .settings-stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
    gap: var(--s-3, 12px);
  }
  
  .settings-stat {
    padding: var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-sm, 5px);
    text-align: center;
  }
  
  .settings-stat-value {
    display: block;
    font-family: var(--font-mono);
    font-size: 20px;
    font-weight: 700;
    color: var(--text-1, #f8f8fc);
    margin-bottom: var(--s-1, 4px);
  }
  
  .settings-stat-label {
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-4, #505068);
  }
  
  /* === Legacy classes (for backwards compatibility) === */
  .setting-row-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-4, 16px);
  }
  
  .setting-label {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .setting-desc {
    font-size: 12px;
    color: var(--text-4, #505068);
    margin-top: 2px;
  }
  
  .setting-actions {
    display: flex;
    gap: var(--s-2, 8px);
  }
  
  /* Network Radio Cards - v6.1 compliant */
  .radio-card-group {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--s-4, 16px);
  }

  .radio-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    cursor: pointer;
    transition: all 200ms ease-out;
    text-align: center;
  }

  .radio-card:hover {
    background: var(--void-up, #181824);
    border-color: var(--border-strong, rgba(255, 255, 255, 0.12));
  }

  .radio-card.selected {
    border-color: var(--cyan-500, #06b6d4);
    background: rgba(0, 212, 170, 0.05);
  }

  .radio-card-radio {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    border: 2px solid var(--border-default);
    transition: all 200ms;
  }

  .radio-card.selected .radio-card-radio {
    border-color: var(--cyan-500);
    background: var(--cyan-500);
    box-shadow: inset 0 0 0 3px var(--void-deep);
  }

  .radio-card-content {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .radio-card-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-1);
  }

  .radio-card-desc {
    font-size: 11px;
    color: var(--text-4);
  }

  .radio-card-badge {
    font-size: 10px;
    padding: 4px 8px;
    border-radius: var(--r-xs);
    margin-top: 4px;
  }

  .radio-card-badge.warn {
    background: rgba(251, 191, 36, 0.15);
    color: var(--status-warn, #fbbf24);
  }

  .radio-card-badge.test {
    background: rgba(167, 139, 250, 0.15);
    color: var(--violet-400, #a78bfa);
  }

  .radio-card-badge.safe {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok, #34d399);
  }
  
  /* Form styles (.form-group, .form-label, .form-hint) come from hologram.css */
  
  /* Connection Badge - v6.1 compliant */
  .connection-badge {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 12px;
    border-radius: var(--r-sm);
    font-size: 12px;
    font-weight: 500;
  }

  .connection-badge.connected {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok, #34d399);
  }
  
  .connection-badge.disconnected {
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err, #f87171);
  }

  .connection-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
  }

  /* === Connected Apps v6.1 Styles === */
  .section-desc {
    font-size: 13px;
    color: var(--text-4, #505068);
    margin-bottom: 20px;
  }

  .loading-container {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 48px;
  }

  .loading-spinner {
    width: 32px;
    height: 32px;
    border: 3px solid var(--border-dim);
    border-top-color: var(--cyan);
    border-radius: 50%;
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .empty-state-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 48px 32px;
    background: var(--void-deep);
    border: 1px solid var(--border-subtle);
    border-radius: 8px;
    text-align: center;
  }

  .empty-state-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-4);
    margin-bottom: 16px;
  }

  .empty-state-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0 0 8px 0;
  }

  .empty-state-desc {
    font-size: 13px;
    color: var(--text-4);
    margin: 0;
    line-height: 1.5;
  }

  .apps-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .app-card {
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
    overflow: hidden;
  }

  .app-card-header {
    width: 100%;
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px;
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background 0.2s;
    text-align: left;
  }

  .app-card-header:hover {
    background: var(--void-up, #181824);
  }

  .app-card-info {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .app-card-icon {
    width: 48px;
    height: 48px;
    border-radius: 8px;
    background: rgba(0, 212, 170, 0.15);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--cyan);
  }

  .app-card-details {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .app-card-name-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .app-card-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-1);
  }

  .app-status-badge {
    padding: 4px 8px;
    font-size: 10px;
    font-weight: 500;
    border-radius: var(--r-xs);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .app-status-badge.connected {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok, #34d399);
  }

  .app-card-origin {
    font-size: 12px;
    font-family: var(--font-mono);
    color: var(--text-4);
    margin: 0;
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .app-card-meta {
    display: flex;
    align-items: center;
    gap: 12px;
    color: var(--text-4);
  }

  .app-perm-count {
    font-size: 12px;
  }

  .app-card-expanded {
    padding: 16px;
    border-top: 1px solid var(--border-dim);
    background: var(--void-pure, #000000);
  }

  .app-timestamps {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
    margin-bottom: 16px;
  }

  .app-timestamp {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .timestamp-label {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-4);
  }

  .timestamp-value {
    font-size: 13px;
    color: var(--text-2);
  }

  .app-permissions {
    margin-bottom: 16px;
  }

  .permissions-label {
    display: block;
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--text-4);
    margin-bottom: 8px;
  }

  .permissions-list {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .permission-tag {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 12px;
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
  }

  .permission-name {
    font-size: 12px;
    color: var(--text-2);
  }

  .permission-revoke {
    background: transparent;
    border: none;
    color: var(--text-4);
    cursor: pointer;
    padding: 0;
    display: flex;
    transition: color 0.2s;
  }

  .permission-revoke:hover {
    color: var(--status-err);
  }

  .no-permissions {
    font-size: 12px;
    color: var(--text-4);
    margin: 0;
  }

  .app-actions {
    display: flex;
    justify-content: flex-end;
    padding-top: 12px;
    border-top: 1px solid var(--border-dim);
  }

  .revoke-all-section {
    margin-top: 24px;
    padding-top: 24px;
    border-top: 1px solid var(--border-dim);
  }

  .btn-danger {
    padding: 8px 16px;
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err, #f87171);
    border: none;
    border-radius: 5px;
    font-size: 12px;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.2s;
  }

  .btn-danger:hover {
    background: rgba(248, 113, 113, 0.25);
  }

  /* Permission Reference v6.1 */
  .permission-ref-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .permission-ref-item {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 12px;
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: 8px;
  }

  .permission-ref-icon {
    width: 32px;
    height: 32px;
    border-radius: 5px;
    background: var(--void-up);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--cyan);
    flex-shrink: 0;
  }

  .permission-ref-content {
    flex: 1;
  }

  .permission-ref-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 4px;
  }

  .permission-ref-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-1);
  }

  .permission-ref-badge {
    padding: 4px 8px;
    font-size: 9px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    background: rgba(251, 191, 36, 0.15);
    color: var(--status-warn, #fbbf24);
    border-radius: var(--r-xs);
  }

  .permission-ref-desc {
    font-size: 11px;
    color: var(--text-4);
    margin: 0;
    line-height: 1.4;
  }
  
  /* === v6.1 Status Row Styles === */
  .status-row-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .status-info-group {
    display: flex;
    flex-direction: column;
  }
  
  .status-detail-text {
    font-size: 13px;
    color: var(--text-3, #707088);
  }
  
  .progress-section {
    margin-top: var(--s-5, 20px);
    padding-top: var(--s-4, 16px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .progress-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2, 8px);
  }
  
  .progress-value {
    font-size: 12px;
    font-weight: 500;
  }
  
  /* btn-danger styles come from hologram.css */
  
  .c-text-4 {
    color: var(--text-4, #505068);
  }
  
  /* Slider styles - supplement hologram.css */
  .slider-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2, 8px);
  }
  
  .slider-value {
    font-size: 13px;
    font-weight: 500;
  }
  
  /* Checkbox styles - supplement hologram.css with local layout helpers */
  .checkbox-group {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .checkbox-item {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
    cursor: pointer;
  }
  
  .checkbox-content {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  
  .checkbox-text {
    font-size: 13px;
    color: var(--text-2, #a8a8b8);
  }
  
  .checkbox-hint {
    font-size: 10px;
    color: var(--text-4, #505068);
  }
  
  /* Card and section styles now come from hologram.css via .section-card classes */
  /* Only keeping Tailwind utility overrides for backwards compatibility */
  
  /* === Mining Section Styles === */
  .mining-stats-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--s-4, 16px);
  }
  
  .mining-stat {
    background: var(--void-deep, #0a0a0f);
    border-radius: 8px;
    padding: var(--s-4, 16px);
    text-align: center;
  }
  
  .mining-stat-label {
    display: block;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4, #505068);
    margin-bottom: 4px;
  }
  
  .mining-stat-value {
    display: block;
    font-size: 18px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-1, #e8e8f0);
  }
  
  .mining-difficulty {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  @media (max-width: 640px) {
    .mining-stats-grid {
      grid-template-columns: repeat(2, 1fr);
    }
  }
  
  /* === Phase 2 v6.1 Spacing Classes === */
  
  /* Info List */
  .info-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-3, 12px);
    font-size: 13px;
    color: var(--text-3, #707088);
  }
  
  .info-item {
    line-height: 1.5;
  }
  
  /* Benchmark Form */
  .benchmark-form {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .benchmark-field {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .benchmark-label {
    display: block;
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .benchmark-slider {
    width: 100%;
    accent-color: var(--cyan-500, #06b6d4);
  }
  
  .benchmark-slider-labels {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    color: var(--text-5, #404058);
  }
  
  .benchmark-running {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2, 8px);
  }
  
  .benchmark-result {
    margin-top: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-lg, 12px);
    border: 1px solid rgba(6, 182, 212, 0.3);
  }
  
  .benchmark-result-header {
    text-align: center;
    margin-bottom: var(--s-4, 16px);
  }
  
  .benchmark-hashrate {
    font-size: 28px;
    font-weight: 700;
    color: var(--cyan-400, #22d3ee);
  }
  
  .benchmark-threads-used {
    font-size: 12px;
    color: var(--text-5, #404058);
    margin-top: var(--s-1, 4px);
  }
  
  .benchmark-per-thread {
    margin-top: var(--s-3, 12px);
  }
  
  .per-thread-label {
    font-size: 12px;
    color: var(--text-5, #404058);
  }
  
  .per-thread-list {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1, 4px);
    margin-top: var(--s-1, 4px);
  }
  
  .per-thread-item {
    padding: 4px var(--s-2, 8px);
    background: var(--void-mid);
    border-radius: var(--r-sm);
    font-size: 12px;
    font-family: var(--font-mono);
    color: var(--text-3);
  }
  
  /* Toggle label + switch for page headers */
  .toggle-label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-3);
    cursor: pointer;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  .toggle-text {
    color: var(--text-3);
  }

  .toggle {
    width: 44px;
    height: 22px;
    appearance: none;
    -webkit-appearance: none;
    -moz-appearance: none;
    background: var(--void-up);
    border-width: 1px !important;
    border-style: solid !important;
    border-color: #1e1e2a !important;
    outline: none !important;
    box-shadow: none !important;
    border-radius: 11px;
    position: relative;
    cursor: pointer;
    transition: background 0.2s ease;
    flex-shrink: 0;
  }

  .toggle:checked {
    background: var(--cyan);
    border-color: var(--cyan) !important;
  }

  .toggle::after {
    content: '';
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    background: #ffffff;
    border-width: 0 !important;
    border-style: none !important;
    border-color: transparent !important;
    box-shadow: none !important;
    border-radius: 50%;
    transition: transform 0.2s ease;
  }

  .toggle:checked::after {
    transform: translateX(22px);
  }
  
  .dev-support-status {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  
  .status-indicator {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px;
    background: var(--void-deep, #08080e);
    border-radius: 8px;
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
  }
  
  .status-indicator .status-dot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--text-4, #505068);
  }
  
  .status-indicator.active .status-dot {
    background: var(--emerald, #10b981);
    box-shadow: 0 0 8px var(--emerald, #10b981);
    animation: pulse-dot 2s infinite;
  }
  
  .status-indicator.paused .status-dot {
    background: var(--status-warn, #fbbf24);
  }
  
  .status-indicator.disabled .status-dot {
    background: var(--status-err, #f87171);
  }
  
  @keyframes pulse-dot {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.5; }
  }
  
  .status-indicator .status-text {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
  }
  
  .pause-reason-box {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 12px 16px;
    background: rgba(251, 191, 36, 0.1);
    border-radius: 8px;
    border: 1px solid rgba(251, 191, 36, 0.2);
  }
  
  .pause-reason-box .btn {
    align-self: flex-start;
  }
  
  .pause-reason {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--status-warn, #fbbf24);
    margin: 0;
  }
  
  .active-info {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--emerald, #10b981);
    padding: 8px 16px;
    background: rgba(16, 185, 129, 0.1);
    border-radius: 6px;
    margin: 0;
  }
  
  .contributions-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 16px;
  }
  
  .contribution-stat {
    text-align: center;
    padding: 20px;
    background: var(--void-deep, #08080e);
    border-radius: 8px;
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
  }
  
  .contribution-value {
    display: block;
    font-size: 24px;
    font-weight: 600;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-1, #f8f8fc);
    margin-bottom: 4px;
  }
  
  .contribution-label {
    display: block;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4, #505068);
  }
  
  .sessions-info {
    text-align: center;
    font-size: 12px;
    color: var(--text-4, #505068);
    margin-top: 12px;
  }
  
  .info-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
  }
  
  .info-item {
    display: flex;
    gap: 12px;
    padding: 16px;
    background: var(--void-deep, #08080e);
    border-radius: 8px;
  }
  
  .info-icon {
    flex-shrink: 0;
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(34, 211, 238, 0.1);
    border-radius: 8px;
    color: var(--cyan-400, #22d3ee);
  }
  
  .info-content strong {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
    margin-bottom: 4px;
  }
  
  .info-content p {
    font-size: 12px;
    color: var(--text-4, #505068);
    line-height: 1.4;
    margin: 0;
  }
  
  /* Legacy epoch-info-card styles (kept for compatibility) */
  .epoch-info-card {
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-xl, 16px);
    padding: var(--s-5, 20px);
  }
  
  .epoch-info-title {
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
    margin-bottom: var(--s-4, 16px);
  }
  
  .epoch-status-label {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  /* Stat Grid */
  .stat-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3, 12px);
  }
  
  /* Simulator wallet balance row */
  .sim-wallet-balance-row {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(52, 211, 153, 0.08);
    border-radius: var(--r-md, 8px);
    border: 1px solid rgba(52, 211, 153, 0.15);
  }
  .sim-wallet-balance-label {
    font-size: 12px;
    color: var(--text-4, #505068);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  .sim-wallet-balance-value {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 14px;
    font-weight: 600;
    color: var(--emerald-400, #34d399);
  }
  .sim-wallet-balance-hint {
    font-size: 11px;
    color: var(--text-5, #404058);
    margin-left: auto;
  }
  
  /* Action Input Group for Simulator Quick Actions */
  .action-input-group {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  /* Installed Info Row */
  .installed-info {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .version-text {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    color: var(--text-4, #505068);
  }
  
  /* Prune Input Row */
  .prune-input-row {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
  }
  
  /* Add Host Form */
  .add-host-form {
    display: flex;
    gap: var(--s-2, 8px);
    margin-bottom: var(--s-4, 16px);
  }
  
  .host-input {
    flex: 1;
  }
  
  /* Connection Log */
  .connection-log {
    max-height: 192px;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--s-1, 4px);
  }
  
  .connection-log-empty {
    font-size: 13px;
    color: var(--text-5, #404058);
    text-align: center;
    padding: var(--s-4, 16px);
  }
  
  .connection-log-entry {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    font-size: 12px;
  }
  
  .log-dot {
    width: 8px;
    height: 8px;
    margin-top: 4px;
    border-radius: var(--r-full, 9999px);
    flex-shrink: 0;
  }
  
  .log-dot-ok {
    background: var(--status-ok, #34d399);
  }
  
  .log-dot-err {
    background: var(--status-err, #f87171);
  }
  
  .log-entry-info {
    flex: 1;
    min-width: 0;
  }
  
  .log-entry-host {
    display: block;
    color: var(--text-3, #707088);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .log-entry-reason {
    color: var(--text-5, #404058);
  }
  
  /* === Console Terminal Styles === */
  .console-content {
    padding: 0;
  }
  
  .console-viewport {
    height: 400px;
    overflow-y: auto;
    overflow-x: hidden;
    padding: var(--s-4);
    background: var(--void-pure);
    border-radius: var(--r-md);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.6;
  }
  
  .console-empty {
    color: var(--text-5);
    font-style: italic;
    padding: var(--s-4);
  }
  
  .console-line {
    display: flex;
    gap: var(--s-2);
    padding: 2px 0;
    color: var(--text-2);
  }
  
  .console-timestamp {
    color: var(--text-5);
    flex-shrink: 0;
  }
  
  .console-message {
    word-break: break-word;
  }
  
  .console-line.level-error {
    color: var(--status-err);
  }
  
  .console-line.level-warn {
    color: var(--status-warn);
  }
  
  .console-actions {
    display: flex;
    gap: var(--s-2);
  }
  
  .explorer-header-right {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  
  /* === Phase 3 v6.1 Typography Classes === */
  
  /* Sync Progress */
  .sync-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2, 8px);
  }
  
  .sync-progress-value {
    font-size: 13px;
    color: var(--cyan-400, #22d3ee);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
  }
  
  /* Update Notice */
  .update-notice {
    font-size: 13px;
  }
  
  /* Difficulty Value */
  .difficulty-value {
    color: var(--cyan-400, #22d3ee);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
  }
  
  /* EPOCH Hint */
  .epoch-hint {
    font-size: 12px;
    margin-top: var(--s-2, 8px);
    color: var(--text-4, #505068);
  }
  
  /* === Phase 4 v6.1 Layout Classes === */
  
  /* Section Header */
  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-4, 16px);
  }
  
  /* === About Section Styles === */
  .about-details {
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    overflow: hidden;
    margin-bottom: var(--s-6, 24px);
  }
  
  .about-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-3, 12px) var(--s-4, 16px);
  }
  
  .about-row:not(:last-child) {
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.04));
  }
  
  .about-label {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-4, #505068);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  
  .about-value {
    font-size: 13px;
    color: var(--text-2, #b0b0c0);
  }
  
  .about-value.mono {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
  }
  
  .about-network-badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: var(--r-sm, 4px);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  .about-network-badge.mainnet {
    background: rgba(34, 211, 238, 0.1);
    color: var(--cyan-400, #22d3ee);
    border: 1px solid rgba(34, 211, 238, 0.25);
  }
  
  .about-network-badge.testnet {
    background: rgba(251, 191, 36, 0.1);
    color: var(--amber-400, #fbbf24);
    border: 1px solid rgba(251, 191, 36, 0.25);
  }
  
  .about-network-badge.simulator {
    background: rgba(248, 113, 113, 0.1);
    color: var(--status-err, #f87171);
    border: 1px solid rgba(248, 113, 113, 0.25);
  }
  
  .about-footer {
    text-align: center;
    padding-top: var(--s-4, 16px);
    border-top: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
  }
  
  .about-copyright {
    font-size: 12px;
    color: var(--text-5, #404058);
    margin: 0 0 var(--s-1, 4px) 0;
  }
  
  .about-tagline {
    font-size: 13px;
    font-style: italic;
    color: var(--text-4, #505068);
    margin: 0;
  }
  
  /* Search Exclusions Styles */
  .exclusions-list {
    padding: var(--s-2);
    background: var(--void-base);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-subtle);
  }
  
  .exclusions-header {
    font-size: 11px;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: var(--s-2);
  }
  
  .exclusions-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-1);
  }
  
  .exclusion-tag {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    padding: 2px 8px;
    background: var(--surface-elevated);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-family: var(--font-mono);
    color: var(--text-secondary);
  }
  
  .exclusion-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: 2px;
    font-size: 14px;
    line-height: 1;
  }
  
  .exclusion-remove:hover {
    background: var(--status-error);
    color: white;
  }
  
  .exclusion-input-row {
    display: flex;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
  }
  
  .exclusion-input-row .form-input {
    flex: 1;
  }
  
  .exclusions-modal-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    max-height: 300px;
    overflow-y: auto;
  }
  
  .exclusion-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-2);
    background: var(--surface-base);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
  }
  
  .exclusion-text {
    font-family: var(--font-mono);
    font-size: 13px;
    color: var(--text-primary);
  }
  
  .empty-exclusions {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    padding: var(--s-6);
    color: var(--text-muted);
  }
  
  .empty-exclusions p {
    margin: 0;
    font-size: 13px;
  }
  
  .input-group {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .form-input {
    padding: var(--s-2) var(--s-3);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-default, rgba(255, 255, 255, 0.09));
    border-radius: var(--r-md, 8px);
    color: var(--text-1, #f8f8fc);
    font-family: var(--font-mono);
    font-size: 13px;
    outline: none;
    transition: border-color 200ms ease-out, box-shadow 200ms ease-out;
    -moz-appearance: textfield;
  }
  
  .form-input::-webkit-outer-spin-button,
  .form-input::-webkit-inner-spin-button {
    -webkit-appearance: none;
    margin: 0;
  }
  
  .form-input:focus {
    border-color: var(--cyan-500, #06b6d4);
    box-shadow: 0 0 0 2px rgba(6, 182, 212, 0.15);
  }
  
  .form-input::placeholder {
    color: var(--text-5, #404058);
  }
  
  .input-suffix {
    font-family: var(--font-mono);
    font-size: 13px;
    color: var(--text-4, #505068);
  }
  
  .btn-group {
    display: flex;
    gap: var(--s-1);
  }
  
  .btn-danger-outline {
    background: transparent;
    border: 1px solid var(--status-error);
    color: var(--status-error);
  }
  
  .btn-danger-outline:hover {
    background: var(--status-error);
    color: white;
  }
  
  /* Modal Styles */
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    backdrop-filter: blur(4px);
  }
  
  .modal-content {
    background: var(--void-base);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-lg);
    width: 90%;
    max-width: 500px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 20px 40px rgba(0, 0, 0, 0.5);
  }
  
  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4);
    border-bottom: 1px solid var(--border-subtle);
  }
  
  .modal-title {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
  }
  
  .modal-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border: none;
    background: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: var(--radius-sm);
  }
  
  .modal-close:hover {
    background: var(--surface-hover);
    color: var(--text-primary);
  }
  
  .modal-body {
    padding: var(--s-4);
    overflow-y: auto;
  }
  
  .modal-desc {
    margin: 0 0 var(--s-4);
    font-size: 13px;
    color: var(--text-secondary);
    line-height: 1.5;
  }
  
  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--s-2);
    padding: var(--s-3) var(--s-4);
    border-top: 1px solid var(--border-subtle);
  }
  
  /* Toggle Switch */
  .toggle-switch {
    position: relative;
    width: 44px;
    height: 24px;
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: 12px;
    cursor: pointer;
    transition: all 0.2s ease;
    padding: 0;
  }
  
  .toggle-switch:hover {
    border-color: var(--border-default);
  }
  
  .toggle-switch.active {
    background: rgba(52, 211, 153, 0.2);
    border-color: var(--status-ok);
  }
  
  .toggle-slider {
    position: absolute;
    top: 3px;
    left: 3px;
    width: 16px;
    height: 16px;
    background: var(--text-4);
    border-radius: 50%;
    transition: all 0.2s ease;
  }
  
  .toggle-switch.active .toggle-slider {
    background: var(--status-ok);
    transform: translateX(20px);
  }
</style>
