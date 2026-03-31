<script>
  import { createEventDispatcher, onMount, onDestroy } from 'svelte';
  import { appState, walletState, settingsState, requestWalletApproval, syncNetworkMode, toast, combinedSyncProgress } from '../stores/appState.js';
  import { 
    SetSetting, GetEpochStats, SetNetworkMode,
    StartSimulatorMode, StopSimulatorMode, GetSimulatorStatus,
    ApproveWalletConnection, ConnectXSWD,
    GetRecentWalletsWithInfo, SwitchWallet, GetActiveXSWDConnections, RevokeXSWDConnection,
    DisconnectXSWD, CloseWallet, RemoveRecentWallet
  } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { DotIndicator, Icons } from './holo';
  import Wordmark from './Wordmark.svelte';
  import { 
    Globe, Palette, Blocks, Wallet, Settings, 
    Diamond,
    Globe2, FlaskConical, Gamepad2, Radio, FolderOpen
  } from 'lucide-svelte';
  import { getAvatarUrl, clearAvatarCache } from '../utils/avatarService.js';
  
  // EPOCH status
  let epochStats = { active: false, enabled: true, hashes: 0 };
  let epochInterval;
  
  // Simulator status
  let simulatorStatus = { isInitialized: false, isStarting: false };
  let showSimulatorConfirm = false;
  let simulatorAction = null; // 'start' or 'stop'
  let simulatorStarting = false;
  let simulatorProgress = {
    step: 0,
    message: '',
    status: ''
  };
  
  // Network switch modal state
  let showNetworkSwitchModal = false;
  let networkSwitchTarget = 'simulator'; // 'simulator' or 'mainnet'
  const networkSwitchSteps = [
    { id: 0, label: 'Checking current connection' },
    { id: 1, label: 'Checking simulator binary' },
    { id: 2, label: 'Starting simulator daemon' },
    { id: 3, label: 'Waiting for daemon ready' },
    { id: 4, label: 'Setting up test wallets' },
    { id: 5, label: 'Configuring Gnomon' }
  ];
  
  // Wallet menu state
  let showWalletMenu = false;
  let recentWalletsInfo = [];
  let switchingWallet = null;
  let switchPassword = '';
  let switchError = '';
  let connecting = false;
  let showConnectedApps = false;
  let connectedApps = [];
  let loadingApps = false;
  
  // Determine wallet address to display (reactive statements)
  // Priority: Integrated wallet address > Engram (external) address
  $: walletDisplayAddress = $walletState.isOpen ? $walletState.address : null;
  
  // Wallet connection status - TRUE only when an actual wallet is connected
  // Not just when XSWD server is running (that's separate)
  $: walletIsConnected = $walletState.isOpen || $appState.engramConnected;
  
  // Wallet mode detection for UI
  $: walletMode = $walletState.isOpen ? 'integrated' : ($appState.engramConnected ? 'engram' : 'none');
  
  // XSWD server ready but no wallet connected
  $: xswdReadyNoWallet = $appState.xswdServerRunning && !walletIsConnected;

  function detectAddressNetwork(address) {
    if (!address) return 'unknown';
    if (address.startsWith('dero1') || address.startsWith('deroi1')) return 'mainnet';
    if (address.startsWith('deto1') || address.startsWith('detoi1')) return 'simulator';
    return 'unknown';
  }
  
  // Format address for display - show only the unique part (last 8 chars)
  function formatAddressForDisplay(address) {
    if (!address) return 'Wallet';
    // DERO addresses are typically 69 characters
    // Most start with "dero1qy" (mainnet)
    // Show only the last 8 characters for uniqueness and compact display
    if (address.length > 8) {
      return '…' + address.slice(-8);
    }
    return address;
  }
  
  // Avatar state
  let walletAvatarUrl = null;
  let loadingAvatar = false;
  let currentAvatarSize = null;
  
  // Load avatar when wallet connects or address changes
  $: if (walletDisplayAddress && walletIsConnected) {
    const avatarSize = collapsed ? 24 : 40;
    // Only reload if size changed or avatar not loaded
    if (currentAvatarSize !== avatarSize || !walletAvatarUrl) {
      loadWalletAvatar(walletDisplayAddress, avatarSize);
    }
  } else {
    // Clear avatar when wallet disconnects
    if (walletAvatarUrl) {
      if (walletDisplayAddress) {
        clearAvatarCache(walletDisplayAddress);
      }
      walletAvatarUrl = null;
      currentAvatarSize = null;
    }
  }
  
  async function loadWalletAvatar(address, size) {
    if (!address || loadingAvatar) return;
    
    try {
      loadingAvatar = true;
      currentAvatarSize = size;
      // Get avatar URL (frame renders instantly, custom pixels load in background)
      walletAvatarUrl = await getAvatarUrl(address, size);
    } catch (error) {
      console.error('Failed to load wallet avatar:', error);
      walletAvatarUrl = null;
      currentAvatarSize = null;
    } finally {
      loadingAvatar = false;
    }
  }
  
  onMount(async () => {
    await refreshEpochStatus();
    await refreshSimulatorStatus();
    await loadConnectedApps(); // Load for wallet anchor display
    epochInterval = setInterval(() => {
      refreshEpochStatus();
      refreshSimulatorStatus();
      loadConnectedApps(); // Keep connected apps count updated
    }, 5000);
    
    // Listen for simulator progress events
    EventsOn("simulator:progress", (data) => {
      simulatorProgress = {
        step: data.step || 0,
        message: data.message || '',
        status: data.status || ''
      };
    });
    
    // Listen for simulator completion — also triggers from the RPC return path,
    // so we skip the toast here to avoid doubling it up.
    EventsOn("simulator:complete", (data) => {
      simulatorStarting = false;
      showNetworkSwitchModal = false;
      if (data.success) {
        syncNetworkMode();
        refreshSimulatorStatus();
      }
    });
    
    // Listen for simulator errors
    EventsOn("simulator:error", (data) => {
      simulatorStarting = false;
      showNetworkSwitchModal = false;
      toast.error(data.error || 'Simulator startup failed', 5000);
    });
  });
  
  onDestroy(() => {
    if (epochInterval) clearInterval(epochInterval);
    // Clean up avatar cache
    if (walletDisplayAddress) {
      clearAvatarCache(walletDisplayAddress);
    }
    // Clean up event listeners
    EventsOff("simulator:progress");
    EventsOff("simulator:complete");
    EventsOff("simulator:error");
  });
  
  async function refreshEpochStatus() {
    try {
      epochStats = await GetEpochStats();
    } catch (e) {
      // Silently fail - epoch may not be available
    }
  }
  
  async function refreshSimulatorStatus() {
    try {
      simulatorStatus = await GetSimulatorStatus();
    } catch (e) {
      // Silently fail
    }
  }
  
  export let tabs = [];
  export let currentTab = 'browser';
  export let collapsed = false;
  
  const dispatch = createEventDispatcher();
  
  let showNetworkMenu = false;
  
  // Network configuration - v6 colors
  // Mainnet = Green (primary/production), Simulator = Red (local/dev)
  const networks = {
    mainnet: { label: 'Mainnet', icon: 'globe', dotStatus: 'ok' },
    simulator: { label: 'Simulator', icon: 'gamepad', dotStatus: 'err' },
  };
  
  // Use effective network from status (actual connection) when node is connected;
  // otherwise use persisted preference. Prevents "Simulator" + mainnet block height mismatch on restart.
  $: effectiveNetwork = $appState.nodeConnected && $appState.network
    ? $appState.network
    : $settingsState.network;
  $: currentNetwork = networks[effectiveNetwork] || networks.mainnet;
  $: walletAddressNetwork = detectAddressNetwork(walletDisplayAddress);
  $: walletNetworkMismatch = walletMode === 'integrated'
    && walletIsConnected
    && walletAddressNetwork !== 'unknown'
    && walletAddressNetwork !== effectiveNetwork;
  
  function selectTab(tabId) {
    dispatch('tabChange', tabId);
  }
  
  function toggleCollapse() {
    dispatch('toggleCollapse');
  }
  
  async function switchNetwork(networkId) {
    // Special handling for simulator mode
    if (networkId === 'simulator') {
      if (!simulatorStatus.isInitialized) {
        // Show confirmation to start simulator
        simulatorAction = 'start';
        showSimulatorConfirm = true;
        showNetworkMenu = false;
        return;
      }
    } else if ($settingsState.network === 'simulator' && simulatorStatus.isInitialized) {
      // Switching away from running simulator - ask to stop it
      simulatorAction = 'stop';
      showSimulatorConfirm = true;
      showNetworkMenu = false;
      return;
    }
    
    await doSwitchNetwork(networkId);
  }
  
  async function doSwitchNetwork(networkId) {
    try {
      // Use SetNetworkMode to properly update the backend
      const result = await SetNetworkMode(networkId);
      if (result.success) {
        // Sync network mode from backend (updates both appState and settingsState)
        await syncNetworkMode();
        // Also update settings for compatibility
        await SetSetting(JSON.stringify({ network: networkId }));
      } else {
        console.error('Failed to set network mode:', result.error);
      }
    } catch (err) {
      console.error('Failed to switch network:', err);
    }
    showNetworkMenu = false;
  }
  
  async function confirmSimulatorAction() {
    showSimulatorConfirm = false;
    
    if (simulatorAction === 'start') {
      // Start simulator mode with loading state and progress modal
      simulatorStarting = true;
      networkSwitchTarget = 'simulator';
      simulatorProgress = { step: 0, message: 'Starting simulator...', status: 'starting' };
      showNetworkSwitchModal = true;
      
      try {
        const result = await StartSimulatorMode();
        // Always close the modal when the call resolves — the simulator:complete
        // event handler also does this, but the event may race with the RPC return
        // (or be missed if Wails delivers it before the listener is active).
        simulatorStarting = false;
        showNetworkSwitchModal = false;
        
        if (result.success) {
          toast.success(result.message || 'Simulator mode activated!', 3000);
          // Network mode will be updated via the completion event handler
          // But also ensure it's set here as a fallback
          await doSwitchNetwork('simulator');
          await refreshSimulatorStatus();
        } else {
          toast.error(result.error || 'Failed to start simulator', 5000);
        }
      } catch (e) {
        simulatorStarting = false;
        showNetworkSwitchModal = false;
        toast.error(e.message || 'Failed to start simulator', 5000);
      }
    } else if (simulatorAction === 'stop') {
      // Stop simulator and switch to mainnet with progress modal
      networkSwitchTarget = 'mainnet';
      simulatorProgress = { step: 0, message: 'Stopping simulator...', status: 'stopping' };
      showNetworkSwitchModal = true;
      
      try {
        await StopSimulatorMode();
        await doSwitchNetwork('mainnet');
        await refreshSimulatorStatus();
        showNetworkSwitchModal = false;
        toast.success('Switched to Mainnet', 2000);
      } catch (e) {
        console.error('Failed to stop simulator:', e);
        showNetworkSwitchModal = false;
        toast.error('Failed to stop simulator: ' + e.message, 5000);
      }
    }
    
    simulatorAction = null;
  }
  
  function cancelSimulatorAction() {
    showSimulatorConfirm = false;
    simulatorAction = null;
  }
  
  function formatBlockHeight(height) {
    if (!height) return '—';
    return height.toLocaleString();
  }
  
  // Close menu when clicking outside
  function handleClickOutside(event) {
    if (showNetworkMenu) {
      showNetworkMenu = false;
    }
  }
  
  // Status indicator click handlers
  function handleStatusClick(type, event) {
    if (event) {
      event.stopPropagation();
      event.preventDefault();
    }
    
    switch (type) {
      case 'gnomon':
        // Navigate to Settings > Gnomon
        dispatch('statusClick', { type: 'gnomon', tab: 'settings', section: 'gnomon' });
        break;
      case 'node':
        // Navigate to Settings > Node section
        dispatch('statusClick', { type: 'node', tab: 'settings', section: 'node' });
        break;
      case 'wallet':
        // Context-aware behavior based on wallet state
        if (walletIsConnected) {
          // Open wallet menu
          toggleWalletMenu();
        } else {
          // No wallet connected: navigate to Wallet page
          // (User can choose to open a wallet file, create new, or connect via XSWD)
          dispatch('statusClick', { type: 'wallet', tab: 'wallet' });
        }
        break;
      case 'epoch':
        // Navigate to Settings > Developer Support section
        dispatch('statusClick', { type: 'epoch', tab: 'settings', section: 'developer-support' });
        break;
      case 'xswd':
        // Navigate to Settings > Connected Apps
        dispatch('statusClick', { type: 'xswd', tab: 'settings', section: 'connected-apps' });
        break;
      case 'block':
        // Navigate to Explorer with current block
        dispatch('statusClick', { type: 'block', tab: 'explorer', block: $appState.chainHeight });
        break;
    }
  }
  
  // Avatar editing is moving to Villager; keep a clear temporary action for now.
  function handleAvatarClick(event) {
    event.stopPropagation();
    event.preventDefault();
    toast.info('Villager avatar editor coming soon');
  }
  
  async function handleConnectWallet() {
    try {
      if ($settingsState.integratedWallet) {
        // Use integrated wallet modal
        const approval = await requestWalletApproval({
          type: 'connect',
          appName: 'Hologram',
          origin: 'Status Indicator'
        });
        
        if (approval && approval.approved) {
          await ApproveWalletConnection();
        }
      } else {
        // For external XSWD
        await ConnectXSWD();
      }
    } catch (error) {
      console.error('Wallet connection error:', error);
    }
  }
  
  // Wallet menu functions
  async function loadRecentWallets() {
    try {
      const infos = await GetRecentWalletsWithInfo();
      if (infos && infos.length > 0) {
        recentWalletsInfo = infos;
      }
    } catch (e) {
      console.error('Failed to load recent wallets:', e);
    }
  }
  
  async function loadConnectedApps() {
    loadingApps = true;
    try {
      const result = await GetActiveXSWDConnections();
      if (result.success) {
        connectedApps = result.connections || [];
      }
    } catch (e) {
      console.error('Failed to load connected apps:', e);
    } finally {
      loadingApps = false;
    }
  }
  
  async function revokeApp(origin) {
    try {
      await RevokeXSWDConnection(origin);
      await loadConnectedApps();
    } catch (e) {
      console.error('Failed to revoke app:', e);
    }
  }
  
  function toggleConnectedApps() {
    showConnectedApps = !showConnectedApps;
    if (showConnectedApps) {
      loadConnectedApps();
    }
  }
  
  function getPermissionLabel(perm) {
    const labels = {
      'view_address': 'View Address',
      'view_balance': 'View Balance',
      'sign_transfers': 'Sign Transfers',
      'sign_sc': 'Sign SC Calls'
    };
    return labels[perm] || perm;
  }
  
  async function toggleWalletMenu() {
    showWalletMenu = !showWalletMenu;
    if (showWalletMenu) {
      await loadRecentWallets();
      await loadConnectedApps();
    } else {
      switchingWallet = null;
      switchPassword = '';
      switchError = '';
      showConnectedApps = false;
    }
  }
  
  async function handleQuickSwitch(wallet) {
    if (wallet.isCurrent) return;
    switchingWallet = wallet;
    switchPassword = '';
    switchError = '';
  }
  
  async function handleRemoveWallet(wallet, event) {
    event.stopPropagation();
    try {
      const result = await RemoveRecentWallet(wallet.path);
      if (result.success) {
        await loadRecentWallets();
      }
    } catch (e) {
      console.error('Failed to remove wallet:', e);
    }
  }
  
  async function confirmSwitch() {
    if (!switchingWallet || !switchPassword) return;
    
    connecting = true;
    switchError = '';
    
    try {
      const result = await SwitchWallet(switchingWallet.path, switchPassword);
      if (!result.success) {
        switchError = result.error || 'Failed to switch wallet';
        connecting = false;
        return;
      }
      
      walletState.update(state => ({
        ...state,
        isOpen: true,
        address: result.address,
        balance: result.balance,
        lockedBalance: result.lockedBalance,
        walletPath: switchingWallet.path,
      }));
      
      settingsState.update(s => ({ ...s, lastWalletPath: switchingWallet.path }));
      
      showWalletMenu = false;
      switchingWallet = null;
      switchPassword = '';
      
      await loadRecentWallets();
    } catch (e) {
      switchError = e.message || 'Failed to switch wallet';
    } finally {
      connecting = false;
    }
  }
  
  async function toggleWalletConnection() {
    // Only consider actual wallet connections, not just XSWD server running
    const hasWalletConnected = $walletState.isOpen || $appState.engramConnected;
    
    if (hasWalletConnected) {
      if ($appState.engramConnected) {
        await DisconnectXSWD();
      }
      if ($walletState.isOpen) {
        try {
          await CloseWallet();
          walletState.update(state => ({
            ...state,
            isOpen: false,
            address: '',
            balance: 0,
            lockedBalance: 0,
          }));
        } catch (e) {
          console.error('Failed to close wallet:', e);
        }
      }
      showWalletMenu = false;
    } else {
      await handleConnectWallet();
    }
  }
  
  // Close menu when clicking outside
  function handleWalletMenuClickOutside(event) {
    if (!showWalletMenu) return;
    
    const target = event.target;
    if (!target) return;
    
    // Check if click is inside the wallet menu or wallet display
    const clickedInside = target.closest('.wallet-menu') || target.closest('.wallet-display-bottom');
    
    if (!clickedInside) {
      showWalletMenu = false;
      switchingWallet = null;
      switchPassword = '';
      switchError = '';
    }
  }
  
  // Combined click handler for window events
  function handleWindowClick(event) {
    try {
      handleClickOutside(event);
      handleWalletMenuClickOutside(event);
    } catch (e) {
      console.error('Error in handleWindowClick:', e);
    }
  }
  
  // Map tab IDs to Lucide components
  const tabIconMap = {
    explorer: Blocks,
    browser: Globe,
    wallet: Wallet,
    studio: Palette,
    settings: Settings,
  };
</script>

<svelte:window on:click={handleWindowClick} />

<aside 
  class="sidebar"
  class:collapsed
>
  <!-- v6.2 Wordmark - Clean, no version (moved to Settings/About) -->
  <div class="sidebar-head">
    {#if collapsed}
      <img src="src/assets/hex_hologram_logo.svg" alt="" class="sidebar-logo-sm" />
    {:else}
      <Wordmark size="sm" glow={true} />
    {/if}
  </div>
  
  <!-- v6.1 Navigation Tabs -->
  <nav class="sidebar-menu">
    {#each tabs as tab}
      <button
        on:click={() => selectTab(tab.id)}
        class="nav-item"
        class:active={currentTab === tab.id}
      >
        <span class="nav-icon">
          <svelte:component this={tabIconMap[tab.id] || Settings} size={16} strokeWidth={1.5} />
        </span>
        {#if !collapsed}
          <span>{tab.label}</span>
        {:else}
          <!-- v6.3 Tooltip for collapsed state -->
          <div class="rail-tooltip">
            <span class="rail-tooltip-label">{tab.label}</span>
          </div>
        {/if}
      </button>
    {/each}
  </nav>
  
  <!-- v6.4 Status Panel - Redesigned: Services Grid + Info Rows -->
  <div class="sidebar-status">
    {#if !collapsed}
      <!-- E2 DESIGN: All Status Rows (Fully Unified) -->
      <div class="info-rows">
        <!-- Service Status Rows -->
        <button
          class="service-row"
          on:click|stopPropagation={(e) => handleStatusClick('node', e)}
          title={$appState.nodeConnected ? 'Node Online - Click for details' : 'Node Offline - Click for details'}
        >
          <span class="service-row-label">NODE</span>
          <span class="service-row-dot" class:ok={$appState.nodeConnected} class:err={!$appState.nodeConnected}></span>
        </button>
        
        <button
          class="service-row"
          on:click|stopPropagation={(e) => handleStatusClick('xswd', e)}
          title={$appState.xswdServerRunning ? 'XSWD Server Active - Click for details' : 'XSWD Server Offline - Click for details'}
        >
          <span class="service-row-label">XSWD</span>
          <span class="service-row-dot" class:ok={$appState.xswdServerRunning} class:err={!$appState.xswdServerRunning}></span>
        </button>
        
        <button
          class="service-row"
          on:click|stopPropagation={(e) => handleStatusClick('epoch', e)}
          title={!epochStats.enabled ? 'EPOCH Disabled' : epochStats.paused ? 'EPOCH Paused' : epochStats.active && epochStats.worker_running ? 'EPOCH Active' : epochStats.active ? 'EPOCH Ready' : 'EPOCH Connecting'}
        >
          <span class="service-row-label">EPOCH</span>
          <span class="service-row-dot" 
            class:ok={epochStats.enabled && epochStats.active && !epochStats.paused && epochStats.worker_running}
            class:warn={epochStats.enabled && (epochStats.paused || !epochStats.active || !epochStats.worker_running)}
            class:err={!epochStats.enabled}></span>
        </button>
        
        <!-- Info Rows: Network selector and Block height -->
        <!-- Network Selector -->
        <div class="network-indicator-wrapper">
          <button
            class="info-row"
            on:click|stopPropagation={() => showNetworkMenu = !showNetworkMenu}
          >
            <span class="info-label">NETWORK</span>
            <span class="info-value" 
              class:value-ok={currentNetwork.dotStatus === 'ok'}
              class:value-warn={currentNetwork.dotStatus === 'warn'}
              class:value-err={currentNetwork.dotStatus === 'err'}>
              {currentNetwork.label}
            </span>
          </button>
          
          {#if showNetworkMenu}
            <div class="network-dropdown">
              {#if simulatorStarting}
                <div class="simulator-progress">
                  <div class="progress-spinner"></div>
                  <div class="progress-text">
                    <div class="progress-message">{simulatorProgress.message}</div>
                    <div class="progress-step">Step {simulatorProgress.step}/5</div>
                  </div>
                </div>
              {/if}
              
              {#each Object.entries(networks) as [id, net]}
                <button
                  on:click|stopPropagation={() => switchNetwork(id)}
                  class="network-dropdown-option"
                  class:active={effectiveNetwork === id}
                  disabled={simulatorStarting && id !== 'simulator'}
                >
                  <span class="dot-column">
                    <span class="unified-dot" 
                      class:dot-ok={net.dotStatus === 'ok'} 
                      class:dot-warn={net.dotStatus === 'warn'} 
                      class:dot-err={net.dotStatus === 'err'}></span>
                  </span>
                  <span>{net.label}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
        
        <!-- Block Height -->
        {#if $appState.chainHeight}
          <button
            class="info-row"
            on:click|stopPropagation={(e) => handleStatusClick('block', e)}
          >
            <span class="info-label">BLOCK</span>
            <span class="info-value value-ok">
              {formatBlockHeight($appState.chainHeight)}
            </span>
          </button>
        {/if}
        
        <!-- GNOMON Progress Row (Bar Only) -->
        <!-- States:
             - syncing: gnomonProgress < 100 (cyan shimmer)
             - loading-apps: gnomonProgress >= 100 but apps not loaded yet (green shimmer+pulse)
             - synced: gnomonProgress >= 100 AND apps loaded (solid green)
             - offline: gnomon not running (dim red) -->
        <button
          class="gnomon-row"
          class:gnomon-synced={$appState.gnomonRunning && $appState.gnomonProgress >= 100 && $appState.gnomonAppsLoaded}
          class:gnomon-loading-apps={$appState.gnomonRunning && $appState.gnomonProgress >= 100 && !$appState.gnomonAppsLoaded}
          class:gnomon-syncing={$appState.gnomonRunning && $appState.gnomonProgress < 100}
          class:gnomon-offline={!$appState.gnomonRunning}
          on:click|stopPropagation={(e) => handleStatusClick('gnomon', e)}
          title={$appState.gnomonRunning ? ($appState.gnomonProgress >= 100 ? ($appState.gnomonAppsLoaded ? 'Gnomon synced + apps loaded - Click for details' : 'Loading apps from Gnomon... - Click for details') : `Gnomon syncing ${$appState.gnomonProgress.toFixed(0)}% - Click for details`) : 'Gnomon Offline - Click for details'}
        >
          <span class="gnomon-label">GNOMON</span>
          <div class="gnomon-progress-container">
            <div class="gnomon-progress-bar">
              <div 
                class="gnomon-progress-fill"
                class:syncing={$appState.gnomonRunning && $appState.gnomonProgress < 100}
                class:loading-apps={$appState.gnomonRunning && $appState.gnomonProgress >= 100 && !$appState.gnomonAppsLoaded}
                class:synced={$appState.gnomonRunning && $appState.gnomonProgress >= 100 && $appState.gnomonAppsLoaded}
                class:offline={!$appState.gnomonRunning}
                style="width: {$appState.gnomonRunning ? Math.min($appState.gnomonProgress, 100) : 100}%"
              ></div>
            </div>
          </div>
        </button>
      </div>
    {:else}
      <!-- COLLAPSED: Vertical LED strip (order matches expanded sidebar) -->
      <div class="status-list">
        <!-- Services: NODE, XSWD, EPOCH -->
        <button
          class="unified-indicator collapsed"
          on:click|stopPropagation={(e) => handleStatusClick('node', e)}
        >
          <span class="unified-dot" class:dot-ok={$appState.nodeConnected} class:dot-err={!$appState.nodeConnected}></span>
          <div class="rail-tooltip">
            <span class="rail-tooltip-label">Node</span>
            <span class="rail-tooltip-value" class:tt-ok={$appState.nodeConnected} class:tt-err={!$appState.nodeConnected}>
              {$appState.nodeConnected ? 'Online' : 'Offline'}
            </span>
          </div>
        </button>
        
        <button
          class="unified-indicator collapsed"
          on:click|stopPropagation={(e) => handleStatusClick('xswd', e)}
        >
          <span class="unified-dot" class:dot-ok={$appState.xswdServerRunning} class:dot-err={!$appState.xswdServerRunning}></span>
          <div class="rail-tooltip">
            <span class="rail-tooltip-label">XSWD</span>
            <span class="rail-tooltip-value" class:tt-ok={$appState.xswdServerRunning} class:tt-err={!$appState.xswdServerRunning}>
              {$appState.xswdServerRunning ? 'Active' : 'Offline'}
            </span>
          </div>
        </button>
        
        <button
          class="unified-indicator collapsed"
          on:click|stopPropagation={(e) => handleStatusClick('epoch', e)}
        >
          <span class="unified-dot" 
            class:dot-ok={epochStats.enabled && epochStats.active && !epochStats.paused && epochStats.worker_running}
            class:dot-warn={epochStats.enabled && (epochStats.paused || !epochStats.active || !epochStats.worker_running)}
            class:dot-err={!epochStats.enabled}></span>
          <div class="rail-tooltip">
            <span class="rail-tooltip-label">EPOCH</span>
            <span class="rail-tooltip-value"
              class:tt-ok={epochStats.enabled && epochStats.active && !epochStats.paused && epochStats.worker_running}
              class:tt-warn={epochStats.enabled && (epochStats.paused || !epochStats.active || !epochStats.worker_running)}
              class:tt-err={!epochStats.enabled}>
              {#if !epochStats.enabled}
                Disabled
              {:else if epochStats.paused}
                Paused
              {:else if epochStats.active && epochStats.worker_running}
                Active
              {:else if epochStats.active}
                Ready
              {:else}
                Connecting
              {/if}
            </span>
          </div>
        </button>
        
        <!-- Info: NETWORK, BLOCK, GNOMON -->
        <div class="network-indicator-wrapper">
          <button
            class="unified-indicator collapsed"
            on:click|stopPropagation={() => showNetworkMenu = !showNetworkMenu}
          >
            <span class="unified-dot" 
              class:dot-ok={currentNetwork.dotStatus === 'ok'} 
              class:dot-warn={currentNetwork.dotStatus === 'warn'} 
              class:dot-err={currentNetwork.dotStatus === 'err'}></span>
            <div class="rail-tooltip">
              <span class="rail-tooltip-label">Network</span>
              <span class="rail-tooltip-value"
                class:tt-ok={currentNetwork.dotStatus === 'ok'}
                class:tt-warn={currentNetwork.dotStatus === 'warn'}
                class:tt-err={currentNetwork.dotStatus === 'err'}>
                {currentNetwork.label}
              </span>
            </div>
          </button>
          
          {#if showNetworkMenu}
            <div class="network-dropdown">
              {#if simulatorStarting}
                <div class="simulator-progress">
                  <div class="progress-spinner"></div>
                  <div class="progress-text">
                    <div class="progress-message">{simulatorProgress.message}</div>
                    <div class="progress-step">Step {simulatorProgress.step}/5</div>
                  </div>
                </div>
              {/if}
              
              {#each Object.entries(networks) as [id, net]}
                <button
                  on:click|stopPropagation={() => switchNetwork(id)}
                  class="network-dropdown-option"
                  class:active={effectiveNetwork === id}
                  disabled={simulatorStarting && id !== 'simulator'}
                >
                  <span class="dot-column">
                    <span class="unified-dot" 
                      class:dot-ok={net.dotStatus === 'ok'} 
                      class:dot-warn={net.dotStatus === 'warn'} 
                      class:dot-err={net.dotStatus === 'err'}></span>
                  </span>
                  <span>{net.label}</span>
                </button>
              {/each}
            </div>
          {/if}
        </div>
        
        {#if $appState.chainHeight}
          <button
            class="unified-indicator collapsed"
            on:click|stopPropagation={(e) => handleStatusClick('block', e)}
          >
            <span class="unified-dot dot-ok"></span>
            <div class="rail-tooltip">
              <span class="rail-tooltip-label">Block</span>
              <span class="rail-tooltip-value tt-ok">
                {formatBlockHeight($appState.chainHeight)}
              </span>
            </div>
          </button>
        {/if}
        
        <!-- GNOMON at bottom (matches expanded sidebar) -->
        <button
          class="unified-indicator collapsed"
          on:click|stopPropagation={(e) => handleStatusClick('gnomon', e)}
        >
          <span class="unified-dot" 
            class:dot-ok={$appState.gnomonRunning && $appState.gnomonProgress >= 100} 
            class:dot-warn={$appState.gnomonRunning && $appState.gnomonProgress < 100}
            class:dot-err={!$appState.gnomonRunning}></span>
          <div class="rail-tooltip">
            <span class="rail-tooltip-label">Gnomon</span>
            <span class="rail-tooltip-value"
              class:tt-ok={$appState.gnomonRunning && $appState.gnomonProgress >= 100}
              class:tt-warn={$appState.gnomonRunning && $appState.gnomonProgress < 100}
              class:tt-err={!$appState.gnomonRunning}>
              {$appState.gnomonRunning ? ($appState.gnomonProgress >= 100 ? 'Synced' : `${$appState.gnomonProgress.toFixed(0)}%`) : 'Offline'}
            </span>
          </div>
        </button>
      </div>
    {/if}
  </div>
  
  <!-- v6.2 Wallet Anchor - Smart States -->
  <div class="wallet-section" class:wallet-section-collapsed={collapsed}>
    <button
      class="wallet-anchor"
      class:wallet-anchor-connected={walletIsConnected && !$appState.pendingXSWDRequests?.length}
      class:wallet-anchor-disconnected={!walletIsConnected && !xswdReadyNoWallet}
      class:wallet-anchor-xswd-only={xswdReadyNoWallet}
      class:wallet-anchor-pending={walletIsConnected && $appState.pendingXSWDRequests?.length > 0}
      on:click|stopPropagation={(e) => handleStatusClick('wallet', e)}
    >
      {#if collapsed}
        <!-- v6.3 Edge Rail: Avatar or Dot + badge -->
        {#if walletIsConnected && walletAvatarUrl}
          <img 
            src={walletAvatarUrl} 
            alt="Wallet avatar"
            title="Villager avatar editor coming soon"
            class="wallet-avatar wallet-avatar-collapsed wallet-avatar-clickable"
            class:wallet-avatar-pending={walletIsConnected && $appState.pendingXSWDRequests?.length > 0}
            on:click={handleAvatarClick}
          />
        {:else}
          <span class="wallet-dot" 
            class:dot-ok={walletIsConnected && !$appState.pendingXSWDRequests?.length}
            class:dot-err={!walletIsConnected && !xswdReadyNoWallet}
            class:dot-warn={walletIsConnected && $appState.pendingXSWDRequests?.length > 0}
            class:dot-cyan={xswdReadyNoWallet}></span>
        {/if}
        {#if connectedApps.length > 0}
          <span class="wallet-apps-badge">{connectedApps.length}</span>
        {/if}
        <!-- Tooltip -->
        <div class="rail-tooltip">
          <span class="rail-tooltip-label">Wallet</span>
          {#if walletMode === 'integrated'}
            <span class="rail-tooltip-value tt-ok">
              {$settingsState.hideAddress ? '••••••••' : formatAddressForDisplay(walletDisplayAddress)}
            </span>
            <span class="rail-tooltip-value tt-dim">Wallet Ready</span>
            {#if connectedApps.length > 0}
              <span class="rail-tooltip-value tt-dim">{connectedApps.length} {connectedApps.length === 1 ? 'app' : 'apps'}</span>
            {/if}
          {:else if walletMode === 'engram'}
            <span class="rail-tooltip-value tt-ok">Engram Connected</span>
          {:else if xswdReadyNoWallet}
            <span class="rail-tooltip-value tt-cyan">XSWD Active</span>
          {:else}
            <span class="rail-tooltip-value tt-err">Not connected</span>
          {/if}
        </div>
      {:else}
        <span class="dot-column">
          {#if walletIsConnected && walletAvatarUrl}
            <img 
              src={walletAvatarUrl} 
              alt="Wallet avatar"
              title="Villager avatar editor coming soon"
              class="wallet-avatar wallet-avatar-expanded wallet-avatar-clickable"
              class:wallet-avatar-pending={walletIsConnected && $appState.pendingXSWDRequests?.length > 0}
              on:click={handleAvatarClick}
            />
          {:else}
            <span class="wallet-dot" 
              class:dot-ok={walletIsConnected && !$appState.pendingXSWDRequests?.length}
              class:dot-err={!walletIsConnected && !xswdReadyNoWallet}
              class:dot-warn={walletIsConnected && $appState.pendingXSWDRequests?.length > 0}
              class:dot-cyan={xswdReadyNoWallet}></span>
          {/if}
        </span>
        <div class="wallet-anchor-content">
          <span class="wallet-anchor-address" class:disconnected={!walletIsConnected} class:xswd-only={xswdReadyNoWallet}>
            {#if walletIsConnected}
              {$settingsState.hideAddress ? '••••••••' : formatAddressForDisplay(walletDisplayAddress)}
            {:else}
              Connect Wallet
            {/if}
          </span>
          <span class="wallet-anchor-status">
            {#if $appState.pendingXSWDRequests?.length > 0}
              <span class="status-warn">{$appState.pendingXSWDRequests.length === 1 ? 'App requesting access' : `${$appState.pendingXSWDRequests.length} apps requesting access`}</span>
            {:else if walletMode === 'integrated'}
              {#if walletNetworkMismatch}
                <span class="status-warn">Network mismatch ({walletAddressNetwork === 'simulator' ? 'deto' : 'dero'})</span>
              {:else if effectiveNetwork === 'simulator'}
                <span class="status-sim">Simulator Wallet</span>
              {:else}
                <span class="status-ok">Wallet Ready</span>
              {/if}
              {#if connectedApps.length > 0}
                <span class="wallet-status-separator">·</span>
                <span class="wallet-apps-count">{connectedApps.length} {connectedApps.length === 1 ? 'app' : 'apps'}</span>
              {/if}
            {:else if walletMode === 'engram'}
              <span class="status-ok">Engram Connected</span>
            {:else if xswdReadyNoWallet}
              <span class="status-xswd">XSWD Active</span>
            {:else}
              <span class="status-err">Not connected</span>
            {/if}
          </span>
        </div>
      {/if}
    </button>
    
    <!-- Wallet Menu - Rendered in Sidebar -->
    {#if showWalletMenu && !collapsed}
      <div class="wallet-menu" on:click|stopPropagation>
        <!-- Current Wallet -->
        <div class="wallet-menu-section">
          <p class="wallet-menu-label">CURRENT WALLET</p>
          <p class="wallet-menu-address">
            {#if $walletState.address}
              {$settingsState.hideAddress ? '••••••••••••••' : `${$walletState.address.slice(0, 12)}...${$walletState.address.slice(-8)}`}
            {:else if $appState.engramConnected}
              Engram Wallet (External)
            {:else if xswdReadyNoWallet}
              XSWD Active
            {:else}
              Not Connected
            {/if}
          </p>
          {#if $walletState.balance !== undefined && $walletState.isOpen}
            <p class="wallet-menu-balance">
              {$settingsState.hideBalance ? '••••••••' : `${($walletState.balance / 100000).toFixed(5)} DERO`}
            </p>
          {/if}
          {#if walletNetworkMismatch && $walletState.isOpen}
            <p class="wallet-menu-warning">
              Address prefix ({walletAddressNetwork === 'simulator' ? 'deto' : 'dero'}) does not match selected network ({currentNetwork.label})
            </p>
          {/if}
        </div>
        
        {#if switchingWallet}
          <!-- Password input for switching -->
          <div class="wallet-switch-section">
            <p class="wallet-switch-label">Switch to: <span class="wallet-switch-name">{switchingWallet.filename}</span></p>
            <input
              type="password"
              bind:value={switchPassword}
              placeholder="Enter password..."
              class="input input-sm"
              on:keydown={(e) => e.key === 'Enter' && confirmSwitch()}
              on:click|stopPropagation
            />
            {#if switchError}
              <p class="wallet-switch-error">{switchError}</p>
            {/if}
            <div class="wallet-switch-actions">
              <button
                on:click|stopPropagation={() => { switchingWallet = null; switchPassword = ''; switchError = ''; }}
                class="btn btn-secondary btn-sm"
              >
                Cancel
              </button>
              <button
                on:click|stopPropagation={confirmSwitch}
                disabled={!switchPassword || connecting}
                class="btn btn-primary btn-sm"
              >
                {connecting ? '...' : 'Switch'}
              </button>
            </div>
          </div>
        {:else}
          <!-- Other wallets list -->
          {#if recentWalletsInfo.filter(w => !w.isCurrent).length > 0}
            <div class="wallet-quickswitch-list">
              <p class="wallet-menu-label">QUICK SWITCH</p>
              {#each recentWalletsInfo.filter(w => !w.isCurrent).slice(0, 5) as wallet}
                <div class="wallet-option-row">
                  <button
                    on:click|stopPropagation={() => handleQuickSwitch(wallet)}
                    class="wallet-option"
                  >
                    <span class="wallet-option-icon"><FolderOpen size={14} /></span>
                    <div class="wallet-option-info">
                      <p class="wallet-option-name">{wallet.filename}</p>
                      {#if wallet.addressPrefix}
                        <p class="wallet-option-addr">{wallet.addressPrefix}</p>
                      {/if}
                    </div>
                  </button>
                  <button
                    on:click|stopPropagation={(e) => handleRemoveWallet(wallet, e)}
                    class="wallet-remove-btn"
                    title="Remove from recent"
                  >
                    <Icons name="close" size={12} />
                  </button>
                </div>
              {/each}
            </div>
          {/if}

          <!-- Manage Avatar Placeholder -->
          <div class="wallet-menu-action">
            <button
              on:click|stopPropagation={handleAvatarClick}
              class="manage-avatar-btn"
              title="Villager avatar editor coming soon"
            >
              <span class="manage-avatar-title">Manage Avatar</span>
              <span class="manage-avatar-subtitle">Coming soon (Villager on TELA)</span>
            </button>
          </div>
          
          <!-- Connected Apps (XSWD Connections) -->
          <div class="connected-apps-section">
            <button
              on:click|stopPropagation={toggleConnectedApps}
              class="connected-apps-toggle"
            >
              <Icons name="link" size={12} />
              <span>Connected Apps</span>
              <span class="apps-count">{connectedApps.length}</span>
              <span class="toggle-arrow">{showConnectedApps ? '▲' : '▼'}</span>
            </button>
            
            {#if showConnectedApps}
              <div class="connected-apps-list">
                {#if loadingApps}
                  <p class="apps-loading">Loading...</p>
                {:else if connectedApps.length === 0}
                  <p class="apps-empty">No apps connected</p>
                {:else}
                  {#each connectedApps as app}
                    <div class="connected-app-item">
                      <div class="app-info">
                        <p class="app-name">{app.appName || 'Unknown App'}</p>
                        <p class="app-origin">{app.origin || 'Unknown origin'}</p>
                        {#if app.permissions}
                          <div class="app-permissions">
                            {#each Object.entries(app.permissions).filter(([_, v]) => v) as [perm, _]}
                              <span class="app-permission-badge">{getPermissionLabel(perm)}</span>
                            {/each}
                          </div>
                        {/if}
                        {#if app.subscriptions}
                          <div class="app-subscriptions">
                            {#if app.subscriptions.new_topoheight}
                              <span class="sub-badge">Height</span>
                            {/if}
                            {#if app.subscriptions.new_balance}
                              <span class="sub-badge">Balance</span>
                            {/if}
                            {#if app.subscriptions.new_entry}
                              <span class="sub-badge">Entries</span>
                            {/if}
                          </div>
                        {/if}
                      </div>
                      <button
                        on:click|stopPropagation={() => revokeApp(app.origin)}
                        class="app-revoke-btn"
                        title="Revoke access"
                      >
                        <Icons name="close" size={14} />
                      </button>
                    </div>
                  {/each}
                {/if}
              </div>
            {/if}
          </div>
          
          <!-- Disconnect button -->
          <div class="wallet-menu-footer">
            <button
              on:click|stopPropagation={toggleWalletConnection}
              class="wallet-disconnect-btn"
            >
              Disconnect Wallet
            </button>
          </div>
        {/if}
      </div>
    {/if}
  </div>
  
  <!-- v6.3 Collapse Toggle - Panel Icon -->
  <button
    on:click={toggleCollapse}
    class="collapse-btn"
    title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
  >
    {#if collapsed}
      <!-- Expand: Panel with arrow pointing right -->
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="panel-icon">
        <rect x="3" y="3" width="18" height="18" rx="2"/>
        <line x1="9" y1="3" x2="9" y2="21"/>
        <polyline points="12 8 15 12 12 16"/>
      </svg>
    {:else}
      <!-- Collapse: Panel with arrow pointing left -->
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" class="panel-icon">
        <rect x="3" y="3" width="18" height="18" rx="2"/>
        <line x1="9" y1="3" x2="9" y2="21"/>
        <polyline points="15 8 12 12 15 16"/>
      </svg>
    {/if}
  </button>
</aside>

<!-- Simulator Confirmation Modal - Hologram v6.1 Style -->
{#if showSimulatorConfirm}
  <div class="sim-modal-backdrop" on:click={cancelSimulatorAction}>
    <div class="sim-modal-card" on:click|stopPropagation>
      <!-- Status Bar -->
      <div class="sim-modal-status">
        <div class="sim-modal-status-left">
          <span class="sim-modal-status-dot" class:start={simulatorAction === 'start'} class:stop={simulatorAction === 'stop'}></span>
          <span class="sim-modal-status-text">{simulatorAction === 'start' ? 'Confirm' : 'Warning'}</span>
        </div>
        <span class="sim-modal-badge" class:start={simulatorAction === 'start'} class:stop={simulatorAction === 'stop'}>
          {simulatorAction === 'start' ? 'SIMULATOR' : 'STOPPING'}
        </span>
      </div>
      
      <!-- Icon + Title -->
      <div class="sim-modal-header">
        <div class="sim-modal-icon" class:start={simulatorAction === 'start'} class:stop={simulatorAction === 'stop'}>
          <Gamepad2 size={28} strokeWidth={1.5} />
        </div>
        <h2 class="sim-modal-title">
          {#if simulatorAction === 'start'}
            Start Simulator Mode
          {:else}
            Stop Simulator
          {/if}
        </h2>
        <p class="sim-modal-desc">
          {#if simulatorAction === 'start'}
            Launch a local test environment for development
          {:else}
            Return to production network
          {/if}
        </p>
      </div>
      
      <!-- Content -->
      <div class="sim-modal-body">
        {#if simulatorAction === 'start'}
          <div class="sim-modal-features">
            <div class="sim-modal-feature">
              <Radio size={14} class="sim-feature-icon" />
              <span>Launch local DERO daemon</span>
            </div>
            <div class="sim-modal-feature">
              <Wallet size={14} class="sim-feature-icon" />
              <span>Create test wallet automatically</span>
            </div>
            <div class="sim-modal-feature">
              <Diamond size={14} class="sim-feature-icon" />
              <span>Generate free test DERO</span>
            </div>
          </div>
          
          <div class="sim-modal-note cyan">
            <strong>Perfect for:</strong> Testing TELA apps, learning smart contracts, development without cost.
          </div>
          
          <div class="sim-modal-note warn">
            Test DERO has no real value and cannot be transferred to mainnet.
          </div>
        {:else}
          <div class="sim-modal-features">
            <div class="sim-modal-feature">
              <Radio size={14} class="sim-feature-icon" />
              <span>Stop local daemon process</span>
            </div>
            <div class="sim-modal-feature">
              <Wallet size={14} class="sim-feature-icon" />
              <span>Close simulator wallet</span>
            </div>
            <div class="sim-modal-feature">
              <Globe2 size={14} class="sim-feature-icon" />
              <span>Switch back to Mainnet</span>
            </div>
          </div>
          
          <div class="sim-modal-note cyan">
            Your simulator data will be preserved for next time.
          </div>
        {/if}
      </div>
      
      <!-- Actions -->
      <div class="sim-modal-actions">
        <button class="sim-modal-btn secondary" on:click={cancelSimulatorAction}>
          Cancel
        </button>
        <button class="sim-modal-btn" class:primary={simulatorAction === 'start'} class:warn={simulatorAction === 'stop'} on:click={confirmSimulatorAction}>
          {#if simulatorAction === 'start'}
            Start Simulator
          {:else}
            Stop Simulator
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Network Switch Progress Modal -->
{#if showNetworkSwitchModal}
  <div class="network-switch-backdrop">
    <div class="network-switch-modal">
      <!-- Header -->
      <div class="network-switch-header">
        <div class="network-switch-icon" class:simulator={networkSwitchTarget === 'simulator'} class:mainnet={networkSwitchTarget === 'mainnet'}>
          {#if networkSwitchTarget === 'simulator'}
            <Gamepad2 size={24} strokeWidth={1.5} />
          {:else}
            <Globe2 size={24} strokeWidth={1.5} />
          {/if}
        </div>
        <h2 class="network-switch-title">
          Switching to {networkSwitchTarget === 'simulator' ? 'Simulator' : 'Mainnet'}
        </h2>
        <p class="network-switch-subtitle">
          {#if networkSwitchTarget === 'simulator'}
            This typically takes about 30 seconds
          {:else}
            Stopping simulator and reconnecting...
          {/if}
        </p>
      </div>
      
      {#if networkSwitchTarget === 'simulator'}
        <!-- Step Checklist (only for simulator start) -->
        <div class="network-switch-steps">
          {#each networkSwitchSteps as step}
            {@const isComplete = simulatorProgress.step > step.id}
            {@const isInProgress = simulatorProgress.step === step.id}
            {@const isPending = simulatorProgress.step < step.id}
            <div class="network-switch-step" class:complete={isComplete} class:in-progress={isInProgress} class:pending={isPending}>
              <div class="step-icon">
                {#if isComplete}
                  <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
                    <path d="M3 8L6.5 11.5L13 5" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                {:else if isInProgress}
                  <div class="step-spinner"></div>
                {:else}
                  <div class="step-circle"></div>
                {/if}
              </div>
              <span class="step-label">{step.label}</span>
            </div>
          {/each}
        </div>
        
        <!-- Progress Bar -->
        <div class="network-switch-progress">
          <div class="progress-bar-track">
            <div 
              class="progress-bar-fill simulator"
              style="width: {Math.min((simulatorProgress.step / 5) * 100, 100)}%"
            ></div>
          </div>
          <span class="progress-text">Step {simulatorProgress.step}/{networkSwitchSteps.length}</span>
        </div>
      {:else}
        <!-- Simple spinner for mainnet (stopping simulator) -->
        <div class="network-switch-simple">
          <div class="simple-spinner"></div>
          <p class="simple-message">Closing simulator services and connecting to mainnet node...</p>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* === SIDEBAR COMPONENT OVERRIDES ===
     Base styles are in hologram.css - only component-specific additions here */
  
  /* Height and transition for collapse animation */
  .sidebar {
    height: 100%;
    transition: width var(--dur-med) var(--ease-out);
    overflow: visible; /* Allow tooltips to overflow */
  }
  
  /* v6.3 Edge Rail - 40px collapsed width */
  .sidebar.collapsed {
    width: 40px;
    overflow: visible; /* Allow tooltips to overflow */
  }
  
  /* Hide any scrollbars in collapsed mode */
  .sidebar.collapsed ::-webkit-scrollbar {
    display: none;
  }
  
  .sidebar.collapsed * {
    scrollbar-width: none; /* Firefox */
  }
  
  /* v6.2 Center the logo now that version is removed */
  .sidebar-head {
    justify-content: center;
  }
  
  /* Collapsed state: maintain same height for alignment */
  .sidebar.collapsed .sidebar-head {
    height: 80px;
    padding: 0;
  }
  
  .sidebar.collapsed .sidebar-logo-sm {
    width: 18px;
    height: 18px;
  }
  
  /* Menu container - 16px horizontal padding for breathing room */
  .sidebar-menu {
    display: flex;
    flex-direction: column;
    gap: 8px;
    overflow-y: auto;
    overflow-x: hidden; /* Never show horizontal scrollbar */
    padding: 8px 16px;
  }
  
  .sidebar.collapsed .sidebar-menu {
    gap: 2px;
    padding: 4px 0;
    flex: 1; /* Take remaining space to push status to bottom */
    overflow: visible; /* Allow tooltips to overflow */
  }
  
  /* Nav items - clean with proper spacing */
  .nav-item {
    width: 100%;
    text-align: left;
    border: 1px solid var(--border-subtle);
    background: var(--void-base);
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    font-size: 13px;
    font-weight: 500;
    letter-spacing: 0.02em;
    color: var(--text-2);
    border-radius: 6px;
    cursor: pointer;
    transition: all 150ms ease;
    position: relative;
  }
  
  .nav-item:hover {
    background: var(--void-mid);
    border-color: var(--cyan-500);
    color: var(--text-1);
  }
  
  .nav-item.active {
    background: var(--void-mid);
    border-color: var(--cyan-500);
    color: var(--cyan-400);
  }
  
  /* v6.3 Edge Rail: Collapsed nav items */
  .sidebar.collapsed .nav-item {
    padding: 0;
    width: 32px;
    height: 32px;
    justify-content: center;
    margin: 0 auto;
    border-radius: var(--r-sm);
    background: transparent;
    overflow: visible; /* Allow tooltips */
    position: relative; /* Ensure tooltip positioning */
  }
  
  .sidebar.collapsed .nav-item:hover {
    background: var(--void-hover);
  }
  
  .sidebar.collapsed .nav-item.active {
    background: transparent;
  }
  
  /* v6.3 Active indicator bar (left edge) */
  .sidebar.collapsed .nav-item.active::before {
    content: '';
    position: absolute;
    left: -4px;
    top: 6px;
    bottom: 6px;
    width: 2px;
    background: var(--cyan-400);
    border-radius: 1px;
    box-shadow: 0 0 6px var(--cyan-400);
  }
  
  /* Nav icon - fixed width column for alignment */
  .nav-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    flex-shrink: 0;
    opacity: 0.7;
  }
  
  .nav-item:hover .nav-icon { 
    color: var(--text-1); 
    opacity: 1;
  }
  .nav-item.active .nav-icon { 
    color: var(--cyan-400); 
    opacity: 1;
  }
  
  /* v6.3 Edge Rail: Larger icon in collapsed */
  .sidebar.collapsed .nav-icon {
    width: auto;
    opacity: 0.7;
  }
  
  .sidebar.collapsed .nav-item:hover .nav-icon {
    opacity: 1;
  }
  
  .sidebar.collapsed .nav-item.active .nav-icon {
    opacity: 1;
    color: var(--cyan-400);
  }
  
  /* Status Panel - 16px horizontal padding to match nav */
  .sidebar-status {
    padding: 8px 16px;
    border-top: 1px solid rgba(255, 255, 255, 0.02);
  }
  
  /* v6.3 Edge Rail: Status LED strip */
  .sidebar.collapsed .sidebar-status {
    padding: 12px 0;
    overflow: visible; /* Allow tooltips to show */
  }
  
  /* ============================================
     v6.6 SERVICE ROWS - E2 Unified Layout
     ============================================ */
  
  .service-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: var(--void-base);
    border: 1px solid var(--border-subtle);
    border-radius: 6px;
    cursor: pointer;
    transition: all 150ms ease;
    width: 100%;
    text-align: left;
  }
  
  .service-row:hover {
    border-color: var(--cyan-500);
    background: var(--void-mid);
  }
  
  .service-row:focus {
    outline: none;
  }
  
  .service-row-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-4);
    transition: all 150ms ease;
    flex-shrink: 0;
    margin-left: auto;
  }
  
  .service-row-dot.ok {
    background: var(--status-ok);
    box-shadow: 0 0 6px var(--status-ok);
  }
  
  .service-row-dot.warn {
    background: var(--status-warn);
    box-shadow: 0 0 6px var(--status-warn);
  }
  
  .service-row-dot.err {
    background: var(--status-err);
    box-shadow: 0 0 4px var(--status-err);
    opacity: 0.6;
  }
  
  .service-row-label {
    font-size: 10px;
    font-weight: 500;
    letter-spacing: 0.15em;
    text-transform: uppercase;
    color: var(--text-4);
  }
  
  /* ============================================
     v6.5 GNOMON PROGRESS ROW - Option B Style
     ============================================ */
  
  .gnomon-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: var(--void-base);
    border: 1px solid var(--border-subtle);
    border-radius: 6px;
    cursor: pointer;
    transition: all 150ms ease;
    gap: 12px;
    width: 100%;
    text-align: left;
  }
  
  .gnomon-row:hover {
    border-color: var(--cyan-500);
    background: var(--void-mid);
  }
  
  .gnomon-label {
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: var(--text-4);
    flex-shrink: 0;
    min-width: 58px;
  }
  
  .gnomon-progress-container {
    flex: 1;
    display: flex;
    align-items: center;
    gap: 0;
  }
  
  .gnomon-progress-bar {
    flex: 1;
    height: 4px;
    background: var(--void-hover);
    border-radius: 2px;
    overflow: hidden;
    position: relative;
  }
  
  .gnomon-progress-fill {
    height: 100%;
    border-radius: 2px;
    transition: width 0.3s ease-out;
  }
  
  /* Syncing state - cyan animated shimmer */
  .gnomon-progress-fill.syncing {
    background: linear-gradient(90deg, var(--cyan-600), var(--cyan-400), var(--cyan-600));
    background-size: 200% 100%;
    animation: gnomon-shimmer 2s ease-in-out infinite;
  }
  
  /* Synced state - green static */
  .gnomon-progress-fill.synced {
    background: var(--status-ok);
  }
  
  /* Loading apps state - green shimmer+pulse animation
     Indicates: Gnomon is synced (100%) but apps are still being discovered/loaded
     This bridges the UX gap between "synced" indicator and apps actually appearing */
  .gnomon-progress-fill.loading-apps {
    background: linear-gradient(90deg, var(--status-ok), #5effc1, var(--status-ok));
    background-size: 200% 100%;
    animation: gnomon-shimmer-pulse 3s ease-in-out infinite;
  }
  
  @keyframes gnomon-shimmer-pulse {
    0% { 
      background-position: 200% 0;
      opacity: 1;
    }
    25% { opacity: 0.7; }
    50% { 
      background-position: 0% 0;
      opacity: 1;
    }
    75% { opacity: 0.7; }
    100% { 
      background-position: -200% 0;
      opacity: 1;
    }
  }
  
  /* Offline state - red dim */
  .gnomon-progress-fill.offline {
    background: var(--status-err);
    opacity: 0.4;
  }
  
  @keyframes gnomon-shimmer {
    0% { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }
  
  /* Row state styling - all borders consistent (E2 style) */
  .gnomon-row.gnomon-synced,
  .gnomon-row.gnomon-syncing,
  .gnomon-row.gnomon-loading-apps,
  .gnomon-row.gnomon-offline {
    border-color: var(--border-subtle);
  }
  
  .gnomon-row.gnomon-synced:hover,
  .gnomon-row.gnomon-syncing:hover,
  .gnomon-row.gnomon-loading-apps:hover,
  .gnomon-row.gnomon-offline:hover {
    border-color: var(--cyan-500);
  }
  
  /* ============================================
     v6.4 INFO ROWS - Network & Block (E3 Style)
     ============================================ */
  
  .info-rows {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  
  .info-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 12px;
    background: var(--void-base);
    border: 1px solid var(--border-subtle);
    border-radius: 6px;
    cursor: pointer;
    transition: all 150ms ease;
    width: 100%;
    text-align: left;
  }
  
  .info-row:hover {
    border-color: var(--cyan-500);
    background: var(--void-mid);
  }
  
  .info-label {
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: var(--text-4);
  }
  
  .info-value {
    font-size: 12px;
    font-weight: 600;
    color: var(--cyan-400);
    text-align: right;
    min-width: 64px;
  }
  
  .info-value.value-ok { color: var(--status-ok); }
  .info-value.value-warn { color: var(--status-warn); }
  .info-value.value-err { color: var(--status-err); }
  .info-value.value-cyan { color: var(--cyan-400); }
  
  /* ============================================
     COLLAPSED STATE - LED Strip (unchanged)
     ============================================ */
  
  .status-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
  }
  
  /* Force all direct children of status-list to take full width */
  .status-list > * {
    flex: 0 0 auto;
    align-self: stretch;
  }
  
  /* v6.3 Edge Rail: Collapsed status list is centered dots */
  .sidebar.collapsed .status-list {
    align-items: center;
    gap: 6px;
  }
  
  /* Dot column - same width as nav-icon for alignment */
  .dot-column {
    width: 18px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  
  /* v6.3: In collapsed mode, dot-column is auto width */
  .sidebar.collapsed .dot-column {
    width: auto;
  }
  
  /* Expand dot-column when it contains an avatar or gradient placeholder */
  .wallet-anchor .dot-column {
    width: auto;
    min-width: 40px;
    align-items: center;
    justify-content: center;
  }
  
  /* Specific widths for avatar sizes */
  .wallet-anchor .dot-column .wallet-avatar-expanded {
    width: 40px !important;
    height: 40px !important;
    min-width: 40px !important;
    min-height: 40px !important;
    max-width: 40px !important;
    max-height: 40px !important;
  }
  
  .wallet-anchor .dot-column .wallet-avatar-collapsed {
    width: 24px !important;
    height: 24px !important;
    min-width: 24px !important;
    min-height: 24px !important;
    max-width: 24px !important;
    max-height: 24px !important;
  }
  
  /* Unified Indicator - matching padding with nav items */
  .unified-indicator {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 14px;
    background: transparent;
    border: none;
    border-radius: var(--r-md);
    cursor: pointer;
    transition: background 150ms ease;
    text-align: left;
    position: relative;
    box-sizing: border-box;
    /* Full width: multiple fallbacks for cross-browser */
    width: 100%;
    min-width: 100%;
  }
  
  .unified-indicator:hover {
    background: var(--void-hover);
  }
  
  /* v6.3 Edge Rail: Collapsed indicator is just a dot */
  .unified-indicator.collapsed {
    display: flex;
    width: auto;
    padding: 3px;
    justify-content: center;
    border-radius: 50%;
  }
  
  .unified-indicator.collapsed:hover {
    background: var(--void-hover);
    border-radius: var(--r-sm);
    padding: 3px 6px;
  }
  
  /* v6.2 Unified Dot - 6px with glow */
  .unified-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--text-4);
    transition: all 150ms ease;
  }
  
  .unified-dot.dot-ok { background: var(--status-ok); box-shadow: 0 0 6px var(--status-ok); }
  .unified-dot.dot-warn { background: var(--status-warn); box-shadow: 0 0 6px var(--status-warn); }
  .unified-dot.dot-err { background: var(--status-err); box-shadow: 0 0 6px var(--status-err); }
  .unified-dot.dot-cyan { background: var(--cyan-400); box-shadow: 0 0 6px var(--cyan-400); }
  
  /* v6.3 Edge Rail: Smaller dots (5px) for LED strip */
  .sidebar.collapsed .unified-dot {
    width: 5px;
    height: 5px;
  }
  
  .sidebar.collapsed .unified-dot.dot-ok { box-shadow: 0 0 4px var(--status-ok); }
  .sidebar.collapsed .unified-dot.dot-warn { box-shadow: 0 0 4px var(--status-warn); }
  .sidebar.collapsed .unified-dot.dot-err { box-shadow: 0 0 4px var(--status-err); }
  .sidebar.collapsed .unified-dot.dot-cyan { box-shadow: 0 0 4px var(--cyan-400); }
  
  /* ============================================
     v6.3 TOOLTIP SYSTEM FOR COLLAPSED STATE
     ============================================ */
  
  .rail-tooltip {
    position: absolute;
    left: calc(100% + 10px);
    top: 50%;
    transform: translateY(-50%) translateX(-6px);
    background: var(--void-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    padding: 8px 12px;
    white-space: nowrap;
    opacity: 0;
    pointer-events: none;
    transition: opacity 150ms ease, transform 150ms ease;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    z-index: 1000;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  /* Show tooltip on hover (only in collapsed state) */
  .sidebar.collapsed .nav-item:hover .rail-tooltip,
  .sidebar.collapsed .unified-indicator:hover .rail-tooltip,
  .sidebar.collapsed .wallet-anchor:hover .rail-tooltip {
    opacity: 1;
    transform: translateY(-50%) translateX(0);
  }
  
  .rail-tooltip-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-1);
  }
  
  .rail-tooltip-value {
    font-size: 10px;
    color: var(--text-3);
  }
  
  .rail-tooltip-value.tt-ok { color: var(--status-ok); }
  .rail-tooltip-value.tt-warn { color: var(--status-warn); }
  .rail-tooltip-value.tt-err { color: var(--status-err); }
  .rail-tooltip-value.tt-cyan { color: var(--cyan-400); }
  .rail-tooltip-value.tt-dim { color: var(--text-4); font-size: 9px; }
  
  /* Hide tooltip in expanded state */
  .sidebar:not(.collapsed) .rail-tooltip {
    display: none;
  }
  
  /* Ensure nav item labels fade smoothly */
  .nav-item > span:not(.nav-icon):not(.rail-tooltip) {
    transition: opacity 150ms ease;
  }
  
  .sidebar.collapsed .nav-item > span:not(.nav-icon):not(.rail-tooltip) {
    display: none;
  }
  
  /* Network Indicator Wrapper (for dropdown positioning) */
  .network-indicator-wrapper {
    position: relative;
    width: -webkit-fill-available;
    width: -moz-available;
    width: stretch;
  }
  
  .network-dropdown {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 100%;
    margin-bottom: 4px;
    background: var(--void-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.5);
    z-index: 50;
    overflow: hidden;
  }
  
  .network-dropdown-option {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 12px;
    font-size: 11px;
    color: var(--text-2);
    background: transparent;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background 150ms ease;
  }
  
  .network-dropdown-option:hover { background: var(--void-hover); }
  .network-dropdown-option.active { color: var(--cyan-400); background: rgba(34, 211, 238, 0.08); }
  .network-dropdown-option:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  /* Simulator Progress Indicator */
  .simulator-progress {
    padding: var(--s-3) var(--s-4);
    display: flex;
    align-items: center;
    gap: var(--s-3);
    border-bottom: 1px solid var(--border-dim);
    background: var(--void-deep);
  }
  
  .progress-spinner {
    width: 16px;
    height: 16px;
    border: 2px solid var(--border-subtle);
    border-top-color: var(--cyan-400);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
    flex-shrink: 0;
  }
  
  .progress-text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  
  .progress-message {
    font-size: 12px;
    color: var(--text-2);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .progress-step {
    font-size: 11px;
    color: var(--text-4);
    font-family: var(--font-mono);
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  /* Wallet Section - E3 Card Style with proper spacing */
  .wallet-section {
    padding: 12px 16px;
    margin-top: 12px;
    position: relative;
  }
  
  /* v6.3 Edge Rail: Collapsed wallet section */
  .wallet-section-collapsed {
    padding: 10px 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
  }
  
  /* v6.2 Wallet Anchor - E3 Card Style */
  .wallet-anchor {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    background: var(--void-base);
    border: 1px solid var(--border-subtle);
    border-radius: 8px;
    cursor: pointer;
    transition: all 150ms ease;
    width: 100%;
    text-align: left;
    position: relative;
    /* Ensure enough space for content */
    min-width: 0;
    overflow: visible;
    box-shadow: none;
  }
  
  .wallet-anchor:hover {
    background: var(--void-mid);
    border-color: rgba(34, 211, 238, 0.35);
    box-shadow: 0 0 10px rgba(34, 211, 238, 0.18);
  }
  
  /* v6.3 Edge Rail: Collapsed wallet anchor */
  .wallet-section-collapsed .wallet-anchor {
    width: auto;
    padding: 6px;
    flex-direction: column;
    gap: 4px;
    background: transparent;
    border: none;
  }
  
  .wallet-section-collapsed .wallet-anchor:hover {
    background: var(--void-hover);
    border-radius: var(--r-sm);
  }
  
  .wallet-anchor-connected {
    background: var(--void-base);
    border-color: rgba(34, 211, 238, 0.35);
    box-shadow: 0 0 10px rgba(34, 211, 238, 0.18);
  }
  
  .wallet-anchor-disconnected {
    background: var(--void-base);
    border-color: var(--border-subtle);
    box-shadow: none;
  }
  
  .wallet-anchor-disconnected:hover {
    background: var(--void-mid);
    border-color: var(--cyan-500);
    box-shadow: 0 0 8px rgba(34, 211, 238, 0.2);
  }
  
  /* XSWD Active but no wallet state - subtle cyan */
  .wallet-anchor-xswd-only {
    background: var(--void-base);
    border-color: rgba(34, 211, 238, 0.28);
    box-shadow: none;
  }
  
  .wallet-anchor-xswd-only:hover {
    background: var(--void-mid);
    border-color: var(--cyan-500);
    box-shadow: 0 0 8px rgba(34, 211, 238, 0.2);
  }
  
  .wallet-anchor-pending {
    background: rgba(251, 191, 36, 0.06);
    border-color: rgba(251, 191, 36, 0.2);
    animation: wallet-pulse 2s ease infinite;
  }
  
  .wallet-anchor-pending:hover {
    background: rgba(251, 191, 36, 0.1);
    border-color: rgba(251, 191, 36, 0.3);
  }
  
  @keyframes wallet-pulse {
    0%, 100% { border-color: rgba(251, 191, 36, 0.2); }
    50% { border-color: rgba(251, 191, 36, 0.35); }
  }
  
  /* Gradient avatar placeholder (E3 Style) - shows when no custom avatar */
  .wallet-dot {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    flex-shrink: 0;
    background: linear-gradient(135deg, var(--cyan-400), var(--emerald-400, #34d399));
    border: 2px solid var(--cyan-400);
    transition: all 150ms ease;
  }
  
  .wallet-dot.dot-ok { 
    background: linear-gradient(135deg, var(--cyan-400), var(--emerald-400, #34d399));
    border-color: var(--cyan-400);
    box-shadow: none;
  }
  .wallet-dot.dot-warn { 
    background: linear-gradient(135deg, var(--status-warn), #f59e0b);
    border-color: var(--status-warn);
    box-shadow: none;
  }
  .wallet-dot.dot-err { 
    background: linear-gradient(135deg, var(--status-err), #dc2626);
    border-color: var(--status-err);
    opacity: 0.5;
    box-shadow: none;
  }
  .wallet-dot.dot-cyan { 
    background: linear-gradient(135deg, var(--cyan-400), var(--emerald-400, #34d399));
    border-color: var(--cyan-400);
    box-shadow: none;
  }
  
  /* Smaller dot for collapsed state */
  .wallet-section-collapsed .wallet-dot {
    width: 24px;
    height: 24px;
    border-width: 1.5px;
  }
  
  /* Villager Avatar Styles */
  .wallet-avatar {
    border-radius: 50%;
    object-fit: cover;
    object-position: center;
    flex-shrink: 0;
    border: 2px solid var(--cyan-400);
    background: var(--void);
    transition: border-color 150ms ease, transform 150ms ease;
    display: block;
    aspect-ratio: 1 / 1;
    /* Force square dimensions - prevent any stretching */
    box-sizing: border-box;
    padding: 0;
    margin: 0;
  }
  
  .wallet-avatar:hover {
    transform: scale(1.05);
  }
  
  /* Clickable avatar with enhanced visual cues */
  .wallet-avatar-clickable {
    cursor: pointer;
    transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
  }
  
  .wallet-avatar-clickable:hover {
    transform: scale(1.1);
    box-shadow: 0 0 12px rgba(0, 255, 255, 0.5);
    border-color: var(--cyan-300);
  }
  
  .wallet-avatar-clickable:active {
    transform: scale(0.95);
  }
  
  .wallet-avatar-collapsed {
    width: 24px;
    height: 24px;
    min-width: 24px;
    min-height: 24px;
    max-width: 24px;
    max-height: 24px;
    border-width: 1.5px;
  }
  
  .wallet-avatar-expanded {
    width: 40px;
    height: 40px;
    min-width: 40px;
    min-height: 40px;
    max-width: 40px;
    max-height: 40px;
    border-width: 2px;
  }
  
  .wallet-avatar-pending {
    border-color: var(--yellow-400);
    animation: wallet-avatar-pulse 2s ease infinite;
  }
  
  @keyframes wallet-avatar-pulse {
    0%, 100% { 
      border-color: var(--yellow-400);
      box-shadow: 0 0 0 0 rgba(251, 191, 36, 0.4);
    }
    50% { 
      border-color: var(--yellow-300);
      box-shadow: 0 0 8px 2px rgba(251, 191, 36, 0.6);
    }
  }
  
  .wallet-anchor-connected .wallet-avatar {
    border-color: var(--cyan-400);
  }
  
  .wallet-anchor-pending .wallet-avatar {
    border-color: var(--yellow-400);
  }
  
  /* v6.3 Edge Rail: Wallet apps badge */
  .wallet-apps-badge {
    font-size: 8px;
    font-weight: 600;
    color: var(--text-4);
    line-height: 1;
  }
  
  .wallet-anchor-content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    justify-content: center;
    gap: 2px;
    text-align: left;
  }
  
  .wallet-anchor-address {
    font-size: 11px;
    font-weight: 600;
    color: var(--cyan-400);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    letter-spacing: 0.01em;
    font-variant-numeric: tabular-nums;
    width: 100%;
  }
  
  .wallet-anchor-address.disconnected {
    color: var(--text-3);
    white-space: normal;
    overflow: visible;
    text-overflow: clip;
  }
  
  .wallet-anchor-status {
    font-size: 10px;
    color: var(--status-ok);
    display: flex;
    align-items: center;
    justify-content: flex-start;
    gap: 6px;
  }
  
  .wallet-status-separator {
    color: var(--text-5);
  }
  
  .wallet-apps-count {
    color: var(--text-3);
  }
  
  /* Wallet Menu - Positioned relative to wallet display */
  .wallet-menu {
    position: absolute;
    left: var(--s-3);
    right: var(--s-3);
    bottom: 100%;
    margin-bottom: 4px;
    min-width: 280px;
    max-width: 320px;
    background: var(--void-surface);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    box-shadow: 0 12px 32px rgba(0, 0, 0, 0.5);
    z-index: 100;
    overflow: hidden;
    max-height: 70vh;
    overflow-y: auto;
  }
  
  .wallet-menu-section {
    padding: var(--s-3);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .wallet-menu-label {
    font-size: 9px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: var(--text-5);
    margin-bottom: var(--s-1);
  }
  
  .wallet-menu-address {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--status-ok);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .wallet-menu-balance {
    font-size: 11px;
    color: var(--text-3);
    margin-top: var(--s-1);
  }

  .wallet-menu-warning {
    margin-top: var(--s-2);
    font-size: 10px;
    line-height: 1.4;
    color: var(--status-warn);
  }
  
  .wallet-option {
    width: 100%;
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    background: transparent;
    border: none;
    cursor: pointer;
    text-align: left;
    transition: background var(--dur-fast);
  }
  
  .wallet-option:hover {
    background: var(--void-hover);
  }
  
  .wallet-option-icon {
    font-size: 12px;
  }
  
  .wallet-option-info {
    flex: 1;
    min-width: 0;
  }
  
  .wallet-option-name {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-1);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .wallet-option-addr {
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-4);
  }
  
  .wallet-option-row {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  
  .wallet-option-row .wallet-option {
    flex: 1;
    min-width: 0;
  }
  
  .wallet-remove-btn {
    padding: 6px;
    background: transparent;
    border: none;
    color: var(--text-5);
    cursor: pointer;
    border-radius: var(--r-sm);
    opacity: 0;
    transition: all var(--dur-fast);
    flex-shrink: 0;
  }
  
  .wallet-option-row:hover .wallet-remove-btn {
    opacity: 1;
  }
  
  .wallet-remove-btn:hover {
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err);
  }
  
  .wallet-menu-footer {
    padding: var(--s-2);
    border-top: 1px solid var(--border-dim);
  }
  
  .wallet-disconnect-btn {
    width: 100%;
    padding: var(--s-2) var(--s-3);
    font-size: 11px;
    font-weight: 500;
    color: var(--status-err);
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    cursor: pointer;
    transition: background var(--dur-fast);
    text-align: center;
  }
  
  .wallet-disconnect-btn:hover {
    background: rgba(248, 113, 113, 0.08);
  }

  /* Manage Avatar Placeholder Button */
  .wallet-menu-action {
    padding: var(--s-2) var(--s-3);
    border-bottom: 1px solid var(--border-dim);
  }

  .manage-avatar-btn {
    width: 100%;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
    padding: var(--s-2) var(--s-3);
    font-size: 11px;
    background: transparent;
    border: 1px dashed rgba(34, 211, 238, 0.28);
    border-radius: var(--r-sm);
    cursor: pointer;
    transition: background var(--dur-fast), border-color var(--dur-fast);
    text-align: left;
  }

  .manage-avatar-btn:hover {
    background: rgba(0, 255, 255, 0.06);
    border-color: rgba(34, 211, 238, 0.42);
  }

  .manage-avatar-title {
    color: var(--cyan-400);
    font-weight: 500;
  }

  .manage-avatar-subtitle {
    color: var(--text-4);
    font-size: 10px;
  }
  
  .wallet-switch-section {
    padding: var(--s-3);
    border-bottom: 1px solid var(--border-dim);
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .wallet-switch-label {
    font-size: 11px;
    color: var(--text-3);
  }
  
  .wallet-switch-name {
    color: var(--text-1);
  }
  
  .wallet-switch-error {
    font-size: 11px;
    color: var(--status-err);
  }
  
  .wallet-switch-actions {
    display: flex;
    gap: var(--s-2);
  }
  
  .wallet-switch-actions .btn {
    flex: 1;
  }
  
  .wallet-quickswitch-list {
    padding: var(--s-1) 0;
  }
  
  .wallet-quickswitch-list .wallet-menu-label {
    padding: var(--s-1) var(--s-3);
  }
  
  .connected-apps-section {
    border-top: 1px solid var(--border-dim);
  }
  
  .connected-apps-toggle {
    width: 100%;
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-2);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background var(--dur-fast);
  }
  
  .connected-apps-toggle:hover {
    background: var(--void-hover);
  }
  
  .apps-count {
    margin-left: auto;
    font-size: 10px;
    padding: 1px 6px;
    background: var(--void-up);
    border-radius: var(--r-sm);
    color: var(--text-3);
  }
  
  .toggle-arrow {
    font-size: 8px;
    color: var(--text-4);
  }
  
  .connected-apps-list {
    max-height: 200px;
    overflow-y: auto;
    border-top: 1px solid var(--border-dim);
  }
  
  .apps-loading,
  .apps-empty {
    padding: var(--s-3);
    font-size: 11px;
    color: var(--text-4);
    text-align: center;
  }
  
  .connected-app-item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: var(--s-2) var(--s-3);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .connected-app-item:last-child {
    border-bottom: none;
  }
  
  .app-info {
    flex: 1;
    min-width: 0;
  }
  
  .app-name {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-1);
    margin-bottom: 2px;
  }
  
  .app-origin {
    font-family: var(--font-mono);
    font-size: 9px;
    color: var(--text-4);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    margin-bottom: 4px;
  }
  
  .app-permissions {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-bottom: 4px;
  }
  
  .app-permission-badge {
    font-size: 8px;
    padding: 1px 4px;
    background: rgba(52, 211, 153, 0.15);
    border: 1px solid rgba(52, 211, 153, 0.3);
    border-radius: 3px;
    color: var(--status-ok);
  }
  
  .app-subscriptions {
    display: flex;
    gap: 4px;
  }
  
  .sub-badge {
    font-size: 8px;
    padding: 1px 4px;
    background: rgba(96, 165, 250, 0.15);
    border: 1px solid rgba(96, 165, 250, 0.3);
    border-radius: 3px;
    color: var(--status-info);
  }
  
  .app-revoke-btn {
    padding: 4px;
    background: transparent;
    border: none;
    color: var(--text-4);
    cursor: pointer;
    border-radius: var(--r-sm);
    transition: all var(--dur-fast);
  }
  
  .app-revoke-btn:hover {
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err);
  }
  
  /* Status color helpers (used in wallet anchor) */
  .status-ok { 
    color: var(--status-ok) !important; 
  }
  
  .status-sim {
    color: var(--emerald-400, #34d399) !important;
  }
  
  .status-warn { 
    color: var(--status-warn) !important; 
  }
  
  .status-err {
    color: var(--status-err) !important;
  }
  
  .status-xswd {
    color: var(--cyan-400) !important;
  }
  
  /* XSWD-only address styling */
  .wallet-anchor-address.xswd-only {
    color: var(--cyan-400);
  }
  
  /* Collapse Button */
  .collapse-btn {
    padding: var(--s-3);
    border: none;
    border-top: 1px solid var(--border-dim);
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
    transition: all var(--dur-fast);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .collapse-btn:hover {
    color: var(--text-1);
    background: var(--void-hover);
  }
  
  .collapse-btn:hover .panel-icon {
    color: var(--cyan-400);
  }
  
  /* v6.3 Panel Icon styling */
  .panel-icon {
    transition: color 150ms ease;
  }
  
  .panel-icon rect {
    stroke: currentColor;
    fill: none;
  }
  
  .panel-icon line {
    stroke: currentColor;
    opacity: 0.5;
  }
  
  .panel-icon polyline {
    stroke: currentColor;
  }
  
  .collapse-btn:hover .panel-icon line {
    opacity: 0.7;
  }
  
  /* v6.3 Edge Rail: Smaller collapse button */
  .sidebar.collapsed .collapse-btn {
    padding: 10px 0;
  }
  
  .sidebar.collapsed .panel-icon {
    width: 16px;
    height: 16px;
  }
  
  /* v6.3 Edge Rail: Network dropdown positioning for collapsed */
  .sidebar.collapsed .network-dropdown {
    left: calc(100% + 8px);
    right: auto;
    bottom: auto;
    top: 50%;
    transform: translateY(-50%);
    min-width: 140px;
  }
  
  /* Simulator Modal - Hologram v6.1 Wizard Style */
  .sim-modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.85);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    backdrop-filter: blur(8px);
  }
  
  .sim-modal-card {
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-lg);
    width: 90%;
    max-width: 420px;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.6);
    overflow: hidden;
    position: relative;
  }
  
  /* Shimmer border effect */
  .sim-modal-card::before {
    content: '';
    position: absolute;
    inset: -1px;
    border-radius: var(--r-lg);
    padding: 1px;
    background: var(--grad-shimmer);
    background-size: 300% 100%;
    animation: shimmer 4s linear infinite;
    -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
    mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
    -webkit-mask-composite: xor;
    mask-composite: exclude;
    z-index: 0;
    pointer-events: none;
  }
  
  /* Status Bar */
  .sim-modal-status {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-3) var(--s-4);
    border-bottom: 1px solid var(--border-dim);
    background: var(--void-deep);
    position: relative;
    z-index: 1;
  }
  
  .sim-modal-status-left {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .sim-modal-status-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--text-4);
  }
  
  .sim-modal-status-dot.start { background: var(--status-ok); box-shadow: 0 0 8px var(--status-ok); }
  .sim-modal-status-dot.stop { background: var(--status-warn); box-shadow: 0 0 8px var(--status-warn); }
  
  .sim-modal-status-text {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-3);
  }
  
  .sim-modal-badge {
    font-family: var(--font-mono);
    font-size: 9px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    padding: 3px 8px;
    border-radius: 4px;
    border: 1px solid;
  }
  
  .sim-modal-badge.start { color: var(--status-ok); border-color: rgba(52, 211, 153, 0.4); background: rgba(52, 211, 153, 0.08); }
  .sim-modal-badge.stop { color: var(--status-warn); border-color: rgba(251, 191, 36, 0.4); background: rgba(251, 191, 36, 0.08); }
  
  /* Header */
  .sim-modal-header {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    padding: var(--s-5) var(--s-4) var(--s-4);
    position: relative;
    z-index: 1;
  }
  
  .sim-modal-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 56px;
    height: 56px;
    border-radius: 50%;
    border: 1px solid;
    margin-bottom: var(--s-3);
  }
  
  .sim-modal-icon.start {
    color: var(--status-ok);
    background: rgba(52, 211, 153, 0.08);
    border-color: rgba(52, 211, 153, 0.25);
    box-shadow: 0 0 20px rgba(52, 211, 153, 0.15);
  }
  
  .sim-modal-icon.stop {
    color: var(--status-warn);
    background: rgba(251, 191, 36, 0.08);
    border-color: rgba(251, 191, 36, 0.25);
    box-shadow: 0 0 20px rgba(251, 191, 36, 0.15);
  }
  
  .sim-modal-title {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-1);
    margin: 0 0 var(--s-2);
  }
  
  .sim-modal-desc {
    font-size: 13px;
    color: var(--text-3);
    margin: 0;
    line-height: 1.5;
  }
  
  /* Body */
  .sim-modal-body {
    padding: 0 var(--s-4) var(--s-4);
    position: relative;
    z-index: 1;
  }
  
  .sim-modal-features {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    margin-bottom: var(--s-3);
  }
  
  .sim-modal-feature {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 12px;
    color: var(--text-2);
    padding: var(--s-2) var(--s-3);
    background: var(--void-deep);
    border-radius: var(--r-sm);
    border: 1px solid var(--border-subtle);
  }
  
  .sim-modal-feature :global(.sim-feature-icon) {
    color: var(--text-4);
    flex-shrink: 0;
  }
  
  .sim-modal-note {
    padding: var(--s-3);
    border-radius: var(--r-md);
    font-size: 12px;
    line-height: 1.5;
    margin-bottom: var(--s-2);
  }
  
  .sim-modal-note:last-child {
    margin-bottom: 0;
  }
  
  .sim-modal-note.cyan {
    background: rgba(34, 211, 238, 0.08);
    border: 1px solid rgba(34, 211, 238, 0.2);
    color: var(--cyan-400);
  }
  
  .sim-modal-note.warn {
    background: rgba(251, 191, 36, 0.08);
    border: 1px solid rgba(251, 191, 36, 0.2);
    color: var(--status-warn);
  }
  
  /* Actions */
  .sim-modal-actions {
    display: flex;
    gap: var(--s-3);
    padding: var(--s-4);
    border-top: 1px solid var(--border-dim);
    background: var(--void-deep);
    position: relative;
    z-index: 1;
  }
  
  .sim-modal-btn {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-3) var(--s-4);
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    border-radius: var(--r-md);
    cursor: pointer;
    transition: all var(--dur-med) var(--ease-out);
    border: none;
  }
  
  .sim-modal-btn.secondary {
    background: var(--void-up);
    color: var(--text-3);
    border: 1px solid var(--border-subtle);
  }
  
  .sim-modal-btn.secondary:hover {
    background: var(--void-surface);
    color: var(--text-1);
    border-color: var(--border-default);
  }
  
  .sim-modal-btn.primary {
    background: var(--status-ok);
    color: var(--void-pure);
  }
  
  .sim-modal-btn.primary:hover {
    filter: brightness(1.1);
    box-shadow: 0 0 16px rgba(52, 211, 153, 0.4);
    transform: translateY(-1px);
  }
  
  .sim-modal-btn.warn {
    background: var(--status-warn);
    color: var(--void-pure);
  }
  
  .sim-modal-btn.warn:hover {
    filter: brightness(1.1);
    box-shadow: 0 0 16px rgba(251, 191, 36, 0.4);
    transform: translateY(-1px);
  }
  
  :global(.c-violet) {
    color: var(--violet-400);
  }
  
  /* ============================================
     NETWORK SWITCH PROGRESS MODAL
     ============================================ */
  .network-switch-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.85);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2000;
    animation: fadeIn 0.2s ease-out;
  }
  
  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }
  
  .network-switch-modal {
    background: var(--void-mid);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    width: 360px;
    max-width: 90vw;
    box-shadow: 0 24px 64px rgba(0, 0, 0, 0.5);
    overflow: hidden;
    animation: slideUp 0.3s ease-out;
  }
  
  @keyframes slideUp {
    from { 
      opacity: 0;
      transform: translateY(20px) scale(0.95);
    }
    to { 
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }
  
  /* Header */
  .network-switch-header {
    padding: var(--s-5) var(--s-5) var(--s-4);
    text-align: center;
    border-bottom: 1px solid var(--border-dim);
  }
  
  .network-switch-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 52px;
    height: 52px;
    border-radius: 50%;
    border: 1px solid;
    margin-bottom: var(--s-3);
  }
  
  .network-switch-icon.simulator {
    color: var(--status-err);
    background: rgba(248, 113, 113, 0.08);
    border-color: rgba(248, 113, 113, 0.25);
    box-shadow: 0 0 20px rgba(248, 113, 113, 0.15);
  }
  
  .network-switch-icon.mainnet {
    color: var(--status-ok);
    background: rgba(52, 211, 153, 0.08);
    border-color: rgba(52, 211, 153, 0.25);
    box-shadow: 0 0 20px rgba(52, 211, 153, 0.15);
  }
  
  .network-switch-title {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: var(--text-1);
    margin: 0 0 var(--s-2);
  }
  
  .network-switch-subtitle {
    font-size: 12px;
    color: var(--text-3);
    margin: 0;
  }
  
  /* Steps Checklist */
  .network-switch-steps {
    padding: var(--s-4) var(--s-5);
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .network-switch-step {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-2) 0;
    transition: opacity 0.2s ease;
  }
  
  .network-switch-step.pending {
    opacity: 0.4;
  }
  
  .network-switch-step.complete {
    opacity: 0.7;
  }
  
  .network-switch-step.in-progress {
    opacity: 1;
  }
  
  .step-icon {
    width: 20px;
    height: 20px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }
  
  .network-switch-step.complete .step-icon {
    color: var(--status-ok);
  }
  
  .network-switch-step.in-progress .step-icon {
    color: var(--cyan-400);
  }
  
  .network-switch-step.pending .step-icon {
    color: var(--text-4);
  }
  
  .step-circle {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    border: 1.5px solid currentColor;
  }
  
  .step-spinner {
    width: 14px;
    height: 14px;
    border: 2px solid transparent;
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  .step-label {
    font-size: 12px;
    font-family: var(--font-mono);
    color: var(--text-2);
  }
  
  .network-switch-step.in-progress .step-label {
    color: var(--text-1);
  }
  
  .network-switch-step.complete .step-label {
    color: var(--text-3);
  }
  
  /* Progress Bar */
  .network-switch-progress {
    padding: var(--s-3) var(--s-5) var(--s-4);
    border-top: 1px solid var(--border-dim);
    background: var(--void-deep);
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .progress-bar-track {
    height: 4px;
    background: var(--void-up);
    border-radius: 2px;
    overflow: hidden;
  }
  
  .progress-bar-fill {
    height: 100%;
    border-radius: 2px;
    transition: width 0.3s ease-out;
  }
  
  .progress-bar-fill.simulator {
    background: linear-gradient(90deg, var(--status-err), var(--pink-400));
  }
  
  .progress-text {
    font-size: 10px;
    font-family: var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--text-4);
    text-align: center;
  }
  
  /* Simple spinner view for mainnet switch */
  .network-switch-simple {
    padding: var(--s-5);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-4);
  }
  
  .simple-spinner {
    width: 40px;
    height: 40px;
    border: 3px solid var(--void-up);
    border-top-color: var(--status-ok);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  
  .simple-message {
    font-size: 12px;
    color: var(--text-3);
    text-align: center;
    line-height: 1.5;
    margin: 0;
  }
</style>
