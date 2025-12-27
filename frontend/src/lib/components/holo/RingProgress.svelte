<script>
  /**
   * RingProgress - v6.1 Circular Progress Component
   * 
   * Usage:
   * <RingProgress value={75} max={100}>
   *   <span class="ring-value">75%</span>
   * </RingProgress>
   */
  export let value = 75;
  export let max = 100;
  export let size = 140;
  export let strokeWidth = 8;
  export let animate = true;
  
  // Calculate ring parameters
  $: radius = (size - strokeWidth) / 2;
  $: circumference = 2 * Math.PI * radius;
  $: offset = circumference - (value / max) * circumference;
  $: center = size / 2;
  
  // Unique ID for gradient
  const gradientId = `ringGrad-${Math.random().toString(36).substr(2, 9)}`;
</script>

<div class="ring-wrap" style="width: {size}px; height: {size}px;">
  <svg width={size} height={size} viewBox="0 0 {size} {size}">
    <defs>
      <linearGradient id={gradientId} x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color: #22d3ee" />
        <stop offset="100%" style="stop-color: #a78bfa" />
      </linearGradient>
    </defs>
    
    <!-- Background ring -->
    <circle 
      class="ring-bg" 
      cx={center} 
      cy={center} 
      r={radius}
      stroke-width={strokeWidth}
    />
    
    <!-- Progress ring -->
    <circle 
      class="ring-fill" 
      class:animate
      cx={center} 
      cy={center} 
      r={radius}
      stroke="url(#{gradientId})"
      stroke-width={strokeWidth}
      stroke-dasharray={circumference}
      stroke-dashoffset={offset}
    />
  </svg>
  
  <div class="ring-content">
    <slot />
  </div>
</div>

<style>
  .ring-wrap {
    position: relative;
  }
  
  .ring-wrap svg {
    transform: rotate(-90deg);
  }
  
  .ring-bg {
    fill: none;
    stroke: var(--void-hover, #262634);
  }
  
  .ring-fill {
    fill: none;
    stroke-linecap: round;
    filter: drop-shadow(0 0 6px rgba(34, 211, 238, 0.4));
    transition: stroke-dashoffset 1.5s cubic-bezier(0.16, 1, 0.3, 1);
  }
  
  .ring-fill.animate {
    animation: ringIn 1.5s cubic-bezier(0.16, 1, 0.3, 1) forwards;
  }
  
  @keyframes ringIn {
    from { stroke-dashoffset: 377; }
  }
  
  .ring-content {
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    text-align: center;
  }
  
  /* Default content styling */
  .ring-content :global(.ring-value) {
    font-family: var(--font-mono);
    font-size: 1.75rem;
    font-weight: 700;
    color: var(--violet-300, #c4b5fd);
    text-shadow: 0 0 25px rgba(167, 139, 250, 0.4);
  }
  
  .ring-content :global(.ring-unit) {
    font-size: 11px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.15em;
    color: var(--text-4, #505068);
  }
</style>

