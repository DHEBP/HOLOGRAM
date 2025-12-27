<script>
  /**
   * HoloInput - v6.1 Form Input Component
   * 
   * A styled input with optional label, hint, and error states.
   * Uses global .form-group, .form-label, .input styles from hologram.css.
   * 
   * Props:
   * - label: Input label text
   * - hint: Helper text below input
   * - error: Error message (shows error state)
   * - type: Input type (text, password, email, number, etc.)
   * - placeholder: Placeholder text
   * - value: Bound value
   * - disabled: Disabled state
   * - readonly: Read-only state
   * - mono: Use monospace font
   * - copyable: Show copy button
   */
  import { createEventDispatcher } from 'svelte';
  import Icons from './Icons.svelte';
  
  export let label = '';
  export let hint = '';
  export let error = '';
  export let type = 'text';
  export let placeholder = '';
  export let value = '';
  export let disabled = false;
  export let readonly = false;
  export let mono = false;
  export let copyable = false;
  export let id = `input-${Math.random().toString(36).substr(2, 9)}`;
  
  const dispatch = createEventDispatcher();
  
  let copied = false;
  
  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(value);
      copied = true;
      dispatch('copy', { value });
      setTimeout(() => copied = false, 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }
  
  $: inputClasses = [
    'input',
    error && 'input-error',
    mono && 'input-mono'
  ].filter(Boolean).join(' ');
</script>

<div class="form-group">
  {#if label}
    <label for={id} class="form-label">{label}</label>
  {/if}
  
  <div class="input-wrap" class:has-action={copyable}>
    {#if type === 'password'}
      <input
        {id}
        type="password"
        {placeholder}
        {disabled}
        {readonly}
        class={inputClasses}
        bind:value
        on:input
        on:change
        on:blur
        on:focus
        on:keydown
        on:keyup
        {...$$restProps}
      />
    {:else if type === 'number'}
      <input
        {id}
        type="number"
        {placeholder}
        {disabled}
        {readonly}
        class={inputClasses}
        bind:value
        on:input
        on:change
        on:blur
        on:focus
        on:keydown
        on:keyup
        {...$$restProps}
      />
    {:else if type === 'email'}
      <input
        {id}
        type="email"
        {placeholder}
        {disabled}
        {readonly}
        class={inputClasses}
        bind:value
        on:input
        on:change
        on:blur
        on:focus
        on:keydown
        on:keyup
        {...$$restProps}
      />
    {:else}
      <input
        {id}
        type="text"
        {placeholder}
        {disabled}
        {readonly}
        class={inputClasses}
        bind:value
        on:input
        on:change
        on:blur
        on:focus
        on:keydown
        on:keyup
        {...$$restProps}
      />
    {/if}
    
    {#if copyable && value}
      <button
        type="button"
        class="input-action"
        on:click={copyToClipboard}
        title={copied ? 'Copied!' : 'Copy to clipboard'}
      >
        <Icons name={copied ? 'check' : 'copy'} size={14} />
      </button>
    {/if}
  </div>
  
  {#if error}
    <span class="form-error">{error}</span>
  {:else if hint}
    <span class="form-hint">{hint}</span>
  {/if}
</div>

<style>
  /* Component-specific styles - base styles come from hologram.css */
  
  .input-wrap {
    position: relative;
  }
  
  .input-wrap.has-action .input {
    padding-right: 40px;
  }
  
  .input-action {
    position: absolute;
    right: var(--s-2, 8px);
    top: 50%;
    transform: translateY(-50%);
    background: transparent;
    border: none;
    color: var(--text-4, #505068);
    cursor: pointer;
    padding: var(--s-2, 8px);
    border-radius: var(--r-xs, 3px);
    transition: all 150ms;
  }
  
  .input-action:hover {
    color: var(--cyan-400, #22d3ee);
    background: rgba(34, 211, 238, 0.1);
  }
  
  .input-mono {
    font-family: var(--font-mono, 'JetBrains Mono', monospace);
  }
</style>
