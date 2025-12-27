<script>
  /**
   * EmptyState - v6.1 Empty State Component
   * 
   * Displays a centered message when content is empty or unavailable.
   * 
   * Props:
   * - icon: Icon name from Icons component
   * - title: Main heading text
   * - description: Secondary descriptive text
   * 
   * Slots:
   * - default: Optional action button area
   * - icon: Custom icon content (replaces icon prop)
   */
  import Icons from './Icons.svelte';
  
  export let icon = 'inbox';
  export let title = '';
  export let description = '';
</script>

<div class="empty-state">
  <div class="empty-state-icon">
    <slot name="icon">
      <Icons name={icon} size={48} />
    </slot>
  </div>
  
  {#if title}
    <h3 class="empty-state-title">{title}</h3>
  {/if}
  
  {#if description}
    <p class="empty-state-description">{description}</p>
  {/if}
  
  {#if $$slots.default}
    <div class="empty-state-actions">
      <slot />
    </div>
  {/if}
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: var(--s-12, 48px) var(--s-6, 24px);
    min-height: 200px;
  }
  
  .empty-state-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: var(--s-4, 16px);
    color: var(--text-4, #505068);
    opacity: 0.6;
  }
  
  .empty-state-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-1, #f8f8fc);
    margin: 0 0 var(--s-2, 8px) 0;
  }
  
  .empty-state-description {
    font-size: 13px;
    color: var(--text-3, #707088);
    margin: 0 0 var(--s-6, 24px) 0;
    max-width: 360px;
    line-height: 1.5;
  }
  
  .empty-state-actions {
    display: flex;
    gap: var(--s-3, 12px);
  }
</style>
