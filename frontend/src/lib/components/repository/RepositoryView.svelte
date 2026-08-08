<script>
  import { createEventDispatcher } from 'svelte';
  import {
    GetINDEXInfo,
    GetRepositoryFiles,
    GetDOCSignatures,
    GetCommitHistory,
    GetCommitContent,
    DiffCommits,
    CloneTELA
  } from '../../../../wailsjs/go/main/App.js';
  import { ClipboardSetText } from '../../../../wailsjs/runtime/runtime.js';
  import { toast } from '../../stores/appState.js';
  import { Icons } from '../holo';
  import { pickDefaultFile, entriesFromCommitFiles } from './repoFiles.js';
  import RepoFileTree from './RepoFileTree.svelte';
  import RepoFileView from './RepoFileView.svelte';
  import RepoCommitRail from './RepoCommitRail.svelte';
  import RepoDiff from './RepoDiff.svelte';
  import RepoFork from './RepoFork.svelte';

  export let scid = '';
  export let closable = true;

  const dispatch = createEventDispatcher();

  let loadedScid = '';

  // Header
  let info = null;
  let infoError = '';

  // Files at the current state
  let headEntries = [];
  let filesError = '';
  let filesLoading = false;
  let repoKind = '';
  let durl = '';

  // Signatures, keyed by DOC SCID.
  //
  // Loaded once per INDEX rather than per commit, on purpose: a DOC's signature
  // is written at install and never changes, while its code can be replaced. So
  // this always describes each file AS IT STANDS NOW, which is why the badges
  // are hidden entirely while a historical version is open.
  let signatures = {};
  let signaturesLoading = false;

  // History
  let commits = [];
  let commitsLoading = false;

  // What the main pane is showing
  let viewedCommit = null;       // null = current state
  let viewedEntries = [];
  let viewedLoading = false;
  let viewedWarning = '';
  let selectedName = '';

  let compareMode = false;
  let compareA = null;
  let compareB = null;
  let diffResult = null;
  let diffLoading = false;

  let cloning = false;
  let clonePrompt = '';

  let forkOpen = false;

  // Re-selecting a commit would otherwise re-clone the whole version from chain
  // every time. Keyed by commit number and thrown away with the component.
  const commitCache = new Map();

  $: if (scid && scid !== loadedScid) {
    loadedScid = scid;
    loadAll();
  }

  $: entries = viewedCommit ? viewedEntries : headEntries;
  $: selectedEntry = entries.find((e) => e.name === selectedName) || null;
  $: selectedSignature = selectedEntry?.scid ? signatures[selectedEntry.scid] || null : null;
  $: atHead = !viewedCommit;

  function resetState() {
    info = null;
    infoError = '';
    headEntries = [];
    filesError = '';
    repoKind = '';
    durl = '';
    signatures = {};
    commits = [];
    viewedCommit = null;
    viewedEntries = [];
    viewedWarning = '';
    selectedName = '';
    compareMode = false;
    compareA = null;
    compareB = null;
    diffResult = null;
    clonePrompt = '';
    forkOpen = false;
    commitCache.clear();
  }

  async function loadAll() {
    resetState();
    // Four independent reads. A failure in any one must not blank the others,
    // so each owns its own error surface.
    await Promise.all([loadInfo(), loadFiles(), loadSignatures(), loadCommits()]);
  }

  async function loadInfo() {
    try {
      const result = await GetINDEXInfo(scid);
      if (result?.success) {
        info = result;
        if (result.durl) durl = result.durl;
      } else {
        infoError = result?.error || 'This contract is not a TELA INDEX.';
      }
    } catch (error) {
      infoError = 'Could not read the INDEX contract.';
    }
  }

  async function loadFiles() {
    filesLoading = true;
    try {
      const result = await GetRepositoryFiles(scid);
      if (result?.success) {
        headEntries = result.files || [];
        repoKind = result.kind || '';
        if (result.durl) durl = result.durl;
        const first = pickDefaultFile(headEntries);
        selectedName = first?.name || '';
      } else {
        filesError = result?.error || 'This contract holds no readable TELA files.';
      }
    } catch (error) {
      filesError = 'Could not read this repository.';
    } finally {
      filesLoading = false;
    }
  }

  async function loadSignatures() {
    signaturesLoading = true;
    try {
      const result = await GetDOCSignatures(scid);
      if (result?.success) {
        const next = {};
        for (const sig of result.signatures || []) next[sig.scid] = sig;
        signatures = next;
      }
      // A failed signature read leaves the map empty and the badges absent,
      // which is the honest outcome: nothing is claimed about any file.
    } catch (error) {
      signatures = {};
    } finally {
      signaturesLoading = false;
    }
  }

  async function loadCommits() {
    commitsLoading = true;
    try {
      // GetCommitHistory, not GetCommitHistoryWithLabels: the labelled variant
      // diffs the last five commits to describe them, and each diff clones two
      // whole versions from chain. That is up to ten clones before the page can
      // paint. This one already carries a plain label per commit.
      const result = await GetCommitHistory(scid);
      if (result?.success) commits = result.commits || [];
    } catch (error) {
      commits = [];
    } finally {
      commitsLoading = false;
    }
  }

  async function openCommit(commit) {
    if (compareMode) {
      pickForCompare(commit);
      return;
    }

    if (commit.isCurrent) {
      backToLatest();
      return;
    }

    viewedCommit = commit;
    viewedWarning = '';

    const cached = commitCache.get(commit.number);
    if (cached) {
      viewedEntries = cached.entries;
      viewedWarning = cached.warning;
      selectedName = pickDefaultFile(cached.entries)?.name || '';
      return;
    }

    viewedLoading = true;
    viewedEntries = [];
    selectedName = '';
    try {
      const result = await GetCommitContent(scid, commit.number);
      if (viewedCommit?.number !== commit.number) return;
      if (result?.success) {
        const built = entriesFromCommitFiles(result.files, result.durl || durl);
        viewedEntries = built;
        viewedWarning = result.warning || (built.length === 0 ? result.message || '' : '');
        selectedName = pickDefaultFile(built)?.name || '';
        commitCache.set(commit.number, { entries: built, warning: viewedWarning });
      } else {
        viewedWarning = result?.error || 'This version could not be reconstructed.';
      }
    } catch (error) {
      if (viewedCommit?.number !== commit.number) return;
      viewedWarning = 'This version could not be reconstructed.';
    } finally {
      if (viewedCommit?.number === commit.number) viewedLoading = false;
    }
  }

  function backToLatest() {
    viewedCommit = null;
    viewedEntries = [];
    viewedWarning = '';
    selectedName = pickDefaultFile(headEntries)?.name || '';
  }

  function toggleCompare() {
    compareMode = !compareMode;
    compareA = null;
    compareB = null;
    diffResult = null;
    if (!compareMode) backToLatest();
  }

  async function pickForCompare(commit) {
    if (!compareA || (compareA && compareB)) {
      compareA = commit;
      compareB = null;
      diffResult = null;
      return;
    }
    if (compareA.number === commit.number) return;

    // Always read low → high so the diff describes going forward in time,
    // whichever order the two were clicked in.
    const [from, to] =
      compareA.number < commit.number ? [compareA, commit] : [commit, compareA];
    compareA = from;
    compareB = to;

    diffLoading = true;
    diffResult = null;
    try {
      const result = await DiffCommits(scid, from.number, to.number);
      if (result?.success) {
        diffResult = result;
      } else {
        toast.error(result?.error || 'Could not compare these versions');
        compareB = null;
      }
    } catch (error) {
      toast.error('Could not compare these versions');
      compareB = null;
    } finally {
      diffLoading = false;
    }
  }

  async function clone(allowUpdates = false) {
    if (cloning) return;
    cloning = true;
    clonePrompt = '';
    try {
      const result = await CloneTELA(scid, allowUpdates);
      if (result?.success) {
        toast.success(`Cloned to ${result.directory}`);
      } else if (result?.requiresConfirm) {
        clonePrompt = result.confirmMessage || 'This content has been updated. Clone the latest version?';
      } else {
        toast.error(result?.error || 'Clone failed');
      }
    } catch (error) {
      toast.error('Clone failed');
    } finally {
      cloning = false;
    }
  }

  async function copyScid() {
    try {
      await ClipboardSetText(scid);
      toast.success('SCID copied');
    } catch {
      toast.error('Could not copy SCID');
    }
  }

  function openApp() {
    dispatch('open', { scid, durl });
  }

  function shortOwner(addr) {
    if (!addr) return '';
    if (addr === 'anon') return 'anonymous';
    return addr.length > 24 ? `${addr.slice(0, 12)}…${addr.slice(-8)}` : addr;
  }

  $: fileCount = entries.filter((e) => e.kind === 'doc').length;
  $: otherCount = entries.length - fileCount;
</script>

<div class="repo-layout">
  <!-- Identity strip -->
  <div class="repo-header">
    <div class="repo-header-left">
      <div class="repo-header-title">
        <span class="repo-header-glyph">◈</span>
        <span class="repo-header-name">{info?.name || durl || 'Repository'}</span>
        {#if repoKind === 'DOC'}
          <span class="repo-tag">single document</span>
        {/if}
        {#if info?.currentVersion}
          <span class="repo-tag">v{info.currentVersion}</span>
        {/if}
        {#if info && info.isLatest === false}
          <span class="repo-tag warn" title="This INDEX was published against an older TELA contract version.">older contract</span>
        {/if}
        {#if info?.mods}
          <span class="repo-tag">mods {info.mods}</span>
        {/if}
      </div>
      <div class="repo-header-sub">
        {#if durl}<span class="repo-durl">{durl}</span>{/if}
        <button type="button" class="repo-scid" title={`${scid} — click to copy`} on:click={copyScid}>
          {scid.slice(0, 16)}…{scid.slice(-8)}
        </button>
        {#if info?.owner}
          <span class="repo-owner" title={info.owner}>owner {shortOwner(info.owner)}</span>
        {/if}
      </div>
      {#if info?.description}
        <p class="repo-desc">{info.description}</p>
      {/if}
    </div>

    <div class="repo-header-actions">
      {#if durl}
        <button type="button" class="repo-btn" on:click={openApp}>
          <Icons name="globe" size={13} />
          <span>Open app</span>
        </button>
      {/if}
      <button type="button" class="repo-btn" on:click={() => clone(false)} disabled={cloning}>
        <Icons name="download" size={13} />
        <span>{cloning ? 'Cloning…' : 'Clone'}</span>
      </button>
      {#if info?.docs?.length}
        <!-- Disabled while an earlier version is open: a fork always lists the
             documents the INDEX holds NOW, so offering it beside a historical
             file tree would promise a version it does not install. -->
        <button
          type="button"
          class="repo-btn"
          on:click={() => (forkOpen = true)}
          disabled={!atHead}
          title={atHead
            ? 'Publish your own INDEX listing these same documents'
            : 'Return to the latest version to fork — a fork lists what the INDEX holds now'}
        >
          <Icons name="git-branch" size={13} />
          <span>Fork</span>
        </button>
      {/if}
      {#if closable}
        <button type="button" class="repo-btn icon" title="Close" on:click={() => dispatch('close')}>
          <Icons name="close" size={14} />
        </button>
      {/if}
    </div>
  </div>

  {#if clonePrompt}
    <div class="repo-banner">
      <span class="repo-banner-text">{clonePrompt}</span>
      <span class="repo-banner-actions">
        <button type="button" class="repo-btn small" on:click={() => clone(true)}>Clone latest</button>
        <button type="button" class="repo-btn small ghost" on:click={() => (clonePrompt = '')}>Cancel</button>
      </span>
    </div>
  {/if}

  {#if infoError && !filesError}
    <div class="repo-banner subtle">
      <span class="repo-banner-text">{infoError} Files below are read directly from the contract.</span>
    </div>
  {/if}

  {#if viewedCommit}
    <div class="repo-banner">
      <span class="repo-banner-text">
        Viewing version {viewedCommit.number}{viewedCommit.height ? ` · block ${viewedCommit.height.toLocaleString()}` : ''}. This is
        not the current state of the repository.
      </span>
      <span class="repo-banner-actions">
        <button type="button" class="repo-btn small" on:click={backToLatest}>Back to latest</button>
      </span>
    </div>
  {/if}

  <div class="repo-body">
    <!-- Sidebar: files, then history -->
    <aside class="repo-sidebar">
      <div class="repo-sidebar-block">
        <div class="repo-sidebar-heading">
          <span>Files</span>
          {#if entries.length > 0}
            <span class="repo-count">
              {fileCount}{otherCount > 0 ? ` + ${otherCount}` : ''}
            </span>
          {/if}
        </div>
        {#if filesError}
          <div class="repo-sidebar-note">{filesError}</div>
        {:else}
          <RepoFileTree
            entries={entries}
            signatures={signatures}
            selected={selectedName}
            loading={filesLoading || viewedLoading}
            showSignatures={atHead}
            signaturesLoading={signaturesLoading}
            on:select={(e) => (selectedName = e.detail.name)}
          />
        {/if}
        {#if !atHead && entries.length > 0}
          <p class="repo-sidebar-note">
            Author signatures describe each file as it stands now, so they are not
            shown for an earlier version.
          </p>
        {/if}
      </div>

      <div class="repo-sidebar-block">
        <div class="repo-sidebar-heading">
          <span>History</span>
          {#if commits.length > 0}
            <button
              type="button"
              class="repo-compare-toggle"
              class:active={compareMode}
              on:click={toggleCompare}
            >
              {compareMode ? 'exit compare' : 'compare'}
            </button>
          {/if}
        </div>
        <RepoCommitRail
          commits={commits}
          loading={commitsLoading}
          selected={viewedCommit?.number ?? null}
          compareMode={compareMode}
          compareA={compareA}
          compareB={compareB}
          on:select={(e) => openCommit(e.detail)}
        />
        {#if compareMode && !compareB}
          <p class="repo-sidebar-note">
            {compareA ? `From v${compareA.number}. Pick a second version.` : 'Pick two versions to compare.'}
          </p>
        {/if}
      </div>
    </aside>

    <!-- Main pane -->
    <section class="repo-content">
      {#if compareMode}
        <RepoDiff
          result={diffResult}
          loading={diffLoading}
          fromLabel={compareA ? `v${compareA.number}` : ''}
          toLabel={compareB ? `v${compareB.number}` : ''}
        />
      {:else if viewedLoading}
        <div class="repo-content-status">
          <span class="repo-content-spinner"></span>
          <span>Reconstructing version {viewedCommit?.number} from chain…</span>
        </div>
      {:else if viewedWarning && entries.length === 0}
        <div class="repo-content-status wrap">{viewedWarning}</div>
      {:else if filesError}
        <div class="repo-empty">
          <div class="repo-empty-icon">○</div>
          <p class="repo-empty-title">Nothing to show</p>
          <p class="repo-empty-text">{filesError}</p>
        </div>
      {:else if filesLoading}
        <div class="repo-content-status">
          <span class="repo-content-spinner"></span>
          <span>Reading contracts…</span>
        </div>
      {:else}
        {#if viewedWarning}
          <div class="repo-inline-note">{viewedWarning}</div>
        {/if}
        <RepoFileView
          entry={selectedEntry}
          signature={selectedSignature}
          showSignature={atHead}
          on:external={(e) => dispatch('external', e.detail)}
        />
      {/if}
    </section>
  </div>
</div>

<RepoFork
  show={forkOpen}
  scid={scid}
  source={info}
  on:close={() => (forkOpen = false)}
/>

<style>
  .repo-layout {
    height: 100%;
    display: flex;
    flex-direction: column;
    background: var(--void-base);
    overflow: hidden;
  }

  /* Rectangular, full-width, no rounded corners — matches .page-header. */
  .repo-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-4) var(--s-6);
    background: var(--void-mid);
    border-bottom: 1px solid var(--border-dim);
    flex-shrink: 0;
  }

  .repo-header-left {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--s-2);
  }

  .repo-header-title {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    flex-wrap: wrap;
  }

  .repo-header-glyph {
    color: var(--cyan-400);
    font-size: 14px;
  }

  .repo-header-name {
    font-family: var(--font-mono);
    font-size: 16px;
    font-weight: 600;
    letter-spacing: 0.08em;
    color: var(--text-1);
  }

  .repo-tag {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-4);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    padding: 1px var(--s-2);
    white-space: nowrap;
  }

  .repo-tag.warn {
    color: var(--status-warn);
    border-color: rgba(251, 191, 36, 0.4);
    cursor: help;
  }

  .repo-header-sub {
    display: flex;
    align-items: center;
    gap: var(--s-3);
    flex-wrap: wrap;
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-4);
  }

  .repo-durl {
    color: var(--cyan-400);
  }

  .repo-scid {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-4);
    background: transparent;
    border: none;
    border-bottom: 1px dashed var(--border-default);
    padding: 0;
    cursor: pointer;
    transition: color var(--dur-fast) ease;
  }

  .repo-scid:hover {
    color: var(--text-2);
  }

  .repo-owner {
    color: var(--text-5);
  }

  .repo-desc {
    margin: 0;
    font-size: 12.5px;
    color: var(--text-3);
    max-width: 70ch;
  }

  .repo-header-actions {
    display: flex;
    align-items: center;
    gap: var(--s-2);
    flex-shrink: 0;
  }

  .repo-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--s-2);
    padding: var(--s-2) var(--s-3);
    font-family: var(--font-mono);
    font-size: 12px;
    color: var(--text-3);
    background: var(--void-up);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
    cursor: pointer;
    transition: all var(--dur-fast) ease;
    white-space: nowrap;
  }

  .repo-btn:hover:not(:disabled) {
    background: var(--void-surface);
    border-color: var(--border-accent);
    color: var(--text-1);
  }

  .repo-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .repo-btn.icon {
    padding: var(--s-2);
  }

  .repo-btn.small {
    padding: 2px var(--s-3);
    font-size: 11px;
  }

  .repo-btn.ghost {
    background: transparent;
  }

  .repo-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-4);
    padding: var(--s-2) var(--s-6);
    background: var(--void-up);
    border-bottom: 1px solid var(--border-dim);
    font-size: 12px;
    color: var(--text-3);
    flex-shrink: 0;
  }

  .repo-banner.subtle {
    color: var(--text-4);
  }

  .repo-banner-text {
    min-width: 0;
  }

  .repo-banner-actions {
    display: flex;
    gap: var(--s-2);
    flex-shrink: 0;
  }

  .repo-body {
    flex: 1;
    display: flex;
    overflow: hidden;
    min-height: 0;
  }

  .repo-sidebar {
    width: 260px;
    flex-shrink: 0;
    background: var(--void-deep);
    border-right: 1px solid var(--border-dim);
    padding: var(--s-4) var(--s-2);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--s-5);
  }

  .repo-sidebar-block {
    display: flex;
    flex-direction: column;
    gap: var(--s-1);
  }

  .repo-sidebar-heading {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--s-2);
    padding: 0 var(--s-3) var(--s-2);
    font-family: var(--font-mono);
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.12em;
    color: var(--text-4);
  }

  .repo-count {
    color: var(--text-5);
  }

  .repo-compare-toggle {
    font-family: var(--font-mono);
    font-size: 10px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-4);
    background: transparent;
    border: 1px solid var(--border-dim);
    border-radius: var(--r-xs);
    padding: 1px var(--s-2);
    cursor: pointer;
    transition: all var(--dur-fast) ease;
  }

  .repo-compare-toggle:hover {
    color: var(--text-2);
  }

  .repo-compare-toggle.active {
    color: var(--cyan-400);
    border-color: var(--border-accent);
  }

  .repo-sidebar-note {
    padding: var(--s-2) var(--s-3) 0;
    margin: 0;
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-5);
  }

  .repo-content {
    flex: 1;
    min-width: 0;
    overflow-y: auto;
    padding: var(--s-5) var(--s-6);
  }

  .repo-content-status {
    display: flex;
    align-items: center;
    justify-content: center;
    gap: var(--s-3);
    padding: var(--s-16) var(--s-6);
    color: var(--text-4);
    font-size: 13px;
  }

  .repo-content-status.wrap {
    text-align: center;
    max-width: 60ch;
    margin: 0 auto;
    line-height: 1.6;
  }

  .repo-content-spinner {
    width: 16px;
    height: 16px;
    border: 2px solid var(--cyan-500);
    border-top-color: transparent;
    border-radius: var(--r-full);
    animation: repo-view-spin 0.6s linear infinite;
  }

  @keyframes repo-view-spin {
    to { transform: rotate(360deg); }
  }

  .repo-inline-note {
    margin-bottom: var(--s-3);
    padding: var(--s-2) var(--s-3);
    font-size: 12px;
    color: var(--text-4);
    background: var(--void-deep);
    border: 1px solid var(--border-dim);
    border-radius: var(--r-sm);
  }

  .repo-empty {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: var(--s-16) var(--s-6);
  }

  .repo-empty-icon {
    font-size: 36px;
    color: var(--text-5);
    margin-bottom: var(--s-3);
  }

  .repo-empty-title {
    font-family: var(--font-mono);
    font-size: 14px;
    color: var(--text-2);
    margin: 0 0 var(--s-1) 0;
  }

  .repo-empty-text {
    font-size: 12px;
    color: var(--text-4);
    margin: 0;
    max-width: 50ch;
  }
</style>
