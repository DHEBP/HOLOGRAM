<script>
  /**
   * LoadingSpinner - v6.1 Loading Indicator Component
   * 
   * A styled loading spinner with optional text.
   * 
   * Props:
   * - size: Spinner size in pixels (default: 32)
   * - text: Optional loading text
   * - overlay: Show as full overlay (covers parent)
   * - inline: Display inline with content
   */
  export let size = 32;
  export let text = '';
  export let overlay = false;
  export let inline = false;
</script>

<div 
  class="loading-spinner-container" 
  class:overlay 
  class:inline
  role="status"
  aria-label={text || 'Loading'}
>
  <div 
    class="loading-spinner" 
    style:width="{size}px" 
    style:height="{size}px"
    style:border-width="{Math.max(2, size / 10)}px"
  ></div>
  
  {#if text}
    <p class="loading-text">{text}</p>
  {/if}
</div>

<style>
  .loading-spinner-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--s-3, 12px);
    padding: var(--s-8, 32px);
  }
  
  .loading-spinner-container.inline {
    flex-direction: row;
    padding: 0;
  }
  
  .loading-spinner-container.overlay {
    position: absolute;
    inset: 0;
    background: var(--void-base, rgba(12, 12, 20, 0.9));
    z-index: 100;
  }
  
  .loading-spinner {
    border: 3px solid var(--void-hover, #262634);
    border-top-color: var(--cyan-400, #22d3ee);
    border-radius: 50%;
    animation: spinner-rotate 0.8s linear infinite;
  }
  
  .loading-text {
    font-size: 13px;
    color: var(--text-3, #707088);
    margin: 0;
  }
  
  @keyframes spinner-rotate {
    to {
      transform: rotate(360deg);
    }
  }
</style>
