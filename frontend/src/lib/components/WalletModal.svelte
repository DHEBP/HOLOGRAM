<script>
  import { fly, fade } from 'svelte/transition';
  import { walletState, settingsState, addressMasked, walletRequests, activeWalletRequest, approveWalletRequest, denyWalletRequest, handleBackendError } from '../stores/appState.js';
  import { OpenWallet, GetBalance, ListRecentWallets, SelectWalletFile, GetRecentWalletsWithInfo, SwitchWallet, SplitIntegratedAddress } from '../../../wailsjs/go/main/App.js';

  // Derived from activeWalletRequest
  $: request = $activeWalletRequest;
  $: isOpen = !!request;

  let password = '';
  let walletPath = '';
  let error = '';
  let isLoading = false;

  const ZERO_SCID = '0000000000000000000000000000000000000000000000000000000000000000';

  // A request carries a real smart contract call only when there is an actual entrypoint
  // (top-level or inside sc_rpc/sc_data). A non-empty sc_args/sc_data array of junk rows is
  // NOT a call — treating it as one labeled burns as "Deposit" while the chain no-ops (R2-B6).
  $: hasSCCall = (() => {
    const ep = request?.payload?.entrypoint;
    if (typeof ep === 'string' && ep.trim().length > 0) return true;
    const rows = [
      ...(Array.isArray(request?.payload?.sc_data) ? request.payload.sc_data : []),
      ...(Array.isArray(request?.payload?.sc_args) ? request.payload.sc_args : []),
    ];
    return rows.some(arg => arg?.name === 'entrypoint' && String(arg?.value ?? arg?.Value ?? '').trim().length > 0);
  })();

  // A destination that sets RPC_NEEDS_REPLYBACK_ADDRESS is answered by a transfer back, so the
  // wallet attaches the payer's address to the payload. A DERO recipient normally cannot tell
  // who paid, so this converts an anonymous payment into an identified one -- the approval has
  // to say so, or a dApp discloses the user's identity with nothing on screen.
  // Resolved per destination, not per request: one call can mix disclosing and ordinary
  // recipients. SplitIntegratedAddress parses locally and needs no open wallet.
  let replybackDests = {};
  $: resolveReplybackDisclosure(request);

  async function resolveReplybackDisclosure(req) {
    replybackDests = {};
    const dests = (req?.payload?.transfers || [])
      .map(t => t?.destination)
      .filter(d => typeof d === 'string' && d.trim().length > 0);
    for (const d of dests) {
      try {
        const res = await SplitIntegratedAddress(d.trim());
        if (res && res.success && res.needsReplyback) {
          replybackDests = { ...replybackDests, [d]: true };
        }
      } catch (err) {
        // Fail loud rather than silent: if we cannot tell, say we cannot tell.
        replybackDests = { ...replybackDests, [d]: 'unknown' };
      }
    }
  }

  // Total native-DERO (zero-SCID) burn across the request's transfers + top-level SC deposits.
  $: scDeroDeposit = Number(request?.payload?.sc_dero_deposit) || 0;
  $: scTokenDeposit = Number(request?.payload?.sc_token_deposit) || 0;
  $: nativeBurnTotal = (request?.payload?.transfers || [])
    .filter(t => !t.scid || t.scid === ZERO_SCID)
    .reduce((sum, t) => sum + (typeof t.burn === 'number' ? t.burn : 0), 0) + scDeroDeposit;

  // BLOCKED: a native-DERO burn with no contract attached destroys the coins permanently and
  // sends them to no one. HOLOGRAM never burns DERO -- this request is rejected outright, with
  // no approve path. Anyone who genuinely intends to burn DERO must use the DERO CLI wallet.
  $: isBurnBlocked = nativeBurnTotal > 0 && !hasSCCall;
  $: blockedBurnAmount = Math.round(nativeBurnTotal / 100000);

  // Effective ring size the backend will actually use. A dApp that requests anonymize
  // but omits/undersizes the ring is clamped up to 16 (minAnonymizeRingSize, wallet.go)
  // so the decoy promise below is truthful -- ring 2 structurally pins the sender.
  $: requestedRing = Number(request?.payload?.ringsize) || 0;
  $: effectiveRing = request?.payload?.anonymize ? Math.max(requestedRing, 16) : requestedRing;

  let recentWallets = [];
  let recentWalletsInfo = [];
  let showWalletSwitcher = false;
  let switchPassword = '';
  let selectedSwitchWallet = null;
  
  // Set by "Always allow" so handleApprove can tell the two answers apart. Reset on every
  // new request object, or one "always" would silently persist the next app's grant too.
  let rememberChoice = false;
  let rememberResetFor = null;
  $: if (request && request !== rememberResetFor) {
    rememberResetFor = request;
    rememberChoice = false;
  }

  // One predicate for BOTH the password demand and the unlock UI, so they cannot diverge.
  //
  // A connect grants public chain data only, so it needs no wallet — EXCEPT the empty-sheet
  // case, which is Sign In with DERO (handleAuthComplete): that signs a challenge and does.
  // A use-time permission prompt needs the wallet for the two doors that read wallet state.
  function requestNeedsOpenWallet(req) {
    if (!req) return true;
    if (req.type === 'permission') {
      return req.permission?.id === 'view_address' || req.permission?.id === 'view_balance';
    }
    if (req.type !== 'connect') return true;
    if (req.isSignIn) return true; // signs a challenge, so the wallet must open
    // A WebSocket dApp still names its doors at connect (that plane has no use-time
    // prompt), so it needs the wallet if any of them read wallet state.
    return connectGrantIds(req).some(id => id === 'view_address' || id === 'view_balance');
  }

  // A TELA origin is a 64-char SCID, which overran the card and was clipped mid-string —
  // an origin you cannot read is not a disclosure. Ellipsise the MIDDLE so both ends stay
  // checkable against the address bar; the full value is on the title attribute.
  function shortOrigin(origin) {
    const s = (origin || '').trim();
    if (!s) return 'Local App';
    if (/^[0-9a-fA-F]{64}$/.test(s)) return `${s.slice(0, 10)}…${s.slice(-8)}`;
    return s;
  }

  // Doors a connect will actually grant. Empty for the in-browser path, which grants public
  // chain data only and asks for everything else at the moment of use.
  // alwaysAsk entries are excluded because they are never stored (CanStorePermission) —
  // listing them would promise a standing grant the wallet refuses to keep.
  function connectGrantList(req) {
    const sheet = Array.isArray(req?.requestedPermissions) ? req.requestedPermissions : [];
    return sheet.filter(p => p?.id && p.id !== 'read_public_data' && !p.alwaysAsk);
  }
  function connectGrantIds(req) {
    return connectGrantList(req).map(p => p.id);
  }
  // Did the app ask for spending? It gets no standing grant, but say so rather than
  // silently dropping the request from the sheet.
  function connectAsksToSpend(req) {
    const sheet = Array.isArray(req?.requestedPermissions) ? req.requestedPermissions : [];
    return sheet.some(p => p?.alwaysAsk);
  }

  $: connectNeedsWallet = requestNeedsOpenWallet(request);

  function getWalletFilename(path) {
    if (!path) return '';
    return path.split(/[\\/]/).pop() || path;
  }

  // Initialize wallet path from settings/state when modal opens
  $: if (isOpen && !walletPath) {
    walletPath = $walletState.walletPath || $settingsState.lastWalletPath || '';
    // Load recent wallets
    loadRecentWallets();
  }

  async function loadRecentWallets() {
    try {
      // Load enhanced wallet info
      const infos = await GetRecentWalletsWithInfo();
      if (infos && infos.length > 0) {
        recentWalletsInfo = infos;
        recentWallets = infos.map(w => w.path);
        // If no wallet path set, use the most recent
        if (!walletPath && infos.length > 0) {
          walletPath = infos[0].path;
        }
      } else {
        // Fallback to simple list
        const recents = await ListRecentWallets();
        if (recents && recents.length > 0) {
          recentWallets = recents;
          recentWalletsInfo = recents.map(p => ({ path: p, filename: getWalletFilename(p), addressPrefix: '', isCurrent: false }));
          if (!walletPath && recents.length > 0) {
            walletPath = recents[0];
          }
        }
      }
    } catch (e) {
      console.error('Failed to load recent wallets:', e);
    }
  }

  async function browseWallet() {
    try {
      const selected = await SelectWalletFile();
      if (selected) {
        walletPath = selected;
      }
    } catch (e) {
      console.error('File dialog failed:', e);
    }
  }

  function selectWalletToSwitch(wallet) {
    selectedSwitchWallet = wallet;
    switchPassword = '';
  }

  async function handleSwitchWallet() {
    if (!selectedSwitchWallet) return;
    
    isLoading = true;
    error = '';
    
    try {
      const result = await SwitchWallet(selectedSwitchWallet.path, switchPassword);
      if (!result.success) {
        error = handleBackendError(result, { showToast: false }) || 'Failed to switch wallet';
        isLoading = false;
        return;
      }
      
      // Update wallet state
      walletState.update(state => ({
        ...state,
        isOpen: true,
        address: result.address,
        balance: result.balance,
        lockedBalance: result.lockedBalance,
        walletPath: selectedSwitchWallet.path,
      }));
      
      // Store wallet path in settings
      settingsState.update(s => ({ ...s, lastWalletPath: selectedSwitchWallet.path }));
      
      // Reset switch state
      selectedSwitchWallet = null;
      switchPassword = '';
      showWalletSwitcher = false;
      
      // Reload recent wallets to update current marker
      await loadRecentWallets();
    } catch (e) {
      error = e.message || 'Unable to switch wallet. Please try again.';
      console.error('[WalletModal] Switch wallet error:', e);
    } finally {
      isLoading = false;
    }
  }

  function cancelSwitch() {
    selectedSwitchWallet = null;
    switchPassword = '';
    showWalletSwitcher = false;
  }

  async function handleApprove() {
    if (!request) return;
    
    // Same predicate the template uses to decide whether to DRAW the unlock form,
    // so the modal can never demand a password it never rendered a field for.
    const needsOpenWallet = requestNeedsOpenWallet(request);

    if (!$walletState.isOpen && needsOpenWallet) {
      if (!walletPath) {
        error = 'Please select a wallet file';
        return;
      }
      if (!password) {
        error = 'Please enter your wallet password';
        return;
      }
      
      // Actually open the wallet
      isLoading = true;
      error = '';
      
      try {
        const result = await OpenWallet(walletPath, password);
        if (!result.success) {
          error = handleBackendError(result, { showToast: false }) || 'Failed to open wallet';
          isLoading = false;
          return;
        }
        
        // Update wallet state
        walletState.update(state => ({
          ...state,
          isOpen: true,
          address: result.address,
          walletPath: walletPath,
        }));
        
        // Store wallet path in settings for next time
        settingsState.update(s => ({ ...s, lastWalletPath: walletPath }));
        
        // Fetch balance
        try {
          const balance = await GetBalance();
          if (balance.success) {
            walletState.update(state => ({
              ...state,
              balance: balance.balance,
              lockedBalance: balance.lockedBalance,
            }));
          }
        } catch (e) {
          console.error('Failed to fetch balance:', e);
        }
      } catch (e) {
        error = e.message || 'Unable to open wallet. Please check the file path and try again.';
        console.error('[WalletModal] Open wallet error:', e);
        isLoading = false;
        return;
      }
    }

    isLoading = true;
    error = '';

    // Hard backstop: HOLOGRAM never burns DERO. A native-DERO burn with no contract attached
    // can never be approved here -- it is rejected at the backend too. This refuses any attempt
    // to approve one, so there is no path through the UI that broadcasts a destructive burn.
    if (isBurnBlocked) {
      error = 'HOLOGRAM does not allow burning DERO. To deliberately burn DERO, use the DERO CLI wallet.';
      isLoading = false;
      return;
    }

    try {
      // A connect grants exactly the doors the sheet showed — nothing hidden is added, and
      // the in-browser path shows none, so this is null and Go falls back to public data.
      const permissions = request.type === 'connect' && connectGrantIds(request).length > 0
        ? connectGrantIds(request)
        : null;
      await approveWalletRequest(request.id, password, null, permissions, rememberChoice);
      password = ''; // Clear password after use
      walletPath = ''; // Reset for next time
      rememberChoice = false;

      // Restore focus to main document to prevent iframe from capturing scroll
      restoreFocus();
    } catch (e) {
      error = e.message || 'Unable to process request. Please try again.';
      console.error('[WalletModal] Approve request error:', e);
    } finally {
      isLoading = false;
    }
  }

  function handleDeny() {
    if (!request) return;
    denyWalletRequest(request.id);
    password = '';
    walletPath = '';
    error = '';
    
    // Restore focus to main document
    restoreFocus();
  }
  
  // Restore interactivity after modal closes
  function restoreFocus() {
    // The iframe should be able to receive scroll events
    // Focus the iframe so scroll events go to its content
    const attempts = [50, 100, 200, 500, 1000];
    attempts.forEach(delay => {
      setTimeout(() => {
        const iframe = document.querySelector('.browser-content-frame');
        if (iframe) {
          // Make sure iframe can receive events
          iframe.style.pointerEvents = 'auto';
          
          // Click inside iframe to give it focus (allows scrolling)
          try {
            iframe.focus();
            // Also try to focus the iframe's document body
            if (iframe.contentWindow && iframe.contentDocument) {
              iframe.contentDocument.body?.focus();
            }
          } catch (e) {
            // Cross-origin restrictions may prevent this
          }
        }
      }, delay);
    });
  }
</script>

{#if isOpen}
  <!-- Backdrop -->
  <div 
    class="modal-panel-overlay"
    transition:fade={{ duration: 200 }}
    on:click={handleDeny}
  ></div>

  <!-- Slide-in Panel -->
  <div 
    class="modal-panel"
    transition:fly={{ x: 300, duration: 300 }}
  >
    <!-- Header -->
    <div class="modal-panel-header">
      <div class="modal-panel-header-row">
        <span class="modal-panel-icon">◈</span>
        <div>
          <h2 class="modal-panel-title">Wallet Request</h2>
          <p class="modal-panel-subtitle">
            {#if request.type === 'connect'}
              Connection Request
            {:else if request.type === 'sign'}
              Transaction Signing
            {:else}
              Permission Request
            {/if}
          </p>
        </div>
      </div>
    </div>

    <!-- Content -->
    <div class="modal-panel-content">
      <!-- App Info -->
      <div class="modal-app-info-card">
        <h3 class="modal-app-info-label">REQUEST FROM</h3>
        <div class="modal-app-info-row">
          <div class="modal-app-icon">◎</div>
          <div>
            <div class="modal-app-name">{request.appName || 'Unknown App'}</div>
            <div class="modal-app-origin" title={request.origin || ''}>
              {shortOrigin(request.origin)}
              <!-- The name and the address are both chosen by the asker unless the
                   browser vouched for the origin. Say which one you are looking at.
                   Shown on permission prompts too: those grant MORE than a connect,
                   so warning only on connect would drop the caveat where it matters most. -->
              {#if (request.type === 'connect' || request.type === 'permission') && !request.originVerified}
                <span class="modal-app-origin-unverified" title="This app told us its own name and address. Nothing has confirmed them.">· self-reported</span>
              {/if}
            </div>
          </div>
        </div>
      </div>

      <!-- Request Details -->
      <div class="wallet-request-details">
        {#if request.type === 'permission'}
          <!-- Asked at the moment of use, so it can name the thing being reached for. -->
          <div>
            <h3 class="modal-section-title">{request.permission?.name || 'Wallet Access'}</h3>
            <p class="wallet-readonly-desc">{request.permission?.description || ''}</p>
            <!-- The note is a flex row, so everything after the icon has to sit inside ONE
                 child. An inline element left as a bare sibling becomes its own flex item and
                 the sentence lays out as columns. -->
            {#if request.methodName}
              <p class="wallet-info-note">
                <span class="wallet-info-icon">i</span>
                <span class="wallet-info-text">Requested by <code>{request.methodName}</code></span>
              </p>
            {/if}
            <p class="wallet-info-note">
              <span class="wallet-info-icon">i</span>
              <span class="wallet-info-text">
                "Always allow" is remembered for this app <strong>and this wallet only</strong> —
                another wallet is asked again. Take it back in Settings → Connected Apps.
              </span>
            </p>
          </div>
        {:else if request.type === 'connect'}
          <div>
            {#if request.isSignIn}
              <!-- Signs a challenge. Say what approving does; do not draw a permission
                   sheet it does not have. -->
              <p class="wallet-info-note">
                <span class="wallet-info-icon">i</span>
                {request.description}
              </p>
            {:else if connectGrantList(request).length > 0}
              <!-- WebSocket dApps name their doors at connect: that plane has no use-time
                   prompt, so this is where they are granted. One decision, no checkboxes. -->
              <h3 class="modal-section-title">This app will be granted</h3>
              <div class="wallet-readonly-permissions">
                {#each connectGrantList(request) as perm}
                  <div class="wallet-readonly-item">
                    <span class="wallet-check-icon">✓</span>
                    <span>{perm.name} — {perm.description}</span>
                  </div>
                {/each}
                {#if connectAsksToSpend(request)}
                  <div class="wallet-readonly-item wallet-readonly-item-denied">
                    <span class="wallet-denied-icon">✗</span>
                    <span>Spending is not granted here — every transaction is approved separately</span>
                  </div>
                {/if}
              </div>
            {:else}
              <!-- The in-browser path. Connecting buys public chain data and nothing else. -->
              <div class="wallet-readonly-badge">
                <span class="wallet-readonly-icon">◎</span>
                <span>Public Data Only</span>
              </div>
              <p class="wallet-readonly-desc">
                Connecting lets this app read public blockchain data. It gets no access to
                your wallet — if it needs your address, your balance, or to spend, it has to
                ask you then.
              </p>
              <div class="wallet-readonly-permissions">
                <div class="wallet-readonly-item">
                  <span class="wallet-check-icon">✓</span>
                  <span>Read public blockchain info (blocks, transactions, network stats)</span>
                </div>
                <div class="wallet-readonly-item wallet-readonly-item-denied">
                  <span class="wallet-denied-icon">✗</span>
                  <span>Cannot see your address, balance or history without asking</span>
                </div>
                <div class="wallet-readonly-item wallet-readonly-item-denied">
                  <span class="wallet-denied-icon">✗</span>
                  <span>Cannot spend — every transaction is approved separately</span>
                </div>
              </div>
            {/if}
          </div>
        {:else if request.type === 'sign'}
          <!-- Transaction Details -->
          <div>
            <h3 class="modal-section-title">Transaction Details</h3>
            
            <div class="modal-tx-details-card">
              <!-- Smart Contract Info (show first if present) -->
              {#if request.payload.scid || request.payload.entrypoint}
                <div class="wallet-tx-sc-header">
                  {#if request.payload.entrypoint}
                    <div class="modal-tx-field">
                      <div class="modal-tx-label">SC FUNCTION</div>
                      <div class="modal-tx-entrypoint">{request.payload.entrypoint}</div>
                    </div>
                  {/if}
                  {#if request.payload.scid}
                    <div class="modal-tx-field">
                      <div class="modal-tx-label">SMART CONTRACT</div>
                      <div class="modal-tx-scid" title={request.payload.scid}>
                        {request.payload.scid.slice(0, 8)}...{request.payload.scid.slice(-8)}
                      </div>
                    </div>
                  {/if}
                </div>
              {/if}
              
              <!-- Transfers + deposits (DERO / token) — must match what executes (R2-B4) -->
              {#if (request.payload.transfers && request.payload.transfers.length > 0) || scDeroDeposit > 0 || scTokenDeposit > 0}
                {@const allTransfers = request.payload.transfers || []}
                {@const deroTransfers = allTransfers.filter(t => !t.scid || t.scid === ZERO_SCID)}
                {@const tokenTransfers = allTransfers.filter(t => t.scid && t.scid !== ZERO_SCID)}
                {@const totalAmount = deroTransfers.reduce((sum, t) => sum + (t.amount || 0), 0)}
                {@const totalBurn = deroTransfers.reduce((sum, t) => sum + (typeof t.burn === 'number' ? t.burn : 0), 0) + scDeroDeposit}
                {@const transferFees = deroTransfers.reduce((sum, t) => sum + (t.fees || 0), 0)}
                {@const topLevelFees = request.payload.fees || 0}
                {@const totalFees = transferFees + topLevelFees}
                {@const totalDero = totalAmount + totalBurn + totalFees}
                {@const destinations = allTransfers.map(t => t.destination).filter(d => typeof d === 'string' && d.trim().length > 0)}

                <!-- BLOCKED BURN: native-DERO burn with no contract attached destroys the coins
                     permanently and sends them to no one. HOLOGRAM never burns DERO, so this
                     request is rejected outright -- there is no approve path. When blocked, we
                     show only this notice and suppress the cost breakdown below, since nothing
                     here is approvable. -->
                {#if isBurnBlocked}
                  <div class="modal-burn-danger">
                    <div class="modal-burn-danger-title">
                      <span class="modal-burn-danger-ic">⚠</span> REQUEST BLOCKED
                    </div>
                    <div class="modal-burn-danger-amount">−{blockedBurnAmount.toLocaleString()} DERO</div>
                    <div class="modal-burn-danger-copy">
                      This request would <strong>permanently destroy {blockedBurnAmount.toLocaleString()} DERO</strong>.
                      A burn with no smart contract attached sends the coins to <strong>no one</strong>
                      and <strong>cannot be undone</strong>. <strong>HOLOGRAM does not burn DERO</strong>,
                      so this request has been blocked.
                    </div>
                    <div class="modal-burn-danger-norecipient">
                      ◇ To deliberately burn DERO, use the DERO CLI wallet.
                    </div>
                  </div>
                {/if}

                <!-- Show total DERO cost -->
                {#if totalDero > 0 && !isBurnBlocked}
                  <div class="modal-tx-field">
                    <div class="modal-tx-label">TOTAL COST</div>
                    <div class="modal-tx-amount modal-tx-amount-total">
                      {(totalDero / 100000).toLocaleString()} DERO
                    </div>
                  </div>
                {/if}

                <!-- Show breakdown of costs if any non-zero values. Suppressed entirely for a
                     blocked burn (nothing here is approvable). A burn that reaches this point
                     therefore always routes to a contract -- a deposit, not destruction. -->
                {#if !isBurnBlocked && (totalAmount > 0 || totalBurn > 0 || totalFees > 0 || scTokenDeposit > 0 || tokenTransfers.length > 0)}
                  <div class="modal-tx-breakdown">
                    <div class="modal-tx-label modal-tx-label-small">BREAKDOWN</div>
                    {#if totalBurn > 0}
                      <div class="modal-tx-breakdown-item">
                        <span class="modal-tx-breakdown-label">Deposit to contract:</span>
                        <span class="modal-tx-breakdown-value">{(totalBurn / 100000).toLocaleString()} DERO</span>
                      </div>
                    {/if}
                    {#if scTokenDeposit > 0}
                      <div class="modal-tx-breakdown-item">
                        <span class="modal-tx-breakdown-label">Token deposit:</span>
                        <span class="modal-tx-breakdown-value">{(scTokenDeposit).toLocaleString()} atomic{#if request.payload.sc_token_deposit_scid} · {String(request.payload.sc_token_deposit_scid).slice(0, 8)}…{/if}</span>
                      </div>
                    {/if}
                    {#if totalFees > 0}
                      <div class="modal-tx-breakdown-item">
                        <span class="modal-tx-breakdown-label">Fees:</span>
                        <span class="modal-tx-breakdown-value">{(totalFees / 100000).toLocaleString()} DERO</span>
                      </div>
                    {/if}
                    {#if totalAmount > 0}
                      <div class="modal-tx-breakdown-item">
                        <span class="modal-tx-breakdown-label">Amount:</span>
                        <span class="modal-tx-breakdown-value">{(totalAmount / 100000).toLocaleString()} DERO</span>
                      </div>
                    {/if}
                    {#each tokenTransfers as tt, ti}
                      <div class="modal-tx-breakdown-item">
                        <span class="modal-tx-breakdown-label">Token transfer{tokenTransfers.length > 1 ? ` ${ti + 1}` : ''}:</span>
                        <span class="modal-tx-breakdown-value modal-tx-token-scid">{(tt.amount || 0).toLocaleString()}{#if tt.burn} + burn {(tt.burn || 0).toLocaleString()}{/if} · {String(tt.scid).slice(0, 8)}…</span>
                      </div>
                    {/each}
                  </div>
                {/if}

                <!-- Every destination that will execute — not just transfers[0] (R2-B4) -->
                {#if destinations.length > 0 && !isBurnBlocked}
                  {#each destinations as dest, di}
                    <div class="modal-tx-field">
                      <div class="modal-tx-label">{destinations.length > 1 ? `DESTINATION ${di + 1}` : 'DESTINATION'}</div>
                      <div class="modal-tx-destination">{dest}</div>
                      {#if replybackDests[dest] === true}
                        <div class="modal-tx-replyback">
                          This service replies by sending you a transfer, so your wallet address
                          is attached — it will know you paid.
                        </div>
                      {:else if replybackDests[dest] === 'unknown'}
                        <div class="modal-tx-replyback">
                          Could not check whether this destination requires your address.
                        </div>
                      {/if}
                    </div>
                  {/each}
                {/if}

                <!-- APPARENT SENDER (attribution disclosure)
                     Surfaces what the dApp asked for via anonymize / preferred_decoys, so the
                     approval reflects the WHOLE request. Informational only -- removes no
                     capability; the user still approves exactly what they approved before.
                     Rendered ONLY when the dApp set one of these knobs; a default honest send
                     shows nothing (the norm needs no disclosure). Neutral/HUD treatment -- not
                     the warning palette, no padlock. Claimed addresses are shown in full
                     (never middle-folded) so the user can read who the payment will appear from. -->
                {#if request.payload.preferred_decoys?.length || request.payload.anonymize}
                  <div class="modal-tx-field">
                    <div class="modal-tx-label">APPARENT SENDER</div>
                    {#if request.payload.preferred_decoys?.length}
                      <!-- Load-bearing case: dApp named the address(es) the payment will appear from -->
                      <div class="modal-tx-attribution-copy">
                        This payment will appear to come from an address this app specified:
                      </div>
                      {#each request.payload.preferred_decoys as decoyAddr}
                        <div class="modal-tx-destination">{decoyAddr}</div>
                      {/each}
                      <div class="modal-tx-attribution-note">
                        It does not change who actually sent it, the amount, or the recipient.
                      </div>
                    {:else if effectiveRing > 2}
                      <!-- anonymize: appears from an unnamed decoy ring member. The ring is
                           clamped >2 server-side (wallet.go), so this promise holds. -->
                      <div class="modal-tx-attribution-copy">
                        This payment will appear to come from a decoy ring member, not your address (ring size {effectiveRing}).
                      </div>
                    {/if}
                  </div>
                {/if}
              {:else if request.payload.scid}
                <!-- SC call with no explicit transfers - show 0 DERO burn -->
                <div class="modal-tx-field">
                  <div class="modal-tx-label">BURN AMOUNT</div>
                  <div class="modal-tx-amount">0 DERO</div>
                </div>
              {:else}
                <!-- No transfers and no SC - unusual, show warning -->
                <div class="modal-tx-field">
                  <div class="modal-tx-label">AMOUNT</div>
                  <div class="modal-tx-amount modal-tx-amount-zero">0 DERO (no transfer)</div>
                </div>
              {/if}
              
              <!-- SC Arguments (if any) -->
              {#if request.payload.sc_args && request.payload.sc_args.length > 0}
                <div class="wallet-tx-sc-section">
                  <div class="modal-tx-label">SC ARGUMENTS</div>
                  <div class="wallet-tx-sc-args">
                    {#each request.payload.sc_args as arg}
                      <div class="wallet-tx-sc-arg">
                        <span class="wallet-tx-sc-arg-name">{arg.name}:</span>
                        <span class="wallet-tx-sc-arg-value" title={String(arg.value)}>
                          {#if String(arg.value).length > 40}
                            {String(arg.value).slice(0, 40)}...
                          {:else}
                            {arg.value}
                          {/if}
                        </span>
                      </div>
                    {/each}
                  </div>
                </div>
              {:else if request.payload.sc_data && Array.isArray(request.payload.sc_data) && request.payload.sc_data.length > 0}
                <!-- Fallback: show raw sc_data if sc_args not parsed -->
                <div class="wallet-tx-sc-section">
                  <div class="modal-tx-label">SMART CONTRACT DATA</div>
                  <div class="wallet-tx-sc-data">
                    {JSON.stringify(request.payload.sc_data, null, 2)}
                  </div>
                </div>
              {/if}
              
              <!-- Ring size if specified -->
              {#if effectiveRing}
                <div class="modal-tx-field modal-tx-field-secondary">
                  <div class="modal-tx-label">RING SIZE</div>
                  <div class="modal-tx-ringsize">{effectiveRing}</div>
                </div>
              {/if}
            </div>
            
            <div class="modal-alert modal-alert-warning">
              <span class="modal-alert-icon">!</span>
              {#if request.payload.scid}
                Review the smart contract call details before approving.
              {:else}
                Double check the destination and amount before approving.
              {/if}
            </div>
          </div>
        {/if}
      </div>

      <!-- Wallet State Section - only show for non-read-only requests -->
      {#if request.type === 'connect' && !connectNeedsWallet}
        <!-- Nothing here touches the wallet, so approving needs no unlock. There is no
             sheet to select from any more, so the copy must not refer to one. -->
        <div class="wallet-readonly-info">
          <span class="wallet-readonly-info-icon">◎</span>
          <span>No wallet access needed to connect</span>
        </div>
      {:else if $walletState.isOpen}
        <!-- Current wallet is open - show wallet switcher option -->
        <div class="modal-wallet-section">
          <div class="modal-wallet-current-row">
            <div>
              <p class="modal-wallet-label">CURRENT WALLET</p>
              <p class="modal-wallet-address">
                {$addressMasked ? '••••••••••••••••' : `${$walletState.address?.slice(0, 16)}...`}
              </p>
            </div>
            <button
              on:click={() => { showWalletSwitcher = !showWalletSwitcher; loadRecentWallets(); }}
              class="modal-link-btn"
            >
              {showWalletSwitcher ? 'Cancel' : 'Switch Wallet'}
            </button>
          </div>
          
          {#if showWalletSwitcher}
            <div class="wallet-switcher">
              <p class="wallet-switcher-label">SELECT WALLET TO USE</p>
              
              {#if selectedSwitchWallet}
                <!-- Password input for selected wallet -->
                <div class="wallet-switcher-form">
                  <div class="wallet-selected-item">
                    <span class="modal-wallet-icon">◇</span>
                    <div class="modal-wallet-info">
                      <p class="modal-wallet-filename">{selectedSwitchWallet.filename}</p>
                      {#if selectedSwitchWallet.addressPrefix}
                        <p class="modal-wallet-prefix">{selectedSwitchWallet.addressPrefix}</p>
                      {/if}
                    </div>
                  </div>
                  
                  <input 
                    type="password" 
                    bind:value={switchPassword}
                    placeholder="Enter password for this wallet..."
                    class="modal-input"
                    on:keydown={(e) => e.key === 'Enter' && handleSwitchWallet()}
                  />
                  
                  <div class="wallet-btn-row">
                    <button on:click={cancelSwitch} class="modal-btn modal-btn-secondary">
                      Back
                    </button>
                    <button
                      on:click={handleSwitchWallet}
                      disabled={!switchPassword || isLoading}
                      class="modal-btn modal-btn-primary"
                    >
                      {isLoading ? 'Switching...' : 'Switch'}
                    </button>
                  </div>
                </div>
              {:else}
                <!-- Wallet list -->
                <div class="modal-wallet-list">
                  {#each recentWalletsInfo.filter(w => !w.isCurrent) as wallet}
                    <button
                      on:click={() => selectWalletToSwitch(wallet)}
                      class="modal-wallet-list-item"
                    >
                      <span class="modal-wallet-icon">◇</span>
                      <div class="modal-wallet-info">
                        <p class="modal-wallet-filename">{wallet.filename}</p>
                        {#if wallet.addressPrefix}
                          <p class="modal-wallet-prefix">{wallet.addressPrefix}</p>
                        {/if}
                      </div>
                      <span class="wallet-arrow-icon">→</span>
                    </button>
                  {/each}
                  
                  {#if recentWalletsInfo.filter(w => !w.isCurrent).length === 0}
                    <p class="wallet-empty-state">No other wallets found</p>
                  {/if}
                </div>
                
                <!-- Browse for different wallet -->
                <button
                  on:click={async () => {
                    const selected = await SelectWalletFile();
                    if (selected) {
                      selectWalletToSwitch({ path: selected, filename: getWalletFilename(selected), addressPrefix: '' });
                    }
                  }}
                  class="modal-browse-btn"
                >
                  <span>+</span>
                  <span>Browse for wallet file</span>
                </button>
              {/if}
            </div>
          {/if}
        </div>
      {:else}
        <!-- Wallet Lock State - no wallet open -->
        <div class="modal-wallet-section">
          <!-- Wallet File Selection -->
          <div class="modal-form-group">
            <label class="modal-form-label">Wallet File</label>
            <div class="modal-input-with-button">
              <input 
                type="text" 
                bind:value={walletPath}
                placeholder="Select wallet file..."
                class="modal-input"
              />
              <button on:click={browseWallet} class="modal-btn modal-btn-secondary">
                Browse
              </button>
            </div>
            
            <!-- Recent Wallets with Info -->
            {#if recentWalletsInfo.length > 0}
              <div class="wallet-recent-wallets">
                <p class="wallet-recent-label">Recent wallets:</p>
                <div class="modal-wallet-list">
                  {#each recentWalletsInfo.slice(0, 5) as wallet}
                    <button
                      on:click={() => walletPath = wallet.path}
                      class="modal-wallet-list-item {walletPath === wallet.path ? 'modal-wallet-list-item-active' : ''}"
                    >
                      <span class="modal-wallet-icon">{walletPath === wallet.path ? '✓' : '◇'}</span>
                      <div class="modal-wallet-info">
                        <p class="modal-wallet-filename">{wallet.filename}</p>
                        {#if wallet.addressPrefix}
                          <p class="modal-wallet-prefix">{wallet.addressPrefix}</p>
                        {/if}
                      </div>
                    </button>
                  {/each}
                </div>
              </div>
            {:else if recentWallets.length > 0 && !walletPath}
              <div class="wallet-recent-wallets">
                <p class="wallet-recent-label">Recent wallets:</p>
                <div class="wallet-recent-simple-list">
                  {#each recentWallets.slice(0, 3) as recent}
                    <button
                      on:click={() => walletPath = recent}
                      class="wallet-recent-simple-item"
                    >
                      {getWalletFilename(recent)}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
          
          <!-- Password -->
          <div class="modal-form-group">
            <label class="modal-form-label">Wallet Password</label>
            <input 
              type="password" 
              bind:value={password}
              placeholder="Enter wallet password..."
              class="modal-input wallet-password-input"
              on:keydown={(e) => e.key === 'Enter' && !isBurnBlocked && handleApprove()}
            />
          </div>
        </div>
      {/if}

      {#if error}
        <div class="modal-alert modal-alert-error">
          {error}
        </div>
      {/if}
    </div>

    <!-- Actions -->
    <!-- Only the permission sheet offers three answers; two fit the 384px foot, three do not. -->
    <div class="modal-panel-actions" class:modal-panel-actions-stack={request.type === 'permission'}>
      {#if isBurnBlocked}
        <!-- A blocked burn has no approve path at all -- only a way to dismiss it. -->
        <button
          on:click={handleDeny}
          class="modal-panel-btn modal-panel-btn-approve"
          disabled={isLoading}
        >
          Dismiss
        </button>
      {:else if request.type === 'permission'}
        <!-- Allow once · Always allow · Deny, stacked. Only reachable for the storable doors —
             spending never routes here, so "Always allow" cannot appear for a transaction.
             Least commitment first: see the .modal-panel-actions-stack note in hologram.css. -->
        <button
          on:click={() => { rememberChoice = false; handleApprove(); }}
          class="modal-panel-btn modal-panel-btn-once"
          disabled={isLoading}
        >
          {isLoading ? 'Processing...' : 'Allow once'}
        </button>
        <button
          on:click={() => { rememberChoice = true; handleApprove(); }}
          class="modal-panel-btn modal-panel-btn-always"
          disabled={isLoading}
        >
          {isLoading ? 'Processing...' : 'Always allow'}
        </button>
        <div class="modal-panel-actions-sep"></div>
        <button
          on:click={handleDeny}
          class="modal-panel-btn modal-panel-btn-deny"
          disabled={isLoading}
        >
          Deny
        </button>
      {:else}
        <button
          on:click={handleDeny}
          class="modal-panel-btn modal-panel-btn-deny"
          disabled={isLoading}
        >
          Deny
        </button>
        <button
          on:click={handleApprove}
          class="modal-panel-btn modal-panel-btn-approve"
          disabled={isLoading}
        >
          {#if isLoading}
            Processing...
          {:else}
            Approve
          {/if}
        </button>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* WalletModal.svelte - Component-specific styles only
     Modal panel base patterns now in hologram.css (.modal-panel-*) */
  
  /* Request Details Section */
  .wallet-request-details {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
  
  /* Info Note */
  .wallet-info-note {
    margin-top: var(--s-4);
    font-size: 12px;
    color: var(--text-4);
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    line-height: 1.5;
  }
  
  /* Holds the whole sentence as a single flex item so it wraps as prose, not columns. */
  .wallet-info-text {
    flex: 1;
    min-width: 0;
  }

  .wallet-info-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 16px;
    height: 16px;
    border-radius: var(--r-full);
    background: rgba(34, 211, 238, 0.15);
    color: var(--cyan-400);
    font-size: 10px;
    font-weight: 700;
    line-height: 1;
    margin-top: 1px;
  }
  
  /* Fallback Permissions */
  .wallet-fallback-permissions {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    list-style: none;
    padding: 0;
    margin: 0;
  }
  
  .wallet-fallback-item {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    font-size: 13px;
    color: var(--text-3);
    line-height: 1.5;
  }
  
  .wallet-check-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 16px;
    height: 16px;
    border-radius: var(--r-full);
    background: rgba(52, 211, 153, 0.15);
    color: var(--status-ok);
    font-size: 10px;
    line-height: 1;
  }
  
  .wallet-denied-icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 16px;
    height: 16px;
    border-radius: var(--r-full);
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-5);
    font-size: 10px;
    line-height: 1;
  }
  
  /* Read-Only Badge */
  .wallet-readonly-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    background: rgba(34, 211, 238, 0.1);
    border: 1px solid rgba(34, 211, 238, 0.3);
    border-radius: var(--r-md);
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--cyan-400);
    margin-bottom: var(--s-3);
  }
  
  .wallet-readonly-icon {
    font-size: 14px;
  }
  
  .wallet-readonly-desc {
    font-size: 13px;
    color: var(--text-3);
    margin-bottom: var(--s-4);
    line-height: 1.5;
  }
  
  .wallet-readonly-permissions {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }
  
  .wallet-readonly-item {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 13px;
    color: var(--text-3);
    line-height: 1.4;
  }
  
  .wallet-readonly-item-denied {
    color: var(--text-5);
  }
  
  .wallet-readonly-info {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-3);
    background: rgba(34, 211, 238, 0.05);
    border: 1px solid rgba(34, 211, 238, 0.2);
    border-radius: var(--r-md);
    font-size: 13px;
    color: var(--cyan-400);
    margin-top: var(--s-2);
  }
  
  .wallet-readonly-info-icon {
    font-size: 16px;
  }
  
  /* Smart Contract Section */
  .wallet-tx-sc-header {
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
    padding-bottom: var(--s-3);
    margin-bottom: var(--s-3);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .wallet-tx-sc-section {
    padding-top: var(--s-2);
    border-top: 1px solid var(--border-dim);
  }
  
  .wallet-tx-sc-data {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--status-warn);
    white-space: pre-wrap;
  }
  
  .wallet-tx-sc-args {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    margin-top: var(--s-2);
  }
  
  .wallet-tx-sc-arg {
    display: flex;
    gap: var(--s-2);
    font-family: var(--font-mono);
    font-size: 12px;
  }
  
  .wallet-tx-sc-arg-name {
    color: var(--cyan-400);
    font-weight: 500;
    flex-shrink: 0;
  }
  
  .wallet-tx-sc-arg-value {
    color: var(--text-3);
    word-break: break-all;
  }
  
  .modal-tx-entrypoint {
    font-family: var(--font-mono);
    font-size: 14px;
    font-weight: 600;
    color: var(--cyan-400);
    padding: var(--s-2) var(--s-3);
    background: rgba(34, 211, 238, 0.1);
    border-radius: var(--r-md);
    border: 1px solid rgba(34, 211, 238, 0.2);
  }
  
  .modal-tx-scid {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-4);
    cursor: help;
  }
  
  .modal-tx-token-scid {
    font-size: 11px;
    color: var(--text-5);
    margin-left: var(--s-1);
  }
  
  .modal-tx-amount-zero {
    color: var(--text-5);
    font-style: italic;
  }
  
  .modal-tx-amount-total {
    font-size: 16px;
    font-weight: 600;
    color: var(--accent);
  }
  
  .modal-tx-breakdown {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    padding: var(--s-2);
    background: rgba(8, 8, 14, 0.3);
    border-radius: var(--r-sm);
    margin-top: var(--s-1);
  }
  
  .modal-tx-label-small {
    font-size: 9px;
    margin-bottom: var(--s-1);
  }
  
  .modal-tx-breakdown-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-family: var(--font-mono);
    font-size: 11px;
  }
  
  .modal-tx-breakdown-label {
    color: var(--text-5);
  }
  
  .modal-tx-breakdown-value {
    color: var(--text-3);
  }
  
  .modal-tx-ringsize {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-4);
  }
  
  .modal-tx-field-secondary {
    opacity: 0.7;
  }

  /* Attribution disclosure (APPARENT SENDER) -- neutral/informational, not a warning.
     Pairs with the .modal-tx-destination boxed value reused for the address(es). */
  .modal-tx-attribution-copy {
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-3);
  }

  /* Amber, matching the Send confirm step: losing anonymity is a consequence to notice
     before approving, not an alarm. Red stays reserved for what cannot work. */
  .modal-tx-replyback {
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.5;
    color: var(--status-warn);
    margin-top: var(--s-2);
  }

  .modal-tx-attribution-note {
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-4);
  }

  /* Wallet Switcher */
  .wallet-switcher {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
    padding: var(--s-3);
    background: rgba(8, 8, 14, 0.5);
    border-radius: var(--r-lg);
    border: 1px solid var(--border-dim);
  }
  
  .wallet-switcher-label {
    font-family: var(--font-mono);
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.2em;
    color: var(--text-4);
  }
  
  .wallet-switcher-form {
    display: flex;
    flex-direction: column;
    gap: var(--s-3);
  }
  
  .wallet-selected-item {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2);
    background: rgba(6, 182, 212, 0.1);
    border: 1px solid rgba(6, 182, 212, 0.3);
    border-radius: var(--r-md);
  }
  
  .wallet-arrow-icon {
    font-size: 12px;
    color: var(--text-5);
  }
  
  .wallet-empty-state {
    font-size: 12px;
    color: var(--text-4);
    text-align: center;
    padding: var(--s-2);
  }
  
  .wallet-btn-row {
    display: flex;
    gap: var(--s-2);
  }
  
  .wallet-btn-row .modal-btn {
    flex: 1;
  }
  
  /* Recent Wallets */
  .wallet-recent-wallets {
    margin-top: var(--s-3);
  }
  
  .wallet-recent-label {
    font-size: 12px;
    color: var(--text-4);
    margin-bottom: var(--s-2);
  }
  
  .wallet-recent-simple-list {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }
  
  .wallet-recent-simple-item {
    width: 100%;
    text-align: left;
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-4);
    padding: var(--s-1) var(--s-2);
    border-radius: var(--r-sm);
    background: transparent;
    border: none;
    cursor: pointer;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: all 200ms ease-out;
  }
  
  .wallet-recent-simple-item:hover {
    color: var(--text-2);
    background: var(--void-up);
  }
  
  /* Password Input (larger for main unlock) */
  .wallet-password-input {
    padding: var(--s-3) var(--s-4);
  }
  
  /* Scrollbar styling */
  ::-webkit-scrollbar {
    width: 6px;
  }
  ::-webkit-scrollbar-track {
    background: transparent;
  }
  ::-webkit-scrollbar-thumb {
    background: var(--void-hover);
    border-radius: var(--r-xs);
  }
</style>