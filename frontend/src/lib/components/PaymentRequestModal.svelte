<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { CreatePaymentRequest, DecodeIntegratedAddress, GetXSWDStatus } from '../../../wailsjs/go/main/App.js';
  import { X, Copy, Check, QrCode, ArrowLeftRight, Loader2, Download } from 'lucide-svelte';
  import { toast } from '../stores/appState.js';
  import QRCode from 'qrcode';
  
  export let show = false;
  
  const dispatch = createEventDispatcher();
  
  // Mode: 'create' or 'decode'
  let mode = 'create';
  
  // Create mode state
  let amount = '';
  let comment = '';
  let generatedAddress = '';
  let creating = false;
  let qrCodeDataUrl = '';
  
  // Decode mode state
  let addressToDecode = '';
  let decodedResult = null;
  let decoding = false;
  
  // Common
  let copied = false;
  let xswdConnected = false;
  
  $: if (show) {
    checkXSWD();
    reset();
  }
  
  async function checkXSWD() {
    try {
      const status = await GetXSWDStatus();
      xswdConnected = status.connected;
    } catch (e) {
      xswdConnected = false;
    }
  }
  
  function reset() {
    amount = '';
    comment = '';
    generatedAddress = '';
    qrCodeDataUrl = '';
    addressToDecode = '';
    decodedResult = null;
    copied = false;
    creating = false;
    decoding = false;
  }
  
  async function generateQRCode(address) {
    try {
      // Generate QR code as data URL with DERO-friendly styling
      qrCodeDataUrl = await QRCode.toDataURL(address, {
        width: 200,
        margin: 2,
        color: {
          dark: '#22d3ee',  // Cyan for dark modules
          light: '#0a0a0f'  // Void for background
        },
        errorCorrectionLevel: 'M'
      });
    } catch (e) {
      console.error('Failed to generate QR code:', e);
      qrCodeDataUrl = '';
    }
  }
  
  function downloadQRCode() {
    if (!qrCodeDataUrl) return;
    
    const link = document.createElement('a');
    link.href = qrCodeDataUrl;
    link.download = `dero-payment-${Date.now()}.png`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    toast.success('QR code downloaded!');
  }
  
  function close() {
    show = false;
    dispatch('close');
  }
  
  async function createRequest() {
    if (!xswdConnected) {
      toast.error('Wallet not connected via XSWD');
      return;
    }
    
    creating = true;
    qrCodeDataUrl = '';
    
    try {
      // Convert amount to atomic units (5 decimal places)
      const atomicAmount = Math.floor(parseFloat(amount || '0') * 100000);
      
      const result = await CreatePaymentRequest(atomicAmount, comment);
      
      if (result.success) {
        generatedAddress = result.integrated_address;
        // Generate QR code for the address
        await generateQRCode(generatedAddress);
        toast.success('Payment request created!');
      } else {
        toast.error(result.error || 'Failed to create payment request');
      }
    } catch (e) {
      toast.error(e.message || 'Failed to create payment request');
    } finally {
      creating = false;
    }
  }
  
  async function decodeAddress() {
    if (!xswdConnected) {
      toast.error('Wallet not connected via XSWD');
      return;
    }
    
    if (!addressToDecode.trim()) {
      toast.error('Please enter an address to decode');
      return;
    }
    
    decoding = true;
    decodedResult = null;
    
    try {
      const result = await DecodeIntegratedAddress(addressToDecode.trim());
      
      if (result.success) {
        decodedResult = result.decoded;
      } else {
        toast.error(result.error || 'Failed to decode address');
      }
    } catch (e) {
      toast.error(e.message || 'Failed to decode address');
    } finally {
      decoding = false;
    }
  }
  
  async function copyAddress() {
    if (generatedAddress) {
      await navigator.clipboard.writeText(generatedAddress);
      copied = true;
      toast.success('Address copied!');
      setTimeout(() => copied = false, 2000);
    }
  }
  
  function getPayloadValue(payload, name) {
    if (!payload) return null;
    const item = payload.find(p => p.name === name);
    return item ? item.value : null;
  }
  
  function formatAtomicAmount(atomic) {
    if (!atomic) return '0';
    return (atomic / 100000).toFixed(5);
  }
</script>

{#if show}
  <div class="modal-overlay" on:click={close}>
    <div class="modal-content modal-content-wide" on:click|stopPropagation>
      <!-- Header -->
      <div class="modal-header">
        <div class="modal-title">Payment Tools</div>
        <button class="modal-close" on:click={close}>
          <X size={18} strokeWidth={1.5} />
        </button>
      </div>
      
      <!-- Mode Tabs -->
      <div class="payment-mode-tabs">
        <button class:active={mode === 'create'} on:click={() => { mode = 'create'; decodedResult = null; }}>
          <QrCode size={14} />
          Create Request
        </button>
        <button class:active={mode === 'decode'} on:click={() => { mode = 'decode'; generatedAddress = ''; }}>
          <ArrowLeftRight size={14} />
          Decode Address
        </button>
      </div>
      
      {#if !xswdConnected}
        <div class="modal-alert modal-alert-warning" style="margin: var(--s-8) var(--s-5); justify-content: center;">
          <p>⚠️ Connect to XSWD wallet to use payment tools</p>
        </div>
      {:else}
        <div class="modal-body">
          {#if mode === 'create'}
            <!-- Create Payment Request -->
            <div class="modal-form-group">
              <label class="modal-form-label" for="amount">Amount (DERO)</label>
              <input 
                id="amount"
                type="number" 
                bind:value={amount} 
                placeholder="0.00000"
                step="0.00001"
                min="0"
                class="modal-input"
              />
            </div>
            
            <div class="modal-form-group">
              <label class="modal-form-label" for="comment">Comment (optional)</label>
              <input 
                id="comment"
                type="text" 
                bind:value={comment} 
                placeholder="Payment description..."
                maxlength="64"
                class="modal-input"
              />
            </div>
            
            <button class="modal-btn modal-btn-primary payment-action-btn" on:click={createRequest} disabled={creating}>
              {#if creating}
                <Loader2 size={14} class="modal-spinner" />
                Creating...
              {:else}
                Generate Payment Address
              {/if}
            </button>
            
            {#if generatedAddress}
              <div class="payment-result-box">
                <!-- QR Code Display -->
                {#if qrCodeDataUrl}
                  <div class="payment-qr-container">
                    <img src={qrCodeDataUrl} alt="Payment QR Code" class="payment-qr-code" />
                    <button class="payment-qr-download-btn" on:click={downloadQRCode} title="Download QR Code">
                      <Download size={14} />
                    </button>
                  </div>
                {/if}
                
                <label class="modal-form-label">Integrated Payment Address:</label>
                <div class="payment-address-display">
                  <code>{generatedAddress}</code>
                  <button class="payment-copy-btn" on:click={copyAddress}>
                    {#if copied}
                      <Check size={14} />
                    {:else}
                      <Copy size={14} />
                    {/if}
                  </button>
                </div>
                <p class="payment-result-hint">
                  Scan QR or share address to receive {amount || '0'} DERO
                  {#if comment}with note: "{comment}"{/if}
                </p>
              </div>
            {/if}
            
          {:else}
            <!-- Decode Integrated Address -->
            <div class="modal-form-group">
              <label class="modal-form-label" for="decode">Integrated Address</label>
              <textarea 
                id="decode"
                bind:value={addressToDecode} 
                placeholder="Paste an integrated address to decode..."
                rows="3"
                class="modal-input payment-textarea"
              ></textarea>
            </div>
            
            <button class="modal-btn modal-btn-primary payment-action-btn" on:click={decodeAddress} disabled={decoding || !addressToDecode.trim()}>
              {#if decoding}
                <Loader2 size={14} class="modal-spinner" />
                Decoding...
              {:else}
                Decode Address
              {/if}
            </button>
            
            {#if decodedResult}
              <div class="payment-result-box">
                <div class="payment-decoded-item">
                  <label class="modal-form-label">Base Address:</label>
                  <code class="payment-mono-sm">{decodedResult.address}</code>
                </div>
                
                {#if decodedResult.payload && decodedResult.payload.length > 0}
                  <div class="payment-decoded-item">
                    <label class="modal-form-label">Embedded Data:</label>
                    <div class="payment-payload-list">
                      {#each decodedResult.payload as item}
                        <div class="payment-payload-item">
                          <span class="payment-payload-name">
                            {#if item.name === 'C'}Comment
                            {:else if item.name === 'A'}Amount
                            {:else}{item.name}{/if}:
                          </span>
                          <span class="payment-payload-value">
                            {#if item.name === 'A'}
                              {formatAtomicAmount(item.value)} DERO
                            {:else}
                              {item.value}
                            {/if}
                          </span>
                        </div>
                      {/each}
                    </div>
                  </div>
                {:else}
                  <p class="payment-no-payload">No embedded payment data</p>
                {/if}
              </div>
            {/if}
          {/if}
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* PaymentRequestModal.svelte - Component-specific styles only
     Modal base patterns now in hologram.css (.modal-*) */
  
  /* Mode Tabs - Payment-specific tab navigation */
  .payment-mode-tabs {
    display: flex;
    gap: 4px;
    padding: var(--s-3) var(--s-5);
    background: var(--void-deep);
  }
  
  .payment-mode-tabs button {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 10px 16px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--r-md);
    color: var(--text-3);
    font-size: 13px;
    cursor: pointer;
    transition: all 0.2s;
  }
  
  .payment-mode-tabs button:hover {
    color: var(--text-2);
    background: var(--void-mid);
  }
  
  .payment-mode-tabs button.active {
    background: var(--void-mid);
    border-color: rgba(34, 211, 238, 0.3);
    color: var(--cyan-400);
  }
  
  /* Payment Action Button (full width) */
  .payment-action-btn {
    width: 100%;
    padding: 14px;
  }
  
  /* Textarea style */
  .payment-textarea {
    resize: vertical;
    min-height: 80px;
  }
  
  /* Result Box */
  .payment-result-box {
    margin-top: var(--s-5);
    padding: var(--s-4);
    background: var(--void-deep);
    border: 1px solid rgba(34, 211, 238, 0.2);
    border-radius: var(--r-lg);
  }
  
  /* QR Code Container */
  .payment-qr-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    margin-bottom: var(--s-4);
    padding-bottom: var(--s-4);
    border-bottom: 1px solid var(--border-dim);
    position: relative;
  }
  
  .payment-qr-code {
    width: 180px;
    height: 180px;
    border-radius: var(--r-lg);
    border: 3px solid var(--cyan-400);
    box-shadow: 0 0 20px rgba(34, 211, 238, 0.3),
                0 0 40px rgba(34, 211, 238, 0.1);
  }
  
  .payment-qr-download-btn {
    position: absolute;
    bottom: 20px;
    right: calc(50% - 100px);
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    color: var(--text-3);
    padding: 8px;
    border-radius: var(--r-md);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s;
  }
  
  .payment-qr-download-btn:hover {
    border-color: rgba(34, 211, 238, 0.3);
    color: var(--cyan-400);
    background: var(--void-up);
  }
  
  /* Address Display */
  .payment-address-display {
    display: flex;
    align-items: flex-start;
    gap: 8px;
  }
  
  .payment-address-display code {
    flex: 1;
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--cyan-400);
    word-break: break-all;
    line-height: 1.5;
  }
  
  .payment-copy-btn {
    flex-shrink: 0;
    background: var(--void-mid);
    border: 1px solid var(--border-dim);
    color: var(--text-3);
    padding: 8px;
    border-radius: var(--r-sm);
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .payment-copy-btn:hover {
    border-color: rgba(34, 211, 238, 0.3);
    color: var(--cyan-400);
  }
  
  .payment-result-hint {
    margin-top: 12px;
    font-size: 12px;
    color: var(--text-4);
  }
  
  /* Decoded Items */
  .payment-decoded-item {
    margin-bottom: var(--s-3);
  }
  
  .payment-decoded-item:last-child {
    margin-bottom: 0;
  }
  
  .payment-mono-sm {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-2);
    word-break: break-all;
    line-height: 1.5;
  }
  
  /* Payload List */
  .payment-payload-list {
    background: var(--void-mid);
    border-radius: var(--r-md);
    padding: 12px;
  }
  
  .payment-payload-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 6px 0;
    border-bottom: 1px solid var(--border-dim);
  }
  
  .payment-payload-item:last-child {
    border-bottom: none;
  }
  
  .payment-payload-name {
    font-size: 12px;
    color: var(--text-3);
  }
  
  .payment-payload-value {
    font-size: 13px;
    font-family: var(--font-mono);
    color: var(--cyan-400);
  }
  
  .payment-no-payload {
    font-size: 12px;
    color: var(--text-4);
    font-style: italic;
  }
</style>

