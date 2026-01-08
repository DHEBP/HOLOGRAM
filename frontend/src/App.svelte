<script>
  import { onMount } from 'svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import FirstRunWizard from './lib/components/FirstRunWizard.svelte';
  import SplashScreen from './lib/components/SplashScreen.svelte';
  import WalletModal from './lib/components/WalletModal.svelte';
  import Toast from './lib/components/Toast.svelte';
  import Browser from './routes/Browser.svelte';
  import Studio from './routes/Studio.svelte';
  import Explorer from './routes/Explorer.svelte';
  import Wallet from './routes/Wallet.svelte';
  import Settings from './routes/Settings.svelte';
  // Mining tab removed - Developer Support now in Settings > Developer Support
  // Network tab removed - node controls moved to Settings > Node
  import { appState, settingsState, updateStatus, addExternalRequest, toast, loadSettings } from './lib/stores/appState.js';
  import { GetSetting, RespondToXSWDRequest, RespondToXSWDRequestWithPermissions } from '../wailsjs/go/main/App.js';
  import { EventsOn } from '../wailsjs/runtime/runtime.js';
  import { waitForWails } from './lib/utils/wails.js';
  
  // Module-level interval ID to prevent duplicates during HMR
  let statusPollingInterval = null;
  
  let currentTab = 'explorer'; // Default to explorer (landing page)
  let sidebarCollapsed = false;
  let showWizard = false;
  let wizardChecked = false;
  
  // Pending search result to pass to Explorer/Browser after navigation
  let pendingSearchResult = null;
  
  // Section navigation state for Settings
  let pendingSection = null;
  
  const tabs = [
    { id: 'explorer', label: 'Explorer', icon: 'search' },
    { id: 'browser', label: 'Browser', icon: 'globe' },
    { id: 'wallet', label: 'Wallet', icon: 'wallet' },
    { id: 'studio', label: 'Studio', icon: 'palette' },
    { id: 'settings', label: 'Settings', icon: 'settings' },
  ];

  function handleTabChange(tabId) {
    currentTab = tabId;
  }
  
  function handleWizardComplete() {
    showWizard = false;
  }

  onMount(async () => {
    console.log('Hologram initializing...');
    
    // Fix for Wails/WebView scroll focus issue on macOS
    // When the app loses and regains focus, scroll events may not work until
    // the webview is explicitly focused. This handler restores scroll functionality.
    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        // Small delay to ensure window is fully focused
        setTimeout(() => {
          // Focus the document body to restore scroll event handling
          document.body.focus();
          // Also trigger a reflow to ensure scroll containers are responsive
          document.body.style.pointerEvents = 'none';
          requestAnimationFrame(() => {
            document.body.style.pointerEvents = '';
          });
        }, 50);
      }
    };
    document.addEventListener('visibilitychange', handleVisibilityChange);
    
    // Also handle window focus events directly
    const handleWindowFocus = () => {
      // Ensure scroll containers are responsive after window regains focus
      setTimeout(() => {
        document.body.focus();
      }, 50);
    };
    window.addEventListener('focus', handleWindowFocus);
    
    // Minimum splash duration (allows animation to complete)
    const splashMinTime = new Promise(resolve => setTimeout(resolve, 3500));
    
    // Wait for Wails Go bindings to be available before any backend calls
    try {
      await waitForWails();
    } catch (err) {
      console.error('Wails runtime failed to initialize:', err);
    }
    
    // Load settings from backend on app startup
    await loadSettings();
    
    // Check if wizard has been completed
    try {
      const wizardComplete = await GetSetting('wizard_complete');
      showWizard = !wizardComplete || wizardComplete === 'false';
    } catch (e) {
      // First run - show wizard
      showWizard = true;
    }
    
    // Wait for minimum splash time before proceeding
    await splashMinTime;
    wizardChecked = true;
    
    // Initial status fetch
    updateStatus();
    
    // Listen for status updates from backend (replaces polling)
    EventsOn("status:update", (status) => {
      // Update app state from broadcasted status
      if (status.node) {
        appState.update(state => ({
          ...state,
          nodeConnected: status.node.connected,
          chainHeight: status.node.chainHeight,
          topoHeight: status.node.topoHeight,
          hashrate: status.node.hashrate,
          difficulty: status.node.difficulty,
          txPoolSize: status.node.txPoolSize,
          peerCount: status.node.peerCount,
          nodeVersion: status.node.version,
          network: status.node.network,
        }));
      }
      if (status.xswd) {
        appState.update(state => ({
          ...state,
          xswdConnected: status.xswd.connected,
          xswdServerRunning: status.xswd.serverRunning || false,
          engramConnected: status.xswd.engramConnected || false,
        }));
      }
      if (status.gnomon) {
        appState.update(state => ({
          ...state,
          gnomonRunning: status.gnomon.running,
          gnomonIndexedHeight: status.gnomon.indexed_height,
          gnomonChainHeight: status.gnomon.chain_height,
          gnomonProgress: status.gnomon.progress,
        }));
      }
      if (status.wallet) {
        appState.update(state => ({
          ...state,
          walletOpen: status.wallet.open,
          walletAddress: status.wallet.address || '',
          walletBalance: status.wallet.balance || 0,
        }));
      }
    });
    
    // Fallback polling in case events don't fire (e.g., during reconnection)
    // Clear any existing interval first (prevents duplicates during HMR)
    if (statusPollingInterval) {
      clearInterval(statusPollingInterval);
    }
    statusPollingInterval = setInterval(updateStatus, 30000); // Reduced frequency as backup
    
    // Listen for tab switch events from child components
    const handleTabSwitch = (e) => {
      if (e.detail && tabs.find(t => t.id === e.detail)) {
        currentTab = e.detail;
      }
    };
    window.addEventListener('switch-tab', handleTabSwitch);
    
    // Global keyboard shortcuts
    const handleGlobalKeydown = (e) => {
      // Cmd+K (Mac) or Ctrl+K (Windows/Linux) to focus search
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        // Switch to explorer tab if not already there
        currentTab = 'explorer';
        // Dispatch event to focus the search input
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('focus-search'));
        }, 50);
      }
      
      // Cmd+1-7 / Ctrl+1-7 to switch tabs
      if ((e.metaKey || e.ctrlKey) && e.key >= '1' && e.key <= '7') {
        e.preventDefault();
        const tabIndex = parseInt(e.key) - 1;
        if (tabs[tabIndex]) {
          currentTab = tabs[tabIndex].id;
        }
      }
      
      // Escape to blur active input (handled by individual components)
    };
    window.addEventListener('keydown', handleGlobalKeydown);
    
    // Listen for search navigation events (from Search landing page)
    const handleSearchNavigate = (e) => {
      const { tab, type, query, result } = e.detail;
      if (tab && tabs.find(t => t.id === tab)) {
        // Store the search result to pass to the target component
        pendingSearchResult = { type, query, result };
        currentTab = tab;
        
        // Dispatch event to the target component after a short delay
        setTimeout(() => {
          window.dispatchEvent(new CustomEvent('search-result', { detail: pendingSearchResult }));
          pendingSearchResult = null;
        }, 100);
      }
    };
    window.addEventListener('search-navigate', handleSearchNavigate);
    
    // Listen for status indicator clicks from Sidebar
    const handleStatusClick = (e) => {
      const { tab, section } = e.detail;
      if (tab && tabs.find(t => t.id === tab)) {
        currentTab = tab;
        if (section) {
          // Store section to pass to component
          pendingSection = section;
          // Dispatch event after component mounts
          setTimeout(() => {
            window.dispatchEvent(new CustomEvent('navigate-section', { detail: { section } }));
            pendingSection = null;
          }, 100);
        }
      }
    };
    window.addEventListener('status-click', handleStatusClick);
    
    // Listen for wallet menu open requests
    const handleOpenWalletMenu = () => {
      // Dispatch event for StatusBar to handle
      window.dispatchEvent(new CustomEvent('statusbar-open-wallet-menu'));
    };
    window.addEventListener('open-wallet-menu', handleOpenWalletMenu);
    
    // Listen for toast notifications from backend
    EventsOn("toast:show", (data) => {
      const type = data.type || 'info';
      const message = data.message || 'Notification';
      switch (type) {
        case 'success': toast.success(message); break;
        case 'warning': toast.warning(message); break;
        case 'error': toast.error(message); break;
        default: toast.info(message);
      }
    });
    
    // Listen for XSWD requests
    EventsOn("xswd:request", (req) => {
      console.log('Received XSWD request:', req);
      const requestType = req.type || (req.method ? 'sign' : 'connect');
      const appName = req.appName || 'External dApp';
      
      const payload = requestType === 'connect'
        ? {
            appName: appName,
            description: req.description,
            origin: req.origin || 'XSWD',
          }
        : {
            transfers: req.params?.transfers,
            sc_data: req.params?.sc_rpc || req.params?.sc_data,
            scid: req.params?.scid,
            entrypoint: req.params?.entrypoint,
          };

      // Show toast notification for incoming request
      const toastMessage = requestType === 'connect'
        ? `${appName} wants to connect`
        : `${appName} requests wallet action`;
      toast.info(toastMessage, 4000);

      addExternalRequest({
        id: req.id,
        type: requestType,
        payload,
        appName: appName,
        origin: req.origin || 'XSWD',
        // Include permission info for connect requests
        requestedPermissions: req.requestedPermissions,
        existingPermissions: req.existingPermissions,
        isReadOnly: req.isReadOnly || false
      }, 
      // On Approve
      (result) => {
        console.log('Approving XSWD request:', req.id, 'permissions:', result.permissions);
        // Use new function with permissions for connect requests
        if (requestType === 'connect' && result.permissions) {
          RespondToXSWDRequestWithPermissions(req.id, true, result.password || "", result.permissions);
        } else {
          RespondToXSWDRequest(req.id, true, result.password || "");
        }
        toast.success(`Request approved for ${appName}`, 3000);
      },
      // On Deny
      () => {
        console.log('Denying XSWD request:', req.id);
        RespondToXSWDRequest(req.id, false, "");
        toast.warning(`Request denied for ${appName}`, 3000);
      });
    });
    
    return () => {
      if (statusPollingInterval) {
        clearInterval(statusPollingInterval);
        statusPollingInterval = null;
      }
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('focus', handleWindowFocus);
      window.removeEventListener('switch-tab', handleTabSwitch);
      window.removeEventListener('search-navigate', handleSearchNavigate);
      window.removeEventListener('keydown', handleGlobalKeydown);
      window.removeEventListener('status-click', handleStatusClick);
      window.removeEventListener('open-wallet-menu', handleOpenWalletMenu);
    };
  });
</script>

<!-- Splash Screen (shows while initializing) -->
<SplashScreen show={!wizardChecked} />

<!-- First Run Wizard -->
{#if wizardChecked && showWizard}
  <FirstRunWizard on:complete={handleWizardComplete} />
{/if}

<!-- Integrated Wallet Modal -->
<WalletModal />

<!-- Toast Notifications -->
<Toast />

<!-- v6.1 Noise Overlay (subtle film grain) -->
<div class="noise-overlay"></div>

<!-- SVG Definitions for gradients -->
<svg width="0" height="0" style="position: absolute;">
  <defs>
    <linearGradient id="areaGrad" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color: #22d3ee; stop-opacity: 0.6" />
      <stop offset="100%" style="stop-color: #22d3ee; stop-opacity: 0" />
    </linearGradient>
    <linearGradient id="ringGrad" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" style="stop-color: #22d3ee" />
      <stop offset="100%" style="stop-color: #a78bfa" />
    </linearGradient>
  </defs>
</svg>

<div class="app-shell">
  <!-- Sidebar -->
  <Sidebar 
    {tabs} 
    {currentTab} 
    collapsed={sidebarCollapsed}
    on:tabChange={(e) => handleTabChange(e.detail)}
    on:toggleCollapse={() => sidebarCollapsed = !sidebarCollapsed}
    on:statusClick={(e) => {
      const event = new CustomEvent('status-click', { detail: e.detail });
      window.dispatchEvent(event);
    }}
  />

  <!-- Main Content Area -->
  <div class="app-main">
    <!-- Content -->
    <main class="app-content">
      {#key currentTab}
      {#if currentTab === 'browser'}
        <Browser />
      {:else if currentTab === 'wallet'}
        <Wallet />
      {:else if currentTab === 'explorer'}
        <Explorer />
      {:else if currentTab === 'studio'}
        <Studio />
      {:else if currentTab === 'settings'}
        <Settings key={pendingSection || 'settings'} />
      {/if}
      {/key}
    </main>
  </div>
</div>
