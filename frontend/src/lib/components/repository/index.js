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

// Only what is used from outside this folder. RepoFileTree, RepoFileView,
// RepoCommitRail, RepoDiff and RepoFork are internals of RepositoryView and had
// no consumer anywhere; exporting them invented a public surface for a private
// component set. SignatureBadge stays because VersionHistory.svelte, outside
// this folder, renders it too.
export { default as RepositoryView } from './RepositoryView.svelte';
export { default as SignatureBadge } from './SignatureBadge.svelte';
