/**
 * HOLOGRAM v6.1 Component Library
 * 
 * Barrel export for all Holo components.
 * 
 * Usage:
 * import { HoloCard, HoloButton, HoloInput, HoloAlert, HoloBadge } from '$lib/components/holo';
 * import { EmptyState, LoadingSpinner, Icons } from '$lib/components/holo';
 */

// Display Components
export { default as HoloCard } from './HoloCard.svelte';
export { default as HoloBadge } from './HoloBadge.svelte';
export { default as RingProgress } from './RingProgress.svelte';
export { default as DotIndicator } from './DotIndicator.svelte';

// Form Components
export { default as HoloButton } from './HoloButton.svelte';
export { default as HoloInput } from './HoloInput.svelte';

// Feedback Components
export { default as HoloAlert } from './HoloAlert.svelte';
export { default as EmptyState } from './EmptyState.svelte';
export { default as LoadingSpinner } from './LoadingSpinner.svelte';

// Icons
export { default as Icons } from './Icons.svelte';
export { default as Icon } from './Icons.svelte';  // Alias for backwards compatibility

