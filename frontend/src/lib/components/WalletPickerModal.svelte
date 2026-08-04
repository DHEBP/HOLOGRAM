<script>
  import { onDestroy } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { walletPicker, walletPickerVisible, walletPickerBusy, walletPickerError } from '../stores/walletPickerStore.js';
  import { walletState, settingsState, saveSetting } from '../stores/appState.js';
  import { goBridge } from '../wallet/goBridge.js';
  import { GetRecentWalletsWithInfo } from '../../../wailsjs/go/main/App.js';
  import { getAvatarUrl, clearAvatarCache } from '../utils/avatarService.js';

  let recentWallets = [];
  let avatarUrls = {}; // address|path -> object URL of the villager avatar
  let fetchedAvatarAddresses = new Set(); // addresses handed to avatarService (for cache cleanup)

  // Avatar loading is deferred + chunked so the villager canvas render runs on idle
  // slots, one wallet at a time, and never blocks typing in the unlock password.
  let avatarQueue = [];              // pending { key, address } entries
  const avatarQueuedKeys = new Set(); // keys already queued or loaded
  let avatarScheduled = false;
  $: connectedAddress = $walletState.address;

  // Signal Dark / stealth: identity core goes dark — no avatar, masked address.
  $: isSignalDark = $settingsState.signalDark;
  $: hideAvatars = isSignalDark || $settingsState.avatarHidden;
  $: defaultWalletPath = $settingsState.defaultWalletPath;

  async function loadRecent() {
    try {
      const raw = await GetRecentWalletsWithInfo();
      const list = Array.isArray(raw) ? raw : (raw?.wallets ?? []);
      recentWallets = list.map(w =>
        typeof w === 'string' ? { path: w, address: '', addressPrefix: '' } :
        ({
          path: w.path ?? w.filePath ?? w.Path ?? '',
          address: w.address ?? w.Address ?? '',
          addressPrefix: w.addressPrefix ?? w.AddressPrefix ?? '',
          filename: w.filename ?? '',
          network: w.network ?? '',
          isCurrent: !!w.isCurrent,
        })
      ).filter(w => w.path);
      loadAvatars();

      // Auto-expand the default wallet into the unlock form (login-page feel).
      const def = $settingsState.defaultWalletPath;
      if (def && !$walletState.isOpen && recentWallets.some(w => w.path === def)) {
        walletPath = def;
        mode = 'unlock';
      }
    } catch { recentWallets = []; }
  }

  async function loadAvatars() {
    // Enqueue every wallet we have an address for (deduped), then kick off the
    // deferred render loop. Cheap: no heavy work happens synchronously here.
    for (const w of recentWallets) {
      enqueueAvatar(w.address || w.path, w.address);
    }
    if (connectedAddress) enqueueAvatar(connectedAddress, connectedAddress);
    scheduleAvatarLoad();
  }

  function enqueueAvatar(key, address) {
    if (!address || !key || avatarUrls[key] || avatarQueuedKeys.has(key)) return;
    avatarQueuedKeys.add(key);
    fetchedAvatarAddresses.add(address);
    avatarQueue.push({ key, address });
  }

  function scheduleAvatarLoad() {
    // Do not render while the user is on the unlock form (typing) or in stealth.
    if (avatarScheduled || hideAvatars || mode === 'unlock') return;
    avatarScheduled = true;
    if (typeof requestIdleCallback === 'function') {
      requestIdleCallback(processAvatarQueue, { timeout: 500 });
    } else {
      setTimeout(processAvatarQueue, 0);
    }
  }

  function processAvatarQueue() {
    avatarScheduled = false;
    if (hideAvatars || mode === 'unlock') return;

    const item = avatarQueue.shift();
    avatarQueuedKeys.delete(item?.key);
    if (!item) { return; }
    if (avatarUrls[item.key]) { scheduleAvatarLoad(); return; }

    // Low-detail thumbnails keep the render cheap; one at a time so the main thread
    // stays free for typing. The next render only starts once this one settles.
    getAvatarUrl(item.address, 44, { lowDetail: true })
      .then(url => { avatarUrls = { ...avatarUrls, [item.key]: url }; })
      .catch(() => {})
      .finally(() => scheduleAvatarLoad());
  }

  $: if (hideAvatars) {
    avatarUrls = {};
    avatarQueue = [];
    avatarQueuedKeys.clear();
    avatarScheduled = false;
  }
  // Enqueue + render when the picker is visible but not stealth-masked.
  $: if (visible && !hideAvatars) loadAvatars();
  // Resume paused renders once the user leaves the unlock form back to the menu.
  $: if (mode === 'menu') scheduleAvatarLoad();

  function walletFilename(w) {
    return w.filename || (w.path.split(/[\\/]/).pop() || w.path);
  }

  function walletShortAddress(w) {
    const a = w.address || '';
    if (a.length > 16) return a.slice(0, 10) + '…' + a.slice(-8);
    return w.addressPrefix || a;
  }

  function openRecent(path) {
    walletPath = path;
    password = ''; // clear so user doesn't accidentally send the wrong password
    mode = 'unlock';
  }

  // Default-wallet toggle (on/off switch, one default at a time)
  function toggleDefault(path, event) {
    event.stopPropagation();
    const makeDefault = defaultWalletPath !== path;
    saveSetting('defaultWalletPath', makeDefault ? path : '');
  }

  let mode = 'menu';            // 'menu' | 'unlock'
  let password = '';
  let walletPath = '';
  let passwordInput;

  $: visible = $walletPickerVisible;
  $: busy = $walletPickerBusy;
  $: error = $walletPickerError;
  $: if (visible) { loadRecent(); reset(); }

  function reset() {
    password = '';
    walletPath = '';
    mode = 'menu';
    walletPicker.setError('');
  }

  function applyWalletResult(res) {
    if (!res?.ok) {
      walletPicker.setError(res?.message || 'Operation failed');
      return false;
    }
    const addr = res.address || '';
    walletState.update(w => ({
      ...w,
      isOpen: true,
      address: addr || w.address,
      walletPath: walletPath || w.walletPath,
    }));
    walletPicker.close();
    reset();
    return true;
  }

  async function doOpen() {
    if (!walletPath) { walletPicker.setError('Select a wallet'); return; }
    walletPicker.setBusy(true); walletPicker.setError('');
    try {
      const res = await goBridge.openWallet(walletPath, password);
      applyWalletResult(res);
    } finally { walletPicker.setBusy(false); }
  }

  function doSkip() {
    walletPicker.close();
    reset();
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && visible) doSkip();
  }

  // Auto-focus the password field when the unlock form appears.
  $: if (mode === 'unlock' && passwordInput) {
    // Tick after render so the input is in the DOM.
    requestAnimationFrame(() => passwordInput.focus());
  }

  // Clear the avatarService cache for every address this modal asked for, so a
  // re-opened picker re-renders fresh instead of reusing a revoked object URL.
  // (avatarService caches URLs module-wide keyed by address+size.)
  function cleanupAvatars() {
    fetchedAvatarAddresses.forEach(addr => clearAvatarCache(addr));
    fetchedAvatarAddresses.clear();
    avatarUrls = {};
  }

  onDestroy(cleanupAvatars);
</script>

<svelte:window on:keydown={onKeydown} />

{#if visible}
  <div class="picker-backdrop" transition:fade={{ duration: 140 }} on:click={doSkip}>
    <div class="picker-card" transition:fly={{ y: 12, duration: 180 }}
         role="dialog" aria-modal="true" aria-label="Choose how to continue"
         on:click|stopPropagation>
      <div class="picker-head">
        <div class="picker-mark">◈</div>
        <div>
          <h2>Connect a wallet</h2>
          <p class="picker-sub">Select a wallet or continue without one.</p>
        </div>
      </div>

      <!-- Wallet cards — stacked like the Hologram identity menu -->
      {#if recentWallets.length > 0}
        <div class="picker-cards">
          {#each recentWallets as w}
            {@const isDefault = defaultWalletPath === w.path}
            <div
              class="wallet-card"
              class:current={w.isCurrent}
              class:default={isDefault}
              class:selected={mode === 'unlock' && walletPath === w.path}
              role="button"
              tabindex="0"
              title="Open this wallet"
              on:click={() => openRecent(w.path)}
              on:keydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); openRecent(w.path); } }}
            >
              <span class="wallet-card-avatar">
                {#if !hideAvatars && avatarUrls[w.address || w.path]}
                  <img src={avatarUrls[w.address || w.path]} alt="" />
                {:else if isSignalDark}
                  <span class="avatar-dark">◈</span>
                {:else}
                  <span class="avatar-empty">◆</span>
                {/if}
              </span>
              <span class="wallet-card-body">
                <span class="wallet-card-name">
                  {walletFilename(w)}
                  {#if w.network === 'simulator'}<span class="net-badge net-sim">SIM</span>{/if}
                  {#if w.network === 'mainnet'}<span class="net-badge net-main">MAIN</span>{/if}
                  {#if isDefault}<span class="net-badge net-default">DEFAULT</span>{/if}
                  {#if w.isCurrent}<span class="net-badge net-current">CURRENT</span>{/if}
                </span>
                <span class="wallet-card-addr">
                  {isSignalDark ? '••••••••••••' : walletShortAddress(w)}
                </span>
              </span>
              <span class="wallet-card-action" title="Set as default wallet">
                <label class="default-switch" on:click|stopPropagation>
                  <span class="default-switch-label">Default</span>
                  <input
                    type="checkbox"
                    class="toggle"
                    checked={isDefault}
                    on:change={(e) => { e.stopPropagation(); toggleDefault(w.path, e); }}
                  />
                </label>
              </span>
            </div>
          {/each}
        </div>
      {:else}
        <div class="picker-empty">
          <span class="empty-mark">◈</span>
          <p>No wallets yet. To get started, create one from the wallet page.</p>
        </div>
      {/if}

      <!-- Unlock form (appears when a wallet card is selected) -->
      {#if mode === 'unlock'}
        <div class="unlock-area">
          <input
            type="password"
            bind:this={passwordInput}
            bind:value={password}
            placeholder="Enter wallet password"
            autocomplete="current-password"
            disabled={busy}
            on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); doOpen(); } }}
          />
          <button class="btn btn-primary" on:click={doOpen} disabled={busy || !password}>
            {busy ? 'Opening…' : 'Unlock'}
          </button>
        </div>
      {/if}

      <!-- Continue without wallet -->
      <button class="continue-btn" on:click={doSkip}>
        Continue without wallet
      </button>

      {#if error}
        <div class="picker-error" role="alert">{error}</div>
      {/if}

      <!-- Show-on-launch switch (replaces the old "Don't show again" checkbox) -->
      <label class="picker-launch">
        <input
          type="checkbox"
          class="toggle"
          checked={$settingsState.walletPickerOnLaunch}
          on:change={(e) => {
            const on = e.target.checked;
            saveSetting('walletPickerOnLaunch', on);
            if (!on) walletPicker.close();
          }}
        />
        <span>Show on launch</span>
      </label>
    </div>
  </div>
{/if}

<style>
  .picker-backdrop {
    position: fixed; inset: 0; z-index: 60;
    display: flex; align-items: center; justify-content: center;
    /* Static translucent backdrop. Avoids backdrop-filter: blur(), which forces an
       expensive re-composite of the whole layer on every repaint (the password
       input's mask + caret) in the software-rendered WebKitGTK webview. */
    background: rgba(4, 10, 18, 0.72);
  }
  .picker-card {
    width: min(460px, calc(100vw - 32px));
    max-height: calc(100vh - 48px);
    overflow-y: auto;
    padding: 22px; border-radius: var(--r-lg, 14px);
    background: var(--void-deep, #08080e);
    border: 1px solid var(--border-default, #223042);
    box-shadow: 0 16px 48px rgba(0,0,0,.45);
    color: var(--text-1, #e8eef6);
  }

  .picker-head {
    display: flex; align-items: flex-start; gap: 12px;
    margin-bottom: 16px;
  }
  .picker-mark {
    width: 34px; height: 34px; border-radius: 10px;
    display: flex; align-items: center; justify-content: center;
    background: linear-gradient(135deg, rgba(34,211,238,.18), rgba(139,92,246,.14));
    border: 1px solid var(--border-accent, rgba(34,211,238,.4));
    color: var(--cyan-400, #22d3ee);
    font-size: 16px;
    flex-shrink: 0;
  }
  .picker-head h2 { margin: 0 0 4px; font-size: 1.1rem; font-weight: 600; }
  .picker-sub { margin: 0; opacity: .7; font-size: .85rem; line-height: 1.45; }

  /* Selected card highlight */
  .wallet-card.selected {
    border-color: var(--cyan-500, #06b6d4);
    background: var(--void-up, #181824);
  }

  /* Wallet cards (stack) */
  .picker-cards {
    display: flex; flex-direction: column; gap: 8px;
    margin-bottom: 14px;
  }
  .wallet-card {
    display: flex; align-items: center; gap: 12px;
    width: 100%; text-align: left;
    padding: 10px 12px;
    background: var(--void-base, #0c0c14);
    border: 1px solid var(--border-subtle, rgba(255,255,255,.06));
    border-radius: var(--r-md, 8px);
    color: inherit; cursor: pointer;
    transition: border-color 150ms ease, background 150ms ease, transform 150ms ease;
  }
  .wallet-card:hover {
    border-color: var(--border-accent, rgba(34,211,238,.4));
    background: var(--void-up, #181824);
    transform: translateY(-1px);
  }
  .wallet-card:active { transform: translateY(0); }
  .wallet-card.current { border-color: rgba(52,211,153,.45); }
  .wallet-card.default { border-color: rgba(34,211,238,.55); }

  .wallet-card-avatar {
    width: 44px; height: 44px; border-radius: 50%;
    border: 2px solid var(--cyan-400, #22d3ee);
    overflow: hidden; flex-shrink: 0;
    display: flex; align-items: center; justify-content: center;
    background: var(--void-mid, #12121c);
  }
  .wallet-card-avatar img { width: 100%; height: 100%; object-fit: cover; }
  .avatar-dark, .avatar-empty {
    font-size: 15px;
  }
  .avatar-dark { color: var(--cyan-400, #22d3ee); opacity: .8; }
  .avatar-empty { color: var(--text-4, #505068); }

  .wallet-card-body { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .wallet-card-name {
    font-weight: 600; font-size: .92rem;
    display: flex; align-items: center; gap: 6px; flex-wrap: wrap;
  }
  .wallet-card-addr {
    font-family: var(--font-mono, monospace);
    font-size: .76rem; opacity: .65; word-break: break-all;
  }

  .net-badge {
    font-family: var(--font-mono, monospace);
    font-size: .58rem; font-weight: 700; letter-spacing: .08em;
    padding: 2px 5px; border-radius: 4px;
    text-transform: uppercase;
  }
  .net-main { background: rgba(52,211,153,.14); color: #34d399; }
  .net-sim { background: rgba(248,113,113,.14); color: #f87171; }
  .net-default { background: rgba(34,211,238,.16); color: #22d3ee; }
  .net-current { background: rgba(34,211,238,.12); color: #a5f3fc; }

  .wallet-card-action {
    flex-shrink: 0;
    display: flex; align-items: center;
    padding: 6px 0 6px 8px;
  }
  .default-switch {
    display: flex; align-items: center; gap: 6px;
    cursor: pointer;
  }
  .default-switch-label {
    font-size: .66rem; text-transform: uppercase; letter-spacing: .08em;
    opacity: .55;
  }

  /* Unlock area */
  .unlock-area {
    display: flex; gap: 8px; align-items: center;
    margin-bottom: 14px;
    padding: 12px 14px;
    background: var(--void-base, #0c0c14);
    border: 1px solid var(--border-accent, rgba(34,211,238,.4));
    border-radius: var(--r-md, 8px);
  }
  .unlock-area input[type=password] {
    flex: 1;
    background: var(--void-mid, #12121c);
    border: 1px solid var(--border-default, rgba(255,255,255,.09));
    border-radius: var(--r-sm, 6px);
    padding: 9px 12px;
    color: inherit;
    font-size: .9rem;
    outline: none;
    transition: border-color 150ms ease;
  }
  .unlock-area input[type=password]:focus {
    border-color: var(--cyan-400, #22d3ee);
  }
  .unlock-area .btn-primary {
    flex-shrink: 0;
    padding: 9px 16px;
    font-size: .88rem;
  }

  /* Continue without wallet — sits below the cards, framed but secondary */
  .continue-btn {
    width: 100%;
    display: flex; align-items: center; justify-content: center;
    gap: 6px;
    padding: 10px 14px;
    background: transparent;
    border: 1px solid var(--border-default, rgba(255,255,255,.09));
    border-radius: var(--r-md, 8px);
    color: var(--text-2, #b0b8c8);
    font-size: .88rem;
    cursor: pointer;
    transition: background 150ms ease, border-color 150ms ease, color 150ms ease;
  }
  .continue-btn:hover {
    background: var(--void-hover, #262634);
    border-color: var(--border-strong, rgba(255,255,255,.12));
    color: var(--text-1, #e8eef6);
  }
  .continue-btn:active {
    background: var(--void-active, #2e2e3e);
  }

  /* Empty state */
  .picker-empty {
    text-align: center; padding: 26px 16px; margin-bottom: 14px;
    border: 1px dashed var(--border-default, rgba(255,255,255,.12));
    border-radius: var(--r-md, 8px);
    color: var(--text-3, #707088); font-size: .86rem; line-height: 1.5;
  }
  .empty-mark { display: block; font-size: 22px; margin-bottom: 8px; opacity: .5; }

  .picker-error {
    margin-top: 14px; padding: 10px 12px; border-radius: var(--r-md, 8px);
    background: rgba(255, 92, 92, .12); border: 1px solid rgba(255,92,92,.35);
    color: #ffb4b4; font-size: .88rem;
  }

  .picker-launch {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
    margin-top: 16px; padding-top: 14px;
    border-top: 1px solid var(--border-subtle, rgba(255,255,255,.06));
    font-size: .82rem; opacity: .8; cursor: pointer;
  }
</style>
