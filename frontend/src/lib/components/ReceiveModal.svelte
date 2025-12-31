<script>
  import { createEventDispatcher } from 'svelte';
  import { fly, fade } from 'svelte/transition';
  import { walletState, toast } from '../stores/appState.js';
  import { X, ArrowDown, Copy, Check } from 'lucide-svelte';

  export let show = false;

  const dispatch = createEventDispatcher();
  
  let copied = false;
  
  $: address = $walletState.address || '';
  
  function close() {
    dispatch('close');
  }
  
  async function copyAddress() {
    if (!address) return;
    
    try {
      await navigator.clipboard.writeText(address);
      copied = true;
      toast.success('Address copied to clipboard!', 2000);
      
      setTimeout(() => {
        copied = false;
      }, 2000);
    } catch (err) {
      toast.error('Failed to copy address');
    }
  }
  
  // Split address into chunks for display
  function formatAddressDisplay(addr) {
    if (!addr) return [];
    const chunkSize = 22;
    const chunks = [];
    for (let i = 0; i < addr.length; i += chunkSize) {
      chunks.push(addr.slice(i, i + chunkSize));
    }
    return chunks;
  }
  
  $: addressChunks = formatAddressDisplay(address);
</script>

{#if show}
  <div class="modal-overlay" transition:fade={{ duration: 150 }} on:click={close}>
    <div class="modal-content" transition:fly={{ y: 20, duration: 200 }} on:click|stopPropagation>
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-title">
          <ArrowDown size={20} class="modal-title-icon" />
          <span>Receive DERO</span>
        </div>
        <button class="modal-close" on:click={close}>
          <X size={18} />
        </button>
      </div>
      
      <!-- Body -->
      <div class="modal-body">
        <div class="modal-center">
          <p class="modal-section-label">Your DERO Address</p>
          
          <div class="modal-address-display">
            {#each addressChunks as chunk, i}
              <span class="modal-address-chunk">{chunk}</span>
            {/each}
          </div>
          
          <button class="modal-copy-btn" class:modal-copy-btn-copied={copied} on:click={copyAddress}>
            {#if copied}
              <Check size={16} />
              <span>Copied!</span>
            {:else}
              <Copy size={16} />
              <span>Copy Address</span>
            {/if}
          </button>
        </div>
        
        <div class="modal-info-section">
          <div class="modal-info-item">
            <span class="modal-info-icon"><Info size={14} /></span>
            <span class="modal-info-text">Share this address to receive DERO payments.</span>
          </div>
          <div class="modal-info-item modal-info-item-warning">
            <span class="modal-info-icon"><AlertTriangle size={14} /></span>
            <span class="modal-info-text">Only send DERO to this address. Sending other cryptocurrencies may result in permanent loss.</span>
          </div>
        </div>
      </div>
      
      <!-- Footer -->
      <div class="modal-footer">
        <button class="modal-btn modal-btn-primary" on:click={close}>Done</button>
      </div>
    </div>
  </div>
{/if}

<style>
  /* ReceiveModal.svelte - Component-specific styles only
     Modal base patterns now in hologram.css (.modal-*) */
</style>

