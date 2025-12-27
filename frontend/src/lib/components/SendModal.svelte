<script>
  import { createEventDispatcher } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { walletState, toast, handleBackendError } from '../stores/appState.js';
  import { InternalWalletCall } from '../../../wailsjs/go/main/App.js';
  import { X, ArrowUp, Eye, EyeOff, AlertTriangle, Check, Loader } from 'lucide-svelte';

  export let show = false;

  const dispatch = createEventDispatcher();
  
  // Form state
  let destination = '';
  let amount = '';
  let password = '';
  let showPassword = false;
  
  // UI state
  let step = 'form'; // 'form' | 'confirm' | 'sending' | 'success' | 'error'
  let error = null;
  let txid = null;
  
  // Computed
  $: availableBalance = $walletState.balance / 100000;
  $: amountAtomic = Math.floor(parseFloat(amount || '0') * 100000);
  $: isValidAmount = !isNaN(amountAtomic) && amountAtomic > 0 && amountAtomic <= $walletState.balance;
  $: isValidAddress = destination.startsWith('dero1') || destination.startsWith('deto1');
  $: canSubmit = isValidAmount && isValidAddress && password.length > 0;
  
  function close() {
    // Reset state
    destination = '';
    amount = '';
    password = '';
    step = 'form';
    error = null;
    txid = null;
    dispatch('close');
  }
  
  function setMax() {
    // Leave a small buffer for fees (0.00001 DERO = 1 atomic unit minimum)
    const maxAmount = Math.max(0, ($walletState.balance - 100) / 100000);
    amount = maxAmount.toFixed(5);
  }
  
  function reviewTransaction() {
    if (!canSubmit) return;
    error = null;
    step = 'confirm';
  }
  
  function backToForm() {
    step = 'form';
  }
  
  async function sendTransaction() {
    step = 'sending';
    error = null;
    
    try {
      const params = {
        transfers: [{
          destination: destination,
          amount: amountAtomic
        }]
      };
      
      const result = await InternalWalletCall('transfer', params, password);
      
      if (result.success) {
        txid = result.txid;
        step = 'success';
        toast.success('Transaction sent successfully!');
      } else {
        // Use friendly error from backend, log technical details
        error = handleBackendError(result, { showToast: false }) || 'Transaction failed';
        step = 'error';
      }
    } catch (err) {
      console.error('[Transaction Exception]', err);
      error = err.message || 'Transaction failed';
      step = 'error';
    }
  }
  
  function formatAmount(atomic) {
    return (atomic / 100000).toFixed(5);
  }
</script>

{#if show}
  <div class="modal-overlay" transition:fade={{ duration: 150 }} on:click={close}>
    <div class="modal-content" transition:fly={{ y: 20, duration: 200 }} on:click|stopPropagation>
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-title">
          <ArrowUp size={20} class="modal-title-icon" />
          <span>Send DERO</span>
        </div>
        <button class="modal-close" on:click={close}>
          <X size={18} />
        </button>
      </div>
      
      <!-- Body -->
      <div class="modal-body">
        {#if step === 'form'}
          <!-- Destination -->
          <div class="modal-form-group">
            <label class="modal-form-label">Recipient Address</label>
            <input
              type="text"
              bind:value={destination}
              placeholder="dero1..."
              class="modal-input"
              class:modal-input-error={destination && !isValidAddress}
            />
            {#if destination && !isValidAddress}
              <span class="modal-form-error">Invalid DERO address</span>
            {/if}
          </div>
          
          <!-- Amount -->
          <div class="modal-form-group">
            <div class="modal-form-label-row">
              <label class="modal-form-label">Amount</label>
              <span class="modal-form-hint">Available: {availableBalance.toFixed(5)} DERO</span>
            </div>
            <div class="modal-input-with-button">
              <input
                type="number"
                bind:value={amount}
                placeholder="0.00000"
                step="0.00001"
                min="0"
                class="modal-input"
                class:modal-input-error={amount && !isValidAmount}
              />
              <button class="modal-btn modal-btn-sm modal-btn-secondary" on:click={setMax}>MAX</button>
            </div>
            {#if amount && !isValidAmount}
              <span class="modal-form-error">
                {amountAtomic <= 0 ? 'Amount must be positive' : 'Insufficient balance'}
              </span>
            {/if}
          </div>
          
          <!-- Password -->
          <div class="modal-form-group">
            <label class="modal-form-label">Wallet Password</label>
            <div class="modal-input-wrap">
              {#if showPassword}
                <input type="text" bind:value={password} placeholder="Enter password" class="modal-input" />
              {:else}
                <input type="password" bind:value={password} placeholder="Enter password" class="modal-input" />
              {/if}
              <button type="button" class="modal-input-action" on:click={() => showPassword = !showPassword}>
                {#if showPassword}
                  <EyeOff size={16} />
                {:else}
                  <Eye size={16} />
                {/if}
              </button>
            </div>
          </div>
          
        {:else if step === 'confirm'}
          <div class="modal-confirm-section">
            <div class="modal-confirm-warning">
              <AlertTriangle size={24} class="modal-confirm-warning-icon" />
              <p>Please verify the transaction details before sending.</p>
            </div>
            
            <div class="modal-confirm-details">
              <div class="modal-confirm-row">
                <span class="modal-confirm-label">To</span>
                <span class="modal-confirm-value modal-confirm-value-address">{destination.slice(0, 20)}...{destination.slice(-8)}</span>
              </div>
              <div class="modal-confirm-row">
                <span class="modal-confirm-label">Amount</span>
                <span class="modal-confirm-value modal-confirm-value-amount">{formatAmount(amountAtomic)} DERO</span>
              </div>
              <div class="modal-confirm-row">
                <span class="modal-confirm-label">From Balance</span>
                <span class="modal-confirm-value">{availableBalance.toFixed(5)} DERO</span>
              </div>
            </div>
          </div>
          
        {:else if step === 'sending'}
          <div class="modal-status-section">
            <div class="modal-status-icon modal-status-icon-loading">
              <Loader size={48} class="modal-spinner" />
            </div>
            <p class="modal-status-title">Sending transaction...</p>
            <p class="modal-status-text">Please wait while your transaction is being processed.</p>
          </div>
          
        {:else if step === 'success'}
          <div class="modal-status-section">
            <div class="modal-status-icon modal-status-icon-success">
              <Check size={48} />
            </div>
            <p class="modal-status-title">Transaction Sent!</p>
            <div class="modal-txid-display">
              <span class="modal-txid-label">Transaction ID:</span>
              <code class="modal-txid-value">{txid?.slice(0, 16)}...{txid?.slice(-8)}</code>
            </div>
          </div>
          
        {:else if step === 'error'}
          <div class="modal-status-section">
            <div class="modal-status-icon modal-status-icon-error">
              <AlertTriangle size={48} />
            </div>
            <p class="modal-status-title">Transaction Failed</p>
            <p class="modal-status-error">{error}</p>
          </div>
        {/if}
      </div>
      
      <!-- Footer -->
      <div class="modal-footer">
        {#if step === 'form'}
          <button class="modal-btn modal-btn-secondary" on:click={close}>Cancel</button>
          <button class="modal-btn modal-btn-primary" disabled={!canSubmit} on:click={reviewTransaction}>
            Review Transaction
          </button>
        {:else if step === 'confirm'}
          <button class="modal-btn modal-btn-secondary" on:click={backToForm}>Back</button>
          <button class="modal-btn modal-btn-gradient" on:click={sendTransaction}>
            Confirm & Send
          </button>
        {:else if step === 'sending'}
          <!-- No buttons while sending -->
        {:else}
          <button class="modal-btn modal-btn-primary" on:click={close}>Close</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  /* SendModal.svelte - Component-specific styles only
     Modal base patterns now in hologram.css (.modal-*) */
  
  /* Confirm step layout */
  .modal-confirm-section {
    display: flex;
    flex-direction: column;
    gap: var(--s-4);
  }
</style>

