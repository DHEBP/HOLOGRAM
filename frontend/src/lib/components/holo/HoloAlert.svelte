<script>
  /**
   * HoloAlert - v6.1 Alert/Notification Component
   * 
   * Displays alert messages with different severity levels.
   * 
   * Variants:
   * - error: Red - for errors and critical issues
   * - warning: Yellow - for warnings and cautions
   * - success: Green - for success messages
   * - info: Cyan - for informational messages (default)
   * 
   * Props:
   * - variant: Alert style variant
   * - dismissible: Show close button
   * - icon: Show icon (auto-selected based on variant)
   */
  import { createEventDispatcher } from 'svelte';
  import Icons from './Icons.svelte';
  
  export let variant = 'info'; // error | warning | success | info
  export let dismissible = false;
  export let icon = true;
  
  const dispatch = createEventDispatcher();
  
  let visible = true;
  
  const iconMap = {
    error: 'alertCircle',
    warning: 'alertTriangle',
    success: 'checkCircle',
    info: 'info'
  };
  
  function dismiss() {
    visible = false;
    dispatch('dismiss');
  }
</script>

{#if visible}
  <div class="holo-alert holo-alert-{variant}" role="alert">
    {#if icon}
      <div class="holo-alert-icon">
        <Icons name={iconMap[variant]} size={18} />
      </div>
    {/if}
    
    <div class="holo-alert-content">
      <slot />
    </div>
    
    {#if dismissible}
      <button class="holo-alert-dismiss" on:click={dismiss} aria-label="Dismiss">
        <Icons name="x" size={14} />
      </button>
    {/if}
  </div>
{/if}

<style>
  .holo-alert {
    display: flex;
    align-items: flex-start;
    gap: var(--s-3, 12px);
    padding: var(--s-3, 12px) var(--s-4, 16px);
    border-radius: var(--r-md, 8px);
    border: 1px solid;
    font-size: 13px;
    line-height: 1.5;
  }
  
  /* Variants */
  .holo-alert-error {
    background: rgba(239, 68, 68, 0.08);
    border-color: rgba(239, 68, 68, 0.25);
    color: var(--red-400, #f87171);
  }
  
  .holo-alert-warning {
    background: rgba(251, 191, 36, 0.08);
    border-color: rgba(251, 191, 36, 0.25);
    color: var(--status-warn, #fbbf24);
  }
  
  .holo-alert-success {
    background: rgba(52, 211, 153, 0.08);
    border-color: rgba(52, 211, 153, 0.25);
    color: var(--status-ok, #34d399);
  }
  
  .holo-alert-info {
    background: rgba(34, 211, 238, 0.08);
    border-color: rgba(34, 211, 238, 0.25);
    color: var(--cyan-400, #22d3ee);
  }
  
  .holo-alert-icon {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    margin-top: 1px;
  }
  
  .holo-alert-content {
    flex: 1;
    min-width: 0;
  }
  
  .holo-alert-dismiss {
    flex-shrink: 0;
    background: transparent;
    border: none;
    color: inherit;
    opacity: 0.6;
    cursor: pointer;
    padding: 4px;
    margin: -4px -4px -4px 0;
    border-radius: var(--r-xs, 3px);
    transition: opacity 150ms;
  }
  
  .holo-alert-dismiss:hover {
    opacity: 1;
  }
</style>
