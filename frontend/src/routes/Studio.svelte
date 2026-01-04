<script>
  import { onMount, onDestroy } from 'svelte';
  import { walletState, settingsState, navigateTo, syncNetworkMode, toast } from '../lib/stores/appState.js';
  import DropZone from '../lib/components/DropZone.svelte';
  import BatchUpload from '../lib/components/BatchUpload.svelte';
  import DiffViewer from '../lib/components/DiffViewer.svelte';
  import ModPickerModal from '../lib/components/ModPickerModal.svelte';
  import VersionHistory from '../lib/components/VersionHistory.svelte';
  import { 
    SetSetting, GetGasEstimate, InstallDOC, InstallINDEX, GetINDEXInfo, UpdateINDEX, SelectFolder, SelectFile,
    IsInSimulatorMode, GetSimulatorDeploymentInfo, CloneTELA, GetClonePath,
    StartLocalDevServer, StopLocalDevServer, GetLocalDevServerStatus, RefreshLocalDevServer,
    StartSimulatorMode, StopSimulatorMode, GetSimulatorStatus, SetNetworkMode,
    ShardFile, ConstructFromShards, InstallSmartContract
  } from '../../wailsjs/go/main/App.js';
  import { BrowserOpenURL, ClipboardSetText } from '../../wailsjs/runtime/runtime.js';
  import { EventsOn, EventsOff, OnFileDrop, OnFileDropOff } from '../../wailsjs/runtime/runtime.js';
  import { 
    Globe, FlaskConical, Gamepad2, FileText, FolderUp, FolderDown, Layers, RefreshCw, 
    Package, Copy, Server, GitCompare, AlertTriangle, X, Plus, Loader2, Lock, Eye, Square,
    Puzzle, Library, Palette, Zap, Database, Shield, Wrench, Search, ArrowRight, Check,
    Radio, Wallet, Diamond, ExternalLink, CheckCircle, Clipboard, FileArchive,
    Link, Lightbulb, ThumbsUp, ThumbsDown, Minus, GitBranch, History, RotateCcw,
    Scissors, FolderOpen, FileCode, Info
  } from 'lucide-svelte';
  import { GetMODsList, GetMODInfo, GetAllMODClasses, PrepareMODInstall, GetTELALibraries, EnsureGnomonRunning, SearchMyContent, SearchMyDOCs, SearchMyINDEXes, GetAvailableDOCTypes, GetCommitHistory, GetCommitContent, DiffCommits } from '../../wailsjs/go/main/App.js';
  
  // Diff viewer state
  let showDiffViewer = false;
  
  // Copy feedback state
  let copiedScid = null;
  
  let activeTab = 'install-doc';
  let stagedFiles = [];
  let deploymentStatus = null;
  let batchFolderPath = '';
  let batchDragging = false;
  let totalGasEstimate = 0;
  let gasLoading = false;
  
  // INDEX base cost estimate (approximately)
  const INDEX_BASE_GAS = 10000;
  
  // =====================================================
  // Install INDEX state (matching tela-cli install-index)
  // =====================================================
  let indexName = '';
  let indexDURL = '';
  let indexDescription = '';
  let indexIconURL = '';
  let indexDocScids = [];      // Array of DOC SCIDs to include
  let newIndexDocScid = '';    // Input for adding new DOC SCID
  let indexRingsize = 2;       // 2 = updateable, 16+ = immutable
  let indexInstalling = false;
  let indexInstallResult = null;
  let indexInstallError = '';
  
  // MODs state for Install INDEX (matching tela-cli modsPrompt)
  let indexEnableMods = false;        // Toggle to enable MODs
  let indexSelectedVsMod = '';        // Variable Store MOD (single selection)
  let indexSelectedTxMods = [];       // Transfer MODs (multi-selection)
  let showModPickerModal = false;     // Modal for advanced MOD selection
  // Note: Uses allMods and modsLoading from MODULES section below
  
  // =====================================================
  // Install DOC state (matching tela-cli install-doc)
  // =====================================================
  let docDURL = '';            // Optional dURL for the DOC
  let docDescription = '';     // Description header
  let docIconURL = '';         // Icon URL header
  let docRingsize = 2;         // 2 = updateable, 16+ = immutable
  let docCompression = false;  // Whether to compress the file
  
  // Confirmation modal state
  let showConfirmModal = false;
  let confirmModalType = '';   // 'doc' or 'index'
  let confirmModalData = null;
  let deployAcknowledged = false; // User must check acknowledgement box
  
  // Update INDEX state
  let updateIndexScid = '';
  let updateIndexLoading = false;
  let updateIndexInfo = null;
  let updateIndexError = '';
  let updateIndexDocs = [];
  let newDocScid = '';
  let updateInProgress = false;
  let updateResult = null;
  
  // Local Dev Server state
  let localServerRunning = false;
  let localServerUrl = '';
  let localServerDirectory = '';
  let localServerPort = 0;
  let localServerWatcherActive = false;
  let serveError = '';
  let serveLoading = false;
  let recentChanges = [];
  
  // =====================================================
  // Clone state
  // =====================================================
  let cloneScid = '';
  let cloneLoading = false;
  let cloneResult = null;
  let cloneError = '';
  let showCloneConfirmModal = false;  // For "content updated" confirmation
  
  // =====================================================
  // My Content state (matching tela-cli search my docs/indexes)
  // =====================================================
  let myContentLoading = false;
  let myContentError = '';
  let myContentGnomonRequired = false;  // True when Gnomon needs to be started
  let myDocs = [];
  let myIndexes = [];
  let myContentTab = 'all';  // 'all', 'docs', 'indexes'
  let myContentDocTypeFilter = '';  // Filter DOCs by type
  let availableDocTypes = [];
  let myContentLoaded = false;
  
  // =====================================================
  // Version History / Actions state (Git-like version control)
  // =====================================================
  let showVersionHistory = false;
  let versionHistoryScid = '';
  let actionsScid = '';           // SCID input for Actions page
  let actionsLoading = false;
  let actionsContentInfo = null;  // Info about the loaded content
  let actionsError = '';

  // =====================================================
  // DocShards state (inline - matching Clone/Serve pattern)
  // =====================================================
  let shardMode = 'shard';        // 'shard' or 'reconstruct'
  let shardFilePath = '';         // File to shard
  let shardFolderPath = '';       // Folder containing shards to reconstruct
  let shardCompress = true;       // Enable GZIP compression
  let shardLoading = false;
  let shardResult = null;
  let shardError = '';

  // =====================================================
  // Deploy SC state (raw smart contract deployment)
  // =====================================================
  let scCode = '';                // DVM-BASIC smart contract code
  let scAnonymous = false;        // Use ringsize 16+ for anonymous deployment
  let scDeploying = false;
  let scDeployResult = null;
  let scDeployError = '';

  // Dropzone element reference for native drag-and-drop
  let batchDropzoneElement;
  
  // Check local server status on mount
  onMount(async () => {
    await checkLocalServerStatus();
    
    // Listen for file change events (hot reload)
    EventsOn('localdev:reload', handleFileChange);
    
    // Set up Wails native drag-and-drop handler for REAL filesystem paths
    // (Browser drag-and-drop API only provides virtual paths for security)
    OnFileDrop((x, y, paths) => {
      // Only handle if we're on the batch-upload tab and no folder is selected yet
      if (activeTab !== 'batch-upload') {
        return;
      }
      if (batchFolderPath) {
        return;
      }
      
      // Check if drop is within the dropzone element bounds
      if (batchDropzoneElement) {
        const rect = batchDropzoneElement.getBoundingClientRect();
        if (x >= rect.left && x <= rect.right && y >= rect.top && y <= rect.bottom) {
          // Use the first dropped path (should be a folder)
          if (paths && paths.length > 0) {
            batchFolderPath = paths[0];
            batchDragging = false;
          }
        }
      }
    }, true); // useDropTarget = true
  });
  
  onDestroy(() => {
    EventsOff('localdev:reload');
    OnFileDropOff();
  });
  
  async function checkLocalServerStatus() {
    try {
      const status = await GetLocalDevServerStatus();
      localServerRunning = status.running || false;
      localServerUrl = status.url || '';
      localServerDirectory = status.directory || '';
      localServerPort = status.port || 0;
      localServerWatcherActive = status.watcherActive || false;
    } catch (e) {
      console.error('Failed to get local server status:', e);
    }
  }
  
  async function selectAndServeDirectory() {
    serveError = '';
    
    try {
      // First select the folder (don't show loading yet - dialog is blocking)
      const selected = await SelectFolder();
      if (!selected) {
        return; // User cancelled
      }
      
      // NOW show loading - we have a folder and are starting the server
      serveLoading = true;
      
      const result = await StartLocalDevServer(selected);
      
      if (result.success) {
        localServerRunning = true;
        localServerUrl = result.url;
        localServerDirectory = result.directory;
        localServerPort = result.port;
        localServerWatcherActive = true;
        recentChanges = [];
      } else {
        serveError = result.error || 'Failed to start server';
      }
    } catch (e) {
      serveError = e.message || 'Failed to start server';
    } finally {
      serveLoading = false;
    }
  }
  
  async function stopLocalServer() {
    try {
      await StopLocalDevServer();
      localServerRunning = false;
      localServerUrl = '';
      localServerDirectory = '';
      localServerPort = 0;
      localServerWatcherActive = false;
      recentChanges = [];
    } catch (e) {
      console.error('Failed to stop server:', e);
    }
  }
  
  function openInBrowser() {
    if (localServerUrl && localServerDirectory) {
      // Set pending navigation with the local URL
      navigateTo(`local://${localServerDirectory}`);
      // Switch to Browser tab
      window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'browser' }));
    }
  }
  
  async function triggerManualRefresh() {
    try {
      await RefreshLocalDevServer();
    } catch (e) {
      console.error('Failed to trigger refresh:', e);
    }
  }
  
  // =====================================================
  // Clone Functions
  // =====================================================
  
  async function cloneContent(allowUpdates = false) {
    if (!cloneScid || cloneScid.trim() === '') {
      cloneError = 'Please enter an SCID';
      return;
    }
    
    cloneLoading = true;
    cloneError = '';
    cloneResult = null;
    showCloneConfirmModal = false;
    
    try {
      const result = await CloneTELA(cloneScid.trim(), allowUpdates);
      
      if (result.success) {
        cloneResult = result;
        cloneError = '';
      } else if (result.requiresConfirm) {
        // Content has been updated - show confirmation modal
        showCloneConfirmModal = true;
        cloneError = '';
      } else {
        cloneError = result.error || 'Clone failed';
      }
    } catch (e) {
      cloneError = e.message || 'Failed to clone content';
    } finally {
      cloneLoading = false;
    }
  }
  
  function confirmCloneUpdate() {
    showCloneConfirmModal = false;
    cloneContent(true);  // Clone with allowUpdates = true
  }
  
  function cancelCloneUpdate() {
    showCloneConfirmModal = false;
  }
  
  function resetClone() {
    cloneScid = '';
    cloneResult = null;
    cloneError = '';
    showCloneConfirmModal = false;
  }
  
  function copyClonePath() {
    if (cloneResult?.directory) {
      ClipboardSetText(cloneResult.directory);
    }
  }
  
  async function openCloneFolder() {
    if (cloneResult?.directory) {
      // Use the shell to open the folder
      try {
        BrowserOpenURL(`file://${cloneResult.directory}`);
      } catch (e) {
        console.error('Failed to open folder:', e);
      }
    }
  }
  
  function serveClonedContent() {
    if (cloneResult?.directory) {
      // Navigate to Serve tab and start serving the cloned content
      // For now, just switch to serve tab - user can select the folder
      activeTab = 'serve';
    }
  }

  // =====================================================
  // DocShards Functions (matching Clone/Serve inline pattern)
  // =====================================================
  
  async function selectShardFile() {
    try {
      const path = await SelectFile();
      if (path) {
        shardFilePath = path;
        shardError = '';
      }
    } catch (e) {
      console.error('File selection error:', e);
      shardError = 'Failed to select file';
    }
  }
  
  async function selectShardFolder() {
    try {
      const path = await SelectFolder();
      if (path) {
        shardFolderPath = path;
        shardError = '';
      }
    } catch (e) {
      console.error('Folder selection error:', e);
      shardError = 'Failed to select folder';
    }
  }
  
  async function performShard() {
    if (!shardFilePath) {
      toast.warning('Please select a file to shard');
      return;
    }
    
    shardLoading = true;
    shardResult = null;
    shardError = '';
    
    try {
      const res = await ShardFile(shardFilePath, shardCompress);
      if (res.success) {
        shardResult = { ...res, mode: 'shard' };
        toast.success(`File sharded into ${res.shardCount} parts`);
      } else {
        shardError = res.error || 'Sharding failed';
        toast.error(shardError);
      }
    } catch (e) {
      shardError = e.message || 'Sharding failed';
      toast.error(shardError);
    } finally {
      shardLoading = false;
    }
  }
  
  async function performReconstruct() {
    if (!shardFolderPath) {
      toast.warning('Please select a folder containing shard files');
      return;
    }
    
    shardLoading = true;
    shardResult = null;
    shardError = '';
    
    try {
      const res = await ConstructFromShards(shardFolderPath);
      if (res.success) {
        shardResult = { ...res, mode: 'reconstruct' };
        toast.success('File reconstructed successfully');
      } else {
        shardError = res.error || 'Reconstruction failed';
        toast.error(shardError);
      }
    } catch (e) {
      shardError = e.message || 'Reconstruction failed';
      toast.error(shardError);
    } finally {
      shardLoading = false;
    }
  }
  
  function resetShard() {
    shardFilePath = '';
    shardFolderPath = '';
    shardResult = null;
    shardError = '';
  }
  
  function formatShardBytes(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  // =====================================================
  // Deploy SC Functions (raw smart contract deployment)
  // =====================================================
  
  async function deploySmartContract() {
    if (!scCode.trim()) {
      scDeployError = 'Please enter smart contract code';
      return;
    }
    
    // Check if wallet is open
    if (!$walletState.isOpen && !isSimulator) {
      scDeployError = 'Please open a wallet first';
      return;
    }
    
    scDeploying = true;
    scDeployError = '';
    scDeployResult = null;
    
    try {
      const result = await InstallSmartContract(scCode, scAnonymous);
      
      if (result.success) {
        scDeployResult = {
          txid: result.txid,
          message: result.message
        };
        toast.success('Smart contract deployed successfully!');
      } else {
        scDeployError = result.error || 'Deployment failed';
        toast.error(scDeployError);
      }
    } catch (e) {
      scDeployError = e.message || 'Deployment failed';
      toast.error(scDeployError);
    } finally {
      scDeploying = false;
    }
  }
  
  function resetSCDeploy() {
    scCode = '';
    scAnonymous = false;
    scDeployResult = null;
    scDeployError = '';
  }

  // =====================================================
  // My Content Functions (matching tela-cli search my docs/indexes)
  // =====================================================
  
  async function loadMyContent() {
    if (myContentLoading) return;
    
    myContentLoading = true;
    myContentError = '';
    myContentGnomonRequired = false;
    
    try {
      const result = await SearchMyContent();
      
      if (result.success) {
        myDocs = result.docs || [];
        myIndexes = result.indexes || [];
        
        // Also load available doc types for filtering
        const typesResult = await GetAvailableDOCTypes();
        if (typesResult.success) {
          availableDocTypes = typesResult.types || [];
        }
      } else {
        // Check if the error is about Gnomon not running
        const errorMsg = result.error || 'Failed to load content';
        if (errorMsg.toLowerCase().includes('gnomon')) {
          myContentGnomonRequired = true;
        }
        myContentError = errorMsg;
      }
    } catch (e) {
      myContentError = e.message || 'Failed to load content';
    } finally {
      myContentLoading = false;
      // IMPORTANT: Always mark as loaded to prevent infinite loop
      // The error state will be shown instead of retrying automatically
      myContentLoaded = true;
    }
  }
  
  async function loadMyDOCs() {
    myContentLoading = true;
    myContentError = '';
    
    try {
      const result = await SearchMyDOCs(myContentDocTypeFilter);
      
      if (result.success) {
        myDocs = result.results || [];
      } else {
        myContentError = result.error || 'Failed to load DOCs';
      }
    } catch (e) {
      myContentError = e.message || 'Failed to load DOCs';
    } finally {
      myContentLoading = false;
    }
  }
  
  async function loadMyINDEXes() {
    myContentLoading = true;
    myContentError = '';
    
    try {
      const result = await SearchMyINDEXes();
      
      if (result.success) {
        myIndexes = result.results || [];
      } else {
        myContentError = result.error || 'Failed to load INDEXes';
      }
    } catch (e) {
      myContentError = e.message || 'Failed to load INDEXes';
    } finally {
      myContentLoading = false;
    }
  }
  
  function refreshMyContent() {
    myContentLoaded = false;
    myContentError = '';
    myContentGnomonRequired = false;
    loadMyContent();
  }
  
  function copyMyContentScid(scid) {
    ClipboardSetText(scid);
    copiedScid = scid;
    setTimeout(() => { copiedScid = null; }, 2000);
  }
  
  function viewMyContentInBrowser(scid) {
    navigateTo(scid);
    window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'browser' }));
  }
  
  function updateMyContent(scid) {
    updateIndexScid = scid;
    activeTab = 'update-index';
  }
  
  // =====================================================
  // Actions / Version History Functions
  // =====================================================
  
  async function loadActionsContent() {
    if (!actionsScid || actionsScid.length !== 64) {
      actionsError = 'Please enter a valid 64-character SCID';
      return;
    }
    
    actionsLoading = true;
    actionsError = '';
    actionsContentInfo = null;
    
    try {
      // Try to get INDEX info first
      const indexResult = await GetINDEXInfo(actionsScid);
      if (indexResult.success) {
        actionsContentInfo = {
          type: 'INDEX',
          scid: actionsScid,
          name: indexResult.name || 'Unnamed INDEX',
          durl: indexResult.durl || '',
          description: indexResult.description || '',
          docCount: indexResult.docs?.length || 0,
        };
      } else {
        // Fallback - it might be a DOC or other SC
        actionsContentInfo = {
          type: 'Unknown',
          scid: actionsScid,
          name: 'Smart Contract',
          description: 'Unable to determine content type',
        };
      }
    } catch (e) {
      actionsError = e.message || 'Failed to load content info';
    } finally {
      actionsLoading = false;
    }
  }
  
  function openVersionHistory(scid) {
    versionHistoryScid = scid || actionsScid;
    showVersionHistory = true;
  }
  
  function closeVersionHistory() {
    showVersionHistory = false;
  }
  
  async function handleVersionRevert(event) {
    const commit = event.detail;
    // Clone at that specific version, then prompt to update
    const cloneScidAtVersion = `${versionHistoryScid}@${commit.txid || commit.height}`;
    cloneScid = cloneScidAtVersion;
    activeTab = 'clone';
    showVersionHistory = false;
  }
  
  async function handleVersionClone(event) {
    const commit = event.detail;
    // Clone at that specific version
    if (commit.txid) {
      cloneScid = `${versionHistoryScid}@${commit.txid}`;
    } else if (commit.height) {
      // Need TXID for CloneAtCommit - for now just use the SCID
      cloneScid = versionHistoryScid;
    }
    activeTab = 'clone';
    showVersionHistory = false;
  }
  
  function viewVersionHistoryFromMyContent(scid) {
    actionsScid = scid;
    versionHistoryScid = scid;
    showVersionHistory = true;
  }
  
  // Auto-load My Content when switching to the tab
  $: if (activeTab === 'my-content' && !myContentLoaded && !myContentLoading && $walletState.isOpen) {
    loadMyContent();
  }
  
  // Reload when wallet changes
  $: if ($walletState.address) {
    myContentLoaded = false;
    if (activeTab === 'my-content') {
      loadMyContent();
    }
  }
  
  function handleFileChange(data) {
    const fileName = data.file || 'unknown';
    const time = new Date().toLocaleTimeString();
    
    recentChanges = [
      { file: fileName, time },
      ...recentChanges.slice(0, 9) // Keep last 10
    ];
  }
  
  function formatDirectory(dir) {
    if (!dir) return '';
    // Show just the last 2 parts of the path
    const parts = dir.split('/').filter(Boolean);
    if (parts.length <= 2) return dir;
    return '.../' + parts.slice(-2).join('/');
  }
  
  // Network state - reactive to settings store
  $: currentNetwork = $settingsState.network || 'mainnet';
  $: currentNetConfig = getNetworkConfig(currentNetwork);
  $: isSimulator = currentNetwork === 'simulator';
  
  const networks = [
    { 
      id: 'mainnet', 
      label: 'Mainnet', 
      icon: 'globe', 
      status: 'err',
      warning: 'Permanent • Costs DERO',
      description: 'Live blockchain - transactions are irreversible'
    },
    { 
      id: 'testnet', 
      label: 'Testnet', 
      icon: 'flask', 
      status: 'warn',
      warning: 'Test Network',
      description: 'Test blockchain - use testnet DERO'
    },
    { 
      id: 'simulator', 
      label: 'Simulator', 
      icon: 'gamepad', 
      status: 'ok',
      warning: 'Safe Testing',
      description: 'Local simulation - perfect for testing'
    },
  ];
  
  function getNetworkConfig(id) {
    return networks.find(n => n.id === id) || networks[0];
  }
  
  // Simulator modal state
  let showSimModal = false;
  let simModalAction = null; // 'start' or 'stop'
  let simIsRunning = false;
  let simIsLoading = false;
  
  // Check simulator status on mount
  onMount(async () => {
    try {
      const status = await GetSimulatorStatus();
      simIsRunning = status?.isInitialized || false;
    } catch (e) {
      simIsRunning = false;
    }
  });
  
  async function switchNetwork(networkId) {
    // Special handling for simulator mode
    if (networkId === 'simulator') {
      if (!simIsRunning) {
        // Show confirmation modal to start simulator
        simModalAction = 'start';
        showSimModal = true;
        return;
      }
    } else if (currentNetwork === 'simulator' && simIsRunning) {
      // Switching away from running simulator - ask to stop
      simModalAction = 'stop';
      showSimModal = true;
      return;
    }
    
    // For non-simulator switches, just update the network mode
    try {
      const result = await SetNetworkMode(networkId);
      if (result.success) {
        await syncNetworkMode();
      } else {
        console.error('Failed to switch network:', result.error);
      }
    } catch (err) {
      console.error('Failed to switch network:', err);
    }
  }
  
  function cancelSimModal() {
    showSimModal = false;
    simModalAction = null;
  }
  
  async function confirmSimModal() {
    showSimModal = false;
    simIsLoading = true;
    
    try {
      if (simModalAction === 'start') {
        const result = await StartSimulatorMode();
        if (result.success) {
          simIsRunning = true;
          await syncNetworkMode();
        } else {
          console.error('Failed to start simulator:', result.error);
          alert('Failed to start simulator: ' + result.error);
        }
      } else if (simModalAction === 'stop') {
        await StopSimulatorMode();
        simIsRunning = false;
        // Switch to mainnet after stopping
        const result = await SetNetworkMode('mainnet');
        if (result.success) {
          await syncNetworkMode();
        }
      }
    } catch (e) {
      console.error('Simulator action failed:', e);
      alert('Simulator action failed: ' + e.message);
    }
    
    simIsLoading = false;
    simModalAction = null;
  }
  
  const tabs = [
    { id: 'install-doc', label: 'Install DOC', icon: 'file' },
    { id: 'batch-upload', label: 'Batch Upload', icon: 'folder' },
    { id: 'install-index', label: 'Install INDEX', icon: 'layers' },
    { id: 'update-index', label: 'Update INDEX', icon: 'refresh' },
    { id: 'deploy-sc', label: 'Deploy SC', icon: 'code' },
    { id: 'my-content', label: 'My Content', icon: 'package' },
    { id: 'actions', label: 'Version Control', icon: 'git' },
    { id: 'clone', label: 'Clone', icon: 'copy' },
    { id: 'serve', label: 'Serve', icon: 'server' },
    { id: 'diff', label: 'Diff', icon: 'diff' },
    { id: 'shards', label: 'DocShards', icon: 'file' },
  ];
  
  // MODULES section tabs
  const moduleTabs = [
    { id: 'modules', label: 'Modules', icon: 'puzzle' },
    { id: 'libraries', label: 'Libraries', icon: 'library' },
  ];
  
  // MODs state
  let modsLoading = false;
  let allMods = [];
  let filteredMods = [];
  let modClasses = [];
  let selectedModClass = 'all';
  let modSearchQuery = '';
  let modsError = null;
  let selectedMod = null;
  let modDetails = null;
  let loadingModDetails = false;
  let showModInstallWizard = false;
  let modInstallScid = '';
  let modInstallLoading = false;
  let modInstallResult = null;
  let modInstallError = null;
  
  // Map class names to icons
  const modClassIconMap = {
    'vs': Palette,
    'tx': Zap,
    'storage': Database,
    'auth': Shield,
  };
  
  function getModClassIcon(className) {
    return modClassIconMap[className?.toLowerCase()] || Wrench;
  }
  
  // Libraries state
  let librariesLoading = false;
  let librariesLoadingStatus = ''; // Shows current loading step
  let libraries = [];
  let librariesError = null;
  let librarySearchQuery = '';
  let selectedLibrary = null;
  let gnomonRequired = false; // Tracks if Gnomon needs to be started
  let librariesLoaded = false; // Prevents re-fetching on tab switch
  
  // Load Libraries data
  async function loadLibrariesData(forceRefresh = false) {
    if (librariesLoading) return; // Prevent double-loading
    
    librariesLoading = true;
    librariesError = null;
    librariesLoadingStatus = 'Checking Gnomon indexer...';
    gnomonRequired = false;
    
    try {
      // Check/start Gnomon with timeout
      const gnomonResult = await Promise.race([
        EnsureGnomonRunning(),
        new Promise((_, reject) => setTimeout(() => reject(new Error('Gnomon startup timed out')), 30000))
      ]);
      
      if (!gnomonResult.success && !gnomonResult.alreadyRunning) {
        gnomonRequired = true;
        librariesError = 'Gnomon indexer is required to browse libraries. Enable it in Settings → Gnomon.';
        librariesLoading = false;
        return;
      }
      
      librariesLoadingStatus = 'Fetching libraries from network...';
      
      // Fetch libraries with timeout
      const result = await Promise.race([
        GetTELALibraries(),
        new Promise((_, reject) => setTimeout(() => reject(new Error('Request timed out')), 15000))
      ]);
      
      if (result.success) {
        libraries = result.libraries || [];
        librariesLoaded = true;
      } else {
        librariesError = result.error || 'Failed to load libraries';
      }
    } catch (e) {
      console.error('Libraries load error:', e);
      librariesError = e.message || 'An error occurred while loading libraries';
    } finally {
      librariesLoading = false;
      librariesLoadingStatus = '';
    }
  }
  
  // Filter libraries by search
  $: filteredLibraries = libraries.filter(lib => {
    if (!librarySearchQuery.trim()) return true;
    const query = librarySearchQuery.toLowerCase();
    return (lib.name?.toLowerCase().includes(query)) ||
           (lib.durl?.toLowerCase().includes(query)) ||
           (lib.description?.toLowerCase().includes(query));
  });
  
  // Load libraries when switching to libraries tab (only first time)
  $: if (activeTab === 'libraries' && !librariesLoaded && !librariesLoading && !librariesError) {
    loadLibrariesData();
  }
  
  function openLibraryDetails(lib) {
    selectedLibrary = lib;
  }
  
  function closeLibraryDetails() {
    selectedLibrary = null;
  }
  
  function copyLibraryScid() {
    if (selectedLibrary?.scid) {
      ClipboardSetText(selectedLibrary.scid);
      // Visual feedback
      copiedScid = selectedLibrary.scid;
      setTimeout(() => { copiedScid = null; }, 2000);
    }
  }
  
  async function cloneLibrary() {
    if (!selectedLibrary?.scid) return;
    try {
      const result = await CloneTELA(selectedLibrary.scid, false);
      if (result.success) {
        toast.success(`Library cloned to: ${result.directory}`);
      } else {
        toast.error(`Clone failed: ${result.error}`);
      }
    } catch (e) {
      toast.error(`Clone error: ${e.message}`);
    }
  }
  
  function previewLibrary() {
    if (selectedLibrary?.scid) {
      navigateTo(`tela://${selectedLibrary.durl || selectedLibrary.scid}`);
      window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'browser' }));
      closeLibraryDetails();
    }
  }
  
  // Embed library into Install INDEX (adds SCID to DOC references)
  function embedLibraryInIndex() {
    if (!selectedLibrary?.scid) return;
    
    // Check if already added
    if (indexDocScids.includes(selectedLibrary.scid)) {
      toast.info(`Library already added to INDEX`);
      return;
    }
    
    // Add to DOC references
    indexDocScids = [...indexDocScids, selectedLibrary.scid];
    closeLibraryDetails();
    
    // Switch to Install INDEX tab
    activeTab = 'install-index';
    toast.success(`Added ${selectedLibrary.durl || 'library'} to DOC references`);
  }
  
  // Navigate to Settings > Gnomon section to enable Gnomon
  function goToSettings() {
    window.dispatchEvent(new CustomEvent('status-click', { detail: { tab: 'settings', section: 'gnomon' } }));
  }
  
  // Load MODs data
  async function loadModsData() {
    modsLoading = true;
    modsError = null;
    
    try {
      const modsResult = await GetMODsList();
      if (modsResult.success) {
        allMods = modsResult.mods || [];
        filteredMods = [...allMods];
      } else {
        modsError = modsResult.error || 'Failed to load Modules';
      }
      
      const classesResult = await GetAllMODClasses();
      if (classesResult.success) {
        modClasses = classesResult.classes || [];
      }
    } catch (e) {
      modsError = e.message || 'An error occurred';
    } finally {
      modsLoading = false;
    }
  }
  
  function filterMods() {
    let result = [...allMods];
    
    if (selectedModClass !== 'all') {
      result = result.filter(m => m.class === selectedModClass);
    }
    
    if (modSearchQuery.trim()) {
      const query = modSearchQuery.toLowerCase();
      result = result.filter(m => 
        m.name.toLowerCase().includes(query) ||
        m.tag.toLowerCase().includes(query) ||
        m.description?.toLowerCase().includes(query)
      );
    }
    
    filteredMods = result;
  }
  
  $: if (selectedModClass || modSearchQuery !== undefined) {
    filterMods();
  }
  
  async function openModDetails(mod) {
    selectedMod = mod;
    modDetails = null;
    loadingModDetails = true;
    
    try {
      const result = await GetMODInfo(mod.tag);
      if (result.success) {
        modDetails = result;
      }
    } catch (e) {
      console.error('Failed to load MOD details:', e);
    } finally {
      loadingModDetails = false;
    }
  }
  
  function closeModDetails() {
    selectedMod = null;
    modDetails = null;
  }
  
  function openModInstallWizard() {
    showModInstallWizard = true;
    modInstallScid = '';
    modInstallResult = null;
    modInstallError = null;
  }
  
  function closeModInstallWizard() {
    showModInstallWizard = false;
    modInstallScid = '';
    modInstallResult = null;
    modInstallError = null;
  }
  
  async function prepareModInstall() {
    if (!selectedMod || !modInstallScid || modInstallScid.length < 64) {
      modInstallError = 'Please enter a valid 64-character SCID';
      return;
    }
    
    modInstallLoading = true;
    modInstallError = null;
    modInstallResult = null;
    
    try {
      const result = await PrepareMODInstall(modInstallScid, selectedMod.tag);
      if (result.success) {
        modInstallResult = result;
      } else {
        modInstallError = result.error || 'Failed to prepare installation';
      }
    } catch (e) {
      modInstallError = e.message || 'An error occurred';
    } finally {
      modInstallLoading = false;
    }
  }
  
  function copyModCode(text, label = 'Code') {
    navigator.clipboard.writeText(text);
    // Could add toast here
  }
  
  // Load MODs when switching to modules tab
  $: if (activeTab === 'modules' && allMods.length === 0 && !modsLoading) {
    loadModsData();
  }
  
  async function handleFilesStaged(event) {
    stagedFiles = event.detail.files;
    await calculateTotalGas();
  }
  
  async function removeFile(index) {
    stagedFiles = stagedFiles.filter((_, i) => i !== index);
    await calculateTotalGas();
  }
  
  async function calculateTotalGas() {
    if (stagedFiles.length === 0) {
      totalGasEstimate = 0;
      return;
    }
    
    gasLoading = true;
    let total = 0;
    
    try {
      for (const file of stagedFiles) {
        // Use backend gas estimate
        const docInfo = JSON.stringify({
          size: file.size,
          path: file.path || '',
        });
        const result = await GetGasEstimate(docInfo);
        if (result.success) {
          total += result.gasEstimate;
        } else {
          // Fallback: simple estimation
          total += 5000 + (file.size * 10);
        }
      }
    } catch (e) {
      // Fallback: simple estimation
      total = stagedFiles.reduce((sum, f) => sum + 5000 + (f.size * 10), 0);
    }
    
    totalGasEstimate = total;
    gasLoading = false;
  }
  
  function formatGas(gas) {
    if (gas >= 1000000) {
      return (gas / 1000000).toFixed(2) + 'M';
    } else if (gas >= 1000) {
      return (gas / 1000).toFixed(1) + 'K';
    }
    return gas.toLocaleString();
  }
  
  function gasToDero(gas) {
    // Rough conversion: gas cost varies, but roughly 1 DERO = 10000 gas in storage
    // This is an estimate - actual costs depend on network conditions
    return (gas / 100000).toFixed(5);
  }
  
  // Check if any staged files can benefit from compression (text-based types)
  // Matches tela-cli's canCompress logic
  function canCompressFiles(files) {
    const compressibleTypes = [
      'text/html', 'text/css', 'text/javascript', 'application/javascript',
      'application/json', 'text/markdown', 'text/x-go', 'text/plain'
    ];
    const compressibleExtensions = ['.html', '.htm', '.css', '.js', '.json', '.md', '.go', '.txt'];
    
    return files.some(file => {
      const ext = file.name?.toLowerCase().split('.').pop();
      const type = file.type?.toLowerCase() || '';
      
      // Skip already compressed files
      if (file.name?.endsWith('.gz')) return false;
      
      return compressibleTypes.some(t => type.includes(t)) ||
             compressibleExtensions.some(e => `.${ext}` === e);
    });
  }
  
  // Update INDEX state - additional vars
  let updateIndexName = '';
  let updateIndexDescription = '';
  let updateIndexIcon = '';
  let updateIsSimulator = false;
  let showUpdateConfirmModal = false;
  
  // Update INDEX functions
  async function loadIndexInfo() {
    if (!updateIndexScid || updateIndexScid.length < 64) {
      updateIndexError = 'Please enter a valid 64-character SCID';
      return;
    }
    
    updateIndexLoading = true;
    updateIndexError = '';
    updateIndexInfo = null;
    updateResult = null;
    
    try {
      // Check if in simulator mode
      updateIsSimulator = await IsInSimulatorMode();
      
      const result = await GetINDEXInfo(updateIndexScid);
      if (result.success) {
        updateIndexInfo = result;
        updateIndexDocs = [...(result.docs || [])];
        // Initialize editable fields with current values
        updateIndexName = result.name || '';
        updateIndexDescription = result.description || '';
        updateIndexIcon = result.icon || '';
        
        // Check if INDEX can be updated
        if (!result.canUpdate) {
          updateIndexError = 'This INDEX is immutable (deployed with Ring 16+) and cannot be updated.';
        } else if (!result.isOwner && !updateIsSimulator) {
          updateIndexError = 'Your wallet is not the owner of this INDEX.';
        }
      } else {
        updateIndexError = result.error || 'Failed to load INDEX info';
      }
    } catch (e) {
      updateIndexError = e.message || 'Failed to load INDEX info';
    } finally {
      updateIndexLoading = false;
    }
  }
  
  function addDocToIndex() {
    if (!newDocScid || newDocScid.length < 64) {
      return;
    }
    if (!updateIndexDocs.includes(newDocScid)) {
      updateIndexDocs = [...updateIndexDocs, newDocScid];
    }
    newDocScid = '';
  }
  
  function removeDocFromIndex(scid) {
    updateIndexDocs = updateIndexDocs.filter(d => d !== scid);
  }
  
  function prepareIndexUpdate() {
    // In simulator mode, skip confirmation
    if (updateIsSimulator) {
      submitIndexUpdate();
    } else {
      showUpdateConfirmModal = true;
    }
  }
  
  function cancelUpdateConfirm() {
    showUpdateConfirmModal = false;
  }
  
  async function submitIndexUpdate() {
    showUpdateConfirmModal = false;
    
    // In simulator mode, wallet may not be needed (uses sim wallet)
    if (!updateIndexInfo) return;
    if (!updateIsSimulator && !$walletState.isOpen) {
      updateResult = { type: 'error', message: 'Please connect your wallet' };
      return;
    }
    
    updateInProgress = true;
    updateResult = null;
    
    try {
      const indexData = JSON.stringify({
        name: updateIndexName,
        description: updateIndexDescription,
        durl: updateIndexInfo.durl,  // dURL cannot be changed
        iconUrl: updateIndexIcon,
        docScids: updateIndexDocs,
      });
      
      const result = await UpdateINDEX(updateIndexScid, indexData);
      if (result.success) {
        updateResult = { 
          type: 'success', 
          message: 'INDEX updated successfully!',
          txid: result.txid
        };
      } else {
        updateResult = { type: 'error', message: result.error || 'Update failed' };
      }
    } catch (e) {
      updateResult = { type: 'error', message: e.message || 'Update failed' };
    } finally {
      updateInProgress = false;
    }
  }
  
  function resetUpdateIndex() {
    updateIndexScid = '';
    updateIndexInfo = null;
    updateIndexDocs = [];
    updateIndexError = '';
    updateResult = null;
    updateIndexName = '';
    updateIndexDescription = '';
    updateIndexIcon = '';
    showUpdateConfirmModal = false;
  }
  
  function isOwner() {
    // Use the backend's isOwner check
    return updateIndexInfo?.isOwner || false;
  }
  
  function canUpdateIndex() {
    return updateIndexInfo?.canUpdate && (updateIndexInfo?.isOwner || updateIsSimulator);
  }
  
  function getDocTypeIcon(docType) {
    const icons = {
      'text/html': 'file-code',
      'text/css': 'palette',
      'application/javascript': 'code',
      'image/svg+xml': 'image',
      'image/png': 'image',
      'image/jpeg': 'image',
      'application/json': 'braces',
    };
    return icons[docType] || 'file';
  }
  
  async function deployBatch() {
    // Allow deployment if wallet is open OR if in simulator mode (uses simulator wallet)
    if (!$walletState.isOpen && !isSimulator) {
      deploymentStatus = { type: 'error', message: 'Please open a wallet first' };
      return;
    }
    
    if (stagedFiles.length === 0) {
      deploymentStatus = { type: 'error', message: 'No files staged for deployment' };
      return;
    }
    
    deploymentStatus = { type: 'info', message: `Deploying ${stagedFiles.length} DOC${stagedFiles.length > 1 ? 's' : ''}...` };
    
    try {
      const results = [];
      for (const stagedFile of stagedFiles) {
        // Read file contents - either from browser File object or from backend data
        let fileContent = '';
        if (stagedFile.data) {
          // Data already provided (from Wails native file picker)
          fileContent = stagedFile.data;
        } else if (stagedFile.file) {
          // Read from browser File object (drag & drop)
          fileContent = await readFileAsText(stagedFile.file);
        }
        
        const docInfo = {
          name: stagedFile.name,
          path: '', // Don't use path - we're sending data directly
          subDir: stagedFile.subDir || '/',
          docType: stagedFile.type || 'text/html',
          size: stagedFile.size,
          description: docDescription || '',  // From metadata fields
          iconUrl: docIconURL || '',          // From metadata fields
          ringsize: docRingsize,              // 2 = updateable, 16+ = immutable
          compressed: docCompression,         // Enable gzip compression (tela-cli parity)
          data: fileContent // Send the actual file content
        };
        
        const result = await InstallDOC(JSON.stringify(docInfo));
        results.push({ file: stagedFile.name, ...result });
        
        if (!result.success) {
          deploymentStatus = { type: 'error', message: `Failed to deploy ${stagedFile.name}: ${result.error}` };
          return;
        }
      }
      
      // All succeeded - store detailed results for success UI
      const deployedResults = results.map((r, i) => ({
        scid: r.txid || r.scid || '',
        txid: r.txid || '',
        fileName: stagedFiles[i]?.name || 'unknown',
        fileSize: stagedFiles[i]?.size || 0,
        fileType: stagedFiles[i]?.type || 'text/html',
      }));
      
      deploymentStatus = { 
        type: 'success', 
        message: `Successfully deployed ${stagedFiles.length} DOC${stagedFiles.length > 1 ? 's' : ''}!`,
        results: deployedResults,
        timestamp: new Date().toLocaleTimeString(),
        network: currentNetwork,
      };
      
      // Clear staged files on success (keep deployment results visible)
      stagedFiles = [];
      
    } catch (e) {
      deploymentStatus = { type: 'error', message: e.message || 'Deployment failed' };
    }
  }
  
  // Helper to read File object as text
  function readFileAsText(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsText(file);
    });
  }
  
  // Copy SCID to clipboard with feedback
  async function copyScid(scid) {
    try {
      await navigator.clipboard.writeText(scid);
      copiedScid = scid;
      setTimeout(() => { copiedScid = null; }, 2000);
    } catch (e) {
      console.error('Failed to copy:', e);
    }
  }
  
  // Navigate to Browser tab with SCID
  function previewInBrowser(scid) {
    navigateTo(scid);
    window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'browser' }));
  }
  
  // Clear deployment results and start fresh
  function clearDeploymentResults() {
    deploymentStatus = null;
  }
  
  // Format file size for display
  function formatFileSize(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
  }
  
  // =====================================================
  // INDEX Installation Functions (matching tela-cli)
  // =====================================================
  
  // Add a DOC SCID to the INDEX
  function addDocToNewIndex() {
    if (!newIndexDocScid || newIndexDocScid.length !== 64) {
      indexInstallError = 'Please enter a valid 64-character SCID';
      return;
    }
    if (indexDocScids.includes(newIndexDocScid)) {
      indexInstallError = 'This DOC is already added';
      return;
    }
    indexDocScids = [...indexDocScids, newIndexDocScid];
    newIndexDocScid = '';
    indexInstallError = '';
  }
  
  // Remove a DOC SCID from the INDEX
  function removeDocFromNewIndex(scid) {
    indexDocScids = indexDocScids.filter(s => s !== scid);
  }
  
  // Install the INDEX
  async function installIndex() {
    // Validate requirements
    if (!indexName.trim()) {
      indexInstallError = 'Application name is required';
      return;
    }
    if (!indexDURL.trim()) {
      indexInstallError = 'dURL is required for INDEX';
      return;
    }
    if (indexDocScids.length === 0) {
      indexInstallError = 'At least one DOC is required';
      return;
    }
    if (!$walletState.isOpen && !isSimulator) {
      indexInstallError = 'Please open a wallet first';
      return;
    }
    
    indexInstalling = true;
    indexInstallError = '';
    indexInstallResult = null;
    
    try {
      const indexData = {
        name: indexName.trim(),
        durl: indexDURL.trim(),
        description: indexDescription.trim(),
        iconUrl: indexIconURL.trim(),
        docScids: indexDocScids,
        licenses: [],
        ringsize: indexRingsize, // 2 = updateable, 16+ = immutable
        mods: indexEnableMods ? getModTags() : '', // MOD tags if enabled
      };
      
      const result = await InstallINDEX(JSON.stringify(indexData));
      
      if (result.success) {
        indexInstallResult = {
          type: 'success',
          scid: result.txid,
          durl: indexDURL,
          message: `INDEX created successfully!`,
          timestamp: new Date().toLocaleTimeString(),
          network: currentNetwork,
        };
        // Clear form on success
        // Keep result visible, let user clear manually
      } else {
        indexInstallError = result.error || 'Failed to create INDEX';
      }
    } catch (e) {
      indexInstallError = e.message || 'Failed to create INDEX';
    } finally {
      indexInstalling = false;
    }
  }
  
  // Reset INDEX form
  function resetIndexForm() {
    indexName = '';
    indexDURL = '';
    indexDescription = '';
    indexIconURL = '';
    indexDocScids = [];
    newIndexDocScid = '';
    indexRingsize = 2;
    indexInstallResult = null;
    indexInstallError = '';
    // Reset MODs state
    indexEnableMods = false;
    indexSelectedVsMod = '';
    indexSelectedTxMods = [];
  }
  
  // =====================================================
  // Icon URL Validation (matching tela-cli format support)
  // =====================================================
  // Valid formats:
  // - Empty (optional field)
  // - HTTPS URL: https://example.com/icon.png
  // - HTTP URL: http://example.com/icon.png (warning: not recommended)
  // - SCID: 64 hex characters (on-chain image reference)
  // - IPFS: ipfs://... (future support)
  
  function validateIconURL(url) {
    if (!url || url.trim() === '') {
      return { valid: true, type: 'empty', message: '' };
    }
    
    const trimmed = url.trim();
    
    // Check if it's a valid SCID (64 hex characters)
    const scidPattern = /^[a-fA-F0-9]{64}$/;
    if (scidPattern.test(trimmed)) {
      return { valid: true, type: 'scid', message: 'Valid SCID (on-chain reference)' };
    }
    
    // Check for HTTPS URL
    if (trimmed.startsWith('https://')) {
      // Basic URL validation
      try {
        new URL(trimmed);
        return { valid: true, type: 'https', message: 'HTTPS URL' };
      } catch {
        return { valid: false, type: 'invalid', message: 'Invalid URL format' };
      }
    }
    
    // Check for HTTP URL (warning)
    if (trimmed.startsWith('http://')) {
      try {
        new URL(trimmed);
        return { valid: true, type: 'http', message: 'HTTP URL (HTTPS recommended)', warning: true };
      } catch {
        return { valid: false, type: 'invalid', message: 'Invalid URL format' };
      }
    }
    
    // Check for IPFS
    if (trimmed.startsWith('ipfs://')) {
      return { valid: true, type: 'ipfs', message: 'IPFS reference' };
    }
    
    // Unknown format - might be invalid
    return { valid: false, type: 'unknown', message: 'Use HTTPS URL, SCID (64 chars), or IPFS' };
  }
  
  // Reactive icon validation for Install INDEX
  $: indexIconValidation = validateIconURL(indexIconURL);
  
  // =====================================================
  // dURL Tag Detection (matching tela-cli conventions)
  // =====================================================
  // Special dURL suffixes indicate content type:
  // - .lib     = Library (collection of reusable DOCs)
  // - .shard   = DocShard DOC
  // - .shards  = DocShards INDEX (requires reconstruction)
  // - .bootstrap = Bootstrap INDEX (collection of apps)
  
  const DURL_TAGS = {
    '.lib': {
      name: 'Library',
      icon: 'lib',
      description: 'A collection of reusable DOCs that can be embedded in other apps',
      color: 'violet'
    },
    '.shard': {
      name: 'DocShard',
      icon: '🧩',
      description: 'A shard DOC (part of a larger file split across multiple contracts)',
      color: 'cyan'
    },
    '.shards': {
      name: 'DocShards',
      icon: 'shards',
      description: 'An INDEX containing DocShards that require reconstruction',
      color: 'cyan'
    },
    '.bootstrap': {
      name: 'Bootstrap',
      icon: 'bootstrap',
      description: 'A collection of TELA apps/content for bootstrapping',
      color: 'amber'
    }
  };
  
  function detectDurlTag(durl) {
    if (!durl || durl.trim() === '') {
      return null;
    }
    
    const trimmed = durl.trim().toLowerCase();
    
    for (const [tag, info] of Object.entries(DURL_TAGS)) {
      if (trimmed.endsWith(tag)) {
        return { tag, ...info };
      }
    }
    
    // Standard .tela suffix (optional but conventional)
    if (trimmed.endsWith('.tela')) {
      return {
        tag: '.tela',
        name: 'Standard',
        icon: '◇',
        description: 'Standard TELA application',
        color: 'default'
      };
    }
    
    return null;
  }
  
  // Reactive dURL tag detection for Install INDEX
  $: indexDurlTag = detectDurlTag(indexDURL);
  
  // Get MODs grouped by class (uses allMods from MODULES section)
  function getVsModOptions() {
    return allMods.filter(m => m.tag.startsWith('vs'));
  }
  
  function getTxModOptions() {
    return allMods.filter(m => m.tag.startsWith('tx'));
  }
  
  // Toggle a Transfer MOD
  function toggleTxMod(tag) {
    if (indexSelectedTxMods.includes(tag)) {
      indexSelectedTxMods = indexSelectedTxMods.filter(t => t !== tag);
    } else {
      indexSelectedTxMods = [...indexSelectedTxMods, tag];
    }
  }
  
  // Get combined MOD tags string
  function getModTags() {
    const tags = [];
    if (indexSelectedVsMod) tags.push(indexSelectedVsMod);
    tags.push(...indexSelectedTxMods);
    return tags.join(',');
  }
  
  // Watch for MODs toggle to load MODs and force ringsize
  $: if (indexEnableMods) {
    if (allMods.length === 0 && !modsLoading) {
      loadModsData(); // Reuse existing function from MODULES section
    }
    indexRingsize = 2; // MODs require ringsize 2
  }
  
  // Handle MOD picker modal confirmation
  function handleModPickerConfirm(event) {
    const { vsMod, txMods } = event.detail;
    indexSelectedVsMod = vsMod;
    indexSelectedTxMods = txMods;
    if (vsMod || txMods.length > 0) {
      indexEnableMods = true;
    }
    showModPickerModal = false;
  }
  
  // =====================================================
  // Confirmation Modal Functions
  // =====================================================
  
  function showDeployConfirmation(type, data) {
    confirmModalType = type;
    confirmModalData = data;
    showConfirmModal = true;
  }
  
  function cancelConfirmation() {
    showConfirmModal = false;
    confirmModalType = '';
    confirmModalData = null;
    deployAcknowledged = false;
  }
  
  async function confirmDeployment() {
    if (!deployAcknowledged) return; // Extra safety check
    showConfirmModal = false;
    deployAcknowledged = false;
    
    if (confirmModalType === 'doc') {
      await deployBatch();
    } else if (confirmModalType === 'index') {
      await installIndex();
    }
    
    confirmModalType = '';
    confirmModalData = null;
  }
  
  // Prepare DOC deployment with confirmation
  function prepareDocDeployment() {
    if (!$walletState.isOpen && !isSimulator) {
      deploymentStatus = { type: 'error', message: 'Please open a wallet first' };
      return;
    }
    if (stagedFiles.length === 0) {
      deploymentStatus = { type: 'error', message: 'No files staged for deployment' };
      return;
    }
    
    // Show confirmation for mainnet, auto-deploy for simulator
    if (isSimulator) {
      deployBatch();
    } else {
      showDeployConfirmation('doc', {
        files: stagedFiles,
        gasEstimate: totalGasEstimate,
        network: currentNetwork,
      });
    }
  }
  
  // Prepare INDEX deployment with confirmation  
  function prepareIndexDeployment() {
    // Validate
    if (!indexName.trim()) {
      indexInstallError = 'Application name is required';
      return;
    }
    if (!indexDURL.trim()) {
      indexInstallError = 'dURL is required for INDEX';
      return;
    }
    if (indexDocScids.length === 0) {
      indexInstallError = 'At least one DOC is required';
      return;
    }
    if (!$walletState.isOpen && !isSimulator) {
      indexInstallError = 'Please open a wallet first';
      return;
    }
    
    // Show confirmation for mainnet, auto-deploy for simulator
    if (isSimulator) {
      installIndex();
    } else {
      showDeployConfirmation('index', {
        name: indexName,
        durl: indexDURL,
        docCount: indexDocScids.length,
        gasEstimate: INDEX_BASE_GAS,
        network: currentNetwork,
      });
    }
  }
</script>

<div class="page-layout">
  <!-- Page Header -->
  <div class="page-header">
    <div class="page-header-inner">
      <div class="page-header-left">
        <h1 class="page-header-title">
          <Palette size={18} class="page-header-icon" strokeWidth={1.5} />
          Studio
        </h1>
        <p class="page-header-desc">Create and deploy TELA applications</p>
      </div>
      <div class="page-header-actions">
        <span class="badge" class:badge-warn={currentNetConfig.status === 'err'} class:badge-ok={currentNetConfig.status === 'ok'} class:badge-cyan={currentNetConfig.status === 'warn'}>
          {currentNetConfig.warning}
        </span>
        <div class="network-toggle-group">
          {#each networks as network}
            <button
              on:click={() => switchNetwork(network.id)}
              class="network-toggle-btn"
              class:active={currentNetwork === network.id}
            >
              {#if network.icon === 'globe'}<Globe size={14} />
              {:else if network.icon === 'flask'}<FlaskConical size={14} />
              {:else}<Gamepad2 size={14} />{/if}
              <span>{network.label}</span>
            </button>
          {/each}
        </div>
      </div>
    </div>
  </div>
  
  <!-- v5.6 Unified Page Body -->
  <div class="page-body">
    <!-- Sidebar -->
    <div class="page-sidebar">
      <div class="page-sidebar-section">ACTIONS</div>
      <nav class="page-sidebar-nav">
    {#each tabs as tab}
      <button
        on:click={() => activeTab = tab.id}
            class="page-sidebar-item"
            class:active={activeTab === tab.id}
      >
            <span class="page-sidebar-item-icon">
        {#if tab.icon === 'file'}<FileText size={14} />
        {:else if tab.icon === 'folder'}<FolderUp size={14} />
        {:else if tab.icon === 'layers'}<Layers size={14} />
        {:else if tab.icon === 'refresh'}<RefreshCw size={14} />
        {:else if tab.icon === 'package'}<Package size={14} />
        {:else if tab.icon === 'history'}<History size={14} />
        {:else if tab.icon === 'copy'}<Copy size={14} />
        {:else if tab.icon === 'server'}<Server size={14} />
        {:else if tab.icon === 'git'}<GitBranch size={14} />
        {:else if tab.icon === 'code'}<FileCode size={14} />
        {:else}<GitCompare size={14} />{/if}
            </span>
            <span class="page-sidebar-item-label">{tab.label}</span>
      </button>
    {/each}
      </nav>
      
      <!-- MODULES Section -->
      <div class="page-sidebar-section" style="margin-top: var(--s-5);">MODULES</div>
      <nav class="page-sidebar-nav">
        {#each moduleTabs as tab}
          <button
            on:click={() => activeTab = tab.id}
            class="page-sidebar-item"
            class:active={activeTab === tab.id}
          >
            <span class="page-sidebar-item-icon">
              {#if tab.icon === 'puzzle'}
                <Puzzle size={14} />
              {:else if tab.icon === 'library'}
                <Library size={14} />
              {:else}
                <Puzzle size={14} />
              {/if}
            </span>
            <span class="page-sidebar-item-label">{tab.label}</span>
            {#if tab.id === 'modules' && allMods.length > 0}
              <span class="page-sidebar-item-count">({allMods.length})</span>
            {:else if tab.id === 'libraries' && libraries.length > 0}
              <span class="page-sidebar-item-count">({libraries.length})</span>
            {/if}
          </button>
        {/each}
      </nav>
  </div>
  
    <!-- Content Area -->
    <div class="page-content">
    {#if activeTab === 'install-doc'}
      <div class="content-section">
        <h2 class="content-section-title">Install TELA DOC</h2>
        <p class="content-section-desc">
          Deploy a single file as a standalone DOC smart contract.
          <button class="batch-hint-link" on:click={() => activeTab = 'batch-upload'}>
            For multi-file apps, use Batch Upload <ArrowRight size={12} />
          </button>
        </p>
        
        <!-- Drop Zone -->
        <DropZone on:filesStaged={handleFilesStaged} />
        
        <!-- v6.1 Staged Files List -->
        {#if stagedFiles.length > 0}
          <div class="staged-section">
            <h3 class="content-section-title">Staged Files <span class="text-text-4">({stagedFiles.length})</span></h3>
            
            <!-- Multi-file info banner -->
            {#if stagedFiles.length > 1}
              <div class="multi-file-info">
                <AlertTriangle size={14} />
                <span>
                  Multiple files will create <strong>{stagedFiles.length} separate DOCs</strong> (no INDEX linking them).
                </span>
                <button class="info-action-btn" on:click={() => activeTab = 'batch-upload'}>
                  Use Batch Upload instead <ArrowRight size={12} />
                </button>
              </div>
            {/if}
            
            <!-- Enhanced staged file list with editable fields -->
            <div class="staged-list">
              {#each stagedFiles as file, index}
                <div class="staged-item-enhanced">
                  <div class="staged-item-header">
                    <FileText size={16} class="staged-icon" />
                    <div class="staged-info">
                      <div class="staged-name">{file.name}</div>
                      <div class="staged-meta">{file.type || 'text/html'} • {(file.size / 1024).toFixed(1)} KB</div>
                    </div>
                    <button on:click={() => removeFile(index)} class="staged-remove" title="Remove file">
                      <X size={14} />
                    </button>
                  </div>
                  
                  <!-- Editable SubDir -->
                  <div class="staged-item-field">
                    <label class="staged-field-label">SubDir</label>
                    <input
                      type="text"
                      bind:value={file.subDir}
                      placeholder="/"
                      class="input input-sm"
                    />
                  </div>
                </div>
              {/each}
            </div>
            
            <!-- DOC Metadata Section -->
            <div class="doc-metadata-section">
              <h4 class="metadata-title">DOC Metadata <span class="text-text-4">(optional)</span></h4>
              
              <div class="metadata-grid">
                <div class="form-group">
                  <label class="form-label">Description</label>
                  <input
                    type="text"
                    bind:value={docDescription}
                    placeholder="Brief description of this content..."
                    class="input"
                  />
                </div>
                
                <div class="form-group">
                  <label class="form-label">Icon URL</label>
                  <input
                    type="text"
                    bind:value={docIconURL}
                    placeholder="https://... or SCID"
                    class="input"
                  />
                </div>
              </div>
              
              <!-- Ringsize Selector -->
              <div class="ringsize-section">
                <label class="form-label">Update Permissions</label>
                <div class="ringsize-options">
                  <button 
                    class="ringsize-option" 
                    class:selected={docRingsize === 2}
                    on:click={() => docRingsize = 2}
                  >
                    <RefreshCw size={14} />
                    <span class="ringsize-label">Updateable</span>
                    <span class="ringsize-hint">Ring 2 - can modify later</span>
                  </button>
                  <button 
                    class="ringsize-option" 
                    class:selected={docRingsize === 16}
                    on:click={() => docRingsize = 16}
                  >
                    <Lock size={14} />
                    <span class="ringsize-label">Immutable</span>
                    <span class="ringsize-hint">Ring 16+ - permanent</span>
                  </button>
                </div>
              </div>
              
              <!-- Compression Toggle (matching tela-cli) -->
              {#if stagedFiles.length > 0 && canCompressFiles(stagedFiles)}
                <div class="compression-section">
                  <label class="form-label">Compression</label>
                  <button 
                    class="compression-toggle"
                    class:enabled={docCompression}
                    on:click={() => docCompression = !docCompression}
                  >
                    <div class="compression-toggle-track">
                      <div class="compression-toggle-thumb"></div>
                    </div>
                    <div class="compression-info">
                      <span class="compression-label">
                        {docCompression ? 'Enabled' : 'Disabled'}
                      </span>
                      <span class="compression-hint">
                        {docCompression 
                          ? 'Files will be gzip compressed (smaller on-chain size)' 
                          : 'Files will be stored uncompressed'}
                      </span>
                    </div>
                  </button>
                  {#if docCompression}
                    <div class="compression-note">
                      <FileArchive size={12} />
                      <span>Text files (HTML, CSS, JS, JSON, MD, Go) will be gzip compressed before deployment</span>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
            
            <!-- v6.1 Gas Estimate -->
            {#if totalGasEstimate > 0 || gasLoading || isSimulator}
              <div class="gas-estimate" class:simulator-mode={isSimulator}>
                <div class="gas-row">
                  <div>
                    <p class="data-label">Estimated Cost</p>
                    {#if isSimulator}
                      <p class="gas-value gas-free">
                        <Gamepad2 size={14} />
                        FREE (Simulator)
                      </p>
                      <p class="gas-dero simulator-note">No real DERO required</p>
                    {:else if gasLoading}
                      <p class="gas-value loading">Calculating...</p>
                    {:else}
                      <p class="gas-value c-emerald">~{formatGas(totalGasEstimate)} gas</p>
                      <p class="gas-dero">≈ {gasToDero(totalGasEstimate)} DERO</p>
                    {/if}
                  </div>
                  <div class="text-right">
                    <p class="data-label">Total Size</p>
                    <p class="gas-size">
                      {(stagedFiles.reduce((sum, f) => sum + f.size, 0) / 1024).toFixed(1)} KB
                    </p>
                  </div>
                </div>
                {#if currentNetwork === 'mainnet' && totalGasEstimate > 100000}
                  <div class="gas-warning">
                    <AlertTriangle size={12} />
                    <span>Large deployment - consider testing on Simulator first</span>
                  </div>
                {/if}
                {#if isSimulator}
                  <div class="simulator-info">
                    <Gamepad2 size={12} />
                    <span>Deploy instantly with auto-confirmation</span>
                  </div>
                {/if}
              </div>
            {/if}
            
            <!-- v6.1 Deploy Button -->
            <div class="deploy-row">
              <button
                on:click={prepareDocDeployment}
                disabled={!$walletState.isOpen && !isSimulator}
                class="btn btn-primary"
                class:btn-simulator={isSimulator}
              >
                {#if isSimulator}
                  <Gamepad2 size={14} />
                  Deploy to Simulator ({stagedFiles.length} DOC{stagedFiles.length > 1 ? 's' : ''})
                {:else}
                  Deploy {stagedFiles.length} DOC{stagedFiles.length > 1 ? 's' : ''}
                {/if}
              </button>
              {#if !$walletState.isOpen && !isSimulator}
                <span class="wallet-warning">
                  <AlertTriangle size={14} />
                  Wallet required for deployment
                </span>
              {:else if isSimulator}
                <span class="simulator-badge-small">
                  <Gamepad2 size={12} />
                  Auto-mines to confirm
                </span>
              {/if}
            </div>
            
            {#if deploymentStatus}
              {#if deploymentStatus.type === 'success' && deploymentStatus.results?.length > 0}
                <!-- Success Card with SCID Display -->
                <div class="deployment-success-card">
                  <div class="success-header">
                    <div class="success-icon">
                      <CheckCircle size={24} />
                    </div>
                    <div class="success-info">
                      <h4 class="success-title">Deployment Successful!</h4>
                      <p class="success-meta">
                        {deploymentStatus.results.length} DOC{deploymentStatus.results.length > 1 ? 's' : ''} deployed 
                        {#if deploymentStatus.network === 'simulator'}
                          <span class="network-badge simulator">Simulator</span>
                        {:else}
                          <span class="network-badge {deploymentStatus.network}">{deploymentStatus.network}</span>
                        {/if}
                        • {deploymentStatus.timestamp}
                      </p>
                    </div>
                    <button class="success-close" on:click={clearDeploymentResults}>
                      <X size={16} />
                    </button>
                  </div>
                  
                  {#each deploymentStatus.results as result, i}
                    <div class="deployed-item">
                      <div class="deployed-file-info">
                        <FileText size={16} class="deployed-icon" />
                        <div class="deployed-details">
                          <span class="deployed-name">{result.fileName}</span>
                          <span class="deployed-size">{formatFileSize(result.fileSize)}</span>
                        </div>
                      </div>
                      
                      {#if result.scid}
                        <div class="deployed-scid-row">
                          <span class="scid-label">SCID:</span>
                          <code class="scid-value">{result.scid}</code>
                        </div>
                        
                        <div class="deployed-actions">
                          <button 
                            class="action-btn copy-btn" 
                            on:click={() => copyScid(result.scid)}
                            title="Copy SCID"
                          >
                            {#if copiedScid === result.scid}
                              <Check size={14} />
                              <span>Copied!</span>
                            {:else}
                              <Clipboard size={14} />
                              <span>Copy SCID</span>
                            {/if}
                          </button>
                          <button 
                            class="action-btn preview-btn" 
                            on:click={() => previewInBrowser(result.scid)}
                            title="View in Browser"
                          >
                            <Eye size={14} />
                            <span>Preview</span>
                          </button>
                        </div>
                      {/if}
                    </div>
                  {/each}
                  
                  {#if deploymentStatus.network === 'simulator'}
                    <div class="success-note">
                      <Gamepad2 size={12} />
                      <span>Content deployed to local simulator. Preview it above or paste the SCID in the browser.</span>
                    </div>
                  {/if}
                </div>
              {:else}
                <!-- Standard status message (error/info) -->
                <div class="deployment-status deployment-status-{deploymentStatus.type}">
                  {deploymentStatus.message}
                </div>
              {/if}
            {/if}
          </div>
        {/if}
        
        <!-- Deployment Status (OUTSIDE stagedFiles check so it persists after success) -->
        {#if deploymentStatus && !stagedFiles.length}
          {#if deploymentStatus.type === 'success' && deploymentStatus.results?.length > 0}
            <!-- Success Card with SCID Display -->
            <div class="deployment-success-card">
              <div class="success-header">
                <div class="success-icon">
                  <CheckCircle size={24} />
                </div>
                <div class="success-info">
                  <h4 class="success-title">Deployment Successful!</h4>
                  <p class="success-meta">
                    {deploymentStatus.results.length} DOC{deploymentStatus.results.length > 1 ? 's' : ''} deployed 
                    {#if deploymentStatus.network === 'simulator'}
                      <span class="network-badge simulator">Simulator</span>
                    {:else}
                      <span class="network-badge {deploymentStatus.network}">{deploymentStatus.network}</span>
                    {/if}
                    • {deploymentStatus.timestamp}
                  </p>
                </div>
                <button class="success-close" on:click={clearDeploymentResults}>
                  <X size={16} />
                </button>
              </div>
              
              {#each deploymentStatus.results as result, i}
                <div class="deployed-item">
                  <div class="deployed-file-info">
                    <FileText size={16} class="deployed-icon" />
                    <div class="deployed-details">
                      <span class="deployed-name">{result.fileName}</span>
                      <span class="deployed-size">{formatFileSize(result.fileSize)}</span>
                    </div>
                  </div>
                  
                  {#if result.scid}
                    <div class="deployed-scid-row">
                      <span class="scid-label">SCID:</span>
                      <code class="scid-value">{result.scid}</code>
                    </div>
                    
                    <div class="deployed-actions">
                      <button 
                        class="action-btn copy-btn" 
                        on:click={() => copyScid(result.scid)}
                        title="Copy SCID"
                      >
                        {#if copiedScid === result.scid}
                          <Check size={14} />
                          <span>Copied!</span>
                        {:else}
                          <Clipboard size={14} />
                          <span>Copy SCID</span>
                        {/if}
                      </button>
                      <button 
                        class="action-btn preview-btn" 
                        on:click={() => previewInBrowser(result.scid)}
                        title="View in Browser"
                      >
                        <Eye size={14} />
                        <span>Preview</span>
                      </button>
                    </div>
                  {/if}
                </div>
              {/each}
              
              {#if deploymentStatus.network === 'simulator'}
                <div class="success-note">
                  <Gamepad2 size={12} />
                  <span>Content deployed to local simulator. Preview it above or paste the SCID in the browser.</span>
                </div>
              {/if}
            </div>
          {:else if deploymentStatus.type === 'error'}
            <!-- Error message when no staged files -->
            <div class="deployment-status deployment-status-error">
              {deploymentStatus.message}
            </div>
          {/if}
        {/if}
      </div>
    
    {:else if activeTab === 'batch-upload'}
      <div class="content-section">
        <h2 class="content-section-title">Batch Upload</h2>
        <p class="content-section-desc">Upload an entire folder to create DOCs + INDEX in one operation.</p>
        
        <!-- v6.1 Folder Selection Dropzone -->
        {#if !batchFolderPath}
          <div 
            bind:this={batchDropzoneElement}
            class="dropzone"
            class:active={batchDragging}
            on:dragover|preventDefault={() => batchDragging = true}
            on:dragleave={() => batchDragging = false}
            on:drop|preventDefault={() => {
              // Visual feedback reset - actual path is set by Wails OnFileDrop handler
              // which provides REAL filesystem paths (browser API only gives virtual paths)
              batchDragging = false;
            }}
            on:click={async () => {
              const selected = await SelectFolder();
              if (selected) {
                batchFolderPath = selected;
              }
            }}
            role="button"
            tabindex="0"
          >
            <div class="dropzone-icon">
              {#if batchDragging}
                <FolderDown size={40} strokeWidth={1.5} />
              {:else}
                <FolderUp size={40} strokeWidth={1.5} />
              {/if}
            </div>
            <p class="dropzone-title">
              {batchDragging ? 'Drop folder here' : 'Drag & drop a folder'}
            </p>
            <p class="dropzone-hint">
              Or click to browse. All files will be scanned for batch deployment.
            </p>
          </div>
        {:else}
          <BatchUpload 
            folderPath={batchFolderPath} 
            on:complete={(e) => {
              // Show success toast notification
              toast.success(`Deployment complete! INDEX: ${e.detail.indexScid?.substring(0, 16)}...`);
              // Don't clear batchFolderPath - let user see the success card and SCIDs
              // They can click "Choose different folder" button to start over
            }}
            on:preview={(e) => {
              // Navigate to browser with the SCID
              previewInBrowser(e.detail.scid);
            }}
          />
          
          <button
            on:click={() => batchFolderPath = ''}
            class="btn btn-ghost back-link"
          >
            ← Choose different folder
          </button>
        {/if}
      </div>
    
    {:else if activeTab === 'install-index'}
      <div class="content-section">
        <h2 class="content-section-title">Install TELA INDEX</h2>
        <p class="content-section-desc">Create a TELA INDEX to organize and serve your DOCs as a web application.</p>
        
        {#if indexInstallResult?.type === 'success'}
          <!-- Success Display -->
          <div class="deployment-success-card">
            <div class="success-header">
              <div class="success-icon">
                <CheckCircle size={24} />
              </div>
              <div class="success-info">
                <h4 class="success-title">INDEX Created Successfully!</h4>
                <p class="success-meta">
                  {#if indexInstallResult.network === 'simulator'}
                    <span class="network-badge simulator">Simulator</span>
                  {:else}
                    <span class="network-badge {indexInstallResult.network}">{indexInstallResult.network}</span>
                  {/if}
                  • {indexInstallResult.timestamp}
                </p>
              </div>
              <button class="success-close" on:click={resetIndexForm}>
                <X size={16} />
              </button>
            </div>
            
            <div class="deployed-item">
              <div class="deployed-file-info">
                <Layers size={16} class="deployed-icon" />
                <div class="deployed-details">
                  <span class="deployed-name">{indexName}</span>
                  <span class="deployed-size">dURL: {indexInstallResult.durl}</span>
                </div>
              </div>
              
              <div class="deployed-scid-row">
                <span class="scid-label">SCID:</span>
                <code class="scid-value">{indexInstallResult.scid}</code>
              </div>
              
              <div class="deployed-actions">
                <button 
                  class="action-btn copy-btn" 
                  on:click={() => copyScid(indexInstallResult.scid)}
                  title="Copy SCID"
                >
                  {#if copiedScid === indexInstallResult.scid}
                    <Check size={14} />
                    <span>Copied!</span>
                  {:else}
                    <Clipboard size={14} />
                    <span>Copy SCID</span>
                  {/if}
                </button>
                <button 
                  class="action-btn preview-btn" 
                  on:click={() => previewInBrowser(indexInstallResult.scid)}
                  title="View in Browser"
                >
                  <Eye size={14} />
                  <span>Preview</span>
                </button>
              </div>
            </div>
            
            {#if indexInstallResult.network === 'simulator'}
              <div class="success-note">
                <Gamepad2 size={12} />
                <span>INDEX deployed to local simulator. Preview it above or use dero://{indexInstallResult.durl}</span>
              </div>
            {/if}
            
            <button 
              class="btn btn-ghost" 
              style="margin-top: var(--s-4);"
              on:click={resetIndexForm}
            >
              <Plus size={14} />
              Create Another INDEX
            </button>
          </div>
        {:else}
          <!-- INDEX Form -->
          <div class="form-stack">
            <!-- Application Name -->
            <div class="form-group">
              <label class="form-label">
                Application Name <span class="required">*</span>
              </label>
              <input
                type="text"
                bind:value={indexName}
                placeholder="My TELA App"
                class="input"
              />
            </div>
            
            <!-- dURL (Required for INDEX) with tag detection -->
            <div class="form-group">
              <label class="form-label">
                dURL <span class="required">*</span>
                {#if indexDurlTag}
                  <span class="durl-tag-badge" class:tag-violet={indexDurlTag.color === 'violet'} class:tag-cyan={indexDurlTag.color === 'cyan'} class:tag-amber={indexDurlTag.color === 'amber'}>
                    <span class="tag-icon">{indexDurlTag.icon}</span>
                    {indexDurlTag.name}
                  </span>
                {/if}
              </label>
              <input
                type="text"
                bind:value={indexDURL}
                placeholder="my-app.tela"
                class="input"
              />
              {#if indexDurlTag && indexDurlTag.tag !== '.tela'}
                <p class="form-hint durl-tag-hint" class:hint-violet={indexDurlTag.color === 'violet'} class:hint-cyan={indexDurlTag.color === 'cyan'} class:hint-amber={indexDurlTag.color === 'amber'}>
                  {indexDurlTag.description}
                </p>
              {:else}
                <p class="form-hint">Accessible via dero://{indexDURL || 'my-app'}</p>
              {/if}
            </div>
            
            <!-- Description -->
            <div class="form-group">
              <label class="form-label">Description</label>
              <textarea
                bind:value={indexDescription}
                placeholder="Describe your application..."
                rows="3"
                class="input textarea"
              ></textarea>
            </div>
            
            <!-- Icon URL with validation -->
            <div class="form-group">
              <label class="form-label">Icon URL</label>
              <div class="icon-url-input-wrapper">
                <input
                  type="text"
                  bind:value={indexIconURL}
                  placeholder="https://example.com/icon.png or SCID"
                  class="input"
                  class:input-valid={indexIconURL && indexIconValidation.valid && !indexIconValidation.warning}
                  class:input-warning={indexIconValidation.warning}
                  class:input-error={indexIconURL && !indexIconValidation.valid}
                />
                {#if indexIconURL}
                  <span class="icon-url-status" class:valid={indexIconValidation.valid && !indexIconValidation.warning} class:warning={indexIconValidation.warning} class:invalid={!indexIconValidation.valid}>
                    {#if indexIconValidation.valid && !indexIconValidation.warning}
                      <CheckCircle size={14} />
                    {:else if indexIconValidation.warning}
                      <AlertTriangle size={14} />
                    {:else}
                      <X size={14} />
                    {/if}
                  </span>
                {/if}
              </div>
              {#if indexIconURL && indexIconValidation.message}
                <p class="form-hint" class:hint-valid={indexIconValidation.valid && !indexIconValidation.warning} class:hint-warning={indexIconValidation.warning} class:hint-error={!indexIconValidation.valid}>
                  {indexIconValidation.message}
                </p>
              {:else}
                <p class="form-hint">URL or SCID of an image for the app icon</p>
              {/if}
            </div>
            
            <!-- DOC References Section -->
            <div class="form-group">
              <label class="form-label">
                DOC References <span class="required">*</span>
                <span class="text-text-4">({indexDocScids.length})</span>
              </label>
              
              {#if indexDocScids.length > 0}
                <div class="docs-list" style="margin-bottom: var(--s-3);">
                  {#each indexDocScids as scid, i}
                    <div class="doc-item">
                      <span class="doc-item-num">{i + 1}.</span>
                      <span class="doc-item-scid">{scid.slice(0, 16)}...{scid.slice(-8)}</span>
                      <button
                        on:click={() => removeDocFromNewIndex(scid)}
                        class="remove-btn"
                        title="Remove DOC"
                      >
                        <X size={14} />
                      </button>
                    </div>
                  {/each}
                </div>
              {:else}
                <p class="form-hint" style="margin-bottom: var(--s-3); color: var(--text-3);">
                  Add at least one DOC SCID that will be part of this INDEX
                </p>
              {/if}
              
              <!-- Add DOC Input -->
              <div class="doc-add-row">
                <input
                  type="text"
                  bind:value={newIndexDocScid}
                  placeholder="Enter DOC SCID (64 characters)..."
                  class="input input-mono"
                  on:keydown={(e) => e.key === 'Enter' && addDocToNewIndex()}
                />
                <button
                  on:click={addDocToNewIndex}
                  class="btn btn-secondary"
                  disabled={newIndexDocScid.length !== 64}
                >
                  <Plus size={14} />
                  Add
                </button>
              </div>
            </div>
            
            <!-- Ringsize Selector for INDEX -->
            <div class="ringsize-section">
              <label class="form-label">Update Permissions</label>
              <div class="ringsize-options">
                <button 
                  class="ringsize-option" 
                  class:selected={indexRingsize === 2}
                  on:click={() => indexRingsize = 2}
                >
                  <RefreshCw size={14} />
                  <span class="ringsize-label">Updateable</span>
                  <span class="ringsize-hint">Ring 2 - can modify DOCs later</span>
                </button>
                <button 
                  class="ringsize-option" 
                  class:selected={indexRingsize === 16}
                  class:disabled={indexEnableMods}
                  on:click={() => !indexEnableMods && (indexRingsize = 16)}
                  disabled={indexEnableMods}
                  title={indexEnableMods ? 'MODs require updateable INDEX (Ring 2)' : ''}
                >
                  <Lock size={14} />
                  <span class="ringsize-label">Immutable</span>
                  <span class="ringsize-hint">{indexEnableMods ? 'Disabled when MODs enabled' : 'Ring 16+ - permanent INDEX'}</span>
                </button>
              </div>
            </div>
            
            <!-- TELA-MODs Section (matching tela-cli modsPrompt) -->
            <div class="mods-section">
              <div class="mods-header">
                <label class="form-label">
                  <Puzzle size={14} />
                  TELA-MODs
                </label>
                <div class="mods-header-actions">
                  <button 
                    class="mods-advanced-btn"
                    on:click={() => showModPickerModal = true}
                    title="Open MOD picker"
                  >
                    <Wrench size={12} />
                    Advanced
                  </button>
                  <button 
                    class="mods-toggle"
                    class:enabled={indexEnableMods}
                    on:click={() => indexEnableMods = !indexEnableMods}
                  >
                    <div class="mods-toggle-track">
                      <div class="mods-toggle-thumb"></div>
                    </div>
                    <span>{indexEnableMods ? 'Enabled' : 'Disabled'}</span>
                  </button>
                </div>
              </div>
              
              {#if indexEnableMods}
                <div class="mods-content">
                  {#if modsLoading}
                    <div class="mods-loading">
                      <Loader2 size={16} class="spin" />
                      <span>Loading MODs...</span>
                    </div>
                  {:else}
                    <p class="mods-description">
                      MODs add smart contract functionality to your INDEX. MODs require Ring 2 (updateable).
                    </p>
                    
                    <!-- Variable Store MOD (single selection) -->
                    <div class="mod-group">
                      <label class="mod-group-label">
                        <Database size={12} />
                        Variable Store
                        <span class="mod-group-hint">(select one)</span>
                      </label>
                      <div class="mod-options">
                        <button 
                          class="mod-option"
                          class:selected={indexSelectedVsMod === ''}
                          on:click={() => indexSelectedVsMod = ''}
                        >
                          <span class="mod-option-name">None</span>
                        </button>
                        {#each getVsModOptions() as mod}
                          <button 
                            class="mod-option"
                            class:selected={indexSelectedVsMod === mod.tag}
                            on:click={() => indexSelectedVsMod = mod.tag}
                            title={mod.description}
                          >
                            <span class="mod-option-tag">{mod.tag}</span>
                            <span class="mod-option-name">{mod.name.replace('Variable store ', '')}</span>
                          </button>
                        {/each}
                      </div>
                    </div>
                    
                    <!-- Transfer MODs (multi-selection) -->
                    <div class="mod-group">
                      <label class="mod-group-label">
                        <Zap size={12} />
                        Transfers
                        <span class="mod-group-hint">(select multiple)</span>
                      </label>
                      <div class="mod-options">
                        {#each getTxModOptions() as mod}
                          <button 
                            class="mod-option"
                            class:selected={indexSelectedTxMods.includes(mod.tag)}
                            on:click={() => toggleTxMod(mod.tag)}
                            title={mod.description}
                          >
                            <span class="mod-option-tag">{mod.tag}</span>
                            <span class="mod-option-name">{mod.name}</span>
                            {#if indexSelectedTxMods.includes(mod.tag)}
                              <Check size={12} class="mod-check" />
                            {/if}
                          </button>
                        {/each}
                      </div>
                    </div>
                    
                    <!-- Selected MODs summary -->
                    {#if getModTags()}
                      <div class="mods-summary">
                        <span class="mods-summary-label">Selected:</span>
                        <code class="mods-summary-tags">{getModTags()}</code>
                      </div>
                    {/if}
                  {/if}
                </div>
              {:else}
                <p class="mods-hint">
                  Enable to add smart contract functionality (variable stores, deposits, transfers)
                </p>
              {/if}
            </div>
            
            <!-- Gas Estimate -->
            <div class="gas-estimate" class:simulator-mode={isSimulator}>
              <div class="gas-row">
                <div>
                  <p class="data-label">Estimated Cost</p>
                  {#if isSimulator}
                    <p class="gas-value gas-free">
                      <Gamepad2 size={14} />
                      FREE (Simulator)
                    </p>
                    <p class="gas-dero simulator-note">No real DERO required</p>
                  {:else}
                    <p class="gas-value c-emerald">~{formatGas(INDEX_BASE_GAS + (indexDocScids.length * 1000))} gas</p>
                    <p class="gas-dero">≈ {gasToDero(INDEX_BASE_GAS + (indexDocScids.length * 1000))} DERO</p>
                  {/if}
                </div>
                <Layers size={24} class="gas-icon" />
              </div>
              {#if currentNetwork === 'mainnet' && !isSimulator}
                <p class="gas-note">
                  INDEX contracts have a base cost. Additional DOC references may increase cost.
                </p>
              {/if}
              {#if isSimulator}
                <div class="simulator-info">
                  <Gamepad2 size={12} />
                  <span>Deploy instantly with auto-confirmation</span>
                </div>
              {/if}
            </div>
            
            <!-- Error Display -->
            {#if indexInstallError}
              <div class="alert-error">
                <AlertTriangle size={14} />
                {indexInstallError}
              </div>
            {/if}
            
            <!-- Create Button -->
            <div class="deploy-row">
              <button
                on:click={prepareIndexDeployment}
                disabled={(!$walletState.isOpen && !isSimulator) || !indexName.trim() || !indexDURL.trim() || indexDocScids.length === 0 || indexInstalling}
                class="btn btn-primary"
                class:btn-simulator={isSimulator}
              >
                {#if indexInstalling}
                  <Loader2 size={14} class="spin" />
                  Creating INDEX...
                {:else if isSimulator}
                  <Gamepad2 size={14} />
                  Create INDEX (Simulator)
                {:else}
                  <Layers size={14} />
                  Create INDEX
                {/if}
              </button>
              
              {#if !$walletState.isOpen && !isSimulator}
                <span class="wallet-warning">
                  <AlertTriangle size={14} />
                  Wallet required for deployment
                </span>
              {:else if isSimulator}
                <span class="simulator-badge-small">
                  <Gamepad2 size={12} />
                  Auto-mines to confirm
                </span>
              {/if}
            </div>
          </div>
        {/if}
      </div>
    
    {:else if activeTab === 'update-index'}
      <div class="content-section">
        <h2 class="content-section-title">Update TELA INDEX</h2>
        <p class="content-section-desc">Update an existing INDEX with new DOCs or configuration.</p>
        
        <!-- Success Display -->
        {#if updateResult?.type === 'success'}
          <div class="update-success-card">
            <div class="update-success-header">
              <CheckCircle size={24} class="update-success-icon" />
              <div>
                <h3 class="update-success-title">INDEX Updated Successfully!</h3>
                <p class="update-success-subtitle">{updateIndexInfo?.name}</p>
              </div>
            </div>
            
            <div class="update-result-details">
              <div class="update-detail-row">
                <span class="update-detail-label">SCID</span>
                <div class="update-scid-row">
                  <code class="update-scid">{updateIndexScid}</code>
                  <button class="update-copy-btn" on:click={() => ClipboardSetText(updateIndexScid)} title="Copy SCID">
                    <Clipboard size={14} />
                  </button>
                </div>
              </div>
              
              <div class="update-detail-row">
                <span class="update-detail-label">TXID</span>
                <div class="update-scid-row">
                  <code class="update-scid">{updateResult.txid}</code>
                  <button class="update-copy-btn" on:click={() => ClipboardSetText(updateResult.txid)} title="Copy TXID">
                    <Clipboard size={14} />
                  </button>
                </div>
              </div>
              
              <div class="update-detail-row">
                <span class="update-detail-label">DOCs</span>
                <span class="update-detail-value">{updateIndexDocs.length} document(s)</span>
              </div>
            </div>
            
            <div class="update-actions">
              <button class="btn btn-secondary" on:click={() => navigateTo(`tela://${updateIndexInfo?.durl}`)}>
                <Eye size={16} />
                Preview
              </button>
              <button class="btn btn-ghost" on:click={resetUpdateIndex}>
                Update Another
              </button>
            </div>
          </div>
        {:else if !updateIndexInfo}
          <!-- v6.1 SCID Input -->
          <div class="content-card">
            <div class="content-card-header">
              <RefreshCw size={32} class="content-card-icon" />
              <p class="content-card-title">Load an INDEX to Update</p>
              <p class="content-card-text">Enter the SCID of an INDEX you own to modify its metadata and DOC references.</p>
            </div>
            
            <div class="form-group" style="margin-top: var(--s-4);">
              <input
                type="text"
                bind:value={updateIndexScid}
                placeholder="64-character SCID..."
                class="input input-mono"
                disabled={updateIndexLoading}
              />
            </div>
            
            <button
              on:click={loadIndexInfo}
              disabled={updateIndexLoading || updateIndexScid.length < 64}
              class="btn btn-primary btn-block"
              style="margin-top: var(--s-4);"
            >
              {#if updateIndexLoading}
                <Loader2 size={16} class="spinner" />
                Loading...
              {:else}
                Load INDEX
              {/if}
            </button>
            
            {#if updateIndexError}
              <div class="alert alert-error" style="margin-top: var(--s-4);">
                <AlertTriangle size={16} />
                <span>{updateIndexError}</span>
              </div>
            {/if}
          </div>
        {:else}
          <!-- INDEX Info Display -->
          <div class="index-info-display">
            <!-- Header with Reset and Mode Badge -->
            <div class="index-info-header">
              <div>
                <div class="index-info-name-row">
                  <h3 class="index-info-name">{updateIndexInfo.name}</h3>
                  {#if updateIsSimulator}
                    <span class="mode-badge simulator">
                      <Gamepad2 size={12} />
                      SIMULATOR
                    </span>
                  {/if}
                </div>
                <p class="index-info-scid">{updateIndexScid.slice(0, 16)}...{updateIndexScid.slice(-8)}</p>
              </div>
              <button on:click={resetUpdateIndex} class="btn btn-ghost">
                ← Load Different
              </button>
            </div>
            
            <!-- Version Warning -->
            {#if updateIndexInfo.currentVersion && !updateIndexInfo.isLatest}
              <div class="alert alert-info" style="margin-bottom: var(--s-4);">
                <RefreshCw size={16} />
                <span>INDEX version {updateIndexInfo.currentVersion} will be upgraded to {updateIndexInfo.latestVersion}</span>
              </div>
            {/if}
            
            <!-- Ownership Status -->
            {#if !updateIndexInfo.canUpdate}
              <div class="alert alert-error" style="margin-bottom: var(--s-4);">
                <Lock size={16} />
                <span>This INDEX is immutable (deployed with Ring 16+) and cannot be updated.</span>
              </div>
            {:else if !updateIndexInfo.isOwner && !updateIsSimulator}
              <div class="alert alert-warning" style="margin-bottom: var(--s-4);">
                <AlertTriangle size={16} />
                <div>
                  <p>Your wallet is not the owner of this INDEX.</p>
                  <p style="font-size: 11px; margin-top: 4px; color: var(--text-4);">Owner: {updateIndexInfo.owner?.slice(0, 24)}...</p>
                </div>
              </div>
            {:else if updateIndexInfo.isOwner}
              <div class="alert alert-success" style="margin-bottom: var(--s-4);">
                <CheckCircle size={16} />
                <span>You are the owner of this INDEX.</span>
              </div>
            {/if}
            
            <!-- Error Display -->
            {#if updateResult?.type === 'error'}
              <div class="alert alert-error" style="margin-bottom: var(--s-4);">
                <AlertTriangle size={16} />
                <span>{updateResult.message}</span>
              </div>
            {/if}
            
            <!-- Editable Metadata -->
            <div class="card-section">
              <h4 class="card-section-title">INDEX Metadata</h4>
              
              <div class="form-group">
                <label class="form-label">Name</label>
                <input
                  type="text"
                  bind:value={updateIndexName}
                  placeholder="INDEX name..."
                  class="input"
                  disabled={!canUpdateIndex()}
                />
              </div>
              
              <div class="form-group">
                <label class="form-label">dURL <span class="label-hint">(cannot be changed)</span></label>
                <input
                  type="text"
                  value={updateIndexInfo.durl || ''}
                  class="input input-mono"
                  disabled
                />
              </div>
              
              <div class="form-group">
                <label class="form-label">Description</label>
                <textarea
                  bind:value={updateIndexDescription}
                  placeholder="Description..."
                  class="input"
                  rows="2"
                  disabled={!canUpdateIndex()}
                ></textarea>
              </div>
              
              <div class="form-group">
                <label class="form-label">Icon URL</label>
                <input
                  type="text"
                  bind:value={updateIndexIcon}
                  placeholder="https://..."
                  class="input"
                  disabled={!canUpdateIndex()}
                />
              </div>
            </div>
            
            <!-- DOCs List -->
            <div class="card-section">
              <div class="docs-list-header">
                <h4 class="card-section-title">DOC References ({updateIndexDocs.length})</h4>
              </div>
              
              {#if updateIndexDocs.length > 0}
                <div class="docs-list">
                  {#each updateIndexDocs as doc, i}
                    <div class="doc-item">
                      <span class="doc-item-num">{i + 1}.</span>
                      <span class="doc-item-scid">{doc}</span>
                      <button
                        on:click={() => removeDocFromIndex(doc)}
                        class="remove-btn"
                        disabled={!canUpdateIndex()}
                      >
                        <X size={14} />
                      </button>
                    </div>
                  {/each}
                </div>
              {:else}
                <p class="docs-list-empty">No DOCs in this INDEX</p>
              {/if}
              
              <!-- Add DOC -->
              {#if canUpdateIndex()}
                <div class="doc-add-row">
                  <input
                    type="text"
                    bind:value={newDocScid}
                    placeholder="Add DOC SCID..."
                    class="input input-mono"
                    on:keydown={(e) => e.key === 'Enter' && addDocToIndex()}
                  />
                  <button
                    on:click={addDocToIndex}
                    disabled={newDocScid.length < 64}
                    class="btn btn-secondary"
                  >
                    + Add
                  </button>
                </div>
              {/if}
            </div>
            
            <!-- Submit Button -->
            <div class="submit-row">
              <button
                on:click={prepareIndexUpdate}
                disabled={!canUpdateIndex() || updateInProgress}
                class="btn btn-primary btn-lg btn-block"
              >
                {#if updateInProgress}
                  <Loader2 size={16} class="spinner" />
                  Updating...
                {:else}
                  <RefreshCw size={16} />
                  Update INDEX
                {/if}
              </button>
              
              {#if updateIsSimulator}
                <p class="simulator-note">
                  <Gamepad2 size={12} />
                  Simulator mode: Free transactions, auto-mines
                </p>
              {:else if !$walletState.isOpen}
                <p class="wallet-warning">Connect wallet to update INDEX</p>
              {/if}
            </div>
          </div>
        {/if}
      </div>
      
      <!-- Update Confirmation Modal -->
      {#if showUpdateConfirmModal}
        <div class="modal-overlay" on:click={cancelUpdateConfirm}>
          <div class="modal-content" on:click|stopPropagation>
            <div class="modal-header">
              <RefreshCw size={24} class="modal-icon" />
              <h3 class="modal-title">Confirm INDEX Update</h3>
            </div>
            
            <div class="modal-body">
              <p>You are about to update:</p>
              <div class="confirm-details">
                <div class="confirm-row">
                  <span class="confirm-label">Name</span>
                  <span class="confirm-value">{updateIndexName}</span>
                </div>
                <div class="confirm-row">
                  <span class="confirm-label">dURL</span>
                  <code class="confirm-value">{updateIndexInfo?.durl}</code>
                </div>
                <div class="confirm-row">
                  <span class="confirm-label">DOCs</span>
                  <span class="confirm-value">{updateIndexDocs.length} document(s)</span>
                </div>
              </div>
              <p class="modal-warning">
                <AlertTriangle size={14} />
                This transaction cannot be undone.
              </p>
            </div>
            
            <div class="modal-actions">
              <button class="btn btn-ghost" on:click={cancelUpdateConfirm}>Cancel</button>
              <button class="btn btn-primary" on:click={submitIndexUpdate}>
                Confirm Update
              </button>
            </div>
          </div>
        </div>
      {/if}
    
    {:else if activeTab === 'my-content'}
      <div class="content-section">
        <h2 class="content-section-title">My Content</h2>
        <p class="content-section-desc">View and manage DOCs and INDEXes deployed by your wallet.</p>
        
        {#if !$walletState.isOpen}
          <!-- Wallet Required State -->
          <div class="content-card">
            <div class="content-card-header">
              <Lock size={32} class="content-card-icon" />
              <p class="content-card-title">Wallet Required</p>
              <p class="content-card-text">Connect a wallet to view your deployed DOCs and INDEXes.</p>
            </div>
            
            <button class="btn btn-primary btn-block" style="margin-top: var(--s-5);" on:click={() => window.dispatchEvent(new CustomEvent('switch-tab', { detail: 'wallet' }))}>
              <Wallet size={16} />
              Open Wallet
            </button>
          </div>
        {:else if myContentGnomonRequired}
          <!-- Gnomon Required State - Special handling to prevent infinite loop -->
          <div class="content-card">
            <div class="content-card-header">
              <Database size={32} class="content-card-icon" style="color: var(--cyan-400);" />
              <p class="content-card-title">Gnomon Indexer Required</p>
              <p class="content-card-text">The Gnomon indexer needs to be running to discover your deployed content. Start Gnomon in Settings, or use the SCID directly in the Browser.</p>
            </div>
            
            <div style="display: flex; gap: var(--s-3); margin-top: var(--s-5);">
              <button class="btn btn-primary" on:click={() => window.dispatchEvent(new CustomEvent('status-click', { detail: { tab: 'settings', section: 'gnomon' } }))}>
                <Database size={16} />
                Open Settings
              </button>
              <button class="btn btn-secondary" on:click={refreshMyContent}>
                <RefreshCw size={16} />
                Retry
              </button>
            </div>
          </div>
        {:else if myContentError}
          <!-- Error State -->
          <div class="alert alert-error" style="margin-bottom: var(--s-4);">
            <AlertTriangle size={16} />
            <span>{myContentError}</span>
          </div>
          <button class="btn btn-secondary" on:click={refreshMyContent}>
            <RefreshCw size={16} />
            Retry
          </button>
        {:else if myContentLoading}
          <!-- Loading State -->
          <div class="content-card centered">
            <Loader2 size={32} class="content-card-icon spin" />
            <p class="content-card-text">Loading your content...</p>
          </div>
        {:else}
          <!-- Main Content Card -->
          <div class="content-card" style="text-align: left; padding: var(--s-6);">
            <!-- Stats Row -->
            <div class="mc-stats-row">
              <div class="mc-stat">
                <span class="mc-stat-value">{myDocs.length}</span>
                <span class="mc-stat-label">DOCS</span>
              </div>
              <div class="mc-stat">
                <span class="mc-stat-value">{myIndexes.length}</span>
                <span class="mc-stat-label">INDEXES</span>
              </div>
              <div class="mc-stat">
                <span class="mc-stat-value">{myDocs.length + myIndexes.length}</span>
                <span class="mc-stat-label">TOTAL</span>
              </div>
              <button class="btn btn-ghost btn-sm mc-refresh-btn" on:click={refreshMyContent} title="Refresh">
                <span class:spin={myContentLoading}><RefreshCw size={14} /></span>
              </button>
            </div>
            
            <!-- Tab Filter -->
            <div class="mc-tabs">
              <button 
                class="mc-tab" 
                class:active={myContentTab === 'all'}
                on:click={() => myContentTab = 'all'}
              >
                All ({myDocs.length + myIndexes.length})
              </button>
              <button 
                class="mc-tab" 
                class:active={myContentTab === 'docs'}
                on:click={() => myContentTab = 'docs'}
              >
                <FileText size={14} />
                DOCs ({myDocs.length})
              </button>
              <button 
                class="mc-tab" 
                class:active={myContentTab === 'indexes'}
                on:click={() => myContentTab = 'indexes'}
              >
                <Layers size={14} />
                INDEXes ({myIndexes.length})
              </button>
            </div>
            
            <!-- DOC Type Filter (for docs tab) -->
            {#if myContentTab === 'docs' && availableDocTypes.length > 0}
              <div class="mc-filter">
                <label class="mc-filter-label">FILTER BY TYPE:</label>
                <select class="mc-filter-select" bind:value={myContentDocTypeFilter} on:change={loadMyDOCs}>
                  <option value="">All Types</option>
                  {#each availableDocTypes as docType}
                    <option value={docType}>{docType}</option>
                  {/each}
                </select>
              </div>
            {/if}
            
            <!-- Content List -->
            <div class="mc-list">
              {#if myContentTab === 'all' || myContentTab === 'indexes'}
                {#each myIndexes as index}
                  <div class="mc-item">
                    <div class="mc-item-icon index">
                      <Layers size={18} />
                    </div>
                    <div class="mc-item-info">
                      <div class="mc-item-header">
                        <span class="mc-item-name">{index.display_name || index.durl || 'INDEX'}</span>
                        <span class="mc-badge index">INDEX</span>
                      </div>
                      {#if index.description}
                        <p class="mc-item-desc">{index.description}</p>
                      {/if}
                      <div class="mc-item-meta">
                        <code class="mc-scid">{index.scid.slice(0, 8)}...{index.scid.slice(-8)}</code>
                        {#if index.doc_count}
                          <span class="mc-doc-count">{index.doc_count} DOC(s)</span>
                        {/if}
                      </div>
                    </div>
                    <div class="mc-item-actions">
                      <button class="btn btn-icon" title="Copy SCID" on:click={() => copyMyContentScid(index.scid)}>
                        {#if copiedScid === index.scid}
                          <Check size={14} />
                        {:else}
                          <Copy size={14} />
                        {/if}
                      </button>
                      <button class="btn btn-icon" title="View in Browser" on:click={() => viewMyContentInBrowser(index.scid)}>
                        <Eye size={14} />
                      </button>
                      <button class="btn btn-icon" title="Version History" on:click={() => viewVersionHistoryFromMyContent(index.scid)}>
                        <History size={14} />
                      </button>
                      <button class="btn btn-icon" title="Update INDEX" on:click={() => updateMyContent(index.scid)}>
                        <RefreshCw size={14} />
                      </button>
                    </div>
                  </div>
                {/each}
              {/if}
              
              {#if myContentTab === 'all' || myContentTab === 'docs'}
                {#each myDocs as doc}
                  <div class="mc-item">
                    <div class="mc-item-icon doc">
                      <FileText size={18} />
                    </div>
                    <div class="mc-item-info">
                      <div class="mc-item-header">
                        <span class="mc-item-name">{doc.display_name || doc.name || 'DOC'}</span>
                        <span class="mc-badge doc">DOC</span>
                        {#if doc.docType}
                          <span class="mc-badge-doctype">{doc.docType}</span>
                        {/if}
                      </div>
                      {#if doc.description}
                        <p class="mc-item-desc">{doc.description}</p>
                      {/if}
                      <div class="mc-item-meta">
                        <code class="mc-scid">{doc.scid.slice(0, 8)}...{doc.scid.slice(-8)}</code>
                        {#if doc.subDir}
                          <span class="mc-subdir">{doc.subDir}</span>
                        {/if}
                      </div>
                    </div>
                    <div class="mc-item-actions">
                      <button class="btn btn-icon" title="Copy SCID" on:click={() => copyMyContentScid(doc.scid)}>
                        {#if copiedScid === doc.scid}
                          <Check size={14} />
                        {:else}
                          <Copy size={14} />
                        {/if}
                      </button>
                      <button class="btn btn-icon" title="View" on:click={() => viewMyContentInBrowser(doc.scid)}>
                        <Eye size={14} />
                      </button>
                    </div>
                  </div>
                {/each}
              {/if}
              
              <!-- Empty State -->
              {#if (myContentTab === 'all' && myDocs.length === 0 && myIndexes.length === 0) || 
                   (myContentTab === 'docs' && myDocs.length === 0) || 
                   (myContentTab === 'indexes' && myIndexes.length === 0)}
                <div class="mc-empty">
                  <Package size={32} />
                  <p class="mc-empty-title">
                    {#if myContentTab === 'all'}
                      No content deployed yet
                    {:else if myContentTab === 'docs'}
                      No DOCs deployed yet
                    {:else}
                      No INDEXes deployed yet
                    {/if}
                  </p>
                  <p class="mc-empty-hint">
                    Deploy DOCs and INDEXes using the Install tabs above.
                  </p>
                </div>
              {/if}
            </div>
          </div>
          
          <!-- Info Panel (matching other Studio pages) -->
          <div class="info-panel" style="margin-top: var(--s-4);">
            <div class="info-panel-icon">◎</div>
            <div class="info-panel-content">
              <p class="info-panel-title">About My Content</p>
              <ul class="info-list">
                <li>Shows DOCs and INDEXes where your wallet is the owner</li>
                <li>Gnomon must be running to index and discover your content</li>
                <li>Use the action buttons to copy SCIDs, view in browser, or update</li>
              </ul>
            </div>
          </div>
        {/if}
      </div>
    
    {:else if activeTab === 'actions'}
      <div class="content-section">
        <h2 class="content-section-title">Version Control</h2>
        <p class="content-section-desc">View version history, compare commits, and perform actions on TELA content.</p>
        
        <!-- Error Display -->
        {#if actionsError}
          <div class="alert alert-error" style="margin-bottom: var(--s-4);">
            <AlertTriangle size={16} />
            <span>{actionsError}</span>
            <button class="btn btn-ghost btn-sm" on:click={() => actionsError = ''}>
              <X size={14} />
            </button>
          </div>
        {/if}
        
        <!-- SCID Input Card -->
        <div class="content-card">
          <div class="content-card-header">
            <GitBranch size={32} class="content-card-icon" />
            <p class="content-card-title">Load an INDEX to Update</p>
            <p class="content-card-text">Enter the SCID of an INDEX you own to modify its metadata and DOC references.</p>
          </div>
          
          <div class="form-group" style="margin-top: var(--s-4);">
            <label class="form-label">SCID <span class="label-hint">(64-character INDEX Smart Contract ID)</span></label>
            <input
              type="text"
              bind:value={actionsScid}
              placeholder="64-character SCID..."
              class="input input-mono"
              maxlength="64"
            />
          </div>
          
          <button 
            class="btn btn-primary btn-block" 
            style="margin-top: var(--s-4);"
            on:click={loadActionsContent}
            disabled={actionsLoading || actionsScid.length !== 64}
          >
            {#if actionsLoading}
              <Loader2 size={16} class="spinner" />
              Loading...
            {:else}
              <Search size={16} />
              Load INDEX
            {/if}
          </button>
        </div>
        
        <!-- Loaded Content Info & Actions -->
        {#if actionsContentInfo}
          <div class="vc-loaded-card">
            <div class="vc-loaded-header">
              <div class="vc-loaded-icon">
                {#if actionsContentInfo.type === 'INDEX'}
                  <Layers size={24} />
                {:else}
                  <FileText size={24} />
                {/if}
              </div>
              <div class="vc-loaded-info">
                <h3 class="vc-loaded-name">{actionsContentInfo.name}</h3>
                <div class="vc-loaded-meta">
                  <span class="badge badge-cyan">{actionsContentInfo.type}</span>
                  {#if actionsContentInfo.durl}
                    <code class="vc-loaded-durl">{actionsContentInfo.durl}</code>
                  {/if}
                  {#if actionsContentInfo.docCount}
                    <span class="vc-loaded-docs">{actionsContentInfo.docCount} DOC(s)</span>
                  {/if}
                </div>
                {#if actionsContentInfo.description}
                  <p class="vc-loaded-desc">{actionsContentInfo.description}</p>
                {/if}
              </div>
            </div>
            
            <div class="vc-loaded-scid">
              <code>{actionsContentInfo.scid}</code>
            </div>
            
            <!-- Action Buttons -->
            <div class="vc-actions-grid">
              <button class="btn btn-primary" on:click={() => openVersionHistory(actionsContentInfo.scid)}>
                <History size={16} />
                Version History
              </button>
              <button class="btn btn-secondary" on:click={() => { cloneScid = actionsContentInfo.scid; activeTab = 'clone'; }}>
                <Copy size={16} />
                Clone Latest
              </button>
              <button class="btn btn-secondary" on:click={() => { updateIndexScid = actionsContentInfo.scid; activeTab = 'update-index'; }}>
                <RefreshCw size={16} />
                Update INDEX
              </button>
              <button class="btn btn-secondary" on:click={() => viewMyContentInBrowser(actionsContentInfo.scid)}>
                <Eye size={16} />
                Preview
              </button>
            </div>
          </div>
        {/if}
        
        <!-- Quick Access: Your INDEXes -->
        {#if !$walletState.isOpen}
          <div class="info-panel" style="margin-top: var(--s-4);">
            <div class="info-panel-icon"><Wallet size={16} /></div>
            <div class="info-panel-content">
              <p class="info-panel-title">Connect a wallet to see your INDEXes for quick access.</p>
            </div>
          </div>
        {:else if myIndexes.length > 0}
          <div class="vc-quick-section">
            <h3 class="vc-section-title">
              <Package size={16} />
              Your INDEXes
            </h3>
            <div class="vc-quick-list">
              {#each myIndexes.slice(0, 5) as index}
                <button 
                  class="vc-quick-item"
                  on:click={() => { actionsScid = index.scid; loadActionsContent(); }}
                >
                  <div class="vc-quick-icon">
                    <Layers size={16} />
                  </div>
                  <div class="vc-quick-info">
                    <span class="vc-quick-name">{index.display_name || index.durl || 'INDEX'}</span>
                    <code class="vc-quick-scid">{index.scid.slice(0, 12)}...{index.scid.slice(-8)}</code>
                  </div>
                  <ArrowRight size={16} class="vc-quick-arrow" />
                </button>
              {/each}
            </div>
            
            {#if myIndexes.length > 5}
              <p class="vc-quick-more">
                <a href="#my-content" on:click|preventDefault={() => activeTab = 'my-content'}>
                  View all {myIndexes.length} INDEXes →
                </a>
              </p>
            {/if}
          </div>
        {:else if $walletState.isOpen && myContentLoaded}
          <div class="info-panel" style="margin-top: var(--s-4);">
            <div class="info-panel-icon"><Lightbulb size={16} /></div>
            <div class="info-panel-content">
              <p class="info-panel-title">No INDEXes Found</p>
              <p class="info-panel-text">Deploy your first INDEX to use version control features.</p>
            </div>
          </div>
        {/if}
        
        <!-- How It Works Info Card -->
        <div class="content-card use-cases-card" style="margin-top: var(--s-4);">
          <h4 class="use-cases-title">
            <GitBranch size={14} style="margin-right: var(--s-2);" />
            How TELA Version Control Works
          </h4>
          <ul class="use-cases-list">
            <li><strong>Immutable DOCs</strong> — TELA-DOC-1 contracts are immutable once deployed. The code never changes.</li>
            <li><strong>Mutable INDEXes</strong> — TELA-INDEX-1 contracts (deployed with ringsize 2) can be updated by their owner.</li>
            <li><strong>Commit History</strong> — Each update creates a new "commit" with a TXID. You can view, compare, or revert to any version.</li>
            <li><strong>Clone at Version</strong> — Use <code>scid@txid</code> format to clone content at a specific version.</li>
          </ul>
        </div>
      </div>
    
    {:else if activeTab === 'clone'}
      <div class="content-section">
        <h2 class="content-section-title">Clone TELA Content</h2>
        <p class="content-section-desc">Download TELA content from the blockchain to your local machine.</p>
        
        <!-- Error Display -->
        {#if cloneError}
          <div class="alert alert-error" style="margin-bottom: var(--s-4);">
            <AlertTriangle size={16} />
            <span>{cloneError}</span>
          </div>
        {/if}
        
        <!-- Success Display -->
        {#if cloneResult}
          <div class="clone-success-card">
            <div class="clone-success-header">
              <CheckCircle size={24} class="clone-success-icon" />
              <div>
                <h3 class="clone-success-title">Content Cloned Successfully!</h3>
                <p class="clone-success-subtitle">{cloneResult.contentType}: {cloneResult.name}</p>
              </div>
            </div>
            
            <div class="clone-result-details">
              {#if cloneResult.dURL}
                <div class="clone-detail-row">
                  <span class="clone-detail-label">dURL</span>
                  <code class="clone-detail-value">{cloneResult.dURL}</code>
                </div>
              {/if}
              
              {#if cloneResult.description}
                <div class="clone-detail-row">
                  <span class="clone-detail-label">Description</span>
                  <span class="clone-detail-value">{cloneResult.description}</span>
                </div>
              {/if}
              
              {#if cloneResult.fileCount}
                <div class="clone-detail-row">
                  <span class="clone-detail-label">Files</span>
                  <span class="clone-detail-value">{cloneResult.fileCount} {cloneResult.contentType === 'INDEX' ? 'DOC(s)' : 'file'}</span>
                </div>
              {/if}
              
              <div class="clone-detail-row">
                <span class="clone-detail-label">Location</span>
                <div class="clone-path-row">
                  <code class="clone-path">{cloneResult.directory}</code>
                  <button class="clone-copy-btn" on:click={copyClonePath} title="Copy path">
                    <Clipboard size={14} />
                  </button>
                </div>
              </div>
            </div>
            
            <div class="clone-actions">
              <button class="btn btn-secondary" on:click={openCloneFolder}>
                <FolderDown size={16} />
                Open Folder
              </button>
              <button class="btn btn-secondary" on:click={serveClonedContent}>
                <Server size={16} />
                Serve Content
              </button>
              <button class="btn btn-ghost" on:click={resetClone}>
                Clone Another
              </button>
            </div>
          </div>
        {:else}
          <!-- Clone Input Card -->
          <div class="content-card">
            <div class="content-card-header">
              <Copy size={32} class="content-card-icon" />
              <p class="content-card-title">Clone from Blockchain</p>
              <p class="content-card-text">Enter an SCID to download TELA content (DOC or INDEX) to your local machine.</p>
            </div>
            
            <div class="form-group" style="margin-top: var(--s-4);">
              <label class="form-label">SCID <span class="label-hint">(64 characters, or scid@txid for specific version)</span></label>
              <input
                type="text"
                bind:value={cloneScid}
                placeholder="Enter SCID or scid@txid..."
                class="input input-mono"
                disabled={cloneLoading}
              />
            </div>
            
            <button 
              class="btn btn-primary btn-block" 
              style="margin-top: var(--s-4);"
              on:click={() => cloneContent(false)}
              disabled={cloneLoading || !cloneScid || cloneScid.length < 64}
            >
              {#if cloneLoading}
                <Loader2 size={16} class="spinner" />
                Cloning...
              {:else}
                <FolderDown size={16} />
                Clone Content
              {/if}
            </button>
          </div>
          
          <!-- Clone Info -->
          <div class="info-panel" style="margin-top: var(--s-4);">
            <div class="info-panel-icon">◎</div>
            <div class="info-panel-content">
              <p class="info-panel-title">About Cloning</p>
              <ul class="info-list">
                <li>Content is downloaded to: <code>~/.tela/datashards/clone/</code></li>
                <li>Use <code>scid@txid</code> format to clone a specific version</li>
                <li>You'll be prompted if content has been updated since original deployment</li>
              </ul>
            </div>
          </div>
        {/if}
      </div>
      
      <!-- Clone Update Confirmation Modal -->
      {#if showCloneConfirmModal}
        <div class="modal-overlay" on:click={cancelCloneUpdate}>
          <div class="modal-content" on:click|stopPropagation>
            <div class="modal-header">
              <AlertTriangle size={24} class="modal-icon warning" />
              <h3 class="modal-title">Content Has Been Updated</h3>
            </div>
            
            <div class="modal-body">
              <p>This TELA content has been updated since its original deployment.</p>
              <p style="margin-top: var(--s-3); color: var(--text-3);">
                Do you want to clone the <strong>latest version</strong>?
              </p>
            </div>
            
            <div class="modal-actions">
              <button class="btn btn-ghost" on:click={cancelCloneUpdate}>Cancel</button>
              <button class="btn btn-primary" on:click={confirmCloneUpdate}>
                Clone Latest Version
              </button>
            </div>
          </div>
        </div>
      {/if}
    
    {:else if activeTab === 'serve'}
      <div class="content-section">
        <h2 class="content-section-title">Local Dev Server</h2>
        <p class="content-section-desc">Preview TELA content locally with hot reload before deploying to the blockchain.</p>
        
        {#if !localServerRunning}
          <!-- Select Directory Card -->
          <div class="content-card">
            <div class="content-card-header">
              <Server size={32} class="content-card-icon" />
              <p class="content-card-title">Select a directory to serve</p>
              <p class="content-card-text">
                Choose a folder containing your TELA app (must have index.html).
                Files will be served locally with hot reload enabled.
              </p>
            </div>
            
            <button 
              class="btn btn-primary btn-block" 
              style="margin-top: var(--s-5);"
              on:click={selectAndServeDirectory}
              disabled={serveLoading}
            >
              {#if serveLoading}
                <Loader2 size={14} class="spin" />
                Starting server...
              {:else}
                <FolderUp size={14} />
                Choose Directory
              {/if}
            </button>
            
            {#if serveError}
              <div class="alert-error" style="margin-top: var(--s-4);">
                {serveError}
              </div>
            {/if}
          </div>
          
          <!-- Features Info Card -->
          <div class="content-card use-cases-card" style="margin-top: var(--s-4);">
            <h4 class="use-cases-title">Features</h4>
            <ul class="use-cases-list">
              <li>Local HTTP server serves your files</li>
              <li>Hot reload on file changes (HTML, CSS, JS)</li>
              <li>XSWD works with your connected wallet</li>
              <li>Test wallet interactions before deploying</li>
              <li>No blockchain costs during development</li>
            </ul>
          </div>
        {:else}
          <!-- Server Running Card -->
          <div class="server-running-card">
            <div class="server-status-header">
              <div class="server-status-indicator running"></div>
              <span class="server-status-text">Local Dev Server Running</span>
            </div>
            
            <div class="server-info-grid">
              <div class="server-info-item">
                <span class="server-info-label">URL</span>
                <span class="server-info-value mono">{localServerUrl}</span>
              </div>
              <div class="server-info-item">
                <span class="server-info-label">Port</span>
                <span class="server-info-value mono">{localServerPort}</span>
              </div>
              <div class="server-info-item">
                <span class="server-info-label">Directory</span>
                <span class="server-info-value" title={localServerDirectory}>{formatDirectory(localServerDirectory)}</span>
              </div>
              <div class="server-info-item">
                <span class="server-info-label">Hot Reload</span>
                <span class="server-info-value" class:status-ok={localServerWatcherActive} class:status-warn={!localServerWatcherActive}>
                  {localServerWatcherActive ? 'Active' : 'Inactive'}
                </span>
              </div>
            </div>
            
            <div class="server-actions">
              <button class="btn btn-primary" on:click={openInBrowser}>
                <Eye size={14} />
                Open in Browser
              </button>
              <button class="btn btn-secondary" on:click={triggerManualRefresh}>
                <RefreshCw size={14} />
                Refresh
              </button>
              <button class="btn btn-ghost btn-danger" on:click={stopLocalServer}>
                <Square size={14} />
                Stop Server
              </button>
            </div>
          </div>
          
          <!-- File Changes Log -->
          {#if recentChanges.length > 0}
            <div class="content-card" style="margin-top: var(--s-4);">
              <h4 class="use-cases-title">Recent Changes (Hot Reload)</h4>
              <div class="changes-list">
                {#each recentChanges as change}
                  <div class="change-item">
                    <span class="change-file">{change.file}</span>
                    <span class="change-time">{change.time}</span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}
          
          <!-- XSWD Info Card -->
          <div class="content-card" style="margin-top: var(--s-4);">
            <h4 class="use-cases-title">XSWD Integration</h4>
            <p class="serve-info-text">
              XSWD is available for your local TELA app. Your app can call wallet methods 
              using the standard <code>telaHost</code> bridge. Make sure you have a wallet 
              connected to test wallet interactions.
            </p>
          </div>
        {/if}
      </div>
    
    {:else if activeTab === 'diff'}
      <div class="content-section">
        <h2 class="content-section-title">Code Diff Viewer</h2>
        <p class="content-section-desc">Compare local files or smart contract code to see differences.</p>
        
        <div class="diff-grid">
          <!-- Compare Local Files -->
          <button on:click={() => showDiffViewer = true} class="content-card diff-option">
            <div class="diff-option-head">
              <FileText size={24} class="diff-option-icon cyan" />
              <h3 class="diff-option-title">Local Files</h3>
            </div>
            <p class="diff-option-desc">Compare two files from your computer to see what changed.</p>
          </button>
          
          <!-- Compare Smart Contracts -->
          <button on:click={() => showDiffViewer = true} class="content-card diff-option">
            <div class="diff-option-head">
              <Layers size={24} class="diff-option-icon emerald" />
              <h3 class="diff-option-title">Smart Contracts</h3>
            </div>
            <p class="diff-option-desc">Compare two deployed smart contracts by their SCID.</p>
          </button>
        </div>
        
        <div class="content-card use-cases-card">
          <h4 class="use-cases-title">Use Cases</h4>
          <ul class="use-cases-list">
            <li>Compare your local code with deployed SC code</li>
            <li>Track changes between different versions</li>
            <li>Review differences before updating an INDEX</li>
            <li>Verify code integrity after deployment</li>
          </ul>
        </div>
      </div>
    
    <!-- MODULES SECTION -->
    {:else if activeTab === 'modules'}
      <div class="content-section modules-section">
        <h2 class="content-section-title">DVM Modules</h2>
        <p class="content-section-desc">Browse and install modular DVM code extensions for your smart contracts.</p>
        
        <!-- Search and Filter -->
        <div class="mods-toolbar">
          <label class="search-box">
            <Search size={16} />
            <input
              type="text"
              bind:value={modSearchQuery}
              placeholder="Search modules..."
              class="search-input"
            />
          </label>
          
          <div class="mods-filter-buttons">
            <button
              on:click={() => selectedModClass = 'all'}
              class="mods-filter-btn"
              class:active={selectedModClass === 'all'}
            >
              <Library size={14} />
              All
              <span class="filter-count">({allMods.length})</span>
            </button>
            {#each modClasses as cls}
              <button
                on:click={() => selectedModClass = cls.name}
                class="mods-filter-btn"
                class:active={selectedModClass === cls.name}
              >
                <svelte:component this={getModClassIcon(cls.name)} size={14} />
                {cls.name}
                <span class="filter-count">({cls.modCount})</span>
              </button>
            {/each}
          </div>
        </div>
        
        <!-- Modules Grid -->
        {#if modsLoading}
          <div class="mods-loading">
            <Loader2 size={32} class="spin" />
            <p>Loading modules...</p>
          </div>
        {:else if modsError}
          <div class="mods-error">
            <AlertTriangle size={32} />
            <p>{modsError}</p>
            <button on:click={loadModsData} class="btn btn-secondary">Retry</button>
          </div>
        {:else if filteredMods.length === 0}
          <div class="mods-empty">
            <Puzzle size={32} />
            <p>No modules found</p>
            {#if modSearchQuery || selectedModClass !== 'all'}
              <button on:click={() => { modSearchQuery = ''; selectedModClass = 'all'; }} class="btn btn-ghost">
                Clear filters
              </button>
            {/if}
          </div>
        {:else}
          <div class="mods-grid">
            {#each filteredMods as mod}
              <button class="mod-card" on:click={() => openModDetails(mod)}>
                <div class="mod-card-icon">
                  <svelte:component this={getModClassIcon(mod.class)} size={24} />
                </div>
                <div class="mod-card-content">
                  <div class="mod-card-name">{mod.name}</div>
                  <div class="mod-card-meta">
                    <span class="badge badge-cyan">{mod.class}</span>
                    <span class="mod-card-tag">by {mod.tag}</span>
                  </div>
                  <p class="mod-card-desc">{mod.description || 'No description available'}</p>
                </div>
                <ArrowRight size={16} class="mod-card-arrow" />
              </button>
            {/each}
          </div>
        {/if}
      </div>
    
    <!-- LIBRARIES SECTION -->
    {:else if activeTab === 'libraries'}
      <div class="content-section libraries-section">
        <h2 class="content-section-title">TELA Libraries</h2>
        <p class="content-section-desc">Reusable code libraries deployed to the TELA network.</p>
        
        <!-- Toolbar -->
        <div class="libs-toolbar">
          <label class="search-box">
            <Search size={16} />
            <input
              type="text"
              bind:value={librarySearchQuery}
              placeholder="Search by name, dURL, or description..."
              class="search-input"
              disabled={librariesLoading}
            />
            {#if librarySearchQuery}
              <button class="search-clear" type="button" on:click={() => librarySearchQuery = ''}>
                <X size={14} />
              </button>
            {/if}
          </label>
          
          <button 
            on:click={() => loadLibrariesData(true)} 
            class="btn btn-ghost libs-refresh-btn" 
            disabled={librariesLoading}
            title="Refresh library list"
          >
            <span class:spin={librariesLoading}><RefreshCw size={14} /></span>
            <span class="libs-refresh-text">Refresh</span>
          </button>
        </div>
        
        <!-- Content Area -->
        <div class="libs-content">
          {#if librariesLoading}
            <div class="libs-loading">
              <div class="libs-loading-animation">
                <div class="libs-loading-ring"></div>
                <Library size={32} class="libs-loading-icon" />
              </div>
              <div class="libs-loading-text">
                <p class="libs-loading-title">Loading Libraries</p>
                <p class="libs-loading-status">{librariesLoadingStatus || 'Please wait...'}</p>
              </div>
            </div>
          {:else if librariesError}
            <div class="libs-error">
              <div class="libs-error-icon">
                <AlertTriangle size={40} />
              </div>
              <h3 class="libs-error-title">Unable to Load Libraries</h3>
              <p class="libs-error-message">{librariesError}</p>
              <div class="libs-error-actions">
                {#if gnomonRequired}
                  <button on:click={goToSettings} class="btn btn-primary">
                    <Database size={14} />
                    Go to Settings
                  </button>
                {/if}
                <button on:click={() => loadLibrariesData(true)} class="btn btn-secondary">
                  <RefreshCw size={14} />
                  Try Again
                </button>
              </div>
            </div>
          {:else if libraries.length === 0}
            <!-- Empty State - Using content-card pattern like Clone/Serve/DocShards -->
            <div class="content-card">
              <div class="content-card-header">
                <Library size={32} class="content-card-icon" />
                <p class="content-card-title">No Libraries Found</p>
                <p class="content-card-text">Libraries are TELA content with a <code>.lib</code> suffix in their dURL.</p>
              </div>
              
              <button 
                class="btn btn-primary btn-block" 
                style="margin-top: var(--s-4);"
                on:click={() => loadLibrariesData(true)}
              >
                <RefreshCw size={16} />
                Check Again
              </button>
            </div>
            
            <!-- Info Panel - Using info-panel pattern like Clone/DocShards -->
            <div class="info-panel" style="margin-top: var(--s-4);">
              <div class="info-panel-icon">◎</div>
              <div class="info-panel-content">
                <p class="info-panel-title">About Libraries</p>
                <ul class="info-list">
                  <li>Deploy reusable JavaScript, CSS, or HTML snippets</li>
                  <li>Reference libraries in your TELA apps using dURL</li>
                  <li>Example: <code>mylib.lib</code> or <code>utils.lib</code></li>
                </ul>
              </div>
            </div>
          {:else if filteredLibraries.length === 0}
            <div class="libs-no-results">
              <span class="libs-no-results-icon"><Search size={32} /></span>
              <p>No libraries match "<strong>{librarySearchQuery}</strong>"</p>
              <button on:click={() => librarySearchQuery = ''} class="btn btn-ghost">
                Clear search
              </button>
            </div>
          {:else}
            <div class="libs-results-info">
              Showing {filteredLibraries.length} {filteredLibraries.length === 1 ? 'library' : 'libraries'}
              {#if librarySearchQuery}
                matching "<strong>{librarySearchQuery}</strong>"
              {/if}
            </div>
            <div class="libs-grid">
              {#each filteredLibraries as lib}
                <button class="lib-card" on:click={() => openLibraryDetails(lib)}>
                  <div class="lib-card-header">
                    <div class="lib-card-icon" class:lib-card-icon-index={lib.type === 'INDEX'}>
                      {#if lib.type === 'INDEX'}
                        <Layers size={20} />
                      {:else}
                        <FileText size={20} />
                      {/if}
                    </div>
                    <div class="lib-card-badges">
                      <span class="lib-card-type" class:lib-type-index={lib.type === 'INDEX'}>
                        {lib.type || 'DOC'}
                      </span>
                      {#if lib.rating && lib.rating.count > 0}
                        <span class="lib-card-rating" title="{lib.rating.likes} likes / {lib.rating.dislikes} dislikes">
                          {#if lib.rating.likes > lib.rating.dislikes}
                            <ThumbsUp size={10} />
                          {:else if lib.rating.likes === lib.rating.dislikes}
                            <Minus size={10} />
                          {:else}
                            <ThumbsDown size={10} />
                          {/if}
                          {lib.rating.count}
                        </span>
                      {/if}
                    </div>
                  </div>
                  
                  <div class="lib-card-body">
                    <h3 class="lib-card-name">{lib.display_name || lib.name || 'Unnamed Library'}</h3>
                    {#if lib.durl}
                      <code class="lib-card-durl">{lib.durl}</code>
                    {/if}
                    {#if lib.description}
                      <p class="lib-card-desc">{lib.description.slice(0, 100)}{lib.description.length > 100 ? '...' : ''}</p>
                    {/if}
                  </div>
                  
                  <div class="lib-card-footer">
                    {#if lib.type === 'INDEX' && lib.doc_count > 0}
                      <span class="lib-card-meta">
                        <Package size={12} />
                        {lib.doc_count} file{lib.doc_count > 1 ? 's' : ''}
                      </span>
                    {:else}
                      <span class="lib-card-meta"></span>
                    {/if}
                    <span class="lib-card-arrow-wrap">
                      <span class="lib-card-arrow-text">View</span>
                      <ArrowRight size={14} />
                    </span>
                  </div>
                </button>
              {/each}
            </div>
          {/if}
        </div>
      </div>
    
    {:else if activeTab === 'shards'}
      <!-- DocShards Tools - Inline pattern matching Clone/Serve -->
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
                <label class="form-label shard-checkbox-label">
                  <input type="checkbox" bind:checked={shardCompress} class="shard-checkbox" />
                  Enable GZIP Compression
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
    
    {:else if activeTab === 'deploy-sc'}
      <!-- Deploy Smart Contract - Raw DVM-BASIC code deployment -->
      <div class="content-section">
        <h2 class="content-section-title">Deploy Smart Contract</h2>
        <p class="content-section-desc">Deploy a raw DVM-BASIC smart contract directly to the blockchain.</p>
        
        <!-- Error Display -->
        {#if scDeployError}
          <div class="alert alert-error" style="margin-bottom: var(--s-4);">
            <AlertTriangle size={16} />
            <span>{scDeployError}</span>
          </div>
        {/if}
        
        <!-- Success Display -->
        {#if scDeployResult}
          <div class="clone-success-card">
            <div class="clone-success-header">
              <CheckCircle size={24} class="clone-success-icon" />
              <div>
                <h3 class="clone-success-title">Smart Contract Deployed!</h3>
                <p class="clone-success-subtitle">Transaction submitted successfully</p>
              </div>
            </div>
            
            <div class="clone-result-details">
              <div class="clone-detail-row">
                <span class="clone-detail-label">Transaction ID</span>
                <code class="clone-detail-value" style="font-size: 11px;">{scDeployResult.txid}</code>
              </div>
              <div class="clone-detail-row">
                <span class="clone-detail-label">Status</span>
                <span class="clone-detail-value">Pending confirmation</span>
              </div>
            </div>
            
            <div class="clone-result-note">
              <Info size={14} />
              <span>The SCID will be the same as the TXID once confirmed. Copy the TXID above.</span>
            </div>
            
            <div class="clone-actions">
              <button class="btn btn-secondary" on:click={() => navigator.clipboard.writeText(scDeployResult.txid)}>
                <Copy size={14} />
                Copy TXID
              </button>
              <button class="btn btn-ghost" on:click={resetSCDeploy}>
                Deploy Another
              </button>
            </div>
          </div>
        {:else}
          <!-- Deployment Form -->
          <div class="content-card">
            <div class="content-card-header">
              <FileCode size={32} class="content-card-icon" />
              <p class="content-card-title">DVM-BASIC Code</p>
              <p class="content-card-text">Enter your smart contract code below. The code will be validated before deployment.</p>
            </div>
            
            <div class="form-group" style="margin-top: var(--s-4);">
              <label class="form-label">Smart Contract Code <span class="required">*</span></label>
              <textarea
                bind:value={scCode}
                placeholder="Function Initialize() Uint64
  10 RETURN 0
End Function"
                class="textarea sc-code-textarea"
                rows="15"
                spellcheck="false"
              ></textarea>
              <span class="form-hint">Write or paste your DVM-BASIC smart contract code</span>
            </div>
            
            <div class="form-group" style="margin-top: var(--s-3);">
              <label class="form-label shard-checkbox-label">
                <input type="checkbox" bind:checked={scAnonymous} class="shard-checkbox" />
                Anonymous Deployment (Ring 16+)
              </label>
              <span class="form-hint">Use higher ring size for enhanced privacy. Standard deployment uses Ring 2.</span>
            </div>
            
            <!-- Wallet Check -->
            {#if !$walletState.isOpen && !isSimulator}
              <div class="alert alert-warning" style="margin-top: var(--s-4);">
                <AlertTriangle size={16} />
                <span>Please open a wallet to deploy smart contracts</span>
              </div>
            {/if}
            
            <button 
              class="btn btn-primary btn-block" 
              style="margin-top: var(--s-4);"
              on:click={deploySmartContract}
              disabled={scDeploying || !scCode.trim() || (!$walletState.isOpen && !isSimulator)}
            >
              {#if scDeploying}
                <Loader2 size={16} class="spinner" />
                Deploying...
              {:else}
                <Zap size={16} />
                Deploy Smart Contract
              {/if}
            </button>
          </div>
          
          <!-- Info Panel -->
          <div class="info-panel" style="margin-top: var(--s-4);">
            <div class="info-panel-icon">◎</div>
            <div class="info-panel-content">
              <p class="info-panel-title">About Smart Contract Deployment</p>
              <ul class="info-list">
                <li>Smart contracts are written in DVM-BASIC, a BASIC-like language</li>
                <li>Every SC must have an <code>Initialize()</code> function that returns Uint64</li>
                <li>The SCID (Smart Contract ID) equals the transaction hash</li>
                <li>Anonymous mode uses Ring 16+ for enhanced privacy but costs more gas</li>
              </ul>
            </div>
          </div>
        {/if}
      </div>
    {/if}
    </div>
  </div>
</div>

<!-- Library Details Modal -->
{#if selectedLibrary}
  <div class="modal-overlay" on:click={closeLibraryDetails}>
    <div class="modal-content lib-modal" on:click|stopPropagation>
      <div class="lib-modal-header">
        <div class="lib-modal-header-bg"></div>
        <button on:click={closeLibraryDetails} class="modal-close lib-modal-close">
          <X size={20} />
        </button>
        <div class="lib-modal-header-content">
          <div class="lib-modal-icon" class:lib-modal-icon-index={selectedLibrary.type === 'INDEX'}>
            {#if selectedLibrary.type === 'INDEX'}
              <Layers size={28} />
            {:else}
              <FileText size={28} />
            {/if}
          </div>
          <h2 class="lib-modal-title">{selectedLibrary.display_name || selectedLibrary.name || 'Library'}</h2>
          <div class="lib-modal-badges">
            {#if selectedLibrary.durl}
              <code class="lib-modal-durl">{selectedLibrary.durl}</code>
            {/if}
            <span class="lib-modal-type" class:lib-type-index={selectedLibrary.type === 'INDEX'}>
              {selectedLibrary.type || 'DOC'}
            </span>
          </div>
        </div>
      </div>
      
      <div class="modal-body lib-modal-body">
        {#if selectedLibrary.description}
          <div class="lib-modal-section">
            <p class="lib-modal-description">{selectedLibrary.description}</p>
          </div>
        {/if}
        
        <div class="lib-modal-section">
          <h3 class="lib-modal-section-title">
            <Database size={14} />
            Contract Details
          </h3>
          <div class="lib-details-grid">
            <div class="lib-detail-item lib-detail-full">
              <span class="lib-detail-label">SCID</span>
              <div class="lib-detail-scid-row">
                <code class="lib-detail-scid-value">{selectedLibrary.scid}</code>
                <button 
                  class="lib-copy-btn" 
                  on:click={copyLibraryScid} 
                  title={copiedScid === selectedLibrary.scid ? 'Copied!' : 'Copy SCID'}
                >
                  {#if copiedScid === selectedLibrary.scid}
                    <Check size={12} />
                  {:else}
                    <Copy size={12} />
                  {/if}
                </button>
              </div>
            </div>
            
            {#if selectedLibrary.type === 'INDEX' && selectedLibrary.doc_count > 0}
              <div class="lib-detail-item">
                <span class="lib-detail-label">Files</span>
                <span class="lib-detail-value lib-detail-highlight">
                  {selectedLibrary.doc_count} DOC{selectedLibrary.doc_count > 1 ? 's' : ''}
                </span>
              </div>
            {/if}
            
            <div class="lib-detail-item">
              <span class="lib-detail-label">Author</span>
              <code class="lib-detail-value lib-detail-mono">
                {selectedLibrary.owner?.slice(0, 12)}...{selectedLibrary.owner?.slice(-8)}
              </code>
            </div>
            
            {#if selectedLibrary.rating && selectedLibrary.rating.count > 0}
              <div class="lib-detail-item">
                <span class="lib-detail-label">Community Rating</span>
                <div class="lib-detail-rating">
                  <span class="lib-rating-likes"><ThumbsUp size={12} /> {selectedLibrary.rating.likes || 0}</span>
                  <span class="lib-rating-sep">/</span>
                  <span class="lib-rating-dislikes"><ThumbsDown size={12} /> {selectedLibrary.rating.dislikes || 0}</span>
                  <span class="lib-rating-count">({selectedLibrary.rating.count} votes)</span>
                </div>
              </div>
            {/if}
          </div>
        </div>
        
        <div class="lib-modal-section">
          <h3 class="lib-modal-section-title">
            <Zap size={14} />
            Usage
          </h3>
          <div class="lib-usage-box">
            <p class="lib-usage-hint">Reference this library in your TELA content using:</p>
            <code class="lib-usage-code">tela://{selectedLibrary.durl || selectedLibrary.scid}</code>
          </div>
        </div>
      </div>
      
      <div class="modal-footer lib-modal-footer">
        <button on:click={cloneLibrary} class="btn btn-ghost">
          <FolderDown size={14} />
          Clone
        </button>
        <div class="lib-modal-footer-right">
          <button on:click={closeLibraryDetails} class="btn btn-secondary">Cancel</button>
          <button on:click={embedLibraryInIndex} class="btn btn-secondary lib-embed-btn" title="Add library SCID to Install INDEX DOC references">
            <Link size={14} />
            Embed in INDEX
          </button>
          <button on:click={previewLibrary} class="btn btn-primary">
            <Eye size={14} />
            Open in Browser
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}

<!-- MOD Details Modal -->
{#if selectedMod}
  <div class="modal-overlay" on:click={closeModDetails}>
    <div class="modal-content mod-modal" on:click|stopPropagation>
      <div class="modal-header">
        <div class="modal-header-left">
          <div class="modal-icon">
            <svelte:component this={getModClassIcon(selectedMod.class)} size={24} />
          </div>
          <div>
            <h2 class="modal-title">{selectedMod.name}</h2>
            <div class="modal-meta">
              <span class="modal-tag">{selectedMod.tag}</span>
              <span class="badge badge-cyan">{selectedMod.class}</span>
            </div>
          </div>
        </div>
        <button on:click={closeModDetails} class="modal-close">
          <X size={20} />
        </button>
      </div>
      
      <div class="modal-body">
        {#if loadingModDetails}
          <div class="modal-loading">
            <Loader2 size={24} class="spin" />
          </div>
        {:else if modDetails}
          <div class="modal-section">
            <h3 class="modal-section-title">Description</h3>
            <p class="modal-section-text">{modDetails.description || selectedMod.description || 'No description available'}</p>
          </div>
          
          {#if modDetails.functionNames?.length > 0}
            <div class="modal-section">
              <h3 class="modal-section-title">Functions ({modDetails.functionNames.length})</h3>
              <div class="function-tags">
                {#each modDetails.functionNames as funcName}
                  <span class="function-tag">{funcName}()</span>
                {/each}
              </div>
            </div>
          {/if}
          
          {#if modDetails.functionCode}
            <div class="modal-section">
              <div class="code-header">
                <h3 class="modal-section-title">DVM Code</h3>
                <button on:click={() => copyModCode(modDetails.functionCode, 'MOD code')} class="copy-btn">
                  <Copy size={12} />
                  <span>Copy</span>
                </button>
              </div>
              <pre class="code-block">{modDetails.functionCode}</pre>
            </div>
          {/if}
        {:else}
          <p class="modal-empty">No additional details available</p>
        {/if}
      </div>
      
      <div class="modal-footer">
        <button on:click={closeModDetails} class="btn btn-ghost">Close</button>
        <button on:click={openModInstallWizard} disabled={!modDetails} class="btn btn-primary">
          <Wrench size={14} />
          Install to SC
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- MOD Install Wizard Modal -->
{#if showModInstallWizard && selectedMod}
  <div class="modal-overlay nested" on:click={closeModInstallWizard}>
    <div class="modal-content mod-modal" on:click|stopPropagation>
      <div class="modal-header">
        <div class="modal-header-left">
          <Wrench size={20} class="modal-icon-inline" />
          <div>
            <h2 class="modal-title">Install: {selectedMod.name}</h2>
            <p class="modal-subtitle">Prepare module for installation</p>
          </div>
        </div>
      </div>
      
      <div class="modal-body">
        {#if !modInstallResult}
          <div class="install-form">
            <div class="form-group">
              <label class="form-label">Target Smart Contract SCID</label>
              <input
                type="text"
                bind:value={modInstallScid}
                placeholder="Enter 64-character SCID..."
                class="input input-mono"
              />
              <p class="form-hint">You must be the owner of this SC to install modules</p>
            </div>
            
            {#if modInstallError}
              <div class="install-error">
                <AlertTriangle size={14} />
                <span>{modInstallError}</span>
              </div>
            {/if}
            
            {#if !$walletState.isOpen}
              <div class="install-warning">
                <AlertTriangle size={14} />
                <span>Please open a wallet first</span>
              </div>
            {/if}
          </div>
        {:else}
          <div class="install-success">
            <Check size={16} />
            <div>
              <span class="install-success-title">Module Code Prepared!</span>
              <p class="install-success-text">Copy the updated code and use UPDATE_SC_CODE via XSWD to update your SC.</p>
            </div>
          </div>
          
          {#if modInstallResult.functionNames?.length > 0}
            <div class="modal-section">
              <h4 class="modal-section-title">Added Functions</h4>
              <div class="function-tags">
                {#each modInstallResult.functionNames as funcName}
                  <span class="function-tag">{funcName}()</span>
                {/each}
              </div>
            </div>
          {/if}
          
          <div class="modal-section">
            <div class="code-header">
              <h4 class="modal-section-title">Updated SC Code</h4>
              <button on:click={() => copyModCode(modInstallResult.updatedCode)} class="copy-btn">
                <Copy size={12} />
                <span>Copy Full Code</span>
              </button>
            </div>
            <pre class="code-block small">{modInstallResult.updatedCode?.slice(-500) || ''}...</pre>
            <p class="form-hint">Showing last 500 characters</p>
          </div>
        {/if}
      </div>
      
      <div class="modal-footer">
        <button on:click={closeModInstallWizard} class="btn btn-ghost">
          {modInstallResult ? 'Done' : 'Cancel'}
        </button>
        
        {#if !modInstallResult}
          <button
            on:click={prepareModInstall}
            disabled={modInstallLoading || !modInstallScid || modInstallScid.length < 64 || !$walletState.isOpen}
            class="btn btn-primary"
          >
            {#if modInstallLoading}
              <Loader2 size={14} class="spin" />
              Preparing...
            {:else}
              Prepare Installation
            {/if}
          </button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<!-- Diff Viewer Modal -->
<DiffViewer bind:visible={showDiffViewer} on:close={() => showDiffViewer = false} />

<!-- Version History Modal -->
<VersionHistory 
  scid={versionHistoryScid} 
  bind:show={showVersionHistory}
  on:close={closeVersionHistory}
  on:revert={handleVersionRevert}
  on:clone={handleVersionClone}
/>

<!-- Simulator Confirmation Modal - Hologram v6.1 Style -->
{#if showSimModal}
  <div class="sim-modal-backdrop" on:click={cancelSimModal}>
    <div class="sim-modal-card" on:click|stopPropagation>
      <!-- Status Bar -->
      <div class="sim-modal-status">
        <div class="sim-modal-status-left">
          <span class="sim-modal-status-dot" class:start={simModalAction === 'start'} class:stop={simModalAction === 'stop'}></span>
          <span class="sim-modal-status-text">{simModalAction === 'start' ? 'Confirm' : 'Warning'}</span>
        </div>
        <span class="sim-modal-badge" class:start={simModalAction === 'start'} class:stop={simModalAction === 'stop'}>
          {simModalAction === 'start' ? 'SIMULATOR' : 'STOPPING'}
        </span>
      </div>
      
      <!-- Icon + Title -->
      <div class="sim-modal-header">
        <div class="sim-modal-icon" class:start={simModalAction === 'start'} class:stop={simModalAction === 'stop'}>
          <Gamepad2 size={28} strokeWidth={1.5} />
        </div>
        <h2 class="sim-modal-title">
          {#if simModalAction === 'start'}
            Start Simulator Mode
          {:else}
            Stop Simulator
          {/if}
        </h2>
        <p class="sim-modal-desc">
          {#if simModalAction === 'start'}
            Launch a local test environment for safe development
          {:else}
            Return to production network
          {/if}
        </p>
      </div>
      
      <!-- Content -->
      <div class="sim-modal-body">
        {#if simModalAction === 'start'}
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
            Takes a moment to initialize. Once ready, you can deploy and test TELA apps with zero cost.
          </div>
          
          <div class="sim-modal-note warn">
            Test DERO has no real value. Perfect for learning and experimentation.
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
              <Globe size={14} class="sim-feature-icon" />
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
        <button class="sim-modal-btn secondary" on:click={cancelSimModal} disabled={simIsLoading}>
          Cancel
        </button>
        <button class="sim-modal-btn" class:primary={simModalAction === 'start'} class:warn={simModalAction === 'stop'} on:click={confirmSimModal} disabled={simIsLoading}>
          {#if simIsLoading}
            <Loader2 size={14} class="spin" /> Working...
          {:else if simModalAction === 'start'}
            Start Simulator
          {:else}
            Stop Simulator
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Deployment Confirmation Modal -->
{#if showConfirmModal}
  <div class="sim-modal-backdrop" on:click={cancelConfirmation}>
    <div class="sim-modal-card" on:click|stopPropagation>
      <!-- Status Bar -->
      <div class="sim-modal-status">
        <div class="sim-modal-status-left">
          <span class="sim-modal-status-dot" class:start={true}></span>
          <span class="sim-modal-status-text">Confirm Deployment</span>
        </div>
        <span class="sim-modal-badge warn">
          {confirmModalData?.network?.toUpperCase() || 'MAINNET'}
        </span>
      </div>
      
      <!-- Icon + Title -->
      <div class="sim-modal-header">
        <div class="sim-modal-icon start">
          {#if confirmModalType === 'doc'}
            <FileText size={28} strokeWidth={1.5} />
          {:else}
            <Layers size={28} strokeWidth={1.5} />
          {/if}
        </div>
        <h2 class="sim-modal-title">
          {#if confirmModalType === 'doc'}
            Deploy {confirmModalData?.files?.length || 0} DOC{(confirmModalData?.files?.length || 0) > 1 ? 's' : ''}
          {:else}
            Create INDEX
          {/if}
        </h2>
        <p class="sim-modal-desc">
          This action will deploy to {confirmModalData?.network || 'mainnet'} and cost DERO
        </p>
      </div>
      
      <!-- Content -->
      <div class="sim-modal-body">
        <div class="confirm-details">
          {#if confirmModalType === 'doc' && confirmModalData?.files}
            <div class="confirm-row">
              <span class="confirm-label">Files</span>
              <span class="confirm-value">{confirmModalData.files.length} DOC{confirmModalData.files.length > 1 ? 's' : ''}</span>
            </div>
            <div class="confirm-row">
              <span class="confirm-label">Total Size</span>
              <span class="confirm-value">{formatFileSize(confirmModalData.files.reduce((s, f) => s + f.size, 0))}</span>
            </div>
          {:else if confirmModalType === 'index'}
            <div class="confirm-row">
              <span class="confirm-label">Name</span>
              <span class="confirm-value">{confirmModalData?.name || 'Unnamed'}</span>
            </div>
            <div class="confirm-row">
              <span class="confirm-label">dURL</span>
              <span class="confirm-value c-cyan">dero://{confirmModalData?.durl}</span>
            </div>
            <div class="confirm-row">
              <span class="confirm-label">DOCs</span>
              <span class="confirm-value">{confirmModalData?.docCount || 0} references</span>
            </div>
          {/if}
          <div class="confirm-row">
            <span class="confirm-label">Est. Cost</span>
            <span class="confirm-value c-emerald">~{formatGas(confirmModalData?.gasEstimate || 0)} gas</span>
          </div>
          <div class="confirm-row">
            <span class="confirm-label">Network</span>
            <span class="confirm-value">
              <span class="network-badge {confirmModalData?.network}">{confirmModalData?.network}</span>
            </span>
          </div>
        </div>
        
        <div class="sim-modal-note warn">
          <AlertTriangle size={14} />
          <span>Mainnet transactions are permanent and cost real DERO. Double-check before confirming.</span>
        </div>
        
        <!-- Acknowledgement Checkbox -->
        <label class="deploy-acknowledge">
          <input 
            type="checkbox" 
            bind:checked={deployAcknowledged}
            class="acknowledge-checkbox"
          />
          <span class="acknowledge-text">
            I understand this deployment is <strong>permanent</strong> and will consume <strong>real DERO</strong>
          </span>
        </label>
      </div>
      
      <!-- Actions -->
      <div class="sim-modal-actions">
        <button class="sim-modal-btn secondary" on:click={cancelConfirmation}>
          Cancel
        </button>
        <button 
          class="sim-modal-btn primary" 
          on:click={confirmDeployment}
          disabled={!deployAcknowledged}
        >
          <Lock size={14} />
          Confirm & Deploy
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- MOD Picker Modal for Install INDEX -->
<ModPickerModal 
  show={showModPickerModal}
  selectedVsMod={indexSelectedVsMod}
  selectedTxMods={indexSelectedTxMods}
  on:confirm={handleModPickerConfirm}
  on:close={() => showModPickerModal = false}
/>

<style>
  /* === STUDIO PAGE - v5.6 Unified Framework === */
  /* Base layout uses .page-layout, .page-body, .page-sidebar, .page-content from hologram.css */
  /* Network banner uses .network-banner-v56 from hologram.css */
  
  /* Content Section */
  .content-section {
    max-width: 800px;
  }
  
  /* Section title/desc now use global .content-section-title / .content-section-desc */
  
  /* =====================================================
     Unified Search Box - HOLOGRAM Rulebook §SEARCH BAR
     ===================================================== */
  .search-box {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-3) var(--s-4);
    background: var(--void-deep);
    border: 1px solid var(--border-default);
    border-radius: var(--r-lg);
    cursor: text;
    transition: all 150ms;
    flex: 1;
    max-width: 480px;
  }
  
  .search-box:focus-within {
    border-color: var(--cyan-500);
    box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.15);
  }
  
  .search-box:has(.search-input:disabled) {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .search-box > :global(svg) {
    color: var(--text-4);
    flex-shrink: 0;
  }
  
  .search-input {
    flex: 1;
    padding: 0;
    font-size: 14px;
    font-family: var(--font-mono);
    background: transparent;
    border: none;
    color: var(--text-1);
    outline: none;
  }
  
  .search-input::placeholder {
    color: var(--text-4);
  }
  
  .search-input:disabled {
    cursor: not-allowed;
  }
  
  .search-clear {
    padding: var(--s-1);
    background: transparent;
    border: none;
    color: var(--text-4);
    cursor: pointer;
    border-radius: var(--r-xs);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    transition: all 150ms;
  }
  
  .search-clear:hover {
    color: var(--text-2);
    background: var(--void-up);
  }
  
  /* Batch Upload hint link in description */
  .batch-hint-link {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: var(--s-2, 8px);
    padding: 2px 8px;
    background: rgba(34, 211, 238, 0.08);
    border: 1px solid rgba(34, 211, 238, 0.2);
    border-radius: var(--r-sm, 5px);
    color: var(--cyan-400, #22d3ee);
    font-size: 12px;
    cursor: pointer;
    transition: all 150ms ease-out;
  }
  
  .batch-hint-link:hover {
    background: rgba(34, 211, 238, 0.15);
    border-color: rgba(34, 211, 238, 0.4);
  }
  
  /* Multi-file info banner */
  .multi-file-info {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px) var(--s-4, 16px);
    margin-bottom: var(--s-4, 16px);
    background: rgba(251, 191, 36, 0.08);
    border: 1px solid rgba(251, 191, 36, 0.2);
    border-radius: var(--r-md, 8px);
    font-size: 12px;
    color: var(--status-warn, #fbbf24);
  }
  
  .multi-file-info strong {
    color: var(--text-1, #f8f8fc);
  }
  
  .info-action-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: auto;
    padding: 4px 10px;
    background: rgba(34, 211, 238, 0.12);
    border: 1px solid rgba(34, 211, 238, 0.3);
    border-radius: var(--r-sm, 5px);
    color: var(--cyan-400, #22d3ee);
    font-size: 11px;
    font-weight: 500;
    cursor: pointer;
    transition: all 150ms ease-out;
    white-space: nowrap;
  }
  
  .info-action-btn:hover {
    background: rgba(34, 211, 238, 0.2);
    border-color: rgba(34, 211, 238, 0.5);
  }
  
  /* v6.1 Staged Files */
  .staged-section {
    margin-top: var(--s-6, 24px);
  }
  
  .staged-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
    margin-top: var(--s-4, 16px);
  }
  
  .staged-item {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    padding: var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
  }
  
  .staged-icon {
    color: var(--text-4, #505068);
    flex-shrink: 0;
  }
  
  .staged-info {
    flex: 1;
    min-width: 0;
  }
  
  .staged-name {
    font-size: 12px;
    font-weight: 400;
    color: var(--text-1, #f8f8fc);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .staged-meta {
    font-size: 10px;
    color: var(--text-4, #505068);
    margin-top: 2px;
  }
  
  .staged-remove {
    padding: 4px;
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    border-radius: var(--r-xs, 3px);
    cursor: pointer;
    transition: all 150ms;
  }
  
  .staged-remove:hover {
    color: var(--status-err, #f87171);
    background: rgba(248, 113, 113, 0.1);
  }
  
  /* Enhanced staged item with editable fields */
  .staged-item-enhanced {
    padding: var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
  }
  
  .staged-item-header {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
  }
  
  .staged-item-field {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-top: var(--s-2, 8px);
    padding-top: var(--s-2, 8px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .staged-field-label {
    font-size: 10px;
    color: var(--text-4, #505068);
    width: 50px;
    flex-shrink: 0;
  }
  
  .staged-item-field .input {
    flex: 1;
    font-size: 11px;
    padding: var(--s-1, 4px) var(--s-2, 8px);
    background: var(--void-mid, #12121c);
  }
  
  /* DOC Metadata Section */
  .doc-metadata-section {
    margin-top: var(--s-5, 20px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-md, 8px);
  }
  
  .metadata-title {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
    margin-bottom: var(--s-3, 12px);
  }
  
  .metadata-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-4, 16px);
  }
  
  .metadata-grid .form-group {
    margin-bottom: 0;
  }
  
  /* Small input variant */
  .input-sm {
    font-size: 11px;
    padding: var(--s-1, 4px) var(--s-2, 8px);
  }
  
  /* Ringsize Selector */
  .ringsize-section {
    margin-top: var(--s-4, 16px);
    padding-top: var(--s-4, 16px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .ringsize-options {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3, 12px);
    margin-top: var(--s-3, 12px);
  }
  
  .ringsize-option {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    cursor: pointer;
    transition: all 150ms ease;
    color: var(--text-3, #707088);
  }
  
  .ringsize-option:hover {
    border-color: var(--border-default, rgba(255, 255, 255, 0.1));
    background: var(--void-mid, #12121c);
  }
  
  .ringsize-option.selected {
    border-color: var(--cyan-400, #22d3ee);
    background: rgba(34, 211, 238, 0.08);
    color: var(--text-1, #f8f8fc);
  }
  
  .ringsize-option.selected :global(svg) {
    color: var(--cyan-400, #22d3ee);
  }
  
  .ringsize-label {
    font-size: 12px;
    font-weight: 500;
    color: inherit;
  }
  
  .ringsize-hint {
    font-size: 10px;
    color: var(--text-4, #505068);
    text-align: center;
  }
  
  .ringsize-option.selected .ringsize-hint {
    color: var(--text-3, #707088);
  }
  
  .ringsize-option.disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  .ringsize-option.disabled:hover {
    border-color: var(--border-subtle, rgba(255, 255, 255, 0.06));
    background: var(--void-deep, #08080e);
  }
  
  /* TELA-MODs Section */
  .mods-section {
    margin-top: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
  }
  
  .mods-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-3, 12px);
  }
  
  .mods-header .form-label {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin: 0;
    color: var(--text-2, #a0a0b8);
  }
  
  .mods-header .form-label :global(svg) {
    color: var(--violet-400, #a78bfa);
  }
  
  .mods-toggle {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-1, 4px) var(--s-2, 8px);
    background: transparent;
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-sm, 6px);
    cursor: pointer;
    font-size: 11px;
    color: var(--text-4, #505068);
    transition: all 150ms ease;
  }
  
  .mods-toggle:hover {
    border-color: var(--border-default, rgba(255, 255, 255, 0.1));
  }
  
  .mods-toggle.enabled {
    border-color: var(--violet-400, #a78bfa);
    color: var(--violet-400, #a78bfa);
  }
  
  .mods-toggle-track {
    width: 28px;
    height: 16px;
    background: var(--void-up, #181824);
    border-radius: 8px;
    position: relative;
    transition: background 150ms ease;
  }
  
  .mods-toggle.enabled .mods-toggle-track {
    background: var(--violet-400, #a78bfa);
  }
  
  .mods-toggle-thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 12px;
    height: 12px;
    background: white;
    border-radius: 50%;
    transition: transform 150ms ease;
  }
  
  .mods-toggle.enabled .mods-toggle-thumb {
    transform: translateX(12px);
  }
  
  .mods-header-actions {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .mods-advanced-btn {
    display: flex;
    align-items: center;
    gap: var(--s-1, 4px);
    padding: var(--s-1, 4px) var(--s-2, 8px);
    background: var(--void-up, #181824);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    border-radius: var(--r-sm, 5px);
    color: var(--text-4, #505068);
    font-size: 11px;
    cursor: pointer;
    transition: all 150ms ease;
  }
  
  .mods-advanced-btn:hover {
    background: var(--void-surface, #1e1e2a);
    border-color: var(--violet-500, #8b5cf6);
    color: var(--violet-400, #a78bfa);
  }
  
  .mods-content {
    margin-top: var(--s-3, 12px);
    padding-top: var(--s-3, 12px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .mods-loading {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    color: var(--text-3, #707088);
    font-size: 12px;
  }
  
  .mods-description {
    font-size: 11px;
    color: var(--text-4, #505068);
    margin: 0 0 var(--s-3, 12px) 0;
    line-height: 1.5;
  }
  
  .mods-hint {
    font-size: 11px;
    color: var(--text-4, #505068);
    margin: var(--s-2, 8px) 0 0 0;
  }
  
  .mod-group {
    margin-bottom: var(--s-3, 12px);
  }
  
  .mod-group:last-child {
    margin-bottom: 0;
  }
  
  .mod-group-label {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3, #707088);
    margin-bottom: var(--s-2, 8px);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  
  .mod-group-label :global(svg) {
    color: var(--cyan-400, #22d3ee);
  }
  
  .mod-group-hint {
    font-weight: 400;
    color: var(--text-4, #505068);
    text-transform: none;
    letter-spacing: 0;
  }
  
  .mod-options {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2, 8px);
  }
  
  .mod-option {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-sm, 6px);
    cursor: pointer;
    font-size: 11px;
    color: var(--text-3, #707088);
    transition: all 150ms ease;
  }
  
  .mod-option:hover {
    border-color: var(--border-default, rgba(255, 255, 255, 0.1));
    background: var(--void-up, #181824);
  }
  
  .mod-option.selected {
    border-color: var(--violet-400, #a78bfa);
    background: rgba(167, 139, 250, 0.1);
    color: var(--text-1, #f8f8fc);
  }
  
  .mod-option-tag {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 10px;
    padding: 2px 6px;
    background: var(--void-deep, #08080e);
    border-radius: 4px;
    color: var(--violet-400, #a78bfa);
  }
  
  .mod-option.selected .mod-option-tag {
    background: rgba(167, 139, 250, 0.2);
  }
  
  .mod-option-name {
    white-space: nowrap;
  }
  
  .mod-option :global(.mod-check) {
    color: var(--violet-400, #a78bfa);
  }
  
  .mods-summary {
    margin-top: var(--s-3, 12px);
    padding-top: var(--s-3, 12px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .mods-summary-label {
    font-size: 11px;
    color: var(--text-4, #505068);
  }
  
  .mods-summary-tags {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 11px;
    padding: 4px 8px;
    background: rgba(167, 139, 250, 0.1);
    border-radius: 4px;
    color: var(--violet-400, #a78bfa);
  }
  
  /* Compression Toggle (matching tela-cli) */
  .compression-section {
    margin-top: var(--s-4, 16px);
    padding-top: var(--s-4, 16px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .compression-toggle {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    width: 100%;
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    cursor: pointer;
    transition: all 150ms ease;
    text-align: left;
    margin-top: var(--s-2, 8px);
  }
  
  .compression-toggle:hover {
    border-color: var(--border-default, rgba(255, 255, 255, 0.1));
    background: var(--void-mid, #12121c);
  }
  
  .compression-toggle.enabled {
    border-color: var(--cyan-400, #22d3ee);
    background: rgba(34, 211, 238, 0.08);
  }
  
  .compression-toggle-track {
    width: 36px;
    height: 20px;
    background: var(--void-up, #181824);
    border-radius: 10px;
    position: relative;
    transition: background 150ms ease;
    flex-shrink: 0;
  }
  
  .compression-toggle.enabled .compression-toggle-track {
    background: var(--cyan-400, #22d3ee);
  }
  
  .compression-toggle-thumb {
    position: absolute;
    top: 2px;
    left: 2px;
    width: 16px;
    height: 16px;
    background: var(--text-4, #484858);
    border-radius: 50%;
    transition: all 150ms ease;
  }
  
  .compression-toggle.enabled .compression-toggle-thumb {
    left: 18px;
    background: #fff;
  }
  
  .compression-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .compression-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2, #c8c8d8);
  }
  
  .compression-toggle.enabled .compression-label {
    color: var(--cyan-400, #22d3ee);
  }
  
  .compression-hint {
    font-size: 11px;
    color: var(--text-4, #484858);
  }
  
  .compression-note {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-top: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(34, 211, 238, 0.05);
    border-radius: var(--r-sm, 4px);
    font-size: 11px;
    color: var(--cyan-400, #22d3ee);
  }
  
  .compression-note :global(svg) {
    flex-shrink: 0;
    opacity: 0.8;
  }
  
  /* v6.1 Gas Estimate */
  .gas-estimate {
    margin-top: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-lg, 12px);
  }
  
  .gas-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
  }
  
  .gas-value {
    font-size: 1.125rem;
    font-weight: 300;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    margin-top: var(--s-1, 4px);
  }
  
  .gas-value.loading {
    color: var(--text-3, #707088);
  }
  
  .gas-dero {
    font-size: 10px;
    color: var(--text-4, #505068);
    margin-top: 2px;
  }
  
  .gas-size {
    font-size: 13px;
    font-weight: 300;
    color: var(--text-2, #a8a8b8);
    margin-top: var(--s-1, 4px);
  }
  
  .gas-icon {
    color: var(--text-4, #505068);
  }
  
  .gas-warning {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-top: var(--s-3, 12px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(251, 191, 36, 0.08);
    border: 1px solid rgba(251, 191, 36, 0.2);
    border-radius: var(--r-sm, 5px);
    font-size: 11px;
    color: var(--status-warn, #fbbf24);
  }
  
  .gas-note {
    font-size: 11px;
    color: var(--text-4, #505068);
    margin-top: var(--s-3, 12px);
  }
  
  /* Simulator Mode Styles */
  .gas-estimate.simulator-mode {
    background: rgba(167, 139, 250, 0.08);
    border-color: rgba(167, 139, 250, 0.3);
  }
  
  .gas-free {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    color: var(--violet-400, #a78bfa) !important;
    font-weight: 600;
  }
  
  .simulator-note {
    color: var(--violet-400, #a78bfa) !important;
    opacity: 0.8;
  }
  
  .simulator-info {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-top: var(--s-3, 12px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(167, 139, 250, 0.08);
    border: 1px solid rgba(167, 139, 250, 0.2);
    border-radius: var(--r-sm, 5px);
    font-size: 11px;
    color: var(--violet-400, #a78bfa);
  }
  
  .btn-simulator {
    background: var(--violet-500, #8b5cf6) !important;
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
  }
  
  .btn-simulator:hover {
    background: var(--violet-400, #a78bfa) !important;
  }
  
  .simulator-badge-small {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 11px;
    color: var(--violet-400, #a78bfa);
    padding: var(--s-1, 4px) var(--s-2, 8px);
    background: rgba(167, 139, 250, 0.1);
    border-radius: var(--r-sm, 5px);
  }
  
  /* Deploy Row */
  .deploy-row {
    display: flex;
    align-items: center;
    gap: var(--s-4, 16px);
    margin-top: var(--s-6, 24px);
  }
  
  .wallet-warning {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    font-size: 12px;
    color: var(--status-warn, #fbbf24);
  }
  
  .wallet-warning.centered {
    justify-content: center;
    width: 100%;
    margin-top: var(--s-3, 12px);
  }
  
  /* === v6.1 Unified Patterns === */
  
  /* Spin animation for loaders */
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
  
  /* Form patterns - Use global .form-group, .form-label from hologram.css */
  
  /* Studio-specific form stack layout */
  .form-stack {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  /* v6.1 Diff Grid */
  .diff-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-4);
    margin-bottom: var(--s-4);
  }
  
  /* Diff Option (uses .content-card base from hologram.css) */
  .diff-option {
    text-align: left;
    cursor: pointer;
    transition: all var(--dur-fast);
  }
  
  .diff-option:hover {
    border-color: var(--border-default);
    background: var(--void-up);
  }
  
  .diff-option-head {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    margin-bottom: var(--s-3);
  }
  
  :global(.diff-option-icon) {
    color: var(--text-4);
    transition: color var(--dur-fast);
  }
  
  :global(.diff-option-icon.cyan) { }
  .diff-option:hover :global(.diff-option-icon.cyan) { color: var(--cyan-400); }
  :global(.diff-option-icon.emerald) { }
  .diff-option:hover :global(.diff-option-icon.emerald) { color: var(--emerald-400); }
  
  .diff-option-title {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-1);
    transition: color var(--dur-fast);
  }
  
  .diff-option:hover .diff-option-title {
    color: var(--cyan-400);
  }
  
  .diff-option-desc {
    font-size: 12px;
    color: var(--text-3);
    line-height: 1.5;
  }
  
  /* Use Cases Card */
  .use-cases-card {
    text-align: left;
  }
  
  .use-cases-title {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3);
    margin-bottom: var(--s-3);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  
  .use-cases-list {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .use-cases-list li {
    font-size: 12px;
    font-weight: 300;
    color: var(--text-4, #505068);
    padding-left: var(--s-3, 12px);
    position: relative;
  }
  
  .use-cases-list li::before {
    content: '•';
    position: absolute;
    left: 0;
    color: var(--text-5, #404058);
  }
  
  /* v6.1 Alert & Status Styles */
  .deployment-status {
    margin-top: var(--s-4, 16px);
    padding: var(--s-3, 12px);
    border-radius: var(--r-md, 8px);
    font-size: 13px;
  }
  
  .deployment-status-error {
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err, #f87171);
  }
  
  .deployment-status-success {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok, #34d399);
  }
  
  .deployment-status-info {
    background: rgba(34, 211, 238, 0.15);
    color: var(--cyan-400, #22d3ee);
  }
  
  /* === Deployment Success Card (Route 1 Enhancement) === */
  .deployment-success-card {
    margin-top: var(--s-4, 16px);
    background: var(--bg-2, #12121a);
    border: 1px solid rgba(52, 211, 153, 0.3);
    border-radius: var(--r-lg, 12px);
    overflow: hidden;
  }
  
  .success-header {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    padding: var(--s-4, 16px);
    background: rgba(52, 211, 153, 0.1);
    border-bottom: 1px solid rgba(52, 211, 153, 0.15);
  }
  
  .success-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    background: rgba(52, 211, 153, 0.2);
    border-radius: var(--r-md, 8px);
    color: var(--status-ok, #34d399);
  }
  
  .success-info {
    flex: 1;
  }
  
  .success-title {
    margin: 0 0 4px 0;
    font-size: 15px;
    font-weight: 600;
    color: var(--status-ok, #34d399);
  }
  
  .success-meta {
    margin: 0;
    font-size: 12px;
    color: var(--text-3, #888898);
  }
  
  .network-badge {
    display: inline-block;
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }
  
  .network-badge.simulator {
    background: rgba(167, 139, 250, 0.2);
    color: var(--violet-400, #a78bfa);
  }
  
  .network-badge.mainnet {
    background: rgba(52, 211, 153, 0.2);
    color: var(--status-ok, #34d399);
  }
  
  .network-badge.testnet {
    background: rgba(34, 211, 238, 0.2);
    color: var(--cyan-400, #22d3ee);
  }
  
  .success-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: transparent;
    border: none;
    border-radius: var(--r-sm, 6px);
    color: var(--text-3, #888898);
    cursor: pointer;
    transition: all var(--dur-fast);
  }
  
  .success-close:hover {
    background: rgba(255, 255, 255, 0.1);
    color: var(--text-1, #fff);
  }
  
  .deployed-item {
    padding: var(--s-4, 16px);
    border-bottom: 1px solid rgba(255, 255, 255, 0.05);
  }
  
  .deployed-item:last-child {
    border-bottom: none;
  }
  
  .deployed-file-info {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-3, 12px);
  }
  
  .deployed-icon {
    color: var(--cyan-400, #22d3ee);
  }
  
  .deployed-details {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  
  .deployed-name {
    font-weight: 500;
    color: var(--text-1, #fff);
  }
  
  .deployed-size {
    font-size: 11px;
    color: var(--text-4, #606078);
  }
  
  .deployed-scid-row {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-bottom: var(--s-3, 12px);
    padding: var(--s-3, 12px);
    background: rgba(0, 0, 0, 0.3);
    border-radius: var(--r-sm, 6px);
    overflow: hidden;
  }
  
  .scid-label {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-4, #606078);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    flex-shrink: 0;
  }
  
  .scid-value {
    font-family: 'JetBrains Mono', monospace;
    font-size: 11px;
    color: var(--cyan-400, #22d3ee);
    word-break: break-all;
    flex: 1;
  }
  
  .deployed-actions {
    display: flex;
    gap: var(--s-2, 8px);
  }
  
  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1, 4px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: rgba(255, 255, 255, 0.05);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: var(--r-sm, 6px);
    font-size: 12px;
    font-weight: 500;
    color: var(--text-2, #ccc);
    cursor: pointer;
    transition: all var(--dur-fast);
  }
  
  .action-btn:hover {
    background: rgba(255, 255, 255, 0.1);
    border-color: rgba(255, 255, 255, 0.2);
    color: var(--text-1, #fff);
  }
  
  .action-btn.copy-btn:hover {
    border-color: rgba(167, 139, 250, 0.5);
    color: var(--violet-400, #a78bfa);
  }
  
  .action-btn.preview-btn:hover {
    border-color: rgba(34, 211, 238, 0.5);
    color: var(--cyan-400, #22d3ee);
  }
  
  .success-note {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px) var(--s-4, 16px);
    background: rgba(167, 139, 250, 0.1);
    font-size: 12px;
    color: var(--violet-400, #a78bfa);
  }
  
  .alert-error {
    margin-top: var(--s-4, 16px);
    padding: var(--s-3, 12px);
    background: rgba(248, 113, 113, 0.15);
    border: 1px solid rgba(248, 113, 113, 0.3);
    border-radius: var(--r-md, 8px);
    color: var(--status-err, #f87171);
    font-size: 13px;
    text-align: center;
  }
  
  .alert-warning {
    padding: var(--s-3, 12px);
    background: rgba(251, 191, 36, 0.15);
    border: 1px solid rgba(251, 191, 36, 0.3);
    border-radius: var(--r-md, 8px);
  }
  
  .alert-text {
    font-size: 13px;
    color: var(--status-warn, #fbbf24);
  }
  
  .alert-subtext {
    font-size: 11px;
    color: rgba(251, 191, 36, 0.7);
    margin-top: var(--s-1, 4px);
  }
  
  .update-result {
    padding: var(--s-3, 12px);
    border-radius: var(--r-md, 8px);
    font-size: 13px;
  }
  
  .update-result-success {
    background: rgba(52, 211, 153, 0.15);
    border: 1px solid rgba(52, 211, 153, 0.3);
    color: var(--status-ok, #34d399);
  }
  
  .update-result-error {
    background: rgba(248, 113, 113, 0.15);
    border: 1px solid rgba(248, 113, 113, 0.3);
    color: var(--status-err, #f87171);
  }
  
  .wallet-warning {
    font-size: 13px;
    color: var(--status-warn, #fbbf24);
    text-align: center;
  }
  
  .remove-btn {
    padding: var(--s-1, 4px);
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    cursor: pointer;
    border-radius: var(--r-xs, 3px);
    transition: color 150ms;
  }
  
  .remove-btn:hover {
    color: var(--status-err, #f87171);
  }
  
  /* DOC List */
  .doc-item {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
  }
  
  .doc-add-row {
    display: flex;
    gap: var(--s-2, 8px);
    padding-top: var(--s-3, 12px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.03));
  }
  
  .input-mono {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
  }
  
  .c-text-4 {
    color: var(--text-4, #505068);
  }
  
  /* v6.1 Studio Styles */
  .textarea {
    resize: vertical;
    min-height: 100px;
  }
  
  .gas-value-ok {
    color: var(--emerald-400, #34d399);
  }
  
  /* SCID Input uses .scid-input-group from hologram.css */
  
  /* INDEX Info Display */
  .index-info-display {
    display: flex;
    flex-direction: column;
    gap: var(--s-6, 24px);
  }
  
  .index-info-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  
  .index-info-name {
    font-size: 1.25rem;
    font-weight: 700;
    color: var(--text-1, #f8f8fc);
  }
  
  .index-info-scid {
    font-size: 12px;
    color: var(--text-4, #505068);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
  }
  
  .index-details-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-4, 16px);
    margin-bottom: var(--s-3, 12px);
  }
  
  .index-detail {
    margin-bottom: var(--s-3, 12px);
  }
  
  .index-detail-label {
    font-size: 9px;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: var(--text-4, #505068);
    margin-bottom: var(--s-1, 4px);
  }
  
  .index-detail-value {
    font-size: 13px;
    color: var(--text-2, #a8a8b8);
  }
  
  .index-detail-durl {
    color: var(--cyan-400, #22d3ee);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
  }
  
  .index-detail-desc {
    font-size: 12px;
    color: var(--text-3, #707088);
  }
  
  /* DOCs List */
  .docs-list-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-4, 16px);
  }
  
  .docs-list-title {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-2, #a8a8b8);
  }
  
  .docs-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
    margin-bottom: var(--s-4, 16px);
    max-height: 200px;
    overflow-y: auto;
  }
  
  .doc-item-num {
    font-size: 11px;
    color: var(--text-4, #505068);
    width: 24px;
    flex-shrink: 0;
  }
  
  .doc-item-scid {
    flex: 1;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 11px;
    color: var(--text-3, #707088);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  
  .docs-list-empty {
    font-size: 12px;
    color: var(--text-4, #505068);
    text-align: center;
    padding: var(--s-4, 16px);
  }
  
  .submit-row {
    display: flex;
    gap: var(--s-4, 16px);
  }
  
  .back-link {
    margin-top: var(--s-4, 16px);
  }
  
  /* === Local Dev Server Styles === */
  .server-running-card {
    padding: var(--s-5, 20px);
    background: var(--void-mid, #12121c);
    border: 1px solid var(--emerald-500, #10b981);
    border-radius: var(--r-lg, 12px);
  }
  
  .server-status-header {
    display: flex;
    align-items: center;
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-5, 20px);
  }
  
  .server-status-indicator {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    background: var(--text-4, #505068);
  }
  
  .server-status-indicator.running {
    background: var(--emerald-400, #34d399);
    box-shadow: 0 0 8px var(--emerald-400, #34d399);
    animation: pulse-glow 2s ease-in-out infinite;
  }
  
  @keyframes pulse-glow {
    0%, 100% { opacity: 1; box-shadow: 0 0 8px var(--emerald-400, #34d399); }
    50% { opacity: 0.7; box-shadow: 0 0 4px var(--emerald-400, #34d399); }
  }
  
  .server-status-text {
    font-size: 14px;
    font-weight: 500;
    color: var(--emerald-400, #34d399);
  }
  
  .server-info-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--s-3, 12px);
    margin-bottom: var(--s-5, 20px);
  }
  
  .server-info-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
  }
  
  .server-info-label {
    font-size: 10px;
    color: var(--text-4, #505068);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  
  .server-info-value {
    font-size: 12px;
    color: var(--text-2, #a8a8b8);
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .server-info-value.mono {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--cyan-400, #22d3ee);
  }
  
  .server-info-value.status-ok {
    color: var(--emerald-400, #34d399);
  }
  
  .server-info-value.status-warn {
    color: var(--status-warn, #fbbf24);
  }
  
  .server-actions {
    display: flex;
    gap: var(--s-3, 12px);
    flex-wrap: wrap;
  }
  
  .btn-danger {
    color: var(--status-err, #f87171);
  }
  
  .btn-danger:hover {
    background: rgba(248, 113, 113, 0.1);
    color: var(--status-err, #f87171);
  }
  
  .changes-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .change-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border-radius: var(--r-sm, 5px);
    border-left: 2px solid var(--cyan-500, #06b6d4);
  }
  
  .change-file {
    font-size: 12px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-2, #a8a8b8);
  }
  
  .change-time {
    font-size: 10px;
    color: var(--text-4, #505068);
  }
  
  .serve-info-text {
    font-size: 12px;
    color: var(--text-3, #707088);
    line-height: 1.6;
  }
  
  .serve-info-text code {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    background: var(--void-deep, #08080e);
    padding: 2px 6px;
    border-radius: var(--r-xs, 3px);
    color: var(--cyan-400, #22d3ee);
  }
  
  /* === MODULES SECTION === */
  .modules-section {
    max-width: 100%;
  }
  
  .mods-toolbar {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
    margin-bottom: var(--s-5, 20px);
  }
  
  /* Mods search uses unified .search-box */
  
  .mods-filter-buttons {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2, 8px);
  }
  
  .mods-filter-btn {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-2, 8px) var(--s-3, 12px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    color: var(--text-3, #707088);
    font-size: 12px;
    cursor: pointer;
    transition: all 150ms ease;
  }
  
  .mods-filter-btn:hover {
    border-color: var(--border-default, rgba(255, 255, 255, 0.09));
    color: var(--text-2, #a8a8b8);
  }
  
  .mods-filter-btn.active {
    background: rgba(6, 182, 212, 0.1);
    border-color: var(--cyan-500, #06b6d4);
    color: var(--cyan-400, #22d3ee);
  }
  
  .filter-count {
    font-size: 10px;
    color: var(--text-4, #505068);
  }
  
  .mods-loading,
  .mods-error,
  .mods-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-10, 40px);
    color: var(--text-4, #505068);
    gap: var(--s-4, 16px);
    min-height: 200px;
  }
  
  .mods-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: var(--s-4, 16px);
  }
  
  .mod-card {
    display: flex;
    align-items: flex-start;
    gap: var(--s-4, 16px);
    padding: var(--s-4, 16px);
    background: var(--void-mid, #101018);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-lg, 12px);
    cursor: pointer;
    transition: all 150ms ease;
    text-align: left;
  }
  
  .mod-card:hover {
    border-color: var(--cyan-500, #06b6d4);
    background: rgba(6, 182, 212, 0.03);
  }
  
  .mod-card-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    color: var(--cyan-400, #22d3ee);
    flex-shrink: 0;
  }
  
  .mod-card-content {
    flex: 1;
    min-width: 0;
  }
  
  .mod-card-name {
    font-size: 14px;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
    margin-bottom: var(--s-1, 4px);
  }
  
  .mod-card-meta {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-bottom: var(--s-2, 8px);
  }
  
  .mod-card-tag {
    font-size: 11px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-4, #505068);
  }
  
  .mod-card-desc {
    font-size: 12px;
    color: var(--text-3, #707088);
    line-height: 1.5;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  
  :global(.mod-card-arrow) {
    color: var(--text-4, #505068);
    flex-shrink: 0;
    margin-top: var(--s-4, 16px);
  }
  
  .mod-card:hover :global(.mod-card-arrow) {
    color: var(--cyan-400, #22d3ee);
  }
  
  /* MOD Modal */
  .mod-modal {
    max-width: 700px;
  }
  
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    backdrop-filter: blur(4px);
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-4, 16px);
  }
  
  .modal-overlay.nested {
    z-index: 60;
    background: rgba(0, 0, 0, 0.7);
  }
  
  .modal-content {
    background: var(--void-mid, #101018);
    border: 1px solid var(--border-subtle, rgba(255, 255, 255, 0.08));
    border-radius: var(--r-xl, 16px);
    width: 100%;
    max-height: 85vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  
  .modal-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    padding: var(--s-5, 20px);
    border-bottom: 1px solid var(--border-dim, rgba(255, 255, 255, 0.06));
  }
  
  .modal-header-left {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
  }
  
  .modal-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    background: var(--void-deep, #08080e);
    border-radius: var(--r-md, 8px);
    color: var(--cyan-400, #22d3ee);
    flex-shrink: 0;
  }
  
  :global(.modal-icon-inline) {
    color: var(--cyan-400, #22d3ee);
    flex-shrink: 0;
  }
  
  .modal-title {
    font-size: 1rem;
    font-weight: 500;
    color: var(--text-1, #f8f8fc);
    margin: 0;
  }
  
  .modal-subtitle {
    font-size: 12px;
    color: var(--text-4, #505068);
    margin: 0;
  }
  
  .modal-meta {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    margin-top: var(--s-1, 4px);
  }
  
  .modal-tag {
    font-size: 11px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    color: var(--text-4, #505068);
  }
  
  .modal-close {
    padding: var(--s-2, 8px);
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    border-radius: var(--r-sm, 5px);
    cursor: pointer;
    transition: all 150ms ease;
  }
  
  .modal-close:hover {
    color: var(--text-1, #f8f8fc);
    background: rgba(255, 255, 255, 0.05);
  }
  
  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: var(--s-5, 20px);
  }
  
  .modal-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--s-10, 40px);
    color: var(--text-4, #505068);
  }
  
  .modal-empty {
    text-align: center;
    color: var(--text-4, #505068);
    padding: var(--s-8, 32px);
  }
  
  .modal-section {
    margin-bottom: var(--s-5, 20px);
  }
  
  .modal-section:last-child {
    margin-bottom: 0;
  }
  
  .modal-section-title {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3, #707088);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin-bottom: var(--s-2, 8px);
  }
  
  .modal-section-text {
    font-size: 13px;
    color: var(--text-2, #a8a8b8);
    line-height: 1.6;
  }
  
  .modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4, 16px) var(--s-5, 20px);
    border-top: 1px solid var(--border-dim, rgba(255, 255, 255, 0.06));
    background: var(--void-deep, #08080e);
  }
  
  .function-tags {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s-2, 8px);
  }
  
  .function-tag {
    font-size: 11px;
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    padding: 4px 10px;
    background: rgba(167, 139, 250, 0.15);
    color: var(--violet-400, #a78bfa);
    border-radius: var(--r-full, 9999px);
  }
  
  .code-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--s-2, 8px);
  }
  
  .copy-btn {
    display: flex;
    align-items: center;
    gap: var(--s-1, 4px);
    font-size: 10px;
    color: var(--text-4, #505068);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: color 150ms ease;
  }
  
  .copy-btn:hover {
    color: var(--cyan-400, #22d3ee);
  }
  
  .code-block {
    padding: var(--s-4, 16px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-dim, rgba(255, 255, 255, 0.06));
    border-radius: var(--r-md, 8px);
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
    font-size: 12px;
    color: var(--text-2, #a8a8b8);
    line-height: 1.6;
    overflow-x: auto;
    max-height: 240px;
    white-space: pre-wrap;
  }
  
  .code-block.small {
    font-size: 10px;
    max-height: 160px;
  }
  
  /* Install form */
  .install-form {
    display: flex;
    flex-direction: column;
    gap: var(--s-4, 16px);
  }
  
  .form-group {
    display: flex;
    flex-direction: column;
    gap: var(--s-2, 8px);
  }
  
  .form-label {
    font-size: 11px;
    font-weight: 500;
    color: var(--text-3, #707088);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  
  .form-hint {
    font-size: 11px;
    color: var(--text-4, #505068);
    margin-top: var(--s-1, 4px);
  }
  
  /* Icon URL Validation Styles */
  .icon-url-input-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }
  
  .icon-url-input-wrapper .input {
    padding-right: 36px;
  }
  
  .icon-url-status {
    position: absolute;
    right: 12px;
    display: flex;
    align-items: center;
    pointer-events: none;
  }
  
  .icon-url-status.valid {
    color: var(--status-ok, #34d399);
  }
  
  .icon-url-status.warning {
    color: var(--status-warn, #fbbf24);
  }
  
  .icon-url-status.invalid {
    color: var(--status-err, #f87171);
  }
  
  .input.input-valid {
    border-color: var(--status-ok, #34d399);
  }
  
  .input.input-warning {
    border-color: var(--status-warn, #fbbf24);
  }
  
  .input.input-error {
    border-color: var(--status-err, #f87171);
  }
  
  .form-hint.hint-valid {
    color: var(--status-ok, #34d399);
  }
  
  .form-hint.hint-warning {
    color: var(--status-warn, #fbbf24);
  }
  
  .form-hint.hint-error {
    color: var(--status-err, #f87171);
  }
  
  /* dURL Tag Detection Styles */
  .durl-tag-badge {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    margin-left: var(--s-2, 8px);
    font-size: 10px;
    font-weight: 500;
    border-radius: var(--r-sm, 5px);
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-3, #707088);
    vertical-align: middle;
  }
  
  .durl-tag-badge.tag-violet {
    background: rgba(139, 92, 246, 0.15);
    color: var(--violet-400, #a78bfa);
  }
  
  .durl-tag-badge.tag-cyan {
    background: rgba(6, 182, 212, 0.15);
    color: var(--cyan-400, #22d3ee);
  }
  
  .durl-tag-badge.tag-amber {
    background: rgba(251, 191, 36, 0.15);
    color: var(--status-warn, #fbbf24);
  }
  
  .tag-icon {
    font-size: 11px;
  }
  
  .durl-tag-hint.hint-violet {
    color: var(--violet-400, #a78bfa);
  }
  
  .durl-tag-hint.hint-cyan {
    color: var(--cyan-400, #22d3ee);
  }
  
  .durl-tag-hint.hint-amber {
    color: var(--status-warn, #fbbf24);
  }
  
  .install-error,
  .install-warning {
    display: flex;
    align-items: center;
    gap: var(--s-2, 8px);
    padding: var(--s-3, 12px);
    border-radius: var(--r-md, 8px);
    font-size: 12px;
  }
  
  .install-error {
    background: rgba(248, 113, 113, 0.08);
    border: 1px solid rgba(248, 113, 113, 0.2);
    color: var(--status-err, #f87171);
  }
  
  .install-warning {
    background: rgba(251, 191, 36, 0.08);
    border: 1px solid rgba(251, 191, 36, 0.2);
    color: var(--status-warn, #fbbf24);
  }
  
  .install-success {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
    padding: var(--s-4, 16px);
    background: rgba(52, 211, 153, 0.08);
    border: 1px solid rgba(52, 211, 153, 0.2);
    border-radius: var(--r-md, 8px);
    color: var(--status-ok, #34d399);
    margin-bottom: var(--s-4, 16px);
  }
  
  .install-success-title {
    font-weight: 500;
    display: block;
  }
  
  .install-success-text {
    font-size: 12px;
    color: var(--text-3, #707088);
    margin-top: var(--s-1, 4px);
  }
  
  /* Sidebar item count */
  .page-sidebar-item-count {
    font-size: 10px;
    color: var(--text-4, #505068);
    margin-left: auto;
  }
  
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
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
    gap: var(--s-2);
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
  
  .sim-modal-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  
  .sim-modal-btn.secondary {
    background: var(--void-up);
    color: var(--text-3);
    border: 1px solid var(--border-subtle);
  }
  
  .sim-modal-btn.secondary:hover:not(:disabled) {
    background: var(--void-surface);
    color: var(--text-1);
    border-color: var(--border-default);
  }
  
  .sim-modal-btn.primary {
    background: var(--status-ok);
    color: var(--void-pure);
  }
  
  .sim-modal-btn.primary:hover:not(:disabled) {
    filter: brightness(1.1);
    box-shadow: 0 0 16px rgba(52, 211, 153, 0.4);
    transform: translateY(-1px);
  }
  
  .sim-modal-btn.warn {
    background: var(--status-warn);
    color: var(--void-pure);
  }
  
  .sim-modal-btn.warn:hover:not(:disabled) {
    filter: brightness(1.1);
    box-shadow: 0 0 16px rgba(251, 191, 36, 0.4);
    transform: translateY(-1px);
  }
  
  /* Confirmation Modal Details */
  .confirm-details {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding: var(--s-3);
    background: var(--void-deep);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-md);
    margin-bottom: var(--s-3);
  }
  
  .confirm-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 12px;
  }
  
  .confirm-label {
    color: var(--text-4);
  }
  
  .confirm-value {
    color: var(--text-2);
    font-weight: 500;
  }
  
  .confirm-value.c-cyan {
    color: var(--cyan-400);
  }
  
  .confirm-value.c-emerald {
    color: var(--emerald-400);
  }
  
  .network-badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: var(--r-sm);
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
  }
  
  .network-badge.mainnet {
    background: rgba(248, 113, 113, 0.15);
    color: var(--status-err);
  }
  
  .network-badge.testnet {
    background: rgba(251, 191, 36, 0.15);
    color: var(--status-warn);
  }
  
  .network-badge.simulator {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok);
  }
  
  .sim-modal-note.warn {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
  }
  
  /* Deploy Acknowledgement Checkbox */
  .deploy-acknowledge {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3);
    padding: var(--s-3);
    background: rgba(139, 92, 246, 0.08);
    border: 1px solid rgba(139, 92, 246, 0.2);
    border-radius: var(--r-md);
    cursor: pointer;
    transition: all 150ms ease;
  }
  
  .deploy-acknowledge:hover {
    background: rgba(139, 92, 246, 0.12);
    border-color: rgba(139, 92, 246, 0.3);
  }
  
  .acknowledge-checkbox {
    width: 18px;
    height: 18px;
    margin: 0;
    margin-top: 1px;
    accent-color: var(--violet-500, #8b5cf6);
    cursor: pointer;
    flex-shrink: 0;
  }
  
  .acknowledge-text {
    font-size: 12px;
    color: var(--text-3, #707088);
    line-height: 1.4;
  }
  
  .acknowledge-text strong {
    color: var(--text-1, #f8f8fc);
  }
  
  .deploy-acknowledge:has(.acknowledge-checkbox:checked) {
    border-color: var(--violet-500, #8b5cf6);
    background: rgba(139, 92, 246, 0.15);
  }
  
  .deploy-acknowledge:has(.acknowledge-checkbox:checked) .acknowledge-text {
    color: var(--text-2, #a8a8b8);
  }
  
  /* =====================================================
     Clone Feature Styles
     ===================================================== */
  
  .clone-success-card {
    padding: var(--s-5);
    background: var(--void-mid);
    border: 1px solid rgba(52, 211, 153, 0.3);
    border-radius: var(--r-xl);
  }
  
  .clone-success-header {
    display: flex;
    align-items: flex-start;
    gap: var(--s-4);
    margin-bottom: var(--s-5);
  }
  
  .clone-success-icon {
    color: var(--status-ok);
    flex-shrink: 0;
  }
  
  .clone-success-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0;
  }
  
  .clone-success-subtitle {
    font-size: 13px;
    color: var(--text-3);
    margin-top: var(--s-1);
  }
  
  .clone-result-details {
    background: var(--void-deep);
    border-radius: var(--r-lg);
    padding: var(--s-3);
    margin-bottom: var(--s-5);
  }
  
  .clone-detail-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2) 0;
    border-bottom: 1px solid var(--border-dim);
  }
  
  .clone-detail-row:last-child {
    border-bottom: none;
  }
  
  .clone-detail-label {
    font-size: 12px;
    color: var(--text-4);
  }
  
  .clone-detail-value {
    font-size: 12px;
    color: var(--text-2);
  }
  
  code.clone-detail-value {
    font-family: var(--font-mono);
    color: var(--cyan-400);
    background: transparent;
  }
  
  .clone-path-row {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    max-width: 400px;
  }
  
  .clone-path {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  
  .clone-copy-btn {
    padding: var(--s-1);
    background: transparent;
    border: none;
    color: var(--text-4);
    cursor: pointer;
    border-radius: var(--r-xs);
    transition: all 150ms;
    flex-shrink: 0;
  }
  
  .clone-copy-btn:hover {
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.1);
  }
  
  .clone-actions {
    display: flex;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  
  .clone-actions .btn {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  /* Info Panel for Clone */
  .info-panel {
    display: flex;
    gap: var(--s-4);
    padding: var(--s-4);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-lg);
  }
  
  .info-panel-icon {
    font-size: 20px;
    color: var(--cyan-400);
    flex-shrink: 0;
  }
  
  .info-panel-content {
    flex: 1;
  }
  
  .info-panel-title {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    margin-bottom: var(--s-2);
  }
  
  .info-list {
    font-size: 12px;
    color: var(--text-4);
    padding-left: var(--s-4);
    margin: 0;
    list-style-type: disc;
  }
  
  .info-list li {
    margin-bottom: var(--s-1);
  }
  
  .info-list code {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-3);
    background: var(--void-up);
    padding: 1px 4px;
    border-radius: var(--r-xs);
  }
  
  .label-hint {
    font-size: 11px;
    color: var(--text-5);
    font-weight: 400;
  }
  
  /* Modal warning icon */
  .modal-icon.warning {
    color: var(--status-warn);
  }
  
  /* =====================================================
     DocShards Styles (Inline - matching Clone/Serve pattern)
     ===================================================== */
  
  .shard-mode-tabs {
    display: flex;
    gap: var(--s-2);
    margin-bottom: var(--s-4);
    padding: var(--s-1);
    background: var(--void-deep);
    border-radius: var(--r-lg);
  }
  
  .shard-mode-tab {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-4);
    border-radius: var(--r-md);
    font-size: 13px;
    font-weight: 500;
    color: var(--text-4);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: all var(--dur-fast);
  }
  
  .shard-mode-tab:hover {
    color: var(--text-2);
    background: var(--void-up);
  }
  
  .shard-mode-tab.active {
    color: var(--void-pure);
    background: var(--cyan-400);
  }
  
  .shard-input-row {
    display: flex;
    gap: var(--s-2);
  }
  
  .shard-input-row .input {
    flex: 1;
  }
  
  .shard-checkbox-label {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    cursor: pointer;
  }
  
  .shard-checkbox {
    width: 16px;
    height: 16px;
    accent-color: var(--cyan-400);
  }
  
  /* =====================================================
     Deploy SC Styles
     ===================================================== */
  
  .sc-code-textarea {
    font-family: var(--font-mono);
    font-size: 13px;
    line-height: 1.6;
    min-height: 300px;
    background: var(--void-pure);
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    padding: var(--s-4);
    color: var(--text-1);
    resize: vertical;
    tab-size: 2;
    white-space: pre;
    overflow-x: auto;
  }
  
  .sc-code-textarea:focus {
    border-color: var(--cyan-500);
    box-shadow: 0 0 0 3px rgba(34, 211, 238, 0.15);
    outline: none;
  }
  
  .sc-code-textarea::placeholder {
    color: var(--text-4);
  }
  
  /* =====================================================
     Update INDEX Styles
     ===================================================== */
  
  .update-success-card {
    padding: var(--s-5);
    background: var(--void-mid);
    border: 1px solid rgba(52, 211, 153, 0.3);
    border-radius: var(--r-xl);
  }
  
  .update-success-header {
    display: flex;
    align-items: flex-start;
    gap: var(--s-4);
    margin-bottom: var(--s-5);
  }
  
  .update-success-icon {
    color: var(--status-ok);
    flex-shrink: 0;
  }
  
  .update-success-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0;
  }
  
  .update-success-subtitle {
    font-size: 13px;
    color: var(--text-3);
    margin-top: var(--s-1);
  }
  
  .update-result-details {
    background: var(--void-deep);
    border-radius: var(--r-lg);
    padding: var(--s-3);
    margin-bottom: var(--s-5);
  }
  
  .update-detail-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2) 0;
    border-bottom: 1px solid var(--border-dim);
  }
  
  .update-detail-row:last-child {
    border-bottom: none;
  }
  
  .update-detail-label {
    font-size: 12px;
    color: var(--text-4);
  }
  
  .update-detail-value {
    font-size: 12px;
    color: var(--text-2);
  }
  
  .update-scid-row {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    max-width: 400px;
  }
  
  .update-scid {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  
  .update-copy-btn {
    padding: var(--s-1);
    background: transparent;
    border: none;
    color: var(--text-4);
    cursor: pointer;
    border-radius: var(--r-xs);
    transition: all 150ms;
    flex-shrink: 0;
  }
  
  .update-copy-btn:hover {
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.1);
  }
  
  .update-actions {
    display: flex;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  
  .update-actions .btn {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .index-info-name-row {
    display: flex;
    align-items: center;
    gap: var(--s-3);
  }
  
  .mode-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    padding: 2px 8px;
    font-size: 10px;
    font-weight: 500;
    border-radius: var(--r-sm);
  }
  
  .mode-badge.simulator {
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok);
  }
  
  .alert-info {
    background: rgba(34, 211, 238, 0.1);
    border: 1px solid rgba(34, 211, 238, 0.3);
    color: var(--cyan-400);
  }
  
  .alert-success {
    background: rgba(52, 211, 153, 0.1);
    border: 1px solid rgba(52, 211, 153, 0.3);
    color: var(--status-ok);
  }
  
  .alert-warning {
    background: rgba(251, 191, 36, 0.1);
    border: 1px solid rgba(251, 191, 36, 0.3);
    color: var(--status-warn);
  }
  
  .card-section {
    background: var(--void-deep);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-lg);
    padding: var(--s-4);
    margin-bottom: var(--s-4);
  }
  
  .card-section-title {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    margin: 0 0 var(--s-4) 0;
  }
  
  .simulator-note {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 12px;
    color: var(--status-ok);
    margin-top: var(--s-3);
    justify-content: center;
  }
  
  .modal-warning {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 12px;
    color: var(--status-warn);
    margin-top: var(--s-3);
  }
  
  .confirm-label {
    font-size: 12px;
    color: var(--text-4);
  }
  
  .confirm-value {
    font-size: 12px;
    color: var(--text-2);
    font-family: var(--font-mono);
  }
  
  textarea.input {
    resize: vertical;
    min-height: 60px;
  }
  
  /* =====================================================
     My Content Section (mc- prefix)
     ===================================================== */
  
  /* Stats Row */
  .mc-stats-row {
    display: flex;
    align-items: center;
    gap: var(--s-6);
    padding-bottom: var(--s-4);
    margin-bottom: var(--s-4);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .mc-stat {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
  }
  
  .mc-stat-value {
    font-size: 20px;
    font-weight: 600;
    color: var(--cyan-400);
    font-family: var(--font-mono);
  }
  
  .mc-stat-label {
    font-size: 10px;
    color: var(--text-4);
    text-transform: uppercase;
    letter-spacing: 0.1em;
  }
  
  .mc-refresh-btn {
    margin-left: auto;
  }
  
  /* Tab Filter */
  .mc-tabs {
    display: flex;
    gap: var(--s-1);
    margin-bottom: var(--s-4);
    padding: 3px;
    background: var(--void-base);
    border-radius: var(--r-lg);
    border: 1px solid var(--border-dim);
  }
  
  .mc-tab {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-4);
    background: transparent;
    border: none;
    border-radius: var(--r-md);
    color: var(--text-4);
    font-size: 13px;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  
  .mc-tab:hover {
    color: var(--text-2);
    background: var(--void-up);
  }
  
  .mc-tab.active {
    color: var(--cyan-400);
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
  }
  
  /* Filter Row */
  .mc-filter {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    margin-bottom: var(--s-4);
  }
  
  .mc-filter-label {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-4);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  /* HOLOGRAM Design System Compliant Select */
  .mc-filter-select {
    padding: var(--s-2) var(--s-3);
    padding-right: 32px; /* Room for dropdown arrow */
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
    background: var(--void-deep);
    /* Custom dropdown arrow - HOLOGRAM standard */
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 12 12'%3E%3Cpath fill='%23707088' d='M2 4l4 4 4-4'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 10px center;
    border: 1px solid var(--border-default);
    border-radius: var(--r-md);
    /* CRITICAL: Remove native OS styling */
    appearance: none;
    -webkit-appearance: none;
    -moz-appearance: none;
    cursor: pointer;
    outline: none;
    transition: all 150ms ease;
    min-width: 140px;
  }
  
  .mc-filter-select:hover {
    border-color: var(--cyan-500);
  }
  
  .mc-filter-select:focus {
    border-color: var(--cyan-400);
    box-shadow: 0 0 0 2px rgba(34, 211, 238, 0.15);
  }
  
  .mc-filter-select option {
    background: var(--void-deep);
    color: var(--text-1);
    padding: var(--s-2);
  }
  
  /* Content List */
  .mc-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  
  .mc-item {
    display: flex;
    align-items: flex-start;
    gap: var(--s-4);
    padding: var(--s-4);
    background: var(--void-base);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-lg);
    transition: all 0.15s ease;
  }
  
  .mc-item:hover {
    border-color: var(--border-default);
    background: var(--void-up);
  }
  
  .mc-item-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 44px;
    height: 44px;
    border-radius: var(--r-lg);
    flex-shrink: 0;
  }
  
  .mc-item-icon.index {
    background: rgba(167, 139, 250, 0.12);
    color: var(--violet-400);
  }
  
  .mc-item-icon.doc {
    background: rgba(34, 211, 238, 0.12);
    color: var(--cyan-400);
  }
  
  .mc-item-info {
    flex: 1;
    min-width: 0;
  }
  
  .mc-item-header {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex-wrap: wrap;
    margin-bottom: var(--s-1);
  }
  
  .mc-item-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-1);
  }
  
  .mc-badge {
    padding: 2px 8px;
    font-size: 10px;
    font-weight: 600;
    border-radius: var(--r-sm);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  .mc-badge.index {
    background: rgba(167, 139, 250, 0.2);
    color: var(--violet-400);
  }
  
  .mc-badge.doc {
    background: rgba(34, 211, 238, 0.2);
    color: var(--cyan-400);
  }
  
  .mc-badge-doctype {
    padding: 2px 8px;
    font-size: 10px;
    background: var(--void-surface);
    color: var(--text-3);
    border-radius: var(--r-sm);
    border: 1px solid var(--border-dim);
  }
  
  .mc-item-desc {
    font-size: 13px;
    color: var(--text-3);
    margin-bottom: var(--s-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .mc-item-meta {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    flex-wrap: wrap;
  }
  
  .mc-scid {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-4);
    background: var(--void-deep);
    padding: 2px 8px;
    border-radius: var(--r-sm);
  }
  
  .mc-doc-count,
  .mc-subdir {
    font-size: 11px;
    color: var(--text-4);
  }
  
  .mc-item-actions {
    display: flex;
    gap: var(--s-2);
    flex-shrink: 0;
  }
  
  /* Empty State */
  .mc-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--s-10);
    background: var(--void-base);
    border: 1px dashed var(--border-subtle);
    border-radius: var(--r-lg);
    text-align: center;
    color: var(--text-4);
  }
  
  .mc-empty-title {
    font-size: 15px;
    font-weight: 500;
    color: var(--text-2);
    margin-top: var(--s-3);
    margin-bottom: var(--s-2);
  }
  
  .mc-empty-hint {
    font-size: 13px;
    color: var(--text-4);
  }
  
  /* =====================================================
     Version Control Section
     ===================================================== */
  
  /* Loaded content card (after INDEX is loaded) */
  .vc-loaded-card {
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-xl);
    padding: var(--s-6);
    margin-top: var(--s-5);
  }
  
  .vc-loaded-header {
    display: flex;
    gap: var(--s-4);
    align-items: flex-start;
  }
  
  .vc-loaded-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    border-radius: var(--r-lg);
    background: rgba(34, 211, 238, 0.1);
    color: var(--cyan-400);
    flex-shrink: 0;
  }
  
  .vc-loaded-info {
    flex: 1;
    min-width: 0;
  }
  
  .vc-loaded-name {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1);
    margin-bottom: var(--s-2);
  }
  
  .vc-loaded-meta {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    flex-wrap: wrap;
    margin-bottom: var(--s-2);
  }
  
  .vc-loaded-durl {
    font-size: 13px;
    color: var(--cyan-400);
    background: var(--void-deep);
    padding: var(--s-1) var(--s-2);
    border-radius: var(--r-sm);
  }
  
  .vc-loaded-docs {
    font-size: 13px;
    color: var(--text-3);
  }
  
  .vc-loaded-desc {
    font-size: 13px;
    color: var(--text-3);
    margin-bottom: var(--s-2);
    line-height: 1.5;
  }
  
  .vc-loaded-scid {
    margin-top: var(--s-4);
    padding: var(--s-3);
    background: var(--void-deep);
    border-radius: var(--r-md);
  }
  
  .vc-loaded-scid code {
    font-size: 11px;
    color: var(--text-4);
    word-break: break-all;
    font-family: var(--font-mono);
  }
  
  .vc-actions-grid {
    display: flex;
    gap: var(--s-3);
    flex-wrap: wrap;
    margin-top: var(--s-5);
    padding-top: var(--s-5);
    border-top: 1px solid var(--border-dim);
  }
  
  /* Quick Access Section */
  .vc-quick-section {
    margin-top: var(--s-5);
    padding: var(--s-5);
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-xl);
  }
  
  .vc-section-title {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 14px;
    font-weight: 600;
    color: var(--text-2);
    margin-bottom: var(--s-4);
  }
  
  .vc-quick-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .vc-quick-item {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    padding: var(--s-3) var(--s-4);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-lg);
    cursor: pointer;
    transition: all 0.15s ease;
    text-align: left;
  }
  
  .vc-quick-item:hover {
    border-color: var(--cyan-400);
    background: var(--void-base);
  }
  
  .vc-quick-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 32px;
    height: 32px;
    border-radius: var(--r-md);
    background: rgba(34, 211, 238, 0.1);
    color: var(--cyan-400);
    flex-shrink: 0;
  }
  
  .vc-quick-info {
    flex: 1;
    min-width: 0;
  }
  
  .vc-quick-name {
    display: block;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-1);
    margin-bottom: 2px;
  }
  
  .vc-quick-scid {
    font-size: 11px;
    color: var(--text-4);
    font-family: var(--font-mono);
  }
  
  :global(.vc-quick-arrow) {
    color: var(--text-5);
    flex-shrink: 0;
    transition: transform 0.15s ease, color 0.15s ease;
  }
  
  .vc-quick-item:hover :global(.vc-quick-arrow) {
    transform: translateX(3px);
    color: var(--cyan-400);
  }
  
  .vc-quick-more {
    margin-top: var(--s-4);
    font-size: 13px;
    text-align: center;
  }
  
  .vc-quick-more a {
    color: var(--cyan-400);
    text-decoration: none;
  }
  
  .vc-quick-more a:hover {
    text-decoration: underline;
  }
  
  /* =====================================================
     Libraries Section - Enhanced Design
     ===================================================== */
  
  .libraries-section {
    max-width: 100%;
  }
  
  /* Libraries section uses standard .content-section-title and .content-section-desc */
  
  /* Toolbar */
  .libs-toolbar {
    display: flex;
    gap: var(--s-3);
    align-items: center;
    margin-bottom: var(--s-5);
  }
  
  /* Libs search uses unified .search-box */
  
  .libs-refresh-btn {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    white-space: nowrap;
  }
  
  @media (max-width: 600px) {
    .libs-refresh-text { display: none; }
  }
  
  /* Content Area */
  .libs-content {
    min-height: 300px;
  }
  
  /* Loading State */
  .libs-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-4);
    padding: var(--s-12) var(--s-6);
  }
  
  .libs-loading-animation {
    position: relative;
    width: 80px;
    height: 80px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .libs-loading-ring {
    position: absolute;
    width: 100%;
    height: 100%;
    border: 3px solid transparent;
    border-top-color: var(--cyan-400);
    border-radius: 50%;
    animation: spin 1.2s linear infinite;
  }
  
  .libs-loading-icon {
    color: var(--cyan-400);
    opacity: 0.8;
  }
  
  .libs-loading-text {
    text-align: center;
  }
  
  .libs-loading-title {
    font-size: 16px;
    font-weight: 500;
    color: var(--text-2);
    margin: 0 0 var(--s-1) 0;
  }
  
  .libs-loading-status {
    font-size: 13px;
    color: var(--text-4);
    margin: 0;
  }
  
  /* Error State */
  .libs-error {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-4);
    padding: var(--s-10);
    background: rgba(239, 68, 68, 0.05);
    border: 1px dashed rgba(239, 68, 68, 0.2);
    border-radius: var(--r-lg);
    text-align: center;
  }
  
  .libs-error-icon {
    color: var(--red-400);
    opacity: 0.8;
  }
  
  .libs-error-title {
    font-size: 16px;
    font-weight: 500;
    color: var(--text-1);
    margin: 0;
  }
  
  .libs-error-message {
    font-size: 13px;
    color: var(--text-4);
    max-width: 400px;
    margin: 0;
    line-height: 1.5;
  }
  
  .libs-error-actions {
    display: flex;
    gap: var(--s-3);
    margin-top: var(--s-2);
  }
  
  /* Empty State - Now uses global .content-card pattern from hologram.css */
  /* Old .libs-empty* styles removed - using standard content-card + info-panel */
  
  /* No Results */
  .libs-no-results {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-3);
    padding: var(--s-10);
    color: var(--text-4);
    text-align: center;
  }
  
  .libs-no-results-icon {
    opacity: 0.4;
  }
  
  .libs-no-results strong {
    color: var(--cyan-400);
  }
  
  /* Results Info */
  .libs-results-info {
    font-size: 12px;
    color: var(--text-4);
    margin-bottom: var(--s-4);
    padding-bottom: var(--s-3);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .libs-results-info strong {
    color: var(--cyan-400);
  }
  
  /* Grid */
  .libs-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
    gap: var(--s-4);
  }
  
  /* Library Card */
  .lib-card {
    display: flex;
    flex-direction: column;
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-lg);
    padding: var(--s-4);
    cursor: pointer;
    transition: all 200ms;
    text-align: left;
    min-height: 160px;
  }
  
  .lib-card:hover {
    border-color: var(--cyan-500);
    background: var(--void-up);
    transform: translateY(-3px);
    box-shadow: 0 12px 24px rgba(0, 0, 0, 0.25), 0 0 0 1px rgba(34, 211, 238, 0.1);
  }
  
  .lib-card-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s-3);
    margin-bottom: var(--s-3);
  }
  
  .lib-card-icon {
    width: 44px;
    height: 44px;
    border-radius: var(--r-md);
    background: rgba(34, 211, 238, 0.12);
    border: 1px solid rgba(34, 211, 238, 0.2);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--cyan-400);
    flex-shrink: 0;
    transition: all 200ms;
  }
  
  .lib-card:hover .lib-card-icon {
    background: rgba(34, 211, 238, 0.2);
    border-color: rgba(34, 211, 238, 0.4);
  }
  
  .lib-card-icon-index {
    background: rgba(139, 92, 246, 0.12);
    border-color: rgba(139, 92, 246, 0.2);
    color: var(--violet-400);
  }
  
  .lib-card:hover .lib-card-icon-index {
    background: rgba(139, 92, 246, 0.2);
    border-color: rgba(139, 92, 246, 0.4);
  }
  
  .lib-card-badges {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .lib-card-type {
    font-size: 10px;
    font-weight: 600;
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.1);
    padding: 3px 8px;
    border-radius: var(--r-sm);
    letter-spacing: 0.03em;
  }
  
  .lib-type-index {
    color: var(--violet-400);
    background: rgba(139, 92, 246, 0.1);
  }
  
  .lib-card-rating {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    font-size: 10px;
    color: var(--text-4);
    background: var(--void-up);
    padding: 3px 8px;
    border-radius: var(--r-sm);
  }
  
  .lib-card-body {
    flex: 1;
    display: flex;
    flex-direction: column;
  }
  
  .lib-card-name {
    font-size: 15px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0 0 var(--s-1) 0;
    line-height: 1.3;
  }
  
  .lib-card-durl {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.08);
    padding: 4px 8px;
    border-radius: var(--r-xs);
    display: inline-block;
    margin-bottom: var(--s-2);
    word-break: break-all;
  }
  
  .lib-card-desc {
    font-size: 12px;
    color: var(--text-4);
    line-height: 1.5;
    margin: 0;
    flex: 1;
  }
  
  .lib-card-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: var(--s-3);
    padding-top: var(--s-3);
    border-top: 1px solid var(--border-dim);
  }
  
  .lib-card-meta {
    font-size: 11px;
    color: var(--text-5);
    display: flex;
    align-items: center;
    gap: var(--s-1);
  }
  
  .lib-card-arrow-wrap {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    font-size: 11px;
    font-weight: 500;
    color: var(--text-4);
    transition: all 150ms;
  }
  
  .lib-card-arrow-text {
    opacity: 0;
    transform: translateX(-8px);
    transition: all 200ms;
  }
  
  .lib-card:hover .lib-card-arrow-wrap {
    color: var(--cyan-400);
  }
  
  .lib-card:hover .lib-card-arrow-text {
    opacity: 1;
    transform: translateX(0);
  }
  
  /* =====================================================
     Library Modal - Enhanced Design
     ===================================================== */
  
  .lib-modal {
    max-width: 520px;
    padding: 0;
    overflow: hidden;
  }
  
  .lib-modal-header {
    position: relative;
    padding: var(--s-6);
    background: linear-gradient(180deg, var(--void-up) 0%, var(--void-mid) 100%);
    border-bottom: 1px solid var(--border-dim);
    overflow: hidden;
  }
  
  .lib-modal-header-bg {
    position: absolute;
    inset: 0;
    background: linear-gradient(135deg, rgba(34, 211, 238, 0.08), rgba(139, 92, 246, 0.04));
    pointer-events: none;
  }
  
  .lib-modal-close {
    position: absolute;
    top: var(--s-3);
    right: var(--s-3);
    z-index: 1;
  }
  
  .lib-modal-header-content {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
  }
  
  .lib-modal-icon {
    width: 56px;
    height: 56px;
    border-radius: var(--r-lg);
    background: rgba(34, 211, 238, 0.15);
    border: 1px solid rgba(34, 211, 238, 0.3);
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--cyan-400);
    margin-bottom: var(--s-3);
  }
  
  .lib-modal-icon-index {
    background: rgba(139, 92, 246, 0.15);
    border-color: rgba(139, 92, 246, 0.3);
    color: var(--violet-400);
  }
  
  .lib-modal-title {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0 0 var(--s-2) 0;
  }
  
  .lib-modal-badges {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex-wrap: wrap;
    justify-content: center;
  }
  
  .lib-modal-durl {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.1);
    padding: 4px 10px;
    border-radius: var(--r-sm);
  }
  
  .lib-modal-type {
    font-size: 10px;
    font-weight: 600;
    color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.15);
    padding: 4px 10px;
    border-radius: var(--r-sm);
    letter-spacing: 0.03em;
  }
  
  .lib-modal-type.lib-type-index {
    color: var(--violet-400);
    background: rgba(139, 92, 246, 0.15);
  }
  
  .lib-modal-body {
    padding: var(--s-5);
  }
  
  .lib-modal-section {
    margin-bottom: var(--s-5);
  }
  
  .lib-modal-section:last-child {
    margin-bottom: 0;
  }
  
  .lib-modal-description {
    font-size: 13px;
    color: var(--text-3);
    line-height: 1.6;
    margin: 0;
  }
  
  .lib-modal-section-title {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-4);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    margin: 0 0 var(--s-3) 0;
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .lib-details-grid {
    display: grid;
    gap: var(--s-3);
  }
  
  .lib-detail-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: var(--s-2) var(--s-3);
    background: var(--void-deep);
    border-radius: var(--r-md);
  }
  
  .lib-detail-full {
    flex-direction: column;
    align-items: stretch;
    gap: var(--s-2);
  }
  
  .lib-detail-label {
    font-size: 11px;
    color: var(--text-5);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }
  
  .lib-detail-value {
    font-size: 12px;
    color: var(--text-2);
  }
  
  .lib-detail-highlight {
    color: var(--violet-400);
    font-weight: 500;
  }
  
  .lib-detail-mono {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-4);
  }
  
  .lib-detail-scid-row {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }
  
  .lib-detail-scid-value {
    flex: 1;
    font-family: var(--font-mono);
    font-size: 10px;
    color: var(--text-4);
    background: var(--void-mid);
    padding: var(--s-2);
    border-radius: var(--r-sm);
    overflow-x: auto;
    white-space: nowrap;
  }
  
  .lib-copy-btn {
    padding: var(--s-2);
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    color: var(--text-4);
    cursor: pointer;
    border-radius: var(--r-sm);
    transition: all 150ms;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .lib-copy-btn:hover {
    color: var(--cyan-400);
    border-color: var(--cyan-400);
    background: rgba(34, 211, 238, 0.1);
  }
  
  .lib-detail-rating {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 12px;
  }
  
  .lib-rating-likes {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    color: var(--status-ok);
  }
  
  .lib-rating-sep {
    color: var(--text-5);
  }
  
  .lib-rating-dislikes {
    display: inline-flex;
    align-items: center;
    gap: var(--s-1);
    color: var(--status-err);
  }
  
  .lib-rating-count {
    color: var(--text-5);
    font-size: 11px;
  }
  
  .lib-usage-box {
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-md);
    padding: var(--s-3);
  }
  
  .lib-usage-hint {
    font-size: 12px;
    color: var(--text-4);
    margin: 0 0 var(--s-2) 0;
  }
  
  .lib-usage-code {
    display: block;
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--cyan-400);
    background: var(--void-mid);
    padding: var(--s-2) var(--s-3);
    border-radius: var(--r-sm);
    word-break: break-all;
  }
  
  .lib-modal-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4) var(--s-5);
    background: var(--void-deep);
    border-top: 1px solid var(--border-dim);
  }
  
  .lib-modal-footer-right {
    display: flex;
    gap: var(--s-3);
  }
  
  .lib-embed-btn {
    border-color: var(--violet-500, #8b5cf6);
    color: var(--violet-400, #a78bfa);
  }
  
  .lib-embed-btn:hover {
    background: rgba(139, 92, 246, 0.15);
    border-color: var(--violet-400, #a78bfa);
  }
</style>

