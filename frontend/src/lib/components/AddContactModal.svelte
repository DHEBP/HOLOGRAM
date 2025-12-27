<script>
  import { createEventDispatcher } from 'svelte';
  import { AddContact, UpdateContact } from '../../../wailsjs/go/main/App.js';
  import { toast } from '../stores/appState.js';
  import { X, Users, Loader2, AlertTriangle } from 'lucide-svelte';
  
  export let show = false;
  export let editContact = null; // If set, we're editing an existing contact
  
  const dispatch = createEventDispatcher();
  
  let label = '';
  let address = '';
  let notes = '';
  let loading = false;
  let error = null;
  
  $: isValidAddress = address.startsWith('dero1') || address.startsWith('deto1');
  $: canSave = label.trim() && isValidAddress;
  $: isEditing = !!editContact;
  
  // Populate form when editing
  $: if (editContact) {
    label = editContact.label || '';
    address = editContact.address || '';
    notes = editContact.notes || '';
  }
  
  function close() {
    show = false;
    reset();
    dispatch('close');
  }
  
  function reset() {
    label = '';
    address = '';
    notes = '';
    error = null;
    loading = false;
  }
  
  async function save() {
    if (!canSave) return;
    
    loading = true;
    error = null;
    
    try {
      let result;
      
      if (isEditing) {
        result = await UpdateContact(editContact.id, label.trim(), address, notes.trim());
      } else {
        result = await AddContact(label.trim(), address, notes.trim());
      }
      
      if (result.success) {
        toast.success(isEditing ? 'Contact updated!' : 'Contact added!');
        dispatch('saved', result.contact || { label, address, notes });
        close();
      } else {
        error = result.error || 'Failed to save contact';
      }
    } catch (err) {
      error = err.message || 'Failed to save contact';
    } finally {
      loading = false;
    }
  }
  
  function handleKeydown(e) {
    if (e.key === 'Escape') close();
    if (e.key === 'Enter' && canSave && !loading) save();
  }
</script>

<svelte:window on:keydown={handleKeydown} />

{#if show}
  <div class="modal-overlay" on:click|self={close}>
    <div class="modal">
      <div class="modal-header">
        <div class="modal-title">
          <Users size={18} />
          <span>{isEditing ? 'Edit Contact' : 'Add Contact'}</span>
        </div>
        <button class="modal-close" on:click={close}>
          <X size={18} />
        </button>
      </div>
      
      <div class="modal-body">
        <div class="form-group">
          <label class="form-label">Label *</label>
          <input 
            type="text" 
            class="input" 
            bind:value={label} 
            placeholder="e.g., Alice, Trading Partner..."
          />
        </div>
        
        <div class="form-group">
          <label class="form-label">Address *</label>
          <input 
            type="text" 
            class="input mono" 
            class:input-error={address && !isValidAddress}
            bind:value={address} 
            placeholder="dero1..."
          />
          {#if address && !isValidAddress}
            <span class="form-error">Invalid DERO address format</span>
          {/if}
        </div>
        
        <div class="form-group">
          <label class="form-label">Notes (optional)</label>
          <textarea 
            class="input textarea" 
            bind:value={notes} 
            placeholder="Any notes about this contact..."
            rows="3"
          ></textarea>
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
        <button class="btn btn-primary" disabled={!canSave || loading} on:click={save}>
          {#if loading}
            <Loader2 size={14} class="spin" />
            Saving...
          {:else}
            {isEditing ? 'Update' : 'Add Contact'}
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
    max-width: 440px;
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
  
  .mono {
    font-family: var(--font-mono);
    font-size: 12px;
  }
  
  .textarea {
    resize: vertical;
    min-height: 60px;
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
