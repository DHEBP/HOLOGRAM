/**
 * TELA Repository view
 *
 * Reads a TELA INDEX the way a forge reads a repository: file tree, rendered
 * README, source with syntax highlighting, commit history, diff between two
 * versions, and the author signature on each file.
 *
 * Usage:
 * import { RepositoryView } from '$lib/components/repository';
 */

export { default as RepositoryView } from './RepositoryView.svelte';
export { default as RepoFileTree } from './RepoFileTree.svelte';
export { default as RepoFileView } from './RepoFileView.svelte';
export { default as RepoCommitRail } from './RepoCommitRail.svelte';
export { default as RepoDiff } from './RepoDiff.svelte';
export { default as RepoFork } from './RepoFork.svelte';
export { default as SignatureBadge } from './SignatureBadge.svelte';
