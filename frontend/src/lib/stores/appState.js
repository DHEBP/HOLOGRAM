import { writable, derived, get } from 'svelte/store';

// Import Wails bindings
import { GetXSWDStatus, GetNetworkInfo, GetGnomonStatus, GetNetworkMode, GetNodeStatus, GetAllSettings, SetSetting } from '../../../wailsjs/go/main/App.js';

// ==================== Settings Key Mapping ====================
// Maps backend snake_case keys to frontend camelCase keys
const settingsKeyMap = {
  // Backend key → Frontend key
  'min_rating': 'minRating',
  'block_malware': 'blockMalware',
  'show_nsfw': 'showNSFW',
  'auto_connect_ws': 'autoConnectXSWD',
  'gnomon_enabled': 'gnomonEnabled',
  'daemon_endpoint': 'daemonEndpoint',
  'network': 'network',
  'integrated_wallet': 'integratedWallet',
  'cypherpunk_mode': 'cypherpunkMode',
  'last_wallet_path': 'lastWalletPath',
  // FirstRunWizard settings
  'use_embedded_node': 'useEmbeddedNode',
  'node_data_dir': 'nodeDataDir',
  'wizard_complete': 'wizardComplete',
  // Developer Support (EPOCH)
  'dev_support_enabled': 'epochEnabled',
  'epoch_enabled': 'epochEnabled',
};

// Reverse map for saving (frontend → backend)
const settingsKeyMapReverse = Object.fromEntries(
  Object.entries(settingsKeyMap).map(([k, v]) => [v, k])
);

// Browser Navigation History (Global to persist across tab switches)
export const browserHistory = writable([]);
export const browserHistoryIndex = writable(-1);

export function pushToHistory(url) {
  // Get current state
  const currentHistory = get(browserHistory);
  const currentIndex = get(browserHistoryIndex);
  
  // Truncate forward history if we're navigating from middle
  let newHistory = currentHistory;
  if (currentIndex < currentHistory.length - 1) {
    newHistory = currentHistory.slice(0, currentIndex + 1);
  }
  
  // Add new entry
  newHistory = [...newHistory, url];
  
  // Limit history size (keep last 100)
  if (newHistory.length > 100) {
    newHistory.shift();
  }
  
  // Update stores
  browserHistory.set(newHistory);
  browserHistoryIndex.set(newHistory.length - 1);
  
  console.log('📜 Global History updated:', { length: newHistory.length, index: newHistory.length - 1 });
}

// ==================== Wallet Request History ====================
// Persisted log of all wallet approval/denial events (defined early so it can be used by queue functions)
export const walletRequestHistory = writable(loadRequestHistoryFromStorage());

// Load history from localStorage
function loadRequestHistoryFromStorage() {
  try {
    if (typeof localStorage !== 'undefined') {
      const stored = localStorage.getItem('walletRequestHistory');
      if (stored) {
        return JSON.parse(stored);
      }
    }
  } catch (e) {
    console.error('Failed to load request history:', e);
  }
  return [];
}

// Save history to localStorage
function saveRequestHistory(history) {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem('walletRequestHistory', JSON.stringify(history.slice(-50))); // Keep last 50
    }
  } catch (e) {
    console.error('Failed to save request history:', e);
  }
}

// Log a wallet request to history
export function logWalletRequest(request, status, txid = null) {
  const entry = {
    id: request.id || Date.now().toString(36),
    timestamp: Date.now(),
    type: request.type || 'unknown',
    appName: request.appName || 'Unknown App',
    origin: request.origin || 'Local',
    status: status, // 'approved' | 'denied'
    txid: txid,
    payload: request.payload ? {
      // Sanitize payload - don't store sensitive data
      method: request.payload.method,
      transfers: request.payload.transfers?.map(t => ({
        destination: t.destination?.slice(0, 20) + '...',
        amount: t.amount
      })),
      scid: request.payload.scid,
    } : null,
  };

  walletRequestHistory.update(history => {
    const newHistory = [...history, entry].slice(-50); // Keep last 50
    saveRequestHistory(newHistory);
    return newHistory;
  });

  return entry;
}

// Clear request history
export function clearRequestHistory() {
  walletRequestHistory.set([]);
  if (typeof localStorage !== 'undefined') {
    localStorage.removeItem('walletRequestHistory');
  }
}

// ==================== Wallet Request Queue ====================
export const walletRequests = writable([]);
export const activeWalletRequest = derived(walletRequests, $requests => $requests.length > 0 ? $requests[0] : null);

export function requestWalletApproval(request) {
  return new Promise((resolve, reject) => {
    const id = Date.now().toString(36) + Math.random().toString(36).substr(2);
    const fullRequest = {
      id,
      timestamp: Date.now(),
      ...request,
      resolve,
      reject
    };
    
    walletRequests.update(reqs => [...reqs, fullRequest]);
  });
}

export function addExternalRequest(request, onApprove, onDeny) {
  const fullRequest = {
    ...request,
    timestamp: Date.now(),
    resolve: onApprove,
    reject: onDeny
  };
  walletRequests.update(reqs => [...reqs, fullRequest]);
}

export async function approveWalletRequest(id, password, txid = null, permissions = null) {
  // Find the request
  const requests = get(walletRequests);
  const request = requests.find(r => r.id === id);
  
  if (request) {
    // Log to history
    logWalletRequest(request, 'approved', txid);
    
    // We resolve with the password and permissions so the caller can use them
    request.resolve({ approved: true, password, permissions });
    
    // Remove from queue
    walletRequests.update(reqs => reqs.filter(r => r.id !== id));
  }
}

export function denyWalletRequest(id) {
  const requests = get(walletRequests);
  const request = requests.find(r => r.id === id);
  
  if (request) {
    // Log to history
    logWalletRequest(request, 'denied');
    
    request.reject(new Error('User denied request'));
    walletRequests.update(reqs => reqs.filter(r => r.id !== id));
  }
}

// Core application state
export const appState = writable({
  xswdConnected: false,       // Legacy: any XSWD activity (server running OR engram connected)
  xswdServerRunning: false,   // HOLOGRAM's XSWD server is running (can accept dApp connections)
  engramConnected: false,     // Connected to Engram wallet as XSWD client
  nodeConnected: false,
  gnomonRunning: false,
  gnomonProgress: 0,
  gnomonIndexedHeight: 0,
  chainHeight: 0,
  networkInfo: null,
  network: 'mainnet', // Single source of truth for current network
  currentNetworkMode: null, // Full network mode object from backend
  currentEndpoint: 'http://127.0.0.1:10102', // Actual current daemon endpoint
  currentSCID: null,
  history: [],
  historyIndex: -1,
  bookmarks: [],
});

// Wallet state
export const walletState = writable({
  isOpen: false,
  address: '',
  balance: 0,
  lockedBalance: 0,
  walletPath: '',
  recentWallets: [],
});

// Settings state
export const settingsState = writable({
  minRating: 60,
  blockMalware: true,
  showNSFW: false,
  autoConnectXSWD: true,
  gnomonEnabled: false,
  network: 'mainnet',
  daemonEndpoint: 'http://127.0.0.1:10102',
  cypherpunkMode: false,
  integratedWallet: true,
  lastWalletPath: '', // Store the last used wallet path for quick connection
});

// Load settings from backend and sync to frontend store
export async function loadSettings() {
  try {
    const backendSettings = await GetAllSettings();
    if (!backendSettings) {
      console.warn('No settings returned from backend');
      return;
    }

    // Map backend keys to frontend keys
    const mappedSettings = {};
    for (const [backendKey, value] of Object.entries(backendSettings)) {
      const frontendKey = settingsKeyMap[backendKey];
      if (frontendKey) {
        mappedSettings[frontendKey] = value;
      }
    }

    // Update settings state with mapped values
    settingsState.update(state => ({
      ...state,
      ...mappedSettings,
    }));

    console.log('✅ Settings loaded from backend:', mappedSettings);
  } catch (error) {
    console.error('❌ Failed to load settings:', error);
  }
}

// Save a setting to backend using the correct backend key
export async function saveSetting(frontendKey, value) {
  const backendKey = settingsKeyMapReverse[frontendKey] || frontendKey;
  
  // Update frontend state immediately
  settingsState.update(state => ({ ...state, [frontendKey]: value }));
  
  // Save to backend with the correct key
  try {
    await SetSetting(JSON.stringify({ [backendKey]: value }));
    console.log(`⚙️ Setting saved: ${backendKey} = ${value}`);
  } catch (error) {
    console.error(`Failed to save setting ${backendKey}:`, error);
  }
}

// Console logs
export const consoleLogs = writable([]);

// Pending navigation (used when switching from Discover to Browser)
export const pendingNavigation = writable(null);

// Set pending navigation target
export function navigateTo(url, app = null) {
  pendingNavigation.set({ url, app, timestamp: Date.now() });
}

// Clear pending navigation
export function clearPendingNavigation() {
  pendingNavigation.set(null);
}

// Derived stores
export const isConnected = derived(
  appState,
  ($appState) => $appState.xswdConnected || $appState.nodeConnected
);

export const syncProgress = derived(
  appState,
  ($appState) => {
    if ($appState.chainHeight === 0) return 0;
    return ($appState.gnomonIndexedHeight / $appState.chainHeight) * 100;
  }
);

// Actions
export async function updateStatus() {
  try {
    // Check XSWD status
    const xswdStatus = await GetXSWDStatus();
    
    // Check network info
    const networkInfo = await GetNetworkInfo();
    
    // Check Gnomon status
    const gnomonStatus = await GetGnomonStatus();
    
    appState.update(state => ({
      ...state,
      xswdConnected: xswdStatus?.connected || false,
      xswdServerRunning: xswdStatus?.serverRunning || false,
      engramConnected: xswdStatus?.engramConnected || false,
      nodeConnected: networkInfo?.success || false,
      networkInfo: networkInfo?.info || null,
      chainHeight: networkInfo?.info?.height || 0,
      gnomonRunning: gnomonStatus?.status?.running || false,
      gnomonProgress: gnomonStatus?.status?.progress || 0,
      gnomonIndexedHeight: gnomonStatus?.status?.indexed_height || 0,
    }));
  } catch (error) {
    console.error('Status update error:', error);
  }
}

// Sync network mode from backend to frontend stores
// This ensures all network indicators stay in sync across the app
export async function syncNetworkMode() {
  try {
    const networkMode = await GetNetworkMode();
    const nodeStatus = await GetNodeStatus();
    
    if (networkMode && networkMode.network) {
      const network = networkMode.network;
      // Get endpoint from networkMode or construct from rpcPort
      let endpoint = networkMode.endpoint;
      if (!endpoint && networkMode.rpcPort) {
        endpoint = `http://127.0.0.1:${networkMode.rpcPort}`;
      } else if (!endpoint) {
        // Default ports based on network
        const defaultPorts = {
          mainnet: 10102,
          testnet: 40402,
          simulator: 20000,
        };
        endpoint = `http://127.0.0.1:${defaultPorts[network] || 10102}`;
      }
      
      // Update appState (single source of truth)
      appState.update(state => ({
        ...state,
        network: network,
        currentNetworkMode: networkMode,
        currentEndpoint: endpoint,
      }));
      
      // Also update settingsState for Settings page compatibility
      settingsState.update(state => ({
        ...state,
        network: network,
        daemonEndpoint: endpoint,
      }));
      
      console.log('✅ Network mode synced:', { network, endpoint });
    }
  } catch (error) {
    console.error('❌ Failed to sync network mode:', error);
  }
}

export function addToHistory(scid) {
  appState.update(state => {
    // Remove any forward history
    const newHistory = state.history.slice(0, state.historyIndex + 1);
    newHistory.push(scid);
    
    // Limit history size
    if (newHistory.length > 100) {
      newHistory.shift();
    }
    
    return {
      ...state,
      history: newHistory,
      historyIndex: newHistory.length - 1,
      currentSCID: scid,
    };
  });
}

export function addConsoleLog(message, level = 'info') {
  const timestamp = new Date().toLocaleTimeString();
  consoleLogs.update(logs => [
    ...logs.slice(-499), // Keep last 500 logs
    { timestamp, message, level }
  ]);
}

export function clearConsoleLogs() {
  consoleLogs.set([]);
}

// ==================== Toast Notifications ====================
// Transient UI notifications
export const toastNotifications = writable([]);

let toastIdCounter = 0;

// Show a toast notification
export function showToast(message, type = 'info', duration = 5000) {
  const id = ++toastIdCounter;
  const toast = {
    id,
    message,
    type, // 'info' | 'success' | 'warning' | 'error'
    timestamp: Date.now(),
  };

  toastNotifications.update(toasts => [...toasts, toast]);

  // Auto-dismiss after duration
  if (duration > 0) {
    setTimeout(() => {
      dismissToast(id);
    }, duration);
  }

  return id;
}

// Dismiss a specific toast
export function dismissToast(id) {
  toastNotifications.update(toasts => toasts.filter(t => t.id !== id));
}

// Dismiss all toasts
export function dismissAllToasts() {
  toastNotifications.set([]);
}

// Convenience functions for different toast types
export const toast = {
  info: (message, duration) => showToast(message, 'info', duration),
  success: (message, duration) => showToast(message, 'success', duration),
  warning: (message, duration) => showToast(message, 'warning', duration),
  error: (message, duration) => showToast(message, 'error', duration),
};

// ==================== Backend Error Handling ====================
// Standardized handler for backend responses with friendly + technical errors
// Usage: const error = handleBackendError(result) or handleBackendError(result, { showToast: false })

/**
 * Handle a backend response that may contain an error
 * @param {Object} result - Backend response { success, error, technicalError }
 * @param {Object} options - Configuration options
 * @param {boolean} options.showToast - Whether to show toast notification (default: true)
 * @param {number} options.duration - Toast duration in ms (default: 5000)
 * @param {boolean} options.logTechnical - Whether to log technical error (default: true)
 * @returns {string|null} - The friendly error message, or null if no error
 */
export function handleBackendError(result, options = {}) {
  const { showToast: shouldShowToast = true, duration = 5000, logTechnical = true } = options;
  
  if (!result || result.success) {
    return null;
  }
  
  const friendlyError = result.error || 'An unexpected error occurred';
  const technicalError = result.technicalError;
  
  // Log technical error for debugging
  if (logTechnical && technicalError) {
    console.error('[Backend Error]', technicalError);
  }
  
  // Show toast notification
  if (shouldShowToast && friendlyError) {
    toast.error(friendlyError, duration);
  }
  
  return friendlyError;
}

/**
 * Wrap an async backend call with standardized error handling
 * @param {Function} fn - Async function that returns a backend response
 * @param {Object} options - Error handling options (same as handleBackendError)
 * @returns {Promise<Object>} - The result, with error field populated if failed
 */
export async function withErrorHandling(fn, options = {}) {
  try {
    const result = await fn();
    handleBackendError(result, options);
    return result;
  } catch (e) {
    const error = e.message || 'An unexpected error occurred';
    console.error('[Backend Exception]', e);
    if (options.showToast !== false) {
      toast.error(error);
    }
    return { success: false, error };
  }
}

