<script>
  import { createEventDispatcher } from 'svelte';
  import { ParseSCFunctions, InvokeSCFunction } from '../../../wailsjs/go/main/App.js';
  import { walletState, toast } from '../stores/appState.js';
  import { Loader2, Play, AlertTriangle, ChevronDown, Zap, Check, Copy } from 'lucide-svelte';

  export let scid = '';

  const dispatch = createEventDispatcher();

  let functions = [];
  let selectedFunction = null;
  let selectedFunctionName = '';
  let paramValues = {};
  let deroAmount = '';
  let assetScid = '';
  let assetAmount = '';
  let anonymous = false;
  let loading = false;
  let parsing = false;
  let error = '';
  let result = null;
  let copied = false;

  // Parse functions when SCID changes
  $: if (scid && scid.length === 64) {
    parseFunctions();
  }

  async function parseFunctions() {
    parsing = true;
    error = '';
    functions = [];
    selectedFunction = null;
    selectedFunctionName = '';
    result = null;

    try {
      const res = await ParseSCFunctions(scid);
      if (res.success) {
        functions = res.functions || [];
        if (functions.length > 0) {
          selectFunction(functions[0]);
        }
      } else {
        error = res.error || 'Failed to parse functions';
      }
    } catch (e) {
      error = e.message || 'Failed to parse smart contract';
    } finally {
      parsing = false;
    }
  }

  function selectFunction(fn) {
    selectedFunction = fn;
    selectedFunctionName = fn.name;
    paramValues = {};
    // Initialize param values
    fn.params.forEach(p => {
      paramValues[p.name] = '';
    });
    // Reset amounts
    deroAmount = '';
    assetScid = '';
    assetAmount = '';
    result = null;
    error = '';
    // Disable anonymous if SIGNER is used
    if (fn.usesSigner) {
      anonymous = false;
    }
  }

  function handleFunctionChange(event) {
    const fn = functions.find(f => f.name === event.target.value);
    if (fn) {
      selectFunction(fn);
    }
  }

  async function invokeFunction() {
    if (!selectedFunction) return;

    loading = true;
    error = '';
    result = null;

    try {
      // Build params object with correct types
      const params = {};
      selectedFunction.params.forEach(p => {
        const val = paramValues[p.name];
        if (p.type === 'Uint64') {
          params[p.name] = parseInt(val) || 0;
        } else {
          params[p.name] = val || '';
        }
      });

      const invokeParams = {
        scid: scid,
        function: selectedFunction.name,
        params: params,
        deroAmount: deroAmount ? Math.floor(parseFloat(deroAmount) * 100000) : 0,
        assetScid: assetScid,
        assetAmount: assetAmount ? parseInt(assetAmount) : 0,
        anonymous: anonymous
      };

      const res = await InvokeSCFunction(JSON.stringify(invokeParams));
      
      if (res.success) {
        result = res;
        toast.success(`Function ${selectedFunction.name} called successfully!`);
        dispatch('invoked', res);
      } else {
        error = res.error || 'Invocation failed';
        toast.error(error);
      }
    } catch (e) {
      error = e.message || 'Failed to invoke function';
      toast.error(error);
    } finally {
      loading = false;
    }
  }

  function copyTxid() {
    if (result?.txid) {
      navigator.clipboard.writeText(result.txid);
      copied = true;
      setTimeout(() => copied = false, 2000);
    }
  }

  function formatFunctionSignature(fn) {
    const params = fn.params.map(p => `${p.name}: ${p.type}`).join(', ');
    return `${fn.name}(${params})`;
  }
</script>

<div class="sc-interactor">
  <div class="interactor-header">
    <span class="interactor-icon"><Zap size={16} /></span>
    <h3>Call Smart Contract Function</h3>
  </div>

  {#if parsing}
    <div class="parsing-state">
      <Loader2 size={18} class="spin" />
      <span>Parsing smart contract...</span>
    </div>
  {:else if error && functions.length === 0}
    <div class="error-state">
      <AlertTriangle size={18} />
      <span>{error}</span>
    </div>
  {:else if functions.length === 0}
    <div class="empty-state">
      <span>No callable functions found in this contract</span>
    </div>
  {:else}
    <!-- Function Selector -->
    <div class="function-selector">
      <label>Function</label>
      <div class="select-wrapper">
        <select bind:value={selectedFunctionName} on:change={handleFunctionChange}>
          {#each functions as fn}
            <option value={fn.name}>
              {formatFunctionSignature(fn)}
            </option>
          {/each}
        </select>
        <ChevronDown size={16} class="select-icon" />
      </div>
    </div>

    {#if selectedFunction}
      <!-- Parameters -->
      {#if selectedFunction.params.length > 0}
        <div class="params-section">
          <label class="section-label">Parameters</label>
          {#each selectedFunction.params as param}
            <div class="param-row">
              <div class="param-info">
                <span class="param-name">{param.name}</span>
                <span class="param-type">{param.type}</span>
              </div>
              {#if param.type === 'Uint64'}
                <input
                  type="number"
                  bind:value={paramValues[param.name]}
                  placeholder="0"
                  class="param-input"
                />
              {:else}
                <input
                  type="text"
                  bind:value={paramValues[param.name]}
                  placeholder="Enter value..."
                  class="param-input"
                />
              {/if}
            </div>
          {/each}
        </div>
      {:else}
        <div class="no-params">
          <span>This function has no parameters</span>
        </div>
      {/if}

      <!-- DERO Value (if detected) -->
      {#if selectedFunction.usesDero}
        <div class="value-section dero-value">
          <label>
            <span class="value-badge dero">DEROVALUE</span>
            DERO Amount to Send
          </label>
          <div class="value-input-row">
            <input
              type="number"
              step="0.00001"
              min="0"
              bind:value={deroAmount}
              placeholder="0.00000"
            />
            <span class="unit">DERO</span>
          </div>
        </div>
      {/if}

      <!-- Asset Value (if detected) -->
      {#if selectedFunction.usesAsset}
        <div class="value-section asset-value">
          <label>
            <span class="value-badge asset">ASSETVALUE</span>
            Token Transfer
          </label>
          <input
            type="text"
            bind:value={assetScid}
            placeholder="Token SCID (64 hex characters)"
            class="asset-scid-input"
          />
          <div class="value-input-row">
            <input
              type="number"
              min="0"
              bind:value={assetAmount}
              placeholder="Amount (atomic units)"
            />
            <span class="unit">units</span>
          </div>
        </div>
      {/if}

      <!-- Anonymous Option -->
      <div class="anonymous-section">
        <label class="checkbox-label">
          <input
            type="checkbox"
            bind:checked={anonymous}
            disabled={selectedFunction.usesSigner}
          />
          <span>Anonymous transaction (ringsize 16)</span>
        </label>
        {#if selectedFunction.usesSigner}
          <div class="signer-warning">
            <AlertTriangle size={14} />
            <span>SIGNER() detected - anonymous mode disabled</span>
          </div>
        {/if}
      </div>

      <!-- Error Display -->
      {#if error}
        <div class="invoke-error">
          <AlertTriangle size={16} />
          <span>{error}</span>
        </div>
      {/if}

      <!-- Result Display -->
      {#if result}
        <div class="invoke-result">
          <div class="result-header">
            <Check size={16} />
            <span>Transaction Sent</span>
          </div>
          <div class="result-txid">
            <span class="txid-label">TXID:</span>
            <code class="txid">{result.txid}</code>
            <button class="copy-btn" on:click={copyTxid} title="Copy TXID">
              {#if copied}
                <Check size={14} />
              {:else}
                <Copy size={14} />
              {/if}
            </button>
          </div>
        </div>
      {/if}

      <!-- Submit Button -->
      <button
        class="invoke-btn"
        on:click={invokeFunction}
        disabled={loading || (!$walletState.isOpen && !$walletState.xswdConnected)}
      >
        {#if loading}
          <Loader2 size={16} class="spin" />
          <span>Calling {selectedFunction.name}...</span>
        {:else}
          <Play size={16} />
          <span>Call {selectedFunction.name}</span>
        {/if}
      </button>

      {#if !$walletState.isOpen && !$walletState.xswdConnected}
        <p class="wallet-warning">Open a wallet or connect via XSWD to call functions</p>
      {/if}
    {/if}
  {/if}
</div>

<style>
  .sc-interactor {
    background: var(--void-mid);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-lg);
    padding: var(--s-4);
    margin-top: var(--s-4);
  }

  .interactor-header {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    margin-bottom: var(--s-4);
    padding-bottom: var(--s-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .interactor-header h3 {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-1);
    margin: 0;
  }

  .interactor-icon {
    color: var(--accent);
  }

  .parsing-state,
  .error-state,
  .empty-state {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-3);
    color: var(--text-muted);
    font-size: 13px;
  }

  .error-state {
    color: var(--status-error);
    background: rgba(255, 100, 100, 0.1);
    border-radius: var(--r-md);
  }

  .function-selector {
    margin-bottom: var(--s-4);
  }

  .function-selector label,
  .section-label {
    display: block;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: var(--s-2);
  }

  .select-wrapper {
    position: relative;
  }

  .select-wrapper select {
    width: 100%;
    padding: var(--s-2) var(--s-3);
    padding-right: var(--s-6);
    background: var(--void-up);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-md);
    color: var(--text-1);
    font-size: 13px;
    font-family: var(--font-mono);
    appearance: none;
    cursor: pointer;
  }

  .select-wrapper select:focus {
    outline: none;
    border-color: var(--accent);
  }

  .select-wrapper :global(.select-icon) {
    position: absolute;
    right: var(--s-3);
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-muted);
    pointer-events: none;
  }

  .params-section {
    margin-bottom: var(--s-4);
  }

  .no-params {
    padding: var(--s-3);
    background: var(--void-down);
    border-radius: var(--r-md);
    font-size: 12px;
    color: var(--text-muted);
    margin-bottom: var(--s-4);
  }

  .param-row {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
    margin-bottom: var(--s-3);
  }

  .param-info {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }

  .param-name {
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-2);
    font-weight: 500;
  }

  .param-type {
    font-size: 10px;
    padding: 2px 6px;
    background: var(--void-up);
    border-radius: var(--r-sm);
    color: var(--text-muted);
    font-family: var(--font-mono);
  }

  .param-input {
    padding: var(--s-2) var(--s-3);
    background: var(--void-up);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-md);
    color: var(--text-1);
    font-size: 13px;
    font-family: var(--font-mono);
    width: 100%;
  }

  .param-input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .value-section {
    margin-bottom: var(--s-3);
    padding: var(--s-3);
    background: var(--void-down);
    border-radius: var(--r-md);
    border: 1px solid var(--border-subtle);
  }

  .value-section label {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 12px;
    color: var(--text-2);
    margin-bottom: var(--s-2);
  }

  .value-badge {
    display: inline-block;
    padding: 2px 6px;
    border-radius: var(--r-sm);
    font-size: 9px;
    font-weight: 700;
    font-family: var(--font-mono);
  }

  .value-badge.dero {
    background: var(--accent);
    color: var(--void);
  }

  .value-badge.asset {
    background: #9b59b6;
    color: white;
  }

  .value-input-row {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }

  .value-input-row input {
    flex: 1;
    padding: var(--s-2) var(--s-3);
    background: var(--void-up);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-md);
    color: var(--text-1);
    font-size: 13px;
    font-family: var(--font-mono);
  }

  .value-input-row input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .asset-scid-input {
    width: 100%;
    padding: var(--s-2) var(--s-3);
    background: var(--void-up);
    border: 1px solid var(--border-subtle);
    border-radius: var(--r-md);
    color: var(--text-1);
    font-size: 12px;
    font-family: var(--font-mono);
    margin-bottom: var(--s-2);
  }

  .asset-scid-input:focus {
    outline: none;
    border-color: var(--accent);
  }

  .unit {
    font-size: 12px;
    color: var(--text-muted);
    font-weight: 500;
  }

  .anonymous-section {
    margin-bottom: var(--s-4);
    padding: var(--s-3);
    background: var(--void-down);
    border-radius: var(--r-md);
  }

  .checkbox-label {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    cursor: pointer;
    font-size: 13px;
    color: var(--text-2);
  }

  .checkbox-label input[type="checkbox"] {
    width: 16px;
    height: 16px;
    cursor: pointer;
    accent-color: var(--accent);
  }

  .checkbox-label input[type="checkbox"]:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .signer-warning {
    display: flex;
    align-items: center;
    gap: var(--s-1);
    margin-top: var(--s-2);
    font-size: 11px;
    color: var(--status-warning);
  }

  .invoke-error {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    padding: var(--s-3);
    background: rgba(255, 100, 100, 0.1);
    border: 1px solid var(--status-error);
    border-radius: var(--r-md);
    color: var(--status-error);
    font-size: 12px;
    margin-bottom: var(--s-3);
  }

  .invoke-result {
    padding: var(--s-3);
    background: rgba(100, 255, 150, 0.1);
    border: 1px solid var(--status-success);
    border-radius: var(--r-md);
    margin-bottom: var(--s-3);
  }

  .result-header {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    color: var(--status-success);
    font-size: 13px;
    font-weight: 600;
    margin-bottom: var(--s-2);
  }

  .result-txid {
    display: flex;
    align-items: center;
    gap: var(--s-2);
  }

  .txid-label {
    font-size: 11px;
    color: var(--text-muted);
  }

  .txid {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-2);
    word-break: break-all;
    flex: 1;
  }

  .copy-btn {
    padding: var(--s-1);
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    border-radius: var(--r-sm);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .copy-btn:hover {
    color: var(--text-1);
    background: var(--void-up);
  }

  .invoke-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-2);
    width: 100%;
    padding: var(--s-3);
    background: var(--accent);
    border: none;
    border-radius: var(--r-md);
    color: var(--void);
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s ease;
  }

  .invoke-btn:hover:not(:disabled) {
    filter: brightness(1.1);
  }

  .invoke-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .wallet-warning {
    text-align: center;
    font-size: 12px;
    color: var(--text-muted);
    margin-top: var(--s-2);
    margin-bottom: 0;
  }

  :global(.spin) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }
</style>

