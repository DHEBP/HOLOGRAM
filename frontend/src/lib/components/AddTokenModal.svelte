<script>
  import { createEventDispatcher } from 'svelte';
  import { AddTrackedToken } from '../../../wailsjs/go/main/App.js';
  import { toast } from '../stores/appState.js';
  import { X, Coins, Loader2, AlertTriangle } from 'lucide-svelte';
  
  export let show = false;
  
  const dispatch = createEventDispatcher();
  
  let scid = '';
  let name = '';
  let symbol = '';
  let loading = false;
  let error = null;
  
  $: isValidSCID = scid.length === 64 && /^[0-9a-fA-F]+$/.test(scid);
  
  function close() {
    show = false;
    scid = '';
    name = '';
    symbol = '';
    error = null;
    dispatch('close');
  }
  
  async function addToken() {
    if (!isValidSCID) {
      error = 'Invalid SCID format';
      return;
    }
    
    loading = true;
    error = null;
    
    try {
      const result = await AddTrackedToken(scid, name, symbol);
      if (result.success) {
        toast.success('Token added to portfolio!');
        dispatch('added', result.token);
        close();
      } else {
        error = result.error || 'Failed to add token';
      }
    } catch (err) {
      error = err.message || 'Failed to add token';
    } finally {
      loading = false;
    }
  }
  
  function handleKeydown(e) {
    if (e.key === 'Escape') close();
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if show}
  <div class="modal-overlay" on:click|self={close}>
    <div class="modal">
      <div class="modal-header">
        <div class="modal-title">
          <Coins size={18} />
          <span>Add Token</span>
        </div>
        <button class="modal-close" on:click={close}>
          <X size={18} />
        </button>
      </div>
      
      <div class="modal-body">
        <p class="modal-desc">
          Enter the Smart Contract ID (SCID) of the token you want to track.
        </p>
        
        <div class="form-group">
          <label class="form-label">Token SCID *</label>
          <input 
            type="text" 
            class="input mono" 
            class:input-error={scid && !isValidSCID}
            bind:value={scid} 
            placeholder="Enter 64-character hex SCID..."
            maxlength="64"
          />
          {#if scid && !isValidSCID}
            <span class="form-error">SCID must be 64 hexadecimal characters</span>
          {:else}
            <span class="form-hint">{scid.length}/64 characters</span>
          {/if}
        </div>
        
        <div class="form-row">
          <div class="form-group">
            <label class="form-label">Name (optional)</label>
            <input 
              type="text" 
              class="input" 
              bind:value={name} 
              placeholder="Token Name"
            />
          </div>
          
          <div class="form-group">
            <label class="form-label">Symbol (optional)</label>
            <input 
              type="text" 
              class="input" 
              bind:value={symbol} 
              placeholder="TKN"
              maxlength="10"
            />
          </div>
        </div>
        
        <div class="info-box">
          <AlertTriangle size={14} />
          <span>Token metadata (name, symbol) will be auto-fetched from the blockchain if available.</span>
        </div>
        
        {#if error}
          <div class="alert alert-error">
            <AlertTriangle size={14} />
            <span>{error}</span>
          </div>
        {/if}
      </div>
      
      <div class="modal-footer">
        <button class="btn btn-ghost" on:click={close}>Cancel</button>
        <button class="btn btn-primary" disabled={!isValidSCID || loading} on:click={addToken}>
          {#if loading}
            <Loader2 size={14} class="spin" />
            Adding...
          {:else}
            Add Token
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.7);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: var(--s-4);
  }
  
  .modal {
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-lg);
    width: 100%;
    max-width: 480px;
    max-height: 90vh;
    overflow: hidden;
    display: flex;
    flex-direction: column;
  }
  
  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--s-4);
    border-bottom: 1px solid var(--border-dim);
  }
  
  .modal-title {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    font-size: 14px;
    font-weight: 600;
    color: var(--text-1);
  }
  
  .modal-close {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: transparent;
    border: none;
    border-radius: var(--r-sm);
    color: var(--text-3);
    cursor: pointer;
    transition: all 150ms ease;
  }
  
  .modal-close:hover {
    background: var(--void-up);
    color: var(--text-1);
  }
  
  .modal-body {
    padding: var(--s-4);
    overflow-y: auto;
  }
  
  .modal-desc {
    font-size: 13px;
    color: var(--text-3);
    margin: 0 0 var(--s-4) 0;
    line-height: 1.5;
  }
  
  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--s-3);
  }
  
  .mono {
    font-family: var(--font-mono);
    font-size: 11px;
  }
  
  .info-box {
    display: flex;
    align-items: flex-start;
    gap: var(--s-2);
    padding: var(--s-3);
    background: rgba(34, 211, 238, 0.05);
    border: 1px solid rgba(34, 211, 238, 0.1);
    border-radius: var(--r-md);
    font-size: 11px;
    color: var(--text-3);
    margin-top: var(--s-4);
  }
  
  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: var(--s-3);
    padding: var(--s-4);
    border-top: 1px solid var(--border-dim);
  }
  
  :global(.spin) {
    animation: spin 1s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }
</style>
