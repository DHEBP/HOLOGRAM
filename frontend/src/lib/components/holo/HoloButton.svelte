<script>
  /**
   * HoloButton - v6.1 Button Component
   * 
   * Wraps the global .btn styles from hologram.css with a clean Svelte interface.
   * 
   * Variants:
   * - primary: Solid cyan button (default)
   * - secondary: Outlined cyan button
   * - ghost: Text-only button
   * - danger: Red/error button
   * 
   * Props:
   * - variant: Button style variant
   * - block: Full width button
   * - disabled: Disabled state
   * - loading: Shows loading spinner
   * - type: Button type (button, submit, reset)
   * - href: If provided, renders as anchor
   */
  export let variant = 'primary'; // primary | secondary | ghost | danger
  export let block = false;
  export let disabled = false;
  export let loading = false;
  export let type = 'button';
  export let href = null;
  
  $: classes = [
    'btn',
    `btn-${variant}`,
    block && 'btn-block',
    loading && 'btn-loading'
  ].filter(Boolean).join(' ');
</script>

{#if href && !disabled}
  <a {href} class={classes} {...$$restProps}>
    {#if loading}
      <span class="btn-spinner"></span>
    {/if}
    <slot />
  </a>
{:else}
  <button
    {type}
    disabled={disabled || loading}
    class={classes}
    on:click
    on:mouseenter
    on:mouseleave
    {...$$restProps}
  >
    {#if loading}
      <span class="btn-spinner"></span>
    {/if}
    <slot />
  </button>
{/if}

<style>
  /* Component-specific styles only - base .btn comes from hologram.css */
  
  .btn-loading {
    position: relative;
    color: transparent !important;
  }
  
  .btn-spinner {
    position: absolute;
    width: 16px;
    height: 16px;
    border: 2px solid currentColor;
    border-right-color: transparent;
    border-radius: 50%;
    animation: btn-spin 0.6s linear infinite;
  }
  
  .btn-loading .btn-spinner {
    color: var(--text-1, #f8f8fc);
  }
  
  .btn-primary.btn-loading .btn-spinner {
    color: var(--void-pure, #000);
  }
  
  @keyframes btn-spin {
    to { transform: rotate(360deg); }
  }
</style>
